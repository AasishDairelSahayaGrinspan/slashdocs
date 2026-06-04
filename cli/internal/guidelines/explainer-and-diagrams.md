# Explainer & diagrams — concepts, feature deep-dives, figures

Use when the job is to make someone UNDERSTAND something: a concept, how a
feature works, an architecture. The lever HTML gives you is a diagram a reader
can see and, where it helps, a demo they can poke.

## Concept explainer

- Title, subtitle, one-paragraph TL;DR before any detail.
- The **core insight as a single sentence** with the key term emphasized.
- A **live interactive demo** where the concept is spatial or stateful — a slider
  or toggle that moves an SVG / changes a number. Inline JS only.
- A comparison to the naive approach, **with numbers**.
- "Where you'll meet this" — real systems/code where it shows up.

## Feature deep-dive (understanding code in a repo)

- TL;DR box: what it does, where it lives, key files.
- **Collapsible sections per lifecycle phase** using `<details>` (deep stuff
  collapsed by default — and `<details>` keeps its text in the DOM, so comments
  still anchor).
- Tabbed code snippets (same logic across languages/tests/config) via inline JS
  that toggles visibility — don't destroy the inactive tabs' nodes.
- A short FAQ, then "where to look next."

## Diagrams / figure sheets

- Each figure in a `<figure>` with a `<figcaption>`, drawn as **inline SVG**.
- One consistent visual language across the set: same stroke weight, arrowhead
  style, palette, type.
- Follow the SVG craftsmanship rules in `aidocs guidelines overview` (`viewBox`,
  `currentColor`, rounded coords, direction on edges).

## The anchor-safe interactivity rule

Interactive demos must change **visuals, not prose**. A slider that redraws an SVG
or updates a readout: good. JS that rewrites paragraphs of explanatory text:
breaks comment anchors — don't. Keep the words a reader might highlight stable.

## Load-bearing

- The demo (when the idea is spatial) and the comparison-to-naive with numbers.
- `<details>` for depth instead of one endless scroll.
- Diagrams as inline SVG with a consistent visual language.

## Common mistakes

- A Mermaid block (won't render — CSP). Draw inline SVG.
- A demo that rewrites text on interaction (kills anchoring).
- Bottom-of-page glossary nobody reads — define terms inline or in the margin.

## Sketch (anchor-safe SVG demo)

```html
<label>Replicas <input id="r" type="range" min="1" max="6" value="3"></label>
<svg id="fig" viewBox="0 0 240 60" aria-label="replica fan-out">
  <g id="nodes" fill="currentColor"></g>
</svg>
<script>
  const draw=n=>{const g=document.getElementById('nodes');g.textContent='';
    for(let i=0;i<n;i++){const c=document.createElementNS('http://www.w3.org/2000/svg','circle');
      c.setAttribute('cx',20+i*40);c.setAttribute('cy',30);c.setAttribute('r',10);g.appendChild(c);}};
  const r=document.getElementById('r');draw(+r.value);
  r.addEventListener('input',()=>draw(+r.value));   // moves the figure, not the prose
</script>
```
