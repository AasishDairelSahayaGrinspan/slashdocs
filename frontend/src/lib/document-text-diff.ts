export type TextDiffKind = "context" | "added" | "removed";

export type TextDiffLine = {
  kind: TextDiffKind;
  text: string;
  oldLine?: number;
  newLine?: number;
};

export type TextDiffGap = {
  kind: "gap";
  count: number;
};

export type TextDiffRow = TextDiffLine | TextDiffGap;

export type DocumentTextDiff = {
  rows: TextDiffRow[];
  added: number;
  removed: number;
  baseLineCount: number;
  proposedLineCount: number;
};

const BLOCK_ELEMENTS = new Set([
  "ADDRESS",
  "ARTICLE",
  "ASIDE",
  "BLOCKQUOTE",
  "CAPTION",
  "DETAILS",
  "DIV",
  "DL",
  "DT",
  "DD",
  "FIGCAPTION",
  "FIGURE",
  "FOOTER",
  "FORM",
  "H1",
  "H2",
  "H3",
  "H4",
  "H5",
  "H6",
  "HEADER",
  "HR",
  "LI",
  "MAIN",
  "NAV",
  "OL",
  "P",
  "SECTION",
  "SUMMARY",
  "TABLE",
  "TBODY",
  "TFOOT",
  "THEAD",
  "TR",
  "UL",
]);

const SKIPPED_ELEMENTS = new Set([
  "HEAD",
  "SCRIPT",
  "STYLE",
  "TEMPLATE",
  "NOSCRIPT",
  "IFRAME",
  "OBJECT",
]);

function normalizeInlineText(value: string) {
  return value.replace(/[\t\n\r\f ]+/g, " ").replace(/\u00a0/g, " ");
}

// Extracts the human-readable document copy into semantic lines. HTML tags,
// scripts, styles and other implementation details never enter the diff.
export function extractDocumentText(html: string): string[] {
  const parsed = new DOMParser().parseFromString(html, "text/html");
  const lines: string[] = [];
  let current = "";
  let pendingSpace = false;

  const flush = () => {
    const line = current.replace(/\s+/g, " ").trim();
    if (line) lines.push(line);
    current = "";
    pendingSpace = false;
  };
  const append = (value: string) => {
    const text = normalizeInlineText(value);
    if (!text.trim()) {
      if (current) pendingSpace = true;
      return;
    }
    if (current && (pendingSpace || text.startsWith(" "))) current += " ";
    current += text.trim();
    pendingSpace = text.endsWith(" ");
  };

  const visit = (node: Node) => {
    if (node.nodeType === Node.TEXT_NODE) {
      append(node.nodeValue || "");
      return;
    }
    if (node.nodeType !== Node.ELEMENT_NODE) return;

    const element = node as HTMLElement;
    const tag = element.tagName;
    if (
      SKIPPED_ELEMENTS.has(tag) ||
      element.hidden ||
      element.getAttribute("aria-hidden") === "true" ||
      /(?:display\s*:\s*none|visibility\s*:\s*hidden)/i.test(
        element.getAttribute("style") || "",
      )
    ) {
      return;
    }
    if (tag === "BR") {
      flush();
      return;
    }
    if (tag === "PRE") {
      flush();
      for (const line of (element.textContent || "").split(/\r?\n/)) {
        const normalized = line.replace(/\u00a0/g, " ").trimEnd();
        if (normalized.trim()) lines.push(normalized);
      }
      return;
    }

    const block = BLOCK_ELEMENTS.has(tag);
    if (block) flush();
    if (tag === "LI") append("• ");

    let previousWasTableCell = false;
    for (const child of Array.from(element.childNodes)) {
      const childIsTableCell =
        child.nodeType === Node.ELEMENT_NODE &&
        ((child as Element).tagName === "TD" || (child as Element).tagName === "TH");
      if (childIsTableCell && previousWasTableCell) append(" | ");
      visit(child);
      previousWasTableCell = childIsTableCell;
    }
    if (block) flush();
  };

  visit(parsed.body || parsed.documentElement);
  flush();
  return lines;
}

function lineDiff(base: string[], proposed: string[]): TextDiffLine[] {
  const n = base.length;
  const m = proposed.length;

  // A bounded dynamic-programming table gives precise, stable line matches
  // for normal documents. Very large documents use the safe fallback below
  // to avoid an unbounded browser allocation.
  if (n * m > 1_500_000) {
    let prefix = 0;
    while (prefix < n && prefix < m && base[prefix] === proposed[prefix]) prefix++;
    let suffix = 0;
    while (
      suffix < n - prefix &&
      suffix < m - prefix &&
      base[n - suffix - 1] === proposed[m - suffix - 1]
    ) {
      suffix++;
    }
    const result: TextDiffLine[] = [];
    for (let index = 0; index < prefix; index++) {
      result.push({
        kind: "context",
        text: base[index],
        oldLine: index + 1,
        newLine: index + 1,
      });
    }
    for (let index = prefix; index < n - suffix; index++) {
      result.push({ kind: "removed", text: base[index], oldLine: index + 1 });
    }
    for (let index = prefix; index < m - suffix; index++) {
      result.push({ kind: "added", text: proposed[index], newLine: index + 1 });
    }
    for (let offset = suffix; offset > 0; offset--) {
      const oldIndex = n - offset;
      const newIndex = m - offset;
      result.push({
        kind: "context",
        text: base[oldIndex],
        oldLine: oldIndex + 1,
        newLine: newIndex + 1,
      });
    }
    return result;
  }

  const lcs = Array.from({ length: n + 1 }, () => new Uint32Array(m + 1));
  for (let oldIndex = n - 1; oldIndex >= 0; oldIndex--) {
    for (let newIndex = m - 1; newIndex >= 0; newIndex--) {
      lcs[oldIndex][newIndex] =
        base[oldIndex] === proposed[newIndex]
          ? lcs[oldIndex + 1][newIndex + 1] + 1
          : Math.max(lcs[oldIndex + 1][newIndex], lcs[oldIndex][newIndex + 1]);
    }
  }

  const result: TextDiffLine[] = [];
  let oldIndex = 0;
  let newIndex = 0;
  while (oldIndex < n && newIndex < m) {
    if (base[oldIndex] === proposed[newIndex]) {
      result.push({
        kind: "context",
        text: base[oldIndex],
        oldLine: oldIndex + 1,
        newLine: newIndex + 1,
      });
      oldIndex++;
      newIndex++;
    } else if (lcs[oldIndex + 1][newIndex] >= lcs[oldIndex][newIndex + 1]) {
      result.push({
        kind: "removed",
        text: base[oldIndex],
        oldLine: oldIndex + 1,
      });
      oldIndex++;
    } else {
      result.push({
        kind: "added",
        text: proposed[newIndex],
        newLine: newIndex + 1,
      });
      newIndex++;
    }
  }
  while (oldIndex < n) {
    result.push({ kind: "removed", text: base[oldIndex], oldLine: oldIndex + 1 });
    oldIndex++;
  }
  while (newIndex < m) {
    result.push({ kind: "added", text: proposed[newIndex], newLine: newIndex + 1 });
    newIndex++;
  }
  return result;
}

function collapseContext(lines: TextDiffLine[], contextSize = 3): TextDiffRow[] {
  const keep = new Set<number>();
  lines.forEach((line, index) => {
    if (line.kind === "context") return;
    for (
      let nearby = Math.max(0, index - contextSize);
      nearby <= Math.min(lines.length - 1, index + contextSize);
      nearby++
    ) {
      keep.add(nearby);
    }
  });

  const rows: TextDiffRow[] = [];
  let index = 0;
  while (index < lines.length) {
    if (keep.has(index)) {
      rows.push(lines[index]);
      index++;
      continue;
    }
    const start = index;
    while (index < lines.length && !keep.has(index)) index++;
    rows.push({ kind: "gap", count: index - start });
  }
  return rows;
}

export function buildDocumentTextDiff(
  baseHTML: string,
  proposedHTML: string,
): DocumentTextDiff {
  const base = extractDocumentText(baseHTML);
  const proposed = extractDocumentText(proposedHTML);
  const lines = lineDiff(base, proposed);
  const added = lines.filter((line) => line.kind === "added").length;
  const removed = lines.filter((line) => line.kind === "removed").length;
  return {
    rows: added || removed ? collapseContext(lines) : [],
    added,
    removed,
    baseLineCount: base.length,
    proposedLineCount: proposed.length,
  };
}
