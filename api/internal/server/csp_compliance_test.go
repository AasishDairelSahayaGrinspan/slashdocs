package server

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// forbidden are resource references the render CSP
// (default-src 'none'; img-src data: https:; style-src 'unsafe-inline';
// script-src 'unsafe-inline') blocks inside a rendered document. Any shipped
// example/template HTML that uses them would silently fail to render.
var cspForbiddenPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)<link[^>]+rel=["']?stylesheet`),
	regexp.MustCompile(`(?i)<script[^>]+src=`),
	regexp.MustCompile(`(?i)@import\s+url`),
	regexp.MustCompile(`(?i)@font-face`),
	regexp.MustCompile(`(?i)fonts\.googleapis\.com`),
	regexp.MustCompile(`(?i)\bmermaid\b`),
}

func scanForbidden(html string) []string {
	var hits []string
	for _, re := range cspForbiddenPatterns {
		if re.MatchString(html) {
			hits = append(hits, re.String())
		}
	}
	return hits
}

func TestScanForbiddenDetectsExternalAssets(t *testing.T) {
	bad := `<link rel="stylesheet" href="https://cdn/x.css">`
	if got := scanForbidden(bad); len(got) == 0 {
		t.Fatal("expected forbidden detector to flag external stylesheet")
	}
	good := `<style>body{color:var(--ink)}</style><svg viewBox="0 0 10 10"></svg>`
	if got := scanForbidden(good); len(got) != 0 {
		t.Fatalf("expected inline HTML to pass, flagged: %v", got)
	}
}

func TestShippedHTMLIsCSPCompliant(t *testing.T) {
	roots := []string{"onboarding"} // extend as templates are added
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".html") {
				return nil
			}
			b, _ := os.ReadFile(path)
			if hits := scanForbidden(string(b)); len(hits) != 0 {
				t.Errorf("%s violates render CSP: %v", path, hits)
			}
			return nil
		})
	}
}
