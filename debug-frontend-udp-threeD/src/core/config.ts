import type { Obstacle, WorldConfig } from "./types";

export const CONFIG = {
  player: {
    radius: 10,
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
  scene: {
    scale: 0.045,
    cameraLerp: 0.08,
  },
};

export const DEFAULT_WORLD: Required<Pick<WorldConfig, "gameWorldX" | "gameWorldY" | "gameWorldZ" | "dimension">> & {
  obstacles: Obstacle[];
} = {
  gameWorldX: 720,
  gameWorldY: 405,
  gameWorldZ: 180,
  dimension: "3d",
  obstacles: [],
};

export function getUDPBridgeUrl(location: Location): string {
  const protocol = location.protocol === "https:" ? "wss" : "ws";
  const host = location.host || `${location.hostname || "localhost"}:${CONFIG.server.port}`;

  return `${protocol}://${host}${CONFIG.server.path}`;
}

export function normalizeWorld(world?: WorldConfig): WorldConfig {
  return {
    gameWorldX: world?.gameWorldX || DEFAULT_WORLD.gameWorldX,
    gameWorldY: world?.gameWorldY || DEFAULT_WORLD.gameWorldY,
    gameWorldZ: world?.gameWorldZ || DEFAULT_WORLD.gameWorldZ,
    dimension: world?.dimension || "3d",
    obstacles: world?.obstacles || [],
  };
}
