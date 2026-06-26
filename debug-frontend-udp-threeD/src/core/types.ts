export type Dimension = "2d" | "3d";
export type ConnectionStatus = "connecting" | "connected" | "disconnected" | "error";

export interface Obstacle {
  id: string;
  x: number;
  y: number;
  z: number;
  width: number;
  depth: number;
  height: number;
}

export interface WorldConfig {
  gameWorldX: number;
  gameWorldY: number;
  gameWorldZ?: number;
  dimension?: Dimension;
  obstacles?: Obstacle[];
}

export interface Player {
  id: string;
  x: number;
  y: number;
  z?: number;
  dimension?: Dimension;
}

export interface MovementVector {
  x: number;
  y: number;
  z: number;
}

export interface SnapshotMessage {
  type: "snapshot";
  selfId: string;
  tick: number;
  lastSeq: number;
  world?: WorldConfig;
  players: Record<string, Player>;
}

export interface ParsedSnapshot {
  selfId: string;
  tick: number;
  lastSeq: number;
  world?: WorldConfig;
  players: Map<string, Player>;
}

export interface BufferedPlayer {
  time: number;
  player: Player;
}

export interface PendingInput extends MovementVector {
  seq: number;
}
