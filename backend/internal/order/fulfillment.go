package order

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/google/uuid"

	"auction-platform/backend/internal/apperr"
	"auction-platform/backend/internal/audit"
	"auction-platform/backend/internal/database/db"
	"auction-platform/backend/internal/notify"
	"auction-platform/backend/internal/store"
	"auction-platform/backend/internal/textutil"
)

const (
	MaxMessageLength = 1000
	// ReportReasonNoContact is the system report filed against a buyer who
	// did not contact the seller before the contact deadline.
	ReportReasonNoContact = "NO_CONTACT_24H"
)

var (
	ErrNotBuyer        = apperr.Forbidden("NOT_BUYER", "只有買家可以進行此操作")
	ErrNotSeller       = apperr.Forbidden("NOT_SELLER", "只有賣家可以進行此操作")
	ErrOrderClosed     = apperr.Conflict("ORDER_CLOSED", "此訂單已取消，無法再傳送訊息")
	ErrNoAddress       = apperr.Conflict("SHIPPING_INFO_MISSING", "買家尚未填寫收件資訊")
	ErrAlreadyShipped  = apperr.Conflict("ALREADY_SHIPPED", "賣家已出貨，無法修改收件資訊")
	ErrInvalidMessage  = apperr.Invalid("INVALID_MESSAGE", fmt.Sprintf("訊息需為 1 ~ %d 字的純文字", MaxMessageLength))
	errNoTransitionYet = errors.New("no transition")
)

type party int

const (
	partyBuyer party = iota + 1
	partySeller
)

// lockAsParty locks the order row and checks user is its buyer or seller.
// Orders of other users are reported as not found (IDOR protection).
func lockAsParty(ctx context.Context, q *db.Queries, orderID, user uuid.UUID) (db.Order, party, error) {
	o, err := q.GetOrderForUpdate(ctx, orderID)
	if store.IsNoRows(err) {
		return db.Order{}, 0, ErrNotFound
	}
	if err != nil {
		return db.Order{}, 0, fmt.Errorf("lock order: %w", err)
	}
	switch user {
	case o.BuyerID:
		return o, partyBuyer, nil
	case o.SellerID:
		return o, partySeller, nil
	}
	return db.Order{}, 0, ErrNotFound
}

func ptr[T any](v T) *T { return &v }

// MessageDTO is one order message.
type MessageDTO struct {
	ID        string     `json:"id"`
	Mine      bool       `json:"mine"`
	Content   string     `json:"content"`
	CreatedAt time.Time  `json:"created_at"`
	ReadAt    *time.Time `json:"read_at"`
}

func messageDTO(m db.Message, viewer uuid.UUID) MessageDTO {
	return MessageDTO{ID: m.ID.String(), Mine: m.SenderID == viewer, Content: m.Content, CreatedAt: m.CreatedAt.UTC(), ReadAt: m.ReadAt}
}

// SendMessage posts a plain-text message to the order's other party. The
// buyer's first message counts as contacting the seller and moves the order
// from AWAITING_CONTACT to AWAITING_PAYMENT.
func (s *Service) SendMessage(ctx context.Context, user, orderID uuid.UUID, content string) (MessageDTO, error) {
	content, ok := textutil.Clean(content, MaxMessageLength, true)
	if !ok || content == "" {
		return MessageDTO{}, ErrInvalidMessage
	}
	var batch notify.Batch
	var out MessageDTO
	err := s.store.InTx(ctx, func(q *db.Queries) error {
		o, who, err := lockAsParty(ctx, q, orderID, user)
		if err != nil {
			return err
		}
		if Status(o.Status) == Cancelled {
			return ErrOrderClosed
		}
		now := s.now().UTC()
		receiver := o.SellerID
		if who == partySeller {
			receiver = o.BuyerID
		}
		unread, err := q.CountUnreadMessages(ctx, db.CountUnreadMessagesParams{OrderID: o.ID, ReceiverID: receiver})
		if err != nil {
			return fmt.Errorf("count unread: %w", err)
		}
		m, err := q.InsertMessage(ctx, db.InsertMessageParams{OrderID: o.ID, SenderID: user, ReceiverID: receiver, Content: content, CreatedAt: now})
		if err != nil {
			return fmt.Errorf("insert message: %w", err)
		}
		out = messageDTO(m, user)

		// One notification per batch of unread messages, not one per message.
		if unread == 0 {
			if err := batch.Add(ctx, q, now, notify.Message{
				UserID: receiver, Type: notify.TypeMessageReceived, OrderID: o.ID,
				Title: "收到新的訂單訊息", Content: preview(content),
			}); err != nil {
				return err
			}
		}

		if who == partyBuyer && Status(o.Status) == AwaitingContact && o.ContactedAt == nil {
			if err := q.SetOrderTimestamps(ctx, db.SetOrderTimestampsParams{ID: o.ID, ContactedAt: &now}); err != nil {
				return fmt.Errorf("set contacted_at: %w", err)
			}
			if err := Transition(ctx, q, now, o.ID, AwaitingContact, AwaitingPayment, user, map[string]any{"trigger": "first_buyer_message"}); err != nil {
				return err
			}
			return batch.Add(ctx, q, now, notify.Message{
				UserID: o.SellerID, Type: notify.TypeOrderStatus, OrderID: o.ID,
				Title: "買家已聯絡您", Content: "訂單進入「等待付款」，買家付款後會通知您。",
			})
		}
		return nil
	})
	if err != nil {
		return MessageDTO{}, err
	}
	s.dispatch.Dispatch(batch)
	return out, nil
}

func preview(s string) string {
	r := []rune(s)
	if len(r) > 50 {
		return string(r[:50]) + "…"
	}
	return s
}

// Messages returns the order's messages (oldest first) and marks the ones
// sent to user as read.
func (s *Service) Messages(ctx context.Context, user, orderID uuid.UUID) ([]MessageDTO, error) {
	var out []MessageDTO
	err := s.store.InTx(ctx, func(q *db.Queries) error {
		o, _, err := lockAsParty(ctx, q, orderID, user)
		if err != nil {
			return err
		}
		rows, err := q.ListMessages(ctx, o.ID)
		if err != nil {
			return fmt.Errorf("list messages: %w", err)
		}
		out = make([]MessageDTO, len(rows))
		for i, m := range rows {
			out[len(rows)-1-i] = messageDTO(m, user)
		}
		return q.MarkMessagesRead(ctx, db.MarkMessagesReadParams{Now: s.now().UTC(), OrderID: o.ID, ReceiverID: user})
	})
	return out, err
}

// ShippingInput is the recipient info a buyer enters after paying.
type ShippingInput struct {
	RecipientName    string `json:"recipient_name"`
	RecipientPhone   string `json:"recipient_phone"`
	RecipientAddress string `json:"recipient_address"`
}

var phoneRe = regexp.MustCompile(`^[0-9+\-() ]{6,20}$`)

func (in ShippingInput) normalize() (ShippingInput, error) {
	var ok bool
	if in.RecipientName, ok = textutil.Clean(in.RecipientName, 50, false); !ok || in.RecipientName == "" {
		return in, apperr.Invalid("INVALID_RECIPIENT_NAME", "收件人姓名必填，且不可超過 50 字")
	}
	if in.RecipientPhone, ok = textutil.Clean(in.RecipientPhone, 20, false); !ok || !phoneRe.MatchString(in.RecipientPhone) {
		return in, apperr.Invalid("INVALID_RECIPIENT_PHONE", "請輸入有效的電話號碼")
	}
	if in.RecipientAddress, ok = textutil.Clean(in.RecipientAddress, 300, true); !ok || in.RecipientAddress == "" {
		return in, apperr.Invalid("INVALID_RECIPIENT_ADDRESS", "收件地址或超商門市必填，且不可超過 300 字")
	}
	return in, nil
}

// SetShippingInfo stores the buyer's recipient info. It can be changed until
// the seller ships. Only the order's seller (and the buyer) can read it.
func (s *Service) SetShippingInfo(ctx context.Context, buyer, orderID uuid.UUID, in ShippingInput) error {
	in, err := in.normalize()
	if err != nil {
		return err
	}
	var batch notify.Batch
	err = s.store.InTx(ctx, func(q *db.Queries) error {
		o, who, err := lockAsParty(ctx, q, orderID, buyer)
		if err != nil {
			return err
		}
		if who != partyBuyer {
			return ErrNotBuyer
		}
		if Status(o.Status) != AwaitingShipment {
			return apperr.Conflict("ORDER_NOT_PAID", "付款完成後才能填寫收件資訊")
		}
		now := s.now().UTC()
		n, err := q.UpsertShipmentAddress(ctx, db.UpsertShipmentAddressParams{
			OrderID: o.ID, RecipientName: &in.RecipientName, RecipientPhone: &in.RecipientPhone,
			RecipientAddress: &in.RecipientAddress, AddressSetAt: &now,
		})
		if err != nil {
			return fmt.Errorf("save shipping info: %w", err)
		}
		if n == 0 {
			return ErrAlreadyShipped
		}
		// The address itself is personal data and is not copied into the log.
		if err := audit.Record(ctx, q, now, audit.Entry{Actor: buyer, Action: audit.ActionShippingInfoSet, TargetType: audit.TargetOrder, TargetID: o.ID.String()}); err != nil {
			return err
		}
		return batch.Add(ctx, q, now, notify.Message{
			UserID: o.SellerID, Type: notify.TypeOrderStatus, OrderID: o.ID,
			Title: "買家已填寫收件資訊", Content: "請準備出貨並填寫物流資訊。",
		})
	})
	if err != nil {
		return err
	}
	s.dispatch.Dispatch(batch)
	return nil
}

// ShipInput is the seller's logistics info.
type ShipInput struct {
	Carrier        string `json:"carrier"`
	TrackingNumber string `json:"tracking_number"`
}

// Ship records the shipment and moves the order to SHIPPED.
func (s *Service) Ship(ctx context.Context, seller, orderID uuid.UUID, in ShipInput) error {
	carrier, ok := textutil.Clean(in.Carrier, 50, false)
	if !ok || carrier == "" {
		return apperr.Invalid("INVALID_CARRIER", "物流方式必填，且不可超過 50 字")
	}
	tracking, ok := textutil.Clean(in.TrackingNumber, 100, false)
	if !ok || tracking == "" {
		return apperr.Invalid("INVALID_TRACKING_NUMBER", "物流單號必填，且不可超過 100 字")
	}
	var batch notify.Batch
	err := s.store.InTx(ctx, func(q *db.Queries) error {
		o, who, err := lockAsParty(ctx, q, orderID, seller)
		if err != nil {
			return err
		}
		if who != partySeller {
			return ErrNotSeller
		}
		if Status(o.Status) != AwaitingShipment {
			return ErrInvalidTransition
		}
		now := s.now().UTC()
		n, err := q.SetShipmentShipped(ctx, db.SetShipmentShippedParams{OrderID: o.ID, Carrier: &carrier, TrackingNumber: &tracking, ShippedAt: &now})
		if err != nil {
			return fmt.Errorf("set shipped: %w", err)
		}
		if n == 0 {
			return ErrNoAddress
		}
		if err := q.SetOrderTimestamps(ctx, db.SetOrderTimestampsParams{ID: o.ID, ShippedAt: &now}); err != nil {
			return err
		}
		if err := Transition(ctx, q, now, o.ID, AwaitingShipment, Shipped, seller, map[string]any{"carrier": carrier, "tracking_number": tracking}); err != nil {
			return err
		}
		if err := audit.Record(ctx, q, now, audit.Entry{
			Actor: seller, Action: audit.ActionShipmentCreate, TargetType: audit.TargetOrder, TargetID: o.ID.String(),
			Metadata: map[string]any{"carrier": carrier, "tracking_number": tracking},
		}); err != nil {
			return err
		}
		return batch.Add(ctx, q, now, notify.Message{
			UserID: o.BuyerID, Type: notify.TypeOrderShipped, OrderID: o.ID,
			Title: "賣家已出貨", Content: fmt.Sprintf("物流：%s，單號：%s。收到商品後請確認收貨。", carrier, tracking),
		})
	})
	if err != nil {
		return err
	}
	s.dispatch.Dispatch(batch)
	return nil
}

// ConfirmReceived lets the buyer confirm delivery (SHIPPED -> RECEIVED).
func humanDays(d time.Duration) string {
	if d%(24*time.Hour) == 0 {
		return fmt.Sprintf("%d 天", int(d/(24*time.Hour)))
	}
	return fmt.Sprintf("%.0f 小時", d.Hours())
}

func (s *Service) ConfirmReceived(ctx context.Context, buyer, orderID uuid.UUID) error {
	return s.buyerStep(ctx, buyer, orderID, Shipped, Received, "買家已確認收貨", fmt.Sprintf("訂單將於 %s後自動完成，買家也可提前完成。", humanDays(s.opts.AutoCompleteAfter)))
}

// Complete lets the buyer finish the order (RECEIVED -> COMPLETED).
func (s *Service) Complete(ctx context.Context, buyer, orderID uuid.UUID) error {
	return s.buyerStep(ctx, buyer, orderID, Received, Completed, "訂單已完成", "交易已完成，可以互相評價了。")
}

func (s *Service) buyerStep(ctx context.Context, buyer, orderID uuid.UUID, from, to Status, title, content string) error {
	var batch notify.Batch
	err := s.store.InTx(ctx, func(q *db.Queries) error {
		o, who, err := lockAsParty(ctx, q, orderID, buyer)
		if err != nil {
			return err
		}
		if who != partyBuyer {
			return ErrNotBuyer
		}
		if Status(o.Status) != from {
			return ErrInvalidTransition
		}
		return s.advance(ctx, q, &batch, o, from, to, buyer, title, content)
	})
	if err != nil {
		return err
	}
	s.dispatch.Dispatch(batch)
	return nil
}

// advance performs a RECEIVED or COMPLETED transition, sets its timestamp
// and notifies both parties. actor is uuid.Nil for automatic transitions.
func (s *Service) advance(ctx context.Context, q *db.Queries, batch *notify.Batch, o db.Order, from, to Status, actor uuid.UUID, title, content string) error {
	now := s.now().UTC()
	ts := db.SetOrderTimestampsParams{ID: o.ID}
	switch to {
	case Received:
		ts.ReceivedAt = ptr(now)
	case Completed:
		ts.CompletedAt = ptr(now)
	}
	if err := q.SetOrderTimestamps(ctx, ts); err != nil {
		return err
	}
	meta := map[string]any{}
	if actor == uuid.Nil {
		meta["trigger"] = "auto"
	}
	if err := Transition(ctx, q, now, o.ID, from, to, actor, meta); err != nil {
		return err
	}
	for _, u := range []uuid.UUID{o.BuyerID, o.SellerID} {
		if err := batch.Add(ctx, q, now, notify.Message{UserID: u, Type: notify.TypeOrderStatus, OrderID: o.ID, Title: title, Content: content}); err != nil {
			return err
		}
	}
	return nil
}

const jobBatch = 100

// RunJobs performs the time-based order rules. Like settlement, every step
// is idempotent and safe to run concurrently.
func (s *Service) RunJobs(ctx context.Context) error {
	var errs []error
	if err := s.markOverdue(ctx); err != nil {
		errs = append(errs, fmt.Errorf("overdue: %w", err))
	}
	if err := s.autoAdvance(ctx, Shipped, Received, s.opts.AutoReceiveAfter); err != nil {
		errs = append(errs, fmt.Errorf("auto receive: %w", err))
	}
	if err := s.autoAdvance(ctx, Received, Completed, s.opts.AutoCompleteAfter); err != nil {
		errs = append(errs, fmt.Errorf("auto complete: %w", err))
	}
	// Housekeeping that rides on the same job: drop long-dead sessions.
	if _, err := s.store.DeleteStaleSessions(ctx, s.now().UTC().Add(-7*24*time.Hour)); err != nil {
		errs = append(errs, fmt.Errorf("delete stale sessions: %w", err))
	}
	if _, err := s.store.PurgeShipmentRecipients(ctx, db.PurgeShipmentRecipientsParams{
		Now: s.now().UTC(), Cutoff: s.now().UTC().Add(-s.opts.ShippingInfoRetention),
	}); err != nil {
		errs = append(errs, fmt.Errorf("purge shipping info: %w", err))
	}
	return errors.Join(errs...)
}

// markOverdue flags orders whose buyer did not contact the seller in time,
// audits it and files a system report against the buyer for admins.
func (s *Service) markOverdue(ctx context.Context) error {
	var batch notify.Batch
	err := s.store.InTx(ctx, func(q *db.Queries) error {
		now := s.now().UTC()
		claimed, err := q.ClaimOverdueOrders(ctx, now)
		if err != nil {
			return fmt.Errorf("claim overdue: %w", err)
		}
		for _, o := range claimed {
			if err := q.InsertSystemReport(ctx, db.InsertSystemReportParams{
				TargetUserID: uuid.NullUUID{UUID: o.BuyerID, Valid: true}, OrderID: uuid.NullUUID{UUID: o.ID, Valid: true},
				Reason: ReportReasonNoContact, Description: "得標後 24 小時內未透過平台訊息聯絡賣家（系統自動檢舉）", CreatedAt: now,
			}); err != nil {
				return fmt.Errorf("file report: %w", err)
			}
			if err := audit.Record(ctx, q, now, audit.Entry{
				Action: audit.ActionOrderOverdue, TargetType: audit.TargetOrder, TargetID: o.ID.String(),
				Metadata: map[string]any{"buyer": o.BuyerID.String(), "report_reason": ReportReasonNoContact},
			}); err != nil {
				return err
			}
			for _, m := range []notify.Message{
				{UserID: o.BuyerID, Type: notify.TypeOrderStatus, OrderID: o.ID, Title: "訂單已逾期",
					Content: "您未在 24 小時內聯絡賣家，訂單已標記逾期並送交管理員處理。請盡快透過訂單訊息聯絡賣家。"},
				{UserID: o.SellerID, Type: notify.TypeOrderStatus, OrderID: o.ID, Title: "買家逾期未聯絡",
					Content: "買家未在 24 小時內聯絡您，系統已自動送交管理員處理。"},
			} {
				if err := batch.Add(ctx, q, now, m); err != nil {
					return err
				}
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

func (s *Service) autoAdvance(ctx context.Context, from, to Status, after time.Duration) error {
	cutoff := s.now().UTC().Add(-after)
	var ids []uuid.UUID
	var err error
	switch from {
	case Shipped:
		ids, err = s.store.ListOrdersShippedBefore(ctx, db.ListOrdersShippedBeforeParams{Cutoff: cutoff, Lim: jobBatch})
	case Received:
		ids, err = s.store.ListOrdersReceivedBefore(ctx, db.ListOrdersReceivedBeforeParams{Cutoff: cutoff, Lim: jobBatch})
	}
	if err != nil {
		return err
	}
	title, content := "已自動確認收貨", fmt.Sprintf("出貨已超過 %s，系統自動確認收貨。", humanDays(after))
	if to == Completed {
		title, content = "訂單已自動完成", fmt.Sprintf("收貨已超過 %s，系統自動完成訂單，可以互相評價了。", humanDays(after))
	}
	var errs []error
	for _, id := range ids {
		var batch notify.Batch
		err := s.store.InTx(ctx, func(q *db.Queries) error {
			o, err := q.GetOrderForUpdate(ctx, id)
			if err != nil {
				return err
			}
			if Status(o.Status) != from {
				return errNoTransitionYet // changed meanwhile (e.g. buyer confirmed)
			}
			return s.advance(ctx, q, &batch, o, from, to, uuid.Nil, title, content)
		})
		if errors.Is(err, errNoTransitionYet) {
			continue
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("order %s: %w", id, err))
			continue
		}
		s.dispatch.Dispatch(batch)
	}
	return errors.Join(errs...)
}
