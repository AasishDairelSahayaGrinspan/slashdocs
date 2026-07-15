package postgres_test

import (
	"context"
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

func TestPostgresMultiDocumentPullRequestMergeAndRename(t *testing.T) {
	url := os.Getenv("AIDOCS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set AIDOCS_TEST_DATABASE_URL to run postgres integration tests")
	}
	if err := db.Migrate(url); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	store, err := pgrepo.ConnectWithBlob(ctx, url, blob.NewMemory())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	owner := auth.Principal{Type: auth.PrincipalUser, ID: "usr_multi_owner_" + suffix, Email: "multi-owner-" + suffix + "@example.com", Name: "Owner"}
	reviewer, err := store.EnsureUserByEmail(ctx, "multi-reviewer-"+suffix+"@example.com")
	if err != nil {
		t.Fatal(err)
	}
	root, err := store.CreateFolder(ctx, owner, "Multi root "+suffix, "")
	if err != nil {
		t.Fatal(err)
	}
	first, firstBase, err := store.CreateDocument(ctx, owner, "First "+suffix, []byte("<p>first base</p>"))
	if err != nil {
		t.Fatal(err)
	}
	second, secondBase, err := store.CreateDocument(ctx, owner, "Second "+suffix, []byte("<p>second base</p>"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.MoveDocumentToFolder(ctx, owner, root.ID, first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.MoveDocumentToFolder(ctx, owner, root.ID, second.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateGrant(ctx, "folder", root.ID, reviewer, repo.RoleEditor, owner); err != nil {
		t.Fatal(err)
	}
	repository, err := store.CreateRepository(ctx, owner, repo.CreateRepositoryParams{
		Name: "Multi " + suffix, Slug: "multi-" + suffix, Scope: repo.RepositoryScopePersonal,
		NamespaceID: owner.ID, NamespaceName: owner.Name, RootFolderID: root.ID,
		RequiredApprovals: 1, Protected: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	pullRequest, err := store.CreatePullRequestChangeSet(ctx, repository.ID, "Update both", "", "multi", []repo.PullRequestChangeInput{
		{Operation: repo.PullRequestChangeUpdate, DocumentID: first.ID, BaseVersionID: firstBase.ID, HTML: []byte("<p>first head</p>")},
		{Operation: repo.PullRequestChangeUpdate, DocumentID: second.ID, BaseVersionID: secondBase.ID, HTML: []byte("<p>second head</p>")},
	}, owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(pullRequest.Changes) != 2 || pullRequest.Patchset != 1 {
		t.Fatalf("pull request = %+v", pullRequest)
	}
	if _, err := store.ReviewPullRequest(ctx, repository.ID, pullRequest.Number, reviewer, repo.ReviewStateApproved, "approved"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.MergePullRequest(ctx, repository.ID, pullRequest.Number, reviewer); err != nil {
		t.Fatal(err)
	}
	firstAfter, err := store.GetDocument(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	secondAfter, err := store.GetDocument(ctx, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if firstAfter.CurrentVersionID != pullRequest.Changes[0].HeadVersionID || secondAfter.CurrentVersionID != pullRequest.Changes[1].HeadVersionID {
		t.Fatalf("merged heads = %q, %q; changes = %+v", firstAfter.CurrentVersionID, secondAfter.CurrentVersionID, pullRequest.Changes)
	}
	rename, err := store.CreatePullRequestChangeSet(ctx, repository.ID, "Rename", "", "rename", []repo.PullRequestChangeInput{{
		Operation: repo.PullRequestChangeRename, DocumentID: first.ID, BaseVersionID: firstAfter.CurrentVersionID, Title: "Renamed " + suffix,
	}}, owner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReviewPullRequest(ctx, repository.ID, rename.Number, reviewer, repo.ReviewStateApproved, "approved"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.MergePullRequest(ctx, repository.ID, rename.Number, reviewer); err != nil {
		t.Fatal(err)
	}
	renamed, err := store.GetDocument(ctx, first.ID)
	if err != nil || renamed.Title != "Renamed "+suffix {
		t.Fatalf("renamed = %+v, err = %v", renamed, err)
	}

	patchsetPR, err := store.CreatePullRequest(ctx, repository.ID, second.ID, secondAfter.CurrentVersionID, "Patchsets", "", "one", []byte("<p>patch one</p>"), owner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReviewPullRequest(ctx, repository.ID, patchsetPR.Number, reviewer, repo.ReviewStateApproved, "approved"); err != nil {
		t.Fatal(err)
	}
	patched, err := store.CreatePullRequestPatchset(ctx, repository.ID, patchsetPR.Number, "two", []repo.PullRequestChangeInput{{
		Operation: repo.PullRequestChangeUpdate, DocumentID: second.ID, BaseVersionID: secondAfter.CurrentVersionID, HTML: []byte("<p>patch two</p>"),
	}}, owner)
	if err != nil {
		t.Fatal(err)
	}
	if patched.Patchset != 2 || patched.Mergeable || patched.MergeBlockedReason != repo.ErrApprovalRequired.Error() || len(patched.Reviews) != 1 || patched.Reviews[0].Patchset != 1 {
		t.Fatalf("patched = %+v", patched)
	}

	stale, err := store.CreatePullRequest(ctx, repository.ID, first.ID, renamed.CurrentVersionID, "Stale", "", "stale", []byte("<p>stale</p>"), owner)
	if err != nil {
		t.Fatal(err)
	}
	winner, err := store.CreatePullRequest(ctx, repository.ID, first.ID, renamed.CurrentVersionID, "Winner", "", "winner", []byte("<p>winner</p>"), owner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReviewPullRequest(ctx, repository.ID, winner.Number, reviewer, repo.ReviewStateApproved, "approved"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.MergePullRequest(ctx, repository.ID, winner.Number, reviewer); err != nil {
		t.Fatal(err)
	}
	rebased, err := store.RebasePullRequest(ctx, repository.ID, stale.Number, owner)
	if err != nil {
		t.Fatal(err)
	}
	if rebased.Patchset != 2 || len(rebased.Changes) != 1 || rebased.Changes[0].BaseVersionID != winner.HeadVersionID || rebased.MergeBlockedReason != repo.ErrApprovalRequired.Error() {
		t.Fatalf("rebased = %+v; winner = %+v", rebased, winner)
	}

	createdPR, err := store.CreatePullRequestChangeSet(ctx, repository.ID, "Create document", "", "create", []repo.PullRequestChangeInput{{
		Operation: repo.PullRequestChangeCreate, FolderID: repository.RootFolderID, Title: "Created " + suffix, HTML: []byte("<p>created</p>"),
	}}, owner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReviewPullRequest(ctx, repository.ID, createdPR.Number, reviewer, repo.ReviewStateApproved, "approved"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.MergePullRequest(ctx, repository.ID, createdPR.Number, reviewer); err != nil {
		t.Fatal(err)
	}
	createdDocument, err := store.GetDocument(ctx, createdPR.Changes[0].DocumentID)
	if err != nil || createdDocument.CurrentVersionID != createdPR.Changes[0].HeadVersionID {
		t.Fatalf("created document = %+v, err = %v; pull request = %+v", createdDocument, err, createdPR)
	}
	deletePR, err := store.CreatePullRequestChangeSet(ctx, repository.ID, "Delete document", "", "delete", []repo.PullRequestChangeInput{{
		Operation: repo.PullRequestChangeDelete, DocumentID: createdDocument.ID, BaseVersionID: createdDocument.CurrentVersionID,
	}}, owner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReviewPullRequest(ctx, repository.ID, deletePR.Number, reviewer, repo.ReviewStateApproved, "approved"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.MergePullRequest(ctx, repository.ID, deletePR.Number, reviewer); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetDocument(ctx, createdDocument.ID); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("deleted document error = %v, want not found", err)
	}
}
