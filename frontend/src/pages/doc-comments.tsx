import { useEffect, useRef, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "react-hot-toast";
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
    <div style={{ display: "flex", height: "100%", flexDirection: "column" }}>
      <div style={{ borderBottom: "1px solid #e4e4e7", padding: "12px 16px" }}>
        <div style={{ marginBottom: "12px", display: "flex", alignItems: "center", justifyContent: "space-between" }}>
          <h2 style={{ fontSize: "14px", fontWeight: 600 }}>Comments</h2>
          <Badge variant="muted">{comments.length}</Badge>
        </div>
        <div style={{ display: "flex", gap: "4px", borderRadius: "6px", background: "#f4f4f5", padding: "2px", fontSize: "12px" }}>
          {COMMENT_FILTERS.map((f) => (
            <button
              key={f}
              onClick={() => setFilter(f)}
              style={{
                flex: 1,
                borderRadius: "4px",
                padding: "4px 8px",
                textTransform: "capitalize",
                transition: "background-color 0.15s",
                ...(filter === f
                  ? { background: "#fff", color: "#18181b", boxShadow: "0 1px 2px 0 rgba(0,0,0,0.05)" }
                  : { color: "#71717a", background: "transparent", border: "none", cursor: "pointer" }),
              }}
            >
              {f}
            </button>
          ))}
        </div>
      </div>
      <div style={{ flex: 1, overflowY: "auto", padding: "16px" }}>
        {quote && (
          <form
            style={{
              marginBottom: "16px",
              display: "flex",
              flexDirection: "column",
              gap: "8px",
              borderRadius: "12px",
              border: "1px solid #e4e4e7",
              background: "rgba(244,244,245,0.5)",
              padding: "12px",
            }}
            onSubmit={(e) => {
              e.preventDefault();
              create.mutate();
            }}
          >
            <div style={{ borderLeft: "2px solid #2028ff", paddingLeft: "8px", fontSize: "12px", fontStyle: "italic", color: "#71717a" }}>
              “{quote.length > 80 ? quote.slice(0, 80) + "…" : quote}”
            </div>
            <Textarea
              autoFocus
              rows={3}
              value={body}
              onChange={(e) => setBody(e.target.value)}
              onKeyDown={(e) => {
                if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
                  e.preventDefault();
                  if (body.trim() && !create.isPending) create.mutate();
                }
              }}
              placeholder="Add a comment…"
            />
            <div style={{ display: "flex", justifyContent: "flex-end", gap: "8px" }}>
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
          <div style={{ display: "flex", flexDirection: "column", gap: "12px" }}>
            <Skeleton style={{ height: "80px", width: "100%" }} />
            <Skeleton style={{ height: "80px", width: "100%" }} />
          </div>
        ) : commentsError ? (
          <Center>Could not load comments.</Center>
        ) : items.length === 0 ? (
          <EmptyState
            title={filter === COMMENT_STATUS.open ? "No open comments" : "Nothing here"}
            description="Select text inside the rendered document to start a thread."
          />
        ) : (
          <div style={{ display: "flex", flexDirection: "column", gap: "12px" }}>
            {items.map((c) => (
              <CommentCard key={c.id} c={c} docId={docId} />
            ))}
          </div>
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
      style={{
        cursor: "pointer",
        borderRadius: "12px",
        border: `1px solid ${active ? "#d4d4d8" : "#e4e4e7"}`,
        background: "#fff",
        padding: "12px",
        transition: "border-color 0.15s",
        ...(active ? { outline: "1px solid #d4d4d8" } : {}),
        ...(resolved ? { opacity: 0.7 } : {}),
      }}
      onClick={() => setActiveComment(active ? undefined : c.id)}
    >
      <div style={{ marginBottom: "4px", display: "flex", alignItems: "center", justifyContent: "space-between" }}>
        <span style={{ fontSize: "12px", fontWeight: 600 }}>
          {c.author.name || c.author.email || c.author.id}
        </span>
        {resolved ? (
          <Badge variant="success">
            <CheckCircle2 style={{ height: "12px", width: "12px" }} /> resolved
          </Badge>
        ) : (
          <Badge variant="accent">open</Badge>
        )}
      </div>
      {c.current_placement?.status &&
        c.current_placement.status !== PLACEMENT_STATUS.attached && (
          <div style={{ marginBottom: "8px" }}>
            <Badge variant="warning">
              {c.current_placement.status}
            </Badge>
          </div>
        )}
      {c.selected_text && (
        <blockquote style={{ marginBottom: "8px", borderLeft: "2px solid #d4d4d8", paddingLeft: "8px", fontSize: "12px", fontStyle: "italic", color: "#71717a" }}>
          {c.selected_text}
        </blockquote>
      )}
      <div style={{ display: "flex", alignItems: "flex-start", justifyContent: "space-between", gap: "8px" }}>
        <p style={{ fontSize: "14px" }}>{c.body}</p>
        <DeleteCommentButton
          c={c}
          docId={docId}
          canDelete={samePrincipal(principal, c.author)}
        />
      </div>
      {(c.replies || []).length > 0 && (
        <div style={{ marginTop: "12px", display: "flex", flexDirection: "column", gap: "8px", borderLeft: "1px solid #e4e4e7", paddingLeft: "12px" }}>
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
      <div style={{ marginTop: "8px", display: "flex", justifyContent: "flex-end" }}>
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
    <div style={{ borderRadius: "6px", background: "rgba(244,244,245,0.5)", padding: "8px" }}>
      <div style={{ marginBottom: "4px", display: "flex", alignItems: "center", justifyContent: "space-between", gap: "8px" }}>
        <span style={{ fontSize: "12px", fontWeight: 600 }}>
          {r.author.name || r.author.email || r.author.id}
        </span>
        <DeleteCommentButton
          c={r}
          docId={docId}
          canDelete={canDelete}
        />
      </div>
      <p style={{ fontSize: "14px" }}>{r.body}</p>
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
      style={{ marginTop: "12px", display: "flex", gap: "8px" }}
      onSubmit={(e) => {
        e.preventDefault();
        if (!body.trim() || pending) return;
        onReply(body.trim());
        setBody("");
      }}
    >
      <Textarea
        rows={1}
        value={body}
        onChange={(e) => setBody(e.target.value)}
        placeholder="Reply…"
      />
      <Button size="sm" type="submit" disabled={!body.trim() || pending}>
        Reply
      </Button>
    </form>
  );
}

function DeleteCommentButton({
  c,
  docId,
  canDelete,
}: {
  c: Comment;
  docId: string;
  canDelete: boolean;
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
          aria-label="Delete comment"
          onClick={(e) => e.stopPropagation()}
        >
          <Trash2 style={{ height: "14px", width: "14px" }} />
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
