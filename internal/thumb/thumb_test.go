package thumb

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

// quadrants is a w×h image with a red top left quarter, green top right,
// blue bottom left and white bottom right, to see where pixels end up.
func quadrants(w, h int, alpha uint8) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.NRGBA{255, 255, 255, alpha}
			switch {
			case x < w/2 && y < h/2:
				c = color.NRGBA{255, 0, 0, alpha}
			case y < h/2:
				c = color.NRGBA{0, 255, 0, alpha}
			case x < w/2:
				c = color.NRGBA{0, 0, 255, alpha}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// withOrientation inserts an EXIF block with an orientation after SOI.
func withOrientation(data []byte, o uint16) []byte {
	var t bytes.Buffer
	t.WriteString("MM\x00*")
	binary.Write(&t, binary.BigEndian, uint32(8))
	binary.Write(&t, binary.BigEndian, uint16(1))
	binary.Write(&t, binary.BigEndian, []uint16{0x0112, 3})
	binary.Write(&t, binary.BigEndian, uint32(1))
	binary.Write(&t, binary.BigEndian, []uint16{o, 0})
	binary.Write(&t, binary.BigEndian, uint32(0))
	payload := append([]byte("Exif\x00\x00"), t.Bytes()...)
	seg := binary.BigEndian.AppendUint16([]byte{0xFF, 0xE1}, uint16(len(payload)+2))
	out := append([]byte{}, data[:2]...)
	out = append(out, seg...)
	out = append(out, payload...)
	return append(out, data[2:]...)
}

func decodeOut(t *testing.T, data []byte, ext string) image.Image {
	t.Helper()
	var img image.Image
	var err error
	if ext == "png" {
		img, err = png.Decode(bytes.NewReader(data))
	} else {
		img, err = jpeg.Decode(bytes.NewReader(data))
	}
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func near(c color.Color, r, g, b uint8) bool {
	cr, cg, cb, _ := c.RGBA()
	d := func(a uint32, b uint8) bool { x := int(a>>8) - int(b); return x > -40 && x < 40 }
	return d(cr, r) && d(cg, g) && d(cb, b)
}

func TestCropAndScale(t *testing.T) {
	im, err := Decode("png", encodePNG(t, quadrants(400, 200, 255)))
	if err != nil {
		t.Fatal(err)
	}
	data, ext, err := im.Make(50, 50)
	if err != nil {
		t.Fatal(err)
	}
	if ext != "jpg" {
		t.Errorf("opaque image encoded as %s, want jpg", ext)
	}
	out := decodeOut(t, data, ext)
	if b := out.Bounds(); b.Dx() != 50 || b.Dy() != 50 {
		t.Fatalf("size %v, want 50×50", b)
	}
	// The middle square of a 2:1 image keeps all four quadrants.
	if c := out.At(5, 5); !near(c, 255, 0, 0) {
		t.Errorf("top left %v, want red", c)
	}
	if c := out.At(45, 45); !near(c, 255, 255, 255) {
		t.Errorf("bottom right %v, want white", c)
	}
	again, _, _ := im.Make(50, 50)
	if !bytes.Equal(data, again) {
		t.Error("not deterministic")
	}
}

func TestNoUpscale(t *testing.T) {
	im, err := Decode("png", encodePNG(t, quadrants(40, 40, 255)))
	if err != nil {
		t.Fatal(err)
	}
	data, ext, err := im.Make(640, 360)
	if err != nil {
		t.Fatal(err)
	}
	if b := decodeOut(t, data, ext).Bounds(); b.Dx() != 40 || b.Dy() != 22 {
		t.Errorf("size %v, want 40×22 (cropped to 16:9, not enlarged)", b)
	}
}

func TestTransparencyKeepsPNG(t *testing.T) {
	im, err := Decode("png", encodePNG(t, quadrants(64, 64, 128)))
	if err != nil {
		t.Fatal(err)
	}
	_, ext, err := im.Make(32, 32)
	if err != nil {
		t.Fatal(err)
	}
	if ext != "png" {
		t.Errorf("transparent image encoded as %s, want png", ext)
	}
}

func TestOrientation(t *testing.T) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, quadrants(200, 100, 255), &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	// Stored landscape, shown turned a quarter clockwise: portrait, with
	// the stored bottom left (blue) at the top left.
	im, err := Decode("jpg", withOrientation(buf.Bytes(), 6))
	if err != nil {
		t.Fatal(err)
	}
	data, ext, err := im.Make(50, 100)
	if err != nil {
		t.Fatal(err)
	}
	out := decodeOut(t, data, ext)
	if b := out.Bounds(); b.Dx() != 50 || b.Dy() != 100 {
		t.Fatalf("size %v, want 50×100", b)
	}
	if c := out.At(5, 5); !near(c, 0, 0, 255) {
		t.Errorf("top left %v, want blue", c)
	}
	if c := out.At(45, 95); !near(c, 0, 255, 0) {
		t.Errorf("bottom right %v, want green", c)
	}
}

func TestNoMetadata(t *testing.T) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, quadrants(100, 100, 255), nil); err != nil {
		t.Fatal(err)
	}
	in := withOrientation(buf.Bytes(), 1)
	in = append(in[:2], append([]byte("\xFF\xFE\x00\x0cGPS-SECRET"), in[2:]...)...)
	im, err := Decode("jpeg", in)
	if err != nil {
		t.Fatal(err)
	}
	data, _, err := im.Make(20, 20)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("GPS-SECRET")) || bytes.Contains(data, []byte("Exif")) {
		t.Error("metadata survived")
	}
}

func TestDecodeErrors(t *testing.T) {
	if _, err := Decode("svg", []byte("<svg/>")); err != ErrFormat {
		t.Errorf("svg: %v, want ErrFormat", err)
	}
	if _, err := Decode("png", []byte("not a png")); err == nil {
		t.Error("malformed png decoded")
	}
	// A header claiming a huge image is refused before decoding.
	big := encodePNG(t, image.NewNRGBA(image.Rect(0, 0, 1, 1)))
	binary.BigEndian.PutUint32(big[16:], 20000)
	binary.BigEndian.PutUint32(big[20:], 20000)
	binary.BigEndian.PutUint32(big[29:], crc32.ChecksumIEEE(big[12:29]))
	if _, err := Decode("png", big); err != ErrTooLarge {
		t.Errorf("huge: %v, want ErrTooLarge", err)
	}
}
