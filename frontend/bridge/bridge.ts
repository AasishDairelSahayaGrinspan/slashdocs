import { textLinkAnchor } from "./text-link.ts";
import { renderedTextFromRange } from "./anchoring/rendered-text.ts";
import { snapshotForPrint } from "./document-export.ts";
import type { RenderedTextWithBrPositions } from "./anchoring/rendered-text.ts";
import { renderedTextOf } from "./anchoring/rendered-text.ts";
import { describeAnchor, resolveAnchor } from "./anchoring/index.ts";
import { TextRange } from "./anchoring/text-range.ts";
import type { CommentAnchor } from "./anchoring/types.ts";
import {
  clearHighlights,
  clearHighlightsFor,
  highlightRange,
  refreshSVGRects,
} from "./highlight.ts";
import {
  reduceVimScroll,
  vimLineScrollDistance,
  type VimScrollAction,
} from "../src/lib/vim-scroll.ts";

type KVItems = Record<string, unknown>;

interface KVPatch {
  set: KVItems;
  delete: string[];
}

interface KVHistoryEntry {
  forward: KVPatch;
  inverse: KVPatch;
}

interface KVPendingRequest {
  resolve(value: KVItems): void;
  reject(error: unknown): void;
}

interface GitHubPendingRequest {
  resolve(value: unknown): void;
  reject(error: unknown): void;
}

type KVChangeHandler = (detail: unknown) => void;

interface KVHandlers {
  kvchange: KVChangeHandler[];
}

interface QueuedKVOperation {
  op: string;
  args?: never[];
  resolve?: (value: unknown) => void;
  reject?: (error: unknown) => void;
}

type KVFunction = (...args: never[]) => Promise<unknown>;
type GitHubFunction = (input: {
  owner: string;
  repo: string;
  path: string;
  ref: string;
}) => Promise<unknown>;
type GitHubConnectFunction = () => void;

interface AidocsPublicAPI {
  print?: () => void;
  download?: (input: { blob: Blob; filename: string }) => Promise<void>;
  kv?: Record<string, KVFunction>;
  github?: {
    getRawContent: GitHubFunction;
    connect: GitHubConnectFunction;
  };
  on?: (event: string, handler: KVChangeHandler) => (() => void) | undefined;
  off?: (event: string, handler: KVChangeHandler) => void;
  [key: string]: unknown;
}

declare global {
  interface Window {
    __AIDOCS_APP_ORIGIN__?: string;
    __aidocsKVHandlers?: KVHandlers;
    __aidocsKVQueue?: QueuedKVOperation[];
    __aidocsGitHubQueue?: Array<{
      input: Parameters<GitHubFunction>[0];
      resolve(value: unknown): void;
      reject(error: unknown): void;
    }>;
    __aidocsGitHubConnectQueued?: boolean;
    aidocs?: AidocsPublicAPI;
    CustomEvent: typeof CustomEvent;
  }
}

interface PaintItem {
  id?: string;
  anchor?: CommentAnchor;
  quote?: string;
  selected_text?: string;
}

type Theme = "dark" | "light";

(function bridge(): void {
  const appOrigin = window.__AIDOCS_APP_ORIGIN__ || "*";
  const frame = document.getElementById("aidocs-doc") as HTMLIFrameElement;
  let kvRequestID = 0;
  const kvPending: Record<string, KVPendingRequest> = {};
  let githubRequestID = 0;
  const githubPending: Record<string, GitHubPendingRequest> = {};
  const kvRequestTimeoutMS = 10000;
  const kvHistoryLimit = 100;
  let kvUndoStack: KVHistoryEntry[] = [];
  let kvRedoStack: KVHistoryEntry[] = [];
  let kvMutationQueue: Promise<unknown> = Promise.resolve();
  let kvHistoryGeneration = 0;

  function doc(): Document | null {
    try {
      return frame.contentDocument || frame.contentWindow!.document;
    } catch (e) {
      return null;
    }
  }

  function targetOrigin(): string {
    if (!appOrigin || appOrigin === "*") return "*";
    if (appOrigin === "self" || appOrigin === "'self'") return window.location.origin;
    try {
      return new URL(appOrigin, window.location.href).origin;
    } catch (e) {
      return "*";
    }
  }

  function isParentMessage(e: MessageEvent): boolean {
    if (e.source !== parent) return false;
    const origin = targetOrigin();
    return origin === "*" || e.origin === origin;
  }

  function innerWindow(): Window | null {
    try {
      return frame.contentWindow;
    } catch (e) {
      return null;
    }
  }

  function jsonValue(value: unknown): unknown {
    const text = JSON.stringify(value);
    if (text === undefined) throw new TypeError("KV value must be JSON serializable");
    return JSON.parse(text);
  }

  function emptyKVPatch(): KVPatch {
    return { set: {}, delete: [] };
  }

  function hasOwn(obj: object | null | undefined, key: PropertyKey): boolean {
    return Object.prototype.hasOwnProperty.call(obj || {}, key);
  }

  function hasKVPatchChanges(patch: KVPatch | null | undefined): boolean {
    return (
      Object.keys((patch && patch.set) || {}).length > 0 ||
      !!(patch && patch.delete && patch.delete.length)
    );
  }

  function cloneKVPatch(patch: KVPatch | null | undefined): KVPatch {
    const set: KVItems = {};
    Object.keys((patch && patch.set) || {}).forEach((key) => {
      set[key] = jsonValue(patch!.set[key]);
    });
    return { set, delete: ((patch && patch.delete) || []).map(String) };
  }

  function pushKVStack(stack: KVHistoryEntry[], entry: KVHistoryEntry): void {
    stack.push(entry);
    while (stack.length > kvHistoryLimit) stack.shift();
  }

  function resetKVHistory(): void {
    kvHistoryGeneration++;
    kvUndoStack = [];
    kvRedoStack = [];
    kvMutationQueue = Promise.resolve();
  }

  function kvPatchFromSet(key: unknown, value: unknown): KVPatch {
    if (value === undefined) {
      throw new TypeError("aidocs.kv.set requires a JSON value");
    }
    const patch = emptyKVPatch();
    const k = String(key);
    if (value === null) patch.delete.push(k);
    else patch.set[k] = jsonValue(value);
    return patch;
  }

  function kvPatchFromDelete(key: unknown): KVPatch {
    const patch = emptyKVPatch();
    patch.delete.push(String(key));
    return patch;
  }

  function kvPatchFromObject(object: unknown): KVPatch {
    if (!object || typeof object !== "object" || Array.isArray(object)) {
      throw new TypeError("aidocs.kv.patch requires an object");
    }
    const patch = emptyKVPatch();
    Object.keys(object).forEach((key) => {
      const value = (object as KVItems)[key];
      if (value === null || value === undefined) {
        patch.delete.push(key);
      } else {
        patch.set[key] = jsonValue(value);
      }
    });
    return patch;
  }

  function kvRequestItems(): Promise<KVItems> {
    return kvRequest("all").then((res) =>
      res && res.items && typeof res.items === "object" ? (res.items as KVItems) : {},
    );
  }

  function kvInversePatch(forward: KVPatch, items: KVItems): KVPatch {
    const inverse = emptyKVPatch();
    const keys: Record<string, boolean> = {};
    Object.keys((forward && forward.set) || {}).forEach((key) => {
      keys[key] = true;
    });
    ((forward && forward.delete) || []).forEach((key) => {
      keys[key] = true;
    });
    Object.keys(keys).forEach((key) => {
      if (hasOwn(items, key)) inverse.set[key] = jsonValue(items[key]);
      else inverse.delete.push(key);
    });
    return inverse;
  }

  function sameKVValue(a: unknown, b: unknown): boolean {
    if (Object.is(a, b)) return true;
    if (a === null || b === null) return a === b;
    if (typeof a !== typeof b) return false;
    if (typeof a !== "object") return false;
    const aIsArray = Array.isArray(a);
    const bIsArray = Array.isArray(b);
    if (aIsArray || bIsArray) {
      if (!aIsArray || !bIsArray || a.length !== b.length) return false;
      for (let i = 0; i < a.length; i++) {
        if (!sameKVValue(a[i], b[i])) return false;
      }
      return true;
    }
    const aKeys = Object.keys(a).sort();
    const bKeys = Object.keys(b as object).sort();
    if (aKeys.length !== bKeys.length) return false;
    for (let i = 0; i < aKeys.length; i++) {
      const key = aKeys[i];
      if (key !== bKeys[i] || !sameKVValue((a as KVItems)[key], (b as KVItems)[key]))
        return false;
    }
    return true;
  }

  function kvPatchMatchesItems(patch: KVPatch, items: KVItems): boolean {
    const set = (patch && patch.set) || {};
    for (const key of Object.keys(set)) {
      if (!hasOwn(items, key) || !sameKVValue(items[key], set[key])) return false;
    }
    for (const key of (patch && patch.delete) || []) {
      if (hasOwn(items, key)) return false;
    }
    return true;
  }

  function rejectStaleKVHistory(message: string): Promise<never> {
    resetKVHistory();
    return Promise.reject(new Error(message));
  }

  function rejectResetKVHistory(): Promise<never> {
    return Promise.reject(new Error("KV history was reset"));
  }

  function queueKVMutation(task: () => Promise<unknown>): Promise<unknown> {
    const run = kvMutationQueue.then(task, task);
    kvMutationQueue = run.catch(() => {});
    return run;
  }

  function kvApplyPatch(patch: KVPatch, operation: string): Promise<KVItems> {
    const outgoing = cloneKVPatch(patch);
    return kvRequest("patch", {
      set: outgoing.set,
      delete: outgoing.delete,
      source: "local",
      kvOperation: operation,
    });
  }

  function kvApplyLocalPatch(forward: KVPatch, operation: string): Promise<unknown> {
    const generation = kvHistoryGeneration;
    const patch = cloneKVPatch(forward);
    return queueKVMutation(() => {
      if (generation !== kvHistoryGeneration) return rejectResetKVHistory();
      if (!hasKVPatchChanges(patch)) return kvApplyPatch(patch, operation);
      return kvRequestItems().then((items) => {
        if (generation !== kvHistoryGeneration) return rejectResetKVHistory();
        const inverse = kvInversePatch(patch, items);
        return kvApplyPatch(patch, operation).then((result) => {
          if (generation === kvHistoryGeneration) {
            pushKVStack(kvUndoStack, { forward: patch, inverse });
            kvRedoStack = [];
          }
          return result;
        });
      });
    });
  }

  function kvUndo(): Promise<unknown> {
    const generation = kvHistoryGeneration;
    return queueKVMutation(() => {
      if (generation !== kvHistoryGeneration) return rejectResetKVHistory();
      const entry = kvUndoStack.pop();
      if (!entry)
        return Promise.resolve({ changes: [], empty: true, operation: "undo" });
      return kvRequestItems().then((items) => {
        if (generation !== kvHistoryGeneration) return rejectResetKVHistory();
        if (!kvPatchMatchesItems(entry.forward, items)) {
          return rejectStaleKVHistory("KV undo history is stale");
        }
        return kvApplyPatch(entry.inverse, "undo").then(
          (result) => {
            if (generation === kvHistoryGeneration) pushKVStack(kvRedoStack, entry);
            return result;
          },
          (err) => {
            if (generation === kvHistoryGeneration) kvUndoStack.push(entry);
            throw err;
          },
        );
      });
    });
  }

  function kvRedo(): Promise<unknown> {
    const generation = kvHistoryGeneration;
    return queueKVMutation(() => {
      if (generation !== kvHistoryGeneration) return rejectResetKVHistory();
      const entry = kvRedoStack.pop();
      if (!entry)
        return Promise.resolve({ changes: [], empty: true, operation: "redo" });
      return kvRequestItems().then((items) => {
        if (generation !== kvHistoryGeneration) return rejectResetKVHistory();
        if (!kvPatchMatchesItems(entry.inverse, items)) {
          return rejectStaleKVHistory("KV redo history is stale");
        }
        return kvApplyPatch(entry.forward, "redo").then(
          (result) => {
            if (generation === kvHistoryGeneration) pushKVStack(kvUndoStack, entry);
            return result;
          },
          (err) => {
            if (generation === kvHistoryGeneration) kvRedoStack.push(entry);
            throw err;
          },
        );
      });
    });
  }

  function kvRequest(operation: string, payload?: KVItems): Promise<KVItems> {
    const requestId = "kv_" + ++kvRequestID + "_" + Date.now();
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => {
        delete kvPending[requestId];
        reject(new Error("KV request timed out"));
      }, kvRequestTimeoutMS);
      kvPending[requestId] = {
        resolve(value) {
          clearTimeout(timer);
          resolve(value);
        },
        reject(error) {
          clearTimeout(timer);
          reject(error);
        },
      };
      parent.postMessage(
        Object.assign(
          { type: "aidocs:kv-request", requestId, operation },
          payload || {},
        ),
        targetOrigin(),
      );
    });
  }

  function githubRawContent(input: Parameters<GitHubFunction>[0]): Promise<unknown> {
    if (!input || typeof input !== "object") {
      return Promise.reject(
        new TypeError("aidocs.github.getRawContent requires an input object"),
      );
    }
    const payload = {
      owner: String(input.owner || ""),
      repo: String(input.repo || ""),
      path: String(input.path || ""),
      ref: String(input.ref || ""),
    };
    if (!payload.owner || !payload.repo || !payload.path || !payload.ref) {
      return Promise.reject(new TypeError("owner, repo, path, and ref are required"));
    }
    const requestId = "github_" + ++githubRequestID + "_" + Date.now();
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => {
        delete githubPending[requestId];
        reject(new Error("GitHub request timed out"));
      }, 30000);
      githubPending[requestId] = {
        resolve(value) {
          clearTimeout(timer);
          resolve(value);
        },
        reject(error) {
          clearTimeout(timer);
          reject(error);
        },
      };
      parent.postMessage(
        { type: "aidocs:github-request", requestId, operation: "raw", ...payload },
        targetOrigin(),
      );
    });
  }

  function githubConnect(): void {
    parent.postMessage({ type: "aidocs:github-connect" }, targetOrigin());
  }

  function emitKVChange(detail: unknown): void {
    const w = innerWindow();
    if (!w) return;
    try {
      const ev = new w.CustomEvent("kvchange", { detail });
      w.dispatchEvent(ev);
    } catch (e) {}
    const handlers = (w.__aidocsKVHandlers && w.__aidocsKVHandlers.kvchange) || [];
    handlers.slice().forEach((handler) => {
      try {
        handler(detail);
      } catch (e) {
        setTimeout(() => {
          throw e;
        }, 0);
      }
    });
  }

  let printPreparing = false;
  const exportPending = new Map<string, { resolve: () => void; reject: () => void }>();
  let exportID = 0;
  function requestPrint(): void {
    parent.postMessage({ type: "aidocs:print-request" }, targetOrigin());
  }
  function requestDownload(input: { blob: Blob; filename: string }): Promise<void> {
    return new Promise((resolve, reject) => {
      const requestId = `export-${++exportID}`;
      const timer = setTimeout(() => {
        exportPending.delete(requestId);
        reject(new Error("Download request timed out."));
      }, 5000);
      exportPending.set(requestId, {
        resolve: () => {
          clearTimeout(timer);
          resolve();
        },
        reject: () => {
          clearTimeout(timer);
          reject(
            new Error(
              "Download needs a click, a valid filename, and a file no larger than 20 MB. Please try again.",
            ),
          );
        },
      });
      parent.postMessage(
        {
          type: "aidocs:download-request",
          requestId,
          blob: input.blob,
          filename: input.filename,
        },
        targetOrigin(),
      );
    });
  }

  function installAidocsAPI(): void {
    const w = innerWindow();
    if (!w) return;
    const existing: AidocsPublicAPI =
      w.aidocs && typeof w.aidocs === "object" ? w.aidocs : {};
    const handlers =
      w.__aidocsKVHandlers && w.__aidocsKVHandlers.kvchange
        ? w.__aidocsKVHandlers
        : { kvchange: [] };
    w.__aidocsKVHandlers = handlers;

    function off(event: string, handler: KVChangeHandler): void {
      if (event !== "kvchange") return;
      handlers.kvchange = handlers.kvchange.filter((h) => h !== handler);
    }
    function on(event: string, handler: KVChangeHandler): (() => void) | undefined {
      if (event !== "kvchange" || typeof handler !== "function") return;
      handlers.kvchange.push(handler);
      return function unsubscribe() {
        off(event, handler);
      };
    }
    const kv: Record<string, KVFunction> = {
      get(key: unknown) {
        return kvRequest("get", { key: String(key) }).then((res) => res.value);
      },
      all() {
        return kvRequest("all").then((res) => res.items || {});
      },
      set(key: unknown, value: unknown) {
        return kvApplyLocalPatch(kvPatchFromSet(key, value), "set");
      },
      delete(key: unknown) {
        return kvApplyLocalPatch(kvPatchFromDelete(key), "delete");
      },
      patch(object: unknown) {
        return kvApplyLocalPatch(kvPatchFromObject(object), "patch");
      },
      undo() {
        return kvUndo();
      },
      redo() {
        return kvRedo();
      },
    };
    existing.print = requestPrint;
    existing.download = requestDownload;
    w.print = requestPrint;
    existing.kv = kv;
    existing.github = {
      getRawContent: githubRawContent,
      connect: githubConnect,
    };
    existing.on = on;
    existing.off = off;
    try {
      w.aidocs = existing;
    } catch (e) {}

    const githubQueued = w.__aidocsGitHubQueue || [];
    w.__aidocsGitHubQueue = [];
    githubQueued.forEach((entry) => {
      githubRawContent(entry.input).then(entry.resolve, entry.reject);
    });
    if (w.__aidocsGitHubConnectQueued) {
      w.__aidocsGitHubConnectQueued = false;
      githubConnect();
    }

    const queued = w.__aidocsKVQueue || [];
    w.__aidocsKVQueue = [];
    queued.forEach((entry) => {
      const fn = kv[entry && entry.op];
      if (!fn) {
        entry.reject && entry.reject(new Error("unknown aidocs KV operation"));
        return;
      }
      Promise.resolve()
        .then(() => fn.apply(kv, entry.args || []))
        .then(entry.resolve, entry.reject);
    });
  }

  function hydrateKV(): void {
    kvRequest("hydrate", { source: "initial", operation: "hydrate" })
      .then((res) => {
        emitKVChange(
          Object.assign(
            { source: "initial", operation: "hydrate", changes: [] },
            res || {},
          ),
        );
      })
      .catch(() => {});
  }

  function injectStyles(): void {
    const d = doc();
    if (!d || d.getElementById("aidocs-highlight-style")) return;
    const s = d.createElement("style");
    s.id = "aidocs-highlight-style";
    s.textContent =
      ".aidocs-mark{background:#fef08a!important;color:#111827!important;-webkit-text-fill-color:#111827!important;box-decoration-break:clone;-webkit-box-decoration-break:clone;border-radius:2px;box-shadow:0 0 0 1px rgba(202,138,4,.55);cursor:pointer;transition:background-color .15s,box-shadow .15s}.aidocs-mark-active{background:#fde047!important;box-shadow:0 0 0 2px rgba(202,138,4,.95)}.aidocs-svg-mark-text{fill:#111827!important;-webkit-text-fill-color:#111827!important;cursor:pointer}.aidocs-svg-mark-rect{fill:#fef08a;stroke:#ca8a04;stroke-width:1;rx:3;ry:3;pointer-events:auto;opacity:.72}.aidocs-svg-mark-rect.aidocs-mark-active{fill:#fde047;stroke-width:2;opacity:.84}@media print{.aidocs-mark{background:transparent!important;box-shadow:none!important;-webkit-text-fill-color:inherit!important;color:inherit!important}.aidocs-svg-mark-rect{display:none!important}.aidocs-svg-mark-text{fill:inherit!important;-webkit-text-fill-color:inherit!important}}";
    (d.head || d.documentElement).appendChild(s);
  }

  // The last paint payload, kept so document mutations can re-anchor without
  // a round-trip to the parent (documents that render content with JS after
  // load would otherwise never get their highlights).
  let lastPaint: { items: PaintItem[]; active: string | null } | null = null;
  let mutationObserver: MutationObserver | null = null;
  let observedRoot: Node | null = null;
  let mutationRepaintTimer: number | null = null;
  let mutationRepaintDeadline = 0;
  const mutationObserverOptions: MutationObserverInit = {
    childList: true,
    characterData: true,
    subtree: true,
  };
  // Identity of the last fully painted item list, and its results, so a
  // repaint that only changes the active comment can skip re-anchoring.
  let lastPaintKey: string | null = null;
  let lastPaintResults: { id: string; anchored: boolean }[] | null = null;
  let commentPositionFrame = 0;
  let commentScrollFrame = 0;
  // The active comment whose mark was last revealed (scrolled into view).
  // Reveal fires only when activation changes to a different comment:
  // repaints also arrive for reasons unrelated to activation (a comment
  // created, replied to, resolved or deleted all refetch the list, as does a
  // window refocus), and revealing on those yanked the reader away from
  // where they were to the active mark. Cleared when activation clears, so
  // re-activating the same comment reveals it again — mirroring the
  // edge-triggered card scroll on the sidebar side.
  let revealedActive: string | null = null;
  // Selection restored after a repaint, kept briefly so the resulting
  // selectionchange does not re-emit aidocs:selection (which would re-open
  // the comment composer the user may have dismissed).
  let restoredSelection: Range | null = null;
  let restoredSelectionExpiry = 0;

  function paintKeyOf(list: PaintItem[]): string {
    return JSON.stringify(
      list.map((x) => [x && x.id, x && x.anchor, x && x.quote, x && x.selected_text]),
    );
  }

  // Snapshot the user's live selection as character offsets into
  // body.textContent (unchanged by highlight wrapping) so a repaint can put
  // it back. Without this, unwrapping and re-wrapping the text nodes under
  // the selection makes the browser drop the native selection highlight
  // while the user is still selecting.
  function saveSelection(
    d: Document,
    body: HTMLElement,
  ): { start: number; end: number; backward: boolean } | null {
    const sel = d.getSelection();
    if (!sel || sel.isCollapsed || sel.rangeCount < 1) return null;
    try {
      const range = sel.getRangeAt(0);
      if (!body.contains(range.commonAncestorContainer)) return null;
      const textRange = TextRange.fromRange(range).relativeTo(body);
      const probe = d.createRange();
      probe.setStart(sel.anchorNode!, sel.anchorOffset);
      probe.collapse(true);
      const backward = probe.comparePoint(sel.focusNode!, sel.focusOffset) === -1;
      return { start: textRange.start.offset, end: textRange.end.offset, backward };
    } catch {
      return null;
    }
  }

  function restoreSelection(
    d: Document,
    body: HTMLElement,
    saved: { start: number; end: number; backward: boolean },
  ): void {
    try {
      const range = TextRange.fromOffsets(body, saved.start, saved.end).toRange();
      // Offset resolution is forward-biased: an end offset on a node
      // boundary resolves to (nextNode, 0), which visually stretches the
      // restored selection across the block boundary. Pull it back to the
      // end of the preceding text node.
      if (range.endOffset === 0 && range.endContainer !== range.startContainer) {
        const tw = d.createTreeWalker(body, NodeFilter.SHOW_TEXT);
        tw.currentNode = range.endContainer;
        const prev = tw.previousNode() as Text | null;
        if (prev) range.setEnd(prev, prev.data.length);
      }
      const sel = d.getSelection();
      if (!sel) return;
      if (saved.backward) {
        sel.setBaseAndExtent(
          range.endContainer,
          range.endOffset,
          range.startContainer,
          range.startOffset,
        );
      } else {
        sel.setBaseAndExtent(
          range.startContainer,
          range.startOffset,
          range.endContainer,
          range.endOffset,
        );
      }
      restoredSelection = range;
      restoredSelectionExpiry = Date.now() + 1000;
    } catch {
      // The selected text no longer exists after a document mutation.
    }
  }

  // Repaint with the same comments but a different active id: move the
  // active class instead of re-anchoring, leaving the DOM structure (and
  // any live selection) untouched.
  function applyActiveOnly(d: Document, active: string | null, scroll: boolean): void {
    let scrollTarget: Element | null = null;
    d.querySelectorAll(
      "mark.aidocs-mark,.aidocs-svg-mark-rect,.aidocs-svg-mark-text",
    ).forEach((el) => {
      const isActive = !!active && (el as HTMLElement).dataset.cid === active;
      el.classList.toggle("aidocs-mark-active", isActive);
      if (isActive && !scrollTarget) scrollTarget = el;
    });
    if (scroll && scrollTarget) {
      const reducedMotion =
        d.defaultView?.matchMedia("(prefers-reduced-motion: reduce)").matches === true;
      (scrollTarget as Element).scrollIntoView({
        block: "center",
        behavior: reducedMotion ? "instant" : "smooth",
      });
    }
  }

  // Whitespace-insensitive text key: mark text omits synthesized <br> spaces
  // and reformatting must not read as a content change.
  function strippedText(s: string): string {
    return s.replace(/\s+/g, "");
  }

  // Painted quote (whitespace-stripped) per comment id, used to detect
  // whether a highlight survived a document mutation intact.
  const paintedQuotes = new Map<string, string>();

  function postCommentScroll(): void {
    const d = doc();
    if (!d) return;
    const view = d.defaultView;
    parent.postMessage(
      {
        type: "aidocs:comment-scroll",
        scrollTop: view?.scrollY || d.documentElement.scrollTop || 0,
        scrollHeight: Math.max(
          d.documentElement.scrollHeight,
          d.body?.scrollHeight || 0,
        ),
        viewportHeight: view?.innerHeight || d.documentElement.clientHeight || 1,
      },
      targetOrigin(),
    );
  }

  function scheduleCommentScroll(): void {
    const view = doc()?.defaultView;
    if (!view || commentScrollFrame) return;
    commentScrollFrame = view.requestAnimationFrame(() => {
      commentScrollFrame = 0;
      postCommentScroll();
    });
  }

  const commentBlockSelector =
    "p,li,blockquote,pre,td,th,dd,dt,figcaption,h1,h2,h3,h4,h5,h6";

  function postCommentPositions(): void {
    const d = doc();
    if (!d || !lastPaint) return;
    const view = d.defaultView;
    const scrollTop = view?.scrollY || d.documentElement.scrollTop || 0;
    const scrollHeight = Math.max(
      d.documentElement.scrollHeight,
      d.body?.scrollHeight || 0,
    );
    const viewportHeight = view?.innerHeight || d.documentElement.clientHeight || 1;
    const positions: Array<{
      id: string;
      top: number;
      bottom: number;
      left: number;
      blockTop: number;
      blockBottom: number;
    }> = [];
    const seen = new Set<string>();
    for (const item of lastPaint.items) {
      const id = item && item.id;
      if (!id || seen.has(id)) continue;
      seen.add(id);
      const escaped = CSS.escape(id);
      const marks = Array.from(
        d.querySelectorAll(
          `mark.aidocs-mark[data-cid="${escaped}"],.aidocs-svg-mark-text[data-cid="${escaped}"],.aidocs-svg-mark-rect[data-cid="${escaped}"]`,
        ),
      );
      let top = Number.POSITIVE_INFINITY;
      let bottom = Number.NEGATIVE_INFINITY;
      let left = Number.POSITIVE_INFINITY;
      let blockTop = Number.POSITIVE_INFINITY;
      let blockBottom = Number.NEGATIVE_INFINITY;
      const blocks = new Set<Element>();
      for (const mark of marks) {
        const rect = mark.getBoundingClientRect();
        if (!Number.isFinite(rect.top) || !Number.isFinite(rect.bottom)) continue;
        // A mark can wrap across lines. Its bounding rect uses the smallest
        // left edge from every line, which does not represent where the text
        // begins. Use the first client rect for reading order while retaining
        // the full bounding rect for visibility and block measurements.
        const firstRect = Array.from(mark.getClientRects()).find(
          (clientRect) => clientRect.width > 0 || clientRect.height > 0,
        );
        const orderTop = firstRect?.top ?? rect.top;
        const orderLeft = firstRect?.left ?? rect.left;
        if (orderTop < top - 0.5) {
          top = orderTop;
          left = orderLeft;
        } else if (Math.abs(orderTop - top) <= 0.5) {
          left = Math.min(left, orderLeft);
        }
        bottom = Math.max(bottom, rect.bottom);
        const block = mark.closest(commentBlockSelector);
        if (block && !blocks.has(block)) {
          blocks.add(block);
          const blockRect = block.getBoundingClientRect();
          if (Number.isFinite(blockRect.top) && Number.isFinite(blockRect.bottom)) {
            blockTop = Math.min(blockTop, blockRect.top);
            blockBottom = Math.max(blockBottom, blockRect.bottom);
          }
        }
      }
      if (Number.isFinite(top) && Number.isFinite(bottom)) {
        positions.push({
          id,
          top: top + scrollTop,
          bottom: bottom + scrollTop,
          left,
          blockTop: (Number.isFinite(blockTop) ? blockTop : top) + scrollTop,
          blockBottom:
            (Number.isFinite(blockBottom) ? blockBottom : bottom) + scrollTop,
        });
      }
    }
    parent.postMessage(
      {
        type: "aidocs:comment-positions",
        positions,
        scrollTop,
        scrollHeight,
        viewportHeight,
      },
      targetOrigin(),
    );
  }

  function scheduleCommentPositions(): void {
    const view = doc()?.defaultView;
    if (!view || commentPositionFrame) return;
    commentPositionFrame = view.requestAnimationFrame(() => {
      commentPositionFrame = 0;
      const current = doc();
      if (!current) return;
      refreshSVGRects(current);
      postCommentPositions();
    });
  }

  function highlightIntact(d: Document, id: string): boolean {
    const expected = paintedQuotes.get(id);
    if (!expected) return false;
    const cid = CSS.escape(id);
    let text = "";
    d.querySelectorAll('mark.aidocs-mark[data-cid="' + cid + '"]').forEach((m) => {
      text += m.textContent || "";
    });
    d.querySelectorAll('.aidocs-svg-mark-text[data-cid="' + cid + '"]').forEach(
      (el) => {
        text += el.textContent || "";
      },
    );
    if (!text) return false;
    // Containment, not equality: SVG artifacts cover whole elements, which
    // are supersets of the quote.
    return strippedText(text).indexOf(expected) >= 0;
  }
  // True while a selection gesture (mouse drag or touch) is in progress.
  // Repairs must wait: replacing text nodes or programmatically setting the
  // selection mid-drag makes the browser stop rendering the native
  // selection highlight until the user stops moving.
  let selectionGestureActive = false;

  // Debounce mutation-triggered scans (trailing 250ms, forced within 1s) so
  // a burst of DOM writes is handled once, and a document that mutates
  // continuously is scanned at a bounded rate.
  function scheduleMutationScan(): void {
    if (!lastPaint || !lastPaint.items.length) return;
    const now = Date.now();
    if (!mutationRepaintDeadline) mutationRepaintDeadline = now + 1000;
    clearTimeout(mutationRepaintTimer ?? undefined);
    mutationRepaintTimer = setTimeout(
      () => {
        mutationRepaintTimer = null;
        mutationRepaintDeadline = 0;
        if (selectionGestureActive) {
          // Try again after the gesture settles; the reschedule keeps the
          // 250ms cadence without doing any DOM work mid-drag.
          scheduleMutationScan();
          return;
        }
        repairAfterMutations();
      },
      Math.max(0, Math.min(now + 250, mutationRepaintDeadline) - now),
    );
  }

  // Repair only what a mutation actually affected. Highlights are DOM nodes
  // and move with the document, so an anchored comment whose marks are still
  // present with their text intact needs no work at all — an unrelated DOM
  // change (opening a dropdown, an animation tick) touches nothing. Only
  // comments whose highlighted text was removed or edited, and comments
  // still waiting for their text to appear, are re-anchored.
  function repairAfterMutations(): void {
    const d = doc();
    const body = d && d.body;
    if (!d || !body || !lastPaint || !lastPaint.items.length || !lastPaintResults) {
      return;
    }
    refreshSVGRects(d);
    const resultByID = new Map(lastPaintResults.map((r) => [r.id, r]));
    const dirty: PaintItem[] = [];
    for (const x of lastPaint.items) {
      if (!x || !x.id) continue;
      const res = resultByID.get(x.id);
      if (!res) continue;
      // Unanchored comments retry (their text may just have been rendered);
      // anchored comments only repaint when their highlight was damaged.
      if (!res.anchored || !highlightIntact(d, x.id)) dirty.push(x);
    }
    if (!dirty.length) return;

    suspendMutationObserver();
    const savedSelection = saveSelection(d, body);
    let mutatedDOM = false;
    try {
      injectStyles();
      let rendered: RenderedTextWithBrPositions | undefined;
      const getRenderedText = () => (rendered ??= renderedTextOf(body));
      for (const x of dirty) {
        let painted: boolean;
        try {
          if (clearHighlightsFor(d, x.id!) > 0) mutatedDOM = true;
          const anchor: CommentAnchor = Object.assign({}, x.anchor);
          if (!anchor.quote) anchor.quote = x.quote || x.selected_text || "";
          paintedQuotes.set(x.id!, strippedText(anchor.quote));
          const r = resolveAnchor(body, anchor, getRenderedText);
          painted =
            !!r && highlightRange(r, x.id === lastPaint.active, x.id, false) > 0;
          if (painted) mutatedDOM = true;
        } catch {
          painted = false;
        }
        const res = resultByID.get(x.id!);
        if (res) res.anchored = painted;
      }
      // Only put the selection back when this repair actually touched the
      // DOM; a fruitless retry must not churn the user's selection.
      if (savedSelection && mutatedDOM) restoreSelection(d, body, savedSelection);
    } catch {
      // Never let one exotic document abort the repair loop.
    } finally {
      resumeMutationObserver();
    }
    parent.postMessage(
      { type: "aidocs:paint-result", results: lastPaintResults },
      targetOrigin(),
    );
    scheduleCommentPositions();
  }

  // Our own painting mutates the document (splitText, mark wrapping, svg
  // rects, normalize); suspend observation around it so only the document's
  // own changes trigger re-anchoring. Pending external records are drained
  // first — disconnect() would silently discard them.
  function suspendMutationObserver(): void {
    if (!mutationObserver) return;
    if (mutationObserver.takeRecords().length) scheduleMutationScan();
    mutationObserver.disconnect();
  }

  function resumeMutationObserver(): void {
    if (mutationObserver && observedRoot) {
      mutationObserver.observe(observedRoot, mutationObserverOptions);
    }
  }

  function observeMutations(d: Document): void {
    if (mutationObserver) mutationObserver.disconnect();
    mutationObserver = new MutationObserver(() => {
      prepareHeadings(d);
      scheduleTextNavigation();
      scheduleMutationScan();
      scheduleCommentPositions();
    });
    // Observe the root element rather than body so a script replacing
    // document.body entirely is still seen.
    observedRoot = d.documentElement;
    mutationObserver.observe(observedRoot, mutationObserverOptions);
  }

  function paint(
    items: PaintItem[] | null | undefined,
    active: string | null | undefined,
    revealActive = true,
  ): void {
    const d = doc();
    const list = items || [];
    const activeID = typeof active === "string" ? active : null;
    lastPaint = { items: list, active: activeID };
    const key = paintKeyOf(list);
    const body = d && d.body;

    // Repaint with the same comments (sidebar clicks changing the active
    // id): move the active class without touching DOM structure. Any damage
    // from document mutations is healed by repairAfterMutations.
    const reveal = revealActive && activeID !== null && activeID !== revealedActive;
    if (d && body && lastPaintResults && key === lastPaintKey) {
      revealedActive = activeID;
      applyActiveOnly(d, activeID, reveal);
      parent.postMessage(
        { type: "aidocs:paint-result", results: lastPaintResults },
        targetOrigin(),
      );
      scheduleCommentPositions();
      return;
    }

    const results: { id: string; anchored: boolean }[] = [];
    if (d && body) {
      revealedActive = activeID;
      suspendMutationObserver();
      const savedSelection = saveSelection(d, body);
      try {
        clearHighlights(d);
        paintedQuotes.clear();
        injectStyles();
        // One rendered-text snapshot serves every comment in this paint: marks
        // add no text, so wrapping earlier comments never invalidates it.
        let rendered: RenderedTextWithBrPositions | undefined;
        const getRenderedText = () => (rendered ??= renderedTextOf(body));
        list.forEach((x) => {
          let painted: boolean;
          // A single off-contract item must not abort the whole paint: catch
          // per comment and report it unanchored instead.
          try {
            const anchor: CommentAnchor = Object.assign({}, x && x.anchor);
            if (!anchor.quote) anchor.quote = (x && (x.quote || x.selected_text)) || "";
            if (x && x.id) paintedQuotes.set(x.id, strippedText(anchor.quote || ""));
            // Resolve immediately before painting each comment: wrapping a
            // previous comment's marks moves text nodes, which corrupts other
            // already-resolved live Ranges (overlapping comments would paint
            // wrong extents), while character offsets into textContent stay
            // valid across wraps and splits.
            const r = resolveAnchor(body, anchor, getRenderedText);
            painted = !!r && highlightRange(r, x.id === activeID, x.id, reveal) > 0;
          } catch {
            painted = false;
          }
          if (x && x.id) results.push({ id: x.id, anchored: painted });
        });
        if (savedSelection) restoreSelection(d, body, savedSelection);
      } catch {
        // Even a document with exotic DOM state (throwing accessors, broken
        // replaceWith) must not prevent the paint-result post below.
      } finally {
        resumeMutationObserver();
      }
      lastPaintKey = key;
      lastPaintResults = results;
    }
    // Post even when the inner document is unavailable so the parent never
    // keeps stale "highlight not found" state.
    parent.postMessage({ type: "aidocs:paint-result", results }, targetOrigin());
    scheduleCommentPositions();
  }

  function activate(e: MouseEvent): void {
    const m = (e.target &&
      (e.target as Element).closest &&
      (e.target as Element).closest(
        "mark.aidocs-mark,.aidocs-svg-mark-text,.aidocs-svg-mark-rect",
      )) as HTMLElement | null;
    if (!m || !m.dataset.cid) return;
    parent.postMessage({ type: "aidocs:activate", id: m.dataset.cid }, targetOrigin());
  }

  function cssString(s: unknown): string {
    return String(s || "")
      .replace(/\\/g, "\\\\")
      .replace(/"/g, '\\"');
  }

  function fragmentTarget(d: Document, hash: string): Element | null {
    if (!hash || hash === "#") return null;
    let id;
    try {
      id = decodeURIComponent(hash.slice(1));
    } catch (e) {
      id = hash.slice(1);
    }
    return d.getElementById(id) || d.querySelector('[name="' + cssString(id) + '"]');
  }

  function openURL(url: string): void {
    try {
      const w = window.open(url, "_blank", "noopener,noreferrer");
      if (w) w.opener = null;
      if (!w) {
        const a = document.createElement("a");
        a.href = url;
        a.target = "_blank";
        a.rel = "noopener noreferrer";
        document.body.appendChild(a);
        a.click();
        a.remove();
      }
    } catch (e) {}
  }

  function handleLinkClick(e: MouseEvent): void {
    const d = doc();
    if (!d || !e.target || !(e.target as Element).closest) return;
    const a = (e.target as Element).closest("a[href]") as HTMLAnchorElement | null;
    if (!a) return;
    const href = a.getAttribute("href") || "";
    if (!href || /^javascript:/i.test(href)) {
      e.preventDefault();
      return;
    }
    if (href[0] === "#") {
      const t = fragmentTarget(d, href);
      if (t) {
        e.preventDefault();
        t.scrollIntoView({ block: "start", behavior: "smooth" });
        try {
          d.defaultView!.history.replaceState(null, "", href);
        } catch (err) {}
        parent.postMessage(
          { type: "aidocs:heading", id: t.id || href.slice(1) },
          targetOrigin(),
        );
      }
      return;
    }
    let u;
    try {
      u = new URL(
        href,
        d.location && d.location.href ? d.location.href : location.href,
      );
    } catch (err) {
      return;
    }
    if (u.protocol === "http:" || u.protocol === "https:" || u.protocol === "mailto:") {
      e.preventDefault();
      openURL(u.href);
    }
  }

  function prepareLinks(): void {
    const d = doc();
    if (!d) return;
    d.querySelectorAll<HTMLAnchorElement>("a[href]").forEach((a) => {
      const href = a.getAttribute("href") || "";
      if (href && !/^#|javascript:/i.test(href)) {
        a.setAttribute("target", "_blank");
        a.setAttribute("rel", "noopener noreferrer");
      }
    });
  }

  let curTheme: Theme | null = null;
  function initialTheme(): Theme | null {
    try {
      const t = new URLSearchParams(location.search).get("aidocs_theme");
      return t === "dark" || t === "light" ? t : null;
    } catch (e) {
      return null;
    }
  }

  function applyTheme(t: unknown): void {
    if (t !== "dark" && t !== "light") return;
    curTheme = t;
    const d = doc();
    if (!d || !d.documentElement) return;
    d.documentElement.setAttribute("data-aidocs-theme", t);
    try {
      d.defaultView!.dispatchEvent(
        new CustomEvent("aidocs:theme", { detail: { theme: t } }),
      );
    } catch (e) {}
  }

  // Return the selection's range, merging multiple ranges (a Firefox quirk:
  // https://bugzilla.mozilla.org/show_bug.cgi?id=1773065) into their union.
  function selectedRange(s: Selection | null): Range | null {
    if (!s || s.isCollapsed || s.rangeCount < 1) return null;
    let range = s.getRangeAt(0);
    for (let i = 1; i < s.rangeCount; i++) {
      const other = s.getRangeAt(i);
      const merged = range.cloneRange();
      if (other.compareBoundaryPoints(Range.START_TO_START, range) < 0) {
        merged.setStart(other.startContainer, other.startOffset);
      }
      if (other.compareBoundaryPoints(Range.END_TO_END, range) > 0) {
        merged.setEnd(other.endContainer, other.endOffset);
      }
      range = merged;
    }
    return range.collapsed ? null : range;
  }

  function labelIcon(button: HTMLButtonElement, label: string): void {
    button.dataset.aidocsTooltip = label;
    const align = () => {
      const rect = button.getBoundingClientRect();
      const width = button.ownerDocument.defaultView!.innerWidth;
      button.style.setProperty("--aidocs-tip-enter-y", rect.top < 48 ? "-3px" : "3px");
      button.style.setProperty(
        "--aidocs-tip-top",
        rect.top < 48 ? "calc(100% + 6px)" : "auto",
      );
      button.style.setProperty(
        "--aidocs-tip-bottom",
        rect.top < 48 ? "auto" : "calc(100% + 6px)",
      );
      button.style.setProperty(
        "--aidocs-tip-left",
        rect.left < 120 ? "0%" : rect.right > width - 120 ? "100%" : "50%",
      );
      button.style.setProperty(
        "--aidocs-tip-shift",
        rect.left < 120 ? "0%" : rect.right > width - 120 ? "-100%" : "-50%",
      );
    };
    button.addEventListener("pointerenter", align);
    button.addEventListener("focus", align);
    const d = button.ownerDocument;
    if (!d.getElementById("aidocs-icon-tooltip-style")) {
      const style = d.createElement("style");
      style.id = "aidocs-icon-tooltip-style";
      style.textContent = `
        [data-aidocs-tooltip]::after{all:initial!important;content:attr(data-aidocs-tooltip)!important;position:absolute!important;z-index:2147483647!important;top:var(--aidocs-tip-top,auto)!important;bottom:var(--aidocs-tip-bottom,calc(100% + 6px))!important;left:var(--aidocs-tip-left,50%)!important;transform:translate(var(--aidocs-tip-shift,-50%),var(--aidocs-tip-enter-y,3px))!important;width:max-content!important;max-width:min(220px,calc(100vw - 16px))!important;box-sizing:border-box!important;padding:6px 10px!important;border-radius:6px!important;background:#18181b!important;color:#fff!important;font:500 12px/1.4 system-ui,sans-serif!important;white-space:normal!important;text-align:center!important;box-shadow:0 2px 8px #0003!important;pointer-events:none!important;visibility:hidden!important;opacity:0!important;transition:opacity .1s ease-out,transform .14s ease-out,visibility 0s linear .1s!important}
        [data-aidocs-tooltip]:hover::after,[data-aidocs-tooltip]:focus-visible::after{visibility:visible!important;opacity:1!important;transform:translate(var(--aidocs-tip-shift,-50%),0)!important;transition:opacity .14s ease-out .5s,transform .14s ease-out .5s,visibility 0s linear .5s!important}
        [data-aidocs-tooltip]:focus-visible::after{transition-delay:0s!important}
        @media(prefers-reduced-motion:reduce){[data-aidocs-tooltip]::after{transition-duration:0s!important;transform:translateX(var(--aidocs-tip-shift,-50%))!important}}
        @media print{[data-aidocs-tooltip]::after{display:none!important}}`;
      d.head.appendChild(style);
    }
  }

  let selectionAction: HTMLDivElement | null = null;
  let actionRange: Range | null = null;
  let selectionDismissed = false;

  function dismissSelectionAction(): void {
    selectionAction?.remove();
    selectionAction = null;
    selectionDismissed = true;
  }

  function commitSelection(range: Range): void {
    const d = doc();
    if (!d?.body) return;
    const anchor = describeAnchor(d.body, range);
    if (!anchor) return;
    const rect = range.getBoundingClientRect();
    const scrollTop = d.defaultView?.scrollY || d.documentElement.scrollTop || 0;
    dismissSelectionAction();
    parent.postMessage(
      { type: "aidocs:selection", anchor, top: rect.top + scrollTop },
      targetOrigin(),
    );
  }

  function selection(): void {
    const d = doc();
    if (!d || !d.body) return;
    const range = selectedRange(d.getSelection());
    if (!range || isTextEditingTarget(d.activeElement)) {
      dismissSelectionAction();
      actionRange = null;
      return;
    }
    if (selectionGestureActive) return;
    // A selection this bridge just restored after a repaint is not a new
    // user gesture: re-emitting it would re-open the comment composer.
    if (restoredSelection && Date.now() < restoredSelectionExpiry) {
      try {
        if (
          range.compareBoundaryPoints(Range.START_TO_START, restoredSelection) === 0 &&
          range.compareBoundaryPoints(Range.END_TO_END, restoredSelection) === 0
        ) {
          return;
        }
      } catch {
        // The restored range's nodes were removed since; treat as new.
      }
    }
    restoredSelection = null;
    let unchanged = false;
    try {
      unchanged =
        !!actionRange &&
        range.compareBoundaryPoints(Range.START_TO_START, actionRange) === 0 &&
        range.compareBoundaryPoints(Range.END_TO_END, actionRange) === 0;
    } catch {
      /* The document replaced the previously selected nodes. */
    }
    if (!unchanged) selectionDismissed = false;
    actionRange = range.cloneRange();
    if (selectionDismissed) return;
    if (!selectionAction?.isConnected) {
      selectionAction = d.createElement("div");
      selectionAction.className = "aidocs-selection-action";
      selectionAction.setAttribute("role", "group");
      selectionAction.setAttribute("aria-label", "Selected text actions");
      selectionAction.addEventListener("mousedown", (e) => e.preventDefault());
      selectionAction.addEventListener("click", (e) => e.stopPropagation());
      const addAction = (
        label: string,
        title: string,
        path: string,
        click: () => void,
      ) => {
        const button = d.createElement("button");
        button.type = "button";
        button.setAttribute("aria-label", label);
        labelIcon(button, title);
        const svg = d.createElementNS("http://www.w3.org/2000/svg", "svg");
        svg.setAttribute("viewBox", "0 0 24 24");
        svg.setAttribute("aria-hidden", "true");
        const shape = d.createElementNS("http://www.w3.org/2000/svg", "path");
        shape.setAttribute("d", path);
        svg.appendChild(shape);
        button.appendChild(svg);
        button.addEventListener("click", click);
        selectionAction!.appendChild(button);
        return button;
      };
      addAction(
        "Comment on selected text",
        "Comment (⌘⌥M / Ctrl+Alt+M)",
        "M21 11.5a8.38 8.38 0 0 1-.9 3.8 8.5 8.5 0 0 1-7.6 4.7 8.38 8.38 0 0 1-3.8-.9L3 21l1.9-5.7a8.38 8.38 0 0 1-.9-3.8 8.5 8.5 0 0 1 4.7-7.6 8.38 8.38 0 0 1 3.8-.9H13a8.48 8.48 0 0 1 8 8v.5Z",
        () => {
          if (actionRange) commitSelection(actionRange);
        },
      );
      addAction(
        "Copy link to selected text",
        "Copy link to text",
        "M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71",
        () => {
          if (!actionRange) return;
          const anchor = describeAnchor(d.body, actionRange);
          parent.postMessage({ type: "aidocs:copy-text", anchor }, targetOrigin());
        },
      );
      if (!d.getElementById("aidocs-selection-style")) {
        const style = d.createElement("style");
        style.id = "aidocs-selection-style";
        style.textContent = `
          .aidocs-selection-action{all:initial!important;position:fixed!important;z-index:2147483647!important;display:flex!important;padding:2px!important;border-radius:10px!important;background:#fff!important;color:#52525b!important;box-shadow:0 0 0 1px #0001,0 4px 16px #0002!important}
          .aidocs-selection-action button{all:initial!important;position:relative!important;box-sizing:border-box!important;display:flex!important;align-items:center!important;justify-content:center!important;width:40px!important;height:40px!important;border-radius:8px!important;color:inherit!important;cursor:pointer!important;transition:background-color .15s,transform .15s!important}
          .aidocs-selection-action button:hover{background:#f4f4f5!important;color:#18181b!important}
          .aidocs-selection-action button:active{transform:scale(.96)!important}
          .aidocs-selection-action button:focus-visible{outline:2px solid #6366f1!important;outline-offset:-2px!important}
          .aidocs-selection-action svg{all:initial!important;color:inherit!important;width:17px!important;height:17px!important;fill:none!important;stroke:currentColor!important;stroke-width:1.8!important;stroke-linecap:round!important;stroke-linejoin:round!important;pointer-events:none!important}
          .aidocs-selection-action path{fill:none!important;stroke:inherit!important;stroke-width:inherit!important}
          [data-aidocs-theme=dark] .aidocs-selection-action{background:#27272a!important;color:#d4d4d8!important;box-shadow:0 0 0 1px #fff2,0 4px 16px #0004!important}
          [data-aidocs-theme=dark] .aidocs-selection-action button:hover{background:#3f3f46!important;color:#fff!important}
          @media(prefers-reduced-motion:reduce){.aidocs-selection-action button{transition:none!important}}
          @media print{.aidocs-selection-action,.aidocs-text-flash{display:none!important}}`;
        d.head.appendChild(style);
      }
      d.body.appendChild(selectionAction);
    }
    const rect = range.getBoundingClientRect();
    const view = d.defaultView!;
    selectionAction.style.setProperty(
      "left",
      `${Math.max(8, Math.min(rect.right - 84, view.innerWidth - 92))}px`,
      "important",
    );
    selectionAction.style.setProperty(
      "top",
      `${Math.max(8, rect.bottom + 8 + 44 < view.innerHeight ? rect.bottom + 8 : rect.top - 52)}px`,
      "important",
    );
  }

  function isTextEditingTarget(target: EventTarget | null): boolean {
    const d = doc();
    if (d && String(d.designMode || "").toLowerCase() === "on") return true;
    const keyTarget = target || (d && d.activeElement);
    const el =
      keyTarget && (keyTarget as Node).nodeType === 1
        ? (keyTarget as Element)
        : keyTarget && (keyTarget as Node).parentElement
          ? (keyTarget as Node).parentElement
          : null;
    if (!el || !el.closest) return false;
    const editable = el.closest("input,textarea,[contenteditable],[role='textbox']");
    if (!editable) return false;
    if (editable.matches && editable.matches("[contenteditable]")) {
      return (
        String(editable.getAttribute("contenteditable") || "").toLowerCase() !== "false"
      );
    }
    const role = String(editable.getAttribute("role") || "").toLowerCase();
    if (role === "textbox") return true;
    const tag = String(editable.tagName || "").toLowerCase();
    if (tag === "textarea") return true;
    if (tag !== "input") return false;
    const type = String(editable.getAttribute("type") || "text").toLowerCase();
    return !/^(button|checkbox|radio|submit|reset|file|image|color|range|hidden)$/.test(
      type,
    );
  }

  function handleKVUndoRedoShortcut(ev: KeyboardEvent): void {
    if (ev.defaultPrevented || isTextEditingTarget(ev.target)) return;
    if (!(ev.ctrlKey || ev.metaKey) || ev.altKey) return;
    const key = String(ev.key || "").toLowerCase();
    const wantsRedo = key === "y" || (key === "z" && ev.shiftKey);
    const wantsUndo = key === "z" && !ev.shiftKey;
    if (wantsRedo) {
      if (!kvRedoStack.length) return;
      ev.preventDefault();
      ev.stopPropagation();
      kvRedo().catch(() => {});
      return;
    }
    if (!wantsUndo || !kvUndoStack.length) return;
    ev.preventDefault();
    ev.stopPropagation();
    kvUndo().catch(() => {});
  }

  curTheme = initialTheme();
  let setupDoc: Document | null = null;

  let pendingHeading = "";
  let headingFlash: Animation | undefined;
  function prepareHeadings(d: Document): void {
    const used = new Set(Array.from(d.querySelectorAll("[id]")).map((el) => el.id));
    if (!d.getElementById("aidocs-heading-link-style")) {
      const style = d.createElement("style");
      style.id = "aidocs-heading-link-style";
      style.textContent = `.aidocs-heading-link{position:relative!important;display:inline-flex!important;vertical-align:middle;width:40px;height:40px;margin:0 0 0 4px;padding:0;align-items:center;justify-content:center;border:0;border-radius:6px;background:transparent;color:inherit;font:20px sans-serif;opacity:0;cursor:pointer;transition:opacity .15s}.aidocs-heading-link::before{content:"#"}h1:hover>.aidocs-heading-link,h2:hover>.aidocs-heading-link,h3:hover>.aidocs-heading-link,h4:hover>.aidocs-heading-link,h5:hover>.aidocs-heading-link,h6:hover>.aidocs-heading-link,.aidocs-heading-link:focus-visible{opacity:.65}.aidocs-heading-link:focus-visible{outline:2px solid currentColor}@media(pointer:coarse){.aidocs-heading-link{opacity:.5}}@media print{.aidocs-heading-link{display:none!important}}`;
      d.head.appendChild(style);
    }
    Array.from(d.querySelectorAll("h1,h2,h3,h4,h5,h6")).forEach((heading) => {
      if (!heading.id) {
        const base =
          (heading.textContent || "section")
            .trim()
            .toLowerCase()
            .replace(/[^a-z0-9]+/g, "-")
            .replace(/^-+|-+$/g, "") || "section";
        let id = base;
        for (let suffix = 2; used.has(id); suffix++) id = `${base}-${suffix}`;
        heading.id = id;
        used.add(id);
      }
      if (!heading.querySelector(":scope > .aidocs-heading-link")) {
        const button = d.createElement("button");
        button.type = "button";
        button.className = "aidocs-heading-link";
        button.setAttribute(
          "aria-label",
          `Copy link to ${heading.textContent?.trim() || "heading"}`,
        );
        labelIcon(button, "Copy link to heading");
        button.addEventListener("click", (event) => {
          event.preventDefault();
          event.stopPropagation();
          parent.postMessage(
            { type: "aidocs:copy-heading", id: heading.id },
            targetOrigin(),
          );
        });
        heading.appendChild(button);
      }
    });
    if (pendingHeading) navigateHeading(pendingHeading);
  }

  function navigateHeading(id: string): void {
    pendingText = null;
    textVisibilityObserver?.disconnect();
    clearTimeout(textNoticeTimer);
    pendingHeading = id;
    const d = doc();
    if (!d || !id) return;
    const target = d.getElementById(id);
    if (!target) return;
    for (let el = target.parentElement; el; el = el.parentElement) {
      if (el.tagName === "DETAILS") (el as HTMLDetailsElement).open = true;
    }
    if (!(target as HTMLElement).getClientRects().length) return;
    pendingHeading = "";
    target.scrollIntoView({ block: "start", behavior: "instant" });
    headingFlash?.cancel();
    const highlight = { backgroundColor: "rgba(129, 140, 248, 0.25)" };
    const resting = {
      backgroundColor: d.defaultView!.getComputedStyle(target).backgroundColor,
    };
    const reducedMotion = d.defaultView!.matchMedia(
      "(prefers-reduced-motion: reduce)",
    ).matches;
    // A single arrival cue, with no layout changes or persistent author styles.
    headingFlash = target.animate(
      [highlight, { ...highlight, offset: 0.25 }, resting],
      { duration: 1400, easing: reducedMotion ? "steps(1, end)" : "ease-out" },
    );
  }
  let textVisibilityObserver: MutationObserver | undefined;
  function scheduleTextNavigation(): void {
    if (pendingText && !textRetryTimer) {
      textRetryTimer = setTimeout(() => {
        textRetryTimer = undefined;
        tryNavigateText();
      }, 200);
    }
  }

  let textFlashStarted = 0;
  let pendingText: CommentAnchor | null = null;
  let textRetryTimer: ReturnType<typeof setTimeout> | undefined;
  let textNoticeTimer: ReturnType<typeof setTimeout> | undefined;

  function navigateText(value: unknown): void {
    pendingHeading = "";
    pendingText = textLinkAnchor(value);
    textVisibilityObserver?.disconnect();
    clearTimeout(textNoticeTimer);
    doc()
      ?.querySelectorAll(".aidocs-text-flash")
      .forEach((el) => el.remove());
    if (!pendingText) return;
    tryNavigateText();
    if (pendingText) {
      // Attribute-only tab/panel reveals do not trigger the comment text observer.
      textVisibilityObserver = new MutationObserver(scheduleTextNavigation);
      const root = doc()?.documentElement;
      if (root)
        textVisibilityObserver.observe(root, {
          subtree: true,
          attributes: true,
          attributeFilter: ["hidden", "class", "style", "open"],
        });
      textNoticeTimer = setTimeout(() => {
        if (pendingText)
          parent.postMessage({ type: "aidocs:text-unavailable" }, targetOrigin());
      }, 2000);
    }
  }

  function tryNavigateText(): void {
    const d = doc();
    if (!pendingText || !d?.body) return;
    // Deep links must target the quoted text, never an approximate replacement.
    const rendered = renderedTextOf(d.body);
    if (!rendered.text.includes(pendingText.quote!)) return;
    const range = resolveAnchor(d.body, pendingText, () => rendered);
    if (!range || renderedTextFromRange(range) !== pendingText.quote) return;
    const start =
      range.startContainer.nodeType === 1
        ? (range.startContainer as Element)
        : range.startContainer.parentElement;
    for (let el = start; el; el = el.parentElement) {
      if (el.tagName === "DETAILS") (el as HTMLDetailsElement).open = true;
    }
    if (!Array.from(range.getClientRects()).some((r) => r.width && r.height)) return;
    pendingText = null;
    textVisibilityObserver?.disconnect();
    clearTimeout(textNoticeTimer);
    start?.scrollIntoView({ block: "center", behavior: "instant" });
    const rect = range.getBoundingClientRect();
    if (rect.top < 0 || rect.top > d.defaultView!.innerHeight - 40) {
      d.defaultView!.scrollBy({
        top: rect.top - d.defaultView!.innerHeight / 2,
        behavior: "instant",
      });
    }
    textFlashStarted = Date.now();
    const reducedMotion = d.defaultView!.matchMedia(
      "(prefers-reduced-motion: reduce)",
    ).matches;
    // Non-interactive overlays avoid splitting text nodes or disturbing comments.
    Array.from(range.getClientRects())
      .slice(0, 200)
      .forEach((r) => {
        if (!r.width || !r.height) return;
        const flash = d.createElement("span");
        flash.className = "aidocs-text-flash";
        flash.setAttribute("aria-hidden", "true");
        // Keep opacity animatable: an important `all` reset overrides animation values.
        flash.style.cssText = `all:initial;position:fixed!important;pointer-events:none!important;z-index:2147483646!important;left:${r.left}px!important;top:${r.top}px!important;width:${r.width}px!important;height:${r.height}px!important;background:rgba(56,189,248,.32)!important;box-shadow:0 0 0 1px rgba(14,165,233,.65)!important;border-radius:3px!important`;
        d.body.appendChild(flash);
        const animation = flash.animate(
          reducedMotion
            ? [{ opacity: 1 }, { opacity: 1, offset: 0.85 }, { opacity: 0 }]
            : [
                { opacity: 0.2 },
                { opacity: 1, offset: 0.12 },
                { opacity: 0.25, offset: 0.25 },
                { opacity: 1, offset: 0.38 },
                { opacity: 1, offset: 0.72 },
                { opacity: 0 },
              ],
          { duration: 2200, easing: reducedMotion ? "steps(1, end)" : "ease-in-out" },
        );
        animation.onfinish = () => flash.remove();
      });
  }

  function setupFrame(): void {
    const d = doc();
    if (!d || !d.documentElement || !d.body || setupDoc === d) return;
    setupDoc = d;
    pendingText = null;
    textVisibilityObserver?.disconnect();
    clearTimeout(textRetryTimer);
    textRetryTimer = undefined;
    clearTimeout(textNoticeTimer);
    // A freshly loaded document has no highlights; the parent repaints after
    // the ready signal below. Dropping the previous document's payload — and
    // any repaint already scheduled for it — keeps its comments from being
    // anchored into the new document mid-load.
    lastPaint = null;
    lastPaintKey = null;
    lastPaintResults = null;
    commentPositionFrame = 0;
    commentScrollFrame = 0;
    revealedActive = null;
    restoredSelection = null;
    paintedQuotes.clear();
    clearTimeout(mutationRepaintTimer ?? undefined);
    mutationRepaintTimer = null;
    mutationRepaintDeadline = 0;
    observeMutations(d);
    resetKVHistory();
    injectStyles();
    installAidocsAPI();
    dismissSelectionAction();
    actionRange = null;
    selectionDismissed = false;
    d.defaultView?.addEventListener(
      "scroll",
      () => {
        if (Date.now() - textFlashStarted > 100)
          d.querySelectorAll(".aidocs-text-flash").forEach((el) => el.remove());
      },
      { passive: true },
    );
    d.defaultView?.addEventListener("scroll", dismissSelectionAction, {
      passive: true,
    });
    prepareHeadings(d);
    prepareLinks();
    applyTheme(curTheme);
    // Track the selection gesture so mutation repairs never run mid-drag
    // (they would make the browser stop rendering the native selection).
    d.addEventListener("mousedown", () => {
      selectionGestureActive = true;
    });
    d.addEventListener("mouseup", () => {
      selectionGestureActive = false;
    });
    d.addEventListener("mouseup", () => setTimeout(selection, 0));
    d.addEventListener("keyup", () => setTimeout(selection, 0));
    // Touch devices: while a finger is down the user is still adjusting the
    // selection (long-press then drag the handles), so we must NOT emit yet
    // — doing so pops the comments sheet and steals the drag. We only emit
    // once the finger lifts (touchend), i.e. the gesture has settled.
    var touching = false;
    var touchEmitTimer: number | null = null;
    d.addEventListener(
      "touchstart",
      function () {
        touching = true;
        selectionGestureActive = true;
        // A new touch (e.g. grabbing a selection handle to drag) cancels a
        // pending emit from the previous touchend.
        if (touchEmitTimer) {
          clearTimeout(touchEmitTimer);
          touchEmitTimer = null;
        }
      },
      { passive: true },
    );
    d.addEventListener("touchend", function () {
      touching = false;
      selectionGestureActive = false;
      // Wait after the finger lifts before emitting. If the user grabs a
      // selection handle to keep dragging, touchstart cancels this timer, so
      // the sheet only appears once they've truly finished selecting.
      if (touchEmitTimer) clearTimeout(touchEmitTimer);
      touchEmitTimer = setTimeout(function () {
        touchEmitTimer = null;
        selection();
      }, 450);
    });
    d.addEventListener("touchcancel", function () {
      touching = false;
      selectionGestureActive = false;
    });
    // selectionchange handles mouse drag-selection on desktop; on touch it
    // fires continuously mid-drag, so skip it while a finger is down.
    var selTimer: number | null = null;
    d.addEventListener("selectionchange", function () {
      if (touching) return;
      if (selTimer) clearTimeout(selTimer);
      selTimer = setTimeout(selection, 350);
    });
    const chordKeys = ["t", "f", "c", "h"];
    let chordArmedAt = 0;
    let vimGArmed = false;
    let vimGTimer = 0;
    const runVimScroll = (action: VimScrollAction, repeat = false) => {
      const view = d.defaultView;
      if (!view) return;
      const behavior = view.matchMedia("(prefers-reduced-motion: reduce)").matches
        ? "instant"
        : "smooth";
      if (action === "line-down")
        view.scrollBy({
          top: vimLineScrollDistance(repeat),
          behavior,
        });
      else if (action === "line-up")
        view.scrollBy({
          top: -vimLineScrollDistance(repeat),
          behavior,
        });
      else if (action === "half-page-down")
        view.scrollBy({ top: Math.max(160, view.innerHeight * 0.5), behavior });
      else if (action === "half-page-up")
        view.scrollBy({
          top: -Math.max(160, view.innerHeight * 0.5),
          behavior,
        });
      else if (action === "top") view.scrollTo({ top: 0, behavior });
      else
        view.scrollTo({
          top: d.documentElement.scrollHeight,
          behavior,
        });
    };
    d.addEventListener("keydown", (ev) => {
      if (ev.key === "Escape") dismissSelectionAction();
      if (
        (ev.metaKey || ev.ctrlKey) &&
        ev.altKey &&
        ev.key.toLowerCase() === "m" &&
        !isTextEditingTarget(ev.target)
      ) {
        const range = selectedRange(d.getSelection());
        if (range) {
          ev.preventDefault();
          commitSelection(range);
          return;
        }
      }
      const target = ev.target as Element | null;
      const textEditing = !!target?.closest?.(
        'input,textarea,select,[contenteditable="true"]',
      );
      if (ev.defaultPrevented || textEditing) {
        chordArmedAt = 0;
        vimGArmed = false;
        clearTimeout(vimGTimer);
        handleKVUndoRedoShortcut(ev);
        return;
      }

      const modified = ev.metaKey || ev.ctrlKey || ev.altKey;
      if (ev.key === "Escape") {
        chordArmedAt = modified ? 0 : Date.now();
        parent.postMessage({ type: "aidocs:keydown", key: "Escape" }, targetOrigin());
      } else if (chordArmedAt) {
        const withinWindow = Date.now() - chordArmedAt <= 800;
        chordArmedAt = 0;
        const key = ev.key.toLowerCase();
        if (withinWindow && !modified && chordKeys.includes(key)) {
          parent.postMessage({ type: "aidocs:keydown", key }, targetOrigin());
        }
      }

      const vim = reduceVimScroll(
        {
          key: ev.key,
          ctrlKey: ev.ctrlKey,
          metaKey: ev.metaKey,
          altKey: ev.altKey,
        },
        vimGArmed,
      );
      vimGArmed = vim.gArmed;
      clearTimeout(vimGTimer);
      if (vimGArmed) {
        vimGTimer = setTimeout(() => {
          vimGArmed = false;
        }, 600);
      }
      if (vim.action) {
        ev.preventDefault();
        runVimScroll(vim.action, ev.repeat);
      }
      handleKVUndoRedoShortcut(ev);
    });
    // Forward trackpad pinch-to-zoom (wheel + ctrlKey) to parent so the
    // app can scale only the document iframe instead of browser-zooming.
    d.addEventListener(
      "wheel",
      function (ev) {
        if (!ev.ctrlKey) return;
        ev.preventDefault();
        parent.postMessage(
          {
            type: "aidocs:zoom",
            deltaY: ev.deltaY,
            clientX: ev.clientX,
            clientY: ev.clientY,
          },
          targetOrigin(),
        );
      },
      { passive: false },
    );
    d.addEventListener("click", activate);
    d.addEventListener("click", handleLinkClick);
    d.addEventListener(
      "click",
      () => parent.postMessage({ type: "aidocs:document-click" }, targetOrigin()),
      true,
    );
    d.defaultView?.addEventListener("scroll", scheduleCommentScroll, {
      passive: true,
    });
    d.defaultView?.addEventListener("resize", scheduleCommentPositions);
    parent.postMessage({ type: "aidocs:ready" }, targetOrigin());
    hydrateKV();
  }

  frame.addEventListener("load", setupFrame);
  try {
    const d = doc();
    if (d && d.readyState !== "loading") setTimeout(setupFrame, 0);
  } catch (e) {}

  window.addEventListener("message", (e) => {
    if (!e.data) return;
    if (!isParentMessage(e)) return;
    if (
      e.data.type === "aidocs:print-snapshot" &&
      typeof e.data.requestId === "string"
    ) {
      const d = doc();
      if (!d || printPreparing) return;
      printPreparing = true;
      void snapshotForPrint(d)
        .then((html) => {
          parent.postMessage(
            {
              type: "aidocs:print-snapshot-result",
              requestId: e.data.requestId,
              html,
              title: d.title,
            },
            targetOrigin(),
          );
        })
        .catch(() => {
          parent.postMessage(
            { type: "aidocs:print-snapshot-result", requestId: e.data.requestId },
            targetOrigin(),
          );
        });
      return;
    }
    if (e.data.type === "aidocs:print-finished") {
      printPreparing = false;
      innerWindow()?.dispatchEvent(new Event("afterprint"));
      innerWindow()?.dispatchEvent(new CustomEvent("aidocs:afterprint"));
      return;
    }
    if (e.data.type === "aidocs:export-response") {
      const pending = exportPending.get(e.data.requestId);
      exportPending.delete(e.data.requestId);
      if (e.data.ok === true) pending?.resolve();
      else pending?.reject();
      return;
    }
    if (e.data.type === "aidocs:paint") {
      paint(e.data.comments, e.data.active, e.data.revealActive !== false);
    } else if (e.data.type === "aidocs:theme") applyTheme(e.data.theme);
    else if (e.data.type === "aidocs:kv-response") {
      const requestId = String(e.data.requestId || "");
      const pending = kvPending[requestId];
      if (!pending) return;
      delete kvPending[requestId];
      if (e.data.ok) pending.resolve(e.data.result || {});
      else
        pending.reject(
          new Error((e.data.error && e.data.error.message) || "KV request failed"),
        );
    } else if (e.data.type === "aidocs:github-response") {
      const requestId = String(e.data.requestId || "");
      const pending = githubPending[requestId];
      if (!pending) return;
      delete githubPending[requestId];
      if (e.data.ok) pending.resolve(e.data.result || {});
      else
        pending.reject(
          new Error((e.data.error && e.data.error.message) || "GitHub request failed"),
        );
    } else if (e.data.type === "aidocs:navigate-text") {
      navigateText(e.data.anchor);
    } else if (e.data.type === "aidocs:navigate-heading") {
      navigateHeading(String(e.data.id || ""));
    } else if (e.data.type === "aidocs:request-comment-positions") {
      scheduleCommentPositions();
    } else if (
      e.data.type === "aidocs:reveal-comment" &&
      typeof e.data.id === "string" &&
      e.data.id.length <= 1024
    ) {
      const d = doc();
      if (d) applyActiveOnly(d, e.data.id, true);
    } else if (
      e.data.type === "aidocs:scroll-to" &&
      typeof e.data.top === "number" &&
      Number.isFinite(e.data.top)
    ) {
      doc()?.defaultView?.scrollTo({
        top: Math.max(0, e.data.top),
        behavior: "instant",
      });
    } else if (e.data.type === "aidocs:vim-key") {
      const d = doc();
      const KeyboardEventCtor = d?.defaultView?.KeyboardEvent;
      if (!d || !KeyboardEventCtor) return;
      d.dispatchEvent(
        new KeyboardEventCtor("keydown", {
          key: String(e.data.key || ""),
          ctrlKey: e.data.ctrlKey === true,
          metaKey: e.data.metaKey === true,
          altKey: e.data.altKey === true,
          repeat: e.data.repeat === true,
        }),
      );
    } else if (e.data.type === "aidocs:kv-event") {
      emitKVChange(e.data.detail || {});
    }
  });
})();
