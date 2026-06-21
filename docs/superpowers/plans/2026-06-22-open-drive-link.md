# Open Drive Link (web + CLI) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Let a user open a Drive HTML file by pasting its link — from the web Documents page and from the CLI — reusing the existing Drive-backed document model.

**Architecture:** One authed endpoint `POST /v1/drive/documents {url}` parses the Drive URL server-side (new `parseDriveFileRef`) and calls the existing `UpsertDriveDocument`, returning `{id}`. The web dialog and the CLI command both just send the raw URL. Rendering is unchanged (the existing client-side GIS-fetch render path runs when the doc page loads).

**Tech Stack:** Go (gin), React 19 + react-router 7 + react-query 5, Cobra CLI.

## Global Constraints

- Builds on `docs/superpowers/specs/2026-06-22-open-drive-link-design.md`. Every task's requirements implicitly include this section.
- Reuse the existing `UpsertDriveDocument` and the existing render path — do NOT add a new render route or a new repo method.
- Parsing is server-side ONLY. The CLI and web send the raw URL string; there is no client-side Drive URL parser.
- Drive docs: grants stay inert (Drive ACL gates). The CLI must NOT apply the org-wide-viewer nudge to Drive docs.
- Native Google Docs URLs (`docs.google.com/document|spreadsheets|presentation/...`) must be rejected with a clear 400, not silently treated as a file id.
- Branch: `feat/google-drive-open-with` (extends PR #16). Backend tests run with `-tags testfixtures`.

---

## Task 1: Drive URL parser

**Files:**
- Create: `api/internal/server/drive_url.go`
- Test: `api/internal/server/drive_url_test.go`

**Interfaces:**
- Produces: `parseDriveFileRef(raw string) (fileID string, resourceKey string, err error)` (package-private, `package server`).

- [ ] **Step 1: Write the failing test**

```go
// api/internal/server/drive_url_test.go
package server

import "testing"

func TestParseDriveFileRef(t *testing.T) {
	cases := []struct {
		name, in, wantID, wantRK string
	}{
		{"file-view", "https://drive.google.com/file/d/ABC123/view?usp=sharing", "ABC123", ""},
		{"file-view-resourcekey", "https://drive.google.com/file/d/ABC123/view?resourcekey=RK9", "ABC123", "RK9"},
		{"open-id", "https://drive.google.com/open?id=XYZ789", "XYZ789", ""},
		{"open-id-rk", "https://drive.google.com/open?id=XYZ789&resourcekey=RK1", "XYZ789", "RK1"},
		{"uc", "https://drive.google.com/uc?id=UC456", "UC456", ""},
		{"bare-id", "ABC123", "ABC123", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			id, rk, err := parseDriveFileRef(c.in)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if id != c.wantID || rk != c.wantRK {
				t.Fatalf("got (%q,%q), want (%q,%q)", id, rk, c.wantID, c.wantRK)
			}
		})
	}
}

func TestParseDriveFileRef_Rejects(t *testing.T) {
	for _, in := range []string{
		"",
		"   ",
		"https://docs.google.com/document/d/DOCID/edit",
		"https://docs.google.com/spreadsheets/d/SID/edit",
		"https://docs.google.com/presentation/d/PID/edit",
		"https://example.com/not-a-drive-link",
	} {
		if _, _, err := parseDriveFileRef(in); err == nil {
			t.Fatalf("expected error for %q", in)
		}
	}
}
```

- [ ] **Step 2: Run it — expect FAIL**

Run: `cd api && go test ./internal/server/ -run TestParseDriveFileRef -v`
Expected: FAIL — `parseDriveFileRef` undefined.

- [ ] **Step 3: Implement the parser**

```go
// api/internal/server/drive_url.go
package server

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

// bareIDPattern matches a plausible Drive file id (alphanumeric, dash, underscore).
var bareIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// fileDPathPattern extracts the id from a /file/d/<ID>/... path.
var fileDPathPattern = regexp.MustCompile(`/file/d/([A-Za-z0-9_-]+)`)

// parseDriveFileRef extracts a Drive file id (and optional resource key) from a
// Drive share URL or a bare id. It rejects native Google Docs URLs
// (docs.google.com/document|spreadsheets|presentation), which are not raw HTML
// files, and anything it cannot recognise.
func parseDriveFileRef(raw string) (string, string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", "", errors.New("empty drive link")
	}
	// Bare id (no scheme, no slash): accept as-is.
	if !strings.Contains(s, "/") && bareIDPattern.MatchString(s) {
		return s, "", nil
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", "", errors.New("invalid drive link")
	}
	host := strings.ToLower(u.Hostname())
	if host == "docs.google.com" {
		return "", "", errors.New("this is a Google Docs/Sheets/Slides link, not an HTML file in Drive")
	}
	if host != "drive.google.com" {
		return "", "", errors.New("not a Google Drive link")
	}
	rk := u.Query().Get("resourcekey")
	// drive.google.com/file/d/<ID>/view
	if m := fileDPathPattern.FindStringSubmatch(u.Path); m != nil {
		return m[1], rk, nil
	}
	// drive.google.com/open?id=<ID> or /uc?id=<ID>
	if id := u.Query().Get("id"); id != "" && bareIDPattern.MatchString(id) {
		return id, rk, nil
	}
	return "", "", errors.New("could not find a file id in the drive link")
}
```

- [ ] **Step 4: Run it — expect PASS**

Run: `cd api && go test ./internal/server/ -run TestParseDriveFileRef -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/internal/server/drive_url.go api/internal/server/drive_url_test.go
git commit -m "feat(server): parse Drive file links into file ids"
```

---

## Task 2: POST /v1/drive/documents endpoint + shared helper

**Files:**
- Modify: `api/internal/server/handlers_drive.go` (add `createDriveDocument`; extract a shared upsert helper used by both `driveOpen` and the new handler)
- Modify: `api/internal/server/server.go` (register `authed.POST("/drive/documents", h.createDriveDocument)`)
- Test: `api/internal/server/server_test.go`

**Interfaces:**
- Consumes: `parseDriveFileRef` (Task 1), `h.deps.repository.UpsertDriveDocument`, `current(c)`, `badRequest`, `internalErr`.
- Produces: `POST /v1/drive/documents` (authed) — body `{ "url": "..." }`; on success `201 {"id": "<docID>"}`; invalid/Google-Doc URL → 400 with the parser's message.

- [ ] **Step 1: Write the failing handler test**

```go
// api/internal/server/server_test.go
func TestCreateDriveDocument(t *testing.T) {
	srv, repo := newTestServerWithRepo(t) // use the real helper name from testhelpers_test.go
	_ = repo
	// authenticated POST with a valid Drive link
	rec := doAuthedJSON(t, srv, "POST", "/v1/drive/documents",
		`{"url":"https://drive.google.com/file/d/ABC123/view"}`) // adapt helper to real one
	if rec.Code != 201 {
		t.Fatalf("got %d, body %s", rec.Code, rec.Body.String())
	}
	var m map[string]any
	mustJSON(t, rec, &m)
	if m["id"] == nil || m["id"] == "" {
		t.Fatalf("missing id: %v", m)
	}

	// Google Docs link → 400
	rec2 := doAuthedJSON(t, srv, "POST", "/v1/drive/documents",
		`{"url":"https://docs.google.com/document/d/DOCID/edit"}`)
	if rec2.Code != 400 {
		t.Fatalf("expected 400 for google doc, got %d", rec2.Code)
	}
}
```
Adapt the request/JSON/auth helpers to those actually in `testhelpers_test.go` (e.g. the `X-Test-Principal` header + a JSON POST helper used by other authed handler tests such as `TestDriveRender_WrapsHTML`).

- [ ] **Step 2: Run it — expect FAIL**

Run: `cd api && go test -tags testfixtures ./internal/server/ -run TestCreateDriveDocument -v`
Expected: FAIL — route 404 / handler undefined.

- [ ] **Step 3: Extract the shared helper and add the handler**

In `handlers_drive.go`, factor the upsert step out of `driveOpen` into a helper, then reuse it:

```go
// upsertDriveDoc creates (or dedupe-returns) the Drive-backed document for the
// given file id on behalf of the principal. Shared by driveOpen and
// createDriveDocument so there is one path for minting Drive-backed docs.
func (h handlers) upsertDriveDoc(c *gin.Context, p auth.Principal, fileID, resourceKey string) (repo.Document, error) {
	return h.deps.repository.UpsertDriveDocument(c.Request.Context(), p, fileID, resourceKey, "Drive document")
}

// createDriveDocument registers a Drive-backed document from a pasted Drive
// link. Authed; the body is {"url": "<drive link or file id>"}.
func (h handlers) createDriveDocument(c *gin.Context) {
	p := current(c)
	if p == nil {
		c.JSON(http.StatusUnauthorized, errorResponse("unauthorized", "login required", nil))
		return
	}
	var in struct {
		URL string `json:"url"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		badRequest(c, "invalid request body")
		return
	}
	fileID, rk, err := parseDriveFileRef(in.URL)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	doc, err := h.upsertDriveDoc(c, *p, fileID, rk)
	if err != nil {
		internalErr(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": doc.ID})
}
```
Then update `driveOpen` to call `h.upsertDriveDoc(c, *p, fileID, rk)` instead of the inline `h.deps.repository.UpsertDriveDocument(...)`, so both handlers share the helper. Confirm `auth` is already imported (it is, for `resolveSession`).

- [ ] **Step 4: Register the route**

In `server.go`, on the authed group near `authed.POST("/drive/render", ...)`:

```go
authed.POST("/drive/documents", h.createDriveDocument)
```

- [ ] **Step 5: Run it — expect PASS**

Run: `cd api && go test -tags testfixtures ./internal/server/ -run TestCreateDriveDocument -v`
Expected: PASS.

- [ ] **Step 6: Run the package suite (no regressions in driveOpen)**

Run: `cd api && go build ./... && go test -tags testfixtures ./internal/server/...`
Expected: PASS (existing `/drive/open` tests still green after the refactor).

- [ ] **Step 7: Commit**

```bash
git add api/internal/server/handlers_drive.go api/internal/server/server.go api/internal/server/server_test.go
git commit -m "feat(server): create Drive-backed document from a pasted link"
```

---

## Task 3: Web — "Open Drive link" dialog on Documents page

**Files:**
- Modify: `frontend/src/api.ts` (add `createDriveDocument`)
- Modify: `frontend/src/pages/documents.tsx` (add the dialog + entry point, mirroring `UploadBackupDialog`/`UploadBackupItem`)

**Interfaces:**
- Produces: `api.createDriveDocument(url: string): Promise<{ id: string }>`.

- [ ] **Step 1: Add the API method**

In `frontend/src/api.ts` `api` object (mirror existing methods):

```ts
createDriveDocument: (url: string) =>
  request<{ id: string }>("/v1/drive/documents", {
    method: "POST",
    body: JSON.stringify({ url }),
  }),
```

- [ ] **Step 2: Add the dialog + entry point**

In `frontend/src/pages/documents.tsx`, mirror the existing `UploadBackupDialog` (read it first — it already wraps a dialog with a trigger and an action). Add an `OpenDriveLinkDialog` with a single URL `Input`, a submit button, and inline error display, plus an `OpenDriveLinkItem` placed next to `UploadBackupItem` in the "More" dropdown (line ~116). On submit:

```tsx
const nav = useNavigate(); // already imported/used in this file
const [url, setUrl] = useState("");
const m = useMutation({
  mutationFn: () => api.createDriveDocument(url.trim()),
  onSuccess: (res) => nav(`/app/d/${res.id}`),
});
// render: <Input value={url} onChange=... placeholder="https://drive.google.com/file/d/…/view" />
//         <Button disabled={!url.trim() || m.isPending} onClick={() => m.mutate()}>Open</Button>
//         {m.error instanceof APIError && <p className="text-[var(--color-danger)]">{m.error.message}</p>}
```
Match the existing dialog component imports already used by `UploadBackupDialog` (Dialog primitives, `Button`, `Input`, `APIError`). Keep copy concise: title "Open from Google Drive", helper text "Paste a link to an HTML file in Drive."

- [ ] **Step 3: Build**

Run: `cd frontend && npm run build`
Expected: tsc + vite build clean.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/api.ts frontend/src/pages/documents.tsx
git commit -m "feat(frontend): open a Drive HTML file from a pasted link"
```

---

## Task 4: CLI — `aidocs docs drive <url>`

**Files:**
- Modify: `cli/internal/cmd_docs.go` (add the `drive` subcommand; register it in `docsCmd`'s `AddCommand`)
- Test: `cli/internal/e2e_test.go` (mirror an existing `docs` command test if the suite supports it; otherwise rely on build + the server endpoint test)

**Interfaces:**
- Produces: `aidocs docs drive <url>` — POSTs `{url}` to `/v1/drive/documents`, prints the review URL (compact) / `url` field (JSON), mirroring `docs create` minus the org-wide nudge.

- [ ] **Step 1: Add the command**

In `cli/internal/cmd_docs.go`, add (mirroring the `create` command's output handling at lines 35-67, but without `maybeGrantOrgWideViewer`/nudge):

```go
driveOpen := &cobra.Command{
	Use:     "drive <url>",
	Short:   "Open a Google Drive HTML file in aidocs by link",
	Long:    "Register a Google Drive HTML file (by share link or file id) as an aidocs document and print its review URL.\n\nThe file stays in Drive; aidocs renders it live. Native Google Docs/Sheets/Slides are not supported.",
	Example: "  aidocs docs drive 'https://drive.google.com/file/d/ABC123/view'",
	Args:    exactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cl, err := client(g)
		if err != nil {
			return err
		}
		b, err := cl.doJSON("POST", "/v1/drive/documents", map[string]any{"url": args[0]})
		if err != nil {
			return err
		}
		if g.quiet {
			return nil
		}
		var m map[string]any
		if json.Unmarshal(b, &m) != nil {
			return render(out, g, b)
		}
		id := fmt.Sprint(value(m, "id"))
		if id == "" || id == "<nil>" {
			if g.json {
				return render(out, g, b)
			}
			confirm(out, g, "Created Drive document. URL unavailable: response did not include id.")
			return nil
		}
		docURL := browserURL(cl.Base, "/app/d/%s", id)
		if g.json {
			m["url"] = docURL
			nb, err := json.Marshal(m)
			if err != nil {
				return err
			}
			return render(out, g, nb)
		}
		confirm(out, g, fmt.Sprintf("Opened Drive document (%s). Review it at %s", id, docURL))
		return nil
	},
}
```
Confirm the exact signatures of `cl.doJSON`, `client`, `browserURL`, `confirm`, `render`, `value`, `exactArgs`, and the `globals` fields (`g.quiet`, `g.json`) by reading the surrounding file — they are all used by neighboring commands. Adjust if any differ.

- [ ] **Step 2: Register it**

In `docsCmd`'s `c.AddCommand(...)` call (line 92), add `driveOpen` to the list.

- [ ] **Step 3: Build + vet**

Run: `cd cli && go build ./... && go vet ./...` (or from repo root `go build ./cli/...`).
Expected: clean.

- [ ] **Step 4: Test (if the e2e harness supports it)**

If `cli/internal/e2e_test.go` has a pattern for exercising a `docs` subcommand against a stub/real server, add a case for `docs drive` asserting it prints a review URL / JSON `url`. If the harness can't easily cover it, note that the endpoint is covered by Task 2's server test and the CLI is a thin pass-through; rely on build + vet.

- [ ] **Step 5: Commit**

```bash
git add cli/internal/cmd_docs.go cli/internal/e2e_test.go
git commit -m "feat(cli): open a Drive HTML file by link via 'docs drive'"
```

---

## Self-Review

**Spec coverage:**
- §1 shared backend primitive → Task 2 (endpoint + helper) + Task 1 (parser). ✓
- §1 accepted/rejected URL forms → Task 1 tests. ✓
- §2 web button/dialog → Task 3. ✓
- §3 CLI command → Task 4. ✓
- §5 testing → parser tests (T1), endpoint test (T2), CLI/build (T4). ✓

**Type consistency:** `parseDriveFileRef(raw) (fileID, resourceKey, err)` used in T1/T2. `createDriveDocument` returns `{id}` (backend T2) consumed as `{ id: string }` (frontend T3) and read as `m["id"]` (CLI T4). Endpoint path `/v1/drive/documents` consistent across T2/T3/T4.

**Known soft spots (read from code, not placeholders):** the exact test-helper names in `testhelpers_test.go` (T2) and the CLI helper signatures + `UploadBackupDialog` structure (T3/T4) must be confirmed by the implementer against the codebase — flagged at each step.
