//go:build testfixtures

package repo

import "github.com/razorpay/aidocs/api/internal/auth"

// NewMemorySeeded returns an in-memory repository preloaded with a fixture
// graph: an owner-owned document (doc_1 / ver_1), a service account (sa_1), a
// comment (cmt_1), a folder (fld_1), and roles for the editor_1/commenter_1/viewer_1 principals.
//
// Roles are seeded into fixtureRoles/fixturePrincipals rather than as real
// grants so the grantN sequence stays at gr_1 for the first created grant.
func NewMemorySeeded() *Memory {
	m := NewMemory()
	owner := auth.Principal{Type: auth.PrincipalUser, ID: "owner_1", Email: "owner@example.com", Name: "Owner"}
	m.users[owner.ID] = owner
	m.docs["doc_1"] = Document{ID: "doc_1", Title: "fixture", Owner: owner, CurrentVersionID: "ver_1"}
	m.docs["doc_2"] = Document{ID: "doc_2", Title: "fixture2", Owner: owner, CurrentVersionID: "ver_2"}
	m.versions["ver_1"] = Version{ID: "ver_1", Number: 1, DocumentID: "doc_1", CreatedBy: owner, SHA256: "sha_1"}
	m.versions["ver_2"] = Version{ID: "ver_2", Number: 1, DocumentID: "doc_2", CreatedBy: owner, SHA256: "sha_2"}
	m.sas["sa_1"] = ServiceAccount{ID: "sa_1", Name: "fixture", Owner: owner}
	m.comments["cmt_1"] = Comment{ID: "cmt_1", DocumentID: "doc_1", VersionID: "ver_1", Author: auth.Principal{Type: auth.PrincipalUser, ID: "commenter_1", Email: "commenter@example.com", Name: "Commenter"}, Body: "original", SelectedText: "fixture", Anchor: Anchor{Quote: "fixture"}, Status: StatusOpen, RootCommentID: "cmt_1"}
	m.folders["fld_1"] = Folder{ID: "fld_1", Title: "fixture folder", Owner: owner}
	m.folderN = 1
	m.fixtureRoles = map[string]Role{
		"owner_1":     RoleOwner,
		"editor_1":    RoleEditor,
		"commenter_1": RoleCommenter,
		"viewer_1":    RoleViewer,
	}
	m.fixturePrincipals = map[string]bool{
		"owner_1":     true,
		"editor_1":    true,
		"commenter_1": true,
		"viewer_1":    true,
	}
	return m
}
