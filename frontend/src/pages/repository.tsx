import { useState } from "react";
import { Link, useNavigate, useParams, useSearchParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Activity,
  ArrowLeft,
  BookOpen,
  Building2,
  CheckCircle2,
  ChevronRight,
  CircleDot,
  Cloud,
  FilePlus2,
  FileText,
  Folder as FolderIcon,
  GitPullRequest,
  LockKeyhole,
  MessageSquareText,
  Network,
  Plus,
  Settings2,
  Share2,
  XCircle,
} from "lucide-react";
import { toast } from "sonner";
import {
  api,
  APIError,
  docID,
  docTitle,
  repositoryNamespaceName,
  type Document,
  type Folder,
  type FolderEntry,
  type Principal,
  type PullRequest,
  type PullRequestReview,
} from "@/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input, Label, Textarea } from "@/components/ui/input";
import { Sheet, SheetTrigger } from "@/components/ui/sheet";
import { HtmlFileInput, useStagedFile } from "@/components/ui/upload";
import { Center, EmptyState, Skeleton } from "@/components/ui/misc";
import { ShareSheet } from "@/pages/doc-share-sheet";
import { RepositoryActivity } from "@/pages/repository-activity";
import { RepositoryInsights } from "@/pages/repository-insights";
import { RepositorySettings } from "@/pages/repository-settings";
import { RepositorySources } from "@/pages/repository-sources";
import { queryKeys } from "@/lib/queryKeys";
import { errorMessage } from "@/lib/errors";
import { effectiveRoleForPrincipal } from "@/lib/constants";
import { useTitle } from "@/lib/use-title";

type RepositoryTab =
  | "documents"
  | "pulls"
  | "sources"
  | "activity"
  | "insights"
  | "settings";

export function RepositoryPage() {
  const { id = "" } = useParams();
  const [searchParams, setSearchParams] = useSearchParams();
  // Old `?tab=code` links intentionally fall back to the document view.
  const tab = repositoryTab(searchParams.get("tab"));
  const [composerOpen, setComposerOpen] = useState(false);
  const [addDocumentOpen, setAddDocumentOpen] = useState(false);
  const repository = useQuery({
    queryKey: queryKeys.repository(id),
    queryFn: () => api.getRepository(id),
    enabled: !!id,
    meta: { suppressGlobalError: true },
  });
  const rootFolderId = repository.data?.root_folder_id || "";
  const tree = useQuery({
    queryKey: queryKeys.folders(rootFolderId),
    queryFn: () => api.listFolders(rootFolderId),
    enabled: !!rootFolderId,
    meta: { suppressGlobalError: true },
  });
  const documents = useQuery({
    queryKey: queryKeys.repositoryDocuments(id),
    queryFn: () => api.listRepositoryDocuments(id),
    enabled: !!id,
  });
  const pulls = useQuery({
    queryKey: queryKeys.pullRequests(id),
    queryFn: () => api.listPullRequests(id),
    enabled: !!id,
  });
  const me = useQuery({ queryKey: queryKeys.me(), queryFn: api.me });
  const createdByMe = samePrincipal(repository.data?.created_by, me.data?.principal);
  const ownsRepository =
    createdByMe || samePrincipal(repository.data?.owner, me.data?.principal);
  const access = useQuery({
    queryKey: queryKeys.folderAccess(rootFolderId),
    queryFn: () => api.folderAccess(rootFolderId),
    enabled: !!rootFolderId && !!me.data?.principal && !ownsRepository,
  });
  const rootRole = effectiveRoleForPrincipal(
    access.data?.items || [],
    me.data?.principal,
  );
  const canManageAccess = ownsRepository || rootRole === "owner";
  const canCreatePullRequest =
    ownsRepository || rootRole === "owner" || rootRole === "editor";
  const canAddDocument = canCreatePullRequest;
  const permissionsLoading = !ownsRepository && access.isLoading;
  useTitle(repository.data?.name || "Repository");

  if (repository.isLoading) {
    return (
      <div className="mx-auto max-w-6xl px-4 py-6 sm:px-6 sm:py-10">
        <Skeleton className="mb-3 h-8 w-72" />
        <Skeleton className="mb-6 h-5 w-96 max-w-full" />
        <Skeleton className="h-80 w-full" />
      </div>
    );
  }
  if (repository.error) {
    if (repository.error instanceof APIError && repository.error.status === 403) {
      return (
        <Center>
          <EmptyState
            icon={<LockKeyhole className="h-5 w-5" />}
            title="You don't have access to this repository"
            description="Ask a repository owner to share it with you."
          />
        </Center>
      );
    }
    if (!(repository.error instanceof APIError) || repository.error.status !== 404) {
      return (
        <Center>
          <EmptyState
            icon={<BookOpen className="h-5 w-5" />}
            title="Could not load this repository"
            description="Check your connection and try again."
            action={
              <Button variant="outline" onClick={() => repository.refetch()}>
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
          icon={<BookOpen className="h-5 w-5" />}
          title="Repository not found"
          description="It may have been removed, or the link may be incorrect."
          action={
            <Button asChild variant="outline">
              <Link to="/app/repositories">Back to repositories</Link>
            </Button>
          }
        />
      </Center>
    );
  }
  if (!repository.data) return null;

  const repo = repository.data;
  const documentItems = documents.data?.items || [];
  const hasDocuments = documentItems.length > 0;
  const repositoryIsEmpty = !documents.isLoading && !documents.error && !hasDocuments;
  const pullItems = pulls.data?.items || [];
  const openPulls = pullItems.filter((pull) => pull.status === "open").length;
  const setTab = (next: RepositoryTab) => {
    setSearchParams(next === "documents" ? {} : { tab: next }, { replace: true });
  };

  return (
    <div className="mx-auto max-w-6xl px-4 py-6 sm:px-6 sm:py-10">
      <Link
        to="/app/repositories"
        className="mb-5 inline-flex items-center gap-1.5 text-sm text-[var(--color-fg-muted)] hover:text-[var(--color-fg)]"
      >
        <ArrowLeft className="h-4 w-4" /> Repositories
      </Link>

      <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="min-w-0 truncate text-2xl font-semibold tracking-tight">
              <span className="font-normal text-[var(--color-fg-muted)]">
                {repositoryNamespaceName(repo)} /
              </span>{" "}
              {repo.name}
            </h1>
            <Badge variant="muted" className="capitalize">
              {repo.scope}
            </Badge>
            {repo.protected ? (
              <Badge variant="accent">
                <LockKeyhole className="h-3 w-3" /> Protected
              </Badge>
            ) : null}
          </div>
          <p className="mt-2 max-w-3xl text-sm text-[var(--color-fg-muted)]">
            {repo.description || "No description has been added yet."}
          </p>
          <p className="mt-2 text-xs text-[var(--color-fg-muted)]">
            {repo.required_approvals || 0} required approval
            {(repo.required_approvals || 0) === 1 ? "" : "s"} to merge
          </p>
          {repo.scope === "organization" ? (
            <p className="mt-2 flex items-center gap-1.5 text-xs text-[var(--color-fg-muted)]">
              <Building2 className="h-3.5 w-3.5" /> Everyone in the organization can
              discover and view this repository.
            </p>
          ) : null}
        </div>
        <div className="flex shrink-0 items-center gap-2">
          {rootFolderId ? (
            <Sheet>
              <SheetTrigger asChild>
                <Button variant="outline">
                  <Share2 className="h-4 w-4" /> Share
                </Button>
              </SheetTrigger>
              <ShareSheet
                resourceType="folder"
                resourceId={rootFolderId}
                resourceLabel="repository"
                shareURL={`${window.location.origin}/app/repos/${repo.id}`}
                canManageAccess={canManageAccess}
              />
            </Sheet>
          ) : null}
          {tab === "documents" && repositoryIsEmpty ? (
            <Button
              onClick={() => setAddDocumentOpen(true)}
              disabled={permissionsLoading || !canAddDocument || !rootFolderId}
              title={
                !canAddDocument && !permissionsLoading
                  ? "Editor access is required to add a document"
                  : !rootFolderId
                    ? "This repository does not have a document root"
                    : undefined
              }
            >
              <FilePlus2 className="h-4 w-4" /> Add first document
            </Button>
          ) : (tab === "documents" || tab === "pulls") && hasDocuments ? (
            <Button
              onClick={() => setComposerOpen(true)}
              disabled={!hasDocuments || permissionsLoading || !canCreatePullRequest}
              title={
                !canCreatePullRequest && !permissionsLoading
                  ? "Editor access is required to open a pull request"
                  : undefined
              }
            >
              <GitPullRequest className="h-4 w-4" /> New pull request
            </Button>
          ) : null}
        </div>
      </div>

      <div className="mt-7 flex max-w-full gap-1 overflow-x-auto border-b border-[var(--color-border)]">
        <TabButton active={tab === "documents"} onClick={() => setTab("documents")}>
          <BookOpen className="h-4 w-4" /> Documents
        </TabButton>
        <TabButton active={tab === "pulls"} onClick={() => setTab("pulls")}>
          <GitPullRequest className="h-4 w-4" /> Pull requests
          {openPulls > 0 ? <Badge variant="muted">{openPulls}</Badge> : null}
        </TabButton>
        <TabButton active={tab === "sources"} onClick={() => setTab("sources")}>
          <Cloud className="h-4 w-4" /> Sources
        </TabButton>
        <TabButton active={tab === "activity"} onClick={() => setTab("activity")}>
          <Activity className="h-4 w-4" /> Activity
        </TabButton>
        <TabButton active={tab === "insights"} onClick={() => setTab("insights")}>
          <Network className="h-4 w-4" /> Insights
        </TabButton>
        <TabButton active={tab === "settings"} onClick={() => setTab("settings")}>
          <Settings2 className="h-4 w-4" /> Settings
        </TabButton>
      </div>

      <div className="pt-5">
        {tab === "documents" ? (
          <RepositoryTree
            repositoryId={id}
            rootFolderId={rootFolderId}
            tree={tree}
            onCreatePullRequest={() => setComposerOpen(true)}
            canCreatePullRequest={canCreatePullRequest && hasDocuments}
            onAddDocument={() => setAddDocumentOpen(true)}
            canAddDocument={canAddDocument && !!rootFolderId && !permissionsLoading}
          />
        ) : tab === "pulls" ? (
          <PullRequestList
            repositoryId={id}
            pulls={pullItems}
            loading={pulls.isLoading}
            error={!!pulls.error}
            onCreate={() => setComposerOpen(true)}
            canCreate={canCreatePullRequest && hasDocuments}
          />
        ) : tab === "sources" ? (
          <RepositorySources
            repositoryId={id}
            canManage={canManageAccess && !permissionsLoading}
            canSync={canCreatePullRequest && !permissionsLoading}
          />
        ) : tab === "activity" ? (
          <RepositoryActivity repositoryId={id} />
        ) : tab === "insights" ? (
          <RepositoryInsights repositoryId={id} />
        ) : (
          <RepositorySettings repository={repo} canManage={canManageAccess} />
        )}
      </div>

      <NewPullRequestDialog
        open={composerOpen}
        onOpenChange={setComposerOpen}
        repositoryId={id}
        documents={documents.data?.items || []}
        documentsLoading={documents.isLoading}
      />
      <AddDocumentDialog
        open={addDocumentOpen}
        onOpenChange={setAddDocumentOpen}
        repositoryId={id}
        rootFolderId={rootFolderId}
      />
    </div>
  );
}

function repositoryTab(value: string | null): RepositoryTab {
  if (
    value === "pulls" ||
    value === "sources" ||
    value === "activity" ||
    value === "insights" ||
    value === "settings"
  ) {
    return value;
  }
  return "documents";
}

function TabButton({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`relative flex min-h-11 items-center gap-2 px-4 text-sm font-medium transition-colors ${
        active
          ? "text-[var(--color-fg)] after:absolute after:inset-x-2 after:-bottom-px after:h-0.5 after:rounded-full after:bg-[var(--color-accent)]"
          : "text-[var(--color-fg-muted)] hover:text-[var(--color-fg)]"
      }`}
    >
      {children}
    </button>
  );
}

function RepositoryTree({
  repositoryId,
  rootFolderId,
  tree,
  onCreatePullRequest,
  canCreatePullRequest,
  onAddDocument,
  canAddDocument,
}: {
  repositoryId: string;
  rootFolderId: string;
  tree: ReturnType<typeof useQuery<{ folder?: unknown; items: FolderEntry[] }>>;
  onCreatePullRequest: () => void;
  canCreatePullRequest: boolean;
  onAddDocument: () => void;
  canAddDocument: boolean;
}) {
  if (!rootFolderId) {
    return (
      <EmptyState
        icon={<BookOpen className="h-5 w-5" />}
        title="Empty repository"
        description="This repository has no document root, so content cannot be added yet."
      />
    );
  }
  if (tree.isLoading) {
    return (
      <div className="space-y-2">
        <Skeleton className="h-12 w-full" />
        <Skeleton className="h-12 w-full" />
        <Skeleton className="h-12 w-full" />
      </div>
    );
  }
  if (tree.error) {
    return (
      <EmptyState
        title="Could not load repository contents"
        description="Refresh the page to try again."
        action={
          <Button variant="outline" onClick={() => tree.refetch()}>
            Try again
          </Button>
        }
      />
    );
  }
  const entries = tree.data?.items || [];
  if (entries.length === 0) {
    return (
      <EmptyState
        icon={<BookOpen className="h-5 w-5" />}
        title="No documents yet"
        description="Create the first document here. Once it has content, changes can move through pull requests and review."
        action={
          canAddDocument ? (
            <Button onClick={onAddDocument}>
              <FilePlus2 className="h-4 w-4" /> Add first document
            </Button>
          ) : undefined
        }
      />
    );
  }
  return (
    <div className="overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)]">
      <div className="flex items-center justify-between border-b border-[var(--color-border)] bg-[var(--color-surface-muted)]/55 px-4 py-2.5">
        <div className="flex items-center gap-2 text-xs font-medium text-[var(--color-fg-muted)]">
          <BookOpen className="h-3.5 w-3.5" /> Documents
        </div>
        {canCreatePullRequest ? (
          <Button size="sm" variant="ghost" onClick={onCreatePullRequest}>
            <Plus className="h-3.5 w-3.5" /> Propose change
          </Button>
        ) : null}
      </div>
      <ul className="divide-y divide-[var(--color-border)]">
        {entries.map((entry) => (
          <RepositoryTreeRow
            key={entry.folder?.id || entry.document?.id}
            entry={entry}
            repositoryId={repositoryId}
          />
        ))}
      </ul>
    </div>
  );
}

function RepositoryTreeRow({
  entry,
  repositoryId,
}: {
  entry: FolderEntry;
  repositoryId: string;
}) {
  const isFolder = entry.type === "folder" && !!entry.folder;
  if (isFolder) {
    return (
      <RepositoryFolderRow
        folder={entry.folder!}
        lastActive={entry.last_active}
        depth={0}
        repositoryId={repositoryId}
      />
    );
  }
  return <RepositoryDocumentRow entry={entry} depth={0} repositoryId={repositoryId} />;
}

function RepositoryFolderRow({
  folder,
  lastActive,
  depth,
  repositoryId,
}: {
  folder: Folder;
  lastActive?: string;
  depth: number;
  repositoryId: string;
}) {
  const [open, setOpen] = useState(false);
  const children = useQuery({
    queryKey: queryKeys.folders(folder.id),
    queryFn: () => api.listFolders(folder.id),
    enabled: open,
  });
  return (
    <>
      <li>
        <button
          type="button"
          onClick={() => setOpen((value) => !value)}
          aria-expanded={open}
          style={{ paddingLeft: 16 + depth * 22 }}
          className="group flex min-h-12 w-full items-center gap-3 py-2.5 pr-4 text-left transition-colors hover:bg-[var(--color-surface-muted)]/55"
        >
          <ChevronRight
            className={`h-3.5 w-3.5 shrink-0 text-[var(--color-fg-muted)] transition-transform ${open ? "rotate-90" : ""}`}
          />
          <FolderIcon className="h-4 w-4 shrink-0 text-[var(--color-accent)]" />
          <span className="min-w-0 flex-1 truncate text-sm font-medium group-hover:underline">
            {folder.title}
          </span>
          <span className="hidden text-xs text-[var(--color-fg-muted)] sm:block">
            {formatRelativeDate(lastActive)}
          </span>
        </button>
      </li>
      {open && children.isLoading ? (
        <li
          style={{ paddingLeft: 38 + depth * 22 }}
          className="px-4 py-3 text-xs text-[var(--color-fg-muted)]"
        >
          Loading folder…
        </li>
      ) : null}
      {open && children.error ? (
        <li
          style={{ paddingLeft: 38 + depth * 22 }}
          className="px-4 py-3 text-xs text-[var(--color-danger)]"
        >
          Could not load this folder.
        </li>
      ) : null}
      {open && children.data?.items.length === 0 ? (
        <li
          style={{ paddingLeft: 38 + depth * 22 }}
          className="px-4 py-3 text-xs text-[var(--color-fg-muted)]"
        >
          Empty folder
        </li>
      ) : null}
      {open
        ? (children.data?.items || []).map((entry) =>
            entry.type === "folder" && entry.folder ? (
              <RepositoryFolderRow
                key={entry.folder.id}
                folder={entry.folder}
                lastActive={entry.last_active}
                depth={depth + 1}
                repositoryId={repositoryId}
              />
            ) : (
              <RepositoryDocumentRow
                key={entry.document?.id}
                entry={entry}
                depth={depth + 1}
                repositoryId={repositoryId}
              />
            ),
          )
        : null}
    </>
  );
}

function RepositoryDocumentRow({
  entry,
  depth,
  repositoryId,
}: {
  entry: FolderEntry;
  depth: number;
  repositoryId: string;
}) {
  const document = entry.document;
  if (!document) return null;
  return (
    <li>
      <Link
        to={`/app/d/${docID(document)}?repo=${encodeURIComponent(repositoryId)}`}
        style={{ paddingLeft: 38 + depth * 22 }}
        className="group flex min-h-12 items-center gap-3 px-4 py-2.5 transition-colors hover:bg-[var(--color-surface-muted)]/55"
      >
        <FileText className="h-4 w-4 shrink-0 text-[var(--color-fg-muted)]" />
        <span className="min-w-0 flex-1 truncate text-sm font-medium group-hover:underline">
          {docTitle(document)}
        </span>
        <span className="hidden text-xs text-[var(--color-fg-muted)] sm:block">
          {formatRelativeDate(entry.last_active)}
        </span>
        <ChevronRight className="h-4 w-4 text-[var(--color-fg-muted)]" />
      </Link>
    </li>
  );
}

function PullRequestList({
  repositoryId,
  pulls,
  loading,
  error,
  onCreate,
  canCreate,
}: {
  repositoryId: string;
  pulls: PullRequest[];
  loading: boolean;
  error: boolean;
  onCreate: () => void;
  canCreate: boolean;
}) {
  if (loading) {
    return (
      <div className="space-y-2">
        <Skeleton className="h-20 w-full" />
        <Skeleton className="h-20 w-full" />
      </div>
    );
  }
  if (error) {
    return <EmptyState title="Could not load pull requests" />;
  }
  if (pulls.length === 0) {
    return (
      <EmptyState
        icon={<GitPullRequest className="h-5 w-5" />}
        title="No pull requests yet"
        description="Propose an update to a document and send it for review."
        action={
          canCreate ? (
            <Button onClick={onCreate}>
              <Plus className="h-4 w-4" /> New pull request
            </Button>
          ) : undefined
        }
      />
    );
  }
  const sorted = [...pulls].sort((a, b) => b.number - a.number);
  return (
    <div className="overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)]">
      <ul className="divide-y divide-[var(--color-border)]">
        {sorted.map((pull) => {
          const approvals = latestReviewCounts(pull).approved;
          const authorProfile = principalProfilePath(pull.author);
          return (
            <li
              key={pull.id}
              className="group flex gap-3 px-4 py-4 transition-colors hover:bg-[var(--color-surface-muted)]/50"
            >
              <PullStatusIcon status={pull.status} />
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <Link
                    to={`/app/repos/${repositoryId}/pulls/${pull.number}`}
                    className="truncate font-semibold group-hover:underline"
                  >
                    {pull.title}
                  </Link>
                  <PullStatusBadge status={pull.status} />
                </div>
                <div className="mt-1 text-xs text-[var(--color-fg-muted)]">
                  #{pull.number} opened by{" "}
                  {authorProfile ? (
                    <Link
                      to={authorProfile}
                      className="font-medium hover:text-[var(--color-fg)] hover:underline"
                    >
                      {principalLabel(pull.author)}
                    </Link>
                  ) : (
                    principalLabel(pull.author)
                  )}{" "}
                  · {formatRelativeDate(pull.created_at)}
                </div>
                <div className="mt-2 flex flex-wrap gap-3 text-xs text-[var(--color-fg-muted)]">
                  <span className="flex items-center gap-1">
                    <CheckCircle2 className="h-3.5 w-3.5" /> {approvals}/
                    {pull.required_approvals || 0} approvals
                  </span>
                  {(pull.reviews || []).length > 0 ? (
                    <span className="flex items-center gap-1">
                      <MessageSquareText className="h-3.5 w-3.5" />
                      {(pull.reviews || []).length} reviews
                    </span>
                  ) : null}
                </div>
              </div>
              <Link
                to={`/app/repos/${repositoryId}/pulls/${pull.number}`}
                aria-label={`Open pull request ${pull.title}`}
                className="-m-2 mt-[-7px] flex shrink-0 items-start p-2 text-[var(--color-fg-muted)] hover:text-[var(--color-fg)]"
              >
                <ChevronRight className="h-4 w-4" />
              </Link>
            </li>
          );
        })}
      </ul>
    </div>
  );
}

function AddDocumentDialog({
  open,
  onOpenChange,
  repositoryId,
  rootFolderId,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  repositoryId: string;
  rootFolderId: string;
}) {
  const navigate = useNavigate();
  const qc = useQueryClient();
  const [title, setTitle] = useState("");
  const [createError, setCreateError] = useState<string>();
  const create = useMutation({
    mutationFn: async () => {
      if (!rootFolderId) throw new Error("Repository document root is unavailable");
      const nextTitle = title.trim();
      const created = await api.createRepositoryDocument(
        repositoryId,
        nextTitle,
        createBlankDocumentFile(nextTitle),
      );
      return created.document;
    },
    onSuccess: (document) => {
      qc.invalidateQueries({ queryKey: queryKeys.repositoryDocuments(repositoryId) });
      qc.invalidateQueries({ queryKey: queryKeys.folders(rootFolderId) });
      qc.invalidateQueries({ queryKey: queryKeys.documents() });
      qc.invalidateQueries({ queryKey: queryKeys.repository(repositoryId) });
      setTitle("");
      setCreateError(undefined);
      onOpenChange(false);
      toast.success("Document created.");
      navigate(`/app/d/${document.id}?repo=${encodeURIComponent(repositoryId)}`);
    },
    onError: (error) => {
      if (error instanceof APIError && error.status === 409) {
        qc.invalidateQueries({
          queryKey: queryKeys.repositoryDocuments(repositoryId),
        });
        qc.invalidateQueries({ queryKey: queryKeys.folders(rootFolderId) });
        setCreateError(
          "This repository already has a document. Refresh to see its contents.",
        );
        return;
      }
      setCreateError(errorMessage(error, "Could not create document"));
    },
  });

  const reset = () => {
    setTitle("");
    setCreateError(undefined);
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        onOpenChange(next);
        if (!next && !create.isPending) reset();
      }}
    >
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>Add a document</DialogTitle>
          <DialogDescription>
            Start with a clean document. You can edit its text after it opens.
          </DialogDescription>
        </DialogHeader>
        <form
          className="space-y-4"
          onSubmit={(event) => {
            event.preventDefault();
            setCreateError(undefined);
            if (title.trim()) create.mutate();
          }}
        >
          <div className="space-y-1.5">
            <Label htmlFor="new-repository-document-title">Document title</Label>
            <Input
              id="new-repository-document-title"
              value={title}
              onChange={(event) => setTitle(event.target.value)}
              placeholder="Getting started"
              autoFocus
            />
          </div>
          <div className="rounded-xl border border-[var(--color-border)] bg-[var(--color-surface-muted)]/45 p-3 text-sm text-[var(--color-fg-muted)]">
            A title and an editable first page are created for you—there is no file or
            markup to prepare.
          </div>
          {createError ? (
            <div
              role="alert"
              className="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-300"
            >
              {createError}
            </div>
          ) : null}
          <DialogFooter>
            <Button
              type="button"
              variant="ghost"
              disabled={create.isPending}
              onClick={() => {
                reset();
                onOpenChange(false);
              }}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={!title.trim() || create.isPending}>
              {create.isPending ? "Creating…" : "Create document"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function NewPullRequestDialog({
  open,
  onOpenChange,
  repositoryId,
  documents,
  documentsLoading,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  repositoryId: string;
  documents: Document[];
  documentsLoading: boolean;
}) {
  const navigate = useNavigate();
  const qc = useQueryClient();
  const [documentId, setDocumentId] = useState("");
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [summary, setSummary] = useState("");
  const [submitError, setSubmitError] = useState<string>();
  const { file, setFile, reset: resetFile } = useStagedFile();
  const selected = documents.find((document) => document.id === documentId);
  const create = useMutation({
    mutationFn: () =>
      api.createPullRequest(repositoryId, {
        document_id: documentId,
        base_version_id: selected?.current_version_id || "",
        title: title.trim(),
        body: body.trim(),
        change_summary: summary.trim(),
        file: file!,
      }),
    onSuccess: (pull) => {
      qc.invalidateQueries({ queryKey: queryKeys.pullRequests(repositoryId) });
      toast.success(`Pull request #${pull.number} opened.`);
      onOpenChange(false);
      reset();
      navigate(`/app/repos/${repositoryId}/pulls/${pull.number}`);
    },
    onError: (error) =>
      setSubmitError(errorMessage(error, "Could not open pull request")),
  });
  const reset = () => {
    setDocumentId("");
    setTitle("");
    setBody("");
    setSummary("");
    setSubmitError(undefined);
    resetFile();
  };
  const canSubmit = !!selected?.current_version_id && !!file && title.trim().length > 0;

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        onOpenChange(next);
        if (!next && !create.isPending) reset();
      }}
    >
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>Open a pull request</DialogTitle>
          <DialogDescription>
            The fastest path is to open a document and choose Edit text. Use this backup
            flow when you already have a complete document file.
          </DialogDescription>
        </DialogHeader>
        <form
          className="space-y-4"
          onSubmit={(event) => {
            event.preventDefault();
            setSubmitError(undefined);
            if (canSubmit) create.mutate();
          }}
        >
          <div className="space-y-1.5">
            <Label htmlFor="pull-document">Document</Label>
            <select
              id="pull-document"
              value={documentId}
              onChange={(event) => {
                const next = event.target.value;
                setDocumentId(next);
                const document = documents.find((item) => item.id === next);
                if (document && !title) setTitle(`Update ${docTitle(document)}`);
              }}
              disabled={documentsLoading}
              className="h-9 w-full rounded-[10px] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 text-sm disabled:opacity-50"
            >
              <option value="">
                {documentsLoading ? "Loading documents…" : "Choose a document"}
              </option>
              {documents.map((document) => (
                <option key={document.id} value={document.id}>
                  {docTitle(document)}
                </option>
              ))}
            </select>
            {selected && !selected.current_version_id ? (
              <p className="text-xs text-[var(--color-danger)]">
                This document has no current version to use as a base.
              </p>
            ) : null}
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="pull-title">Title</Label>
            <Input
              id="pull-title"
              value={title}
              onChange={(event) => setTitle(event.target.value)}
              placeholder="Explain the proposed change"
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="pull-body">Description</Label>
            <Textarea
              id="pull-body"
              value={body}
              onChange={(event) => setBody(event.target.value)}
              placeholder="Why is this change needed? What should reviewers focus on?"
              rows={4}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="pull-summary">Version summary</Label>
            <Input
              id="pull-summary"
              value={summary}
              onChange={(event) => setSummary(event.target.value)}
              placeholder="Short entry for version history"
            />
          </div>
          <div className="space-y-1.5">
            <Label>Backup document file</Label>
            <HtmlFileInput onFile={setFile} />
          </div>
          {submitError ? (
            <div
              role="alert"
              className="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-300"
            >
              {submitError}
            </div>
          ) : null}
          <DialogFooter>
            <Button
              type="button"
              variant="ghost"
              disabled={create.isPending}
              onClick={() => {
                reset();
                onOpenChange(false);
              }}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={!canSubmit || create.isPending}>
              {create.isPending ? "Opening…" : "Open pull request"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function createBlankDocumentFile(title: string) {
  const escapedTitle = escapeDocumentText(title);
  const filename =
    title
      .toLowerCase()
      .trim()
      .replace(/[^a-z0-9]+/g, "-")
      .replace(/^-+|-+$/g, "") || "document";
  const html = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>${escapedTitle}</title>
  <style>
    :root { color-scheme: light dark; font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
    body { margin: 0; background: #fff; color: #18181b; }
    main { max-width: 760px; margin: 0 auto; padding: 64px 28px 96px; }
    h1 { margin: 0 0 24px; font-size: 2.25rem; line-height: 1.15; letter-spacing: -0.035em; }
    p { margin: 0; color: #52525b; font-size: 1rem; line-height: 1.75; }
    @media (prefers-color-scheme: dark) { body { background: #0b0b0d; color: #fafafa; } p { color: #a1a1aa; } }
  </style>
</head>
<body>
  <main>
    <h1>${escapedTitle}</h1>
    <p>Start writing your document here.</p>
  </main>
</body>
</html>`;
  return new File([html], `${filename}.html`, { type: "text/html" });
}

function escapeDocumentText(value: string) {
  return value.replace(
    /[&<>"']/g,
    (character) =>
      ({
        "&": "&amp;",
        "<": "&lt;",
        ">": "&gt;",
        '"': "&quot;",
        "'": "&#39;",
      })[character] || character,
  );
}

export function PullStatusBadge({ status }: { status: PullRequest["status"] }) {
  if (status === "merged") return <Badge variant="accent">Merged</Badge>;
  if (status === "closed") return <Badge variant="danger">Closed</Badge>;
  return <Badge variant="success">Open</Badge>;
}

function PullStatusIcon({ status }: { status: PullRequest["status"] }) {
  if (status === "closed") {
    return <XCircle className="mt-0.5 h-5 w-5 shrink-0 text-[var(--color-danger)]" />;
  }
  if (status === "merged") {
    return <GitPullRequest className="mt-0.5 h-5 w-5 shrink-0 text-purple-600" />;
  }
  return <CircleDot className="mt-0.5 h-5 w-5 shrink-0 text-[var(--color-success)]" />;
}

export function latestReviewCounts(pull: PullRequest) {
  const latest = new Map<string, PullRequestReview>();
  for (const review of (pull.reviews || []).filter(
    (item) => !item.head_version_id || item.head_version_id === pull.head_version_id,
  )) {
    const key = `${review.reviewer.type}:${review.reviewer.id}`;
    latest.set(key, review);
  }
  return {
    approved: [...latest.values()].filter((review) => review.state === "approved")
      .length,
    changesRequested: [...latest.values()].filter(
      (review) => review.state === "changes_requested",
    ).length,
  };
}

export function principalLabel(principal?: Principal) {
  return principal?.name || principal?.email || principal?.id || "Unknown";
}

function principalProfilePath(principal?: Principal) {
  return principal?.type === "user" && principal.id
    ? `/app/users/${encodeURIComponent(principal.id)}`
    : undefined;
}

function samePrincipal(a?: Principal, b?: Principal) {
  return !!a && !!b && a.type === b.type && a.id === b.id;
}

export function formatRelativeDate(value?: string) {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  const delta = date.getTime() - Date.now();
  const abs = Math.abs(delta);
  const formatter = new Intl.RelativeTimeFormat(undefined, { numeric: "auto" });
  if (abs < 60_000) return formatter.format(Math.round(delta / 1_000), "second");
  if (abs < 3_600_000) return formatter.format(Math.round(delta / 60_000), "minute");
  if (abs < 86_400_000) return formatter.format(Math.round(delta / 3_600_000), "hour");
  if (abs < 2_592_000_000)
    return formatter.format(Math.round(delta / 86_400_000), "day");
  return date.toLocaleDateString();
}
