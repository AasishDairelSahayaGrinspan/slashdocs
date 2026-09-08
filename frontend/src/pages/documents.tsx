import { useTitle } from "@/lib/use-title";
import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useOnboarding, autoMarkFirstDoc } from "@/lib/onboarding";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { errorMessage } from "@/lib/errors";
import { MoreHorizontal, Plus, Sparkles } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Skeleton, EmptyState } from "@/components/ui/misc";
import { Dropdown, DropdownContent, DropdownTrigger } from "@/components/ui/dropdown";
import { api, docID, docTitle, type Folder, type FolderEntry } from "@/api";
import { queryKeys } from "@/lib/queryKeys";
import { viewModeStore } from "@/lib/view-mode";
import { DRAG_TYPE, type ActiveDrag } from "@/components/browse/drag";
import { BrowseDragOverlay } from "@/components/browse/drag-overlay";
import { UnifiedGrid } from "@/components/browse/unified-grid";
import { UnifiedTable } from "@/components/browse/unified-table";
import { ViewModeToggle } from "@/components/browse/view-mode-toggle";
import { AccessWarningDialog, NewFolderDialog } from "@/components/browse/dialogs";
import { UploadBackupItem } from "@/components/browse/upload-backup";
import { DocumentSearchInput } from "@/components/browse/document-search-input";
import { hasDocumentSearchOperators } from "@/lib/search-query";
import {
  DndContext,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragStartEvent,
} from "@dnd-kit/core";

export function DocumentsPage() {
  useTitle("Documents");
  const [q, setQ] = useState("");
  const [debouncedQ, setDebouncedQ] = useState("");
  useEffect(() => {
    const t = window.setTimeout(() => setDebouncedQ(q.trim()), 250);
    return () => window.clearTimeout(t);
  }, [q]);
  const me = useQuery({ queryKey: queryKeys.me(), queryFn: api.me });
  const config = useQuery({ queryKey: queryKeys.config(), queryFn: api.getConfig });
  const searchTagsEnabled = config.data?.search_tags_enabled === true;
  const docs = useQuery({
    queryKey: [...queryKeys.documents(), debouncedQ],
    queryFn: () => api.listDocuments(debouncedQ || undefined),
    placeholderData: (prev) => prev,
  });
  const pins = useQuery({
    queryKey: queryKeys.pins(),
    queryFn: api.listPins,
  });
  const structuredSearch = searchTagsEnabled && hasDocumentSearchOperators(debouncedQ);
  const rootFolders = useQuery({
    queryKey: [...queryKeys.folders(), debouncedQ],
    queryFn: () => api.listFolders(undefined, undefined, debouncedQ || undefined),
    placeholderData: (prev) => prev,
    enabled: !structuredSearch,
  });
  const { state: onb } = useOnboarding();
  const nav = useNavigate();
  useEffect(() => {
    if (onb.status === "active" && !onb.steps.cli_installed && !onb.steps.cli_authed) {
      nav("/app/start", { replace: true });
    }
  }, [onb, nav]);
  useEffect(() => {
    if (docs.data && (docs.data.items || []).length > 0) {
      autoMarkFirstDoc();
    }
  }, [docs.data]);
  const documents = useMemo(() => docs.data?.items || [], [docs.data]);
  const folderEntries = (structuredSearch ? [] : rootFolders.data?.items || []).filter(
    (e): e is FolderEntry & { folder: Folder } => e.type === "folder" && !!e.folder,
  );
  const folders = folderEntries.map((e) => e.folder);

  const pinnedIds = useMemo(
    () => new Set((pins.data?.items || []).map((p) => p.resource_id)),
    [pins.data],
  );

  // Merge folders and documents into a single list sorted by pinned status, last activity, then alphabetically
  const merged = useMemo(() => {
    const folderItems: FolderEntry[] = folderEntries.map((e) => ({
      type: "folder" as const,
      folder: e.folder,
      last_active: e.last_active,
    }));
    const docItems: FolderEntry[] = documents.map((d) => ({
      type: "document" as const,
      document: d,
      last_active: d.updated_at,
    }));
    return [...folderItems, ...docItems].sort((a, b) => {
      const ap = pinnedIds.has(a.folder?.id || docID(a.document!));
      const bp = pinnedIds.has(b.folder?.id || docID(b.document!));
      if (ap !== bp) return ap ? -1 : 1;
      const ta = a.last_active || "";
      const tb = b.last_active || "";
      if (ta && tb) return tb.localeCompare(ta);
      if (tb) return 1;
      if (ta) return -1;
      const na = (a.folder?.title || a.document?.title || "").toLowerCase();
      const nb = (b.folder?.title || b.document?.title || "").toLowerCase();
      return na.localeCompare(nb);
    });
  }, [folderEntries, documents, pinnedIds]);

  const viewMode = viewModeStore.useStore();
  const [newFolderOpen, setNewFolderOpen] = useState(false);
  const qc = useQueryClient();

  // Drag and drop
  const [activeDrag, setActiveDrag] = useState<ActiveDrag | null>(null);
  const [accessWarning, setAccessWarning] = useState<string | null>(null);
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 8 } }),
  );
  const moveMutation = useMutation({
    mutationFn: async ({
      dragType,
      dragId,
      targetFolderId,
    }: {
      dragType: string;
      dragId: string;
      targetFolderId: string;
    }) => {
      if (dragType === DRAG_TYPE.folder) {
        return api.updateFolder(dragId, undefined, targetFolderId);
      } else {
        return api.moveDocumentToFolder(targetFolderId, dragId);
      }
    },
    onSuccess: (data) => {
      qc.invalidateQueries({ queryKey: queryKeys.foldersAll() });
      qc.invalidateQueries({ queryKey: queryKeys.documents() });
      const warning = (data as Record<string, unknown>)?.permission_warning as
        { message?: string } | undefined;
      if (warning?.message) {
        setAccessWarning(warning.message);
      } else {
        toast.success("Moved.");
      }
    },
    onError: (e) => toast.error(errorMessage(e, "Move failed")),
  });

  function handleDragStart(event: DragStartEvent) {
    const raw = event.active.id as string;
    const [type, id] = raw.split(":", 2);
    const dragged =
      type === DRAG_TYPE.folder
        ? folders.find((f) => f.id === id)?.title
        : (() => {
            const doc = documents.find((d) => docID(d) === id);
            return doc ? docTitle(doc) : undefined;
          })();
    // The entry can vanish mid-drag (e.g. a background refetch removed it);
    // fall back to a generic label rather than crashing.
    const title = dragged || (type === DRAG_TYPE.folder ? "Folder" : "Document");
    setActiveDrag({ type, id, title });
  }
  function handleDragEnd(event: DragEndEvent) {
    setActiveDrag(null);
    const { active, over } = event;
    if (!over) return;
    const [dragType, dragId] = (active.id as string).split(":", 2);
    const targetFolderId = (over.id as string).replace("drop:", "");
    if (dragType === DRAG_TYPE.folder && dragId === targetFolderId) return;
    moveMutation.mutate({ dragType, dragId, targetFolderId });
  }

  if (docs.isLoading) {
    return (
      <div className="mx-auto max-w-6xl px-6 py-10">
        <Skeleton className="mb-3 h-9 w-48" />
        <Skeleton className="h-64 w-full" />
      </div>
    );
  }
  const searching = debouncedQ.length > 0;
  const empty = !searching && documents.length === 0 && folders.length === 0;

  return (
    <div className="mx-auto max-w-6xl px-4 py-6 sm:px-6 sm:py-10">
      <div className="mb-6 flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between sm:gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Documents</h1>
          <p className="mt-1 text-sm text-[var(--color-fg-muted)]">
            Documents your agents have published.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <DocumentSearchInput
            value={q}
            onChange={setQ}
            searchTagsEnabled={searchTagsEnabled}
          />
          <ViewModeToggle />
          <Button
            variant="outline"
            size="md"
            aria-label="New folder"
            tooltip="New folder"
            onClick={() => setNewFolderOpen(true)}
            className="shrink-0 px-3 sm:px-4"
          >
            <Plus className="h-3.5 w-3.5 sm:mr-1.5" />{" "}
            <span className="hidden sm:inline">New folder</span>
          </Button>
          <Dropdown>
            <DropdownTrigger asChild>
              <Button
                variant="outline"
                size="icon"
                className="shrink-0"
                aria-label="More"
              >
                <MoreHorizontal className="h-4 w-4" />
              </Button>
            </DropdownTrigger>
            <DropdownContent>
              <UploadBackupItem />
            </DropdownContent>
          </Dropdown>
        </div>
      </div>

      {docs.error ? (
        <EmptyState
          title={searching ? "Search query not valid" : "Could not load documents"}
          description={errorMessage(docs.error, "Could not search documents.")}
        />
      ) : empty ? (
        <EmptyDocs />
      ) : searching ? (
        documents.length === 0 && folders.length === 0 ? (
          <EmptyState title="No matches" description={`Nothing matches "${q}".`} />
        ) : viewMode === "list" ? (
          <UnifiedTable entries={merged} principal={me.data?.principal} />
        ) : (
          <UnifiedGrid entries={merged} />
        )
      ) : (
        <DndContext
          sensors={sensors}
          onDragStart={handleDragStart}
          onDragEnd={handleDragEnd}
        >
          {viewMode === "list" ? (
            <UnifiedTable
              entries={merged}
              principal={me.data?.principal}
              pinnedIds={pinnedIds}
            />
          ) : (
            <UnifiedGrid entries={merged} pinnedIds={pinnedIds} />
          )}
          <BrowseDragOverlay activeDrag={activeDrag} viewMode={viewMode} />
        </DndContext>
      )}

      <NewFolderDialog open={newFolderOpen} onOpenChange={setNewFolderOpen} />

      <AccessWarningDialog
        warning={accessWarning}
        onClose={() => setAccessWarning(null)}
      />
    </div>
  );
}

function EmptyDocs() {
  return (
    <div className="mx-auto max-w-xl space-y-4">
      <div className="rounded-[14px] border border-[var(--color-border)] bg-[var(--color-surface)] p-6">
        <div className="mb-3 flex h-10 w-10 items-center justify-center rounded-md bg-[var(--color-accent-muted)] text-[var(--color-accent)]">
          <Sparkles className="h-5 w-5" />
        </div>
        <h3 className="text-base font-semibold">Set up your agent</h3>
        <p className="mt-1 text-sm text-[var(--color-fg-muted)]">
          Documents are published by your AI agent. Walk through the three-step setup so
          it knows how to use aidocs.
        </p>
        <div className="mt-4">
          <Button asChild>
            <Link to="/app/start">Open setup guide</Link>
          </Button>
        </div>
      </div>
    </div>
  );
}
