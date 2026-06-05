package internal

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
	if Version == "" || Version == "dev" || os.Getenv("AIDOCS_DISABLE_AUTO_UPDATE") != "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if g.verbose {
		fmt.Fprintf(os.Stderr, "checking for aidocs update at %s/version.json\n", c.Base)
	}
	v, changed, err := updateCLI(ctx, c, false)
	if g.verbose && err == nil && !changed {
		fmt.Fprintf(os.Stderr, "aidocs is up to date or no release is available (local=%s, server=%s).\n", Version, v.Version)
	}
	if err != nil {
		printManualUpdate(c, v, err, g.verbose)
		return
	}
	if changed {
		fmt.Fprintf(os.Stderr, "aidocs updated to %s; the next command will use the new CLI.\n", v.Version)
	}
}

func updateCLI(ctx context.Context, c *Client, force bool) (serverVersion, bool, error) {
	v, ok := fetchServerVersion(ctx, c)
	if !ok || v.Version == "" || v.Version == "dev" {
		return v, false, nil
	}
	if !force && v.Version == Version {
		return v, false, nil
	}
	asset, ok := v.asset(runtime.GOOS, runtime.GOARCH)
	if !ok || strings.TrimSpace(asset.URL) == "" {
		return v, false, fmt.Errorf("no CLI asset for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	if err := installCLIUpdate(ctx, c, asset); err != nil {
		return v, false, err
	}
	return v, true, nil
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
	defer res.Body.Close()
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
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned HTTP %d", res.StatusCode)
	}
	dir := filepath.Dir(exe)
	tmp, err := os.CreateTemp(dir, ".aidocs-update-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
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
	if err := os.Chmod(tmpName, 0o755); err != nil {
		return err
	}
	return os.Rename(tmpName, exe)
}
