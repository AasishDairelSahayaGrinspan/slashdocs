import { FileText, Folder as FolderIcon } from "lucide-react";
import { DragOverlay } from "@dnd-kit/core";
import type { ViewMode } from "@/lib/view-mode";
import { DRAG_TYPE, type ActiveDrag } from "./drag";

// Ghost preview rendered while dragging a card or row; shared verbatim between
// the documents and folder pages.
export function BrowseDragOverlay({
  activeDrag,
  viewMode,
}: {
  activeDrag: ActiveDrag | null;
  viewMode: ViewMode;
}) {
  return (
    <DragOverlay dropAnimation={null}>
      {activeDrag && viewMode === "gallery" && (
        <div className="flex h-36 w-48 flex-col items-center justify-center rounded-lg border border-[var(--color-accent)] bg-[var(--color-surface)] opacity-30 shadow-xl">
          {activeDrag.type === DRAG_TYPE.folder ? (
            <FolderIcon className="h-10 w-10 text-[var(--color-accent)]/60" />
          ) : (
            <FileText className="h-10 w-10 text-[var(--color-fg-muted)]/30" />
          )}
          <div className="mt-2 max-w-[10rem] truncate px-3 text-sm font-medium">
            {activeDrag.title}
          </div>
        </div>
      )}
      {activeDrag && viewMode === "list" && (
        <div className="flex w-64 items-center gap-2 rounded-md border border-[var(--color-accent)] bg-[var(--color-surface)] px-3 py-2 opacity-30 shadow-xl">
          {activeDrag.type === DRAG_TYPE.folder ? (
            <FolderIcon className="h-4 w-4 text-[var(--color-accent)]" />
          ) : (
            <FileText className="h-4 w-4 text-[var(--color-fg-muted)]" />
          )}
          <span className="truncate text-sm font-medium">{activeDrag.title}</span>
        </div>
      )}
    </DragOverlay>
  );
}
