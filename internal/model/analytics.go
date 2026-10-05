package model

import (
	"net/url"
	"regexp"
	"strings"
)

// Analytics settings are theme parameters, so they can be set in the
// settings note or the server's theme_params:
//
//	analytics:     plausible | umami | goatcounter
//	analytics_id:  the site's domain (Plausible), website ID (Umami) or
//	               code (GoatCounter)
//	analytics_url: the script's address, for a self-hosted service
//
// They are checked here, with warnings in the publish report, and handed
// to themes as SiteConfig.Analytics.

// Analytics is a privacy-friendly analytics service to load on every page.
type Analytics struct {
	Provider string `json:"provider"`           // plausible, umami or goatcounter
	ID       string `json:"id,omitempty"`       // domain, website ID or code
	Script   string `json:"script"`             // https URL of the script
	Endpoint string `json:"endpoint,omitempty"` // GoatCounter's count address
}

var (
	analyticsScripts = map[string]string{
		"plausible":   "https://plausible.io/js/script.js",
		"umami":       "https://cloud.umami.is/script.js",
		"goatcounter": "https://gc.zgo.at/count.js",
	}
	umamiID       = regexp.MustCompile(`^[0-9a-fA-F-]{8,64}$`)
	goatcounterID = regexp.MustCompile(`^[a-z0-9-]{1,63}$`)
	domainName    = regexp.MustCompile(`^[a-zA-Z0-9.-]+(,[a-zA-Z0-9.-]+)*$`)
)

// analytics reads the analytics settings from the site's parameters.
func (b *builder) analytics(c *SiteConfig) *Analytics {
	str := func(k string) string { return strings.TrimSpace(scalar(c.Params[k])) }
	provider := strings.ToLower(str("analytics"))
	if provider == "" || provider == "none" || provider == "false" {
		return nil
	}
	script, ok := analyticsScripts[provider]
	if !ok {
		b.warn(WarnSiteConfig, "", "analytics %q is not supported; use plausible, umami or goatcounter", provider)
		return nil
	}
	if u := str("analytics_url"); u != "" {
		if !isHTTPS(u) {
			b.warn(WarnSiteConfig, "", "analytics_url %q must be an https:// address; analytics left out", u)
			return nil
		}
		script = u
	}
	a := &Analytics{Provider: provider, Script: script, ID: str("analytics_id")}
	switch provider {
	case "plausible":
		if a.ID == "" {
			if u, err := url.Parse(c.BaseURL); err == nil {
				a.ID = u.Hostname()
			}
		}
		if !domainName.MatchString(a.ID) {
			b.warn(WarnSiteConfig, "", "analytics_id %q should be the site's domain, as Plausible knows it; analytics left out", a.ID)
			return nil
		}
	case "umami":
		if !umamiID.MatchString(a.ID) {
			b.warn(WarnSiteConfig, "", "analytics_id %q should be the Umami website ID; analytics left out", a.ID)
			return nil
		}
	case "goatcounter":
		switch {
		case goatcounterID.MatchString(a.ID):
			a.Endpoint = "https://" + a.ID + ".goatcounter.com/count"
		case isHTTPS(a.ID):
			a.Endpoint = a.ID // a self-hosted GoatCounter's /count address
		default:
			b.warn(WarnSiteConfig, "", "analytics_id %q should be the GoatCounter code (the name in code.goatcounter.com) or its https:// count address; analytics left out", a.ID)
			return nil
		}
	}
	return a
}

func isHTTPS(s string) bool {
	u, err := url.Parse(s)
	return err == nil && u.Scheme == "https" && u.Host != "" && !strings.ContainsAny(s, "\"'<> ")
}
