package server

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/razorpay/aidocs/api/internal/auth"
	"github.com/razorpay/aidocs/api/internal/bots"
	"github.com/razorpay/aidocs/api/internal/repo"
)

// resolveGrantPrincipal turns a grant request's address or inline principal
// spec into a concrete auth.Principal.
// Sentinel errors returned by resolveGrantPrincipal so callers can map them to
// HTTP responses without matching on error message text.
var (
	errInvalidAddress      = errors.New("invalid grant address")
	errPrincipalIDRequired = errors.New("principal id required")
)

func resolveGrantPrincipal(c *gin.Context, r repo.Repository, address string, principal auth.Principal) (auth.Principal, error) {
	normalized, err := normalizeGrantPrincipal(address, principal)
	if err != nil {
		return auth.Principal{}, err
	}
	return loadGrantPrincipal(c.Request.Context(), r, normalized)
}

func normalizeGrantPrincipal(address string, principal auth.Principal) (auth.Principal, error) {
	// The "anyone" principal is the whole-server audience; it has no address and
	// no backing record to resolve.
	if principal.Type == auth.PrincipalAnyone {
		return auth.Principal{Type: auth.PrincipalAnyone}, nil
	}
	if address != "" {
		return grantPrincipalFromAddress(address)
	}
	if principal.Type == auth.PrincipalUser && principal.ID == "" && principal.Email != "" {
		var domain string
		principal.Email, domain = normalizeGrantAddress(principal.Email)
		if principal.Email == "" || strings.HasSuffix(domain, ".bot") {
			return auth.Principal{}, errInvalidAddress
		}
	}
	return principal, nil
}

func grantPrincipalFromAddress(address string) (auth.Principal, error) {
	address, domain := normalizeGrantAddress(address)
	if address == "" {
		return auth.Principal{}, errInvalidAddress
	}
	if strings.HasSuffix(domain, ".bot") {
		return auth.Principal{Type: auth.PrincipalServiceAccount, Name: address}, nil
	}
	return auth.Principal{Type: auth.PrincipalUser, Email: address}, nil
}

func loadGrantPrincipal(ctx context.Context, r repo.Repository, principal auth.Principal) (auth.Principal, error) {
	switch principal.Type {
	case auth.PrincipalAnyone:
		return auth.Principal{Type: auth.PrincipalAnyone}, nil
	case auth.PrincipalUser:
		if principal.ID != "" {
			return r.ResolveUser(ctx, principal.ID)
		}
		if principal.Email != "" {
			return r.EnsureUserByEmail(ctx, principal.Email)
		}
	case auth.PrincipalServiceAccount:
		if principal.ID != "" {
			sa, err := r.GetServiceAccount(ctx, principal.ID)
			if err != nil {
				return auth.Principal{}, err
			}
			return auth.Principal{Type: auth.PrincipalServiceAccount, ID: sa.ID, Name: sa.Name}, nil
		}
		if principal.Name != "" {
			sa, err := r.GetServiceAccountByName(ctx, principal.Name)
			if err != nil {
				return auth.Principal{}, err
			}
			return auth.Principal{Type: auth.PrincipalServiceAccount, ID: sa.ID, Name: sa.Name}, nil
		}
	}
	return auth.Principal{}, errPrincipalIDRequired
}

// createGrantResponse is the payload of POST /v1/documents/{id}/grants.
type createGrantResponse struct {
	ID        string         `json:"id"`
	Resource  grantResource  `json:"resource"`
	Principal grantPrincipal `json:"principal"`
	Role      repo.Role      `json:"role"`
	GrantedBy grantGrantedBy `json:"granted_by"`
}

type grantResource struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type grantPrincipal struct {
	Type  auth.PrincipalType `json:"type"`
	ID    string             `json:"id"`
	Email string             `json:"email,omitempty"`
	Name  string             `json:"name,omitempty"`
}

type grantGrantedBy struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

func normalizeGrantAddress(address string) (string, string) {
	address = strings.ToLower(strings.TrimSpace(address))
	_, domain, ok := bots.Split(address)
	if !ok {
		return "", ""
	}
	return address, domain
}

// CreateGrant godoc
// @Summary Create document grant
// @Tags grants
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Document ID"
// @Success 201 {object} createGrantResponse
// @Router /v1/documents/{id}/grants [post]
func (h handlers) createGrant(c *gin.Context) {
	if !h.needDocRole(c, c.Param("id"), repo.RoleCommenter) {
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
	if !requireJSON(c) {
		return
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
		case errors.Is(err, repo.ErrNotFound):
			notFound(c)
		default:
			internalErr(c, err)
		}
		return
	}
	g, err := h.deps.repository.CreateGrant(c.Request.Context(), "document", c.Param("id"), resolved, in.Role, *p)
	if err != nil {
		internalErr(c, err)
		return
	}
	incGrant("created", string(g.Role), string(g.Principal.Type), actorType(c))
	c.JSON(http.StatusCreated, createGrantResponse{
		ID:       g.ID,
		Resource: grantResource{Type: "document", ID: g.DocumentID},
		Principal: grantPrincipal{
			Type:  g.Principal.Type,
			ID:    g.Principal.ID,
			Email: g.Principal.Email,
			Name:  g.Principal.Name,
		},
		Role:      g.Role,
		GrantedBy: grantGrantedBy{ID: g.GrantedBy.ID, Email: g.GrantedBy.Email},
	})
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
	if err := c.ShouldBindJSON(&in); err != nil || !validGrantRole(in.Role) {
		badRequest(c, "invalid grant")
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
