package render

import (
	"regexp"
	"strings"
	"testing"
)

// A light-only token colour must never apply in dark mode: github defines
// classes github-dark doesn't, and those came out #1f2328 on #0d1117.
func TestSyntaxCSSScopesBothStyles(t *testing.T) {
	css, err := syntaxCSS("github", "github-dark")
	if err != nil {
		t.Fatal(err)
	}
	unscoped := regexp.MustCompile(`(^|[,}\n])\s*\.chroma`)
	if m := unscoped.FindString(css); m != "" {
		t.Errorf("a .chroma rule applies in both modes: %q", m)
	}
	for _, want := range []string{
		`:root[data-theme="light"] .chroma`,
		"@media not all and (prefers-color-scheme: dark) {\n",
		`:root:not([data-theme="dark"]) .chroma`,
		`:root[data-theme="dark"] .chroma`,
		`:root:not([data-theme="light"]) .chroma`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("missing %q", want)
		}
	}
}
