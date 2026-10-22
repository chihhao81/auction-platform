package settlement

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"auction-platform/backend/internal/notify"
	"auction-platform/backend/internal/order"
	"auction-platform/backend/internal/storage"
	"auction-platform/backend/internal/store"
	"auction-platform/backend/internal/testdb"
)

// These tests run against real PostgreSQL (TEST_DATABASE_URL).

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = t
}

type auctionSpec struct {
	seller   uuid.UUID
	end      time.Time
	shipping int64
	contact  string
	// bids in ascending order; the last one is the highest.
	bids []bidSpec
}

type bidSpec struct {
	bidder uuid.UUID
	amount int64
}

func insertAuction(t *testing.T, pool *pgxpool.Pool, s auctionSpec) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	price, count := int64(0), len(s.bids)
	var highest *uuid.UUID
	if count > 0 {
		last := s.bids[count-1]
		price, highest = last.amount, &last.bidder
	}
	var id uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO auctions (seller_id, title, description, start_price, min_increment, max_increment,
		    current_price, highest_bidder_id, bid_count, start_time, end_time, original_end_time,
		    contact_info, shipping_method, shipping_fee)
		VALUES ($1, 'gecko', '', 0, 1, 100000, $2, $3, $4, $5, $6, $6, $7, 'post', $8) RETURNING id`,
		s.seller, price, highest, count, s.end.Add(-48*time.Hour), s.end, s.contact, s.shipping).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range s.bids {
		if _, err := pool.Exec(ctx, "INSERT INTO bids (auction_id, bidder_id, amount) VALUES ($1, $2, $3)", id, b.bidder, b.amount); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

func notificationTypes(t *testing.T, pool *pgxpool.Pool, user uuid.UUID) map[string]int {
	t.Helper()
	rows, err := pool.Query(context.Background(), "SELECT type FROM notifications WHERE user_id = $1", user)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var typ string
		if err := rows.Scan(&typ); err != nil {
			t.Fatal(err)
		}
		out[typ]++
	}
	return out
}

func count(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func setup(t *testing.T) (*pgxpool.Pool, *Service, *clock) {
	pool := testdb.New(t)
	c := &clock{t: time.Now().UTC().Truncate(time.Microsecond)}
	return pool, NewService(store.New(pool), notify.Noop{}, c.now), c
}

func TestIntegrationSettleCreatesWinAndOrder(t *testing.T) {
	pool, svc, c := setup(t)
	ctx := context.Background()
	seller := testdb.CreateUser(t, pool, "seller")
	bob := testdb.CreateUser(t, pool, "bob")
	carol := testdb.CreateUser(t, pool, "carol")
	id := insertAuction(t, pool, auctionSpec{seller: seller, end: c.now().Add(-time.Minute), shipping: 120, contact: "line: s",
		bids: []bidSpec{{carol, 450}, {bob, 500}}})

	n, err := svc.SettleEnded(ctx)
	if err != nil || n != 1 {
		t.Fatalf("settled %d, err %v", n, err)
	}
	if n, _ := svc.SettleEnded(ctx); n != 0 {
		t.Fatalf("second run settled %d, want 0 (idempotent)", n)
	}
	if got := notificationTypes(t, pool, bob); got["AUCTION_WON"] != 1 {
		t.Fatalf("bob notifications %v", got)
	}
	if got := notificationTypes(t, pool, carol); got["AUCTION_LOST"] != 1 || got["AUCTION_WON"] != 0 {
		t.Fatalf("carol notifications %v", got)
	}
	if got := notificationTypes(t, pool, seller); got["AUCTION_SOLD"] != 1 {
		t.Fatalf("seller notifications %v", got)
	}

	orders, err := svc.FinalizeOrders(ctx)
	if err != nil || orders != 1 {
		t.Fatalf("orders %d err %v", orders, err)
	}
	if n, _ := svc.FinalizeOrders(ctx); n != 0 {
		t.Fatalf("second finalize created %d orders", n)
	}
	var status string
	var itemsTotal, shipping, total int64
	var deadline time.Time
	var orderID uuid.UUID
	err = pool.QueryRow(ctx, `SELECT o.id, o.status, o.items_total, o.shipping_fee, o.total_amount, o.contact_deadline
		FROM orders o JOIN order_items i ON i.order_id = o.id WHERE i.auction_id = $1`, id).
		Scan(&orderID, &status, &itemsTotal, &shipping, &total, &deadline)
	if err != nil {
		t.Fatal(err)
	}
	if status != "AWAITING_CONTACT" || itemsTotal != 500 || shipping != 120 || total != 620 || !deadline.Equal(c.now().Add(24*time.Hour)) {
		t.Fatalf("order %s %d %d %d %v", status, itemsTotal, shipping, total, deadline)
	}
	if notificationTypes(t, pool, bob)["ORDER_CREATED"] != 1 || notificationTypes(t, pool, seller)["ORDER_CREATED"] != 1 {
		t.Fatal("both parties should be told the order was created")
	}
	if count(t, pool, "SELECT COUNT(*) FROM audit_logs WHERE action IN ('AUCTION_SETTLE','ORDER_CREATE')") != 2 {
		t.Fatal("settle and order creation must be audited")
	}

	// Order visibility (IDOR): buyer and seller only; contact info only to buyer.
	orders2 := order.NewService(store.New(pool), &storage.Local{BaseURL: "/uploads"}, notify.Noop{}, order.DefaultOptions, c.now)
	buyerView, err := orders2.Get(ctx, orderID, bob)
	if err != nil || buyerView.Role != "buyer" || len(buyerView.SellerContact) != 1 || len(buyerView.Items) != 1 {
		t.Fatalf("buyer view %+v err %v", buyerView, err)
	}
	sellerView, err := orders2.Get(ctx, orderID, seller)
	if err != nil || sellerView.Role != "seller" || len(sellerView.SellerContact) != 0 {
		t.Fatalf("seller view %+v err %v", sellerView, err)
	}
	if _, err := orders2.Get(ctx, orderID, carol); err != order.ErrNotFound {
		t.Fatalf("stranger must get not found, got %v", err)
	}
}

func TestIntegrationSettleWithoutBids(t *testing.T) {
	pool, svc, c := setup(t)
	seller := testdb.CreateUser(t, pool, "seller")
	insertAuction(t, pool, auctionSpec{seller: seller, end: c.now().Add(-time.Minute)})
	if n, err := svc.SettleEnded(context.Background()); err != nil || n != 1 {
		t.Fatalf("settled %d err %v", n, err)
	}
	if notificationTypes(t, pool, seller)["AUCTION_UNSOLD"] != 1 {
		t.Fatal("seller should be told nobody bid")
	}
	if count(t, pool, "SELECT COUNT(*) FROM order_items") != 0 {
		t.Fatal("no order item without a winner")
	}
}

func TestIntegrationMergeWaitsForSellersOtherAuctions(t *testing.T) {
	pool, svc, c := setup(t)
	ctx := context.Background()
	seller := testdb.CreateUser(t, pool, "seller")
	bob := testdb.CreateUser(t, pool, "bob")
	carol := testdb.CreateUser(t, pool, "carol")
	start := c.now()
	insertAuction(t, pool, auctionSpec{seller: seller, end: start.Add(-time.Minute), shipping: 100, bids: []bidSpec{{bob, 500}}})
	insertAuction(t, pool, auctionSpec{seller: seller, end: start.Add(-time.Minute), shipping: 80, bids: []bidSpec{{carol, 300}}})
	// Still running, ends within the 10 minute merge window.
	later := insertAuction(t, pool, auctionSpec{seller: seller, end: start.Add(5 * time.Minute), shipping: 150, bids: []bidSpec{{bob, 700}}})

	if n, _ := svc.SettleEnded(ctx); n != 2 {
		t.Fatalf("settled %d, want 2", n)
	}
	if notificationTypes(t, pool, bob)["ORDER_WAITING"] != 1 {
		t.Fatal("winner should be told the order waits for other auctions")
	}
	if n, _ := svc.FinalizeOrders(ctx); n != 0 {
		t.Fatalf("created %d orders while seller still has an auction ending soon", n)
	}

	// The running auction gets extended: still waiting after the original end.
	if _, err := pool.Exec(ctx, "UPDATE auctions SET end_time = $2 WHERE id = $1", later, start.Add(7*time.Minute)); err != nil {
		t.Fatal(err)
	}
	c.set(start.Add(6 * time.Minute))
	if n, _ := svc.SettleEnded(ctx); n != 0 {
		t.Fatal("extended auction must not settle early")
	}
	if n, _ := svc.FinalizeOrders(ctx); n != 0 {
		t.Fatal("must keep waiting while the extended auction runs")
	}

	c.set(start.Add(8 * time.Minute))
	if n, _ := svc.SettleEnded(ctx); n != 1 {
		t.Fatal("extended auction should settle after its new end")
	}
	if n, err := svc.FinalizeOrders(ctx); err != nil || n != 2 {
		t.Fatalf("orders %d err %v, want 2 (bob merged, carol separate)", n, err)
	}
	var items int
	var itemsTotal, shipping int64
	if err := pool.QueryRow(ctx, `SELECT (SELECT COUNT(*) FROM order_items i WHERE i.order_id = o.id), o.items_total, o.shipping_fee
		FROM orders o WHERE o.buyer_id = $1`, bob).Scan(&items, &itemsTotal, &shipping); err != nil {
		t.Fatal(err)
	}
	if items != 2 || itemsTotal != 1200 || shipping != 150 {
		t.Fatalf("bob's merged order: items=%d total=%d shipping=%d", items, itemsTotal, shipping)
	}
}

func TestIntegrationConcurrentRunsDoNotDuplicate(t *testing.T) {
	pool, svc, c := setup(t)
	seller := testdb.CreateUser(t, pool, "seller")
	bob := testdb.CreateUser(t, pool, "bob")
	for range 5 {
		insertAuction(t, pool, auctionSpec{seller: seller, end: c.now().Add(-time.Minute), bids: []bidSpec{{bob, 100}}})
	}
	var wg sync.WaitGroup
	for range 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = svc.RunOnce(context.Background())
		}()
	}
	wg.Wait()
	_ = svc.RunOnce(context.Background())
	if n := count(t, pool, "SELECT COUNT(*) FROM order_items"); n != 5 {
		t.Fatalf("order items %d, want 5", n)
	}
	if n := count(t, pool, "SELECT COUNT(*) FROM orders"); n != 1 {
		t.Fatalf("orders %d, want 1 merged order", n)
	}
	if n := notificationTypes(t, pool, bob)["AUCTION_WON"]; n != 5 {
		t.Fatalf("won notifications %d, want 5", n)
	}
	if n := notificationTypes(t, pool, bob)["ORDER_CREATED"]; n != 1 {
		t.Fatalf("order created notifications %d, want 1", n)
	}
}

func TestIntegrationEndingSoonNotifiesOnce(t *testing.T) {
	pool, svc, c := setup(t)
	ctx := context.Background()
	seller := testdb.CreateUser(t, pool, "seller")
	bob := testdb.CreateUser(t, pool, "bob")
	carol := testdb.CreateUser(t, pool, "carol")
	insertAuction(t, pool, auctionSpec{seller: seller, end: c.now().Add(5 * time.Minute), bids: []bidSpec{{bob, 100}, {carol, 200}, {bob, 300}}})
	insertAuction(t, pool, auctionSpec{seller: seller, end: c.now().Add(5 * time.Minute)})                               // no bids
	insertAuction(t, pool, auctionSpec{seller: seller, end: c.now().Add(30 * time.Minute), bids: []bidSpec{{bob, 100}}}) // too far

	if n, err := svc.NotifyEndingSoon(ctx); err != nil || n != 1 {
		t.Fatalf("claimed %d err %v", n, err)
	}
	if n, _ := svc.NotifyEndingSoon(ctx); n != 0 {
		t.Fatal("must notify only once")
	}
	if notificationTypes(t, pool, bob)["AUCTION_ENDING_SOON"] != 1 || notificationTypes(t, pool, carol)["AUCTION_ENDING_SOON"] != 1 {
		t.Fatal("each bidder gets exactly one ending-soon notification")
	}
	if notificationTypes(t, pool, seller)["AUCTION_ENDING_SOON"] != 0 {
		t.Fatal("seller is not a bidder")
	}
}

func TestIntegrationBannedWinnerGetsNoOrder(t *testing.T) {
	pool, svc, c := setup(t)
	ctx := context.Background()
	seller := testdb.CreateUser(t, pool, "seller")
	bob := testdb.CreateUser(t, pool, "bob")
	insertAuction(t, pool, auctionSpec{seller: seller, end: c.now().Add(-time.Minute), bids: []bidSpec{{bob, 500}}})
	if _, err := pool.Exec(ctx, "UPDATE users SET status='BANNED' WHERE id=$1", bob); err != nil {
		t.Fatal(err)
	}
	if n, err := svc.SettleEnded(ctx); err != nil || n != 1 {
		t.Fatalf("settled %d err %v", n, err)
	}
	_, _ = svc.FinalizeOrders(ctx)
	if count(t, pool, "SELECT COUNT(*) FROM order_items") != 0 || count(t, pool, "SELECT COUNT(*) FROM orders") != 0 {
		t.Fatal("a banned winner must not get an order")
	}
	if notificationTypes(t, pool, seller)["AUCTION_UNSOLD"] != 1 {
		t.Fatal("seller should be told the auction ended without an order")
	}
}
