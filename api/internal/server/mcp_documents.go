package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

func mcpDocumentID() mcp.ToolOption {
	return mcp.WithString("document_id", mcp.Required(), mcp.Description("Document ID (doc_…)."))
}

func mcpVersionID(required bool) mcp.ToolOption {
	options := []mcp.PropertyOption{mcp.Description("Document version ID (ver_…).")}
	if required {
		options = append(options, mcp.Required())
	}
	return mcp.WithString("version_id", options...)
}

func mcpCommentID() mcp.ToolOption {
	return mcp.WithString("comment_id", mcp.Required(), mcp.Description("Comment ID (cmt_…)."))
}

func registerMCPDocumentTools(server *mcpserver.MCPServer, api mcpAPIAdapter) {
	registerMCPDocumentCRUDTools(server, api)
	registerMCPDocumentMoveTool(server, api)
	registerMCPVersionTools(server, api)
	registerMCPCommentTools(server, api)
	registerMCPCommentMutationTools(server, api)
	registerMCPDocumentAccessTools(server, api)
}

func registerMCPDocumentCRUDTools(server *mcpserver.MCPServer, api mcpAPIAdapter) {
	server.AddTool(mcp.NewTool("list_documents", readOnlyOptions(
		mcp.WithDescription("List documents visible to the caller, optionally filtering by title."),
		mcp.WithString("query", mcp.Description("Optional title search text.")),
	)...), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		values := url.Values{}
		if query := request.GetString("query", ""); query != "" {
			values.Set("q", query)
		}
		target := "/v1/documents"
		if len(values) > 0 {
			target += "?" + values.Encode()
		}
		return api.get(ctx, target, nil, api.handlers.listDocuments)
	})

	server.AddTool(mcp.NewTool("get_document", readOnlyOptions(
		mcp.WithDescription("Get document metadata, including owner, folder and current_version_id."),
		mcpDocumentID(),
	)...), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, invalid := mcpRequired(request, "document_id")
		if invalid != nil {
			return invalid, nil
		}
		return api.get(ctx, "/v1/documents/"+id, mcpParams("id", id), api.handlers.getDocument)
	})

	server.AddTool(mcp.NewTool("create_document", mutationOptions(false, false,
		mcp.WithDescription("Create a private aidocs document from complete self-contained HTML. Returns its document and initial version IDs."),
		mcp.WithString("title", mcp.Required(), mcp.Description("Human-readable document title.")),
		mcp.WithString("html", mcp.Required(), mcp.Description("Complete self-contained HTML, up to 10 MiB.")),
	)...), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		title, invalid := mcpRequired(request, "title")
		if invalid != nil {
			return invalid, nil
		}
		html, invalid := mcpRequired(request, "html")
		if invalid != nil {
			return invalid, nil
		}
		return api.multipart(ctx, "/v1/documents", map[string]string{"title": title}, html, nil, api.handlers.createDocument)
	})

	server.AddTool(mcp.NewTool("rename_document", mutationOptions(true, true,
		mcp.WithDescription("Rename a document without changing its HTML or access grants."),
		mcpDocumentID(),
		mcp.WithString("title", mcp.Required(), mcp.Description("New document title.")),
	)...), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, invalid := mcpRequired(request, "document_id")
		if invalid != nil {
			return invalid, nil
		}
		title, invalid := mcpRequired(request, "title")
		if invalid != nil {
			return invalid, nil
		}
		return api.json(ctx, http.MethodPatch, "/v1/documents/"+id, map[string]any{"title": title}, mcpParams("id", id), api.handlers.patchDocument)
	})

	server.AddTool(mcp.NewTool("delete_document", mutationOptions(true, true,
		mcp.WithDescription("Permanently delete a document and its versions."),
		mcpDocumentID(),
	)...), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, invalid := mcpRequired(request, "document_id")
		if invalid != nil {
			return invalid, nil
		}
		return api.delete(ctx, "/v1/documents/"+id, mcpParams("id", id), api.handlers.deleteDocument)
	})

}

func registerMCPDocumentMoveTool(server *mcpserver.MCPServer, api mcpAPIAdapter) {
	server.AddTool(mcp.NewTool("move_document", mutationOptions(true, true,
		mcp.WithDescription("Move a document into a folder, or move it to the workspace root. Pass exactly one of folder_id or to_root=true."),
		mcpDocumentID(),
		mcp.WithString("folder_id", mcp.Description("Destination folder ID (fld_…).")),
		mcp.WithBoolean("to_root", mcp.Description("Move the document to the workspace root.")),
	)...), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		documentID, invalid := mcpRequired(request, "document_id")
		if invalid != nil {
			return invalid, nil
		}
		folderID := request.GetString("folder_id", "")
		toRoot := request.GetBool("to_root", false)
		if (folderID == "") == !toRoot {
			return mcp.NewToolResultError("pass exactly one of folder_id or to_root=true"), nil
		}
		if folderID != "" {
			return api.json(ctx, http.MethodPost, "/v1/folders/"+folderID+"/documents", map[string]any{"document_id": documentID}, mcpParams("id", folderID), api.handlers.moveDocToFolder)
		}
		response, err := api.call(ctx, http.MethodGet, "/v1/documents/"+documentID, "", nil, mcpParams("id", documentID), api.handlers.getDocument)
		if err != nil || response.status < 200 || response.status >= 300 {
			return api.result(response, err, false)
		}
		var document struct {
			ParentFolderID string `json:"parent_folder_id"`
		}
		if err := json.Unmarshal(response.body, &document); err != nil {
			return mcp.NewToolResultError("could not decode document metadata: " + err.Error()), nil
		}
		if document.ParentFolderID == "" {
			return mcp.NewToolResultError("document is already at the workspace root"), nil
		}
		return api.delete(ctx, "/v1/folders/"+document.ParentFolderID+"/documents/"+documentID, mcpParams("id", document.ParentFolderID, "doc_id", documentID), api.handlers.moveDocToRoot)
	})
}

func registerMCPVersionTools(server *mcpserver.MCPServer, api mcpAPIAdapter) {
	server.AddTool(mcp.NewTool("list_document_versions", readOnlyOptions(
		mcp.WithDescription("List all versions of a document."),
		mcpDocumentID(),
	)...), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, invalid := mcpRequired(request, "document_id")
		if invalid != nil {
			return invalid, nil
		}
		return api.get(ctx, "/v1/documents/"+id+"/versions", mcpParams("id", id), api.handlers.listVersions)
	})

	server.AddTool(mcp.NewTool("get_document_version", readOnlyOptions(
		mcp.WithDescription("Get metadata for a specific document version."),
		mcpDocumentID(), mcpVersionID(true),
	)...), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		doc, invalid := mcpRequired(request, "document_id")
		if invalid != nil {
			return invalid, nil
		}
		version, invalid := mcpRequired(request, "version_id")
		if invalid != nil {
			return invalid, nil
		}
		return api.get(ctx, "/v1/documents/"+doc+"/versions/"+version, mcpParams("id", doc, "version_id", version), api.handlers.getVersion)
	})

	server.AddTool(mcp.NewTool("get_document_html", readOnlyOptions(
		mcp.WithDescription("Fetch raw document HTML. Omit version_id to fetch the current version."),
		mcpDocumentID(), mcpVersionID(false),
	)...), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		doc, invalid := mcpRequired(request, "document_id")
		if invalid != nil {
			return invalid, nil
		}
		version := request.GetString("version_id", "")
		if version == "" {
			response, err := api.call(ctx, http.MethodGet, "/v1/documents/"+doc, "", nil, mcpParams("id", doc), api.handlers.getDocument)
			if err != nil || response.status < 200 || response.status >= 300 {
				return api.result(response, err, false)
			}
			var metadata struct {
				CurrentVersionID string `json:"current_version_id"`
			}
			if err := json.Unmarshal(response.body, &metadata); err != nil || metadata.CurrentVersionID == "" {
				return mcp.NewToolResultError("document response did not include current_version_id"), nil
			}
			version = metadata.CurrentVersionID
		}
		return api.html(ctx, "/v1/documents/"+doc+"/versions/"+version+"/html", mcpParams("id", doc, "version_id", version), api.handlers.getVersionHTML)
	})

	server.AddTool(mcp.NewTool("update_document", mutationOptions(false, false,
		mcp.WithDescription("Publish complete HTML as a new document version using optimistic concurrency."),
		mcpDocumentID(),
		mcp.WithString("base_version_id", mcp.Required(), mcp.Description("Version on which these edits are based.")),
		mcp.WithString("html", mcp.Required(), mcp.Description("Complete replacement HTML, up to 10 MiB.")),
		mcp.WithString("change_summary", mcp.Description("Short summary of the changes.")),
	)...), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		doc, invalid := mcpRequired(request, "document_id")
		if invalid != nil {
			return invalid, nil
		}
		base, invalid := mcpRequired(request, "base_version_id")
		if invalid != nil {
			return invalid, nil
		}
		html, invalid := mcpRequired(request, "html")
		if invalid != nil {
			return invalid, nil
		}
		fields := map[string]string{"base_version_id": base, "change_summary": request.GetString("change_summary", "")}
		return api.multipart(ctx, "/v1/documents/"+doc+"/versions", fields, html, mcpParams("id", doc), api.handlers.createVersion)
	})
}

func registerMCPCommentTools(server *mcpserver.MCPServer, api mcpAPIAdapter) {
	server.AddTool(mcp.NewTool("list_comments", readOnlyOptions(
		mcp.WithDescription("List document comment threads, optionally filtered by status or version."),
		mcpDocumentID(),
		mcp.WithString("status", mcp.Description("Optional status filter."), mcp.Enum("open", "resolved", "stale", "orphaned", "all")),
		mcp.WithString("version_id", mcp.Description("Optional version filter.")),
	)...), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		doc, invalid := mcpRequired(request, "document_id")
		if invalid != nil {
			return invalid, nil
		}
		values := url.Values{}
		if status := request.GetString("status", ""); status != "" {
			values.Set("status", status)
		}
		if version := request.GetString("version_id", ""); version != "" {
			values.Set("version_id", version)
		}
		target := "/v1/documents/" + doc + "/comments"
		if len(values) > 0 {
			target += "?" + values.Encode()
		}
		return api.get(ctx, target, mcpParams("id", doc), api.handlers.listComments)
	})

	server.AddTool(mcp.NewTool("create_comment", mutationOptions(false, false,
		mcp.WithDescription("Create a review comment anchored to exact text in a document version."),
		mcpDocumentID(), mcpVersionID(true),
		mcp.WithString("body", mcp.Required(), mcp.Description("Comment text.")),
		mcp.WithString("quote", mcp.Required(), mcp.Description("Exact selected text.")),
		mcp.WithString("prefix", mcp.Description("Text immediately before the quote.")),
		mcp.WithString("suffix", mcp.Description("Text immediately after the quote.")),
	)...), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		doc, invalid := mcpRequired(request, "document_id")
		if invalid != nil {
			return invalid, nil
		}
		version, invalid := mcpRequired(request, "version_id")
		if invalid != nil {
			return invalid, nil
		}
		body, invalid := mcpRequired(request, "body")
		if invalid != nil {
			return invalid, nil
		}
		quote, invalid := mcpRequired(request, "quote")
		if invalid != nil {
			return invalid, nil
		}
		payload := map[string]any{"version_id": version, "body": body, "anchor": map[string]any{
			"quote": quote, "prefix": request.GetString("prefix", ""), "suffix": request.GetString("suffix", ""),
		}}
		return api.json(ctx, http.MethodPost, "/v1/documents/"+doc+"/comments", payload, mcpParams("id", doc), api.handlers.createComment)
	})

	server.AddTool(mcp.NewTool("reply_comment", mutationOptions(false, false,
		mcp.WithDescription("Reply to an existing document comment thread."),
		mcpDocumentID(), mcpCommentID(),
		mcp.WithString("body", mcp.Required(), mcp.Description("Reply text.")),
	)...), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		doc, comment, body, invalid := mcpCommentArguments(request, true)
		if invalid != nil {
			return invalid, nil
		}
		return api.json(ctx, http.MethodPost, "/v1/documents/"+doc+"/comments/"+comment+"/replies", map[string]any{"body": body}, mcpParams("id", doc, "comment_id", comment), api.handlers.createCommentReply)
	})

}

func registerMCPCommentMutationTools(server *mcpserver.MCPServer, api mcpAPIAdapter) {
	server.AddTool(mcp.NewTool("update_comment", mutationOptions(true, false,
		mcp.WithDescription("Update a comment's body and/or status."),
		mcpDocumentID(), mcpCommentID(),
		mcp.WithString("body", mcp.Description("Replacement comment text.")),
		mcp.WithString("status", mcp.Description("Replacement status."), mcp.Enum("open", "resolved")),
	)...), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		doc, comment, _, invalid := mcpCommentArguments(request, false)
		if invalid != nil {
			return invalid, nil
		}
		payload := map[string]any{}
		if body := request.GetString("body", ""); body != "" {
			payload["body"] = body
		}
		if status := request.GetString("status", ""); status != "" {
			payload["status"] = status
		}
		if len(payload) == 0 {
			return mcp.NewToolResultError("pass body and/or status"), nil
		}
		return api.json(ctx, http.MethodPatch, "/v1/documents/"+doc+"/comments/"+comment, payload, mcpParams("id", doc, "comment_id", comment), api.handlers.patchComment)
	})

	server.AddTool(mcp.NewTool("delete_comment", mutationOptions(true, true,
		mcp.WithDescription("Permanently delete a document comment."), mcpDocumentID(), mcpCommentID(),
	)...), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		doc, comment, _, invalid := mcpCommentArguments(request, false)
		if invalid != nil {
			return invalid, nil
		}
		return api.delete(ctx, "/v1/documents/"+doc+"/comments/"+comment, mcpParams("id", doc, "comment_id", comment), api.handlers.deleteComment)
	})

	for _, definition := range []struct {
		name, status, description string
	}{
		{"resolve_comment", "resolved", "Mark a document comment thread as resolved."},
		{"reopen_comment", "open", "Reopen a resolved document comment thread."},
	} {
		server.AddTool(mcp.NewTool(definition.name, mutationOptions(true, true,
			mcp.WithDescription(definition.description), mcpDocumentID(), mcpCommentID(),
		)...), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			doc, comment, _, invalid := mcpCommentArguments(request, false)
			if invalid != nil {
				return invalid, nil
			}
			return api.json(ctx, http.MethodPatch, "/v1/documents/"+doc+"/comments/"+comment, map[string]any{"status": definition.status}, mcpParams("id", doc, "comment_id", comment), api.handlers.patchComment)
		})
	}
}

func mcpCommentArguments(request mcp.CallToolRequest, requireBody bool) (string, string, string, *mcp.CallToolResult) {
	doc, invalid := mcpRequired(request, "document_id")
	if invalid != nil {
		return "", "", "", invalid
	}
	comment, invalid := mcpRequired(request, "comment_id")
	if invalid != nil {
		return "", "", "", invalid
	}
	body := ""
	if requireBody {
		body, invalid = mcpRequired(request, "body")
		if invalid != nil {
			return "", "", "", invalid
		}
	}
	return doc, comment, body, nil
}

func registerMCPDocumentAccessTools(server *mcpserver.MCPServer, api mcpAPIAdapter) {
	server.AddTool(mcp.NewTool("list_document_access", readOnlyOptions(
		mcp.WithDescription("List effective access to a document and the grants that provide it."), mcpDocumentID(),
	)...), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		doc, invalid := mcpRequired(request, "document_id")
		if invalid != nil {
			return invalid, nil
		}
		return api.get(ctx, "/v1/documents/"+doc+"/access", mcpParams("id", doc), api.handlers.documentAccess)
	})

	server.AddTool(mcp.NewTool("share_document", mutationOptions(false, false,
		mcp.WithDescription("Grant document access to an email/bot address or to everyone. Pass exactly one of address or everyone=true."),
		mcpDocumentID(),
		mcp.WithString("address", mcp.Description("User email or bot address.")),
		mcp.WithBoolean("everyone", mcp.Description("Grant access to everyone who can reach the server.")),
		mcp.WithString("role", mcp.Description("Role to grant."), mcp.Enum("viewer", "commenter", "editor"), mcp.DefaultString("viewer")),
	)...), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		doc, invalid := mcpRequired(request, "document_id")
		if invalid != nil {
			return invalid, nil
		}
		payload, invalid := mcpGrantPayload(request)
		if invalid != nil {
			return invalid, nil
		}
		return api.json(ctx, http.MethodPost, "/v1/documents/"+doc+"/grants", payload, mcpParams("id", doc), api.handlers.createGrant)
	})

	server.AddTool(mcp.NewTool("update_document_grant", mutationOptions(true, true,
		mcp.WithDescription("Change an existing document grant's role."),
		mcpDocumentID(),
		mcp.WithString("grant_id", mcp.Required(), mcp.Description("Grant ID.")),
		mcp.WithString("role", mcp.Required(), mcp.Enum("viewer", "commenter", "editor")),
	)...), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		doc, invalid := mcpRequired(request, "document_id")
		if invalid != nil {
			return invalid, nil
		}
		grant, invalid := mcpRequired(request, "grant_id")
		if invalid != nil {
			return invalid, nil
		}
		role, invalid := mcpRequired(request, "role")
		if invalid != nil {
			return invalid, nil
		}
		return api.json(ctx, http.MethodPatch, "/v1/documents/"+doc+"/grants/"+grant, map[string]any{"role": role}, mcpParams("id", doc, "grant_id", grant), api.handlers.patchGrant)
	})

	server.AddTool(mcp.NewTool("revoke_document_grant", mutationOptions(true, true,
		mcp.WithDescription("Revoke an existing document grant."),
		mcpDocumentID(), mcp.WithString("grant_id", mcp.Required(), mcp.Description("Grant ID.")),
	)...), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		doc, invalid := mcpRequired(request, "document_id")
		if invalid != nil {
			return invalid, nil
		}
		grant, invalid := mcpRequired(request, "grant_id")
		if invalid != nil {
			return invalid, nil
		}
		return api.delete(ctx, "/v1/documents/"+doc+"/grants/"+grant, mcpParams("id", doc, "grant_id", grant), api.handlers.deleteGrant)
	})
}

func mcpGrantPayload(request mcp.CallToolRequest) (map[string]any, *mcp.CallToolResult) {
	address := request.GetString("address", "")
	everyone := request.GetBool("everyone", false)
	if (address == "") == !everyone {
		return nil, mcp.NewToolResultError("pass exactly one of address or everyone=true")
	}
	payload := map[string]any{"role": request.GetString("role", "viewer")}
	if everyone {
		payload["principal"] = map[string]any{"type": "anyone"}
	} else {
		payload["address"] = address
	}
	return payload, nil
}
