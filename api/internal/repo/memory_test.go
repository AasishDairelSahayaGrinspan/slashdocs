package repo

import (
	"context"
	"errors"
	"testing"

	"github.com/razorpay/aidocs/api/internal/auth"
)

func TestMemoryRepositoryDocumentGrantVersionAndCommentFlow(t *testing.T) {
	ctx := context.Background()
	store := NewMemory()
	owner := auth.Principal{Type: auth.PrincipalUser, ID: "usr_1", Email: "owner@example.com", Name: "Owner"}
	editor := auth.Principal{Type: auth.PrincipalUser, ID: "usr_2", Email: "editor@example.com", Name: "Editor"}

	doc, ver, err := store.CreateDocument(ctx, owner, "Report", []byte("<html>v1</html>"))
	if err != nil {
		t.Fatal(err)
	}
	if role, err := store.RoleForDocument(ctx, owner, doc.ID); err != nil || role != RoleOwner {
		t.Fatalf("owner role = %q err=%v, want owner nil", role, err)
	}
	if _, err := store.RoleForDocument(ctx, editor, doc.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("editor role before grant = %v, want ErrNotFound", err)
	}

	grant, err := store.CreateGrant(ctx, doc.ID, editor, RoleCommenter, owner)
	if err != nil {
		t.Fatal(err)
	}
	if role, err := store.RoleForDocument(ctx, editor, doc.ID); err != nil || role != RoleCommenter {
		t.Fatalf("editor role after grant = %q err=%v, want commenter nil", role, err)
	}
	if grant, err = store.UpdateGrant(ctx, doc.ID, grant.ID, RoleEditor); err != nil || grant.Role != RoleEditor {
		t.Fatalf("UpdateGrant = %+v %v, want editor nil", grant, err)
	}
	if grants, err := store.ListGrants(ctx, doc.ID); err != nil || len(grants) != 1 {
		t.Fatalf("ListGrants len=%d err=%v, want 1 nil", len(grants), err)
	}

	if _, err := store.CreateVersion(ctx, doc.ID, "stale", "bad", nil, editor); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("CreateVersion stale = %v, want ErrVersionConflict", err)
	}
	ver2, err := store.CreateVersion(ctx, doc.ID, ver.ID, "update", []byte("<html>v2</html>"), editor)
	if err != nil {
		t.Fatal(err)
	}
	if ver2.Number != 2 {
		t.Fatalf("version number = %d, want 2", ver2.Number)
	}
	if versions, err := store.ListVersions(ctx, doc.ID); err != nil || len(versions) != 2 {
		t.Fatalf("ListVersions len=%d err=%v, want 2 nil", len(versions), err)
	}
	if html, err := store.GetVersionHTML(ctx, ver2.ID); err != nil || string(html) != "<html></html>" {
		t.Fatalf("GetVersionHTML = %q %v, want fixture html nil", html, err)
	}

	comment, err := store.CreateComment(ctx, doc.ID, ver2.ID, "Needs detail", Anchor{Quote: "v2"}, editor)
	if err != nil {
		t.Fatal(err)
	}
	reply, err := store.CreateCommentReply(ctx, doc.ID, comment.ID, "Done", owner)
	if err != nil {
		t.Fatal(err)
	}
	if reply.ParentCommentID != comment.ID || reply.RootCommentID != comment.ID {
		t.Fatalf("reply linkage = %+v", reply)
	}
	if updated, err := store.UpdateComment(ctx, comment.ID, "Resolved", "resolved"); err != nil || updated.Status != "resolved" {
		t.Fatalf("UpdateComment = %+v %v, want resolved nil", updated, err)
	}
	if comments, err := store.ListComments(ctx, doc.ID, "resolved", ver2.ID); err != nil || len(comments) != 1 {
		t.Fatalf("ListComments len=%d err=%v, want 1 nil", len(comments), err)
	}
	if err := store.DeleteComment(ctx, reply.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteGrant(ctx, doc.ID, grant.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteDocument(ctx, doc.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetDocument(ctx, doc.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetDocument after delete = %v, want ErrNotFound", err)
	}
}

func TestMemoryRepositoryUsersCredentialsAndServiceAccounts(t *testing.T) {
	ctx := context.Background()
	store := NewMemory()

	user, err := store.UpsertGoogleUser(ctx, "usr_1", "Owner@Example.com", "Owner", "sub", "pic")
	if err != nil {
		t.Fatal(err)
	}
	updated, err := store.UpsertGoogleUser(ctx, "usr_ignored", "owner@example.com", "Owner Two", "sub2", "pic2")
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != user.ID || updated.Name != "Owner Two" || updated.PictureURL != "pic2" {
		t.Fatalf("case-insensitive upsert = %+v, original %+v", updated, user)
	}
	if got, err := store.UserByEmail(ctx, "OWNER@example.com"); err != nil || got.ID != user.ID {
		t.Fatalf("UserByEmail = %+v %v, want user nil", got, err)
	}
	if users, err := store.SearchUsersByEmail(ctx, "owner"); err != nil || len(users) != 1 {
		t.Fatalf("SearchUsersByEmail len=%d err=%v, want 1 nil", len(users), err)
	}
	placeholder, err := store.EnsureUserByEmail(ctx, "new@example.com")
	if err != nil || placeholder.Email != "new@example.com" {
		t.Fatalf("EnsureUserByEmail = %+v %v", placeholder, err)
	}

	credID, err := store.CreateCLICredential(ctx, user.ID, "laptop", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if creds, err := store.ListCLICredentials(ctx, user.ID); err != nil || len(creds) != 1 || creds[0].ID != credID {
		t.Fatalf("ListCLICredentials = %+v %v", creds, err)
	}
	if principal, err := store.ResolveBearerToken(ctx, "hash"); err != nil || principal.ID != user.ID {
		t.Fatalf("ResolveBearerToken = %+v %v", principal, err)
	}
	if err := store.RevokeCLICredential(ctx, user.ID, credID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResolveBearerToken(ctx, "hash"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ResolveBearerToken revoked = %v, want ErrNotFound", err)
	}

	sa, err := store.CreateServiceAccount(ctx, user, "bot@example.bot")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateServiceAccount(ctx, user, "bot@example.bot"); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate service account = %v, want ErrConflict", err)
	}
	if got, err := store.GetServiceAccountByName(ctx, "bot@example.bot"); err != nil || got.ID != sa.ID {
		t.Fatalf("GetServiceAccountByName = %+v %v", got, err)
	}
	disabled := true
	if got, err := store.UpdateServiceAccount(ctx, sa.ID, "bot2@example.bot", &disabled); err != nil || !got.Disabled || got.Name != "bot2@example.bot" {
		t.Fatalf("UpdateServiceAccount = %+v %v", got, err)
	}
	keyID, err := store.CreateServiceAccountKey(ctx, sa.ID, "default", "sa_hash")
	if err != nil || keyID == "" {
		t.Fatalf("CreateServiceAccountKey = %q %v", keyID, err)
	}
	if principal, err := store.ResolveBearerToken(ctx, "sa_hash"); err != nil || principal.Type != auth.PrincipalServiceAccount || principal.ID != sa.ID {
		t.Fatalf("ResolveBearerToken service account = %+v %v", principal, err)
	}
	if exists, err := store.PrincipalExists(ctx, auth.Principal{Type: auth.PrincipalServiceAccount, ID: sa.ID}); err != nil || !exists {
		t.Fatalf("PrincipalExists service account = %v %v, want true nil", exists, err)
	}
}

func TestMemoryRepositoryOwnershipTransfers(t *testing.T) {
	ctx := context.Background()
	store := NewMemory()
	from := auth.Principal{Type: auth.PrincipalUser, ID: "usr_from", Email: "from@example.com"}
	to, err := store.EnsureUserByEmail(ctx, "to@example.com")
	if err != nil {
		t.Fatal(err)
	}
	xfer, err := store.CreateOwnershipTransfer(ctx, "sa_1", from, to.Email)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AcceptOwnershipTransfer(ctx, xfer.ID, from); !errors.Is(err, ErrNotTransferTarget) {
		t.Fatalf("AcceptOwnershipTransfer wrong user = %v, want ErrNotTransferTarget", err)
	}
	accepted, err := store.AcceptOwnershipTransfer(ctx, xfer.ID, to)
	if err != nil || accepted.Status != StatusAccepted {
		t.Fatalf("AcceptOwnershipTransfer = %+v %v, want accepted nil", accepted, err)
	}
	if _, err := store.AcceptOwnershipTransfer(ctx, xfer.ID, to); !errors.Is(err, ErrTransferNotPending) {
		t.Fatalf("AcceptOwnershipTransfer accepted = %v, want ErrTransferNotPending", err)
	}

	decline, err := store.CreateOwnershipTransfer(ctx, "sa_2", from, to.Email)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.DeclineOwnershipTransfer(ctx, decline.ID, from); err != nil {
		t.Fatal(err)
	}
	if err := store.DeclineOwnershipTransfer(ctx, "missing", from); !errors.Is(err, ErrNotFound) {
		t.Fatalf("DeclineOwnershipTransfer missing = %v, want ErrNotFound", err)
	}
}
