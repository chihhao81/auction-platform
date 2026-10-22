package review

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"auction-platform/backend/internal/apperr"
	"auction-platform/backend/internal/notify"
	"auction-platform/backend/internal/order"
	"auction-platform/backend/internal/store"
	"auction-platform/backend/internal/testdb"
)

// Integration test: needs TEST_DATABASE_URL.
func TestIntegrationReviews(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	svc := NewService(store.New(pool), notify.Noop{}, func() time.Time { return time.Now().UTC() })
	buyer, seller, stranger := testdb.CreateUser(t, pool, "buyer"), testdb.CreateUser(t, pool, "seller"), testdb.CreateUser(t, pool, "stranger")
	mk := func(status string) uuid.UUID {
		var id uuid.UUID
		if err := pool.QueryRow(ctx, `INSERT INTO orders (buyer_id, seller_id, items_total, shipping_fee, total_amount, status, contact_deadline)
			VALUES ($1, $2, 100, 0, 100, $3, NOW()) RETURNING id`, buyer, seller, status).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	done, open := mk("COMPLETED"), mk("SHIPPED")

	if err := svc.Create(ctx, buyer, open, Positive, ""); err != ErrNotCompleted {
		t.Fatalf("review before completion: %v", err)
	}
	if err := svc.Create(ctx, stranger, done, Positive, ""); err != order.ErrNotFound {
		t.Fatalf("stranger review: %v", err)
	}
	if err := svc.Create(ctx, buyer, done, "FIVE_STARS", ""); !apperr.IsCode(err, "INVALID_RATING") {
		t.Fatalf("bad rating: %v", err)
	}
	if err := svc.Create(ctx, buyer, done, Positive, strings.Repeat("好", 501)); !apperr.IsCode(err, "INVALID_COMMENT") {
		t.Fatalf("long comment: %v", err)
	}
	if err := svc.Create(ctx, buyer, done, Positive, "守宮很健康，包裝仔細"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Create(ctx, buyer, done, Negative, "改主意"); err != ErrAlreadyReviewed {
		t.Fatalf("second review: %v", err)
	}
	if err := svc.Create(ctx, seller, done, Negative, "付款很慢"); err != nil {
		t.Fatal(err)
	}

	p, err := svc.Profile(ctx, seller)
	if err != nil {
		t.Fatal(err)
	}
	if p.Reputation.Positive != 1 || p.Reputation.Negative != 0 || p.Reputation.CompletedOrders != 1 {
		t.Fatalf("seller reputation %+v", p.Reputation)
	}
	if len(p.Reviews) != 1 || !p.Reviews[0].FromBuyer || p.Reviews[0].Comment != "守宮很健康，包裝仔細" {
		t.Fatalf("seller reviews %+v", p.Reviews)
	}
	bp, _ := svc.Profile(ctx, buyer)
	if bp.Reputation.Negative != 1 || bp.Reviews[0].FromBuyer {
		t.Fatalf("buyer profile %+v", bp)
	}
	if _, err := svc.Profile(ctx, uuid.New()); err != ErrUserNotFound {
		t.Fatalf("unknown profile: %v", err)
	}
}
