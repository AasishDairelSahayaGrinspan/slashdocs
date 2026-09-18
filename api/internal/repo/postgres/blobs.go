package postgres

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/razorpay/aidocs/api/internal/blob"
)

// Version HTML lives only in blob storage, and deleting a document cascades the
// rows that hold its keys. The keys are therefore queued in the blob_deletions
// outbox inside the deleting transaction, and every object deletion belongs to
// the sweeper: the request path only commits the keys and wakes it, so no user
// waits on the object store. Rows are removed only once the object is gone;
// they are never expired, because expiry would strand the object for good.
const (
	// blobDeletionBatch is how many queued keys one sweep claims.
	blobDeletionBatch = 100
	// blobSweepTimeout bounds a whole sweep — the claim, the deletes and the
	// dequeue — so a hung object store cannot leave the sweeper stuck and
	// every later tick blocked. ClaimBlobDeletions leases claimed rows for
	// longer than this, so no other replica retries a row this sweep still
	// holds.
	blobSweepTimeout = 45 * time.Second
	// blobDeleteTimeout bounds one object deletion, so a single hung key
	// costs its own slot rather than the whole batch.
	blobDeleteTimeout = 10 * time.Second
	// blobDeleteConcurrency is how many objects a sweep deletes at once.
	blobDeleteConcurrency = 8
	// blobDequeueTimeout bounds the dequeue of keys whose objects are gone.
	// It runs detached from the sweep's context, so a deadline that passes
	// mid-sweep does not throw away finished work for the next sweep to redo.
	blobDequeueTimeout = 5 * time.Second
)

// BlobDeletionsQueued reports commits that queued blob keys, so the sweeper on
// this replica can run at once instead of waiting for its next tick. The
// channel holds a single pending signal: one is enough to ask for a sweep.
func (s *Store) BlobDeletionsQueued() <-chan struct{} { return s.blobDeletionsQueued }

// signalBlobDeletions wakes the sweeper without ever blocking the caller, whose
// delete has already succeeded.
func (s *Store) signalBlobDeletions() {
	select {
	case s.blobDeletionsQueued <- struct{}{}:
	default:
		// A signal is already pending, and one sweep covers both deletes.
	}
}

// SweepBlobDeletions claims a batch of due blob deletions, deletes the objects,
// and returns how many outbox rows it cleared. Claiming leases rows with FOR
// UPDATE SKIP LOCKED, so replicas sweep disjoint batches, and a claim that
// fails is retried with backoff rather than lost.
func (s *Store) SweepBlobDeletions(ctx context.Context) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, s.blobSweepTimeout)
	defer cancel()
	keys, err := s.q.ClaimBlobDeletions(ctx, blobDeletionBatch)
	if err != nil || len(keys) == 0 {
		return 0, err
	}
	return s.deleteBlobs(ctx, keys)
}

// deleteBlobs deletes the objects, a bounded number at a time, and dequeues the
// ones that are gone, returning how many rows it cleared along with any
// deletion failures. An object that is already missing counts as deleted: S3
// reports a repeated delete as success, and a store that reports
// blob.ErrNotFound means the same. Keys the sweep's deadline cuts short keep
// their rows and come back on a later sweep.
func (s *Store) deleteBlobs(ctx context.Context, keys []string) (int, error) {
	var (
		mu       sync.Mutex
		deleted  = make([]string, 0, len(keys))
		failures error
		wg       sync.WaitGroup
		slots    = make(chan struct{}, blobDeleteConcurrency)
	)
claim:
	for _, key := range keys {
		if ctx.Err() != nil {
			break
		}
		select {
		case slots <- struct{}{}:
		case <-ctx.Done():
			break claim
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			err := s.deleteBlob(ctx, key)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failures = errors.Join(failures, err)
				return
			}
			deleted = append(deleted, key)
		}()
	}
	wg.Wait()
	if len(deleted) == 0 {
		return 0, failures
	}
	// The objects are gone whether or not the sweep still has time left, so
	// the dequeue detaches from its context, as the rollback compensation in
	// postgres.go does.
	dequeueCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), blobDequeueTimeout)
	defer cancel()
	if err := s.q.DeleteBlobDeletions(dequeueCtx, deleted); err != nil {
		return 0, errors.Join(failures, err)
	}
	return len(deleted), failures
}

// deleteBlob removes one object under its own deadline, so a Delete the store
// never answers cannot hold its slot for the rest of the sweep.
func (s *Store) deleteBlob(ctx context.Context, key string) error {
	ctx, cancel := context.WithTimeout(ctx, s.blobDeleteTimeout)
	defer cancel()
	if err := s.blobs.Delete(ctx, key); err != nil && !errors.Is(err, blob.ErrNotFound) {
		return err
	}
	return nil
}
