import { Link, useLocation, useNavigate } from "react-router-dom";
import {
  Bell,
  BookCopy,
  Check,
  Files,
  GitPullRequest,
  LayoutTemplate,
  LogOut,
  Rocket,
  Settings2,
  User as UserIcon,
  Users,
} from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { GetStartedPill } from "./get-started-pill";
import { useOnboarding } from "@/lib/onboarding";
import {
  Dropdown,
  DropdownContent,
  DropdownItem,
  DropdownLabel,
  DropdownSeparator,
  DropdownTrigger,
} from "@/components/ui/dropdown";
import { Avatar } from "@/components/ui/misc";
import { setFrontendSignedOut } from "@/lib/auth-state";
import { api, type Principal } from "@/api";
import { queryKeys } from "@/lib/queryKeys";

export function TopBar({
  user,
  development = false,
}: {
  user: Principal;
  development?: boolean;
}) {
  const nav = useNavigate();
  const location = useLocation();
  const { reset } = useOnboarding();
  const notifications = useQuery({
    queryKey: queryKeys.notifications(),
    queryFn: api.listNotifications,
    refetchInterval: 60_000,
    meta: { suppressGlobalError: true },
  });
  const unread = (notifications.data?.items || []).filter((item) => !item.read).length;
  return (
    <header className="sticky top-0 z-30 flex h-14 items-center justify-between border-b border-[var(--color-border)]/50 bg-[var(--color-surface)]/70 px-5 backdrop-blur-xl backdrop-saturate-150">
      <div className="flex min-w-0 items-center gap-2 sm:gap-5">
        <Link
          to="/app/documents"
          className="group flex shrink-0 items-center gap-2 font-semibold"
        >
          <img
            src="/favicon.svg"
            alt=""
            className="h-10 w-10 transition-transform duration-300 ease-out group-hover:rotate-[-8deg] group-hover:scale-110"
          />
          <span className="hidden sm:inline">aidocs</span>
        </Link>
        <nav className="flex items-center gap-1" aria-label="Workspace">
          <WorkspaceLink
            to="/app/documents"
            active={
              location.pathname.startsWith("/app/documents") ||
              location.pathname.startsWith("/app/folders/") ||
              (location.pathname.startsWith("/app/d/") &&
                !new URLSearchParams(location.search).has("repo"))
            }
            label="Documents"
            icon={<Files className="h-4 w-4" />}
          />
          <WorkspaceLink
            to="/app/repositories"
            active={
              location.pathname.startsWith("/app/repositories") ||
              location.pathname.startsWith("/app/repos/") ||
              (location.pathname.startsWith("/app/d/") &&
                new URLSearchParams(location.search).has("repo"))
            }
            label="Repositories"
            icon={<BookCopy className="h-4 w-4" />}
          />
          <WorkspaceLink
            to="/app/review-inbox"
            active={location.pathname.startsWith("/app/review-inbox")}
            label="Review inbox"
            icon={<GitPullRequest className="h-4 w-4" />}
          />
          <WorkspaceLink
            to="/app/templates"
            active={location.pathname.startsWith("/app/templates")}
            label="Templates"
            icon={<LayoutTemplate className="h-4 w-4" />}
          />
        </nav>
      </div>
      <div className="flex items-center gap-2">
        <GetStartedPill />
        <Link
          to="/app/review-inbox?tab=notifications"
          className="relative flex h-10 w-10 items-center justify-center rounded-full text-[var(--color-fg-muted)] transition-colors hover:bg-[var(--color-surface-muted)] hover:text-[var(--color-fg)]"
          aria-label={unread ? `${unread} unread notifications` : "Notifications"}
        >
          <Bell className="h-4 w-4" />
          {unread ? (
            <span className="absolute right-0.5 top-0.5 min-w-4 rounded-full bg-[var(--color-accent)] px-1 text-center text-[9px] font-bold leading-4 text-[var(--color-accent-fg)]">
              {unread > 9 ? "9+" : unread}
            </span>
          ) : null}
        </Link>
        <Dropdown>
          <DropdownTrigger asChild>
            <button
              className="min-h-10 min-w-10 rounded-full transition-transform duration-150 ease-out active:scale-[0.96] outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-accent)]"
              aria-label="Account menu"
            >
              <Avatar
                src={user.picture_url}
                name={user.name || user.email || user.id}
              />
            </button>
          </DropdownTrigger>
          <DropdownContent>
            <div className="px-2 py-2">
              <div className="text-sm font-medium">{user.name || "Signed in"}</div>
              <div className="text-xs text-[var(--color-fg-muted)]">
                {user.email || user.id}
              </div>
            </div>
            <DropdownSeparator />
            <DropdownItem
              onSelect={() => {
                reset();
                nav("/app/start");
              }}
            >
              <Rocket className="h-4 w-4" /> Setup guide
            </DropdownItem>
            <DropdownItem onSelect={() => nav("/app/profile")}>
              <UserIcon className="h-4 w-4" /> Profile
            </DropdownItem>
            <DropdownItem onSelect={() => nav("/app/settings/appearance")}>
              <Settings2 className="h-4 w-4" /> Settings
            </DropdownItem>
            <DropdownSeparator />
            {development && (
              <>
                <DropdownLabel>Local identity</DropdownLabel>
                <DevelopmentIdentityItem
                  identity="owner"
                  label="Dev Owner"
                  selected={user.id === "dev_owner"}
                  returnTo={location.pathname + location.search}
                />
                <DevelopmentIdentityItem
                  identity="reviewer"
                  label="Dev Reviewer"
                  selected={user.id === "dev_reviewer"}
                  returnTo={location.pathname + location.search}
                />
                <DropdownSeparator />
              </>
            )}
            {!development && (
              <>
                <DropdownSeparator />
                <DropdownItem
                  onSelect={() => {
                    setFrontendSignedOut(true);
                    nav("/");
                  }}
                >
                  <LogOut className="h-4 w-4" /> Sign out
                </DropdownItem>
              </>
            )}
          </DropdownContent>
        </Dropdown>
      </div>
    </header>
  );
}

function DevelopmentIdentityItem({
  identity,
  label,
  selected,
  returnTo,
}: {
  identity: "owner" | "reviewer";
  label: string;
  selected: boolean;
  returnTo: string;
}) {
  return (
    <DropdownItem
      onSelect={() => {
        setFrontendSignedOut(false);
        const query = new URLSearchParams({ as: identity, redirect: returnTo });
        window.location.assign(`/__dev/login?${query.toString()}`);
      }}
    >
      <Users className="h-4 w-4" /> {label}
      {selected && <Check className="ml-auto h-4 w-4" />}
    </DropdownItem>
  );
}

function WorkspaceLink({
  to,
  active,
  label,
  icon,
}: {
  to: string;
  active: boolean;
  label: string;
  icon: React.ReactNode;
}) {
  return (
    <Link
      to={to}
      aria-current={active ? "page" : undefined}
      className={`flex min-h-9 items-center gap-1.5 rounded-lg px-2.5 text-sm font-medium transition-colors sm:px-3 ${
        active
          ? "bg-[var(--color-surface-muted)] text-[var(--color-fg)]"
          : "text-[var(--color-fg-muted)] hover:bg-[var(--color-surface-muted)]/65 hover:text-[var(--color-fg)]"
      }`}
    >
      {icon}
      <span className="hidden md:inline">{label}</span>
    </Link>
  );
}
