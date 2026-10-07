import { useState, type ComponentType, type ReactNode } from "react";
import { Link, useLocation } from "react-router-dom";
import { Bot, FileText, Menu, Rocket, Terminal, User as UserIcon } from "lucide-react";
import { AccountMenu } from "./account-menu";
import { Logo } from "./logo";
import { Sheet, SheetContent, SheetTitle } from "@/components/ui/sheet";
import { Button } from "@/components/ui/button";
import { onboardingActive, onboardingProgress, useOnboarding } from "@/lib/onboarding";
import type { Principal } from "@/api";

// The navigation rail, as in Slash: the brand, the destinations, a labeled
// settings group, and the profile card at the foot.
function RailNav({ user, onNavigate }: { user: Principal; onNavigate?: () => void }) {
  const { state } = useOnboarding();
  const { done, total } = onboardingProgress(state);
  const link = { onNavigate };
  return (
    <nav aria-label="Main" className="flex h-full flex-col px-3">
      <Link
        to="/app/documents"
        onClick={onNavigate}
        aria-label="slashdocs home"
        className="flex h-16 shrink-0 items-center px-3"
      >
        <Logo className="h-[18px] w-auto" />
      </Link>
      <div className="flex-1 space-y-1 overflow-y-auto py-2">
        <RailLink
          {...link}
          to="/app/documents"
          also="/app/folders"
          icon={FileText}
          label="Documents"
        />
        {onboardingActive(state) && (
          <RailLink {...link} to="/app/start" icon={Rocket} label="Get started">
            <span className="ml-auto rounded-full bg-[var(--color-accent-muted)] px-1.5 font-mono text-[11px] text-[var(--color-accent)]">
              {done}/{total}
            </span>
          </RailLink>
        )}
        <p className="flex items-center gap-3 px-3 pb-1 pt-6 text-xs text-[var(--color-fg-muted)] after:h-px after:flex-1 after:bg-gradient-to-r after:from-[var(--razorsense-wash-from)] after:to-transparent after:opacity-60">
          Settings
        </p>
        <RailLink {...link} to="/app/developers" icon={Terminal} label="Developers" />
        <RailLink
          {...link}
          to="/app/settings/service-accounts"
          icon={Bot}
          label="Bots"
        />
        <RailLink
          {...link}
          to="/app/settings/profile"
          icon={UserIcon}
          label="Profile"
        />
      </div>
      <div className="shrink-0 py-3">
        <AccountMenu user={user} card />
      </div>
    </nav>
  );
}

function RailLink({
  to,
  also,
  icon: Icon,
  label,
  onNavigate,
  children,
}: {
  to: string;
  also?: string;
  icon: ComponentType<{ className?: string }>;
  label: string;
  onNavigate?: () => void;
  children?: ReactNode;
}) {
  const { pathname } = useLocation();
  const current = [to, also].some((p) => p && pathname.startsWith(p));
  return (
    <Link
      to={to}
      onClick={onNavigate}
      aria-current={current ? "page" : undefined}
      className="rail-link flex h-10 items-center gap-3 rounded-sm px-3 font-heading text-sm text-[var(--color-fg-muted)] outline-none aria-[current=page]:font-medium"
    >
      <Icon className="h-4 w-4 shrink-0" />
      {label}
      {children}
    </Link>
  );
}

// The rail on wide screens. Narrow screens get a bar with a menu button that
// opens the same rail in a sheet.
export function Rail({ user }: { user: Principal }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <aside className="hidden w-60 shrink-0 border-r border-[var(--color-border)] bg-[var(--color-rail)] md:block">
        <RailNav user={user} />
      </aside>
      <header className="flex h-14 shrink-0 items-center justify-between border-b border-[var(--color-border)] bg-[var(--color-rail)] px-2 md:hidden">
        <Button
          variant="ghost"
          size="icon"
          aria-label="Open navigation"
          onClick={() => setOpen(true)}
        >
          <Menu className="h-5 w-5" />
        </Button>
        <Link to="/app/documents" aria-label="slashdocs home">
          <Logo className="h-4 w-auto" />
        </Link>
        <span className="w-9" />
      </header>
      <Sheet open={open} onOpenChange={setOpen}>
        <SheetContent side="left" className="max-w-72 bg-[var(--color-rail)]">
          <SheetTitle className="sr-only">Navigation</SheetTitle>
          <RailNav user={user} onNavigate={() => setOpen(false)} />
        </SheetContent>
      </Sheet>
    </>
  );
}
