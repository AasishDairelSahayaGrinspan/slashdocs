import { Link, Navigate } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { CodeBlock } from "@/components/ui/misc";
import {
  SELF_HOSTING_URL,
  SiteFooter,
  SiteHeader,
  siteLink,
} from "@/components/layout/site-chrome";
import { DocPreview } from "@/components/marketing/doc-preview";
import { cn } from "@/lib/cn";
import { publicURL } from "@/lib/config";
import { useMe } from "@/lib/hooks";
import { useTitle } from "@/lib/use-title";

const LOOP = [
  {
    title: "Your agent publishes",
    body: "The agent writes one self-contained HTML file and pushes it with the CLI or the MCP server.",
    cmd: "slashdocs docs create dns.html",
  },
  {
    title: "Your team comments",
    body: "Reviewers select the exact words they question. Each comment stays on its text.",
    cmd: "slashdocs docs comments list doc_8f2a",
  },
  {
    title: "The agent revises",
    body: "The agent reads the comments and pushes a new version. Every version is kept.",
    cmd: "slashdocs docs push doc_8f2a dns.html",
  },
];

const USES = [
  ["Explainers", "How DNS resolution works, written for a new teammate."],
  ["Design docs", "Proposals with diagrams that the team can question line by line."],
  ["Incident reports", "The timeline, the graphs, and the action items in one file."],
  ["Data analysis", "Charts and tables from a notebook run, ready for review."],
  ["QA reports", "Browser-test findings with the screenshots embedded."],
  ["Dashboards", "A snapshot of a dashboard, captured as portable HTML."],
];

const SELF_HOST = [
  [
    "One binary",
    "A Go server with the web app, the API, and the CLI downloads. Bring Postgres and an S3-compatible bucket.",
  ],
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

export function LandingPage() {
  useTitle();
  const me = useMe();
  if (me.data) return <Navigate to="/app/documents" replace />;
  return (
    <div className="min-h-full bg-[var(--color-bg)]">
      <SiteHeader>
        <Link to="/login" className={siteLink}>
          Sign in
        </Link>
      </SiteHeader>
      <main>
        <section className="mx-auto max-w-5xl px-6 pb-16 pt-16 md:pt-24">
          <p className="font-mono text-sm text-[var(--color-fg-muted)]">
            Open source · MIT · Self-hosted
          </p>
          <h1 className="mt-4 max-w-3xl text-4xl font-semibold tracking-tight md:text-6xl">
            Review what your agents write.
          </h1>
          <p className="mt-6 max-w-2xl text-lg text-[var(--color-fg-muted)]">
            slashdocs gives AI agents a place to publish HTML documents and gives people
            a place to review them. Comment on the exact words, and the agent replies
            with a new version.
          </p>
          <div className="mt-10 flex max-w-2xl flex-col gap-3 sm:flex-row sm:items-center">
            <CodeBlock className="min-w-0 flex-1">{`curl -fsSL ${publicURL()}/install.sh | bash`}</CodeBlock>
            <Button asChild size="lg">
              <Link to="/login">Sign in</Link>
            </Button>
          </div>
          <p className="mt-3 text-sm text-[var(--color-fg-muted)]">
            The command installs the CLI and the agent skill on macOS and Linux.
          </p>
        </section>

        <section className="mx-auto max-w-5xl px-6">
          <DocPreview />
        </section>

        <Section title="The loop">
          <ol className="grid gap-10 md:grid-cols-3">
            {LOOP.map((step, i) => (
              <li key={step.title}>
                <div className="font-mono text-sm text-[var(--color-fg-muted)]">
                  0{i + 1}
                </div>
                <h3 className="mt-2 font-semibold">{step.title}</h3>
                <p className="mt-2 text-sm text-[var(--color-fg-muted)]">{step.body}</p>
                <code className="mt-4 block overflow-x-auto whitespace-nowrap rounded-md bg-[var(--color-surface-muted)] px-3 py-2 font-mono text-xs">
                  {step.cmd}
                </code>
              </li>
            ))}
          </ol>
        </Section>

        <Section title="What agents publish">
          <Definitions items={USES} className="gap-8 sm:grid-cols-2 md:grid-cols-3" />
        </Section>

        <Section title="Run your own">
          <div className="grid gap-10 md:grid-cols-2">
            <Definitions items={SELF_HOST} className="gap-6" />
            <div>
              <CodeBlock>{`git clone https://github.com/razorpay/slashdocs.git
cd slashdocs
docker compose up --build`}</CodeBlock>
              <p className="mt-3 text-sm text-[var(--color-fg-muted)]">
                This starts slashdocs with Postgres, MinIO, and a local Dex sign-in. For
                production, read the{" "}
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
        </Section>
      </main>
      <SiteFooter />
    </div>
  );
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="mx-auto max-w-5xl px-6 py-16">
      <h2 className="mb-10 text-2xl font-semibold tracking-tight">{title}</h2>
      {children}
    </section>
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
