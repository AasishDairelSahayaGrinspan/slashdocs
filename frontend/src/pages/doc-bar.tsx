import { Link, useNavigate } from "react-router-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { errorMessage } from "@/lib/errors";
import {
  ArrowLeft,
  Check,
  ExternalLink,
  GitBranch,
  MoreHorizontal,
  Share2,
  Trash2,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Dropdown,
  DropdownContent,
  DropdownItem,
  DropdownSeparator,
  DropdownTrigger,
} from "@/components/ui/dropdown";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Sheet, SheetTrigger } from "@/components/ui/sheet";
import { DeleteDocumentDialog } from "@/components/delete-document-dialog";
import { api, type Principal, type Version } from "@/api";
import { queryKeys } from "@/lib/queryKeys";
import { ShareSheet } from "./doc-share-sheet";
import { UploadVersionItem } from "./doc-upload-version";

export function DocBar({
  title,
  docId,
  owner,
  principal,
  current,
  latest,
  versions,
  onSelectVersion,
}: {
  title: string;
  docId: string;
  owner?: Principal;
  principal?: Principal;
  current: string;
  latest: string;
  versions: Version[];
  onSelectVersion: (versionID: string) => void;
}) {
  const nav = useNavigate();
  const q = useQueryClient();
  const currentVersion = versions.find((v) => v.id === current);
  const isViewingOlder = !!current && !!latest && current !== latest;
  const canManage = samePrincipal(principal, owner);
  const del = useMutation({
    mutationFn: () => api.deleteDocument(docId),
    onSuccess: () => {
      toast.success("Document deleted.");
      q.invalidateQueries({ queryKey: queryKeys.documents() });
      nav("/app/documents");
    },
    onError: (e) =>
      toast.error(errorMessage(e, "Delete failed")),
  });
  return (
    <div className="flex h-14 items-center gap-3 border-b border-[var(--color-border)] bg-[var(--color-surface)] px-4">
      <Button asChild variant="ghost" size="icon">
        <Link to="/app/documents" aria-label="Back to documents">
          <ArrowLeft className="h-4 w-4" />
        </Link>
      </Button>
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm font-semibold">{title}</div>
        {isViewingOlder && currentVersion ? (
          <div className="truncate text-[11px] text-amber-600 dark:text-amber-300">
            {`Viewing older version - version ${currentVersion.number}`}
          </div>
        ) : null}
      </div>
      {isViewingOlder ? (
        <Button variant="secondary" size="sm" onClick={() => onSelectVersion(latest)}>
          Back to latest
        </Button>
      ) : null}
      <VersionHistoryDialog
        current={current}
        latest={latest}
        versions={versions}
        onSelectVersion={onSelectVersion}
      />
      {canManage ? (
        <Sheet>
          <SheetTrigger asChild>
            <Button variant="outline" size="sm">
              <Share2 className="h-4 w-4" /> Share
            </Button>
          </SheetTrigger>
          <ShareSheet docId={docId} />
        </Sheet>
      ) : null}
      <Dropdown>
        <DropdownTrigger asChild>
          <Button variant="ghost" size="icon" aria-label="More">
            <MoreHorizontal className="h-4 w-4" />
          </Button>
        </DropdownTrigger>
        <DropdownContent>
          <DropdownItem
            onSelect={() =>
              window.open(`/v1/documents/${docId}/html`, "_blank")
            }
          >
            <ExternalLink className="h-4 w-4" /> Open raw HTML
          </DropdownItem>
          {canManage ? (
            <>
              <UploadVersionItem docId={docId} baseVersion={latest || current} />
              <DropdownSeparator />
              <DeleteDocumentDialog
                title={title}
                onConfirm={() => del.mutate()}
                trigger={
                  <DropdownItem
                    onSelect={(e) => e.preventDefault()}
                    className="text-[var(--color-danger)] data-[highlighted]:bg-red-50 dark:data-[highlighted]:bg-red-950/40"
                  >
                    <Trash2 className="h-4 w-4" /> Delete document
                  </DropdownItem>
                }
              />
            </>
          ) : null}
        </DropdownContent>
      </Dropdown>
    </div>
  );
}

function samePrincipal(a?: Principal, b?: Principal) {
  return !!a && !!b && a.type === b.type && a.id === b.id;
}

function VersionHistoryDialog({
  current,
  latest,
  versions,
  onSelectVersion,
}: {
  current: string;
  latest: string;
  versions: Version[];
  onSelectVersion: (versionID: string) => void;
}) {
  const sorted = [...versions].sort((a, b) => b.number - a.number);
  const currentVersion = versions.find((v) => v.id === current);
  return (
    <Dialog>
      <DialogTrigger asChild>
        <Button variant="outline" size="sm" className="hidden sm:inline-flex">
          <GitBranch className="h-4 w-4" />
          {currentVersion ? `v${currentVersion.number}` : "Versions"}
        </Button>
      </DialogTrigger>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>Version history</DialogTitle>
          <DialogDescription>
            Immutable HTML revisions for this document. Open any version to
            inspect the exact HTML your reviewers saw.
          </DialogDescription>
        </DialogHeader>
        {sorted.length === 0 ? (
          <div className="rounded-lg border border-[var(--color-border)] p-4 text-sm text-[var(--color-fg-muted)]">
            No versions available yet.
          </div>
        ) : (
          <div className="max-h-[60vh] space-y-2 overflow-auto pr-1">
            {sorted.map((v) => (
              <button
                key={v.id}
                type="button"
                onClick={() => onSelectVersion(v.id)}
                className={`flex w-full gap-3 rounded-xl border border-[var(--color-border)] p-4 text-left transition-colors hover:bg-[var(--color-surface-muted)]/60 ${
                  v.id === current
                    ? "bg-[var(--color-accent-muted)]/35"
                    : "bg-[var(--color-surface)]"
                }`}
                aria-current={v.id === current ? "true" : undefined}
              >
                <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-[var(--color-accent-muted)] text-sm font-semibold text-[var(--color-accent)]">
                  {v.number}
                </div>
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <div className="font-medium">Version {v.number}</div>
                    {v.id === current ? (
                      <Badge variant="accent">
                        <Check className="h-3 w-3" /> Viewing
                      </Badge>
                    ) : null}
                    {v.id === latest ? <Badge variant="muted">Latest</Badge> : null}
                  </div>
                  <div className="mt-1 text-sm text-[var(--color-fg-muted)]">
                    {v.change_summary ||
                      (v.number === 1 ? "Initial version" : "No change summary")}
                  </div>
                  <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-[var(--color-fg-muted)]">
                    <span>Created by {principalLabel(v.created_by)}</span>
                    <span className="font-mono">{v.id}</span>
                  </div>
                </div>
              </button>
            ))}
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}

function principalLabel(p?: Principal) {
  if (!p) return "Unknown";
  if (p.name) return p.name;
  if (p.email) return p.email;
  if (p.type === "service_account") return p.id || "Service account";
  return p.id || "Unknown";
}
