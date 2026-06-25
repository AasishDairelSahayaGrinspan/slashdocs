import * as React from "react";
import { Button } from "./button";

export const AlertDialog = ({ open, onOpenChange, children }: {
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
      <div style={{ position: "absolute", inset: 0, background: "rgba(0,0,0,0.4)" }} />
      <div
        style={{ position: "relative", maxWidth: "448px", width: "100%", borderRadius: "14px", border: "1px solid #e4e4e7", background: "#fff", padding: "24px", boxShadow: "0 8px 24px rgba(0,0,0,0.12)" }}
        onClick={(e) => e.stopPropagation()}
      >
        {children}
      </div>
    </div>
  );
};

export const AlertDialogTrigger = ({ children, asChild }: { children: React.ReactNode; asChild?: boolean }) => {
  return <>{children}</>;
};

export function AlertDialogContent({ children, ...p }: React.HTMLAttributes<HTMLDivElement>) {
  return <div {...p}>{children}</div>;
}

export function AlertDialogTitle({ children, ...p }: React.HTMLAttributes<HTMLHeadingElement>) {
  return <h2 style={{ fontSize: "18px", fontWeight: 600 }} {...p}>{children}</h2>;
}

export function AlertDialogDescription({ children, ...p }: React.HTMLAttributes<HTMLParagraphElement>) {
  return <p style={{ marginTop: "4px", fontSize: "14px", color: "#71717a" }} {...p}>{children}</p>;
}

export function AlertDialogFooter({ children, ...p }: React.HTMLAttributes<HTMLDivElement>) {
  return <div style={{ marginTop: "24px", display: "flex", justifyContent: "flex-end", gap: "8px" }} {...p}>{children}</div>;
}

export function AlertDialogCancel({ children, ...p }: React.ButtonHTMLAttributes<HTMLButtonElement>) {
  return <Button variant="outline" {...p}>{children}</Button>;
}

export function AlertDialogAction({
  children,
  variant = "primary",
  ...p
}: React.ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: "primary" | "danger";
}) {
  return <Button variant={variant} {...p}>{children}</Button>;
}
