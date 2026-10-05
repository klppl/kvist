package model

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/klppl/kvist/internal/config"
	"github.com/klppl/kvist/internal/source/dir"
)

const settingsConfig = `
data_dir = "unused"
[[site]]
id = "garden"
base_url = "https://garden.example.com"
title = "Server title"
description = "Server description"
  [site.publish]
  always_public_folders = ["Kvist"]
  exclude_folders = ["Private"]
  [site.theme_params]
  accent = "#000000"
  footer = "server footer"
`

func buildVault(t *testing.T, files map[string]string) *Site {
	t.Helper()
	d := t.TempDir()
	for p, c := range files {
		full := filepath.Join(d, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := config.Parse([]byte(settingsConfig))
	if err != nil {
		t.Fatal(err)
	}
	snap, err := dir.New(d).Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	site, err := Build(cfg.Sites[0], snap)
	if err != nil {
		t.Fatal(err)
	}
	return site
}

func TestSettingsNote(t *testing.T) {
	site := buildVault(t, map[string]string{
		"Kvist/Welcome.md":  "# Welcome\nHi.",
		"Kvist/About me.md": "About.",
		"Private/Secret.md": "SECRET",
		"Kvist/_site.md": `---
title: Ada's garden
description:
home: "[[Welcome]]"
groups: "articles, #projects"
accent: "#8a4f9e"
base_url: https://evil.example.com
tags: [settings]
---
Settings text with an inline [[About me]] link that is not a menu entry.

## Links
- [Mastodon](https://mastodon.social/@ada)
- [Tags](/tags/)
- [[About me]]
- [[About me|Me]]
- [[Secret]]

%%
- [Example](/example/)
%%
`,
	})

	c := site.Config
	if c.Title != "Ada's garden" {
		t.Errorf("title = %q", c.Title)
	}
	if c.Description != "Server description" {
		t.Errorf("an empty property must keep the server's value, got %q", c.Description)
	}
	if c.Params["accent"] != "#8a4f9e" || c.Params["footer"] != "server footer" {
		t.Errorf("params = %v", c.Params)
	}
	if got := c.Params["groups"]; !reflect.DeepEqual(got, []any{"articles", "projects"}) {
		t.Errorf("groups = %#v", got)
	}
	if _, ok := c.Params["base_url"]; ok || strings.Contains(c.BaseURL, "evil") {
		t.Error("a server-only key reached the site config")
	}
	if _, ok := c.Params["tags"]; ok {
		t.Error("Obsidian's tags property became a theme setting")
	}
	want := []NavItem{
		{Title: "Mastodon", URL: "https://mastodon.social/@ada"},
		{Title: "Tags", URL: "/tags/"},
		{Title: "About me", URL: "/kvist/about-me/"},
		{Title: "Me", URL: "/kvist/about-me/"},
	}
	if !reflect.DeepEqual(c.Nav, want) {
		t.Errorf("nav = %+v", c.Nav)
	}
	if site.Home == nil || site.Home.URL != "/kvist/welcome/" {
		t.Errorf("home = %+v", site.Home)
	}
	for _, n := range site.Notes {
		if strings.Contains(n.Path, "_site") || strings.Contains(n.URL, "site") {
			t.Errorf("the settings note was published as %s", n.URL)
		}
	}
	var warned []string
	for _, w := range site.Warnings {
		warned = append(warned, w.Message)
	}
	all := strings.Join(warned, "\n")
	if !strings.Contains(all, `"base_url" can only be set in the server config`) {
		t.Errorf("no warning for base_url: %s", all)
	}
	if !strings.Contains(all, `menu link "Secret"`) || strings.Contains(all, "Example") {
		t.Errorf("menu link warnings: %s", all)
	}
}

func TestSettingsNoteOverridesSiteToml(t *testing.T) {
	site := buildVault(t, map[string]string{
		"Kvist/Note.md": "x",
		".kvist/site.toml": `title = "From toml"
author = "Toml author"
[[nav]]
title = "Toml link"
url = "/toml/"
[theme_params]
nav_tags = ["old"]
`,
		"_site.md": "---\ntitle: From note\n---\n",
	})
	c := site.Config
	if c.Title != "From note" || c.Author != "Toml author" {
		t.Errorf("title %q, author %q", c.Title, c.Author)
	}
	if len(c.Nav) != 1 || c.Nav[0].URL != "/toml/" {
		t.Errorf("the note has no links, so site.toml's nav stays: %+v", c.Nav)
	}
	if got := c.Params["groups"]; !reflect.DeepEqual(got, []any{"old"}) {
		t.Errorf("nav_tags should become groups, got %#v", got)
	}
}

func TestTwoSettingsNotes(t *testing.T) {
	d := t.TempDir()
	for _, p := range []string{"_site.md", "Kvist/_site.md"} {
		os.MkdirAll(filepath.Join(d, filepath.Dir(p)), 0o755)
		os.WriteFile(filepath.Join(d, p), []byte("x"), 0o644)
	}
	_, err := dir.New(d).Snapshot(context.Background())
	if err == nil || !strings.Contains(err.Error(), "keep one") {
		t.Errorf("expected an error naming both notes, got %v", err)
	}
}

func TestStrictLineBreaks(t *testing.T) {
	note := "one\ntwo"
	site := buildVault(t, map[string]string{"Kvist/Note.md": note})
	if c := string(site.Notes[0].Content); !strings.Contains(c, "<br>") {
		t.Errorf("line breaks should become <br> by default: %s", c)
	}
	site = buildVault(t, map[string]string{
		"Kvist/Note.md":  note,
		"Kvist/_site.md": "---\nstrict_line_breaks: true\n---\n",
	})
	if c := string(site.Notes[0].Content); strings.Contains(c, "<br>") {
		t.Errorf("strict line breaks: single breaks should stay spaces: %s", c)
	}
	if _, ok := site.Config.Params["strict_line_breaks"]; ok {
		t.Error("strict_line_breaks is a site setting, not a theme parameter")
	}
}

func TestImagesAndProfile(t *testing.T) {
	site := buildVault(t, map[string]string{
		"Kvist/Note.md":      "---\nimage: \"[[cover.png]]\"\n---\nText without the picture.",
		"Kvist/Web.md":       "---\ncover: https://example.com/c.png\n---\nText.",
		"Kvist/Broken.md":    "---\nimage: \"[[missing.png]]\"\ncover: \"[[cover.png]]\"\n---\nText.",
		"Kvist/cover.png":    "png",
		"Kvist/me.jpg":       "jpg",
		"Private/hidden.png": "png",
		"Kvist/_site.md": `---
image: "[[hidden.png]]"
avatar: "[[me.jpg]]"
bio: Gardener.
profile_links:
  - https://github.com/ada
  - "[Toots](https://mastodon.social/@ada)"
  - Blog: https://ada.example.com
  - "Docs: https://docs.example.com"
  - mailto:ada@example.com
  - javascript:alert(1)
---
`,
	})
	byPath := map[string]*Note{}
	for _, n := range site.Notes {
		byPath[n.Path] = n
	}
	var cover string
	for _, a := range site.Assets {
		if a.Path == "Kvist/cover.png" {
			cover = a.URL
		}
		if a.Path == "Private/hidden.png" {
			t.Error("an image in an excluded folder was published")
		}
	}
	if cover == "" || byPath["Kvist/Note.md"].Image != cover {
		t.Errorf("note image = %q, cover asset = %q", byPath["Kvist/Note.md"].Image, cover)
	}
	if got := byPath["Kvist/Web.md"].Image; got != "https://example.com/c.png" {
		t.Errorf("web image = %q", got)
	}
	if got := byPath["Kvist/Broken.md"].Image; got != "" {
		t.Errorf("an unresolved image should be left out, not fall back: %q", got)
	}
	if site.Config.Image != "" {
		t.Errorf("site image from an excluded folder: %q", site.Config.Image)
	}
	p := site.Config.Profile
	if p == nil || !strings.HasPrefix(p.Avatar, "/_assets/") || p.Bio != "Gardener." {
		t.Fatalf("profile = %+v", p)
	}
	want := []ProfileLink{
		{"GitHub", "https://github.com/ada", "github"},
		{"Toots", "https://mastodon.social/@ada", "mastodon"},
		{"Blog", "https://ada.example.com", "website"},
		{"Docs", "https://docs.example.com", "website"},
		{"Email", "mailto:ada@example.com", "email"},
	}
	if !reflect.DeepEqual(p.Links, want) {
		t.Errorf("links = %+v", p.Links)
	}
	for _, k := range []string{"image", "avatar", "bio", "profile_links"} {
		if _, ok := site.Config.Params[k]; ok {
			t.Errorf("%s should not be a theme parameter", k)
		}
	}
	var warned bool
	for _, w := range site.Warnings {
		warned = warned || strings.Contains(w.Message, "javascript:")
	}
	if !warned {
		t.Error("no warning for the javascript: profile link")
	}
}
