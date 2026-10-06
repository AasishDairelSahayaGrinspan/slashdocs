package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/razorpay/slashdocs/api/internal/auth"
)

func TestGrantDocumentAccessToServiceAccount(t *testing.T) {
	e := newTestEnv(t)
	rr := do(t, e.h, http.MethodPost, "/v1/documents/"+e.doc.ID+"/grants", fmt.Sprintf(`{
	  "principal": { "type": "service_account", "id": %q },
	  "role": "editor"
	}`, e.sa.ID), map[string]string{
		"Content-Type":     "application/json",
		"X-Test-Principal": principalHeader(fixtureOwner),
	})

	assertStatus(t, rr, http.StatusCreated)
	grantID := jsonField(t, rr.Body.Bytes(), "id")
	assertJSON(t, rr.Body.Bytes(), fmt.Sprintf(`{
	  "id": %q,
	  "resource": { "type": "document", "id": %q },
	  "principal": { "type": "service_account", "id": %q },
	  "role": "editor",
	  "granted_by": { "id": "owner_1", "email": "owner@example.com" }
	}`, grantID, e.doc.ID, e.sa.ID))
}

func TestGrantDocumentAccessToUserByEmail(t *testing.T) {
	e := newTestEnv(t)
	rr := do(t, e.h, http.MethodPost, "/v1/documents/"+e.doc.ID+"/grants", `{
	  "principal": { "type": "user", "email": "owner@example.com" },
	  "role": "commenter"
	}`, map[string]string{
		"Content-Type":     "application/json",
		"X-Test-Principal": principalHeader(fixtureOwner),
	})

	assertStatus(t, rr, http.StatusCreated)
	grantID := jsonField(t, rr.Body.Bytes(), "id")
	assertJSON(t, rr.Body.Bytes(), fmt.Sprintf(`{
	  "id": %q,
	  "resource": { "type": "document", "id": %q },
	  "principal": { "type": "user", "id": "owner_1", "email": "owner@example.com", "name": "Owner" },
	  "role": "commenter",
	  "granted_by": { "id": "owner_1", "email": "owner@example.com" }
	}`, grantID, e.doc.ID))
}

func TestGrantDocumentAccessRejectsUnknownUserEmail(t *testing.T) {
	e := newTestEnv(t)
	rr := do(t, e.h, http.MethodPost, "/v1/documents/"+e.doc.ID+"/grants", `{
	  "principal": { "type": "user", "email": "reviewer@nowhere.example" },
	  "role": "commenter"
	}`, map[string]string{
		"Content-Type":     "application/json",
		"X-Test-Principal": principalHeader(fixtureOwner),
	})

	assertStatus(t, rr, http.StatusBadRequest)
	if !strings.Contains(rr.Body.String(), "No slashdocs user exists") {
		t.Fatalf("response = %s", rr.Body.String())
	}
}

func TestPatchGrantRejectsOwnerRole(t *testing.T) {
	e := newTestEnv(t)
	rr := do(t, e.h, http.MethodPost, "/v1/documents/"+e.doc.ID+"/grants", fmt.Sprintf(`{
	  "principal": { "type": "service_account", "id": %q },
	  "role": "viewer"
	}`, e.sa.ID), map[string]string{
		"Content-Type":     "application/json",
		"X-Test-Principal": principalHeader(fixtureOwner),
	})
	assertStatus(t, rr, http.StatusCreated)
	grantID := jsonField(t, rr.Body.Bytes(), "id")

	rr = do(t, e.h, http.MethodPatch, "/v1/documents/"+e.doc.ID+"/grants/"+grantID, `{"role":"owner"}`, map[string]string{
		"Content-Type":     "application/json",
		"X-Test-Principal": principalHeader(fixtureOwner),
	})

	assertStatus(t, rr, http.StatusBadRequest)
}

func TestCommenterCanManageDocumentGrants(t *testing.T) {
	e := newTestEnv(t)
	commenter := map[string]string{
		"Content-Type":     "application/json",
		"X-Test-Principal": principalHeader(fixtureCommenter),
	}

	rr := do(t, e.h, http.MethodPost, "/v1/documents/"+e.doc.ID+"/grants", fmt.Sprintf(`{
	  "principal": { "type": "service_account", "id": %q },
	  "role": "viewer"
	}`, e.sa.ID), commenter)
	assertStatus(t, rr, http.StatusCreated)
	grantID := jsonField(t, rr.Body.Bytes(), "id")

	rr = do(t, e.h, http.MethodPatch, "/v1/documents/"+e.doc.ID+"/grants/"+grantID, `{"role":"editor"}`, commenter)
	assertStatus(t, rr, http.StatusOK)

	rr = do(t, e.h, http.MethodDelete, "/v1/documents/"+e.doc.ID+"/grants/"+grantID, "", commenter)
	assertStatus(t, rr, http.StatusNoContent)
}

// TestGrantReAddAfterRevokeReturnsPersistedID locks in that re-granting a
// previously revoked (resource, principal) pair returns the id of the
// resurrected row: the id handed back by create must round-trip through
// revoke, or every add->revoke->add sequence strands an unusable id.
func TestGrantReAddAfterRevokeReturnsPersistedID(t *testing.T) {
	e := newTestEnv(t)
	owner := map[string]string{
		"Content-Type":     "application/json",
		"X-Test-Principal": principalHeader(fixtureOwner),
	}
	body := fmt.Sprintf(`{
	  "principal": { "type": "service_account", "id": %q },
	  "role": "viewer"
	}`, e.sa.ID)

	rr := do(t, e.h, http.MethodPost, "/v1/documents/"+e.doc.ID+"/grants", body, owner)
	assertStatus(t, rr, http.StatusCreated)
	firstID := jsonField(t, rr.Body.Bytes(), "id")

	rr = do(t, e.h, http.MethodDelete, "/v1/documents/"+e.doc.ID+"/grants/"+firstID, "", owner)
	assertStatus(t, rr, http.StatusNoContent)

	rr = do(t, e.h, http.MethodPost, "/v1/documents/"+e.doc.ID+"/grants", body, owner)
	assertStatus(t, rr, http.StatusCreated)
	secondID := jsonField(t, rr.Body.Bytes(), "id")
	if secondID != firstID {
		t.Fatalf("re-add returned id %q, want resurrected grant %q", secondID, firstID)
	}

	rr = do(t, e.h, http.MethodDelete, "/v1/documents/"+e.doc.ID+"/grants/"+secondID, "", owner)
	assertStatus(t, rr, http.StatusNoContent)
}

func TestViewerCannotManageDocumentGrants(t *testing.T) {
	e := newTestEnv(t)
	rr := do(t, e.h, http.MethodPost, "/v1/documents/"+e.doc.ID+"/grants", fmt.Sprintf(`{
	  "principal": { "type": "service_account", "id": %q },
	  "role": "viewer"
	}`, e.sa.ID), map[string]string{
		"Content-Type":     "application/json",
		"X-Test-Principal": principalHeader(fixtureViewer),
	})

	assertStatus(t, rr, http.StatusForbidden)
}

func TestGrantRejectsNonexistentServiceAccount(t *testing.T) {
	e := newTestEnv(t)
	rr := do(t, e.h, http.MethodPost, "/v1/documents/"+e.doc.ID+"/grants", `{
	  "principal": { "type": "service_account", "id": "sa_missing" },
	  "role": "viewer"
	}`, map[string]string{
		"Content-Type":     "application/json",
		"X-Test-Principal": principalHeader(fixtureOwner),
	})

	assertStatus(t, rr, http.StatusNotFound)
}

func TestGrantPatchIsScopedToDocument(t *testing.T) {
	e := newTestEnv(t)
	rr := do(t, e.h, http.MethodPost, "/v1/documents/"+e.doc.ID+"/grants", fmt.Sprintf(`{
	  "principal": { "type": "service_account", "id": %q },
	  "role": "viewer"
	}`, e.sa.ID), map[string]string{
		"Content-Type":     "application/json",
		"X-Test-Principal": principalHeader(fixtureOwner),
	})
	assertStatus(t, rr, http.StatusCreated)
	grantID := jsonField(t, rr.Body.Bytes(), "id")

	otherDocID, _ := e.createDocument("Other", "<html>other</html>", fixtureOwner)
	rr = do(t, e.h, http.MethodPatch, "/v1/documents/"+otherDocID+"/grants/"+grantID, `{"role":"editor"}`, map[string]string{
		"Content-Type":     "application/json",
		"X-Test-Principal": principalHeader(fixtureOwner),
	})
	assertStatus(t, rr, http.StatusNotFound)
}

// accessCallerFields decodes the caller-describing fields of an /access
// response. The share sheet gates its controls on current_role
// (canManageSharing) and canonical_owner (owner-group changes), so both must
// be present and describe the CALLER, not the resource owner.
func accessCallerFields(t *testing.T, body []byte) (role string, canonical bool) {
	t.Helper()
	var out struct {
		CurrentRole    *string `json:"current_role"`
		CanonicalOwner *bool   `json:"canonical_owner"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("access response is not JSON: %v; body=%s", err, body)
	}
	if out.CurrentRole == nil || out.CanonicalOwner == nil {
		t.Fatalf("access response missing current_role/canonical_owner: %s", body)
	}
	return *out.CurrentRole, *out.CanonicalOwner
}

func TestDocumentAccessReportsCallerRole(t *testing.T) {
	e := newTestEnv(t)
	for _, tc := range []struct {
		principal auth.Principal
		wantRole  string
		wantOwner bool
	}{
		{fixtureOwner, "owner", true},
		{fixtureEditor, "editor", false},
		{fixtureCommenter, "commenter", false},
		{fixtureViewer, "viewer", false},
	} {
		rr := do(t, e.h, http.MethodGet, "/v1/documents/"+e.doc.ID+"/access", "", map[string]string{
			"X-Test-Principal": principalHeader(tc.principal),
		})
		assertStatus(t, rr, http.StatusOK)
		role, canonical := accessCallerFields(t, rr.Body.Bytes())
		if role != tc.wantRole || canonical != tc.wantOwner {
			t.Fatalf("%s: current_role=%q canonical_owner=%v, want %q/%v", tc.principal.Email, role, canonical, tc.wantRole, tc.wantOwner)
		}
	}
}

func TestFolderAccessReportsCallerRole(t *testing.T) {
	e := newTestEnv(t)
	owner := map[string]string{
		"Content-Type":     "application/json",
		"X-Test-Principal": principalHeader(fixtureOwner),
	}
	folderID := createFolder(t, e, `{"title":"Shared"}`, principalHeader(fixtureOwner))
	rr := do(t, e.h, http.MethodPost, "/v1/folders/"+folderID+"/grants", fmt.Sprintf(`{
	  "principal": { "type": "user", "email": %q },
	  "role": "editor"
	}`, fixtureEditor.Email), owner)
	assertStatus(t, rr, http.StatusCreated)

	rr = do(t, e.h, http.MethodGet, "/v1/folders/"+folderID+"/access", "", owner)
	assertStatus(t, rr, http.StatusOK)
	if role, canonical := accessCallerFields(t, rr.Body.Bytes()); role != "owner" || !canonical {
		t.Fatalf("owner: current_role=%q canonical_owner=%v, want owner/true", role, canonical)
	}

	rr = do(t, e.h, http.MethodGet, "/v1/folders/"+folderID+"/access", "", map[string]string{
		"X-Test-Principal": principalHeader(fixtureEditor),
	})
	assertStatus(t, rr, http.StatusOK)
	if role, canonical := accessCallerFields(t, rr.Body.Bytes()); role != "editor" || canonical {
		t.Fatalf("editor: current_role=%q canonical_owner=%v, want editor/false", role, canonical)
	}
}
