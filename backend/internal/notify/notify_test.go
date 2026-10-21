package notify

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"

	"auction-platform/backend/internal/database/db"
)

func TestFormatPrice(t *testing.T) {
	cases := map[int64]string{0: "NT$ 0", 50: "NT$ 50", 1000: "NT$ 1,000", 1234567: "NT$ 1,234,567", -1500: "NT$ -1,500"}
	for in, want := range cases {
		if got := FormatPrice(in); got != want {
			t.Errorf("FormatPrice(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestValidEndpoint(t *testing.T) {
	ok := []string{
		"https://fcm.googleapis.com/fcm/send/abc",
		"https://updates.push.services.mozilla.com/wpush/v2/abc",
		"https://web.push.apple.com/abc",
		"https://wns2-par02p.notify.windows.com/w/?token=abc",
	}
	bad := []string{
		"http://fcm.googleapis.com/fcm/send/abc", // not https
		"https://evil.com/fcm.googleapis.com",
		"https://fcm.googleapis.com.evil.com/x",
		"https://evilfcm.googleapis.com.attacker.net/x",
		"https://169.254.169.254/latest/meta-data",
		"https://localhost/x",
		"https://user:pw@fcm.googleapis.com/x",
		"https://fcm.googleapis.com:8443/x",
		"not a url",
	}
	for _, e := range ok {
		if !ValidEndpoint(e) {
			t.Errorf("%s should be valid", e)
		}
	}
	for _, e := range bad {
		if ValidEndpoint(e) {
			t.Errorf("%s should be rejected", e)
		}
	}
}

func TestValidKey(t *testing.T) {
	p256 := base64.RawURLEncoding.EncodeToString(make([]byte, 65))
	auth := base64.URLEncoding.EncodeToString(make([]byte, 16)) // padded is fine
	if !validKey(p256, 65) || !validKey(auth, 16) {
		t.Fatal("valid keys rejected")
	}
	if validKey(p256, 16) || validKey("!!!", 16) || validKey("", 16) {
		t.Fatal("invalid keys accepted")
	}
}

func TestLink(t *testing.T) {
	a, o := uuid.New(), uuid.New()
	if got := Link(db.Notification{RelatedAuctionID: uuid.NullUUID{UUID: a, Valid: true}}); got != "/auctions/"+a.String() {
		t.Errorf("auction link %s", got)
	}
	both := db.Notification{RelatedAuctionID: uuid.NullUUID{UUID: a, Valid: true}, RelatedOrderID: uuid.NullUUID{UUID: o, Valid: true}}
	if got := Link(both); got != "/orders/"+o.String() {
		t.Errorf("order link should win: %s", got)
	}
	if got := Link(db.Notification{}); got != "/notifications" {
		t.Errorf("default link %s", got)
	}
}

type fakePushStore struct {
	mu      sync.Mutex
	enabled bool
	subs    []db.PushSubscription
	deleted []uuid.UUID
}

func (f *fakePushStore) IsPushEnabled(context.Context, db.IsPushEnabledParams) (bool, error) {
	return f.enabled, nil
}

func (f *fakePushStore) ListPushSubscriptions(context.Context, uuid.UUID) ([]db.PushSubscription, error) {
	return f.subs, nil
}

func (f *fakePushStore) DeletePushSubscriptionByID(_ context.Context, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, id)
	return nil
}

func TestPusherDeliversAndRemovesExpiredSubscriptions(t *testing.T) {
	live := db.PushSubscription{ID: uuid.New(), Endpoint: "https://fcm.googleapis.com/fcm/send/live"}
	gone := db.PushSubscription{ID: uuid.New(), Endpoint: "https://fcm.googleapis.com/fcm/send/gone"}
	broken := db.PushSubscription{ID: uuid.New(), Endpoint: "https://fcm.googleapis.com/fcm/send/broken"}
	ssrf := db.PushSubscription{ID: uuid.New(), Endpoint: "https://internal.example/x"}
	st := &fakePushStore{enabled: true, subs: []db.PushSubscription{live, gone, broken, ssrf}}

	var mu sync.Mutex
	var sentTo []string
	var payload pushPayload
	p := NewPusher(st, func(_ context.Context, body []byte, sub db.PushSubscription) (int, error) {
		mu.Lock()
		defer mu.Unlock()
		sentTo = append(sentTo, sub.Endpoint)
		_ = json.Unmarshal(body, &payload)
		switch sub.ID {
		case gone.ID:
			return 410, nil
		case broken.ID:
			return 0, errors.New("network down")
		}
		return 201, nil
	})
	auction := uuid.New()
	p.Dispatch([]db.Notification{{ID: uuid.New(), UserID: uuid.New(), Type: "OUTBID", Title: "T", Content: "C",
		RelatedAuctionID: uuid.NullUUID{UUID: auction, Valid: true}}})
	p.Wait(context.Background())

	if len(sentTo) != 3 {
		t.Fatalf("should send to the 3 valid endpoints only, sent to %v", sentTo)
	}
	if len(st.deleted) != 1 || st.deleted[0] != gone.ID {
		t.Fatalf("only the 410 subscription should be deleted: %v", st.deleted)
	}
	if payload.Title != "T" || payload.Body != "C" || payload.URL != "/auctions/"+auction.String() {
		t.Fatalf("payload %+v", payload)
	}
}

func TestPusherRespectsOptOut(t *testing.T) {
	st := &fakePushStore{enabled: false, subs: []db.PushSubscription{{ID: uuid.New(), Endpoint: "https://fcm.googleapis.com/x"}}}
	called := false
	p := NewPusher(st, func(context.Context, []byte, db.PushSubscription) (int, error) { called = true; return 201, nil })
	p.Dispatch([]db.Notification{{ID: uuid.New(), UserID: uuid.New(), Type: "OUTBID"}})
	p.Wait(context.Background())
	if called {
		t.Fatal("push must not be sent when the user disabled the type")
	}
}

func TestValidType(t *testing.T) {
	if !ValidType(TypeOutbid) || ValidType("NOPE") {
		t.Fatal("ValidType wrong")
	}
}
