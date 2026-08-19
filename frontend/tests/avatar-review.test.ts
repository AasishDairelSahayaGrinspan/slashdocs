import test from "node:test";
import assert from "node:assert/strict";
import {
  countAvatarComments,
  feedbackPersona,
  isTerminalReviewState,
  reviewInProgress,
  reviewStatusLine,
  severityBadgeVariant,
} from "../src/lib/avatar-review.ts";
import type { AvatarInfo, AvatarReview, Comment } from "../src/api.ts";

const review = (state: string, extra: Partial<AvatarReview> = {}): AvatarReview => ({
  review_id: "avrev_1",
  avatar_id: "av_priya",
  display_name: "Priya's avatar",
  state: state as AvatarReview["state"],
  comment_count: 0,
  ...extra,
});

test("isTerminalReviewState", () => {
  assert.equal(isTerminalReviewState("queued"), false);
  assert.equal(isTerminalReviewState("running"), false);
  assert.equal(isTerminalReviewState("commented"), true);
  assert.equal(isTerminalReviewState("no_comments"), true);
  assert.equal(isTerminalReviewState("failed"), true);
});

test("reviewInProgress is per-avatar, true while any avatar is active", () => {
  assert.equal(reviewInProgress([]), false);
  assert.equal(reviewInProgress(undefined), false);
  assert.equal(reviewInProgress([review("queued")]), true);
  assert.equal(reviewInProgress([review("commented"), review("running")]), true);
  assert.equal(reviewInProgress([review("commented"), review("failed")]), false);
});

test("reviewStatusLine names the avatar and distinguishes silent from failed", () => {
  assert.equal(reviewStatusLine(review("queued")), "Priya's avatar is getting ready…");
  assert.equal(reviewStatusLine(review("running")), "Priya's avatar is reading…");
  assert.equal(
    reviewStatusLine(review("commented", { comment_count: 3 })),
    "Priya's avatar found 3 things",
  );
  assert.equal(
    reviewStatusLine(review("commented", { comment_count: 1 })),
    "Priya's avatar found 1 thing",
  );
  const silent = reviewStatusLine(review("no_comments"));
  const failed = reviewStatusLine(review("failed"));
  assert.equal(silent, "Priya's avatar reviewed — no comments");
  assert.equal(failed, "Priya's avatar couldn't finish");
  assert.notEqual(silent, failed);
});

const comment = (extra: Partial<Comment>): Comment => ({
  id: "cmt_1",
  body: "b",
  status: "open",
  author: { type: "service_account", id: "sa_1" },
  created_on_version_id: "ver_1",
  ...extra,
});

test("countAvatarComments counts root avatar comments on the given version", () => {
  const comments: Comment[] = [
    comment({ id: "c1", author_display: "Priya's avatar", severity: "blocking" }),
    comment({
      id: "c2",
      author_display: "Priya's avatar",
      created_on_version_id: "ver_2",
    }),
    comment({ id: "c3" }), // human comment
    comment({ id: "c4", author_display: "Arjun's avatar", parent_comment_id: "c1" }),
  ];
  assert.equal(countAvatarComments(comments, "ver_1"), 1);
  assert.equal(countAvatarComments(comments, "ver_2"), 1);
  assert.equal(countAvatarComments(comments, ""), 0);
});

test("severityBadgeVariant maps severities to badge variants", () => {
  assert.equal(severityBadgeVariant("blocking"), "danger");
  assert.equal(severityBadgeVariant("should-fix"), "warning");
  assert.equal(severityBadgeVariant("consider"), "muted");
  assert.equal(severityBadgeVariant(undefined), null);
  assert.equal(severityBadgeVariant("bogus"), null);
});

test("feedbackPersona is ic only for the person behind the commenting avatar", () => {
  const avatars: AvatarInfo[] = [
    {
      id: "av_priya",
      display_name: "Priya's avatar",
      person_email: "priya@example.com",
      focus_areas: [],
      relevance_hint: "",
    },
  ];
  const avatarComment = comment({ author_display: "Priya's avatar" });
  assert.equal(feedbackPersona(avatars, avatarComment, "priya@example.com"), "ic");
  assert.equal(feedbackPersona(avatars, avatarComment, "other@example.com"), "author");
  assert.equal(
    feedbackPersona(undefined, avatarComment, "priya@example.com"),
    "author",
  );
  assert.equal(feedbackPersona(avatars, avatarComment, undefined), "author");
});
