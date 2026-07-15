import { useEffect, useMemo, useRef, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  BookCopy,
  Building2,
  FileText,
  Folder as FolderIcon,
  LockKeyhole,
  Plus,
  Search,
  Upload,
  UserRound,
} from "lucide-react";
import { toast } from "sonner";
import {
  api,
  APIError,
  docTitle,
  repositoryNamespaceName,
  type Document,
  type Folder,
  type FolderEntry,
  type Repository,
  type RepositoryScope,
} from "@/api";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input, Label, Textarea } from "@/components/ui/input";
import { EmptyState, Skeleton } from "@/components/ui/misc";
import { queryKeys } from "@/lib/queryKeys";
import { errorMessage } from "@/lib/errors";
import { useTitle } from "@/lib/use-title";

type SourceKind = "empty" | "folder" | "document" | "upload-folder" | "upload-document";

type UploadSelection = {
  files: File[];
  paths: string[];
  rootName: string;
  ignoredFiles: number;
  totalBytes: number;
};

const MAX_IMPORT_FILES = 500;
const MAX_IMPORT_FILE_BYTES = 10 * 1024 * 1024;
const MAX_IMPORT_TOTAL_BYTES = 50 * 1024 * 1024;

export function RepositoriesPage() {
  useTitle("Repositories");
  const [search, setSearch] = useState("");
  const [createOpen, setCreateOpen] = useState(false);
  const repositories = useQuery({
    queryKey: queryKeys.repositories(),
    queryFn: api.listRepositories,
  });
  const root = useQuery({
    queryKey: queryKeys.folders(),
    queryFn: () => api.listFolders(),
  });
  const documents = useQuery({
    queryKey: queryKeys.documents(),
    queryFn: () => api.listDocuments(),
  });
  const config = useQuery({
    queryKey: ["config"],
    queryFn: api.getConfig,
    staleTime: Infinity,
  });
  const me = useQuery({ queryKey: queryKeys.me(), queryFn: api.me });

  const repositoryRootIds = new Set(
    (repositories.data?.items || [])
      .map((repository) => repository.root_folder_id)
      .filter(Boolean),
  );
  const rootFolders = (root.data?.items || [])
    .filter(
      (entry): entry is FolderEntry & { folder: Folder } =>
        entry.type === "folder" && !!entry.folder,
    )
    .map((entry) => entry.folder)
    .filter(
      (folder) =>
        !folder.parent_folder_id &&
        !repositoryRootIds.has(folder.id) &&
        samePrincipal(folder.owner, me.data?.principal),
    );
  const looseDocuments = (documents.data?.items || []).filter(
    (document) =>
      !document.parent_folder_id &&
      !document.repository_id &&
      samePrincipal(document.owner, me.data?.principal),
  );
  const term = search.trim().toLowerCase();
  const filtered = useMemo(
    () =>
      (repositories.data?.items || []).filter((repository) =>
        [
          repository.name,
          repository.slug,
          repository.description,
          repositoryNamespaceName(repository),
        ]
          .filter(Boolean)
          .some((value) => String(value).toLowerCase().includes(term)),
      ),
    [repositories.data, term],
  );
  const personal = filtered.filter((repository) => repository.scope !== "organization");
  const organization = filtered.filter(
    (repository) => repository.scope === "organization",
  );
  const organizationName = config.data?.org_name?.trim() || "Organization";

  if (repositories.isLoading) {
    return (
      <div className="mx-auto max-w-6xl px-4 py-6 sm:px-6 sm:py-10">
        <Skeleton className="mb-6 h-9 w-56" />
        <div className="grid gap-4 md:grid-cols-2">
          <Skeleton className="h-36 w-full" />
          <Skeleton className="h-36 w-full" />
        </div>
      </div>
    );
  }

  if (repositories.error) {
    return (
      <div className="mx-auto max-w-6xl px-4 py-10 sm:px-6">
        <EmptyState
          icon={<BookCopy className="h-5 w-5" />}
          title="Could not load repositories"
          description="Refresh the page to try again."
          action={
            <Button variant="outline" onClick={() => repositories.refetch()}>
              Try again
            </Button>
          }
        />
      </div>
    );
  }

  const hasRepositories = (repositories.data?.items || []).length > 0;

  return (
    <div className="mx-auto max-w-6xl px-4 py-6 sm:px-6 sm:py-10">
      <div className="mb-7 flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Repositories</h1>
          <p className="mt-1 max-w-2xl text-sm text-[var(--color-fg-muted)]">
            Organize documents into reviewable spaces for you and your organizations.
          </p>
        </div>
        <Button onClick={() => setCreateOpen(true)}>
          <Plus className="h-4 w-4" /> New repository
        </Button>
      </div>

      {hasRepositories ? (
        <div className="mb-7 max-w-md">
          <div className="relative">
            <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-[var(--color-fg-muted)]" />
            <Input
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder="Find a repository…"
              className="pl-9"
            />
          </div>
        </div>
      ) : null}

      {!hasRepositories ? (
        <EmptyState
          icon={<BookCopy className="h-5 w-5" />}
          title="Create your first repository"
          description="Start with an empty repository, or bring an existing folder or document into a reviewable space."
          action={
            <Button onClick={() => setCreateOpen(true)}>
              <Plus className="h-4 w-4" /> New repository
            </Button>
          }
        />
      ) : personal.length === 0 && organization.length === 0 ? (
        <EmptyState
          icon={<Search className="h-5 w-5" />}
          title="No matching repositories"
          description={`Nothing matches “${search}”.`}
        />
      ) : (
        <div className="space-y-9">
          <RepositorySection
            title={organizationName}
            icon={<Building2 className="h-4 w-4" />}
            repositories={organization}
            description="Organization repositories are discoverable by everyone in your organization."
          />
          <RepositorySection
            title="Personal"
            icon={<UserRound className="h-4 w-4" />}
            repositories={personal}
            description="Repositories owned by you or shared directly with you."
          />
        </div>
      )}

      <CreateRepositoryDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        rootFolders={rootFolders}
        looseDocuments={looseDocuments}
        sourcesLoading={root.isLoading || documents.isLoading || me.isLoading}
        organizationEnabled={config.data?.deployment === "org"}
        everyoneLabel={config.data?.everyone_label || "everyone"}
        configLoading={config.isLoading}
      />
    </div>
  );
}

function RepositorySection({
  title,
  icon,
  repositories,
  description,
}: {
  title: string;
  icon: React.ReactNode;
  repositories: Repository[];
  description?: string;
}) {
  if (repositories.length === 0) return null;
  return (
    <section>
      <div className="mb-3 flex flex-col gap-1 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h2 className="flex items-center gap-2 text-sm font-semibold">
            {icon} {title}
            <span className="font-normal text-[var(--color-fg-muted)]">
              {repositories.length}
            </span>
          </h2>
          {description ? (
            <p className="mt-1 text-xs text-[var(--color-fg-muted)]">{description}</p>
          ) : null}
        </div>
      </div>
      <div className="grid gap-3 md:grid-cols-2">
        {repositories.map((repository) => (
          <Link
            key={repository.id}
            to={`/app/repos/${repository.id}`}
            className="group rounded-[14px] border border-[var(--color-border)] bg-[var(--color-surface)] p-5 transition-[border-color,box-shadow,transform] duration-200 hover:-translate-y-0.5 hover:border-[var(--color-border-strong)] hover:shadow-md"
          >
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <div className="truncate text-xs text-[var(--color-fg-muted)]">
                  {repositoryNamespaceName(repository)}
                </div>
                <div className="mt-0.5 flex items-center gap-2">
                  <BookCopy className="h-4 w-4 shrink-0 text-[var(--color-accent)]" />
                  <h3 className="truncate font-semibold group-hover:underline">
                    {repository.name}
                  </h3>
                </div>
              </div>
              <Badge variant="muted" className="capitalize">
                {repository.scope}
              </Badge>
            </div>
            <p className="mt-3 line-clamp-2 min-h-10 text-sm text-[var(--color-fg-muted)]">
              {repository.description || "No description yet."}
            </p>
            <div className="mt-4 flex items-center gap-3 text-xs text-[var(--color-fg-muted)]">
              {repository.protected ? (
                <span className="flex items-center gap-1">
                  <LockKeyhole className="h-3.5 w-3.5" /> Protected
                </span>
              ) : null}
              <span>
                {repository.required_approvals || 0} required approval
                {(repository.required_approvals || 0) === 1 ? "" : "s"}
              </span>
            </div>
          </Link>
        ))}
      </div>
    </section>
  );
}

function CreateRepositoryDialog({
  open,
  onOpenChange,
  rootFolders,
  looseDocuments,
  sourcesLoading,
  organizationEnabled,
  everyoneLabel,
  configLoading,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  rootFolders: Folder[];
  looseDocuments: Document[];
  sourcesLoading: boolean;
  organizationEnabled: boolean;
  everyoneLabel: string;
  configLoading: boolean;
}) {
  const navigate = useNavigate();
  const qc = useQueryClient();
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [description, setDescription] = useState("");
  const [scope, setScope] = useState<RepositoryScope>(
    organizationEnabled ? "organization" : "personal",
  );
  const [sourceKind, setSourceKind] = useState<SourceKind>("empty");
  const [sourceId, setSourceId] = useState("");
  const [upload, setUpload] = useState<UploadSelection>();
  const [uploadError, setUploadError] = useState<string>();
  const [createError, setCreateError] = useState<string>();

  const reset = () => {
    setName("");
    setSlug("");
    setDescription("");
    setScope(organizationEnabled ? "organization" : "personal");
    setSourceKind("empty");
    setSourceId("");
    setUpload(undefined);
    setUploadError(undefined);
    setCreateError(undefined);
  };

  useEffect(() => {
    if (open && organizationEnabled) setScope("organization");
  }, [open, organizationEnabled]);

  const create = useMutation({
    mutationFn: async () => {
      const common = {
        name: name.trim(),
        slug: slug.trim(),
        description: description.trim() || undefined,
        scope,
      };
      if (sourceKind === "upload-folder" || sourceKind === "upload-document") {
        if (!upload) throw new Error("Choose content to upload first.");
        const result = await api.importRepository({
          ...common,
          import_kind: sourceKind === "upload-folder" ? "folder" : "document",
          files: upload.files,
          paths: upload.paths,
        });
        return result.repository;
      }
      return api.createRepository({
        ...common,
        ...(sourceKind === "empty"
          ? {}
          : sourceKind === "folder"
            ? { root_folder_id: sourceId }
            : { document_id: sourceId }),
      });
    },
    onSuccess: (repository) => {
      qc.invalidateQueries({ queryKey: queryKeys.repositories() });
      qc.invalidateQueries({ queryKey: queryKeys.documents() });
      qc.invalidateQueries({ queryKey: queryKeys.folders() });
      toast.success("Repository created.");
      onOpenChange(false);
      reset();
      navigate(`/app/repos/${repository.id}`);
    },
    onError: (error) => {
      const message =
        error instanceof APIError && error.status === 409
          ? "That repository name or slug is already in use."
          : errorMessage(error, "Could not create repository");
      setCreateError(message);
    },
  });

  const canSubmit =
    name.trim().length > 0 &&
    slug.trim().length > 0 &&
    (sourceKind === "empty" ||
      ((sourceKind === "folder" || sourceKind === "document") && sourceId.length > 0) ||
      ((sourceKind === "upload-folder" || sourceKind === "upload-document") &&
        !!upload &&
        !uploadError));
  const sourceOptions = sourceKind === "folder" ? rootFolders : looseDocuments;

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        onOpenChange(next);
        if (!next && !create.isPending) reset();
      }}
    >
      <DialogContent className="max-h-[92vh] max-w-2xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle>New repository</DialogTitle>
          <DialogDescription>
            Create a home for documents, review, and approved changes.
          </DialogDescription>
        </DialogHeader>
        <form
          className="space-y-4"
          onSubmit={(event) => {
            event.preventDefault();
            setCreateError(undefined);
            if (canSubmit) create.mutate();
          }}
        >
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-1.5">
              <Label htmlFor="repository-name">Name</Label>
              <Input
                id="repository-name"
                value={name}
                onChange={(event) => {
                  const next = event.target.value;
                  setName(next);
                  if (!slug || slug === slugify(name)) setSlug(slugify(next));
                }}
                placeholder="Payments handbook"
                autoFocus
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="repository-slug">Slug</Label>
              <Input
                id="repository-slug"
                value={slug}
                onChange={(event) => setSlug(slugify(event.target.value))}
                placeholder="payments-handbook"
                autoCapitalize="off"
                spellCheck={false}
              />
            </div>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="repository-description">Description</Label>
            <Textarea
              id="repository-description"
              value={description}
              onChange={(event) => setDescription(event.target.value)}
              placeholder="What belongs in this repository?"
              rows={2}
            />
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-1.5">
              <Label htmlFor="repository-scope">Owner scope</Label>
              <select
                id="repository-scope"
                value={scope}
                onChange={(event) => setScope(event.target.value as RepositoryScope)}
                className="h-9 w-full rounded-[10px] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 text-sm"
              >
                <option value="personal">Personal</option>
                {organizationEnabled ? (
                  <option value="organization">Organization</option>
                ) : null}
              </select>
              {!organizationEnabled && !configLoading ? (
                <p className="text-[11px] text-[var(--color-fg-muted)]">
                  Organization repositories are unavailable on this deployment.
                </p>
              ) : scope === "organization" ? (
                <p className="text-[11px] leading-4 text-[var(--color-fg-muted)]">
                  Automatically visible to {everyoneLabel}.
                </p>
              ) : (
                <p className="text-[11px] leading-4 text-[var(--color-fg-muted)]">
                  Private to you until you invite collaborators.
                </p>
              )}
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="repository-source">Start with</Label>
              <select
                id="repository-source"
                value={sourceKind}
                onChange={(event) => {
                  setSourceKind(event.target.value as SourceKind);
                  setSourceId("");
                  setUpload(undefined);
                  setUploadError(undefined);
                }}
                className="h-9 w-full rounded-[10px] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 text-sm"
              >
                <option value="empty">Empty repository</option>
                <option value="folder">Use an existing aidocs root folder</option>
                <option value="upload-folder">Upload a local folder</option>
                <option value="document">Use an existing aidocs loose document</option>
                <option value="upload-document">Upload a local document</option>
              </select>
            </div>
          </div>
          {sourceKind === "empty" ? (
            <div className="rounded-xl border border-[var(--color-border)] bg-[var(--color-surface-muted)]/45 p-3">
              <p className="flex items-start gap-2 text-sm">
                <BookCopy className="mt-0.5 h-4 w-4 shrink-0 text-[var(--color-accent)]" />
                <span>
                  Start clean. You can add the first document after the repository is
                  created.
                </span>
              </p>
            </div>
          ) : sourceKind === "upload-folder" || sourceKind === "upload-document" ? (
            <LocalUploadPicker
              kind={sourceKind === "upload-folder" ? "folder" : "document"}
              selection={upload}
              error={uploadError}
              onSelect={(selection, error) => {
                setUpload(selection);
                setUploadError(error);
                if (selection && !name) {
                  const nextName = titleFromFilename(selection.rootName);
                  setName(nextName);
                  setSlug(slugify(nextName));
                }
              }}
            />
          ) : (
            <div className="space-y-1.5 rounded-xl border border-[var(--color-border)] bg-[var(--color-surface-muted)]/45 p-3">
              <Label htmlFor="repository-source-item">
                {sourceKind === "folder" ? "Folder" : "Document"}
              </Label>
              <select
                id="repository-source-item"
                value={sourceId}
                onChange={(event) => {
                  const next = event.target.value;
                  setSourceId(next);
                  const selected = sourceOptions.find((item) => item.id === next);
                  if (selected && !name) {
                    const nextName =
                      "title" in selected
                        ? selected.title
                        : docTitle(selected as Document);
                    setName(nextName);
                    setSlug(slugify(nextName));
                  }
                }}
                disabled={sourcesLoading}
                className="h-9 w-full rounded-[10px] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 text-sm disabled:opacity-50"
              >
                <option value="">
                  {sourcesLoading
                    ? "Loading…"
                    : sourceOptions.length === 0
                      ? `No existing aidocs ${sourceKind === "folder" ? "root folders" : "loose documents"}`
                      : `Choose a ${sourceKind}`}
                </option>
                {sourceOptions.map((item) => (
                  <option key={item.id} value={item.id}>
                    {"title" in item ? item.title : docTitle(item as Document)}
                  </option>
                ))}
              </select>
              <p className="flex items-center gap-1.5 text-xs text-[var(--color-fg-muted)]">
                {sourceKind === "folder" ? (
                  <FolderIcon className="h-3.5 w-3.5" />
                ) : (
                  <FileText className="h-3.5 w-3.5" />
                )}
                The selected item moves into this repository.
              </p>
              {!sourcesLoading && sourceOptions.length === 0 ? (
                <button
                  type="button"
                  className="text-left text-xs font-medium text-[var(--color-accent)] hover:underline"
                  onClick={() =>
                    setSourceKind(
                      sourceKind === "folder" ? "upload-folder" : "upload-document",
                    )
                  }
                >
                  {sourceKind === "folder"
                    ? "Upload a folder from this computer instead"
                    : "Upload a document from this computer instead"}
                </button>
              ) : null}
            </div>
          )}
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
            <Button type="submit" disabled={!canSubmit || create.isPending}>
              {create.isPending
                ? sourceKind.startsWith("upload-")
                  ? "Uploading & creating…"
                  : "Creating…"
                : "Create repository"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function LocalUploadPicker({
  kind,
  selection,
  error,
  onSelect,
}: {
  kind: "folder" | "document";
  selection?: UploadSelection;
  error?: string;
  onSelect: (selection?: UploadSelection, error?: string) => void;
}) {
  const inputId = `repository-upload-${kind}`;
  const isFolder = kind === "folder";
  const supportsFolderPicker =
    !isFolder ||
    typeof document === "undefined" ||
    "webkitdirectory" in document.createElement("input");
  const inputRef = useRef<HTMLInputElement>(null);
  return (
    <div className="space-y-3 rounded-xl border border-[var(--color-border)] bg-[var(--color-surface-muted)]/45 p-3">
      <div>
        <p className="text-sm font-medium">
          {isFolder ? "Upload a folder of documents" : "Upload a document"}
        </p>
        <p className="mt-1 text-xs leading-5 text-[var(--color-fg-muted)]">
          {isFolder && supportsFolderPicker
            ? "Folder structure is preserved. HTML documents are imported; other files are skipped."
            : isFolder
              ? "This browser cannot select a whole folder. Select multiple HTML documents instead; they will be imported at the repository root."
              : "Choose one .html or .htm document. It becomes the first document in this repository."}
        </p>
      </div>
      <input
        id={inputId}
        type="file"
        className="hidden"
        tabIndex={-1}
        accept=".html,.htm,text/html"
        multiple={isFolder}
        ref={(input) => {
          inputRef.current = input;
          if (!input) return;
          if (isFolder && supportsFolderPicker) {
            input.setAttribute("webkitdirectory", "");
            input.setAttribute("directory", "");
          } else {
            input.removeAttribute("webkitdirectory");
            input.removeAttribute("directory");
          }
        }}
        onChange={(event) => {
          const result = validateUpload(
            kind,
            Array.from(event.currentTarget.files || []),
          );
          onSelect(result.selection, result.error);
          event.currentTarget.value = "";
        }}
      />
      <Button type="button" variant="outline" onClick={() => inputRef.current?.click()}>
        <Upload className="h-4 w-4" />
        {selection
          ? isFolder && supportsFolderPicker
            ? "Choose another folder"
            : isFolder
              ? "Choose other documents"
              : "Choose another document"
          : isFolder && supportsFolderPicker
            ? "Choose folder"
            : isFolder
              ? "Choose documents"
              : "Choose document"}
      </Button>
      {error ? (
        <p role="alert" className="text-xs leading-5 text-[var(--color-danger)]">
          {error}
        </p>
      ) : null}
      {selection ? (
        <div
          aria-live="polite"
          className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-3"
        >
          <div className="flex items-start gap-2">
            {isFolder ? (
              <FolderIcon className="mt-0.5 h-4 w-4 shrink-0 text-[var(--color-accent)]" />
            ) : (
              <FileText className="mt-0.5 h-4 w-4 shrink-0 text-[var(--color-accent)]" />
            )}
            <div className="min-w-0">
              <p className="truncate text-sm font-medium">{selection.rootName}</p>
              <p className="text-xs text-[var(--color-fg-muted)]">
                {selection.files.length} document
                {selection.files.length === 1 ? "" : "s"} ·{" "}
                {formatBytes(selection.totalBytes)}
              </p>
            </div>
          </div>
          {isFolder ? (
            <ul className="mt-3 max-h-32 space-y-1 overflow-auto border-t border-[var(--color-border)] pt-2 font-mono text-[11px] text-[var(--color-fg-muted)]">
              {selection.paths.slice(0, 8).map((path) => (
                <li key={path} className="truncate">
                  {path}
                </li>
              ))}
              {selection.paths.length > 8 ? (
                <li>+ {selection.paths.length - 8} more documents</li>
              ) : null}
            </ul>
          ) : null}
          {selection.ignoredFiles > 0 ? (
            <p className="mt-2 text-xs text-[var(--color-fg-muted)]">
              {selection.ignoredFiles} non-HTML file
              {selection.ignoredFiles === 1 ? " was" : "s were"} skipped.
            </p>
          ) : null}
        </div>
      ) : null}
      <p className="text-[11px] leading-4 text-[var(--color-fg-muted)]">
        Up to {MAX_IMPORT_FILES} documents, {formatBytes(MAX_IMPORT_FILE_BYTES)} per
        document, and {formatBytes(MAX_IMPORT_TOTAL_BYTES)} total.
      </p>
    </div>
  );
}

function validateUpload(
  kind: "folder" | "document",
  selectedFiles: File[],
): { selection?: UploadSelection; error?: string } {
  if (selectedFiles.length === 0) return { error: "Nothing was selected." };
  const htmlFiles = selectedFiles.filter((file) => /\.html?$/i.test(file.name));
  if (kind === "document" && selectedFiles.length !== 1) {
    return { error: "Choose exactly one HTML document." };
  }
  if (htmlFiles.length === 0) {
    return { error: "No .html or .htm documents were found in that selection." };
  }
  if (htmlFiles.length > MAX_IMPORT_FILES) {
    return {
      error: `That selection has ${htmlFiles.length} HTML documents. The limit is ${MAX_IMPORT_FILES}.`,
    };
  }
  const oversized = htmlFiles.find((file) => file.size > MAX_IMPORT_FILE_BYTES);
  if (oversized) {
    return {
      error: `${oversized.name} is ${formatBytes(oversized.size)}. Each document must be ${formatBytes(MAX_IMPORT_FILE_BYTES)} or smaller.`,
    };
  }
  const totalBytes = htmlFiles.reduce((total, file) => total + file.size, 0);
  if (totalBytes > MAX_IMPORT_TOTAL_BYTES) {
    return {
      error: `That selection is ${formatBytes(totalBytes)}. The combined limit is ${formatBytes(MAX_IMPORT_TOTAL_BYTES)}.`,
    };
  }

  const rawPaths = htmlFiles.map((file) =>
    (file.webkitRelativePath || file.name).replaceAll("\\", "/"),
  );
  const rootName =
    kind === "folder"
      ? rawPaths[0]?.split("/")[0] || "Uploaded folder"
      : htmlFiles[0].name;
  const paths = rawPaths.map((path) => {
    const parts = path.split("/").filter(Boolean);
    if (kind === "folder" && parts.length > 1) parts.shift();
    return parts.join("/");
  });
  if (paths.some((path) => !path || path === "." || path.includes("../"))) {
    return { error: "One or more document paths are invalid." };
  }
  const normalized = new Set<string>();
  for (const path of paths) {
    const key = path.toLowerCase();
    if (normalized.has(key)) {
      return { error: `Two documents resolve to the same path: ${path}` };
    }
    normalized.add(key);
  }
  return {
    selection: {
      files: htmlFiles,
      paths,
      rootName,
      ignoredFiles: selectedFiles.length - htmlFiles.length,
      totalBytes,
    },
  };
}

function titleFromFilename(value: string) {
  return value
    .replace(/\.html?$/i, "")
    .replace(/[-_]+/g, " ")
    .replace(/\b\w/g, (character) => character.toUpperCase());
}

function formatBytes(bytes: number) {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KiB`;
  return `${(bytes / (1024 * 1024)).toFixed(bytes % (1024 * 1024) ? 1 : 0)} MiB`;
}

function slugify(value: string) {
  return value
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 80);
}

function samePrincipal(
  a?: { type?: string; id?: string },
  b?: { type?: string; id?: string },
) {
  return !!a && !!b && a.type === b.type && a.id === b.id;
}
