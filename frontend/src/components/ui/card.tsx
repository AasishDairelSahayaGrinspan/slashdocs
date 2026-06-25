import * as React from "react";

export function Card({
  children,
  ...p
}: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      style={{
        borderRadius: "14px",
        border: "1px solid #e4e4e7",
        background: "#fff",
      }}
      {...p}
    >
      {children}
    </div>
  );
}

export function CardHeader({ children, ...p }: React.HTMLAttributes<HTMLDivElement>) {
  return <div style={{ padding: "20px 20px 12px" }} {...p}>{children}</div>;
}

export function CardTitle({ children, ...p }: React.HTMLAttributes<HTMLHeadingElement>) {
  return <h3 style={{ fontSize: "16px", fontWeight: 600, letterSpacing: "-0.02em" }} {...p}>{children}</h3>;
}

export function CardDescription({ children, ...p }: React.HTMLAttributes<HTMLParagraphElement>) {
  return <p style={{ fontSize: "14px", color: "#71717a" }} {...p}>{children}</p>;
}

export function CardContent({ children, ...p }: React.HTMLAttributes<HTMLDivElement>) {
  return <div style={{ padding: "20px 8px" }} {...p}>{children}</div>;
}

export function CardFooter({ children, ...p }: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      style={{
        display: "flex",
        alignItems: "center",
        gap: "8px",
        padding: "20px 20px 0",
        borderTop: "1px solid #e4e4e7",
        marginTop: "12px",
      }}
      {...p}
    >
      {children}
    </div>
  );
}
