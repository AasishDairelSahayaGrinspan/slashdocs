// Package guidelines holds the aidocs document authoring guidelines shared by
// the CLI (aidocs guidelines) and the server-side MCP tool
// (get_authoring_guidelines). Content is intentionally plain text so agents
// can read it directly before writing an aidocs document; both surfaces offer
// the same sections with progressive disclosure from an index.
package guidelines

import "strings"

// Page is one addressable guidelines section.
type Page struct {
	Path    []string
	Summary string
	Body    string
	Aliases [][]string
}

var Pages = []Page{
	{
		Path:    []string{"core"},
		Summary: "The single-file HTML contract and default authoring posture.",
		Body: `aidocs guidelines: core

A document is ONE HTML file.
  - Anything reasonable that fits in a single .html file is fine, including
    inline code and external stylesheets, fonts, scripts, Mermaid/CDN assets,
    iframes, workers, media, data: URLs, and blob: URLs.
  - Prefer semantic, selectable HTML for prose and data. Review comments anchor
    more reliably when text is real text instead of pixels.
  - Keep the first screen useful. aidocs opens the rendered document directly,
    so avoid marketing-style wrappers around documents that are meant to be read
    or reviewed.
  - Do not hide important content behind hover-only interactions. Readers and
    reviewers should be able to scan, select, copy, and comment on the artifact.

Useful next pages:
  aidocs guidelines core assets
  aidocs guidelines rendering theme
  aidocs guidelines review
`,
	},
	{
		Path:    []string{"core", "assets"},
		Summary: "How images, fonts, CDNs, and embedded media should be handled.",
		Aliases: [][]string{{"assets"}},
		Body: `aidocs guidelines: core assets

aidocs stores your HTML, not a directory of supporting files.
  - aidocs does NOT host assets for you. There is no image/file hosting.
  - Embed important images as base64 data URLs:
      data:image/png;base64,...
  - External HTTPS URLs are allowed when you control them or they are stable
    enough for the document's purpose.
  - CDNs, external stylesheets, fonts, scripts, iframes, workers, media,
    data: URLs, and blob: URLs are allowed.
  - If the document must remain useful offline or years later, inline the
    critical CSS, JS, and images.
  - Keep large generated assets intentional. A single HTML file can become hard
    to review if it carries many megabytes of media.
`,
	},
	{
		Path:    []string{"core", "publishing"},
		Summary: "Create and update documents from the CLI.",
		Aliases: [][]string{{"publishing"}, {"publish"}},
		Body: `aidocs guidelines: core publishing

Publish a new document:
  aidocs docs create report.html --title 'Report'

Markdown files work too; the CLI converts .md/.markdown to HTML before upload:
  aidocs docs create notes.md --title 'Notes'

Push an update to an existing document:
  aidocs docs push <doc_id> report.html --base-version <version_id> --summary 'Address comments'

If you previously ran aidocs docs pull <doc_id>, the CLI remembers that base
version for the active context, so --base-version is optional.

Publishing model:
  - Creating a document stores the uploaded HTML as version 1.
  - Pushing HTML creates a new immutable version.
  - The aidocs UI is for review; agents and CLI/API publishers create versions.
  - Runtime KV changes from uploaded HTML do not create versions; see:
      aidocs guidelines interactivity kv

Before publishing, run the relevant guideline pages for the kind of document you
are authoring. For example:
  aidocs guidelines rendering theme
  aidocs guidelines review
`,
	},
	{
		Path:    []string{"rendering"},
		Summary: "How aidocs renders uploaded HTML inside the reader.",
		Body: `aidocs guidelines: rendering

aidocs renders uploaded HTML inside the document reader.
  - The document should be complete on its own and not depend on parent-page DOM.
  - Assume the document may be resized often as sidebars, comments, and browser
    windows change.
  - Use responsive layout constraints instead of viewport-only tricks.
  - Keep text selectable. Commenting and review workflows work best on real text.
  - Links should be explicit and safe for a reader to open in a new tab.
  - If your document has scripts, make initialization idempotent; the render
    frame may reload as versions or theme settings change.

Useful next pages:
  aidocs guidelines rendering theme
  aidocs guidelines rendering layout
  aidocs guidelines interactivity
`,
	},
	{
		Path:    []string{"rendering", "theme"},
		Summary: "Follow the reader's light/dark theme.",
		Aliases: [][]string{{"theme"}, {"themes"}},
		Body: `aidocs guidelines: rendering theme

Theme: follow the reader's aidocs theme.
  The viewer chooses light or dark in the aidocs UI. The render bridge sets
  this attribute on <html> inside your document and keeps it in sync:

    <html data-aidocs-theme="light">
    <html data-aidocs-theme="dark">

  Opt in by defining your palette against the attribute and driving the page
  from variables:

    :root[data-aidocs-theme="dark"] {
      --bg: #0b0d12; --ink: #e7ecf3;
    }
    :root { --bg: #fff; --ink: #111; }
    body { background: var(--bg); color: var(--ink); }
    html { color-scheme: light; }
    :root[data-aidocs-theme="dark"] { color-scheme: dark; }

  Notes:
  - "system" is resolved to a concrete light/dark before it reaches you.
  - The value updates live when the reader toggles; CSS reacts automatically.
  - For JS, listen for the event on your own window:
      window.addEventListener('aidocs:theme', e => {
        // e.detail.theme is "light" or "dark"
      });
  - Do NOT ship your own theme toggle. The aidocs UI is the single control.
  - A document that never references data-aidocs-theme keeps its hard-coded
    look, so theming is strictly opt-in.
`,
	},
	{
		Path:    []string{"rendering", "layout"},
		Summary: "Responsive, printable, review-friendly layout guidance.",
		Aliases: [][]string{{"layout"}},
		Body: `aidocs guidelines: rendering layout

Layout for reading and review:
  - Use real headings, paragraphs, tables, lists, and figures where they fit.
  - Keep line lengths comfortable and leave enough space for comment highlights.
  - Prefer responsive grids and wrapping tables to fixed-width canvases for
    operational reports, specs, and trackers.
  - If you include a print stylesheet, keep it additive and avoid hiding content
    that reviewers need in the browser.
  - Avoid UI text overlapping content at narrow widths. Test mobile-sized and
    split-pane-sized viewports, not only a full desktop window.
  - For dashboards or trackers, make controls compact and predictable. The first
    screen should be the working surface, not a landing page.
`,
	},
	{
		Path:    []string{"interactivity"},
		Summary: "Scripts, controls, links, and stateful document behavior.",
		Body: `aidocs guidelines: interactivity

Interactive documents are allowed.
  - Inline scripts, external scripts, iframes, workers, blob: URLs, and
    client-side rendering are allowed inside the single HTML file.
  - Prefer progressive enhancement: the core information should remain readable
    if a script is slow or a CDN is unavailable.
  - Use ordinary form controls for document-local tools: selects for option
    sets, checkboxes/toggles for binary state, inputs/sliders for numbers, and
    buttons for clear commands.
  - For links, make the target obvious in the visible text.
  - For persisted document-local state such as approvals, tracker rows, and
    dropdown selections, use the aidocs KV primitive:
      aidocs guidelines interactivity kv
`,
	},
	{
		Path:    []string{"interactivity", "kv"},
		Summary: "Persist approval states, tracker fields, dropdowns, and other document-local state.",
		Aliases: [][]string{{"kv"}, {"state"}, {"store"}, {"storage"}},
		Body: `aidocs guidelines: interactivity kv

Uploaded HTML can use a document-scoped KV store through window.aidocs.kv.
This is for runtime state such as approvals, tracker fields, dropdown choices,
checkboxes, and lightweight workflow metadata.

API:
  const value = await window.aidocs.kv.get("approval:pm");
  const all = await window.aidocs.kv.all();

  await window.aidocs.kv.set("approval:pm", {
    status: "approved",
    by: "pm@example.com"
  });

  await window.aidocs.kv.delete("approval:pm");

  await window.aidocs.kv.patch({
    "approval:pm": { status: "approved" },
    "tracker:old-row": null
  });

  await window.aidocs.kv.undo();
  await window.aidocs.kv.redo();

Events:
  function renderFromKV(event) {
    // event.source is "initial" for hydrate and "local" for local writes.
    // event.revision is the document KV revision.
    // event.items exists on hydrate/all-key snapshots.
    // event.changes exists on writes and lists changed keys.
    // event.operation is "set", "delete", "patch", "undo", or "redo".
  }

  window.aidocs.on("kvchange", renderFromKV);
  window.aidocs.off("kvchange", renderFromKV);

  window.addEventListener("kvchange", e => {
    // e.detail is the same event payload.
  });

Semantics:
  - KV is scoped to one document. One document's keys are not visible to another.
  - KV is document-level state, not version-level state.
  - KV writes persist immediately. There is no Save button for KV updates.
  - KV writes do not create document versions.
  - Template/HTML updates still go through the ordinary publish/version flow.
  - Viewers can read KV values. Editors can write KV values.
  - Values must be JSON-serializable.
  - null is a delete sentinel. set(key, null) deletes key, and passing null or
    undefined in patch deletes that key.
  - Keys must be non-empty, at most 128 bytes, and use only letters, numbers,
    dot, underscore, colon, or dash.
  - Values can be up to 64 KiB. A patch can change up to 256 keys and the
    request body can be up to 256 KiB.
  - aidocs keeps an in-session undo/redo stack for local KV writes made through
    window.aidocs.kv. Ctrl/Cmd+Z undoes the last local KV write; Ctrl/Cmd+Y and
    Ctrl/Cmd+Shift+Z redo it. Undo/redo apply ordinary KV patches, persist
    immediately, and emit kvchange events with operation "undo" or "redo".
  - Undo/redo is scoped to the current document iframe session. It does not
    create versions and should not be used as an audit log.
  - Treat kvchange as a render/update signal and read the payload or call
    get/all for the state you need. Do not rely on remote realtime fanout unless
    a future aidocs guideline explicitly promises it.

Small dropdown example:
  <select id="status">
    <option value="pending">Pending</option>
    <option value="approved">Approved</option>
    <option value="blocked">Blocked</option>
  </select>

  <script>
    const key = "approval:status";
    const select = document.querySelector("#status");

    window.aidocs.kv.get(key).then(value => {
      if (value) select.value = value;
    });

    select.addEventListener("change", () => {
      window.aidocs.kv.set(key, select.value);
    });

    window.aidocs.on("kvchange", event => {
      if (event.items && event.items[key]) {
        select.value = event.items[key];
        return;
      }
      if ((event.changes || []).some(change => change.key === key)) {
        window.aidocs.kv.get(key).then(value => {
          if (value) select.value = value;
        });
      }
    });
  </script>
`,
	},
	{
		Path:    []string{"review"},
		Summary: "Make documents easy to comment on, revise, and approve.",
		Aliases: [][]string{{"comments"}, {"accessibility"}, {"a11y"}},
		Body: `aidocs guidelines: review

Write documents for review, not just display.
  - Use real text for important claims, numbers, headings, and decisions.
  - Keep tables and lists selectable whenever possible.
  - Give charts and screenshots nearby captions or summaries so comments can
    refer to them precisely.
  - Preserve stable headings between versions so reviewers can keep orientation.
  - Use accessible labels for controls in interactive documents.
  - Keep approval or tracker controls close to the content they affect.
  - Use KV for live document-local workflow state. Use versioned HTML updates for
    changes to the document template or source content.
`,
	},
}

// Find returns the page whose path or alias matches the normalized path.
func Find(path []string) (Page, bool) {
	for _, p := range Pages {
		if samePath(path, p.Path) {
			return p, true
		}
		for _, alias := range p.Aliases {
			if samePath(path, NormalizePath(alias)) {
				return p, true
			}
		}
	}
	return Page{}, false
}

// NormalizePath lowercases, splits, and canonicalizes user-supplied section
// path segments so lookups tolerate spacing and underscore variants.
func NormalizePath(parts []string) []string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		for _, token := range strings.Fields(strings.ToLower(part)) {
			token = strings.ReplaceAll(token, "_", "-")
			if token != "" {
				out = append(out, token)
			}
		}
	}
	return out
}

func samePath(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
