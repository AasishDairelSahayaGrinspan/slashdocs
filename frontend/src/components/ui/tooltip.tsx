import * as React from "react";
import * as Primitive from "@radix-ui/react-tooltip";

const HasTooltipProvider = React.createContext(false);

export function TooltipProvider({ children }: { children: React.ReactNode }) {
  return (
    <HasTooltipProvider.Provider value={true}>
      <Primitive.Provider delayDuration={500} skipDelayDuration={0}>
        {children}
      </Primitive.Provider>
    </HasTooltipProvider.Provider>
  );
}

export function Tooltip({
  label,
  children,
}: {
  label: string;
  children: React.ReactElement<{ title?: string; disabled?: boolean }>;
}) {
  const hasProvider = React.useContext(HasTooltipProvider);
  // Native titles otherwise appear on top of the custom tooltip after a delay.
  const child = React.cloneElement(children, { title: undefined });
  const content = (
    <Primitive.Root>
      <Primitive.Trigger asChild>
        {children.props.disabled ? (
          <span className="inline-flex shrink-0">{child}</span>
        ) : (
          child
        )}
      </Primitive.Trigger>
      <Primitive.Portal>
        <Primitive.Content
          side="bottom"
          sideOffset={6}
          collisionPadding={8}
          className="icon-tooltip z-[200] max-w-64 rounded-md bg-zinc-900 px-2.5 py-1.5 text-xs font-medium leading-snug text-white shadow-lg dark:bg-zinc-100 dark:text-zinc-900"
        >
          {label}
          <Primitive.Arrow
            className="fill-zinc-900 dark:fill-zinc-100"
            width={8}
            height={4}
          />
        </Primitive.Content>
      </Primitive.Portal>
    </Primitive.Root>
  );
  // Standalone controls (including component previews) also work without the app shell.
  return hasProvider ? content : <TooltipProvider>{content}</TooltipProvider>;
}
