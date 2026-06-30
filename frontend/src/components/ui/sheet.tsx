import * as React from "react";
import * as DialogPrimitive from "@radix-ui/react-dialog";
import { X } from "lucide-react";
import { cn } from "@/lib/cn";

export const Sheet = DialogPrimitive.Root;
export const SheetTrigger = DialogPrimitive.Trigger;
export const SheetClose = DialogPrimitive.Close;

export function SheetContent({
  className,
  children,
  side = "right",
  ...p
}: React.ComponentPropsWithoutRef<typeof DialogPrimitive.Content> & {
  side?: "right" | "left";
}) {
  return (
    <DialogPrimitive.Portal>
      <DialogPrimitive.Overlay className="fixed inset-0 z-50 bg-black/40 transition-opacity duration-150 ease-out data-[state=closed]:opacity-0 data-[state=open]:opacity-100" />
      <DialogPrimitive.Content
        className={cn(
          "fixed z-50 top-0 flex h-full w-full max-w-md flex-col border-[var(--color-border)] bg-[var(--color-surface)] shadow-xl transition-[opacity,transform] duration-200 ease-out data-[state=closed]:opacity-0 data-[state=open]:opacity-100 focus:outline-none",
          side === "right"
            ? "right-0 border-l data-[state=closed]:translate-x-2 data-[state=open]:translate-x-0"
            : "left-0 border-r data-[state=closed]:-translate-x-2 data-[state=open]:translate-x-0",
          className,
        )}
        {...p}
      >
        {children}
        <DialogPrimitive.Close className="absolute right-4 top-4 inline-flex min-h-10 min-w-10 items-center justify-center rounded-md p-1 text-[var(--color-fg-muted)] transition-[background-color,color,transform] duration-150 ease-out active:scale-[0.96] hover:bg-[var(--color-surface-muted)] hover:text-[var(--color-fg)]">
          <X className="h-4 w-4" />
        </DialogPrimitive.Close>
      </DialogPrimitive.Content>
    </DialogPrimitive.Portal>
  );
}
export function SheetHeader({
  className,
  ...p
}: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn(
        "px-6 py-4 border-b border-[var(--color-border)]",
        className,
      )}
      {...p}
    />
  );
}
export function SheetTitle({
  className,
  ...p
}: React.ComponentPropsWithoutRef<typeof DialogPrimitive.Title>) {
  return (
    <DialogPrimitive.Title
      className={cn("text-balance text-base font-semibold", className)}
      {...p}
    />
  );
}
export function SheetBody({
  className,
  ...p
}: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div className={cn("flex-1 overflow-y-auto p-6", className)} {...p} />
  );
}
