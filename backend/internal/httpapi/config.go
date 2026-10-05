package httpapi

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

type Config struct {
	FrontendURL       string
	LineChannelID     string
	LineChannelSecret string
	LineCallbackURL   string
	SessionSecret     string
	TermsVersion      string
	TermsText         string
	TermsLoadError    string
	CookieSecure      bool
	CookieSameSite    string
}

func ConfigFromEnv() Config {
	termsText := os.Getenv("TERMS_TEXT")
	termsLoadError := ""
	if path := os.Getenv("TERMS_TEXT_FILE"); path != "" {
		contents, err := os.ReadFile(path)
		if err != nil {
			termsLoadError = fmt.Sprintf("read TERMS_TEXT_FILE: %v", err)
		} else {
			termsText = string(contents)
		}
	}
	return Config{
		FrontendURL:       env("FRONTEND_URL", "http://localhost:3000"),
		LineChannelID:     os.Getenv("LINE_CHANNEL_ID"),
		LineChannelSecret: os.Getenv("LINE_CHANNEL_SECRET"),
		LineCallbackURL:   os.Getenv("LINE_CALLBACK_URL"),
		SessionSecret:     os.Getenv("SESSION_SECRET"),
		TermsVersion:      env("TERMS_VERSION", "v1"),
		TermsText:         termsText,
		TermsLoadError:    termsLoadError,
		CookieSecure:      os.Getenv("SESSION_COOKIE_SECURE") == "true",
		CookieSameSite:    strings.ToLower(env("SESSION_COOKIE_SAMESITE", "lax")),
	}
}

func (c Config) Validate() error {
	if c.TermsLoadError != "" {
		return fmt.Errorf("%s", c.TermsLoadError)
	}
	frontend, err := url.Parse(c.FrontendURL)
	if err != nil || frontend.Host == "" || (frontend.Scheme != "http" && frontend.Scheme != "https") {
		return fmt.Errorf("FRONTEND_URL must be an absolute http(s) URL")
	}
	if (frontend.Path != "" && frontend.Path != "/") || frontend.RawQuery != "" || frontend.Fragment != "" {
		return fmt.Errorf("FRONTEND_URL must be an origin without a path, query, or fragment")
	}
	if c.LineChannelID == "" || c.LineChannelSecret == "" || c.LineCallbackURL == "" {
		return fmt.Errorf("LINE_CHANNEL_ID, LINE_CHANNEL_SECRET, and LINE_CALLBACK_URL are required")
	}
	callback, err := url.Parse(c.LineCallbackURL)
	if err != nil || callback.Host == "" || (callback.Scheme != "http" && callback.Scheme != "https") {
		return fmt.Errorf("LINE_CALLBACK_URL must be an absolute http(s) URL")
	}
	if len(c.SessionSecret) < 32 {
		return fmt.Errorf("SESSION_SECRET must contain at least 32 bytes")
	}
	if c.TermsVersion == "" {
		return fmt.Errorf("TERMS_VERSION is required")
	}
	if c.CookieSameSite != "lax" && c.CookieSameSite != "strict" && c.CookieSameSite != "none" {
		return fmt.Errorf("SESSION_COOKIE_SAMESITE must be lax, strict, or none")
	}
	if (callback.Scheme == "https" || frontend.Scheme == "https" || c.CookieSameSite == "none") && !c.CookieSecure {
		return fmt.Errorf("SESSION_COOKIE_SECURE=true is required for HTTPS or SameSite=None deployments")
	}
	return nil
}
