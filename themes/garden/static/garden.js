// kvist garden theme: progressive enhancements. Every page works without
// this script; it adds the theme toggle, search, the graph, math,
// diagrams and instant navigation. Parts that belong to the page run in
// initPage, again after every instant navigation; the rest runs once.
(function () {
  "use strict";
  var root = document.documentElement;
  var $ = function (sel, el) { return (el || document).querySelector(sel); };
  var css = function (name) { return getComputedStyle(root).getPropertyValue(name).trim(); };
  var isDark = function () {
    return root.dataset.theme === "dark" ||
      (!root.dataset.theme && window.matchMedia("(prefers-color-scheme: dark)").matches);
  };
  var onThemeChange = []; // for the whole visit
  var onNavigate = [];    // after instant navigation
  var pageHooks = [];     // theme changes for the current page only
  var noteId = null;      // the current page's note, if it is one
  var openSearch = null;  // set by search
  // go opens a page of the site: in place with instant navigation, else
  // by loading it.
  var go = function (url) { location.href = url; };
  // The interface's words in the site's language (i18n/*.toml, [script]).
  var words = {};
  try { words = JSON.parse(document.body.dataset.i18n || "{}"); } catch (e) {}
  var tr = function (key, def, arg) {
    var s = words[key] || def;
    return arg == null ? s : s.replace("%s", arg);
  };

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
  // Stylesheets and scripts another page of the site needs (KaTeX,
  // Mermaid, code styles) and this one lacks, for showing that page here.
  // Resolves when the scripts have loaded.
  function addAssets(doc) {
    var sameNode = function (a, b) {
      return a.tagName === b.tagName && (a.getAttribute("href") || a.getAttribute("src")) === (b.getAttribute("href") || b.getAttribute("src"));
    };
    var waits = [];
    doc.head.querySelectorAll('link[rel="stylesheet"], script[src]').forEach(function (n) {
      var have = Array.prototype.some.call(document.head.querySelectorAll(n.tagName), function (o) { return sameNode(o, n); });
      if (have) return;
      if (n.tagName === "LINK") { document.head.appendChild(document.importNode(n, true)); return; }
      var s = document.createElement("script");
      Array.prototype.forEach.call(n.attributes, function (a) { if (a.name !== "defer") s.setAttribute(a.name, a.value); });
      waits.push(new Promise(function (resolve) { s.onload = s.onerror = resolve; }));
      document.head.appendChild(s);
    });
    return Promise.all(waits);
  }

  // ---- color theme: system → light → dark → system ----
  var toggle = $(".theme-toggle");
  if (toggle) {
    toggle.hidden = false;
    var order = ["", "light", "dark"];
    var label = function () { toggle.title = tr("theme", "Theme: %s", tr("theme_" + (root.dataset.theme || "system"), root.dataset.theme || "system")); };
    label();
    toggle.addEventListener("click", function () {
      var next = order[(order.indexOf(root.dataset.theme || "") + 1) % order.length];
      if (next) root.dataset.theme = next; else delete root.dataset.theme;
      try { next ? localStorage.setItem("kvist-theme", next) : localStorage.removeItem("kvist-theme"); } catch (e) {}
      label();
      onThemeChange.concat(pageHooks).forEach(function (f) { f(); });
    });
  }

  // ---- reader mode: only the page, remembered across pages ----
  var reader = $(".reader-toggle");
  if (reader) {
    reader.hidden = false;
    var readerState = function () { reader.setAttribute("aria-pressed", root.hasAttribute("data-reader") ? "true" : "false"); };
    readerState();
    reader.addEventListener("click", function () {
      var on = !root.hasAttribute("data-reader");
      if (on) root.dataset.reader = ""; else delete root.dataset.reader;
      try { on ? localStorage.setItem("kvist-reader", "1") : localStorage.removeItem("kvist-reader"); } catch (e) {}
      readerState();
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
  // A pane that instant navigation kept is set up already.
  function initList() {
    var listPane = $(".list-pane");
    if (!listPane || listPane.dataset.ready) return;
    listPane.dataset.ready = "1";
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
        if (first) go(first.href);
      });
    }
    var listSearch = $(".list-search", listPane);
    if (listSearch) listSearch.addEventListener("click", function () { if (openSearch) openSearch(filter.value); });
  }

  // ---- tag index: filter and sort the tag cards ----
  function initTagTools() {
    var tagTools = $(".tag-tools");
    var tagGrid = $(".tag-grid");
    if (!tagTools || !tagGrid) return;
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

  // ---- heading anchors: copy a link to the section ----
  function initAnchors(page) {
    page.querySelectorAll(".content :is(h1, h2, h3, h4, h5, h6)[id]").forEach(function (h) {
      if (h.closest(".embed")) return; // ids of embedded notes belong to their own page
      var a = document.createElement("a");
      a.className = "heading-anchor";
      a.href = "#" + encodeURIComponent(h.id);
      a.textContent = "#";
      a.setAttribute("aria-label", tr("copy_heading_link", "Copy link to “%s”", h.textContent));
      a.addEventListener("click", function (e) {
        if (!navigator.clipboard) return; // plain jump to the section
        e.preventDefault();
        var url = location.origin + location.pathname + a.getAttribute("href");
        history.replaceState(history.state, "", a.getAttribute("href"));
        navigator.clipboard.writeText(url).then(function () {
          a.classList.add("copied");
          a.dataset.label = tr("link_copied", "Link copied");
          setTimeout(function () { a.classList.remove("copied"); }, 1600);
        }, function () { location.hash = a.getAttribute("href"); });
      });
      h.appendChild(a);
    });
  }

  // ---- code blocks: a copy button ----
  function initCode(page) {
    if (!navigator.clipboard) return;
    page.querySelectorAll(".content pre:not(.mermaid)").forEach(function (pre) {
      var wrap = pre.parentNode.classList.contains("code-block") ? pre.parentNode : null;
      if (!wrap) { // plain blocks without a language aren't wrapped yet
        wrap = el("div", "code-block");
        pre.parentNode.insertBefore(wrap, pre);
        wrap.appendChild(pre);
      }
      var copy = tr("copy_code", "Copy");
      var b = el("button", "code-copy", copy);
      b.type = "button";
      b.addEventListener("click", function () {
        var code = pre.querySelector("code") || pre;
        navigator.clipboard.writeText(code.textContent.replace(/\n$/, "")).then(function () {
          b.textContent = tr("code_copied", "Copied");
          b.classList.add("copied");
          setTimeout(function () { b.textContent = copy; b.classList.remove("copied"); }, 1600);
        });
      });
      wrap.appendChild(b);
    });
  }

  // ---- math (KaTeX is loaded only on pages that need it) ----
  function renderMath(scope) {
    if (!window.katex) return;
    scope.querySelectorAll(".math:not([data-rendered])").forEach(function (el) {
      var tex = el.textContent.replace(/^\\[([]/, "").replace(/\\[)\]]$/, "");
      try {
        window.katex.render(tex, el, { displayMode: el.classList.contains("math-display"), throwOnError: false });
        el.dataset.rendered = "";
      } catch (e) { /* leave the source visible */ }
    });
  }

  // ---- diagrams (Mermaid is loaded only on pages that need it) ----
  function initDiagrams(page) {
    if (!window.mermaid) return;
    var sources = [];
    page.querySelectorAll("pre.mermaid:not([data-processed])").forEach(function (el) { sources.push([el, el.textContent]); });
    if (!sources.length) return;
    var drawDiagrams = function () {
      sources.forEach(function (s) { s[0].removeAttribute("data-processed"); s[0].textContent = s[1]; });
      window.mermaid.initialize({ startOnLoad: false, theme: isDark() ? "dark" : "default", securityLevel: "strict" });
      window.mermaid.run({ nodes: sources.map(function (s) { return s[0]; }) }).catch(function () {});
    };
    drawDiagrams();
    pageHooks.push(drawDiagrams);
  }

  // ---- link previews: hover a link to a note (tap on touch screens) ----
  var hidePreview = function () {};
  if (document.body.hasAttribute("data-previews") && window.fetch && window.DOMParser) {
    var pages = {}, popup = null, popupFor = null, showTimer = 0, hideTimer = 0, lastPointer = "mouse";
    var fetchPage = function (url) {
      if (!pages[url]) {
        pages[url] = fetch(url, { credentials: "same-origin" }).then(function (r) {
          if (!r.ok) throw new Error(r.status);
          return r.text();
        }).then(function (html) { return new DOMParser().parseFromString(html, "text/html"); });
        pages[url].catch(function () { delete pages[url]; });
      }
      return pages[url];
    };
    // The part of the page a link points to: the section of a #heading,
    // the block of a #^id, else the whole note.
    var excerpt = function (doc, hash) {
      var content = doc.querySelector(".note .content");
      if (!content) return null;
      var out = document.createDocumentFragment();
      var start = null;
      if (hash) {
        try { start = content.querySelector("#" + CSS.escape(decodeURIComponent(hash))); } catch (e) {}
      }
      if (start && /^H[1-6]$/.test(start.tagName) && start.parentNode === content) {
        var level = Number(start.tagName[1]);
        for (var n = start; n; n = n.nextElementSibling) {
          if (n !== start && /^H[1-6]$/.test(n.tagName) && Number(n.tagName[1]) <= level) break;
          out.appendChild(document.importNode(n, true));
        }
      } else if (start) {
        out.appendChild(document.importNode(start, true));
      } else {
        Array.prototype.forEach.call(content.children, function (c) { out.appendChild(document.importNode(c, true)); });
      }
      // Keep previews light: no players or frames, no duplicate ids.
      out.querySelectorAll("iframe, video, audio, script").forEach(function (x) { x.remove(); });
      out.querySelectorAll("[id]").forEach(function (x) { x.removeAttribute("id"); });
      return out;
    };
    var hide = hidePreview = function () {
      clearTimeout(showTimer); clearTimeout(hideTimer);
      if (popup) popup.hidden = true;
      popupFor = null;
    };
    var place = function (link) {
      var r = link.getBoundingClientRect(), vw = document.documentElement.clientWidth, vh = window.innerHeight;
      var w = Math.min(popup.offsetWidth, vw - 16);
      var left = Math.max(8, Math.min(r.left, vw - w - 8));
      var below = vh - r.bottom, above = r.top;
      popup.style.left = left + "px";
      popup.style.maxHeight = Math.max(160, Math.min(352, (below > above ? below : above) - 16)) + "px";
      if (below >= popup.offsetHeight + 12 || below > above) {
        popup.style.top = r.bottom + 6 + "px"; popup.style.bottom = "auto";
      } else {
        popup.style.top = "auto"; popup.style.bottom = vh - r.top + 6 + "px";
      }
    };
    var show = function (link) {
      var url = new URL(link.href, location.href);
      var page = url.pathname;
      if (!popup) {
        popup = el("div", "link-preview");
        popup.setAttribute("role", "dialog");
        popup.setAttribute("aria-label", tr("preview", "Preview"));
        popup.hidden = true;
        popup.addEventListener("mouseenter", function () { clearTimeout(hideTimer); });
        popup.addEventListener("mouseleave", function () { if (lastPointer === "mouse") hideTimer = setTimeout(hide, 250); });
        document.body.appendChild(popup);
      }
      popupFor = link;
      fetchPage(page).then(function (doc) {
        if (popupFor !== link) return;
        var body = excerpt(doc, url.hash.slice(1));
        if (!body) { hide(); return; }
        popup.textContent = "";
        var head = el("a", "link-preview-title", (doc.querySelector(".note h1") || {}).textContent || link.textContent);
        head.href = link.href;
        popup.appendChild(head);
        var inner = el("div", "content link-preview-body");
        inner.appendChild(body);
        popup.appendChild(inner);
        if (lastPointer !== "mouse") {
          var open = el("a", "link-preview-open", tr("open_note", "Open note →"));
          open.href = link.href;
          popup.appendChild(open);
        }
        renderMath(inner);
        popup.hidden = false;
        popup.scrollTop = 0;
        place(link);
      }).catch(hide);
    };
    var previewable = function (a) {
      if (!a || !a.matches("a.internal-link") || !a.closest(".content, .backlinks") || a.closest(".link-preview")) return false;
      var url = new URL(a.href, location.href);
      return url.origin === location.origin && url.pathname !== location.pathname && url.pathname.indexOf("/_assets/") !== 0;
    };
    document.addEventListener("pointerdown", function (e) {
      lastPointer = e.pointerType || "mouse";
      if (popup && !popup.hidden && !popup.contains(e.target) && e.target.closest("a") !== popupFor) hide();
    }, true);
    document.addEventListener("mouseover", function (e) {
      if (lastPointer !== "mouse") return;
      var a = e.target.closest && e.target.closest("a");
      if (!previewable(a)) return;
      clearTimeout(hideTimer);
      if (a === popupFor) return;
      clearTimeout(showTimer);
      showTimer = setTimeout(function () { show(a); }, 350);
    });
    document.addEventListener("mouseout", function (e) {
      var a = e.target.closest && e.target.closest("a");
      if (!previewable(a) || (e.relatedTarget && a.contains(e.relatedTarget))) return;
      clearTimeout(showTimer);
      if (popupFor) hideTimer = setTimeout(hide, 250);
    });
    // On touch screens the first tap previews, a second tap opens.
    document.addEventListener("click", function (e) {
      if (lastPointer === "mouse" || e.defaultPrevented || e.metaKey || e.ctrlKey || e.shiftKey) return;
      var a = e.target.closest && e.target.closest("a");
      if (!previewable(a) || (a === popupFor && popup && !popup.hidden)) return;
      e.preventDefault();
      hide();
      show(a);
    });
    document.addEventListener("keydown", function (e) { if (e.key === "Escape") hide(); });
    window.addEventListener("scroll", function () { if (popupFor && lastPointer === "mouse") hide(); }, { passive: true });
  }

  // ---- search ----
  var searchBtn = $(".search-toggle");
  if (searchBtn && window.fetch && window.HTMLDialogElement) {
    searchBtn.hidden = false;
    var dialog, input, list, engine, docs = {}, results = [], active = -1, filtering = false;

    var build = function () {
      dialog = el("dialog", "search-dialog");
      dialog.setAttribute("aria-label", tr("search", "Search"));
      var form = el("form");
      form.method = "dialog";
      input = el("input");
      input.type = "search";
      input.placeholder = tr("search_placeholder", "Search notes…");
      input.setAttribute("aria-label", tr("search_label", "Search notes"));
      input.autocomplete = "off";
      list = el("ul", "search-results");
      list.setAttribute("role", "listbox");
      form.appendChild(input);
      dialog.appendChild(form);
      dialog.appendChild(list);
      dialog.appendChild(el("p", "search-hint", tr("search_hint", "↑↓ to choose · Enter to open · Esc to close")));
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
          if (r) go(docs[r.id].url);
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
        if (filtering && d.tags && d.tags.length) {
          a.appendChild(el("span", "search-tags", d.tags.map(function (t) { return "#" + t; }).join(" ")));
        }
        li.appendChild(a);
        list.appendChild(li);
      });
      var sel = list.querySelector('[aria-selected="true"]');
      if (sel) sel.scrollIntoView({ block: "nearest" });
    };

    // "#tag" words filter the results to notes with a tag starting with
    // them (so #garden also finds #garden/soil); the other words search.
    // Tags alone list every note that has them.
    var run = function () {
      if (!engine) return; // still loading; ready() runs the query when done
      var q = input.value.trim();
      var tags = [], words = [];
      q.split(/\s+/).forEach(function (w) {
        if (/^#[^#]/.test(w)) tags.push(w.slice(1).toLowerCase()); else if (w && w !== "#") words.push(w);
      });
      var tagged = function (id) {
        var have = (docs[id].tags || []).map(function (t) { return t.toLowerCase(); });
        return tags.every(function (t) { return have.some(function (h) { return h.indexOf(t) === 0; }); });
      };
      filtering = tags.length > 0;
      if (words.length) {
        results = engine.search(words.join(" "), filtering ? { filter: function (r) { return tagged(r.id); } } : undefined).slice(0, 12);
      } else if (filtering) {
        results = Object.keys(docs).filter(tagged).sort(function (a, b) {
          return docs[a].title.localeCompare(docs[b].title);
        }).slice(0, 50).map(function (id) { return { id: id, terms: [] }; });
      } else {
        results = [];
      }
      active = results.length ? 0 : -1;
      if (q && !results.length) {
        list.textContent = "";
        list.appendChild(el("li", "search-empty", tr("search_empty", "No notes found.")));
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
        list.appendChild(el("li", "search-empty", tr("search_unavailable", "Search is not available.")));
      });
    };
    openSearch = open;
    searchBtn.addEventListener("click", function () { open(); });
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
      return { id: n.id, title: n.title, url: n.url, tag: n.tag, x: r * Math.cos(a), y: r * Math.sin(a), vx: 0, vy: 0, deg: 0 };
    });
    var byId = {};
    nodes.forEach(function (n) { byId[n.id] = n; });
    var links = data.edges.filter(function (e) { return byId[e.source] && byId[e.target] && e.source !== e.target; })
      .map(function (e) { byId[e.source].deg++; byId[e.target].deg++; return { s: byId[e.source], t: byId[e.target] }; });
    var current = byId[opts.current];
    var view = { x: 0, y: 0, k: 1 }, w = 0, h = 0, hover = null, alpha = 1, raf = 0, dragging = null;

    var distance = graphOptions.distance;
    function radius(n) { return graphOptions.size === "links" ? 3 + Math.min(6, Math.sqrt(n.deg) * 1.6) : 4; }

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
        f = (d - distance) / d * 0.06 * alpha;
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
        ctx.fillStyle = n === current || n === hover ? accent : muted;
        ctx.beginPath(); ctx.arc(p[0], p[1], r, 0, 6.2832);
        if (n.tag && n !== hover) { ctx.strokeStyle = accent; ctx.lineWidth = 1.5; ctx.stroke(); ctx.lineWidth = 1; }
        else ctx.fill();
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
      if (!moved) { var t = pick(ev); if (t && t !== current) go(t.url); }
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
    (opts.page ? pageHooks : onThemeChange).push(draw);
    // The note to highlight, after instant navigation.
    this.setCurrent = function (id) { current = byId[id]; draw(); };
    resize();
    fit();
    kick(Math.min(alpha, 0.05));
  }

  // Graph settings from the theme params (graph_orphans, graph_tags,
  // graph_node_size, graph_link_distance), written on <body>.
  var graphOptions = (function () {
    var o = {};
    try { o = JSON.parse(document.body.dataset.graph || "{}") || {}; } catch (e) {}
    var on = function (v, def) { return v == null || v === "" ? def : v === true || String(v).toLowerCase() === "true"; };
    var dist = Number(o.distance);
    return {
      orphans: on(o.orphans, true),
      tags: on(o.tags, false),
      size: String(o.size || "links").toLowerCase() === "links" ? "links" : "same",
      distance: dist > 0 ? Math.min(dist, 400) : 45
    };
  })();

  // The global graph's data with the settings applied: tags become nodes
  // linked to their notes (and nested tags to their parent), and notes
  // without any link are left out unless orphans are shown.
  function globalGraph(data) {
    var nodes = data.nodes.slice(), edges = data.edges.slice();
    if (graphOptions.tags && data.tags) {
      var known = {};
      data.tags.forEach(function (t) { known[t.name] = true; });
      data.tags.forEach(function (t) {
        nodes.push({ id: "#" + t.name, title: "#" + t.name, url: t.url, tag: true });
        var slash = t.name.lastIndexOf("/");
        if (slash > 0 && known[t.name.slice(0, slash)]) edges.push({ source: "#" + t.name, target: "#" + t.name.slice(0, slash) });
      });
      data.nodes.forEach(function (n) {
        (n.tags || []).forEach(function (t) { if (known[t]) edges.push({ source: n.id, target: "#" + t }); });
      });
    }
    if (!graphOptions.orphans) {
      var linked = {};
      edges.forEach(function (e) { if (e.source !== e.target) { linked[e.source] = true; linked[e.target] = true; } });
      nodes = nodes.filter(function (n) { return linked[n.id]; });
    }
    return { nodes: nodes, edges: edges };
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

  function initLocalGraph(page) {
    var local = $(".local-graph", page);
    if (!local || !noteId || !window.fetch || !window.HTMLCanvasElement) return;
    var id = noteId;
    getJSON("/graph.json").then(function (data) {
      var sub = neighborhood(data, id, 2);
      if (sub.nodes.length < 2 || !local.isConnected) return;
      local.hidden = false;
      new Graph($(".graph-canvas", local), sub, { current: id, labels: sub.nodes.length <= 6, page: true });
    }).catch(function () {});
  }

  var graphBtn = $(".graph-toggle");
  if (graphBtn && window.fetch && window.HTMLDialogElement && window.HTMLCanvasElement) {
    graphBtn.hidden = false;
    var gdialog = null, allGraph = null;
    onNavigate.push(function () { if (allGraph) allGraph.setCurrent(noteId); });
    graphBtn.addEventListener("click", function () {
      if (!gdialog) {
        gdialog = el("dialog", "graph-dialog");
        gdialog.setAttribute("aria-label", tr("graph_all", "Graph of all notes"));
        var close = el("button", "icon-button graph-close", "×");
        close.type = "button";
        close.setAttribute("aria-label", tr("close", "Close"));
        close.addEventListener("click", function () { gdialog.close(); });
        var area = el("div", "graph-canvas");
        gdialog.appendChild(close);
        gdialog.appendChild(area);
        gdialog.addEventListener("click", function (e) { if (e.target === gdialog) gdialog.close(); });
        document.body.appendChild(gdialog);
        gdialog.showModal();
        getJSON("/graph.json").then(function (data) {
          data = globalGraph(data);
          allGraph = new Graph(area, data, { current: noteId, labels: data.nodes.length <= 40 });
        }).catch(function () { area.textContent = tr("graph_unavailable", "The graph is not available."); });
        return;
      }
      gdialog.showModal();
    });
  }
  // ---- the parts of a page ----
  function initPage() {
    var page = $(".page") || document;
    noteId = document.body.dataset.note || null;
    pageHooks = [];
    initList();
    initTagTools();
    initAnchors(page);
    initCode(page);
    page.querySelectorAll(".link-unpublished").forEach(function (el) { el.title = tr("not_published", "Not published"); });
    renderMath(page);
    initDiagrams(page);
    initLocalGraph(page);
  }

  // ---- stacked pages: linked notes open side by side ----
  // On wide screens a link in a note opens its note in a pane to the right
  // of the one it is in, closing the panes after that. Panes slide over
  // each other, leaving a spine with the title of the ones underneath. The
  // open panes are in the address (?stack=/a/&stack=/b/), so back,
  // forward, reloads and shared links keep them.
  if (document.body.hasAttribute("data-stacked") && window.fetch && window.DOMParser && history.pushState) {
    var wide = window.matchMedia("(min-width: 821px)");
    var stackDocs = {}, stackToken = 0, stackHere = location.pathname;
    var fetchNote = function (path) {
      if (!stackDocs[path]) {
        stackDocs[path] = fetch(path, { credentials: "same-origin" }).then(function (r) {
          var type = r.headers.get("content-type") || "";
          if (!r.ok || type.indexOf("text/html") !== 0) throw new Error(r.status);
          return r.text();
        }).then(function (html) {
          var doc = new DOMParser().parseFromString(html, "text/html");
          if (!$("main .page .note", doc)) throw new Error("not a note");
          return doc;
        });
        stackDocs[path].catch(function () { delete stackDocs[path]; });
      }
      return stackDocs[path];
    };
    var stackPaths = function () { return new URLSearchParams(location.search).getAll("stack"); };
    var stackURL = function (paths) {
      var q = new URLSearchParams(location.search);
      q.delete("stack");
      var parts = paths.map(function (p) { return "stack=" + encodeURI(p).replace(/[&#+]/g, encodeURIComponent); });
      if (q.toString()) parts.unshift(q.toString());
      return location.pathname + (parts.length ? "?" + parts.join("&") : "");
    };
    var panes = function () { return Array.prototype.slice.call(document.querySelectorAll("main > .stack-pane")); };
    // The page and the open panes, left to right.
    var columns = function () { return [$("main > .page")].concat(panes()); };
    var paneWidth = function () { return $("main > .page").offsetWidth; };
    var spineWidth = function () { return parseFloat(css("--spine")) * parseFloat(getComputedStyle(root).fontSize) || 40; };

    // Show column i as far left as it slides, or, with whole, scroll just
    // enough to see all of it.
    var reveal = function (i, whole) {
      var main = $("main");
      var left = whole ? Math.max(main.scrollLeft, (i + 1) * paneWidth() - main.clientWidth) : i * (paneWidth() - spineWidth());
      main.scrollTo({ left: Math.max(0, left), behavior: "smooth" });
    };
    var scrollToId = function (col, hash) {
      if (!col || !hash) return;
      var t = null;
      try { t = col.querySelector('[id="' + CSS.escape(decodeURIComponent(hash.slice(1))) + '"]'); } catch (e) {}
      if (t) col.scrollTop += t.getBoundingClientRect().top - col.getBoundingClientRect().top - 16;
    };
    // A column with the next one slid over it shows its spine.
    var markCovered = function () {
      var cols = columns(), spine = spineWidth();
      cols.forEach(function (c, i) {
        var next = cols[i + 1];
        c.classList.toggle("covered", !!next && document.body.classList.contains("stacking") &&
          next.getBoundingClientRect().left - c.getBoundingClientRect().left < spine * 4);
      });
    };
    var addSpine = function (col, title) {
      if ($(":scope > .stack-spine", col)) return;
      var spine = el("button", "stack-spine");
      spine.type = "button";
      spine.appendChild(el("span", null, title));
      spine.tabIndex = -1; // keyboard users reach the pane itself
      spine.addEventListener("click", function () { reveal(columns().indexOf(col)); });
      col.insertBefore(spine, col.firstChild);
    };
    var makePane = function (path, doc) {
      var title = ($(".note h1", doc) || {}).textContent || path;
      var pane = el("section", "stack-pane");
      pane.dataset.path = path;
      pane.tabIndex = -1;
      pane.setAttribute("aria-label", title);
      var close = el("button", "icon-button stack-close", "×");
      close.type = "button";
      close.title = tr("close", "Close");
      close.setAttribute("aria-label", close.title);
      close.addEventListener("click", function () { openStack(stackPaths().slice(0, panes().indexOf(pane)), "", true); });
      var body = document.importNode($("main .page .page-main", doc), true);
      // Links to sections of the pane's own note point to the note.
      body.querySelectorAll('a[href^="#"]').forEach(function (a) { a.setAttribute("href", path + a.getAttribute("href")); });
      body.querySelectorAll("script").forEach(function (s) { s.remove(); });
      body.querySelectorAll(".link-unpublished").forEach(function (x) { x.title = tr("not_published", "Not published"); });
      pane.appendChild(close);
      pane.appendChild(body);
      addSpine(pane, title);
      initCode(body);
      addAssets(doc).then(function () { renderMath(body); initDiagrams(body); });
      return pane;
    };
    var setStacking = function () {
      var page = $("main > .page");
      var on = panes().length > 0;
      document.body.classList.toggle("stacking", on);
      if (on) addSpine(page, ($("h1", page) || {}).textContent || document.title);
      markCovered();
    };
    // openStack shows the panes for paths: the open ones are kept while
    // they match, the rest closed and the missing ones loaded. push
    // records it in history and brings the last pane into view.
    var openStack = function (paths, hash, push) {
      var token = ++stackToken;
      if (push) history.pushState(history.state, "", stackURL(paths));
      var have = panes(), keep = 0;
      while (keep < have.length && keep < paths.length && have[keep].dataset.path === paths[keep]) keep++;
      have.slice(keep).forEach(function (p) { p.remove(); });
      setStacking();
      var want = paths.slice(keep);
      if (!want.length) return;
      hidePreview();
      Promise.all(want.map(fetchNote)).then(function (docs) {
        if (token !== stackToken) return;
        var last = null;
        docs.forEach(function (doc, i) { last = makePane(want[i], doc); $("main").appendChild(last); });
        setStacking();
        if (!push) return;
        reveal(paths.length, true);
        scrollToId(last, hash);
        last.focus({ preventScroll: true });
      }).catch(function () {
        // A pane that isn't a note (any more): leave it out.
        if (token !== stackToken) return;
        history.replaceState(history.state, "", stackURL(paths.slice(0, keep)));
        openStack(paths.slice(0, keep), "", false);
      });
    };
    var syncStack = function () { openStack(stackPaths(), "", false); };

    document.addEventListener("click", function (e) {
      if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
      // Sections of a pane's note (footnotes, [[#heading]]) stay in the pane.
      var own = e.target.closest && e.target.closest("main > .stack-pane a[href]");
      if (own && own.hash && own.pathname === own.closest(".stack-pane").dataset.path && own.origin === location.origin) {
        e.preventDefault();
        scrollToId(own.closest(".stack-pane"), own.hash);
        return;
      }
      if (!wide.matches) return;
      var a = e.target.closest && e.target.closest("a.internal-link[href]");
      if (!a || !a.closest(".content, .backlinks")) return;
      var col = a.closest("main > .page, main > .stack-pane");
      if (!col) return;
      var url = new URL(a.href, location.href);
      if (url.origin !== location.origin || /^\/_(assets|kvist)\//.test(url.pathname) || /\.(?!html?$)[a-z0-9]+$/i.test(url.pathname)) return;
      e.preventDefault();
      var cols = columns();
      var at = url.pathname === location.pathname ? 0 : panes().map(function (p) { return p.dataset.path; }).indexOf(url.pathname) + 1;
      if (at > 0 || url.pathname === location.pathname) { // open already: show it
        reveal(at);
        scrollToId(cols[at], url.hash);
        return;
      }
      // Only notes stack; other pages open the usual way.
      var paths = stackPaths().slice(0, cols.indexOf(col)).concat(url.pathname);
      fetchNote(url.pathname).then(function () { openStack(paths, url.hash, true); }, function () { go(url.href); });
    });
    // Back and forward between stacks of the same page; instant navigation
    // handles other pages and calls syncStack after.
    window.addEventListener("popstate", function () { if (location.pathname === stackHere) syncStack(); });
    onNavigate.push(function () { stackHere = location.pathname; syncStack(); });
    $("main").addEventListener("scroll", markCovered, { passive: true });
    window.addEventListener("resize", markCovered);
    syncStack();
  }

  // ---- instant navigation ----
  // Links to pages of the site load in place: the page, the list (unless
  // it shows the same notes) and the head's metadata are swapped, the
  // menu keeps its scroll position and open folders, and search and the
  // graph stay loaded. Anything unexpected falls back to a normal load.
  if (document.body.hasAttribute("data-instant") && window.fetch && window.DOMParser && history.pushState) {
    var navToken = 0, here = location.pathname, progress = null, progressTimer = 0;
    var live = el("p", "visually-hidden");
    live.setAttribute("aria-live", "polite");
    document.body.appendChild(live);
    history.scrollRestoration = "manual";
    history.replaceState({ y: window.scrollY }, "");
    // Remember the scroll position, for going back.
    var saveTimer = 0;
    window.addEventListener("scroll", function () {
      clearTimeout(saveTimer);
      saveTimer = setTimeout(function () { history.replaceState({ y: window.scrollY }, ""); }, 120);
    }, { passive: true });

    var headSel = 'meta[name="description"], meta[name="robots"], link[rel="canonical"], meta[property^="og:"], meta[property^="article:"], meta[name^="twitter:"]';
    // Mark the current page in the menu as the new page does, and open
    // the folders it opens. Folders the reader opened stay open.
    var syncNav = function (doc) {
      var nav = $(".nav-pane"), next = $(".nav-pane", doc);
      if (!nav || !next) return;
      var marks = {}, open = {};
      next.querySelectorAll("a[href]").forEach(function (a) { marks[a.getAttribute("href")] = a.getAttribute("aria-current"); });
      next.querySelectorAll("details[open] > summary > a").forEach(function (a) { open[a.getAttribute("href")] = true; });
      nav.querySelectorAll("a[href]").forEach(function (a) {
        var m = marks[a.getAttribute("href")];
        if (m) a.setAttribute("aria-current", m); else a.removeAttribute("aria-current");
      });
      nav.querySelectorAll("details > summary > a").forEach(function (a) { if (open[a.getAttribute("href")]) a.parentNode.parentNode.open = true; });
    };
    // Keep the list pane if it lists the same notes, so it keeps its
    // scroll position and filter; else take the new one.
    var swapList = function (doc) {
      var list = $(".list-pane"), next = $(".list-pane", doc);
      var key = function (p) { return Array.prototype.map.call(p.querySelectorAll("#list-title, .list-items a"), function (a) { return a.getAttribute("href") || a.textContent; }).join("\n"); };
      if (list && next && key(list) === key(next)) {
        var marks = {};
        next.querySelectorAll(".list-items a").forEach(function (a) { marks[a.getAttribute("href")] = a.getAttribute("aria-current"); });
        list.querySelectorAll(".list-items a").forEach(function (a) {
          var m = marks[a.getAttribute("href")];
          if (m) a.setAttribute("aria-current", m); else a.removeAttribute("aria-current");
        });
        return;
      }
      if (list) list.remove();
      if (next) $(".shell").insertBefore(document.importNode(next, true), $("main"));
    };
    // Scripts in a note's own HTML run as they would on a normal load.
    var runScripts = function (scope) {
      scope.querySelectorAll("script").forEach(function (old) {
        var s = document.createElement("script");
        Array.prototype.forEach.call(old.attributes, function (a) { s.setAttribute(a.name, a.value); });
        s.textContent = old.textContent;
        old.replaceWith(s);
      });
    };
    var swap = function (doc) {
      document.title = doc.title;
      document.head.querySelectorAll(headSel).forEach(function (n) { n.remove(); });
      var anchor = document.head.querySelector('meta[name="color-scheme"]');
      doc.head.querySelectorAll(headSel).forEach(function (n) { document.head.insertBefore(document.importNode(n, true), anchor); });
      var loaded = addAssets(doc);
      var nav = document.body.classList.contains("nav-open");
      document.body.className = doc.body.className;
      if (doc.body.dataset.note) document.body.dataset.note = doc.body.dataset.note; else delete document.body.dataset.note;
      if (nav && menuToggle) menuToggle.setAttribute("aria-expanded", "false");
      syncNav(doc);
      swapList(doc);
      var page = $(".page"), next = $("main .page", doc);
      var fresh = document.importNode(next, true);
      page.replaceWith(fresh);
      runScripts(fresh);
      initPage();
      loaded.then(function () { if ($(".page") === fresh) { renderMath(fresh); initDiagrams(fresh); } });
      onNavigate.forEach(function (f) { f(); });
      live.textContent = doc.title;
    };
    var showProgress = function (on) {
      clearTimeout(progressTimer);
      if (on) {
        progressTimer = setTimeout(function () {
          if (!progress) { progress = el("div", "nav-progress"); document.body.appendChild(progress); }
          progress.hidden = false;
        }, 150);
      } else if (progress) {
        progress.hidden = true;
      }
    };
    var scrollTo = function (hash, y) {
      var target = null;
      if (hash) { try { target = document.getElementById(decodeURIComponent(hash.slice(1))); } catch (e) {} }
      if (target) target.scrollIntoView();
      else window.scrollTo(0, y || 0);
    };
    var navigate = function (href, push, y) {
      var url = new URL(href, location.href);
      var token = ++navToken;
      hidePreview();
      document.querySelectorAll("dialog[open]").forEach(function (d) { d.close(); });
      showProgress(true);
      fetch(url.pathname + url.search, { credentials: "same-origin" }).then(function (r) {
        var type = r.headers.get("content-type") || "";
        if ((!r.ok && r.status !== 404) || type.indexOf("text/html") !== 0) throw new Error(r.status);
        return r.text().then(function (html) { return { html: html, url: r.url }; });
      }).then(function (res) {
        if (token !== navToken) return;
        var doc = new DOMParser().parseFromString(res.html, "text/html");
        if (!$("main .page", doc) || !$(".page")) throw new Error("not a page");
        var final = new URL(res.url || url.href);
        final.hash = url.hash;
        if (push) history.pushState({ y: 0 }, "", final.href);
        else if (final.pathname !== location.pathname) history.replaceState(history.state, "", final.href);
        here = location.pathname;
        swap(doc);
        showProgress(false);
        scrollTo(url.hash, y);
        var main = $("main");
        main.setAttribute("tabindex", "-1");
        main.focus({ preventScroll: true });
        if (window.goatcounter && window.goatcounter.count) window.goatcounter.count({ path: location.pathname + location.search });
      }).catch(function () {
        if (token === navToken) location.href = url.href;
      });
    };
    go = function (href) { navigate(href, true); };

    document.addEventListener("click", function (e) {
      if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
      var a = e.target.closest && e.target.closest("a[href]");
      if (!a || (a.target && a.target !== "_self") || a.hasAttribute("download")) return;
      var url = new URL(a.href, location.href);
      if (url.origin !== location.origin || /^\/_(assets|kvist)\//.test(url.pathname) || /\.(?!html?$)[a-z0-9]+$/i.test(url.pathname)) return;
      if (url.pathname === location.pathname && url.search === location.search && url.hash) return; // a section of this page
      e.preventDefault();
      if (url.href === location.href) { scrollTo("", 0); return; }
      navigate(url.href, true);
    });
    window.addEventListener("popstate", function (e) {
      if (location.pathname === here) return; // only the #section changed
      navigate(location.href, false, e.state && e.state.y);
    });
  }

  initPage();
})();
