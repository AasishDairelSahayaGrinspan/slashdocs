# Agent Panel Design

**Date:** 2026-09-24  
**Author:** Archit Agarwal  
**Status:** Draft

---

## Intent

Add a Google Docs–style AI panel to the document viewer. Users can ask questions about the document and request a summary without leaving the page. The agent has full document context and runs through Razorpay's internal LiteLLM gateway.

**Success criteria:**
- Panel slides in from the right, same as the comments panel
- "Summarize" and "Key takeaways" quick-action chips work out of the box
- Feature is silently absent when `AIDOCS_AGENT_BASE_URL` is unset (no broken UI)
- Agent responses only if the caller already has read access to the document

---

## Architecture

### Backend — `handlers_agent.go`

New route registered in `server.go` routes block:

```
POST /v1/documents/{id}/agent/chat
```

**Request body:**
```json
{
  "messages": [
    {"role": "user", "content": "Summarize this document"}
  ],
  "version_id": "optional-version-id"
}
```

**Handler steps:**
1. Authenticate via existing middleware (same as all `/v1/documents/{id}/*` routes).
2. Authorize: caller must have read access to the document (existing `requireDocumentAccess` pattern).
3. Resolve version: use `version_id` from request body if supplied, otherwise the document's `current_version_id`.
4. Fetch HTML: `repo.GetVersionHTML(ctx, versionID)` — already exists.
5. Strip HTML to plaintext with `golang.org/x/net/html` (already a transitive dep via `go.sum`).
6. Truncate plaintext to 60 000 characters to stay within context limits.
7. Build LiteLLM request:
   - System prompt: `"You are a helpful assistant. The document the user is asking about follows.\n\n<document>\n{plaintext}\n</document>"`
   - Messages: append request body messages verbatim.
   - Model: `AIDOCS_AGENT_MODEL` (default `"gpt-4o"` to match LiteLLM convention).
8. Stream response via SSE (`text/event-stream`). Each chunk: `data: {text}\n\n`. Final event: `data: [DONE]\n\n`.
9. Error: if LiteLLM returns non-2xx, return `502 Bad Gateway` with a JSON error body matching the existing error shape.

**Config (all optional; feature disabled when `AIDOCS_AGENT_BASE_URL` is empty):**

| Env var | Default | Purpose |
|---|---|---|
| `AIDOCS_AGENT_BASE_URL` | _(unset)_ | LiteLLM base URL — gates the feature |
| `AIDOCS_AGENT_API_KEY` | _(unset)_ | Bearer token for LiteLLM |
| `AIDOCS_AGENT_MODEL` | `gpt-4o` | Model name passed to LiteLLM |

**Config struct addition in `Config` (`server.go`):**
```go
AgentBaseURL string
AgentAPIKey  string
AgentModel   string
```

**Feature flag exposure:** `GET /v1/config` already returns server config to the frontend. Add `"agent_enabled": true/false` to its response body — derived from `AgentBaseURL != ""`.

### Frontend — `doc-agent.tsx`

New file mirroring `doc-comments.tsx` structure.

**Component:** `<AgentPanel />`

Layout (top to bottom):
- Header: "Ask AI" title + clear-chat icon button
- Quick-action chips (shown only when chat is empty): "Summarize" · "Key takeaways"
- Message thread: alternating user (right-aligned, accent bubble) / assistant (left-aligned, muted bubble) messages. Assistant streams token-by-token.
- Input area: textarea + send button. Send on Enter (Shift+Enter = newline). Disabled while streaming.

State:
- `messages: {role, content, id}[]`
- `streaming: boolean`
- `streamedText: string` (accumulated current assistant turn)

API call: `POST /v1/documents/{id}/agent/chat` with `ReadableStream` / `EventSource`. Appends chunks to `streamedText`; on `[DONE]` finalizes into `messages`.

### `doc-page.tsx` changes

- Add `agentOpen: boolean` state (default `false`).
- Opening agent panel closes comments: `setCommentsOpen(false)` when `setAgentOpen(true)` and vice versa.
- Second `motion.aside` block (same animation as comments panel, same `w-[360px]`):

```tsx
{agentOpen && !isMobile && (
  <motion.aside key="agent-rail" ...>
    <div className="h-full w-[360px]"><AgentPanel /></div>
  </motion.aside>
)}
```

- Mobile: second `<Sheet>` for agent, same pattern as comments sheet.

### `doc-bar.tsx` changes

- Read `agent_enabled` from `/v1/config` query (already fetched elsewhere or add a minimal hook).
- New icon button (`Sparkles` from lucide-react) beside the existing comments toggle. Hidden when `agent_enabled` is false.
- Props added: `agentOpen: boolean`, `onToggleAgent: () => void`.

---

## Data flow

```
User types question
  → POST /v1/documents/{id}/agent/chat  (with messages[])
  → handler: auth check → GetVersionHTML → strip HTML → call LiteLLM
  → SSE stream back to browser
  → frontend appends tokens to streamedText
  → on [DONE]: commit to messages[]
```

---

## Error handling

| Scenario | Behaviour |
|---|---|
| `AIDOCS_AGENT_BASE_URL` unset | Button hidden; endpoint returns 404 |
| LiteLLM non-2xx | 502 JSON error; frontend shows inline error banner |
| Document has no version | 422; frontend shows "No content to analyze" |
| Stream interrupted mid-response | Frontend shows partial text + error banner |

---

## Testing

**Backend:**
- Unit: `TestAgentChat_NoAccess`, `TestAgentChat_NoAgentBaseURL`, `TestAgentChat_Streams`
- Uses existing `testhelpers_test.go` patterns; mock LiteLLM with `httptest.Server`

**Frontend:**
- Existing `*.test.tsx` pattern; mock `fetch` for SSE stream
- Test: chips trigger correct message; input disabled during stream; clear resets state

---

## Files touched

| File | Change |
|---|---|
| `api/internal/server/handlers_agent.go` | New |
| `api/internal/server/server.go` | Add `AgentBaseURL/APIKey/Model` to `Config`; register route |
| `api/cmd/aidocs-server/main.go` | Read 3 env vars; pass to `Config` |
| `api/internal/server/handlers_health.go` | Expose `agent_enabled` in `/v1/config` response |
| `frontend/src/pages/doc-agent.tsx` | New |
| `frontend/src/pages/doc-page.tsx` | `agentOpen` state; second aside; mutual exclusion |
| `frontend/src/pages/doc-bar.tsx` | `Sparkles` button; `agentOpen`/`onToggleAgent` props |
| `frontend/src/api.ts` | `agentChat(docId, messages, versionId?)` returning async iterator |
| `.env.example` | Document 3 new env vars |

---

## Out of scope

- Conversation persistence (sessions are in-memory, lost on refresh)
- Selecting text → ask agent about selection (future)
- Agent writing back to the document (future)
