package config

import (
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
