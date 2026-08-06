package server_test

import (
	"encoding/json"
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

func TestInstallPowerShellScriptUsesConfiguredAppOrigin(t *testing.T) {
	e := newConfiguredTestEnv(t, server.Config{Environment: "test", AppOrigin: "https://aidocs.example", SessionSecret: "test-secret"})
	req := httptest.NewRequest(http.MethodGet, "/install.ps1", nil)
	req.Host = "aidocs.example"
	req.Header.Set("X-Forwarded-Host", "evil.example")
	rr := httptest.NewRecorder()
	e.h.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusOK)
	body := rr.Body.String()
	if strings.Contains(body, "evil.example") {
		t.Fatalf("PowerShell install script reflected attacker-controlled forwarded host: %q", body)
	}
	if !strings.Contains(body, `$BaseUrl = "__BASE_URL__"`) && !strings.Contains(body, `$BaseUrl = "https://aidocs.example"`) {
		t.Fatalf("PowerShell install script did not use configured app origin: %q", body)
	}
}

func TestInstallPowerShellScriptInstallsBundledSkill(t *testing.T) {
	e := newConfiguredTestEnv(t, server.Config{Environment: "test", AppOrigin: "https://aidocs.example", SessionSecret: "test-secret"})
	req := httptest.NewRequest(http.MethodGet, "/install.ps1", nil)
	rr := httptest.NewRecorder()
	e.h.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusOK)
	body := rr.Body.String()
	for _, want := range []string{
		`$BaseUrl/skills/aidocs/SKILL.md`,
		`.agents\skills\aidocs`,
		`.claude\skills\aidocs`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("PowerShell install script missing %q in:\n%s", want, body)
		}
	}
}

func TestInstallPowerShellScriptSeedsCLIConfig(t *testing.T) {
	e := newConfiguredTestEnv(t, server.Config{Environment: "test", AppOrigin: "https://aidocs.example", SessionSecret: "test-secret"})
	req := httptest.NewRequest(http.MethodGet, "/install.ps1", nil)
	rr := httptest.NewRecorder()
	e.h.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusOK)
	body := rr.Body.String()
	for _, want := range []string{
		`config.json`,
		`active_context`,
		`"server"`,
		`Test-Path $ConfigPath`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("PowerShell install script missing %q in:\n%s", want, body)
		}
	}
}

func TestInstallPowerShellScriptDownloadsWindowsBinary(t *testing.T) {
	e := newConfiguredTestEnv(t, server.Config{Environment: "test", AppOrigin: "https://aidocs.example", SessionSecret: "test-secret"})
	req := httptest.NewRequest(http.MethodGet, "/install.ps1", nil)
	rr := httptest.NewRecorder()
	e.h.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusOK)
	body := rr.Body.String()
	for _, want := range []string{
		`/cli/windows/`,
		`aidocs.exe`,
		`Invoke-WebRequest`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("PowerShell install script missing %q in:\n%s", want, body)
		}
	}
}

func TestInstallPowerShellScriptAddsPath(t *testing.T) {
	e := newConfiguredTestEnv(t, server.Config{Environment: "test", AppOrigin: "https://aidocs.example", SessionSecret: "test-secret"})
	req := httptest.NewRequest(http.MethodGet, "/install.ps1", nil)
	rr := httptest.NewRecorder()
	e.h.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusOK)
	body := rr.Body.String()
	for _, want := range []string{
		`SetEnvironmentVariable`,
		`"Path"`,
		`"User"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("PowerShell install script missing %q in:\n%s", want, body)
		}
	}
}

func TestInstallBatScriptUsesConfiguredAppOrigin(t *testing.T) {
	e := newConfiguredTestEnv(t, server.Config{Environment: "test", AppOrigin: "https://aidocs.example", SessionSecret: "test-secret"})
	req := httptest.NewRequest(http.MethodGet, "/install.bat", nil)
	rr := httptest.NewRecorder()
	e.h.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusOK)
	body := rr.Body.String()
	if strings.Contains(body, "evil.example") {
		t.Fatalf("batch install script reflected attacker-controlled forwarded host: %q", body)
	}
	if !strings.Contains(body, `set "BASE_URL=https://aidocs.example"`) {
		t.Fatalf("batch install script did not use configured app origin: %q", body)
	}
}

func TestInstallBatScriptInvokesPowerShell(t *testing.T) {
	e := newConfiguredTestEnv(t, server.Config{Environment: "test", AppOrigin: "https://aidocs.example", SessionSecret: "test-secret"})
	req := httptest.NewRequest(http.MethodGet, "/install.bat", nil)
	rr := httptest.NewRecorder()
	e.h.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusOK)
	body := rr.Body.String()
	for _, want := range []string{
		`powershell`,
		`install.ps1`,
		`irm`,
		`iex`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("batch install script missing %q in:\n%s", want, body)
		}
	}
}

func TestInstallPowerShellScriptRequiresConfiguredAppOriginForNonLocalHost(t *testing.T) {
	e := newConfiguredTestEnv(t, server.Config{Environment: "test", SessionSecret: "test-secret"})
	req := httptest.NewRequest(http.MethodGet, "/install.ps1", nil)
	req.Host = "aidocs.example"
	req.Header.Set("X-Forwarded-Host", "evil.example")
	rr := httptest.NewRecorder()
	e.h.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusInternalServerError)
	if strings.Contains(rr.Body.String(), "evil.example") || strings.Contains(rr.Body.String(), "aidocs.example") {
		t.Fatalf("misconfigured installer response reflected request host: %q", rr.Body.String())
	}
}

func TestInstallBatScriptRequiresConfiguredAppOriginForNonLocalHost(t *testing.T) {
	e := newConfiguredTestEnv(t, server.Config{Environment: "test", SessionSecret: "test-secret"})
	req := httptest.NewRequest(http.MethodGet, "/install.bat", nil)
	req.Host = "aidocs.example"
	req.Header.Set("X-Forwarded-Host", "evil.example")
	rr := httptest.NewRecorder()
	e.h.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusInternalServerError)
	if strings.Contains(rr.Body.String(), "evil.example") || strings.Contains(rr.Body.String(), "aidocs.example") {
		t.Fatalf("misconfigured installer response reflected request host: %q", rr.Body.String())
	}
}

func TestVersionJSONIncludesWindowsInstallURL(t *testing.T) {
	e := newConfiguredTestEnv(t, server.Config{Environment: "test", AppOrigin: "https://aidocs.example", SessionSecret: "test-secret"})
	req := httptest.NewRequest(http.MethodGet, "/version.json", nil)
	rr := httptest.NewRecorder()
	e.h.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusOK)
	var v struct {
		Install        string `json:"install"`
		InstallWindows string `json:"install_windows"`
		Assets         []struct {
			OS   string `json:"os"`
			Arch string `json:"arch"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&v); err != nil {
		t.Fatalf("decode version.json: %v", err)
	}
	if v.Install != "/install.sh" {
		t.Fatalf("install = %q, want /install.sh", v.Install)
	}
	if v.InstallWindows != "/install.ps1" {
		t.Fatalf("install_windows = %q, want /install.ps1", v.InstallWindows)
	}
	var hasWindows bool
	for _, a := range v.Assets {
		if a.OS == "windows" {
			hasWindows = true
			break
		}
	}
	if !hasWindows {
		t.Fatal("version.json assets missing windows platform")
	}
}
