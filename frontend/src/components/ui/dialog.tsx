import * as React from "react";

export const Dialog = ({ open, onOpenChange, children }: {
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  children?: React.ReactNode;
}) => {
  if (!open) return null;
  return (
    <div
      style={{ position: "fixed", inset: 0, zIndex: 50, display: "flex", alignItems: "center", justifyContent: "center" }}
      onClick={() => onOpenChange?.(false)}
    >
      <div style={{ position: "absolute", inset: 0, background: "rgba(0,0,0,0.4)", backdropFilter: "blur(4px)" }} />
      <div
        style={{ position: "relative", maxWidth: "512px", width: "100%", borderRadius: "14px", border: "1px solid #e4e4e7", background: "#fff", padding: "24px", boxShadow: "0 8px 24px rgba(0,0,0,0.12)" }}
        onClick={(e) => e.stopPropagation()}
      >
        {children}
      </div>
    </div>
  );
};

export const DialogTrigger = ({ children, asChild }: { children: React.ReactNode; asChild?: boolean }) => {
  return <>{children}</>;
};

export const DialogClose = ({ children }: { children?: React.ReactNode }) => {
  return <>{children}</>;
};

export function DialogContent({ children, ...p }: React.HTMLAttributes<HTMLDivElement>) {
  return <div {...p}>{children}</div>;
}

export function DialogHeader({ children, ...p }: React.HTMLAttributes<HTMLDivElement>) {
  return <div style={{ marginBottom: "16px" }} {...p}>{children}</div>;
}

export function DialogTitle({ children, ...p }: React.HTMLAttributes<HTMLHeadingElement>) {
  return <h2 style={{ fontSize: "18px", fontWeight: 600, letterSpacing: "-0.02em" }} {...p}>{children}</h2>;
}

export function DialogDescription({ children, ...p }: React.HTMLAttributes<HTMLParagraphElement>) {
  return <p style={{ fontSize: "14px", color: "#71717a", marginTop: "4px" }} {...p}>{children}</p>;
}

export function DialogFooter({ children, ...p }: React.HTMLAttributes<HTMLDivElement>) {
  return <div style={{ marginTop: "24px", display: "flex", justifyContent: "flex-end", gap: "8px" }} {...p}>{children}</div>;
}
