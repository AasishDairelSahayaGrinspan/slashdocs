package server

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type failingMCPReadCloser struct{ err error }

func (r failingMCPReadCloser) Read([]byte) (int, error) { return 0, r.err }
func (failingMCPReadCloser) Close() error               { return nil }

func TestMCPRequestBodyEnforcesLimitWithoutSecondFullBuffer(t *testing.T) {
	body := newMCPRequestBody(io.NopCloser(strings.NewReader("abcdef")), 4)
	got, err := io.ReadAll(body)
	var maxErr *http.MaxBytesError
	if !errors.As(err, &maxErr) {
		t.Fatalf("ReadAll error = %v, want MaxBytesError", err)
	}
	if string(got) != "abcd" {
		t.Fatalf("ReadAll body = %q, want %q", got, "abcd")
	}
	if !body.tooLarge {
		t.Fatal("request body did not record the limit violation")
	}

	recorder := httptest.NewRecorder()
	response := newMCPResponseWriter(recorder, body)
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusBadRequest)
	if _, err := response.Write([]byte(`{"jsonrpc":"2.0"}`)); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("response status = %d, want %d", recorder.Code, http.StatusRequestEntityTooLarge)
	}
	if strings.Contains(recorder.Body.String(), "jsonrpc") {
		t.Fatalf("oversize response leaked MCP parse error: %s", recorder.Body.String())
	}
}

func TestMCPRequestBodyPreservesReadFailureResponse(t *testing.T) {
	readFailure := errors.New("fixture read failure")
	body := newMCPRequestBody(failingMCPReadCloser{err: readFailure}, 1024)
	if _, err := io.ReadAll(body); !errors.Is(err, readFailure) {
		t.Fatalf("ReadAll error = %v, want %v", err, readFailure)
	}
	if !errors.Is(body.readErr, readFailure) {
		t.Fatalf("recorded read error = %v, want %v", body.readErr, readFailure)
	}

	recorder := httptest.NewRecorder()
	response := newMCPResponseWriter(recorder, body)
	response.WriteHeader(http.StatusBadRequest)
	if _, err := response.Write([]byte(`{"jsonrpc":"2.0"}`)); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("response status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if body := recorder.Body.String(); body != "could not read MCP request body\n" {
		t.Fatalf("read failure response = %q", body)
	}
}

func TestMCPRequestBodyTreatsWrappedEOFAsReadFailure(t *testing.T) {
	wrappedEOF := fmt.Errorf("wrapped EOF: %w", io.EOF)
	body := newMCPRequestBody(failingMCPReadCloser{err: wrappedEOF}, 1024)
	if _, err := io.ReadAll(body); err != wrappedEOF {
		t.Fatalf("ReadAll error = %v, want %v", err, wrappedEOF)
	}
	if body.readErr != wrappedEOF {
		t.Fatalf("recorded read error = %v, want %v", body.readErr, wrappedEOF)
	}
}

func TestMCPRequestBodyProbeLabelsTruncatedMessage(t *testing.T) {
	source := `{"jsonrpc":"2.0","method":"tools/call","params":{"payload":"` + strings.Repeat("x", 200)
	body := newMCPRequestBody(io.NopCloser(strings.NewReader(source)), 100)
	_, _ = io.ReadAll(body)
	if got := body.metricMethod(); got != "tools_call" {
		t.Fatalf("metric method = %q, want tools_call", got)
	}
	if body.probe.Len() > maxMCPMetricProbeBytes {
		t.Fatalf("metric probe retained %d bytes, limit %d", body.probe.Len(), maxMCPMetricProbeBytes)
	}
}
