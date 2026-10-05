package render

import (
	"sort"
	"testing"
	"testing/fstest"
	"time"

	"github.com/klppl/kvist/themes"
)

func garden(t *testing.T) *Theme {
	t.Helper()
	th, err := LoadTheme(themes.Builtin("garden"), nil)
	if err != nil {
		t.Fatal(err)
	}
	return th
}

// TestGardenTranslationsComplete keeps every language in step with
// English: the same keys, the same plural tables and full name lists.
func TestGardenTranslationsComplete(t *testing.T) {
	th := garden(t)
	en := th.i18n["en"]
	if len(en) == 0 {
		t.Fatal("no English words")
	}
	keys := func(m map[string]any) []string {
		var out []string
		for k, v := range m {
			if sub, ok := v.(map[string]any); ok {
				for sk := range sub {
					out = append(out, k+"."+sk)
				}
				continue
			}
			out = append(out, k)
		}
		sort.Strings(out)
		return out
	}
	want := keys(en)
	for _, lang := range th.languages() {
		got := keys(th.i18n[lang])
		if len(got) != len(want) {
			t.Errorf("%s has %d keys, en has %d", lang, len(got), len(want))
		}
		have := map[string]bool{}
		for _, k := range got {
			have[k] = true
		}
		for _, k := range want {
			if !have[k] {
				t.Errorf("%s is missing %s", lang, k)
			}
		}
		m := th.Messages(lang)
		for key, n := range map[string]int{"months": 12, "months_short": 12, "days": 7, "days_short": 7} {
			if m.list(key, n) == nil {
				t.Errorf("%s: %s should be a list of %d words", lang, key, n)
			}
		}
	}
	if len(th.languages()) < 2 {
		t.Errorf("languages = %v", th.languages())
	}
}

func TestMessages(t *testing.T) {
	th := garden(t)
	sv := th.Messages("sv-SE")
	if got := sv.T("notes", 1); got != "1 anteckning" {
		t.Errorf("one = %q", got)
	}
	if got := sv.T("notes", 3); got != "3 anteckningar" {
		t.Errorf("other = %q", got)
	}
	if got := sv.T("reading_time", 4); got != "4 min läsning" {
		t.Errorf("reading_time = %q", got)
	}
	if got := th.Messages("pt-BR").T("planted"); got != "Planted" {
		t.Errorf("a language without a file falls back to English: %q", got)
	}
	if got := sv.T("no_such_key"); got != "no_such_key" {
		t.Errorf("missing key = %q", got)
	}
	if got := sv.Table("script")["close"]; got != "Stäng" {
		t.Errorf("script table close = %q", got)
	}

	d := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC) // a Monday
	for _, c := range []struct{ lang, layout, want string }{
		{"en", "2 Jan 2006", "4 May 2026"},
		{"sv", "2 Jan 2006", "4 maj 2026"},
		{"sv", "Monday 2 January 2006", "måndag 4 maj 2026"},
		{"de", "Mon, 2. January", "Mo., 4. Mai"},
		{"fr", "2006-01-02", "2026-05-04"},
		{"xx", "Jan 2, 2006", "May 4, 2026"},
	} {
		if got := th.Messages(c.lang).FormatDate(c.layout, d); got != c.want {
			t.Errorf("%s %q = %q, want %q", c.lang, c.layout, got, c.want)
		}
	}
}

// TestI18nOverrides adds a word and a language from theme_overrides.
func TestI18nOverrides(t *testing.T) {
	over := fstest.MapFS{
		"i18n/sv.toml": {Data: []byte(`planted = "Sådd"`)},
		"i18n/nb.toml": {Data: []byte(`planted = "Plantet"`)},
	}
	th, err := LoadTheme(themes.Builtin("garden"), over)
	if err != nil {
		t.Fatal(err)
	}
	if got := th.Messages("sv").T("planted"); got != "Sådd" {
		t.Errorf("overridden word = %q", got)
	}
	if got := th.Messages("sv").T("tended"); got != "Skött" {
		t.Errorf("the rest of sv.toml should stay: %q", got)
	}
	if got := th.Messages("nb").T("planted"); got != "Plantet" {
		t.Errorf("new language = %q", got)
	}
	if got := th.Messages("nb").T("tended"); got != "Tended" {
		t.Errorf("new language falls back to English: %q", got)
	}
}
