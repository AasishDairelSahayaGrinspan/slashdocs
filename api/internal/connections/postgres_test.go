package connections_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/razorpay/aidocs/api/internal/connections"
	"github.com/razorpay/aidocs/api/internal/testdb"
)

func TestPostgresStoreLifecycleAndOwnership(t *testing.T) {
	ctx := context.Background()
	db, err := pgxpool.New(ctx, testdb.New(t))
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	t.Cleanup(db.Close)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	userID, otherUserID := "usr_connection_"+suffix, "usr_connection_other_"+suffix
	for _, user := range []string{userID, otherUserID} {
		if _, err := db.Exec(ctx, `INSERT INTO users(id,email,name) VALUES($1,$2,$3)`, user, user+"@example.com", user); err != nil {
			t.Fatalf("insert user: %v", err)
		}
	}
	store := connections.NewPostgres(db)
	expires := time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)
	account := connections.Account{
		ID:                   "conn_" + suffix,
		UserID:               userID,
		Provider:             connections.ProviderSlack,
		ExternalAccountID:    "U123",
		DisplayName:          "Bilal",
		WorkspaceID:          "T123",
		WorkspaceName:        "Razorpay",
		Scopes:               []string{"channels:history", " users:read ", "channels:history"},
		Status:               connections.StatusActive,
		CredentialCiphertext: []byte("encrypted-token"),
		ExpiresAt:            &expires,
	}
	created, err := store.Put(ctx, account)
	if err != nil {
		t.Fatalf("put account: %v", err)
	}
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() || len(created.Scopes) != 2 {
		t.Fatalf("created account = %#v", created)
	}
	got, err := store.Get(ctx, account.ID)
	if err != nil {
		t.Fatalf("get account: %v", err)
	}
	if got.UserID != userID || string(got.CredentialCiphertext) != "encrypted-token" || got.ExpiresAt == nil || !got.ExpiresAt.Equal(expires) {
		t.Fatalf("stored account = %#v", got)
	}
	listed, err := store.ListByUser(ctx, userID)
	if err != nil || len(listed) != 1 || listed[0].ID != account.ID {
		t.Fatalf("list accounts = %#v, err %v", listed, err)
	}
	other, err := store.ListByUser(ctx, otherUserID)
	if err != nil || len(other) != 0 {
		t.Fatalf("other user's list = %#v, err %v", other, err)
	}
	refreshInput := got
	refreshInput.CredentialCiphertext = []byte("refreshed-token")
	refreshed, err := store.RefreshCredentials(ctx, refreshInput, got.UpdatedAt)
	if err != nil {
		t.Fatalf("refresh credentials: %v", err)
	}
	if string(refreshed.CredentialCiphertext) != "refreshed-token" {
		t.Fatalf("refreshed account = %#v", refreshed)
	}
	if _, err := store.RefreshCredentials(ctx, refreshInput, got.UpdatedAt); !errors.Is(err, connections.ErrConflict) {
		t.Fatalf("stale refresh error = %v, want ErrConflict", err)
	}
	if _, err := store.Disconnect(ctx, otherUserID, account.ID); !errors.Is(err, connections.ErrNotFound) {
		t.Fatalf("cross-user disconnect error = %v, want ErrNotFound", err)
	}
	disconnected, err := store.Disconnect(ctx, userID, account.ID)
	if err != nil {
		t.Fatalf("disconnect: %v", err)
	}
	if disconnected.Status != connections.StatusDisconnected || disconnected.ExpiresAt != nil || len(disconnected.CredentialCiphertext) != 0 {
		t.Fatalf("disconnected account = %#v", disconnected)
	}
	if _, err := store.RefreshCredentials(ctx, refreshed, refreshed.UpdatedAt); !errors.Is(err, connections.ErrConflict) {
		t.Fatalf("refresh after disconnect error = %v, want ErrConflict", err)
	}

	duplicate := account
	duplicate.ID = account.ID + "_duplicate"
	if _, err := store.Put(ctx, duplicate); !errors.Is(err, connections.ErrConflict) {
		t.Fatalf("duplicate identity error = %v, want ErrConflict", err)
	}
	pivot := account
	pivot.UserID = otherUserID
	if _, err := store.Put(ctx, pivot); !errors.Is(err, connections.ErrConflict) {
		t.Fatalf("identity pivot error = %v, want ErrConflict", err)
	}
}

func TestPostgresStoreRequiresExistingUser(t *testing.T) {
	ctx := context.Background()
	db, err := pgxpool.New(ctx, testdb.New(t))
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	t.Cleanup(db.Close)
	store := connections.NewPostgres(db)
	_, err = store.Put(ctx, connections.Account{
		ID:                   "conn_missing_user",
		UserID:               "usr_missing",
		Provider:             connections.ProviderGitHub,
		ExternalAccountID:    "octocat",
		Status:               connections.StatusActive,
		CredentialCiphertext: []byte("encrypted"),
	})
	if err == nil {
		t.Fatal("Put accepted a connection for a missing user")
	}
}
