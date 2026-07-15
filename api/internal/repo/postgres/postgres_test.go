package postgres_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/razorpay/aidocs/api/internal/auth"
	"github.com/razorpay/aidocs/api/internal/blob"
	"github.com/razorpay/aidocs/api/internal/db"
	"github.com/razorpay/aidocs/api/internal/repo"
	pgrepo "github.com/razorpay/aidocs/api/internal/repo/postgres"
)

func TestPostgresRepositoryDocumentGrantVersionCommentFlow(t *testing.T) {
	url := os.Getenv("AIDOCS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set AIDOCS_TEST_DATABASE_URL to run postgres integration tests")
	}
	if err := db.Migrate(url); err != nil {
		t.Fatal(err)
	}
	store, err := pgrepo.Connect(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	owner := auth.Principal{Type: auth.PrincipalUser, ID: "usr_pg_owner", Email: "pg-owner@example.com", Name: "Owner"}
	doc, ver, err := store.CreateDocument(context.Background(), owner, "Postgres doc", []byte("<html>v1</html>"))
	if err != nil {
		t.Fatal(err)
	}
	if doc.CurrentVersionID != ver.ID {
		t.Fatalf("current version mismatch")
	}

	role, err := store.RoleForDocument(context.Background(), owner, doc.ID)
	if err != nil || role != repo.RoleOwner {
		t.Fatalf("role=%s err=%v", role, err)
	}

	sa, err := store.CreateServiceAccount(context.Background(), owner, "pg-bot")
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.CreateGrant(context.Background(), "document", doc.ID, auth.Principal{Type: auth.PrincipalServiceAccount, ID: sa.ID}, repo.RoleEditor, owner)
	if err != nil {
		t.Fatal(err)
	}
	role, err = store.RoleForDocument(context.Background(), auth.Principal{Type: auth.PrincipalServiceAccount, ID: sa.ID}, doc.ID)
	if err != nil || role != repo.RoleEditor {
		t.Fatalf("sa role=%s err=%v", role, err)
	}

	_, err = store.CreateVersion(context.Background(), doc.ID, "stale", "bad", []byte("<html>bad</html>"), owner)
	if err == nil || err.Error() != "version_conflict" {
		t.Fatalf("want version_conflict, got %v", err)
	}

	ver2, err := store.CreateVersion(context.Background(), doc.ID, ver.ID, "updated", []byte("<html>v2</html>"), owner)
	if err != nil {
		t.Fatal(err)
	}
	if ver2.Number != 2 {
		t.Fatalf("number=%d", ver2.Number)
	}

	commenter := auth.Principal{Type: auth.PrincipalUser, ID: "usr_pg_commenter", Email: "c@example.com", Name: "Commenter"}
	cm, err := store.CreateComment(context.Background(), doc.ID, ver2.ID, "Looks good", repo.Anchor{Quote: "v2", Prefix: "<html>", Suffix: "</html>", DOMPath: "body", StartOffset: 0, EndOffset: 2}, commenter)
	if err != nil {
		t.Fatal(err)
	}
	if cm.SelectedText != "v2" || cm.Status != "open" {
		t.Fatalf("bad comment: %+v", cm)
	}
}

func TestPostgresGoogleGroupMembershipGrantAndInheritanceFlow(t *testing.T) {
	url := os.Getenv("AIDOCS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set AIDOCS_TEST_DATABASE_URL to run postgres integration tests")
	}
	if err := db.Migrate(url); err != nil {
		t.Fatal(err)
	}
	store, err := pgrepo.Connect(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	owner := auth.Principal{Type: auth.PrincipalUser, ID: "usr_pg_group_owner", Email: "pg-group-owner@example.com", Name: "Owner"}
	folder, err := store.CreateFolder(ctx, owner, "PG Group Parent", "")
	if err != nil {
		t.Fatal(err)
	}
	doc, _, err := store.CreateDocument(ctx, owner, "PG Group Inherited Doc", []byte("<html>group</html>"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.MoveDocumentToFolder(ctx, owner, folder.ID, doc.ID); err != nil {
		t.Fatal(err)
	}
	rootDoc, _, err := store.CreateDocument(ctx, owner, "PG Group Root Doc", []byte("<html>root</html>"))
	if err != nil {
		t.Fatal(err)
	}
	member, err := store.EnsureUserByEmail(ctx, "pg-group-member@example.com")
	if err != nil {
		t.Fatal(err)
	}
	group := repo.GoogleGroup{ID: "groups/pg-group-1", CustomerID: "C123", Email: "pg-group@example.com", Name: "PG Group", LastVerifiedAt: now}
	if err := store.ReplaceGoogleGroupMemberships(ctx, member.ID, []repo.GoogleGroup{group}, now); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.GoogleGroupMembershipSnapshot(ctx, member.ID)
	if err != nil || len(snapshot.GroupIDs) != 1 || snapshot.GroupIDs[0] != group.ID || !snapshot.LastSuccessAt.Equal(now) {
		t.Fatalf("snapshot=%+v err=%v", snapshot, err)
	}
	groupPrincipal := auth.Principal{Type: auth.PrincipalGoogleGroup, ID: group.ID, Email: group.Email, Name: group.Name}
	folderGrant, err := store.CreateGrant(ctx, "folder", folder.ID, groupPrincipal, repo.RoleEditor, owner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateGrant(ctx, "document", rootDoc.ID, groupPrincipal, repo.RoleViewer, owner); err != nil {
		t.Fatal(err)
	}
	member.GoogleGroupIDs = snapshot.GroupIDs
	if role, err := store.RoleForDocument(ctx, member, doc.ID); err != nil || role != repo.RoleEditor {
		t.Fatalf("inherited role=%s err=%v", role, err)
	}
	if role, err := store.RoleForFolder(ctx, member, folder.ID); err != nil || role != repo.RoleEditor {
		t.Fatalf("folder role=%s err=%v", role, err)
	}
	if !listed(t, store, member, rootDoc.ID) {
		t.Fatal("direct group grant missing from workspace")
	}
	results, err := store.SearchDocumentsByTitle(ctx, member, "Inherited Doc", false, 25)
	if err != nil || len(results) != 1 || results[0].ID != doc.ID {
		t.Fatalf("group inherited search=%+v err=%v", results, err)
	}
	folders, err := store.SearchFoldersByTitle(ctx, member, "PG Group Parent", false, 25)
	if err != nil || len(folders) != 1 || folders[0].ID != folder.ID {
		t.Fatalf("group folder search=%+v err=%v", folders, err)
	}
	access, err := store.ListEffectiveAccess(ctx, "document", doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	foundGroupSource := false
	for _, item := range access {
		if item.Principal.Type == auth.PrincipalGoogleGroup && item.Principal.ID == group.ID && item.EffectiveRole == repo.RoleEditor && len(item.Sources) > 0 && item.Sources[0].GrantID == folderGrant.ID {
			foundGroupSource = true
		}
	}
	if !foundGroupSource {
		t.Fatalf("effective access missing inherited group source: %+v", access)
	}

	group.Email = "pg-group-renamed@example.com"
	group.Name = "PG Group Renamed"
	group.LastVerifiedAt = now.Add(time.Second)
	if err := store.UpsertGoogleGroup(ctx, group); err != nil {
		t.Fatal(err)
	}
	gotGrant, err := store.GetGrant(ctx, folder.ID, folderGrant.ID)
	if err != nil || gotGrant.Principal.Email != group.Email || gotGrant.Principal.ID != group.ID {
		t.Fatalf("renamed grant=%+v err=%v", gotGrant, err)
	}
	if role, err := store.RoleForDocument(ctx, member, doc.ID); err != nil || role != repo.RoleEditor {
		t.Fatalf("rename broke immutable-ID authorization: role=%s err=%v", role, err)
	}

	failedAt := now.Add(2 * time.Second)
	if err := store.RecordGoogleGroupMembershipFailure(ctx, member.ID, "timeout", failedAt); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = store.GoogleGroupMembershipSnapshot(ctx, member.ID)
	if len(snapshot.GroupIDs) != 1 || snapshot.LastErrorCode != "timeout" || !snapshot.LastSuccessAt.Equal(now) {
		t.Fatalf("failed refresh did not preserve diagnostics snapshot: %+v", snapshot)
	}
	removedAt := now.Add(3 * time.Second)
	if err := store.ReplaceGoogleGroupMemberships(ctx, member.ID, nil, removedAt); err != nil {
		t.Fatal(err)
	}
	member.GoogleGroupIDs = nil
	if role, err := store.RoleForDocument(ctx, member, doc.ID); err == nil || role != repo.RoleNone {
		t.Fatalf("removed member retained access: role=%s err=%v", role, err)
	}

	ownerGrant, err := store.CreateGrant(ctx, "document", rootDoc.ID, groupPrincipal, repo.RoleOwner, owner)
	if err != nil {
		t.Fatal(err)
	}
	member.GoogleGroupIDs = []string{group.ID}
	if role, err := store.RoleForDocument(ctx, member, rootDoc.ID); err != nil || role != repo.RoleOwner {
		t.Fatalf("owner group role=%s err=%v", role, err)
	}
	storedRoot, _ := store.GetDocument(ctx, rootDoc.ID)
	if storedRoot.Owner.ID != owner.ID || ownerGrant.Principal.ID != group.ID {
		t.Fatalf("owner grant replaced canonical owner: doc=%+v grant=%+v", storedRoot, ownerGrant)
	}
}

// A public ("anyone") grant gives a stranger access to the document via the
// link, but it only joins their workspace listing once they open it -- and an
// explicit grant lists the document without needing an open.
func TestPostgresAnyoneGrantListsAfterOpen(t *testing.T) {
	url := os.Getenv("AIDOCS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set AIDOCS_TEST_DATABASE_URL to run postgres integration tests")
	}
	if err := db.Migrate(url); err != nil {
		t.Fatal(err)
	}
	store, err := pgrepo.Connect(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()

	owner := auth.Principal{Type: auth.PrincipalUser, ID: "usr_pg_anyone_owner", Email: "anyone-owner@example.com", Name: "Owner"}
	doc, _, err := store.CreateDocument(ctx, owner, "Public sample", []byte("<html>public</html>"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateGrant(ctx, "document", doc.ID, auth.Principal{Type: auth.PrincipalAnyone}, repo.RoleViewer, owner); err != nil {
		t.Fatal(err)
	}

	stranger := auth.Principal{Type: auth.PrincipalUser, ID: "usr_pg_anyone_stranger", Email: "stranger@example.com", Name: "Stranger"}

	// The stranger can reach the document via the public link.
	role, err := store.RoleForDocument(ctx, stranger, doc.ID)
	if err != nil || role != repo.RoleViewer {
		t.Fatalf("stranger role=%s err=%v, want viewer", role, err)
	}

	// Before opening, the public document is not in the stranger's workspace.
	if listed(t, store, stranger, doc.ID) {
		t.Fatalf("public doc %s listed before stranger opened it", doc.ID)
	}

	// Opening it adds it to their workspace.
	if err := store.RecordDocumentOpened(ctx, doc.ID, stranger); err != nil {
		t.Fatal(err)
	}
	if !listed(t, store, stranger, doc.ID) {
		t.Fatalf("public doc %s not listed after stranger opened it", doc.ID)
	}

	// A second user with an explicit grant sees it without opening.
	invitee := auth.Principal{Type: auth.PrincipalUser, ID: "usr_pg_anyone_invitee", Email: "invitee@example.com", Name: "Invitee"}
	if _, err := store.CreateGrant(ctx, "document", doc.ID, invitee, repo.RoleViewer, owner); err != nil {
		t.Fatal(err)
	}
	if !listed(t, store, invitee, doc.ID) {
		t.Fatalf("explicitly-granted doc %s not listed for invitee", doc.ID)
	}
}

func listed(t *testing.T, store *pgrepo.Store, p auth.Principal, docID string) bool {
	t.Helper()
	docs, err := store.ListDocuments(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range docs {
		if d.ID == docID {
			return true
		}
	}
	return false
}

func TestPostgresCreateDocumentDeletesBlobWhenDBFailsAfterPut(t *testing.T) {
	url := os.Getenv("AIDOCS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set AIDOCS_TEST_DATABASE_URL to run postgres integration tests")
	}
	if err := db.Migrate(url); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	blobs := &cancelAfterPutBlob{cancel: cancel}
	store, err := pgrepo.ConnectWithBlob(context.Background(), url, blobs)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	owner := auth.Principal{Type: auth.PrincipalUser, ID: "usr_blob_cleanup_doc", Email: "blob-cleanup-doc@example.com", Name: "Owner"}
	_, _, err = store.CreateDocument(ctx, owner, "cleanup doc", []byte("<html>cleanup</html>"))
	if err == nil {
		t.Fatal("CreateDocument succeeded; want context cancellation after blob put")
	}
	if blobs.puts != 1 || blobs.deletes != 1 {
		t.Fatalf("blob puts=%d deletes=%d, want 1/1", blobs.puts, blobs.deletes)
	}
	if blobs.deletedKey != blobs.putKey {
		t.Fatalf("deleted key %q, want put key %q", blobs.deletedKey, blobs.putKey)
	}
}

func TestPostgresCreateVersionDeletesBlobWhenDBFailsAfterPut(t *testing.T) {
	url := os.Getenv("AIDOCS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set AIDOCS_TEST_DATABASE_URL to run postgres integration tests")
	}
	if err := db.Migrate(url); err != nil {
		t.Fatal(err)
	}
	blobs := &cancelAfterPutBlob{}
	store, err := pgrepo.ConnectWithBlob(context.Background(), url, blobs)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	owner := auth.Principal{Type: auth.PrincipalUser, ID: "usr_blob_cleanup_ver", Email: "blob-cleanup-ver@example.com", Name: "Owner"}
	doc, ver, err := store.CreateDocument(context.Background(), owner, "cleanup version", []byte("<html>v1</html>"))
	if err != nil {
		t.Fatal(err)
	}
	blobs.puts, blobs.deletes, blobs.putKey, blobs.deletedKey = 0, 0, "", ""
	ctx, cancel := context.WithCancel(context.Background())
	blobs.cancel = cancel
	_, err = store.CreateVersion(ctx, doc.ID, ver.ID, "will fail", []byte("<html>v2</html>"), owner)
	if err == nil {
		t.Fatal("CreateVersion succeeded; want context cancellation after blob put")
	}
	if blobs.puts != 1 || blobs.deletes != 1 {
		t.Fatalf("blob puts=%d deletes=%d, want 1/1", blobs.puts, blobs.deletes)
	}
	if blobs.deletedKey != blobs.putKey {
		t.Fatalf("deleted key %q, want put key %q", blobs.deletedKey, blobs.putKey)
	}
}

type cancelAfterPutBlob struct {
	cancel     context.CancelFunc
	puts       int
	deletes    int
	putKey     string
	deletedKey string
}

func (b *cancelAfterPutBlob) Put(ctx context.Context, key, contentType string, body []byte) error {
	b.puts++
	b.putKey = key
	if b.cancel != nil {
		b.cancel()
	}
	return nil
}
func (b *cancelAfterPutBlob) Get(ctx context.Context, key string) ([]byte, string, error) {
	return nil, "", blob.ErrNotFound
}
func (b *cancelAfterPutBlob) Delete(ctx context.Context, key string) error {
	b.deletes++
	b.deletedKey = key
	return nil
}

func TestPostgresSearchDocumentsViaFolderGrant(t *testing.T) {
	url := os.Getenv("AIDOCS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set AIDOCS_TEST_DATABASE_URL to run postgres integration tests")
	}
	if err := db.Migrate(url); err != nil {
		t.Fatal(err)
	}
	store, err := pgrepo.Connect(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()

	owner := auth.Principal{Type: auth.PrincipalUser, ID: "usr_pg_search_folder_owner", Email: "search-folder-owner@test.com", Name: "Owner"}
	viewer := auth.Principal{Type: auth.PrincipalUser, ID: "usr_pg_search_folder_viewer", Email: "search-folder-viewer@test.com", Name: "Viewer"}

	folder, err := store.CreateFolder(ctx, owner, "SearchTestFolder", "")
	if err != nil {
		t.Fatal(err)
	}
	doc, _, err := store.CreateDocument(ctx, owner, "SearchableInFolder", []byte("<html>hello</html>"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.MoveDocumentToFolder(ctx, owner, folder.ID, doc.ID); err != nil {
		t.Fatal(err)
	}

	// Before granting folder access, viewer cannot find the doc
	results, err := store.SearchDocumentsByTitle(ctx, viewer, "SearchableInFolder", false, 25)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		if r.ID == doc.ID {
			t.Fatal("viewer found doc before folder grant")
		}
	}

	// Grant viewer access on the folder
	if _, err := store.CreateGrant(ctx, "folder", folder.ID, viewer, repo.RoleViewer, owner); err != nil {
		t.Fatal(err)
	}

	// Now the viewer can find the doc via folder inheritance
	results, err = store.SearchDocumentsByTitle(ctx, viewer, "SearchableInFolder", false, 25)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range results {
		if r.ID == doc.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("viewer cannot find doc after folder grant — folder-inherited search is broken")
	}
}
