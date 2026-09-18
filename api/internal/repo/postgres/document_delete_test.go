package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
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

// Blob storage being down must not fail the user's delete, and must not lose
// the objects either: the keys stay queued until a later sweep removes them.
func TestPostgresDeleteDocumentSurvivesBlobStorageOutage(t *testing.T) {
	ctx := context.Background()
	blobs := &scriptedBlobs{Store: blob.NewMemory()}
	store := newStoreWithBlobs(t, blobs)

	owner := auth.Principal{Type: auth.PrincipalUser, ID: "usr_delete_outage", Email: "delete-outage@example.com", Name: "Owner"}
	seedUsers(t, store, owner)
	doc, ver, err := store.CreateDocument(ctx, owner, "Outage", []byte("<html>v1</html>"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateVersion(ctx, doc.ID, ver.ID, "v2", []byte("<html>v2</html>"), owner); err != nil {
		t.Fatal(err)
	}
	keys := versionBlobKeys(t, store, doc.ID)

	blobs.failDeletes(errors.New("blob storage unavailable"))
	if err := store.DeleteDocument(ctx, doc.ID); err != nil {
		t.Fatalf("delete with blob storage down: %v", err)
	}
	if _, err := store.GetDocument(ctx, doc.ID); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("document after delete: err=%v, want not found", err)
	}
	assertQueuedBlobKeys(t, store, keys)
	for _, key := range keys {
		if _, _, err := blobs.Get(ctx, key); err != nil {
			t.Fatalf("blob %q removed while storage was down: %v", key, err)
		}
	}

	// A later sweep against a healthy store finishes the job.
	blobs.failDeletes(nil)
	cleared, err := store.SweepBlobDeletions(ctx)
	if err != nil || cleared != len(keys) {
		t.Fatalf("sweep cleared=%d err=%v, want %d and no error", cleared, err, len(keys))
	}
	for _, key := range keys {
		if _, _, err := blobs.Get(ctx, key); !errors.Is(err, blob.ErrNotFound) {
			t.Errorf("blob %q still in the store after the sweep (err=%v)", key, err)
		}
	}
	assertQueuedBlobKeys(t, store, nil)
}

// An object that is already gone is as deleted as one this sweep removed, so
// its row must not stay queued forever.
func TestPostgresDeleteDocumentTreatsMissingObjectAsDeleted(t *testing.T) {
	ctx := context.Background()
	blobs := &scriptedBlobs{Store: blob.NewMemory()}
	store := newStoreWithBlobs(t, blobs)

	owner := auth.Principal{Type: auth.PrincipalUser, ID: "usr_delete_missing", Email: "delete-missing@example.com", Name: "Owner"}
	seedUsers(t, store, owner)
	doc, _, err := store.CreateDocument(ctx, owner, "Already gone", []byte("<html>v1</html>"))
	if err != nil {
		t.Fatal(err)
	}

	blobs.failDeletes(fmt.Errorf("%w: delete", blob.ErrNotFound))
	if err := store.DeleteDocument(ctx, doc.ID); err != nil {
		t.Fatal(err)
	}
	assertQueuedBlobKeys(t, store, nil)
}

// The queued keys and the row delete are one transaction: a failure after the
// outbox insert must leave neither queued keys nor a half-deleted document.
func TestPostgresDeleteDocumentQueuesBlobKeysAtomically(t *testing.T) {
	ctx := context.Background()
	blobs := &scriptedBlobs{Store: blob.NewMemory()}
	store := newStoreWithBlobs(t, blobs)

	owner := auth.Principal{Type: auth.PrincipalUser, ID: "usr_delete_atomic", Email: "delete-atomic@example.com", Name: "Owner"}
	viewer := auth.Principal{Type: auth.PrincipalUser, ID: "usr_delete_atomic_viewer", Email: "delete-atomic-viewer@example.com", Name: "Viewer"}
	seedUsers(t, store, owner, viewer)
	doc, _, err := store.CreateDocument(ctx, owner, "Atomic", []byte("<html>v1</html>"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateGrant(ctx, "document", doc.ID, viewer, repo.RoleViewer, owner); err != nil {
		t.Fatal(err)
	}
	if err := store.PutPin(ctx, owner, "document", doc.ID); err != nil {
		t.Fatal(err)
	}
	keys := versionBlobKeys(t, store, doc.ID)

	// A deferred constraint trigger raises at COMMIT, so the delete fails only
	// after every statement in it, the outbox insert included, has run.
	exec(t, store, `CREATE FUNCTION reject_blob_deletions() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'blob deletion outbox unavailable'; END $$`)
	exec(t, store, `CREATE CONSTRAINT TRIGGER reject_blob_deletions AFTER INSERT ON blob_deletions
		DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION reject_blob_deletions()`)
	assertTriggerFiresAtCommit(t, store)

	if err := store.DeleteDocument(ctx, doc.ID); err == nil {
		t.Fatal("DeleteDocument succeeded; want the deferred trigger to fail the commit")
	}

	if _, err := store.GetDocument(ctx, doc.ID); err != nil {
		t.Fatalf("document after failed delete: err=%v, want it still there", err)
	}
	if got := versionBlobKeys(t, store, doc.ID); len(got) != len(keys) {
		t.Errorf("version rows after failed delete = %d, want %d", len(got), len(keys))
	}
	for _, key := range keys {
		if _, _, err := blobs.Get(ctx, key); err != nil {
			t.Errorf("blob %q deleted by a failed delete: %v", key, err)
		}
	}
	if n := countRows(t, store, `SELECT count(*) FROM resource_grants WHERE resource_type='document' AND resource_id=$1`, doc.ID); n != 1 {
		t.Errorf("resource_grants rows after failed delete = %d, want 1", n)
	}
	if n := countRows(t, store, `SELECT count(*) FROM pins WHERE resource_type='document' AND resource_id=$1`, doc.ID); n != 1 {
		t.Errorf("pins rows after failed delete = %d, want 1", n)
	}
	assertQueuedBlobKeys(t, store, nil)
}

// assertTriggerFiresAtCommit checks the premise of the test above: the
// deferred trigger lets the outbox insert through and rejects the commit, so
// the delete really does fail after the keys were queued.
func assertTriggerFiresAtCommit(t *testing.T, store *pgrepo.Store) {
	t.Helper()
	ctx := context.Background()
	probe, err := store.Pool().Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = probe.Rollback(ctx) }()
	if _, err := probe.Exec(ctx, `INSERT INTO blob_deletions(blob_key) VALUES('probe')`); err != nil {
		t.Fatalf("outbox insert already failed: %v; want the trigger to fire at commit", err)
	}
	if err := probe.Commit(ctx); err == nil {
		t.Fatal("probe commit succeeded; the trigger is not failing commits")
	}
}

// scriptedBlobs wraps a real store so a test can fail Delete on demand and
// inspect which keys the store was asked to delete.
type scriptedBlobs struct {
	blob.Store
	mu        sync.Mutex
	deleteErr error
	deleted   []string
}

func (b *scriptedBlobs) Delete(ctx context.Context, key string) error {
	b.mu.Lock()
	err := b.deleteErr
	b.deleted = append(b.deleted, key)
	b.mu.Unlock()
	if err != nil {
		return err
	}
	return b.Store.Delete(ctx, key)
}

// failDeletes makes every later Delete return err; nil restores the store.
func (b *scriptedBlobs) failDeletes(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.deleteErr = err
}

// forgetDeleted drops the recorded keys so a later phase of a test starts
// from a clean record.
func (b *scriptedBlobs) forgetDeleted() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.deleted = nil
}

func (b *scriptedBlobs) deletedKeys() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.deleted...)
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

func exec(t *testing.T, store *pgrepo.Store, query string, args ...any) {
	t.Helper()
	if _, err := store.Pool().Exec(context.Background(), query, args...); err != nil {
		t.Fatal(err)
	}
}

// assertQueuedBlobKeys compares the blob deletion outbox with want, order
// independent.
func assertQueuedBlobKeys(t *testing.T, store *pgrepo.Store, want []string) {
	t.Helper()
	rows, err := store.Pool().Query(context.Background(), `SELECT blob_key FROM blob_deletions`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	queued := map[string]bool{}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			t.Fatal(err)
		}
		queued[key] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(queued) != len(want) {
		t.Fatalf("queued blob keys = %v, want %v", queued, want)
	}
	for _, key := range want {
		if !queued[key] {
			t.Fatalf("blob key %q not queued; queued = %v", key, queued)
		}
	}
}
