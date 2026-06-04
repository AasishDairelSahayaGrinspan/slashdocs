# Theme — the default Razorpay (Blade) look

Paste this `<style>` block into your document `<head>` and build the page from the
`--rzp-*` variables and the semantic classes. It is the DEFAULT for aidocs
documents: a restrained, AA-contrast, system-font token set derived from the
Razorpay Blade design system (github.com/razorpay/blade,
`packages/blade/src/tokens`). It is theme-aware (light + dark via
`data-aidocs-theme`) and fully inline — no web fonts, no external CSS (the render
CSP blocks those).

Source-of-truth notes:
- Tokens are Blade's, expressed as `hsl()` exactly as Blade stores them.
- Accent = Blade **azure**. Use `--rzp-primary` (azure 600) for text/links/borders
  on light surfaces — it clears WCAG AA. Use `--rzp-primary-bold` (azure 500) for
  FILLS, and always pair a fill with `--rzp-on-primary` (white) text.
- Feedback = Blade emerald / crimson / cider / sapphire (positive / negative /
  notice / information). Callouts and pills use the *subtle* background with the
  *intense* foreground — that pairing is AA by construction.
- Brand font is Inter, but web fonts can't load under the CSP, so `--rzp-font-sans`
  is a system stack (Inter is listed so it's used where locally available).

```html
<style>
  :root{
    /* neutrals — Blade ashGray/blueGray family (light) */
    --rzp-bg:hsl(0 0% 100%);
    --rzp-surface:hsl(0 0% 100%);
    --rzp-surface-muted:hsl(220 20% 97%);
    --rzp-border:hsl(220 16% 88%);
    --rzp-ink:hsl(220 26% 14%);
    --rzp-ink-soft:hsl(220 14% 32%);
    --rzp-muted:hsl(220 10% 48%);
    /* primary — Blade azure */
    --rzp-primary:hsl(218 87% 43%);        /* azure.600 — text/links/borders (AA) */
    --rzp-primary-bold:hsl(218 89% 51%);   /* azure.500 — fills */
    --rzp-primary-subtle:hsl(218 90% 96%);
    --rzp-on-primary:hsl(0 0% 100%);
    /* feedback — emerald / crimson / cider / sapphire */
    --rzp-positive:hsl(150 100% 22%); --rzp-positive-subtle:hsl(150 60% 95%);
    --rzp-negative:hsl(4 85% 40%);   --rzp-negative-subtle:hsl(5 80% 96%);
    --rzp-notice:hsl(25 100% 35%);   --rzp-notice-subtle:hsl(25 90% 95%);
    --rzp-information:hsl(200 100% 34%); --rzp-information-subtle:hsl(200 90% 95%);
    /* type — Blade fontSize scale + system stack */
    --rzp-font-sans:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,Inter,Helvetica,Arial,sans-serif;
    --rzp-font-mono:ui-monospace,SFMono-Regular,"SF Mono",Menlo,Consolas,monospace;
    --rzp-fs-75:12px; --rzp-fs-100:14px; --rzp-fs-200:16px; --rzp-fs-400:20px;
    --rzp-fs-500:24px; --rzp-fs-600:32px; --rzp-fs-700:40px;
    /* spacing — Blade 4px grid */
    --rzp-2:4px; --rzp-3:8px; --rzp-4:12px; --rzp-5:16px; --rzp-6:20px;
    --rzp-7:24px; --rzp-8:32px; --rzp-9:40px; --rzp-10:48px;
    /* radii */
    --rzp-r-sm:4px; --rzp-r-md:8px; --rzp-r-lg:12px; --rzp-r-pill:9999px;
    color-scheme:light;
  }
  :root[data-aidocs-theme="dark"]{
    --rzp-bg:hsl(220 30% 7%);
    --rzp-surface:hsl(220 24% 11%);
    --rzp-surface-muted:hsl(220 22% 15%);
    --rzp-border:hsl(220 16% 24%);
    --rzp-ink:hsl(220 22% 92%);
    --rzp-ink-soft:hsl(220 14% 72%);
    --rzp-muted:hsl(220 12% 56%);
    --rzp-primary:hsl(218 90% 72%);        /* lighter azure for dark surfaces (AA) */
    --rzp-primary-bold:hsl(218 89% 56%);
    --rzp-primary-subtle:hsl(218 60% 18%);
    --rzp-on-primary:hsl(220 30% 7%);
    --rzp-positive:hsl(150 65% 62%); --rzp-positive-subtle:hsl(150 40% 16%);
    --rzp-negative:hsl(5 90% 72%);   --rzp-negative-subtle:hsl(5 50% 18%);
    --rzp-notice:hsl(30 95% 64%);    --rzp-notice-subtle:hsl(28 50% 16%);
    --rzp-information:hsl(200 90% 66%); --rzp-information-subtle:hsl(200 50% 16%);
    color-scheme:dark;
  }
  *{box-sizing:border-box}
  body{margin:0;background:var(--rzp-bg);color:var(--rzp-ink);
    font:var(--rzp-fs-200)/1.55 var(--rzp-font-sans);
    -webkit-font-smoothing:antialiased}
  main{max-width:74ch;margin:0 auto;padding:var(--rzp-9) var(--rzp-6)}
  h1{font-size:var(--rzp-fs-700);line-height:1.15;letter-spacing:-.02em;margin:0 0 var(--rzp-5)}
  h2{font-size:var(--rzp-fs-500);line-height:1.25;margin:var(--rzp-9) 0 var(--rzp-4)}
  h3{font-size:var(--rzp-fs-400);margin:var(--rzp-7) 0 var(--rzp-3)}
  a{color:var(--rzp-primary);text-underline-offset:2px}
  .lede{font-size:var(--rzp-fs-400);line-height:1.5;color:var(--rzp-ink-soft);margin:0 0 var(--rzp-7)}
  .card{background:var(--rzp-surface);border:1px solid var(--rzp-border);
    border-radius:var(--rzp-r-lg);padding:var(--rzp-7)}
  .callout{border:1px solid var(--rzp-border);border-radius:var(--rzp-r-md);
    padding:var(--rzp-4) var(--rzp-5);margin:var(--rzp-5) 0}
  .callout--positive{background:var(--rzp-positive-subtle);border-color:transparent;color:var(--rzp-positive)}
  .callout--negative{background:var(--rzp-negative-subtle);border-color:transparent;color:var(--rzp-negative)}
  .callout--notice{background:var(--rzp-notice-subtle);border-color:transparent;color:var(--rzp-notice)}
  .callout--information{background:var(--rzp-information-subtle);border-color:transparent;color:var(--rzp-information)}
  .pill{display:inline-block;font:600 var(--rzp-fs-75)/1.4 var(--rzp-font-sans);
    border-radius:var(--rzp-r-pill);padding:2px 10px}
  .pill--positive{background:var(--rzp-positive-subtle);color:var(--rzp-positive)}
  .pill--negative{background:var(--rzp-negative-subtle);color:var(--rzp-negative)}
  .pill--notice{background:var(--rzp-notice-subtle);color:var(--rzp-notice)}
  .pill--information{background:var(--rzp-information-subtle);color:var(--rzp-information)}
  table{border-collapse:collapse;width:100%;font-size:var(--rzp-fs-100)}
  th,td{text-align:left;padding:var(--rzp-3) var(--rzp-4);border-bottom:1px solid var(--rzp-border)}
  th{color:var(--rzp-muted);font-weight:600;text-transform:uppercase;font-size:var(--rzp-fs-75);letter-spacing:.04em}
  tr:hover td{background:var(--rzp-surface-muted)}
  code,pre{font-family:var(--rzp-font-mono);font-size:var(--rzp-fs-100)}
  pre{background:var(--rzp-surface-muted);border:1px solid var(--rzp-border);
    border-radius:var(--rzp-r-md);padding:var(--rzp-5);overflow:auto}
  .metrics{display:grid;grid-template-columns:auto 1fr;gap:var(--rzp-2) var(--rzp-5);margin:0}
  .metrics dt{color:var(--rzp-muted)} .metrics dd{margin:0;font-variant-numeric:tabular-nums}
  .timeline{list-style:none;margin:0;padding:0}
  .timeline li{display:grid;grid-template-columns:5rem 1fr;gap:var(--rzp-5);
    padding:var(--rzp-4) 0;border-left:2px solid var(--rzp-border);padding-left:var(--rzp-5)}
  .timeline time{color:var(--rzp-muted);font-variant-numeric:tabular-nums}
  .cols{display:grid;grid-template-columns:repeat(auto-fit,minmax(18rem,1fr));gap:var(--rzp-5)}
  @media(max-width:640px){main{padding:var(--rzp-7) var(--rzp-5)}}
</style>
```

## Usage

Wrap content in `<main>` for the reading column. Reach for `.card`, `.callout--*`,
`.pill--*`, `.timeline`, `.metrics`, `.cols` as the category playbooks reference
them. Keep to at most two accents (azure + one feedback color per context). Do not
add a theme toggle — the aidocs UI owns that.
