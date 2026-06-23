import { useMemo, useState } from "react";
import { Link, useParams, useNavigate } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { errorMessage } from "@/lib/errors";
import { useTitle } from "@/lib/use-title";
import {
  ChevronRight,
  FileText,
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
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  api,
  docID,
  type Folder,
  type FolderEntry,
  type Document,
} from "@/api";
import { queryKeys } from "@/lib/queryKeys";
import { ShareSheet } from "@/pages/doc-share-sheet";
import { Sheet, SheetTrigger } from "@/components/ui/sheet";
import { LayoutGrid, List } from "lucide-react";
import { UnifiedGrid, UnifiedTable, viewModeStore } from "@/pages/documents";
import {
  DndContext,
  DragOverlay,
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

  const viewMode = viewModeStore.useStore();
  useTitle(folder.data?.title || "Folder");

  const pinnedIds = useMemo(() => new Set((pins.data?.items || []).map((p) => p.resource_id)), [pins.data]);

  const [newFolderOpen, setNewFolderOpen] = useState(false);
  const [newFolderTitle, setNewFolderTitle] = useState("");
  const [renameOpen, setRenameOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [renameTitle, setRenameTitle] = useState("");

  const delFolder = useMutation({
    mutationFn: () => api.deleteFolder(id),
    onSuccess: () => {
      setDeleteOpen(false);
      toast.success("Deleted folder.");
      qc.invalidateQueries({ queryKey: queryKeys.folders() });
      nav("/app/documents");
    },
    onError: (e) => toast.error(errorMessage(e, "Delete failed")),
  });

  const createSub = useMutation({
    mutationFn: () => api.createFolder(newFolderTitle, id),
    onSuccess: () => {
      setNewFolderOpen(false);
      setNewFolderTitle("");
      qc.invalidateQueries({ queryKey: queryKeys.folders(id) });
      toast.success("Created subfolder.");
    },
    onError: (e) => toast.error(errorMessage(e, "Create failed")),
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

  const [activeDrag, setActiveDrag] = useState<{ type: string; id: string; title: string } | null>(null);
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 8 } }));
  const [accessWarning, setAccessWarning] = useState<string | null>(null);

  const moveMutation = useMutation({
    mutationFn: async ({ dragType, dragId, targetFolderId }: { dragType: string; dragId: string; targetFolderId: string }) => {
      const isDoc = dragType === "doc" || dragType === "document";
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
      qc.invalidateQueries({ queryKey: ["folders"] });
      qc.invalidateQueries({ queryKey: queryKeys.folders() });
      qc.invalidateQueries({ queryKey: queryKeys.documents() });
      qc.invalidateQueries({ queryKey: queryKeys.pins() });
      const warning = (data as Record<string, unknown>)?.permission_warning as { message?: string } | undefined;
      if (warning?.message) {
        setAccessWarning(warning.message);
      } else {
        toast.success("Moved.");
      }
    },
    onError: (e) => toast.error(errorMessage(e, "Move failed")),
  });

  if (folder.isLoading || contents.isLoading) {
    return (
      <div className="mx-auto max-w-6xl px-6 py-10">
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
      type === "folder" ? e.folder?.id === did : e.document?.id === did,
    );
    const title = type === "folder" ? entry?.folder?.title || "Folder" : entry?.document?.title || "Document";
    setActiveDrag({ type, id: did, title });
  }

  function handleDragEnd(event: DragEndEvent) {
    setActiveDrag(null);
    const { active, over } = event;
    if (!over) return;
    const [dragType, dragId] = (active.id as string).split(":", 2);
    const targetFolderId = (over.id as string).replace("drop:", "");
    if (dragType === "folder" && dragId === targetFolderId) return;
    if (targetFolderId === id) return;
    moveMutation.mutate({ dragType, dragId, targetFolderId });
  }

  return (
    <DndContext sensors={sensors} onDragStart={handleDragStart} onDragEnd={handleDragEnd}>
    <div className="mx-auto max-w-6xl px-6 py-10">
      {/* Breadcrumbs */}
      <nav className="mb-4 flex items-center gap-1 text-sm text-[var(--color-fg-muted)]">
        <DroppableBreadcrumb dropId="drop:__root__" to="/app/documents" label="Documents" isDragging={!!activeDrag} />
        {breadcrumbs.map((b) => (
          <span key={b.id} className="flex items-center gap-1">
            <ChevronRight className="h-3 w-3" />
            <DroppableBreadcrumb dropId={`drop:${b.id}`} to={`/app/folders/${b.id}`} label={b.title} isDragging={!!activeDrag} />
          </span>
        ))}
        <ChevronRight className="h-3 w-3" />
        <span className="font-medium text-[var(--color-fg)]">{f.title}</span>
      </nav>

      {/* Header */}
      <div className="mb-6 flex items-center justify-between">
        <div className="flex items-center gap-3">
          <FolderIcon className="h-6 w-6 text-[var(--color-accent)]" />
          <div>
            <h1 className="text-2xl font-semibold tracking-tight">{f.title}</h1>
            <p className="text-sm text-[var(--color-fg-muted)]">{f.owner?.name || f.owner?.email || ""}</p>
          </div>
        </div>
        <div className="flex items-center gap-2">
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
          <Sheet>
            <SheetTrigger asChild>
              <Button variant="outline" size="sm">
                <Share2 className="mr-1.5 h-3.5 w-3.5" /> Share
              </Button>
            </SheetTrigger>
            <ShareSheet resourceType="folder" resourceId={id} />
          </Sheet>
          <Dropdown>
            <DropdownTrigger asChild>
              <Button variant="ghost" size="icon">
                <MoreHorizontal className="h-4 w-4" />
              </Button>
            </DropdownTrigger>
            <DropdownContent>
              <DropdownItem onSelect={() => { setRenameTitle(f.title); setRenameOpen(true); }}>
                <Pencil className="h-4 w-4" /> Rename
              </DropdownItem>
              <DropdownItem onSelect={() => setNewFolderOpen(true)}>
                <Plus className="h-4 w-4" /> New subfolder
              </DropdownItem>
              <DropdownItem onSelect={() => window.setTimeout(() => setDeleteOpen(true), 0)} className="text-[var(--color-danger)]">
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
            This will delete this folder, its subfolders, and all documents inside them.
          </AlertDialogDescription>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction variant="danger" onClick={(e) => { e.preventDefault(); delFolder.mutate(); }}>
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
      <Dialog open={newFolderOpen} onOpenChange={setNewFolderOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>New subfolder</DialogTitle>
          </DialogHeader>
          <form
            onSubmit={(e) => {
              e.preventDefault();
              if (newFolderTitle.trim()) createSub.mutate();
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
                disabled={!newFolderTitle.trim() || createSub.isPending}
              >
                Create
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

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
              <Button type="button" variant="ghost" onClick={() => setRenameOpen(false)}>
                Cancel
              </Button>
              <Button type="submit" disabled={!renameTitle.trim() || renameFolder.isPending}>
                Rename
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

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
    <DragOverlay dropAnimation={null}>
      {activeDrag && viewMode === "gallery" && (
        <div className="flex h-36 w-48 flex-col items-center justify-center rounded-lg border border-[var(--color-accent)] bg-[var(--color-surface)] opacity-30 shadow-xl">
          {activeDrag.type === "folder" ? <FolderIcon className="h-10 w-10 text-[var(--color-accent)]/60" /> : <FileText className="h-10 w-10 text-[var(--color-fg-muted)]/30" />}
          <div className="mt-2 max-w-[10rem] truncate px-3 text-sm font-medium">{activeDrag.title}</div>
        </div>
      )}
      {activeDrag && viewMode === "list" && (
        <div className="flex w-64 items-center gap-2 rounded-md border border-[var(--color-accent)] bg-[var(--color-surface)] px-3 py-2 opacity-30 shadow-xl">
          {activeDrag.type === "folder" ? <FolderIcon className="h-4 w-4 text-[var(--color-accent)]" /> : <FileText className="h-4 w-4 text-[var(--color-fg-muted)]" />}
          <span className="truncate text-sm font-medium">{activeDrag.title}</span>
        </div>
      )}
    </DragOverlay>
    </DndContext>
  );
}

function FolderContents({ items, pinnedIds }: { items: FolderEntry[]; pinnedIds: Set<string> }) {
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
    return (a.folder?.title || a.document?.title || "").localeCompare(b.folder?.title || b.document?.title || "");
  });

  return viewMode === "list" ? (
    <UnifiedTable entries={entries} principal={me.data?.principal} pinnedIds={pinnedIds} />
  ) : (
    <UnifiedGrid entries={entries} pinnedIds={pinnedIds} />
  );
}

function DroppableBreadcrumb({ dropId, to, label, isDragging }: { dropId: string; to: string; label: string; isDragging: boolean }) {
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

