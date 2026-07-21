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
  const documentParams = loc.pathname.startsWith("/app/d/")
    ? new URLSearchParams(loc.search)
    : null;
  // Full screen and the HTML editor both take over the viewport. The editor
  // additionally has unsaved work to protect: every control in the TopBar
  // (the logo link, the account menu's navigations) leaves the editor's tree,
  // so none of them can prompt about the draft, and beforeunload does not
  // fire on SPA navigation. Hiding the bar keeps the largest unguarded exit
  // off the screen for the same reason the doc bar's own links are
  // suppressed while editing.
  const fullBleedDocument =
    documentParams?.get("fullscreen") === "1" || documentParams?.get("edit") === "1";
  return (
    <div className="flex h-full flex-col">
      {!fullBleedDocument && <TopBar user={user} />}
      <main
        className={
          fullBleedDocument ? "flex-1 overflow-hidden" : "flex-1 overflow-auto"
        }
      >
        <Outlet />
      </main>
    </div>
  );
}
