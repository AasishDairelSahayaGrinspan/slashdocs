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
import { cn } from "@/lib/cn";
import { publicURL } from "@/lib/config";
import { useMe } from "@/lib/hooks";
import { useTitle } from "@/lib/use-title";

const PILLARS = [
  {
    icon: Link2,
    title: "A link, not a paste",
    body: "Each document gets a URL. Share it with a person, a Google Group, or the whole organization. Folders, labels, and search find it again next month.",
  },
  {
    icon: MessageSquareQuote,
    title: "Comments on the exact words",
    body: "Select a sentence and comment. Reply, mention, and react, with Slack notifications. The agent gets each comment with the quote it is about.",
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
    body: "The install command adds a skill for Claude Code and for agents that read ~/.agents/skills. The skill teaches the agent to write, publish, and revise.",
    code: "slashdocs docs create report.html",
  },
  {
    title: "Remote MCP",
    body: "Add one URL to any MCP client. You sign in through the browser with OAuth. There is nothing to install.",
    code: `${publicURL()}/mcp`,
  },
  {
    title: "Service accounts",
    body: "CI jobs and scheduled agents publish with their own key, not with your account. Give the job the key as SLASHDOCS_TOKEN.",
    code: "slashdocs sa key create sa_123",
  },
];

const DOCUMENT = [
  [
    "Charts, diagrams, and screenshots",
    "Inline SVG, Mermaid diagrams, and images embedded in the file.",
  ],
  [
    "Shared state",
    "Approvals, checklists, and trackers save to the document. Everyone sees the same state.",
  ],
  [
    "Files from GitHub",
    "Show a file from a repository at a branch, a tag, or a commit.",
  ],
  [
    "Light and dark",
    "Documents follow the theme of the reader, and they print cleanly.",
  ],
];

const SELF_HOST = [
  [
    "One binary",
    "A Go server for the API, the web app, and the CLI downloads. You supply Postgres and an S3-compatible bucket.",
  ],
  ["Your sign-in", "Any OIDC provider: Google, Okta, Entra ID, Keycloak, or Dex."],
  [
    "Sandboxed documents",
    "Documents render on a separate origin. Their scripts cannot reach your session.",
  ],
  ["MIT licensed", "Read the source, change it, and run it as you like."],
];

const label = "font-mono text-xs uppercase tracking-[0.14em]";
const headline = "font-semibold tracking-[-0.04em]";
const section = "mx-auto max-w-6xl px-6 py-24";

export function LandingPage() {
  useTitle();
  const me = useMe();
  if (me.data) return <Navigate to="/app/documents" replace />;
  return (
    <div className="min-h-full overflow-x-clip bg-[var(--color-bg)]">
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
        <section className="relative">
          <Glow className="-top-48 left-1/3 h-[560px] w-[900px]" />
          <Glow className="right-[-240px] top-[560px] h-[520px] w-[520px]" />
          <div className="relative mx-auto max-w-6xl px-6 pb-24 pt-14 md:pt-24">
            <p className={cn(label, "text-[var(--color-accent)]")}>
              Open source · Self-hosted
            </p>
            <h1
              className={cn(
                headline,
                "mt-5 max-w-4xl text-5xl leading-[0.95] md:text-7xl",
              )}
            >
              The collaborative workspace for agentic organizations.
            </h1>
            <p className="mt-6 max-w-2xl text-lg text-[var(--color-fg-muted)] md:text-xl">
              Your agents write the reports, specs, and analyses. slashdocs gives each
              one a link, a comment thread, and a version history. Your team comments in
              the browser, and your agents publish the next version.
            </p>
            <div className="mt-10 flex max-w-2xl flex-col gap-3 sm:flex-row sm:items-center">
              <CodeBlock className="min-w-0 flex-1 bg-[var(--color-surface)]">{`curl -fsSL ${publicURL()}/install.sh | sh`}</CodeBlock>
              <Button asChild size="lg">
                <Link to="/login">Sign in</Link>
              </Button>
            </div>
            <p className="mt-3 text-sm text-[var(--color-fg-muted)]">
              The command installs the CLI and the agent skill on macOS and Linux.
            </p>
            <div className="mt-20">
              <AgentLoop />
            </div>
          </div>
        </section>

        <section className="relative overflow-hidden bg-[#0b0b10] bg-[radial-gradient(rgba(255,255,255,0.08)_1px,transparent_1px)] bg-[size:12px_12px] text-white">
          <Glow className="-left-60 -top-60 h-[520px] w-[720px] opacity-40" />
          <div className={cn(section, "relative")}>
            <p className={cn(label, "text-[#818cff]")}>Why</p>
            <h2
              className={cn(
                headline,
                "mt-4 max-w-4xl text-3xl leading-tight md:text-5xl",
              )}
            >
              Agent work should not end in a chat window.
            </h2>
            <p className="mt-6 max-w-2xl text-lg text-zinc-400">
              An agent can write a twenty-page incident report in minutes. Then the
              report stays in a terminal or a chat history, where nobody else can read
              it, question it, or find it again.
            </p>
            <ul className="mt-16 grid border-white/15 md:grid-cols-3 md:border-l">
              {PILLARS.map((p) => (
                <li
                  key={p.title}
                  className="border-white/15 py-6 max-md:border-t md:border-r md:px-8 md:py-2"
                >
                  <p.icon className="h-5 w-5 text-[#818cff]" />
                  <h3 className="mt-4 font-semibold">{p.title}</h3>
                  <p className="mt-2 text-sm text-zinc-400">{p.body}</p>
                </li>
              ))}
            </ul>
          </div>
        </section>

        <section className={section}>
          <p className={cn(label, "text-[var(--color-accent)]")}>For agents</p>
          <h2 className={cn(headline, "mt-4 max-w-3xl text-3xl md:text-5xl")}>
            Works with the agent you already use.
          </h2>
          <div className="mt-12 grid gap-10 md:grid-cols-3">
            {CONNECT.map((c) => (
              <div key={c.title} className="flex min-w-0 flex-col">
                <h3 className="font-semibold">{c.title}</h3>
                <p className="mb-4 mt-1 flex-1 text-sm text-[var(--color-fg-muted)]">
                  {c.body}
                </p>
                <CodeBlock className="bg-[var(--color-surface)]">{c.code}</CodeBlock>
              </div>
            ))}
          </div>
        </section>

        <section className="border-y border-[var(--color-border)] bg-[var(--color-surface)]">
          <div className={section}>
            <p className={cn(label, "text-[var(--color-accent)]")}>The document</p>
            <h2 className={cn(headline, "mt-4 max-w-3xl text-3xl md:text-5xl")}>
              One HTML file, as rich as a web page.
            </h2>
            <p className="mt-6 max-w-2xl text-lg text-[var(--color-fg-muted)]">
              Teams publish incident reports, specs, architecture reviews, data
              analysis, QA findings, and dashboards.
            </p>
            <div className="mt-12 grid gap-12 md:grid-cols-2">
              <Definitions items={DOCUMENT} className="gap-7" />
              <div className="min-w-0">
                <CodeBlock className="bg-[var(--color-bg)]">{`// In the document: record an approval for everyone.
await window.slashdocs.kv.set("approval:pm", {
  status: "approved",
  by: "pm@example.com",
});`}</CodeBlock>
                <p className="mt-3 text-sm text-[var(--color-fg-muted)]">
                  Agents learn this API from <code>slashdocs guidelines</code>.
                </p>
              </div>
            </div>
          </div>
        </section>

        <section className="relative">
          <Glow className="-left-72 top-0 h-[460px] w-[640px] opacity-60" />
          <div className={cn(section, "relative")}>
            <p className={cn(label, "text-[var(--color-accent)]")}>Self-hosting</p>
            <h2 className={cn(headline, "mt-4 max-w-3xl text-3xl md:text-5xl")}>
              Run it on your own servers.
            </h2>
            <div className="mt-12 grid gap-12 md:grid-cols-2">
              <Definitions items={SELF_HOST} className="gap-7" />
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
            </div>
          </div>
        </section>
      </main>
      <SiteFooter />
    </div>
  );
}

// Soft blue light, like the brand gradients.
function Glow({ className }: { className: string }) {
  return (
    <div
      aria-hidden
      className={cn(
        "pointer-events-none absolute rounded-full bg-[radial-gradient(closest-side,#4a6bff,#818cff_45%,transparent)] opacity-25 blur-3xl",
        className,
      )}
    />
  );
}

function Definitions({ items, className }: { items: string[][]; className: string }) {
  return (
    <dl className={cn("grid", className)}>
      {items.map(([term, detail]) => (
        <div key={term}>
          <dt className="font-semibold">{term}</dt>
          <dd className="mt-1 text-sm text-[var(--color-fg-muted)]">{detail}</dd>
        </div>
      ))}
    </dl>
  );
}
