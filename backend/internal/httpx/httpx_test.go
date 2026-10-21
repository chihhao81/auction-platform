package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
}

func TestCSRF(t *testing.T) {
	h := CSRF(false, "/api/payments/callback")(okHandler())

	t.Run("safe request without cookie gets a token cookie", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/x", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		if !strings.Contains(rec.Header().Get("Set-Cookie"), CSRFCookieName+"=") {
			t.Fatal("expected csrf cookie to be set")
		}
	})

	cases := []struct {
		name   string
		path   string
		cookie string
		header string
		want   int
	}{
		{"post without cookie", "/api/x", "", "", http.StatusForbidden},
		{"post without header", "/api/x", "tok", "", http.StatusForbidden},
		{"post with wrong header", "/api/x", "tok", "other", http.StatusForbidden},
		{"post with matching header", "/api/x", "tok", "tok", http.StatusOK},
		{"exempt path", "/api/payments/callback", "", "", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tc.path, nil)
			if tc.cookie != "" {
				req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: tc.cookie})
			}
			if tc.header != "" {
				req.Header.Set(CSRFHeaderName, tc.header)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}

func TestSafeRedirectPath(t *testing.T) {
	cases := map[string]string{
		"":                    "/",
		"/auctions/1":         "/auctions/1",
		"//evil.com":          "/",
		"https://evil.com":    "/",
		"/\\evil.com":         "/",
		"/ok?x=1":             "/ok?x=1",
		"javascript:alert(1)": "/",
		"/a\r\nSet-Cookie:x":  "/",
	}
	for in, want := range cases {
		if got := SafeRedirectPath(in); got != want {
			t.Errorf("SafeRedirectPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRateLimiter(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := NewRateLimiter(1, 2)
	l.now = func() time.Time { return now }
	if !l.Allow("a") || !l.Allow("a") {
		t.Fatal("burst should be allowed")
	}
	if l.Allow("a") {
		t.Fatal("third request should be limited")
	}
	if !l.Allow("b") {
		t.Fatal("other keys are independent")
	}
	now = now.Add(time.Second)
	if !l.Allow("a") {
		t.Fatal("token should refill after 1s")
	}
}

func TestDecodeJSONRejectsUnknownFields(t *testing.T) {
	var dst struct {
		Name string `json:"name"`
	}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"x","role":"ADMIN"}`))
	if err := DecodeJSON(httptest.NewRecorder(), req, &dst); err == nil {
		t.Fatal("expected unknown field to be rejected")
	}
	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"x"}{"name":"y"}`))
	if err := DecodeJSON(httptest.NewRecorder(), req, &dst); err == nil {
		t.Fatal("expected trailing data to be rejected")
	}
	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"x"}`))
	if err := DecodeJSON(httptest.NewRecorder(), req, &dst); err != nil || dst.Name != "x" {
		t.Fatalf("valid body: err=%v name=%q", err, dst.Name)
	}
}

func TestClientIP(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "203.0.113.5, 10.0.0.2")
	if got := clientIP(req, ""); got != "10.0.0.1" {
		t.Errorf("without trusted header got %q", got)
	}
	if got := clientIP(req, "X-Forwarded-For"); got != "203.0.113.5" {
		t.Errorf("with trusted header got %q", got)
	}
	req.Header.Set("X-Forwarded-For", "not-an-ip")
	if got := clientIP(req, "X-Forwarded-For"); got != "10.0.0.1" {
		t.Errorf("invalid header should fall back, got %q", got)
	}
}
