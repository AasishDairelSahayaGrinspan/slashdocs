package server

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/razorpay/aidocs/api/internal/knowledge"
	"github.com/razorpay/aidocs/api/internal/repo"
)

type lifecycleKnowledgeStore struct {
	*knowledge.Memory
	claimCalls    atomic.Int64
	claimStarted  chan struct{}
	claimCanceled chan struct{}
	allowReturn   chan struct{}
	startOnce     sync.Once
	cancelOnce    sync.Once
	returnOnce    sync.Once
}

type cancellationSensitiveKnowledgeStore struct{ *knowledge.Memory }

func (s cancellationSensitiveKnowledgeStore) AddSyncRun(ctx context.Context, run knowledge.SyncRun) (knowledge.SyncRun, error) {
	if err := ctx.Err(); err != nil {
		return run, err
	}
	return s.Memory.AddSyncRun(ctx, run)
}

type blockingKnowledgeProvider struct {
	started chan struct{}
	once    sync.Once
}

func (p *blockingKnowledgeProvider) Compare(ctx context.Context, _ knowledge.Source, _, _ string) (knowledge.CompareResult, error) {
	p.once.Do(func() { close(p.started) })
	<-ctx.Done()
	return knowledge.CompareResult{}, ctx.Err()
}

func newLifecycleKnowledgeStore() *lifecycleKnowledgeStore {
	return &lifecycleKnowledgeStore{
		Memory:        knowledge.NewMemory(),
		claimStarted:  make(chan struct{}),
		claimCanceled: make(chan struct{}),
		allowReturn:   make(chan struct{}),
	}
}

func (s *lifecycleKnowledgeStore) ClaimQueuedSyncRuns(ctx context.Context, _ int) ([]knowledge.SyncRun, error) {
	s.claimCalls.Add(1)
	s.startOnce.Do(func() { close(s.claimStarted) })
	<-ctx.Done()
	s.cancelOnce.Do(func() { close(s.claimCanceled) })
	<-s.allowReturn
	return nil, ctx.Err()
}

func (s *lifecycleKnowledgeStore) releaseClaim() {
	s.returnOnce.Do(func() { close(s.allowReturn) })
}

func waitForLifecycleSignal(t *testing.T, signal <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}

func assertKnowledgePollingStopped(t *testing.T, store *lifecycleKnowledgeStore) {
	t.Helper()
	got := store.claimCalls.Load()
	time.Sleep(30 * time.Millisecond)
	if after := store.claimCalls.Load(); after != got {
		t.Fatalf("knowledge processor polled after shutdown: calls changed from %d to %d", got, after)
	}
}

func TestServerCloseCancelsAndWaitsForKnowledgeProcessor(t *testing.T) {
	store := newLifecycleKnowledgeStore()
	app := New(Config{Environment: envTest}, WithRepository(repo.NewMemory()), WithKnowledgeStore(store))
	t.Cleanup(func() {
		store.releaseClaim()
		app.Close()
	})
	waitForLifecycleSignal(t, store.claimStarted, "initial knowledge queue claim")

	closed := make(chan struct{})
	go func() {
		app.Close()
		close(closed)
	}()
	waitForLifecycleSignal(t, store.claimCanceled, "knowledge queue cancellation")
	select {
	case <-closed:
		t.Fatal("Server.Close returned before the knowledge processor exited")
	case <-time.After(20 * time.Millisecond):
	}

	store.releaseClaim()
	waitForLifecycleSignal(t, closed, "Server.Close")
	assertKnowledgePollingStopped(t, store)

	// Close is deliberately idempotent for callers with overlapping cleanup
	// paths.
	app.Close()
}

func TestLifecycleContextStopsKnowledgeProcessor(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	store := newLifecycleKnowledgeStore()
	app := New(Config{Environment: envTest}, WithLifecycleContext(ctx), WithRepository(repo.NewMemory()), WithKnowledgeStore(store))
	t.Cleanup(func() {
		cancel()
		store.releaseClaim()
		app.Close()
	})
	waitForLifecycleSignal(t, store.claimStarted, "initial knowledge queue claim")

	cancel()
	waitForLifecycleSignal(t, store.claimCanceled, "application lifecycle cancellation")
	store.releaseClaim()
	app.Close()
	assertKnowledgePollingStopped(t, store)
}

func TestServerCloseRequeuesRunInterruptedByCancellation(t *testing.T) {
	ctx := context.Background()
	store := cancellationSensitiveKnowledgeStore{Memory: knowledge.NewMemory()}
	source := knowledge.Source{
		ID:                 "source-lifecycle",
		RepositoryID:       "repository-lifecycle",
		Provider:           knowledge.ProviderGitHub,
		ExternalRepository: "acme/payments",
		Branch:             "main",
		Status:             "active",
	}
	if _, err := store.PutSource(ctx, source); err != nil {
		t.Fatal(err)
	}
	run := knowledge.SyncRun{
		ID:           "run-lifecycle",
		RepositoryID: source.RepositoryID,
		SourceID:     source.ID,
		BeforeSHA:    "before",
		AfterSHA:     "after",
		Status:       "queued",
		StartedAt:    knowledge.Now(),
	}
	if _, err := store.AddSyncRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	provider := &blockingKnowledgeProvider{started: make(chan struct{})}
	app := New(
		Config{Environment: envTest},
		WithRepository(repo.NewMemory()),
		WithKnowledgeStore(store),
		WithKnowledgeProviderClient(provider),
	)
	t.Cleanup(app.Close)
	waitForLifecycleSignal(t, provider.started, "provider comparison")

	app.Close()
	runs, err := store.ListSyncRuns(ctx, source.RepositoryID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].ID != run.ID || runs[0].Status != "queued" {
		t.Fatalf("interrupted runs = %#v, want %s requeued", runs, run.ID)
	}
}
