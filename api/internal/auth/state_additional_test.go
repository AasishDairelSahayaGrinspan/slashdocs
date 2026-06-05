package auth_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/razorpay/aidocs/api/internal/auth"
)

func TestStateStoreTakeOnceExpiryAndCodes(t *testing.T) {
	ctx := context.Background()
	store := auth.NewStateStore()

	if err := store.PutState(ctx, "state_1", auth.LoginState{Mode: "web"}); err != nil {
		t.Fatal(err)
	}
	st, ok, err := store.TakeState(ctx, "state_1")
	if err != nil || !ok || st.Mode != "web" {
		t.Fatalf("TakeState = (%+v, %v, %v), want web true nil", st, ok, err)
	}
	if _, ok, err := store.TakeState(ctx, "state_1"); err != nil || ok {
		t.Fatalf("second TakeState ok=%v err=%v, want false nil", ok, err)
	}

	if err := store.PutState(ctx, "expired", auth.LoginState{ExpiresAt: time.Now().Add(-time.Second)}); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.TakeState(ctx, "expired"); err != nil || ok {
		t.Fatalf("expired TakeState ok=%v err=%v, want false nil", ok, err)
	}

	if err := store.PutCode(ctx, "code_1", auth.LoginState{UserID: "usr_1"}); err != nil {
		t.Fatal(err)
	}
	code, ok, err := store.TakeCode(ctx, "code_1")
	if err != nil || !ok || code.UserID != "usr_1" {
		t.Fatalf("TakeCode = (%+v, %v, %v), want usr_1 true nil", code, ok, err)
	}
	if _, ok, err := store.TakeCode(ctx, "code_1"); err != nil || ok {
		t.Fatalf("second TakeCode ok=%v err=%v, want false nil", ok, err)
	}
}

func TestPKCESessionAndBearerEdgeCases(t *testing.T) {
	sum := sha256.Sum256([]byte("verifier"))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	if !auth.VerifyPKCES256("verifier", challenge) {
		t.Fatal("expected valid PKCE challenge")
	}
	if auth.VerifyPKCES256("verifier", "not base64") {
		t.Fatal("invalid base64 challenge verified")
	}
	if auth.VerifyPKCES256("other", challenge) {
		t.Fatal("wrong verifier verified")
	}

	codec := auth.SessionCodec{Secret: []byte("secret")}
	token := codec.SignForAudience("usr_1", "render", time.Minute)
	if got, ok := codec.VerifyAudience(token, "render"); !ok || got != "usr_1" {
		t.Fatalf("VerifyAudience = %q %v, want usr_1 true", got, ok)
	}
	if _, ok := codec.VerifySession(token); ok {
		t.Fatal("render token verified as session token")
	}
	expired := codec.SignForAudience("usr_1", "render", -time.Minute)
	if _, ok := codec.VerifyAudience(expired, "render"); ok {
		t.Fatal("expired token verified")
	}
	tampered := strings.TrimSuffix(token, token[len(token)-1:]) + "x"
	if _, ok := codec.VerifyAudience(tampered, "render"); ok {
		t.Fatal("tampered token verified")
	}

	if got := auth.BearerFromHeader("Bearer token"); got != "token" {
		t.Fatalf("BearerFromHeader = %q, want token", got)
	}
	if got := auth.BearerFromHeader("bearer   token  "); got != "token" {
		t.Fatalf("BearerFromHeader lower = %q, want token", got)
	}
	if got := auth.BearerFromHeader("Basic token"); got != "" {
		t.Fatalf("BearerFromHeader basic = %q, want empty", got)
	}
}
