package main

import (
	"strings"
	"testing"

	"github.com/klppl/kvist/internal/config"
)

func TestTokenSite(t *testing.T) {
	cfg := func(ids ...string) *config.Config {
		c := &config.Config{}
		for _, id := range ids {
			c.Sites = append(c.Sites, &config.Site{ID: id})
		}
		return c
	}
	for _, tc := range []struct {
		cfg     *config.Config
		id      string
		want    string // the site's id, or a part of the error
		wantErr bool
	}{
		{cfg("garden"), "", "garden", false},
		{cfg("garden"), "garden", "garden", false},
		{cfg("garden"), "other", `unknown site "other" (sites: garden)`, true},
		{cfg("a", "b"), "b", "b", false},
		{cfg("a", "b"), "", "several sites (a, b); choose one with --site", true},
		{cfg(), "", "no sites", true},
		{cfg(), "garden", "no sites", true},
	} {
		sc, err := tokenSite(tc.cfg, tc.id)
		switch {
		case tc.wantErr && (err == nil || !strings.Contains(err.Error(), tc.want)):
			t.Errorf("tokenSite(%d sites, %q) error = %v, want %q", len(tc.cfg.Sites), tc.id, err, tc.want)
		case !tc.wantErr && (err != nil || sc.ID != tc.want):
			t.Errorf("tokenSite(%d sites, %q) = %v, %v, want %s", len(tc.cfg.Sites), tc.id, sc, err, tc.want)
		}
	}
}
