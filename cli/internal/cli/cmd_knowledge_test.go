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

func TestRepositorySourceAddAndCheck(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch requests {
		case 1:
			if r.Method != http.MethodPost || r.URL.Path != "/v1/repositories/repo_1/sources" {
				t.Fatalf("source add request = %s %s", r.Method, r.URL.Path)
			}
			var in map[string]any
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				t.Fatal(err)
			}
			if in["provider"] != "github" || in["external_repository"] != "razorpay/payments" || in["target_document_id"] != "doc_1" {
				t.Fatalf("source body = %#v", in)
			}
			writeTestJSON(t, w, map[string]any{"id": "src_1", "provider": "github", "external_repository": "razorpay/payments"})
		case 2:
			if r.Method != http.MethodPost || r.URL.Path != "/v1/repositories/repo_1/sources/src_1/check" {
				t.Fatalf("source check request = %s %s", r.Method, r.URL.Path)
			}
			var in map[string]any
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				t.Fatal(err)
			}
			files, _ := in["changed_files"].([]any)
			if in["after_sha"] != "abc123" || len(files) != 1 || files[0] != "payments/refunds.go" {
				t.Fatalf("check body = %#v", in)
			}
			writeTestJSON(t, w, map[string]any{"id": "sync_1", "outcome": "pull_request_opened", "pull_request_number": 4})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	setRepositoryCLIEnv(t, srv.URL)

	got, err := Execute([]string{"repos", "sources", "add", "repo_1", "--provider", "github", "--repository", "razorpay/payments", "--target-document", "doc_1"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "Connected github source") || !strings.Contains(got, "src_1") {
		t.Fatalf("source output = %q", got)
	}
	got, err = Execute([]string{"repos", "sources", "check", "repo_1", "src_1", "--after", "abc123", "--changed-file", "payments/refunds.go"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "pull_request_opened") {
		t.Fatalf("check output = %q", got)
	}
}

func TestRepositorySearchAndAskEscapeQuery(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch requests {
		case 1:
			if r.Method != http.MethodGet || r.URL.Path != "/v1/repositories/repo_1/search" || r.URL.Query().Get("q") != "refund ownership" {
				t.Fatalf("search request = %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
			}
			writeTestJSON(t, w, map[string]any{"items": []any{map[string]any{"document_id": "doc_1", "title": "Refunds"}}})
		case 2:
			if r.Method != http.MethodPost || r.URL.Path != "/v1/repositories/repo_1/questions" {
				t.Fatalf("question request = %s %s", r.Method, r.URL.Path)
			}
			var in map[string]string
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				t.Fatal(err)
			}
			if in["question"] != "Who owns refunds?" {
				t.Fatalf("question body = %#v", in)
			}
			writeTestJSON(t, w, map[string]any{"answer": "Payments owns refunds.", "citations": []any{map[string]any{"document_id": "doc_1"}}})
		default:
			t.Fatalf("unexpected request")
		}
	}))
	defer srv.Close()
	setRepositoryCLIEnv(t, srv.URL)

	if _, err := Execute([]string{"repos", "search", "repo_1", "refund ownership"}); err != nil {
		t.Fatal(err)
	}
	got, err := Execute([]string{"repos", "ask", "repo_1", "Who owns refunds?"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "Payments owns refunds") || !strings.Contains(got, "doc_1") {
		t.Fatalf("ask output = %q", got)
	}
}

func TestRepositoryChangeSetAndRebase(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch requests {
		case 1:
			if r.Method != http.MethodPost || r.URL.Path != "/v1/repositories/repo_1/pull-requests/change-set" {
				t.Fatalf("change-set request = %s %s", r.Method, r.URL.Path)
			}
			var in map[string]any
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				t.Fatal(err)
			}
			changes, _ := in["changes"].([]any)
			if in["title"] != "Refresh knowledge" || len(changes) != 2 {
				t.Fatalf("change-set body = %#v", in)
			}
			writeTestJSON(t, w, map[string]any{"id": "pr_1", "number": 8, "patchset": 1})
		case 2:
			if r.Method != http.MethodPost || r.URL.Path != "/v1/repositories/repo_1/pull-requests/8/rebase" {
				t.Fatalf("rebase request = %s %s", r.Method, r.URL.Path)
			}
			writeTestJSON(t, w, map[string]any{"id": "pr_1", "number": 8, "patchset": 2})
		default:
			t.Fatalf("unexpected request")
		}
	}))
	defer srv.Close()
	setRepositoryCLIEnv(t, srv.URL)

	spec := filepath.Join(t.TempDir(), "changes.json")
	if err := os.WriteFile(spec, []byte(`{"title":"Refresh knowledge","changes":[{"operation":"update","document_id":"doc_1","base_version_id":"ver_1","content_html":"<h1>Updated</h1>"},{"operation":"rename","document_id":"doc_2","title":"New title"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Execute([]string{"repos", "prs", "change-set", "repo_1", spec}); err != nil {
		t.Fatal(err)
	}
	got, err := Execute([]string{"repos", "prs", "rebase", "repo_1", "8"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "Rebased pull request #8") {
		t.Fatalf("rebase output = %q", got)
	}
}

func TestRepositorySettingsUpdateAndTransferLifecycle(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch requests {
		case 1:
			if r.Method != http.MethodPatch || r.URL.Path != "/v1/repositories/repo_1/settings" {
				t.Fatalf("settings update request = %s %s", r.Method, r.URL.Path)
			}
			var in map[string]any
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				t.Fatal(err)
			}
			if in["required_reviewers"] == nil || in["freshness_policy"] == nil {
				t.Fatalf("settings body = %#v", in)
			}
			writeTestJSON(t, w, in)
		case 2:
			if r.Method != http.MethodPost || r.URL.Path != "/v1/repositories/repo_1/transfer" {
				t.Fatalf("transfer request = %s %s", r.Method, r.URL.Path)
			}
			var in map[string]any
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				t.Fatal(err)
			}
			if in["new_owner_email"] != "new-owner@example.com" {
				t.Fatalf("transfer body = %#v", in)
			}
			writeTestJSON(t, w, map[string]any{"repository_id": "repo_1", "transfer": map[string]any{"status": "pending"}})
		case 3:
			if r.Method != http.MethodPost || r.URL.Path != "/v1/repositories/repo_1/transfer/accept" {
				t.Fatalf("accept request = %s %s", r.Method, r.URL.Path)
			}
			writeTestJSON(t, w, map[string]any{"repository_id": "repo_1", "transfer": map[string]any{"status": "accepted"}})
		case 4:
			if r.Method != http.MethodPost || r.URL.Path != "/v1/repositories/repo_1/transfer/cancel" {
				t.Fatalf("cancel request = %s %s", r.Method, r.URL.Path)
			}
			writeTestJSON(t, w, map[string]any{"repository_id": "repo_1"})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	setRepositoryCLIEnv(t, srv.URL)

	settings := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(settings, []byte(`{"required_reviewers":["usr_1"],"freshness_policy":{"stale_after_hours":24,"on_stale":"notify"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Execute([]string{"repos", "settings", "update", "repo_1", settings}); err != nil {
		t.Fatal(err)
	}
	got, err := Execute([]string{"repos", "settings", "transfer", "repo_1", "--to", "new-owner@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "Requested transfer") || !strings.Contains(got, "must accept") {
		t.Fatalf("pending transfer output = %q", got)
	}
	got, err = Execute([]string{"repos", "settings", "transfer", "accept", "repo_1"})
	if err != nil || !strings.Contains(got, "Accepted repository transfer") {
		t.Fatalf("accept output = %q, err = %v", got, err)
	}
	got, err = Execute([]string{"repos", "settings", "transfer", "cancel", "repo_1"})
	if err != nil || !strings.Contains(got, "Cancelled repository transfer") {
		t.Fatalf("cancel output = %q, err = %v", got, err)
	}
}

func TestRepositorySourceUpdateFromJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/v1/repositories/repo_1/sources/src_1" {
			t.Fatalf("source update request = %s %s", r.Method, r.URL.Path)
		}
		var in map[string]any
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			t.Fatal(err)
		}
		if in["branch"] != "develop" || in["status"] != "paused" {
			t.Fatalf("source update body = %#v", in)
		}
		writeTestJSON(t, w, map[string]any{"id": "src_1", "branch": "develop", "status": "paused"})
	}))
	defer srv.Close()
	setRepositoryCLIEnv(t, srv.URL)

	settings := filepath.Join(t.TempDir(), "source.json")
	if err := os.WriteFile(settings, []byte(`{"branch":"develop","status":"paused"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Execute([]string{"repos", "sources", "update", "repo_1", "src_1", settings}); err != nil {
		t.Fatal(err)
	}
}

func TestRepositoryTeamsCRUDAndMembership(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch requests {
		case 1:
			if r.Method != http.MethodGet || r.URL.Path != "/v1/repositories/repo_1/teams" {
				t.Fatalf("team list request = %s %s", r.Method, r.URL.Path)
			}
			writeTestJSON(t, w, map[string]any{"items": []any{}})
		case 2:
			if r.Method != http.MethodPost || r.URL.Path != "/v1/repositories/repo_1/teams" {
				t.Fatalf("team create request = %s %s", r.Method, r.URL.Path)
			}
			var in map[string]any
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				t.Fatal(err)
			}
			members, _ := in["member_ids"].([]any)
			if in["name"] != "Payments" || len(members) != 2 {
				t.Fatalf("team create body = %#v", in)
			}
			writeTestJSON(t, w, map[string]any{"id": "team_1", "name": "Payments"})
		case 3:
			if r.Method != http.MethodPatch || r.URL.Path != "/v1/repositories/repo_1/teams/team_1" {
				t.Fatalf("team update request = %s %s", r.Method, r.URL.Path)
			}
			var in map[string]any
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				t.Fatal(err)
			}
			if in["description"] != "Payment reviewers" {
				t.Fatalf("team update body = %#v", in)
			}
			writeTestJSON(t, w, map[string]any{"id": "team_1", "name": "Payments"})
		case 4:
			if r.Method != http.MethodPut || r.URL.Path != "/v1/repositories/repo_1/teams/team_1/members/usr_3" {
				t.Fatalf("member add request = %s %s", r.Method, r.URL.Path)
			}
			writeTestJSON(t, w, map[string]any{"id": "team_1"})
		case 5:
			if r.Method != http.MethodDelete || r.URL.Path != "/v1/repositories/repo_1/teams/team_1/members/usr_2" {
				t.Fatalf("member remove request = %s %s", r.Method, r.URL.Path)
			}
			writeTestJSON(t, w, map[string]any{"id": "team_1"})
		case 6:
			if r.Method != http.MethodDelete || r.URL.Path != "/v1/repositories/repo_1/teams/team_1" {
				t.Fatalf("team delete request = %s %s", r.Method, r.URL.Path)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	setRepositoryCLIEnv(t, srv.URL)

	commands := [][]string{
		{"repos", "teams", "list", "repo_1"},
		{"repos", "teams", "create", "repo_1", "--name", "Payments", "--member", "usr_1", "--member", "usr_2"},
		{"repos", "teams", "update", "repo_1", "team_1", "--description", "Payment reviewers"},
		{"repos", "teams", "members", "add", "repo_1", "team_1", "usr_3"},
		{"repos", "teams", "member", "remove", "repo_1", "team_1", "usr_2"},
		{"repos", "teams", "delete", "repo_1", "team_1"},
	}
	for _, command := range commands {
		if _, err := Execute(command); err != nil {
			t.Fatalf("Execute(%v): %v", command, err)
		}
	}
}

func TestRepositoryBacklinksAndTemplateApplyFlags(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch requests {
		case 1:
			if r.Method != http.MethodGet || r.URL.Path != "/v1/repositories/repo_1/backlinks" || r.URL.Query().Get("document_id") != "doc_1" {
				t.Fatalf("backlinks request = %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
			}
			writeTestJSON(t, w, map[string]any{"document_id": "doc_1", "items": []any{}})
		case 2:
			if r.Method != http.MethodPost || r.URL.Path != "/v1/repositories/repo_1/templates/runbook/apply" {
				t.Fatalf("template request = %s %s", r.Method, r.URL.Path)
			}
			var in map[string]any
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				t.Fatal(err)
			}
			if in["target_document_id"] != "doc_1" || in["title"] != "Payments runbook" || in["preview_only"] != true {
				t.Fatalf("template body = %#v", in)
			}
			writeTestJSON(t, w, map[string]any{"status": "preview"})
		default:
			t.Fatalf("unexpected request")
		}
	}))
	defer srv.Close()
	setRepositoryCLIEnv(t, srv.URL)

	if _, err := Execute([]string{"repos", "backlinks", "repo_1", "doc_1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Execute([]string{"templates", "apply", "repo_1", "runbook", "--target-document", "doc_1", "--title", "Payments runbook", "--preview"}); err != nil {
		t.Fatal(err)
	}
}
