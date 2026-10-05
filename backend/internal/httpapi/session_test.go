package httpapi

import (
	"net/http/httptest"
	"testing"
)

func TestSessionRoundTripAndTamperRejection(t *testing.T) {
	app := &App{cfg: Config{SessionSecret: "0123456789abcdef0123456789abcdef", CookieSameSite: "lax"}}
	response := httptest.NewRecorder()
	app.setSession(response, 42)
	cookie := response.Result().Cookies()[0]
	request := httptest.NewRequest("GET", "/v1/me", nil)
	request.AddCookie(cookie)

	got, err := app.sessionUserID(request)
	if err != nil || got != 42 {
		t.Fatalf("sessionUserID() = %d, %v; want 42, nil", got, err)
	}
	cookie.Value += "tampered"
	request = httptest.NewRequest("GET", "/v1/me", nil)
	request.AddCookie(cookie)
	if _, err := app.sessionUserID(request); err == nil {
		t.Fatal("tampered session should be rejected")
	}
}
