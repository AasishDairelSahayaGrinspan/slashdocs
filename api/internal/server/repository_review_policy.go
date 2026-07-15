package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/razorpay/aidocs/api/internal/knowledge"
	"github.com/razorpay/aidocs/api/internal/repo"
)

type requiredReviewApprovalDetails struct {
	Reviewers []string `json:"reviewers"`
	Teams     []string `json:"teams"`
}

func (h handlers) requireActiveRepository(c *gin.Context, repositoryID string) bool {
	settings, err := h.deps.knowledge.GetSettings(c.Request.Context(), repositoryID)
	if err != nil {
		internalErr(c, err)
		return false
	}
	if settings.Archived {
		c.JSON(http.StatusConflict, errorResponse("repository_archived", "restore the repository before changing its documents", nil))
		return false
	}
	return true
}

//nolint:gocyclo // Review policy evaluation reports each distinct unmet requirement with its precise API response.
func (h handlers) requireConfiguredReviewApprovals(c *gin.Context, repositoryID string, number int64) bool {
	settings, err := h.deps.knowledge.GetSettings(c.Request.Context(), repositoryID)
	if err != nil {
		internalErr(c, err)
		return false
	}
	if len(settings.RequiredReviewers) == 0 && len(settings.RequiredTeams) == 0 && len(settings.PathRules) == 0 {
		return true
	}
	pullRequest, err := h.deps.repository.GetPullRequest(c.Request.Context(), repositoryID, number)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			notFound(c)
		} else {
			internalErr(c, err)
		}
		return false
	}

	requiredReviewers := append([]string{}, settings.RequiredReviewers...)
	requiredTeams := append([]string{}, settings.RequiredTeams...)
	paths := make([]string, 0, len(pullRequest.Changes)*2)
	for _, change := range pullRequest.Changes {
		paths = append(paths, change.Path, change.BasePath)
	}
	if runs, listErr := h.deps.knowledge.ListSyncRuns(c.Request.Context(), repositoryID); listErr == nil {
		for _, run := range runs {
			if run.PullRequestNumber == number {
				paths = append(paths, run.ChangedFiles...)
			}
		}
	}
	for _, rule := range settings.PathRules {
		if anyPathMatches(rule.Pattern, paths) {
			requiredReviewers = append(requiredReviewers, rule.RequiredReviewers...)
			requiredTeams = append(requiredTeams, rule.RequiredTeams...)
		}
	}

	approved := map[string]bool{}
	for _, review := range pullRequest.Reviews {
		if review.Patchset != pullRequest.Patchset || review.State != repo.ReviewStateApproved {
			continue
		}
		approved[strings.ToLower(review.Reviewer.ID)] = true
		approved[strings.ToLower(review.Reviewer.Email)] = true
	}
	missingReviewers := []string{}
	for _, reviewer := range uniqueStrings(requiredReviewers) {
		if !approved[strings.ToLower(reviewer)] {
			missingReviewers = append(missingReviewers, reviewer)
		}
	}

	teams, err := h.deps.knowledge.ListTeams(c.Request.Context(), repositoryID)
	if err != nil {
		internalErr(c, err)
		return false
	}
	missingTeams := []string{}
	for _, required := range uniqueStrings(requiredTeams) {
		team, found := configuredTeam(teams, required)
		if !found || !teamHasApproval(team, approved) {
			missingTeams = append(missingTeams, required)
		}
	}
	if len(missingReviewers) == 0 && len(missingTeams) == 0 {
		return true
	}
	c.JSON(http.StatusConflict, errorResponse(
		"required_review_approval",
		"the current patchset still needs approval from configured reviewers or teams",
		requiredReviewApprovalDetails{Reviewers: missingReviewers, Teams: missingTeams},
	))
	return false
}

func anyPathMatches(pattern string, paths []string) bool {
	for _, candidate := range paths {
		if candidate != "" && globMatch(pattern, candidate) {
			return true
		}
	}
	return false
}

func configuredTeam(teams []knowledge.Team, required string) (knowledge.Team, bool) {
	for _, team := range teams {
		if team.ID == required || strings.EqualFold(team.Name, required) {
			return team, true
		}
	}
	return knowledge.Team{}, false
}

func teamHasApproval(team knowledge.Team, approved map[string]bool) bool {
	for _, memberID := range team.MemberIDs {
		if approved[strings.ToLower(memberID)] {
			return true
		}
	}
	return false
}
