package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/razorpay/aidocs/api/internal/auth"
	"github.com/razorpay/aidocs/api/internal/db"
	"github.com/razorpay/aidocs/api/internal/repo"
	pgrepo "github.com/razorpay/aidocs/api/internal/repo/postgres"
)

func TestPostgresTransferRepositoryOwnership(t *testing.T) {
	url := os.Getenv("AIDOCS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set AIDOCS_TEST_DATABASE_URL to run postgres integration tests")
	}
	if err := db.Migrate(url); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	store, err := pgrepo.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	owner := auth.Principal{Type: auth.PrincipalUser, ID: "usr_transfer_owner_" + suffix, Email: "transfer-owner-" + suffix + "@example.com", Name: "Original Owner"}
	target := auth.Principal{Type: auth.PrincipalUser, ID: "usr_transfer_target_" + suffix, Email: "transfer-target-" + suffix + "@example.com", Name: "Target Owner"}
	viewer := auth.Principal{Type: auth.PrincipalUser, ID: "usr_transfer_viewer_" + suffix, Email: "transfer-viewer-" + suffix + "@example.com", Name: "Viewer"}
	for _, principal := range []auth.Principal{owner, target, viewer} {
		if _, err := store.UpsertGoogleUser(ctx, principal.ID, principal.Email, principal.Name, "sub-"+principal.ID, ""); err != nil {
			t.Fatal(err)
		}
	}

	root, err := store.CreateFolder(ctx, owner, "Transfer root "+suffix, "")
	if err != nil {
		t.Fatal(err)
	}
	nested, err := store.CreateFolder(ctx, owner, "Nested", root.ID)
	if err != nil {
		t.Fatal(err)
	}
	document, version, err := store.CreateDocument(ctx, owner, "Transfer document "+suffix, []byte("<p>history</p>"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.MoveDocumentToFolder(ctx, owner, nested.ID, document.ID); err != nil {
		t.Fatal(err)
	}
	grant, err := store.CreateGrant(ctx, "document", document.ID, viewer, repo.RoleViewer, owner)
	if err != nil {
		t.Fatal(err)
	}
	repository, err := store.CreateRepository(ctx, owner, repo.CreateRepositoryParams{
		Name: "Transfer repository " + suffix, Slug: "transfer-repository-" + suffix,
		Scope: repo.RepositoryScopePersonal, NamespaceID: owner.ID, NamespaceName: owner.Name,
		RootFolderID: root.ID, RequiredApprovals: 1, Protected: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.TransferRepositoryOwnership(ctx, repository.ID, viewer, target); !errors.Is(err, repo.ErrForbidden) {
		t.Fatalf("wrong-owner transfer error = %v, want forbidden", err)
	}
	transferred, err := store.TransferRepositoryOwnership(ctx, repository.ID, owner, target)
	if err != nil {
		t.Fatal(err)
	}
	if transferred.CreatedBy.ID != target.ID || transferred.NamespaceID != target.ID || transferred.NamespaceName != target.Name {
		t.Fatalf("transferred repository = %+v", transferred)
	}
	for _, folderID := range []string{root.ID, nested.ID} {
		folder, err := store.GetFolder(ctx, folderID)
		if err != nil || folder.Owner.ID != target.ID {
			t.Fatalf("folder %q = %+v, err = %v", folderID, folder, err)
		}
	}
	transferredDocument, err := store.GetDocument(ctx, document.ID)
	if err != nil || transferredDocument.Owner.ID != target.ID {
		t.Fatalf("document = %+v, err = %v", transferredDocument, err)
	}
	historicalVersion, err := store.GetVersion(ctx, version.ID)
	if err != nil || historicalVersion.CreatedBy.ID != owner.ID {
		t.Fatalf("history = %+v, err = %v", historicalVersion, err)
	}
	grants, err := store.ListGrants(ctx, document.ID)
	if err != nil || len(grants) != 1 || grants[0].ID != grant.ID || grants[0].GrantedBy.ID != owner.ID {
		t.Fatalf("grants after transfer = %+v, err = %v", grants, err)
	}

	conflictSourceDocument, _, err := store.CreateDocument(ctx, owner, "Conflict source "+suffix, []byte("<p>source</p>"))
	if err != nil {
		t.Fatal(err)
	}
	conflictSource, err := store.CreateRepository(ctx, owner, repo.CreateRepositoryParams{
		Name: "Conflict source " + suffix, Slug: "conflicting-slug-" + suffix,
		Scope: repo.RepositoryScopePersonal, NamespaceID: owner.ID, NamespaceName: owner.Name,
		DocumentID: conflictSourceDocument.ID, RequiredApprovals: 1, Protected: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	conflictTargetDocument, _, err := store.CreateDocument(ctx, target, "Conflict target "+suffix, []byte("<p>target</p>"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateRepository(ctx, target, repo.CreateRepositoryParams{
		Name: "Conflict target " + suffix, Slug: conflictSource.Slug,
		Scope: repo.RepositoryScopePersonal, NamespaceID: target.ID, NamespaceName: target.Name,
		DocumentID: conflictTargetDocument.ID, RequiredApprovals: 1, Protected: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.TransferRepositoryOwnership(ctx, conflictSource.ID, owner, target); !errors.Is(err, repo.ErrConflict) {
		t.Fatalf("slug-conflict transfer error = %v, want conflict", err)
	}
	unchanged, _ := store.GetRepository(ctx, conflictSource.ID)
	unchangedDocument, _ := store.GetDocument(ctx, conflictSourceDocument.ID)
	if unchanged.CreatedBy.ID != owner.ID || unchanged.NamespaceID != owner.ID || unchangedDocument.Owner.ID != owner.ID {
		t.Fatalf("conflict partially mutated repository=%+v document=%+v", unchanged, unchangedDocument)
	}

	organization, err := store.CreateRepository(ctx, owner, repo.CreateRepositoryParams{
		Name: "Organization " + suffix, Slug: "organization-" + suffix,
		Scope: repo.RepositoryScopeOrganization, NamespaceID: "razorpay", NamespaceName: "Razorpay",
		RequiredApprovals: 1, Protected: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	organization, err = store.TransferRepositoryOwnership(ctx, organization.ID, owner, target)
	if err != nil {
		t.Fatal(err)
	}
	if organization.CreatedBy.ID != target.ID || organization.NamespaceID != "razorpay" || organization.NamespaceName != "Razorpay" {
		t.Fatalf("organization transfer = %+v", organization)
	}
}
