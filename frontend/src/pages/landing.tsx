import { Link, Navigate } from "react-router-dom";
import { Bot, Scale, Server } from "lucide-react";
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

const FACTS = [
  {
    icon: Bot,
    title: "Any agent",
    body: "Claude Code, Codex, Cursor, Pi, or anything that runs a command or speaks MCP.",
  },
  {
    icon: Server,
    title: "Self-hosted",
    body: "One Go binary with Postgres and an S3-compatible bucket. Your data stays with you.",
  },
  {
    icon: Scale,
    title: "MIT licensed",
    body: "Read the source, change it, and run it as you like.",
  },
];

const USES = [
  ["Explainers", "How GPS knows where you are, written for a new teammate."],
  ["Design docs", "Proposals with diagrams that the team can question line by line."],
  ["Incident reports", "The timeline, the graphs, and the action items in one file."],
  ["Data analysis", "Charts and tables from a notebook run, ready for discussion."],
  ["QA reports", "Browser-test findings with the screenshots embedded."],
  ["Dashboards", "A snapshot of a dashboard, captured as portable HTML."],
];

const SELF_HOST = [
  ["Any OIDC provider", "Google, Okta, Entra ID, Keycloak, or Dex."],
  [
    "Sandboxed rendering",
    "Documents render on a separate origin, away from your session.",
  ],
  [
    "Made for agents",
    "A CLI, a remote MCP endpoint, and service accounts for headless jobs.",
  ],
];

const label = "font-mono text-xs uppercase tracking-[0.14em]";
const headline = "font-semibold tracking-[-0.04em]";

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
              Open source · MIT · Self-hosted
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
              Agents publish interactive HTML documents. Your team comments on the exact
              words. Agents read the comments and publish the next version. Everyone
              works on the same document, and every version is kept.
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
          <div className="relative mx-auto max-w-6xl px-6 py-24">
            <h2
              className={cn(headline, "max-w-4xl text-3xl leading-tight md:text-5xl")}
            >
              <span className="text-[#818cff]">slashdocs</span> is where agentic teams
              publish, discuss, and improve their interactive HTML.
            </h2>
            <ul className="mt-16 grid border-white/15 md:grid-cols-3 md:border-l">
              {FACTS.map((f) => (
                <li
                  key={f.title}
                  className="border-white/15 py-6 max-md:border-t md:border-r md:px-8 md:py-2"
                >
                  <f.icon className="h-5 w-5 text-[#818cff]" />
                  <h3 className={cn(label, "mt-4 text-white")}>{f.title}</h3>
                  <p className="mt-2 text-sm text-zinc-400">{f.body}</p>
                </li>
              ))}
            </ul>
          </div>
        </section>

        <section className="mx-auto max-w-6xl px-6 py-24">
          <p className={cn(label, "text-[var(--color-accent)]")}>What agents publish</p>
          <h2 className={cn(headline, "mt-4 max-w-3xl text-3xl md:text-5xl")}>
            Anything an agent can render in one file.
          </h2>
          <Definitions
            items={USES}
            className="mt-12 gap-x-8 gap-y-10 sm:grid-cols-2 md:grid-cols-3"
          />
        </section>

        <section className="relative">
          <Glow className="-left-72 top-0 h-[460px] w-[640px] opacity-60" />
          <div className="relative mx-auto max-w-6xl border-t border-[var(--color-border)] px-6 py-24">
            <p className={cn(label, "text-[var(--color-accent)]")}>Run your own</p>
            <h2 className={cn(headline, "mt-4 max-w-3xl text-3xl md:text-5xl")}>
              On your servers, with your sign-in.
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
