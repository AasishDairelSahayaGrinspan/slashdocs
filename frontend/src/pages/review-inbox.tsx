import { Link, useSearchParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Bell,
  Check,
  CheckCheck,
  ChevronRight,
  Clock3,
  GitPullRequest,
  MessageSquareText,
} from "lucide-react";
import { api, type AidocsNotification, type ReviewInboxItem } from "@/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { EmptyState, Skeleton } from "@/components/ui/misc";
import { queryKeys } from "@/lib/queryKeys";
import { useTitle } from "@/lib/use-title";

export function ReviewInboxPage() {
  useTitle("Review inbox");
  const [params, setParams] = useSearchParams();
  const tab = params.get("tab") === "notifications" ? "notifications" : "reviews";
  const inbox = useQuery({
    queryKey: queryKeys.reviewInbox(),
    queryFn: api.listReviewInbox,
  });
  const notifications = useQuery({
    queryKey: queryKeys.notifications(),
    queryFn: api.listNotifications,
  });
  const unread = (notifications.data?.items || []).filter((item) => !item.read).length;
  return (
    <div className="mx-auto max-w-5xl px-4 py-6 sm:px-6 sm:py-10">
      <div className="mb-7">
        <h1 className="text-2xl font-semibold tracking-tight">Review inbox</h1>
        <p className="mt-1 text-sm text-[var(--color-fg-muted)]">
          Pull requests waiting for you, mentions, source updates, and knowledge alerts.
        </p>
      </div>
      <div className="mb-5 flex gap-1 border-b border-[var(--color-border)]">
        <InboxTab active={tab === "reviews"} onClick={() => setParams({})}>
          <GitPullRequest className="h-4 w-4" /> Reviews
          {(inbox.data?.items || []).length ? (
            <Badge variant="muted">{inbox.data?.items.length}</Badge>
          ) : null}
        </InboxTab>
        <InboxTab
          active={tab === "notifications"}
          onClick={() => setParams({ tab: "notifications" })}
        >
          <Bell className="h-4 w-4" /> Notifications
          {unread ? <Badge variant="accent">{unread}</Badge> : null}
        </InboxTab>
      </div>
      {tab === "reviews" ? (
        <ReviewList query={inbox} />
      ) : (
        <NotificationList query={notifications} />
      )}
    </div>
  );
}

function ReviewList({
  query,
}: {
  query: ReturnType<typeof useQuery<{ items: ReviewInboxItem[] }>>;
}) {
  if (query.isLoading) return <InboxSkeleton />;
  if (query.error)
    return (
      <EmptyState
        title="Could not load your review inbox"
        action={
          <Button variant="outline" onClick={() => query.refetch()}>
            Try again
          </Button>
        }
      />
    );
  const items = query.data?.items || [];
  if (!items.length)
    return (
      <EmptyState
        icon={<CheckCheck className="h-5 w-5" />}
        title="You're all caught up"
        description="Pull requests that need your review will appear here."
      />
    );
  return (
    <div className="overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)]">
      <ul className="divide-y divide-[var(--color-border)]">
        {items.map((item, index) => {
          const pull = item.pull_request;
          const repository = item.repository;
          const repositoryId =
            repository?.id || item.repository_id || pull?.repository_id;
          const href =
            repositoryId && pull
              ? `/app/repos/${repositoryId}/pulls/${pull.number}`
              : undefined;
          const content = (
            <>
              <div className="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-emerald-50 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300">
                <GitPullRequest className="h-4 w-4" />
              </div>
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="truncate font-semibold">
                    {pull?.title || "Review requested"}
                  </span>
                  {pull ? <Badge variant="success">Open</Badge> : null}
                </div>
                <p className="mt-1 text-sm text-[var(--color-fg-muted)]">
                  {repository?.name || "Repository"}
                  {pull ? ` · #${pull.number}` : ""}
                  {item.reason ? ` · ${item.reason}` : ""}
                </p>
                <div className="mt-2 flex items-center gap-3 text-xs text-[var(--color-fg-muted)]">
                  <span className="flex items-center gap-1">
                    <Clock3 className="h-3.5 w-3.5" />{" "}
                    {formatDate(
                      item.requested_at || item.updated_at || pull?.updated_at,
                    )}
                  </span>
                  {pull?.reviews?.length ? (
                    <span className="flex items-center gap-1">
                      <MessageSquareText className="h-3.5 w-3.5" />{" "}
                      {pull.reviews.length} reviews
                    </span>
                  ) : null}
                </div>
              </div>
              {href ? (
                <ChevronRight className="mt-2 h-4 w-4 shrink-0 text-[var(--color-fg-muted)]" />
              ) : null}
            </>
          );
          return (
            <li key={item.id || pull?.id || index}>
              {href ? (
                <Link
                  to={href}
                  className="flex gap-3 px-4 py-4 transition-colors hover:bg-[var(--color-surface-muted)]/50"
                >
                  {content}
                </Link>
              ) : (
                <div className="flex gap-3 px-4 py-4">{content}</div>
              )}
            </li>
          );
        })}
      </ul>
    </div>
  );
}

function NotificationList({
  query,
}: {
  query: ReturnType<typeof useQuery<{ items: AidocsNotification[] }>>;
}) {
  const qc = useQueryClient();
  const mark = useMutation({
    mutationFn: (id: string) => api.markNotificationRead(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.notifications() }),
  });
  if (query.isLoading) return <InboxSkeleton />;
  if (query.error)
    return (
      <EmptyState
        title="Could not load notifications"
        action={
          <Button variant="outline" onClick={() => query.refetch()}>
            Try again
          </Button>
        }
      />
    );
  const items = [...(query.data?.items || [])].sort((a, b) =>
    (b.created_at || "").localeCompare(a.created_at || ""),
  );
  if (!items.length)
    return (
      <EmptyState
        icon={<Bell className="h-5 w-5" />}
        title="No notifications"
        description="Mentions, approvals, sync failures, and stale-doc reminders will appear here."
      />
    );
  return (
    <div className="overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)]">
      <ul className="divide-y divide-[var(--color-border)]">
        {items.map((item) => {
          const href = notificationHref(item);
          const content = (
            <>
              <span
                className={`mt-2 h-2.5 w-2.5 shrink-0 rounded-full ${item.read ? "bg-transparent" : "bg-[var(--color-accent)]"}`}
              />
              <div className="min-w-0 flex-1">
                <div
                  className={
                    item.read
                      ? "font-medium text-[var(--color-fg-muted)]"
                      : "font-semibold"
                  }
                >
                  {item.title}
                </div>
                {item.body ? (
                  <p className="mt-1 text-sm leading-6 text-[var(--color-fg-muted)]">
                    {item.body}
                  </p>
                ) : null}
                <div className="mt-2 text-xs text-[var(--color-fg-muted)]">
                  {item.repository_name || formatNotificationType(item.type)}
                  {item.created_at ? ` · ${formatDate(item.created_at)}` : ""}
                </div>
              </div>
            </>
          );
          return (
            <li
              key={item.id}
              className="flex items-start gap-2 px-4 py-4 transition-colors hover:bg-[var(--color-surface-muted)]/40"
            >
              {href ? (
                <Link
                  to={href}
                  onClick={() => {
                    if (!item.read) mark.mutate(item.id);
                  }}
                  className="flex min-w-0 flex-1 items-start gap-3"
                >
                  {content}
                </Link>
              ) : (
                <div className="flex min-w-0 flex-1 items-start gap-3">{content}</div>
              )}
              {!item.read ? (
                <Button
                  size="icon"
                  variant="ghost"
                  aria-label="Mark notification read"
                  disabled={mark.isPending}
                  onClick={() => mark.mutate(item.id)}
                >
                  <Check className="h-4 w-4" />
                </Button>
              ) : null}
            </li>
          );
        })}
      </ul>
    </div>
  );
}

function InboxTab({
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
      className={`relative flex min-h-11 items-center gap-2 px-4 text-sm font-medium ${active ? "text-[var(--color-fg)] after:absolute after:inset-x-2 after:-bottom-px after:h-0.5 after:rounded-full after:bg-[var(--color-accent)]" : "text-[var(--color-fg-muted)] hover:text-[var(--color-fg)]"}`}
    >
      {children}
    </button>
  );
}

function InboxSkeleton() {
  return (
    <div className="space-y-2">
      <Skeleton className="h-24 w-full" />
      <Skeleton className="h-24 w-full" />
      <Skeleton className="h-24 w-full" />
    </div>
  );
}

function notificationHref(item: AidocsNotification) {
  if (item.url) return item.url;
  if (item.repository_id && item.pull_request_number)
    return `/app/repos/${item.repository_id}/pulls/${item.pull_request_number}`;
  if (item.document_id)
    return `/app/d/${item.document_id}${item.repository_id ? `?repo=${encodeURIComponent(item.repository_id)}` : ""}`;
  if (item.repository_id) return `/app/repos/${item.repository_id}`;
  return undefined;
}

function formatNotificationType(type?: string) {
  return type ? type.replaceAll("_", " ") : "aidocs";
}
function formatDate(value?: string) {
  if (!value) return "";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}
