import * as React from "react";
import { Button as BladeButton } from "@razorpay/blade/components";

type Variant = "primary" | "secondary" | "outline" | "ghost" | "danger" | "link";
type Size = "sm" | "md" | "lg" | "icon";

export interface ButtonProps
  extends Omit<React.ButtonHTMLAttributes<HTMLButtonElement>, "color"> {
  variant?: Variant;
  size?: Size;
  asChild?: boolean;
  icon?: React.ReactNode;
  isLoading?: boolean;
  isFullWidth?: boolean;
}

const variantMap: Record<Variant, { variant: "primary" | "secondary" | "tertiary"; color?: "primary" | "negative" }> = {
  primary: { variant: "primary" },
  secondary: { variant: "secondary" },
  outline: { variant: "secondary" },
  ghost: { variant: "tertiary" },
  danger: { variant: "primary", color: "negative" },
  link: { variant: "tertiary" },
};

const sizeMap: Record<Size, "xsmall" | "small" | "medium" | "large"> = {
  sm: "small",
  md: "medium",
  lg: "large",
  icon: "small",
};

export const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ variant = "primary", size = "md", asChild, children, ...props }, ref) => {
    const bladeProps = variantMap[variant] || variantMap.primary;
    const bladeSize = sizeMap[size] || sizeMap.md;

    if (asChild && React.isValidElement(children)) {
      const childProps = (children as React.ReactElement).props as Record<string, unknown>;
      return React.cloneElement(children as React.ReactElement, {
        ...childProps,
        ...props,
      });
    }

    return (
      <BladeButton
        variant={bladeProps.variant}
        color={bladeProps.color as "primary" | "negative" | undefined}
        size={bladeSize}
        onClick={props.onClick as never}
        isDisabled={props.disabled}
        type={props.type as "button" | "reset" | "submit" | undefined}
        isLoading={props.isLoading}
        isFullWidth={props.isFullWidth}
      >
        {children as string}
      </BladeButton>
    );
  },
);
Button.displayName = "Button";
