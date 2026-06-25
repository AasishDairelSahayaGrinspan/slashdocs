import { useEffect, useRef, useState } from "react";
import { cn } from "@/lib/cn";

type Line = { kind: "cmd" | "out"; text: string };

export function Terminal({
  lines,
  active,
  speed = 18,
  className,
  height = 200,
}: {
  lines: Line[];
  active: boolean;
  speed?: number;
  className?: string;
  height?: number;
}) {
  const [shown, setShown] = useState<{ line: number; col: number }>({
    line: 0,
    col: 0,
  });
  const startedRef = useRef(false);
  const linesRef = useRef(lines);
  linesRef.current = lines;
  const speedRef = useRef(speed);
  speedRef.current = speed;
  const reduced = usePrefersReducedMotion();

  useEffect(() => {
    if (!active || startedRef.current) return;
    startedRef.current = true;
    if (reduced) {
      setShown({ line: linesRef.current.length, col: 0 });
      return;
    }
    let li = 0;
    let ci = 0;
    const tick = () => {
      const ls = linesRef.current;
      const line = ls[li];
      if (!line) return;
      if (ci < line.text.length) {
        ci++;
        setShown({ line: li, col: ci });
        const delay = line.kind === "cmd" ? speedRef.current : 4;
        setTimeout(tick, delay);
      } else {
        li++;
        ci = 0;
        setShown({ line: li, col: 0 });
        if (li < ls.length) setTimeout(tick, ls[li].kind === "out" ? 120 : 280);
      }
    };
    tick();
  }, [active, reduced]);

  return (
    <div
      className={cn("terminal-component", className)}
      style={{
        overflow: "hidden",
        borderRadius: "12px",
        border: "1px solid #e4e4e7",
        background: "#0b0d12",
        fontFamily: "ui-monospace, SFMono-Regular, Menlo, Consolas, monospace",
        fontSize: "12.5px",
        lineHeight: 1.7,
        color: "#e4e4e7",
        boxShadow: "0 8px 24px rgba(0,0,0,0.12)",
      }}
    >
      <div style={{ display: "flex", alignItems: "center", gap: "6px", borderBottom: "1px solid rgba(255,255,255,0.05)", padding: "8px 12px" }}>
        <span style={{ height: "10px", width: "10px", borderRadius: "9999px", background: "rgba(248,113,113,0.8)" }} />
        <span style={{ height: "10px", width: "10px", borderRadius: "9999px", background: "rgba(250,204,21,0.8)" }} />
        <span style={{ height: "10px", width: "10px", borderRadius: "9999px", background: "rgba(74,222,128,0.8)" }} />
        <span style={{ marginLeft: "8px", fontSize: "11px", color: "#71717a" }}>~ aidocs</span>
      </div>
      <div style={{ padding: "16px", height, overflow: "hidden" }} aria-live="polite">
        {lines.map((l, i) => {
          const isCurrent = i === shown.line;
          const reveal = i < shown.line ? l.text.length : isCurrent ? shown.col : 0;
          if (reveal === 0 && !isCurrent) return null;
          return (
            <div key={i} style={{ whiteSpace: "pre-wrap" }}>
              {l.kind === "cmd" ? (
                <span>
                  <span style={{ color: "#2028ff" }}>$ </span>
                  <span>{l.text.slice(0, reveal)}</span>
                  {isCurrent && reveal < l.text.length && (
                    <span className="caret">▋</span>
                  )}
                </span>
              ) : (
                <span style={{ color: "#a1a1aa" }}>{l.text.slice(0, reveal)}</span>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}

export function usePrefersReducedMotion() {
  const [reduced, setReduced] = useState(false);
  useEffect(() => {
    const mq = window.matchMedia("(prefers-reduced-motion: reduce)");
    setReduced(mq.matches);
    const fn = () => setReduced(mq.matches);
    mq.addEventListener("change", fn);
    return () => mq.removeEventListener("change", fn);
  }, []);
  return reduced;
}
