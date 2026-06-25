import * as React from "react";
import { Badge as BladeBadge } from "@razorpay/blade/components";

type Variant = "default" | "muted" | "accent" | "success" | "warning" | "danger";

const variantMap: Record<Variant, { color: "neutral" | "positive" | "negative" | "notice" | "primary" }> = {
  default: { color: "neutral" },
  muted: { color: "neutral" },
  accent: { color: "primary" },
  success: { color: "positive" },
  warning: { color: "notice" },
  danger: { color: "negative" },
};

export function Badge({
  variant = "default",
  children,
  ...p
}: React.HTMLAttributes<HTMLSpanElement> & { variant?: Variant }) {
  const bladeProps = variantMap[variant] || variantMap.default;
  return (
    <BladeBadge
      color={bladeProps.color}
      size="medium"
    >
      {children as string}
    </BladeBadge>
  );
}
