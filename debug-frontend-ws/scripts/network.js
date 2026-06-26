(function () {
  const { getWebSocketUrl } = window.EnservaDebug;

  function createGameSocket({ onPlayers, onStatus }) {
    const socket = new WebSocket(getWebSocketUrl(window.location));
    let sequence = 0;

    socket.addEventListener("open", () => {
      onStatus("connected", "Connected");
    });

    socket.addEventListener("close", () => {
      onStatus("disconnected", "Disconnected");
    });

    socket.addEventListener("error", () => {
      onStatus("error", "Connection error");
    });

    socket.addEventListener("message", (event) => {
      try {
        onPlayers(parseSnapshot(event.data));
      } catch (error) {
        console.warn("Ignored invalid server message:", event.data);
      }
    });

    return {
      sendInput(input) {
        if (socket.readyState === WebSocket.OPEN) {
          sequence += 1;
          socket.send(JSON.stringify({ seq: sequence, x: input.x, y: input.y }));
          return sequence;
        }

        return null;
      }
    };
  }

  function parseSnapshot(message) {
    const update = JSON.parse(message);
    if (!update || update.type !== "snapshot" || !update.players) {
      throw new Error("Unsupported server message");
    }

    const players = new Map();

    Object.values(update.players).forEach((player) => {
      if (isValidPlayer(player)) {
        players.set(player.id || createFallbackId(), player);
      }
    });

    return {
      selfId: update.selfId,
      tick: update.tick,
      lastSeq: update.lastSeq || 0,
      players
    };
  }

  function isValidPlayer(player) {
    return player && typeof player.x === "number" && typeof player.y === "number";
  }

  function createFallbackId() {
    if (window.crypto && window.crypto.randomUUID) {
      return window.crypto.randomUUID();
    }

    return `remote-${Date.now()}-${Math.random().toString(16).slice(2)}`;
  }

  window.EnservaDebug = {
    ...window.EnservaDebug,
    createGameSocket
  };
}());
