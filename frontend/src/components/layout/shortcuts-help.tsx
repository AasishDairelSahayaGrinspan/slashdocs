import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { cn } from "@/lib/cn";

// How consecutive key chips relate: pressed one after the other ("then"),
// held together ("combo"), or either one works ("or").
type Join = "then" | "combo" | "or";

interface ShortcutItem {
  keys: string[];
  join: Join;
  label: string;
}

const SHORTCUT_GROUPS: ReadonlyArray<{
  title: string;
  items: ReadonlyArray<ShortcutItem>;
}> = [
  {
    title: "Document viewer",
    items: [
      { keys: ["Esc", "T"], join: "then", label: "Cycle theme" },
      { keys: ["Esc", "F"], join: "then", label: "Full screen" },
      { keys: ["Esc", "C"], join: "then", label: "Toggle comments" },
      { keys: ["Esc", "H"], join: "then", label: "Back home" },
      { keys: ["Esc"], join: "then", label: "Exit / deselect" },
    ],
  },
  {
    title: "Reading",
    items: [
      { keys: ["J", "K"], join: "or", label: "Scroll lines" },
      { keys: ["Ctrl D", "Ctrl U"], join: "or", label: "Half page" },
      { keys: ["G G", "Shift G"], join: "or", label: "Top / bottom" },
      { keys: ["Pinch", "Ctrl Scroll"], join: "or", label: "Zoom document" },
    ],
  },
  {
    title: "Comments",
    items: [
      { keys: ["Select text"], join: "or", label: "Start comment" },
      { keys: ["⌘ / Ctrl", "Enter"], join: "combo", label: "Post comment" },
    ],
  },
];

const SEPARATORS: Record<Join, string> = {
  then: "then",
  combo: "+",
  or: "/",
};

function Keys({ item }: { item: ShortcutItem }) {
  return (
    <div className="flex flex-wrap items-center gap-1.5">
      {item.keys.map((key, i) => (
        <span key={key} className="flex items-center gap-1.5">
          {i > 0 && (
            <span className="text-[10px] text-[var(--color-fg-muted)]">
              {SEPARATORS[item.join]}
            </span>
          )}
          <kbd className="inline-flex h-6 min-w-6 items-center justify-center rounded-[6px] border border-[var(--color-border)] bg-[var(--color-surface)] px-1.5 font-mono text-[11px] font-medium text-[var(--color-fg)]">
            {key}
          </kbd>
        </span>
      ))}
    </div>
  );
}

export function ShortcutList({ className }: { className?: string }) {
  return (
    <div className={cn("space-y-5", className)}>
      {SHORTCUT_GROUPS.map((group) => (
        <section key={group.title}>
          <h4 className="mb-1.5 text-[11px] font-medium uppercase tracking-wider text-[var(--color-fg-muted)]">
            {group.title}
          </h4>
          <ul>
            {group.items.map((item) => (
              <li
                key={item.label}
                className="grid grid-cols-1 items-center gap-x-4 gap-y-1 py-1 sm:grid-cols-[11rem_1fr]"
              >
                <Keys item={item} />
                <div className="text-sm text-[var(--color-fg-muted)]">{item.label}</div>
              </li>
            ))}
          </ul>
        </section>
      ))}
    </div>
  );
}

export function ShortcutsHelpDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>Keyboard shortcuts</DialogTitle>
          <DialogDescription>
            Esc chords: press Esc, then the key. All of these work while reading a
            document.
          </DialogDescription>
        </DialogHeader>
        <div className="max-h-[60vh] overflow-y-auto pr-1">
          <ShortcutList />
        </div>
      </DialogContent>
    </Dialog>
  );
}
