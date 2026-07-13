package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

// configCmd reports the connected server's deployment type and the label it
// uses for the "everyone" grant, so an agent knows the scope/wording of the
// context it is talking to.
func configCmd(g *globals, out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Show the connected server's deployment configuration",
		Args:  exactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(g, out, func(c *Client) ([]byte, error) {
				return c.do("GET", "/v1/config", nil, "")
			})
		},
	}
}

// everyoneLabelFor fetches /v1/config to resolve the human label for the
// "everyone" grant on the connected server. It falls back to a generic phrase
// if the server is unreachable or pre-dates the endpoint, so sharing still
// reports something sensible.
func everyoneLabelFor(g *globals) string {
	cfg := serverConfigFor(g)
	if cfg.EveryoneLabel == "" {
		return "everyone on this server"
	}
	return cfg.EveryoneLabel
}

type serverConfig struct {
	OrgName       string `json:"org_name"`
	EveryoneLabel string `json:"everyone_label"`
}

func serverConfigFor(g *globals) serverConfig {
	cl, err := client(g)
	if err != nil {
		return serverConfig{}
	}
	b, err := cl.do("GET", "/v1/config", nil, "")
	if err != nil {
		return serverConfig{}
	}
	var cfg serverConfig
	_ = json.Unmarshal(b, &cfg)
	return cfg
}

func maybeGrantOrgWideViewer(c *Client, docID string) (bool, error) {
	if os.Getenv(orgWideViewerEnv) == "" {
		return false, nil
	}
	_, err := c.doJSON("POST", apiPath("/v1/documents/%s/grants", docID), map[string]any{
		"principal": map[string]any{"type": "anyone"},
		"role":      "viewer",
	})
	return true, err
}

func orgWideViewerNudge(g *globals, docID string) string {
	if documentHasGrants(g, docID) {
		return ""
	}
	org := serverConfigFor(g).OrgName
	if org == "" {
		org = "your org"
	}
	return fmt.Sprintf("Give org-wide read-only access? We are transparent here at %s. You can do it with: aidocs docs grants add %s --everyone --role viewer", org, docID)
}

func printOrgWideViewerNudge(out io.Writer, g *globals, docID string) error {
	if g.quiet || g.json {
		return nil
	}
	if nudge := orgWideViewerNudge(g, docID); nudge != "" {
		_, err := fmt.Fprintln(out, nudge)
		return err
	}
	return nil
}

func documentHasGrants(g *globals, docID string) bool {
	cl, err := client(g)
	if err != nil {
		return true
	}
	b, err := cl.do("GET", apiPath("/v1/documents/%s/grants", docID), nil, "")
	if err != nil {
		return true
	}
	var resp struct {
		Items []any `json:"items"`
	}
	if json.Unmarshal(b, &resp) != nil {
		return true
	}
	return len(resp.Items) > 0
}
