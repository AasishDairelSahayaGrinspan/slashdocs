import { useMemo, useState } from "react";
import { Link, useParams, useNavigate } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { errorMessage } from "@/lib/errors";
import { useTitle } from "@/lib/use-title";
import {
  ChevronRight,
  Folder as FolderIcon,
  MoreHorizontal,
  Pencil,
  Plus,
  Share2,
  Trash2,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Center, Skeleton, EmptyState } from "@/components/ui/misc";
import {
  Dropdown,
  DropdownContent,
  DropdownItem,
  DropdownTrigger,
} from "@/components/ui/dropdown";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { api, docID, type FolderEntry } from "@/api";
import { canEdit, canManageSharing } from "@/lib/constants";
import { queryKeys } from "@/lib/queryKeys";
import { samePrincipal } from "@/lib/principals";
import { viewModeStore } from "@/lib/view-mode";
import { ShareSheet } from "@/pages/doc-share-sheet";
import { Sheet, SheetTrigger } from "@/components/ui/sheet";
import { DRAG_TYPE, type ActiveDrag } from "@/components/browse/drag";
import { BrowseDragOverlay } from "@/components/browse/drag-overlay";
import { UnifiedGrid } from "@/components/browse/unified-grid";
import { UnifiedTable } from "@/components/browse/unified-table";
import { ViewModeToggle } from "@/components/browse/view-mode-toggle";
import { AccessWarningDialog, NewFolderDialog } from "@/components/browse/dialogs";
import {
  UploadBackupItem,
  type CreateDocumentResult,
} from "@/components/browse/upload-backup";
import {
  DndContext,
  PointerSensor,
  useSensor,
  useSensors,
  useDroppable,
  type DragEndEvent,
  type DragStartEvent,
} from "@dnd-kit/core";

export function FolderPage() {
  const { id = "" } = useParams();
  const nav = useNavigate();
  const qc = useQueryClient();

  const folder = useQuery({
    queryKey: queryKeys.folder(id),
    queryFn: () => api.getFolder(id),
    enabled: !!id,
  });
  const contents = useQuery({
    queryKey: queryKeys.folders(id),
    queryFn: () => api.listFolders(id),
    enabled: !!id,
  });
  const pins = useQuery({
    queryKey: queryKeys.pins(),
    queryFn: api.listPins,
  });
  const me = useQuery({ queryKey: queryKeys.me(), queryFn: api.me });
  const isOwner = samePrincipal(me.data?.principal, folder.data?.owner);
  const access = useQuery({
    queryKey: queryKeys.folderAccess(id),
    queryFn: () => api.folderAccess(id),
    enabled: !!id && !!me.data?.principal && !isOwner,
    staleTime: 30_000,
  });
  const currentAccessRole = access.data?.items.find((item) =>
    samePrincipal(item.principal, me.data?.principal),
  )?.effective_role;
  const canManageAccess = isOwner || canManageSharing(currentAccessRole);
  // Uploading into this folder is a write to the folder; the server requires
  // editor, so don't offer the action to viewers and commenters.
  const canUploadHere = isOwner || canEdit(currentAccessRole);

  const viewMode = viewModeStore.useStore();
  useTitle(folder.data?.title || "Folder");

  const pinnedIds = useMemo(
    () => new Set((pins.data?.items || []).map((p) => p.resource_id)),
    [pins.data],
  );

  const [newFolderOpen, setNewFolderOpen] = useState(false);
  const [renameOpen, setRenameOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [renameTitle, setRenameTitle] = useState("");

  const delFolder = useMutation({
    mutationFn: () => api.deleteFolder(id),
    onSuccess: () => {
      setDeleteOpen(false);
      toast.success("Deleted folder.");
      qc.invalidateQueries({ queryKey: queryKeys.foldersAll() });
      qc.invalidateQueries({ queryKey: queryKeys.documents() });
      qc.invalidateQueries({ queryKey: queryKeys.pins() });
      qc.removeQueries({ queryKey: queryKeys.folder(id) });
      nav("/app/documents");
    },
    onError: (e) => toast.error(errorMessage(e, "Delete failed")),
  });

  const renameFolder = useMutation({
    mutationFn: () => api.updateFolder(id, renameTitle),
    onSuccess: () => {
      setRenameOpen(false);
      qc.invalidateQueries({ queryKey: queryKeys.folder(id) });
      toast.success("Renamed.");
    },
    onError: (e) => toast.error(errorMessage(e, "Rename failed")),
  });

  const [activeDrag, setActiveDrag] = useState<ActiveDrag | null>(null);
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 8 } }),
  );
  const [accessWarning, setAccessWarning] = useState<string | null>(null);

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
      const isDoc = dragType === DRAG_TYPE.document;
      if (targetFolderId === "__root__") {
        if (isDoc) {
          return api.moveDocumentToRoot(id, dragId);
        } else {
          return api.updateFolder(dragId, undefined, null);
        }
      }
      if (!isDoc) {
        return api.updateFolder(dragId, undefined, targetFolderId);
      } else {
        return api.moveDocumentToFolder(targetFolderId, dragId);
      }
    },
    onSuccess: (data) => {
      qc.invalidateQueries({ queryKey: queryKeys.foldersAll() });
      qc.invalidateQueries({ queryKey: queryKeys.documents() });
      qc.invalidateQueries({ queryKey: queryKeys.pins() });
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

  // Surface a folder-grant warning in the same dialog a drag-in uses; let the
  // dialog fall back to its own toast otherwise.
  function handleUploaded(r: CreateDocumentResult) {
    qc.invalidateQueries({ queryKey: queryKeys.folders(id) });
    if (r.permission_warning?.message) {
      setAccessWarning(r.permission_warning.message);
      return true;
    }
    return false;
  }

  if (folder.isLoading || contents.isLoading) {
    return (
      <div className="mx-auto max-w-6xl px-4 py-6 sm:px-6 sm:py-10">
        <Skeleton className="mb-3 h-9 w-48" />
        <Skeleton className="h-64 w-full" />
      </div>
    );
  }
  if (folder.error || contents.error) return <Center>Could not load folder.</Center>;

  const f = folder.data!;
  const items = contents.data?.items || [];
  const breadcrumbs = f.ancestors || [];

  function handleDragStart(event: DragStartEvent) {
    const raw = event.active.id as string;
    const [type, did] = raw.split(":", 2);
    const entry = items.find((e) =>
      type === DRAG_TYPE.folder ? e.folder?.id === did : e.document?.id === did,
    );
    const title =
      type === DRAG_TYPE.folder
        ? entry?.folder?.title || "Folder"
        : entry?.document?.title || "Document";
    setActiveDrag({ type, id: did, title });
  }

  function handleDragEnd(event: DragEndEvent) {
    setActiveDrag(null);
    const { active, over } = event;
    if (!over) return;
    const [dragType, dragId] = (active.id as string).split(":", 2);
    const targetFolderId = (over.id as string).replace("drop:", "");
    if (dragType === DRAG_TYPE.folder && dragId === targetFolderId) return;
    if (targetFolderId === id) return;
    moveMutation.mutate({ dragType, dragId, targetFolderId });
  }

  return (
    <DndContext
      sensors={sensors}
      onDragStart={handleDragStart}
      onDragEnd={handleDragEnd}
    >
      <div className="mx-auto max-w-6xl px-4 py-6 sm:px-6 sm:py-10">
        {/* Breadcrumbs */}
        <nav className="mb-4 flex items-center gap-1 text-sm text-[var(--color-fg-muted)]">
          <DroppableBreadcrumb
            dropId="drop:__root__"
            to="/app/documents"
            label="Documents"
            isDragging={!!activeDrag}
          />
          {breadcrumbs.map((b) => (
            <span key={b.id} className="flex items-center gap-1">
              <ChevronRight className="h-3 w-3" />
              <DroppableBreadcrumb
                dropId={`drop:${b.id}`}
                to={`/app/folders/${b.id}`}
                label={b.title}
                isDragging={!!activeDrag}
              />
            </span>
          ))}
          <ChevronRight className="h-3 w-3" />
          <span className="font-medium text-[var(--color-fg)]">{f.title}</span>
        </nav>

        {/* Header */}
        <div className="mb-6 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex min-w-0 items-center gap-3">
            <FolderIcon className="h-6 w-6 shrink-0 text-[var(--color-accent)]" />
            <div className="min-w-0">
              <h1 className="truncate text-2xl font-semibold tracking-tight">
                {f.title}
              </h1>
              <p className="truncate text-sm text-[var(--color-fg-muted)]">
                {f.owner?.name || f.owner?.email || ""}
              </p>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <ViewModeToggle />
            <Button
              variant="outline"
              size="md"
              onClick={() => setNewFolderOpen(true)}
              className="shrink-0 px-3 sm:px-4"
            >
              <Plus className="h-3.5 w-3.5 sm:mr-1.5" />
              <span className="hidden sm:inline">New folder</span>
            </Button>
            <Sheet>
              <SheetTrigger asChild>
                <Button variant="outline" size="sm" className="shrink-0 px-2.5 sm:px-3">
                  <Share2 className="h-3.5 w-3.5 sm:mr-1.5" />
                  <span className="hidden sm:inline">Share</span>
                </Button>
              </SheetTrigger>
              <ShareSheet
                resourceType="folder"
                resourceId={id}
                canManageAccess={canManageAccess}
              />
            </Sheet>
            <Dropdown>
              <DropdownTrigger asChild>
                <Button variant="ghost" size="icon" aria-label="More">
                  <MoreHorizontal className="h-4 w-4" />
                </Button>
              </DropdownTrigger>
              <DropdownContent>
                <DropdownItem
                  onSelect={() => {
                    setRenameTitle(f.title);
                    setRenameOpen(true);
                  }}
                >
                  <Pencil className="h-4 w-4" /> Rename
                </DropdownItem>
                <DropdownItem onSelect={() => setNewFolderOpen(true)}>
                  <Plus className="h-4 w-4" /> New subfolder
                </DropdownItem>
                {canUploadHere && (
                  <UploadBackupItem folderId={id} onUploaded={handleUploaded} />
                )}
                <DropdownItem
                  onSelect={() => window.setTimeout(() => setDeleteOpen(true), 0)}
                  className="text-[var(--color-danger)]"
                >
                  <Trash2 className="h-4 w-4" /> Delete folder
                </DropdownItem>
              </DropdownContent>
            </Dropdown>
          </div>
        </div>

        <AlertDialog open={deleteOpen} onOpenChange={setDeleteOpen}>
          <AlertDialogContent>
            <AlertDialogTitle>Delete "{f.title}"?</AlertDialogTitle>
            <AlertDialogDescription>
              This deletes the folder and its subfolders. Documents inside them will
              move to root.
            </AlertDialogDescription>
            <AlertDialogFooter>
              <AlertDialogCancel>Cancel</AlertDialogCancel>
              <AlertDialogAction
                variant="danger"
                onClick={(e) => {
                  e.preventDefault();
                  delFolder.mutate();
                }}
              >
                Delete
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>

        {/* Contents */}
        {items.length === 0 ? (
          <EmptyState
            title="Empty folder"
            description="Add documents or create subfolders."
          />
        ) : (
          <FolderContents items={items} pinnedIds={pinnedIds} />
        )}

        {/* New subfolder dialog */}
        <NewFolderDialog
          open={newFolderOpen}
          onOpenChange={setNewFolderOpen}
          parentFolderId={id}
        />

        {/* Rename dialog */}
        <Dialog open={renameOpen} onOpenChange={setRenameOpen}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Rename folder</DialogTitle>
            </DialogHeader>
            <form
              onSubmit={(e) => {
                e.preventDefault();
                if (renameTitle.trim()) renameFolder.mutate();
              }}
            >
              <Input
                value={renameTitle}
                onChange={(e) => setRenameTitle(e.target.value)}
                autoFocus
              />
              <DialogFooter className="mt-4">
                <Button
                  type="button"
                  variant="ghost"
                  onClick={() => setRenameOpen(false)}
                >
                  Cancel
                </Button>
                <Button
                  type="submit"
                  disabled={!renameTitle.trim() || renameFolder.isPending}
                >
                  Rename
                </Button>
              </DialogFooter>
            </form>
          </DialogContent>
        </Dialog>

        <AccessWarningDialog
          warning={accessWarning}
          onClose={() => setAccessWarning(null)}
        />
      </div>
      <BrowseDragOverlay activeDrag={activeDrag} viewMode={viewMode} />
    </DndContext>
  );
}

function FolderContents({
  items,
  pinnedIds,
}: {
  items: FolderEntry[];
  pinnedIds: Set<string>;
}) {
  const me = useQuery({ queryKey: queryKeys.me(), queryFn: api.me });
  const viewMode = viewModeStore.useStore();

  const entries = [...items].sort((a, b) => {
    const aid = a.folder?.id || (a.document ? docID(a.document) : "");
    const bid = b.folder?.id || (b.document ? docID(b.document) : "");
    const ap = pinnedIds.has(aid);
    const bp = pinnedIds.has(bid);
    if (ap !== bp) return ap ? -1 : 1;
    const ta = a.last_active || "";
    const tb = b.last_active || "";
    if (ta && tb) return tb.localeCompare(ta);
    if (tb) return 1;
    if (ta) return -1;
    return (a.folder?.title || a.document?.title || "").localeCompare(
      b.folder?.title || b.document?.title || "",
    );
  });

  return viewMode === "list" ? (
    <UnifiedTable
      entries={entries}
      principal={me.data?.principal}
      pinnedIds={pinnedIds}
    />
  ) : (
    <UnifiedGrid entries={entries} pinnedIds={pinnedIds} />
  );
}

function DroppableBreadcrumb({
  dropId,
  to,
  label,
  isDragging,
}: {
  dropId: string;
  to: string;
  label: string;
  isDragging: boolean;
}) {
  const { setNodeRef, isOver } = useDroppable({ id: dropId });
  return (
    <Link
      ref={setNodeRef}
      to={to}
      className={`rounded px-1.5 py-0.5 transition-colors hover:underline ${
        isDragging ? "ring-1 ring-[var(--color-border)] ring-dashed" : ""
      } ${isOver ? "bg-[var(--color-accent)]/15 ring-2 ring-[var(--color-accent)] font-medium text-[var(--color-accent)]" : ""}`}
    >
      {label}
    </Link>
  );
}
