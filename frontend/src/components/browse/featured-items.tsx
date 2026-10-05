import { useId } from "react";
import { Link } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { FileText, Folder as FolderIcon, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { api, docID, docTitle, type FeaturedSection, type FolderEntry } from "@/api";
import { errorMessage } from "@/lib/errors";
import { principalLabel } from "@/lib/principals";
import { queryKeys } from "@/lib/queryKeys";

// FeaturedItems shows the sections of documents and folders that the server
// features for every user, in the configured order.
export function FeaturedItems() {
  const featured = useQuery({
    queryKey: queryKeys.featured(),
    queryFn: api.listFeatured,
  });
  return (featured.data?.items || []).map((section) => (
    <FeaturedRow key={section.title} section={section} />
  ));
}

function FeaturedRow({ section }: { section: FeaturedSection }) {
  const headingID = useId();
  return (
    <section aria-labelledby={headingID} className="mb-8">
      <h2 id={headingID} className="text-sm font-semibold">
        {section.title}
      </h2>
      {section.description && (
        <p className="mt-0.5 text-xs text-[var(--color-fg-muted)]">
          {section.description}
        </p>
      )}
      <div className="stagger-grid mt-3 grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {section.items.map((item) => (
          <FeaturedCard
            key={item.folder?.id || docID(item.document!)}
            item={item}
            hideable={!section.permanent}
          />
        ))}
      </div>
    </section>
  );
}

function FeaturedCard({ item, hideable }: { item: FolderEntry; hideable: boolean }) {
  const isFolder = item.type === "folder";
  const id = item.folder?.id || docID(item.document!);
  const title = item.folder?.title || docTitle(item.document!);
  const qc = useQueryClient();
  const hide = useMutation({
    mutationFn: () => api.hideFeatured(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.featured() }),
    onError: (e) => toast.error(errorMessage(e, "Could not hide")),
  });
  const Icon = isFolder ? FolderIcon : FileText;

  return (
    <div className="group relative flex items-center rounded-[14px] border border-[var(--color-border)] bg-[var(--color-surface)] transition-[background-color,transform,box-shadow] duration-200 ease-out hover:-translate-y-0.5 hover:bg-[var(--color-surface-muted)] hover:shadow-md">
      <Link
        to={isFolder ? `/app/folders/${id}` : `/app/d/${id}`}
        className={`flex min-w-0 flex-1 items-center gap-3 p-2 ${hideable ? "pr-12" : ""}`}
      >
        <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-[6px] bg-[var(--color-accent-muted)] text-[var(--color-accent)]">
          <Icon className="h-[18px] w-[18px]" />
        </span>
        <span className="min-w-0">
          <span className="block truncate text-sm font-medium">{title}</span>
          <span className="block truncate text-xs text-[var(--color-fg-muted)]">
            {isFolder ? "Folder" : "Document"} ·{" "}
            {principalLabel(item.folder?.owner || item.document?.owner)}
          </span>
        </span>
      </Link>
      {hideable && (
        <Button
          variant="ghost"
          size="icon"
          aria-label="Hide"
          className="absolute right-1 h-10 w-10 cursor-pointer opacity-0 transition-opacity group-hover:opacity-100 focus-visible:opacity-100"
          disabled={hide.isPending}
          onClick={() => hide.mutate()}
        >
          <X className="h-3.5 w-3.5" />
        </Button>
      )}
    </div>
  );
}
