package auction

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"auction-platform/backend/internal/apperr"
	"auction-platform/backend/internal/notify"
	"auction-platform/backend/internal/store"
	"auction-platform/backend/internal/testdb"
)

// These tests need a real PostgreSQL (TEST_DATABASE_URL) because they verify
// row locking and constraints, which a mock cannot.

func insertAuction(t *testing.T, pool *pgxpool.Pool, sellerID uuid.UUID, title string, start, end time.Time, startPrice, minInc, maxInc int64) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := pool.QueryRow(context.Background(), `
		INSERT INTO auctions (seller_id, title, description, start_price, min_increment, max_increment,
		                      current_price, start_time, end_time, original_end_time, shipping_method, shipping_fee)
		VALUES ($1, $2, '', $3, $4, $5, $3, $6, $7, $7, 'post', 0) RETURNING id`,
		sellerID, title, startPrice, minInc, maxInc, start, end).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestIntegrationConcurrentSamePriceBids(t *testing.T) {
	pool := testdb.New(t)
	svc := NewService(store.New(pool), newMemStorage(), notify.Noop{}, func() time.Time { return time.Now().UTC() })
	now := time.Now().UTC()
	sellerID := testdb.CreateUser(t, pool, "seller")
	// min == max: exactly one legal next bid, so only one of N racers may win.
	id := insertAuction(t, pool, sellerID, "race", now.Add(-time.Hour), now.Add(time.Hour), 1000, 50, 50)

	const n = 10
	bidders := make([]uuid.UUID, n)
	for i := range bidders {
		bidders[i] = testdb.CreateUser(t, pool, fmt.Sprintf("b%d", i))
	}
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, errs[i] = svc.PlaceBid(context.Background(), bidders[i], id, 1050)
		}(i)
	}
	close(start)
	wg.Wait()

	wins := 0
	for _, err := range errs {
		switch {
		case err == nil:
			wins++
		case apperr.IsCode(err, "BID_OUT_OF_RANGE"):
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if wins != 1 {
		t.Fatalf("%d bids succeeded, want exactly 1", wins)
	}
	var price int64
	var count, rows int
	if err := pool.QueryRow(context.Background(), "SELECT current_price, bid_count FROM auctions WHERE id=$1", id).Scan(&price, &count); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM bids WHERE auction_id=$1", id).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if price != 1050 || count != 1 || rows != 1 {
		t.Fatalf("price=%d bid_count=%d bid rows=%d", price, count, rows)
	}
}

func TestIntegrationConcurrentMixedBidsNeverLowerPrice(t *testing.T) {
	pool := testdb.New(t)
	svc := NewService(store.New(pool), newMemStorage(), notify.Noop{}, func() time.Time { return time.Now().UTC() })
	now := time.Now().UTC()
	sellerID := testdb.CreateUser(t, pool, "seller")
	id := insertAuction(t, pool, sellerID, "mixed", now.Add(-time.Hour), now.Add(time.Hour), 0, 10, 1000)

	const n = 12
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range n {
		bidder := testdb.CreateUser(t, pool, fmt.Sprintf("m%d", i))
		wg.Add(1)
		go func(amount int64) {
			defer wg.Done()
			<-start
			_, _ = svc.PlaceBid(context.Background(), bidder, id, amount)
		}(int64(10 + i*37))
	}
	close(start)
	wg.Wait()

	// Accepted bids, in acceptance order, must be strictly increasing and the
	// auction must reflect the last (highest) one.
	rows, err := pool.Query(context.Background(), "SELECT amount, bidder_id FROM bids WHERE auction_id=$1 ORDER BY created_at, amount", id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var last int64 = -1
	var lastBidder uuid.UUID
	count := 0
	for rows.Next() {
		var amt int64
		if err := rows.Scan(&amt, &lastBidder); err != nil {
			t.Fatal(err)
		}
		if amt <= last {
			t.Fatalf("bid %d accepted after higher bid %d", amt, last)
		}
		last = amt
		count++
	}
	var price int64
	var bidCount int
	var highest uuid.UUID
	if err := pool.QueryRow(context.Background(), "SELECT current_price, bid_count, highest_bidder_id FROM auctions WHERE id=$1", id).Scan(&price, &bidCount, &highest); err != nil {
		t.Fatal(err)
	}
	if count == 0 || price != last || bidCount != count || highest != lastBidder {
		t.Fatalf("auction price=%d count=%d highest=%v; bids last=%d count=%d bidder=%v", price, bidCount, highest, last, count, lastBidder)
	}
}

func TestIntegrationBidExtendsEndTime(t *testing.T) {
	pool := testdb.New(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	svc := NewService(store.New(pool), newMemStorage(), notify.Noop{}, func() time.Time { return now })
	sellerID := testdb.CreateUser(t, pool, "seller")
	bidder := testdb.CreateUser(t, pool, "bidder")
	end := now.Add(30 * time.Second)
	id := insertAuction(t, pool, sellerID, "ext", now.Add(-time.Hour), end, 0, 50, 50)

	res, err := svc.PlaceBid(context.Background(), bidder, id, 50)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Extended || !res.Auction.EndTime.Equal(end.Add(2*time.Minute)) {
		t.Fatalf("end=%v extended=%v", res.Auction.EndTime, res.Extended)
	}
	var orig time.Time
	if err := pool.QueryRow(context.Background(), "SELECT original_end_time FROM auctions WHERE id=$1", id).Scan(&orig); err != nil {
		t.Fatal(err)
	}
	if !orig.Equal(end) {
		t.Fatalf("original_end_time changed to %v", orig)
	}
}

func TestIntegrationOpenAuctionLimitUnderConcurrency(t *testing.T) {
	pool := testdb.New(t)
	now := time.Now().UTC()
	svc := NewService(store.New(pool), newMemStorage(), notify.Noop{}, func() time.Time { return now })
	sellerID := testdb.CreateUser(t, pool, "seller")
	png := pngBytes(t)

	const n = 8
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			in := Input{Title: fmt.Sprintf("a%d", i), ShippingMethod: "post"}
			_, errs[i] = svc.Create(context.Background(), sellerID, in, []Upload{{Filename: "a.png", Data: png}})
		}(i)
	}
	close(start)
	wg.Wait()
	ok := 0
	for _, err := range errs {
		if err == nil {
			ok++
		} else if !apperr.IsCode(err, "TOO_MANY_OPEN_AUCTIONS") {
			t.Errorf("unexpected error %v", err)
		}
	}
	if ok != MaxOpenAuctionsPerSeller {
		t.Fatalf("%d auctions created, want %d", ok, MaxOpenAuctionsPerSeller)
	}
}

func TestIntegrationSearchAndPrices(t *testing.T) {
	pool := testdb.New(t)
	now := time.Now().UTC()
	svc := NewService(store.New(pool), newMemStorage(), notify.Noop{}, func() time.Time { return now })
	sellerID := testdb.CreateUser(t, pool, "seller")
	active := insertAuction(t, pool, sellerID, "豹紋守宮 active", now.Add(-time.Hour), now.Add(time.Hour), 0, 50, 50)
	upcoming := insertAuction(t, pool, sellerID, "鼠婦 upcoming", now.Add(time.Hour), now.Add(3*time.Hour), 0, 50, 50)
	ended := insertAuction(t, pool, sellerID, "守宮 ended 100%", now.Add(-5*time.Hour), now.Add(-time.Hour), 0, 50, 50)
	closed := insertAuction(t, pool, sellerID, "守宮 closed", now.Add(-50*time.Hour), now.Add(-25*time.Hour), 0, 50, 50)

	ids := func(items []Summary) []string {
		out := make([]string, len(items))
		for i, it := range items {
			out[i] = it.ID
		}
		return out
	}
	check := func(keyword, status string, want ...uuid.UUID) {
		t.Helper()
		items, _, err := svc.Search(context.Background(), keyword, status, 1)
		if err != nil {
			t.Fatal(err)
		}
		got := ids(items)
		if len(got) != len(want) {
			t.Fatalf("Search(%q,%q) = %v, want %v", keyword, status, got, want)
		}
		for i := range want {
			if got[i] != want[i].String() {
				t.Fatalf("Search(%q,%q) = %v, want %v (order matters)", keyword, status, got, want)
			}
		}
	}
	check("", "", active, upcoming, ended) // closed hidden; active, upcoming, ended order
	check("", "ACTIVE", active)
	check("", "UPCOMING", upcoming)
	check("", "ENDED", ended)
	check("守宮", "", active, ended)
	check("100%", "", ended) // % is literal
	check("1_0", "")         // _ is literal, matches nothing

	prices, _, err := svc.Prices(context.Background(), []uuid.UUID{active, closed, upcoming})
	if err != nil {
		t.Fatal(err)
	}
	if len(prices) != 2 {
		t.Fatalf("prices should omit closed auctions: %+v", prices)
	}
}

func TestIntegrationAuditLogIsAppendOnly(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "INSERT INTO audit_logs (action, target_type) VALUES ('X', 'Y')"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE audit_logs SET action = 'Z'"); err == nil {
		t.Fatal("UPDATE on audit_logs must fail")
	}
	if _, err := pool.Exec(ctx, "DELETE FROM audit_logs"); err == nil {
		t.Fatal("DELETE on audit_logs must fail")
	}
}
