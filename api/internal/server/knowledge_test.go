package server

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/razorpay/aidocs/api/internal/auth"
	"github.com/razorpay/aidocs/api/internal/knowledge"
	"github.com/razorpay/aidocs/api/internal/repo"
)

type knowledgeTestAuth struct{ principals map[string]auth.Principal }

func (a knowledgeTestAuth) Authenticate(r *http.Request) (*auth.Principal, error) {
	p, ok := a.principals[r.Header.Get("X-Knowledge-Principal")]
	if !ok {
		return nil, auth.ErrUnauthorized
	}
	return &p, nil
}

type knowledgeFixture struct {
	handler                  http.Handler
	repository               *repo.Memory
	knowledge                *knowledge.Memory
	repositoryID, documentID string
	owner, editor, viewer    auth.Principal
}

func newKnowledgeFixture(t *testing.T) knowledgeFixture {
	return newKnowledgeFixtureWithOptions(t)
}

func newKnowledgeFixtureWithOptions(t *testing.T, extraOptions ...Option) knowledgeFixture {
	t.Helper()
	ctx := context.Background()
	storage := repo.NewMemory()
	owner := auth.Principal{Type: auth.PrincipalUser, ID: "knowledge_owner", Email: "owner@example.com", Name: "Owner"}
	editor := auth.Principal{Type: auth.PrincipalUser, ID: "knowledge_editor", Email: "editor@example.com", Name: "Editor"}
	viewer := auth.Principal{Type: auth.PrincipalUser, ID: "knowledge_viewer", Email: "viewer@example.com", Name: "Viewer"}
	for _, p := range []auth.Principal{owner, editor, viewer} {
		if _, err := storage.UpsertGoogleUser(ctx, p.ID, p.Email, p.Name, "google-"+p.ID, ""); err != nil {
			t.Fatal(err)
		}
	}
	record, err := storage.CreateRepository(ctx, owner, repo.CreateRepositoryParams{Name: "Payments knowledge", Slug: "payments-knowledge", Scope: repo.RepositoryScopeOrganization, NamespaceID: "acme", NamespaceName: "Acme", RequiredApprovals: 0, Protected: true, GrantOrgViewer: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := storage.CreateGrant(ctx, "folder", record.RootFolderID, editor, repo.RoleEditor, owner); err != nil {
		t.Fatal(err)
	}
	document, _, err := storage.CreateRepositoryDocument(ctx, record.ID, owner, "Retry architecture", []byte(`<!doctype html><html><body><h1>Retry architecture</h1><p>Payments retry transient gateway failures with bounded exponential backoff.</p></body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	knowledgeStore := knowledge.NewMemory()
	//nolint:gosec // Test-only webhook signing material is intentionally non-secret.
	options := []Option{WithRepository(storage), WithKnowledgeStore(knowledgeStore), WithAuthenticator(knowledgeTestAuth{principals: map[string]auth.Principal{"owner": owner, "editor": editor, "viewer": viewer}})}
	options = append(options, extraOptions...)
	app := New(Config{Environment: envTest, Deployment: DeploymentOrg, OrgName: "Acme", GitHubWebhookSecret: "test-github-secret", SlackSigningSecret: "test-slack-signing-secret", DevRevWebhookSecret: "test-devrev-secret"}, options...)
	t.Cleanup(app.Close)
	return knowledgeFixture{handler: app.Handler(), repository: storage, knowledge: knowledgeStore, repositoryID: record.ID, documentID: document.ID, owner: owner, editor: editor, viewer: viewer}
}

func knowledgeRequest(t *testing.T, fixture knowledgeFixture, method, path, principal string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	if principal != "" {
		req.Header.Set("X-Knowledge-Principal", principal)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rr := httptest.NewRecorder()
	fixture.handler.ServeHTTP(rr, req)
	return rr
}

func responseID(t *testing.T, rr *httptest.ResponseRecorder) string {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	value, _ := payload["id"].(string)
	return value
}

func signedSlackRequest(t *testing.T, fixture knowledgeFixture, body []byte, at time.Time) *httptest.ResponseRecorder {
	t.Helper()
	timestamp := fmt.Sprintf("%d", at.Unix())
	mac := hmac.New(sha256.New, []byte("test-slack-signing-secret"))
	_, _ = mac.Write([]byte("v0:" + timestamp + ":" + string(body)))
	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/slack", bytes.NewReader(body))
	req.Header.Set("X-Slack-Request-Timestamp", timestamp)
	req.Header.Set("X-Slack-Signature", "v0="+hex.EncodeToString(mac.Sum(nil)))
	rr := httptest.NewRecorder()
	fixture.handler.ServeHTTP(rr, req)
	return rr
}

func signedDevRevRequest(t *testing.T, fixture knowledgeFixture, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	mac := hmac.New(sha256.New, []byte("test-devrev-secret"))
	_, _ = mac.Write(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/devrev", bytes.NewReader(body))
	req.Header.Set("X-DevRev-Signature", hex.EncodeToString(mac.Sum(nil)))
	rr := httptest.NewRecorder()
	fixture.handler.ServeHTTP(rr, req)
	return rr
}

func waitForKnowledgeRun(t *testing.T, fixture knowledgeFixture, deliveryID string) knowledge.SyncRun {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		runs, err := fixture.knowledge.ListSyncRuns(context.Background(), fixture.repositoryID)
		if err != nil {
			t.Fatal(err)
		}
		for _, run := range runs {
			if run.DeliveryID == deliveryID && run.Status == "completed" {
				return run
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("delivery %s did not complete", deliveryID)
	return knowledge.SyncRun{}
}

func TestKnowledgeSourceAuthorizationAndCRUD(t *testing.T) {
	fixture := newKnowledgeFixture(t)
	path := "/v1/repositories/" + fixture.repositoryID + "/sources"
	if rr := knowledgeRequest(t, fixture, http.MethodPost, path, "viewer", map[string]any{"provider": "github", "external_repository": "acme/payments"}); rr.Code != http.StatusForbidden {
		t.Fatalf("viewer create status=%d body=%s", rr.Code, rr.Body.String())
	}
	create := knowledgeRequest(t, fixture, http.MethodPost, path, "owner", map[string]any{"provider": "github", "external_repository": "acme/payments", "branch": "main", "include_paths": []string{"services/**"}, "target_document_id": fixture.documentID})
	if create.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", create.Code, create.Body.String())
	}
	id := responseID(t, create)
	patch := knowledgeRequest(t, fixture, http.MethodPatch, path+"/"+id, "owner", map[string]any{"status": "paused", "exclude_paths": []string{"vendor/**"}})
	if patch.Code != http.StatusOK || !strings.Contains(patch.Body.String(), `"status":"paused"`) {
		t.Fatalf("patch status=%d body=%s", patch.Code, patch.Body.String())
	}
	list := knowledgeRequest(t, fixture, http.MethodGet, path, "viewer", nil)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), id) {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}
	if rr := knowledgeRequest(t, fixture, http.MethodDelete, path+"/"+id, "owner", nil); rr.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestKnowledgeSourceProvidersAndEventIdentityValidation(t *testing.T) {
	fixture := newKnowledgeFixture(t)
	path := "/v1/repositories/" + fixture.repositoryID + "/sources"
	for _, provider := range []string{"gitlab", "bitbucket"} {
		rr := knowledgeRequest(t, fixture, http.MethodPost, path, "owner", map[string]any{"provider": provider, "external_repository": "acme/legacy"})
		if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "unsupported provider") {
			t.Fatalf("provider %s status=%d body=%s", provider, rr.Code, rr.Body.String())
		}
	}
	if rr := knowledgeRequest(t, fixture, http.MethodPost, path, "owner", map[string]any{"provider": "slack", "external_repository": "only-a-channel"}); rr.Code != http.StatusBadRequest {
		t.Fatalf("malformed Slack identity status=%d body=%s", rr.Code, rr.Body.String())
	}
	slack := knowledgeRequest(t, fixture, http.MethodPost, path, "owner", map[string]any{"provider": "slack", "external_repository": "T123/C456/1712345678.000100", "target_document_id": fixture.documentID})
	if slack.Code != http.StatusCreated || !strings.Contains(slack.Body.String(), `"trigger_mode":"event"`) {
		t.Fatalf("Slack source status=%d body=%s", slack.Code, slack.Body.String())
	}
	devrev := knowledgeRequest(t, fixture, http.MethodPost, path, "owner", map[string]any{"provider": "devrev", "external_repository": "don:integration:dvrv-us-1:devo/1:webhook/2", "target_document_id": fixture.documentID})
	if devrev.Code != http.StatusCreated || !strings.Contains(devrev.Body.String(), `"trigger_mode":"event"`) {
		t.Fatalf("DevRev source status=%d body=%s", devrev.Code, devrev.Body.String())
	}
	if rr := knowledgeRequest(t, fixture, http.MethodPost, path, "owner", map[string]any{"provider": "devrev", "external_repository": "TKT-1", "trigger_mode": "scheduled"}); rr.Code != http.StatusCreated {
		t.Fatalf("scheduled DevRev trigger status=%d body=%s", rr.Code, rr.Body.String())
	}
	if rr := knowledgeRequest(t, fixture, http.MethodPost, path, "owner", map[string]any{"provider": "devrev", "external_repository": "TKT-1", "trigger_mode": "default_branch"}); rr.Code != http.StatusBadRequest {
		t.Fatalf("invalid DevRev trigger status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestKnowledgeSourceSimulationIsNonMutatingAndCheckUsesBot(t *testing.T) {
	fixture := newKnowledgeFixture(t)
	now := knowledge.Now()
	source := knowledge.Source{ID: "src_manual", RepositoryID: fixture.repositoryID, Provider: knowledge.ProviderManual, Branch: "main", IncludePaths: []string{}, ExcludePaths: []string{}, TargetDocumentID: fixture.documentID, TriggerMode: "manual", Direction: "into_aidocs", Status: "active", Freshness: knowledge.Freshness{State: "never_synced"}, CreatedBy: fixture.owner, CreatedAt: now, UpdatedAt: now}
	if _, err := fixture.knowledge.PutSource(context.Background(), source); err != nil {
		t.Fatal(err)
	}
	documentBefore, err := fixture.repository.GetDocument(context.Background(), fixture.documentID)
	if err != nil {
		t.Fatal(err)
	}
	path := "/v1/repositories/" + fixture.repositoryID + "/sources/" + source.ID
	simulate := knowledgeRequest(t, fixture, http.MethodPost, path+"/simulate", "editor", map[string]any{"after_sha": "simulation", "changed_files": []string{"domain/model.go"}})
	if simulate.Code != http.StatusOK || !strings.Contains(simulate.Body.String(), `"outcome":"preview"`) {
		t.Fatalf("simulate status=%d body=%s", simulate.Code, simulate.Body.String())
	}
	if runs, _ := fixture.knowledge.ListSyncRuns(context.Background(), fixture.repositoryID); len(runs) != 0 {
		t.Fatalf("simulate persisted %d runs", len(runs))
	}
	if prs, _ := fixture.repository.ListPullRequests(context.Background(), fixture.repositoryID); len(prs) != 0 {
		t.Fatalf("simulate created %d pull requests", len(prs))
	}
	documentAfter, err := fixture.repository.GetDocument(context.Background(), fixture.documentID)
	if err != nil {
		t.Fatal(err)
	}
	if documentAfter.CurrentVersionID != documentBefore.CurrentVersionID {
		t.Fatalf("simulate advanced document from %s to %s", documentBefore.CurrentVersionID, documentAfter.CurrentVersionID)
	}
	check := knowledgeRequest(t, fixture, http.MethodPost, path+"/check", "editor", map[string]any{"after_sha": "checked", "changed_files": []string{"domain/model.go"}})
	if check.Code != http.StatusOK || !strings.Contains(check.Body.String(), `"outcome":"pull_request_opened"`) {
		t.Fatalf("check status=%d body=%s", check.Code, check.Body.String())
	}
	prs, err := fixture.repository.ListPullRequests(context.Background(), fixture.repositoryID)
	if err != nil || len(prs) != 1 {
		t.Fatalf("check PRs=%d err=%v", len(prs), err)
	}
	if prs[0].Author.ID != connectedKnowledgeBot().ID || prs[0].Author.Type != auth.PrincipalServiceAccount {
		t.Fatalf("check author=%+v", prs[0].Author)
	}
}

func TestAppendSourceSummaryReplacesPriorMarker(t *testing.T) {
	source := knowledge.Source{ID: "src_replace", ExternalRepository: "acme/payments"}
	first := appendSourceSummary([]byte("<html><body><p>Human content</p></body></html>"), source, "111", []string{"old.go"})
	second := appendSourceSummary(first, source, "222", []string{"new.go"})
	body := string(second)
	if strings.Count(body, "aidocs-source:start:src_replace") != 1 || strings.Count(body, "data-aidocs-source=\"src_replace\"") != 1 {
		t.Fatalf("source section duplicated: %s", body)
	}
	if strings.Contains(body, "old.go") || !strings.Contains(body, "new.go") || !strings.Contains(body, "Human content") {
		t.Fatalf("source section was not replaced safely: %s", body)
	}
	legacy := []byte(`<html><body><section data-aidocs-source="src_replace"><p>legacy.go</p></section></body></html>`)
	migrated := string(appendSourceSummary(legacy, source, "333", []string{"migrated.go"}))
	if strings.Contains(migrated, "legacy.go") || strings.Count(migrated, "data-aidocs-source=\"src_replace\"") != 1 {
		t.Fatalf("legacy source section was not replaced: %s", migrated)
	}
}

func TestConnectedKnowledgeSyncCreatesNewPRAfterMerge(t *testing.T) {
	fixture := newKnowledgeFixture(t)
	ctx := context.Background()
	source := knowledge.Source{ID: "src_history", RepositoryID: fixture.repositoryID, Provider: knowledge.ProviderGitHub, ExternalRepository: "acme/payments", TargetDocumentID: fixture.documentID}
	document, err := fixture.repository.GetDocument(ctx, fixture.documentID)
	if err != nil {
		t.Fatal(err)
	}
	base, err := fixture.repository.GetVersion(ctx, document.CurrentVersionID)
	if err != nil {
		t.Fatal(err)
	}
	body, err := fixture.repository.GetVersionHTML(ctx, base.ID)
	if err != nil {
		t.Fatal(err)
	}
	deps := dependencies{repository: fixture.repository, knowledge: fixture.knowledge}
	first, updated, err := upsertConnectedKnowledgePullRequest(ctx, deps, source, document, base, "first", appendSourceSummary(body, source, "111", []string{"first.go"}))
	if err != nil || updated {
		t.Fatalf("first PR updated=%t err=%v", updated, err)
	}
	if _, err := fixture.repository.MergePullRequest(ctx, fixture.repositoryID, first.Number, fixture.owner); err != nil {
		t.Fatal(err)
	}
	document, err = fixture.repository.GetDocument(ctx, fixture.documentID)
	if err != nil {
		t.Fatal(err)
	}
	base, err = fixture.repository.GetVersion(ctx, document.CurrentVersionID)
	if err != nil {
		t.Fatal(err)
	}
	body, err = fixture.repository.GetVersionHTML(ctx, base.ID)
	if err != nil {
		t.Fatal(err)
	}
	second, updated, err := upsertConnectedKnowledgePullRequest(ctx, deps, source, document, base, "second", appendSourceSummary(body, source, "222", []string{"second.go"}))
	if err != nil || updated {
		t.Fatalf("second PR updated=%t err=%v", updated, err)
	}
	if second.Number == first.Number {
		t.Fatalf("merged PR %d was reused", first.Number)
	}
	items, err := fixture.repository.ListPullRequests(ctx, fixture.repositoryID)
	if err != nil || len(items) != 2 {
		t.Fatalf("pull requests=%d err=%v", len(items), err)
	}
}

func TestKnowledgeWebhookDedupeAndProcessorOpensPullRequest(t *testing.T) {
	fixture := newKnowledgeFixture(t)
	now := knowledge.Now()
	source := knowledge.Source{ID: "src_webhook", RepositoryID: fixture.repositoryID, Provider: knowledge.ProviderGitHub, ExternalRepository: "acme/payments", Branch: "main", IncludePaths: []string{"services/**"}, ExcludePaths: []string{}, TargetDocumentID: fixture.documentID, TriggerMode: "default_branch", Direction: "into_aidocs", Status: "active", Freshness: knowledge.Freshness{State: "never_synced"}, CreatedBy: fixture.owner, CreatedAt: now, UpdatedAt: now}
	if _, err := fixture.knowledge.PutSource(context.Background(), source); err != nil {
		t.Fatal(err)
	}
	settings := knowledge.DefaultSettings(fixture.repositoryID)
	settings.RequiredReviewers = []string{fixture.editor.ID}
	if _, err := fixture.knowledge.PutSettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	send := func(delivery, before, after, file string) *httptest.ResponseRecorder {
		payload := []byte(fmt.Sprintf(`{"ref":"refs/heads/main","before":%q,"after":%q,"repository":{"full_name":"acme/payments"},"commits":[{"modified":[%q]}]}`, before, after, file))
		mac := hmac.New(sha256.New, []byte("test-github-secret"))
		_, _ = mac.Write(payload)
		signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/github", bytes.NewReader(payload))
		req.Header.Set("X-Hub-Signature-256", signature)
		req.Header.Set("X-GitHub-Delivery", delivery)
		req.Header.Set("X-GitHub-Event", "push")
		rr := httptest.NewRecorder()
		fixture.handler.ServeHTTP(rr, req)
		return rr
	}
	if rr := send("delivery-1", "111", "222", "services/retry.go"); rr.Code != http.StatusAccepted {
		t.Fatalf("webhook status=%d body=%s", rr.Code, rr.Body.String())
	}
	if rr := send("delivery-1", "111", "222", "services/retry.go"); rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"duplicate":true`) {
		t.Fatalf("duplicate status=%d body=%s", rr.Code, rr.Body.String())
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		runs, _ := fixture.knowledge.ListSyncRuns(context.Background(), fixture.repositoryID)
		if len(runs) == 1 && runs[0].Status == "completed" {
			if runs[0].Outcome != "pull_request_opened" {
				t.Fatalf("outcome=%s message=%s", runs[0].Outcome, runs[0].Message)
			}
			prs, err := fixture.repository.ListPullRequests(context.Background(), fixture.repositoryID)
			if err != nil || len(prs) != 1 {
				t.Fatalf("pull requests=%d err=%v", len(prs), err)
			}
			if prs[0].Author.ID != "sa_connected_knowledge" || prs[0].Author.Type != auth.PrincipalServiceAccount || prs[0].Patchset != 1 {
				t.Fatalf("initial PR author=%+v patchset=%d", prs[0].Author, prs[0].Patchset)
			}
			if rr := knowledgeRequest(t, fixture, http.MethodGet, "/v1/review-inbox", "editor", nil); rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"pull_request"`) {
				t.Fatalf("review inbox status=%d body=%s", rr.Code, rr.Body.String())
			}
			if rr := knowledgeRequest(t, fixture, http.MethodGet, "/v1/notifications?unread=true", "editor", nil); rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "Documentation review requested") || !strings.Contains(rr.Body.String(), "/app/repos/"+fixture.repositoryID+"/pulls/1") {
				t.Fatalf("notifications status=%d body=%s", rr.Code, rr.Body.String())
			}
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if time.Now().After(deadline) {
		t.Fatal("queued webhook run was not completed")
	}
	if rr := send("delivery-2", "222", "333", "services/router.go"); rr.Code != http.StatusAccepted {
		t.Fatalf("second webhook status=%d body=%s", rr.Code, rr.Body.String())
	}
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		runs, _ := fixture.knowledge.ListSyncRuns(context.Background(), fixture.repositoryID)
		completed := false
		for _, run := range runs {
			if run.AfterSHA == "333" && run.Status == "completed" {
				if run.Outcome != "pull_request_updated" {
					t.Fatalf("second outcome=%s message=%s", run.Outcome, run.Message)
				}
				completed = true
			}
		}
		if completed {
			prs, err := fixture.repository.ListPullRequests(context.Background(), fixture.repositoryID)
			if err != nil || len(prs) != 1 {
				t.Fatalf("after update pull requests=%d err=%v", len(prs), err)
			}
			if prs[0].Patchset != 2 {
				t.Fatalf("patchset=%d want 2", prs[0].Patchset)
			}
			body, err := fixture.repository.GetVersionHTML(context.Background(), prs[0].HeadVersionID)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(string(body), "aidocs-source:start:src_webhook") != 1 || !strings.Contains(string(body), "services/router.go") || strings.Contains(string(body), "services/retry.go") {
				t.Fatalf("updated generated section=%s", body)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("second webhook did not update the open pull request")
}

func TestKnowledgeSearchQuestionArchivePortalTeamsAndTemplates(t *testing.T) {
	fixture := newKnowledgeFixture(t)
	base := "/v1/repositories/" + fixture.repositoryID
	if rr := knowledgeRequest(t, fixture, http.MethodGet, base+"/search?q=exponential", "viewer", nil); rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), fixture.documentID) {
		t.Fatalf("search status=%d body=%s", rr.Code, rr.Body.String())
	}
	if rr := knowledgeRequest(t, fixture, http.MethodPost, base+"/questions", "viewer", map[string]any{"question": "How do payment retries back off?"}); rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"citations":[{"document_id":"`+fixture.documentID) {
		t.Fatalf("question status=%d body=%s", rr.Code, rr.Body.String())
	}
	if rr := knowledgeRequest(t, fixture, http.MethodPost, base+"/archive", "owner", nil); rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"archived":true`) {
		t.Fatalf("archive status=%d body=%s", rr.Code, rr.Body.String())
	}
	portal := knowledgeRequest(t, fixture, http.MethodPatch, base+"/settings", "owner", map[string]any{"portal": map[string]any{"enabled": true, "slug": "payments-docs", "visibility": "public"}})
	if portal.Code != http.StatusOK {
		t.Fatalf("portal settings status=%d body=%s", portal.Code, portal.Body.String())
	}
	if rr := knowledgeRequest(t, fixture, http.MethodGet, "/v1/public/portals/payments-docs", "", nil); rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "Retry architecture") {
		t.Fatalf("portal status=%d body=%s", rr.Code, rr.Body.String())
	}
	team := knowledgeRequest(t, fixture, http.MethodPost, base+"/teams", "owner", map[string]any{"name": "Payments reviewers", "member_ids": []string{fixture.editor.ID}})
	if team.Code != http.StatusCreated {
		t.Fatalf("team status=%d body=%s", team.Code, team.Body.String())
	}
	if rr := knowledgeRequest(t, fixture, http.MethodGet, "/v1/templates", "viewer", nil); rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "domain-context") || !strings.Contains(rr.Body.String(), "runbook") {
		t.Fatalf("templates status=%d body=%s", rr.Code, rr.Body.String())
	}
	if rr := knowledgeRequest(t, fixture, http.MethodPost, base+"/templates/runbook/apply", "editor", map[string]any{"preview_only": true}); rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"status":"preview"`) {
		t.Fatalf("template preview status=%d body=%s", rr.Code, rr.Body.String())
	}
	if rr := knowledgeRequest(t, fixture, http.MethodPost, base+"/restore", "owner", nil); rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"archived":false`) {
		t.Fatalf("restore status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestKnowledgeWebhookRejectsInvalidSignature(t *testing.T) {
	fixture := newKnowledgeFixture(t)
	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/github", strings.NewReader(`{}`))
	req.Header.Set("X-Hub-Signature-256", "sha256=bad")
	req.Header.Set("X-GitHub-Delivery", "bad-delivery")
	req.Header.Set("X-GitHub-Event", "push")
	rr := httptest.NewRecorder()
	fixture.handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestKnowledgeWebhookMalformedPayloadDoesNotConsumeDelivery(t *testing.T) {
	fixture := newKnowledgeFixture(t)
	send := func(body []byte) *httptest.ResponseRecorder {
		mac := hmac.New(sha256.New, []byte("test-github-secret"))
		_, _ = mac.Write(body)
		req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/github", bytes.NewReader(body))
		req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
		req.Header.Set("X-GitHub-Delivery", "reusable-delivery")
		req.Header.Set("X-GitHub-Event", "push")
		rr := httptest.NewRecorder()
		fixture.handler.ServeHTTP(rr, req)
		return rr
	}
	if rr := send([]byte(`{`)); rr.Code != http.StatusBadRequest {
		t.Fatalf("malformed status=%d body=%s", rr.Code, rr.Body.String())
	}
	valid := []byte(`{"ref":"refs/heads/main","before":"111","after":"222","repository":{"full_name":"acme/unconfigured"},"commits":[]}`)
	if rr := send(valid); rr.Code != http.StatusAccepted || !strings.Contains(rr.Body.String(), `"duplicate":false`) {
		t.Fatalf("reused delivery status=%d body=%s", rr.Code, rr.Body.String())
	}
	if rr := send(valid); rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"duplicate":true`) {
		t.Fatalf("duplicate status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestSlackWebhookVerifiesChallengeAndRejectsReplay(t *testing.T) {
	fixture := newKnowledgeFixture(t)
	body, err := json.Marshal(map[string]any{"type": "url_verification", "challenge": "slack-challenge"})
	if err != nil {
		t.Fatal(err)
	}
	if rr := signedSlackRequest(t, fixture, body, time.Now()); rr.Code != http.StatusOK || rr.Body.String() != "slack-challenge" {
		t.Fatalf("challenge status=%d body=%s", rr.Code, rr.Body.String())
	}
	if rr := signedSlackRequest(t, fixture, body, time.Now().Add(-6*time.Minute)); rr.Code != http.StatusUnauthorized || !strings.Contains(rr.Body.String(), "stale_request") {
		t.Fatalf("stale status=%d body=%s", rr.Code, rr.Body.String())
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/slack", bytes.NewReader(body))
	req.Header.Set("X-Slack-Request-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
	req.Header.Set("X-Slack-Signature", "v0=invalid")
	rr := httptest.NewRecorder()
	fixture.handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized || !strings.Contains(rr.Body.String(), "invalid_signature") {
		t.Fatalf("invalid signature status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestSlackThreadWebhookQueuesDeduplicatesAndWritesEscapedMessage(t *testing.T) {
	fixture := newKnowledgeFixture(t)
	now := knowledge.Now()
	source := knowledge.Source{ID: "src_slack", RepositoryID: fixture.repositoryID, Provider: knowledge.ProviderSlack, ExternalRepository: "T123/C456/1712345678.000100", IncludePaths: []string{}, ExcludePaths: []string{}, TargetDocumentID: fixture.documentID, TriggerMode: "event", Direction: "into_aidocs", Status: "active", Freshness: knowledge.Freshness{State: "never_synced"}, CreatedBy: fixture.owner, CreatedAt: now, UpdatedAt: now}
	if _, err := fixture.knowledge.PutSource(context.Background(), source); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{
		"type": "event_callback", "event_id": "Ev-slack-1", "team_id": "T123",
		"event": map[string]any{"type": "message", "channel": "C456", "ts": "1712345680.000200", "thread_ts": "1712345678.000100", "user": "U789", "text": "Refund policy <script>alert('x')</script>\nUse the documented retry window."},
	})
	if err != nil {
		t.Fatal(err)
	}
	if rr := signedSlackRequest(t, fixture, payload, time.Now()); rr.Code != http.StatusAccepted || !strings.Contains(rr.Body.String(), `"queued":1`) {
		t.Fatalf("event status=%d body=%s", rr.Code, rr.Body.String())
	}
	if rr := signedSlackRequest(t, fixture, payload, time.Now()); rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"duplicate":true`) {
		t.Fatalf("duplicate status=%d body=%s", rr.Code, rr.Body.String())
	}
	run := waitForKnowledgeRun(t, fixture, "Ev-slack-1")
	if run.Outcome != "pull_request_opened" || len(run.ContextItems) != 1 || !strings.Contains(run.ContextItems[0].Body, "Refund policy") {
		t.Fatalf("run=%+v", run)
	}
	prs, err := fixture.repository.ListPullRequests(context.Background(), fixture.repositoryID)
	if err != nil || len(prs) != 1 {
		t.Fatalf("pull requests=%d err=%v", len(prs), err)
	}
	html, err := fixture.repository.GetVersionHTML(context.Background(), prs[0].HeadVersionID)
	if err != nil {
		t.Fatal(err)
	}
	content := string(html)
	if !strings.Contains(content, "Refund policy &lt;script&gt;alert(&#39;x&#39;)&lt;/script&gt;") || !strings.Contains(content, "Use the documented retry window.") || strings.Contains(content, "<script>") {
		t.Fatalf("Slack context was not safely rendered: %s", content)
	}
	if !strings.Contains(content, "U789") || !strings.Contains(content, "1712345678.000100") {
		t.Fatalf("Slack thread metadata missing: %s", content)
	}
}

func TestDevRevWebhookChallengeTicketQueueAndReplayProtection(t *testing.T) {
	fixture := newKnowledgeFixture(t)
	webhookID := "don:integration:dvrv-us-1:devo/1:webhook/2"
	challenge, err := json.Marshal(map[string]any{
		"id": "devrev-verify-1", "webhook_id": webhookID, "timestamp": time.Now().UTC().Format(time.RFC3339Nano), "type": "verify", "verify": map[string]any{"challenge": "devrev-challenge"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if rr := signedDevRevRequest(t, fixture, challenge); rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"challenge":"devrev-challenge"`) {
		t.Fatalf("challenge status=%d body=%s", rr.Code, rr.Body.String())
	}
	invalidRequest := httptest.NewRequest(http.MethodPost, "/v1/webhooks/devrev", bytes.NewReader(challenge))
	invalidRequest.Header.Set("X-DevRev-Signature", "invalid")
	invalidResponse := httptest.NewRecorder()
	fixture.handler.ServeHTTP(invalidResponse, invalidRequest)
	if invalidResponse.Code != http.StatusUnauthorized || !strings.Contains(invalidResponse.Body.String(), "invalid_signature") {
		t.Fatalf("invalid signature status=%d body=%s", invalidResponse.Code, invalidResponse.Body.String())
	}
	stale, err := json.Marshal(map[string]any{
		"id": "devrev-verify-stale", "webhook_id": webhookID, "timestamp": time.Now().Add(-6 * time.Minute).UTC().Format(time.RFC3339Nano), "type": "verify", "verify": map[string]any{"challenge": "stale"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if rr := signedDevRevRequest(t, fixture, stale); rr.Code != http.StatusUnauthorized || !strings.Contains(rr.Body.String(), "stale_request") {
		t.Fatalf("stale status=%d body=%s", rr.Code, rr.Body.String())
	}
	now := knowledge.Now()
	source := knowledge.Source{ID: "src_devrev", RepositoryID: fixture.repositoryID, Provider: knowledge.ProviderDevRev, ExternalRepository: webhookID, IncludePaths: []string{}, ExcludePaths: []string{}, TargetDocumentID: fixture.documentID, TriggerMode: "event", Direction: "into_aidocs", Status: "active", Freshness: knowledge.Freshness{State: "never_synced"}, CreatedBy: fixture.owner, CreatedAt: now, UpdatedAt: now}
	if _, err := fixture.knowledge.PutSource(context.Background(), source); err != nil {
		t.Fatal(err)
	}
	ticket, err := json.Marshal(map[string]any{
		"id": "devrev-event-1", "webhook_id": webhookID, "timestamp": time.Now().UTC().Format(time.RFC3339Nano), "type": "work_updated",
		"work_updated": map[string]any{"work": map[string]any{"type": "ticket", "id": "don:core:dvrv-us-1:devo/1:ticket/77", "display_id": "TKT-77", "title": "Refund <rules>", "body": "Never render <script>ticket()</script>; keep this text.", "state": "open"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if rr := signedDevRevRequest(t, fixture, ticket); rr.Code != http.StatusAccepted || !strings.Contains(rr.Body.String(), `"queued":1`) {
		t.Fatalf("ticket status=%d body=%s", rr.Code, rr.Body.String())
	}
	if rr := signedDevRevRequest(t, fixture, ticket); rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"duplicate":true`) {
		t.Fatalf("duplicate status=%d body=%s", rr.Code, rr.Body.String())
	}
	run := waitForKnowledgeRun(t, fixture, "devrev-event-1")
	if run.Outcome != "pull_request_opened" || len(run.ContextItems) != 1 || run.ContextItems[0].Metadata["state"] != "open" {
		t.Fatalf("run=%+v", run)
	}
	prs, err := fixture.repository.ListPullRequests(context.Background(), fixture.repositoryID)
	if err != nil || len(prs) != 1 {
		t.Fatalf("pull requests=%d err=%v", len(prs), err)
	}
	html, err := fixture.repository.GetVersionHTML(context.Background(), prs[0].HeadVersionID)
	if err != nil {
		t.Fatal(err)
	}
	content := string(html)
	if !strings.Contains(content, "Refund &lt;rules&gt;") || !strings.Contains(content, "Never render &lt;script&gt;ticket()&lt;/script&gt;; keep this text.") || strings.Contains(content, "<script>") || !strings.Contains(content, "<dd>open</dd>") {
		t.Fatalf("DevRev ticket context was not safely rendered: %s", content)
	}
	issue, err := json.Marshal(map[string]any{
		"id": "devrev-issue-1", "webhook_id": webhookID, "timestamp": time.Now().UTC().Format(time.RFC3339Nano), "type": "work_created",
		"work_created": map[string]any{"work": map[string]any{"type": "issue", "display_id": "ISS-1", "title": "Not a ticket"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if rr := signedDevRevRequest(t, fixture, issue); rr.Code != http.StatusAccepted || !strings.Contains(rr.Body.String(), `"ignored":true`) || !strings.Contains(rr.Body.String(), `"queued":0`) {
		t.Fatalf("issue status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestDevRevDeletedTicketUsesDocumentedIDOnlyEnvelope(t *testing.T) {
	fixture := newKnowledgeFixture(t)
	webhookID := "don:integration:dvrv-us-1:devo/1:webhook/delete"
	now := knowledge.Now()
	source := knowledge.Source{ID: "src_devrev_delete", RepositoryID: fixture.repositoryID, Provider: knowledge.ProviderDevRev, ExternalRepository: webhookID, IncludePaths: []string{}, ExcludePaths: []string{}, TargetDocumentID: fixture.documentID, TriggerMode: "event", Direction: "into_aidocs", Status: "active", Freshness: knowledge.Freshness{State: "never_synced"}, CreatedBy: fixture.owner, CreatedAt: now, UpdatedAt: now}
	if _, err := fixture.knowledge.PutSource(context.Background(), source); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{
		"id": "devrev-delete-1", "webhook_id": webhookID, "timestamp": time.Now().UTC().Format(time.RFC3339Nano), "type": "work_deleted",
		"work_deleted": map[string]any{"id": "TKT-88"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if rr := signedDevRevRequest(t, fixture, payload); rr.Code != http.StatusAccepted || !strings.Contains(rr.Body.String(), `"queued":1`) {
		t.Fatalf("delete status=%d body=%s", rr.Code, rr.Body.String())
	}
	run := waitForKnowledgeRun(t, fixture, "devrev-delete-1")
	if run.Outcome != "pull_request_opened" || len(run.ContextItems) != 1 || run.ContextItems[0].ExternalID != "TKT-88" || run.ContextItems[0].Metadata["state"] != "deleted" {
		t.Fatalf("run=%+v", run)
	}
	prs, err := fixture.repository.ListPullRequests(context.Background(), fixture.repositoryID)
	if err != nil || len(prs) != 1 {
		t.Fatalf("pull requests=%d err=%v", len(prs), err)
	}
	html, err := fixture.repository.GetVersionHTML(context.Background(), prs[0].HeadVersionID)
	if err != nil {
		t.Fatal(err)
	}
	if content := string(html); !strings.Contains(content, "TKT-88") || !strings.Contains(content, "<dd>deleted</dd>") {
		t.Fatalf("deleted ticket context missing: %s", content)
	}
	issue, err := json.Marshal(map[string]any{
		"id": "devrev-delete-issue", "webhook_id": webhookID, "timestamp": time.Now().UTC().Format(time.RFC3339Nano), "type": "work_deleted",
		"work_deleted": map[string]any{"id": "ISS-12345"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if rr := signedDevRevRequest(t, fixture, issue); rr.Code != http.StatusAccepted || !strings.Contains(rr.Body.String(), `"ignored":true`) || !strings.Contains(rr.Body.String(), `"queued":0`) {
		t.Fatalf("deleted issue status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestKnowledgeTransferRequiresTargetAcceptance(t *testing.T) {
	fixture := newKnowledgeFixture(t)
	base := "/v1/repositories/" + fixture.repositoryID
	request := knowledgeRequest(t, fixture, http.MethodPost, base+"/transfer", "owner", map[string]any{"new_owner_id": fixture.editor.ID})
	if request.Code != http.StatusAccepted || !strings.Contains(request.Body.String(), `"status":"pending"`) {
		t.Fatalf("request status=%d body=%s", request.Code, request.Body.String())
	}
	if rr := knowledgeRequest(t, fixture, http.MethodPatch, base+"/settings", "editor", map[string]any{"required_reviewers": []string{fixture.editor.ID}}); rr.Code != http.StatusForbidden {
		t.Fatalf("pending target mutated settings: status=%d body=%s", rr.Code, rr.Body.String())
	}
	if rr := knowledgeRequest(t, fixture, http.MethodPost, base+"/transfer/accept", "viewer", nil); rr.Code != http.StatusForbidden {
		t.Fatalf("wrong target accepted: status=%d body=%s", rr.Code, rr.Body.String())
	}
	if rr := knowledgeRequest(t, fixture, http.MethodPost, base+"/transfer/accept", "editor", nil); rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"status":"accepted"`) {
		t.Fatalf("accept status=%d body=%s", rr.Code, rr.Body.String())
	}
	repository, err := fixture.repository.GetRepository(context.Background(), fixture.repositoryID)
	if err != nil || repository.CreatedBy.ID != fixture.editor.ID {
		t.Fatalf("transferred repository=%+v err=%v", repository, err)
	}
	document, err := fixture.repository.GetDocument(context.Background(), fixture.documentID)
	if err != nil || document.Owner.ID != fixture.editor.ID {
		t.Fatalf("transferred document=%+v err=%v", document, err)
	}
	if rr := knowledgeRequest(t, fixture, http.MethodPatch, base+"/settings", "editor", map[string]any{"required_reviewers": []string{fixture.viewer.ID}}); rr.Code != http.StatusOK {
		t.Fatalf("new owner status=%d body=%s", rr.Code, rr.Body.String())
	}
	if rr := knowledgeRequest(t, fixture, http.MethodPatch, base+"/settings", "owner", map[string]any{"required_reviewers": []string{fixture.owner.ID}}); rr.Code != http.StatusForbidden {
		t.Fatalf("previous owner retained mutation access: status=%d body=%s", rr.Code, rr.Body.String())
	}
}
