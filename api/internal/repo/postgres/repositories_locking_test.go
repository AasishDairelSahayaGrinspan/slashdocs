package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/razorpay/aidocs/api/internal/auth"
	"github.com/razorpay/aidocs/api/internal/repo"
	pgrepo "github.com/razorpay/aidocs/api/internal/repo/postgres"
)

func TestPullRequestMutationsAcquireTreeBeforePullRequestRow(t *testing.T) {
	tests := []struct {
		name string
		run  func(context.Context, *pgrepo.Store, string, int64, auth.Principal) error
	}{
		{
			name: "rebase",
			run: func(ctx context.Context, store *pgrepo.Store, repositoryID string, number int64, actor auth.Principal) error {
				_, err := store.RebasePullRequest(ctx, repositoryID, number, actor)
				return err
			},
		},
		{
			name: "merge",
			run: func(ctx context.Context, store *pgrepo.Store, repositoryID string, number int64, actor auth.Principal) error {
				_, err := store.MergePullRequest(ctx, repositoryID, number, actor)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newStore(t)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			suffix := fmt.Sprintf("%s-%d", tt.name, time.Now().UnixNano())
			owner := auth.Principal{Type: auth.PrincipalUser, ID: "lock-owner-" + suffix, Email: "lock-owner-" + suffix + "@example.com", Name: "Lock owner"}
			patcher := auth.Principal{Type: auth.PrincipalUser, ID: "lock-patcher-" + suffix, Email: "lock-patcher-" + suffix + "@example.com", Name: "Lock patcher"}
			finalActor := auth.Principal{Type: auth.PrincipalUser, ID: "lock-final-" + suffix, Email: "lock-final-" + suffix + "@example.com", Name: "Lock finalizer"}

			document, base, err := store.CreateDocument(ctx, owner, "Lock order", []byte("<p>base</p>"))
			if err != nil {
				t.Fatal(err)
			}
			repository, err := store.CreateRepository(ctx, owner, repo.CreateRepositoryParams{
				Name: "Lock order " + suffix, Slug: "lock-order-" + suffix, Scope: repo.RepositoryScopePersonal,
				NamespaceID: owner.ID, NamespaceName: owner.Name, DocumentID: document.ID, Protected: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			pullRequest, err := store.CreatePullRequest(ctx, repository.ID, document.ID, base.ID, "Lock order", "", "initial", []byte("<p>one</p>"), owner)
			if err != nil {
				t.Fatal(err)
			}

			blocker, err := store.Pool().Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			blockerOpen := true
			t.Cleanup(func() {
				if blockerOpen {
					_ = blocker.Rollback(context.Background())
				}
			})
			if _, err := blocker.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 701239))`, "folder:"+repository.RootFolderID); err != nil {
				t.Fatal(err)
			}

			patchResult := make(chan error, 1)
			go func() {
				_, err := store.CreatePullRequestPatchset(ctx, repository.ID, pullRequest.Number, "replacement", []repo.PullRequestChangeInput{{
					Operation: repo.PullRequestChangeUpdate, DocumentID: document.ID, BaseVersionID: base.ID, HTML: []byte("<p>two</p>"),
				}}, patcher)
				patchResult <- err
			}()
			waitForAdvisoryWaiters(ctx, t, store.Pool(), 1)

			finalResult := make(chan error, 1)
			go func() {
				finalResult <- tt.run(ctx, store, repository.ID, pullRequest.Number, finalActor)
			}()
			waitForAdvisoryWaiters(ctx, t, store.Pool(), 2)

			// While both operations are queued behind the tree lock, the PR row must
			// still be available. The old PR-row-then-tree order fails here with
			// lock_not_available and can deadlock once the tree lock is released.
			probe, err := store.Pool().Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			var id string
			probeErr := probe.QueryRow(ctx, `SELECT id FROM pull_requests WHERE repository_id=$1 AND number=$2 FOR UPDATE NOWAIT`, repository.ID, pullRequest.Number).Scan(&id)
			_ = probe.Rollback(context.Background())

			if err := blocker.Rollback(ctx); err != nil {
				t.Fatal(err)
			}
			blockerOpen = false

			for name, result := range map[string]<-chan error{"patchset": patchResult, tt.name: finalResult} {
				select {
				case err := <-result:
					if err != nil {
						var pgErr *pgconn.PgError
						if errors.As(err, &pgErr) && pgErr.Code == "40P01" {
							t.Fatalf("%s deadlocked: %v", name, err)
						}
						t.Fatalf("%s failed: %v", name, err)
					}
				case <-ctx.Done():
					t.Fatalf("%s did not complete: %v", name, ctx.Err())
				}
			}

			if probeErr != nil {
				var pgErr *pgconn.PgError
				if errors.As(probeErr, &pgErr) && pgErr.Code == "55P03" {
					t.Fatalf("%s locked the pull-request row before acquiring the tree lock", tt.name)
				}
				t.Fatalf("probe pull-request row: %v", probeErr)
			}
		})
	}
}

func waitForAdvisoryWaiters(ctx context.Context, t *testing.T, pool *pgxpool.Pool, want int) {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var count int
		err := pool.QueryRow(ctx, `
			SELECT count(*) FROM pg_locks
			WHERE locktype='advisory' AND NOT granted
			  AND database=(SELECT oid FROM pg_database WHERE datname=current_database())`).Scan(&count)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("count advisory waiters: %v", err)
		}
		if count >= want {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("waiting for %d advisory lock waiters: %v", want, ctx.Err())
		case <-ticker.C:
		}
	}
}
