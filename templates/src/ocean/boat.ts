// The boat: a small fishing boat built from code (hull lofted from cross
// sections, a cabin, a mast and gear), and how it moves: throttle and
// rudder, riding the waves (four points on the hull feel the water), and
// stopping where the ground comes up.

import * as THREE from "three";
import { clamp, lerp } from "./noise";
import { merge } from "./islands";

export const LENGTH = 7.4, BEAM = 2.6;
const KEEL = -0.62;

/** Half the hull's width at a point along it (s: 0 stern → 1 bow). */
export function halfWidth(s: number) {
  return (BEAM / 2) * (s < 0.58 ? 0.9 + 0.1 * Math.sin((s / 0.58) * (Math.PI / 2)) : Math.pow(Math.max(0, Math.cos(((s - 0.58) / 0.42) * (Math.PI / 2))), 0.75));
}
const keelY = (s: number) => KEEL + 0.28 * Math.pow(Math.max(0, (s - 0.62) / 0.38), 2);
const gunY = (s: number) => 1.02 + 0.6 * s * s;
const deckY = (s: number) => gunY(s) - 0.36;

function colored(g: THREE.BufferGeometry, f: (x: number, y: number, z: number) => THREE.Color) {
  const p = g.attributes.position, a = new Float32Array(p.count * 3);
  for (let i = 0; i < p.count; i++) { const c = f(p.getX(i), p.getY(i), p.getZ(i)); a.set([c.r, c.g, c.b], i * 3); }
  g.setAttribute("color", new THREE.BufferAttribute(a, 3));
  return g;
}
const solid = (g: THREE.BufferGeometry, c: THREE.Color) => colored(g, () => c);

// (linear colours: they look lighter on screen)
const HULL_TOP = new THREE.Color(0.62, 0.64, 0.66), HULL_LOW = new THREE.Color(0.025, 0.027, 0.03), STRIPE = new THREE.Color(0.42, 0.035, 0.025);
const RED = new THREE.Color(0.42, 0.04, 0.025), WHITE = new THREE.Color(0.72, 0.72, 0.7), DECK = new THREE.Color(0.17, 0.14, 0.11);
const DARK = new THREE.Color(0.01, 0.01, 0.012), BLUE = new THREE.Color(0.015, 0.07, 0.42), ORANGE = new THREE.Color(0.88, 0.2, 0.02), STEEL = new THREE.Color(0.28, 0.29, 0.3);

/** The hull: outside, a rail cap, the inside of the bulwark, the deck and the transom. */
function hull() {
  const S = 28, M = 10;
  const pos: number[] = [], idx: number[] = [];
  const ring = (s: number, side: number, inner: boolean) => {
    const out: number[] = [];
    const z = (s - 0.5) * LENGTH, hw = halfWidth(s);
    for (let j = 0; j <= M; j++) {
      const t = j / M;
      let x: number, y: number;
      if (!inner) {
        const p = lerp(2.6, 1.15, Math.pow(s, 2));
        x = hw * (1 - Math.pow(1 - t, p));
        y = lerp(keelY(s), gunY(s), t);
      } else {
        x = hw * 0.9;
        y = lerp(gunY(s), deckY(s), t);
      }
      out.push(pos.length / 3);
      pos.push(side * x, y, z);
    }
    return out;
  };
  const strip = (a: number[][], flip: boolean) => {
    for (let i = 0; i < a.length - 1; i++) for (let j = 0; j < a[i].length - 1; j++) {
      const p = a[i][j], q = a[i + 1][j], r = a[i][j + 1], s = a[i + 1][j + 1];
      if (flip) idx.push(p, r, q, q, r, s); else idx.push(p, q, r, q, s, r);
    }
  };
  for (const side of [1, -1]) {
    const outer: number[][] = [], inner: number[][] = [];
    for (let i = 0; i <= S; i++) { const s = i / S; outer.push(ring(s, side, false)); inner.push(ring(s, side, true)); }
    strip(outer, side < 0);
    strip(inner, side > 0);
    // the rail cap between the outside and the bulwark's inside
    const cap: number[][] = [];
    for (let i = 0; i <= S; i++) {
      const s = i / S, z = (s - 0.5) * LENGTH, hw = halfWidth(s);
      const a = pos.length / 3; pos.push(side * hw, gunY(s) + 0.04, z);
      const b = pos.length / 3; pos.push(side * hw * 0.9, gunY(s) + 0.04, z);
      cap.push([a, b]);
    }
    strip(cap, side > 0);
  }
  // the deck
  const deck: number[][] = [];
  for (let i = 0; i <= S; i++) {
    const s = i / S, z = (s - 0.5) * LENGTH, hw = halfWidth(s) * 0.9;
    const a = pos.length / 3; pos.push(-hw, deckY(s), z);
    const b = pos.length / 3; pos.push(hw, deckY(s), z);
    deck.push([a, b]);
  }
  strip(deck, true);
  // the transom (the flat stern)
  const tz = -0.5 * LENGTH, hw0 = halfWidth(0);
  const tr: number[][] = [];
  for (let j = 0; j <= M; j++) {
    const t = j / M, x = hw0 * (1 - Math.pow(1 - t, 2.6)), y = lerp(keelY(0), gunY(0), t);
    const a = pos.length / 3; pos.push(-x, y, tz);
    const b = pos.length / 3; pos.push(x, y, tz);
    tr.push([a, b]);
  }
  strip(tr, false);
  const g = new THREE.BufferGeometry();
  g.setAttribute("position", new THREE.Float32BufferAttribute(pos, 3));
  g.setIndex(idx);
  g.computeVertexNormals();
  return colored(g, (x, y, z) => {
    const s = z / LENGTH + 0.5;
    if (Math.abs(x) < halfWidth(s) * 0.905 && y <= deckY(s) + 0.01) return DECK;
    if (y > gunY(s) - 0.02) return RED;
    if (y < -0.05) return HULL_LOW;
    if (y < 0.08) return STRIPE;
    return HULL_TOP;
  });
}

function box(w: number, h: number, d: number, x: number, y: number, z: number, c: THREE.Color) {
  const g = new THREE.BoxGeometry(w, h, d);
  g.translate(x, y, z);
  return solid(g, c);
}
function cyl(r: number, h: number, x: number, y: number, z: number, c: THREE.Color, seg = 10, rx = 0, rz = 0) {
  const g = new THREE.CylinderGeometry(r, r, h, seg);
  if (rx) g.rotateX(rx);
  if (rz) g.rotateZ(rz);
  g.translate(x, y, z);
  return solid(g, c);
}

/** The whole boat, its bow toward +z. Paint and glass are two materials. */
export function boatModel() {
  const group = new THREE.Group();
  const paint = new THREE.MeshStandardMaterial({ vertexColors: true, roughness: 0.55, metalness: 0.05, side: THREE.DoubleSide });
  const glass = new THREE.MeshStandardMaterial({ color: 0x0b0f14, roughness: 0.08, metalness: 0.2 });
  const cz = -0.25, cy = deckY(0.47), cw = 1.75, ch = 1.55, cd = 2.1; // the cabin
  const parts = [
    hull(),
    box(cw, ch, cd, 0, cy + ch / 2, cz, WHITE),
    box(cw + 0.22, 0.12, cd + 0.35, 0, cy + ch + 0.06, cz + 0.05, RED),
    box(cw + 0.06, 0.1, cd + 0.06, 0, cy + 0.05, cz, RED),
    // mast, a yard, antennas and a radar
    cyl(0.05, 2.4, 0, cy + ch + 1.25, cz - 0.3, STEEL),
    cyl(0.03, 1.4, 0, cy + ch + 2.05, cz - 0.3, STEEL, 6, 0, Math.PI / 2),
    cyl(0.015, 1.6, 0.55, cy + ch + 0.9, cz - 0.85, STEEL, 5),
    cyl(0.015, 1.2, -0.55, cy + ch + 0.7, cz - 0.85, STEEL, 5),
    box(0.5, 0.12, 0.18, 0, cy + ch + 0.3, cz + 0.3, DARK),
    // gear on deck: a blue barrel, an orange buoy, a fish box, a life ring
    cyl(0.28, 0.85, -0.62, deckY(0.12) + 0.43, -2.6, BLUE, 14),
    box(0.9, 0.42, 0.6, 0.45, deckY(0.1) + 0.21, -2.75, new THREE.Color(0.25, 0.42, 0.55)),
    solid(new THREE.SphereGeometry(0.22, 12, 8).translate(-halfWidth(0.2) - 0.05, gunY(0.2) - 0.25, -1.9), ORANGE),
    solid(new THREE.TorusGeometry(0.32, 0.07, 6, 16).translate(0, cy + 0.95, cz - cd / 2 - 0.08), ORANGE),
    // rail posts along the bow
    ...[0.7, 0.78, 0.86].flatMap((s) => [1, -1].map((side) => cyl(0.025, 0.45, side * halfWidth(s) * 0.92, gunY(s) + 0.22, (s - 0.5) * LENGTH, STEEL, 5))),
  ];
  const body = new THREE.Mesh(merge(parts), paint);
  group.add(body);
  // windows: front three panes, two along each side, one at the back
  const win = new THREE.Group();
  const pane = (w: number, h: number, x: number, y: number, z: number, ry: number) => {
    const m = new THREE.Mesh(new THREE.PlaneGeometry(w, h), glass);
    m.position.set(x, y, z); m.rotation.y = ry; win.add(m);
  };
  const wy = cy + ch * 0.68;
  for (const x of [-0.56, 0, 0.56]) pane(0.48, 0.55, x, wy, cz + cd / 2 + 0.006, 0);
  for (const side of [1, -1]) for (const z of [cz + 0.45, cz - 0.4]) pane(0.68, 0.5, side * (cw / 2 + 0.006), wy, z, (side * Math.PI) / 2);
  pane(0.9, 0.45, 0, wy, cz - cd / 2 - 0.006, Math.PI);
  group.add(win);
  group.traverse((o) => { (o as THREE.Mesh).castShadow = true; });
  return group;
}

export type BoatState = {
  x: number; z: number; heading: number; // radians, 0 = north, clockwise
  speed: number; // m/s along the heading
  throttle: number; // -0.4 … 1 (the lever)
  rudder: number; // -1 … 1 (left … right)
  y: number; vy: number; pitch: number; roll: number; vp: number; vr: number;
  aground: number; // seconds since it last touched ground (Infinity: never)
};
export type BoatInput = { throttle: -1 | 0 | 1; steer: -1 | 0 | 1; stop: boolean };

export const MAX_SPEED = 13; // m/s (about 25 knots)
export const THROTTLE_STEPS = [-0.4, -0.2, 0, 0.25, 0.5, 0.75, 1];

export function newBoat(x = 0, z = 0, heading = 0): BoatState {
  return { x, z, heading, speed: 0, throttle: 0, rudder: 0, y: 0, vy: 0, pitch: 0, roll: 0, vp: 0, vr: 0, aground: Infinity };
}

/** Moves the boat on by dt seconds. `ground` gives the ground's height at
 *  a point, `water` the sea's surface there now; the boat stops (and is
 *  pushed back) where it would touch the ground. */
export function stepBoat(b: BoatState, inp: BoatInput, dt: number, water: (x: number, z: number) => number, ground: (x: number, z: number) => number) {
  dt = Math.min(dt, 0.05);
  // the lever moves while a key is held; the rudder springs back
  if (inp.stop) b.throttle = 0;
  else if (inp.throttle) b.throttle = clamp(b.throttle + inp.throttle * dt * 0.55, -0.4, 1);
  b.rudder += ((inp.steer || 0) - b.rudder) * Math.min(1, dt * (inp.steer ? 3.2 : 4.5));
  // speed: thrust against water drag
  const thrust = b.throttle * (b.throttle > 0 ? 3.4 : 2.2);
  const drag = 0.06 * b.speed + 0.0145 * b.speed * Math.abs(b.speed);
  b.speed += (thrust - drag) * dt;
  if (Math.abs(b.speed) < 0.02 && !b.throttle) b.speed = 0;
  // turning: the rudder works with the water going past it
  const flow = clamp(Math.abs(b.speed) / 3, 0, 1) * Math.sign(b.speed || 1) + (b.throttle > 0 ? 0.25 * b.throttle : 0);
  b.heading += b.rudder * 0.42 * flow * dt * (1 - 0.35 * clamp(b.speed / MAX_SPEED, 0, 1));
  b.heading = ((b.heading % (Math.PI * 2)) + Math.PI * 2) % (Math.PI * 2);
  const fx = Math.sin(b.heading), fz = -Math.cos(b.heading);
  let nx = b.x + fx * b.speed * dt, nz = b.z + fz * b.speed * dt;
  // ground ahead (or astern when backing): stop and bounce off
  const ahead = (b.speed >= 0 ? 1 : -1) * (LENGTH / 2 - 0.3);
  const g = Math.max(ground(nx + fx * ahead, nz + fz * ahead), ground(nx, nz));
  if (g > KEEL - 0.15) {
    nx = b.x - fx * b.speed * dt * 0.5; nz = b.z - fz * b.speed * dt * 0.5;
    if (Math.abs(b.speed) > 1.2) b.aground = 0;
    b.speed *= -0.25;
  }
  b.x = nx; b.z = nz;
  b.aground += dt;
  // riding the waves: four points on the hull feel the water
  const rx = Math.cos(b.heading), rz = Math.sin(b.heading); // to starboard
  const hb = water(b.x + fx * 2.6, b.z + fz * 2.6);
  const hs = water(b.x - fx * 2.6, b.z - fz * 2.6);
  const hr = water(b.x + rx * 1.1, b.z + rz * 1.1);
  const hl = water(b.x - rx * 1.1, b.z - rz * 1.1);
  const v = clamp(b.speed / MAX_SPEED, -0.4, 1);
  const wantY = (hb + hs + hr + hl) / 4 + 0.12 * v; // a little lift when planing
  const wantP = Math.atan2(hb - hs, 5.2) + 0.06 * v * (1 - v * 0.5); // bow up when pushing
  const wantR = Math.atan2(hl - hr, 2.2) + b.rudder * 0.07 * clamp(Math.abs(b.speed) / 6, 0, 1); // lean out of the turn
  b.vy += ((wantY - b.y) * 14 - b.vy * 4.2) * dt; b.y += b.vy * dt;
  b.vp += ((wantP - b.pitch) * 10 - b.vp * 3.4) * dt; b.pitch += b.vp * dt;
  b.vr += ((wantR - b.roll) * 7 - b.vr * 2.2) * dt; b.roll += b.vr * dt;
  if (b.aground < 0.35) b.roll += Math.sin(b.aground * 60) * 0.01;
}

/** Puts the model where the boat is. */
export function placeBoat(model: THREE.Object3D, b: BoatState) {
  model.position.set(b.x, b.y, b.z);
  model.rotation.set(-b.pitch, Math.PI - b.heading, b.roll, "YXZ");
}

/** The hull's outline at the waterline, for keeping the sea out of it. */
export const HULL_GLSL = /* glsl */ `
float hullHalfWidth(float s) {
  float hw = ${(BEAM / 2).toFixed(3)};
  if (s < 0.58) return hw * (0.9 + 0.1 * sin(s / 0.58 * 1.5708));
  return hw * pow(max(cos((s - 0.58) / 0.42 * 1.5708), 0.0), 0.75);
}
// the hull's width at a height (as hull() lofts it): water inside it is not drawn
bool insideHull(vec3 local) {
  float s = local.z / ${LENGTH.toFixed(3)} + 0.5;
  if (s < 0.0 || s > 1.0) return false;
  float keel = ${KEEL.toFixed(3)} + 0.28 * pow(max(0.0, (s - 0.62) / 0.38), 2.0);
  float gun = 1.02 + 0.6 * s * s;
  float t = clamp((local.y - keel) / (gun - keel), 0.0, 1.0);
  if (local.y < keel) return false;
  float p = mix(2.6, 1.15, s * s);
  return abs(local.x) < hullHalfWidth(s) * (1.0 - pow(1.0 - t, p)) * 0.985;
}
`;
