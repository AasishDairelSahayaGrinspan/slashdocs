package internal

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

func foldersCmd(g *globals, out io.Writer) *cobra.Command {
	c := &cobra.Command{Use: "folders", Short: "Manage folders"}

	var title string
	create := &cobra.Command{
		Use:     "create",
		Short:   "Create a folder",
		Example: "  aidocs folders create --title 'Team Reports'",
		Args:    exactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			if title == "" {
				return errors.New("--title is required")
			}
			return action(g, out, func(c *Client) ([]byte, error) {
				return c.doJSON("POST", "/v1/folders", map[string]any{"title": title})
			}, func(m map[string]any) string {
				return fmt.Sprintf("Created folder %q (%s).", title, fmt.Sprint(value(m, "id")))
			})
		},
	}
	create.Flags().StringVar(&title, "title", "", "folder title")

	var ut string
	update := &cobra.Command{
		Use:   "update <folder_id>",
		Short: "Rename a folder",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Changed("title") {
				return errors.New("nothing to update; pass --title")
			}
			return action(g, out, func(c *Client) ([]byte, error) {
				return c.doJSON("PATCH", apiPath("/v1/folders/%s", args[0]), map[string]any{"title": ut})
			}, func(map[string]any) string { return "Renamed folder " + args[0] + "." })
		},
	}
	update.Flags().StringVar(&ut, "title", "", "new folder title")

	del := mutate(g, out, "delete <folder_id>", "Delete a folder", "DELETE", "/v1/folders/%s", 1,
		func(a []string) string { return "Deleted folder " + a[0] + "." })

	c.AddCommand(
		simple(g, out, "list", "GET", "/v1/folders", 0),
		create,
		simplePath(g, out, "show", "GET", "/v1/folders/%s"),
		update,
		del,
		folderGrantsCmd(g, out),
		folderDocsCmd(g, out),
	)
	return c
}

func folderGrantsCmd(g *globals, out io.Writer) *cobra.Command {
	c := &cobra.Command{Use: "grants", Short: "Manage folder grants"}
	var principal, address, role string
	var everyone bool
	add := &cobra.Command{
		Use:   "add <folder_id>",
		Short: "Share a folder with a person or everyone",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{"role": role}
			var target string
			switch {
			case everyone && (address != "" || principal != ""):
				return errors.New("--everyone cannot be combined with --to or --principal")
			case everyone:
				body["principal"] = map[string]any{"type": "anyone"}
				target = everyoneLabelFor(g)
			case address != "":
				body["address"] = address
				target = address
			case principal != "":
				body["principal"] = parsePrincipal(principal)
				target = principal
			default:
				return errors.New("pass --to <email> or --everyone")
			}
			return action(g, out, func(c *Client) ([]byte, error) {
				return c.doJSON("POST", apiPath("/v1/folders/%s/grants", args[0]), body)
			}, func(m map[string]any) string {
				return fmt.Sprintf("Shared folder %s with %s as %s.", args[0], target, role)
			})
		},
	}
	add.Flags().StringVar(&address, "to", "", "email or bot address")
	add.Flags().BoolVar(&everyone, "everyone", false, "share with everyone")
	add.Flags().StringVar(&principal, "principal", "", "sa:<id> or user:<email>")
	add.Flags().StringVar(&role, "role", "viewer", "viewer, commenter, or editor")

	var r string
	upd := &cobra.Command{
		Use:   "update <folder_id> <grant_id>",
		Short: "Update a folder grant role",
		Args:  exactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return action(g, out, func(c *Client) ([]byte, error) {
				return c.doJSON("PATCH", apiPath("/v1/folders/%s/grants/%s", args[0], args[1]), map[string]any{"role": r})
			}, func(map[string]any) string {
				return "Changed grant " + args[1] + " on folder " + args[0] + " to " + r + "."
			})
		},
	}
	upd.Flags().StringVar(&r, "role", "viewer", "new role: viewer, commenter, editor, or owner")

	revoke := mutate(g, out, "revoke <folder_id> <grant_id>", "Revoke a folder grant", "DELETE",
		"/v1/folders/%s/grants/%s", 2, func(a []string) string {
			return "Revoked grant " + a[1] + " on folder " + a[0] + "."
		})

	c.AddCommand(simplePath(g, out, "list", "GET", "/v1/folders/%s/grants"), add, upd, revoke)
	return c
}

func folderDocsCmd(g *globals, out io.Writer) *cobra.Command {
	c := &cobra.Command{Use: "docs", Short: "Manage documents in a folder"}

	var docID string
	add := &cobra.Command{
		Use:   "add <folder_id>",
		Short: "Add a document to a folder",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if docID == "" {
				return errors.New("--doc is required")
			}
			return action(g, out, func(c *Client) ([]byte, error) {
				return c.doJSON("POST", apiPath("/v1/folders/%s/documents", args[0]), map[string]any{"document_id": docID})
			}, func(m map[string]any) string {
				conflicts := conflictCount(m)
				if conflicts > 0 {
					return fmt.Sprintf("Added %s to folder %s. Warning: %d grant conflict(s) detected — run 'aidocs folders show %s' to review.", docID, args[0], conflicts, args[0])
				}
				return fmt.Sprintf("Added %s to folder %s.", docID, args[0])
			})
		},
	}
	add.Flags().StringVar(&docID, "doc", "", "document ID to add")

	remove := &cobra.Command{
		Use:   "remove <folder_id> <doc_id>",
		Short: "Remove a document from a folder",
		Args:  exactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return action(g, out, func(c *Client) ([]byte, error) {
				return c.do("DELETE", apiPath("/v1/folders/%s/documents/%s", args[0], args[1]), nil, "")
			}, func(map[string]any) string {
				return "Removed " + args[1] + " from folder " + args[0] + "."
			})
		},
	}

	c.AddCommand(simplePath(g, out, "list", "GET", "/v1/folders/%s/documents"), add, remove)
	return c
}

// conflictCount extracts the length of the grant_conflicts array from a response map.
func conflictCount(m map[string]any) int {
	raw, ok := m["grant_conflicts"]
	if !ok {
		return 0
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return 0
	}
	var arr []any
	if err := json.Unmarshal(b, &arr); err != nil {
		return 0
	}
	return len(arr)
}
