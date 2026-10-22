package config

import (
	"os"
	"strings"
	"testing"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadMockDevelopment(t *testing.T) {
	c, err := load(env(map[string]string{
		"DATABASE_URL":        "postgres://x",
		"MOCK_LINE_AUTH":      "true",
		"ADMIN_LINE_USER_IDS": " U1 , U2,,",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !c.MockLineAuth || c.CookieSecure || c.IsProduction() {
		t.Fatalf("unexpected config %+v", c)
	}
	if !c.AdminLineUserIDs["U1"] || !c.AdminLineUserIDs["U2"] || len(c.AdminLineUserIDs) != 2 {
		t.Fatalf("admins = %v", c.AdminLineUserIDs)
	}
}

func TestLoadRejectsMockInProduction(t *testing.T) {
	_, err := load(env(map[string]string{
		"APP_ENV": "production", "DATABASE_URL": "postgres://x", "MOCK_LINE_AUTH": "true",
		"FRONTEND_URL": "https://example.com",
	}))
	if err == nil || !strings.Contains(err.Error(), "MOCK_LINE_AUTH") {
		t.Fatalf("expected mock rejection, got %v", err)
	}
}

func TestLoadRequiresLineSecretsWithoutMock(t *testing.T) {
	_, err := load(env(map[string]string{"DATABASE_URL": "postgres://x"}))
	if err == nil || !strings.Contains(err.Error(), "LINE_CHANNEL_ID") {
		t.Fatalf("expected LINE config error, got %v", err)
	}
}

func TestProductionDefaultsToSecureCookies(t *testing.T) {
	c, err := load(env(map[string]string{
		"APP_ENV": "production", "DATABASE_URL": "postgres://x", "FRONTEND_URL": "https://example.com",
		"LINE_CHANNEL_ID": "id", "LINE_CHANNEL_SECRET": "s", "LINE_REDIRECT_URI": "https://example.com/cb",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !c.CookieSecure {
		t.Fatal("production must default to secure cookies")
	}
}

func TestR2StorageConfig(t *testing.T) {
	base := map[string]string{"DATABASE_URL": "postgres://x", "MOCK_LINE_AUTH": "true", "STORAGE_BACKEND": "r2"}
	if _, err := load(env(base)); err == nil || !strings.Contains(err.Error(), "R2_") {
		t.Fatalf("incomplete r2 config must fail, got %v", err)
	}
	base["R2_ACCOUNT_ID"] = "acc"
	base["R2_ACCESS_KEY_ID"] = "ak"
	base["R2_SECRET_ACCESS_KEY"] = "sk"
	base["R2_BUCKET"] = "b"
	base["R2_PUBLIC_BASE_URL"] = "https://img.example.com"
	c, err := load(env(base))
	if err != nil {
		t.Fatal(err)
	}
	if c.R2Endpoint != "acc.r2.cloudflarestorage.com" {
		t.Fatalf("endpoint %q", c.R2Endpoint)
	}
	if _, err := load(env(map[string]string{"DATABASE_URL": "postgres://x", "MOCK_LINE_AUTH": "true", "STORAGE_BACKEND": "s4"})); err == nil {
		t.Fatal("unknown backend must fail")
	}
}

// Every key in the env templates must be one the config actually reads, so a
// typo or a renamed setting cannot silently fall back to a default.
func TestEnvTemplatesOnlyUseKnownKeys(t *testing.T) {
	read := map[string]bool{}
	_, _ = load(func(k string) string { read[k] = true; return "" })
	// Keys read only in some branches, plus runtime settings handled outside config.
	for _, k := range []string{"R2_ENDPOINT", "R2_ACCOUNT_ID", "TEST_DATABASE_URL", "GOMEMLIMIT"} {
		read[k] = true
	}
	for _, path := range []string{"../../.env.example", "../../../deploy/backend.env.production.example"} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "#"))
			key, _, ok := strings.Cut(line, "=")
			if !ok || key == "" || strings.ContainsAny(key, " ./:") || strings.ToUpper(key) != key {
				continue
			}
			if !read[key] {
				t.Errorf("%s: unknown setting %s", path, key)
			}
		}
	}
}

func TestLoginMethodConfig(t *testing.T) {
	prod := func(extra map[string]string) map[string]string {
		m := map[string]string{"APP_ENV": "production", "DATABASE_URL": "postgres://x", "FRONTEND_URL": "https://app.example.com"}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	if _, err := load(env(prod(nil))); err == nil || !strings.Contains(err.Error(), "no login method") {
		t.Fatalf("production without any login method must fail: %v", err)
	}
	if _, err := load(env(prod(map[string]string{"TEST_LOGIN_PASSWORD": "short"}))); err == nil || !strings.Contains(err.Error(), "16 characters") {
		t.Fatalf("short test password must fail: %v", err)
	}
	c, err := load(env(prod(map[string]string{"TEST_LOGIN_PASSWORD": "a-long-enough-test-pass"})))
	if err != nil || c.LineEnabled() {
		t.Fatalf("test login alone must be enough in production: %v", err)
	}
	if _, err := load(env(prod(map[string]string{"TEST_LOGIN_PASSWORD": "a-long-enough-test-pass", "LINE_CHANNEL_ID": "x"}))); err == nil || !strings.Contains(err.Error(), "set together") {
		t.Fatalf("partial LINE config must fail: %v", err)
	}
	// The development template pre-fills LINE_REDIRECT_URI; with mock login on
	// that must not block startup.
	if _, err := load(env(map[string]string{"DATABASE_URL": "postgres://x", "MOCK_LINE_AUTH": "true",
		"LINE_REDIRECT_URI": "http://localhost:3000/api/auth/line/callback"})); err != nil {
		t.Fatalf("mock + template redirect URI must start: %v", err)
	}
}
