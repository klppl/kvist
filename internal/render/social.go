package render

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// Social preview images: for notes without an image property, when the
// theme's social_images param is on, a 1200×630 PNG with the note's title,
// description and the site's name. They are named by a hash of what they
// show, so an unchanged image is reused from the previous build and can be
// cached forever.

const (
	socialW, socialH = 1200, 630
	socialPad        = 80
	// socialVersion changes the hash of every image when the drawing changes.
	socialVersion = "1"
)

// Reuser is an Output that can keep a file from the previous build
// instead of having it written again. Files passed to it are named by
// their content.
type Reuser interface {
	Reuse(p string) bool
}

var socialBold = sync.OnceValues(func() (*opentype.Font, error) {
	return opentype.Parse(gobold.TTF)
})

var socialRegular = sync.OnceValues(func() (*opentype.Font, error) {
	return opentype.Parse(goregular.TTF)
})

type socialCard struct {
	Site, Title, Description, Domain string
	Accent                           color.RGBA
}

// socialImages writes an image for every note page without an image
// property and returns their URLs by note path.
func (r *renderer) socialImages() (map[string]string, error) {
	if on, _ := lookupParam(r.theme, r.site, "social_images").(bool); !on {
		return nil, nil
	}
	bold, err := socialBold()
	if err != nil {
		return nil, err
	}
	regular, err := socialRegular()
	if err != nil {
		return nil, err
	}
	accent := parseHex(fmt.Sprint(lookupParam(r.theme, r.site, "accent")))
	domain := r.site.Config.BaseURL
	if i := strings.Index(domain, "://"); i >= 0 {
		domain = domain[i+3:]
	}
	urls := map[string]string{}
	done := map[string]bool{}
	for _, n := range r.site.Notes {
		if n.Image != "" || n.URL == "/" {
			continue
		}
		c := socialCard{Site: r.site.Config.Title, Title: n.Title, Description: n.Description, Domain: domain, Accent: accent}
		if !covers(bold, c.Site+c.Title) || !covers(regular, c.Description+c.Domain) {
			continue // the Go fonts lack some letters (CJK, …): no image beats boxes
		}
		u := "/_kvist/social/" + c.hash() + ".png"
		urls[n.Path] = u
		if done[u] {
			continue
		}
		done[u] = true
		rel := strings.TrimPrefix(u, "/")
		if ru, ok := r.out.(Reuser); ok && ru.Reuse(rel) {
			continue
		}
		b, err := c.draw(bold, regular)
		if err != nil {
			return nil, fmt.Errorf("social image for %s: %w", n.Path, err)
		}
		if err := r.out.WriteFile(rel, b); err != nil {
			return nil, err
		}
	}
	return urls, nil
}

func (c socialCard) hash() string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s\x00%s\x00%v", socialVersion, c.Site, c.Title, c.Description, c.Domain, c.Accent)
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// covers reports whether the font has a glyph for every letter of s.
func covers(f *opentype.Font, s string) bool {
	var buf sfnt.Buffer
	for _, r := range s {
		if r == ' ' || r == '\n' || r == '\t' {
			continue
		}
		if i, err := f.GlyphIndex(&buf, r); err != nil || i == 0 {
			return false
		}
	}
	return true
}

var (
	socialBg    = color.RGBA{0xf8, 0xf9, 0xfa, 0xff}
	socialFg    = color.RGBA{0x16, 0x18, 0x1d, 0xff}
	socialMuted = color.RGBA{0x6b, 0x70, 0x79, 0xff}
)

func (c socialCard) draw(bold, regular *opentype.Font) ([]byte, error) {
	img := image.NewRGBA(image.Rect(0, 0, socialW, socialH))
	draw.Draw(img, img.Bounds(), image.NewUniform(socialBg), image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(0, socialH-14, socialW, socialH), image.NewUniform(c.Accent), image.Point{}, draw.Src)
	width := socialW - 2*socialPad

	face := func(f *opentype.Font, size float64) (font.Face, error) {
		return opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingNone})
	}
	text := func(f font.Face, col color.Color, x, y int, s string) {
		d := font.Drawer{Dst: img, Src: image.NewUniform(col), Face: f, Dot: fixed.P(x, y)}
		d.DrawString(s)
	}

	// The site's name, with an accent dot.
	siteFace, err := face(bold, 32)
	if err != nil {
		return nil, err
	}
	defer siteFace.Close()
	dot := image.Rect(socialPad, socialPad+4, socialPad+18, socialPad+22)
	draw.DrawMask(img, dot, image.NewUniform(c.Accent), image.Point{}, &circle{dot}, dot.Min, draw.Over)
	text(siteFace, socialFg, socialPad+32, socialPad+25, fit(siteFace, c.Site, width-32, 1)[0])

	// The title: as large as fits in three lines.
	var titleFace font.Face
	var lines []string
	size := 76.0
	for ; ; size -= 6 {
		if titleFace != nil {
			titleFace.Close()
		}
		if titleFace, err = face(bold, size); err != nil {
			return nil, err
		}
		lines = wrap(titleFace, c.Title, width)
		if len(lines) <= 3 || size <= 52 {
			break
		}
	}
	defer titleFace.Close()
	lines = fit(titleFace, c.Title, width, 3)
	lh := int(size * 1.18)
	y := 230 + int(size*0.8)
	for _, l := range lines {
		text(titleFace, socialFg, socialPad, y, l)
		y += lh
	}

	// The description, below the title if there is room.
	descFace, err := face(regular, 30)
	if err != nil {
		return nil, err
	}
	defer descFace.Close()
	foot := socialH - socialPad - 6
	y += 14
	if room := min(2, (foot-50-y)/42+1); c.Description != "" && room > 0 {
		for _, l := range fit(descFace, c.Description, width, room) {
			text(descFace, socialMuted, socialPad, y, l)
			y += 42
		}
	}
	if c.Domain != "" {
		text(descFace, socialMuted, socialPad, foot, fit(descFace, c.Domain, width, 1)[0])
	}

	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// wrap breaks s into lines no wider than width, at spaces, and inside a
// word only when the word alone is too wide.
func wrap(f font.Face, s string, width int) []string {
	w := fixed.I(width)
	var lines []string
	line := ""
	for _, word := range strings.Fields(s) {
		try := word
		if line != "" {
			try = line + " " + word
		}
		if font.MeasureString(f, try) <= w {
			line = try
			continue
		}
		if line != "" {
			lines = append(lines, line)
		}
		line = ""
		for _, r := range word {
			if line != "" && font.MeasureString(f, line+string(r)) > w {
				lines = append(lines, line)
				line = ""
			}
			line += string(r)
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		lines = []string{""}
	}
	return lines
}

// fit wraps s into at most max lines, ending the last with … if cut.
func fit(f font.Face, s string, width, max int) []string {
	lines := wrap(f, s, width)
	if len(lines) <= max {
		return lines
	}
	lines = lines[:max]
	last := []rune(lines[max-1])
	for len(last) > 0 && font.MeasureString(f, string(last)+"…") > fixed.I(width) {
		last = last[:len(last)-1]
	}
	lines[max-1] = strings.TrimRight(string(last), " ,.;:") + "…"
	return lines
}

// circle is an antialiased disc filling r, as a mask.
type circle struct{ r image.Rectangle }

func (c *circle) ColorModel() color.Model { return color.AlphaModel }
func (c *circle) Bounds() image.Rectangle { return c.r }
func (c *circle) At(x, y int) color.Color {
	cx, cy := float64(c.r.Min.X+c.r.Max.X)/2, float64(c.r.Min.Y+c.r.Max.Y)/2
	rad := float64(c.r.Dx()) / 2
	dx, dy := float64(x)+.5-cx, float64(y)+.5-cy
	d := rad - math.Hypot(dx, dy)
	switch {
	case d >= .5:
		return color.Alpha{255}
	case d <= -.5:
		return color.Alpha{0}
	}
	return color.Alpha{uint8((d + .5) * 255)}
}

// parseHex reads #rgb or #rrggbb, falling back to the garden green.
func parseHex(s string) color.RGBA {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	var r, g, b uint8
	if len(s) != 6 {
		return color.RGBA{0x3f, 0x7d, 0x4e, 0xff}
	}
	if _, err := fmt.Sscanf(s, "%02x%02x%02x", &r, &g, &b); err != nil {
		return color.RGBA{0x3f, 0x7d, 0x4e, 0xff}
	}
	return color.RGBA{r, g, b, 0xff}
}
