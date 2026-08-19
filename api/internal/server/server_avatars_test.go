package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/razorpay/aidocs/api/internal/repo"
	"github.com/razorpay/aidocs/api/internal/server"
)

const avatarFixtureHTML = `<html><body><h1>Rate limiter proposal</h1>
<p>Requests beyond the burst budget are shed with a retriable status code.</p>
<p>The limiter state lives in a shared cache with a five second sliding window.</p>
<p>Every rejected request emits a metric tagged with the calling service.</p>
</body></html>`

func newAvatarTestEnv(t *testing.T) *testEnv {
	return newConfiguredTestEnv(t, server.Config{Environment: "test"},
		server.WithAvatarMock(time.Millisecond, 5*time.Millisecond))
}

type avatarReviewStatusResponse struct {
	RequestID string `json:"request_id"`
	AidocID   string `json:"aidoc_id"`
	Reviews   []struct {
		AvatarID     string `json:"avatar_id"`
		DisplayName  string `json:"display_name"`
		State        string `json:"state"`
		CommentCount int    `json:"comment_count"`
	} `json:"reviews"`
}

func decodeAvatarStatus(t *testing.T, body []byte) avatarReviewStatusResponse {
	t.Helper()
	var out avatarReviewStatusResponse
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("bad review status response: %v; body=%s", err, body)
	}
	return out
}

func waitAvatarReviewTerminal(t *testing.T, e *testEnv, requestID string) avatarReviewStatusResponse {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		rr := do(t, e.h, http.MethodGet, "/v1/avatar-reviews/"+requestID, "", map[string]string{
			"X-Test-Principal": principalHeader(fixtureCommenter),
		})
		assertStatus(t, rr, http.StatusOK)
		status := decodeAvatarStatus(t, rr.Body.Bytes())
		terminal := true
		for _, review := range status.Reviews {
			if review.State == "queued" || review.State == "running" {
				terminal = false
			}
		}
		if terminal {
			return status
		}
		time.Sleep(3 * time.Millisecond)
	}
	t.Fatal("avatar review never reached a terminal state")
	return avatarReviewStatusResponse{}
}

func TestAvatarEndpointsAbsentWhenDisabled(t *testing.T) {
	e := newTestEnv(t)
	// Unknown GET paths fall through to the SPA index when the frontend is
	// built, so assert the API never answers with avatar JSON.
	rr := do(t, e.h, http.MethodGet, "/v1/avatars?document_id="+e.doc.ID, "", map[string]string{
		"X-Test-Principal": principalHeader(fixtureCommenter),
	})
	if rr.Code == http.StatusOK && strings.Contains(rr.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("disabled server answered the avatar picker: %s", rr.Body.String())
	}

	rr = do(t, e.h, http.MethodPost, "/v1/avatar-reviews", `{"document_id":"x","version_id":"y","avatar_ids":["av_priya"]}`, map[string]string{
		"Content-Type":     "application/json",
		"X-Test-Principal": principalHeader(fixtureCommenter),
	})
	assertStatus(t, rr, http.StatusNotFound)

	rr = do(t, e.h, http.MethodGet, "/v1/config", "", nil)
	assertStatus(t, rr, http.StatusOK)
	if strings.Contains(rr.Body.String(), "avatar_reviews_enabled\":true") {
		t.Fatalf("config advertises avatar reviews while disabled: %s", rr.Body.String())
	}
}

func TestAvatarConfigAdvertisesFeature(t *testing.T) {
	e := newAvatarTestEnv(t)
	rr := do(t, e.h, http.MethodGet, "/v1/config", "", nil)
	assertStatus(t, rr, http.StatusOK)
	var cfg struct {
		AvatarReviewsEnabled bool `json:"avatar_reviews_enabled"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("bad config response: %v", err)
	}
	if !cfg.AvatarReviewsEnabled {
		t.Fatalf("avatar_reviews_enabled missing: %s", rr.Body.String())
	}
}

func TestAvatarPickerRolesAndShape(t *testing.T) {
	e := newAvatarTestEnv(t)

	rr := do(t, e.h, http.MethodGet, "/v1/avatars?document_id="+e.doc.ID, "", map[string]string{
		"X-Test-Principal": principalHeader(fixtureViewer),
	})
	assertStatus(t, rr, http.StatusForbidden)

	rr = do(t, e.h, http.MethodGet, "/v1/avatars?document_id="+e.doc.ID, "", map[string]string{
		"X-Test-Principal": principalHeader(fixtureCommenter),
	})
	assertStatus(t, rr, http.StatusOK)
	var list struct {
		Items []struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
			ComingSoon  bool   `json:"coming_soon"`
		} `json:"items"`
		MaxSelectable int `json:"max_selectable"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &list); err != nil {
		t.Fatalf("bad avatars response: %v", err)
	}
	if list.MaxSelectable != 1 || len(list.Items) < 4 {
		t.Fatalf("unexpected picker payload: %s", rr.Body.String())
	}
	if last := list.Items[len(list.Items)-1]; !last.ComingSoon || last.DisplayName != "The Bench" {
		t.Fatalf("bench card missing or not last: %+v", last)
	}
}

func TestAvatarReviewEndToEnd(t *testing.T) {
	e := newAvatarTestEnv(t)
	docID, verID := e.createDocument("Avatar target", avatarFixtureHTML, fixtureOwner)
	e.grantDocumentRoleOn(docID, fixtureCommenter, repo.RoleCommenter)
	e.grantDocumentRoleOn(docID, fixtureViewer, repo.RoleViewer)

	rr := do(t, e.h, http.MethodPost, "/v1/avatar-reviews", fmt.Sprintf(
		`{"document_id": %q, "version_id": %q, "avatar_ids": ["av_priya"]}`, docID, verID,
	), map[string]string{
		"Content-Type":     "application/json",
		"X-Test-Principal": principalHeader(fixtureCommenter),
	})
	assertStatus(t, rr, http.StatusOK)
	created := decodeAvatarStatus(t, rr.Body.Bytes())
	if created.RequestID == "" || len(created.Reviews) != 1 || created.Reviews[0].State != "queued" {
		t.Fatalf("unexpected create response: %s", rr.Body.String())
	}

	// Repeat click is idempotent: same request id, no second convening.
	rr = do(t, e.h, http.MethodPost, "/v1/avatar-reviews", fmt.Sprintf(
		`{"document_id": %q, "version_id": %q, "avatar_ids": ["av_priya"]}`, docID, verID,
	), map[string]string{
		"Content-Type":     "application/json",
		"X-Test-Principal": principalHeader(fixtureCommenter),
	})
	assertStatus(t, rr, http.StatusOK)
	if repeat := decodeAvatarStatus(t, rr.Body.Bytes()); repeat.RequestID != created.RequestID {
		t.Fatalf("repeat request minted a new request id %q != %q", repeat.RequestID, created.RequestID)
	}

	status := waitAvatarReviewTerminal(t, e, created.RequestID)
	if status.Reviews[0].State != "commented" || status.Reviews[0].CommentCount < 2 {
		t.Fatalf("review did not comment: %+v", status.Reviews[0])
	}

	// Strangers cannot read review status for documents they cannot view.
	rr = do(t, e.h, http.MethodGet, "/v1/avatar-reviews/"+created.RequestID, "", map[string]string{
		"X-Test-Principal": principalHeader(fixtureStranger),
	})
	assertStatus(t, rr, http.StatusForbidden)

	// The fabricated comments arrive through the normal comment pipeline.
	rr = do(t, e.h, http.MethodGet, "/v1/documents/"+docID+"/comments", "", map[string]string{
		"X-Test-Principal": principalHeader(fixtureViewer),
	})
	assertStatus(t, rr, http.StatusOK)
	var listed struct {
		Items []struct {
			ID            string `json:"id"`
			AuthorDisplay string `json:"author_display"`
			Severity      string `json:"severity"`
			SelectedText  string `json:"selected_text"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &listed); err != nil {
		t.Fatalf("bad comments response: %v", err)
	}
	avatarComments := 0
	for _, item := range listed.Items {
		if item.AuthorDisplay != "Priya's avatar" {
			continue
		}
		avatarComments++
		if item.Severity == "" {
			t.Fatalf("avatar comment without severity: %+v", item)
		}
		if !strings.Contains(avatarFixtureHTML, item.SelectedText) {
			t.Fatalf("avatar comment quote %q is not verbatim document text", item.SelectedText)
		}
	}
	if avatarComments != status.Reviews[0].CommentCount {
		t.Fatalf("listed %d avatar comments, review reported %d", avatarComments, status.Reviews[0].CommentCount)
	}

	// Feedback: a viewer votes, toggles, and the state round-trips.
	target := ""
	for _, item := range listed.Items {
		if item.AuthorDisplay != "" {
			target = item.ID
			break
		}
	}
	rr = do(t, e.h, http.MethodPost, "/v1/avatar-comments/"+target+"/feedback", `{"signal":"up","persona":"author"}`, map[string]string{
		"Content-Type":     "application/json",
		"X-Test-Principal": principalHeader(fixtureViewer),
	})
	assertStatus(t, rr, http.StatusOK)
	var vote struct {
		Signal  string `json:"signal"`
		UpCount int    `json:"up_count"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &vote); err != nil {
		t.Fatalf("bad feedback response: %v", err)
	}
	if vote.Signal != "up" || vote.UpCount != 1 {
		t.Fatalf("feedback state = %+v", vote)
	}
}

func TestAvatarReviewValidation(t *testing.T) {
	e := newAvatarTestEnv(t)
	docID, verID := e.createDocument("Avatar validation", avatarFixtureHTML, fixtureOwner)
	e.grantDocumentRoleOn(docID, fixtureCommenter, repo.RoleCommenter)
	e.grantDocumentRoleOn(docID, fixtureViewer, repo.RoleViewer)

	// Viewer cannot trigger a review.
	rr := do(t, e.h, http.MethodPost, "/v1/avatar-reviews", fmt.Sprintf(
		`{"document_id": %q, "version_id": %q, "avatar_ids": ["av_priya"]}`, docID, verID,
	), map[string]string{
		"Content-Type":     "application/json",
		"X-Test-Principal": principalHeader(fixtureViewer),
	})
	assertStatus(t, rr, http.StatusForbidden)

	// The version must belong to the document.
	rr = do(t, e.h, http.MethodPost, "/v1/avatar-reviews", fmt.Sprintf(
		`{"document_id": %q, "version_id": %q, "avatar_ids": ["av_priya"]}`, docID, e.ver.ID,
	), map[string]string{
		"Content-Type":     "application/json",
		"X-Test-Principal": principalHeader(fixtureCommenter),
	})
	assertStatus(t, rr, http.StatusNotFound)

	// Unknown avatars are a client error.
	rr = do(t, e.h, http.MethodPost, "/v1/avatar-reviews", fmt.Sprintf(
		`{"document_id": %q, "version_id": %q, "avatar_ids": ["av_nope"]}`, docID, verID,
	), map[string]string{
		"Content-Type":     "application/json",
		"X-Test-Principal": principalHeader(fixtureCommenter),
	})
	assertStatus(t, rr, http.StatusBadRequest)

	// Unknown request ids are 404.
	rr = do(t, e.h, http.MethodGet, "/v1/avatar-reviews/avreq_missing", "", map[string]string{
		"X-Test-Principal": principalHeader(fixtureCommenter),
	})
	assertStatus(t, rr, http.StatusNotFound)
}

func TestAvatarFeedbackRejectsHumanComments(t *testing.T) {
	e := newAvatarTestEnv(t)
	rr := do(t, e.h, http.MethodPost, "/v1/avatar-comments/"+e.cmt.ID+"/feedback", `{"signal":"up"}`, map[string]string{
		"Content-Type":     "application/json",
		"X-Test-Principal": principalHeader(fixtureViewer),
	})
	assertStatus(t, rr, http.StatusNotFound)

	rr = do(t, e.h, http.MethodPost, "/v1/avatar-comments/cmt_missing/feedback", `{"signal":"up"}`, map[string]string{
		"Content-Type":     "application/json",
		"X-Test-Principal": principalHeader(fixtureViewer),
	})
	assertStatus(t, rr, http.StatusNotFound)
}
