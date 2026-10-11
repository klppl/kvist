package render

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"sort"
	"time"
	"unicode/utf8"

	"github.com/klppl/kvist/internal/model"
)

// Limits for generated files.
const (
	searchTextLimit = 20000 // characters of note text in the search index
	feedItems       = 30
)

// generated writes the theme-independent files: the search index, the
// graph, the RSS feed, the sitemap and robots.txt. All of them derive from
// the model, which only holds published notes, and leave out unlisted ones.
func (r *renderer) generated() error {
	for _, g := range []struct {
		path string
		fn   func(*model.Site) ([]byte, error)
	}{
		{"search-index.json", searchIndex},
		{"graph.json", graphJSON},
		{"index.xml", feed},
		{"sitemap.xml", sitemap},
		{"robots.txt", robots},
	} {
		b, err := g.fn(r.site)
		if err != nil {
			return err
		}
		if err := r.out.WriteFile(g.path, b); err != nil {
			return err
		}
	}
	return nil
}

type searchDoc struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	URL         string   `json:"url"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Aliases     []string `json:"aliases,omitempty"`
	Text        string   `json:"text"`
}

func searchIndex(s *model.Site) ([]byte, error) {
	docs := make([]searchDoc, 0, len(s.Notes))
	for _, n := range s.Notes {
		if n.Unlisted {
			continue
		}
		text := n.Text
		if utf8.RuneCountInString(text) > searchTextLimit {
			text = string([]rune(text)[:searchTextLimit])
		}
		docs = append(docs, searchDoc{ID: n.ID, Title: n.Title, URL: n.URL, Description: n.Description, Tags: n.TagNames, Aliases: n.Aliases, Text: text})
	}
	return marshal(map[string]any{"version": 1, "docs": docs})
}

func graphJSON(s *model.Site) ([]byte, error) { return marshal(s.Graph) }

func marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	err := enc.Encode(v)
	return buf.Bytes(), err
}

// --- RSS 2.0 ---

type rss struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Atom    string     `xml:"xmlns:atom,attr"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title         string    `xml:"title"`
	Link          string    `xml:"link"`
	Description   string    `xml:"description"`
	Language      string    `xml:"language,omitempty"`
	LastBuildDate string    `xml:"lastBuildDate"`
	Generator     string    `xml:"generator"`
	Self          atomLink  `xml:"atom:link"`
	Items         []rssItem `xml:"item"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
	Type string `xml:"type,attr"`
}

type rssItem struct {
	Title       string   `xml:"title"`
	Link        string   `xml:"link"`
	GUID        rssGUID  `xml:"guid"`
	PubDate     string   `xml:"pubDate"`
	Description string   `xml:"description,omitempty"`
	Categories  []string `xml:"category,omitempty"`
}

type rssGUID struct {
	Value       string `xml:",chardata"`
	IsPermaLink bool   `xml:"isPermaLink,attr"`
}

// newest returns the listed notes sorted by creation time, newest first.
func newest(s *model.Site) []*model.Note {
	notes := listed(s.Notes)
	sort.SliceStable(notes, func(i, j int) bool {
		if !notes[i].Created.Equal(notes[j].Created) {
			return notes[i].Created.After(notes[j].Created)
		}
		return notes[i].URL < notes[j].URL
	})
	return notes
}

func feed(s *model.Site) ([]byte, error) {
	base := s.Config.BaseURL
	desc := s.Config.Description
	if desc == "" {
		desc = s.Config.Title
	}
	ch := rssChannel{
		Title:         s.Config.Title,
		Link:          absoluteURL(s, "/"),
		Description:   desc,
		Language:      s.Config.Language,
		LastBuildDate: s.BuiltAt.UTC().Format(time.RFC1123Z),
		Generator:     "kvist",
		Self:          atomLink{Href: base + "/index.xml", Rel: "self", Type: "application/rss+xml"},
	}
	for i, n := range newest(s) {
		if i == feedItems {
			break
		}
		ch.Items = append(ch.Items, rssItem{
			Title:       n.Title,
			Link:        absoluteURL(s, n.URL),
			GUID:        rssGUID{Value: absoluteURL(s, n.URL), IsPermaLink: true},
			PubDate:     n.Created.UTC().Format(time.RFC1123Z),
			Description: n.Description,
			Categories:  n.TagNames,
		})
	}
	b, err := xml.MarshalIndent(rss{Version: "2.0", Atom: "http://www.w3.org/2005/Atom", Channel: ch}, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), append(b, '\n')...), nil
}

// --- sitemap ---

type urlset struct {
	XMLName xml.Name     `xml:"urlset"`
	NS      string       `xml:"xmlns,attr"`
	URLs    []sitemapURL `xml:"url"`
}

type sitemapURL struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod,omitempty"`
}

// sitemap lists every listed page. Folder and tag pages are as new as
// the newest note they list, so crawlers revisit them when a note changes.
func sitemap(s *model.Site) ([]byte, error) {
	seen := map[string]bool{}
	var urls []sitemapURL
	add := func(u string, t time.Time) {
		if seen[u] {
			return
		}
		seen[u] = true
		e := sitemapURL{Loc: absoluteURL(s, u)}
		if !t.IsZero() {
			e.LastMod = t.UTC().Format("2006-01-02")
		}
		urls = append(urls, e)
	}
	homeTime := s.BuiltAt
	if s.Home != nil {
		homeTime = s.Home.Updated
	}
	add("/", homeTime)
	for _, n := range listed(s.Notes) {
		if n != s.Home { // its content is the home page's; its own page points there
			add(n.URL, n.Updated)
		}
	}
	var folders func(f *model.Folder) time.Time
	folders = func(f *model.Folder) time.Time {
		t := latest(f.Notes)
		for _, c := range f.Children {
			if ct := folders(c); ct.After(t) {
				t = ct
			}
		}
		if f.URL != "/" {
			add(f.URL, t)
		}
		return t
	}
	if s.Root != nil {
		folders(s.Root)
	}
	var tagged []*model.Note
	for _, t := range s.Tags {
		tagged = append(tagged, t.Notes...)
	}
	add("/tags/", latest(tagged))
	for _, t := range s.AllTags {
		add(t.URL, latest(t.Notes))
	}
	sort.Slice(urls, func(i, j int) bool { return urls[i].Loc < urls[j].Loc })
	b, err := xml.MarshalIndent(urlset{NS: "http://www.sitemaps.org/schemas/sitemap/0.9", URLs: urls}, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), append(b, '\n')...), nil
}

// latest returns the newest update time of notes, zero for none.
func latest(notes []*model.Note) time.Time {
	var t time.Time
	for _, n := range notes {
		if n.Updated.After(t) {
			t = n.Updated
		}
	}
	return t
}

// listed returns a copy of notes without the unlisted ones.
func listed(notes []*model.Note) []*model.Note {
	out := make([]*model.Note, 0, len(notes))
	for _, n := range notes {
		if !n.Unlisted {
			out = append(out, n)
		}
	}
	return out
}

func robots(s *model.Site) ([]byte, error) {
	return []byte("User-agent: *\nAllow: /\n\nSitemap: " + absoluteURL(s, "/sitemap.xml") + "\n"), nil
}
