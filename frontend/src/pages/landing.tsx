import { useTitle } from "@/lib/use-title";
import { Link, Navigate } from "react-router-dom";
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
import { CodeBlock } from "@/components/ui/misc";
import { HowItWorks } from "@/components/marketing/how-it-works";
import { useFrontendSignedOut } from "@/lib/auth-state";
import { useMe } from "@/lib/hooks";

const INSTALL_SNIPPET = `curl -fsSL https://aidocs.concierge.razorpay.com/install.sh | bash

aidocs auth login
aidocs docs create report.html`;

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
    <div style={{ minHeight: "100%", background: "#fafafa" }}>
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
    <header style={{ position: "sticky", top: 0, zIndex: 30, borderBottom: "1px solid #e4e4e7", background: "rgba(250,250,250,0.8)", backdropFilter: "blur(8px)" }}>
      <div style={{ margin: "0 auto", display: "flex", height: "56px", maxWidth: "1152px", alignItems: "center", justifyContent: "space-between", padding: "0 24px" }}>
        <Link to="/" style={{ display: "flex", alignItems: "center", gap: "8px", fontWeight: 600 }}>
          <img src="/favicon.svg" alt="" style={{ height: "40px", width: "40px" }} />
          aidocs
        </Link>
        <nav style={{ display: "flex", alignItems: "center", gap: "8px" }}>
          <a
            href="https://github.com/razorpay/aidocs"
            target="_blank"
            rel="noreferrer"
            aria-label="GitHub"
            style={{ borderRadius: "6px", padding: "8px", color: "#71717a" }}
          >
            <GithubIcon />
          </a>
          <Button asChild variant="ghost" size="sm">
            <a href="#install">Install</a>
          </Button>
          <Button asChild size="sm">
            <Link to={isSignedIn ? "/app/documents" : "/login"}>
              {isSignedIn ? "Open app" : "Sign in"} <ArrowRight width={16} height={16} />
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
    <section style={{ position: "relative", overflow: "hidden" }}>
      <div
        style={{
          position: "absolute",
          inset: 0,
          zIndex: -1,
          opacity: 0.6,
          backgroundImage:
            "radial-gradient(ellipse at 20% 0%, rgba(32,40,255,0.10), transparent 50%), radial-gradient(ellipse at 80% 10%, rgba(129,140,255,0.10), transparent 50%)",
        }}
      />
      <div style={{ margin: "0 auto", maxWidth: "1152px", padding: "96px 24px" }}>
        <div style={{ margin: "0 auto", maxWidth: "768px", textAlign: "center" }}>
          <div style={{ marginBottom: "24px", display: "inline-flex", alignItems: "center", gap: "8px", borderRadius: "9999px", border: "1px solid #e4e4e7", background: "#fff", padding: "4px 12px", fontSize: "12px", fontWeight: 500, color: "#71717a" }}>
            <Sparkles width={12} height={12} style={{ color: "#2028ff" }} />
            Review layer for AI-generated documents
          </div>
          <h1 style={{ fontSize: "36px", fontWeight: 600, letterSpacing: "-0.02em" }}>
            Give your agents the power to publish{" "}
            <span style={{ color: "#2028ff" }}>reviewable</span> documents.
          </h1>
          <p style={{ margin: "24px auto 0", maxWidth: "672px", fontSize: "16px", color: "#71717a" }}>
            aidocs is a Google-Docs-style review layer for self-contained HTML
            artifacts. Agents push reports, specs, and RCAs through a CLI. Humans review
            them in the browser with anchored comments and immutable versions.
          </p>
          <div style={{ marginTop: "32px", display: "flex", flexWrap: "wrap", alignItems: "center", justifyContent: "center", gap: "12px" }}>
            <Button asChild size="lg">
              <Link to={isSignedIn ? "/app/documents" : "/login"}>
                {isSignedIn ? "Open app" : "Sign in"} <ArrowRight width={16} height={16} />
              </Link>
            </Button>
            <Button asChild size="lg" variant="outline">
              <a href="#install">Install the CLI</a>
            </Button>
          </div>
        </div>
      </div>
    </section>
  );
}

function Install() {
  return (
    <section id="install" style={{ borderTop: "1px solid #e4e4e7", padding: "96px 0" }}>
      <div style={{ margin: "0 auto", maxWidth: "672px", padding: "0 24px" }}>
        <div style={{ marginBottom: "32px", textAlign: "center" }}>
          <h2 style={{ fontSize: "30px", fontWeight: 600, letterSpacing: "-0.02em" }}>
            Install in seconds
          </h2>
          <p style={{ marginTop: "8px", color: "#71717a" }}>
            One command for macOS and Linux.
          </p>
        </div>
        <CodeBlock>{INSTALL_SNIPPET}</CodeBlock>
        <p style={{ marginTop: "16px", textAlign: "center", fontSize: "14px", color: "#71717a" }}>
          Other install methods are documented in the{" "}
          <a
            href="https://github.com/razorpay/aidocs"
            style={{ color: "#18181b" }}
            target="_blank"
            rel="noreferrer"
          >
            GitHub repo
          </a>
          .
        </p>
      </div>
    </section>
  );
}

function UseCases() {
  return (
    <section style={{ borderTop: "1px solid #e4e4e7", background: "rgba(255,255,255,0.5)", padding: "96px 0" }}>
      <div style={{ margin: "0 auto", maxWidth: "1152px", padding: "0 24px" }}>
        <div style={{ marginBottom: "40px", textAlign: "center" }}>
          <h2 style={{ fontSize: "30px", fontWeight: 600, letterSpacing: "-0.02em" }}>
            What agents publish
          </h2>
          <p style={{ marginTop: "8px", color: "#71717a" }}>
            Anything that fits in a self-contained HTML file.
          </p>
        </div>
        <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: "12px" }}>
          {useCases.map((u) => (
            <div
              key={u.title}
              style={{ display: "flex", alignItems: "center", gap: "12px", borderRadius: "12px", border: "1px solid #e4e4e7", background: "#fff", padding: "16px" }}
            >
              <div style={{ display: "flex", height: "36px", width: "36px", alignItems: "center", justifyContent: "center", borderRadius: "6px", background: "rgba(238,240,255,1)", color: "#2028ff" }}>
                <u.icon width={16} height={16} />
              </div>
              <div style={{ fontSize: "14px", fontWeight: 500 }}>{u.title}</div>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}

function GithubIcon() {
  return (
    <svg viewBox="0 0 24 24" width={16} height={16} fill="currentColor" aria-hidden>
      <path d="M12 .5C5.65.5.5 5.65.5 12c0 5.08 3.29 9.39 7.86 10.91.58.1.79-.25.79-.56v-2.18c-3.2.7-3.87-1.36-3.87-1.36-.53-1.34-1.29-1.7-1.29-1.7-1.05-.72.08-.7.08-.7 1.16.08 1.77 1.19 1.77 1.19 1.03 1.77 2.7 1.26 3.36.96.1-.75.4-1.26.73-1.55-2.55-.29-5.24-1.28-5.24-5.69 0-1.26.45-2.29 1.19-3.1-.12-.29-.51-1.46.11-3.05 0 0 .97-.31 3.18 1.18.92-.26 1.91-.39 2.89-.39.98 0 1.97.13 2.89.39 2.21-1.49 3.18-1.18 3.18-1.18.62 1.59.23 2.76.11 3.05.74.81 1.19 1.84 1.19 3.1 0 4.42-2.69 5.4-5.25 5.68.41.36.78 1.07.78 2.15v3.19c0 .31.21.67.8.56C20.22 21.39 23.5 17.08 23.5 12 23.5 5.65 18.35.5 12 .5z" />
    </svg>
  );
}

function Footer() {
  return (
    <footer style={{ borderTop: "1px solid #e4e4e7", padding: "40px 0" }}>
      <div style={{ margin: "0 auto", display: "flex", maxWidth: "1152px", flexDirection: "column", alignItems: "center", justifyContent: "space-between", gap: "12px", padding: "0 24px", fontSize: "14px", color: "#71717a" }}>
        <div style={{ display: "flex", alignItems: "center", gap: "8px" }}>
          <img src="/favicon.svg" alt="" style={{ height: "20px", width: "20px" }} />
          aidocs
        </div>
        <div style={{ display: "flex", alignItems: "center", gap: "16px" }}>
          <a href="/openapi.json">OpenAPI</a>
          <a href="/api-docs">API docs</a>
          <a href="https://github.com/razorpay/aidocs" target="_blank" rel="noreferrer">
            GitHub
          </a>
        </div>
      </div>
    </footer>
  );
}
