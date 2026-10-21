// Package server wires handlers and middleware into the HTTP router.
package server

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"auction-platform/backend/internal/auction"
	"auction-platform/backend/internal/auth"
	"auction-platform/backend/internal/config"
	"auction-platform/backend/internal/httpx"
	"auction-platform/backend/internal/notify"
	"auction-platform/backend/internal/order"
	"auction-platform/backend/internal/storage"
	"auction-platform/backend/internal/store"
)

type Deps struct {
	Config  *config.Config
	Store   *store.Store
	Auth    *auth.Service
	Line    auth.LineClient
	Auction *auction.Service
	Order   *order.Service
	// Uploads serves locally stored images; nil when using remote storage.
	Uploads *storage.Local
	Now     func() time.Time
}

func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(httpx.SecurityHeaders)
	r.Use(httpx.ClientMeta(d.Config.ClientIPHeader))
	r.Use(httpx.RateLimitByIP(httpx.NewRateLimiter(10, 40)))
	r.Use(httpx.CSRF(d.Config.CookieSecure))

	r.Route("/api", func(r chi.Router) {
		r.Get("/health", httpx.Handle(func(w http.ResponseWriter, r *http.Request) error {
			ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
			defer cancel()
			if err := d.Store.Ping(ctx); err != nil {
				return err
			}
			httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
			return nil
		}))

		r.Group(func(r chi.Router) {
			r.Use(d.Auth.LoadUser)
			auth.NewHandler(d.Auth, d.Line, d.Config.FrontendURL, d.Config.CookieSecure, d.Config.MockLineAuth).Routes(r)
			auction.NewHandler(d.Auction).Routes(r)
			order.NewHandler(d.Order).Routes(r)
			notify.NewHandler(d.Store, d.Config.VAPIDPublicKey, d.Now).Routes(r)
		})
	})

	if d.Uploads != nil {
		r.Get(d.Uploads.BaseURL+"/*", d.Uploads.Handler().ServeHTTP)
	}

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusNotFound, map[string]any{"error": map[string]string{"code": "NOT_FOUND", "message": "找不到資源"}})
	})
	return r
}
