# PR & code review — annotated diffs, reviews, module maps

Three related shapes. All of them keep code readable and put the commentary
where it belongs.

## 1. Annotated diff (for review)

- Title with the PR/branch, one-line summary, and a **"where to focus"** framing
  block up top — tell the reviewer what matters before they read line one.
- The diff with `+`/`-` line styling and syntax colors via inline CSS.
- **Annotations in the margin**, not interleaved between code lines — interleaving
  destroys the reading flow of the code. A margin note points at its lines.
- **Severity tags**, color-coded and labelled: blocking, nit, question, praise.
  Color plus a word — never color alone.

## 2. PR writeup (for the author / reviewers)

- Imperative title ("Add streaming to /chat").
- Motivation (why), then a **before/after shown side by side** (real visual, not
  two prose paragraphs).
- A **file-by-file tour grouped by theme**, not alphabetically.
- "Where to focus the review", risks, and how it was tested.

## 3. Module map / "explain this code"

- One-sentence summary, then a **boxes-and-arrows inline-SVG diagram** of the
  modules and how they call each other. The diagram is the whole point — it's
  what you can't do in Markdown.
- Highlight the hot path in a distinct color; call out entry points ("trying to
  do X? start at Y").
- Per-module cards: what it does, key types/functions, gotchas. Then trace one
  realistic input through the system.

## Load-bearing

- Margin annotations, not interleaved comments.
- Severity color + label.
- The diagram (for the module map) — and it's inline SVG, never Mermaid.
- "Where to focus."

## Common mistakes

- Comments jammed between code lines so the code is unreadable.
- A module map written as prose because Mermaid "didn't render" (it can't — CSP).
- Color-only severity with no text label.

## Sketch (severity tag + margin annotation)

```html
<style>
  .diff{display:grid;grid-template-columns:1fr 16rem;gap:1rem}
  .tag{font:600 12px/1 var(--rzp-font-mono);border-radius:9999px;padding:2px 8px}
  .tag--block{background:var(--rzp-negative-subtle);color:var(--rzp-negative)}
  .add{background:var(--rzp-positive-subtle)} .del{background:var(--rzp-negative-subtle)}
</style>
<div class="diff">
  <pre><code><span class="del">- timeout := 5</span>
<span class="add">+ timeout := cfg.Timeout</span></code></pre>
  <aside><span class="tag tag--block">blocking</span>
    cfg.Timeout can be 0 — guard it.</aside>
</div>
```
