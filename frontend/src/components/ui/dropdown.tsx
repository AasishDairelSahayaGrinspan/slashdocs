import * as React from "react";

export const Dropdown = ({ children }: { children: React.ReactNode } & Record<string, unknown>) => {
  return <>{children}</>;
};

export const DropdownTrigger = ({ children, asChild }: { children: React.ReactNode; asChild?: boolean }) => {
  return <>{children}</>;
};

export function DropdownContent({ children, ...p }: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      style={{
        minWidth: "200px",
        borderRadius: "10px",
        border: "1px solid #e4e4e7",
        background: "#fff",
        padding: "4px",
        boxShadow: "0 4px 12px rgba(0,0,0,0.08)",
      }}
      {...p}
    >
      {children}
    </div>
  );
}

export function DropdownItem({ children, ...p }: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      style={{
        display: "flex",
        cursor: "pointer",
        alignItems: "center",
        gap: "8px",
        borderRadius: "6px",
        padding: "6px 8px",
        fontSize: "14px",
      }}
      {...p}
    >
      {children}
    </div>
  );
}

export function DropdownSeparator() {
  return <div style={{ margin: "4px 0", height: "1px", background: "#e4e4e7" }} />;
}

export function DropdownLabel({ children, ...p }: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      style={{
        padding: "4px 8px",
        fontSize: "11px",
        fontWeight: 500,
        textTransform: "uppercase",
        letterSpacing: "0.05em",
        color: "#71717a",
      }}
      {...p}
    >
      {children}
    </div>
  );
}
