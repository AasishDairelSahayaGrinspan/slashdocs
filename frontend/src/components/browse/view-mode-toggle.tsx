import { LayoutGrid, List } from "lucide-react";
import { Button } from "@/components/ui/button";
import { viewModeStore } from "@/lib/view-mode";

export function ViewModeToggle() {
  const viewMode = viewModeStore.useStore();
  return (
    <div className="flex shrink-0 rounded-md border border-[var(--color-border)]">
      <Button
        variant="ghost"
        size="icon"
        className={`rounded-r-none ${viewMode === "gallery" ? "bg-[var(--color-surface-muted)]" : ""}`}
        onClick={() => viewModeStore.write("gallery")}
        aria-label="Gallery view"
      >
        <LayoutGrid className="h-4 w-4" />
      </Button>
      <Button
        variant="ghost"
        size="icon"
        className={`rounded-l-none border-l border-[var(--color-border)] ${viewMode === "list" ? "bg-[var(--color-surface-muted)]" : ""}`}
        onClick={() => viewModeStore.write("list")}
        aria-label="List view"
      >
        <List className="h-4 w-4" />
      </Button>
    </div>
  );
}
