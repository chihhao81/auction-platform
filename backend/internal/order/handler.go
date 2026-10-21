package order

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"auction-platform/backend/internal/apperr"
	"auction-platform/backend/internal/auth"
	"auction-platform/backend/internal/httpx"
)

type Handler struct {
	svc          *Service
	messageLimit *httpx.RateLimiter
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc, messageLimit: httpx.NewRateLimiter(1, 5)}
}

// Routes mounts order endpoints. auth.Service.LoadUser must run before.
func (h *Handler) Routes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireMember)
		r.Get("/orders", httpx.Handle(h.list))
		r.Get("/orders/{id}", httpx.Handle(h.get))
		r.Get("/orders/{id}/messages", httpx.Handle(h.messages))
		r.Post("/orders/{id}/messages", httpx.Handle(h.sendMessage))
		r.Put("/orders/{id}/shipping-info", httpx.Handle(h.setShippingInfo))
		r.Post("/orders/{id}/ship", httpx.Handle(h.ship))
		r.Post("/orders/{id}/receive", httpx.Handle(h.receive))
		r.Post("/orders/{id}/complete", httpx.Handle(h.complete))
	})
}

// OrderID parses the {id} route parameter.
func OrderID(r *http.Request) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		return uuid.Nil, ErrNotFound
	}
	return id, nil
}

func userID(r *http.Request) uuid.UUID { return auth.UserFromContext(r.Context()).ID }

func (h *Handler) list(w http.ResponseWriter, r *http.Request) error {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	items, err := h.svc.List(r.Context(), userID(r), r.URL.Query().Get("role"), page)
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
	return nil
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) error {
	id, err := OrderID(r)
	if err != nil {
		return err
	}
	d, err := h.svc.Get(r.Context(), id, userID(r))
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"order": d})
	return nil
}

func (h *Handler) messages(w http.ResponseWriter, r *http.Request) error {
	id, err := OrderID(r)
	if err != nil {
		return err
	}
	items, err := h.svc.Messages(r.Context(), userID(r), id)
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
	return nil
}

func (h *Handler) sendMessage(w http.ResponseWriter, r *http.Request) error {
	id, err := OrderID(r)
	if err != nil {
		return err
	}
	if !h.messageLimit.Allow(userID(r).String()) {
		return apperr.TooMany("RATE_LIMITED", "訊息傳送過於頻繁，請稍後再試")
	}
	var req struct {
		Content string `json:"content"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return err
	}
	m, err := h.svc.SendMessage(r.Context(), userID(r), id, req.Content)
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"message": m})
	return nil
}

func (h *Handler) setShippingInfo(w http.ResponseWriter, r *http.Request) error {
	id, err := OrderID(r)
	if err != nil {
		return err
	}
	var req ShippingInput
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return err
	}
	if err := h.svc.SetShippingInfo(r.Context(), userID(r), id, req); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) ship(w http.ResponseWriter, r *http.Request) error {
	id, err := OrderID(r)
	if err != nil {
		return err
	}
	var req ShipInput
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return err
	}
	if err := h.svc.Ship(r.Context(), userID(r), id, req); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) receive(w http.ResponseWriter, r *http.Request) error {
	id, err := OrderID(r)
	if err != nil {
		return err
	}
	if err := h.svc.ConfirmReceived(r.Context(), userID(r), id); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) complete(w http.ResponseWriter, r *http.Request) error {
	id, err := OrderID(r)
	if err != nil {
		return err
	}
	if err := h.svc.Complete(r.Context(), userID(r), id); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
