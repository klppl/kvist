// Package imagemeta removes metadata (EXIF with GPS position and camera
// details, XMP, IPTC, comments, text chunks) from JPEG, PNG and WebP files
// without re-encoding the image. Color profiles are kept, and so is the
// JPEG orientation, written back as a minimal EXIF block, so photos are not
// shown rotated.
package imagemeta

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"strings"
)

// ErrFormat means the data is not a well-formed file of the claimed type.
var ErrFormat = errors.New("imagemeta: malformed image")

// Supported reports whether Strip handles files with this extension.
func Supported(ext string) bool {
	switch strings.ToLower(strings.TrimPrefix(ext, ".")) {
	case "jpg", "jpeg", "png", "webp":
		return true
	}
	return false
}

// Strip returns data without metadata. Unsupported types are returned
// unchanged. A malformed file returns ErrFormat: callers should not publish
// it, since its metadata could not be removed.
func Strip(ext string, data []byte) ([]byte, error) {
	switch strings.ToLower(strings.TrimPrefix(ext, ".")) {
	case "jpg", "jpeg":
		return stripJPEG(data)
	case "png":
		return stripPNG(data)
	case "webp":
		return stripWebP(data)
	}
	return data, nil
}

// --- JPEG ---

func stripJPEG(data []byte) ([]byte, error) {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return nil, ErrFormat
	}
	out := bytes.NewBuffer(make([]byte, 0, len(data)))
	out.Write(data[:2])
	orientation := 0
	insertedExif := false
	writeExif := func() {
		if !insertedExif && orientation > 1 && orientation <= 8 {
			out.Write(minimalExif(orientation))
		}
		insertedExif = true
	}
	i := 2
	for {
		// Skip fill bytes.
		for i < len(data) && data[i] == 0xFF && i+1 < len(data) && data[i+1] == 0xFF {
			i++
		}
		if i+2 > len(data) || data[i] != 0xFF {
			return nil, ErrFormat
		}
		marker := data[i+1]
		switch {
		case marker == 0xD9: // EOI
			writeExif()
			out.Write(data[i : i+2])
			return out.Bytes(), nil
		case marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7):
			out.Write(data[i : i+2])
			i += 2
			continue
		}
		if i+4 > len(data) {
			return nil, ErrFormat
		}
		segLen := int(binary.BigEndian.Uint16(data[i+2 : i+4]))
		end := i + 2 + segLen
		if segLen < 2 || end > len(data) {
			return nil, ErrFormat
		}
		payload := data[i+4 : end]
		keep := true
		switch {
		case marker == 0xE0: // JFIF
		case marker == 0xE1: // EXIF or XMP
			if o := exifOrientation(payload); o > 0 {
				orientation = o
			}
			keep = false
		case marker == 0xE2: // ICC profile is harmless; FlashPix and others go
			keep = bytes.HasPrefix(payload, []byte("ICC_PROFILE\x00"))
		case marker == 0xEE: // Adobe color transform
		case marker >= 0xE3 && marker <= 0xEF, marker == 0xFE: // other APPn (IPTC…), comments
			keep = false
		}
		if marker != 0xE0 && marker != 0xE1 {
			// Write the orientation right after JFIF (or SOI).
			writeExif()
		}
		if keep {
			out.Write(data[i:end])
		}
		i = end
		if marker == 0xDA { // start of scan: the rest is image data
			out.Write(data[i:])
			return out.Bytes(), nil
		}
	}
}

// exifOrientation reads the Orientation tag (0x0112) from an APP1 EXIF
// payload, or returns 0.
func exifOrientation(p []byte) int {
	if !bytes.HasPrefix(p, []byte("Exif\x00\x00")) {
		return 0
	}
	t := p[6:]
	if len(t) < 8 {
		return 0
	}
	var bo binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 0
	}
	off := int(bo.Uint32(t[4:8]))
	if off+2 > len(t) || off < 8 {
		return 0
	}
	n := int(bo.Uint16(t[off : off+2]))
	for k := 0; k < n; k++ {
		e := off + 2 + 12*k
		if e+12 > len(t) {
			return 0
		}
		if bo.Uint16(t[e:e+2]) == 0x0112 && bo.Uint16(t[e+2:e+4]) == 3 {
			return int(bo.Uint16(t[e+8 : e+10]))
		}
	}
	return 0
}

// minimalExif builds an APP1 segment holding only the orientation.
func minimalExif(orientation int) []byte {
	var b bytes.Buffer
	b.Write([]byte{0xFF, 0xE1, 0x00, 0x22}) // length 34
	b.WriteString("Exif\x00\x00")
	b.WriteString("MM\x00\x2A\x00\x00\x00\x08")                     // TIFF header, IFD0 at 8
	b.Write([]byte{0x00, 0x01})                                     // one entry
	b.Write([]byte{0x01, 0x12, 0x00, 0x03, 0x00, 0x00, 0x00, 0x01}) // Orientation, SHORT, count 1
	b.Write([]byte{0x00, byte(orientation), 0x00, 0x00})            // value
	b.Write([]byte{0x00, 0x00, 0x00, 0x00})                         // no next IFD
	return b.Bytes()
}

// --- PNG ---

var pngSig = []byte("\x89PNG\r\n\x1a\n")

// pngDrop lists ancillary chunks that carry metadata.
var pngDrop = map[string]bool{"tEXt": true, "zTXt": true, "iTXt": true, "eXIf": true, "tIME": true}

func stripPNG(data []byte) ([]byte, error) {
	if !bytes.HasPrefix(data, pngSig) {
		return nil, ErrFormat
	}
	out := bytes.NewBuffer(make([]byte, 0, len(data)))
	out.Write(pngSig)
	i := len(pngSig)
	for i < len(data) {
		if i+12 > len(data) {
			return nil, ErrFormat
		}
		n := int(binary.BigEndian.Uint32(data[i : i+4]))
		typ := string(data[i+4 : i+8])
		end := i + 12 + n
		if n < 0 || end > len(data) {
			return nil, ErrFormat
		}
		if crc32.ChecksumIEEE(data[i+4:i+8+n]) != binary.BigEndian.Uint32(data[i+8+n:end]) {
			return nil, ErrFormat
		}
		if !pngDrop[typ] {
			out.Write(data[i:end])
		}
		i = end
		if typ == "IEND" {
			return out.Bytes(), nil
		}
	}
	return nil, ErrFormat
}

// --- WebP ---

func stripWebP(data []byte) ([]byte, error) {
	if len(data) < 12 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return nil, ErrFormat
	}
	riffEnd := 8 + int(binary.LittleEndian.Uint32(data[4:8]))
	if riffEnd > len(data) {
		return nil, ErrFormat
	}
	var body bytes.Buffer
	i := 12
	for i < riffEnd {
		if i+8 > riffEnd {
			return nil, ErrFormat
		}
		fourcc := string(data[i : i+4])
		n := int(binary.LittleEndian.Uint32(data[i+4 : i+8]))
		end := i + 8 + n + n%2
		if n < 0 || i+8+n > riffEnd {
			return nil, ErrFormat
		}
		if end > riffEnd {
			end = riffEnd
		}
		switch fourcc {
		case "EXIF", "XMP ":
		case "VP8X":
			chunk := append([]byte(nil), data[i:end]...)
			if n >= 1 {
				chunk[8] &^= 0x08 | 0x04 // clear the EXIF and XMP flags
			}
			body.Write(chunk)
		default:
			body.Write(data[i:end])
		}
		i = end
	}
	out := make([]byte, 0, 12+body.Len())
	out = append(out, "RIFF"...)
	out = binary.LittleEndian.AppendUint32(out, uint32(4+body.Len()))
	out = append(out, "WEBP"...)
	return append(out, body.Bytes()...), nil
}
