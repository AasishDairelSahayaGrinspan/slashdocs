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

func TestBridgeDoesNotExposeManualEditContract(t *testing.T) {
	for _, needle := range []string{
		"aidocs:edit-mode",
		"aidocs:edit-dirty",
		"aidocs:edit-request-html",
		"aidocs:edit-html",
		"aidocs-edit-mode",
	} {
		if strings.Contains(bridgeJSSource, needle) {
			t.Fatalf("bridge.js still exposes manual edit hook %q", needle)
		}
	}
}

func TestBridgeImplementsDocumentKVContract(t *testing.T) {
	for _, needle := range []string{
		"w.aidocs",
		"aidocs:kv-request",
		"aidocs:kv-response",
		"aidocs:kv-event",
		"kvchange",
		"get(key)",
		"all()",
		"set(key, value)",
		"delete(key)",
		"patch(object)",
		"undo()",
		"redo()",
	} {
		if !strings.Contains(bridgeJSSource, needle) {
			t.Fatalf("bridge.js missing KV hook %q", needle)
		}
	}
}

func TestBridgeImplementsDocumentKVUndoRedoContract(t *testing.T) {
	for _, needle := range []string{
		"kvUndoStack",
		"kvRedoStack",
		"kvInversePatch",
		`kvApplyPatch(entry.inverse, "undo")`,
		`kvApplyPatch(entry.forward, "redo")`,
		"kvOperation: operation",
		"handleKVUndoRedoShortcut",
		"isTextEditingTarget",
		"ev.ctrlKey",
		"ev.metaKey",
		"ev.shiftKey",
	} {
		if !strings.Contains(bridgeJSSource, needle) {
			t.Fatalf("bridge.js missing KV undo/redo hook %q", needle)
		}
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
