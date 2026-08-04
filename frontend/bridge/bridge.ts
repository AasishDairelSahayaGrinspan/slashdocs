import type { RenderedTextWithBrPositions } from "./anchoring/rendered-text.ts";
import { renderedTextOf } from "./anchoring/rendered-text.ts";
import { describeAnchor, resolveAnchor } from "./anchoring/index.ts";
import type { CommentAnchor } from "./anchoring/types.ts";
import { clearHighlights, highlightRange } from "./highlight.ts";

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

interface AidocsPublicAPI {
  kv?: Record<string, KVFunction>;
  on?: (event: string, handler: KVChangeHandler) => (() => void) | undefined;
  off?: (event: string, handler: KVChangeHandler) => void;
  [key: string]: unknown;
}

declare global {
  interface Window {
    __AIDOCS_APP_ORIGIN__?: string;
    __aidocsKVHandlers?: KVHandlers;
    __aidocsKVQueue?: QueuedKVOperation[];
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
    existing.kv = kv;
    existing.on = on;
    existing.off = off;
    try {
      w.aidocs = existing;
    } catch (e) {}

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

  // Debounce mutation-triggered repaints (trailing 250ms, forced within 1s)
  // so a burst of DOM writes anchors once, and a document that mutates
  // continuously still repaints at a bounded rate.
  function scheduleMutationRepaint(): void {
    if (!lastPaint || !lastPaint.items.length) return;
    const now = Date.now();
    if (!mutationRepaintDeadline) mutationRepaintDeadline = now + 1000;
    clearTimeout(mutationRepaintTimer ?? undefined);
    mutationRepaintTimer = setTimeout(
      () => {
        mutationRepaintTimer = null;
        mutationRepaintDeadline = 0;
        if (lastPaint) paint(lastPaint.items, lastPaint.active, false);
      },
      Math.max(0, Math.min(now + 250, mutationRepaintDeadline) - now),
    );
  }

  // Our own painting mutates the document (splitText, mark wrapping, svg
  // rects, normalize); suspend observation around it so only the document's
  // own changes trigger re-anchoring. Pending external records are drained
  // first — disconnect() would silently discard them.
  function suspendMutationObserver(): void {
    if (!mutationObserver) return;
    if (mutationObserver.takeRecords().length) scheduleMutationRepaint();
    mutationObserver.disconnect();
  }

  function resumeMutationObserver(): void {
    if (mutationObserver && observedRoot) {
      mutationObserver.observe(observedRoot, mutationObserverOptions);
    }
  }

  function observeMutations(d: Document): void {
    if (mutationObserver) mutationObserver.disconnect();
    mutationObserver = new MutationObserver(scheduleMutationRepaint);
    // Observe the root element rather than body so a script replacing
    // document.body entirely is still seen.
    observedRoot = d.documentElement;
    mutationObserver.observe(observedRoot, mutationObserverOptions);
  }

  function paint(
    items: PaintItem[] | null | undefined,
    active: string | null | undefined,
    scroll = true,
  ): void {
    const d = doc();
    const list = items || [];
    lastPaint = { items: list, active: typeof active === "string" ? active : null };
    const results: { id: string; anchored: boolean }[] = [];
    const body = d && d.body;
    if (d && body) {
      suspendMutationObserver();
      try {
        clearHighlights(d);
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
            // Resolve immediately before painting each comment: wrapping a
            // previous comment's marks moves text nodes, which corrupts other
            // already-resolved live Ranges (overlapping comments would paint
            // wrong extents), while character offsets into textContent stay
            // valid across wraps and splits.
            const r = resolveAnchor(body, anchor, getRenderedText);
            painted = !!r && highlightRange(r, x.id === active, x.id, scroll) > 0;
          } catch {
            painted = false;
          }
          if (x && x.id) results.push({ id: x.id, anchored: painted });
        });
      } catch {
        // Even a document with exotic DOM state (throwing accessors, broken
        // replaceWith) must not prevent the paint-result post below.
      } finally {
        resumeMutationObserver();
      }
    }
    // Post even when the inner document is unavailable so the parent never
    // keeps stale "highlight not found" state.
    parent.postMessage({ type: "aidocs:paint-result", results }, targetOrigin());
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

  function selection(): void {
    const d = doc();
    if (!d || !d.body) return;
    const range = selectedRange(d.getSelection());
    if (!range) return;
    const anchor = describeAnchor(d.body, range);
    if (!anchor) return;
    parent.postMessage({ type: "aidocs:selection", anchor }, targetOrigin());
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
  function setupFrame(): void {
    const d = doc();
    if (!d || !d.documentElement || !d.body || setupDoc === d) return;
    setupDoc = d;
    // A freshly loaded document has no highlights; the parent repaints after
    // the ready signal below. Dropping the previous document's payload — and
    // any repaint already scheduled for it — keeps its comments from being
    // anchored into the new document mid-load.
    lastPaint = null;
    clearTimeout(mutationRepaintTimer ?? undefined);
    mutationRepaintTimer = null;
    mutationRepaintDeadline = 0;
    observeMutations(d);
    resetKVHistory();
    injectStyles();
    installAidocsAPI();
    prepareLinks();
    applyTheme(curTheme);
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
    });
    // selectionchange handles mouse drag-selection on desktop; on touch it
    // fires continuously mid-drag, so skip it while a finger is down.
    var selTimer: number | null = null;
    d.addEventListener("selectionchange", function () {
      if (touching) return;
      if (selTimer) clearTimeout(selTimer);
      selTimer = setTimeout(selection, 350);
    });
    d.addEventListener("keydown", (ev) => {
      if (ev.key === "Escape") {
        parent.postMessage({ type: "aidocs:keydown", key: "Escape" }, targetOrigin());
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
    if (e.data.type === "aidocs:paint") paint(e.data.comments, e.data.active);
    else if (e.data.type === "aidocs:theme") applyTheme(e.data.theme);
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
    } else if (e.data.type === "aidocs:kv-event") {
      emitKVChange(e.data.detail || {});
    } else if (e.data.type === "aidocs:print") {
      const w = innerWindow();
      if (!w) return;
      try {
        w.focus();
        w.print();
      } catch (err) {}
    }
  });
})();
