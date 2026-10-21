package order

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v4"

	"auction-platform/backend/internal/apperr"
	"auction-platform/backend/internal/database/db"
)

func TestCanTransition(t *testing.T) {
	allowed := [][2]Status{
		{AwaitingContact, AwaitingPayment},
		{AwaitingPayment, Paid},
		{Paid, AwaitingShipment},
		{AwaitingShipment, Shipped},
		{Shipped, Received},
		{Received, Completed},
		{AwaitingContact, Cancelled},
		{AwaitingPayment, Cancelled},
		{Shipped, Disputed},
		{Disputed, Completed},
		{Disputed, Cancelled},
	}
	for _, tr := range allowed {
		if !CanTransition(tr[0], tr[1]) {
			t.Errorf("%s -> %s should be allowed", tr[0], tr[1])
		}
	}
	denied := [][2]Status{
		{AwaitingContact, Paid},      // cannot skip contact
		{AwaitingPayment, Shipped},   // cannot ship before payment
		{Paid, Cancelled},            // paid orders go through dispute (refund)
		{Shipped, Completed},         // must be received first
		{Completed, Disputed},        // terminal
		{Cancelled, AwaitingPayment}, // terminal
		{Received, Shipped},          // no going back
		{AwaitingContact, AwaitingContact},
	}
	for _, tr := range denied {
		if CanTransition(tr[0], tr[1]) {
			t.Errorf("%s -> %s should be denied", tr[0], tr[1])
		}
	}
	if !IsTerminal(Completed) || !IsTerminal(Cancelled) || IsTerminal(Disputed) {
		t.Error("terminal states wrong")
	}
}

func newQueries(t *testing.T) (*db.Queries, pgxmock.PgxPoolIface) {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		mock.Close()
	})
	return db.New(mock), mock
}

func TestTransitionWritesAuditOnSuccess(t *testing.T) {
	q, mock := newQueries(t)
	id, actor := uuid.New(), uuid.New()
	now := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	mock.ExpectExec("UPDATE orders").WithArgs("AWAITING_PAYMENT", now, id, "AWAITING_CONTACT").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec("INSERT INTO audit_logs").
		WithArgs(pgxmock.AnyArg(), "ORDER_STATUS_CHANGE", "ORDER", id.String(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), now).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	if err := Transition(context.Background(), q, now, id, AwaitingContact, AwaitingPayment, actor, nil); err != nil {
		t.Fatal(err)
	}
}

func TestTransitionRejectsStaleStatus(t *testing.T) {
	q, mock := newQueries(t)
	id := uuid.New()
	now := time.Now().UTC()
	mock.ExpectExec("UPDATE orders").WithArgs("AWAITING_PAYMENT", now, id, "AWAITING_CONTACT").WillReturnResult(pgxmock.NewResult("UPDATE", 0))
	err := Transition(context.Background(), q, now, id, AwaitingContact, AwaitingPayment, uuid.Nil, nil)
	if !apperr.IsCode(err, "ORDER_STATUS_CHANGED") {
		t.Fatalf("got %v", err)
	}
}

func TestTransitionRejectsInvalidWithoutQuery(t *testing.T) {
	q, _ := newQueries(t)
	err := Transition(context.Background(), q, time.Now(), uuid.New(), AwaitingContact, Shipped, uuid.Nil, nil)
	if !apperr.IsCode(err, "INVALID_ORDER_TRANSITION") {
		t.Fatalf("got %v", err)
	}
}
