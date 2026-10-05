package httpapi

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigRequiresStrongSessionSecret(t *testing.T) {
	cfg := Config{
		FrontendURL: "http://localhost:3000", LineChannelID: "channel", LineChannelSecret: "secret",
		LineCallbackURL: "http://localhost:8080/v1/auth/line/callback", SessionSecret: "short",
		TermsVersion: "v1", CookieSameSite: "lax",
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected short session secret to be rejected")
	}
	cfg.SessionSecret = "0123456789abcdef0123456789abcdef"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid local config, got %v", err)
	}
}

func TestSameSiteNoneRequiresSecureCookie(t *testing.T) {
	cfg := Config{
		FrontendURL: "https://example.test", LineChannelID: "channel", LineChannelSecret: "secret",
		LineCallbackURL: "https://api.example.test/v1/auth/line/callback", SessionSecret: "0123456789abcdef0123456789abcdef",
		TermsVersion: "v1", CookieSameSite: "none",
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected insecure cross-site cookie config to be rejected")
	}
}

func TestConfigRequiresFrontendOriginOnly(t *testing.T) {
	cfg := Config{
		FrontendURL: "https://example.test/nested", LineChannelID: "channel", LineChannelSecret: "secret",
		LineCallbackURL: "https://api.example.test/v1/auth/line/callback", SessionSecret: "0123456789abcdef0123456789abcdef",
		TermsVersion: "v1", CookieSameSite: "lax", CookieSecure: true,
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected frontend path to be rejected")
	}
}

func TestConfigLoadsTermsTextFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "terms.txt")
	if err := os.WriteFile(path, []byte("Reviewed terms"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TERMS_TEXT", "")
	t.Setenv("TERMS_TEXT_FILE", path)
	cfg := ConfigFromEnv()
	if cfg.TermsText != "Reviewed terms" || cfg.TermsLoadError != "" {
		t.Fatalf("unexpected terms configuration: text=%q loadError=%q", cfg.TermsText, cfg.TermsLoadError)
	}
}

func TestConfigRejectsMissingTermsFile(t *testing.T) {
	t.Setenv("TERMS_TEXT_FILE", filepath.Join(t.TempDir(), "missing.txt"))
	if err := ConfigFromEnv().Validate(); err == nil {
		t.Fatal("expected missing terms file to fail startup validation")
	}
}
