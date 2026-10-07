import { Outlet, useLocation, useNavigate } from "react-router-dom";
import { useEffect } from "react";
import { Rail } from "./rail";
import { useMe } from "@/lib/hooks";
import { Center } from "@/components/ui/misc";
import { cn } from "@/lib/cn";

export function AppShell() {
  const me = useMe();
  const nav = useNavigate();
  const loc = useLocation();
  useEffect(() => {
    if (me.unauth) {
      const next = encodeURIComponent(loc.pathname + loc.search);
      nav(`/login?next=${next}`, { replace: true });
    }
  }, [me.unauth, loc.pathname, loc.search, nav]);
  if (me.isLoading) return <Center>Loading workspace…</Center>;
  if (!me.data) return null;
  const user = me.data.user || me.data.principal;
  const documentPage = loc.pathname.startsWith("/app/d/");
  const fullscreenDocument =
    documentPage && new URLSearchParams(loc.search).get("fullscreen") === "1";
  // A document is a focused view with its own bar, so the rail steps aside.
  return (
    <div className="flex h-full flex-col md:flex-row">
      {!documentPage && <Rail user={user} />}
      <main
        className={cn(
          "min-w-0 flex-1",
          fullscreenDocument ? "overflow-hidden" : "overflow-auto",
        )}
      >
        <Outlet />
      </main>
    </div>
  );
}
