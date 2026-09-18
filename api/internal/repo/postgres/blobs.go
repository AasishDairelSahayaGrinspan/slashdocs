package postgres

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/razorpay/aidocs/api/internal/blob"
)

// Version HTML lives only in blob storage, and deleting a document cascades the
// rows that hold its keys. The keys are therefore queued in the blob_deletions
// outbox inside the deleting transaction: the attempt made right after the
// commit removes almost all of them, and SweepBlobDeletions retries whatever
// the blob store refused. Rows are removed only once the object is gone; they
// are never expired, because expiry would strand the object for good.
const (
	// blobDeletionTimeout bounds the post-commit attempt, which runs on a
	// context detached from the caller's.
	blobDeletionTimeout = 15 * time.Second
	// blobDeletionBatch is how many queued keys one sweep claims.
	blobDeletionBatch = 100
)

// deleteQueuedBlobs removes the objects for keys already queued by a committed
// transaction and clears their outbox rows. The user-visible delete has
// succeeded by now, so failures are logged rather than returned: the keys stay
// queued and the sweeper retries them.
func (s *Store) deleteQueuedBlobs(ctx context.Context, keys []string) {
	if len(keys) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), blobDeletionTimeout)
	defer cancel()
	if _, err := s.deleteBlobs(ctx, keys); err != nil {
		slog.Warn("queued blob deletion incomplete; retrying in the background",
			slog.Int("keys", len(keys)), slog.Any("error", err))
	}
}

// SweepBlobDeletions claims a batch of due blob deletions, deletes the objects,
// and returns how many outbox rows it cleared. Claiming leases rows with FOR
// UPDATE SKIP LOCKED, so replicas sweep disjoint batches, and a claim that
// fails is retried with backoff rather than lost.
func (s *Store) SweepBlobDeletions(ctx context.Context) (int, error) {
	keys, err := s.q.ClaimBlobDeletions(ctx, blobDeletionBatch)
	if err != nil || len(keys) == 0 {
		return 0, err
	}
	return s.deleteBlobs(ctx, keys)
}

// deleteBlobs deletes each object and dequeues the ones that are gone,
// returning how many rows it cleared along with any deletion failures. An
// object that is already missing counts as deleted: S3 reports a repeated
// delete as success, and a store that reports blob.ErrNotFound means the same.
func (s *Store) deleteBlobs(ctx context.Context, keys []string) (int, error) {
	deleted := make([]string, 0, len(keys))
	var failures error
	for _, key := range keys {
		if err := s.blobs.Delete(ctx, key); err != nil && !errors.Is(err, blob.ErrNotFound) {
			failures = errors.Join(failures, err)
			continue
		}
		deleted = append(deleted, key)
	}
	if len(deleted) == 0 {
		return 0, failures
	}
	if err := s.q.DeleteBlobDeletions(ctx, deleted); err != nil {
		return 0, errors.Join(failures, err)
	}
	return len(deleted), failures
}
