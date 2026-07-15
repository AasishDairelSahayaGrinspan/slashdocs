package server

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type googleGroupSearchItem struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

type googleGroupSearchResponse struct {
	Items []googleGroupSearchItem `json:"items"`
}

// SearchGoogleGroups godoc
// @Summary Search verified organization Google Groups
// @Tags google-groups
// @Security bearerAuth
// @Security cookieAuth
// @Produce json
// @Param q query string true "Email or display-name query (minimum 2 characters)"
// @Success 200 {object} googleGroupSearchResponse
// @Failure 400 {object} map[string]interface{}
// @Failure 503 {object} map[string]interface{}
// @Router /v1/google-groups [get]
func (h handlers) searchGoogleGroups(c *gin.Context) {
	if !h.deps.googleGroupsEnabled || h.deps.googleGroupDirectory == nil {
		c.JSON(http.StatusNotFound, errorResponse("feature_disabled", "Google Group sharing is disabled", nil))
		return
	}
	query := strings.TrimSpace(c.Query("q"))
	if len([]rune(query)) < 2 {
		badRequest(c, "q must contain at least two characters")
		return
	}
	groups, err := h.deps.googleGroupDirectory.SearchGroups(c.Request.Context(), query, 20)
	if err != nil {
		incGoogleGroupOperation("search", "failure")
		c.JSON(http.StatusServiceUnavailable, errorResponse("dependency_unavailable", "Google Group directory is temporarily unavailable", nil))
		return
	}
	items := make([]googleGroupSearchItem, 0, len(groups))
	for _, group := range groups {
		items = append(items, googleGroupSearchItem{ID: group.ID, Email: group.Email, Name: group.Name})
		if len(items) == 20 {
			break
		}
	}
	incGoogleGroupOperation("search", "success")
	c.JSON(http.StatusOK, googleGroupSearchResponse{Items: items})
}
