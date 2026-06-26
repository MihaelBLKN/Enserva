import { CONFIG, normalizeWorld } from "./config";
import { arePlayersColliding, clamp, collidesWithAnyObstacle, lerp, normalizeMovement } from "./math";
import type { BufferedPlayer, MovementVector, ParsedSnapshot, PendingInput, Player, WorldConfig } from "./types";

const tickSeconds = 1 / CONFIG.net.inputRate;

export interface GameState {
  world: WorldConfig;
  localPlayer: Player;
  remotePlayers: Map<string, Player>;
  remoteBuffers: Map<string, BufferedPlayer[]>;
  pendingInputs: PendingInput[];
  serverTick: number;
}

export function createGameState(): GameState {
  const world = normalizeWorld();

  return {
    world,
    localPlayer: {
      id: "",
      x: world.gameWorldX / 2,
      y: world.gameWorldY / 2,
      z: CONFIG.player.radius,
      dimension: "3d",
    },
    remotePlayers: new Map(),
    remoteBuffers: new Map(),
    pendingInputs: [],
    serverTick: 0,
  };
}

export function predictLocalInput(state: GameState, sequence: number | null, input: MovementVector): void {
  if (!sequence) {
    return;
  }

  const normalized = normalizeMovement(input, "3d");
  state.pendingInputs.push({
    seq: sequence,
    x: normalized.x,
    y: normalized.y,
    z: normalized.z,
  });
  applyMovement(state.localPlayer, normalized, tickSeconds, state);
}

export function applySnapshot(state: GameState, snapshot: ParsedSnapshot, receivedAt: number): void {
  state.world = normalizeWorld(snapshot.world || state.world);
  const localPlayer = snapshot.players.get(snapshot.selfId);

  if (localPlayer) {
    state.localPlayer = {
      ...localPlayer,
      z: localPlayer.z || CONFIG.player.radius,
      dimension: "3d",
    };
    state.pendingInputs = state.pendingInputs.filter((input) => input.seq > snapshot.lastSeq);
    state.pendingInputs.forEach((input) => {
      applyMovement(state.localPlayer, input, tickSeconds, state);
    });
  }

  updateRemoteBuffers(state, snapshot, receivedAt);
  state.serverTick = snapshot.tick || state.serverTick;
}

export function updateInterpolatedRemotePlayers(state: GameState, now: number): void {
  const renderTime = now - CONFIG.net.interpolationDelayMs;
  const remotePlayers = new Map<string, Player>();

  state.remoteBuffers.forEach((buffer, id) => {
    trimBuffer(buffer, renderTime);

    const player = interpolateBuffer(buffer, renderTime);
    if (player) {
      remotePlayers.set(id, player);
    }
  });

  state.remotePlayers = remotePlayers;
}

function updateRemoteBuffers(state: GameState, snapshot: ParsedSnapshot, receivedAt: number): void {
  const seen = new Set<string>();

  snapshot.players.forEach((player, id) => {
    if (id === snapshot.selfId) {
      return;
    }

    seen.add(id);
    const buffer = state.remoteBuffers.get(id) || [];
    buffer.push({
      time: receivedAt,
      player: {
        ...player,
        z: player.z || CONFIG.player.radius,
        dimension: "3d",
      },
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

function interpolateBuffer(buffer: BufferedPlayer[], renderTime: number): Player | null {
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
        z: lerp(previous.player.z || CONFIG.player.radius, next.player.z || CONFIG.player.radius, alpha),
      };
    }
  }

  return { ...buffer[buffer.length - 1].player };
}

function trimBuffer(buffer: BufferedPlayer[], renderTime: number): void {
  while (buffer.length > 2 && buffer[1].time <= renderTime) {
    buffer.shift();
  }
}

function applyMovement(player: Player, input: PendingInput, deltaSeconds: number, state: GameState): void {
  const nextPlayer: Player = {
    ...player,
    x: player.x + input.x * CONFIG.player.speed * deltaSeconds,
    y: player.y + input.y * CONFIG.player.speed * deltaSeconds,
    z: (player.z || CONFIG.player.radius) + input.z * CONFIG.player.speed * deltaSeconds,
  };

  clampPlayerToBounds(nextPlayer, state.world);

  for (const remotePlayer of state.remotePlayers.values()) {
    if (arePlayersColliding(nextPlayer, remotePlayer, "3d")) {
      return;
    }
  }

  if (collidesWithAnyObstacle(nextPlayer, state.world.obstacles)) {
    return;
  }

  player.x = nextPlayer.x;
  player.y = nextPlayer.y;
  player.z = nextPlayer.z;
}

function clampPlayerToBounds(player: Player, world: WorldConfig): void {
  const radius = CONFIG.player.radius;

  player.x = clamp(player.x, radius, world.gameWorldX - radius);
  player.y = clamp(player.y, radius, world.gameWorldY - radius);
  player.z = clamp(player.z || radius, radius, (world.gameWorldZ || radius * 2) - radius);
}
