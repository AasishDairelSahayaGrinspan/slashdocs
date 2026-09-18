package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// serverVersion is the payload of <server>/version.json. The server also
// advertises an "install" URL; it is deliberately not decoded, because the
// reinstall hint is always built from the trusted server's own base.
type serverVersion struct {
	Version string        `json:"version"`
	Commit  string        `json:"commit"`
	Assets  []serverAsset `json:"assets"`
}

type serverAsset struct {
	OS     string `json:"os"`
	Arch   string `json:"arch"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int    `json:"size"`
}

// untrustedUpdateSourceError reports a server that may not update this install.
type untrustedUpdateSourceError struct{ server string }

func (e untrustedUpdateSourceError) Error() string {
	return fmt.Sprintf("refusing to update from %s: the CLI only updates itself and its agent skill from a server you have logged in to; run `aidocs auth login %s` first", e.server, e.server)
}

// trustedUpdateSource reports whether server — the one this invocation resolved
// — is a server the user explicitly logged in to: a stored context, matched
// after the same normalization the config uses, that holds a stored credential.
//
// Self-update and the agent skill sync both rewrite files the user never named
// on the command line (the CLI binary itself, and the SKILL.md files that
// coding agents auto-load), so they must not follow a one-off `--server`,
// `AIDOCS_SERVER`, or a bare `--token`. A context that install.sh wrote but
// nobody has logged in to yet is not enough either: `aidocs auth login` is the
// deliberate, interactive act that makes a server trusted.
func trustedUpdateSource(server string) bool {
	cfg, err := loadConfig()
	if err != nil {
		return false
	}
	srv := normalizeServer(server)
	cx := cfg.Contexts[ctxName(srv)]
	if cx == nil || len(cx.Credential) == 0 {
		return false
	}
	return normalizeServer(cx.Server) == srv
}

// ensureTrustedUpdateSource guards every entry point that can rewrite the
// binary or the skill files. There is deliberately no override flag: a flag
// would be exactly what an agent following instructions would be told to pass.
func ensureTrustedUpdateSource(c *Client) error {
	if trustedUpdateSource(c.Base) {
		return nil
	}
	return untrustedUpdateSourceError{server: c.Base}
}

func maybeNudgeUpdate(g *globals, c *Client) { maybeAutoUpdate(g, c) }

func maybeAutoUpdate(g *globals, c *Client) {
	if Version == "" || os.Getenv("AIDOCS_DISABLE_AUTO_UPDATE") != "" {
		return
	}
	// Housekeeping after a command must stay silent and must never let the
	// server this command happened to target update the install; only a
	// logged-in server gets asked at all.
	if err := ensureTrustedUpdateSource(c); err != nil {
		if g.verbose {
			fmt.Fprintf(os.Stderr, "skipping the aidocs update check: %s is not a logged-in server\n", c.Base)
		}
		return
	}
	// This check is best-effort housekeeping after a command: use the
	// short-timeout client and bound the whole operation so a slow server
	// never noticeably delays the CLI. Failures stay silent unless --verbose.
	c = &Client{Base: c.Base, Token: c.Token, HTTP: updateHTTPClient}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if g.verbose {
		fmt.Fprintf(os.Stderr, "checking for aidocs update at %s/version.json\n", c.Base)
	}
	v, binaryChanged, skillRefreshed, err := updateCLI(ctx, c, false)
	if g.verbose && err == nil && !binaryChanged && !skillRefreshed {
		fmt.Fprintf(os.Stderr, "aidocs is up to date or no release is available (local=%s, server=%s).\n", Version, v.Version)
	}
	if err != nil {
		printManualUpdate(c, v, err, g.verbose)
		return
	}
	if binaryChanged {
		fmt.Fprintf(os.Stderr, "aidocs updated to %s; the next command will use the new CLI.\n", v.Version)
	} else if skillRefreshed && g.verbose {
		fmt.Fprintln(os.Stderr, "aidocs skill refreshed.")
	}
}

// updateCLI syncs the agent skill and the binary from a trusted server.
//
// Versions are opaque build identifiers (commit SHAs), so they cannot be
// ordered: "the server advertises a different build" is the only available
// trigger. With the source pinned to a server the user logged in to, that is a
// documented property rather than an accident — the install tracks that
// server's current build, and follows it back down if the server is rolled
// back.
func updateCLI(ctx context.Context, c *Client, force bool) (serverVersion, bool, bool, error) {
	if err := ensureTrustedUpdateSource(c); err != nil {
		return serverVersion{}, false, false, err
	}
	skillRefreshed, skillErr := installBundledSkill(ctx, c)
	v, ok := fetchServerVersion(ctx, c)
	if skillErr != nil {
		return v, false, false, skillErr
	}
	if !ok || v.Version == "" {
		return v, false, skillRefreshed, nil
	}
	if !force && v.Version == Version {
		return v, false, skillRefreshed, nil
	}
	asset, ok := v.asset(runtime.GOOS, runtime.GOARCH)
	if !ok || strings.TrimSpace(asset.URL) == "" {
		return v, false, skillRefreshed, fmt.Errorf("no CLI asset for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	if err := installCLIUpdate(ctx, c, asset); err != nil {
		return v, false, skillRefreshed, err
	}
	return v, true, skillRefreshed, nil
}

func printManualUpdate(c *Client, v serverVersion, err error, verbose bool) {
	if verbose {
		fmt.Fprintf(os.Stderr, "aidocs auto-update failed: %v\n", err)
	}
	if v.Version != "" {
		fmt.Fprintf(os.Stderr, "aidocs %s is available, but auto-update failed. Update with: aidocs update\n", v.Version)
		// Always the trusted server's own installer: the version payload is
		// not allowed to point the user at another host.
		fmt.Fprintf(os.Stderr, "Or reinstall with: curl -fsSL %s/install.sh | bash\n", c.Base)
	}
}

func fetchServerVersion(ctx context.Context, c *Client) (serverVersion, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+"/version.json", nil)
	if err != nil {
		return serverVersion{}, false
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return serverVersion{}, false
	}
	defer func() {
		_ = res.Body.Close()
	}()
	if res.StatusCode != http.StatusOK {
		return serverVersion{}, false
	}
	var v serverVersion
	if err := json.NewDecoder(res.Body).Decode(&v); err != nil {
		return serverVersion{}, false
	}
	return v, true
}

func (v serverVersion) asset(goos, goarch string) (serverAsset, bool) {
	for _, a := range v.Assets {
		if a.OS == goos && a.Arch == goarch {
			return a, true
		}
	}
	return serverAsset{}, false
}

func installBundledSkill(ctx context.Context, c *Client) (bool, error) {
	_, changed, err := installBundledSkillFromServer(ctx, c)
	return changed, err
}

// installBundledSkillFromServer writes the trusted server's SKILL.md into the
// agent skill directories. It returns the installed paths and whether any of
// them changed on disk.
func installBundledSkillFromServer(ctx context.Context, c *Client) ([]string, bool, error) {
	if err := ensureTrustedUpdateSource(c); err != nil {
		return nil, false, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+"/skills/aidocs/SKILL.md", nil)
	if err != nil {
		return nil, false, err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer func() {
		_ = res.Body.Close()
	}()
	if res.StatusCode == http.StatusNotFound {
		// Older/self-hosted servers may not expose the skill endpoint yet.
		return nil, false, nil
	}
	if res.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("skill download returned HTTP %d", res.StatusCode)
	}
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, false, err
	}
	return installBundledSkillBytes(b)
}

func installBundledSkillBytes(b []byte) ([]string, bool, error) {
	if len(strings.TrimSpace(string(b))) == 0 {
		return nil, false, fmt.Errorf("skill download was empty")
	}
	dirs := bundledSkillDirs()
	if len(dirs) == 0 {
		return nil, false, fmt.Errorf("could not determine home directory for skill install")
	}
	installed := make([]string, 0, len(dirs))
	changed := false
	for _, dir := range dirs {
		p := filepath.Join(dir, "SKILL.md")
		//nolint:gosec // G304: p is the fixed agent skill path under the user's home directory.
		if current, err := os.ReadFile(p); err == nil && bytes.Equal(current, b) {
			// Identical content: leave the file, and its mtime, alone.
			installed = append(installed, p)
			continue
		}
		//nolint:gosec // G301: agent skill directories contain public documentation and are intentionally traversable.
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, changed, err
		}
		//nolint:gosec // G306: bundled SKILL.md is public documentation, not user data or credentials.
		if err := os.WriteFile(p, b, 0o644); err != nil {
			return nil, changed, err
		}
		installed = append(installed, p)
		changed = true
	}
	return installed, changed, nil
}

func bundledSkillDirs() []string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return nil
	}
	return []string{
		filepath.Join(home, ".agents", "skills", "aidocs"),
		filepath.Join(home, ".claude", "skills", "aidocs"),
	}
}

// sameOriginURL resolves an advertised download URL against the trusted base.
// A relative path is joined onto the base; an absolute URL is accepted only
// when its scheme, host, and port are identical to the base's, so a trusted
// server cannot hand the download off to another origin.
func sameOriginURL(base, raw string) (string, error) {
	b, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("invalid server URL %q: %w", base, err)
	}
	u, err := b.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("invalid download URL %q: %w", raw, err)
	}
	if u.Scheme != b.Scheme || u.Host != b.Host {
		return "", fmt.Errorf("download URL %s is not on the same origin as %s", raw, base)
	}
	return u.String(), nil
}

// assetSHA256 returns the asset's checksum. It is mandatory: without one, a
// download cannot be told apart from anything else the connection returns, so
// an asset that advertises no usable sha256 is refused before it is fetched.
func assetSHA256(asset serverAsset) (string, error) {
	sum := strings.TrimSpace(asset.SHA256)
	if b, err := hex.DecodeString(sum); err != nil || len(b) != sha256.Size {
		return "", fmt.Errorf("CLI asset for %s/%s has no usable sha256", asset.OS, asset.Arch)
	}
	return sum, nil
}

// executablePath resolves the binary a self-update replaces. It is a variable
// so tests can point the update at a throwaway file instead of the test binary.
var executablePath = os.Executable

func installCLIUpdate(ctx context.Context, c *Client, asset serverAsset) error {
	want, err := assetSHA256(asset)
	if err != nil {
		return err
	}
	assetURL, err := sameOriginURL(c.Base, asset.URL)
	if err != nil {
		return err
	}
	exe, err := executablePath()
	if err != nil {
		return err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_ = res.Body.Close()
	}()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned HTTP %d", res.StatusCode)
	}
	dir := filepath.Dir(exe)
	tmp, err := os.CreateTemp(dir, ".aidocs-update-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()
	h := sha256.New()
	written, err := io.Copy(io.MultiWriter(tmp, h), res.Body)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if asset.Size > 0 && int(written) != asset.Size {
		return fmt.Errorf("download size mismatch: got %d, want %d", written, asset.Size)
	}
	if got := hex.EncodeToString(h.Sum(nil)); !strings.EqualFold(got, want) {
		return fmt.Errorf("download sha256 mismatch")
	}
	//nolint:gosec // G302: the checksum-verified CLI follows the installer's conventional world-executable mode.
	if err := os.Chmod(tmpName, 0o755); err != nil {
		return err
	}
	return os.Rename(tmpName, exe)
}
