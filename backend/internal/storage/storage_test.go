package storage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestValidKey(t *testing.T) {
	good := NewImageKey("auctions")
	if !ValidKey(good) {
		t.Fatalf("generated key %q should be valid", good)
	}
	for _, k := range []string{
		"", "../etc/passwd", "auctions/../../secret.jpg", "auctions/abc.jpg",
		"/auctions/" + good[9:], "auctions\\x.jpg", "Auctions/" + good[9:],
	} {
		if ValidKey(k) {
			t.Errorf("key %q should be rejected", k)
		}
	}
}

func TestLocalPutServeDelete(t *testing.T) {
	dir := t.TempDir()
	l := &Local{Dir: dir, BaseURL: "/uploads"}
	key := NewImageKey("auctions")
	if err := l.Put(context.Background(), key, []byte("jpegdata"), "image/jpeg"); err != nil {
		t.Fatal(err)
	}
	if err := l.Put(context.Background(), "../escape.jpg", []byte("x"), "image/jpeg"); err == nil {
		t.Fatal("invalid key must be rejected")
	}

	rec := httptest.NewRecorder()
	l.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, l.URL(key), nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "jpegdata" || rec.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("serve: %d %q %q", rec.Code, rec.Body.String(), rec.Header().Get("Content-Type"))
	}

	// A file outside the key format must not be served even if it exists.
	if err := os.WriteFile(filepath.Join(dir, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"/uploads/secret.txt", "/uploads/../secret.txt", "/uploads/auctions/..%2fsecret.txt"} {
		rec = httptest.NewRecorder()
		l.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404", p, rec.Code)
		}
	}

	if err := l.Delete(context.Background(), key); err != nil {
		t.Fatal(err)
	}
	if err := l.Delete(context.Background(), key); err != nil {
		t.Fatalf("deleting a missing file should be a no-op: %v", err)
	}
}
