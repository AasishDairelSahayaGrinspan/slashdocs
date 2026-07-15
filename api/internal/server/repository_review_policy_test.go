package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/razorpay/aidocs/api/internal/knowledge"
)

func TestRepositoryConfiguredReviewPolicyBlocksMergeUntilSatisfied(t *testing.T) {
	fixture := newKnowledgeFixture(t)
	ctx := context.Background()
	document, err := fixture.repository.GetDocument(ctx, fixture.documentID)
	if err != nil {
		t.Fatal(err)
	}
	settings := knowledge.DefaultSettings(fixture.repositoryID)
	settings.RequiredReviewers = []string{fixture.owner.Email}
	settings.RequiredTeams = []string{"team_owners"}
	if _, err := fixture.knowledge.PutSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.knowledge.PutTeam(ctx, knowledge.Team{
		ID:           "team_owners",
		RepositoryID: fixture.repositoryID,
		Name:         "Repository owners",
		MemberIDs:    []string{fixture.owner.ID},
		CreatedAt:    knowledge.Now(),
		UpdatedAt:    knowledge.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	create := knowledgeRequest(t, fixture, http.MethodPost, "/v1/repositories/"+fixture.repositoryID+"/pull-requests/change-set", "editor", map[string]any{
		"title": "Update retry ownership",
		"changes": []map[string]any{{
			"operation":       "update",
			"document_id":     fixture.documentID,
			"base_version_id": document.CurrentVersionID,
			"content_html":    "<html><body><h1>Retry architecture</h1><p>Owned by Payments Platform.</p></body></html>",
		}},
	})
	if create.Code != http.StatusCreated {
		t.Fatalf("create change set status=%d body=%s", create.Code, create.Body.String())
	}
	var pullRequest struct {
		Number int64 `json:"number"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &pullRequest); err != nil {
		t.Fatal(err)
	}
	mergePath := fmt.Sprintf("/v1/repositories/%s/pull-requests/%d/merge", fixture.repositoryID, pullRequest.Number)
	blocked := knowledgeRequest(t, fixture, http.MethodPost, mergePath, "editor", map[string]any{})
	if blocked.Code != http.StatusConflict || !strings.Contains(blocked.Body.String(), `"code":"required_review_approval"`) || !strings.Contains(blocked.Body.String(), fixture.owner.Email) || !strings.Contains(blocked.Body.String(), "team_owners") {
		t.Fatalf("merge without configured review status=%d body=%s", blocked.Code, blocked.Body.String())
	}
	reviewPath := fmt.Sprintf("/v1/repositories/%s/pull-requests/%d/reviews", fixture.repositoryID, pullRequest.Number)
	review := knowledgeRequest(t, fixture, http.MethodPost, reviewPath, "owner", map[string]any{"state": "approved"})
	if review.Code != http.StatusOK {
		t.Fatalf("configured review status=%d body=%s", review.Code, review.Body.String())
	}
	merged := knowledgeRequest(t, fixture, http.MethodPost, mergePath, "editor", map[string]any{})
	if merged.Code != http.StatusOK || !strings.Contains(merged.Body.String(), `"status":"merged"`) {
		t.Fatalf("merge after configured review status=%d body=%s", merged.Code, merged.Body.String())
	}
}

func TestArchivedRepositoryRejectsContentChanges(t *testing.T) {
	fixture := newKnowledgeFixture(t)
	settings := knowledge.DefaultSettings(fixture.repositoryID)
	settings.Archived = true
	if _, err := fixture.knowledge.PutSettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	response := knowledgeRequest(t, fixture, http.MethodPost, "/v1/repositories/"+fixture.repositoryID+"/pull-requests/change-set", "editor", map[string]any{
		"title": "Should not be created",
		"changes": []map[string]any{{
			"operation":    "create",
			"title":        "Archived content",
			"content_html": "<html><body>blocked</body></html>",
		}},
	})
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), `"code":"repository_archived"`) {
		t.Fatalf("archived change set status=%d body=%s", response.Code, response.Body.String())
	}
}
