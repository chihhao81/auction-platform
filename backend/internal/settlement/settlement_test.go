package settlement

import (
	"testing"

	"github.com/google/uuid"

	"auction-platform/backend/internal/database/db"
)

func TestTotalsUseHighestShippingFee(t *testing.T) {
	items := []db.OrderItem{{Amount: 500, ShippingFee: 100}, {Amount: 800, ShippingFee: 150}, {Amount: 50, ShippingFee: 0}}
	total, ship := totals(items)
	if total != 1350 || ship != 150 {
		t.Fatalf("total=%d ship=%d", total, ship)
	}
}

func TestGroupByBuyer(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	items := []db.OrderItem{{BuyerID: a, Amount: 1}, {BuyerID: b, Amount: 2}, {BuyerID: a, Amount: 3}}
	groups := groupByBuyer(items)
	if len(groups) != 2 || len(groups[0]) != 2 || groups[0][1].Amount != 3 || groups[1][0].BuyerID != b {
		t.Fatalf("groups = %+v", groups)
	}
}
