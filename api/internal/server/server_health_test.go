package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/razorpay/aidocs/api/internal/auth"
	"github.com/razorpay/aidocs/api/internal/repo"
	"github.com/razorpay/aidocs/api/internal/server"
)

func TestLiveness(t *testing.T) {
	rr := do(t, newTestEnv(t).h, http.MethodGet, "/livez", "", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	assertJSON(t, rr.Body.Bytes(), `{"status":"ok"}`)
}

func TestReadiness(t *testing.T) {
	ok := server.New(server.Config{Environment: "test"},
		server.WithRepository(newTestStore(t)),
		server.WithAuthenticator(testPrincipalAuth{}),
		server.WithReadinessCheck("postgres", func(context.Context) error { return nil }),
		server.WithReadinessCheck("blob", func(context.Context) error { return nil }),
	).Handler()
	rr := do(t, ok, http.MethodGet, "/readyz", "", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	assertJSON(t, rr.Body.Bytes(), `{"status":"ok","checks":{"postgres":"ok","blob":"ok"}}`)

	fail := server.New(server.Config{Environment: "test"},
		server.WithRepository(newTestStore(t)),
		server.WithAuthenticator(testPrincipalAuth{}),
		server.WithReadinessCheck("postgres", func(context.Context) error { return nil }),
		server.WithReadinessCheck("blob", func(context.Context) error { return context.Canceled }),
	).Handler()
	rr = do(t, fail, http.MethodGet, "/readyz", "", nil)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: %s", rr.Code, rr.Body.String())
	}
	assertJSON(t, rr.Body.Bytes(), `{"status":"error","checks":{"postgres":"ok","blob":"error"}}`)
}

func TestDiscovery(t *testing.T) {
	rr := do(t, newTestEnv(t).h, http.MethodGet, "/.well-known/aidocs.json", "", nil)
	assertStatus(t, rr, http.StatusOK)
	assertJSON(t, rr.Body.Bytes(), `{
	  "name": "aidocs",
	  "api_version": "v1",
	  "auth": { "modes": ["web", "cli", "mcp"] }
	}`)
}

func TestMetricsEndpoint(t *testing.T) {
	h := newTestEnv(t).h
	_ = do(t, h, http.MethodGet, "/v1/me", "", nil)
	rr := do(t, server.MetricsHandler(), http.MethodGet, "/metrics", "", nil)
	assertStatus(t, rr, http.StatusOK)
	body := rr.Body.String()
	for _, want := range []string{"aidocs_http_requests_total", "aidocs_auth_attempts_total", "go_goroutines"} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics response missing %q\n%s", want, body)
		}
	}
}

func TestAuthFailureMetricIncrements(t *testing.T) {
	h := newTestEnv(t).h
	before := scrapeMetric(t, "aidocs_auth_attempts_total", map[string]string{"kind": "request", "outcome": "failure"})
	_ = do(t, h, http.MethodGet, "/v1/me", "", nil)
	after := scrapeMetric(t, "aidocs_auth_attempts_total", map[string]string{"kind": "request", "outcome": "failure"})
	if after != before+1 {
		t.Fatalf("auth failure counter = %v, want %v", after, before+1)
	}
}

func TestDocumentCreateMetricsIncrement(t *testing.T) {
	e := newTestEnv(t)
	docBefore := scrapeMetric(t, "aidocs_document_events_total", map[string]string{"event": "created", "actor_type": "user"})
	verBefore := scrapeMetric(t, "aidocs_version_events_total", map[string]string{"event": "created_initial", "actor_type": "user"})
	body, contentType := multipartBody(t, map[string]string{"title": "Metrics doc"}, "file", "doc.html", "<html><body>metrics</body></html>")
	rr := doRaw(t, e.h, http.MethodPost, "/v1/documents", body, map[string]string{
		"Content-Type":     contentType,
		"X-Test-Principal": principalHeader(fixtureUser),
	})
	assertStatus(t, rr, http.StatusCreated)
	docAfter := scrapeMetric(t, "aidocs_document_events_total", map[string]string{"event": "created", "actor_type": "user"})
	verAfter := scrapeMetric(t, "aidocs_version_events_total", map[string]string{"event": "created_initial", "actor_type": "user"})
	if docAfter != docBefore+1 {
		t.Fatalf("document create counter = %v, want %v", docAfter, docBefore+1)
	}
	if verAfter != verBefore+1 {
		t.Fatalf("initial version counter = %v, want %v", verAfter, verBefore+1)
	}
}

func TestAPIDocsRoutes(t *testing.T) {
	h := newTestEnv(t).h

	openapi := do(t, h, http.MethodGet, "/openapi.json", "", nil)
	assertStatus(t, openapi, http.StatusOK)
	var spec map[string]any
	if err := json.Unmarshal(openapi.Body.Bytes(), &spec); err != nil {
		t.Fatalf("openapi response is not JSON: %v", err)
	}
	if spec["swagger"] != "2.0" {
		t.Fatalf("swagger version = %v, want 2.0", spec["swagger"])
	}
	paths, ok := spec["paths"].(map[string]any)
	if !ok || paths["/v1/documents"] == nil || paths["/v1/service-accounts"] == nil {
		t.Fatalf("openapi paths missing expected endpoints: %#v", spec["paths"])
	}
	definitions, ok := spec["definitions"].(map[string]any)
	if !ok {
		t.Fatalf("openapi definitions = %#v, want object", spec["definitions"])
	}
	const mentionPrincipalTypeRef = "#/definitions/server.commentMentionPrincipalType"
	for _, schemaName := range []string{
		"server.commentMentionPrincipalRef",
		"server.commentMentionPrincipalResponse",
	} {
		schema, ok := definitions[schemaName].(map[string]any)
		if !ok {
			t.Fatalf("openapi definition %q = %#v, want object", schemaName, definitions[schemaName])
		}
		properties, ok := schema["properties"].(map[string]any)
		if !ok {
			t.Fatalf("openapi definition %q properties = %#v, want object", schemaName, schema["properties"])
		}
		typeProperty, ok := properties["type"].(map[string]any)
		if !ok || typeProperty["$ref"] != mentionPrincipalTypeRef {
			t.Fatalf("openapi definition %q type = %#v, want ref %q", schemaName, properties["type"], mentionPrincipalTypeRef)
		}
	}
	mentionPrincipalType, ok := definitions["server.commentMentionPrincipalType"].(map[string]any)
	if !ok {
		t.Fatalf("openapi mention principal type = %#v, want object", definitions["server.commentMentionPrincipalType"])
	}
	enum, ok := mentionPrincipalType["enum"].([]any)
	if !ok || len(enum) != 2 || enum[0] != "user" || enum[1] != "service_account" {
		t.Fatalf("openapi mention principal enum = %#v, want [user service_account]", mentionPrincipalType["enum"])
	}

	docs := do(t, h, http.MethodGet, "/api-docs", "", nil)
	assertStatus(t, docs, http.StatusFound)
	if loc := docs.Header().Get("Location"); loc != "/api-docs/index.html" {
		t.Fatalf("Location = %q, want /api-docs/index.html", loc)
	}
}

func TestConfigReportsDeploymentWording(t *testing.T) {
	public := do(t, newTestEnv(t).h, http.MethodGet, "/v1/config", "", nil)
	assertStatus(t, public, http.StatusOK)
	assertJSON(t, public.Body.Bytes(), `{
	  "deployment": "public",
	  "org_name": "",
	  "everyone_label": "Anyone with the link",
	  "provider_name": "SSO"
	}`)

	org := newConfiguredTestEnv(t, server.Config{Environment: "test", Deployment: "org", OrgName: "Acme", OIDCProviderName: "Google"})
	rr := do(t, org.h, http.MethodGet, "/v1/config", "", nil)
	assertStatus(t, rr, http.StatusOK)
	assertJSON(t, rr.Body.Bytes(), `{
	  "deployment": "org",
	  "org_name": "Acme",
	  "everyone_label": "Anyone in Acme",
	  "provider_name": "Google"
	}`)

	development := newConfiguredTestEnv(t, server.Config{Environment: server.EnvDevelopment, Deployment: "org", OrgName: "Local Development"})
	rr = do(t, development.h, http.MethodGet, "/v1/config", "", nil)
	assertStatus(t, rr, http.StatusOK)
	assertJSON(t, rr.Body.Bytes(), `{
	  "deployment": "org",
	  "org_name": "Local Development",
	  "everyone_label": "Anyone in Local Development",
	  "provider_name": "SSO",
	  "development": true
	}`)
}

func TestFrontendFallbackServesEmbeddedIndex(t *testing.T) {
	rr := do(t, newTestEnv(t).h, http.MethodGet, "/docs/some_doc", "", nil)
	assertStatus(t, rr, http.StatusOK)
	if !strings.Contains(rr.Body.String(), "<title>aidocs</title>") || !strings.Contains(rr.Body.String(), "id=\"root\"") {
		t.Fatalf("body = %q, want embedded frontend", rr.Body.String())
	}
}

func TestHandlersPassRequestContextToRepository(t *testing.T) {
	wrapped := &contextCheckingRepo{Repository: newTestStore(t)}
	h := server.New(server.Config{Environment: "test"}, server.WithRepository(wrapped), server.WithAuthenticator(testPrincipalAuth{})).Handler()
	ctx := context.WithValue(context.Background(), contextMarkerKey{}, "seen")
	req := httptest.NewRequest(http.MethodGet, "/v1/documents", nil).WithContext(ctx)
	req.Header.Set("X-Test-Principal", principalHeader(fixtureOwner))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusOK)
	if !wrapped.sawMarker {
		t.Fatal("repository did not receive request context")
	}
}

type contextMarkerKey struct{}

type contextCheckingRepo struct {
	repo.Repository
	sawMarker bool
}

func (r *contextCheckingRepo) ListDocuments(ctx context.Context, p auth.Principal) ([]repo.Document, error) {
	if ctx.Value(contextMarkerKey{}) == "seen" {
		r.sawMarker = true
	}
	return r.Repository.ListDocuments(ctx, p)
}
