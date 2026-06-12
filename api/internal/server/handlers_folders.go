package server

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/razorpay/aidocs/api/internal/auth"
	"github.com/razorpay/aidocs/api/internal/repo"
)

// CreateFolder godoc
// @Summary Create folder
// @Tags folders
// @Security bearerAuth
// @Security cookieAuth
// @Produce json
// @Success 201 {object} repo.Folder
// @Router /v1/folders [post]
func (h handlers) createFolder(c *gin.Context) {
	p := current(c)
	var in struct{ Title string }
	if err := c.ShouldBindJSON(&in); err != nil || in.Title == "" {
		badRequest(c, "title is required")
		return
	}
	f, err := h.deps.repository.CreateFolder(c.Request.Context(), *p, in.Title)
	if err != nil {
		internalErr(c, err)
		return
	}
	c.JSON(http.StatusCreated, f)
}

// ListFolders godoc
// @Summary List folders
// @Tags folders
// @Security bearerAuth
// @Security cookieAuth
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /v1/folders [get]
func (h handlers) listFolders(c *gin.Context) {
	items, err := h.deps.repository.ListFolders(c.Request.Context(), *current(c))
	if err != nil {
		internalErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// GetFolder godoc
// @Summary Get folder
// @Tags folders
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Folder ID"
// @Success 200 {object} repo.Folder
// @Router /v1/folders/{id} [get]
func (h handlers) getFolder(c *gin.Context) {
	f, err := h.deps.repository.GetFolder(c.Request.Context(), c.Param("id"))
	if err != nil {
		notFound(c)
		return
	}
	role, _ := h.deps.repository.RoleForFolder(c.Request.Context(), *current(c), f.ID)
	if !atLeast(role, repo.RoleViewer) {
		forbidden(c, "viewer role required")
		return
	}
	c.JSON(http.StatusOK, f)
}

// PatchFolder godoc
// @Summary Update folder
// @Tags folders
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Folder ID"
// @Success 200 {object} repo.Folder
// @Router /v1/folders/{id} [patch]
func (h handlers) patchFolder(c *gin.Context) {
	if !h.needFolderRole(c, c.Param("id"), repo.RoleOwner) {
		return
	}
	var in struct{ Title string }
	if err := c.ShouldBindJSON(&in); err != nil || in.Title == "" {
		badRequest(c, "title is required")
		return
	}
	f, err := h.deps.repository.UpdateFolder(c.Request.Context(), c.Param("id"), in.Title)
	if err != nil {
		internalErr(c, err)
		return
	}
	c.JSON(http.StatusOK, f)
}

// DeleteFolder godoc
// @Summary Delete folder
// @Tags folders
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Folder ID"
// @Success 204
// @Router /v1/folders/{id} [delete]
func (h handlers) deleteFolder(c *gin.Context) {
	if !h.needFolderRole(c, c.Param("id"), repo.RoleOwner) {
		return
	}
	if err := h.deps.repository.DeleteFolder(c.Request.Context(), c.Param("id")); err != nil {
		internalErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// CreateFolderGrant godoc
// @Summary Create folder grant
// @Tags folders
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Folder ID"
// @Success 201 {object} map[string]interface{}
// @Router /v1/folders/{id}/grants [post]
func (h handlers) createFolderGrant(c *gin.Context) {
	if !h.needFolderRole(c, c.Param("id"), repo.RoleOwner) {
		return
	}
	p := current(c)
	var in struct {
		Address   string `json:"address"`
		Principal struct {
			Type  auth.PrincipalType `json:"type"`
			ID    string             `json:"id"`
			Email string             `json:"email"`
			Name  string             `json:"name"`
		} `json:"principal"`
		Role repo.Role `json:"role"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		badRequest(c, "invalid grant")
		return
	}
	if !validGrantRole(in.Role) {
		badRequest(c, "invalid role")
		return
	}
	principal := auth.Principal{Type: in.Principal.Type, ID: in.Principal.ID, Email: in.Principal.Email, Name: in.Principal.Name}
	resolved, err := resolveGrantPrincipal(c, h.deps.repository, in.Address, principal)
	if err != nil {
		switch {
		case errors.Is(err, errInvalidAddress):
			badRequest(c, "That doesn't look like an email or bot address.")
		case errors.Is(err, errPrincipalIDRequired):
			badRequest(c, "principal id, user email, or service account name is required")
		case errors.Is(err, errUnknownUserEmail):
			badRequest(c, "No aidocs user exists for that email. Ask them to sign in once, then try again.")
		case errors.Is(err, repo.ErrNotFound):
			notFound(c)
		default:
			internalErr(c, err)
		}
		return
	}
	g, err := h.deps.repository.CreateFolderGrant(c.Request.Context(), c.Param("id"), resolved, in.Role, *p)
	if err != nil {
		internalErr(c, err)
		return
	}
	pr := gin.H{"type": g.Principal.Type, "id": g.Principal.ID}
	if g.Principal.Email != "" {
		pr["email"] = g.Principal.Email
	}
	if g.Principal.Name != "" {
		pr["name"] = g.Principal.Name
	}
	c.JSON(http.StatusCreated, gin.H{"id": g.ID, "resource": gin.H{"type": "folder", "id": g.DocumentID}, "principal": pr, "role": g.Role, "granted_by": gin.H{"id": g.GrantedBy.ID, "email": g.GrantedBy.Email}})
}

// ListFolderGrants godoc
// @Summary List folder grants
// @Tags folders
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Folder ID"
// @Success 200 {object} map[string]interface{}
// @Router /v1/folders/{id}/grants [get]
func (h handlers) listFolderGrants(c *gin.Context) {
	if !h.needFolderRole(c, c.Param("id"), repo.RoleOwner) {
		return
	}
	items, err := h.deps.repository.ListFolderGrants(c.Request.Context(), c.Param("id"))
	if err != nil {
		internalErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// PatchFolderGrant godoc
// @Summary Update folder grant
// @Tags folders
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Folder ID"
// @Param grant_id path string true "Grant ID"
// @Success 200 {object} repo.Grant
// @Router /v1/folders/{id}/grants/{grant_id} [patch]
func (h handlers) patchFolderGrant(c *gin.Context) {
	if !h.needFolderRole(c, c.Param("id"), repo.RoleOwner) {
		return
	}
	var in struct {
		Role repo.Role `json:"role"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || !validGrantRole(in.Role) {
		badRequest(c, "invalid grant")
		return
	}
	g, err := h.deps.repository.UpdateFolderGrant(c.Request.Context(), c.Param("id"), c.Param("grant_id"), in.Role)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			notFound(c)
			return
		}
		internalErr(c, err)
		return
	}
	c.JSON(http.StatusOK, g)
}

// DeleteFolderGrant godoc
// @Summary Delete folder grant
// @Tags folders
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Folder ID"
// @Param grant_id path string true "Grant ID"
// @Success 204
// @Router /v1/folders/{id}/grants/{grant_id} [delete]
func (h handlers) deleteFolderGrant(c *gin.Context) {
	if !h.needFolderRole(c, c.Param("id"), repo.RoleOwner) {
		return
	}
	if err := h.deps.repository.DeleteFolderGrant(c.Request.Context(), c.Param("id"), c.Param("grant_id")); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			notFound(c)
			return
		}
		internalErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// AddDocumentToFolder godoc
// @Summary Add document to folder
// @Tags folders
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Folder ID"
// @Success 200 {object} repo.AddDocToFolderResult
// @Router /v1/folders/{id}/documents [post]
func (h handlers) addDocumentToFolder(c *gin.Context) {
	if !h.needFolderRole(c, c.Param("id"), repo.RoleEditor) {
		return
	}
	var in struct {
		DocumentID string `json:"document_id"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || in.DocumentID == "" {
		badRequest(c, "document_id is required")
		return
	}
	// Verify the caller has at least viewer access to the document.
	docRole, err := h.deps.repository.RoleForDocument(c.Request.Context(), *current(c), in.DocumentID)
	if err != nil || !atLeast(docRole, repo.RoleViewer) {
		forbidden(c, "you must have access to the document to add it to a folder")
		return
	}
	result, err := h.deps.repository.AddDocumentToFolder(c.Request.Context(), c.Param("id"), in.DocumentID, *current(c))
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			notFound(c)
			return
		}
		internalErr(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// ListFolderDocuments godoc
// @Summary List documents in folder
// @Tags folders
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Folder ID"
// @Success 200 {object} map[string]interface{}
// @Router /v1/folders/{id}/documents [get]
func (h handlers) listFolderDocuments(c *gin.Context) {
	if !h.needFolderRole(c, c.Param("id"), repo.RoleViewer) {
		return
	}
	items, err := h.deps.repository.ListFolderDocuments(c.Request.Context(), c.Param("id"))
	if err != nil {
		internalErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// RemoveDocumentFromFolder godoc
// @Summary Remove document from folder
// @Tags folders
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Folder ID"
// @Param doc_id path string true "Document ID"
// @Success 204
// @Router /v1/folders/{id}/documents/{doc_id} [delete]
func (h handlers) removeDocumentFromFolder(c *gin.Context) {
	if !h.needFolderRole(c, c.Param("id"), repo.RoleEditor) {
		return
	}
	if err := h.deps.repository.RemoveDocumentFromFolder(c.Request.Context(), c.Param("id"), c.Param("doc_id")); err != nil {
		internalErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h handlers) needFolderRole(c *gin.Context, folderID string, need repo.Role) bool {
	role, _ := h.deps.repository.RoleForFolder(c.Request.Context(), *current(c), folderID)
	if !atLeast(role, need) {
		forbidden(c, string(need)+" role required")
		return false
	}
	return true
}
