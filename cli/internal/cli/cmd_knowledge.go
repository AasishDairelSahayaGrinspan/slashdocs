package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func repositorySourcesCmd(g *globals, out io.Writer) *cobra.Command {
	c := &cobra.Command{Use: "sources", Short: "Manage connected knowledge sources"}

	list := simplePath(g, out, "list", http.MethodGet, "/v1/repositories/%s/sources")
	list.Use = "list <repo_id>"
	list.Short = "List connected sources"

	var provider, externalRepository, branch, targetFolder, targetDocument, trigger, direction string
	var includePaths, excludePaths []string
	add := &cobra.Command{
		Use:   "add <repo_id>",
		Short: "Connect a Git or knowledge source",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if provider == "" {
				return errors.New("--provider is required")
			}
			body := map[string]any{
				"provider":            provider,
				"external_repository": externalRepository,
				"branch":              branch,
				"include_paths":       includePaths,
				"exclude_paths":       excludePaths,
				"target_folder_id":    targetFolder,
				"target_document_id":  targetDocument,
				"trigger_mode":        trigger,
				"direction":           direction,
			}
			return action(g, out, func(c *Client) ([]byte, error) {
				return c.doJSON(http.MethodPost, apiPath("/v1/repositories/%s/sources", args[0]), body)
			}, func(m map[string]any) string {
				return fmt.Sprintf("Connected %s source %s (%s).", value(m, "provider"), value(m, "external_repository"), value(m, "id"))
			})
		},
	}
	add.Flags().StringVar(&provider, "provider", "", "source provider: github, slack, devrev, google_drive, confluence, notion, or manual")
	add.Flags().StringVar(&externalRepository, "repository", "", "provider repository or external source identifier")
	add.Flags().StringVar(&branch, "branch", "main", "tracked branch or revision")
	add.Flags().StringSliceVar(&includePaths, "include", nil, "included path glob (repeatable)")
	add.Flags().StringSliceVar(&excludePaths, "exclude", nil, "excluded path glob (repeatable)")
	add.Flags().StringVar(&targetFolder, "target-folder", "", "target aidocs folder ID")
	add.Flags().StringVar(&targetDocument, "target-document", "", "target aidocs document ID")
	add.Flags().StringVar(&trigger, "trigger", "", "default_branch, merged_pull_requests, event, manual, or scheduled (defaults by provider)")
	add.Flags().StringVar(&direction, "direction", "into_aidocs", "into_aidocs, bidirectional, or export_only")

	var before, after, delivery string
	var changedFiles []string
	checkCommand := func(name string, persist bool) *cobra.Command {
		short := "Preview a source update"
		pathSuffix := "simulate"
		if persist {
			short = "Check a source and record its documentation proposal"
			pathSuffix = "check"
		}
		cmd := &cobra.Command{
			Use:   name + " <repo_id> <source_id>",
			Short: short,
			Args:  exactArgs(2),
			RunE: func(cmd *cobra.Command, args []string) error {
				return run(g, out, func(c *Client) ([]byte, error) {
					return c.doJSON(http.MethodPost, apiPath("/v1/repositories/%s/sources/%s/"+pathSuffix, args[0], args[1]), map[string]any{
						"before_sha": before, "after_sha": after, "delivery_id": delivery, "changed_files": changedFiles,
					})
				})
			},
		}
		cmd.Flags().StringVar(&before, "before", "", "previous provider revision")
		cmd.Flags().StringVar(&after, "after", "", "new provider revision")
		cmd.Flags().StringVar(&delivery, "delivery", "", "provider delivery id")
		cmd.Flags().StringSliceVar(&changedFiles, "changed-file", nil, "changed source path (repeatable; avoids a provider API call)")
		return cmd
	}
	check := checkCommand("check", true)
	simulate := checkCommand("simulate", false)

	export := &cobra.Command{
		Use:   "export <repo_id> <source_id>",
		Short: "Create a reviewable provider export proposal",
		Args:  exactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(g, out, func(c *Client) ([]byte, error) {
				return c.doJSON(http.MethodPost, apiPath("/v1/repositories/%s/sources/%s/export", args[0], args[1]), map[string]any{})
			})
		},
	}
	deleteSource := mutate(g, out, "delete <repo_id> <source_id>", "Disconnect a source", http.MethodDelete, "/v1/repositories/%s/sources/%s", 2, func(args []string) string {
		return "Disconnected source " + args[1] + "."
	})
	update := jsonFileCommand(g, out,
		"update <repo_id> <source_id> <settings.json>",
		"Update a connected source from a JSON file",
		http.MethodPatch,
		"/v1/repositories/%s/sources/%s",
		2,
	)

	c.AddCommand(list, add, update, check, simulate, export, deleteSource)
	return c
}

func repositorySettingsCmd(g *globals, out io.Writer) *cobra.Command {
	c := &cobra.Command{Use: "settings", Short: "Manage repository policy and lifecycle"}
	show := simplePath(g, out, "show", http.MethodGet, "/v1/repositories/%s/settings")
	show.Use = "show <repo_id>"
	update := jsonFileCommand(g, out,
		"update <repo_id> <settings.json>",
		"Update repository settings from a JSON file",
		http.MethodPatch,
		"/v1/repositories/%s/settings",
		1,
	)
	archive := mutate(g, out, "archive <repo_id>", "Archive a repository", http.MethodPost, "/v1/repositories/%s/archive", 1, func(args []string) string {
		return "Archived repository " + args[0] + "."
	})
	restore := mutate(g, out, "restore <repo_id>", "Restore an archived repository", http.MethodPost, "/v1/repositories/%s/restore", 1, func(args []string) string {
		return "Restored repository " + args[0] + "."
	})
	var ownerEmail string
	transfer := &cobra.Command{
		Use:   "transfer <repo_id>",
		Short: "Transfer repository ownership",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(ownerEmail) == "" {
				return errors.New("--to is required")
			}
			return action(g, out, func(c *Client) ([]byte, error) {
				return c.doJSON(http.MethodPost, apiPath("/v1/repositories/%s/transfer", args[0]), map[string]any{"new_owner_email": ownerEmail})
			}, func(map[string]any) string {
				return "Requested transfer of repository " + args[0] + " to " + ownerEmail + ". They must accept it."
			})
		},
	}
	transfer.Flags().StringVar(&ownerEmail, "to", "", "new owner email")
	transfer.AddCommand(
		mutate(g, out, "accept <repo_id>", "Accept an incoming repository transfer", http.MethodPost, "/v1/repositories/%s/transfer/accept", 1, func(args []string) string {
			return "Accepted repository transfer for " + args[0] + "."
		}),
		mutate(g, out, "cancel <repo_id>", "Cancel a pending repository transfer", http.MethodPost, "/v1/repositories/%s/transfer/cancel", 1, func(args []string) string {
			return "Cancelled repository transfer for " + args[0] + "."
		}),
	)
	c.AddCommand(show, update, archive, restore, transfer)
	return c
}

func repositoryTeamsCmd(g *globals, out io.Writer) *cobra.Command {
	teams := &cobra.Command{Use: "teams", Short: "Manage repository review teams"}
	list := simplePath(g, out, "list", http.MethodGet, "/v1/repositories/%s/teams")
	list.Use = "list <repo_id>"
	list.Short = "List repository teams"

	var createName, createDescription string
	var createMembers []string
	create := &cobra.Command{
		Use:   "create <repo_id>",
		Short: "Create a repository team",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(createName) == "" {
				return errors.New("--name is required")
			}
			return action(g, out, func(c *Client) ([]byte, error) {
				return c.doJSON(http.MethodPost, apiPath("/v1/repositories/%s/teams", args[0]), map[string]any{
					"name": createName, "description": createDescription, "member_ids": createMembers,
				})
			}, func(m map[string]any) string {
				return fmt.Sprintf("Created team %s (%s).", value(m, "name"), value(m, "id"))
			})
		},
	}
	create.Flags().StringVar(&createName, "name", "", "team name (required)")
	create.Flags().StringVar(&createDescription, "description", "", "team description")
	create.Flags().StringSliceVar(&createMembers, "member", nil, "initial user ID (repeatable)")

	var updateName, updateDescription string
	var updateMembers []string
	update := &cobra.Command{
		Use:   "update <repo_id> <team_id>",
		Short: "Update a repository team",
		Args:  exactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{}
			if cmd.Flags().Changed("name") {
				if strings.TrimSpace(updateName) == "" {
					return errors.New("--name cannot be empty")
				}
				body["name"] = updateName
			}
			if cmd.Flags().Changed("description") {
				body["description"] = updateDescription
			}
			if cmd.Flags().Changed("member") {
				body["member_ids"] = updateMembers
			}
			if len(body) == 0 {
				return errors.New("pass at least one of --name, --description, or --member")
			}
			return action(g, out, func(c *Client) ([]byte, error) {
				return c.doJSON(http.MethodPatch, apiPath("/v1/repositories/%s/teams/%s", args[0], args[1]), body)
			}, func(m map[string]any) string {
				return fmt.Sprintf("Updated team %s (%s).", value(m, "name"), args[1])
			})
		},
	}
	update.Flags().StringVar(&updateName, "name", "", "new team name")
	update.Flags().StringVar(&updateDescription, "description", "", "new team description")
	update.Flags().StringSliceVar(&updateMembers, "member", nil, "replace membership with user ID (repeatable)")

	deleteTeam := mutate(g, out, "delete <repo_id> <team_id>", "Delete a repository team", http.MethodDelete, "/v1/repositories/%s/teams/%s", 2, func(args []string) string {
		return "Deleted team " + args[1] + "."
	})
	members := &cobra.Command{Use: "members", Aliases: []string{"member"}, Short: "Manage team members"}
	members.AddCommand(
		mutate(g, out, "add <repo_id> <team_id> <user_id>", "Add a user to a team", http.MethodPut, "/v1/repositories/%s/teams/%s/members/%s", 3, func(args []string) string {
			return "Added user " + args[2] + " to team " + args[1] + "."
		}),
		mutate(g, out, "remove <repo_id> <team_id> <user_id>", "Remove a user from a team", http.MethodDelete, "/v1/repositories/%s/teams/%s/members/%s", 3, func(args []string) string {
			return "Removed user " + args[2] + " from team " + args[1] + "."
		}),
	)
	teams.AddCommand(list, create, update, deleteTeam, members)
	return teams
}

func repositoryInsightsCommands(g *globals, out io.Writer) []*cobra.Command {
	activity := simplePath(g, out, "activity", http.MethodGet, "/v1/repositories/%s/activity")
	activity.Use = "activity <repo_id>"
	activity.Short = "List repository audit activity"
	graph := simplePath(g, out, "graph", http.MethodGet, "/v1/repositories/%s/graph")
	graph.Use = "graph <repo_id>"
	graph.Short = "Show document links and dependencies"
	backlinks := &cobra.Command{Use: "backlinks <repo_id> <document_id>", Short: "List document backlinks", Args: exactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		return run(g, out, func(c *Client) ([]byte, error) {
			q := url.Values{"document_id": []string{args[1]}}
			return c.do(http.MethodGet, apiPath("/v1/repositories/%s/backlinks", args[0])+"?"+q.Encode(), nil, "")
		})
	}}
	runs := simplePath(g, out, "sync-runs", http.MethodGet, "/v1/repositories/%s/sync-runs")
	runs.Use = "sync-runs <repo_id>"
	runs.Short = "List source synchronization history"

	search := &cobra.Command{Use: "search <repo_id> <query>", Short: "Search repository document text", Args: exactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		return run(g, out, func(c *Client) ([]byte, error) {
			q := url.Values{"q": []string{args[1]}}
			return c.do(http.MethodGet, apiPath("/v1/repositories/%s/search", args[0])+"?"+q.Encode(), nil, "")
		})
	}}
	ask := &cobra.Command{Use: "ask <repo_id> <question>", Short: "Answer a question with repository citations", Args: exactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		return run(g, out, func(c *Client) ([]byte, error) {
			return c.doJSON(http.MethodPost, apiPath("/v1/repositories/%s/questions", args[0]), map[string]any{"question": args[1]})
		})
	}}
	return []*cobra.Command{activity, graph, backlinks, runs, search, ask}
}

func connectedKnowledgeRootCommands(g *globals, out io.Writer) []*cobra.Command {
	inbox := simple(g, out, "inbox", http.MethodGet, "/v1/review-inbox", 0)
	inbox.Short = "Show pull requests awaiting your review"

	notifications := &cobra.Command{Use: "notifications", Short: "Manage notifications"}
	notifications.AddCommand(simple(g, out, "list", http.MethodGet, "/v1/notifications", 0))
	var unread bool
	mark := &cobra.Command{Use: "mark <notification_id>", Short: "Mark a notification read or unread", Args: exactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return run(g, out, func(c *Client) ([]byte, error) {
			return c.doJSON(http.MethodPatch, apiPath("/v1/notifications/%s", args[0]), map[string]any{"read": !unread})
		})
	}}
	mark.Flags().BoolVar(&unread, "unread", false, "mark the notification unread")
	notifications.AddCommand(mark)

	templates := &cobra.Command{Use: "templates", Short: "Use repository document templates"}
	templates.AddCommand(simple(g, out, "list", http.MethodGet, "/v1/templates", 0))
	var targetDocument, templateTitle string
	var preview bool
	apply := &cobra.Command{Use: "apply <repo_id> <template_id>", Short: "Apply a template to a repository", Args: exactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		return run(g, out, func(c *Client) ([]byte, error) {
			return c.doJSON(http.MethodPost, apiPath("/v1/repositories/%s/templates/%s/apply", args[0], args[1]), map[string]any{
				"target_document_id": targetDocument,
				"title":              templateTitle,
				"preview_only":       preview,
			})
		})
	}}
	apply.Flags().StringVar(&targetDocument, "target-document", "", "document ID to update through a pull request")
	apply.Flags().StringVar(&templateTitle, "title", "", "override the generated document title")
	apply.Flags().BoolVar(&preview, "preview", false, "preview without opening a pull request")
	templates.AddCommand(apply)
	return []*cobra.Command{inbox, notifications, templates}
}

func jsonFileCommand(g *globals, out io.Writer, use, short, method, pathTemplate string, idCount int) *cobra.Command {
	return &cobra.Command{Use: use, Short: short, Args: exactArgs(idCount + 1), RunE: func(cmd *cobra.Command, args []string) error {
		data, err := os.ReadFile(args[idCount])
		if err != nil {
			return err
		}
		var payload any
		if err := json.Unmarshal(data, &payload); err != nil {
			return fmt.Errorf("invalid JSON file: %w", err)
		}
		ids := append([]string(nil), args[:idCount]...)
		return run(g, out, func(c *Client) ([]byte, error) {
			return c.do(method, apiPath(pathTemplate, ids...), bytes.NewReader(data), "application/json")
		})
	}}
}
