package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/razorpay/aidocs/api/internal/auth"
	"github.com/razorpay/aidocs/api/internal/blob"
	"github.com/razorpay/aidocs/api/internal/db"
	"github.com/razorpay/aidocs/api/internal/repo"
	pgrepo "github.com/razorpay/aidocs/api/internal/repo/postgres"
)

func TestPostgresDeleteRepositoryCleansBoundaryAndBlobs(t *testing.T) {
	url := os.Getenv("AIDOCS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set AIDOCS_TEST_DATABASE_URL to run postgres integration tests")
	}
	if err := db.Migrate(url); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	blobs := blob.NewMemory()
	store, err := pgrepo.ConnectWithBlob(ctx, url, blobs)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	owner := auth.Principal{Type: auth.PrincipalUser, ID: "usr_repo_delete_" + suffix, Email: "repo-delete-" + suffix + "@example.com", Name: "Repository Owner"}
	viewer, err := store.EnsureUserByEmail(ctx, "repo-delete-viewer-"+suffix+"@example.com")
	if err != nil {
		t.Fatal(err)
	}
	root, err := store.CreateFolder(ctx, owner, "Delete root "+suffix, "")
	if err != nil {
		t.Fatal(err)
	}
	nested, err := store.CreateFolder(ctx, owner, "Nested", root.ID)
	if err != nil {
		t.Fatal(err)
	}
	document, baseVersion, err := store.CreateDocument(ctx, owner, "Delete document "+suffix, []byte("<html><body>base</body></html>"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.MoveDocumentToFolder(ctx, owner, nested.ID, document.ID); err != nil {
		t.Fatal(err)
	}
	repository, err := store.CreateRepository(ctx, owner, repo.CreateRepositoryParams{
		Name:              "Delete fixture " + suffix,
		Slug:              "delete-fixture-" + suffix,
		Scope:             repo.RepositoryScopePersonal,
		NamespaceID:       owner.ID,
		NamespaceName:     owner.Name,
		RootFolderID:      root.ID,
		RequiredApprovals: 1,
		Protected:         true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateGrant(ctx, "folder", root.ID, viewer, repo.RoleEditor, owner); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateGrant(ctx, "document", document.ID, viewer, repo.RoleViewer, owner); err != nil {
		t.Fatal(err)
	}
	if err := store.PutPin(ctx, viewer, "folder", nested.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.PutPin(ctx, viewer, "document", document.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordFolderOpened(ctx, root.ID, viewer); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordDocumentOpened(ctx, document.ID, viewer); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PatchDocumentKV(ctx, document.ID, map[string]json.RawMessage{"status": json.RawMessage(`"draft"`)}, nil, owner); err != nil {
		t.Fatal(err)
	}
	comment, err := store.CreateComment(ctx, document.ID, baseVersion.ID, "Delete me", repo.Anchor{Quote: "base"}, viewer, nil)
	if err != nil {
		t.Fatal(err)
	}
	pullRequest, err := store.CreatePullRequest(ctx, repository.ID, document.ID, baseVersion.ID, "Delete proposal", "", "Update", []byte("<html><body>head</body></html>"), owner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReviewPullRequest(ctx, repository.ID, pullRequest.Number, viewer, repo.ReviewStateApproved, "Approved"); err != nil {
		t.Fatal(err)
	}

	if err := store.DeleteRepository(ctx, repository.ID, viewer); !errors.Is(err, repo.ErrForbidden) {
		t.Fatalf("viewer delete error = %v, want %v", err, repo.ErrForbidden)
	}
	if _, err := store.GetRepository(ctx, repository.ID); err != nil {
		t.Fatalf("repository disappeared after forbidden delete: %v", err)
	}
	if err := store.DeleteRepository(ctx, repository.ID, owner); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteRepository(ctx, repository.ID, owner); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("second delete error = %v, want %v", err, repo.ErrNotFound)
	}

	if _, err := store.GetRepository(ctx, repository.ID); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("GetRepository error = %v, want %v", err, repo.ErrNotFound)
	}
	for _, folderID := range []string{root.ID, nested.ID} {
		if _, err := store.GetFolder(ctx, folderID); !errors.Is(err, repo.ErrNotFound) {
			t.Fatalf("GetFolder(%q) error = %v, want %v", folderID, err, repo.ErrNotFound)
		}
	}
	if _, err := store.GetDocument(ctx, document.ID); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("GetDocument error = %v, want %v", err, repo.ErrNotFound)
	}
	for _, versionID := range []string{baseVersion.ID, pullRequest.HeadVersionID} {
		if _, err := store.GetVersion(ctx, versionID); !errors.Is(err, repo.ErrNotFound) {
			t.Fatalf("GetVersion(%q) error = %v, want %v", versionID, err, repo.ErrNotFound)
		}
		if _, _, err := blobs.Get(ctx, fmt.Sprintf("docs/%s/%s.html", document.ID, versionID)); !errors.Is(err, blob.ErrNotFound) {
			t.Fatalf("blob %q error = %v, want %v", versionID, err, blob.ErrNotFound)
		}
	}
	if _, err := store.GetComment(ctx, comment.ID); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("GetComment error = %v, want %v", err, repo.ErrNotFound)
	}
	if _, err := store.GetPullRequest(ctx, repository.ID, pullRequest.Number); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("GetPullRequest error = %v, want %v", err, repo.ErrNotFound)
	}
	grants, err := store.ListGrants(ctx, document.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(grants) != 0 {
		t.Fatalf("document grants remain: %+v", grants)
	}
	pins, err := store.ListPins(ctx, viewer)
	if err != nil {
		t.Fatal(err)
	}
	for _, pin := range pins {
		if pin.ResourceID == root.ID || pin.ResourceID == nested.ID || pin.ResourceID == document.ID {
			t.Fatalf("pin remains for deleted resource: %+v", pin)
		}
	}
}
