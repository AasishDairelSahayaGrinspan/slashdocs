import { createContext, useContext } from "react";
import type { Anchor, Comment, Principal } from "@/api";

// A text selection captured from the render bridge that a new comment will
// anchor to. `anchor` is present only when the bridge supplies real offsets.
export type PendingSelection = {
  quote: string;
  anchor?: Partial<Anchor>;
  top?: number;
};

export type CommentPosition = {
  top: number;
  bottom: number;
};

export type CommentViewport = {
  scrollTop: number;
  scrollHeight: number;
};

export type DocContextValue = {
  docId: string;
  version: string;
  comments: Comment[];
  commentsLoading: boolean;
  commentsError: boolean;
  principal?: Principal;
  selection: PendingSelection | null;
  setSelection: (s: PendingSelection | null) => void;
  activeComment?: string;
  setActiveComment: (id?: string) => void;
  // Ids of comments whose highlight the bridge could not anchor in the
  // currently rendered version; replaced wholesale on every repaint.
  unanchoredComments: ReadonlySet<string>;
  setUnanchoredComments: (ids: ReadonlySet<string>) => void;
  commentPositions: ReadonlyMap<string, CommentPosition>;
  setCommentPositions: (positions: ReadonlyMap<string, CommentPosition>) => void;
  commentViewport: CommentViewport;
  setCommentViewport: (viewport: CommentViewport) => void;
  registerDocumentScroller: (scroller: ((top: number) => void) | null) => void;
  scrollDocumentTo: (top: number) => void;
  registerCommentRailSync: (sync: ((top: number) => void) | null) => void;
  syncCommentRail: (top: number) => void;
  handleDocCommentClick: (id: string) => void;
};

const DocContext = createContext<DocContextValue | null>(null);

export const DocProvider = DocContext.Provider;

export function useDoc(): DocContextValue {
  const v = useContext(DocContext);
  if (!v) throw new Error("useDoc must be used within a DocProvider");
  return v;
}
