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
	// TestLoginPassword enables password-protected test accounts ("name +
	// shared password") for early testing of a deployed site. Empty = off.
	TestLoginPassword string
	AdminLineUserIDs  map[string]bool

	SessionTTL   time.Duration
	CookieSecure bool

	// ClientIPHeader names a header set by a trusted reverse proxy that holds
	// the client IP (e.g. X-Forwarded-For). Empty means use the TCP peer.
	ClientIPHeader string

	// StorageBackend is "local" (disk, development) or "r2" (Cloudflare R2 /
	// any S3-compatible store, production).
	StorageBackend string
	// UploadDir is where the local storage backend keeps images.
	UploadDir string
	// R2 settings; the secret key stays in the backend env only.
	R2Endpoint        string
	R2AccessKeyID     string
	R2SecretAccessKey string
	R2Bucket          string
	R2PublicBaseURL   string

	// The scheduler settles ended auctions, merges orders and sends timed
	// notifications. Disable it on extra instances sharing a database if
	// desired (jobs are idempotent, so running twice is still safe).
	SchedulerEnabled  bool
	SchedulerInterval time.Duration

	// Web Push (VAPID). Push is disabled when the keys are empty. The private
	// key must only ever live in the backend environment.
	VAPIDPublicKey  string
	VAPIDPrivateKey string
	VAPIDSubject    string

	// ECPay (綠界). HashKey/HashIV are secrets and stay in the backend env.
	// Payments are disabled while ECPAY_MERCHANT_ID is empty.
	ECPayMerchantID string
	ECPayHashKey    string
	ECPayHashIV     string
	ECPayBaseURL    string
	// ECPayAllowSimulated accepts stage "simulate paid" notifications.
	ECPayAllowSimulated bool
	// PublicBaseURL is the site origin ECPay calls back to (the frontend
	// proxies /api to the backend). Defaults to FRONTEND_URL.
	PublicBaseURL string

	OrderAutoReceiveAfter  time.Duration
	OrderAutoCompleteAfter time.Duration
	ShippingInfoRetention  time.Duration
}

// LineEnabled reports whether real LINE Login is configured.
func (c *Config) LineEnabled() bool {
	return c.LineChannelID != "" && c.LineChannelSecret != "" && c.LineRedirectURI != ""
}

// ECPayEnabled reports whether ECPay payments are configured.
func (c *Config) ECPayEnabled() bool { return c.ECPayMerchantID != "" }

// PushEnabled reports whether Web Push is configured.
func (c *Config) PushEnabled() bool { return c.VAPIDPublicKey != "" && c.VAPIDPrivateKey != "" }

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
		StorageBackend:    get("STORAGE_BACKEND", "local"),
		R2AccessKeyID:     get("R2_ACCESS_KEY_ID", ""),
		R2SecretAccessKey: get("R2_SECRET_ACCESS_KEY", ""),
		R2Bucket:          get("R2_BUCKET", ""),
		R2PublicBaseURL:   get("R2_PUBLIC_BASE_URL", ""),
		VAPIDPublicKey:    get("VAPID_PUBLIC_KEY", ""),
		VAPIDPrivateKey:   get("VAPID_PRIVATE_KEY", ""),
		VAPIDSubject:      get("VAPID_SUBJECT", ""),
		ECPayMerchantID:   get("ECPAY_MERCHANT_ID", ""),
		ECPayHashKey:      get("ECPAY_HASH_KEY", ""),
		ECPayHashIV:       get("ECPAY_HASH_IV", ""),
		ECPayBaseURL:      get("ECPAY_BASE_URL", "https://payment-stage.ecpay.com.tw"),
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
	c.TestLoginPassword = getenv("TEST_LOGIN_PASSWORD") // not trimmed: spaces are allowed
	c.CookieSecure = parseBool("COOKIE_SECURE", c.IsProduction())
	c.SchedulerEnabled = parseBool("SCHEDULER_ENABLED", true)
	c.R2Endpoint = get("R2_ENDPOINT", "")
	if c.R2Endpoint == "" && get("R2_ACCOUNT_ID", "") != "" {
		c.R2Endpoint = get("R2_ACCOUNT_ID", "") + ".r2.cloudflarestorage.com"
	}
	c.ECPayAllowSimulated = parseBool("ECPAY_ALLOW_SIMULATED", !c.IsProduction())
	c.PublicBaseURL = strings.TrimRight(get("PUBLIC_BASE_URL", c.FrontendURL), "/")

	parseDur := func(key, def string, min time.Duration) time.Duration {
		d, err := time.ParseDuration(get(key, def))
		if err != nil || d < min {
			errs = append(errs, fmt.Errorf("%s: must be a duration >= %s", key, min))
		}
		return d
	}
	c.OrderAutoReceiveAfter = parseDur("ORDER_AUTO_RECEIVE_AFTER", "72h", time.Hour)
	c.OrderAutoCompleteAfter = parseDur("ORDER_AUTO_COMPLETE_AFTER", "168h", time.Hour)
	c.ShippingInfoRetention = parseDur("SHIPPING_INFO_RETENTION", "720h", 24*time.Hour)

	interval, err := time.ParseDuration(get("SCHEDULER_INTERVAL", "1m"))
	if err != nil || interval < 10*time.Second || interval > time.Hour {
		errs = append(errs, fmt.Errorf("SCHEDULER_INTERVAL must be between 10s and 1h"))
	}
	c.SchedulerInterval = interval

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
	lineSet := c.LineChannelID != "" || c.LineChannelSecret != "" || c.LineRedirectURI != ""
	// Mock login replaces LINE entirely, so partial LINE settings (e.g. the
	// redirect URI pre-filled in .env.example) are only an error without it.
	if lineSet && !c.LineEnabled() && !c.MockLineAuth {
		errs = append(errs, errors.New("LINE_CHANNEL_ID, LINE_CHANNEL_SECRET and LINE_REDIRECT_URI must be set together"))
	}
	if c.TestLoginPassword != "" && len([]rune(c.TestLoginPassword)) < 16 {
		errs = append(errs, errors.New("TEST_LOGIN_PASSWORD must be at least 16 characters"))
	}
	if !c.MockLineAuth && !c.LineEnabled() && c.TestLoginPassword == "" {
		errs = append(errs, errors.New("no login method: set LINE_CHANNEL_ID/LINE_CHANNEL_SECRET/LINE_REDIRECT_URI, TEST_LOGIN_PASSWORD, or MOCK_LINE_AUTH=true (development only)"))
	}
	if (c.VAPIDPublicKey == "") != (c.VAPIDPrivateKey == "") {
		errs = append(errs, errors.New("VAPID_PUBLIC_KEY and VAPID_PRIVATE_KEY must be set together"))
	}
	if c.PushEnabled() && !strings.HasPrefix(c.VAPIDSubject, "mailto:") && !strings.HasPrefix(c.VAPIDSubject, "https://") {
		errs = append(errs, errors.New("VAPID_SUBJECT must be a mailto: or https: URL when push is enabled"))
	}
	if c.ECPayEnabled() && (c.ECPayHashKey == "" || c.ECPayHashIV == "") {
		errs = append(errs, errors.New("ECPAY_HASH_KEY and ECPAY_HASH_IV are required when ECPAY_MERCHANT_ID is set"))
	}
	switch c.StorageBackend {
	case "local":
	case "r2":
		if c.R2Endpoint == "" || c.R2AccessKeyID == "" || c.R2SecretAccessKey == "" || c.R2Bucket == "" || c.R2PublicBaseURL == "" {
			errs = append(errs, errors.New("STORAGE_BACKEND=r2 requires R2_ACCOUNT_ID (or R2_ENDPOINT), R2_ACCESS_KEY_ID, R2_SECRET_ACCESS_KEY, R2_BUCKET and R2_PUBLIC_BASE_URL"))
		}
	default:
		errs = append(errs, errors.New("STORAGE_BACKEND must be local or r2"))
	}
	if c.IsProduction() && c.ECPayAllowSimulated {
		errs = append(errs, errors.New("ECPAY_ALLOW_SIMULATED must be false in production"))
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return c, nil
}
