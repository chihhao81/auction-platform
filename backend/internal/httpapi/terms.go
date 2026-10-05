package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
)

type termsResponse struct {
	Version string `json:"version"`
	Text    string `json:"text"`
}

func (a *App) currentTerms(w http.ResponseWriter, r *http.Request) {
	if a.cfg.TermsText == "" {
		writeError(w, http.StatusServiceUnavailable, "terms text is not configured")
		return
	}
	writeJSON(w, http.StatusOK, termsResponse{Version: a.cfg.TermsVersion, Text: a.cfg.TermsText})
}

func (a *App) acceptTerms(w http.ResponseWriter, r *http.Request) {
	if a.db == nil {
		writeError(w, http.StatusServiceUnavailable, "database is not configured")
		return
	}
	if !a.validOrigin(r) {
		writeError(w, http.StatusForbidden, "origin not allowed")
		return
	}
	if a.cfg.TermsText == "" {
		writeError(w, http.StatusServiceUnavailable, "terms text is not configured")
		return
	}
	userID, err := a.sessionUserID(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "login required")
		return
	}
	var status string
	err = a.db.QueryRowContext(r.Context(), `SELECT status FROM users WHERE id = $1`, userID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusUnauthorized, "login required")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load account")
		return
	}
	if status != "ACTIVE" {
		writeError(w, http.StatusForbidden, "account is not active")
		return
	}
	mediaType, _, mediaErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mediaErr != nil || mediaType != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "application/json required")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request struct {
		Version string `json:"version"`
	}
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if request.Version != a.cfg.TermsVersion {
		writeJSON(w, http.StatusPreconditionRequired, errorResponse{Error: "current terms version required", TermsVersion: a.cfg.TermsVersion})
		return
	}
	_, err = a.db.ExecContext(r.Context(), `
		INSERT INTO user_terms (user_id, terms_version)
		VALUES ($1, $2)
		ON CONFLICT (user_id, terms_version) DO NOTHING`, userID, a.cfg.TermsVersion)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not record terms acceptance")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
