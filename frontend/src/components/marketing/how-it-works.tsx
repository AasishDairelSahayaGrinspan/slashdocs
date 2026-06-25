import { useEffect, useRef, useState } from "react";
import { motion } from "framer-motion";
import { Terminal, usePrefersReducedMotion } from "./terminal";
import { MockDocViewer } from "./mock-viewer";
import { MockDocsTable } from "./mock-table";

type SceneState = {
  cmdStarted: boolean;
  uiActivated: boolean;
};

function useSceneIntersection() {
  const ref = useRef<HTMLDivElement>(null);
  const [state, setState] = useState<SceneState>({
    cmdStarted: false,
    uiActivated: false,
  });
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const io = new IntersectionObserver(
      ([entry]) => {
        if (entry.intersectionRatio > 0.35) {
          setState((s) => (s.cmdStarted ? s : { cmdStarted: true, uiActivated: false }));
          window.setTimeout(
            () => setState((s) => ({ ...s, uiActivated: true })),
            1400,
          );
        }
      },
      { threshold: [0, 0.35, 0.7] },
    );
    io.observe(el);
    return () => io.disconnect();
  }, []);
  return { ref, state };
}

export function HowItWorks() {
  return (
    <section
      id="how-it-works"
      aria-label="How aidocs works"
      style={{ borderTop: "1px solid #e4e4e7", background: "#fafafa", padding: "96px 0" }}
    >
      <div style={{ margin: "0 auto 64px", maxWidth: "768px", padding: "0 24px", textAlign: "center" }}>
        <div style={{ marginBottom: "12px", display: "inline-block", borderRadius: "9999px", border: "1px solid #e4e4e7", background: "#fff", padding: "4px 12px", fontSize: "12px", fontWeight: 500, color: "#71717a" }}>
          How it works
        </div>
        <h2 style={{ fontSize: "30px", fontWeight: 600, letterSpacing: "-0.02em" }}>
          Agents push. Humans review.
        </h2>
        <p style={{ marginTop: "12px", color: "#71717a" }}>
          Three steps from a generated HTML file to a reviewed, versioned
          document.
        </p>
      </div>

      <ol style={{ position: "absolute", width: "1px", height: "1px", padding: 0, margin: -1, overflow: "hidden", clip: "rect(0,0,0,0)", border: 0 }}>
        <li>Step 1: An agent publishes a document via the CLI.</li>
        <li>Step 2: Reviewers comment on the exact text in the browser.</li>
        <li>Step 3: The agent ships a new version and resolves the thread.</li>
      </ol>

      <div style={{ display: "flex", flexDirection: "column", gap: "96px" }}>
        <SceneAgentPublishes />
        <SceneHumanReviews />
        <SceneAgentResponds />
      </div>
    </section>
  );
}

function SceneFrame({
  step,
  title,
  caption,
  children,
  inputRef,
}: {
  step: number;
  title: string;
  caption: string;
  children: React.ReactNode;
  inputRef: React.RefObject<HTMLDivElement | null>;
}) {
  return (
    <div ref={inputRef} style={{ margin: "0 auto", width: "100%", maxWidth: "1152px", padding: "0 24px" }}>
      <div style={{ marginBottom: "24px", display: "flex", alignItems: "center", gap: "12px" }}>
        <span style={{ display: "flex", height: "28px", width: "28px", alignItems: "center", justifyContent: "center", borderRadius: "9999px", background: "#2028ff", fontSize: "12px", fontWeight: 600, color: "#fff" }}>
          {step}
        </span>
        <div>
          <div style={{ fontSize: "18px", fontWeight: 600 }}>{title}</div>
          <div style={{ fontSize: "14px", color: "#71717a" }}>{caption}</div>
        </div>
      </div>
      <div style={{ display: "grid", gridTemplateColumns: "1fr", gap: "24px" }}>
        {children}
      </div>
    </div>
  );
}

function SceneAgentPublishes() {
  const { ref, state } = useSceneIntersection();
  return (
    <SceneFrame
      step={1}
      title="Your agent pushes a document"
      caption="The CLI uploads a self-contained HTML file."
      inputRef={ref}
    >
      <Terminal
        active={state.cmdStarted}
        height={190}
        lines={[
          { kind: "cmd", text: "aidocs auth login" },
          { kind: "out", text: "signed in as alice@example.com" },
          { kind: "cmd", text: "aidocs docs create report.html" },
          { kind: "out", text: "doc_id=doc_8f2a" },
          { kind: "out", text: "url=https://aidocs.dev/app/d/doc_8f2a" },
        ]}
      />
      <motion.div
        initial={{ opacity: 0, x: 12 }}
        animate={{ opacity: state.uiActivated ? 1 : 0.3, x: 0 }}
        transition={{ duration: 0.4 }}
      >
        <MockDocsTable hasRow={state.uiActivated} />
      </motion.div>
    </SceneFrame>
  );
}

function SceneHumanReviews() {
  const { ref, state } = useSceneIntersection();
  const [phase, setPhase] = useState<"none" | "selecting" | "open">("none");
  useEffect(() => {
    if (!state.uiActivated) return;
    setPhase("selecting");
    const t1 = setTimeout(() => setPhase("open"), 1100);
    return () => clearTimeout(t1);
  }, [state.uiActivated]);
  return (
    <SceneFrame
      step={2}
      title="Reviewers comment on exact text"
      caption="Selections are anchored to the rendered HTML — and visible to the agent."
      inputRef={ref}
    >
      <div style={{ position: "relative" }}>
        <MockDocViewer
          version={1}
          commentState={phase === "none" ? "none" : phase === "selecting" ? "selecting" : "open"}
          highlight={phase !== "none"}
        />
      </div>
      <Terminal
        active={state.cmdStarted && phase === "open"}
        height={150}
        lines={[
          { kind: "cmd", text: "aidocs docs comments list doc_8f2a" },
          { kind: "out", text: "cmt_a1  status=open" },
          { kind: "out", text: '  quote="…impact window…"' },
          { kind: "out", text: "  author=alice@example.com" },
        ]}
      />
    </SceneFrame>
  );
}

function SceneAgentResponds() {
  const { ref, state } = useSceneIntersection();
  const [version, setVersion] = useState<1 | 2>(1);
  const [resolved, setResolved] = useState(false);
  useEffect(() => {
    if (!state.uiActivated) return;
    const t1 = setTimeout(() => setVersion(2), 200);
    const t2 = setTimeout(() => setResolved(true), 1200);
    return () => {
      clearTimeout(t1);
      clearTimeout(t2);
    };
  }, [state.uiActivated]);
  return (
    <SceneFrame
      step={3}
      title="Agent ships a new version"
      caption="Push a revision, resolve the thread, move on."
      inputRef={ref}
    >
      <Terminal
        active={state.cmdStarted}
        height={190}
        lines={[
          { kind: "cmd", text: "aidocs docs push doc_8f2a report.html" },
          { kind: "out", text: "version=ver_2" },
          { kind: "cmd", text: "aidocs docs comments resolve doc_8f2a cmt_a1" },
          { kind: "out", text: "ok" },
        ]}
      />
      <MockDocViewer
        version={version}
        commentState={resolved ? "resolved" : "open"}
      />
    </SceneFrame>
  );
}

export { usePrefersReducedMotion };
