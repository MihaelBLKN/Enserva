<template>
  <main class="debug3d-shell">
    <canvas ref="canvasRef" class="world-canvas" aria-label="Enserva UDP 3D debug world"></canvas>

    <section class="hud" aria-live="polite">
      <div class="hud-row">
        <span class="status-pill" :class="`status-pill--${view.status}`">{{ view.statusText }}</span>
        <span>Tick {{ view.tick }}</span>
      </div>
      <div class="hud-grid">
        <span>Players {{ view.players }}</span>
        <span>Remotes {{ view.remotes }}</span>
        <span>X {{ view.x }}</span>
        <span>Y {{ view.y }}</span>
        <span>Z {{ view.z }}</span>
      </div>
    </section>
  </main>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, reactive, ref } from "vue";
import { CONFIG } from "./core/config";
import { InputController } from "./core/input";
import { createGameSocket, type GameSocket } from "./core/network";
import {
  applySnapshot,
  createGameState,
  predictLocalInput,
  updateInterpolatedRemotePlayers,
  type GameState,
} from "./core/state";
import { ThreeWorld } from "./core/threeWorld";
import type { ConnectionStatus } from "./core/types";

const canvasRef = ref<HTMLCanvasElement | null>(null);
const view = reactive({
  status: "connecting" as ConnectionStatus,
  statusText: "Connecting...",
  tick: 0,
  players: 1,
  remotes: 0,
  x: "0",
  y: "0",
  z: "0",
});

let state: GameState | null = null;
let world: ThreeWorld | null = null;
let input: InputController | null = null;
let socket: GameSocket | null = null;
let inputTimer = 0;
let animationFrame = 0;
const onResize = () => world?.resize();

onMounted(() => {
  const canvas = canvasRef.value;
  if (!canvas) {
    return;
  }

  state = createGameState();
  world = new ThreeWorld(canvas);
  input = new InputController();
  input.bind();

  socket = createGameSocket({
    onSnapshot(snapshot) {
      if (state) {
        applySnapshot(state, snapshot, performance.now());
      }
    },
    onStatus(status, text) {
      view.status = status;
      view.statusText = text;
    },
  });

  inputTimer = window.setInterval(() => {
    if (!state || !input || !socket) {
      return;
    }

    const movement = input.getMovementVector();
    const sequence = socket.sendInput(movement);
    predictLocalInput(state, sequence, movement);
  }, 1000 / CONFIG.net.inputRate);

  window.addEventListener("resize", onResize);

  const loop = () => {
    if (state && world) {
      updateInterpolatedRemotePlayers(state, performance.now());
      world.render({
        localPlayer: state.localPlayer,
        remotePlayers: state.remotePlayers,
        world: state.world,
      });
      syncHud(state);
    }

    animationFrame = requestAnimationFrame(loop);
  };

  loop();
});

onBeforeUnmount(() => {
  window.removeEventListener("resize", onResize);
  window.clearInterval(inputTimer);
  window.cancelAnimationFrame(animationFrame);
  input?.destroy();
  socket?.close();
  world?.destroy();
});

function syncHud(nextState: GameState): void {
  const local = nextState.localPlayer;

  view.tick = nextState.serverTick;
  view.remotes = nextState.remotePlayers.size;
  view.players = (local.id ? 1 : 0) + nextState.remotePlayers.size;
  view.x = local.x.toFixed(1);
  view.y = local.y.toFixed(1);
  view.z = (local.z || 0).toFixed(1);
}
</script>
