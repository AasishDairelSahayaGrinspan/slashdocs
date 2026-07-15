//go:build aidocsdev

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/razorpay/aidocs/api/internal/auth"
	"github.com/razorpay/aidocs/api/internal/repo"
)

func TestDevelopmentAddressIsAlwaysLoopback(t *testing.T) {
	tests := []struct {
		port    string
		want    string
		wantErr bool
	}{
		{want: "127.0.0.1:8080"},
		{port: " 9091 ", want: "127.0.0.1:9091"},
		{port: "0", wantErr: true},
		{port: "65536", wantErr: true},
		{port: "0.0.0.0:8080", wantErr: true},
	}
	for _, tt := range tests {
		got, err := developmentAddress(tt.port)
		if (err != nil) != tt.wantErr {
			t.Fatalf("developmentAddress(%q) error = %v, wantErr %v", tt.port, err, tt.wantErr)
		}
		if got != tt.want {
			t.Fatalf("developmentAddress(%q) = %q, want %q", tt.port, got, tt.want)
		}
	}
}

func TestDevelopmentApplicationSwitchesIdentityWithoutOAuth(t *testing.T) {
	handler, err := newDevelopmentApplication("http://127.0.0.1:8080")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(handler.Close)

	owner := requestDevelopmentApp(t, handler, "/v1/me", nil)
	if owner.Code != http.StatusOK {
		t.Fatalf("owner /v1/me status = %d, body=%s", owner.Code, owner.Body.String())
	}
	if got := responsePrincipalID(t, owner); got != "dev_owner" {
		t.Fatalf("default principal = %q, want dev_owner", got)
	}

	switchResponse := requestDevelopmentApp(t, handler, developmentSwitchPath+"?as=reviewer&redirect=%2Fapp%2Frepositories", nil)
	if switchResponse.Code != http.StatusSeeOther {
		t.Fatalf("switch status = %d, body=%s", switchResponse.Code, switchResponse.Body.String())
	}
	if location := switchResponse.Header().Get("Location"); location != "/app/repositories" {
		t.Fatalf("switch Location = %q, want /app/repositories", location)
	}
	cookies := switchResponse.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != auth.DevIdentityCookie || cookies[0].Value != "reviewer" || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("switch cookies = %#v", cookies)
	}

	reviewer := requestDevelopmentApp(t, handler, "/v1/me", cookies[0])
	if reviewer.Code != http.StatusOK {
		t.Fatalf("reviewer /v1/me status = %d, body=%s", reviewer.Code, reviewer.Body.String())
	}
	if got := responsePrincipalID(t, reviewer); got != "dev_reviewer" {
		t.Fatalf("switched principal = %q, want dev_reviewer", got)
	}
	repositories := requestDevelopmentApp(t, handler, "/v1/repositories", cookies[0])
	if repositories.Code != http.StatusOK || !strings.Contains(repositories.Body.String(), "docs-playground") {
		t.Fatalf("reviewer repositories status=%d body=%s", repositories.Code, repositories.Body.String())
	}

	oauth := requestDevelopmentApp(t, handler, "/v1/auth/google/start", nil)
	if oauth.Code != http.StatusSeeOther || oauth.Header().Get("Location") != developmentHomePath {
		t.Fatalf("OAuth replacement status=%d location=%q", oauth.Code, oauth.Header().Get("Location"))
	}
}

func TestDevelopmentApplicationCompletesReviewLifecycle(t *testing.T) {
	handler, err := newDevelopmentApplication("http://127.0.0.1:8080")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(handler.Close)
	reviewerCookie := switchDevelopmentIdentity(t, handler, "reviewer")

	repositoriesResponse := requestDevelopmentApp(t, handler, "/v1/repositories", reviewerCookie)
	var repositories struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	decodeDevelopmentJSON(t, repositoriesResponse, &repositories)
	if len(repositories.Items) != 1 {
		t.Fatalf("seeded repositories = %#v", repositories.Items)
	}
	repositoryID := repositories.Items[0].ID

	documentsResponse := requestDevelopmentApp(t, handler, "/v1/repositories/"+repositoryID+"/documents", reviewerCookie)
	var documents struct {
		Items []struct {
			ID               string `json:"id"`
			CurrentVersionID string `json:"current_version_id"`
		} `json:"items"`
	}
	decodeDevelopmentJSON(t, documentsResponse, &documents)
	if len(documents.Items) != 1 {
		t.Fatalf("seeded documents = %#v", documents.Items)
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	fields := map[string]string{
		"document_id":     documents.Items[0].ID,
		"base_version_id": documents.Items[0].CurrentVersionID,
		"title":           "Reviewer proposal",
		"change_summary":  "Exercise local approval",
	}
	for name, value := range fields {
		if err := writer.WriteField(name, value); err != nil {
			t.Fatal(err)
		}
	}
	file, err := writer.CreateFormFile("file", "proposal.html")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("<html><body>approved locally</body></html>")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	create := performDevelopmentRequest(t, handler, http.MethodPost, "/v1/repositories/"+repositoryID+"/pull-requests", body.Bytes(), reviewerCookie, map[string]string{"Content-Type": writer.FormDataContentType()})
	if create.Code != http.StatusCreated {
		t.Fatalf("create PR status=%d body=%s", create.Code, create.Body.String())
	}
	var pullRequest struct {
		Number        int64  `json:"number"`
		HeadVersionID string `json:"head_version_id"`
		Status        string `json:"status"`
	}
	decodeDevelopmentJSON(t, create, &pullRequest)

	ownerCookie := switchDevelopmentIdentity(t, handler, "owner")
	pullRequestPath := fmt.Sprintf("/v1/repositories/%s/pull-requests/%d", repositoryID, pullRequest.Number)
	review := performDevelopmentRequest(t, handler, http.MethodPost, pullRequestPath+"/reviews", []byte(`{"state":"approved","body":"Local approval"}`), ownerCookie, map[string]string{"Content-Type": "application/json"})
	if review.Code != http.StatusOK {
		t.Fatalf("approve PR status=%d body=%s", review.Code, review.Body.String())
	}
	merge := performDevelopmentRequest(t, handler, http.MethodPost, pullRequestPath+"/merge", nil, ownerCookie, nil)
	if merge.Code != http.StatusOK {
		t.Fatalf("merge PR status=%d body=%s", merge.Code, merge.Body.String())
	}
	decodeDevelopmentJSON(t, merge, &pullRequest)
	if pullRequest.Status != repo.PullRequestStatusMerged {
		t.Fatalf("merged PR status = %q", pullRequest.Status)
	}
}

func TestDevelopmentApplicationSeedsConnectedSourceWorkflow(t *testing.T) {
	handler, err := newDevelopmentApplication("http://127.0.0.1:8080")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(handler.Close)
	ownerCookie := switchDevelopmentIdentity(t, handler, "owner")

	repositoriesResponse := requestDevelopmentApp(t, handler, "/v1/repositories", ownerCookie)
	var repositories struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	decodeDevelopmentJSON(t, repositoriesResponse, &repositories)
	if len(repositories.Items) != 1 {
		t.Fatalf("seeded repositories = %#v", repositories.Items)
	}
	repositoryID := repositories.Items[0].ID

	sourcesResponse := requestDevelopmentApp(t, handler, "/v1/repositories/"+repositoryID+"/sources", ownerCookie)
	var sources struct {
		Items []struct {
			ID               string `json:"id"`
			Provider         string `json:"provider"`
			TargetDocumentID string `json:"target_document_id"`
		} `json:"items"`
	}
	decodeDevelopmentJSON(t, sourcesResponse, &sources)
	if len(sources.Items) != 1 || sources.Items[0].ID != "source_dev_github" || sources.Items[0].Provider != "github" || sources.Items[0].TargetDocumentID == "" {
		t.Fatalf("seeded sources = %#v", sources.Items)
	}

	portal := requestDevelopmentApp(t, handler, "/v1/public/portals/docs-playground", nil)
	if portal.Code != http.StatusOK || !strings.Contains(portal.Body.String(), "Welcome to aidocs") {
		t.Fatalf("seeded portal status=%d body=%s", portal.Code, portal.Body.String())
	}

	sourcePath := "/v1/repositories/" + repositoryID + "/sources/source_dev_github"
	simulate := performDevelopmentRequest(t, handler, http.MethodPost, sourcePath+"/simulate", []byte(`{"after_sha":"local-preview","changed_files":["api/refunds/handler.go"]}`), ownerCookie, map[string]string{"Content-Type": "application/json"})
	if simulate.Code != http.StatusOK || !strings.Contains(simulate.Body.String(), `"outcome":"preview"`) {
		t.Fatalf("simulate source status=%d body=%s", simulate.Code, simulate.Body.String())
	}

	pullRequestsPath := "/v1/repositories/" + repositoryID + "/pull-requests"
	pullRequests := requestDevelopmentApp(t, handler, pullRequestsPath, ownerCookie)
	if pullRequests.Code != http.StatusOK || strings.Contains(pullRequests.Body.String(), `"number":`) {
		t.Fatalf("simulate mutated pull requests: status=%d body=%s", pullRequests.Code, pullRequests.Body.String())
	}

	check := performDevelopmentRequest(t, handler, http.MethodPost, sourcePath+"/check", []byte(`{"after_sha":"local-check-1","changed_files":["api/refunds/handler.go"]}`), ownerCookie, map[string]string{"Content-Type": "application/json"})
	if check.Code != http.StatusOK || !strings.Contains(check.Body.String(), `"outcome":"pull_request_opened"`) {
		t.Fatalf("check source status=%d body=%s", check.Code, check.Body.String())
	}
	var firstRun struct {
		PullRequestNumber int64 `json:"pull_request_number"`
	}
	decodeDevelopmentJSON(t, check, &firstRun)
	if firstRun.PullRequestNumber == 0 {
		t.Fatalf("check source did not create a pull request: %s", check.Body.String())
	}

	checkAgain := performDevelopmentRequest(t, handler, http.MethodPost, sourcePath+"/check", []byte(`{"after_sha":"local-check-2","changed_files":["docs/refunds.md"]}`), ownerCookie, map[string]string{"Content-Type": "application/json"})
	if checkAgain.Code != http.StatusOK || !strings.Contains(checkAgain.Body.String(), `"outcome":"pull_request_updated"`) {
		t.Fatalf("second source check status=%d body=%s", checkAgain.Code, checkAgain.Body.String())
	}
	var secondRun struct {
		PullRequestNumber int64 `json:"pull_request_number"`
	}
	decodeDevelopmentJSON(t, checkAgain, &secondRun)
	if secondRun.PullRequestNumber != firstRun.PullRequestNumber {
		t.Fatalf("source checks opened competing pull requests: first=%d second=%d", firstRun.PullRequestNumber, secondRun.PullRequestNumber)
	}

	pullRequest := requestDevelopmentApp(t, handler, fmt.Sprintf("%s/%d", pullRequestsPath, firstRun.PullRequestNumber), ownerCookie)
	if pullRequest.Code != http.StatusOK || !strings.Contains(pullRequest.Body.String(), `"id":"sa_connected_knowledge"`) || !strings.Contains(pullRequest.Body.String(), `"patchset":2`) {
		t.Fatalf("source pull request status=%d body=%s", pullRequest.Code, pullRequest.Body.String())
	}
}

func TestDevelopmentApplicationRejectsUnsafeIdentityRedirect(t *testing.T) {
	handler, err := newDevelopmentApplication("http://127.0.0.1:8080")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(handler.Close)
	response := requestDevelopmentApp(t, handler, developmentSwitchPath+"?as=owner&redirect=https%3A%2F%2Fevil.example", nil)
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != developmentAppPath {
		t.Fatalf("unsafe redirect status=%d location=%q", response.Code, response.Header().Get("Location"))
	}
}

func TestDevelopmentApplicationEnforcesHostAndBrowserOrigin(t *testing.T) {
	handler, err := newDevelopmentApplication("http://127.0.0.1:8080")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(handler.Close)

	wrongHost := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/v1/me", nil)
	wrongHost.Host = "evil.example"
	wrongHostResponse := httptest.NewRecorder()
	handler.ServeHTTP(wrongHostResponse, wrongHost)
	if wrongHostResponse.Code != http.StatusForbidden {
		t.Fatalf("mismatched Host status = %d", wrongHostResponse.Code)
	}

	crossOriginSwitch := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080"+developmentSwitchPath+"?as=reviewer", nil)
	crossOriginSwitch.Header.Set("Referer", "https://evil.example/")
	crossOriginSwitchResponse := httptest.NewRecorder()
	handler.ServeHTTP(crossOriginSwitchResponse, crossOriginSwitch)
	if crossOriginSwitchResponse.Code != http.StatusForbidden {
		t.Fatalf("cross-origin switch status = %d", crossOriginSwitchResponse.Code)
	}

	crossOriginAPI := performDevelopmentRequest(t, handler, http.MethodPost, "/v1/repositories", nil, nil, map[string]string{"Origin": "https://evil.example"})
	if crossOriginAPI.Code != http.StatusForbidden {
		t.Fatalf("cross-origin API status = %d", crossOriginAPI.Code)
	}
	withoutBrowserHeaders := performDevelopmentRequest(t, handler, http.MethodPost, "/v1/repositories", nil, nil, nil)
	if withoutBrowserHeaders.Code == http.StatusForbidden {
		t.Fatalf("headerless curl-style API request was rejected: %s", withoutBrowserHeaders.Body.String())
	}
}

func TestDevelopmentIdentityPageClearsSignedOutState(t *testing.T) {
	handler, err := newDevelopmentApplication("http://127.0.0.1:8080")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(handler.Close)
	response := requestDevelopmentApp(t, handler, developmentHomePath, nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `localStorage.removeItem("aidocs.frontend_signed_out")`) || !strings.Contains(response.Body.String(), developmentAppPath) {
		t.Fatalf("identity page status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestDevelopmentMemoryPreservesPullRequestHTML(t *testing.T) {
	ctx := context.Background()
	store := repo.NewMemory()
	identities := developmentIdentities()
	if err := seedDevelopmentRepository(ctx, store, identities); err != nil {
		t.Fatal(err)
	}
	repositories, err := store.ListRepositories(ctx, identities["reviewer"])
	if err != nil || len(repositories) != 1 {
		t.Fatalf("ListRepositories() = %#v, %v", repositories, err)
	}
	if repositories[0].Scope != repo.RepositoryScopeOrganization || repositories[0].NamespaceID != "organization" || repositories[0].NamespaceName != "Local Development" {
		t.Fatalf("development repository scope = %+v, want Local Development organization", repositories[0])
	}
	documents, err := store.ListRepositoryDocuments(ctx, identities["reviewer"], repositories[0].ID)
	if err != nil || len(documents) != 1 {
		t.Fatalf("ListRepositoryDocuments() = %#v, %v", documents, err)
	}
	headHTML := []byte("<html><body>review this exact proposal</body></html>")
	pullRequest, err := store.CreatePullRequest(ctx, repositories[0].ID, documents[0].ID, documents[0].CurrentVersionID, "Review the proposal", "", "", headHTML, identities["reviewer"])
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.GetVersionHTML(ctx, pullRequest.HeadVersionID)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(headHTML) {
		t.Fatalf("GetVersionHTML() = %q, want %q", got, headHTML)
	}
	got[0] = 'X'
	again, err := store.GetVersionHTML(ctx, pullRequest.HeadVersionID)
	if err != nil || string(again) != string(headHTML) {
		t.Fatalf("GetVersionHTML() returned mutable storage: %q, %v", again, err)
	}
}

func requestDevelopmentApp(t *testing.T, handler http.Handler, path string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	return performDevelopmentRequest(t, handler, http.MethodGet, path, nil, cookie, nil)
}

func performDevelopmentRequest(t *testing.T, handler http.Handler, method, path string, body []byte, cookie *http.Cookie, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, "http://127.0.0.1:8080"+path, bytes.NewReader(body))
	if cookie != nil {
		request.AddCookie(cookie)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func switchDevelopmentIdentity(t *testing.T, handler http.Handler, identity string) *http.Cookie {
	t.Helper()
	response := requestDevelopmentApp(t, handler, developmentSwitchPath+"?as="+url.QueryEscape(identity), nil)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("switch to %s status=%d body=%s", identity, response.Code, response.Body.String())
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("switch to %s cookies=%#v", identity, cookies)
	}
	return cookies[0]
}

func decodeDevelopmentJSON(t *testing.T, response *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(response.Body.Bytes(), target); err != nil {
		t.Fatalf("decode response: %v; status=%d body=%s", err, response.Code, response.Body.String())
	}
}

func responsePrincipalID(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Principal auth.Principal `json:"principal"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode /v1/me: %v; body=%s", err, response.Body.String())
	}
	return body.Principal.ID
}
