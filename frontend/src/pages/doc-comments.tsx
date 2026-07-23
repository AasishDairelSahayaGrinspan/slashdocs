import { type KeyboardEvent, type ReactNode, useEffect, useRef, useState } from "react";
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
import { api, type Anchor, type Comment, type Principal } from "@/api";
import { formatTimestamp } from "@/lib/format-date";
import { queryKeys } from "@/lib/queryKeys";
import {
  COMMENT_FILTERS,
  COMMENT_FILTER_ALL,
  COMMENT_STATUS,
  DEFAULT_COMMENT_FILTER,
  PLACEMENT_STATUS,
  type CommentFilter,
} from "@/lib/constants";
import { useDoc } from "./doc-context";

type MentionTarget = {
  start: number;
  end: number;
  query: string;
};

const MENTION_BOUNDARY = "\u2063";

function principalLabel(p: Principal) {
  return p.name || p.email || p.id;
}

function getMentionTarget(value: string, caret: number): MentionTarget | null {
  const beforeCaret = value.slice(0, caret);
  const match = /(^|[\s([{])@([A-Za-z0-9._%+-]*)$/.exec(beforeCaret);
  if (!match) return null;
  const start = match.index + match[1].length;
  return { start, end: caret, query: match[2] };
}

function MentionTextarea({
  value,
  onChange,
  onKeyDown,
  placeholder,
  rows,
  className,
  autoFocus,
}: {
  value: string;
  onChange: (value: string) => void;
  onKeyDown?: (event: KeyboardEvent<HTMLTextAreaElement>) => void;
  placeholder?: string;
  rows?: number;
  className?: string;
  autoFocus?: boolean;
}) {
  const ref = useRef<HTMLTextAreaElement>(null);
  const [caret, setCaret] = useState(0);
  const [target, setTarget] = useState<MentionTarget | null>(null);
  const [suggestions, setSuggestions] = useState<Principal[]>([]);
  const [activeIndex, setActiveIndex] = useState(0);
  const [open, setOpen] = useState(false);

  const syncTarget = (el: HTMLTextAreaElement) => {
    const nextCaret = el.selectionStart ?? 0;
    setCaret(nextCaret);
    setTarget(getMentionTarget(el.value, nextCaret));
  };

  useEffect(() => {
    setTarget(getMentionTarget(value, caret));
  }, [value, caret]);

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

  const accept = (user: Principal) => {
    if (!target) return;
    const mention = `@${principalLabel(user)}${MENTION_BOUNDARY} `;
    const nextValue = value.slice(0, target.start) + mention + value.slice(target.end);
    const nextCaret = target.start + mention.length;
    onChange(nextValue);
    setOpen(false);
    setSuggestions([]);
    setCaret(nextCaret);
    window.requestAnimationFrame(() => {
      ref.current?.focus();
      ref.current?.setSelectionRange(nextCaret, nextCaret);
    });
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
        value={value}
        onChange={(event) => {
          onChange(event.target.value);
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
                  {principalLabel(user)}
                </span>
                {user.email && user.email !== principalLabel(user) ? (
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

const MENTION_PATTERN =
  /@(?:[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}|[A-Za-z][A-Za-z0-9._'-]*(?: [A-Z][A-Za-z0-9._'-]*){0,3})/g;
const EXPLICIT_MENTION_PATTERN = new RegExp(
  `@([^${MENTION_BOUNDARY}\\n]+)${MENTION_BOUNDARY}`,
  "g",
);

function mentionNode(mention: string, key: string) {
  return (
    <span
      key={key}
      className="rounded bg-[var(--color-accent-muted)] px-1 py-0.5 font-medium text-[var(--color-accent)]"
    >
      {mention}
    </span>
  );
}

function renderFallbackMentionText(text: string, keyPrefix = ""): ReactNode[] {
  const parts: ReactNode[] = [];
  let lastIndex = 0;
  let match: RegExpExecArray | null;
  while ((match = MENTION_PATTERN.exec(text))) {
    const raw = match[0];
    const trailing = raw.match(/[.,;:!?)]$/)?.[0] || "";
    const mention = trailing ? raw.slice(0, -trailing.length) : raw;
    const start = match.index;
    const end = start + mention.length;
    if (start > lastIndex) parts.push(text.slice(lastIndex, start));
    parts.push(mentionNode(mention, `${keyPrefix}${start}-${mention}`));
    if (trailing) parts.push(trailing);
    lastIndex = start + raw.length;
  }
  const rest = text.slice(lastIndex).replaceAll(MENTION_BOUNDARY, "");
  if (rest) parts.push(rest);
  return parts;
}

function renderMentionText(text: string): ReactNode[] {
  const parts: ReactNode[] = [];
  let lastIndex = 0;
  let match: RegExpExecArray | null;
  while ((match = EXPLICIT_MENTION_PATTERN.exec(text))) {
    if (match.index > lastIndex) {
      parts.push(
        ...renderFallbackMentionText(
          text.slice(lastIndex, match.index),
          `plain-${lastIndex}-`,
        ),
      );
    }
    const mention = `@${match[1].trimEnd()}`;
    parts.push(mentionNode(mention, `explicit-${match.index}-${mention}`));
    lastIndex = match.index + match[0].length;
  }
  if (lastIndex < text.length) {
    parts.push(
      ...renderFallbackMentionText(text.slice(lastIndex), `plain-${lastIndex}-`),
    );
  }
  return parts;
}

function CommentBody({ text }: { text: string }) {
  return (
    <p className="whitespace-pre-wrap break-words text-sm">{renderMentionText(text)}</p>
  );
}

export function Comments() {
  const {
    docId,
    version,
    comments,
    commentsLoading,
    commentsError,
    selection,
    setSelection,
    setActiveComment,
  } = useDoc();
  const q = useQueryClient();
  const [filter, setFilter] = useState<CommentFilter>(DEFAULT_COMMENT_FILTER);
  const [quote, setQuote] = useState("");
  const [body, setBody] = useState("");
  const anchorRef = useRef<Partial<Anchor> | undefined>(undefined);

  useEffect(() => {
    if (selection?.quote) {
      setQuote(selection.quote);
      anchorRef.current = selection.anchor;
    }
  }, [selection]);

  const create = useMutation({
    mutationFn: () => api.createComment(docId, version, body, quote, anchorRef.current),
    onSuccess: () => {
      setBody("");
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
    setBody("");
    anchorRef.current = undefined;
    setSelection(null);
  };

  const items = comments.filter((c) => {
    if (filter === COMMENT_FILTER_ALL) return true;
    if (filter === COMMENT_STATUS.resolved) return c.status === COMMENT_STATUS.resolved;
    return c.status !== COMMENT_STATUS.resolved;
  });

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
        {quote && (
          <form
            className="mb-4 space-y-2 rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface-muted)]/50 p-3"
            onSubmit={(e) => {
              e.preventDefault();
              create.mutate();
            }}
          >
            <div className="border-l-2 border-[var(--color-accent)] pl-2 text-xs italic text-[var(--color-fg-muted)]">
              “{quote.length > 80 ? quote.slice(0, 80) + "…" : quote}”
            </div>
            <MentionTextarea
              autoFocus
              rows={3}
              value={body}
              onChange={setBody}
              onKeyDown={(e) => {
                if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
                  e.preventDefault();
                  if (body.trim() && !create.isPending) create.mutate();
                }
              }}
              placeholder="Add a comment…"
            />
            <div className="flex justify-end gap-2">
              <Button type="button" variant="ghost" size="sm" onClick={clear}>
                Cancel
              </Button>
              <Button size="sm" type="submit" disabled={!body || create.isPending}>
                Comment ⌘↵
              </Button>
            </div>
          </form>
        )}
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

function CommentCard({ c, docId }: { c: Comment; docId: string }) {
  const q = useQueryClient();
  const { activeComment, setActiveComment, principal } = useDoc();
  const cardRef = useRef<HTMLElement>(null);
  const active = activeComment === c.id;
  useEffect(() => {
    if (active)
      cardRef.current?.scrollIntoView({ block: "center", behavior: "smooth" });
  }, [active]);
  const reply = useMutation({
    mutationFn: (body: string) => api.createCommentReply(docId, c.id, body),
    onSuccess: () => q.invalidateQueries({ queryKey: queryKeys.comments(docId) }),
    onError: (e) => toast.error(errorMessage(e, "Could not add reply")),
  });
  const m = useMutation({
    mutationFn: (status: string) => api.patchComment(docId, c.id, c.body, status),
    onSuccess: () => q.invalidateQueries({ queryKey: queryKeys.comments(docId) }),
    onError: (e) => toast.error(errorMessage(e, "Could not update comment")),
  });
  const resolved = c.status === COMMENT_STATUS.resolved;
  return (
    <article
      ref={cardRef}
      className={`group cursor-pointer rounded-[12px] border bg-[var(--color-surface)] p-3 transition-[border-color,box-shadow,opacity] duration-200 ease-out hover:border-[var(--color-border-strong)] hover:shadow-sm ${active ? "border-[var(--color-border-strong)] shadow-sm ring-1 ring-[var(--color-border-strong)]" : "border-[var(--color-border)]"} ${resolved ? "opacity-70" : ""}`}
      onClick={() => setActiveComment(active ? undefined : c.id)}
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
      {(c.created_at || (resolved && c.resolved_at)) && (
        <div className="mb-2 flex flex-wrap gap-x-3 text-[11px] text-[var(--color-fg-muted)]">
          {c.created_at ? <span>Added {formatTimestamp(c.created_at)}</span> : null}
          {resolved && c.resolved_at ? (
            <span>Resolved {formatTimestamp(c.resolved_at)}</span>
          ) : null}
        </div>
      )}
      {c.current_placement?.status &&
        c.current_placement.status !== PLACEMENT_STATUS.attached && (
          <Badge variant="warning" className="mb-2">
            {c.current_placement.status}
          </Badge>
        )}
      {c.selected_text && (
        <blockquote className="mb-2 border-l-2 border-[var(--color-border-strong)] pl-2 text-xs italic text-[var(--color-fg-muted)]">
          {c.selected_text}
        </blockquote>
      )}
      <div className="flex items-start justify-between gap-2">
        <CommentBody text={c.body} />
        <DeleteCommentButton
          c={c}
          docId={docId}
          canDelete={samePrincipal(principal, c.author)}
          className="opacity-0 transition-opacity group-hover:opacity-100"
        />
      </div>
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
      <ReplyComposer onReply={(body) => reply.mutate(body)} pending={reply.isPending} />
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
      {r.created_at ? (
        <div className="mb-1 text-[11px] text-[var(--color-fg-muted)]">
          {formatTimestamp(r.created_at)}
        </div>
      ) : null}
      <CommentBody text={r.body} />
    </div>
  );
}

function ReplyComposer({
  onReply,
  pending,
}: {
  onReply: (body: string) => void;
  pending: boolean;
}) {
  const [body, setBody] = useState("");
  return (
    <form
      className="mt-3 flex gap-2"
      onSubmit={(e) => {
        e.preventDefault();
        if (!body.trim() || pending) return;
        onReply(body.trim());
        setBody("");
      }}
    >
      <MentionTextarea
        rows={1}
        value={body}
        onChange={setBody}
        placeholder="Reply…"
        className="min-h-9 text-sm"
      />
      <Button
        size="sm"
        type="submit"
        disabled={!body.trim() || pending}
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

function samePrincipal(a: Principal | undefined, b: Principal) {
  return !!a && a.type === b.type && a.id === b.id;
}
