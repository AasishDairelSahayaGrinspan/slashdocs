package server_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/razorpay/aidocs/api/internal/auth"
	"github.com/razorpay/aidocs/api/internal/repo"
)

// commentWrite is one request against a comment write endpoint. The table
// covers every comment mutation the API exposes, so a rule proved for one verb
// is proved for all of them rather than for whichever one the test picked.
type commentWrite struct {
	name        string
	method      string
	path        string
	body        string
	contentType string
}

// commentWrites lists the write verbs reachable for an existing comment, with
// delete last so the earlier requests still address a live row.
func commentWrites(e *testEnv, commentID string) []commentWrite {
	base := "/v1/documents/" + e.doc.ID + "/comments/" + commentID
	return []commentWrite{
		{"patch body", http.MethodPatch, base, `{"body":"edited without access"}`, "application/json"},
		{"patch resolved", http.MethodPatch, base, `{"status":"resolved"}`, "application/json"},
		{"patch reopened", http.MethodPatch, base, `{"status":"open"}`, "application/json"},
		{"reply", http.MethodPost, base + "/replies", `{"body":"reply without access"}`, "application/json"},
		{"react", http.MethodPut, base + "/reactions/+1", "", ""},
		{"unreact", http.MethodDelete, base + "/reactions/+1", "", ""},
		{"delete", http.MethodDelete, base, "", ""},
	}
}

func (w commentWrite) headers(principal string) map[string]string {
	headers := map[string]string{"X-Test-Principal": principal}
	if w.contentType != "" {
		headers["Content-Type"] = w.contentType
	}
	return headers
}

// documentGrantID returns the id of the principal's grant on the fixture
// document.
func documentGrantID(t *testing.T, e *testEnv, p auth.Principal) string {
	t.Helper()
	grants, err := e.store.ListGrants(context.Background(), e.doc.ID)
	if err != nil {
		t.Fatalf("list grants: %v", err)
	}
	for _, g := range grants {
		if g.Principal.Type == p.Type && g.Principal.ID == p.ID {
			return g.ID
		}
	}
	t.Fatalf("no grant for %s on %s", p.ID, e.doc.ID)
	return ""
}

// revokeDocumentGrant takes the principal's access away and leaves the
// comments they already wrote in place, which is the state this file is about.
func revokeDocumentGrant(t *testing.T, e *testEnv, p auth.Principal) {
	t.Helper()
	if err := e.store.DeleteGrant(context.Background(), e.doc.ID, documentGrantID(t, e, p), fixtureOwner); err != nil {
		t.Fatalf("revoke grant for %s: %v", p.ID, err)
	}
}

func setDocumentGrantRole(t *testing.T, e *testEnv, p auth.Principal, role repo.Role) {
	t.Helper()
	if _, err := e.store.UpdateGrant(context.Background(), e.doc.ID, documentGrantID(t, e, p), role, fixtureOwner); err != nil {
		t.Fatalf("set %s role for %s: %v", role, p.ID, err)
	}
}

// assertCommentUnchanged reloads the comment and fails if any refused write
// nevertheless reached the database.
func assertCommentUnchanged(t *testing.T, e *testEnv, want repo.Comment) {
	t.Helper()
	got, err := e.store.GetComment(context.Background(), want.ID)
	if err != nil {
		t.Fatalf("reload comment %s: %v", want.ID, err)
	}
	if got.Body != want.Body || got.Status != want.Status || len(got.Mentions) != len(want.Mentions) || len(got.Reactions) != len(want.Reactions) {
		t.Fatalf("comment changed: got %#v, want body %q status %q with %d mentions and %d reactions",
			got, want.Body, want.Status, len(want.Mentions), len(want.Reactions))
	}
}

func queuedCommentNotifications(t *testing.T, e *testEnv, commentID string) int {
	t.Helper()
	var count int
	if err := e.store.Pool().QueryRow(context.Background(),
		"SELECT count(*) FROM comment_notification_outbox WHERE comment_id = $1", commentID).Scan(&count); err != nil {
		t.Fatalf("count queued notifications: %v", err)
	}
	return count
}

// Authorship records who wrote a comment; it is not a standing permission.
// Once the author's access is gone or demoted below commenter, every write
// verb must refuse, whether the grant was revoked outright or downgraded.
func TestCommentMutationRequiresCurrentDocumentAccess(t *testing.T) {
	for _, tc := range []struct {
		name      string
		loseWrite func(*testing.T, *testEnv)
	}{
		{"revoked grant", func(t *testing.T, e *testEnv) { revokeDocumentGrant(t, e, fixtureCommenter) }},
		{"downgraded to viewer", func(t *testing.T, e *testEnv) { setDocumentGrantRole(t, e, fixtureCommenter, repo.RoleViewer) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newTestEnv(t)
			tc.loseWrite(t, e)
			for _, write := range commentWrites(e, e.cmt.ID) {
				t.Run(write.name, func(t *testing.T) {
					rr := do(t, e.h, write.method, write.path, write.body, write.headers(principalHeader(fixtureCommenter)))
					assertStatus(t, rr, http.StatusForbidden)
				})
			}
			assertCommentUnchanged(t, e, e.cmt)
			if got := commentCount(t, e); got != 1 {
				t.Fatalf("comments on the document = %d, want only the fixture comment", got)
			}
		})
	}
}

// When Google Group membership cannot be refreshed the caller's effective role
// is unknown, because a group grant could still carry access. Every comment
// write says so instead of returning a 403 it cannot stand behind -- and an
// author is no exception, since their own access is exactly what is in doubt.
func TestCommentMutationReportsUnresolvedGroupMembership(t *testing.T) {
	e := newTestEnv(t)
	revokeDocumentGrant(t, e, fixtureCommenter)
	unresolved := principalHeader(fixtureCommenter) + ":" + auth.GoogleGroupMembershipUnavailable

	for _, write := range commentWrites(e, e.cmt.ID) {
		t.Run(write.name, func(t *testing.T) {
			rr := do(t, e.h, write.method, write.path, write.body, write.headers(unresolved))
			assertStatus(t, rr, http.StatusServiceUnavailable)
		})
	}
	assertCommentUnchanged(t, e, e.cmt)
}

// A mention edit is the sharpest form of the bug: it delivers attacker-chosen
// text to someone else's Slack DM. The refusal must happen before the
// notification is queued, not after.
func TestRevokedAuthorMentionEditQueuesNoNotification(t *testing.T) {
	e := newTestEnv(t)
	e.store.SetNotificationsEnabled(true)
	revokeDocumentGrant(t, e, fixtureCommenter)

	body := "Ping @owner@example.com"
	payload := map[string]any{
		"body": body,
		"mentions": []map[string]any{
			mentionRequest(t, body, "@owner@example.com", map[string]any{"type": "user", "id": fixtureOwner.ID}),
		},
	}
	rr := do(t, e.h, http.MethodPatch, "/v1/documents/"+e.doc.ID+"/comments/"+e.cmt.ID, jsonString(t, payload), map[string]string{
		"Content-Type":     commentMentionsContentType,
		"X-Test-Principal": principalHeader(fixtureCommenter),
	})

	assertStatus(t, rr, http.StatusForbidden)
	if got := queuedCommentNotifications(t, e, e.cmt.ID); got != 0 {
		t.Fatalf("queued comment notifications = %d, want none", got)
	}
	assertCommentUnchanged(t, e, e.cmt)
}

// Without current access the answer must not depend on the comment: an
// existing comment, one that lives in another document, and an id that never
// existed all get the generic refusal the other document routes give, so the
// endpoints cannot be used to probe for comment ids.
func TestCommentWritesWithoutAccessHideWhetherCommentExists(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	other, otherVersion, err := e.store.CreateDocument(ctx, fixtureOwner, "other", []byte("<html><body>other</body></html>"))
	if err != nil {
		t.Fatalf("seed other document: %v", err)
	}
	elsewhere, err := e.store.CreateComment(ctx, other.ID, otherVersion.ID, "elsewhere", repo.Anchor{Quote: "other"}, fixtureOwner, nil)
	if err != nil {
		t.Fatalf("seed other comment: %v", err)
	}

	var first, firstName string
	for _, tc := range []struct {
		name      string
		commentID string
	}{
		{"comment in this document", e.cmt.ID},
		{"comment in another document", elsewhere.ID},
		{"comment that does not exist", "cmt_missing"},
	} {
		responses := make([]string, 0, len(commentWrites(e, tc.commentID)))
		for _, write := range commentWrites(e, tc.commentID) {
			rr := do(t, e.h, write.method, write.path, write.body, write.headers(principalHeader(fixtureAttacker)))
			assertStatus(t, rr, http.StatusForbidden)
			responses = append(responses, write.name+" -> "+rr.Body.String())
		}
		joined := strings.Join(responses, "\n")
		if firstName == "" {
			first, firstName = joined, tc.name
			continue
		}
		if joined != first {
			t.Fatalf("responses differ between %q and %q:\n%s\n%s", firstName, tc.name, first, joined)
		}
	}
}

// The mirror image of the rule: a commenter who still holds the grant keeps
// full control of the comments they wrote.
func TestCommenterCanStillMutateOwnComment(t *testing.T) {
	e := newTestEnv(t)
	commenter := map[string]string{
		"Content-Type":     "application/json",
		"X-Test-Principal": principalHeader(fixtureCommenter),
	}
	path := "/v1/documents/" + e.doc.ID + "/comments/" + e.cmt.ID

	rr := do(t, e.h, http.MethodPatch, path, `{"body":"edited by the author"}`, commenter)
	assertStatus(t, rr, http.StatusOK)
	rr = do(t, e.h, http.MethodPatch, path, `{"status":"resolved"}`, commenter)
	assertStatus(t, rr, http.StatusOK)
	rr = do(t, e.h, http.MethodPost, path+"/replies", `{"body":"still allowed"}`, commenter)
	assertStatus(t, rr, http.StatusCreated)
	rr = do(t, e.h, http.MethodPut, path+"/reactions/+1", "", commenter)
	assertStatus(t, rr, http.StatusNoContent)
	rr = do(t, e.h, http.MethodDelete, path, "", commenter)
	assertStatus(t, rr, http.StatusNoContent)
}

// Editors moderate the thread but may not erase what someone else wrote;
// TestEditorCannotDeleteAnotherUsersComment covers the delete half.
func TestEditorCanPatchAnotherUsersComment(t *testing.T) {
	e := newTestEnv(t)
	rr := do(t, e.h, http.MethodPatch, "/v1/documents/"+e.doc.ID+"/comments/"+e.cmt.ID, `{"status":"resolved"}`, map[string]string{
		"Content-Type":     "application/json",
		"X-Test-Principal": principalHeader(fixtureEditor),
	})

	assertStatus(t, rr, http.StatusOK)
}

// The MCP comment tools replay the same handlers, so the rule must hold for an
// agent holding a revoked user's token just as it does over HTTP.
func TestMCPCommentToolsRequireCurrentDocumentAccess(t *testing.T) {
	e := newMCPOAuthTestEnv(t)
	ctx := context.Background()
	document, version, err := e.store.CreateDocument(ctx, fixtureOwner, "MCP revoked access", []byte("<html><body>mcp revoked</body></html>"))
	if err != nil {
		t.Fatalf("seed document: %v", err)
	}
	grant, err := e.store.CreateGrant(ctx, "document", document.ID, fixtureUser, repo.RoleCommenter, fixtureOwner)
	if err != nil {
		t.Fatalf("seed grant: %v", err)
	}
	comment, err := e.store.CreateComment(ctx, document.ID, version.ID, "original", repo.Anchor{Quote: "mcp revoked"}, fixtureUser, nil)
	if err != nil {
		t.Fatalf("seed comment: %v", err)
	}
	if err := e.store.DeleteGrant(ctx, document.ID, grant.ID, fixtureOwner); err != nil {
		t.Fatalf("revoke grant: %v", err)
	}

	token := auth.NewMCPAccessToken(fixtureUser.ID, e.resource, oauthTestSigningMaterial, time.Minute)
	identifiers := map[string]any{"document_id": document.ID, "comment_id": comment.ID}
	for _, tc := range []struct {
		tool      string
		arguments map[string]any
	}{
		{"update_comment", map[string]any{"document_id": document.ID, "comment_id": comment.ID, "body": "edited without access"}},
		{"resolve_comment", identifiers},
		{"reopen_comment", identifiers},
		{"reply_comment", map[string]any{"document_id": document.ID, "comment_id": comment.ID, "body": "reply without access"}},
		{"react_comment", map[string]any{"document_id": document.ID, "comment_id": comment.ID, "emoji": "+1"}},
		{"unreact_comment", map[string]any{"document_id": document.ID, "comment_id": comment.ID, "emoji": "+1"}},
		{"delete_comment", identifiers},
	} {
		t.Run(tc.tool, func(t *testing.T) {
			assertMCPToolError(t, callMCPToolHTTP(t, e, token, tc.tool, tc.arguments), "commenter role required")
		})
	}

	got, err := e.store.GetComment(ctx, comment.ID)
	if err != nil {
		t.Fatalf("reload comment: %v", err)
	}
	if got.Body != comment.Body || got.Status != comment.Status || len(got.Reactions) != 0 {
		t.Fatalf("comment changed over MCP: %#v", got)
	}
}
