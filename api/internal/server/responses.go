package server

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

// errorBody is the payload of every error response.
type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// errorEnvelope wraps errorBody under the "error" key; all error responses
// share this shape.
type errorEnvelope struct {
	Error errorBody `json:"error"`
}

func notFound(c *gin.Context) {
	c.JSON(http.StatusNotFound, errorResponse("not_found", "not found", nil))
}

func badRequest(c *gin.Context, msg string) {
	// A body the handler could not decode because it ran past the route's cap
	// is a 413, not a 400. Every handler reports an undecodable body through
	// here, so the translation happens once instead of in every handler.
	if limit, tooLarge := requestBodyLimitExceeded(c); tooLarge {
		payloadTooLarge(c, limit.message)
		return
	}
	c.JSON(http.StatusBadRequest, errorResponse("bad_request", msg, nil))
}

// payloadTooLarge reports a body the server refuses to read in full.
func payloadTooLarge(c *gin.Context, msg string) {
	c.JSON(http.StatusRequestEntityTooLarge, errorResponse("payload_too_large", msg, nil))
}

func forbidden(c *gin.Context, msg string) {
	c.JSON(http.StatusForbidden, errorResponse("forbidden", msg, nil))
}

func featureDisabled(c *gin.Context, message string) {
	c.JSON(http.StatusNotFound, errorResponse("feature_disabled", message, nil))
}

// internalErr logs err (if non-nil) and writes a 500 response.
func internalErr(c *gin.Context, err error) {
	if err != nil {
		requestLogger(c).Error("internal error", slog.Any("error", err))
	} else {
		requestLogger(c).Error("internal error")
	}
	c.JSON(http.StatusInternalServerError, errorResponse("internal", "internal server error", nil))
}

func errorResponse(code, message string, details any) errorEnvelope {
	return errorEnvelope{Error: errorBody{Code: code, Message: message, Details: details}}
}
