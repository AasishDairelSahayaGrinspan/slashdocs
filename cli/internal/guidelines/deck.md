# Deck — arrow-key slide presentation

Use when the artifact is a presentation: ~5–20 slides, one idea per slide, big
type, advanced with the keyboard. Not a document with `<h2>`s — a real deck.

## Layout

- One `<section class="slide">` per slide. Big type (titles 40–64px, body
  24–32px), one idea per slide, strong hierarchy, minimal words.
- A slide counter in a corner: `4 / 12`.
- Arrow keys (and optionally space / `F` fullscreen / `Esc` overview) move
  between slides, via INLINE `<script>`.
- 16:9 feel with letterboxing; center the active slide in the viewport.

## Load-bearing — and the aidocs twist

- **Keyboard navigation.** Without it, it's just a tall web page. Inline JS only.
- **ALL SLIDES STAY IN THE DOM.** This is the aidocs-specific rule. Show the
  current slide and hide the rest with **CSS** (`display:none` / opacity /
  transform on a `.is-active` class you toggle). NEVER remove or regenerate slide
  nodes in JS — readers anchor comments to slide text, and unmounting a slide
  destroys its anchors. `display:none` keeps the node (and its text) in the DOM,
  so comments survive; that's the trade you want.
- **A print/scroll fallback.** Because every slide is already in the DOM, a
  `@media print` (or a "show all" toggle) that reveals every slide stacked lets a
  reader read and comment linearly. Cheap, and it makes the deck reviewable.

## Common mistakes

- JS that builds slides from a data array on load (no stable DOM to anchor to).
- Removing non-active slides from the DOM "for performance."
- Paragraphs of body copy on a slide — that's a `report`, not a deck.
- A custom theme toggle, or external fonts for "big display type" (CSP blocks
  them; use the system stack from `aidocs guidelines theme`).

## Sketch

```html
<style>
  .slide{position:fixed;inset:0;display:none;place-content:center;padding:8vmin}
  .slide.is-active{display:grid}
  .counter{position:fixed;right:24px;bottom:16px;color:var(--rzp-muted)}
  @media print{.slide{display:grid !important;position:static;min-height:100vh}}
</style>
<section class="slide is-active"><h1>Title</h1></section>
<section class="slide"><h2>One idea</h2></section>
<div class="counter"><span id="i">1</span> / <span id="n">2</span></div>
<script>
  const s=[...document.querySelectorAll('.slide')]; let i=0;
  const show=n=>{i=Math.max(0,Math.min(s.length-1,n));
    s.forEach((el,k)=>el.classList.toggle('is-active',k===i));
    document.getElementById('i').textContent=i+1;};
  document.getElementById('n').textContent=s.length;
  addEventListener('keydown',e=>{
    if(e.key==='ArrowRight'||e.key===' ')show(i+1);
    if(e.key==='ArrowLeft')show(i-1);});
</script>
```
