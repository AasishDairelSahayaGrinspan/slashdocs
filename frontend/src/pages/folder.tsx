import { useMemo, useState } from "react";
import { Link, useParams, useNavigate } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "react-hot-toast";
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
      qc.invalidateQueries({ queryKey: ["folders"] });
      qc.invalidateQueries({ queryKey: queryKeys.documents() });
      qc.invalidateQueries({ queryKey: queryKeys.pins() });
      qc.removeQueries({ queryKey: queryKeys.folder(id) });
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
      <div style={{ maxWidth: "1152px", margin: "0 auto", paddingLeft: "24px", paddingRight: "24px", paddingTop: "40px", paddingBottom: "40px" }}>
        <Skeleton style={{ marginBottom: "12px", height: "36px", width: "192px" }} />
        <Skeleton style={{ height: "256px", width: "100%" }} />
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
    <div style={{ maxWidth: "1152px", margin: "0 auto", paddingLeft: "24px", paddingRight: "24px", paddingTop: "40px", paddingBottom: "40px" }}>
      <nav style={{ marginBottom: "16px", display: "flex", alignItems: "center", gap: "4px", fontSize: "14px", color: "#71717a" }}>
        <DroppableBreadcrumb dropId="drop:__root__" to="/app/documents" label="Documents" isDragging={!!activeDrag} />
        {breadcrumbs.map((b) => (
          <span key={b.id} style={{ display: "flex", alignItems: "center", gap: "4px" }}>
            <ChevronRight style={{ height: "12px", width: "12px" }} />
            <DroppableBreadcrumb dropId={`drop:${b.id}`} to={`/app/folders/${b.id}`} label={b.title} isDragging={!!activeDrag} />
          </span>
        ))}
        <ChevronRight style={{ height: "12px", width: "12px" }} />
        <span style={{ fontWeight: 500, color: "#18181b" }}>{f.title}</span>
      </nav>

      <div style={{ marginBottom: "24px", display: "flex", alignItems: "center", justifyContent: "space-between" }}>
        <div style={{ display: "flex", alignItems: "center", gap: "12px" }}>
          <FolderIcon style={{ height: "24px", width: "24px", color: "#2028ff" }} />
          <div>
            <h1 style={{ fontSize: "24px", fontWeight: 600, letterSpacing: "-0.02em" }}>{f.title}</h1>
            <p style={{ fontSize: "14px", color: "#71717a" }}>{f.owner?.name || f.owner?.email || ""}</p>
          </div>
        </div>
        <div style={{ display: "flex", alignItems: "center", gap: "8px" }}>
          <div style={{ display: "flex", borderRadius: "6px", border: "1px solid #e4e4e7" }}>
            <Button
              variant="ghost"
              size="icon"
              style={{ borderRadius: "6px 0 0 6px", ...(viewMode === "gallery" ? { background: "#f4f4f5" } : {}) }}
              onClick={() => viewModeStore.write("gallery")}
              aria-label="Gallery view"
            >
              <LayoutGrid style={{ height: "16px", width: "16px" }} />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              style={{ borderRadius: "0 6px 6px 0", borderLeft: "1px solid #e4e4e7", ...(viewMode === "list" ? { background: "#f4f4f5" } : {}) }}
              onClick={() => viewModeStore.write("list")}
              aria-label="List view"
            >
              <List style={{ height: "16px", width: "16px" }} />
            </Button>
          </div>
          <Button variant="outline" size="md" onClick={() => setNewFolderOpen(true)}>
            <Plus style={{ marginRight: "6px", height: "14px", width: "14px" }} /> New folder
          </Button>
          <Sheet>
            <SheetTrigger asChild>
              <Button variant="outline" size="sm">
                <Share2 style={{ marginRight: "6px", height: "14px", width: "14px" }} /> Share
              </Button>
            </SheetTrigger>
            <ShareSheet resourceType="folder" resourceId={id} />
          </Sheet>
          <Dropdown>
            <DropdownTrigger asChild>
              <Button variant="ghost" size="icon">
                <MoreHorizontal style={{ height: "16px", width: "16px" }} />
              </Button>
            </DropdownTrigger>
            <DropdownContent>
              <DropdownItem onSelect={() => { setRenameTitle(f.title); setRenameOpen(true); }}>
                <Pencil style={{ height: "16px", width: "16px" }} /> Rename
              </DropdownItem>
              <DropdownItem onSelect={() => setNewFolderOpen(true)}>
                <Plus style={{ height: "16px", width: "16px" }} /> New subfolder
              </DropdownItem>
              <DropdownItem onSelect={() => window.setTimeout(() => setDeleteOpen(true), 0)} style={{ color: "#dc2626" }}>
                <Trash2 style={{ height: "16px", width: "16px" }} /> Delete folder
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

      {items.length === 0 ? (
        <EmptyState
          title="Empty folder"
          description="Add documents or create subfolders."
        />
      ) : (
        <FolderContents items={items} pinnedIds={pinnedIds} />
      )}

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
            <DialogFooter style={{ marginTop: "16px" }}>
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
            <DialogFooter style={{ marginTop: "16px" }}>
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
        <div style={{
          display: "flex",
          height: "144px",
          width: "192px",
          flexDirection: "column",
          alignItems: "center",
          justifyContent: "center",
          borderRadius: "8px",
          border: "1px solid #2028ff",
          background: "#fff",
          opacity: 0.3,
          boxShadow: "0 20px 25px -5px rgba(0,0,0,0.1), 0 8px 10px -6px rgba(0,0,0,0.1)",
        }}>
          {activeDrag.type === "folder" ? <FolderIcon style={{ height: "40px", width: "40px", color: "rgba(32,40,255,0.6)" }} /> : <FileText style={{ height: "40px", width: "40px", color: "rgba(113,113,122,0.3)" }} />}
          <div style={{ marginTop: "8px", maxWidth: "160px", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", padding: "0 12px", fontSize: "14px", fontWeight: 500 }}>{activeDrag.title}</div>
        </div>
      )}
      {activeDrag && viewMode === "list" && (
        <div style={{
          display: "flex",
          width: "256px",
          alignItems: "center",
          gap: "8px",
          borderRadius: "6px",
          border: "1px solid #2028ff",
          background: "#fff",
          padding: "8px 12px",
          opacity: 0.3,
          boxShadow: "0 20px 25px -5px rgba(0,0,0,0.1), 0 8px 10px -6px rgba(0,0,0,0.1)",
        }}>
          {activeDrag.type === "folder" ? <FolderIcon style={{ height: "16px", width: "16px", color: "#2028ff" }} /> : <FileText style={{ height: "16px", width: "16px", color: "#71717a" }} />}
          <span style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", fontSize: "14px", fontWeight: 500 }}>{activeDrag.title}</span>
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
      style={{
        borderRadius: "4px",
        padding: "2px 6px",
        transition: "background-color 0.15s",
        ...(isDragging ? { outline: "1px dashed #e4e4e7" } : {}),
        ...(isOver ? { background: "rgba(32,40,255,0.15)", outline: "2px solid #2028ff", fontWeight: 500, color: "#2028ff" } : {}),
      }}
    >
      {label}
    </Link>
  );
}
