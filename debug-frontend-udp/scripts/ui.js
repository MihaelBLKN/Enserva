(function () {
  function createStatusView(element) {
    return {
      setStatus(type, text) {
        element.textContent = text;
        element.className = `status status--${type}`;
      }
    };
  }

  window.EnservaDebug = {
    ...window.EnservaDebug,
    createStatusView
  };
}());
