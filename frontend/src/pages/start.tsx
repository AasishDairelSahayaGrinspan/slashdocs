import { useTitle } from "@/lib/use-title";
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link, useNavigate } from "react-router-dom";
import {
  ArrowRight,
  Check,
  Circle,
  ExternalLink,
  PartyPopper,
  Rocket,
  Sparkles,
  X,
} from "lucide-react";
import { api } from "@/api";
import { publicURL } from "@/lib/config";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { CodeBlock } from "@/components/ui/misc";
import {
  onboardingProgress,
  useOnboarding,
  type OnboardingState,
} from "@/lib/onboarding";

type StepKey = keyof OnboardingState["steps"];

const SKILL_REPO = "github:razorpay/aidocs";

const AGENT_TARGETS = [
  { id: "claude-code", label: "Claude Code" },
  { id: "codex", label: "Codex" },
  { id: "cursor", label: "Cursor" },
  { id: "pi", label: "Pi" },
  { id: "opencode", label: "OpenCode" },
  { id: "aider", label: "Aider" },
];

export function StartPage() {
  useTitle("Get started");
  const { state, setStep, skip } = useOnboarding();
  const config = useQuery({
    queryKey: ["config"],
    queryFn: () => api.getConfig(),
    staleTime: Infinity,
  });
  const loginCmd =
    config.data?.deployment === "org"
      ? `aidocs auth login ${publicURL()}`
      : "aidocs auth login";
  const { done, total } = onboardingProgress(state);
  const nav = useNavigate();

  const skipTutorial = () => {
    skip();
    nav("/app/documents");
  };

  const allRequiredDone =
    state.steps.cli_installed && state.steps.cli_authed && state.steps.agent_configured;

  return (
    <div style={{ maxWidth: "768px", margin: "0 auto", paddingLeft: "24px", paddingRight: "24px", paddingTop: "40px", paddingBottom: "40px" }}>
      <div style={{ marginBottom: "32px", display: "flex", alignItems: "flex-start", justifyContent: "space-between", gap: "16px" }}>
        <div>
          <div style={{
            marginBottom: "8px",
            display: "inline-flex",
            alignItems: "center",
            gap: "8px",
            borderRadius: "9999px",
            border: "1px solid #e4e4e7",
            background: "#fff",
            padding: "4px 12px",
            fontSize: "12px",
            fontWeight: 500,
            color: "#71717a",
          }}>
            <Rocket style={{ height: "12px", width: "12px", color: "#2028ff" }} /> Setup guide
          </div>
          <h1 style={{ fontSize: "30px", fontWeight: 600, letterSpacing: "-0.02em" }}>
            Get your agent publishing to aidocs
          </h1>
          <p style={{ marginTop: "8px", maxWidth: "576px", fontSize: "14px", color: "#71717a" }}>
            aidocs is meant to be driven by your AI agent — Claude Code, Codex, Cursor,
            Pi, OpenCode, Aider, or anything else. These three steps wire it up. About 2
            minutes.
          </p>
        </div>
        <button
          onClick={skipTutorial}
          style={{
            display: "flex",
            flexShrink: 0,
            alignItems: "center",
            gap: "4px",
            borderRadius: "6px",
            padding: "4px 8px",
            fontSize: "12px",
            color: "#71717a",
            background: "transparent",
            border: "none",
            cursor: "pointer",
          }}
        >
          Skip tutorial <X style={{ height: "12px", width: "12px" }} />
        </button>
      </div>

      {allRequiredDone && state.status !== "skipped" && (
        <Card style={{ marginBottom: "24px", borderColor: "rgba(32,40,255,0.3)", background: "rgba(238,240,255,0.4)" }}>
          <CardContent style={{ display: "flex", alignItems: "center", gap: "12px", paddingTop: "20px" }}>
            <PartyPopper style={{ height: "20px", width: "20px", color: "#2028ff" }} />
            <div style={{ flex: 1 }}>
              <div style={{ fontWeight: 500 }}>You're all set.</div>
              <div style={{ fontSize: "12px", color: "#71717a" }}>
                Ask your agent to publish something to aidocs. It'll show up here.
              </div>
            </div>
            <Button asChild size="sm">
              <Link to="/app/documents">
                Open documents <ArrowRight style={{ height: "16px", width: "16px" }} />
              </Link>
            </Button>
          </CardContent>
        </Card>
      )}

      <div style={{ marginBottom: "12px", display: "flex", alignItems: "center", gap: "8px", fontSize: "12px", color: "#71717a" }}>
        <div style={{ height: "4px", flex: 1, overflow: "hidden", borderRadius: "9999px", background: "#f4f4f5" }}>
          <div
            style={{ height: "100%", background: "#2028ff", transition: "all 0.15s", width: `${(done / total) * 100}%` }}
          />
        </div>
        <span style={{ fontFamily: "ui-monospace, monospace" }}>
          {done}/{total} complete
        </span>
      </div>

      <ol style={{ display: "flex", flexDirection: "column", gap: "12px", listStyle: "none", padding: 0, margin: 0 }}>
        <Step
          n={1}
          title="Install the CLI on your machine"
          stepKey="cli_installed"
          state={state}
          setStep={setStep}
        >
          <p style={{ marginBottom: "12px", fontSize: "14px", color: "#71717a" }}>
            One-time, on the machine where your agent runs. Your agent calls this CLI
            under the hood — you won't need to use it directly.
          </p>
          <CodeBlock>{`curl -fsSL ${publicURL()}/install.sh | bash`}</CodeBlock>
          <p style={{ marginTop: "12px", fontSize: "12px", color: "#71717a" }}>
            Works on macOS and Linux.
          </p>
        </Step>

        <Step
          n={2}
          title="Sign in once from your terminal"
          stepKey="cli_authed"
          state={state}
          setStep={setStep}
        >
          <p style={{ marginBottom: "12px", fontSize: "14px", color: "#71717a" }}>
            One-time. After this, your agent can publish documents on your behalf — no
            keys or tokens to copy around.
          </p>
          <CodeBlock>{loginCmd}</CodeBlock>
        </Step>

        <Step
          n={3}
          title="Teach your agent the aidocs skill"
          stepKey="agent_configured"
          state={state}
          setStep={setStep}
        >
          <AgentConfigure />
        </Step>

        <Step
          n={4}
          title="Try it end-to-end (optional)"
          stepKey="first_doc_seen"
          state={state}
          setStep={setStep}
          optional
        >
          <TryItOut />
        </Step>
      </ol>
    </div>
  );
}

function Step({
  n,
  title,
  stepKey,
  state,
  setStep,
  optional,
  children,
}: {
  n: number;
  title: string;
  stepKey: StepKey;
  state: OnboardingState;
  setStep: (k: StepKey, v: boolean) => void;
  optional?: boolean;
  children: React.ReactNode;
}) {
  const done = state.steps[stepKey];
  const requiredOrder: StepKey[] = [
    "cli_installed",
    "cli_authed",
    "agent_configured",
    "first_doc_seen",
  ];
  const firstIncomplete = requiredOrder.find((k) => !state.steps[k]);
  const [forced, setForced] = useState<boolean | null>(null);
  const expanded = forced ?? (done ? false : firstIncomplete === stepKey);
  return (
    <li>
      <Card style={{ transition: "background-color 0.15s", ...(done ? { background: "rgba(244,244,245,0.5)" } : {}) }}>
        <button
          type="button"
          onClick={() => setForced((v) => (v === null ? !expanded : !v))}
          style={{
            display: "flex",
            width: "100%",
            alignItems: "center",
            gap: "12px",
            padding: "16px 20px",
            textAlign: "left",
            background: "transparent",
            border: "none",
            cursor: "pointer",
          }}
        >
          <span
            style={{
              display: "flex",
              height: "24px",
              width: "24px",
              flexShrink: 0,
              alignItems: "center",
              justifyContent: "center",
              borderRadius: "9999px",
              fontSize: "12px",
              fontWeight: 600,
              ...(done
                ? { background: "#16a34a", color: "#fff" }
                : { border: "2px solid #d4d4d8", color: "#71717a" }),
            }}
          >
            {done ? <Check style={{ height: "14px", width: "14px" }} /> : n}
          </span>
          <div style={{ flex: 1 }}>
            <div
              style={{
                fontWeight: 500,
                ...(done ? { color: "#71717a", textDecoration: "line-through", textDecorationThickness: "1px" } : {}),
              }}
            >
              {title}
              {optional && (
                <span style={{
                  marginLeft: "8px",
                  borderRadius: "9999px",
                  background: "#f4f4f5",
                  padding: "1px 6px",
                  fontSize: "10px",
                  fontWeight: 500,
                  color: "#71717a",
                }}>
                  optional
                </span>
              )}
            </div>
          </div>
          {!done && (
            <span style={{ fontSize: "12px", color: "#71717a" }}>
              {expanded ? "Collapse" : "Show"}
            </span>
          )}
        </button>
        {expanded && (
          <div style={{ borderTop: "1px solid #e4e4e7", padding: "16px 20px 20px" }}>
            {children}
            {!done && (
              <div style={{ marginTop: "16px", display: "flex", justifyContent: "flex-end" }}>
                <Button size="sm" onClick={() => setStep(stepKey, true)}>
                  <Check style={{ height: "14px", width: "14px" }} /> Mark done
                </Button>
              </div>
            )}
            {done && (
              <div style={{ marginTop: "12px", display: "flex", justifyContent: "flex-end" }}>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => setStep(stepKey, false)}
                >
                  <Circle style={{ height: "14px", width: "14px" }} /> Mark incomplete
                </Button>
              </div>
            )}
          </div>
        )}
      </Card>
    </li>
  );
}

function AgentConfigure() {
  const cmd = `npx skills add ${SKILL_REPO} -s aidocs --all`;
  return (
    <div>
      <p style={{ marginBottom: "12px", fontSize: "14px", color: "#71717a" }}>
        Run this once to install the aidocs skill into the agents you use. It tells
        them: "when the user asks for a doc / report / RCA / spec, publish it to
        aidocs". <code>--all</code> installs across every coding agent it finds on your
        machine.
      </p>
      <CodeBlock>{cmd}</CodeBlock>
      <div style={{ marginTop: "16px" }}>
        <div style={{ marginBottom: "8px", fontSize: "12px", fontWeight: 500, color: "#71717a" }}>
          Works with
        </div>
        <div style={{ display: "flex", flexWrap: "wrap", gap: "6px" }}>
          {AGENT_TARGETS.map((t) => (
            <span
              key={t.id}
              style={{
                borderRadius: "9999px",
                border: "1px solid #e4e4e7",
                padding: "4px 10px",
                fontSize: "12px",
                color: "#71717a",
              }}
            >
              {t.label}
            </span>
          ))}
        </div>
      </div>
      <p style={{ marginTop: "16px", fontSize: "12px", color: "#71717a" }}>
        Skill source:{" "}
        <a
          href="https://github.com/razorpay/aidocs/tree/main/skills/aidocs"
          target="_blank"
          rel="noreferrer"
          style={{ color: "#18181b" }}
        >
          skills/aidocs/SKILL.md
        </a>
        . Read it, fork it, change the trigger wording — it's just a Markdown file.
      </p>
    </div>
  );
}

function TryItOut() {
  const url = publicURL();
  const prompt = `Download the aidocs sample report and publish it as a reviewable document:

curl -sSL ${url}/onboarding/sample.html -o sample.html

Then use the aidocs CLI to publish sample.html to aidocs. Give me the document URL when you're done.`;
  return (
    <div>
      <p style={{ marginBottom: "12px", fontSize: "14px", color: "#71717a" }}>
        Want to see the loop work without writing anything? Copy this prompt into your
        agent — it'll fetch a sample report and publish it to aidocs.
      </p>
      <CodeBlock>{prompt}</CodeBlock>
      <div style={{ marginTop: "16px" }}>
        <Button asChild variant="ghost" size="sm">
          <Link to="/app/documents">
            <Sparkles style={{ height: "16px", width: "16px" }} /> Open documents
            <ExternalLink style={{ height: "14px", width: "14px" }} />
          </Link>
        </Button>
      </div>
    </div>
  );
}
