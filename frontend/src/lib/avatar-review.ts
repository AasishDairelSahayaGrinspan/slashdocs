import type { AvatarInfo, AvatarReview, AvatarReviewState, Comment } from "@/api";

// Pure avatar-review presentation logic, kept out of components so the Node
// test runner can exercise it directly.

export function isTerminalReviewState(state: AvatarReviewState | string): boolean {
  return state === "commented" || state === "no_comments" || state === "failed";
}

// reviewInProgress is true while ANY avatar is still working. Avatars finish
// independently; callers must render per-avatar lines rather than gate the
// whole panel on the slowest one.
export function reviewInProgress(reviews: AvatarReview[] | undefined): boolean {
  return (reviews || []).some((r) => !isTerminalReviewState(r.state));
}

// reviewStatusLine renders one avatar's named progress line. A silent avatar
// ("reviewed — no comments") and a failed one ("couldn't finish") must never
// look the same.
export function reviewStatusLine(review: AvatarReview): string {
  switch (review.state) {
    case "queued":
      return `${review.display_name} is getting ready…`;
    case "running":
      return `${review.display_name} is reading…`;
    case "commented":
      return `${review.display_name} found ${review.comment_count} ${
        review.comment_count === 1 ? "thing" : "things"
      }`;
    case "no_comments":
      return `${review.display_name} reviewed — no comments`;
    case "failed":
      return `${review.display_name} couldn't finish`;
    default:
      return review.display_name;
  }
}

export function isAvatarComment(c: Comment): boolean {
  return !!c.author_display;
}

// countAvatarComments counts root avatar comments created on the given
// version, for the doc-bar "Avatar · N" badge.
export function countAvatarComments(comments: Comment[], versionId: string): number {
  if (!versionId) return 0;
  return comments.filter(
    (c) =>
      isAvatarComment(c) &&
      !c.parent_comment_id &&
      c.created_on_version_id === versionId,
  ).length;
}

// severityBadgeVariant maps a comment severity onto a Badge variant, or null
// when there is nothing to render.
export function severityBadgeVariant(
  severity: string | undefined,
): "danger" | "warning" | "muted" | null {
  switch (severity) {
    case "blocking":
      return "danger";
    case "should-fix":
      return "warning";
    case "consider":
      return "muted";
    default:
      return null;
  }
}

// feedbackPersona resolves whether the viewer votes as a bystander ("author"
// persona in the avatar-service vocabulary) or as the IC behind the avatar
// ("ic"), by matching the viewer's email against the commenting avatar's
// person. Unknown roster or viewer falls back to the bystander persona; the
// avatar service remains authoritative.
export function feedbackPersona(
  avatars: AvatarInfo[] | undefined,
  comment: Comment,
  viewerEmail: string | undefined,
): "author" | "ic" {
  if (!avatars || !viewerEmail || !comment.author_display) return "author";
  const avatar = avatars.find((a) => a.display_name === comment.author_display);
  if (avatar?.person_email && avatar.person_email === viewerEmail) return "ic";
  return "author";
}
