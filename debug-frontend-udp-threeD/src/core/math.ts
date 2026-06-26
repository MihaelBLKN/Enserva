import { CONFIG } from "./config";
import type { Dimension, MovementVector, Obstacle, Player } from "./types";

export function clamp(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, value));
}

export function lerp(start: number, end: number, alpha: number): number {
  return start + (end - start) * alpha;
}

export function normalizeMovement(input: MovementVector, dimension: Dimension = "3d"): MovementVector {
  const x = clamp(input.x, -1, 1);
  const y = clamp(input.y, -1, 1);
  const z = dimension === "3d" ? clamp(input.z, -1, 1) : 0;
  const length = dimension === "3d" ? Math.hypot(x, y, z) : Math.hypot(x, y);

  if (length <= 1) {
    return { x, y, z };
  }

  return {
    x: x / length,
    y: y / length,
    z: z / length,
  };
}

export function arePlayersColliding(playerA: Player, playerB: Player, dimension: Dimension = "3d"): boolean {
  const dx = playerA.x - playerB.x;
  const dy = playerA.y - playerB.y;
  const dz = dimension === "3d" ? (playerA.z || 0) - (playerB.z || 0) : 0;

  return Math.hypot(dx, dy, dz) < CONFIG.player.radius * 2;
}

export function playerCollidesWithObstacle(player: Player, obstacle: Obstacle): boolean {
  const radius = CONFIG.player.radius;
  const minX = obstacle.x - obstacle.width / 2;
  const maxX = obstacle.x + obstacle.width / 2;
  const minY = obstacle.y - obstacle.depth / 2;
  const maxY = obstacle.y + obstacle.depth / 2;
  const minZ = obstacle.z - obstacle.height / 2;
  const maxZ = obstacle.z + obstacle.height / 2;
  const closestX = clamp(player.x, minX, maxX);
  const closestY = clamp(player.y, minY, maxY);
  const closestZ = clamp(player.z || 0, minZ, maxZ);
  const dx = player.x - closestX;
  const dy = player.y - closestY;
  const dz = (player.z || 0) - closestZ;

  return dx * dx + dy * dy + dz * dz < radius * radius;
}

export function collidesWithAnyObstacle(player: Player, obstacles: Obstacle[] = []): boolean {
  return obstacles.some((obstacle) => playerCollidesWithObstacle(player, obstacle));
}
