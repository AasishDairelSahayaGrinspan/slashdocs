import * as React from "react";
import { Button } from "./button";

export function Skeleton({ className, ...p }: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      style={{
        borderRadius: "6px",
        background: "#f4f4f5",
        animation: "pulse 1.5s ease-in-out infinite",
      }}
      {...p}
    />
  );
}

export function CodeBlock({
  children,
  copy = true,
  className,
}: {
  children: string;
  copy?: boolean;
  className?: string;
}) {
  const [copied, setCopied] = React.useState(false);
  const onCopy = async () => {
    try {
      await navigator.clipboard.writeText(children);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
    }
  };
  return (
    <div
      style={{
        position: "relative",
        borderRadius: "10px",
        border: "1px solid #e4e4e7",
        background: "#f4f4f5",
        fontFamily: "ui-monospace, SFMono-Regular, Menlo, Consolas, monospace",
        fontSize: "12px",
      }}
    >
      <pre style={{ overflowX: "auto", padding: "12px 40px 12px 12px", lineHeight: 1.6, color: "#18181b", margin: 0 }}>
        {children}
      </pre>
      {copy && (
        <button
          onClick={onCopy}
          style={{
            position: "absolute",
            right: "8px",
            top: "8px",
            borderRadius: "6px",
            padding: "6px",
            border: "none",
            background: "transparent",
            cursor: "pointer",
            color: "#71717a",
          }}
          aria-label={copied ? "Copied" : "Copy"}
        >
          {copied ? "✓" : "⎘"}
        </button>
      )}
    </div>
  );
}

export function EmptyState({
  icon,
  title,
  description,
  action,
  className,
}: {
  icon?: React.ReactNode;
  title: string;
  description?: React.ReactNode;
  action?: React.ReactNode;
  className?: string;
}) {
  return (
    <div
      style={{
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        justifyContent: "center",
        borderRadius: "14px",
        border: "1px dashed #e4e4e7",
        background: "#fff",
        padding: "40px",
        textAlign: "center",
      }}
    >
      {icon && (
        <div style={{ marginBottom: "12px", display: "flex", height: "40px", width: "40px", alignItems: "center", justifyContent: "center", borderRadius: "9999px", background: "#f4f4f5", color: "#71717a" }}>
          {icon}
        </div>
      )}
      <h3 style={{ fontSize: "16px", fontWeight: 600 }}>{title}</h3>
      {description && (
        <p style={{ marginTop: "4px", maxWidth: "448px", fontSize: "14px", color: "#71717a" }}>
          {description}
        </p>
      )}
      {action && <div style={{ marginTop: "16px" }}>{action}</div>}
    </div>
  );
}

export function Center({ children, className }: { children: React.ReactNode; className?: string }) {
  return (
    <div
      style={{
        display: "flex",
        height: "100%",
        width: "100%",
        alignItems: "center",
        justifyContent: "center",
        padding: "40px",
        fontSize: "14px",
        color: "#71717a",
      }}
    >
      {children}
    </div>
  );
}

export function Avatar({ name, src, size = 32 }: { name?: string; src?: string; size?: number }) {
  const initials =
    (name || "?")
      .split(/\s+/)
      .map((x) => x[0])
      .filter(Boolean)
      .join("")
      .slice(0, 2)
      .toUpperCase() || "?";
  const [failed, setFailed] = React.useState(false);
  React.useEffect(() => {
    setFailed(false);
  }, [src]);
  const showImg = src && !failed;
  return (
    <div
      style={{
        width: size,
        height: size,
        fontSize: Math.max(10, Math.floor(size * 0.38)),
        display: "flex",
        flexShrink: 0,
        alignItems: "center",
        justifyContent: "center",
        overflow: "hidden",
        borderRadius: "9999px",
        background: "rgba(238,240,255,1)",
        fontWeight: 600,
        color: "#2028ff",
      }}
    >
      {showImg ? (
        <img
          src={src}
          alt=""
          referrerPolicy="no-referrer"
          onError={() => setFailed(true)}
          style={{ height: "100%", width: "100%", objectFit: "cover" }}
        />
      ) : (
        initials
      )}
    </div>
  );
}

export { Button };
