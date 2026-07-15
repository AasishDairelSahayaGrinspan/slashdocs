package knowledge_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/razorpay/aidocs/api/internal/auth"
	"github.com/razorpay/aidocs/api/internal/connections"
	"github.com/razorpay/aidocs/api/internal/knowledge"
	"github.com/razorpay/aidocs/api/internal/repo"
	pgrepo "github.com/razorpay/aidocs/api/internal/repo/postgres"
	"github.com/razorpay/aidocs/api/internal/testdb"
)

func TestPostgresPortalSlugUniquenessIsAtomicUnderConcurrency(t *testing.T) {
	databaseURL := testdb.New(t)
	base, err := pgrepo.Connect(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer base.Close()

	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	owner := auth.Principal{Type: auth.PrincipalUser, ID: "portal_owner_" + suffix, Email: "portal-owner-" + suffix + "@example.com", Name: "Portal owner"}
	create := func(label string) repo.RepositoryRecord {
		repository, createErr := base.CreateRepository(ctx, owner, repo.CreateRepositoryParams{
			Name: "Portal " + label + " " + suffix, Slug: "portal-" + label + "-" + suffix,
			Scope: repo.RepositoryScopePersonal, NamespaceID: owner.ID, NamespaceName: owner.Name,
			RequiredApprovals: 1, Protected: true,
		})
		if createErr != nil {
			t.Fatal(createErr)
		}
		return repository
	}
	first, second := create("first"), create("second")

	store := knowledge.NewPostgres(base.Pool())
	settings := []knowledge.Settings{knowledge.DefaultSettings(first.ID), knowledge.DefaultSettings(second.ID)}
	settings[0].Portal = knowledge.PortalSettings{Enabled: true, Slug: " Concurrent Portal " + suffix, Visibility: "public"}
	settings[1].Portal = knowledge.PortalSettings{Enabled: true, Slug: "concurrent---portal---" + suffix, Visibility: "public"}

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
	for _, putErr := range errorsByRepository {
		switch {
		case putErr == nil:
			successes++
		case errors.Is(putErr, knowledge.ErrConflict):
			conflicts++
		default:
			t.Fatalf("PutSettings error = %v", putErr)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent results successes=%d conflicts=%d, want one each", successes, conflicts)
	}

	winner, err := store.FindPortal(ctx, "CONCURRENT / PORTAL / "+suffix)
	if err != nil || (winner.RepositoryID != first.ID && winner.RepositoryID != second.ID) {
		t.Fatalf("FindPortal() = %+v, %v", winner, err)
	}
	if winner.Portal.Slug != knowledge.NormalizePortalSlug("concurrent portal "+suffix) {
		t.Fatalf("stored portal slug = %q", winner.Portal.Slug)
	}
	loserID := first.ID
	if winner.RepositoryID == loserID {
		loserID = second.ID
	}
	loser, err := store.GetSettings(ctx, loserID)
	if err != nil || loser.Portal.Enabled {
		t.Fatalf("conflicting write mutated loser settings = %+v, %v", loser, err)
	}
}

func TestPostgresConnectedKnowledgePersistence(t *testing.T) {
	databaseURL := testdb.New(t)
	base, err := pgrepo.Connect(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer base.Close()
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	owner := auth.Principal{Type: auth.PrincipalUser, ID: "knowledge_pg_owner_" + suffix, Email: "knowledge-pg-" + suffix + "@example.com", Name: "Knowledge owner"}
	repository, err := base.CreateRepository(ctx, owner, repo.CreateRepositoryParams{Name: "Connected knowledge Postgres " + suffix, Slug: "connected-knowledge-postgres-" + suffix, Scope: repo.RepositoryScopePersonal, NamespaceID: owner.ID, NamespaceName: owner.Name, RequiredApprovals: 1, Protected: true})
	if err != nil {
		t.Fatal(err)
	}
	store := knowledge.NewPostgres(base.Pool())
	settings := knowledge.DefaultSettings(repository.ID)
	portalSlug := "knowledge-postgres-" + suffix
	settings.Portal = knowledge.PortalSettings{Enabled: true, Slug: portalSlug, Visibility: "public"}
	if _, err := store.PutSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	if got, err := store.FindPortal(ctx, portalSlug); err != nil || got.RepositoryID != repository.ID {
		t.Fatalf("FindPortal repository=%q err=%v", got.RepositoryID, err)
	}
	now := knowledge.Now()
	connection := connections.Account{ID: "conn_knowledge_pg_" + suffix, UserID: owner.ID, Provider: connections.ProviderGitHub, ExternalAccountID: "acme", DisplayName: "Acme GitHub", Status: connections.StatusActive, CredentialCiphertext: []byte("encrypted-token")}
	if _, err := connections.NewPostgres(base.Pool()).Put(ctx, connection); err != nil {
		t.Fatal(err)
	}
	source := knowledge.Source{ID: "src_knowledge_pg_" + suffix, RepositoryID: repository.ID, ConnectionID: connection.ID, Provider: knowledge.ProviderGitHub, ExternalRepository: "acme/payments", Branch: "main", IncludePaths: []string{}, ExcludePaths: []string{}, TriggerMode: "default_branch", Direction: "into_aidocs", Status: "active", CreatedBy: owner, CreatedAt: now, UpdatedAt: now}
	if _, err := store.PutSource(ctx, source); err != nil {
		t.Fatal(err)
	}
	// Simulate an older application instance replacing the JSON payload. The
	// real connection_id column must remain authoritative during rolling deploys.
	if _, err := base.Pool().Exec(ctx, `UPDATE connected_knowledge_records SET payload=payload - 'connection_id' WHERE kind='source' AND id=$1`, source.ID); err != nil {
		t.Fatal(err)
	}
	gotSource, err := store.GetSource(ctx, repository.ID, source.ID)
	if err != nil || gotSource.ConnectionID != connection.ID {
		t.Fatalf("GetSource connection=%q err=%v", gotSource.ConnectionID, err)
	}
	listedSources, err := store.ListSources(ctx, repository.ID)
	if err != nil || len(listedSources) != 1 || listedSources[0].ConnectionID != connection.ID {
		t.Fatalf("ListSources=%+v err=%v", listedSources, err)
	}
	if _, err := store.PutSource(ctx, gotSource); err != nil {
		t.Fatalf("rewrite source: %v", err)
	}
	var storedConnectionID string
	if err := base.Pool().QueryRow(ctx, `SELECT connection_id FROM connected_knowledge_records WHERE kind='source' AND id=$1`, source.ID).Scan(&storedConnectionID); err != nil || storedConnectionID != connection.ID {
		t.Fatalf("stored connection=%q err=%v", storedConnectionID, err)
	}
	deliveryID := "delivery-pg-" + suffix
	if accepted, err := store.AcceptDelivery(ctx, "github", deliveryID); err != nil || !accepted {
		t.Fatalf("first delivery accepted=%t err=%v", accepted, err)
	}
	if accepted, err := store.AcceptDelivery(ctx, "github", deliveryID); err != nil || accepted {
		t.Fatalf("duplicate delivery accepted=%t err=%v", accepted, err)
	}
	run := knowledge.SyncRun{ID: "sync_knowledge_pg_" + suffix, RepositoryID: repository.ID, SourceID: source.ID, ChangedFiles: []string{"service.go"}, Status: "queued", StartedAt: now}
	if _, err := store.AddSyncRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.ClaimQueuedSyncRuns(ctx, 1)
	if err != nil || len(claimed) != 1 || claimed[0].Status != "processing" {
		t.Fatalf("claimed=%+v err=%v", claimed, err)
	}
}
