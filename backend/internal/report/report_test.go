package report

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"auction-platform/backend/internal/apperr"
	"auction-platform/backend/internal/store"
	"auction-platform/backend/internal/testdb"
)

// Integration test: needs TEST_DATABASE_URL.
func TestIntegrationReports(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	svc := NewService(store.New(pool), func() time.Time { return time.Now().UTC() })
	reporter, target := testdb.CreateUser(t, pool, "reporter"), testdb.CreateUser(t, pool, "target")
	var auctionID uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO auctions (seller_id, title, description, start_price, min_increment, max_increment,
		current_price, start_time, end_time, original_end_time, shipping_method, shipping_fee)
		VALUES ($1, 't', '', 0, 1, 1, 0, NOW(), NOW() + INTERVAL '1 hour', NOW() + INTERVAL '1 hour', 'post', 0) RETURNING id`, target).Scan(&auctionID); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		by   uuid.UUID
		in   Input
		code string
	}{
		{"self", reporter, Input{TargetType: TargetUser, TargetID: reporter, Reason: "FRAUD"}, "CANNOT_REPORT_SELF"},
		{"own auction", target, Input{TargetType: TargetAuction, TargetID: auctionID, Reason: "FRAUD"}, "CANNOT_REPORT_SELF"},
		{"bad type", reporter, Input{TargetType: "ORDER", TargetID: target, Reason: "FRAUD"}, "INVALID_TARGET_TYPE"},
		{"reason for other type", reporter, Input{TargetType: TargetAuction, TargetID: auctionID, Reason: "HARASSMENT"}, "INVALID_REASON"},
		{"missing user", reporter, Input{TargetType: TargetUser, TargetID: uuid.New(), Reason: "FRAUD"}, "REPORT_TARGET_NOT_FOUND"},
		{"missing auction", reporter, Input{TargetType: TargetAuction, TargetID: uuid.New(), Reason: "FRAUD"}, "REPORT_TARGET_NOT_FOUND"},
	}
	for _, tc := range cases {
		if _, err := svc.Create(ctx, tc.by, tc.in); !apperr.IsCode(err, tc.code) {
			t.Errorf("%s: got %v want %s", tc.name, err, tc.code)
		}
	}

	r, err := svc.Create(ctx, reporter, Input{TargetType: TargetAuction, TargetID: auctionID, Reason: "PROHIBITED_ITEM", Description: "疑似保育類"})
	if err != nil {
		t.Fatal(err)
	}
	if !r.TargetUserID.Valid || r.TargetUserID.UUID != target || r.Status != "PENDING" {
		t.Fatalf("auction report should also point at the seller: %+v", r)
	}
	if _, err := svc.Create(ctx, reporter, Input{TargetType: TargetAuction, TargetID: auctionID, Reason: "FRAUD"}); err != ErrDuplicate {
		t.Fatalf("duplicate open report: %v", err)
	}
	if _, err := svc.Create(ctx, reporter, Input{TargetType: TargetUser, TargetID: target, Reason: "FRAUD"}); err != nil {
		t.Fatalf("a user report is a different target: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE reports SET status='REJECTED' WHERE id=$1", r.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, reporter, Input{TargetType: TargetAuction, TargetID: auctionID, Reason: "FRAUD"}); err != nil {
		t.Fatalf("can report again after the previous one is closed: %v", err)
	}
}
