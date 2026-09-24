# Agent Panel Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Google Docs–style AI panel to the document viewer where users can ask questions about the open document and request a summary.

**Architecture:** A new Go handler (`handlers_agent.go`) streams responses from a LiteLLM-compatible endpoint. It fetches the document HTML via `repo.GetVersionHTML`, strips it to plaintext, and forwards it as system context. The frontend gains a `<AgentPanel />` component (new `doc-agent.tsx`) that slides in from the right as a second aside, mutually exclusive with the comments panel. The feature is silently disabled when `AIDOCS_AGENT_BASE_URL` is unset.

**Tech Stack:** Go 1.22, Gin, `golang.org/x/net/html` (already in `go.mod`), React 18, TypeScript, Framer Motion, Lucide React, TanStack Query

**Spec:** `docs/superpowers/specs/2026-09-24-agent-panel-design.md`

## Global Constraints

- Feature MUST be silently absent (button hidden, endpoint returns 404) when `AIDOCS_AGENT_BASE_URL` is unset
- Agent MUST only respond to callers with at least `repo.RoleViewer` on the document
- LiteLLM request MUST use the OpenAI-compatible chat completions API with `stream: true`
- Document plaintext passed as context MUST be truncated to 60 000 characters
- Agent panel and comments panel MUST be mutually exclusive (opening one closes the other)
- SSE stream format: `data: {text}\n\n` per chunk; `data: [DONE]\n\n` as final event
- No conversation persistence — chat state lives only in React component memory

## Review Focus

- Caller without doc access hits the agent endpoint — must get 403, not a leaked LLM response
- `AIDOCS_AGENT_BASE_URL` set but LiteLLM returns 5xx — client must see a 502, not a hang or panic
- Document has no `current_version_id` and no `version_id` in request — must return 422
- Stream interrupted mid-response — frontend must show partial text plus error banner, not blank panel
- Plaintext exceeds 60 000 characters — must truncate before sending to LiteLLM, not fail

---

### Task 1: Backend config + feature flag

**Files:**
- Modify: `api/internal/server/server.go` — add 3 fields to `Config` and `dependencies` structs
- Modify: `api/internal/server/handlers_auth.go` — add `agent_enabled` to `configResponse`
- Modify: `api/cmd/aidocs-server/main.go` — read env vars, pass to `Config`
- Modify: `.env.example` — document new env vars
- Create: `api/internal/server/server_agent_test.go`

**Interfaces:**
- Produces: `Config.AgentBaseURL string`, `Config.AgentAPIKey string`, `Config.AgentModel string`
- Produces: `dependencies.agentBaseURL`, `dependencies.agentAPIKey`, `dependencies.agentModel` — consumed by Task 2
- Produces: `configResponse.AgentEnabled bool` serialised as `"agent_enabled"` — consumed by Task 5 frontend

- [ ] **Step 1: Add fields to Config struct**

In `api/internal/server/server.go`, find `type Config struct {` and add after `GoogleGroupsEnabled bool`:

```go
// Agent fields — all empty means agent feature is disabled.
AgentBaseURL string
AgentAPIKey  string
AgentModel   string
```

- [ ] **Step 2: Store agent fields on dependencies**

In the same file, find `type dependencies struct {` and add:

```go
agentBaseURL string
agentAPIKey  string
agentModel   string
```

In `func New(cfg Config, ...)`, where other deps are populated from cfg (e.g. `deps.appOrigin = cfg.AppOrigin`), add:

```go
deps.agentBaseURL = cfg.AgentBaseURL
deps.agentAPIKey  = cfg.AgentAPIKey
deps.agentModel   = cfg.AgentModel
if deps.agentModel == "" {
    deps.agentModel = "gpt-4o"
}
```

- [ ] **Step 3: Add AgentEnabled to configResponse**

In `api/internal/server/handlers_auth.go`, find `type configResponse struct {` and add:

```go
AgentEnabled bool `json:"agent_enabled"`
```

Find the `config` handler function body where it constructs `configResponse{...}`. Look at how `LabelsEnabled` is set (it reads from `h.deps.disableLabels` or similar). Mirror that pattern:

```go
AgentEnabled: h.deps.agentBaseURL != "",
```

- [ ] **Step 4: Write the test**

Create `api/internal/server/server_agent_test.go`:

```go
package server_test

import (
    "encoding/json"
    "net/http"
    "testing"

    "github.com/razorpay/aidocs/api/internal/server"
    "github.com/stretchr/testify/require"
)

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
```

- [ ] **Step 5: Run test — expect FAIL**

```bash
cd api && go test ./internal/server/... -run TestConfigResponseAgentEnabled -v
```

Expected: compile error or FAIL (fields don't exist yet in handler response).

- [ ] **Step 6: Wire env vars in main.go**

In `api/cmd/aidocs-server/main.go`, find `server.New(server.Config{` and add:

```go
AgentBaseURL: os.Getenv("AIDOCS_AGENT_BASE_URL"),
AgentAPIKey:  os.Getenv("AIDOCS_AGENT_API_KEY"),
AgentModel:   envOrDefault("AIDOCS_AGENT_MODEL", "gpt-4o"),
```

- [ ] **Step 7: Run test — expect PASS**

```bash
cd api && go test ./internal/server/... -run TestConfigResponseAgentEnabled -v
```

Expected: PASS.

- [ ] **Step 8: Document env vars in .env.example**

Append to `.env.example` after the `# Feature rollout switches` section:

```
# ---------------------------------------------------------------------------
# AI agent panel (optional)
# ---------------------------------------------------------------------------

# When set, enables the in-app AI agent panel. Point to your LiteLLM base URL.
# AIDOCS_AGENT_BASE_URL=http://litellm.internal

# Bearer token for the LiteLLM endpoint.
# AIDOCS_AGENT_API_KEY=

# Model name to pass to LiteLLM (default: gpt-4o).
# AIDOCS_AGENT_MODEL=gpt-4o
```

- [ ] **Step 9: Commit**

```bash
git add api/internal/server/server.go api/internal/server/handlers_auth.go \
        api/cmd/aidocs-server/main.go .env.example \
        api/internal/server/server_agent_test.go
git commit -m "feat: add agent config fields and agent_enabled to /v1/config"
```

---

### Task 2: Backend agent chat handler

**Files:**
- Create: `api/internal/server/handlers_agent.go`
- Modify: `api/internal/server/server.go` — register route
- Modify: `api/internal/server/server_agent_test.go` — add chat tests

**Interfaces:**
- Consumes: `h.deps.agentBaseURL`, `h.deps.agentAPIKey`, `h.deps.agentModel` from Task 1
- Consumes: `h.deps.repository.GetDocument(ctx, docID) (repo.Document, error)`
- Consumes: `h.deps.repository.GetVersion(ctx, versionID) (repo.Version, error)`
- Consumes: `h.deps.repository.GetVersionHTML(ctx, versionID) ([]byte, error)`
- Consumes: `h.needDocRole(c, docID, repo.RoleViewer) bool`
- Consumes: `notFound(c)`, `badRequest(c, msg)`, `internalErr(c, err)` from `responses.go`
- Produces: `POST /v1/documents/:id/agent/chat` — SSE text/event-stream

- [ ] **Step 1: Write failing tests**

Add to `api/internal/server/server_agent_test.go`:

```go
import (
    "bufio"
    "net/http/httptest"
    "strings"
)

// createTestDoc creates a document owned by fixtureUser and returns its ID.
// Requires multipartBody and doRaw helpers from testhelpers_test.go.
func createTestDoc(t *testing.T, e *testEnv) string {
    t.Helper()
    body, ct := multipartBody(t, map[string]string{"title": "test doc"},
        "file", "doc.html", "<html><body><p>Test content.</p></body></html>")
    w := doRaw(t, e.h, http.MethodPost, "/v1/documents", body,
        map[string]string{
            "Content-Type":     ct,
            "X-Test-Principal": principalHeader(fixtureUser),
        })
    require.Equal(t, http.StatusCreated, w.Code)
    var resp struct{ ID string `json:"id"` }
    require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
    return resp.ID
}

func TestAgentChat_FeatureDisabled(t *testing.T) {
    e := newConfiguredTestEnv(t, server.Config{}) // no AgentBaseURL
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
    // fixtureOtherUser has no grant on docID
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
    // Fake LiteLLM returning two OpenAI-format SSE chunks then [DONE].
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
```

Note: check `testhelpers_test.go` for `fixtureOtherUser`. If absent, add:
```go
var fixtureOtherUser = auth.Principal{Type: auth.PrincipalUser, ID: "other@example.com"}
```

- [ ] **Step 2: Run tests — expect FAIL**

```bash
cd api && go test ./internal/server/... -run "TestAgentChat" -v
```

Expected: compile errors or FAIL with 404 (route not registered yet).

- [ ] **Step 3: Create handlers_agent.go**

Create `api/internal/server/handlers_agent.go`:

```go
package server

import (
    "bufio"
    "bytes"
    "encoding/json"
    "fmt"
    "net/http"
    "strings"

    "github.com/gin-gonic/gin"
    "golang.org/x/net/html"

    "github.com/razorpay/aidocs/api/internal/repo"
)

const agentMaxPlaintextBytes = 60_000

type agentMessage struct {
    Role    string `json:"role"`
    Content string `json:"content"`
}

type agentChatRequest struct {
    Messages  []agentMessage `json:"messages"`
    VersionID string         `json:"version_id,omitempty"`
}

func (h handlers) agentChat(c *gin.Context) {
    if h.deps.agentBaseURL == "" {
        notFound(c)
        return
    }

    docID := c.Param("id")
    if !h.needDocRole(c, docID, repo.RoleViewer) {
        return
    }

    var in agentChatRequest
    if err := c.ShouldBindJSON(&in); err != nil || len(in.Messages) == 0 {
        badRequest(c, "messages must not be empty")
        return
    }

    versionID := in.VersionID
    if versionID == "" {
        doc, err := h.deps.repository.GetDocument(c.Request.Context(), docID)
        if err != nil {
            internalErr(c, err)
            return
        }
        versionID = doc.CurrentVersionID
    } else {
        v, err := h.deps.repository.GetVersion(c.Request.Context(), versionID)
        if err != nil || v.DocumentID != docID {
            notFound(c)
            return
        }
    }

    if versionID == "" {
        c.JSON(http.StatusUnprocessableEntity, gin.H{
            "error": gin.H{"message": "document has no version to analyze"},
        })
        return
    }

    raw, err := h.deps.repository.GetVersionHTML(c.Request.Context(), versionID)
    if err != nil {
        internalErr(c, err)
        return
    }

    plaintext := htmlToText(raw)
    if len(plaintext) > agentMaxPlaintextBytes {
        plaintext = plaintext[:agentMaxPlaintextBytes]
    }

    systemPrompt := "You are a helpful assistant. The document the user is asking about follows.\n\n<document>\n" +
        plaintext + "\n</document>"

    messages := make([]agentMessage, 0, len(in.Messages)+1)
    messages = append(messages, agentMessage{Role: "system", Content: systemPrompt})
    messages = append(messages, in.Messages...)

    payload, err := json.Marshal(map[string]any{
        "model":    h.deps.agentModel,
        "messages": messages,
        "stream":   true,
    })
    if err != nil {
        internalErr(c, err)
        return
    }

    req, err := http.NewRequestWithContext(
        c.Request.Context(),
        http.MethodPost,
        h.deps.agentBaseURL+"/chat/completions",
        bytes.NewReader(payload),
    )
    if err != nil {
        internalErr(c, err)
        return
    }
    req.Header.Set("Content-Type", "application/json")
    if h.deps.agentAPIKey != "" {
        req.Header.Set("Authorization", "Bearer "+h.deps.agentAPIKey)
    }

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        internalErr(c, err)
        return
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        c.JSON(http.StatusBadGateway, gin.H{
            "error": gin.H{"message": "agent backend returned " + resp.Status},
        })
        return
    }

    c.Header("Content-Type", "text/event-stream")
    c.Header("Cache-Control", "no-cache")
    c.Header("X-Accel-Buffering", "no")

    sc := bufio.NewScanner(resp.Body)
    for sc.Scan() {
        after, ok := strings.CutPrefix(sc.Text(), "data: ")
        if !ok {
            continue
        }
        if after == "[DONE]" {
            fmt.Fprint(c.Writer, "data: [DONE]\n\n")
            c.Writer.(http.Flusher).Flush()
            return
        }
        var chunk struct {
            Choices []struct {
                Delta struct {
                    Content string `json:"content"`
                } `json:"delta"`
            } `json:"choices"`
        }
        if err := json.Unmarshal([]byte(after), &chunk); err != nil {
            continue
        }
        if len(chunk.Choices) == 0 || chunk.Choices[0].Delta.Content == "" {
            continue
        }
        fmt.Fprintf(c.Writer, "data: %s\n\n", chunk.Choices[0].Delta.Content)
        c.Writer.(http.Flusher).Flush()
    }
}

// htmlToText extracts visible text nodes from HTML, skipping script/style/head.
func htmlToText(raw []byte) string {
    doc, err := html.Parse(bytes.NewReader(raw))
    if err != nil {
        return string(raw)
    }
    var sb strings.Builder
    var walk func(*html.Node)
    walk = func(n *html.Node) {
        if n.Type == html.ElementNode {
            switch n.Data {
            case "script", "style", "head":
                return
            }
        }
        if n.Type == html.TextNode {
            if t := strings.TrimSpace(n.Data); t != "" {
                sb.WriteString(t)
                sb.WriteByte('\n')
            }
        }
        for child := n.FirstChild; child != nil; child = child.NextSibling {
            walk(child)
        }
    }
    walk(doc)
    return sb.String()
}
```

- [ ] **Step 4: Register route**

In `api/internal/server/server.go`, after the last `authed.POST("/documents/...` line, add:

```go
authed.POST("/documents/:id/agent/chat", h.agentChat)
```

- [ ] **Step 5: Run tests — expect PASS**

```bash
cd api && go test ./internal/server/... -run "TestAgentChat" -v
```

Expected: all four tests PASS.

- [ ] **Step 6: Run full backend tests**

```bash
cd api && go test ./... -count=1
```

Expected: all tests PASS.

- [ ] **Step 7: Commit**

```bash
git add api/internal/server/handlers_agent.go \
        api/internal/server/server.go \
        api/internal/server/server_agent_test.go
git commit -m "feat: add POST /v1/documents/:id/agent/chat streaming SSE endpoint"
```

---

### Task 3: Frontend API client

**Files:**
- Modify: `frontend/src/api.ts`

**Interfaces:**
- Consumes: `POST /v1/documents/:id/agent/chat` SSE endpoint from Task 2
- Produces: `ServerConfig.agent_enabled: boolean` — consumed by Task 5 DocBar
- Produces: `export type AgentMessage` and `export async function* agentChat(...)` — consumed by Task 4

- [ ] **Step 1: Add agent_enabled to ServerConfig**

In `frontend/src/api.ts`, find `export type ServerConfig = {` and add:

```ts
agent_enabled: boolean;
```

- [ ] **Step 2: Add AgentMessage type and agentChat generator**

After the `export class APIError` block and before `export const api = {`, add:

```ts
export type AgentMessage = { role: "user" | "assistant" | "system"; content: string };

export async function* agentChat(
  docId: string,
  messages: AgentMessage[],
  versionId?: string,
): AsyncGenerator<string> {
  const body: Record<string, unknown> = { messages };
  if (versionId) body.version_id = versionId;

  const res = await fetch(`/v1/documents/${encodeURIComponent(docId)}/agent/chat`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });

  if (!res.ok) {
    let msg = res.statusText;
    try {
      const payload = (await res.json()) as { error?: { message?: string } };
      msg = payload.error?.message || msg;
    } catch {}
    throw new APIError(res.status, "agent_error", msg);
  }

  const reader = res.body?.getReader();
  if (!reader) throw new APIError(0, "agent_error", "no response body");

  const decoder = new TextDecoder();
  let buffer = "";

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    const lines = buffer.split("\n");
    buffer = lines.pop() ?? "";
    for (const line of lines) {
      if (!line.startsWith("data: ")) continue;
      const after = line.slice(6);
      if (after === "[DONE]") return;
      yield after;
    }
  }
}
```

- [ ] **Step 3: Verify TypeScript compiles**

```bash
cd frontend && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/api.ts
git commit -m "feat: add agentChat async generator and agent_enabled to ServerConfig"
```

---

### Task 4: Agent panel UI component

**Files:**
- Create: `frontend/src/pages/doc-agent.tsx`

**Interfaces:**
- Consumes: `import { agentChat, type AgentMessage } from "@/api"` (Task 3)
- Consumes: `import { useDoc } from "./doc-context"` — provides `{ docId: string, version: string | undefined }`
- Produces: `export function AgentPanel({ mobile }: { mobile?: boolean })` — consumed by Task 5

- [ ] **Step 1: Check available CSS tokens**

Before writing JSX, confirm which tokens exist:

```bash
grep -o "var(--color-[a-z-]*)" frontend/src/styles.css | sort -u | head -40
```

Use only tokens that appear in that output. Safe fallbacks: `--color-surface`, `--color-surface-muted`, `--color-fg`, `--color-fg-muted`, `--color-border`, `--color-accent`.

- [ ] **Step 2: Create doc-agent.tsx**

Create `frontend/src/pages/doc-agent.tsx`:

```tsx
import { useState, useRef, useCallback } from "react";
import { Send, Trash2, Bot } from "lucide-react";
import { Button } from "@/components/ui/button";
import { agentChat, type AgentMessage } from "@/api";
import { useDoc } from "./doc-context";

type Message = AgentMessage & { id: string };

let msgCounter = 0;
function nextId() {
  return String(++msgCounter);
}

export function AgentPanel({ mobile }: { mobile?: boolean }) {
  const { docId, version } = useDoc();
  const [messages, setMessages] = useState<Message[]>([]);
  const [input, setInput] = useState("");
  const [streaming, setStreaming] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const bottomRef = useRef<HTMLDivElement>(null);

  const scrollBottom = useCallback(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, []);

  const send = useCallback(
    async (text: string) => {
      if (!text.trim() || streaming) return;
      setError(null);

      const userMsg: Message = { id: nextId(), role: "user", content: text };
      const assistantId = nextId();

      setMessages((prev) => [
        ...prev,
        userMsg,
        { id: assistantId, role: "assistant", content: "" },
      ]);
      setInput("");
      setStreaming(true);
      scrollBottom();

      const history: AgentMessage[] = [
        ...messages.map(({ role, content }) => ({ role, content })),
        { role: "user", content: text },
      ];

      try {
        for await (const chunk of agentChat(docId, history, version)) {
          setMessages((prev) =>
            prev.map((m) =>
              m.id === assistantId ? { ...m, content: m.content + chunk } : m,
            ),
          );
          scrollBottom();
        }
      } catch (e) {
        const msg = e instanceof Error ? e.message : "Something went wrong";
        setError(msg);
        setMessages((prev) =>
          prev.filter((m) => m.id !== assistantId || m.content !== ""),
        );
      } finally {
        setStreaming(false);
      }
    },
    [docId, version, messages, streaming, scrollBottom],
  );

  const clear = useCallback(() => {
    setMessages([]);
    setError(null);
    setInput("");
  }, []);

  const isEmpty = messages.length === 0;

  return (
    <div
      className={`flex h-full flex-col bg-[var(--color-surface)] ${mobile ? "" : "border-l border-[var(--color-border)]"}`}
    >
      <div className="flex items-center justify-between border-b border-[var(--color-border)] px-4 py-3">
        <div className="flex items-center gap-2 text-sm font-semibold text-[var(--color-fg)]">
          <Bot className="h-4 w-4" aria-hidden="true" />
          Ask AI
        </div>
        {!isEmpty && (
          <Button variant="ghost" size="icon" onClick={clear} aria-label="Clear conversation">
            <Trash2 className="h-4 w-4" />
          </Button>
        )}
      </div>

      {isEmpty && (
        <div className="flex flex-col gap-2 p-4">
          <p className="text-xs text-[var(--color-fg-muted)]">Try asking</p>
          {["Summarize this document", "What are the key takeaways?"].map((prompt) => (
            <button
              key={prompt}
              onClick={() => void send(prompt)}
              disabled={streaming}
              className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-muted)] px-3 py-2 text-left text-sm text-[var(--color-fg)] transition-colors hover:opacity-80 disabled:opacity-50"
            >
              {prompt}
            </button>
          ))}
        </div>
      )}

      {!isEmpty && (
        <div className="flex-1 overflow-y-auto space-y-3 px-4 py-3">
          {messages.map((m) => (
            <div key={m.id} className={`flex ${m.role === "user" ? "justify-end" : "justify-start"}`}>
              <div
                className={`max-w-[85%] rounded-xl px-3 py-2 text-sm whitespace-pre-wrap ${
                  m.role === "user"
                    ? "bg-[var(--color-accent)] text-white"
                    : "bg-[var(--color-surface-muted)] text-[var(--color-fg)]"
                }`}
              >
                {m.content}
                {m.role === "assistant" && m.content === "" && streaming && (
                  <span className="inline-block h-3 w-1 animate-pulse bg-current align-middle" />
                )}
              </div>
            </div>
          ))}
          <div ref={bottomRef} />
        </div>
      )}

      {error && (
        <div className="mx-4 mb-2 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-900 dark:bg-red-950 dark:text-red-400">
          {error}
        </div>
      )}

      <div className="border-t border-[var(--color-border)] p-3">
        <div className="flex items-end gap-2">
          <textarea
            rows={2}
            value={input}
            disabled={streaming}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey) {
                e.preventDefault();
                void send(input);
              }
            }}
            placeholder="Ask anything about this document…"
            className="flex-1 resize-none rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-muted)] px-3 py-2 text-sm text-[var(--color-fg)] placeholder:text-[var(--color-fg-muted)] focus:outline-none focus:ring-1 focus:ring-[var(--color-accent)] disabled:opacity-50"
          />
          <Button
            size="icon"
            onClick={() => void send(input)}
            disabled={streaming || !input.trim()}
            aria-label="Send"
          >
            <Send className="h-4 w-4" />
          </Button>
        </div>
        <p className="mt-1 text-xs text-[var(--color-fg-muted)]">
          Enter to send · Shift+Enter for newline
        </p>
      </div>
    </div>
  );
}
```

- [ ] **Step 3: Verify TypeScript compiles**

```bash
cd frontend && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/pages/doc-agent.tsx
git commit -m "feat: add AgentPanel component with streaming chat and quick-action chips"
```

---

### Task 5: Wire AgentPanel into DocPage and DocBar

**Files:**
- Modify: `frontend/src/pages/doc-page.tsx`
- Modify: `frontend/src/pages/doc-bar.tsx`

**Interfaces:**
- Consumes: `AgentPanel` from `"./doc-agent"` (Task 4)
- Consumes: `config.data?.agent_enabled` from existing `useQuery` in `doc-bar.tsx` (line ~90)
- Consumes: existing `commentsOpen`/`setCommentsOpen`, `isMobile`, `mobilePanelOpen`/`setMobilePanelOpen`

- [ ] **Step 1: Add agentOpen state and mutual exclusion to doc-page.tsx**

In `frontend/src/pages/doc-page.tsx`:

Add import at top:
```tsx
import { AgentPanel } from "./doc-agent";
```

After `const [commentsOpen, setCommentsOpen] = useState(true);` add:
```tsx
const [agentOpen, setAgentOpen] = useState(false);
```

Replace `toggleComments` with:
```tsx
const toggleComments = useCallback(() => {
  if (isMobile) setMobilePanelOpen((o) => !o);
  else
    setCommentsOpen((o) => {
      if (!o) setAgentOpen(false);
      return !o;
    });
}, [isMobile]);
```

Add `toggleAgent` after `toggleComments`:
```tsx
const toggleAgent = useCallback(() => {
  if (isMobile) setMobilePanelOpen((o) => !o);
  else
    setAgentOpen((o) => {
      if (!o) setCommentsOpen(false);
      return !o;
    });
}, [isMobile]);
```

Add new props to `<DocBar ...>` (inside the existing JSX, after `onToggleComments={toggleComments}`):
```tsx
agentOpen={isMobile ? mobilePanelOpen : agentOpen}
onToggleAgent={toggleAgent}
```

After the closing `</AnimatePresence>` of the comments rail, add:
```tsx
<AnimatePresence initial={false}>
  {agentOpen && !isMobile && (
    <motion.aside
      key="agent-rail"
      initial={{ width: 0, opacity: 0 }}
      animate={{ width: 360, opacity: 1 }}
      exit={{ width: 0, opacity: 0 }}
      transition={{ type: "spring", duration: 0.35, bounce: 0 }}
      className="relative hidden shrink-0 overflow-hidden bg-[var(--color-surface-muted)] md:block"
    >
      <div className="h-full w-[360px]">
        <AgentPanel />
      </div>
    </motion.aside>
  )}
</AnimatePresence>
```

After the existing comments `<Sheet>` block, add:
```tsx
<Sheet
  open={agentOpen && isMobile}
  onOpenChange={(open) => { if (!open) setAgentOpen(false); }}
>
  <SheetContent side="right" className="w-full max-w-md p-0 md:hidden" aria-describedby={undefined}>
    <SheetHeader className="sr-only">
      <SheetTitle>Ask AI</SheetTitle>
    </SheetHeader>
    <AgentPanel mobile />
  </SheetContent>
</Sheet>
```

- [ ] **Step 2: Add Sparkles button to DocBar**

In `frontend/src/pages/doc-bar.tsx`:

Add `Sparkles` to the `lucide-react` import.

Add to the props type (next to `commentsOpen: boolean;`):
```tsx
agentOpen: boolean;
onToggleAgent: () => void;
```

Destructure in the function signature (next to `commentsOpen,`):
```tsx
agentOpen,
onToggleAgent,
```

After the existing `config` query line (~line 90), add:
```tsx
const agentEnabled = config.data?.agent_enabled ?? false;
```

Add Sparkles button immediately before the comments toggle button:
```tsx
{agentEnabled && (
  <Tooltip tooltip={agentOpen ? false : "Ask AI"}>
    <Button
      variant="ghost"
      size="icon"
      onClick={onToggleAgent}
      aria-label={agentOpen ? "Hide AI panel" : "Ask AI"}
      aria-pressed={agentOpen}
    >
      <Sparkles className="h-4 w-4" />
    </Button>
  </Tooltip>
)}
```

- [ ] **Step 3: Verify TypeScript compiles**

```bash
cd frontend && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 4: Run frontend tests**

```bash
cd frontend && npm test -- --run
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/pages/doc-page.tsx frontend/src/pages/doc-bar.tsx
git commit -m "feat: wire AgentPanel into doc page with Sparkles toggle in toolbar"
```
