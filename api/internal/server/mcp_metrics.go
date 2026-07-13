package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"

	mcpserver "github.com/mark3labs/mcp-go/server"
)

const maxMCPMetricResponseBytes = 128 * 1024

var mcpMetricMethods = map[string]string{
	"initialize":                "initialize",
	"notifications/initialized": "notifications_initialized",
	"ping":                      "ping",
	"tools/list":                "tools_list",
	"tools/call":                "tools_call",
}

type mcpMetricRequest struct {
	Method string `json:"method"`
	Params struct {
		Name string `json:"name"`
	} `json:"params"`
}

func mcpRequestMetricLabels(body []byte, tools map[string]*mcpserver.ServerTool) (method, tool string) {
	var request mcpMetricRequest
	if json.Unmarshal(body, &request) != nil {
		return "unknown", ""
	}
	method = mcpMetricMethods[request.Method]
	if method == "" {
		method = "unknown"
	}
	if request.Method != "tools/call" {
		return method, ""
	}
	if _, ok := tools[request.Params.Name]; ok {
		return method, request.Params.Name
	}
	return method, "unknown"
}

type mcpResponseCapture struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func newMCPResponseCapture(w http.ResponseWriter) *mcpResponseCapture {
	return &mcpResponseCapture{ResponseWriter: w}
}

func (w *mcpResponseCapture) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *mcpResponseCapture) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	remaining := maxMCPMetricResponseBytes - w.body.Len()
	if remaining > 0 {
		if len(p) < remaining {
			remaining = len(p)
		}
		_, _ = w.body.Write(p[:remaining])
	}
	return w.ResponseWriter.Write(p)
}

func (w *mcpResponseCapture) Status() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

func (w *mcpResponseCapture) Body() []byte { return w.body.Bytes() }

func mcpResponseMetricOutcomes(status int, body []byte) (request, tool string) {
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return "transport_error", "transport_error"
	}
	var response struct {
		Error  json.RawMessage `json:"error"`
		Result struct {
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) > 0 && json.Unmarshal(trimmed, &response) == nil {
		if len(response.Error) > 0 && !strings.EqualFold(string(response.Error), "null") {
			return "protocol_error", "protocol_error"
		}
		if response.Result.IsError {
			return "success", "tool_error"
		}
	}
	return "success", "success"
}
