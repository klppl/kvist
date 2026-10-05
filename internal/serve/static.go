// Package serve serves built sites. Only each site's public/ directory is
// ever served (§5.3 "Old builds"); it is a symlink swapped atomically on
// every build.
package serve

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
)

// Site is one served site.
type Site struct {
	Host      string // host name from base_url, without port
	PublicDir string // <data>/sites/<id>/public
}

// Static serves sites by Host header.
type Static struct {
	sites    map[string]string // host → public dir
	fallback string            // public dir when only one site is served

	mu        sync.Mutex
	redirects map[string]redirectMap // public dir → its current build's redirects
}

// redirectMap is a build's redirects (old URL → new URL), read once per
// build.
type redirectMap struct {
	build string // the public dir's resolved target
	to    map[string]string
}

// redirectsFile is build.RedirectsFile; serve doesn't import build.
const redirectsFile = "_kvist/redirects.json"

// New returns a handler for sites. With a single site, requests for an
// unknown host get that site too (handy for http://localhost:8080).
func New(sites []Site) *Static {
	s := &Static{sites: map[string]string{}, redirects: map[string]redirectMap{}}
	for _, site := range sites {
		s.sites[strings.ToLower(site.Host)] = site.PublicDir
	}
	if len(sites) == 1 {
		s.fallback = sites[0].PublicDir
	}
	return s
}

// HostOf returns the host name of a base URL.
func HostOf(baseURL string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// Has reports whether a request's host maps to a served site.
func (s *Static) Has(r *http.Request) bool { return s.dir(r) != "" }

func (s *Static) dir(r *http.Request) string {
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if d, ok := s.sites[strings.ToLower(host)]; ok {
		return d
	}
	return s.fallback
}

// Cache lifetimes. HTML stays short so an unpublished page disappears from
// CDN caches quickly; hashed assets never change.
const (
	cacheShort     = "public, max-age=60"
	cacheImmutable = "public, max-age=31536000, immutable"
)

func (s *Static) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	dir := s.dir(r)
	if dir == "" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "strict-origin-when-cross-origin")

	clean := path.Clean("/" + r.URL.Path)
	// A moved note's old address: a permanent redirect to the new one.
	if to, ok := s.redirect(dir, clean+"/"); ok {
		if r.URL.RawQuery != "" {
			to += "?" + r.URL.RawQuery
		}
		h.Set("Cache-Control", cacheShort)
		http.Redirect(w, r, to, http.StatusMovedPermanently)
		return
	}
	rel := strings.TrimPrefix(clean, "/")
	if strings.HasSuffix(r.URL.Path, "/") || rel == "" {
		rel = path.Join(rel, "index.html")
	}
	f, fi, err := open(dir, rel)
	if err == nil && fi.IsDir() {
		f.Close()
		// /notes → /notes/ if that folder has a page.
		if _, _, err := open(dir, path.Join(rel, "index.html")); err == nil {
			target := clean + "/"
			if r.URL.RawQuery != "" {
				target += "?" + r.URL.RawQuery
			}
			http.Redirect(w, r, target, http.StatusMovedPermanently)
			return
		}
		err = fs.ErrNotExist
	}
	if err != nil {
		s.notFound(w, r, dir)
		return
	}
	defer f.Close()
	if strings.HasPrefix(clean, "/_assets/") || strings.HasPrefix(clean, "/_kvist/") {
		h.Set("Cache-Control", cacheImmutable)
	} else {
		h.Set("Cache-Control", cacheShort)
	}
	if ct := mime.TypeByExtension(path.Ext(rel)); ct != "" {
		h.Set("Content-Type", ct)
	}
	http.ServeContent(w, r, "", fi.ModTime(), f)
}

// redirect looks up a page URL in the redirects of dir's current build.
func (s *Static) redirect(dir, u string) (string, bool) {
	build, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", false
	}
	s.mu.Lock()
	rm, ok := s.redirects[dir]
	s.mu.Unlock()
	if !ok || rm.build != build {
		rm = redirectMap{build: build}
		if b, err := os.ReadFile(filepath.Join(build, filepath.FromSlash(redirectsFile))); err == nil {
			_ = json.Unmarshal(b, &rm.to)
		}
		s.mu.Lock()
		s.redirects[dir] = rm
		s.mu.Unlock()
	}
	to, ok := rm.to[u]
	return to, ok
}

func (s *Static) notFound(w http.ResponseWriter, r *http.Request, dir string) {
	w.Header().Set("Cache-Control", cacheShort)
	f, _, err := open(dir, "404.html")
	if err != nil {
		http.Error(w, "404 not found", http.StatusNotFound)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	if r.Method != http.MethodHead {
		_, _ = io.Copy(w, f)
	}
}

// open opens rel below dir. rel is already cleaned and relative, so it
// cannot leave dir; public/ itself is a symlink and is resolved per request.
func open(dir, rel string) (*os.File, os.FileInfo, error) {
	if strings.Contains(rel, "\x00") || strings.HasPrefix(rel, "..") {
		return nil, nil, fs.ErrNotExist
	}
	f, err := os.Open(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		return nil, nil, err
	}
	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	if !fi.Mode().IsRegular() && !fi.IsDir() {
		f.Close()
		return nil, nil, errors.New("not a regular file")
	}
	return f, fi, nil
}
