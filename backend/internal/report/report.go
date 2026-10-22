// Package report lets users report auctions and other users.
package report

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"auction-platform/backend/internal/apperr"
	"auction-platform/backend/internal/audit"
	"auction-platform/backend/internal/auth"
	"auction-platform/backend/internal/database/db"
	"auction-platform/backend/internal/httpx"
	"auction-platform/backend/internal/store"
	"auction-platform/backend/internal/textutil"
)

const (
	TargetUser    = "USER"
	TargetAuction = "AUCTION"
	pageSize      = 20
)

// Reasons a user can pick, per target type.
var Reasons = map[string]map[string]string{
	TargetAuction: {
		"PROHIBITED_ITEM": "違禁或違反動保規定的商品",
		"MISLEADING":      "商品描述不實",
		"FRAUD":           "疑似詐騙",
		"OTHER":           "其他",
	},
	TargetUser: {
		"FRAUD":       "疑似詐騙",
		"NON_PAYMENT": "得標不付款",
		"NO_SHIPMENT": "收款不出貨",
		"HARASSMENT":  "騷擾或不當言論",
		"SHILL":       "疑似哄抬價格",
		"OTHER":       "其他",
	},
}

var (
	ErrTargetNotFound = apperr.NotFound("REPORT_TARGET_NOT_FOUND", "找不到檢舉對象")
	ErrSelf           = apperr.Invalid("CANNOT_REPORT_SELF", "不能檢舉自己或自己的競標")
	ErrDuplicate      = apperr.Conflict("REPORT_ALREADY_OPEN", "您對此對象的檢舉仍在處理中")
)

type Service struct {
	store *store.Store
	now   func() time.Time
}

func NewService(st *store.Store, now func() time.Time) *Service { return &Service{store: st, now: now} }

type Input struct {
	TargetType  string    `json:"target_type"`
	TargetID    uuid.UUID `json:"target_id"`
	Reason      string    `json:"reason"`
	Description string    `json:"description"`
}

// Create files a report by reporter.
func (s *Service) Create(ctx context.Context, reporter uuid.UUID, in Input) (db.Report, error) {
	reasons, ok := Reasons[in.TargetType]
	if !ok {
		return db.Report{}, apperr.Invalid("INVALID_TARGET_TYPE", "檢舉對象類型錯誤")
	}
	if _, ok := reasons[in.Reason]; !ok {
		return db.Report{}, apperr.Invalid("INVALID_REASON", "請選擇檢舉原因")
	}
	desc, ok := textutil.Clean(in.Description, 2000, true)
	if !ok {
		return db.Report{}, apperr.Invalid("INVALID_DESCRIPTION", "說明不可超過 2000 字")
	}
	params := db.InsertUserReportParams{
		ReporterID: uuid.NullUUID{UUID: reporter, Valid: true}, TargetType: in.TargetType,
		Reason: in.Reason, Description: desc, CreatedAt: s.now().UTC(),
	}
	switch in.TargetType {
	case TargetUser:
		if in.TargetID == reporter {
			return db.Report{}, ErrSelf
		}
		if _, err := s.store.GetUserByID(ctx, in.TargetID); err != nil {
			if store.IsNoRows(err) {
				return db.Report{}, ErrTargetNotFound
			}
			return db.Report{}, err
		}
		params.TargetUserID = uuid.NullUUID{UUID: in.TargetID, Valid: true}
	case TargetAuction:
		a, err := s.store.GetAuction(ctx, in.TargetID)
		if err != nil {
			if store.IsNoRows(err) {
				return db.Report{}, ErrTargetNotFound
			}
			return db.Report{}, err
		}
		if a.SellerID == reporter {
			return db.Report{}, ErrSelf
		}
		params.TargetAuctionID = uuid.NullUUID{UUID: a.ID, Valid: true}
		// Also link the seller so admins see who is responsible.
		params.TargetUserID = uuid.NullUUID{UUID: a.SellerID, Valid: true}
	}
	var out db.Report
	err := s.store.InTx(ctx, func(q *db.Queries) error {
		r, err := q.InsertUserReport(ctx, params)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrDuplicate
		}
		if err != nil {
			return fmt.Errorf("insert report: %w", err)
		}
		out = r
		return audit.Record(ctx, q, params.CreatedAt, audit.Entry{
			Actor: reporter, Action: audit.ActionReportCreate, TargetType: audit.TargetReport, TargetID: r.ID.String(),
			Metadata: map[string]any{"target_type": in.TargetType, "target_id": in.TargetID.String(), "reason": in.Reason},
		})
	})
	return out, err
}

// DTO is a report as shown to its reporter.
type DTO struct {
	ID              string     `json:"id"`
	TargetType      string     `json:"target_type"`
	TargetUserID    *string    `json:"target_user_id"`
	TargetAuctionID *string    `json:"target_auction_id"`
	Reason          string     `json:"reason"`
	ReasonLabel     string     `json:"reason_label"`
	Description     string     `json:"description"`
	Status          string     `json:"status"`
	ResolutionNote  string     `json:"resolution_note"`
	HandledAt       *time.Time `json:"handled_at"`
	CreatedAt       time.Time  `json:"created_at"`
}

func idPtr(u uuid.NullUUID) *string {
	if !u.Valid {
		return nil
	}
	s := u.UUID.String()
	return &s
}

// ToDTO converts a report row.
func ToDTO(r db.Report) DTO {
	label := Reasons[r.TargetType][r.Reason]
	if label == "" {
		label = r.Reason
	}
	return DTO{
		ID: r.ID.String(), TargetType: r.TargetType, TargetUserID: idPtr(r.TargetUserID), TargetAuctionID: idPtr(r.TargetAuctionID),
		Reason: r.Reason, ReasonLabel: label, Description: r.Description, Status: r.Status,
		ResolutionNote: r.ResolutionNote, HandledAt: r.HandledAt, CreatedAt: r.CreatedAt.UTC(),
	}
}

type Handler struct {
	svc   *Service
	limit *httpx.RateLimiter
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc, limit: httpx.NewRateLimiter(0.1, 5)} // ~6 per minute
}

func (h *Handler) Routes(r chi.Router) {
	r.Get("/reports/reasons", httpx.Handle(h.reasons))
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireMember)
		r.Post("/reports", httpx.Handle(h.create))
		r.Get("/me/reports", httpx.Handle(h.mine))
	})
}

func (h *Handler) reasons(w http.ResponseWriter, r *http.Request) error {
	httpx.WriteJSON(w, http.StatusOK, Reasons)
	return nil
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) error {
	u := auth.UserFromContext(r.Context())
	if !h.limit.Allow(u.ID.String()) {
		return apperr.TooMany("RATE_LIMITED", "檢舉過於頻繁，請稍後再試")
	}
	var in Input
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		return err
	}
	rep, err := h.svc.Create(r.Context(), u.ID, in)
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"report": ToDTO(rep)})
	return nil
}

func (h *Handler) mine(w http.ResponseWriter, r *http.Request) error {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 || page > 1000 {
		page = 1
	}
	rows, err := h.svc.store.ListReportsByReporter(r.Context(), db.ListReportsByReporterParams{
		ReporterID: uuid.NullUUID{UUID: auth.UserFromContext(r.Context()).ID, Valid: true}, Lim: pageSize, Off: int32((page - 1) * pageSize),
	})
	if err != nil {
		return err
	}
	items := make([]DTO, len(rows))
	for i, row := range rows {
		items[i] = ToDTO(row)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
	return nil
}
