package build

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/klppl/kvist/internal/config"
	"github.com/klppl/kvist/internal/imagemeta"
	"github.com/klppl/kvist/internal/model"
	"github.com/klppl/kvist/internal/protocol"
	"github.com/klppl/kvist/internal/render"
	"github.com/klppl/kvist/internal/source"
	pushsource "github.com/klppl/kvist/internal/source/push"
	"github.com/klppl/kvist/internal/store"
	"github.com/klppl/kvist/themes"
)

// LoadTheme finds a site's theme: a path (if the name contains a slash), a
// folder in themesDir, or a built-in theme. theme_overrides, if set,
// replaces individual files.
func LoadTheme(themesDir string, site *config.Site) (*render.Theme, error) {
	var fsys fs.FS
	name := site.Theme
	switch {
	case strings.ContainsAny(name, `/\`):
		fsys = os.DirFS(name)
	default:
		if themesDir != "" {
			if fi, err := os.Stat(filepath.Join(themesDir, name, "theme.toml")); err == nil && !fi.IsDir() {
				fsys = os.DirFS(filepath.Join(themesDir, name))
			}
		}
		if fsys == nil {
			fsys = themes.Builtin(name)
		}
	}
	if fsys == nil {
		return nil, fmt.Errorf("theme %q not found (looked in %s and the built-in themes)", name, themesDir)
	}
	var overrides fs.FS
	if site.ThemeDir != "" {
		overrides = os.DirFS(site.ThemeDir)
	}
	return render.LoadTheme(fsys, overrides)
}

// dirOutput writes files below a directory.
type dirOutput struct{ root string }

func (o dirOutput) WriteFile(p string, data []byte) error {
	dst := filepath.Join(o.root, filepath.FromSlash(p))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

func (o dirOutput) copy(p string, r io.Reader) error {
	dst := filepath.Join(o.root, filepath.FromSlash(p))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// WriteSite builds the site from a snapshot into dir, which must be empty
// or absent. It returns the build's warnings.
func WriteSite(ctx context.Context, site *config.Site, theme *render.Theme, snap source.Snapshot, dir string) ([]protocol.Warning, error) {
	m, err := model.Build(site, snap)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	out := dirOutput{root: dir}
	if err := render.Render(m, theme, out); err != nil {
		return nil, err
	}
	files := map[string]source.File{}
	for _, f := range snap.Files() {
		files[f.Path] = f
	}
	warnings := m.Warnings
	strip := site.Publish.StripImageMetadata != nil && *site.Publish.StripImageMetadata
	for _, a := range m.Assets {
		f, ok := files[a.Path]
		if !ok {
			return nil, fmt.Errorf("asset %s is missing from the snapshot", a.Path)
		}
		dst := strings.TrimPrefix(a.URL, "/")
		r, err := snap.Open(f)
		if err != nil {
			return nil, err
		}
		if strip && imagemeta.Supported(path.Ext(a.Path)) {
			data, err := io.ReadAll(r)
			r.Close()
			if err != nil {
				return nil, err
			}
			clean, err := imagemeta.Strip(path.Ext(a.Path), data)
			if err != nil {
				// Metadata that can't be removed must not be published.
				warnings = append(warnings, protocol.Warning{Code: protocol.WarnBuild, Path: a.Path,
					Message: "image could not be read to remove its metadata (location, camera); not published"})
				continue
			}
			if err := out.WriteFile(dst, clean); err != nil {
				return nil, err
			}
			continue
		}
		err = out.copy(dst, r)
		r.Close()
		if err != nil {
			return nil, err
		}
	}
	return warnings, nil
}

// SiteBuilder builds sites from the content store. It implements Builder
// for the build queue.
type SiteBuilder struct {
	Config *config.Config
	Store  *store.Store
	Log    *slog.Logger
}

// Build builds one revision and publishes it atomically: the output goes to
// builds/<id>/ and the public symlink is swapped to it with rename(2), so
// readers see the old site or the new one, never a mix.
func (b *SiteBuilder) Build(ctx context.Context, siteID, revision, buildID string) ([]protocol.Warning, error) {
	sc := b.Config.Site(siteID)
	if sc == nil {
		return nil, fmt.Errorf("unknown site %q", siteID)
	}
	ss, err := b.Store.Site(siteID)
	if err != nil {
		return nil, err
	}
	snap, err := pushsource.New(ss).SnapshotAt(revision)
	if err != nil {
		return nil, err
	}
	theme, err := LoadTheme(b.Config.ThemesDir, sc)
	if err != nil {
		return nil, err
	}
	builds := filepath.Join(ss.Dir(), "builds")
	final := filepath.Join(builds, buildID)
	tmp := final + ".tmp"
	_ = os.RemoveAll(tmp)
	warnings, err := WriteSite(ctx, sc, theme, snap, tmp)
	if err != nil {
		_ = os.RemoveAll(tmp)
		return warnings, err
	}
	if err := os.Rename(tmp, final); err != nil {
		_ = os.RemoveAll(tmp)
		return warnings, err
	}
	if err := swapSymlink(filepath.Join(ss.Dir(), "public"), filepath.Join("builds", buildID)); err != nil {
		return warnings, err
	}
	if err := pruneBuilds(builds, buildID, sc.Retention.Builds); err != nil && b.Log != nil {
		b.Log.Warn("prune builds", "site", siteID, "err", err)
	}
	return warnings, nil
}

// swapSymlink atomically points link at target (relative to link's folder).
func swapSymlink(link, target string) error {
	tmp := link + ".new"
	_ = os.Remove(tmp)
	if err := os.Symlink(target, tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, link); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// pruneBuilds keeps the newest keep builds (ids sort by time) and always
// the current one. Leftover .tmp folders of crashed builds are removed.
func pruneBuilds(dir, current string, keep int) error {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var ids []string
	var errs []error
	for _, e := range ents {
		switch {
		case strings.HasSuffix(e.Name(), ".tmp"):
			errs = append(errs, os.RemoveAll(filepath.Join(dir, e.Name())))
		case e.IsDir():
			ids = append(ids, e.Name())
		}
	}
	sort.Strings(ids)
	for i := 0; i < len(ids)-keep; i++ {
		if ids[i] != current {
			errs = append(errs, os.RemoveAll(filepath.Join(dir, ids[i])))
		}
	}
	return errors.Join(errs...)
}
