package auth

import (
	"crypto/subtle"
	"net/http"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"

	"auction-platform/backend/internal/apperr"
	"auction-platform/backend/internal/httpx"
	"auction-platform/backend/internal/terms"
)

const (
	stateCookieName = "line_oauth_state"
	nextCookieName  = "login_next"
	stateCookieTTL  = 10 * time.Minute
)

type Handler struct {
	svc          *Service
	line         LineClient
	frontendURL  string
	cookieSecure bool
	mock         bool
}

func NewHandler(svc *Service, line LineClient, frontendURL string, cookieSecure, mock bool) *Handler {
	return &Handler{svc: svc, line: line, frontendURL: frontendURL, cookieSecure: cookieSecure, mock: mock}
}

// Routes mounts the auth endpoints. LoadUser must run before these.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/auth/line/login", h.login)
	r.Get("/auth/line/callback", h.callback)
	r.Get("/terms", httpx.Handle(h.getTerms))
	r.Group(func(r chi.Router) {
		r.Use(RequireUser)
		r.Post("/auth/logout", httpx.Handle(h.logout))
		r.Get("/me", httpx.Handle(h.me))
		r.Post("/terms/accept", httpx.Handle(h.acceptTerms))
	})
}

func (h *Handler) setCookie(w http.ResponseWriter, name, value string, maxAge time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: value, Path: "/", HttpOnly: true,
		Secure: h.cookieSecure, SameSite: http.SameSiteLaxMode,
		MaxAge: int(maxAge.Seconds()),
	})
}

func (h *Handler) clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: "", Path: "/", HttpOnly: true,
		Secure: h.cookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
}

func (h *Handler) redirectFrontend(w http.ResponseWriter, r *http.Request, path string) {
	http.Redirect(w, r, h.frontendURL+path, http.StatusFound)
}

func (h *Handler) loginError(w http.ResponseWriter, r *http.Request, code string) {
	h.redirectFrontend(w, r, "/login?error="+url.QueryEscape(code))
}

// login starts the OAuth flow. The random state is bound to the browser via
// an HttpOnly cookie and checked on callback, preventing login CSRF.
func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	state := httpx.RandomToken()
	h.setCookie(w, stateCookieName, state, stateCookieTTL)
	h.setCookie(w, nextCookieName, url.QueryEscape(httpx.SafeRedirectPath(r.URL.Query().Get("next"))), stateCookieTTL)

	target := h.line.AuthURL(state)
	if h.mock {
		// Mock mode lets developers pick an identity: ?mock_user=alice
		name := r.URL.Query().Get("mock_user")
		if name == "" {
			name = "dev-user"
		}
		code, ok := MockCode(name)
		if !ok {
			h.loginError(w, r, "INVALID_MOCK_USER")
			return
		}
		target = h.frontendURL + "/api/auth/line/callback?" + url.Values{"code": {code}, "state": {state}}.Encode()
	}
	http.Redirect(w, r, target, http.StatusFound)
}

func (h *Handler) callback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	stateCookie, err := r.Cookie(stateCookieName)
	h.clearCookie(w, stateCookieName)
	if err != nil || stateCookie.Value == "" || subtle.ConstantTimeCompare([]byte(stateCookie.Value), []byte(q.Get("state"))) != 1 {
		h.loginError(w, r, "INVALID_STATE")
		return
	}
	if q.Get("error") != "" || q.Get("code") == "" {
		h.loginError(w, r, "LINE_DENIED")
		return
	}

	res, err := h.svc.Login(r.Context(), q.Get("code"))
	if err != nil {
		if e, ok := apperr.As(err); ok {
			h.loginError(w, r, e.Code)
			return
		}
		httpx.WriteError(w, r, err)
		return
	}
	h.setCookie(w, SessionCookieName, res.Token, time.Until(res.ExpiresAt))

	next := "/"
	if c, err := r.Cookie(nextCookieName); err == nil {
		if v, err := url.QueryUnescape(c.Value); err == nil {
			next = httpx.SafeRedirectPath(v)
		}
	}
	h.clearCookie(w, nextCookieName)
	h.redirectFrontend(w, r, next)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) error {
	c, err := r.Cookie(SessionCookieName)
	if err == nil && c.Value != "" {
		if err := h.svc.Logout(r.Context(), c.Value, UserFromContext(r.Context())); err != nil {
			return err
		}
	}
	h.clearCookie(w, SessionCookieName)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// UserDTO is the authenticated user's own view of their account.
type UserDTO struct {
	ID          string    `json:"id"`
	DisplayName string    `json:"display_name"`
	AvatarURL   string    `json:"avatar_url"`
	Role        string    `json:"role"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

type meResponse struct {
	User  UserDTO     `json:"user"`
	Terms termsStatus `json:"terms"`
}

type termsStatus struct {
	CurrentVersion string `json:"current_version"`
	Accepted       bool   `json:"accepted"`
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) error {
	u := UserFromContext(r.Context())
	httpx.WriteJSON(w, http.StatusOK, meResponse{
		User: UserDTO{
			ID: u.ID.String(), DisplayName: u.DisplayName, AvatarURL: u.AvatarUrl,
			Role: u.Role, Status: u.Status, CreatedAt: u.CreatedAt.UTC(),
		},
		Terms: termsStatus{CurrentVersion: terms.CurrentVersion, Accepted: u.TermsAccepted},
	})
	return nil
}

func (h *Handler) getTerms(w http.ResponseWriter, r *http.Request) error {
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"version": terms.CurrentVersion, "content": terms.Content})
	return nil
}

func (h *Handler) acceptTerms(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Version string `json:"version"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return err
	}
	if err := h.svc.AcceptTerms(r.Context(), UserFromContext(r.Context()), req.Version); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
