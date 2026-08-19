import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { errorMessage } from "@/lib/errors";
import { Bot, Loader2, Sparkles, ThumbsDown, ThumbsUp, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { api, type AvatarFeedbackState, type Comment } from "@/api";
import { queryKeys } from "@/lib/queryKeys";
import {
  countAvatarComments,
  feedbackPersona,
  isAvatarComment,
  reviewInProgress,
  reviewStatusLine,
} from "@/lib/avatar-review";
import { useDoc } from "./doc-context";
import {
  AvatarReviewContext,
  useAvatarReview,
  type AvatarReviewContextValue,
} from "./doc-avatar-context";

// Avatar reviews: the aidocs-side trigger, progress, and feedback surface.
// Everything stateful happens in the avatar service (or its mock) behind the
// server's /v1/avatars* endpoints — this file only renders what it returns.

const requestStorageKey = (docId: string, version: string) =>
  `aidocs:avatar-request:${docId}:${version}`;

function storedRequestId(docId: string, version: string): string {
  if (!docId || !version) return "";
  try {
    return localStorage.getItem(requestStorageKey(docId, version)) || "";
  } catch {
    return "";
  }
}

export function AvatarReviewProvider({ children }: { children: React.ReactNode }) {
  const { docId, version, comments } = useDoc();
  const q = useQueryClient();
  const config = useQuery({
    queryKey: queryKeys.config(),
    queryFn: api.getConfig,
    staleTime: Infinity,
  });
  const enabled = !!config.data?.avatar_reviews_enabled;

  const [pickerOpen, setPickerOpen] = useState(false);
  // The active review request for the viewed version survives reloads so a
  // convening review keeps rendering after a refresh.
  const [requestId, setRequestId] = useState(() => storedRequestId(docId, version));
  useEffect(() => {
    setRequestId(storedRequestId(docId, version));
  }, [docId, version]);

  const hasAvatarComments = comments.some(isAvatarComment);
  // The roster feeds the picker and IC-persona matching. Commenter role is
  // required server-side, so viewers simply go without (persona falls back).
  const avatars = useQuery({
    queryKey: queryKeys.avatars(docId),
    queryFn: () => api.listAvatars(docId),
    enabled: enabled && (pickerOpen || hasAvatarComments),
    staleTime: 60_000,
    retry: false,
    meta: { suppressGlobalError: true },
  });

  const status = useQuery({
    queryKey: queryKeys.avatarReview(requestId),
    queryFn: () => api.getAvatarReview(requestId),
    enabled: enabled && !!requestId,
    retry: false,
    meta: { suppressGlobalError: true },
    refetchInterval: (query) =>
      reviewInProgress(query.state.data?.reviews) ? 1500 : false,
  });

  // When a review lands its comments, pull them through the normal comment
  // pipeline.
  const reviewStates = useRef<Record<string, string>>({});
  useEffect(() => {
    for (const review of status.data?.reviews || []) {
      const previous = reviewStates.current[review.review_id];
      if (previous !== review.state && review.state === "commented") {
        void q.invalidateQueries({ queryKey: queryKeys.comments(docId) });
      }
      reviewStates.current[review.review_id] = review.state;
    }
  }, [status.data, docId, q]);

  const request = useMutation({
    mutationFn: (avatarIds: string[]) =>
      api.requestAvatarReview(docId, version, avatarIds),
    onSuccess: (created) => {
      setRequestId(created.request_id);
      try {
        localStorage.setItem(requestStorageKey(docId, version), created.request_id);
      } catch {}
      setPickerOpen(false);
    },
    onError: (e) => toast.error(errorMessage(e, "Could not request the review")),
  });

  const dismissStatus = useCallback(() => {
    setRequestId("");
    try {
      localStorage.removeItem(requestStorageKey(docId, version));
    } catch {}
  }, [docId, version]);

  // A stale stored request the service no longer knows is silently dropped.
  useEffect(() => {
    if (status.error && requestId) dismissStatus();
  }, [status.error, requestId, dismissStatus]);

  const requestReview = useCallback(
    (avatarIds: string[]) => request.mutate(avatarIds),
    [request],
  );

  const value = useMemo<AvatarReviewContextValue>(
    () => ({
      enabled,
      avatars: avatars.data?.items,
      maxSelectable: avatars.data?.max_selectable ?? 1,
      status: status.data,
      inProgress: reviewInProgress(status.data?.reviews),
      avatarCommentCount: countAvatarComments(comments, version),
      pickerOpen,
      setPickerOpen,
      requestReview,
      requesting: request.isPending,
      dismissStatus,
    }),
    [
      enabled,
      avatars.data,
      status.data,
      comments,
      version,
      pickerOpen,
      requestReview,
      request.isPending,
      dismissStatus,
    ],
  );

  return (
    <AvatarReviewContext.Provider value={value}>
      {children}
      {enabled ? <AvatarPickerDialog /> : null}
    </AvatarReviewContext.Provider>
  );
}

// AvatarButton lives in the doc bar next to Share. Three states: promoted
// default (glow), in-progress ("{Name}'s avatar is reading…"), and done
// ("Avatar · N" for avatar comments on the viewed version).
export function AvatarButton() {
  const ctx = useAvatarReview();
  if (!ctx?.enabled) return null;
  const { status, inProgress, avatarCommentCount, setPickerOpen } = ctx;
  const reading = status?.reviews.find(
    (r) => r.state === "queued" || r.state === "running",
  );
  return (
    <Button
      variant="outline"
      size="sm"
      className={`shrink-0 px-2.5 sm:px-3 ${!inProgress && avatarCommentCount === 0 ? "avatar-glow" : ""}`}
      aria-label="Avatar review"
      onClick={() => setPickerOpen(true)}
    >
      {inProgress ? (
        <>
          <Loader2 className="h-4 w-4 animate-spin" />
          <span className="hidden sm:inline">
            {reading ? `${reading.display_name} is reading…` : "Avatar"}
          </span>
        </>
      ) : (
        <>
          <Sparkles className="h-4 w-4" />
          <span className="hidden sm:inline">
            {avatarCommentCount > 0 ? `Avatar · ${avatarCommentCount}` : "Avatar"}
          </span>
          {avatarCommentCount > 0 ? (
            <span className="sm:hidden">{avatarCommentCount}</span>
          ) : null}
        </>
      )}
    </Button>
  );
}

// The picker: one roster call renders the cards; aidocs applies no
// eligibility logic of its own. The Bench teaser is always last and never
// selectable.
function AvatarPickerDialog() {
  const ctx = useAvatarReview();
  const [selected, setSelected] = useState<string[]>([]);
  if (!ctx) return null;
  const {
    avatars,
    maxSelectable,
    pickerOpen,
    setPickerOpen,
    requestReview,
    requesting,
  } = ctx;

  const toggle = (id: string) => {
    setSelected((current) => {
      if (current.includes(id)) return current.filter((s) => s !== id);
      if (maxSelectable <= 1) return [id];
      if (current.length >= maxSelectable) return current;
      return [...current, id];
    });
  };

  return (
    <Dialog
      open={pickerOpen}
      onOpenChange={(open) => {
        setPickerOpen(open);
        if (!open) setSelected([]);
      }}
    >
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>Request an avatar review</DialogTitle>
          <DialogDescription>
            A senior IC's avatar reads this version and leaves anchored comments in
            their name.
          </DialogDescription>
        </DialogHeader>
        {!avatars ? (
          <div className="rounded-lg border border-[var(--color-border)] p-4 text-sm text-[var(--color-fg-muted)]">
            Loading avatars…
          </div>
        ) : (
          <div className="max-h-[55vh] space-y-2 overflow-auto pr-1">
            {avatars.map((a) =>
              a.coming_soon ? (
                <div
                  key={a.id}
                  className="flex w-full items-start gap-3 rounded-xl border border-dashed border-[var(--color-border)] bg-[var(--color-surface-muted)]/40 p-3 text-left opacity-80"
                  title="Coming soon: the reviews-everything default avatar."
                >
                  <Bot className="mt-0.5 h-5 w-5 shrink-0 text-[var(--color-fg-muted)]" />
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="font-medium">{a.display_name}</span>
                      <Badge variant="muted">Coming soon</Badge>
                    </div>
                    <div className="mt-1 text-sm text-[var(--color-fg-muted)]">
                      {a.relevance_hint}
                    </div>
                  </div>
                </div>
              ) : (
                <button
                  key={a.id}
                  type="button"
                  onClick={() => toggle(a.id)}
                  aria-pressed={selected.includes(a.id)}
                  className={`flex w-full items-start gap-3 rounded-xl border p-3 text-left transition-colors hover:bg-[var(--color-surface-muted)]/60 ${
                    selected.includes(a.id)
                      ? "border-[var(--color-accent)] bg-[var(--color-accent-muted)]/35 ring-1 ring-[var(--color-accent)]"
                      : "border-[var(--color-border)] bg-[var(--color-surface)]"
                  }`}
                >
                  <Bot className="mt-0.5 h-5 w-5 shrink-0 text-[var(--color-accent)]" />
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="font-medium">{a.display_name}</span>
                      {a.focus_areas.map((f) => (
                        <Badge key={f} variant="muted">
                          {f}
                        </Badge>
                      ))}
                    </div>
                    <div className="mt-1 text-sm text-[var(--color-fg-muted)]">
                      {a.relevance_hint}
                    </div>
                  </div>
                </button>
              ),
            )}
          </div>
        )}
        <div className="flex justify-end gap-2">
          <Button variant="ghost" size="sm" onClick={() => setPickerOpen(false)}>
            Cancel
          </Button>
          <Button
            size="sm"
            disabled={selected.length === 0 || requesting}
            onClick={() => requestReview(selected)}
          >
            {requesting ? <Loader2 className="h-4 w-4 animate-spin" /> : null}
            Request review
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}

// Per-avatar convening lines at the top of the comments panel. Kept
// per-avatar (a list of one today) because multi-avatar panels are upcoming
// and each avatar completes independently.
export function AvatarReviewProgress() {
  const ctx = useAvatarReview();
  if (!ctx?.enabled || !ctx.status) return null;
  const { status, inProgress, dismissStatus } = ctx;
  return (
    <div className="border-b border-[var(--color-border)] bg-[var(--color-accent-muted)]/20 px-4 py-2">
      {status.reviews.map((review) => (
        <div
          key={review.review_id}
          className="flex items-center gap-2 py-1 text-xs text-[var(--color-fg)]"
        >
          {review.state === "queued" || review.state === "running" ? (
            <Loader2 className="h-3.5 w-3.5 shrink-0 animate-spin text-[var(--color-accent)]" />
          ) : review.state === "failed" ? (
            <X className="h-3.5 w-3.5 shrink-0 text-[var(--color-danger)]" />
          ) : (
            <Bot className="h-3.5 w-3.5 shrink-0 text-[var(--color-accent)]" />
          )}
          <span className="min-w-0 flex-1 truncate">{reviewStatusLine(review)}</span>
          {!inProgress ? (
            <button
              type="button"
              onClick={dismissStatus}
              aria-label="Dismiss review status"
              className="shrink-0 rounded p-0.5 text-[var(--color-fg-muted)] hover:text-[var(--color-fg)]"
            >
              <X className="h-3 w-3" />
            </button>
          ) : null}
        </div>
      ))}
    </div>
  );
}

// AvatarCommentMarker renders the bot-treated attribution for an avatar
// comment, visually distinct from a human commenter.
export function AvatarCommentMarker({ name }: { name: string }) {
  return (
    <span className="flex min-w-0 items-center gap-1.5 text-xs font-semibold">
      <span className="flex h-4 w-4 shrink-0 items-center justify-center rounded-full bg-[var(--color-accent-muted)]">
        <Bot className="h-3 w-3 text-[var(--color-accent)]" />
      </span>
      <span className="truncate">{name}</span>
      <Badge variant="accent" className="shrink-0">
        AI
      </Badge>
    </span>
  );
}

// Thumbs feedback on avatar comments. One live vote per person; the avatar
// service upserts on (comment, voter) and returns the state to render —
// aidocs stores nothing.
export function AvatarCommentFeedback({ comment }: { comment: Comment }) {
  const ctx = useAvatarReview();
  const { principal } = useDoc();
  const [state, setState] = useState<AvatarFeedbackState | null>(null);
  const vote = useMutation({
    mutationFn: (signal: "up" | "down") =>
      api.submitAvatarFeedback(
        comment.id,
        signal,
        feedbackPersona(ctx?.avatars, comment, principal?.email),
      ),
    onSuccess: setState,
    onError: (e) => toast.error(errorMessage(e, "Could not record feedback")),
  });
  if (!ctx?.enabled || !isAvatarComment(comment)) return null;
  const buttons: Array<{
    signal: "up" | "down";
    icon: typeof ThumbsUp;
    count: number;
    label: string;
  }> = [
    {
      signal: "up",
      icon: ThumbsUp,
      count: state?.up_count ?? 0,
      label: "Helpful",
    },
    {
      signal: "down",
      icon: ThumbsDown,
      count: state?.down_count ?? 0,
      label: "Not helpful",
    },
  ];
  return (
    <div className="flex items-center gap-1">
      {buttons.map(({ signal, icon: Icon, count, label }) => (
        <button
          key={signal}
          type="button"
          aria-label={label}
          aria-pressed={state?.signal === signal}
          disabled={vote.isPending}
          onClick={(e) => {
            e.stopPropagation();
            vote.mutate(signal);
          }}
          className={`flex items-center gap-1 rounded-md px-1.5 py-1 text-xs transition-colors ${
            state?.signal === signal
              ? "bg-[var(--color-accent-muted)] text-[var(--color-accent)]"
              : "text-[var(--color-fg-muted)] hover:bg-[var(--color-surface-muted)] hover:text-[var(--color-fg)]"
          }`}
        >
          <Icon className="h-3.5 w-3.5" />
          {count > 0 ? <span>{count}</span> : null}
        </button>
      ))}
    </div>
  );
}
