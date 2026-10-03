// Package config loads and validates the server configuration (TOML).
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/klppl/kvist/internal/protocol"
)

// Config is the server configuration file.
type Config struct {
	DataDir   string   `toml:"data_dir"`
	Listen    string   `toml:"listen"`
	ThemesDir string   `toml:"themes_dir"`
	TLSCert   string   `toml:"tls_cert"`
	TLSKey    string   `toml:"tls_key"`
	SyncTTL   Duration `toml:"sync_ttl"`
	Sites     []*Site  `toml:"site"`
}

// Site configures one published site.
type Site struct {
	ID          string         `toml:"id"`
	BaseURL     string         `toml:"base_url"`
	Title       string         `toml:"title"`
	Description string         `toml:"description"`
	Author      string         `toml:"author"`
	Language    string         `toml:"language"`
	Theme       string         `toml:"theme"`
	ThemeDir    string         `toml:"theme_overrides"` // folder whose files override the theme's
	Serve       bool           `toml:"serve"`
	Publish     Publish        `toml:"publish"`
	Limits      Limits         `toml:"limits"`
	Retention   Retention      `toml:"retention"`
	Cloudflare  Cloudflare     `toml:"cloudflare"`
	ThemeParams map[string]any `toml:"theme_params"`
}

// Cloudflare purges the zone's cache after every successful build. The API
// token (permission: Zone → Cache Purge) is read from an environment
// variable or a file, never from the config itself.
type Cloudflare struct {
	ZoneID       string `toml:"zone_id"`
	APITokenEnv  string `toml:"api_token_env"`
	APITokenFile string `toml:"api_token_file"`
}

// Token returns the Cloudflare API token, or "" if purging is off.
func (c Cloudflare) Token() (string, error) {
	if c.ZoneID == "" {
		return "", nil
	}
	if c.APITokenEnv != "" {
		if v := strings.TrimSpace(os.Getenv(c.APITokenEnv)); v != "" {
			return v, nil
		}
	}
	if c.APITokenFile != "" {
		b, err := os.ReadFile(c.APITokenFile)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(b)), nil
	}
	return "", fmt.Errorf("cloudflare.zone_id is set but no API token was found (api_token_env or api_token_file)")
}

// Publish holds the publish rules and leak-handling options of a site.
type Publish struct {
	AlwaysPublicFolders  []string `toml:"always_public_folders"`
	ExcludeFolders       []string `toml:"exclude_folders"`
	PublicTag            string   `toml:"public_tag"`
	PrivateTag           string   `toml:"private_tag"`
	FrontmatterKey       string   `toml:"frontmatter_key"`
	AttachmentExtensions []string `toml:"attachment_extensions"`
	UnpublishedLinks     string   `toml:"unpublished_links"`  // ignore | warn | error
	UnpublishedEmbeds    string   `toml:"unpublished_embeds"` // ignore | warn | error
	ExposeFrontmatter    []string `toml:"expose_frontmatter"`
	StripImageMetadata   *bool    `toml:"strip_image_metadata"`
}

// Limits are per-site upload limits.
type Limits struct {
	MaxFileSize int64 `toml:"max_file_size"`
	MaxFiles    int   `toml:"max_files"`
}

// Retention controls how much history is kept on disk.
type Retention struct {
	Revisions int `toml:"revisions"`
	Builds    int `toml:"builds"`
}

// Duration is a time.Duration written as a string ("1h", "90s") in TOML.
type Duration struct{ time.Duration }

// UnmarshalText implements encoding.TextUnmarshaler.
func (d *Duration) UnmarshalText(b []byte) error {
	v, err := time.ParseDuration(string(b))
	if err != nil {
		return err
	}
	d.Duration = v
	return nil
}

// Defaults.
const (
	DefaultListen      = "127.0.0.1:8080"
	DefaultSyncTTL     = time.Hour
	DefaultMaxFileSize = 50 << 20
	DefaultMaxFiles    = 20000
	DefaultRevisions   = 10
	DefaultBuilds      = 3
	DefaultTheme       = "garden"
	DefaultLanguage    = "en"
	DefaultPublicTag   = "public"
	DefaultPrivateTag  = "private"
	DefaultFrontmatter = "publish"
	LeakActionIgnore   = "ignore"
	LeakActionWarn     = "warn"
	LeakActionError    = "error"
)

// DefaultAttachmentExtensions are accepted when a site lists none.
var DefaultAttachmentExtensions = []string{
	"png", "jpg", "jpeg", "gif", "webp", "svg", "avif",
	"pdf", "mp3", "m4a", "ogg", "wav", "mp4", "webm",
}

var siteIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// ValidSiteID reports whether id is a valid site identifier.
func ValidSiteID(id string) bool { return siteIDRe.MatchString(id) }

// Load reads, defaults and validates a config file.
func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(b)
}

// Parse decodes, defaults and validates config bytes.
func Parse(b []byte) (*Config, error) {
	var c Config
	md, err := toml.Decode(string(b), &c)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	if und := md.Undecoded(); len(und) > 0 {
		keys := make([]string, len(und))
		for i, k := range und {
			keys[i] = k.String()
		}
		return nil, fmt.Errorf("config: unknown keys: %s", strings.Join(keys, ", "))
	}
	c.applyDefaults()
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) applyDefaults() {
	if c.Listen == "" {
		c.Listen = DefaultListen
	}
	if c.SyncTTL.Duration == 0 {
		c.SyncTTL.Duration = DefaultSyncTTL
	}
	if c.ThemesDir == "" {
		c.ThemesDir = "themes"
	}
	for _, s := range c.Sites {
		s.applyDefaults()
	}
}

func (s *Site) applyDefaults() {
	if s.Theme == "" {
		s.Theme = DefaultTheme
	}
	if s.Language == "" {
		s.Language = DefaultLanguage
	}
	if s.Title == "" {
		s.Title = s.ID
	}
	p := &s.Publish
	if p.PublicTag == "" {
		p.PublicTag = DefaultPublicTag
	}
	if p.PrivateTag == "" {
		p.PrivateTag = DefaultPrivateTag
	}
	if p.FrontmatterKey == "" {
		p.FrontmatterKey = DefaultFrontmatter
	}
	if p.AttachmentExtensions == nil {
		p.AttachmentExtensions = append([]string{}, DefaultAttachmentExtensions...)
	}
	for i, e := range p.AttachmentExtensions {
		p.AttachmentExtensions[i] = strings.ToLower(strings.TrimPrefix(e, "."))
	}
	p.PublicTag = strings.TrimPrefix(p.PublicTag, "#")
	p.PrivateTag = strings.TrimPrefix(p.PrivateTag, "#")
	p.AlwaysPublicFolders = cleanFolders(p.AlwaysPublicFolders)
	p.ExcludeFolders = cleanFolders(p.ExcludeFolders)
	if p.UnpublishedLinks == "" {
		p.UnpublishedLinks = LeakActionWarn
	}
	if p.UnpublishedEmbeds == "" {
		p.UnpublishedEmbeds = LeakActionWarn
	}
	if p.StripImageMetadata == nil {
		t := true
		p.StripImageMetadata = &t
	}
	if s.Limits.MaxFileSize == 0 {
		s.Limits.MaxFileSize = DefaultMaxFileSize
	}
	if s.Limits.MaxFiles == 0 {
		s.Limits.MaxFiles = DefaultMaxFiles
	}
	if s.Retention.Revisions == 0 {
		s.Retention.Revisions = DefaultRevisions
	}
	if s.Retention.Builds == 0 {
		s.Retention.Builds = DefaultBuilds
	}
}

// cleanFolders trims slashes so "Garden/" and "/Garden" both mean "Garden".
// The vault root must be written "/" and is kept as "/"; an empty entry is
// left empty and rejected by Validate.
func cleanFolders(in []string) []string {
	out := make([]string, 0, len(in))
	for _, f := range in {
		f = protocol.NormalizePath(strings.TrimSpace(f))
		if f != "" && strings.Trim(f, "/") == "" {
			out = append(out, "/")
			continue
		}
		out = append(out, strings.Trim(f, "/"))
	}
	return out
}

// Validate checks the configuration for errors.
func (c *Config) Validate() error {
	var errs []error
	if c.DataDir == "" {
		errs = append(errs, errors.New("data_dir is required"))
	}
	if (c.TLSCert == "") != (c.TLSKey == "") {
		errs = append(errs, errors.New("tls_cert and tls_key must be set together"))
	}
	seen := map[string]bool{}
	for i, s := range c.Sites {
		where := fmt.Sprintf("site[%d]", i)
		if s.ID != "" {
			where = fmt.Sprintf("site %q", s.ID)
		}
		if !ValidSiteID(s.ID) {
			errs = append(errs, fmt.Errorf("%s: id must match %s", where, siteIDRe))
		}
		if seen[s.ID] {
			errs = append(errs, fmt.Errorf("%s: duplicate id", where))
		}
		seen[s.ID] = true
		if s.BaseURL == "" {
			errs = append(errs, fmt.Errorf("%s: base_url is required", where))
		} else if u, err := url.Parse(s.BaseURL); err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			errs = append(errs, fmt.Errorf("%s: base_url must be an absolute http(s) URL", where))
		} else if strings.Trim(u.Path, "/") != "" || u.RawQuery != "" || u.Fragment != "" {
			errs = append(errs, fmt.Errorf("%s: base_url must be the root of a host (sites under a sub-path are not supported yet)", where))
		}
		p := s.Publish
		for _, v := range []struct{ key, val string }{
			{"unpublished_links", p.UnpublishedLinks},
			{"unpublished_embeds", p.UnpublishedEmbeds},
		} {
			if v.val != LeakActionIgnore && v.val != LeakActionWarn && v.val != LeakActionError {
				errs = append(errs, fmt.Errorf("%s: publish.%s must be ignore, warn or error", where, v.key))
			}
		}
		if strings.EqualFold(p.PublicTag, p.PrivateTag) {
			errs = append(errs, fmt.Errorf("%s: public_tag and private_tag must differ", where))
		}
		for _, f := range append(append([]string{}, p.AlwaysPublicFolders...), p.ExcludeFolders...) {
			if f == "" {
				errs = append(errs, fmt.Errorf("%s: empty folder in publish rules (write \"/\" for the whole vault)", where))
			}
		}
		for _, e := range p.AttachmentExtensions {
			if e == "md" || e == "" {
				errs = append(errs, fmt.Errorf("%s: invalid attachment extension %q", where, e))
			}
		}
		if s.Limits.MaxFileSize < 0 || s.Limits.MaxFiles < 0 {
			errs = append(errs, fmt.Errorf("%s: limits must be positive", where))
		}
		if cf := s.Cloudflare; cf.ZoneID != "" && cf.APITokenEnv == "" && cf.APITokenFile == "" {
			errs = append(errs, fmt.Errorf("%s: cloudflare needs api_token_env or api_token_file", where))
		}
		if s.Retention.Revisions < 1 || s.Retention.Builds < 1 {
			errs = append(errs, fmt.Errorf("%s: retention must keep at least 1", where))
		}
	}
	return errors.Join(errs...)
}

// Site returns the site with the given id, or nil.
func (c *Config) Site(id string) *Site {
	for _, s := range c.Sites {
		if s.ID == id {
			return s
		}
	}
	return nil
}

// Rules returns the site's publish rules in wire form.
func (s *Site) Rules() protocol.Rules {
	p := s.Publish
	return protocol.Rules{
		AlwaysPublicFolders:  append([]string{}, p.AlwaysPublicFolders...),
		ExcludeFolders:       append([]string{}, p.ExcludeFolders...),
		PublicTag:            p.PublicTag,
		PrivateTag:           p.PrivateTag,
		FrontmatterKey:       p.FrontmatterKey,
		AttachmentExtensions: append([]string{}, p.AttachmentExtensions...),
	}
}

// ProtocolLimits returns the site's limits in wire form.
func (s *Site) ProtocolLimits() protocol.Limits {
	return protocol.Limits{
		MaxFileSize:  s.Limits.MaxFileSize,
		MaxFiles:     s.Limits.MaxFiles,
		MaxPathBytes: protocol.DefaultMaxPathBytes,
	}
}
