// Package imageproc validates uploaded images and re-encodes them.
//
// Re-encoding to JPEG guarantees the stored file is a plain image (no
// polyglot payloads), strips EXIF metadata such as GPS location, applies the
// EXIF orientation and bounds the stored size.
package imageproc

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png" // register decoder
	"net/http"
	"path/filepath"
	"strings"

	"github.com/disintegration/imaging"
	_ "golang.org/x/image/webp" // register decoder

	"auction-platform/backend/internal/apperr"
)

const (
	MaxFileBytes = 8 << 20 // 8 MiB per uploaded file
	maxPixels    = 30_000_000
	maxSide      = 2048 // longest side after resizing
	jpegQuality  = 85
)

var allowed = map[string][]string{
	"image/jpeg": {".jpg", ".jpeg"},
	"image/png":  {".png"},
	"image/webp": {".webp"},
}

var ErrInvalidImage = apperr.Invalid("INVALID_IMAGE", "圖片格式不支援，僅接受 JPG、PNG、WebP")

// Decoding a large image needs width*height*4 bytes. One image at a time keeps
// peak memory bounded on a 1 GB VM.
var sem = make(chan struct{}, 1)

type Result struct {
	Data          []byte
	Width, Height int
}

// Process validates data (with the client supplied filename used only for its
// extension) and returns a re-encoded JPEG.
func Process(data []byte, filename string) (Result, error) {
	if len(data) == 0 || len(data) > MaxFileBytes {
		return Result{}, apperr.Invalid("IMAGE_TOO_LARGE", "每張圖片不可超過 8MB")
	}
	// The MIME type is sniffed from the content; the client's Content-Type
	// header is ignored.
	mime := http.DetectContentType(data)
	exts, ok := allowed[mime]
	if !ok {
		return Result{}, ErrInvalidImage
	}
	ext := strings.ToLower(filepath.Ext(filename))
	extOK := false
	for _, e := range exts {
		extOK = extOK || e == ext
	}
	if !extOK {
		return Result{}, apperr.Invalid("IMAGE_EXTENSION_MISMATCH", "副檔名與圖片格式不符")
	}

	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || "image/"+format != mime {
		return Result{}, ErrInvalidImage
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > maxPixels {
		return Result{}, apperr.Invalid("IMAGE_DIMENSIONS", "圖片解析度過大")
	}

	sem <- struct{}{}
	defer func() { <-sem }()

	img, err := imaging.Decode(bytes.NewReader(data), imaging.AutoOrientation(true))
	if err != nil {
		return Result{}, ErrInvalidImage
	}
	b := img.Bounds()
	if b.Dx() > maxSide || b.Dy() > maxSide {
		img = imaging.Fit(img, maxSide, maxSide, imaging.Lanczos)
	}
	if mime != "image/jpeg" {
		// JPEG has no alpha channel: flatten transparency onto white.
		bg := imaging.New(img.Bounds().Dx(), img.Bounds().Dy(), color.White)
		img = imaging.Overlay(bg, img, image.Pt(0, 0), 1.0)
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return Result{}, err
	}
	b = img.Bounds()
	return Result{Data: buf.Bytes(), Width: b.Dx(), Height: b.Dy()}, nil
}
