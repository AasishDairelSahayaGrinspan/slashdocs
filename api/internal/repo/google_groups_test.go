package repo

import (
	"context"
	"testing"

	"github.com/razorpay/aidocs/api/internal/auth"
)

func TestGoogleGroupRoleMergesWithDirectAndAnyoneUsingHighestPrecedence(t *testing.T) {
	ctx := context.Background()
	store := NewMemory()
	owner := auth.Principal{Type: auth.PrincipalUser, ID: "owner", Email: "owner@example.com"}
	member := auth.Principal{
		Type: auth.PrincipalUser, ID: "member", Email: "member@example.com",
		GoogleGroupIDs: []string{"groups/platform"},
	}
	doc, _, err := store.CreateDocument(ctx, owner, "Role precedence", []byte("<html></html>"))
	if err != nil {
		t.Fatal(err)
	}
	group := auth.Principal{Type: auth.PrincipalGoogleGroup, ID: "groups/platform", Email: "platform@example.com"}
	if _, err := store.CreateGrant(ctx, "document", doc.ID, group, RoleViewer, owner); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateGrant(ctx, "document", doc.ID, member, RoleCommenter, owner); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateGrant(ctx, "document", doc.ID, auth.Principal{Type: auth.PrincipalAnyone}, RoleEditor, owner); err != nil {
		t.Fatal(err)
	}
	if role, err := store.RoleForDocument(ctx, member, doc.ID); err != nil || role != RoleEditor {
		t.Fatalf("merged role=%q err=%v, want editor", role, err)
	}

	if _, err := store.CreateGrant(ctx, "document", doc.ID, group, RoleOwner, owner); err != nil {
		t.Fatal(err)
	}
	if role, err := store.RoleForDocument(ctx, member, doc.ID); err != nil || role != RoleOwner {
		t.Fatalf("owner group role=%q err=%v", role, err)
	}

	serviceAccount := auth.Principal{
		Type: auth.PrincipalServiceAccount, ID: "sa_1",
		GoogleGroupIDs: []string{"groups/platform"},
	}
	if role, err := store.RoleForDocument(ctx, serviceAccount, doc.ID); err != nil || role != RoleEditor {
		t.Fatalf("service account inherited Google membership: role=%q err=%v", role, err)
	}
}
