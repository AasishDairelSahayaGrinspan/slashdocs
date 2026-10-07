import { useNavigate } from "react-router-dom";
import {
  Check,
  ChevronsUpDown,
  LogOut,
  Monitor,
  Moon,
  Rocket,
  Sun,
  Terminal,
  User as UserIcon,
} from "lucide-react";
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

// The account menu. In the rail it is the profile card at the foot of the
// rail; elsewhere it is an avatar button.
export function AccountMenu({
  user,
  card = false,
}: {
  user: Principal;
  card?: boolean;
}) {
  const nav = useNavigate();
  const signOut = useSignOut();
  const { theme, setTheme } = useTheme();
  const { reset } = useOnboarding();
  return (
    <Dropdown>
      <DropdownTrigger asChild>
        {card ? (
          <button
            aria-label="Account menu"
            className="flex w-full items-center gap-3 rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] p-2 text-left shadow-[var(--shadow-card)] transition-colors hover:border-[var(--color-border-strong)] focus-visible:outline-2 focus-visible:outline-[var(--color-accent)] dark:border-transparent"
          >
            <Avatar src={user.picture_url} name={user.name || user.email || user.id} />
            <span className="min-w-0 flex-1">
              <span className="block truncate text-sm font-medium">
                {user.name || "Signed in"}
              </span>
              <span className="block truncate text-xs text-[var(--color-fg-muted)]">
                {user.email || user.id}
              </span>
            </span>
            <ChevronsUpDown className="h-4 w-4 shrink-0 text-[var(--color-fg-muted)]" />
          </button>
        ) : (
          <Button
            variant="ghost"
            size="icon"
            className="min-h-10 min-w-10 rounded-full transition-transform duration-150 ease-out active:scale-[0.96] outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-accent)]"
            aria-label="Account menu"
          >
            <Avatar src={user.picture_url} name={user.name || user.email || user.id} />
          </Button>
        )}
      </DropdownTrigger>
      <DropdownContent
        side={card ? "top" : "bottom"}
        align={card ? "start" : "end"}
        className={card ? "w-[var(--radix-dropdown-menu-trigger-width)]" : undefined}
      >
        {/* The rail card already shows who is signed in. */}
        {!card && (
          <>
            <div className="px-2 py-2">
              <div className="text-sm font-medium">{user.name || "Signed in"}</div>
              <div className="text-xs text-[var(--color-fg-muted)]">
                {user.email || user.id}
              </div>
            </div>
            <DropdownSeparator />
          </>
        )}
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
