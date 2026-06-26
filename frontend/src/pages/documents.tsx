import { useTitle } from "@/lib/use-title";
import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useOnboarding, autoMarkFirstDoc } from "@/lib/onboarding";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { errorMessage } from "@/lib/errors";
import {
  FileText,
  Folder as FolderIcon,
  MoreHorizontal,
  Pin,
  PinOff,
  Plus,
  Search,
  Sparkles,
  Trash2,
  Upload,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Avatar, Center, Skeleton, EmptyState } from "@/components/ui/misc";
import {
  Dropdown,
  DropdownContent,
  DropdownItem,
  DropdownTrigger,
} from "@/components/ui/dropdown";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { HtmlFileInput, useStagedFile } from "@/components/ui/upload";
import { DeleteDocumentDialog } from "@/components/delete-document-dialog";
import {
  api,
  docID,
  docTitle,
  type Document,
  type Folder,
  type FolderEntry,
  type Principal,
} from "@/api";
import { queryKeys } from "@/lib/queryKeys";
import { createPersistentStore } from "@/lib/persistent-store";
import { LayoutGrid, List } from "lucide-react";
import {
  DndContext,
  DragOverlay,
  useDraggable,
  useDroppable,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragStartEvent,
} from "@dnd-kit/core";

export type ViewMode = "list" | "gallery";
export const viewModeStore = createPersistentStore<ViewMode>(
  "aidocs:view-mode",
  "gallery",
);

export function DocumentsPage() {
  useTitle("Documents");
  const [q, setQ] = useState("");
  const [debouncedQ, setDebouncedQ] = useState("");
  useEffect(() => {
    const t = window.setTimeout(() => setDebouncedQ(q.trim()), 250);
    return () => window.clearTimeout(t);
  }, [q]);
  const me = useQuery({ queryKey: queryKeys.me(), queryFn: api.me });
  const docs = useQuery({
    queryKey: [...queryKeys.documents(), debouncedQ],
    queryFn: () => api.listDocuments(debouncedQ || undefined),
    placeholderData: (prev) => prev,
  });
  const pins = useQuery({
    queryKey: queryKeys.pins(),
    queryFn: api.listPins,
  });
  const rootFolders = useQuery({
    queryKey: [...queryKeys.folders(), debouncedQ],
    queryFn: () => api.listFolders(undefined, undefined, debouncedQ || undefined),
    placeholderData: (prev) => prev,
  });
  const { state: onb } = useOnboarding();
  const nav = useNavigate();
  useEffect(() => {
    if (
      onb.status === "active" &&
      !onb.steps.cli_installed &&
      !onb.steps.cli_authed &&
      !onb.steps.agent_configured
    ) {
      nav("/app/start", { replace: true });
    }
  }, [onb, nav]);
  useEffect(() => {
    if (docs.data && (docs.data.items || []).length > 0) {
      autoMarkFirstDoc();
    }
  }, [docs.data]);
  const documents = useMemo(() => docs.data?.items || [], [docs.data]);
  const pinnedItems = pins.data?.items || [];
  const folderEntries = (rootFolders.data?.items || []).filter(
    (e): e is FolderEntry & { folder: Folder } => e.type === "folder" && !!e.folder,
  );
  const folders = folderEntries.map((e) => e.folder);

  const pinnedIds = useMemo(
    () => new Set(pinnedItems.map((p) => p.resource_id)),
    [pinnedItems],
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
  const [newFolderTitle, setNewFolderTitle] = useState("");
  const qc = useQueryClient();
  const createFolder = useMutation({
    mutationFn: () => api.createFolder(newFolderTitle),
    onSuccess: () => {
      setNewFolderOpen(false);
      setNewFolderTitle("");
      qc.invalidateQueries({ queryKey: queryKeys.folders() });
      toast.success("Created folder.");
    },
    onError: (e) => toast.error(errorMessage(e, "Create failed")),
  });

  // Drag and drop
  const [activeDrag, setActiveDrag] = useState<{
    type: string;
    id: string;
    title: string;
  } | null>(null);
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
      if (dragType === "folder") {
        return api.updateFolder(dragId, undefined, targetFolderId);
      } else {
        return api.moveDocumentToFolder(targetFolderId, dragId);
      }
    },
    onSuccess: (data) => {
      qc.invalidateQueries({ queryKey: ["folders"] });
      qc.invalidateQueries({ queryKey: queryKeys.documents() });
      const warning = (data as Record<string, unknown>)?.permission_warning as
        | { message?: string }
        | undefined;
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
    const title =
      type === "folder"
        ? folders.find((f) => f.id === id)?.title || "Folder"
        : docTitle(documents.find((d) => docID(d) === id)!);
    setActiveDrag({ type, id, title });
  }
  function handleDragEnd(event: DragEndEvent) {
    setActiveDrag(null);
    const { active, over } = event;
    if (!over) return;
    const [dragType, dragId] = (active.id as string).split(":", 2);
    const targetFolderId = (over.id as string).replace("drop:", "");
    if (dragType === "folder" && dragId === targetFolderId) return;
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
  if (docs.error) return <Center>Could not load documents.</Center>;

  const searching = debouncedQ.length > 0;
  const empty = !searching && documents.length === 0 && folders.length === 0;

  return (
    <div className="mx-auto max-w-6xl px-6 py-10">
      <div className="mb-6 flex items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Documents</h1>
          <p className="mt-1 text-sm text-[var(--color-fg-muted)]">
            Documents your agents have published.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <div className="relative">
            <Search className="pointer-events-none absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-[var(--color-fg-muted)]" />
            <Input
              value={q}
              onChange={(e) => setQ(e.target.value)}
              placeholder="Search…"
              className="w-56 pl-8"
            />
          </div>
          <div className="flex rounded-md border border-[var(--color-border)]">
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
          <Button variant="outline" size="md" onClick={() => setNewFolderOpen(true)}>
            <Plus className="mr-1.5 h-3.5 w-3.5" /> New folder
          </Button>
          <Dropdown>
            <DropdownTrigger asChild>
              <Button variant="outline" size="icon" aria-label="More">
                <MoreHorizontal className="h-4 w-4" />
              </Button>
            </DropdownTrigger>
            <DropdownContent>
              <UploadBackupItem />
            </DropdownContent>
          </Dropdown>
        </div>
      </div>

      {empty ? (
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
          <DragOverlay dropAnimation={null}>
            {activeDrag && viewMode === "gallery" && (
              <div className="flex h-36 w-48 flex-col items-center justify-center rounded-lg border border-[var(--color-accent)] bg-[var(--color-surface)] opacity-30 shadow-xl">
                {activeDrag.type === "folder" ? (
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
                {activeDrag.type === "folder" ? (
                  <FolderIcon className="h-4 w-4 text-[var(--color-accent)]" />
                ) : (
                  <FileText className="h-4 w-4 text-[var(--color-fg-muted)]" />
                )}
                <span className="truncate text-sm font-medium">{activeDrag.title}</span>
              </div>
            )}
          </DragOverlay>
        </DndContext>
      )}

      {/* New folder dialog */}
      <Dialog open={newFolderOpen} onOpenChange={setNewFolderOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>New folder</DialogTitle>
          </DialogHeader>
          <form
            onSubmit={(e) => {
              e.preventDefault();
              if (newFolderTitle.trim()) createFolder.mutate();
            }}
          >
            <Input
              placeholder="Folder name"
              value={newFolderTitle}
              onChange={(e) => setNewFolderTitle(e.target.value)}
              autoFocus
            />
            <DialogFooter className="mt-4">
              <Button
                type="button"
                variant="ghost"
                onClick={() => setNewFolderOpen(false)}
              >
                Cancel
              </Button>
              <Button
                type="submit"
                disabled={!newFolderTitle.trim() || createFolder.isPending}
              >
                Create
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Access warning after move */}
      <Dialog open={!!accessWarning} onOpenChange={() => setAccessWarning(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Access may change</DialogTitle>
            <DialogDescription>{accessWarning}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button onClick={() => setAccessWarning(null)}>Got it</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

export function UnifiedGrid({
  entries,
  pinnedIds,
}: {
  entries: FolderEntry[];
  pinnedIds?: Set<string>;
}) {
  const me = useQuery({ queryKey: queryKeys.me(), queryFn: api.me });
  const qc = useQueryClient();
  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
      {entries.map((e) => {
        if (e.type === "folder" && e.folder) {
          const canDelete = samePrincipal(me.data?.principal, e.folder.owner);
          return (
            <DraggableFolderCard
              key={e.folder.id}
              folder={e.folder}
              canDelete={canDelete}
              qc={qc}
              pinned={pinnedIds?.has(e.folder.id)}
            />
          );
        }
        if (e.type === "document" && e.document) {
          const canDelete = samePrincipal(me.data?.principal, e.document.owner);
          return (
            <DraggableDocCard
              key={docID(e.document)}
              doc={e.document}
              canDelete={canDelete}
              qc={qc}
              pinned={pinnedIds?.has(docID(e.document))}
            />
          );
        }
        return null;
      })}
    </div>
  );
}

function DraggableFolderCard({
  folder: f,
  canDelete,
  qc,
  pinned,
}: {
  folder: Folder;
  canDelete: boolean;
  qc: ReturnType<typeof useQueryClient>;
  pinned?: boolean;
}) {
  const {
    attributes,
    listeners,
    setNodeRef: dragRef,
    isDragging,
  } = useDraggable({ id: `folder:${f.id}` });
  const { setNodeRef: dropRef, isOver } = useDroppable({ id: `drop:${f.id}` });

  return (
    <div
      ref={(node) => {
        dragRef(node);
        dropRef(node);
      }}
      {...listeners}
      {...attributes}
      className={`group relative overflow-hidden rounded-lg border bg-[var(--color-surface)] transition-colors hover:bg-[var(--color-surface-muted)] ${isOver ? "border-[var(--color-accent)] ring-2 ring-[var(--color-accent)]/30" : "border-[var(--color-border)]"} ${isDragging ? "opacity-40" : ""}`}
    >
      <Link
        to={`/app/folders/${f.id}`}
        className="relative flex h-36 cursor-default flex-col items-center justify-center px-3 pb-8"
        draggable={false}
      >
        <FolderIcon className="h-10 w-10 text-[var(--color-accent)]/60" />
        <div className="absolute inset-x-3 bottom-2.5">
          <div className="truncate text-sm font-medium">{f.title}</div>
          <div className="truncate text-xs text-[var(--color-fg-muted)]">
            {principalLabel(f.owner)}
          </div>
        </div>
      </Link>
      <GridCardActions
        resourceType="folder"
        resourceId={f.id}
        canDelete={canDelete}
        title={f.title}
        qc={qc}
        pinned={pinned}
      />
    </div>
  );
}

function DraggableDocCard({
  doc: d,
  canDelete,
  qc,
  pinned,
}: {
  doc: Document;
  canDelete: boolean;
  qc: ReturnType<typeof useQueryClient>;
  pinned?: boolean;
}) {
  const { attributes, listeners, setNodeRef, isDragging } = useDraggable({
    id: `doc:${docID(d)}`,
  });

  return (
    <div
      ref={setNodeRef}
      {...listeners}
      {...attributes}
      className={`group relative overflow-hidden rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] transition-colors hover:bg-[var(--color-surface-muted)] ${isDragging ? "opacity-40" : ""}`}
    >
      <Link
        to={`/app/d/${docID(d)}`}
        className="relative flex h-36 cursor-default flex-col items-center justify-center px-3 pb-8"
        draggable={false}
      >
        <FileText className="h-10 w-10 text-[var(--color-fg-muted)]/30" />
        <div className="absolute inset-x-3 bottom-2.5">
          <div className="truncate text-sm font-medium">{docTitle(d)}</div>
          <div className="truncate text-xs text-[var(--color-fg-muted)]">
            {principalLabel(d.owner)}
          </div>
        </div>
      </Link>
      <GridCardActions
        resourceType="document"
        resourceId={docID(d)}
        canDelete={canDelete}
        title={docTitle(d)}
        qc={qc}
        pinned={pinned}
      />
    </div>
  );
}

export function UnifiedTable({
  entries,
  principal,
  pinnedIds,
}: {
  entries: FolderEntry[];
  principal?: Principal;
  pinnedIds?: Set<string>;
}) {
  return (
    <div className="overflow-hidden rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)]">
      <table className="w-full text-sm">
        <thead className="bg-[var(--color-surface-muted)]/50 text-[11px] uppercase tracking-wider text-[var(--color-fg-muted)]">
          <tr>
            <th className="px-4 py-2.5 text-left font-medium">Name</th>
            <th className="px-4 py-2.5 text-left font-medium">Owner</th>
            <th className="w-12 px-2 py-2.5"></th>
          </tr>
        </thead>
        <tbody>
          {entries.map((e) => {
            if (e.type === "folder" && e.folder) {
              return (
                <DraggableFolderRow
                  key={e.folder.id}
                  folder={e.folder}
                  canDelete={samePrincipal(principal, e.folder.owner)}
                  pinned={pinnedIds?.has(e.folder.id)}
                />
              );
            }
            if (e.type === "document" && e.document) {
              return (
                <DraggableDocRow
                  key={docID(e.document)}
                  doc={e.document}
                  canDelete={samePrincipal(principal, e.document.owner)}
                  pinned={pinnedIds?.has(docID(e.document))}
                />
              );
            }
            return null;
          })}
        </tbody>
      </table>
    </div>
  );
}

function DraggableFolderRow({
  folder: f,
  canDelete,
  pinned,
}: {
  folder: Folder;
  canDelete: boolean;
  pinned?: boolean;
}) {
  const {
    attributes,
    listeners,
    setNodeRef: dragRef,
    isDragging,
  } = useDraggable({ id: `folder:${f.id}` });
  const { setNodeRef: dropRef, isOver } = useDroppable({ id: `drop:${f.id}` });
  const q = useQueryClient();
  const del = useMutation({
    mutationFn: () => api.deleteFolder(f.id),
    onSuccess: () => {
      toast.success("Deleted.");
      q.invalidateQueries({ queryKey: ["folders"] });
      q.invalidateQueries({ queryKey: queryKeys.documents() });
      q.invalidateQueries({ queryKey: queryKeys.pins() });
    },
    onError: (e) => toast.error(errorMessage(e, "Delete failed")),
  });

  return (
    <tr
      ref={(node) => {
        dragRef(node);
        dropRef(node);
      }}
      {...listeners}
      {...attributes}
      className={`group border-t transition-colors ${isOver ? "bg-[var(--color-accent)]/10 border-[var(--color-accent)]" : "border-[var(--color-border)] hover:bg-[var(--color-surface-muted)]/40"} ${isDragging ? "opacity-40" : ""}`}
    >
      <td className="px-4 py-3">
        <Link
          to={`/app/folders/${f.id}`}
          className="flex cursor-default items-center gap-2 font-medium hover:underline"
          draggable={false}
        >
          <FolderIcon className="h-4 w-4 text-[var(--color-accent)]" />
          {f.title}
        </Link>
      </td>
      <td className="px-4 py-3">
        <OwnerCell owner={f.owner} />
      </td>
      <td className="px-2 py-3">
        <div
          className="flex gap-0.5"
          onClick={(e) => e.stopPropagation()}
          onPointerDown={(e) => e.stopPropagation()}
        >
          <RowPinButton resourceType="folder" resourceId={f.id} pinned={pinned} />
          {canDelete && (
            <DeleteDocumentDialog
              title={f.title}
              description="This deletes the folder and its subfolders. Documents inside them will move to root."
              onConfirm={() => del.mutate()}
              trigger={
                <Button
                  variant="ghost"
                  size="icon"
                  className="cursor-pointer opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100"
                  aria-label="Delete folder"
                >
                  <Trash2 className="h-3.5 w-3.5 text-[var(--color-danger)]" />
                </Button>
              }
            />
          )}
        </div>
      </td>
    </tr>
  );
}

function DraggableDocRow({
  doc,
  canDelete,
  pinned,
}: {
  doc: Document;
  canDelete: boolean;
  pinned?: boolean;
}) {
  const { attributes, listeners, setNodeRef, isDragging } = useDraggable({
    id: `doc:${docID(doc)}`,
  });
  const q = useQueryClient();
  const del = useMutation({
    mutationFn: () => api.deleteDocument(docID(doc)),
    onSuccess: () => {
      toast.success("Deleted.");
      q.invalidateQueries({ queryKey: ["folders"] });
      q.invalidateQueries({ queryKey: queryKeys.documents() });
      q.invalidateQueries({ queryKey: queryKeys.pins() });
    },
    onError: (e) => toast.error(errorMessage(e, "Delete failed")),
  });
  return (
    <tr
      ref={setNodeRef}
      {...listeners}
      {...attributes}
      className={`group border-t border-[var(--color-border)] transition-colors hover:bg-[var(--color-surface-muted)]/40 ${isDragging ? "opacity-40" : ""}`}
    >
      <td className="px-4 py-3">
        <Link
          to={`/app/d/${docID(doc)}`}
          className="flex cursor-default items-center gap-2 font-medium hover:underline"
          draggable={false}
        >
          <FileText className="h-4 w-4 text-[var(--color-fg-muted)]" />
          {docTitle(doc)}
        </Link>
      </td>
      <td className="px-4 py-3">
        <OwnerCell owner={doc.owner} />
      </td>
      <td className="px-2 py-3">
        <div
          className="flex gap-0.5"
          onClick={(e) => e.stopPropagation()}
          onPointerDown={(e) => e.stopPropagation()}
        >
          <RowPinButton
            resourceType="document"
            resourceId={docID(doc)}
            pinned={pinned}
          />
          {canDelete && (
            <DeleteDocumentDialog
              title={docTitle(doc)}
              onConfirm={() => del.mutate()}
              trigger={
                <Button
                  variant="ghost"
                  size="icon"
                  className="cursor-pointer opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100"
                  aria-label="Delete document"
                >
                  <Trash2 className="h-3.5 w-3.5 text-[var(--color-danger)]" />
                </Button>
              }
            />
          )}
        </div>
      </td>
    </tr>
  );
}

function RowPinButton({
  resourceType,
  resourceId,
  pinned,
}: {
  resourceType: "document" | "folder";
  resourceId: string;
  pinned?: boolean;
}) {
  const qc = useQueryClient();
  const pins = useQuery({ queryKey: queryKeys.pins(), queryFn: api.listPins });
  const isPinned =
    pinned ??
    (pins.data?.items || []).some(
      (p) => p.resource_type === resourceType && p.resource_id === resourceId,
    );
  const togglePin = useMutation({
    mutationFn: () =>
      isPinned
        ? api.deletePin(resourceType, resourceId)
        : api.putPin(resourceType, resourceId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.pins() });
      toast.success(isPinned ? "Unpinned." : "Pinned.");
    },
    onError: (e) => toast.error(errorMessage(e)),
  });
  return (
    <Button
      variant="ghost"
      size="icon"
      className="cursor-pointer opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100"
      aria-label={isPinned ? "Unpin" : "Pin"}
      onClick={(e) => {
        e.preventDefault();
        e.stopPropagation();
        togglePin.mutate();
      }}
    >
      {isPinned ? <PinOff className="h-3.5 w-3.5" /> : <Pin className="h-3.5 w-3.5" />}
    </Button>
  );
}

function GridCardActions({
  resourceType,
  resourceId,
  canDelete,
  title,
  qc,
  pinned,
}: {
  resourceType: "document" | "folder";
  resourceId: string;
  canDelete: boolean;
  title: string;
  qc: ReturnType<typeof useQueryClient>;
  pinned?: boolean;
}) {
  const pins = useQuery({ queryKey: queryKeys.pins(), queryFn: api.listPins });
  const isPinned =
    pinned ??
    (pins.data?.items || []).some(
      (p) => p.resource_type === resourceType && p.resource_id === resourceId,
    );
  const togglePin = useMutation({
    mutationFn: () =>
      isPinned
        ? api.deletePin(resourceType, resourceId)
        : api.putPin(resourceType, resourceId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.pins() });
      toast.success(isPinned ? "Unpinned." : "Pinned.");
    },
    onError: (e) => toast.error(errorMessage(e)),
  });
  const del = useMutation({
    mutationFn: () =>
      resourceType === "folder"
        ? api.deleteFolder(resourceId)
        : api.deleteDocument(resourceId),
    onSuccess: () => {
      toast.success("Deleted.");
      qc.invalidateQueries({ queryKey: ["folders"] });
      qc.invalidateQueries({ queryKey: queryKeys.documents() });
      qc.invalidateQueries({ queryKey: queryKeys.pins() });
    },
    onError: (e) => toast.error(errorMessage(e, "Delete failed")),
  });
  return (
    <div
      className="absolute right-2 top-2 flex gap-0.5 opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100 [&_button]:cursor-pointer"
      onClick={(e) => e.stopPropagation()}
      onPointerDown={(e) => e.stopPropagation()}
    >
      <Button
        variant="ghost"
        size="icon"
        className="h-7 w-7"
        aria-label={isPinned ? "Unpin" : "Pin"}
        onClick={(e) => {
          e.preventDefault();
          e.stopPropagation();
          togglePin.mutate();
        }}
      >
        {isPinned ? (
          <PinOff className="h-3.5 w-3.5" />
        ) : (
          <Pin className="h-3.5 w-3.5" />
        )}
      </Button>
      {canDelete && (
        <DeleteDocumentDialog
          title={title}
          description={
            resourceType === "folder"
              ? "This deletes the folder and its subfolders. Documents inside them will move to root."
              : undefined
          }
          onConfirm={() => del.mutate()}
          trigger={
            <Button
              variant="ghost"
              size="icon"
              className="h-7 w-7"
              aria-label="Delete"
              onClick={(e) => e.stopPropagation()}
            >
              <Trash2 className="h-3.5 w-3.5 text-[var(--color-danger)]" />
            </Button>
          }
        />
      )}
    </div>
  );
}

function OwnerCell({ owner }: { owner?: Principal }) {
  const label = principalLabel(owner);
  const detail =
    owner?.email && owner.email !== label
      ? owner.email
      : owner?.type === "service_account"
        ? "Service account"
        : "Owner";
  return (
    <div className="flex min-w-0 items-center gap-2">
      <Avatar name={label} src={owner?.picture_url} size={28} />
      <div className="min-w-0">
        <div className="truncate text-sm font-medium">{label}</div>
        <div className="truncate text-xs text-[var(--color-fg-muted)]">{detail}</div>
      </div>
    </div>
  );
}

function samePrincipal(a?: Principal, b?: Principal) {
  return !!a && !!b && a.type === b.type && a.id === b.id;
}

function principalLabel(p?: Principal) {
  if (!p) return "Unknown owner";
  if (p.name) return p.name;
  if (p.email) return p.email;
  if (p.type === "service_account") return p.id || "Service account";
  return p.id || "Unknown owner";
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

function UploadBackupItem() {
  return (
    <UploadBackupDialog
      trigger={
        <DropdownItem onSelect={(e) => e.preventDefault()}>
          <Upload className="h-4 w-4" /> Upload HTML (backup)
        </DropdownItem>
      }
    />
  );
}

function UploadBackupDialog({ trigger }: { trigger: React.ReactNode }) {
  const q = useQueryClient();
  const nav = useNavigate();
  const [title, setTitle] = useState("");
  const { file, setFile, reset } = useStagedFile();
  const [open, setOpen] = useState(false);
  const m = useMutation({
    mutationFn: () => api.createDocument(title || file?.name || "Untitled", file!),
    onSuccess: (r) => {
      q.invalidateQueries({ queryKey: queryKeys.documents() });
      toast.success("Document uploaded.");
      setOpen(false);
      reset();
      nav(`/app/d/${r.id}`);
    },
    onError: (e) => toast.error(errorMessage(e, "Upload failed")),
  });
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>{trigger}</DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Upload HTML (backup)</DialogTitle>
          <DialogDescription>
            Agents normally publish via the CLI. Use this only as a manual fallback.
          </DialogDescription>
        </DialogHeader>
        <form
          className="space-y-3"
          onSubmit={(e) => {
            e.preventDefault();
            if (file) m.mutate();
          }}
        >
          <Input
            placeholder="Title"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
          />
          <HtmlFileInput onFile={setFile} />
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={() => setOpen(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={!file || m.isPending}>
              {m.isPending ? "Uploading…" : "Upload"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
