// Package storage stores uploaded objects. Local disk is used in development;
// an S3-compatible backend (Cloudflare R2) can implement the same interface
// for production so images do not fill the VM's disk.
package storage

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

type Storage interface {
	Put(ctx context.Context, key string, data []byte, contentType string) error
	Delete(ctx context.Context, key string) error
	// URL returns the public URL path or absolute URL for key.
	URL(key string) string
}

// keyRe matches every key this application generates. Anything else is
// rejected, which rules out path traversal through crafted keys.
var keyRe = regexp.MustCompile(`^[a-z]+/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\.jpg$`)

// NewImageKey returns a fresh random key under prefix. User supplied file
// names are never used.
func NewImageKey(prefix string) string {
	return prefix + "/" + uuid.NewString() + ".jpg"
}

func ValidKey(key string) bool { return keyRe.MatchString(key) }

// Local stores objects under Dir and serves them under BaseURL.
type Local struct {
	Dir     string
	BaseURL string // e.g. "/uploads"
}

func (l *Local) path(key string) (string, error) {
	if !ValidKey(key) {
		return "", fmt.Errorf("invalid storage key %q", key)
	}
	return filepath.Join(l.Dir, filepath.FromSlash(key)), nil
}

func (l *Local) Put(_ context.Context, key string, data []byte, _ string) error {
	p, err := l.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

func (l *Local) Delete(_ context.Context, key string) error {
	p, err := l.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (l *Local) URL(key string) string { return l.BaseURL + "/" + key }

// Handler serves stored images. Only well-formed keys are served, always as
// image/jpeg, so the endpoint cannot be used to read arbitrary files.
func (l *Local) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.URL.Path, l.BaseURL+"/")
		p, err := l.path(key)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		f, err := os.Open(p)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		st, err := f.Stat()
		if err != nil || st.IsDir() {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		http.ServeContent(w, r, "", st.ModTime(), f)
	})
}
