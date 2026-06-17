import { useTitle } from "@/lib/use-title";
import { useState } from "react";
import { Link } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { errorMessage } from "@/lib/errors";
import { List, MoreHorizontal, Plus, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Center, EmptyState, Skeleton } from "@/components/ui/misc";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
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
import { api, type DocList } from "@/api";
import { queryKeys } from "@/lib/queryKeys";

export function ListsPage() {
  useTitle("Lists");
  const lists = useQuery({
    queryKey: queryKeys.lists(),
    queryFn: api.listLists,
  });

  if (lists.isLoading) {
    return (
      <div className="mx-auto max-w-6xl px-6 py-10">
        <Skeleton className="mb-3 h-9 w-48" />
        <Skeleton className="h-64 w-full" />
      </div>
    );
  }
  if (lists.error) return <Center>Could not load lists.</Center>;

  const items = lists.data?.items || [];

  return (
    <div className="mx-auto max-w-6xl px-6 py-10">
      <div className="mb-6 flex items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Lists</h1>
          <p className="mt-1 text-sm text-[var(--color-fg-muted)]">
            Curated collections of documents.
          </p>
        </div>
        <CreateListButton />
      </div>

      {items.length === 0 ? (
        <EmptyState
          title="No lists yet"
          description="Create a list to organise your documents — Tech Specs, To Read, etc."
        />
      ) : (
        <ListTable items={items} />
      )}
    </div>
  );
}

function ListTable({ items }: { items: DocList[] }) {
  return (
    <div className="overflow-hidden rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)]">
      <table className="w-full text-sm">
        <thead className="bg-[var(--color-surface-muted)]/50 text-[11px] uppercase tracking-wider text-[var(--color-fg-muted)]">
          <tr>
            <th className="px-4 py-2.5 text-left font-medium">Name</th>
            <th className="w-12 px-2 py-2.5"></th>
          </tr>
        </thead>
        <tbody>
          {items.map((l) => (
            <ListRow key={l.id} list={l} />
          ))}
        </tbody>
      </table>
    </div>
  );
}

function ListRow({ list }: { list: DocList }) {
  const q = useQueryClient();
  const del = useMutation({
    mutationFn: () => api.deleteList(list.id),
    onSuccess: () => {
      toast.success("List deleted.");
      q.invalidateQueries({ queryKey: queryKeys.lists() });
    },
    onError: (e) => toast.error(errorMessage(e, "Delete failed")),
  });
  return (
    <tr className="group border-t border-[var(--color-border)] transition-colors hover:bg-[var(--color-surface-muted)]/40">
      <td className="px-4 py-3">
        <Link
          to={`/app/lists/${list.id}`}
          className="flex items-center gap-2 font-medium hover:underline"
        >
          <List className="h-4 w-4 text-[var(--color-fg-muted)]" />
          {list.name}
        </Link>
      </td>
      <td className="px-2 py-3">
        <AlertDialog>
          <AlertDialogTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              className="opacity-0 transition-opacity group-hover:opacity-100"
              aria-label="Delete list"
            >
              <Trash2 className="h-3.5 w-3.5 text-[var(--color-danger)]" />
            </Button>
          </AlertDialogTrigger>
          <AlertDialogContent>
            <AlertDialogTitle>Delete "{list.name}"?</AlertDialogTitle>
              <AlertDialogDescription>
                The list will be deleted. Documents inside it are not affected.
              </AlertDialogDescription>
            <AlertDialogFooter>
              <AlertDialogCancel>Cancel</AlertDialogCancel>
              <AlertDialogAction
                onClick={() => del.mutate()}
                className="bg-[var(--color-danger)] text-white hover:bg-[var(--color-danger)]/90"
              >
                Delete
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </td>
    </tr>
  );
}

function CreateListButton() {
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const q = useQueryClient();
  const create = useMutation({
    mutationFn: () => api.createList(name.trim()),
    onSuccess: () => {
      toast.success("List created.");
      q.invalidateQueries({ queryKey: queryKeys.lists() });
      setOpen(false);
      setName("");
    },
    onError: (e) => toast.error(errorMessage(e, "Could not create list")),
  });
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button>
          <Plus className="mr-1.5 h-4 w-4" />
          New list
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>New list</DialogTitle>
        </DialogHeader>
        <form
          className="space-y-3"
          onSubmit={(e) => {
            e.preventDefault();
            if (name.trim()) create.mutate();
          }}
        >
          <Input
            placeholder="e.g. Tech Specs, To Read…"
            value={name}
            onChange={(e) => setName(e.target.value)}
            autoFocus
          />
          <DialogFooter>
            <Button
              type="button"
              variant="ghost"
              onClick={() => setOpen(false)}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={!name.trim() || create.isPending}>
              {create.isPending ? "Creating…" : "Create"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

// Re-exported so documents.tsx can use it without importing the whole page
export function AddToListDialog({
  documentId,
  trigger,
}: {
  documentId: string;
  trigger: React.ReactNode;
}) {
  const [open, setOpen] = useState(false);
  const [newListName, setNewListName] = useState("");
  const [showNewList, setShowNewList] = useState(false);
  const q = useQueryClient();

  const lists = useQuery({
    queryKey: queryKeys.lists(),
    queryFn: api.listLists,
    enabled: open,
  });

  const add = useMutation({
    mutationFn: (listId: string) =>
      api.addDocumentToList(listId, documentId),
    onSuccess: (_, listId) => {
      toast.success("Added to list.");
      q.invalidateQueries({ queryKey: queryKeys.listDocuments(listId) });
      setOpen(false);
    },
    onError: (e) => toast.error(errorMessage(e, "Could not add to list")),
  });

  const createAndAdd = useMutation({
    mutationFn: async () => {
      const list = await api.createList(newListName.trim());
      await api.addDocumentToList(list.id, documentId);
      return list;
    },
    onSuccess: (list) => {
      toast.success(`Added to "${list.name}".`);
      q.invalidateQueries({ queryKey: queryKeys.lists() });
      q.invalidateQueries({ queryKey: queryKeys.listDocuments(list.id) });
      setOpen(false);
      setNewListName("");
      setShowNewList(false);
    },
    onError: (e) => toast.error(errorMessage(e, "Could not add to list")),
  });

  const items = lists.data?.items || [];

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>{trigger}</DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Add to list</DialogTitle>
        </DialogHeader>
        <div className="space-y-2">
          {lists.isLoading && (
            <p className="text-sm text-[var(--color-fg-muted)]">Loading…</p>
          )}
          {items.map((l) => (
            <button
              key={l.id}
              className="flex w-full items-center gap-2 rounded-md px-3 py-2 text-sm hover:bg-[var(--color-surface-muted)] text-left"
              onClick={() => add.mutate(l.id)}
              disabled={add.isPending}
            >
              <List className="h-4 w-4 text-[var(--color-fg-muted)]" />
              {l.name}
            </button>
          ))}
          {!showNewList ? (
            <button
              className="flex w-full items-center gap-2 rounded-md px-3 py-2 text-sm text-[var(--color-accent)] hover:bg-[var(--color-surface-muted)] text-left"
              onClick={() => setShowNewList(true)}
            >
              <Plus className="h-4 w-4" />
              New list…
            </button>
          ) : (
            <form
              className="flex gap-2"
              onSubmit={(e) => {
                e.preventDefault();
                if (newListName.trim()) createAndAdd.mutate();
              }}
            >
              <Input
                placeholder="List name"
                value={newListName}
                onChange={(e) => setNewListName(e.target.value)}
                autoFocus
                className="flex-1"
              />
              <Button
                type="submit"
                size="sm"
                disabled={!newListName.trim() || createAndAdd.isPending}
              >
                {createAndAdd.isPending ? "…" : "Create & add"}
              </Button>
            </form>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
