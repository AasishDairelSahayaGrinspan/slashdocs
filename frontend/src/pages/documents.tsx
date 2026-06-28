import { useTitle } from "@/lib/use-title";
import { useEffect, useMemo, useRef, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useOnboarding, autoMarkFirstDoc } from "@/lib/onboarding";
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useVirtualizer } from "@tanstack/react-virtual";
import { toast } from "sonner";
import { errorMessage } from "@/lib/errors";
import { cn } from "@/lib/cn";
import { timeAgo } from "@/lib/time";
import {
  FileText,
  Folder as FolderIcon,
  HardDriveUpload,
  MoreHorizontal,
  Pin,
  PinOff,
  Plus,
  Search,
  Sparkles,
  Terminal,
  Trash2,
  X,
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
import { FileDropZone, useStagedFile } from "@/components/ui/upload";
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

/* ── card color swatches ─────────────────────────────────────────────── */
const DOC_SWATCHES = [
  { strip: "bg-blue-500",    iconBg: "bg-blue-50 dark:bg-blue-950/50",    iconText: "text-blue-600 dark:text-blue-400"    },
  { strip: "bg-violet-500",  iconBg: "bg-violet-50 dark:bg-violet-950/50", iconText: "text-violet-600 dark:text-violet-400" },
  { strip: "bg-emerald-500", iconBg: "bg-emerald-50 dark:bg-emerald-950/50", iconText: "text-emerald-600 dark:text-emerald-400" },
  { strip: "bg-amber-500",   iconBg: "bg-amber-50 dark:bg-amber-950/50",   iconText: "text-amber-600 dark:text-amber-400"   },
  { strip: "bg-rose-500",    iconBg: "bg-rose-50 dark:bg-rose-950/50",    iconText: "text-rose-600 dark:text-rose-400"    },
  { strip: "bg-cyan-500",    iconBg: "bg-cyan-50 dark:bg-cyan-950/50",    iconText: "text-cyan-600 dark:text-cyan-400"    },
  { strip: "bg-orange-500",  iconBg: "bg-orange-50 dark:bg-orange-950/50", iconText: "text-orange-600 dark:text-orange-400" },
  { strip: "bg-teal-500",    iconBg: "bg-teal-50 dark:bg-teal-950/50",   iconText: "text-teal-600 dark:text-teal-400"    },
] as const;

function strHash(s: string): number {
  let h = 0;
  for (let i = 0; i < s.length; i++) h = (Math.imul(31, h) + s.charCodeAt(i)) | 0;
  return Math.abs(h);
}
function getDocSwatch(id: string) {
  return DOC_SWATCHES[strHash(id) % DOC_SWATCHES.length];
}

/* ── card dimensions ────────────────────────────────────────────────── */
const CARD_STRIP = 3;
const CARD_PREVIEW = 165;
const CARD_FOOTER = 52;
const CARD_H = CARD_STRIP + CARD_PREVIEW + CARD_FOOTER; // 220px
const GRID_GAP = 12;
const ROW_H = CARD_H + GRID_GAP; // 232px

/* ── page ────────────────────────────────────────────────────────────── */

export function DocumentsPage() {
  useTitle("Documents");
  const [q, setQ] = useState("");
  const [debouncedQ, setDebouncedQ] = useState("");
  const searchRef = useRef<HTMLInputElement>(null);
  const scrollContainerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const t = window.setTimeout(() => setDebouncedQ(q.trim()), 250);
    return () => window.clearTimeout(t);
  }, [q]);

  // "/" shortcut focuses search
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (
        e.key === "/" &&
        !e.metaKey &&
        !e.ctrlKey &&
        !["INPUT", "TEXTAREA"].includes((e.target as HTMLElement).tagName)
      ) {
        e.preventDefault();
        searchRef.current?.focus();
        searchRef.current?.select();
      }
    };
    document.addEventListener("keydown", handler);
    return () => document.removeEventListener("keydown", handler);
  }, []);

  const DOC_PAGE_SIZE = 20;
  const me = useQuery({ queryKey: queryKeys.me(), queryFn: api.me });
  const docs = useInfiniteQuery({
    queryKey: [...queryKeys.documents(), debouncedQ],
    queryFn: ({ pageParam }) =>
      api.listDocuments({ q: debouncedQ || undefined, limit: DOC_PAGE_SIZE, offset: pageParam }),
    initialPageParam: 0,
    getNextPageParam: (lastPage, allPages) => {
      if (!lastPage.has_more) return undefined;
      return allPages.reduce((sum, p) => sum + p.items.length, 0);
    },
    placeholderData: (prev) => prev,
  });
  const pins = useQuery({ queryKey: queryKeys.pins(), queryFn: api.listPins });
  const rootFolders = useQuery({
    queryKey: [...queryKeys.folders(), debouncedQ],
    queryFn: () => api.listFolders(undefined, undefined, debouncedQ || undefined),
    placeholderData: (prev) => prev,
  });
  const { state: onb } = useOnboarding();
  const nav = useNavigate();

  useEffect(() => {
    if (onb.status === "active" && !onb.steps.cli_installed && !onb.steps.cli_authed && !onb.steps.agent_configured) {
      nav("/app/start", { replace: true });
    }
  }, [onb, nav]);

  useEffect(() => {
    if (docs.data?.pages.some((p) => p.items.length > 0)) autoMarkFirstDoc();
  }, [docs.data]);

  const documents = useMemo(
    () => docs.data?.pages.flatMap((p) => p.items) ?? [],
    [docs.data],
  );
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
      if (dragType === "folder") return api.updateFolder(dragId, undefined, targetFolderId);
      return api.moveDocumentToFolder(targetFolderId, dragId);
    },
    onSuccess: (data) => {
      qc.invalidateQueries({ queryKey: ["folders"] });
      qc.invalidateQueries({ queryKey: queryKeys.documents() });
      const warning = (data as Record<string, unknown>)?.permission_warning as { message?: string } | undefined;
      if (warning?.message) setAccessWarning(warning.message);
      else toast.success("Moved.");
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

  const searching = debouncedQ.length > 0;
  const empty = !searching && documents.length === 0 && folders.length === 0;
  const isLoading = docs.isLoading;

  return (
    <div className="flex h-full flex-col">
      {/* ── non-scrolling header ── */}
      <div className="shrink-0 bg-[var(--color-bg)]">
        <div className="mx-auto max-w-6xl px-6 pt-8 pb-5">
          <div className="flex items-end justify-between gap-4">
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
                  ref={searchRef}
                  value={q}
                  onChange={(e) => setQ(e.target.value)}
                  placeholder="Search…"
                  className={cn("w-56 pl-8", q ? "pr-7" : "pr-8")}
                />
                {q ? (
                  <button
                    type="button"
                    onClick={() => { setQ(""); searchRef.current?.focus(); }}
                    className="absolute right-2 top-1/2 -translate-y-1/2 rounded p-0.5 text-[var(--color-fg-muted)] hover:text-[var(--color-fg)]"
                    aria-label="Clear search"
                  >
                    <X className="h-3.5 w-3.5" />
                  </button>
                ) : (
                  <kbd className="pointer-events-none absolute right-2 top-1/2 -translate-y-1/2 rounded border border-[var(--color-border)] bg-[var(--color-surface-muted)] px-1 py-px text-[10px] text-[var(--color-fg-muted)]">
                    /
                  </kbd>
                )}
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
        </div>
      </div>

      {/* ── scrolling body ── */}
      <div ref={scrollContainerRef} className="flex-1 overflow-auto">
        {isLoading ? (
          <div className="mx-auto max-w-6xl px-6 py-6">
            <Skeleton className="mb-3 h-9 w-48" />
            <Skeleton className="h-64 w-full" />
          </div>
        ) : docs.error ? (
          <Center>Could not load documents.</Center>
        ) : empty ? (
          <div className="mx-auto max-w-6xl px-6 py-6">
            <EmptyDocs />
          </div>
        ) : searching ? (
          <div className="mx-auto max-w-6xl px-6 py-6">
            {documents.length === 0 && folders.length === 0 ? (
              <EmptyState title="No matches" description={`Nothing matches "${q}".`} />
            ) : viewMode === "list" ? (
              <UnifiedTable entries={merged} principal={me.data?.principal} />
            ) : (
              <UnifiedGrid entries={merged} />
            )}
          </div>
        ) : (
          <DndContext sensors={sensors} onDragStart={handleDragStart} onDragEnd={handleDragEnd}>
            {viewMode === "list" ? (
              <VirtualTable
                entries={merged}
                principal={me.data?.principal}
                pinnedIds={pinnedIds}
                scrollRef={scrollContainerRef}
                fetchNextPage={docs.fetchNextPage}
                hasNextPage={docs.hasNextPage}
                isFetchingNextPage={docs.isFetchingNextPage}
              />
            ) : (
              <VirtualGrid
                entries={merged}
                pinnedIds={pinnedIds}
                scrollRef={scrollContainerRef}
                fetchNextPage={docs.fetchNextPage}
                hasNextPage={docs.hasNextPage}
                isFetchingNextPage={docs.isFetchingNextPage}
              />
            )}
            <DragOverlay dropAnimation={null}>
              {activeDrag && viewMode === "gallery" && (
                <div className="flex h-[220px] w-52 flex-col items-center justify-center rounded-xl border border-[var(--color-accent)] bg-[var(--color-surface)] opacity-30 shadow-xl">
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
        )}
      </div>

      {/* ── dialogs (outside scroll container) ── */}
      <Dialog open={newFolderOpen} onOpenChange={setNewFolderOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>New folder</DialogTitle>
          </DialogHeader>
          <form onSubmit={(e) => { e.preventDefault(); if (newFolderTitle.trim()) createFolder.mutate(); }}>
            <Input placeholder="Folder name" value={newFolderTitle} onChange={(e) => setNewFolderTitle(e.target.value)} autoFocus />
            <DialogFooter className="mt-4">
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

/* ── virtual grid (gallery view) ─────────────────────────────────────── */

export function VirtualGrid({
  entries,
  pinnedIds,
  scrollRef,
  fetchNextPage,
  hasNextPage,
  isFetchingNextPage,
}: {
  entries: FolderEntry[];
  pinnedIds?: Set<string>;
  scrollRef: React.RefObject<HTMLDivElement | null>;
  fetchNextPage?: () => void;
  hasNextPage?: boolean;
  isFetchingNextPage?: boolean;
}) {
  const containerRef = useRef<HTMLDivElement>(null);
  const sentinelRef = useRef<HTMLDivElement>(null);
  const me = useQuery({ queryKey: queryKeys.me(), queryFn: api.me });
  const qc = useQueryClient();
  const [containerWidth, setContainerWidth] = useState(800);

  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const obs = new ResizeObserver(([entry]) => setContainerWidth(entry.contentRect.width));
    obs.observe(el);
    setContainerWidth(el.clientWidth);
    return () => obs.disconnect();
  }, []);

  useEffect(() => {
    const el = sentinelRef.current;
    if (!el || !fetchNextPage || !hasNextPage || isFetchingNextPage) return;
    const obs = new IntersectionObserver(
      ([entry]) => { if (entry.isIntersecting && !isFetchingNextPage) fetchNextPage(); },
      { rootMargin: "200px" },
    );
    obs.observe(el);
    return () => obs.disconnect();
  }, [fetchNextPage, hasNextPage, isFetchingNextPage]);

  const cols = containerWidth >= 1024 ? 4 : containerWidth >= 640 ? 3 : 2;

  const rows = useMemo(() => {
    const r: FolderEntry[][] = [];
    for (let i = 0; i < entries.length; i += cols) r.push(entries.slice(i, i + cols));
    return r;
  }, [entries, cols]);

  const rowVirtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ROW_H,
    overscan: 2,
  });

  return (
    <div ref={containerRef} className="mx-auto max-w-6xl px-6 py-6">
      <div style={{ height: rowVirtualizer.getTotalSize(), position: "relative" }}>
        {rowVirtualizer.getVirtualItems().map((vrow) => (
          <div
            key={vrow.key}
            style={{
              position: "absolute",
              top: 0,
              left: 0,
              width: "100%",
              transform: `translateY(${vrow.start}px)`,
              paddingBottom: GRID_GAP,
            }}
          >
            <div
              className="grid"
              style={{ gridTemplateColumns: `repeat(${cols}, 1fr)`, gap: GRID_GAP }}
            >
              {rows[vrow.index].map((e) => {
                if (e.type === "folder" && e.folder) {
                  const canDelete = samePrincipal(me.data?.principal, e.folder.owner);
                  return (
                    <DraggableFolderCard
                      key={e.folder.id}
                      folder={e.folder}
                      canDelete={canDelete}
                      qc={qc}
                      pinned={pinnedIds?.has(e.folder.id)}
                      lastActive={e.last_active}
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
          </div>
        ))}
      </div>
      <div ref={sentinelRef} className="h-1" />
      {isFetchingNextPage && (
        <div className="flex justify-center py-4">
          <div className="h-5 w-5 animate-spin rounded-full border-2 border-[var(--color-accent)] border-t-transparent" />
        </div>
      )}
    </div>
  );
}

/* ── virtual table (list view) ───────────────────────────────────────── */

const ROW_LIST_H = 56;

export function VirtualTable({
  entries,
  principal,
  pinnedIds,
  scrollRef,
  fetchNextPage,
  hasNextPage,
  isFetchingNextPage,
}: {
  entries: FolderEntry[];
  principal?: Principal;
  pinnedIds?: Set<string>;
  scrollRef: React.RefObject<HTMLDivElement | null>;
  fetchNextPage?: () => void;
  hasNextPage?: boolean;
  isFetchingNextPage?: boolean;
}) {
  const containerRef = useRef<HTMLDivElement>(null);
  const sentinelRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const el = sentinelRef.current;
    if (!el || !fetchNextPage || !hasNextPage || isFetchingNextPage) return;
    const obs = new IntersectionObserver(
      ([entry]) => { if (entry.isIntersecting && !isFetchingNextPage) fetchNextPage(); },
      { rootMargin: "200px" },
    );
    obs.observe(el);
    return () => obs.disconnect();
  }, [fetchNextPage, hasNextPage, isFetchingNextPage]);

  const rowVirtualizer = useVirtualizer({
    count: entries.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ROW_LIST_H,
    overscan: 5,
  });

  return (
    <div ref={containerRef} className="mx-auto max-w-6xl px-6 py-6">
      <div className="overflow-hidden rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)]">
        <table className="w-full table-fixed text-sm">
          <thead className="bg-[var(--color-surface-muted)]/50 text-[11px] uppercase tracking-wider text-[var(--color-fg-muted)]">
            <tr>
              <th className="w-1/2 px-4 py-2.5 text-left font-medium">Name</th>
              <th className="w-1/3 px-4 py-2.5 text-left font-medium">Owner</th>
              <th className="w-12 px-2 py-2.5"></th>
            </tr>
          </thead>
        </table>
        <div
          style={{
            height: rowVirtualizer.getTotalSize(),
            position: "relative",
            overflow: "hidden",
          }}
        >
          {rowVirtualizer.getVirtualItems().map((vrow) => {
            const e = entries[vrow.index];
            return (
              <div
                key={vrow.key}
                style={{
                  position: "absolute",
                  top: 0,
                  left: 0,
                  width: "100%",
                  transform: `translateY(${vrow.start}px)`,
                  height: ROW_LIST_H,
                }}
              >
                <table className="w-full table-fixed text-sm">
                  <tbody>
                    {e.type === "folder" && e.folder ? (
                      <DraggableFolderRow
                        folder={e.folder}
                        canDelete={samePrincipal(principal, e.folder.owner)}
                        pinned={pinnedIds?.has(e.folder.id)}
                        lastActive={e.last_active}
                      />
                    ) : e.type === "document" && e.document ? (
                      <DraggableDocRow
                        doc={e.document}
                        canDelete={samePrincipal(principal, e.document.owner)}
                        pinned={pinnedIds?.has(docID(e.document))}
                      />
                    ) : null}
                  </tbody>
                </table>
              </div>
            );
          })}
        </div>
      </div>
      <div ref={sentinelRef} className="h-1" />
      {isFetchingNextPage && (
        <div className="flex justify-center py-4">
          <div className="h-5 w-5 animate-spin rounded-full border-2 border-[var(--color-accent)] border-t-transparent" />
        </div>
      )}
    </div>
  );
}

/* ── static grid/table (used for search results only) ─────────────────── */

export function UnifiedGrid({ entries, pinnedIds }: { entries: FolderEntry[]; pinnedIds?: Set<string> }) {
  const me = useQuery({ queryKey: queryKeys.me(), queryFn: api.me });
  const qc = useQueryClient();
  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
      {entries.map((e) => {
        if (e.type === "folder" && e.folder) {
          const canDelete = samePrincipal(me.data?.principal, e.folder.owner);
          return <DraggableFolderCard key={e.folder.id} folder={e.folder} canDelete={canDelete} qc={qc} pinned={pinnedIds?.has(e.folder.id)} lastActive={e.last_active} />;
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

export function UnifiedTable({ entries, principal, pinnedIds }: { entries: FolderEntry[]; principal?: Principal; pinnedIds?: Set<string> }) {
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
              return <DraggableFolderRow key={e.folder.id} folder={e.folder} canDelete={samePrincipal(principal, e.folder.owner)} pinned={pinnedIds?.has(e.folder.id)} lastActive={e.last_active} />;
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

/* ── lazy preview helpers ────────────────────────────────────────────── */

function useInView(rootMargin = "50px") {
  const ref = useRef<HTMLDivElement>(null);
  const [inView, setInView] = useState(false);
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const obs = new IntersectionObserver(
      ([entry]) => { if (entry.isIntersecting) { setInView(true); obs.disconnect(); } },
      { rootMargin },
    );
    obs.observe(el);
    return () => obs.disconnect();
  }, [rootMargin]);
  return { ref, inView };
}

function DocCardPreview({ docId, versionId }: { docId: string; versionId: string }) {
  const { ref, inView } = useInView();
  const [loaded, setLoaded] = useState(false);
  const [containerWidth, setContainerWidth] = useState(240);
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const obs = new ResizeObserver(([entry]) => setContainerWidth(entry.contentRect.width));
    obs.observe(el);
    setContainerWidth(el.clientWidth);
    return () => obs.disconnect();
  }, []);
  const RENDER_W = 900;
  const RENDER_H = 600;
  const scale = containerWidth / RENDER_W;
  const tokenQ = useQuery({
    queryKey: queryKeys.render(versionId),
    queryFn: () => api.renderToken(docId, versionId),
    staleTime: 4.5 * 60_000,
    gcTime: 5 * 60_000,
    enabled: inView,
  });
  return (
    <div ref={ref} className="relative overflow-hidden bg-[var(--color-surface-muted)]" style={{ height: CARD_PREVIEW }}>
      {tokenQ.data?.url && (
        <iframe
          src={tokenQ.data.url}
          title=""
          className={cn(
            "absolute left-0 top-0 origin-top-left border-0 transition-opacity duration-500",
            loaded ? "opacity-100" : "opacity-0",
          )}
          style={{ width: `${RENDER_W}px`, height: `${RENDER_H}px`, transform: `scale(${scale})`, pointerEvents: "none" }}
          sandbox="allow-scripts"
          tabIndex={-1}
          onLoad={() => setLoaded(true)}
        />
      )}
      {!loaded && (
        <div className="absolute inset-0 flex flex-col justify-start gap-1.5 p-3">
          {[80, 65, 90, 50, 72, 60, 85].map((w, i) => (
            <div
              key={i}
              className="h-1.5 animate-pulse rounded-full bg-[var(--color-border)]"
              style={{ width: `${w}%`, animationDelay: `${i * 60}ms` }}
            />
          ))}
        </div>
      )}
    </div>
  );
}

function FolderCardPreview({ folderId }: { folderId: string }) {
  const { ref, inView } = useInView();
  const docsQ = useQuery({
    queryKey: ["folder-docs-preview", folderId],
    queryFn: () => api.listFolderDocuments(folderId),
    staleTime: 2 * 60_000,
    gcTime: 5 * 60_000,
    enabled: inView,
  });
  const items = docsQ.data?.items?.slice(0, 5) ?? [];
  const total = docsQ.data?.items?.length ?? 0;
  return (
    <div ref={ref} className="relative overflow-hidden bg-[var(--color-surface-muted)] px-3 py-3" style={{ height: CARD_PREVIEW }}>
      {docsQ.isLoading ? (
        <div className="flex flex-col gap-2">
          {[75, 55, 85, 40, 65].map((w, i) => (
            <div key={i} className="h-6 animate-pulse rounded-md bg-[var(--color-border)]" style={{ width: `${w}%`, animationDelay: `${i * 60}ms` }} />
          ))}
        </div>
      ) : items.length > 0 ? (
        <div className="flex flex-col gap-2">
          {items.map((doc) => {
            const sw = getDocSwatch(docID(doc));
            return (
              <div key={docID(doc)} className="flex items-center gap-2 rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] px-2.5 py-1.5 shadow-sm">
                <div className={cn("h-[16px] w-[3px] shrink-0 rounded-full", sw.strip)} />
                <span className="truncate text-[11px] font-medium text-[var(--color-fg)]">{docTitle(doc)}</span>
              </div>
            );
          })}
          {total > 5 && (
            <div className="pl-1 text-[10px] text-[var(--color-fg-muted)]">+{total - 5} more</div>
          )}
        </div>
      ) : (
        <div className="flex h-full items-center justify-center">
          <FolderIcon className="h-10 w-10 text-[var(--color-accent)]/20" />
        </div>
      )}
    </div>
  );
}

/* ── gallery cards ───────────────────────────────────────────────────── */

function DraggableFolderCard({
  folder: f,
  canDelete,
  qc,
  pinned,
  lastActive,
}: {
  folder: Folder;
  canDelete: boolean;
  qc: ReturnType<typeof useQueryClient>;
  pinned?: boolean;
  lastActive?: string;
}) {
  const { attributes, listeners, setNodeRef: dragRef, isDragging } = useDraggable({ id: `folder:${f.id}` });
  const { setNodeRef: dropRef, isOver } = useDroppable({ id: `drop:${f.id}` });

  return (
    <div
      ref={(node) => { dragRef(node); dropRef(node); }}
      {...listeners}
      {...attributes}
      className={cn(
        "group relative overflow-hidden rounded-xl border bg-[var(--color-surface)] transition-all duration-200",
        isOver
          ? "border-[var(--color-accent)] ring-2 ring-[var(--color-accent)]/20 shadow-md"
          : "border-[var(--color-border)] hover:border-[var(--color-border-strong)] hover:shadow-md hover:shadow-black/5",
        isDragging && "opacity-40",
      )}
      style={{ height: CARD_H }}
    >
      {/* accent strip */}
      <div className="w-full bg-[var(--color-accent)]/50" style={{ height: CARD_STRIP }} />
      {/* preview */}
      <Link to={`/app/folders/${f.id}`} className="block cursor-default" draggable={false}>
        <FolderCardPreview folderId={f.id} />
      </Link>
      {/* solid footer */}
      <div
        className="flex items-center justify-between gap-2 border-t border-[var(--color-border)] bg-[var(--color-surface)] px-3"
        style={{ height: CARD_FOOTER }}
      >
        <div className="min-w-0 flex-1">
          <div className="truncate text-[13px] font-semibold leading-snug">{f.title}</div>
          <div className="truncate text-[11px] text-[var(--color-fg-muted)]">
            {lastActive ? timeAgo(lastActive) : principalLabel(f.owner)}
          </div>
        </div>
        {pinned && <Pin className="h-3 w-3 shrink-0 fill-current text-[var(--color-accent)]" />}
      </div>
      <GridCardActions resourceType="folder" resourceId={f.id} canDelete={canDelete} title={f.title} qc={qc} pinned={pinned} />
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
  const { attributes, listeners, setNodeRef, isDragging } = useDraggable({ id: `doc:${docID(d)}` });
  const dId = docID(d);
  const swatch = getDocSwatch(dId);
  const lastActive = d.updated_at ? timeAgo(d.updated_at) : null;

  return (
    <div
      ref={setNodeRef}
      {...listeners}
      {...attributes}
      className={cn(
        "group relative overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] transition-all duration-200",
        "hover:border-[var(--color-border-strong)] hover:shadow-md hover:shadow-black/5",
        isDragging && "opacity-40",
      )}
      style={{ height: CARD_H }}
    >
      {/* deterministic color strip */}
      <div className={cn("w-full", swatch.strip)} style={{ height: CARD_STRIP }} />
      {/* preview */}
      <Link to={`/app/d/${dId}`} className="block cursor-default" draggable={false}>
        {d.current_version_id ? (
          <DocCardPreview docId={dId} versionId={d.current_version_id} />
        ) : (
          <div className={cn("flex items-center justify-center", swatch.iconBg)} style={{ height: CARD_PREVIEW }}>
            <FileText className={cn("h-6 w-6", swatch.iconText)} />
          </div>
        )}
      </Link>
      {/* solid footer */}
      <div
        className="flex items-center justify-between gap-2 border-t border-[var(--color-border)] bg-[var(--color-surface)] px-3"
        style={{ height: CARD_FOOTER }}
      >
        <div className="min-w-0 flex-1">
          <div className="truncate text-[13px] font-semibold leading-snug">{docTitle(d)}</div>
          <div className="truncate text-[11px] text-[var(--color-fg-muted)]">
            {lastActive ?? principalLabel(d.owner)}
          </div>
        </div>
        {pinned && <Pin className="h-3 w-3 shrink-0 fill-current text-[var(--color-accent)]" />}
      </div>
      <GridCardActions resourceType="document" resourceId={dId} canDelete={canDelete} title={docTitle(d)} qc={qc} pinned={pinned} />
    </div>
  );
}

/* ── list rows ───────────────────────────────────────────────────────── */

function DraggableFolderRow({
  folder: f,
  canDelete,
  pinned,
  lastActive,
}: {
  folder: Folder;
  canDelete: boolean;
  pinned?: boolean;
  lastActive?: string;
}) {
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
      className={cn(
        "group border-t transition-colors",
        isOver ? "bg-[var(--color-accent)]/10 border-[var(--color-accent)]" : "border-[var(--color-border)] hover:bg-[var(--color-surface-muted)]/40",
        isDragging && "opacity-40",
      )}
    >
      <td className="w-1/2 px-4 py-3">
        <Link to={`/app/folders/${f.id}`} className="flex cursor-default items-center gap-2 hover:underline" draggable={false}>
          <FolderIcon className="h-4 w-4 shrink-0 text-[var(--color-accent)]" />
          <div className="min-w-0">
            <div className="truncate font-medium">{f.title}</div>
            {lastActive && <div className="text-xs text-[var(--color-fg-muted)]">{timeAgo(lastActive)}</div>}
          </div>
        </Link>
      </td>
      <td className="w-1/3 px-4 py-3"><OwnerCell owner={f.owner} /></td>
      <td className="w-12 px-2 py-3">
        <div className="flex gap-0.5" onClick={(e) => e.stopPropagation()} onPointerDown={(e) => e.stopPropagation()}>
          <RowPinButton resourceType="folder" resourceId={f.id} pinned={pinned} />
          {canDelete && (
            <DeleteDocumentDialog
              title={f.title}
              description="This deletes the folder and its subfolders. Documents inside them will move to root."
              onConfirm={() => del.mutate()}
              trigger={
                <Button variant="ghost" size="icon" className="cursor-pointer opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100" aria-label="Delete folder">
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

function DraggableDocRow({ doc, canDelete, pinned }: { doc: Document; canDelete: boolean; pinned?: boolean }) {
  const { attributes, listeners, setNodeRef, isDragging } = useDraggable({ id: `doc:${docID(doc)}` });
  const swatch = getDocSwatch(docID(doc));
  const lastActive = doc.updated_at ? timeAgo(doc.updated_at) : null;
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
      className={cn(
        "group border-t border-[var(--color-border)] transition-colors hover:bg-[var(--color-surface-muted)]/40",
        isDragging && "opacity-40",
      )}
    >
      <td className="w-1/2 px-4 py-3">
        <Link to={`/app/d/${docID(doc)}`} className="flex cursor-default items-center gap-2 hover:underline" draggable={false}>
          <FileText className={cn("h-4 w-4 shrink-0", swatch.iconText)} />
          <div className="min-w-0">
            <div className="truncate font-medium">{docTitle(doc)}</div>
            {lastActive && <div className="text-xs text-[var(--color-fg-muted)]">{lastActive}</div>}
          </div>
        </Link>
      </td>
      <td className="w-1/3 px-4 py-3">
        <OwnerCell owner={doc.owner} />
      </td>
      <td className="w-12 px-2 py-3">
        <div className="flex gap-0.5" onClick={(e) => e.stopPropagation()} onPointerDown={(e) => e.stopPropagation()}>
          <RowPinButton resourceType="document" resourceId={docID(doc)} pinned={pinned} />
          {canDelete && (
            <DeleteDocumentDialog
              title={docTitle(doc)}
              onConfirm={() => del.mutate()}
              trigger={
                <Button variant="ghost" size="icon" className="cursor-pointer opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100" aria-label="Delete document">
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

/* ── shared card actions ─────────────────────────────────────────────── */

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
      className="cursor-pointer opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100"
      aria-label={isPinned ? "Unpin" : "Pin"}
      onClick={(e) => { e.preventDefault(); e.stopPropagation(); togglePin.mutate(); }}
    >
      {isPinned ? <PinOff className="h-3.5 w-3.5" /> : <Pin className="h-3.5 w-3.5" />}
    </Button>
  );
}

function GridCardActions({
  resourceType, resourceId, canDelete, title, qc, pinned,
}: {
  resourceType: "document" | "folder";
  resourceId: string;
  canDelete: boolean;
  title: string;
  qc: ReturnType<typeof useQueryClient>;
  pinned?: boolean;
}) {
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
      className="absolute right-2 top-2 z-10 flex gap-0.5 opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100 [&_button]:cursor-pointer"
      onClick={(e) => e.stopPropagation()}
      onPointerDown={(e) => e.stopPropagation()}
    >
      <Button variant="ghost" size="icon" className="h-7 w-7 bg-[var(--color-surface)]/80 backdrop-blur-sm" aria-label={isPinned ? "Unpin" : "Pin"} onClick={(e) => { e.preventDefault(); e.stopPropagation(); togglePin.mutate(); }}>
        {isPinned ? <PinOff className="h-3.5 w-3.5" /> : <Pin className="h-3.5 w-3.5" />}
      </Button>
      {canDelete && (
        <DeleteDocumentDialog
          title={title}
          description={resourceType === "folder" ? "This deletes the folder and its subfolders. Documents inside them will move to root." : undefined}
          onConfirm={() => del.mutate()}
          trigger={
            <Button variant="ghost" size="icon" className="h-7 w-7 bg-[var(--color-surface)]/80 backdrop-blur-sm" aria-label="Delete" onClick={(e) => e.stopPropagation()}>
              <Trash2 className="h-3.5 w-3.5 text-[var(--color-danger)]" />
            </Button>
          }
        />
      )}
    </div>
  );
}

/* ── misc helpers ────────────────────────────────────────────────────── */

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
          Documents are published by your AI agent. Walk through the three-step setup so it knows how to use aidocs.
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
          <HardDriveUpload className="h-4 w-4" /> Upload HTML (backup)
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
    <Dialog open={open} onOpenChange={(v) => { setOpen(v); if (!v) { reset(); setTitle(""); } }}>
      <DialogTrigger asChild>{trigger}</DialogTrigger>
      <DialogContent className="max-w-[480px] p-0 overflow-hidden gap-0">
        <div className="relative px-6 pt-6 pb-5 bg-gradient-to-br from-[var(--color-accent-muted)] via-[var(--color-surface)] to-[var(--color-surface)]">
          <div className="pointer-events-none absolute -right-8 -top-8 h-32 w-32 rounded-full bg-[var(--color-accent)]/8 blur-2xl" />
          <div className="pointer-events-none absolute right-8 top-4 h-16 w-16 rounded-full bg-[var(--color-accent)]/10 blur-xl" />
          <div className="relative flex items-start justify-between gap-3">
            <div className="flex items-center gap-3">
              <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-[var(--color-accent)] shadow-lg shadow-[var(--color-accent)]/25">
                <HardDriveUpload className="h-5 w-5 text-white" />
              </div>
              <div>
                <DialogTitle className="text-base font-bold">Upload HTML document</DialogTitle>
                <p className="mt-0.5 text-xs text-[var(--color-fg-muted)]">Manual fallback — agents use the CLI</p>
              </div>
            </div>
          </div>
        </div>
        <form className="px-6 pb-6 pt-5 space-y-4" onSubmit={(e) => { e.preventDefault(); if (file) m.mutate(); }}>
          <FileDropZone file={file} onFile={setFile} />
          <div className="space-y-1.5">
            <label className="flex items-center gap-1.5 text-xs font-medium text-[var(--color-fg-muted)]">
              Document title
              <span className="rounded bg-[var(--color-surface-muted)] px-1.5 py-0.5 text-[10px] text-[var(--color-fg-muted)]/60">optional</span>
            </label>
            <Input placeholder="Defaults to filename if left blank" value={title} onChange={(e) => setTitle(e.target.value)} className="h-10" />
          </div>
          <div className="flex items-center gap-2 rounded-lg bg-[var(--color-surface-muted)] px-3 py-2.5">
            <Terminal className="h-3 w-3 shrink-0 text-[var(--color-fg-muted)]/60" />
            <p className="text-[11px] leading-relaxed text-[var(--color-fg-muted)]/70">
              Your AI agent publishes automatically via <code className="font-mono text-[10px]">aidocs docs upload</code>. This form is for manual uploads only.
            </p>
          </div>
          <div className="flex gap-2.5 pt-1">
            <Button type="button" variant="ghost" className="flex-1" onClick={() => setOpen(false)}>Cancel</Button>
            <Button type="submit" className="flex-1" disabled={!file || m.isPending}>
              {m.isPending ? (
                <span className="flex items-center gap-2">
                  <span className="h-3.5 w-3.5 animate-spin rounded-full border-2 border-current border-t-transparent" />
                  Uploading…
                </span>
              ) : (
                <span className="flex items-center gap-2">
                  <HardDriveUpload className="h-3.5 w-3.5" />
                  Upload document
                </span>
              )}
            </Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  );
}
