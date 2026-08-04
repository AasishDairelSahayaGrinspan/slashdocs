// Comment anchoring engine for the aidocs render bridge.
//
// Follows the Hypothesis client's anchoring architecture: a selection is
// described as a TextPosition selector (fast, exact) plus a TextQuote
// selector (robust), and resolution tries them in order:
//
//   1. Position: resolve the stored character offsets and verify the text
//      still matches the stored quote.
//   2. Quote: search the whole document text for the best exact or fuzzy
//      match, scored by prefix/suffix context and proximity to the stored
//      position.
//
// All offsets are character offsets into the document body's raw
// `textContent`. Quotes are stored in "rendered" form (each `<br>`
// substituted by a space) so line-broken selections read naturally.
import { matchQuote } from "./match-quote.ts";
import {
  rawOffsetToRendered,
  renderedOffsetToRaw,
  renderedTextFromRange,
  renderedTextOf,
  type RenderedTextWithBrPositions,
} from "./rendered-text.ts";
import { TextRange } from "./text-range.ts";
import type { CommentAnchor } from "./types.ts";

export type { CommentAnchor } from "./types.ts";

/**
 * Number of characters around the quote to capture as prefix/suffix context.
 * Matches the Hypothesis client.
 */
const CONTEXT_LENGTH = 32;

/**
 * Minimum `matchQuote` score for a fuzzy quote-tier match to be accepted.
 *
 * `matchQuote` caps the search at an edit distance of half the quote length,
 * so every candidate scores >= 0.5 on the quote component (weight 50 of 92);
 * this floor additionally requires reasonable prefix/suffix agreement before
 * a degraded match is highlighted, preferring "no highlight" (which the UI
 * surfaces as a badge) over a wrong one. Verbatim matches bypass the floor:
 * an exact occurrence whose stored context was completely rewritten (score
 * ~0.57) must still anchor.
 */
const MIN_ANCHOR_SCORE = 0.6;

/**
 * Collapse runs of whitespace so quote verification tolerates reformatting
 * that doesn't change visible text.
 */
function normalizeSpace(s: string): string {
  return s.replace(/\s+/g, " ").trim();
}

/**
 * Describe `range` as a comment anchor: a quote with surrounding context plus
 * character offsets into `root.textContent`.
 *
 * Returns `null` if the range contains no non-whitespace text.
 */
export function describeAnchor(root: Element, range: Range): CommentAnchor | null {
  let trimmed: Range;
  try {
    trimmed = TextRange.trimmedRange(range);
  } catch {
    return null;
  }

  try {
    const textRange = TextRange.fromRange(trimmed).relativeTo(root);
    const start = textRange.start.offset;
    const end = textRange.end.offset;
    const rawTextLength = (root.textContent ?? "").length;

    const quote = renderedTextFromRange(trimmed);
    if (!quote.trim()) {
      return null;
    }
    const prefixRange = TextRange.fromOffsets(
      root,
      Math.max(0, start - CONTEXT_LENGTH),
      start,
    ).toRange();
    const suffixRange = TextRange.fromOffsets(
      root,
      end,
      Math.min(rawTextLength, end + CONTEXT_LENGTH),
    ).toRange();

    return {
      quote,
      prefix: renderedTextFromRange(prefixRange),
      suffix: renderedTextFromRange(suffixRange),
      dom_path: "body",
      start_offset: start,
      end_offset: end,
    };
  } catch {
    return null;
  }
}

/**
 * Resolve a stored anchor to a DOM Range within `root`, or `null` when the
 * anchor's text can no longer be located with confidence.
 */
export function resolveAnchor(
  root: Element,
  anchor: CommentAnchor,
  getRenderedText?: () => RenderedTextWithBrPositions,
): Range | null {
  // The anchor arrives as JSON from the API; tolerate off-contract field
  // types instead of letting them reach the matcher.
  const quote = typeof anchor.quote === "string" ? anchor.quote : "";
  const prefix = typeof anchor.prefix === "string" ? anchor.prefix : undefined;
  const suffix = typeof anchor.suffix === "string" ? anchor.suffix : undefined;
  const start = anchor.start_offset;
  const end = anchor.end_offset;
  const hasPosition =
    typeof start === "number" && typeof end === "number" && start >= 0 && end > start;

  // Tier 1: character offsets, verified against the stored quote so a stale
  // position can never produce a wrong highlight.
  if (hasPosition) {
    try {
      const range = TextRange.fromOffsets(root, start, end).toRange();
      const rendered = renderedTextFromRange(range);
      if (
        !quote ||
        rendered === quote ||
        normalizeSpace(rendered) === normalizeSpace(quote)
      ) {
        return range;
      }
    } catch {
      // Offsets exceed the document text; fall through to the quote tier.
    }
  }

  if (!quote) {
    return null;
  }

  // Tier 2: exact-or-fuzzy quote search over the whole document text,
  // disambiguated by context and the stored position.
  const { text, brPositionsInText } = getRenderedText
    ? getRenderedText()
    : renderedTextOf(root);
  const match = matchQuote(text, quote, {
    prefix,
    suffix,
    // The stored offset is a raw-textContent position while matchQuote works
    // in rendered coordinates, so convert it (they drift apart by one per
    // preceding <br>). Clamp too: offsets from a longer prior version would
    // push posScore negative and orphan even a perfect quote+context match.
    hint: hasPosition
      ? Math.min(rawOffsetToRendered(brPositionsInText, start), text.length)
      : undefined,
  });
  if (!match) {
    return null;
  }
  // A verbatim occurrence of the quote always anchors: the score floor exists
  // to reject garbled fuzzy matches, not exact text whose surrounding context
  // was rewritten.
  const exact = text.slice(match.start, match.end) === quote;
  if (!exact && match.score < MIN_ANCHOR_SCORE) {
    return null;
  }
  try {
    return TextRange.fromOffsets(
      root,
      renderedOffsetToRaw(brPositionsInText, match.start),
      renderedOffsetToRaw(brPositionsInText, match.end),
    ).toRange();
  } catch {
    return null;
  }
}
