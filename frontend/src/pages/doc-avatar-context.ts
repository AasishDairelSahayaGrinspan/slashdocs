import { createContext, useContext } from "react";
import type { AvatarInfo, AvatarReviewStatus } from "@/api";

// Avatar-review state shared between the doc bar (button), the comments
// panel (progress + feedback), and the picker dialog. Provided by
// AvatarReviewProvider in doc-avatar.tsx; null outside it or when the server
// has no avatar service configured.
export type AvatarReviewContextValue = {
  enabled: boolean;
  avatars?: AvatarInfo[];
  maxSelectable: number;
  status?: AvatarReviewStatus;
  inProgress: boolean;
  avatarCommentCount: number;
  pickerOpen: boolean;
  setPickerOpen: (open: boolean) => void;
  requestReview: (avatarIds: string[]) => void;
  requesting: boolean;
  dismissStatus: () => void;
};

export const AvatarReviewContext = createContext<AvatarReviewContextValue | null>(null);

export function useAvatarReview(): AvatarReviewContextValue | null {
  return useContext(AvatarReviewContext);
}
