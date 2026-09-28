// Runs before first paint so the chosen theme applies without a flash.
(function () {
  var root = document.documentElement;
  root.classList.add("js");
  try {
    var theme = localStorage.getItem("piu-theme");
    if (theme === "light" || theme === "dark") root.dataset.theme = theme;
  } catch (e) {}
})();
