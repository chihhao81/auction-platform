// Package config loads runtime configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Env            string // "development" or "production"
	Port           string
	DatabaseURL    string
	MigrateOnStart bool

	// FrontendURL is the public origin of the Next.js app. The frontend proxies
	// /api/* to this backend, so cookies are first-party on this origin.
	FrontendURL string

	LineChannelID     string
	LineChannelSecret string
	LineRedirectURI   string
	MockLineAuth      bool
	AdminLineUserIDs  map[string]bool

	SessionTTL   time.Duration
	CookieSecure bool

	// ClientIPHeader names a header set by a trusted reverse proxy that holds
	// the client IP (e.g. X-Forwarded-For). Empty means use the TCP peer.
	ClientIPHeader string

	// UploadDir is where the local storage backend keeps images.
	UploadDir string
}

func (c *Config) IsProduction() bool { return c.Env == "production" }

// Load reads configuration from the process environment.
func Load() (*Config, error) { return load(os.Getenv) }

func load(getenv func(string) string) (*Config, error) {
	get := func(key, def string) string {
		if v := strings.TrimSpace(getenv(key)); v != "" {
			return v
		}
		return def
	}
	c := &Config{
		Env:               get("APP_ENV", "development"),
		Port:              get("PORT", "8080"),
		DatabaseURL:       get("DATABASE_URL", ""),
		FrontendURL:       strings.TrimRight(get("FRONTEND_URL", "http://localhost:3000"), "/"),
		LineChannelID:     get("LINE_CHANNEL_ID", ""),
		LineChannelSecret: get("LINE_CHANNEL_SECRET", ""),
		LineRedirectURI:   get("LINE_REDIRECT_URI", ""),
		ClientIPHeader:    get("CLIENT_IP_HEADER", ""),
		UploadDir:         get("UPLOAD_DIR", "./uploads"),
		AdminLineUserIDs:  map[string]bool{},
	}
	var errs []error
	parseBool := func(key string, def bool) bool {
		v := get(key, strconv.FormatBool(def))
		b, err := strconv.ParseBool(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", key, err))
		}
		return b
	}
	c.MigrateOnStart = parseBool("MIGRATE_ON_START", true)
	c.MockLineAuth = parseBool("MOCK_LINE_AUTH", false)
	c.CookieSecure = parseBool("COOKIE_SECURE", c.IsProduction())

	ttl, err := time.ParseDuration(get("SESSION_TTL", "168h"))
	if err != nil || ttl <= 0 {
		errs = append(errs, fmt.Errorf("SESSION_TTL: invalid duration"))
	}
	c.SessionTTL = ttl

	for _, id := range strings.Split(get("ADMIN_LINE_USER_IDS", ""), ",") {
		if id = strings.TrimSpace(id); id != "" {
			c.AdminLineUserIDs[id] = true
		}
	}

	if c.Env != "development" && c.Env != "production" {
		errs = append(errs, fmt.Errorf("APP_ENV must be development or production"))
	}
	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}
	if u, err := url.Parse(c.FrontendURL); err != nil || u.Scheme == "" || u.Host == "" {
		errs = append(errs, errors.New("FRONTEND_URL must be an absolute URL"))
	}
	if c.MockLineAuth && c.IsProduction() {
		errs = append(errs, errors.New("MOCK_LINE_AUTH must not be enabled in production"))
	}
	if !c.MockLineAuth && (c.LineChannelID == "" || c.LineChannelSecret == "" || c.LineRedirectURI == "") {
		errs = append(errs, errors.New("LINE_CHANNEL_ID, LINE_CHANNEL_SECRET and LINE_REDIRECT_URI are required unless MOCK_LINE_AUTH=true"))
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return c, nil
}
