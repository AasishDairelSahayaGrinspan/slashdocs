import { Monitor, Moon, Palette, Sun } from "lucide-react";
import { cn } from "@/lib/cn";
import { useTheme } from "@/lib/theme";
import { useTitle } from "@/lib/use-title";

export function AppearanceSettingsPage() {
  useTitle("Appearance settings");
  const { theme, setTheme } = useTheme();
  const options = [
    {
      value: "light" as const,
      label: "Light",
      description: "Always use the light color theme.",
      icon: Sun,
    },
    {
      value: "dark" as const,
      label: "Dark",
      description: "Always use the dark color theme.",
      icon: Moon,
    },
    {
      value: "system" as const,
      label: "System",
      description: "Follow your device appearance automatically.",
      icon: Monitor,
    },
  ];

  return (
    <div className="p-6 sm:p-8">
      <div className="flex items-center gap-3">
        <Palette className="h-5 w-5 text-[var(--color-accent)]" />
        <div>
          <h2 className="text-lg font-semibold">Appearance</h2>
          <p className="text-sm text-[var(--color-fg-muted)]">
            Choose how aidocs looks to you. This only affects your browser.
          </p>
        </div>
      </div>

      <fieldset className="mt-6">
        <legend className="mb-3 text-sm font-semibold">Color theme</legend>
        <div className="grid gap-3 md:grid-cols-3">
          {options.map(({ value, label, description, icon: Icon }) => (
            <button
              key={value}
              type="button"
              aria-pressed={theme === value}
              onClick={() => setTheme(value)}
              className={cn(
                "flex min-h-36 flex-col items-start rounded-xl border p-4 text-left transition-[border-color,background-color,transform] active:scale-[0.99]",
                theme === value
                  ? "border-[var(--color-accent)] bg-[var(--color-accent-muted)]"
                  : "border-[var(--color-border)] hover:border-[var(--color-border-strong)] hover:bg-[var(--color-surface-muted)]/55",
              )}
            >
              <span className="flex h-9 w-9 items-center justify-center rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)]">
                <Icon className="h-4 w-4" />
              </span>
              <span className="mt-3 text-sm font-semibold">{label}</span>
              <span className="mt-1 text-xs leading-5 text-[var(--color-fg-muted)]">
                {description}
              </span>
            </button>
          ))}
        </div>
      </fieldset>
    </div>
  );
}
