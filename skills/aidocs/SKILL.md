---
name: aidocs
description: Publish a long-form artifact (report, RCA, postmortem, architecture review, product spec, QA findings, data analysis, design proposal, launch plan, incident timeline) to aidocs as a reviewable HTML document. Use this whenever the user asks for a "doc", "document", "report", "review", "writeup", "spec", "RCA", "postmortem", "analysis", or "findings" and hasn't specified a different output format. Also use when a user asks Slash to "publish to aidocs", "upload this HTML", "get a shareable link", or attaches an HTML file to a Slack message and asks for a shareable link. Produces a single HTML file (external stylesheets/fonts/scripts allowed; aidocs does not host assets, so embed images as base64) and uploads it via the `aidocs` CLI so a human can review and leave anchored comments in the browser.
---

# aidocs

When the user wants a long-form artifact and hasn't specified a format,
publish it to aidocs instead of dumping the content inline in chat.

## Workflow

1. Read the authoring rules first: run `aidocs guidelines`. It is the
   source of truth for the single-file/base64-image constraints and the
   reader-theme contract below.
2. Write the artifact as one HTML file. Anything that fits in a single
   .html file is fine, including external stylesheets, fonts, and
   scripts. aidocs does NOT host assets, so embed images as base64 data
   URLs (or reference a URL you already control) and use inline SVG or
   Mermaid for diagrams.
3. Make the document follow the reader's aidocs theme (see below).
4. Save it locally (e.g. `report.html`).
5. Publish it with the CLI. Discover the exact syntax from
   `aidocs --help` and the relevant subcommand help — don't assume.
6. Return the printed document URL to the user so they can open and
   review it.

## Follow the reader's theme

The aidocs viewer has a light/dark toggle and the render layer exposes the
chosen theme to your document. Opt in so the doc matches the reader's
chrome instead of shipping its own toggle.

The render bridge sets and keeps in sync, on `<html>` inside your document:
`data-aidocs-theme="light|dark"`. Drive the page from CSS variables keyed on
that attribute:

```css
:root                           { --bg: #fff;    --ink: #111; }
:root[data-aidocs-theme="dark"] { --bg: #0b0d12; --ink: #e7ecf3; }
body { background: var(--bg); color: var(--ink); }
html { color-scheme: light; }
:root[data-aidocs-theme="dark"] { color-scheme: dark; }
```

Notes:
- `system` is resolved to a concrete `light`/`dark` before it reaches you,
  and the attribute updates live when the reader toggles — CSS reacts on
  its own.
- For JS, listen on your own window:
  `window.addEventListener('aidocs:theme', e => e.detail.theme)`.
- Do NOT add your own theme switch; the aidocs UI is the single control.
- Theming is opt-in: a document that never references `data-aidocs-theme`
  keeps its hard-coded look.
- `aidocs guidelines` prints this same contract if you need it inline.

## Discovering the CLI

The `aidocs` CLI is the source of truth. Run these to find the current
commands and flags rather than guessing:

```
aidocs --help
aidocs docs --help
aidocs docs create --help
```

The CLI supports `--json` for structured output if you need to parse
identifiers (e.g. the new document ID) reliably.

## Slack / Slash integration

Slash (the Razorpay AI coding assistant) can publish to aidocs in two modes:

**Mode 1 — Generate HTML then publish**
When the user asks Slash to create a document (e.g. "generate an architecture diagram and share it"):
1. Generate the HTML as described in the Workflow section.
2. Save it locally (e.g. `/tmp/doc.html`).
3. Run `aidocs docs create /tmp/doc.html --title "Title" --json` to upload it.
4. Share the printed URL back in Slack.

**Mode 2 — Upload an HTML file the user already attached**
When the user attaches an HTML file to the Slack message, the system saves it to disk and
includes a note like `[File attached — path: /app/uploads/slack_attachments/slack_attachment_<id>.html]`
in the prompt. In that case:
1. Read the file from the given path (do NOT re-save it — the file is already on disk).
2. Run `aidocs docs create <path> --title "Title" --json` directly on that path.
3. Share the printed URL back in Slack.

**Sharing after upload**
Documents are private by default. After creating, grant read access to everyone in the org:

```bash
aidocs docs grants add <doc_id> --everyone --role commenter
```

This lets any Razorpay employee open the link and leave comments without needing an invite.

**Service account authentication**
When running inside Slash, `AIDOCS_TOKEN` and `AIDOCS_SERVER` are pre-set as env vars.
The CLI picks them up automatically — no `aidocs auth login` is needed. If you see an auth
error, surface it verbatim and stop; do not attempt to re-authenticate.

## When NOT to use this skill

- Short answers, code snippets, or anything under ~10 paragraphs —
  reply inline as usual.
- The user explicitly asked for Markdown, plain text, a PDF, or a
  different destination.
- The user asked to edit an existing local file rather than publish a
  new document.
- The user is working in an environment where the `aidocs` CLI is not
  installed or not authenticated — surface that to them instead of
  trying to install or authenticate on their behalf.

## Authentication

Assume the user has already run `aidocs auth login` interactively at
least once. If the CLI reports an auth error, show the error to the
user verbatim and stop; do not attempt to re-authenticate.

## Updating an existing document

If the user is iterating on a document that already exists in aidocs,
push a new version rather than creating a duplicate. Use the CLI help
to find the exact `push` (or equivalent) subcommand and pass a short
`--summary` describing what changed.
