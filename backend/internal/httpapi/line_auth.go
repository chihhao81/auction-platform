package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	lineAuthorizeURL = "https://access.line.me/oauth2/v2.1/authorize"
	lineTokenURL     = "https://api.line.me/oauth2/v2.1/token"
	lineVerifyURL    = "https://api.line.me/oauth2/v2.1/verify"
)

type lineTokenResponse struct {
	AccessToken string `json:"access_token"`
	IDToken     string `json:"id_token"`
}

type lineIdentity struct {
	Issuer    string `json:"iss"`
	Subject   string `json:"sub"`
	Audience  string `json:"aud"`
	ExpiresAt int64  `json:"exp"`
	IssuedAt  int64  `json:"iat"`
	Name      string `json:"name"`
	Picture   string `json:"picture"`
	Nonce     string `json:"nonce"`
}

func (a *App) startLineLogin(w http.ResponseWriter, r *http.Request) {
	if a.cfg.LineChannelID == "" || a.cfg.LineChannelSecret == "" || a.cfg.LineCallbackURL == "" || len(a.cfg.SessionSecret) < 32 {
		writeError(w, http.StatusServiceUnavailable, "LINE Login is not configured")
		return
	}
	state, err := randomToken(32)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not start login")
		return
	}
	nonce, err := randomToken(32)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not start login")
		return
	}
	for name, value := range map[string]string{"line_oauth_state": state, "line_oauth_nonce": nonce} {
		http.SetCookie(w, &http.Cookie{
			Name: name, Value: value, Path: "/v1/auth/line/callback", HttpOnly: true,
			Secure: a.cfg.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: 600,
		})
	}
	query := url.Values{
		"response_type": {"code"}, "client_id": {a.cfg.LineChannelID}, "redirect_uri": {a.cfg.LineCallbackURL},
		"state": {state}, "scope": {"profile openid"}, "nonce": {nonce},
	}
	http.Redirect(w, r, lineAuthorizeURL+"?"+query.Encode(), http.StatusFound)
}

func (a *App) finishLineLogin(w http.ResponseWriter, r *http.Request) {
	defer clearOAuthCookie(w, "line_oauth_state", a.cfg.CookieSecure)
	defer clearOAuthCookie(w, "line_oauth_nonce", a.cfg.CookieSecure)
	if r.URL.Query().Get("error") != "" {
		http.Redirect(w, r, a.cfg.FrontendURL+"/?login=cancelled", http.StatusSeeOther)
		return
	}
	stateCookie, errState := r.Cookie("line_oauth_state")
	nonceCookie, errNonce := r.Cookie("line_oauth_nonce")
	queryState := r.URL.Query().Get("state")
	if errState != nil || errNonce != nil || queryState == "" ||
		subtle.ConstantTimeCompare([]byte(stateCookie.Value), []byte(queryState)) != 1 {
		writeError(w, http.StatusBadRequest, "invalid login state")
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" || a.cfg.LineChannelID == "" || a.cfg.LineChannelSecret == "" {
		writeError(w, http.StatusBadRequest, "invalid login response")
		return
	}
	identity, err := a.exchangeAndVerify(r.Context(), code, nonceCookie.Value)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "LINE Login could not be verified")
		return
	}
	if a.db == nil {
		writeError(w, http.StatusServiceUnavailable, "database is not configured")
		return
	}
	var userID int64
	var status string
	err = a.db.QueryRowContext(r.Context(), `
		INSERT INTO users (line_user_id, display_name, avatar_url)
		VALUES ($1, $2, $3)
		ON CONFLICT (line_user_id) DO UPDATE SET
			display_name = EXCLUDED.display_name,
			avatar_url = EXCLUDED.avatar_url,
			updated_at = now()
		WHERE users.status = 'ACTIVE'
		RETURNING id, status`, identity.Subject, identity.Name, identity.Picture).Scan(&userID, &status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusForbidden, "account is not active")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not load account")
		return
	}
	if status != "ACTIVE" {
		writeError(w, http.StatusForbidden, "account is not active")
		return
	}
	a.setSession(w, userID)
	http.Redirect(w, r, a.cfg.FrontendURL+"/", http.StatusSeeOther)
}

func (a *App) exchangeAndVerify(ctx context.Context, code, expectedNonce string) (lineIdentity, error) {
	var empty lineIdentity
	form := url.Values{
		"grant_type": {"authorization_code"}, "code": {code},
		"redirect_uri": {a.cfg.LineCallbackURL}, "client_id": {a.cfg.LineChannelID},
		"client_secret": {a.cfg.LineChannelSecret},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, lineTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return empty, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := &http.Client{Timeout: 8 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return empty, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return empty, errors.New("LINE token exchange failed")
	}
	var tokens lineTokenResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&tokens); err != nil || tokens.IDToken == "" {
		return empty, errors.New("LINE token response was invalid")
	}
	verifyForm := url.Values{"id_token": {tokens.IDToken}, "client_id": {a.cfg.LineChannelID}}
	verifyRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, lineVerifyURL, strings.NewReader(verifyForm.Encode()))
	if err != nil {
		return empty, err
	}
	verifyRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	verifyResponse, err := client.Do(verifyRequest)
	if err != nil {
		return empty, err
	}
	defer verifyResponse.Body.Close()
	if verifyResponse.StatusCode != http.StatusOK {
		return empty, errors.New("LINE ID token was rejected")
	}
	var identity lineIdentity
	if err := json.NewDecoder(io.LimitReader(verifyResponse.Body, 1<<20)).Decode(&identity); err != nil {
		return empty, errors.New("LINE ID token response was invalid")
	}
	if err := validateLineIdentity(identity, a.cfg.LineChannelID, expectedNonce, time.Now().UTC()); err != nil {
		return empty, err
	}
	return identity, nil
}

func validateLineIdentity(identity lineIdentity, channelID, expectedNonce string, now time.Time) error {
	if identity.Issuer != "https://access.line.me" || identity.Subject == "" || identity.Audience != channelID ||
		identity.ExpiresAt <= now.Unix() || identity.IssuedAt <= 0 || identity.IssuedAt > now.Add(time.Minute).Unix() ||
		identity.Nonce == "" || subtle.ConstantTimeCompare([]byte(identity.Nonce), []byte(expectedNonce)) != 1 {
		return errors.New("LINE ID token claims did not match")
	}
	return nil
}

func randomToken(bytes int) (string, error) {
	value := make([]byte, bytes)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func clearOAuthCookie(w http.ResponseWriter, name string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: "", Path: "/v1/auth/line/callback", HttpOnly: true,
		Secure: secure, SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
}
