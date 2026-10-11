package imagemeta

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"image"
	_ "image/gif" // register decoders for image.DecodeConfig
	_ "image/jpeg"
	_ "image/png"
	"math"
	"strconv"
	"strings"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
)

// SizeHeader is how much of a file Size needs: image headers, and the
// EXIF block of a JPEG, sit at the start.
const SizeHeader = 256 << 10

// Size returns the displayed size of an image in pixels from the start of
// its file (see SizeHeader), so pages can reserve the space before it
// loads. JPEGs turned by their EXIF orientation have width and height
// swapped, as browsers show them. SVGs use their width and height, else
// their viewBox. ok is false when the size can't be told (AVIF, SVGs in
// relative units, malformed files).
func Size(ext string, head []byte) (w, h int, ok bool) {
	switch strings.ToLower(strings.TrimPrefix(ext, ".")) {
	case "svg":
		return svgSize(head)
	case "png", "jpg", "jpeg", "gif", "webp", "bmp":
	default:
		return 0, 0, false
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(head))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return 0, 0, false
	}
	w, h = cfg.Width, cfg.Height
	if o := jpegOrientation(head); o >= 5 && o <= 8 {
		w, h = h, w
	}
	return w, h, true
}

// jpegOrientation returns the EXIF orientation of a JPEG, or 0.
func jpegOrientation(data []byte) int {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return 0
	}
	for i := 2; i+4 <= len(data) && data[i] == 0xFF; {
		marker := data[i+1]
		if marker == 0xDA || marker == 0xD9 {
			return 0
		}
		end := i + 2 + int(binary.BigEndian.Uint16(data[i+2:i+4]))
		if end > len(data) || end < i+4 {
			return 0
		}
		if marker == 0xE1 {
			if o := exifOrientation(data[i+4 : end]); o > 0 {
				return o
			}
		}
		i = end
	}
	return 0
}

// svgSize reads the size of an SVG from its root element.
func svgSize(head []byte) (int, int, bool) {
	d := xml.NewDecoder(bytes.NewReader(head))
	d.Strict = false
	for {
		tok, err := d.Token()
		if err != nil {
			return 0, 0, false
		}
		el, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if el.Name.Local != "svg" {
			return 0, 0, false
		}
		var ws, hs, vb string
		for _, a := range el.Attr {
			switch a.Name.Local {
			case "width":
				ws = a.Value
			case "height":
				hs = a.Value
			case "viewBox":
				vb = a.Value
			}
		}
		w, wok := svgLength(ws)
		h, hok := svgLength(hs)
		var vw, vh float64
		if f := strings.Fields(strings.ReplaceAll(vb, ",", " ")); len(f) == 4 {
			vw, _ = strconv.ParseFloat(f[2], 64)
			vh, _ = strconv.ParseFloat(f[3], 64)
		}
		switch {
		case wok && hok:
		case wok && vw > 0 && vh > 0:
			h = w * vh / vw
		case hok && vw > 0 && vh > 0:
			w = h * vw / vh
		case !wok && !hok && vw > 0 && vh > 0:
			w, h = vw, vh
		default:
			return 0, 0, false
		}
		wi, hi := int(math.Round(w)), int(math.Round(h))
		if wi <= 0 || hi <= 0 || wi > 100000 || hi > 100000 {
			return 0, 0, false
		}
		return wi, hi, true
	}
}

// svgLength parses an absolute SVG length in pixels ("120", "120px").
// Percentages and font-relative units have no fixed size.
func svgLength(s string) (float64, bool) {
	s = strings.TrimSuffix(strings.TrimSpace(s), "px")
	v, err := strconv.ParseFloat(s, 64)
	return v, err == nil && v > 0
}
