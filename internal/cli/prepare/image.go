package prepare

import (
	"bytes"
	"image"
	_ "image/gif" // register decoders for image.Decode
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"

	"github.com/gen2brain/webp" // also registers the WebP decoder
	"golang.org/x/image/draw"

	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/media"
)

// prepareImage turns a PNG, JPEG, GIF (first frame) or WebP into a WebP whose
// longest side is at most Limits.ImageSide, plus a preview. A WebP that is
// already small enough is kept byte for byte.
func prepareImage(path string, src media.Source, size int64, opts Options) (*Prepared, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, kernel.Internal(err, "cannot read %s", path)
	}
	name := filepath.Base(path)
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, kernel.Invalid("%s is not a readable image: %v", name, err).
			WithFix("export the image again as PNG or WebP")
	}
	if int64(cfg.Width)*int64(cfg.Height) > MaxPixels {
		return nil, kernel.Invalid("%s is %dx%d pixels; the limit is %d megapixels", name, cfg.Width, cfg.Height, MaxPixels/1_000_000).
			WithFix("resize the image to at most %d pixels on its longest side", opts.Limits.ImageSide)
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, kernel.Invalid("%s is not a readable image: %v", name, err).
			WithFix("export the image again as PNG or WebP")
	}

	lim := opts.Limits
	var main []byte
	if src == media.SourceWebP && longest(img.Bounds()) <= lim.ImageSide && size <= media.MaxBytes(media.TypeWebP) {
		main = raw
	} else {
		img = fit(img, lim.ImageSide)
		if main, err = encodeWebP(img, opts); err != nil {
			return nil, err
		}
	}
	if int64(len(main)) > media.MaxBytes(media.TypeWebP) {
		return nil, tooBig(path, int64(len(main)), media.MaxBytes(media.TypeWebP), "resize the image or lower its detail")
	}
	prev, err := encodeWebP(fit(img, lim.PreviewSide), opts)
	if err != nil {
		return nil, err
	}

	outName := replaceExt(path, ".webp")
	prevSum := hashBytes(prev)
	ref := media.FileRef{SHA256: hashBytes(main), Size: int64(len(main)), Type: media.TypeWebP, Name: outName, PreviewSHA256: prevSum}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	return &Prepared{
		File: ref,
		Data: main,
		Preview: &Preview{
			File: media.FileRef{SHA256: prevSum, Size: int64(len(prev)), Type: media.TypeWebP, Name: replaceExt(path, ".preview.webp")},
			Data: prev,
		},
	}, nil
}

func longest(r image.Rectangle) int { return max(r.Dx(), r.Dy()) }

// fit scales img down so its longest side is at most side pixels, keeping
// the aspect ratio. It never scales up and returns img unchanged when it
// already fits.
func fit(img image.Image, side int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if max(w, h) <= side {
		return img
	}
	var nw, nh int
	if w >= h {
		nw, nh = side, max(1, int(int64(h)*int64(side)/int64(w)))
	} else {
		nw, nh = max(1, int(int64(w)*int64(side)/int64(h))), side
	}
	dst := image.NewNRGBA(image.Rect(0, 0, nw, nh))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
	return dst
}

// encodeWebP encodes img as lossy WebP with the configured quality and method.
func encodeWebP(img image.Image, opts Options) ([]byte, error) {
	q := opts.WebPQuality
	if q <= 0 {
		q = DefaultWebPQuality
	}
	m := opts.WebPMethod
	if m <= 0 {
		m = DefaultWebPMethod
	}
	var buf bytes.Buffer
	if err := webp.Encode(&buf, img, webp.Options{Quality: q, Method: m}); err != nil {
		return nil, kernel.Internal(err, "webp encode failed")
	}
	return buf.Bytes(), nil
}
