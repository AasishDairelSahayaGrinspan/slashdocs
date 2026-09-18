package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/razorpay/aidocs/api/internal/auth"
	"github.com/razorpay/aidocs/api/internal/repo"
	pgrepo "github.com/razorpay/aidocs/api/internal/repo/postgres"
)

// TestPostgresFolderInvariants exercises the folder rules (inheritance,
// hierarchy limits, pins) directly against the repository.
func TestPostgresFolderInvariants(t *testing.T) {
	ctx := context.Background()
	owner := auth.Principal{Type: auth.PrincipalUser, ID: "u_owner", Email: "folder-owner@example.com", Name: "Owner"}
	other := auth.Principal{Type: auth.PrincipalUser, ID: "u_other", Email: "folder-other@example.com", Name: "Other"}
	third := auth.Principal{Type: auth.PrincipalUser, ID: "u_third", Email: "folder-third@example.com", Name: "Third"}

	// newFolderStore hands each subtest an isolated database with the three
	// principals present, so grants and pins resolve against real user rows.
	newFolderStore := func(t *testing.T) *pgrepo.Store {
		t.Helper()
		store := newStore(t)
		for _, p := range []auth.Principal{owner, other, third} {
			if _, err := store.UpsertGoogleUser(ctx, p.ID, p.Email, p.Name, "sub-"+p.ID, ""); err != nil {
				t.Fatalf("seed user %s: %v", p.ID, err)
			}
		}
		return store
	}

	t.Run("additive inheritance", func(t *testing.T) {
		r := newFolderStore(t)
		root, _ := r.CreateFolder(ctx, owner, "Root", "")
		doc, _, _ := r.CreateDocument(ctx, owner, "Doc", []byte("x"))
		if _, err := r.CreateGrant(ctx, "folder", root.ID, other, repo.RoleViewer, owner); err != nil {
			t.Fatal(err)
		}
		if _, err := r.MoveDocumentToFolder(ctx, owner, root.ID, doc.ID); err != nil {
			t.Fatal(err)
		}
		role, err := r.RoleForDocument(ctx, other, doc.ID)
		if err != nil || role != repo.RoleViewer {
			t.Fatalf("role=%q err=%v", role, err)
		}
	})
	t.Run("nested inheritance", func(t *testing.T) {
		r := newFolderStore(t)
		root, _ := r.CreateFolder(ctx, owner, "Root", "")
		child, _ := r.CreateFolder(ctx, owner, "Child", root.ID)
		doc, _, _ := r.CreateDocument(ctx, owner, "Doc", []byte("x"))
		if _, err := r.CreateGrant(ctx, "folder", root.ID, other, repo.RoleCommenter, owner); err != nil {
			t.Fatal(err)
		}
		if _, err := r.MoveDocumentToFolder(ctx, owner, child.ID, doc.ID); err != nil {
			t.Fatal(err)
		}
		role, err := r.RoleForDocument(ctx, other, doc.ID)
		if err != nil || role != repo.RoleCommenter {
			t.Fatalf("role=%q err=%v", role, err)
		}
	})
	t.Run("monotonic invariant", func(t *testing.T) {
		r := newFolderStore(t)
		root, _ := r.CreateFolder(ctx, owner, "Root", "")
		child, _ := r.CreateFolder(ctx, owner, "Child", root.ID)
		doc, _, _ := r.CreateDocument(ctx, owner, "Doc", []byte("x"))
		if _, err := r.CreateGrant(ctx, "folder", root.ID, other, repo.RoleEditor, owner); err != nil {
			t.Fatal(err)
		}
		if _, err := r.CreateGrant(ctx, "document", doc.ID, other, repo.RoleViewer, owner); err != nil {
			t.Fatal(err)
		}
		if _, err := r.MoveDocumentToFolder(ctx, owner, child.ID, doc.ID); err != nil {
			t.Fatal(err)
		}
		role, _ := r.RoleForDocument(ctx, other, doc.ID)
		if role != repo.RoleEditor {
			t.Fatalf("role=%q", role)
		}
	})
	t.Run("reduce via ancestor provenance sources", func(t *testing.T) {
		r := newFolderStore(t)
		root, _ := r.CreateFolder(ctx, owner, "Root", "")
		doc, _, _ := r.CreateDocument(ctx, owner, "Doc", []byte("x"))
		if _, err := r.CreateGrant(ctx, "folder", root.ID, other, repo.RoleViewer, owner); err != nil {
			t.Fatal(err)
		}
		if _, err := r.MoveDocumentToFolder(ctx, owner, root.ID, doc.ID); err != nil {
			t.Fatal(err)
		}
		items, _ := r.ListEffectiveAccess(ctx, "document", doc.ID)
		found := false
		for _, it := range items {
			if it.Principal.ID == other.ID {
				for _, s := range it.Sources {
					if s.ResourceType == "folder" && s.ResourceID == root.ID {
						found = true
					}
				}
			}
		}
		if !found {
			t.Fatalf("missing folder provenance: %#v", items)
		}
	})
	t.Run("effective access hydrates canonical owner", func(t *testing.T) {
		r := newFolderStore(t)
		doc, _, err := r.CreateDocument(ctx, owner, "Owned doc", []byte("x"))
		if err != nil {
			t.Fatal(err)
		}
		items, err := r.ListEffectiveAccess(ctx, "document", doc.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range items {
			if item.EffectiveRole == repo.RoleOwner && item.Principal.ID == owner.ID {
				if item.Principal.Name != owner.Name || item.Principal.Email != owner.Email {
					t.Fatalf("owner profile = %#v, want name %q and email %q", item.Principal, owner.Name, owner.Email)
				}
				return
			}
		}
		t.Fatalf("canonical owner missing from effective access: %#v", items)
	})
	t.Run("cycle prevention", func(t *testing.T) {
		r := newFolderStore(t)
		a, _ := r.CreateFolder(ctx, owner, "A", "")
		b, _ := r.CreateFolder(ctx, owner, "B", a.ID)
		_, err := r.UpdateFolder(ctx, a.ID, "", &b.ID)
		if !errors.Is(err, repo.ErrConflict) {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("depth limit", func(t *testing.T) {
		r := newFolderStore(t)
		p := ""
		var deepest repo.Folder
		for i := 0; i < 15; i++ {
			f, err := r.CreateFolder(ctx, owner, fmt.Sprintf("L%d", i), p)
			if err != nil {
				t.Fatal(err)
			}
			deepest = f
			p = f.ID
		}
		got, err := r.GetFolder(ctx, deepest.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Ancestors) != 14 {
			t.Fatalf("deepest folder has %d ancestors, want 14", len(got.Ancestors))
		}
		if _, err := r.CreateFolder(ctx, owner, "TooDeep", p); !errors.Is(err, repo.ErrConflict) {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("sibling title uniqueness", func(t *testing.T) {
		r := newFolderStore(t)
		if _, err := r.CreateFolder(ctx, owner, "A", ""); err != nil {
			t.Fatal(err)
		}
		if _, err := r.CreateFolder(ctx, owner, "a", ""); !errors.Is(err, repo.ErrConflict) {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("pin access validation", func(t *testing.T) {
		r := newFolderStore(t)
		root, _ := r.CreateFolder(ctx, owner, "Root", "")
		doc, _, _ := r.CreateDocument(ctx, owner, "Doc", []byte("x"))
		if _, err := r.MoveDocumentToFolder(ctx, owner, root.ID, doc.ID); err != nil {
			t.Fatal(err)
		}
		if role, _ := r.RoleForDocument(ctx, third, doc.ID); role != repo.RoleNone {
			t.Fatalf("unexpected role %q", role)
		}
		if _, err := r.CreateGrant(ctx, "folder", root.ID, third, repo.RoleViewer, owner); err != nil {
			t.Fatal(err)
		}
		if role, _ := r.RoleForDocument(ctx, third, doc.ID); role != repo.RoleViewer {
			t.Fatalf("inherited role %q", role)
		}
		if err := r.PutPin(ctx, third, "document", doc.ID); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("per principal pin isolation", func(t *testing.T) {
		r := newFolderStore(t)
		f1, _ := r.CreateFolder(ctx, owner, "F1", "")
		f2, _ := r.CreateFolder(ctx, other, "F2", "")
		if err := r.PutPin(ctx, owner, "folder", f1.ID); err != nil {
			t.Fatal(err)
		}
		if err := r.PutPin(ctx, other, "folder", f2.ID); err != nil {
			t.Fatal(err)
		}
		pins, _ := r.ListPins(ctx, owner)
		if len(pins) != 1 || pins[0].ResourceID != f1.ID {
			t.Fatalf("pins=%#v", pins)
		}
	})
	// Grants and pins reference folders by a generic (resource_type,
	// resource_id) pair no foreign key covers, so deleting a subtree has to
	// remove them for every folder it deletes. Documents inside are moved to
	// root instead of deleted and keep their own grants.
	t.Run("subtree delete removes grants and pins", func(t *testing.T) {
		r := newFolderStore(t)
		root, err := r.CreateFolder(ctx, owner, "Root", "")
		if err != nil {
			t.Fatal(err)
		}
		child, err := r.CreateFolder(ctx, owner, "Child", root.ID)
		if err != nil {
			t.Fatal(err)
		}
		doc, _, err := r.CreateDocument(ctx, owner, "Doc", []byte("x"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := r.MoveDocumentToFolder(ctx, owner, child.ID, doc.ID); err != nil {
			t.Fatal(err)
		}
		for _, folderID := range []string{root.ID, child.ID} {
			if _, err := r.CreateGrant(ctx, "folder", folderID, other, repo.RoleViewer, owner); err != nil {
				t.Fatal(err)
			}
			if err := r.PutPin(ctx, owner, "folder", folderID); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := r.CreateGrant(ctx, "document", doc.ID, third, repo.RoleViewer, owner); err != nil {
			t.Fatal(err)
		}

		if err := r.DeleteFolder(ctx, root.ID, owner); err != nil {
			t.Fatal(err)
		}

		deleted := []string{root.ID, child.ID}
		if n := countRows(t, r, `SELECT count(*) FROM resource_grants WHERE resource_type='folder' AND resource_id=ANY($1)`, deleted); n != 0 {
			t.Errorf("resource_grants rows for deleted folders = %d, want 0", n)
		}
		if n := countRows(t, r, `SELECT count(*) FROM pins WHERE resource_type='folder' AND resource_id=ANY($1)`, deleted); n != 0 {
			t.Errorf("pins rows for deleted folders = %d, want 0", n)
		}
		moved, err := r.GetDocument(ctx, doc.ID)
		if err != nil || moved.ParentFolderID != "" {
			t.Fatalf("document after folder delete = %+v, err=%v, want it at root", moved, err)
		}
		if role, _ := r.RoleForDocument(ctx, third, doc.ID); role != repo.RoleViewer {
			t.Errorf("document grant role after folder delete = %q, want viewer", role)
		}
	})
}
