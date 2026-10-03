// Package resolve resolves Obsidian links against a set of vault paths.
//
// Obsidian resolves a link by, in order: an exact vault path, a path relative
// to the linking note, a path suffix, and finally a bare file name. Matching
// ignores case. When several files match a name, the one in the linking
// note's folder wins, then the shortest path, then the lexically first.
package resolve

import (
	"path"
	"sort"
	"strings"

	"github.com/klppl/kvist/internal/vault"
)

// Index is a lookup structure over a set of vault paths.
type Index struct {
	byPath map[string]string   // lower(path) → path
	byName map[string][]string // lower(file name) → paths; notes also under their stem
	paths  []string
}

// NewIndex indexes paths.
func NewIndex(paths []string) *Index {
	ix := &Index{byPath: map[string]string{}, byName: map[string][]string{}}
	ix.paths = append(ix.paths, paths...)
	sort.Strings(ix.paths)
	for _, p := range ix.paths {
		lp := strings.ToLower(p)
		ix.byPath[lp] = p
		name := path.Base(lp)
		ix.byName[name] = append(ix.byName[name], p)
		if stem, ok := strings.CutSuffix(name, ".md"); ok {
			ix.byName[stem] = append(ix.byName[stem], p)
		}
	}
	return ix
}

// Has reports whether p is in the index (exact).
func (ix *Index) Has(p string) bool {
	q, ok := ix.byPath[strings.ToLower(p)]
	return ok && q == p
}

// Resolve returns the vault path a link from note `from` points to.
func (ix *Index) Resolve(from string, l vault.Link) (string, bool) {
	t := strings.TrimSpace(l.Target)
	if t == "" {
		return from, true
	}
	t = strings.TrimPrefix(t, "/")
	dir := path.Dir(from)

	var candidates []string
	if l.Markdown || strings.HasPrefix(t, "./") || strings.HasPrefix(t, "../") {
		candidates = append(candidates, path.Join(dir, t))
	}
	candidates = append(candidates, path.Clean(t))
	for _, c := range candidates {
		if strings.HasPrefix(c, "../") || c == ".." {
			continue
		}
		if p, ok := ix.lookupPath(c); ok {
			return p, true
		}
	}

	// Suffix match for "Folder/Note", then bare name.
	lt := strings.ToLower(path.Clean(t))
	name := path.Base(lt)
	var matches []string
	for _, p := range ix.byName[name] {
		lp := strings.ToLower(p)
		if !strings.Contains(lt, "/") || strings.HasSuffix(lp, "/"+lt) || strings.HasSuffix(lp, "/"+lt+".md") ||
			lp == lt || lp == lt+".md" {
			matches = append(matches, p)
		}
	}
	if len(matches) == 0 {
		return "", false
	}
	return best(matches, dir), true
}

func (ix *Index) lookupPath(c string) (string, bool) {
	lc := strings.ToLower(c)
	if p, ok := ix.byPath[lc]; ok {
		return p, true
	}
	if path.Ext(lc) == "" || path.Ext(lc) != ".md" {
		if p, ok := ix.byPath[lc+".md"]; ok {
			return p, true
		}
	}
	return "", false
}

// best picks among several matches: same folder, then shortest, then first.
func best(matches []string, dir string) string {
	uniq := map[string]bool{}
	var ms []string
	for _, m := range matches {
		if !uniq[m] {
			uniq[m] = true
			ms = append(ms, m)
		}
	}
	sort.Slice(ms, func(i, j int) bool {
		si, sj := path.Dir(ms[i]) == dir, path.Dir(ms[j]) == dir
		if si != sj {
			return si
		}
		if len(ms[i]) != len(ms[j]) {
			return len(ms[i]) < len(ms[j])
		}
		return ms[i] < ms[j]
	})
	return ms[0]
}
