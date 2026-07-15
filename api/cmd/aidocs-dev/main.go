//go:build aidocsdev

// Command aidocs-dev runs a production-isolated, localhost-only aidocs server.
// It is intentionally excluded from normal builds and uses only in-memory
// state plus a fixed allow-list of development identities.
package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"html"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/razorpay/aidocs/api/internal/auth"
	"github.com/razorpay/aidocs/api/internal/connections"
	"github.com/razorpay/aidocs/api/internal/knowledge"
	"github.com/razorpay/aidocs/api/internal/repo"
	"github.com/razorpay/aidocs/api/internal/server"
)

const (
	defaultDevelopmentPort = "8080"
	developmentHomePath    = "/__dev"
	developmentSwitchPath  = "/__dev/login"
	developmentAppPath     = "/app/repositories"
)

func main() {
	addr, err := developmentAddress(os.Getenv("AIDOCS_DEV_PORT"))
	if err != nil {
		log.Fatal(err)
	}
	origin := "http://" + addr
	handler, err := newDevelopmentApplication(origin)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("aidocs development server listening on %s", origin)    //nolint:gosec // Origin is a loopback address with a validated numeric port.
	log.Printf("switch identity at %s%s", origin, developmentHomePath) //nolint:gosec // Origin is a loopback address with a validated numeric port.
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	serveErr := httpServer.ListenAndServe()
	handler.Close()
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		log.Fatal(serveErr)
	}
}

func developmentAddress(port string) (string, error) {
	port = strings.TrimSpace(port)
	if port == "" {
		port = defaultDevelopmentPort
	}
	value, err := strconv.Atoi(port)
	if err != nil || value < 1 || value > 65535 {
		return "", fmt.Errorf("AIDOCS_DEV_PORT must be a number between 1 and 65535 (got %q)", port)
	}
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(value)), nil
}

func developmentIdentities() map[string]auth.Principal {
	return map[string]auth.Principal{
		"owner": {
			Type:  auth.PrincipalUser,
			ID:    "dev_owner",
			Email: "owner@aidocs.local",
			Name:  "Dev Owner",
		},
		"reviewer": {
			Type:  auth.PrincipalUser,
			ID:    "dev_reviewer",
			Email: "reviewer@aidocs.local",
			Name:  "Dev Reviewer",
		},
	}
}

func newDevelopmentApplication(origin string) (*developmentApplication, error) {
	identities := developmentIdentities()
	authenticator, err := auth.NewDevAuthenticator(identities, "owner")
	if err != nil {
		return nil, err
	}
	sessionSecret, err := auth.RandomURLToken()
	if err != nil {
		return nil, fmt.Errorf("generate development session secret: %w", err)
	}
	store := repo.NewMemory()
	knowledgeStore := knowledge.NewMemory()
	connectionStore := connections.NewMemory()
	connectionKey := sha256.Sum256([]byte("aidocs-dev-personal-connections-v1"))
	connectionCipher, err := connections.NewCipher(connectionKey[:])
	if err != nil {
		return nil, fmt.Errorf("create development connection cipher: %w", err)
	}
	ctx := context.Background()
	if err := seedDevelopmentRepository(ctx, store, identities); err != nil {
		return nil, err
	}
	developmentConnections, err := seedDevelopmentConnections(ctx, connectionStore, identities)
	if err != nil {
		return nil, err
	}
	if err := seedDevelopmentKnowledge(ctx, store, knowledgeStore, identities, developmentConnections[identities["owner"].ID+"/"+connections.ProviderGitHub]); err != nil {
		return nil, err
	}
	originURL, err := url.Parse(origin)
	if err != nil || originURL.Scheme != "http" || originURL.Host == "" {
		return nil, fmt.Errorf("invalid development origin %q", origin)
	}

	app := server.New(server.Config{ //nolint:gosec // Build-tagged localhost server intentionally uses fixed webhook fixtures.
		Environment:         server.EnvDevelopment,
		AppOrigin:           origin,
		RenderOrigin:        origin,
		SessionSecret:       sessionSecret,
		Deployment:          server.DeploymentOrg,
		OrgName:             "Local Development",
		GitHubWebhookSecret: "aidocs-dev-github-secret",
		SlackSigningSecret:  "aidocs-dev-slack-signing-secret",
		DevRevWebhookSecret: "aidocs-dev-devrev-secret",
	}, server.WithRepository(store), server.WithKnowledgeStore(knowledgeStore), server.WithConnectionStore(connectionStore), server.WithConnectionCipher(connectionCipher), server.WithAuthenticator(authenticator))
	return &developmentApplication{
		Handler: &developmentHandler{app: app.Handler(), authenticator: authenticator, origin: originURL},
		server:  app,
	}, nil
}

func seedDevelopmentConnections(ctx context.Context, store connections.Store, identities map[string]auth.Principal) (map[string]string, error) {
	ids := map[string]string{}
	for _, principal := range identities {
		for _, provider := range []string{connections.ProviderGitHub, connections.ProviderSlack, connections.ProviderGoogleDrive, connections.ProviderDevRev} {
			id := "conn_" + strings.TrimPrefix(principal.ID, "dev_") + "_" + provider
			account := connections.Account{
				ID: id, UserID: principal.ID, Provider: provider,
				ExternalAccountID: principal.ID + "_" + provider,
				DisplayName:       principal.Name,
				Scopes:            []string{"local:demo"}, Status: connections.StatusDemo,
			}
			switch provider {
			case connections.ProviderSlack:
				account.WorkspaceID, account.WorkspaceName = "T_AIDOCS_DEV", "Local Development"
			case connections.ProviderGoogleDrive:
				account.WorkspaceName = principal.Email
			case connections.ProviderDevRev:
				account.WorkspaceID, account.WorkspaceName = "dev-org", "Local Development"
			}
			if _, err := store.Put(ctx, account); err != nil {
				return nil, fmt.Errorf("seed %s connection for %s: %w", provider, principal.ID, err)
			}
			ids[principal.ID+"/"+provider] = id
		}
	}
	return ids, nil
}

func seedDevelopmentRepository(ctx context.Context, store *repo.Memory, identities map[string]auth.Principal) error {
	owner := identities["owner"]
	reviewer := identities["reviewer"]
	for name, principal := range map[string]auth.Principal{"owner": owner, "reviewer": reviewer} {
		if _, err := store.UpsertGoogleUser(ctx, principal.ID, principal.Email, principal.Name, "aidocsdev-"+name, ""); err != nil {
			return fmt.Errorf("seed %s identity: %w", name, err)
		}
	}

	document, _, err := store.CreateDocument(ctx, owner, "Welcome to aidocs", []byte(`<!doctype html><html><body><h1>Welcome to aidocs</h1><p>Open a pull request as the reviewer, then approve it as the owner.</p></body></html>`))
	if err != nil {
		return fmt.Errorf("seed document: %w", err)
	}
	folder, err := store.CreateFolder(ctx, owner, "Docs playground", "")
	if err != nil {
		return fmt.Errorf("seed folder: %w", err)
	}
	if _, err := store.MoveDocumentToFolder(ctx, owner, folder.ID, document.ID); err != nil {
		return fmt.Errorf("seed document placement: %w", err)
	}
	if _, err := store.CreateGrant(ctx, "folder", folder.ID, reviewer, repo.RoleEditor, owner); err != nil {
		return fmt.Errorf("seed reviewer grant: %w", err)
	}
	if _, err := store.CreateRepository(ctx, owner, repo.CreateRepositoryParams{
		Name:              "Docs playground",
		Slug:              "docs-playground",
		Description:       "An ephemeral repository for local pull-request development.",
		Scope:             repo.RepositoryScopeOrganization,
		NamespaceID:       "organization",
		NamespaceName:     "Local Development",
		RootFolderID:      folder.ID,
		RequiredApprovals: 1,
		Protected:         true,
		GrantOrgViewer:    true,
	}); err != nil {
		return fmt.Errorf("seed repository: %w", err)
	}
	return nil
}

func seedDevelopmentKnowledge(ctx context.Context, store *repo.Memory, knowledgeStore *knowledge.Memory, identities map[string]auth.Principal, githubConnectionID string) error {
	owner := identities["owner"]
	reviewer := identities["reviewer"]
	repositories, err := store.ListRepositories(ctx, owner)
	if err != nil {
		return fmt.Errorf("find seeded repository: %w", err)
	}
	if len(repositories) == 0 {
		return errors.New("find seeded repository: no repositories returned")
	}
	repository := repositories[0]
	documents, err := store.ListRepositoryDocuments(ctx, owner, repository.ID)
	if err != nil {
		return fmt.Errorf("find seeded repository document: %w", err)
	}
	if len(documents) == 0 {
		return errors.New("find seeded repository document: no documents returned")
	}
	document := documents[0]
	now := knowledge.Now()

	settings := knowledge.DefaultSettings(repository.ID)
	settings.RequiredReviewers = []string{owner.ID}
	settings.RequiredTeams = []string{}
	settings.PathRules = []knowledge.PathRule{{
		Pattern:       "api/**",
		RequiredTeams: []string{"dev_docs_reviewers"},
	}}
	settings.Portal = knowledge.PortalSettings{
		Enabled:    true,
		Slug:       "docs-playground",
		Visibility: "public",
	}
	if _, err := knowledgeStore.PutSettings(ctx, settings); err != nil {
		return fmt.Errorf("seed connected knowledge settings: %w", err)
	}
	if _, err := knowledgeStore.PutTeam(ctx, knowledge.Team{
		ID:           "dev_docs_reviewers",
		RepositoryID: repository.ID,
		Name:         "Docs reviewers",
		Description:  "The local reviewer team used to exercise review routing.",
		MemberIDs:    []string{reviewer.ID},
		CreatedAt:    now,
		UpdatedAt:    now,
	}); err != nil {
		return fmt.Errorf("seed connected knowledge team: %w", err)
	}
	if _, err := knowledgeStore.PutSource(ctx, knowledge.Source{
		ID:                 "source_dev_github",
		RepositoryID:       repository.ID,
		ConnectionID:       githubConnectionID,
		Provider:           knowledge.ProviderGitHub,
		ExternalRepository: "razorpay/aidocs-example",
		Branch:             "main",
		IncludePaths:       []string{"README.md", "docs/**", "api/**"},
		ExcludePaths:       []string{"vendor/**", "**/node_modules/**"},
		TargetDocumentID:   document.ID,
		TriggerMode:        "default_branch",
		Direction:          "into_aidocs",
		Status:             "active",
		Freshness: knowledge.Freshness{
			State:           "not_synced",
			CheckedAt:       now,
			StaleAfterHours: 168,
		},
		CreatedBy: owner,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		return fmt.Errorf("seed connected knowledge source: %w", err)
	}
	if err := knowledgeStore.AddAudit(ctx, knowledge.AuditEvent{
		ID:           "audit_dev_source_connected",
		RepositoryID: repository.ID,
		Action:       "source.connected",
		Actor:        owner,
		SubjectType:  "source",
		SubjectID:    "source_dev_github",
		Details: map[string]any{
			"external_repository": "razorpay/aidocs-example",
			"local_demo":          true,
		},
		CreatedAt: now,
	}); err != nil {
		return fmt.Errorf("seed connected knowledge activity: %w", err)
	}
	return nil
}

type developmentHandler struct {
	app           http.Handler
	authenticator *auth.DevAuthenticator
	origin        *url.URL
}

type developmentApplication struct {
	http.Handler
	server *server.Server
}

func (a *developmentApplication) Close() { a.server.Close() }

func (h *developmentHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.allowRequest(w, r) {
		return
	}
	switch r.URL.Path {
	case developmentHomePath:
		h.serveIdentityPage(w, r)
	case developmentSwitchPath:
		h.switchIdentity(w, r)
	case "/v1/auth/google/start", "/v1/auth/google/callback":
		http.Redirect(w, r, developmentHomePath, http.StatusSeeOther)
	default:
		h.app.ServeHTTP(w, r)
	}
}

func (h *developmentHandler) allowRequest(w http.ResponseWriter, r *http.Request) bool {
	if !strings.EqualFold(r.Host, h.origin.Host) {
		http.Error(w, "development server host mismatch", http.StatusForbidden)
		return false
	}
	checkBrowserOrigin := r.URL.Path == developmentSwitchPath || (strings.HasPrefix(r.URL.Path, "/v1/") && isUnsafeMethod(r.Method))
	if !checkBrowserOrigin {
		return true
	}
	for _, header := range []string{"Origin", "Referer"} {
		value := strings.TrimSpace(r.Header.Get(header))
		if value != "" && !h.sameOrigin(value) {
			http.Error(w, "cross-origin development request rejected", http.StatusForbidden)
			return false
		}
	}
	return true
}

func (h *developmentHandler) sameOrigin(value string) bool {
	candidate, err := url.Parse(value)
	return err == nil && candidate.User == nil && strings.EqualFold(candidate.Scheme, h.origin.Scheme) && strings.EqualFold(candidate.Host, h.origin.Host)
}

func isUnsafeMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func (h *developmentHandler) serveIdentityPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	current, _ := h.authenticator.SelectedIdentity(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprintf(w, `<!doctype html><html><body><script>try{localStorage.removeItem("aidocs.frontend_signed_out")}catch(e){}</script><h1>aidocs local identities</h1><p>Current identity: <strong>%s</strong></p><ul>`, html.EscapeString(current))
	for _, name := range h.authenticator.IdentityNames() {
		target := developmentSwitchPath + "?as=" + url.QueryEscape(name) + "&redirect=" + url.QueryEscape(developmentAppPath)
		_, _ = fmt.Fprintf(w, `<li><a href="%s">Use %s</a></li>`, html.EscapeString(target), html.EscapeString(name))
	}
	_, _ = fmt.Fprintf(w, `</ul><p><a href="%s">Continue to aidocs</a></p></body></html>`, developmentAppPath)
}

func (h *developmentHandler) switchIdentity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	requested := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("as")))
	valid := false
	for _, name := range h.authenticator.IdentityNames() {
		if requested == name {
			valid = true
			break
		}
	}
	if !valid {
		http.Error(w, "unknown development identity", http.StatusBadRequest)
		return
	}
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // Local HTTP cannot use Secure; this remains HttpOnly and SameSiteStrict.
		Name:     auth.DevIdentityCookie,
		Value:    requested,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   24 * 60 * 60,
	})
	http.Redirect(w, r, safeDevelopmentRedirect(r.URL.Query().Get("redirect")), http.StatusSeeOther) //nolint:gosec // The helper rejects absolute, host-bearing, scheme-relative, and backslash URLs.
}

func safeDevelopmentRedirect(raw string) string {
	if raw == "" || strings.Contains(raw, `\`) {
		return developmentAppPath
	}
	target, err := url.Parse(raw)
	if err != nil || target.IsAbs() || target.Host != "" || strings.Contains(target.Path, `\`) || !strings.HasPrefix(target.Path, "/") || strings.HasPrefix(target.Path, "//") {
		return developmentAppPath
	}
	return target.RequestURI()
}
