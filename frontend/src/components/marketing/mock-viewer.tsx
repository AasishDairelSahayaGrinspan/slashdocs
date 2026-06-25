import { motion, AnimatePresence } from "framer-motion";
import { CheckCircle2 } from "lucide-react";
import { cn } from "@/lib/cn";

type CommentState = "none" | "selecting" | "open" | "resolved";

export function MockDocViewer({
  version,
  commentState,
  highlight,
  className,
}: {
  version: 1 | 2;
  commentState: CommentState;
  highlight?: boolean;
  className?: string;
}) {
  return (
    <div
      className={cn("mock-doc-viewer", className)}
      style={{
        position: "relative",
        overflow: "hidden",
        borderRadius: "12px",
        border: "1px solid #e4e4e7",
        background: "#fff",
        boxShadow: "0 8px 24px rgba(0,0,0,0.12)",
      }}
    >
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", borderBottom: "1px solid #e4e4e7", padding: "10px 16px" }}>
        <div style={{ display: "flex", alignItems: "center", gap: "8px", fontSize: "14px" }}>
          <span style={{ fontWeight: 600 }}>Q3 incident review</span>
          <motion.span
            key={version}
            initial={{ scale: 0.6, opacity: 0 }}
            animate={{ scale: 1, opacity: 1 }}
            transition={{ type: "spring", stiffness: 400, damping: 18 }}
            style={{
              borderRadius: "9999px",
              background: "rgba(238,240,255,1)",
              padding: "2px 8px",
              fontSize: "10px",
              fontWeight: 500,
              color: "#2028ff",
            }}
          >
            v{version}
          </motion.span>
        </div>
        <div style={{ display: "flex", gap: "6px" }}>
          <span style={{ height: "8px", width: "8px", borderRadius: "9999px", background: "#d4d4d8" }} />
          <span style={{ height: "8px", width: "8px", borderRadius: "9999px", background: "#d4d4d8" }} />
          <span style={{ height: "8px", width: "8px", borderRadius: "9999px", background: "#d4d4d8" }} />
        </div>
      </div>
      <div style={{ display: "flex" }}>
        <div style={{ position: "relative", flex: 1, padding: "20px", fontSize: "11.5px", lineHeight: 1.6, color: "#18181b" }}>
          <RichDoc highlight={!!highlight} />
          <AnimatePresence>
            {commentState === "selecting" && (
              <motion.div
                initial={{ opacity: 0, y: -4 }}
                animate={{ opacity: 1, y: 0 }}
                exit={{ opacity: 0 }}
                style={{
                  position: "absolute",
                  left: "22%",
                  top: "58%",
                  display: "flex",
                  alignItems: "center",
                  gap: "4px",
                  borderRadius: "6px",
                  background: "#18181b",
                  padding: "4px 8px",
                  fontSize: "10px",
                  fontWeight: 500,
                  color: "#fff",
                  boxShadow: "0 4px 12px rgba(0,0,0,0.15)",
                  pointerEvents: "none",
                }}
              >
                <span style={{ display: "inline-block", height: "6px", width: "6px", borderRadius: "9999px", background: "#2028ff" }} />
                Comment
              </motion.div>
            )}
          </AnimatePresence>
        </div>
        <div style={{ width: "180px", flexShrink: 0, borderLeft: "1px solid #e4e4e7", background: "rgba(244,244,245,0.4)", padding: "10px" }}>
          <div style={{ marginBottom: "8px", fontSize: "10px", fontWeight: 500, textTransform: "uppercase", letterSpacing: "0.05em", color: "#71717a" }}>
            Comments
          </div>
          <AnimatePresence>
            {(commentState === "open" || commentState === "resolved") && (
              <motion.div
                initial={{ opacity: 0, y: 8 }}
                animate={{ opacity: 1, y: 0 }}
                exit={{ opacity: 0 }}
                style={{
                  borderRadius: "6px",
                  border: "1px solid #e4e4e7",
                  background: "#fff",
                  padding: "8px",
                  fontSize: "10px",
                  opacity: commentState === "resolved" ? 0.6 : 1,
                }}
              >
                <div style={{ marginBottom: "4px", display: "flex", alignItems: "center", justifyContent: "space-between" }}>
                  <span style={{ fontWeight: 600 }}>alice</span>
                  {commentState === "resolved" ? (
                    <CheckCircle2 width={12} height={12} style={{ color: "#16a34a" }} />
                  ) : (
                    <span style={{ borderRadius: "9999px", background: "rgba(238,240,255,1)", padding: "1px 6px", fontSize: "8px", fontWeight: 500, color: "#2028ff" }}>
                      open
                    </span>
                  )}
                </div>
                <div style={{ marginBottom: "4px", borderLeft: "2px solid #d4d4d8", paddingLeft: "4px", color: "#71717a" }}>
                  …impact window…
                </div>
                <div style={{ color: "#18181b" }}>
                  Clarify the impact window.
                </div>
              </motion.div>
            )}
          </AnimatePresence>
        </div>
      </div>
    </div>
  );
}

function RichDoc({ highlight }: { highlight: boolean }) {
  return (
    <>
      <h3 style={{ marginBottom: "4px", fontSize: "15px", fontWeight: 600, letterSpacing: "-0.02em" }}>
        Q3 payments incident — postmortem
      </h3>
      <div style={{ marginBottom: "12px", display: "flex", alignItems: "center", gap: "8px", fontSize: "10px", color: "#71717a" }}>
        <span>Sept 14, 2025</span>
        <span>·</span>
        <span style={{ borderRadius: "4px", background: "rgba(239,68,68,0.15)", padding: "0 6px", color: "#ef4444" }}>
          Sev-1
        </span>
        <span>·</span>
        <span>owner: payments-platform</span>
      </div>
      <p style={{ marginBottom: "12px" }}>
        On Sept 14, the payments API returned <code style={{ borderRadius: "4px", background: "#f4f4f5", padding: "0 4px", fontFamily: "monospace", fontSize: "10px" }}>5xx</code> errors for{" "}
        <span
          style={{
            borderRadius: "4px",
            padding: "0 2px",
            transition: "background-color 0.3s",
            background: highlight ? "rgba(250,204,21,0.8)" : "transparent",
          }}
        >
          roughly 12 minutes during the impact window
        </span>
        , affecting 3.2% of merchants.
      </p>

      <h4 style={{ marginBottom: "4px", marginTop: "12px", fontSize: "12px", fontWeight: 600 }}>Impact</h4>
      <table style={{ marginBottom: "12px", width: "100%", fontSize: "10px" }}>
        <thead style={{ textAlign: "left", color: "#71717a" }}>
          <tr>
            <th style={{ borderBottom: "1px solid #e4e4e7", paddingBottom: "4px", fontWeight: 500 }}>
              Metric
            </th>
            <th style={{ borderBottom: "1px solid #e4e4e7", paddingBottom: "4px", fontWeight: 500 }}>
              Value
            </th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <td style={{ padding: "4px 0" }}>Failed requests</td>
            <td style={{ padding: "4px 0", fontFamily: "monospace" }}>14,203</td>
          </tr>
          <tr>
            <td style={{ padding: "4px 0" }}>Merchants affected</td>
            <td style={{ padding: "4px 0", fontFamily: "monospace" }}>3.2%</td>
          </tr>
          <tr>
            <td style={{ padding: "4px 0" }}>Revenue at risk</td>
            <td style={{ padding: "4px 0", fontFamily: "monospace" }}>$48,200</td>
          </tr>
        </tbody>
      </table>

      <h4 style={{ marginBottom: "4px", marginTop: "12px", fontSize: "12px", fontWeight: 600 }}>Error rate</h4>
      <div style={{ marginBottom: "12px", display: "flex", height: "56px", alignItems: "flex-end", gap: "4px", borderRadius: "6px", background: "rgba(244,244,245,0.6)", padding: "8px" }}>
        {[12, 18, 14, 22, 88, 95, 76, 30, 14, 11, 9, 10].map((h, i) => (
          <div
            key={i}
            style={{
              height: `${h}%`,
              flex: 1,
              borderRadius: "2px",
              background: h > 50 ? "rgba(248,113,113,0.8)" : "rgba(32,40,255,0.7)",
            }}
          />
        ))}
      </div>

      <h4 style={{ marginBottom: "4px", fontSize: "12px", fontWeight: 600 }}>Root cause</h4>
      <pre style={{ overflow: "hidden", borderRadius: "6px", background: "#f4f4f5", padding: "8px", fontFamily: "monospace", fontSize: "9.5px", lineHeight: 1.6, color: "#18181b" }}>
        {`if conn.IdleTimeout < ctx.Deadline() {
  // pool returned a stale conn → 5xx
  return ErrUpstream
}`}
      </pre>
    </>
  );
}
