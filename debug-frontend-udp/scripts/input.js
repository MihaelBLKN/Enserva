(function () {
  const pressedKeys = new Set();

  function bindKeyboard() {
    window.addEventListener("keydown", (event) => {
      pressedKeys.add(event.key.toLowerCase());
    });

    window.addEventListener("keyup", (event) => {
      pressedKeys.delete(event.key.toLowerCase());
    });
  }

  function getMovementVector() {
    let x = 0;
    let y = 0;

    if (pressedKeys.has("arrowleft") || pressedKeys.has("a")) {
      x -= 1;
    }
    if (pressedKeys.has("arrowright") || pressedKeys.has("d")) {
      x += 1;
    }
    if (pressedKeys.has("arrowup") || pressedKeys.has("w")) {
      y -= 1;
    }
    if (pressedKeys.has("arrowdown") || pressedKeys.has("s")) {
      y += 1;
    }

    return { x, y };
  }

  window.EnservaDebug = {
    ...window.EnservaDebug,
    bindKeyboard,
    getMovementVector
  };
}());
