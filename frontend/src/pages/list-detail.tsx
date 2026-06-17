import { useTitle } from "@/lib/use-title";
import { useState } from "react";
import { Link, useParams, useNavigate } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { errorMessage } from "@/lib/errors";
import {
  ArrowLeft,
  FileText,
  MoreHorizontal,
  Pencil,
  Trash2,
  X,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Avatar, Center, EmptyState, Skeleton } from "@/components/ui/misc";
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
import { api, docID, docTitle, type Document } from "@/api";
import { queryKeys } from "@/lib/queryKeys";

export function ListDetailPage() {
  const { id = "" } = useParams<{ id: string }>();
  const nav = useNavigate();
  const q = useQueryClient();

  const list = useQuery({
    queryKey: queryKeys.list(id),
    queryFn: () => api.listLists().then((r) => r.items.find((l) => l.id === id)),
    enabled: !!id,
  });
  const docs = useQuery({
    queryKey: queryKeys.listDocuments(id),
    queryFn: () => api.getListDocuments(id),
    enabled: !!id,
  });

  useTitle(list.data?.name ?? "List");

  const [renaming, setRenaming] = useState(false);
  const [nameInput, setNameInput] = useState("");
  const [deleteOpen, setDeleteOpen] = useState(false);

  const rename = useMutation({
    mutationFn: (name: string) => api.updateList(id, name),
    onSuccess: (updated) => {
      toast.success("List renamed.");
      q.invalidateQueries({ queryKey: queryKeys.lists() });
      q.setQueryData(queryKeys.list(id), updated);
      setRenaming(false);
    },
    onError: (e) => toast.error(errorMessage(e, "Could not rename")),
  });

  const deleteList = useMutation({
    mutationFn: () => api.deleteList(id),
    onSuccess: () => {
      toast.success("List deleted.");
      q.invalidateQueries({ queryKey: queryKeys.lists() });
      nav("/app/lists", { replace: true });
    },
    onError: (e) => toast.error(errorMessage(e, "Could not delete list")),
  });

  const removeDoc = useMutation({
    mutationFn: (docId: string) => api.removeDocumentFromList(id, docId),
    onSuccess: () => {
      toast.success("Removed from list.");
      q.invalidateQueries({ queryKey: queryKeys.listDocuments(id) });
    },
    onError: (e) => toast.error(errorMessage(e, "Could not remove document")),
  });

  if (list.isLoading || docs.isLoading) {
    return (
      <div className="mx-auto max-w-6xl px-6 py-10">
        <Skeleton className="mb-3 h-9 w-48" />
        <Skeleton className="h-64 w-full" />
      </div>
    );
  }
  if (!list.data) return <Center>List not found.</Center>;
  if (docs.error) return <Center>Could not load documents.</Center>;

  const items = docs.data?.items || [];
  const listName = list.data.name;

  return (
    <div className="mx-auto max-w-6xl px-6 py-10">
      <div className="mb-6">
        <Link
          to="/app/lists"
          className="mb-4 flex items-center gap-1 text-sm text-[var(--color-fg-muted)] hover:text-[var(--color-fg)]"
        >
          <ArrowLeft className="h-3.5 w-3.5" /> Lists
        </Link>
        <div className="flex items-end justify-between gap-4">
          <div className="flex-1">
            {renaming ? (
              <form
                className="flex items-center gap-2"
                onSubmit={(e) => {
                  e.preventDefault();
                  if (nameInput.trim()) rename.mutate(nameInput.trim());
                }}
              >
                <Input
                  value={nameInput}
                  onChange={(e) => setNameInput(e.target.value)}
                  autoFocus
                  className="max-w-xs text-2xl font-semibold"
                />
                <Button type="submit" size="sm" disabled={rename.isPending}>
                  Save
                </Button>
                <Button
                  type="button"
                  size="sm"
                  variant="ghost"
                  onClick={() => setRenaming(false)}
                >
                  Cancel
                </Button>
              </form>
            ) : (
              <h1 className="text-2xl font-semibold tracking-tight">
                {listName}
              </h1>
            )}
            <p className="mt-1 text-sm text-[var(--color-fg-muted)]">
              {items.length} document{items.length !== 1 ? "s" : ""}
            </p>
          </div>
          <Dropdown>
            <DropdownTrigger asChild>
              <Button variant="outline" size="icon" aria-label="List actions">
                <MoreHorizontal className="h-4 w-4" />
              </Button>
            </DropdownTrigger>
            <DropdownContent>
              <DropdownItem
                onSelect={(e) => {
                  e.preventDefault();
                  setNameInput(listName);
                  setRenaming(true);
                }}
              >
                <Pencil className="h-4 w-4" /> Rename
              </DropdownItem>
              <DropdownItem
                onSelect={(e) => {
                  e.preventDefault();
                  setDeleteOpen(true);
                }}
                className="text-[var(--color-danger)]"
              >
                <Trash2 className="h-4 w-4" /> Delete list
              </DropdownItem>
            </DropdownContent>
          </Dropdown>
        </div>
      </div>

      {items.length === 0 ? (
        <EmptyState
          title="No documents yet"
          description='Open a document and use "Add to list" to add it here.'
        />
      ) : (
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
                <ListDocRow
                  key={docID(d)}
                  doc={d}
                  onRemove={() => removeDoc.mutate(docID(d))}
                />
              ))}
            </tbody>
          </table>
        </div>
      )}

      <AlertDialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <AlertDialogContent>
          <AlertDialogTitle>Delete "{listName}"?</AlertDialogTitle>
            <AlertDialogDescription>
              The list will be deleted. Documents inside it are not affected.
            </AlertDialogDescription>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={() => deleteList.mutate()}
              className="bg-[var(--color-danger)] text-white hover:bg-[var(--color-danger)]/90"
            >
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

function ListDocRow({
  doc,
  onRemove,
}: {
  doc: Document;
  onRemove: () => void;
}) {
  const owner = doc.owner;
  const label = owner?.name || owner?.email || owner?.id || "Unknown owner";
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
        <div className="flex min-w-0 items-center gap-2">
          <Avatar name={label} src={owner?.picture_url} size={28} />
          <span className="truncate text-sm">{label}</span>
        </div>
      </td>
      <td className="px-2 py-3">
        <Button
          variant="ghost"
          size="icon"
          className="opacity-0 transition-opacity group-hover:opacity-100"
          aria-label="Remove from list"
          onClick={onRemove}
          title="Remove from list"
        >
          <X className="h-3.5 w-3.5 text-[var(--color-fg-muted)]" />
        </Button>
      </td>
    </tr>
  );
}
