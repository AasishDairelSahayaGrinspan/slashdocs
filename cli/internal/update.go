package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

type serverVersion struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Install string `json:"install"`
}

func maybeNudgeUpdate(g *globals, c *Client) {
	if g.quiet || g.json || Version == "" || Version == "dev" || os.Getenv("AIDOCS_DISABLE_UPDATE_CHECK") != "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+"/version.json", nil)
	if err != nil {
		return
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return
	}
	var v serverVersion
	if err := json.NewDecoder(res.Body).Decode(&v); err != nil || v.Version == "" || v.Version == Version || v.Version == "dev" {
		return
	}
	install := strings.TrimSpace(v.Install)
	if install == "" {
		install = "/install.sh"
	}
	if strings.HasPrefix(install, "/") {
		install = c.Base + install
	}
	fmt.Fprintf(os.Stderr, "aidocs %s is available; update with: curl -fsSL %s | bash\n", v.Version, install)
}
