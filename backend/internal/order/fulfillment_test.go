package order

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"auction-platform/backend/internal/apperr"
	"auction-platform/backend/internal/notify"
	"auction-platform/backend/internal/storage"
	"auction-platform/backend/internal/store"
	"auction-platform/backend/internal/testdb"
)

func TestShippingInputValidation(t *testing.T) {
	valid := ShippingInput{RecipientName: " 王小明 ", RecipientPhone: "0912-345-678", RecipientAddress: "7-11 守宮門市 (123456)"}
	got, err := valid.normalize()
	if err != nil || got.RecipientName != "王小明" {
		t.Fatalf("valid input: %+v %v", got, err)
	}
	cases := map[string]ShippingInput{
		"INVALID_RECIPIENT_NAME":    {RecipientName: "", RecipientPhone: "0912345678", RecipientAddress: "a"},
		"INVALID_RECIPIENT_PHONE":   {RecipientName: "a", RecipientPhone: "call me", RecipientAddress: "a"},
		"INVALID_RECIPIENT_ADDRESS": {RecipientName: "a", RecipientPhone: "0912345678", RecipientAddress: " "},
	}
	for code, in := range cases {
		if _, err := in.normalize(); !apperr.IsCode(err, code) {
			t.Errorf("%s: got %v", code, err)
		}
	}
}

func TestHumanDays(t *testing.T) {
	if humanDays(72*time.Hour) != "3 天" || humanDays(36*time.Hour) != "36 小時" {
		t.Fatal(humanDays(72*time.Hour), humanDays(36*time.Hour))
	}
}

// --- integration tests (TEST_DATABASE_URL) ---

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time      { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) add(d time.Duration) { c.mu.Lock(); defer c.mu.Unlock(); c.t = c.t.Add(d) }

type fixture struct {
	pool          *pgxpool.Pool
	svc           *Service
	clk           *clock
	buyer, seller uuid.UUID
	order         uuid.UUID
}

func setup(t *testing.T, status string) fixture {
	pool := testdb.New(t)
	clk := &clock{t: time.Now().UTC().Truncate(time.Microsecond)}
	f := fixture{pool: pool, clk: clk, buyer: testdb.CreateUser(t, pool, "buyer"), seller: testdb.CreateUser(t, pool, "seller")}
	if err := pool.QueryRow(context.Background(), `INSERT INTO orders (buyer_id, seller_id, items_total, shipping_fee, total_amount, status, contact_deadline)
		VALUES ($1, $2, 1000, 100, 1100, $3, $4) RETURNING id`, f.buyer, f.seller, status, clk.now().Add(24*time.Hour)).Scan(&f.order); err != nil {
		t.Fatal(err)
	}
	f.svc = NewService(store.New(pool), &storage.Local{BaseURL: "/uploads"}, notify.Noop{}, DefaultOptions, clk.now)
	return f
}

func (f fixture) status(t *testing.T) Status {
	var s string
	if err := f.pool.QueryRow(context.Background(), "SELECT status FROM orders WHERE id=$1", f.order).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return Status(s)
}

func (f fixture) count(t *testing.T, sql string, args ...any) int {
	var n int
	if err := f.pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestIntegrationFirstBuyerMessageMovesToAwaitingPayment(t *testing.T) {
	f := setup(t, "AWAITING_CONTACT")
	ctx := context.Background()

	if _, err := f.svc.SendMessage(ctx, f.seller, f.order, "您好，我是賣家"); err != nil {
		t.Fatal(err)
	}
	if f.status(t) != AwaitingContact {
		t.Fatal("a seller message must not count as the buyer contacting")
	}
	if _, err := f.svc.SendMessage(ctx, f.buyer, f.order, "<script>alert(1)</script> 您好"); err != nil {
		t.Fatal(err)
	}
	if f.status(t) != AwaitingPayment {
		t.Fatalf("status %s, want AWAITING_PAYMENT", f.status(t))
	}
	if f.count(t, "SELECT COUNT(*) FROM orders WHERE id=$1 AND contacted_at IS NOT NULL", f.order) != 1 {
		t.Fatal("contacted_at not set")
	}
	if _, err := f.svc.SendMessage(ctx, f.buyer, f.order, "第二則"); err != nil {
		t.Fatal(err)
	}
	// The receiver gets one notification per batch of unread messages.
	if n := f.count(t, "SELECT COUNT(*) FROM notifications WHERE user_id=$1 AND type='MESSAGE_RECEIVED'", f.seller); n != 1 {
		t.Fatalf("seller message notifications %d, want 1", n)
	}

	msgs, err := f.svc.Messages(ctx, f.seller, f.order)
	if err != nil || len(msgs) != 3 || msgs[0].Content != "您好，我是賣家" || !msgs[0].Mine || msgs[1].Mine {
		t.Fatalf("messages %+v err %v", msgs, err)
	}
	// Stored verbatim as text; the frontend renders it escaped.
	if msgs[1].Content != "<script>alert(1)</script> 您好" {
		t.Fatalf("content altered: %q", msgs[1].Content)
	}
	if f.count(t, "SELECT COUNT(*) FROM messages WHERE receiver_id=$1 AND read_at IS NULL", f.seller) != 0 {
		t.Fatal("reading should mark the seller's messages as read")
	}

	stranger := testdb.CreateUser(t, f.pool, "stranger")
	if _, err := f.svc.SendMessage(ctx, stranger, f.order, "hi"); err != ErrNotFound {
		t.Fatalf("stranger message: %v", err)
	}
	if _, err := f.svc.Messages(ctx, stranger, f.order); err != ErrNotFound {
		t.Fatalf("stranger read: %v", err)
	}
	for _, bad := range []string{"", "   ", string(make([]rune, 1001)), "a\x00b"} {
		if _, err := f.svc.SendMessage(ctx, f.buyer, f.order, bad); !apperr.IsCode(err, "INVALID_MESSAGE") {
			t.Errorf("message %q: %v", bad, err)
		}
	}
}

func TestIntegrationOverdueFlagsAndReportsOnce(t *testing.T) {
	f := setup(t, "AWAITING_CONTACT")
	ctx := context.Background()
	if err := f.svc.RunJobs(ctx); err != nil {
		t.Fatal(err)
	}
	if f.count(t, "SELECT COUNT(*) FROM reports") != 0 {
		t.Fatal("not overdue yet")
	}
	f.clk.add(24*time.Hour + time.Minute)
	for range 3 {
		if err := f.svc.RunJobs(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if f.count(t, "SELECT COUNT(*) FROM orders WHERE id=$1 AND overdue_at IS NOT NULL", f.order) != 1 {
		t.Fatal("order not flagged overdue")
	}
	if f.status(t) != AwaitingContact {
		t.Fatal("overdue keeps the status (decision: flag only)")
	}
	if n := f.count(t, "SELECT COUNT(*) FROM reports WHERE target_user_id=$1 AND reason=$2 AND status='PENDING' AND reporter_id IS NULL", f.buyer, ReportReasonNoContact); n != 1 {
		t.Fatalf("reports %d, want exactly 1", n)
	}
	if n := f.count(t, "SELECT COUNT(*) FROM audit_logs WHERE action='ORDER_OVERDUE'"); n != 1 {
		t.Fatalf("overdue audits %d", n)
	}
	// Contacting late still moves the order on; the overdue record remains.
	if _, err := f.svc.SendMessage(ctx, f.buyer, f.order, "抱歉晚了"); err != nil {
		t.Fatal(err)
	}
	if f.status(t) != AwaitingPayment {
		t.Fatal("late contact should still advance")
	}
}

func TestIntegrationShippingFlowAndAutoTransitions(t *testing.T) {
	f := setup(t, "AWAITING_SHIPMENT")
	ctx := context.Background()
	addr := ShippingInput{RecipientName: "王小明", RecipientPhone: "0912345678", RecipientAddress: "台北市信義區守宮路 1 號"}

	if err := f.svc.Ship(ctx, f.seller, f.order, ShipInput{Carrier: "黑貓", TrackingNumber: "123"}); !apperr.IsCode(err, "SHIPPING_INFO_MISSING") {
		t.Fatalf("ship without address: %v", err)
	}
	if err := f.svc.SetShippingInfo(ctx, f.seller, f.order, addr); err != ErrNotBuyer {
		t.Fatalf("seller setting address: %v", err)
	}
	if err := f.svc.SetShippingInfo(ctx, f.buyer, f.order, addr); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Ship(ctx, f.buyer, f.order, ShipInput{Carrier: "黑貓", TrackingNumber: "123"}); err != ErrNotSeller {
		t.Fatalf("buyer shipping: %v", err)
	}

	sellerView, err := f.svc.Get(ctx, f.order, f.seller)
	if err != nil || sellerView.Shipment == nil || sellerView.Shipment.RecipientAddress != addr.RecipientAddress {
		t.Fatalf("seller must see the address: %+v %v", sellerView.Shipment, err)
	}

	if err := f.svc.Ship(ctx, f.seller, f.order, ShipInput{Carrier: "黑貓宅急便", TrackingNumber: "9001-2345"}); err != nil {
		t.Fatal(err)
	}
	if f.status(t) != Shipped {
		t.Fatalf("status %s", f.status(t))
	}
	if err := f.svc.SetShippingInfo(ctx, f.buyer, f.order, addr); err == nil {
		t.Fatal("address must be locked after shipping")
	}
	if err := f.svc.Complete(ctx, f.buyer, f.order); !apperr.IsCode(err, "INVALID_ORDER_TRANSITION") {
		t.Fatalf("complete before receive: %v", err)
	}

	// Auto receive after 3 days, auto complete 7 days later.
	f.clk.add(72*time.Hour - time.Minute)
	_ = f.svc.RunJobs(ctx)
	if f.status(t) != Shipped {
		t.Fatal("received too early")
	}
	f.clk.add(2 * time.Minute)
	_ = f.svc.RunJobs(ctx)
	if f.status(t) != Received {
		t.Fatalf("status %s, want RECEIVED", f.status(t))
	}
	view, _ := f.svc.Get(ctx, f.order, f.buyer)
	if view.AutoAdvanceAt == nil || !view.AutoAdvanceAt.Equal(view.ReceivedAt.Add(7*24*time.Hour)) {
		t.Fatalf("auto complete time %v", view.AutoAdvanceAt)
	}
	f.clk.add(7*24*time.Hour + time.Minute)
	_ = f.svc.RunJobs(ctx)
	if f.status(t) != Completed {
		t.Fatalf("status %s, want COMPLETED", f.status(t))
	}

	// The address is erased after the retention period.
	f.clk.add(30*24*time.Hour + time.Minute)
	_ = f.svc.RunJobs(ctx)
	view, _ = f.svc.Get(ctx, f.order, f.seller)
	if view.Shipment == nil || !view.Shipment.Purged || view.Shipment.RecipientAddress != "" || view.Shipment.TrackingNumber == "" {
		t.Fatalf("shipment after retention %+v", view.Shipment)
	}
}

func TestIntegrationBuyerConfirmsAndCompletes(t *testing.T) {
	f := setup(t, "SHIPPED")
	ctx := context.Background()
	if err := f.svc.ConfirmReceived(ctx, f.seller, f.order); err != ErrNotBuyer {
		t.Fatalf("seller confirming: %v", err)
	}
	if err := f.svc.ConfirmReceived(ctx, f.buyer, f.order); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.ConfirmReceived(ctx, f.buyer, f.order); !apperr.IsCode(err, "INVALID_ORDER_TRANSITION") {
		t.Fatalf("double confirm: %v", err)
	}
	if err := f.svc.Complete(ctx, f.buyer, f.order); err != nil {
		t.Fatal(err)
	}
	if f.status(t) != Completed {
		t.Fatalf("status %s", f.status(t))
	}
	if n := f.count(t, "SELECT COUNT(*) FROM audit_logs WHERE action='ORDER_STATUS_CHANGE' AND target_id=$1", f.order.String()); n != 2 {
		t.Fatalf("status audits %d", n)
	}
}
