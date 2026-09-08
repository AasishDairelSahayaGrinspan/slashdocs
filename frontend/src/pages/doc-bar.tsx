import { Tooltip } from "@/components/ui/tooltip";
import { useRef, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { errorMessage } from "@/lib/errors";
import {
  ArrowLeft,
  Check,
  ExternalLink,
  FileDown,
  FolderIcon,
  ArrowUpToLine,
  Maximize2,
  GitBranch,
  MoreHorizontal,
  PanelRightClose,
  PanelRightOpen,
  Share2,
  Tag,
  Trash2,
} from "lucide-react";
import { AccountMenu } from "@/components/layout/topbar";
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
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Sheet, SheetTrigger } from "@/components/ui/sheet";
import { DeleteDocumentDialog } from "@/components/delete-document-dialog";
import { api, type Principal, type Version } from "@/api";
import { canManageSharing, ROLE_RANK } from "@/lib/constants";
import { formatTimestamp } from "@/lib/format-date";
import { queryKeys } from "@/lib/queryKeys";
import { samePrincipal } from "@/lib/principals";
import { ShareSheet } from "./doc-share-sheet";
import { useDoc } from "./doc-context";
import { UploadVersionItem } from "./doc-upload-version";

export function DocBar({
  title,
  docId,
  owner,
  principal,
  accountUser,
  parentFolder,
  current,
  latest,
  versions,
  labels,
  onSelectVersion,
  commentsOpen,
  onToggleComments,
  shareOpen,
  onShareOpenChange,
}: {
  title: string;
  docId: string;
  owner?: Principal;
  principal?: Principal;
  accountUser?: Principal;
  parentFolder?: { id: string; title: string };
  current: string;
  latest: string;
  versions: Version[];
  labels: string[];
  onSelectVersion: (versionID: string) => void;
  commentsOpen: boolean;
  onToggleComments: () => void;
  shareOpen: boolean;
  onShareOpenChange: (open: boolean) => void;
}) {
  const nav = useNavigate();
  const q = useQueryClient();
  const backTo = parentFolder ? `/app/folders/${parentFolder.id}` : "/app/documents";
  const config = useQuery({ queryKey: queryKeys.config(), queryFn: api.getConfig });
  const currentVersion = versions.find((v) => v.id === current);
  const isViewingOlder = !!current && !!latest && current !== latest;
  const canManage = samePrincipal(principal, owner);
  const access = useQuery({
    queryKey: queryKeys.documentAccess(docId),
    queryFn: () => api.documentAccess(docId),
    enabled: !!docId && !!principal && !canManage,
    staleTime: 30_000,
  });
  const currentAccessRole = access.data?.current_role;
  const canManageAccess = canManage || canManageSharing(currentAccessRole);
  const canEdit =
    canManage || (ROLE_RANK[currentAccessRole || ""] || 0) >= ROLE_RANK.editor;
  const shareFrameFocusRef = useRef<HTMLIFrameElement | null>(null);
  const del = useMutation({
    mutationFn: () => api.deleteDocument(docId),
    onSuccess: () => {
      toast.success("Document deleted.");
      q.invalidateQueries({ queryKey: queryKeys.documents() });
      nav(backTo);
    },
    onError: (e) => toast.error(errorMessage(e, "Delete failed")),
  });
  const { printDocument: exportPDF } = useDoc();
  return (
    <header className="flex h-14 items-center gap-1 border-b border-[var(--color-border)] bg-[var(--color-surface)] px-2 sm:gap-2 sm:px-3">
      <Button asChild variant="ghost" size="icon" className="shrink-0">
        <Link
          to={backTo}
          title={parentFolder ? "Back to folder" : "Back to documents"}
          aria-label={parentFolder ? "Back to folder" : "Back to documents"}
        >
          <ArrowLeft className="h-4 w-4" />
        </Link>
      </Button>
      <Tooltip label="All documents">
        <Link
          to="/app/documents"
          aria-label="aidocs home"
          title="All documents"
          className="hidden h-10 w-10 shrink-0 items-center justify-center rounded-[10px] hover:bg-[var(--color-surface-muted)] sm:flex"
        >
          <img src="/favicon.svg" alt="" className="h-8 w-8" />
        </Link>
      </Tooltip>
      <div className="flex min-w-0 flex-1 items-center gap-2">
        {parentFolder ? (
          <>
            <Link
              to={`/app/folders/${parentFolder.id}`}
              title={parentFolder.title}
              aria-label={`Folder: ${parentFolder.title}`}
              className="hidden h-10 max-w-32 shrink items-center gap-1 text-sm text-[var(--color-fg-muted)] hover:text-[var(--color-fg)] lg:flex"
            >
              <FolderIcon className="h-4 w-4 shrink-0" />
              <span className="truncate">{parentFolder.title}</span>
            </Link>
            <span
              aria-hidden="true"
              className="hidden text-[var(--color-fg-muted)] lg:inline"
            >
              /
            </span>
          </>
        ) : null}
        <div title={title} className="truncate text-sm font-semibold">
          {title}
        </div>
        {currentVersion && (
          <span
            className={`hidden shrink-0 text-xs tabular-nums sm:inline ${isViewingOlder ? "text-amber-600 dark:text-amber-300" : "text-[var(--color-fg-muted)]"}`}
            title={isViewingOlder ? "Viewing an older version" : "Latest version"}
          >
            v{currentVersion.number}
            {isViewingOlder ? " · older" : ""}
          </span>
        )}
      </div>
      {isViewingOlder ? (
        <Button
          variant="ghost"
          size="icon"
          title="Back to latest version"
          aria-label="Back to latest version"
          className="shrink-0"
          onClick={() => onSelectVersion(latest)}
        >
          <ArrowUpToLine className="h-4 w-4" />
        </Button>
      ) : null}
      <VersionHistoryDialog
        current={current}
        latest={latest}
        versions={versions}
        onSelectVersion={onSelectVersion}
      />
      {config.data?.labels_enabled === true ? (
        <LabelsDialog docId={docId} labels={labels} canEdit={canEdit} />
      ) : null}
      <Button
        asChild
        variant="ghost"
        size="icon"
        className="hidden shrink-0 sm:inline-flex"
      >
        <Link
          to={`/app/d/${docId}?fullscreen=1&version=${current}`}
          title="Full screen"
          aria-label="Full screen"
        >
          <Maximize2 className="h-4 w-4" />
        </Link>
      </Button>
      <Sheet modal={false} open={shareOpen} onOpenChange={onShareOpenChange}>
        <SheetTrigger asChild>
          <Button
            variant="ghost"
            size="icon"
            className="shrink-0"
            title="Share"
            aria-label="Share"
            onPointerDown={() => {
              const active = document.activeElement;
              shareFrameFocusRef.current =
                active instanceof HTMLIFrameElement &&
                active.title === "Rendered document"
                  ? active
                  : null;
            }}
          >
            <Share2 className="h-4 w-4" />
          </Button>
        </SheetTrigger>
        <ShareSheet
          docId={docId}
          canManageAccess={canManageAccess}
          isCanonicalOwner={canManage}
          onOpenAutoFocus={(event) => {
            const frame = shareFrameFocusRef.current;
            shareFrameFocusRef.current = null;
            if (!frame?.isConnected) return;
            event.preventDefault();
            frame.focus();
          }}
          onInteractOutside={(event) => {
            const target = event.target;
            if (
              target instanceof HTMLIFrameElement &&
              target.title === "Rendered document"
            ) {
              event.preventDefault();
            }
          }}
        />
      </Sheet>
      <Button
        variant="ghost"
        size="icon"
        onClick={onToggleComments}
        aria-label={commentsOpen ? "Hide comments" : "Show comments"}
        tooltip={commentsOpen ? false : "Show comments"}
        aria-pressed={commentsOpen}
        className="shrink-0"
      >
        {commentsOpen ? (
          <PanelRightClose className="h-4 w-4" />
        ) : (
          <PanelRightOpen className="h-4 w-4" />
        )}
      </Button>
      <Dropdown>
        <DropdownTrigger asChild>
          <Button
            variant="ghost"
            size="icon"
            className="shrink-0"
            aria-label="More"
            title="More document actions"
          >
            <MoreHorizontal className="h-4 w-4" />
          </Button>
        </DropdownTrigger>
        <DropdownContent>
          <DropdownItem
            className="sm:hidden"
            onSelect={() => nav(`/app/d/${docId}?fullscreen=1&version=${current}`)}
          >
            <Maximize2 className="h-4 w-4" /> Full screen
          </DropdownItem>
          <DropdownItem
            onSelect={() =>
              window.open(
                `/v1/documents/${docId}/versions/${current}/html`,
                "_blank",
                "noopener,noreferrer",
              )
            }
          >
            <ExternalLink className="h-4 w-4" /> Open raw HTML
          </DropdownItem>
          <DropdownItem onSelect={() => void exportPDF()}>
            <FileDown className="h-4 w-4" /> Export as PDF
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
      {accountUser && (
        <div className="shrink-0 border-l border-[var(--color-border)] pl-2">
          <AccountMenu user={accountUser} />
        </div>
      )}
    </header>
  );
}

function LabelsDialog({
  docId,
  labels,
  canEdit,
}: {
  docId: string;
  labels: string[];
  canEdit: boolean;
}) {
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState(labels.join(", "));
  const qc = useQueryClient();
  const save = useMutation({
    mutationFn: () =>
      api.replaceDocumentLabels(
        docId,
        draft
          .split(",")
          .map((label) => label.trim())
          .filter(Boolean),
      ),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.document(docId) });
      qc.invalidateQueries({ queryKey: queryKeys.documents() });
      setOpen(false);
      toast.success("Labels updated.");
    },
    onError: (e) => toast.error(errorMessage(e, "Could not update labels")),
  });
  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        setOpen(next);
        if (next) setDraft(labels.join(", "));
      }}
    >
      <DialogTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          className="hidden shrink-0 lg:inline-flex"
          aria-label="Document labels"
          title={labels.length ? `Labels: ${labels.join(", ")}` : "Document labels"}
        >
          <Tag className="h-4 w-4" />
        </Button>
      </DialogTrigger>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Document labels</DialogTitle>
          <DialogDescription>
            Add comma-separated labels. Labels are normalized to lowercase and can be
            used from the documents search page.
          </DialogDescription>
        </DialogHeader>
        <Input
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
          placeholder="runbook, payments, onboarding"
          disabled={!canEdit}
        />
        <DialogFooter>
          {canEdit ? (
            <Button onClick={() => save.mutate()} disabled={save.isPending}>
              Save labels
            </Button>
          ) : null}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
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
  return (
    <Dialog>
      <DialogTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          className="hidden shrink-0 sm:inline-flex"
          aria-label="Version history"
          title="Version history"
        >
          <GitBranch className="h-4 w-4" />
        </Button>
      </DialogTrigger>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>Version history</DialogTitle>
          <DialogDescription>
            Immutable HTML revisions for this document. Open any version to inspect the
            exact HTML your reviewers saw.
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
                    {v.created_at ? <span>{formatTimestamp(v.created_at)}</span> : null}
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
