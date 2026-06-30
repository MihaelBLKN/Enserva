(function () {
  var root = document.documentElement;

  function applyScheme() {
    root.setAttribute("data-md-color-scheme", "enserva");
    root.setAttribute("data-md-color-primary", "black");
    root.setAttribute("data-md-color-accent", "indigo");
  }

  applyScheme();

  try {
    localStorage.setItem("__palette", JSON.stringify({ index: 0 }));
  } catch (e) {}

  document.addEventListener("DOMContentLoaded", applyScheme);
})();
