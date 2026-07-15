package connections

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMemoryStoreLifecycleAndIsolation(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, time.July, 15, 10, 0, 0, 0, time.UTC)
	store := NewMemory()
	store.now = func() time.Time { return now }
	expires := now.Add(time.Hour)
	account := Account{
		ID:                   "conn_1",
		UserID:               "usr_1",
		Provider:             ProviderGitHub,
		ExternalAccountID:    "octocat",
		DisplayName:          "Octocat",
		Scopes:               []string{"repo", " read:user ", "repo"},
		Status:               StatusActive,
		CredentialCiphertext: []byte("encrypted"),
		ExpiresAt:            &expires,
	}
	created, err := store.Put(ctx, account)
	if err != nil {
		t.Fatalf("put account: %v", err)
	}
	if !created.CreatedAt.Equal(now) || !created.UpdatedAt.Equal(now) {
		t.Fatalf("timestamps = %v/%v, want %v", created.CreatedAt, created.UpdatedAt, now)
	}
	if len(created.Scopes) != 2 || created.Scopes[0] != "read:user" || created.Scopes[1] != "repo" {
		t.Fatalf("normalized scopes = %#v", created.Scopes)
	}

	// Returned slices must not alias stored secrets or scopes.
	created.Scopes[0] = "changed"
	created.CredentialCiphertext[0] = 'X'
	got, err := store.Get(ctx, account.ID)
	if err != nil {
		t.Fatalf("get account: %v", err)
	}
	if got.Scopes[0] != "read:user" || string(got.CredentialCiphertext) != "encrypted" {
		t.Fatalf("stored account was mutated through return value: %#v", got)
	}

	listed, err := store.ListByUser(ctx, "usr_1")
	if err != nil {
		t.Fatalf("list accounts: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != account.ID {
		t.Fatalf("listed accounts = %#v", listed)
	}
	other, err := store.ListByUser(ctx, "usr_2")
	if err != nil || len(other) != 0 {
		t.Fatalf("other user's accounts = %#v, err %v", other, err)
	}

	now = now.Add(30 * time.Second)
	refreshedInput := got
	refreshedInput.CredentialCiphertext = []byte("refreshed-encrypted")
	refreshed, err := store.RefreshCredentials(ctx, refreshedInput, got.UpdatedAt)
	if err != nil {
		t.Fatalf("refresh credentials: %v", err)
	}
	if string(refreshed.CredentialCiphertext) != "refreshed-encrypted" || !refreshed.UpdatedAt.Equal(now) {
		t.Fatalf("refreshed account = %#v", refreshed)
	}
	if _, err := store.RefreshCredentials(ctx, refreshedInput, got.UpdatedAt); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale refresh error = %v, want ErrConflict", err)
	}

	now = now.Add(time.Minute)
	disconnected, err := store.Disconnect(ctx, "usr_1", account.ID)
	if err != nil {
		t.Fatalf("disconnect: %v", err)
	}
	if disconnected.Status != StatusDisconnected || len(disconnected.CredentialCiphertext) != 0 || disconnected.ExpiresAt != nil {
		t.Fatalf("disconnected account retains credentials: %#v", disconnected)
	}
	if !disconnected.CreatedAt.Equal(created.CreatedAt) || !disconnected.UpdatedAt.Equal(now) {
		t.Fatalf("disconnect timestamps = %v/%v", disconnected.CreatedAt, disconnected.UpdatedAt)
	}
	if _, err := store.RefreshCredentials(ctx, refreshed, refreshed.UpdatedAt); !errors.Is(err, ErrConflict) {
		t.Fatalf("refresh after disconnect error = %v, want ErrConflict", err)
	}
	if _, err := store.Disconnect(ctx, "usr_2", account.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user disconnect error = %v, want ErrNotFound", err)
	}
}

func TestMemoryStoreRejectsIdentityConflictsAndInvalidAccounts(t *testing.T) {
	ctx := context.Background()
	store := NewMemory()
	base := Account{
		ID:                   "conn_1",
		UserID:               "usr_1",
		Provider:             ProviderSlack,
		ExternalAccountID:    "U123",
		WorkspaceID:          "T123",
		Status:               StatusActive,
		CredentialCiphertext: []byte("encrypted"),
	}
	if _, err := store.Put(ctx, base); err != nil {
		t.Fatalf("put base account: %v", err)
	}
	duplicate := base
	duplicate.ID = "conn_2"
	if _, err := store.Put(ctx, duplicate); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate identity error = %v, want ErrConflict", err)
	}
	pivot := base
	pivot.UserID = "usr_2"
	if _, err := store.Put(ctx, pivot); !errors.Is(err, ErrConflict) {
		t.Fatalf("identity pivot error = %v, want ErrConflict", err)
	}
	invalid := base
	invalid.ID = ""
	if _, err := store.Put(ctx, invalid); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid account error = %v, want ErrInvalid", err)
	}
}
