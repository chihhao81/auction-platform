package payment

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"auction-platform/backend/internal/auth"
	"auction-platform/backend/internal/ecpay"
	"auction-platform/backend/internal/httpx"
	"auction-platform/backend/internal/order"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes mounts payment endpoints under /api. auth.Service.LoadUser must run
// before the member routes; the ECPay callback needs no session.
func (h *Handler) Routes(r chi.Router) {
	r.Post("/payments/ecpay/notify", h.notify)
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireMember)
		r.Post("/orders/{id}/payments", httpx.Handle(h.checkout))
		r.Post("/orders/{id}/payments/sync", httpx.Handle(h.sync))
	})
}

func (h *Handler) checkout(w http.ResponseWriter, r *http.Request) error {
	id, err := order.OrderID(r)
	if err != nil {
		return err
	}
	co, err := h.svc.CreateCheckout(r.Context(), auth.UserFromContext(r.Context()).ID, id)
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusCreated, co)
	return nil
}

func (h *Handler) sync(w http.ResponseWriter, r *http.Request) error {
	id, err := order.OrderID(r)
	if err != nil {
		return err
	}
	if err := h.svc.Sync(r.Context(), auth.UserFromContext(r.Context()).ID, id); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// notify is ECPay's ReturnURL. It answers in ECPay's plain-text protocol.
func (h *Handler) notify(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("0|Bad Request"))
		return
	}
	body, ok := h.svc.HandleCallback(r.Context(), ecpay.FormToMap(r.PostForm))
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
	}
	_, _ = w.Write([]byte(body))
}
