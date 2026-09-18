package server

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Request bodies are capped before anything parses them, so an over-sized (or
// endless) body costs the server at most one cap's worth of reading instead of
// being buffered in memory and spooled to disk first.
const (
	// maxJSONRequestBytes caps every route that is neither a document upload
	// nor /mcp. The largest body those routes legitimately carry is a document
	// KV patch, which patchDocumentKV caps at maxKVPatchBytes (256 KiB);
	// comment, label, grant, folder and service-account payloads are far
	// smaller. 1 MiB keeps four times that headroom while staying small enough
	// that an unauthenticated caller (POST /v1/auth/cli/exchange) cannot stream
	// anything meaningful into a JSON decoder.
	maxJSONRequestBytes int64 = 1 << 20

	// maxMultipartFramingBytes is what an upload gets on top of maxHTMLBytes for
	// the multipart boundaries, part headers, and the sibling form fields
	// (title, default_permission, base_version_id, change_summary).
	maxMultipartFramingBytes int64 = 1 << 20

	// maxUploadRequestBytes caps a whole multipart upload request. It is also
	// the memory allowance handed to ParseMultipartForm: no part of a request
	// this size can be larger than the request itself, so an in-limit upload is
	// always parsed in memory and never spooled to os.TempDir() — deployments
	// run with a read-only root filesystem and no writable temp volume.
	maxUploadRequestBytes = maxHTMLBytes + maxMultipartFramingBytes
)

// keepUploadsInMemory sets the engine's multipart parse allowance to the upload
// cap. Multipart parsing keeps a part in memory up to that allowance and spools
// the rest to os.TempDir(); an upload request is capped at the same value, so
// no part can exceed it and an in-limit upload never touches disk. The router
// and the in-process engine the MCP adapter replays uploads through both call
// this, so the two cannot drift.
func keepUploadsInMemory(engine *gin.Engine) {
	engine.MaxMultipartMemory = maxUploadRequestBytes
}

// htmlTooLargeMessage is what an over-sized upload has always reported. The
// body cap must not change what a client sees, so the cap and the upload
// handlers share this one string.
const htmlTooLargeMessage = "HTML file exceeds 50 MiB"

// requestBodyLimit is a cap plus the 413 message the capped route answers with.
type requestBodyLimit struct {
	bytes   int64
	message string
}

var (
	jsonRequestBodyLimit   = requestBodyLimit{bytes: maxJSONRequestBytes, message: "request body exceeds 1 MiB"}
	uploadRequestBodyLimit = requestBodyLimit{bytes: maxUploadRequestBytes, message: htmlTooLargeMessage}
)

// mcpRoutePath and uploadRoutePaths are gin full-path spellings of routes
// registered in routes(). TestUploadRejectsOversizedBodyBeforeParsingIt and
// TestMCPTransportIsExemptFromTheJSONBodyCap fail if a route moves away from
// them.
const mcpRoutePath = "/mcp"

var uploadRoutePaths = map[string]struct{}{
	"/v1/documents":              {},
	"/v1/documents/:id/versions": {},
}

// requestBodyLimitContextKey holds the *boundedRequestBody installed for the
// current request.
const requestBodyLimitContextKey = "request_body_limit"

// boundedRequestBody caps the request body and remembers that the cap was hit.
// Handlers report a body they cannot decode as a 400 in their own wording, so
// recording the overflow here is what lets badRequest answer 413 instead
// without every handler having to know about the cap.
type boundedRequestBody struct {
	io.ReadCloser
	limit    requestBodyLimit
	exceeded bool
}

func (b *boundedRequestBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		b.exceeded = true
	}
	return n, err
}

// limitRequestBodies caps the body of every routed request before a handler can
// read it. It is registered once on the engine so a new handler is bounded by
// default instead of having to remember http.MaxBytesReader.
func limitRequestBodies() gin.HandlerFunc {
	return func(c *gin.Context) {
		limit, capped := requestBodyLimitForRoute(c.FullPath(), c.Request.Method)
		if !capped {
			c.Next()
			return
		}
		// A declared Content-Length over the cap is refused before a single
		// body byte is read.
		if c.Request.ContentLength > limit.bytes {
			c.Abort()
			payloadTooLarge(c, limit.message)
			return
		}
		limitRequestBody(c, limit)
		c.Next()
	}
}

// requestBodyLimitForRoute picks the cap for one route. /mcp is exempt because
// its transport applies the much larger maxMCPRequestBytes: it carries document
// HTML inside JSON-RPC. The two multipart upload routes get the upload cap;
// everything else gets the JSON cap.
func requestBodyLimitForRoute(fullPath, method string) (requestBodyLimit, bool) {
	if fullPath == mcpRoutePath {
		return requestBodyLimit{}, false
	}
	if _, upload := uploadRoutePaths[fullPath]; upload && method == http.MethodPost {
		return uploadRequestBodyLimit, true
	}
	return jsonRequestBodyLimit, true
}

// limitRequestBody caps c.Request.Body and records the cap on the context. It
// is idempotent: the first cap installed for a request wins, so an in-process
// caller can install one defensively without overriding the router's choice.
// A handler that wants a smaller cap still wraps the body itself; that reader
// sits inside this one and reports its own error first.
func limitRequestBody(c *gin.Context, limit requestBodyLimit) {
	if c.Request == nil || c.Request.Body == nil || c.Request.Body == http.NoBody {
		return
	}
	if _, installed := c.Get(requestBodyLimitContextKey); installed {
		return
	}
	body := &boundedRequestBody{
		ReadCloser: http.MaxBytesReader(unwrapResponseWriter(c.Writer), c.Request.Body, limit.bytes),
		limit:      limit,
	}
	c.Request.Body = body
	c.Set(requestBodyLimitContextKey, body)
}

// requestBodyLimitExceeded reports the cap a request body ran past, if it did.
func requestBodyLimitExceeded(c *gin.Context) (requestBodyLimit, bool) {
	value, installed := c.Get(requestBodyLimitContextKey)
	if !installed {
		return requestBodyLimit{}, false
	}
	body, ok := value.(*boundedRequestBody)
	if !ok || !body.exceeded {
		return requestBodyLimit{}, false
	}
	return body.limit, true
}

// unwrapResponseWriter returns net/http's own ResponseWriter. The hook
// http.MaxBytesReader uses to stop reading an over-sized body and close the
// connection instead of draining it is unexported, so it is invisible through
// gin's wrapper and only fires when the wrapped writer is handed over.
func unwrapResponseWriter(writer gin.ResponseWriter) http.ResponseWriter {
	if unwrapper, ok := writer.(interface{ Unwrap() http.ResponseWriter }); ok {
		return unwrapper.Unwrap()
	}
	return writer
}
