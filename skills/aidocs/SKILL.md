---
name: aidocs
description: Publish a long-form artifact (report, RCA, postmortem, architecture review, product spec, QA findings, data analysis, design proposal, launch plan, incident timeline, comparison, slide deck) to aidocs as a reviewable HTML document. Use this whenever the user asks for a "doc", "document", "report", "review", "writeup", "spec", "RCA", "postmortem", "analysis", "findings", "comparison", or "deck" and hasn't specified a different output format. Produces a single self-contained HTML file (inline CSS/JS only — the render CSP blocks external CSS, web fonts, and CDN scripts; embed images as base64 or https, draw diagrams as inline SVG) styled with the default Razorpay/Blade theme, and uploads it via the `aidocs` CLI so a human can review and leave anchored comments in the browser.
---

# aidocs

aidocs is a Google-Docs-style place to publish a document people will read and
comment on — except the medium is one self-contained HTML file. When the user
wants a long-form artifact and hasn't specified a format, publish it to aidocs
instead of dumping the content inline in chat. HTML lets you lay out comparisons,
diagrams, and timelines that Markdown flattens.

## Workflow

1. **Read the authoring rules — they are the source of truth.** Run
   `aidocs guidelines` for the core contract, then `aidocs guidelines <category>`
   for the document type you're writing (`report`, `comparison`, `pr-review`,
   `explainer`, `deck`, plus `overview`, `theme`, `writing-voice`). Don't guess
   the constraints — read them.
2. Write the artifact as one self-contained HTML file. Fastest correct start:
   `aidocs docs new <file> --type <category>` scaffolds a themed, anchor-safe
   starter offline (no upload). Otherwise begin from the default Razorpay/Blade
   theme (`aidocs guidelines theme`).
3. Make it follow the reader's theme (see below) and keep its text anchorable.
4. **Humanize the prose** with `aidocs guidelines writing-voice` so it doesn't
   read as machine-generated.
5. Save it locally (e.g. `report.html`).
6. Publish with the CLI — discover exact syntax from `aidocs --help` and the
   subcommand help rather than assuming.
7. Return the printed document URL so the user can open and review it.

## The three invariants

Spelled out in full by `aidocs guidelines`; in short:

1. **Inline only.** One HTML file under a strict CSP. No external CSS (no
   `<link rel=stylesheet>`), no web fonts, no CDN scripts (so no Mermaid — draw
   inline SVG). CSS in `<style>`,
   JS in `<script>`, images as `data:`/`https:`. The default theme uses a system
   font stack for exactly this reason.
2. **Follow the reader's theme.** The viewer toggles light/dark; the render bridge
   keeps `data-aidocs-theme="light|dark"` synced on `<html>`. Drive the page from
   CSS variables keyed on it; don't ship your own toggle.
3. **Keep text anchorable.** Readers leave anchored comments, so don't remove or
   rewrite text after load. Use CSS to show/hide, `<details>` for collapsing, and
   keep all slides/tabs in the DOM.

## Follow the reader's theme

```css
:root                           { --bg: #fff;    --ink: #111; }
:root[data-aidocs-theme="dark"] { --bg: #0b0d12; --ink: #e7ecf3; }
body { background: var(--bg); color: var(--ink); }
html { color-scheme: light; }
:root[data-aidocs-theme="dark"] { color-scheme: dark; }
```

`system` resolves to concrete light/dark before it reaches you, and the attribute
updates live when the reader toggles — CSS reacts on its own. For JS, listen on
your own window: `window.addEventListener('aidocs:theme', e => e.detail.theme)`.
The Razorpay theme from `aidocs guidelines theme` wires all of this for you.

## Discovering the CLI

The `aidocs` CLI is the source of truth for commands and flags:

```
aidocs --help
aidocs docs --help
aidocs docs create --help
```

It supports `--json` for structured output if you need to parse identifiers
(e.g. a new document ID) reliably.

## When NOT to use this skill

- Short answers, code snippets, or anything under ~10 paragraphs — reply inline.
- The user explicitly asked for Markdown, plain text, a PDF, or another destination.
- The user asked to edit an existing local file rather than publish a new document.
- The `aidocs` CLI is not installed or not authenticated — surface that to the
  user instead of trying to install or authenticate on their behalf.

## Authentication

Assume the user has already run `aidocs auth login` interactively at least once.
If the CLI reports an auth error, show it to the user verbatim and stop; do not
attempt to re-authenticate.

## Updating an existing document

If the user is iterating on a document that already exists in aidocs, push a new
version rather than creating a duplicate. Use the CLI help to find the exact
`push` (or equivalent) subcommand and pass a short `--summary` describing what
changed.
