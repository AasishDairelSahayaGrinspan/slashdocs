package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func TestGrantDocumentAccessToServiceAccount(t *testing.T) {
	e := newTestEnv(t)
	rr := do(t, e.h, http.MethodPost, "/v1/documents/"+e.doc.ID+"/grants", fmt.Sprintf(`{
	  "principal": { "type": "service_account", "id": %q, "name": "attacker.bot" },
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
	  "principal": { "type": "service_account", "id": %q, "name": "fixture" },
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

func TestGrantDocumentAccessCreatesPendingEmailInvite(t *testing.T) {
	e := newTestEnv(t)
	rr := do(t, e.h, http.MethodPost, "/v1/documents/"+e.doc.ID+"/grants", `{
	  "principal": { "type": "user", "email": " Reviewer@Nowhere.Example " },
	  "role": "commenter"
	}`, map[string]string{
		"Content-Type":     "application/json",
		"X-Test-Principal": principalHeader(fixtureOwner),
	})

	assertStatus(t, rr, http.StatusCreated)
	var created struct {
		Principal struct {
			ID    string `json:"id"`
			Email string `json:"email"`
		} `json:"principal"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if created.Principal.ID == "" || created.Principal.Email != "reviewer@nowhere.example" {
		t.Fatalf("pending principal = %#v", created.Principal)
	}
}

func TestGrantDocumentAccessCanonicalizesExistingUserByID(t *testing.T) {
	e := newTestEnv(t)
	rr := do(t, e.h, http.MethodPost, "/v1/documents/"+e.doc.ID+"/grants", `{
	  "principal": { "type": "user", "id": "owner_1", "email": "attacker@example.com", "name": "Attacker" },
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

func TestGrantDocumentAccessRejectsInvalidInlineUserEmail(t *testing.T) {
	for _, email := range []string{"   ", "missing-at", "robot@brave.otter.bot"} {
		t.Run(email, func(t *testing.T) {
			e := newTestEnv(t)
			rr := do(t, e.h, http.MethodPost, "/v1/documents/"+e.doc.ID+"/grants", fmt.Sprintf(`{
			  "principal": { "type": "user", "email": %q },
			  "role": "commenter"
			}`, email), map[string]string{
				"Content-Type":     "application/json",
				"X-Test-Principal": principalHeader(fixtureOwner),
			})
			assertStatus(t, rr, http.StatusBadRequest)
		})
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
