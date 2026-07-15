import { NavLink, Outlet } from "react-router-dom";
import { Bot, Code2, Link2, Palette, UserRound } from "lucide-react";
import { cn } from "@/lib/cn";

const sections = [
  {
    label: "Personal",
    items: [
      { to: "/app/settings/account", label: "Account", icon: UserRound },
      { to: "/app/settings/connections", label: "Connections", icon: Link2 },
      { to: "/app/settings/appearance", label: "Appearance", icon: Palette },
    ],
  },
  {
    label: "Workspace",
    items: [
      { to: "/app/settings/service-accounts", label: "Agent access", icon: Bot },
      { to: "/app/settings/developer", label: "Developer", icon: Code2 },
    ],
  },
];

export function SettingsLayout() {
  return (
    <div className="mx-auto w-full max-w-7xl px-4 py-6 sm:px-6 sm:py-10">
      <h1 className="mb-6 text-2xl font-semibold tracking-tight">Settings</h1>
      <div className="grid gap-7 lg:grid-cols-[220px_minmax(0,1fr)]">
        <aside className="min-w-0">
          <nav
            aria-label="Settings"
            className="flex gap-2 overflow-x-auto pb-2 lg:block lg:space-y-5 lg:overflow-visible lg:pb-0"
          >
            {sections.map((section) => (
              <div key={section.label} className="shrink-0 lg:shrink">
                <div className="mb-1 hidden px-3 text-xs font-semibold uppercase tracking-wider text-[var(--color-fg-muted)] lg:block">
                  {section.label}
                </div>
                <div className="flex gap-1 lg:block lg:space-y-1">
                  {section.items.map(({ to, label, icon: Icon }) => (
                    <NavLink
                      key={to}
                      to={to}
                      className={({ isActive }) =>
                        cn(
                          "flex min-h-10 items-center gap-2 whitespace-nowrap rounded-lg px-3 text-sm font-medium transition-colors",
                          isActive
                            ? "bg-[var(--color-accent-muted)] text-[var(--color-fg)]"
                            : "text-[var(--color-fg-muted)] hover:bg-[var(--color-surface-muted)] hover:text-[var(--color-fg)]",
                        )
                      }
                    >
                      <Icon className="h-4 w-4" /> {label}
                    </NavLink>
                  ))}
                </div>
              </div>
            ))}
          </nav>
        </aside>
        <div className="min-w-0 overflow-hidden rounded-2xl border border-[var(--color-border)] bg-[var(--color-surface)]">
          <Outlet />
        </div>
      </div>
    </div>
  );
}
