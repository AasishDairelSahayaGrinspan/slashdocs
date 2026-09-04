package server_test

import (
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"

	"github.com/razorpay/aidocs/api/internal/server"
)

func TestInstallScriptUsesConfiguredAppOriginNotForwardedHost(t *testing.T) {
	e := newConfiguredTestEnv(t, server.Config{Environment: "test", AppOrigin: "https://aidocs.example", SessionSecret: "test-secret"})
	req := httptest.NewRequest(http.MethodGet, "/install.sh", nil)
	req.Host = "aidocs.example"
	req.Header.Set("X-Forwarded-Host", "evil.example")
	req.Header.Set("X-Forwarded-Proto", "https")
	rr := httptest.NewRecorder()
	e.h.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusOK)
	body := rr.Body.String()
	if strings.Contains(body, "evil.example") {
		t.Fatalf("install script reflected attacker-controlled forwarded host: %q", body)
	}
	if !strings.Contains(body, `BASE_URL="${AIDOCS_BASE_URL:-https://aidocs.example}"`) {
		t.Fatalf("install script did not use configured app origin: %q", body)
	}
}

func TestInstallScriptInstallsBundledSkill(t *testing.T) {
	e := newConfiguredTestEnv(t, server.Config{Environment: "test", AppOrigin: "https://aidocs.example", SessionSecret: "test-secret"})
	req := httptest.NewRequest(http.MethodGet, "/install.sh", nil)
	rr := httptest.NewRecorder()
	e.h.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusOK)
	body := rr.Body.String()
	for _, want := range []string{
		`$BASE_URL/skills/aidocs/SKILL.md`,
		`$HOME/.agents/skills/aidocs`,
		`$HOME/.claude/skills/aidocs`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("install script missing %q in:\n%s", want, body)
		}
	}
}

// TestInstallScriptSeedsCLIConfig locks in that the served installer points a
// fresh CLI at the serving instance (and never clobbers an existing config),
// since the CLI has no baked-in default server.
func TestInstallScriptSeedsCLIConfig(t *testing.T) {
	e := newConfiguredTestEnv(t, server.Config{Environment: "test", AppOrigin: "https://aidocs.example", SessionSecret: "test-secret"})
	req := httptest.NewRequest(http.MethodGet, "/install.sh", nil)
	rr := httptest.NewRecorder()
	e.h.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusOK)
	body := rr.Body.String()
	for _, want := range []string{
		`configure_default_context`,
		`config_path="$config_dir/config.json"`,
		`if [ -e "$config_path" ]; then`,
		`"server": "$BASE_URL",`,
		`"active_context": "$host",`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("install script missing %q in:\n%s", want, body)
		}
	}
}

func TestInstallScriptIsValidBash(t *testing.T) {
	e := newConfiguredTestEnv(t, server.Config{Environment: "test", AppOrigin: "https://aidocs.example", SessionSecret: "test-secret"})
	req := httptest.NewRequest(http.MethodGet, "/install.sh", nil)
	rr := httptest.NewRecorder()
	e.h.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusOK)
	body := rr.Body.String()
	cmd := exec.Command("bash", "-n")
	cmd.Stdin = strings.NewReader(body)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install script is not valid bash: %v\n%s\nscript:\n%s", err, out, body)
	}
}

func TestInstallScriptAddsInstallDirToPath(t *testing.T) {
	e := newConfiguredTestEnv(t, server.Config{Environment: "test", AppOrigin: "https://aidocs.example", SessionSecret: "test-secret"})
	req := httptest.NewRequest(http.MethodGet, "/install.sh", nil)
	rr := httptest.NewRecorder()
	e.h.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusOK)
	body := rr.Body.String()
	for _, want := range []string{
		`ensure_bin_dir_on_path`,
		`export PATH="$BIN_DIR:$PATH"`,
		`echo "# Added by aidocs installer"`,
		`echo "Added $BIN_DIR to PATH in $profile."`,
		`echo "$HOME/.zshrc"`,
		`echo "$HOME/.bashrc"`,
		`echo "$HOME/.bash_profile"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("install script missing %q in:\n%s", want, body)
		}
	}
}

func TestInstallScriptRequiresConfiguredAppOriginForNonLocalHost(t *testing.T) {
	e := newConfiguredTestEnv(t, server.Config{Environment: "test", SessionSecret: "test-secret"})
	req := httptest.NewRequest(http.MethodGet, "/install.sh", nil)
	req.Host = "aidocs.example"
	req.Header.Set("X-Forwarded-Host", "evil.example")
	rr := httptest.NewRecorder()
	e.h.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusInternalServerError)
	if strings.Contains(rr.Body.String(), "evil.example") || strings.Contains(rr.Body.String(), "aidocs.example") {
		t.Fatalf("misconfigured installer response reflected request host: %q", rr.Body.String())
	}
}

func TestDownloadSkillServesAidocsSkill(t *testing.T) {
	rr := do(t, newTestEnv(t).h, http.MethodGet, "/skills/aidocs/SKILL.md", "", nil)

	assertStatus(t, rr, http.StatusOK)
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/markdown") {
		t.Fatalf("Content-Type = %q, want text/markdown", ct)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "name: aidocs") || !strings.Contains(body, "aidocs docs create") {
		t.Fatalf("unexpected skill body: %q", body)
	}
}

func TestDownloadSkillRejectsUnknownSkill(t *testing.T) {
	rr := do(t, newTestEnv(t).h, http.MethodGet, "/skills/unknown/SKILL.md", "", nil)

	assertStatus(t, rr, http.StatusNotFound)
}
