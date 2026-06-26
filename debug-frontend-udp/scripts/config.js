(function () {
  const CONFIG = {
    canvas: {
      gridSize: 45,
      playerRadius: 10,
    },
    colors: {
      grid: "#1e293b",
      localPlayer: "#f6ad55",
      remotePlayer: "#63b3ed",
    },
    player: {
      speed: 180,
    },
    net: {
      inputRate: 60,
      interpolationDelayMs: 100,
      interpolationBufferSize: 8,
    },
    server: {
      port: 8080,
      path: "/udp-bridge",
    },
  };

  function getUDPBridgeUrl(location) {
    const protocol = location.protocol === "https:" ? "wss" : "ws";
    const host = location.host || `${location.hostname || "localhost"}:${CONFIG.server.port}`;

    return `${protocol}://${host}${CONFIG.server.path}`;
  }

  window.EnservaDebug = {
    ...window.EnservaDebug,
    CONFIG,
    getUDPBridgeUrl,
  };
})();
