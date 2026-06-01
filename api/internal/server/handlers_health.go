package server

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func (h handlers) livez(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h handlers) readyz(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	checks := make(map[string]string, len(h.deps.readinessChecks))
	ready := true
	for _, check := range h.deps.readinessChecks {
		if err := check.fn(ctx); err != nil {
			checks[check.name] = "error"
			ready = false
			continue
		}
		checks[check.name] = "ok"
	}
	if !ready {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "error", "checks": checks})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok", "checks": checks})
}
