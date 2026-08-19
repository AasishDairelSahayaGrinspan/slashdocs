package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/razorpay/aidocs/api/internal/auth"
	"github.com/razorpay/aidocs/api/internal/avatar"
	"github.com/razorpay/aidocs/api/internal/repo"
)

// avatarMockAuthor is the synthetic service-account identity the built-in
// mock posts fabricated comments as. It has no service_accounts row and no
// key; it exists only as the denormalized author snapshot on mock comments.
// TODO(avatar-service): mocked — the real service posts comments through
// aidocs MCP as its own provisioned service account.
var avatarMockAuthor = auth.Principal{Type: auth.PrincipalServiceAccount, ID: "sa_avatar_mock", Name: "avatar-service (mock)"}

// avatarDocumentStore adapts the repository to the mock's fabrication needs:
// reading version HTML and inserting quote-anchored avatar comments.
type avatarDocumentStore struct{ repository repo.Repository }

func (s avatarDocumentStore) VersionHTML(ctx context.Context, documentID, versionID string) ([]byte, error) {
	v, err := s.repository.GetVersion(ctx, versionID)
	if err != nil {
		return nil, err
	}
	if v.DocumentID != documentID {
		return nil, repo.ErrNotFound
	}
	return s.repository.GetVersionHTML(ctx, versionID)
}

func (s avatarDocumentStore) CreateAvatarComment(ctx context.Context, documentID, versionID, quote, body, authorDisplay, severity string) (string, error) {
	cm, err := s.repository.CreateComment(ctx, documentID, versionID, body, repo.Anchor{Quote: quote}, avatarMockAuthor, repo.CommentMeta{AuthorDisplay: authorDisplay, Severity: severity}, nil)
	if err != nil {
		return "", err
	}
	return cm.ID, nil
}

type createAvatarReviewRequest struct {
	DocumentID string   `json:"document_id"`
	VersionID  string   `json:"version_id"`
	AvatarIDs  []string `json:"avatar_ids"`
}

type avatarFeedbackRequest struct {
	Signal  string `json:"signal"`
	Persona string `json:"persona"`
}

// ListAvatars godoc
// @Summary List avatars available to review a document
// @Tags avatars
// @Security bearerAuth
// @Security cookieAuth
// @Param document_id query string true "Document ID"
// @Success 200 {object} avatar.AvatarList
// @Router /v1/avatars [get]
func (h handlers) listAvatars(c *gin.Context) {
	docID := c.Query("document_id")
	if docID == "" {
		badRequest(c, "document_id is required")
		return
	}
	if !h.needDocRole(c, docID, repo.RoleCommenter) {
		return
	}
	list, err := h.deps.avatarClient.ListAvatars(c.Request.Context(), docID)
	if err != nil {
		internalErr(c, err)
		return
	}
	c.JSON(http.StatusOK, list)
}

// CreateAvatarReview godoc
// @Summary Request an avatar review of a document version
// @Tags avatars
// @Security bearerAuth
// @Security cookieAuth
// @Accept json
// @Param review body createAvatarReviewRequest true "Document, version, and selected avatars"
// @Success 200 {object} avatar.ReviewRequestStatus
// @Router /v1/avatar-reviews [post]
func (h handlers) createAvatarReview(c *gin.Context) {
	var in createAvatarReviewRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		badRequest(c, "invalid avatar review request")
		return
	}
	if in.DocumentID == "" || in.VersionID == "" || len(in.AvatarIDs) == 0 {
		badRequest(c, "document_id, version_id and avatar_ids are required")
		return
	}
	if !h.needDocRole(c, in.DocumentID, repo.RoleCommenter) {
		return
	}
	v, err := h.deps.repository.GetVersion(c.Request.Context(), in.VersionID)
	if errors.Is(err, repo.ErrNotFound) || (err == nil && v.DocumentID != in.DocumentID) {
		notFound(c)
		return
	}
	if err != nil {
		internalErr(c, err)
		return
	}
	status, err := h.deps.avatarClient.RequestReview(c.Request.Context(), avatar.ReviewRequest{
		AidocID:          in.DocumentID,
		AidocVersion:     in.VersionID,
		AvatarIDs:        in.AvatarIDs,
		TriggeredByEmail: current(c).Email,
	})
	if err != nil {
		avatarClientError(c, err)
		return
	}
	c.JSON(http.StatusOK, status)
}

// GetAvatarReview godoc
// @Summary Poll the status of an avatar review request
// @Tags avatars
// @Security bearerAuth
// @Security cookieAuth
// @Param request_id path string true "Review request ID"
// @Success 200 {object} avatar.ReviewRequestStatus
// @Router /v1/avatar-reviews/{request_id} [get]
func (h handlers) getAvatarReview(c *gin.Context) {
	status, err := h.deps.avatarClient.GetReview(c.Request.Context(), c.Param("request_id"))
	if err != nil {
		avatarClientError(c, err)
		return
	}
	if !h.needDocRole(c, status.AidocID, repo.RoleViewer) {
		return
	}
	c.JSON(http.StatusOK, status)
}

// SubmitAvatarCommentFeedback godoc
// @Summary Vote on an avatar comment
// @Description Upserts the caller's vote with the avatar service; aidocs stores nothing. Repeating the same signal toggles the vote off.
// @Tags avatars
// @Security bearerAuth
// @Security cookieAuth
// @Accept json
// @Param comment_id path string true "Comment ID"
// @Param feedback body avatarFeedbackRequest true "Signal (up|down) and persona (author|ic)"
// @Success 200 {object} avatar.FeedbackState
// @Router /v1/avatar-comments/{comment_id}/feedback [post]
func (h handlers) submitAvatarCommentFeedback(c *gin.Context) {
	var in avatarFeedbackRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		badRequest(c, "invalid feedback")
		return
	}
	if in.Persona == "" {
		in.Persona = avatar.PersonaAuthor
	}
	if in.Persona != avatar.PersonaAuthor && in.Persona != avatar.PersonaIC {
		badRequest(c, "invalid feedback persona")
		return
	}
	cm, err := h.deps.repository.GetComment(c.Request.Context(), c.Param("comment_id"))
	if errors.Is(err, repo.ErrNotFound) {
		notFound(c)
		return
	}
	if err != nil {
		internalErr(c, err)
		return
	}
	if !h.needDocRole(c, cm.DocumentID, repo.RoleViewer) {
		return
	}
	// Only avatar comments carry feedback controls.
	if cm.AuthorDisplay == "" {
		notFound(c)
		return
	}
	p := current(c)
	if p.Email == "" {
		forbidden(c, "feedback requires a signed-in user")
		return
	}
	state, err := h.deps.avatarClient.SubmitFeedback(c.Request.Context(), cm.ID, avatar.Feedback{
		Signal:  in.Signal,
		Persona: in.Persona,
		Surface: "aidoc",
		GivenBy: p.Email,
	})
	if err != nil {
		avatarClientError(c, err)
		return
	}
	c.JSON(http.StatusOK, state)
}

// avatarClientError maps avatar-service client errors onto API responses.
func avatarClientError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, avatar.ErrUnknownRequest):
		notFound(c)
	case errors.Is(err, avatar.ErrUnknownAvatar), errors.Is(err, avatar.ErrInvalidRequest):
		badRequest(c, fmt.Sprintf("avatar service rejected the request: %v", err))
	default:
		internalErr(c, err)
	}
}
