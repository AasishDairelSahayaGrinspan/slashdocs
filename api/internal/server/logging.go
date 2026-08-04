package server

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// requestIDKey is the gin context key under which the request ID is stored.
const requestIDKey = "request_id"

// requestIDHeader is the canonical request-ID header, accepted inbound and
// echoed on every response so a request can be correlated across proxies,
// server logs, and clients.
const requestIDHeader = "X-Request-ID"

// maxRequestIDLength caps inbound request IDs so a hostile client cannot
// inflate log lines with an arbitrarily long header value.
const maxRequestIDLength = 128

// requestIDMiddleware assigns each request an ID: the inbound X-Request-ID
// when present (trusted deployments put a proxy in front that sets it), a
// freshly generated one otherwise. The ID is stored on the context for log
// correlation and echoed in the response header.
func requestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(requestIDHeader)
		if id == "" || len(id) > maxRequestIDLength {
			id = newRequestID()
		}
		c.Set(requestIDKey, id)
		c.Writer.Header().Set(requestIDHeader, id)
		c.Next()
	}
}

func newRequestID() string {
	var b [8]byte
	rand.Read(b[:]) // never fails (crypto/rand.Read panics internally instead)
	return hex.EncodeToString(b[:])
}

// requestID returns the ID assigned by requestIDMiddleware, or "" outside it.
func requestID(c *gin.Context) string {
	return c.GetString(requestIDKey)
}

// requestLogger returns the process logger annotated with the request's
// correlation attributes.
func requestLogger(c *gin.Context) *slog.Logger {
	return slog.Default().With(
		slog.String("request_id", requestID(c)),
		slog.String("method", c.Request.Method),
		slog.String("path", c.Request.URL.Path),
	)
}

// accessLogMiddleware emits one structured log line per request, replacing
// gin's default access logger so the server speaks a single logging dialect.
func accessLogMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		requestLogger(c).Info("request",
			slog.Int("status", c.Writer.Status()),
			slog.String("client_ip", c.ClientIP()),
			slog.Duration("duration", time.Since(start)),
		)
	}
}

// recoveryMiddleware converts a handler panic into a structured error log and
// a 500 response, replacing gin.Recovery's unstructured stack dump.
func recoveryMiddleware() gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(nil, func(c *gin.Context, err any) {
		requestLogger(c).Error("panic recovered", slog.Any("panic", err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, errorResponse("internal", "internal server error", nil))
	})
}
