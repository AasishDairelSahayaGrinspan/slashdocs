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
			c.JSON(http.StatusConflict, errorResponse("conflict", "A folder named \""+in.Title+"\" already exists here.", nil))
			return
		}
		internalErr(c, err)
		return
	}
	c.JSON(http.StatusCreated, f)
}
func (h handlers) listFolders(c *gin.Context) {
	if q := c.Query("q"); q != "" {
		includeOrgWide := h.deps.deployment == DeploymentOrg
		folders, err := h.deps.repository.SearchFoldersByTitle(c.Request.Context(), *current(c), q, includeOrgWide, 25)
		if err != nil {
			internalErr(c, err)
			return
		}
		items := make([]repo.FolderEntry, 0, len(folders))
		for i := range folders {
			items = append(items, repo.FolderEntry{Type: "folder", Folder: &folders[i]})
		}
		c.JSON(http.StatusOK, gin.H{"folder": nil, "items": items, "has_more": len(folders) >= 25})
		return
	}
	folderID := c.Query("folder_id")
	if folderID != "" {
		if !h.needFolderRole(c, folderID, repo.RoleViewer) {
			return
		}
	}
	items, has, folder, err := h.deps.repository.ListFolder(c.Request.Context(), *current(c), folderID, c.Query("path"))
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			notFound(c)
			return
		}
		var ambErr *repo.AmbiguousPathError
		if errors.As(err, &ambErr) {
			c.JSON(http.StatusConflict, gin.H{
				"error": gin.H{
					"code":    "ambiguous_path",
					"message": "Multiple folders named \"" + ambErr.Segment + "\" found. Use --folder-id to specify.",
				},
				"candidates": ambErr.Candidates,
			})
			return
		}
		internalErr(c, err)
		return
	}
	if folder != nil && folderID == "" {
		if !h.needFolderRole(c, folder.ID, repo.RoleViewer) {
			return
		}
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
	p := current(c)
	if !(p.Type == f.Owner.Type && p.ID == f.Owner.ID) {
		_ = h.deps.repository.RecordFolderOpened(c.Request.Context(), f.ID, *p)
	}
	// Filter ancestors to only those the viewer can access (bottom-up).
	if len(f.Ancestors) > 0 {
		visibleFrom := len(f.Ancestors)
		for i := len(f.Ancestors) - 1; i >= 0; i-- {
			if r, _ := h.deps.repository.RoleForFolder(c.Request.Context(), *p, f.Ancestors[i].ID); atLeast(r, repo.RoleViewer) {
				visibleFrom = i
			} else {
				break
			}
		}
		f.Ancestors = f.Ancestors[visibleFrom:]
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
	if in.ParentFolderID != nil && *in.ParentFolderID != "" {
		if !h.needFolderRole(c, *in.ParentFolderID, repo.RoleEditor) {
			return
		}
	}
	f, err := h.deps.repository.UpdateFolder(c.Request.Context(), c.Param("id"), in.Title, in.ParentFolderID)
	if err != nil {
		if errors.Is(err, repo.ErrConflict) {
			c.JSON(http.StatusConflict, errorResponse("conflict", "A folder with that name already exists in the destination.", nil))
			return
		}
		if errors.Is(err, repo.ErrNotFound) {
			notFound(c)
			return
		}
		internalErr(c, err)
		return
	}
	out := gin.H{"id": f.ID, "title": f.Title, "owner": f.Owner, "parent_folder_id": f.ParentFolderID}
	if in.ParentFolderID != nil && *in.ParentFolderID != "" {
		if hasGrants, _ := h.deps.repository.FolderHasGrants(c.Request.Context(), *in.ParentFolderID); hasGrants {
			out["permission_warning"] = gin.H{"kind": "may_broaden_access", "message": "The destination folder has active grants — contents of the moved folder may become visible to others."}
		}
	}
	c.JSON(http.StatusOK, out)
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
	if !h.needFolderRole(c, c.Param("id"), repo.RoleViewer) {
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
	if !h.needDocRole(c, c.Param("id"), repo.RoleViewer) {
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
func (h handlers) moveDocToFolder(c *gin.Context) {
	folderID := c.Param("id")
	if !h.needFolderRole(c, folderID, repo.RoleEditor) {
		return
	}
	var in struct {
		DocumentID string `json:"document_id"`
	}
	if c.ShouldBindJSON(&in) != nil || in.DocumentID == "" {
		badRequest(c, "invalid document")
		return
	}
	p := current(c)
	docRole, _ := h.deps.repository.RoleForDocument(c.Request.Context(), *p, in.DocumentID)
	if !atLeast(docRole, repo.RoleEditor) {
		forbidden(c, "editor role on the document is required")
		return
	}
	hasGrants, err := h.deps.repository.MoveDocumentToFolder(c.Request.Context(), *p, folderID, in.DocumentID)
	if err != nil {
		notFound(c)
		return
	}
	out := gin.H{"document_id": in.DocumentID, "moved": true}
	if hasGrants {
		out["permission_warning"] = gin.H{"kind": "may_broaden_access", "message": "This folder has active grants — everyone with access may see this document."}
	}
	c.JSON(http.StatusOK, out)
}
func (h handlers) moveDocToRoot(c *gin.Context) {
	if !h.needFolderRole(c, c.Param("id"), repo.RoleEditor) {
		return
	}
	p := current(c)
	docRole, _ := h.deps.repository.RoleForDocument(c.Request.Context(), *p, c.Param("doc_id"))
	if !atLeast(docRole, repo.RoleEditor) {
		forbidden(c, "editor role on the document is required")
		return
	}
	if err := h.deps.repository.MoveDocumentToRoot(c.Request.Context(), c.Param("id"), c.Param("doc_id")); err != nil {
		notFound(c)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h handlers) createFolderGrant(c *gin.Context) {
	if !h.needFolderRole(c, c.Param("id"), repo.RoleEditor) {
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
	g, err := h.deps.repository.CreateGrant(c.Request.Context(), "folder", c.Param("id"), resolved, in.Role, *current(c))
	if err != nil {
		internalErr(c, err)
		return
	}
	g.ResourceType, g.ResourceID = "folder", c.Param("id")
	c.JSON(http.StatusCreated, g)
}
func (h handlers) patchFolderGrant(c *gin.Context) {
	if !h.needFolderRole(c, c.Param("id"), repo.RoleEditor) { return }
	var in struct{ Role repo.Role `json:"role"` }
	if err := c.ShouldBindJSON(&in); err != nil || !validGrantRole(in.Role) { badRequest(c, "invalid grant"); return }
	g, err := h.deps.repository.UpdateGrant(c.Request.Context(), c.Param("id"), c.Param("grant_id"), in.Role)
	if err != nil { if errors.Is(err, repo.ErrNotFound) { notFound(c); return }; internalErr(c, err); return }
	c.JSON(http.StatusOK, g)
}
func (h handlers) deleteFolderGrant(c *gin.Context) {
	if !h.needFolderRole(c, c.Param("id"), repo.RoleEditor) { return }
	if err := h.deps.repository.DeleteGrant(c.Request.Context(), c.Param("id"), c.Param("grant_id")); err != nil { if errors.Is(err, repo.ErrNotFound) { notFound(c); return }; internalErr(c, err); return }
	c.Status(http.StatusNoContent)
}
