import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "react-hot-toast";
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
import { api, AccessSource, EffectiveAccessItem } from "@/api";
import { queryKeys } from "@/lib/queryKeys";
import { ROLES, DEFAULT_ROLE, GENERAL_ACCESS_NONE } from "@/lib/constants";

const ROLE_RANK: Record<string, number> = { none: 0, viewer: 1, commenter: 2, editor: 3, owner: 4 };

type PendingChange = {
  label: string;
  source: AccessSource;
  action: "update" | "revoke";
  newRole?: string;
};

export function ShareSheet({ docId, resourceType = "document", resourceId }: { docId?: string; resourceType?: "document" | "folder"; resourceId?: string }) {
  const id = resourceId || docId || "";
  const isFolder = resourceType === "folder";
  const q = useQueryClient();
  const accessKey = isFolder ? queryKeys.folderAccess(id) : queryKeys.documentAccess(id);
  const access = useQuery({
    queryKey: accessKey,
    queryFn: () => isFolder ? api.folderAccess(id) : api.documentAccess(id),
  });
  const config = useQuery({
    queryKey: ["config"],
    queryFn: () => api.getConfig(),
    staleTime: Infinity,
  });
  const [address, setAddress] = useState("");
  const [debouncedAddress, setDebouncedAddress] = useState("");
  const [role, setRole] = useState(DEFAULT_ROLE);
  const [pendingChange, setPendingChange] = useState<PendingChange | null>(null);
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

  const items = access.data?.items || [];
  const anyoneItem = items.find((i) => i.principal?.type === "anyone");
  const namedItems = items.filter((i) => i.principal?.type !== "anyone" && i.principal?.id && i.effective_role !== "owner");

  const anyoneDirectSource = useMemo(() => {
    if (!anyoneItem) return undefined;
    return anyoneItem.sources?.find((s) => s.resource_id === id);
  }, [anyoneItem, id]);

  const anyoneInheritedSource = useMemo(() => {
    if (!anyoneItem) return undefined;
    return anyoneItem.sources?.find((s) => s.resource_id !== id);
  }, [anyoneItem, id]);

  const anyoneEffectiveRole = anyoneItem?.effective_role || GENERAL_ACCESS_NONE;
  const anyoneIsInherited = !anyoneDirectSource && !!anyoneInheritedSource;

  const directGrantId = (item: EffectiveAccessItem) =>
    item.sources?.find((s) => s.resource_id === id)?.grant_id;

  const inheritedSource = (item: EffectiveAccessItem) =>
    item.sources?.find((s) => s.resource_id !== id && s.grant_id);

  const isInheritedOnly = (item: EffectiveAccessItem) =>
    !item.sources?.some((s) => s.resource_id === id);

  const invalidateAccess = () => {
    q.invalidateQueries({ queryKey: accessKey });
    q.invalidateQueries({ queryKey: ["folder-access"] });
  };

  const m = useMutation({
    mutationFn: () => isFolder ? api.createFolderGrant(id, address.trim(), role) : api.createGrant(id, address.trim(), role),
    onSuccess: () => {
      setAddress("");
      toast.success("Access granted.");
      invalidateAccess();
    },
    onError: (e) => toast.error(errorMessage(e, "Couldn't share.")),
  });
  const updateGrant = useMutation({
    mutationFn: ({ resourceId: rid, grantId, nextRole }: { resourceId: string; grantId: string; nextRole: string }) =>
      api.updateFolderGrant(rid, grantId, nextRole),
    onSuccess: () => {
      toast.success("Access updated.");
      invalidateAccess();
      setPendingChange(null);
    },
    onError: (e) => toast.error(errorMessage(e, "Couldn't update access.")),
  });
  const revokeGrant = useMutation({
    mutationFn: ({ resourceId: rid, grantId }: { resourceId: string; grantId: string }) =>
      api.deleteFolderGrant(rid, grantId),
    onSuccess: () => {
      toast.success("Access revoked.");
      invalidateAccess();
      setPendingChange(null);
    },
    onError: (e) => toast.error(errorMessage(e, "Couldn't revoke access.")),
  });
  const updateDirectGrant = useMutation({
    mutationFn: ({ grantId, nextRole }: { grantId: string; nextRole: string }) =>
      isFolder ? api.updateFolderGrant(id, grantId, nextRole) : api.updateGrant(id, grantId, nextRole),
    onSuccess: () => {
      toast.success("Access updated.");
      invalidateAccess();
    },
    onError: (e) => toast.error(errorMessage(e, "Couldn't update access.")),
  });
  const revokeDirectGrant = useMutation({
    mutationFn: (grantId: string) => isFolder ? api.deleteFolderGrant(id, grantId) : api.deleteGrant(id, grantId),
    onSuccess: () => {
      toast.success("Access revoked.");
      invalidateAccess();
    },
    onError: (e) => toast.error(errorMessage(e, "Couldn't revoke access.")),
  });
  const general = useMutation({
    mutationFn: async (next: string) => {
      if (next === GENERAL_ACCESS_NONE) {
        if (anyoneDirectSource?.grant_id) {
          if (isFolder) await api.deleteFolderGrant(id, anyoneDirectSource.grant_id);
          else await api.deleteGrant(id, anyoneDirectSource.grant_id);
        }
        return;
      }
      if (isFolder) await api.setFolderGeneralAccess(id, next);
      else await api.setGeneralAccess(id, next);
    },
    onSuccess: () => {
      toast.success("General access updated.");
      invalidateAccess();
      setPendingChange(null);
    },
    onError: (e) => toast.error(errorMessage(e, "Couldn't update general access.")),
  });

  const confirmPending = () => {
    if (!pendingChange) return;
    const { source, action, newRole } = pendingChange;
    if (action === "revoke") {
      revokeGrant.mutate({ resourceId: source.resource_id, grantId: source.grant_id! });
    } else if (newRole) {
      updateGrant.mutate({ resourceId: source.resource_id, grantId: source.grant_id!, nextRole: newRole });
    }
  };

  const handleInheritedChange = (item: EffectiveAccessItem, nextValue: string) => {
    const src = inheritedSource(item);
    if (!src?.grant_id) return;
    const label = item.principal?.name || item.principal?.email || item.principal?.id || "this principal";
    if (nextValue === "revoke") {
      setPendingChange({ label, source: src, action: "revoke" });
    } else {
      setPendingChange({ label, source: src, action: "update", newRole: nextValue });
    }
  };

  const handleGeneralInheritedChange = (nextValue: string) => {
    if (!anyoneInheritedSource?.grant_id) return;
    const label = config.data?.everyone_label || "Everyone on this server";
    if (nextValue === GENERAL_ACCESS_NONE) {
      setPendingChange({ label, source: anyoneInheritedSource, action: "revoke" });
    } else {
      setPendingChange({ label, source: anyoneInheritedSource, action: "update", newRole: nextValue });
    }
  };

  const everyoneLabel = config.data?.everyone_label || "Everyone on this server";
  const userPlaceholder =
    config.data?.deployment === "public" ? "user@gmail.com" : "user@org.example";

  return (
    <SheetContent>
      <SheetHeader>
        <SheetTitle>Share {isFolder ? "folder" : "document"}</SheetTitle>
      </SheetHeader>
      <SheetBody>
        {pendingChange ? (
          <div style={{
            display: "flex",
            flexDirection: "column",
            gap: "12px",
            borderRadius: "8px",
            border: "1px solid #d97706",
            background: "#f4f4f5",
            padding: "16px",
          }}>
            <p style={{ fontSize: "14px", fontWeight: 500 }}>
              {pendingChange.action === "revoke" ? "Remove" : "Change"} access for {pendingChange.label}?
            </p>
            <p style={{ fontSize: "12px", color: "#71717a" }}>
              This access is inherited from <span style={{ fontWeight: 500 }}>{pendingChange.source.resource_title}</span>.
              {pendingChange.action === "revoke"
                ? " Removing it will affect everyone who accesses items through that folder."
                : " Changing it will affect everyone who accesses items through that folder."}
            </p>
            <div style={{ display: "flex", gap: "8px" }}>
              <Button
                size="sm"
                variant="outline"
                onClick={() => setPendingChange(null)}
              >
                Cancel
              </Button>
              <Button
                size="sm"
                disabled={updateGrant.isPending || revokeGrant.isPending}
                onClick={confirmPending}
              >
                {pendingChange.action === "revoke" ? "Remove from folder" : "Change on folder"}
              </Button>
            </div>
          </div>
        ) : (
          <>
            <form
              style={{ display: "flex", flexDirection: "column", gap: "12px" }}
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
                <p style={{ marginTop: "6px", fontSize: "11px", color: "#71717a" }}>
                  Search existing aidocs users by email, or add a bot by its address.
                </p>
                {userSuggestions.length > 0 ? (
                  <div style={{ marginTop: "8px", overflow: "hidden", borderRadius: "8px", border: "1px solid #e4e4e7", background: "#fff" }}>
                    {userSuggestions.map((u) => (
                      <button
                        key={u.id || u.email}
                        type="button"
                        style={{ display: "block", width: "100%", padding: "8px 12px", textAlign: "left", fontSize: "14px", background: "transparent", border: "none", cursor: "pointer" }}
                        onClick={() => setAddress(u.email || "")}
                      >
                        <div style={{ fontWeight: 500 }}>{u.name || u.email}</div>
                        {u.name && u.email ? (
                          <div style={{ fontSize: "12px", color: "#71717a" }}>
                            {u.email}
                          </div>
                        ) : null}
                      </button>
                    ))}
                  </div>
                ) : debouncedAddress.length >= 2 &&
                  !debouncedAddress.includes(".bot") &&
                  !userSearch.isFetching ? (
                  <div style={{ marginTop: "8px", borderRadius: "8px", border: "1px solid #e4e4e7", background: "#f4f4f5", padding: "8px 12px", fontSize: "12px", color: "#71717a" }}>
                    No matching aidocs users. They need to sign in once before you can share
                    with them.
                  </div>
                ) : null}
              </div>
              <select
                value={role}
                onChange={(e) => setRole(e.target.value)}
                aria-label="Access role"
                style={{
                  height: "36px",
                  width: "100%",
                  borderRadius: "10px",
                  border: "1px solid #e4e4e7",
                  background: "#fff",
                  padding: "0 12px",
                  fontSize: "14px",
                }}
              >
                {ROLES.map((r) => (
                  <option key={r.value} value={r.value}>
                    {r.label}
                  </option>
                ))}
              </select>
              <Button
                isFullWidth
                type="submit"
                disabled={!address.trim() || m.isPending}
              >
                Share
              </Button>
            </form>
            <div style={{ marginTop: "24px" }}>
              <h4 style={{ marginBottom: "8px", fontSize: "12px", fontWeight: 500, textTransform: "uppercase", letterSpacing: "0.05em", color: "#71717a" }}>
                People with access
              </h4>
              <div style={{ display: "flex", flexDirection: "column", gap: "4px" }}>
                {access.isLoading ? (
                  <>
                    <Skeleton style={{ height: "36px", width: "100%" }} />
                    <Skeleton style={{ height: "36px", width: "100%" }} />
                  </>
                ) : access.error ? (
                  <div style={{ fontSize: "12px", color: "#dc2626" }}>
                    Could not load access list.
                  </div>
                ) : namedItems.length === 0 ? (
                  <div style={{ fontSize: "12px", color: "#71717a" }}>
                    No people or bots added yet.
                  </div>
                ) : (
                  namedItems.map((item) => {
                    const grantId = directGrantId(item);
                    const inherited = isInheritedOnly(item);
                    const src = inheritedSource(item);
                    return (
                      <div
                        key={item.principal.type + ":" + item.principal.id}
                        style={{
                          display: "flex",
                          alignItems: "center",
                          justifyContent: "space-between",
                          gap: "12px",
                          borderRadius: "6px",
                          border: "1px solid #e4e4e7",
                          background: "#fff",
                          padding: "8px 12px",
                          fontSize: "14px",
                        }}
                      >
                        <div style={{ minWidth: 0 }}>
                          <div style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", fontWeight: 500 }}>
                            {item.principal?.name || item.principal?.email || item.principal?.id}
                            {item.principal?.type === "service_account" ? " (bot)" : ""}
                          </div>
                          {item.principal?.name && item.principal?.email ? (
                            <div style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", fontSize: "12px", color: "#71717a" }}>
                              {item.principal.email}
                            </div>
                          ) : null}
                        </div>
                        <select
                          value={item.effective_role}
                          disabled={updateGrant.isPending || revokeGrant.isPending || updateDirectGrant.isPending || revokeDirectGrant.isPending}
                          onChange={(e) => {
                            const next = e.target.value;
                            const inheritedRole = src?.role;
                            const reducingBelowInherited = inheritedRole && (ROLE_RANK[next] || 0) < (ROLE_RANK[inheritedRole] || 0);
                            if (reducingBelowInherited) {
                              handleInheritedChange(item, next);
                            } else if (next === "revoke" && grantId) {
                              revokeDirectGrant.mutate(grantId);
                            } else if (grantId) {
                              updateDirectGrant.mutate({ grantId, nextRole: next });
                            } else {
                              handleInheritedChange(item, next);
                            }
                          }}
                          aria-label="Access role"
                          style={{
                            height: "32px",
                            flexShrink: 0,
                            borderRadius: "10px",
                            border: "1px solid #e4e4e7",
                            background: "#fff",
                            padding: "0 8px",
                            fontSize: "14px",
                          }}
                        >
                          {ROLES.map((r) => (
                            <option key={r.value} value={r.value}>
                              {r.label}
                            </option>
                          ))}
                          <option value="revoke">Remove access</option>
                        </select>
                      </div>
                    );
                  })
                )}
              </div>
            </div>
            <div style={{ marginTop: "24px" }}>
              <h4 style={{ marginBottom: "8px", fontSize: "12px", fontWeight: 500, textTransform: "uppercase", letterSpacing: "0.05em", color: "#71717a" }}>
                General access
              </h4>
              <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: "12px", borderRadius: "6px", border: "1px solid #e4e4e7", background: "#fff", padding: "8px 12px", fontSize: "14px" }}>
                <span style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{everyoneLabel}</span>
                <select
                  value={anyoneEffectiveRole}
                  disabled={general.isPending}
                  onChange={(e) => {
                    const next = e.target.value;
                    const inheritedRole = anyoneInheritedSource?.role;
                    if (inheritedRole && (ROLE_RANK[next] || 0) < (ROLE_RANK[inheritedRole] || 0)) {
                      handleGeneralInheritedChange(next);
                    } else {
                      general.mutate(next);
                    }
                  }}
                  aria-label="General access role"
                  style={{
                    height: "32px",
                    borderRadius: "10px",
                    border: "1px solid #e4e4e7",
                    background: "#fff",
                    padding: "0 8px",
                    fontSize: "14px",
                  }}
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
            <div style={{ marginTop: "24px", borderRadius: "8px", border: "1px dashed #e4e4e7", background: "rgba(244,244,245,0.45)", padding: "12px" }}>
              <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: "12px" }}>
                <div style={{ fontSize: "14px", fontWeight: 500 }}>{isFolder ? "Folder" : "Document"} link</div>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={async () => {
                    await navigator.clipboard.writeText(shareURL);
                    toast.success("Share link copied.");
                  }}
                >
                  <Copy style={{ height: "16px", width: "16px" }} /> Copy link
                </Button>
              </div>
            </div>
          </>
        )}
      </SheetBody>
    </SheetContent>
  );
}
