(function () {
  const { CONFIG } = window.EnservaDebug;

  const tickSeconds = 1 / CONFIG.net.inputRate;

  function createGameState(canvas) {
    return {
      bounds: {
        width: canvas.width,
        height: canvas.height,
      },
      localPlayer: {
        id: "",
        x: canvas.width / 2,
        y: canvas.height / 2,
      },
      remotePlayers: new Map(),
      remoteBuffers: new Map(),
      pendingInputs: [],
      serverTick: 0,
    };
  }

  function predictLocalInput(state, sequence, input) {
    if (!sequence) {
      return;
    }

    const normalized = normalizeInput(input);
    state.pendingInputs.push({
      seq: sequence,
      x: normalized.x,
      y: normalized.y,
    });
    applyMovement(state.localPlayer, normalized, tickSeconds, state.bounds, state.remotePlayers);
  }

  function applySnapshot(state, snapshot, receivedAt) {
    const localPlayer = snapshot.players.get(snapshot.selfId);
    if (localPlayer) {
      state.localPlayer = { ...localPlayer };
      state.pendingInputs = state.pendingInputs.filter((input) => input.seq > snapshot.lastSeq);
      state.pendingInputs.forEach((input) => {
        applyMovement(state.localPlayer, input, tickSeconds, state.bounds, state.remotePlayers);
      });
    }

    updateRemoteBuffers(state, snapshot, receivedAt);
    state.serverTick = snapshot.tick || state.serverTick;
  }

  function updateInterpolatedRemotePlayers(state, now) {
    const renderTime = now - CONFIG.net.interpolationDelayMs;
    const remotePlayers = new Map();

    state.remoteBuffers.forEach((buffer, id) => {
      trimBuffer(buffer, renderTime);

      const player = interpolateBuffer(buffer, renderTime);
      if (player) {
        remotePlayers.set(id, player);
      }
    });

    state.remotePlayers = remotePlayers;
  }

  function updateRemoteBuffers(state, snapshot, receivedAt) {
    const seen = new Set();

    snapshot.players.forEach((player, id) => {
      if (id === snapshot.selfId) {
        return;
      }

      seen.add(id);
      const buffer = state.remoteBuffers.get(id) || [];
      buffer.push({
        time: receivedAt,
        player: { ...player },
      });

      if (buffer.length > CONFIG.net.interpolationBufferSize) {
        buffer.splice(0, buffer.length - CONFIG.net.interpolationBufferSize);
      }

      state.remoteBuffers.set(id, buffer);
    });

    Array.from(state.remoteBuffers.keys()).forEach((id) => {
      if (!seen.has(id)) {
        state.remoteBuffers.delete(id);
      }
    });
  }

  function interpolateBuffer(buffer, renderTime) {
    if (buffer.length === 0) {
      return null;
    }
    if (buffer.length === 1 || renderTime <= buffer[0].time) {
      return { ...buffer[0].player };
    }

    for (let index = 0; index < buffer.length - 1; index += 1) {
      const previous = buffer[index];
      const next = buffer[index + 1];

      if (renderTime >= previous.time && renderTime <= next.time) {
        const span = Math.max(1, next.time - previous.time);
        const alpha = (renderTime - previous.time) / span;

        return {
          ...next.player,
          x: lerp(previous.player.x, next.player.x, alpha),
          y: lerp(previous.player.y, next.player.y, alpha),
        };
      }
    }

    return { ...buffer[buffer.length - 1].player };
  }

  function trimBuffer(buffer, renderTime) {
    while (buffer.length > 2 && buffer[1].time <= renderTime) {
      buffer.shift();
    }
  }

  function applyMovement(player, input, deltaSeconds, bounds, remotePlayers) {
    const nextPlayer = {
      ...player,
      x: player.x + input.x * CONFIG.player.speed * deltaSeconds,
      y: player.y + input.y * CONFIG.player.speed * deltaSeconds,
    };

    clampPlayerToBounds(nextPlayer, bounds);

    for (const remotePlayer of remotePlayers.values()) {
      if (window.EnservaDebug.arePlayersColliding(nextPlayer, remotePlayer)) {
        return;
      }
    }

    player.x = nextPlayer.x;
    player.y = nextPlayer.y;
  }

  function normalizeInput(input) {
    const x = clamp(input.x, -1, 1);
    const y = clamp(input.y, -1, 1);
    const length = Math.hypot(x, y);

    if (length <= 1) {
      return { x, y };
    }

    return {
      x: x / length,
      y: y / length,
    };
  }

  function clampPlayerToBounds(player, bounds) {
    const radius = CONFIG.canvas.playerRadius;

    player.x = clamp(player.x, radius, bounds.width - radius);
    player.y = clamp(player.y, radius, bounds.height - radius);
  }

  function clamp(value, min, max) {
    return Math.max(min, Math.min(max, value));
  }

  function lerp(start, end, alpha) {
    return start + (end - start) * alpha;
  }

  window.EnservaDebug = {
    ...window.EnservaDebug,
    createGameState,
    predictLocalInput,
    applySnapshot,
    updateInterpolatedRemotePlayers,
  };
})();
