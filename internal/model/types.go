// Package model builds content model v1 (§6): the contract between the
// build pipeline and themes. Everything in a Site is derived from published
// notes only, after the publish filter, so a theme cannot leak what it never
// receives.
//
// Pointer fields that would make cycles (note ↔ folder, note ↔ tag,
// backlinks) are hidden from JSON and mirrored by id fields.
package model

import (
	"html/template"
	"time"

	"github.com/klppl/kvist/internal/markdown"
	"github.com/klppl/kvist/internal/protocol"
)

// Version is the content model version. Breaking changes bump it; themes
// declare the versions they support.
const Version = 1

// Site is the whole published site.
type Site struct {
	ModelVersion int        `json:"model_version"`
	Config       SiteConfig `json:"config"`
	Revision     string     `json:"revision"`
	BuiltAt      time.Time  `json:"built_at"` // commit time of the revision: deterministic
	Notes        []*Note    `json:"notes"`    // sorted by URL
	Assets       []*Asset   `json:"assets"`   // sorted by URL
	Tags         []*Tag     `json:"tags"`     // top-level tags; children nest
	AllTags      []*Tag     `json:"-"`        // every tag, sorted by name
	Root         *Folder    `json:"root"`
	Graph        Graph      `json:"graph"`
	Home         *Note      `json:"-"`
	HomeID       string     `json:"home,omitempty"`

	Warnings []protocol.Warning `json:"-"`
	notesBy  map[string]*Note
}

// SiteConfig is the presentation config visible to themes.
type SiteConfig struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	BaseURL     string `json:"base_url"`
	Language    string `json:"language"`
	Author      string `json:"author,omitempty"`
	// StrictLineBreaks: single line breaks in notes are spaces, not <br>.
	StrictLineBreaks bool      `json:"strict_line_breaks,omitempty"`
	Nav              []NavItem `json:"nav,omitempty"`
	// PrimaryNav are the settings note's primary_links, for the top of the menu.
	PrimaryNav []NavItem      `json:"primary_nav,omitempty"`
	Params     map[string]any `json:"params,omitempty"`
	// Image is the default social preview image: the URL of a published
	// attachment, or an http(s) URL.
	Image   string   `json:"image,omitempty"`
	Favicon string   `json:"favicon,omitempty"` // like Image; the theme's icon when empty
	Profile *Profile `json:"profile,omitempty"` // nil when not set
	// Analytics is the analytics service to load, nil for none.
	Analytics *Analytics `json:"analytics,omitempty"`
}

// Profile is the site owner's card: a picture, a short bio and links.
type Profile struct {
	Avatar string        `json:"avatar,omitempty"` // like SiteConfig.Image
	Bio    string        `json:"bio,omitempty"`
	Links  []ProfileLink `json:"links,omitempty"`
}

// ProfileLink is a link on the profile. Kind names the service, for an
// icon: github, gitlab, mastodon, bluesky, linkedin, x, youtube, email or
// website.
type ProfileLink struct {
	Title string `json:"title"`
	URL   string `json:"url"`
	Kind  string `json:"kind"`
}

// NavItem is one navigation link.
type NavItem struct {
	Title string `json:"title" toml:"title"`
	URL   string `json:"url" toml:"url"`
}

// Note is a published note.
type Note struct {
	ID       string    `json:"id"`
	Path     string    `json:"path"`
	URL      string    `json:"url"`
	Slug     string    `json:"slug"`
	Title    string    `json:"title"`
	Aliases  []string  `json:"aliases,omitempty"`
	Tags     []*Tag    `json:"-"`
	TagNames []string  `json:"tags"`
	Created  time.Time `json:"created"`
	Updated  time.Time `json:"updated"`
	// Order: frontmatter order (or weight). Notes with one come first in
	// their folder, lowest first; nil for the rest.
	Order       *float64 `json:"order,omitempty"`
	Description string   `json:"description,omitempty"`
	// Lead is Description when the frontmatter sets it; empty when
	// Description was taken from the first paragraph. A layout can show it
	// under the title without repeating the note's first lines.
	Lead string `json:"lead,omitempty"`
	// Layout is how the page is laid out: one of Layouts, from the
	// frontmatter layout property; "" (an article) when not set.
	Layout string         `json:"layout,omitempty"`
	Params map[string]any `json:"params,omitempty"`
	Image  string         `json:"image,omitempty"` // social preview image, like SiteConfig.Image
	// ImageThumb (a square) and ImageCard (16:9) are small copies of Image
	// for note lists and cards, made by the build (internal/build). They are
	// Image itself when it can't be made smaller (an SVG, a web address),
	// and in a model that wasn't built into a site (kvist build
	// --emit-model).
	ImageThumb string `json:"image_thumb,omitempty"`
	ImageCard  string `json:"image_card,omitempty"`
	// Unlisted: published, but left out of folders, tags, the graph,
	// backlinks, search, the feed and the sitemap (frontmatter unlisted).
	Unlisted   bool   `json:"unlisted,omitempty"`
	folderSort string // frontmatter sort, read when this is a folder note
	folderIcon string // frontmatter icon, read when this is a folder note
	// Canvas: the page is an Obsidian canvas (a .canvas file). Content is
	// then a board of cards and arrows, and there are no properties,
	// aliases or TOC.
	Canvas    bool          `json:"canvas,omitempty"`
	Content   template.HTML `json:"content"`
	TOC       []*Heading    `json:"toc,omitempty"`
	Links     []*Link       `json:"links,omitempty"`
	Backlinks []*Backlink   `json:"backlinks,omitempty"`
	// Related: up to five listed notes with words, tags or linked notes in
	// common, best first, without the ones this note links to or is linked
	// from. Empty for unlisted notes.
	Related     []*Note           `json:"-"`
	RelatedIDs  []string          `json:"related,omitempty"`
	Folder      *Folder           `json:"-"`
	FolderPath  string            `json:"folder"`
	Features    markdown.Features `json:"features"`
	WordCount   int               `json:"word_count"`
	ReadingTime int               `json:"reading_time"` // minutes
	Text        string            `json:"-"`            // plain text, for search
}

// Heading is a TOC entry.
type Heading struct {
	Level    int        `json:"level"`
	Text     string     `json:"text"`
	ID       string     `json:"id"`
	Children []*Heading `json:"children,omitempty"`
}

// Link is a resolved outgoing link to another published note.
type Link struct {
	Target   *Note  `json:"-"`
	TargetID string `json:"target"`
	Embed    bool   `json:"embed,omitempty"`
}

// Backlink is an incoming link from another published note.
type Backlink struct {
	Source   *Note  `json:"-"`
	SourceID string `json:"source"`
	Context  string `json:"context,omitempty"`
}

// Asset is a published attachment.
type Asset struct {
	Path      string `json:"path"`
	URL       string `json:"url"`
	Size      int64  `json:"size"`
	MediaType string `json:"media_type"` // MIME type
	Hash      string `json:"hash"`
	// Width and Height: an image's displayed size in pixels, when known.
	Width  int `json:"width,omitempty"`
	Height int `json:"height,omitempty"`
}

// Tag is a tag with the notes that carry it. Nested tags (#a/b) are
// children of their parent (#a); a note tagged #a/b also counts for #a.
type Tag struct {
	Name     string   `json:"name"` // full name, e.g. "a/b"
	Slug     string   `json:"slug"`
	URL      string   `json:"url"`
	Notes    []*Note  `json:"-"` // listed notes tagged exactly this tag or a child, sorted by URL
	NoteIDs  []string `json:"notes"`
	Children []*Tag   `json:"children,omitempty"`
	Parent   *Tag     `json:"-"`
}

// Folder is a folder containing published notes.
type Folder struct {
	Name    string   `json:"name"`
	Path    string   `json:"path"`
	URL     string   `json:"url"`
	Notes   []*Note  `json:"-"` // listed notes only, without the folder note or the home note, in Sort order
	NoteIDs []string `json:"notes"`
	// Sort is how Notes are ordered after the notes with an Order:
	// "updated" (newest first, the default), "created" (newest first) or
	// "title" (A–Z), from the folder note's sort property.
	Sort string `json:"sort"`
	// Icon and IconText come from the folder note's icon property; at
	// most one is set. Icon is the name of an icon for the theme to draw
	// (lowercase letters, digits and hyphens, e.g. "book"); IconText is
	// anything else, such as an emoji, shown as it is.
	Icon     string    `json:"icon,omitempty"`
	IconText string    `json:"icon_text,omitempty"`
	Children []*Folder `json:"children,omitempty"`
	Index    *Note     `json:"-"` // folder note, if any
	IndexID  string    `json:"index,omitempty"`
	Parent   *Folder   `json:"-"`
	// Unlisted: the folder holds only unlisted notes. It is not among its
	// parent's Children and has no page; its notes' Folder still points
	// to it.
	Unlisted bool `json:"unlisted,omitempty"`
}

// Graph holds listed notes and the links between them. There are no
// nodes for unresolved links or unlisted notes.
type Graph struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
	Tags  []GraphTag  `json:"tags"` // every tag, sorted by name, for tag nodes
}

// GraphTag is a tag with its page, so themes can draw tags as nodes.
type GraphTag struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// GraphNode is a listed note in the graph.
type GraphNode struct {
	ID    string   `json:"id"`
	Title string   `json:"title"`
	URL   string   `json:"url"`
	Tags  []string `json:"tags,omitempty"`
}

// GraphEdge is a link between two notes.
type GraphEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

// Note returns the note with the given vault path, or nil.
func (s *Site) Note(path string) *Note { return s.notesBy[path] }
