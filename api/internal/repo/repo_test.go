package repo_test

import (
	"context"
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
