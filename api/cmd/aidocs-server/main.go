package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/razorpay/aidocs/api/internal/auth"
	"github.com/razorpay/aidocs/api/internal/blob"
	"github.com/razorpay/aidocs/api/internal/db"
	"github.com/razorpay/aidocs/api/internal/googlegroups"
	pgrepo "github.com/razorpay/aidocs/api/internal/repo/postgres"
	"github.com/razorpay/aidocs/api/internal/server"
	"golang.org/x/oauth2/google"
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
	port := os.Getenv("AIDOCS_PORT")
	if port == "" {
		port = "8080"
	}
	addr := ":" + port
	metricsPort := os.Getenv("AIDOCS_METRICS_PORT")
	if metricsPort == "" {
		metricsPort = "9090"
	}
	metricsAddr := ":" + metricsPort

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	if os.Getenv("AIDOCS_MIGRATE") != "false" {
		if err := db.Migrate(databaseURL); err != nil {
			log.Fatal(err)
		}
	}
	blobs, err := newBlobStore(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	store, err := pgrepo.ConnectWithBlob(context.Background(), databaseURL, blobs)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	appOrigin := os.Getenv("AIDOCS_APP_ORIGIN")
	if appOrigin == "" {
		log.Fatal("AIDOCS_APP_ORIGIN is required")
	}
	if err := validateOrigin("AIDOCS_APP_ORIGIN", appOrigin); err != nil {
		log.Fatal(err)
	}
	clientID := os.Getenv("GOOGLE_OAUTH_CLIENT_ID")
	clientSecret := os.Getenv("GOOGLE_OAUTH_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		log.Fatal("GOOGLE_OAUTH_CLIENT_ID and GOOGLE_OAUTH_CLIENT_SECRET are required")
	}
	sessionSecret := os.Getenv("SESSION_SECRET")
	if len(sessionSecret) < 32 {
		log.Fatal("SESSION_SECRET must be at least 32 bytes")
	}

	renderOrigin := os.Getenv("AIDOCS_RENDER_ORIGIN")
	if renderOrigin == "" {
		log.Fatal("AIDOCS_RENDER_ORIGIN is required")
	}
	if err := validateOrigin("AIDOCS_RENDER_ORIGIN", renderOrigin); err != nil {
		log.Fatal(err)
	}

	// Deployment type. An org deployment gates Google login to the org's
	// domains; those domains double as the OAuth allow-list. A public
	// deployment lets any Google account in (subject to ALLOWED_OAUTH_DOMAINS).
	deployment := os.Getenv("AIDOCS_DEPLOYMENT")
	if deployment == "" {
		deployment = server.DeploymentPublic
	}
	orgDomains := splitCSV(os.Getenv("AIDOCS_ORG_DOMAINS"))
	allowedOAuthDomains := splitCSV(os.Getenv("ALLOWED_OAUTH_DOMAINS"))
	orgName := os.Getenv("AIDOCS_ORG_NAME")
	switch deployment {
	case server.DeploymentPublic:
	case server.DeploymentOrg:
		if len(orgDomains) == 0 {
			log.Fatal("AIDOCS_ORG_DOMAINS is required when AIDOCS_DEPLOYMENT=org")
		}
		// On an org deployment the org's domains are the login gate.
		allowedOAuthDomains = orgDomains
	default:
		log.Fatalf("AIDOCS_DEPLOYMENT must be %q or %q (got %q)", server.DeploymentPublic, server.DeploymentOrg, deployment)
	}
	groupConfig, err := loadGoogleGroupsConfig(context.Background(), deployment)
	if err != nil {
		log.Fatal(err)
	}

	options := []server.Option{
		server.WithRepository(store),
		server.WithAuthenticator(auth.DBAuthenticator{Resolver: store, SessionSecret: sessionSecret}),
		server.WithStateStore(auth.NewPostgresStateStore(store.Pool())),
		server.WithReadinessCheck("postgres", store.Ping),
	}
	if pinger, ok := blobs.(blob.Pinger); ok {
		options = append(options, server.WithReadinessCheck("blob", pinger.Ping))
	}
	if groupConfig.directory != nil {
		options = append(options, server.WithGoogleGroupDirectory(groupConfig.directory))
	}
	srv := server.New(server.Config{
		Environment:               server.EnvProduction,
		AppOrigin:                 appOrigin,
		RenderOrigin:              renderOrigin,
		GoogleOAuth:               auth.NewGoogleOAuth(clientID, clientSecret, appOrigin+"/v1/auth/google/callback"),
		SessionSecret:             sessionSecret,
		AllowedOAuthDomains:       allowedOAuthDomains,
		Deployment:                deployment,
		OrgName:                   orgName,
		GoogleGroupsEnabled:       groupConfig.enabled,
		GoogleGroupsMembershipTTL: groupConfig.membershipTTL,
	}, options...)

	// Serve Prometheus metrics on a separate listener so /metrics is kept off
	// the public app port (expose only `addr` publicly; scrape metricsAddr over
	// private networking).
	go func() {
		log.Printf("metrics listening on %s", metricsAddr)
		if err := http.ListenAndServe(metricsAddr, server.MetricsHandler()); err != nil {
			log.Fatalf("metrics server: %v", err)
		}
	}()

	if err := srv.Run(addr); err != nil {
		log.Fatal(err)
	}
}

type googleGroupsConfig struct {
	enabled       bool
	membershipTTL time.Duration
	directory     googlegroups.GoogleGroupDirectory
}

func loadGoogleGroupsConfig(ctx context.Context, deployment string) (googleGroupsConfig, error) {
	var cfg googleGroupsConfig
	enabledRaw := os.Getenv("AIDOCS_GOOGLE_GROUPS_ENABLED")
	if enabledRaw == "" {
		enabledRaw = "false"
	}
	if enabledRaw != "true" && enabledRaw != "false" {
		return cfg, fmt.Errorf("AIDOCS_GOOGLE_GROUPS_ENABLED must be true or false")
	}
	enabled := enabledRaw == "true"
	cfg.enabled = enabled
	if !enabled {
		return cfg, nil
	}
	if deployment != server.DeploymentOrg {
		return cfg, fmt.Errorf("AIDOCS_GOOGLE_GROUPS_ENABLED requires AIDOCS_DEPLOYMENT=org")
	}
	customerID := os.Getenv("GOOGLE_GROUPS_CUSTOMER_ID")
	adminEmail := strings.TrimSpace(os.Getenv("GOOGLE_GROUPS_ADMIN_EMAIL"))
	credentialsFile := os.Getenv("GOOGLE_GROUPS_CREDENTIALS_FILE")
	if !strings.HasPrefix(customerID, "C") || strings.ContainsAny(customerID, " /\\\t\r\n") {
		return cfg, fmt.Errorf("GOOGLE_GROUPS_CUSTOMER_ID must be a Workspace customer ID beginning with C")
	}
	if adminEmail == "" || !strings.Contains(adminEmail, "@") {
		return cfg, fmt.Errorf("GOOGLE_GROUPS_ADMIN_EMAIL must be a valid delegated administrator email")
	}
	if credentialsFile == "" {
		return cfg, fmt.Errorf("GOOGLE_GROUPS_CREDENTIALS_FILE is required when Google Groups are enabled")
	}
	credentials, err := os.ReadFile(credentialsFile)
	if err != nil {
		return cfg, fmt.Errorf("GOOGLE_GROUPS_CREDENTIALS_FILE must be present and readable")
	}
	ttl, err := positiveDurationEnv("GOOGLE_GROUPS_MEMBERSHIP_TTL", time.Minute)
	if err != nil {
		return cfg, err
	}
	requestTimeout, err := positiveDurationEnv("GOOGLE_GROUPS_REQUEST_TIMEOUT", 5*time.Second)
	if err != nil {
		return cfg, err
	}
	jwtConfig, err := google.JWTConfigFromJSON(credentials, googlegroups.ReadOnlyScope)
	if err != nil {
		return cfg, fmt.Errorf("GOOGLE_GROUPS_CREDENTIALS_FILE does not contain valid service-account credentials")
	}
	jwtConfig.Subject = adminEmail
	directory, err := googlegroups.NewHTTPDirectory(googlegroups.HTTPDirectoryConfig{
		Client: jwtConfig.Client(ctx), CustomerID: customerID, Timeout: requestTimeout,
		Observer: server.ObserveGoogleGroupRequest,
	})
	if err != nil {
		return cfg, err
	}
	cfg.membershipTTL = ttl
	cfg.directory = directory
	return cfg, nil
}

func positiveDurationEnv(name string, fallback time.Duration) (time.Duration, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback, nil
	}
	duration, err := time.ParseDuration(raw)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("%s must be a positive Go duration", name)
	}
	return duration, nil
}

func validateOrigin(name, value string) error {
	if strings.HasSuffix(value, "/") {
		return fmt.Errorf("%s must not end with a trailing slash (got %q). Set it to something like https://example.com", name, value)
	}
	u, err := url.Parse(value)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("%s must be an absolute URL with scheme http:// or https:// (got %q). Set it to something like https://example.com", name, value)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%s must use http:// or https:// (got scheme %q in %q)", name, u.Scheme, value)
	}
	if u.Path != "" && u.Path != "/" {
		return fmt.Errorf("%s must not include a path (got %q). Set it to something like https://example.com", name, value)
	}
	return nil
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
