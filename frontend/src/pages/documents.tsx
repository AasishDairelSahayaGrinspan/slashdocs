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
import { api, docID, docTitle, type Document, type Folder, type FolderEntry, type Pin as PinType, type Principal } from "@/api";
import { queryKeys } from "@/lib/queryKeys";
import { createPersistentStore } from "@/lib/persistent-store";
import { LayoutGrid, List } from "lucide-react";

type ViewMode = "list" | "gallery";
const viewModeStore = createPersistentStore<ViewMode>("aidocs:view-mode", "gallery");

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
  });
  const pins = useQuery({
    queryKey: queryKeys.pins(),
    queryFn: api.listPins,
  });
  const rootFolders = useQuery({
    queryKey: queryKeys.folders(),
    queryFn: () => api.listFolders(),
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
  const items = useMemo(() => docs.data?.items || [], [docs.data]);
  const pinnedItems = pins.data?.items || [];
  const folders = (rootFolders.data?.items || [])
    .filter((e): e is FolderEntry & { folder: Folder } => e.type === "folder" && !!e.folder)
    .map((e) => e.folder);

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
  const empty = !searching && (docs.data?.items || []).length === 0 && folders.length === 0;

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
          <Dropdown>
            <DropdownTrigger asChild>
              <Button variant="outline" size="icon" aria-label="More">
                <MoreHorizontal className="h-4 w-4" />
              </Button>
            </DropdownTrigger>
            <DropdownContent>
              <DropdownItem onSelect={() => setNewFolderOpen(true)}>
                <Plus className="h-4 w-4" /> New folder
              </DropdownItem>
              <UploadBackupItem />
            </DropdownContent>
          </Dropdown>
        </div>
      </div>

      {empty ? (
        <EmptyDocs />
      ) : searching ? (
        items.length === 0 ? (
          <EmptyState title="No matches" description={`Nothing matches "${q}".`} />
        ) : viewMode === "list" ? (
          <DocTable items={items} principal={me.data?.principal} />
        ) : (
          <DocGrid items={items} />
        )
      ) : (
        <div className="space-y-8">
          {/* Pinned section */}
          {pinnedItems.length > 0 && (
            <section>
              <h2 className="mb-3 flex items-center gap-2 text-sm font-medium text-[var(--color-fg-muted)]">
                <Pin className="h-3.5 w-3.5" /> Pinned
              </h2>
              <PinnedGrid items={pinnedItems} documents={items} folders={folders} />
            </section>
          )}

          {/* Folders section */}
          {folders.length > 0 && (
            <section>
              <h2 className="mb-3 flex items-center gap-2 text-sm font-medium text-[var(--color-fg-muted)]">
                <FolderIcon className="h-3.5 w-3.5" /> Folders
              </h2>
              {viewMode === "list" ? <FolderTable folders={folders} /> : <FolderGrid folders={folders} />}
            </section>
          )}

          {/* All documents */}
          {items.length > 0 && (
            <section>
              <h2 className="mb-3 text-sm font-medium text-[var(--color-fg-muted)]">
                All documents
              </h2>
              {viewMode === "list" ? (
                <DocTable items={items} principal={me.data?.principal} />
              ) : (
                <DocGrid items={items} />
              )}
            </section>
          )}
        </div>
      )}

      {/* New folder dialog */}
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
    </div>
  );
}

function PinnedGrid({ items, documents, folders }: { items: PinType[]; documents: Document[]; folders: Folder[] }) {
  type PinCard = { key: string; to: string; icon: "folder" | "document"; title: string };
  const resolved: PinCard[] = [];
  for (const pin of items) {
    if (pin.resource_type === "folder") {
      const f = folders.find((x) => x.id === pin.resource_id);
      if (f) resolved.push({ key: `pin:folder:${f.id}`, to: `/app/folders/${f.id}`, icon: "folder", title: f.title });
    } else {
      const d = documents.find((x) => x.id === pin.resource_id);
      if (d) resolved.push({ key: `pin:doc:${d.id}`, to: `/app/d/${d.id}`, icon: "document", title: docTitle(d) });
    }
  }
  if (resolved.length === 0) return null;
  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
      {resolved.map((item) => (
        <Link key={item.key} to={item.to} className="flex items-center gap-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2.5 text-sm font-medium transition-colors hover:bg-[var(--color-surface-muted)]">
          {item.icon === "folder" ? <FolderIcon className="h-4 w-4 text-[var(--color-accent)]" /> : <FileText className="h-4 w-4 text-[var(--color-fg-muted)]" />}
          <span className="truncate">{item.title}</span>
        </Link>
      ))}
    </div>
  );
}

function FolderGrid({ folders }: { folders: Folder[] }) {
  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
      {folders.map((f) => (
        <Link key={f.id} to={`/app/folders/${f.id}`} className="group relative flex items-center gap-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2.5 text-sm font-medium transition-colors hover:bg-[var(--color-surface-muted)]">
          <FolderIcon className="h-4 w-4 shrink-0 text-[var(--color-accent)]" />
          <span className="truncate">{f.title}</span>
          <span className="absolute right-2 top-1/2 -translate-y-1/2 opacity-0 transition-opacity group-hover:opacity-100">
            <MoreHorizontal className="h-3.5 w-3.5 text-[var(--color-fg-muted)]" />
          </span>
        </Link>
      ))}
    </div>
  );
}

function FolderTable({ folders }: { folders: Folder[] }) {
  return (
    <div className="overflow-hidden rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)]">
      <table className="w-full text-sm">
        <thead className="bg-[var(--color-surface-muted)]/50 text-[11px] uppercase tracking-wider text-[var(--color-fg-muted)]">
          <tr>
            <th className="px-4 py-2.5 text-left font-medium">Title</th>
            <th className="px-4 py-2.5 text-left font-medium">Owner</th>
          </tr>
        </thead>
        <tbody>
          {folders.map((f) => (
            <tr key={f.id} className="border-t border-[var(--color-border)] transition-colors hover:bg-[var(--color-surface-muted)]/40">
              <td className="px-4 py-3">
                <Link to={`/app/folders/${f.id}`} className="flex items-center gap-2 font-medium hover:underline">
                  <FolderIcon className="h-4 w-4 text-[var(--color-accent)]" />
                  {f.title}
                </Link>
              </td>
              <td className="px-4 py-3">
                <OwnerCell owner={f.owner} />
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function DocGrid({ items }: { items: Document[] }) {
  const me = useQuery({ queryKey: queryKeys.me(), queryFn: api.me });
  const qc = useQueryClient();
  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
      {items.map((d) => {
        const canDelete = samePrincipal(me.data?.principal, d.owner);
        return (
          <div key={docID(d)} className="group relative rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] transition-colors hover:bg-[var(--color-surface-muted)]">
            <Link to={`/app/d/${docID(d)}`} className="flex flex-col gap-2 p-3">
              <div className="flex h-20 items-center justify-center rounded-md bg-[var(--color-surface-muted)]/60">
                <FileText className="h-8 w-8 text-[var(--color-fg-muted)]/40" />
              </div>
              <div className="min-w-0">
                <div className="truncate text-sm font-medium">{docTitle(d)}</div>
                <div className="truncate text-xs text-[var(--color-fg-muted)]">
                  {d.owner?.name || d.owner?.email || ""}
                </div>
              </div>
            </Link>
            {canDelete && (
              <DocGridActions docId={docID(d)} title={docTitle(d)} qc={qc} />
            )}
          </div>
        );
      })}
    </div>
  );
}

function DocGridActions({ docId, title, qc }: { docId: string; title: string; qc: ReturnType<typeof useQueryClient> }) {
  const del = useMutation({
    mutationFn: () => api.deleteDocument(docId),
    onSuccess: () => { toast.success("Deleted."); qc.invalidateQueries({ queryKey: queryKeys.documents() }); },
    onError: (e) => toast.error(errorMessage(e, "Delete failed")),
  });
  return (
    <div className="absolute right-2 top-2 opacity-0 transition-opacity group-hover:opacity-100">
      <DeleteDocumentDialog
        title={title}
        onConfirm={() => del.mutate()}
        trigger={
          <Button variant="ghost" size="icon" className="h-7 w-7" aria-label="Delete">
            <Trash2 className="h-3.5 w-3.5 text-[var(--color-danger)]" />
          </Button>
        }
      />
    </div>
  );
}

function DocTable({ items, principal }: { items: Document[]; principal?: Principal }) {
  return (
    <div className="overflow-hidden rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)]">
      <table className="w-full text-sm">
        <thead className="bg-[var(--color-surface-muted)]/50 text-[11px] uppercase tracking-wider text-[var(--color-fg-muted)]">
          <tr>
            <th className="px-4 py-2.5 text-left font-medium">Title</th>
            <th className="px-4 py-2.5 text-left font-medium">Owner</th>
            <th className="w-12 px-2 py-2.5"></th>
          </tr>
        </thead>
        <tbody>
          {items.map((d) => (
            <DocRow
              key={docID(d)}
              doc={d}
              canDelete={samePrincipal(principal, d.owner)}
            />
          ))}
        </tbody>
      </table>
    </div>
  );
}

function DocRow({ doc, canDelete }: { doc: Document; canDelete: boolean }) {
  const q = useQueryClient();
  const del = useMutation({
    mutationFn: () => api.deleteDocument(docID(doc)),
    onSuccess: () => {
      toast.success("Deleted.");
      q.invalidateQueries({ queryKey: queryKeys.documents() });
    },
    onError: (e) => toast.error(errorMessage(e, "Delete failed")),
  });
  return (
    <tr className="group border-t border-[var(--color-border)] transition-colors hover:bg-[var(--color-surface-muted)]/40">
      <td className="px-4 py-3">
        <Link
          to={`/app/d/${docID(doc)}`}
          className="flex items-center gap-2 font-medium hover:underline"
        >
          <FileText className="h-4 w-4 text-[var(--color-fg-muted)]" />
          {docTitle(doc)}
        </Link>
      </td>
      <td className="px-4 py-3">
        <OwnerCell owner={doc.owner} />
      </td>
      <td className="px-2 py-3">
        {canDelete ? (
          <DeleteDocumentDialog
            title={docTitle(doc)}
            onConfirm={() => del.mutate()}
            trigger={
              <Button
                variant="ghost"
                size="icon"
                className="opacity-0 transition-opacity group-hover:opacity-100"
                aria-label="Delete document"
              >
                <Trash2 className="h-3.5 w-3.5 text-[var(--color-danger)]" />
              </Button>
            }
          />
        ) : null}
      </td>
    </tr>
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
      <UploadBackupCard />
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

function UploadBackupCard() {
  return (
    <div className="rounded-[14px] border border-dashed border-[var(--color-border)] p-4 text-center text-sm text-[var(--color-fg-muted)]">
      Manual fallback —{" "}
      <UploadBackupDialog
        trigger={
          <button className="font-medium text-[var(--color-fg)] underline-offset-2 hover:underline">
            upload an HTML file manually
          </button>
        }
      />
      .
    </div>
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
