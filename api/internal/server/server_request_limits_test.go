package server_test

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/razorpay/aidocs/api/internal/auth"
)

// The caps the server promises, spelled out independently of the production
// constants so widening one has to be a deliberate change to this contract.
const (
	uploadRequestCapBytes int64 = 51 << 20 // 50 MiB of HTML + 1 MiB of multipart framing
	jsonRequestCapBytes   int64 = 1 << 20

	uploadTooLargeBody = `{"error":{"code":"payload_too_large","message":"HTML file exceeds 50 MiB"}}`
	jsonTooLargeBody   = `{"error":{"code":"payload_too_large","message":"request body exceeds 1 MiB"}}`
)

// repeatingByteReader is an endless source of one byte so a test can send a
// body far larger than any cap without allocating it.
type repeatingByteReader struct{ value byte }

func (r repeatingByteReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = r.value
	}
	return len(p), nil
}

// countingReader records how much of a request body the server actually read.
type countingReader struct {
	reader io.Reader
	read   int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.reader.Read(p)
	c.read += int64(n)
	return n, err
}

// sendCounted issues one request and reports how many body bytes the server
// consumed. A negative contentLength sends the body without a declared length,
// the way a chunked request arrives.
func sendCounted(t *testing.T, h http.Handler, method, path string, body io.Reader, contentLength int64, headers map[string]string) (*httptest.ResponseRecorder, int64) {
	t.Helper()
	counter := &countingReader{reader: body}
	req := httptest.NewRequest(method, path, counter)
	req.ContentLength = contentLength
	if contentLength < 0 {
		req.TransferEncoding = []string{"chunked"}
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr, counter.read
}

// multipartUpload frames a file part of an arbitrary size around a repeating
// reader: only the framing is ever materialized. It returns the body, its
// content type, and its exact length.
func multipartUpload(fields map[string]string, fileBytes int64) (io.Reader, string, int64) {
	const boundary = "aidocsrequestlimitboundary"
	var head bytes.Buffer
	for key, value := range fields {
		fmt.Fprintf(&head, "--%s\r\nContent-Disposition: form-data; name=%q\r\n\r\n%s\r\n", boundary, key, value)
	}
	fmt.Fprintf(&head, "--%s\r\nContent-Disposition: form-data; name=\"file\"; filename=\"large.html\"\r\nContent-Type: text/html\r\n\r\n", boundary)
	tail := fmt.Sprintf("\r\n--%s--\r\n", boundary)
	body := io.MultiReader(
		bytes.NewReader(head.Bytes()),
		io.LimitReader(repeatingByteReader{value: 'x'}, fileBytes),
		strings.NewReader(tail),
	)
	return body, "multipart/form-data; boundary=" + boundary, int64(head.Len()) + fileBytes + int64(len(tail))
}

// jsonBody frames padding bytes inside a JSON object of the given total size.
func jsonBody(field string, padding int64) (io.Reader, int64) {
	head := `{"` + field + `":"`
	const tail = `"}`
	body := io.MultiReader(
		strings.NewReader(head),
		io.LimitReader(repeatingByteReader{value: 'x'}, padding),
		strings.NewReader(tail),
	)
	return body, int64(len(head)) + padding + int64(len(tail))
}

// assertTempDirEmpty fails when parsing spooled a request to disk. The
// container runs with a read-only root filesystem, so a spill is an outage.
func assertTempDirEmpty(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read temp dir %s: %v", dir, err)
	}
	if len(entries) > 0 {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("request spooled %d file(s) to %s: %v", len(entries), dir, names)
	}
}

// An upload past the cap is refused after reading at most one cap's worth of
// body, with the same 413 an over-sized HTML file has always produced, and
// without spooling anything to os.TempDir().
func TestUploadRejectsOversizedBodyBeforeParsingIt(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("TMPDIR", tempDir)
	e := newTestEnv(t)
	const fileBytes = 120 << 20

	for _, route := range []struct {
		name   string
		path   string
		fields map[string]string
	}{
		{"create document", "/v1/documents", map[string]string{"title": "huge"}},
		{"create version", "/v1/documents/" + e.doc.ID + "/versions", map[string]string{"base_version_id": e.ver.ID}},
	} {
		for _, framing := range []struct {
			name        string
			declared    bool
			maxBodyRead int64
		}{
			// A declared length over the cap is refused before the first byte.
			{"declared content length", true, 0},
			{"chunked", false, uploadRequestCapBytes + 1},
		} {
			t.Run(route.name+"/"+framing.name, func(t *testing.T) {
				body, contentType, length := multipartUpload(route.fields, fileBytes)
				if !framing.declared {
					length = -1
				}
				rr, read := sendCounted(t, e.h, http.MethodPost, route.path, body, length, map[string]string{
					"Content-Type":     contentType,
					"X-Test-Principal": principalHeader(fixtureOwner),
				})
				assertStatus(t, rr, http.StatusRequestEntityTooLarge)
				assertJSON(t, rr.Body.Bytes(), uploadTooLargeBody)
				if read > framing.maxBodyRead {
					t.Fatalf("server read %d body bytes of %d, want at most %d", read, fileBytes, framing.maxBodyRead)
				}
			})
		}
	}
	assertTempDirEmpty(t, tempDir)
}

// A 40 MiB upload is over gin's 32 MiB in-memory multipart default but under
// the documented limit, so it must parse entirely in memory: os.TempDir()
// points at a directory that does not exist, as it effectively does under a
// read-only root filesystem.
func TestUploadUnderLimitNeverSpoolsToDisk(t *testing.T) {
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "unwritable"))
	e := newTestEnv(t)
	const fileBytes = 40 << 20

	body, contentType, length := multipartUpload(map[string]string{"title": "large but allowed"}, fileBytes)
	rr, _ := sendCounted(t, e.h, http.MethodPost, "/v1/documents", body, length, map[string]string{
		"Content-Type":     contentType,
		"X-Test-Principal": principalHeader(fixtureOwner),
	})
	assertStatus(t, rr, http.StatusCreated)
	docID := jsonField(t, rr.Body.Bytes(), "id")
	versionID := jsonField(t, rr.Body.Bytes(), "current_version_id")

	body, contentType, length = multipartUpload(map[string]string{"base_version_id": versionID}, fileBytes)
	rr, _ = sendCounted(t, e.h, http.MethodPost, "/v1/documents/"+docID+"/versions", body, length, map[string]string{
		"Content-Type":     contentType,
		"X-Test-Principal": principalHeader(fixtureOwner),
	})
	assertStatus(t, rr, http.StatusCreated)
}

// Every route that reads a JSON body is capped, including the unauthenticated
// CLI exchange, and an over-sized body is a 413 rather than the 400 a failed
// bind reports.
func TestJSONRoutesRejectOversizedBodies(t *testing.T) {
	e := newTestEnv(t)
	const padding = 16 << 20

	for _, route := range []struct {
		name    string
		method  string
		path    string
		field   string
		headers map[string]string
	}{
		{
			name: "unauthenticated cli exchange", method: http.MethodPost, path: "/v1/auth/cli/exchange", field: "code",
			headers: map[string]string{"Content-Type": "application/json"},
		},
		{
			name: "authenticated document patch", method: http.MethodPatch, path: "/v1/documents/" + e.doc.ID, field: "title",
			headers: map[string]string{"Content-Type": "application/json", "X-Test-Principal": principalHeader(fixtureOwner)},
		},
	} {
		for _, framing := range []struct {
			name        string
			declared    bool
			maxBodyRead int64
		}{
			{"declared content length", true, 0},
			{"chunked", false, jsonRequestCapBytes + 1},
		} {
			t.Run(route.name+"/"+framing.name, func(t *testing.T) {
				body, length := jsonBody(route.field, padding)
				if !framing.declared {
					length = -1
				}
				rr, read := sendCounted(t, e.h, route.method, route.path, body, length, route.headers)
				assertStatus(t, rr, http.StatusRequestEntityTooLarge)
				assertJSON(t, rr.Body.Bytes(), jsonTooLargeBody)
				if read > framing.maxBodyRead {
					t.Fatalf("server read %d body bytes of %d, want at most %d", read, padding, framing.maxBodyRead)
				}
			})
		}
	}
}

// A route that sets its own, smaller limit keeps it: the KV patch still reports
// its own 413 at 256 KiB instead of the shared default at 1 MiB.
func TestRouteWithSmallerOwnLimitKeepsIt(t *testing.T) {
	e := newTestEnv(t)
	const padding = 512 << 10

	body, length := jsonBody("set", padding)
	rr, read := sendCounted(t, e.h, http.MethodPatch, "/v1/documents/"+e.doc.ID+"/kv", body, length, map[string]string{
		"Content-Type":     "application/json",
		"X-Test-Principal": principalHeader(fixtureOwner),
	})
	assertStatus(t, rr, http.StatusRequestEntityTooLarge)
	assertJSON(t, rr.Body.Bytes(), `{"error":{"code":"payload_too_large","message":"KV patch exceeds 256 KiB"}}`)
	if want := int64(256<<10) + 2; read > want {
		t.Fatalf("server read %d body bytes, want at most %d", read, want)
	}
}

// /mcp is exempt from the JSON cap: it carries document HTML inside JSON-RPC
// and enforces its own, much larger transport limit.
func TestMCPTransportIsExemptFromTheJSONBodyCap(t *testing.T) {
	e := newMCPOAuthTestEnv(t)
	token := auth.NewMCPAccessToken(fixtureUser.ID, e.resource, oauthTestSigningMaterial, time.Minute)

	html := "<html><body>" + strings.Repeat("x", 2<<20) + "</body></html>"
	created := mustMCPObject(t, assertMCPToolOK(t, callMCPToolHTTP(t, e, token, "create_document", map[string]any{
		"title": "large MCP payload", "html": html,
	}), `"id"`))
	if id := requiredMCPString(t, created, "id"); !strings.HasPrefix(id, "doc_") {
		t.Fatalf("created document id = %q, want doc_ prefix", id)
	}
}

// The MCP document tools replay the upload handlers through an in-process
// request that never passes through the router, so they carry the same 50 MiB
// limit — and must not spool to disk on the way there either.
func TestMCPDocumentToolsKeepTheUploadLimit(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("TMPDIR", tempDir)
	e := newMCPOAuthTestEnv(t)
	token := auth.NewMCPAccessToken(fixtureUser.ID, e.resource, oauthTestSigningMaterial, time.Minute)

	atLimit := documentHTMLOfSize(50 << 20)
	created := mustMCPObject(t, assertMCPToolOK(t, callMCPToolHTTP(t, e, token, "create_document", map[string]any{
		"title": "at the limit", "html": atLimit,
	}), `"id"`))
	docID := requiredMCPString(t, created, "id")
	versionID := requiredMCPString(t, created, "current_version_id")

	overLimit := documentHTMLOfSize(52 << 20)
	assertMCPToolError(t, callMCPToolHTTP(t, e, token, "create_document", map[string]any{
		"title": "over the limit", "html": overLimit,
	}), "HTML file exceeds 50 MiB")
	assertMCPToolError(t, callMCPToolHTTP(t, e, token, "update_document", map[string]any{
		"document_id": docID, "base_version_id": versionID, "html": overLimit,
	}), "HTML file exceeds 50 MiB")

	assertTempDirEmpty(t, tempDir)
}

func documentHTMLOfSize(size int) string {
	const prefix, suffix = "<html><body>", "</body></html>"
	return prefix + strings.Repeat("x", size-len(prefix)-len(suffix)) + suffix
}
