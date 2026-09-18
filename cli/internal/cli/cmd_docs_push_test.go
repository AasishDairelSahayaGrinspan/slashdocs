package cli

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// versionedDocsAPI is a minimal aidocs API that enforces the server's real
// optimistic-concurrency rule: a version create must name the document's
// current version as its base, or it answers 409 version_conflict. Consecutive
// pushes by a single writer therefore only work if the CLI remembers the
// version it just created.
type versionedDocsAPI struct {
	mu      sync.Mutex
	current map[string]string
	next    int
}

const pushTestDocID = "doc_1"

func newVersionedDocsAPI() *versionedDocsAPI {
	return &versionedDocsAPI{current: map[string]string{pushTestDocID: "ver_1"}, next: 1}
}

// nextVersionID mints the next version id. The caller holds the lock.
func (a *versionedDocsAPI) nextVersionID() string {
	a.next++
	return fmt.Sprintf("ver_%d", a.next)
}

// setCurrent stands in for a concurrent edit by another writer.
func (a *versionedDocsAPI) setCurrent(docID, versionID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.current[docID] = versionID
}

func (a *versionedDocsAPI) currentVersion(docID string) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.current[docID]
}

func (a *versionedDocsAPI) start(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/config":
			writeTestJSON(t, w, map[string]any{"deployment": "org", "org_name": "Acme", "everyone_label": "Acme"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/documents":
			requireMultipart(t, r)
			a.mu.Lock()
			defer a.mu.Unlock()
			version := a.nextVersionID()
			a.current["doc_created"] = version
			w.WriteHeader(http.StatusCreated)
			writeTestJSON(t, w, map[string]any{
				"id":                 "doc_created",
				"current_version_id": version,
				"default_grant":      map[string]any{"grant_id": "gr_1", "principal": map[string]any{"type": "anyone"}, "role": "viewer"},
			})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/versions"):
			requireMultipart(t, r)
			docID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v1/documents/"), "/versions")
			a.mu.Lock()
			defer a.mu.Unlock()
			latest := a.current[docID]
			if r.FormValue("base_version_id") != latest {
				w.WriteHeader(http.StatusConflict)
				writeTestJSON(t, w, map[string]any{"error": map[string]any{
					"code":    "version_conflict",
					"message": "base_version_id is stale",
					"details": map[string]any{"current_version_id": latest},
				}})
				return
			}
			version := a.nextVersionID()
			a.current[docID] = version
			w.WriteHeader(http.StatusCreated)
			writeTestJSON(t, w, map[string]any{"id": version, "number": a.next, "sha256": "abc"})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/html"):
			w.Header().Set("Content-Type", "text/html")
			writeTestString(t, w, "<h1>ok</h1>")
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/access"):
			writeTestJSON(t, w, map[string]any{"items": []any{map[string]any{
				"effective_role": "viewer",
				"sources":        []any{map[string]any{"grant_id": "gr_1"}},
			}}})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/documents/"):
			docID := strings.TrimPrefix(r.URL.Path, "/v1/documents/")
			a.mu.Lock()
			defer a.mu.Unlock()
			writeTestJSON(t, w, map[string]any{"id": docID, "current_version_id": a.current[docID]})
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// pushTestEnv points the CLI at srv with a throwaway config home, and returns
// the path of the HTML file the push commands upload.
func pushTestEnv(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	t.Setenv("AIDOCS_TOKEN", "tok")
	t.Setenv("AIDOCS_SERVER", srv.URL)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("AIDOCS_NO_BROWSER", "1")
	t.Setenv("AIDOCS_NO_KEYCHAIN", "1")
	t.Setenv("AIDOCS_DISABLE_AUTO_UPDATE", "1")
	html := filepath.Join(t.TempDir(), "report.html")
	writeTestFile(t, html, []byte("<h1>x</h1>"))
	return html
}

// storedBase reads the base version the CLI recorded for docID in contextName.
func storedBase(t *testing.T, contextName, docID string) string {
	t.Helper()
	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cx := cfg.Contexts[contextName]
	if cx == nil {
		t.Fatalf("config has no context %q (contexts: %v)", contextName, cfg.Contexts)
	}
	return cx.Pulled[docID]
}

// TestPushAfterPullRecordsNewBaseVersion is the pull -> push -> push loop from
// issue #101: the second push must not fail with a stale base.
func TestPushAfterPullRecordsNewBaseVersion(t *testing.T) {
	api := newVersionedDocsAPI()
	srv := api.start(t)
	html := pushTestEnv(t, srv)
	out := filepath.Join(t.TempDir(), "out.html")

	if _, err := Execute([]string{"docs", "pull", pushTestDocID, "--out", out}); err != nil {
		t.Fatalf("pull: %v", err)
	}
	if _, err := Execute([]string{"docs", "push", pushTestDocID, html}); err != nil {
		t.Fatalf("first push: %v", err)
	}
	if _, err := Execute([]string{"docs", "push", pushTestDocID, html}); err != nil {
		t.Fatalf("second push after a successful push: %v", err)
	}
	if got := api.currentVersion(pushTestDocID); got != "ver_3" {
		t.Fatalf("server current version = %q, want ver_3", got)
	}
}

// TestCreateRecordsInitialBaseVersion covers create -> push, which today fails
// with "no base version known".
func TestCreateRecordsInitialBaseVersion(t *testing.T) {
	api := newVersionedDocsAPI()
	srv := api.start(t)
	html := pushTestEnv(t, srv)

	if _, err := Execute([]string{"docs", "create", html, "--title", "T"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := Execute([]string{"docs", "push", "doc_created", html}); err != nil {
		t.Fatalf("push after create: %v", err)
	}
	if got := api.currentVersion("doc_created"); got != "ver_3" {
		t.Fatalf("server current version = %q, want ver_3", got)
	}
}

// TestPushStoresReturnedVersionAsBase locks in that the stored base is exactly
// the version id the server returned, for both create and push.
func TestPushStoresReturnedVersionAsBase(t *testing.T) {
	api := newVersionedDocsAPI()
	srv := api.start(t)
	html := pushTestEnv(t, srv)
	name := ctxName(srv.URL)

	if _, err := Execute([]string{"docs", "create", html, "--title", "T"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if got, want := storedBase(t, name, "doc_created"), api.currentVersion("doc_created"); got != want {
		t.Fatalf("stored base after create = %q, want %q", got, want)
	}
	if _, err := Execute([]string{"docs", "push", pushTestDocID, html, "--base-version", "ver_1"}); err != nil {
		t.Fatalf("push: %v", err)
	}
	if got, want := storedBase(t, name, pushTestDocID), api.currentVersion(pushTestDocID); got != want {
		t.Fatalf("stored base after push = %q, want %q", got, want)
	}
}

// TestConcurrentEditStillConflicts keeps the optimistic-concurrency guarantee:
// remembering our own version must not paper over somebody else's edit.
func TestConcurrentEditStillConflicts(t *testing.T) {
	api := newVersionedDocsAPI()
	srv := api.start(t)
	html := pushTestEnv(t, srv)
	out := filepath.Join(t.TempDir(), "out.html")

	if _, err := Execute([]string{"docs", "pull", pushTestDocID, "--out", out}); err != nil {
		t.Fatalf("pull: %v", err)
	}
	if _, err := Execute([]string{"docs", "push", pushTestDocID, html}); err != nil {
		t.Fatalf("first push: %v", err)
	}
	api.setCurrent(pushTestDocID, "ver_99")
	_, err := Execute([]string{"docs", "push", pushTestDocID, html})
	if err == nil || !strings.Contains(err.Error(), "version_conflict") {
		t.Fatalf("push over a concurrent edit should conflict, got %v", err)
	}
	if code := ExitCode(err); code != 5 {
		t.Fatalf("conflict exit code = %d, want 5", code)
	}
}

// TestBaseVersionFlagOverridesStoredBase keeps --base-version authoritative in
// both directions: a stale flag conflicts even when the stored base is current.
func TestBaseVersionFlagOverridesStoredBase(t *testing.T) {
	api := newVersionedDocsAPI()
	srv := api.start(t)
	html := pushTestEnv(t, srv)
	out := filepath.Join(t.TempDir(), "out.html")

	if _, err := Execute([]string{"docs", "pull", pushTestDocID, "--out", out}); err != nil {
		t.Fatalf("pull: %v", err)
	}
	if _, err := Execute([]string{"docs", "push", pushTestDocID, html}); err != nil {
		t.Fatalf("first push: %v", err)
	}
	_, err := Execute([]string{"docs", "push", pushTestDocID, html, "--base-version", "ver_1"})
	if err == nil || !strings.Contains(err.Error(), "version_conflict") {
		t.Fatalf("explicit stale --base-version should conflict, got %v", err)
	}
	if _, err := Execute([]string{"docs", "push", pushTestDocID, html}); err != nil {
		t.Fatalf("push on the remembered base: %v", err)
	}
}

// TestPushRecordsBaseInStoredServerContext checks the context the base lands
// in when --server names a server the user already has a context for: the
// existing context, not a second one, and without stealing the active context.
func TestPushRecordsBaseInStoredServerContext(t *testing.T) {
	api := newVersionedDocsAPI()
	srv := api.start(t)
	pushTestEnv(t, srv)
	t.Setenv("AIDOCS_SERVER", "")
	t.Setenv("AIDOCS_TOKEN", "")
	html := filepath.Join(t.TempDir(), "report.html")
	writeTestFile(t, html, []byte("<h1>x</h1>"))

	name := ctxName(srv.URL)
	cfg := Config{ActiveContext: "other.example", Contexts: map[string]*Context{
		"other.example": {Server: "https://other.example", Pulled: map[string]string{}},
		name:            {Server: normalizeServer(srv.URL), Credential: map[string]any{"token": "tok"}, Pulled: map[string]string{}},
	}}
	if err := saveConfig(cfg); err != nil {
		t.Fatal(err)
	}

	if _, err := Execute([]string{"--server", srv.URL, "docs", "push", pushTestDocID, html, "--base-version", "ver_1"}); err != nil {
		t.Fatalf("push: %v", err)
	}
	if got, want := storedBase(t, name, pushTestDocID), api.currentVersion(pushTestDocID); got != want {
		t.Fatalf("stored base = %q, want %q", got, want)
	}
	saved, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Contexts) != 2 {
		t.Fatalf("push created an extra context: %v", saved.Contexts)
	}
	if saved.ActiveContext != "other.example" {
		t.Fatalf("active context = %q, want other.example", saved.ActiveContext)
	}
}

// TestPushSucceedsWhenBaseVersionCannotBeRecorded keeps a config write problem
// from turning a version that exists on the server into a failed command.
func TestPushSucceedsWhenBaseVersionCannotBeRecorded(t *testing.T) {
	api := newVersionedDocsAPI()
	srv := api.start(t)
	html := pushTestEnv(t, srv)
	// A regular file where the config directory belongs: reads degrade to an
	// empty config, writes cannot succeed.
	writeTestFile(t, filepath.Join(configHome(t), "aidocs"), []byte("not a directory"))

	var out, errOut bytes.Buffer
	root := NewRoot(&out)
	root.SetErr(&errOut)
	root.SetArgs([]string{"docs", "push", pushTestDocID, html, "--base-version", "ver_1"})
	if err := root.Execute(); err != nil {
		t.Fatalf("push must not fail because the base could not be recorded: %v", err)
	}
	if !strings.Contains(out.String(), "Pushed version") {
		t.Fatalf("stdout = %q, want the pushed confirmation", out.String())
	}
	if !strings.Contains(errOut.String(), "--base-version") {
		t.Fatalf("stderr = %q, want a warning naming the --base-version fallback", errOut.String())
	}
}

func configHome(t *testing.T) string {
	t.Helper()
	dir := filepath.Dir(filepath.Dir(configPath()))
	return dir
}
