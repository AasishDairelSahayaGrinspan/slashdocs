import { useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import {
  Activity,
  AlertTriangle,
  CheckCircle2,
  Cloud,
  FileText,
  GitPullRequest,
  Settings2,
  Users,
} from "lucide-react";
import { api, type RepositoryActivity as ActivityItem } from "@/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { EmptyState, Skeleton } from "@/components/ui/misc";
import { Avatar } from "@/components/ui/misc";
import { queryKeys } from "@/lib/queryKeys";

const filters = ["all", "documents", "pull requests", "sources", "settings"] as const;

export function RepositoryActivity({ repositoryId }: { repositoryId: string }) {
  const [filter, setFilter] = useState<(typeof filters)[number]>("all");
  const activity = useQuery({
    queryKey: queryKeys.repositoryActivity(repositoryId),
    queryFn: () => api.listRepositoryActivity(repositoryId),
  });
  const items = useMemo(() => {
    const sorted = [...(activity.data?.items || [])].sort((a, b) =>
      activityDate(b).localeCompare(activityDate(a)),
    );
    return filter === "all"
      ? sorted
      : sorted.filter((item) => activityGroup(activityType(item)) === filter);
  }, [activity.data, filter]);

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h2 className="text-lg font-semibold">Repository activity</h2>
          <p className="mt-1 text-sm text-[var(--color-fg-muted)]">
            A durable audit trail of document, review, source, and access changes.
          </p>
        </div>
        <div className="flex max-w-full gap-1 overflow-x-auto rounded-lg bg-[var(--color-surface-muted)] p-1">
          {filters.map((item) => (
            <button
              key={item}
              type="button"
              onClick={() => setFilter(item)}
              className={`shrink-0 rounded-md px-3 py-1.5 text-xs font-medium capitalize transition-colors ${
                filter === item
                  ? "bg-[var(--color-surface)] text-[var(--color-fg)] shadow-sm"
                  : "text-[var(--color-fg-muted)] hover:text-[var(--color-fg)]"
              }`}
            >
              {item}
            </button>
          ))}
        </div>
      </div>

      {activity.isLoading ? (
        <div className="space-y-3">
          <Skeleton className="h-24 w-full" />
          <Skeleton className="h-24 w-full" />
          <Skeleton className="h-24 w-full" />
        </div>
      ) : activity.error ? (
        <EmptyState
          icon={<AlertTriangle className="h-5 w-5" />}
          title="Could not load activity"
          action={
            <Button variant="outline" onClick={() => activity.refetch()}>
              Try again
            </Button>
          }
        />
      ) : items.length === 0 ? (
        <EmptyState
          icon={<Activity className="h-5 w-5" />}
          title={filter === "all" ? "No activity yet" : `No ${filter} activity`}
          description="Actions in this repository will appear here with their actor and time."
        />
      ) : (
        <div className="relative rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] px-4">
          <div className="absolute bottom-8 left-[37px] top-8 w-px bg-[var(--color-border)]" />
          <ol className="relative divide-y divide-[var(--color-border)]">
            {items.map((item) => (
              <ActivityRow key={item.id} item={item} repositoryId={repositoryId} />
            ))}
          </ol>
        </div>
      )}
    </div>
  );
}

function ActivityRow({
  item,
  repositoryId,
}: {
  item: ActivityItem;
  repositoryId: string;
}) {
  const href = activityHref(item, repositoryId);
  const body = (
    <div className="min-w-0 flex-1">
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-medium">
          {item.title || activityTitle(activityType(item))}
        </span>
        <Badge variant="muted" className="capitalize">
          {activityGroup(activityType(item))}
        </Badge>
      </div>
      {item.description ? (
        <p className="mt-1 text-sm leading-6 text-[var(--color-fg-muted)]">
          {item.description}
        </p>
      ) : null}
      <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-[var(--color-fg-muted)]">
        {item.actor ? (
          <>
            <Avatar
              size={20}
              src={item.actor.picture_url}
              name={item.actor.name || item.actor.email || item.actor.id}
            />
            <span>{item.actor.name || item.actor.email || item.actor.id}</span>
            <span>·</span>
          </>
        ) : null}
        <time>{formatDate(activityDate(item))}</time>
      </div>
    </div>
  );
  return (
    <li className="flex gap-4 py-5">
      <div className="z-10 flex h-10 w-10 shrink-0 items-center justify-center rounded-full border border-[var(--color-border)] bg-[var(--color-surface)] text-[var(--color-fg-muted)]">
        {activityIcon(activityType(item))}
      </div>
      {href ? (
        <Link to={href} className="min-w-0 flex-1 rounded-lg hover:underline">
          {body}
        </Link>
      ) : (
        body
      )}
    </li>
  );
}

function activityGroup(type: string): (typeof filters)[number] {
  const value = type.toLowerCase();
  if (value.includes("pull") || value.includes("review") || value.includes("merge")) {
    return "pull requests";
  }
  if (value.includes("source") || value.includes("sync") || value.includes("export")) {
    return "sources";
  }
  if (
    value.includes("setting") ||
    value.includes("team") ||
    value.includes("access") ||
    value.includes("portal")
  ) {
    return "settings";
  }
  return "documents";
}

function activityIcon(type: string) {
  switch (activityGroup(type)) {
    case "pull requests":
      return type.includes("merge") ? (
        <CheckCircle2 className="h-4 w-4" />
      ) : (
        <GitPullRequest className="h-4 w-4" />
      );
    case "sources":
      return <Cloud className="h-4 w-4" />;
    case "settings":
      return type.includes("team") ? (
        <Users className="h-4 w-4" />
      ) : (
        <Settings2 className="h-4 w-4" />
      );
    default:
      return <FileText className="h-4 w-4" />;
  }
}

function activityTitle(type: string) {
  return type.replace(/[._]/g, " ").replace(/\b\w/g, (letter) => letter.toUpperCase());
}

function activityDate(item: ActivityItem) {
  return item.occurred_at || item.created_at || "";
}

function activityHref(item: ActivityItem, repositoryId: string) {
  const pullRequestNumber =
    item.pull_request_number ||
    (typeof item.details?.pull_request_number === "number"
      ? item.details.pull_request_number
      : undefined);
  const documentId =
    item.document_id ||
    (item.subject_type === "document" ? item.subject_id : undefined);
  if (pullRequestNumber) {
    return `/app/repos/${repositoryId}/pulls/${pullRequestNumber}`;
  }
  if (documentId) {
    return `/app/d/${documentId}?repo=${encodeURIComponent(repositoryId)}`;
  }
  return undefined;
}

function activityType(item: ActivityItem) {
  return item.type || item.action || item.subject_type || "repository.activity";
}

function formatDate(value?: string) {
  if (!value) return "Unknown time";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}
