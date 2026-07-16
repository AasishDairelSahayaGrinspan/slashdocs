import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Check, CircleDashed, X } from "lucide-react";

import {
  api,
  type ApprovalDecision,
  type ApprovalListItem,
  type ApprovalStatus,
  type Principal,
} from "@/api";
import { Avatar } from "@/components/ui/misc";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { errorMessage } from "@/lib/errors";
import { queryKeys } from "@/lib/queryKeys";
import { samePrincipal } from "@/lib/principals";
import { useDoc } from "./doc-context";
import { Comments } from "./doc-comments";

const STATUS_BADGE: Record<
  ApprovalStatus,
  { label: string; variant: "success" | "warning" | "danger" }
> = {
  approved: { label: "Approved", variant: "success" },
  pending: { label: "Pending", variant: "warning" },
  changes_requested: { label: "Changes requested", variant: "danger" },
};

// ApprovalStatusBadge is the compact aggregate-status chip shown in the
// document bar. Hidden entirely when the document has no assigned approvers.
export function ApprovalStatusBadge({
  status,
  approved,
  total,
}: {
  status?: ApprovalStatus;
  approved?: number;
  total?: number;
}) {
  if (!status) return null;
  const badge = STATUS_BADGE[status];
  const count = status === "pending" && total ? ` (${approved ?? 0}/${total})` : "";
  return (
    <Badge variant={badge.variant} className="shrink-0">
      {badge.label + count}
    </Badge>
  );
}

type PanelTab = "comments" | "approvals";

// ReviewPanel hosts the side panel's segmented Comments / Approvals switch.
// The Approvals tab only appears once the document has assigned approvers or
// the viewer is the owner (who can assign the first one).
export function ReviewPanel() {
  const { docId, principal } = useDoc();
  const [tab, setTab] = useState<PanelTab>("comments");
  const doc = useQuery({
    queryKey: queryKeys.document(docId),
    queryFn: () => api.getDocument(docId),
  });
  const isOwner = samePrincipal(principal, doc.data?.owner);
  const summary = doc.data?.approval_summary;
  const showApprovals = isOwner || !!summary;
  useEffect(() => {
    if (!showApprovals && tab === "approvals") setTab("comments");
  }, [showApprovals, tab]);
  if (!showApprovals) return <Comments />;
  return (
    <div className="flex h-full flex-col">
      <div className="border-b border-[var(--color-border)] px-4 pt-3">
        <div className="flex gap-1 rounded-md bg-[var(--color-surface-muted)] p-0.5 text-xs">
          {(["comments", "approvals"] as const).map((t) => (
            <button
              key={t}
              onClick={() => setTab(t)}
              className={`flex-1 rounded px-2 py-1 capitalize transition-colors ${tab === t ? "bg-[var(--color-surface)] text-[var(--color-fg)] shadow-sm" : "text-[var(--color-fg-muted)]"}`}
            >
              {t === "approvals" && summary
                ? `Approvals (${summary.approved}/${summary.total})`
                : t}
            </button>
          ))}
        </div>
      </div>
      <div className="min-h-0 flex-1">
        {tab === "comments" ? <Comments /> : <Approvals isOwner={isOwner} />}
      </div>
    </div>
  );
}

function decisionLabel(item: ApprovalListItem) {
  if (item.decision === "approved") {
    return (
      <span className="flex items-center gap-1 text-xs font-medium text-green-700 dark:text-green-300">
        <Check className="h-3.5 w-3.5" /> Approved
        {item.carried_from_version_id ? (
          <span className="font-normal text-[var(--color-fg-muted)]">
            (carried forward)
          </span>
        ) : null}
      </span>
    );
  }
  if (item.decision === "changes_requested") {
    return (
      <span className="flex items-center gap-1 text-xs font-medium text-red-700 dark:text-red-300">
        <X className="h-3.5 w-3.5" /> Changes requested
      </span>
    );
  }
  return (
    <span className="flex items-center gap-1 text-xs text-[var(--color-fg-muted)]">
      <CircleDashed className="h-3.5 w-3.5" /> Pending
    </span>
  );
}

// Approvals renders the per-approver decision list for the version being
// viewed, with decision actions for the signed-in approver and assignment
// controls for the owner.
export function Approvals({ isOwner }: { isOwner: boolean }) {
  const { docId, version, principal } = useDoc();
  const q = useQueryClient();
  const doc = useQuery({
    queryKey: queryKeys.document(docId),
    queryFn: () => api.getDocument(docId),
  });
  const latest = doc.data?.current_version_id;
  const isCurrentVersion = !version || !latest || version === latest;
  const approvals = useQuery({
    queryKey: queryKeys.approvals(docId, version),
    queryFn: () => api.listApprovals(docId, version),
    enabled: !!docId,
  });

  const invalidate = () => {
    q.invalidateQueries({ queryKey: queryKeys.approvalsAll(docId) });
    q.invalidateQueries({ queryKey: queryKeys.approvers(docId) });
    q.invalidateQueries({ queryKey: queryKeys.document(docId) });
    q.invalidateQueries({ queryKey: queryKeys.documents() });
  };

  const decide = useMutation({
    mutationFn: (decision: ApprovalDecision) =>
      api.createApproval(docId, version, decision),
    onSuccess: (a) => {
      toast.success(a.decision === "approved" ? "Approved." : "Changes requested.");
      invalidate();
    },
    onError: (e) => toast.error(errorMessage(e, "Could not record decision")),
  });
  const revoke = useMutation({
    mutationFn: (approvalId: string) => api.deleteApproval(docId, approvalId),
    onSuccess: () => {
      toast.success("Decision revoked.");
      invalidate();
    },
    onError: (e) => toast.error(errorMessage(e, "Could not revoke decision")),
  });

  const items = approvals.data?.items || [];
  const summary = approvals.data?.summary;
  const own = items.find((item) => samePrincipal(item.approver, principal));

  return (
    <div className="flex h-full flex-col">
      <div className="border-b border-[var(--color-border)] px-4 py-3">
        <div className="flex items-center justify-between pr-10 md:pr-0">
          <h2 className="text-sm font-semibold">Approvals</h2>
          <ApprovalStatusBadge
            status={approvals.data?.status}
            approved={summary?.approved}
            total={summary?.total}
          />
        </div>
        {!isCurrentVersion ? (
          <p className="mt-1 text-[11px] text-[var(--color-fg-muted)]">
            Viewing decisions on an older version.
          </p>
        ) : null}
      </div>
      <div className="flex-1 space-y-1 overflow-y-auto p-3">
        {approvals.isLoading ? (
          <p className="px-1 text-xs text-[var(--color-fg-muted)]">Loading…</p>
        ) : items.length === 0 ? (
          <p className="px-1 text-xs text-[var(--color-fg-muted)]">
            No approvers yet.
            {isOwner ? " Assign someone below to request a review." : ""}
          </p>
        ) : (
          items.map((item) => (
            <ApproverRow
              key={item.approver.type + ":" + item.approver.id}
              item={item}
              docId={docId}
              isOwner={isOwner}
              onChanged={invalidate}
            />
          ))
        )}
      </div>
      {own && isCurrentVersion ? (
        <div className="flex gap-2 border-t border-[var(--color-border)] p-3">
          {own.decision !== "approved" ? (
            <Button
              size="sm"
              disabled={decide.isPending}
              onClick={() => decide.mutate("approved")}
            >
              <Check className="h-3.5 w-3.5" /> Approve
            </Button>
          ) : null}
          {own.decision !== "changes_requested" ? (
            <Button
              size="sm"
              variant="secondary"
              disabled={decide.isPending}
              onClick={() => decide.mutate("changes_requested")}
            >
              Request changes
            </Button>
          ) : null}
          {own.decision && own.id ? (
            <Button
              size="sm"
              variant="ghost"
              disabled={revoke.isPending}
              onClick={() => revoke.mutate(own.id!)}
            >
              Revoke
            </Button>
          ) : null}
        </div>
      ) : null}
      {isOwner ? <AddApprover docId={docId} onAdded={invalidate} /> : null}
    </div>
  );
}

function ApproverRow({
  item,
  docId,
  isOwner,
  onChanged,
}: {
  item: ApprovalListItem;
  docId: string;
  isOwner: boolean;
  onChanged: () => void;
}) {
  const approvers = useQuery({
    queryKey: queryKeys.approvers(docId),
    queryFn: () => api.listApprovers(docId),
    enabled: isOwner,
    staleTime: 30_000,
  });
  const remove = useMutation({
    mutationFn: (approverId: string) => api.deleteApprover(docId, approverId),
    onSuccess: () => {
      toast.success("Approver removed.");
      onChanged();
    },
    onError: (e) => toast.error(errorMessage(e, "Could not remove approver")),
  });
  const request = approvers.data?.items.find((r) =>
    samePrincipal(r.approver, item.approver),
  );
  const label = item.approver.name || item.approver.email || item.approver.id;
  return (
    <div className="group flex items-center gap-2 rounded-md px-1.5 py-1.5 hover:bg-[var(--color-surface-muted)]">
      <Avatar name={label} src={item.approver.picture_url} size={24} />
      <div className="min-w-0 flex-1">
        <div className="truncate text-xs font-medium">{label}</div>
        {decisionLabel(item)}
      </div>
      {isOwner && request ? (
        <Button
          size="icon"
          variant="ghost"
          className="h-6 w-6 opacity-0 transition-opacity group-hover:opacity-100"
          aria-label={`Remove ${label} as approver`}
          disabled={remove.isPending}
          onClick={() => remove.mutate(request.id)}
        >
          <X className="h-3.5 w-3.5" />
        </Button>
      ) : null}
    </div>
  );
}

function AddApprover({ docId, onAdded }: { docId: string; onAdded: () => void }) {
  const [address, setAddress] = useState("");
  const [debounced, setDebounced] = useState("");
  useEffect(() => {
    const t = window.setTimeout(() => setDebounced(address.trim()), 250);
    return () => window.clearTimeout(t);
  }, [address]);
  const suggestions = useQuery({
    queryKey: queryKeys.userSearch(debounced),
    queryFn: () => api.searchUsers(debounced),
    enabled: debounced.length >= 2,
    staleTime: 30_000,
  });
  const add = useMutation({
    mutationFn: (p: Partial<Principal>) => api.createApprover(docId, p),
    onSuccess: () => {
      setAddress("");
      toast.success("Approver assigned.");
      onAdded();
    },
    onError: (e) => toast.error(errorMessage(e, "Could not assign approver")),
  });
  const items = suggestions.data?.items || [];
  return (
    <form
      className="border-t border-[var(--color-border)] p-3"
      onSubmit={(e) => {
        e.preventDefault();
        if (address.trim()) add.mutate({ type: "user", email: address.trim() });
      }}
    >
      <div className="flex gap-2">
        <Input
          placeholder="Add approver by email…"
          value={address}
          onChange={(e) => setAddress(e.target.value)}
          className="h-8 text-xs"
        />
        <Button size="sm" type="submit" disabled={!address.trim() || add.isPending}>
          Add
        </Button>
      </div>
      {address && items.length > 0 ? (
        <div className="mt-1 overflow-hidden rounded-md border border-[var(--color-border)]">
          {items.slice(0, 5).map((u) => (
            <button
              key={u.id}
              type="button"
              className="flex w-full items-center gap-2 px-2 py-1.5 text-left text-xs hover:bg-[var(--color-surface-muted)]"
              onClick={() => setAddress(u.email || "")}
            >
              <span className="font-medium">{u.name || u.email}</span>
              <span className="text-[var(--color-fg-muted)]">{u.email}</span>
            </button>
          ))}
        </div>
      ) : null}
    </form>
  );
}
