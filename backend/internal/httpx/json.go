// Package httpx contains small HTTP helpers shared by all handlers.
package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"auction-platform/backend/internal/apperr"
)

const maxJSONBody = 1 << 20 // 1 MiB

// HandlerFunc is an http.HandlerFunc that returns an error. Errors are
// written by Handle so handlers can simply `return err`.
type HandlerFunc func(w http.ResponseWriter, r *http.Request) error

// Handle adapts a HandlerFunc to http.HandlerFunc.
func Handle(h HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			WriteError(w, r, err)
		}
	}
}

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// WriteError writes err as a JSON error. Unexpected errors are logged and
// reported as a generic 500 so internals are never leaked to clients.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	if e, ok := apperr.As(err); ok {
		WriteJSON(w, statusFor(e.Kind), errorBody{errorDetail{e.Code, e.Message}})
		return
	}
	slog.ErrorContext(r.Context(), "request failed", "method", r.Method, "path", r.URL.Path, "err", err)
	WriteJSON(w, http.StatusInternalServerError, errorBody{errorDetail{"INTERNAL", "伺服器發生錯誤"}})
}

func statusFor(k apperr.Kind) int {
	switch k {
	case apperr.KindInvalid:
		return http.StatusBadRequest
	case apperr.KindUnauthorized:
		return http.StatusUnauthorized
	case apperr.KindForbidden:
		return http.StatusForbidden
	case apperr.KindNotFound:
		return http.StatusNotFound
	case apperr.KindConflict:
		return http.StatusConflict
	case apperr.KindTooManyRequests:
		return http.StatusTooManyRequests
	}
	return http.StatusInternalServerError
}

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

// DecodeJSON strictly decodes the request body into dst. Unknown fields are
// rejected, which prevents mass assignment of fields a DTO does not declare.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
	return DecodeJSONBytes(r.Body, dst)
}

// DecodeJSONBytes strictly decodes a single JSON object from rd.
func DecodeJSONBytes(rd io.Reader, dst any) error {
	dec := json.NewDecoder(rd)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return apperr.Invalid("BODY_TOO_LARGE", "請求內容過大")
		}
		return apperr.Invalid("INVALID_JSON", "請求格式錯誤")
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return apperr.Invalid("INVALID_JSON", "請求內容只能包含單一 JSON 物件")
	}
	return nil
}
