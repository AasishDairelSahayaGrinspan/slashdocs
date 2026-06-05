import { renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import {
  autoMarkFirstDoc,
  onboardingProgress,
  shouldShowPill,
  useOnboarding,
  type OnboardingState,
} from "./onboarding";

describe("onboarding", () => {
  it("computes progress and visibility", () => {
    const state: OnboardingState = {
      status: "active",
      steps: {
        cli_installed: true,
        cli_authed: false,
        agent_configured: true,
        first_doc_seen: false,
      },
    };
    expect(onboardingProgress(state)).toEqual({ done: 2, total: 3 });
    expect(shouldShowPill(state)).toBe(true);
    expect(shouldShowPill({ ...state, status: "complete" })).toBe(false);
  });

  it("auto-completes required steps, skips, resets, and marks first doc", async () => {
    const { result } = renderHook(() => useOnboarding());

    result.current.setStep("cli_installed", true);
    result.current.setStep("cli_authed", true);
    result.current.setStep("agent_configured", true);
    await waitFor(() => expect(result.current.state.status).toBe("complete"));

    result.current.skip();
    await waitFor(() => expect(result.current.state.status).toBe("skipped"));

    result.current.reset();
    await waitFor(() => expect(result.current.state.status).toBe("active"));

    autoMarkFirstDoc();
    await waitFor(() => expect(result.current.state.steps.first_doc_seen).toBe(true));
  });
});
