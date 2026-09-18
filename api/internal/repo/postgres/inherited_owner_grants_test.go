package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/razorpay/aidocs/api/internal/auth"
	"github.com/razorpay/aidocs/api/internal/repo"
	pgrepo "github.com/razorpay/aidocs/api/internal/repo/postgres"
)

// inheritedOwnerFixture builds the tree the ownership-cap rule is about:
//
//	folder F          owned by mover, carries an owner-level grant for group
//	  doc moverDoc    owned by mover
//	  doc victimDoc   owned by victim, moved in by mover
//	  folder victimF  owned by victim, re-parented under F by victim
//	    doc nestedDoc owned by victim
type inheritedOwnerFixture struct {
	store     *pgrepo.Store
	victim    auth.Principal
	mover     auth.Principal
	member    auth.Principal
	group     auth.Principal
	folderF   string
	moverDoc  string
	victimDoc string
	victimF   string
	nestedDoc string
}

func newInheritedOwnerFixture(t *testing.T) *inheritedOwnerFixture {
	t.Helper()
	ctx := context.Background()
	f := &inheritedOwnerFixture{
		store:  newStore(t),
		victim: auth.Principal{Type: auth.PrincipalUser, ID: "cap_victim", Email: "cap-victim@example.com"},
		mover:  auth.Principal{Type: auth.PrincipalUser, ID: "cap_mover", Email: "cap-mover@example.com", GoogleGroupIDs: []string{"groups/platform"}},
		member: auth.Principal{Type: auth.PrincipalUser, ID: "cap_member", Email: "cap-member@example.com", GoogleGroupIDs: []string{"groups/platform"}},
		group:  auth.Principal{Type: auth.PrincipalGoogleGroup, ID: "groups/platform", Email: "platform@example.com"},
	}
	for _, p := range []auth.Principal{f.victim, f.mover, f.member} {
		if _, err := f.store.UpsertGoogleUser(ctx, p.ID, p.Email, p.ID, "sub-"+p.ID, ""); err != nil {
			t.Fatalf("seed user %s: %v", p.ID, err)
		}
	}
	if err := f.store.UpsertGoogleGroup(ctx, repo.GoogleGroup{ID: f.group.ID, CustomerID: "C123", Email: f.group.Email, Name: "Platform", LastVerifiedAt: time.Now()}); err != nil {
		t.Fatalf("seed group: %v", err)
	}

	folder, err := f.store.CreateFolder(ctx, f.mover, "MoverFolder", "")
	if err != nil {
		t.Fatalf("create mover folder: %v", err)
	}
	f.folderF = folder.ID

	moverDoc, _, err := f.store.CreateDocument(ctx, f.mover, "Mover doc", []byte("<html></html>"))
	if err != nil {
		t.Fatalf("create mover document: %v", err)
	}
	f.moverDoc = moverDoc.ID

	victimDoc, _, err := f.store.CreateDocument(ctx, f.victim, "Victim doc", []byte("<html></html>"))
	if err != nil {
		t.Fatalf("create victim document: %v", err)
	}
	f.victimDoc = victimDoc.ID
	// The mover only ever holds editor on the victim's document; that is all a
	// move requires.
	if _, err := f.store.CreateGrant(ctx, "document", f.victimDoc, f.mover, repo.RoleEditor, f.victim); err != nil {
		t.Fatalf("grant mover editor: %v", err)
	}

	victimF, err := f.store.CreateFolder(ctx, f.victim, "VictimFolder", "")
	if err != nil {
		t.Fatalf("create victim folder: %v", err)
	}
	f.victimF = victimF.ID
	nestedDoc, _, err := f.store.CreateDocument(ctx, f.victim, "Nested doc", []byte("<html></html>"))
	if err != nil {
		t.Fatalf("create nested document: %v", err)
	}
	f.nestedDoc = nestedDoc.ID
	if _, err := f.store.MoveDocumentToFolder(ctx, f.victim, f.victimF, f.nestedDoc); err != nil {
		t.Fatalf("move nested document: %v", err)
	}
	return f
}

func (f *inheritedOwnerFixture) grantOwnerGroup(t *testing.T, resourceType, resourceID string, grantedBy auth.Principal) {
	t.Helper()
	if _, err := f.store.CreateGrant(context.Background(), resourceType, resourceID, f.group, repo.RoleOwner, grantedBy); err != nil {
		t.Fatalf("grant owner group on %s %s: %v", resourceType, resourceID, err)
	}
}

func (f *inheritedOwnerFixture) assertDocRole(t *testing.T, p auth.Principal, documentID string, want repo.Role) {
	t.Helper()
	role, err := f.store.RoleForDocument(context.Background(), p, documentID)
	if err != nil {
		t.Fatalf("RoleForDocument(%s, %s): %v", p.ID, documentID, err)
	}
	if role != want {
		t.Fatalf("RoleForDocument(%s, %s) = %q, want %q", p.ID, documentID, role, want)
	}
}

func (f *inheritedOwnerFixture) assertFolderRole(t *testing.T, p auth.Principal, folderID string, want repo.Role) {
	t.Helper()
	role, err := f.store.RoleForFolder(context.Background(), p, folderID)
	if err != nil {
		t.Fatalf("RoleForFolder(%s, %s): %v", p.ID, folderID, err)
	}
	if role != want {
		t.Fatalf("RoleForFolder(%s, %s) = %q, want %q", p.ID, folderID, role, want)
	}
}

// effectiveRoleFor returns the effective role the access report attributes to a
// principal on a resource, plus the roles it reports per source resource.
func (f *inheritedOwnerFixture) effectiveRoleFor(t *testing.T, resourceType, resourceID string, p auth.Principal) (repo.Role, map[string]repo.Role) {
	t.Helper()
	items, err := f.store.ListEffectiveAccess(context.Background(), resourceType, resourceID)
	if err != nil {
		t.Fatalf("ListEffectiveAccess(%s, %s): %v", resourceType, resourceID, err)
	}
	for _, item := range items {
		if item.Principal.Type != p.Type || item.Principal.ID != p.ID {
			continue
		}
		sources := map[string]repo.Role{}
		for _, src := range item.Sources {
			sources[src.ResourceID] = src.Role
		}
		return item.EffectiveRole, sources
	}
	t.Fatalf("no effective-access entry for %s:%s on %s %s", p.Type, p.ID, resourceType, resourceID)
	return repo.RoleNone, nil
}

// An owner-level group grant on a folder must not make group members owners of
// resources that belong to someone else, however those resources got there.
func TestInheritedOwnerGroupGrantIsCappedForOtherOwnersResources(t *testing.T) {
	ctx := context.Background()

	t.Run("grant then move", func(t *testing.T) {
		f := newInheritedOwnerFixture(t)
		f.grantOwnerGroup(t, "folder", f.folderF, f.mover)
		if _, err := f.store.MoveDocumentToFolder(ctx, f.mover, f.folderF, f.victimDoc); err != nil {
			t.Fatalf("move victim document: %v", err)
		}
		f.assertDocRole(t, f.mover, f.victimDoc, repo.RoleEditor)
		f.assertDocRole(t, f.member, f.victimDoc, repo.RoleEditor)
	})

	t.Run("move then grant", func(t *testing.T) {
		f := newInheritedOwnerFixture(t)
		if _, err := f.store.MoveDocumentToFolder(ctx, f.mover, f.folderF, f.victimDoc); err != nil {
			t.Fatalf("move victim document: %v", err)
		}
		f.grantOwnerGroup(t, "folder", f.folderF, f.mover)
		f.assertDocRole(t, f.mover, f.victimDoc, repo.RoleEditor)
		f.assertDocRole(t, f.member, f.victimDoc, repo.RoleEditor)
	})

	t.Run("re-parented folder subtree", func(t *testing.T) {
		f := newInheritedOwnerFixture(t)
		f.grantOwnerGroup(t, "folder", f.folderF, f.mover)
		if _, err := f.store.UpdateFolder(ctx, f.victimF, "", &f.folderF); err != nil {
			t.Fatalf("re-parent victim folder: %v", err)
		}
		f.assertFolderRole(t, f.member, f.victimF, repo.RoleEditor)
		f.assertDocRole(t, f.member, f.nestedDoc, repo.RoleEditor)
	})

	t.Run("access report shows the capped role", func(t *testing.T) {
		f := newInheritedOwnerFixture(t)
		f.grantOwnerGroup(t, "folder", f.folderF, f.mover)
		if _, err := f.store.MoveDocumentToFolder(ctx, f.mover, f.folderF, f.victimDoc); err != nil {
			t.Fatalf("move victim document: %v", err)
		}
		role, sources := f.effectiveRoleFor(t, "document", f.victimDoc, f.group)
		if role != repo.RoleEditor {
			t.Fatalf("group effective role on victim document = %q, want editor", role)
		}
		if got := sources[f.folderF]; got != repo.RoleEditor {
			t.Fatalf("group source role from folder = %q, want editor", got)
		}
	})
}

// The cap must not touch grants that genuinely belong to the resource's own
// canonical owner, nor any non-owner inherited role.
func TestInheritedGrantsUnaffectedByOwnershipCap(t *testing.T) {
	ctx := context.Background()

	t.Run("owner group grant on the folder owner's own document", func(t *testing.T) {
		f := newInheritedOwnerFixture(t)
		f.grantOwnerGroup(t, "folder", f.folderF, f.mover)
		if _, err := f.store.MoveDocumentToFolder(ctx, f.mover, f.folderF, f.moverDoc); err != nil {
			t.Fatalf("move mover document: %v", err)
		}
		f.assertDocRole(t, f.member, f.moverDoc, repo.RoleOwner)
		if role, _ := f.effectiveRoleFor(t, "document", f.moverDoc, f.group); role != repo.RoleOwner {
			t.Fatalf("group effective role on mover document = %q, want owner", role)
		}
	})

	t.Run("direct owner group grant on the document", func(t *testing.T) {
		f := newInheritedOwnerFixture(t)
		f.grantOwnerGroup(t, "document", f.victimDoc, f.victim)
		f.assertDocRole(t, f.member, f.victimDoc, repo.RoleOwner)
	})

	t.Run("nested folder keeps its own owner's group grant", func(t *testing.T) {
		f := newInheritedOwnerFixture(t)
		f.grantOwnerGroup(t, "folder", f.folderF, f.mover)
		f.grantOwnerGroup(t, "folder", f.victimF, f.victim)
		if _, err := f.store.UpdateFolder(ctx, f.victimF, "", &f.folderF); err != nil {
			t.Fatalf("re-parent victim folder: %v", err)
		}
		f.assertFolderRole(t, f.member, f.victimF, repo.RoleOwner)
		f.assertDocRole(t, f.member, f.nestedDoc, repo.RoleOwner)
		// The mover's folder itself is still owned by the mover, so the grant
		// on it is unchanged for the folder that carries it.
		f.assertFolderRole(t, f.member, f.folderF, repo.RoleOwner)
	})

	t.Run("non-owner inherited roles are unchanged", func(t *testing.T) {
		f := newInheritedOwnerFixture(t)
		for _, role := range []repo.Role{repo.RoleViewer, repo.RoleCommenter, repo.RoleEditor} {
			if _, err := f.store.CreateGrant(ctx, "folder", f.folderF, f.group, role, f.mover); err != nil {
				t.Fatalf("grant %s: %v", role, err)
			}
			if _, err := f.store.MoveDocumentToFolder(ctx, f.mover, f.folderF, f.victimDoc); err != nil {
				t.Fatalf("move victim document: %v", err)
			}
			f.assertDocRole(t, f.member, f.victimDoc, role)
		}
	})
}
