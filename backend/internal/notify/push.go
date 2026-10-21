package notify

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/google/uuid"

	"auction-platform/backend/internal/database/db"
)

// pushHostSuffixes are the browser push services we deliver to. Restricting
// endpoints prevents the server from being used to POST to arbitrary URLs
// (SSRF) through a crafted subscription.
var pushHostSuffixes = []string{
	"fcm.googleapis.com",        // Chrome, Edge (Chromium), Android
	"push.services.mozilla.com", // Firefox
	"notify.windows.com",        // legacy Edge / Windows
	"push.apple.com",            // Safari
}

// ValidEndpoint reports whether endpoint is an https URL on a known push
// service.
func ValidEndpoint(endpoint string) bool {
	if len(endpoint) > 2048 {
		return false
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, s := range pushHostSuffixes {
		if host == s || strings.HasSuffix(host, "."+s) {
			return true
		}
	}
	return false
}

// PushStore is the data the pusher needs.
type PushStore interface {
	IsPushEnabled(ctx context.Context, arg db.IsPushEnabledParams) (bool, error)
	ListPushSubscriptions(ctx context.Context, userID uuid.UUID) ([]db.PushSubscription, error)
	DeletePushSubscriptionByID(ctx context.Context, id uuid.UUID) error
}

// SendFunc sends one push message and returns the push service status code.
type SendFunc func(ctx context.Context, payload []byte, sub db.PushSubscription) (int, error)

// VAPIDSender returns a SendFunc using the given VAPID keys.
func VAPIDSender(publicKey, privateKey, subject string) SendFunc {
	client := &http.Client{Timeout: 10 * time.Second}
	return func(ctx context.Context, payload []byte, sub db.PushSubscription) (int, error) {
		resp, err := webpush.SendNotificationWithContext(ctx, payload, &webpush.Subscription{
			Endpoint: sub.Endpoint,
			Keys:     webpush.Keys{P256dh: sub.P256dh, Auth: sub.Auth},
		}, &webpush.Options{
			HTTPClient:      client,
			Subscriber:      subject,
			VAPIDPublicKey:  publicKey,
			VAPIDPrivateKey: privateKey,
			TTL:             24 * 3600,
			Urgency:         webpush.UrgencyNormal,
		})
		if err != nil {
			return 0, err
		}
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return resp.StatusCode, nil
	}
}

// Pusher sends notifications as Web Push in background goroutines after the
// business transaction has committed. Failures are logged and never affect
// the caller. Concurrency is bounded to keep resource use small.
type Pusher struct {
	store PushStore
	send  SendFunc
	sem   chan struct{}
	wg    sync.WaitGroup
}

func NewPusher(store PushStore, send SendFunc) *Pusher {
	return &Pusher{store: store, send: send, sem: make(chan struct{}, 4)}
}

type pushPayload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url"`
	Tag   string `json:"tag"`
}

// Dispatch queues delivery of notifications and returns immediately.
func (p *Pusher) Dispatch(notifications []db.Notification) {
	for _, n := range notifications {
		p.wg.Add(1)
		go func(n db.Notification) {
			defer p.wg.Done()
			p.sem <- struct{}{}
			defer func() { <-p.sem }()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			p.deliver(ctx, n)
		}(n)
	}
}

// Wait blocks until in-flight deliveries finish or ctx is done (shutdown).
func (p *Pusher) Wait(ctx context.Context) {
	done := make(chan struct{})
	go func() { p.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

func (p *Pusher) deliver(ctx context.Context, n db.Notification) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("push panic", "notification", n.ID, "panic", r)
		}
	}()
	enabled, err := p.store.IsPushEnabled(ctx, db.IsPushEnabledParams{UserID: n.UserID, Type: n.Type})
	if err != nil {
		slog.Warn("push: load preference", "user", n.UserID, "err", err)
		return
	}
	if !enabled {
		return
	}
	subs, err := p.store.ListPushSubscriptions(ctx, n.UserID)
	if err != nil {
		slog.Warn("push: list subscriptions", "user", n.UserID, "err", err)
		return
	}
	payload, _ := json.Marshal(pushPayload{Title: n.Title, Body: n.Content, URL: Link(n), Tag: n.ID.String()})
	for _, sub := range subs {
		if !ValidEndpoint(sub.Endpoint) {
			continue
		}
		status, err := p.send(ctx, payload, sub)
		switch {
		case err != nil:
			slog.Warn("push: send failed", "subscription", sub.ID, "err", err)
		case status == http.StatusNotFound || status == http.StatusGone:
			// The browser unsubscribed or the subscription expired.
			if err := p.store.DeletePushSubscriptionByID(ctx, sub.ID); err != nil {
				slog.Warn("push: delete expired subscription", "subscription", sub.ID, "err", err)
			} else {
				slog.Info("push: removed expired subscription", "subscription", sub.ID, "status", status)
			}
		case status >= 400:
			slog.Warn("push: rejected", "subscription", sub.ID, "status", status)
		}
	}
}
