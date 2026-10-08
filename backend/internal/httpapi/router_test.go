package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestHealth(t *testing.T) {
	response := httptest.NewRecorder()
	NewRouter().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/health", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if got := response.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
}

func TestLineVerifyResponseClaims(t *testing.T) {
	const payload = `{"iss":"https://access.line.me","sub":"U123","aud":"channel-1","exp":2000000100,"iat":2000000000,"nonce":"expected","name":"Sample","picture":"https://example.test/avatar.png"}`
	var identity lineIdentity
	if err := json.Unmarshal([]byte(payload), &identity); err != nil {
		t.Fatal(err)
	}
	now := time.Unix(2_000_000_000, 0).UTC()
	if err := validateLineIdentity(identity, "channel-1", "expected", now); err != nil {
		t.Fatalf("valid provider claims rejected: %v", err)
	}
	identity.Audience = "other-channel"
	if err := validateLineIdentity(identity, "channel-1", "expected", now); err == nil {
		t.Fatal("wrong audience should be rejected")
	}
}

func TestLineLoginRequiresConfiguration(t *testing.T) {
	t.Setenv("LINE_CHANNEL_ID", "")
	t.Setenv("LINE_CHANNEL_SECRET", "")
	t.Setenv("LINE_CALLBACK_URL", "")
	response := httptest.NewRecorder()
	NewRouter().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/auth/line", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
}

func TestLineCallbackRejectsMissingOAuthState(t *testing.T) {
	response := httptest.NewRecorder()
	NewRouter().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/auth/line/callback", nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestLineLoginRedirectIncludesStateAndNonceCookies(t *testing.T) {
	cfg := Config{
		FrontendURL: "http://localhost:3000", LineChannelID: "test-channel", LineChannelSecret: "test-secret",
		LineCallbackURL: "http://localhost:8080/v1/auth/line/callback", SessionSecret: "0123456789abcdef0123456789abcdef",
		TermsVersion: "v1", CookieSameSite: "lax",
	}
	response := httptest.NewRecorder()
	NewRouterWithConfig(nil, cfg).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/auth/line", nil))
	if response.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusFound)
	}
	location, err := url.Parse(response.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	query := location.Query()
	if query.Get("client_id") != cfg.LineChannelID || query.Get("redirect_uri") != cfg.LineCallbackURL || query.Get("nonce") == "" {
		t.Fatalf("unexpected LINE authorization query: %v", query)
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 2 {
		t.Fatalf("got %d OAuth cookies, want state and nonce", len(cookies))
	}
	values := map[string]string{}
	for _, cookie := range cookies {
		values[cookie.Name] = cookie.Value
		if !cookie.HttpOnly || cookie.Path != "/v1/auth/line/callback" || cookie.SameSite != http.SameSiteLaxMode {
			t.Errorf("unsafe OAuth cookie settings: %+v", cookie)
		}
	}
	if values["line_oauth_state"] != query.Get("state") || values["line_oauth_nonce"] != query.Get("nonce") {
		t.Fatal("OAuth state/nonce cookies did not match redirect parameters")
	}
}

func TestConfiguredFrontendOriginGetsCredentialedCORS(t *testing.T) {
	cfg := Config{FrontendURL: "http://localhost:3000", CookieSameSite: "lax"}
	request := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	request.Header.Set("Origin", "http://localhost:3000")
	response := httptest.NewRecorder()
	NewRouterWithConfig(nil, cfg).ServeHTTP(response, request)
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Fatalf("Access-Control-Allow-Origin = %q", got)
	}
	if got := response.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("Access-Control-Allow-Credentials = %q", got)
	}
	request = httptest.NewRequest(http.MethodOptions, "/v1/auctions/1", nil)
	request.Header.Set("Origin", "http://localhost:3000")
	request.Header.Set("Access-Control-Request-Method", "PATCH")
	response = httptest.NewRecorder()
	NewRouterWithConfig(nil, cfg).ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || !strings.Contains(response.Header().Get("Access-Control-Allow-Methods"), "PATCH") {
		t.Fatalf("PATCH preflight rejected: status=%d methods=%q", response.Code, response.Header().Get("Access-Control-Allow-Methods"))
	}
	request = httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	request.Header.Set("Origin", "https://attacker.example")
	response = httptest.NewRecorder()
	NewRouterWithConfig(nil, cfg).ServeHTTP(response, request)
	if strings.Contains(response.Header().Get("Access-Control-Allow-Origin"), "attacker") {
		t.Fatal("untrusted origin received CORS access")
	}
}
