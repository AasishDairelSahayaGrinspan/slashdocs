import { Fragment } from "react";

const HOPS = ["Browser", "Resolver", "Root", ".com", "example.com"];

// A still picture of the review screen: an explainer, one anchored comment,
// and the agent's reply. Static on purpose, so it reads like a screenshot.
export function DocPreview() {
  return (
    <figure className="overflow-hidden rounded-[14px] border border-[var(--color-border)] bg-[var(--color-surface)] shadow-[var(--shadow-elevated)]">
      <div className="flex items-center justify-between gap-4 border-b border-[var(--color-border)] px-5 py-3 text-sm">
        <span className="truncate font-medium">How DNS resolution works</span>
        <span className="shrink-0 font-mono text-xs text-[var(--color-fg-muted)]">
          v2 · 1 open comment
        </span>
      </div>
      <div className="grid md:grid-cols-[1fr_280px]">
        <article className="p-6 text-sm leading-relaxed md:p-10">
          <h3 className="text-xl font-semibold tracking-tight">
            How DNS resolution works
          </h3>
          <p className="mt-1 text-xs text-[var(--color-fg-muted)]">
            An explainer for new teammates
          </p>
          <p className="mt-5">
            When you open example.com, your computer needs an IP address. It asks a{" "}
            <mark className="rounded-t-[2px] bg-amber-100 text-zinc-900 shadow-[inset_0_-2px_0_#fbbf24]">
              recursive resolver
            </mark>
            , which walks the DNS tree for you and caches each answer.
          </p>
          <ol className="my-6 flex flex-wrap items-center gap-2 font-mono text-xs">
            {HOPS.map((hop, i) => (
              <Fragment key={hop}>
                {i > 0 && (
                  <li aria-hidden className="text-[var(--color-fg-muted)]">
                    →
                  </li>
                )}
                <li className="rounded-md border border-[var(--color-border)] bg-[var(--color-surface-muted)] px-2 py-1">
                  {hop}
                </li>
              </Fragment>
            ))}
          </ol>
          <p className="text-[var(--color-fg-muted)]">
            Each answer carries a TTL. Until the TTL expires, the resolver answers from
            its cache and skips the walk.
          </p>
        </article>
        <aside className="border-t border-[var(--color-border)] bg-[var(--color-surface-muted)]/40 p-4 text-xs md:border-l md:border-t-0">
          <div className="rounded-[10px] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
            <Author name="Priya" />
            <p className="mt-2 border-l-2 border-amber-400 pl-2 text-[var(--color-fg-muted)]">
              recursive resolver
            </p>
            <p className="mt-2">Who runs this resolver? Name the usual options.</p>
            <div className="mt-3 border-t border-[var(--color-border)] pt-3">
              <Author name="agent" />
              <p className="mt-2">
                Added in v2: your ISP, or a public resolver such as 1.1.1.1.
              </p>
            </div>
          </div>
        </aside>
      </div>
    </figure>
  );
}

function Author({ name }: { name: string }) {
  return (
    <div className="flex items-center gap-2 font-medium">
      <span className="flex h-5 w-5 items-center justify-center rounded-full bg-[var(--color-accent-muted)] text-[10px] uppercase text-[var(--color-accent)]">
        {name[0]}
      </span>
      {name}
    </div>
  );
}
