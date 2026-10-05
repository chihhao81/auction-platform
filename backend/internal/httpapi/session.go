package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const sessionCookieName = "auction_session"
const sessionLifetime = 24 * time.Hour

func (a *App) setSession(w http.ResponseWriter, userID int64) {
	expires := time.Now().UTC().Add(sessionLifetime).Unix()
	payload := strconv.FormatInt(userID, 10) + ":" + strconv.FormatInt(expires, 10)
	mac := hmac.New(sha256.New, []byte(a.cfg.SessionSecret))
	_, _ = mac.Write([]byte(payload))
	value := payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: value, Path: "/", HttpOnly: true,
		Secure: a.cfg.CookieSecure, SameSite: a.sameSite(), MaxAge: int(sessionLifetime.Seconds()),
	})
}

func (a *App) clearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: "", Path: "/", HttpOnly: true,
		Secure: a.cfg.CookieSecure, SameSite: a.sameSite(), MaxAge: -1,
	})
}

func (a *App) sameSite() http.SameSite {
	switch a.cfg.CookieSameSite {
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteLaxMode
	}
}

func (a *App) sessionUserID(r *http.Request) (int64, error) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return 0, err
	}
	parts := strings.Split(cookie.Value, ".")
	if len(parts) != 2 {
		return 0, errors.New("invalid session")
	}
	fields := strings.Split(parts[0], ":")
	if len(fields) != 2 {
		return 0, errors.New("invalid session")
	}
	userID, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil || userID <= 0 {
		return 0, errors.New("invalid session")
	}
	expires, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil || time.Now().UTC().Unix() >= expires {
		return 0, errors.New("expired session")
	}
	provided, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return 0, errors.New("invalid session")
	}
	mac := hmac.New(sha256.New, []byte(a.cfg.SessionSecret))
	_, _ = mac.Write([]byte(parts[0]))
	if !hmac.Equal(provided, mac.Sum(nil)) {
		return 0, errors.New("invalid session")
	}
	return userID, nil
}
