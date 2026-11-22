// The sky: where the sun is at a given hour, the colours of the air and
// light, and the GLSL that paints the sky (gradient, sun, clouds). The
// water reflects the same sky, so both use the one function here.

import * as THREE from "three";
import { clamp, lerp, smoothstep } from "./noise";

export type Weather = "clear" | "cloudy" | "overcast";

export type SkyLook = {
  sun: THREE.Vector3; // toward the sun
  sunColor: THREE.Color; // its light, already dimmed by cloud
  zenith: THREE.Color;
  horizon: THREE.Color;
  cover: number; // cloud cover 0–1
  haze: number; // how quickly the distance fades
  ambient: number;
};

const mix = (a: THREE.Color, b: THREE.Color, t: number) => a.clone().lerp(b, clamp(t, 0, 1));
const C = (r: number, g: number, b: number) => new THREE.Color(r, g, b);

/** The sky at an hour (5–19.5, local sun time) in a weather. North = -z. */
export function skyLook(hour: number, weather: Weather): SkyLook {
  const h = clamp(hour, 5, 19.5);
  // the sun rises in the east (+x) at 6, is highest in the south at 12
  const day = (h - 6) / 12; // 0 at sunrise, 1 at sunset
  // (no night: at the ends of the day the sun sits just over the horizon)
  const elev = Math.max(0.03, Math.sin(clamp(day, 0, 1) * Math.PI) * (62 * Math.PI) / 180 + 0.02);
  const azim = (90 + day * 180) * (Math.PI / 180); // compass bearing
  const sun = new THREE.Vector3(Math.sin(azim) * Math.cos(elev), Math.sin(elev), -Math.cos(azim) * Math.cos(elev)).normalize();
  const cover = weather === "clear" ? 0.18 : weather === "cloudy" ? 0.5 : 0.92;
  const low = 1 - smoothstep(0.02, 0.45, sun.y); // golden light near the horizon
  const warm = C(1.0, 0.52, 0.26), white = C(1.0, 0.95, 0.88);
  const sunColor = mix(white, warm, low).multiplyScalar(lerp(3.2, 0.9, low) * (1 - 0.78 * cover * cover));
  let zenith = mix(C(0.16, 0.36, 0.72), C(0.1, 0.16, 0.34), low * 0.9);
  let horizon = mix(C(0.66, 0.78, 0.88), C(0.98, 0.66, 0.44), low * 0.85);
  const grey = C(0.62, 0.66, 0.7).multiplyScalar(lerp(1, 0.55, low));
  zenith = mix(zenith, grey.clone().multiplyScalar(0.92), cover * cover * 1.05);
  horizon = mix(horizon, grey, cover * cover * 0.9);
  return { sun, sunColor, zenith, horizon, cover, haze: lerp(0.00042, 0.00072, cover), ambient: lerp(1, 0.6, low) };
}

/** Uniforms every sky-aware material shares (one object, updated in place). */
export function skyUniforms() {
  return {
    uSun: { value: new THREE.Vector3(0, 1, 0) },
    uSunColor: { value: new THREE.Color(1, 1, 1) },
    uZenith: { value: new THREE.Color() },
    uHorizon: { value: new THREE.Color() },
    uCover: { value: 0.4 },
    uHaze: { value: 0.0002 },
    uTime: { value: 0 },
    uWind: { value: new THREE.Vector2(1, 0) },
  };
}
export type SkyUniforms = ReturnType<typeof skyUniforms>;

export function applySky(u: SkyUniforms, look: SkyLook, windDeg: number) {
  u.uSun.value.copy(look.sun);
  u.uSunColor.value.copy(look.sunColor);
  u.uZenith.value.copy(look.zenith);
  u.uHorizon.value.copy(look.horizon);
  u.uCover.value = look.cover;
  u.uHaze.value = look.haze;
  const a = ((windDeg + 180) * Math.PI) / 180;
  u.uWind.value.set(Math.sin(a), -Math.cos(a));
}

/** GLSL: the sky seen in a direction (clouds with `octaves` of detail). */
export const SKY_GLSL = /* glsl */ `
uniform vec3 uSun;
uniform vec3 uSunColor;
uniform vec3 uZenith;
uniform vec3 uHorizon;
uniform float uCover;
uniform float uHaze;
uniform float uTime;
uniform vec2 uWind;

float hash12(vec2 p) { vec3 p3 = fract(vec3(p.xyx) * 0.1031); p3 += dot(p3, p3.yzx + 33.33); return fract((p3.x + p3.y) * p3.z); }
float vnoise(vec2 p) {
  vec2 i = floor(p), f = fract(p);
  vec2 u = f * f * (3.0 - 2.0 * f);
  return mix(mix(hash12(i), hash12(i + vec2(1, 0)), u.x), mix(hash12(i + vec2(0, 1)), hash12(i + vec2(1, 1)), u.x), u.y);
}
float cloudFbm(vec2 p, int octaves) {
  float s = 0.0, a = 0.5;
  mat2 m = mat2(1.6, 1.2, -1.2, 1.6);
  for (int i = 0; i < 6; i++) { if (i >= octaves) break; s += a * vnoise(p); p = m * p; a *= 0.5; }
  return s;
}

// The clear air: blue above, pale at the horizon, a glow around the sun.
vec3 airColor(vec3 d) {
  float y = max(d.y, 0.0);
  vec3 c = mix(uHorizon, uZenith, pow(y, 0.45));
  float mu = max(dot(d, uSun), 0.0);
  float glow = pow(mu, 8.0) * 0.35 + pow(mu, 64.0) * 0.6;
  c += uSunColor * glow * (0.25 + 0.75 * (1.0 - uCover)) * 0.35;
  return c;
}

// Cloud cover in a direction (0–1) and its light.
vec4 clouds(vec3 d, int octaves) {
  if (d.y < 0.01) return vec4(0.0);
  vec2 p = d.xz / (d.y + 0.06) * 1.4;
  p += uWind * uTime * 0.004;
  float n = cloudFbm(p * 0.9, octaves);
  float thr = 1.0 - uCover;
  float c = smoothstep(thr * 0.62 + 0.12, thr * 0.62 + 0.42, n);
  // lit from the sun's side: a second look a little toward it
  float n2 = cloudFbm(p * 0.9 + uSun.xz * 0.08, octaves);
  float lit = clamp(0.55 + (n - n2) * 3.5, 0.0, 1.0);
  float mu = max(dot(d, uSun), 0.0);
  vec3 dark = mix(uHorizon, vec3(dot(uHorizon, vec3(0.333))), 0.6) * 0.72;
  vec3 bright = uSunColor * 0.34 + uZenith * 0.25 + vec3(0.42);
  vec3 col = mix(dark, bright, lit) + uSunColor * pow(mu, 12.0) * 0.25 * (1.0 - c);
  float fadeH = smoothstep(0.0, 0.18, d.y);
  return vec4(col, c * fadeH);
}

vec3 skyColor(vec3 d, int octaves, bool withSun) {
  vec3 c = airColor(d);
  vec4 cl = clouds(d, octaves);
  c = mix(c, cl.rgb, cl.a * 0.92);
  if (withSun) {
    float mu = dot(d, uSun);
    float disk = smoothstep(0.99955, 0.99975, mu);
    c += uSunColor * disk * 6.0 * (1.0 - cl.a) * (1.0 - uCover * 0.8);
  }
  // the low haze over the sea
  float h = 1.0 - smoothstep(-0.02, 0.12, d.y);
  return mix(c, uHorizon, h * 0.85);
}
`;

/** The sky dome (drawn first, around the eye). */
export function skyDome(u: SkyUniforms) {
  const mat = new THREE.ShaderMaterial({
    uniforms: u as unknown as Record<string, THREE.IUniform>,
    vertexShader: /* glsl */ `
      varying vec3 vDir;
      void main() {
        vDir = position;
        vec4 p = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
        gl_Position = p.xyww;
      }`,
    fragmentShader: SKY_GLSL + /* glsl */ `
      varying vec3 vDir;
      void main() {
        vec3 d = normalize(vDir);
        vec3 c = skyColor(d, 5, true);
        gl_FragColor = vec4(c, 1.0);
        #include <tonemapping_fragment>
        #include <colorspace_fragment>
      }`,
    side: THREE.BackSide,
    depthWrite: false,
  });
  const m = new THREE.Mesh(new THREE.SphereGeometry(1000, 48, 24), mat);
  m.frustumCulled = false;
  m.renderOrder = -10;
  return m;
}
