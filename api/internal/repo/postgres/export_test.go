package postgres

import "time"

// The blob sweep's bounds, for the tests in postgres_test that assert on them:
// the lease the claim query hands out has to outlast BlobSweepTimeout, and a
// sweep must never run more than BlobDeleteConcurrency deletes at once.
const (
	BlobSweepTimeout      = blobSweepTimeout
	BlobDeleteConcurrency = blobDeleteConcurrency
)

// SetBlobDeleteTimeout shortens the per-object deadline, so a test with a
// deliberately hung Delete does not have to wait out the production one.
func (s *Store) SetBlobDeleteTimeout(d time.Duration) { s.blobDeleteTimeout = d }
