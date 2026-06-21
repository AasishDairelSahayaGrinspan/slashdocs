# Google Drive "Open with" — Operational Setup

This integration lets users open an HTML file from Google Drive directly in
aidocs ("Open with → aidocs"). The file stays in Drive; aidocs renders it live
in its sandboxed viewer and layers comments on top. Access is gated entirely by
Drive (per-viewer), and **no Google tokens are stored server-side** — the Drive
read happens in the user's browser.

Distribution is **internal to the razorpay.com Workspace only**, so the app is
admin-trusted and skips public OAuth verification.

The code is in place (see `docs/superpowers/specs/2026-06-21-google-drive-open-with-design.md`).
The steps below are the one-time Google Cloud / Workspace configuration needed
to turn it on. They require a GCP project owner and a Google Workspace admin.

## 1. Enable the Drive API

In the GCP project that backs aidocs' OAuth client:

1. APIs & Services → **Enable APIs and Services** → enable **Google Drive API**.

## 2. Configure the Drive UI integration

APIs & Services → **Google Drive API** → **Drive UI Integration** tab:

- **Application name** and **Application icon** (PNG with transparent background,
  the sizes Google requests).
- **Open URL:** `https://<aidocs-domain>/drive/open`
  (must be a fully-qualified domain — not `localhost`).
- **Default MIME types:** `text/html`
- **Default file extensions:** `html`, `htm`
- Leave **Importing** and **Shared drives** off for v1 (out of scope).

When a user picks "Open with → aidocs", Google redirects to the Open URL with a
URL-encoded `state` parameter (`{ids:[fileId], action:"open", userId, resourceKeys?}`),
which the `GET /drive/open` handler parses.

## 3. OAuth consent screen + scopes

APIs & Services → **OAuth consent screen**:

- **User type: Internal** (razorpay.com). Internal apps are trusted by the
  Workspace and do not go through Google's public verification / security
  assessment, even with the Drive scopes below.
- Add scopes:
  - `https://www.googleapis.com/auth/drive.install` — makes aidocs appear in the
    Drive "Open with" menu.
  - `https://www.googleapis.com/auth/drive.readonly` — lets the browser read the
    opened file (per-viewer; Drive ACL is the access gate).
  - (Plus the existing `openid`, `email`, `profile` used for login.)

## 4. OAuth client — authorized JavaScript origins

The browser obtains the short-lived `drive.readonly` token via Google Identity
Services (GIS), so the OAuth client (APIs & Services → **Credentials** → the
OAuth 2.0 Client ID aidocs uses) must list the aidocs app origin under
**Authorized JavaScript origins**, e.g. `https://<aidocs-domain>`.

(The existing **Authorized redirect URIs** for server-side login stay as they are.)

## 5. Publish to the Workspace Marketplace (domain-scoped)

To make aidocs show up in "Open with" for users:

1. Create a **Google Workspace Marketplace** listing for the app, scoped to the
   razorpay.com domain.
2. Have a **Workspace admin install it domain-wide.** A domain-wide install
   pre-authorizes the `drive.readonly` scope, so end users are not prompted with
   a consent screen on first open.

## 6. aidocs server configuration

No new secrets are introduced. The integration reuses the existing Google OAuth
client credentials and app origin:

| Env var | Used for |
| --- | --- |
| `GOOGLE_OAUTH_CLIENT_ID` | Login **and** now surfaced to the browser via `GET /v1/config` (`google_oauth_client_id`) so GIS can request the Drive token. |
| `GOOGLE_OAUTH_CLIENT_SECRET` | Login (unchanged). |
| `APP_ORIGIN` | CSP `frame-ancestors` and the render bridge's postMessage target (unchanged). |
| `AIDOCS_RENDER_ORIGIN` | Render isolation origin; surfaced via `/v1/config` (`render_origin`). For v1 the Drive render path is served same-origin — see the note below. |

## 7. Verify end to end

1. Upload an `.html` file to Drive (in a razorpay.com account).
2. Right-click → **Open with → aidocs**.
3. Expect: redirect through aidocs login if needed → land on `/app/d/<id>` →
   GIS token grant (silent if admin-installed) → the file renders in the
   sandboxed viewer.
4. Add a comment; reload — it persists. Re-open the same file from Drive — same
   aidocs document, same comments (dedupe by Drive `fileId`).
5. As a user **without** Drive access to the file → "You don't have access to
   this file in Google Drive."

## Known v1 limitations

- **Same-origin render only.** The Drive HTML is wrapped by `POST /v1/drive/render`
  on the app origin. A separately-configured `AIDOCS_RENDER_ORIGIN` would require
  CORS on that POST; defer until a distinct render origin is actually deployed.
- **Multi-account.** The `state.userId` from Drive is not reconciled against the
  active aidocs session; the session user becomes the document owner. Fine for
  single-account users; revisit if multi-account opens are common.
- **Google Docs are not supported** — only raw `text/html` files. Native Google
  Docs would need `files.export`, which is out of scope.
