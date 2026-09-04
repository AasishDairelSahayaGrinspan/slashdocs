package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type serverVersion struct {
	Version string        `json:"version"`
	Commit  string        `json:"commit"`
	Install string        `json:"install"`
	Assets  []serverAsset `json:"assets"`
}

type serverAsset struct {
	OS     string `json:"os"`
	Arch   string `json:"arch"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int    `json:"size"`
}

func maybeNudgeUpdate(g *globals, c *Client) { maybeAutoUpdate(g, c) }

func maybeAutoUpdate(g *globals, c *Client) {
	if Version == "" || os.Getenv("AIDOCS_DISABLE_AUTO_UPDATE") != "" {
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

func updateCLI(ctx context.Context, c *Client, force bool) (serverVersion, bool, bool, error) {
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
	install := strings.TrimSpace(v.Install)
	if install == "" {
		install = "/install.sh"
	}
	if strings.HasPrefix(install, "/") {
		install = c.Base + install
	}
	if v.Version != "" {
		fmt.Fprintf(os.Stderr, "aidocs %s is available, but auto-update failed. Update with: aidocs update\n", v.Version)
		fmt.Fprintf(os.Stderr, "Or reinstall with: curl -fsSL %s | bash\n", install)
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
	paths, err := installBundledSkillFromServer(ctx, c)
	return len(paths) > 0, err
}

func installBundledSkillFromServer(ctx context.Context, c *Client) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+"/skills/aidocs/SKILL.md", nil)
	if err != nil {
		return nil, err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = res.Body.Close()
	}()
	if res.StatusCode == http.StatusNotFound {
		// Older/self-hosted servers may not expose the skill endpoint yet.
		return nil, nil
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("skill download returned HTTP %d", res.StatusCode)
	}
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	return installBundledSkillBytes(b)
}

func installBundledSkillBytes(b []byte) ([]string, error) {
	if len(strings.TrimSpace(string(b))) == 0 {
		return nil, fmt.Errorf("skill download was empty")
	}
	dirs := bundledSkillDirs()
	if len(dirs) == 0 {
		return nil, fmt.Errorf("could not determine home directory for skill install")
	}
	installed := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		//nolint:gosec // G301: agent skill directories contain public documentation and are intentionally traversable.
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
		p := filepath.Join(dir, "SKILL.md")
		//nolint:gosec // G306: bundled SKILL.md is public documentation, not user data or credentials.
		if err := os.WriteFile(p, b, 0o644); err != nil {
			return nil, err
		}
		installed = append(installed, p)
	}
	return installed, nil
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

func installCLIUpdate(ctx context.Context, c *Client, asset serverAsset) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return err
	}
	url := strings.TrimSpace(asset.URL)
	if strings.HasPrefix(url, "/") {
		url = c.Base + url
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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
	if asset.SHA256 != "" {
		got := hex.EncodeToString(h.Sum(nil))
		if !strings.EqualFold(got, asset.SHA256) {
			return fmt.Errorf("download sha256 mismatch")
		}
	}
	//nolint:gosec // G302: the checksum-verified CLI follows the installer's conventional world-executable mode.
	if err := os.Chmod(tmpName, 0o755); err != nil {
		return err
	}
	return os.Rename(tmpName, exe)
}
