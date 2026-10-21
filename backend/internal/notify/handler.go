package notify

import (
	"encoding/base64"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"auction-platform/backend/internal/apperr"
	"auction-platform/backend/internal/auth"
	"auction-platform/backend/internal/database/db"
	"auction-platform/backend/internal/httpx"
	"auction-platform/backend/internal/store"
)

const pageSize = 30

type Handler struct {
	store          *store.Store
	vapidPublicKey string // empty when push is not configured
	now            func() time.Time
}

func NewHandler(st *store.Store, vapidPublicKey string, now func() time.Time) *Handler {
	return &Handler{store: st, vapidPublicKey: vapidPublicKey, now: now}
}

// Routes mounts notification endpoints. auth.Service.LoadUser must run before.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/push/public-key", httpx.Handle(h.publicKey))
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireMember)
		r.Get("/notifications", httpx.Handle(h.list))
		r.Get("/notifications/unread-count", httpx.Handle(h.unreadCount))
		r.Post("/notifications/{id}/read", httpx.Handle(h.markRead))
		r.Post("/notifications/read-all", httpx.Handle(h.markAllRead))
		r.Get("/notifications/preferences", httpx.Handle(h.preferences))
		r.Put("/notifications/preferences", httpx.Handle(h.updatePreferences))
		r.Post("/push/subscriptions", httpx.Handle(h.subscribe))
		r.Delete("/push/subscriptions", httpx.Handle(h.unsubscribe))
	})
}

type DTO struct {
	ID        string     `json:"id"`
	Type      string     `json:"type"`
	Title     string     `json:"title"`
	Content   string     `json:"content"`
	AuctionID *string    `json:"related_auction_id"`
	OrderID   *string    `json:"related_order_id"`
	Link      string     `json:"link"`
	ReadAt    *time.Time `json:"read_at"`
	CreatedAt time.Time  `json:"created_at"`
}

func toDTO(n db.Notification) DTO {
	d := DTO{
		ID: n.ID.String(), Type: n.Type, Title: n.Title, Content: n.Content,
		Link: Link(n), ReadAt: n.ReadAt, CreatedAt: n.CreatedAt.UTC(),
	}
	if n.RelatedAuctionID.Valid {
		s := n.RelatedAuctionID.UUID.String()
		d.AuctionID = &s
	}
	if n.RelatedOrderID.Valid {
		s := n.RelatedOrderID.UUID.String()
		d.OrderID = &s
	}
	return d
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) error {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 || page > 1000 {
		page = 1
	}
	u := auth.UserFromContext(r.Context())
	rows, err := h.store.ListNotifications(r.Context(), db.ListNotificationsParams{UserID: u.ID, Lim: pageSize, Off: int32((page - 1) * pageSize)})
	if err != nil {
		return err
	}
	items := make([]DTO, len(rows))
	for i, n := range rows {
		items[i] = toDTO(n)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items, "has_more": len(rows) == pageSize})
	return nil
}

func (h *Handler) unreadCount(w http.ResponseWriter, r *http.Request) error {
	n, err := h.store.CountUnreadNotifications(r.Context(), auth.UserFromContext(r.Context()).ID)
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]int64{"count": n})
	return nil
}

func (h *Handler) markRead(w http.ResponseWriter, r *http.Request) error {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		return apperr.NotFound("NOTIFICATION_NOT_FOUND", "找不到通知")
	}
	// Scoped by user_id, so another user's notification is never touched.
	if err := h.store.MarkNotificationRead(r.Context(), db.MarkNotificationReadParams{
		Now: h.now().UTC(), ID: id, UserID: auth.UserFromContext(r.Context()).ID,
	}); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) markAllRead(w http.ResponseWriter, r *http.Request) error {
	if err := h.store.MarkAllNotificationsRead(r.Context(), db.MarkAllNotificationsReadParams{
		Now: h.now().UTC(), UserID: auth.UserFromContext(r.Context()).ID,
	}); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

type preference struct {
	TypeInfo
	PushEnabled bool `json:"push_enabled"`
}

func (h *Handler) preferences(w http.ResponseWriter, r *http.Request) error {
	off, err := h.store.ListPushOptOuts(r.Context(), auth.UserFromContext(r.Context()).ID)
	if err != nil {
		return err
	}
	disabled := map[string]bool{}
	for _, t := range off {
		disabled[t] = true
	}
	out := make([]preference, len(Types))
	for i, t := range Types {
		out[i] = preference{TypeInfo: t, PushEnabled: !disabled[string(t.Type)]}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": out})
	return nil
}

func (h *Handler) updatePreferences(w http.ResponseWriter, r *http.Request) error {
	var req map[Type]bool
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return err
	}
	for t := range req {
		if !ValidType(t) {
			return apperr.Invalid("INVALID_NOTIFICATION_TYPE", "未知的通知類型")
		}
	}
	u := auth.UserFromContext(r.Context())
	now := h.now().UTC()
	err := h.store.InTx(r.Context(), func(q *db.Queries) error {
		for t, enabled := range req {
			if err := q.UpsertPushPreference(r.Context(), db.UpsertPushPreferenceParams{
				UserID: u.ID, Type: string(t), PushEnabled: enabled, UpdatedAt: now,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return h.preferences(w, r)
}

func (h *Handler) publicKey(w http.ResponseWriter, r *http.Request) error {
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"public_key": h.vapidPublicKey, "enabled": h.vapidPublicKey != ""})
	return nil
}

type subscriptionRequest struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

// validKey checks a base64url (unpadded or padded) key decodes to n bytes.
func validKey(s string, n int) bool {
	b, err := base64.RawURLEncoding.DecodeString(trimPad(s))
	return err == nil && len(b) == n
}

func trimPad(s string) string {
	for len(s) > 0 && s[len(s)-1] == '=' {
		s = s[:len(s)-1]
	}
	return s
}

func (h *Handler) subscribe(w http.ResponseWriter, r *http.Request) error {
	if h.vapidPublicKey == "" {
		return apperr.Conflict("PUSH_DISABLED", "伺服器未啟用推播")
	}
	var req subscriptionRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return err
	}
	if !ValidEndpoint(req.Endpoint) || !validKey(req.Keys.P256dh, 65) || !validKey(req.Keys.Auth, 16) {
		return apperr.Invalid("INVALID_SUBSCRIPTION", "推播訂閱資料無效")
	}
	u := auth.UserFromContext(r.Context())
	if err := h.store.UpsertPushSubscription(r.Context(), db.UpsertPushSubscriptionParams{
		UserID: u.ID, Endpoint: req.Endpoint, P256dh: req.Keys.P256dh, Auth: req.Keys.Auth,
		UserAgent: httpx.MetaFromContext(r.Context()).UserAgent, CreatedAt: h.now().UTC(),
	}); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) unsubscribe(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Endpoint string `json:"endpoint"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return err
	}
	if err := h.store.DeleteUserPushSubscription(r.Context(), db.DeleteUserPushSubscriptionParams{
		UserID: auth.UserFromContext(r.Context()).ID, Endpoint: req.Endpoint,
	}); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
