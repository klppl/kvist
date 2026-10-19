package render

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/klppl/kvist/internal/model"
)

func TestPreviewable(t *testing.T) {
	for u, want := range map[string]bool{
		"":                                  false,
		"/_assets/x/a.png":                  true,
		"/_assets/x/a.JPG":                  true,
		"/_assets/x/a.svg":                  false,
		"/_assets/x/a.avif":                 false,
		"https://cdn.example.com/a.svg":     false,
		"https://cdn.example.com/a.svg?w=2": false,
		"https://cdn.example.com/pic":       true,
	} {
		if got := previewable(u); got != want {
			t.Errorf("previewable(%q) = %v, want %v", u, got, want)
		}
	}
}

func TestAbsoluteURL(t *testing.T) {
	site := &model.Site{Config: model.SiteConfig{BaseURL: "https://example.com"}}
	for in, want := range map[string]string{
		"/":                   "https://example.com/",
		"/garden/räksmörgås/": "https://example.com/garden/r%C3%A4ksm%C3%B6rg%C3%A5s/",
		"/_assets/x/a b.png":  "https://example.com/_assets/x/a%20b.png",
		"/a/%C3%A4/":          "https://example.com/a/%C3%A4/",
		"notes/#top":          "https://example.com/notes/#top",
		"https://other.org/ä": "https://other.org/ä",
	} {
		if got := absoluteURL(site, in); got != want {
			t.Errorf("absoluteURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestListDescription(t *testing.T) {
	if got := listDescription("2 notes", []string{"Soil", "Tomatoes"}); got != "2 notes: Soil, Tomatoes" {
		t.Errorf("got %q", got)
	}
	if got := listDescription("0 notes", nil); got != "" {
		t.Errorf("empty list: got %q", got)
	}
	long := listDescription("40 notes", strings.Split(strings.Repeat("A fairly long title,", 40), ","))
	if n := len([]rune(long)); n > 161 || !strings.HasSuffix(long, "…") {
		t.Errorf("not cut to a snippet (%d): %q", n, long)
	}
}

func TestStructuredData(t *testing.T) {
	site := &model.Site{Config: model.SiteConfig{Title: "Garden", BaseURL: "https://example.com", Language: "en", Author: "Ada",
		Profile: &model.Profile{Links: []model.ProfileLink{{URL: "https://github.com/ada"}, {URL: "mailto:ada@example.com"}}}}}
	root := &model.Folder{URL: "/"}
	sub := &model.Folder{Name: "Kitchen", URL: "/kitchen/", Parent: root}
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	note := &model.Note{Title: "Bread </script>", URL: "/kitchen/bread/", Folder: sub, Created: day, Updated: day, TagNames: []string{"food"}, WordCount: 12}
	sd := structuredData(garden(t).Messages("en"), site)

	got, err := sd(&Page{Kind: "note", Site: site, Title: note.Title, URL: note.URL, Canonical: note.URL, Note: note, SocialImage: "/_kvist/social/x.png"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "</script>") {
		t.Fatalf("JSON-LD can end its script element: %s", got)
	}
	var graph []map[string]any
	if err := json.Unmarshal([]byte(got), &graph); err != nil || len(graph) != 2 {
		t.Fatalf("want an article and breadcrumbs: %v %s", err, got)
	}
	a := graph[0]
	if a["@type"] != "Article" || a["headline"] != note.Title || a["datePublished"] != "2026-03-01T00:00:00Z" ||
		a["image"] != "https://example.com/_kvist/social/x.png" || a["keywords"] != "food" {
		t.Errorf("article: %v", a)
	}
	if au, _ := a["author"].(map[string]any); au["name"] != "Ada" || len(au["sameAs"].([]any)) != 1 {
		t.Errorf("author: %v", a["author"])
	}
	items := graph[1]["itemListElement"].([]any)
	if len(items) != 3 || items[1].(map[string]any)["item"] != "https://example.com/kitchen/" {
		t.Errorf("breadcrumbs: %v", items)
	}

	// The home note's own page describes the home page, without a trail.
	got, _ = sd(&Page{Kind: "note", Site: site, Title: note.Title, URL: note.URL, Canonical: "/", Note: note})
	if !strings.HasPrefix(string(got), "{") || !strings.Contains(string(got), `"url":"https://example.com/"`) {
		t.Errorf("home note page: %s", got)
	}
	if got, _ := sd(&Page{Kind: "home", Site: site, URL: "/", Canonical: "/"}); !strings.Contains(string(got), `"@type":"WebSite"`) {
		t.Errorf("home: %s", got)
	}
	// Layouts: an about page is the author's profile page, with the
	// avatar as their picture; page and intro are web pages.
	site.Config.Profile.Avatar = "/_assets/x/ada.png"
	about := &model.Note{Title: "About", URL: "/about/", Layout: "about", Created: day, Updated: day}
	got, _ = sd(&Page{Kind: "note", Site: site, Title: about.Title, URL: about.URL, Canonical: about.URL, Note: about})
	var pp map[string]any
	if err := json.Unmarshal([]byte(got), &pp); err != nil || pp["@type"] != "ProfilePage" || pp["dateCreated"] != "2026-03-01T00:00:00Z" {
		t.Fatalf("about page: %v %s", err, got)
	}
	if m, _ := pp["mainEntity"].(map[string]any); m["@type"] != "Person" || m["name"] != "Ada" || m["image"] != "https://example.com/_assets/x/ada.png" {
		t.Errorf("about page person: %v", pp["mainEntity"])
	}
	for _, l := range []string{"page", "intro"} {
		n := &model.Note{Title: "Now", URL: "/now/", Layout: l, Created: day, Updated: day}
		got, _ = sd(&Page{Kind: "note", Site: site, Title: n.Title, URL: n.URL, Canonical: n.URL, Note: n})
		if !strings.Contains(string(got), `"@type":"WebPage"`) || strings.Contains(string(got), "Article") {
			t.Errorf("%s layout: %s", l, got)
		}
	}
	site.Config.Author = ""
	got, _ = sd(&Page{Kind: "note", Site: site, Title: about.Title, URL: about.URL, Canonical: about.URL, Note: about})
	if !strings.Contains(string(got), `"@type":"WebPage"`) {
		t.Errorf("about page without an author: %s", got)
	}
	if got, _ := sd(&Page{Kind: "404", Site: site}); got != "" {
		t.Errorf("404 has structured data: %s", got)
	}
}
