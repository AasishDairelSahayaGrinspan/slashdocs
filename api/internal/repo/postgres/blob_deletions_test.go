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
	keys := queueBlobDeletions(t, store, "usr_sweep_locked", 3)

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
	keys := queueBlobDeletions(t, store, "usr_sweep_concurrent", 8)

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

// One object the store never answers for must cost its own slot and no more:
// the rest of the batch is deleted and dequeued, and the hung key comes back
// on a later sweep.
func TestPostgresSweepBlobDeletionsIsNotStalledByOneHungKey(t *testing.T) {
	ctx := context.Background()
	blobs := &hangingKeyBlobs{Store: blob.NewMemory()}
	store := newStoreWithBlobs(t, blobs)
	// The production per-object deadline is 10s; the sweep only has to prove
	// it applies one, so shorten it.
	store.SetBlobDeleteTimeout(100 * time.Millisecond)
	keys := queueBlobDeletions(t, store, "usr_sweep_hung", 5)
	blobs.hang(keys[0])

	swept := sweepInBackground(ctx, store)
	r := awaitSweep(t, swept, "one hung key stalled the whole batch")
	if r.cleared != len(keys)-1 {
		t.Fatalf("sweep cleared=%d err=%v, want %d", r.cleared, r.err, len(keys)-1)
	}
	if r.err == nil {
		t.Fatal("sweep reported no failure; the hung key's deadline must surface as one")
	}

	for _, key := range keys[1:] {
		if _, _, err := blobs.Get(ctx, key); !errors.Is(err, blob.ErrNotFound) {
			t.Errorf("blob %q behind the hung key survived the sweep (err=%v)", key, err)
		}
	}
	if _, _, err := blobs.Get(ctx, keys[0]); err != nil {
		t.Errorf("hung key's object %q removed: %v", keys[0], err)
	}
	assertQueuedBlobKeys(t, store, keys[:1])
}

// A sweep that runs out of time keeps the rows it never reached and still
// dequeues the objects it did delete: throwing those away would leave the next
// sweep deleting objects that are already gone.
func TestPostgresSweepBlobDeletionsHonoursItsDeadline(t *testing.T) {
	ctx, endSweep := context.WithCancel(context.Background())
	defer endSweep()
	const reachable = 3
	blobs := newBudgetedDeleteBlobs(reachable)
	store := newStoreWithBlobs(t, blobs)
	keys := queueBlobDeletions(t, store, "usr_sweep_deadline", 6)

	swept := sweepInBackground(ctx, store)
	// End the sweep's time the way its own deadline would, as soon as a key
	// finds the budget spent.
	select {
	case <-blobs.starved:
	case <-time.After(30 * time.Second):
		t.Fatal("the sweep never reached a key beyond its budget")
	}
	endSweep()

	r := awaitSweep(t, swept, "the sweep outlived the end of its context")
	// Every delete has to run under a deadline, and none may outlast a sweep.
	if unbounded, longest := blobs.deadlines(); unbounded {
		t.Fatal("an object delete ran on a context with no deadline")
	} else if longest > pgrepo.BlobSweepTimeout {
		t.Fatalf("an object delete had %s left, want at most one sweep's %s", longest, pgrepo.BlobSweepTimeout)
	}
	if r.cleared != reachable {
		t.Fatalf("sweep cleared=%d err=%v, want %d: the keys it did delete must still be dequeued", r.cleared, r.err, reachable)
	}

	deleted := blobs.deletedKeys()
	if len(deleted) != reachable {
		t.Fatalf("objects deleted = %v, want %d", deleted, reachable)
	}
	for _, key := range deleted {
		if _, _, err := blobs.Get(ctx, key); !errors.Is(err, blob.ErrNotFound) {
			t.Errorf("blob %q reported deleted but still in the store (err=%v)", key, err)
		}
	}
	assertQueuedBlobKeys(t, store, remaining(keys, deleted))
}

// Deleting a batch concurrently is what keeps a sweep inside its deadline, but
// the fan-out has to stay bounded rather than open the whole batch at once.
func TestPostgresSweepBlobDeletionsBoundsItsConcurrency(t *testing.T) {
	ctx := context.Background()
	blobs := newCountingDeleteBlobs(pgrepo.BlobDeleteConcurrency)
	defer blobs.releaseAll()
	store := newStoreWithBlobs(t, blobs)
	keys := queueBlobDeletions(t, store, "usr_sweep_bounded", 2*pgrepo.BlobDeleteConcurrency)

	swept := sweepInBackground(ctx, store)
	select {
	case <-blobs.full:
	case <-time.After(30 * time.Second):
		t.Fatalf("at most %d deletes ever ran at once, want %d: the sweep is not deleting concurrently",
			blobs.peak(), pgrepo.BlobDeleteConcurrency)
	}
	blobs.releaseAll()

	r := awaitSweep(t, swept, "the sweep never finished once its deletes were released")
	if r.err != nil || r.cleared != len(keys) {
		t.Fatalf("sweep cleared=%d err=%v, want %d and no error", r.cleared, r.err, len(keys))
	}
	if peak := blobs.peak(); peak <= 1 || peak > pgrepo.BlobDeleteConcurrency {
		t.Fatalf("deletes running at once peaked at %d, want more than 1 and at most %d", peak, pgrepo.BlobDeleteConcurrency)
	}
	assertQueuedBlobKeys(t, store, nil)
}

// hangingKeyBlobs never answers the delete of one key, and deletes every other
// one normally.
type hangingKeyBlobs struct {
	blob.Store
	mu  sync.Mutex
	key string
}

func (b *hangingKeyBlobs) hang(key string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.key = key
}

func (b *hangingKeyBlobs) Delete(ctx context.Context, key string) error {
	b.mu.Lock()
	hung := b.key == key
	b.mu.Unlock()
	if hung {
		<-ctx.Done()
		return ctx.Err()
	}
	return b.Store.Delete(ctx, key)
}

// budgetedDeleteBlobs deletes a fixed number of objects and leaves every other
// key waiting on the sweep's context, standing in for a sweep whose deadline
// arrives mid-batch.
type budgetedDeleteBlobs struct {
	blob.Store
	budget chan struct{}
	// starved reports the first key the budget did not cover.
	starved chan struct{}

	mu        sync.Mutex
	deleted   []string
	unbounded bool
	longest   time.Duration
}

func newBudgetedDeleteBlobs(budget int) *budgetedDeleteBlobs {
	b := &budgetedDeleteBlobs{Store: blob.NewMemory(), budget: make(chan struct{}, budget), starved: make(chan struct{}, 1)}
	for i := 0; i < budget; i++ {
		b.budget <- struct{}{}
	}
	return b
}

func (b *budgetedDeleteBlobs) Delete(ctx context.Context, key string) error {
	b.recordDeadline(ctx)
	select {
	case <-b.budget:
	default:
		select {
		case b.starved <- struct{}{}:
		default:
		}
		<-ctx.Done()
		return ctx.Err()
	}
	b.mu.Lock()
	b.deleted = append(b.deleted, key)
	b.mu.Unlock()
	return b.Store.Delete(ctx, key)
}

// recordDeadline notes how much time the sweep gave this delete, so a test can
// check that every one of them is bounded.
func (b *budgetedDeleteBlobs) recordDeadline(ctx context.Context) {
	deadline, ok := ctx.Deadline()
	b.mu.Lock()
	defer b.mu.Unlock()
	if !ok {
		b.unbounded = true
		return
	}
	if left := time.Until(deadline); left > b.longest {
		b.longest = left
	}
}

func (b *budgetedDeleteBlobs) deadlines() (unbounded bool, longest time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.unbounded, b.longest
}

func (b *budgetedDeleteBlobs) deletedKeys() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.deleted...)
}

// countingDeleteBlobs holds every delete until the test releases it, recording
// how many ran at once and reporting when limit of them are in flight.
type countingDeleteBlobs struct {
	blob.Store
	limit    int
	full     chan struct{}
	reported sync.Once
	release  chan struct{}
	freed    sync.Once

	mu     sync.Mutex
	active int
	peaked int
}

func newCountingDeleteBlobs(limit int) *countingDeleteBlobs {
	return &countingDeleteBlobs{Store: blob.NewMemory(), limit: limit, full: make(chan struct{}), release: make(chan struct{})}
}

func (b *countingDeleteBlobs) Delete(ctx context.Context, key string) error {
	b.mu.Lock()
	b.active++
	if b.active > b.peaked {
		b.peaked = b.active
	}
	atLimit := b.active >= b.limit
	b.mu.Unlock()
	if atLimit {
		b.reported.Do(func() { close(b.full) })
	}
	defer func() {
		b.mu.Lock()
		b.active--
		b.mu.Unlock()
	}()
	select {
	case <-b.release:
	case <-ctx.Done():
		return ctx.Err()
	}
	return b.Store.Delete(ctx, key)
}

func (b *countingDeleteBlobs) peak() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.peaked
}

// releaseAll lets every held delete finish, including when the test fails
// before it gets that far.
func (b *countingDeleteBlobs) releaseAll() { b.freed.Do(func() { close(b.release) }) }

// remaining returns the keys of all that are not in deleted.
func remaining(all, deleted []string) []string {
	gone := map[string]bool{}
	for _, key := range deleted {
		gone[key] = true
	}
	var left []string
	for _, key := range all {
		if !gone[key] {
			left = append(left, key)
		}
	}
	return left
}

// queueBlobDeletions deletes a document of versions blob keys, leaving exactly
// those keys queued in the outbox for a sweep to remove.
func queueBlobDeletions(t *testing.T, store *pgrepo.Store, ownerID string, versions int) []string {
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

	if err := store.DeleteDocument(ctx, doc.ID); err != nil {
		t.Fatal(err)
	}
	assertQueuedBlobKeys(t, store, keys)
	return keys
}

// sweepResult carries what a sweep returned to the test goroutine waiting on
// it, which runs the sweep in the background so it can be bounded.
type sweepResult struct {
	cleared int
	err     error
}

// sweepInBackground starts a sweep and returns the channel its result arrives
// on, so a test can fail on a sweep that never finishes instead of hanging.
func sweepInBackground(ctx context.Context, store *pgrepo.Store) <-chan sweepResult {
	swept := make(chan sweepResult, 1)
	go func() {
		cleared, err := store.SweepBlobDeletions(ctx)
		swept <- sweepResult{cleared: cleared, err: err}
	}()
	return swept
}

func awaitSweep(t *testing.T, swept <-chan sweepResult, stalled string) sweepResult {
	t.Helper()
	select {
	case r := <-swept:
		return r
	case <-time.After(30 * time.Second):
		t.Fatal(stalled)
		return sweepResult{}
	}
}
