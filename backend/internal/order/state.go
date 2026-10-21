// Package order implements orders and their state machine.
package order

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"auction-platform/backend/internal/apperr"
	"auction-platform/backend/internal/audit"
	"auction-platform/backend/internal/database/db"
)

type Status string

const (
	AwaitingContact  Status = "AWAITING_CONTACT"
	AwaitingPayment  Status = "AWAITING_PAYMENT"
	Paid             Status = "PAID"
	AwaitingShipment Status = "AWAITING_SHIPMENT"
	Shipped          Status = "SHIPPED"
	Received         Status = "RECEIVED"
	Completed        Status = "COMPLETED"
	Cancelled        Status = "CANCELLED"
	Disputed         Status = "DISPUTED"
)

// transitions is the complete set of allowed status changes. Who may trigger
// each one (buyer, seller, payment callback, scheduler, admin) is checked by
// the operation performing it.
//
//	AWAITING_CONTACT  -> AWAITING_PAYMENT   buyer sends the first message
//	AWAITING_PAYMENT  -> PAID               verified ECPay callback
//	PAID              -> AWAITING_SHIPMENT  automatically after payment
//	AWAITING_SHIPMENT -> SHIPPED            seller enters shipment info
//	SHIPPED           -> RECEIVED           buyer confirms, or 3 days after shipping
//	RECEIVED          -> COMPLETED          buyer confirms, or 7 days after receipt
//	unpaid states     -> CANCELLED          admin
//	any open state    -> DISPUTED           dispute raised
//	DISPUTED          -> COMPLETED / CANCELLED  admin resolution
var transitions = map[Status][]Status{
	AwaitingContact:  {AwaitingPayment, Cancelled, Disputed},
	AwaitingPayment:  {Paid, Cancelled, Disputed},
	Paid:             {AwaitingShipment, Disputed},
	AwaitingShipment: {Shipped, Disputed},
	Shipped:          {Received, Disputed},
	Received:         {Completed, Disputed},
	Disputed:         {Completed, Cancelled},
}

// CanTransition reports whether from -> to is allowed.
func CanTransition(from, to Status) bool {
	for _, s := range transitions[from] {
		if s == to {
			return true
		}
	}
	return false
}

// IsTerminal reports whether no further transitions are possible.
func IsTerminal(s Status) bool { return len(transitions[s]) == 0 }

var (
	ErrInvalidTransition = apperr.Conflict("INVALID_ORDER_TRANSITION", "訂單目前狀態無法進行此操作")
	ErrStaleStatus       = apperr.Conflict("ORDER_STATUS_CHANGED", "訂單狀態已變更，請重新整理")
)

// Transition moves an order from -> to inside the caller's transaction and
// records an audit entry. The UPDATE is conditional on the current status, so
// concurrent transitions cannot both succeed. actor is uuid.Nil for system
// (scheduler / payment callback) changes.
func Transition(ctx context.Context, q *db.Queries, now time.Time, orderID uuid.UUID, from, to Status, actor uuid.UUID, meta map[string]any) error {
	if !CanTransition(from, to) {
		return ErrInvalidTransition
	}
	n, err := q.UpdateOrderStatus(ctx, db.UpdateOrderStatusParams{
		ToStatus: string(to), Now: now.UTC(), ID: orderID, FromStatus: string(from),
	})
	if err != nil {
		return fmt.Errorf("update order status: %w", err)
	}
	if n == 0 {
		return ErrStaleStatus
	}
	m := map[string]any{"from": from, "to": to}
	for k, v := range meta {
		m[k] = v
	}
	return audit.Record(ctx, q, now, audit.Entry{
		Actor: actor, Action: audit.ActionOrderStatus, TargetType: audit.TargetOrder, TargetID: orderID.String(), Metadata: m,
	})
}
