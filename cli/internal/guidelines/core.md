aidocs document authoring guidelines (core contract)

aidocs is a place to publish a document people will READ and COMMENT on — think
Google Docs, except the medium is one self-contained HTML file. HTML earns its
keep here: it shows comparisons side by side, draws real diagrams, lays out a
timeline as a timeline. Use that. A wall of stacked headings is a wasted page.

Before you write, pick the layout that fits what you're communicating, then read
that playbook:

  aidocs guidelines <category>

Categories:
  report      status updates, RCAs, postmortems, incident timelines
  comparison  options/approaches weighed side by side, recommendations
  pr-review   annotated diffs, code review, module maps, "explain this code"
  explainer   concept explainers, feature deep-dives, diagrams, learning material
  deck        arrow-key slide presentations
  theme       the default Razorpay (Blade) look — colors, type, spacing, classes
  writing-voice  make the prose read human, not AI-generated (run before publish)

================================================================================
THREE INVARIANTS — every aidocs document must hold all three.
================================================================================

1. INLINE ONLY. A document is ONE HTML file rendered under a strict Content
   Security Policy. External resources DO NOT LOAD:
     - NO external stylesheets (<link rel="stylesheet">) — put CSS in <style>.
     - NO web fonts (Google Fonts, @font-face from a URL) — use a system font
       stack. The default theme already does this.
     - NO external/CDN scripts — put JS in <script>. This means NO Mermaid, no
       CDN chart libraries. Draw diagrams as INLINE SVG instead.
     - Images: data: URIs (data:image/png;base64,...) or https: URLs only.
       aidocs hosts no assets, so prefer base64-embedding images you control.
   Inline <style> and inline <script> both work — interactivity is fine, it just
   can't be fetched from the network.

2. FOLLOW THE READER'S THEME. The viewer chooses light or dark in the aidocs UI.
   The render bridge sets and keeps in sync, on <html> inside your document:

       <html data-aidocs-theme="light">
       <html data-aidocs-theme="dark">

   Drive the page from CSS variables keyed on that attribute (the default theme
   from `aidocs guidelines theme` does this for you):

       :root                           { --bg: #fff;    --ink: #111; }
       :root[data-aidocs-theme="dark"] { --bg: #0b0d12; --ink: #e7ecf3; }
       body { background: var(--bg); color: var(--ink); }
       html { color-scheme: light; }
       :root[data-aidocs-theme="dark"] { color-scheme: dark; }

   Notes:
     - "system" is resolved to concrete light/dark before it reaches you, and the
       attribute updates live when the reader toggles — CSS reacts on its own.
     - For JS: window.addEventListener('aidocs:theme', e => e.detail.theme).
     - Do NOT ship your own theme toggle. The aidocs UI is the single control.
     - Theming is opt-in: a document that never references data-aidocs-theme keeps
       its hard-coded look. The DEFAULT is the Razorpay/Blade theme — see
       `aidocs guidelines theme`.

3. KEEP TEXT ANCHORABLE. Readers select text and leave anchored comments. If your
   document REMOVES or REWRITES its text after load, those anchors break. So:
     - Prefer CSS to show/hide (display, opacity), never JS that deletes nodes.
     - <details>/<summary> is safe — collapsed content stays in the DOM.
     - Decks keep ALL slides in the DOM (see `aidocs guidelines deck`).
     - Interactive demos should change visuals (an SVG, a number), not rewrite
       the prose a reader might comment on.

================================================================================
DEFAULT LOOK
================================================================================
Unless the user asks otherwise, start from the Razorpay (Blade) theme:
`aidocs guidelines theme`. It is a tasteful, restrained, AA-contrast token set
(system fonts, Razorpay azure accent, calm neutrals, light + dark) you paste
inline. Resist the generic-AI look: no gradient hero, no grid of identical
shadowed cards, no emoji section headers, no four shades of one color. Body text
16px, line length 60–75ch, generous spacing.

================================================================================
BEFORE YOU PUBLISH
================================================================================
Run the prose through `aidocs guidelines writing-voice` so the document does not
read as machine-generated. Then publish:

  aidocs docs create report.html --title 'Report' --visibility private
  aidocs docs push <doc_id> report.html --summary 'Address comments'
