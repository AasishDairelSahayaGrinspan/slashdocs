# Comparison — options weighed side by side

Use when the reader has to choose between two or more approaches/designs/vendors.
The reason to reach for HTML here is literal: put the options NEXT TO each other.

## Layout

- **One column per option**, all with the IDENTICAL internal structure so the eye
  compares row-to-row: a one-line framing, the artifact (code/mockup/sketch), a
  pro/con block, and a hard-metrics row.
- **Pro/con as a two-column table inside each option**, not loose bullets.
- **A hard-metrics row** across the bottom of every option, same metrics in the
  same order: bundle size, complexity, migration cost, testability — whatever the
  decision actually turns on. Numbers, not adjectives.
- **A recommendation block at the end that PICKS ONE.** Say which and why, and
  name what you'd graft from the runners-up. "It depends" is not a recommendation.

## Load-bearing

- Identical structure across columns (otherwise it's not a comparison, it's three
  mini-essays).
- The metrics row with real numbers.
- An actual recommendation.

## Common mistakes

- All the code in one block with the comparison underneath as prose.
- Three variations of the same approach dressed up as distinct options.
- Refusing to recommend.
- Columns that drift out of structural sync so rows don't line up.

## Sketch

```html
<div class="cols">
  <section class="card">
    <h3>A · Server-side render</h3>
    <p class="lede">Render on the edge, ship HTML.</p>
    <table><tr><th>Pros</th><th>Cons</th></tr>
      <tr><td>Fast first paint</td><td>No offline</td></tr></table>
    <dl class="metrics"><dt>Bundle</dt><dd>12 KB</dd>
      <dt>Complexity</dt><dd>Low</dd></dl>
  </section>
  <!-- option B, same structure -->
</div>
<section class="card callout callout--information">
  <b>Recommendation: A.</b> First paint dominates our funnel; we can add B's
  prefetch later.
</section>
```
(`.cols` is a CSS grid; classes from `aidocs guidelines theme`.)
