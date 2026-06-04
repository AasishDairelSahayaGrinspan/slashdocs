# Report — status updates, RCAs, postmortems, incident timelines

The most common aidocs document. The failure mode is a wall of paragraphs. The
fix is structure that lets a reader find the part they need and trust it fast.

## Status report / weekly update

- **Shipped / In-flight / Blocked** as three visually distinct columns, not one
  long list. One line per item, each linked to its PR/ticket.
- A small chart if there's a trend worth showing (inline SVG — sparkline is fine).
- An **Asks** section, visually separated: what you need from the reader, by when.
- Footer: timestamp + who to ping.

## RCA / postmortem

- **Header block:** incident name, severity, duration, and a one-line customer
  impact summary. A reader who reads only this should still get the gist.
- **Timeline as a real vertical timeline**, not a numbered list: timestamps down
  the left, events on the right, log excerpts inline at the moment they happened.
- **Root cause**, written so someone who didn't follow the timeline understands it.
- **What worked / what didn't** — two columns. Keeps it blameless and scannable.
- **Action items** as a checklist with an owner and a date on each. No owner =
  not an action item.

## Load-bearing

- The customer-impact summary up top (most readers stop after it — make it carry).
- The timeline drawn as a timeline. A numbered list throws away the one thing
  HTML adds here.
- Owned, dated action items.

## Common mistakes

- Burying severity/impact three paragraphs in.
- "Action items" with no owner or date.
- A timeline that's just `1. 2. 3.` — you had position to use and didn't.

## Sketch (timeline row + impact header)

```html
<header class="card">
  <span class="pill pill--negative">SEV1</span>
  <h1>Checkout 5xx spike</h1>
  <p class="lede">14:02–14:51 IST · ~8% of payment attempts failed · refunded.</p>
</header>
<ol class="timeline">
  <li><time>14:02</time><div><b>Error rate climbs.</b>
      <pre><code>pg_pool: 0 idle conns</code></pre></div></li>
  <li><time>14:09</time><div>Paged on-call; rolled back deploy <code>a1b2c3</code>.</div></li>
</ol>
```
(`.card`, `.pill`, `.lede`, `.timeline` come from `aidocs guidelines theme`.)
