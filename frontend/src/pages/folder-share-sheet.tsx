import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { errorMessage } from "@/lib/errors";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Avatar, Skeleton } from "@/components/ui/misc";
import {
  SheetBody,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { api, type EffectiveAccessItem } from "@/api";
import { queryKeys } from "@/lib/queryKeys";
import { ROLES, DEFAULT_ROLE } from "@/lib/constants";

export function FolderShareSheet({ folderId }: { folderId: string }) {
  const qc = useQueryClient();
  const access = useQuery({
    queryKey: queryKeys.folderAccess(folderId),
    queryFn: () => api.folderAccess(folderId),
  });
  const [address, setAddress] = useState("");
  const [debouncedAddress, setDebouncedAddress] = useState("");
  const [role, setRole] = useState(DEFAULT_ROLE);
  useEffect(() => {
    const t = window.setTimeout(() => setDebouncedAddress(address.trim()), 250);
    return () => window.clearTimeout(t);
  }, [address]);
  const userSearch = useQuery({
    queryKey: ["users", "search", debouncedAddress],
    queryFn: () => api.searchUsers(debouncedAddress),
    enabled: debouncedAddress.length >= 2,
    staleTime: 30_000,
  });
  const userSuggestions = userSearch.data?.items || [];
  const share = useMutation({
    mutationFn: () => api.createFolderGrant(folderId, address.trim(), role),
    onSuccess: () => {
      setAddress("");
      toast.success("Access granted.");
      qc.invalidateQueries({ queryKey: queryKeys.folderAccess(folderId) });
      qc.invalidateQueries({ queryKey: queryKeys.folderGrants(folderId) });
    },
    onError: (e) => toast.error(errorMessage(e, "Couldn't share.")),
  });

  const items = access.data?.items || [];

  return (
    <SheetContent side="right" className="w-[420px]">
      <SheetHeader>
        <SheetTitle>Share folder</SheetTitle>
      </SheetHeader>
      <SheetBody className="space-y-4">
        {/* Add people */}
        <form
          className="flex gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            if (address.trim()) share.mutate();
          }}
        >
          <div className="relative flex-1">
            <Input
              value={address}
              onChange={(e) => setAddress(e.target.value)}
              placeholder="Add people by email…"
            />
            {userSuggestions.length > 0 && debouncedAddress.length >= 2 && (
              <div className="absolute left-0 right-0 top-full z-10 mt-1 rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] py-1 shadow-md">
                {userSuggestions.map((u) => (
                  <button
                    key={u.id}
                    type="button"
                    className="flex w-full items-center gap-2 px-3 py-1.5 text-sm hover:bg-[var(--color-surface-muted)]"
                    onClick={() => {
                      setAddress(u.email || u.id);
                      setDebouncedAddress("");
                    }}
                  >
                    <Avatar name={u.name || u.email || ""} size={20} />
                    <span>{u.email || u.id}</span>
                  </button>
                ))}
              </div>
            )}
          </div>
          <select
            className="rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] px-2 text-sm"
            value={role}
            onChange={(e) => setRole(e.target.value)}
          >
            {ROLES.map((r) => (
              <option key={r.value} value={r.value}>
                {r.label}
              </option>
            ))}
          </select>
          <Button type="submit" size="sm" disabled={!address.trim() || share.isPending}>
            Share
          </Button>
        </form>

        {/* Effective access list */}
        <div className="space-y-1">
          <h3 className="text-xs font-medium uppercase tracking-wider text-[var(--color-fg-muted)]">
            People with access
          </h3>
          {access.isLoading ? (
            <Skeleton className="h-32 w-full" />
          ) : items.length === 0 ? (
            <p className="py-4 text-center text-sm text-[var(--color-fg-muted)]">
              Only you have access.
            </p>
          ) : (
            <ul className="space-y-1">
              {items.map((item) => (
                <AccessRow key={principalKey(item)} item={item} folderId={folderId} />
              ))}
            </ul>
          )}
        </div>
      </SheetBody>
    </SheetContent>
  );
}

function AccessRow({
  item,
  folderId,
}: {
  item: EffectiveAccessItem;
  folderId: string;
}) {
  const p = item.principal;
  const label = p.name || p.email || p.id || "Unknown";
  const detail = p.email && p.email !== label ? p.email : roleLabel(item.effective_role);
  const isInherited = item.sources.every((s) => s.resource_id !== folderId);
  const inheritedFrom = isInherited
    ? item.sources.find((s) => s.resource_type === "folder")
    : null;

  return (
    <li className="flex items-center gap-3 rounded-md px-2 py-2 hover:bg-[var(--color-surface-muted)]/40">
      <Avatar name={label} src={p.picture_url} size={32} />
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm font-medium">{label}</div>
        <div className="truncate text-xs text-[var(--color-fg-muted)]">{detail}</div>
      </div>
      <div className="flex items-center gap-2">
        <span className="text-xs capitalize text-[var(--color-fg-muted)]">
          {item.effective_role}
        </span>
        {isInherited && inheritedFrom && (
          <span className="text-[10px] text-[var(--color-fg-muted)]">
            via {inheritedFrom.resource_title}
          </span>
        )}
      </div>
    </li>
  );
}

function principalKey(item: EffectiveAccessItem) {
  return `${item.principal.type}:${item.principal.id}`;
}

function roleLabel(role: string) {
  return role.charAt(0).toUpperCase() + role.slice(1);
}
