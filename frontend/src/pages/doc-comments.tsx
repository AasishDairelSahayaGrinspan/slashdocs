import {
  useCallback,
  type KeyboardEvent,
  type ReactNode,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { motion } from "framer-motion";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { errorMessage } from "@/lib/errors";
import { CheckCircle2, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Center, EmptyState, Skeleton } from "@/components/ui/misc";
import { Textarea } from "@/components/ui/input";
import {
  api,
  REACTION_EMOJI,
  type Anchor,
  type Comment,
  type UserPrincipal,
} from "@/api";
import { cn } from "@/lib/cn";
import { queryKeys } from "@/lib/queryKeys";
import { layoutCommentRail } from "@/lib/comment-rail";
import { samePrincipal } from "@/lib/principals";
import {
  emptyMentionDraft,
  inferTextareaEdit,
  insertMention,
  MAX_COMMENT_MENTIONS,
  mentionPrincipalLabel,
  mentionTextSegments,
  reconcileMentionDraft,
  type MentionDraft,
  type TextSelection,
} from "@/lib/comment-mentions";
import {
  COMMENT_FILTERS,
  COMMENT_FILTER_ALL,
  COMMENT_STATUS,
  DEFAULT_COMMENT_FILTER,
  type CommentFilter,
} from "@/lib/constants";
import { useDoc } from "./doc-context";

type MentionTarget = {
  start: number;
  end: number;
  query: string;
};

function getMentionTarget(value: string, caret: number): MentionTarget | null {
  const beforeCaret = value.slice(0, caret);
  const match = /(^|\p{White_Space}|[([{])@([A-Za-z0-9._%+-]*)$/u.exec(beforeCaret);
  if (!match) return null;
  const start = match.index + match[1].length;
  return { start, end: caret, query: match[2] };
}

function MentionTextarea({
  draft,
  onChange,
  onKeyDown,
  placeholder,
  rows,
  className,
  autoFocus,
}: {
  draft: MentionDraft;
  onChange: (draft: MentionDraft) => void;
  onKeyDown?: (event: KeyboardEvent<HTMLTextAreaElement>) => void;
  placeholder?: string;
  rows?: number;
  className?: string;
  autoFocus?: boolean;
}) {
  const ref = useRef<HTMLTextAreaElement>(null);
  const beforeInputSelection = useRef<TextSelection | null>(null);
  // Caret position to apply right after the commit that renders an accepted
  // mention. A layout effect (not requestAnimationFrame) applies it: rAF can
  // fire after further keystrokes and yank the caret back mid-word.
  const caretRestore = useRef<number | null>(null);
  const [caret, setCaret] = useState(0);
  const [target, setTarget] = useState<MentionTarget | null>(null);
  const [suggestions, setSuggestions] = useState<UserPrincipal[]>([]);
  const [activeIndex, setActiveIndex] = useState(0);
  const [open, setOpen] = useState(false);

  const syncTarget = (el: HTMLTextAreaElement) => {
    const nextCaret = el.selectionStart ?? 0;
    setCaret(nextCaret);
    setTarget(getMentionTarget(el.value, nextCaret));
  };

  useEffect(() => {
    setTarget(getMentionTarget(draft.body, caret));
  }, [draft.body, caret]);

  useEffect(() => {
    const query = target?.query.trim() || "";
    if (query.length < 2) {
      setSuggestions([]);
      setOpen(false);
      return;
    }
    let cancelled = false;
    const timeout = window.setTimeout(async () => {
      try {
        const result = await api.searchUsers(query);
        if (cancelled) return;
        setSuggestions(result.items || []);
        setActiveIndex(0);
        setOpen((result.items || []).length > 0);
      } catch {
        if (!cancelled) {
          setSuggestions([]);
          setOpen(false);
        }
      }
    }, 250);
    return () => {
      cancelled = true;
      window.clearTimeout(timeout);
    };
  }, [target?.query]);

  useLayoutEffect(() => {
    if (caretRestore.current === null) return;
    const el = ref.current;
    if (el) {
      el.focus();
      el.setSelectionRange(caretRestore.current, caretRestore.current);
    }
    caretRestore.current = null;
  });

  const accept = (user: UserPrincipal) => {
    if (!target) return;
    if (draft.mentions.length >= MAX_COMMENT_MENTIONS) {
      toast.error(`A comment can mention up to ${MAX_COMMENT_MENTIONS} people.`);
      setOpen(false);
      return;
    }
    const label = `@${mentionPrincipalLabel(user)}`;
    const nextCaret = target.start + label.length + 1;
    onChange(insertMention(draft, target.start, target.end, user));
    setOpen(false);
    setSuggestions([]);
    setCaret(nextCaret);
    caretRestore.current = nextCaret;
  };

  const handleKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (open && suggestions.length > 0) {
      if (event.key === "ArrowDown") {
        event.preventDefault();
        setActiveIndex((index) => (index + 1) % suggestions.length);
        return;
      }
      if (event.key === "ArrowUp") {
        event.preventDefault();
        setActiveIndex(
          (index) => (index - 1 + suggestions.length) % suggestions.length,
        );
        return;
      }
      if (event.key === "Enter" || event.key === "Tab") {
        event.preventDefault();
        accept(suggestions[activeIndex]);
        return;
      }
      if (event.key === "Escape") {
        event.preventDefault();
        setOpen(false);
        return;
      }
    }
    onKeyDown?.(event);
  };

  return (
    <div className="relative min-w-0 flex-1">
      <Textarea
        ref={ref}
        autoFocus={autoFocus}
        rows={rows}
        value={draft.body}
        onBeforeInput={(event) => {
          beforeInputSelection.current = {
            start: event.currentTarget.selectionStart ?? 0,
            end: event.currentTarget.selectionEnd ?? 0,
          };
        }}
        // React's synthetic onChange may suppress an input event whose final
        // value is unchanged (for example, pasting the same label over a
        // selection). Native onInput still lets that exact replacement clear
        // an overlapping structured mention.
        onInput={(event) => {
          if (event.currentTarget.value !== draft.body) return;
          const before = beforeInputSelection.current;
          beforeInputSelection.current = null;
          if (!before) {
            onChange({ body: event.currentTarget.value, mentions: [] });
            syncTarget(event.currentTarget);
            return;
          }
          const after = {
            start: event.currentTarget.selectionStart ?? 0,
            end: event.currentTarget.selectionEnd ?? 0,
          };
          const edit = inferTextareaEdit(
            draft.body,
            event.currentTarget.value,
            before,
            after,
          );
          onChange(
            edit
              ? reconcileMentionDraft(draft, event.currentTarget.value, edit)
              : { body: event.currentTarget.value, mentions: [] },
          );
          syncTarget(event.currentTarget);
        }}
        onChange={(event) => {
          if (event.target.value === draft.body) return;
          const before = beforeInputSelection.current;
          beforeInputSelection.current = null;
          const after = {
            start: event.target.selectionStart ?? 0,
            end: event.target.selectionEnd ?? 0,
          };
          const edit = before
            ? inferTextareaEdit(draft.body, event.target.value, before, after)
            : null;
          onChange(reconcileMentionDraft(draft, event.target.value, edit));
          syncTarget(event.target);
        }}
        onClick={(event) => syncTarget(event.currentTarget)}
        onBlur={() => window.setTimeout(() => setOpen(false), 100)}
        onKeyUp={(event) => syncTarget(event.currentTarget)}
        onSelect={(event) => syncTarget(event.currentTarget)}
        onKeyDown={handleKeyDown}
        placeholder={placeholder}
        className={className}
      />
      {open && suggestions.length > 0 ? (
        <div className="absolute left-0 right-0 top-full z-20 mt-1 overflow-hidden rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] py-1 shadow-lg">
          {suggestions.map((user, index) => {
            const active = index === activeIndex;
            return (
              <button
                key={user.id || user.email}
                type="button"
                className={`block w-full px-3 py-2 text-left text-sm transition-colors ${active ? "bg-[var(--color-surface-muted)] text-[var(--color-fg)]" : "text-[var(--color-fg)] hover:bg-[var(--color-surface-muted)]"}`}
                onMouseDown={(event) => event.preventDefault()}
                onClick={(event) => {
                  event.stopPropagation();
                  accept(user);
                }}
              >
                <span className="block truncate font-medium">
                  {mentionPrincipalLabel(user)}
                </span>
                {user.email && user.email !== mentionPrincipalLabel(user) ? (
                  <span className="block truncate text-xs text-[var(--color-fg-muted)]">
                    {user.email}
                  </span>
                ) : null}
              </button>
            );
          })}
        </div>
      ) : null}
    </div>
  );
}

function CommentBody({
  text,
  mentions,
}: {
  text: string;
  mentions: Comment["mentions"];
}) {
  return (
    <p className="whitespace-pre-wrap break-words text-sm">
      {mentionTextSegments(text, mentions || []).map((segment, index) =>
        segment.mention ? (
          <span
            key={`${segment.mention.offset}-${segment.mention.principal.type}-${segment.mention.principal.id}`}
            className="rounded bg-[var(--color-accent-muted)] px-1 py-0.5 font-medium text-[var(--color-accent)]"
          >
            {segment.text}
          </span>
        ) : (
          <span key={`text-${index}`}>{segment.text}</span>
        ),
      )}
    </p>
  );
}

// REACTION_GLYPH maps the server's stable slugs to the glyph this client
// shows. The slug is the contract; the glyph is ours to choose.
const REACTION_GLYPH: Record<string, string> = {
  "+1": "👍",
  "-1": "👎",
  laugh: "😄",
  hooray: "🎉",
  confused: "😕",
  heart: "❤️",
  rocket: "🚀",
  eyes: "👀",
};

function ReactionBar({ c, docId }: { c: Comment; docId: string }) {
  const q = useQueryClient();
  const [picking, setPicking] = useState(false);
  const reactions = c.reactions || [];
  const toggle = useMutation({
    mutationFn: ({ emoji, reacted }: { emoji: string; reacted: boolean }) =>
      api.setCommentReaction(docId, c.id, emoji, reacted),
    onSuccess: () => q.invalidateQueries({ queryKey: queryKeys.comments(docId) }),
    onError: (e) => toast.error(errorMessage(e, "Reaction failed")),
  });
  const react = (emoji: string, reacted: boolean) => {
    setPicking(false);
    toggle.mutate({ emoji, reacted });
  };
  return (
    <div className="mt-2 flex flex-wrap items-center gap-1">
      {reactions.map((r) => (
        <button
          key={r.emoji}
          type="button"
          aria-pressed={r.reacted}
          aria-label={`${r.emoji}, ${r.count} ${r.count === 1 ? "reaction" : "reactions"}`}
          onClick={(e) => {
            e.stopPropagation();
            react(r.emoji, !r.reacted);
          }}
          className={cn(
            "inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs transition-colors",
            r.reacted
              ? "border-[var(--color-accent)] bg-[var(--color-accent)]/10 text-[var(--color-accent)]"
              : "border-[var(--color-border)] text-[var(--color-fg-muted)] hover:bg-[var(--color-surface-muted)]",
          )}
        >
          <span aria-hidden="true">{REACTION_GLYPH[r.emoji] || r.emoji}</span>
          <span className="tabular-nums">{r.count}</span>
        </button>
      ))}
      <div className="relative">
        <button
          type="button"
          aria-label="Add reaction"
          aria-expanded={picking}
          onClick={(e) => {
            e.stopPropagation();
            setPicking((open) => !open);
          }}
          className="rounded-full border border-dashed border-[var(--color-border)] px-2 py-0.5 text-xs text-[var(--color-fg-muted)] hover:bg-[var(--color-surface-muted)]"
        >
          +
        </button>
        {picking && (
          <div
            role="menu"
            className="absolute bottom-full left-0 z-10 mb-1 flex gap-0.5 rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] p-1 shadow-md"
          >
            {REACTION_EMOJI.map((emoji) => (
              <button
                key={emoji}
                type="button"
                role="menuitem"
                aria-label={emoji}
                title={emoji}
                onClick={(e) => {
                  e.stopPropagation();
                  react(emoji, !reactions.some((r) => r.emoji === emoji && r.reacted));
                }}
                className="rounded p-1 text-sm hover:bg-[var(--color-surface-muted)]"
              >
                <span aria-hidden="true">{REACTION_GLYPH[emoji]}</span>
              </button>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

export function Comments() {
  const { docId, comments, commentsLoading, commentsError, unanchoredComments } =
    useDoc();
  const [filter, setFilter] = useState<CommentFilter>(DEFAULT_COMMENT_FILTER);

  const items = useMemo(
    () =>
      comments
        .filter((c) => {
          if (filter === COMMENT_FILTER_ALL) return true;
          if (filter === COMMENT_STATUS.resolved)
            return c.status === COMMENT_STATUS.resolved;
          return c.status !== COMMENT_STATUS.resolved;
        })
        .sort(
          (a, b) =>
            Number(unanchoredComments.has(b.id)) - Number(unanchoredComments.has(a.id)),
        ),
    [comments, filter, unanchoredComments],
  );

  return (
    <div className="flex h-full flex-col">
      <div className="border-b border-[var(--color-border)] px-4 py-3">
        <div className="mb-3 flex items-center justify-between pr-10 md:pr-0">
          <h2 className="text-sm font-semibold">Comments</h2>
          <Badge variant="muted">{comments.length}</Badge>
        </div>
        <div className="flex gap-1 rounded-md bg-[var(--color-surface-muted)] p-0.5 text-xs">
          {COMMENT_FILTERS.map((f) => (
            <button
              key={f}
              onClick={() => setFilter(f)}
              className={`flex-1 rounded px-2 py-1 capitalize transition-colors ${filter === f ? "bg-[var(--color-surface)] text-[var(--color-fg)] shadow-sm" : "text-[var(--color-fg-muted)]"}`}
            >
              {f}
            </button>
          ))}
        </div>
      </div>
      <div className="flex-1 overflow-y-auto p-4">
        <CommentComposer className="mb-4" />
        {commentsLoading ? (
          <div className="space-y-3">
            <Skeleton className="h-20 w-full" />
            <Skeleton className="h-20 w-full" />
          </div>
        ) : commentsError ? (
          <Center>Could not load comments.</Center>
        ) : items.length === 0 ? (
          <EmptyState
            title={filter === COMMENT_STATUS.open ? "No open comments" : "Nothing here"}
            description="Select text inside the rendered document to start a thread."
          />
        ) : (
          <motion.div
            className="space-y-3"
            initial="hidden"
            animate="visible"
            variants={{
              hidden: {},
              visible: { transition: { staggerChildren: 0.06 } },
            }}
          >
            {items.map((c) => (
              <motion.div
                key={c.id}
                variants={{
                  hidden: { opacity: 0, x: 12 },
                  visible: {
                    opacity: 1,
                    x: 0,
                    transition: { duration: 0.3, ease: [0.25, 0.4, 0.25, 1] as const },
                  },
                }}
              >
                <CommentCard c={c} docId={docId} />
              </motion.div>
            ))}
          </motion.div>
        )}
      </div>
    </div>
  );
}

function CommentComposer({ className = "" }: { className?: string }) {
  const { docId, version, selection, setSelection } = useDoc();
  const q = useQueryClient();
  const [quote, setQuote] = useState("");
  const [draft, setDraft] = useState<MentionDraft>(emptyMentionDraft);
  const anchorRef = useRef<Partial<Anchor> | undefined>(undefined);

  useEffect(() => {
    if (!selection?.quote) return;
    setQuote(selection.quote);
    anchorRef.current = selection.anchor;
  }, [selection]);

  const create = useMutation({
    mutationFn: () =>
      api.createComment(
        docId,
        version,
        draft.body,
        quote,
        anchorRef.current,
        draft.mentions,
      ),
    onSuccess: () => {
      setDraft(emptyMentionDraft());
      setQuote("");
      anchorRef.current = undefined;
      setSelection(null);
      toast.success("Comment added.");
      q.invalidateQueries({ queryKey: queryKeys.comments(docId) });
    },
    onError: (e) => toast.error(errorMessage(e, "Could not add comment")),
  });

  const clear = () => {
    setQuote("");
    setDraft(emptyMentionDraft());
    anchorRef.current = undefined;
    setSelection(null);
  };

  if (!quote) return null;
  return (
    <form
      className={`space-y-2 rounded-[14px] bg-[var(--color-surface)] p-3 shadow-lg ring-1 ring-[var(--color-border)] ${className}`}
      onSubmit={(event) => {
        event.preventDefault();
        create.mutate();
      }}
    >
      <div className="border-l-2 border-[var(--color-accent)] pl-2 text-xs italic text-[var(--color-fg-muted)]">
        “{quote.length > 80 ? quote.slice(0, 80) + "…" : quote}”
      </div>
      <MentionTextarea
        autoFocus
        rows={3}
        draft={draft}
        onChange={setDraft}
        onKeyDown={(event) => {
          if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
            event.preventDefault();
            if (draft.body.trim() && !create.isPending) create.mutate();
          }
        }}
        placeholder="Add a comment…"
      />
      <div className="flex justify-end gap-2">
        <Button type="button" variant="ghost" size="sm" onClick={clear}>
          Cancel
        </Button>
        <Button
          size="sm"
          type="submit"
          disabled={!draft.body.trim() || create.isPending}
        >
          Comment ⌘↵
        </Button>
      </div>
    </form>
  );
}

const RAIL_CARD_ESTIMATE = 190;
const RAIL_COMPOSER_ESTIMATE = 210;

export function CommentRail() {
  const {
    docId,
    comments,
    commentsLoading,
    commentsError,
    selection,
    unanchoredComments,
    commentPositions,
    commentViewport,
    scrollDocumentTo,
    registerCommentRailSync,
  } = useDoc();
  const railRef = useRef<HTMLDivElement>(null);
  const applyingDocumentScroll = useRef(false);
  const scrollSyncFrame = useRef(0);
  const [railHeight, setRailHeight] = useState(0);
  const [heights, setHeights] = useState<ReadonlyMap<string, number>>(new Map());

  useLayoutEffect(() => {
    const rail = railRef.current;
    if (!rail) return;
    const update = () => setRailHeight(rail.clientHeight);
    update();
    const observer = new ResizeObserver(update);
    observer.observe(rail);
    return () => observer.disconnect();
  }, []);

  const timelineComments = useMemo(
    () =>
      comments.filter((comment) => {
        if (comment.status === COMMENT_STATUS.resolved) return false;
        if (unanchoredComments.has(comment.id)) return true;
        return commentPositions.has(comment.id);
      }),
    [comments, commentPositions, unanchoredComments],
  );

  const layoutItems = useMemo(() => {
    const items = timelineComments.map((comment) => ({
      id: comment.id,
      anchorTop: commentPositions.get(comment.id)?.top ?? 0,
      height: heights.get(comment.id) ?? RAIL_CARD_ESTIMATE,
      atStart: unanchoredComments.has(comment.id),
    }));
    if (selection?.quote) {
      items.push({
        id: "__composer",
        anchorTop: selection.top ?? 0,
        height: heights.get("__composer") ?? RAIL_COMPOSER_ESTIMATE,
        atStart: selection.top === undefined,
      });
    }
    return items;
  }, [commentPositions, heights, selection, timelineComments, unanchoredComments]);

  const timelineHeight = Math.max(commentViewport.scrollHeight, railHeight, 1);

  const placements = useMemo(
    () => layoutCommentRail(layoutItems, timelineHeight),
    [layoutItems, timelineHeight],
  );
  const contentHeight = Math.max(
    timelineHeight,
    ...placements.map((placement) => placement.y + placement.height + 12),
  );
  const commentByID = useMemo(
    () => new Map(timelineComments.map((comment) => [comment.id, comment])),
    [timelineComments],
  );
  const onHeight = useCallback((id: string, height: number) => {
    setHeights((current) => {
      if (Math.abs((current.get(id) || 0) - height) < 1) return current;
      const next = new Map(current);
      next.set(id, height);
      return next;
    });
  }, []);

  const applyDocumentScroll = useCallback((top: number) => {
    const rail = railRef.current;
    if (!rail) return;
    const target = Math.max(0, Math.min(top, rail.scrollHeight - rail.clientHeight));
    if (Math.abs(rail.scrollTop - target) < 0.5) return;
    if (scrollSyncFrame.current) cancelAnimationFrame(scrollSyncFrame.current);
    applyingDocumentScroll.current = true;
    rail.scrollTop = target;
    scrollSyncFrame.current = requestAnimationFrame(() => {
      applyingDocumentScroll.current = false;
      scrollSyncFrame.current = 0;
    });
  }, []);

  useLayoutEffect(() => {
    applyDocumentScroll(commentViewport.scrollTop);
  }, [applyDocumentScroll, commentViewport.scrollTop, contentHeight]);

  useEffect(() => {
    registerCommentRailSync(applyDocumentScroll);
    return () => registerCommentRailSync(null);
  }, [applyDocumentScroll, registerCommentRailSync]);

  useEffect(
    () => () => {
      if (scrollSyncFrame.current) cancelAnimationFrame(scrollSyncFrame.current);
    },
    [],
  );

  return (
    <div
      ref={railRef}
      className="relative h-full overflow-x-hidden overflow-y-auto overscroll-contain"
      onScroll={(event) => {
        if (applyingDocumentScroll.current) return;
        scrollDocumentTo(event.currentTarget.scrollTop);
      }}
    >
      {commentsLoading ? (
        <div className="sticky top-0 space-y-3 px-3 py-3">
          <Skeleton className="h-28 w-full rounded-[14px]" />
          <Skeleton className="h-28 w-full rounded-[14px]" />
        </div>
      ) : commentsError ? (
        <div className="sticky top-0 px-3 py-3">
          <div className="rounded-[14px] bg-[var(--color-surface)] p-3 text-sm text-[var(--color-fg-muted)] shadow-lg ring-1 ring-[var(--color-border)]">
            Could not load comments.
          </div>
        </div>
      ) : (
        <div className="relative min-h-full" style={{ height: contentHeight }}>
          {placements.map((placement) => {
            const comment = commentByID.get(placement.id);
            if (placement.id !== "__composer" && !comment) return null;
            return (
              <MeasuredRailItem
                key={placement.id}
                id={placement.id}
                y={placement.y}
                connected={!placement.atStart}
                onHeight={onHeight}
              >
                {placement.id === "__composer" ? (
                  <CommentComposer />
                ) : (
                  <CommentCard c={comment!} docId={docId} autoReveal={false} />
                )}
              </MeasuredRailItem>
            );
          })}
        </div>
      )}
    </div>
  );
}

function MeasuredRailItem({
  id,
  y,
  connected,
  onHeight,
  children,
}: {
  id: string;
  y: number;
  connected: boolean;
  onHeight: (id: string, height: number) => void;
  children: ReactNode;
}) {
  const ref = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    const element = ref.current;
    if (!element) return;
    const update = () => onHeight(id, element.offsetHeight);
    update();
    const observer = new ResizeObserver(update);
    observer.observe(element);
    return () => observer.disconnect();
  }, [id, onHeight]);
  return (
    <div ref={ref} className="absolute inset-x-3 top-0" style={{ top: y }}>
      {connected ? (
        <span
          aria-hidden="true"
          className="pointer-events-none absolute -left-3 top-5 h-px w-3 bg-[var(--color-border-strong)]"
        />
      ) : null}
      {children}
    </div>
  );
}

function CommentCard({
  c,
  docId,
  autoReveal = true,
}: {
  c: Comment;
  docId: string;
  autoReveal?: boolean;
}) {
  const q = useQueryClient();
  const { activeComment, setActiveComment, principal, unanchoredComments } = useDoc();
  const cardRef = useRef<HTMLElement>(null);
  const active = activeComment === c.id;
  useEffect(() => {
    if (active && autoReveal)
      cardRef.current?.scrollIntoView({ block: "center", behavior: "smooth" });
  }, [active, autoReveal]);
  const reply = useMutation({
    mutationFn: (draft: MentionDraft) =>
      api.createCommentReply(docId, c.id, draft.body, draft.mentions),
    onSuccess: () => q.invalidateQueries({ queryKey: queryKeys.comments(docId) }),
    onError: (e) => toast.error(errorMessage(e, "Could not add reply")),
  });
  const m = useMutation({
    mutationFn: (status: string) => api.patchComment(docId, c.id, { status }),
    onSuccess: () => q.invalidateQueries({ queryKey: queryKeys.comments(docId) }),
    onError: (e) => toast.error(errorMessage(e, "Could not update comment")),
  });
  const resolved = c.status === COMMENT_STATUS.resolved;
  const toggleActive = () => setActiveComment(active ? undefined : c.id);
  return (
    <article
      ref={cardRef}
      role="button"
      tabIndex={0}
      className={`group cursor-pointer rounded-[14px] bg-[var(--color-surface)] p-3 shadow-lg ring-1 transition-[box-shadow,opacity] duration-200 ease-out hover:shadow-xl ${active ? "ring-2 ring-[var(--color-border-strong)]" : "ring-[var(--color-border)]"} ${resolved ? "opacity-70" : ""}`}
      onClick={toggleActive}
      onKeyDown={(e) => {
        // Only toggle when the card itself is focused, not when typing in the
        // reply composer or activating a nested button.
        if (e.target !== e.currentTarget) return;
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          toggleActive();
        }
      }}
    >
      <div className="mb-1 flex items-center justify-between">
        <span className="text-xs font-semibold">
          {c.author.name || c.author.email || c.author.id}
        </span>
        {resolved ? (
          <Badge variant="success">
            <CheckCircle2 className="h-3 w-3" /> resolved
          </Badge>
        ) : (
          <Badge variant="accent">open</Badge>
        )}
      </div>
      {unanchoredComments.has(c.id) && (
        <Badge variant="warning" className="mb-2">
          highlight not found
        </Badge>
      )}
      {c.selected_text && (
        <blockquote className="mb-2 border-l-2 border-[var(--color-border-strong)] pl-2 text-xs italic text-[var(--color-fg-muted)]">
          {c.selected_text}
        </blockquote>
      )}
      <div className="flex items-start justify-between gap-2">
        <CommentBody text={c.body} mentions={c.mentions} />
        <DeleteCommentButton
          c={c}
          docId={docId}
          canDelete={samePrincipal(principal, c.author)}
          className="opacity-0 transition-opacity group-hover:opacity-100"
        />
      </div>
      <ReactionBar c={c} docId={docId} />
      {(c.replies || []).length > 0 && (
        <div className="mt-3 space-y-2 border-l border-[var(--color-border)] pl-3">
          {(c.replies || []).map((r) => (
            <ReplyItem
              key={r.id}
              r={r}
              docId={docId}
              canDelete={samePrincipal(principal, r.author)}
            />
          ))}
        </div>
      )}
      <ReplyComposer
        onReply={async (draft) => {
          await reply.mutateAsync(draft);
        }}
        pending={reply.isPending}
      />
      <div className="mt-2 flex justify-end">
        <Button
          variant="ghost"
          size="sm"
          onClick={(e) => {
            e.stopPropagation();
            m.mutate(resolved ? COMMENT_STATUS.open : COMMENT_STATUS.resolved);
          }}
        >
          {resolved ? "Reopen" : "Resolve"}
        </Button>
      </div>
    </article>
  );
}

function ReplyItem({
  r,
  docId,
  canDelete,
}: {
  r: Comment;
  docId: string;
  canDelete: boolean;
}) {
  return (
    <div className="group/reply rounded-md bg-[var(--color-surface-muted)]/50 p-2">
      <div className="mb-1 flex items-center justify-between gap-2">
        <span className="text-xs font-semibold">
          {r.author.name || r.author.email || r.author.id}
        </span>
        <DeleteCommentButton
          c={r}
          docId={docId}
          canDelete={canDelete}
          className="opacity-0 transition-opacity group-hover/reply:opacity-100"
        />
      </div>
      <CommentBody text={r.body} mentions={r.mentions} />
      <ReactionBar c={r} docId={docId} />
    </div>
  );
}

function ReplyComposer({
  onReply,
  pending,
}: {
  onReply: (draft: MentionDraft) => Promise<void>;
  pending: boolean;
}) {
  const [draft, setDraft] = useState<MentionDraft>(emptyMentionDraft);
  return (
    <form
      className="mt-3 flex gap-2"
      onSubmit={(e) => {
        e.preventDefault();
        if (!draft.body.trim() || pending) return;
        const submittedDraft = draft;
        void onReply(submittedDraft)
          .then(() => {
            setDraft((current) =>
              current === submittedDraft ? emptyMentionDraft() : current,
            );
          })
          // The mutation displays the error; retaining the draft lets the user retry.
          .catch(() => undefined);
      }}
    >
      <MentionTextarea
        rows={1}
        draft={draft}
        onChange={setDraft}
        placeholder="Reply…"
        className="min-h-9 text-sm"
      />
      <Button
        size="sm"
        type="submit"
        disabled={!draft.body.trim() || pending}
        className="shrink-0"
      >
        Reply
      </Button>
    </form>
  );
}

function DeleteCommentButton({
  c,
  docId,
  canDelete,
  className = "",
}: {
  c: Comment;
  docId: string;
  canDelete: boolean;
  className?: string;
}) {
  const q = useQueryClient();
  const del = useMutation({
    mutationFn: () => api.deleteComment(docId, c.id),
    onSuccess: () => {
      toast.success("Deleted.");
      q.invalidateQueries({ queryKey: queryKeys.comments(docId) });
    },
    onError: (e) => toast.error(errorMessage(e, "Could not delete comment")),
  });
  if (!canDelete) return null;
  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          className={`h-7 w-7 shrink-0 ${className}`}
          aria-label="Delete comment"
          onClick={(e) => e.stopPropagation()}
        >
          <Trash2 className="h-3.5 w-3.5" />
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent onClick={(e) => e.stopPropagation()}>
        <AlertDialogTitle>Delete comment?</AlertDialogTitle>
        <AlertDialogDescription>
          This removes the comment permanently. Replies are removed when deleting a
          thread starter.
        </AlertDialogDescription>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction variant="danger" onClick={() => del.mutate()}>
            Delete
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
