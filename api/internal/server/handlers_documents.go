package server

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/razorpay/aidocs/api/internal/blob"
	"github.com/razorpay/aidocs/api/internal/repo"
)


// CreateDocument godoc
// @Summary Create document
// @Tags documents
// @Security bearerAuth
// @Security cookieAuth
// @Accept multipart/form-data
// @Produce json
// @Param title formData string true "Document title"
// @Param file formData file true "Single-file HTML"
// @Success 201 {object} map[string]interface{}
// @Router /v1/documents [post]
func (h handlers) createDocument(c *gin.Context) {
	p := current(c)
	title := c.PostForm("title")
	html, err := readMultipartFile(c, "file")
	if errors.Is(err, errPayloadTooLarge) {
		c.JSON(http.StatusRequestEntityTooLarge, errorResponse("payload_too_large", "HTML file exceeds 10 MiB", nil))
		return
	}
	if err != nil || title == "" {
		badRequest(c, "invalid document upload")
		return
	}
	observeHTML("document_create", len(html))
	d, _, err := h.deps.repository.CreateDocument(c.Request.Context(), *p, title, html)
	if err != nil {
		if errors.Is(err, blob.ErrStorage) {
			c.JSON(http.StatusBadGateway, errorResponse("blob_storage_failed", "could not upload HTML to blob storage", nil))
			return
		}
		internalErr(c, err)
		return
	}
	incDocument("created", actorType(c))
	incVersion("created_initial", actorType(c))
	c.JSON(http.StatusCreated, gin.H{"id": d.ID, "current_version_id": d.CurrentVersionID})
}

// ListDocuments godoc
// @Summary List documents
// @Tags documents
// @Security bearerAuth
// @Security cookieAuth
// @Produce json
// @Param q query string false "Title search query"
// @Success 200 {object} map[string]interface{}
// @Router /v1/documents [get]
func (h handlers) listDocuments(c *gin.Context) {
	q := c.Query("q")
	if q != "" {
		includeOrgWide := h.deps.deployment == DeploymentOrg
		items, err := h.deps.repository.SearchDocumentsByTitle(c.Request.Context(), *current(c), q, includeOrgWide, 25)
		if err != nil {
			internalErr(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": items, "has_more": false})
		return
	}

	limit := docPageDefault
	if l, err := strconv.Atoi(c.DefaultQuery("limit", "")); err == nil && l > 0 {
		if l > docPageMax {
			l = docPageMax
		}
		limit = l
	}
	offset := 0
	if o, err := strconv.Atoi(c.DefaultQuery("offset", "")); err == nil && o > 0 {
		offset = o
	}

	all, err := h.deps.repository.ListDocuments(c.Request.Context(), *current(c))
	if err != nil {
		internalErr(c, err)
		return
	}

	total := len(all)
	if offset >= total {
		c.JSON(http.StatusOK, gin.H{"items": []repo.Document{}, "has_more": false})
		return
	}
	end := offset + limit
	hasMore := end < total
	if end > total {
		end = total
	}
	c.JSON(http.StatusOK, gin.H{"items": all[offset:end], "has_more": hasMore})
}

// GetDocument godoc
// @Summary Get document
// @Tags documents
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Document ID"
// @Success 200 {object} repo.Document
// @Router /v1/documents/{id} [get]
func (h handlers) getDocument(c *gin.Context) {
	d, err := h.deps.repository.GetDocument(c.Request.Context(), c.Param("id"))
	if err != nil {
		notFound(c)
		return
	}
	role, _ := h.deps.repository.RoleForDocument(c.Request.Context(), *current(c), d.ID)
	if !atLeast(role, repo.RoleViewer) {
		forbidden(c, "viewer role required")
		return
	}
	p := current(c)
	if !(p.Type == d.Owner.Type && p.ID == d.Owner.ID) {
		_ = h.deps.repository.RecordDocumentOpened(c.Request.Context(), d.ID, *p)
	}
	// Show parent folder in breadcrumb only if viewer has access to it.
	if d.ParentFolderID != "" {
		if r, _ := h.deps.repository.RoleForFolder(c.Request.Context(), *p, d.ParentFolderID); atLeast(r, repo.RoleViewer) {
			f, err := h.deps.repository.GetFolder(c.Request.Context(), d.ParentFolderID)
			if err == nil {
				d.ParentFolder = &repo.FolderBreadcrumb{ID: f.ID, Title: f.Title}
			}
		}
	}
	c.JSON(http.StatusOK, d)
}

// PatchDocument godoc
// @Summary Update document
// @Tags documents
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Document ID"
// @Success 200 {object} repo.Document
// @Router /v1/documents/{id} [patch]
func (h handlers) patchDocument(c *gin.Context) {
	if !h.needDocRole(c, c.Param("id"), repo.RoleOwner) {
		return
	}
	var in struct{ Title string }
	if err := c.ShouldBindJSON(&in); err != nil {
		badRequest(c, "invalid body")
		return
	}
	if in.Title == "" {
		badRequest(c, "nothing to update; provide a title")
		return
	}
	d, err := h.deps.repository.UpdateDocument(c.Request.Context(), c.Param("id"), in.Title)
	if err != nil {
		internalErr(c, err)
		return
	}
	c.JSON(http.StatusOK, d)
}

// DeleteDocument godoc
// @Summary Delete document
// @Tags documents
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Document ID"
// @Success 204
// @Router /v1/documents/{id} [delete]
func (h handlers) deleteDocument(c *gin.Context) {
	if !h.needDocRole(c, c.Param("id"), repo.RoleOwner) {
		return
	}
	if err := h.deps.repository.DeleteDocument(c.Request.Context(), c.Param("id")); err != nil {
		internalErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
