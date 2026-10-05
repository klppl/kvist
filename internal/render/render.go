package render

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"path"
	"regexp"
	"strings"

	"github.com/klppl/kvist/internal/markdown"
	"github.com/klppl/kvist/internal/model"
)

// Output receives the files of a build.
type Output interface {
	WriteFile(p string, data []byte) error
}

// Page is the data every page template receives.
type Page struct {
	Kind        string // home, note, folder, tag, tags, 404
	Site        *model.Site
	Theme       *Theme
	Title       string
	URL         string
	Description string
	Note        *model.Note   // note pages, and home when a home note exists
	Folder      *model.Folder // folder pages
	Tag         *model.Tag    // tag pages
	Features    markdown.Features
	// SocialImage is the generated preview image of a note page without
	// an image property, when the theme's social_images param is on.
	SocialImage string
}

// Render writes every page and the theme's static files. It does not copy
// attachments; the caller does (they live in the content source).
func Render(site *model.Site, t *Theme, out Output) error {
	pages := map[string]*template.Template{}
	for kind, tmpl := range t.pages {
		c, err := tmpl.Clone()
		if err != nil {
			return err
		}
		pages[kind] = c.Funcs(funcMap(t, site))
	}
	r := &renderer{site: site, theme: t, pages: pages, out: out, written: map[string]string{}}
	msg := t.Messages(site.Config.Language)
	social, err := r.socialImages()
	if err != nil {
		return err
	}

	// Home.
	home := &Page{Kind: "home", Title: site.Config.Title, URL: "/", Description: site.Config.Description, Note: site.Home}
	if site.Home != nil {
		home.Features = site.Home.Features
		if site.Home.Description != "" {
			home.Description = site.Home.Description
		}
	}
	if err := r.page("/", home); err != nil {
		return err
	}
	for _, n := range site.Notes {
		if n.URL == "/" {
			continue // rendered as the home page
		}
		if err := r.page(n.URL, &Page{Kind: "note", Title: n.Title, URL: n.URL, Description: n.Description, Note: n, Features: n.Features, SocialImage: social[n.Path]}); err != nil {
			return err
		}
	}
	if err := r.folders(site.Root); err != nil {
		return err
	}
	if err := r.page("/tags/", &Page{Kind: "tags", Title: msg.T("tags"), URL: "/tags/"}); err != nil {
		return err
	}
	for _, tag := range site.AllTags {
		if err := r.page(tag.URL, &Page{Kind: "tag", Title: "#" + tag.Name, URL: tag.URL, Tag: tag}); err != nil {
			return err
		}
	}
	if err := r.file("/404.html", "404", &Page{Kind: "404", Title: msg.T("not_found"), URL: "/404.html"}); err != nil {
		return err
	}
	if err := r.generated(); err != nil {
		return err
	}
	return r.static()
}

type renderer struct {
	site    *model.Site
	theme   *Theme
	pages   map[string]*template.Template
	out     Output
	written map[string]string // output path → URL, to catch collisions
}

func (r *renderer) folders(f *model.Folder) error {
	if f.URL != "/" && f.Index == nil {
		if err := r.page(f.URL, &Page{Kind: "folder", Title: f.Name, URL: f.URL, Folder: f}); err != nil {
			return err
		}
	}
	for _, c := range f.Children {
		if err := r.folders(c); err != nil {
			return err
		}
	}
	return nil
}

// page renders a page at a directory URL ("/a/b/" → a/b/index.html).
func (r *renderer) page(url string, p *Page) error {
	return r.file(path.Join(url, "index.html"), p.Kind, p)
}

func (r *renderer) file(p, kind string, page *Page) error {
	page.Site, page.Theme = r.site, r.theme
	rel, err := outputPath(p)
	if err != nil {
		return err
	}
	if prev, ok := r.written[rel]; ok {
		return fmt.Errorf("%s and %s both render to %s", prev, page.URL, rel)
	}
	r.written[rel] = page.URL
	var buf bytes.Buffer
	if err := r.pages[kind].ExecuteTemplate(&buf, "base.html", page); err != nil {
		return fmt.Errorf("theme %s: rendering %s: %w", r.theme.Name, page.URL, err)
	}
	return r.out.WriteFile(rel, stripHTMLComments(buf.Bytes()))
}

func (r *renderer) static() error {
	for _, p := range r.theme.static {
		data, err := fs.ReadFile(r.theme.fsys, "static/"+p)
		if err != nil {
			return err
		}
		rel, err := outputPath(r.theme.AssetURL(p))
		if err != nil {
			return err
		}
		if err := r.out.WriteFile(rel, data); err != nil {
			return err
		}
	}
	css, err := syntaxCSS(lookupParam(r.theme, r.site, "code_style_light"), lookupParam(r.theme, r.site, "code_style_dark"))
	if err != nil {
		return err
	}
	rel, _ := outputPath(r.theme.AssetURL("syntax.css"))
	return r.out.WriteFile(rel, []byte(css))
}

// outputPath maps a site URL path to a relative file path, refusing
// anything that could escape the output directory.
func outputPath(u string) (string, error) {
	p := strings.TrimPrefix(path.Clean("/"+u), "/")
	if p == "" || strings.HasPrefix(p, "..") || strings.Contains(p, "\x00") {
		return "", fmt.Errorf("invalid output path %q", u)
	}
	return p, nil
}

var htmlComment = regexp.MustCompile(`(?s)<!--.*?-->`)

// stripHTMLComments removes HTML comments from rendered pages (§5.3). Notes
// have theirs removed before parsing already; this covers templates.
func stripHTMLComments(b []byte) []byte {
	return htmlComment.ReplaceAll(b, nil)
}

var cssSelectorRe = regexp.MustCompile(`\.chroma|\.bg\b`)

// syntaxCSS builds light and dark code highlighting styles. Dark applies
// with data-theme="dark", or with the system preference unless the reader
// chose light.
func syntaxCSS(light, dark any) (string, error) {
	ls, _ := light.(string)
	ds, _ := dark.(string)
	if ls == "" {
		ls = "github"
	}
	if ds == "" {
		ds = "github-dark"
	}
	l, err := markdown.HighlightCSS(ls)
	if err != nil {
		return "", err
	}
	d, err := markdown.HighlightCSS(ds)
	if err != nil {
		return "", err
	}
	explicit := cssSelectorRe.ReplaceAllStringFunc(d, func(s string) string { return `:root[data-theme="dark"] ` + s })
	system := cssSelectorRe.ReplaceAllStringFunc(d, func(s string) string { return `:root:not([data-theme="light"]) ` + s })
	return "/* light: " + ls + " */\n" + l + "\n/* dark: " + ds + " */\n" + explicit +
		"\n@media (prefers-color-scheme: dark) {\n" + system + "}\n", nil
}
