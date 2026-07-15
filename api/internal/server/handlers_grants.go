package server

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/razorpay/aidocs/api/internal/auth"
	"github.com/razorpay/aidocs/api/internal/bots"
	"github.com/razorpay/aidocs/api/internal/googlegroups"
	"github.com/razorpay/aidocs/api/internal/repo"
)

// resolveGrantPrincipal turns a grant request's address or inline principal
// spec into a concrete auth.Principal.
// Sentinel errors returned by resolveGrantPrincipal so callers can map them to
// HTTP responses without matching on error message text.
var (
	errInvalidAddress           = errors.New("invalid grant address")
	errPrincipalIDRequired      = errors.New("principal id required")
	errUnknownUserEmail         = errors.New("unknown user email")
	errGoogleGroupsDisabled     = errors.New("google groups disabled")
	errGoogleGroupEmailRequired = errors.New("google group email required")
)

type grantPrincipalInput struct {
	Type  auth.PrincipalType `json:"type" enums:"user,service_account,google_group,anyone"`
	ID    string             `json:"id,omitempty"`
	Email string             `json:"email,omitempty"`
	Name  string             `json:"name,omitempty"`
}

type grantInput struct {
	Address   string              `json:"address,omitempty"`
	Principal grantPrincipalInput `json:"principal"`
	Role      repo.Role           `json:"role" enums:"viewer,commenter,editor,owner"`
}

func resolveGrantPrincipal(c *gin.Context, r repo.Repository, directory googlegroups.GoogleGroupDirectory, groupsEnabled bool, address string, principal auth.Principal) (auth.Principal, error) {
	// The "anyone" principal is the whole-server audience; it has no address and
	// no backing record to resolve.
	if principal.Type == auth.PrincipalAnyone {
		return auth.Principal{Type: auth.PrincipalAnyone}, nil
	}
	if principal.Type == auth.PrincipalGoogleGroup {
		if !groupsEnabled || directory == nil {
			return auth.Principal{}, errGoogleGroupsDisabled
		}
		if strings.TrimSpace(principal.Email) == "" {
			return auth.Principal{}, errGoogleGroupEmailRequired
		}
		// Never trust a caller-supplied group ID. Lookup+get verifies the
		// canonical address, immutable groups/{id}, customer, and group type.
		group, err := directory.LookupGroup(c.Request.Context(), principal.Email)
		if err != nil {
			return auth.Principal{}, err
		}
		if err := r.UpsertGoogleGroup(c.Request.Context(), repo.GoogleGroup{
			ID: group.ID, CustomerID: group.CustomerID, Email: group.Email,
			Name: group.Name, LastVerifiedAt: time.Now(),
		}); err != nil {
			return auth.Principal{}, err
		}
		return auth.Principal{Type: auth.PrincipalGoogleGroup, ID: group.ID, Email: group.Email, Name: group.Name}, nil
	}
	if address != "" {
		_, domain, ok := bots.Split(address)
		if !ok {
			return auth.Principal{}, errInvalidAddress
		}
		if strings.HasSuffix(domain, ".bot") {
			principal = auth.Principal{Type: auth.PrincipalServiceAccount, Name: address}
		} else {
			principal = auth.Principal{Type: auth.PrincipalUser, Email: address}
		}
	}
	switch {
	case principal.Type == auth.PrincipalUser && principal.ID == "" && principal.Email != "":
		p, err := r.UserByEmail(c.Request.Context(), principal.Email)
		if errors.Is(err, repo.ErrNotFound) {
			return auth.Principal{}, errUnknownUserEmail
		}
		if err != nil {
			return auth.Principal{}, err
		}
		return p, nil
	case principal.Type == auth.PrincipalServiceAccount && principal.ID == "" && principal.Name != "":
		sa, err := r.GetServiceAccountByName(c.Request.Context(), principal.Name)
		if err != nil {
			return auth.Principal{}, err
		}
		return auth.Principal{Type: auth.PrincipalServiceAccount, ID: sa.ID, Name: sa.Name}, nil
	default:
		if principal.ID == "" {
			return auth.Principal{}, errPrincipalIDRequired
		}
		exists, err := r.PrincipalExists(c.Request.Context(), principal)
		if err != nil {
			return auth.Principal{}, err
		}
		if !exists {
			return auth.Principal{}, repo.ErrNotFound
		}
		return principal, nil
	}
}

func validGrantRoleForPrincipal(principalType auth.PrincipalType, role repo.Role) bool {
	if role == repo.RoleOwner {
		return principalType == auth.PrincipalGoogleGroup
	}
	return validGrantRole(role)
}

func (h handlers) isCanonicalOwner(c *gin.Context, resourceType, resourceID string) bool {
	p := current(c)
	if p == nil {
		return false
	}
	if resourceType == "folder" {
		resource, err := h.deps.repository.GetFolder(c.Request.Context(), resourceID)
		return err == nil && resource.Owner.Type == p.Type && resource.Owner.ID == p.ID
	}
	resource, err := h.deps.repository.GetDocument(c.Request.Context(), resourceID)
	return err == nil && resource.Owner.Type == p.Type && resource.Owner.ID == p.ID
}

func (h handlers) requireCanonicalOwnerForOwnerGroupChange(c *gin.Context, resourceType, resourceID string, before, after repo.Grant) bool {
	changesOwnerGroup := before.Principal.Type == auth.PrincipalGoogleGroup && before.Role == repo.RoleOwner ||
		after.Principal.Type == auth.PrincipalGoogleGroup && after.Role == repo.RoleOwner
	if !changesOwnerGroup {
		return true
	}
	// Group-derived owners are co-owners; they never replace the canonical
	// owner and cannot alter the grants that confer owner-level group access.
	if !h.isCanonicalOwner(c, resourceType, resourceID) {
		forbidden(c, "only the canonical owner may manage owner-level Google Group grants")
		return false
	}
	return true
}

// CreateGrant godoc
// @Summary Create document grant
// @Tags grants
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Document ID"
// @Param grant body grantInput true "Grant target and role; google_group principals are resolved from principal.email"
// @Success 201 {object} map[string]interface{}
// @Router /v1/documents/{id}/grants [post]
func (h handlers) createGrant(c *gin.Context) {
	if !h.needDocRole(c, c.Param("id"), repo.RoleCommenter) {
		return
	}
	p := current(c)
	var in grantInput
	if !requireJSON(c) {
		return
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		badRequest(c, "invalid grant")
		return
	}
	if !validGrantRoleForPrincipal(in.Principal.Type, in.Role) {
		badRequest(c, "invalid role")
		return
	}
	principal := auth.Principal{Type: in.Principal.Type, ID: in.Principal.ID, Email: in.Principal.Email, Name: in.Principal.Name}
	resolved, err := resolveGrantPrincipal(c, h.deps.repository, h.deps.googleGroupDirectory, h.deps.googleGroupsEnabled, in.Address, principal)
	if err != nil {
		switch {
		case errors.Is(err, errInvalidAddress):
			badRequest(c, "That doesn't look like an email or bot address.")
		case errors.Is(err, errPrincipalIDRequired):
			badRequest(c, "principal id, user email, or service account name is required")
		case errors.Is(err, errUnknownUserEmail):
			badRequest(c, "No aidocs user exists for that email. Ask them to sign in once, then try again.")
		case errors.Is(err, errGoogleGroupsDisabled):
			badRequest(c, "Google Group sharing is disabled")
		case errors.Is(err, errGoogleGroupEmailRequired), googlegroups.IsNotFound(err):
			badRequest(c, "Select a verified Google Group result")
		case errors.Is(err, repo.ErrNotFound):
			notFound(c)
		default:
			if in.Principal.Type == auth.PrincipalGoogleGroup {
				incGoogleGroupOperation("grant_resolution", "failure")
				c.JSON(http.StatusServiceUnavailable, errorResponse("dependency_unavailable", "Google Group directory is temporarily unavailable", nil))
			} else {
				internalErr(c, err)
			}
		}
		return
	}
	if !h.requireCanonicalOwnerForOwnerGroupChange(c, "document", c.Param("id"), repo.Grant{}, repo.Grant{Principal: resolved, Role: in.Role}) {
		return
	}
	g, err := h.deps.repository.CreateGrant(c.Request.Context(), "document", c.Param("id"), resolved, in.Role, *p)
	if err != nil {
		internalErr(c, err)
		return
	}
	if resolved.Type == auth.PrincipalGoogleGroup {
		incGoogleGroupOperation("grant_resolution", "success")
	}
	pr := gin.H{"type": g.Principal.Type, "id": g.Principal.ID}
	if g.Principal.Email != "" {
		pr["email"] = g.Principal.Email
	}
	if g.Principal.Name != "" {
		pr["name"] = g.Principal.Name
	}
	incGrant("created", string(g.Role), string(g.Principal.Type), actorType(c))
	c.JSON(http.StatusCreated, gin.H{"id": g.ID, "resource": gin.H{"type": "document", "id": g.DocumentID}, "principal": pr, "role": g.Role, "granted_by": gin.H{"id": g.GrantedBy.ID, "email": g.GrantedBy.Email}})
}

// PatchGrant godoc
// @Summary Update document grant
// @Tags grants
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Document ID"
// @Param grant_id path string true "Grant ID"
// @Success 200 {object} repo.Grant
// @Router /v1/documents/{id}/grants/{grant_id} [patch]
func (h handlers) patchGrant(c *gin.Context) {
	if !h.needDocRole(c, c.Param("id"), repo.RoleCommenter) {
		return
	}
	var in struct {
		Role repo.Role `json:"role"`
	}
	if !requireJSON(c) {
		return
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		badRequest(c, "invalid grant")
		return
	}
	existing, err := h.deps.repository.GetGrant(c.Request.Context(), c.Param("id"), c.Param("grant_id"))
	if err != nil {
		notFound(c)
		return
	}
	if !validGrantRoleForPrincipal(existing.Principal.Type, in.Role) {
		badRequest(c, "invalid grant")
		return
	}
	if !h.requireCanonicalOwnerForOwnerGroupChange(c, "document", c.Param("id"), existing, repo.Grant{Principal: existing.Principal, Role: in.Role}) {
		return
	}
	g, err := h.deps.repository.UpdateGrant(c.Request.Context(), c.Param("id"), c.Param("grant_id"), in.Role)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			notFound(c)
			return
		}
		internalErr(c, err)
		return
	}
	incGrant("updated", string(g.Role), string(g.Principal.Type), actorType(c))
	c.JSON(http.StatusOK, g)
}

// DeleteGrant godoc
// @Summary Delete document grant
// @Tags grants
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Document ID"
// @Param grant_id path string true "Grant ID"
// @Success 204
// @Router /v1/documents/{id}/grants/{grant_id} [delete]
func (h handlers) deleteGrant(c *gin.Context) {
	if !h.needDocRole(c, c.Param("id"), repo.RoleCommenter) {
		return
	}
	existing, err := h.deps.repository.GetGrant(c.Request.Context(), c.Param("id"), c.Param("grant_id"))
	if err != nil {
		notFound(c)
		return
	}
	if !h.requireCanonicalOwnerForOwnerGroupChange(c, "document", c.Param("id"), existing, repo.Grant{}) {
		return
	}
	if err := h.deps.repository.DeleteGrant(c.Request.Context(), c.Param("id"), c.Param("grant_id")); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			notFound(c)
			return
		}
		internalErr(c, err)
		return
	}
	incGrant("deleted", labelUnknown, labelUnknown, actorType(c))
	c.Status(http.StatusNoContent)
}
