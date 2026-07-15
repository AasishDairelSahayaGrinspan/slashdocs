package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/razorpay/aidocs/api/internal/auth"
	"github.com/razorpay/aidocs/api/internal/connections"
	"github.com/razorpay/aidocs/api/internal/knowledge"
	"github.com/razorpay/aidocs/api/internal/repo"
)

func testConnectionCipher(t *testing.T) *connections.Cipher {
	t.Helper()
	cipher, err := connections.NewCipher([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	return cipher
}

func putTestConnection(t *testing.T, store connections.Store, cipher *connections.Cipher, account connections.Account, token string) connections.Account {
	t.Helper()
	sealed, err := cipher.Seal(account, connections.Credentials{AccessToken: token, TokenType: "Bearer"})
	if err != nil {
		t.Fatal(err)
	}
	account.CredentialCiphertext = sealed
	stored, err := store.Put(context.Background(), account)
	if err != nil {
		t.Fatal(err)
	}
	return stored
}

func TestSourceConnectionOwnershipProviderAndDisconnect(t *testing.T) {
	connectionStore := connections.NewMemory()
	cipher := testConnectionCipher(t)
	ownerConnection := putTestConnection(t, connectionStore, cipher, connections.Account{
		ID: "conn_owner_github", UserID: "knowledge_owner", Provider: connections.ProviderGitHub,
		ExternalAccountID: "101", DisplayName: "Owner GitHub", WorkspaceID: "private-workspace-id",
		WorkspaceName: "owner-private@example.com", Status: connections.StatusActive,
	}, "owner-provider-token")
	putTestConnection(t, connectionStore, cipher, connections.Account{
		ID: "conn_editor_github", UserID: "knowledge_editor", Provider: connections.ProviderGitHub,
		ExternalAccountID: "202", DisplayName: "Editor GitHub", Status: connections.StatusActive,
	}, "editor-provider-token")
	putTestConnection(t, connectionStore, cipher, connections.Account{
		ID: "conn_owner_slack", UserID: "knowledge_owner", Provider: connections.ProviderSlack,
		ExternalAccountID: "U1", WorkspaceID: "T1", DisplayName: "Owner Slack", Status: connections.StatusActive,
	}, "owner-slack-token")

	fixture := newKnowledgeFixtureWithOptions(t, WithConnectionStore(connectionStore), WithConnectionCipher(cipher))
	path := "/v1/repositories/" + fixture.repositoryID + "/sources"
	base := map[string]any{
		"provider": "github", "external_repository": "acme/payments", "branch": "main",
		"trigger_mode": "default_branch", "direction": "into_aidocs",
	}

	crossUser := cloneJSONMap(base)
	crossUser["connection_id"] = "conn_editor_github"
	response := knowledgeRequest(t, fixture, http.MethodPost, path, "owner", crossUser)
	if response.Code != http.StatusForbidden {
		t.Fatalf("cross-user source status=%d body=%s", response.Code, response.Body.String())
	}

	wrongProvider := cloneJSONMap(base)
	wrongProvider["connection_id"] = "conn_owner_slack"
	response = knowledgeRequest(t, fixture, http.MethodPost, path, "owner", wrongProvider)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("provider mismatch status=%d body=%s", response.Code, response.Body.String())
	}

	valid := cloneJSONMap(base)
	valid["connection_id"] = ownerConnection.ID
	valid["target_document_id"] = fixture.documentID
	response = knowledgeRequest(t, fixture, http.MethodPost, path, "owner", valid)
	if response.Code != http.StatusCreated || strings.Contains(response.Body.String(), "provider-token") || !strings.Contains(response.Body.String(), `"connection"`) {
		t.Fatalf("connected source status=%d body=%s", response.Code, response.Body.String())
	}
	var created struct {
		ID           string `json:"id"`
		ConnectionID string `json:"connection_id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ConnectionID != ownerConnection.ID {
		t.Fatalf("connection id=%q", created.ConnectionID)
	}
	viewerList := knowledgeRequest(t, fixture, http.MethodGet, path, "viewer", nil)
	if viewerList.Code != http.StatusOK || strings.Contains(viewerList.Body.String(), "private-workspace-id") || strings.Contains(viewerList.Body.String(), "owner-private@example.com") || strings.Contains(viewerList.Body.String(), `"external_account_id":"101"`) {
		t.Fatalf("viewer source response leaked account identity: status=%d body=%s", viewerList.Code, viewerList.Body.String())
	}

	pausedInput := cloneJSONMap(base)
	pausedInput["connection_id"] = ownerConnection.ID
	pausedInput["external_repository"] = "acme/paused-docs"
	pausedResponse := knowledgeRequest(t, fixture, http.MethodPost, path, "owner", pausedInput)
	if pausedResponse.Code != http.StatusCreated {
		t.Fatalf("create paused source status=%d body=%s", pausedResponse.Code, pausedResponse.Body.String())
	}
	var pausedSource struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(pausedResponse.Body.Bytes(), &pausedSource); err != nil {
		t.Fatal(err)
	}
	pause := knowledgeRequest(t, fixture, http.MethodPatch, path+"/"+pausedSource.ID, "owner", map[string]any{"status": "paused"})
	if pause.Code != http.StatusOK {
		t.Fatalf("pause source status=%d body=%s", pause.Code, pause.Body.String())
	}

	disconnect := knowledgeRequest(t, fixture, http.MethodDelete, "/v1/connections/"+ownerConnection.ID, "owner", nil)
	if disconnect.Code != http.StatusNoContent {
		t.Fatalf("disconnect status=%d body=%s", disconnect.Code, disconnect.Body.String())
	}
	stored, err := fixture.knowledge.GetSource(context.Background(), fixture.repositoryID, created.ID)
	if err != nil || stored.Status != "needs_reconnect" {
		t.Fatalf("source after disconnect=%#v err=%v", stored, err)
	}
	pausedStored, err := fixture.knowledge.GetSource(context.Background(), fixture.repositoryID, pausedSource.ID)
	if err != nil || pausedStored.Status != "paused" {
		t.Fatalf("paused source after disconnect=%#v err=%v", pausedStored, err)
	}
	handlers{deps: dependencies{knowledge: fixture.knowledge}}.restoreConnectionSources(context.Background(), ownerConnection.ID)
	pausedStored, err = fixture.knowledge.GetSource(context.Background(), fixture.repositoryID, pausedSource.ID)
	if err != nil || pausedStored.Status != "paused" {
		t.Fatalf("paused source after reconnect=%#v err=%v", pausedStored, err)
	}
	disconnected, err := connectionStore.Get(context.Background(), ownerConnection.ID)
	if err != nil || disconnected.Status != connections.StatusDisconnected || len(disconnected.CredentialCiphertext) != 0 {
		t.Fatalf("connection after disconnect=%#v err=%v", disconnected, err)
	}
}

func TestCanonicalSlackThreadURLUsesConnectedWorkspace(t *testing.T) {
	got, err := canonicalSlackSourceIdentity("https://aidocs.slack.com/archives/C123/p1712345678901234?thread_ts=1712345000.123456&cid=C123", "T_CONNECTED")
	if err != nil || got != "T_CONNECTED/C123/1712345000.123456" {
		t.Fatalf("canonical identity=%q err=%v", got, err)
	}
	got, err = canonicalSlackSourceIdentity("https://aidocs.slack.com/archives/C123/p1712345678901234", "T_CONNECTED")
	if err != nil || got != "T_CONNECTED/C123/1712345678.901234" {
		t.Fatalf("canonical root identity=%q err=%v", got, err)
	}
	if _, err := canonicalSlackSourceIdentity("https://example.com/archives/C123/p1712345678901234", "T_CONNECTED"); err == nil {
		t.Fatal("accepted non-Slack thread URL")
	}
}

func TestGitHubConnectionOAuthIsBoundOneTimeAndTokenSafe(t *testing.T) {
	var tokenRequest url.Values
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			tokenRequest = r.Form
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"super-secret-provider-token","token_type":"bearer","scope":"repo read:user"}`))
		case "/user":
			if got := r.Header.Get("Authorization"); got != "Bearer super-secret-provider-token" {
				t.Errorf("authorization=%q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":123,"login":"bilal","name":"Bilal"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(provider.Close)

	storage := repoMemoryWithUser(t, auth.Principal{Type: auth.PrincipalUser, ID: "oauth_user", Email: "oauth@example.com", Name: "OAuth User"})
	connectionStore := connections.NewMemory()
	cipher := testConnectionCipher(t)
	stateStore := auth.NewStateStore()
	authenticator := knowledgeTestAuth{principals: map[string]auth.Principal{"user": {Type: auth.PrincipalUser, ID: "oauth_user"}, "service": {Type: auth.PrincipalServiceAccount, ID: "sa_1"}}}
	app := New(Config{
		Environment: envTest, AppOrigin: "http://app.example", SessionSecret: "connection-test-session-secret-at-least-32-bytes",
		ConnectionProviders: ConnectionProviders{GitHub: ConnectionOAuthProvider{ClientID: "client-id", ClientSecret: "client-secret", AuthURL: provider.URL + "/authorize", TokenURL: provider.URL + "/token", APIBaseURL: provider.URL}},
	}, WithRepository(storage), WithAuthenticator(authenticator), WithConnectionStore(connectionStore), WithConnectionCipher(cipher), WithStateStore(stateStore), WithConnectionStateStore(stateStore), WithConnectionHTTPClient(provider.Client()))
	t.Cleanup(app.Close)

	serviceStart := connectionRequest(t, app.Handler(), http.MethodGet, "/v1/connections/github/start", "service", nil)
	if serviceStart.Code != http.StatusForbidden {
		t.Fatalf("service start status=%d", serviceStart.Code)
	}
	start := connectionRequest(t, app.Handler(), http.MethodGet, "/v1/connections/github/start?redirect=%2Fapp%2Fsettings%2Fconnections", "user", nil)
	if start.Code != http.StatusFound {
		t.Fatalf("start status=%d body=%s", start.Code, start.Body.String())
	}
	authorize, err := url.Parse(start.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	stateID := authorize.Query().Get("state")
	if stateID == "" || authorize.Query().Get("code_challenge") == "" || authorize.Query().Get("scope") != "repo" {
		t.Fatalf("authorize query=%v", authorize.Query())
	}
	cookies := start.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("state cookies=%#v", cookies)
	}
	callback := httptest.NewRequest(http.MethodGet, "/v1/connections/github/callback?state="+url.QueryEscape(stateID)+"&code=provider-code", nil)
	callback.AddCookie(cookies[0])
	callbackResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(callbackResponse, callback)
	if callbackResponse.Code != http.StatusFound || !strings.Contains(callbackResponse.Header().Get("Location"), "connected=github") {
		t.Fatalf("callback status=%d location=%q body=%s", callbackResponse.Code, callbackResponse.Header().Get("Location"), callbackResponse.Body.String())
	}
	if tokenRequest.Get("code_verifier") == "" || tokenRequest.Get("code") != "provider-code" {
		t.Fatalf("token form=%v", tokenRequest)
	}

	list := connectionRequest(t, app.Handler(), http.MethodGet, "/v1/connections", "user", nil)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "Bilal") || strings.Contains(list.Body.String(), "super-secret-provider-token") || strings.Contains(list.Body.String(), "oauth_user") {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}
	replay := httptest.NewRequest(http.MethodGet, "/v1/connections/github/callback?state="+url.QueryEscape(stateID)+"&code=provider-code", nil)
	replay.AddCookie(cookies[0])
	replayResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(replayResponse, replay)
	if replayResponse.Code != http.StatusBadRequest {
		t.Fatalf("replay status=%d body=%s", replayResponse.Code, replayResponse.Body.String())
	}
}

func TestConnectionAccessTokenResolverRefreshesGoogleCredential(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseForm(); err != nil || r.Form.Get("refresh_token") != "refresh-secret" {
			http.Error(w, "bad refresh", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"fresh-access","token_type":"Bearer","expires_in":3600}`))
	}))
	t.Cleanup(provider.Close)
	store := connections.NewMemory()
	cipher := testConnectionCipher(t)
	expired := time.Now().Add(-time.Minute)
	account := connections.Account{
		ID: "conn_drive_refresh", UserID: "user_drive", Provider: connections.ProviderGoogleDrive,
		ExternalAccountID: "drive-user", DisplayName: "Drive User", Status: connections.StatusActive, ExpiresAt: &expired,
	}
	sealed, err := cipher.Seal(account, connections.Credentials{AccessToken: "expired-access", RefreshToken: "refresh-secret", TokenType: "Bearer"})
	if err != nil {
		t.Fatal(err)
	}
	account.CredentialCiphertext = sealed
	if _, err := store.Put(context.Background(), account); err != nil {
		t.Fatal(err)
	}
	resolver := NewConnectionAccessTokenResolver(store, cipher, ConnectionProviders{GoogleDrive: ConnectionOAuthProvider{ClientID: "client", ClientSecret: "secret", TokenURL: provider.URL + "/token"}}, provider.Client())
	token, err := resolver(context.Background(), account.ID)
	if err != nil || token != "fresh-access" {
		t.Fatalf("token=%q err=%v", token, err)
	}
	updated, err := store.Get(context.Background(), account.ID)
	if err != nil || updated.ExpiresAt == nil || !updated.ExpiresAt.After(time.Now()) {
		t.Fatalf("updated=%#v err=%v", updated, err)
	}
	credentials, err := cipher.Open(updated, updated.CredentialCiphertext)
	if err != nil || credentials.AccessToken != "fresh-access" || credentials.RefreshToken != "refresh-secret" {
		t.Fatalf("credentials=%#v err=%v", credentials, err)
	}
}

func TestConnectionAccessTokenResolverRefreshesRotatingSlackUserCredential(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseForm(); err != nil || r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != "slack-refresh-secret" {
			http.Error(w, "bad refresh", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"authed_user":{"access_token":"fresh-slack-user","refresh_token":"next-slack-refresh","token_type":"user","expires_in":3600}}`))
	}))
	t.Cleanup(provider.Close)
	store := connections.NewMemory()
	cipher := testConnectionCipher(t)
	expired := time.Now().Add(-time.Minute)
	account := connections.Account{
		ID: "conn_slack_refresh", UserID: "user_slack", Provider: connections.ProviderSlack,
		ExternalAccountID: "U123", WorkspaceID: "T123", DisplayName: "Slack User",
		Status: connections.StatusActive, ExpiresAt: &expired,
	}
	sealed, err := cipher.Seal(account, connections.Credentials{AccessToken: "expired-slack", RefreshToken: "slack-refresh-secret", TokenType: "user"})
	if err != nil {
		t.Fatal(err)
	}
	account.CredentialCiphertext = sealed
	if _, err := store.Put(context.Background(), account); err != nil {
		t.Fatal(err)
	}
	resolver := NewConnectionAccessTokenResolver(store, cipher, ConnectionProviders{Slack: ConnectionOAuthProvider{ClientID: "client", ClientSecret: "secret", TokenURL: provider.URL + "/token"}}, provider.Client())
	token, err := resolver(context.Background(), account.ID)
	if err != nil || token != "fresh-slack-user" {
		t.Fatalf("token=%q err=%v", token, err)
	}
	updated, err := store.Get(context.Background(), account.ID)
	if err != nil || updated.ExpiresAt == nil || !updated.ExpiresAt.After(time.Now()) {
		t.Fatalf("updated=%#v err=%v", updated, err)
	}
	credentials, err := cipher.Open(updated, updated.CredentialCiphertext)
	if err != nil || credentials.AccessToken != "fresh-slack-user" || credentials.RefreshToken != "next-slack-refresh" {
		t.Fatalf("credentials=%#v err=%v", credentials, err)
	}
}

func TestDevRevPersonalTokenIsValidatedEncryptedAndNeverReturned(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/auth-tokens.info" || r.Header.Get("Authorization") != "Bearer devrev-personal-secret" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"organization":{"id":"devo-1","slug":"Razorpay"},"token":{"expiry":4070995445},"user":{"id":"devu-1","display_name":"Bilal","email":"bilal@example.com"}}`))
	}))
	t.Cleanup(provider.Close)
	principal := auth.Principal{Type: auth.PrincipalUser, ID: "devrev_user", Email: "devrev@example.com", Name: "DevRev User"}
	storage := repoMemoryWithUser(t, principal)
	store := connections.NewMemory()
	cipher := testConnectionCipher(t)
	app := New(Config{Environment: envTest, ConnectionProviders: ConnectionProviders{DevRevAPIBaseURL: provider.URL}},
		WithRepository(storage), WithAuthenticator(knowledgeTestAuth{principals: map[string]auth.Principal{"user": principal}}),
		WithConnectionStore(store), WithConnectionCipher(cipher), WithConnectionHTTPClient(provider.Client()))
	t.Cleanup(app.Close)
	response := connectionRequest(t, app.Handler(), http.MethodPost, "/v1/connections/devrev", "user", strings.NewReader(`{"personal_access_token":"devrev-personal-secret"}`))
	if response.Code != http.StatusCreated || !strings.Contains(response.Body.String(), "Razorpay") || strings.Contains(response.Body.String(), "devrev-personal-secret") {
		t.Fatalf("connect status=%d body=%s", response.Code, response.Body.String())
	}
	accounts, err := store.ListByUser(context.Background(), principal.ID)
	if err != nil || len(accounts) != 1 || len(accounts[0].CredentialCiphertext) == 0 || strings.Contains(string(accounts[0].CredentialCiphertext), "devrev-personal-secret") {
		t.Fatalf("accounts=%#v err=%v", accounts, err)
	}
	credentials, err := cipher.Open(accounts[0], accounts[0].CredentialCiphertext)
	if err != nil || credentials.AccessToken != "devrev-personal-secret" {
		t.Fatalf("credentials=%#v err=%v", credentials, err)
	}
}

func TestDevRevPersonalTokenRejectsIdentitylessSuccess(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(provider.Close)
	principal := auth.Principal{Type: auth.PrincipalUser, ID: "devrev_empty_user", Email: "empty@example.com", Name: "Empty"}
	store := connections.NewMemory()
	app := New(Config{Environment: envTest, ConnectionProviders: ConnectionProviders{DevRevAPIBaseURL: provider.URL}},
		WithRepository(repoMemoryWithUser(t, principal)), WithAuthenticator(knowledgeTestAuth{principals: map[string]auth.Principal{"user": principal}}),
		WithConnectionStore(store), WithConnectionCipher(testConnectionCipher(t)), WithConnectionHTTPClient(provider.Client()))
	t.Cleanup(app.Close)
	response := connectionRequest(t, app.Handler(), http.MethodPost, "/v1/connections/devrev", "user", strings.NewReader(`{"personal_access_token":"identityless-secret"}`))
	if response.Code != http.StatusUnauthorized || strings.Contains(response.Body.String(), "identityless-secret") {
		t.Fatalf("identityless DevRev status=%d body=%s", response.Code, response.Body.String())
	}
	accounts, err := store.ListByUser(context.Background(), principal.ID)
	if err != nil || len(accounts) != 0 {
		t.Fatalf("identityless account was stored: %#v err=%v", accounts, err)
	}
}

type fixtureProviderInspector struct{}

func (fixtureProviderInspector) Compare(context.Context, knowledge.Source, string, string) (knowledge.CompareResult, error) {
	return knowledge.CompareResult{}, nil
}

func (fixtureProviderInspector) Inspect(_ context.Context, _ knowledge.Source, previous string) (knowledge.InspectResult, error) {
	if previous == "revision-1" {
		return knowledge.InspectResult{Revision: "revision-1", Unchanged: true}, nil
	}
	return knowledge.InspectResult{
		Revision: "revision-1", ChangedFiles: []string{"slack/threads/C1-171.txt"},
		ContextItems:  []knowledge.ContextItem{{Kind: "slack_thread", ExternalID: "171", Title: "Incident thread", Body: "The retry owner is Payments Platform."}},
		ProvenanceURL: "https://example.slack.com/archives/C1/p171",
	}, nil
}

func TestConnectedSourceSyncNowInspectsProviderAndOpensReview(t *testing.T) {
	connectionStore := connections.NewMemory()
	cipher := testConnectionCipher(t)
	connection := putTestConnection(t, connectionStore, cipher, connections.Account{
		ID: "conn_owner_slack_sync", UserID: "knowledge_owner", Provider: connections.ProviderSlack,
		ExternalAccountID: "U1", WorkspaceID: "T1", DisplayName: "Owner", Status: connections.StatusActive,
	}, "slack-user-token")
	fixture := newKnowledgeFixtureWithOptions(t, WithConnectionStore(connectionStore), WithConnectionCipher(cipher), WithKnowledgeProviderClient(fixtureProviderInspector{}))
	create := knowledgeRequest(t, fixture, http.MethodPost, "/v1/repositories/"+fixture.repositoryID+"/sources", "owner", map[string]any{
		"provider": "slack", "connection_id": connection.ID, "external_repository": "T1/C1/171",
		"target_document_id": fixture.documentID, "trigger_mode": "event", "direction": "into_aidocs",
	})
	if create.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", create.Code, create.Body.String())
	}
	var source struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &source); err != nil {
		t.Fatal(err)
	}
	check := knowledgeRequest(t, fixture, http.MethodPost, "/v1/repositories/"+fixture.repositoryID+"/sources/"+source.ID+"/check", "owner", map[string]any{})
	if check.Code != http.StatusOK || !strings.Contains(check.Body.String(), `"outcome":"pull_request_opened"`) || !strings.Contains(check.Body.String(), `"after_sha":"revision-1"`) {
		t.Fatalf("sync status=%d body=%s", check.Code, check.Body.String())
	}
	pullRequests, err := fixture.repository.ListPullRequests(context.Background(), fixture.repositoryID)
	if err != nil || len(pullRequests) != 1 {
		t.Fatalf("pull requests=%#v err=%v", pullRequests, err)
	}
	generated, err := fixture.repository.GetVersionHTML(context.Background(), pullRequests[0].HeadVersionID)
	if err != nil || !strings.Contains(string(generated), "The retry owner is Payments Platform") || !strings.Contains(string(generated), "slack/threads/C1-171.txt") {
		t.Fatalf("generated document=%s err=%v", generated, err)
	}
	second := knowledgeRequest(t, fixture, http.MethodPost, "/v1/repositories/"+fixture.repositoryID+"/sources/"+source.ID+"/check", "owner", map[string]any{})
	if second.Code != http.StatusOK || !strings.Contains(second.Body.String(), `"outcome":"no_op"`) {
		t.Fatalf("second sync status=%d body=%s", second.Code, second.Body.String())
	}
}

func TestSourcePollingQueuesOncePerWindow(t *testing.T) {
	fixture := newKnowledgeFixture(t)
	now := knowledge.Now()
	if _, err := fixture.knowledge.PutSource(context.Background(), knowledge.Source{
		ID: "source_poll", RepositoryID: fixture.repositoryID, ConnectionID: "conn_poll",
		Provider: knowledge.ProviderGitHub, ExternalRepository: "acme/payments", Branch: "main",
		TriggerMode: "default_branch", Direction: "into_aidocs", Status: "active",
		CreatedBy: fixture.owner, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	deps := dependencies{knowledge: fixture.knowledge, sourcePollInterval: 15 * time.Minute}
	window := time.Unix(1_800_000_000, 0)
	queueSourcePolls(context.Background(), deps, window)
	queueSourcePolls(context.Background(), deps, window.Add(time.Minute))
	runs, err := fixture.knowledge.ListSyncRuns(context.Background(), fixture.repositoryID)
	if err != nil || len(runs) != 1 || runs[0].Status != "queued" || !strings.HasPrefix(runs[0].DeliveryID, "poll:") {
		t.Fatalf("poll runs=%#v err=%v", runs, err)
	}
	queueSourcePolls(context.Background(), deps, window.Add(15*time.Minute))
	runs, err = fixture.knowledge.ListSyncRuns(context.Background(), fixture.repositoryID)
	if err != nil || len(runs) != 2 {
		t.Fatalf("next-window runs=%#v err=%v", runs, err)
	}
}

func connectionRequest(t *testing.T, handler http.Handler, method, path, principal string, body *strings.Reader) *httptest.ResponseRecorder {
	t.Helper()
	var request *http.Request
	if body == nil {
		request = httptest.NewRequest(method, path, nil)
	} else {
		request = httptest.NewRequest(method, path, body)
		request.Header.Set("Content-Type", "application/json")
	}
	if principal != "" {
		request.Header.Set("X-Knowledge-Principal", principal)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func cloneJSONMap(input map[string]any) map[string]any {
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func repoMemoryWithUser(t *testing.T, principal auth.Principal) *repo.Memory {
	t.Helper()
	storage := repo.NewMemory()
	if _, err := storage.UpsertGoogleUser(context.Background(), principal.ID, principal.Email, principal.Name, "sub-"+principal.ID, ""); err != nil {
		t.Fatal(err)
	}
	return storage
}
