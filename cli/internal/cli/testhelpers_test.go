package cli

import (
	"encoding/json"
	"io"
	"os"
	"testing"
)

func writeTestJSON(t *testing.T, w io.Writer, value any) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Errorf("write JSON fixture response: %v", err)
	}
}

func writeTestString(t *testing.T, w io.Writer, value string) {
	t.Helper()
	if _, err := io.WriteString(w, value); err != nil {
		t.Errorf("write fixture response: %v", err)
	}
}

func writeTestFile(t *testing.T, path string, contents []byte) {
	t.Helper()
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatalf("write test file %s: %v", path, err)
	}
}

func readTestFile(t *testing.T, path string) []byte {
	t.Helper()
	contents, err := os.ReadFile(path) //nolint:gosec // Test callers provide paths rooted in t.TempDir.
	if err != nil {
		t.Fatalf("read test file %s: %v", path, err)
	}
	return contents
}
