-- name: EnqueueBlobDeletions :exec
-- Queues blob keys inside the caller's transaction, so a committed row delete
-- can never lose the keys of the objects it just orphaned.
INSERT INTO blob_deletions(blob_key)
SELECT unnest(@blob_keys::text[])
ON CONFLICT (blob_key) DO NOTHING;

-- name: DeleteBlobDeletions :exec
DELETE FROM blob_deletions WHERE blob_key = ANY(@blob_keys::text[]);

-- name: ClaimBlobDeletions :many
-- Leases a batch of due deletions. FOR UPDATE SKIP LOCKED keeps concurrent
-- sweepers on disjoint batches, and the new available_at both holds the lease
-- and applies the retry backoff (2^attempts seconds, capped at 256).
UPDATE blob_deletions b
SET attempts=b.attempts+1,
    available_at=now()+power(2, least(b.attempts+1, 8))*interval '1 second'
FROM (
  SELECT blob_key FROM blob_deletions WHERE available_at<=now()
  ORDER BY available_at FOR UPDATE SKIP LOCKED LIMIT @row_limit
) pending
WHERE b.blob_key=pending.blob_key
RETURNING b.blob_key;
