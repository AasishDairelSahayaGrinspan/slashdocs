package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/razorpay/aidocs/api/internal/auth"
	"github.com/razorpay/aidocs/api/internal/blob"
	pgrepo "github.com/razorpay/aidocs/api/internal/repo/postgres"
)

// A sweep must step over rows another replica already holds instead of
// blocking on them, and must leave them queued for whoever holds the lock.
func TestPostgresSweepBlobDeletionsSkipsLockedRows(t *testing.T) {
	ctx := context.Background()
	blobs := &scriptedBlobs{Store: blob.NewMemory()}
	store := newStoreWithBlobs(t, blobs)
	keys := queueBlobDeletions(t, store, blobs, "usr_sweep_locked", 3)

	// Hold one queued row the way a concurrent sweeper's claim would.
	holder, err := store.Pool().Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holder.Rollback(ctx) }()
	var locked string
	if err := holder.QueryRow(ctx, `SELECT blob_key FROM blob_deletions ORDER BY blob_key LIMIT 1 FOR UPDATE`).Scan(&locked); err != nil {
		t.Fatal(err)
	}

	// Without SKIP LOCKED the claim would block on the held row until the
	// timeout instead of clearing the rest.
	sweepCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cleared, err := store.SweepBlobDeletions(sweepCtx)
	if err != nil || cleared != len(keys)-1 {
		t.Fatalf("sweep cleared=%d err=%v, want %d and no error", cleared, err, len(keys)-1)
	}
	assertQueuedBlobKeys(t, store, []string{locked})

	// Once the lock is gone the held row is swept like any other.
	if err := holder.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if cleared, err := store.SweepBlobDeletions(ctx); err != nil || cleared != 1 {
		t.Fatalf("second sweep cleared=%d err=%v, want 1 and no error", cleared, err)
	}
	assertQueuedBlobKeys(t, store, nil)
}

// Two sweepers running at once must share the queue, not duplicate it.
func TestPostgresSweepBlobDeletionsDoesNotDoubleProcess(t *testing.T) {
	ctx := context.Background()
	blobs := &scriptedBlobs{Store: blob.NewMemory()}
	store := newStoreWithBlobs(t, blobs)
	keys := queueBlobDeletions(t, store, blobs, "usr_sweep_concurrent", 8)

	var wg sync.WaitGroup
	cleared := make([]int, 2)
	failures := make([]error, 2)
	for i := range cleared {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cleared[i], failures[i] = store.SweepBlobDeletions(ctx)
		}()
	}
	wg.Wait()

	for i, err := range failures {
		if err != nil {
			t.Fatalf("sweeper %d: %v", i, err)
		}
	}
	if total := cleared[0] + cleared[1]; total != len(keys) {
		t.Fatalf("sweepers cleared %d rows (%v), want %d", total, cleared, len(keys))
	}
	seen := map[string]int{}
	for _, key := range blobs.deletedKeys() {
		seen[key]++
		if seen[key] > 1 {
			t.Fatalf("blob %q deleted %d times; concurrent sweepers claimed the same row", key, seen[key])
		}
	}
	assertQueuedBlobKeys(t, store, nil)
}

// queueBlobDeletions deletes a document of versions blob keys while blob
// storage is down, leaving exactly those keys queued in the outbox.
func queueBlobDeletions(t *testing.T, store *pgrepo.Store, blobs *scriptedBlobs, ownerID string, versions int) []string {
	t.Helper()
	ctx := context.Background()
	owner := auth.Principal{Type: auth.PrincipalUser, ID: ownerID, Email: ownerID + "@example.com", Name: "Owner"}
	seedUsers(t, store, owner)
	doc, ver, err := store.CreateDocument(ctx, owner, "Queued", []byte("<html>v1</html>"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < versions; i++ {
		if ver, err = store.CreateVersion(ctx, doc.ID, ver.ID, "next", []byte("<html>next</html>"), owner); err != nil {
			t.Fatal(err)
		}
	}
	keys := versionBlobKeys(t, store, doc.ID)
	if len(keys) != versions {
		t.Fatalf("version blob keys = %d, want %d", len(keys), versions)
	}

	blobs.failDeletes(errors.New("blob storage unavailable"))
	if err := store.DeleteDocument(ctx, doc.ID); err != nil {
		t.Fatal(err)
	}
	blobs.failDeletes(nil)
	blobs.forgetDeleted()
	assertQueuedBlobKeys(t, store, keys)
	return keys
}
