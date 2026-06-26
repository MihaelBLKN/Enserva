import * as THREE from "three";
import { CONFIG, normalizeWorld } from "./config";
import type { Obstacle, Player, WorldConfig } from "./types";

const palette = {
  background: 0x171610,
  ground: 0x2e3a28,
  grid: 0xb9b58a,
  wall: 0x314044,
  obstacleA: 0xb95f3d,
  obstacleB: 0x607c72,
  local: 0xffce5c,
  remote: 0x63d8ee,
};

interface RenderPayload {
  localPlayer: Player;
  remotePlayers: Map<string, Player>;
  world: WorldConfig;
}

export class ThreeWorld {
  private readonly renderer: THREE.WebGLRenderer;
  private readonly scene = new THREE.Scene();
  private readonly camera = new THREE.PerspectiveCamera(54, 1, 0.1, 200);
  private readonly playerMeshes = new Map<string, THREE.Mesh>();
  private readonly localMaterial = new THREE.MeshStandardMaterial({ color: palette.local, roughness: 0.38, metalness: 0.08 });
  private readonly remoteMaterial = new THREE.MeshStandardMaterial({ color: palette.remote, roughness: 0.32, metalness: 0.1 });
  private readonly playerGeometry = new THREE.SphereGeometry(CONFIG.player.radius * CONFIG.scene.scale, 24, 16);
  private world = normalizeWorld();
  private worldGroup = new THREE.Group();
  private obstacleHash = "";

  constructor(private readonly canvas: HTMLCanvasElement) {
    this.renderer = new THREE.WebGLRenderer({
      canvas,
      antialias: true,
      alpha: false,
      powerPreference: "high-performance",
    });
    this.renderer.setClearColor(palette.background, 1);
    this.renderer.shadowMap.enabled = true;
    this.renderer.shadowMap.type = THREE.PCFSoftShadowMap;
    this.scene.add(this.worldGroup);
    this.scene.add(new THREE.HemisphereLight(0xf5f0d8, 0x293026, 1.25));

    const keyLight = new THREE.DirectionalLight(0xffffff, 1.9);
    keyLight.position.set(-8, 12, 7);
    keyLight.castShadow = true;
    keyLight.shadow.mapSize.set(1024, 1024);
    this.scene.add(keyLight);

    const rimLight = new THREE.DirectionalLight(0xffb36b, 0.65);
    rimLight.position.set(8, 6, -6);
    this.scene.add(rimLight);

    this.camera.position.set(0, 6.5, 8.5);
    this.rebuildWorld();
    this.resize();
  }

  destroy(): void {
    this.playerMeshes.forEach((mesh) => {
      this.scene.remove(mesh);
      mesh.geometry.dispose();
    });
    this.playerMeshes.clear();
    this.disposeWorldGroup();
    this.playerGeometry.dispose();
    this.localMaterial.dispose();
    this.remoteMaterial.dispose();
    this.renderer.dispose();
  }

  resize(): void {
    const width = Math.max(1, this.canvas.clientWidth);
    const height = Math.max(1, this.canvas.clientHeight);

    this.renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 2));
    this.renderer.setSize(width, height, false);
    this.camera.aspect = width / height;
    this.camera.updateProjectionMatrix();
  }

  render(payload: RenderPayload): void {
    this.setWorld(payload.world);
    this.updatePlayerMesh("local", payload.localPlayer, this.localMaterial);

    const seenRemoteIds = new Set<string>();
    payload.remotePlayers.forEach((player, id) => {
      seenRemoteIds.add(id);
      this.updatePlayerMesh(id, player, this.remoteMaterial);
    });

    Array.from(this.playerMeshes.keys()).forEach((id) => {
      if (id !== "local" && !seenRemoteIds.has(id)) {
        const mesh = this.playerMeshes.get(id);
        if (mesh) {
          this.scene.remove(mesh);
          this.playerMeshes.delete(id);
        }
      }
    });

    this.followPlayer(payload.localPlayer);
    this.renderer.render(this.scene, this.camera);
  }

  private setWorld(world: WorldConfig): void {
    const normalized = normalizeWorld(world);
    const nextHash = JSON.stringify({
      x: normalized.gameWorldX,
      y: normalized.gameWorldY,
      z: normalized.gameWorldZ,
      obstacles: normalized.obstacles,
    });

    if (nextHash === this.obstacleHash) {
      return;
    }

    this.world = normalized;
    this.obstacleHash = nextHash;
    this.rebuildWorld();
  }

  private rebuildWorld(): void {
    this.disposeWorldGroup();
    this.worldGroup = new THREE.Group();
    this.scene.add(this.worldGroup);

    const width = this.world.gameWorldX * CONFIG.scene.scale;
    const depth = this.world.gameWorldY * CONFIG.scene.scale;
    const ground = new THREE.Mesh(
      new THREE.PlaneGeometry(width, depth),
      new THREE.MeshStandardMaterial({ color: palette.ground, roughness: 0.78, metalness: 0.02 }),
    );
    ground.rotation.x = -Math.PI / 2;
    ground.receiveShadow = true;
    this.worldGroup.add(ground);

    const grid = new THREE.GridHelper(Math.max(width, depth), 16, palette.grid, palette.grid);
    const gridMaterial = grid.material as THREE.Material;
    gridMaterial.transparent = true;
    gridMaterial.opacity = 0.32;
    grid.position.y = 0.012;
    this.worldGroup.add(grid);

    this.addBoundary(width, depth);
    (this.world.obstacles || []).forEach((obstacle, index) => this.addObstacle(obstacle, index));
  }

  private addBoundary(width: number, depth: number): void {
    const wallMaterial = new THREE.MeshStandardMaterial({ color: palette.wall, roughness: 0.6, metalness: 0.06 });
    const thickness = 0.12;
    const height = 0.46;
    const walls = [
      { x: 0, z: -depth / 2, w: width, d: thickness },
      { x: 0, z: depth / 2, w: width, d: thickness },
      { x: -width / 2, z: 0, w: thickness, d: depth },
      { x: width / 2, z: 0, w: thickness, d: depth },
    ];

    walls.forEach((wall) => {
      const mesh = new THREE.Mesh(new THREE.BoxGeometry(wall.w, height, wall.d), wallMaterial.clone());
      mesh.position.set(wall.x, height / 2, wall.z);
      mesh.castShadow = true;
      mesh.receiveShadow = true;
      this.worldGroup.add(mesh);
    });
  }

  private addObstacle(obstacle: Obstacle, index: number): void {
    const material = new THREE.MeshStandardMaterial({
      color: index % 2 === 0 ? palette.obstacleA : palette.obstacleB,
      roughness: 0.5,
      metalness: 0.08,
    });
    const mesh = new THREE.Mesh(
      new THREE.BoxGeometry(
        obstacle.width * CONFIG.scene.scale,
        obstacle.height * CONFIG.scene.scale,
        obstacle.depth * CONFIG.scene.scale,
      ),
      material,
    );

    mesh.position.copy(this.toScenePosition({
      id: obstacle.id,
      x: obstacle.x,
      y: obstacle.y,
      z: obstacle.z,
    }));
    mesh.castShadow = true;
    mesh.receiveShadow = true;
    this.worldGroup.add(mesh);
  }

  private updatePlayerMesh(id: string, player: Player, material: THREE.Material): void {
    let mesh = this.playerMeshes.get(id);

    if (!mesh) {
      mesh = new THREE.Mesh(this.playerGeometry.clone(), material);
      mesh.castShadow = true;
      this.scene.add(mesh);
      this.playerMeshes.set(id, mesh);
    }

    const nextPosition = this.toScenePosition(player);
    mesh.position.lerp(nextPosition, id === "local" ? 0.55 : 0.35);
  }

  private followPlayer(player: Player): void {
    const target = this.toScenePosition(player);
    const desired = new THREE.Vector3(target.x, target.y + 5.4, target.z + 7.2);

    this.camera.position.lerp(desired, CONFIG.scene.cameraLerp);
    this.camera.lookAt(target.x, target.y + 0.45, target.z);
  }

  private toScenePosition(player: Pick<Player, "x" | "y" | "z">): THREE.Vector3 {
    return new THREE.Vector3(
      (player.x - this.world.gameWorldX / 2) * CONFIG.scene.scale,
      (player.z || CONFIG.player.radius) * CONFIG.scene.scale,
      (player.y - this.world.gameWorldY / 2) * CONFIG.scene.scale,
    );
  }

  private disposeWorldGroup(): void {
    this.scene.remove(this.worldGroup);
    this.worldGroup.traverse((object) => {
      const mesh = object as THREE.Mesh;
      if (mesh.geometry) {
        mesh.geometry.dispose();
      }
      if (Array.isArray(mesh.material)) {
        mesh.material.forEach((material) => material.dispose());
      } else if (mesh.material) {
        mesh.material.dispose();
      }
    });
  }
}
