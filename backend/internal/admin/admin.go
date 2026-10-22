// Package admin implements the admin back office. Every route requires the
// ADMIN role, and every state-changing action is written to the audit log
// with the acting admin, in the same transaction as the change.
package admin

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"auction-platform/backend/internal/apperr"
	"auction-platform/backend/internal/auction"
	"auction-platform/backend/internal/audit"
	"auction-platform/backend/internal/auth"
	"auction-platform/backend/internal/database/db"
	"auction-platform/backend/internal/notify"
	"auction-platform/backend/internal/order"
	"auction-platform/backend/internal/report"
	"auction-platform/backend/internal/store"
	"auction-platform/backend/internal/textutil"
)

const pageSize = 30

var (
	ErrUserNotFound    = apperr.NotFound("USER_NOT_FOUND", "找不到此使用者")
	ErrReportNotFound  = apperr.NotFound("REPORT_NOT_FOUND", "找不到此檢舉")
	ErrCannotBanAdmin  = apperr.Forbidden("CANNOT_BAN_ADMIN", "不能停權管理員或自己")
	ErrAlreadyBanned   = apperr.Conflict("ALREADY_BANNED", "此使用者已被停權")
	ErrNotBanned       = apperr.Conflict("NOT_BANNED", "此使用者未被停權")
	ErrAlreadyClosed   = apperr.Conflict("AUCTION_ALREADY_CLOSED", "此競標已關閉")
	ErrAlreadySettled  = apperr.Conflict("AUCTION_ALREADY_SETTLED", "此競標已結標，請改為處理訂單")
	ErrNotCancellable  = apperr.Conflict("ORDER_NOT_CANCELLABLE", "只有尚未付款或爭議中的訂單可以取消")
	ErrInvalidReason   = apperr.Invalid("INVALID_REASON", "請填寫原因（500 字以內）")
	ErrInvalidReportOp = apperr.Invalid("INVALID_REPORT_STATUS", "檢舉狀態只能設為 REVIEWING、RESOLVED 或 REJECTED")
)

type Service struct {
	store    *store.Store
	dispatch notify.Dispatcher
	now      func() time.Time
}

func NewService(st *store.Store, d notify.Dispatcher, now func() time.Time) *Service {
	return &Service{store: st, dispatch: d, now: now}
}

func reason(s string) (string, error) {
	s, ok := textutil.Clean(s, 500, true)
	if !ok || s == "" {
		return "", ErrInvalidReason
	}
	return s, nil
}

// Ban blocks a user: status BANNED, a ban history row, and every session
// revoked so the ban applies immediately. History data is never deleted.
func (s *Service) Ban(ctx context.Context, adminID, userID uuid.UUID, why string) error {
	why, err := reason(why)
	if err != nil {
		return err
	}
	return s.store.InTx(ctx, func(q *db.Queries) error {
		u, err := q.GetUserForUpdate(ctx, userID)
		if store.IsNoRows(err) {
			return ErrUserNotFound
		}
		if err != nil {
			return err
		}
		if u.ID == adminID || u.Role == auth.RoleAdmin {
			return ErrCannotBanAdmin
		}
		if u.Status == auth.StatusBanned {
			return ErrAlreadyBanned
		}
		now := s.now().UTC()
		if err := q.SetUserStatus(ctx, db.SetUserStatusParams{ID: u.ID, Status: auth.StatusBanned, UpdatedAt: now}); err != nil {
			return err
		}
		ban, err := q.InsertUserBan(ctx, db.InsertUserBanParams{UserID: u.ID, Reason: why, BannedBy: adminID, BannedAt: now})
		if err != nil {
			return fmt.Errorf("insert ban: %w", err)
		}
		if err := q.RevokeUserSessions(ctx, db.RevokeUserSessionsParams{Now: now, UserID: u.ID}); err != nil {
			return fmt.Errorf("revoke sessions: %w", err)
		}
		return audit.Record(ctx, q, now, audit.Entry{
			Actor: adminID, Action: audit.ActionUserBan, TargetType: audit.TargetUser, TargetID: u.ID.String(),
			Metadata: map[string]any{"reason": why, "ban_id": ban.ID.String()},
		})
	})
}

// Unban lifts the user's active ban.
func (s *Service) Unban(ctx context.Context, adminID, userID uuid.UUID, why string) error {
	why, err := reason(why)
	if err != nil {
		return err
	}
	return s.store.InTx(ctx, func(q *db.Queries) error {
		u, err := q.GetUserForUpdate(ctx, userID)
		if store.IsNoRows(err) {
			return ErrUserNotFound
		}
		if err != nil {
			return err
		}
		if u.Status != auth.StatusBanned {
			return ErrNotBanned
		}
		now := s.now().UTC()
		if err := q.SetUserStatus(ctx, db.SetUserStatusParams{ID: u.ID, Status: auth.StatusActive, UpdatedAt: now}); err != nil {
			return err
		}
		if _, err := q.LiftActiveBan(ctx, db.LiftActiveBanParams{
			UserID: u.ID, UnbannedBy: uuid.NullUUID{UUID: adminID, Valid: true}, UnbannedAt: &now, UnbanReason: why,
		}); err != nil {
			return fmt.Errorf("lift ban: %w", err)
		}
		return audit.Record(ctx, q, now, audit.Entry{
			Actor: adminID, Action: audit.ActionUserUnban, TargetType: audit.TargetUser, TargetID: u.ID.String(),
			Metadata: map[string]any{"reason": why},
		})
	})
}

// CloseAuction closes an auction that has not been settled yet. It becomes
// CLOSED at once (hidden, no more bids) and is never turned into an order.
func (s *Service) CloseAuction(ctx context.Context, adminID, auctionID uuid.UUID, why string) error {
	why, err := reason(why)
	if err != nil {
		return err
	}
	var batch notify.Batch
	err = s.store.InTx(ctx, func(q *db.Queries) error {
		a, err := q.GetAuctionForUpdate(ctx, auctionID)
		if store.IsNoRows(err) {
			return auction.ErrNotFound
		}
		if err != nil {
			return err
		}
		if a.ClosedAt != nil {
			return ErrAlreadyClosed
		}
		if a.SettledAt != nil {
			return ErrAlreadySettled
		}
		now := s.now().UTC()
		if err := q.CloseAuction(ctx, db.CloseAuctionParams{Now: now, ClosedBy: uuid.NullUUID{UUID: adminID, Valid: true}, CloseReason: why, ID: a.ID}); err != nil {
			return err
		}
		if err := audit.Record(ctx, q, now, audit.Entry{
			Actor: adminID, Action: audit.ActionAuctionClose, TargetType: audit.TargetAuction, TargetID: a.ID.String(),
			Metadata: map[string]any{"reason": why, "status_before": auction.StatusOf(a, now), "bid_count": a.BidCount},
		}); err != nil {
			return err
		}
		bidders, err := q.ListAuctionBidderIDs(ctx, a.ID)
		if err != nil {
			return err
		}
		recipients := append([]uuid.UUID{a.SellerID}, bidders...)
		for _, u := range recipients {
			if err := batch.Add(ctx, q, now, notify.Message{
				UserID: u, Type: notify.TypeAuctionClosed, AuctionID: a.ID, Title: "競標已被管理員關閉",
				Content: fmt.Sprintf("「%s」已被管理員關閉，原因：%s", a.Title, why),
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.dispatch.Dispatch(batch)
	return nil
}

// CancelOrder cancels an unpaid or disputed order.
func (s *Service) CancelOrder(ctx context.Context, adminID, orderID uuid.UUID, why string) error {
	why, err := reason(why)
	if err != nil {
		return err
	}
	var batch notify.Batch
	err = s.store.InTx(ctx, func(q *db.Queries) error {
		o, err := q.GetOrderForUpdate(ctx, orderID)
		if store.IsNoRows(err) {
			return order.ErrNotFound
		}
		if err != nil {
			return err
		}
		from := order.Status(o.Status)
		if !order.CanTransition(from, order.Cancelled) {
			return ErrNotCancellable
		}
		now := s.now().UTC()
		if err := order.Transition(ctx, q, now, o.ID, from, order.Cancelled, adminID, map[string]any{"reason": why, "by": "admin"}); err != nil {
			return err
		}
		for _, u := range []uuid.UUID{o.BuyerID, o.SellerID} {
			if err := batch.Add(ctx, q, now, notify.Message{
				UserID: u, Type: notify.TypeOrderStatus, OrderID: o.ID, Title: "訂單已被管理員取消", Content: "原因：" + why,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.dispatch.Dispatch(batch)
	return nil
}

// UpdateReport moves a report through PENDING -> REVIEWING -> RESOLVED /
// REJECTED. The reporter is notified of the final result.
func (s *Service) UpdateReport(ctx context.Context, adminID, reportID uuid.UUID, status, note string) error {
	switch status {
	case "REVIEWING", "RESOLVED", "REJECTED":
	default:
		return ErrInvalidReportOp
	}
	note, ok := textutil.Clean(note, 1000, true)
	if !ok {
		return apperr.Invalid("INVALID_NOTE", "處理說明不可超過 1000 字")
	}
	var batch notify.Batch
	err := s.store.InTx(ctx, func(q *db.Queries) error {
		r, err := q.GetReportForUpdate(ctx, reportID)
		if store.IsNoRows(err) {
			return ErrReportNotFound
		}
		if err != nil {
			return err
		}
		if r.Status == "RESOLVED" || r.Status == "REJECTED" {
			return apperr.Conflict("REPORT_CLOSED", "此檢舉已處理完畢")
		}
		now := s.now().UTC()
		if err := q.UpdateReportStatus(ctx, db.UpdateReportStatusParams{
			ID: r.ID, Status: status, ResolutionNote: note, HandledBy: uuid.NullUUID{UUID: adminID, Valid: true}, HandledAt: &now,
		}); err != nil {
			return err
		}
		if err := audit.Record(ctx, q, now, audit.Entry{
			Actor: adminID, Action: audit.ActionReportUpdate, TargetType: audit.TargetReport, TargetID: r.ID.String(),
			Metadata: map[string]any{"from": r.Status, "to": status, "note": note},
		}); err != nil {
			return err
		}
		if r.ReporterID.Valid && (status == "RESOLVED" || status == "REJECTED") {
			result := "已處理"
			if status == "REJECTED" {
				result = "未成立"
			}
			content := "您的檢舉" + result + "。"
			if note != "" {
				content += "說明：" + note
			}
			return batch.Add(ctx, q, now, notify.Message{
				UserID: r.ReporterID.UUID, Type: notify.TypeReportResult, Title: "檢舉處理結果", Content: content,
			})
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.dispatch.Dispatch(batch)
	return nil
}

// ReportDTO is a report as seen by admins.
type ReportDTO struct {
	report.DTO
	ReporterName       string  `json:"reporter_name"`
	TargetUserName     string  `json:"target_user_name"`
	TargetAuctionTitle string  `json:"target_auction_title"`
	OrderID            *string `json:"order_id"`
}
