// The sky over a whole day: where the sun and the moon are at an hour,
// the colours of the air from midnight through dawn and noon to dusk, the
// weather (cloud, rain, storm, sea fog), and the GLSL that paints it
// (gradient, glow, sun, moon, stars, clouds, lightning). The water
// reflects the same sky, so both use the one function here.

import * as THREE from "three";
import { clamp, lerp, smoothstep } from "./noise";

export type Weather = "clear" | "cloudy" | "overcast" | "rain" | "storm" | "fog";
export const WEATHERS: Weather[] = ["clear", "cloudy", "overcast", "rain", "storm", "fog"];

export type SkyLook = {
  sunPos: THREE.Vector3; // toward the sun (below the horizon at night)
  moon: THREE.Vector3; // toward the moon
  light: THREE.Vector3; // the light that shines now: the sun by day, the moon by night
  lightColor: THREE.Color; // its colour and strength, after the clouds
  glow: THREE.Color; // the glow around the sun (gold and rose at dawn and dusk)
  zenith: THREE.Color;
  horizon: THREE.Color;
  cover: number; // cloud cover 0–1
  haze: number; // how quickly the distance fades
  day: number; // 0 night … 1 day
  stars: number; // how much of the starry sky shows
  rain: number; // 0–1
  fog: number; // 0–1
  storm: boolean;
  exposure: number; // eyes adjust: brighter at night
};

const mix = (a: THREE.Color, b: THREE.Color, t: number) => a.clone().lerp(b, clamp(t, 0, 1));
const C = (r: number, g: number, b: number) => new THREE.Color(r, g, b);
const LAT = (18 * Math.PI) / 180; // a tropical sea: the sun passes high, a little to the south

/** Toward the sun at an hour (0–24, local sun time). North = −z, east = +x. */
export function sunAt(hour: number, lat = LAT) {
  const H = ((hour - 12) / 24) * Math.PI * 2; // the hour angle
  return new THREE.Vector3(-Math.sin(H), Math.cos(lat) * Math.cos(H), Math.sin(lat) * Math.cos(H)).normalize();
}

/** Toward the moon: full, rising in the east as the sun sets, riding low
 *  across the south (at most about 24° up, where a boat's camera sees it). */
export function moonAt(hour: number) {
  return sunAt(hour + 12.4, (66 * Math.PI) / 180);
}

/** Colours through the day, by how high the sun is (its y). */
const KEYS: { y: number; zen: THREE.Color; hor: THREE.Color; glow: THREE.Color }[] = [
  { y: -1, zen: C(0.003, 0.006, 0.016), hor: C(0.01, 0.017, 0.035), glow: C(0, 0, 0) },
  { y: -0.2, zen: C(0.004, 0.008, 0.022), hor: C(0.014, 0.022, 0.045), glow: C(0.02, 0.01, 0.02) },
  { y: -0.08, zen: C(0.03, 0.05, 0.12), hor: C(0.3, 0.17, 0.2), glow: C(0.55, 0.2, 0.12) },
  { y: 0.0, zen: C(0.08, 0.13, 0.3), hor: C(0.95, 0.48, 0.26), glow: C(2.2, 0.85, 0.3) },
  { y: 0.12, zen: C(0.12, 0.25, 0.55), hor: C(0.95, 0.68, 0.46), glow: C(1.6, 0.95, 0.5) },
  { y: 0.35, zen: C(0.16, 0.36, 0.72), hor: C(0.66, 0.78, 0.88), glow: C(1, 0.95, 0.88) },
  { y: 1, zen: C(0.14, 0.33, 0.7), hor: C(0.62, 0.76, 0.88), glow: C(1, 0.96, 0.9) },
];
function keyed(y: number) {
  let i = 0;
  while (i < KEYS.length - 2 && y > KEYS[i + 1].y) i++;
  const a = KEYS[i], b = KEYS[i + 1], t = clamp((y - a.y) / (b.y - a.y), 0, 1);
  return { zen: mix(a.zen, b.zen, t), hor: mix(a.hor, b.hor, t), glow: mix(a.glow, b.glow, t) };
}

/** The sky at an hour (0–24) in a weather. */
export function skyLook(hour: number, weather: Weather): SkyLook {
  const h = ((hour % 24) + 24) % 24;
  const sunPos = sunAt(h);
  // a moon a little more than half a day behind the sun: up through the night
  const moon = moonAt(h);
  const w = WEATHERS.includes(weather) ? weather : "cloudy";
  const cover = { clear: 0.16, cloudy: 0.5, overcast: 0.9, rain: 0.96, storm: 1, fog: 0.72 }[w];
  const rain = w === "rain" ? 0.6 : w === "storm" ? 1 : 0;
  const fog = w === "fog" ? 1 : w === "rain" ? 0.25 : w === "storm" ? 0.35 : 0;
  const k = keyed(sunPos.y);
  const day = smoothstep(-0.12, 0.08, sunPos.y);
  const low = 1 - smoothstep(0.02, 0.45, sunPos.y); // golden light near the horizon
  // the sun's light: white at noon, gold low down, gone below the horizon
  const sunLight = mix(C(1, 0.95, 0.88), C(1, 0.5, 0.24), low).multiplyScalar(lerp(3.2, 0.9, low) * smoothstep(-0.03, 0.06, sunPos.y));
  // the moon's: faint and blue
  const moonLight = C(0.07, 0.09, 0.14).multiplyScalar(smoothstep(-0.02, 0.15, moon.y) * (1 - day));
  const byDay = sunPos.y > -0.04 || moonLight.r < 0.01;
  const dim = (1 - 0.78 * cover * cover) * (1 - 0.35 * rain);
  const lightColor = (byDay ? sunLight : moonLight).multiplyScalar(dim);
  const light = byDay ? sunPos.clone() : moon.clone();
  if (light.y < 0.03) light.y = 0.03;
  light.normalize();
  // clouds and rain grey the sky; at night everything is dark
  const grey = C(0.62, 0.66, 0.7).multiplyScalar(lerp(1, 0.55, low) * lerp(0.03, 1, day) * (1 - 0.45 * rain));
  const zenith = mix(k.zen, grey.clone().multiplyScalar(0.92), cover * cover * 1.05);
  const horizon = mix(mix(k.hor, grey, cover * cover * 0.9), grey.clone().multiplyScalar(1.05), fog * 0.85);
  const glow = k.glow.clone().multiplyScalar(1 - 0.7 * cover * cover);
  const haze = lerp(0.00042, 0.00072, cover) + fog * 0.0058 + rain * 0.0006;
  const stars = smoothstep(-0.05, -0.22, sunPos.y) * (1 - cover * 0.9) * (1 - fog);
  return { sunPos, moon, light, lightColor, glow, zenith, horizon, cover, haze, day, stars, rain, fog, storm: w === "storm", exposure: lerp(2.6, 0.85, smoothstep(-0.15, 0.1, sunPos.y)) };
}

/** Uniforms every sky-aware material shares (one object, updated in place).
 *  uSun/uSunColor are the light that shines now (the moon's at night). */
export function skyUniforms() {
  return {
    uSun: { value: new THREE.Vector3(0, 1, 0) },
    uSunColor: { value: new THREE.Color(1, 1, 1) },
    uSunPos: { value: new THREE.Vector3(0, 1, 0) },
    uMoon: { value: new THREE.Vector3(0, -1, 0) },
    uGlow: { value: new THREE.Color(1, 1, 1) },
    uZenith: { value: new THREE.Color() },
    uHorizon: { value: new THREE.Color() },
    uCover: { value: 0.4 },
    uHaze: { value: 0.0002 },
    uDay: { value: 1 },
    uStars: { value: 0 },
    uRain: { value: 0 },
    uFlash: { value: 0 },
    uTime: { value: 0 },
    uWind: { value: new THREE.Vector2(1, 0) },
  };
}
export type SkyUniforms = ReturnType<typeof skyUniforms>;

export function applySky(u: SkyUniforms, look: SkyLook, windDeg: number) {
  u.uSun.value.copy(look.light);
  u.uSunColor.value.copy(look.lightColor);
  u.uSunPos.value.copy(look.sunPos);
  u.uMoon.value.copy(look.moon);
  u.uGlow.value.copy(look.glow);
  u.uZenith.value.copy(look.zenith);
  u.uHorizon.value.copy(look.horizon);
  u.uCover.value = look.cover;
  u.uHaze.value = look.haze;
  u.uDay.value = look.day;
  u.uStars.value = look.stars;
  u.uRain.value = look.rain;
  const a = ((windDeg + 180) * Math.PI) / 180;
  u.uWind.value.set(Math.sin(a), -Math.cos(a));
}

/** GLSL: the sky seen in a direction (clouds with `octaves` of detail). */
export const SKY_GLSL = /* glsl */ `
uniform vec3 uSun;
uniform vec3 uSunColor;
uniform vec3 uSunPos;
uniform vec3 uMoon;
uniform vec3 uGlow;
uniform vec3 uZenith;
uniform vec3 uHorizon;
uniform float uCover;
uniform float uHaze;
uniform float uDay;
uniform float uStars;
uniform float uRain;
uniform float uFlash;
uniform float uTime;
uniform vec2 uWind;

float hash12(vec2 p) { vec3 p3 = fract(vec3(p.xyx) * 0.1031); p3 += dot(p3, p3.yzx + 33.33); return fract((p3.x + p3.y) * p3.z); }
float hash13(vec3 p3) { p3 = fract(p3 * 0.1031); p3 += dot(p3, p3.zyx + 31.32); return fract((p3.x + p3.y) * p3.z); }
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

// The clear air: blue above, pale at the horizon, a glow around the sun
// (gold and rose when it is low, a little even after it has set).
vec3 airColor(vec3 d) {
  float y = max(d.y, 0.0);
  vec3 c = mix(uHorizon, uZenith, pow(y, 0.45));
  float mu = max(dot(d, uSunPos), 0.0);
  float wide = pow(mu, 3.0) * (1.0 - smoothstep(0.0, 0.5, y)); // along the horizon toward the sun
  float glow = pow(mu, 8.0) * 0.35 + pow(mu, 64.0) * 0.6 + wide * 0.25;
  c += uGlow * glow * (0.25 + 0.75 * (1.0 - uCover)) * 0.45;
  // the moon's halo
  float mm = max(dot(d, uMoon), 0.0);
  c += vec3(0.5, 0.55, 0.65) * (pow(mm, 200.0) * 0.12 + pow(mm, 20.0) * 0.02) * (1.0 - uDay) * smoothstep(-0.05, 0.05, uMoon.y);
  return c;
}

// Stars: a few thousand, twinkling, thicker away from the horizon.
vec3 stars(vec3 d) {
  vec3 p = d * 260.0, i = floor(p);
  float h = hash13(i);
  if (h < 0.9975) return vec3(0.0);
  vec3 c = i + 0.5 + (vec3(hash13(i + 1.7), hash13(i + 3.1), hash13(i + 5.3)) - 0.5) * 0.7;
  float s = smoothstep(0.32, 0.0, length(p - c));
  float tw = 0.75 + 0.25 * sin(uTime * (2.0 + h * 40.0) + h * 300.0);
  vec3 tint = mix(vec3(0.8, 0.85, 1.0), vec3(1.0, 0.9, 0.75), hash13(i + 9.1));
  return tint * s * tw * (h - 0.9975) * 900.0 * smoothstep(0.02, 0.3, d.y);
}

// Cloud cover in a direction (0–1) and its light: by day lit by the sun,
// at night only by the moon (dark shapes against the stars).
vec4 clouds(vec3 d, int octaves) {
  if (d.y < 0.01) return vec4(0.0);
  vec2 p = d.xz / (d.y + 0.06) * 1.4;
  p += uWind * uTime * 0.004;
  float n = cloudFbm(p * 0.9, octaves);
  float thr = 1.0 - uCover;
  float c = smoothstep(thr * 0.62 + 0.12, thr * 0.62 + 0.42, n);
  float n2 = cloudFbm(p * 0.9 + uSunPos.xz * 0.08, octaves);
  float lit = clamp(0.55 + (n - n2) * 3.5, 0.0, 1.0);
  float mu = max(dot(d, uSunPos), 0.0);
  vec3 dark = mix(uHorizon, vec3(dot(uHorizon, vec3(0.333))), 0.6) * 0.72;
  vec3 bright = uGlow * 0.3 * uDay + uZenith * 0.25 + vec3(0.42) * uDay + vec3(0.035, 0.04, 0.05) * (1.0 - uDay);
  vec3 col = mix(dark, bright, lit) + uGlow * pow(mu, 12.0) * 0.3 * (1.0 - c) * uDay;
  // rain clouds hang darker
  col *= 1.0 - 0.35 * uRain;
  float fadeH = smoothstep(0.0, 0.18, d.y);
  return vec4(col, c * fadeH);
}

// The sky as rough water sees it: the clouds blurred into their average.
vec3 skyMean(vec3 d) {
  vec3 c = airColor(d);
  vec3 dark = mix(uHorizon, vec3(dot(uHorizon, vec3(0.333))), 0.6) * 0.72;
  vec3 bright = uGlow * 0.3 * uDay + uZenith * 0.25 + vec3(0.42) * uDay + vec3(0.035, 0.04, 0.05) * (1.0 - uDay);
  c = mix(c, mix(dark, bright, 0.55) * (1.0 - 0.35 * uRain), uCover * 0.85 * smoothstep(0.0, 0.18, d.y));
  float h = 1.0 - smoothstep(-0.02, 0.12, d.y);
  return mix(c, uHorizon, h * 0.85) + vec3(0.8, 0.85, 1.0) * uFlash * 0.6;
}

vec3 skyColor(vec3 d, int octaves, bool withSun) {
  vec3 c = airColor(d);
  if (withSun) c += stars(d) * uStars;
  vec4 cl = clouds(d, octaves);
  if (withSun) {
    // the moon: a pale disc with darker seas, behind thin cloud
    float mm = dot(d, uMoon);
    if (mm > 0.9995 && uMoon.y > -0.02) {
      vec3 side = normalize(cross(uMoon, vec3(0.0, 1.0, 0.0)));
      vec3 up = cross(side, uMoon);
      vec2 q = vec2(dot(d, side), dot(d, up)) / 0.026;
      float seas = smoothstep(0.45, 0.75, vnoise(q * 3.0 + 7.0)) * 0.35 + vnoise(q * 9.0) * 0.1;
      float disc = smoothstep(0.99962, 0.9997, mm);
      c = mix(c, vec3(0.95, 0.95, 0.88) * (1.0 - seas) * mix(0.25, 1.0, 1.0 - uDay), disc * (1.0 - cl.a * 0.85));
    }
    // the sun: a bright disc with a corona, showing through thin cloud
    float mu = dot(d, uSunPos);
    float disc = smoothstep(0.99985, 0.99992, mu);
    float corona = pow(max(mu, 0.0), 900.0) * 1.4 + pow(max(mu, 0.0), 120.0) * 0.25;
    float through = 1.0 - cl.a * (0.55 + 0.4 * uCover);
    c += uGlow * (disc * 8.0 + corona) * through * smoothstep(-0.02, 0.02, uSunPos.y);
  }
  c = mix(c, cl.rgb, cl.a * 0.92);
  // the low haze over the sea
  float h = 1.0 - smoothstep(-0.02, 0.12, d.y);
  c = mix(c, uHorizon, h * 0.85);
  // lightning lights the whole sky for a moment
  return c + vec3(0.8, 0.85, 1.0) * uFlash * (0.4 + 0.6 * cl.a);
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
