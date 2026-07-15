package knowledge

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

func TestNormalizePortalSlug(t *testing.T) {
	t.Parallel()
	if got := NormalizePortalSlug("  Payments / Δ Docs !!! "); got != "payments-docs" {
		t.Fatalf("NormalizePortalSlug() = %q, want payments-docs", got)
	}
	if got := NormalizePortalSlug(strings.Repeat("a", MaxPortalSlugLength+1)); len(got) != MaxPortalSlugLength {
		t.Fatalf("normalized slug length = %d, want %d", len(got), MaxPortalSlugLength)
	}
}

func TestMemoryPortalSlugUniquenessIsAtomicUnderConcurrency(t *testing.T) {
	t.Parallel()
	store := NewMemory()
	ctx := context.Background()
	settings := []Settings{DefaultSettings("repo-one"), DefaultSettings("repo-two")}
	settings[0].Portal = PortalSettings{Enabled: true, Slug: " Concurrent Portal ", Visibility: "public"}
	settings[1].Portal = PortalSettings{Enabled: true, Slug: "concurrent---portal", Visibility: "public"}

	start := make(chan struct{})
	errorsByRepository := make([]error, len(settings))
	var wg sync.WaitGroup
	for i := range settings {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			_, errorsByRepository[index] = store.PutSettings(ctx, settings[index])
		}(i)
	}
	close(start)
	wg.Wait()

	successes, conflicts := 0, 0
	for _, err := range errorsByRepository {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrConflict):
			conflicts++
		default:
			t.Fatalf("PutSettings error = %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent results successes=%d conflicts=%d, want one each", successes, conflicts)
	}

	winner, err := store.FindPortal(ctx, " CONCURRENT / PORTAL ")
	if err != nil || (winner.RepositoryID != "repo-one" && winner.RepositoryID != "repo-two") {
		t.Fatalf("FindPortal() = %+v, %v", winner, err)
	}
	loserID := "repo-one"
	if winner.RepositoryID == loserID {
		loserID = "repo-two"
	}
	loser, err := store.GetSettings(ctx, loserID)
	if err != nil || loser.Portal.Enabled {
		t.Fatalf("conflicting write mutated loser settings = %+v, %v", loser, err)
	}

	winner.Portal.Enabled = false
	if _, err := store.PutSettings(ctx, winner); err != nil {
		t.Fatalf("disable winning portal: %v", err)
	}
	loser.Portal = PortalSettings{Enabled: true, Slug: "concurrent portal", Visibility: "public"}
	if _, err := store.PutSettings(ctx, loser); err != nil {
		t.Fatalf("released slug could not be claimed: %v", err)
	}
}

func TestMemoryQueueDeliveryIsAtomicAndDeduplicated(t *testing.T) {
	store := NewMemory()
	ctx := context.Background()
	existing := SyncRun{ID: "sync-existing", RepositoryID: "repo-1", Status: "queued"}
	if _, err := store.AddSyncRun(ctx, existing); err != nil {
		t.Fatal(err)
	}
	accepted, err := store.QueueDelivery(ctx, ProviderSlack, "event-1", []SyncRun{{ID: existing.ID, RepositoryID: "repo-1", Status: "queued"}})
	if accepted || !errors.Is(err, ErrConflict) {
		t.Fatalf("conflicting queue accepted=%t err=%v", accepted, err)
	}
	accepted, err = store.QueueDelivery(ctx, ProviderSlack, "event-1", []SyncRun{{ID: "sync-new", RepositoryID: "repo-1", Status: "queued"}})
	if err != nil || !accepted {
		t.Fatalf("delivery was consumed by failed transaction: accepted=%t err=%v", accepted, err)
	}
	accepted, err = store.QueueDelivery(ctx, ProviderSlack, "event-1", []SyncRun{{ID: "sync-duplicate", RepositoryID: "repo-1", Status: "queued"}})
	if err != nil || accepted {
		t.Fatalf("duplicate accepted=%t err=%v", accepted, err)
	}
	runs, err := store.ListSyncRuns(ctx, "repo-1")
	if err != nil || len(runs) != 2 {
		t.Fatalf("runs=%+v err=%v", runs, err)
	}
}
