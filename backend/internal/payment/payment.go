// Package payment connects orders to ECPay.
//
// A payment only counts when ECPay's signed result says so: either the
// server-to-server callback (ReturnURL) or an active trade query made by the
// backend. Whatever the browser reports after returning from ECPay is ignored.
// Applying a result is idempotent: the payment row is locked and only a
// PENDING payment is ever processed.
package payment

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"auction-platform/backend/internal/apperr"
	"auction-platform/backend/internal/audit"
	"auction-platform/backend/internal/database/db"
	"auction-platform/backend/internal/ecpay"
	"auction-platform/backend/internal/notify"
	"auction-platform/backend/internal/order"
	"auction-platform/backend/internal/store"
)

const (
	StatusPending        = "PENDING"
	StatusPaid           = "PAID"
	StatusDuplicate      = "DUPLICATE"
	StatusAmountMismatch = "AMOUNT_MISMATCH"

	// Limits checkout attempts per order to stop accidental or abusive loops.
	maxAttemptsPerMinute = 5
)

var (
	ErrDisabled   = apperr.Conflict("PAYMENT_DISABLED", "伺服器尚未設定金流")
	ErrNotPayable = apperr.Conflict("ORDER_NOT_PAYABLE", "此訂單目前無法付款")
	ErrTooMany    = apperr.TooMany("PAYMENT_RATE_LIMITED", "付款請求過於頻繁，請稍後再試")
)

type Config struct {
	// PublicBaseURL is the site origin ECPay and browsers can reach. The
	// frontend proxies /api, so the callback is <PublicBaseURL>/api/payments/ecpay/notify.
	PublicBaseURL string
	// AllowSimulated accepts ECPay's "simulate paid" notifications, which
	// only exist in the stage environment. Must be false in production.
	AllowSimulated bool
}

type Service struct {
	store    *store.Store
	ecpay    *ecpay.Client // nil when payments are not configured
	dispatch notify.Dispatcher
	cfg      Config
	now      func() time.Time
}

func NewService(st *store.Store, client *ecpay.Client, d notify.Dispatcher, cfg Config, now func() time.Time) *Service {
	return &Service{store: st, ecpay: client, dispatch: d, cfg: cfg, now: now}
}

// NotifyPath is the callback route; it is exempt from CSRF because ECPay
// calls it directly and every request is verified by its CheckMacValue.
const NotifyPath = "/api/payments/ecpay/notify"

// newTradeNo returns a unique 20-character alphanumeric MerchantTradeNo.
func newTradeNo(now time.Time) string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return "CB" + now.UTC().Format("060102150405") + string(b)
}

// itemName joins product titles with "#" (ECPay shows each as a line).
func itemName(titles []string) string {
	var parts []string
	for _, t := range titles {
		parts = append(parts, strings.ReplaceAll(t, "#", " "))
	}
	s := strings.Join(parts, "#")
	if r := []rune(s); len(r) > 200 {
		s = string(r[:200])
	}
	if s == "" {
		s = "Critter Bid 商品"
	}
	return s
}

// Checkout is the auto-submitted form that sends the buyer to ECPay.
type Checkout struct {
	Action string            `json:"action"`
	Fields map[string]string `json:"fields"`
}

// CreateCheckout starts a payment attempt for an order awaiting payment.
func (s *Service) CreateCheckout(ctx context.Context, buyer, orderID uuid.UUID) (Checkout, error) {
	if s.ecpay == nil {
		return Checkout{}, ErrDisabled
	}
	var out Checkout
	err := s.store.InTx(ctx, func(q *db.Queries) error {
		o, err := q.GetOrderForUpdate(ctx, orderID)
		if store.IsNoRows(err) || (err == nil && o.BuyerID != buyer) {
			if err == nil && o.SellerID == buyer {
				return order.ErrNotBuyer
			}
			return order.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock order: %w", err)
		}
		if order.Status(o.Status) != order.AwaitingPayment {
			return ErrNotPayable
		}
		now := s.now().UTC()
		recent, err := q.CountRecentPayments(ctx, db.CountRecentPaymentsParams{OrderID: o.ID, Since: now.Add(-time.Minute)})
		if err != nil {
			return err
		}
		if recent >= maxAttemptsPerMinute {
			return ErrTooMany
		}
		items, err := q.ListOrderItems(ctx, uuid.NullUUID{UUID: o.ID, Valid: true})
		if err != nil {
			return err
		}
		titles := make([]string, len(items))
		for i, it := range items {
			titles[i] = it.Title
		}
		p, err := q.CreatePayment(ctx, db.CreatePaymentParams{OrderID: o.ID, MerchantTradeNo: newTradeNo(now), Amount: o.TotalAmount, CreatedAt: now})
		if err != nil {
			return fmt.Errorf("create payment: %w", err)
		}
		base := strings.TrimRight(s.cfg.PublicBaseURL, "/")
		out.Action, out.Fields = s.ecpay.CheckoutForm(ecpay.Checkout{
			MerchantTradeNo: p.MerchantTradeNo, Amount: p.Amount, ItemName: itemName(titles),
			TradeDesc: "Critter Bid 訂單", Time: now,
			ReturnURL:     base + NotifyPath,
			ClientBackURL: base + "/orders/" + o.ID.String() + "?payment=return",
		})
		return audit.Record(ctx, q, now, audit.Entry{
			Actor: buyer, Action: audit.ActionPaymentCreate, TargetType: audit.TargetPayment, TargetID: p.ID.String(),
			Metadata: map[string]any{"order_id": o.ID.String(), "merchant_trade_no": p.MerchantTradeNo, "amount": p.Amount},
		})
	})
	return out, err
}

// HandleCallback processes ECPay's server notification and returns the body
// ECPay expects ("1|OK" stops its retries). ok is false for requests that
// fail signature verification.
func (s *Service) HandleCallback(ctx context.Context, params map[string]string) (body string, ok bool) {
	if s.ecpay == nil {
		return "0|Payment disabled", false
	}
	res, err := s.ecpay.ParseCallback(params)
	if err != nil {
		slog.WarnContext(ctx, "ecpay callback rejected", "err", err, "merchant_trade_no", params["MerchantTradeNo"])
		return "0|CheckMacValue Error", false
	}
	if err := s.apply(ctx, res, "callback"); err != nil {
		if errors.Is(err, errUnknownTrade) {
			// Signed by ECPay but not ours (e.g. another environment); do not retry.
			slog.WarnContext(ctx, "ecpay callback for unknown trade", "merchant_trade_no", res.MerchantTradeNo)
			return "1|OK", true
		}
		// Transient failure: let ECPay retry later.
		slog.ErrorContext(ctx, "ecpay callback apply failed", "err", err, "merchant_trade_no", res.MerchantTradeNo)
		return "0|Error", true
	}
	return "1|OK", true
}

// Sync actively queries ECPay for the order's pending payments, covering a
// delayed or lost callback (e.g. the buyer closed the page). Either party of
// the order may trigger it.
func (s *Service) Sync(ctx context.Context, user, orderID uuid.UUID) error {
	if s.ecpay == nil {
		return ErrDisabled
	}
	o, err := s.store.GetOrder(ctx, orderID)
	if store.IsNoRows(err) || (err == nil && o.BuyerID != user && o.SellerID != user) {
		return order.ErrNotFound
	}
	if err != nil {
		return err
	}
	if order.Status(o.Status) != order.AwaitingPayment {
		return nil
	}
	tradeNos, err := s.store.ListPendingPaymentTradeNos(ctx, o.ID)
	if err != nil {
		return err
	}
	for _, no := range tradeNos {
		res, err := s.ecpay.QueryTrade(ctx, no, s.now())
		if err != nil {
			slog.WarnContext(ctx, "ecpay query failed", "merchant_trade_no", no, "err", err)
			continue
		}
		if !res.Paid {
			continue
		}
		if err := s.apply(ctx, res, "query"); err != nil {
			return err
		}
	}
	return nil
}

var errUnknownTrade = errors.New("unknown merchant trade no")

// resultJSON keeps only non-sensitive fields of the provider result.
func resultJSON(res ecpay.Result, source string) []byte {
	b, _ := json.Marshal(map[string]any{
		"source": source, "trade_no": res.TradeNo, "amount": res.Amount, "payment_type": res.PaymentType,
		"payment_date": res.PaymentDate, "rtn_msg": res.RtnMsg, "simulated": res.Simulated,
	})
	return b
}

// apply records a verified result. Only PENDING payments are processed, so
// repeated or late notifications are no-ops.
func (s *Service) apply(ctx context.Context, res ecpay.Result, source string) error {
	var batch notify.Batch
	err := s.store.InTx(ctx, func(q *db.Queries) error {
		p, err := q.GetPaymentByTradeNoForUpdate(ctx, res.MerchantTradeNo)
		if store.IsNoRows(err) {
			return errUnknownTrade
		}
		if err != nil {
			return fmt.Errorf("lock payment: %w", err)
		}
		if p.Status != StatusPending || !res.Paid {
			return nil // already processed, or not a successful payment
		}
		now := s.now().UTC()
		record := func(status string, meta map[string]any) error {
			var paidAt *time.Time
			if status == StatusPaid || status == StatusDuplicate {
				paidAt = &now
			}
			if err := q.SetPaymentResult(ctx, db.SetPaymentResultParams{
				ID: p.ID, Status: status, ProviderTradeNo: &res.TradeNo, PaymentType: &res.PaymentType,
				PaidAt: paidAt, Result: resultJSON(res, source), UpdatedAt: now,
			}); err != nil {
				return fmt.Errorf("set payment result: %w", err)
			}
			m := map[string]any{"from": p.Status, "to": status, "source": source, "order_id": p.OrderID.String(),
				"merchant_trade_no": p.MerchantTradeNo, "trade_no": res.TradeNo, "amount": res.Amount}
			for k, v := range meta {
				m[k] = v
			}
			return audit.Record(ctx, q, now, audit.Entry{Action: audit.ActionPaymentStatus, TargetType: audit.TargetPayment, TargetID: p.ID.String(), Metadata: m})
		}

		if res.Simulated && !s.cfg.AllowSimulated {
			slog.WarnContext(ctx, "ignoring simulated ECPay payment", "merchant_trade_no", p.MerchantTradeNo)
			return nil
		}
		if res.Amount != p.Amount {
			slog.ErrorContext(ctx, "ecpay amount mismatch", "merchant_trade_no", p.MerchantTradeNo, "expected", p.Amount, "got", res.Amount)
			return record(StatusAmountMismatch, map[string]any{"expected_amount": p.Amount})
		}
		o, err := q.GetOrderForUpdate(ctx, p.OrderID)
		if err != nil {
			return fmt.Errorf("lock order: %w", err)
		}
		if order.Status(o.Status) != order.AwaitingPayment {
			// Paid twice (another attempt already succeeded) or the order is no
			// longer payable: keep the record for a manual refund.
			slog.ErrorContext(ctx, "payment for order not awaiting payment", "order", o.ID, "status", o.Status, "merchant_trade_no", p.MerchantTradeNo)
			return record(StatusDuplicate, map[string]any{"order_status": o.Status, "needs_refund": true})
		}
		if err := record(StatusPaid, nil); err != nil {
			return err
		}
		if err := q.SetOrderTimestamps(ctx, db.SetOrderTimestampsParams{ID: o.ID, PaidAt: &now}); err != nil {
			return err
		}
		meta := map[string]any{"payment_id": p.ID.String(), "source": source}
		if err := order.Transition(ctx, q, now, o.ID, order.AwaitingPayment, order.Paid, uuid.Nil, meta); err != nil {
			return err
		}
		if err := order.Transition(ctx, q, now, o.ID, order.Paid, order.AwaitingShipment, uuid.Nil, meta); err != nil {
			return err
		}
		total := notify.FormatPrice(o.TotalAmount)
		for _, m := range []notify.Message{
			{UserID: o.BuyerID, Type: notify.TypePaymentDone, OrderID: o.ID, Title: "付款成功",
				Content: fmt.Sprintf("已收到 %s 付款，請填寫收件資訊讓賣家出貨。", total)},
			{UserID: o.SellerID, Type: notify.TypePaymentDone, OrderID: o.ID, Title: "買家已付款",
				Content: fmt.Sprintf("買家已付款 %s，待買家填寫收件資訊後即可出貨。", total)},
		} {
			if err := batch.Add(ctx, q, now, m); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.dispatch.Dispatch(batch)
	return nil
}
