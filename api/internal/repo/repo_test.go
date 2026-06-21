package repo_test

import (
	"context"
	"errors"
	"testing"

	"github.com/razorpay/aidocs/api/internal/auth"
	"github.com/razorpay/aidocs/api/internal/repo"
)

func TestUpsertDriveDocument_DedupesByFileID(t *testing.T) {
	m := repo.NewMemory()
	ctx := context.Background()
	owner := auth.Principal{Type: "user", ID: "u1"}

	d1, err := m.UpsertDriveDocument(ctx, owner, "file-abc", "", "Report")
	if err != nil {
		t.Fatal(err)
	}
	if d1.Source != repo.SourceDrive || d1.DriveFileID != "file-abc" || d1.CurrentVersionID != "" {
		t.Fatalf("unexpected drive doc: %+v", d1)
	}

	d2, err := m.UpsertDriveDocument(ctx, owner, "file-abc", "", "Report v2")
	if err != nil {
		t.Fatal(err)
	}
	if d2.ID != d1.ID {
		t.Fatalf("expected same doc id on re-open, got %s vs %s", d2.ID, d1.ID)
	}
	if d2.Title != "Report v2" {
		t.Fatalf("expected title updated, got %q", d2.Title)
	}
}

// TestUpsertDriveDocument_UpdatesResourceKey guards the resource-key refresh on
// re-open: a later call with a non-empty resource key must overwrite the prior
// one, since Drive resource keys can rotate.
func TestUpsertDriveDocument_UpdatesResourceKey(t *testing.T) {
	m := repo.NewMemory()
	ctx := context.Background()
	owner := auth.Principal{Type: "user", ID: "u1"}

	d1, err := m.UpsertDriveDocument(ctx, owner, "file-xyz", "rk-old", "Doc")
	if err != nil {
		t.Fatal(err)
	}
	if d1.DriveResourceKey != "rk-old" {
		t.Fatalf("expected resource key set, got %q", d1.DriveResourceKey)
	}
	d2, err := m.UpsertDriveDocument(ctx, owner, "file-xyz", "rk-new", "Doc")
	if err != nil {
		t.Fatal(err)
	}
	if d2.DriveResourceKey != "rk-new" {
		t.Fatalf("expected resource key updated, got %q", d2.DriveResourceKey)
	}
}

// TestCreateDocument_SetsNativeSource confirms the existing native path stamps
// Source=SourceNative so reads round-trip the source discriminator.
func TestCreateDocument_SetsNativeSource(t *testing.T) {
	m := repo.NewMemory()
	ctx := context.Background()
	owner := auth.Principal{Type: "user", ID: "u1"}

	d, _, err := m.CreateDocument(ctx, owner, "Native", []byte("<html></html>"))
	if err != nil {
		t.Fatal(err)
	}
	if d.Source != repo.SourceNative {
		t.Fatalf("expected native source, got %q", d.Source)
	}
}

// TestRoleForDocument_DriveDocNonOwnerGetsViewerCommenterFloor guards the C1
// fix: a Drive-backed document is gated by Google Drive per-viewer ACLs, not
// aidocs grants. A second, non-owner principal (user B) who opens the same
// Drive file must be treated as at least a commenter (so they can view and
// comment) while remaining strictly below RoleOwner (so delete/patch/grants/
// transfer stay blocked). The doc's first opener (user A) stays the owner.
func TestRoleForDocument_DriveDocNonOwnerGetsViewerCommenterFloor(t *testing.T) {
	m := repo.NewMemory()
	ctx := context.Background()
	ownerA := auth.Principal{Type: auth.PrincipalUser, ID: "u_a"}
	userB := auth.Principal{Type: auth.PrincipalUser, ID: "u_b"}

	doc, err := m.UpsertDriveDocument(ctx, ownerA, "file-drive-1", "", "Drive report")
	if err != nil {
		t.Fatal(err)
	}

	// The first opener keeps full ownership.
	roleA, err := m.RoleForDocument(ctx, ownerA, doc.ID)
	if err != nil || roleA != repo.RoleOwner {
		t.Fatalf("owner role = %s err = %v, want owner", roleA, err)
	}

	// A different, non-owner principal gets the Drive viewer/commenter floor:
	// high enough to view and comment, strictly below RoleOwner.
	roleB, err := m.RoleForDocument(ctx, userB, doc.ID)
	if err != nil {
		t.Fatalf("non-owner role lookup err = %v, want a role", err)
	}
	if roleB < repo.DriveViewerFloor {
		t.Fatalf("non-owner drive role = %s, want at least %s", roleB, repo.DriveViewerFloor)
	}
	if roleB >= repo.RoleOwner {
		t.Fatalf("non-owner drive role = %s, must stay below %s (no owner-only ops)", roleB, repo.RoleOwner)
	}
	// The floor must specifically permit comment creation (RoleCommenter).
	if roleB < repo.RoleCommenter {
		t.Fatalf("non-owner drive role = %s, want at least commenter to permit commenting", roleB)
	}
}

// TestRoleForDocument_NativeDocNonOwnerStillBlocked is the regression guard:
// the Drive floor must apply ONLY to Drive-backed docs. A native doc owned by
// user A must still yield no role (ErrNotFound) for a non-owner without a
// grant, so native access control is unchanged.
func TestRoleForDocument_NativeDocNonOwnerStillBlocked(t *testing.T) {
	m := repo.NewMemory()
	ctx := context.Background()
	ownerA := auth.Principal{Type: auth.PrincipalUser, ID: "u_a"}
	userB := auth.Principal{Type: auth.PrincipalUser, ID: "u_b"}

	d, _, err := m.CreateDocument(ctx, ownerA, "Native", []byte("<html></html>"))
	if err != nil {
		t.Fatal(err)
	}
	if d.Source != repo.SourceNative {
		t.Fatalf("expected native source, got %q", d.Source)
	}

	roleB, err := m.RoleForDocument(ctx, userB, d.ID)
	if !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("non-owner native role = %s err = %v, want ErrNotFound (no grant => blocked)", roleB, err)
	}
	if roleB != repo.RoleNone {
		t.Fatalf("non-owner native role = %s, want RoleNone", roleB)
	}

	// The owner of the native doc is unaffected.
	roleA, err := m.RoleForDocument(ctx, ownerA, d.ID)
	if err != nil || roleA != repo.RoleOwner {
		t.Fatalf("native owner role = %s err = %v, want owner", roleA, err)
	}
}
