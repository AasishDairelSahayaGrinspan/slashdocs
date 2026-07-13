package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/razorpay/aidocs/api/internal/auth"
)

// JSON string escaping can expand a valid 10 MiB HTML document by up to 6x
// before the MCP envelope reaches the server (for example, for control bytes
// or clients that escape HTML characters). Keep enough authenticated
// transport headroom to preserve parity with the REST and CLI HTML limit.
const maxMCPRequestBytes int64 = 64 << 20

type mcpPrincipalContextKey struct{}

// mcpHTTPOptions contains the deliberately small integration surface around
// the transport. The challenge function keeps OAuth protected-resource
// metadata discovery configurable without coupling transport code to routing.
type mcpHTTPOptions struct {
	unauthorizedChallenge func(*http.Request) string
}

type mcpHTTPOption func(*mcpHTTPOptions)

func withMCPUnauthorizedChallenge(fn func(*http.Request) string) mcpHTTPOption {
	return func(options *mcpHTTPOptions) {
		if fn != nil {
			options.unauthorizedChallenge = fn
		}
	}
}

// registerMCPRoutes mounts the remote, bearer-authenticated MCP endpoint. It is
// kept separate from routes so transport construction and its security policy
// can be tested independently.
func registerMCPRoutes(engine *gin.Engine, deps dependencies, options ...mcpHTTPOption) {
	engine.Any("/mcp", gin.WrapH(newMCPHTTPHandler(deps, options...)))
}

func newMCPHTTPHandler(deps dependencies, options ...mcpHTTPOption) http.Handler {
	config := mcpHTTPOptions{
		unauthorizedChallenge: func(*http.Request) string { return mcpWWWAuthenticate(deps.appOrigin) },
	}
	for _, option := range options {
		option(&config)
	}

	protocol := mcpserver.NewMCPServer(
		"aidocs",
		BuildVersion,
		mcpserver.WithToolCapabilities(false),
		mcpserver.WithRecovery(),
	)
	api := mcpAPIAdapter{handlers: handlers{deps: deps}}
	registerMCPTools(protocol, api)
	knownTools := protocol.ListTools()
	transport := mcpserver.NewStreamableHTTPServer(
		protocol,
		mcpserver.WithStateLess(true),
		mcpserver.WithDisableStreaming(true),
	)
	authenticator := auth.MCPAuthenticator{
		Users:         deps.repository,
		ServiceTokens: deps.authenticator,
		SessionSecret: deps.sessionSecret,
		Resource:      strings.TrimRight(deps.appOrigin, "/") + "/mcp",
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" &&
			(deps.appOrigin == "" || !sameHTTPOrigin(origin, deps.appOrigin)) {
			observeMCPRequest("transport", "origin_rejected", time.Since(started))
			http.Error(w, "cross-origin MCP requests are not allowed", http.StatusForbidden)
			return
		}
		if auth.BearerFromHeader(r.Header.Get("Authorization")) == "" {
			observeMCPRequest("transport", "auth_error", time.Since(started))
			mcpUnauthorized(w, r, config.unauthorizedChallenge)
			return
		}
		principal, err := authenticator.Authenticate(r)
		if err != nil && !errors.Is(err, auth.ErrUnauthorized) {
			slog.ErrorContext(r.Context(), "MCP authentication failed", "error", err)
			observeMCPRequest("transport", "transport_error", time.Since(started))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(errorResponse("internal", "internal server error", nil))
			return
		}
		if err != nil || principal == nil {
			observeMCPRequest("transport", "auth_error", time.Since(started))
			mcpUnauthorized(w, r, config.unauthorizedChallenge)
			return
		}
		method, tool := "transport", ""
		if r.Method == http.MethodPost {
			if message := mcpTransportHeaderError(r); message != "" {
				observeMCPRequest("transport", "bad_request", time.Since(started))
				http.Error(w, message, http.StatusBadRequest)
				return
			}
			body, err := readMCPBody(r.Body, r.ContentLength)
			if err != nil {
				if _, tooLarge := err.(*http.MaxBytesError); tooLarge {
					observeMCPRequest("transport", "payload_too_large", time.Since(started))
					http.Error(w, "MCP request body is too large", http.StatusRequestEntityTooLarge)
					return
				}
				observeMCPRequest("transport", "bad_request", time.Since(started))
				http.Error(w, "could not read MCP request body", http.StatusBadRequest)
				return
			}
			method, tool = mcpRequestMetricLabels(body, knownTools)
			r.Body = io.NopCloser(bytes.NewReader(body))
			r.ContentLength = int64(len(body))
		}
		ctx := context.WithValue(r.Context(), mcpPrincipalContextKey{}, *principal)
		capture := newMCPResponseCapture(w)
		transport.ServeHTTP(capture, r.WithContext(ctx))
		requestOutcome, toolOutcome := mcpResponseMetricOutcomes(capture.Status(), capture.Body())
		duration := time.Since(started)
		observeMCPRequest(method, requestOutcome, duration)
		if tool != "" {
			observeMCPToolCall(tool, toolOutcome, string(principal.Type), duration)
		}
	})
}

func mcpTransportHeaderError(r *http.Request) string {
	acceptsJSON, acceptsEvents := false, false
	for _, value := range strings.Split(r.Header.Get("Accept"), ",") {
		mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(value))
		if err != nil {
			continue
		}
		acceptsJSON = acceptsJSON || mediaType == "application/json"
		acceptsEvents = acceptsEvents || mediaType == "text/event-stream"
	}
	if !acceptsJSON || !acceptsEvents {
		return "Accept must include application/json and text/event-stream"
	}
	if version := r.Header.Get("Mcp-Protocol-Version"); version != "" && !slices.Contains(mcp.ValidProtocolVersions, version) {
		return "unsupported MCP protocol version"
	}
	return ""
}

func readMCPBody(body io.ReadCloser, contentLength int64) ([]byte, error) {
	if body == nil {
		return nil, nil
	}
	defer func() { _ = body.Close() }()
	if contentLength > maxMCPRequestBytes {
		return nil, &http.MaxBytesError{Limit: maxMCPRequestBytes}
	}
	b, err := io.ReadAll(io.LimitReader(body, maxMCPRequestBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > maxMCPRequestBytes {
		return nil, &http.MaxBytesError{Limit: maxMCPRequestBytes}
	}
	return b, nil
}

func mcpUnauthorized(w http.ResponseWriter, r *http.Request, challenge func(*http.Request) string) {
	value := "Bearer"
	if challenge != nil {
		if custom := strings.TrimSpace(challenge(r)); custom != "" {
			value = custom
		}
	}
	w.Header().Set("WWW-Authenticate", value)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(errorResponse("unauthorized", "bearer authentication required", nil))
}

func mcpPrincipal(ctx context.Context) (auth.Principal, error) {
	principal, ok := ctx.Value(mcpPrincipalContextKey{}).(auth.Principal)
	if !ok || principal.Type == "" || principal.ID == "" {
		return auth.Principal{}, auth.ErrUnauthorized
	}
	return principal, nil
}

type mcpAPIAdapter struct{ handlers handlers }

type mcpAPIResponse struct {
	status      int
	contentType string
	body        []byte
}

func (a mcpAPIAdapter) call(
	ctx context.Context,
	method, target, contentType string,
	body io.Reader,
	params gin.Params,
	handler gin.HandlerFunc,
) (mcpAPIResponse, error) {
	principal, err := mcpPrincipal(ctx)
	if err != nil {
		return mcpAPIResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return mcpAPIResponse{}, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = req
	c.Params = params
	c.Set("principal", &principal)
	handler(c)
	return mcpAPIResponse{
		status:      recorder.Code,
		contentType: recorder.Header().Get("Content-Type"),
		body:        recorder.Body.Bytes(),
	}, nil
}

func (a mcpAPIAdapter) get(ctx context.Context, target string, params gin.Params, handler gin.HandlerFunc) (*mcp.CallToolResult, error) {
	response, err := a.call(ctx, http.MethodGet, target, "", nil, params, handler)
	return a.result(response, err, false)
}

func (a mcpAPIAdapter) delete(ctx context.Context, target string, params gin.Params, handler gin.HandlerFunc) (*mcp.CallToolResult, error) {
	response, err := a.call(ctx, http.MethodDelete, target, "", nil, params, handler)
	return a.result(response, err, false)
}

func (a mcpAPIAdapter) json(ctx context.Context, method, target string, value any, params gin.Params, handler gin.HandlerFunc) (*mcp.CallToolResult, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return mcp.NewToolResultError("could not encode request: " + err.Error()), nil
	}
	response, err := a.call(ctx, method, target, "application/json", bytes.NewReader(body), params, handler)
	return a.result(response, err, false)
}

func (a mcpAPIAdapter) multipart(ctx context.Context, target string, fields map[string]string, html string, params gin.Params, handler gin.HandlerFunc) (*mcp.CallToolResult, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return mcp.NewToolResultError("could not encode request: " + err.Error()), nil
		}
	}
	file, err := writer.CreateFormFile("file", "document.html")
	if err != nil {
		return mcp.NewToolResultError("could not encode request: " + err.Error()), nil
	}
	if _, err := io.WriteString(file, html); err != nil {
		return mcp.NewToolResultError("could not encode request: " + err.Error()), nil
	}
	if err := writer.Close(); err != nil {
		return mcp.NewToolResultError("could not encode request: " + err.Error()), nil
	}
	response, err := a.call(ctx, http.MethodPost, target, writer.FormDataContentType(), &body, params, handler)
	return a.result(response, err, false)
}

func (a mcpAPIAdapter) html(ctx context.Context, target string, params gin.Params, handler gin.HandlerFunc) (*mcp.CallToolResult, error) {
	response, err := a.call(ctx, http.MethodGet, target, "", nil, params, handler)
	return a.result(response, err, true)
}

func (mcpAPIAdapter) result(response mcpAPIResponse, err error, plainText bool) (*mcp.CallToolResult, error) {
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if response.status < http.StatusOK || response.status >= http.StatusMultipleChoices {
		var payload any
		if json.Unmarshal(response.body, &payload) == nil && payload != nil {
			// Keep the complete API error envelope as structured content. In
			// particular, optimistic-concurrency errors carry the latest version
			// ID in error.details so an agent can recover safely.
			result := mcp.NewToolResultStructured(payload, string(response.body))
			result.IsError = true
			return result, nil
		}
		message := strings.TrimSpace(string(response.body))
		if message == "" {
			message = http.StatusText(response.status)
		}
		return mcp.NewToolResultError(fmt.Sprintf("HTTP %d: %s", response.status, message)), nil
	}
	if len(response.body) == 0 {
		return mcp.NewToolResultJSON(map[string]any{"ok": true})
	}
	if plainText || !strings.Contains(response.contentType, "application/json") {
		return mcp.NewToolResultText(string(response.body)), nil
	}
	var value any
	if err := json.Unmarshal(response.body, &value); err != nil {
		return mcp.NewToolResultError("could not decode server response: " + err.Error()), nil
	}
	return mcp.NewToolResultJSON(value)
}

func mcpParams(values ...string) gin.Params {
	params := make(gin.Params, 0, len(values)/2)
	for i := 0; i+1 < len(values); i += 2 {
		params = append(params, gin.Param{Key: values[i], Value: values[i+1]})
	}
	return params
}

func mcpRequired(request mcp.CallToolRequest, name string) (string, *mcp.CallToolResult) {
	value, err := request.RequireString(name)
	if err != nil {
		return "", mcp.NewToolResultError(err.Error())
	}
	return value, nil
}

func readOnlyOptions(options ...mcp.ToolOption) []mcp.ToolOption {
	return append(options,
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)
}

func mutationOptions(destructive, idempotent bool, options ...mcp.ToolOption) []mcp.ToolOption {
	return append(options,
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(destructive),
		mcp.WithIdempotentHintAnnotation(idempotent),
		mcp.WithOpenWorldHintAnnotation(false),
	)
}

func registerMCPTools(server *mcpserver.MCPServer, api mcpAPIAdapter) {
	registerMCPGeneralTools(server, api)
	registerMCPDocumentTools(server, api)
	registerMCPFolderTools(server, api)
	registerMCPServiceAccountTools(server, api)
}
