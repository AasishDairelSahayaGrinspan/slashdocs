import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  AlertTriangle,
  CheckCircle2,
  Clock3,
  Cloud,
  Download,
  FileInput,
  FileText,
  GitFork,
  MessageSquareText,
  MoreHorizontal,
  Plus,
  RefreshCw,
  Trash2,
  Ticket,
  Upload,
} from "lucide-react";
import { toast } from "sonner";
import {
  api,
  type ConnectedAccount,
  type RepositorySource,
  type RepositorySourceCheckInput,
  type RepositorySourceInput,
  type RepositorySourceProvider,
  type RepositorySyncRun,
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
import {
  Dropdown,
  DropdownContent,
  DropdownItem,
  DropdownTrigger,
} from "@/components/ui/dropdown";
import { Input, Label, Textarea } from "@/components/ui/input";
import { CodeBlock, EmptyState, Skeleton } from "@/components/ui/misc";
import { GoogleDrivePickerButton } from "@/components/google-drive-picker-button";
import {
  connectedAccountContext,
  connectedAccountLabel,
  connectedAccountsForProvider,
  isConnectedAccountProvider,
  isUsableConnectedAccount,
} from "@/lib/connected-accounts";
import { errorMessage } from "@/lib/errors";
import { queryKeys } from "@/lib/queryKeys";

const providers: Array<{
  id: RepositorySourceProvider;
  label: string;
  description: string;
  icon: React.ReactNode;
}> = [
  {
    id: "github",
    label: "GitHub",
    description: "Sync Markdown and docs from a repository.",
    icon: <GitFork className="h-5 w-5" />,
  },
  {
    id: "slack",
    label: "Slack threads",
    description: "Turn channel threads and replies into reviewed context.",
    icon: <MessageSquareText className="h-5 w-5" />,
  },
  {
    id: "devrev",
    label: "DevRev tickets",
    description: "Keep ticket details and updates connected to your docs.",
    icon: <Ticket className="h-5 w-5" />,
  },
  {
    id: "google_drive",
    label: "Google Drive",
    description: "Import selected Docs or text files.",
    icon: <Cloud className="h-5 w-5" />,
  },
  {
    id: "confluence",
    label: "Confluence",
    description: "Sync a space or selected page tree.",
    icon: <FileText className="h-5 w-5" />,
  },
  {
    id: "notion",
    label: "Notion",
    description: "Connect a workspace page and its children.",
    icon: <FileInput className="h-5 w-5" />,
  },
  {
    id: "manual",
    label: "Manual",
    description: "Track an upload or one-off external source.",
    icon: <Upload className="h-5 w-5" />,
  },
];

export function RepositorySources({
  repositoryId,
  canManage,
  canSync,
}: {
  repositoryId: string;
  canManage: boolean;
  canSync: boolean;
}) {
  const qc = useQueryClient();
  const [connectOpen, setConnectOpen] = useState(false);
  const [proposal, setProposal] = useState<unknown>();
  const [checkSource, setCheckSource] = useState<RepositorySource>();
  const [checkResult, setCheckResult] = useState<RepositorySyncRun>();
  const sources = useQuery({
    queryKey: queryKeys.repositorySources(repositoryId),
    queryFn: () => api.listRepositorySources(repositoryId),
  });
  const connections = useQuery({
    queryKey: queryKeys.connectedAccounts(),
    queryFn: api.listConnections,
    enabled: canManage || canSync,
    meta: { suppressGlobalError: true },
  });
  const syncRuns = useQuery({
    queryKey: queryKeys.repositorySyncRuns(repositoryId),
    queryFn: () => api.listRepositorySyncRuns(repositoryId),
  });
  const refresh = () => {
    qc.invalidateQueries({ queryKey: queryKeys.repositorySources(repositoryId) });
    qc.invalidateQueries({ queryKey: queryKeys.repositorySyncRuns(repositoryId) });
    qc.invalidateQueries({ queryKey: queryKeys.repositoryActivity(repositoryId) });
  };
  const check = useMutation({
    mutationFn: ({
      sourceId,
      input,
      simulate,
    }: {
      sourceId: string;
      input: RepositorySourceCheckInput;
      simulate: boolean;
    }) =>
      simulate
        ? api.simulateRepositorySource(repositoryId, sourceId, input)
        : api.checkRepositorySource(repositoryId, sourceId, input),
    onSuccess: (result, variables) => {
      refresh();
      setCheckResult(result);
      toast.success(
        variables.simulate
          ? "Simulation completed without changing repository state."
          : "Update check completed.",
      );
    },
    onError: (error) => toast.error(errorMessage(error, "Check failed")),
  });
  const exportSource = useMutation({
    mutationFn: (sourceId: string) =>
      api.exportRepositorySource(repositoryId, sourceId),
    onSuccess: (result) => {
      setProposal(result.proposal);
      refresh();
      toast.success("Export proposal created.");
    },
    onError: (error) => toast.error(errorMessage(error, "Export failed")),
  });
  const toggle = useMutation({
    mutationFn: (source: RepositorySource) =>
      api.updateRepositorySource(repositoryId, source.id, {
        status: source.status === "paused" ? "active" : "paused",
      }),
    onSuccess: refresh,
  });
  const remove = useMutation({
    mutationFn: (sourceId: string) =>
      api.deleteRepositorySource(repositoryId, sourceId),
    onSuccess: () => {
      refresh();
      toast.success("Source disconnected.");
    },
  });

  const sourceItems = sources.data?.items || [];
  const connectionItems = connections.data?.connections || [];
  const connectionForSource = (source: RepositorySource) =>
    source.connection ||
    connectionItems.find((connection) => connection.id === source.connection_id);
  const runItems = useMemo(
    () =>
      [...(syncRuns.data?.items || [])].sort((a, b) =>
        (b.started_at || b.created_at || "").localeCompare(
          a.started_at || a.created_at || "",
        ),
      ),
    [syncRuns.data],
  );

  return (
    <div className="space-y-8">
      <section>
        <div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <h2 className="text-lg font-semibold">Connected sources</h2>
            <p className="mt-1 max-w-2xl text-sm text-[var(--color-fg-muted)]">
              Keep knowledge connected to the systems where it already lives. Imports
              become reviewable proposals before published documents change.
            </p>
          </div>
          {canManage ? (
            <Button onClick={() => setConnectOpen(true)}>
              <Plus className="h-4 w-4" /> Connect source
            </Button>
          ) : null}
        </div>

        {sources.isLoading ? (
          <div className="grid gap-3 lg:grid-cols-2">
            <Skeleton className="h-52 w-full" />
            <Skeleton className="h-52 w-full" />
          </div>
        ) : sources.error ? (
          <EmptyState
            icon={<AlertTriangle className="h-5 w-5" />}
            title="Could not load sources"
            description="Check your connection and try again."
            action={
              <Button variant="outline" onClick={() => sources.refetch()}>
                Try again
              </Button>
            }
          />
        ) : sourceItems.length === 0 ? (
          <EmptyState
            icon={<Cloud className="h-5 w-5" />}
            title="No sources connected"
            description="Connect GitHub, Drive, Confluence, Notion, or another source to detect changes and keep these docs fresh."
            action={
              canManage ? (
                <Button onClick={() => setConnectOpen(true)}>
                  <Plus className="h-4 w-4" /> Connect your first source
                </Button>
              ) : undefined
            }
          />
        ) : (
          <div className="grid gap-3 lg:grid-cols-2">
            {sourceItems.map((source) => (
              <SourceCard
                key={source.id}
                source={source}
                connection={connectionForSource(source)}
                canManage={canManage}
                canSync={canSync}
                checking={check.isPending && check.variables?.sourceId === source.id}
                exporting={
                  exportSource.isPending && exportSource.variables === source.id
                }
                onCheck={() => {
                  setCheckResult(undefined);
                  const connection = connectionForSource(source);
                  if (
                    source.connection_id &&
                    source.provider !== "manual" &&
                    connection?.status !== "demo"
                  ) {
                    check.mutate({ sourceId: source.id, input: {}, simulate: false });
                  } else {
                    setCheckSource(source);
                  }
                }}
                onExport={() => exportSource.mutate(source.id)}
                onToggle={() => toggle.mutate(source)}
                onDelete={() => {
                  if (window.confirm("Disconnect this source? Sync history is kept.")) {
                    remove.mutate(source.id);
                  }
                }}
              />
            ))}
          </div>
        )}
      </section>

      <section>
        <div className="mb-3">
          <h2 className="text-lg font-semibold">Sync history</h2>
          <p className="mt-1 text-sm text-[var(--color-fg-muted)]">
            Checks, imports, exports, and their latest outcomes.
          </p>
        </div>
        {syncRuns.isLoading ? (
          <Skeleton className="h-48 w-full" />
        ) : syncRuns.error ? (
          <EmptyState
            title="Could not load sync history"
            action={
              <Button variant="outline" onClick={() => syncRuns.refetch()}>
                Try again
              </Button>
            }
          />
        ) : runItems.length === 0 ? (
          <EmptyState
            icon={<Clock3 className="h-5 w-5" />}
            title="No sync runs yet"
            description="Use Sync now or Check for updates on a source to create the first run."
          />
        ) : (
          <div className="overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)]">
            <div className="overflow-x-auto">
              <table className="w-full min-w-[720px] text-left text-sm">
                <thead className="bg-[var(--color-surface-muted)]/55 text-xs text-[var(--color-fg-muted)]">
                  <tr>
                    <th className="px-4 py-3 font-medium">Run</th>
                    <th className="px-4 py-3 font-medium">Direction</th>
                    <th className="px-4 py-3 font-medium">Status</th>
                    <th className="px-4 py-3 font-medium">Changes</th>
                    <th className="px-4 py-3 font-medium">Started</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-[var(--color-border)]">
                  {runItems.map((run) => (
                    <tr key={run.id}>
                      <td className="px-4 py-3">
                        <div className="font-medium">
                          {run.provider || "Source sync"}
                        </div>
                        <div className="mt-0.5 max-w-xs truncate text-xs text-[var(--color-fg-muted)]">
                          {run.message || run.summary || run.error || run.id}
                        </div>
                      </td>
                      <td className="px-4 py-3 capitalize">
                        {(
                          run.outcome ||
                          run.direction ||
                          run.trigger ||
                          "check"
                        ).replaceAll("_", " ")}
                      </td>
                      <td className="px-4 py-3">
                        <StatusBadge status={run.status} />
                      </td>
                      <td className="px-4 py-3 text-[var(--color-fg-muted)]">
                        {run.changed_files?.length
                          ? `${run.changed_files.length} changed files`
                          : `+${run.added || 0} · ~${run.updated || 0} · −${run.removed || 0}`}
                      </td>
                      <td className="px-4 py-3 text-[var(--color-fg-muted)]">
                        {formatDate(run.started_at || run.created_at)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        )}
      </section>

      <ConnectSourceDialog
        open={connectOpen}
        onOpenChange={setConnectOpen}
        repositoryId={repositoryId}
        connections={connectionItems}
        connectionsLoading={connections.isLoading}
        connectionsError={!!connections.error}
        onCreated={refresh}
      />
      <SourceCheckDialog
        source={checkSource}
        result={checkResult}
        checking={check.isPending}
        onOpenChange={(open) => {
          if (!open && !check.isPending) {
            setCheckSource(undefined);
            setCheckResult(undefined);
          }
        }}
        onRun={(input, simulate) => {
          if (!checkSource) return;
          check.mutate({ sourceId: checkSource.id, input, simulate });
        }}
      />
      <Dialog open={proposal !== undefined} onOpenChange={() => setProposal(undefined)}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>Export proposal</DialogTitle>
            <DialogDescription>
              Review the generated export proposal before applying it upstream.
            </DialogDescription>
          </DialogHeader>
          <CodeBlock className="max-h-[28rem] overflow-auto">
            {JSON.stringify(proposal, null, 2)}
          </CodeBlock>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function SourceCheckDialog({
  source,
  result,
  checking,
  onOpenChange,
  onRun,
}: {
  source?: RepositorySource;
  result?: RepositorySyncRun;
  checking: boolean;
  onOpenChange: (open: boolean) => void;
  onRun: (input: RepositorySourceCheckInput, simulate: boolean) => void;
}) {
  const [beforeSHA, setBeforeSHA] = useState("");
  const [afterSHA, setAfterSHA] = useState("");
  const [changedFiles, setChangedFiles] = useState("");
  const open = !!source;
  const suggestedPath = source ? sourceExamplePath(source) : "README.md";
  useEffect(() => {
    setBeforeSHA("");
    setAfterSHA("");
    setChangedFiles(open ? suggestedPath : "");
  }, [open, suggestedPath]);
  const input = (): RepositorySourceCheckInput => ({
    before_sha: beforeSHA.trim() || undefined,
    after_sha: afterSHA.trim() || `local-${Date.now()}`,
    changed_files: splitLines(changedFiles || suggestedPath),
  });
  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (next && !changedFiles) setChangedFiles(suggestedPath);
        onOpenChange(next);
      }}
    >
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>Check source impact</DialogTitle>
          <DialogDescription>
            Preview or record a change from{" "}
            {source?.external_repository || "this source"}. Add the affected{" "}
            {source?.provider === "slack"
              ? "thread references"
              : source?.provider === "devrev"
                ? "ticket references"
                : "files"}{" "}
            to verify the review flow locally without provider credentials.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-1.5">
              <Label htmlFor="source-before-sha">
                {source && isEventProvider(source.provider)
                  ? "Previous event cursor (optional)"
                  : "Before revision (optional)"}
              </Label>
              <Input
                id="source-before-sha"
                value={beforeSHA}
                onChange={(event) => setBeforeSHA(event.target.value)}
                placeholder={
                  source && isEventProvider(source.provider)
                    ? "Previous event ID"
                    : "Previous commit SHA"
                }
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="source-after-sha">
                {source && isEventProvider(source.provider)
                  ? "Event cursor (optional)"
                  : "After revision (optional)"}
              </Label>
              <Input
                id="source-after-sha"
                value={afterSHA}
                onChange={(event) => setAfterSHA(event.target.value)}
                placeholder={
                  source && isEventProvider(source.provider)
                    ? "A local event ID is generated when blank"
                    : "A local revision is generated when blank"
                }
              />
            </div>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="source-changed-files">
              {source?.provider === "slack"
                ? "Affected threads"
                : source?.provider === "devrev"
                  ? "Affected tickets"
                  : "Changed files"}
            </Label>
            <Textarea
              id="source-changed-files"
              value={changedFiles || suggestedPath}
              onChange={(event) => setChangedFiles(event.target.value)}
              rows={5}
              className="font-mono text-xs"
              placeholder={
                source?.provider === "slack"
                  ? "One Slack thread reference per line"
                  : source?.provider === "devrev"
                    ? "One DevRev ticket reference per line"
                    : "One repository path per line"
              }
            />
            <p className="text-xs text-[var(--color-fg-muted)]">
              One reference per line. Source filters are applied before a pull request
              is proposed.
            </p>
          </div>
          {result ? (
            <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-muted)]/55 p-3">
              <div className="flex flex-wrap items-center gap-2">
                <StatusBadge status={result.outcome || result.status} />
                {result.pull_request_number ? (
                  <span className="text-sm font-medium">
                    Pull request #{result.pull_request_number}
                  </span>
                ) : null}
              </div>
              <p className="mt-2 text-sm text-[var(--color-fg-muted)]">
                {result.message || "Source impact check completed."}
              </p>
            </div>
          ) : null}
        </div>
        <DialogFooter>
          <Button
            variant="ghost"
            disabled={checking}
            onClick={() => onOpenChange(false)}
          >
            Close
          </Button>
          <Button
            variant="outline"
            disabled={checking}
            onClick={() => onRun(input(), true)}
          >
            {checking ? "Checking…" : "Simulate only"}
          </Button>
          <Button disabled={checking} onClick={() => onRun(input(), false)}>
            <RefreshCw className={`h-4 w-4 ${checking ? "animate-spin" : ""}`} />
            {checking ? "Checking…" : "Run check & create PR"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function sourceExamplePath(source: RepositorySource) {
  if (source.provider === "slack") return "slack/threads/example-thread";
  if (source.provider === "devrev") return "devrev/tickets/TKT-123";
  const pattern = source.include_paths?.[0] || "README.md";
  if (!pattern.includes("*")) return pattern;
  if (pattern.endsWith("/**")) {
    const prefix = pattern.slice(0, -3);
    return `${prefix}/${prefix.includes("doc") ? "example.md" : "example.go"}`;
  }
  return pattern.replaceAll("**", "src").replaceAll("*", "example");
}

function splitLines(value: string) {
  return value
    .split(/[\n,]/)
    .map((item) => item.trim())
    .filter(Boolean);
}

function SourceCard({
  source,
  connection,
  canManage,
  canSync,
  checking,
  exporting,
  onCheck,
  onExport,
  onToggle,
  onDelete,
}: {
  source: RepositorySource;
  connection?: ConnectedAccount;
  canManage: boolean;
  canSync: boolean;
  checking: boolean;
  exporting: boolean;
  onCheck: () => void;
  onExport: () => void;
  onToggle: () => void;
  onDelete: () => void;
}) {
  const provider = providers.find((item) => item.id === source.provider);
  const connected = !!source.connection_id;
  const connectionUnavailable =
    connected && !!connection && !isUsableConnectedAccount(connection);
  const syncDisabled = source.status === "paused" || connectionUnavailable;
  return (
    <Card className={source.status === "paused" ? "opacity-70" : undefined}>
      <CardHeader className="flex-row items-start justify-between gap-3">
        <div className="flex min-w-0 items-start gap-3">
          <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-[var(--color-surface-muted)] text-[var(--color-fg-muted)]">
            {provider?.icon || <Cloud className="h-5 w-5" />}
          </div>
          <div className="min-w-0">
            <CardTitle className="truncate">
              {source.external_repository || provider?.label || source.provider}
            </CardTitle>
            <div className="mt-1 flex flex-wrap items-center gap-2">
              <Badge variant="muted" className="capitalize">
                {provider?.label || source.provider.replaceAll("_", " ")}
              </Badge>
              <StatusBadge
                status={source.freshness?.state || source.status || "ready"}
              />
              {connection ? <StatusBadge status={connection.status} /> : null}
            </div>
          </div>
        </div>
        {canManage ? (
          <Dropdown>
            <DropdownTrigger asChild>
              <Button size="icon" variant="ghost" aria-label="Source actions">
                <MoreHorizontal className="h-4 w-4" />
              </Button>
            </DropdownTrigger>
            <DropdownContent>
              <DropdownItem onSelect={onToggle}>
                {source.status === "paused" ? "Resume source" : "Pause source"}
              </DropdownItem>
              <DropdownItem onSelect={onDelete} className="text-[var(--color-danger)]">
                <Trash2 className="h-4 w-4" /> Disconnect
              </DropdownItem>
            </DropdownContent>
          </Dropdown>
        ) : null}
      </CardHeader>
      <CardContent className="space-y-4">
        <dl className="grid grid-cols-2 gap-x-4 gap-y-3 text-xs">
          <SourceFact
            label={
              source.provider === "slack"
                ? "Content"
                : source.provider === "devrev"
                  ? "Content"
                  : "Branch"
            }
            value={
              source.provider === "slack"
                ? "Threads and replies"
                : source.provider === "devrev"
                  ? "Ticket updates"
                  : source.branch || "Default"
            }
          />
          <SourceFact
            label="Target"
            value={
              source.target_folder_id || source.target_document_id || "Repository root"
            }
          />
          <SourceFact label="Direction" value={source.direction || "import"} />
          <SourceFact label="Trigger" value={source.trigger_mode || "manual"} />
          <SourceFact
            label="Account"
            value={
              connection
                ? connectedAccountLabel(connection)
                : connected
                  ? "Connected account"
                  : "Legacy / manual"
            }
          />
        </dl>
        {connection &&
        connectedAccountContext(connection) !== connectedAccountLabel(connection) ? (
          <div className="rounded-lg bg-[var(--color-surface-muted)]/60 px-3 py-2 text-xs text-[var(--color-fg-muted)]">
            Connected through {connectedAccountContext(connection)}.
          </div>
        ) : null}
        {connectionUnavailable ? (
          <div className="rounded-lg border border-[var(--color-warning)]/40 bg-amber-100/60 px-3 py-2 text-xs text-amber-800 dark:bg-amber-950/35 dark:text-amber-300">
            This account needs to be reconnected before the source can sync.
          </div>
        ) : null}
        {source.include_paths?.length || source.exclude_paths?.length ? (
          <div className="rounded-lg bg-[var(--color-surface-muted)]/60 px-3 py-2 text-xs text-[var(--color-fg-muted)]">
            {source.include_paths?.length ? (
              <div>Includes: {source.include_paths.join(", ")}</div>
            ) : null}
            {source.exclude_paths?.length ? (
              <div>Excludes: {source.exclude_paths.join(", ")}</div>
            ) : null}
          </div>
        ) : null}
        <div className="flex flex-wrap items-center justify-between gap-3 border-t border-[var(--color-border)] pt-3">
          <span className="text-xs text-[var(--color-fg-muted)]">
            {source.last_synced_at
              ? `Last synced ${formatDate(source.last_synced_at)}`
              : "Not synced yet"}
          </span>
          {canSync ? (
            <div className="flex gap-2">
              <Button
                size="sm"
                variant="outline"
                disabled={exporting || syncDisabled}
                onClick={onExport}
              >
                <Download className="h-4 w-4" /> {exporting ? "Exporting…" : "Export"}
              </Button>
              <Button size="sm" disabled={checking || syncDisabled} onClick={onCheck}>
                <RefreshCw className={`h-4 w-4 ${checking ? "animate-spin" : ""}`} />
                {checking
                  ? connected
                    ? "Syncing…"
                    : "Checking…"
                  : connected
                    ? "Sync now"
                    : "Check for updates"}
              </Button>
            </div>
          ) : null}
        </div>
      </CardContent>
    </Card>
  );
}

function ConnectSourceDialog({
  open,
  onOpenChange,
  repositoryId,
  connections,
  connectionsLoading,
  connectionsError,
  onCreated,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  repositoryId: string;
  connections: ConnectedAccount[];
  connectionsLoading: boolean;
  connectionsError: boolean;
  onCreated: () => void;
}) {
  const [provider, setProvider] = useState<RepositorySourceProvider>("github");
  const [connectionID, setConnectionID] = useState("");
  const [selectedDriveFile, setSelectedDriveFile] = useState("");
  const [externalRepository, setExternalRepository] = useState("");
  const [branch, setBranch] = useState("main");
  const [includes, setIncludes] = useState("docs/**\nREADME.md");
  const [excludes, setExcludes] = useState("node_modules/**");
  const [targetFolder, setTargetFolder] = useState("");
  const [targetDocument, setTargetDocument] = useState("");
  const [direction, setDirection] = useState("import");
  const [triggerMode, setTriggerMode] = useState("default_branch");
  const [formError, setFormError] = useState<string>();
  const selected = providers.find((item) => item.id === provider)!;
  const connectedProvider = isConnectedAccountProvider(provider);
  const providerConnections = connectedProvider
    ? connectedAccountsForProvider(connections, provider)
    : [];
  const usableConnections = providerConnections.filter(isUsableConnectedAccount);
  const selectedConnection = providerConnections.find(
    (connection) => connection.id === connectionID,
  );
  const eventProvider = isEventProvider(provider);
  const locationCopy = sourceLocationCopy(provider);
  const documents = useQuery({
    queryKey: queryKeys.repositoryDocuments(repositoryId),
    queryFn: () => api.listRepositoryDocuments(repositoryId),
    enabled: open,
  });
  const create = useMutation({
    mutationFn: () => {
      const input: RepositorySourceInput = {
        provider,
        connection_id: connectedProvider ? connectionID : undefined,
        external_repository: externalRepository.trim() || undefined,
        branch: isGitProvider(provider) ? branch.trim() || "main" : undefined,
        include_paths: eventProvider ? undefined : splitPaths(includes),
        exclude_paths: eventProvider ? undefined : splitPaths(excludes),
        target_folder_id: targetFolder.trim() || undefined,
        target_document_id: targetDocument || undefined,
        direction: eventProvider
          ? "into_aidocs"
          : direction === "import"
            ? "into_aidocs"
            : direction === "export"
              ? "export_only"
              : "bidirectional",
        trigger_mode: eventProvider ? triggerMode || "scheduled" : triggerMode,
        status: "active",
      };
      return api.createRepositorySource(repositoryId, input);
    },
    onSuccess: () => {
      onCreated();
      onOpenChange(false);
      setFormError(undefined);
      toast.success(`${selected.label} connected.`);
    },
    onError: (error) => setFormError(errorMessage(error, "Could not connect source")),
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[92vh] max-w-3xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle>Connect a source</DialogTitle>
          <DialogDescription>
            Choose what to sync, where it lands, and how updates should be detected.
          </DialogDescription>
        </DialogHeader>
        <form
          className="space-y-5"
          onSubmit={(event) => {
            event.preventDefault();
            setFormError(undefined);
            create.mutate();
          }}
        >
          <div
            role="group"
            aria-label="Source provider"
            className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4"
          >
            {providers.map((item) => (
              <button
                key={item.id}
                type="button"
                aria-pressed={provider === item.id}
                onClick={() => {
                  setProvider(item.id);
                  setConnectionID("");
                  setSelectedDriveFile("");
                  setExternalRepository("");
                  setFormError(undefined);
                  if (item.id === "slack" || item.id === "devrev") {
                    setBranch("");
                    setIncludes("");
                    setExcludes("");
                    setDirection("import");
                    setTriggerMode("scheduled");
                  } else if (item.id === "github") {
                    setBranch("main");
                    setIncludes("docs/**\nREADME.md");
                    setExcludes("node_modules/**");
                    setTriggerMode("default_branch");
                  } else {
                    setTriggerMode("manual");
                  }
                }}
                className={`rounded-xl border p-3 text-left transition-colors ${
                  provider === item.id
                    ? "border-[var(--color-accent)] bg-[var(--color-accent-muted)]/60"
                    : "border-[var(--color-border)] hover:bg-[var(--color-surface-muted)]"
                }`}
              >
                <span className="text-[var(--color-fg-muted)]">{item.icon}</span>
                <span className="mt-2 block text-sm font-semibold">{item.label}</span>
                <span className="mt-1 block text-[11px] leading-4 text-[var(--color-fg-muted)]">
                  {item.description}
                </span>
              </button>
            ))}
          </div>

          {connectedProvider ? (
            <div className="rounded-xl border border-[var(--color-border)] bg-[var(--color-surface-muted)]/35 p-4">
              <div className="space-y-1.5">
                <Label htmlFor="source-connection">Connected account</Label>
                <select
                  id="source-connection"
                  value={connectionID}
                  onChange={(event) => setConnectionID(event.target.value)}
                  disabled={connectionsLoading || connectionsError}
                  required
                  className="h-9 w-full rounded-[10px] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 text-sm disabled:opacity-50"
                >
                  <option value="">
                    {connectionsLoading
                      ? "Loading your accounts…"
                      : usableConnections.length
                        ? `Choose a ${selected.label} account`
                        : `No usable ${selected.label} account connected`}
                  </option>
                  {providerConnections.map((connection) => (
                    <option
                      key={connection.id}
                      value={connection.id}
                      disabled={!isUsableConnectedAccount(connection)}
                    >
                      {connectedAccountLabel(connection)}
                      {connectedAccountContext(connection) !==
                      connectedAccountLabel(connection)
                        ? ` · ${connectedAccountContext(connection)}`
                        : ""}
                      {!isUsableConnectedAccount(connection)
                        ? ` · ${connection.status.replaceAll("_", " ")}`
                        : connection.status === "demo"
                          ? " · local demo"
                          : ""}
                    </option>
                  ))}
                </select>
                {connectionsError ? (
                  <p className="text-xs text-[var(--color-danger)]">
                    Could not load your connected accounts. Refresh and try again.
                  </p>
                ) : (
                  <p className="text-xs leading-5 text-[var(--color-fg-muted)]">
                    {usableConnections.length ? (
                      <>
                        This source uses your access. You can manage accounts in{" "}
                        <Link
                          to="/app/settings/connections"
                          className="font-medium text-[var(--color-fg)] underline-offset-2 hover:underline"
                        >
                          personal settings
                        </Link>
                        .
                      </>
                    ) : (
                      <>
                        <Link
                          to="/app/settings/connections"
                          className="font-medium text-[var(--color-accent)] underline-offset-2 hover:underline"
                        >
                          Connect {selected.label}
                        </Link>{" "}
                        first, then return here to choose the account.
                      </>
                    )}
                  </p>
                )}
              </div>
            </div>
          ) : null}

          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-1.5 sm:col-span-2">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <Label htmlFor="source-location">{locationCopy.label}</Label>
                {provider === "google_drive" &&
                selectedConnection &&
                selectedConnection.status !== "demo" ? (
                  <GoogleDrivePickerButton
                    connectionId={selectedConnection.id}
                    onPick={(location, name) => {
                      setExternalRepository(location);
                      setSelectedDriveFile(name || "Selected Drive file");
                    }}
                  />
                ) : null}
              </div>
              <Input
                id="source-location"
                value={externalRepository}
                onChange={(event) => {
                  setExternalRepository(event.target.value);
                  setSelectedDriveFile("");
                }}
                placeholder={locationCopy.placeholder}
                required={provider !== "manual"}
              />
              <p className="text-xs leading-5 text-[var(--color-fg-muted)]">
                {selectedDriveFile ? `Selected: ${selectedDriveFile}. ` : null}
                {provider === "google_drive" && selectedConnection?.status === "demo"
                  ? "Local demo: paste any Drive file URL to exercise the source review flow."
                  : locationCopy.hint}
              </p>
            </div>
            {isGitProvider(provider) ? (
              <div className="space-y-1.5">
                <Label htmlFor="source-branch">Branch</Label>
                <Input
                  id="source-branch"
                  value={branch}
                  onChange={(event) => setBranch(event.target.value)}
                  placeholder="main"
                />
              </div>
            ) : null}
            <div className="space-y-1.5">
              <Label htmlFor="source-target-document">Document to update</Label>
              <select
                id="source-target-document"
                value={targetDocument}
                onChange={(event) => setTargetDocument(event.target.value)}
                className="h-9 w-full rounded-[10px] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 text-sm"
              >
                <option value="">No single document</option>
                {(documents.data?.items || []).map((document) => (
                  <option key={document.id} value={document.id}>
                    {document.title}
                  </option>
                ))}
              </select>
              <p className="text-xs text-[var(--color-fg-muted)]">
                Selecting a document lets source checks open reviewable pull requests.
              </p>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="source-target">Target folder (optional)</Label>
              <Input
                id="source-target"
                value={targetFolder}
                onChange={(event) => setTargetFolder(event.target.value)}
                placeholder="Repository root"
              />
              <p className="text-xs text-[var(--color-fg-muted)]">
                Leave blank to place proposed context at the repository root.
              </p>
            </div>
            {!eventProvider ? (
              <>
                <div className="space-y-1.5">
                  <Label htmlFor="source-includes">
                    {isGitProvider(provider) ? "Paths to include" : "Items to include"}
                  </Label>
                  <Textarea
                    id="source-includes"
                    value={includes}
                    onChange={(event) => setIncludes(event.target.value)}
                    rows={3}
                    placeholder={
                      isGitProvider(provider) ? "docs/**" : "Optional filters"
                    }
                  />
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="source-excludes">
                    {isGitProvider(provider) ? "Paths to exclude" : "Items to exclude"}
                  </Label>
                  <Textarea
                    id="source-excludes"
                    value={excludes}
                    onChange={(event) => setExcludes(event.target.value)}
                    rows={3}
                    placeholder={
                      isGitProvider(provider) ? "archive/**" : "Optional filters"
                    }
                  />
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="source-direction">Direction</Label>
                  <select
                    id="source-direction"
                    value={direction}
                    onChange={(event) => setDirection(event.target.value)}
                    className="h-9 w-full rounded-[10px] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 text-sm"
                  >
                    <option value="import">Import into aidocs</option>
                    <option value="export">Export from aidocs</option>
                    <option value="bidirectional">Two-way proposals</option>
                  </select>
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="source-trigger">Update trigger</Label>
                  <select
                    id="source-trigger"
                    value={triggerMode}
                    onChange={(event) => setTriggerMode(event.target.value)}
                    className="h-9 w-full rounded-[10px] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 text-sm"
                  >
                    {isGitProvider(provider) ? (
                      <>
                        <option value="default_branch">Default branch updates</option>
                        <option value="merged_pull_requests">
                          Merged pull requests
                        </option>
                      </>
                    ) : null}
                    <option value="manual">Manual checks</option>
                    <option value="scheduled">Scheduled checks</option>
                  </select>
                </div>
              </>
            ) : (
              <div className="sm:col-span-2 rounded-xl border border-[var(--color-border)] bg-[var(--color-surface-muted)]/55 p-3">
                <p className="text-sm font-medium">Synced import</p>
                <p className="mt-1 text-xs leading-5 text-[var(--color-fg-muted)]">
                  {provider === "slack"
                    ? "Sync the selected Slack thread now or through scheduled checks. Changes are proposed in aidocs for review."
                    : "Sync this DevRev ticket now or through scheduled checks. Changes are proposed in aidocs for review."}
                </p>
              </div>
            )}
          </div>
          {formError ? (
            <p role="alert" className="text-sm text-[var(--color-danger)]">
              {formError}
            </p>
          ) : null}
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button
              type="submit"
              disabled={
                create.isPending ||
                (connectedProvider &&
                  (!connectionID || connectionsLoading || connectionsError))
              }
            >
              {create.isPending ? "Connecting…" : `Connect ${selected.label}`}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function SourceFact({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <dt className="text-[var(--color-fg-muted)]">{label}</dt>
      <dd className="mt-0.5 truncate font-medium capitalize">{value}</dd>
    </div>
  );
}

function StatusBadge({ status }: { status: string }) {
  const normalized = status.toLowerCase();
  const positive = [
    "ready",
    "fresh",
    "success",
    "completed",
    "connected",
    "active",
    "demo",
  ].some((value) => normalized.includes(value));
  const negative = ["failed", "error", "stale", "disconnected"].some((value) =>
    normalized.includes(value),
  );
  return (
    <Badge variant={negative ? "danger" : positive ? "success" : "muted"}>
      {negative ? (
        <AlertTriangle className="h-3 w-3" />
      ) : positive ? (
        <CheckCircle2 className="h-3 w-3" />
      ) : (
        <Clock3 className="h-3 w-3" />
      )}
      <span className="capitalize">{status.replaceAll("_", " ")}</span>
    </Badge>
  );
}

function isGitProvider(provider: RepositorySourceProvider) {
  return provider === "github";
}

function isEventProvider(provider: RepositorySourceProvider) {
  return provider === "slack" || provider === "devrev";
}

function sourceLocationCopy(provider: RepositorySourceProvider) {
  switch (provider) {
    case "github":
      return {
        label: "Repository URL or owner/name",
        placeholder: "razorpay/payments-docs",
        hint: "Connect the repository that contains the code or documentation this knowledge should follow.",
      };
    case "slack":
      return {
        label: "Slack thread URL",
        placeholder: "Paste a Slack thread link",
        hint: "Open the thread in Slack, copy its link, and paste it here. The selected account must be able to read it.",
      };
    case "devrev":
      return {
        label: "DevRev ticket",
        placeholder: "TKT-123 or a DevRev ticket URL",
        hint: "Paste a ticket ID or link that the selected DevRev account can access.",
      };
    case "google_drive":
      return {
        label: "Google Drive file",
        placeholder: "Choose a Drive file or paste an already granted file URL",
        hint: "Choose from Drive to grant access. A pasted URL works only when this aidocs connection already has access to that file.",
      };
    case "confluence":
      return {
        label: "Confluence space or page URL",
        placeholder: "Paste a Confluence space or page URL",
        hint: "Connect one page tree or an entire space as a source of context.",
      };
    case "notion":
      return {
        label: "Notion page URL",
        placeholder: "Paste a Notion page URL",
        hint: "The selected page and its children can be checked for changes.",
      };
    default:
      return {
        label: "Source label",
        placeholder: "Quarterly policy upload",
        hint: "Give this manual source a name people will recognize.",
      };
  }
}

function splitPaths(value: string) {
  return value
    .split(/[\n,]/)
    .map((item) => item.trim())
    .filter(Boolean);
}

function formatDate(value?: string) {
  if (!value) return "—";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}
