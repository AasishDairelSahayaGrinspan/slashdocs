import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  AlertTriangle,
  Archive,
  Check,
  ExternalLink,
  Globe2,
  LockKeyhole,
  Plus,
  RotateCcw,
  Save,
  Settings2,
  ShieldCheck,
  Trash2,
  UserPlus,
  Users,
} from "lucide-react";
import { toast } from "sonner";
import {
  api,
  type Repository,
  type RepositorySettings as Settings,
  type RepositoryTeam,
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
import { Input, Label, Textarea } from "@/components/ui/input";
import { EmptyState, Skeleton } from "@/components/ui/misc";
import { errorMessage } from "@/lib/errors";
import { queryKeys } from "@/lib/queryKeys";

type SettingsSection = "general" | "reviews" | "teams" | "portal" | "danger";

const sections: Array<{
  id: SettingsSection;
  label: string;
  description: string;
  icon: React.ReactNode;
}> = [
  {
    id: "general",
    label: "General",
    description: "Freshness and identity",
    icon: <Settings2 className="h-4 w-4" />,
  },
  {
    id: "reviews",
    label: "Review rules",
    description: "Approvals and path owners",
    icon: <ShieldCheck className="h-4 w-4" />,
  },
  {
    id: "teams",
    label: "Teams",
    description: "Repository groups",
    icon: <Users className="h-4 w-4" />,
  },
  {
    id: "portal",
    label: "Portal",
    description: "Published knowledge site",
    icon: <Globe2 className="h-4 w-4" />,
  },
  {
    id: "danger",
    label: "Danger zone",
    description: "Archive, transfer, delete",
    icon: <AlertTriangle className="h-4 w-4" />,
  },
];

export function RepositorySettings({
  repository,
  canManage,
}: {
  repository: Repository;
  canManage: boolean;
}) {
  const [section, setSection] = useState<SettingsSection>("general");
  const settings = useQuery({
    queryKey: queryKeys.repositorySettings(repository.id),
    queryFn: () => api.getRepositorySettings(repository.id),
  });
  if (settings.isLoading) {
    return (
      <div className="grid gap-5 md:grid-cols-[220px_minmax(0,1fr)]">
        <Skeleton className="h-72 w-full" />
        <Skeleton className="h-96 w-full" />
      </div>
    );
  }
  if (settings.error || !settings.data) {
    return (
      <EmptyState
        icon={<AlertTriangle className="h-5 w-5" />}
        title="Could not load repository settings"
        description={settings.error ? errorMessage(settings.error) : undefined}
        action={
          <Button variant="outline" onClick={() => settings.refetch()}>
            Try again
          </Button>
        }
      />
    );
  }
  return (
    <div className="grid gap-6 md:grid-cols-[220px_minmax(0,1fr)]">
      <aside className="self-start rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] p-2 md:sticky md:top-4">
        {sections.map((item) => (
          <button
            key={item.id}
            type="button"
            onClick={() => setSection(item.id)}
            className={`flex w-full items-start gap-3 rounded-lg px-3 py-3 text-left transition-colors ${
              section === item.id
                ? "bg-[var(--color-accent-muted)] text-[var(--color-fg)]"
                : "text-[var(--color-fg-muted)] hover:bg-[var(--color-surface-muted)] hover:text-[var(--color-fg)]"
            }`}
          >
            <span className="mt-0.5">{item.icon}</span>
            <span>
              <span className="block text-sm font-medium">{item.label}</span>
              <span className="mt-0.5 block text-[11px] leading-4">
                {item.description}
              </span>
            </span>
          </button>
        ))}
      </aside>
      <main className="min-w-0">
        {!canManage ? (
          <div className="mb-4 flex items-center gap-2 rounded-xl border border-[var(--color-border)] bg-[var(--color-surface-muted)]/55 px-4 py-3 text-sm text-[var(--color-fg-muted)]">
            <LockKeyhole className="h-4 w-4" /> Settings are read-only. Repository owner
            access is required to make changes.
          </div>
        ) : null}
        {section === "general" ? (
          <GeneralSettings
            repository={repository}
            settings={settings.data}
            canManage={canManage}
          />
        ) : section === "reviews" ? (
          <ReviewSettings
            repositoryId={repository.id}
            settings={settings.data}
            canManage={canManage}
          />
        ) : section === "teams" ? (
          <TeamSettings repositoryId={repository.id} canManage={canManage} />
        ) : section === "portal" ? (
          <PortalSettings
            repository={repository}
            settings={settings.data}
            canManage={canManage}
          />
        ) : (
          <DangerSettings
            repository={repository}
            settings={settings.data}
            canManage={canManage}
          />
        )}
      </main>
    </div>
  );
}

function GeneralSettings({
  repository,
  settings,
  canManage,
}: {
  repository: Repository;
  settings: Settings;
  canManage: boolean;
}) {
  const qc = useQueryClient();
  const [staleAfter, setStaleAfter] = useState(
    String(settings.freshness_policy?.stale_after_hours || 720),
  );
  const [onStale, setOnStale] = useState(
    settings.freshness_policy?.on_stale || "notify",
  );
  useEffect(() => {
    setStaleAfter(String(settings.freshness_policy?.stale_after_hours || 720));
    setOnStale(settings.freshness_policy?.on_stale || "notify");
  }, [settings]);
  const save = useMutation({
    mutationFn: () =>
      api.updateRepositorySettings(repository.id, {
        freshness_policy: {
          stale_after_hours: Math.max(1, Number(staleAfter) || 720),
          on_stale: onStale,
        },
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.repositorySettings(repository.id) });
      toast.success("General settings saved.");
    },
  });
  return (
    <div className="space-y-5">
      <SettingsHeading
        title="General"
        description="Repository identity and knowledge freshness policy."
      />
      <Card>
        <CardHeader>
          <CardTitle>Repository identity</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-4 sm:grid-cols-2">
          <ReadOnlyField label="Name" value={repository.name} />
          <ReadOnlyField label="Slug" value={repository.slug} />
          <ReadOnlyField label="Owner scope" value={repository.scope} />
          <ReadOnlyField
            label="Review protection"
            value={repository.protected ? "Protected" : "Unprotected"}
          />
          <div className="sm:col-span-2">
            <ReadOnlyField
              label="Description"
              value={repository.description || "No description"}
            />
          </div>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>Freshness policy</CardTitle>
          <p className="mt-1 text-sm text-[var(--color-fg-muted)]">
            Identify docs that have not been validated recently and decide what happens.
          </p>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-1.5">
              <Label htmlFor="stale-after">Mark stale after (hours)</Label>
              <Input
                id="stale-after"
                type="number"
                min={1}
                value={staleAfter}
                onChange={(event) => setStaleAfter(event.target.value)}
                disabled={!canManage}
              />
              <p className="text-xs text-[var(--color-fg-muted)]">
                720 hours is about 30 days.
              </p>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="on-stale">When knowledge becomes stale</Label>
              <select
                id="on-stale"
                value={onStale}
                onChange={(event) => setOnStale(event.target.value)}
                disabled={!canManage}
                className="h-9 w-full rounded-[10px] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 text-sm disabled:opacity-50"
              >
                <option value="notify">Notify owners</option>
                <option value="open_pull_request">Open a review pull request</option>
                <option value="none">Track only, without notification</option>
              </select>
            </div>
          </div>
          {canManage ? (
            <Button onClick={() => save.mutate()} disabled={save.isPending}>
              <Save className="h-4 w-4" /> {save.isPending ? "Saving…" : "Save policy"}
            </Button>
          ) : null}
        </CardContent>
      </Card>
    </div>
  );
}

function ReviewSettings({
  repositoryId,
  settings,
  canManage,
}: {
  repositoryId: string;
  settings: Settings;
  canManage: boolean;
}) {
  const qc = useQueryClient();
  const [reviewers, setReviewers] = useState(
    (settings.required_reviewers || []).join(", "),
  );
  const [requiredTeams, setRequiredTeams] = useState(
    (settings.required_teams || []).join(", "),
  );
  const [rules, setRules] = useState(pathRulesToText(settings.path_rules || []));
  useEffect(() => {
    setReviewers((settings.required_reviewers || []).join(", "));
    setRequiredTeams((settings.required_teams || []).join(", "));
    setRules(pathRulesToText(settings.path_rules || []));
  }, [settings]);
  const parsedRules = useMemo(() => parsePathRules(rules), [rules]);
  const save = useMutation({
    mutationFn: () =>
      api.updateRepositorySettings(repositoryId, {
        required_reviewers: splitIdentifiers(reviewers),
        required_teams: splitIdentifiers(requiredTeams),
        path_rules: parsedRules,
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.repositorySettings(repositoryId) });
      qc.invalidateQueries({ queryKey: queryKeys.repository(repositoryId) });
      toast.success("Review rules saved.");
    },
  });
  return (
    <div className="space-y-5">
      <SettingsHeading
        title="Review rules"
        description="Set baseline approvals and CODEOWNERS-like requirements for sensitive paths."
      />
      <Card>
        <CardHeader>
          <CardTitle>Required people and teams</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-1.5">
            <Label htmlFor="required-reviewers">Reviewer user IDs</Label>
            <Input
              id="required-reviewers"
              value={reviewers}
              onChange={(event) => setReviewers(event.target.value)}
              disabled={!canManage}
              placeholder="user_123, owner@example.com"
            />
            <p className="text-xs text-[var(--color-fg-muted)]">
              These reviewers are requested on every pull request.
            </p>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="required-teams">Required team IDs</Label>
            <Input
              id="required-teams"
              value={requiredTeams}
              onChange={(event) => setRequiredTeams(event.target.value)}
              disabled={!canManage}
              placeholder="team_compliance, team_security"
            />
            <p className="text-xs text-[var(--color-fg-muted)]">
              Create and manage team membership in the Teams section.
            </p>
          </div>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>Path owners</CardTitle>
          <p className="mt-1 text-sm text-[var(--color-fg-muted)]">
            One rule per line:{" "}
            <code className="rounded bg-[var(--color-surface-muted)] px-1 py-0.5">
              docs/payments/** @owner@example.com @team:compliance
            </code>
          </p>
        </CardHeader>
        <CardContent className="space-y-4">
          <Textarea
            value={rules}
            onChange={(event) => setRules(event.target.value)}
            rows={9}
            spellCheck={false}
            disabled={!canManage}
            className="font-mono text-xs"
            placeholder="policies/** @team:legal"
          />
          {parsedRules.length ? (
            <div className="overflow-hidden rounded-lg border border-[var(--color-border)]">
              {parsedRules.map((rule, index) => (
                <div
                  key={`${rule.pattern}-${index}`}
                  className="flex flex-wrap items-center gap-2 border-b border-[var(--color-border)] px-3 py-2 text-xs last:border-b-0"
                >
                  <code className="font-semibold">{rule.pattern}</code>
                  {(rule.required_reviewers || []).map((reviewer) => (
                    <Badge key={reviewer} variant="muted">
                      @{reviewer}
                    </Badge>
                  ))}
                  {(rule.required_teams || []).map((team) => (
                    <Badge key={team} variant="accent">
                      team:{team}
                    </Badge>
                  ))}
                </div>
              ))}
            </div>
          ) : null}
          {canManage ? (
            <Button onClick={() => save.mutate()} disabled={save.isPending}>
              <Save className="h-4 w-4" />{" "}
              {save.isPending ? "Saving…" : "Save review rules"}
            </Button>
          ) : null}
        </CardContent>
      </Card>
    </div>
  );
}

function TeamSettings({
  repositoryId,
  canManage,
}: {
  repositoryId: string;
  canManage: boolean;
}) {
  const qc = useQueryClient();
  const [createOpen, setCreateOpen] = useState(false);
  const teams = useQuery({
    queryKey: queryKeys.repositoryTeams(repositoryId),
    queryFn: () => api.listRepositoryTeams(repositoryId),
  });
  const refresh = () =>
    qc.invalidateQueries({ queryKey: queryKeys.repositoryTeams(repositoryId) });
  const remove = useMutation({
    mutationFn: (teamId: string) => api.deleteRepositoryTeam(repositoryId, teamId),
    onSuccess: () => {
      refresh();
      toast.success("Team removed.");
    },
  });
  return (
    <div className="space-y-5">
      <div className="flex items-end justify-between gap-3">
        <SettingsHeading
          title="Teams"
          description="Group reviewers so path rules can require the right subject-matter experts."
        />
        {canManage ? (
          <Button onClick={() => setCreateOpen(true)}>
            <Plus className="h-4 w-4" /> New team
          </Button>
        ) : null}
      </div>
      {teams.isLoading ? (
        <Skeleton className="h-64 w-full" />
      ) : teams.error ? (
        <EmptyState
          title="Could not load teams"
          action={
            <Button variant="outline" onClick={() => teams.refetch()}>
              Try again
            </Button>
          }
        />
      ) : (teams.data?.items || []).length === 0 ? (
        <EmptyState
          icon={<Users className="h-5 w-5" />}
          title="No repository teams"
          description="Create teams for owners, reviewers, and subject-matter experts."
          action={
            canManage ? (
              <Button onClick={() => setCreateOpen(true)}>
                <Plus className="h-4 w-4" /> Create team
              </Button>
            ) : undefined
          }
        />
      ) : (
        <div className="grid gap-3 lg:grid-cols-2">
          {(teams.data?.items || []).map((team) => (
            <TeamCard
              key={team.id}
              repositoryId={repositoryId}
              team={team}
              canManage={canManage}
              onChanged={refresh}
              onDelete={() => {
                if (window.confirm(`Delete ${team.name}?`)) remove.mutate(team.id);
              }}
            />
          ))}
        </div>
      )}
      <CreateTeamDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        repositoryId={repositoryId}
        onCreated={refresh}
      />
    </div>
  );
}

function TeamCard({
  repositoryId,
  team,
  canManage,
  onChanged,
  onDelete,
}: {
  repositoryId: string;
  team: RepositoryTeam;
  canManage: boolean;
  onChanged: () => void;
  onDelete: () => void;
}) {
  const [userId, setUserId] = useState("");
  const memberIds = team.member_ids || (team.members || []).map((member) => member.id);
  const add = useMutation({
    mutationFn: () => api.addRepositoryTeamMember(repositoryId, team.id, userId.trim()),
    onSuccess: () => {
      setUserId("");
      onChanged();
    },
  });
  const remove = useMutation({
    mutationFn: (id: string) =>
      api.removeRepositoryTeamMember(repositoryId, team.id, id),
    onSuccess: onChanged,
  });
  return (
    <Card>
      <CardHeader className="flex-row items-start justify-between gap-3">
        <div>
          <CardTitle>{team.name}</CardTitle>
          <p className="mt-1 text-sm text-[var(--color-fg-muted)]">
            {team.description || "No description"}
          </p>
        </div>
        <Badge variant="muted">
          {memberIds.length} member{memberIds.length === 1 ? "" : "s"}
        </Badge>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="space-y-2">
          {memberIds.length === 0 ? (
            <p className="text-sm text-[var(--color-fg-muted)]">No members yet.</p>
          ) : (
            memberIds.map((memberId) => (
              <div
                key={memberId}
                className="flex items-center justify-between gap-3 rounded-lg bg-[var(--color-surface-muted)]/55 px-3 py-2 text-sm"
              >
                <span className="truncate">{memberId}</span>
                {canManage ? (
                  <button
                    type="button"
                    onClick={() => remove.mutate(memberId)}
                    className="text-xs text-[var(--color-danger)] hover:underline"
                  >
                    Remove
                  </button>
                ) : null}
              </div>
            ))
          )}
        </div>
        {canManage ? (
          <div className="flex gap-2 border-t border-[var(--color-border)] pt-3">
            <Input
              value={userId}
              onChange={(event) => setUserId(event.target.value)}
              placeholder="User ID"
            />
            <Button
              size="icon"
              variant="outline"
              aria-label="Add team member"
              disabled={!userId.trim() || add.isPending}
              onClick={() => add.mutate()}
            >
              <UserPlus className="h-4 w-4" />
            </Button>
            <Button
              size="icon"
              variant="ghost"
              aria-label="Delete team"
              onClick={onDelete}
            >
              <Trash2 className="h-4 w-4 text-[var(--color-danger)]" />
            </Button>
          </div>
        ) : null}
      </CardContent>
    </Card>
  );
}

function CreateTeamDialog({
  open,
  onOpenChange,
  repositoryId,
  onCreated,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  repositoryId: string;
  onCreated: () => void;
}) {
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const create = useMutation({
    mutationFn: () =>
      api.createRepositoryTeam(repositoryId, {
        name: name.trim(),
        description: description.trim() || undefined,
      }),
    onSuccess: () => {
      onCreated();
      onOpenChange(false);
      setName("");
      setDescription("");
      toast.success("Team created.");
    },
  });
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>New repository team</DialogTitle>
          <DialogDescription>
            Create a reusable group for access and review rules.
          </DialogDescription>
        </DialogHeader>
        <form
          className="space-y-4"
          onSubmit={(event) => {
            event.preventDefault();
            if (name.trim()) create.mutate();
          }}
        >
          <div className="space-y-1.5">
            <Label htmlFor="team-name">Name</Label>
            <Input
              id="team-name"
              value={name}
              onChange={(event) => setName(event.target.value)}
              placeholder="Compliance reviewers"
              autoFocus
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="team-description">Description</Label>
            <Textarea
              id="team-description"
              value={description}
              onChange={(event) => setDescription(event.target.value)}
              rows={3}
            />
          </div>
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={!name.trim() || create.isPending}>
              {create.isPending ? "Creating…" : "Create team"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function PortalSettings({
  repository,
  settings,
  canManage,
}: {
  repository: Repository;
  settings: Settings;
  canManage: boolean;
}) {
  const qc = useQueryClient();
  const [enabled, setEnabled] = useState(settings.portal?.enabled || false);
  const [slug, setSlug] = useState(settings.portal?.slug || repository.slug);
  const [visibility, setVisibility] = useState(settings.portal?.visibility || "public");
  useEffect(() => {
    setEnabled(settings.portal?.enabled || false);
    setSlug(settings.portal?.slug || repository.slug);
    setVisibility(settings.portal?.visibility || "public");
  }, [settings, repository.slug]);
  const save = useMutation({
    mutationFn: () =>
      api.updateRepositorySettings(repository.id, {
        portal: { enabled, slug: portalSlug(slug), visibility },
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.repositorySettings(repository.id) });
      toast.success("Portal settings saved.");
    },
  });
  const previewSlug = portalSlug(slug);
  return (
    <div className="space-y-5">
      <SettingsHeading
        title="Knowledge portal"
        description="Publish approved documents as a browsable, branded knowledge site."
      />
      <Card>
        <CardContent className="space-y-5 pt-5">
          <label className="flex items-start justify-between gap-4 rounded-xl border border-[var(--color-border)] p-4">
            <span>
              <span className="block font-medium">Publish portal</span>
              <span className="mt-1 block text-sm text-[var(--color-fg-muted)]">
                Only merged, current document versions are shown.
              </span>
            </span>
            <input
              type="checkbox"
              checked={enabled}
              onChange={(event) => setEnabled(event.target.checked)}
              disabled={!canManage}
              className="mt-1 h-4 w-4 accent-[var(--color-accent)]"
            />
          </label>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-1.5">
              <Label htmlFor="portal-slug">Portal address</Label>
              <div className="flex items-center rounded-[10px] border border-[var(--color-border)] bg-[var(--color-surface)] pl-3">
                <span className="text-xs text-[var(--color-fg-muted)]">/portal/</span>
                <Input
                  id="portal-slug"
                  value={slug}
                  onChange={(event) => setSlug(portalSlug(event.target.value))}
                  disabled={!canManage}
                  className="border-0 bg-transparent focus-visible:ring-0"
                />
              </div>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="portal-visibility">Audience</Label>
              <select
                id="portal-visibility"
                value={visibility}
                onChange={(event) => setVisibility(event.target.value)}
                disabled={!canManage}
                className="h-9 w-full rounded-[10px] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 text-sm"
              >
                <option value="public">Public internet</option>
                <option value="organization">Organization only</option>
                <option value="private">Invited people</option>
              </select>
            </div>
          </div>
          <div className="flex flex-wrap gap-2">
            {canManage ? (
              <Button
                onClick={() => save.mutate()}
                disabled={!previewSlug || save.isPending}
              >
                <Save className="h-4 w-4" />{" "}
                {save.isPending ? "Saving…" : "Save portal"}
              </Button>
            ) : null}
            {enabled && previewSlug && visibility === "public" ? (
              <Button asChild variant="outline">
                <Link to={`/portal/${previewSlug}`} target="_blank">
                  <ExternalLink className="h-4 w-4" /> Open portal
                </Link>
              </Button>
            ) : null}
          </div>
          {enabled && visibility !== "public" ? (
            <p className="text-xs text-[var(--color-fg-muted)]">
              Organization and invited-person portal links become available after an
              authenticated portal audience is configured.
            </p>
          ) : null}
        </CardContent>
      </Card>
    </div>
  );
}

function DangerSettings({
  repository,
  settings,
  canManage,
}: {
  repository: Repository;
  settings: Settings;
  canManage: boolean;
}) {
  const nav = useNavigate();
  const qc = useQueryClient();
  const me = useQuery({ queryKey: queryKeys.me(), queryFn: api.me });
  const [transfer, setTransfer] = useState("");
  const [confirmDelete, setConfirmDelete] = useState("");
  const archive = useMutation({
    mutationFn: () =>
      settings.archived
        ? api.restoreRepository(repository.id)
        : api.archiveRepository(repository.id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.repositorySettings(repository.id) });
      qc.invalidateQueries({ queryKey: queryKeys.repositories() });
      toast.success(
        settings.archived ? "Repository restored." : "Repository archived.",
      );
    },
  });
  const transferMutation = useMutation({
    mutationFn: () => api.transferRepository(repository.id, transfer.trim()),
    onSuccess: () => {
      qc.invalidateQueries({
        queryKey: queryKeys.repositorySettings(repository.id),
      });
      setTransfer("");
      toast.success("Transfer request sent.");
    },
  });
  const transferDecision = useMutation({
    mutationFn: (decision: "accept" | "cancel") =>
      decision === "accept"
        ? api.acceptRepositoryTransfer(repository.id)
        : api.cancelRepositoryTransfer(repository.id),
    onSuccess: (_, decision) => {
      qc.invalidateQueries({
        queryKey: queryKeys.repositorySettings(repository.id),
      });
      qc.invalidateQueries({ queryKey: queryKeys.repository(repository.id) });
      qc.invalidateQueries({ queryKey: queryKeys.repositories() });
      toast.success(
        decision === "accept" ? "Transfer accepted." : "Transfer cancelled.",
      );
    },
  });
  const transferPending = settings.transfer?.status === "pending";
  const isTransferTarget =
    transferPending && settings.transfer?.owner_id === me.data?.principal.id;
  const remove = useMutation({
    mutationFn: () => api.deleteRepository(repository.id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.repositories() });
      toast.success("Repository deleted.");
      nav("/app/repositories", { replace: true });
    },
  });
  return (
    <div className="space-y-5">
      <SettingsHeading
        title="Danger zone"
        description="These actions affect the entire repository and everyone who uses it."
      />
      <div className="overflow-hidden rounded-xl border border-[var(--color-danger)]/45 bg-[var(--color-surface)]">
        <DangerRow
          title={settings.archived ? "Restore repository" : "Archive repository"}
          description={
            settings.archived
              ? "Make this repository active and writable again."
              : "Make documents read-only while preserving history, links, and the portal."
          }
          action={
            <Button
              variant="outline"
              disabled={!canManage || archive.isPending}
              onClick={() => archive.mutate()}
            >
              {settings.archived ? (
                <RotateCcw className="h-4 w-4" />
              ) : (
                <Archive className="h-4 w-4" />
              )}
              {settings.archived ? "Restore" : "Archive"}
            </Button>
          }
        />
        <DangerRow
          title="Transfer repository"
          description={
            settings.transfer?.status === "pending"
              ? `Transfer pending for ${settings.transfer.owner_email || settings.transfer.owner_id || "the new owner"}.`
              : "Request ownership transfer to another aidocs user. They must accept before ownership changes."
          }
          action={
            transferPending ? (
              <div className="flex gap-2">
                {isTransferTarget ? (
                  <Button
                    onClick={() => transferDecision.mutate("accept")}
                    disabled={transferDecision.isPending}
                  >
                    <Check className="h-4 w-4" /> Accept transfer
                  </Button>
                ) : null}
                {canManage ? (
                  <Button
                    variant="outline"
                    onClick={() => transferDecision.mutate("cancel")}
                    disabled={transferDecision.isPending}
                  >
                    Cancel request
                  </Button>
                ) : null}
              </div>
            ) : (
              <div className="flex gap-2">
                <Input
                  value={transfer}
                  onChange={(event) => setTransfer(event.target.value)}
                  placeholder="User ID or email"
                  disabled={!canManage}
                  className="w-44"
                />
                <Button
                  variant="outline"
                  disabled={
                    !canManage || !transfer.trim() || transferMutation.isPending
                  }
                  onClick={() => transferMutation.mutate()}
                >
                  Request transfer
                </Button>
              </div>
            )
          }
        />
        <DangerRow
          title="Delete repository"
          description="Permanently remove repository settings, source links, and review history. This cannot be undone."
          action={
            <div className="space-y-2">
              <Input
                value={confirmDelete}
                onChange={(event) => setConfirmDelete(event.target.value)}
                placeholder={`Type ${repository.slug}`}
                disabled={!canManage}
                className="w-52"
              />
              <Button
                variant="danger"
                className="w-full"
                disabled={
                  !canManage || confirmDelete !== repository.slug || remove.isPending
                }
                onClick={() => remove.mutate()}
              >
                <Trash2 className="h-4 w-4" />{" "}
                {remove.isPending ? "Deleting…" : "Delete repository"}
              </Button>
            </div>
          }
          last
        />
      </div>
    </div>
  );
}

function DangerRow({
  title,
  description,
  action,
  last = false,
}: {
  title: string;
  description: string;
  action: React.ReactNode;
  last?: boolean;
}) {
  return (
    <div
      className={`flex flex-col gap-4 p-5 sm:flex-row sm:items-center sm:justify-between ${last ? "" : "border-b border-[var(--color-border)]"}`}
    >
      <div>
        <h3 className="font-semibold">{title}</h3>
        <p className="mt-1 max-w-xl text-sm text-[var(--color-fg-muted)]">
          {description}
        </p>
      </div>
      <div className="shrink-0">{action}</div>
    </div>
  );
}

function SettingsHeading({
  title,
  description,
}: {
  title: string;
  description: string;
}) {
  return (
    <div>
      <h2 className="text-lg font-semibold">{title}</h2>
      <p className="mt-1 text-sm text-[var(--color-fg-muted)]">{description}</p>
    </div>
  );
}

function ReadOnlyField({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <div className="text-xs font-medium text-[var(--color-fg-muted)]">{label}</div>
      <div className="mt-1 rounded-lg bg-[var(--color-surface-muted)] px-3 py-2 text-sm capitalize">
        {value}
      </div>
    </div>
  );
}

function pathRulesToText(rules: NonNullable<Settings["path_rules"]>) {
  return rules
    .map((rule) =>
      [
        rule.pattern,
        ...(rule.required_reviewers || []).map((reviewer) => `@${reviewer}`),
        ...(rule.required_teams || []).map((team) => `@team:${team}`),
      ]
        .filter(Boolean)
        .join(" "),
    )
    .join("\n");
}

function parsePathRules(value: string): NonNullable<Settings["path_rules"]> {
  return value
    .split("\n")
    .map((line) => line.trim())
    .filter((line) => line && !line.startsWith("#"))
    .map((line) => {
      const parts = line.split(/\s+/);
      const reviewers: string[] = [];
      const teams: string[] = [];
      for (const part of parts.slice(1)) {
        if (part.startsWith("@team:")) teams.push(part.slice(6));
        else if (part.startsWith("@")) reviewers.push(part.slice(1));
      }
      return {
        pattern: parts[0],
        required_reviewers: reviewers,
        required_teams: teams,
      };
    });
}

function splitIdentifiers(value: string) {
  return value
    .split(/[\n,]/)
    .map((item) => item.trim())
    .filter(Boolean);
}

function portalSlug(value: string) {
  return value
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 80);
}
