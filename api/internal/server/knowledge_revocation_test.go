package server

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/razorpay/aidocs/api/internal/connections"
	"github.com/razorpay/aidocs/api/internal/knowledge"
)

type revokingProviderInspector struct {
	inspectCalls int
	compareCalls int
	onInspect    func(knowledge.Source)
}

func (p *revokingProviderInspector) Compare(context.Context, knowledge.Source, string, string) (knowledge.CompareResult, error) {
	p.compareCalls++
	return knowledge.CompareResult{ChangedFiles: []string{"docs/context.md"}}, nil
}

func (p *revokingProviderInspector) Inspect(_ context.Context, source knowledge.Source, _ string) (knowledge.InspectResult, error) {
	p.inspectCalls++
	if p.onInspect != nil {
		p.onInspect(source)
	}
	return knowledge.InspectResult{Revision: "revision-after-claim", ChangedFiles: []string{"docs/context.md"}}, nil
}

func TestQueuedSourceRunRevalidatesRevocationBeforePullRequest(t *testing.T) {
	for _, test := range []struct {
		name   string
		revoke func(t *testing.T, fixture knowledgeFixture, store *connections.Memory, account connections.Account, source knowledge.Source)
	}{
		{
			name: "source paused during provider inspection",
			revoke: func(t *testing.T, fixture knowledgeFixture, _ *connections.Memory, _ connections.Account, source knowledge.Source) {
				t.Helper()
				source.Status = "paused"
				source.UpdatedAt = knowledge.Now()
				if _, err := fixture.knowledge.PutSource(context.Background(), source); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "account disconnected during provider inspection",
			revoke: func(t *testing.T, fixture knowledgeFixture, store *connections.Memory, account connections.Account, _ knowledge.Source) {
				t.Helper()
				if _, err := store.Disconnect(context.Background(), fixture.owner.ID, account.ID); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			connectionStore := connections.NewMemory()
			account, err := connectionStore.Put(context.Background(), connections.Account{
				ID: "conn_revocation", UserID: "knowledge_owner", Provider: connections.ProviderSlack,
				ExternalAccountID: "U1", WorkspaceID: "T1", DisplayName: "Owner Slack", Status: connections.StatusDemo,
			})
			if err != nil {
				t.Fatal(err)
			}
			provider := &revokingProviderInspector{}
			fixture := newKnowledgeFixtureWithOptions(t, WithConnectionStore(connectionStore), WithKnowledgeProviderClient(provider))
			now := knowledge.Now()
			source := knowledge.Source{
				ID: "source_revocation", RepositoryID: fixture.repositoryID, ConnectionID: account.ID,
				Provider: knowledge.ProviderSlack, ExternalRepository: "T1/C1/171.000001",
				TargetDocumentID: fixture.documentID, TriggerMode: "event", Direction: "into_aidocs", Status: "active",
				CreatedBy: fixture.owner, CreatedAt: now, UpdatedAt: now,
			}
			if _, err := fixture.knowledge.PutSource(context.Background(), source); err != nil {
				t.Fatal(err)
			}
			provider.onInspect = func(inspected knowledge.Source) {
				test.revoke(t, fixture, connectionStore, account, inspected)
			}

			run := knowledge.SyncRun{
				ID: "sync_revocation", RepositoryID: fixture.repositoryID, SourceID: source.ID,
				Status: "processing", ChangedFiles: []string{}, StartedAt: knowledge.Now(),
			}
			processKnowledgeRun(context.Background(), dependencies{
				repository: fixture.repository, knowledge: fixture.knowledge, connections: connectionStore, providerClient: provider,
			}, run)

			if provider.inspectCalls != 1 || provider.compareCalls != 0 {
				t.Fatalf("provider calls inspect=%d compare=%d", provider.inspectCalls, provider.compareCalls)
			}
			pullRequests, err := fixture.repository.ListPullRequests(context.Background(), fixture.repositoryID)
			if err != nil || len(pullRequests) != 0 {
				t.Fatalf("pull requests=%#v err=%v", pullRequests, err)
			}
			runs, err := fixture.knowledge.ListSyncRuns(context.Background(), fixture.repositoryID)
			if err != nil || len(runs) != 1 || runs[0].Outcome != "no_op" || !strings.Contains(runs[0].Message, "cancelled") {
				t.Fatalf("runs=%#v err=%v", runs, err)
			}
			storedSource, err := fixture.knowledge.GetSource(context.Background(), fixture.repositoryID, source.ID)
			if err != nil || storedSource.LastSyncedAt != "" {
				t.Fatalf("cancelled source=%#v err=%v", storedSource, err)
			}
			if test.name == "source paused during provider inspection" && storedSource.Status != "paused" {
				t.Fatalf("paused source status=%q", storedSource.Status)
			}
		})
	}
}

func TestSourceCheckAndSimulationRejectExportOnlyImports(t *testing.T) {
	fixture := newKnowledgeFixture(t)
	now := knowledge.Now()
	source := knowledge.Source{
		ID: "source_export_only", RepositoryID: fixture.repositoryID, Provider: knowledge.ProviderManual,
		TargetDocumentID: fixture.documentID, TriggerMode: "manual", Direction: "export_only", Status: "active",
		CreatedBy: fixture.owner, CreatedAt: now, UpdatedAt: now,
	}
	if _, err := fixture.knowledge.PutSource(context.Background(), source); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"check", "simulate"} {
		response := knowledgeRequest(t, fixture, http.MethodPost,
			"/v1/repositories/"+fixture.repositoryID+"/sources/"+source.ID+"/"+action,
			"owner", map[string]any{"changed_files": []string{"docs/context.md"}},
		)
		if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), `"code":"source_export_only"`) {
			t.Fatalf("%s status=%d body=%s", action, response.Code, response.Body.String())
		}
	}
	if runs, err := fixture.knowledge.ListSyncRuns(context.Background(), fixture.repositoryID); err != nil || len(runs) != 0 {
		t.Fatalf("sync runs=%#v err=%v", runs, err)
	}
	if pullRequests, err := fixture.repository.ListPullRequests(context.Background(), fixture.repositoryID); err != nil || len(pullRequests) != 0 {
		t.Fatalf("pull requests=%#v err=%v", pullRequests, err)
	}
}

func TestSourcePollingSkipsExportOnlySources(t *testing.T) {
	store := knowledge.NewMemory()
	now := knowledge.Now()
	for _, source := range []knowledge.Source{
		{ID: "source_import", RepositoryID: "repo_poll_direction", ConnectionID: "conn_import", Provider: knowledge.ProviderGitHub, TriggerMode: "scheduled", Direction: "into_aidocs", Status: "active", CreatedAt: now, UpdatedAt: now},
		{ID: "source_export", RepositoryID: "repo_poll_direction", ConnectionID: "conn_export", Provider: knowledge.ProviderGitHub, TriggerMode: "scheduled", Direction: "export_only", Status: "active", CreatedAt: now, UpdatedAt: now},
	} {
		if _, err := store.PutSource(context.Background(), source); err != nil {
			t.Fatal(err)
		}
	}
	queueSourcePolls(context.Background(), dependencies{knowledge: store, sourcePollInterval: 15 * time.Minute}, time.Unix(1_800_000_000, 0))
	runs, err := store.ListSyncRuns(context.Background(), "repo_poll_direction")
	if err != nil || len(runs) != 1 || runs[0].SourceID != "source_import" {
		t.Fatalf("poll runs=%#v err=%v", runs, err)
	}
}

func TestRepositoryTransferMarksPersonalSourcesNeedsReconnect(t *testing.T) {
	fixture := newKnowledgeFixture(t)
	now := knowledge.Now()
	for _, source := range []knowledge.Source{
		{ID: "source_personal", RepositoryID: fixture.repositoryID, ConnectionID: "conn_previous_owner", Provider: knowledge.ProviderGitHub, Direction: "into_aidocs", Status: "active", CreatedBy: fixture.owner, CreatedAt: now, UpdatedAt: now},
		{ID: "source_unlinked", RepositoryID: fixture.repositoryID, Provider: knowledge.ProviderManual, Direction: "into_aidocs", Status: "active", CreatedBy: fixture.owner, CreatedAt: now, UpdatedAt: now},
	} {
		if _, err := fixture.knowledge.PutSource(context.Background(), source); err != nil {
			t.Fatal(err)
		}
	}
	base := "/v1/repositories/" + fixture.repositoryID
	if response := knowledgeRequest(t, fixture, http.MethodPost, base+"/transfer", "owner", map[string]any{"new_owner_id": fixture.editor.ID}); response.Code != http.StatusAccepted {
		t.Fatalf("request transfer status=%d body=%s", response.Code, response.Body.String())
	}
	if response := knowledgeRequest(t, fixture, http.MethodPost, base+"/transfer/accept", "editor", nil); response.Code != http.StatusOK {
		t.Fatalf("accept transfer status=%d body=%s", response.Code, response.Body.String())
	}
	personal, err := fixture.knowledge.GetSource(context.Background(), fixture.repositoryID, "source_personal")
	if err != nil || personal.Status != "needs_reconnect" {
		t.Fatalf("personal source=%#v err=%v", personal, err)
	}
	unlinked, err := fixture.knowledge.GetSource(context.Background(), fixture.repositoryID, "source_unlinked")
	if err != nil || unlinked.Status != "active" {
		t.Fatalf("unlinked source=%#v err=%v", unlinked, err)
	}
}
