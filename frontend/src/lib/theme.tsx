import * as React from "react";
import { createPersistentStore } from "./persistent-store";

type Theme = "light" | "dark" | "system";

const store = createPersistentStore<Theme>("aidocs.theme", "system", (parsed) =>
  parsed === "light" || parsed === "dark" ? parsed : "system",
);

function apply(t: Theme) {
  const isDark =
    t === "dark" ||
    (t === "system" && window.matchMedia("(prefers-color-scheme: dark)").matches);
  document.documentElement.classList.toggle("dark", isDark);
}

export function ThemeProvider({ children }: { children: React.ReactNode }) {
  const theme = store.useStore();
  React.useEffect(() => {
    apply(theme);
    const mq = window.matchMedia("(prefers-color-scheme: dark)");
    const onChange = () => theme === "system" && apply("system");
    mq.addEventListener("change", onChange);
    return () => mq.removeEventListener("change", onChange);
  }, [theme]);
  return <>{children}</>;
}

export function useTheme() {
  const theme = store.useStore();
  return { theme, setTheme: store.write };
}

function resolve(t: Theme): "light" | "dark" {
  if (t === "light" || t === "dark") return t;
  return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

export function useResolvedTheme(): "light" | "dark" {
  const theme = store.useStore();
  const [resolved, setResolved] = React.useState(() => resolve(theme));
  React.useEffect(() => {
    setResolved(resolve(theme));
    if (theme !== "system") return;
    const mq = window.matchMedia("(prefers-color-scheme: dark)");
    const onChange = () => setResolved(resolve("system"));
    mq.addEventListener("change", onChange);
    return () => mq.removeEventListener("change", onChange);
  }, [theme]);
  return resolved;
}
