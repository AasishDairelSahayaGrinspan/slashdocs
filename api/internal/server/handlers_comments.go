package server

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/razorpay/aidocs/api/internal/auth"
	"github.com/razorpay/aidocs/api/internal/repo"
)

type commentMentionPrincipalType string

const (
	commentMentionPrincipalUser           commentMentionPrincipalType = "user"
	commentMentionPrincipalServiceAccount commentMentionPrincipalType = "service_account"
)

func (t commentMentionPrincipalType) authType() auth.PrincipalType {
	switch t {
	case commentMentionPrincipalUser:
		return auth.PrincipalUser
	case commentMentionPrincipalServiceAccount:
		return auth.PrincipalServiceAccount
	default:
		// Preserve an unknown wire value so repository validation rejects it.
		return auth.PrincipalType(t)
	}
}

type commentMentionPrincipalRef struct {
	Type commentMentionPrincipalType `json:"type"`
	ID   string                      `json:"id"`
}

type commentMentionRequest struct {
	Principal commentMentionPrincipalRef `json:"principal"`
	// Offset is the zero-based start in UTF-16 code units.
	Offset int64 `json:"offset"`
	// Length is the range length in UTF-16 code units.
	Length int64 `json:"length"`
}

type createCommentRequest struct {
	VersionID string                  `json:"version_id"`
	Body      string                  `json:"body"`
	Anchor    repo.Anchor             `json:"anchor"`
	Mentions  []commentMentionRequest `json:"mentions"`
}

type createCommentReplyRequest struct {
	Body     string                  `json:"body"`
	Mentions []commentMentionRequest `json:"mentions"`
}

type patchCommentRequest struct {
	Body     *string                  `json:"body"`
	Status   string                   `json:"status"`
	Mentions *[]commentMentionRequest `json:"mentions"`
}

// CreateComment godoc
// @Summary Create comment
// @Description Requests with non-empty mentions must use Content-Type application/vnd.aidocs.comment-mentions+json so mixed-version deployments fail closed instead of dropping metadata.
// @Tags comments
// @Security bearerAuth
// @Security cookieAuth
// @Accept json,application/vnd.aidocs.comment-mentions+json
// @Param id path string true "Document ID"
// @Param comment body createCommentRequest true "Comment body, anchor, and structured mentions"
// @Success 201 {object} commentResponse
// @Router /v1/documents/{id}/comments [post]
func (h handlers) createComment(c *gin.Context) {
	if !h.needDocRole(c, c.Param("id"), repo.RoleCommenter) {
		return
	}
	p := current(c)
	var in createCommentRequest
	if !requireCommentJSON(c) {
		return
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		badRequest(c, "invalid comment")
		return
	}
	if len(in.Mentions) > 0 && !requireStructuredCommentMediaType(c) {
		return
	}
	in.Body = repo.NormalizeCommentBody(in.Body)
	if in.VersionID == "" || in.Body == "" || in.Anchor.Quote == "" {
		badRequest(c, "invalid comment")
		return
	}
	cm, err := h.deps.repository.CreateComment(c.Request.Context(), c.Param("id"), in.VersionID, in.Body, in.Anchor, *p, commentMentions(in.Mentions))
	if err != nil {
		commentWriteError(c, err)
		return
	}
	incComment("created", cm.Status, actorType(c))
	c.JSON(http.StatusCreated, commentJSON(cm, cm.VersionID, ""))
}

// ListComments godoc
// @Summary List comments
// @Tags comments
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Document ID"
// @Param status query string false "open|resolved|stale|orphaned|all"
// @Param version_id query string false "Version ID"
// @Success 200 {object} listResponse[commentResponse]
// @Router /v1/documents/{id}/comments [get]
func (h handlers) listComments(c *gin.Context) {
	if !h.needDocRole(c, c.Param("id"), repo.RoleViewer) {
		return
	}
	versionID := c.Query("version_id")
	items, err := h.deps.repository.ListComments(c.Request.Context(), c.Param("id"), c.Query("status"), versionID)
	if err != nil {
		internalErr(c, err)
		return
	}
	placementVersionID := versionID
	if placementVersionID == "" {
		d, err := h.deps.repository.GetDocument(c.Request.Context(), c.Param("id"))
		if err == nil {
			placementVersionID = d.CurrentVersionID
		}
	}
	var placementHTML []byte
	if placementVersionID != "" {
		placementHTML, _ = h.deps.repository.GetVersionHTML(c.Request.Context(), placementVersionID)
	}
	c.JSON(http.StatusOK, listResponse[commentResponse]{Items: commentsJSON(items, placementVersionID, placementHTML)})
}

// CreateCommentReply godoc
// @Summary Create comment reply
// @Description Requests with non-empty mentions must use Content-Type application/vnd.aidocs.comment-mentions+json so mixed-version deployments fail closed instead of dropping metadata.
// @Tags comments
// @Security bearerAuth
// @Security cookieAuth
// @Accept json,application/vnd.aidocs.comment-mentions+json
// @Param id path string true "Document ID"
// @Param comment_id path string true "Parent Comment ID"
// @Param reply body createCommentReplyRequest true "Reply body and structured mentions"
// @Success 201 {object} commentResponse
// @Router /v1/documents/{id}/comments/{comment_id}/replies [post]
func (h handlers) createCommentReply(c *gin.Context) {
	if !h.needDocRole(c, c.Param("id"), repo.RoleCommenter) {
		return
	}
	var in createCommentReplyRequest
	if !requireCommentJSON(c) {
		return
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		badRequest(c, "invalid reply")
		return
	}
	if len(in.Mentions) > 0 && !requireStructuredCommentMediaType(c) {
		return
	}
	in.Body = repo.NormalizeCommentBody(in.Body)
	if in.Body == "" {
		badRequest(c, "invalid reply")
		return
	}
	cm, err := h.deps.repository.CreateCommentReply(c.Request.Context(), c.Param("id"), c.Param("comment_id"), in.Body, *current(c), commentMentions(in.Mentions))
	if err != nil {
		commentWriteError(c, err)
		return
	}
	incComment("created", cm.Status, actorType(c))
	c.JSON(http.StatusCreated, commentJSON(cm, "", ""))
}

// PatchComment godoc
// @Summary Update comment
// @Description Any request containing the mentions key, including an empty array, must use Content-Type application/vnd.aidocs.comment-mentions+json so mixed-version deployments fail closed.
// @Tags comments
// @Security bearerAuth
// @Security cookieAuth
// @Accept json,application/vnd.aidocs.comment-mentions+json
// @Param id path string true "Document ID"
// @Param comment_id path string true "Comment ID"
// @Param update body patchCommentRequest true "Fields to update; omitted mentions are preserved unless the body changes"
// @Success 200 {object} commentResponse
// @Router /v1/documents/{id}/comments/{comment_id} [patch]
func (h handlers) patchComment(c *gin.Context) {
	if !h.canMutateComment(c, c.Param("id"), c.Param("comment_id")) {
		return
	}
	var in patchCommentRequest
	if !requireCommentJSON(c) {
		return
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		badRequest(c, "invalid body")
		return
	}
	if in.Mentions != nil && !requireStructuredCommentMediaType(c) {
		return
	}
	if message := normalizePatchCommentRequest(&in); message != "" {
		badRequest(c, message)
		return
	}
	var mentions *[]repo.CommentMention
	if in.Mentions != nil {
		converted := commentMentions(*in.Mentions)
		mentions = &converted
	}
	cm, err := h.deps.repository.UpdateComment(c.Request.Context(), c.Param("comment_id"), repo.CommentUpdate{Body: in.Body, Status: in.Status, Mentions: mentions})
	if err != nil {
		commentWriteError(c, err)
		return
	}
	event := "updated"
	switch in.Status {
	case commentStatusResolved:
		event = "resolved"
	case commentStatusOpen:
		event = "reopened"
	}
	incComment(event, cm.Status, actorType(c))
	c.JSON(http.StatusOK, commentJSON(cm, cm.VersionID, ""))
}

func normalizePatchCommentRequest(in *patchCommentRequest) string {
	if in.Body != nil {
		normalized := repo.NormalizeCommentBody(*in.Body)
		in.Body = &normalized
	}
	// Older CLI versions sent body:"" alongside a status change. The previous
	// handler treated that as omitted, so retain the behavior across upgrades.
	if in.Body != nil && *in.Body == "" && in.Status != "" && in.Mentions == nil {
		in.Body = nil
	}
	switch {
	case in.Body != nil && *in.Body == "":
		return "comment body cannot be empty"
	case in.Status != "" && in.Status != commentStatusOpen && in.Status != commentStatusResolved:
		return "invalid comment status"
	case in.Body == nil && in.Status == "" && in.Mentions == nil:
		return "nothing to update"
	default:
		return ""
	}
}

func commentMentions(items []commentMentionRequest) []repo.CommentMention {
	out := make([]repo.CommentMention, 0, len(items))
	for _, item := range items {
		out = append(out, repo.CommentMention{
			Principal:   auth.Principal{Type: item.Principal.Type.authType(), ID: item.Principal.ID},
			OffsetUTF16: item.Offset,
			LengthUTF16: item.Length,
		})
	}
	return out
}

func commentWriteError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, repo.ErrInvalidCommentBody):
		badRequest(c, "comment body cannot be empty")
	case errors.Is(err, repo.ErrInvalidCommentMention):
		badRequest(c, "invalid comment mention")
	case errors.Is(err, repo.ErrNotFound):
		notFound(c)
	default:
		internalErr(c, err)
	}
}

// DeleteComment godoc
// @Summary Delete comment
// @Tags comments
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Document ID"
// @Param comment_id path string true "Comment ID"
// @Success 204
// @Router /v1/documents/{id}/comments/{comment_id} [delete]
func (h handlers) deleteComment(c *gin.Context) {
	if !h.canDeleteComment(c, c.Param("id"), c.Param("comment_id")) {
		return
	}
	if err := h.deps.repository.DeleteComment(c.Request.Context(), c.Param("comment_id")); err != nil {
		internalErr(c, err)
		return
	}
	incComment("deleted", labelUnknown, actorType(c))
	c.Status(http.StatusNoContent)
}

func (h handlers) canDeleteComment(c *gin.Context, docID, commentID string) bool {
	p := current(c)
	cm, err := h.deps.repository.GetComment(c.Request.Context(), commentID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			forbidden(c, "comment access required")
			return false
		}
		internalErr(c, err)
		return false
	}
	if cm.DocumentID != docID {
		c.JSON(http.StatusNotFound, errorResponse("not_found", "comment not found in document", nil))
		return false
	}
	if cm.Author.Type == p.Type && cm.Author.ID == p.ID {
		return true
	}
	forbidden(c, "only the comment author can delete this comment")
	return false
}

func (h handlers) canMutateComment(c *gin.Context, docID, commentID string) bool {
	p := current(c)
	cm, err := h.deps.repository.GetComment(c.Request.Context(), commentID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			forbidden(c, "comment access required")
			return false
		}
		internalErr(c, err)
		return false
	}
	if cm.DocumentID != docID {
		c.JSON(http.StatusNotFound, errorResponse("not_found", "comment not found in document", nil))
		return false
	}
	if cm.Author.Type == p.Type && cm.Author.ID == p.ID {
		return true
	}
	role, err := h.deps.repository.RoleForDocument(c.Request.Context(), *p, cm.DocumentID)
	if err != nil {
		internalErr(c, err)
		return false
	}
	if role == repo.RoleOwner || role == repo.RoleEditor {
		return true
	}
	forbidden(c, "comment access required")
	return false
}
