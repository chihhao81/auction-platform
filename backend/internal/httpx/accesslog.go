package httpx

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// AccessLog writes one structured line per request. Only the path is logged,
// never the query string: it can carry secrets such as the OAuth code on the
// LINE login callback.
func AccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		status := ww.Status()
		if status == 0 {
			status = http.StatusOK
		}
		level := slog.LevelInfo
		if status >= 500 {
			level = slog.LevelError
		}
		slog.Log(r.Context(), level, "http",
			"method", r.Method, "path", r.URL.Path, "status", status,
			"duration_ms", time.Since(start).Milliseconds(), "bytes", ww.BytesWritten(),
			"ip", MetaFromContext(r.Context()).IP, "request_id", middleware.GetReqID(r.Context()))
	})
}

// HSTS tells browsers to use HTTPS only. Enable it only when the site is
// served over HTTPS (production).
func HSTS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		next.ServeHTTP(w, r)
	})
}
