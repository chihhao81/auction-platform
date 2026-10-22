package admin

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"auction-platform/backend/internal/apperr"
	"auction-platform/backend/internal/auction"
	"auction-platform/backend/internal/auth"
	"auction-platform/backend/internal/database/db"
	"auction-platform/backend/internal/httpx"
	"auction-platform/backend/internal/order"
	"auction-platform/backend/internal/report"
	"auction-platform/backend/internal/review"
	"auction-platform/backend/internal/store"
)

type Handler struct {
	svc     *Service
	reviews *review.Service
}

func NewHandler(svc *Service, reviews *review.Service) *Handler {
	return &Handler{svc: svc, reviews: reviews}
}

// Routes mounts /admin endpoints. auth.Service.LoadUser must run before.
func (h *Handler) Routes(r chi.Router) {
	r.Route("/admin", func(r chi.Router) {
		r.Use(auth.RequireAdmin)
		r.Get("/users", httpx.Handle(h.users))
		r.Get("/users/{id}", httpx.Handle(h.user))
		r.Post("/users/{id}/ban", httpx.Handle(h.ban))
		r.Post("/users/{id}/unban", httpx.Handle(h.unban))
		r.Get("/auctions", httpx.Handle(h.auctions))
		r.Post("/auctions/{id}/close", httpx.Handle(h.closeAuction))
		r.Get("/orders", httpx.Handle(h.orders))
		r.Post("/orders/{id}/cancel", httpx.Handle(h.cancelOrder))
		r.Get("/reports", httpx.Handle(h.reports))
		r.Post("/reports/{id}", httpx.Handle(h.updateReport))
		r.Get("/payments/attention", httpx.Handle(h.payments))
		r.Get("/audit-logs", httpx.Handle(h.auditLogs))
	})
}

func adminID(r *http.Request) uuid.UUID { return auth.UserFromContext(r.Context()).ID }

func page(r *http.Request) (lim, off int32) {
	p, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if p < 1 || p > 1000 {
		p = 1
	}
	return pageSize, int32((p - 1) * pageSize)
}

func idParam(r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	return id, err == nil
}

type reasonBody struct {
	Reason string `json:"reason"`
}

type UserDTO struct {
	ID              string     `json:"id"`
	DisplayName     string     `json:"display_name"`
	AvatarURL       string     `json:"avatar_url"`
	Role            string     `json:"role"`
	Status          string     `json:"status"`
	CreatedAt       time.Time  `json:"created_at"`
	ActiveBanAt     *time.Time `json:"active_ban_at"`
	ActiveBanReason *string    `json:"active_ban_reason"`
}

func (h *Handler) users(w http.ResponseWriter, r *http.Request) error {
	lim, off := page(r)
	q := r.URL.Query()
	rows, err := h.svc.store.AdminListUsers(r.Context(), db.AdminListUsersParams{
		Pattern: auction.LikePattern(q.Get("q")), Status: q.Get("status"), Lim: lim, Off: off,
	})
	if err != nil {
		return err
	}
	items := make([]UserDTO, len(rows))
	for i, u := range rows {
		items[i] = UserDTO{u.ID.String(), u.DisplayName, u.AvatarUrl, u.Role, u.Status, u.CreatedAt.UTC(), u.ActiveBanAt, u.ActiveBanReason}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
	return nil
}

type BanDTO struct {
	ID             string     `json:"id"`
	Reason         string     `json:"reason"`
	BannedAt       time.Time  `json:"banned_at"`
	BannedByName   string     `json:"banned_by_name"`
	UnbannedAt     *time.Time `json:"unbanned_at"`
	UnbannedByName string     `json:"unbanned_by_name"`
	UnbanReason    string     `json:"unban_reason"`
}

func (h *Handler) user(w http.ResponseWriter, r *http.Request) error {
	id, ok := idParam(r)
	if !ok {
		return ErrUserNotFound
	}
	u, err := h.svc.store.GetUserByID(r.Context(), id)
	if err != nil {
		if store.IsNoRows(err) {
			return ErrUserNotFound
		}
		return err
	}
	bans, err := h.svc.store.ListUserBans(r.Context(), id)
	if err != nil {
		return err
	}
	rep, err := h.reviews.Reputation(r.Context(), id)
	if err != nil {
		return err
	}
	history := make([]BanDTO, len(bans))
	for i, b := range bans {
		history[i] = BanDTO{b.ID.String(), b.Reason, b.BannedAt.UTC(), b.BannedByName, b.UnbannedAt, b.UnbannedByName, b.UnbanReason}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"user": UserDTO{ID: u.ID.String(), DisplayName: u.DisplayName, AvatarURL: u.AvatarUrl, Role: u.Role, Status: u.Status, CreatedAt: u.CreatedAt.UTC()},
		"bans": history, "reputation": rep,
	})
	return nil
}

func (h *Handler) withReason(w http.ResponseWriter, r *http.Request, notFound error, fn func(id uuid.UUID, reason string) error) error {
	id, ok := idParam(r)
	if !ok {
		return notFound
	}
	var body reasonBody
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		return err
	}
	if err := fn(id, body.Reason); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) ban(w http.ResponseWriter, r *http.Request) error {
	return h.withReason(w, r, ErrUserNotFound, func(id uuid.UUID, why string) error { return h.svc.Ban(r.Context(), adminID(r), id, why) })
}

func (h *Handler) unban(w http.ResponseWriter, r *http.Request) error {
	return h.withReason(w, r, ErrUserNotFound, func(id uuid.UUID, why string) error { return h.svc.Unban(r.Context(), adminID(r), id, why) })
}

func (h *Handler) closeAuction(w http.ResponseWriter, r *http.Request) error {
	return h.withReason(w, r, auction.ErrNotFound, func(id uuid.UUID, why string) error {
		return h.svc.CloseAuction(r.Context(), adminID(r), id, why)
	})
}

func (h *Handler) cancelOrder(w http.ResponseWriter, r *http.Request) error {
	return h.withReason(w, r, order.ErrNotFound, func(id uuid.UUID, why string) error {
		return h.svc.CancelOrder(r.Context(), adminID(r), id, why)
	})
}

type AuctionDTO struct {
	ID           string         `json:"id"`
	Title        string         `json:"title"`
	SellerID     string         `json:"seller_id"`
	SellerName   string         `json:"seller_name"`
	Status       auction.Status `json:"status"`
	CurrentPrice int64          `json:"current_price"`
	BidCount     int32          `json:"bid_count"`
	StartTime    time.Time      `json:"start_time"`
	EndTime      time.Time      `json:"end_time"`
	Settled      bool           `json:"settled"`
	ClosedAt     *time.Time     `json:"closed_at"`
	CloseReason  string         `json:"close_reason"`
}

func (h *Handler) auctions(w http.ResponseWriter, r *http.Request) error {
	lim, off := page(r)
	rows, err := h.svc.store.AdminListAuctions(r.Context(), db.AdminListAuctionsParams{Pattern: auction.LikePattern(r.URL.Query().Get("q")), Lim: lim, Off: off})
	if err != nil {
		return err
	}
	now := h.svc.now().UTC()
	items := make([]AuctionDTO, len(rows))
	for i, row := range rows {
		a := row.Auction
		items[i] = AuctionDTO{
			ID: a.ID.String(), Title: a.Title, SellerID: a.SellerID.String(), SellerName: row.SellerName,
			Status: auction.StatusOf(a, now), CurrentPrice: a.CurrentPrice, BidCount: a.BidCount,
			StartTime: a.StartTime.UTC(), EndTime: a.EndTime.UTC(), Settled: a.SettledAt != nil,
			ClosedAt: a.ClosedAt, CloseReason: a.CloseReason,
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
	return nil
}

type OrderDTO struct {
	ID          string     `json:"id"`
	Status      string     `json:"status"`
	BuyerID     string     `json:"buyer_id"`
	BuyerName   string     `json:"buyer_name"`
	SellerID    string     `json:"seller_id"`
	SellerName  string     `json:"seller_name"`
	TotalAmount int64      `json:"total_amount"`
	Overdue     bool       `json:"overdue"`
	PaidAt      *time.Time `json:"paid_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

func (h *Handler) orders(w http.ResponseWriter, r *http.Request) error {
	lim, off := page(r)
	rows, err := h.svc.store.AdminListOrders(r.Context(), db.AdminListOrdersParams{Status: r.URL.Query().Get("status"), Lim: lim, Off: off})
	if err != nil {
		return err
	}
	items := make([]OrderDTO, len(rows))
	for i, o := range rows {
		items[i] = OrderDTO{o.ID.String(), o.Status, o.BuyerID.String(), o.BuyerName, o.SellerID.String(), o.SellerName,
			o.TotalAmount, o.OverdueAt != nil, o.PaidAt, o.CreatedAt.UTC()}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
	return nil
}

func (h *Handler) reports(w http.ResponseWriter, r *http.Request) error {
	lim, off := page(r)
	rows, err := h.svc.store.AdminListReports(r.Context(), db.AdminListReportsParams{Status: r.URL.Query().Get("status"), Lim: lim, Off: off})
	if err != nil {
		return err
	}
	items := make([]ReportDTO, len(rows))
	for i, row := range rows {
		rep := db.Report{
			ID: row.ID, ReporterID: row.ReporterID, TargetType: row.TargetType, TargetUserID: row.TargetUserID,
			TargetAuctionID: row.TargetAuctionID, OrderID: row.OrderID, Reason: row.Reason, Description: row.Description,
			Status: row.Status, ResolutionNote: row.ResolutionNote, HandledBy: row.HandledBy, HandledAt: row.HandledAt,
			CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		}
		items[i] = ReportDTO{DTO: report.ToDTO(rep), ReporterName: row.ReporterName, TargetUserName: row.TargetUserName,
			TargetAuctionTitle: row.TargetAuctionTitle, OrderID: idPtr(row.OrderID)}
		if !row.ReporterID.Valid {
			items[i].ReporterName = "（系統）"
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
	return nil
}

func idPtr(u uuid.NullUUID) *string {
	if !u.Valid {
		return nil
	}
	s := u.UUID.String()
	return &s
}

func (h *Handler) updateReport(w http.ResponseWriter, r *http.Request) error {
	id, ok := idParam(r)
	if !ok {
		return ErrReportNotFound
	}
	var body struct {
		Status string `json:"status"`
		Note   string `json:"note"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		return err
	}
	if err := h.svc.UpdateReport(r.Context(), adminID(r), id, body.Status, body.Note); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

type PaymentDTO struct {
	ID              string    `json:"id"`
	OrderID         string    `json:"order_id"`
	OrderStatus     string    `json:"order_status"`
	MerchantTradeNo string    `json:"merchant_trade_no"`
	ProviderTradeNo *string   `json:"provider_trade_no"`
	Amount          int64     `json:"amount"`
	Status          string    `json:"status"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (h *Handler) payments(w http.ResponseWriter, r *http.Request) error {
	rows, err := h.svc.store.ListPaymentsNeedingAttention(r.Context())
	if err != nil {
		return err
	}
	items := make([]PaymentDTO, len(rows))
	for i, p := range rows {
		items[i] = PaymentDTO{p.ID.String(), p.OrderID.String(), p.OrderStatus, p.MerchantTradeNo, p.ProviderTradeNo, p.Amount, p.Status, p.UpdatedAt.UTC()}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
	return nil
}

type AuditDTO struct {
	ID         string          `json:"id"`
	ActorID    *string         `json:"actor_id"`
	ActorName  string          `json:"actor_name"`
	Action     string          `json:"action"`
	TargetType string          `json:"target_type"`
	TargetID   string          `json:"target_id"`
	Metadata   json.RawMessage `json:"metadata"`
	IP         string          `json:"ip"`
	UserAgent  string          `json:"user_agent"`
	CreatedAt  time.Time       `json:"created_at"`
}

// auditLogs lets admins search the append-only audit log.
func (h *Handler) auditLogs(w http.ResponseWriter, r *http.Request) error {
	lim, off := page(r)
	q := r.URL.Query()
	params := db.AdminListAuditLogsParams{
		Action: q.Get("action"), TargetType: q.Get("target_type"), TargetID: q.Get("target_id"), Lim: lim, Off: off,
	}
	if a := q.Get("actor"); a != "" {
		id, err := uuid.Parse(a)
		if err != nil {
			return apperr.Invalid("INVALID_ACTOR", "操作者 ID 格式錯誤")
		}
		params.ActorUserID = uuid.NullUUID{UUID: id, Valid: true}
	}
	rows, err := h.svc.store.AdminListAuditLogs(r.Context(), params)
	if err != nil {
		return err
	}
	items := make([]AuditDTO, len(rows))
	for i, a := range rows {
		items[i] = AuditDTO{
			ID: a.ID.String(), ActorID: idPtr(a.ActorUserID), ActorName: a.ActorName, Action: a.Action,
			TargetType: a.TargetType, TargetID: a.TargetID, Metadata: json.RawMessage(a.Metadata),
			IP: a.Ip, UserAgent: a.UserAgent, CreatedAt: a.CreatedAt.UTC(),
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
	return nil
}
