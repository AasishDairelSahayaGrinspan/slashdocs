import { useMemo } from "react";
import { Link, useParams, useSearchParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import {
  BookOpen,
  CheckCircle2,
  FileText,
  GitMerge,
  GitPullRequest,
  MessageSquareText,
} from "lucide-react";
import {
  api,
  APIError,
  repositoryNamespaceName,
  type UserContribution,
  type UserProfile,
} from "@/api";
import { Badge } from "@/components/ui/badge";
import { Avatar, EmptyState, Skeleton } from "@/components/ui/misc";
import { queryKeys } from "@/lib/queryKeys";
import { useMe } from "@/lib/hooks";
import { useTitle } from "@/lib/use-title";

type ProfileTab = "overview" | "documents" | "pulls" | "contributions";

export function ProfilePage() {
  const { id: routeID } = useParams();
  const [searchParams, setSearchParams] = useSearchParams();
  const me = useMe();
  const userID = routeID || me.data?.user?.id || me.data?.principal.id || "";
  const profile = useQuery({
    queryKey: queryKeys.userProfile(userID),
    queryFn: () => api.getUserProfile(userID),
    enabled: !!userID,
    meta: { suppressGlobalError: true },
  });
  const rawTab = searchParams.get("tab");
  const tab: ProfileTab =
    rawTab === "documents" || rawTab === "pulls" || rawTab === "contributions"
      ? rawTab
      : "overview";
  const user = profile.data?.user;
  const isMe = !!userID && userID === me.data?.principal.id;
  useTitle(user ? `${user.name || user.email || "User"}` : "Profile");

  if (me.isLoading || (!userID && !me.error) || profile.isLoading) {
    return <ProfileSkeleton />;
  }
  if (
    profile.error instanceof APIError &&
    (profile.error.status === 404 || profile.error.status === 403)
  ) {
    return (
      <div className="mx-auto max-w-5xl px-4 py-10 sm:px-6">
        <EmptyState
          title="Profile not available"
          description="This person could not be found, or their activity is not visible to you."
        />
      </div>
    );
  }
  if (profile.error || !profile.data || !user) {
    return (
      <div className="mx-auto max-w-5xl px-4 py-10 sm:px-6">
        <EmptyState
          title="Could not load this profile"
          description="Refresh the page to try again."
        />
      </div>
    );
  }

  const data = profile.data;
  const setTab = (next: ProfileTab) => {
    setSearchParams(next === "overview" ? {} : { tab: next }, { replace: true });
  };

  return (
    <div className="mx-auto max-w-6xl px-4 py-6 sm:px-6 sm:py-10">
      <div className="grid gap-8 lg:grid-cols-[250px_minmax(0,1fr)]">
        <aside>
          <Avatar
            size={112}
            src={user.picture_url}
            name={user.name || user.email || user.id}
          />
          <h1 className="mt-4 text-2xl font-semibold tracking-tight">
            {user.name || "aidocs user"}
          </h1>
          <p className="mt-1 break-all text-sm text-[var(--color-fg-muted)]">
            {user.email || user.id}
          </p>
          {isMe ? (
            <Link
              to="/app/settings/account"
              className="mt-4 inline-flex min-h-9 w-full items-center justify-center rounded-md border border-[var(--color-border)] px-3 text-sm font-medium transition-colors hover:bg-[var(--color-surface-muted)]"
            >
              Edit personal settings
            </Link>
          ) : null}
          <div className="mt-6 grid grid-cols-2 gap-2 text-sm">
            <Stat label="Documents" value={data.stats.documents_created} />
            <Stat label="Pull requests" value={data.stats.pull_requests_authored} />
            <Stat label="Contributions" value={data.stats.contributions} />
            <Stat label="Repositories" value={data.stats.repositories_contributed_to} />
          </div>
        </aside>

        <main className="min-w-0">
          <div className="flex gap-1 overflow-x-auto border-b border-[var(--color-border)]">
            <ProfileTabButton
              active={tab === "overview"}
              onClick={() => setTab("overview")}
            >
              Overview
            </ProfileTabButton>
            <ProfileTabButton
              active={tab === "documents"}
              onClick={() => setTab("documents")}
            >
              Documents <Badge variant="muted">{data.documents.length}</Badge>
            </ProfileTabButton>
            <ProfileTabButton active={tab === "pulls"} onClick={() => setTab("pulls")}>
              Pull requests <Badge variant="muted">{data.pull_requests.length}</Badge>
            </ProfileTabButton>
            <ProfileTabButton
              active={tab === "contributions"}
              onClick={() => setTab("contributions")}
            >
              Contributions <Badge variant="muted">{data.contributions.length}</Badge>
            </ProfileTabButton>
          </div>

          <div className="pt-5">
            {tab === "overview" ? <Overview profile={data} /> : null}
            {tab === "documents" ? <DocumentsList profile={data} /> : null}
            {tab === "pulls" ? <PullRequestsList profile={data} /> : null}
            {tab === "contributions" ? (
              <ContributionList items={data.contributions} />
            ) : null}
          </div>
        </main>
      </div>
    </div>
  );
}

function Overview({ profile }: { profile: UserProfile }) {
  const recent = useMemo(
    () => [...profile.contributions].sort(byNewest).slice(0, 8),
    [profile.contributions],
  );
  return (
    <div className="space-y-7">
      <section>
        <div className="mb-3 flex items-center justify-between gap-3">
          <h2 className="font-semibold">Recent contributions</h2>
          {profile.contributions.length > recent.length ? (
            <span className="text-xs text-[var(--color-fg-muted)]">
              Showing {recent.length} of {profile.contributions.length}
            </span>
          ) : null}
        </div>
        <ContributionList items={recent} compact />
      </section>
      <section>
        <h2 className="mb-3 font-semibold">Recently created documents</h2>
        <DocumentsList profile={profile} limit={4} />
      </section>
    </div>
  );
}

function DocumentsList({ profile, limit }: { profile: UserProfile; limit?: number }) {
  const items = limit ? profile.documents.slice(0, limit) : profile.documents;
  if (items.length === 0) {
    return (
      <EmptyState
        className="p-8"
        icon={<FileText className="h-5 w-5" />}
        title="No visible documents yet"
        description="Documents created by this person that you can access will appear here."
      />
    );
  }
  return (
    <div className="overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)]">
      {items.map(({ document, repository }, index) => (
        <Link
          key={document.id}
          to={`/app/d/${document.id}${repository ? `?repo=${encodeURIComponent(repository.id)}` : ""}`}
          className={`flex items-start gap-3 px-4 py-3 transition-colors hover:bg-[var(--color-surface-muted)]/65 ${index ? "border-t border-[var(--color-border)]" : ""}`}
        >
          <FileText className="mt-0.5 h-4 w-4 shrink-0 text-[var(--color-accent)]" />
          <div className="min-w-0 flex-1">
            <div className="truncate text-sm font-medium">{document.title}</div>
            <div className="mt-1 truncate text-xs text-[var(--color-fg-muted)]">
              {repository
                ? `${repositoryNamespaceName(repository)} / ${repository.name}`
                : "Personal document"}
            </div>
          </div>
          {document.updated_at ? (
            <span className="shrink-0 text-xs text-[var(--color-fg-muted)]">
              {relativeDate(document.updated_at)}
            </span>
          ) : null}
        </Link>
      ))}
    </div>
  );
}

function PullRequestsList({ profile }: { profile: UserProfile }) {
  if (profile.pull_requests.length === 0) {
    return (
      <EmptyState
        className="p-8"
        icon={<GitPullRequest className="h-5 w-5" />}
        title="No visible pull requests yet"
        description="Pull requests raised by this person in repositories you can access will appear here."
      />
    );
  }
  return (
    <div className="overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)]">
      {profile.pull_requests.map(({ pull_request: pull, repository }, index) => (
        <Link
          key={pull.id}
          to={`/app/repos/${repository.id}/pulls/${pull.number}`}
          className={`flex items-start gap-3 px-4 py-3 transition-colors hover:bg-[var(--color-surface-muted)]/65 ${index ? "border-t border-[var(--color-border)]" : ""}`}
        >
          {pull.status === "merged" ? (
            <GitMerge className="mt-0.5 h-4 w-4 shrink-0 text-violet-500" />
          ) : (
            <GitPullRequest className="mt-0.5 h-4 w-4 shrink-0 text-[var(--color-success)]" />
          )}
          <div className="min-w-0 flex-1">
            <div className="truncate text-sm font-medium">{pull.title}</div>
            <div className="mt-1 truncate text-xs text-[var(--color-fg-muted)]">
              {repositoryNamespaceName(repository)} / {repository.name} #{pull.number}
            </div>
          </div>
          <Badge variant="muted" className="capitalize">
            {pull.status}
          </Badge>
        </Link>
      ))}
    </div>
  );
}

function ContributionList({
  items,
  compact = false,
}: {
  items: UserContribution[];
  compact?: boolean;
}) {
  if (items.length === 0) {
    return (
      <EmptyState
        className="p-8"
        icon={<CheckCircle2 className="h-5 w-5" />}
        title="No visible contributions yet"
        description="Reviews, proposed changes, and other document activity will appear here."
      />
    );
  }
  return (
    <div className="overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)]">
      {items.map((item, index) => {
        const href = contributionHref(item);
        const content = (
          <>
            <ContributionIcon type={item.type} />
            <div className="min-w-0 flex-1">
              <div className="truncate text-sm font-medium">{item.title}</div>
              <div className="mt-1 truncate text-xs text-[var(--color-fg-muted)]">
                {contributionDescription(item)}
              </div>
            </div>
            {item.occurred_at ? (
              <span className="shrink-0 text-xs text-[var(--color-fg-muted)]">
                {relativeDate(item.occurred_at)}
              </span>
            ) : null}
          </>
        );
        const className = `flex items-start gap-3 px-4 ${compact ? "py-3" : "py-4"} ${index ? "border-t border-[var(--color-border)]" : ""}`;
        return href ? (
          <Link
            key={`${item.type}-${item.id}`}
            to={href}
            className={`${className} transition-colors hover:bg-[var(--color-surface-muted)]/65`}
          >
            {content}
          </Link>
        ) : (
          <div key={`${item.type}-${item.id}`} className={className}>
            {content}
          </div>
        );
      })}
    </div>
  );
}

function ContributionIcon({ type }: { type: string }) {
  if (type.includes("review")) {
    return <MessageSquareText className="mt-0.5 h-4 w-4 shrink-0 text-blue-500" />;
  }
  if (type.includes("merge")) {
    return <GitMerge className="mt-0.5 h-4 w-4 shrink-0 text-violet-500" />;
  }
  if (type.includes("pull") || type.includes("proposal")) {
    return (
      <GitPullRequest className="mt-0.5 h-4 w-4 shrink-0 text-[var(--color-success)]" />
    );
  }
  return <BookOpen className="mt-0.5 h-4 w-4 shrink-0 text-[var(--color-accent)]" />;
}

function Stat({ label, value }: { label: string; value: number }) {
  return (
    <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2.5">
      <div className="text-lg font-semibold">{value}</div>
      <div className="text-xs text-[var(--color-fg-muted)]">{label}</div>
    </div>
  );
}

function ProfileTabButton({
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
      className={`relative flex min-h-11 shrink-0 items-center gap-2 px-3 text-sm font-medium transition-colors ${
        active
          ? "text-[var(--color-fg)] after:absolute after:inset-x-2 after:-bottom-px after:h-0.5 after:rounded-full after:bg-[var(--color-accent)]"
          : "text-[var(--color-fg-muted)] hover:text-[var(--color-fg)]"
      }`}
    >
      {children}
    </button>
  );
}

function ProfileSkeleton() {
  return (
    <div className="mx-auto max-w-6xl px-4 py-10 sm:px-6">
      <div className="grid gap-8 lg:grid-cols-[250px_minmax(0,1fr)]">
        <div>
          <Skeleton className="h-28 w-28 rounded-full" />
          <Skeleton className="mt-4 h-8 w-44" />
          <Skeleton className="mt-2 h-4 w-52" />
        </div>
        <div>
          <Skeleton className="h-11 w-full" />
          <Skeleton className="mt-5 h-72 w-full" />
        </div>
      </div>
    </div>
  );
}

function contributionHref(item: UserContribution) {
  if (item.repository_id && item.pull_request_number) {
    return `/app/repos/${item.repository_id}/pulls/${item.pull_request_number}`;
  }
  if (item.document_id) {
    return `/app/d/${item.document_id}${item.repository_id ? `?repo=${encodeURIComponent(item.repository_id)}` : ""}`;
  }
  if (item.repository_id) return `/app/repos/${item.repository_id}`;
  return "";
}

function contributionDescription(item: UserContribution) {
  const action = item.type.replaceAll("_", " ");
  const place = item.repository_name || item.document_title;
  return [action, place, item.state].filter(Boolean).join(" · ");
}

function byNewest(a: UserContribution, b: UserContribution) {
  return (b.occurred_at || "").localeCompare(a.occurred_at || "");
}

function relativeDate(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  const seconds = Math.round((date.getTime() - Date.now()) / 1000);
  const abs = Math.abs(seconds);
  const formatter = new Intl.RelativeTimeFormat(undefined, { numeric: "auto" });
  if (abs < 60) return formatter.format(seconds, "second");
  if (abs < 3600) return formatter.format(Math.round(seconds / 60), "minute");
  if (abs < 86400) return formatter.format(Math.round(seconds / 3600), "hour");
  if (abs < 2_592_000) return formatter.format(Math.round(seconds / 86400), "day");
  return date.toLocaleDateString();
}
