//go:build testfixtures

package server_test

import (
	"context"
	"fmt"
	"net/http"

	"github.com/razorpay/aidocs/api/internal/auth"
	"github.com/razorpay/aidocs/api/internal/repo"
	"github.com/razorpay/aidocs/api/internal/server"
)

// newTestServer is the compact in-memory fixture used by the repository and
// pull-request contract suite. The broader server suite uses newTestEnv and a
// real Postgres store; keeping this fixture build-tagged preserves deterministic
// doc_1/ver_1 identifiers without putting test seeding into production code.
func newTestServer() http.Handler {
	return newConfiguredTestServer(server.Config{Environment: "test"})
}

func newConfiguredTestServer(cfg server.Config) http.Handler {
	ctx := context.Background()
	store := repo.NewMemory()
	owner := auth.Principal{Type: auth.PrincipalUser, ID: "owner_1", Email: "owner@example.com", Name: "Owner"}
	principals := []auth.Principal{
		owner,
		{Type: auth.PrincipalUser, ID: "editor_1", Email: "editor@example.com", Name: "Editor"},
		{Type: auth.PrincipalUser, ID: "commenter_1", Email: "commenter@example.com", Name: "Commenter"},
		{Type: auth.PrincipalUser, ID: "viewer_1", Email: "viewer@example.com", Name: "Viewer"},
		{Type: auth.PrincipalUser, ID: "usr_1", Email: "anurag@example.com", Name: "Anurag"},
		{Type: auth.PrincipalUser, ID: "attacker_1", Email: "attacker@example.com", Name: "Attacker"},
	}
	for _, principal := range principals {
		if _, err := store.UpsertGoogleUser(ctx, principal.ID, principal.Email, principal.Name, "test-"+principal.ID, ""); err != nil {
			panic(fmt.Sprintf("seed test user %s: %v", principal.ID, err))
		}
	}
	document, version, err := store.CreateDocument(ctx, owner, "fixture", []byte("<html><body>fixture</body></html>"))
	if err != nil || document.ID != "doc_1" || version.ID != "ver_1" {
		panic(fmt.Sprintf("seed deterministic document: document=%s version=%s err=%v", document.ID, version.ID, err))
	}
	for index, role := range []repo.Role{repo.RoleEditor, repo.RoleCommenter, repo.RoleViewer} {
		if _, err := store.CreateGrant(ctx, "document", document.ID, principals[index+1], role, owner); err != nil {
			panic(fmt.Sprintf("seed %s document role: %v", role, err))
		}
	}
	if _, err := store.CreateServiceAccount(ctx, owner, "fixture"); err != nil {
		panic(fmt.Sprintf("seed service account: %v", err))
	}
	return server.New(cfg, server.WithRepository(store), server.WithAuthenticator(testPrincipalAuth{})).Handler()
}
