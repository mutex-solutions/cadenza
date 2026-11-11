package prepare

import (
	"bytes"
	"context"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"
	"time"

	"github.com/gen2brain/webp"

	"github.com/mutex-solutions/cadenza/internal/media"
)

// testImage is a w x h gradient, so encoders have real content to work on.
func testImage(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), uint8(x ^ y), 255})
		}
	}
	return img
}

func encodePNG(t testing.TB, img image.Image) []byte {
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func encodeTestWebP(t testing.TB, img image.Image) []byte {
	var b bytes.Buffer
	if err := webp.Encode(&b, img, webp.Options{Quality: 80, Method: 1}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// webpSize checks b is a WebP file and returns its dimensions.
func webpSize(t *testing.T, b []byte) (int, int) {
	t.Helper()
	if len(b) < 12 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WEBP" {
		t.Fatalf("not WebP: % x", b[:min(12, len(b))])
	}
	cfg, err := webp.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return cfg.Width, cfg.Height
}

// small limits keep encoding fast; the defaults are tested once below.
var smallOpts = Options{WebPMethod: 1, Limits: Limits{ImageSide: 200, PreviewSide: 50}}

func TestImageResize(t *testing.T) {
	path := writeFile(t, "shot.png", encodePNG(t, testImage(400, 100)))
	p, err := PrepareFile(context.Background(), path, smallOpts)
	if err != nil {
		t.Fatal(err)
	}
	main := checkRef(t, p)
	if w, h := webpSize(t, main); w != 200 || h != 50 {
		t.Fatalf("main is %dx%d, want 200x50", w, h)
	}
	if p.Source != media.SourceRaster || p.File.Type != media.TypeWebP || p.File.Name != "shot.webp" {
		t.Fatalf("got %+v", p.File)
	}
	if p.Preview == nil || p.File.PreviewSHA256 != p.Preview.File.SHA256 || p.Preview.File.SHA256 != hashBytes(p.Preview.Data) ||
		p.Preview.File.Size != int64(len(p.Preview.Data)) || p.Preview.File.Type != media.TypeWebP {
		t.Fatalf("bad preview %+v", p.Preview)
	}
	if w, h := webpSize(t, p.Preview.Data); w != 50 || h != 12 {
		t.Fatalf("preview is %dx%d, want 50x12", w, h)
	}
}

func TestImagePortraitAndNoUpscale(t *testing.T) {
	var jb bytes.Buffer
	if err := jpeg.Encode(&jb, testImage(30, 60), nil); err != nil {
		t.Fatal(err)
	}
	p, err := PrepareFile(context.Background(), writeFile(t, "small.jpg", jb.Bytes()), smallOpts)
	if err != nil {
		t.Fatal(err)
	}
	if w, h := webpSize(t, checkRef(t, p)); w != 30 || h != 60 {
		t.Fatalf("small image changed size: %dx%d", w, h)
	}
	if w, h := webpSize(t, p.Preview.Data); w != 25 || h != 50 {
		t.Fatalf("portrait preview is %dx%d, want 25x50", w, h)
	}
}

func TestImageGIFFirstFrame(t *testing.T) {
	pal := image.NewPaletted(image.Rect(0, 0, 40, 20), color.Palette{color.Black, color.White})
	var gb bytes.Buffer
	if err := gif.EncodeAll(&gb, &gif.GIF{Image: []*image.Paletted{pal, pal}, Delay: []int{1, 1}}); err != nil {
		t.Fatal(err)
	}
	p, err := PrepareFile(context.Background(), writeFile(t, "anim.gif", gb.Bytes()), smallOpts)
	if err != nil {
		t.Fatal(err)
	}
	if w, h := webpSize(t, checkRef(t, p)); w != 40 || h != 20 {
		t.Fatalf("gif became %dx%d", w, h)
	}
}

func TestWebPPassThrough(t *testing.T) {
	orig := encodeTestWebP(t, testImage(120, 80))
	p, err := PrepareFile(context.Background(), writeFile(t, "ready.webp", orig), smallOpts)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(checkRef(t, p), orig) || p.Source != media.SourceWebP {
		t.Fatal("small WebP was re-encoded")
	}
	if w, h := webpSize(t, p.Preview.Data); w != 50 || h != 33 {
		t.Fatalf("preview is %dx%d", w, h)
	}
}

func TestWebPTooLargeIsResized(t *testing.T) {
	orig := encodeTestWebP(t, testImage(300, 150))
	p, err := PrepareFile(context.Background(), writeFile(t, "big.webp", orig), smallOpts)
	if err != nil {
		t.Fatal(err)
	}
	if w, h := webpSize(t, checkRef(t, p)); w != 200 || h != 100 {
		t.Fatalf("got %dx%d", w, h)
	}
}

func TestCorruptImage(t *testing.T) {
	_, err := PrepareFile(context.Background(), writeFile(t, "bad.png", []byte("\x89PNG\r\n\x1a\ngarbage")), smallOpts)
	wantInvalid(t, err)
}

func TestPixelBombRefused(t *testing.T) {
	b := encodePNG(t, testImage(1, 1))
	// IHDR data starts at 16; rewrite width and height, then fix the CRC.
	binary.BigEndian.PutUint32(b[16:], 20000)
	binary.BigEndian.PutUint32(b[20:], 20000)
	binary.BigEndian.PutUint32(b[29:], crc32.ChecksumIEEE(b[12:29]))
	_, err := PrepareFile(context.Background(), writeFile(t, "bomb.png", b), smallOpts)
	if e := wantInvalid(t, err); e.Fix == "" {
		t.Fatal("missing fix")
	}
}

func TestFitBounds(t *testing.T) {
	cases := []struct{ w, h, side, ww, wh int }{
		{5000, 2500, 2560, 2560, 1280},
		{2500, 5000, 2560, 1280, 2560},
		{2560, 100, 2560, 2560, 100},
		{10, 10, 640, 10, 10},
		{5000, 1, 640, 640, 1},
	}
	for _, c := range cases {
		r := fit(image.NewRGBA(image.Rect(0, 0, c.w, c.h)), c.side).Bounds()
		if r.Dx() != c.ww || r.Dy() != c.wh {
			t.Errorf("fit %dx%d to %d = %dx%d, want %dx%d", c.w, c.h, c.side, r.Dx(), r.Dy(), c.ww, c.wh)
		}
	}
}

// TestDefaultLimits runs the real 2560/640 limits once.
func TestDefaultLimits(t *testing.T) {
	if testing.Short() {
		t.Skip("encodes a full-size image")
	}
	path := writeFile(t, "big.png", encodePNG(t, testImage(3000, 2000)))
	start := time.Now()
	p, err := PrepareFile(context.Background(), path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("3000x2000 PNG -> 2560 WebP + preview in %v (method %d)", time.Since(start), DefaultWebPMethod)
	if w, h := webpSize(t, checkRef(t, p)); w != media.MaxImageSide || h != 1706 {
		t.Fatalf("main is %dx%d", w, h)
	}
	if w, _ := webpSize(t, p.Preview.Data); w != media.PreviewSide {
		t.Fatalf("preview width %d", w)
	}
}

func BenchmarkEncode2560(b *testing.B) {
	img := testImage(2560, 1440)
	for _, m := range []int{2, 4} {
		b.Run("method"+string(rune('0'+m)), func(b *testing.B) {
			for b.Loop() {
				if _, err := encodeWebP(img, Options{WebPMethod: m}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
