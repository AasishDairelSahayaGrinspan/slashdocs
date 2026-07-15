package server

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/razorpay/aidocs/api/internal/auth"
	"github.com/razorpay/aidocs/api/internal/connections"
	"github.com/razorpay/aidocs/api/internal/knowledge"
	"github.com/razorpay/aidocs/api/internal/kvevents"
	"github.com/razorpay/aidocs/api/internal/repo"
)

// ConnectionOAuthProvider configures one personal-account OAuth provider.
// Endpoint fields are exported so tests and self-hosted installations can use
// compatible provider gateways without changing the aidocs binary.
type ConnectionOAuthProvider struct {
	ClientID     string
	ClientSecret string
	AuthURL      string
	TokenURL     string
	APIBaseURL   string
}

// ConnectionProviders holds the optional external-account integrations. An
// omitted provider is shown as unavailable rather than falling back to a
// deployment-wide credential.
type ConnectionProviders struct {
	GitHub             ConnectionOAuthProvider
	Slack              ConnectionOAuthProvider
	GoogleDrive        ConnectionOAuthProvider
	DevRevAPIBaseURL   string
	GooglePickerAPIKey string
	GooglePickerAppID  string
}

// Config holds static configuration for the server.
type Config struct {
	Environment         string
	AppOrigin           string
	RenderOrigin        string
	OIDC                auth.OIDC
	OIDCProviderName    string
	SessionSecret       string
	AllowedOAuthDomains []string
	// Deployment is "public" or "org". OrgName labels the org in UI copy.
	Deployment          string
	OrgName             string
	GitHubWebhookSecret string
	SlackSigningSecret  string
	DevRevWebhookSecret string
	ConnectionProviders ConnectionProviders
	SourcePollInterval  time.Duration
}

// Server wraps the gin engine and owns its background workers.
type Server struct {
	engine                   *gin.Engine
	closeOnce                sync.Once
	knowledgeProcessorCancel context.CancelFunc
	knowledgeProcessorDone   <-chan struct{}
}

type readinessCheck struct {
	name string
	fn   func(context.Context) error
}

type dependencies struct {
	environment          string
	authenticator        auth.Authenticator
	repository           repo.Repository
	knowledge            knowledge.Store
	knowledgeProcessor   bool
	providerClient       knowledge.ProviderClient
	connections          connections.Store
	connectionCipher     *connections.Cipher
	connectionStates     auth.ConnectionStateStore
	connectionProviders  ConnectionProviders
	connectionHTTPClient *http.Client
	sourcePollInterval   time.Duration
	githubWebhookSecret  string
	slackSigningSecret   string
	devRevWebhookSecret  string
	stateStore           auth.LoginStateStore
	oidc                 auth.OIDC
	oidcProviderName     string
	sessionSecret        string
	appOrigin            string
	renderOrigin         string
	allowedOAuthDomains  []string
	deployment           string
	orgName              string
	readinessChecks      []readinessCheck
	documentKVEvents     kvevents.Subscriber
	lifecycleContext     context.Context
}

// Option configures the Server.
type Option func(*dependencies)

func WithAuthenticator(a auth.Authenticator) Option {
	return func(d *dependencies) { d.authenticator = a }
}

func WithRepository(r repo.Repository) Option { return func(d *dependencies) { d.repository = r } }

// WithDocumentKVEvents supplies the process-wide persisted-event broker used
// by document KV SSE streams.
func WithDocumentKVEvents(events kvevents.Subscriber) Option {
	return func(d *dependencies) { d.documentKVEvents = events }
}

// WithKnowledgeStore supplies durable connected-source configuration and
// queue storage. Supplying it also enables the background sync processor.
func WithKnowledgeStore(store knowledge.Store) Option {
	return func(d *dependencies) { d.knowledge = store; d.knowledgeProcessor = true }
}

func WithKnowledgeProviderClient(client knowledge.ProviderClient) Option {
	return func(d *dependencies) { d.providerClient = client }
}

// WithConnectionStore supplies personal provider accounts. The store contains
// encrypted credentials; callers must also provide a Cipher before new real
// accounts can be connected.
func WithConnectionStore(store connections.Store) Option {
	return func(d *dependencies) { d.connections = store }
}

func WithConnectionCipher(cipher *connections.Cipher) Option {
	return func(d *dependencies) { d.connectionCipher = cipher }
}

func WithConnectionStateStore(store auth.ConnectionStateStore) Option {
	return func(d *dependencies) { d.connectionStates = store }
}

func WithConnectionHTTPClient(client *http.Client) Option {
	return func(d *dependencies) { d.connectionHTTPClient = client }
}

func WithStateStore(st auth.LoginStateStore) Option {
	return func(d *dependencies) { d.stateStore = st }
}

func WithReadinessCheck(name string, fn func(context.Context) error) Option {
	return func(d *dependencies) {
		if name != "" && fn != nil {
			d.readinessChecks = append(d.readinessChecks, readinessCheck{name: name, fn: fn})
		}
	}
}

// WithLifecycleContext stops server-owned background work when ctx is
// cancelled. Close must still be called so callers wait for that work to exit
// before closing its dependencies.
func WithLifecycleContext(ctx context.Context) Option {
	return func(d *dependencies) {
		if ctx != nil {
			d.lifecycleContext = ctx
		}
	}
}

// denyAllAuthenticator rejects every request. It is the default authenticator,
// so a server constructed without WithAuthenticator denies all requests instead
// of silently trusting them.
type denyAllAuthenticator struct{}

func (denyAllAuthenticator) Authenticate(_ *http.Request) (*auth.Principal, error) {
	return nil, auth.ErrUnauthorized
}

// New constructs and returns a configured Server. The authenticator defaults to
// one that denies every request, and a repository must be supplied via
// WithRepository outside of tests.
func New(cfg Config, opts ...Option) *Server {
	gin.SetMode(gin.ReleaseMode)
	deps := dependencies{
		environment:         cfg.Environment,
		authenticator:       denyAllAuthenticator{},
		stateStore:          auth.NewStateStore(),
		oidc:                cfg.OIDC,
		oidcProviderName:    cfg.OIDCProviderName,
		sessionSecret:       cfg.SessionSecret,
		appOrigin:           cfg.AppOrigin,
		renderOrigin:        cfg.RenderOrigin,
		allowedOAuthDomains: cfg.AllowedOAuthDomains,
		deployment:          cfg.Deployment,
		orgName:             cfg.OrgName,
		githubWebhookSecret: cfg.GitHubWebhookSecret,
		slackSigningSecret:  cfg.SlackSigningSecret,
		devRevWebhookSecret: cfg.DevRevWebhookSecret,
		connectionProviders: cfg.ConnectionProviders,
		sourcePollInterval:  cfg.SourcePollInterval,
		lifecycleContext:    context.Background(),
	}
	for _, opt := range opts {
		opt(&deps)
	}
	// Missing storage is a startup wiring bug, not a recoverable request error.
	if deps.repository == nil {
		panic("server.New: repository is required (use server.WithRepository)")
	}
	if deps.documentKVEvents == nil {
		// Do not create an unowned background broker for programmatic callers.
		deps.documentKVEvents = unavailableDocumentKVEvents{}
	}
	if deps.knowledge == nil {
		deps.knowledge = knowledge.NewMemory()
	}
	if deps.connections == nil {
		deps.connections = connections.NewMemory()
	}
	if deps.connectionStates == nil {
		if states, ok := deps.stateStore.(auth.ConnectionStateStore); ok {
			deps.connectionStates = states
		}
	}
	if deps.providerClient == nil {
		deps.providerClient = knowledge.HTTPProviderClient{}
	}
	r := gin.New()
	r.RedirectTrailingSlash = false
	r.Use(requestIDMiddleware())
	if cfg.Environment != envTest {
		r.Use(accessLogMiddleware())
	}
	r.Use(recoveryMiddleware())
	r.Use(prometheusMiddleware())
	s := &Server{engine: r}
	s.routes(deps)
	if deps.knowledgeProcessor {
		processorCtx, cancel := context.WithCancel(deps.lifecycleContext)
		s.knowledgeProcessorCancel = cancel
		s.knowledgeProcessorDone = startKnowledgeProcessor(processorCtx, deps)
	}
	return s
}

type unavailableDocumentKVEvents struct{}

func (unavailableDocumentKVEvents) Subscribe(context.Context, string) (<-chan repo.DocumentKVEvent, func(), error) {
	return nil, func() {}, errors.New("repository does not support document KV event streaming")
}

func (s *Server) Handler() http.Handler { return s.engine }

// Close cancels server-owned background work and waits for it to stop. It is
// safe to call more than once.
func (s *Server) Close() {
	s.closeOnce.Do(func() {
		if s.knowledgeProcessorCancel != nil {
			s.knowledgeProcessorCancel()
		}
		if s.knowledgeProcessorDone != nil {
			<-s.knowledgeProcessorDone
		}
	})
}

func (s *Server) routes(deps dependencies) {
	h := handlers{deps: deps}
	s.engine.GET("/.well-known/aidocs.json", h.discovery)
	s.engine.GET("/.well-known/oauth-protected-resource", h.mcpProtectedResourceMetadata)
	s.engine.GET("/.well-known/oauth-protected-resource/mcp", h.mcpProtectedResourceMetadata)
	s.engine.GET("/.well-known/oauth-authorization-server", h.mcpAuthorizationServerMetadata)
	s.engine.POST("/oauth/register", h.mcpRegisterClient)
	s.engine.POST("/oauth/token", h.mcpToken)
	mcpOAuth := s.engine.Group("/oauth", requireAuth(deps.authenticator, deps.appOrigin))
	mcpOAuth.GET("/authorize", h.mcpAuthorize)
	mcpOAuth.POST("/authorize", h.mcpAuthorize)
	registerMCPRoutes(s.engine, deps)
	s.engine.GET("/commit.txt", h.commitTXT)
	s.engine.GET("/version.json", h.versionJSON)
	s.engine.GET("/install.sh", h.installScript)
	s.engine.GET("/skills/:name/SKILL.md", h.downloadSkill)
	s.engine.GET("/cli/:os/:arch", h.downloadCLI)
	registerAPIDocsRoutes(s.engine)
	s.engine.GET("/v/:id/:version_id", h.renderVersion)
	registerFrontendRoutes(s.engine, deps.appOrigin)

	s.engine.GET("/livez", h.livez)
	s.engine.GET("/readyz", h.readyz)

	v1 := s.engine.Group("/v1")
	v1.GET("/config", h.config)
	// The /auth/google/* paths predate generic OIDC support and stay
	// registered so existing Google redirect URIs, CLIs, and bookmarks keep
	// working; /auth/oidc/* is the provider-neutral spelling.
	v1.GET("/auth/google/start", h.oauthStart)
	v1.GET("/auth/google/callback", h.oauthCallback)
	v1.GET("/auth/oidc/start", h.oauthStart)
	v1.GET("/auth/oidc/callback", h.oauthCallback)
	v1.POST("/auth/cli/exchange", h.cliExchange)
	v1.POST("/webhooks/github", h.githubWebhook)
	v1.POST("/webhooks/slack", h.slackWebhook)
	v1.POST("/webhooks/devrev", h.devRevWebhook)
	v1.GET("/connections/:provider/callback", h.connectionCallback)
	v1.GET("/public/portals/:slug", h.publicPortal)

	// Every route below requires authentication.
	authed := v1.Group("", requireAuth(deps.authenticator, deps.appOrigin))
	authed.GET("/me", h.me)
	authed.GET("/connections", h.listConnections)
	authed.GET("/connections/:provider/start", h.startConnection)
	authed.POST("/connections/devrev", h.connectDevRev)
	authed.POST("/connections/:id/google-picker-token", h.googlePickerToken)
	authed.DELETE("/connections/:id", h.disconnectConnection)
	authed.GET("/users", h.searchUsers)
	authed.GET("/users/:id/profile", h.getUserProfile)
	authed.GET("/auth/cli/credentials", h.listCLICredentials)
	authed.DELETE("/auth/cli/credentials/:id", h.revokeCLICredential)
	authed.POST("/service-accounts", h.createServiceAccount)
	authed.GET("/service-accounts", h.listServiceAccounts)
	authed.PATCH("/service-accounts/:id", h.patchServiceAccount)
	authed.POST("/service-accounts/:id/keys", h.createServiceAccountKey)
	authed.GET("/service-accounts/:id/keys", h.listServiceAccountKeys)
	authed.DELETE("/service-accounts/:id/keys/:key_id", h.revokeServiceAccountKey)
	authed.POST("/service-accounts/:id/transfer", h.createOwnershipTransfer)
	authed.GET("/service-accounts/transfers", h.listOwnershipTransfers)
	authed.POST("/service-accounts/transfers/:id/accept", h.acceptOwnershipTransfer)
	authed.POST("/service-accounts/transfers/:id/decline", h.declineOwnershipTransfer)
	authed.POST("/documents", h.createDocument)
	authed.POST("/repositories", h.createRepository)
	authed.POST("/repositories/import", h.importRepository)
	authed.GET("/repositories", h.listRepositories)
	authed.GET("/repositories/:id", h.getRepository)
	authed.DELETE("/repositories/:id", h.deleteRepository)
	registerKnowledgeRoutes(authed, h)
	authed.GET("/repositories/:id/documents", h.listRepositoryDocuments)
	authed.POST("/repositories/:id/documents", h.createRepositoryDocument)
	authed.POST("/repositories/:id/pull-requests", h.createPullRequest)
	authed.POST("/repositories/:id/pull-requests/change-set", h.createPullRequestChangeSet)
	authed.GET("/repositories/:id/pull-requests", h.listPullRequests)
	authed.GET("/repositories/:id/pull-requests/:number", h.getPullRequest)
	authed.POST("/repositories/:id/pull-requests/:number/patchsets", h.createPullRequestPatchset)
	authed.POST("/repositories/:id/pull-requests/:number/rebase", h.rebasePullRequest)
	authed.POST("/repositories/:id/pull-requests/:number/reviews", h.reviewPullRequest)
	authed.POST("/repositories/:id/pull-requests/:number/merge", h.mergePullRequest)
	authed.PATCH("/repositories/:id/pull-requests/:number", h.patchPullRequest)
	authed.GET("/documents", h.listDocuments)
	authed.GET("/documents/:id", h.getDocument)
	authed.GET("/documents/:id/access", h.documentAccess)
	authed.PATCH("/documents/:id", h.patchDocument)
	authed.DELETE("/documents/:id", h.deleteDocument)
	authed.POST("/documents/:id/grants", h.createGrant)
	authed.PATCH("/documents/:id/grants/:grant_id", h.patchGrant)
	authed.DELETE("/documents/:id/grants/:grant_id", h.deleteGrant)
	authed.POST("/documents/:id/versions", h.createVersion)
	authed.GET("/documents/:id/versions", h.listVersions)
	authed.GET("/documents/:id/versions/:version_id", h.getVersion)
	authed.GET("/documents/:id/versions/:version_id/html", h.getVersionHTML)
	authed.POST("/documents/:id/versions/:version_id/render-token", h.createRenderToken)
	authed.POST("/documents/:id/comments", h.createComment)
	authed.GET("/documents/:id/comments", h.listComments)
	authed.GET("/documents/:id/kv", h.listDocumentKV)
	authed.GET("/documents/:id/kv/events", h.streamDocumentKVEvents)
	authed.PATCH("/documents/:id/kv", h.patchDocumentKV)
	authed.POST("/documents/:id/comments/:comment_id/replies", h.createCommentReply)
	authed.PATCH("/documents/:id/comments/:comment_id", h.patchComment)
	authed.DELETE("/documents/:id/comments/:comment_id", h.deleteComment)
	authed.POST("/folders", h.createFolder)
	authed.GET("/folders", h.listFolders)
	authed.GET("/folders/:id", h.getFolder)
	authed.PATCH("/folders/:id", h.patchFolder)
	authed.DELETE("/folders/:id", h.deleteFolder)
	authed.GET("/folders/:id/access", h.folderAccess)
	authed.POST("/folders/:id/grants", h.createFolderGrant)
	authed.PATCH("/folders/:id/grants/:grant_id", h.patchFolderGrant)
	authed.DELETE("/folders/:id/grants/:grant_id", h.deleteFolderGrant)
	authed.GET("/folders/:id/documents", h.listFolderDocs)
	authed.POST("/folders/:id/documents", h.moveDocToFolder)
	authed.DELETE("/folders/:id/documents/:doc_id", h.moveDocToRoot)
	authed.GET("/pins", h.listPins)
	authed.PUT("/pins/:resource_type/:resource_id", h.putPin)
	authed.DELETE("/pins/:resource_type/:resource_id", h.deletePin)
}

type handlers struct{ deps dependencies }

const principalContextKey = "principal"

func (h handlers) needDocRole(c *gin.Context, docID string, need repo.Role) bool {
	role, err := h.deps.repository.RoleForDocument(c.Request.Context(), *current(c), docID)
	if err != nil {
		// A lookup failure is a server fault, not a permission verdict.
		internalErr(c, err)
		return false
	}
	if !atLeast(role, need) {
		forbidden(c, string(need)+" role required")
		return false
	}
	return true
}

func requireAuth(a auth.Authenticator, appOrigin string) gin.HandlerFunc {
	return func(c *gin.Context) {
		p, err := a.Authenticate(c.Request)
		if err != nil {
			incAuth("request", "failure")
			if wantsBrowserLogin(c) {
				redirect := c.Request.URL.RequestURI()
				if !safeWebRedirect(redirect) {
					redirect = "/"
				}
				c.Abort()
				c.Redirect(http.StatusFound, "/v1/auth/google/start?mode=web&redirect="+url.QueryEscape(redirect))
				return
			}
			c.AbortWithStatusJSON(http.StatusUnauthorized, errorResponse("unauthorized", "authentication required", nil))
			return
		}
		if !allowCookieUnsafeRequest(c, appOrigin) {
			incAuth("request", "failure")
			c.Abort()
			forbidden(c, "csrf check failed")
			return
		}
		incAuth("request", "success")
		c.Set(principalContextKey, p)
		c.Next()
	}
}

func allowCookieUnsafeRequest(c *gin.Context, appOrigin string) bool {
	switch c.Request.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return true
	}
	if auth.BearerFromHeader(c.GetHeader("Authorization")) != "" {
		return true
	}
	if _, err := c.Request.Cookie(sessionCookieName); err != nil {
		return true
	}
	if appOrigin == "" {
		return true
	}
	if origin := c.GetHeader("Origin"); origin != "" {
		return sameHTTPOrigin(origin, appOrigin)
	}
	if referer := c.GetHeader("Referer"); referer != "" {
		return sameHTTPOrigin(referer, appOrigin)
	}
	return false
}

func wantsBrowserLogin(c *gin.Context) bool {
	if c.Request.Method != http.MethodGet {
		return false
	}
	accept := c.GetHeader("Accept")
	return strings.Contains(accept, "text/html") && !strings.Contains(accept, "application/json")
}

func current(c *gin.Context) *auth.Principal {
	p, ok := c.Get(principalContextKey)
	if !ok {
		return nil
	}
	principal, _ := p.(*auth.Principal)
	return principal
}

var errPayloadTooLarge = errors.New("payload too large")

const maxHTMLBytes = 10 * 1024 * 1024

func readMultipartFile(c *gin.Context, field string) ([]byte, error) {
	f, err := c.FormFile(field)
	if err != nil {
		return nil, err
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rc.Close()
	}()
	b, err := io.ReadAll(io.LimitReader(rc, maxHTMLBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxHTMLBytes {
		return nil, errPayloadTooLarge
	}
	return b, nil
}
