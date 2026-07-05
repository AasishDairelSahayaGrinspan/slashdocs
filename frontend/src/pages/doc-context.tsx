import { createContext, useContext } from "react";
import type { Anchor, Comment, Principal } from "@/api";

// A text selection captured from the render bridge that a new comment will
// anchor to. `anchor` is present only when the bridge supplies real offsets.
export type PendingSelection = {
  quote: string;
  anchor?: Partial<Anchor>;
};

export type DocContextValue = {
  docId: string;
  version: string;
  comments: Comment[];
  commentsLoading: boolean;
  commentsError: boolean;
  principal?: Principal;
  htmlEditMode: boolean;
  setHtmlEditMode: (enabled: boolean) => void;
  htmlEditDirty: boolean;
  setHtmlEditDirty: (dirty: boolean) => void;
  htmlEditSaving: boolean;
  setHtmlEditSaving: (saving: boolean) => void;
  htmlEditSaveRequest: number;
  requestHtmlEditSave: () => void;
  handleHtmlVersionSaved: (versionID: string) => void;
  selection: PendingSelection | null;
  setSelection: (s: PendingSelection | null) => void;
  activeComment?: string;
  setActiveComment: (id?: string) => void;
  handleDocCommentClick: (id: string) => void;
};

const DocContext = createContext<DocContextValue | null>(null);

export const DocProvider = DocContext.Provider;

export function useDoc(): DocContextValue {
  const v = useContext(DocContext);
  if (!v) throw new Error("useDoc must be used within a DocProvider");
  return v;
}
