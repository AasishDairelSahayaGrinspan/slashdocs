package server

import (
	"strings"
	"testing"
)

func TestRenderWrapperPreservesInlineScriptClosingTag(t *testing.T) {
	userHTML := []byte(`<html><body><script>window.__aidocsRegression=42;</script></body></html>`)
	out := string(renderWrapperHTML(userHTML, "https://app.example"))

	for _, needle := range []string{`<\/script`, `&lt;\/script`, `<\\/script`} {
		if strings.Contains(out, needle) {
			t.Fatalf("rendered wrapper contains %q, which corrupts user inline <script>: %s", needle, out)
		}
	}

	if !strings.Contains(out, `&lt;/script&gt;`) {
		t.Fatalf("expected user's </script> close to be HTML-entity-encoded inside srcdoc; got: %s", out)
	}
}

func TestBridgeImplementsThemeContract(t *testing.T) {
	for _, needle := range []string{
		"data-aidocs-theme",
		"aidocs_theme",
		"aidocs:theme",
	} {
		if !strings.Contains(bridgeJSSource, needle) {
			t.Fatalf("bridge.js missing theme-handshake hook %q", needle)
		}
	}
}

func TestRenderWrapperAllowsSafePopups(t *testing.T) {
	out := string(renderWrapperHTML([]byte(`<html><body><a href="https://example.com">x</a></body></html>`), "https://app.example"))
	for _, needle := range []string{"allow-popups", "allow-popups-to-escape-sandbox"} {
		if !strings.Contains(out, needle) {
			t.Fatalf("render wrapper sandbox missing %q: %s", needle, out)
		}
	}
}

func TestBridgeHandlesDocumentLinks(t *testing.T) {
	for _, needle := range []string{
		"handleLinkClick",
		"prepareLinks",
		"scrollIntoView",
		"window.open",
		"noopener,noreferrer",
	} {
		if !strings.Contains(bridgeJSSource, needle) {
			t.Fatalf("bridge.js missing document link handling hook %q", needle)
		}
	}
}

func TestBridgeImplementsEditModeContract(t *testing.T) {
	for _, needle := range []string{
		"aidocs:edit-mode",
		"aidocs:edit-dirty",
		"aidocs:edit-request-html",
		"aidocs:edit-html",
		"contenteditable",
		"isParentMessage",
	} {
		if !strings.Contains(bridgeJSSource, needle) {
			t.Fatalf("bridge.js missing edit-mode hook %q", needle)
		}
	}
}

func TestRenderWrapperLeavesScriptSubstringInJSAlone(t *testing.T) {
	userHTML := []byte(`<html><body><script>var s = "</script>oops";</script></body></html>`)
	out := string(renderWrapperHTML(userHTML, "https://app.example"))
	if strings.Contains(out, `\/script`) {
		t.Fatalf("rendered wrapper still injects backslash before /script: %s", out)
	}
	if !strings.Contains(out, `&lt;/script&gt;oops&#34;;&lt;/script&gt;`) {
		t.Fatalf("expected both inline </script> occurrences to be HTML-entity-encoded; got: %s", out)
	}
}
