import { useTitle } from "@/lib/use-title";
import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useOnboarding, autoMarkFirstDoc } from "@/lib/onboarding";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "react-hot-toast";
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
import { api, docID, docTitle, type Document, type Folder, type FolderEntry, type Principal } from "@/api";
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
export const viewModeStore = createPersistentStore<ViewMode>("aidocs:view-mode", "gallery");

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
  const folderEntries = (rootFolders.data?.items || [])
    .filter((e): e is FolderEntry & { folder: Folder } => e.type === "folder" && !!e.folder);
  const folders = folderEntries.map((e) => e.folder);

  const pinnedIds = useMemo(() => new Set(pinnedItems.map((p) => p.resource_id)), [pinnedItems]);

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

  const [activeDrag, setActiveDrag] = useState<{ type: string; id: string; title: string } | null>(null);
  const [accessWarning, setAccessWarning] = useState<string | null>(null);
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 8 } }));
  const moveMutation = useMutation({
    mutationFn: async ({ dragType, dragId, targetFolderId }: { dragType: string; dragId: string; targetFolderId: string }) => {
      if (dragType === "folder") {
        return api.updateFolder(dragId, undefined, targetFolderId);
      } else {
        return api.moveDocumentToFolder(targetFolderId, dragId);
      }
    },
    onSuccess: (data) => {
      qc.invalidateQueries({ queryKey: ["folders"] });
      qc.invalidateQueries({ queryKey: queryKeys.documents() });
      const warning = (data as Record<string, unknown>)?.permission_warning as { message?: string } | undefined;
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
    const title = type === "folder"
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
      <div style={{ maxWidth: "1152px", margin: "0 auto", paddingLeft: "24px", paddingRight: "24px", paddingTop: "40px", paddingBottom: "40px" }}>
        <Skeleton style={{ marginBottom: "12px", height: "36px", width: "192px" }} />
        <Skeleton style={{ height: "256px", width: "100%" }} />
      </div>
    );
  }
  if (docs.error) return <Center>Could not load documents.</Center>;

  const searching = debouncedQ.length > 0;
  const empty = !searching && documents.length === 0 && folders.length === 0;

  return (
    <div style={{ maxWidth: "1152px", margin: "0 auto", paddingLeft: "24px", paddingRight: "24px", paddingTop: "40px", paddingBottom: "40px" }}>
      <div style={{ marginBottom: "24px", display: "flex", alignItems: "flex-end", justifyContent: "space-between", gap: "16px" }}>
        <div>
          <h1 style={{ fontSize: "24px", fontWeight: 600, letterSpacing: "-0.02em" }}>Documents</h1>
          <p style={{ marginTop: "4px", fontSize: "14px", color: "#71717a" }}>
            Documents your agents have published.
          </p>
        </div>
        <div style={{ display: "flex", alignItems: "center", gap: "8px" }}>
          <div style={{ position: "relative" }}>
            <Search style={{ pointerEvents: "none", position: "absolute", left: "10px", top: "50%", height: "16px", width: "16px", transform: "translateY(-50%)", color: "#71717a" }} />
            <Input
              value={q}
              onChange={(e) => setQ(e.target.value)}
              placeholder="Search…"
            />
          </div>
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
          <Dropdown>
            <DropdownTrigger asChild>
              <Button variant="outline" size="icon" aria-label="More">
                <MoreHorizontal style={{ height: "16px", width: "16px" }} />
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
        <DndContext sensors={sensors} onDragStart={handleDragStart} onDragEnd={handleDragEnd}>
          {viewMode === "list" ? (
            <UnifiedTable entries={merged} principal={me.data?.principal} pinnedIds={pinnedIds} />
          ) : (
            <UnifiedGrid entries={merged} pinnedIds={pinnedIds} />
          )}
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
      )}

      <Dialog open={newFolderOpen} onOpenChange={setNewFolderOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>New folder</DialogTitle>
          </DialogHeader>
          <form onSubmit={(e) => { e.preventDefault(); if (newFolderTitle.trim()) createFolder.mutate(); }}>
            <Input placeholder="Folder name" value={newFolderTitle} onChange={(e) => setNewFolderTitle(e.target.value)} autoFocus />
            <DialogFooter style={{ marginTop: "16px" }}>
              <Button type="button" variant="ghost" onClick={() => setNewFolderOpen(false)}>Cancel</Button>
              <Button type="submit" disabled={!newFolderTitle.trim() || createFolder.isPending}>Create</Button>
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
  );
}

export function UnifiedGrid({ entries, pinnedIds }: { entries: FolderEntry[]; pinnedIds?: Set<string> }) {
  const me = useQuery({ queryKey: queryKeys.me(), queryFn: api.me });
  const qc = useQueryClient();
  return (
    <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: "12px" }}>
      {entries.map((e) => {
        if (e.type === "folder" && e.folder) {
          const canDelete = samePrincipal(me.data?.principal, e.folder.owner);
          return <DraggableFolderCard key={e.folder.id} folder={e.folder} canDelete={canDelete} qc={qc} pinned={pinnedIds?.has(e.folder.id)} />;
        }
        if (e.type === "document" && e.document) {
          const canDelete = samePrincipal(me.data?.principal, e.document.owner);
          return <DraggableDocCard key={docID(e.document)} doc={e.document} canDelete={canDelete} qc={qc} pinned={pinnedIds?.has(docID(e.document))} />;
        }
        return null;
      })}
    </div>
  );
}

function DraggableFolderCard({ folder: f, canDelete, qc, pinned }: { folder: Folder; canDelete: boolean; qc: ReturnType<typeof useQueryClient>; pinned?: boolean }) {
  const { attributes, listeners, setNodeRef: dragRef, isDragging } = useDraggable({ id: `folder:${f.id}` });
  const { setNodeRef: dropRef, isOver } = useDroppable({ id: `drop:${f.id}` });

  return (
    <div
      ref={(node) => { dragRef(node); dropRef(node); }}
      {...listeners}
      {...attributes}
      style={{
        position: "relative",
        overflow: "hidden",
        borderRadius: "8px",
        border: `1px solid ${isOver ? "#2028ff" : "#e4e4e7"}`,
        background: "#fff",
        transition: "background-color 0.15s",
        ...(isOver ? { outline: "2px solid rgba(32,40,255,0.3)" } : {}),
        ...(isDragging ? { opacity: 0.4 } : {}),
      }}
    >
      <Link to={`/app/folders/${f.id}`} style={{ position: "relative", display: "flex", height: "144px", cursor: "default", flexDirection: "column", alignItems: "center", justifyContent: "center", padding: "0 12px 32px" }} draggable={false}>
        <FolderIcon style={{ height: "40px", width: "40px", color: "rgba(32,40,255,0.6)" }} />
        <div style={{ position: "absolute", left: "12px", right: "12px", bottom: "10px" }}>
          <div style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", fontSize: "14px", fontWeight: 500 }}>{f.title}</div>
          <div style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", fontSize: "12px", color: "#71717a" }}>
            {principalLabel(f.owner)}
          </div>
        </div>
      </Link>
      <GridCardActions resourceType="folder" resourceId={f.id} canDelete={canDelete} title={f.title} qc={qc} pinned={pinned} />
    </div>
  );
}

function DraggableDocCard({ doc: d, canDelete, qc, pinned }: { doc: Document; canDelete: boolean; qc: ReturnType<typeof useQueryClient>; pinned?: boolean }) {
  const { attributes, listeners, setNodeRef, isDragging } = useDraggable({ id: `doc:${docID(d)}` });

  return (
    <div
      ref={setNodeRef}
      {...listeners}
      {...attributes}
      style={{
        position: "relative",
        overflow: "hidden",
        borderRadius: "8px",
        border: "1px solid #e4e4e7",
        background: "#fff",
        transition: "background-color 0.15s",
        ...(isDragging ? { opacity: 0.4 } : {}),
      }}
    >
      <Link to={`/app/d/${docID(d)}`} style={{ position: "relative", display: "flex", height: "144px", cursor: "default", flexDirection: "column", alignItems: "center", justifyContent: "center", padding: "0 12px 32px" }} draggable={false}>
        <FileText style={{ height: "40px", width: "40px", color: "rgba(113,113,122,0.3)" }} />
        <div style={{ position: "absolute", left: "12px", right: "12px", bottom: "10px" }}>
          <div style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", fontSize: "14px", fontWeight: 500 }}>{docTitle(d)}</div>
          <div style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", fontSize: "12px", color: "#71717a" }}>
            {principalLabel(d.owner)}
          </div>
        </div>
      </Link>
      <GridCardActions resourceType="document" resourceId={docID(d)} canDelete={canDelete} title={docTitle(d)} qc={qc} pinned={pinned} />
    </div>
  );
}

export function UnifiedTable({ entries, principal, pinnedIds }: { entries: FolderEntry[]; principal?: Principal; pinnedIds?: Set<string> }) {
  return (
    <div style={{ overflow: "hidden", borderRadius: "12px", border: "1px solid #e4e4e7", background: "#fff" }}>
      <table style={{ width: "100%", fontSize: "14px" }}>
        <thead style={{ background: "rgba(244,244,245,0.5)", fontSize: "11px", textTransform: "uppercase", letterSpacing: "0.05em", color: "#71717a" }}>
          <tr>
            <th style={{ padding: "10px 16px", textAlign: "left", fontWeight: 500 }}>Name</th>
            <th style={{ padding: "10px 16px", textAlign: "left", fontWeight: 500 }}>Owner</th>
            <th style={{ width: "48px", padding: "10px 8px" }}></th>
          </tr>
        </thead>
        <tbody>
          {entries.map((e) => {
            if (e.type === "folder" && e.folder) {
              return <DraggableFolderRow key={e.folder.id} folder={e.folder} canDelete={samePrincipal(principal, e.folder.owner)} pinned={pinnedIds?.has(e.folder.id)} />;
            }
            if (e.type === "document" && e.document) {
              return <DraggableDocRow key={docID(e.document)} doc={e.document} canDelete={samePrincipal(principal, e.document.owner)} pinned={pinnedIds?.has(docID(e.document))} />;
            }
            return null;
          })}
        </tbody>
      </table>
    </div>
  );
}

function DraggableFolderRow({ folder: f, canDelete, pinned }: { folder: Folder; canDelete: boolean; pinned?: boolean }) {
  const { attributes, listeners, setNodeRef: dragRef, isDragging } = useDraggable({ id: `folder:${f.id}` });
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
      ref={(node) => { dragRef(node); dropRef(node); }}
      {...listeners}
      {...attributes}
      style={{
        borderTop: `1px solid ${isOver ? "#2028ff" : "#e4e4e7"}`,
        transition: "background-color 0.15s",
        ...(isOver ? { background: "rgba(32,40,255,0.1)" } : {}),
        ...(isDragging ? { opacity: 0.4 } : {}),
      }}
    >
      <td style={{ padding: "12px 16px" }}>
        <Link to={`/app/folders/${f.id}`} style={{ display: "flex", cursor: "default", alignItems: "center", gap: "8px", fontWeight: 500 }} draggable={false}>
          <FolderIcon style={{ height: "16px", width: "16px", color: "#2028ff" }} />
          {f.title}
        </Link>
      </td>
      <td style={{ padding: "12px 16px" }}><OwnerCell owner={f.owner} /></td>
      <td style={{ padding: "12px 8px" }}>
        <div style={{ display: "flex", gap: "2px" }} onClick={(e) => e.stopPropagation()} onPointerDown={(e) => e.stopPropagation()}>
          <RowPinButton resourceType="folder" resourceId={f.id} pinned={pinned} />
          {canDelete && (
            <DeleteDocumentDialog
              title={f.title}
              description="This deletes the folder and its subfolders. Documents inside them will move to root."
              onConfirm={() => del.mutate()}
              trigger={
                <Button variant="ghost" size="icon" aria-label="Delete folder">
                  <Trash2 style={{ height: "14px", width: "14px", color: "#dc2626" }} />
                </Button>
              }
            />
          )}
        </div>
      </td>
    </tr>
  );
}

function DraggableDocRow({ doc, canDelete, pinned }: { doc: Document; canDelete: boolean; pinned?: boolean }) {
  const { attributes, listeners, setNodeRef, isDragging } = useDraggable({ id: `doc:${docID(doc)}` });
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
      style={{
        borderTop: "1px solid #e4e4e7",
        transition: "background-color 0.15s",
        ...(isDragging ? { opacity: 0.4 } : {}),
      }}
    >
      <td style={{ padding: "12px 16px" }}>
        <Link
          to={`/app/d/${docID(doc)}`}
          style={{ display: "flex", cursor: "default", alignItems: "center", gap: "8px", fontWeight: 500 }}
          draggable={false}
        >
          <FileText style={{ height: "16px", width: "16px", color: "#71717a" }} />
          {docTitle(doc)}
        </Link>
      </td>
      <td style={{ padding: "12px 16px" }}>
        <OwnerCell owner={doc.owner} />
      </td>
      <td style={{ padding: "12px 8px" }}>
        <div style={{ display: "flex", gap: "2px" }} onClick={(e) => e.stopPropagation()} onPointerDown={(e) => e.stopPropagation()}>
          <RowPinButton resourceType="document" resourceId={docID(doc)} pinned={pinned} />
          {canDelete && (
            <DeleteDocumentDialog
              title={docTitle(doc)}
              onConfirm={() => del.mutate()}
              trigger={
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label="Delete document"
                >
                  <Trash2 style={{ height: "14px", width: "14px", color: "#dc2626" }} />
                </Button>
              }
            />
          )}
        </div>
      </td>
    </tr>
  );
}

function RowPinButton({ resourceType, resourceId, pinned }: { resourceType: "document" | "folder"; resourceId: string; pinned?: boolean }) {
  const qc = useQueryClient();
  const pins = useQuery({ queryKey: queryKeys.pins(), queryFn: api.listPins });
  const isPinned = pinned ?? (pins.data?.items || []).some(
    (p) => p.resource_type === resourceType && p.resource_id === resourceId,
  );
  const togglePin = useMutation({
    mutationFn: () => isPinned ? api.deletePin(resourceType, resourceId) : api.putPin(resourceType, resourceId),
    onSuccess: () => { qc.invalidateQueries({ queryKey: queryKeys.pins() }); toast.success(isPinned ? "Unpinned." : "Pinned."); },
    onError: (e) => toast.error(errorMessage(e)),
  });
  return (
    <Button
      variant="ghost"
      size="icon"
      aria-label={isPinned ? "Unpin" : "Pin"}
      onClick={(e) => { e.preventDefault(); e.stopPropagation(); togglePin.mutate(); }}
    >
      {isPinned ? <PinOff style={{ height: "14px", width: "14px" }} /> : <Pin style={{ height: "14px", width: "14px" }} />}
    </Button>
  );
}

function GridCardActions({ resourceType, resourceId, canDelete, title, qc, pinned }: { resourceType: "document" | "folder"; resourceId: string; canDelete: boolean; title: string; qc: ReturnType<typeof useQueryClient>; pinned?: boolean }) {
  const pins = useQuery({ queryKey: queryKeys.pins(), queryFn: api.listPins });
  const isPinned = pinned ?? (pins.data?.items || []).some(
    (p) => p.resource_type === resourceType && p.resource_id === resourceId,
  );
  const togglePin = useMutation({
    mutationFn: () => isPinned ? api.deletePin(resourceType, resourceId) : api.putPin(resourceType, resourceId),
    onSuccess: () => { qc.invalidateQueries({ queryKey: queryKeys.pins() }); toast.success(isPinned ? "Unpinned." : "Pinned."); },
    onError: (e) => toast.error(errorMessage(e)),
  });
  const del = useMutation({
    mutationFn: () => resourceType === "folder" ? api.deleteFolder(resourceId) : api.deleteDocument(resourceId),
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
      style={{ position: "absolute", right: "8px", top: "8px", display: "flex", gap: "2px" }}
      onClick={(e) => e.stopPropagation()}
      onPointerDown={(e) => e.stopPropagation()}
    >
      <Button variant="ghost" size="icon" aria-label={isPinned ? "Unpin" : "Pin"} onClick={(e) => { e.preventDefault(); e.stopPropagation(); togglePin.mutate(); }}>
        {isPinned ? <PinOff style={{ height: "14px", width: "14px" }} /> : <Pin style={{ height: "14px", width: "14px" }} />}
      </Button>
      {canDelete && (
        <DeleteDocumentDialog
          title={title}
          description={resourceType === "folder" ? "This deletes the folder and its subfolders. Documents inside them will move to root." : undefined}
          onConfirm={() => del.mutate()}
          trigger={
            <Button variant="ghost" size="icon" aria-label="Delete" onClick={(e) => e.stopPropagation()}>
              <Trash2 style={{ height: "14px", width: "14px", color: "#dc2626" }} />
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
    <div style={{ display: "flex", minWidth: 0, alignItems: "center", gap: "8px" }}>
      <Avatar name={label} src={owner?.picture_url} size={28} />
      <div style={{ minWidth: 0 }}>
        <div style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", fontSize: "14px", fontWeight: 500 }}>{label}</div>
        <div style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", fontSize: "12px", color: "#71717a" }}>{detail}</div>
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
    <div style={{ maxWidth: "576px", margin: "0 auto", display: "flex", flexDirection: "column", gap: "16px" }}>
      <div style={{ borderRadius: "14px", border: "1px solid #e4e4e7", background: "#fff", padding: "24px" }}>
        <div style={{ marginBottom: "12px", display: "flex", height: "40px", width: "40px", alignItems: "center", justifyContent: "center", borderRadius: "6px", background: "#eef0ff", color: "#2028ff" }}>
          <Sparkles style={{ height: "20px", width: "20px" }} />
        </div>
        <h3 style={{ fontSize: "16px", fontWeight: 600 }}>Set up your agent</h3>
        <p style={{ marginTop: "4px", fontSize: "14px", color: "#71717a" }}>
          Documents are published by your AI agent. Walk through the three-step setup so
          it knows how to use aidocs.
        </p>
        <div style={{ marginTop: "16px" }}>
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
          <Upload style={{ height: "16px", width: "16px" }} /> Upload HTML (backup)
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
          style={{ display: "flex", flexDirection: "column", gap: "12px" }}
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
