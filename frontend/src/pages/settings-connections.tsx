import { useEffect, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Cloud,
  ExternalLink,
  GitFork,
  Link2,
  MessageSquareText,
  Plus,
  Ticket,
  Trash2,
  type LucideIcon,
} from "lucide-react";
import { toast } from "sonner";
import { api, type ConnectedAccount, type ConnectedAccountProvider } from "@/api";
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
import { Input, Label } from "@/components/ui/input";
import { EmptyState, Skeleton } from "@/components/ui/misc";
import {
  connectedAccountContext,
  connectedAccountLabel,
  connectedAccountsForProvider,
} from "@/lib/connected-accounts";
import { errorMessage } from "@/lib/errors";
import { queryKeys } from "@/lib/queryKeys";
import { useTitle } from "@/lib/use-title";

type ProviderDefinition = {
  id: ConnectedAccountProvider;
  label: string;
  description: string;
  icon: LucideIcon;
};

const providers: ProviderDefinition[] = [
  {
    id: "github",
    label: "GitHub",
    description: "Use repositories you can access with your GitHub account.",
    icon: GitFork,
  },
  {
    id: "slack",
    label: "Slack",
    description: "Connect a workspace and sync threads that you can read.",
    icon: MessageSquareText,
  },
  {
    id: "google_drive",
    label: "Google Drive",
    description: "Select Drive files that should stay connected to your docs.",
    icon: Cloud,
  },
  {
    id: "devrev",
    label: "DevRev",
    description: "Use your personal access token to connect tickets you can access.",
    icon: Ticket,
  },
];

export function ConnectedAccountsSettingsPage() {
  useTitle("Connected accounts");
  const qc = useQueryClient();
  const [searchParams, setSearchParams] = useSearchParams();
  const handledConnection = useRef("");
  const [devRevOpen, setDevRevOpen] = useState(false);
  const connections = useQuery({
    queryKey: queryKeys.connectedAccounts(),
    queryFn: api.listConnections,
  });
  const disconnect = useMutation({
    mutationFn: (connection: ConnectedAccount) => api.deleteConnection(connection.id),
    onSuccess: (_, connection) => {
      qc.invalidateQueries({ queryKey: queryKeys.connectedAccounts() });
      qc.invalidateQueries({ queryKey: queryKeys.repositorySourcesAll() });
      toast.success(`${connectedAccountLabel(connection)} disconnected.`);
    },
    onError: (error) =>
      toast.error(errorMessage(error, "Could not disconnect account")),
  });

  const items = connections.data?.connections || [];
  useEffect(() => {
    const connected = searchParams.get("connected");
    const provider = providers.find((item) => item.id === connected);
    if (!provider || handledConnection.current === connected) return;
    handledConnection.current = connected || "";
    toast.success(`${provider.label} connected.`);
    const next = new URLSearchParams(searchParams);
    next.delete("connected");
    setSearchParams(next, { replace: true });
  }, [searchParams, setSearchParams]);

  return (
    <div className="p-6 sm:p-8">
      <div className="flex items-center gap-3">
        <Link2 className="h-5 w-5 text-[var(--color-accent)]" />
        <div>
          <h2 className="text-lg font-semibold">Connections</h2>
          <p className="text-sm text-[var(--color-fg-muted)]">
            Link your own accounts once, then choose them when connecting a repository
            source.
          </p>
        </div>
      </div>

      {connections.isLoading ? (
        <div className="mt-6 grid gap-4 xl:grid-cols-2">
          {providers.map((provider) => (
            <Skeleton key={provider.id} className="h-48 w-full" />
          ))}
        </div>
      ) : connections.error ? (
        <EmptyState
          className="mt-6"
          title="Could not load your connections"
          description="Refresh the page or try again."
          action={
            <Button variant="outline" onClick={() => connections.refetch()}>
              Try again
            </Button>
          }
        />
      ) : (
        <div className="mt-6 grid gap-4 xl:grid-cols-2">
          {providers.map((provider) => (
            <ProviderCard
              key={provider.id}
              provider={provider}
              connections={connectedAccountsForProvider(items, provider.id)}
              disconnectingId={
                disconnect.isPending ? disconnect.variables?.id : undefined
              }
              onConnectDevRev={() => setDevRevOpen(true)}
              onDisconnect={(connection) => disconnect.mutate(connection)}
            />
          ))}
        </div>
      )}

      <DevRevConnectionDialog
        open={devRevOpen}
        onOpenChange={setDevRevOpen}
        onConnected={() => {
          qc.invalidateQueries({ queryKey: queryKeys.connectedAccounts() });
          toast.success("DevRev connected.");
        }}
      />
    </div>
  );
}

function ProviderCard({
  provider,
  connections,
  disconnectingId,
  onConnectDevRev,
  onDisconnect,
}: {
  provider: ProviderDefinition;
  connections: ConnectedAccount[];
  disconnectingId?: string;
  onConnectDevRev: () => void;
  onDisconnect: (connection: ConnectedAccount) => void;
}) {
  const Icon = provider.icon;
  const connectLabel = connections.length
    ? `Connect another`
    : `Connect ${provider.label}`;
  return (
    <Card className="flex min-h-48 flex-col">
      <CardHeader className="flex flex-row items-start justify-between gap-3">
        <div className="flex min-w-0 items-start gap-3">
          <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-[var(--color-surface-muted)] text-[var(--color-fg-muted)]">
            <Icon className="h-5 w-5" />
          </span>
          <div className="min-w-0">
            <CardTitle>{provider.label}</CardTitle>
            <p className="mt-1 text-xs leading-5 text-[var(--color-fg-muted)]">
              {provider.description}
            </p>
          </div>
        </div>
        {provider.id === "devrev" ? (
          <Button size="sm" variant="outline" onClick={onConnectDevRev}>
            <Plus className="h-3.5 w-3.5" /> {connectLabel}
          </Button>
        ) : (
          <Button asChild size="sm" variant="outline">
            <a href={api.connectionStartURL(provider.id, "/app/settings/connections")}>
              <ExternalLink className="h-3.5 w-3.5" /> {connectLabel}
            </a>
          </Button>
        )}
      </CardHeader>
      <CardContent className="mt-auto space-y-2">
        {connections.length ? (
          connections.map((connection) => (
            <ConnectionRow
              key={connection.id}
              connection={connection}
              disconnecting={disconnectingId === connection.id}
              onDisconnect={() => onDisconnect(connection)}
            />
          ))
        ) : (
          <div className="rounded-lg border border-dashed border-[var(--color-border)] px-3 py-4 text-center text-xs text-[var(--color-fg-muted)]">
            No {provider.label} account connected yet.
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function ConnectionRow({
  connection,
  disconnecting,
  onDisconnect,
}: {
  connection: ConnectedAccount;
  disconnecting: boolean;
  onDisconnect: () => void;
}) {
  const context = connectedAccountContext(connection);
  const label = connectedAccountLabel(connection);
  return (
    <div className="flex items-center gap-3 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-muted)]/35 px-3 py-2.5">
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-2">
          <span className="truncate text-sm font-medium">{label}</span>
          <ConnectionStatus status={connection.status} />
        </div>
        {context && context !== label ? (
          <div className="mt-0.5 truncate text-xs text-[var(--color-fg-muted)]">
            {context}
          </div>
        ) : null}
        {connection.expires_at ? (
          <div className="mt-0.5 text-[11px] text-[var(--color-fg-muted)]">
            Expires {formatDate(connection.expires_at)}
          </div>
        ) : null}
      </div>
      <AlertDialog>
        <AlertDialogTrigger asChild>
          <Button
            size="icon"
            variant="ghost"
            disabled={disconnecting}
            aria-label={`Disconnect ${label}`}
          >
            <Trash2 className="h-4 w-4 text-[var(--color-danger)]" />
          </Button>
        </AlertDialogTrigger>
        <AlertDialogContent>
          <AlertDialogTitle>Disconnect {label}?</AlertDialogTitle>
          <AlertDialogDescription>
            Sources using this account will stop syncing and keep their existing aidocs
            content. You can reconnect later.
          </AlertDialogDescription>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction variant="danger" onClick={onDisconnect}>
              Disconnect
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

function ConnectionStatus({ status }: { status: string }) {
  const label = status.replaceAll("_", " ");
  const variant =
    status === "active"
      ? "success"
      : status === "demo"
        ? "accent"
        : status === "needs_reconnect"
          ? "warning"
          : "muted";
  return (
    <Badge variant={variant} className="capitalize">
      {label}
    </Badge>
  );
}

function DevRevConnectionDialog({
  open,
  onOpenChange,
  onConnected,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onConnected: () => void;
}) {
  const [token, setToken] = useState("");
  const [formError, setFormError] = useState<string>();
  const connect = useMutation({
    mutationFn: () => api.connectDevRev(token.trim()),
    onSuccess: () => {
      setToken("");
      setFormError(undefined);
      onConnected();
      onOpenChange(false);
    },
    onError: (error) => setFormError(errorMessage(error, "Could not connect DevRev")),
  });
  const setOpen = (next: boolean) => {
    if (!next && !connect.isPending) {
      setToken("");
      setFormError(undefined);
    }
    onOpenChange(next);
  };
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Connect DevRev</DialogTitle>
          <DialogDescription>
            DevRev uses a personal access token instead of a third-party OAuth flow. The
            token inherits your DevRev access and is never shown again.
          </DialogDescription>
        </DialogHeader>
        <form
          className="space-y-4"
          onSubmit={(event) => {
            event.preventDefault();
            setFormError(undefined);
            if (token.trim()) connect.mutate();
          }}
        >
          <div className="space-y-1.5">
            <Label htmlFor="devrev-personal-access-token">Personal access token</Label>
            <Input
              id="devrev-personal-access-token"
              type="password"
              value={token}
              onChange={(event) => setToken(event.target.value)}
              placeholder="Paste your DevRev token"
              autoComplete="off"
              autoCapitalize="off"
              autoCorrect="off"
              spellCheck={false}
              autoFocus
            />
          </div>
          {formError ? (
            <p role="alert" className="text-sm text-[var(--color-danger)]">
              {formError}
            </p>
          ) : null}
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={() => setOpen(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={!token.trim() || connect.isPending}>
              {connect.isPending ? "Connecting…" : "Connect DevRev"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function formatDate(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleDateString();
}
