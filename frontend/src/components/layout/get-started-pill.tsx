import { Link } from "react-router-dom";
import { Rocket } from "lucide-react";
import { onboardingProgress, shouldShowPill, useOnboarding } from "@/lib/onboarding";

export function GetStartedPill() {
  const { state } = useOnboarding();
  if (!shouldShowPill(state)) return null;
  const { done, total } = onboardingProgress(state);
  return (
    <Link
      to="/app/start"
      style={{
        display: "inline-flex",
        alignItems: "center",
        gap: "8px",
        borderRadius: "9999px",
        border: "1px solid rgba(32,40,255,0.4)",
        background: "rgba(238,240,255,1)",
        padding: "4px 12px",
        fontSize: "12px",
        fontWeight: 500,
        color: "#2028ff",
      }}
      aria-label={`Get started: ${done} of ${total} complete`}
    >
      <Rocket width={14} height={14} />
      Get started
      <span style={{ borderRadius: "9999px", background: "rgba(32,40,255,0.15)", padding: "0 6px", fontFamily: "monospace", fontSize: "10px" }}>
        {done}/{total}
      </span>
    </Link>
  );
}
