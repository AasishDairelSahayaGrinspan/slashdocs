package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/razorpay/slashdocs/api/internal/auth"
	"github.com/razorpay/slashdocs/api/internal/server"
	"github.com/stretchr/testify/require"
)

const docentCommentFixResponse = `{
  "action": "fix",
  "reply": "Good catch — step two now says EU instead of US.",
  "story": {
    "title": "How the thing works",
    "date": "generated",
    "steps": [
      {"narration": "First, this happens.", "elements": [
        {"id": "a", "type": "card", "tone": "start", "x": 80, "y": 100, "w": 200, "h": 80, "text": "Step one"}
      ]},
      {"narration": "Then this, in the EU region.", "elements": [
        {"id": "b", "type": "card", "tone": "process", "x": 400, "y": 100, "w": 200, "h": 80, "text": "Step two, EU"},
        {"type": "arrow", "from": "a", "to": "b"}
      ]},
      {"narration": "Finally this.", "elements": [
        {"id": "c", "type": "card", "tone": "end", "x": 720, "y": 100, "w": 200, "h": 80, "text": "Step three"}
      ]}
    ]
  }
}`

const docentCommentExplainResponse = `{
  "action": "explain",
  "reply": "The source document doesn't mention pricing, so adding that would mean inventing it."
}`

// docentFakeLLM serves validDocentStory on the first call (board generation)
// and a fixed second response on every call after that (the comment-response
// trigger), so one fake server backs both phases of the flow.
func docentFakeLLM(t *testing.T, secondResponse string) (*httptest.Server, *int32) {
	t.Helper()
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			_, _ = w.Write([]byte(chatCompletionBody(validDocentStory)))
			return
		}
		_, _ = w.Write([]byte(chatCompletionBody(secondResponse)))
	}))
	return srv, &calls
}

func createDocentBoard(t *testing.T, e *testEnv) (docID, verID string) {
	t.Helper()
	sourceID, _ := e.createDocument("Source doc", docentLongSource, fixtureUser)
	w, start := startDocent(t, e, sourceID, fixtureUser)
	assertStatus(t, w, http.StatusAccepted)
	job := pollDocentJob(t, e, sourceID, start.JobID, fixtureUser)
	require.Equal(t, "done", job.Status)
	return job.ID, job.CurrentVersionID
}

func postComment(t *testing.T, e *testEnv, docID, verID, body string, p auth.Principal) string {
	t.Helper()
	rr := do(t, e.h, http.MethodPost, "/v1/documents/"+docID+"/comments", `{
	  "version_id": "`+verID+`",
	  "body": "`+body+`",
	  "anchor": {"quote": "Step one", "prefix": "", "suffix": "", "dom_path": "html/body", "start_offset": 0, "end_offset": 8}
	}`, map[string]string{
		"Content-Type":     "application/json",
		"X-Test-Principal": principalHeader(p),
	})
	assertStatus(t, rr, http.StatusCreated)
	return jsonField(t, rr.Body.Bytes(), "id")
}

// testCommentResponse is a minimal local decode of the comment list JSON
// shape — the real commentResponse type is unexported in package server and
// this file builds as package server_test, so it can't be reused here.
type testCommentResponse struct {
	ID     string `json:"id"`
	Author struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	} `json:"author"`
	Body    string                 `json:"body"`
	Replies *[]testCommentResponse `json:"replies"`
}

// pollForReply polls the comment list until the named comment has at least
// one reply, or the deadline passes (returns nil in that case).
func pollForReply(t *testing.T, e *testEnv, docID, commentID string, p auth.Principal) *testCommentResponse {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		rr := do(t, e.h, http.MethodGet, "/v1/documents/"+docID+"/comments", "", map[string]string{
			"X-Test-Principal": principalHeader(p),
		})
		assertStatus(t, rr, http.StatusOK)
		var listed struct {
			Items []testCommentResponse `json:"items"`
		}
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &listed))
		for _, item := range listed.Items {
			if item.ID == commentID && item.Replies != nil && len(*item.Replies) > 0 {
				reply := (*item.Replies)[0]
				return &reply
			}
		}
		if time.Now().After(deadline) {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestDocentComment_FixUpdatesBoardAndReplies(t *testing.T) {
	fakeLLM, calls := docentFakeLLM(t, docentCommentFixResponse)
	defer fakeLLM.Close()

	e := newConfiguredTestEnv(t, server.Config{DocentAgentBaseURL: fakeLLM.URL})
	boardID, boardVerID := createDocentBoard(t, e)

	commentID := postComment(t, e, boardID, boardVerID, "This says US but it should say EU.", fixtureUser)

	reply := pollForReply(t, e, boardID, commentID, fixtureUser)
	require.NotNil(t, reply, "Docent never replied to the comment")
	require.Equal(t, "service_account", reply.Author.Type)
	require.Equal(t, "docent", reply.Author.ID)
	require.Contains(t, reply.Body, "EU")
	require.Equal(t, int32(2), atomic.LoadInt32(calls), "expected exactly one board-generation call and one comment-response call")

	// The board must have a new version with the corrected content, and must
	// still carry the Docent source marker (so a second comment keeps working).
	docReq := do(t, e.h, http.MethodGet, "/v1/documents/"+boardID, "", map[string]string{
		"X-Test-Principal": principalHeader(fixtureUser),
	})
	assertStatus(t, docReq, http.StatusOK)
	newVerID := jsonField(t, docReq.Body.Bytes(), "current_version_id")
	require.NotEqual(t, boardVerID, newVerID, "a fix must create a new version, not edit in place")

	htmlReq := do(t, e.h, http.MethodGet, "/v1/documents/"+boardID+"/versions/"+newVerID+"/html", "", map[string]string{
		"X-Test-Principal": principalHeader(fixtureUser),
	})
	assertStatus(t, htmlReq, http.StatusOK)
	html := htmlReq.Body.String()
	require.Contains(t, html, "Step two, EU")
	require.Contains(t, html, "docent-source-doc-id")
}

func TestDocentComment_ExplainLeavesBoardUnchanged(t *testing.T) {
	fakeLLM, calls := docentFakeLLM(t, docentCommentExplainResponse)
	defer fakeLLM.Close()

	e := newConfiguredTestEnv(t, server.Config{DocentAgentBaseURL: fakeLLM.URL})
	boardID, boardVerID := createDocentBoard(t, e)

	commentID := postComment(t, e, boardID, boardVerID, "Can you add pricing info?", fixtureUser)

	reply := pollForReply(t, e, boardID, commentID, fixtureUser)
	require.NotNil(t, reply, "Docent never replied to the comment")
	require.Equal(t, "docent", reply.Author.ID)
	require.Contains(t, reply.Body, "inventing")
	require.Equal(t, int32(2), atomic.LoadInt32(calls))

	docReq := do(t, e.h, http.MethodGet, "/v1/documents/"+boardID, "", map[string]string{
		"X-Test-Principal": principalHeader(fixtureUser),
	})
	assertStatus(t, docReq, http.StatusOK)
	require.Equal(t, boardVerID, jsonField(t, docReq.Body.Bytes(), "current_version_id"),
		"an explain decision must never create a new version")
}

func TestDocentComment_OrdinaryDocumentNeverTriggersLLM(t *testing.T) {
	fakeLLM, calls := docentFakeLLM(t, docentCommentExplainResponse)
	defer fakeLLM.Close()

	e := newConfiguredTestEnv(t, server.Config{DocentAgentBaseURL: fakeLLM.URL})
	docID, verID := e.createDocument("Ordinary doc", docentLongSource, fixtureUser)

	commentID := postComment(t, e, docID, verID, "Just a regular comment.", fixtureUser)

	reply := pollForReply(t, e, docID, commentID, fixtureUser)
	require.Nil(t, reply, "a document Docent did not generate must never get an automated reply")
	require.Equal(t, int32(0), atomic.LoadInt32(calls), "a non-Docent document must never call the LLM backend")
}
