function bridge() {
  const appOrigin = window.__AIDOCS_APP_ORIGIN__ || "*";
  const frame = document.getElementById("aidocs-doc");
  const svgNS = "http://www.w3.org/2000/svg";
  let editMode = false;
  let editDirty = false;
  let originalBodyAttrs = null;
  let originalDocAttrs = null;
  let preparedLinkID = 0;
  const preparedLinks = {};
  let kvRequestID = 0;
  const kvPending = {};
  const kvRequestTimeoutMS = 10000;

  function doc() {
    try {
      return frame.contentDocument || frame.contentWindow.document;
    } catch (e) {
      return null;
    }
  }

  function targetOrigin() {
    if (!appOrigin || appOrigin === "*") return "*";
    if (appOrigin === "self" || appOrigin === "'self'") return window.location.origin;
    try {
      return new URL(appOrigin, window.location.href).origin;
    } catch (e) {
      return "*";
    }
  }

  function isParentMessage(e) {
    if (e.source !== parent) return false;
    const origin = targetOrigin();
    return origin === "*" || e.origin === origin;
  }

  function innerWindow() {
    try {
      return frame.contentWindow;
    } catch (e) {
      return null;
    }
  }

  function jsonValue(value) {
    const text = JSON.stringify(value);
    if (text === undefined) throw new TypeError("KV value must be JSON serializable");
    return JSON.parse(text);
  }

  function kvRequest(operation, payload) {
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
        Object.assign({ type: "aidocs:kv-request", requestId, operation }, payload || {}),
        targetOrigin(),
      );
    });
  }

  function emitKVChange(detail) {
    const w = innerWindow();
    if (!w) return;
    try {
      const ev = new w.CustomEvent("kvchange", { detail });
      w.dispatchEvent(ev);
    } catch (e) {}
    const handlers =
      (w.__aidocsKVHandlers && w.__aidocsKVHandlers.kvchange) || [];
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

  function installAidocsAPI() {
    const w = innerWindow();
    if (!w) return;
    const existing = w.aidocs && typeof w.aidocs === "object" ? w.aidocs : {};
    const handlers =
      w.__aidocsKVHandlers && w.__aidocsKVHandlers.kvchange
        ? w.__aidocsKVHandlers
        : { kvchange: [] };
    w.__aidocsKVHandlers = handlers;

    function off(event, handler) {
      if (event !== "kvchange") return;
      handlers.kvchange = handlers.kvchange.filter((h) => h !== handler);
    }
    function on(event, handler) {
      if (event !== "kvchange" || typeof handler !== "function") return;
      handlers.kvchange.push(handler);
      return function unsubscribe() {
        off(event, handler);
      };
    }
    const kv = {
      get(key) {
        return kvRequest("get", { key: String(key) }).then((res) => res.value);
      },
      all() {
        return kvRequest("all").then((res) => res.items || {});
      },
      set(key, value) {
        if (value === undefined) {
          throw new TypeError("aidocs.kv.set requires a JSON value");
        }
        const set = {};
        set[String(key)] = jsonValue(value);
        return kvRequest("patch", { set, delete: [], source: "local", kvOperation: "set" });
      },
      delete(key) {
        return kvRequest("patch", {
          set: {},
          delete: [String(key)],
          source: "local",
          kvOperation: "delete",
        });
      },
      patch(object) {
        if (!object || typeof object !== "object" || Array.isArray(object)) {
          throw new TypeError("aidocs.kv.patch requires an object");
        }
        const set = {};
        const deleteKeys = [];
        Object.keys(object).forEach((key) => {
          const value = object[key];
          if (value === null || value === undefined) {
            deleteKeys.push(key);
          } else {
            set[key] = jsonValue(value);
          }
        });
        return kvRequest("patch", {
          set,
          delete: deleteKeys,
          source: "local",
          kvOperation: "patch",
        });
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

  function hydrateKV() {
    kvRequest("hydrate", { source: "initial", operation: "hydrate" })
      .then((res) => {
        emitKVChange(
          Object.assign({ source: "initial", operation: "hydrate", changes: [] }, res || {}),
        );
      })
      .catch(() => {});
  }

  function textNodes(root) {
    const w = (doc() || document).createTreeWalker(root, NodeFilter.SHOW_TEXT, {
      acceptNode: (n) =>
        n.nodeValue.trim() ? NodeFilter.FILTER_ACCEPT : NodeFilter.FILTER_REJECT,
    });
    const a = [];
    let n;
    while ((n = w.nextNode())) a.push(n);
    return a;
  }

  function injectStyles() {
    const d = doc();
    if (!d || d.getElementById("aidocs-highlight-style")) return;
    const s = d.createElement("style");
    s.id = "aidocs-highlight-style";
    s.textContent =
      ".aidocs-mark{background:#fef08a!important;color:#111827!important;-webkit-text-fill-color:#111827!important;box-decoration-break:clone;-webkit-box-decoration-break:clone;border-radius:2px;box-shadow:0 0 0 1px rgba(202,138,4,.55);cursor:pointer;transition:background-color .15s,box-shadow .15s}.aidocs-mark-active{background:#fde047!important;box-shadow:0 0 0 2px rgba(202,138,4,.95)}.aidocs-svg-mark-text{fill:#111827!important;-webkit-text-fill-color:#111827!important;cursor:pointer}.aidocs-svg-mark-rect{fill:#fef08a;stroke:#ca8a04;stroke-width:1;rx:3;ry:3;pointer-events:auto;opacity:.72}.aidocs-svg-mark-rect.aidocs-mark-active{fill:#fde047;stroke-width:2;opacity:.84}";
    (d.head || d.documentElement).appendChild(s);
  }

  function injectEditStyles() {
    const d = doc();
    if (!d || d.getElementById("aidocs-edit-style")) return;
    const s = d.createElement("style");
    s.id = "aidocs-edit-style";
    s.textContent =
      "html.aidocs-edit-mode body{min-height:100vh!important;outline:2px solid rgba(37,99,235,.65)!important;outline-offset:-2px!important;cursor:text!important;caret-color:#2563eb!important}html.aidocs-edit-mode body:focus{outline:2px solid rgba(37,99,235,.9)!important}";
    (d.head || d.documentElement).appendChild(s);
  }

  function clear() {
    const d = doc();
    if (!d) return;
    d.querySelectorAll("mark.aidocs-mark").forEach((m) =>
      m.replaceWith(...m.childNodes),
    );
    d.querySelectorAll(".aidocs-svg-mark-rect").forEach((r) => r.remove());
    d.querySelectorAll(".aidocs-svg-mark-text").forEach((t) => {
      t.classList.remove("aidocs-svg-mark-text");
      delete t.dataset.cid;
    });
    d.body && d.body.normalize();
  }

  function rememberOriginalBodyAttrs(d) {
    if (originalBodyAttrs || !d || !d.body) return;
    originalBodyAttrs = {
      contenteditable: d.body.getAttribute("contenteditable"),
      spellcheck: d.body.getAttribute("spellcheck"),
      aidocsEditing: d.body.getAttribute("data-aidocs-editing"),
      designMode: d.designMode || "off",
    };
  }

  function rememberOriginalDocAttrs(d) {
    if (originalDocAttrs || !d || !d.documentElement) return;
    originalDocAttrs = {
      aidocsTheme: d.documentElement.getAttribute("data-aidocs-theme"),
    };
  }

  function setOrRemoveAttr(el, name, value) {
    if (value === null || value === undefined) el.removeAttribute(name);
    else el.setAttribute(name, value);
  }

  function restoreBodyAttrs(body) {
    if (!body) return;
    if (!originalBodyAttrs) {
      body.removeAttribute("contenteditable");
      body.removeAttribute("spellcheck");
      body.removeAttribute("data-aidocs-editing");
      return;
    }
    setOrRemoveAttr(body, "contenteditable", originalBodyAttrs.contenteditable);
    setOrRemoveAttr(body, "spellcheck", originalBodyAttrs.spellcheck);
    setOrRemoveAttr(body, "data-aidocs-editing", originalBodyAttrs.aidocsEditing);
  }

  function postEditDirty() {
    parent.postMessage({ type: "aidocs:edit-dirty", dirty: editDirty }, targetOrigin());
  }

  function markEditDirty() {
    if (!editMode || editDirty) return;
    editDirty = true;
    postEditDirty();
  }

  function applyEditMode(enabled) {
    const d = doc();
    if (!d || !d.body) return;
    editMode = !!enabled;
    rememberOriginalBodyAttrs(d);
    injectEditStyles();
    if (editMode) {
      clear();
      d.documentElement.classList.add("aidocs-edit-mode");
      d.body.setAttribute("contenteditable", "true");
      d.body.setAttribute("spellcheck", "true");
      d.body.setAttribute("data-aidocs-editing", "true");
      try {
        d.designMode = "off";
        d.body.focus({ preventScroll: true });
      } catch (e) {}
    } else {
      d.documentElement.classList.remove("aidocs-edit-mode");
      restoreBodyAttrs(d.body);
      try {
        d.designMode =
          originalBodyAttrs && originalBodyAttrs.designMode
            ? originalBodyAttrs.designMode
            : "off";
      } catch (e) {}
    }
    postEditDirty();
  }

  function cleanupCloneForSave(root) {
    root
      .querySelectorAll("mark.aidocs-mark")
      .forEach((m) => m.replaceWith(...m.childNodes));
    root.querySelectorAll(".aidocs-svg-mark-rect").forEach((r) => r.remove());
    root.querySelectorAll(".aidocs-svg-mark-text").forEach((t) => {
      t.classList.remove("aidocs-svg-mark-text");
      delete t.dataset.cid;
    });
    root
      .querySelectorAll("#aidocs-highlight-style,#aidocs-edit-style,#aidocs-bootstrap")
      .forEach((s) => s.remove());
    root.classList.remove("aidocs-edit-mode");
    setOrRemoveAttr(
      root,
      "data-aidocs-theme",
      originalDocAttrs ? originalDocAttrs.aidocsTheme : null,
    );
    root.querySelectorAll("a[data-aidocs-link-bridge-id]").forEach((a) => {
      const id = a.getAttribute("data-aidocs-link-bridge-id");
      const original = preparedLinks[id] || {};
      if (original.target === null || original.target === undefined) {
        a.removeAttribute("target");
      } else {
        a.setAttribute("target", original.target);
      }
      if (original.rel === null || original.rel === undefined) {
        a.removeAttribute("rel");
      } else {
        a.setAttribute("rel", original.rel);
      }
      if (original.hadBridgeID) {
        a.setAttribute("data-aidocs-link-bridge-id", original.bridgeID || "");
      } else {
        a.removeAttribute("data-aidocs-link-bridge-id");
      }
    });
    restoreBodyAttrs(root.querySelector("body"));
  }

  function doctypeString(d) {
    const dt = d && d.doctype;
    if (!dt) return "<!doctype html>";
    let out = "<!doctype " + dt.name;
    if (dt.publicId) out += ' PUBLIC "' + dt.publicId + '"';
    if (dt.systemId) out += (dt.publicId ? "" : " SYSTEM") + ' "' + dt.systemId + '"';
    return out + ">";
  }

  function editedHTML() {
    const d = doc();
    if (!d || !d.documentElement) return "";
    const root = d.documentElement.cloneNode(true);
    cleanupCloneForSave(root);
    return doctypeString(d) + "\n" + root.outerHTML;
  }

  function sendEditedHTML(requestId) {
    parent.postMessage(
      { type: "aidocs:edit-html", requestId, html: editedHTML() },
      targetOrigin(),
    );
  }

  function norm(s) {
    return (s || "").replace(/\s+/g, " ").trim();
  }

  function isSVGNode(n) {
    return !!(n && n.parentElement && n.parentElement.namespaceURI === svgNS);
  }

  function svgHighlight(n, active, id) {
    const el = n.parentElement;
    if (!el || !el.getBBox) return 0;
    let box;
    try {
      box = el.getBBox();
    } catch (e) {
      return 0;
    }
    if (!box || box.width <= 0 || box.height <= 0) return 0;
    const rect = el.ownerDocument.createElementNS(svgNS, "rect");
    rect.classList.add("aidocs-svg-mark-rect");
    if (active) rect.classList.add("aidocs-mark-active");
    if (id) {
      rect.dataset.cid = id;
      el.dataset.cid = id;
    }
    rect.setAttribute("x", String(box.x - 3));
    rect.setAttribute("y", String(box.y - 2));
    rect.setAttribute("width", String(box.width + 6));
    rect.setAttribute("height", String(box.height + 4));
    el.parentNode.insertBefore(rect, el);
    el.classList.add("aidocs-svg-mark-text");
    if (active) el.scrollIntoView({ block: "center", behavior: "smooth" });
    return 1;
  }

  function htmlHighlight(n, q, active, id) {
    const d = doc();
    const raw = n.nodeValue;
    const i = raw.indexOf(q);
    const r = d.createRange();
    if (i >= 0) {
      r.setStart(n, i);
      r.setEnd(n, Math.min(raw.length, i + q.length));
    } else {
      r.selectNode(n);
    }
    return highlightRange(r, active, id);
  }

  function rangeAtOffsets(d, nodes, start, end) {
    if (start < 0 || end <= start) return null;
    let pos = 0;
    let startNode = null;
    let startOffset = 0;
    let endNode = null;
    let endOffset = 0;
    for (const n of nodes) {
      const len = n.nodeValue.length;
      const ns = pos;
      const ne = pos + len;
      if (!startNode && start >= ns && start <= ne) {
        startNode = n;
        startOffset = Math.min(len, Math.max(0, start - ns));
      }
      if (startNode && end >= ns && end <= ne) {
        endNode = n;
        endOffset = Math.min(len, Math.max(0, end - ns));
        break;
      }
      pos = ne;
    }
    if (!startNode || !endNode) return null;
    const r = d.createRange();
    r.setStart(startNode, startOffset);
    r.setEnd(endNode, endOffset);
    return r;
  }

  function highlightRange(r, active, id) {
    const d = doc();
    const m = d.createElement("mark");
    m.className = "aidocs-mark" + (active ? " aidocs-mark-active" : "");
    if (id) m.dataset.cid = id;
    try {
      r.surroundContents(m);
      if (active) m.scrollIntoView({ block: "center", behavior: "smooth" });
      return 1;
    } catch (e) {
      return 0;
    }
  }

  function markAtAnchor(a, active, id) {
    const d = doc();
    if (
      !d ||
      !a ||
      typeof a.start_offset !== "number" ||
      typeof a.end_offset !== "number"
    ) {
      return 0;
    }
    const nodes = textNodes(d.body);
    const r = rangeAtOffsets(d, nodes, a.start_offset, a.end_offset);
    if (!r) return 0;
    if (a.quote && norm(r.toString()) !== norm(a.quote)) return 0;

    if (r.startContainer === r.endContainer && isSVGNode(r.startContainer)) {
      return svgHighlight(r.startContainer, active, id);
    }
    return highlightRange(r, active, id);
  }

  function markQuote(q, active, id) {
    const d = doc();
    if (!d || !q) return 0;
    injectStyles();
    const nq = norm(q);
    if (!nq) return 0;
    for (const n of textNodes(d.body)) {
      const raw = n.nodeValue;
      const nText = norm(raw);
      if (nText.indexOf(nq) < 0 && raw.indexOf(q) < 0) continue;
      if (isSVGNode(n)) {
        if (svgHighlight(n, active, id)) return 1;
        continue;
      }
      if (htmlHighlight(n, q, active, id)) return 1;
    }
    return 0;
  }

  function paint(items, active) {
    clear();
    injectStyles();
    (items || []).forEach((x) => {
      const a = x.anchor || {};
      if (!markAtAnchor(a, x.id === active, x.id)) {
        markQuote(a.quote || x.quote || x.selected_text, x.id === active, x.id);
      }
    });
  }

  function activate(e) {
    if (editMode) return;
    const m =
      e.target &&
      e.target.closest &&
      e.target.closest("mark.aidocs-mark,.aidocs-svg-mark-text,.aidocs-svg-mark-rect");
    if (!m || !m.dataset.cid) return;
    parent.postMessage({ type: "aidocs:activate", id: m.dataset.cid }, targetOrigin());
  }

  function cssString(s) {
    return String(s || "")
      .replace(/\\/g, "\\\\")
      .replace(/"/g, '\\"');
  }

  function fragmentTarget(d, hash) {
    if (!hash || hash === "#") return null;
    let id;
    try {
      id = decodeURIComponent(hash.slice(1));
    } catch (e) {
      id = hash.slice(1);
    }
    return d.getElementById(id) || d.querySelector('[name="' + cssString(id) + '"]');
  }

  function openURL(url) {
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

  function handleLinkClick(e) {
    const d = doc();
    if (!d || !e.target || !e.target.closest) return;
    const a = e.target.closest("a[href]");
    if (!a) return;
    if (editMode) {
      e.preventDefault();
      return;
    }
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
          d.defaultView.history.replaceState(null, "", href);
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

  function prepareLinks() {
    const d = doc();
    if (!d) return;
    d.querySelectorAll("a[href]").forEach((a) => {
      const href = a.getAttribute("href") || "";
      if (href && !/^#|javascript:/i.test(href)) {
        const hadBridgeID = a.hasAttribute("data-aidocs-link-bridge-id");
        const bridgeID = hadBridgeID
          ? a.getAttribute("data-aidocs-link-bridge-id") || ""
          : "";
        const id = String(++preparedLinkID);
        preparedLinks[id] = {
          target: a.hasAttribute("target") ? a.getAttribute("target") || "" : null,
          rel: a.hasAttribute("rel") ? a.getAttribute("rel") || "" : null,
          hadBridgeID,
          bridgeID,
        };
        a.setAttribute("data-aidocs-link-bridge-id", id);
        a.setAttribute("target", "_blank");
        a.setAttribute("rel", "noopener noreferrer");
      }
    });
  }

  let curTheme = null;
  function initialTheme() {
    try {
      const t = new URLSearchParams(location.search).get("aidocs_theme");
      return t === "dark" || t === "light" ? t : null;
    } catch (e) {
      return null;
    }
  }

  function applyTheme(t) {
    if (t !== "dark" && t !== "light") return;
    curTheme = t;
    const d = doc();
    if (!d || !d.documentElement) return;
    d.documentElement.setAttribute("data-aidocs-theme", t);
    try {
      d.defaultView.dispatchEvent(
        new CustomEvent("aidocs:theme", { detail: { theme: t } }),
      );
    } catch (e) {}
  }

  function offsetOfNode(nodes, target, offset) {
    let pos = 0;
    for (const n of nodes) {
      if (n === target) return pos + offset;
      pos += n.nodeValue.length;
    }
    return -1;
  }

  function selection() {
    if (editMode) return;
    const d = doc();
    if (!d) return;
    const s = d.getSelection();
    if (!s || s.isCollapsed || s.rangeCount < 1) return;
    const q = s.toString().trim();
    if (!q) return;
    let pre = "";
    let suf = "";
    let start = 0;
    let end = q.length;
    const domPath = "body";
    try {
      const r = s.getRangeAt(0);
      const nodes = textNodes(d.body);
      start = offsetOfNode(nodes, r.startContainer, r.startOffset);
      end = offsetOfNode(nodes, r.endContainer, r.endOffset);
      const body = d.body.textContent || "";
      if (start < 0 || end < 0) {
        start = body.indexOf(q);
        end = start + q.length;
      }
      pre = body.slice(Math.max(0, start - 64), start);
      suf = body.slice(end, end + 64);
    } catch (e) {}
    parent.postMessage(
      {
        type: "aidocs:selection",
        anchor: {
          quote: q,
          prefix: pre,
          suffix: suf,
          dom_path: domPath,
          start_offset: start,
          end_offset: end,
        },
      },
      targetOrigin(),
    );
  }

  function handleEditPaste(ev) {
    if (!editMode) return;
    const text = ev.clipboardData && ev.clipboardData.getData("text/plain");
    if (!text) return;
    ev.preventDefault();
    try {
      doc().execCommand("insertText", false, text);
    } catch (e) {
      const s = doc().getSelection();
      if (!s || s.rangeCount < 1) return;
      s.deleteFromDocument();
      s.getRangeAt(0).insertNode(doc().createTextNode(text));
      s.collapseToEnd();
    }
  }

  curTheme = initialTheme();
  frame.addEventListener("load", () => {
    const d = doc();
    if (!d) return;
    editMode = false;
    editDirty = false;
    originalBodyAttrs = null;
    originalDocAttrs = null;
    preparedLinkID = 0;
    Object.keys(preparedLinks).forEach((key) => delete preparedLinks[key]);
    rememberOriginalDocAttrs(d);
    injectStyles();
    installAidocsAPI();
    prepareLinks();
    applyTheme(curTheme);
    d.addEventListener("mouseup", () => setTimeout(selection, 0));
    d.addEventListener("keyup", () => setTimeout(selection, 0));
    d.addEventListener("input", markEditDirty);
    d.addEventListener("paste", handleEditPaste);
    // Touch devices: while a finger is down the user is still adjusting the
    // selection (long-press then drag the handles), so we must NOT emit yet
    // — doing so pops the comments sheet and steals the drag. We only emit
    // once the finger lifts (touchend), i.e. the gesture has settled.
    var touching = false;
    var touchEmitTimer = null;
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
    var selTimer = null;
    d.addEventListener("selectionchange", function () {
      if (touching) return;
      if (selTimer) clearTimeout(selTimer);
      selTimer = setTimeout(selection, 350);
    });
    d.addEventListener("keydown", (ev) => {
      if (ev.key === "Escape") {
        parent.postMessage({ type: "aidocs:keydown", key: "Escape" }, targetOrigin());
      }
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
  });

  window.addEventListener("message", (e) => {
    if (!e.data) return;
    if (!isParentMessage(e)) return;
    if (e.data.type === "aidocs:paint") paint(e.data.comments, e.data.active);
    else if (e.data.type === "aidocs:theme") applyTheme(e.data.theme);
    else if (e.data.type === "aidocs:edit-mode") applyEditMode(!!e.data.enabled);
    else if (e.data.type === "aidocs:edit-request-html")
      sendEditedHTML(e.data.requestId);
    else if (e.data.type === "aidocs:kv-response") {
      const requestId = String(e.data.requestId || "");
      const pending = kvPending[requestId];
      if (!pending) return;
      delete kvPending[requestId];
      if (e.data.ok) pending.resolve(e.data.result || {});
      else pending.reject(new Error((e.data.error && e.data.error.message) || "KV request failed"));
    } else if (e.data.type === "aidocs:kv-event") {
      emitKVChange(e.data.detail || {});
    }
  });
}
