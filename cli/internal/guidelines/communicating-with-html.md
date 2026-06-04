# Overview — communicating with HTML

aidocs exists because Markdown flattens everything into linear text. It can't put
two options next to each other, can't draw a diagram, can't show a timeline as a
timeline. People stop engaging with a long Markdown doc around 100 lines. HTML
lifts that ceiling — and because an aidocs document is something a human will
read, comment on, and act on, the difference between "a doc I'd skim" and "a doc
I'd actually read" is the whole point.

The move in every category is the same: **pick a layout that makes the shape of
the content visible**, instead of translating Markdown headings 1:1 into `<h2>`s.

## When richer layout earns its keep

- **Comparison** — two+ options to weigh. Columns beat stacked sections.
- **Spatial information** — diffs, call graphs, flowcharts, timelines, before/after.
- **Color or hierarchy carries meaning** — severity, status, syntax.
- **Reference material** — navigated non-linearly: jump links, `<details>`, tables.
- **It will be shared** — a spec for leadership, an RCA for a team. People read
  an HTML page far more readily than a Markdown file.

## Universal rules (hold these in every document)

1. **Self-contained, inline only.** One HTML file. CSS in `<style>`, JS in
   `<script>`, images as `data:`/`https:`, diagrams as inline SVG. No external
   CSS/fonts/CDN scripts — the render CSP blocks them. (See the core contract.)
2. **Real layout, not stacked headers.** Comparison → columns. Timeline →
   timeline. Diff → diff. If you're just emitting headings and paragraphs, you
   didn't need HTML.
3. **Readable in five seconds.** Title at the top, a one-paragraph TL;DR or
   framing sentence right below, then the substance.
4. **Real typography.** Body 16px, line length 60–75ch, line-height ~1.5,
   generous spacing. Use the default theme (`aidocs guidelines theme`).
5. **Theme-aware and anchor-safe.** Follow `data-aidocs-theme`; keep text in the
   DOM so comments anchor (see the core contract's three invariants).
6. **Human prose.** Run `aidocs guidelines writing-voice` before publishing.

## Anti-slop checklist (if you hit three of these, restart)

- A full-bleed gradient hero.
- Everything is a rounded, shadowed card in an identical grid.
- Emoji as section headers.
- Four shades of one color, none of them meaning anything.
- Centered everything.
- Decoration that does no work — icons, glassmorphism, gloss for vibe.

Good looks like Stripe's docs, the NYT graphics desk, Ciechanowski's explainers:
calm typography, restrained ink, at most two accents, real diagrams over icon
decoration.

## SVG craftsmanship (diagrams are inline SVG here — no Mermaid)

- Use `viewBox`, not fixed `width`/`height`, so it scales.
- Use `currentColor` for strokes/fills so the figure adapts to light/dark.
- Round coordinates (`x="120"`, not `x="119.7843"`) so it stays hand-editable.
- Group with `<g>`, label edges with direction (arrowheads via `marker-end`).
- Type set in SVG `<text>`, never text-as-paths. No raster fallbacks.

## A note on interactivity

The blog that inspired aidocs celebrates two-way "editor" artifacts with
copy-as-JSON export buttons. aidocs is different: it's a REVIEW surface, and its
round-trip is the **anchored-comment + version loop**, not an export button. So
keep interactivity that aids comprehension (tabs, `<details>`, sliders that move
an SVG, keyboard-nav decks) and skip the throwaway-editor/export pattern. Crucial
constraint: interactive bits must change visuals, not rewrite the prose a reader
might be commenting on.
