package imageproc

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"

	"auction-platform/backend/internal/apperr"
)

func encode(t *testing.T, format string, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		img.Set(x, 0, color.NRGBA{R: 200, G: 100, B: 50, A: 0}) // transparent row
	}
	var buf bytes.Buffer
	var err error
	switch format {
	case "png":
		err = png.Encode(&buf, img)
	case "jpeg":
		err = jpeg.Encode(&buf, img, nil)
	case "gif":
		err = gif.Encode(&buf, img, nil)
	}
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestProcessAcceptsAndReencodes(t *testing.T) {
	for _, tc := range []struct{ format, name string }{{"png", "a.PNG"}, {"jpeg", "b.jpg"}, {"jpeg", "c.jpeg"}} {
		res, err := Process(encode(t, tc.format, 10, 6), tc.name)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if res.Width != 10 || res.Height != 6 {
			t.Fatalf("%s: size %dx%d", tc.name, res.Width, res.Height)
		}
		if _, f, err := image.DecodeConfig(bytes.NewReader(res.Data)); err != nil || f != "jpeg" {
			t.Fatalf("%s: output is not jpeg (%s, %v)", tc.name, f, err)
		}
	}
}

func TestProcessFlattensTransparencyToWhite(t *testing.T) {
	res, err := Process(encode(t, "png", 4, 4), "a.png")
	if err != nil {
		t.Fatal(err)
	}
	img, err := jpeg.Decode(bytes.NewReader(res.Data))
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := img.At(1, 0).RGBA()
	if r>>8 < 240 || g>>8 < 240 || b>>8 < 240 {
		t.Fatalf("transparent pixel should be white, got %d,%d,%d", r>>8, g>>8, b>>8)
	}
}

func TestProcessDownscalesLargeImages(t *testing.T) {
	res, err := Process(encode(t, "png", 3000, 1500), "big.png")
	if err != nil {
		t.Fatal(err)
	}
	if res.Width != 2048 || res.Height != 1024 {
		t.Fatalf("size %dx%d, want 2048x1024", res.Width, res.Height)
	}
}

func TestProcessRejects(t *testing.T) {
	cases := []struct {
		name     string
		data     []byte
		filename string
		code     string
	}{
		{"empty", nil, "a.png", "IMAGE_TOO_LARGE"},
		{"too large", make([]byte, MaxFileBytes+1), "a.png", "IMAGE_TOO_LARGE"},
		{"gif not allowed", encode(t, "gif", 2, 2), "a.gif", "INVALID_IMAGE"},
		{"html disguised as png", []byte("<html><script>alert(1)</script></html>"), "a.png", "INVALID_IMAGE"},
		{"extension mismatch", encode(t, "png", 2, 2), "a.jpg", "IMAGE_EXTENSION_MISMATCH"},
		{"no extension", encode(t, "png", 2, 2), "image", "IMAGE_EXTENSION_MISMATCH"},
		{"executable extension", encode(t, "png", 2, 2), "a.php", "IMAGE_EXTENSION_MISMATCH"},
		{"truncated png", encode(t, "png", 20, 20)[:40], "a.png", "INVALID_IMAGE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Process(tc.data, tc.filename)
			if !apperr.IsCode(err, tc.code) {
				t.Fatalf("got %v, want %s", err, tc.code)
			}
		})
	}
}
