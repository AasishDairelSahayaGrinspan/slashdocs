package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/razorpay/slashdocs/api/internal/auth"
	"github.com/razorpay/slashdocs/api/internal/server"
	"github.com/stretchr/testify/require"
)

var fixtureDocentOtherUser = auth.Principal{Type: auth.PrincipalUser, ID: "docent-other@example.com", Email: "docent-other@example.com"}

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
	docID, _ := e.createDocument("Source doc", "<html><body><p>Some content.</p></body></html>", fixtureUser)
	w := do(t, e.h, http.MethodPost, "/v1/documents/"+docID+"/docent/generate", "",
		map[string]string{"X-Test-Principal": principalHeader(fixtureUser)},
	)
	assertStatus(t, w, http.StatusNotFound)
}

func TestGenerateDocent_NoAccess(t *testing.T) {
	e := newConfiguredTestEnv(t, server.Config{DocentAgentBaseURL: "http://unused"})
	docID, _ := e.createDocument("Source doc", "<html><body><p>Some content.</p></body></html>", fixtureUser)
	e.seedUser(fixtureDocentOtherUser)
	w := do(t, e.h, http.MethodPost, "/v1/documents/"+docID+"/docent/generate", "",
		map[string]string{"X-Test-Principal": principalHeader(fixtureDocentOtherUser)},
	)
	assertStatus(t, w, http.StatusForbidden)
}

func TestGenerateDocent_BackendError(t *testing.T) {
	fakeLLM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer fakeLLM.Close()

	e := newConfiguredTestEnv(t, server.Config{DocentAgentBaseURL: fakeLLM.URL})
	docID, _ := e.createDocument("Source doc", "<html><body><p>Some content, long enough to narrate.</p></body></html>", fixtureUser)
	w := do(t, e.h, http.MethodPost, "/v1/documents/"+docID+"/docent/generate", "",
		map[string]string{"X-Test-Principal": principalHeader(fixtureUser)},
	)
	assertStatus(t, w, http.StatusBadGateway)
}

func TestGenerateDocent_UnparseableResponse(t *testing.T) {
	fakeLLM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(chatCompletionBody("not json at all")))
	}))
	defer fakeLLM.Close()

	e := newConfiguredTestEnv(t, server.Config{DocentAgentBaseURL: fakeLLM.URL})
	docID, _ := e.createDocument("Source doc", "<html><body><p>Some content, long enough to narrate.</p></body></html>", fixtureUser)
	w := do(t, e.h, http.MethodPost, "/v1/documents/"+docID+"/docent/generate", "",
		map[string]string{"X-Test-Principal": principalHeader(fixtureUser)},
	)
	assertStatus(t, w, http.StatusBadGateway)
	require.Contains(t, w.Body.String(), "could not parse")
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
	docID, _ := e.createDocument("Source doc", "<html><body><p>Some content, long enough to narrate.</p></body></html>", fixtureUser)
	w := do(t, e.h, http.MethodPost, "/v1/documents/"+docID+"/docent/generate", "",
		map[string]string{"X-Test-Principal": principalHeader(fixtureUser)},
	)
	assertStatus(t, w, http.StatusCreated)
	require.Equal(t, "test-model", gotModel)

	var out struct {
		ID               string `json:"id"`
		CurrentVersionID string `json:"current_version_id"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	require.NotEmpty(t, out.ID)
	require.NotEqual(t, docID, out.ID, "the board must be a new, separate document, not a new version of the source")
	require.NotEmpty(t, out.CurrentVersionID)

	// Fetch the generated board's HTML and sanity-check the story made it in,
	// safely escaped for inline-script embedding.
	htmlReq := do(t, e.h, http.MethodGet,
		"/v1/documents/"+out.ID+"/versions/"+out.CurrentVersionID+"/html", "",
		map[string]string{"X-Test-Principal": principalHeader(fixtureUser)},
	)
	assertStatus(t, htmlReq, http.StatusOK)
	body := htmlReq.Body.String()
	require.Contains(t, body, "Step one")
	require.Contains(t, body, "How the thing works")
	require.False(t, strings.Contains(body, "</script>\n</script>"), "malformed double-close would indicate a broken escape")
}
