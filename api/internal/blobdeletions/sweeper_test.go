package blobdeletions_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/razorpay/aidocs/api/internal/blobdeletions"
)

// fakeStore returns the scripted results of successive sweeps and counts how
// many times it was called.
type fakeStore struct {
	mu      sync.Mutex
	results []result
	calls   int
	// swept, when set, receives one value per sweep, so a test can wait for
	// the next sweep instead of polling for it. The sweep then waits for a
	// token on resume, if there is one, which holds the sweeper inside a
	// sweep while the test does something the sweeper must not observe yet.
	swept  chan struct{}
	resume chan struct{}
}

type result struct {
	cleared int
	err     error
}

func (s *fakeStore) SweepBlobDeletions(ctx context.Context) (int, error) {
	s.mu.Lock()
	s.calls++
	calls := s.calls
	var r result
	if calls <= len(s.results) {
		r = s.results[calls-1]
	}
	s.mu.Unlock()
	if s.swept != nil {
		select {
		case s.swept <- struct{}{}:
		case <-ctx.Done():
		}
	}
	if s.resume != nil {
		select {
		case <-s.resume:
		case <-ctx.Done():
		}
	}
	return r.cleared, r.err
}

func (s *fakeStore) sweeps() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// A backlog is drained batch by batch instead of one batch per tick.
func TestSweepDrainsUntilTheQueueIsEmpty(t *testing.T) {
	store := &fakeStore{results: []result{{cleared: 100}, {cleared: 100}, {cleared: 7}, {cleared: 0}}}
	sweeper := &blobdeletions.Sweeper{Store: store}
	sweeper.Sweep(context.Background())
	if store.sweeps() != 4 {
		t.Fatalf("sweeps = %d, want 4 (three batches plus the empty one that ends the drain)", store.sweeps())
	}
}

// A failing sweep stops the drain; the queued rows come back on a later tick.
func TestSweepStopsOnError(t *testing.T) {
	store := &fakeStore{results: []result{{err: errors.New("blob storage unavailable")}, {cleared: 1}}}
	sweeper := &blobdeletions.Sweeper{Store: store}
	sweeper.Sweep(context.Background())
	if store.sweeps() != 1 {
		t.Fatalf("sweeps = %d, want 1", store.sweeps())
	}
}

// Rows a previous process left queued must not wait out a whole interval.
func TestRunSweepsBeforeTheFirstTick(t *testing.T) {
	store := &fakeStore{swept: make(chan struct{}, 1)}
	sweeper := &blobdeletions.Sweeper{Store: store, Interval: time.Hour}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go sweeper.Run(ctx)

	waitForSweep(t, store, "the start-up sweep")
}

// A queued delete wakes the sweeper instead of leaving its objects for the
// next tick, and a burst of deletes still costs a single sweep.
func TestRunSweepsWhenWoken(t *testing.T) {
	store := &fakeStore{swept: make(chan struct{}, 4), resume: make(chan struct{})}
	// The one-slot channel the store signals on: while a sweep is in flight
	// the extra signals of a burst are dropped, because the pending one
	// already asks for the sweep that follows it.
	wake := make(chan struct{}, 1)
	sweeper := &blobdeletions.Sweeper{Store: store, Interval: time.Hour, Wake: wake}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); sweeper.Run(ctx) }()
	waitForSweep(t, store, "the start-up sweep")

	for i := 0; i < 5; i++ {
		select {
		case wake <- struct{}{}:
		default:
		}
	}
	releaseSweep(t, store)
	waitForSweep(t, store, "the sweep the wake-up asks for")
	releaseSweep(t, store)

	// The interval is an hour away, so shutting down now leaves exactly the
	// sweeps that ran: one at start-up and one for the whole burst.
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("sweeper did not stop when its context was cancelled")
	}
	if store.sweeps() != 2 {
		t.Fatalf("sweeps = %d, want 2 (start-up, then one for the coalesced burst)", store.sweeps())
	}
}

// A sweeper with no wake-up channel keeps sweeping on its ticker.
func TestRunSweepsOnItsTickerWithoutAWakeChannel(t *testing.T) {
	store := &fakeStore{swept: make(chan struct{}, 4)}
	sweeper := &blobdeletions.Sweeper{Store: store, Interval: time.Millisecond}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go sweeper.Run(ctx)

	waitForSweep(t, store, "the start-up sweep")
	waitForSweep(t, store, "the sweep on the first tick")
}

func waitForSweep(t *testing.T, store *fakeStore, what string) {
	t.Helper()
	select {
	case <-store.swept:
	case <-time.After(5 * time.Second):
		t.Fatalf("%s never ran", what)
	}
}

// releaseSweep lets the sweep the sweeper is parked in finish.
func releaseSweep(t *testing.T, store *fakeStore) {
	t.Helper()
	select {
	case store.resume <- struct{}{}:
	case <-time.After(5 * time.Second):
		t.Fatal("sweeper never picked up the go-ahead to finish its sweep")
	}
}

// Run sweeps on its interval and returns when the process shuts down.
func TestRunSweepsUntilContextIsCancelled(t *testing.T) {
	store := &fakeStore{}
	sweeper := &blobdeletions.Sweeper{Store: store, Interval: time.Millisecond}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); sweeper.Run(ctx) }()

	deadline := time.After(5 * time.Second)
	for store.sweeps() == 0 {
		select {
		case <-deadline:
			t.Fatal("sweeper never ran")
		case <-time.After(time.Millisecond):
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("sweeper did not stop when its context was cancelled")
	}
}
