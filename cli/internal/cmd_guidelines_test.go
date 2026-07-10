package internal

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestGuidelinesIndexPointsToSections(t *testing.T) {
	got, err := Execute([]string{"guidelines"})
	if err != nil {
		t.Fatalf("guidelines returned error: %v", err)
	}
	for _, want := range []string{
		"aidocs document authoring guidelines",
		"aidocs guidelines core",
		"aidocs guidelines rendering theme",
		"aidocs guidelines interactivity kv",
		"aidocs guidelines all",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("index output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "window.aidocs.kv.get") {
		t.Fatalf("index should not include full KV detail:\n%s", got)
	}
}

func TestGuidelinesSectionLookup(t *testing.T) {
	got, err := Execute([]string{"guidelines", "rendering", "theme"})
	if err != nil {
		t.Fatalf("theme guidelines returned error: %v", err)
	}
	for _, want := range []string{
		"aidocs guidelines: rendering theme",
		`<html data-aidocs-theme="light">`,
		"window.addEventListener('aidocs:theme'",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("theme output missing %q:\n%s", want, got)
		}
	}
}

func TestGuidelinesKVAliasDocumentsPrimitive(t *testing.T) {
	got, err := Execute([]string{"guidelines", "kv"})
	if err != nil {
		t.Fatalf("KV guidelines returned error: %v", err)
	}
	for _, want := range []string{
		"window.aidocs.kv.get",
		"window.aidocs.kv.all",
		"window.aidocs.kv.set",
		"window.aidocs.kv.delete",
		"window.aidocs.kv.patch",
		"window.aidocs.kv.undo",
		"window.aidocs.kv.redo",
		`window.aidocs.on("kvchange"`,
		"KV writes persist immediately",
		"KV writes do not create document versions",
		"One document's keys are not visible to another",
		"Template/HTML updates still go through the ordinary publish/version flow",
		"Editors can write KV values",
		"set(key, null) deletes key",
		"event.changes exists on writes",
		"Ctrl/Cmd+Z undoes the last local KV write",
		`operation "undo" or "redo"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("KV output missing %q:\n%s", want, got)
		}
	}
}

func TestGuidelinesPublishingExamplesMatchCLIFlags(t *testing.T) {
	got, err := Execute([]string{"guidelines", "core", "publishing"})
	if err != nil {
		t.Fatalf("publishing guidelines returned error: %v", err)
	}
	for _, want := range []string{
		"aidocs docs create report.html --title 'Report'",
		"aidocs docs push <doc_id> report.html --base-version <version_id> --summary 'Address comments'",
		"aidocs docs pull <doc_id>",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("publishing output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "--visibility") {
		t.Fatalf("publishing output should not mention unsupported --visibility flag:\n%s", got)
	}
}

func TestGuidelinesUnknownSectionIsUsageError(t *testing.T) {
	_, err := Execute([]string{"guidelines", "missing"})
	if err == nil {
		t.Fatal("expected unknown section error")
	}
	var uerr usageError
	if !errors.As(err, &uerr) {
		t.Fatalf("expected usageError, got %T %v", err, err)
	}
	if got := ExitCode(err); got != 2 {
		t.Fatalf("ExitCode = %d, want 2", got)
	}
	if !strings.Contains(err.Error(), `unknown guidelines section "missing"`) || !strings.Contains(err.Error(), "aidocs guidelines interactivity kv") {
		t.Fatalf("unknown section error should be helpful, got %q", err.Error())
	}
}

func TestGuidelinesDoesNotCallUpdateEndpoints(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	defer srv.Close()
	t.Setenv("AIDOCS_SERVER", srv.URL)
	t.Setenv("AIDOCS_TOKEN", "tok")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("AIDOCS_DISABLE_AUTO_UPDATE", "")

	if _, err := Execute([]string{"guidelines"}); err != nil {
		t.Fatalf("guidelines returned error: %v", err)
	}
	if got := atomic.LoadInt32(&hits); got != 0 {
		t.Fatalf("guidelines should not make network requests, got %d", got)
	}
}
