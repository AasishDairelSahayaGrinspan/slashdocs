package cli

import (
	"errors"
	"io"
	"net/url"

	"github.com/spf13/cobra"
)

func documentHeadingsCmd(g *globals, out io.Writer) *cobra.Command {
	var version string
	cmd := &cobra.Command{Use: "headings <doc_id>", Short: "List headings, their IDs, and shareable URLs", Args: exactArgs(1), RunE: func(_ *cobra.Command, args []string) error {
		path := apiPath("/v1/documents/%s/headings", args[0])
		if version != "" {
			path += "?" + url.Values{"version": {version}}.Encode()
		}
		return run(g, out, func(c *Client) ([]byte, error) { return c.do("GET", path, nil, "") })
	}}
	cmd.Flags().StringVar(&version, "version", "", "list and link to a specific version")
	return cmd
}

func documentLinkCmd(g *globals, out io.Writer) *cobra.Command {
	var heading, comment, version string
	cmd := &cobra.Command{Use: "link <doc_id>", Short: "Get a URL to a document, heading, or comment", Args: exactArgs(1), RunE: func(_ *cobra.Command, args []string) error {
		if comment != "" && (heading != "" || version != "") {
			return usageError{errors.New("--comment cannot be combined with --heading or --version")}
		}
		q := url.Values{}
		if heading != "" {
			q.Set("heading", heading)
		}
		if comment != "" {
			q.Set("comment", comment)
		}
		if version != "" {
			q.Set("version", version)
		}
		return run(g, out, func(c *Client) ([]byte, error) {
			return c.do("GET", apiPath("/v1/documents/%s/link", args[0])+"?"+q.Encode(), nil, "")
		})
	}}
	cmd.Flags().StringVar(&heading, "heading", "", "HTML heading ID (discover with docs headings)")
	cmd.Flags().StringVar(&comment, "comment", "", "comment or reply ID")
	cmd.Flags().StringVar(&version, "version", "", "pin document or heading link to a version")
	cmd.Example = "  aidocs docs headings doc_123 --json\n  aidocs docs link doc_123 --heading rollout\n  aidocs docs link doc_123 --comment cmt_456"
	return cmd
}
