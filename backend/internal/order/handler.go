package order

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"auction-platform/backend/internal/auth"
	"auction-platform/backend/internal/httpx"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes mounts order endpoints. auth.Service.LoadUser must run before.
func (h *Handler) Routes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireMember)
		r.Get("/orders", httpx.Handle(h.list))
		r.Get("/orders/{id}", httpx.Handle(h.get))
	})
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) error {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	items, err := h.svc.List(r.Context(), auth.UserFromContext(r.Context()).ID, r.URL.Query().Get("role"), page)
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
	return nil
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) error {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		return ErrNotFound
	}
	d, err := h.svc.Get(r.Context(), id, auth.UserFromContext(r.Context()).ID)
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"order": d})
	return nil
}
