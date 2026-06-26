import { getUDPBridgeUrl } from "./config";
import type { ConnectionStatus, MovementVector, ParsedSnapshot, Player, SnapshotMessage } from "./types";

interface GameSocketOptions {
  onSnapshot: (snapshot: ParsedSnapshot) => void;
  onStatus: (status: ConnectionStatus, text: string) => void;
}

export interface GameSocket {
  sendInput: (input: MovementVector) => number | null;
  close: () => void;
}

export function createGameSocket(options: GameSocketOptions): GameSocket {
  const socket = new WebSocket(getUDPBridgeUrl(window.location));
  let sequence = 0;

  socket.addEventListener("open", () => {
    options.onStatus("connected", "Connected via UDP");
  });

  socket.addEventListener("close", () => {
    options.onStatus("disconnected", "Disconnected");
  });

  socket.addEventListener("error", () => {
    options.onStatus("error", "Connection error");
  });

  socket.addEventListener("message", (event) => {
    try {
      options.onSnapshot(parseSnapshot(event.data));
    } catch (error) {
      console.warn("Ignored invalid server message:", event.data, error);
    }
  });

  return {
    sendInput(input: MovementVector): number | null {
      if (socket.readyState !== WebSocket.OPEN) {
        return null;
      }

      sequence += 1;
      socket.send(JSON.stringify({ seq: sequence, x: input.x, y: input.y, z: input.z }));
      return sequence;
    },
    close(): void {
      socket.close();
    },
  };
}

function parseSnapshot(message: string): ParsedSnapshot {
  const update = JSON.parse(message) as SnapshotMessage;
  if (!update || update.type !== "snapshot" || !update.players) {
    throw new Error("Unsupported server message");
  }

  const players = new Map<string, Player>();
  Object.values(update.players).forEach((player) => {
    if (isValidPlayer(player)) {
      players.set(player.id || createFallbackId(), {
        ...player,
        z: typeof player.z === "number" ? player.z : 0,
        dimension: player.dimension || update.world?.dimension || "3d",
      });
    }
  });

  return {
    selfId: update.selfId,
    tick: update.tick,
    lastSeq: update.lastSeq || 0,
    world: update.world,
    players,
  };
}

function isValidPlayer(player: Player): boolean {
  return Boolean(player && typeof player.x === "number" && typeof player.y === "number");
}

function createFallbackId(): string {
  if (window.crypto && window.crypto.randomUUID) {
    return window.crypto.randomUUID();
  }

  return `remote-${Date.now()}-${Math.random().toString(16).slice(2)}`;
}
