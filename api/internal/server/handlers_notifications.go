package server

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/razorpay/aidocs/api/internal/repo"
)

// notificationPreferences godoc
// @Summary Read your notification preferences
// @Tags notifications
// @Security bearerAuth
// @Security cookieAuth
// @Success 200 {object} repo.NotificationPreferences
// @Router /v1/me/notifications [get]
func (h handlers) notificationPreferences(c *gin.Context) {
	docID := c.Param("id")
	if docID != "" && !h.needDocRole(c, docID, repo.RoleViewer) {
		return
	}
	prefs, err := h.deps.repository.NotificationPreferences(c.Request.Context(), *current(c), docID)
	if err != nil {
		internalErr(c, err)
		return
	}
	c.JSON(http.StatusOK, prefs)
}

type notificationPreferencesRequest struct {
	Enabled *bool `json:"enabled"`
}

// setNotificationPreferences godoc
// @Summary Set your notification preferences
// @Description Applies only to the authenticated principal. Document preferences never change access or other users' notifications. Global disable overrides document preferences.
// @Tags notifications
// @Security bearerAuth
// @Security cookieAuth
// @Accept json
// @Param body body notificationPreferencesRequest true "Notification preference"
// @Success 200 {object} repo.NotificationPreferences
// @Router /v1/me/notifications [put]
func (h handlers) setNotificationPreferences(c *gin.Context) {
	docID := c.Param("id")
	if docID != "" && !h.needDocRole(c, docID, repo.RoleViewer) {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	var in notificationPreferencesRequest
	if err := c.ShouldBindJSON(&in); err != nil || in.Enabled == nil {
		badRequest(c, "enabled must be a boolean")
		return
	}
	prefs, err := h.deps.repository.SetNotificationPreferences(c.Request.Context(), *current(c), docID, *in.Enabled)
	if err != nil {
		internalErr(c, err)
		return
	}
	c.JSON(http.StatusOK, prefs)
}

// documentNotificationPreferences godoc
// @Summary Read your notification preferences for a document
// @Tags notifications
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Document ID"
// @Success 200 {object} repo.NotificationPreferences
// @Router /v1/documents/{id}/notifications [get]
func (h handlers) documentNotificationPreferences(c *gin.Context) { h.notificationPreferences(c) }

// setDocumentNotificationPreferences godoc
// @Summary Enable or mute your document notifications
// @Description Personal preference only. Global disable overrides this preference.
// @Tags notifications
// @Security bearerAuth
// @Security cookieAuth
// @Accept json
// @Param id path string true "Document ID"
// @Param body body notificationPreferencesRequest true "Notification preference"
// @Success 200 {object} repo.NotificationPreferences
// @Router /v1/documents/{id}/notifications [put]
func (h handlers) setDocumentNotificationPreferences(c *gin.Context) { h.setNotificationPreferences(c) }
