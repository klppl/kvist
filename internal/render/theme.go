// Package render turns a content model into a static site with a theme.
//
// A theme is a folder:
//
//	theme.toml            name, version, license, model = [1], [params]
//	templates/            base.html, note.html, folder.html, tag.html,
//	                      tags.html, home.html, 404.html, partials/*.html
//	static/               copied to /_kvist/<hash>/
//
// Templates are html/template with a small function set (funcs.go) and no
// file system or network access.
package render

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"path"
	"slices"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/klppl/kvist/internal/model"
)

// PageKinds are the page templates a theme must provide.
var PageKinds = []string{"home", "note", "folder", "tag", "tags", "404"}

// Theme is a loaded theme.
type Theme struct {
	Name    string
	Version string
	License string
	Models  []int
	Params  map[string]any

	fsys      fs.FS
	pages     map[string]*template.Template
	static    []string // static file paths relative to static/
	assetBase string   // /_kvist/<hash>/
}

type themeManifest struct {
	Name    string         `toml:"name"`
	Version string         `toml:"version"`
	License string         `toml:"license"`
	Model   []int          `toml:"model"`
	Params  map[string]any `toml:"params"`
}

// overlay serves files from top first, then from base.
type overlay struct{ top, base fs.FS }

func (o overlay) Open(name string) (fs.File, error) {
	if o.top != nil {
		if f, err := o.top.Open(name); err == nil {
			return f, nil
		}
	}
	return o.base.Open(name)
}

// LoadTheme loads a theme from fsys. overrides, if not nil, is a folder with
// the same layout whose files replace the theme's.
func LoadTheme(fsys, overrides fs.FS) (*Theme, error) {
	var fsx fs.FS = fsys
	if overrides != nil {
		fsx = overlay{top: overrides, base: fsys}
	}
	b, err := fs.ReadFile(fsx, "theme.toml")
	if err != nil {
		return nil, fmt.Errorf("theme: %w", err)
	}
	var m themeManifest
	if _, err := toml.Decode(string(b), &m); err != nil {
		return nil, fmt.Errorf("theme.toml: %w", err)
	}
	if m.Name == "" {
		return nil, errors.New("theme.toml: name is required")
	}
	if !slices.Contains(m.Model, model.Version) {
		return nil, fmt.Errorf("theme %q supports content model %v; this kvist produces version %d", m.Name, m.Model, model.Version)
	}
	t := &Theme{Name: m.Name, Version: m.Version, License: m.License, Models: m.Model, Params: m.Params, fsys: fsx, pages: map[string]*template.Template{}}
	if t.Params == nil {
		t.Params = map[string]any{}
	}

	statics, err := listFiles(fsys, overrides, "static")
	if err != nil {
		return nil, err
	}
	t.static = statics
	h := sha256.New()
	for _, p := range statics {
		data, err := fs.ReadFile(fsx, "static/"+p)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(h, "%s\x00%d\x00", p, len(data))
		h.Write(data)
	}
	t.assetBase = "/_kvist/" + hex.EncodeToString(h.Sum(nil))[:10] + "/"

	partials, err := listFiles(fsys, overrides, "templates/partials")
	if err != nil {
		return nil, err
	}
	base := template.New("base.html").Funcs(funcMap(t, nil))
	if err := parseInto(base, fsx, "templates/base.html"); err != nil {
		return nil, err
	}
	for _, p := range partials {
		if strings.HasSuffix(p, ".html") {
			if err := parseInto(base, fsx, "templates/partials/"+p); err != nil {
				return nil, err
			}
		}
	}
	for _, kind := range PageKinds {
		tmpl, err := base.Clone()
		if err != nil {
			return nil, err
		}
		if err := parseInto(tmpl, fsx, "templates/"+kind+".html"); err != nil {
			return nil, err
		}
		t.pages[kind] = tmpl
	}
	return t, nil
}

func parseInto(t *template.Template, fsys fs.FS, name string) error {
	b, err := fs.ReadFile(fsys, name)
	if err != nil {
		return fmt.Errorf("theme: %w", err)
	}
	target := t
	if t.Name() != path.Base(name) {
		target = t.New(path.Base(name))
	}
	if _, err := target.Parse(string(b)); err != nil {
		return fmt.Errorf("theme: %w", err)
	}
	return nil
}

// listFiles lists regular files under dir in both file systems, relative to
// dir, sorted and without duplicates.
func listFiles(base, top fs.FS, dir string) ([]string, error) {
	seen := map[string]bool{}
	for _, fsys := range []fs.FS{base, top} {
		if fsys == nil {
			continue
		}
		err := fs.WalkDir(fsys, dir, func(p string, d fs.DirEntry, err error) error {
			if errors.Is(err, fs.ErrNotExist) && p == dir {
				return fs.SkipDir
			}
			if err != nil {
				return err
			}
			if d.Type().IsRegular() && !strings.HasPrefix(d.Name(), ".") {
				seen[strings.TrimPrefix(p, dir+"/")] = true
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out, nil
}

// AssetURL returns the URL of a theme static file.
func (t *Theme) AssetURL(p string) string { return t.assetBase + strings.TrimPrefix(p, "/") }
