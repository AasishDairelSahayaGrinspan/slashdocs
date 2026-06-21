# Open a Drive HTML file by link (web + CLI)

**Date:** 2026-06-22
**Status:** Approved design
**Builds on:** `2026-06-21-google-drive-open-with-design.md` (Drive-backed documents, `UpsertDriveDocument`, client-side render).

## Summary

In addition to Drive's "Open with → aidocs" flow, let a user paste a **Drive link**
to an HTML file and open it in aidocs — from the web UI and from the CLI. The
file stays in Drive; this reuses the existing Drive-backed document model and
client-side render entirely. The only new logic is parsing a Drive URL into a
file id.

## Decisions (locked during brainstorming)

| Question | Decision |
| --- | --- |
| Web entry point | A button on the Documents list → small paste-URL dialog → navigate to the doc |
| CLI behavior | `aidocs docs drive <url>` mirrors `docs create`: register the doc, print the aidocs review URL (and JSON `url`) |
| URL parsing location | Server-side only (Go), so web and CLI share one parser; CLI sends the raw URL |
| Title | Placeholder "Drive document" (same as `/drive/open`); the HTML's own `<title>` shows in the render |

## 1. Shared backend primitive

New **authed** endpoint `POST /v1/drive/documents`, body `{ "url": "<drive link or file id>" }`:

1. Parse the URL with a new Go helper `ParseDriveFileRef(raw string) (fileID, resourceKey string, err error)`.
2. Call the existing `UpsertDriveDocument(ctx, owner, fileID, resourceKey, "Drive document")` (dedupe by fileId).
3. Return `201 { "id": "<docID>" }`.

**DRY refactor:** extract the "fileId → Drive-backed doc" step shared by this
handler and the existing `/drive/open` handler into one helper
(`upsertDriveDocFromRef` or similar), so there is a single code path for creating
a Drive-backed document.

### Accepted URL forms (`ParseDriveFileRef`)

- `https://drive.google.com/file/d/<ID>/view` (optional `&resourcekey=<RK>`)
- `https://drive.google.com/open?id=<ID>` (optional `&resourcekey=<RK>`)
- `https://drive.google.com/uc?id=<ID>`
- a bare `<ID>` (already a file id)

### Rejected (clear 400 with message)

- `https://docs.google.com/document/d/<ID>/...` and the `spreadsheets`/
  `presentation` variants — native Google Docs are not raw HTML files
  (out of scope; would need `files.export`).
- Empty / unparseable input → 400.

## 2. Web — "Open Drive link" button

On the Documents list page, add an "Open Drive link" button next to the existing
create/upload action. It opens a small dialog with a URL input and a submit:

- Calls `api.createDriveDocument(url)` → `POST /v1/drive/documents`.
- On success, navigate to `/app/d/:id`; the existing render path (live GIS fetch
  → sandbox) takes over unchanged.
- On 400, show the server's error message inline (invalid link / Google Doc).

## 3. CLI — `aidocs docs drive <url>`

A new subcommand under `docs`, mirroring `docs create`:

- POSTs `{ "url": <arg> }` to `/v1/drive/documents`.
- Compact mode: `Created document (<id>). Review it at <browserURL(/app/d/<id>)>`.
- `--json` mode: include the `url` field, like `create`.
- Reuses the existing `browserURL` / `confirm` / `render` helpers.
- Skips the org-wide-viewer nudge (grants are inert for Drive docs; Drive ACL
  gates access).

## 4. Out of scope (YAGNI)

Auto-opening the browser from the CLI, client-side URL parsing, Google Docs
export, fetching the real filename for the title.

## 5. Testing

- **Parser unit tests:** every accepted form (with/without resource key, bare id)
  and every rejection (Google Docs URL, empty, garbage).
- **Endpoint test:** valid URL → 201 + dedupe by fileId; Google-Doc URL → 400.
- **CLI:** mirror an existing `docs` command test.
- **Frontend:** clean typed build; the dialog wiring verified by build + manual.
