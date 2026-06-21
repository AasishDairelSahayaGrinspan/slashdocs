# Google Drive "Open with" → aidocs Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a user open an HTML file from Google Drive in aidocs ("Open with → aidocs"); the file stays in Drive, aidocs renders it live in its sandboxed viewer and layers its own comments on top.

**Architecture:** aidocs registers as an internal Workspace Drive app. The Drive UI "Open URL" hits `GET /drive/open`, which upserts a *Drive-backed document* (a row storing the Drive `fileId`, no HTML, no versions) and redirects to the normal doc page. On that page the **browser** obtains a short-lived `drive.readonly` token via Google Identity Services, fetches the file bytes directly from Drive, and POSTs them to a server endpoint that wraps them in the existing sandbox+`bridge.js` and returns the render page. Access is gated entirely by Drive (per-viewer); no Google tokens are ever stored server-side.

**Tech Stack:** Go (gin, sqlc, goose-style numbered SQL migrations), Postgres + in-memory repo, React 19 + react-router 7 + @tanstack/react-query 5 + Vite, Google Identity Services (GIS) browser token client.

## Global Constraints

- Distribution is **razorpay.com Workspace only** — admin-trusted app, no public OAuth verification. Code must not assume a public consent screen.
- Drive-backed docs store **no blob and no versions**. `current_version_id` is NULL for them.
- For Drive docs, **Drive ACL is the sole access gate**. aidocs grants are inert; do not add grant checks that would block a Drive viewer.
- **No Google refresh/access tokens are persisted server-side**, ever. The Drive token lives only in the browser.
- Spec: `docs/superpowers/specs/2026-06-21-google-drive-open-with-design.md`. Every task's requirements implicitly include this section.
- Repository changes must land in **both** implementations: `api/internal/repo/repo.go` (`Memory`) and `api/internal/repo/postgres/postgres.go` (`Store`), plus the `Repository` interface (`api/internal/repo/repo.go:145+`).
- Run backend tests with `go test ./...` from `api/`. Run frontend with `npm test` (vitest) / `npm run build` from `frontend/`.

---

## Task 1: Data model — Drive-backed document columns

**Files:**
- Create: `api/internal/db/migrations/00011_drive_documents.sql`
- Modify: `api/internal/repo/repo.go:70-75` (the `Document` struct)
- Test: `api/internal/repo/postgres/postgres_test.go` (new test in existing suite)

**Interfaces:**
- Produces: `Document` gains `Source string` (`"native"`|`"drive"`), `DriveFileID string` (empty for native), `DriveResourceKey string`. New constants `repo.SourceNative = "native"`, `repo.SourceDrive = "drive"`.

- [ ] **Step 1: Write the migration**

```sql
-- api/internal/db/migrations/00011_drive_documents.sql
ALTER TABLE documents
  ADD COLUMN source TEXT NOT NULL DEFAULT 'native',
  ADD COLUMN drive_file_id TEXT,
  ADD COLUMN drive_resource_key TEXT;

-- One aidocs document per Drive file, so re-opening reuses its comments.
CREATE UNIQUE INDEX documents_drive_file_id_key
  ON documents (drive_file_id) WHERE drive_file_id IS NOT NULL;
```

- [ ] **Step 2: Add struct fields + constants**

In `api/internal/repo/repo.go`, extend the struct (keep existing fields):

```go
const (
	SourceNative = "native"
	SourceDrive  = "drive"
)

type Document struct {
	ID               string         `json:"id"`
	Title            string         `json:"title"`
	Owner            auth.Principal `json:"owner"`
	CurrentVersionID string         `json:"current_version_id"`
	Source           string         `json:"source"`
	DriveFileID      string         `json:"drive_file_id,omitempty"`
	DriveResourceKey string         `json:"drive_resource_key,omitempty"`
}
```

- [ ] **Step 3: Confirm migration applies**

Run (from `api/`): `go test ./internal/repo/postgres/... -run TestMigrations -v` if such a test exists; otherwise the migrate-on-connect path runs in the postgres suite setup. Expected: suite boots without migration error. (Postgres tests are skipped automatically when `TEST_DATABASE_URL` is unset — that is fine; the SQL is verified by Task 2's CRUD test when a DB is available.)

- [ ] **Step 4: Commit**

```bash
git add api/internal/db/migrations/00011_drive_documents.sql api/internal/repo/repo.go
git commit -m "feat(db): add drive-backed document columns"
```

---

## Task 2: Repository — UpsertDriveDocument

**Files:**
- Modify: `api/internal/repo/repo.go` (interface ~line 152, `Memory` impl near `CreateDocument` ~line 343, and all places that build a `Document` to set `Source`)
- Modify: `api/internal/repo/postgres/postgres.go` (near `CreateDocument` ~line 197; and the row→`Document` mappers)
- Modify: `api/internal/db/queries/*.sql` + regenerate sqlc (`api/internal/db/sqlc/`)
- Test: `api/internal/repo/repo_test.go` (Memory) and `api/internal/repo/postgres/postgres_test.go`

**Interfaces:**
- Produces on `Repository`:
  `UpsertDriveDocument(ctx context.Context, owner auth.Principal, fileID, resourceKey, title string) (Document, error)` — returns the existing Drive doc for `fileID` if one exists (updating title), else creates one with `Source="drive"`, `CurrentVersionID=""`, the given owner.
- Consumes: `auth.Principal`, existing ID-generation pattern used by `CreateDocument`.

- [ ] **Step 1: Write the failing Memory test**

```go
// api/internal/repo/repo_test.go
func TestUpsertDriveDocument_DedupesByFileID(t *testing.T) {
	m := repo.NewMemory()
	ctx := context.Background()
	owner := auth.Principal{Type: "user", ID: "u1"}

	d1, err := m.UpsertDriveDocument(ctx, owner, "file-abc", "", "Report")
	if err != nil {
		t.Fatal(err)
	}
	if d1.Source != repo.SourceDrive || d1.DriveFileID != "file-abc" || d1.CurrentVersionID != "" {
		t.Fatalf("unexpected drive doc: %+v", d1)
	}

	d2, err := m.UpsertDriveDocument(ctx, owner, "file-abc", "", "Report v2")
	if err != nil {
		t.Fatal(err)
	}
	if d2.ID != d1.ID {
		t.Fatalf("expected same doc id on re-open, got %s vs %s", d2.ID, d1.ID)
	}
	if d2.Title != "Report v2" {
		t.Fatalf("expected title updated, got %q", d2.Title)
	}
}
```

- [ ] **Step 2: Run it — expect FAIL**

Run: `cd api && go test ./internal/repo/ -run TestUpsertDriveDocument -v`
Expected: FAIL — `m.UpsertDriveDocument undefined`.

- [ ] **Step 3: Add to the `Repository` interface**

In `api/internal/repo/repo.go`, in the interface block (after `CreateDocument`):

```go
	UpsertDriveDocument(ctx context.Context, owner auth.Principal, fileID, resourceKey, title string) (Document, error)
```

- [ ] **Step 4: Implement on `Memory`**

Mirror `Memory.CreateDocument` (repo.go:343) for ID generation. Add near it:

```go
func (m *Memory) UpsertDriveDocument(ctx context.Context, owner auth.Principal, fileID, resourceKey, title string) (Document, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, d := range m.docs {
		if d.Source == SourceDrive && d.DriveFileID == fileID {
			d.Title = title
			d.DriveResourceKey = resourceKey
			m.docs[id] = d
			return d, nil
		}
	}
	m.docN++
	id := fmt.Sprintf("doc_%d", m.docN) // match the existing CreateDocument id scheme
	d := Document{
		ID:               id,
		Title:            title,
		Owner:            owner,
		Source:           SourceDrive,
		DriveFileID:      fileID,
		DriveResourceKey: resourceKey,
	}
	m.docs[id] = d
	return d, nil
}
```
**Note:** open `CreateDocument` first and copy its exact id-generation line (e.g. it may use a helper, not `fmt.Sprintf`). Also set `Source = SourceNative` inside `CreateDocument` so native docs report a source.

- [ ] **Step 5: Run Memory test — expect PASS**

Run: `cd api && go test ./internal/repo/ -run TestUpsertDriveDocument -v`
Expected: PASS.

- [ ] **Step 6: Add the sqlc query + regenerate**

Find the `.sql` query file holding `CreateDocument`/`GetDocument` (under `api/internal/db/queries/`). Add:

```sql
-- name: UpsertDriveDocument :one
INSERT INTO documents (id, title, owner_user_id, owner_service_account_id, source, drive_file_id, drive_resource_key)
VALUES ($1, $2, $3, $4, 'drive', $5, $6)
ON CONFLICT (drive_file_id) WHERE drive_file_id IS NOT NULL
DO UPDATE SET title = EXCLUDED.title, drive_resource_key = EXCLUDED.drive_resource_key
RETURNING *;
```
Match the **actual owner columns** used by the existing `CreateDocument` query (inspect it — owner may be split into user vs service-account columns). Add `source`, `drive_file_id`, `drive_resource_key` to the `SELECT` lists of existing `GetDocument`/`ListDocuments`/`SearchDocumentsByTitle` queries so the columns round-trip.

Regenerate: `cd api && sqlc generate` (config: `sqlc.yaml`). Expected: `api/internal/db/sqlc/*.sql.go` updated, compiles.

- [ ] **Step 7: Implement on `postgres.Store`**

Add `Store.UpsertDriveDocument` mirroring `Store.CreateDocument` (postgres.go:197) — generate the id the same way, call the new sqlc method, map the row to `repo.Document` (set `Source`, `DriveFileID`, `DriveResourceKey`). Update the existing row→`Document` mapper(s) to populate the three new fields for all reads.

- [ ] **Step 8: Postgres CRUD test (runs only with TEST_DATABASE_URL)**

```go
// api/internal/repo/postgres/postgres_test.go — add within the existing suite
func TestStore_UpsertDriveDocument(t *testing.T) {
	store, cleanup := newTestStore(t) // use the suite's existing helper name
	defer cleanup()
	ctx := context.Background()
	owner := auth.Principal{Type: "user", ID: seedUserID(t, store)}

	d1, err := store.UpsertDriveDocument(ctx, owner, "drv-1", "rk-1", "Spec")
	if err != nil { t.Fatal(err) }
	d2, err := store.UpsertDriveDocument(ctx, owner, "drv-1", "rk-1", "Spec 2")
	if err != nil { t.Fatal(err) }
	if d1.ID != d2.ID { t.Fatalf("expected dedupe by file id") }
	if d2.Title != "Spec 2" { t.Fatalf("expected title update") }

	got, err := store.GetDocument(ctx, d1.ID)
	if err != nil { t.Fatal(err) }
	if got.Source != repo.SourceDrive || got.DriveFileID != "drv-1" {
		t.Fatalf("drive fields not round-tripped: %+v", got)
	}
}
```
Use the suite's real store/seed helpers (read the top of `postgres_test.go` for their names; the lines above are illustrative).

- [ ] **Step 9: Run full repo tests**

Run: `cd api && go test ./internal/repo/...`
Expected: PASS (postgres test skips cleanly if `TEST_DATABASE_URL` unset).

- [ ] **Step 10: Commit**

```bash
git add api/internal/repo api/internal/db
git commit -m "feat(repo): UpsertDriveDocument across memory and postgres"
```

---

## Task 3: OAuth — add Drive scopes

**Files:**
- Modify: `api/internal/auth/oauth.go:21`
- Test: `api/internal/auth/auth_test.go`

**Interfaces:**
- Produces: OAuth config requests `drive.install` + `drive.readonly` in addition to `openid email profile`.

- [ ] **Step 1: Write the failing test**

```go
// api/internal/auth/auth_test.go
func TestNewGoogleOAuth_RequestsDriveScopes(t *testing.T) {
	g := auth.NewGoogleOAuth("cid", "secret", "https://app/cb")
	want := map[string]bool{
		"openid": true, "email": true, "profile": true,
		"https://www.googleapis.com/auth/drive.install":  true,
		"https://www.googleapis.com/auth/drive.readonly": true,
	}
	for _, s := range g.Config.Scopes {
		delete(want, s)
	}
	if len(want) != 0 {
		t.Fatalf("missing scopes: %v", want)
	}
}
```

- [ ] **Step 2: Run it — expect FAIL**

Run: `cd api && go test ./internal/auth/ -run TestNewGoogleOAuth_RequestsDriveScopes -v`
Expected: FAIL (drive scopes missing).

- [ ] **Step 3: Add the scopes**

`api/internal/auth/oauth.go:21` — change the `Scopes` slice:

```go
Scopes: []string{
	"openid", "email", "profile",
	"https://www.googleapis.com/auth/drive.install",
	"https://www.googleapis.com/auth/drive.readonly",
},
```

- [ ] **Step 4: Run it — expect PASS**

Run: `cd api && go test ./internal/auth/ -run TestNewGoogleOAuth_RequestsDriveScopes -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/internal/auth/oauth.go api/internal/auth/auth_test.go
git commit -m "feat(auth): request Drive install + readonly scopes"
```

---

## Task 4: Backend — GET /drive/open handler

**Files:**
- Create: `api/internal/server/handlers_drive.go`
- Modify: `api/internal/server/server.go` (register route)
- Create: `api/internal/server/drive_state.go` (state parser — pure, easy to test)
- Test: `api/internal/server/drive_state_test.go`, and a handler test in `api/internal/server/server_test.go` style.

**Interfaces:**
- Produces: `parseDriveState(raw string) (driveOpenState, error)` where
  `type driveOpenState struct { IDs []string; Action string; UserID string; ResourceKeys map[string]string }`.
- Route: `GET /drive/open?state=<urlencoded-json>` → 302 redirect to `/app/d/<docID>` (or to login with a return-to if no session).

- [ ] **Step 1: Write the failing state-parser test**

```go
// api/internal/server/drive_state_test.go
package server

import "testing"

func TestParseDriveState(t *testing.T) {
	raw := `%7B%22ids%22%3A%5B%22FILE1%22%5D%2C%22action%22%3A%22open%22%2C%22userId%22%3A%22112233%22%7D`
	st, err := parseDriveState(raw)
	if err != nil {
		t.Fatal(err)
	}
	if st.Action != "open" || len(st.IDs) != 1 || st.IDs[0] != "FILE1" || st.UserID != "112233" {
		t.Fatalf("bad parse: %+v", st)
	}
}

func TestParseDriveState_Rejects(t *testing.T) {
	for _, raw := range []string{"", "not-json", `%7B%22action%22%3A%22open%22%7D`} { // missing ids
		if _, err := parseDriveState(raw); err == nil {
			t.Fatalf("expected error for %q", raw)
		}
	}
}
```

- [ ] **Step 2: Run it — expect FAIL**

Run: `cd api && go test ./internal/server/ -run TestParseDriveState -v`
Expected: FAIL — `parseDriveState undefined`.

- [ ] **Step 3: Implement the parser**

```go
// api/internal/server/drive_state.go
package server

import (
	"encoding/json"
	"errors"
	"net/url"
)

type driveOpenState struct {
	IDs          []string          `json:"ids"`
	Action       string            `json:"action"`
	UserID       string            `json:"userId"`
	ResourceKeys map[string]string `json:"resourceKeys"`
}

func parseDriveState(raw string) (driveOpenState, error) {
	if raw == "" {
		return driveOpenState{}, errors.New("empty state")
	}
	decoded, err := url.QueryUnescape(raw)
	if err != nil {
		return driveOpenState{}, err
	}
	var st driveOpenState
	if err := json.Unmarshal([]byte(decoded), &st); err != nil {
		return driveOpenState{}, err
	}
	if len(st.IDs) == 0 || st.IDs[0] == "" {
		return driveOpenState{}, errors.New("state missing file id")
	}
	return st, nil
}
```

- [ ] **Step 4: Run it — expect PASS**

Run: `cd api && go test ./internal/server/ -run TestParseDriveState -v`
Expected: PASS.

- [ ] **Step 5: Implement the handler**

```go
// api/internal/server/handlers_drive.go
package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// driveOpen is the Drive UI "Open with" entry point. Google redirects the user
// here with a URL-encoded JSON `state` param identifying the opened file.
func (h handlers) driveOpen(c *gin.Context) {
	st, err := parseDriveState(c.Query("state"))
	if err != nil || st.Action != "open" {
		badRequest(c, "invalid drive open request")
		return
	}
	// Require an aidocs session. current(c) yields the principal; if unauthenticated
	// the authed middleware would have rejected — but /drive/open is public, so
	// bounce to login with a return-to that replays this exact URL.
	p := current(c)
	if p == nil {
		login := "/v1/auth/google/start?mode=web&redirect=" +
			"/drive/open%3Fstate%3D" + c.Query("state") // re-encode handled below
		c.Redirect(http.StatusFound, login)
		return
	}
	fileID := st.IDs[0]
	rk := st.ResourceKeys[fileID]
	doc, err := h.deps.repository.UpsertDriveDocument(c.Request.Context(), *p, fileID, rk, "Drive document")
	if err != nil {
		internalErr(c, err)
		return
	}
	c.Redirect(http.StatusFound, "/app/d/"+doc.ID)
}
```
**Implementer notes:**
- Confirm how `current(c)` behaves on a public route and how existing code builds the login redirect (see `api.loginURL` consumer / `handlers_auth.go` `googleStart`, which reads `redirect`). Build the `redirect` value with `url.Values` so the nested `state` is correctly escaped — do not hand-concatenate. The replayed target must be `/drive/open?state=<original raw state>`.
- Title `"Drive document"` is a placeholder filename; Task 8's frontend can PATCH the real title once it reads Drive metadata, or a later iteration can fetch `files.get?fields=name` server-side. For v1 the placeholder is acceptable (the rendered HTML usually carries its own `<title>`).

- [ ] **Step 6: Register the route**

In `api/internal/server/server.go`, next to the other public `s.engine.GET(...)` routes (around line 137-143):

```go
s.engine.GET("/drive/open", h.driveOpen)
```

- [ ] **Step 7: Handler test**

Add a test in the existing server test style that builds the test server (see `testhelpers_test.go`) with an authenticated session, hits `GET /drive/open?state=<encoded>`, and asserts a 302 to `/app/d/...` plus that a Drive doc now exists. Mirror an existing handler test for session setup.

- [ ] **Step 8: Run server tests**

Run: `cd api && go test ./internal/server/...`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add api/internal/server/handlers_drive.go api/internal/server/drive_state.go api/internal/server/drive_state_test.go api/internal/server/server.go
git commit -m "feat(server): handle Drive Open-with entry point"
```

---

## Task 5: Backend — expose Google client ID + render origin to the frontend

**Files:**
- Modify: `api/internal/server/handlers_auth.go:240` (`config` handler)
- Modify: `api/internal/server/server.go` (add `googleClientID` to `dependencies` + `WithGoogleClientID` option if not already threaded; the OAuth client id is already configured for login — reuse it)
- Test: `api/internal/server/server_test.go`

**Interfaces:**
- Produces: `GET /v1/config` JSON gains `google_oauth_client_id` (string) and `render_origin` (string, may be empty meaning same-origin).

- [ ] **Step 1: Write the failing test**

```go
// api/internal/server/server_test.go
func TestConfig_ExposesGoogleClientID(t *testing.T) {
	srv := newTestServer(t, WithGoogleClientID("cid.apps.googleusercontent.com")) // use real option name
	rec := doGET(t, srv, "/v1/config")
	var body map[string]any
	mustJSON(t, rec, &body)
	if body["google_oauth_client_id"] != "cid.apps.googleusercontent.com" {
		t.Fatalf("client id not exposed: %v", body["google_oauth_client_id"])
	}
}
```
Use the test helpers that already exist (`newTestServer`, request helper). Read `testhelpers_test.go` for exact names.

- [ ] **Step 2: Run it — expect FAIL**

Run: `cd api && go test ./internal/server/ -run TestConfig_ExposesGoogleClientID -v`
Expected: FAIL (key absent).

- [ ] **Step 3: Thread the client id + extend config**

The Google client id already exists for OAuth (passed to `NewGoogleOAuth`). Store it on `dependencies` (add field `googleClientID string` and, if no setter exists, `func WithGoogleClientID(id string) Option`). Wire it in `cmd/aidocs-server/main.go` from the same env var used for OAuth. Then extend the handler:

```go
c.JSON(http.StatusOK, gin.H{
	"deployment":              deployment,
	"org_name":                h.deps.orgName,
	"everyone_label":          everyoneLabel(deployment, h.deps.orgName),
	"google_oauth_client_id":  h.deps.googleClientID,
	"render_origin":           h.deps.renderOrigin,
})
```

- [ ] **Step 4: Run it — expect PASS**

Run: `cd api && go test ./internal/server/ -run TestConfig_ExposesGoogleClientID -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/internal/server/handlers_auth.go api/internal/server/server.go api/cmd/aidocs-server/main.go
git commit -m "feat(server): expose google client id and render origin in /config"
```

---

## Task 6: Backend — render-wrap endpoint for client-fetched Drive HTML

**Files:**
- Modify: `api/internal/server/handlers_drive.go` (add `driveRender`)
- Modify: `api/internal/server/server.go` (register `POST /drive/render`)
- Test: `api/internal/server/render_internal_test.go` (a wrapper assertion) + a handler test.

**Interfaces:**
- Consumes: `renderWrapperHTML(userHTML []byte, appOrigin string) []byte` (render.go:30), `current(c)`, `h.deps.repository.GetDocument`.
- Produces: `POST /drive/render` — authed (session); body `{ "document_id": "...", "html": "<...>" }`; verifies the doc exists and `Source == SourceDrive`; returns the sandbox wrapper page (same CSP/headers as `renderVersion`). The posted HTML is **never persisted**.

- [ ] **Step 1: Write the failing handler test**

```go
// api/internal/server/server_test.go (or handlers_drive_test.go)
func TestDriveRender_WrapsHTML(t *testing.T) {
	srv, repo := newTestServerWithRepo(t) // helper that returns the in-memory repo too
	owner := seedSession(t, srv)          // existing helper: logs in a user, returns principal/cookie
	doc, _ := repo.UpsertDriveDocument(context.Background(), owner, "f1", "", "t")

	body := `{"document_id":"` + doc.ID + `","html":"<h1>hi</h1>"}`
	rec := doPOST(t, srv, "/drive/render", body /*, with session cookie */)
	if rec.Code != 200 {
		t.Fatalf("got %d", rec.Code)
	}
	out := rec.Body.String()
	if !strings.Contains(out, "<iframe id=\"aidocs-doc\"") || !strings.Contains(out, "&lt;h1&gt;hi&lt;/h1&gt;") {
		t.Fatalf("wrapper or escaped html missing: %s", out[:200])
	}
}
```
Adapt helper names to those in `testhelpers_test.go`.

- [ ] **Step 2: Run it — expect FAIL**

Run: `cd api && go test ./internal/server/ -run TestDriveRender_WrapsHTML -v`
Expected: FAIL — route 404 / handler undefined.

- [ ] **Step 3: Implement the handler**

```go
// add to api/internal/server/handlers_drive.go
func (h handlers) driveRender(c *gin.Context) {
	p := current(c)
	if p == nil {
		c.JSON(http.StatusUnauthorized, errorResponse("unauthorized", "login required", nil))
		return
	}
	var in struct {
		DocumentID string `json:"document_id"`
		HTML       string `json:"html"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || in.DocumentID == "" {
		badRequest(c, "invalid render request")
		return
	}
	doc, err := h.deps.repository.GetDocument(c.Request.Context(), in.DocumentID)
	if err != nil || doc.Source != repo.SourceDrive {
		notFound(c)
		return
	}
	appOrigin := h.deps.appOrigin
	if appOrigin == "" {
		appOrigin = "'self'"
	}
	// Same CSP/headers as renderVersion (handlers_versions.go) — keep them identical.
	c.Header("Content-Security-Policy", "default-src 'none'; img-src * data: blob:; media-src * data: blob:; font-src * data: blob:; style-src * 'unsafe-inline'; script-src * 'unsafe-inline' 'unsafe-eval'; connect-src *; frame-src *; worker-src * blob:; object-src 'none'; frame-ancestors "+appOrigin)
	c.Header("X-Content-Type-Options", "nosniff")
	observeHTML("drive_render", len(in.HTML))
	c.Data(http.StatusOK, "text/html; charset=utf-8", renderWrapperHTML([]byte(in.HTML), appOrigin))
}
```
Add the `repo` import if not present. **Access note:** we intentionally do *not* call `RoleForDocument` — a session user who can fetch the bytes from Drive is authorized to view; aidocs grants are inert for Drive docs (Global Constraints). Requiring a valid session prevents this from being an open HTML-wrapping proxy.

- [ ] **Step 4: Register the route**

In `server.go`, on the **authed** group (so a session is required) — mirror line 171's `authed.POST("/documents", ...)`. The route lives outside `/v1` to match `/drive/open`; if the authed group is under `/v1`, register a sibling authed handler. Simplest: register under the authed group and call it `POST /v1/drive/render`. Pick one path and keep the frontend (Task 8) in sync. Recommended: `authed.POST("/drive/render", h.driveRender)` under `/v1`.

> **ponytail: same-origin render only for v1.** The wrapper is returned from the app origin. A separately-configured `renderOrigin` would require CORS on this POST (the browser fetch is cross-origin with credentials). Defer CORS until a distinct `renderOrigin` is actually deployed — add `connect-src`/`Access-Control-Allow-Origin` then.

- [ ] **Step 5: Run it — expect PASS**

Run: `cd api && go test ./internal/server/ -run TestDriveRender_WrapsHTML -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add api/internal/server/handlers_drive.go api/internal/server/server.go api/internal/server/server_test.go
git commit -m "feat(server): wrap client-fetched Drive HTML in sandbox renderer"
```

---

## Task 7: Frontend — config hook + types

**Files:**
- Modify: `frontend/src/api.ts` (extend `ServerConfig`, ensure `getConfig` returns it; extend `Document` type with `source`/`drive_file_id`)
- Create: `frontend/src/lib/useConfig.ts`
- Test: `frontend/src/lib/useConfig.test.tsx` (vitest + React Query wrapper) — optional if the suite lacks a render harness; otherwise a type-level test.

**Interfaces:**
- Produces: `useConfig()` → React Query result of `ServerConfig` (cached `staleTime: Infinity`, key `["config"]`). `ServerConfig` gains `google_oauth_client_id: string` and `render_origin: string`. `Document` type gains `source: "native" | "drive"` and `drive_file_id?: string`.

- [ ] **Step 1: Extend types**

In `frontend/src/api.ts`:

```ts
export type ServerConfig = {
  deployment: "public" | "org";
  org_name: string;
  everyone_label: string;
  google_oauth_client_id: string;
  render_origin: string;
};
```
And add to the `Document` type (find its declaration): `source: "native" | "drive"; drive_file_id?: string;`.

- [ ] **Step 2: Add the hook**

```ts
// frontend/src/lib/useConfig.ts
import { useQuery } from "@tanstack/react-query";
import { api } from "@/api";

export function useConfig() {
  return useQuery({ queryKey: ["config"], queryFn: api.getConfig, staleTime: Infinity });
}
```
Refactor `doc-share-sheet.tsx` and `start.tsx` to use `useConfig()` instead of their inline `useQuery(["config"], ...)` (DRY — they already use the same key, so cache is shared regardless; this is a tidy-up, do it in this task only if quick).

- [ ] **Step 3: Build the frontend**

Run: `cd frontend && npm run build`
Expected: type-checks and builds with the new fields.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/api.ts frontend/src/lib/useConfig.ts
git commit -m "feat(frontend): config hook with google client id; document source field"
```

---

## Task 8: Frontend — GIS Drive token + file fetch

**Files:**
- Modify: `frontend/index.html` (load GIS script)
- Create: `frontend/src/lib/drive.ts` (token client + fetch helpers)
- Test: `frontend/src/lib/drive.test.ts` (vitest; mock `window.google` + `fetch`)

**Interfaces:**
- Produces:
  - `getDriveAccessToken(clientId: string): Promise<string>` — resolves a short-lived `drive.readonly` token via GIS token client.
  - `fetchDriveFile(fileId: string, token: string, resourceKey?: string): Promise<string>` — returns HTML text; throws `DriveAccessError` (with `.status`) on 403/404.

- [ ] **Step 1: Load GIS in index.html**

Add to `frontend/index.html` `<head>` (mirroring the existing `__AIDOCS_PUBLIC_URL__` script block placement):

```html
<script src="https://accounts.google.com/gsi/client" async defer></script>
```

- [ ] **Step 2: Write the failing test**

```ts
// frontend/src/lib/drive.test.ts
import { describe, it, expect, vi } from "vitest";
import { fetchDriveFile, DriveAccessError } from "./drive";

describe("fetchDriveFile", () => {
  it("returns HTML on 200", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(
      new Response("<h1>ok</h1>", { status: 200 }),
    ));
    expect(await fetchDriveFile("f1", "tok")).toBe("<h1>ok</h1>");
  });
  it("throws DriveAccessError with status on 403", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("", { status: 403 })));
    await expect(fetchDriveFile("f1", "tok")).rejects.toMatchObject({ status: 403 });
  });
});
```

- [ ] **Step 3: Run it — expect FAIL**

Run: `cd frontend && npx vitest run src/lib/drive.test.ts`
Expected: FAIL — module/exports missing.

- [ ] **Step 4: Implement drive.ts**

```ts
// frontend/src/lib/drive.ts
export class DriveAccessError extends Error {
  status: number;
  constructor(status: number) {
    super(status === 403 ? "no-drive-access" : status === 404 ? "drive-file-missing" : `drive-error-${status}`);
    this.status = status;
  }
}

const DRIVE_SCOPE = "https://www.googleapis.com/auth/drive.readonly";

export function getDriveAccessToken(clientId: string): Promise<string> {
  return new Promise((resolve, reject) => {
    const g = (window as any).google;
    if (!g?.accounts?.oauth2) {
      reject(new Error("GIS not loaded"));
      return;
    }
    const client = g.accounts.oauth2.initTokenClient({
      client_id: clientId,
      scope: DRIVE_SCOPE,
      callback: (resp: { access_token?: string; error?: string }) => {
        if (resp.access_token) resolve(resp.access_token);
        else reject(new Error(resp.error || "token-failed"));
      },
    });
    client.requestAccessToken();
  });
}

export async function fetchDriveFile(fileId: string, token: string, resourceKey?: string): Promise<string> {
  const headers: Record<string, string> = { Authorization: `Bearer ${token}` };
  if (resourceKey) headers["X-Goog-Drive-Resource-Keys"] = `${fileId}/${resourceKey}`;
  const res = await fetch(
    `https://www.googleapis.com/drive/v3/files/${encodeURIComponent(fileId)}?alt=media`,
    { headers },
  );
  if (!res.ok) throw new DriveAccessError(res.status);
  return res.text();
}
```

- [ ] **Step 5: Run it — expect PASS**

Run: `cd frontend && npx vitest run src/lib/drive.test.ts`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add frontend/index.html frontend/src/lib/drive.ts frontend/src/lib/drive.test.ts
git commit -m "feat(frontend): GIS drive.readonly token + file fetch helpers"
```

---

## Task 9: Frontend — render Drive-backed docs in the viewer

**Files:**
- Modify: `frontend/src/pages/doc-render-frame.tsx` (`RenderFrame`)
- Modify: `frontend/src/api.ts` (add `driveRender` call)
- Modify: `frontend/src/pages/doc-page.tsx` (render `RenderFrame` for Drive docs even when there is no version)
- Test: manual (end-to-end) + a vitest for the branch selection if feasible.

**Interfaces:**
- Consumes: `useConfig()`, `getDriveAccessToken`, `fetchDriveFile`, `api.driveRender`.
- Produces: when `doc.source === "drive"`, `RenderFrame` fetches the file client-side, POSTs to `/v1/drive/render`, and sets the returned wrapper HTML as the iframe `srcdoc`.

- [ ] **Step 1: Add the API call**

In `frontend/src/api.ts` `api` object:

```ts
driveRender: (documentId: string, html: string) =>
  request<string>("/v1/drive/render", {
    method: "POST",
    body: JSON.stringify({ document_id: documentId, html }),
  }),
```
(`request` returns text when the response isn't JSON — confirmed in api.ts.)

- [ ] **Step 2: Branch RenderFrame on source**

In `doc-render-frame.tsx`, when `doc.source === "drive"`, skip the render-token query and instead run a query that:
1. `const cfg = useConfig()` → `google_oauth_client_id`.
2. `getDriveAccessToken(clientId)` → token.
3. `fetchDriveFile(doc.drive_file_id!, token, doc.drive_resource_key)` → html.
4. `api.driveRender(doc.id, html)` → wrapper string.
5. Set `<iframe srcdoc={wrapper} sandbox="allow-scripts allow-same-origin allow-popups allow-popups-to-escape-sandbox">` (drop `src`).

Concretely:

```tsx
const cfg = useConfig();
const driveQuery = useQuery({
  queryKey: ["drive-render", doc.id],
  enabled: doc.source === "drive" && !!cfg.data?.google_oauth_client_id,
  staleTime: 60_000,
  queryFn: async () => {
    const token = await getDriveAccessToken(cfg.data!.google_oauth_client_id);
    const html = await fetchDriveFile(doc.drive_file_id!, token, doc.drive_resource_key);
    return api.driveRender(doc.id, html);
  },
});
```
Render: if `doc.source === "drive"`, use `srcdoc={driveQuery.data}`; handle `driveQuery.error instanceof DriveAccessError` with status-specific copy ("You don't have access to this file in Google Drive." / "This file no longer exists in Drive."). Otherwise keep the existing token/`src` path unchanged.

**postMessage note:** with `srcdoc`, the inner content's origin is the embedding (app) origin (since `allow-same-origin` is set). The existing `onMsg` validates against `frameOrigin` parsed from the token URL — for the Drive path there is no token URL, so set `frameOrigin` to `window.location.origin` when `doc.source === "drive"`, and target `paint`/`theme` postMessages at `"*"` or that same origin. Keep the rest of the bridge wiring identical so comments work.

- [ ] **Step 3: Show the frame for version-less Drive docs**

In `doc-page.tsx`, the frame currently renders only when `current` (a version id) is truthy. Change the condition to also render when `doc.data?.source === "drive"` (which has no version). Pass the `doc` object to `RenderFrame` so it can branch.

- [ ] **Step 4: Build**

Run: `cd frontend && npm run build`
Expected: builds clean.

- [ ] **Step 5: Manual end-to-end (documented, run by a human)**

1. Configure Google Cloud (Task 10) in a test Razorpay Workspace.
2. Upload an `.html` file to Drive → right-click → Open with → aidocs.
3. Verify: redirected through aidocs login (if needed) → lands on `/app/d/...` → GIS consent (first time) → file renders in the sandbox.
4. Add a comment; reload; comment persists. Re-open the same file from Drive → same doc, same comment.
5. Open as a user without Drive access → "no access" state.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/pages/doc-render-frame.tsx frontend/src/pages/doc-page.tsx frontend/src/api.ts
git commit -m "feat(frontend): render Drive-backed documents via client-side fetch"
```

---

## Task 10: Operational setup (no code) — Drive UI integration + Marketplace

**Files:** Create `docs/google-drive-setup.md` capturing the steps so rollout is reproducible.

**Interfaces:** none (Google Cloud Console + Workspace admin actions).

- [ ] **Step 1: Document and perform the console setup**

Write `docs/google-drive-setup.md` covering:
1. In the GCP project backing aidocs OAuth: enable **Google Drive API**.
2. Open **Drive UI integration**: set app name/icon, **Open URL** = `https://<aidocs-domain>/drive/open`, default MIME type `text/html`, file extension `html`/`htm`.
3. OAuth consent screen: app type **Internal** (razorpay.com), add scopes `drive.install`, `drive.readonly`.
4. Authorized JavaScript origins for the OAuth client must include the aidocs app origin (GIS token client requirement).
5. Publish a **Google Workspace Marketplace** listing scoped to the domain; have a Workspace admin install it domain-wide (pre-authorizes `drive.readonly`, so end users skip the consent screen).
6. Record the env vars aidocs needs: existing Google client id/secret (now also used by the frontend via `/v1/config`), `APP_ORIGIN`.

- [ ] **Step 2: Commit**

```bash
git add docs/google-drive-setup.md
git commit -m "docs: Google Drive Open-with operational setup"
```

---

## Self-Review

**Spec coverage:**
- §1 Open-with entry point → Task 4 (`/drive/open`) + Task 10 (registration). ✓
- §2 Data model → Task 1 (columns) + Task 2 (upsert/dedupe). ✓
- §3 OAuth scopes → Task 3. ✓
- §4 Render path (client fetch + reuse sandbox/bridge) → Task 6 (wrap endpoint) + Task 8 (GIS/fetch) + Task 9 (frame). ✓
- §5 Error handling → Task 8 `DriveAccessError`, Task 9 status copy, Task 4 state validation. ✓
- §6 Out of scope → respected (no "New" URL, no export, no caching, no version snapshots). ✓
- §7 Testing → unit tests in Tasks 1,2,3,4,6,8; manual e2e in Task 9. ✓
- §8 Operational prerequisites → Task 10. ✓

**Type consistency:** `UpsertDriveDocument(ctx, owner, fileID, resourceKey, title)` used identically in Tasks 2/4/6 tests. `Document.Source/DriveFileID/DriveResourceKey` consistent across backend (Task 1) and frontend (`source`/`drive_file_id`, Task 7). `driveRender(documentId, html)` matches backend body `{document_id, html}` (Tasks 6/9). `ServerConfig.google_oauth_client_id` matches backend `/config` key (Tasks 5/7).

**Known soft spots flagged inline (not placeholders):** exact id-generation call in `CreateDocument`, owner column names in the sqlc query, and test-helper names in `*_test.go` must be read from the codebase by the implementer — each is called out at its step.
