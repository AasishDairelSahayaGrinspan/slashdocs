import { useTitle } from "@/lib/use-title";
import { useState } from "react";
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
import { publicURL } from "@/lib/config";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { CodeBlock } from "@/components/ui/misc";
import { cn } from "@/lib/cn";
import {
  onboardingProgress,
  useOnboarding,
  type OnboardingState,
} from "@/lib/onboarding";

type StepKey = keyof OnboardingState["steps"];

export function StartPage() {
  useTitle("Get started");
  const { state, setStep, skip } = useOnboarding();
  const loginCmd = `slashdocs auth login ${publicURL()}`;
  const { done, total } = onboardingProgress(state);
  const nav = useNavigate();

  const skipTutorial = () => {
    skip();
    nav("/app/documents");
  };

  const allRequiredDone = state.steps.cli_installed && state.steps.cli_authed;

  return (
    <div className="mx-auto max-w-3xl px-4 py-6 sm:px-6 sm:py-10">
      <div className="mb-8 flex items-start justify-between gap-4">
        <div>
          <div className="mb-2 inline-flex items-center gap-2 rounded-full border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1 text-xs font-medium text-[var(--color-fg-muted)]">
            <Rocket className="h-3 w-3 text-[var(--color-accent)]" /> Setup guide
          </div>
          <h1 className="text-3xl font-semibold tracking-tight">
            Get your agent publishing to slashdocs
          </h1>
          <p className="mt-2 max-w-xl text-sm text-[var(--color-fg-muted)]">
            slashdocs is meant to be driven by your AI agent — Claude Code, Codex,
            Cursor, Pi, OpenCode, Aider, or anything else. These steps wire it up. About
            2 minutes.
          </p>
        </div>
        <button
          onClick={skipTutorial}
          className="flex shrink-0 items-center gap-1 rounded-md px-2 py-1 text-xs text-[var(--color-fg-muted)] hover:bg-[var(--color-surface-muted)] hover:text-[var(--color-fg)]"
        >
          Skip tutorial <X className="h-3 w-3" />
        </button>
      </div>

      {allRequiredDone && state.status !== "skipped" && (
        <Card className="mb-6 border-[var(--color-accent)]/30 bg-[var(--color-accent-muted)]/40">
          <CardContent className="flex items-center gap-3 pt-5">
            <PartyPopper className="h-5 w-5 text-[var(--color-accent)]" />
            <div className="flex-1">
              <div className="font-medium">You're all set.</div>
              <div className="text-xs text-[var(--color-fg-muted)]">
                Ask your agent to publish something to slashdocs. It'll show up here.
              </div>
            </div>
            <Button asChild size="sm">
              <Link to="/app/documents">
                Open documents <ArrowRight className="h-4 w-4" />
              </Link>
            </Button>
          </CardContent>
        </Card>
      )}

      <div className="mb-3 flex items-center gap-2 text-xs text-[var(--color-fg-muted)]">
        <div className="h-1 flex-1 overflow-hidden rounded-full bg-[var(--color-surface-muted)]">
          <div
            className="relative h-full overflow-hidden bg-[var(--color-accent)] transition-all"
            style={{ width: `${(done / total) * 100}%` }}
          >
            <div
              aria-hidden
              className="absolute inset-0 animate-[pill-shimmer_2s_ease-in-out_infinite] bg-gradient-to-r from-transparent via-white/25 to-transparent"
            />
          </div>
        </div>
        <span className="font-mono">
          {done}/{total} complete
        </span>
      </div>

      <ol className="space-y-3">
        <Step
          n={1}
          title="Install the CLI and slashdocs skill"
          stepKey="cli_installed"
          state={state}
          setStep={setStep}
        >
          <p className="mb-3 text-sm text-[var(--color-fg-muted)]">
            One-time, on the machine where your agent runs. This installs the slashdocs
            CLI plus the bundled agent skill, so your agent knows when to publish docs
            to slashdocs.
          </p>
          <CodeBlock>{`curl -fsSL ${publicURL()}/install.sh | sh`}</CodeBlock>
          <p className="mt-3 text-xs text-[var(--color-fg-muted)]">
            Works on macOS and Linux. The skill is fetched from this slashdocs server
            and installed into the standard agent skills directory and Claude's skills
            directory — no GitHub auth or npx step required.
          </p>
        </Step>

        <Step
          n={2}
          title="Sign in once from your terminal"
          stepKey="cli_authed"
          state={state}
          setStep={setStep}
        >
          <p className="mb-3 text-sm text-[var(--color-fg-muted)]">
            One-time. After this, your agent can publish documents on your behalf — no
            keys or tokens to copy around.
          </p>
          <CodeBlock>{loginCmd}</CodeBlock>
        </Step>

        <Step
          n={3}
          title="Try it end-to-end"
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
  // Find the first incomplete step among required ones to default-expand.
  const requiredOrder: StepKey[] = ["cli_installed", "cli_authed", "first_doc_seen"];
  const firstIncomplete = requiredOrder.find((k) => !state.steps[k]);
  const [forced, setForced] = useState<boolean | null>(null);
  const expanded = forced ?? (done ? false : firstIncomplete === stepKey);
  return (
    <li>
      <Card
        className={cn(
          "transition-colors",
          done && "bg-[var(--color-surface-muted)]/50",
        )}
      >
        <button
          type="button"
          onClick={() => setForced((v) => (v === null ? !expanded : !v))}
          className="flex w-full items-center gap-3 px-5 py-4 text-left"
        >
          <span
            className={cn(
              "flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-xs font-semibold",
              done
                ? "bg-[var(--color-success)] text-white"
                : "border-2 border-[var(--color-border-strong)] text-[var(--color-fg-muted)]",
            )}
          >
            {done ? <Check className="h-3.5 w-3.5" /> : n}
          </span>
          <div className="flex-1">
            <div
              className={cn(
                "font-medium",
                done && "text-[var(--color-fg-muted)] line-through decoration-1",
              )}
            >
              {title}
              {optional && (
                <span className="ml-2 rounded-full bg-[var(--color-surface-muted)] px-1.5 py-px text-[10px] font-medium text-[var(--color-fg-muted)]">
                  optional
                </span>
              )}
            </div>
          </div>
          {!done && (
            <span className="text-xs text-[var(--color-fg-muted)]">
              {expanded ? "Collapse" : "Show"}
            </span>
          )}
        </button>
        {expanded && (
          <div className="border-t border-[var(--color-border)] px-5 pb-5 pt-4">
            {children}
            {!done && (
              <div className="mt-4 flex justify-end">
                <Button size="sm" onClick={() => setStep(stepKey, true)}>
                  <Check className="h-3.5 w-3.5" /> Mark done
                </Button>
              </div>
            )}
            {done && (
              <div className="mt-3 flex justify-end">
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => setStep(stepKey, false)}
                >
                  <Circle className="h-3.5 w-3.5" /> Mark incomplete
                </Button>
              </div>
            )}
          </div>
        )}
      </Card>
    </li>
  );
}

const TRY_IT_PROMPT = `Write an explainer, "How GPS knows where you are", for technical readers who are not GPS experts. Cover:
- how a receiver finds its position from the distances to four satellites
- why the satellite clocks must agree to a few nanoseconds
- why relativity makes those clocks drift, and how the system corrects it

Make it visual. Use as many diagrams and illustrations as the story needs. For example:
- circles that grow from three satellites and meet at one point
- a slider that turns a clock error in nanoseconds into a position error in meters
- two clocks, one on the ground and one in orbit, that drift apart by 38 microseconds a day
- a map pin that wanders about 10 km a day when nobody corrects the drift

These are ideas, not a checklist. Use your creative liberty and add your own touch.

Run "slashdocs guidelines" first and follow it. Then publish the explainer with the slashdocs CLI and give me the document URL.`;

function TryItOut() {
  return (
    <div>
      <p className="mb-3 text-sm text-[var(--color-fg-muted)]">
        Copy this prompt into your agent. It writes an illustrated explainer and
        publishes it to slashdocs.
      </p>
      <CodeBlock wrap>{TRY_IT_PROMPT}</CodeBlock>
      <div className="mt-4">
        <Button asChild variant="ghost" size="sm">
          <Link to="/app/documents">
            <Sparkles className="h-4 w-4" /> Open documents
            <ExternalLink className="h-3.5 w-3.5" />
          </Link>
        </Button>
      </div>
    </div>
  );
}
