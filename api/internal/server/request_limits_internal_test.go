package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// The upload cap has to admit a maxHTMLBytes file plus its framing, and the
// shared JSON cap has to stay above the largest limit a handler sets for
// itself, or a legitimate request would be rejected by the shared cap first.
func TestRequestBodyCapsAdmitTheLargestLegitimateBodies(t *testing.T) {
	if maxUploadRequestBytes <= maxHTMLBytes {
		t.Fatalf("upload cap %d leaves no room for multipart framing around a %d byte file", maxUploadRequestBytes, maxHTMLBytes)
	}
	if maxJSONRequestBytes <= maxKVPatchBytes {
		t.Fatalf("JSON cap %d is not above the KV patch limit %d", maxJSONRequestBytes, maxKVPatchBytes)
	}
}

func TestRequestBodyLimitForRoute(t *testing.T) {
	for _, test := range []struct {
		name     string
		fullPath string
		method   string
		want     requestBodyLimit
		capped   bool
	}{
		{"document upload", "/v1/documents", http.MethodPost, uploadRequestBodyLimit, true},
		{"version upload", "/v1/documents/:id/versions", http.MethodPost, uploadRequestBodyLimit, true},
		{"document list", "/v1/documents", http.MethodGet, jsonRequestBodyLimit, true},
		{"unauthenticated exchange", "/v1/auth/cli/exchange", http.MethodPost, jsonRequestBodyLimit, true},
		{"unmatched route", "", http.MethodPost, jsonRequestBodyLimit, true},
		{"MCP transport", mcpRoutePath, http.MethodPost, requestBodyLimit{}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			limit, capped := requestBodyLimitForRoute(test.fullPath, test.method)
			if capped != test.capped || limit != test.want {
				t.Fatalf("requestBodyLimitForRoute(%q, %s) = %+v, %t; want %+v, %t", test.fullPath, test.method, limit, capped, test.want, test.capped)
			}
		})
	}
}

// http.MaxBytesReader only stops the server reading the rest of an over-sized
// body, and closes the connection, when it is handed net/http's own
// ResponseWriter: the hook is unexported, so gin's wrapper has to come off.
func TestUnwrapResponseWriterReturnsTheWrappedWriter(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	if got := unwrapResponseWriter(c.Writer); got != http.ResponseWriter(recorder) {
		t.Fatalf("unwrapped writer = %#v, want the recorder gin wraps", got)
	}
}
