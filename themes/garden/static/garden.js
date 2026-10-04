// kvist garden theme: progressive enhancements. Every page works without
// this script; it adds the theme toggle, search, the graph, math and
// diagrams.
(function () {
  "use strict";
  var root = document.documentElement;
  var $ = function (sel, el) { return (el || document).querySelector(sel); };
  var css = function (name) { return getComputedStyle(root).getPropertyValue(name).trim(); };
  var isDark = function () {
    return root.dataset.theme === "dark" ||
      (!root.dataset.theme && window.matchMedia("(prefers-color-scheme: dark)").matches);
  };
  var onThemeChange = [];

  // ---- color theme: system → light → dark → system ----
  var toggle = $(".theme-toggle");
  if (toggle) {
    toggle.hidden = false;
    var order = ["", "light", "dark"];
    var label = function () { toggle.title = "Theme: " + (root.dataset.theme || "system"); };
    label();
    toggle.addEventListener("click", function () {
      var next = order[(order.indexOf(root.dataset.theme || "") + 1) % order.length];
      if (next) root.dataset.theme = next; else delete root.dataset.theme;
      try { next ? localStorage.setItem("kvist-theme", next) : localStorage.removeItem("kvist-theme"); } catch (e) {}
      label();
      onThemeChange.forEach(function (f) { f(); });
    });
  }

  // ---- print: light colors and folded callouts opened ----
  var printState = null;
  window.addEventListener("beforeprint", function () {
    var closed = Array.prototype.filter.call(document.querySelectorAll("details:not([open])"), function (d) { d.open = true; return true; });
    printState = { theme: root.dataset.theme, closed: closed };
    root.dataset.theme = "light"; // also switches code highlighting to light
  });
  window.addEventListener("afterprint", function () {
    if (!printState) return;
    if (printState.theme) root.dataset.theme = printState.theme; else delete root.dataset.theme;
    printState.closed.forEach(function (d) { d.open = false; });
    printState = null;
  });

  // ---- nav menu on small screens ----
  var menuToggle = $(".menu-toggle");
  if (menuToggle) {
    menuToggle.hidden = false;
    menuToggle.addEventListener("click", function () {
      var open = document.body.classList.toggle("nav-open");
      menuToggle.setAttribute("aria-expanded", open ? "true" : "false");
    });
  }

  // ---- list pane: keep the current note in view, filter the list ----
  var listPane = $(".list-pane");
  if (listPane) {
    var current = $('.list-items [aria-current="page"]', listPane);
    if (current && listPane.scrollHeight > listPane.clientHeight) {
      listPane.scrollTop = current.offsetTop - listPane.clientHeight / 3; // the pane is the offset parent
    }
    var filter = $(".list-filter", listPane);
    var empty = $(".list-empty", listPane);
    var items = listPane.querySelectorAll(".list-items > li");
    if (filter && items.length > 1) {
      filter.hidden = false;
      filter.addEventListener("input", function () {
        var q = filter.value.trim().toLowerCase();
        var shown = 0;
        items.forEach(function (li) {
          var hit = !q || li.textContent.toLowerCase().indexOf(q) >= 0;
          li.hidden = !hit;
          if (hit) shown++;
        });
        if (empty) empty.hidden = shown > 0;
      });
      filter.addEventListener("keydown", function (e) {
        if (e.key !== "Enter") return;
        var first = listPane.querySelector(".list-items > li:not([hidden]) a");
        if (first) location.href = first.href;
      });
    }
  }

  // ---- tag index: filter and sort the tag cards ----
  var tagTools = $(".tag-tools");
  var tagGrid = $(".tag-grid");
  if (tagTools && tagGrid) {
    tagTools.hidden = false;
    var cards = Array.prototype.slice.call(tagGrid.children);
    var tagFilter = $(".tag-filter", tagTools);
    var tagEmpty = $(".tag-empty");
    tagFilter.addEventListener("input", function () {
      var q = tagFilter.value.trim().replace(/^#/, "").toLowerCase();
      var shown = 0;
      cards.forEach(function (c) {
        var hit = !q || c.dataset.search.toLowerCase().indexOf(q) >= 0;
        c.hidden = !hit;
        if (hit) shown++;
      });
      if (tagEmpty) tagEmpty.hidden = shown > 0;
    });
    tagTools.querySelectorAll("[data-sort]").forEach(function (b) {
      b.addEventListener("click", function () {
        var by = b.dataset.sort;
        tagTools.querySelectorAll("[data-sort]").forEach(function (o) { o.setAttribute("aria-pressed", o === b ? "true" : "false"); });
        cards.sort(function (x, y) {
          if (by === "count") {
            var d = Number(y.dataset.count) - Number(x.dataset.count);
            if (d) return d;
          }
          return x.dataset.name.localeCompare(y.dataset.name);
        });
        cards.forEach(function (c) { tagGrid.appendChild(c); });
      });
    });
  }

  document.querySelectorAll(".link-unpublished").forEach(function (el) { el.title = "Not published"; });

  // ---- math (KaTeX is loaded only on pages that need it) ----
  if (window.katex) {
    document.querySelectorAll(".math").forEach(function (el) {
      var tex = el.textContent.replace(/^\\[([]/, "").replace(/\\[)\]]$/, "");
      try {
        window.katex.render(tex, el, { displayMode: el.classList.contains("math-display"), throwOnError: false });
      } catch (e) { /* leave the source visible */ }
    });
  }

  // ---- diagrams (Mermaid is loaded only on pages that need it) ----
  if (window.mermaid) {
    var sources = [];
    document.querySelectorAll("pre.mermaid").forEach(function (el) { sources.push([el, el.textContent]); });
    var drawDiagrams = function () {
      sources.forEach(function (s) { s[0].removeAttribute("data-processed"); s[0].textContent = s[1]; });
      window.mermaid.initialize({ startOnLoad: false, theme: isDark() ? "dark" : "default", securityLevel: "strict" });
      window.mermaid.run({ nodes: sources.map(function (s) { return s[0]; }) }).catch(function () {});
    };
    drawDiagrams();
    onThemeChange.push(drawDiagrams);
  }

  // ---- data loading ----
  var cache = {};
  function getJSON(url) {
    if (!cache[url]) {
      cache[url] = fetch(url, { credentials: "same-origin" }).then(function (r) {
        if (!r.ok) throw new Error(r.status);
        return r.json();
      });
    }
    return cache[url];
  }
  function loadScript(src) {
    return new Promise(function (resolve, reject) {
      var s = document.createElement("script");
      s.src = src; s.onload = resolve; s.onerror = reject;
      document.head.appendChild(s);
    });
  }
  function el(tag, cls, text) {
    var e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text != null) e.textContent = text;
    return e;
  }

  // ---- search ----
  var searchBtn = $(".search-toggle");
  if (searchBtn && window.fetch && window.HTMLDialogElement) {
    searchBtn.hidden = false;
    var dialog, input, list, engine, docs = {}, results = [], active = -1;

    var build = function () {
      dialog = el("dialog", "search-dialog");
      dialog.setAttribute("aria-label", "Search");
      var form = el("form");
      form.method = "dialog";
      input = el("input");
      input.type = "search";
      input.placeholder = "Search notes…";
      input.setAttribute("aria-label", "Search notes");
      input.autocomplete = "off";
      list = el("ul", "search-results");
      list.setAttribute("role", "listbox");
      form.appendChild(input);
      dialog.appendChild(form);
      dialog.appendChild(list);
      dialog.appendChild(el("p", "search-hint", "↑↓ to choose · Enter to open · Esc to close"));
      document.body.appendChild(dialog);
      dialog.addEventListener("click", function (e) { if (e.target === dialog) dialog.close(); });
      input.addEventListener("input", run);
      input.addEventListener("keydown", function (e) {
        if (e.key === "ArrowDown" || e.key === "ArrowUp") {
          e.preventDefault();
          if (!results.length) return;
          active = (active + (e.key === "ArrowDown" ? 1 : -1) + results.length) % results.length;
          paint();
        } else if (e.key === "Enter") {
          e.preventDefault();
          var r = results[active >= 0 ? active : 0];
          if (r) location.href = docs[r.id].url;
        }
      });
    };

    var ready = function () {
      if (engine) return Promise.resolve();
      return Promise.all([
        window.MiniSearch ? Promise.resolve() : loadScript(searchBtn.dataset.lib),
        getJSON(searchBtn.dataset.index)
      ]).then(function (res) {
        var data = res[1];
        engine = new window.MiniSearch({
          fields: ["title", "aliases", "tags", "description", "text"],
          storeFields: [],
          extractField: function (doc, field) {
            var v = doc[field];
            return Array.isArray(v) ? v.join(" ") : v;
          },
          searchOptions: { boost: { title: 4, aliases: 3, tags: 2, description: 1.5 }, prefix: true, fuzzy: 0.2 }
        });
        data.docs.forEach(function (d) { docs[d.id] = d; });
        engine.addAll(data.docs);
      });
    };

    var snippet = function (doc, terms) {
      var text = doc.text || "";
      var lower = text.toLowerCase();
      var at = -1;
      for (var i = 0; i < terms.length && at < 0; i++) at = lower.indexOf(terms[i].toLowerCase());
      if (at < 0) return doc.description || text.slice(0, 160);
      var start = Math.max(0, at - 60);
      return (start > 0 ? "…" : "") + text.slice(start, start + 180) + "…";
    };

    var paint = function () {
      list.textContent = "";
      results.forEach(function (r, i) {
        var d = docs[r.id];
        var li = el("li");
        li.setAttribute("role", "option");
        if (i === active) li.setAttribute("aria-selected", "true");
        var a = el("a");
        a.href = d.url;
        a.appendChild(el("span", "search-title", d.title));
        a.appendChild(el("span", "search-snippet", snippet(d, r.terms)));
        li.appendChild(a);
        list.appendChild(li);
      });
      var sel = list.querySelector('[aria-selected="true"]');
      if (sel) sel.scrollIntoView({ block: "nearest" });
    };

    var run = function () {
      if (!engine) return; // still loading; ready() runs the query when done
      var q = input.value.trim();
      results = q ? engine.search(q).slice(0, 12) : [];
      active = results.length ? 0 : -1;
      if (q && !results.length) {
        list.textContent = "";
        list.appendChild(el("li", "search-empty", "No notes found."));
        return;
      }
      paint();
    };

    var open = function (query) {
      if (!dialog) build();
      dialog.showModal();
      if (typeof query === "string") input.value = query;
      input.select();
      ready().then(run).catch(function () {
        list.textContent = "";
        list.appendChild(el("li", "search-empty", "Search is not available."));
      });
    };
    searchBtn.addEventListener("click", function () { open(); });
    var listSearch = $(".list-search");
    if (listSearch) listSearch.addEventListener("click", function () { open($(".list-filter").value); });
    document.addEventListener("keydown", function (e) {
      var t = e.target;
      var typing = t && (t.isContentEditable || /^(input|textarea|select)$/i.test(t.tagName));
      if (!typing && (e.key === "/" || ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k"))) {
        e.preventDefault();
        open();
      }
    });
  }

  // ---- graph ----
  // A small force-directed layout on <canvas>: repulsion between all
  // nodes, springs along links, gravity to the center.
  function Graph(container, data, opts) {
    var canvas = el("canvas");
    container.appendChild(canvas);
    var ctx = canvas.getContext("2d");
    var nodes = data.nodes.map(function (n, i) {
      var a = i * 2.399963; // golden angle spiral as a stable start
      var r = 12 * Math.sqrt(i + 1);
      return { id: n.id, title: n.title, url: n.url, x: r * Math.cos(a), y: r * Math.sin(a), vx: 0, vy: 0, deg: 0 };
    });
    var byId = {};
    nodes.forEach(function (n) { byId[n.id] = n; });
    var links = data.edges.filter(function (e) { return byId[e.source] && byId[e.target] && e.source !== e.target; })
      .map(function (e) { byId[e.source].deg++; byId[e.target].deg++; return { s: byId[e.source], t: byId[e.target] }; });
    var current = byId[opts.current];
    var view = { x: 0, y: 0, k: 1 }, w = 0, h = 0, hover = null, alpha = 1, raf = 0, dragging = null;

    function radius(n) { return 3 + Math.min(6, Math.sqrt(n.deg) * 1.6); }

    function resize() {
      var dpr = window.devicePixelRatio || 1;
      w = container.clientWidth; h = container.clientHeight;
      canvas.width = w * dpr; canvas.height = h * dpr;
      canvas.style.width = w + "px"; canvas.style.height = h + "px";
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      draw();
    }

    function tick() {
      var n = nodes.length, i, j, a, b, dx, dy, d2, f;
      for (i = 0; i < n; i++) {
        a = nodes[i];
        for (j = i + 1; j < n; j++) {
          b = nodes[j];
          dx = a.x - b.x; dy = a.y - b.y;
          d2 = dx * dx + dy * dy + 0.01;
          if (d2 > 90000) continue;
          f = 900 / d2 * alpha;
          a.vx += dx * f; a.vy += dy * f; b.vx -= dx * f; b.vy -= dy * f;
        }
      }
      links.forEach(function (l) {
        dx = l.t.x - l.s.x; dy = l.t.y - l.s.y;
        var d = Math.sqrt(dx * dx + dy * dy) || 1;
        f = (d - 45) / d * 0.06 * alpha;
        l.s.vx += dx * f; l.s.vy += dy * f; l.t.vx -= dx * f; l.t.vy -= dy * f;
      });
      nodes.forEach(function (p) {
        p.vx -= p.x * 0.004 * alpha; p.vy -= p.y * 0.004 * alpha;
        if (p !== dragging) { p.x += p.vx; p.y += p.vy; }
        p.vx *= 0.6; p.vy *= 0.6;
      });
      alpha *= 0.985;
    }

    function toScreen(p) { return [w / 2 + (p.x + view.x) * view.k, h / 2 + (p.y + view.y) * view.k]; }
    function toWorld(sx, sy) { return [(sx - w / 2) / view.k - view.x, (sy - h / 2) / view.k - view.y]; }

    function draw() {
      var fg = css("--fg"), muted = css("--muted"), accent = css("--accent"), border = css("--border");
      ctx.clearRect(0, 0, w, h);
      var near = {};
      if (hover) {
        near[hover.id] = true;
        links.forEach(function (l) { if (l.s === hover) near[l.t.id] = true; if (l.t === hover) near[l.s.id] = true; });
      }
      ctx.lineWidth = 1;
      links.forEach(function (l) {
        var a = toScreen(l.s), b = toScreen(l.t);
        ctx.strokeStyle = hover && (l.s === hover || l.t === hover) ? accent : border;
        ctx.beginPath(); ctx.moveTo(a[0], a[1]); ctx.lineTo(b[0], b[1]); ctx.stroke();
      });
      nodes.forEach(function (n) {
        var p = toScreen(n), r = radius(n) * Math.max(0.7, Math.min(1.6, view.k));
        ctx.globalAlpha = hover && !near[n.id] ? 0.25 : 1;
        ctx.fillStyle = n === current ? accent : (n === hover ? accent : muted);
        ctx.beginPath(); ctx.arc(p[0], p[1], r, 0, 6.2832); ctx.fill();
        if (n === current || n === hover || near[n.id] && hover || view.k > 1.6 || opts.labels) {
          ctx.fillStyle = fg;
          ctx.font = (n === current ? "600 " : "") + "12px system-ui, sans-serif";
          ctx.textAlign = "center";
          ctx.fillText(n.title, p[0], p[1] + r + 13);
        }
      });
      ctx.globalAlpha = 1;
    }

    function loop() {
      for (var i = 0; i < 3 && alpha > 0.02; i++) tick();
      draw();
      raf = alpha > 0.02 || dragging ? requestAnimationFrame(loop) : 0;
    }
    function kick(a) { alpha = Math.max(alpha, a); if (!raf) raf = requestAnimationFrame(loop); }

    function pick(ev) {
      var rect = canvas.getBoundingClientRect();
      var wpt = toWorld(ev.clientX - rect.left, ev.clientY - rect.top), best = null, bd = 1e9;
      nodes.forEach(function (n) {
        var dx = n.x - wpt[0], dy = n.y - wpt[1], d = dx * dx + dy * dy, r = (radius(n) + 4) / view.k;
        if (d < r * r && d < bd) { best = n; bd = d; }
      });
      return best;
    }

    var panStart = null, moved = false;
    canvas.addEventListener("pointerdown", function (ev) {
      canvas.setPointerCapture(ev.pointerId);
      moved = false;
      dragging = pick(ev);
      panStart = dragging ? null : { x: ev.clientX, y: ev.clientY, vx: view.x, vy: view.y };
      if (dragging) kick(0.3);
    });
    canvas.addEventListener("pointermove", function (ev) {
      if (dragging) {
        var rect = canvas.getBoundingClientRect(), wpt = toWorld(ev.clientX - rect.left, ev.clientY - rect.top);
        dragging.x = wpt[0]; dragging.y = wpt[1]; moved = true; kick(0.3);
      } else if (panStart) {
        view.x = panStart.vx + (ev.clientX - panStart.x) / view.k;
        view.y = panStart.vy + (ev.clientY - panStart.y) / view.k;
        moved = true; draw();
      } else {
        var n = pick(ev);
        if (n !== hover) { hover = n; canvas.style.cursor = n ? "pointer" : "grab"; draw(); }
      }
    });
    canvas.addEventListener("pointerup", function (ev) {
      var n = dragging;
      dragging = null; panStart = null;
      if (!moved) { var t = pick(ev); if (t && t !== current) location.href = t.url; }
      else if (n) kick(0.1);
    });
    canvas.addEventListener("pointerleave", function () { if (hover) { hover = null; draw(); } });
    canvas.addEventListener("wheel", function (ev) {
      ev.preventDefault();
      view.k = Math.max(0.2, Math.min(4, view.k * Math.exp(-ev.deltaY * 0.0015)));
      draw();
    }, { passive: false });

    // Fit all nodes (and room for labels) into the view.
    function fit() {
      if (!nodes.length || !w) return;
      var x0 = 1e9, x1 = -1e9, y0 = 1e9, y1 = -1e9;
      nodes.forEach(function (n) {
        x0 = Math.min(x0, n.x); x1 = Math.max(x1, n.x); y0 = Math.min(y0, n.y); y1 = Math.max(y1, n.y);
      });
      var padX = Math.min(90, w * 0.28), padY = Math.min(30, h * 0.12);
      view.k = Math.max(0.2, Math.min(1.5, (w - 2 * padX) / Math.max(1, x1 - x0), (h - 2 * padY) / Math.max(1, y1 - y0)));
      view.x = -(x0 + x1) / 2;
      view.y = -(y0 + y1) / 2;
    }

    // Settle before the first paint so the graph doesn't explode in. The
    // repulsion is O(n²), so large graphs settle less up front.
    var settle = nodes.length > 600 ? 40 : 300;
    for (var s = 0; s < settle; s++) tick();
    if (window.ResizeObserver) new ResizeObserver(function () { resize(); }).observe(container);
    onThemeChange.push(draw);
    resize();
    fit();
    kick(Math.min(alpha, 0.05));
  }

  // Neighborhood of a note: the note, its links and backlinks, and theirs.
  function neighborhood(data, id, depth) {
    var keep = {}, frontier = [id];
    keep[id] = true;
    for (var d = 0; d < depth; d++) {
      var next = [];
      data.edges.forEach(function (e) {
        frontier.forEach(function (f) {
          if (e.source === f && !keep[e.target]) { keep[e.target] = true; next.push(e.target); }
          if (e.target === f && !keep[e.source]) { keep[e.source] = true; next.push(e.source); }
        });
      });
      frontier = next;
    }
    return {
      nodes: data.nodes.filter(function (n) { return keep[n.id]; }),
      edges: data.edges.filter(function (e) { return keep[e.source] && keep[e.target]; })
    };
  }

  var noteId = document.body.dataset.note;
  var local = $(".local-graph");
  if (local && noteId && window.fetch && window.HTMLCanvasElement) {
    getJSON("/graph.json").then(function (data) {
      var sub = neighborhood(data, noteId, 2);
      if (sub.nodes.length < 2) return;
      local.hidden = false;
      new Graph($(".graph-canvas", local), sub, { current: noteId, labels: sub.nodes.length <= 6 });
    }).catch(function () {});
  }

  var graphBtn = $(".graph-toggle");
  if (graphBtn && window.fetch && window.HTMLDialogElement && window.HTMLCanvasElement) {
    graphBtn.hidden = false;
    var gdialog = null;
    graphBtn.addEventListener("click", function () {
      if (!gdialog) {
        gdialog = el("dialog", "graph-dialog");
        gdialog.setAttribute("aria-label", "Graph of all notes");
        var close = el("button", "icon-button graph-close", "×");
        close.type = "button";
        close.setAttribute("aria-label", "Close");
        close.addEventListener("click", function () { gdialog.close(); });
        var area = el("div", "graph-canvas");
        gdialog.appendChild(close);
        gdialog.appendChild(area);
        gdialog.addEventListener("click", function (e) { if (e.target === gdialog) gdialog.close(); });
        document.body.appendChild(gdialog);
        gdialog.showModal();
        getJSON("/graph.json").then(function (data) {
          new Graph(area, data, { current: noteId, labels: data.nodes.length <= 40 });
        }).catch(function () { area.textContent = "The graph is not available."; });
        return;
      }
      gdialog.showModal();
    });
  }
})();
