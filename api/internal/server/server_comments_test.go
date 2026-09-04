package server_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

func assertRFC3339Timestamp(t *testing.T, value string) {
	t.Helper()
	if _, err := time.Parse(time.RFC3339Nano, value); err != nil {
		t.Fatalf("timestamp %q is not RFC3339: %v", value, err)
	}
}

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
	createdAt := jsonField(t, rr.Body.Bytes(), "created_at")
	assertRFC3339Timestamp(t, createdAt)
	assertJSON(t, rr.Body.Bytes(), fmt.Sprintf(`{
	  "id": %q,
		  "author": { "type": "user", "id": "commenter_1", "email": "commenter@example.com", "name": "Commenter" },
		  "body": "Add a number here.",
		  "mentions": [],
		  "reactions": [],
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
	  "created_at": %q,
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
	}`, cmtID, createdAt, e.ver.ID, e.ver.ID))
}

func TestCreateCommentReplyAndListThread(t *testing.T) {
	e := newTestEnv(t)
	rr := do(t, e.h, http.MethodPost, "/v1/documents/"+e.doc.ID+"/comments/"+e.cmt.ID+"/replies", `{"body":"reply body"}`, map[string]string{
		"Content-Type":     "application/json",
		"X-Test-Principal": principalHeader(fixtureCommenter),
	})
	assertStatus(t, rr, http.StatusCreated)
	replyID := jsonField(t, rr.Body.Bytes(), "id")
	replyCreatedAt := jsonField(t, rr.Body.Bytes(), "created_at")
	assertRFC3339Timestamp(t, replyCreatedAt)
	assertJSON(t, rr.Body.Bytes(), fmt.Sprintf(`{
	  "id": %q,
		  "author": { "type": "user", "id": "commenter_1", "email": "commenter@example.com", "name": "Commenter" },
		  "body": "reply body",
		  "mentions": [],
		  "reactions": [],
		  "status": "open",
	  "created_at": %q,
	  "created_on_version_id": %q,
	  "parent_comment_id": %q
	}`, replyID, replyCreatedAt, e.ver.ID, e.cmt.ID))

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
		    "reactions": [],
		    "selected_text": "fixture",
	    "anchor": { "quote": "fixture", "prefix": "", "suffix": "", "dom_path": "", "start_offset": 0, "end_offset": 0 },
	    "status": "open",
	    "created_at": %q,
	    "created_on_version_id": %q,
	    "current_placement": { "version_id": %q, "status": "attached", "anchor": { "quote": "fixture", "prefix": "", "suffix": "", "dom_path": "", "start_offset": 0, "end_offset": 0 }, "matched_text": "fixture", "confidence": 1 },
	    "replies": [{
	      "id": %q,
		      "author": { "type": "user", "id": "commenter_1", "email": "commenter@example.com", "name": "Commenter" },
		      "body": "reply body",
		      "mentions": [],
		      "reactions": [],
		      "status": "open",
	      "created_at": %q,
	      "created_on_version_id": %q,
	      "parent_comment_id": %q
	    }]
	  }]
	}`, e.cmt.ID, e.cmt.CreatedAt, e.ver.ID, e.ver.ID, replyID, replyCreatedAt, e.ver.ID, e.cmt.ID))
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
