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

type cliAsset struct {
	OS     string `json:"os"`
	Arch   string `json:"arch"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256,omitempty"`
	Size   int    `json:"size,omitempty"`
}

var cliPlatforms = []struct{ os, arch string }{
	{"darwin", "arm64"},
	{"darwin", "amd64"},
	{"linux", "arm64"},
	{"linux", "amd64"},
}

func cliAssetName(goos, goarch string) string { return "aidocs-" + goos + "-" + goarch }

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
	c.JSON(http.StatusOK, gin.H{"version": releaseVersion(), "commit": releaseCommit(), "install": "/install.sh", "assets": assets})
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
	name := cliAssetName(goos, goarch)
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
	base := externalBase(c)
	c.Data(http.StatusOK, "text/x-shellscript; charset=utf-8", []byte(strings.ReplaceAll(installScriptTemplate, "__BASE_URL__", base)))
}

func externalBase(c *gin.Context) string {
	proto := c.GetHeader("X-Forwarded-Proto")
	if proto == "" {
		proto = "https"
		if c.Request.TLS == nil && strings.HasPrefix(c.Request.Host, "localhost") {
			proto = "http"
		}
	}
	host := c.GetHeader("X-Forwarded-Host")
	if host == "" {
		host = c.Request.Host
	}
	return proto + "://" + host
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
  echo "Existing aidocs install found at $old_path." >&2
  echo "Remove it first, or install to that location instead." >&2
  echo "Target install path: $BIN_PATH" >&2
  exit 1
fi

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
url="$BASE_URL/cli/$os/$arch"
echo "Downloading aidocs from $url"
curl -fsSL "$url" -o "$tmp"
chmod +x "$tmp"

if [ ! -d "$BIN_DIR" ]; then
  if [ "$INSTALL_MODE" = "system" ]; then
    mkdir -p "$BIN_DIR" 2>/dev/null || sudo mkdir -p "$BIN_DIR"
  else
    mkdir -p "$BIN_DIR"
  fi
fi
if [ -w "$BIN_DIR" ]; then
  mv "$tmp" "$BIN_PATH"
elif [ "$INSTALL_MODE" = "system" ]; then
  sudo mv "$tmp" "$BIN_PATH"
else
  echo "install dir is not writable: $BIN_DIR" >&2
  echo "Try: curl -fsSL $BASE_URL/install.sh | bash -s -- --system" >&2
  echo "Or set AIDOCS_INSTALL_DIR to a writable directory." >&2
  exit 1
fi

echo "Installed $($BIN_PATH version 2>/dev/null || echo aidocs) at $BIN_PATH"

case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *) echo "Add $BIN_DIR to PATH to use this install." ;;
esac

active_path="$(command -v aidocs 2>/dev/null || true)"
if [ -n "$active_path" ] && [ "$active_path" != "$BIN_PATH" ]; then
  echo "Warning: 'aidocs' currently resolves to $active_path."
  echo "Move $BIN_DIR earlier in PATH or remove the older install."
fi
`
