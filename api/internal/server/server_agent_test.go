package server_test

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/razorpay/aidocs/api/internal/auth"
	"github.com/razorpay/aidocs/api/internal/server"
	"github.com/stretchr/testify/require"
)

var fixtureOtherUser = auth.Principal{Type: auth.PrincipalUser, ID: "other@example.com", Email: "other@example.com"}

func createTestDoc(t *testing.T, e *testEnv) string {
	t.Helper()
	body, ct := multipartBody(t, map[string]string{"title": "test doc"},
		"file", "doc.html", "<html><body><p>Test document content.</p></body></html>")
	w := doRaw(t, e.h, http.MethodPost, "/v1/documents", body,
		map[string]string{
			"Content-Type":     ct,
			"X-Test-Principal": principalHeader(fixtureUser),
		})
	require.Equal(t, http.StatusCreated, w.Code)
	var resp struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp.ID
}

func TestConfigResponseAgentEnabled(t *testing.T) {
	e := newConfiguredTestEnv(t, server.Config{
		AgentBaseURL: "http://litellm.internal",
		AgentModel:   "gpt-4o",
	})
	w := do(t, e.h, http.MethodGet, "/v1/config", "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		AgentEnabled bool `json:"agent_enabled"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.True(t, body.AgentEnabled)

	e2 := newConfiguredTestEnv(t, server.Config{})
	w2 := do(t, e2.h, http.MethodGet, "/v1/config", "", nil)
	require.Equal(t, http.StatusOK, w2.Code)
	var body2 struct {
		AgentEnabled bool `json:"agent_enabled"`
	}
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &body2))
	require.False(t, body2.AgentEnabled)
}

func TestAgentChat_FeatureDisabled(t *testing.T) {
	e := newConfiguredTestEnv(t, server.Config{})
	docID := createTestDoc(t, e)
	w := do(t, e.h, http.MethodPost,
		"/v1/documents/"+docID+"/agent/chat",
		`{"messages":[{"role":"user","content":"hi"}]}`,
		map[string]string{"X-Test-Principal": principalHeader(fixtureUser)},
	)
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestAgentChat_NoAccess(t *testing.T) {
	e := newConfiguredTestEnv(t, server.Config{AgentBaseURL: "http://unused"})
	docID := createTestDoc(t, e)
	w := do(t, e.h, http.MethodPost,
		"/v1/documents/"+docID+"/agent/chat",
		`{"messages":[{"role":"user","content":"hi"}]}`,
		map[string]string{"X-Test-Principal": principalHeader(fixtureOtherUser)},
	)
	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestAgentChat_BackendError(t *testing.T) {
	fakeLLM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer fakeLLM.Close()

	e := newConfiguredTestEnv(t, server.Config{AgentBaseURL: fakeLLM.URL})
	docID := createTestDoc(t, e)
	w := do(t, e.h, http.MethodPost,
		"/v1/documents/"+docID+"/agent/chat",
		`{"messages":[{"role":"user","content":"hi"}]}`,
		map[string]string{"X-Test-Principal": principalHeader(fixtureUser)},
	)
	require.Equal(t, http.StatusBadGateway, w.Code)
}

func TestAgentChat_Streams(t *testing.T) {
	fakeLLM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		f := w.(http.Flusher)
		for _, c := range []string{
			`{"choices":[{"delta":{"content":"Hello"}}]}`,
			`{"choices":[{"delta":{"content":" world"}}]}`,
		} {
			_, _ = w.Write([]byte("data: " + c + "\n\n"))
			f.Flush()
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		f.Flush()
	}))
	defer fakeLLM.Close()

	e := newConfiguredTestEnv(t, server.Config{AgentBaseURL: fakeLLM.URL, AgentModel: "test-model"})
	docID := createTestDoc(t, e)
	w := do(t, e.h, http.MethodPost,
		"/v1/documents/"+docID+"/agent/chat",
		`{"messages":[{"role":"user","content":"hi"}]}`,
		map[string]string{"X-Test-Principal": principalHeader(fixtureUser)},
	)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Header().Get("Content-Type"), "text/event-stream")

	var got []string
	sc := bufio.NewScanner(strings.NewReader(w.Body.String()))
	for sc.Scan() {
		if after, ok := strings.CutPrefix(sc.Text(), "data: "); ok {
			got = append(got, after)
		}
	}
	require.Equal(t, []string{"Hello", " world", "[DONE]"}, got)
}
