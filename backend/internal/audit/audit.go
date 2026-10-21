// Package audit writes append-only audit log entries. Entries for a business
// change should be written with the same transaction's queries so the change
// and its audit record commit or roll back together.
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"auction-platform/backend/internal/database/db"
	"auction-platform/backend/internal/httpx"
)

// Actions recorded in audit_logs.action.
const (
	ActionLogin             = "LOGIN"
	ActionLoginRejectBanned = "LOGIN_REJECTED_BANNED"
	ActionLogout            = "LOGOUT"
	ActionUserCreate        = "USER_CREATE"
	ActionTermsAccept       = "TERMS_ACCEPT"
	ActionAuctionCreate     = "AUCTION_CREATE"
	ActionAuctionUpdate     = "AUCTION_UPDATE"
	ActionBidPlace          = "BID_PLACE"
	ActionAuctionSettle     = "AUCTION_SETTLE"
	ActionOrderCreate       = "ORDER_CREATE"
	ActionOrderStatus       = "ORDER_STATUS_CHANGE"
	ActionOrderOverdue      = "ORDER_OVERDUE"
	ActionShippingInfoSet   = "SHIPPING_INFO_SET"
	ActionShipmentCreate    = "SHIPMENT_CREATE"
	ActionPaymentCreate     = "PAYMENT_CREATE"
	ActionPaymentStatus     = "PAYMENT_STATUS_CHANGE"
	ActionReportCreate      = "REPORT_CREATE"
)

// Target types recorded in audit_logs.target_type.
const (
	TargetUser    = "USER"
	TargetAuction = "AUCTION"
	TargetOrder   = "ORDER"
	TargetPayment = "PAYMENT"
	TargetReport  = "REPORT"
)

type Inserter interface {
	InsertAuditLog(ctx context.Context, arg db.InsertAuditLogParams) error
}

type Entry struct {
	Actor      uuid.UUID // uuid.Nil for system actions
	Action     string
	TargetType string
	TargetID   string
	Metadata   map[string]any
}

// Record inserts e, taking the client IP and user agent from ctx.
func Record(ctx context.Context, q Inserter, now time.Time, e Entry) error {
	meta := []byte("{}")
	if len(e.Metadata) > 0 {
		b, err := json.Marshal(e.Metadata)
		if err != nil {
			return fmt.Errorf("marshal audit metadata: %w", err)
		}
		meta = b
	}
	rm := httpx.MetaFromContext(ctx)
	err := q.InsertAuditLog(ctx, db.InsertAuditLogParams{
		ActorUserID: uuid.NullUUID{UUID: e.Actor, Valid: e.Actor != uuid.Nil},
		Action:      e.Action,
		TargetType:  e.TargetType,
		TargetID:    e.TargetID,
		Metadata:    meta,
		Ip:          rm.IP,
		UserAgent:   rm.UserAgent,
		CreatedAt:   now.UTC(),
	})
	if err != nil {
		return fmt.Errorf("insert audit log %s: %w", e.Action, err)
	}
	return nil
}
