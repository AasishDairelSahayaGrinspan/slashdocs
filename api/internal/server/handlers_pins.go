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
	ok := false
	if rt == "document" {
		role, _ := h.deps.repository.RoleForDocument(c.Request.Context(), *current(c), rid)
		ok = atLeast(role, repo.RoleViewer)
	}
	if rt == "folder" {
		role, _ := h.deps.repository.RoleForFolder(c.Request.Context(), *current(c), rid)
		ok = atLeast(role, repo.RoleViewer)
	}
	if !ok {
		forbidden(c, "viewer role required")
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
