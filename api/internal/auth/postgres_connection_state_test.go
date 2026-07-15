package auth_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/razorpay/aidocs/api/internal/auth"
	"github.com/razorpay/aidocs/api/internal/testdb"
)

func TestPostgresConnectionStateIsDurableOneTimeAndKindIsolated(t *testing.T) {
	ctx := context.Background()
	db, err := pgxpool.New(ctx, testdb.New(t))
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	t.Cleanup(db.Close)
	store := auth.NewPostgresStateStore(db)
	want := auth.ConnectionState{
		UserID:       "usr_1",
		Provider:     "google_drive",
		Redirect:     "/app/settings/connections",
		CodeVerifier: "pkce-verifier",
	}
	if err := store.PutState(ctx, "shared-id", auth.LoginState{Mode: "web"}); err != nil {
		t.Fatalf("put login state: %v", err)
	}
	if err := store.PutConnectionState(ctx, "shared-id", want); err != nil {
		t.Fatalf("put connection state: %v", err)
	}
	var kind string
	if err := db.QueryRow(ctx, `SELECT kind FROM oauth_states WHERE id=$1 AND kind='connection_state'`, "shared-id").Scan(&kind); err != nil || kind != "connection_state" {
		t.Fatalf("persisted kind = %q, err %v", kind, err)
	}

	got, ok, err := store.TakeConnectionState(ctx, "shared-id")
	if err != nil || !ok {
		t.Fatalf("take connection state = %#v, %t, %v", got, ok, err)
	}
	if got.UserID != want.UserID || got.Provider != want.Provider || got.Redirect != want.Redirect || got.CodeVerifier != want.CodeVerifier || got.ExpiresAt.IsZero() {
		t.Fatalf("connection state = %#v, want fields %#v plus expiry", got, want)
	}
	if _, ok, err := store.TakeConnectionState(ctx, "shared-id"); err != nil || ok {
		t.Fatalf("second take unexpectedly succeeded: ok=%t err=%v", ok, err)
	}
	if _, ok, err := store.TakeState(ctx, "shared-id"); err != nil || !ok {
		t.Fatalf("connection state collided with login state: ok=%t err=%v", ok, err)
	}
}

func TestPostgresConnectionStateConsumesExpiredState(t *testing.T) {
	ctx := context.Background()
	db, err := pgxpool.New(ctx, testdb.New(t))
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	t.Cleanup(db.Close)
	store := auth.NewPostgresStateStore(db)
	if err := store.PutConnectionState(ctx, "expired", auth.ConnectionState{
		UserID: "usr_1", Provider: "devrev", ExpiresAt: time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("put expired state: %v", err)
	}
	if _, ok, err := store.TakeConnectionState(ctx, "expired"); err != nil || ok {
		t.Fatalf("take expired state: ok=%t err=%v", ok, err)
	}
	var count int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM oauth_states WHERE kind='connection_state' AND id='expired'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("expired row count = %d, err %v", count, err)
	}
}
