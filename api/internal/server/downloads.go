package server

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
)

// BuildVersion and BuildCommit are set with -ldflags in release builds.
var (
	BuildVersion = "dev"
	BuildCommit  = ""
)

//go:embed download_dist/*
var downloadDist embed.FS

// Keep this file in sync with ../../../../skills/aidocs/SKILL.md. It is
// embedded into the server binary so self-hosted installs do not depend on
// GitHub access, npx, or a Node runtime to install the agent skill.
//
//go:embed embedded/skills/aidocs/SKILL.md
var aidocsSkillMD []byte

type cliAsset struct {
	OS     string `json:"os"`
	Arch   string `json:"arch"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256,omitempty"`
	Size   int    `json:"size,omitempty"`
}

// versionResponse is the payload of GET /version.json.
type versionResponse struct {
	Version        string     `json:"version"`
	Commit         string     `json:"commit"`
	Install        string     `json:"install"`
	InstallWindows string     `json:"install_windows,omitempty"`
	Assets         []cliAsset `json:"assets"`
}

var cliPlatforms = []struct{ os, arch string }{
	{"darwin", "arm64"},
	{"darwin", "amd64"},
	{"linux", "arm64"},
	{"linux", "amd64"},
	{"windows", "amd64"},
	{"windows", "arm64"},
}

// cliAssetName is the name used in the embedded filesystem (no extension).
func cliAssetName(goos, goarch string) string { return "aidocs-" + goos + "-" + goarch }

// cliDownloadFilename returns the filename for the Content-Disposition header.
// Windows binaries carry an .exe extension so browsers save them correctly.
func cliDownloadFilename(goos, goarch string) string {
	name := cliAssetName(goos, goarch)
	if goos == "windows" {
		name += ".exe"
	}
	return name
}

func embeddedCLIAsset(goos, goarch string) ([]byte, bool) {
	b, err := downloadDist.ReadFile(path.Join("download_dist", cliAssetName(goos, goarch)))
	return b, err == nil && len(b) > 0
}

func (h handlers) versionJSON(c *gin.Context) {
	assets := make([]cliAsset, 0, len(cliPlatforms))
	for _, p := range cliPlatforms {
		asset := cliAsset{OS: p.os, Arch: p.arch, URL: "/cli/" + p.os + "/" + p.arch}
		if b, ok := embeddedCLIAsset(p.os, p.arch); ok {
			sum := sha256.Sum256(b)
			asset.SHA256 = hex.EncodeToString(sum[:])
			asset.Size = len(b)
		}
		assets = append(assets, asset)
	}
	c.JSON(http.StatusOK, versionResponse{Version: releaseVersion(), Commit: releaseCommit(), Install: "/install.sh", InstallWindows: "/install.ps1", Assets: assets})
}

func (h handlers) downloadSkill(c *gin.Context) {
	name := strings.TrimSpace(c.Param("name"))
	if name != "aidocs" {
		c.JSON(http.StatusNotFound, errorResponse("not_found", "skill is not available", nil))
		return
	}
	c.Header("Content-Disposition", `attachment; filename="SKILL.md"`)
	c.Data(http.StatusOK, "text/markdown; charset=utf-8", aidocsSkillMD)
}

func (h handlers) downloadCLI(c *gin.Context) {
	goos, goarch := c.Param("os"), c.Param("arch")
	if !validCLIPlatform(goos, goarch) {
		c.JSON(http.StatusNotFound, errorResponse("not_found", "unsupported platform", nil))
		return
	}
	b, ok := embeddedCLIAsset(goos, goarch)
	if !ok {
		c.JSON(http.StatusNotFound, errorResponse("not_found", "cli binary is not available in this build", nil))
		return
	}
	name := cliDownloadFilename(goos, goarch)
	c.Header("Content-Disposition", `attachment; filename="`+name+`"`)
	c.Data(http.StatusOK, "application/octet-stream", b)
}

func validCLIPlatform(goos, goarch string) bool {
	for _, p := range cliPlatforms {
		if p.os == goos && p.arch == goarch {
			return true
		}
	}
	return false
}

func (h handlers) installScript(c *gin.Context) {
	base, ok := h.installBase(c)
	if !ok {
		c.Data(http.StatusInternalServerError, "text/plain; charset=utf-8", []byte("aidocs server is missing APP_ORIGIN; cannot generate installer\n"))
		return
	}
	c.Data(http.StatusOK, "text/x-shellscript; charset=utf-8", []byte(strings.ReplaceAll(installScriptTemplate, "__BASE_URL__", base)))
}

func (h handlers) installPowerShellScript(c *gin.Context) {
	base, ok := h.installBase(c)
	if !ok {
		c.Data(http.StatusInternalServerError, "text/plain; charset=utf-8", []byte("aidocs server is missing APP_ORIGIN; cannot generate installer\n"))
		return
	}
	c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(strings.ReplaceAll(installPowerShellScriptTemplate, "__BASE_URL__", base)))
}

func (h handlers) installBatScript(c *gin.Context) {
	base, ok := h.installBase(c)
	if !ok {
		c.Data(http.StatusInternalServerError, "text/plain; charset=utf-8", []byte("aidocs server is missing APP_ORIGIN; cannot generate installer\n"))
		return
	}
	c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(strings.ReplaceAll(installBatScriptTemplate, "__BASE_URL__", base)))
}

func (h handlers) installBase(c *gin.Context) (string, bool) {
	if base, ok := canonicalHTTPOrigin(h.deps.appOrigin); ok {
		return base, true
	}
	// Tests and local development may not configure APP_ORIGIN. Keep that path
	// usable, but do not derive public installer URLs from proxy-controlled
	// forwarding headers or arbitrary non-local Host values.
	if isLocalHost(c.Request.Host) {
		proto := "https"
		if c.Request.TLS == nil {
			proto = "http"
		}
		return proto + "://" + sanitizeHost(c.Request.Host), true
	}
	return "", false
}

func releaseVersion() string {
	if BuildVersion != "" {
		return BuildVersion
	}
	return "dev"
}

func releaseCommit() string {
	if BuildCommit != "" {
		return BuildCommit
	}
	return ""
}

const installScriptTemplate = `#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${AIDOCS_BASE_URL:-__BASE_URL__}"
INSTALL_MODE="user"
for arg in "$@"; do
  case "$arg" in
    --user) INSTALL_MODE="user" ;;
    --system) INSTALL_MODE="system" ;;
    *) echo "unknown option: $arg" >&2; exit 2 ;;
  esac
done

if [ -n "${AIDOCS_INSTALL_DIR:-}" ]; then
  BIN_DIR="$AIDOCS_INSTALL_DIR"
elif [ "$INSTALL_MODE" = "system" ]; then
  BIN_DIR="/usr/local/bin"
else
  BIN_DIR="$HOME/.local/bin"
fi
BIN_PATH="$BIN_DIR/aidocs"

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$os" in
  darwin|linux) ;;
  *) echo "unsupported OS: $os" >&2; exit 1 ;;
esac
case "$arch" in
  arm64|aarch64) arch="arm64" ;;
  x86_64|amd64) arch="amd64" ;;
  *) echo "unsupported arch: $arch" >&2; exit 1 ;;
esac

if command -v brew >/dev/null 2>&1 && brew list aidocs >/dev/null 2>&1; then
  echo "Removing Homebrew aidocs..."
  brew uninstall aidocs
fi

old_path="$(command -v aidocs 2>/dev/null || true)"
if [ -n "$old_path" ] && [ "$old_path" != "$BIN_PATH" ]; then
  old_dir="$(dirname "$old_path")"
  echo "Existing aidocs install found at $old_path." >&2
  echo "Remove it first, or install there with:" >&2
  echo "  curl -fsSL $BASE_URL/install.sh | AIDOCS_INSTALL_DIR=$old_dir bash" >&2
  echo "Target install path: $BIN_PATH" >&2
  exit 1
fi

bin_tmp="$(mktemp)"
skill_tmp="$(mktemp)"
trap 'rm -f "$bin_tmp" "$skill_tmp"' EXIT
url="$BASE_URL/cli/$os/$arch"
echo "Downloading aidocs from $url"
curl -fsSL "$url" -o "$bin_tmp"
chmod +x "$bin_tmp"

skill_url="$BASE_URL/skills/aidocs/SKILL.md"
echo "Downloading aidocs skill from $skill_url"
curl -fsSL "$skill_url" -o "$skill_tmp"

if [ ! -d "$BIN_DIR" ]; then
  if [ "$INSTALL_MODE" = "system" ]; then
    mkdir -p "$BIN_DIR" 2>/dev/null || sudo mkdir -p "$BIN_DIR"
  else
    mkdir -p "$BIN_DIR"
  fi
fi
if [ -w "$BIN_DIR" ]; then
  mv "$bin_tmp" "$BIN_PATH"
elif [ "$INSTALL_MODE" = "system" ]; then
  sudo mv "$bin_tmp" "$BIN_PATH"
else
  echo "install dir is not writable: $BIN_DIR" >&2
  echo "Try: curl -fsSL $BASE_URL/install.sh | bash -s -- --system" >&2
  echo "Or set AIDOCS_INSTALL_DIR to a writable directory." >&2
  exit 1
fi

echo "Installed $($BIN_PATH version 2>/dev/null || echo aidocs) at $BIN_PATH"

install_skill_dir() {
  skill_dir="$1"
  mkdir -p "$skill_dir"
  cp "$skill_tmp" "$skill_dir/SKILL.md"
}

install_skill_dir "$HOME/.agents/skills/aidocs"
install_skill_dir "$HOME/.claude/skills/aidocs"
echo "Installed aidocs skill at:"
echo "  $HOME/.agents/skills/aidocs/SKILL.md"
echo "  $HOME/.claude/skills/aidocs/SKILL.md"

# Point a fresh CLI at the server this installer came from, so
# "curl <server>/install.sh | bash" ends with aidocs already configured.
# An existing config is never touched: it may hold other contexts/credentials.
configure_default_context() {
  config_dir="${XDG_CONFIG_HOME:-$HOME/.config}/aidocs"
  config_path="$config_dir/config.json"
  if [ -e "$config_path" ]; then
    echo "Keeping existing aidocs config at $config_path"
    return
  fi
  host="${BASE_URL#*://}"
  host="${host%%/*}"
  mkdir -p "$config_dir"
  chmod 700 "$config_dir"
  cat > "$config_path" <<EOF
{
  "active_context": "$host",
  "contexts": {
    "$host": {
      "server": "$BASE_URL",
      "pulled": {}
    }
  }
}
EOF
  chmod 600 "$config_path"
  echo "Configured aidocs to use $BASE_URL (run 'aidocs auth login' to sign in)."
}

configure_default_context

profile_for_current_shell() {
  shell_name="$(basename "${SHELL:-}")"
  case "$shell_name" in
    zsh) echo "$HOME/.zshrc" ;;
    bash)
      if [ "$os" = "darwin" ]; then
        echo "$HOME/.bash_profile"
      else
        echo "$HOME/.bashrc"
      fi
      ;;
    fish) echo "$HOME/.config/fish/config.fish" ;;
    *) echo "$HOME/.profile" ;;
  esac
}

ensure_bin_dir_on_path() {
  case ":$PATH:" in
    *":$BIN_DIR:"*) return ;;
  esac

  export PATH="$BIN_DIR:$PATH"

  profile="$(profile_for_current_shell)"
  shell_name="$(basename "${SHELL:-}")"
  path_line="export PATH=\"$BIN_DIR:\$PATH\""
  if [ "$shell_name" = "fish" ]; then
    mkdir -p "$(dirname "$profile")" 2>/dev/null || {
      echo "Add $BIN_DIR to PATH to use this install."
      return
    }
    path_line="set -gx PATH \"$BIN_DIR\" \$PATH"
  fi

  touch "$profile" 2>/dev/null || {
    echo "Add $BIN_DIR to PATH to use this install."
    return
  }
  if grep -F "$BIN_DIR" "$profile" >/dev/null 2>&1; then
    echo "Exported $BIN_DIR into PATH for this install. It already appears in $profile."
    return
  fi
  {
    echo ""
    echo "# Added by aidocs installer"
    echo "$path_line"
  } >> "$profile" || {
    echo "Add $BIN_DIR to PATH to use this install."
    return
  }
  echo "Added $BIN_DIR to PATH in $profile."
  echo "New terminals will pick this up automatically."
}

ensure_bin_dir_on_path

active_path="$(command -v aidocs 2>/dev/null || true)"
if [ -n "$active_path" ] && [ "$active_path" != "$BIN_PATH" ]; then
  echo "Warning: 'aidocs' currently resolves to $active_path."
  echo "Move $BIN_DIR earlier in PATH or remove the older install."
fi
`

// installPowerShellScriptTemplate is the Windows equivalent of
// installScriptTemplate. It is served at /install.ps1 and can be run with:
//
//	irm https://aidocs.example/install.ps1 | iex
//
// It mirrors the bash installer: downloads the CLI binary and skill, installs
// them, seeds a default CLI context, and adds the install directory to the
// user PATH.
const installPowerShellScriptTemplate = `# aidocs installer for Windows (PowerShell)
# Usage:
#   irm __BASE_URL__/install.ps1 | iex
#   # or download and run:
#   powershell -ExecutionPolicy Bypass -File install.ps1

$ErrorActionPreference = "Stop"

$BaseUrl = $env:AIDOCS_BASE_URL
if (-not $BaseUrl) { $BaseUrl = "__BASE_URL__" }

# --- Architecture detection ---
$arch = $env:PROCESSOR_ARCHITECTURE
if ($arch -eq "AMD64") { $arch = "amd64" }
elseif ($arch -eq "ARM64") { $arch = "arm64" }
else {
  Write-Error "Unsupported architecture: $arch (expected amd64 or arm64)"
  exit 1
}

# --- Install directory ---
$InstallDir = $env:AIDOCS_INSTALL_DIR
if (-not $InstallDir) { $InstallDir = Join-Path $env:USERPROFILE ".local\bin" }
$BinPath = Join-Path $InstallDir "aidocs.exe"

# --- Download CLI binary ---
$CliUrl = "$BaseUrl/cli/windows/$arch"
Write-Host "Downloading aidocs from $CliUrl"
try {
  Invoke-WebRequest -Uri $CliUrl -OutFile $BinPath -UseBasicParsing
} catch {
  Write-Error "Failed to download aidocs CLI: $_"
  exit 1
}

# --- Download skill ---
$SkillUrl = "$BaseUrl/skills/aidocs/SKILL.md"
Write-Host "Downloading aidocs skill from $SkillUrl"
$SkillTemp = Join-Path $env:TEMP "aidocs_skill_$([System.IO.Path]::GetRandomFileName()).md"
try {
  Invoke-WebRequest -Uri $SkillUrl -OutFile $SkillTemp -UseBasicParsing
} catch {
  Write-Error "Failed to download aidocs skill: $_"
  exit 1
}

# --- Install skill ---
function Install-Skill($SkillDir) {
  New-Item -ItemType Directory -Force -Path $SkillDir | Out-Null
  Copy-Item $SkillTemp -Destination (Join-Path $SkillDir "SKILL.md") -Force
}

$SkillDirs = @(
  (Join-Path $env:USERPROFILE ".agents\skills\aidocs"),
  (Join-Path $env:USERPROFILE ".claude\skills\aidocs")
)
foreach ($Dir in $SkillDirs) { Install-Skill $Dir }
Write-Host "Installed aidocs skill at:"
foreach ($Dir in $SkillDirs) { Write-Host "  $(Join-Path $Dir 'SKILL.md')" }

Remove-Item $SkillTemp -Force -ErrorAction SilentlyContinue

# --- Seed default CLI context ---
$ConfigDir = Join-Path $env:USERPROFILE ".config\aidocs"
$ConfigPath = Join-Path $ConfigDir "config.json"
if (Test-Path $ConfigPath) {
  Write-Host "Keeping existing aidocs config at $ConfigPath"
} else {
  $Host_ = [System.Uri]::new($BaseUrl).Host
  New-Item -ItemType Directory -Force -Path $ConfigDir | Out-Null
  $Config = @{
    active_context = $Host_
    contexts = @{
      $Host_ = @{
        server = $BaseUrl
        pulled = @{}
      }
    }
  } | ConvertTo-Json -Depth 10
  Set-Content -Path $ConfigPath -Value $Config -Encoding UTF8
  Write-Host "Configured aidocs to use $BaseUrl (run 'aidocs auth login' to sign in)."
}

# --- Add install directory to user PATH ---
$UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($UserPath -notlike "*$InstallDir*") {
  [Environment]::SetEnvironmentVariable("Path", "$InstallDir;$UserPath", "User")
  Write-Host "Added $InstallDir to user PATH. New terminals will pick this up automatically."
} else {
  Write-Host "$InstallDir is already on PATH."
}

# --- Verify ---
if (Test-Path $BinPath) {
  Write-Host "Installed aidocs at $BinPath"
} else {
  Write-Error "Installation failed: $BinPath not found"
  exit 1
}
`

// installBatScriptTemplate is a thin batch wrapper around the PowerShell
// installer. It is served at /install.bat and can be run by double-clicking
// or from cmd.exe:
//
//	curl -fsSL __BASE_URL__/install.bat -o install.bat && install.bat
const installBatScriptTemplate = `@echo off
REM aidocs installer for Windows (batch wrapper)
REM Downloads and runs the PowerShell installer.
REM
REM Usage:
REM   curl -fsSL __BASE_URL__/install.bat -o install.bat && install.bat

setlocal
set "BASE_URL=__BASE_URL__"

where powershell >nul 2>nul
if errorlevel 1 (
    echo PowerShell is required but was not found. >&2
    exit /b 1
)

powershell -NoProfile -ExecutionPolicy Bypass -Command "irm '%BASE_URL%/install.ps1' | iex"
if errorlevel 1 (
    echo aidocs installation failed. >&2
    exit /b 1
)

echo.
echo aidocs installed successfully. Open a new terminal and run: aidocs version
endlocal
`
