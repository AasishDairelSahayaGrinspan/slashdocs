package repo

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/razorpay/aidocs/api/internal/auth"
)

func TestMemoryPersistsVersionHTML(t *testing.T) {
	ctx := context.Background()
	memory := NewMemory()
	owner := auth.Principal{Type: auth.PrincipalUser, ID: "owner_1", Email: "owner@example.com", Name: "Owner"}

	initialHTML := []byte("<html><body>initial</body></html>")
	wantInitialHTML := append([]byte(nil), initialHTML...)
	document, initialVersion, err := memory.CreateDocument(ctx, owner, "Document", initialHTML)
	if err != nil {
		t.Fatalf("CreateDocument() error = %v", err)
	}
	initialHTML[0] = '!'
	assertVersionHTMLCopy(t, memory, initialVersion.ID, wantInitialHTML)

	updatedHTML := []byte("<html><body>updated</body></html>")
	wantUpdatedHTML := append([]byte(nil), updatedHTML...)
	updatedVersion, err := memory.CreateVersion(ctx, document.ID, initialVersion.ID, "Update", updatedHTML, owner)
	if err != nil {
		t.Fatalf("CreateVersion() error = %v", err)
	}
	updatedHTML[0] = '!'
	assertVersionHTMLCopy(t, memory, updatedVersion.ID, wantUpdatedHTML)

	repository, err := memory.CreateRepository(ctx, owner, CreateRepositoryParams{
		Name:              "Docs",
		Slug:              "docs",
		Scope:             RepositoryScopePersonal,
		NamespaceID:       owner.ID,
		NamespaceName:     owner.Name,
		DocumentID:        document.ID,
		RequiredApprovals: 1,
		Protected:         true,
	})
	if err != nil {
		t.Fatalf("CreateRepository() error = %v", err)
	}

	pullRequestHTML := []byte("<html><body>pull request</body></html>")
	wantPullRequestHTML := append([]byte(nil), pullRequestHTML...)
	pullRequest, err := memory.CreatePullRequest(ctx, repository.ID, document.ID, updatedVersion.ID, "Update document", "Body", "Pull request update", pullRequestHTML, owner)
	if err != nil {
		t.Fatalf("CreatePullRequest() error = %v", err)
	}
	pullRequestHTML[0] = '!'
	assertVersionHTMLCopy(t, memory, pullRequest.HeadVersionID, wantPullRequestHTML)

	if _, err := memory.GetVersionHTML(ctx, "ver_missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetVersionHTML(missing) error = %v, want ErrNotFound", err)
	}
}

func assertVersionHTMLCopy(t *testing.T, memory *Memory, versionID string, want []byte) {
	t.Helper()
	got, err := memory.GetVersionHTML(context.Background(), versionID)
	if err != nil {
		t.Fatalf("GetVersionHTML(%q) error = %v", versionID, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("GetVersionHTML(%q) = %q, want %q", versionID, got, want)
	}

	got[0] = '!'
	again, err := memory.GetVersionHTML(context.Background(), versionID)
	if err != nil {
		t.Fatalf("second GetVersionHTML(%q) error = %v", versionID, err)
	}
	if !bytes.Equal(again, want) {
		t.Fatalf("second GetVersionHTML(%q) = %q after caller mutation, want %q", versionID, again, want)
	}
}
