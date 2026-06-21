import { useState } from "react";
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
  Pin,
  PinOff,
  Plus,
  Share2,
  Trash2,
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
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  api,
  type Folder,
  type FolderEntry,
  type Document,
} from "@/api";
import { queryKeys } from "@/lib/queryKeys";
import { ShareSheet } from "@/pages/doc-share-sheet";
import { Sheet, SheetTrigger } from "@/components/ui/sheet";

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

  useTitle(folder.data?.title || "Folder");

  const isPinned = (pins.data?.items || []).some(
    (p) => p.resource_type === "folder" && p.resource_id === id,
  );

  const togglePin = useMutation({
    mutationFn: () =>
      isPinned ? api.deletePin("folder", id) : api.putPin("folder", id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.pins() });
      toast.success(isPinned ? "Unpinned." : "Pinned.");
    },
    onError: (e) => toast.error(errorMessage(e)),
  });

  const delFolder = useMutation({
    mutationFn: () => api.deleteFolder(id),
    onSuccess: () => {
      toast.success("Deleted folder.");
      qc.invalidateQueries({ queryKey: queryKeys.folders() });
      nav("/app/documents");
    },
    onError: (e) => toast.error(errorMessage(e, "Delete failed")),
  });

  const [newFolderOpen, setNewFolderOpen] = useState(false);
  const [newFolderTitle, setNewFolderTitle] = useState("");
  const [renameOpen, setRenameOpen] = useState(false);
  const [renameTitle, setRenameTitle] = useState("");
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
  const breadcrumbs = buildBreadcrumbs(f, contents.data?.folder);

  return (
    <div className="mx-auto max-w-6xl px-6 py-10">
      {/* Breadcrumbs */}
      <nav className="mb-4 flex items-center gap-1 text-sm text-[var(--color-fg-muted)]">
        <Link to="/app/documents" className="hover:underline">
          Documents
        </Link>
        {breadcrumbs.map((b) => (
          <span key={b.id} className="flex items-center gap-1">
            <ChevronRight className="h-3 w-3" />
            <Link to={`/app/folders/${b.id}`} className="hover:underline">
              {b.title}
            </Link>
          </span>
        ))}
      </nav>

      {/* Header */}
      <div className="mb-6 flex items-center justify-between">
        <div className="flex items-center gap-3">
          <FolderIcon className="h-6 w-6 text-[var(--color-accent)]" />
          <h1 className="text-2xl font-semibold tracking-tight">{f.title}</h1>
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="ghost"
            size="icon"
            onClick={() => togglePin.mutate()}
            aria-label={isPinned ? "Unpin" : "Pin"}
          >
            {isPinned ? (
              <PinOff className="h-4 w-4" />
            ) : (
              <Pin className="h-4 w-4" />
            )}
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
              <AlertDialog>
                <AlertDialogTrigger asChild>
                  <DropdownItem onSelect={(e) => e.preventDefault()} className="text-[var(--color-danger)]">
                    <Trash2 className="h-4 w-4" /> Delete folder
                  </DropdownItem>
                </AlertDialogTrigger>
                <AlertDialogContent>
                  <AlertDialogTitle>Delete "{f.title}"?</AlertDialogTitle>
                  <AlertDialogDescription>
                    This will delete all subfolders. Documents inside will be detached, not deleted.
                  </AlertDialogDescription>
                  <AlertDialogFooter>
                    <AlertDialogCancel>Cancel</AlertDialogCancel>
                    <AlertDialogAction onClick={() => delFolder.mutate()}>
                      Delete
                    </AlertDialogAction>
                  </AlertDialogFooter>
                </AlertDialogContent>
              </AlertDialog>
            </DropdownContent>
          </Dropdown>
        </div>
      </div>

      {/* Contents */}
      {items.length === 0 ? (
        <EmptyState
          title="Empty folder"
          description="Add documents or create subfolders."
        />
      ) : (
        <div className="overflow-hidden rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)]">
          <table className="w-full text-sm">
            <thead className="bg-[var(--color-surface-muted)]/50 text-[11px] uppercase tracking-wider text-[var(--color-fg-muted)]">
              <tr>
                <th className="px-4 py-2.5 text-left font-medium">Name</th>
                <th className="px-4 py-2.5 text-left font-medium">Owner</th>
              </tr>
            </thead>
            <tbody>
              {items.map((entry) => (
                <EntryRow key={entryKey(entry)} entry={entry} />
              ))}
            </tbody>
          </table>
        </div>
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
    </div>
  );
}

function EntryRow({ entry }: { entry: FolderEntry }) {
  if (entry.type === "folder" && entry.folder) {
    const f = entry.folder;
    return (
      <tr className="border-t border-[var(--color-border)] transition-colors hover:bg-[var(--color-surface-muted)]/40">
        <td className="px-4 py-3">
          <Link
            to={`/app/folders/${f.id}`}
            className="flex items-center gap-2 font-medium hover:underline"
          >
            <FolderIcon className="h-4 w-4 text-[var(--color-accent)]" />
            {f.title}
          </Link>
        </td>
        <td className="px-4 py-3">
          <OwnerLabel owner={f.owner} />
        </td>
      </tr>
    );
  }
  if (entry.type === "document" && entry.document) {
    const d = entry.document;
    return (
      <tr className="border-t border-[var(--color-border)] transition-colors hover:bg-[var(--color-surface-muted)]/40">
        <td className="px-4 py-3">
          <Link
            to={`/app/d/${d.id}`}
            className="flex items-center gap-2 font-medium hover:underline"
          >
            <FileText className="h-4 w-4 text-[var(--color-fg-muted)]" />
            {d.title || "Untitled"}
          </Link>
        </td>
        <td className="px-4 py-3">
          <OwnerLabel owner={d.owner} />
        </td>
      </tr>
    );
  }
  return null;
}

function OwnerLabel({ owner }: { owner?: { name?: string; email?: string } }) {
  const label = owner?.name || owner?.email || "Unknown";
  return <span className="text-sm text-[var(--color-fg-muted)]">{label}</span>;
}

function entryKey(e: FolderEntry) {
  return e.type === "folder" ? `f:${e.folder?.id}` : `d:${e.document?.id}`;
}

function buildBreadcrumbs(
  _current: Folder,
  _parent?: Folder,
): { id: string; title: string }[] {
  // For now return just the current folder in breadcrumbs.
  // Full ancestor chain would require an additional API call.
  return [{ id: _current.id, title: _current.title }];
}
