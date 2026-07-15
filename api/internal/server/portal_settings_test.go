package server

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/razorpay/aidocs/api/internal/repo"
)

func TestRepositorySettingsRejectDuplicateNormalizedPortalSlug(t *testing.T) {
	fixture := newKnowledgeFixture(t)
	ctx := context.Background()
	second, err := fixture.repository.CreateRepository(ctx, fixture.owner, repo.CreateRepositoryParams{
		Name: "Refunds knowledge", Slug: "refunds-knowledge", Scope: repo.RepositoryScopeOrganization,
		NamespaceID: "acme", NamespaceName: "Acme", RequiredApprovals: 0, Protected: true, GrantOrgViewer: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	firstPath := "/v1/repositories/" + fixture.repositoryID + "/settings"
	secondPath := "/v1/repositories/" + second.ID + "/settings"
	first := knowledgeRequest(t, fixture, http.MethodPatch, firstPath, "owner", map[string]any{
		"portal": map[string]any{"enabled": true, "slug": " Payments Docs!!! ", "visibility": "public"},
	})
	if first.Code != http.StatusOK || !strings.Contains(first.Body.String(), `"slug":"payments-docs"`) {
		t.Fatalf("first portal status=%d body=%s", first.Code, first.Body.String())
	}
	duplicate := knowledgeRequest(t, fixture, http.MethodPatch, secondPath, "owner", map[string]any{
		"portal": map[string]any{"enabled": true, "slug": "payments---docs", "visibility": "public"},
	})
	if duplicate.Code != http.StatusConflict || !strings.Contains(duplicate.Body.String(), `"code":"portal_slug_taken"`) {
		t.Fatalf("duplicate portal status=%d body=%s", duplicate.Code, duplicate.Body.String())
	}
	unchanged, err := fixture.knowledge.GetSettings(ctx, second.ID)
	if err != nil || unchanged.Portal.Enabled {
		t.Fatalf("duplicate request mutated settings=%+v err=%v", unchanged, err)
	}

	invalid := knowledgeRequest(t, fixture, http.MethodPatch, secondPath, "owner", map[string]any{
		"portal": map[string]any{"enabled": true, "slug": "---", "visibility": "public"},
	})
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("empty normalized slug status=%d body=%s", invalid.Code, invalid.Body.String())
	}
	disabled := knowledgeRequest(t, fixture, http.MethodPatch, firstPath, "owner", map[string]any{
		"portal": map[string]any{"enabled": false, "slug": "payments docs", "visibility": "public"},
	})
	if disabled.Code != http.StatusOK {
		t.Fatalf("disable first portal status=%d body=%s", disabled.Code, disabled.Body.String())
	}
	claimed := knowledgeRequest(t, fixture, http.MethodPatch, secondPath, "owner", map[string]any{
		"portal": map[string]any{"enabled": true, "slug": "PAYMENTS / DOCS", "visibility": "public"},
	})
	if claimed.Code != http.StatusOK || !strings.Contains(claimed.Body.String(), `"slug":"payments-docs"`) {
		t.Fatalf("released portal slug status=%d body=%s", claimed.Code, claimed.Body.String())
	}
	portal, err := fixture.knowledge.FindPortal(ctx, " Payments --- Docs ")
	if err != nil || portal.RepositoryID != second.ID {
		t.Fatalf("FindPortal after transfer = %+v, %v", portal, err)
	}
}
