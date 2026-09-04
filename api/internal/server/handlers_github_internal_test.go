package server

import (
	"bytes"
	"testing"
)

func TestGitHubTokenEncryptionRoundTripAndTamperDetection(t *testing.T) {
	sealed, err := sealGitHubToken("session-secret", "github-token")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, []byte("github-token")) {
		t.Fatal("ciphertext contains plaintext token")
	}
	got, err := openGitHubToken("session-secret", sealed)
	if err != nil || got != "github-token" {
		t.Fatalf("open token = %q, %v", got, err)
	}
	sealed[len(sealed)-1] ^= 1
	if _, err := openGitHubToken("session-secret", sealed); err == nil {
		t.Fatal("tampered ciphertext was accepted")
	}
}

func TestPrintableHTMLInjectsTopLevelPrintBootstrap(t *testing.T) {
	got := printableHTML([]byte("<!doctype html><html><body><h1>Report</h1></body></html>"))
	if !bytes.Contains(got, []byte(`window.print()`)) || !bytes.Contains(got, []byte(`<h1>Report</h1>`)) {
		t.Fatalf("printable HTML missing content or bootstrap: %s", got)
	}
}
