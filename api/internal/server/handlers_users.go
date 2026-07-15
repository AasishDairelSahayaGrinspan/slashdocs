package server

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/razorpay/aidocs/api/internal/auth"
	"github.com/razorpay/aidocs/api/internal/repo"
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

func sameProfilePrincipal(a, b auth.Principal) bool {
	return a.Type == b.Type && a.ID == b.ID
}

func profileContributionTitle(prefix string, pullRequest repo.PullRequest) string {
	return fmt.Sprintf("%s #%d: %s", prefix, pullRequest.Number, pullRequest.Title)
}

// getUserProfile godoc
// @Summary Get a user's visible profile activity
// @Description Returns documents, pull requests, reviews, merges, and versions for the target user, filtered to resources the requester can read.
// @Tags users
// @Security bearerAuth
// @Security cookieAuth
// @Param id path string true "User ID"
// @Success 200 {object} repo.UserProfile
// @Router /v1/users/{id}/profile [get]
// getUserProfile returns only activity attached to documents and repositories
// the requesting principal can read. Repository list/read methods are checked
// again here instead of trusting a target user's ownership to imply visibility.
//
//nolint:gocyclo // The profile response intentionally aggregates independently optional contribution categories.
func (h handlers) getUserProfile(c *gin.Context) {
	ctx := c.Request.Context()
	target, err := h.deps.repository.ResolveUser(ctx, strings.TrimSpace(c.Param("id")))
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			notFound(c)
			return
		}
		internalErr(c, err)
		return
	}
	viewer := *current(c)
	profile := repo.UserProfile{
		User:          target,
		Documents:     []repo.UserProfileDocument{},
		PullRequests:  []repo.UserProfilePullRequest{},
		Contributions: []repo.UserContribution{},
	}

	visibleDocuments := map[string]repo.UserProfileDocument{}
	documents, err := h.deps.repository.ListDocuments(ctx, viewer)
	if err != nil {
		internalErr(c, err)
		return
	}
	// In an org deployment, profiles can discover org-wide documents. Public
	// deployments retain the workspace-entry behavior for anyone-shared docs.
	// RoleForDocument remains the authoritative filter and prevents the memory
	// test implementation's broad title search from leaking records.
	discovered, err := h.deps.repository.SearchDocumentsByTitle(ctx, viewer, "", h.deps.deployment == DeploymentOrg, 1000)
	if err != nil {
		internalErr(c, err)
		return
	}
	documents = append(documents, discovered...)
	for _, document := range documents {
		role, roleErr := h.deps.repository.RoleForDocument(ctx, viewer, document.ID)
		if roleErr == nil && atLeast(role, repo.RoleViewer) {
			visibleDocuments[document.ID] = repo.UserProfileDocument{Document: document}
		}
	}

	repositories, err := h.deps.repository.ListRepositories(ctx, viewer)
	if err != nil {
		internalErr(c, err)
		return
	}
	visiblePullRequests := []repo.UserProfilePullRequest{}
	for _, repository := range repositories {
		role, roleErr := h.deps.repository.RoleForRepository(ctx, viewer, repository.ID)
		if roleErr != nil || !atLeast(role, repo.RoleViewer) {
			continue
		}
		repositoryDocuments, listErr := h.deps.repository.ListRepositoryDocuments(ctx, viewer, repository.ID)
		if listErr != nil {
			if errors.Is(listErr, repo.ErrNotFound) {
				continue
			}
			internalErr(c, listErr)
			return
		}
		for _, document := range repositoryDocuments {
			repositoryCopy := repository
			visibleDocuments[document.ID] = repo.UserProfileDocument{Document: document, Repository: &repositoryCopy}
		}
		pullRequests, listErr := h.deps.repository.ListPullRequests(ctx, repository.ID)
		if listErr != nil {
			if errors.Is(listErr, repo.ErrNotFound) {
				continue
			}
			internalErr(c, listErr)
			return
		}
		for _, pullRequest := range pullRequests {
			visiblePullRequests = append(visiblePullRequests, repo.UserProfilePullRequest{PullRequest: pullRequest, Repository: repository})
		}
	}

	for _, item := range visibleDocuments {
		if sameProfilePrincipal(item.Document.Owner, target) {
			profile.Documents = append(profile.Documents, item)
		}
		versions, listErr := h.deps.repository.ListVersions(ctx, item.Document.ID)
		if listErr != nil {
			internalErr(c, listErr)
			return
		}
		for _, version := range versions {
			if !sameProfilePrincipal(version.CreatedBy, target) {
				continue
			}
			title := strings.TrimSpace(version.ChangeSummary)
			if title == "" {
				title = "Updated " + item.Document.Title
			}
			contribution := repo.UserContribution{
				ID:            "version:" + version.ID,
				Type:          "document_version",
				Title:         title,
				DocumentID:    item.Document.ID,
				DocumentTitle: item.Document.Title,
				State:         fmt.Sprintf("version_%d", version.Number),
			}
			if item.Repository != nil {
				contribution.RepositoryID = item.Repository.ID
				contribution.RepositoryName = item.Repository.Name
			}
			profile.Contributions = append(profile.Contributions, contribution)
		}
	}

	for _, item := range visiblePullRequests {
		pullRequest := item.PullRequest
		documentTitle := ""
		if document, ok := visibleDocuments[pullRequest.DocumentID]; ok {
			documentTitle = document.Document.Title
		}
		if sameProfilePrincipal(pullRequest.Author, target) {
			profile.PullRequests = append(profile.PullRequests, item)
			profile.Contributions = append(profile.Contributions, repo.UserContribution{
				ID:                "pull_request:" + pullRequest.ID,
				Type:              "pull_request",
				Title:             profileContributionTitle("Opened", pullRequest),
				DocumentID:        pullRequest.DocumentID,
				DocumentTitle:     documentTitle,
				RepositoryID:      item.Repository.ID,
				RepositoryName:    item.Repository.Name,
				PullRequestNumber: pullRequest.Number,
				State:             pullRequest.Status,
				OccurredAt:        pullRequest.CreatedAt,
			})
		}
		if pullRequest.MergedBy != nil && sameProfilePrincipal(*pullRequest.MergedBy, target) {
			profile.Contributions = append(profile.Contributions, repo.UserContribution{
				ID:                "merge:" + pullRequest.ID,
				Type:              "merge",
				Title:             profileContributionTitle("Merged", pullRequest),
				DocumentID:        pullRequest.DocumentID,
				DocumentTitle:     documentTitle,
				RepositoryID:      item.Repository.ID,
				RepositoryName:    item.Repository.Name,
				PullRequestNumber: pullRequest.Number,
				State:             repo.PullRequestStatusMerged,
				OccurredAt:        pullRequest.MergedAt,
			})
		}
		for _, review := range pullRequest.Reviews {
			if !sameProfilePrincipal(review.Reviewer, target) {
				continue
			}
			profile.Contributions = append(profile.Contributions, repo.UserContribution{
				ID:                "review:" + review.ID,
				Type:              "review",
				Title:             profileContributionTitle("Reviewed", pullRequest),
				DocumentID:        pullRequest.DocumentID,
				DocumentTitle:     documentTitle,
				RepositoryID:      item.Repository.ID,
				RepositoryName:    item.Repository.Name,
				PullRequestNumber: pullRequest.Number,
				State:             review.State,
				OccurredAt:        review.SubmittedAt,
			})
		}
	}

	sort.Slice(profile.Documents, func(i, j int) bool {
		if profile.Documents[i].Document.UpdatedAt == profile.Documents[j].Document.UpdatedAt {
			return strings.ToLower(profile.Documents[i].Document.Title) < strings.ToLower(profile.Documents[j].Document.Title)
		}
		return profile.Documents[i].Document.UpdatedAt > profile.Documents[j].Document.UpdatedAt
	})
	sort.Slice(profile.PullRequests, func(i, j int) bool {
		return profile.PullRequests[i].PullRequest.CreatedAt > profile.PullRequests[j].PullRequest.CreatedAt
	})
	sort.Slice(profile.Contributions, func(i, j int) bool {
		if profile.Contributions[i].OccurredAt == profile.Contributions[j].OccurredAt {
			return profile.Contributions[i].ID < profile.Contributions[j].ID
		}
		return profile.Contributions[i].OccurredAt > profile.Contributions[j].OccurredAt
	})

	contributedRepositories := map[string]bool{}
	for _, contribution := range profile.Contributions {
		if contribution.RepositoryID != "" {
			contributedRepositories[contribution.RepositoryID] = true
		}
	}
	profile.Stats = repo.UserProfileStats{
		DocumentsCreated:          len(profile.Documents),
		PullRequestsAuthored:      len(profile.PullRequests),
		Contributions:             len(profile.Contributions),
		RepositoriesContributedTo: len(contributedRepositories),
	}
	c.JSON(http.StatusOK, profile)
}
