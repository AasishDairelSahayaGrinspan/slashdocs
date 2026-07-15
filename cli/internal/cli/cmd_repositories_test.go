package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepositoriesCreateFromDocument(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/repositories" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var in map[string]any
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			t.Fatal(err)
		}
		if in["name"] != "Handbook" || in["scope"] != "organization" || in["document_id"] != "doc_1" {
			t.Fatalf("repository body = %#v", in)
		}
		writeTestJSON(t, w, map[string]any{
			"id":             "repo_1",
			"slug":           "handbook",
			"namespace_name": "Razorpay",
		})
	}))
	defer srv.Close()
	setRepositoryCLIEnv(t, srv.URL)

	got, err := Execute([]string{"repos", "create", "--name", "Handbook", "--scope", "organization", "--document-id", "doc_1"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Razorpay/handbook", "repo_1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output %q missing %q", got, want)
		}
	}
}

func TestRepositoriesCreateEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			t.Fatal(err)
		}
		if in["name"] != "Handbook" || in["root_folder_id"] != nil || in["document_id"] != nil {
			t.Fatalf("empty repository body = %#v", in)
		}
		writeTestJSON(t, w, map[string]any{
			"id":             "repo_empty",
			"slug":           "handbook",
			"namespace_name": "Personal",
		})
	}))
	defer srv.Close()
	setRepositoryCLIEnv(t, srv.URL)

	got, err := Execute([]string{"repos", "create", "--name", "Handbook"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "repo_empty") {
		t.Fatalf("output = %q, want empty repository id", got)
	}
}

func TestRepositoriesCreateRejectsTwoSeeds(t *testing.T) {
	setRepositoryCLIEnv(t, "http://127.0.0.1:1")

	_, err := Execute([]string{"repos", "create", "--name", "Handbook", "--folder-id", "fld_1", "--document-id", "doc_1"})
	if err == nil || !strings.Contains(err.Error(), "at most one") {
		t.Fatalf("error = %v, want at-most-one-seed validation", err)
	}
}

func TestRepositoriesDelete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/v1/repositories/repo_1" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	setRepositoryCLIEnv(t, srv.URL)

	got, err := Execute([]string{"repos", "delete", "repo_1"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "Deleted repository repo_1.") {
		t.Fatalf("unexpected output %q", got)
	}
}

func TestRepositoryPullRequestCreate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/repositories/repo_1/pull-requests" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		requireMultipart(t, r)
		for field, want := range map[string]string{
			"document_id":     "doc_1",
			"base_version_id": "ver_1",
			"title":           "Clarify ownership",
			"change_summary":  "Address review",
		} {
			if got := r.FormValue(field); got != want {
				t.Fatalf("field %s = %q, want %q", field, got, want)
			}
		}
		writeTestJSON(t, w, map[string]any{
			"id":            "pr_1",
			"repository_id": "repo_1",
			"number":        7,
			"status":        "open",
		})
	}))
	defer srv.Close()
	setRepositoryCLIEnv(t, srv.URL)
	html := filepath.Join(t.TempDir(), "handbook.html")
	if err := os.WriteFile(html, []byte("<h1>Updated</h1>"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := Execute([]string{
		"repos", "prs", "create", "repo_1", "doc_1", html,
		"--base-version", "ver_1",
		"--title", "Clarify ownership",
		"--summary", "Address review",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "pull request #7") || !strings.Contains(got, "/app/repos/repo_1/pulls/7") {
		t.Fatalf("unexpected output %q", got)
	}
}

func TestRepositoryPullRequestReview(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/repositories/repo_1/pull-requests/7/reviews" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var in map[string]any
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			t.Fatal(err)
		}
		if in["state"] != "approved" || in["body"] != "Ship it" {
			t.Fatalf("review body = %#v", in)
		}
		writeTestJSON(t, w, map[string]any{
			"id":    "review_1",
			"state": "approved",
		})
	}))
	defer srv.Close()
	setRepositoryCLIEnv(t, srv.URL)

	got, err := Execute([]string{"repos", "prs", "review", "repo_1", "7", "--approve", "--body", "Ship it"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "Approved pull request #7") {
		t.Fatalf("unexpected output %q", got)
	}
}

func TestDocsPushExplainsProtectedRepositoryWorkflow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/documents/doc_1/versions" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		writeTestJSON(t, w, map[string]any{
			"error": map[string]any{
				"code":    "pull_request_required",
				"message": "repository documents must be changed through a pull request",
				"details": map[string]any{"repository_id": "repo_1"},
			},
		})
	}))
	defer srv.Close()
	setRepositoryCLIEnv(t, srv.URL)
	html := filepath.Join(t.TempDir(), "handbook.html")
	if err := os.WriteFile(html, []byte("<h1>Updated</h1>"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Execute([]string{"docs", "push", "doc_1", html, "--base-version", "ver_1", "--summary", "Review me"})
	if err == nil {
		t.Fatal("expected protected repository error")
	}
	for _, want := range []string{"aidocs repos prs create", "repo_1", "doc_1", "ver_1"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err, want)
		}
	}
}

func setRepositoryCLIEnv(t *testing.T, server string) {
	t.Helper()
	t.Setenv("AIDOCS_TOKEN", "tok")
	t.Setenv("AIDOCS_SERVER", server)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("AIDOCS_DISABLE_AUTO_UPDATE", "1")
}
