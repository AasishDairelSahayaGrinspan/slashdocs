import { Link, useNavigate } from "react-router-dom";
import { LogOut, UserRound } from "lucide-react";
import { Avatar, Center, Skeleton } from "@/components/ui/misc";
import { Button } from "@/components/ui/button";
import { useMe } from "@/lib/hooks";
import { useTitle } from "@/lib/use-title";
import { setFrontendSignedOut } from "@/lib/auth-state";

export function AccountSettingsPage() {
  useTitle("Account settings");
  const me = useMe();
  const nav = useNavigate();
  const user = me.data?.user || me.data?.principal;

  if (me.isLoading) {
    return (
      <div className="p-6 sm:p-8">
        <Skeleton className="h-7 w-40" />
        <Skeleton className="mt-5 h-24 w-full" />
      </div>
    );
  }
  if (!user) return <Center>Could not load your account.</Center>;

  return (
    <div className="p-6 sm:p-8">
      <div className="flex items-center gap-3">
        <UserRound className="h-5 w-5 text-[var(--color-accent)]" />
        <div>
          <h2 className="text-lg font-semibold">Account</h2>
          <p className="text-sm text-[var(--color-fg-muted)]">
            Your identity and account session.
          </p>
        </div>
      </div>

      <div className="mt-6 flex flex-col gap-4 rounded-xl border border-[var(--color-border)] bg-[var(--color-surface-muted)]/35 p-5 sm:flex-row sm:items-center">
        <Avatar size={64} src={user.picture_url} name={user.name || user.email} />
        <div className="min-w-0 flex-1">
          <div className="truncate font-medium">{user.name || "Signed-in user"}</div>
          <div className="truncate text-sm text-[var(--color-fg-muted)]">
            {user.email || user.id}
          </div>
        </div>
        <Button asChild variant="outline">
          <Link to="/app/profile">View public profile</Link>
        </Button>
      </div>

      <div className="mt-8 border-t border-[var(--color-border)] pt-6">
        <h3 className="text-sm font-semibold">Session</h3>
        <p className="mt-1 text-sm text-[var(--color-fg-muted)]">
          Sign out of aidocs on this browser.
        </p>
        <Button
          variant="outline"
          className="mt-4"
          onClick={() => {
            setFrontendSignedOut(true);
            nav("/");
          }}
        >
          <LogOut className="h-4 w-4" /> Sign out
        </Button>
      </div>
    </div>
  );
}
