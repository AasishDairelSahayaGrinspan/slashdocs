package server

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/razorpay/aidocs/api/internal/auth"
	"github.com/razorpay/aidocs/api/internal/repo"
)

func (h handlers) needFolderRole(c *gin.Context, id string, need repo.Role) bool {
	role, _ := h.deps.repository.RoleForFolder(c.Request.Context(), *current(c), id)
	if !atLeast(role, need) {
		forbidden(c, string(need)+" role required")
		return false
	}
	return true
}

func (h handlers) createFolder(c *gin.Context) {
	var in struct {
		Title          string `json:"title"`
		ParentFolderID string `json:"parent_folder_id"`
	}
	if c.ShouldBindJSON(&in) != nil || in.Title == "" {
		badRequest(c, "invalid folder")
		return
	}
	if in.ParentFolderID != "" && !h.needFolderRole(c, in.ParentFolderID, repo.RoleEditor) {
		return
	}
	f, err := h.deps.repository.CreateFolder(c.Request.Context(), *current(c), in.Title, in.ParentFolderID)
	if err != nil {
		if errors.Is(err, repo.ErrConflict) {
			c.JSON(http.StatusConflict, errorResponse("conflict", "folder conflict", nil))
			return
		}
		internalErr(c, err)
		return
	}
	c.JSON(http.StatusCreated, f)
}
func (h handlers) listFolders(c *gin.Context) {
	items, has, folder, err := h.deps.repository.ListFolder(c.Request.Context(), *current(c), c.Query("folder_id"), c.Query("path"))
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			notFound(c)
			return
		}
		internalErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"folder": folder, "items": items, "has_more": has})
}
func (h handlers) getFolder(c *gin.Context) {
	if !h.needFolderRole(c, c.Param("id"), repo.RoleViewer) {
		return
	}
	f, err := h.deps.repository.GetFolder(c.Request.Context(), c.Param("id"))
	if err != nil {
		notFound(c)
		return
	}
	c.JSON(http.StatusOK, f)
}
func (h handlers) patchFolder(c *gin.Context) {
	if !h.needFolderRole(c, c.Param("id"), repo.RoleOwner) {
		return
	}
	var in struct {
		Title          string  `json:"title"`
		ParentFolderID *string `json:"parent_folder_id"`
	}
	if c.ShouldBindJSON(&in) != nil {
		badRequest(c, "invalid folder")
		return
	}
	f, err := h.deps.repository.UpdateFolder(c.Request.Context(), c.Param("id"), in.Title, in.ParentFolderID)
	if err != nil {
		if errors.Is(err, repo.ErrConflict) {
			c.JSON(http.StatusConflict, errorResponse("conflict", "folder conflict", nil))
			return
		}
		if errors.Is(err, repo.ErrNotFound) {
			notFound(c)
			return
		}
		internalErr(c, err)
		return
	}
	c.JSON(http.StatusOK, f)
}
func (h handlers) deleteFolder(c *gin.Context) {
	if !h.needFolderRole(c, c.Param("id"), repo.RoleOwner) {
		return
	}
	if err := h.deps.repository.DeleteFolder(c.Request.Context(), c.Param("id")); err != nil {
		notFound(c)
		return
	}
	c.Status(http.StatusNoContent)
}
func (h handlers) folderAccess(c *gin.Context) {
	if !h.needFolderRole(c, c.Param("id"), repo.RoleOwner) {
		return
	}
	items, err := h.deps.repository.ListEffectiveAccess(c.Request.Context(), "folder", c.Param("id"))
	if err != nil {
		internalErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}
func (h handlers) documentAccess(c *gin.Context) {
	if !h.needDocRole(c, c.Param("id"), repo.RoleOwner) {
		return
	}
	items, err := h.deps.repository.ListEffectiveAccess(c.Request.Context(), "document", c.Param("id"))
	if err != nil {
		internalErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h handlers) listFolderDocs(c *gin.Context) {
	if !h.needFolderRole(c, c.Param("id"), repo.RoleViewer) {
		return
	}
	docs, err := h.deps.repository.ListFolderDocuments(c.Request.Context(), *current(c), c.Param("id"))
	if err != nil {
		internalErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": docs})
}
func (h handlers) addFolderDoc(c *gin.Context) {
	if !h.needFolderRole(c, c.Param("id"), repo.RoleEditor) {
		return
	}
	var in struct {
		DocumentID string `json:"document_id"`
	}
	if c.ShouldBindJSON(&in) != nil || in.DocumentID == "" {
		badRequest(c, "invalid document")
		return
	}
	shared, err := h.deps.repository.AddDocumentToFolder(c.Request.Context(), *current(c), c.Param("id"), in.DocumentID)
	if err != nil {
		notFound(c)
		return
	}
	out := gin.H{"document_id": in.DocumentID}
	if shared {
		out["permission_warning"] = gin.H{"kind": "may_broaden_access", "message": "Everyone with access to this folder may gain access. Existing document shares will remain."}
	}
	c.JSON(http.StatusOK, out)
}
func (h handlers) removeFolderDoc(c *gin.Context) {
	if !h.needFolderRole(c, c.Param("id"), repo.RoleEditor) {
		return
	}
	if err := h.deps.repository.RemoveDocumentFromFolder(c.Request.Context(), c.Param("id"), c.Param("doc_id")); err != nil {
		notFound(c)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h handlers) createFolderGrant(c *gin.Context) {
	if !h.needFolderRole(c, c.Param("id"), repo.RoleOwner) {
		return
	}
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
	if err := c.ShouldBindJSON(&in); err != nil || !validGrantRole(in.Role) {
		badRequest(c, "invalid grant")
		return
	}
	principal := auth.Principal{Type: in.Principal.Type, ID: in.Principal.ID, Email: in.Principal.Email, Name: in.Principal.Name}
	resolved, err := resolveGrantPrincipal(c, h.deps.repository, in.Address, principal)
	if err != nil {
		badRequest(c, "invalid principal")
		return
	}
	g, err := h.deps.repository.CreateGrant(c.Request.Context(), c.Param("id"), resolved, in.Role, *current(c))
	if err != nil {
		internalErr(c, err)
		return
	}
	g.ResourceType, g.ResourceID = "folder", c.Param("id")
	c.JSON(http.StatusCreated, g)
}
func (h handlers) listFolderGrants(c *gin.Context) {
	if !h.needFolderRole(c, c.Param("id"), repo.RoleOwner) { return }
	items, err := h.deps.repository.ListGrants(c.Request.Context(), c.Param("id")); if err != nil { internalErr(c, err); return }
	c.JSON(http.StatusOK, gin.H{"items": items})
}
func (h handlers) patchFolderGrant(c *gin.Context) {
	if !h.needFolderRole(c, c.Param("id"), repo.RoleOwner) { return }
	var in struct{ Role repo.Role `json:"role"` }
	if err := c.ShouldBindJSON(&in); err != nil || !validGrantRole(in.Role) { badRequest(c, "invalid grant"); return }
	g, err := h.deps.repository.UpdateGrant(c.Request.Context(), c.Param("id"), c.Param("grant_id"), in.Role)
	if err != nil { if errors.Is(err, repo.ErrNotFound) { notFound(c); return }; internalErr(c, err); return }
	c.JSON(http.StatusOK, g)
}
func (h handlers) deleteFolderGrant(c *gin.Context) {
	if !h.needFolderRole(c, c.Param("id"), repo.RoleOwner) { return }
	if err := h.deps.repository.DeleteGrant(c.Request.Context(), c.Param("id"), c.Param("grant_id")); err != nil { if errors.Is(err, repo.ErrNotFound) { notFound(c); return }; internalErr(c, err); return }
	c.Status(http.StatusNoContent)
}
