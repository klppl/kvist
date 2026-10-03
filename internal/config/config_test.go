package config

import (
	"strings"
	"testing"
)

func TestDefaults(t *testing.T) {
	c, err := Parse([]byte(`
data_dir = "/tmp/x"
[[site]]
id = "garden"
base_url = "https://g.example.com"
  [site.publish]
  always_public_folders = ["Garden/", "/"]
  exclude_folders = ["/Private"]
  public_tag = "#pub"
  attachment_extensions = [".PNG"]
`))
	if err != nil {
		t.Fatal(err)
	}
	s := c.Site("garden")
	r := s.Rules()
	if r.PublicTag != "pub" || r.PrivateTag != "private" || r.FrontmatterKey != "publish" {
		t.Errorf("rules = %+v", r)
	}
	if strings.Join(r.AlwaysPublicFolders, ",") != "Garden,/" || r.ExcludeFolders[0] != "Private" {
		t.Errorf("folders = %v %v", r.AlwaysPublicFolders, r.ExcludeFolders)
	}
	if r.AttachmentExtensions[0] != "png" {
		t.Errorf("extensions = %v", r.AttachmentExtensions)
	}
	if s.Retention.Revisions != DefaultRevisions || s.Limits.MaxFiles != DefaultMaxFiles || !*s.Publish.StripImageMetadata {
		t.Errorf("site defaults not applied: %+v", s)
	}
	if c.SyncTTL.Duration != DefaultSyncTTL || c.Listen != DefaultListen {
		t.Errorf("server defaults not applied: %+v", c)
	}
}

func TestValidationErrors(t *testing.T) {
	cases := map[string]string{
		"missing data_dir": `[[site]]
id="a"
base_url="https://a"`,
		"bad id": `data_dir="x"
[[site]]
id="Bad ID"
base_url="https://a"`,
		"duplicate": `data_dir="x"
[[site]]
id="a"
base_url="https://a"
[[site]]
id="a"
base_url="https://b"`,
		"relative url": `data_dir="x"
[[site]]
id="a"
base_url="/a"`,
		"unknown key": `data_dir="x"
typo = 1`,
		"empty folder": `data_dir="x"
[[site]]
id="a"
base_url="https://a"
publish.always_public_folders = [""]`,
		"same tags": `data_dir="x"
[[site]]
id="a"
base_url="https://a"
publish.public_tag = "x"
publish.private_tag = "X"`,
		"bad leak action": `data_dir="x"
[[site]]
id="a"
base_url="https://a"
publish.unpublished_links = "explode"`,
	}
	for name, src := range cases {
		if _, err := Parse([]byte(src)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

// TestShippedExamples keeps the example configs valid.
func TestShippedExamples(t *testing.T) {
	for _, p := range []string{"../../kvist.example.toml", "../../deploy/kvist.toml"} {
		if _, err := Load(p); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
}
