package repo

import (
	"context"
	"errors"
	"testing"

	"github.com/razorpay/aidocs/api/internal/auth"
)

func TestMemoryCreateEmptyRepositoryCreatesRootFolder(t *testing.T) {
	ctx := context.Background()
	store := NewMemory()
	owner := auth.Principal{Type: auth.PrincipalUser, ID: "empty_owner", Email: "empty@example.com", Name: "Empty Owner"}
	repository, err := store.CreateRepository(ctx, owner, CreateRepositoryParams{
		Name:              "Empty handbook",
		Slug:              "empty-handbook",
		Scope:             RepositoryScopePersonal,
		NamespaceID:       owner.ID,
		NamespaceName:     owner.Name,
		RequiredApprovals: 1,
		Protected:         true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if repository.RootFolderID == "" {
		t.Fatalf("repository = %+v, want generated root folder", repository)
	}
	root, err := store.GetFolder(ctx, repository.RootFolderID)
	if err != nil {
		t.Fatal(err)
	}
	if root.Title != repository.Name || root.Owner.ID != owner.ID {
		t.Fatalf("root = %+v, want repository name and owner", root)
	}
	documents, err := store.ListRepositoryDocuments(ctx, owner, repository.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(documents) != 0 {
		t.Fatalf("empty repository documents = %+v", documents)
	}
	viewer := auth.Principal{Type: auth.PrincipalUser, ID: "empty_viewer", Email: "viewer@example.com", Name: "Viewer"}
	if _, _, err := store.CreateRepositoryDocument(ctx, repository.ID, viewer, "Forbidden", []byte("<html></html>")); err != ErrForbidden {
		t.Fatalf("viewer bootstrap error = %v, want %v", err, ErrForbidden)
	}
	document, version, err := store.CreateRepositoryDocument(ctx, repository.ID, owner, "Getting started", []byte("<html><body>start</body></html>"))
	if err != nil {
		t.Fatal(err)
	}
	if document.ParentFolderID != repository.RootFolderID || document.CurrentVersionID != version.ID || version.Number != 1 {
		t.Fatalf("bootstrap document = %+v, version = %+v", document, version)
	}
	if _, _, err := store.CreateRepositoryDocument(ctx, repository.ID, owner, "Second", []byte("<html></html>")); err != ErrRepositoryNotEmpty {
		t.Fatalf("second bootstrap error = %v, want %v", err, ErrRepositoryNotEmpty)
	}
	if _, err := store.CreateVersion(ctx, document.ID, version.ID, "direct", []byte("<html>changed</html>"), owner); err == nil || !errors.Is(err, ErrPullRequestRequired) {
		t.Fatalf("post-bootstrap direct version error = %v, want %v", err, ErrPullRequestRequired)
	}
}

func TestMemoryOrganizationRepositoryIsImplicitlyVisible(t *testing.T) {
	ctx := context.Background()
	store := NewMemory()
	owner := auth.Principal{Type: auth.PrincipalUser, ID: "org_owner", Email: "owner@example.com", Name: "Owner"}
	viewer := auth.Principal{Type: auth.PrincipalUser, ID: "org_viewer", Email: "viewer@example.com", Name: "Viewer"}
	document, _, err := store.CreateDocument(ctx, owner, "Organization handbook", []byte("<html><body>org</body></html>"))
	if err != nil {
		t.Fatal(err)
	}
	repository, err := store.CreateRepository(ctx, owner, CreateRepositoryParams{
		Name:              "Organization handbook",
		Slug:              "organization-handbook",
		Scope:             RepositoryScopeOrganization,
		NamespaceID:       "acme",
		NamespaceName:     "Acme",
		DocumentID:        document.ID,
		RequiredApprovals: 1,
		Protected:         true,
		// Deliberately false to model organization repositories created before
		// explicit anyone grants were added.
		GrantOrgViewer: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	role, err := store.RoleForRepository(ctx, viewer, repository.ID)
	if err != nil || role != RoleViewer {
		t.Fatalf("repository role = %q, err = %v; want viewer", role, err)
	}
	role, err = store.RoleForDocument(ctx, viewer, document.ID)
	if err != nil || role != RoleViewer {
		t.Fatalf("document role = %q, err = %v; want viewer", role, err)
	}
	role, err = store.RoleForFolder(ctx, viewer, repository.RootFolderID)
	if err != nil || role != RoleViewer {
		t.Fatalf("folder role = %q, err = %v; want viewer", role, err)
	}
	repositories, err := store.ListRepositories(ctx, viewer)
	if err != nil {
		t.Fatal(err)
	}
	if len(repositories) != 1 || repositories[0].ID != repository.ID {
		t.Fatalf("viewer repositories = %+v, want %q", repositories, repository.ID)
	}
	documents, err := store.ListRepositoryDocuments(ctx, viewer, repository.ID)
	if err != nil || len(documents) != 1 || documents[0].ID != document.ID {
		t.Fatalf("viewer documents = %+v, err = %v; want %q", documents, err, document.ID)
	}
}

func TestMemoryDeleteRepositoryIsOwnerOnlyAndCleansBoundary(t *testing.T) {
	ctx := context.Background()
	store := NewMemory()
	owner := auth.Principal{Type: auth.PrincipalUser, ID: "delete_owner", Email: "delete-owner@example.com", Name: "Delete Owner"}
	viewer := auth.Principal{Type: auth.PrincipalUser, ID: "delete_viewer", Email: "delete-viewer@example.com", Name: "Delete Viewer"}

	root, err := store.CreateFolder(ctx, owner, "Repository root", "")
	if err != nil {
		t.Fatal(err)
	}
	nested, err := store.CreateFolder(ctx, owner, "Nested", root.ID)
	if err != nil {
		t.Fatal(err)
	}
	document, baseVersion, err := store.CreateDocument(ctx, owner, "Repository document", []byte("<html><body>base</body></html>"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.MoveDocumentToFolder(ctx, owner, nested.ID, document.ID); err != nil {
		t.Fatal(err)
	}
	repository, err := store.CreateRepository(ctx, owner, CreateRepositoryParams{
		Name:              "Delete fixture",
		Slug:              "delete-fixture",
		Scope:             RepositoryScopePersonal,
		NamespaceID:       owner.ID,
		NamespaceName:     owner.Name,
		RootFolderID:      root.ID,
		RequiredApprovals: 1,
		Protected:         true,
	})
	if err != nil {
		t.Fatal(err)
	}
	pullRequest, err := store.CreatePullRequest(ctx, repository.ID, document.ID, baseVersion.ID, "Proposed update", "", "Update", []byte("<html><body>head</body></html>"), owner)
	if err != nil {
		t.Fatal(err)
	}
	comment, err := store.CreateComment(ctx, document.ID, baseVersion.ID, "Comment", Anchor{Quote: "base"}, viewer, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateGrant(ctx, "folder", root.ID, viewer, RoleEditor, owner); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateGrant(ctx, "document", document.ID, viewer, RoleViewer, owner); err != nil {
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
	store.kv[document.ID] = map[string]DocumentKVChange{"status": {Key: "status", Revision: 1}}
	store.kvRev[document.ID] = 1
	store.kvEvents[document.ID] = []DocumentKVEvent{{DocumentID: document.ID, Revision: 1}}

	if err := store.DeleteRepository(ctx, repository.ID, viewer); !errors.Is(err, ErrForbidden) {
		t.Fatalf("viewer delete error = %v, want %v", err, ErrForbidden)
	}
	if _, err := store.GetRepository(ctx, repository.ID); err != nil {
		t.Fatalf("repository disappeared after forbidden delete: %v", err)
	}
	if err := store.DeleteRepository(ctx, repository.ID, owner); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteRepository(ctx, repository.ID, owner); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete error = %v, want %v", err, ErrNotFound)
	}

	if _, ok := store.repos[repository.ID]; ok {
		t.Fatalf("repository %q remains", repository.ID)
	}
	for _, folderID := range []string{root.ID, nested.ID} {
		if _, ok := store.folders[folderID]; ok {
			t.Fatalf("folder %q remains", folderID)
		}
		if _, ok := store.folderWorkspace[folderID]; ok {
			t.Fatalf("folder workspace entry %q remains", folderID)
		}
	}
	if _, ok := store.docs[document.ID]; ok {
		t.Fatalf("document %q remains", document.ID)
	}
	for _, versionID := range []string{baseVersion.ID, pullRequest.HeadVersionID} {
		if _, ok := store.versions[versionID]; ok {
			t.Fatalf("version %q remains", versionID)
		}
		if _, ok := store.versionHTML[versionID]; ok {
			t.Fatalf("version HTML %q remains", versionID)
		}
	}
	if _, ok := store.prs[pullRequest.ID]; ok {
		t.Fatalf("pull request %q remains", pullRequest.ID)
	}
	if _, ok := store.comments[comment.ID]; ok {
		t.Fatalf("comment %q remains", comment.ID)
	}
	if _, ok := store.kv[document.ID]; ok || store.kvRev[document.ID] != 0 || len(store.kvEvents[document.ID]) != 0 {
		t.Fatalf("document metadata remains for %q", document.ID)
	}
	if _, ok := store.docWorkspace[document.ID]; ok {
		t.Fatalf("document workspace entry %q remains", document.ID)
	}
	for _, pin := range store.pins {
		if pin.ResourceID == root.ID || pin.ResourceID == nested.ID || pin.ResourceID == document.ID {
			t.Fatalf("pin remains for deleted resource: %+v", pin)
		}
	}
	for _, grant := range store.grants {
		if grant.ResourceID == root.ID || grant.ResourceID == nested.ID || grant.ResourceID == document.ID {
			t.Fatalf("grant remains for deleted resource: %+v", grant)
		}
	}
}

func newMemoryMultiDocumentRepository(t *testing.T) (*Memory, RepositoryRecord, auth.Principal, auth.Principal, Document, Version, Document, Version) {
	t.Helper()
	ctx := context.Background()
	store := NewMemory()
	owner := auth.Principal{Type: auth.PrincipalUser, ID: "multi_owner", Email: "multi-owner@example.com", Name: "Owner"}
	reviewer := auth.Principal{Type: auth.PrincipalUser, ID: "multi_reviewer", Email: "multi-reviewer@example.com", Name: "Reviewer"}
	if _, err := store.UpsertGoogleUser(ctx, owner.ID, owner.Email, owner.Name, "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertGoogleUser(ctx, reviewer.ID, reviewer.Email, reviewer.Name, "", ""); err != nil {
		t.Fatal(err)
	}
	root, err := store.CreateFolder(ctx, owner, "Multi-document root", "")
	if err != nil {
		t.Fatal(err)
	}
	first, firstVersion, err := store.CreateDocument(ctx, owner, "First", []byte("<p>first base</p>"))
	if err != nil {
		t.Fatal(err)
	}
	second, secondVersion, err := store.CreateDocument(ctx, owner, "Second", []byte("<p>second base</p>"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.MoveDocumentToFolder(ctx, owner, root.ID, first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.MoveDocumentToFolder(ctx, owner, root.ID, second.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateGrant(ctx, "folder", root.ID, reviewer, RoleEditor, owner); err != nil {
		t.Fatal(err)
	}
	repository, err := store.CreateRepository(ctx, owner, CreateRepositoryParams{
		Name: "Multi-document", Slug: "multi-document", Scope: RepositoryScopePersonal,
		NamespaceID: owner.ID, NamespaceName: owner.Name, RootFolderID: root.ID,
		RequiredApprovals: 1, Protected: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return store, repository, owner, reviewer, first, firstVersion, second, secondVersion
}

func TestMemoryMultiDocumentPullRequestMergesAtomically(t *testing.T) {
	ctx := context.Background()
	store, repository, owner, reviewer, first, firstBase, second, secondBase := newMemoryMultiDocumentRepository(t)
	pullRequest, err := store.CreatePullRequestChangeSet(ctx, repository.ID, "Update both", "", "two updates", []PullRequestChangeInput{
		{Operation: PullRequestChangeUpdate, DocumentID: first.ID, BaseVersionID: firstBase.ID, HTML: []byte("<p>first head</p>")},
		{Operation: PullRequestChangeUpdate, DocumentID: second.ID, BaseVersionID: secondBase.ID, HTML: []byte("<p>second head</p>")},
	}, owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(pullRequest.Changes) != 2 || pullRequest.Patchset != 1 {
		t.Fatalf("pull request = %+v", pullRequest)
	}
	if _, err := store.ReviewPullRequest(ctx, repository.ID, pullRequest.Number, reviewer, ReviewStateApproved, "both look good"); err != nil {
		t.Fatal(err)
	}
	merged, err := store.MergePullRequest(ctx, repository.ID, pullRequest.Number, reviewer)
	if err != nil {
		t.Fatal(err)
	}
	if merged.Status != PullRequestStatusMerged {
		t.Fatalf("merged status = %q", merged.Status)
	}
	firstAfter, _ := store.GetDocument(ctx, first.ID)
	secondAfter, _ := store.GetDocument(ctx, second.ID)
	if firstAfter.CurrentVersionID != pullRequest.Changes[0].HeadVersionID || secondAfter.CurrentVersionID != pullRequest.Changes[1].HeadVersionID {
		t.Fatalf("heads after merge = %q, %q; changes = %+v", firstAfter.CurrentVersionID, secondAfter.CurrentVersionID, pullRequest.Changes)
	}
}

func TestMemoryMultiDocumentPullRequestRollsBackBeforeAnyChange(t *testing.T) {
	ctx := context.Background()
	store, repository, owner, reviewer, first, firstBase, second, secondBase := newMemoryMultiDocumentRepository(t)
	pullRequest, err := store.CreatePullRequestChangeSet(ctx, repository.ID, "Update both", "", "two updates", []PullRequestChangeInput{
		{Operation: PullRequestChangeUpdate, DocumentID: first.ID, BaseVersionID: firstBase.ID, HTML: []byte("<p>first head</p>")},
		{Operation: PullRequestChangeUpdate, DocumentID: second.ID, BaseVersionID: secondBase.ID, HTML: []byte("<p>second head</p>")},
	}, owner)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate loss of the second proposed head. Merge must validate the entire
	// patchset before publishing the first head.
	delete(store.versions, pullRequest.Changes[1].HeadVersionID)
	delete(store.versionHTML, pullRequest.Changes[1].HeadVersionID)
	if _, err := store.ReviewPullRequest(ctx, repository.ID, pullRequest.Number, reviewer, ReviewStateApproved, "approved"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.MergePullRequest(ctx, repository.ID, pullRequest.Number, reviewer); !errors.Is(err, ErrStalePullRequest) {
		t.Fatalf("merge error = %v, want stale", err)
	}
	firstAfter, _ := store.GetDocument(ctx, first.ID)
	secondAfter, _ := store.GetDocument(ctx, second.ID)
	if firstAfter.CurrentVersionID != firstBase.ID || secondAfter.CurrentVersionID != secondBase.ID {
		t.Fatalf("partial merge occurred: first=%q second=%q", firstAfter.CurrentVersionID, secondAfter.CurrentVersionID)
	}
}

func TestMemoryPatchsetDismissesPreviousApproval(t *testing.T) {
	ctx := context.Background()
	store, repository, owner, reviewer, first, firstBase, _, _ := newMemoryMultiDocumentRepository(t)
	pullRequest, err := store.CreatePullRequest(ctx, repository.ID, first.ID, firstBase.ID, "Edit", "", "first patch", []byte("<p>one</p>"), owner)
	if err != nil {
		t.Fatal(err)
	}
	approved, err := store.ReviewPullRequest(ctx, repository.ID, pullRequest.Number, reviewer, ReviewStateApproved, "approved")
	if err != nil || !approved.Mergeable {
		t.Fatalf("approved = %+v, err = %v", approved, err)
	}
	patched, err := store.CreatePullRequestPatchset(ctx, repository.ID, pullRequest.Number, "second patch", []PullRequestChangeInput{{
		Operation: PullRequestChangeUpdate, DocumentID: first.ID, BaseVersionID: firstBase.ID, HTML: []byte("<p>two</p>"),
	}}, owner)
	if err != nil {
		t.Fatal(err)
	}
	if patched.Patchset != 2 || patched.Mergeable || patched.MergeBlockedReason != ErrApprovalRequired.Error() {
		t.Fatalf("patched pull request = %+v", patched)
	}
	if len(patched.Reviews) != 1 || patched.Reviews[0].Patchset != 1 {
		t.Fatalf("old review was not retained as dismissed history: %+v", patched.Reviews)
	}
}

func TestMemoryStalePullRequestCanBeRebased(t *testing.T) {
	ctx := context.Background()
	store, repository, owner, reviewer, first, firstBase, _, _ := newMemoryMultiDocumentRepository(t)
	stale, err := store.CreatePullRequest(ctx, repository.ID, first.ID, firstBase.ID, "Stale", "", "stale", []byte("<p>stale proposal</p>"), owner)
	if err != nil {
		t.Fatal(err)
	}
	winner, err := store.CreatePullRequest(ctx, repository.ID, first.ID, firstBase.ID, "Winner", "", "winner", []byte("<p>winner</p>"), owner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReviewPullRequest(ctx, repository.ID, winner.Number, reviewer, ReviewStateApproved, "approved"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.MergePullRequest(ctx, repository.ID, winner.Number, reviewer); err != nil {
		t.Fatal(err)
	}
	staleNow, err := store.GetPullRequest(ctx, repository.ID, stale.Number)
	if err != nil || staleNow.MergeBlockedReason != ErrStalePullRequest.Error() {
		t.Fatalf("stale pull request = %+v, err = %v", staleNow, err)
	}
	rebased, err := store.RebasePullRequest(ctx, repository.ID, stale.Number, owner)
	if err != nil {
		t.Fatal(err)
	}
	if rebased.Patchset != 2 || rebased.Changes[0].BaseVersionID != winner.HeadVersionID || rebased.MergeBlockedReason != ErrApprovalRequired.Error() {
		t.Fatalf("rebased pull request = %+v, winner = %+v", rebased, winner)
	}
}

func TestMemoryPullRequestCanRenameDocument(t *testing.T) {
	ctx := context.Background()
	store, repository, owner, reviewer, first, firstBase, _, _ := newMemoryMultiDocumentRepository(t)
	pullRequest, err := store.CreatePullRequestChangeSet(ctx, repository.ID, "Rename first", "", "rename", []PullRequestChangeInput{{
		Operation: PullRequestChangeRename, DocumentID: first.ID, BaseVersionID: firstBase.ID, Title: "First renamed",
	}}, owner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReviewPullRequest(ctx, repository.ID, pullRequest.Number, reviewer, ReviewStateApproved, "approved"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.MergePullRequest(ctx, repository.ID, pullRequest.Number, reviewer); err != nil {
		t.Fatal(err)
	}
	renamed, err := store.GetDocument(ctx, first.ID)
	if err != nil || renamed.Title != "First renamed" || renamed.CurrentVersionID != firstBase.ID {
		t.Fatalf("renamed = %+v, err = %v", renamed, err)
	}
}

func TestMemoryTransferRepositoryOwnershipMovesEntireTreeAndPreservesHistory(t *testing.T) {
	ctx := context.Background()
	store := NewMemory()
	owner := auth.Principal{Type: auth.PrincipalUser, ID: "transfer_owner", Email: "owner@example.com", Name: "Original Owner"}
	target := auth.Principal{Type: auth.PrincipalUser, ID: "transfer_target", Email: "target@example.com", Name: "Target Owner"}
	viewer := auth.Principal{Type: auth.PrincipalUser, ID: "transfer_viewer", Email: "viewer@example.com", Name: "Viewer"}
	for _, principal := range []auth.Principal{owner, target, viewer} {
		if _, err := store.UpsertGoogleUser(ctx, principal.ID, principal.Email, principal.Name, "sub-"+principal.ID, ""); err != nil {
			t.Fatal(err)
		}
	}
	root, err := store.CreateFolder(ctx, owner, "Transfer root", "")
	if err != nil {
		t.Fatal(err)
	}
	nested, err := store.CreateFolder(ctx, owner, "Nested", root.ID)
	if err != nil {
		t.Fatal(err)
	}
	document, version, err := store.CreateDocument(ctx, owner, "Owned document", []byte("<p>history</p>"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.MoveDocumentToFolder(ctx, owner, nested.ID, document.ID); err != nil {
		t.Fatal(err)
	}
	grant, err := store.CreateGrant(ctx, "document", document.ID, viewer, RoleViewer, owner)
	if err != nil {
		t.Fatal(err)
	}
	repository, err := store.CreateRepository(ctx, owner, CreateRepositoryParams{
		Name: "Transfer repository", Slug: "transfer-repository", Scope: RepositoryScopePersonal,
		NamespaceID: owner.ID, NamespaceName: owner.Name, RootFolderID: root.ID,
		RequiredApprovals: 1, Protected: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.TransferRepositoryOwnership(ctx, repository.ID, viewer, target); !errors.Is(err, ErrForbidden) {
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
	ownedDocument, err := store.GetDocument(ctx, document.ID)
	if err != nil || ownedDocument.Owner.ID != target.ID {
		t.Fatalf("document = %+v, err = %v", ownedDocument, err)
	}
	historicalVersion, err := store.GetVersion(ctx, version.ID)
	if err != nil || historicalVersion.CreatedBy.ID != owner.ID {
		t.Fatalf("historical version = %+v, err = %v", historicalVersion, err)
	}
	grants, err := store.ListGrants(ctx, document.ID)
	if err != nil || len(grants) != 1 || grants[0].ID != grant.ID || grants[0].GrantedBy.ID != owner.ID {
		t.Fatalf("grants after transfer = %+v, err = %v", grants, err)
	}
	role, err := store.RoleForRepository(ctx, target, repository.ID)
	if err != nil || role != RoleOwner {
		t.Fatalf("target role = %q, err = %v", role, err)
	}
}

func TestMemoryTransferRepositoryOwnershipRejectsPersonalSlugConflictAtomically(t *testing.T) {
	ctx := context.Background()
	store := NewMemory()
	owner := auth.Principal{Type: auth.PrincipalUser, ID: "conflict_owner", Email: "conflict-owner@example.com", Name: "Owner"}
	target := auth.Principal{Type: auth.PrincipalUser, ID: "conflict_target", Email: "conflict-target@example.com", Name: "Target"}
	create := func(principal auth.Principal, title string) (RepositoryRecord, Document) {
		document, _, err := store.CreateDocument(ctx, principal, title+" doc", []byte("<p>doc</p>"))
		if err != nil {
			t.Fatal(err)
		}
		repository, err := store.CreateRepository(ctx, principal, CreateRepositoryParams{
			Name: title, Slug: "same-slug", Scope: RepositoryScopePersonal,
			NamespaceID: principal.ID, NamespaceName: principal.Name, DocumentID: document.ID,
			RequiredApprovals: 1, Protected: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		return repository, document
	}
	source, sourceDocument := create(owner, "Source")
	_, _ = create(target, "Target")
	if _, err := store.TransferRepositoryOwnership(ctx, source.ID, owner, target); !errors.Is(err, ErrConflict) {
		t.Fatalf("transfer error = %v, want conflict", err)
	}
	unchanged, _ := store.GetRepository(ctx, source.ID)
	unchangedDocument, _ := store.GetDocument(ctx, sourceDocument.ID)
	if unchanged.CreatedBy.ID != owner.ID || unchanged.NamespaceID != owner.ID || unchangedDocument.Owner.ID != owner.ID {
		t.Fatalf("conflicting transfer partially mutated repository=%+v document=%+v", unchanged, unchangedDocument)
	}
}

func TestMemoryTransferOrganizationRepositoryKeepsNamespace(t *testing.T) {
	ctx := context.Background()
	store := NewMemory()
	owner := auth.Principal{Type: auth.PrincipalUser, ID: "org_transfer_owner", Email: "owner@example.com", Name: "Owner"}
	target := auth.Principal{Type: auth.PrincipalUser, ID: "org_transfer_target", Email: "target@example.com", Name: "Target"}
	repository, err := store.CreateRepository(ctx, owner, CreateRepositoryParams{
		Name: "Org repository", Slug: "org-repository", Scope: RepositoryScopeOrganization,
		NamespaceID: "razorpay", NamespaceName: "Razorpay", RequiredApprovals: 1, Protected: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	transferred, err := store.TransferRepositoryOwnership(ctx, repository.ID, owner, target)
	if err != nil {
		t.Fatal(err)
	}
	if transferred.CreatedBy.ID != target.ID || transferred.NamespaceID != "razorpay" || transferred.NamespaceName != "Razorpay" {
		t.Fatalf("organization transfer = %+v", transferred)
	}
}
