package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/razorpay/aidocs/api/internal/auth"
	"github.com/razorpay/aidocs/api/internal/blob"
	"github.com/razorpay/aidocs/api/internal/repo"
	pgrepo "github.com/razorpay/aidocs/api/internal/repo/postgres"
	"github.com/razorpay/aidocs/api/internal/testdb"
)

// Deleting a document must remove the content the user asked to remove: every
// version's HTML object, plus the grants and pins that reference the document
// by a generic (resource_type, resource_id) pair no foreign key covers.
func TestPostgresDeleteDocumentRemovesBlobsGrantsAndPins(t *testing.T) {
	ctx := context.Background()
	blobs := blob.NewMemory()
	store := newStoreWithBlobs(t, blobs)

	owner := auth.Principal{Type: auth.PrincipalUser, ID: "usr_delete_owner", Email: "delete-owner@example.com", Name: "Owner"}
	viewer := auth.Principal{Type: auth.PrincipalUser, ID: "usr_delete_viewer", Email: "delete-viewer@example.com", Name: "Viewer"}
	seedUsers(t, store, owner, viewer)

	doc, ver, err := store.CreateDocument(ctx, owner, "Delete me", []byte("<html>v1</html>"))
	if err != nil {
		t.Fatal(err)
	}
	ver2, err := store.CreateVersion(ctx, doc.ID, ver.ID, "v2", []byte("<html>v2</html>"), owner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateVersion(ctx, doc.ID, ver2.ID, "v3", []byte("<html>v3</html>"), owner); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateGrant(ctx, "document", doc.ID, viewer, repo.RoleViewer, owner); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateGrant(ctx, "document", doc.ID, auth.Principal{Type: auth.PrincipalAnyone}, repo.RoleViewer, owner); err != nil {
		t.Fatal(err)
	}
	for _, p := range []auth.Principal{owner, viewer} {
		if err := store.PutPin(ctx, p, "document", doc.ID); err != nil {
			t.Fatal(err)
		}
	}

	keys := versionBlobKeys(t, store, doc.ID)
	if len(keys) != 3 {
		t.Fatalf("version blob keys = %d, want 3", len(keys))
	}
	for _, key := range keys {
		if _, _, err := blobs.Get(ctx, key); err != nil {
			t.Fatalf("blob %q missing before delete: %v", key, err)
		}
	}

	if err := store.DeleteDocument(ctx, doc.ID); err != nil {
		t.Fatal(err)
	}

	for _, key := range keys {
		if _, _, err := blobs.Get(ctx, key); !errors.Is(err, blob.ErrNotFound) {
			t.Errorf("blob %q still in the store after delete (err=%v)", key, err)
		}
	}
	if n := countRows(t, store, `SELECT count(*) FROM resource_grants WHERE resource_type='document' AND resource_id=$1`, doc.ID); n != 0 {
		t.Errorf("resource_grants rows for deleted document = %d, want 0", n)
	}
	if n := countRows(t, store, `SELECT count(*) FROM pins WHERE resource_type='document' AND resource_id=$1`, doc.ID); n != 0 {
		t.Errorf("pins rows for deleted document = %d, want 0", n)
	}
}

// newStoreWithBlobs connects a Store backed by blobs to a fresh test database.
func newStoreWithBlobs(t *testing.T, blobs blob.Store) *pgrepo.Store {
	t.Helper()
	store, err := pgrepo.ConnectWithBlob(context.Background(), testdb.New(t), blobs)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	return store
}

// seedUsers inserts user rows so grants and pins resolve against real users.
func seedUsers(t *testing.T, store *pgrepo.Store, principals ...auth.Principal) {
	t.Helper()
	for _, p := range principals {
		if _, err := store.UpsertGoogleUser(context.Background(), p.ID, p.Email, p.Name, "sub-"+p.ID, ""); err != nil {
			t.Fatalf("seed user %s: %v", p.ID, err)
		}
	}
}

// versionBlobKeys reads the blob keys of a document's versions straight from
// the database, before a delete cascades the rows that record them.
func versionBlobKeys(t *testing.T, store *pgrepo.Store, documentID string) []string {
	t.Helper()
	rows, err := store.Pool().Query(context.Background(), `SELECT html_blob_key FROM versions WHERE document_id=$1 ORDER BY number`, documentID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			t.Fatal(err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return keys
}

func countRows(t *testing.T, store *pgrepo.Store, query string, args ...any) int {
	t.Helper()
	var n int
	if err := store.Pool().QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
