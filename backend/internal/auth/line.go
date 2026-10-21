package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Profile is the LINE identity returned after a successful login.
type Profile struct {
	UserID      string
	DisplayName string
	PictureURL  string
}

// LineClient performs the server side of the LINE Login OAuth2 code flow.
type LineClient interface {
	// AuthURL is where the browser is sent to start login.
	AuthURL(state string) string
	// Exchange trades an authorization code for the user's profile. The LINE
	// access token is used once and never stored.
	Exchange(ctx context.Context, code string) (Profile, error)
}

// HTTPLineClient talks to the real LINE Login API.
type HTTPLineClient struct {
	ChannelID     string
	ChannelSecret string
	RedirectURI   string
	HTTP          *http.Client
	// Base URLs are overridable for tests.
	AuthBase string
	APIBase  string
}

func NewHTTPLineClient(channelID, channelSecret, redirectURI string) *HTTPLineClient {
	return &HTTPLineClient{
		ChannelID: channelID, ChannelSecret: channelSecret, RedirectURI: redirectURI,
		HTTP:     &http.Client{Timeout: 10 * time.Second},
		AuthBase: "https://access.line.me",
		APIBase:  "https://api.line.me",
	}
}

func (c *HTTPLineClient) AuthURL(state string) string {
	q := url.Values{
		"response_type": {"code"},
		"client_id":     {c.ChannelID},
		"redirect_uri":  {c.RedirectURI},
		"state":         {state},
		"scope":         {"profile openid"},
	}
	return c.AuthBase + "/oauth2/v2.1/authorize?" + q.Encode()
}

func (c *HTTPLineClient) Exchange(ctx context.Context, code string) (Profile, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {c.RedirectURI},
		"client_id":     {c.ChannelID},
		"client_secret": {c.ChannelSecret},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.APIBase+"/oauth2/v2.1/token", strings.NewReader(form.Encode()))
	if err != nil {
		return Profile{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := c.doJSON(req, &tok); err != nil {
		return Profile{}, fmt.Errorf("line token exchange: %w", err)
	}
	if tok.AccessToken == "" {
		return Profile{}, errors.New("line token exchange: empty access token")
	}

	req, err = http.NewRequestWithContext(ctx, http.MethodGet, c.APIBase+"/v2/profile", nil)
	if err != nil {
		return Profile{}, err
	}
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	var p struct {
		UserID      string `json:"userId"`
		DisplayName string `json:"displayName"`
		PictureURL  string `json:"pictureUrl"`
	}
	if err := c.doJSON(req, &p); err != nil {
		return Profile{}, fmt.Errorf("line profile: %w", err)
	}
	if p.UserID == "" {
		return Profile{}, errors.New("line profile: empty userId")
	}
	return Profile{UserID: p.UserID, DisplayName: p.DisplayName, PictureURL: p.PictureURL}, nil
}

func (c *HTTPLineClient) doJSON(req *http.Request, dst any) error {
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body := io.LimitReader(resp.Body, 64<<10)
	if resp.StatusCode != http.StatusOK {
		// Do not include the body: error responses may echo request details.
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return json.NewDecoder(body).Decode(dst)
}

// MockLineClient accepts codes of the form "mock:<name>" and returns a fake
// profile. It lets local development and tests run without a LINE channel.
// config.Load refuses to enable it in production.
type MockLineClient struct {
	FrontendURL string
}

var mockNameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,32}$`)

// MockCode builds the code a mock login for name exchanges.
func MockCode(name string) (string, bool) {
	if !mockNameRe.MatchString(name) {
		return "", false
	}
	return "mock:" + name, true
}

func (m MockLineClient) AuthURL(state string) string {
	code, _ := MockCode("dev-user")
	return m.FrontendURL + "/api/auth/line/callback?" + url.Values{"code": {code}, "state": {state}}.Encode()
}

func (m MockLineClient) Exchange(_ context.Context, code string) (Profile, error) {
	name, ok := strings.CutPrefix(code, "mock:")
	if !ok || !mockNameRe.MatchString(name) {
		return Profile{}, errors.New("invalid mock code")
	}
	return Profile{UserID: "mock-" + name, DisplayName: "Mock " + name}, nil
}
