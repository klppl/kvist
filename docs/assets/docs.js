// kvist docs: theme toggle, mobile menu, copy buttons, heading anchors and
// the "On this page" list. Every page works without it.
(function () {
  "use strict";
  var root = document.documentElement;
  var $ = function (s, el) { return (el || document).querySelector(s); };
  var $$ = function (s, el) { return Array.prototype.slice.call((el || document).querySelectorAll(s)); };

  var toggle = $(".theme-toggle");
  if (toggle) {
    toggle.hidden = false;
    var order = ["", "light", "dark"];
    var label = function () { toggle.title = "Theme: " + (root.dataset.theme || "system"); };
    label();
    toggle.addEventListener("click", function () {
      var next = order[(order.indexOf(root.dataset.theme || "") + 1) % order.length];
      if (next) root.dataset.theme = next; else delete root.dataset.theme;
      try { next ? localStorage.setItem("kvist-docs-theme", next) : localStorage.removeItem("kvist-docs-theme"); } catch (e) {}
      label();
    });
  }

  var menu = $(".menu-toggle");
  if (menu) {
    menu.hidden = false;
    menu.addEventListener("click", function () {
      var open = document.body.classList.toggle("nav-open");
      menu.setAttribute("aria-expanded", open ? "true" : "false");
    });
  }

  $$("pre").forEach(function (pre) {
    if (!navigator.clipboard) return;
    var b = document.createElement("button");
    b.type = "button";
    b.className = "copy";
    b.textContent = "Copy";
    b.addEventListener("click", function () {
      var code = $("code", pre) || pre;
      navigator.clipboard.writeText(code.innerText.replace(/\n$/, "")).then(function () {
        b.textContent = "Copied";
        setTimeout(function () { b.textContent = "Copy"; }, 1500);
      });
    });
    pre.appendChild(b);
  });

  var article = $("article");
  if (!article) return;
  $$("h2[id], h3[id]", article).forEach(function (h) {
    var a = document.createElement("a");
    a.className = "anchor";
    a.href = "#" + h.id;
    a.setAttribute("aria-label", "Link to this section");
    a.textContent = "#";
    h.appendChild(a);
  });

  var heads = $$("h2[id]", article);
  var toc = $(".toc");
  if (!toc || heads.length < 3) return;
  var nav = document.createElement("nav");
  nav.setAttribute("aria-label", "On this page");
  nav.innerHTML = "<h2>On this page</h2>";
  var ul = document.createElement("ul");
  var links = heads.map(function (h) {
    var li = document.createElement("li");
    var a = document.createElement("a");
    a.href = "#" + h.id;
    a.textContent = h.firstChild.textContent.trim();
    li.appendChild(a);
    ul.appendChild(li);
    return a;
  });
  nav.appendChild(ul);
  toc.appendChild(nav);
  $(".page").classList.add("has-toc");

  if ("IntersectionObserver" in window) {
    var current = null;
    var io = new IntersectionObserver(function (entries) {
      entries.forEach(function (e) {
        if (e.isIntersecting) current = e.target;
      });
      links.forEach(function (a, i) { a.classList.toggle("active", heads[i] === current); });
    }, { rootMargin: "0px 0px -70% 0px" });
    heads.forEach(function (h) { io.observe(h); });
  }
})();
