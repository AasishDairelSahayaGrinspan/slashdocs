package server_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/razorpay/aidocs/api/internal/auth"
	"github.com/razorpay/aidocs/api/internal/repo"
	"github.com/razorpay/aidocs/api/internal/server"
)

const (
	githubTestAppOrigin    = "https://aidocs.example.com"
	githubTestRenderOrigin = "https://aidocs-render.example.com"
	githubTestAccessToken  = "gh-access-token"
)

// fakeGitHub stands in for github.com: it answers the OAuth token exchange,
// the profile lookup, and repository content reads, and records the content
// paths it served so a test can prove a refused request never reached GitHub.
type fakeGitHub struct {
	mu sync.Mutex
	// files maps "owner/repo/path" to file content.
	files    map[string]string
	requests []string
}

func newFakeGitHub() *fakeGitHub {
	return &fakeGitHub{files: map[string]string{
		"acme/docs/README.md":    "# Public handbook",
		"acme/secrets/.env":      "STRIPE_KEY=sk_live_1",
		"contoso/docs/README.md": "# Other tenant",
	}}
}

func (f *fakeGitHub) RoundTrip(req *http.Request) (*http.Response, error) {
	switch {
	case req.URL.Host == "github.com" && req.URL.Path == "/login/oauth/access_token":
		return fakeGitHubResponse(http.StatusOK, "application/json",
			fmt.Sprintf(`{"access_token":%q,"token_type":"bearer","scope":"repo"}`, githubTestAccessToken)), nil
	case req.URL.Host == "api.github.com" && req.URL.Path == "/user":
		return fakeGitHubResponse(http.StatusOK, "application/json", `{"id":42,"login":"octo-reader","avatar_url":""}`), nil
	case req.URL.Host == "api.github.com" && strings.HasPrefix(req.URL.Path, "/repos/"):
		if got := req.Header.Get("Authorization"); got != "Bearer "+githubTestAccessToken {
			return fakeGitHubResponse(http.StatusUnauthorized, "application/json", `{"message":"Bad credentials"}`), nil
		}
		// GitHub resolves owner and repository case-insensitively; the file
		// path keeps its case.
		key := strings.Replace(strings.TrimPrefix(req.URL.Path, "/repos/"), "/contents/", "/", 1)
		if parts := strings.SplitN(key, "/", 3); len(parts) == 3 {
			key = strings.ToLower(parts[0]) + "/" + strings.ToLower(parts[1]) + "/" + parts[2]
		}
		f.mu.Lock()
		f.requests = append(f.requests, key)
		content, ok := f.files[key]
		f.mu.Unlock()
		if !ok {
			return fakeGitHubResponse(http.StatusNotFound, "application/json", `{"message":"Not Found"}`), nil
		}
		res := fakeGitHubResponse(http.StatusOK, "text/plain; charset=utf-8", content)
		res.Header.Set("ETag", `W/"etag-1"`)
		return res, nil
	}
	return nil, fmt.Errorf("unexpected GitHub request %s %s", req.Method, req.URL)
}

// served returns the "owner/repo/path" of every content read GitHub was asked
// for, in order.
func (f *fakeGitHub) served() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.requests...)
}

func fakeGitHubResponse(status int, contentType, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{contentType}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func newGitHubTestEnv(t *testing.T) (*testEnv, *fakeGitHub) {
	t.Helper()
	github := newFakeGitHub()
	e := newConfiguredTestEnv(t, server.Config{
		Environment:        "test",
		AppOrigin:          githubTestAppOrigin,
		RenderOrigin:       githubTestRenderOrigin,
		SessionSecret:      "github-consent-test-secret",
		GitHubClientID:     "client-id",
		GitHubClientSecret: "client-secret",
	}, server.WithGitHubHTTPClient(&http.Client{Transport: github}))
	return e, github
}

// connectGitHub runs the real connect flow against the fake GitHub, so tests
// read content through a stored token instead of seeding ciphertext.
func connectGitHub(e *testEnv, p auth.Principal) {
	e.t.Helper()
	start := do(e.t, e.h, http.MethodGet, "/v1/integrations/github/start", "", map[string]string{
		"X-Test-Principal": principalHeader(p),
	})
	assertStatus(e.t, start, http.StatusFound)
	location, err := url.Parse(start.Header().Get("Location"))
	if err != nil {
		e.t.Fatalf("parse github authorize redirect: %v", err)
	}
	state := location.Query().Get("state")
	if state == "" {
		e.t.Fatal("github authorize redirect carried no state")
	}
	callback := do(e.t, e.h, http.MethodGet, "/v1/integrations/github/callback?code=test-code&state="+url.QueryEscape(state), "", map[string]string{
		"X-Test-Principal": principalHeader(p),
	})
	assertStatus(e.t, callback, http.StatusFound)
}

func githubRaw(e *testEnv, docID string, p auth.Principal, owner, repoName, path string) *httptest.ResponseRecorder {
	e.t.Helper()
	return do(e.t, e.h, http.MethodPost, "/v1/documents/"+docID+"/github/raw",
		fmt.Sprintf(`{"owner":%q,"repo":%q,"path":%q,"ref":"main"}`, owner, repoName, path),
		map[string]string{
			"Content-Type":     "application/json",
			"X-Test-Principal": principalHeader(p),
		})
}

func githubGrantConsent(e *testEnv, docID string, p auth.Principal, owner, repoName string) *httptest.ResponseRecorder {
	e.t.Helper()
	return do(e.t, e.h, http.MethodPost, "/v1/documents/"+docID+"/github/consents",
		fmt.Sprintf(`{"owner":%q,"repo":%q}`, owner, repoName),
		map[string]string{
			"Content-Type":     "application/json",
			"X-Test-Principal": principalHeader(p),
		})
}

func githubListConsents(e *testEnv, docID string, p auth.Principal) *httptest.ResponseRecorder {
	e.t.Helper()
	return do(e.t, e.h, http.MethodGet, "/v1/documents/"+docID+"/github/consents", "", map[string]string{
		"X-Test-Principal": principalHeader(p),
	})
}

// A document's JavaScript picks the repository coordinates, so a viewer role
// on the document must not let it spend the viewer's GitHub token.
func TestGitHubRawContentRefusesRepositoryWithoutConsent(t *testing.T) {
	e, github := newGitHubTestEnv(t)
	connectGitHub(e, fixtureViewer)

	rr := githubRaw(e, e.doc.ID, fixtureViewer, "acme", "secrets", ".env")

	assertStatus(t, rr, http.StatusForbidden)
	assertJSON(t, rr.Body.Bytes(), `{
	  "error": {
	    "code": "github_consent_required",
	    "message": "Approve this document reading acme/secrets before it can load repository content",
	    "details": { "owner": "acme", "repo": "secrets" }
	  }
	}`)
	if served := github.served(); len(served) != 0 {
		t.Fatalf("refused request still reached GitHub: %v", served)
	}
}

func TestGitHubRawContentReturnsContentAfterConsent(t *testing.T) {
	e, github := newGitHubTestEnv(t)
	connectGitHub(e, fixtureViewer)

	grant := githubGrantConsent(e, e.doc.ID, fixtureViewer, "acme", "docs")
	assertStatus(t, grant, http.StatusCreated)

	rr := githubRaw(e, e.doc.ID, fixtureViewer, "acme", "docs", "README.md")

	assertStatus(t, rr, http.StatusOK)
	assertJSON(t, rr.Body.Bytes(), `{
	  "owner": "acme",
	  "repo": "docs",
	  "path": "README.md",
	  "ref": "main",
	  "content": "# Public handbook",
	  "content_type": "text/plain; charset=utf-8",
	  "etag": "W/\"etag-1\""
	}`)
	if served := github.served(); len(served) != 1 || served[0] != "acme/docs/README.md" {
		t.Fatalf("GitHub reads = %v, want one read of acme/docs/README.md", served)
	}
}

func TestGitHubRepoConsentCoversOnlyTheApprovedRepository(t *testing.T) {
	e, github := newGitHubTestEnv(t)
	connectGitHub(e, fixtureViewer)
	assertStatus(t, githubGrantConsent(e, e.doc.ID, fixtureViewer, "acme", "docs"), http.StatusCreated)

	for _, tc := range []struct{ owner, repoName, path string }{
		{"acme", "secrets", ".env"},
		{"contoso", "docs", "README.md"},
	} {
		rr := githubRaw(e, e.doc.ID, fixtureViewer, tc.owner, tc.repoName, tc.path)
		assertStatus(t, rr, http.StatusForbidden)
		if code := jsonErrorCode(t, rr.Body.Bytes()); code != "github_consent_required" {
			t.Fatalf("%s/%s error code = %q, want github_consent_required", tc.owner, tc.repoName, code)
		}
	}
	if served := github.served(); len(served) != 0 {
		t.Fatalf("refused requests still reached GitHub: %v", served)
	}
}

func TestGitHubRepoConsentCoversOnlyTheApprovingUser(t *testing.T) {
	e, _ := newGitHubTestEnv(t)
	connectGitHub(e, fixtureViewer)
	connectGitHub(e, fixtureEditor)
	assertStatus(t, githubGrantConsent(e, e.doc.ID, fixtureViewer, "acme", "docs"), http.StatusCreated)

	rr := githubRaw(e, e.doc.ID, fixtureEditor, "acme", "docs", "README.md")

	assertStatus(t, rr, http.StatusForbidden)
	if code := jsonErrorCode(t, rr.Body.Bytes()); code != "github_consent_required" {
		t.Fatalf("error code = %q, want github_consent_required", code)
	}
	assertJSON(t, githubListConsents(e, e.doc.ID, fixtureEditor).Body.Bytes(), `{"items": []}`)
}

func TestGitHubRepoConsentCoversOnlyTheApprovedDocument(t *testing.T) {
	e, _ := newGitHubTestEnv(t)
	connectGitHub(e, fixtureViewer)
	otherDoc, _ := e.createDocument("second", "<html><body>second</body></html>", fixtureViewer)
	assertStatus(t, githubGrantConsent(e, e.doc.ID, fixtureViewer, "acme", "docs"), http.StatusCreated)

	rr := githubRaw(e, otherDoc, fixtureViewer, "acme", "docs", "README.md")

	assertStatus(t, rr, http.StatusForbidden)
	if code := jsonErrorCode(t, rr.Body.Bytes()); code != "github_consent_required" {
		t.Fatalf("error code = %q, want github_consent_required", code)
	}
}

// GitHub owner and repository names are case-insensitive, so consent has to be
// recorded and matched in one spelling.
func TestGitHubRepoConsentIgnoresRepositoryCase(t *testing.T) {
	e, _ := newGitHubTestEnv(t)
	connectGitHub(e, fixtureViewer)

	grant := githubGrantConsent(e, e.doc.ID, fixtureViewer, "ACME", "Docs")
	assertStatus(t, grant, http.StatusCreated)
	owner, repoName := jsonField(t, grant.Body.Bytes(), "owner"), jsonField(t, grant.Body.Bytes(), "repo")
	if owner != "acme" || repoName != "docs" {
		t.Fatalf("stored consent = %s/%s, want acme/docs", owner, repoName)
	}

	assertStatus(t, githubRaw(e, e.doc.ID, fixtureViewer, "Acme", "DOCS", "README.md"), http.StatusOK)

	list := githubListConsents(e, e.doc.ID, fixtureViewer)
	assertStatus(t, list, http.StatusOK)
	if body := list.Body.String(); !strings.Contains(body, `"owner":"acme"`) || !strings.Contains(body, `"repo":"docs"`) {
		t.Fatalf("consent list = %s, want the lowercased repository", body)
	}
}

func TestGitHubRepoConsentRejectsInvalidCoordinates(t *testing.T) {
	e, github := newGitHubTestEnv(t)
	connectGitHub(e, fixtureViewer)

	for _, tc := range []struct {
		name            string
		owner, repoName string
	}{
		{"owner with a slash", "acme/evil", "docs"},
		{"owner with consecutive hyphens", "ac--me", "docs"},
		{"owner with a leading hyphen", "-acme", "docs"},
		{"owner with a trailing hyphen", "acme-", "docs"},
		{"owner that is too long", strings.Repeat("a", 40), "docs"},
		{"owner with a space", "ac me", "docs"},
		{"empty owner", "", "docs"},
		{"repo with a slash", "acme", "docs/evil"},
		{"repo that is a dot", "acme", "."},
		{"repo that is two dots", "acme", ".."},
		{"repo with a space", "acme", "do cs"},
		{"repo that is too long", "acme", strings.Repeat("d", 101)},
		{"empty repo", "acme", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertStatus(t, githubGrantConsent(e, e.doc.ID, fixtureViewer, tc.owner, tc.repoName), http.StatusBadRequest)
			assertStatus(t, githubRaw(e, e.doc.ID, fixtureViewer, tc.owner, tc.repoName, "README.md"), http.StatusBadRequest)
		})
	}
	if served := github.served(); len(served) != 0 {
		t.Fatalf("rejected coordinates still reached GitHub: %v", served)
	}
}

// The render origin is same-site with the app origin, so a SameSite=Lax
// session cookie rides along with a cross-origin request from a document.
// Recording consent from there would defeat the whole prompt.
func TestGitHubRepoConsentGrantRejectsRenderOriginRequest(t *testing.T) {
	e, _ := newGitHubTestEnv(t)
	connectGitHub(e, fixtureViewer)

	for _, tc := range []struct {
		name        string
		contentType string
		headers     map[string]string
		want        int
	}{
		{"json fetch with the render origin", "application/json", map[string]string{"Origin": githubTestRenderOrigin}, http.StatusForbidden},
		{"json fetch with only a render referer", "application/json", map[string]string{"Referer": githubTestRenderOrigin + "/v/doc/version"}, http.StatusForbidden},
		{"simple form post from the render origin", "application/x-www-form-urlencoded", map[string]string{"Origin": githubTestRenderOrigin}, http.StatusForbidden},
		{"simple text post from the render origin", "text/plain;charset=UTF-8", map[string]string{"Origin": githubTestRenderOrigin}, http.StatusForbidden},
		// A cookie request that names no origin at all is refused too, so a
		// stripped Origin header is not a way around the check.
		{"post with a cookie and no origin or referer", "application/json", nil, http.StatusForbidden},
		// And even from the app origin, consent is only accepted as JSON, so
		// no form or text post — the shapes a browser sends without a
		// preflight — can record one.
		{"simple form post from the app origin", "application/x-www-form-urlencoded", map[string]string{"Origin": githubTestAppOrigin}, http.StatusUnsupportedMediaType},
		{"simple text post from the app origin", "text/plain;charset=UTF-8", map[string]string{"Origin": githubTestAppOrigin}, http.StatusUnsupportedMediaType},
	} {
		t.Run(tc.name, func(t *testing.T) {
			headers := map[string]string{
				"Content-Type":     tc.contentType,
				"Cookie":           "aidocs_session=test-session",
				"X-Test-Principal": principalHeader(fixtureViewer),
			}
			for k, v := range tc.headers {
				headers[k] = v
			}
			rr := do(t, e.h, http.MethodPost, "/v1/documents/"+e.doc.ID+"/github/consents",
				`{"owner":"acme","repo":"docs"}`, headers)
			assertStatus(t, rr, tc.want)
		})
	}

	assertJSON(t, githubListConsents(e, e.doc.ID, fixtureViewer).Body.Bytes(), `{"items": []}`)
	assertStatus(t, githubRaw(e, e.doc.ID, fixtureViewer, "acme", "docs", "README.md"), http.StatusForbidden)
}

func TestGitHubRepoConsentRequiresDocumentAccess(t *testing.T) {
	e, _ := newGitHubTestEnv(t)
	connectGitHub(e, fixtureStranger)

	assertStatus(t, githubGrantConsent(e, e.doc.ID, fixtureStranger, "acme", "docs"), http.StatusForbidden)
	assertStatus(t, githubListConsents(e, e.doc.ID, fixtureStranger), http.StatusForbidden)
	assertStatus(t, githubRaw(e, e.doc.ID, fixtureStranger, "acme", "docs", "README.md"), http.StatusForbidden)
}

func TestGitHubRepoConsentRejectsServiceAccountPrincipal(t *testing.T) {
	e, _ := newGitHubTestEnv(t)
	sa := auth.Principal{Type: auth.PrincipalServiceAccount, ID: e.sa.ID}
	if _, err := e.store.CreateGrant(t.Context(), "document", e.doc.ID, sa, repo.RoleViewer, fixtureOwner); err != nil {
		t.Fatalf("grant the service account viewer access: %v", err)
	}

	assertStatus(t, githubGrantConsent(e, e.doc.ID, sa, "acme", "docs"), http.StatusForbidden)
	assertStatus(t, githubListConsents(e, e.doc.ID, sa), http.StatusForbidden)
	assertStatus(t, githubRaw(e, e.doc.ID, sa, "acme", "docs", "README.md"), http.StatusForbidden)
}

func TestGitHubRepoConsentRevokeStopsFurtherReads(t *testing.T) {
	e, _ := newGitHubTestEnv(t)
	connectGitHub(e, fixtureViewer)
	assertStatus(t, githubGrantConsent(e, e.doc.ID, fixtureViewer, "acme", "docs"), http.StatusCreated)
	assertStatus(t, githubRaw(e, e.doc.ID, fixtureViewer, "acme", "docs", "README.md"), http.StatusOK)

	revoke := do(t, e.h, http.MethodDelete, "/v1/documents/"+e.doc.ID+"/github/consents/Acme/DOCS", "", map[string]string{
		"X-Test-Principal": principalHeader(fixtureViewer),
	})
	assertStatus(t, revoke, http.StatusNoContent)

	assertJSON(t, githubListConsents(e, e.doc.ID, fixtureViewer).Body.Bytes(), `{"items": []}`)
	assertStatus(t, githubRaw(e, e.doc.ID, fixtureViewer, "acme", "docs", "README.md"), http.StatusForbidden)
}

func TestGitHubDisconnectClearsRepoConsents(t *testing.T) {
	e, _ := newGitHubTestEnv(t)
	connectGitHub(e, fixtureViewer)
	assertStatus(t, githubGrantConsent(e, e.doc.ID, fixtureViewer, "acme", "docs"), http.StatusCreated)

	disconnect := do(t, e.h, http.MethodDelete, "/v1/integrations/github", "", map[string]string{
		"X-Test-Principal": principalHeader(fixtureViewer),
	})
	assertStatus(t, disconnect, http.StatusNoContent)

	assertJSON(t, githubListConsents(e, e.doc.ID, fixtureViewer).Body.Bytes(), `{"items": []}`)
	// Reconnecting must not re-arm the approvals the disconnect withdrew.
	connectGitHub(e, fixtureViewer)
	assertStatus(t, githubRaw(e, e.doc.ID, fixtureViewer, "acme", "docs", "README.md"), http.StatusForbidden)
}

func TestDeletingDocumentRemovesGitHubRepoConsents(t *testing.T) {
	e, _ := newGitHubTestEnv(t)
	connectGitHub(e, fixtureOwner)
	docID, _ := e.createDocument("disposable", "<html><body>disposable</body></html>", fixtureOwner)
	assertStatus(t, githubGrantConsent(e, docID, fixtureOwner, "acme", "docs"), http.StatusCreated)

	del := do(t, e.h, http.MethodDelete, "/v1/documents/"+docID, "", map[string]string{
		"X-Test-Principal": principalHeader(fixtureOwner),
	})
	assertStatus(t, del, http.StatusNoContent)

	consents, err := e.store.ListGitHubRepoConsents(t.Context(), fixtureOwner.ID, docID)
	if err != nil {
		t.Fatalf("list consents after deleting the document: %v", err)
	}
	if len(consents) != 0 {
		t.Fatalf("consents survived the document: %v", consents)
	}
}

// jsonErrorCode reads error.code from an error response body.
func jsonErrorCode(t *testing.T, body []byte) string {
	t.Helper()
	var payload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("response is not an error envelope: %v; body=%s", err, body)
	}
	return payload.Error.Code
}
