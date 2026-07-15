//go:build testfixtures

package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/razorpay/aidocs/api/internal/auth"
	"github.com/razorpay/aidocs/api/internal/googlegroups"
	"github.com/razorpay/aidocs/api/internal/repo"
	"github.com/razorpay/aidocs/api/internal/server"
)

type serverFakeGroupDirectory struct {
	mu          sync.Mutex
	search      []googlegroups.GoogleGroup
	lookup      googlegroups.GoogleGroup
	memberships map[string][]googlegroups.GoogleGroup
	err         error
}

func (d *serverFakeGroupDirectory) SearchGroups(context.Context, string, int) ([]googlegroups.GoogleGroup, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]googlegroups.GoogleGroup(nil), d.search...), d.err
}
func (d *serverFakeGroupDirectory) LookupGroup(context.Context, string) (googlegroups.GoogleGroup, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.lookup, d.err
}
func (d *serverFakeGroupDirectory) TransitiveGroupsForMember(_ context.Context, email string) ([]googlegroups.GoogleGroup, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]googlegroups.GoogleGroup(nil), d.memberships[email]...), d.err
}

func groupTestServer(r repo.Repository, directory googlegroups.GoogleGroupDirectory, ttl time.Duration) http.Handler {
	return server.New(server.Config{
		Environment: server.EnvProduction, Deployment: server.DeploymentOrg,
		GoogleGroupsEnabled: true, GoogleGroupsMembershipTTL: ttl,
	}, server.WithRepository(r), server.WithAuthenticator(testPrincipalAuth{}), server.WithGoogleGroupDirectory(directory)).Handler()
}

func TestGoogleGroupSearchAPI(t *testing.T) {
	group := googlegroups.GoogleGroup{ID: "groups/1", CustomerID: "C123", Email: "platform@example.com", Name: "Platform"}
	directory := &serverFakeGroupDirectory{search: []googlegroups.GoogleGroup{group}, memberships: map[string][]googlegroups.GoogleGroup{}}
	h := groupTestServer(repo.NewMemory(), directory, time.Minute)
	headers := map[string]string{"X-Test-Principal": "user:usr_1:owner@example.com:Owner"}

	rr := do(t, h, http.MethodGet, "/v1/google-groups?q=p", "", headers)
	assertStatus(t, rr, http.StatusBadRequest)
	rr = do(t, h, http.MethodGet, "/v1/google-groups?q=pl", "", headers)
	assertStatus(t, rr, http.StatusOK)
	assertJSON(t, rr.Body.Bytes(), `{"items":[{"id":"groups/1","email":"platform@example.com","name":"Platform"}]}`)

	directory.mu.Lock()
	directory.search = make([]googlegroups.GoogleGroup, 25)
	for i := range directory.search {
		directory.search[i] = group
	}
	directory.mu.Unlock()
	rr = do(t, h, http.MethodGet, "/v1/google-groups?q=platform", "", headers)
	assertStatus(t, rr, http.StatusOK)
	var capped struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &capped); err != nil || len(capped.Items) != 20 {
		t.Fatalf("search cap items=%d err=%v", len(capped.Items), err)
	}

	directory.mu.Lock()
	directory.err = errors.New("outage containing owner@example.com")
	directory.mu.Unlock()
	rr = do(t, h, http.MethodGet, "/v1/google-groups?q=platform", "", headers)
	assertStatus(t, rr, http.StatusServiceUnavailable)
	if got := rr.Body.String(); got == "" || contains(got, "owner@example.com") {
		t.Fatalf("response leaked directory error: %s", got)
	}

	disabled := server.New(server.Config{Environment: "test"}, server.WithRepository(repo.NewMemory()), server.WithAuthenticator(testPrincipalAuth{})).Handler()
	rr = do(t, disabled, http.MethodGet, "/v1/google-groups?q=platform", "", headers)
	assertStatus(t, rr, http.StatusNotFound)
}

func TestDocumentAndFolderGoogleGroupGrantUsesVerifiedImmutableID(t *testing.T) {
	r := repo.NewMemory()
	owner := auth.Principal{Type: auth.PrincipalUser, ID: "owner", Email: "owner@example.com", Name: "Owner"}
	doc, _, _ := r.CreateDocument(context.Background(), owner, "Doc", []byte("<html></html>"))
	folder, _ := r.CreateFolder(context.Background(), owner, "Folder", "")
	verified := googlegroups.GoogleGroup{ID: "groups/immutable", CustomerID: "C123", Email: "canonical@example.com", Name: "Platform"}
	directory := &serverFakeGroupDirectory{lookup: verified, memberships: map[string][]googlegroups.GoogleGroup{}}
	h := groupTestServer(r, directory, time.Minute)
	headers := map[string]string{"X-Test-Principal": "user:owner:owner@example.com:Owner", "Content-Type": "application/json"}
	body := `{"principal":{"type":"google_group","id":"groups/spoofed","email":"alias@example.com"},"role":"commenter"}`

	rr := do(t, h, http.MethodPost, "/v1/documents/"+doc.ID+"/grants", body, headers)
	assertStatus(t, rr, http.StatusCreated)
	assertJSONContains(t, rr.Body.Bytes(), map[string]any{"role": "commenter"})
	grants, _ := r.ListGrants(context.Background(), doc.ID)
	if len(grants) != 1 || grants[0].Principal.ID != "groups/immutable" || grants[0].Principal.Email != "canonical@example.com" {
		t.Fatalf("document grants = %+v", grants)
	}

	rr = do(t, h, http.MethodPost, "/v1/folders/"+folder.ID+"/grants", body, headers)
	assertStatus(t, rr, http.StatusCreated)
	grants, _ = r.ListGrants(context.Background(), folder.ID)
	if len(grants) != 1 || grants[0].Principal.ID != "groups/immutable" {
		t.Fatalf("folder grants = %+v", grants)
	}
	folderGrantID := grants[0].ID

	docGrants, _ := r.ListGrants(context.Background(), doc.ID)
	docGrantID := docGrants[0].ID
	rr = do(t, h, http.MethodPatch, "/v1/documents/"+doc.ID+"/grants/"+docGrantID, `{"role":"editor"}`, headers)
	assertStatus(t, rr, http.StatusOK)
	rr = do(t, h, http.MethodDelete, "/v1/documents/"+doc.ID+"/grants/"+docGrantID, "", headers)
	assertStatus(t, rr, http.StatusNoContent)
	rr = do(t, h, http.MethodPatch, "/v1/folders/"+folder.ID+"/grants/"+folderGrantID, `{"role":"viewer"}`, headers)
	assertStatus(t, rr, http.StatusOK)
	rr = do(t, h, http.MethodDelete, "/v1/folders/"+folder.ID+"/grants/"+folderGrantID, "", headers)
	assertStatus(t, rr, http.StatusNoContent)

	directory.mu.Lock()
	directory.err = googlegroups.ErrNotFound
	directory.mu.Unlock()
	rr = do(t, h, http.MethodPost, "/v1/documents/"+doc.ID+"/grants", body, headers)
	assertStatus(t, rr, http.StatusBadRequest)
	directory.mu.Lock()
	directory.err = errors.New("directory outage")
	directory.mu.Unlock()
	rr = do(t, h, http.MethodPost, "/v1/documents/"+doc.ID+"/grants", body, headers)
	assertStatus(t, rr, http.StatusServiceUnavailable)
}

func TestGroupMembershipAccessRefreshRemovalInheritanceAndFailure(t *testing.T) {
	r := repo.NewMemory()
	owner := auth.Principal{Type: auth.PrincipalUser, ID: "owner", Email: "owner@example.com", Name: "Owner"}
	member := auth.Principal{Type: auth.PrincipalUser, ID: "member", Email: "member@example.com", Name: "Member"}
	groupPrincipal := auth.Principal{Type: auth.PrincipalGoogleGroup, ID: "groups/1", Email: "group@example.com", Name: "Group"}
	group := googlegroups.GoogleGroup{ID: "groups/1", CustomerID: "C123", Email: "group@example.com", Name: "Group"}
	folder, _ := r.CreateFolder(context.Background(), owner, "Shared", "")
	doc, _, _ := r.CreateDocument(context.Background(), owner, "Inherited", []byte("<html></html>"))
	_, _ = r.MoveDocumentToFolder(context.Background(), owner, folder.ID, doc.ID)
	_, _ = r.CreateGrant(context.Background(), "folder", folder.ID, groupPrincipal, repo.RoleEditor, owner)
	directory := &serverFakeGroupDirectory{memberships: map[string][]googlegroups.GoogleGroup{member.Email: {group}}}
	h := groupTestServer(r, directory, time.Nanosecond)
	headers := map[string]string{"X-Test-Principal": "user:member:member@example.com:Member"}

	rr := do(t, h, http.MethodGet, "/v1/documents/"+doc.ID, "", headers)
	assertStatus(t, rr, http.StatusOK)
	rr = do(t, h, http.MethodGet, "/v1/documents?q=Inherited", "", headers)
	assertStatus(t, rr, http.StatusOK)

	directory.mu.Lock()
	directory.memberships[member.Email] = []googlegroups.GoogleGroup{}
	directory.mu.Unlock()
	time.Sleep(time.Microsecond)
	rr = do(t, h, http.MethodGet, "/v1/documents/"+doc.ID, "", headers)
	assertStatus(t, rr, http.StatusForbidden)

	directory.mu.Lock()
	directory.err = errors.New("google down")
	directory.mu.Unlock()
	time.Sleep(time.Microsecond)
	rr = do(t, h, http.MethodGet, "/v1/documents/"+doc.ID, "", headers)
	assertStatus(t, rr, http.StatusServiceUnavailable)
	assertJSONContains(t, rr.Body.Bytes(), map[string]any{"error": map[string]any{"code": "group_membership_unavailable"}})

	_, _ = r.CreateGrant(context.Background(), "document", doc.ID, member, repo.RoleViewer, owner)
	rr = do(t, h, http.MethodGet, "/v1/documents/"+doc.ID, "", headers)
	assertStatus(t, rr, http.StatusOK)
	rr = do(t, h, http.MethodGet, "/v1/documents", "", headers)
	assertStatus(t, rr, http.StatusServiceUnavailable)
}

func TestOwnerGoogleGroupGrantCanonicalOwnerInvariants(t *testing.T) {
	r := repo.NewMemory()
	owner := auth.Principal{Type: auth.PrincipalUser, ID: "owner", Email: "owner@example.com", Name: "Owner"}
	doc, _, _ := r.CreateDocument(context.Background(), owner, "Owner doc", []byte("<html></html>"))
	group := googlegroups.GoogleGroup{ID: "groups/owners", CustomerID: "C123", Email: "owners@example.com", Name: "Owners"}
	directory := &serverFakeGroupDirectory{
		lookup: group,
		memberships: map[string][]googlegroups.GoogleGroup{
			"coowner@example.com": {group},
		},
	}
	h := groupTestServer(r, directory, time.Minute)
	ownerHeaders := map[string]string{"X-Test-Principal": "user:owner:owner@example.com:Owner", "Content-Type": "application/json"}

	rr := do(t, h, http.MethodPost, "/v1/documents/"+doc.ID+"/grants", `{"principal":{"type":"google_group","email":"owners@example.com"},"role":"owner"}`, ownerHeaders)
	assertStatus(t, rr, http.StatusCreated)
	grants, _ := r.ListGrants(context.Background(), doc.ID)
	grantID := grants[0].ID
	storedDoc, _ := r.GetDocument(context.Background(), doc.ID)
	if storedDoc.Owner.ID != owner.ID {
		t.Fatalf("canonical owner changed: %+v", storedDoc.Owner)
	}

	coownerHeaders := map[string]string{"X-Test-Principal": "user:coowner:coowner@example.com:Coowner", "Content-Type": "application/json"}
	rr = do(t, h, http.MethodPatch, "/v1/documents/"+doc.ID, `{"title":"Changed by group owner"}`, coownerHeaders)
	assertStatus(t, rr, http.StatusOK)
	rr = do(t, h, http.MethodPatch, "/v1/documents/"+doc.ID+"/grants/"+grantID, `{"role":"editor"}`, coownerHeaders)
	assertStatus(t, rr, http.StatusForbidden)
	rr = do(t, h, http.MethodPost, "/v1/documents/"+doc.ID+"/grants", `{"principal":{"type":"google_group","email":"owners@example.com"},"role":"owner"}`, coownerHeaders)
	assertStatus(t, rr, http.StatusForbidden)
	rr = do(t, h, http.MethodDelete, "/v1/documents/"+doc.ID+"/grants/"+grantID, "", coownerHeaders)
	assertStatus(t, rr, http.StatusForbidden)

	rr = do(t, h, http.MethodPatch, "/v1/documents/"+doc.ID+"/grants/"+grantID, `{"role":"editor"}`, ownerHeaders)
	assertStatus(t, rr, http.StatusOK)
	rr = do(t, h, http.MethodPatch, "/v1/documents/"+doc.ID+"/grants/"+grantID, `{"role":"owner"}`, ownerHeaders)
	assertStatus(t, rr, http.StatusOK)

	rr = do(t, h, http.MethodPost, "/v1/documents/"+doc.ID+"/grants", `{"principal":{"type":"user","id":"someone"},"role":"owner"}`, ownerHeaders)
	assertStatus(t, rr, http.StatusBadRequest)

	folder, _ := r.CreateFolder(context.Background(), owner, "Owner folder", "")
	rr = do(t, h, http.MethodPost, "/v1/folders/"+folder.ID+"/grants", `{"principal":{"type":"google_group","email":"owners@example.com"},"role":"owner"}`, ownerHeaders)
	assertStatus(t, rr, http.StatusCreated)
	folderGrants, _ := r.ListGrants(context.Background(), folder.ID)
	folderGrantID := folderGrants[0].ID
	rr = do(t, h, http.MethodPatch, "/v1/folders/"+folder.ID+"/grants/"+folderGrantID, `{"role":"editor"}`, coownerHeaders)
	assertStatus(t, rr, http.StatusForbidden)
	rr = do(t, h, http.MethodDelete, "/v1/folders/"+folder.ID+"/grants/"+folderGrantID, "", coownerHeaders)
	assertStatus(t, rr, http.StatusForbidden)
	rr = do(t, h, http.MethodPatch, "/v1/folders/"+folder.ID+"/grants/"+folderGrantID, `{"role":"editor"}`, ownerHeaders)
	assertStatus(t, rr, http.StatusOK)
	rr = do(t, h, http.MethodPatch, "/v1/folders/"+folder.ID+"/grants/"+folderGrantID, `{"role":"owner"}`, ownerHeaders)
	assertStatus(t, rr, http.StatusOK)
	rr = do(t, h, http.MethodDelete, "/v1/folders/"+folder.ID, "", coownerHeaders)
	assertStatus(t, rr, http.StatusNoContent)
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

func assertJSONContains(t *testing.T, body []byte, subset map[string]any) {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("invalid JSON: %v: %s", err, body)
	}
	var check func(map[string]any, map[string]any) bool
	check = func(actual, expected map[string]any) bool {
		for key, want := range expected {
			have, ok := actual[key]
			if !ok {
				return false
			}
			if nested, ok := want.(map[string]any); ok {
				haveNested, ok := have.(map[string]any)
				if !ok || !check(haveNested, nested) {
					return false
				}
				continue
			}
			if !reflect.DeepEqual(have, want) {
				return false
			}
		}
		return true
	}
	if !check(got, subset) {
		t.Fatalf("JSON %v does not contain %v", got, subset)
	}
}
