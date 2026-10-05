package build

import (
	"encoding/json"
	"errors"
	"html/template"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/klppl/kvist/internal/model"
	"github.com/klppl/kvist/internal/slug"
)

// Redirects keep old links working when a note moves. After every build
// the server compares the new note pages with the previous build's: a page
// whose address is gone redirects to its note's new address, if the note
// can be found again. The history lives next to the site's builds, so it
// survives restarts and grows over every publish.
//
// A note is found again, in order, by
//  1. the same vault path (its permalink or a parent folder's name changed),
//  2. the same content at a new path (moved or renamed, not edited), or
//  3. the only new page whose file has the same name (moved and edited).
//
// Only published notes are ever targets, and an address that has a page
// again stops redirecting, so redirects never hide a page or reveal a
// private note.

// RedirectsFile is the redirect map in a build's output, read from disk
// by the built-in server, which doesn't serve it. Pages at the old addresses also redirect, for servers
// that only serve files.
const RedirectsFile = "_kvist/redirects.json"

// historyFile is the redirect history in a site's folder.
const historyFile = "redirects.json"

// maxRedirects bounds the history; the oldest-sorting entries go first.
const maxRedirects = 20000

// History is a site's redirect history.
type History struct {
	Version   int               `json:"version"`
	Pages     map[string]Page   `json:"pages"`     // vault path → its page in the last build
	Redirects map[string]string `json:"redirects"` // old URL → current URL
}

// Page is a note's page in a build.
type Page struct {
	URL  string `json:"url"`
	Hash string `json:"hash"` // of the note's source file
}

// LoadHistory reads a site's redirect history; a missing file is an empty
// history.
func LoadHistory(siteDir string) (*History, error) {
	h := &History{Version: 1, Pages: map[string]Page{}, Redirects: map[string]string{}}
	b, err := os.ReadFile(filepath.Join(siteDir, historyFile))
	if errors.Is(err, fs.ErrNotExist) {
		return h, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, h); err != nil {
		return nil, err
	}
	if h.Pages == nil {
		h.Pages = map[string]Page{}
	}
	if h.Redirects == nil {
		h.Redirects = map[string]string{}
	}
	return h, nil
}

// Save writes the history atomically.
func (h *History) Save(siteDir string) error {
	b, err := json.Marshal(h)
	if err != nil {
		return err
	}
	tmp := filepath.Join(siteDir, historyFile+".tmp")
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(siteDir, historyFile))
}

// next returns the history after a build with the given note pages. live
// reports whether a URL has a page in the new build.
func (h *History) next(pages map[string]Page, live func(url string) bool) *History {
	n := &History{Version: 1, Pages: pages, Redirects: map[string]string{}}
	byURL := map[string]bool{}
	for _, p := range pages {
		byURL[p.URL] = true
	}
	// Pages that are new in this build, by content and by file name.
	byHash, byName := map[string][]string{}, map[string][]string{}
	for p, pg := range pages {
		if _, old := h.Pages[p]; old {
			continue
		}
		byHash[pg.Hash] = append(byHash[pg.Hash], pg.URL)
		name := strings.ToLower(path.Base(p))
		byName[name] = append(byName[name], pg.URL)
	}
	add := func(from, to string) {
		if from != to && !live(from) && validURL(from) {
			n.Redirects[from] = to
		}
	}
	for p, old := range h.Pages {
		if byURL[old.URL] {
			continue // the address still has a note
		}
		if pg, ok := pages[p]; ok {
			add(old.URL, pg.URL)
		} else if us := byHash[old.Hash]; len(us) == 1 {
			add(old.URL, us[0])
		} else if us := byName[strings.ToLower(path.Base(p))]; len(us) == 1 {
			add(old.URL, us[0])
		}
	}
	// Older redirects follow their target if it moved too, and are dropped
	// when it is gone or their own address has a page again.
	for from, to := range h.Redirects {
		if _, set := n.Redirects[from]; set {
			continue
		}
		for i := 0; i < 10 && !byURL[to]; i++ {
			nt, ok := n.Redirects[to]
			if !ok {
				break
			}
			to = nt
		}
		if byURL[to] {
			add(from, to)
		}
	}
	if len(n.Redirects) > maxRedirects {
		keys := make([]string, 0, len(n.Redirects))
		for k := range n.Redirects {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys[:len(keys)-maxRedirects] {
			delete(n.Redirects, k)
		}
	}
	return n
}

// validURL accepts the addresses of note pages: an absolute path ending in
// a slash, without dot segments.
// movedFolders redirects the old addresses of folder pages to their new
// ones after the site's root folder changed: with root_folder "Kvist",
// /kvist/garden/ is now /garden/ and /kvist/ is the home page. Notes are
// followed by their path already; this covers the folders. Only addresses
// the previous build had (existed) are redirected, and never one that has
// a page now (live).
func (h *History) movedFolders(root *model.Folder, existed, live func(url string) bool) {
	var walk func(f *model.Folder)
	walk = func(f *model.Folder) {
		if f.Path != "" {
			old := "/" + slug.Path(f.Path) + "/"
			if _, set := h.Redirects[old]; !set && old != f.URL && validURL(old) && existed(old) && !live(old) {
				h.Redirects[old] = f.URL
			}
		}
		for _, c := range f.Children {
			walk(c)
		}
	}
	walk(root)
}

func validURL(u string) bool {
	return strings.HasPrefix(u, "/") && strings.HasSuffix(u, "/") && u != "/" &&
		path.Clean(u)+"/" == u && !strings.ContainsAny(u, "\x00?#\\")
}

var redirectPage = template.Must(template.New("redirect").Parse(`<!doctype html>
<html><head><meta charset="utf-8"><title>Moved</title>
<meta name="robots" content="noindex">
<link rel="canonical" href="{{.Abs}}">
<meta http-equiv="refresh" content="0; url={{.To}}">
</head><body><p>This page has moved to <a href="{{.To}}">{{.Abs}}</a>.</p></body></html>
`))

// writeRedirects writes the redirect map and a page at each old address.
func writeRedirects(out *dirOutput, redirects map[string]string, baseURL string) error {
	if len(redirects) == 0 {
		return nil
	}
	b, err := json.Marshal(redirects)
	if err != nil {
		return err
	}
	if err := out.WriteFile(RedirectsFile, b); err != nil {
		return err
	}
	froms := make([]string, 0, len(redirects))
	for f := range redirects {
		froms = append(froms, f)
	}
	sort.Strings(froms)
	for _, from := range froms {
		to := redirects[from]
		var sb strings.Builder
		if err := redirectPage.Execute(&sb, map[string]string{"To": to, "Abs": strings.TrimSuffix(baseURL, "/") + to}); err != nil {
			return err
		}
		if err := out.WriteFile(strings.TrimPrefix(from, "/")+"index.html", []byte(sb.String())); err != nil {
			return err
		}
	}
	return nil
}
