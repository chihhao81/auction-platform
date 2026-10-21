// Package notify creates in-app notifications and delivers them as Web Push.
//
// Notifications are inserted with the caller's transaction queries (Batch.Add)
// so they commit atomically with the business change. Push delivery happens
// only after commit (Dispatcher.Dispatch) and can never roll anything back.
package notify

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"

	"auction-platform/backend/internal/database/db"
)

type Type string

const (
	TypeOutbid          Type = "OUTBID"
	TypeAuctionEnding   Type = "AUCTION_ENDING_SOON"
	TypeAuctionWon      Type = "AUCTION_WON"
	TypeAuctionLost     Type = "AUCTION_LOST"
	TypeAuctionSold     Type = "AUCTION_SOLD"
	TypeAuctionUnsold   Type = "AUCTION_UNSOLD"
	TypeOrderWaiting    Type = "ORDER_WAITING"
	TypeOrderCreated    Type = "ORDER_CREATED"
	TypeOrderStatus     Type = "ORDER_STATUS_CHANGED"
	TypeMessageReceived Type = "MESSAGE_RECEIVED"
)

// TypeInfo describes a type for the preference settings UI.
type TypeInfo struct {
	Type  Type   `json:"type"`
	Label string `json:"label"`
}

// Types lists the notification types a user can toggle push for.
var Types = []TypeInfo{
	{TypeOutbid, "出價被超越"},
	{TypeAuctionEnding, "參與的競標即將結束"},
	{TypeAuctionWon, "得標"},
	{TypeAuctionLost, "未得標"},
	{TypeAuctionSold, "我的商品結標"},
	{TypeAuctionUnsold, "我的商品無人出價"},
	{TypeOrderWaiting, "等待其他競標結束"},
	{TypeOrderCreated, "訂單成立"},
	{TypeOrderStatus, "訂單狀態變更"},
	{TypeMessageReceived, "收到訂單訊息"},
}

// ValidType reports whether t is a known notification type.
func ValidType(t Type) bool {
	for _, i := range Types {
		if i.Type == t {
			return true
		}
	}
	return false
}

// Message is a notification to create.
type Message struct {
	UserID    uuid.UUID
	Type      Type
	Title     string
	Content   string
	AuctionID uuid.UUID // uuid.Nil if none
	OrderID   uuid.UUID // uuid.Nil if none
}

// Batch collects notifications created inside a transaction so they can be
// pushed after it commits.
type Batch []db.Notification

// Add inserts m using the transaction's queries and remembers it.
func (b *Batch) Add(ctx context.Context, q *db.Queries, now time.Time, m Message) error {
	n, err := q.InsertNotification(ctx, db.InsertNotificationParams{
		UserID:           m.UserID,
		Type:             string(m.Type),
		Title:            m.Title,
		Content:          m.Content,
		RelatedAuctionID: uuid.NullUUID{UUID: m.AuctionID, Valid: m.AuctionID != uuid.Nil},
		RelatedOrderID:   uuid.NullUUID{UUID: m.OrderID, Valid: m.OrderID != uuid.Nil},
		CreatedAt:        now.UTC(),
	})
	if err != nil {
		return fmt.Errorf("insert notification %s: %w", m.Type, err)
	}
	*b = append(*b, n)
	return nil
}

// Dispatcher delivers committed notifications out of band.
type Dispatcher interface {
	Dispatch(notifications []db.Notification)
}

// Noop discards notifications (push disabled, tests).
type Noop struct{}

func (Noop) Dispatch([]db.Notification) {}

// FormatPrice renders whole NT dollars as "NT$ 1,234".
func FormatPrice(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := n < 0
	if neg {
		s = s[1:]
	}
	out := make([]byte, 0, len(s)+len(s)/3)
	for i := range len(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, s[i])
	}
	if neg {
		return "NT$ -" + string(out)
	}
	return "NT$ " + string(out)
}

// Link returns the frontend path a notification opens.
func Link(n db.Notification) string {
	switch {
	case n.RelatedOrderID.Valid:
		return "/orders/" + n.RelatedOrderID.UUID.String()
	case n.RelatedAuctionID.Valid:
		return "/auctions/" + n.RelatedAuctionID.UUID.String()
	}
	return "/notifications"
}
