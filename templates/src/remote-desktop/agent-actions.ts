/** Remote desktop domain adapter. Authorization, routing and private artifact
 * storage belong to the framework; this module only defines remote semantics. */
import { ActionError, type ActionArtifact, type ActionOptions, type ActionSession, type ActionTask } from "../_lib/actions";

export const desktopPermissions = { view: "remote.desktop.view", control: "remote.desktop.control" } as const;
export type DesktopObservation = {
  frame_id: string;
  captured_at: string;
  display_id: string;
  width: number;
  height: number;
  artifact: ActionArtifact;
};
export type DesktopInput =
  | { kind: "move"; x: number; y: number }
  | { kind: "button"; x: number; y: number; button: 0 | 1 | 2; clicks?: number; down: boolean }
  | { kind: "key"; code: string; down: boolean }
  | { kind: "text"; text: string }
  | { kind: "scroll"; delta: number }
  | { kind: "release" };
export type DesktopObserveOptions = { display_id?: string; max_width?: number; format?: "jpeg" | "png" };
export type DesktopInputResult = { applied: number };

function invalid(message: string): never {
  throw new ActionError({ code: "invalid_argument", message, retryable: false });
}
const keys = new Set([
  "Enter", "Escape", "Backspace", "Tab", "Space", "ArrowLeft", "ArrowUp", "ArrowRight", "ArrowDown",
  "Delete", "Home", "End", "PageUp", "PageDown", "ShiftLeft", "ShiftRight", "ControlLeft", "ControlRight",
  "AltLeft", "AltRight", "MetaLeft", "MetaRight", "Minus", "Equal", "BracketLeft", "BracketRight",
  "Backslash", "Semicolon", "Quote", "Backquote", "Comma", "Period", "Slash",
]);
const point = (x: number) => Number.isFinite(x) && x >= 0 && x <= 1;

/** Validate the whole batch before dispatch; never silently truncate, clamp
 * or partially type an invalid request. The host must validate again. */
export function validateDesktopInput(events: readonly DesktopInput[]): void {
  if (!Array.isArray(events) || events.length < 1 || events.length > 64) invalid("Input requires between 1 and 64 events.");
  for (const e of events) {
    if (!e || typeof e !== "object") invalid("Invalid input event.");
    switch (e.kind) {
      case "move":
        if (!point(e.x) || !point(e.y)) invalid("Coordinates must be finite numbers between 0 and 1.");
        break;
      case "button":
        if (!point(e.x) || !point(e.y) || ![0, 1, 2].includes(e.button) || typeof e.down !== "boolean" ||
          (e.clicks !== undefined && (!Number.isInteger(e.clicks) || e.clicks < 0 || e.clicks > 3))) invalid("Invalid mouse button event.");
        break;
      case "key":
        if (typeof e.code !== "string" || !(keys.has(e.code) || /^Key[A-Z]$/.test(e.code) || /^Digit[0-9]$/.test(e.code)) || typeof e.down !== "boolean") invalid("Unsupported physical key.");
        break;
      case "text":
        if (typeof e.text !== "string" || !e.text.length || [...e.text].length > 256 || new TextEncoder().encode(e.text).length > 1024 ||
          /[\u0000-\u001f\u007f-\u009f\ud800-\udfff]/u.test(e.text)) invalid("Text must be valid Unicode without control characters (up to 256 characters / 1024 bytes).");
        break;
      case "scroll":
        if (!Number.isFinite(e.delta) || Math.abs(e.delta) > 1200) invalid("Scroll delta must be between -1200 and 1200.");
        break;
      case "release": break;
      default: invalid("Unsupported input event.");
    }
  }
}

/** Pixel coordinates refer to the delivered image, never App CSS/native
 * pixels. Out-of-image positions fail instead of clicking a different place. */
export function desktopPoint(frame: Pick<DesktopObservation, "width" | "height">, x: number, y: number): { x: number; y: number } {
  if (!Number.isInteger(frame.width) || frame.width < 1 || !Number.isInteger(frame.height) || frame.height < 1 ||
    !Number.isFinite(x) || !Number.isFinite(y) || x < 0 || y < 0 || x >= frame.width || y >= frame.height) invalid("The point is outside the observed image.");
  return { x: x / frame.width, y: y / frame.height };
}

export function desktopClick(frame: Pick<DesktopObservation, "width" | "height">, x: number, y: number, button: 0 | 1 | 2 = 0): DesktopInput[] {
  const p = desktopPoint(frame, x, y);
  return [ { kind: "button", ...p, button, clicks: 1, down: true }, { kind: "button", ...p, button, clicks: 1, down: false } ];
}

/** All calls still require a live server-authorized ActionSession. This SDK
 * neither starts sharing nor installs/approves the optional native component. */
export class RemoteDesktopActions {
  constructor(private readonly session: ActionSession) {
    if (session.grant.target.kind !== "machine") invalid("Remote desktop requires a machine target.");
  }
  observe(key: string, args: DesktopObserveOptions = {}, options?: ActionOptions): Promise<ActionTask<DesktopObservation>> {
    if ((args.format !== undefined && !["jpeg", "png"].includes(args.format)) ||
      (args.max_width !== undefined && (!Number.isInteger(args.max_width) || args.max_width < 320 || args.max_width > 2560))) invalid("Unsupported image format or maximum width.");
    return this.session.invoke<DesktopObservation>("remote.desktop.observe", 1, args, key, options);
  }
  input(frame: DesktopObservation, events: readonly DesktopInput[], key: string, options?: ActionOptions): Promise<ActionTask<DesktopInputResult>> {
    if (!this.session.grant.permissions?.includes(desktopPermissions.control)) throw new ActionError({ code: "permission_denied", message: "Desktop input requires separately approved control permission.", retryable: false });
    if (!frame.frame_id) invalid("Input requires an observation's frame ID.");
    validateDesktopInput(events);
    return this.session.invoke<DesktopInputResult>("remote.desktop.input", 1, { frame_id: frame.frame_id, events }, key, options);
  }
  read(frame: DesktopObservation, options?: ActionOptions): Promise<Uint8Array> {
    return this.session.read(frame.artifact, options);
  }
  end(options?: ActionOptions) { return this.session.end(options); }
}
