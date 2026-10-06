package server

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/razorpay/slashdocs/api/internal/auth"
)

func (h handlers) searchUsers(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	if len(q) < 2 {
		c.JSON(http.StatusOK, listResponse[auth.Principal]{Items: []auth.Principal{}})
		return
	}
	items, err := h.deps.repository.SearchUsersByEmail(c.Request.Context(), q)
	if err != nil {
		internalErr(c, err)
		return
	}
	c.JSON(http.StatusOK, listResponse[auth.Principal]{Items: items})
}
