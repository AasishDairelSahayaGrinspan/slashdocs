import { Outlet, useLocation, useNavigate } from "react-router-dom";
import { useEffect } from "react";
import { AccountMenu, GetStarted, TopBar } from "./topbar";
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
  return (
    <main
      className={cn(
        "relative h-full",
        fullscreenDocument ? "overflow-hidden" : "overflow-auto",
      )}
    >
      {!documentPage && (
        <TopBar>
          <GetStarted />
          <AccountMenu user={user} />
        </TopBar>
      )}
      <Outlet />
    </main>
  );
}
