import { useTitle } from "@/lib/use-title";
import { Link, Navigate } from "react-router-dom";
import { motion } from "framer-motion";
import {
  ArrowRight,
  BookOpen,
  Bot,
  FileText,
  LayoutDashboard,
  Server,
  Sparkles,
  Terminal as TerminalIcon,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Tooltip } from "@/components/ui/tooltip";
import { CodeBlock } from "@/components/ui/misc";
import { HowItWorks } from "@/components/marketing/how-it-works";
import { Logo } from "@/components/layout/logo";
import { useFrontendSignedOut } from "@/lib/auth-state";
import { useMe } from "@/lib/hooks";
import { publicURL } from "@/lib/config";

// Shared animation variants for the landing page
const stagger = {
  hidden: {},
  visible: { transition: { staggerChildren: 0.12, delayChildren: 0.15 } },
};
const fadeUp = {
  hidden: { opacity: 0, y: 20, filter: "blur(8px)" },
  visible: {
    opacity: 1,
    y: 0,
    filter: "blur(0px)",
    transition: { duration: 0.6, ease: [0.25, 0.4, 0.25, 1] as const },
  },
};
const fadeUpSoft = {
  hidden: { opacity: 0, y: 12, filter: "blur(4px)" },
  visible: {
    opacity: 1,
    y: 0,
    filter: "blur(0px)",
    transition: { duration: 0.5, ease: [0.25, 0.4, 0.25, 1] as const },
  },
};

const useCases = [
  { icon: FileText, title: "Incident reports & RCAs" },
  { icon: LayoutDashboard, title: "Architecture reviews" },
  { icon: BookOpen, title: "Product specs" },
  { icon: Bot, title: "Agent QA findings" },
  { icon: TerminalIcon, title: "Data analysis reports" },
  { icon: Server, title: "Internal dashboards" },
];

export function LandingPage() {
  useTitle();
  const me = useMe();
  const frontendSignedOut = useFrontendSignedOut();
  if (me.data && !me.unauth && !frontendSignedOut) {
    return <Navigate to="/app/documents" replace />;
  }
  return (
    <div className="min-h-full bg-[var(--color-bg)]">
      <Nav />
      <Hero />
      <HowItWorks />
      <Install />
      <UseCases />
      <Footer />
    </div>
  );
}

function Nav() {
  const me = useMe();
  const frontendSignedOut = useFrontendSignedOut();
  const isSignedIn = !!me.data && !me.unauth && !frontendSignedOut;
  return (
    <header className="sticky top-0 z-30 border-b border-[var(--color-border)]/50 bg-[var(--color-bg)]/70 backdrop-blur-xl backdrop-saturate-150">
      <div className="mx-auto flex h-14 max-w-6xl items-center justify-between px-6">
        <Link to="/">
          <Logo className="h-5 w-auto" />
        </Link>
        <nav className="flex items-center gap-2">
          <Tooltip label="GitHub">
            <a
              href="https://github.com/razorpay/aidocs"
              target="_blank"
              rel="noreferrer"
              aria-label="GitHub"
              className="rounded-md p-2 text-[var(--color-fg-muted)] hover:bg-[var(--color-surface-muted)] hover:text-[var(--color-fg)]"
            >
              <GithubIcon />
            </a>
          </Tooltip>
          <Button asChild variant="ghost" size="sm">
            <a href="#install">Install</a>
          </Button>
          <Button asChild size="sm">
            <Link to={isSignedIn ? "/app/documents" : "/login"}>
              {isSignedIn ? "Open app" : "Sign in"} <ArrowRight className="h-4 w-4" />
            </Link>
          </Button>
        </nav>
      </div>
    </header>
  );
}

function Hero() {
  const me = useMe();
  const frontendSignedOut = useFrontendSignedOut();
  const isSignedIn = !!me.data && !me.unauth && !frontendSignedOut;
  return (
    <section className="relative overflow-hidden">
      <div className="absolute inset-0 -z-10 overflow-hidden" aria-hidden>
        <div
          className="absolute -inset-12 animate-[float_25s_ease-in-out_infinite] opacity-60"
          style={{
            backgroundImage:
              "radial-gradient(ellipse at 20% 0%, rgba(32,40,255,0.12), transparent 50%), radial-gradient(ellipse at 80% 10%, rgba(129,140,255,0.12), transparent 50%)",
          }}
        />
        <div
          className="absolute -inset-12 animate-[float_32s_ease-in-out_infinite_reverse] opacity-30"
          style={{
            backgroundImage:
              "radial-gradient(ellipse at 50% 50%, rgba(32,40,255,0.15), transparent 40%)",
          }}
        />
        <div
          className="absolute inset-0 opacity-[0.35] dark:opacity-[0.15]"
          style={{
            backgroundImage:
              "radial-gradient(circle, var(--color-border) 1px, transparent 1px)",
            backgroundSize: "24px 24px",
          }}
        />
      </div>
      <div className="mx-auto max-w-6xl px-6 py-24 md:py-32">
        <motion.div
          className="mx-auto max-w-3xl text-center"
          initial="hidden"
          animate="visible"
          variants={stagger}
        >
          <motion.div
            variants={fadeUp}
            className="mb-6 inline-flex items-center gap-2 rounded-full border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1 text-xs font-medium text-[var(--color-fg-muted)]"
          >
            <Sparkles className="h-3 w-3 text-[var(--color-accent)]" />
            Review layer for AI-generated documents
          </motion.div>
          <motion.h1
            variants={fadeUp}
            className="text-balance text-4xl font-semibold tracking-tight md:text-6xl"
          >
            Give your agents the power to publish{" "}
            <span className="bg-gradient-to-r from-[var(--color-accent)] to-[#818cff] bg-clip-text text-transparent">
              reviewable
            </span>{" "}
            documents.
          </motion.h1>
          <motion.p
            variants={fadeUp}
            className="mx-auto mt-6 max-w-2xl text-pretty text-base text-[var(--color-fg-muted)] md:text-lg"
          >
            aidocs is a Google-Docs-style review layer for self-contained HTML
            artifacts. Agents push reports, specs, and RCAs through a CLI. Humans review
            them in the browser with anchored comments and immutable versions.
          </motion.p>
          <motion.div
            variants={fadeUp}
            className="mt-8 flex flex-wrap items-center justify-center gap-3"
          >
            <Button asChild size="lg">
              <Link to={isSignedIn ? "/app/documents" : "/login"}>
                {isSignedIn ? "Open app" : "Sign in"} <ArrowRight className="h-4 w-4" />
              </Link>
            </Button>
            <Button asChild size="lg" variant="outline">
              <a href="#install">Install the CLI</a>
            </Button>
          </motion.div>
        </motion.div>
      </div>
    </section>
  );
}

function Install() {
  // Origin comes from the server-injected AIDOCS_APP_ORIGIN (same pattern as
  // the setup guide and developers page), so the snippet matches this
  // instance instead of a hard-coded host.
  const installSnippet = `curl -fsSL ${publicURL()}/install.sh | bash

aidocs auth login ${publicURL()}
aidocs docs create report.html`;
  return (
    <motion.section
      id="install"
      initial={{ opacity: 0, y: 32 }}
      whileInView={{ opacity: 1, y: 0 }}
      viewport={{ once: true, margin: "-80px" }}
      transition={{ duration: 0.7, ease: [0.25, 0.4, 0.25, 1] as const }}
      className="border-t border-[var(--color-border)] py-24"
    >
      <div className="mx-auto max-w-2xl px-6">
        <div className="mb-8 text-center">
          <h2 className="text-3xl font-semibold tracking-tight md:text-4xl">
            Install in seconds
          </h2>
          <p className="mt-2 text-[var(--color-fg-muted)]">
            One command for macOS and Linux.
          </p>
        </div>
        <div className="relative">
          <div
            aria-hidden
            className="pointer-events-none absolute -inset-4 rounded-2xl bg-[var(--color-accent)] opacity-[0.04] blur-2xl"
          />
          <CodeBlock>{installSnippet}</CodeBlock>
        </div>
        <p className="mt-4 text-center text-sm text-[var(--color-fg-muted)]">
          Other install methods are documented in the{" "}
          <a
            href="https://github.com/razorpay/aidocs"
            className="text-[var(--color-fg)] underline-offset-2 hover:underline"
            target="_blank"
            rel="noreferrer"
          >
            GitHub repo
          </a>
          .
        </p>
      </div>
    </motion.section>
  );
}

function UseCases() {
  return (
    <motion.section
      initial={{ opacity: 0, y: 32 }}
      whileInView={{ opacity: 1, y: 0 }}
      viewport={{ once: true, margin: "-80px" }}
      transition={{ duration: 0.7, ease: [0.25, 0.4, 0.25, 1] as const }}
      className="border-t border-[var(--color-border)] bg-[var(--color-surface)]/50 py-24"
    >
      <div className="mx-auto max-w-6xl px-6">
        <div className="mb-10 text-center">
          <h2 className="text-3xl font-semibold tracking-tight md:text-4xl">
            What agents publish
          </h2>
          <p className="mt-2 text-[var(--color-fg-muted)]">
            Anything that fits in a self-contained HTML file.
          </p>
        </div>
        <motion.div
          className="grid grid-cols-2 gap-3 md:grid-cols-3"
          initial="hidden"
          whileInView="visible"
          viewport={{ once: true, margin: "-60px" }}
          variants={{ hidden: {}, visible: { transition: { staggerChildren: 0.08 } } }}
        >
          {useCases.map((u) => (
            <motion.div
              key={u.title}
              variants={fadeUpSoft}
              className="flex items-center gap-3 rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-[box-shadow,transform] duration-200 ease-out hover:-translate-y-0.5 hover:shadow-md"
            >
              <div className="flex h-9 w-9 items-center justify-center rounded-md bg-[var(--color-accent-muted)] text-[var(--color-accent)]">
                <u.icon className="h-4 w-4" />
              </div>
              <div className="text-sm font-medium">{u.title}</div>
            </motion.div>
          ))}
        </motion.div>
      </div>
    </motion.section>
  );
}

function GithubIcon() {
  return (
    <svg viewBox="0 0 24 24" className="h-4 w-4" fill="currentColor" aria-hidden>
      <path d="M12 .5C5.65.5.5 5.65.5 12c0 5.08 3.29 9.39 7.86 10.91.58.1.79-.25.79-.56v-2.18c-3.2.7-3.87-1.36-3.87-1.36-.53-1.34-1.29-1.7-1.29-1.7-1.05-.72.08-.7.08-.7 1.16.08 1.77 1.19 1.77 1.19 1.03 1.77 2.7 1.26 3.36.96.1-.75.4-1.26.73-1.55-2.55-.29-5.24-1.28-5.24-5.69 0-1.26.45-2.29 1.19-3.1-.12-.29-.51-1.46.11-3.05 0 0 .97-.31 3.18 1.18.92-.26 1.91-.39 2.89-.39.98 0 1.97.13 2.89.39 2.21-1.49 3.18-1.18 3.18-1.18.62 1.59.23 2.76.11 3.05.74.81 1.19 1.84 1.19 3.1 0 4.42-2.69 5.4-5.25 5.68.41.36.78 1.07.78 2.15v3.19c0 .31.21.67.8.56C20.22 21.39 23.5 17.08 23.5 12 23.5 5.65 18.35.5 12 .5z" />
    </svg>
  );
}

function Footer() {
  return (
    <footer className="relative py-10">
      <div
        aria-hidden
        className="absolute inset-x-0 top-0 h-px bg-gradient-to-r from-transparent via-[var(--color-border-strong)] to-transparent"
      />
      <div className="mx-auto flex max-w-6xl flex-col items-center justify-between gap-3 px-6 text-sm text-[var(--color-fg-muted)] md:flex-row">
        <Logo className="h-4 w-auto" />
        <div className="flex items-center gap-4">
          <a href="/openapi.json" className="hover:text-[var(--color-fg)]">
            OpenAPI
          </a>
          <a href="/api-docs" className="hover:text-[var(--color-fg)]">
            API docs
          </a>
          <a
            href="https://github.com/razorpay/aidocs"
            target="_blank"
            rel="noreferrer"
            className="hover:text-[var(--color-fg)]"
          >
            GitHub
          </a>
        </div>
      </div>
    </footer>
  );
}
