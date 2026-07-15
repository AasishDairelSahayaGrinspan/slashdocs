import { useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowLeft,
  Check,
  CheckCircle2,
  ExternalLink,
  FileText,
  GitMerge,
  GitPullRequest,
  Layers3,
  LockKeyhole,
  MessageSquareText,
  Move,
  Pencil,
  Plus,
  RotateCcw,
  ShieldAlert,
  Trash2,
  X,
  XCircle,
} from "lucide-react";
import { toast } from "sonner";
import {
  api,
  APIError,
  type Principal,
  type PullRequestChange,
  type PullRequestChangeInput,
  type PullRequestReviewState,
} from "@/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label, Textarea } from "@/components/ui/input";
import { Center, EmptyState, Skeleton } from "@/components/ui/misc";
import { errorMessage } from "@/lib/errors";
import { effectiveRoleForPrincipal } from "@/lib/constants";
import { queryKeys } from "@/lib/queryKeys";
import {
  buildDocumentTextDiff,
  type DocumentTextDiff,
  type TextDiffLine,
} from "@/lib/document-text-diff";
import { useTitle } from "@/lib/use-title";
import {
  formatRelativeDate,
  latestReviewCounts,
  principalLabel,
  PullStatusBadge,
} from "./repository";

export function PullRequestPage() {
  const { id = "", number = "" } = useParams();
  const qc = useQueryClient();
  const repository = useQuery({
    queryKey: queryKeys.repository(id),
    queryFn: () => api.getRepository(id),
    enabled: !!id,
    meta: { suppressGlobalError: true },
  });
  const pull = useQuery({
    queryKey: queryKeys.pullRequest(id, number),
    queryFn: () => api.getPullRequest(id, number),
    enabled: !!id && !!number,
    meta: { suppressGlobalError: true },
  });
  const me = useQuery({ queryKey: queryKeys.me(), queryFn: api.me });
  const rootFolderId = repository.data?.root_folder_id || "";
  const access = useQuery({
    queryKey: queryKeys.folderAccess(rootFolderId),
    queryFn: () => api.folderAccess(rootFolderId),
    enabled: !!rootFolderId && !!me.data?.principal,
  });
  const [reviewBody, setReviewBody] = useState("");
  const [reviewError, setReviewError] = useState<string>();
  const [patchsetOpen, setPatchsetOpen] = useState(false);
  useTitle(pull.data ? `${pull.data.title} · PR #${pull.data.number}` : "Pull request");

  const invalidatePullRequest = () => {
    qc.invalidateQueries({ queryKey: queryKeys.pullRequest(id, number) });
    qc.invalidateQueries({ queryKey: queryKeys.pullRequests(id) });
  };
  const review = useMutation({
    mutationFn: async (state: PullRequestReviewState) => ({
      state,
      result: await api.reviewPullRequest(id, number, state, reviewBody.trim()),
    }),
    onSuccess: ({ state }) => {
      setReviewBody("");
      setReviewError(undefined);
      invalidatePullRequest();
      toast.success(
        state === "approved" ? "Pull request approved." : "Changes requested.",
      );
    },
    onError: (error) => setReviewError(errorMessage(error, "Could not submit review")),
  });
  const merge = useMutation({
    mutationFn: () => api.mergePullRequest(id, number),
    onSuccess: (result) => {
      invalidatePullRequest();
      qc.invalidateQueries({ queryKey: queryKeys.repository(id) });
      qc.invalidateQueries({ queryKey: queryKeys.repositoryDocuments(id) });
      const changedDocumentIds = new Set(
        [
          result.document_id,
          ...(result.changes || []).map((change) => change.document_id || ""),
        ].filter(Boolean),
      );
      for (const documentId of changedDocumentIds) {
        qc.invalidateQueries({ queryKey: queryKeys.document(documentId) });
        qc.invalidateQueries({ queryKey: queryKeys.versions(documentId) });
      }
      if (repository.data?.root_folder_id) {
        qc.invalidateQueries({
          queryKey: queryKeys.folders(repository.data.root_folder_id),
        });
      }
      toast.success(`Pull request #${result.number} merged.`);
    },
    onError: (error) =>
      toast.error(errorMessage(error, "Could not merge pull request")),
  });
  const close = useMutation({
    mutationFn: () => api.closePullRequest(id, number),
    onSuccess: () => {
      invalidatePullRequest();
      toast.success("Pull request closed.");
    },
    onError: (error) =>
      toast.error(errorMessage(error, "Could not close pull request")),
  });
  const rebase = useMutation({
    mutationFn: () => api.rebasePullRequest(id, number),
    onSuccess: (result) => {
      invalidatePullRequest();
      toast.success(`Rebased onto patchset ${result.patchset || 1}.`);
    },
    onError: (error) =>
      toast.error(errorMessage(error, "Could not rebase pull request")),
  });

  if (pull.isLoading || repository.isLoading) {
    return (
      <div className="mx-auto max-w-7xl px-4 py-6 sm:px-6 sm:py-10">
        <Skeleton className="mb-3 h-9 w-3/4 max-w-2xl" />
        <Skeleton className="mb-7 h-5 w-80" />
        <div className="grid gap-4 lg:grid-cols-2">
          <Skeleton className="h-[32rem] w-full" />
          <Skeleton className="h-[32rem] w-full" />
        </div>
      </div>
    );
  }
  if (pull.error || !pull.data) {
    const denied = pull.error instanceof APIError && pull.error.status === 403;
    const missing = pull.error instanceof APIError && pull.error.status === 404;
    if (!denied && !missing) {
      return (
        <Center>
          <EmptyState
            icon={<GitPullRequest className="h-5 w-5" />}
            title="Could not load this pull request"
            description="Check your connection and try again."
            action={
              <Button variant="outline" onClick={() => pull.refetch()}>
                Try again
              </Button>
            }
          />
        </Center>
      );
    }
    return (
      <Center>
        <EmptyState
          icon={
            denied ? (
              <LockKeyhole className="h-5 w-5" />
            ) : (
              <GitPullRequest className="h-5 w-5" />
            )
          }
          title={denied ? "You don't have access" : "Pull request not found"}
          description={
            denied
              ? "Ask a repository owner to grant you access."
              : "It may not exist, or the link may be incorrect."
          }
          action={
            <Button asChild variant="outline">
              <Link to={`/app/repos/${id}?tab=pulls`}>Back to pull requests</Link>
            </Button>
          }
        />
      </Center>
    );
  }

  const pr = pull.data;
  const changeItems = pr.changes || [];
  const isChangeSet = changeItems.length > 0;
  const primaryChange = changeItems.find(
    (change) => change.document_id && change.head_version_id,
  );
  const proposedDocumentId = primaryChange?.document_id || pr.document_id;
  const proposedVersionId = primaryChange?.head_version_id || pr.head_version_id;
  const reviews = [...(pr.reviews || [])].sort((a, b) =>
    (a.submitted_at || "").localeCompare(b.submitted_at || ""),
  );
  const counts = latestReviewCounts(pr);
  const required = pr.required_approvals ?? repository.data?.required_approvals ?? 0;
  const open = pr.status === "open";
  const currentRole = effectiveRoleForPrincipal(
    access.data?.items || [],
    me.data?.principal,
  );
  const createdByMe = samePrincipal(repository.data?.created_by, me.data?.principal);
  const canEdit = createdByMe || currentRole === "owner" || currentRole === "editor";
  const isAuthor = samePrincipal(pr.author, me.data?.principal);
  const permissionsLoading = me.isLoading || (!!rootFolderId && access.isLoading);
  const hasRepositoryAccess = !!repository.data;

  return (
    <div className="mx-auto max-w-7xl px-4 py-6 sm:px-6 sm:py-10">
      <Link
        to={
          hasRepositoryAccess
            ? `/app/repos/${id}?tab=pulls`
            : `/app/d/${pr.document_id}`
        }
        className="mb-5 inline-flex items-center gap-1.5 text-sm text-[var(--color-fg-muted)] hover:text-[var(--color-fg)]"
      >
        <ArrowLeft className="h-4 w-4" />
        {hasRepositoryAccess
          ? `${repository.data?.name || "Repository"} pull requests`
          : "Back to document"}
      </Link>

      <div className="flex flex-col gap-4 border-b border-[var(--color-border)] pb-6 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="text-2xl font-semibold tracking-tight">{pr.title}</h1>
            <span className="text-xl font-normal text-[var(--color-fg-muted)]">
              #{pr.number}
            </span>
            {isChangeSet ? (
              <Badge variant="muted">
                <Layers3 className="h-3 w-3" /> {changeItems.length} changes
              </Badge>
            ) : null}
            {pr.patchset ? (
              <Badge variant="accent">Patchset {pr.patchset}</Badge>
            ) : null}
          </div>
          <div className="mt-3 flex flex-wrap items-center gap-2 text-sm text-[var(--color-fg-muted)]">
            <PullStatusBadge status={pr.status} />
            <span>
              <PrincipalProfileLink principal={pr.author} /> opened this{" "}
              {formatRelativeDate(pr.created_at)}
            </span>
            {pr.merged_by ? (
              <span>
                · merged by <PrincipalProfileLink principal={pr.merged_by} />
              </span>
            ) : null}
          </div>
        </div>
        {proposedDocumentId && proposedVersionId ? (
          <Button asChild variant="outline" className="shrink-0">
            <Link
              to={`/app/d/${proposedDocumentId}?version=${proposedVersionId}${hasRepositoryAccess ? `&repo=${encodeURIComponent(id)}` : ""}`}
            >
              <ExternalLink className="h-4 w-4" />
              {isChangeSet ? "Open proposed document" : "Open proposed version"}
            </Link>
          </Button>
        ) : null}
      </div>

      <div className="mt-6 grid gap-6 xl:grid-cols-[minmax(0,1fr)_340px]">
        <main className="min-w-0 space-y-6">
          {pr.body || pr.change_summary ? (
            <Card>
              <CardHeader>
                <CardTitle>Proposal</CardTitle>
              </CardHeader>
              <CardContent className="space-y-3">
                {pr.body ? (
                  <p className="whitespace-pre-wrap text-sm leading-6">{pr.body}</p>
                ) : null}
                {pr.change_summary ? (
                  <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-muted)]/55 px-3 py-2 text-sm">
                    <span className="text-xs font-medium uppercase tracking-wider text-[var(--color-fg-muted)]">
                      Version summary
                    </span>
                    <div className="mt-1">{pr.change_summary}</div>
                  </div>
                ) : null}
              </CardContent>
            </Card>
          ) : null}

          {isChangeSet ? (
            <PullRequestChangeSet changes={changeItems} repositoryId={id} />
          ) : (
            <TextVersionDiff
              documentId={pr.document_id}
              baseVersionId={pr.base_version_id}
              proposedVersionId={pr.head_version_id}
            />
          )}

          <section>
            <div className="mb-3 flex items-center gap-2">
              <MessageSquareText className="h-4 w-4" />
              <h2 className="font-semibold">Review timeline</h2>
            </div>
            {reviews.length === 0 ? (
              <EmptyState
                className="p-7"
                title="No reviews yet"
                description="Approvals and change requests will appear here."
              />
            ) : (
              <div className="space-y-3">
                {reviews.map((item) => (
                  <div
                    key={item.id}
                    className="rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
                  >
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <div className="flex items-center gap-2 text-sm font-medium">
                        {item.state === "approved" ? (
                          <CheckCircle2 className="h-4 w-4 text-[var(--color-success)]" />
                        ) : (
                          <XCircle className="h-4 w-4 text-[var(--color-danger)]" />
                        )}
                        <PrincipalProfileLink principal={item.reviewer} />
                        <span className="font-normal text-[var(--color-fg-muted)]">
                          {item.state === "approved" ? "approved" : "requested changes"}
                        </span>
                      </div>
                      <span className="text-xs text-[var(--color-fg-muted)]">
                        {formatRelativeDate(item.submitted_at)}
                      </span>
                    </div>
                    {item.body ? (
                      <p className="mt-3 whitespace-pre-wrap text-sm leading-6 text-[var(--color-fg-muted)]">
                        {item.body}
                      </p>
                    ) : null}
                  </div>
                ))}
              </div>
            )}
          </section>
        </main>

        <aside className="space-y-4">
          {open && canEdit && isChangeSet ? (
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <Layers3 className="h-4 w-4" /> Patchset controls
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-2">
                <Button
                  variant="outline"
                  className="w-full"
                  disabled={rebase.isPending}
                  onClick={() => rebase.mutate()}
                >
                  <RotateCcw
                    className={`h-4 w-4 ${rebase.isPending ? "animate-spin" : ""}`}
                  />
                  {rebase.isPending ? "Rebasing…" : "Rebase onto current docs"}
                </Button>
                <Button className="w-full" onClick={() => setPatchsetOpen(true)}>
                  <Plus className="h-4 w-4" /> New patchset
                </Button>
                <p className="pt-1 text-xs leading-5 text-[var(--color-fg-muted)]">
                  A new patchset replaces the proposed change bundle while preserving
                  the review timeline.
                </p>
              </CardContent>
            </Card>
          ) : null}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <ShieldAlert className="h-4 w-4" /> Merge requirements
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              <RequirementRow
                met={counts.approved >= required}
                label={`${counts.approved} of ${required} required approvals`}
              />
              <RequirementRow
                met={counts.changesRequested === 0}
                label={
                  counts.changesRequested === 0
                    ? "No active change requests"
                    : `${counts.changesRequested} reviewer${counts.changesRequested === 1 ? "" : "s"} requested changes`
                }
              />
              {open && canEdit ? (
                <div className="border-t border-[var(--color-border)] pt-3">
                  <Button
                    className="w-full"
                    disabled={!pr.mergeable || merge.isPending}
                    onClick={() => merge.mutate()}
                  >
                    <GitMerge className="h-4 w-4" />
                    {merge.isPending ? "Merging…" : "Merge pull request"}
                  </Button>
                  {!pr.mergeable ? (
                    <p className="mt-2 text-xs leading-5 text-[var(--color-fg-muted)]">
                      {mergeBlockedMessage(pr.merge_blocked_reason)}
                    </p>
                  ) : null}
                </div>
              ) : !open ? (
                <div className="rounded-lg bg-[var(--color-surface-muted)] px-3 py-2 text-sm text-[var(--color-fg-muted)]">
                  This pull request is {pr.status}.
                </div>
              ) : (
                <div className="rounded-lg bg-[var(--color-surface-muted)] px-3 py-2 text-sm text-[var(--color-fg-muted)]">
                  {permissionsLoading
                    ? "Checking repository permissions…"
                    : "Editor access is required to merge this pull request."}
                </div>
              )}
            </CardContent>
          </Card>

          {open && canEdit && !isAuthor ? (
            <Card>
              <CardHeader>
                <CardTitle>Submit a review</CardTitle>
              </CardHeader>
              <CardContent>
                <div className="space-y-1.5">
                  <Label htmlFor="review-body">Review note</Label>
                  <Textarea
                    id="review-body"
                    value={reviewBody}
                    onChange={(event) => setReviewBody(event.target.value)}
                    placeholder="Leave context for the author…"
                    rows={4}
                  />
                </div>
                {reviewError ? (
                  <p role="alert" className="mt-2 text-xs text-[var(--color-danger)]">
                    {reviewError}
                  </p>
                ) : null}
                <div className="mt-3 grid gap-2">
                  <Button
                    disabled={review.isPending}
                    onClick={() => review.mutate("approved")}
                  >
                    <Check className="h-4 w-4" /> Approve
                  </Button>
                  <Button
                    variant="outline"
                    disabled={review.isPending || !reviewBody.trim()}
                    onClick={() => review.mutate("changes_requested")}
                  >
                    <X className="h-4 w-4" /> Request changes
                  </Button>
                </div>
              </CardContent>
            </Card>
          ) : open ? (
            <Card>
              <CardContent className="pt-5 text-sm text-[var(--color-fg-muted)]">
                {permissionsLoading
                  ? "Checking repository permissions…"
                  : isAuthor
                    ? "Authors cannot approve or request changes on their own pull request."
                    : "Editor access is required to submit a review."}
              </CardContent>
            </Card>
          ) : null}

          {open && canEdit ? (
            <Button
              variant="ghost"
              className="w-full text-[var(--color-danger)]"
              disabled={close.isPending}
              onClick={() => close.mutate()}
            >
              <XCircle className="h-4 w-4" />
              {close.isPending ? "Closing…" : "Close pull request"}
            </Button>
          ) : null}
        </aside>
      </div>
      <PatchsetDialog
        open={patchsetOpen}
        onOpenChange={setPatchsetOpen}
        repositoryId={id}
        number={number}
        changes={changeItems}
        onCreated={invalidatePullRequest}
      />
    </div>
  );
}

function samePrincipal(
  a?: { type: string; id: string },
  b?: { type: string; id: string },
) {
  return !!a && !!b && a.type === b.type && a.id === b.id;
}

function mergeBlockedMessage(reason?: string) {
  if (!reason)
    return "This pull request does not currently meet the merge requirements.";
  const messages: Record<string, string> = {
    approval_required:
      "More approvals are required before this pull request can merge.",
    changes_requested: "A reviewer has requested changes.",
    stale_pull_request:
      "The base document changed. Open a fresh pull request from the latest version.",
    pull_request_closed: "Closed pull requests cannot be merged.",
  };
  return (
    messages[reason] || (reason.includes("_") ? reason.replaceAll("_", " ") : reason)
  );
}

function PullRequestChangeSet({
  changes,
  repositoryId,
}: {
  changes: PullRequestChange[];
  repositoryId: string;
}) {
  const counts = useMemo(() => {
    const result = new Map<string, number>();
    for (const change of changes) {
      result.set(change.operation, (result.get(change.operation) || 0) + 1);
    }
    return result;
  }, [changes]);
  return (
    <section>
      <div className="mb-3 flex flex-wrap items-end justify-between gap-3">
        <div>
          <div className="flex items-center gap-2">
            <Layers3 className="h-4 w-4" />
            <h2 className="font-semibold">Changed documents</h2>
          </div>
          <p className="mt-1 text-sm text-[var(--color-fg-muted)]">
            One review covers this complete document and folder change set.
          </p>
        </div>
        <div className="flex flex-wrap gap-1.5">
          {[...counts.entries()].map(([operation, count]) => (
            <Badge key={operation} variant="muted" className="capitalize">
              {count} {operation}
            </Badge>
          ))}
        </div>
      </div>
      <div className="space-y-3">
        {changes.map((change, index) => {
          const canDiff =
            change.operation === "update" &&
            !!change.document_id &&
            !!change.base_version_id &&
            !!change.head_version_id;
          const href =
            change.document_id && change.head_version_id
              ? `/app/d/${change.document_id}?version=${change.head_version_id}&repo=${encodeURIComponent(repositoryId)}`
              : undefined;
          return (
            <Card key={change.id || `${change.operation}-${index}`}>
              <CardHeader className="flex-row items-start justify-between gap-3 pb-4">
                <div className="flex min-w-0 items-start gap-3">
                  <span
                    className={`flex h-9 w-9 shrink-0 items-center justify-center rounded-lg ${changeOperationTone(change.operation)}`}
                  >
                    {changeOperationIcon(change.operation)}
                  </span>
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <CardTitle className="truncate">
                        {change.title ||
                          change.base_title ||
                          change.path ||
                          "Document change"}
                      </CardTitle>
                      <Badge variant="muted" className="capitalize">
                        {change.operation}
                      </Badge>
                    </div>
                    <ChangePath change={change} />
                  </div>
                </div>
                {href ? (
                  <Button asChild size="sm" variant="outline">
                    <Link to={href}>
                      <ExternalLink className="h-3.5 w-3.5" /> Open
                    </Link>
                  </Button>
                ) : null}
              </CardHeader>
              {canDiff ? (
                <CardContent className="border-t border-[var(--color-border)] pt-4">
                  <TextVersionDiff
                    documentId={change.document_id!}
                    baseVersionId={change.base_version_id!}
                    proposedVersionId={change.head_version_id!}
                    compact
                  />
                </CardContent>
              ) : (
                <CardContent className="border-t border-[var(--color-border)] pt-4 text-sm text-[var(--color-fg-muted)]">
                  {changeOperationDescription(change)}
                </CardContent>
              )}
            </Card>
          );
        })}
      </div>
    </section>
  );
}

function ChangePath({ change }: { change: PullRequestChange }) {
  if (change.base_path && change.path && change.base_path !== change.path) {
    return (
      <div className="mt-1 flex flex-wrap items-center gap-1.5 font-mono text-xs text-[var(--color-fg-muted)]">
        <span className="line-through">{change.base_path}</span>
        <span>→</span>
        <span>{change.path}</span>
      </div>
    );
  }
  const path = change.path || change.base_path;
  return path ? (
    <div className="mt-1 truncate font-mono text-xs text-[var(--color-fg-muted)]">
      {path}
    </div>
  ) : null;
}

function PatchsetDialog({
  open,
  onOpenChange,
  repositoryId,
  number,
  changes,
  onCreated,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  repositoryId: string;
  number: string;
  changes: PullRequestChange[];
  onCreated: () => void;
}) {
  const [summary, setSummary] = useState("");
  const initialJSON = useMemo(
    () =>
      JSON.stringify(
        changes.map((change) => ({
          operation: change.operation,
          document_id: change.document_id,
          folder_id: change.folder_id,
          base_version_id: change.base_version_id,
          title: change.title,
          path: change.path,
        })),
        null,
        2,
      ),
    [changes],
  );
  const [changeJSON, setChangeJSON] = useState(initialJSON);
  const [formError, setFormError] = useState<string>();
  const create = useMutation({
    mutationFn: () => {
      let parsed: unknown;
      try {
        parsed = JSON.parse(changeJSON);
      } catch {
        throw new Error("Changes must be valid JSON");
      }
      if (!Array.isArray(parsed) || parsed.length === 0) {
        throw new Error("Include at least one change");
      }
      return api.createPullRequestPatchset(repositoryId, number, {
        change_summary: summary.trim() || undefined,
        changes: parsed as PullRequestChangeInput[],
      });
    },
    onSuccess: (result) => {
      onCreated();
      onOpenChange(false);
      setSummary("");
      setFormError(undefined);
      toast.success(`Patchset ${result.patchset || "updated"} created.`);
    },
    onError: (error) => setFormError(errorMessage(error, "Could not create patchset")),
  });
  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        onOpenChange(next);
        if (next) setChangeJSON(initialJSON);
      }}
    >
      <DialogContent className="max-h-[90vh] max-w-2xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle>New patchset</DialogTitle>
          <DialogDescription>
            Replace the proposed bundle without opening another pull request. Structural
            changes can be edited here; document copy should normally be changed with
            Edit text.
          </DialogDescription>
        </DialogHeader>
        <form
          className="space-y-4"
          onSubmit={(event) => {
            event.preventDefault();
            setFormError(undefined);
            create.mutate();
          }}
        >
          <div className="space-y-1.5">
            <Label htmlFor="patchset-summary">Patchset summary</Label>
            <Textarea
              id="patchset-summary"
              value={summary}
              onChange={(event) => setSummary(event.target.value)}
              rows={2}
              placeholder="Address reviewer feedback"
            />
          </div>
          <details className="rounded-xl border border-[var(--color-border)] p-3">
            <summary className="cursor-pointer text-sm font-medium">
              Advanced change bundle
            </summary>
            <p className="mt-2 text-xs leading-5 text-[var(--color-fg-muted)]">
              Rename, move, create, update, or delete operations. Updates that replace
              copy can include a content_html field supplied by an integration.
            </p>
            <Textarea
              value={changeJSON}
              onChange={(event) => setChangeJSON(event.target.value)}
              rows={14}
              spellCheck={false}
              className="mt-3 font-mono text-xs"
            />
          </details>
          {formError ? (
            <p role="alert" className="text-sm text-[var(--color-danger)]">
              {formError}
            </p>
          ) : null}
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={create.isPending}>
              <Layers3 className="h-4 w-4" />
              {create.isPending ? "Creating…" : "Create patchset"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function changeOperationIcon(operation: PullRequestChange["operation"]) {
  if (operation === "delete") return <Trash2 className="h-4 w-4" />;
  if (operation === "move") return <Move className="h-4 w-4" />;
  if (operation === "create") return <Plus className="h-4 w-4" />;
  return <Pencil className="h-4 w-4" />;
}

function changeOperationTone(operation: PullRequestChange["operation"]) {
  if (operation === "delete")
    return "bg-red-50 text-red-700 dark:bg-red-950/35 dark:text-red-300";
  if (operation === "create")
    return "bg-emerald-50 text-emerald-700 dark:bg-emerald-950/35 dark:text-emerald-300";
  return "bg-[var(--color-accent-muted)] text-[var(--color-accent)]";
}

function changeOperationDescription(change: PullRequestChange) {
  if (change.operation === "create")
    return "Creates a new document in this repository.";
  if (change.operation === "delete")
    return "Removes this document when the pull request is merged.";
  if (change.operation === "rename")
    return `Renames ${change.base_title || "the document"} to ${change.title || "its proposed title"}.`;
  if (change.operation === "move")
    return `Moves this content from ${change.base_path || "its current folder"} to ${change.path || "the proposed folder"}.`;
  return "Updates this document as part of the change set.";
}

async function fetchVersionHTML(documentId: string, versionId: string) {
  const response = await fetch(
    `/v1/documents/${encodeURIComponent(documentId)}/versions/${encodeURIComponent(versionId)}/html`,
    { credentials: "include", headers: { Accept: "text/html" } },
  );
  if (!response.ok) {
    throw new Error(`Could not load version text (${response.status})`);
  }
  return response.text();
}

function TextVersionDiff({
  documentId,
  baseVersionId,
  proposedVersionId,
  compact = false,
}: {
  documentId: string;
  baseVersionId: string;
  proposedVersionId: string;
  compact?: boolean;
}) {
  const diff = useQuery<DocumentTextDiff>({
    queryKey: ["pull-request-text-diff", documentId, baseVersionId, proposedVersionId],
    queryFn: async () => {
      const [baseHTML, proposedHTML] = await Promise.all([
        fetchVersionHTML(documentId, baseVersionId),
        fetchVersionHTML(documentId, proposedVersionId),
      ]);
      return buildDocumentTextDiff(baseHTML, proposedHTML);
    },
    enabled: !!documentId && !!baseVersionId && !!proposedVersionId,
    staleTime: Number.POSITIVE_INFINITY,
    meta: { suppressGlobalError: true },
  });

  return (
    <section>
      <div
        className={`${compact ? "mb-2" : "mb-3"} flex flex-wrap items-end justify-between gap-3`}
      >
        <div>
          <div className="flex items-center gap-2">
            <FileText className="h-4 w-4" />
            <h2 className={compact ? "text-sm font-semibold" : "font-semibold"}>
              Text changes
            </h2>
          </div>
          {!compact ? (
            <p className="mt-1 text-sm text-[var(--color-fg-muted)]">
              Review readable document copy without HTML or Markdown details.
            </p>
          ) : null}
        </div>
        <div className="flex items-center gap-2">
          {diff.data ? (
            <>
              <span className="text-xs font-medium text-emerald-600 dark:text-emerald-400">
                +{diff.data.added}
              </span>
              <span className="text-xs font-medium text-red-600 dark:text-red-400">
                −{diff.data.removed}
              </span>
            </>
          ) : null}
          {!compact ? <Badge variant="muted">Text only</Badge> : null}
        </div>
      </div>
      <div className="overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)]">
        {diff.isLoading ? (
          <div
            className="space-y-px bg-[var(--color-border)]"
            aria-label="Loading text changes"
          >
            {Array.from({ length: 8 }, (_, index) => (
              <div
                key={index}
                className="flex h-9 items-center bg-[var(--color-surface)] px-4"
              >
                <Skeleton className={`h-3 ${index % 3 === 0 ? "w-2/3" : "w-4/5"}`} />
              </div>
            ))}
          </div>
        ) : diff.error ? (
          <EmptyState
            className="p-8"
            icon={<FileText className="h-5 w-5" />}
            title="Could not load text changes"
            description="The document versions could not be compared. Check your access and try again."
            action={
              <Button variant="outline" onClick={() => diff.refetch()}>
                Try again
              </Button>
            }
          />
        ) : !diff.data || (diff.data.added === 0 && diff.data.removed === 0) ? (
          <EmptyState
            className="p-8"
            icon={<FileText className="h-5 w-5" />}
            title="No readable text changes"
            description="The proposal does not change document copy. Styling and embedded behavior are intentionally omitted from this view."
          />
        ) : (
          <div
            role="table"
            aria-label={`${diff.data.added} added and ${diff.data.removed} removed text lines`}
            className={`${compact ? "max-h-[24rem]" : "max-h-[38rem]"} overflow-auto font-mono text-xs leading-5`}
          >
            {diff.data.rows.map((row, index) =>
              row.kind === "gap" ? (
                <div
                  key={`gap-${index}`}
                  role="row"
                  className="border-y border-[var(--color-border)] bg-[var(--color-surface-muted)]/70 px-4 py-1.5 text-center text-[11px] text-[var(--color-fg-muted)]"
                >
                  {row.count} unchanged line{row.count === 1 ? "" : "s"}
                </div>
              ) : (
                <TextDiffLineRow key={`${row.kind}-${index}`} line={row} />
              ),
            )}
          </div>
        )}
      </div>
    </section>
  );
}

function TextDiffLineRow({ line }: { line: TextDiffLine }) {
  const added = line.kind === "added";
  const removed = line.kind === "removed";
  const background = added
    ? "bg-emerald-500/10"
    : removed
      ? "bg-red-500/10"
      : "bg-[var(--color-surface)]";
  const prefix = added ? "+" : removed ? "−" : " ";
  return (
    <div
      role="row"
      className={`grid grid-cols-[3rem_3rem_1.5rem_minmax(0,1fr)] ${background}`}
    >
      <span
        role="cell"
        className="select-none border-r border-[var(--color-border)] px-2 py-1 text-right text-[var(--color-fg-muted)]"
      >
        {line.oldLine || ""}
      </span>
      <span
        role="cell"
        className="select-none border-r border-[var(--color-border)] px-2 py-1 text-right text-[var(--color-fg-muted)]"
      >
        {line.newLine || ""}
      </span>
      <span
        role="cell"
        className={`select-none px-2 py-1 font-semibold ${
          added
            ? "text-emerald-600 dark:text-emerald-400"
            : removed
              ? "text-red-600 dark:text-red-400"
              : "text-[var(--color-fg-muted)]"
        }`}
      >
        {prefix}
      </span>
      <span role="cell" className="whitespace-pre-wrap break-words py-1 pr-4">
        {line.text}
      </span>
    </div>
  );
}

function PrincipalProfileLink({ principal }: { principal?: Principal }) {
  const label = principalLabel(principal);
  if (principal?.type !== "user" || !principal.id) return <>{label}</>;
  return (
    <Link
      to={`/app/users/${encodeURIComponent(principal.id)}`}
      className="font-medium text-[var(--color-fg)] hover:text-[var(--color-accent)] hover:underline"
    >
      {label}
    </Link>
  );
}

function RequirementRow({ met, label }: { met: boolean; label: string }) {
  return (
    <div className="flex items-start gap-2 text-sm">
      {met ? (
        <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-[var(--color-success)]" />
      ) : (
        <XCircle className="mt-0.5 h-4 w-4 shrink-0 text-[var(--color-danger)]" />
      )}
      <span className={met ? "" : "text-[var(--color-fg-muted)]"}>{label}</span>
    </div>
  );
}
