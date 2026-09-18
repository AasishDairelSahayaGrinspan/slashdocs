package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// updateFixture serves everything the self-update path talks to: /version.json,
// the CLI asset, the agent skill, and enough of the API for `docs list` to
// succeed. Zero-valued fields describe a well-formed server advertising a build
// that differs from the local one, i.e. the case that would update.
type updateFixture struct {
	version    string // advertised build identifier
	payload    string // bytes served as the CLI asset
	skill      string // bytes served as SKILL.md
	assetURL   string // advertised asset URL, defaulting to the fixture's own path
	sha256     string // advertised digest, defaulting to the payload's real one
	omitSHA256 bool   // advertise no digest at all
	install    string // advertised install hint

	srv         *httptest.Server
	digest      string
	assetPath   string
	versionHits atomic.Int64
	assetHits   atomic.Int64
	skillHits   atomic.Int64
}

func (f *updateFixture) start(t *testing.T) *updateFixture {
	t.Helper()
	f.version = first(f.version, "server-build")
	f.payload = first(f.payload, "#!/bin/sh\necho served\n")
	f.skill = first(f.skill, "---\nname: aidocs\ndescription: served skill\n---\n")
	f.install = first(f.install, "/install.sh")
	sum := sha256.Sum256([]byte(f.payload))
	f.digest = hex.EncodeToString(sum[:])
	f.assetPath = "/cli/" + runtime.GOOS + "/" + runtime.GOARCH
	f.assetURL = first(f.assetURL, f.assetPath)
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/version.json":
			f.versionHits.Add(1)
			writeTestJSON(t, w, f.versionPayload())
		case f.assetPath:
			f.assetHits.Add(1)
			writeTestString(t, w, f.payload)
		case "/skills/aidocs/SKILL.md":
			f.skillHits.Add(1)
			w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
			writeTestString(t, w, f.skill)
		case "/v1/documents":
			writeTestJSON(t, w, map[string]any{"items": []any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *updateFixture) versionPayload() map[string]any {
	asset := map[string]any{"os": runtime.GOOS, "arch": runtime.GOARCH, "url": f.assetURL, "size": len(f.payload)}
	if !f.omitSHA256 {
		asset["sha256"] = first(f.sha256, f.digest)
	}
	return map[string]any{"version": f.version, "commit": "commit-sha", "install": f.install, "assets": []any{asset}}
}

// hits reports how many update-related requests the CLI made, which is how the
// tests prove an untrusted server is never contacted at all.
func (f *updateFixture) hits() (int64, int64, int64) {
	return f.versionHits.Load(), f.assetHits.Load(), f.skillHits.Load()
}

// pinVersion fixes the local build identifier for the duration of a test.
func pinVersion(t *testing.T, v string) {
	t.Helper()
	old := Version
	Version = v
	t.Cleanup(func() { Version = old })
}

// trustServer stores a logged-in context for server in an isolated config dir.
// That stored credential is what makes the server a trusted update source.
func trustServer(t *testing.T, server string) {
	t.Helper()
	t.Setenv("AIDOCS_NO_KEYCHAIN", "1")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	name := ctxName(server)
	cfg := Config{ActiveContext: name, Contexts: map[string]*Context{name: {
		Server:     normalizeServer(server),
		Credential: map[string]any{"id": "cred_1", "token": "tok"},
		Pulled:     map[string]string{},
	}}}
	if err := saveConfig(cfg); err != nil {
		t.Fatalf("save trusted context: %v", err)
	}
}

// isolateConfig points the CLI at an empty config dir, so no server is trusted.
func isolateConfig(t *testing.T) {
	t.Helper()
	t.Setenv("AIDOCS_NO_KEYCHAIN", "1")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
}

// fakeHome isolates the agent skill directories under a temp HOME.
func fakeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

func skillPathsIn(home string) []string {
	return []string{
		filepath.Join(home, ".agents", "skills", "aidocs", "SKILL.md"),
		filepath.Join(home, ".claude", "skills", "aidocs", "SKILL.md"),
	}
}

func requireNoSkillFiles(t *testing.T, home string) {
	t.Helper()
	for _, p := range skillPathsIn(home) {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("%s must not be written (stat error = %v)", p, err)
		}
	}
}

func requireSkillContents(t *testing.T, home, want string) {
	t.Helper()
	for _, p := range skillPathsIn(home) {
		if got := string(readTestFile(t, p)); got != want {
			t.Fatalf("%s content = %q, want %q", p, got, want)
		}
	}
}

// fakeExecutable points the self-update at a throwaway file so a test can
// assert whether the running binary would have been replaced.
func fakeExecutable(t *testing.T, contents string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "aidocs")
	writeTestFile(t, p, []byte(contents))
	old := executablePath
	executablePath = func() (string, error) { return p, nil }
	t.Cleanup(func() { executablePath = old })
	return p
}

func requireExecutable(t *testing.T, path, want string) {
	t.Helper()
	if got := string(readTestFile(t, path)); got != want {
		t.Fatalf("executable content = %q, want %q", got, want)
	}
}

// captureStderr collects what fn writes to os.Stderr, which is where the
// automatic update path reports.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	old := os.Stderr
	os.Stderr = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	fn()
	os.Stderr = old
	if err := w.Close(); err != nil {
		t.Fatalf("close stderr pipe: %v", err)
	}
	out := <-done
	if err := r.Close(); err != nil {
		t.Fatalf("close stderr reader: %v", err)
	}
	return out
}

// TestAutoUpdateSkipsServerWithoutLoggedInContext is the core guarantee: a
// one-off --server may not replace the binary or the agent skill files, and is
// not even asked for an update.
func TestAutoUpdateSkipsServerWithoutLoggedInContext(t *testing.T) {
	pinVersion(t, "local-build")
	home := fakeHome(t)
	isolateConfig(t)
	t.Setenv("AIDOCS_DISABLE_AUTO_UPDATE", "")
	f := (&updateFixture{}).start(t)
	exe := fakeExecutable(t, "original binary")

	stderr := captureStderr(t, func() {
		if _, err := Execute([]string{"docs", "list", "--server", f.srv.URL, "--token", "tok"}); err != nil {
			t.Errorf("docs list returned error: %v", err)
		}
	})

	requireExecutable(t, exe, "original binary")
	requireNoSkillFiles(t, home)
	if v, a, s := f.hits(); v != 0 || a != 0 || s != 0 {
		t.Fatalf("untrusted server was contacted for updates: version=%d asset=%d skill=%d", v, a, s)
	}
	if strings.TrimSpace(stderr) != "" {
		t.Fatalf("the skipped update check must stay silent, got %q", stderr)
	}
}

// TestAutoUpdateVerboseReportsUntrustedServer: silent by default, one line
// under --verbose so the behaviour is discoverable.
func TestAutoUpdateVerboseReportsUntrustedServer(t *testing.T) {
	pinVersion(t, "local-build")
	fakeHome(t)
	isolateConfig(t)
	f := (&updateFixture{}).start(t)
	fakeExecutable(t, "original binary")

	stderr := captureStderr(t, func() {
		if _, err := Execute([]string{"--verbose", "docs", "list", "--server", f.srv.URL, "--token", "tok"}); err != nil {
			t.Errorf("docs list returned error: %v", err)
		}
	})

	if !strings.Contains(stderr, "skipping the aidocs update check") || !strings.Contains(stderr, f.srv.URL) {
		t.Fatalf("--verbose should explain the skipped check, got %q", stderr)
	}
}

// TestAutoUpdateSkipsServerFromEnvWithoutLoggedInContext covers the second
// entry point for an attacker-chosen server: AIDOCS_SERVER.
func TestAutoUpdateSkipsServerFromEnvWithoutLoggedInContext(t *testing.T) {
	pinVersion(t, "local-build")
	home := fakeHome(t)
	isolateConfig(t)
	f := (&updateFixture{}).start(t)
	t.Setenv("AIDOCS_SERVER", f.srv.URL)
	t.Setenv("AIDOCS_TOKEN", "tok")
	exe := fakeExecutable(t, "original binary")

	if _, err := Execute([]string{"docs", "list"}); err != nil {
		t.Fatalf("docs list returned error: %v", err)
	}

	requireExecutable(t, exe, "original binary")
	requireNoSkillFiles(t, home)
	if v, a, s := f.hits(); v != 0 || a != 0 || s != 0 {
		t.Fatalf("untrusted server was contacted for updates: version=%d asset=%d skill=%d", v, a, s)
	}
}

// TestAutoUpdateAppliesFromLoggedInServer is the other half of the pin: the
// server the user logged in to still updates the binary and the skill.
func TestAutoUpdateAppliesFromLoggedInServer(t *testing.T) {
	pinVersion(t, "local-build")
	home := fakeHome(t)
	f := (&updateFixture{}).start(t)
	trustServer(t, f.srv.URL)
	exe := fakeExecutable(t, "original binary")

	if _, err := Execute([]string{"docs", "list", "--server", f.srv.URL, "--token", "tok"}); err != nil {
		t.Fatalf("docs list returned error: %v", err)
	}

	requireExecutable(t, exe, f.payload)
	requireSkillContents(t, home, f.skill)
}

// TestAutoUpdateHonorsDisableEnv locks in the existing opt-out.
func TestAutoUpdateHonorsDisableEnv(t *testing.T) {
	pinVersion(t, "local-build")
	home := fakeHome(t)
	f := (&updateFixture{}).start(t)
	trustServer(t, f.srv.URL)
	t.Setenv("AIDOCS_DISABLE_AUTO_UPDATE", "1")
	exe := fakeExecutable(t, "original binary")

	if _, err := Execute([]string{"docs", "list", "--server", f.srv.URL, "--token", "tok"}); err != nil {
		t.Fatalf("docs list returned error: %v", err)
	}

	requireExecutable(t, exe, "original binary")
	requireNoSkillFiles(t, home)
	if v, a, s := f.hits(); v != 0 || a != 0 || s != 0 {
		t.Fatalf("AIDOCS_DISABLE_AUTO_UPDATE still contacted the server: version=%d asset=%d skill=%d", v, a, s)
	}
}

// TestUpdateRejectsCrossOriginAsset: a trusted server may not hand the update
// off to another host.
func TestUpdateRejectsCrossOriginAsset(t *testing.T) {
	pinVersion(t, "local-build")
	fakeHome(t)
	var elsewhereHits atomic.Int64
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		elsewhereHits.Add(1)
		writeTestString(t, w, "attacker binary")
	}))
	t.Cleanup(elsewhere.Close)
	f := (&updateFixture{assetURL: elsewhere.URL + "/cli", payload: "attacker binary"}).start(t)
	trustServer(t, f.srv.URL)
	exe := fakeExecutable(t, "original binary")

	_, err := Execute([]string{"update", "--server", f.srv.URL})
	if err == nil || !strings.Contains(err.Error(), elsewhere.URL) || !strings.Contains(err.Error(), "same origin") {
		t.Fatalf("update error = %v, want a same-origin refusal naming %s", err, elsewhere.URL)
	}
	requireExecutable(t, exe, "original binary")
	if got := elsewhereHits.Load(); got != 0 {
		t.Fatalf("cross-origin asset was downloaded %d times", got)
	}
}

// TestUpdateRequiresAssetDigest: no checksum, no update — and no download.
func TestUpdateRequiresAssetDigest(t *testing.T) {
	pinVersion(t, "local-build")
	fakeHome(t)
	f := (&updateFixture{omitSHA256: true}).start(t)
	trustServer(t, f.srv.URL)
	exe := fakeExecutable(t, "original binary")

	_, err := Execute([]string{"update", "--server", f.srv.URL})
	if err == nil || !strings.Contains(err.Error(), "sha256") {
		t.Fatalf("update error = %v, want a missing-checksum refusal", err)
	}
	requireExecutable(t, exe, "original binary")
	if _, a, _ := f.hits(); a != 0 {
		t.Fatalf("asset was downloaded %d times despite the missing checksum", a)
	}
}

// TestUpdateRejectsMalformedAssetDigest covers the other half of "mandatory
// checksum": a digest that is present but not a sha256.
func TestUpdateRejectsMalformedAssetDigest(t *testing.T) {
	pinVersion(t, "local-build")
	fakeHome(t)
	f := (&updateFixture{sha256: "not-a-digest"}).start(t)
	trustServer(t, f.srv.URL)
	exe := fakeExecutable(t, "original binary")

	_, err := Execute([]string{"update", "--server", f.srv.URL})
	if err == nil || !strings.Contains(err.Error(), "sha256") {
		t.Fatalf("update error = %v, want a bad-checksum refusal", err)
	}
	requireExecutable(t, exe, "original binary")
}

// TestManualUpdateHintIgnoresServerInstallURL: the reinstall hint is always
// built from the trusted base, never from the JSON's install value.
func TestManualUpdateHintIgnoresServerInstallURL(t *testing.T) {
	pinVersion(t, "local-build")
	fakeHome(t)
	f := (&updateFixture{install: "https://install.example.com/pwn.sh", omitSHA256: true}).start(t)
	trustServer(t, f.srv.URL)
	fakeExecutable(t, "original binary")

	stderr := captureStderr(t, func() {
		if _, err := Execute([]string{"docs", "list", "--server", f.srv.URL, "--token", "tok"}); err != nil {
			t.Errorf("docs list returned error: %v", err)
		}
	})

	if !strings.Contains(stderr, "curl -fsSL "+f.srv.URL+"/install.sh") {
		t.Fatalf("stderr should offer the trusted install script, got %q", stderr)
	}
	if strings.Contains(stderr, "install.example.com") {
		t.Fatalf("stderr echoed the server-supplied install URL: %q", stderr)
	}
}

// TestSkillSyncLeavesIdenticalFileUntouched: unchanged content is never
// rewritten, so local mtimes stay stable.
func TestSkillSyncLeavesIdenticalFileUntouched(t *testing.T) {
	pinVersion(t, "local-build")
	home := fakeHome(t)
	f := (&updateFixture{}).start(t)
	trustServer(t, f.srv.URL)

	stamp := time.Date(2020, time.January, 2, 3, 4, 5, 0, time.UTC)
	for _, p := range skillPathsIn(home) {
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatalf("create skill dir: %v", err)
		}
		writeTestFile(t, p, []byte(f.skill))
		if err := os.Chtimes(p, stamp, stamp); err != nil {
			t.Fatalf("set skill mtime: %v", err)
		}
	}

	out, err := Execute([]string{"skills", "update", "--server", f.srv.URL})
	if err != nil {
		t.Fatalf("skills update returned error: %v", err)
	}
	if !strings.Contains(out, "already up to date") {
		t.Fatalf("skills update output = %q, want an unchanged report", out)
	}
	requireSkillContents(t, home, f.skill)
	for _, p := range skillPathsIn(home) {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("stat %s: %v", p, err)
		}
		if !info.ModTime().UTC().Equal(stamp) {
			t.Fatalf("%s was rewritten: mtime = %s, want %s", p, info.ModTime().UTC(), stamp)
		}
	}
}

// TestSkillSyncWritesChangedContent is the counterpart: real changes still land.
func TestSkillSyncWritesChangedContent(t *testing.T) {
	pinVersion(t, "local-build")
	home := fakeHome(t)
	f := (&updateFixture{}).start(t)
	trustServer(t, f.srv.URL)
	for _, p := range skillPathsIn(home) {
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatalf("create skill dir: %v", err)
		}
		writeTestFile(t, p, []byte("stale skill\n"))
	}

	out, err := Execute([]string{"skills", "update", "--server", f.srv.URL})
	if err != nil {
		t.Fatalf("skills update returned error: %v", err)
	}
	if !strings.Contains(out, "Updated aidocs skill") {
		t.Fatalf("skills update output = %q, want an updated report", out)
	}
	requireSkillContents(t, home, f.skill)
}

// TestUpdateCommandRefusesUntrustedServer: the explicit command fails loudly
// and actionably instead of silently doing nothing.
func TestUpdateCommandRefusesUntrustedServer(t *testing.T) {
	pinVersion(t, "local-build")
	home := fakeHome(t)
	isolateConfig(t)
	f := (&updateFixture{}).start(t)
	exe := fakeExecutable(t, "original binary")

	_, err := Execute([]string{"update", "--server", f.srv.URL})
	if err == nil {
		t.Fatal("update against an untrusted server must fail")
	}
	if !strings.Contains(err.Error(), f.srv.URL) || !strings.Contains(err.Error(), "aidocs auth login "+f.srv.URL) {
		t.Fatalf("update error = %q, want it to name the server and the login command", err.Error())
	}
	if got := ExitCode(err); got == 0 {
		t.Fatalf("ExitCode = %d, want non-zero", got)
	}
	requireExecutable(t, exe, "original binary")
	requireNoSkillFiles(t, home)
	if v, a, s := f.hits(); v != 0 || a != 0 || s != 0 {
		t.Fatalf("untrusted server was contacted: version=%d asset=%d skill=%d", v, a, s)
	}
}

// TestUpdateCommandForceDoesNotOverrideTrust: --force only re-installs the same
// build from a trusted server; it is not a trust override.
func TestUpdateCommandForceDoesNotOverrideTrust(t *testing.T) {
	pinVersion(t, "local-build")
	home := fakeHome(t)
	isolateConfig(t)
	f := (&updateFixture{}).start(t)
	exe := fakeExecutable(t, "original binary")

	_, err := Execute([]string{"update", "--force", "--server", f.srv.URL})
	if err == nil || !strings.Contains(err.Error(), "aidocs auth login") {
		t.Fatalf("update --force error = %v, want the same refusal as without --force", err)
	}
	requireExecutable(t, exe, "original binary")
	requireNoSkillFiles(t, home)
}

// TestSkillsUpdateCommandRefusesUntrustedServer covers the third entry point.
func TestSkillsUpdateCommandRefusesUntrustedServer(t *testing.T) {
	home := fakeHome(t)
	isolateConfig(t)
	f := (&updateFixture{}).start(t)

	_, err := Execute([]string{"skills", "update", "--server", f.srv.URL})
	if err == nil {
		t.Fatal("skills update against an untrusted server must fail")
	}
	if !strings.Contains(err.Error(), f.srv.URL) || !strings.Contains(err.Error(), "aidocs auth login "+f.srv.URL) {
		t.Fatalf("skills update error = %q, want it to name the server and the login command", err.Error())
	}
	if got := ExitCode(err); got == 0 {
		t.Fatalf("ExitCode = %d, want non-zero", got)
	}
	requireNoSkillFiles(t, home)
	if _, _, s := f.hits(); s != 0 {
		t.Fatalf("untrusted skill endpoint was fetched %d times", s)
	}
}

// TestTrustedUpdateSource pins the predicate itself: a stored context alone is
// not enough, the credential and the normalized server both have to match.
func TestTrustedUpdateSource(t *testing.T) {
	isolateConfig(t)
	cfg := Config{ActiveContext: "install.example.com", Contexts: map[string]*Context{
		// What install.sh writes: a context, but nobody has logged in yet.
		"install.example.com": {Server: "https://install.example.com", Pulled: map[string]string{}},
		// What `aidocs auth login` writes.
		"login.example.com": {Server: "https://login.example.com", Credential: map[string]any{"id": "cred_1"}, Pulled: map[string]string{}},
	}}
	if err := saveConfig(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	cases := []struct {
		server string
		want   bool
	}{
		{"https://login.example.com", true},
		{"login.example.com", true},
		{"https://login.example.com/", true},
		{"https://install.example.com", false},
		// Same host, different scheme than the stored context.
		{"http://login.example.com", false},
		{"https://login.example.com:8443", false},
		{"https://unknown.example.com", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := trustedUpdateSource(normalizeServer(tc.server)); got != tc.want {
			t.Fatalf("trustedUpdateSource(%q) = %v, want %v", tc.server, got, tc.want)
		}
	}
}

func TestUpdateCLIRefreshesBundledSkillWhenBinaryIsCurrent(t *testing.T) {
	pinVersion(t, "commit-sha")
	home := fakeHome(t)

	var skillHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/version.json":
			_ = json.NewEncoder(w).Encode(serverVersion{Version: "commit-sha"})
		case "/skills/aidocs/SKILL.md":
			skillHits++
			w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
			_, _ = w.Write([]byte("---\nname: aidocs\ndescription: test skill\n---\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	trustServer(t, srv.URL)

	v, changed, skillRefreshed, err := updateCLI(context.Background(), &Client{Base: srv.URL, HTTP: srv.Client()}, false)
	if err != nil {
		t.Fatalf("updateCLI returned error: %v", err)
	}
	if changed {
		t.Fatalf("changed = true, want false for already-current binary")
	}
	if !skillRefreshed {
		t.Fatalf("skillRefreshed = false, want true")
	}
	if v.Version != "commit-sha" {
		t.Fatalf("server version = %q", v.Version)
	}
	if skillHits != 1 {
		t.Fatalf("skill endpoint hits = %d, want 1", skillHits)
	}
	requireSkillContents(t, home, "---\nname: aidocs\ndescription: test skill\n---\n")
}

func TestUpdateCLIRefreshesBundledSkillFromDevServer(t *testing.T) {
	pinVersion(t, "local-build")
	home := fakeHome(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/version.json":
			_ = json.NewEncoder(w).Encode(serverVersion{Version: "dev"})
		case "/skills/aidocs/SKILL.md":
			_, _ = w.Write([]byte("---\nname: aidocs\ndescription: dev skill\n---\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	trustServer(t, srv.URL)

	v, changed, skillRefreshed, err := updateCLI(context.Background(), &Client{Base: srv.URL, HTTP: srv.Client()}, false)
	if err == nil || !strings.Contains(err.Error(), "no CLI asset") {
		t.Fatalf("updateCLI error = %v, want missing asset error", err)
	}
	if v.Version != "dev" {
		t.Fatalf("server version = %q", v.Version)
	}
	if !skillRefreshed {
		t.Fatalf("skillRefreshed = false, want true")
	}
	if changed {
		t.Fatalf("changed = true, want false because dev server published no binary asset")
	}
	requireSkillContents(t, home, "---\nname: aidocs\ndescription: dev skill\n---\n")
}
