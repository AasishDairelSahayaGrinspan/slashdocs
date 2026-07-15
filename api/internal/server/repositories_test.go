//go:build testfixtures

package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/razorpay/aidocs/api/internal/server"
)

const (
	repositoryOwner     = "user:owner_1:owner@example.com:Owner"
	repositoryEditor    = "user:editor_1:editor@example.com:Editor"
	repositoryCommenter = "user:commenter_1:commenter@example.com:Commenter"
	repositoryViewer    = "user:viewer_1:viewer@example.com:Viewer"
)

type repositoryHTTPResponse struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Slug              string `json:"slug"`
	Description       string `json:"description"`
	Scope             string `json:"scope"`
	RootFolderID      string `json:"root_folder_id"`
	RequiredApprovals int    `json:"required_approvals"`
	Protected         bool   `json:"protected"`
}

type pullRequestHTTPResponse struct {
	ID            string `json:"id"`
	RepositoryID  string `json:"repository_id"`
	Number        int    `json:"number"`
	DocumentID    string `json:"document_id"`
	BaseVersionID string `json:"base_version_id"`
	HeadVersionID string `json:"head_version_id"`
	Patchset      int    `json:"patchset"`
	Changes       []struct {
		Operation     string `json:"operation"`
		DocumentID    string `json:"document_id"`
		BaseVersionID string `json:"base_version_id"`
		HeadVersionID string `json:"head_version_id"`
		Title         string `json:"title"`
	} `json:"changes"`
	Title              string `json:"title"`
	Status             string `json:"status"`
	RequiredApprovals  int    `json:"required_approvals"`
	Mergeable          bool   `json:"mergeable"`
	MergeBlockedReason string `json:"merge_blocked_reason"`
}

func TestRepositoriesCreateListGetAndSlugConflict(t *testing.T) {
	h := newTestServer()
	created := createRepositoryHTTP(t, h, repositoryOwner, map[string]any{
		"name":        "Product handbook",
		"slug":        "product-handbook",
		"description": "The product team's source of truth",
		"scope":       "personal",
		"document_id": "doc_1",
	})
	if created.ID == "" {
		t.Fatalf("created repository has no id: %#v", created)
	}
	if created.Name != "Product handbook" || created.Slug != "product-handbook" || created.Scope != "personal" {
		t.Fatalf("created repository = %#v", created)
	}
	if created.RequiredApprovals != 1 || !created.Protected {
		t.Fatalf("repository defaults = approvals %d protected %v, want 1/true", created.RequiredApprovals, created.Protected)
	}

	list := do(t, h, http.MethodGet, "/v1/repositories", "", map[string]string{
		"X-Test-Principal": repositoryOwner,
	})
	assertStatus(t, list, http.StatusOK)
	if !repositoryListContains(t, list, created.ID) {
		t.Fatalf("repository %q missing from list: %s", created.ID, list.Body.String())
	}

	get := do(t, h, http.MethodGet, "/v1/repositories/"+created.ID, "", map[string]string{
		"X-Test-Principal": repositoryOwner,
	})
	assertStatus(t, get, http.StatusOK)
	got := decodeRepositoryHTTP(t, get)
	if got.ID != created.ID || got.Slug != created.Slug || got.Description != created.Description {
		t.Fatalf("get repository = %#v, want %#v", got, created)
	}

	conflict := repositoryJSONRequest(t, h, http.MethodPost, "/v1/repositories", repositoryOwner, map[string]any{
		"name":        "Another handbook",
		"slug":        "product-handbook",
		"scope":       "personal",
		"document_id": "doc_1",
	})
	assertStatus(t, conflict, http.StatusConflict)
}

func TestRepositoryCanBeCreatedEmpty(t *testing.T) {
	h := newTestServer()
	repository := createRepositoryHTTP(t, h, repositoryOwner, map[string]any{
		"name":        "Empty handbook",
		"slug":        "empty-handbook",
		"description": "Start writing after the repository exists",
		"scope":       "personal",
	})
	if repository.RootFolderID == "" {
		t.Fatalf("empty repository has no root folder: %#v", repository)
	}
	list := do(t, h, http.MethodGet, "/v1/repositories/"+repository.ID+"/documents", "", map[string]string{
		"X-Test-Principal": repositoryOwner,
	})
	assertStatus(t, list, http.StatusOK)
	var envelope struct {
		Items []any `json:"items"`
	}
	decodeRepositoryJSON(t, list, &envelope)
	if len(envelope.Items) != 0 {
		t.Fatalf("empty repository documents = %s", list.Body.String())
	}
	body, contentType := multipartBody(t, map[string]string{"title": "Getting started"}, "file", "getting-started.html", "<html><body>start</body></html>")
	bootstrap := doRaw(t, h, http.MethodPost, "/v1/repositories/"+repository.ID+"/documents", body, map[string]string{
		"Content-Type":     contentType,
		"X-Test-Principal": repositoryOwner,
	})
	assertStatus(t, bootstrap, http.StatusCreated)
	var created struct {
		Document struct {
			ID               string `json:"id"`
			CurrentVersionID string `json:"current_version_id"`
			ParentFolderID   string `json:"parent_folder_id"`
		} `json:"document"`
		Version struct {
			ID     string `json:"id"`
			Number int    `json:"number"`
		} `json:"version"`
	}
	decodeRepositoryJSON(t, bootstrap, &created)
	if created.Document.ID == "" || created.Document.CurrentVersionID != created.Version.ID || created.Document.ParentFolderID != repository.RootFolderID || created.Version.Number != 1 {
		t.Fatalf("bootstrap response = %s", bootstrap.Body.String())
	}

	secondBody, secondContentType := multipartBody(t, map[string]string{"title": "Second"}, "file", "second.html", "<html></html>")
	second := doRaw(t, h, http.MethodPost, "/v1/repositories/"+repository.ID+"/documents", secondBody, map[string]string{
		"Content-Type":     secondContentType,
		"X-Test-Principal": repositoryOwner,
	})
	assertStatus(t, second, http.StatusConflict)
	assertRepositoryErrorCode(t, second, "repository_not_empty")

	direct := createVersionHTTP(t, h, created.Document.ID, created.Version.ID, repositoryOwner, "<html>changed</html>")
	assertStatus(t, direct, http.StatusConflict)
	assertRepositoryErrorCode(t, direct, "pull_request_required")

	both := repositoryJSONRequest(t, h, http.MethodPost, "/v1/repositories", repositoryOwner, map[string]any{
		"name":           "Invalid seed",
		"slug":           "invalid-seed",
		"scope":          "personal",
		"root_folder_id": repository.RootFolderID,
		"document_id":    "doc_1",
	})
	assertStatus(t, both, http.StatusBadRequest)
}

func TestDeleteRepositoryIsOwnerOnlyAndReturnsNoContent(t *testing.T) {
	h := newConfiguredTestServer(server.Config{Environment: "test", Deployment: "org", OrgName: "Razorpay"})
	repository := createRepositoryHTTP(t, h, repositoryOwner, map[string]any{
		"name":  "Disposable handbook",
		"slug":  "disposable-handbook",
		"scope": "organization",
	})

	viewerDelete := do(t, h, http.MethodDelete, "/v1/repositories/"+repository.ID, "", map[string]string{
		"X-Test-Principal": repositoryViewer,
	})
	assertStatus(t, viewerDelete, http.StatusForbidden)
	assertRepositoryErrorCode(t, viewerDelete, "forbidden")

	stillExists := do(t, h, http.MethodGet, "/v1/repositories/"+repository.ID, "", map[string]string{
		"X-Test-Principal": repositoryOwner,
	})
	assertStatus(t, stillExists, http.StatusOK)

	ownerDelete := do(t, h, http.MethodDelete, "/v1/repositories/"+repository.ID, "", map[string]string{
		"X-Test-Principal": repositoryOwner,
	})
	assertStatus(t, ownerDelete, http.StatusNoContent)
	if ownerDelete.Body.Len() != 0 {
		t.Fatalf("delete response body = %q, want empty", ownerDelete.Body.String())
	}

	missingDelete := do(t, h, http.MethodDelete, "/v1/repositories/"+repository.ID, "", map[string]string{
		"X-Test-Principal": repositoryOwner,
	})
	assertStatus(t, missingDelete, http.StatusNotFound)
	assertRepositoryErrorCode(t, missingDelete, "not_found")
}

func TestOrganizationRepositoryDeploymentPolicy(t *testing.T) {
	create := map[string]any{
		"name":        "Payments",
		"slug":        "payments",
		"scope":       "organization",
		"document_id": "doc_1",
	}

	public := newConfiguredTestServer(server.Config{Environment: "test", Deployment: "public"})
	forbidden := repositoryJSONRequest(t, public, http.MethodPost, "/v1/repositories", repositoryOwner, create)
	assertStatus(t, forbidden, http.StatusForbidden)
	assertRepositoryErrorCode(t, forbidden, "forbidden")

	organization := newConfiguredTestServer(server.Config{Environment: "test", Deployment: "org", OrgName: "Razorpay"})
	accepted := repositoryJSONRequest(t, organization, http.MethodPost, "/v1/repositories", repositoryOwner, create)
	assertStatus(t, accepted, http.StatusCreated)
	repository := decodeRepositoryHTTP(t, accepted)
	if repository.Scope != "organization" || repository.Slug != "payments" {
		t.Fatalf("organization repository = %#v", repository)
	}
	organizationMember := "user:org_member:member@example.com:Member"
	listed := do(t, organization, http.MethodGet, "/v1/repositories", "", map[string]string{"X-Test-Principal": organizationMember})
	assertStatus(t, listed, http.StatusOK)
	if !repositoryListContains(t, listed, repository.ID) {
		t.Fatalf("organization repository %q missing for member: %s", repository.ID, listed.Body.String())
	}
	assertRepositoryContainsDocument(t, organization, repository.ID, "doc_1", organizationMember)
	document := do(t, organization, http.MethodGet, "/v1/documents/doc_1", "", map[string]string{"X-Test-Principal": organizationMember})
	assertStatus(t, document, http.StatusOK)
}

func TestUserProfileIncludesOnlyVisibleActivity(t *testing.T) {
	h, repository := newGovernedRepositoryTestServer(t)
	pullRequest := createPullRequestHTTP(t, h, repository.ID, repositoryEditor, "ver_1", "Clarify profile activity", "<html><body>profile</body></html>")
	approvePullRequestHTTP(t, h, repository.ID, pullRequest.Number, repositoryOwner)
	merge := do(t, h, http.MethodPost, pullRequestPath(repository.ID, pullRequest.Number)+"/merge", "", map[string]string{
		"X-Test-Principal": repositoryEditor,
	})
	assertStatus(t, merge, http.StatusOK)

	profileResponse := do(t, h, http.MethodGet, "/v1/users/editor_1/profile", "", map[string]string{
		"X-Test-Principal": repositoryViewer,
	})
	assertStatus(t, profileResponse, http.StatusOK)
	var profile struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
		Stats struct {
			DocumentsCreated          int `json:"documents_created"`
			PullRequestsAuthored      int `json:"pull_requests_authored"`
			Contributions             int `json:"contributions"`
			RepositoriesContributedTo int `json:"repositories_contributed_to"`
		} `json:"stats"`
		Documents    []any `json:"documents"`
		PullRequests []struct {
			PullRequest struct {
				ID string `json:"id"`
			} `json:"pull_request"`
			Repository struct {
				ID string `json:"id"`
			} `json:"repository"`
		} `json:"pull_requests"`
		Contributions []struct {
			Type         string `json:"type"`
			RepositoryID string `json:"repository_id"`
		} `json:"contributions"`
	}
	decodeRepositoryJSON(t, profileResponse, &profile)
	if profile.User.ID != "editor_1" || profile.Stats.DocumentsCreated != 0 || profile.Stats.PullRequestsAuthored != 1 || len(profile.PullRequests) != 1 {
		t.Fatalf("editor profile = %#v", profile)
	}
	if profile.PullRequests[0].PullRequest.ID != pullRequest.ID || profile.PullRequests[0].Repository.ID != repository.ID {
		t.Fatalf("profile pull request = %#v, want %q in %q", profile.PullRequests[0], pullRequest.ID, repository.ID)
	}
	if profile.Stats.Contributions < 3 || profile.Stats.RepositoriesContributedTo != 1 {
		t.Fatalf("profile stats = %#v, contributions = %#v", profile.Stats, profile.Contributions)
	}

	privateProfile := do(t, h, http.MethodGet, "/v1/users/owner_1/profile", "", map[string]string{
		"X-Test-Principal": "user:stranger:stranger@example.com:Stranger",
	})
	assertStatus(t, privateProfile, http.StatusOK)
	var private struct {
		Documents     []any `json:"documents"`
		PullRequests  []any `json:"pull_requests"`
		Contributions []any `json:"contributions"`
	}
	decodeRepositoryJSON(t, privateProfile, &private)
	if len(private.Documents) != 0 || len(private.PullRequests) != 0 || len(private.Contributions) != 0 {
		t.Fatalf("inaccessible profile activity leaked: %s", privateProfile.Body.String())
	}
}

func TestRepositoryCanConvertLooseDocument(t *testing.T) {
	h := newTestServer()
	repository := createRepositoryHTTP(t, h, repositoryOwner, map[string]any{
		"name":        "Fixture document",
		"slug":        "fixture-document",
		"scope":       "personal",
		"document_id": "doc_1",
	})
	assertRepositoryContainsDocument(t, h, repository.ID, "doc_1", repositoryOwner)

	direct := createVersionHTTP(t, h, "doc_1", "ver_1", repositoryEditor, "direct update")
	assertStatus(t, direct, http.StatusConflict)
	assertRepositoryErrorCode(t, direct, "pull_request_required")
}

func TestRepositoryFolderConversionRequiresPullRequests(t *testing.T) {
	h, repository := newGovernedRepositoryTestServer(t)
	assertRepositoryContainsDocument(t, h, repository.ID, "doc_1", repositoryOwner)

	direct := createVersionHTTP(t, h, "doc_1", "ver_1", repositoryEditor, "direct update")
	assertStatus(t, direct, http.StatusConflict)
	assertRepositoryErrorCode(t, direct, "pull_request_required")
}

func TestRepositoryPullRequestApprovalAndMerge(t *testing.T) {
	h, repository := newGovernedRepositoryTestServer(t)
	pr := createPullRequestHTTP(t, h, repository.ID, repositoryEditor, "ver_1", "Clarify the fixture", "<html><body>head</body></html>")
	if pr.RepositoryID != repository.ID || pr.DocumentID != "doc_1" || pr.BaseVersionID != "ver_1" || pr.HeadVersionID == "" || pr.Status != "open" {
		t.Fatalf("created pull request = %#v", pr)
	}

	documentBefore := getDocumentHTTP(t, h, "doc_1", repositoryOwner)
	if documentBefore.CurrentVersionID != "ver_1" {
		t.Fatalf("current version after opening PR = %q, want ver_1", documentBefore.CurrentVersionID)
	}
	head := do(t, h, http.MethodGet, "/v1/documents/doc_1/versions/"+pr.HeadVersionID, "", map[string]string{
		"X-Test-Principal": repositoryOwner,
	})
	assertStatus(t, head, http.StatusOK)

	selfReview := repositoryJSONRequest(t, h, http.MethodPost, pullRequestPath(repository.ID, pr.Number)+"/reviews", repositoryEditor, map[string]any{
		"state": "approved",
		"body":  "self approval",
	})
	assertStatus(t, selfReview, http.StatusForbidden)
	assertRepositoryErrorCode(t, selfReview, "self_review")

	tooEarly := do(t, h, http.MethodPost, pullRequestPath(repository.ID, pr.Number)+"/merge", "", map[string]string{
		"X-Test-Principal": repositoryEditor,
	})
	assertStatus(t, tooEarly, http.StatusConflict)
	assertRepositoryErrorCode(t, tooEarly, "approval_required")

	approve := repositoryJSONRequest(t, h, http.MethodPost, pullRequestPath(repository.ID, pr.Number)+"/reviews", repositoryOwner, map[string]any{
		"state": "approved",
		"body":  "Ship it",
	})
	assertStatus(t, approve, http.StatusOK)

	merge := do(t, h, http.MethodPost, pullRequestPath(repository.ID, pr.Number)+"/merge", "", map[string]string{
		"X-Test-Principal": repositoryEditor,
	})
	assertStatus(t, merge, http.StatusOK)

	merged := getPullRequestHTTP(t, h, repository.ID, pr.Number, repositoryOwner)
	if merged.Status != "merged" {
		t.Fatalf("pull request status after merge = %q, want merged: %#v", merged.Status, merged)
	}
	documentAfter := getDocumentHTTP(t, h, "doc_1", repositoryOwner)
	if documentAfter.CurrentVersionID != pr.HeadVersionID {
		t.Fatalf("current version after merge = %q, want %q", documentAfter.CurrentVersionID, pr.HeadVersionID)
	}
}

func TestRepositoryPullRequestDocumentFallbackDoesNotDiscloseOtherChanges(t *testing.T) {
	h, repository := newGovernedRepositoryTestServer(t)

	// The commenter retains a direct grant on doc_1 but has no grant on the
	// repository root. Preserve this compatibility path for a single-document
	// pull request.
	single := createPullRequestHTTP(t, h, repository.ID, repositoryEditor, "ver_1", "Single document", "<html><body>single</body></html>")
	singleResponse := do(t, h, http.MethodGet, pullRequestPath(repository.ID, single.Number), "", map[string]string{
		"X-Test-Principal": repositoryCommenter,
	})
	assertStatus(t, singleResponse, http.StatusOK)

	// The compatibility document_id below is doc_1 (the first change), but the
	// response also describes a new document. A direct doc_1 grant must not
	// disclose that repository-scoped change set.
	multiResponse := repositoryJSONRequest(t, h, http.MethodPost, "/v1/repositories/"+repository.ID+"/pull-requests/change-set", repositoryEditor, map[string]any{
		"title": "Multiple documents",
		"changes": []map[string]any{
			{
				"operation":       "update",
				"document_id":     "doc_1",
				"base_version_id": "ver_1",
				"content_html":    "<html><body>updated fixture</body></html>",
			},
			{
				"operation":    "create",
				"folder_id":    repository.RootFolderID,
				"title":        "Undisclosed draft",
				"content_html": "<html><body>private draft</body></html>",
			},
		},
	})
	assertStatus(t, multiResponse, http.StatusCreated)
	multi := decodePullRequestHTTP(t, multiResponse)
	if multi.DocumentID != "doc_1" || len(multi.Changes) != 2 || multi.Changes[1].DocumentID == "" {
		t.Fatalf("multi-document pull request = %s", multiResponse.Body.String())
	}

	repositoryViewerResponse := do(t, h, http.MethodGet, pullRequestPath(repository.ID, multi.Number), "", map[string]string{
		"X-Test-Principal": repositoryViewer,
	})
	assertStatus(t, repositoryViewerResponse, http.StatusOK)

	documentOnlyResponse := do(t, h, http.MethodGet, pullRequestPath(repository.ID, multi.Number), "", map[string]string{
		"X-Test-Principal": repositoryCommenter,
	})
	assertStatus(t, documentOnlyResponse, http.StatusNotFound)
	assertRepositoryErrorCode(t, documentOnlyResponse, "not_found")
}

func TestRepositoryPullRequestChangesRequestedBlocksMerge(t *testing.T) {
	h, repository := newGovernedRepositoryTestServer(t)
	pr := createPullRequestHTTP(t, h, repository.ID, repositoryEditor, "ver_1", "Risky change", "<html><body>needs work</body></html>")

	review := repositoryJSONRequest(t, h, http.MethodPost, pullRequestPath(repository.ID, pr.Number)+"/reviews", repositoryOwner, map[string]any{
		"state": "changes_requested",
		"body":  "Please address the security section",
	})
	assertStatus(t, review, http.StatusOK)

	merge := do(t, h, http.MethodPost, pullRequestPath(repository.ID, pr.Number)+"/merge", "", map[string]string{
		"X-Test-Principal": repositoryEditor,
	})
	assertStatus(t, merge, http.StatusConflict)
	assertRepositoryErrorCode(t, merge, "changes_requested")
}

func TestRepositoryPullRequestStaleBaseBlocksMerge(t *testing.T) {
	h, repository := newGovernedRepositoryTestServer(t)
	stale := createPullRequestHTTP(t, h, repository.ID, repositoryEditor, "ver_1", "First proposal", "<html><body>first</body></html>")
	winner := createPullRequestHTTP(t, h, repository.ID, repositoryEditor, "ver_1", "Second proposal", "<html><body>second</body></html>")

	approvePullRequestHTTP(t, h, repository.ID, winner.Number, repositoryOwner)
	mergeWinner := do(t, h, http.MethodPost, pullRequestPath(repository.ID, winner.Number)+"/merge", "", map[string]string{
		"X-Test-Principal": repositoryEditor,
	})
	assertStatus(t, mergeWinner, http.StatusOK)

	approvePullRequestHTTP(t, h, repository.ID, stale.Number, repositoryOwner)
	mergeStale := do(t, h, http.MethodPost, pullRequestPath(repository.ID, stale.Number)+"/merge", "", map[string]string{
		"X-Test-Principal": repositoryEditor,
	})
	assertStatus(t, mergeStale, http.StatusConflict)
	assertRepositoryErrorCode(t, mergeStale, "stale_pull_request")
}

func TestRepositoryViewerCannotCreateReviewOrMerge(t *testing.T) {
	h, repository := newGovernedRepositoryTestServer(t)

	createBody, contentType := multipartBody(t, map[string]string{
		"document_id":     "doc_1",
		"base_version_id": "ver_1",
		"title":           "Viewer proposal",
		"body":            "should not be accepted",
		"change_summary":  "viewer edit",
	}, "file", "viewer.html", "<html><body>viewer</body></html>")
	create := doRaw(t, h, http.MethodPost, "/v1/repositories/"+repository.ID+"/pull-requests", createBody, map[string]string{
		"Content-Type":     contentType,
		"X-Test-Principal": repositoryViewer,
	})
	assertStatus(t, create, http.StatusForbidden)

	pr := createPullRequestHTTP(t, h, repository.ID, repositoryEditor, "ver_1", "Editor proposal", "<html><body>editor</body></html>")
	review := repositoryJSONRequest(t, h, http.MethodPost, pullRequestPath(repository.ID, pr.Number)+"/reviews", repositoryViewer, map[string]any{
		"state": "approved",
		"body":  "viewer approval",
	})
	assertStatus(t, review, http.StatusForbidden)

	merge := do(t, h, http.MethodPost, pullRequestPath(repository.ID, pr.Number)+"/merge", "", map[string]string{
		"X-Test-Principal": repositoryViewer,
	})
	assertStatus(t, merge, http.StatusForbidden)
}

func TestRepositoryPullRequestCanBeClosed(t *testing.T) {
	h, repository := newGovernedRepositoryTestServer(t)
	pr := createPullRequestHTTP(t, h, repository.ID, repositoryEditor, "ver_1", "Abandoned proposal", "<html><body>unused</body></html>")

	closeResponse := repositoryJSONRequest(t, h, http.MethodPatch, pullRequestPath(repository.ID, pr.Number), repositoryEditor, map[string]any{
		"status": "closed",
	})
	assertStatus(t, closeResponse, http.StatusOK)
	closed := getPullRequestHTTP(t, h, repository.ID, pr.Number, repositoryOwner)
	if closed.Status != "closed" {
		t.Fatalf("pull request status after close = %q, want closed", closed.Status)
	}
}

func TestRepositoryChangeSetPatchsetAndRenameRoutes(t *testing.T) {
	h, repository := newGovernedRepositoryTestServer(t)
	create := repositoryJSONRequest(t, h, http.MethodPost, "/v1/repositories/"+repository.ID+"/pull-requests/change-set", repositoryEditor, map[string]any{
		"title":          "Structured proposal",
		"change_summary": "first patchset",
		"changes": []map[string]any{{
			"operation":       "update",
			"document_id":     "doc_1",
			"base_version_id": "ver_1",
			"content_html":    "<html><body>first patchset</body></html>",
		}},
	})
	assertStatus(t, create, http.StatusCreated)
	pr := decodePullRequestHTTP(t, create)
	if pr.Patchset != 1 || len(pr.Changes) != 1 || pr.Changes[0].Operation != "update" || pr.Changes[0].HeadVersionID == "" {
		t.Fatalf("change-set response = %s", create.Body.String())
	}
	approvePullRequestHTTP(t, h, repository.ID, pr.Number, repositoryOwner)

	patchset := repositoryJSONRequest(t, h, http.MethodPost, pullRequestPath(repository.ID, pr.Number)+"/patchsets", repositoryEditor, map[string]any{
		"change_summary": "second patchset",
		"changes": []map[string]any{{
			"operation":       "update",
			"document_id":     "doc_1",
			"base_version_id": "ver_1",
			"content_html":    "<html><body>second patchset</body></html>",
		}},
	})
	assertStatus(t, patchset, http.StatusOK)
	patched := decodePullRequestHTTP(t, patchset)
	if patched.Patchset != 2 || patched.Mergeable || patched.MergeBlockedReason != "approval_required" || patched.HeadVersionID == pr.HeadVersionID {
		t.Fatalf("patchset response = %s", patchset.Body.String())
	}
	approvePullRequestHTTP(t, h, repository.ID, patched.Number, repositoryOwner)
	merge := do(t, h, http.MethodPost, pullRequestPath(repository.ID, patched.Number)+"/merge", "", map[string]string{"X-Test-Principal": repositoryEditor})
	assertStatus(t, merge, http.StatusOK)

	rename := repositoryJSONRequest(t, h, http.MethodPost, "/v1/repositories/"+repository.ID+"/pull-requests/change-set", repositoryEditor, map[string]any{
		"title": "Rename the document",
		"changes": []map[string]any{{
			"operation":       "rename",
			"document_id":     "doc_1",
			"base_version_id": patched.HeadVersionID,
			"title":           "Renamed by pull request",
		}},
	})
	assertStatus(t, rename, http.StatusCreated)
	renamePR := decodePullRequestHTTP(t, rename)
	approvePullRequestHTTP(t, h, repository.ID, renamePR.Number, repositoryOwner)
	renameMerge := do(t, h, http.MethodPost, pullRequestPath(repository.ID, renamePR.Number)+"/merge", "", map[string]string{"X-Test-Principal": repositoryEditor})
	assertStatus(t, renameMerge, http.StatusOK)
	document := do(t, h, http.MethodGet, "/v1/documents/doc_1", "", map[string]string{"X-Test-Principal": repositoryOwner})
	assertStatus(t, document, http.StatusOK)
	var renamed struct {
		Title string `json:"title"`
	}
	decodeRepositoryJSON(t, document, &renamed)
	if renamed.Title != "Renamed by pull request" {
		t.Fatalf("renamed document = %s", document.Body.String())
	}
}

func TestRepositoryStalePullRequestRebaseRoute(t *testing.T) {
	h, repository := newGovernedRepositoryTestServer(t)
	stale := createPullRequestHTTP(t, h, repository.ID, repositoryEditor, "ver_1", "Stale proposal", "<html><body>stale</body></html>")
	winner := createPullRequestHTTP(t, h, repository.ID, repositoryEditor, "ver_1", "Winner", "<html><body>winner</body></html>")
	approvePullRequestHTTP(t, h, repository.ID, winner.Number, repositoryOwner)
	merge := do(t, h, http.MethodPost, pullRequestPath(repository.ID, winner.Number)+"/merge", "", map[string]string{"X-Test-Principal": repositoryEditor})
	assertStatus(t, merge, http.StatusOK)
	rebase := do(t, h, http.MethodPost, pullRequestPath(repository.ID, stale.Number)+"/rebase", "", map[string]string{"X-Test-Principal": repositoryEditor})
	assertStatus(t, rebase, http.StatusOK)
	rebased := decodePullRequestHTTP(t, rebase)
	if rebased.Patchset != 2 || len(rebased.Changes) != 1 || rebased.Changes[0].BaseVersionID != winner.HeadVersionID || rebased.MergeBlockedReason != "approval_required" {
		t.Fatalf("rebased pull request = %s", rebase.Body.String())
	}
}

func newGovernedRepositoryTestServer(t *testing.T) (http.Handler, repositoryHTTPResponse) {
	t.Helper()
	h := newTestServer()

	folder := repositoryJSONRequest(t, h, http.MethodPost, "/v1/folders", repositoryOwner, map[string]any{
		"title": "Repository root",
	})
	assertStatus(t, folder, http.StatusCreated)
	var createdFolder struct {
		ID string `json:"id"`
	}
	decodeRepositoryJSON(t, folder, &createdFolder)
	if createdFolder.ID == "" {
		t.Fatalf("created folder has no id: %s", folder.Body.String())
	}

	move := repositoryJSONRequest(t, h, http.MethodPost, "/v1/folders/"+createdFolder.ID+"/documents", repositoryOwner, map[string]any{
		"document_id": "doc_1",
	})
	assertStatus(t, move, http.StatusOK)

	for _, grant := range []struct {
		principalID string
		role        string
	}{
		{principalID: "editor_1", role: "editor"},
		{principalID: "viewer_1", role: "viewer"},
	} {
		response := repositoryJSONRequest(t, h, http.MethodPost, "/v1/folders/"+createdFolder.ID+"/grants", repositoryOwner, map[string]any{
			"principal": map[string]any{"type": "user", "id": grant.principalID},
			"role":      grant.role,
		})
		assertStatus(t, response, http.StatusCreated)
	}

	repository := createRepositoryHTTP(t, h, repositoryOwner, map[string]any{
		"name":               "Governed fixture",
		"slug":               "governed-fixture",
		"description":        "Repository PR test fixture",
		"scope":              "personal",
		"root_folder_id":     createdFolder.ID,
		"required_approvals": 1,
		"protected":          true,
	})
	return h, repository
}

func createRepositoryHTTP(t *testing.T, h http.Handler, principal string, body map[string]any) repositoryHTTPResponse {
	t.Helper()
	rr := repositoryJSONRequest(t, h, http.MethodPost, "/v1/repositories", principal, body)
	assertStatus(t, rr, http.StatusCreated)
	return decodeRepositoryHTTP(t, rr)
}

func createPullRequestHTTP(t *testing.T, h http.Handler, repositoryID, principal, baseVersionID, title, html string) pullRequestHTTPResponse {
	t.Helper()
	body, contentType := multipartBody(t, map[string]string{
		"document_id":     "doc_1",
		"base_version_id": baseVersionID,
		"title":           title,
		"body":            "Please review this document update.",
		"change_summary":  title,
	}, "file", "proposal.html", html)
	rr := doRaw(t, h, http.MethodPost, "/v1/repositories/"+repositoryID+"/pull-requests", body, map[string]string{
		"Content-Type":     contentType,
		"X-Test-Principal": principal,
	})
	assertStatus(t, rr, http.StatusCreated)
	return decodePullRequestHTTP(t, rr)
}

func approvePullRequestHTTP(t *testing.T, h http.Handler, repositoryID string, number int, principal string) {
	t.Helper()
	rr := repositoryJSONRequest(t, h, http.MethodPost, pullRequestPath(repositoryID, number)+"/reviews", principal, map[string]any{
		"state": "approved",
		"body":  "Approved",
	})
	assertStatus(t, rr, http.StatusOK)
}

func getPullRequestHTTP(t *testing.T, h http.Handler, repositoryID string, number int, principal string) pullRequestHTTPResponse {
	t.Helper()
	rr := do(t, h, http.MethodGet, pullRequestPath(repositoryID, number), "", map[string]string{
		"X-Test-Principal": principal,
	})
	assertStatus(t, rr, http.StatusOK)
	return decodePullRequestHTTP(t, rr)
}

func pullRequestPath(repositoryID string, number int) string {
	return "/v1/repositories/" + repositoryID + "/pull-requests/" + jsonNumber(number)
}

func jsonNumber(number int) string {
	b, _ := json.Marshal(number)
	return string(b)
}

func createVersionHTTP(t *testing.T, h http.Handler, documentID, baseVersionID, principal, html string) *httptest.ResponseRecorder {
	t.Helper()
	body, contentType := multipartBody(t, map[string]string{
		"base_version_id": baseVersionID,
		"change_summary":  "direct update",
	}, "file", "direct.html", html)
	return doRaw(t, h, http.MethodPost, "/v1/documents/"+documentID+"/versions", body, map[string]string{
		"Content-Type":     contentType,
		"X-Test-Principal": principal,
	})
}

func getDocumentHTTP(t *testing.T, h http.Handler, documentID, principal string) struct {
	ID               string `json:"id"`
	CurrentVersionID string `json:"current_version_id"`
} {
	t.Helper()
	rr := do(t, h, http.MethodGet, "/v1/documents/"+documentID, "", map[string]string{
		"X-Test-Principal": principal,
	})
	assertStatus(t, rr, http.StatusOK)
	var document struct {
		ID               string `json:"id"`
		CurrentVersionID string `json:"current_version_id"`
	}
	decodeRepositoryJSON(t, rr, &document)
	return document
}

func assertRepositoryContainsDocument(t *testing.T, h http.Handler, repositoryID, documentID, principal string) {
	t.Helper()
	rr := do(t, h, http.MethodGet, "/v1/repositories/"+repositoryID+"/documents", "", map[string]string{
		"X-Test-Principal": principal,
	})
	assertStatus(t, rr, http.StatusOK)
	var envelope struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	decodeRepositoryJSON(t, rr, &envelope)
	for _, document := range envelope.Items {
		if document.ID == documentID {
			return
		}
	}
	t.Fatalf("document %q missing from repository %q: %s", documentID, repositoryID, rr.Body.String())
}

func repositoryListContains(t *testing.T, rr *httptest.ResponseRecorder, repositoryID string) bool {
	t.Helper()
	var envelope struct {
		Items []repositoryHTTPResponse `json:"items"`
	}
	decodeRepositoryJSON(t, rr, &envelope)
	for _, repository := range envelope.Items {
		if repository.ID == repositoryID {
			return true
		}
	}
	return false
}

func repositoryJSONRequest(t *testing.T, h http.Handler, method, path, principal string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return do(t, h, method, path, string(b), map[string]string{
		"Content-Type":     "application/json",
		"X-Test-Principal": principal,
	})
}

func decodeRepositoryHTTP(t *testing.T, rr *httptest.ResponseRecorder) repositoryHTTPResponse {
	t.Helper()
	var repository repositoryHTTPResponse
	decodeRepositoryJSON(t, rr, &repository)
	return repository
}

func decodePullRequestHTTP(t *testing.T, rr *httptest.ResponseRecorder) pullRequestHTTPResponse {
	t.Helper()
	var pullRequest pullRequestHTTPResponse
	decodeRepositoryJSON(t, rr, &pullRequest)
	return pullRequest
}

func decodeRepositoryJSON(t *testing.T, rr *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(rr.Body.Bytes(), target); err != nil {
		t.Fatalf("response is not valid JSON: %v; body=%s", err, rr.Body.String())
	}
}

func assertRepositoryErrorCode(t *testing.T, rr *httptest.ResponseRecorder, want string) {
	t.Helper()
	var response struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	decodeRepositoryJSON(t, rr, &response)
	if response.Error.Code != want {
		t.Fatalf("error code = %q, want %q; body=%s", response.Error.Code, want, rr.Body.String())
	}
}
