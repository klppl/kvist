// Package slug turns titles and paths into URL slugs and heading anchors.
package slug

import (
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Make lowercases s and replaces every run of characters that are not
// letters or digits with a single '-'. Unicode letters are kept (URLs may
// contain them; browsers show them as typed).
func Make(s string) string {
	s = norm.NFC.String(s)
	var b strings.Builder
	dash := false
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(unicode.ToLower(r))
		case unicode.IsMark(r):
			b.WriteRune(r)
		default:
			dash = true
		}
	}
	return b.String()
}

// Path slugifies every segment of a vault path and drops the .md extension:
// "Garden/My Note.md" → "garden/my-note".
func Path(p string) string {
	p = strings.TrimSuffix(p, ".md")
	p = strings.TrimSuffix(p, ".MD")
	segs := strings.Split(p, "/")
	out := segs[:0]
	for _, s := range segs {
		if v := Make(s); v != "" {
			out = append(out, v)
		}
	}
	return strings.Join(out, "/")
}

// Unique hands out anchors that are unique within one document.
type Unique struct{ seen map[string]bool }

// Get returns slug(text), suffixed with -1, -2, … if already taken. Empty
// slugs become "section".
func (u *Unique) Get(text string) string {
	if u.seen == nil {
		u.seen = map[string]bool{}
	}
	base := Make(text)
	if base == "" {
		base = "section"
	}
	id := base
	for n := 1; u.seen[id]; n++ {
		id = base + "-" + strconv.Itoa(n)
	}
	u.seen[id] = true
	return id
}
