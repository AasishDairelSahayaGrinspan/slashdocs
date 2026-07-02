function bridge() {
  const appOrigin = window.__AIDOCS_APP_ORIGIN__ || "*";
  const frame = document.getElementById("aidocs-doc");
  const svgNS = "http://www.w3.org/2000/svg";

  function doc() {
    try {
      return frame.contentDocument || frame.contentWindow.document;
    } catch (e) {
      return null;
    }
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
    const m =
      e.target &&
      e.target.closest &&
      e.target.closest("mark.aidocs-mark,.aidocs-svg-mark-text,.aidocs-svg-mark-rect");
    if (!m || !m.dataset.cid) return;
    const targetOrigin = appOrigin === "self" ? "*" : appOrigin;
    parent.postMessage({ type: "aidocs:activate", id: m.dataset.cid }, targetOrigin);
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
    const targetOrigin = appOrigin === "self" ? "*" : appOrigin;
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
      targetOrigin,
    );
  }

  curTheme = initialTheme();
  frame.addEventListener("load", () => {
    const d = doc();
    if (!d) return;
    injectStyles();
    prepareLinks();
    applyTheme(curTheme);
    d.addEventListener("mouseup", () => setTimeout(selection, 0));
    d.addEventListener("keyup", () => setTimeout(selection, 0));
    // Touch devices don't fire a reliable mouseup after selecting text.
    // touchend catches tap-to-select; selectionchange (debounced) catches
    // dragging the selection handles on mobile.
    d.addEventListener("touchend", () => setTimeout(selection, 0));
    var selTimer = null;
    d.addEventListener("selectionchange", function () {
      if (selTimer) clearTimeout(selTimer);
      selTimer = setTimeout(selection, 350);
    });
    d.addEventListener("keydown", (ev) => {
      if (ev.key === "Escape") {
        const t = appOrigin === "self" ? "*" : appOrigin;
        parent.postMessage({ type: "aidocs:keydown", key: "Escape" }, t);
      }
    });
    // Forward trackpad pinch-to-zoom (wheel + ctrlKey) to parent so the
    // app can scale only the document iframe instead of browser-zooming.
    d.addEventListener("wheel", function (ev) {
      if (!ev.ctrlKey) return;
      ev.preventDefault();
      var t = appOrigin === "self" ? "*" : appOrigin;
      parent.postMessage(
        {
          type: "aidocs:zoom",
          deltaY: ev.deltaY,
          clientX: ev.clientX,
          clientY: ev.clientY,
        },
        t,
      );
    }, { passive: false });
    d.addEventListener("click", activate);
    d.addEventListener("click", handleLinkClick);
    const targetOrigin = appOrigin === "self" ? "*" : appOrigin;
    parent.postMessage({ type: "aidocs:ready" }, targetOrigin);
  });

  window.addEventListener("message", (e) => {
    if (!e.data) return;
    if (e.data.type === "aidocs:paint") paint(e.data.comments, e.data.active);
    else if (e.data.type === "aidocs:theme") applyTheme(e.data.theme);
  });
}
