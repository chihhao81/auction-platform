// Package review implements two-way reviews and public user profiles.
package review

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
	"auction-platform/backend/internal/notify"
	"auction-platform/backend/internal/order"
	"auction-platform/backend/internal/store"
	"auction-platform/backend/internal/textutil"
)

const (
	Positive   = "POSITIVE"
	Negative   = "NEGATIVE"
	pageSize   = 20
	maxComment = 500
)

var (
	ErrNotCompleted    = apperr.Conflict("ORDER_NOT_COMPLETED", "訂單完成後才能評價")
	ErrAlreadyReviewed = apperr.Conflict("ALREADY_REVIEWED", "您已評價過此訂單")
	ErrUserNotFound    = apperr.NotFound("USER_NOT_FOUND", "找不到此使用者")
)

type Service struct {
	store    *store.Store
	dispatch notify.Dispatcher
	now      func() time.Time
}

func NewService(st *store.Store, d notify.Dispatcher, now func() time.Time) *Service {
	return &Service{store: st, dispatch: d, now: now}
}

// Create records reviewer's review of the other party of a completed order.
// Each party may review once per order (enforced by a unique constraint).
func (s *Service) Create(ctx context.Context, reviewer, orderID uuid.UUID, rating, comment string) error {
	if rating != Positive && rating != Negative {
		return apperr.Invalid("INVALID_RATING", "評價只能為正評或負評")
	}
	comment, ok := textutil.Clean(comment, maxComment, true)
	if !ok {
		return apperr.Invalid("INVALID_COMMENT", fmt.Sprintf("留言不可超過 %d 字", maxComment))
	}
	var batch notify.Batch
	err := s.store.InTx(ctx, func(q *db.Queries) error {
		o, err := q.GetOrderForUpdate(ctx, orderID)
		if store.IsNoRows(err) {
			return order.ErrNotFound
		}
		if err != nil {
			return err
		}
		var reviewee uuid.UUID
		switch reviewer {
		case o.BuyerID:
			reviewee = o.SellerID
		case o.SellerID:
			reviewee = o.BuyerID
		default:
			return order.ErrNotFound
		}
		if order.Status(o.Status) != order.Completed {
			return ErrNotCompleted
		}
		now := s.now().UTC()
		r, err := q.InsertReview(ctx, db.InsertReviewParams{
			OrderID: o.ID, ReviewerID: reviewer, RevieweeID: reviewee, Rating: rating, Comment: comment, CreatedAt: now,
		})
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrAlreadyReviewed
		}
		if err != nil {
			return fmt.Errorf("insert review: %w", err)
		}
		if err := audit.Record(ctx, q, now, audit.Entry{
			Actor: reviewer, Action: audit.ActionReviewCreate, TargetType: audit.TargetOrder, TargetID: o.ID.String(),
			Metadata: map[string]any{"review_id": r.ID.String(), "reviewee": reviewee.String(), "rating": rating},
		}); err != nil {
			return err
		}
		label := "正評"
		if rating == Negative {
			label = "負評"
		}
		return batch.Add(ctx, q, now, notify.Message{
			UserID: reviewee, Type: notify.TypeReviewReceived, OrderID: o.ID,
			Title: "您收到一則" + label, Content: previewOr(comment, "對方沒有留言。"),
		})
	})
	if err != nil {
		return err
	}
	s.dispatch.Dispatch(batch)
	return nil
}

func previewOr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	if r := []rune(s); len(r) > 60 {
		return string(r[:60]) + "…"
	}
	return s
}

type Reputation struct {
	Positive        int64 `json:"positive"`
	Negative        int64 `json:"negative"`
	CompletedOrders int64 `json:"completed_orders"`
}

type ReviewDTO struct {
	ID           string    `json:"id"`
	Rating       string    `json:"rating"`
	Comment      string    `json:"comment"`
	ReviewerName string    `json:"reviewer_name"`
	FromBuyer    bool      `json:"from_buyer"`
	CreatedAt    time.Time `json:"created_at"`
}

type ProfileDTO struct {
	ID          string      `json:"id"`
	DisplayName string      `json:"display_name"`
	AvatarURL   string      `json:"avatar_url"`
	Banned      bool        `json:"banned"`
	CreatedAt   time.Time   `json:"created_at"`
	Reputation  Reputation  `json:"reputation"`
	Reviews     []ReviewDTO `json:"reviews"`
}

func (s *Service) reviews(ctx context.Context, user uuid.UUID, page int) ([]ReviewDTO, error) {
	if page < 1 || page > 1000 {
		page = 1
	}
	rows, err := s.store.ListReviewsForUser(ctx, db.ListReviewsForUserParams{RevieweeID: user, Lim: pageSize, Off: int32((page - 1) * pageSize)})
	if err != nil {
		return nil, fmt.Errorf("list reviews: %w", err)
	}
	out := make([]ReviewDTO, len(rows))
	for i, r := range rows {
		out[i] = ReviewDTO{ID: r.ID.String(), Rating: r.Rating, Comment: r.Comment, ReviewerName: r.ReviewerName, FromBuyer: r.FromBuyer, CreatedAt: r.CreatedAt.UTC()}
	}
	return out, nil
}

// Reputation returns a user's review counts and completed transactions.
func (s *Service) Reputation(ctx context.Context, user uuid.UUID) (Reputation, error) {
	r, err := s.store.UserReputation(ctx, user)
	if err != nil {
		return Reputation{}, fmt.Errorf("reputation: %w", err)
	}
	return Reputation{Positive: r.Positive, Negative: r.Negative, CompletedOrders: r.CompletedOrders}, nil
}

// Profile is a user's public page. It never includes the LINE ID or any
// contact data.
func (s *Service) Profile(ctx context.Context, user uuid.UUID) (ProfileDTO, error) {
	u, err := s.store.GetUserByID(ctx, user)
	if store.IsNoRows(err) {
		return ProfileDTO{}, ErrUserNotFound
	}
	if err != nil {
		return ProfileDTO{}, err
	}
	rep, err := s.Reputation(ctx, user)
	if err != nil {
		return ProfileDTO{}, err
	}
	revs, err := s.reviews(ctx, user, 1)
	if err != nil {
		return ProfileDTO{}, err
	}
	return ProfileDTO{
		ID: u.ID.String(), DisplayName: u.DisplayName, AvatarURL: u.AvatarUrl, Banned: u.Status == auth.StatusBanned,
		CreatedAt: u.CreatedAt.UTC(), Reputation: rep, Reviews: revs,
	}, nil
}

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes mounts review endpoints. auth.Service.LoadUser must run before.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/users/{id}", httpx.Handle(h.profile))
	r.Get("/users/{id}/reviews", httpx.Handle(h.list))
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireMember)
		r.Post("/orders/{id}/reviews", httpx.Handle(h.create))
	})
}

func userParam(r *http.Request) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		return uuid.Nil, ErrUserNotFound
	}
	return id, nil
}

func (h *Handler) profile(w http.ResponseWriter, r *http.Request) error {
	id, err := userParam(r)
	if err != nil {
		return err
	}
	p, err := h.svc.Profile(r.Context(), id)
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"user": p})
	return nil
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) error {
	id, err := userParam(r)
	if err != nil {
		return err
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	items, err := h.svc.reviews(r.Context(), id, page)
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items, "has_more": len(items) == pageSize})
	return nil
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) error {
	id, err := order.OrderID(r)
	if err != nil {
		return err
	}
	var req struct {
		Rating  string `json:"rating"`
		Comment string `json:"comment"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return err
	}
	if err := h.svc.Create(r.Context(), auth.UserFromContext(r.Context()).ID, id, req.Rating, req.Comment); err != nil {
		return err
	}
	w.WriteHeader(http.StatusCreated)
	return nil
}
