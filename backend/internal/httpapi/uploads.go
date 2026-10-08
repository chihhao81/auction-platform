package httpapi

import (
	"bytes"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	maxAuctionImageBytes = 4 << 20
	maxPendingImages     = 10
	maxPendingImageBytes = 50 << 20
)

func imageFormatForExtension(name string) (string, bool) {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg":
		return "jpeg", true
	case ".png":
		return "png", true
	case ".gif":
		return "gif", true
	default:
		return "", false
	}
}

func inspectAuctionImage(name string, content []byte) (string, error) {
	if len(content) == 0 || len(content) > maxAuctionImageBytes {
		return "", errors.New("image must be between 1 byte and 4 MB")
	}
	format, ok := imageFormatForExtension(name)
	if !ok {
		return "", errors.New("only JPEG, PNG, and GIF images are supported")
	}
	expectedMIME := map[string]string{"jpeg": "image/jpeg", "png": "image/png", "gif": "image/gif"}[format]
	actualMIME := http.DetectContentType(content[:min(len(content), 512)])
	if actualMIME != expectedMIME {
		return "", errors.New("image extension and file content do not match")
	}
	config, decodedFormat, err := image.DecodeConfig(bytes.NewReader(content))
	if err != nil || decodedFormat != format || config.Width <= 0 || config.Height <= 0 || config.Width > 10000 || config.Height > 10000 || int64(config.Width)*int64(config.Height) > 20_000_000 {
		return "", errors.New("invalid or excessively large image dimensions")
	}
	return expectedMIME, nil
}

func (a *App) uploadAuctionImage(w http.ResponseWriter, r *http.Request) {
	if !a.validOrigin(r) {
		writeError(w, http.StatusForbidden, "origin not allowed")
		return
	}
	userID, ok := a.requireAuctionUser(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxAuctionImageBytes+(256<<10))
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid image upload")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	if r.MultipartForm == nil || len(r.MultipartForm.Value) != 0 || len(r.MultipartForm.File) != 1 || len(r.MultipartForm.File["image"]) != 1 {
		writeError(w, http.StatusBadRequest, "upload exactly one image in the image field")
		return
	}
	file, header, err := r.FormFile("image")
	if err != nil {
		writeError(w, http.StatusBadRequest, "image file is required")
		return
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maxAuctionImageBytes+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid image file")
		return
	}
	expectedMIME, err := inspectAuctionImage(header.Filename, content)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save image")
		return
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(r.Context(), `SELECT pg_advisory_xact_lock($1)`, userID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not save image")
		return
	}
	var pendingCount, pendingBytes int64
	if err := tx.QueryRowContext(r.Context(), `SELECT count(*), COALESCE(sum(byte_size),0) FROM auction_image_uploads WHERE owner_id=$1 AND auction_id IS NULL`, userID).Scan(&pendingCount, &pendingBytes); err != nil {
		writeError(w, http.StatusInternalServerError, "could not check image quota")
		return
	}
	if pendingCount >= maxPendingImages || pendingBytes+int64(len(content)) > maxPendingImageBytes {
		writeError(w, http.StatusConflict, "too many unlisted images; finish or remove an existing draft first")
		return
	}
	var keyBytes [32]byte
	if _, err := rand.Read(keyBytes[:]); err != nil {
		writeError(w, http.StatusInternalServerError, "could not save image")
		return
	}
	id := hex.EncodeToString(keyBytes[:])
	if err := os.MkdirAll(a.uploadDir, 0700); err != nil {
		writeError(w, http.StatusInternalServerError, "image storage is unavailable")
		return
	}
	path := filepath.Join(a.uploadDir, id)
	stored, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save image")
		return
	}
	if _, err = stored.Write(content); err == nil {
		err = stored.Sync()
	}
	closeErr := stored.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(path)
		writeError(w, http.StatusInternalServerError, "could not save image")
		return
	}
	if _, err = tx.ExecContext(r.Context(), `INSERT INTO auction_image_uploads (id,owner_id,mime_type,byte_size) VALUES ($1,$2,$3,$4)`, id, userID, expectedMIME, len(content)); err != nil {
		_ = os.Remove(path)
		writeError(w, http.StatusInternalServerError, "could not save image")
		return
	}
	if err = tx.Commit(); err != nil {
		_ = os.Remove(path)
		writeError(w, http.StatusInternalServerError, "could not save image")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id, "url": "/v1/uploads/" + id})
}

func (a *App) serveAuctionImage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validUploadID(id) || a.db == nil {
		http.NotFound(w, r)
		return
	}
	var contentType string
	err := a.db.QueryRowContext(r.Context(), `SELECT u.mime_type FROM auction_image_uploads u JOIN auctions a ON a.id=u.auction_id WHERE u.id=$1 AND now() < a.ends_at + interval '24 hours'`, id).Scan(&contentType)
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load image")
		return
	}
	file, err := os.Open(filepath.Join(a.uploadDir, id))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", `inline; filename="auction-image"`)
	w.Header().Set("Cache-Control", "public, max-age=3600, immutable")
	http.ServeContent(w, r, "auction-image", info.ModTime(), file)
}

func (a *App) deleteAuctionImage(w http.ResponseWriter, r *http.Request) {
	if !a.validOrigin(r) {
		writeError(w, http.StatusForbidden, "origin not allowed")
		return
	}
	userID, ok := a.requireAuctionUser(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !validUploadID(id) {
		writeError(w, http.StatusBadRequest, "invalid image id")
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete image")
		return
	}
	defer tx.Rollback()
	var ownerID int64
	var auctionID sql.NullInt64
	err = tx.QueryRowContext(r.Context(), `SELECT owner_id,auction_id FROM auction_image_uploads WHERE id=$1 FOR UPDATE`, id).Scan(&ownerID, &auctionID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "image not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete image")
		return
	}
	if ownerID != userID {
		writeError(w, http.StatusForbidden, "image belongs to another user")
		return
	}
	if auctionID.Valid {
		writeError(w, http.StatusConflict, "remove the image from its upcoming auction before deleting it")
		return
	}
	if _, err = tx.ExecContext(r.Context(), `DELETE FROM auction_image_uploads WHERE id=$1`, id); err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete image")
		return
	}
	if err = tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete image")
		return
	}
	if err = os.Remove(filepath.Join(a.uploadDir, id)); err != nil && !errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusInternalServerError, "image record deleted but file cleanup failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
