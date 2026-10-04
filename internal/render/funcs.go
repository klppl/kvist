package render

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/klppl/kvist/internal/markdown"
	"github.com/klppl/kvist/internal/model"
)

// funcMap is the documented template function set (docs/themes.md).
func funcMap(t *Theme, site *model.Site) template.FuncMap {
	return template.FuncMap{
		// asset returns the URL of a theme static file: {{asset "style.css"}}.
		"asset": func(p string) string { return t.AssetURL(p) },
		// absURL makes a site URL absolute with the base URL.
		"absURL": func(u string) string {
			if site == nil {
				return u
			}
			return site.Config.BaseURL + "/" + strings.TrimPrefix(u, "/")
		},
		// dateFormat formats a time with a Go layout: {{dateFormat "2 Jan 2006" .Updated}}.
		"dateFormat": func(layout string, v time.Time) string { return v.Format(layout) },
		// isoDate formats a time as RFC 3339.
		"isoDate": func(v time.Time) string { return v.UTC().Format(time.RFC3339) },
		// json encodes a value for use inside <script>.
		"json": func(v any) (template.JS, error) {
			b, err := json.Marshal(v)
			return template.JS(b), err
		},
		// markdownify renders a string as Markdown (links stay plain text).
		"markdownify": func(s string) (template.HTML, error) {
			d := markdown.Parse([]byte(s))
			d.Resolve(nullResolver{})
			out, err := d.Render()
			out = strings.TrimSpace(out)
			if strings.HasPrefix(out, "<p>") && strings.HasSuffix(out, "</p>") && strings.Count(out, "<p>") == 1 {
				out = out[3 : len(out)-4]
			}
			return template.HTML(out), err
		},
		// truncate shortens text to n characters on a word boundary.
		"truncate": func(n int, s string) string {
			if utf8.RuneCountInString(s) <= n {
				return s
			}
			r := string([]rune(s)[:n])
			if i := strings.LastIndexByte(r, ' '); i > len(r)/2 {
				r = r[:i]
			}
			return strings.TrimRight(r, " ,.;:") + "…"
		},
		// param reads a theme parameter (site config over theme defaults).
		"param": func(key string) any { return lookupParam(t, site, key) },
		// default returns v unless it is empty, else def: {{default "x" .Value}}.
		"default": func(def, v any) any {
			if isEmpty(v) {
				return def
			}
			return v
		},
		// recent returns the n most recently updated notes.
		"recent": func(nv any) []*model.Note {
			if site == nil {
				return nil
			}
			return byUpdated(toInt(nv), site.Notes)
		},
		// newest returns the n most recently updated of the given notes
		// (all of them for n <= 0): {{range newest 3 .Tag.Notes}}.
		"newest": func(nv any, notes []*model.Note) []*model.Note {
			return byUpdated(toInt(nv), notes)
		},
		// dict builds a map for passing several values to a template.
		"dict": func(kv ...any) (map[string]any, error) {
			if len(kv)%2 != 0 {
				return nil, errors.New("dict: odd number of arguments")
			}
			m := make(map[string]any, len(kv)/2)
			for i := 0; i < len(kv); i += 2 {
				k, ok := kv[i].(string)
				if !ok {
					return nil, fmt.Errorf("dict: key %v is not a string", kv[i])
				}
				m[k] = kv[i+1]
			}
			return m, nil
		},
		"lower": strings.ToLower,
		"upper": strings.ToUpper,
		"title": func(s string) string {
			if s == "" {
				return s
			}
			r, size := utf8.DecodeRuneInString(s)
			return strings.ToUpper(string(r)) + s[size:]
		},
		"join":      strings.Join,
		"hasPrefix": strings.HasPrefix,
		"contains":  strings.Contains,
		"add":       func(a, b int) int { return a + b },
		"sub":       func(a, b int) int { return a - b },
		"str":       func(v any) string { return fmt.Sprint(v) },
	}
}

func toInt(v any) int {
	switch v := v.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return 0
}

// byUpdated sorts a copy of notes by update time, newest first (ties by
// URL, so builds stay deterministic), and keeps the first n (all for n <= 0).
func byUpdated(n int, notes []*model.Note) []*model.Note {
	out := append([]*model.Note(nil), notes...)
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].Updated.Equal(out[j].Updated) {
			return out[i].Updated.After(out[j].Updated)
		}
		return out[i].URL < out[j].URL
	})
	if n > 0 && len(out) > n {
		out = out[:n]
	}
	return out
}

func lookupParam(t *Theme, site *model.Site, key string) any {
	if site != nil {
		if v, ok := site.Config.Params[key]; ok {
			return v
		}
	}
	return t.Params[key]
}

func isEmpty(v any) bool {
	switch v := v.(type) {
	case nil:
		return true
	case string:
		return v == ""
	case []any:
		return len(v) == 0
	case map[string]any:
		return len(v) == 0
	case bool:
		return !v
	case int:
		return v == 0
	case int64:
		return v == 0
	case float64:
		return v == 0
	}
	return false
}

type nullResolver struct{}

func (nullResolver) Resolve(markdown.Ref) markdown.Target { return markdown.Target{} }
func (nullResolver) Embed(markdown.Target, markdown.Ref) (string, bool) {
	return "", false
}
func (nullResolver) TagURL(string) string { return "" }
