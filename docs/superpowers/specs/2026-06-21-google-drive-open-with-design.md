# Google Drive "Open with" → aidocs

**Date:** 2026-06-21
**Status:** Approved design

## Summary

Register aidocs as an internal Google Workspace **Drive app** so users can
right-click an HTML file in Drive → **Open with → aidocs** → view it in aidocs'
existing sandboxed renderer with comments.

The file **never leaves Drive**. aidocs stores only a pointer (`fileId`) plus its
own comments. Access is gated entirely by Drive: per-viewer `drive.readonly`,
content fetched client-side, **zero tokens stored server-side**.

Distribution: razorpay.com Workspace domain only (admin-trusted app, domain-wide
scope grant — no public Google OAuth verification).

## Decisions (locked during brainstorming)

| Question | Decision |
| --- | --- |
| Relationship to Drive file | File stays in Drive; opened directly, not imported |
| What aidocs provides | Drive-backed document: pointer + aidocs comments, content fetched live |
| Access gate | Per-viewer; **Drive ACL is the sole authority** (aidocs grants ignored for Drive docs) |
| Scope + fetch | `drive.readonly` + **client-side** fetch (browser → Drive), no stored tokens |
| Distribution | Internal razorpay.com Workspace only |
| Versions | None — Drive docs are always the live latest content |

## 1. The "Open with" entry point

- Register a Drive UI integration in Google Cloud Console: app name/icon,
  default MIME type `text/html`, **Open URL** = `https://<aidocs>/drive/open`.
- Google redirects there with a URL-encoded `state` query param:
  `{ids:[fileId], action:"open", userId, resourceKeys?}`.
- New handler `GET /drive/open`:
  1. Parse `state` (URL-decode, JSON-parse, validate `action == "open"` and `ids`).
  2. Ensure an aidocs session. If none, run the existing Google OAuth login. If
     `state.userId` ≠ current session user, re-auth as the requested user.
  3. **Upsert** a Drive-backed document record keyed by `fileId` (re-opening the
     same file reuses its existing comments).
  4. Redirect to the normal aidocs document page for that record.

## 2. Data model

Add to the documents record:

- `source` — `native` | `drive`
- `drive_file_id` — unique, nullable (set for `drive` docs)
- `drive_resource_key` — nullable (passed via `X-Goog-Drive-Resource-Keys`)

Drive docs store **no blob and no versions** — content is always the live Drive
file. The comments and grants tables are unchanged. For Drive docs, "who can
view/comment" is enforced by whether the viewer can fetch the file from Drive,
**not** by aidocs grants (grants are intentionally inert for Drive docs).

## 3. OAuth changes

Add `https://www.googleapis.com/auth/drive.install` and
`https://www.googleapis.com/auth/drive.readonly` to the OAuth config
(`api/internal/auth/oauth.go`). `drive.install` makes aidocs appear in the
Open-with menu; `drive.readonly` lets the browser read the opened file. Login
flow otherwise unchanged (still uses the `id_token`). No refresh tokens persisted.

## 4. Render path (reuses existing sandbox + comment bridge)

The existing render flow serves a sandboxed iframe on a separate `renderOrigin`
with `bridge.js` injected (`renderWrapperHTML`). The Drive path reuses it.

On the document page, for a Drive doc the **browser**:

1. Gets a short-lived Drive access token via Google Identity Services (GIS) token
   client.
2. Fetches the file: `GET drive/v3/files/{fileId}?alt=media`
   (+ `X-Goog-Drive-Resource-Keys` header if a resource key is present).
   - `403` → "No Drive access" state.
   - `404` → "File deleted/moved" state.
3. Sends the fetched HTML bytes to a new render endpoint on `renderOrigin` that
   wraps them with the **existing `renderWrapperHTML`** (same sandbox iframe +
   `bridge.js`) and returns the page. Bytes are **never persisted**; the Drive
   token never reaches the server. Comment anchoring and origin isolation behave
   identically to native docs.

**Rationale for the round-trip:** the sandbox wrapper must be served from the
isolated `renderOrigin` (untrusted HTML must not run on the app's cookie origin).
Alternative — the render frame itself performs the GIS fetch so HTML never hits
the server — is possible but adds GIS/token plumbing on `renderOrigin`. The
round-trip is preferred; HTML transits the server transiently and is discarded.

## 5. Error / edge handling

- Drive ACL revoked / file deleted → clear "source unavailable" state, no stale cache.
- `state.userId` mismatch with session → re-auth as the requested user.
- Non-HTML MIME → friendly rejection (only `text/html` is registered).
- Google Doc (not a raw HTML file) → out of scope v1 (would need `files.export`);
  reject clearly.

## 6. Out of scope (YAGNI)

"New" URL (create-in-Drive), Google Docs export, Shared Drive nuances beyond
`resourceKeys`, version snapshots, content caching (add only if Drive
latency/quota bites), editing/writing back to Drive.

## 7. Testing

- **Unit:** `state` param parse (encoding, multi-account, missing fields), MIME
  guard, document upsert-by-`fileId` dedup.
- **Render:** Drive HTML bytes → wrapper output matches native render (sandbox
  attributes, `bridge.js` injected).
- **Manual:** end-to-end Open-with in a Razorpay test Workspace; verify 403/404
  states.

## 8. Operational prerequisites (outside code)

- Google Cloud project with Drive API enabled and the Drive UI integration
  configured (Open URL, icon, `text/html`).
- Google Workspace Marketplace listing scoped to razorpay.com, admin-installed
  domain-wide (pre-authorizes `drive.readonly` so end users skip consent).
