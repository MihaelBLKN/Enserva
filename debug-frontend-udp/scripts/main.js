(function () {
  const {
    applySnapshot,
    bindKeyboard,
    createGameSocket,
    createGameState,
    createRenderer,
    createStatusView,
    getMovementVector,
    predictLocalInput,
    updateInterpolatedRemotePlayers
  } = window.EnservaDebug;

  const canvas = document.querySelector("#game");
  const statusEl = document.querySelector("#status");

  const state = createGameState(canvas);
  const renderer = createRenderer(canvas);
  const statusView = createStatusView(statusEl);
  const socket = createGameSocket({
    onPlayers(snapshot) {
      applySnapshot(state, snapshot, performance.now());
    },
    onStatus(type, text) {
      statusView.setStatus(type, text);
    }
  });

  bindKeyboard();

  window.setInterval(() => {
    const input = getMovementVector();
    const sequence = socket.sendInput(input);
    predictLocalInput(state, sequence, input);
  }, 1000 / 60);

  function loop() {
    updateInterpolatedRemotePlayers(state, performance.now());
    renderer.draw({
      localPlayer: state.localPlayer,
      remotePlayers: state.remotePlayers
    });
    requestAnimationFrame(loop);
  }

  loop();
}());
