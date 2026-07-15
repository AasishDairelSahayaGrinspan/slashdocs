package auth

import (
	"context"
	"testing"
	"time"
)

func TestMemoryConnectionStateIsOneTimeAndIndependent(t *testing.T) {
	ctx := context.Background()
	store := NewStateStore()
	want := ConnectionState{
		UserID:       "usr_1",
		Provider:     "github",
		Redirect:     "/app/settings/connections",
		CodeVerifier: "pkce-verifier",
	}
	if err := store.PutState(ctx, "shared-id", LoginState{Mode: "web"}); err != nil {
		t.Fatalf("put login state: %v", err)
	}
	if err := store.PutConnectionState(ctx, "shared-id", want); err != nil {
		t.Fatalf("put connection state: %v", err)
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

func TestMemoryConnectionStateConsumesExpiredState(t *testing.T) {
	ctx := context.Background()
	store := NewStateStore()
	if err := store.PutConnectionState(ctx, "expired", ConnectionState{UserID: "usr_1", Provider: "slack", ExpiresAt: time.Now().Add(-time.Minute)}); err != nil {
		t.Fatalf("put expired state: %v", err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if _, ok, err := store.TakeConnectionState(ctx, "expired"); err != nil || ok {
			t.Fatalf("expired take %d: ok=%t err=%v", attempt, ok, err)
		}
	}
}
