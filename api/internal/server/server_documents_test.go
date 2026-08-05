package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/razorpay/aidocs/api/internal/auth"
	"github.com/razorpay/aidocs/api/internal/repo"
	"github.com/razorpay/aidocs/api/internal/server"
)

// createDocumentWithFields uploads a document with extra multipart form
// fields and returns the raw response for default_permission assertions.
func createDocumentWithFields(t *testing.T, e *testEnv, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	if _, ok := fields["title"]; !ok {
		fields["title"] = "Default sharing"
	}
	body, contentType := multipartBody(t, fields, "file", "doc.html", "<html><body>x</body></html>")
	return doRaw(t, e.h, http.MethodPost, "/v1/documents", body, map[string]string{
		"Content-Type":     contentType,
		"X-Test-Principal": principalHeader(fixtureUser),
	})
}

// decodeDefaultGrant parses a 201 create response into id plus the optional
// default_grant payload (nil when the document stayed private).
func decodeDefaultGrant(t *testing.T, body []byte) (docID string, grant *struct {
	GrantID   string `json:"grant_id"`
	Principal struct {
		Type string `json:"type"`
	} `json:"principal"`
	Role string `json:"role"`
}) {
	t.Helper()
	var out struct {
		ID           string `json:"id"`
		DefaultGrant *struct {
			GrantID   string `json:"grant_id"`
			Principal struct {
				Type string `json:"type"`
			} `json:"principal"`
			Role string `json:"role"`
		} `json:"default_grant"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("bad create document response: %v", err)
	}
	return out.ID, out.DefaultGrant
}

func TestCreateDocument(t *testing.T) {
	e := newTestEnv(t)
	body, contentType := multipartBody(t, map[string]string{
		"title": "Q3 business review",
	}, "file", "review.html", "<html><body>Hello</body></html>")

	rr := doRaw(t, e.h, http.MethodPost, "/v1/documents", body, map[string]string{
		"Content-Type":     contentType,
		"X-Test-Principal": principalHeader(fixtureUser),
	})

	assertStatus(t, rr, http.StatusCreated)
	docID := jsonField(t, rr.Body.Bytes(), "id")
	verID := jsonField(t, rr.Body.Bytes(), "current_version_id")
	if !strings.HasPrefix(docID, "doc_") || !strings.HasPrefix(verID, "ver_") {
		t.Fatalf("created ids = %q / %q, want doc_/ver_ prefixes", docID, verID)
	}

	got := do(t, e.h, http.MethodGet, "/v1/documents/"+docID, "", map[string]string{
		"X-Test-Principal": principalHeader(fixtureUser),
	})
	assertStatus(t, got, http.StatusOK)
	if jsonField(t, got.Body.Bytes(), "title") != "Q3 business review" {
		t.Fatalf("document round-trip lost title: %s", got.Body.String())
	}
	if jsonField(t, got.Body.Bytes(), "current_version_id") != verID {
		t.Fatalf("document round-trip lost version: %s", got.Body.String())
	}
}

func TestServiceAccountCanCreateDocument(t *testing.T) {
	e := newTestEnv(t)
	body, contentType := multipartBody(t, map[string]string{"title": "Bot doc"}, "file", "bot.html", "<html></html>")
	rr := doRaw(t, e.h, http.MethodPost, "/v1/documents", body, map[string]string{
		"Content-Type":     contentType,
		"X-Test-Principal": "service_account:" + e.sa.ID,
	})

	assertStatus(t, rr, http.StatusCreated)
	docID := jsonField(t, rr.Body.Bytes(), "id")
	verID := jsonField(t, rr.Body.Bytes(), "current_version_id")
	if !strings.HasPrefix(docID, "doc_") || !strings.HasPrefix(verID, "ver_") {
		t.Fatalf("created ids = %q / %q, want doc_/ver_ prefixes", docID, verID)
	}
}

func TestUploadRejectsPayloadOver10MiB(t *testing.T) {
	largeHTML := "<html>" + strings.Repeat("x", 10*1024*1024+1) + "</html>"
	body, contentType := multipartBody(t, map[string]string{"title": "Too large"}, "file", "large.html", largeHTML)

	rr := doRaw(t, newTestEnv(t).h, http.MethodPost, "/v1/documents", body, map[string]string{
		"Content-Type":     contentType,
		"X-Test-Principal": principalHeader(fixtureUser),
	})

	assertStatus(t, rr, http.StatusRequestEntityTooLarge)
}

func TestAnyoneGrantGivesStrangerAccess(t *testing.T) {
	e := newTestEnv(t)
	stranger := map[string]string{"X-Test-Principal": principalHeader(fixtureStranger)}

	// Before any "anyone" grant, a stranger cannot see the document.
	rr := do(t, e.h, http.MethodGet, "/v1/documents/"+e.doc.ID, "", stranger)
	assertStatus(t, rr, http.StatusForbidden)

	// Owner grants the whole-server audience commenter access.
	rr = do(t, e.h, http.MethodPost, "/v1/documents/"+e.doc.ID+"/grants", `{
	  "principal": { "type": "anyone" },
	  "role": "commenter"
	}`, map[string]string{
		"Content-Type":     "application/json",
		"X-Test-Principal": principalHeader(fixtureOwner),
	})
	assertStatus(t, rr, http.StatusCreated)

	// Now the stranger can view the document and create a comment (commenter).
	rr = do(t, e.h, http.MethodGet, "/v1/documents/"+e.doc.ID, "", stranger)
	assertStatus(t, rr, http.StatusOK)
}

func TestServiceAccountWithoutGrantCannotReadDocument(t *testing.T) {
	e := newTestEnv(t)
	rr := do(t, e.h, http.MethodGet, "/v1/documents/"+e.doc.ID, "", map[string]string{
		"X-Test-Principal": "service_account:" + e.sa.ID,
	})

	assertStatus(t, rr, http.StatusForbidden)
}

// A server-wide default applies the "anyone" grant at create and reports it
// back with the persisted grant id.
func TestCreateDocumentAppliesServerDefaultPermission(t *testing.T) {
	e := newConfiguredTestEnv(t, server.Config{Environment: "test", Deployment: "org", OrgName: "Acme", DefaultDocPermission: repo.RoleViewer})
	rr := createDocumentWithFields(t, e, map[string]string{})
	assertStatus(t, rr, http.StatusCreated)
	docID, grant := decodeDefaultGrant(t, rr.Body.Bytes())
	if grant == nil {
		t.Fatalf("create response missing default_grant: %s", rr.Body.String())
		return
	}
	if !strings.HasPrefix(grant.GrantID, "gr_") || grant.Principal.Type != "anyone" || grant.Role != "viewer" {
		t.Fatalf("default_grant = %+v, want gr_ id / anyone / viewer", grant)
	}

	// The reported grant id is the persisted one.
	access := do(t, e.h, http.MethodGet, "/v1/documents/"+docID+"/access", "", map[string]string{
		"X-Test-Principal": principalHeader(fixtureUser),
	})
	assertStatus(t, access, http.StatusOK)
	if !strings.Contains(access.Body.String(), grant.GrantID) {
		t.Fatalf("listed grants do not include default grant %s: %s", grant.GrantID, access.Body.String())
	}
}

// The request field overrides the server default in both directions: a
// private-default server honors an editor override, and a sharing-default
// server honors a private override by applying no grant.
func TestCreateDocumentRequestOverridesServerDefault(t *testing.T) {
	private := newTestEnv(t)
	rr := createDocumentWithFields(t, private, map[string]string{"default_permission": "editor"})
	assertStatus(t, rr, http.StatusCreated)
	if _, grant := decodeDefaultGrant(t, rr.Body.Bytes()); grant == nil || grant.Role != "editor" {
		t.Fatalf("editor override on a private server got %+v, want editor grant", grant)
	}

	sharing := newConfiguredTestEnv(t, server.Config{Environment: "test", Deployment: "org", OrgName: "Acme", DefaultDocPermission: repo.RoleViewer})
	rr = createDocumentWithFields(t, sharing, map[string]string{"default_permission": "private"})
	assertStatus(t, rr, http.StatusCreated)
	docID, grant := decodeDefaultGrant(t, rr.Body.Bytes())
	if grant != nil {
		t.Fatalf("private override on a sharing server still granted %+v", grant)
	}
	if strings.Contains(rr.Body.String(), "default_grant") {
		t.Fatalf("private create response should omit default_grant: %s", rr.Body.String())
	}
	access := do(t, sharing.h, http.MethodGet, "/v1/documents/"+docID+"/access", "", map[string]string{
		"X-Test-Principal": principalHeader(fixtureUser),
	})
	assertStatus(t, access, http.StatusOK)
	if strings.Contains(access.Body.String(), `"anyone"`) {
		t.Fatalf("private override still created an anyone grant: %s", access.Body.String())
	}
}

// Only the server-wide env is org-gated; an explicit request override is
// honored even on a public deployment.
func TestCreateDocumentOverrideHonoredOnPublicDeployment(t *testing.T) {
	e := newConfiguredTestEnv(t, server.Config{Environment: "test", Deployment: "public"})
	rr := createDocumentWithFields(t, e, map[string]string{"default_permission": "viewer"})
	assertStatus(t, rr, http.StatusCreated)
	if _, grant := decodeDefaultGrant(t, rr.Body.Bytes()); grant == nil || grant.Role != "viewer" {
		t.Fatalf("viewer override on a public server got %+v, want viewer grant", grant)
	}
}

func TestCreateDocumentRejectsInvalidDefaultPermission(t *testing.T) {
	rr := createDocumentWithFields(t, newTestEnv(t), map[string]string{"default_permission": "everyone"})
	assertStatus(t, rr, http.StatusBadRequest)
	if !strings.Contains(rr.Body.String(), "default_permission must be viewer, commenter, editor or private") {
		t.Fatalf("400 body missing vocabulary message: %s", rr.Body.String())
	}
}

// failingGrantRepo makes CreateGrant fail so the create handler's partial
// state (document exists, promised default grant absent) is observable.
type failingGrantRepo struct {
	repo.Repository
}

func (failingGrantRepo) CreateGrant(context.Context, string, string, auth.Principal, repo.Role, auth.Principal) (repo.Grant, error) {
	return repo.Grant{}, errors.New("forced grant failure")
}

// TestCreateDocumentGrantFailureErrors locks in the spec's failure contract:
// when the default grant cannot be applied, the request errors instead of
// returning a document that is silently less shared than promised.
func TestCreateDocumentGrantFailureErrors(t *testing.T) {
	e := newConfiguredTestEnv(t,
		server.Config{Environment: "test", Deployment: "org", DefaultDocPermission: repo.RoleViewer},
	)
	e.h = server.New(
		server.Config{Environment: "test", Deployment: "org", DefaultDocPermission: repo.RoleViewer},
		server.WithRepository(failingGrantRepo{e.store}),
		server.WithAuthenticator(testPrincipalAuth{}),
	).Handler()
	rr := createDocumentWithFields(t, e, map[string]string{})
	assertStatus(t, rr, http.StatusInternalServerError)
	if strings.Contains(rr.Body.String(), "default_grant") {
		t.Fatalf("failed grant must not report a default_grant: %s", rr.Body.String())
	}
}
