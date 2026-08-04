package server_test

import (
	"net/http"
	"strings"
	"testing"
)

func TestCreateDocument(t *testing.T) {
	e := newTestEnv(t)
	body, contentType := multipartBody(t, map[string]string{
		"title": "Q3 business review",
	}, "file", "review.html", "<html><body>Hello</body></html>")

	rr := doRaw(t, e.h, http.MethodPost, "/v1/documents", body, map[string]string{
		"Content-Type":     contentType,
		"X-Test-Principal": principalHeader(fixtureUser),
	})

	assertStatus(t, rr, http.StatusCreated)
	docID := jsonField(t, rr.Body.Bytes(), "id")
	verID := jsonField(t, rr.Body.Bytes(), "current_version_id")
	if !strings.HasPrefix(docID, "doc_") || !strings.HasPrefix(verID, "ver_") {
		t.Fatalf("created ids = %q / %q, want doc_/ver_ prefixes", docID, verID)
	}

	got := do(t, e.h, http.MethodGet, "/v1/documents/"+docID, "", map[string]string{
		"X-Test-Principal": principalHeader(fixtureUser),
	})
	assertStatus(t, got, http.StatusOK)
	if jsonField(t, got.Body.Bytes(), "title") != "Q3 business review" {
		t.Fatalf("document round-trip lost title: %s", got.Body.String())
	}
	if jsonField(t, got.Body.Bytes(), "current_version_id") != verID {
		t.Fatalf("document round-trip lost version: %s", got.Body.String())
	}
}

func TestServiceAccountCanCreateDocument(t *testing.T) {
	e := newTestEnv(t)
	body, contentType := multipartBody(t, map[string]string{"title": "Bot doc"}, "file", "bot.html", "<html></html>")
	rr := doRaw(t, e.h, http.MethodPost, "/v1/documents", body, map[string]string{
		"Content-Type":     contentType,
		"X-Test-Principal": "service_account:" + e.sa.ID,
	})

	assertStatus(t, rr, http.StatusCreated)
	docID := jsonField(t, rr.Body.Bytes(), "id")
	verID := jsonField(t, rr.Body.Bytes(), "current_version_id")
	if !strings.HasPrefix(docID, "doc_") || !strings.HasPrefix(verID, "ver_") {
		t.Fatalf("created ids = %q / %q, want doc_/ver_ prefixes", docID, verID)
	}
}

func TestUploadRejectsPayloadOver10MiB(t *testing.T) {
	largeHTML := "<html>" + strings.Repeat("x", 10*1024*1024+1) + "</html>"
	body, contentType := multipartBody(t, map[string]string{"title": "Too large"}, "file", "large.html", largeHTML)

	rr := doRaw(t, newTestEnv(t).h, http.MethodPost, "/v1/documents", body, map[string]string{
		"Content-Type":     contentType,
		"X-Test-Principal": principalHeader(fixtureUser),
	})

	assertStatus(t, rr, http.StatusRequestEntityTooLarge)
}

func TestAnyoneGrantGivesStrangerAccess(t *testing.T) {
	e := newTestEnv(t)
	stranger := map[string]string{"X-Test-Principal": principalHeader(fixtureStranger)}

	// Before any "anyone" grant, a stranger cannot see the document.
	rr := do(t, e.h, http.MethodGet, "/v1/documents/"+e.doc.ID, "", stranger)
	assertStatus(t, rr, http.StatusForbidden)

	// Owner grants the whole-server audience commenter access.
	rr = do(t, e.h, http.MethodPost, "/v1/documents/"+e.doc.ID+"/grants", `{
	  "principal": { "type": "anyone" },
	  "role": "commenter"
	}`, map[string]string{
		"Content-Type":     "application/json",
		"X-Test-Principal": principalHeader(fixtureOwner),
	})
	assertStatus(t, rr, http.StatusCreated)

	// Now the stranger can view the document and create a comment (commenter).
	rr = do(t, e.h, http.MethodGet, "/v1/documents/"+e.doc.ID, "", stranger)
	assertStatus(t, rr, http.StatusOK)
}

func TestServiceAccountWithoutGrantCannotReadDocument(t *testing.T) {
	e := newTestEnv(t)
	rr := do(t, e.h, http.MethodGet, "/v1/documents/"+e.doc.ID, "", map[string]string{
		"X-Test-Principal": "service_account:" + e.sa.ID,
	})

	assertStatus(t, rr, http.StatusForbidden)
}
