package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/razorpay/aidocs/api/internal/auth"
	"github.com/razorpay/aidocs/api/internal/blob"
	"github.com/razorpay/aidocs/api/internal/db"
	"github.com/razorpay/aidocs/api/internal/kvevents"
	pgrepo "github.com/razorpay/aidocs/api/internal/repo/postgres"
	"github.com/razorpay/aidocs/api/internal/server"
)

// HTTP server timeouts. WriteTimeout is deliberately absent (zero): the KV
// event stream holds SSE responses open indefinitely, and a write deadline
// would sever them.
const (
	readHeaderTimeout = 10 * time.Second
	readTimeout       = 60 * time.Second
	idleTimeout       = 120 * time.Second
	shutdownTimeout   = 15 * time.Second
)

// @title aidocs API
// @version v1
// @description This API powers a review tool where people can upload one self-contained HTML file, view it, and leave comments anchored to specific text ranges inside that HTML.
// @BasePath /
// @securityDefinitions.apikey cookieAuth
// @in cookie
// @name aidocs_session
// @securityDefinitions.apikey bearerAuth
// @in header
// @name Authorization
func main() {
	slog.SetDefault(slog.New(newLogHandler(os.Stderr)))
	if err := run(); err != nil {
		slog.Error("server exiting", slog.Any("error", err))
		os.Exit(1)
	}
}

// newLogHandler picks the log format: human-readable text on a terminal
// (local development), JSON otherwise (production, where logs feed a
// structured pipeline).
func newLogHandler(f *os.File) slog.Handler {
	if info, err := f.Stat(); err == nil && info.Mode()&os.ModeCharDevice != 0 {
		return slog.NewTextHandler(f, nil)
	}
	return slog.NewJSONHandler(f, nil)
}

func run() error {
	if err := server.ValidateEmbeddedBridge(); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	databaseURL, err := requiredEnv("DATABASE_URL")
	if err != nil {
		return err
	}
	kvEventsMode, err := kvevents.ParseMode(os.Getenv("AIDOCS_KV_EVENTS_MODE"))
	if err != nil {
		return err
	}
	if os.Getenv("AIDOCS_MIGRATE") != "false" {
		if err := db.Migrate(databaseURL); err != nil {
			return err
		}
	}
	blobs, err := newBlobStore(ctx)
	if err != nil {
		return err
	}
	store, err := pgrepo.ConnectWithBlob(ctx, databaseURL, blobs)
	if err != nil {
		return err
	}
	defer store.Close()

	listenerFactory := newKVEventListenerFactory(
		kvEventsMode,
		databaseURL,
		os.Getenv("AIDOCS_KV_EVENTS_LISTENER_DATABASE_URL"),
		store.Pool(),
	)
	kvEvents, err := kvevents.New(ctx, kvevents.Config{Mode: kvEventsMode}, store, listenerFactory)
	if err != nil {
		return err
	}
	defer kvEvents.Close()
	slog.Info("document KV event delivery ready",
		slog.String("requested_mode", string(kvEvents.RequestedMode())),
		slog.String("active_mode", string(kvEvents.ActiveMode())))

	srv, err := newApplicationServer(ctx, store, blobs, kvEvents)
	if err != nil {
		return err
	}
	return serve(ctx, srv.Handler(), ":"+envOrDefault("AIDOCS_PORT", "8080"), ":"+envOrDefault("AIDOCS_METRICS_PORT", "9090"))
}

// newKVEventListenerFactory returns the interface type, not the concrete
// *kvevents.PostgresListenerFactory: returning a nil concrete pointer here
// would box into a non-nil interface, defeat the broker's nil-factory
// fallback, and panic on Open in the default auto-mode configuration.
func newKVEventListenerFactory(mode kvevents.Mode, databaseURL, configuredListenerURL string, sender kvevents.NotificationSender) kvevents.ListenerFactory {
	listenerURL := strings.TrimSpace(configuredListenerURL)
	switch mode {
	case kvevents.ModeAuto:
		// Auto mode never probes an unknown DATABASE_URL because it may point at
		// a transaction-pooling proxy that cannot safely preserve LISTEN state.
		if listenerURL == "" {
			return nil
		}
	case kvevents.ModeNotify:
		// Forced notify is an explicit operator assertion that DATABASE_URL is
		// safe when no dedicated listener URL is supplied.
		if listenerURL == "" {
			listenerURL = databaseURL
		}
	case kvevents.ModePoll:
		return nil
	}
	return kvevents.NewPostgresListenerFactory(listenerURL, sender)
}

type originSettings struct {
	appOrigin     string
	renderOrigin  string
	sessionSecret string
}

func loadOriginSettings() (originSettings, error) {
	appOrigin, err := requiredEnv("AIDOCS_APP_ORIGIN")
	if err != nil {
		return originSettings{}, err
	}
	if err := validateOrigin("AIDOCS_APP_ORIGIN", appOrigin); err != nil {
		return originSettings{}, err
	}
	renderOrigin, err := requiredEnv("AIDOCS_RENDER_ORIGIN")
	if err != nil {
		return originSettings{}, err
	}
	if err := validateOrigin("AIDOCS_RENDER_ORIGIN", renderOrigin); err != nil {
		return originSettings{}, err
	}
	if err := server.ValidateRenderOrigin(appOrigin, renderOrigin); err != nil {
		if !errors.Is(err, server.ErrUnsafeRenderOrigin) || os.Getenv("AIDOCS_ALLOW_UNSAFE_RENDER_ORIGIN") != "true" {
			return originSettings{}, err
		}
		slog.Warn("UNSAFE: AIDOCS_RENDER_ORIGIN shares a hostname with AIDOCS_APP_ORIGIN; scheme and port do not isolate host-scoped cookies, so uploaded document HTML can script against the app. Never run production like this.",
			slog.String("app_origin", appOrigin),
			slog.String("render_origin", renderOrigin))
	}
	sessionSecret := os.Getenv("SESSION_SECRET")
	if len(sessionSecret) < 32 {
		return originSettings{}, errors.New("SESSION_SECRET must be at least 32 bytes")
	}
	return originSettings{appOrigin: appOrigin, renderOrigin: renderOrigin, sessionSecret: sessionSecret}, nil
}

type deploymentSettings struct {
	name                string
	orgName             string
	allowedOAuthDomains []string
}

func loadDeploymentSettings() (deploymentSettings, error) {
	deployment := envOrDefault("AIDOCS_DEPLOYMENT", server.DeploymentPublic)
	allowedOAuthDomains := splitCSV(os.Getenv("ALLOWED_OAUTH_DOMAINS"))
	switch deployment {
	case server.DeploymentPublic:
	case server.DeploymentOrg:
		allowedOAuthDomains = splitCSV(os.Getenv("AIDOCS_ORG_DOMAINS"))
		if len(allowedOAuthDomains) == 0 {
			return deploymentSettings{}, errors.New("AIDOCS_ORG_DOMAINS is required when AIDOCS_DEPLOYMENT=org")
		}
	default:
		return deploymentSettings{}, fmt.Errorf("AIDOCS_DEPLOYMENT must be %q or %q (got %q)", server.DeploymentPublic, server.DeploymentOrg, deployment)
	}
	return deploymentSettings{name: deployment, orgName: os.Getenv("AIDOCS_ORG_NAME"), allowedOAuthDomains: allowedOAuthDomains}, nil
}

func loadOIDC(ctx context.Context, appOrigin string) (auth.OIDC, string, error) {
	settings, err := auth.ResolveOIDCSettings(os.Getenv)
	if err != nil {
		return auth.OIDC{}, "", err
	}
	callbackPath := "/v1/auth/oidc/callback"
	if settings.IssuerURL == auth.GoogleIssuer {
		callbackPath = "/v1/auth/google/callback"
	}
	provider, err := auth.NewOIDC(ctx, auth.OIDCConfig{
		IssuerURL:         settings.IssuerURL,
		InternalIssuerURL: settings.InternalIssuerURL,
		ClientID:          settings.ClientID,
		ClientSecret:      settings.ClientSecret,
		RedirectURL:       appOrigin + callbackPath,
	})
	return provider, settings.ProviderName, err
}

func newApplicationServer(ctx context.Context, store *pgrepo.Store, blobs blob.Store, kvEvents kvevents.Subscriber) (*server.Server, error) {
	origins, err := loadOriginSettings()
	if err != nil {
		return nil, err
	}
	deployment, err := loadDeploymentSettings()
	if err != nil {
		return nil, err
	}
	oidcProvider, providerName, err := loadOIDC(ctx, origins.appOrigin)
	if err != nil {
		return nil, err
	}
	options := []server.Option{
		server.WithRepository(store),
		server.WithDocumentKVEvents(kvEvents),
		server.WithAuthenticator(auth.DBAuthenticator{Resolver: store, SessionSecret: origins.sessionSecret}),
		server.WithStateStore(auth.NewPostgresStateStore(store.Pool())),
		server.WithReadinessCheck("postgres", store.Ping),
	}
	if pinger, ok := blobs.(blob.Pinger); ok {
		options = append(options, server.WithReadinessCheck("blob", pinger.Ping))
	}
	return server.New(server.Config{
		Environment:         server.EnvProduction,
		AppOrigin:           origins.appOrigin,
		RenderOrigin:        origins.renderOrigin,
		OIDC:                oidcProvider,
		OIDCProviderName:    providerName,
		SessionSecret:       origins.sessionSecret,
		AllowedOAuthDomains: deployment.allowedOAuthDomains,
		Deployment:          deployment.name,
		OrgName:             deployment.orgName,
		PrivateByDefault:    os.Getenv("AIDOCS_PRIVATE_BY_DEFAULT") != "",
	}, options...), nil
}

func serve(ctx context.Context, handler http.Handler, addr, metricsAddr string) error {
	appServer := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		IdleTimeout:       idleTimeout,
	}
	// Prometheus metrics get a separate listener so /metrics stays off the
	// public app port (expose only addr publicly; scrape metricsAddr over
	// private networking).
	metricsServer := &http.Server{
		Addr:              metricsAddr,
		Handler:           server.MetricsHandler(),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		IdleTimeout:       idleTimeout,
	}

	serverErr := make(chan error, 2)
	var serverWG sync.WaitGroup
	serverWG.Add(2)
	go func() {
		defer serverWG.Done()
		slog.Info("metrics listening", slog.String("addr", metricsAddr))
		if err := metricsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- fmt.Errorf("metrics server: %w", err)
		}
	}()
	go func() {
		defer serverWG.Done()
		slog.Info("server listening", slog.String("addr", addr))
		if err := appServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- fmt.Errorf("app server: %w", err)
		}
	}()

	var serveErr error
	select {
	case serveErr = <-serverErr:
	case <-ctx.Done():
	}

	// Graceful shutdown: stop accepting connections, then drain in-flight
	// requests (SSE streams included) for up to shutdownTimeout.
	slog.Info("shutting down", slog.Duration("timeout", shutdownTimeout))
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	appShutdownErr := shutdownHTTPServer(shutdownCtx, appServer)
	metricsShutdownErr := shutdownHTTPServer(shutdownCtx, metricsServer)
	serverWG.Wait()
	return errors.Join(serveErr, appShutdownErr, metricsShutdownErr)
}

// shutdownHTTPServer drains in-flight requests until ctx expires, then forces
// any remaining long-lived connections closed so the listener goroutine can
// always terminate.
func shutdownHTTPServer(ctx context.Context, srv *http.Server) error {
	shutdownErr := srv.Shutdown(ctx)
	if shutdownErr == nil {
		return nil
	}
	closeErr := srv.Close()
	if errors.Is(closeErr, http.ErrServerClosed) {
		closeErr = nil
	}
	return errors.Join(shutdownErr, closeErr)
}

func requiredEnv(name string) (string, error) {
	value := os.Getenv(name)
	if value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func validateOrigin(name, value string) error {
	if strings.HasSuffix(value, "/") {
		return fmt.Errorf("%s must not end with a trailing slash (got %q). Set it to something like https://example.com", name, value)
	}
	u, err := url.Parse(value)
	if err != nil || u.Scheme == "" || u.Host == "" || u.Hostname() == "" {
		return fmt.Errorf("%s must be an absolute URL with scheme http:// or https:// (got %q). Set it to something like https://example.com", name, value)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%s must use http:// or https:// (got scheme %q in %q)", name, u.Scheme, value)
	}
	if !validOriginPort(u.Port()) {
		return fmt.Errorf("%s has an invalid port in %q", name, value)
	}
	if u.Path != "" && u.Path != "/" {
		return fmt.Errorf("%s must not include a path (got %q). Set it to something like https://example.com", name, value)
	}
	if u.User != nil || u.RawQuery != "" || u.ForceQuery || strings.Contains(value, "#") {
		return fmt.Errorf("%s must be an origin without credentials, query parameters, or a fragment (got %q). Set it to something like https://example.com", name, value)
	}
	return nil
}

func validOriginPort(port string) bool {
	if port == "" {
		return true
	}
	_, err := strconv.ParseUint(port, 10, 16)
	return err == nil
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func newBlobStore(ctx context.Context) (blob.Store, error) {
	bucket := os.Getenv("BLOB_BUCKET")
	if bucket == "" {
		return nil, fmt.Errorf("BLOB_BUCKET is required")
	}
	region := os.Getenv("BLOB_REGION")
	if region == "" {
		region = os.Getenv("AWS_REGION")
	}
	if region == "" {
		region = "us-east-1"
	}
	return blob.NewS3(ctx, blob.S3Config{Bucket: bucket, Region: region, Endpoint: os.Getenv("BLOB_ENDPOINT"), AccessKeyID: os.Getenv("BLOB_ACCESS_KEY_ID"), SecretAccessKey: os.Getenv("BLOB_SECRET_ACCESS_KEY"), ForcePathStyle: os.Getenv("BLOB_FORCE_PATH_STYLE") == "true"})
}
