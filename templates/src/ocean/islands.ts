// Islands from the page's records: each one's ground (a height field shaped
// by its kind, size and seed), its colours, its palms, and the ground's
// height for the boat (running aground) and the map. Pure geometry here;
// world.ts puts them in the scene.

import * as THREE from "three";
import { clamp, fbm, noise2, ridged, rng, smoothstep } from "./noise";

export type Kind = "island" | "rocks" | "stack" | "sandbar";
export const KINDS: Kind[] = ["island", "rocks", "stack", "sandbar"];
export type Isle = { id: string; name: string; x: number; z: number; size: number; height: number; kind: Kind; palms: number; seed: number; note: string };

const DEEP = -24; // the open sea floor (only its shallows show)

type Mound = { x: number; z: number; r: number; h: number; steep: number };

/** One island's shape: its ground height anywhere (world coordinates). */
export class Shape {
  readonly mounds: Mound[] = [];
  readonly reach: number; // how far from the centre the ground rises off the floor
  readonly shelf: number;
  constructor(readonly isle: Isle) {
    const { size, height, kind, seed } = isle;
    const r = rng(seed * 7919 + 13);
    if (kind === "stack") {
      const n = 1 + Math.floor(r() * 2.2);
      for (let i = 0; i < n; i++) {
        const a = r() * Math.PI * 2, d = i ? size * (1.1 + r() * 0.9) : 0;
        this.mounds.push({ x: Math.cos(a) * d, z: Math.sin(a) * d, r: size * (i ? 0.55 + r() * 0.35 : 1), h: height * (i ? 0.45 + r() * 0.4 : 1), steep: 7 });
      }
      this.shelf = size * 2.6;
    } else if (kind === "rocks") {
      const n = 4 + Math.floor(r() * 5);
      for (let i = 0; i < n; i++) {
        const a = r() * Math.PI * 2, d = Math.sqrt(r()) * size * 0.8;
        this.mounds.push({ x: Math.cos(a) * d, z: Math.sin(a) * d, r: size * (0.12 + r() * 0.2), h: Math.max(0.8, height * (0.25 + r() * 0.75)), steep: 2.5 });
      }
      this.shelf = size * 1.5;
    } else if (kind === "sandbar") {
      this.mounds.push({ x: 0, z: 0, r: size, h: Math.min(height, 2.2), steep: 0.6 });
      this.shelf = size * 2.2;
    } else {
      this.mounds.push({ x: 0, z: 0, r: size * 0.8, h: height, steep: 0.85 });
      const n = 2 + Math.floor(r() * 3);
      for (let i = 0; i < n; i++) {
        const a = r() * Math.PI * 2, d = size * (0.25 + r() * 0.4);
        this.mounds.push({ x: Math.cos(a) * d, z: Math.sin(a) * d, r: size * (0.3 + r() * 0.3), h: height * (0.35 + r() * 0.45), steep: 1.1 });
      }
      this.shelf = size * 1.9;
    }
    this.reach = this.shelf * 1.15;
  }

  /** The ground's height (m, 0 = sea level) at a point, local to the centre. */
  local(x: number, z: number) {
    const { size, kind, seed } = this.isle;
    const d = Math.hypot(x, z);
    if (d > this.reach) return DEEP;
    const ang = Math.atan2(z, x);
    // a wobbly coastline
    const wob = 1 + 0.28 * (fbm(Math.cos(ang) * 1.3 + 5, Math.sin(ang) * 1.3 + 5, seed, 3) - 0.5) * 2;
    const D = d / (this.shelf * wob);
    // the shelf around it: sand going down into the deep
    let g = DEEP + (DEEP * -1 - (kind === "rocks" ? 1.2 : kind === "stack" ? 6 : 1.6)) * (1 - smoothstep(0.35, 1, D));
    if (kind === "sandbar") g = Math.max(g, -0.9 - 3 * smoothstep(0.2, 0.7, D));
    for (const m of this.mounds) {
      const md = Math.hypot(x - m.x, z - m.z) / (m.r * wob);
      if (md >= 1.6) continue;
      let p: number;
      if (m.steep >= 5) {
        // a sea stack: steep fluted sides, a rounded top
        const flute = 0.06 * (noise2(Math.atan2(z - m.z, x - m.x) * 3.2, 0.5, seed + 7) - 0.5);
        p = Math.pow(Math.max(0, 1 - Math.pow(md + flute, 3)), 0.42);
      }
      else p = Math.pow(Math.max(0, 1 - md * md), m.steep);
      let h = m.h * p;
      if (kind !== "sandbar" && p > 0) {
        const rough = ridged((x + 300) / (size * 0.35), (z + 300) / (size * 0.35), seed, 5);
        h += m.h * (m.steep >= 5 ? 0.12 : 0.42) * (rough - 0.45) * Math.min(1, p * 3);
        if (m.steep >= 5) h += m.h * 0.08 * (noise2(x * 0.3, z * 0.3, seed) - 0.5) * smoothstep(0.6, 1, md);
      }
      g = Math.max(g, h - (m.steep >= 5 ? 0 : 1.6 * smoothstep(0.85, 1.3, md)));
    }
    // beaches: the ground just above the water lies flatter
    if (g > -1.2 && g < 1.8 && kind !== "stack") g = g < 0 ? g : 1.8 * Math.pow(g / 1.8, 1.6);
    return g;
  }
  at(wx: number, wz: number) { return this.local(wx - this.isle.x, wz - this.isle.z); }
}

/** The highest ground at a point among all the islands (DEEP in the open). */
export function groundAt(shapes: Shape[], x: number, z: number) {
  let g = DEEP;
  for (const s of shapes) {
    if (Math.abs(x - s.isle.x) > s.reach || Math.abs(z - s.isle.z) > s.reach) continue;
    g = Math.max(g, s.at(x, z));
  }
  return g;
}

// (linear colours: they look lighter on screen)
const SAND = new THREE.Color(0.62, 0.53, 0.36), WET = new THREE.Color(0.28, 0.24, 0.17), ROCK = new THREE.Color(0.028, 0.027, 0.025), ROCK2 = new THREE.Color(0.075, 0.07, 0.062);
const GRASS = new THREE.Color(0.03, 0.075, 0.016), GRASS2 = new THREE.Color(0.075, 0.12, 0.028), FLOOR = new THREE.Color(0.42, 0.4, 0.3);

/** The island's ground as a mesh (world position), coloured by height and slope. */
export function groundMesh(shape: Shape) {
  const { isle } = shape;
  const ext = shape.reach * 2;
  const cell = clamp(ext / 200, 0.45, 4);
  const n = Math.min(320, Math.ceil(ext / cell));
  const geo = new THREE.PlaneGeometry(ext, ext, n, n);
  geo.rotateX(-Math.PI / 2);
  const pos = geo.attributes.position as THREE.BufferAttribute;
  for (let i = 0; i < pos.count; i++) pos.setY(i, shape.local(pos.getX(i), pos.getZ(i)));
  geo.computeVertexNormals();
  const nor = geo.attributes.normal as THREE.BufferAttribute;
  const col = new Float32Array(pos.count * 3);
  const c = new THREE.Color();
  for (let i = 0; i < pos.count; i++) {
    const x = pos.getX(i), y = pos.getY(i), z = pos.getZ(i), up = nor.getY(i);
    const v = noise2(x * 0.15 + isle.seed, z * 0.15, isle.seed + 3);
    if (y < -1.5) c.copy(FLOOR).lerp(SAND, 0.4);
    else if (y < 0.25) c.copy(WET).lerp(SAND, smoothstep(-1.5, 0.25, y) * 0.6);
    else if (y < 2.2 && up > 0.8 && isle.kind !== "stack") c.copy(SAND);
    else c.copy(ROCK).lerp(ROCK2, v);
    // green on gentle ground above the beach (not on bare stacks)
    const greenOK = isle.kind === "island" || (isle.kind === "stack" && y > isle.height * 0.85);
    if (greenOK && y > 2 && up > 0.7) {
      const g = smoothstep(0.7, 0.86, up) * smoothstep(2.5, 5, y);
      c.lerp(GRASS.clone().lerp(GRASS2, fbm(x * 0.05, z * 0.05, isle.seed + 9, 3)), g);
    }
    c.multiplyScalar(0.85 + 0.3 * v);
    col.set([c.r, c.g, c.b], i * 3);
  }
  geo.setAttribute("color", new THREE.BufferAttribute(col, 3));
  geo.translate(isle.x, 0, isle.z);
  geo.computeBoundingSphere();
  return geo;
}

/** A palm: a leaning, curving trunk and a crown of drooping fronds. */
export function palmGeometry() {
  const parts: THREE.BufferGeometry[] = [];
  const pts: THREE.Vector3[] = [];
  for (let i = 0; i <= 8; i++) { const t = i / 8; pts.push(new THREE.Vector3(Math.pow(t, 1.8) * 1.6, t * 7.5, 0)); }
  const trunk = new THREE.TubeGeometry(new THREE.CatmullRomCurve3(pts), 10, 0.17, 6);
  paint(trunk, new THREE.Color(0.11, 0.075, 0.045));
  parts.push(trunk);
  const top = pts[pts.length - 1];
  for (let i = 0; i < 9; i++) {
    const a = (i / 9) * Math.PI * 2 + (i % 2) * 0.2;
    const frond = new THREE.BufferGeometry();
    const v: number[] = [], seg = 8, len = 3.6 + (i % 3) * 0.4;
    for (let s = 0; s <= seg; s++) {
      const t = s / seg, w = Math.sin(t * Math.PI) * 0.65 + 0.05;
      const r = t * len, y = 0.6 * t - 2.1 * t * t;
      v.push(r, y, -w, r, y + 0.12, 0, r, y, w);
    }
    const idx: number[] = [];
    for (let s = 0; s < seg; s++) { const o = s * 3; idx.push(o, o + 3, o + 1, o + 1, o + 3, o + 4, o + 1, o + 4, o + 2, o + 2, o + 4, o + 5); }
    frond.setAttribute("position", new THREE.Float32BufferAttribute(v, 3));
    frond.setIndex(idx);
    frond.rotateY(a);
    frond.translate(top.x, top.y, top.z);
    frond.computeVertexNormals();
    paint(frond, new THREE.Color(0.035, 0.09, 0.018).lerp(new THREE.Color(0.085, 0.14, 0.03), (i % 3) / 3));
    parts.push(frond);
  }
  return merge(parts);
}

/** Where the palms of an island stand (and how big each is). */
export function palmSpots(shape: Shape) {
  const { isle } = shape;
  const out: { x: number; y: number; z: number; s: number; a: number }[] = [];
  if (isle.kind !== "island" || isle.palms <= 0) return out;
  const r = rng(isle.seed * 31 + 5);
  const want = Math.min(160, Math.round(clamp(isle.palms, 0, 1) * isle.size * isle.size / 140));
  for (let tries = 0; out.length < want && tries < want * 25; tries++) {
    const a = r() * Math.PI * 2, d = Math.sqrt(r()) * shape.reach;
    const x = isle.x + Math.cos(a) * d, z = isle.z + Math.sin(a) * d;
    const y = shape.at(x, z);
    if (y < 1.1 || y > isle.height * 0.7) continue;
    const slope = Math.abs(shape.at(x + 1, z) - y) + Math.abs(shape.at(x, z + 1) - y);
    if (slope > 0.95) continue;
    // in groves: denser where a noise says so
    if (fbm(x * 0.02, z * 0.02, isle.seed + 4, 3) < 0.42 && r() < 0.6) continue;
    out.push({ x, y: y - 0.2, z, s: 0.75 + r() * 0.55, a: r() * Math.PI * 2 });
  }
  return out;
}

/** The outline of each island for the maps: cells above water. */
export function landMask(shape: Shape, px = 1.5) {
  const ext = shape.reach * 2, n = clamp(Math.ceil(ext / px), 8, 256);
  const cells = new Uint8Array(n * n);
  for (let j = 0; j < n; j++) for (let i = 0; i < n; i++) {
    const x = shape.isle.x - shape.reach + (i + 0.5) * (ext / n), z = shape.isle.z - shape.reach + (j + 0.5) * (ext / n);
    const g = shape.at(x, z);
    cells[j * n + i] = g > 0 ? 2 : g > -3 ? 1 : 0;
  }
  return { n, ext, cells };
}

function paint(g: THREE.BufferGeometry, c: THREE.Color) {
  const n = g.attributes.position.count, a = new Float32Array(n * 3);
  for (let i = 0; i < n; i++) a.set([c.r, c.g, c.b], i * 3);
  g.setAttribute("color", new THREE.BufferAttribute(a, 3));
}

/** Merges geometries that have position, normal and color. */
export function merge(parts: THREE.BufferGeometry[]) {
  const ps: number[] = [], ns: number[] = [], cs: number[] = [], idx: number[] = [];
  let base = 0;
  for (let g of parts) {
    if (!g.index) g = g.toNonIndexed();
    if (!g.attributes.normal) g.computeVertexNormals();
    const p = g.attributes.position, n = g.attributes.normal, c = g.attributes.color;
    for (let i = 0; i < p.count; i++) {
      ps.push(p.getX(i), p.getY(i), p.getZ(i));
      ns.push(n.getX(i), n.getY(i), n.getZ(i));
      cs.push(c ? c.getX(i) : 1, c ? c.getY(i) : 1, c ? c.getZ(i) : 1);
    }
    if (g.index) for (let i = 0; i < g.index.count; i++) idx.push(g.index.getX(i) + base);
    else for (let i = 0; i < p.count; i++) idx.push(i + base);
    base += p.count;
  }
  const out = new THREE.BufferGeometry();
  out.setAttribute("position", new THREE.Float32BufferAttribute(ps, 3));
  out.setAttribute("normal", new THREE.Float32BufferAttribute(ns, 3));
  out.setAttribute("color", new THREE.Float32BufferAttribute(cs, 3));
  out.setIndex(idx);
  return out;
}

/** An island record's data as an island (defaults for what is missing). */
export function isleOf(id: string, d: Record<string, unknown>): Isle {
  const n = (v: unknown, def: number) => (typeof v === "number" && Number.isFinite(v) ? v : def);
  const kind = (KINDS as string[]).includes(String(d.kind)) ? (d.kind as Kind) : "island";
  const size = clamp(n(d.size, kind === "stack" ? 14 : kind === "rocks" ? 60 : 120), 4, 1500);
  const height = clamp(n(d.height, kind === "stack" ? 28 : kind === "rocks" ? 3 : kind === "sandbar" ? 1.2 : 30), 0.5, 400);
  let seed = n(d.seed, NaN);
  if (!Number.isFinite(seed)) { seed = 0; for (const ch of id) seed = (seed * 31 + ch.charCodeAt(0)) | 0; }
  return { id, name: String(d.name || ""), x: n(d.x, 0), z: n(d.z, 0), size, height, kind, palms: clamp(n(d.palms, kind === "island" ? 0.5 : 0), 0, 1), seed: Math.abs(seed) % 100000, note: String(d.note || "") };
}
