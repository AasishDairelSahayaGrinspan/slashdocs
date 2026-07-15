package knowledge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestHTTPProviderClientGitHubInspectUsesLinkedCredentialAndCompare(t *testing.T) {
	t.Parallel()

	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer linked-token" {
			t.Errorf("Authorization = %q, want linked credential", got)
		}
		requests = append(requests, r.URL.RequestURI())
		switch r.URL.Path {
		case "/repos/acme/docs/branches/main":
			writeProviderJSON(t, w, map[string]any{"commit": map[string]any{"sha": "new-sha", "html_url": "https://github.test/acme/docs/commit/new-sha"}})
		case "/repos/acme/docs/compare/old-sha...new-sha":
			writeProviderJSON(t, w, map[string]any{
				"html_url": "https://github.test/acme/docs/compare/old-sha...new-sha",
				"files":    []map[string]string{{"filename": "docs/b.md"}, {"filename": "docs/a.md"}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	resolvedConnection := ""
	client := HTTPProviderClient{
		Client:           server.Client(),
		GitHubToken:      "global-token-must-not-be-used",
		GitHubAPIBaseURL: server.URL,
		AccessTokenResolver: func(_ context.Context, connectionID string) (string, error) {
			resolvedConnection = connectionID
			return "linked-token", nil
		},
	}
	result, err := client.Inspect(context.Background(), Source{
		Provider: ProviderGitHub, ExternalRepository: "acme/docs", Branch: "main", ConnectionID: "conn-github",
	}, "old-sha")
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if resolvedConnection != "conn-github" {
		t.Fatalf("resolver connection = %q, want conn-github", resolvedConnection)
	}
	if result.Revision != "new-sha" || result.Unchanged {
		t.Fatalf("Inspect() revision/unchanged = %q/%v", result.Revision, result.Unchanged)
	}
	if want := []string{"docs/a.md", "docs/b.md"}; !reflect.DeepEqual(result.ChangedFiles, want) {
		t.Fatalf("Inspect() changed files = %#v, want %#v", result.ChangedFiles, want)
	}
	if len(result.ContextItems) != 1 || result.ContextItems[0].Kind != "github_compare" {
		t.Fatalf("Inspect() context = %#v", result.ContextItems)
	}
	if len(requests) != 2 {
		t.Fatalf("requests = %#v, want branch and compare", requests)
	}
}

func TestHTTPProviderClientGitHubInspectFirstSyncBuildsTreeContext(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/docs/branches/trunk":
			writeProviderJSON(t, w, map[string]any{"commit": map[string]any{"sha": "head-sha"}})
		case "/repos/acme/docs/git/trees/head-sha":
			if r.URL.Query().Get("recursive") != "1" {
				t.Errorf("recursive = %q, want 1", r.URL.Query().Get("recursive"))
			}
			writeProviderJSON(t, w, map[string]any{
				"truncated": false,
				"tree": []map[string]string{
					{"path": "README.md", "type": "blob"},
					{"path": "docs", "type": "tree"},
					{"path": "docs/guide.md", "type": "blob"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, err := (HTTPProviderClient{Client: server.Client(), GitHubAPIBaseURL: server.URL}).Inspect(
		context.Background(), Source{Provider: ProviderGitHub, ExternalRepository: "https://github.com/acme/docs.git", Branch: "trunk"}, "",
	)
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if want := []string{"README.md", "docs/guide.md"}; !reflect.DeepEqual(result.ChangedFiles, want) {
		t.Fatalf("Inspect() changed files = %#v, want %#v", result.ChangedFiles, want)
	}
	if len(result.ContextItems) != 1 || result.ContextItems[0].Kind != "github_repository_snapshot" || !strings.Contains(result.ContextItems[0].Body, "README.md") {
		t.Fatalf("Inspect() first-sync context = %#v", result.ContextItems)
	}
}

func TestHTTPProviderClientLegacyGitHubCompareUsesGlobalToken(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer legacy-token" {
			t.Errorf("Authorization = %q, want legacy token", got)
		}
		writeProviderJSON(t, w, map[string]any{
			"html_url": "https://github.test/compare", "files": []map[string]string{{"filename": "README.md"}},
		})
	}))
	defer server.Close()

	result, err := (HTTPProviderClient{Client: server.Client(), GitHubToken: "legacy-token", GitHubAPIBaseURL: server.URL}).Compare(
		context.Background(), Source{Provider: ProviderGitHub, ExternalRepository: "acme/docs"}, "before", "after",
	)
	if err != nil {
		t.Fatalf("Compare() error = %v", err)
	}
	if want := []string{"README.md"}; !reflect.DeepEqual(result.ChangedFiles, want) {
		t.Fatalf("Compare() changed files = %#v, want %#v", result.ChangedFiles, want)
	}
}

func TestHTTPProviderClientLinkedCredentialNeverFallsBackOrLeaks(t *testing.T) {
	t.Parallel()

	const resolverSecret = "resolver-secret-xoxp"
	client := HTTPProviderClient{
		GitHubToken: "global-secret-ghp",
		AccessTokenResolver: func(context.Context, string) (string, error) {
			return "", errors.New("credential refresh failed: " + resolverSecret)
		},
	}
	_, err := client.Inspect(context.Background(), Source{
		Provider: ProviderGitHub, ExternalRepository: "acme/docs", ConnectionID: "conn-broken",
	}, "")
	if err == nil {
		t.Fatal("Inspect() error = nil, want credential error")
	}
	if !errors.Is(err, ErrProviderCredentialUnavailable) {
		t.Fatalf("Inspect() error = %v, want ErrProviderCredentialUnavailable", err)
	}
	for _, secret := range []string{resolverSecret, "global-secret-ghp"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("Inspect() error leaked credential %q: %v", secret, err)
		}
	}
}

func TestHTTPProviderClientSlackInspectPaginatesHashesAndHandlesRetryAfter(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/conversations.replies" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer slack-token" {
			t.Errorf("Authorization = %q, want Slack token", got)
		}
		if r.URL.Query().Get("limit") != "15" {
			t.Errorf("limit = %q, want 15", r.URL.Query().Get("limit"))
		}
		if r.URL.Query().Get("cursor") == "" {
			writeProviderJSON(t, w, map[string]any{
				"ok":                true,
				"messages":          []map[string]string{{"type": "message", "ts": "171.001", "text": "parent", "user": "U1"}},
				"response_metadata": map[string]string{"next_cursor": "page-two"},
			})
			return
		}
		if r.URL.Query().Get("cursor") != "page-two" {
			t.Errorf("cursor = %q, want page-two", r.URL.Query().Get("cursor"))
			http.Error(w, "unexpected cursor", http.StatusBadRequest)
			return
		}
		writeProviderJSON(t, w, map[string]any{
			"ok":                true,
			"messages":          []map[string]string{{"type": "message", "ts": "171.002", "thread_ts": "171.001", "text": "reply", "user": "U2"}},
			"response_metadata": map[string]string{"next_cursor": ""},
		})
	}))
	defer server.Close()

	client := HTTPProviderClient{
		Client:          server.Client(),
		SlackAPIBaseURL: server.URL,
		AccessTokenResolver: func(context.Context, string) (string, error) {
			return "slack-token", nil
		},
	}
	source := Source{Provider: ProviderSlack, ExternalRepository: "T1/C1/171.001", ConnectionID: "conn-slack"}
	first, err := client.Inspect(context.Background(), source, "")
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if first.Revision == "" || len(first.ContextItems) != 2 || first.Unchanged {
		t.Fatalf("Inspect() result = %#v", first)
	}
	second, err := client.Inspect(context.Background(), source, first.Revision)
	if err != nil {
		t.Fatalf("Inspect(previous revision) error = %v", err)
	}
	if !second.Unchanged || len(second.ChangedFiles) != 0 || second.Revision != first.Revision {
		t.Fatalf("Inspect(previous revision) result = %#v", second)
	}

	rateServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer rateServer.Close()
	client.Client = rateServer.Client()
	client.SlackAPIBaseURL = rateServer.URL
	_, err = client.Inspect(context.Background(), source, "")
	var rateErr *RateLimitError
	if !errors.As(err, &rateErr) {
		t.Fatalf("Inspect() error = %v, want RateLimitError", err)
	}
	if rateErr.RetryAfter != 7*time.Second {
		t.Fatalf("RetryAfter = %s, want 7s", rateErr.RetryAfter)
	}
}

func TestHTTPProviderClientGoogleDriveInspectExportsTextAndHashes(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer drive-token" {
			t.Errorf("Authorization = %q, want Drive token", got)
		}
		switch r.URL.Path {
		case "/files/file-123":
			if r.URL.Query().Get("fields") == "" {
				t.Error("metadata request omitted fields")
			}
			writeProviderJSON(t, w, map[string]any{
				"id": "file-123", "name": "Domain context", "mimeType": "application/vnd.google-apps.document",
				"modifiedTime": "2026-07-15T10:00:00Z", "version": "7", "webViewLink": "https://drive.test/file-123",
				"capabilities": map[string]bool{"canDownload": true},
			})
		case "/files/file-123/export":
			if got := r.URL.Query().Get("mimeType"); got != "text/plain" {
				t.Errorf("export mimeType = %q, want text/plain", got)
			}
			_, _ = fmt.Fprint(w, "Payments domain context\n")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := HTTPProviderClient{
		Client:                server.Client(),
		GoogleDriveAPIBaseURL: server.URL,
		AccessTokenResolver:   func(context.Context, string) (string, error) { return "drive-token", nil },
	}
	source := Source{Provider: ProviderGoogleDrive, ExternalRepository: "https://docs.google.com/document/d/file-123/edit", ConnectionID: "conn-drive"}
	first, err := client.Inspect(context.Background(), source, "")
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if first.Revision == "" || len(first.ContextItems) != 1 || first.ContextItems[0].Body != "Payments domain context\n" {
		t.Fatalf("Inspect() result = %#v", first)
	}
	if want := []string{"drive/Domain-context"}; !reflect.DeepEqual(first.ChangedFiles, want) {
		t.Fatalf("Inspect() changed files = %#v, want %#v", first.ChangedFiles, want)
	}
	second, err := client.Inspect(context.Background(), source, first.Revision)
	if err != nil {
		t.Fatalf("Inspect(previous revision) error = %v", err)
	}
	if !second.Unchanged || len(second.ChangedFiles) != 0 || second.Revision != first.Revision {
		t.Fatalf("Inspect(previous revision) result = %#v", second)
	}
}

func TestHTTPProviderClientGoogleDriveInspectDownloadsPlainTextFile(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/files/file-markdown" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("alt") == "media" {
			_, _ = fmt.Fprint(w, "# Runbook\n")
			return
		}
		writeProviderJSON(t, w, map[string]any{
			"id": "file-markdown", "name": "runbook.md", "mimeType": "text/markdown", "version": 3,
		})
	}))
	defer server.Close()

	client := HTTPProviderClient{
		Client:                server.Client(),
		GoogleDriveAPIBaseURL: server.URL,
		AccessTokenResolver:   func(context.Context, string) (string, error) { return "drive-token", nil },
	}
	result, err := client.Inspect(context.Background(), Source{
		Provider: ProviderGoogleDrive, ExternalRepository: "file-markdown", ConnectionID: "conn-drive",
	}, "")
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if len(result.ContextItems) != 1 || result.ContextItems[0].Body != "# Runbook\n" {
		t.Fatalf("Inspect() result = %#v", result)
	}
}

func TestHTTPProviderClientDevRevInspectPostsWorkAndHashes(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/works.get" {
			t.Errorf("request = %s %s, want POST /works.get", r.Method, r.URL.Path)
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer devrev-token" {
			t.Errorf("Authorization = %q, want DevRev token", got)
		}
		var request map[string]string
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if request["id"] != "TKT-42" {
			t.Errorf("work id = %q, want TKT-42", request["id"])
			http.Error(w, "unexpected work id", http.StatusBadRequest)
			return
		}
		writeProviderJSON(t, w, map[string]any{"work": map[string]any{
			"id": "don:core:dvrv-us-1:devo/1:ticket/42", "display_id": "TKT-42", "type": "ticket",
			"title": "Payment retry fails", "body": "Retry the gateway request safely.", "state": "open",
			"modified_date": "2026-07-15T10:00:00Z", "web_url": "https://app.devrev.test/tickets/TKT-42",
		}})
	}))
	defer server.Close()

	client := HTTPProviderClient{
		Client:              server.Client(),
		DevRevAPIBaseURL:    server.URL,
		AccessTokenResolver: func(context.Context, string) (string, error) { return "devrev-token", nil },
	}
	source := Source{Provider: ProviderDevRev, ExternalRepository: "TKT-42", ConnectionID: "conn-devrev"}
	first, err := client.Inspect(context.Background(), source, "")
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if first.Revision == "" || first.ProvenanceURL != "https://app.devrev.test/tickets/TKT-42" || len(first.ContextItems) != 1 {
		t.Fatalf("Inspect() result = %#v", first)
	}
	if want := []string{"devrev/works/TKT-42.txt"}; !reflect.DeepEqual(first.ChangedFiles, want) {
		t.Fatalf("Inspect() changed files = %#v, want %#v", first.ChangedFiles, want)
	}
	second, err := client.Inspect(context.Background(), source, first.Revision)
	if err != nil {
		t.Fatalf("Inspect(previous revision) error = %v", err)
	}
	if !second.Unchanged || len(second.ChangedFiles) != 0 || second.Revision != first.Revision {
		t.Fatalf("Inspect(previous revision) result = %#v", second)
	}
}

func writeProviderJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}
