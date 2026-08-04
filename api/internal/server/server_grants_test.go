package server_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
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
	if !strings.Contains(rr.Body.String(), "No aidocs user exists") {
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
