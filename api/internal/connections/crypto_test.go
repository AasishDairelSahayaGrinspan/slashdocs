package connections

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestCipherRoundTripAndAccountBinding(t *testing.T) {
	cipher, err := NewCipher(bytes.Repeat([]byte{0x42}, 32))
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}
	account := Account{ID: "conn_1", UserID: "usr_1", Provider: ProviderGitHub}
	want := Credentials{AccessToken: "access-secret", RefreshToken: "refresh-secret", TokenType: "Bearer"}

	first, err := cipher.Seal(account, want)
	if err != nil {
		t.Fatalf("seal credentials: %v", err)
	}
	second, err := cipher.Seal(account, want)
	if err != nil {
		t.Fatalf("seal credentials again: %v", err)
	}
	if bytes.Equal(first, second) {
		t.Fatal("credential encryption reused a nonce")
	}
	if bytes.Contains(first, []byte(want.AccessToken)) || bytes.Contains(first, []byte(want.RefreshToken)) {
		t.Fatal("credential envelope contains plaintext")
	}
	got, err := cipher.Open(account, first)
	if err != nil {
		t.Fatalf("open credentials: %v", err)
	}
	if got != want {
		t.Fatalf("credentials = %#v, want %#v", got, want)
	}

	otherUser := account
	otherUser.UserID = "usr_2"
	if _, err := cipher.Open(otherUser, first); err == nil {
		t.Fatal("ciphertext opened for a different user")
	}
	tampered := append([]byte(nil), first...)
	tampered[len(tampered)-1] ^= 1
	if _, err := cipher.Open(account, tampered); err == nil {
		t.Fatal("tampered ciphertext opened")
	}
}

func TestCipherRejectsInvalidInputs(t *testing.T) {
	for _, size := range []int{0, 16, 24, 31, 33} {
		if _, err := NewCipher(make([]byte, size)); err == nil {
			t.Fatalf("NewCipher accepted %d-byte key", size)
		}
	}
	cipher, err := NewCipher(make([]byte, 32))
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}
	account := Account{ID: "conn_1", UserID: "usr_1", Provider: ProviderSlack}
	if _, err := cipher.Seal(account, Credentials{}); err == nil {
		t.Fatal("Seal accepted an empty access token")
	}
	if _, err := cipher.Open(account, []byte{credentialEnvelopeVersion}); err == nil {
		t.Fatal("Open accepted a truncated envelope")
	}
}

func TestAccountAndCredentialsJSONNeverExposeSecrets(t *testing.T) {
	account := Account{
		ID:                   "conn_1",
		UserID:               "usr_1",
		Provider:             ProviderDevRev,
		ExternalAccountID:    "external",
		CredentialCiphertext: []byte("ciphertext-secret"),
	}
	raw, err := json.Marshal(account)
	if err != nil {
		t.Fatalf("marshal account: %v", err)
	}
	if strings.Contains(string(raw), "ciphertext-secret") || strings.Contains(string(raw), "credential") {
		t.Fatalf("account JSON exposes credential material: %s", raw)
	}
	raw, err = json.Marshal(Credentials{AccessToken: "access-secret", RefreshToken: "refresh-secret"})
	if err != nil {
		t.Fatalf("marshal credentials: %v", err)
	}
	if strings.Contains(string(raw), "secret") {
		t.Fatalf("credentials JSON exposes plaintext: %s", raw)
	}
}
