import { useTitle } from "@/lib/use-title";
import { Link, Navigate, useSearchParams } from "react-router-dom";
import { ArrowLeft, MessageSquareText, Share2, Sparkles } from "lucide-react";
import { Button } from "@/components/ui/button";
import { api } from "@/api";
import { setFrontendSignedOut, useFrontendSignedOut } from "@/lib/auth-state";
import { useMe } from "@/lib/hooks";

const HIGHLIGHTS = [
  { icon: Sparkles, title: "Built for AI output", body: "Your agents publish finished documents straight to aidocs." },
  { icon: MessageSquareText, title: "Comment in context", body: "Highlight a passage and leave feedback, just like Google Docs." },
  { icon: Share2, title: "Share and iterate", body: "Versioned, shareable, and ready for the next revision." },
];

export function LoginPage() {
  useTitle("Sign in");
  const [params] = useSearchParams();
  const next = params.get("next") || "/app/documents";
  const me = useMe();
  const frontendSignedOut = useFrontendSignedOut();
  const url = api.loginURL(next);
  if (me.data && !me.unauth && !frontendSignedOut) {
    return <Navigate to={next} replace />;
  }
  return (
    <div style={{ position: "relative", display: "flex", minHeight: "100%", flexDirection: "column", alignItems: "center", justifyContent: "center", overflow: "hidden", background: "#fafafa", padding: "48px 24px" }}>
      <div
        aria-hidden
        style={{ pointerEvents: "none", position: "absolute", left: "50%", top: "33%", height: "28rem", width: "28rem", transform: "translate(-50%, -50%)", borderRadius: "9999px", background: "#2028ff", opacity: 0.07, filter: "blur(64px)" }}
      />
      <Link
        to="/"
        style={{ position: "absolute", left: "24px", top: "24px", display: "flex", alignItems: "center", gap: "4px", fontSize: "14px", color: "#71717a" }}
      >
        <ArrowLeft width={16} height={16} /> Home
      </Link>
      <div style={{ position: "relative", width: "100%", maxWidth: "384px" }}>
        <div style={{ borderRadius: "16px", border: "1px solid #e4e4e7", background: "#fff", padding: "32px", boxShadow: "0 1px 3px rgba(0,0,0,0.04)" }}>
          <div style={{ marginBottom: "28px", display: "flex", flexDirection: "column", alignItems: "center", textAlign: "center" }}>
            <img src="/favicon.svg" alt="" style={{ marginBottom: "16px", height: "56px", width: "56px" }} />
            <h1 style={{ fontSize: "24px", fontWeight: 600, letterSpacing: "-0.02em" }}>Sign in to aidocs</h1>
            <p style={{ marginTop: "8px", fontSize: "14px", color: "#71717a" }}>
              The review layer for documents your agents write.
            </p>
          </div>
          <Button asChild size="lg" isFullWidth>
            <a href={url} onClick={() => setFrontendSignedOut(false)}>
              <GoogleIcon /> Continue with Google
            </a>
          </Button>
        </div>
        <ul style={{ marginTop: "24px", listStyle: "none", padding: 0, display: "flex", flexDirection: "column", gap: "12px" }}>
          {HIGHLIGHTS.map((h) => (
            <li key={h.title} style={{ display: "flex", alignItems: "flex-start", gap: "12px" }}>
              <span style={{ marginTop: "2px", display: "flex", height: "28px", width: "28px", flexShrink: 0, alignItems: "center", justifyContent: "center", borderRadius: "6px", background: "rgba(238,240,255,1)", color: "#2028ff" }}>
                <h.icon width={14} height={14} />
              </span>
              <div>
                <div style={{ fontSize: "14px", fontWeight: 500 }}>{h.title}</div>
                <div style={{ fontSize: "12px", color: "#71717a" }}>{h.body}</div>
              </div>
            </li>
          ))}
        </ul>
      </div>
    </div>
  );
}

function GoogleIcon() {
  return (
    <svg viewBox="0 0 24 24" width={16} height={16} aria-hidden>
      <path
        fill="#fff"
        d="M21.35 11.1H12v2.9h5.35c-.23 1.45-1.7 4.25-5.35 4.25-3.22 0-5.85-2.67-5.85-5.95s2.63-5.95 5.85-5.95c1.83 0 3.06.78 3.76 1.45l2.56-2.47C16.66 3.78 14.55 2.8 12 2.8 6.92 2.8 2.8 6.92 2.8 12s4.12 9.2 9.2 9.2c5.31 0 8.83-3.73 8.83-8.98 0-.6-.06-1.06-.15-1.51z"
      />
    </svg>
  );
}
