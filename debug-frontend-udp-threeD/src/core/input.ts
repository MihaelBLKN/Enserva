import type { MovementVector } from "./types";

const movementKeys = new Set([
  "arrowleft",
  "arrowright",
  "arrowup",
  "arrowdown",
  "a",
  "d",
  "w",
  "s",
  " ",
  "shift",
  "control",
  "c",
]);

export class InputController {
  private readonly pressedKeys = new Set<string>();
  private readonly onKeyDown = (event: KeyboardEvent) => this.setKey(event, true);
  private readonly onKeyUp = (event: KeyboardEvent) => this.setKey(event, false);
  private readonly onBlur = () => this.pressedKeys.clear();

  bind(): void {
    window.addEventListener("keydown", this.onKeyDown);
    window.addEventListener("keyup", this.onKeyUp);
    window.addEventListener("blur", this.onBlur);
  }

  destroy(): void {
    window.removeEventListener("keydown", this.onKeyDown);
    window.removeEventListener("keyup", this.onKeyUp);
    window.removeEventListener("blur", this.onBlur);
    this.pressedKeys.clear();
  }

  getMovementVector(): MovementVector {
    let x = 0;
    let y = 0;
    let z = 0;

    if (this.has("arrowleft") || this.has("a")) {
      x -= 1;
    }
    if (this.has("arrowright") || this.has("d")) {
      x += 1;
    }
    if (this.has("arrowup") || this.has("w")) {
      y -= 1;
    }
    if (this.has("arrowdown") || this.has("s")) {
      y += 1;
    }
    if (this.has(" ")) {
      z += 1;
    }
    if (this.has("shift") || this.has("control") || this.has("c")) {
      z -= 1;
    }

    return { x, y, z };
  }

  private setKey(event: KeyboardEvent, isPressed: boolean): void {
    const key = event.key.toLowerCase();

    if (movementKeys.has(key)) {
      event.preventDefault();
    }

    if (isPressed) {
      this.pressedKeys.add(key);
      return;
    }

    this.pressedKeys.delete(key);
  }

  private has(key: string): boolean {
    return this.pressedKeys.has(key);
  }
}
