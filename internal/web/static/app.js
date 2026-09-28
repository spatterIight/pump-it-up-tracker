// PIU Tracker: small progressive enhancements. Every page works without it.
(function () {
  "use strict";

  var root = document.documentElement;

  function each(list, fn) {
    Array.prototype.forEach.call(list, fn);
  }

  function store(key, value) {
    try {
      if (value === undefined) return localStorage.getItem(key);
      localStorage.setItem(key, value);
    } catch (e) {
      return null;
    }
  }

  // ---- Theme ----------------------------------------------------------------

  var toggle = document.querySelector(".theme-toggle");
  if (toggle) {
    toggle.addEventListener("click", function () {
      var current = root.dataset.theme ||
        (window.matchMedia("(prefers-color-scheme: light)").matches ? "light" : "dark");
      var next = current === "light" ? "dark" : "light";
      root.dataset.theme = next;
      store("piu-theme", next);
    });
  }

  // ---- Song library: search, mode filter, sort --------------------------------

  var grid = document.querySelector("[data-grid]");
  if (grid) {
    var cards = Array.prototype.slice.call(grid.querySelectorAll(".card"));
    var search = document.querySelector("input[data-search]");
    var sort = document.querySelector("select[data-sort]");
    var count = document.querySelector("[data-count]");
    var empty = document.querySelector("[data-empty]");
    var chips = document.querySelectorAll(".chip[data-mode]");
    var mode = "";

    var filter = function () {
      var q = search.value.trim().toLowerCase();
      var visible = 0;
      cards.forEach(function (card) {
        var modes = card.dataset.modes.split(" ");
        var modeOK = !mode ||
          (mode === "perf" ? modes.indexOf("sp") >= 0 || modes.indexOf("dp") >= 0 : modes.indexOf(mode) >= 0);
        var textOK = !q || card.dataset.search.indexOf(q) >= 0;
        card.hidden = !(modeOK && textOK);
        if (!card.hidden) visible++;
      });
      count.textContent = visible;
      empty.hidden = visible > 0;
    };

    var num = function (card, key) {
      return parseFloat(card.dataset[key]) || 0;
    };
    var order = function () {
      var key = sort.value;
      cards.slice().sort(function (a, b) {
        switch (key) {
          case "title":
            return a.dataset.title.localeCompare(b.dataset.title, undefined, { sensitivity: "base" });
          case "level":
            return num(b, "level") - num(a, "level") || num(b, "last") - num(a, "last");
          case "grade":
            return num(b, "grade") - num(a, "grade") || num(b, "level") - num(a, "level");
          case "plays":
            return num(b, "plays") - num(a, "plays") || num(b, "last") - num(a, "last");
          default:
            return num(b, "last") - num(a, "last") || a.dataset.title.localeCompare(b.dataset.title);
        }
      }).forEach(function (card) {
        grid.appendChild(card);
      });
    };

    each(chips, function (chip) {
      chip.addEventListener("click", function () {
        mode = chip.dataset.mode;
        each(chips, function (c) {
          c.setAttribute("aria-pressed", String(c === chip));
        });
        filter();
      });
    });
    search.addEventListener("input", filter);
    var savedSort = store("piu-sort");
    if (savedSort && sort.querySelector('option[value="' + savedSort + '"]')) sort.value = savedSort;
    sort.addEventListener("change", function () {
      store("piu-sort", sort.value);
      order();
    });
    order();
    filter();
  }

  // ---- Song page: chart tabs ----------------------------------------------------

  var tabs = document.querySelectorAll("[data-tab]");
  var activate = function (key, remember) {
    var found = false;
    each(tabs, function (tab) {
      found = found || tab.dataset.tab === key;
    });
    if (!found) return;
    each(tabs, function (tab) {
      var on = tab.dataset.tab === key;
      tab.classList.toggle("is-active", on);
      tab.setAttribute("aria-selected", String(on));
    });
    each(document.querySelectorAll("[data-panel]"), function (panel) {
      panel.classList.toggle("is-active", panel.dataset.panel === key);
    });
    if (remember) history.replaceState(null, "", "#" + key);
  };
  each(tabs, function (tab) {
    tab.addEventListener("click", function (e) {
      e.preventDefault();
      activate(tab.dataset.tab, true);
    });
  });
  if (tabs.length && location.hash) activate(location.hash.slice(1), false);

  // ---- Song page: metric switch ---------------------------------------------------

  each(document.querySelectorAll(".graph-card"), function (card) {
    var buttons = card.querySelectorAll("[data-metric]");
    each(buttons, function (button) {
      button.addEventListener("click", function () {
        each(buttons, function (b) {
          b.setAttribute("aria-pressed", String(b === button));
        });
        each(card.querySelectorAll("[data-graph]"), function (graph) {
          graph.classList.toggle("is-active", graph.dataset.graph === button.dataset.metric);
        });
        hideTip();
      });
    });
  });

  // ---- Chart tooltips ---------------------------------------------------------------

  var tip = document.querySelector(".tooltip");
  var current = null;

  function hideTip() {
    if (!tip) return;
    tip.hidden = true;
    if (current) current.classList.remove("is-hover");
    current = null;
  }

  function showTip(point) {
    if (!tip) return;
    if (current) current.classList.remove("is-hover");
    current = point;
    point.classList.add("is-hover");
    var lines = point.dataset.tip.split("\n");
    tip.textContent = "";
    var strong = document.createElement("strong");
    strong.textContent = lines[1] || "";
    tip.appendChild(strong);
    [lines[0]].concat(lines.slice(2)).forEach(function (line) {
      var span = document.createElement("span");
      span.textContent = line;
      tip.appendChild(span);
    });
    tip.hidden = false;
    var dot = point.querySelector(".pt-dot").getBoundingClientRect();
    var x = dot.left + dot.width / 2;
    var half = tip.offsetWidth / 2 + 8;
    x = Math.max(half, Math.min(window.innerWidth - half, x));
    var below = dot.top < tip.offsetHeight + 24;
    tip.style.left = x + "px";
    tip.style.top = (below ? dot.bottom + tip.offsetHeight + 28 : dot.top) + "px";
  }

  each(document.querySelectorAll(".pt"), function (point) {
    // The SVG <title> is for screen readers without JavaScript; with the
    // custom tooltip it would show a second, native one.
    var title = point.querySelector("title");
    if (title) {
      point.setAttribute("aria-label", title.textContent);
      point.removeChild(title);
    }
    point.addEventListener("mouseenter", function () { showTip(point); });
    point.addEventListener("mouseleave", hideTip);
    point.addEventListener("focus", function () { showTip(point); });
    point.addEventListener("blur", hideTip);
    point.addEventListener("click", function (e) {
      e.stopPropagation();
      if (current === point) hideTip(); else showTip(point);
    });
  });
  document.addEventListener("click", hideTip);
  window.addEventListener("scroll", hideTip, { passive: true });
})();
