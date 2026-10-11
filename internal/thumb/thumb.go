// Package thumb makes small copies of images for note lists and cards: the
// image is turned upright, cropped from the middle to the wanted shape,
// scaled down (never up) and encoded again. Drawing it again from the pixels
// leaves out all metadata. The output depends only on the input bytes and
// the size, so builds stay deterministic (§4).
package thumb

import (
	"bytes"
	"errors"
	"image"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"strings"

	"golang.org/x/image/bmp"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/webp"

	"github.com/klppl/kvist/internal/imagemeta"
)

// MaxPixels limits the images Decode reads, so a huge picture can't take
// the server's memory. Larger ones keep being shown full size.
const MaxPixels = 50_000_000

// ErrTooLarge means the image has more than MaxPixels pixels.
var ErrTooLarge = errors.New("thumb: image too large")

// ErrFormat means the extension is not one Decode reads.
var ErrFormat = errors.New("thumb: unsupported image type")

// Supported reports whether Decode reads files with this extension. SVGs
// scale by themselves and are left alone.
func Supported(ext string) bool {
	switch strings.ToLower(strings.TrimPrefix(ext, ".")) {
	case "jpg", "jpeg", "png", "gif", "webp", "bmp":
		return true
	}
	return false
}

// Image is a decoded image, ready to be made into thumbnails.
type Image struct {
	src         image.Image
	orientation int // EXIF orientation, 1–8
}

// Decode reads an image file. A GIF counts by its first frame.
func Decode(ext string, data []byte) (*Image, error) {
	var decode func(r *bytes.Reader) (image.Image, error)
	var config func(r *bytes.Reader) (image.Config, error)
	orientation := 1
	switch strings.ToLower(strings.TrimPrefix(ext, ".")) {
	case "jpg", "jpeg":
		decode = func(r *bytes.Reader) (image.Image, error) { return jpeg.Decode(r) }
		config = func(r *bytes.Reader) (image.Config, error) { return jpeg.DecodeConfig(r) }
		if o := imagemeta.JPEGOrientation(data); o > 0 {
			orientation = o
		}
	case "png":
		decode = func(r *bytes.Reader) (image.Image, error) { return png.Decode(r) }
		config = func(r *bytes.Reader) (image.Config, error) { return png.DecodeConfig(r) }
	case "gif":
		decode = func(r *bytes.Reader) (image.Image, error) { return gif.Decode(r) }
		config = func(r *bytes.Reader) (image.Config, error) { return gif.DecodeConfig(r) }
	case "webp":
		decode = func(r *bytes.Reader) (image.Image, error) { return webp.Decode(r) }
		config = func(r *bytes.Reader) (image.Config, error) { return webp.DecodeConfig(r) }
	case "bmp":
		decode = func(r *bytes.Reader) (image.Image, error) { return bmp.Decode(r) }
		config = func(r *bytes.Reader) (image.Config, error) { return bmp.DecodeConfig(r) }
	default:
		return nil, ErrFormat
	}
	cfg, err := config(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, errors.New("thumb: empty image")
	}
	if int64(cfg.Width)*int64(cfg.Height) > MaxPixels {
		return nil, ErrTooLarge
	}
	src, err := decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return &Image{src: src, orientation: orientation}, nil
}

// Make returns the image cropped to the shape w×h from the middle and
// scaled to fit it, and the extension of the encoding: "jpg", or "png" for
// images with transparency. An image smaller than w×h is cropped but not
// enlarged.
func (im *Image) Make(w, h int) ([]byte, string, error) {
	if w <= 0 || h <= 0 {
		return nil, "", errors.New("thumb: bad size")
	}
	// Work in the stored (not yet upright) orientation: orientations 5–8
	// turn the picture a quarter, so the shape is h×w there. A crop from
	// the middle stays in the middle when turned.
	sideways := im.orientation >= 5
	if sideways {
		w, h = h, w
	}
	b := im.src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	cw, ch := sw, sw*h/w
	if ch > sh {
		cw, ch = sh*w/h, sh
	}
	cw, ch = max(cw, 1), max(ch, 1)
	at := image.Pt(b.Min.X+(sw-cw)/2, b.Min.Y+(sh-ch)/2)
	crop := image.Rectangle{Min: at, Max: at.Add(image.Pt(cw, ch))}
	dw, dh := w, h
	if cw < w {
		dw, dh = cw, ch
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	if dw == cw && dh == ch {
		draw.Draw(dst, dst.Bounds(), im.src, crop.Min, draw.Src)
	} else {
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), im.src, crop, xdraw.Src, nil)
	}
	out := upright(dst, im.orientation)

	var buf bytes.Buffer
	if opaque(out) {
		if err := jpeg.Encode(&buf, out, &jpeg.Options{Quality: 82}); err != nil {
			return nil, "", err
		}
		return buf.Bytes(), "jpg", nil
	}
	if err := png.Encode(&buf, out); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), "png", nil
}

func opaque(img *image.RGBA) bool {
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] != 0xFF {
			return false
		}
	}
	return true
}

// upright applies an EXIF orientation: the stored image is shown flipped
// and turned like this.
func upright(src *image.RGBA, orientation int) *image.RGBA {
	if orientation <= 1 || orientation > 8 {
		return src
	}
	w, h := src.Rect.Dx(), src.Rect.Dy()
	dw, dh := w, h
	if orientation >= 5 {
		dw, dh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			var sx, sy int
			switch orientation {
			case 2: // flipped left to right
				sx, sy = w-1-x, y
			case 3: // turned half way
				sx, sy = w-1-x, h-1-y
			case 4: // flipped top to bottom
				sx, sy = x, h-1-y
			case 5: // transposed
				sx, sy = y, x
			case 6: // to be turned a quarter clockwise
				sx, sy = y, h-1-x
			case 7: // transversed
				sx, sy = w-1-y, h-1-x
			case 8: // to be turned a quarter counterclockwise
				sx, sy = w-1-y, x
			}
			copy(dst.Pix[dst.PixOffset(x, y):][:4], src.Pix[src.PixOffset(sx, sy):][:4])
		}
	}
	return dst
}
