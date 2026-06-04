package server

import (
	"strings"

	"github.com/gin-gonic/gin"
)

func (h handlers) searchUsers(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	if len(q) < 2 {
		c.JSON(200, gin.H{"items": []any{}})
		return
	}
	items, err := h.deps.repository.SearchUsersByEmail(c.Request.Context(), q)
	if err != nil {
		internalErr(c, err)
		return
	}
	c.JSON(200, gin.H{"items": items})
}
