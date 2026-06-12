package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ListPins godoc
// @Summary List pinned resources
// @Tags pins
// @Security bearerAuth
// @Security cookieAuth
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /v1/pins [get]
func (h handlers) listPins(c *gin.Context) {
	items, err := h.deps.repository.ListPins(c.Request.Context(), *current(c))
	if err != nil {
		internalErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// PinResource godoc
// @Summary Pin a resource
// @Tags pins
// @Security bearerAuth
// @Security cookieAuth
// @Param resource_type path string true "Resource type: document or folder"
// @Param resource_id path string true "Resource ID"
// @Success 204
// @Router /v1/pins/{resource_type}/{resource_id} [put]
func (h handlers) pinResource(c *gin.Context) {
	rt := c.Param("resource_type")
	if rt != "document" && rt != "folder" {
		badRequest(c, "resource_type must be 'document' or 'folder'")
		return
	}
	if err := h.deps.repository.PinResource(c.Request.Context(), *current(c), rt, c.Param("resource_id")); err != nil {
		internalErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// UnpinResource godoc
// @Summary Unpin a resource
// @Tags pins
// @Security bearerAuth
// @Security cookieAuth
// @Param resource_type path string true "Resource type: document or folder"
// @Param resource_id path string true "Resource ID"
// @Success 204
// @Router /v1/pins/{resource_type}/{resource_id} [delete]
func (h handlers) unpinResource(c *gin.Context) {
	rt := c.Param("resource_type")
	if rt != "document" && rt != "folder" {
		badRequest(c, "resource_type must be 'document' or 'folder'")
		return
	}
	if err := h.deps.repository.UnpinResource(c.Request.Context(), *current(c), rt, c.Param("resource_id")); err != nil {
		internalErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
