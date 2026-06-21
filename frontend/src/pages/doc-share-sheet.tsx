import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { errorMessage } from "@/lib/errors";
import { Copy } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/misc";
import {
  SheetBody,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { api } from "@/api";
import { queryKeys } from "@/lib/queryKeys";
import { ROLES, DEFAULT_ROLE, GENERAL_ACCESS_NONE } from "@/lib/constants";

export function ShareSheet({ docId, resourceType = "document", resourceId }: { docId?: string; resourceType?: "document" | "folder"; resourceId?: string }) {
  const id = resourceId || docId || "";
  const isFolder = resourceType === "folder";
  const q = useQueryClient();
  const grants = useQuery({
    queryKey: isFolder ? queryKeys.folderGrants(id) : queryKeys.grants(id),
    queryFn: () => isFolder ? api.listFolderGrants(id) : api.listGrants(id),
  });
  const config = useQuery({
    queryKey: ["config"],
    queryFn: () => api.getConfig(),
    staleTime: Infinity,
  });
  const [address, setAddress] = useState("");
  const [debouncedAddress, setDebouncedAddress] = useState("");
  const [role, setRole] = useState(DEFAULT_ROLE);
  const shareURL = `${window.location.origin}/app/${isFolder ? "folders" : "d"}/${id}`;
  useEffect(() => {
    const t = window.setTimeout(() => setDebouncedAddress(address.trim()), 250);
    return () => window.clearTimeout(t);
  }, [address]);
  const userSearch = useQuery({
    queryKey: ["users", "search", debouncedAddress],
    queryFn: () => api.searchUsers(debouncedAddress),
    enabled: debouncedAddress.length >= 2 && !debouncedAddress.includes(".bot"),
    staleTime: 30_000,
  });
  const userSuggestions = userSearch.data?.items || [];
  const m = useMutation({
    mutationFn: () => isFolder ? api.createFolderGrant(id, address.trim(), role) : api.createGrant(id, address.trim(), role),
    onSuccess: () => {
      setAddress("");
      toast.success("Access granted.");
      q.invalidateQueries({ queryKey: isFolder ? queryKeys.folderGrants(id) : queryKeys.grants(id) });
    },
    onError: (e) => toast.error(errorMessage(e, "Couldn't share.")),
  });
  const items = grants.data?.items || [];
  const anyoneGrant = items.find((g) => g.principal?.type === "anyone");
  const namedItems = items.filter((g) => g.principal?.type !== "anyone");
  const everyoneLabel = config.data?.everyone_label || "Everyone on this server";
  const userPlaceholder =
    config.data?.deployment === "public" ? "user@gmail.com" : "user@org.example";
  const updateGrant = useMutation({
    mutationFn: ({ grantId, nextRole }: { grantId: string; nextRole: string }) =>
      isFolder ? api.updateFolderGrant(id, grantId, nextRole) : api.updateGrant(id, grantId, nextRole),
    onSuccess: () => {
      toast.success("Access updated.");
      q.invalidateQueries({ queryKey: isFolder ? queryKeys.folderGrants(id) : queryKeys.grants(id) });
    },
    onError: (e) => toast.error(errorMessage(e, "Couldn't update access.")),
  });
  const revokeGrant = useMutation({
    mutationFn: (grantId: string) => isFolder ? api.deleteFolderGrant(id, grantId) : api.deleteGrant(id, grantId),
    onSuccess: () => {
      toast.success("Access revoked.");
      q.invalidateQueries({ queryKey: isFolder ? queryKeys.folderGrants(id) : queryKeys.grants(id) });
    },
    onError: (e) => toast.error(errorMessage(e, "Couldn't revoke access.")),
  });
  const general = useMutation({
    mutationFn: async (next: string) => {
      if (next === GENERAL_ACCESS_NONE) {
        if (anyoneGrant) {
          if (isFolder) await api.deleteFolderGrant(id, anyoneGrant.id);
          else await api.deleteGrant(id, anyoneGrant.id);
        }
        return;
      }
      if (isFolder) await api.createFolderGrant(id, "", next);
      else await api.setGeneralAccess(id, next);
    },
    onSuccess: () => {
      toast.success("General access updated.");
      q.invalidateQueries({ queryKey: isFolder ? queryKeys.folderGrants(id) : queryKeys.grants(id) });
    },
    onError: (e) => toast.error(errorMessage(e, "Couldn't update general access.")),
  });
  return (
    <SheetContent>
      <SheetHeader>
        <SheetTitle>Share {isFolder ? "folder" : "document"}</SheetTitle>
      </SheetHeader>
      <SheetBody>
        <form
          className="space-y-3"
          onSubmit={(e) => {
            e.preventDefault();
            m.mutate();
          }}
        >
          <div>
            <Input
              placeholder={`${userPlaceholder}  or  report-creator@brave.otter.bot`}
              value={address}
              onChange={(e) => setAddress(e.target.value)}
              autoCapitalize="off"
              autoCorrect="off"
              spellCheck={false}
            />
            <p className="mt-1.5 text-[11px] text-[var(--color-fg-muted)]">
              Search existing aidocs users by email, or add a bot by its address.
            </p>
            {userSuggestions.length > 0 ? (
              <div className="mt-2 overflow-hidden rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)]">
                {userSuggestions.map((u) => (
                  <button
                    key={u.id || u.email}
                    type="button"
                    className="block w-full px-3 py-2 text-left text-sm hover:bg-[var(--color-surface-muted)]"
                    onClick={() => setAddress(u.email || "")}
                  >
                    <div className="font-medium">{u.name || u.email}</div>
                    {u.name && u.email ? (
                      <div className="text-xs text-[var(--color-fg-muted)]">
                        {u.email}
                      </div>
                    ) : null}
                  </button>
                ))}
              </div>
            ) : debouncedAddress.length >= 2 &&
              !debouncedAddress.includes(".bot") &&
              !userSearch.isFetching ? (
              <div className="mt-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-muted)] px-3 py-2 text-xs text-[var(--color-fg-muted)]">
                No matching aidocs users. They need to sign in once before you can share
                with them.
              </div>
            ) : null}
          </div>
          <select
            value={role}
            onChange={(e) => setRole(e.target.value)}
            aria-label="Access role"
            className="h-9 w-full rounded-[10px] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 text-sm"
          >
            {ROLES.map((r) => (
              <option key={r.value} value={r.value}>
                {r.label}
              </option>
            ))}
          </select>
          <Button
            className="w-full"
            type="submit"
            disabled={!address.trim() || m.isPending}
          >
            Share
          </Button>
        </form>
        <div className="mt-6">
          <h4 className="mb-2 text-xs font-medium uppercase tracking-wider text-[var(--color-fg-muted)]">
            People with access
          </h4>
          <div className="space-y-1">
            {grants.isLoading ? (
              <>
                <Skeleton className="h-9 w-full" />
                <Skeleton className="h-9 w-full" />
              </>
            ) : grants.error ? (
              <div className="text-xs text-[var(--color-danger)]">
                Could not load access list.
              </div>
            ) : namedItems.length === 0 ? (
              <div className="text-xs text-[var(--color-fg-muted)]">
                No people or bots added yet.
              </div>
            ) : (
              namedItems.map((g) => (
                <div
                  key={g.id}
                  className="flex items-center justify-between gap-3 rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm"
                >
                  <div className="min-w-0">
                    <div className="truncate font-medium">
                      {g.principal?.name || g.principal?.email || g.principal?.id}
                      {g.principal?.type === "service_account" ? " (bot)" : ""}
                    </div>
                    {g.principal?.name && g.principal?.email ? (
                      <div className="truncate text-xs text-[var(--color-fg-muted)]">
                        {g.principal.email}
                      </div>
                    ) : null}
                  </div>
                  <select
                    value={g.role}
                    disabled={updateGrant.isPending || revokeGrant.isPending}
                    onChange={(e) => {
                      const next = e.target.value;
                      if (next === "revoke") revokeGrant.mutate(g.id);
                      else updateGrant.mutate({ grantId: g.id, nextRole: next });
                    }}
                    aria-label="Access role"
                    className="h-8 shrink-0 rounded-[10px] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 text-sm"
                  >
                    {ROLES.map((r) => (
                      <option key={r.value} value={r.value}>
                        {r.label}
                      </option>
                    ))}
                    <option value="revoke">Remove access</option>
                  </select>
                </div>
              ))
            )}
          </div>
        </div>
        <div className="mt-6">
          <h4 className="mb-2 text-xs font-medium uppercase tracking-wider text-[var(--color-fg-muted)]">
            General access
          </h4>
          <div className="flex items-center justify-between gap-3 rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm">
            <span className="truncate">{everyoneLabel}</span>
            <select
              value={anyoneGrant?.role || GENERAL_ACCESS_NONE}
              disabled={general.isPending}
              onChange={(e) => general.mutate(e.target.value)}
              aria-label="General access role"
              className="h-8 rounded-[10px] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 text-sm"
            >
              <option value={GENERAL_ACCESS_NONE}>No access</option>
              {ROLES.map((r) => (
                <option key={r.value} value={r.value}>
                  {r.label}
                </option>
              ))}
            </select>
          </div>
        </div>
        <div className="mt-6 rounded-lg border border-dashed border-[var(--color-border)] bg-[var(--color-surface-muted)]/45 p-3">
          <div className="flex items-center justify-between gap-3">
            <div className="text-sm font-medium">Document link</div>
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="shrink-0"
              onClick={async () => {
                await navigator.clipboard.writeText(shareURL);
                toast.success("Share link copied.");
              }}
            >
              <Copy className="h-4 w-4" /> Copy link
            </Button>
          </div>
        </div>
      </SheetBody>
    </SheetContent>
  );
}
