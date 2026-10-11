package render

import (
	"encoding/json"
	"html/template"
	"path"
	"strings"
	"time"

	"github.com/klppl/kvist/internal/model"
)

// Search engine and social preview helpers. Like every other output they
// read only the published model (§5.3).

// previewImageExts are image types social networks and search engines
// don't show as a preview (Open Graph, X cards, Google's image previews).
// A note whose image is one of these gets a generated image instead.
var previewImageExts = map[string]bool{
	".svg": true, ".svgz": true, ".avif": true, ".heic": true, ".heif": true,
	".tif": true, ".tiff": true, ".bmp": true, ".ico": true,
}

// previewable reports whether an image URL can be a social preview. URLs
// without a known bad extension (also external ones without any) pass.
func previewable(u string) bool {
	if u == "" {
		return false
	}
	if i := strings.IndexAny(u, "?#"); i >= 0 {
		u = u[:i]
	}
	return !previewImageExts[strings.ToLower(path.Ext(u))]
}

// pageImage is the preview image of a page: the note's image, else its
// generated image, else the site's image, skipping ones that can't be
// previews.
func pageImage(p *Page) string {
	if p.Note != nil && previewable(p.Note.Image) {
		return p.Note.Image
	}
	if p.SocialImage != "" {
		return p.SocialImage
	}
	if previewable(p.Site.Config.Image) {
		return p.Site.Config.Image
	}
	return ""
}

// escapePath percent-encodes what may not appear as-is in a URL (spaces,
// non-ASCII letters in slugs, …), so absolute URLs in meta tags, the
// sitemap and the feed are valid URIs. Existing escapes, query and
// fragment are left alone.
func escapePath(u string) string {
	const keep = "-._~!$&'()*+,;=:@/%?#"
	var b strings.Builder
	for i := 0; i < len(u); i++ {
		c := u[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.IndexByte(keep, c) >= 0 {
			b.WriteByte(c)
			continue
		}
		const hex = "0123456789ABCDEF"
		b.WriteByte('%')
		b.WriteByte(hex[c>>4])
		b.WriteByte(hex[c&15])
	}
	return b.String()
}

// absoluteURL makes a site URL (or an http(s) URL, kept) absolute.
func absoluteURL(site *model.Site, u string) string {
	if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
		return u
	}
	return site.Config.BaseURL + escapePath("/"+strings.TrimPrefix(u, "/"))
}

// listDescription describes a folder or tag page by what it lists:
// "3 notes: Soil, Tomatoes, Watering", cut to a snippet's length.
func listDescription(count string, names []string) string {
	if len(names) == 0 {
		return ""
	}
	return truncateText(160, count+": "+strings.Join(names, ", "))
}

func noteTitles(notes []*model.Note) []string {
	out := make([]string, 0, len(notes))
	for _, n := range notes {
		out = append(out, n.Title)
	}
	return out
}

// structuredData returns the schema.org JSON-LD of a page (an Article,
// the WebSite or a CollectionPage, and its breadcrumbs), or "" for none.
// Maps marshal with sorted keys, so the output is deterministic.
func structuredData(msg messages, site *model.Site) func(*Page) (template.JS, error) {
	return func(p *Page) (template.JS, error) {
		if site == nil || p == nil || p.Kind == "404" {
			return "", nil
		}
		c := site.Config
		abs := func(u string) string { return absoluteURL(site, u) }
		website := map[string]any{"@type": "WebSite", "name": c.Title, "url": abs("/")}
		var author map[string]any
		if c.Author != "" {
			author = map[string]any{"@type": "Person", "name": c.Author, "url": abs("/")}
			if c.Profile != nil {
				var same []string
				for _, l := range c.Profile.Links {
					if strings.HasPrefix(l.URL, "https://") || strings.HasPrefix(l.URL, "http://") {
						same = append(same, l.URL)
					}
				}
				if len(same) > 0 {
					author["sameAs"] = same
				}
			}
		}
		set := func(m map[string]any, k string, v any) {
			switch v := v.(type) {
			case string:
				if v == "" {
					return
				}
			case map[string]any:
				if v == nil {
					return
				}
			case time.Time:
				if v.IsZero() {
					return
				}
				m[k] = v.UTC().Format(time.RFC3339)
				return
			}
			m[k] = v
		}
		img := pageImage(p)
		if img != "" {
			img = abs(img)
		}

		var main map[string]any
		var crumbs []crumb
		switch p.Kind {
		case "home":
			main = website
			set(main, "description", p.Description)
			set(main, "inLanguage", c.Language)
			set(main, "author", author)
			set(main, "image", img)
		case "note":
			n := p.Note
			main = map[string]any{"@type": "Article", "headline": p.Title, "url": abs(p.Canonical), "mainEntityOfPage": abs(p.Canonical), "isPartOf": website}
			set(main, "description", p.Description)
			set(main, "datePublished", n.Created)
			set(main, "dateModified", n.Updated)
			set(main, "inLanguage", c.Language)
			set(main, "author", author)
			set(main, "image", img)
			set(main, "keywords", strings.Join(n.TagNames, ", "))
			if n.WordCount > 0 {
				main["wordCount"] = n.WordCount
			}
			// Like the theme's breadcrumbs: a folder note is its folder's
			// page, so its trail ends at the folder above.
			f := n.Folder
			if f != nil && f.URL == n.URL {
				f = f.Parent
			}
			if f != nil && f.Parent != nil && p.Canonical == p.URL {
				crumbs = append(folderCrumbs(msg.T("home"), f), crumb{p.Title, p.URL})
			}
		case "folder", "tag", "tags":
			main = map[string]any{"@type": "CollectionPage", "name": p.Title, "url": abs(p.URL), "isPartOf": website}
			set(main, "description", p.Description)
			set(main, "inLanguage", c.Language)
			switch {
			case p.Folder != nil && p.Folder.Parent != nil:
				crumbs = append(folderCrumbs(msg.T("home"), p.Folder.Parent), crumb{p.Title, p.URL})
			case p.Tag != nil:
				crumbs = []crumb{{msg.T("tags"), "/tags/"}}
				var up []crumb
				for t := p.Tag; t != nil; t = t.Parent {
					up = append([]crumb{{"#" + t.Name, t.URL}}, up...)
				}
				crumbs = append(crumbs, up...)
			}
		default:
			return "", nil
		}
		graph := []any{withContext(main)}
		if len(crumbs) > 0 {
			items := make([]any, len(crumbs))
			for i, cr := range crumbs {
				items[i] = map[string]any{"@type": "ListItem", "position": i + 1, "name": cr.Name, "item": abs(cr.URL)}
			}
			graph = append(graph, withContext(map[string]any{"@type": "BreadcrumbList", "itemListElement": items}))
		}
		var v any = graph[0]
		if len(graph) > 1 {
			v = graph
		}
		// json.Marshal escapes <, > and &, so the result can't end the
		// <script> element it is written into.
		b, err := json.Marshal(v)
		return template.JS(b), err
	}
}

type crumb struct{ Name, URL string }

// folderCrumbs is the trail from the home page to f: home, then each
// listed folder below the root (unlisted ones have no page).
func folderCrumbs(home string, f *model.Folder) []crumb {
	var out []crumb
	for ; f != nil && f.Parent != nil; f = f.Parent {
		if !f.Unlisted {
			out = append([]crumb{{f.Name, f.URL}}, out...)
		}
	}
	return append([]crumb{{home, "/"}}, out...)
}

func withContext(m map[string]any) map[string]any {
	m["@context"] = "https://schema.org"
	return m
}
