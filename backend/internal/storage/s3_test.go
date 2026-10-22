package storage

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeS3 records the requests the client makes.
type fakeS3 struct {
	mu   sync.Mutex
	reqs []string
	body []byte
	hdr  http.Header
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reqs = append(f.reqs, r.Method+" "+r.URL.Path)
	if r.Method == http.MethodPut {
		f.body, _ = io.ReadAll(r.Body)
		f.hdr = r.Header.Clone()
		w.Header().Set("ETag", `"abc"`)
	}
	if r.Method == http.MethodDelete {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func TestS3PutDeleteURL(t *testing.T) {
	fake := &fakeS3{}
	srv := httptest.NewServer(fake)
	defer srv.Close()

	s, err := NewS3(S3Config{
		Endpoint: strings.TrimPrefix(srv.URL, "http://"), AccessKeyID: "ak", SecretAccessKey: "sk",
		Bucket: "critter", PublicBaseURL: "https://img.example.com/", Insecure: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	key := NewImageKey("auctions")
	if err := s.Put(context.Background(), key, []byte("jpeg"), "image/jpeg"); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(context.Background(), key); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(context.Background(), "../../etc/passwd", []byte("x"), "image/jpeg"); err == nil {
		t.Fatal("invalid key must be rejected before any request")
	}

	want := []string{"PUT /critter/" + key, "DELETE /critter/" + key}
	if strings.Join(fake.reqs, ",") != strings.Join(want, ",") {
		t.Fatalf("requests %v, want %v", fake.reqs, want)
	}
	if string(fake.body) != "jpeg" || fake.hdr.Get("Content-Type") != "image/jpeg" ||
		!strings.Contains(fake.hdr.Get("Cache-Control"), "immutable") || !strings.HasPrefix(fake.hdr.Get("Authorization"), "AWS4-HMAC-SHA256") {
		t.Fatalf("upload headers %v body %q", fake.hdr, fake.body)
	}
	if got := s.URL(key); got != "https://img.example.com/"+key {
		t.Fatalf("URL = %s", got)
	}
	if _, err := NewS3(S3Config{Endpoint: "x"}); err == nil {
		t.Fatal("incomplete config must fail")
	}
}
