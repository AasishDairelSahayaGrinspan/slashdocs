package server

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/razorpay/aidocs/api/internal/repo"
)

// createList godoc
// @Summary Create list
// @Tags lists
// @Security bearerAuth
// @Security cookieAuth
// @Accept json
// @Produce json
// @Param body body object true "List name"
// @Success 201 {object} repo.List
// @Router /v1/lists [post]
func (h handlers) createList(c *gin.Context) {
	var in struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || in.Name == "" {
		badRequest(c, "name is required")
		return
	}
	l, err := h.deps.repository.CreateList(c.Request.Context(), *current(c), in.Name)
	if err != nil {
		internalErr(c, err)
		return
	}
	c.JSON(http.StatusCreated, l)
}

// listLists godoc
// @Summary List lists
// @Tags lists
// @Security bearerAuth
// @Security cookieAuth
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /v1/lists [get]
func (h handlers) listLists(c *gin.Context) {
	items, err := h.deps.repository.ListLists(c.Request.Context(), *current(c))
	if err != nil {
		internalErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// getList godoc
// @Summary Get list
// @Tags lists
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "List ID"
// @Success 200 {object} repo.List
// @Router /v1/lists/{id} [get]
func (h handlers) getList(c *gin.Context) {
	l, ok := h.requireListOwner(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, l)
}

// updateList godoc
// @Summary Update list
// @Tags lists
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "List ID"
// @Success 200 {object} repo.List
// @Router /v1/lists/{id} [patch]
func (h handlers) updateList(c *gin.Context) {
	if _, ok := h.requireListOwner(c); !ok {
		return
	}
	var in struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || in.Name == "" {
		badRequest(c, "name is required")
		return
	}
	l, err := h.deps.repository.UpdateList(c.Request.Context(), c.Param("id"), in.Name)
	if err != nil {
		internalErr(c, err)
		return
	}
	c.JSON(http.StatusOK, l)
}

// deleteList godoc
// @Summary Delete list
// @Tags lists
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "List ID"
// @Success 204
// @Router /v1/lists/{id} [delete]
func (h handlers) deleteList(c *gin.Context) {
	if _, ok := h.requireListOwner(c); !ok {
		return
	}
	if err := h.deps.repository.DeleteList(c.Request.Context(), c.Param("id")); err != nil {
		internalErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// listDocumentsByList godoc
// @Summary List documents in a list
// @Tags lists
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "List ID"
// @Success 200 {object} map[string]interface{}
// @Router /v1/lists/{id}/items [get]
func (h handlers) listDocumentsByList(c *gin.Context) {
	if _, ok := h.requireListOwner(c); !ok {
		return
	}
	docs, err := h.deps.repository.ListDocumentsByList(c.Request.Context(), c.Param("id"))
	if err != nil {
		internalErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": docs})
}

// addDocumentToList godoc
// @Summary Add document to list
// @Tags lists
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "List ID"
// @Accept json
// @Success 204
// @Router /v1/lists/{id}/items [post]
func (h handlers) addDocumentToList(c *gin.Context) {
	if _, ok := h.requireListOwner(c); !ok {
		return
	}
	var in struct {
		DocumentID string `json:"document_id"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || in.DocumentID == "" {
		badRequest(c, "document_id is required")
		return
	}
	// Verify caller can at least view the document before adding it.
	role, err := h.deps.repository.RoleForDocument(c.Request.Context(), *current(c), in.DocumentID)
	if err != nil || !atLeast(role, repo.RoleViewer) {
		forbidden(c, "viewer role required on document")
		return
	}
	if err := h.deps.repository.AddDocumentToList(c.Request.Context(), c.Param("id"), in.DocumentID); err != nil {
		internalErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// removeDocumentFromList godoc
// @Summary Remove document from list
// @Tags lists
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "List ID"
// @Param doc_id path string true "Document ID"
// @Success 204
// @Router /v1/lists/{id}/items/{doc_id} [delete]
func (h handlers) removeDocumentFromList(c *gin.Context) {
	if _, ok := h.requireListOwner(c); !ok {
		return
	}
	if err := h.deps.repository.RemoveDocumentFromList(c.Request.Context(), c.Param("id"), c.Param("doc_id")); err != nil {
		internalErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// requireListOwner fetches the list and verifies the caller is the owner.
// Returns (list, true) on success or writes an error response and returns
// (zero, false) so the caller can just `return`.
func (h handlers) requireListOwner(c *gin.Context) (repo.List, bool) {
	l, err := h.deps.repository.GetList(c.Request.Context(), c.Param("id"))
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			notFound(c)
		} else {
			internalErr(c, err)
		}
		return repo.List{}, false
	}
	p := current(c)
	if l.Owner.Type != p.Type || l.Owner.ID != p.ID {
		forbidden(c, "owner role required")
		return repo.List{}, false
	}
	return l, true
}
