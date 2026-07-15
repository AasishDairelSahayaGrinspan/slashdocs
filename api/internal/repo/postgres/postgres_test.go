package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/razorpay/aidocs/api/internal/auth"
	"github.com/razorpay/aidocs/api/internal/blob"
	"github.com/razorpay/aidocs/api/internal/repo"
	pgrepo "github.com/razorpay/aidocs/api/internal/repo/postgres"
	"github.com/razorpay/aidocs/api/internal/testdb"
)

func TestPostgresRepositoryDocumentGrantVersionCommentFlow(t *testing.T) {
	store := newStore(t)

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
	cm, err := store.CreateComment(context.Background(), doc.ID, ver2.ID, "Looks good", repo.Anchor{Quote: "v2", Prefix: "<html>", Suffix: "</html>", DOMPath: "body", StartOffset: 0, EndOffset: 2}, commenter, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cm.SelectedText != "v2" || cm.Status != "open" {
		t.Fatalf("bad comment: %+v", cm)
	}
}

func TestPostgresInviteThenMixedCaseLoginPreservesIdentityAndGrant(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	owner := auth.Principal{
		Type: auth.PrincipalUser, ID: "usr_email_invite_owner_" + suffix,
		Email: "email-invite-owner-" + suffix + "@example.com", Name: "Owner",
	}
	document, _, err := store.CreateDocument(ctx, owner, "Email invite "+suffix, []byte("<p>invite</p>"))
	if err != nil {
		t.Fatal(err)
	}

	address := "reviewer-" + suffix + "@example.com"
	placeholder, err := store.EnsureUserByEmail(ctx, "  ReViEwEr-"+suffix+"@Example.COM  ")
	if err != nil {
		t.Fatal(err)
	}
	if placeholder.ID == "" || placeholder.Email != address {
		t.Fatalf("placeholder = %#v, want normalized address %q", placeholder, address)
	}
	if _, err := store.CreateGrant(ctx, "document", document.ID, placeholder, repo.RoleCommenter, owner); err != nil {
		t.Fatal(err)
	}

	loggedIn, err := store.UpsertGoogleUser(
		ctx,
		"usr_email_login_candidate_"+suffix,
		"REVIEWER-"+suffix+"@EXAMPLE.COM",
		"Invited reviewer",
		"oidc-email-invite-"+suffix,
		"https://example.com/reviewer.png",
	)
	if err != nil {
		t.Fatal(err)
	}
	if loggedIn.ID != placeholder.ID || loggedIn.Email != address {
		t.Fatalf("logged-in identity = %#v, want placeholder id %q and email %q", loggedIn, placeholder.ID, address)
	}

	byEmail, err := store.UserByEmail(ctx, " Reviewer-"+suffix+"@Example.com ")
	if err != nil || byEmail.ID != placeholder.ID {
		t.Fatalf("case-insensitive signed-in lookup = %#v, err=%v", byEmail, err)
	}
	sharedAgain, err := store.EnsureUserByEmail(ctx, "REVIEWER-"+suffix+"@EXAMPLE.COM")
	if err != nil || sharedAgain.ID != placeholder.ID {
		t.Fatalf("case-insensitive share lookup = %#v, err=%v", sharedAgain, err)
	}
	role, err := store.RoleForDocument(ctx, loggedIn, document.ID)
	if err != nil || role != repo.RoleCommenter {
		t.Fatalf("grant after login = %q, err=%v", role, err)
	}

	serviceAccount, err := store.CreateServiceAccount(ctx, owner, "email-invite-bot-"+suffix)
	if err != nil {
		t.Fatal(err)
	}
	transfer, err := store.CreateOwnershipTransfer(ctx, serviceAccount.ID, owner, " REVIEWER-"+suffix+"@Example.com ")
	if err != nil {
		t.Fatal(err)
	}
	if transfer.ToUserID != placeholder.ID {
		t.Fatalf("ownership transfer target = %q, want %q", transfer.ToUserID, placeholder.ID)
	}
}

func TestPostgresProtectedRepositoryPullRequestFlow(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	owner := auth.Principal{
		Type:  auth.PrincipalUser,
		ID:    "usr_pg_pr_owner_" + suffix,
		Email: "pg-pr-owner-" + suffix + "@example.com",
		Name:  "Repository Owner " + suffix,
	}
	editor := auth.Principal{
		Type:  auth.PrincipalUser,
		ID:    "usr_pg_pr_editor_" + suffix,
		Email: "pg-pr-editor-" + suffix + "@example.com",
		Name:  "Repository Editor " + suffix,
	}
	baseHTML := []byte("<html><body>base " + suffix + "</body></html>")
	document, base, err := store.CreateDocument(ctx, owner, "Governed document "+suffix, baseHTML)
	if err != nil {
		t.Fatal(err)
	}

	repository, err := store.CreateRepository(ctx, owner, repo.CreateRepositoryParams{
		Name:              "Governed repository " + suffix,
		Slug:              "governed-repository-" + suffix,
		Scope:             repo.RepositoryScopePersonal,
		NamespaceID:       owner.ID,
		NamespaceName:     owner.Name,
		DocumentID:        document.ID,
		RequiredApprovals: 1,
		Protected:         true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if repository.RootFolderID == "" || repository.RequiredApprovals != 1 || !repository.Protected {
		t.Fatalf("repository = %+v, want protected root with one required approval", repository)
	}
	if _, err := store.CreateGrant(ctx, "folder", repository.RootFolderID, editor, repo.RoleEditor, owner); err != nil {
		t.Fatal(err)
	}
	role, err := store.RoleForRepository(ctx, editor, repository.ID)
	if err != nil || role != repo.RoleEditor {
		t.Fatalf("editor role = %q, err = %v; want editor", role, err)
	}

	_, err = store.CreateVersion(ctx, document.ID, base.ID, "bypass governance", []byte("<html>direct</html>"), owner)
	if !errors.Is(err, repo.ErrPullRequestRequired) {
		t.Fatalf("direct CreateVersion error = %v, want %v", err, repo.ErrPullRequestRequired)
	}
	var pullRequestRequired *repo.PullRequestRequiredError
	if !errors.As(err, &pullRequestRequired) || pullRequestRequired.RepositoryID != repository.ID {
		t.Fatalf("direct CreateVersion error = %#v, want repository id %q", err, repository.ID)
	}

	proposedHTML := []byte("<html><body>proposal " + suffix + "</body></html>")
	pullRequest, err := store.CreatePullRequest(
		ctx,
		repository.ID,
		document.ID,
		base.ID,
		"Publish the governed update",
		"Database-backed pull request",
		"replace the body",
		proposedHTML,
		owner,
	)
	if err != nil {
		t.Fatal(err)
	}
	if pullRequest.HeadVersionID == "" || pullRequest.HeadVersionID == base.ID {
		t.Fatalf("head version = %q, want a distinct proposed version", pullRequest.HeadVersionID)
	}
	if pullRequest.Status != repo.PullRequestStatusOpen || pullRequest.Mergeable || pullRequest.MergeBlockedReason != repo.ErrApprovalRequired.Error() {
		t.Fatalf("new pull request = %+v, want open and blocked on approval", pullRequest)
	}

	afterOpen, err := store.GetDocument(ctx, document.ID)
	if err != nil {
		t.Fatal(err)
	}
	if afterOpen.CurrentVersionID != base.ID {
		t.Fatalf("current version after opening pull request = %q, want %q", afterOpen.CurrentVersionID, base.ID)
	}
	versions, err := store.ListVersions(ctx, document.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 || versions[0].ID != base.ID {
		t.Fatalf("published versions after opening pull request = %+v, want only base %q", versions, base.ID)
	}
	proposed, err := store.GetVersion(ctx, pullRequest.HeadVersionID)
	if err != nil {
		t.Fatal(err)
	}
	if proposed.Number != 0 {
		t.Fatalf("proposed version number = %d, want 0 until merge", proposed.Number)
	}
	gotHTML, err := store.GetVersionHTML(ctx, pullRequest.HeadVersionID)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotHTML) != string(proposedHTML) {
		t.Fatalf("proposed html = %q, want %q", gotHTML, proposedHTML)
	}

	if _, err := store.ReviewPullRequest(ctx, repository.ID, pullRequest.Number, owner, repo.ReviewStateApproved, "self approve"); !errors.Is(err, repo.ErrSelfReview) {
		t.Fatalf("self review error = %v, want %v", err, repo.ErrSelfReview)
	}
	if _, err := store.MergePullRequest(ctx, repository.ID, pullRequest.Number, owner); !errors.Is(err, repo.ErrApprovalRequired) {
		t.Fatalf("merge before approval error = %v, want %v", err, repo.ErrApprovalRequired)
	}
	approved, err := store.ReviewPullRequest(ctx, repository.ID, pullRequest.Number, editor, repo.ReviewStateApproved, "ship it")
	if err != nil {
		t.Fatal(err)
	}
	if !approved.Mergeable || len(approved.Reviews) != 1 || approved.Reviews[0].Reviewer.ID != editor.ID {
		t.Fatalf("approved pull request = %+v, want eligible editor approval", approved)
	}

	merged, err := store.MergePullRequest(ctx, repository.ID, pullRequest.Number, owner)
	if err != nil {
		t.Fatal(err)
	}
	if merged.Status != repo.PullRequestStatusMerged || merged.MergedBy == nil || merged.MergedBy.ID != owner.ID {
		t.Fatalf("merged pull request = %+v", merged)
	}
	afterMerge, err := store.GetDocument(ctx, document.ID)
	if err != nil {
		t.Fatal(err)
	}
	if afterMerge.CurrentVersionID != pullRequest.HeadVersionID {
		t.Fatalf("current version after merge = %q, want %q", afterMerge.CurrentVersionID, pullRequest.HeadVersionID)
	}
	published, err := store.GetVersion(ctx, pullRequest.HeadVersionID)
	if err != nil {
		t.Fatal(err)
	}
	if published.Number != 2 {
		t.Fatalf("merged version number = %d, want 2", published.Number)
	}
	versions, err = store.ListVersions(ctx, document.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 {
		t.Fatalf("published versions after merge = %+v, want two", versions)
	}
}

func TestPostgresEmptyAndImplicitOrganizationRepositoryAccess(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	owner := auth.Principal{Type: auth.PrincipalUser, ID: "usr_pg_org_owner_" + suffix, Email: "org-owner-" + suffix + "@example.com", Name: "Org Owner"}
	viewer := auth.Principal{Type: auth.PrincipalUser, ID: "usr_pg_org_viewer_" + suffix, Email: "org-viewer-" + suffix + "@example.com", Name: "Org Viewer"}

	empty, err := store.CreateRepository(ctx, owner, repo.CreateRepositoryParams{
		Name:              "Empty repository " + suffix,
		Slug:              "empty-repository-" + suffix,
		Scope:             repo.RepositoryScopePersonal,
		NamespaceID:       owner.ID,
		NamespaceName:     owner.Name,
		RequiredApprovals: 1,
		Protected:         true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if empty.RootFolderID == "" {
		t.Fatalf("empty repository = %+v, want generated root folder", empty)
	}
	if documents, err := store.ListRepositoryDocuments(ctx, owner, empty.ID); err != nil || len(documents) != 0 {
		t.Fatalf("empty repository documents = %+v, err = %v", documents, err)
	}
	bootstrapDocument, bootstrapVersion, err := store.CreateRepositoryDocument(ctx, empty.ID, owner, "Getting started "+suffix, []byte("<html><body>start</body></html>"))
	if err != nil {
		t.Fatal(err)
	}
	if bootstrapDocument.ParentFolderID != empty.RootFolderID || bootstrapDocument.CurrentVersionID != bootstrapVersion.ID || bootstrapVersion.Number != 1 {
		t.Fatalf("bootstrap document = %+v, version = %+v", bootstrapDocument, bootstrapVersion)
	}
	if _, _, err := store.CreateRepositoryDocument(ctx, empty.ID, owner, "Second", []byte("<html></html>")); !errors.Is(err, repo.ErrRepositoryNotEmpty) {
		t.Fatalf("second bootstrap error = %v, want %v", err, repo.ErrRepositoryNotEmpty)
	}
	if _, err := store.CreateVersion(ctx, bootstrapDocument.ID, bootstrapVersion.ID, "direct", []byte("<html>changed</html>"), owner); !errors.Is(err, repo.ErrPullRequestRequired) {
		t.Fatalf("post-bootstrap direct version error = %v, want %v", err, repo.ErrPullRequestRequired)
	}

	document, _, err := store.CreateDocument(ctx, owner, "Organization document "+suffix, []byte("<html><body>org</body></html>"))
	if err != nil {
		t.Fatal(err)
	}
	organization, err := store.CreateRepository(ctx, owner, repo.CreateRepositoryParams{
		Name:              "Organization repository " + suffix,
		Slug:              "organization-repository-" + suffix,
		Scope:             repo.RepositoryScopeOrganization,
		NamespaceID:       "acme",
		NamespaceName:     "Acme",
		DocumentID:        document.ID,
		RequiredApprovals: 1,
		Protected:         true,
		GrantOrgViewer:    false,
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, roleCheck := range map[string]func() (repo.Role, error){
		"repository": func() (repo.Role, error) { return store.RoleForRepository(ctx, viewer, organization.ID) },
		"document":   func() (repo.Role, error) { return store.RoleForDocument(ctx, viewer, document.ID) },
		"folder":     func() (repo.Role, error) { return store.RoleForFolder(ctx, viewer, organization.RootFolderID) },
	} {
		role, err := roleCheck()
		if err != nil || role != repo.RoleViewer {
			t.Fatalf("%s role = %q, err = %v; want viewer", name, role, err)
		}
	}
	repositories, err := store.ListRepositories(ctx, viewer)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range repositories {
		found = found || item.ID == organization.ID
	}
	if !found {
		t.Fatalf("organization repository %q missing from viewer list: %+v", organization.ID, repositories)
	}
	if documents, err := store.ListRepositoryDocuments(ctx, viewer, organization.ID); err != nil || len(documents) != 1 || documents[0].ID != document.ID {
		t.Fatalf("organization documents = %+v, err = %v; want %q", documents, err, document.ID)
	}
}

func TestPostgresGrantByCanonicalUserIDPreservesIdentity(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	owner := auth.Principal{
		Type:  auth.PrincipalUser,
		ID:    "usr_pg_grant_owner_" + suffix,
		Email: "pg-grant-owner-" + suffix + "@example.com",
		Name:  "Grant Owner " + suffix,
	}
	canonical := auth.Principal{
		Type:  auth.PrincipalUser,
		ID:    "usr_pg_grantee_" + suffix,
		Email: "pg-canonical-" + suffix + "@example.com",
		Name:  "Canonical Grantee " + suffix,
	}
	document, _, err := store.CreateDocument(ctx, owner, "Grant target "+suffix, []byte("<html>owner</html>"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.CreateDocument(ctx, canonical, "Canonical identity "+suffix, []byte("<html>canonical</html>")); err != nil {
		t.Fatal(err)
	}

	forged := auth.Principal{
		Type:  auth.PrincipalUser,
		ID:    canonical.ID,
		Email: "pg-forged-" + suffix + "@example.com",
		Name:  "Forged Name",
	}
	grant, err := store.CreateGrant(ctx, "document", document.ID, forged, repo.RoleViewer, owner)
	if err != nil {
		t.Fatal(err)
	}
	if grant.Principal.Email != canonical.Email || grant.Principal.Name != canonical.Name {
		t.Fatalf("granted principal = %+v, want canonical identity %+v", grant.Principal, canonical)
	}
	resolved, err := store.ResolveUser(ctx, canonical.ID)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Email != canonical.Email || resolved.Name != canonical.Name {
		t.Fatalf("canonical identity after grant = %+v, want email %q and name %q", resolved, canonical.Email, canonical.Name)
	}
}

// A public ("anyone") grant gives a stranger access to the document via the
// link, but it only joins their workspace listing once they open it -- and an
// explicit grant lists the document without needing an open.
func TestPostgresAnyoneGrantListsAfterOpen(t *testing.T) {
	store := newStore(t)
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
	ctx, cancel := context.WithCancel(context.Background())
	blobs := &cancelAfterPutBlob{cancel: cancel}
	store, err := pgrepo.ConnectWithBlob(context.Background(), testdb.New(t), blobs)
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
	blobs := &cancelAfterPutBlob{}
	store, err := pgrepo.ConnectWithBlob(context.Background(), testdb.New(t), blobs)
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
	store := newStore(t)
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

// newStore connects a Store to a fresh test database.
func newStore(t *testing.T) *pgrepo.Store {
	t.Helper()
	store, err := pgrepo.Connect(context.Background(), testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	return store
}
