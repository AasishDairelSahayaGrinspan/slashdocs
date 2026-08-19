package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/razorpay/aidocs/api/internal/repo"
)

func TestCreateComment(t *testing.T) {
	e := newTestEnv(t)
	rr := do(t, e.h, http.MethodPost, "/v1/documents/"+e.doc.ID+"/comments", fmt.Sprintf(`{
	  "version_id": %q,
	  "body": "Add a number here.",
	  "anchor": {
	    "quote": "fixture",
	    "prefix": "",
	    "suffix": "",
	    "dom_path": "html/body",
	    "start_offset": 0,
	    "end_offset": 7
	  }
	}`, e.ver.ID), map[string]string{
		"Content-Type":     "application/json",
		"X-Test-Principal": principalHeader(fixtureCommenter),
	})

	assertStatus(t, rr, http.StatusCreated)
	cmtID := jsonField(t, rr.Body.Bytes(), "id")
	assertJSON(t, rr.Body.Bytes(), fmt.Sprintf(`{
	  "id": %q,
		  "author": { "type": "user", "id": "commenter_1", "email": "commenter@example.com", "name": "Commenter" },
		  "body": "Add a number here.",
		  "mentions": [],
		  "selected_text": "fixture",
	  "anchor": {
	    "quote": "fixture",
	    "prefix": "",
	    "suffix": "",
	    "dom_path": "html/body",
	    "start_offset": 0,
	    "end_offset": 7
	  },
	  "status": "open",
	  "created_on_version_id": %q,
	  "current_placement": {
	    "version_id": %q,
	    "status": "attached",
	    "anchor": {
	      "quote": "fixture",
	      "prefix": "",
	      "suffix": "",
	      "dom_path": "html/body",
	      "start_offset": 0,
	      "end_offset": 7
	    },
	    "matched_text": "fixture",
	    "confidence": 1
	  }
	}`, cmtID, e.ver.ID, e.ver.ID))
}

func TestCreateCommentReplyAndListThread(t *testing.T) {
	e := newTestEnv(t)
	rr := do(t, e.h, http.MethodPost, "/v1/documents/"+e.doc.ID+"/comments/"+e.cmt.ID+"/replies", `{"body":"reply body"}`, map[string]string{
		"Content-Type":     "application/json",
		"X-Test-Principal": principalHeader(fixtureCommenter),
	})
	assertStatus(t, rr, http.StatusCreated)
	replyID := jsonField(t, rr.Body.Bytes(), "id")
	assertJSON(t, rr.Body.Bytes(), fmt.Sprintf(`{
	  "id": %q,
		  "author": { "type": "user", "id": "commenter_1", "email": "commenter@example.com", "name": "Commenter" },
		  "body": "reply body",
		  "mentions": [],
		  "status": "open",
	  "created_on_version_id": %q,
	  "parent_comment_id": %q
	}`, replyID, e.ver.ID, e.cmt.ID))

	rr = do(t, e.h, http.MethodGet, "/v1/documents/"+e.doc.ID+"/comments", "", map[string]string{
		"X-Test-Principal": principalHeader(fixtureCommenter),
	})
	assertStatus(t, rr, http.StatusOK)
	assertJSON(t, rr.Body.Bytes(), fmt.Sprintf(`{
	  "items": [{
	    "id": %q,
		    "author": { "type": "user", "id": "commenter_1", "email": "commenter@example.com", "name": "Commenter" },
		    "body": "original",
		    "mentions": [],
		    "selected_text": "fixture",
	    "anchor": { "quote": "fixture", "prefix": "", "suffix": "", "dom_path": "", "start_offset": 0, "end_offset": 0 },
	    "status": "open",
	    "created_on_version_id": %q,
	    "current_placement": { "version_id": %q, "status": "attached", "anchor": { "quote": "fixture", "prefix": "", "suffix": "", "dom_path": "", "start_offset": 0, "end_offset": 0 }, "matched_text": "fixture", "confidence": 1 },
	    "replies": [{
	      "id": %q,
		      "author": { "type": "user", "id": "commenter_1", "email": "commenter@example.com", "name": "Commenter" },
		      "body": "reply body",
		      "mentions": [],
		      "status": "open",
	      "created_on_version_id": %q,
	      "parent_comment_id": %q
	    }]
	  }]
	}`, e.cmt.ID, e.ver.ID, e.ver.ID, replyID, e.ver.ID, e.cmt.ID))
}

func TestServiceAccountCreatesAvatarComment(t *testing.T) {
	e := newTestEnv(t)
	sa := serviceAccountPrincipal(e)
	e.grantDocumentRole(sa, repo.RoleCommenter)

	rr := do(t, e.h, http.MethodPost, "/v1/documents/"+e.doc.ID+"/comments", fmt.Sprintf(`{
	  "version_id": %q,
	  "body": "Consider tightening this claim.",
	  "anchor": { "quote": "fixture" },
	  "author_display": "Priya's avatar",
	  "severity": "should-fix"
	}`, e.ver.ID), map[string]string{
		"Content-Type":     "application/json",
		"X-Test-Principal": principalHeader(sa),
	})

	assertStatus(t, rr, http.StatusCreated)
	cmtID := jsonField(t, rr.Body.Bytes(), "id")
	if got := jsonField(t, rr.Body.Bytes(), "author_display"); got != "Priya's avatar" {
		t.Fatalf("author_display = %q, want %q", got, "Priya's avatar")
	}
	if got := jsonField(t, rr.Body.Bytes(), "severity"); got != "should-fix" {
		t.Fatalf("severity = %q, want %q", got, "should-fix")
	}

	rr = do(t, e.h, http.MethodGet, "/v1/documents/"+e.doc.ID+"/comments", "", map[string]string{
		"X-Test-Principal": principalHeader(fixtureViewer),
	})
	assertStatus(t, rr, http.StatusOK)
	var listed struct {
		Items []struct {
			ID            string `json:"id"`
			AuthorDisplay string `json:"author_display"`
			Severity      string `json:"severity"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &listed); err != nil {
		t.Fatalf("bad list response: %v", err)
	}
	found := false
	for _, item := range listed.Items {
		if item.ID != cmtID {
			continue
		}
		found = true
		if item.AuthorDisplay != "Priya's avatar" || item.Severity != "should-fix" {
			t.Fatalf("listed avatar comment = %+v, want author_display and severity preserved", item)
		}
	}
	if !found {
		t.Fatalf("avatar comment %s missing from list", cmtID)
	}
}

func TestHumanCannotSetAvatarCommentFields(t *testing.T) {
	e := newTestEnv(t)
	for _, body := range []string{
		fmt.Sprintf(`{"version_id": %q, "body": "x", "anchor": {"quote": "fixture"}, "author_display": "Priya's avatar"}`, e.ver.ID),
		fmt.Sprintf(`{"version_id": %q, "body": "x", "anchor": {"quote": "fixture"}, "severity": "blocking"}`, e.ver.ID),
	} {
		rr := do(t, e.h, http.MethodPost, "/v1/documents/"+e.doc.ID+"/comments", body, map[string]string{
			"Content-Type":     "application/json",
			"X-Test-Principal": principalHeader(fixtureCommenter),
		})
		assertStatus(t, rr, http.StatusForbidden)
	}
}

func TestCreateCommentRejectsInvalidSeverity(t *testing.T) {
	e := newTestEnv(t)
	sa := serviceAccountPrincipal(e)
	e.grantDocumentRole(sa, repo.RoleCommenter)

	rr := do(t, e.h, http.MethodPost, "/v1/documents/"+e.doc.ID+"/comments", fmt.Sprintf(`{
	  "version_id": %q,
	  "body": "x",
	  "anchor": { "quote": "fixture" },
	  "severity": "catastrophic"
	}`, e.ver.ID), map[string]string{
		"Content-Type":     "application/json",
		"X-Test-Principal": principalHeader(sa),
	})

	assertStatus(t, rr, http.StatusBadRequest)
}

func TestViewerCannotCreateComment(t *testing.T) {
	e := newTestEnv(t)
	rr := do(t, e.h, http.MethodPost, "/v1/documents/"+e.doc.ID+"/comments", fmt.Sprintf(`{
	  "version_id": %q,
	  "body": "should not be allowed",
	  "anchor": { "quote": "hello" }
	}`, e.ver.ID), map[string]string{
		"Content-Type":     "application/json",
		"X-Test-Principal": principalHeader(fixtureViewer),
	})

	assertStatus(t, rr, http.StatusForbidden)
}

func TestCreateCommentRejectsVersionFromAnotherDocument(t *testing.T) {
	e := newTestEnv(t)
	_, otherVerID := e.createDocument("Other", "<html>other</html>", fixtureOwner)

	rr := do(t, e.h, http.MethodPost, "/v1/documents/"+e.doc.ID+"/comments", fmt.Sprintf(`{
	  "version_id": %q,
	  "body": "wrong doc",
	  "anchor": { "quote": "hello" }
	}`, otherVerID), map[string]string{
		"Content-Type":     "application/json",
		"X-Test-Principal": principalHeader(fixtureCommenter),
	})

	assertStatus(t, rr, http.StatusNotFound)
}

func TestRandomUserCannotPatchAnotherUsersComment(t *testing.T) {
	e := newTestEnv(t)
	rr := do(t, e.h, http.MethodPatch, "/v1/documents/"+e.doc.ID+"/comments/"+e.cmt.ID, `{"body":"hacked"}`, map[string]string{
		"Content-Type":     "application/json",
		"X-Test-Principal": principalHeader(fixtureAttacker),
	})

	assertStatus(t, rr, http.StatusForbidden)
}

func TestRandomUserCannotDeleteAnotherUsersComment(t *testing.T) {
	e := newTestEnv(t)
	rr := do(t, e.h, http.MethodDelete, "/v1/documents/"+e.doc.ID+"/comments/"+e.cmt.ID, "", map[string]string{
		"X-Test-Principal": principalHeader(fixtureAttacker),
	})

	assertStatus(t, rr, http.StatusForbidden)
}

func TestEditorCannotDeleteAnotherUsersComment(t *testing.T) {
	e := newTestEnv(t)
	rr := do(t, e.h, http.MethodDelete, "/v1/documents/"+e.doc.ID+"/comments/"+e.cmt.ID, "", map[string]string{
		"X-Test-Principal": principalHeader(fixtureEditor),
	})

	assertStatus(t, rr, http.StatusForbidden)
}

func TestPatchCommentRejectsWrongDocument(t *testing.T) {
	e := newTestEnv(t)
	rr := do(t, e.h, http.MethodPatch, "/v1/documents/doc_wrong/comments/"+e.cmt.ID, `{"status":"resolved"}`, map[string]string{
		"Content-Type":     "application/json",
		"X-Test-Principal": principalHeader(fixtureCommenter),
	})

	assertStatus(t, rr, http.StatusNotFound)
}

func TestPatchCommentRejectsInvalidStatus(t *testing.T) {
	e := newTestEnv(t)
	rr := do(t, e.h, http.MethodPatch, "/v1/documents/"+e.doc.ID+"/comments/"+e.cmt.ID, `{"status":"closed"}`, map[string]string{
		"Content-Type":     "application/json",
		"X-Test-Principal": principalHeader(fixtureCommenter),
	})

	assertStatus(t, rr, http.StatusBadRequest)
}
