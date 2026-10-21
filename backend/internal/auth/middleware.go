package auth

import (
	"context"
	"log/slog"
	"net/http"

	"auction-platform/backend/internal/apperr"
	"auction-platform/backend/internal/httpx"
)

const SessionCookieName = "session"

type userKey struct{}

// UserFromContext returns the authenticated user, or nil.
func UserFromContext(ctx context.Context) *CurrentUser {
	u, _ := ctx.Value(userKey{}).(*CurrentUser)
	return u
}

// WithUser returns ctx carrying u.
func WithUser(ctx context.Context, u *CurrentUser) context.Context {
	return context.WithValue(ctx, userKey{}, u)
}

var (
	ErrUnauthenticated = apperr.Unauthorized("UNAUTHENTICATED", "請先登入")
	ErrTermsRequired   = apperr.Forbidden("TERMS_NOT_ACCEPTED", "請先同意最新版本的使用者條款")
	ErrAdminRequired   = apperr.Forbidden("ADMIN_REQUIRED", "需要管理員權限")
)

// LoadUser attaches the session user (if any) to the request context.
// Banned users are treated as anonymous, so a ban takes effect immediately
// even for sessions opened before it.
func (s *Service) LoadUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(SessionCookieName)
		if err != nil || c.Value == "" {
			next.ServeHTTP(w, r)
			return
		}
		u, err := s.Authenticate(r.Context(), c.Value)
		if err != nil {
			slog.ErrorContext(r.Context(), "authenticate", "err", err)
			httpx.WriteError(w, r, err)
			return
		}
		if u != nil && u.Status != StatusBanned {
			r = r.WithContext(WithUser(r.Context(), u))
		}
		next.ServeHTTP(w, r)
	})
}

// RequireUser rejects anonymous requests. Use it for endpoints that must work
// before terms acceptance (profile lookup, accepting terms, logout).
func RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if UserFromContext(r.Context()) == nil {
			httpx.WriteError(w, r, ErrUnauthenticated)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireMember rejects anonymous users and users who have not accepted the
// current terms. All protected business endpoints use it.
func RequireMember(next http.Handler) http.Handler {
	return RequireUser(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !UserFromContext(r.Context()).TermsAccepted {
			httpx.WriteError(w, r, ErrTermsRequired)
			return
		}
		next.ServeHTTP(w, r)
	}))
}

// RequireAdmin rejects non-admin members.
func RequireAdmin(next http.Handler) http.Handler {
	return RequireMember(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !UserFromContext(r.Context()).IsAdmin() {
			httpx.WriteError(w, r, ErrAdminRequired)
			return
		}
		next.ServeHTTP(w, r)
	}))
}
