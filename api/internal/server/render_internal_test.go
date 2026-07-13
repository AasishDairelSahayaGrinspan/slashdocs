package server

import (
	"errors"
	"regexp"
	"strings"
	"testing"
)

func requireEmbeddedBridge(t *testing.T) string {
	t.Helper()
	if bridgeJSSource == "" {
		t.Skip("bridge bundle is absent; make test builds it before running canonical checks")
	}
	return bridgeJSSource
}

func requireBridgePattern(t *testing.T, pattern string) {
	t.Helper()
	if !regexp.MustCompile(pattern).MatchString(requireEmbeddedBridge(t)) {
		t.Fatalf("embedded bridge is missing pattern %q", pattern)
	}
}

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
	bridge := requireEmbeddedBridge(t)
	for _, needle := range []string{
		"data-aidocs-theme",
		"aidocs_theme",
		"aidocs:theme",
	} {
		if !strings.Contains(bridge, needle) {
			t.Fatalf("bridge.js missing theme-handshake hook %q", needle)
		}
	}
	requireBridgePattern(t, `readyState\s*!==\s*["']loading["']`)
}

func TestBridgePreservesPostMessageSecurityBoundary(t *testing.T) {
	for _, pattern := range []string{
		`e\.source\s*!==\s*parent`,
		`e\.origin\s*===\s*origin`,
		`new URL\(appOrigin,\s*window\.location\.href\)\.origin`,
		`parent\.postMessage\(`,
	} {
		requireBridgePattern(t, pattern)
	}
}

func TestBridgePreservesWireMessageFamilies(t *testing.T) {
	bridge := requireEmbeddedBridge(t)
	for _, messageType := range []string{
		"aidocs:activate",
		"aidocs:keydown",
		"aidocs:kv-event",
		"aidocs:kv-request",
		"aidocs:kv-response",
		"aidocs:paint",
		"aidocs:ready",
		"aidocs:selection",
		"aidocs:theme",
		"aidocs:zoom",
	} {
		if !strings.Contains(bridge, messageType) {
			t.Fatalf("embedded bridge is missing wire message %q", messageType)
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
	bridge := requireEmbeddedBridge(t)
	for _, needle := range []string{
		"handleLinkClick",
		"prepareLinks",
		"scrollIntoView",
		"window.open",
		"noopener,noreferrer",
	} {
		if !strings.Contains(bridge, needle) {
			t.Fatalf("bridge.js missing document link handling hook %q", needle)
		}
	}
}

func TestBridgeDoesNotExposeManualEditContract(t *testing.T) {
	bridge := requireEmbeddedBridge(t)
	for _, needle := range []string{
		"aidocs:edit-mode",
		"aidocs:edit-dirty",
		"aidocs:edit-request-html",
		"aidocs:edit-html",
		"aidocs-edit-mode",
	} {
		if strings.Contains(bridge, needle) {
			t.Fatalf("bridge.js still exposes manual edit hook %q", needle)
		}
	}
}

func TestBridgeImplementsDocumentKVContract(t *testing.T) {
	bridge := requireEmbeddedBridge(t)
	for _, needle := range []string{
		"w.aidocs",
		"aidocs:kv-request",
		"aidocs:kv-response",
		"aidocs:kv-event",
		"kvchange",
	} {
		if !strings.Contains(bridge, needle) {
			t.Fatalf("bridge.js missing KV hook %q", needle)
		}
	}
	for _, pattern := range []string{
		`get\(key\)`,
		`all\(\)`,
		`set\(key,\s*value\)`,
		`delete\(key\)`,
		`patch\(object\)`,
		`undo\(\)`,
		`redo\(\)`,
	} {
		requireBridgePattern(t, pattern)
	}
}

func TestBridgeImplementsDocumentKVUndoRedoContract(t *testing.T) {
	bridge := requireEmbeddedBridge(t)
	for _, needle := range []string{
		"kvUndoStack",
		"kvRedoStack",
		"kvInversePatch",
		"handleKVUndoRedoShortcut",
		"isTextEditingTarget",
		"ev.ctrlKey",
		"ev.metaKey",
		"ev.shiftKey",
	} {
		if !strings.Contains(bridge, needle) {
			t.Fatalf("bridge.js missing KV undo/redo hook %q", needle)
		}
	}
	requireBridgePattern(t, `kvOperation\s*:\s*operation`)
	requireBridgePattern(t, `kvApplyPatch\(entry\.inverse,\s*["']undo["']\)`)
	requireBridgePattern(t, `kvApplyPatch\(entry\.forward,\s*["']redo["']\)`)
}

func TestRenderWrapperToleratesAbsentBridgeBundle(t *testing.T) {
	builtBridge := bridgeJSSource
	bridgeJSSource = ""
	t.Cleanup(func() { bridgeJSSource = builtBridge })

	out := string(renderWrapperHTML([]byte(`<html><body>hello</body></html>`), "https://app.example"))
	for _, needle := range []string{
		`<iframe id="aidocs-doc"`,
		`window.__AIDOCS_APP_ORIGIN__="https://app.example";`,
		`aidocs-bootstrap`,
	} {
		if !strings.Contains(out, needle) {
			t.Fatalf("render wrapper without bridge bundle is missing %q: %s", needle, out)
		}
	}
	if strings.Contains(out, "<no value>") || strings.Contains(out, "%!") {
		t.Fatalf("render wrapper exposed a missing-bundle template artifact: %s", out)
	}
}

func TestRenderWrapperInstallsKVBootstrap(t *testing.T) {
	out := string(renderWrapperHTML([]byte(`<html><body>hello</body></html>`), "https://app.example"))
	for _, needle := range []string{
		"aidocs-bootstrap",
		"__aidocsKVQueue",
		"window.aidocs",
		`undo:function(){return enqueue(&#34;undo&#34;,arguments);}`,
		`redo:function(){return enqueue(&#34;redo&#34;,arguments);}`,
	} {
		if !strings.Contains(out, needle) {
			t.Fatalf("render wrapper missing KV bootstrap hook %q: %s", needle, out)
		}
	}
}

func TestBridgePreludePreservesDocumentDoctype(t *testing.T) {
	userHTML := []byte(`<!doctype html><html><head><title>doc</title></head><body>hello</body></html>`)
	withPrelude := userHTMLWithBridgePrelude(userHTML)
	if !strings.HasPrefix(strings.ToLower(withPrelude), "<!doctype html>") {
		t.Fatalf("bridge prelude moved the user doctype out of first position: %s", withPrelude)
	}
	preludeAt := strings.Index(withPrelude, "aidocs-bootstrap")
	titleAt := strings.Index(withPrelude, "<title>")
	if preludeAt < 0 || titleAt < 0 || preludeAt > titleAt {
		t.Fatalf("bridge prelude should be installed before user head scripts/content: %s", withPrelude)
	}
}

func TestRenderWrapperScrubsOnlyRenderToken(t *testing.T) {
	out := string(renderWrapperHTML([]byte(`<html><body>hello</body></html>`), "https://app.example"))
	for _, needle := range []string{
		`searchParams.has("token")`,
		`searchParams.delete("token")`,
		`u.pathname+(u.search||"")+u.hash`,
	} {
		if !strings.Contains(out, needle) {
			t.Fatalf("render wrapper missing token-only URL scrub hook %q: %s", needle, out)
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

func TestValidateRenderOrigin(t *testing.T) {
	cases := []struct {
		name         string
		app, render  string
		wantRejected bool
	}{
		{"distinct hostnames", "https://app.example.com", "https://render.example.com", false},
		{"distinct hosts same port", "http://localhost:8080", "http://127.0.0.1:8080", false},
		{"same origin", "https://app.example.com", "https://app.example.com", true},
		{"same origin host case", "https://app.example.com", "https://APP.EXAMPLE.COM", true},
		{"same origin scheme case", "https://app.example.com", "HTTPS://app.example.com", true},
		{"same origin default HTTPS port", "https://app.example.com", "https://app.example.com:443", true},
		{"same origin zero-padded port", "https://app.example.com", "https://app.example.com:0443", true},
		{"same origin default HTTP port", "http://app.example.com", "http://app.example.com:80", true},
		{"same origin IDNA hostname", "https://bücher.example", "https://xn--bcher-kva.example", true},
		{"same origin non-transitional IDNA hostname", "https://faß.de", "https://xn--fa-hia.de", true},
		{"same origin equivalent IPv6", "https://[0:0:0:0:0:0:0:1]", "https://[::1]", true},
		{"same host terminal root dot", "https://app.example.com", "https://app.example.com.", true},
		{"ambiguous WHATWG IPv4 hostname", "http://127.1:8080", "http://127.0.0.1:8080", true},
		{"same host different scheme", "http://app.example.com", "https://app.example.com", true},
		{"same host different port", "http://app.example.com:8080", "http://app.example.com:9090", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateRenderOrigin(tc.app, tc.render)
			if got := err != nil; got != tc.wantRejected {
				t.Fatalf("ValidateRenderOrigin(%q, %q) error = %v, want rejected=%v", tc.app, tc.render, err, tc.wantRejected)
			}
		})
	}
}

func TestValidateRenderOriginClassifiesOnlyValidSameHostnameAsOverridable(t *testing.T) {
	equalErr := ValidateRenderOrigin("https://app.example", "https://APP.EXAMPLE:443")
	if !errors.Is(equalErr, ErrUnsafeRenderOrigin) {
		t.Fatalf("same-host origins returned %v, want ErrUnsafeRenderOrigin", equalErr)
	}

	invalidErr := ValidateRenderOrigin("http://127.1:8080", "http://127.0.0.1:8080")
	if invalidErr == nil || errors.Is(invalidErr, ErrUnsafeRenderOrigin) {
		t.Fatalf("ambiguous origin returned %v, want a non-overridable validation error", invalidErr)
	}
}
