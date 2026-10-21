package httpx

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"auction-platform/backend/internal/apperr"
)

// SecurityHeaders sets conservative defaults for an API server.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

const (
	CSRFCookieName = "csrf_token"
	CSRFHeaderName = "X-CSRF-Token"
)

// CSRF implements the double-submit cookie pattern: every unsafe request must
// echo the csrf_token cookie in the X-CSRF-Token header. A cross-site page can
// neither read the cookie nor attach custom headers without a CORS preflight.
// Paths in exempt (exact match) skip the check; they must authenticate the
// caller by other means (e.g. a signed payment callback).
func CSRF(secure bool, exempt ...string) func(http.Handler) http.Handler {
	skip := map[string]bool{}
	for _, p := range exempt {
		skip[p] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var token string
			if c, err := r.Cookie(CSRFCookieName); err == nil {
				token = c.Value
			}
			if token == "" {
				http.SetCookie(w, &http.Cookie{
					Name: CSRFCookieName, Value: RandomToken(), Path: "/",
					Secure: secure, SameSite: http.SameSiteLaxMode,
					MaxAge: int((30 * 24 * time.Hour).Seconds()),
				})
			}
			if isUnsafe(r.Method) && !skip[r.URL.Path] {
				header := r.Header.Get(CSRFHeaderName)
				if token == "" || subtle.ConstantTimeCompare([]byte(header), []byte(token)) != 1 {
					WriteError(w, r, apperr.Forbidden("CSRF_FAILED", "CSRF 驗證失敗，請重新整理頁面"))
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func isUnsafe(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	return true
}

// RandomToken returns 32 bytes of crypto randomness, base64url encoded.
func RandomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand does not fail on supported platforms
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// RateLimiter is an in-memory per-key token bucket limiter. It is adequate
// for a single-instance deployment; idle entries are swept lazily.
type RateLimiter struct {
	mu        sync.Mutex
	limit     rate.Limit
	burst     int
	entries   map[string]*limiterEntry
	lastSweep time.Time
	now       func() time.Time
}

type limiterEntry struct {
	lim      *rate.Limiter
	lastSeen time.Time
}

func NewRateLimiter(perSecond float64, burst int) *RateLimiter {
	return &RateLimiter{
		limit: rate.Limit(perSecond), burst: burst,
		entries: map[string]*limiterEntry{}, now: time.Now,
	}
}

// Allow reports whether a request for key may proceed.
func (l *RateLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if now.Sub(l.lastSweep) > 5*time.Minute {
		for k, e := range l.entries {
			if now.Sub(e.lastSeen) > 10*time.Minute {
				delete(l.entries, k)
			}
		}
		l.lastSweep = now
	}
	e, ok := l.entries[key]
	if !ok {
		e = &limiterEntry{lim: rate.NewLimiter(l.limit, l.burst)}
		l.entries[key] = e
	}
	e.lastSeen = now
	return e.lim.AllowN(now, 1)
}

// RateLimitByIP rejects requests from an IP that exceeds the limiter.
func RateLimitByIP(l *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !l.Allow(MetaFromContext(r.Context()).IP) {
				w.Header().Set("Retry-After", "1")
				WriteError(w, r, apperr.TooMany("RATE_LIMITED", "請求過於頻繁，請稍後再試"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// SafeRedirectPath returns p if it is a same-origin relative path, else "/".
// It prevents open redirects through ?next= parameters.
func SafeRedirectPath(p string) string {
	if p == "" || !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") || strings.ContainsAny(p, "\\\r\n") {
		return "/"
	}
	return p
}
