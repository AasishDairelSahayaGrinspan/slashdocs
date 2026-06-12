import { useTitle } from "@/lib/use-title";
import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useOnboarding, autoMarkFirstDoc } from "@/lib/onboarding";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { errorMessage } from "@/lib/errors";
import {
  FileText,
  Folder,
  FolderPlus,
  MoreHorizontal,
  Pin,
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
  type Folder as FolderType,
  type Pin as PinType,
  type Principal,
} from "@/api";
import { queryKeys } from "@/lib/queryKeys";

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
  const foldersQuery = useQuery({
    queryKey: queryKeys.folders(),
    queryFn: api.listFolders,
  });
  const pinsQuery = useQuery({
    queryKey: queryKeys.pins(),
    queryFn: api.listPins,
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

  const pinnedIDs = useMemo(() => {
    const s = new Set<string>();
    for (const p of pinsQuery.data?.items ?? []) {
      if (p.resource_type === "document") s.add(p.resource_id);
    }
    return s;
  }, [pinsQuery.data]);

  const pinnedFolderIDs = useMemo(() => {
    const s = new Set<string>();
    for (const p of pinsQuery.data?.items ?? []) {
      if (p.resource_type === "folder") s.add(p.resource_id);
    }
    return s;
  }, [pinsQuery.data]);

  const items = useMemo(() => docs.data?.items || [], [docs.data]);
  const { pinned, rest } = useMemo(() => {
    const pinned: Document[] = [];
    const rest: Document[] = [];
    for (const d of items) {
      if (pinnedIDs.has(docID(d))) pinned.push(d);
      else rest.push(d);
    }
    return { pinned, rest };
  }, [items, pinnedIDs]);

  const folders = useMemo(() => foldersQuery.data?.items ?? [], [foldersQuery.data]);
  const { pinnedFolders, restFolders } = useMemo(() => {
    const pinnedFolders: FolderType[] = [];
    const restFolders: FolderType[] = [];
    for (const f of folders) {
      if (pinnedFolderIDs.has(f.id)) pinnedFolders.push(f);
      else restFolders.push(f);
    }
    return { pinnedFolders, restFolders };
  }, [folders, pinnedFolderIDs]);

  if (docs.isLoading || pinsQuery.isLoading) {
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
          <Dropdown>
            <DropdownTrigger asChild>
              <Button variant="outline" size="icon" aria-label="More">
                <MoreHorizontal className="h-4 w-4" />
              </Button>
            </DropdownTrigger>
            <DropdownContent>
              <UploadBackupItem />
              <CreateFolderItem />
            </DropdownContent>
          </Dropdown>
        </div>
      </div>

      {empty ? (
        <EmptyDocs />
      ) : items.length === 0 && !searching ? (
        <EmptyState title="No matches" description={`Nothing matches "${q}".`} />
      ) : (
        <div className="space-y-6">
          {/* Pinned section — folders then docs */}
          {(pinnedFolders.length > 0 || pinned.length > 0) && (
            <Section
              icon={<Pin className="h-3.5 w-3.5" />}
              label="Pinned"
              tinted
            >
              {pinnedFolders.map((f) => (
                <FolderRow
                  key={f.id}
                  folder={f}
                  pinned
                  principal={me.data?.principal}
                />
              ))}
              {pinned.map((d) => (
                <DocRow
                  key={docID(d)}
                  doc={d}
                  pinned
                  canDelete={samePrincipal(me.data?.principal, d.owner)}
                />
              ))}
            </Section>
          )}

          {/* Folders section */}
          {restFolders.length > 0 && (
            <Section
              icon={<Folder className="h-3.5 w-3.5" />}
              label="Folders"
            >
              {restFolders.map((f) => (
                <FolderRow
                  key={f.id}
                  folder={f}
                  pinned={false}
                  principal={me.data?.principal}
                />
              ))}
            </Section>
          )}

          {/* All documents */}
          {(rest.length > 0 || searching) && (
            <Section
              icon={<FileText className="h-3.5 w-3.5" />}
              label={searching ? "Search results" : "All documents"}
            >
              {rest.length === 0 ? (
                <tr>
                  <td colSpan={3} className="px-4 py-6 text-center text-sm text-[var(--color-fg-muted)]">
                    Nothing matches &ldquo;{q}&rdquo;.
                  </td>
                </tr>
              ) : (
                rest.map((d) => (
                  <DocRow
                    key={docID(d)}
                    doc={d}
                    pinned={false}
                    canDelete={samePrincipal(me.data?.principal, d.owner)}
                  />
                ))
              )}
            </Section>
          )}
        </div>
      )}
    </div>
  );
}

function Section({
  icon,
  label,
  tinted = false,
  children,
}: {
  icon: React.ReactNode;
  label: string;
  tinted?: boolean;
  children: React.ReactNode;
}) {
  return (
    <div className="overflow-hidden rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)]">
      <table className="w-full text-sm">
        <thead
          className={`text-[11px] uppercase tracking-wider text-[var(--color-fg-muted)] ${
            tinted
              ? "bg-[var(--color-accent-muted)]/30"
              : "bg-[var(--color-surface-muted)]/50"
          }`}
        >
          <tr>
            <th className="px-4 py-2.5 text-left font-medium">
              <span className="flex items-center gap-1.5">
                {icon}
                {label}
              </span>
            </th>
            <th className="px-4 py-2.5 text-left font-medium">Owner</th>
            <th className="w-20 px-2 py-2.5"></th>
          </tr>
        </thead>
        <tbody>{children}</tbody>
      </table>
    </div>
  );
}

function FolderRow({
  folder,
  pinned,
  principal,
}: {
  folder: FolderType;
  pinned: boolean;
  principal?: Principal;
}) {
  const q = useQueryClient();
  const canDelete = samePrincipal(principal, folder.owner);
  const delMut = useMutation({
    mutationFn: () => api.deleteFolder(folder.id),
    onSuccess: () => {
      toast.success("Folder deleted.");
      q.invalidateQueries({ queryKey: queryKeys.folders() });
    },
    onError: (e) => toast.error(errorMessage(e, "Delete failed")),
  });
  const pinMut = useMutation({
    mutationFn: () =>
      pinned
        ? api.unpinResource("folder", folder.id)
        : api.pinResource("folder", folder.id),
    onSuccess: () => q.invalidateQueries({ queryKey: queryKeys.pins() }),
    onError: (e) => toast.error(errorMessage(e, "Pin failed")),
  });

  return (
    <tr className="group border-t border-[var(--color-border)] transition-colors hover:bg-[var(--color-surface-muted)]/40">
      <td className="px-4 py-3">
        <div className="flex items-center gap-2 font-medium">
          <Folder className="h-4 w-4 text-[var(--color-accent)]" />
          {folder.title}
        </div>
      </td>
      <td className="px-4 py-3">
        <OwnerCell owner={folder.owner} />
      </td>
      <td className="px-2 py-3">
        <div className="flex items-center gap-1 opacity-0 transition-opacity group-hover:opacity-100">
          <Button
            variant="ghost"
            size="icon"
            onClick={() => pinMut.mutate()}
            aria-label={pinned ? "Unpin folder" : "Pin folder"}
          >
            <Pin
              className={`h-3.5 w-3.5 ${pinned ? "fill-[var(--color-accent)] text-[var(--color-accent)]" : "text-[var(--color-fg-muted)]"}`}
            />
          </Button>
          {canDelete && (
            <Button
              variant="ghost"
              size="icon"
              onClick={() => delMut.mutate()}
              aria-label="Delete folder"
            >
              <Trash2 className="h-3.5 w-3.5 text-[var(--color-danger)]" />
            </Button>
          )}
        </div>
      </td>
    </tr>
  );
}

function DocRow({
  doc,
  canDelete,
  pinned,
}: {
  doc: Document;
  canDelete: boolean;
  pinned: boolean;
}) {
  const q = useQueryClient();
  const del = useMutation({
    mutationFn: () => api.deleteDocument(docID(doc)),
    onSuccess: () => {
      toast.success("Deleted.");
      q.invalidateQueries({ queryKey: queryKeys.documents() });
    },
    onError: (e) => toast.error(errorMessage(e, "Delete failed")),
  });
  const pinMut = useMutation({
    mutationFn: () =>
      pinned
        ? api.unpinResource("document", docID(doc))
        : api.pinResource("document", docID(doc)),
    onSuccess: () => q.invalidateQueries({ queryKey: queryKeys.pins() }),
    onError: (e) => toast.error(errorMessage(e, "Pin failed")),
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
        <div className="flex items-center gap-1 opacity-0 transition-opacity group-hover:opacity-100">
          <Button
            variant="ghost"
            size="icon"
            onClick={() => pinMut.mutate()}
            aria-label={pinned ? "Unpin" : "Pin"}
          >
            <Pin
              className={`h-3.5 w-3.5 ${pinned ? "fill-[var(--color-accent)] text-[var(--color-accent)]" : "text-[var(--color-fg-muted)]"}`}
            />
          </Button>
          {canDelete ? (
            <DeleteDocumentDialog
              title={docTitle(doc)}
              onConfirm={() => del.mutate()}
              trigger={
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label="Delete document"
                >
                  <Trash2 className="h-3.5 w-3.5 text-[var(--color-danger)]" />
                </Button>
              }
            />
          ) : null}
        </div>
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

function CreateFolderItem() {
  return (
    <CreateFolderDialog
      trigger={
        <DropdownItem onSelect={(e) => e.preventDefault()}>
          <FolderPlus className="h-4 w-4" /> New folder
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

function CreateFolderDialog({ trigger }: { trigger: React.ReactNode }) {
  const q = useQueryClient();
  const [title, setTitle] = useState("");
  const [open, setOpen] = useState(false);
  const m = useMutation({
    mutationFn: () => api.createFolder(title),
    onSuccess: () => {
      q.invalidateQueries({ queryKey: queryKeys.folders() });
      toast.success(`Folder "${title}" created.`);
      setOpen(false);
      setTitle("");
    },
    onError: (e) => toast.error(errorMessage(e, "Failed to create folder")),
  });
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>{trigger}</DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>New folder</DialogTitle>
          <DialogDescription>
            Folders help you organise documents and share access with your team.
          </DialogDescription>
        </DialogHeader>
        <form
          className="space-y-3"
          onSubmit={(e) => {
            e.preventDefault();
            if (title.trim()) m.mutate();
          }}
        >
          <Input
            placeholder="Folder name"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            autoFocus
          />
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={() => setOpen(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={!title.trim() || m.isPending}>
              {m.isPending ? "Creating…" : "Create folder"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
