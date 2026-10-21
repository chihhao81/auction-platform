package httpx

import (
	"context"
	"net"
	"net/http"
	"strings"
)

type metaKey struct{}

// RequestMeta carries client information used for audit logging and rate
// limiting.
type RequestMeta struct {
	IP        string
	UserAgent string
}

// MetaFromContext returns the request metadata stored by ClientMeta.
func MetaFromContext(ctx context.Context) RequestMeta {
	m, _ := ctx.Value(metaKey{}).(RequestMeta)
	return m
}

// WithMeta returns a context carrying m.
func WithMeta(ctx context.Context, m RequestMeta) context.Context {
	return context.WithValue(ctx, metaKey{}, m)
}

// ClientMeta records the client IP and user agent on the request context.
// ipHeader is only honoured when the backend sits behind a trusted proxy that
// overwrites it; otherwise pass "" to use the TCP peer address.
func ClientMeta(ipHeader string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ua := r.UserAgent()
			if len(ua) > 512 {
				ua = ua[:512]
			}
			m := RequestMeta{IP: clientIP(r, ipHeader), UserAgent: ua}
			next.ServeHTTP(w, r.WithContext(WithMeta(r.Context(), m)))
		})
	}
}

func clientIP(r *http.Request, ipHeader string) string {
	if ipHeader != "" {
		if v := r.Header.Get(ipHeader); v != "" {
			first := strings.TrimSpace(strings.Split(v, ",")[0])
			if ip := net.ParseIP(first); ip != nil {
				return ip.String()
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
