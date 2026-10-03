// kvist garden theme: progressive enhancements. Pages work without this.
(function () {
  "use strict";
  var root = document.documentElement;

  // Color theme: system → light → dark → system.
  var toggle = document.querySelector(".theme-toggle");
  if (toggle) {
    toggle.hidden = false;
    var order = ["", "light", "dark"];
    var label = function () {
      var t = root.dataset.theme || "";
      toggle.title = "Theme: " + (t || "system");
    };
    label();
    toggle.addEventListener("click", function () {
      var next = order[(order.indexOf(root.dataset.theme || "") + 1) % order.length];
      if (next) root.dataset.theme = next; else delete root.dataset.theme;
      try { next ? localStorage.setItem("kvist-theme", next) : localStorage.removeItem("kvist-theme"); } catch (e) {}
      label();
    });
  }

  // Folder tree on small screens.
  var treeToggle = document.querySelector(".tree-toggle");
  if (treeToggle && document.querySelector(".sidebar-left")) {
    treeToggle.hidden = false;
    treeToggle.addEventListener("click", function () {
      var open = document.body.classList.toggle("tree-open");
      treeToggle.setAttribute("aria-expanded", open ? "true" : "false");
    });
  }

  // Unpublished links: explain on hover without saying why.
  document.querySelectorAll(".link-unpublished").forEach(function (el) {
    el.title = "Not published";
  });
})();
