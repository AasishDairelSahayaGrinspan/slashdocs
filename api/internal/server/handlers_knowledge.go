package server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"sort"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"
	"golang.org/x/net/html"

	"github.com/razorpay/aidocs/api/internal/auth"
	"github.com/razorpay/aidocs/api/internal/knowledge"
	"github.com/razorpay/aidocs/api/internal/repo"
)

type repositoryTransferResponse struct {
	RepositoryID string                  `json:"repository_id"`
	Target       *auth.Principal         `json:"target,omitempty"`
	Repository   *repo.RepositoryRecord  `json:"repository,omitempty"`
	Transfer     knowledge.TransferState `json:"transfer"`
}

type changeProposalResponse struct {
	Proposal knowledge.ChangeProposal `json:"proposal"`
}

type repositorySearchItem struct {
	DocumentID string  `json:"document_id"`
	Title      string  `json:"title"`
	VersionID  string  `json:"version_id"`
	Excerpt    string  `json:"excerpt"`
	Score      float64 `json:"score"`
}

type repositorySearchResponse struct {
	Query string                 `json:"query"`
	Items []repositorySearchItem `json:"items"`
}

type repositoryQuestionResponse struct {
	Question  string               `json:"question"`
	Answer    string               `json:"answer"`
	Citations []knowledge.Citation `json:"citations"`
}

type repositoryGraphNode struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Title string `json:"title"`
}

type repositoryGraphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"`
}

type repositoryGraphResponse struct {
	Nodes []repositoryGraphNode `json:"nodes"`
	Edges []repositoryGraphEdge `json:"edges"`
}

type repositoryBacklinksResponse struct {
	DocumentID string                `json:"document_id"`
	Items      []repositoryGraphEdge `json:"items"`
}

type reviewInboxItem struct {
	Repository        repo.RepositoryRecord `json:"repository"`
	PullRequest       repo.PullRequest      `json:"pull_request"`
	RequiredReviewers []string              `json:"required_reviewers"`
	RequiredTeams     []string              `json:"required_teams"`
}

type templateApplicationResponse struct {
	Status      string             `json:"status"`
	Template    knowledge.Template `json:"template"`
	Message     string             `json:"message,omitempty"`
	PullRequest *repo.PullRequest  `json:"pull_request,omitempty"`
}

type publicPortalRepository struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
}

type publicPortalDocument struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	VersionID string `json:"version_id"`
	Text      string `json:"text"`
}

type publicPortalResponse struct {
	Repository publicPortalRepository   `json:"repository"`
	Portal     knowledge.PortalSettings `json:"portal"`
	Documents  []publicPortalDocument   `json:"documents"`
}

func registerKnowledgeRoutes(authed *gin.RouterGroup, h handlers) {
	authed.GET("/repositories/:id/settings", h.getRepositorySettings)
	authed.PATCH("/repositories/:id/settings", h.patchRepositorySettings)
	authed.POST("/repositories/:id/archive", h.archiveRepository)
	authed.POST("/repositories/:id/restore", h.restoreRepository)
	authed.POST("/repositories/:id/transfer", h.transferRepository)
	authed.POST("/repositories/:id/transfer/accept", h.acceptRepositoryTransfer)
	authed.POST("/repositories/:id/transfer/cancel", h.cancelRepositoryTransfer)
	authed.GET("/repositories/:id/sources", h.listSources)
	authed.POST("/repositories/:id/sources", h.createSource)
	authed.PATCH("/repositories/:id/sources/:source_id", h.patchSource)
	authed.DELETE("/repositories/:id/sources/:source_id", h.deleteSource)
	authed.POST("/repositories/:id/sources/:source_id/check", h.checkSource)
	authed.POST("/repositories/:id/sources/:source_id/simulate", h.simulateSource)
	authed.POST("/repositories/:id/sources/:source_id/export", h.exportSource)
	authed.GET("/repositories/:id/sync-runs", h.listSyncRuns)
	authed.GET("/repositories/:id/activity", h.repositoryActivity)
	authed.GET("/repositories/:id/search", h.repositorySearch)
	authed.POST("/repositories/:id/questions", h.repositoryQuestion)
	authed.GET("/repositories/:id/backlinks", h.repositoryBacklinks)
	authed.GET("/repositories/:id/graph", h.repositoryGraph)
	authed.GET("/repositories/:id/teams", h.listTeams)
	authed.POST("/repositories/:id/teams", h.createTeam)
	authed.PATCH("/repositories/:id/teams/:team_id", h.patchTeam)
	authed.DELETE("/repositories/:id/teams/:team_id", h.deleteTeam)
	authed.PUT("/repositories/:id/teams/:team_id/members/:user_id", h.addTeamMember)
	authed.DELETE("/repositories/:id/teams/:team_id/members/:user_id", h.removeTeamMember)
	authed.GET("/notifications", h.listNotifications)
	authed.PATCH("/notifications/:notification_id", h.patchNotification)
	authed.GET("/review-inbox", h.reviewInbox)
	authed.GET("/templates", h.listTemplates)
	authed.POST("/repositories/:id/templates/:template_id/apply", h.applyTemplate)
}

func knowledgeID(prefix string) string {
	var data [10]byte
	if _, err := rand.Read(data[:]); err != nil {
		panic(err)
	}
	return prefix + "_" + hex.EncodeToString(data[:])
}

func (h handlers) knowledgeRole(c *gin.Context, repositoryID string) repo.Role {
	role, _ := h.deps.repository.RoleForRepository(c.Request.Context(), *current(c), repositoryID)
	return role
}

func (h handlers) needKnowledgeRole(c *gin.Context, repositoryID string, need repo.Role) bool {
	if !atLeast(h.knowledgeRole(c, repositoryID), need) {
		forbidden(c, string(need)+" role required")
		return false
	}
	return true
}

func (h handlers) addKnowledgeAudit(c *gin.Context, repositoryID, action, subjectType, subjectID string, details map[string]any) {
	_ = h.deps.knowledge.AddAudit(c.Request.Context(), knowledge.AuditEvent{
		ID: knowledgeID("audit"), RepositoryID: repositoryID, Action: action, Actor: *current(c),
		SubjectType: subjectType, SubjectID: subjectID, Details: details, CreatedAt: knowledge.Now(),
	})
}

// getRepositorySettings godoc
// @Summary Get connected-knowledge repository settings
// @Description Return archive state, review rules, path rules, portal configuration, freshness policy, and transfer state.
// @Tags connected-knowledge
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Repository ID"
// @Success 200 {object} knowledge.Settings
// @Failure 403 {object} map[string]interface{}
// @Router /v1/repositories/{id}/settings [get]
func (h handlers) getRepositorySettings(c *gin.Context) {
	id := c.Param("id")
	if !h.needKnowledgeRole(c, id, repo.RoleViewer) {
		return
	}
	settings, err := h.deps.knowledge.GetSettings(c.Request.Context(), id)
	if err != nil {
		internalErr(c, err)
		return
	}
	c.JSON(http.StatusOK, settings)
}

type settingsPatch struct {
	Archived          *bool                      `json:"archived"`
	RequiredReviewers *[]string                  `json:"required_reviewers"`
	RequiredTeams     *[]string                  `json:"required_teams"`
	PathRules         *[]knowledge.PathRule      `json:"path_rules"`
	Portal            *knowledge.PortalSettings  `json:"portal"`
	FreshnessPolicy   *knowledge.FreshnessPolicy `json:"freshness_policy"`
}

func validSettingsPatch(in settingsPatch) bool {
	if in.Portal != nil {
		if in.Portal.Visibility != "public" && in.Portal.Visibility != "organization" && in.Portal.Visibility != "private" {
			return false
		}
		if in.Portal.Enabled && knowledge.NormalizePortalSlug(in.Portal.Slug) == "" {
			return false
		}
	}
	if in.FreshnessPolicy != nil && (in.FreshnessPolicy.StaleAfterHours < 1 || (in.FreshnessPolicy.OnStale != "notify" && in.FreshnessPolicy.OnStale != "open_pull_request" && in.FreshnessPolicy.OnStale != "none")) {
		return false
	}
	for _, rule := range valueOr(in.PathRules, []knowledge.PathRule{}) {
		if strings.TrimSpace(rule.Pattern) == "" {
			return false
		}
	}
	return true
}

func valueOr[T any](p *T, fallback T) T {
	if p == nil {
		return fallback
	}
	return *p
}

// patchRepositorySettings godoc
// @Summary Update connected-knowledge repository settings
// @Description Update reviewer requirements, CODEOWNERS-style path rules, public portal options, freshness policy, or archive state. Owner access is required.
// @Tags connected-knowledge
// @Security bearerAuth
// @Security cookieAuth
// @Accept json
// @Produce json
// @Param id path string true "Repository ID"
// @Param body body settingsPatch true "Settings fields to update"
// @Success 200 {object} knowledge.Settings
// @Failure 400 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Failure 409 {object} map[string]interface{}
// @Router /v1/repositories/{id}/settings [patch]
func (h handlers) patchRepositorySettings(c *gin.Context) {
	id := c.Param("id")
	if !h.needKnowledgeRole(c, id, repo.RoleOwner) {
		return
	}
	var in settingsPatch
	if !requireJSON(c) {
		return
	}
	if err := c.ShouldBindJSON(&in); err != nil || !validSettingsPatch(in) {
		badRequest(c, "invalid repository settings")
		return
	}
	settings, err := h.deps.knowledge.GetSettings(c.Request.Context(), id)
	if err != nil {
		internalErr(c, err)
		return
	}
	if in.Archived != nil {
		settings.Archived = *in.Archived
	}
	if in.RequiredReviewers != nil {
		settings.RequiredReviewers = *in.RequiredReviewers
	}
	if in.RequiredTeams != nil {
		settings.RequiredTeams = *in.RequiredTeams
	}
	if in.PathRules != nil {
		settings.PathRules = *in.PathRules
	}
	if in.Portal != nil {
		settings.Portal = *in.Portal
	}
	if in.FreshnessPolicy != nil {
		settings.FreshnessPolicy = *in.FreshnessPolicy
	}
	settings.Portal.Slug = knowledge.NormalizePortalSlug(settings.Portal.Slug)
	settings, err = h.deps.knowledge.PutSettings(c.Request.Context(), settings)
	if errors.Is(err, knowledge.ErrConflict) {
		c.JSON(http.StatusConflict, errorResponse("portal_slug_taken", "another enabled portal already uses that address", nil))
		return
	}
	if err != nil {
		internalErr(c, err)
		return
	}
	h.addKnowledgeAudit(c, id, "repository.settings_updated", "repository", id, nil)
	c.JSON(http.StatusOK, settings)
}

func (h handlers) setArchived(c *gin.Context, archived bool) {
	id := c.Param("id")
	if !h.needKnowledgeRole(c, id, repo.RoleOwner) {
		return
	}
	settings, err := h.deps.knowledge.GetSettings(c.Request.Context(), id)
	if err != nil {
		internalErr(c, err)
		return
	}
	settings.Archived = archived
	settings, err = h.deps.knowledge.PutSettings(c.Request.Context(), settings)
	if err != nil {
		internalErr(c, err)
		return
	}
	action := "repository.restored"
	if archived {
		action = "repository.archived"
	}
	h.addKnowledgeAudit(c, id, action, "repository", id, nil)
	c.JSON(http.StatusOK, settings)
}

// archiveRepository godoc
// @Summary Archive a repository
// @Description Pause connected-source updates and mark the repository archived. Owner access is required.
// @Tags connected-knowledge
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Repository ID"
// @Success 200 {object} knowledge.Settings
// @Failure 403 {object} map[string]interface{}
// @Router /v1/repositories/{id}/archive [post]
func (h handlers) archiveRepository(c *gin.Context) { h.setArchived(c, true) }

// restoreRepository godoc
// @Summary Restore an archived repository
// @Description Resume connected-source updates for an archived repository. Owner access is required.
// @Tags connected-knowledge
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Repository ID"
// @Success 200 {object} knowledge.Settings
// @Failure 403 {object} map[string]interface{}
// @Router /v1/repositories/{id}/restore [post]
func (h handlers) restoreRepository(c *gin.Context) { h.setArchived(c, false) }

// transferRepository godoc
// @Summary Request repository ownership transfer
// @Description Create a pending repository ownership transfer for a user ID or email. The target must explicitly accept.
// @Tags connected-knowledge
// @Security bearerAuth
// @Security cookieAuth
// @Accept json
// @Produce json
// @Param id path string true "Repository ID"
// @Param body body object true "new_owner_id or new_owner_email"
// @Success 202 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /v1/repositories/{id}/transfer [post]
func (h handlers) transferRepository(c *gin.Context) {
	id := c.Param("id")
	if !h.needKnowledgeRole(c, id, repo.RoleOwner) {
		return
	}
	var in struct {
		NewOwnerID    string `json:"new_owner_id"`
		NewOwnerEmail string `json:"new_owner_email"`
	}
	if !requireJSON(c) {
		return
	}
	if err := c.ShouldBindJSON(&in); err != nil || (strings.TrimSpace(in.NewOwnerID) == "" && strings.TrimSpace(in.NewOwnerEmail) == "") {
		badRequest(c, "new_owner_id or new_owner_email is required")
		return
	}
	var target auth.Principal
	var err error
	if in.NewOwnerID != "" {
		target, err = h.deps.repository.ResolveUser(c.Request.Context(), strings.TrimSpace(in.NewOwnerID))
	} else {
		target, err = h.deps.repository.UserByEmail(c.Request.Context(), strings.ToLower(strings.TrimSpace(in.NewOwnerEmail)))
	}
	if err != nil {
		notFound(c)
		return
	}
	settings, err := h.deps.knowledge.GetSettings(c.Request.Context(), id)
	if err != nil {
		internalErr(c, err)
		return
	}
	if settings.Transfer.TemporaryGrantID != "" {
		if repository, repositoryErr := h.deps.repository.GetRepository(c.Request.Context(), id); repositoryErr == nil {
			_ = h.deps.repository.DeleteGrant(c.Request.Context(), repository.RootFolderID, settings.Transfer.TemporaryGrantID)
		}
	}
	temporaryGrantID := ""
	if role, _ := h.deps.repository.RoleForRepository(c.Request.Context(), target, id); role == repo.RoleNone {
		repository, repositoryErr := h.deps.repository.GetRepository(c.Request.Context(), id)
		if repositoryErr != nil {
			internalErr(c, repositoryErr)
			return
		}
		grant, grantErr := h.deps.repository.CreateGrant(c.Request.Context(), "folder", repository.RootFolderID, target, repo.RoleViewer, *current(c))
		if grantErr != nil {
			internalErr(c, grantErr)
			return
		}
		temporaryGrantID = grant.ID
	}
	settings.Transfer = knowledge.TransferState{OwnerID: target.ID, OwnerEmail: target.Email, Status: "pending", RequestedByID: current(c).ID, TemporaryGrantID: temporaryGrantID, UpdatedAt: knowledge.Now()}
	settings, err = h.deps.knowledge.PutSettings(c.Request.Context(), settings)
	if err != nil {
		internalErr(c, err)
		return
	}
	h.addKnowledgeAudit(c, id, "repository.transfer_requested", "repository", id, map[string]any{"new_owner_id": target.ID})
	_ = h.deps.knowledge.AddNotification(c.Request.Context(), knowledge.Notification{ID: knowledgeID("notification"), UserID: target.ID, RepositoryID: id, Type: "repository_transfer", Title: "Repository transfer requested", Body: "Accept the transfer to become the repository owner.", URL: "/app/repos/" + id + "/settings", CreatedAt: knowledge.Now()})
	c.JSON(http.StatusAccepted, repositoryTransferResponse{RepositoryID: id, Target: &target, Transfer: settings.Transfer})
}

// acceptRepositoryTransfer godoc
// @Summary Accept repository ownership transfer
// @Description Accept a pending ownership transfer as its target user and atomically transfer the repository tree.
// @Tags connected-knowledge
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Repository ID"
// @Success 200 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Router /v1/repositories/{id}/transfer/accept [post]
func (h handlers) acceptRepositoryTransfer(c *gin.Context) {
	id := c.Param("id")
	settings, err := h.deps.knowledge.GetSettings(c.Request.Context(), id)
	if err != nil {
		internalErr(c, err)
		return
	}
	if settings.Transfer.Status != "pending" || settings.Transfer.OwnerID != current(c).ID {
		forbidden(c, "only the pending transfer target can accept")
		return
	}
	repository, err := h.deps.repository.GetRepository(c.Request.Context(), id)
	if errors.Is(err, repo.ErrNotFound) {
		notFound(c)
		return
	}
	if err != nil {
		internalErr(c, err)
		return
	}
	updatedRepository, err := h.deps.repository.TransferRepositoryOwnership(c.Request.Context(), id, repository.CreatedBy, *current(c))
	if err != nil {
		switch {
		case errors.Is(err, repo.ErrForbidden):
			forbidden(c, "the repository owner changed before this transfer was accepted")
		case errors.Is(err, repo.ErrConflict):
			c.JSON(http.StatusConflict, errorResponse("transfer_conflict", "the target namespace already contains a conflicting repository or root folder", nil))
		case errors.Is(err, repo.ErrNotFound):
			notFound(c)
		default:
			internalErr(c, err)
		}
		return
	}
	if settings.Transfer.TemporaryGrantID != "" {
		_ = h.deps.repository.DeleteGrant(c.Request.Context(), repository.RootFolderID, settings.Transfer.TemporaryGrantID)
		settings.Transfer.TemporaryGrantID = ""
	}
	settings.Transfer.Status = "accepted"
	settings.Transfer.UpdatedAt = knowledge.Now()
	settings, err = h.deps.knowledge.PutSettings(c.Request.Context(), settings)
	if err != nil {
		internalErr(c, err)
		return
	}
	h.addKnowledgeAudit(c, id, "repository.transfer_accepted", "repository", id, map[string]any{"new_owner_id": current(c).ID})
	c.JSON(http.StatusOK, repositoryTransferResponse{RepositoryID: id, Repository: &updatedRepository, Transfer: settings.Transfer})
}

// cancelRepositoryTransfer godoc
// @Summary Cancel repository ownership transfer
// @Description Cancel a pending connected-knowledge ownership transfer. Owner access is required.
// @Tags connected-knowledge
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Repository ID"
// @Success 200 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Router /v1/repositories/{id}/transfer/cancel [post]
func (h handlers) cancelRepositoryTransfer(c *gin.Context) {
	id := c.Param("id")
	if !h.needKnowledgeRole(c, id, repo.RoleOwner) {
		return
	}
	settings, err := h.deps.knowledge.GetSettings(c.Request.Context(), id)
	if err != nil {
		internalErr(c, err)
		return
	}
	if settings.Transfer.TemporaryGrantID != "" {
		if repository, repositoryErr := h.deps.repository.GetRepository(c.Request.Context(), id); repositoryErr == nil {
			_ = h.deps.repository.DeleteGrant(c.Request.Context(), repository.RootFolderID, settings.Transfer.TemporaryGrantID)
		}
	}
	settings.Transfer = knowledge.TransferState{}
	settings, err = h.deps.knowledge.PutSettings(c.Request.Context(), settings)
	if err != nil {
		internalErr(c, err)
		return
	}
	h.addKnowledgeAudit(c, id, "repository.transfer_cancelled", "repository", id, nil)
	c.JSON(http.StatusOK, repositoryTransferResponse{RepositoryID: id, Transfer: settings.Transfer})
}

type sourceInput struct {
	Provider           string   `json:"provider"`
	ExternalRepository string   `json:"external_repository"`
	Branch             string   `json:"branch"`
	IncludePaths       []string `json:"include_paths"`
	ExcludePaths       []string `json:"exclude_paths"`
	TargetFolderID     string   `json:"target_folder_id"`
	TargetDocumentID   string   `json:"target_document_id"`
	TriggerMode        string   `json:"trigger_mode"`
	Direction          string   `json:"direction"`
}

func normalizeSourceInput(in sourceInput) (sourceInput, error) {
	in.Provider = strings.ToLower(strings.TrimSpace(in.Provider))
	if !knowledge.Providers[in.Provider] {
		return in, errors.New("unsupported provider")
	}
	in.ExternalRepository = strings.Trim(strings.TrimSpace(in.ExternalRepository), "/")
	in.Branch = strings.TrimSpace(in.Branch)
	eventProvider := isEventSourceProvider(in.Provider)
	if in.Branch == "" && !eventProvider {
		in.Branch = "main"
	}
	if in.TriggerMode == "" {
		in.TriggerMode = "default_branch"
		if eventProvider {
			in.TriggerMode = "event"
		}
	}
	if !validSourceTriggerMode(in.TriggerMode) {
		return in, errors.New("invalid trigger mode")
	}
	if eventProvider && in.TriggerMode != "event" {
		return in, fmt.Errorf("%s sources require event trigger mode", in.Provider)
	}
	if in.Direction == "" {
		in.Direction = "into_aidocs"
	}
	if !validSourceDirection(in.Direction) {
		return in, errors.New("invalid direction")
	}
	if sourceProviderRequiresExternalRepository(in.Provider) && in.ExternalRepository == "" {
		return in, errors.New("external_repository is required")
	}
	if in.Provider == knowledge.ProviderSlack {
		if err := validateSlackSourceIdentity(in.ExternalRepository); err != nil {
			return in, err
		}
	}
	return in, nil
}

func isEventSourceProvider(provider string) bool {
	return provider == knowledge.ProviderSlack || provider == knowledge.ProviderDevRev
}

func validSourceTriggerMode(mode string) bool {
	switch mode {
	case "default_branch", "merged_pull_requests", "manual", "scheduled", "event":
		return true
	default:
		return false
	}
}

func validSourceDirection(direction string) bool {
	switch direction {
	case "into_aidocs", "bidirectional", "export_only":
		return true
	default:
		return false
	}
}

func sourceProviderRequiresExternalRepository(provider string) bool {
	switch provider {
	case knowledge.ProviderManual, knowledge.ProviderGoogleDrive, knowledge.ProviderConfluence, knowledge.ProviderNotion:
		return false
	default:
		return true
	}
}

func validateSlackSourceIdentity(identity string) error {
	parts := strings.Split(identity, "/")
	if (len(parts) != 2 && len(parts) != 3) || parts[0] == "" || parts[1] == "" || (len(parts) == 3 && parts[2] == "") {
		return errors.New("slack external_repository must be TEAM_ID/CHANNEL_ID or TEAM_ID/CHANNEL_ID/THREAD_TS")
	}
	for _, part := range parts {
		if strings.ContainsAny(part, " \t\r\n") {
			return errors.New("slack source identity cannot contain whitespace")
		}
	}
	return nil
}

// listSources godoc
// @Summary List connected sources
// @Description List GitHub, Slack, DevRev, Drive, Confluence, Notion, and manual sources attached to a repository.
// @Tags connected-sources
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Repository ID"
// @Success 200 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Router /v1/repositories/{id}/sources [get]
func (h handlers) listSources(c *gin.Context) {
	id := c.Param("id")
	if !h.needKnowledgeRole(c, id, repo.RoleViewer) {
		return
	}
	items, err := h.deps.knowledge.ListSources(c.Request.Context(), id)
	if err != nil {
		internalErr(c, err)
		return
	}
	c.JSON(http.StatusOK, listResponse[knowledge.Source]{Items: items})
}

// createSource godoc
// @Summary Connect a knowledge source
// @Description Connect a provider repository or knowledge source and configure branch, path filters, target, trigger mode, and synchronization direction.
// @Tags connected-sources
// @Security bearerAuth
// @Security cookieAuth
// @Accept json
// @Produce json
// @Param id path string true "Repository ID"
// @Param body body sourceInput true "Source configuration"
// @Success 201 {object} knowledge.Source
// @Failure 400 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Failure 409 {object} map[string]interface{}
// @Router /v1/repositories/{id}/sources [post]
func (h handlers) createSource(c *gin.Context) {
	id := c.Param("id")
	if !h.needKnowledgeRole(c, id, repo.RoleOwner) {
		return
	}
	settings, _ := h.deps.knowledge.GetSettings(c.Request.Context(), id)
	if settings.Archived {
		c.JSON(http.StatusConflict, errorResponse("repository_archived", "restore the repository before adding a source", nil))
		return
	}
	var in sourceInput
	if !requireJSON(c) {
		return
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		badRequest(c, "invalid source")
		return
	}
	in, err := normalizeSourceInput(in)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	now := knowledge.Now()
	source := knowledge.Source{ID: knowledgeID("src"), RepositoryID: id, Provider: in.Provider, ExternalRepository: in.ExternalRepository, Branch: in.Branch, IncludePaths: in.IncludePaths, ExcludePaths: in.ExcludePaths, TargetFolderID: in.TargetFolderID, TargetDocumentID: in.TargetDocumentID, TriggerMode: in.TriggerMode, Direction: in.Direction, Status: "active", Freshness: knowledge.Freshness{State: "never_synced", StaleAfterHours: settings.FreshnessPolicy.StaleAfterHours}, CreatedBy: *current(c), CreatedAt: now, UpdatedAt: now}
	source, err = h.deps.knowledge.PutSource(c.Request.Context(), source)
	if err != nil {
		internalErr(c, err)
		return
	}
	h.addKnowledgeAudit(c, id, "source.created", "source", source.ID, map[string]any{"provider": source.Provider})
	c.JSON(http.StatusCreated, source)
}

// patchSource godoc
// @Summary Update a connected source
// @Description Update provider location, branch, path filters, target, trigger, direction, or active status. Owner access is required.
// @Tags connected-sources
// @Security bearerAuth
// @Security cookieAuth
// @Accept json
// @Produce json
// @Param id path string true "Repository ID"
// @Param source_id path string true "Source ID"
// @Param body body object true "Source fields to update"
// @Success 200 {object} knowledge.Source
// @Failure 400 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /v1/repositories/{id}/sources/{source_id} [patch]
//
//nolint:gocyclo // Updating a source deliberately validates all optional fields in one atomic handler.
func (h handlers) patchSource(c *gin.Context) {
	id := c.Param("id")
	if !h.needKnowledgeRole(c, id, repo.RoleOwner) {
		return
	}
	existing, err := h.deps.knowledge.GetSource(c.Request.Context(), id, c.Param("source_id"))
	if errors.Is(err, knowledge.ErrNotFound) {
		notFound(c)
		return
	}
	if err != nil {
		internalErr(c, err)
		return
	}
	var in struct {
		ExternalRepository *string   `json:"external_repository"`
		Branch             *string   `json:"branch"`
		IncludePaths       *[]string `json:"include_paths"`
		ExcludePaths       *[]string `json:"exclude_paths"`
		TargetFolderID     *string   `json:"target_folder_id"`
		TargetDocumentID   *string   `json:"target_document_id"`
		TriggerMode        *string   `json:"trigger_mode"`
		Direction          *string   `json:"direction"`
		Status             *string   `json:"status"`
	}
	if !requireJSON(c) {
		return
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		badRequest(c, "invalid source")
		return
	}
	if in.ExternalRepository != nil {
		existing.ExternalRepository = strings.Trim(*in.ExternalRepository, " /")
	}
	if in.Branch != nil {
		existing.Branch = strings.TrimSpace(*in.Branch)
	}
	if in.IncludePaths != nil {
		existing.IncludePaths = *in.IncludePaths
	}
	if in.ExcludePaths != nil {
		existing.ExcludePaths = *in.ExcludePaths
	}
	if in.TargetFolderID != nil {
		existing.TargetFolderID = *in.TargetFolderID
	}
	if in.TargetDocumentID != nil {
		existing.TargetDocumentID = *in.TargetDocumentID
	}
	if in.TriggerMode != nil {
		existing.TriggerMode = *in.TriggerMode
	}
	if in.Direction != nil {
		existing.Direction = *in.Direction
	}
	if in.Status != nil {
		if *in.Status != "active" && *in.Status != "paused" {
			badRequest(c, "status must be active or paused")
			return
		}
		existing.Status = *in.Status
	}
	_, err = normalizeSourceInput(sourceInput{Provider: existing.Provider, ExternalRepository: existing.ExternalRepository, Branch: existing.Branch, IncludePaths: existing.IncludePaths, ExcludePaths: existing.ExcludePaths, TargetFolderID: existing.TargetFolderID, TargetDocumentID: existing.TargetDocumentID, TriggerMode: existing.TriggerMode, Direction: existing.Direction})
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	existing.UpdatedAt = knowledge.Now()
	existing, err = h.deps.knowledge.PutSource(c.Request.Context(), existing)
	if err != nil {
		internalErr(c, err)
		return
	}
	h.addKnowledgeAudit(c, id, "source.updated", "source", existing.ID, nil)
	c.JSON(http.StatusOK, existing)
}

// deleteSource godoc
// @Summary Disconnect a knowledge source
// @Description Remove a connected source without deleting its historical sync runs or reviewed documentation.
// @Tags connected-sources
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Repository ID"
// @Param source_id path string true "Source ID"
// @Success 204
// @Failure 403 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /v1/repositories/{id}/sources/{source_id} [delete]
func (h handlers) deleteSource(c *gin.Context) {
	id := c.Param("id")
	if !h.needKnowledgeRole(c, id, repo.RoleOwner) {
		return
	}
	err := h.deps.knowledge.DeleteSource(c.Request.Context(), id, c.Param("source_id"))
	if errors.Is(err, knowledge.ErrNotFound) {
		notFound(c)
		return
	}
	if err != nil {
		internalErr(c, err)
		return
	}
	h.addKnowledgeAudit(c, id, "source.deleted", "source", c.Param("source_id"), nil)
	c.Status(http.StatusNoContent)
}

type sourceCheckInput struct {
	BeforeSHA    string   `json:"before_sha"`
	AfterSHA     string   `json:"after_sha"`
	ChangedFiles []string `json:"changed_files"`
	DeliveryID   string   `json:"delivery_id"`
}

func globMatch(pattern, name string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return false
	}
	if strings.HasSuffix(pattern, "/**") {
		return strings.HasPrefix(name, strings.TrimSuffix(pattern, "**"))
	}
	ok, _ := path.Match(pattern, name)
	return ok || strings.HasPrefix(name, strings.TrimSuffix(pattern, "*"))
}
func filterSourceFiles(source knowledge.Source, files []string) []string {
	out := []string{}
	for _, file := range files {
		included := len(source.IncludePaths) == 0
		for _, p := range source.IncludePaths {
			if globMatch(p, file) {
				included = true
				break
			}
		}
		for _, p := range source.ExcludePaths {
			if globMatch(p, file) {
				included = false
				break
			}
		}
		if included {
			out = append(out, file)
		}
	}
	sort.Strings(out)
	return out
}

//nolint:gocyclo // Preview and persistence share one ordered source-sync state machine so their outcomes cannot drift.
func (h handlers) sourcePreview(c *gin.Context, persist bool) {
	id := c.Param("id")
	if !h.needKnowledgeRole(c, id, repo.RoleEditor) {
		return
	}
	source, err := h.deps.knowledge.GetSource(c.Request.Context(), id, c.Param("source_id"))
	if errors.Is(err, knowledge.ErrNotFound) {
		notFound(c)
		return
	}
	if err != nil {
		internalErr(c, err)
		return
	}
	if source.Status != "active" {
		c.JSON(http.StatusConflict, errorResponse("source_paused", "activate the source before checking it", nil))
		return
	}
	var in sourceCheckInput
	if c.Request.ContentLength != 0 {
		if !requireJSON(c) {
			return
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			badRequest(c, "invalid source check")
			return
		}
	}
	if len(in.ChangedFiles) == 0 && in.BeforeSHA != "" && in.AfterSHA != "" {
		comparison, compareErr := h.deps.providerClient.Compare(c.Request.Context(), source, in.BeforeSHA, in.AfterSHA)
		if compareErr != nil {
			if persist {
				run := knowledge.SyncRun{ID: knowledgeID("sync"), RepositoryID: id, SourceID: source.ID, BeforeSHA: in.BeforeSHA, AfterSHA: in.AfterSHA, Status: "failed", Outcome: "failure", Message: compareErr.Error(), ChangedFiles: []string{}, StartedAt: knowledge.Now(), FinishedAt: knowledge.Now()}
				_, _ = h.deps.knowledge.AddSyncRun(c.Request.Context(), run)
			}
			c.JSON(http.StatusBadGateway, errorResponse("provider_failed", "could not compare provider revisions", nil))
			return
		}
		in.ChangedFiles = comparison.ChangedFiles
	}
	files := filterSourceFiles(source, in.ChangedFiles)
	run := knowledge.SyncRun{ID: knowledgeID("sync"), RepositoryID: id, SourceID: source.ID, DeliveryID: in.DeliveryID, BeforeSHA: in.BeforeSHA, AfterSHA: in.AfterSHA, ChangedFiles: files, Status: "completed", StartedAt: knowledge.Now(), FinishedAt: knowledge.Now(), Provenance: []knowledge.Citation{}}
	if persist && in.AfterSHA != "" {
		if prior, listErr := h.deps.knowledge.ListSyncRuns(c.Request.Context(), id); listErr == nil {
			for _, item := range prior {
				if item.SourceID == source.ID && item.AfterSHA == in.AfterSHA && item.Status == "completed" {
					run.Outcome = "no_op"
					run.Message = "This provider revision was already processed."
					files = nil
					break
				}
			}
		}
	}
	if run.Outcome == "" {
		switch {
		case len(files) == 0:
			run.Outcome = "no_op"
			run.Message = "No included files changed."
		case source.TargetDocumentID == "":
			run.Outcome = "preview"
			run.Message = "Meaningful source changes found; configure target_document_id to open an aidocs pull request."
		default:
			document, docErr := h.deps.repository.GetDocument(c.Request.Context(), source.TargetDocumentID)
			if docErr != nil {
				run.Status = "failed"
				run.Outcome = "failure"
				run.Message = "target document not found"
			} else {
				base, versionErr := h.deps.repository.GetVersion(c.Request.Context(), document.CurrentVersionID)
				body, htmlErr := h.deps.repository.GetVersionHTML(c.Request.Context(), document.CurrentVersionID)
				switch {
				case versionErr != nil || htmlErr != nil:
					run.Status = "failed"
					run.Outcome = "failure"
					run.Message = "could not read target document"
				case !persist:
					run.Outcome = "preview"
					run.Message = "Documentation changes are available to preview; no pull request was created."
					run.Provenance = []knowledge.Citation{{DocumentID: document.ID, Title: document.Title, VersionID: base.ID, SourceID: source.ID, Provider: source.Provider, URL: providerRevisionURL(source, in.AfterSHA)}}
				default:
					summary := fmt.Sprintf("Sync %s/%s through %s", source.Provider, source.ExternalRepository, in.AfterSHA)
					generated := appendSourceSummary(body, source, in.AfterSHA, files)
					pr, updated, prErr := upsertConnectedKnowledgePullRequest(c.Request.Context(), h.deps, source, document, base, summary, generated)
					if prErr != nil {
						run.Status = "failed"
						run.Outcome = "failure"
						run.Message = prErr.Error()
					} else {
						run.Outcome = "pull_request_opened"
						run.PullRequestNumber = pr.Number
						run.Message = "Documentation changes are ready for review."
						if updated {
							run.Outcome = "pull_request_updated"
							run.Message = "The open documentation pull request has a new patchset ready for review."
						}
						run.Provenance = []knowledge.Citation{{DocumentID: document.ID, Title: document.Title, VersionID: pr.HeadVersionID, SourceID: source.ID, Provider: source.Provider, URL: providerRevisionURL(source, in.AfterSHA)}}
					}
				}
			}
		}
	}
	if persist {
		run, _ = h.deps.knowledge.AddSyncRun(c.Request.Context(), run)
		source.Freshness = knowledge.Freshness{State: "fresh", CheckedAt: knowledge.Now(), StaleAfterHours: source.Freshness.StaleAfterHours}
		source.LastSyncedAt = knowledge.Now()
		source.UpdatedAt = knowledge.Now()
		_, _ = h.deps.knowledge.PutSource(c.Request.Context(), source)
		h.addKnowledgeAudit(c, id, "source.checked", "source", source.ID, map[string]any{"outcome": run.Outcome})
	}
	c.JSON(http.StatusOK, run)
}

func appendSourceSummary(existing []byte, source knowledge.Source, sha string, files []string, contextItems ...knowledge.ContextItem) []byte {
	escapedID := htmlEscape(source.ID)
	startMarker := "<!-- aidocs-source:start:" + escapedID + " -->"
	endMarker := "<!-- aidocs-source:end:" + escapedID + " -->"
	section := startMarker + "<section data-aidocs-source=\"" + escapedID + "\"><h2>Source update: " + htmlEscape(source.ExternalRepository) + "</h2><p>Revision " + htmlEscape(sha) + " changed:</p><ul>"
	for _, file := range files {
		section += "<li><code>" + htmlEscape(file) + "</code></li>"
	}
	section += "</ul>"
	for _, item := range contextItems {
		section = appendSourceContext(section, item)
	}
	section += "</section>" + endMarker
	body := string(existing)
	if start := strings.Index(body, startMarker); start >= 0 {
		if endRelative := strings.Index(body[start:], endMarker); endRelative >= 0 {
			end := start + endRelative + len(endMarker)
			body = body[:start] + section + body[end:]
			return []byte(body)
		}
	}
	legacyStart := "<section data-aidocs-source=\"" + escapedID + "\">"
	if start := strings.Index(body, legacyStart); start >= 0 {
		if endRelative := strings.Index(strings.ToLower(body[start:]), "</section>"); endRelative >= 0 {
			end := start + endRelative + len("</section>")
			body = body[:start] + section + body[end:]
			return []byte(body)
		}
	}
	if i := strings.LastIndex(strings.ToLower(body), "</body>"); i >= 0 {
		body = body[:i] + section + body[i:]
	} else {
		body += section
	}
	return []byte(body)
}

func appendSourceContext(section string, item knowledge.ContextItem) string {
	title := strings.TrimSpace(item.Title)
	if title == "" {
		title = strings.TrimSpace(item.ExternalID)
	}
	if title == "" {
		title = "Source context"
	}
	section += "<article data-aidocs-context=\"" + htmlEscape(item.Kind) + "\"><h3>" + htmlEscape(title) + "</h3>"
	if item.ExternalID != "" && item.ExternalID != title {
		section += "<p><code>" + htmlEscape(item.ExternalID) + "</code></p>"
	}
	if body := strings.TrimSpace(item.Body); body != "" {
		section += "<p>" + strings.ReplaceAll(htmlEscape(body), "\n", "<br>") + "</p>"
	}
	keys := make([]string, 0, len(item.Metadata))
	for key := range item.Metadata {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) > 0 {
		section += "<dl>"
		for _, key := range keys {
			section += "<dt>" + htmlEscape(key) + "</dt><dd>" + htmlEscape(item.Metadata[key]) + "</dd>"
		}
		section += "</dl>"
	}
	return section + "</article>"
}

func htmlEscape(value string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;", "'", "&#39;").Replace(value)
}
func providerRevisionURL(source knowledge.Source, sha string) string {
	if source.Provider == knowledge.ProviderGitHub && source.ExternalRepository != "" && sha != "" {
		return "https://github.com/" + source.ExternalRepository + "/commit/" + sha
	}
	return ""
}

// checkSource godoc
// @Summary Check and synchronize a source
// @Description Compare revisions or supplied changed paths, apply filters, and create or update a reviewable aidocs pull request when documentation changes are meaningful.
// @Tags connected-sources
// @Security bearerAuth
// @Security cookieAuth
// @Accept json
// @Produce json
// @Param id path string true "Repository ID"
// @Param source_id path string true "Source ID"
// @Param body body sourceCheckInput false "Revision comparison or changed paths"
// @Success 200 {object} knowledge.SyncRun
// @Failure 400 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 409 {object} map[string]interface{}
// @Failure 502 {object} map[string]interface{}
// @Router /v1/repositories/{id}/sources/{source_id}/check [post]
func (h handlers) checkSource(c *gin.Context) { h.sourcePreview(c, true) }

// simulateSource godoc
// @Summary Simulate a source synchronization
// @Description Preview filtered source changes and their target without persisting a run, version, pull request, source update, or audit event.
// @Tags connected-sources
// @Security bearerAuth
// @Security cookieAuth
// @Accept json
// @Produce json
// @Param id path string true "Repository ID"
// @Param source_id path string true "Source ID"
// @Param body body sourceCheckInput false "Revision comparison or changed paths"
// @Success 200 {object} knowledge.SyncRun
// @Failure 400 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 409 {object} map[string]interface{}
// @Failure 502 {object} map[string]interface{}
// @Router /v1/repositories/{id}/sources/{source_id}/simulate [post]
func (h handlers) simulateSource(c *gin.Context) { h.sourcePreview(c, false) }

// listSyncRuns godoc
// @Summary List source synchronization runs
// @Description Return queued, processing, completed, no-op, preview, and failed synchronization history for a repository.
// @Tags connected-sources
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Repository ID"
// @Success 200 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Router /v1/repositories/{id}/sync-runs [get]
func (h handlers) listSyncRuns(c *gin.Context) {
	id := c.Param("id")
	if !h.needKnowledgeRole(c, id, repo.RoleViewer) {
		return
	}
	items, err := h.deps.knowledge.ListSyncRuns(c.Request.Context(), id)
	if err != nil {
		internalErr(c, err)
		return
	}
	c.JSON(http.StatusOK, listResponse[knowledge.SyncRun]{Items: items})
}

// exportSource godoc
// @Summary Propose an aidocs-to-provider export
// @Description Build a reviewable provider change proposal without publishing or mutating the connected provider.
// @Tags connected-sources
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Repository ID"
// @Param source_id path string true "Source ID"
// @Success 200 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /v1/repositories/{id}/sources/{source_id}/export [post]
func (h handlers) exportSource(c *gin.Context) {
	id := c.Param("id")
	if !h.needKnowledgeRole(c, id, repo.RoleEditor) {
		return
	}
	source, err := h.deps.knowledge.GetSource(c.Request.Context(), id, c.Param("source_id"))
	if errors.Is(err, knowledge.ErrNotFound) {
		notFound(c)
		return
	}
	if err != nil {
		internalErr(c, err)
		return
	}
	documents, err := h.deps.repository.ListRepositoryDocuments(c.Request.Context(), *current(c), id)
	if err != nil {
		internalErr(c, err)
		return
	}
	files := []string{}
	for _, document := range documents {
		files = append(files, slugifyRepository(document.Title)+".html")
	}
	proposal := knowledge.ChangeProposal{ID: knowledgeID("proposal"), RepositoryID: id, SourceID: source.ID, Provider: source.Provider, Direction: "from_aidocs", Title: "Export aidocs documentation to " + source.ExternalRepository, Summary: "Review this provider change proposal before publishing. No provider content has been changed.", ChangedFiles: files, Status: "proposed", CreatedAt: knowledge.Now()}
	h.addKnowledgeAudit(c, id, "source.export_proposed", "source", source.ID, map[string]any{"proposal_id": proposal.ID})
	c.JSON(http.StatusOK, changeProposalResponse{Proposal: proposal})
}

// githubWebhook godoc
// @Summary Receive GitHub source events
// @Description Validate a GitHub HMAC signature, deduplicate the delivery, and durably queue default-branch pushes or merged pull requests for asynchronous documentation impact analysis.
// @Tags connected-source-webhooks
// @Accept json
// @Produce json
// @Param X-Hub-Signature-256 header string true "sha256 HMAC signature"
// @Param X-GitHub-Delivery header string true "Unique GitHub delivery ID"
// @Param X-GitHub-Event header string true "GitHub event name"
// @Param body body object true "GitHub webhook payload"
// @Success 200 {object} map[string]interface{} "Duplicate delivery"
// @Success 202 {object} map[string]interface{} "Accepted, queued, or intentionally ignored delivery"
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 503 {object} map[string]interface{}
// @Router /v1/webhooks/github [post]
//
//nolint:gocyclo // Signature checks, delivery deduplication, and source dispatch form one ordered webhook pipeline.
func (h handlers) githubWebhook(c *gin.Context) {
	secret := h.deps.githubWebhookSecret
	if secret == "" {
		c.JSON(http.StatusServiceUnavailable, errorResponse("webhook_not_configured", "GitHub webhook secret is not configured", nil))
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		badRequest(c, "could not read webhook")
		return
	}
	signature := strings.TrimSpace(c.GetHeader("X-Hub-Signature-256"))
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(signature), []byte(expected)) {
		c.JSON(http.StatusUnauthorized, errorResponse("invalid_signature", "invalid GitHub webhook signature", nil))
		return
	}
	delivery := strings.TrimSpace(c.GetHeader("X-GitHub-Delivery"))
	if delivery == "" {
		badRequest(c, "X-GitHub-Delivery is required")
		return
	}
	event := strings.TrimSpace(c.GetHeader("X-GitHub-Event"))
	var payload struct {
		Action     string `json:"action"`
		Ref        string `json:"ref"`
		Before     string `json:"before"`
		After      string `json:"after"`
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
		Commits []struct {
			Added    []string `json:"added"`
			Modified []string `json:"modified"`
			Removed  []string `json:"removed"`
		} `json:"commits"`
		PullRequest struct {
			Merged         bool   `json:"merged"`
			MergeCommitSHA string `json:"merge_commit_sha"`
			Base           struct {
				Ref string `json:"ref"`
				SHA string `json:"sha"`
			} `json:"base"`
		} `json:"pull_request"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		badRequest(c, "invalid GitHub payload")
		return
	}
	branch := strings.TrimPrefix(payload.Ref, "refs/heads/")
	before, after := payload.Before, payload.After
	requiredTrigger := "default_branch"
	ignored := false
	switch event {
	case "push":
		if payload.Repository.FullName == "" || !strings.HasPrefix(payload.Ref, "refs/heads/") {
			badRequest(c, "invalid GitHub push payload")
			return
		}
	case "pull_request":
		if payload.Repository.FullName == "" || payload.Action == "" {
			badRequest(c, "invalid GitHub pull request payload")
			return
		}
		if payload.Action != "closed" || !payload.PullRequest.Merged {
			ignored = true
			break
		}
		branch = payload.PullRequest.Base.Ref
		before = payload.PullRequest.Base.SHA
		after = payload.PullRequest.MergeCommitSHA
		requiredTrigger = "merged_pull_requests"
		if branch == "" || after == "" {
			badRequest(c, "invalid merged GitHub pull request payload")
			return
		}
	default:
		ignored = true
	}
	if ignored {
		queueSourceDelivery(c, h, knowledge.ProviderGitHub, delivery, nil, true)
		return
	}
	changedFiles := []string{}
	for _, commit := range payload.Commits {
		changedFiles = append(changedFiles, commit.Added...)
		changedFiles = append(changedFiles, commit.Modified...)
		changedFiles = append(changedFiles, commit.Removed...)
	}
	changedFiles = uniqueStrings(changedFiles)
	sources, err := h.deps.knowledge.ListAllSources(c.Request.Context())
	if err != nil {
		internalErr(c, err)
		return
	}
	runs := []knowledge.SyncRun{}
	for _, source := range sources {
		if source.Provider != knowledge.ProviderGitHub || source.Status != "active" || source.Direction == "export_only" || !strings.EqualFold(source.ExternalRepository, payload.Repository.FullName) || source.Branch != branch || source.TriggerMode != requiredTrigger {
			continue
		}
		runs = append(runs, knowledge.SyncRun{ID: knowledgeID("sync"), RepositoryID: source.RepositoryID, SourceID: source.ID, DeliveryID: delivery, BeforeSHA: before, AfterSHA: after, ChangedFiles: changedFiles, Status: "queued", Outcome: "pending", Message: "GitHub event accepted for asynchronous documentation impact analysis.", Provenance: []knowledge.Citation{{SourceID: source.ID, Provider: knowledge.ProviderGitHub, URL: providerRevisionURL(source, after)}}, StartedAt: knowledge.Now()})
	}
	queueSourceDelivery(c, h, knowledge.ProviderGitHub, delivery, runs, len(runs) == 0)
}

// repositoryActivity godoc
// @Summary List repository audit activity
// @Description Return owner, source, sync, team, template, and export activity visible to repository viewers.
// @Tags knowledge-discovery
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Repository ID"
// @Success 200 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Router /v1/repositories/{id}/activity [get]
func (h handlers) repositoryActivity(c *gin.Context) {
	id := c.Param("id")
	if !h.needKnowledgeRole(c, id, repo.RoleViewer) {
		return
	}
	items, err := h.deps.knowledge.ListAudit(c.Request.Context(), id)
	if err != nil {
		internalErr(c, err)
		return
	}
	c.JSON(http.StatusOK, listResponse[knowledge.AuditEvent]{Items: items})
}

type knowledgeDocument struct {
	Document repo.Document
	Text     string
}

func plainHTML(body []byte) string {
	node, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return strings.Join(strings.Fields(string(body)), " ")
	}
	parts := []string{}
	var visit func(*html.Node, bool)
	visit = func(n *html.Node, hidden bool) {
		if n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style") {
			hidden = true
		}
		if n.Type == html.TextNode && !hidden {
			value := strings.TrimSpace(n.Data)
			if value != "" {
				parts = append(parts, value)
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			visit(child, hidden)
		}
	}
	visit(node, false)
	return strings.Join(strings.Fields(strings.Join(parts, " ")), " ")
}

func (h handlers) repositoryCorpus(c *gin.Context, repositoryID string) ([]knowledgeDocument, error) {
	documents, err := h.deps.repository.ListRepositoryDocuments(c.Request.Context(), *current(c), repositoryID)
	if err != nil {
		return nil, err
	}
	out := make([]knowledgeDocument, 0, len(documents))
	for _, document := range documents {
		body, htmlErr := h.deps.repository.GetVersionHTML(c.Request.Context(), document.CurrentVersionID)
		if htmlErr != nil {
			continue
		}
		out = append(out, knowledgeDocument{Document: document, Text: plainHTML(body)})
	}
	return out, nil
}

func snippet(text string, at int) string {
	r := []rune(text)
	if len(r) <= 260 {
		return text
	}
	start := at - 80
	if start < 0 {
		start = 0
	}
	end := start + 260
	if end > len(r) {
		end = len(r)
	}
	return strings.TrimSpace(string(r[start:end]))
}

// repositorySearch godoc
// @Summary Search repository documents
// @Description Search document titles and rendered text within one accessible repository.
// @Tags knowledge-discovery
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Repository ID"
// @Param q query string true "Search query"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Router /v1/repositories/{id}/search [get]
func (h handlers) repositorySearch(c *gin.Context) {
	id := c.Param("id")
	if !h.needKnowledgeRole(c, id, repo.RoleViewer) {
		return
	}
	query := strings.TrimSpace(c.Query("q"))
	if query == "" {
		badRequest(c, "q is required")
		return
	}
	corpus, err := h.repositoryCorpus(c, id)
	if err != nil {
		internalErr(c, err)
		return
	}
	needle := strings.ToLower(query)
	items := []repositorySearchItem{}
	for _, item := range corpus {
		haystack := strings.ToLower(item.Document.Title + " " + item.Text)
		at := strings.Index(haystack, needle)
		if at < 0 {
			continue
		}
		items = append(items, repositorySearchItem{DocumentID: item.Document.ID, Title: item.Document.Title, VersionID: item.Document.CurrentVersionID, Excerpt: snippet(item.Text, at), Score: 1})
	}
	c.JSON(http.StatusOK, repositorySearchResponse{Query: query, Items: items})
}

func keywords(value string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, word := range strings.FieldsFunc(strings.ToLower(value), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if len(word) < 3 || seen[word] {
			continue
		}
		seen[word] = true
		out = append(out, word)
	}
	return out
}

// repositoryQuestion godoc
// @Summary Ask a cited repository question
// @Description Return a deterministic extractive answer with document and version citations from accessible repository content.
// @Tags knowledge-discovery
// @Security bearerAuth
// @Security cookieAuth
// @Accept json
// @Produce json
// @Param id path string true "Repository ID"
// @Param body body object true "Question payload with a question field"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Router /v1/repositories/{id}/questions [post]
func (h handlers) repositoryQuestion(c *gin.Context) {
	id := c.Param("id")
	if !h.needKnowledgeRole(c, id, repo.RoleViewer) {
		return
	}
	var in struct {
		Question string `json:"question"`
	}
	if !requireJSON(c) {
		return
	}
	if err := c.ShouldBindJSON(&in); err != nil || strings.TrimSpace(in.Question) == "" {
		badRequest(c, "question is required")
		return
	}
	corpus, err := h.repositoryCorpus(c, id)
	if err != nil {
		internalErr(c, err)
		return
	}
	terms := keywords(in.Question)
	type scored struct {
		Item  knowledgeDocument
		Score int
	}
	scores := []scored{}
	for _, item := range corpus {
		hay := strings.ToLower(item.Document.Title + " " + item.Text)
		score := 0
		for _, term := range terms {
			score += strings.Count(hay, term)
		}
		if score > 0 {
			scores = append(scores, scored{item, score})
		}
	}
	sort.Slice(scores, func(i, j int) bool { return scores[i].Score > scores[j].Score })
	citations := []knowledge.Citation{}
	sentences := []string{}
	for i, item := range scores {
		if i == 3 {
			break
		}
		excerpt := snippet(item.Item.Text, 0)
		citations = append(citations, knowledge.Citation{DocumentID: item.Item.Document.ID, Title: item.Item.Document.Title, VersionID: item.Item.Document.CurrentVersionID, Excerpt: excerpt})
		sentences = append(sentences, item.Item.Document.Title+": "+excerpt)
	}
	answer := "I could not find an answer in this repository."
	if len(sentences) > 0 {
		answer = strings.Join(sentences, "\n\n")
	}
	c.JSON(http.StatusOK, repositoryQuestionResponse{Question: in.Question, Answer: answer, Citations: citations})
}

func graphFor(corpus []knowledgeDocument) ([]repositoryGraphNode, []repositoryGraphEdge) {
	nodes := make([]repositoryGraphNode, 0, len(corpus))
	edges := []repositoryGraphEdge{}
	for _, item := range corpus {
		nodes = append(nodes, repositoryGraphNode{ID: item.Document.ID, Type: "document", Title: item.Document.Title})
	}
	for _, from := range corpus {
		hay := strings.ToLower(from.Text)
		for _, to := range corpus {
			if from.Document.ID == to.Document.ID || strings.TrimSpace(to.Document.Title) == "" {
				continue
			}
			if strings.Contains(hay, strings.ToLower(to.Document.Title)) {
				edges = append(edges, repositoryGraphEdge{From: from.Document.ID, To: to.Document.ID, Type: "references"})
			}
		}
	}
	return nodes, edges
}

// repositoryGraph godoc
// @Summary Get repository dependency graph
// @Description Return document nodes and title-reference edges inferred from rendered repository text.
// @Tags knowledge-discovery
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Repository ID"
// @Success 200 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Router /v1/repositories/{id}/graph [get]
func (h handlers) repositoryGraph(c *gin.Context) {
	id := c.Param("id")
	if !h.needKnowledgeRole(c, id, repo.RoleViewer) {
		return
	}
	corpus, err := h.repositoryCorpus(c, id)
	if err != nil {
		internalErr(c, err)
		return
	}
	nodes, edges := graphFor(corpus)
	c.JSON(http.StatusOK, repositoryGraphResponse{Nodes: nodes, Edges: edges})
}

// repositoryBacklinks godoc
// @Summary List document backlinks
// @Description Return repository documents whose rendered text references the requested document title.
// @Tags knowledge-discovery
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Repository ID"
// @Param document_id query string true "Target document ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Router /v1/repositories/{id}/backlinks [get]
func (h handlers) repositoryBacklinks(c *gin.Context) {
	id := c.Param("id")
	if !h.needKnowledgeRole(c, id, repo.RoleViewer) {
		return
	}
	documentID := strings.TrimSpace(c.Query("document_id"))
	if documentID == "" {
		badRequest(c, "document_id is required")
		return
	}
	corpus, err := h.repositoryCorpus(c, id)
	if err != nil {
		internalErr(c, err)
		return
	}
	_, edges := graphFor(corpus)
	items := []repositoryGraphEdge{}
	for _, edge := range edges {
		if edge.To == documentID {
			items = append(items, edge)
		}
	}
	c.JSON(http.StatusOK, repositoryBacklinksResponse{DocumentID: documentID, Items: items})
}

// listTeams godoc
// @Summary List repository reviewer teams
// @Description List teams available to repository review rules and CODEOWNERS-style path rules.
// @Tags repository-teams
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Repository ID"
// @Success 200 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Router /v1/repositories/{id}/teams [get]
func (h handlers) listTeams(c *gin.Context) {
	id := c.Param("id")
	if !h.needKnowledgeRole(c, id, repo.RoleViewer) {
		return
	}
	items, err := h.deps.knowledge.ListTeams(c.Request.Context(), id)
	if err != nil {
		internalErr(c, err)
		return
	}
	c.JSON(http.StatusOK, listResponse[knowledge.Team]{Items: items})
}

// createTeam godoc
// @Summary Create a repository reviewer team
// @Description Create a named team and optional initial membership. Owner access is required.
// @Tags repository-teams
// @Security bearerAuth
// @Security cookieAuth
// @Accept json
// @Produce json
// @Param id path string true "Repository ID"
// @Param body body object true "name, description, and member_ids"
// @Success 201 {object} knowledge.Team
// @Failure 400 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Router /v1/repositories/{id}/teams [post]
func (h handlers) createTeam(c *gin.Context) {
	id := c.Param("id")
	if !h.needKnowledgeRole(c, id, repo.RoleOwner) {
		return
	}
	var in struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		MemberIDs   []string `json:"member_ids"`
	}
	if !requireJSON(c) {
		return
	}
	if err := c.ShouldBindJSON(&in); err != nil || strings.TrimSpace(in.Name) == "" {
		badRequest(c, "name is required")
		return
	}
	now := knowledge.Now()
	team := knowledge.Team{ID: knowledgeID("team"), RepositoryID: id, Name: strings.TrimSpace(in.Name), Description: strings.TrimSpace(in.Description), MemberIDs: uniqueStrings(in.MemberIDs), CreatedAt: now, UpdatedAt: now}
	team, err := h.deps.knowledge.PutTeam(c.Request.Context(), team)
	if err != nil {
		internalErr(c, err)
		return
	}
	h.addKnowledgeAudit(c, id, "team.created", "team", team.ID, nil)
	c.JSON(http.StatusCreated, team)
}
func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

// patchTeam godoc
// @Summary Update a repository reviewer team
// @Description Update team name, description, or complete membership. Owner access is required.
// @Tags repository-teams
// @Security bearerAuth
// @Security cookieAuth
// @Accept json
// @Produce json
// @Param id path string true "Repository ID"
// @Param team_id path string true "Team ID"
// @Param body body object true "Team fields to update"
// @Success 200 {object} knowledge.Team
// @Failure 400 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /v1/repositories/{id}/teams/{team_id} [patch]
func (h handlers) patchTeam(c *gin.Context) {
	id := c.Param("id")
	if !h.needKnowledgeRole(c, id, repo.RoleOwner) {
		return
	}
	team, err := h.deps.knowledge.GetTeam(c.Request.Context(), id, c.Param("team_id"))
	if errors.Is(err, knowledge.ErrNotFound) {
		notFound(c)
		return
	}
	if err != nil {
		internalErr(c, err)
		return
	}
	var in struct {
		Name        *string   `json:"name"`
		Description *string   `json:"description"`
		MemberIDs   *[]string `json:"member_ids"`
	}
	if !requireJSON(c) {
		return
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		badRequest(c, "invalid team")
		return
	}
	if in.Name != nil {
		if strings.TrimSpace(*in.Name) == "" {
			badRequest(c, "name cannot be empty")
			return
		}
		team.Name = strings.TrimSpace(*in.Name)
	}
	if in.Description != nil {
		team.Description = strings.TrimSpace(*in.Description)
	}
	if in.MemberIDs != nil {
		team.MemberIDs = uniqueStrings(*in.MemberIDs)
	}
	team.UpdatedAt = knowledge.Now()
	team, err = h.deps.knowledge.PutTeam(c.Request.Context(), team)
	if err != nil {
		internalErr(c, err)
		return
	}
	h.addKnowledgeAudit(c, id, "team.updated", "team", team.ID, nil)
	c.JSON(http.StatusOK, team)
}

// deleteTeam godoc
// @Summary Delete a repository reviewer team
// @Description Delete a team used by repository review settings. Owner access is required.
// @Tags repository-teams
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Repository ID"
// @Param team_id path string true "Team ID"
// @Success 204
// @Failure 403 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /v1/repositories/{id}/teams/{team_id} [delete]
func (h handlers) deleteTeam(c *gin.Context) {
	id := c.Param("id")
	if !h.needKnowledgeRole(c, id, repo.RoleOwner) {
		return
	}
	err := h.deps.knowledge.DeleteTeam(c.Request.Context(), id, c.Param("team_id"))
	if errors.Is(err, knowledge.ErrNotFound) {
		notFound(c)
		return
	}
	if err != nil {
		internalErr(c, err)
		return
	}
	h.addKnowledgeAudit(c, id, "team.deleted", "team", c.Param("team_id"), nil)
	c.Status(http.StatusNoContent)
}
func (h handlers) mutateTeamMember(c *gin.Context, add bool) {
	id := c.Param("id")
	if !h.needKnowledgeRole(c, id, repo.RoleOwner) {
		return
	}
	userID := c.Param("user_id")
	if _, err := h.deps.repository.ResolveUser(c.Request.Context(), userID); err != nil {
		notFound(c)
		return
	}
	team, err := h.deps.knowledge.GetTeam(c.Request.Context(), id, c.Param("team_id"))
	if errors.Is(err, knowledge.ErrNotFound) {
		notFound(c)
		return
	}
	if err != nil {
		internalErr(c, err)
		return
	}
	members := []string{}
	for _, value := range team.MemberIDs {
		if value != userID {
			members = append(members, value)
		}
	}
	if add {
		members = append(members, userID)
	}
	team.MemberIDs = uniqueStrings(members)
	team.UpdatedAt = knowledge.Now()
	team, err = h.deps.knowledge.PutTeam(c.Request.Context(), team)
	if err != nil {
		internalErr(c, err)
		return
	}
	action := "team.member_removed"
	if add {
		action = "team.member_added"
	}
	h.addKnowledgeAudit(c, id, action, "team", team.ID, map[string]any{"user_id": userID})
	c.JSON(http.StatusOK, team)
}

// addTeamMember godoc
// @Summary Add a repository team member
// @Description Add an existing user to a reviewer team. Owner access is required.
// @Tags repository-teams
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Repository ID"
// @Param team_id path string true "Team ID"
// @Param user_id path string true "User ID"
// @Success 200 {object} knowledge.Team
// @Failure 403 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /v1/repositories/{id}/teams/{team_id}/members/{user_id} [put]
func (h handlers) addTeamMember(c *gin.Context) { h.mutateTeamMember(c, true) }

// removeTeamMember godoc
// @Summary Remove a repository team member
// @Description Remove a user from a reviewer team. Owner access is required.
// @Tags repository-teams
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "Repository ID"
// @Param team_id path string true "Team ID"
// @Param user_id path string true "User ID"
// @Success 200 {object} knowledge.Team
// @Failure 403 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /v1/repositories/{id}/teams/{team_id}/members/{user_id} [delete]
func (h handlers) removeTeamMember(c *gin.Context) { h.mutateTeamMember(c, false) }

// listNotifications godoc
// @Summary List current-user notifications
// @Description Return review requests, repository transfers, and connected-source notifications for the authenticated user.
// @Tags notifications
// @Security bearerAuth
// @Security cookieAuth
// @Param unread query bool false "Only return unread notifications"
// @Success 200 {object} map[string]interface{}
// @Router /v1/notifications [get]
func (h handlers) listNotifications(c *gin.Context) {
	items, err := h.deps.knowledge.ListNotifications(c.Request.Context(), current(c).ID)
	if err != nil {
		internalErr(c, err)
		return
	}
	if c.Query("unread") == "true" {
		filtered := items[:0]
		for _, item := range items {
			if !item.Read {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	c.JSON(http.StatusOK, listResponse[knowledge.Notification]{Items: items})
}

// patchNotification godoc
// @Summary Mark a notification read or unread
// @Description Update the read state of a notification owned by the authenticated user.
// @Tags notifications
// @Security bearerAuth
// @Security cookieAuth
// @Accept json
// @Produce json
// @Param notification_id path string true "Notification ID"
// @Param body body object true "read boolean"
// @Success 200 {object} knowledge.Notification
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /v1/notifications/{notification_id} [patch]
func (h handlers) patchNotification(c *gin.Context) {
	var in struct {
		Read *bool `json:"read"`
	}
	if !requireJSON(c) {
		return
	}
	if err := c.ShouldBindJSON(&in); err != nil || in.Read == nil {
		badRequest(c, "read is required")
		return
	}
	item, err := h.deps.knowledge.SetNotificationRead(c.Request.Context(), current(c).ID, c.Param("notification_id"), *in.Read)
	if errors.Is(err, knowledge.ErrNotFound) {
		notFound(c)
		return
	}
	if err != nil {
		internalErr(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

// reviewInbox godoc
// @Summary List pull requests awaiting review
// @Description Return open pull requests the authenticated editor can review, enriched with repository reviewer and team requirements.
// @Tags pull-requests
// @Security bearerAuth
// @Security cookieAuth
// @Success 200 {object} map[string]interface{}
// @Router /v1/review-inbox [get]
func (h handlers) reviewInbox(c *gin.Context) {
	repositories, err := h.deps.repository.ListRepositories(c.Request.Context(), *current(c))
	if err != nil {
		internalErr(c, err)
		return
	}
	items := []reviewInboxItem{}
	for _, repository := range repositories {
		role, _ := h.deps.repository.RoleForRepository(c.Request.Context(), *current(c), repository.ID)
		if !atLeast(role, repo.RoleEditor) {
			continue
		}
		pullRequests, listErr := h.deps.repository.ListPullRequests(c.Request.Context(), repository.ID)
		if listErr != nil {
			continue
		}
		settings, _ := h.deps.knowledge.GetSettings(c.Request.Context(), repository.ID)
		for _, pullRequest := range pullRequests {
			if pullRequest.Status != repo.PullRequestStatusOpen || pullRequest.Author.ID == current(c).ID {
				continue
			}
			items = append(items, reviewInboxItem{Repository: repository, PullRequest: pullRequest, RequiredReviewers: settings.RequiredReviewers, RequiredTeams: settings.RequiredTeams})
		}
	}
	c.JSON(http.StatusOK, listResponse[reviewInboxItem]{Items: items})
}

// listTemplates godoc
// @Summary List documentation templates
// @Description Return built-in domain context, architecture, API, runbook, and onboarding templates.
// @Tags templates
// @Security bearerAuth
// @Security cookieAuth
// @Success 200 {object} map[string]interface{}
// @Router /v1/templates [get]
func (h handlers) listTemplates(c *gin.Context) {
	c.JSON(http.StatusOK, listResponse[knowledge.Template]{Items: knowledge.BuiltinTemplates})
}

// applyTemplate godoc
// @Summary Preview or apply a documentation template
// @Description Preview a built-in template or open a reviewable pull request against an existing target document.
// @Tags templates
// @Security bearerAuth
// @Security cookieAuth
// @Accept json
// @Produce json
// @Param id path string true "Repository ID"
// @Param template_id path string true "Template ID"
// @Param body body object false "target_document_id, optional title, and preview_only"
// @Success 200 {object} map[string]interface{} "Template preview"
// @Success 201 {object} map[string]interface{} "Template pull request opened"
// @Failure 400 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /v1/repositories/{id}/templates/{template_id}/apply [post]
func (h handlers) applyTemplate(c *gin.Context) {
	id := c.Param("id")
	if !h.needKnowledgeRole(c, id, repo.RoleEditor) {
		return
	}
	template, ok := knowledge.TemplateByID(c.Param("template_id"))
	if !ok {
		notFound(c)
		return
	}
	var in struct {
		TargetDocumentID string `json:"target_document_id"`
		Title            string `json:"title"`
		PreviewOnly      bool   `json:"preview_only"`
	}
	if c.Request.ContentLength != 0 {
		if !requireJSON(c) {
			return
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			badRequest(c, "invalid template application")
			return
		}
	}
	if in.Title != "" {
		template.Title = strings.TrimSpace(in.Title)
	}
	if in.TargetDocumentID == "" || in.PreviewOnly {
		c.JSON(http.StatusOK, templateApplicationResponse{Status: "preview", Template: template, Message: "Choose target_document_id to open a reviewable aidocs pull request."})
		return
	}
	document, err := h.deps.repository.GetDocument(c.Request.Context(), in.TargetDocumentID)
	if err != nil {
		notFound(c)
		return
	}
	pullRequest, err := h.deps.repository.CreatePullRequest(c.Request.Context(), id, document.ID, document.CurrentVersionID, "Apply "+template.Name+" template", "Template changes are proposed for review.", "Apply "+template.Name+" template", []byte(template.HTML), *current(c))
	if err != nil {
		internalErr(c, err)
		return
	}
	h.addKnowledgeAudit(c, id, "template.applied", "template", template.ID, map[string]any{"pull_request_number": pullRequest.Number})
	c.JSON(http.StatusCreated, templateApplicationResponse{Status: "pull_request_opened", Template: template, PullRequest: &pullRequest})
}

// publicPortal godoc
// @Summary Read a public documentation portal
// @Description Return public repository metadata and rendered document text for an enabled public portal. Authentication is not required.
// @Tags public-portals
// @Produce json
// @Param slug path string true "Portal slug"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /v1/public/portals/{slug} [get]
func (h handlers) publicPortal(c *gin.Context) {
	slug := knowledge.NormalizePortalSlug(c.Param("slug"))
	settings, err := h.deps.knowledge.FindPortal(c.Request.Context(), slug)
	if errors.Is(err, knowledge.ErrNotFound) || !settings.Portal.Enabled || settings.Portal.Visibility != "public" {
		notFound(c)
		return
	}
	if err != nil {
		internalErr(c, err)
		return
	}
	repository, err := h.deps.repository.GetRepository(c.Request.Context(), settings.RepositoryID)
	if err != nil {
		notFound(c)
		return
	}
	documents, err := h.deps.repository.ListRepositoryDocuments(c.Request.Context(), repository.CreatedBy, repository.ID)
	if err != nil {
		internalErr(c, err)
		return
	}
	items := []publicPortalDocument{}
	for _, document := range documents {
		body, bodyErr := h.deps.repository.GetVersionHTML(c.Request.Context(), document.CurrentVersionID)
		if bodyErr != nil {
			continue
		}
		items = append(items, publicPortalDocument{ID: document.ID, Title: document.Title, VersionID: document.CurrentVersionID, Text: plainHTML(body)})
	}
	c.Header("Cache-Control", "public, max-age=60")
	c.JSON(http.StatusOK, publicPortalResponse{
		Repository: publicPortalRepository{ID: repository.ID, Name: repository.Name, Slug: repository.Slug, Description: repository.Description},
		Portal:     settings.Portal, Documents: items,
	})
}
