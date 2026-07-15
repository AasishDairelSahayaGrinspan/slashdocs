package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/razorpay/aidocs/api/internal/auth"
	"github.com/razorpay/aidocs/api/internal/repo"
)

func TestPostgresProtectedRepositoryRejectsDirectTreeMutations(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	owner := auth.Principal{Type: auth.PrincipalUser, ID: "usr_boundary_owner", Email: "boundary-owner@example.com", Name: "Boundary Owner"}

	root, err := store.CreateFolder(ctx, owner, "Repository root", "")
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateFolder(ctx, owner, "Child", root.ID)
	if err != nil {
		t.Fatal(err)
	}
	document, _, err := store.CreateDocument(ctx, owner, "Governed", []byte("<html>governed</html>"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.MoveDocumentToFolder(ctx, owner, child.ID, document.ID); err != nil {
		t.Fatal(err)
	}
	repository, err := store.CreateRepository(ctx, owner, repo.CreateRepositoryParams{
		Name:              "Governed repository",
		Slug:              "governed-repository",
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
	var pullRequestRequired *repo.PullRequestRequiredError
	if err := store.DeleteDocument(ctx, document.ID); !errors.As(err, &pullRequestRequired) || pullRequestRequired.RepositoryID != repository.ID {
		t.Fatalf("delete governed document error = %v, want pull request required for repository %q", err, repository.ID)
	}

	assertBoundary := func(name string, err error) {
		t.Helper()
		if !errors.Is(err, repo.ErrRepositoryBoundary) {
			t.Fatalf("%s error = %v, want %v", name, err, repo.ErrRepositoryBoundary)
		}
	}
	_, err = store.CreateFolder(ctx, owner, "Blocked child", child.ID)
	assertBoundary("create folder", err)
	_, err = store.UpdateFolder(ctx, child.ID, "Blocked rename", nil)
	assertBoundary("rename folder", err)
	assertBoundary("delete folder", store.DeleteFolder(ctx, child.ID, owner))
	assertBoundary("move document to root", store.MoveDocumentToRoot(ctx, child.ID, document.ID))

	looseFolder, err := store.CreateFolder(ctx, owner, "Loose", "")
	if err != nil {
		t.Fatal(err)
	}
	looseDocument, _, err := store.CreateDocument(ctx, owner, "Loose document", []byte("<html>loose</html>"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.MoveDocumentToFolder(ctx, owner, child.ID, looseDocument.ID)
	assertBoundary("move loose document into repository", err)
	_, err = store.MoveDocumentToFolder(ctx, owner, looseFolder.ID, document.ID)
	assertBoundary("move repository document out", err)

	got, err := store.GetRepository(ctx, repository.ID)
	if err != nil || got.ID != repository.ID {
		t.Fatalf("repository after rejected mutations = %+v, err = %v", got, err)
	}
	governed, err := store.GetDocument(ctx, document.ID)
	if err != nil || governed.ParentFolderID != child.ID {
		t.Fatalf("governed document after rejected mutations = %+v, err = %v", governed, err)
	}
	if err := store.DeleteDocument(ctx, looseDocument.ID); err != nil {
		t.Fatalf("delete loose document: %v", err)
	}
	if err := store.DeleteDocument(ctx, looseDocument.ID); err != nil {
		t.Fatalf("delete loose document again: %v", err)
	}
}
