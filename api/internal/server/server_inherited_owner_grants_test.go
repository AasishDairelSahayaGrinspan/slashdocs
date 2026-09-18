package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/razorpay/aidocs/api/internal/auth"
	"github.com/razorpay/aidocs/api/internal/googlegroups"
	"github.com/razorpay/aidocs/api/internal/repo"
	"github.com/razorpay/aidocs/api/internal/server"
)

const inheritedOwnerGroupEmail = "platform@example.com"

// newInheritedOwnerGroupEnv builds a group-sharing environment whose directory
// reports the given users as members of one group.
func newInheritedOwnerGroupEnv(t *testing.T, members ...auth.Principal) *testEnv {
	t.Helper()
	group := googlegroups.GoogleGroup{ID: "groups/platform", CustomerID: "C123", Email: inheritedOwnerGroupEmail, Name: "Platform"}
	memberships := map[string][]googlegroups.GoogleGroup{}
	for _, m := range members {
		memberships[m.Email] = []googlegroups.GoogleGroup{group}
	}
	return newConfiguredTestEnv(t, server.Config{
		Environment:               "test",
		Deployment:                server.DeploymentOrg,
		GoogleGroupsEnabled:       true,
		GoogleGroupsMembershipTTL: time.Minute,
	}, server.WithGoogleGroupDirectory(&postgresGroupDirectory{group: group, memberships: memberships}))
}

// grantOwnerGroup shares a resource with the fixture group at owner level; only
// the resource's canonical owner may do this.
func grantOwnerGroup(t *testing.T, e *testEnv, resourcePath, principal string) {
	t.Helper()
	rr := do(t, e.h, http.MethodPost, resourcePath+"/grants",
		`{"principal":{"type":"google_group","email":"`+inheritedOwnerGroupEmail+`"},"role":"owner"}`,
		map[string]string{"X-Test-Principal": principal, "Content-Type": "application/json"})
	assertStatus(t, rr, http.StatusCreated)
}

func moveDocToFolder(t *testing.T, e *testEnv, folderID, docID, principal string) {
	t.Helper()
	rr := do(t, e.h, http.MethodPost, "/v1/folders/"+folderID+"/documents", `{"document_id":"`+docID+`"}`,
		map[string]string{"X-Test-Principal": principal, "Content-Type": "application/json"})
	assertStatus(t, rr, http.StatusOK)
}

func reparentFolder(t *testing.T, e *testEnv, folderID, parentID, principal string) {
	t.Helper()
	rr := do(t, e.h, http.MethodPatch, "/v1/folders/"+folderID, `{"parent_folder_id":"`+parentID+`"}`,
		map[string]string{"X-Test-Principal": principal, "Content-Type": "application/json"})
	assertStatus(t, rr, http.StatusOK)
}

// accessRoleFor returns the effective role the access report gives a principal
// on a resource.
func accessRoleFor(t *testing.T, e *testEnv, resourcePath, viewer, principalID string) string {
	t.Helper()
	rr := do(t, e.h, http.MethodGet, resourcePath+"/access", "", map[string]string{"X-Test-Principal": viewer})
	assertStatus(t, rr, http.StatusOK)
	var response struct {
		Items []struct {
			Principal     struct{ ID string } `json:"principal"`
			EffectiveRole string              `json:"effective_role"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode access report: %v", err)
	}
	for _, item := range response.Items {
		if item.Principal.ID == principalID {
			return item.EffectiveRole
		}
	}
	t.Fatalf("no access entry for %q: %s", principalID, rr.Body.String())
	return ""
}

// An owner-level Google Group grant on a folder must not hand group members
// owner rights over documents and folders someone else owns, whatever order the
// grant and the move happen in.
func TestInheritedOwnerGroupGrantDoesNotConferOwnerOverREST(t *testing.T) {
	victim := principalHeader(fixtureOwner)
	mover := principalHeader(fixtureEditor)
	member := principalHeader(fixtureUser)

	t.Run("grant then move", func(t *testing.T) {
		e := newInheritedOwnerGroupEnv(t, fixtureEditor, fixtureUser)
		folderID := createFolder(t, e, `{"title":"MoverFolder"}`, mover)
		grantOwnerGroup(t, e, "/v1/folders/"+folderID, mover)
		moveDocToFolder(t, e, folderID, e.doc.ID, mover)

		rr := do(t, e.h, http.MethodDelete, "/v1/documents/"+e.doc.ID, "", map[string]string{"X-Test-Principal": mover})
		assertStatus(t, rr, http.StatusForbidden)
		rr = do(t, e.h, http.MethodDelete, "/v1/documents/"+e.doc.ID, "", map[string]string{"X-Test-Principal": member})
		assertStatus(t, rr, http.StatusForbidden)
		rr = do(t, e.h, http.MethodPatch, "/v1/documents/"+e.doc.ID, `{"title":"stolen"}`,
			map[string]string{"X-Test-Principal": mover, "Content-Type": "application/json"})
		assertStatus(t, rr, http.StatusForbidden)

		// Owner-level group grants on the document stay with its canonical owner.
		rr = do(t, e.h, http.MethodPost, "/v1/documents/"+e.doc.ID+"/grants",
			`{"principal":{"type":"google_group","email":"`+inheritedOwnerGroupEmail+`"},"role":"owner"}`,
			map[string]string{"X-Test-Principal": mover, "Content-Type": "application/json"})
		assertStatus(t, rr, http.StatusForbidden)

		if role := accessRoleFor(t, e, "/v1/documents/"+e.doc.ID, victim, "groups/platform"); role != string(repo.RoleEditor) {
			t.Fatalf("access report role for the group = %q, want editor", role)
		}
	})

	t.Run("move then grant", func(t *testing.T) {
		e := newInheritedOwnerGroupEnv(t, fixtureEditor, fixtureUser)
		folderID := createFolder(t, e, `{"title":"MoverFolder"}`, mover)
		moveDocToFolder(t, e, folderID, e.doc.ID, mover)
		grantOwnerGroup(t, e, "/v1/folders/"+folderID, mover)

		rr := do(t, e.h, http.MethodDelete, "/v1/documents/"+e.doc.ID, "", map[string]string{"X-Test-Principal": mover})
		assertStatus(t, rr, http.StatusForbidden)
		rr = do(t, e.h, http.MethodDelete, "/v1/documents/"+e.doc.ID, "", map[string]string{"X-Test-Principal": member})
		assertStatus(t, rr, http.StatusForbidden)
	})

	t.Run("re-parenting a folder holding other people's resources", func(t *testing.T) {
		e := newInheritedOwnerGroupEnv(t, fixtureEditor, fixtureUser)
		folderID := createFolder(t, e, `{"title":"MoverFolder"}`, mover)
		grantOwnerGroup(t, e, "/v1/folders/"+folderID, mover)

		// The mover's own subfolder collects the victim's document and the
		// victim's subfolder before being re-parented under the shared folder.
		subfolderID := createFolder(t, e, `{"title":"Staging"}`, mover)
		moveDocToFolder(t, e, subfolderID, e.doc.ID, mover)
		rr := do(t, e.h, http.MethodPost, "/v1/folders/"+subfolderID+"/grants",
			`{"address":"`+fixtureOwner.Email+`","role":"editor"}`,
			map[string]string{"X-Test-Principal": mover, "Content-Type": "application/json"})
		assertStatus(t, rr, http.StatusCreated)
		victimFolderID := createFolder(t, e, `{"title":"VictimFolder"}`, victim)
		reparentFolder(t, e, victimFolderID, subfolderID, victim)
		reparentFolder(t, e, subfolderID, folderID, mover)

		rr = do(t, e.h, http.MethodDelete, "/v1/documents/"+e.doc.ID, "", map[string]string{"X-Test-Principal": member})
		assertStatus(t, rr, http.StatusForbidden)
		rr = do(t, e.h, http.MethodDelete, "/v1/folders/"+victimFolderID, "", map[string]string{"X-Test-Principal": member})
		assertStatus(t, rr, http.StatusForbidden)
		if role := accessRoleFor(t, e, "/v1/folders/"+victimFolderID, victim, "groups/platform"); role != string(repo.RoleEditor) {
			t.Fatalf("access report role for the group on the victim folder = %q, want editor", role)
		}
	})
}

// Capping inherited owner grants must leave every legitimate owner-level path
// intact.
func TestInheritedOwnerGroupGrantStillAppliesToTheGranterOwnResources(t *testing.T) {
	victim := principalHeader(fixtureOwner)
	mover := principalHeader(fixtureEditor)
	member := principalHeader(fixtureUser)

	t.Run("documents owned by the folder owner", func(t *testing.T) {
		e := newInheritedOwnerGroupEnv(t, fixtureEditor, fixtureUser)
		folderID := createFolder(t, e, `{"title":"MoverFolder"}`, mover)
		grantOwnerGroup(t, e, "/v1/folders/"+folderID, mover)
		moverDocID, _ := e.createDocument("Mover doc", "<p>mine</p>", fixtureEditor)
		moveDocToFolder(t, e, folderID, moverDocID, mover)

		rr := do(t, e.h, http.MethodDelete, "/v1/documents/"+moverDocID, "", map[string]string{"X-Test-Principal": member})
		assertStatus(t, rr, http.StatusNoContent)
	})

	t.Run("a nested folder keeps its own owner's group grant", func(t *testing.T) {
		e := newInheritedOwnerGroupEnv(t, fixtureEditor, fixtureUser)
		folderID := createFolder(t, e, `{"title":"MoverFolder"}`, mover)
		grantOwnerGroup(t, e, "/v1/folders/"+folderID, mover)
		rr := do(t, e.h, http.MethodPost, "/v1/folders/"+folderID+"/grants",
			`{"address":"`+fixtureOwner.Email+`","role":"editor"}`,
			map[string]string{"X-Test-Principal": mover, "Content-Type": "application/json"})
		assertStatus(t, rr, http.StatusCreated)

		victimFolderID := createFolder(t, e, `{"title":"VictimFolder"}`, victim)
		grantOwnerGroup(t, e, "/v1/folders/"+victimFolderID, victim)
		moveDocToFolder(t, e, victimFolderID, e.doc.ID, victim)
		reparentFolder(t, e, victimFolderID, folderID, victim)

		rr = do(t, e.h, http.MethodDelete, "/v1/documents/"+e.doc.ID, "", map[string]string{"X-Test-Principal": member})
		assertStatus(t, rr, http.StatusNoContent)
	})

	t.Run("a direct owner group grant on the document", func(t *testing.T) {
		e := newInheritedOwnerGroupEnv(t, fixtureEditor, fixtureUser)
		grantOwnerGroup(t, e, "/v1/documents/"+e.doc.ID, victim)

		rr := do(t, e.h, http.MethodDelete, "/v1/documents/"+e.doc.ID, "", map[string]string{"X-Test-Principal": member})
		assertStatus(t, rr, http.StatusNoContent)
	})

	t.Run("inherited viewer access is unchanged", func(t *testing.T) {
		e := newInheritedOwnerGroupEnv(t, fixtureEditor, fixtureUser)
		folderID := createFolder(t, e, `{"title":"MoverFolder"}`, mover)
		rr := do(t, e.h, http.MethodPost, "/v1/folders/"+folderID+"/grants",
			`{"principal":{"type":"google_group","email":"`+inheritedOwnerGroupEmail+`"},"role":"viewer"}`,
			map[string]string{"X-Test-Principal": mover, "Content-Type": "application/json"})
		assertStatus(t, rr, http.StatusCreated)
		moveDocToFolder(t, e, folderID, e.doc.ID, mover)

		rr = do(t, e.h, http.MethodGet, "/v1/documents/"+e.doc.ID, "", map[string]string{"X-Test-Principal": member})
		assertStatus(t, rr, http.StatusOK)
		rr = do(t, e.h, http.MethodDelete, "/v1/documents/"+e.doc.ID, "", map[string]string{"X-Test-Principal": member})
		assertStatus(t, rr, http.StatusForbidden)
	})
}

// MCP shares the REST handlers, so the same cap must decide an MCP mutation.
// Google Group membership is not resolved on the MCP transport, so this uses an
// owner-level grant stored directly for the calling user.
func TestMCPInheritedOwnerGrantDoesNotConferOwner(t *testing.T) {
	ctx := context.Background()
	e := newMCPOAuthTestEnv(t)
	caller := fixtureUser

	folder, err := e.store.CreateFolder(ctx, caller, "MCP inherited owner", "")
	if err != nil {
		t.Fatalf("create folder: %v", err)
	}
	victimDoc, _, err := e.store.CreateDocument(ctx, fixtureOwner, "MCP victim", []byte("<html><body>victim</body></html>"))
	if err != nil {
		t.Fatalf("create victim document: %v", err)
	}
	callerDoc, _, err := e.store.CreateDocument(ctx, caller, "MCP own", []byte("<html><body>own</body></html>"))
	if err != nil {
		t.Fatalf("create caller document: %v", err)
	}
	if _, err := e.store.CreateGrant(ctx, "folder", folder.ID, caller, repo.RoleOwner, caller); err != nil {
		t.Fatalf("create inherited owner grant: %v", err)
	}
	for _, documentID := range []string{victimDoc.ID, callerDoc.ID} {
		if _, err := e.store.MoveDocumentToFolder(ctx, caller, folder.ID, documentID); err != nil {
			t.Fatalf("move document %s: %v", documentID, err)
		}
	}

	token := auth.NewMCPAccessToken(caller.ID, e.resource, oauthTestSigningMaterial, time.Minute)
	assertMCPToolError(t, callMCPToolHTTP(t, e, token, "delete_document", map[string]any{"document_id": victimDoc.ID}), "owner role required")
	assertMCPToolOK(t, callMCPToolHTTP(t, e, token, "delete_document", map[string]any{"document_id": callerDoc.ID}), `"ok":true`)
}
