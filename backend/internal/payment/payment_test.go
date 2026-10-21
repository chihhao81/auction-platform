package payment

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"auction-platform/backend/internal/apperr"
	"auction-platform/backend/internal/ecpay"
	"auction-platform/backend/internal/notify"
	"auction-platform/backend/internal/order"
	"auction-platform/backend/internal/store"
	"auction-platform/backend/internal/testdb"
)

func TestNewTradeNo(t *testing.T) {
	re := regexp.MustCompile(`^CB\d{12}[A-Z2-9]{6}$`)
	seen := map[string]bool{}
	for range 1000 {
		n := newTradeNo(time.Now())
		if !re.MatchString(n) || len(n) != 20 {
			t.Fatalf("bad trade no %q", n)
		}
		seen[n] = true
	}
	if len(seen) < 990 {
		t.Fatal("trade numbers should be practically unique")
	}
}

func TestItemName(t *testing.T) {
	if got := itemName([]string{"豹紋#守宮", "鼠婦"}); got != "豹紋 守宮#鼠婦" {
		t.Errorf("got %q", got)
	}
	if got := itemName(nil); got == "" {
		t.Error("empty item name")
	}
	if got := itemName([]string{strings.Repeat("守", 300)}); len([]rune(got)) != 200 {
		t.Errorf("not truncated: %d", len([]rune(got)))
	}
}

// --- integration tests (TEST_DATABASE_URL) ---

func testClient() *ecpay.Client {
	return ecpay.New("3002607", "pwFHCqoQZGmho4w6", "EkRm7iFT261dpevs", "http://unused")
}

type fixture struct {
	pool   *pgxpool.Pool
	svc    *Service
	buyer  uuid.UUID
	seller uuid.UUID
	order  uuid.UUID
}

func setup(t *testing.T, status string, allowSimulated bool) fixture {
	pool := testdb.New(t)
	ctx := context.Background()
	f := fixture{pool: pool, buyer: testdb.CreateUser(t, pool, "buyer"), seller: testdb.CreateUser(t, pool, "seller")}
	if err := pool.QueryRow(ctx, `INSERT INTO orders (buyer_id, seller_id, items_total, shipping_fee, total_amount, status, contact_deadline)
		VALUES ($1, $2, 1500, 175, 1675, $3, NOW() + INTERVAL '1 day') RETURNING id`, f.buyer, f.seller, status).Scan(&f.order); err != nil {
		t.Fatal(err)
	}
	f.svc = NewService(store.New(pool), testClient(), notify.Noop{}, Config{PublicBaseURL: "https://site.test", AllowSimulated: allowSimulated},
		func() time.Time { return time.Now().UTC() })
	return f
}

func (f fixture) callback(tradeNo string, amount int64, extra map[string]string) map[string]string {
	p := map[string]string{
		"MerchantID": "3002607", "MerchantTradeNo": tradeNo, "RtnCode": "1", "RtnMsg": "交易成功",
		"TradeNo": "2410" + tradeNo[len(tradeNo)-6:], "TradeAmt": strconv.FormatInt(amount, 10),
		"PaymentType": "Credit_CreditCard", "PaymentDate": "2026/10/22 10:00:00", "SimulatePaid": "0",
	}
	for k, v := range extra {
		p[k] = v
	}
	p["CheckMacValue"] = testClient().CheckMac(p)
	return p
}

func (f fixture) orderStatus(t *testing.T) string {
	var s string
	if err := f.pool.QueryRow(context.Background(), "SELECT status FROM orders WHERE id=$1", f.order).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func (f fixture) paymentStatus(t *testing.T, tradeNo string) string {
	var s string
	if err := f.pool.QueryRow(context.Background(), "SELECT status FROM payments WHERE merchant_trade_no=$1", tradeNo).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func (f fixture) checkout(t *testing.T) string {
	co, err := f.svc.CreateCheckout(context.Background(), f.buyer, f.order)
	if err != nil {
		t.Fatal(err)
	}
	return co.Fields["MerchantTradeNo"]
}

func TestIntegrationCheckoutRules(t *testing.T) {
	f := setup(t, "AWAITING_PAYMENT", true)
	ctx := context.Background()
	co, err := f.svc.CreateCheckout(ctx, f.buyer, f.order)
	if err != nil {
		t.Fatal(err)
	}
	if co.Fields["TotalAmount"] != "1675" || co.Fields["ReturnURL"] != "https://site.test/api/payments/ecpay/notify" || !testClient().Verify(co.Fields) {
		t.Fatalf("checkout form %+v", co.Fields)
	}
	if _, err := f.svc.CreateCheckout(ctx, f.seller, f.order); !apperr.IsCode(err, "NOT_BUYER") {
		t.Fatalf("seller checkout: %v", err)
	}
	stranger := testdb.CreateUser(t, f.pool, "stranger")
	if _, err := f.svc.CreateCheckout(ctx, stranger, f.order); err != order.ErrNotFound {
		t.Fatalf("stranger checkout: %v", err)
	}
	for range 4 {
		if _, err := f.svc.CreateCheckout(ctx, f.buyer, f.order); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.svc.CreateCheckout(ctx, f.buyer, f.order); !apperr.IsCode(err, "PAYMENT_RATE_LIMITED") {
		t.Fatalf("6th attempt within a minute: %v", err)
	}

	unpaid := setup(t, "AWAITING_CONTACT", true)
	if _, err := unpaid.svc.CreateCheckout(ctx, unpaid.buyer, unpaid.order); !apperr.IsCode(err, "ORDER_NOT_PAYABLE") {
		t.Fatalf("checkout before contact: %v", err)
	}
}

func TestIntegrationCallbackPaysOrderOnceEvenWhenRepeated(t *testing.T) {
	f := setup(t, "AWAITING_PAYMENT", true)
	ctx := context.Background()
	no := f.checkout(t)
	cb := f.callback(no, 1675, nil)

	// ECPay retries and may deliver the same notification concurrently.
	var wg sync.WaitGroup
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if body, ok := f.svc.HandleCallback(ctx, cb); !ok || body != "1|OK" {
				t.Errorf("callback answered %q ok=%v", body, ok)
			}
		}()
	}
	wg.Wait()

	if s := f.orderStatus(t); s != "AWAITING_SHIPMENT" {
		t.Fatalf("order status %s", s)
	}
	if s := f.paymentStatus(t, no); s != "PAID" {
		t.Fatalf("payment status %s", s)
	}
	var transitions, paymentAudits, notifs int
	_ = f.pool.QueryRow(ctx, "SELECT COUNT(*) FROM audit_logs WHERE action='ORDER_STATUS_CHANGE' AND target_id=$1", f.order.String()).Scan(&transitions)
	_ = f.pool.QueryRow(ctx, "SELECT COUNT(*) FROM audit_logs WHERE action='PAYMENT_STATUS_CHANGE'").Scan(&paymentAudits)
	_ = f.pool.QueryRow(ctx, "SELECT COUNT(*) FROM notifications WHERE type='PAYMENT_COMPLETED'").Scan(&notifs)
	if transitions != 2 || paymentAudits != 1 || notifs != 2 {
		t.Fatalf("transitions=%d payment audits=%d notifications=%d, want 2/1/2", transitions, paymentAudits, notifs)
	}
}

func TestIntegrationCallbackRejectsForgedAndMismatched(t *testing.T) {
	f := setup(t, "AWAITING_PAYMENT", true)
	ctx := context.Background()
	no := f.checkout(t)

	forged := f.callback(no, 1675, nil)
	forged["CheckMacValue"] = strings.Repeat("A", 64)
	if body, ok := f.svc.HandleCallback(ctx, forged); ok || !strings.HasPrefix(body, "0|") {
		t.Fatalf("forged callback answered %q ok=%v", body, ok)
	}
	if s := f.orderStatus(t); s != "AWAITING_PAYMENT" {
		t.Fatalf("forged callback changed order to %s", s)
	}

	// Correctly signed but for a different amount: recorded, never applied.
	if body, _ := f.svc.HandleCallback(ctx, f.callback(no, 1, nil)); body != "1|OK" {
		t.Fatalf("mismatch callback answered %q", body)
	}
	if s := f.paymentStatus(t, no); s != "AMOUNT_MISMATCH" {
		t.Fatalf("payment status %s", s)
	}
	if s := f.orderStatus(t); s != "AWAITING_PAYMENT" {
		t.Fatalf("mismatched amount paid the order: %s", s)
	}

	// Failed payment results leave the attempt pending.
	no2 := f.checkout(t)
	f.svc.HandleCallback(ctx, f.callback(no2, 1675, map[string]string{"RtnCode": "10100248", "RtnMsg": "拒絕交易"}))
	if s := f.paymentStatus(t, no2); s != "PENDING" {
		t.Fatalf("failed result changed payment to %s", s)
	}
	// Unknown trade numbers are acknowledged without effect.
	if body, ok := f.svc.HandleCallback(ctx, f.callback("CB000000000000XXXXXX", 1675, nil)); body != "1|OK" || !ok {
		t.Fatalf("unknown trade answered %q", body)
	}
}

func TestIntegrationSecondPaymentIsFlaggedDuplicate(t *testing.T) {
	f := setup(t, "AWAITING_PAYMENT", true)
	ctx := context.Background()
	first, second := f.checkout(t), f.checkout(t)
	f.svc.HandleCallback(ctx, f.callback(first, 1675, nil))
	f.svc.HandleCallback(ctx, f.callback(second, 1675, nil))
	if s := f.paymentStatus(t, first); s != "PAID" {
		t.Fatalf("first %s", s)
	}
	if s := f.paymentStatus(t, second); s != "DUPLICATE" {
		t.Fatalf("second payment should be flagged for refund, got %s", s)
	}
	if s := f.orderStatus(t); s != "AWAITING_SHIPMENT" {
		t.Fatalf("order %s", s)
	}
}

func TestIntegrationSimulatedPaymentRespectsConfig(t *testing.T) {
	f := setup(t, "AWAITING_PAYMENT", false)
	no := f.checkout(t)
	f.svc.HandleCallback(context.Background(), f.callback(no, 1675, map[string]string{"SimulatePaid": "1"}))
	if s := f.orderStatus(t); s != "AWAITING_PAYMENT" {
		t.Fatalf("simulated payment must be ignored when not allowed, order %s", s)
	}
}

func TestIntegrationSyncQueriesECPay(t *testing.T) {
	f := setup(t, "AWAITING_PAYMENT", true)
	client := testClient()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if !client.Verify(ecpay.FormToMap(r.PostForm)) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		resp := map[string]string{"MerchantID": "3002607", "MerchantTradeNo": r.PostForm.Get("MerchantTradeNo"),
			"TradeNo": "T9", "TradeAmt": "1675", "TradeStatus": "1", "PaymentType": "Credit_CreditCard"}
		resp["CheckMacValue"] = client.CheckMac(resp)
		v := url.Values{}
		for k, val := range resp {
			v.Set(k, val)
		}
		_, _ = w.Write([]byte(v.Encode()))
	}))
	defer srv.Close()
	client.BaseURL = srv.URL
	f.svc.ecpay = client

	no := f.checkout(t)
	stranger := testdb.CreateUser(t, f.pool, "stranger")
	if err := f.svc.Sync(context.Background(), stranger, f.order); err != order.ErrNotFound {
		t.Fatalf("stranger sync: %v", err)
	}
	if err := f.svc.Sync(context.Background(), f.buyer, f.order); err != nil {
		t.Fatal(err)
	}
	if s := f.paymentStatus(t, no); s != "PAID" {
		t.Fatalf("payment %s", s)
	}
	if s := f.orderStatus(t); s != "AWAITING_SHIPMENT" {
		t.Fatalf("order %s", s)
	}
}
