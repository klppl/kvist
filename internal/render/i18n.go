package render

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// A theme's words live in i18n/<language>.toml, one file per language,
// so a site's interface follows its language setting. A file in
// theme_overrides adds to the theme's file of the same name, key by key,
// so a site can change a single word or add a language.
//
//	search = "Search"
//	notes  = { one = "%d note", other = "%d notes" }   # chosen by count
//	months = ["January", …]                            # for dateFormat
//
// A word missing from the site's language comes from en.toml, then from
// the key itself.

// fallbackLanguage is the language every theme should translate fully.
const fallbackLanguage = "en"

// loadI18n reads i18n/*.toml from the theme and then its overrides.
func loadI18n(base, top fs.FS) (map[string]map[string]any, error) {
	out := map[string]map[string]any{}
	for _, fsys := range []fs.FS{base, top} {
		if fsys == nil {
			continue
		}
		files, err := listFiles(fsys, nil, "i18n")
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			lang, ok := strings.CutSuffix(f, ".toml")
			if !ok || strings.Contains(lang, "/") {
				continue
			}
			b, err := fs.ReadFile(fsys, "i18n/"+f)
			if err != nil {
				return nil, err
			}
			var m map[string]any
			if _, err := toml.Decode(string(b), &m); err != nil {
				return nil, fmt.Errorf("theme: i18n/%s: %w", f, err)
			}
			lang = strings.ToLower(lang)
			if out[lang] == nil {
				out[lang] = map[string]any{}
			}
			for k, v := range m {
				out[lang][k] = v
			}
		}
	}
	return out, nil
}

// messages are a theme's words in one language, with the fallbacks
// applied.
type messages map[string]any

// Messages returns the theme's words for a language code such as "sv" or
// "pt-BR": that language, then its base language, then English.
func (t *Theme) Messages(lang string) messages {
	m := messages{}
	lang = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(lang)), "_", "-")
	chain := []string{fallbackLanguage}
	if base, _, _ := strings.Cut(lang, "-"); base != "" && base != fallbackLanguage {
		chain = append(chain, base)
	}
	if lang != "" && !contains(chain, lang) {
		chain = append(chain, lang)
	}
	for _, l := range chain {
		for k, v := range t.i18n[l] {
			m[k] = v
		}
	}
	return m
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// T returns the word for key. A table of plural forms picks "one" for a
// count of 1 (the first argument) and "other" otherwise; arguments fill
// the word's %d and %s verbs.
func (m messages) T(key string, args ...any) string {
	v, ok := m[key]
	if !ok {
		return key
	}
	if forms, ok := v.(map[string]any); ok {
		form := "other"
		if len(args) > 0 && toInt(args[0]) == 1 {
			form = "one"
		}
		v = forms[form]
		if v == nil {
			v = forms["other"]
		}
	}
	s, ok := v.(string)
	if !ok {
		return key
	}
	if len(args) > 0 && strings.Contains(s, "%") {
		return fmt.Sprintf(s, args...)
	}
	return s
}

// Table returns the string values of a table, such as the words a theme's
// script needs: {{json (i18n "script")}}.
func (m messages) Table(key string) map[string]string {
	t, _ := m[key].(map[string]any)
	out := make(map[string]string, len(t))
	for k, v := range t {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
}

// list returns a list of n strings, or nil.
func (m messages) list(key string, n int) []string {
	l, ok := m[key].([]any)
	if !ok || len(l) != n {
		return nil
	}
	out := make([]string, n)
	for i, v := range l {
		s, ok := v.(string)
		if !ok {
			return nil
		}
		out[i] = s
	}
	return out
}

// layoutNames are the name elements of Go time layouts, longest first so
// "January" is not read as "Jan" + "uary".
var layoutNames = []string{"January", "Monday", "Jan", "Mon"}

// FormatDate formats v with a Go layout, with month and day names in the
// language (months, months_short, days and days_short; days start on
// Sunday). Names a language doesn't give stay English.
func (m messages) FormatDate(layout string, v time.Time) string {
	names := map[string][]string{
		"January": m.list("months", 12), "Jan": m.list("months_short", 12),
		"Monday": m.list("days", 7), "Mon": m.list("days_short", 7),
	}
	var b strings.Builder
	for layout != "" {
		i, tok := len(layout), ""
		for _, n := range layoutNames {
			if j := strings.Index(layout, n); j >= 0 && (j < i || j == i && len(n) > len(tok)) {
				i, tok = j, n
			}
		}
		b.WriteString(v.Format(layout[:i]))
		if tok == "" {
			break
		}
		switch l := names[tok]; {
		case l == nil:
			b.WriteString(v.Format(tok))
		case tok == "January" || tok == "Jan":
			b.WriteString(l[v.Month()-1])
		default:
			b.WriteString(l[v.Weekday()])
		}
		layout = layout[i+len(tok):]
	}
	return b.String()
}

// languages lists the theme's languages, for tests and docs.
func (t *Theme) languages() []string {
	out := make([]string, 0, len(t.i18n))
	for l := range t.i18n {
		out = append(out, l)
	}
	sort.Strings(out)
	return out
}
