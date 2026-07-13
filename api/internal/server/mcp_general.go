package server

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

const mcpAuthoringGuidelines = `aidocs document authoring guidelines

- Publish one complete, self-contained HTML document. Inline critical CSS and JavaScript.
- Use semantic, selectable HTML for prose and data so review comments anchor reliably.
- Embed important images as data URLs. External HTTPS assets are allowed, but only use stable sources you trust.
- Keep the first screen useful and avoid hiding important content behind hover-only interactions.
- Respect the reader theme. Use CSS variables with sensible fallbacks and support light and dark color schemes.
- Prefer accessible labels, keyboard-operable controls, readable typography, and responsive layouts.
- Use versioned HTML updates for source/template changes. Start every update from the current HTML and current_version_id.
- Check open review comments before publishing a new version and include a short change summary.
- New documents are private until explicitly shared. Grant only the access role the recipient needs.
- Keep secrets and private credentials out of document HTML. Uploaded HTML is rendered as active content on an isolated origin.
`

func registerMCPGeneralTools(server *mcpserver.MCPServer, api mcpAPIAdapter) {
	server.AddTool(mcp.NewTool(
		"whoami",
		readOnlyOptions(mcp.WithDescription("Return the authenticated aidocs principal used by this MCP connection."))...,
	), func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return api.get(ctx, "/v1/me", nil, api.handlers.me)
	})

	server.AddTool(mcp.NewTool(
		"get_server_config",
		readOnlyOptions(mcp.WithDescription("Return this aidocs server's deployment mode, organisation label, and public configuration."))...,
	), func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return api.get(ctx, "/v1/config", nil, api.handlers.config)
	})

	server.AddTool(mcp.NewTool(
		"get_authoring_guidelines",
		readOnlyOptions(mcp.WithDescription("Return the aidocs single-file HTML authoring and publishing rules. Read these before creating or updating a document."))...,
	), func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText(mcpAuthoringGuidelines), nil
	})
}
