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
}

type result struct {
	cleared int
	err     error
}

func (s *fakeStore) SweepBlobDeletions(context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.calls > len(s.results) {
		return 0, nil
	}
	r := s.results[s.calls-1]
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
