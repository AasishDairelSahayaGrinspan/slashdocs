import * as React from "react";

export const Sheet = ({ open, onOpenChange, children }: {
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  children?: React.ReactNode;
}) => {
  if (!open) return null;
  return (
    <div style={{ position: "fixed", inset: 0, zIndex: 50 }} onClick={() => onOpenChange?.(false)}>
      <div style={{ position: "absolute", inset: 0, background: "rgba(0,0,0,0.4)" }} />
      <div
        style={{
          position: "absolute",
          top: 0,
          right: 0,
          height: "100%",
          width: "100%",
          maxWidth: "448px",
          background: "#fff",
          borderLeft: "1px solid #e4e4e7",
          boxShadow: "0 8px 24px rgba(0,0,0,0.12)",
          display: "flex",
          flexDirection: "column",
        }}
        onClick={(e) => e.stopPropagation()}
      >
        {children}
      </div>
    </div>
  );
};

export const SheetTrigger = ({ children, asChild }: { children: React.ReactNode; asChild?: boolean }) => {
  return <>{children}</>;
};

export const SheetClose = ({ children }: { children?: React.ReactNode }) => {
  return <>{children}</>;
};

export function SheetContent({ children, ...p }: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      style={{
        display: "flex",
        flexDirection: "column",
        height: "100%",
        background: "#fff",
      }}
      {...p}
    >
      {children}
    </div>
  );
}

export function SheetHeader({ children, ...p }: React.HTMLAttributes<HTMLDivElement>) {
  return <div style={{ padding: "16px 24px", borderBottom: "1px solid #e4e4e7" }} {...p}>{children}</div>;
}

export function SheetTitle({ children, ...p }: React.HTMLAttributes<HTMLHeadingElement>) {
  return <h2 style={{ fontSize: "16px", fontWeight: 600 }} {...p}>{children}</h2>;
}

export function SheetBody({ children, ...p }: React.HTMLAttributes<HTMLDivElement>) {
  return <div style={{ flex: 1, overflowY: "auto", padding: "24px" }} {...p}>{children}</div>;
}
