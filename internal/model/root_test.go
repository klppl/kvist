package model

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klppl/kvist/internal/config"
	"github.com/klppl/kvist/internal/source/dir"
)

func buildWithConfig(t *testing.T, cfgText string, files map[string]string) (*Site, error) {
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
	cfg, err := config.Parse([]byte(cfgText))
	if err != nil {
		t.Fatal(err)
	}
	snap, err := dir.New(d).Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return Build(cfg.Sites[0], snap)
}

func rootConfig(rootFolder string) string {
	c := "data_dir = \"unused\"\n[[site]]\nid = \"g\"\nbase_url = \"https://g.example.com\"\n"
	if rootFolder != "" {
		c += "root_folder = \"" + rootFolder + "\"\n"
	}
	return c + "  [site.publish]\n  always_public_folders = [\"Kvist\"]\n"
}

var rootVault = map[string]string{
	"Kvist/About.md":         "About.",
	"Kvist/Garden/Garden.md": "The garden.",
	"Kvist/Garden/Soil.md":   "Soil.",
	"Kvist/Garden/Beds/A.md": "Bed A.",
	"Reading list.md":        "Books. #public",
	"Elsewhere/Published.md": "---\npublish: true\n---\nOutside the root.",
}

func TestRootFolder(t *testing.T) {
	site, err := buildWithConfig(t, rootConfig(""), rootVault) // automatic: the only always-public folder
	if err != nil {
		t.Fatal(err)
	}
	for p, want := range map[string]string{
		"Kvist/About.md":         "/about/",
		"Kvist/Garden/Garden.md": "/garden/",
		"Kvist/Garden/Soil.md":   "/garden/soil/",
		"Kvist/Garden/Beds/A.md": "/garden/beds/a/",
		"Reading list.md":        "/reading-list/",
		"Elsewhere/Published.md": "/elsewhere/published/",
	} {
		if n := site.Note(p); n == nil || n.URL != want {
			t.Errorf("%s: %v, want %s", p, n, want)
		}
	}
	var names []string
	for _, f := range site.Root.Children {
		names = append(names, f.Name+"="+f.URL+"@"+f.Path)
	}
	if got := strings.Join(names, " "); got != "Elsewhere=/elsewhere/@Elsewhere Garden=/garden/@Kvist/Garden" {
		t.Errorf("top folders: %s", got)
	}
	if site.Root.Path != "Kvist" || len(site.Root.Notes) != 2 {
		t.Errorf("root folder: path %q, notes %v", site.Root.Path, site.Root.NoteIDs)
	}
	soil := site.Note("Kvist/Garden/Soil.md")
	if soil.Folder.URL != "/garden/" || soil.Folder.Parent != site.Root || soil.FolderPath != "Kvist/Garden" {
		t.Errorf("soil's folder: %s (%s)", soil.Folder.URL, soil.FolderPath)
	}

	// Off with "/", and an explicit folder.
	site, err = buildWithConfig(t, rootConfig("/"), rootVault)
	if err != nil {
		t.Fatal(err)
	}
	if u := site.Note("Kvist/Garden/Soil.md").URL; u != "/kvist/garden/soil/" {
		t.Errorf(`root_folder = "/": %s`, u)
	}
	site, err = buildWithConfig(t, rootConfig("Kvist/Garden/"), rootVault)
	if err != nil {
		t.Fatal(err)
	}
	if u := site.Note("Kvist/Garden/Soil.md").URL; u != "/soil/" {
		t.Errorf("root_folder = Kvist/Garden: %s", u)
	}
	if n := site.Note("Kvist/Garden/Garden.md"); n.URL != "/" || site.Home != n {
		t.Errorf("the root folder's own note is the home page: %s", n.URL)
	}
}

func TestRootFolderReservedAddress(t *testing.T) {
	_, err := buildWithConfig(t, rootConfig(""), map[string]string{"Kvist/Tags.md": "Tags."})
	if err == nil || !strings.Contains(err.Error(), "/tags/, which is reserved") {
		t.Errorf("a note named Tags in the root folder: %v", err)
	}
}
