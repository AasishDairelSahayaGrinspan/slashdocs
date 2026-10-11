package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/razorpay/slashdocs/api/internal/auth"
	"github.com/razorpay/slashdocs/api/internal/server"
	"github.com/stretchr/testify/require"
)

var fixtureDocentOtherUser = auth.Principal{Type: auth.PrincipalUser, ID: "docent-other@example.com", Email: "docent-other@example.com"}

// A source long enough to pass the minimum-content gate (500 bytes); short
// test fixtures elsewhere are deliberately under that bar to exercise it.
const docentLongSource = `<html><body><h1>Weekly On-Call Handoff Tool</h1>
<p>This internal tool coordinates on-call handoffs across three regions: APAC, EU, and US.
Each region runs its own weekly rotation, and a handoff happens every Monday at 9am local time.
If the incoming on-call engineer does not acknowledge the handoff within thirty minutes, it
automatically escalates to that region's lead. Since this was introduced, missed handoffs have
dropped to nearly zero across all three regions, and the on-call rotation schedule is now
reviewed quarterly by the infrastructure team.</p></body></html>`

func chatCompletionBody(content string) string {
	payload, _ := json.Marshal(map[string]any{
		"choices": []map[string]any{
			{"message": map[string]string{"content": content}},
		},
	})
	return string(payload)
}

const validDocentStory = `{
  "title": "How the thing works",
  "date": "generated",
  "steps": [
    {"narration": "First, this happens.", "elements": [
      {"id": "a", "type": "card", "tone": "start", "x": 80, "y": 100, "w": 200, "h": 80, "text": "Step one"}
    ]},
    {"narration": "Then this.", "elements": [
      {"id": "b", "type": "card", "tone": "process", "x": 400, "y": 100, "w": 200, "h": 80, "text": "Step two"},
      {"type": "arrow", "from": "a", "to": "b"}
    ]},
    {"narration": "Finally this.", "elements": [
      {"id": "c", "type": "card", "tone": "end", "x": 720, "y": 100, "w": 200, "h": 80, "text": "Step three"}
    ]}
  ]
}`

// docentStartBody is the shape returned by POST .../docent/generate.
type docentStartBody struct {
	Status           string `json:"status"`
	JobID            string `json:"job_id"`
	ID               string `json:"id"`
	CurrentVersionID string `json:"current_version_id"`
	Truncated        bool   `json:"truncated"`
}

type docentJobBody struct {
	Status           string `json:"status"`
	ID               string `json:"id"`
	CurrentVersionID string `json:"current_version_id"`
	Error            *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func startDocent(t *testing.T, e *testEnv, docID string, p auth.Principal) (*httptest.ResponseRecorder, docentStartBody) {
	t.Helper()
	w := do(t, e.h, http.MethodPost, "/v1/documents/"+docID+"/docent/generate", "",
		map[string]string{"X-Test-Principal": principalHeader(p)},
	)
	var body docentStartBody
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return w, body
}

// pollDocentJob polls until the job leaves "pending", or the deadline passes.
func pollDocentJob(t *testing.T, e *testEnv, docID, jobID string, p auth.Principal) docentJobBody {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		w := do(t, e.h, http.MethodGet, "/v1/documents/"+docID+"/docent/jobs/"+jobID, "",
			map[string]string{"X-Test-Principal": principalHeader(p)},
		)
		require.Equal(t, http.StatusOK, w.Code)
		var body docentJobBody
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		if body.Status != "pending" {
			return body
		}
		if time.Now().After(deadline) {
			t.Fatal("docent job never left pending")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestConfigResponseDocentEnabled(t *testing.T) {
	e := newConfiguredTestEnv(t, server.Config{DocentAgentBaseURL: "http://litellm.internal"})
	w := do(t, e.h, http.MethodGet, "/v1/config", "", nil)
	assertStatus(t, w, http.StatusOK)
	var body struct {
		DocentEnabled bool `json:"docent_enabled"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.True(t, body.DocentEnabled)

	e2 := newConfiguredTestEnv(t, server.Config{})
	w2 := do(t, e2.h, http.MethodGet, "/v1/config", "", nil)
	assertStatus(t, w2, http.StatusOK)
	var body2 struct {
		DocentEnabled bool `json:"docent_enabled"`
	}
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &body2))
	require.False(t, body2.DocentEnabled)
}

func TestGenerateDocent_FeatureDisabled(t *testing.T) {
	e := newConfiguredTestEnv(t, server.Config{})
	docID, _ := e.createDocument("Source doc", docentLongSource, fixtureUser)
	w, _ := startDocent(t, e, docID, fixtureUser)
	assertStatus(t, w, http.StatusNotFound)

	jw := do(t, e.h, http.MethodGet, "/v1/documents/"+docID+"/docent/jobs/djob_doesnotexist", "",
		map[string]string{"X-Test-Principal": principalHeader(fixtureUser)},
	)
	assertStatus(t, jw, http.StatusNotFound)
}

func TestGenerateDocent_NoAccess(t *testing.T) {
	e := newConfiguredTestEnv(t, server.Config{DocentAgentBaseURL: "http://unused"})
	docID, _ := e.createDocument("Source doc", docentLongSource, fixtureUser)
	e.seedUser(fixtureDocentOtherUser)
	w, _ := startDocent(t, e, docID, fixtureDocentOtherUser)
	assertStatus(t, w, http.StatusForbidden)
}

func TestGenerateDocent_SourceTooShort(t *testing.T) {
	e := newConfiguredTestEnv(t, server.Config{DocentAgentBaseURL: "http://unused"})
	docID, _ := e.createDocument("Tiny doc", "<html><body><p>Too short.</p></body></html>", fixtureUser)
	w, body := startDocent(t, e, docID, fixtureUser)
	assertStatus(t, w, http.StatusUnprocessableEntity)
	require.Contains(t, w.Body.String(), "too short")
	require.Empty(t, body.JobID)
}

func TestGenerateDocent_BackendError(t *testing.T) {
	fakeLLM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer fakeLLM.Close()

	e := newConfiguredTestEnv(t, server.Config{DocentAgentBaseURL: fakeLLM.URL})
	docID, _ := e.createDocument("Source doc", docentLongSource, fixtureUser)
	w, start := startDocent(t, e, docID, fixtureUser)
	assertStatus(t, w, http.StatusAccepted)
	require.Equal(t, "pending", start.Status)
	require.NotEmpty(t, start.JobID)

	job := pollDocentJob(t, e, docID, start.JobID, fixtureUser)
	require.Equal(t, "error", job.Status)
	require.NotNil(t, job.Error)
	require.Equal(t, "docent_backend_error", job.Error.Code)
	// The gateway URL and Go transport error must not reach the client (F4).
	require.NotContains(t, job.Error.Message, fakeLLM.URL)
	require.NotContains(t, job.Error.Message, "http.Client")
}

func TestGenerateDocent_UnparseableResponse(t *testing.T) {
	fakeLLM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(chatCompletionBody("not json at all")))
	}))
	defer fakeLLM.Close()

	e := newConfiguredTestEnv(t, server.Config{DocentAgentBaseURL: fakeLLM.URL})
	docID, _ := e.createDocument("Source doc", docentLongSource, fixtureUser)
	w, start := startDocent(t, e, docID, fixtureUser)
	assertStatus(t, w, http.StatusAccepted)

	job := pollDocentJob(t, e, docID, start.JobID, fixtureUser)
	require.Equal(t, "error", job.Status)
	require.Equal(t, "docent_unparseable", job.Error.Code)
	require.Contains(t, job.Error.Message, "could not parse")
	// The raw decoder detail must not leak (F4).
	require.NotContains(t, job.Error.Message, "looking for beginning of value")
}

func TestGenerateDocent_MinimumStepsEnforced(t *testing.T) {
	// A response with fewer steps than docentMinSteps (3) must fail, not be
	// silently accepted — regression test for the `&& len == 0` guard bug.
	oneStep := `{"title":"t","date":"d","steps":[{"narration":"only one","elements":[{"id":"a","type":"card","x":10,"y":10,"w":100,"h":50,"text":"solo"}]}]}`
	fakeLLM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(chatCompletionBody(oneStep)))
	}))
	defer fakeLLM.Close()

	e := newConfiguredTestEnv(t, server.Config{DocentAgentBaseURL: fakeLLM.URL})
	docID, _ := e.createDocument("Source doc", docentLongSource, fixtureUser)
	w, start := startDocent(t, e, docID, fixtureUser)
	assertStatus(t, w, http.StatusAccepted)

	job := pollDocentJob(t, e, docID, start.JobID, fixtureUser)
	require.Equal(t, "error", job.Status)
	require.Equal(t, "docent_unparseable", job.Error.Code)
}

func TestGenerateDocent_Success(t *testing.T) {
	var gotModel string
	fakeLLM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		gotModel = req.Model
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(chatCompletionBody(validDocentStory)))
	}))
	defer fakeLLM.Close()

	e := newConfiguredTestEnv(t, server.Config{
		DocentAgentBaseURL: fakeLLM.URL,
		DocentAgentModel:   "test-model",
	})
	docID, _ := e.createDocument("Source doc", docentLongSource, fixtureUser)
	w, start := startDocent(t, e, docID, fixtureUser)
	assertStatus(t, w, http.StatusAccepted)
	require.Equal(t, "pending", start.Status)

	job := pollDocentJob(t, e, docID, start.JobID, fixtureUser)
	require.Equal(t, "done", job.Status)
	require.Equal(t, "test-model", gotModel)
	require.NotEmpty(t, job.ID)
	require.NotEqual(t, docID, job.ID, "the board must be a new, separate document, not a new version of the source")
	require.NotEmpty(t, job.CurrentVersionID)

	// Fetch the generated board's HTML as the generating principal.
	htmlReq := do(t, e.h, http.MethodGet,
		"/v1/documents/"+job.ID+"/versions/"+job.CurrentVersionID+"/html", "",
		map[string]string{"X-Test-Principal": principalHeader(fixtureUser)},
	)
	assertStatus(t, htmlReq, http.StatusOK)
	body := htmlReq.Body.String()
	require.Contains(t, body, "Step one")
	require.Contains(t, body, "How the thing works")
	require.Contains(t, body, "AI-generated")
	require.False(t, strings.Contains(body, "</script>\n</script>"), "malformed double-close would indicate a broken escape")

	// F1: the board must be private, not auto-shared with "anyone" — a
	// second principal with no explicit grant must be refused.
	e.seedUser(fixtureDocentOtherUser)
	otherReq := do(t, e.h, http.MethodGet, "/v1/documents/"+job.ID, "",
		map[string]string{"X-Test-Principal": principalHeader(fixtureDocentOtherUser)},
	)
	assertStatus(t, otherReq, http.StatusForbidden)
}

func TestGenerateDocent_RepeatRequestReopensExistingBoard(t *testing.T) {
	fakeLLM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(chatCompletionBody(validDocentStory)))
	}))
	defer fakeLLM.Close()

	e := newConfiguredTestEnv(t, server.Config{DocentAgentBaseURL: fakeLLM.URL})
	docID, _ := e.createDocument("Source doc", docentLongSource, fixtureUser)

	w1, start1 := startDocent(t, e, docID, fixtureUser)
	assertStatus(t, w1, http.StatusAccepted)
	job1 := pollDocentJob(t, e, docID, start1.JobID, fixtureUser)
	require.Equal(t, "done", job1.Status)

	// A second request against the same source document+version must reopen
	// the board just created, not generate a fresh one (F12).
	w2, start2 := startDocent(t, e, docID, fixtureUser)
	assertStatus(t, w2, http.StatusOK)
	require.Equal(t, "exists", start2.Status)
	require.Equal(t, job1.ID, start2.ID)
}

func TestGenerateDocent_OnePerPrincipalAtATime(t *testing.T) {
	release := make(chan struct{})
	fakeLLM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(chatCompletionBody(validDocentStory)))
	}))
	defer fakeLLM.Close()
	defer close(release)

	e := newConfiguredTestEnv(t, server.Config{DocentAgentBaseURL: fakeLLM.URL})
	docA, _ := e.createDocument("Source A", docentLongSource, fixtureUser)
	docB, _ := e.createDocument("Source B", docentLongSource, fixtureUser)

	w1, start1 := startDocent(t, e, docA, fixtureUser)
	assertStatus(t, w1, http.StatusAccepted)
	require.NotEmpty(t, start1.JobID)

	// The first job is still running (blocked on `release`); a second
	// generate call by the same principal, even against a different
	// document, must be rejected rather than fan out another LLM call (F5).
	w2, _ := startDocent(t, e, docB, fixtureUser)
	assertStatus(t, w2, http.StatusTooManyRequests)
	require.Contains(t, w2.Body.String(), "docent_busy")
}
