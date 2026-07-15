package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func repositoriesCmd(g *globals, out io.Writer) *cobra.Command {
	c := &cobra.Command{
		Use:     "repos",
		Aliases: []string{"repositories"},
		Short:   "Manage document repositories",
	}

	var name, slug, description, scope, folderID, documentID string
	create := &cobra.Command{
		Use:   "create",
		Short: "Create a personal or organization repository",
		Args:  exactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" {
				return errors.New("--name is required")
			}
			if folderID != "" && documentID != "" {
				return errors.New("pass at most one of --folder-id or --document-id")
			}
			body := map[string]any{
				"name":        name,
				"description": description,
				"scope":       scope,
			}
			if slug != "" {
				body["slug"] = slug
			}
			if folderID != "" {
				body["root_folder_id"] = folderID
			}
			if documentID != "" {
				body["document_id"] = documentID
			}
			return action(g, out, func(c *Client) ([]byte, error) {
				return c.doJSON("POST", "/v1/repositories", body)
			}, func(m map[string]any) string {
				return fmt.Sprintf("Created repository %s/%s (%s).", value(m, "namespace_name"), value(m, "slug"), value(m, "id"))
			})
		},
	}
	create.Flags().StringVar(&name, "name", "", "repository name (required)")
	create.Flags().StringVar(&slug, "slug", "", "URL-friendly repository name (derived from --name when omitted)")
	create.Flags().StringVar(&description, "description", "", "repository description")
	create.Flags().StringVar(&scope, "scope", "personal", "repository scope: personal or organization")
	create.Flags().StringVar(&folderID, "folder-id", "", "optionally convert an owned root folder into this repository")
	create.Flags().StringVar(&documentID, "document-id", "", "optionally seed this repository from an owned loose document")

	list := simple(g, out, "list", "GET", "/v1/repositories", 0)
	list.Short = "List repositories you can access"
	show := simplePath(g, out, "show", "GET", "/v1/repositories/%s")
	show.Short = "Show repository details"
	docs := simplePath(g, out, "docs", "GET", "/v1/repositories/%s/documents")
	docs.Use = "docs <repo_id>"
	docs.Short = "List every document in a repository"
	deleteRepo := mutate(g, out, "delete <repo_id>", "Delete a repository and all of its contents", "DELETE", "/v1/repositories/%s", 1, func(a []string) string {
		return "Deleted repository " + a[0] + "."
	})

	c.AddCommand(list, create, show, docs, deleteRepo, repositoryPullRequestsCmd(g, out), repositorySourcesCmd(g, out), repositorySettingsCmd(g, out), repositoryTeamsCmd(g, out))
	c.AddCommand(repositoryInsightsCommands(g, out)...)
	return c
}

//nolint:gocyclo // PR subcommand wiring stays together so its shared flags remain local.
func repositoryPullRequestsCmd(g *globals, out io.Writer) *cobra.Command {
	c := &cobra.Command{
		Use:     "prs",
		Aliases: []string{"pulls", "pull-requests"},
		Short:   "Manage repository pull requests",
	}

	var listStatus string
	list := &cobra.Command{
		Use:   "list <repo_id>",
		Short: "List pull requests",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := apiPath("/v1/repositories/%s/pull-requests", args[0])
			if listStatus != "" {
				q := url.Values{}
				q.Set("status", listStatus)
				path += "?" + q.Encode()
			}
			return run(g, out, func(c *Client) ([]byte, error) {
				return c.do("GET", path, nil, "")
			})
		},
	}
	list.Flags().StringVar(&listStatus, "status", "", "filter by status: open, merged, or closed")

	show := &cobra.Command{
		Use:   "show <repo_id> <number>",
		Short: "Show a pull request",
		Args:  exactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(g, out, func(c *Client) ([]byte, error) {
				return c.do("GET", apiPath("/v1/repositories/%s/pull-requests/%s", args[0], args[1]), nil, "")
			})
		},
	}

	var base, title, body, summary string
	create := &cobra.Command{
		Use:     "create <repo_id> <doc_id> <file>",
		Short:   "Propose a document update",
		Example: "  aidocs repos prs create repo_123 doc_123 report.html --title 'Update findings' --summary 'Address review'",
		Args:    exactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			repoID, docID, file := args[0], args[1], args[2]
			data, fn, err := readFileArg(file)
			if err != nil {
				return err
			}
			cl, err := client(g)
			if err != nil {
				return err
			}
			baseVersion := base
			if baseVersion == "" {
				_, cx, _ := currentContext(g)
				baseVersion = cx.Pulled[docID]
			}
			if baseVersion == "" {
				b, err := cl.do("GET", apiPath("/v1/documents/%s", docID), nil, "")
				if err != nil {
					return err
				}
				var doc map[string]any
				if err := json.Unmarshal(b, &doc); err != nil {
					return fmt.Errorf("could not parse document response: %w", err)
				}
				baseVersion = fmt.Sprint(value(doc, "current_version_id"))
			}
			if baseVersion == "" || baseVersion == "<nil>" {
				return errors.New("no base version known; pull the document first or pass --base-version")
			}
			prTitle := title
			if prTitle == "" {
				prTitle = summary
			}
			if prTitle == "" {
				prTitle = "Update document"
			}
			resp, err := cl.multipart(
				apiPath("/v1/repositories/%s/pull-requests", repoID),
				map[string]string{
					"document_id":     docID,
					"base_version_id": baseVersion,
					"title":           prTitle,
					"body":            body,
					"change_summary":  summary,
				},
				"file",
				fn,
				data,
			)
			if err != nil {
				return err
			}
			if g.quiet {
				return nil
			}
			var pr map[string]any
			if err := json.Unmarshal(resp, &pr); err != nil {
				return render(out, g, resp)
			}
			number := fmt.Sprint(value(pr, "number"))
			prURL := browserURL(cl.Base, "/app/repos/%s/pulls/%s", repoID, number)
			if g.json {
				pr["url"] = prURL
				b, err := json.Marshal(pr)
				if err != nil {
					return err
				}
				return render(out, g, b)
			}
			return confirm(out, g, fmt.Sprintf("Opened pull request #%s in %s. Review it at %s", number, repoID, prURL))
		},
	}
	create.Flags().StringVar(&base, "base-version", "", "base version ID (defaults to the last pulled or current version)")
	create.Flags().StringVar(&title, "title", "", "pull request title")
	create.Flags().StringVar(&body, "body", "", "pull request description")
	create.Flags().StringVar(&summary, "summary", "", "document change summary")

	var approve, requestChanges bool
	var reviewBody string
	review := &cobra.Command{
		Use:   "review <repo_id> <number>",
		Short: "Approve a pull request or request changes",
		Args:  exactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if approve == requestChanges {
				return errors.New("pass exactly one of --approve or --request-changes")
			}
			state := "approved"
			if requestChanges {
				state = "changes_requested"
			}
			return action(g, out, func(c *Client) ([]byte, error) {
				return c.doJSON("POST", apiPath("/v1/repositories/%s/pull-requests/%s/reviews", args[0], args[1]), map[string]any{
					"state": state,
					"body":  reviewBody,
				})
			}, func(map[string]any) string {
				if approve {
					return "Approved pull request #" + args[1] + "."
				}
				return "Requested changes on pull request #" + args[1] + "."
			})
		},
	}
	review.Flags().BoolVar(&approve, "approve", false, "approve the current proposed version")
	review.Flags().BoolVar(&requestChanges, "request-changes", false, "request changes to the current proposed version")
	review.Flags().StringVar(&reviewBody, "body", "", "review note")

	merge := &cobra.Command{
		Use:   "merge <repo_id> <number>",
		Short: "Merge an approved pull request",
		Args:  exactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return action(g, out, func(c *Client) ([]byte, error) {
				return c.doJSON("POST", apiPath("/v1/repositories/%s/pull-requests/%s/merge", args[0], args[1]), map[string]any{})
			}, func(map[string]any) string {
				return "Merged pull request #" + args[1] + "."
			})
		},
	}

	closePR := &cobra.Command{
		Use:   "close <repo_id> <number>",
		Short: "Close a pull request without merging",
		Args:  exactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return action(g, out, func(c *Client) ([]byte, error) {
				return c.doJSON("PATCH", apiPath("/v1/repositories/%s/pull-requests/%s", args[0], args[1]), map[string]any{"status": "closed"})
			}, func(map[string]any) string {
				return "Closed pull request #" + args[1] + "."
			})
		},
	}

	changeSet := jsonFileCommand(g, out,
		"change-set <repo_id> <spec.json>",
		"Open a multi-document pull request from a JSON change set",
		http.MethodPost,
		"/v1/repositories/%s/pull-requests/change-set",
		1,
	)
	patchset := jsonFileCommand(g, out,
		"patchset <repo_id> <number> <spec.json>",
		"Add a patchset to an open pull request",
		http.MethodPost,
		"/v1/repositories/%s/pull-requests/%s/patchsets",
		2,
	)
	rebase := mutate(g, out,
		"rebase <repo_id> <number>",
		"Rebase an open pull request onto current document versions",
		http.MethodPost,
		"/v1/repositories/%s/pull-requests/%s/rebase",
		2,
		func(args []string) string { return "Rebased pull request #" + args[1] + "." },
	)

	c.AddCommand(list, show, create, changeSet, patchset, rebase, review, merge, closePR)
	return c
}
