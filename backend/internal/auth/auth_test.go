package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v4"

	"auction-platform/backend/internal/apperr"
	"auction-platform/backend/internal/database/db"
	"auction-platform/backend/internal/httpx"
	"auction-platform/backend/internal/store"
	"auction-platform/backend/internal/terms"
)

var fixedNow = time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

func newMockService(t *testing.T, admins map[string]bool) (*Service, pgxmock.PgxPoolIface) {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		mock.Close()
	})
	svc := NewService(store.New(mock), MockLineClient{FrontendURL: "http://front"}, admins, time.Hour, func() time.Time { return fixedNow })
	return svc, mock
}

var upsertCols = []string{"id", "line_user_id", "display_name", "avatar_url", "role", "status", "created_at", "updated_at", "created"}

func TestLoginCreatesUserAndSession(t *testing.T) {
	svc, mock := newMockService(t, map[string]bool{"mock-boss": true})
	id := uuid.New()

	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO users").
		WithArgs("mock-boss", "Mock boss", "", RoleAdmin).
		WillReturnRows(pgxmock.NewRows(upsertCols).AddRow(id, "mock-boss", "Mock boss", "", RoleAdmin, StatusActive, fixedNow, fixedNow, true))
	mock.ExpectExec("INSERT INTO audit_logs").WithArgs(pgxmock.AnyArg(), "USER_CREATE", "USER", id.String(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), fixedNow).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec("INSERT INTO sessions").WithArgs(pgxmock.AnyArg(), id, pgxmock.AnyArg(), pgxmock.AnyArg(), fixedNow, fixedNow.Add(time.Hour)).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec("INSERT INTO audit_logs").WithArgs(pgxmock.AnyArg(), "LOGIN", "USER", id.String(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), fixedNow).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()

	res, err := svc.Login(context.Background(), "mock:boss")
	if err != nil {
		t.Fatal(err)
	}
	if res.Token == "" || res.User.Role != RoleAdmin || !res.ExpiresAt.Equal(fixedNow.Add(time.Hour)) {
		t.Fatalf("unexpected result %+v", res)
	}
}

func TestLoginRejectsBannedUserWithoutSession(t *testing.T) {
	svc, mock := newMockService(t, nil)
	id := uuid.New()

	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO users").WithArgs("mock-bad", "Mock bad", "", RoleUser).
		WillReturnRows(pgxmock.NewRows(upsertCols).AddRow(id, "mock-bad", "Mock bad", "", RoleUser, StatusBanned, fixedNow, fixedNow, false))
	// Only the rejection is audited: no USER_CREATE (existing user) and no session.
	mock.ExpectExec("INSERT INTO audit_logs").WithArgs(pgxmock.AnyArg(), "LOGIN_REJECTED_BANNED", "USER", id.String(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), fixedNow).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()

	_, err := svc.Login(context.Background(), "mock:bad")
	if !IsBanned(err) {
		t.Fatalf("expected banned error, got %v", err)
	}
}

func TestLoginRollsBackOnFailure(t *testing.T) {
	svc, mock := newMockService(t, nil)
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO users").WithArgs("mock-x", "Mock x", "", RoleUser).WillReturnError(context.DeadlineExceeded)
	mock.ExpectRollback()
	if _, err := svc.Login(context.Background(), "mock:x"); err == nil {
		t.Fatal("expected error")
	}
}

func TestLoginRejectsInvalidCode(t *testing.T) {
	svc, _ := newMockService(t, nil)
	_, err := svc.Login(context.Background(), "not-a-mock-code")
	if !apperr.IsCode(err, "LINE_LOGIN_FAILED") {
		t.Fatalf("got %v", err)
	}
}

func TestAcceptTermsVersionMismatch(t *testing.T) {
	svc, _ := newMockService(t, nil)
	err := svc.AcceptTerms(context.Background(), &CurrentUser{User: db.User{ID: uuid.New()}}, "old")
	if !apperr.IsCode(err, "TERMS_VERSION_MISMATCH") {
		t.Fatalf("got %v", err)
	}
}

func TestAcceptTermsRecordsAudit(t *testing.T) {
	svc, mock := newMockService(t, nil)
	id := uuid.New()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO user_terms").WithArgs(id, terms.CurrentVersion, fixedNow).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec("INSERT INTO audit_logs").WithArgs(pgxmock.AnyArg(), "TERMS_ACCEPT", "USER", id.String(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), fixedNow).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	if err := svc.AcceptTerms(context.Background(), &CurrentUser{User: db.User{ID: id}}, terms.CurrentVersion); err != nil {
		t.Fatal(err)
	}
}

var sessionCols = []string{"id", "line_user_id", "display_name", "avatar_url", "role", "status", "created_at", "updated_at", "terms_accepted"}

func TestLoadUserIgnoresBannedUsers(t *testing.T) {
	for _, tc := range []struct {
		status string
		want   bool
	}{{StatusActive, true}, {StatusBanned, false}} {
		t.Run(tc.status, func(t *testing.T) {
			svc, mock := newMockService(t, nil)
			mock.ExpectQuery("FROM sessions").WithArgs(terms.CurrentVersion, hashToken("tok"), fixedNow).
				WillReturnRows(pgxmock.NewRows(sessionCols).AddRow(uuid.New(), "L", "n", "", RoleUser, tc.status, fixedNow, fixedNow, true))
			var got *CurrentUser
			h := svc.LoadUser(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got = UserFromContext(r.Context()) }))
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "tok"})
			h.ServeHTTP(httptest.NewRecorder(), req)
			if (got != nil) != tc.want {
				t.Fatalf("user attached = %v, want %v", got != nil, tc.want)
			}
		})
	}
}

func TestAuthorizationMiddleware(t *testing.T) {
	member := &CurrentUser{User: db.User{Role: RoleUser}, TermsAccepted: true}
	noTerms := &CurrentUser{User: db.User{Role: RoleUser}, TermsAccepted: false}
	admin := &CurrentUser{User: db.User{Role: RoleAdmin}, TermsAccepted: true}
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	cases := []struct {
		name string
		mw   func(http.Handler) http.Handler
		user *CurrentUser
		want int
	}{
		{"user anonymous", RequireUser, nil, 401},
		{"user without terms", RequireUser, noTerms, 200},
		{"member anonymous", RequireMember, nil, 401},
		{"member without terms", RequireMember, noTerms, 403},
		{"member ok", RequireMember, member, 200},
		{"admin as member", RequireAdmin, member, 403},
		{"admin without terms", RequireAdmin, &CurrentUser{User: db.User{Role: RoleAdmin}}, 403},
		{"admin ok", RequireAdmin, admin, 200},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.user != nil {
				req = req.WithContext(WithUser(req.Context(), tc.user))
			}
			rec := httptest.NewRecorder()
			tc.mw(ok).ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}

func TestCallbackRejectsStateMismatch(t *testing.T) {
	svc, _ := newMockService(t, nil)
	h := NewHandler(svc, MockLineClient{}, "http://front", false, true)
	req := httptest.NewRequest(http.MethodGet, "/api/auth/line/callback?code=mock:a&state=attacker", nil)
	req.AddCookie(&http.Cookie{Name: stateCookieName, Value: "victim"})
	rec := httptest.NewRecorder()
	h.callback(rec, req)
	if loc := rec.Header().Get("Location"); loc != "http://front/login?error=INVALID_STATE" {
		t.Fatalf("Location = %q", loc)
	}
}

func TestMockLoginRedirectsToCallback(t *testing.T) {
	svc, _ := newMockService(t, nil)
	h := NewHandler(svc, MockLineClient{FrontendURL: "http://front"}, "http://front", false, true)
	rec := httptest.NewRecorder()
	h.login(rec, httptest.NewRequest(http.MethodGet, "/api/auth/line/login?mock_user=alice&next=//evil", nil))
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil || loc.Path != "/api/auth/line/callback" || loc.Query().Get("code") != "mock:alice" || loc.Query().Get("state") == "" {
		t.Fatalf("Location = %v", loc)
	}
	cookies := rec.Header().Values("Set-Cookie")
	if !strings.Contains(strings.Join(cookies, ";"), nextCookieName+"=%2F") {
		t.Fatalf("open redirect next should be sanitized to /: %v", cookies)
	}
}

func TestHTTPLineClientExchange(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth2/v2.1/token":
			_ = r.ParseForm()
			if r.Form.Get("code") != "c1" || r.Form.Get("client_secret") != "sec" || r.Form.Get("redirect_uri") != "https://app/cb" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "at"})
		case "/v2/profile":
			if r.Header.Get("Authorization") != "Bearer at" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"userId": "U123", "displayName": "Gecko", "pictureUrl": "https://p/x.jpg"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := NewHTTPLineClient("cid", "sec", "https://app/cb")
	c.APIBase = srv.URL
	p, err := c.Exchange(context.Background(), "c1")
	if err != nil {
		t.Fatal(err)
	}
	if p.UserID != "U123" || p.DisplayName != "Gecko" || p.PictureURL != "https://p/x.jpg" {
		t.Fatalf("profile = %+v", p)
	}
	if _, err := c.Exchange(context.Background(), "wrong"); err == nil {
		t.Fatal("expected error for rejected code")
	}
	if u := c.AuthURL("st"); !strings.Contains(u, "state=st") || !strings.Contains(u, "client_id=cid") || strings.Contains(u, "sec") {
		t.Fatalf("AuthURL = %s", u)
	}
}

func TestSanitizers(t *testing.T) {
	if got := sanitizeAvatarURL("javascript:alert(1)"); got != "" {
		t.Errorf("javascript url kept: %q", got)
	}
	if got := sanitizeAvatarURL("http://insecure/x.png"); got != "" {
		t.Errorf("http url kept: %q", got)
	}
	if got := sanitizeAvatarURL("https://profile.line-scdn.net/abc"); got == "" {
		t.Error("https url dropped")
	}
	if got := sanitizeDisplayName("   "); got != "LINE 使用者" {
		t.Errorf("blank name = %q", got)
	}
	if got := sanitizeDisplayName(strings.Repeat("守", 150)); len([]rune(got)) != maxNameLength {
		t.Errorf("long name not truncated: %d", len([]rune(got)))
	}
}

func TestCallbackEndsPreviousSessionEvenWhenLoginIsRejected(t *testing.T) {
	svc, mock := newMockService(t, nil)
	prevUser := uuid.New()
	// The browser is still signed in as prevUser.
	mock.ExpectQuery("FROM sessions").WithArgs(terms.CurrentVersion, hashToken("old-token"), fixedNow).
		WillReturnRows(pgxmock.NewRows(sessionCols).AddRow(prevUser, "L1", "prev", "", RoleAdmin, StatusActive, fixedNow, fixedNow, true))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE sessions SET revoked_at").WithArgs(fixedNow, hashToken("old-token")).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec("INSERT INTO audit_logs").WithArgs(pgxmock.AnyArg(), "LOGOUT", "USER", prevUser.String(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), fixedNow).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	// The new login is for a banned account.
	banned := uuid.New()
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO users").WithArgs("mock-carol", "Mock carol", "", RoleUser).
		WillReturnRows(pgxmock.NewRows(upsertCols).AddRow(banned, "mock-carol", "Mock carol", "", RoleUser, StatusBanned, fixedNow, fixedNow, false))
	mock.ExpectExec("INSERT INTO audit_logs").WithArgs(pgxmock.AnyArg(), "LOGIN_REJECTED_BANNED", "USER", banned.String(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), fixedNow).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()

	h := NewHandler(svc, MockLineClient{}, "http://front", false, true)
	req := httptest.NewRequest(http.MethodGet, "/api/auth/line/callback?code=mock:carol&state=s1", nil)
	req.AddCookie(&http.Cookie{Name: stateCookieName, Value: "s1"})
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "old-token"})
	rec := httptest.NewRecorder()
	h.callback(rec, req)

	if loc := rec.Header().Get("Location"); loc != "http://front/login?error=USER_BANNED" {
		t.Fatalf("Location = %q", loc)
	}
	cleared := false
	for _, c := range rec.Result().Cookies() {
		if c.Name == SessionCookieName && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Fatal("the previous session cookie must be cleared")
	}
}

func TestForgedCallbackDoesNotEndSession(t *testing.T) {
	svc, _ := newMockService(t, nil) // no DB expectations: nothing may be revoked
	h := NewHandler(svc, MockLineClient{}, "http://front", false, true)
	req := httptest.NewRequest(http.MethodGet, "/api/auth/line/callback?code=mock:x&state=forged", nil)
	req.AddCookie(&http.Cookie{Name: stateCookieName, Value: "real"})
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "victim-token"})
	rec := httptest.NewRecorder()
	h.callback(rec, req)
	for _, c := range rec.Result().Cookies() {
		if c.Name == SessionCookieName {
			t.Fatal("a forged callback must not touch the session")
		}
	}
}

func testLoginRequest(name, password string) *http.Request {
	body, _ := json.Marshal(map[string]string{"name": name, "password": password})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/test-login", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	return req.WithContext(httpx.WithMeta(req.Context(), httpx.RequestMeta{IP: "198.51.100.7"}))
}

func TestTestLoginRejections(t *testing.T) {
	svc, _ := newMockService(t, nil) // no DB access expected for any rejection
	h := NewHandler(svc, nil, "http://front", false, false)

	if err := h.testLogin(httptest.NewRecorder(), testLoginRequest("alice", "x")); !apperr.IsCode(err, "NOT_FOUND") {
		t.Fatalf("disabled test login must look like a missing route: %v", err)
	}
	h.TestPassword = "correct horse battery staple"
	for _, tc := range []struct{ name, pw string }{
		{"alice", "wrong password"},
		{"bad name!", "correct horse battery staple"},
		{"", "correct horse battery staple"},
	} {
		if err := h.testLogin(httptest.NewRecorder(), testLoginRequest(tc.name, tc.pw)); !apperr.IsCode(err, "INVALID_TEST_LOGIN") {
			t.Fatalf("%q/%q: %v", tc.name, tc.pw, err)
		}
	}
	// 3 attempts used above; 2 more allowed, then the IP is throttled.
	for range 2 {
		_ = h.testLogin(httptest.NewRecorder(), testLoginRequest("alice", "nope"))
	}
	if err := h.testLogin(httptest.NewRecorder(), testLoginRequest("alice", "correct horse battery staple")); !apperr.IsCode(err, "RATE_LIMITED") {
		t.Fatalf("brute force must be throttled even with the right password: %v", err)
	}
}

func TestTestLoginSuccess(t *testing.T) {
	svc, mock := newMockService(t, map[string]bool{"test-admin": true})
	id := uuid.New()
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO users").WithArgs("test-admin", "[測試] admin", "", RoleAdmin).
		WillReturnRows(pgxmock.NewRows(upsertCols).AddRow(id, "test-admin", "[測試] admin", "", RoleAdmin, StatusActive, fixedNow, fixedNow, false))
	mock.ExpectExec("INSERT INTO sessions").WithArgs(pgxmock.AnyArg(), id, "198.51.100.7", pgxmock.AnyArg(), fixedNow, fixedNow.Add(time.Hour)).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec("INSERT INTO audit_logs").WithArgs(pgxmock.AnyArg(), "LOGIN", "USER", id.String(), []byte(`{"method":"test"}`), pgxmock.AnyArg(), pgxmock.AnyArg(), fixedNow).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()

	h := NewHandler(svc, nil, "http://front", false, false)
	h.TestPassword = "correct horse battery staple"
	rec := httptest.NewRecorder()
	if err := h.testLogin(rec, testLoginRequest("admin", "correct horse battery staple")); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusNoContent || !strings.Contains(rec.Header().Get("Set-Cookie"), SessionCookieName+"=") {
		t.Fatalf("status %d cookies %v", rec.Code, rec.Header().Values("Set-Cookie"))
	}
}

func TestLineLoginWhenNotConfigured(t *testing.T) {
	svc, _ := newMockService(t, nil)
	h := NewHandler(svc, nil, "http://front", false, false)
	rec := httptest.NewRecorder()
	h.login(rec, httptest.NewRequest(http.MethodGet, "/api/auth/line/login", nil))
	if loc := rec.Header().Get("Location"); loc != "http://front/login?error=LINE_NOT_CONFIGURED" {
		t.Fatalf("Location = %q", loc)
	}
	mrec := httptest.NewRecorder()
	_ = h.methods(mrec, httptest.NewRequest(http.MethodGet, "/api/auth/methods", nil))
	if strings.TrimSpace(mrec.Body.String()) != `{"line":false,"mock":false,"test":false}` {
		t.Fatalf("methods = %s", mrec.Body.String())
	}
}
