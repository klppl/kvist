package model

import (
	"reflect"
	"strings"
	"testing"
)

func TestAnalytics(t *testing.T) {
	for _, c := range []struct {
		name, props string
		want        *Analytics
		warn        string
	}{
		{"none", "", nil, ""},
		{"plausible defaults to the site's domain", "analytics: plausible",
			&Analytics{Provider: "plausible", ID: "garden.example.com", Script: "https://plausible.io/js/script.js"}, ""},
		{"plausible self-hosted", "analytics: Plausible\nanalytics_id: example.org\nanalytics_url: https://stats.example.org/js/script.js",
			&Analytics{Provider: "plausible", ID: "example.org", Script: "https://stats.example.org/js/script.js"}, ""},
		{"umami", "analytics: umami\nanalytics_id: 94db1cb1-74f4-4a40-ad6c-962362670409",
			&Analytics{Provider: "umami", ID: "94db1cb1-74f4-4a40-ad6c-962362670409", Script: "https://cloud.umami.is/script.js"}, ""},
		{"umami needs an id", "analytics: umami", nil, "Umami website ID"},
		{"goatcounter code", "analytics: goatcounter\nanalytics_id: ada",
			&Analytics{Provider: "goatcounter", ID: "ada", Script: "https://gc.zgo.at/count.js", Endpoint: "https://ada.goatcounter.com/count"}, ""},
		{"goatcounter self-hosted", "analytics: goatcounter\nanalytics_id: https://stats.example.org/count\nanalytics_url: https://stats.example.org/count.js",
			&Analytics{Provider: "goatcounter", ID: "https://stats.example.org/count", Script: "https://stats.example.org/count.js", Endpoint: "https://stats.example.org/count"}, ""},
		{"unknown provider", "analytics: google", nil, "not supported"},
		{"plain http script", "analytics: plausible\nanalytics_url: http://stats.example.org/script.js", nil, "https://"},
		{"script url with a quote", "analytics: plausible\nanalytics_url: \"https://x.example/a\\\"onload=\\\"b.js\"", nil, "https://"},
		{"javascript url", "analytics: umami\nanalytics_id: 94db1cb1\nanalytics_url: javascript:alert(1)", nil, "https://"},
	} {
		t.Run(c.name, func(t *testing.T) {
			site := buildVault(t, map[string]string{
				"Kvist/Note.md":  "Text.",
				"Kvist/_site.md": "---\n" + c.props + "\n---\n",
			})
			if !reflect.DeepEqual(site.Config.Analytics, c.want) {
				t.Errorf("analytics = %+v, want %+v", site.Config.Analytics, c.want)
			}
			var warned bool
			for _, w := range site.Warnings {
				warned = warned || (c.warn != "" && strings.Contains(w.Message, c.warn))
			}
			if c.warn != "" && !warned {
				t.Errorf("no warning containing %q: %v", c.warn, site.Warnings)
			}
		})
	}
}
