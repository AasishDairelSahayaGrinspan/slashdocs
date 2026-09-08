import { Outlet, useLocation, useNavigate } from "react-router-dom";
import { useEffect } from "react";
import { TopBar } from "./topbar";
import { useMe } from "@/lib/hooks";
import { Center } from "@/components/ui/misc";
import { useFrontendSignedOut } from "@/lib/auth-state";

export function AppShell() {
  const me = useMe();
  const nav = useNavigate();
  const loc = useLocation();
  const frontendSignedOut = useFrontendSignedOut();
  useEffect(() => {
    if (frontendSignedOut) {
      nav("/", { replace: true });
      return;
    }
    if (me.unauth) {
      const next = encodeURIComponent(loc.pathname + loc.search);
      nav(`/login?next=${next}`, { replace: true });
    }
  }, [frontendSignedOut, me.unauth, loc.pathname, loc.search, nav]);
  if (frontendSignedOut) return null;
  if (me.isLoading) return <Center>Loading workspace…</Center>;
  if (!me.data) return null;
  const user = me.data.user || me.data.principal;
  const documentPage = loc.pathname.startsWith("/app/d/");
  const fullscreenDocument =
    documentPage && new URLSearchParams(loc.search).get("fullscreen") === "1";
  return (
    <div className="flex h-full flex-col">
      {!documentPage && <TopBar user={user} />}
      <main
        className={
          fullscreenDocument ? "flex-1 overflow-hidden" : "flex-1 overflow-auto"
        }
      >
        <Outlet />
      </main>
    </div>
  );
}
