package httpapi

import (
	"database/sql"
	"net/http"
	"net/url"
	"strings"
)

type App struct {
	db        *sql.DB
	cfg       Config
	uploadDir string
}

func (a *App) cors(next http.Handler) http.Handler {
	frontend, _ := url.Parse(a.cfg.FrontendURL)
	origin := frontend.Scheme + "://" + frontend.Host
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestOrigin := r.Header.Get("Origin")
		if requestOrigin != "" && requestOrigin == origin {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Add("Vary", "Origin")
			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		if r.Method == http.MethodOptions {
			http.Error(w, "origin not allowed", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) validOrigin(r *http.Request) bool {
	frontend, err := url.Parse(a.cfg.FrontendURL)
	if err != nil {
		return false
	}
	return strings.EqualFold(r.Header.Get("Origin"), frontend.Scheme+"://"+frontend.Host)
}

type currentUserResponse struct {
	ID          int64  `json:"id"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
}

func (a *App) currentUser(w http.ResponseWriter, r *http.Request) {
	if a.db == nil {
		writeError(w, http.StatusServiceUnavailable, "database is not configured")
		return
	}
	userID, err := a.sessionUserID(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "login required")
		return
	}
	var response currentUserResponse
	var status string
	err = a.db.QueryRowContext(r.Context(), `
		SELECT id, display_name, avatar_url, status
		FROM users WHERE id = $1`, userID).
		Scan(&response.ID, &response.DisplayName, &response.AvatarURL, &status)
	if err == sql.ErrNoRows {
		a.clearSession(w)
		writeError(w, http.StatusUnauthorized, "login required")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load user")
		return
	}
	if status != "ACTIVE" {
		a.clearSession(w)
		writeError(w, http.StatusForbidden, "account is not active")
		return
	}
	var accepted bool
	err = a.db.QueryRowContext(r.Context(), `
		SELECT EXISTS (
			SELECT 1 FROM user_terms WHERE user_id = $1 AND terms_version = $2
		)`, userID, a.cfg.TermsVersion).Scan(&accepted)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not check terms acceptance")
		return
	}
	if !accepted {
		writeJSON(w, http.StatusPreconditionRequired, errorResponse{Error: "current terms must be accepted", TermsVersion: a.cfg.TermsVersion})
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	if !a.validOrigin(r) {
		writeError(w, http.StatusForbidden, "origin not allowed")
		return
	}
	a.clearSession(w)
	w.WriteHeader(http.StatusNoContent)
}
