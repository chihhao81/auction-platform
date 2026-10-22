package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/pashagolub/pgxmock/v4"

	"auction-platform/backend/internal/auth"
	"auction-platform/backend/internal/config"
	"auction-platform/backend/internal/httpx"
	"auction-platform/backend/internal/storage"
	"auction-platform/backend/internal/store"
)

// publicRoutes are the only endpoints reachable without a session. Adding a
// route anywhere else without RequireUser/RequireMember/RequireAdmin makes
// TestEveryOtherRouteRequiresLogin fail.
var publicRoutes = map[string]bool{
	"GET /api/health":                 true,
	"GET /api/auth/methods":           true,
	"POST /api/auth/test-login":       true, // requires TEST_LOGIN_PASSWORD; 404 when unset
	"GET /api/auth/line/login":        true,
	"GET /api/auth/line/callback":     true,
	"GET /api/terms":                  true,
	"GET /api/auctions":               true,
	"GET /api/auctions/prices":        true,
	"GET /api/auctions/{id}":          true,
	"GET /api/push/public-key":        true,
	"GET /api/users/{id}":             true,
	"GET /api/users/{id}/reviews":     true,
	"GET /api/reports/reasons":        true,
	"POST /api/payments/ecpay/notify": true, // authenticated by CheckMacValue
	"GET /uploads/*":                  true,
}

func testRouter(t *testing.T) http.Handler {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// Anonymous requests to protected routes must be rejected before
		// touching the database.
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		mock.Close()
	})
	st := store.New(mock)
	cfg := &config.Config{FrontendURL: "http://front", SchedulerInterval: time.Minute}
	return NewRouter(Deps{
		Config: cfg, Store: st,
		Auth: auth.NewService(st, auth.MockLineClient{}, nil, time.Hour, time.Now),
		Line: auth.MockLineClient{}, Now: time.Now,
		Uploads: &storage.Local{Dir: t.TempDir(), BaseURL: "/uploads"},
	})
}

func TestEveryOtherRouteRequiresLogin(t *testing.T) {
	h := testRouter(t)
	routes, ok := h.(chi.Routes)
	if !ok {
		t.Fatal("router does not expose its routes")
	}
	seen := map[string]bool{}
	checked := 0
	err := chi.Walk(routes, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		key := method + " " + route
		seen[key] = true
		if publicRoutes[key] {
			return nil
		}
		path := strings.NewReplacer("{id}", "00000000-0000-0000-0000-000000000001", "*", "x").Replace(route)
		req := httptest.NewRequest(method, path, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: httpx.CSRFCookieName, Value: "t"})
		req.Header.Set(httpx.CSRFHeaderName, "t")
		req.RemoteAddr = "192.0.2.1:1234"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: anonymous request got %d, want 401", key, rec.Code)
		}
		checked++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked < 35 {
		t.Fatalf("only %d protected routes found; router walk is not seeing everything", checked)
	}
	for r := range publicRoutes {
		if !seen[r] {
			t.Errorf("public route %s no longer exists; update the allowlist", r)
		}
	}
}

func TestUnsafeRequestsNeedCSRFToken(t *testing.T) {
	h := testRouter(t)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req.RemoteAddr = "192.0.2.1:1234"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("POST without CSRF token: %d", rec.Code)
	}
}

func TestSecurityHeaders(t *testing.T) {
	h := testRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/terms", nil)
	req.RemoteAddr = "192.0.2.1:1234"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	for k, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
	} {
		if got := rec.Header().Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	if !strings.Contains(rec.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Error("missing CSP")
	}
}
