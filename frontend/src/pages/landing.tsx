import { useState } from "react";
import { Link, Navigate } from "react-router-dom";
import { History, Link2, MessageSquareQuote } from "lucide-react";
import { Button } from "@/components/ui/button";
import { CodeBlock } from "@/components/ui/misc";
import {
  SELF_HOSTING_URL,
  SiteFooter,
  SiteHeader,
  siteLink,
} from "@/components/layout/site-chrome";
import { AgentLoop } from "@/components/marketing/agent-loop";
import { RazorSense } from "@/vendor/razorsense";
import { cn } from "@/lib/cn";
import { publicURL } from "@/lib/config";
import { useMe } from "@/lib/hooks";
import { usePrefersReducedMotion } from "@/lib/use-media-query";
import { useTitle } from "@/lib/use-title";

const PILLARS = [
  {
    icon: Link2,
    title: "A link, not a paste",
    body: "Every document gets a URL. Share it with a person, a Google Group, or the whole organization, and find it again with search.",
  },
  {
    icon: MessageSquareQuote,
    title: "Comments on the exact words",
    body: "Select a sentence and comment. Replies, mentions, and Slack notifications keep people in the loop. Agents get each comment with its quote.",
  },
  {
    icon: History,
    title: "Every version kept",
    body: "Each push makes a new version that never changes. A push from an old version fails, so two agents cannot overwrite each other.",
  },
];

const CONNECT = [
  {
    title: "The CLI and the skill",
    body: "One command installs the CLI and a skill for Claude Code and other agents. The skill teaches the agent to publish and revise.",
    code: `curl -fsSL ${publicURL()}/install.sh | sh
slashdocs auth login ${publicURL()}
slashdocs docs create report.html --title "Q3 incident review"`,
  },
  {
    title: "Remote MCP",
    body: "Add one URL to any MCP client. You sign in through the browser, and there is nothing to install.",
    code: `claude mcp add --transport http slashdocs ${publicURL()}/mcp`,
  },
  {
    title: "Service accounts",
    body: "CI jobs and scheduled agents publish with their own key, not with your account.",
    code: `slashdocs sa create nightly-reports
slashdocs sa key create sa_123
# In CI, set SLASHDOCS_TOKEN to the key, then:
slashdocs docs create report.html --title "Nightly report"`,
  },
];

const SELF_HOST = [
  [
    "One binary",
    "A Go server for the API, the web app, and the CLI downloads, with Postgres and an S3-compatible bucket.",
  ],
  ["Your sign-in", "Any OIDC provider: Google, Okta, Entra ID, Keycloak, or Dex."],
  [
    "Sandboxed documents",
    "Documents render on a separate origin. Their scripts cannot reach your session.",
  ],
  ["MIT licensed", "Read the source, change it, and run it as you like."],
];

const h2 = "text-3xl font-semibold tracking-[-0.02em] md:text-5xl";
const lead = "max-w-[60ch] text-lg text-[var(--color-fg-muted)]";
const tile = "min-w-0 rounded-md border border-[var(--color-border)] p-6 md:p-8";

export function LandingPage() {
  useTitle();
  const me = useMe();
  if (me.data) return <Navigate to="/app/documents" replace />;
  return (
    <div className="relative isolate min-h-full overflow-x-clip bg-[var(--color-bg)]">
      <Halo />
      <SiteHeader>
        <a
          href={SELF_HOSTING_URL}
          target="_blank"
          rel="noreferrer"
          className={cn(siteLink, "max-sm:hidden")}
        >
          Self-hosting
        </a>
        <Link to="/login" className={siteLink}>
          Sign in
        </Link>
      </SiteHeader>
      <main>
        <section className="mx-auto max-w-6xl px-6 pb-24 pt-12 md:pt-20">
          <div className="mx-auto max-w-3xl text-center">
            <p className="font-mono text-xs uppercase tracking-[0.14em] text-[var(--color-accent)]">
              Open source, self-hosted
            </p>
            <h1 className="mt-5 text-4xl font-semibold leading-[1.05] tracking-[-0.02em] md:text-6xl">
              The collaborative workspace for agentic organizations.
            </h1>
            <p className={cn(lead, "mx-auto mt-6 md:text-xl")}>
              Agents publish HTML documents. Your team comments on the exact words.
              Agents read the comments and ship the next version.
            </p>
            <div className="mx-auto mt-10 flex max-w-xl flex-col gap-3 sm:flex-row sm:items-center">
              <CodeBlock className="min-w-0 flex-1 bg-[var(--color-surface)] text-left">{`curl -fsSL ${publicURL()}/install.sh | sh`}</CodeBlock>
              <Button asChild size="lg">
                <Link to="/login">Sign in</Link>
              </Button>
            </div>
          </div>
          <div className="mt-16 md:mt-20">
            <AgentLoop />
          </div>
        </section>

        <section className="border-y border-[var(--color-border)] bg-[var(--color-surface)]">
          <div className="mx-auto grid max-w-6xl gap-14 px-6 py-24 md:grid-cols-[5fr_6fr] md:gap-20 md:py-32">
            <div className="md:sticky md:top-24 md:self-start">
              <h2 className={h2}>Agent work should not end in a chat window.</h2>
              <p className={cn(lead, "mt-6")}>
                An agent writes a twenty-page report in minutes. Then the report stays
                in a terminal, where nobody else can read it or question it.
              </p>
            </div>
            <ol className="grid gap-14">
              {PILLARS.map((p) => (
                <li key={p.title} className="grid grid-cols-[auto_1fr] gap-x-5">
                  <span className="grid h-10 w-10 place-items-center rounded-sm bg-[var(--color-accent-muted)] text-[var(--color-accent)]">
                    <p.icon className="h-5 w-5" strokeWidth={1.75} />
                  </span>
                  <div>
                    <h3 className="text-xl font-semibold tracking-[-0.02em] md:text-2xl">
                      {p.title}
                    </h3>
                    <p className="mt-2 text-[var(--color-fg-muted)]">{p.body}</p>
                  </div>
                </li>
              ))}
            </ol>
          </div>
        </section>

        <section className="mx-auto max-w-6xl px-6 py-24 md:py-32">
          <h2 className={h2}>Works with the agent you already use.</h2>
          <Connect />
        </section>

        <section className="mx-auto max-w-6xl px-6 pb-24 md:pb-32">
          <h2 className={h2}>One HTML file, as rich as a web page.</h2>
          <p className={cn(lead, "mt-6")}>
            Incident reports, specs, architecture reviews, data analysis, QA findings,
            and dashboards.
          </p>
          <div className="mt-12 grid gap-4 md:grid-cols-3">
            <div className={cn(tile, "bg-[var(--color-surface)] md:col-span-2")}>
              <Feature title="Shared state">
                Approvals, checklists, and trackers save to the document. Everyone sees
                the same state.
              </Feature>
              <CodeBlock className="mt-6 bg-[var(--color-bg)]">{`await window.slashdocs.kv.set("approval:pm", {
  status: "approved",
  by: "pm@example.com",
});`}</CodeBlock>
            </div>
            <div className={cn(tile, "bg-[var(--color-accent-muted)]")}>
              <Feature title="Charts, diagrams, and screenshots">
                Inline SVG, Mermaid diagrams, and images embedded in the file. One file
                holds the whole document.
              </Feature>
            </div>
            <div className={cn(tile, "bg-[var(--color-surface-muted)]")}>
              <Feature title="Light and dark">
                Documents follow the theme of the reader, and they print cleanly.
              </Feature>
            </div>
            <div className={cn(tile, "bg-[var(--color-surface)] md:col-span-2")}>
              <Feature title="Files from GitHub">
                Show a file from a repository at a branch, a tag, or a commit. The token
                stays on the server.
              </Feature>
              <CodeBlock className="mt-6 bg-[var(--color-bg)]">{`await window.slashdocs.github.getRawContent({
  owner: "razorpay", repo: "slashdocs", path: "README.md", ref: "main",
});`}</CodeBlock>
            </div>
          </div>
        </section>

        <section className="border-t border-[var(--color-border)]">
          <div className="mx-auto max-w-6xl px-6 py-24 md:py-32">
            <h2 className={h2}>Run it on your own servers.</h2>
            <div className="mt-12 grid gap-12 md:grid-cols-2">
              <div className="min-w-0">
                <CodeBlock className="bg-[var(--color-surface)]">{`git clone https://github.com/razorpay/slashdocs.git
cd slashdocs
docker compose up --build`}</CodeBlock>
                <p className="mt-3 text-sm text-[var(--color-fg-muted)]">
                  This starts slashdocs with Postgres, MinIO, and a local Dex sign-in.
                  For production, read the{" "}
                  <a
                    href={SELF_HOSTING_URL}
                    target="_blank"
                    rel="noreferrer"
                    className="text-[var(--color-fg)] underline underline-offset-2"
                  >
                    self-hosting guide
                  </a>
                  .
                </p>
              </div>
              <dl className="grid content-start gap-x-8 gap-y-10 sm:grid-cols-2">
                {SELF_HOST.map(([term, detail]) => (
                  <div key={term}>
                    <dt className="font-semibold">{term}</dt>
                    <dd className="mt-1 text-sm text-[var(--color-fg-muted)]">
                      {detail}
                    </dd>
                  </div>
                ))}
              </dl>
            </div>
          </div>
        </section>
      </main>
      <SiteFooter />
    </div>
  );
}

// The RazorSense halo behind the top of the page, as in Slash: one paused
// frame of the glass, zoomed in, with only the light sweep moving. The blend
// mode drops the canvas backdrop (white in light mode, black in dark mode).
function Halo() {
  if (usePrefersReducedMotion()) return null;
  return (
    <div
      aria-hidden
      className="pointer-events-none absolute inset-x-0 top-0 -z-10 h-[600px] mix-blend-darken [mask-image:linear-gradient(black_55%,transparent)] dark:mix-blend-screen"
    >
      <RazorSense
        paused
        startTime={8}
        animateLightIndependently
        lightStartFrame={0}
        slitAngle={(20 * Math.PI) / 180}
        numSegments={20}
        zoom={3}
      />
    </div>
  );
}

// Each way to connect an agent, with the commands for the selected one.
function Connect() {
  const [active, setActive] = useState(0);
  return (
    <div className="mt-12 grid gap-8 md:grid-cols-[2fr_3fr] md:gap-12">
      <div role="tablist" aria-label="Ways to connect an agent" className="grid gap-2">
        {CONNECT.map((c, i) => (
          <button
            key={c.title}
            role="tab"
            aria-selected={i === active}
            onClick={() => setActive(i)}
            className={cn(
              "rounded-md border p-5 text-left transition-colors duration-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-accent)]",
              i === active
                ? "border-[var(--color-border-strong)] bg-[var(--color-surface)]"
                : "border-transparent hover:bg-[var(--color-surface-muted)]",
            )}
          >
            <span className="font-semibold">{c.title}</span>
            <span className="mt-1 block text-sm text-[var(--color-fg-muted)]">
              {c.body}
            </span>
          </button>
        ))}
      </div>
      <div role="tabpanel" className={cn(tile, "bg-[var(--color-surface)]")}>
        <h3 className="text-sm font-medium text-[var(--color-fg-muted)]">
          {CONNECT[active].title}
        </h3>
        <CodeBlock
          key={active}
          className="mt-4 animate-[rise_300ms_ease-out] bg-[var(--color-bg)]"
        >
          {CONNECT[active].code}
        </CodeBlock>
      </div>
    </div>
  );
}

function Feature({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <>
      <h3 className="text-lg font-semibold tracking-[-0.02em]">{title}</h3>
      <p className="mt-2 max-w-[52ch] text-sm text-[var(--color-fg-muted)]">
        {children}
      </p>
    </>
  );
}
