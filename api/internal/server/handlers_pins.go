package server

import (
	"github.com/gin-gonic/gin"
	"github.com/razorpay/aidocs/api/internal/repo"
	"net/http"
)

func (h handlers) listPins(c *gin.Context) {
	items, err := h.deps.repository.ListPins(c.Request.Context(), *current(c))
	if err != nil {
		internalErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}
func (h handlers) putPin(c *gin.Context) {
	rt, rid := c.Param("resource_type"), c.Param("resource_id")
	if rt == "document" {
		if !h.needDocRole(c, rid, repo.RoleViewer) {
			return
		}
	} else if rt == "folder" {
		if !h.needFolderRole(c, rid, repo.RoleViewer) {
			return
		}
	} else {
		badRequest(c, "resource_type must be document or folder")
		return
	}
	if err := h.deps.repository.PutPin(c.Request.Context(), *current(c), rt, rid); err != nil {
		internalErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
func (h handlers) deletePin(c *gin.Context) {
	if err := h.deps.repository.DeletePin(c.Request.Context(), *current(c), c.Param("resource_type"), c.Param("resource_id")); err != nil {
		internalErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
