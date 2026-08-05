// Whitespace trimming for DOM ranges, ported from the Hypothesis client
// (src/annotator/anchoring/trim-range.ts, BSD-2-Clause).

/**
 * From which direction to evaluate strings or nodes: from the start of a
 * string or range seeking forwards, or from the end seeking backwards.
 */
type TrimDirection = "forwards" | "backwards";

/**
 * An object representing metadata for a Range position (e.g. for use with
 * Range.setStart or Range.setEnd)
 */
type RangePosition = {
  offset: number;
  node: Node;
};

/**
 * Return the offset of the nearest non-whitespace character to `baseOffset`
 * within the string `text`, looking in the `direction` indicated. Return -1 if
 * no non-whitespace character exists between `baseOffset` (inclusive) and the
 * terminus of the string (start or end depending on `direction`).
 */
function closestNonSpaceInString(
  text: string,
  baseOffset: number,
  direction: TrimDirection,
): number {
  const nextChar = direction === "forwards" ? baseOffset : baseOffset - 1;
  if (text.charAt(nextChar).trim() !== "") {
    // baseOffset is already valid: it points at a non-whitespace character
    return baseOffset;
  }

  let availableChars: string;
  let availableNonWhitespaceChars: string;

  if (direction === "backwards") {
    availableChars = text.substring(0, baseOffset);
    availableNonWhitespaceChars = availableChars.trimEnd();
  } else {
    availableChars = text.substring(baseOffset);
    availableNonWhitespaceChars = availableChars.trimStart();
  }

  if (!availableNonWhitespaceChars.length) {
    return -1;
  }

  const offsetDelta = availableChars.length - availableNonWhitespaceChars.length;

  return direction === "backwards"
    ? baseOffset - offsetDelta
    : baseOffset + offsetDelta;
}

/**
 * Calculate a new Range start position ("forwards") or end position
 * ("backwards") for `range` that represents the nearest non-whitespace
 * character, moving into the `range` away from the relevant initial boundary
 * node towards the terminating boundary node.
 *
 * @throws {RangeError} If no text node with non-whitespace characters found
 */
function closestNonSpaceInRange(range: Range, direction: TrimDirection): RangePosition {
  const nodeIter = range.commonAncestorContainer.ownerDocument!.createNodeIterator(
    range.commonAncestorContainer,
    NodeFilter.SHOW_TEXT,
  );

  const initialBoundaryNode =
    direction === "forwards" ? range.startContainer : range.endContainer;

  const terminalBoundaryNode =
    direction === "forwards" ? range.endContainer : range.startContainer;

  let currentNode = nodeIter.nextNode();

  // Advance the NodeIterator to the `initialBoundaryNode`
  while (currentNode && currentNode !== initialBoundaryNode) {
    currentNode = nodeIter.nextNode();
  }

  if (direction === "backwards") {
    // Reverse the NodeIterator direction. This will return the same node
    // as the previous `nextNode()` call (initial boundary node).
    currentNode = nodeIter.previousNode();
  }

  let trimmedOffset = -1;

  const advance = () => {
    currentNode =
      direction === "forwards" ? nodeIter.nextNode() : nodeIter.previousNode();

    if (currentNode) {
      const nodeText = currentNode.textContent!;
      const baseOffset = direction === "forwards" ? 0 : nodeText.length;
      trimmedOffset = closestNonSpaceInString(nodeText, baseOffset, direction);
    }
  };

  while (currentNode && trimmedOffset === -1 && currentNode !== terminalBoundaryNode) {
    advance();
  }

  if (currentNode && trimmedOffset >= 0) {
    return { node: currentNode, offset: trimmedOffset };
  }
  throw new RangeError("No text nodes with non-whitespace text found in range");
}

/**
 * Return a new DOM Range that adjusts the start and end positions of `range`
 * as needed such that:
 *
 * - `startContainer` and `endContainer` text nodes both contain at least one
 *   non-whitespace character within the Range's text content
 * - `startOffset` and `endOffset` both reference non-whitespace characters,
 *   with `startOffset` immediately before the first non-whitespace character
 *   and `endOffset` immediately after the last
 *
 * Whitespace characters are those that are removed by `String.prototype.trim()`
 *
 * @param range - A DOM Range whose `startContainer` and `endContainer` are
 *   both text nodes, and which contains at least one non-whitespace character.
 * @throws {RangeError}
 */
export function trimRange(range: Range): Range {
  if (!range.toString().trim().length) {
    throw new RangeError("Range contains no non-whitespace text");
  }
  if (range.startContainer.nodeType !== Node.TEXT_NODE) {
    throw new RangeError("Range startContainer is not a text node");
  }
  if (range.endContainer.nodeType !== Node.TEXT_NODE) {
    throw new RangeError("Range endContainer is not a text node");
  }

  const trimmedRange = range.cloneRange();

  let startTrimmed = false;
  let endTrimmed = false;

  const trimmedOffsets = {
    start: closestNonSpaceInString(
      range.startContainer.textContent!,
      range.startOffset,
      "forwards",
    ),
    end: closestNonSpaceInString(
      range.endContainer.textContent!,
      range.endOffset,
      "backwards",
    ),
  };

  if (trimmedOffsets.start >= 0) {
    trimmedRange.setStart(range.startContainer, trimmedOffsets.start);
    startTrimmed = true;
  }

  // Note: An offset of 0 is invalid for an end offset, as no text in the
  // node would be included in the range.
  if (trimmedOffsets.end > 0) {
    trimmedRange.setEnd(range.endContainer, trimmedOffsets.end);
    endTrimmed = true;
  }

  if (startTrimmed && endTrimmed) {
    return trimmedRange;
  }

  if (!startTrimmed) {
    // There are no (non-whitespace) characters between `startOffset` and the
    // end of the `startContainer` node.
    const { node, offset } = closestNonSpaceInRange(trimmedRange, "forwards");

    if (node && offset >= 0) {
      trimmedRange.setStart(node, offset);
    }
  }

  if (!endTrimmed) {
    // There are no (non-whitespace) characters between the start of the
    // Range's `endContainer` text content and the `endOffset`.
    const { node, offset } = closestNonSpaceInRange(trimmedRange, "backwards");

    if (node && offset > 0) {
      trimmedRange.setEnd(node, offset);
    }
  }

  return trimmedRange;
}
