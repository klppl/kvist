package imagemeta

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

func testImage() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for x := 0; x < 8; x++ {
		img.Set(x, x, color.RGBA{200, 30, 30, 255})
	}
	return img
}

// exifSegment builds an APP1 EXIF segment with an orientation and a fake
// GPS string to detect.
func exifSegment(orientation uint16) []byte {
	var t bytes.Buffer
	t.WriteString("II*\x00")
	binary.Write(&t, binary.LittleEndian, uint32(8))
	binary.Write(&t, binary.LittleEndian, uint16(1))
	binary.Write(&t, binary.LittleEndian, uint16(0x0112))
	binary.Write(&t, binary.LittleEndian, uint16(3))
	binary.Write(&t, binary.LittleEndian, uint32(1))
	binary.Write(&t, binary.LittleEndian, orientation)
	binary.Write(&t, binary.LittleEndian, uint16(0))
	binary.Write(&t, binary.LittleEndian, uint32(0))
	t.WriteString("GPS-SECRET 59.3293N 18.0686E Alex's iPhone")
	payload := append([]byte("Exif\x00\x00"), t.Bytes()...)
	seg := []byte{0xFF, 0xE1}
	seg = binary.BigEndian.AppendUint16(seg, uint16(len(payload)+2))
	return append(seg, payload...)
}

func segment(marker byte, payload string) []byte {
	seg := []byte{0xFF, marker}
	seg = binary.BigEndian.AppendUint16(seg, uint16(len(payload)+2))
	return append(seg, payload...)
}

func TestJPEG(t *testing.T) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, testImage(), nil); err != nil {
		t.Fatal(err)
	}
	orig := buf.Bytes()
	// Insert EXIF, XMP, IPTC, a comment and an ICC profile after SOI.
	var in bytes.Buffer
	in.Write(orig[:2])
	in.Write(exifSegment(6))
	in.Write(segment(0xE1, "http://ns.adobe.com/xap/1.0/\x00<x:xmpmeta>XMP-SECRET</x:xmpmeta>"))
	in.Write(segment(0xED, "Photoshop 3.0\x00IPTC-SECRET"))
	in.Write(segment(0xFE, "COMMENT-SECRET"))
	in.Write(segment(0xE2, "ICC_PROFILE\x00\x01\x01profile"))
	in.Write(orig[2:])

	out, err := Strip(".JPG", in.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"GPS-SECRET", "XMP-SECRET", "IPTC-SECRET", "COMMENT-SECRET"} {
		if bytes.Contains(out, []byte(s)) {
			t.Errorf("%s survived", s)
		}
	}
	if !bytes.Contains(out, []byte("ICC_PROFILE")) {
		t.Error("ICC profile removed")
	}
	if _, err := jpeg.Decode(bytes.NewReader(out)); err != nil {
		t.Fatalf("stripped JPEG does not decode: %v", err)
	}
	// Orientation is preserved.
	i := bytes.Index(out, []byte("Exif\x00\x00"))
	if i < 0 || exifOrientation(out[i:]) != 6 {
		t.Errorf("orientation lost")
	}
	// Stripping is idempotent.
	again, err := Strip("jpg", out)
	if err != nil || !bytes.Equal(again, out) {
		t.Errorf("not idempotent: %v", err)
	}
	if _, err := Strip("jpg", []byte("not a jpeg")); err != ErrFormat {
		t.Errorf("malformed: %v", err)
	}
}

func pngChunk(typ string, data []byte) []byte {
	var b []byte
	b = binary.BigEndian.AppendUint32(b, uint32(len(data)))
	b = append(b, typ...)
	b = append(b, data...)
	return binary.BigEndian.AppendUint32(b, crc32.ChecksumIEEE(append([]byte(typ), data...)))
}

func TestPNG(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, testImage()); err != nil {
		t.Fatal(err)
	}
	orig := buf.Bytes()
	ihdrEnd := 8 + 12 + 13
	var in bytes.Buffer
	in.Write(orig[:ihdrEnd])
	in.Write(pngChunk("tEXt", []byte("Author\x00TEXT-SECRET")))
	in.Write(pngChunk("iTXt", []byte("XML:com.adobe.xmp\x00\x00\x00\x00\x00ITXT-SECRET")))
	in.Write(pngChunk("eXIf", []byte("MM\x00*EXIF-SECRET")))
	in.Write(pngChunk("pHYs", []byte{0, 0, 11, 19, 0, 0, 11, 19, 1}))
	in.Write(orig[ihdrEnd:])

	out, err := Strip("png", in.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"TEXT-SECRET", "ITXT-SECRET", "EXIF-SECRET"} {
		if bytes.Contains(out, []byte(s)) {
			t.Errorf("%s survived", s)
		}
	}
	if !bytes.Contains(out, []byte("pHYs")) {
		t.Error("pHYs removed")
	}
	if _, err := png.Decode(bytes.NewReader(out)); err != nil {
		t.Fatalf("stripped PNG does not decode: %v", err)
	}
	bad := append([]byte(nil), in.Bytes()...)
	bad[ihdrEnd+10] ^= 0xFF // corrupt a chunk
	if _, err := Strip("png", bad); err != ErrFormat {
		t.Errorf("corrupt PNG: %v", err)
	}
}

func webpChunk(fourcc string, data []byte) []byte {
	b := append([]byte(fourcc), binary.LittleEndian.AppendUint32(nil, uint32(len(data)))...)
	b = append(b, data...)
	if len(data)%2 == 1 {
		b = append(b, 0)
	}
	return b
}

func TestWebP(t *testing.T) {
	var body []byte
	body = append(body, webpChunk("VP8X", []byte{0x08 | 0x04 | 0x10, 0, 0, 0, 7, 0, 0, 7, 0, 0})...)
	body = append(body, webpChunk("VP8L", []byte("fake image data"))...)
	body = append(body, webpChunk("EXIF", []byte("EXIF-SECRET"))...)
	body = append(body, webpChunk("XMP ", []byte("XMP-SECRET"))...)
	in := append([]byte("RIFF"), binary.LittleEndian.AppendUint32(nil, uint32(4+len(body)))...)
	in = append(in, "WEBP"...)
	in = append(in, body...)

	out, err := Strip("webp", in)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(out, []byte("SECRET")) {
		t.Error("metadata survived")
	}
	if got := int(binary.LittleEndian.Uint32(out[4:8])); got != len(out)-8 {
		t.Errorf("RIFF size %d, file %d", got, len(out))
	}
	if flags := out[20]; flags != 0x10 {
		t.Errorf("VP8X flags = %#x, want alpha only", flags)
	}
	if !bytes.Contains(out, []byte("fake image data")) {
		t.Error("image data lost")
	}
}
