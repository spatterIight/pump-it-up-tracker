// PIU Tracker: small progressive enhancements. Every page works without it.
(function () {
  "use strict";

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

  // ---- Song library: search, mode filter, sort --------------------------------

  function songLibrary() {
    var grid = document.querySelector("[data-grid]");
    if (!grid) return;
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
            // Cleared songs first; grades only compare within a version, so
            // the newest version's come first.
            return (num(b, "grade") > 0) - (num(a, "grade") > 0) ||
              num(b, "version") - num(a, "version") ||
              num(b, "grade") - num(a, "grade") ||
              num(b, "level") - num(a, "level");
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
    each(sort.options, function (option) {
      if (option.value === savedSort) sort.value = savedSort;
    });
    sort.addEventListener("change", function () {
      store("piu-sort", sort.value);
      order();
    });
    order();
    filter();
  }

  // ---- Song page: chart tabs ----------------------------------------------------

  // A chart lineage's tab also answers to the keys of its older charts.
  function resolve(tabs, key) {
    var found = null;
    each(tabs, function (tab) {
      var aliases = (tab.dataset.aliases || "").split(" ");
      if (!found && (tab.dataset.tab === key || aliases.indexOf(key) >= 0)) found = tab.dataset.tab;
    });
    return found;
  }

  function activate(key, remember) {
    var tabs = document.querySelectorAll("[data-tab]");
    key = resolve(tabs, key);
    if (!key) return;
    each(tabs, function (tab) {
      var on = tab.dataset.tab === key;
      tab.classList.toggle("is-active", on);
      tab.setAttribute("aria-selected", String(on));
    });
    each(document.querySelectorAll("[data-panel]"), function (panel) {
      panel.classList.toggle("is-active", panel.dataset.panel === key);
    });
    if (remember) history.replaceState(history.state, "", "#" + key);
  }

  function chartTabs() {
    each(document.querySelectorAll("[data-tab]"), function (tab) {
      tab.addEventListener("click", function (e) {
        e.preventDefault();
        activate(tab.dataset.tab, true);
      });
    });
    if (location.hash) activate(location.hash.slice(1), false);
  }

  // Following a link to another chart of the same song only changes the hash.
  window.addEventListener("hashchange", function () {
    activate(location.hash.slice(1), false);
  });

  // ---- Song page: metric switch ---------------------------------------------------

  function metricSwitch() {
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
  }

  // ---- Chart tooltips ---------------------------------------------------------------

  var tip = null;
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

  function tooltips() {
    tip = document.querySelector(".tooltip");
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
  }

  document.addEventListener("click", hideTip);
  window.addEventListener("scroll", hideTip, { passive: true });

  // enhance sets up a page that has just been loaded or swapped in.
  function enhance() {
    songLibrary();
    chartTabs();
    metricSwitch();
    tooltips();
  }

  // ---- Page navigation --------------------------------------------------------------

  // A link to another page fetches it and swaps it in, instead of the browser
  // loading it, which blanks the screen in between. Back and forward bring back
  // pages as they were left, filters and scroll position included. Anything
  // the swap can't do, such as a link opened in a new tab, a failed fetch or a
  // new release of the app, is left to the browser.

  var root = document.documentElement;
  var base = root.dataset.base || "";
  // Pages left behind, by history entry, so that going back to one is instant.
  var snapshots = new Map();
  var keep = 10;
  // The page on screen: its history entry and its path.
  var shown = null;
  // The fetch of the next page, if one is under way.
  var loading = null;

  function newID() {
    return Date.now().toString(36) + Math.random().toString(36).slice(2);
  }

  function pathOf(url) {
    return url.pathname + url.search;
  }

  // entryID names the current history entry, so a page can be kept for it.
  function entryID() {
    if (!history.state || !history.state.page) history.replaceState({ page: newID() }, "");
    return history.state.page;
  }

  // isPage reports whether a URL is one of the app's pages rather than an
  // image, a static file or the API.
  function isPage(url) {
    if (url.origin !== location.origin) return false;
    var p = url.pathname;
    if (p !== base && p.indexOf(base + "/") !== 0) return false;
    p = p.slice(base.length);
    return !/^\/(static|art|api)\//.test(p) && p !== "/healthz";
  }

  // assets lists a page's stylesheets and scripts, which a new release changes.
  function assets(doc) {
    return Array.prototype.map.call(doc.querySelectorAll("link[rel=stylesheet], script[src]"), function (el) {
      return el.getAttribute("href") || el.getAttribute("src");
    }).join(" ");
  }

  // leave keeps the page on screen for when history comes back to it.
  function leave() {
    snapshots.delete(shown.id);
    snapshots.set(shown.id, { body: document.body, title: document.title, path: shown.path, scroll: window.scrollY });
    while (snapshots.size > keep) snapshots.delete(snapshots.keys().next().value);
  }

  function show(body, title, id) {
    hideTip();
    document.title = title;
    document.body = body;
    // A page kept from before has its own tooltip; a new one sets it up.
    tip = document.querySelector(".tooltip");
    shown = { id: id, path: pathOf(location) };
  }

  function stop() {
    if (loading) loading.abort();
  }

  // load fetches a page and hands it to done, or has the browser load it if
  // it can't be swapped in. Another load, or going back or forward, cancels it.
  function load(url, done) {
    stop();
    var controller = new AbortController();
    var slow = setTimeout(function () { root.classList.add("is-loading"); }, 150);
    var request = loading = {
      // settle ends the request, and reports whether it was still current.
      settle: function () {
        if (loading !== request) return false;
        loading = null;
        clearTimeout(slow);
        root.classList.remove("is-loading");
        return true;
      },
      abort: function () {
        request.settle();
        controller.abort();
      }
    };
    fetch(url.href, { signal: controller.signal, headers: { Accept: "text/html" } })
      .then(function (res) {
        var to = new URL(res.url);
        to.hash = url.hash;
        if (!/^text\/html/.test(res.headers.get("Content-Type") || "") || !isPage(to)) throw new Error("not a page");
        return res.text().then(function (html) {
          var doc = new DOMParser().parseFromString(html, "text/html");
          if (assets(doc) !== assets(document)) throw new Error("the app has been updated");
          return { doc: doc, to: to };
        });
      })
      .then(function (page) {
        if (request.settle()) done(page.doc, page.to);
      }, function () {
        if (!request.settle()) return;
        // Assigning the URL already shown would only jump to its fragment.
        if (url.href === location.href) location.reload();
        else location.assign(url.href);
      });
  }

  // visit goes to a page as following a link to it would.
  function visit(url) {
    load(url, function (doc, to) {
      var id;
      if (pathOf(to) === pathOf(location)) {
        // A link to the current page reloads it in place, as the browser would.
        id = entryID();
        history.replaceState({ page: id }, "", to.href);
      } else {
        leave();
        id = newID();
        // Pushed before the swap, so that the browser keeps the scroll
        // position of the page being left.
        history.pushState({ page: id }, "", to.href);
      }
      show(doc.body, doc.title, id);
      enhance();
      var target = to.hash && document.getElementById(decodeURIComponent(to.hash.slice(1)));
      if (target) target.scrollIntoView();
      else window.scrollTo(0, 0);
    });
  }

  function navigate(e) {
    if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
    var link = e.target.closest && e.target.closest("a[href]");
    if (!(link instanceof HTMLAnchorElement) || (link.target && link.target !== "_self") || link.hasAttribute("download")) return;
    var url = new URL(link.href);
    if (!isPage(url)) return;
    // A link to a part of the page on screen is left to the browser.
    if (url.hash && pathOf(url) === pathOf(location)) return;
    e.preventDefault();
    // Following such a link added a history entry, which the page is now
    // kept for.
    if (pathOf(location) === shown.path) shown.id = entryID();
    visit(url);
  }

  function traverse(e) {
    var path = pathOf(location);
    stop();
    // Only the part of the page changed, which hashchange takes care of.
    if (path === shown.path) return;
    leave();
    var id = e.state && e.state.page;
    var snapshot = id && snapshots.get(id);
    if (snapshot && snapshot.path === path) {
      show(snapshot.body, snapshot.title, id);
      window.scrollTo(0, snapshot.scroll);
      return;
    }
    load(new URL(location.href), function (doc) {
      show(doc.body, doc.title, entryID());
      enhance();
    });
  }

  enhance();
  if (window.fetch && window.AbortController && window.DOMParser && history.pushState && Element.prototype.closest) {
    shown = { id: entryID(), path: pathOf(location) };
    document.addEventListener("click", navigate);
    window.addEventListener("popstate", traverse);
  }
})();
