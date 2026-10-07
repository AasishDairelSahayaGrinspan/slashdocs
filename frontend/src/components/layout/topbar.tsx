import { Link, useNavigate } from "react-router-dom";
import {
  Check,
  LogOut,
  Monitor,
  Moon,
  Rocket,
  Sun,
  Terminal,
  User as UserIcon,
} from "lucide-react";
import { GetStartedPill } from "./get-started-pill";
import { Logo } from "./logo";
import { useOnboarding } from "@/lib/onboarding";
import {
  Dropdown,
  DropdownContent,
  DropdownItem,
  DropdownLabel,
  DropdownSeparator,
  DropdownTrigger,
} from "@/components/ui/dropdown";
import { Button } from "@/components/ui/button";
import { Avatar } from "@/components/ui/misc";
import { useSignOut } from "@/lib/auth-state";
import { useTheme } from "@/lib/theme";
import type { Principal } from "@/api";

export function TopBar({ user }: { user: Principal }) {
  return (
    <header className="sticky top-0 z-30 flex h-14 items-center justify-between border-b border-[var(--color-border)]/50 bg-[var(--color-surface)]/70 px-5 backdrop-blur-xl backdrop-saturate-150">
      <Link to="/app/documents">
        <Logo className="h-5 w-auto" />
      </Link>
      <div className="flex items-center gap-2">
        <GetStartedPill />
        <AccountMenu user={user} />
      </div>
    </header>
  );
}

export function AccountMenu({ user }: { user: Principal }) {
  const nav = useNavigate();
  const signOut = useSignOut();
  const { theme, setTheme } = useTheme();
  const { reset } = useOnboarding();
  return (
    <Dropdown>
      <DropdownTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          className="min-h-10 min-w-10 rounded-full transition-transform duration-150 ease-out active:scale-[0.96] outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-accent)]"
          aria-label="Account menu"
        >
          <Avatar src={user.picture_url} name={user.name || user.email || user.id} />
        </Button>
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
        <DropdownItem onSelect={() => nav("/app/settings/profile")}>
          <UserIcon className="h-4 w-4" /> Profile
        </DropdownItem>
        <DropdownItem onSelect={() => nav("/app/developers")}>
          <Terminal className="h-4 w-4" /> Developers
        </DropdownItem>
        <DropdownSeparator />
        <DropdownLabel>Theme</DropdownLabel>
        {(
          [
            { value: "light", label: "Light", icon: Sun },
            { value: "dark", label: "Dark", icon: Moon },
            { value: "system", label: "System", icon: Monitor },
          ] as const
        ).map(({ value, label, icon: Icon }) => (
          <DropdownItem
            key={value}
            onSelect={(e) => {
              e.preventDefault();
              setTheme(value);
            }}
          >
            <Icon className="h-4 w-4" /> {label}
            {theme === value && <Check className="ml-auto h-4 w-4" />}
          </DropdownItem>
        ))}
        <DropdownSeparator />
        <DropdownItem disabled={signOut.isPending} onSelect={() => signOut.mutate()}>
          <LogOut className="h-4 w-4" /> Sign out
        </DropdownItem>
      </DropdownContent>
    </Dropdown>
  );
}
