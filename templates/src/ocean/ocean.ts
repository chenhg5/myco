// The sea's surface: a disc of rings around the eye (fine near, coarse
// far), moved by the Gerstner waves, and a shader for how water looks:
// the sky reflected (more at a glancing angle), the colour of the water
// (turquoise over sand, deep teal over the deep), light through the
// crests, the sun's glitter, and foam (breaking crests, surf on the
// shallows, the boat's wake), fading into the haze.

import * as THREE from "three";
import { SKY_GLSL, type SkyUniforms } from "./sky";
import { GEO_WAVES } from "./waves";
import { HULL_GLSL } from "./boat";

export const SEABED_LOW = -12, SEABED_SPAN = 14; // what the seabed texture holds (m)

/** Rings of vertices around the centre; each knows how far apart it is from its neighbours. */
function discGeometry(r0 = 0.4, far = 7000, seg = 220, grow = 1.034) {
  const pos: number[] = [0, 0, 0], sp: number[] = [r0], idx: number[] = [];
  const rings: number[] = [];
  for (let r = r0; r < far; r *= grow) rings.push(r);
  rings.push(far * 1.6);
  rings.forEach((r, i) => {
    for (let s = 0; s < seg; s++) {
      const a = (s / seg) * Math.PI * 2;
      pos.push(Math.cos(a) * r, 0, Math.sin(a) * r);
      sp.push(Math.max(r * (grow - 1), (2 * Math.PI * r) / seg));
    }
    const base = 1 + i * seg;
    if (i === 0) for (let s = 0; s < seg; s++) idx.push(0, base + ((s + 1) % seg), base + s);
    else {
      const prev = base - seg;
      for (let s = 0; s < seg; s++) {
        const a = prev + s, b = prev + ((s + 1) % seg), c = base + s, d = base + ((s + 1) % seg);
        idx.push(a, b, c, b, d, c);
      }
    }
  });
  const g = new THREE.BufferGeometry();
  g.setAttribute("position", new THREE.Float32BufferAttribute(pos, 3));
  g.setAttribute("spacing", new THREE.Float32BufferAttribute(sp, 1));
  g.setIndex(idx);
  return g;
}

/** A tiling foam texture: bubbly cells and streaks (made here, not loaded). */
function foamTexture(size = 256) {
  const data = new Uint8Array(size * size * 4);
  const pts: [number, number][] = [];
  let s = 12345;
  const rnd = () => ((s = (s * 1103515245 + 12345) & 0x7fffffff) / 0x7fffffff);
  for (let i = 0; i < 90; i++) pts.push([rnd() * size, rnd() * size]);
  const tri = (x: number, y: number, p: number) => {
    // a cheap tileable value noise of period p
    const xi = Math.floor(x), yi = Math.floor(y), fx = x - xi, fy = y - yi;
    const h = (a: number, b: number) => { const v = Math.sin(((a % p) + p) % p * 127.1 + (((b % p) + p) % p) * 311.7) * 43758.5453; return v - Math.floor(v); };
    const u = fx * fx * (3 - 2 * fx), w = fy * fy * (3 - 2 * fy);
    return h(xi, yi) * (1 - u) * (1 - w) + h(xi + 1, yi) * u * (1 - w) + h(xi, yi + 1) * (1 - u) * w + h(xi + 1, yi + 1) * u * w;
  };
  for (let y = 0; y < size; y++) for (let x = 0; x < size; x++) {
    let d1 = 1e9, d2 = 1e9;
    for (const [px, py] of pts) {
      let dx = Math.abs(x - px), dy = Math.abs(y - py);
      dx = Math.min(dx, size - dx); dy = Math.min(dy, size - dy);
      const d = dx * dx + dy * dy;
      if (d < d1) { d2 = d1; d1 = d; } else if (d < d2) d2 = d;
    }
    const cell = Math.min(1, (Math.sqrt(d2) - Math.sqrt(d1)) / 22); // soft rims round bubbles
    const n = (tri(x / 32, y / 32, size / 32) * 0.45 + tri(x / 16, y / 16, size / 16) * 0.3 + tri(x / 8, y / 8, size / 8) * 0.17 + tri(x / 4, y / 4, size / 4) * 0.08);
    const v = Math.max(0, Math.min(1, n * 1.15 * (0.75 + 0.25 * (1 - cell)) - 0.05));
    const i = (y * size + x) * 4;
    data[i] = v * 255; data[i + 1] = n * 255; data[i + 2] = (1 - cell) * 255; data[i + 3] = 255;
  }
  const t = new THREE.DataTexture(data, size, size, THREE.RGBAFormat);
  t.wrapS = t.wrapT = THREE.RepeatWrapping;
  t.magFilter = THREE.LinearFilter;
  t.minFilter = THREE.LinearMipmapLinearFilter;
  t.generateMipmaps = true;
  t.needsUpdate = true;
  return t;
}

/** Ripples too fine for the mesh: a tiling field of many small waves of
 *  every direction (its slopes in red and green). Mipmaps calm it with
 *  distance, so the far sea does not shimmer. */
function rippleTexture(size = 256) {
  let s = 777;
  const rnd = () => ((s = (s * 1103515245 + 12345) & 0x7fffffff) / 0x7fffffff);
  const waves: { kx: number; ky: number; a: number; p: number }[] = [];
  for (let i = 0; i < 140; i++) {
    const m = 2 + Math.pow(rnd(), 1.6) * 42, ang = rnd() * Math.PI * 2;
    const kx = Math.round(Math.cos(ang) * m), ky = Math.round(Math.sin(ang) * m);
    if (!kx && !ky) continue;
    waves.push({ kx, ky, a: Math.pow(Math.hypot(kx, ky), -1.75), p: rnd() * Math.PI * 2 });
  }
  const sx = new Float32Array(size * size), sy = new Float32Array(size * size);
  let max = 0;
  for (const w of waves) {
    const fx = (2 * Math.PI * w.kx) / size, fy = (2 * Math.PI * w.ky) / size;
    // the slope of a(sin) is a·k·cos: per texel, along x and y
    const ax = w.a * 2 * Math.PI * w.kx, ay = w.a * 2 * Math.PI * w.ky;
    for (let y = 0; y < size; y++) for (let x = 0; x < size; x++) {
      const c = Math.cos(fx * x + fy * y + w.p);
      sx[y * size + x] += ax * c; sy[y * size + x] += ay * c;
    }
  }
  for (let i = 0; i < sx.length; i++) max = Math.max(max, Math.abs(sx[i]), Math.abs(sy[i]));
  const data = new Uint8Array(size * size * 4);
  for (let i = 0; i < sx.length; i++) {
    data[i * 4] = Math.round((0.5 + 0.5 * sx[i] / max) * 255);
    data[i * 4 + 1] = Math.round((0.5 + 0.5 * sy[i] / max) * 255);
    data[i * 4 + 3] = 255;
  }
  const t = new THREE.DataTexture(data, size, size, THREE.RGBAFormat);
  t.wrapS = t.wrapT = THREE.RepeatWrapping;
  t.magFilter = THREE.LinearFilter;
  t.minFilter = THREE.LinearMipmapLinearFilter;
  t.generateMipmaps = true;
  t.anisotropy = 4;
  t.needsUpdate = true;
  return t;
}

export function oceanMaterial(sky: SkyUniforms) {
  const uniforms = {
    ...sky,
    uWA: { value: new Float32Array(GEO_WAVES * 4) },
    uWB: { value: new Float32Array(GEO_WAVES * 4) },
    uRipple: { value: rippleTexture() },
    uRippleAmt: { value: 1 },
    uCenter: { value: new THREE.Vector2() },
    uPix: { value: 0.001 },
    uFoamAmt: { value: 0.5 },
    uFoamTex: { value: foamTexture() },
    uSeabed: { value: null as THREE.Texture | null },
    uSeabedCenter: { value: new THREE.Vector2() },
    uSeabedSize: { value: 1600 },
    uWake: { value: null as THREE.Texture | null },
    uWakeCenter: { value: new THREE.Vector2() },
    uWakeSize: { value: 240 },
    uBoatInv: { value: new THREE.Matrix4() },
  };
  const mat = new THREE.ShaderMaterial({
    uniforms: uniforms as unknown as Record<string, THREE.IUniform>,
    transparent: true,
    vertexShader: /* glsl */ `
      #define GEO ${GEO_WAVES}
      uniform vec4 uWA[GEO];
      uniform vec4 uWB[GEO];
      uniform vec2 uCenter;
      uniform float uTime;
      attribute float spacing;
      varying vec3 vWorld;
      varying vec2 vFlat;
      void main() {
        vec2 p = position.xz + uCenter;
        vec3 d = vec3(0.0);
        for (int i = 0; i < GEO; i++) {
          vec4 A = uWA[i]; vec4 B = uWB[i];
          float L = 6.2831853 / A.z;
          float f = smoothstep(spacing * 2.5, spacing * 6.0, L);
          float th = A.z * dot(A.xy, p) - A.w * uTime + B.z;
          float c = cos(th), s = sin(th);
          d.xz += f * B.y * B.x * A.xy * c;
          d.y += f * B.x * s;
        }
        vec3 wp = vec3(p.x + d.x, d.y, p.y + d.z);
        vWorld = wp;
        vFlat = p;
        gl_Position = projectionMatrix * viewMatrix * vec4(wp, 1.0);
      }`,
    fragmentShader: SKY_GLSL + HULL_GLSL + /* glsl */ `
      #define GEO ${GEO_WAVES}
      uniform vec4 uWA[GEO];
      uniform vec4 uWB[GEO];
      uniform sampler2D uRipple;
      uniform float uRippleAmt;
      uniform float uPix;
      uniform float uFoamAmt;
      uniform sampler2D uFoamTex;
      uniform sampler2D uSeabed;
      uniform vec2 uSeabedCenter;
      uniform float uSeabedSize;
      uniform sampler2D uWake;
      uniform vec2 uWakeCenter;
      uniform float uWakeSize;
      uniform mat4 uBoatInv;
      varying vec3 vWorld;
      varying vec2 vFlat;

      vec2 topUV(vec2 p, vec2 c, float size) { return vec2((p.x - c.x) / size + 0.5, -(p.y - c.y) / size + 0.5); }

      void main() {
        vec3 boatLocal = (uBoatInv * vec4(vWorld, 1.0)).xyz;
        if (insideHull(boatLocal)) discard;

        vec3 toEye = cameraPosition - vWorld;
        float dist = length(toEye);
        vec3 V = toEye / dist;
        float foot = dist * uPix / max(sqrt(abs(V.y)), 0.12);

        // the surface's slope here, from every wave (and fine ripples near by)
        vec2 p = vFlat;
        float nx = 0.0, nz = 0.0, comp = 0.0;
        for (int i = 0; i < GEO; i++) {
          vec4 A = uWA[i]; vec4 B = uWB[i];
          float L = 6.2831853 / A.z;
          float f = smoothstep(foot * 1.5, foot * 5.0, L);
          float th = A.z * dot(A.xy, p) - A.w * uTime + B.z;
          float c = cos(th), s = sin(th);
          float ka = A.z * B.x * f;
          nx -= A.x * ka * c;
          nz -= A.y * ka * c;
          comp += B.y * ka * s;
        }
        // fine ripples: two layers drifting with the wind, at two scales
        vec2 wd = uWind;
        vec2 r1 = texture2D(uRipple, p / 7.3 + wd * uTime * 0.045).rg * 2.0 - 1.0;
        vec2 r2 = texture2D(uRipple, mat2(0.8, 0.6, -0.6, 0.8) * p / 2.9 + vec2(-wd.y, wd.x) * uTime * 0.03 + wd * uTime * 0.06).rg * 2.0 - 1.0;
        vec2 r3 = texture2D(uRipple, mat2(0.6, -0.8, 0.8, 0.6) * p / 23.0 + wd * uTime * 0.02).rg * 2.0 - 1.0;
        float near = 1.0 - smoothstep(80.0, 900.0, dist);
        vec2 rip = (r1 * 0.16 + r2 * 0.09 * near + r3 * 0.22) * uRippleAmt;
        vec3 N = normalize(vec3(nx + rip.x, 1.0 - comp, nz + rip.y));
        float jac = 1.0 - comp; // small where the crest is pinched (it breaks)

        // the ground under the water
        vec2 sbUV = topUV(vWorld.xz, uSeabedCenter, uSeabedSize);
        float ground = -60.0;
        if (sbUV.x > 0.0 && sbUV.x < 1.0 && sbUV.y > 0.0 && sbUV.y < 1.0) {
          float e = texture2D(uSeabed, sbUV).r;
          ground = e > 0.002 ? e * ${SEABED_SPAN.toFixed(1)} + ${SEABED_LOW.toFixed(1)} : -60.0;
        }
        float depth = max(vWorld.y - ground, 0.0);

        // light on things here: the sky's and the sun's
        vec3 L = uSun;
        vec3 skyLight = mix(uHorizon, uZenith, 0.5) * 0.9;
        vec3 sunLight = uSunColor * max(L.y, 0.0);

        // the reflected sky
        vec3 R = reflect(-V, N);
        R.y = abs(R.y);
        vec3 refl = skyColor(R, 3, false);
        float F = 0.02 + 0.98 * pow(1.0 - max(dot(N, V), 0.0), 5.0);
        F = min(F, 0.72);

        float crest = clamp(vWorld.y * 0.45 + 0.25, 0.0, 1.0);
        // the water's own colour
        vec3 deep = vec3(0.008, 0.13, 0.16);
        vec3 shallow = vec3(0.06, 0.56, 0.52);
        vec3 sand = vec3(0.42, 0.52, 0.40);
        float sh = exp(-depth * 0.16) * (1.0 - smoothstep(7.0, 11.5, depth));
        vec3 body = mix(deep, shallow, sh);
        body = mix(body, sand, exp(-depth * 0.75) * 0.6);
        // light through the crests: brighter, greener where the wave stands up
        float through = pow(max(dot(-V, normalize(vec3(L.x, 0.25, L.z))), 0.0), 3.0);
        body += vec3(0.02, 0.2, 0.17) * crest * (0.35 + 1.2 * through);
        body *= skyLight * 0.75 + sunLight * 0.35;

        vec3 col = mix(body, refl, F);

        // the sun's glitter
        vec3 H = normalize(V + L);
        float shin = mix(900.0, 60.0, clamp(foot * 8.0, 0.0, 1.0));
        float spec = pow(max(dot(N, H), 0.0), shin) * (shin + 8.0) / 25.0;
        col += uSunColor * spec * F * 1.4 * (1.0 - uCover * 0.6);

        // foam: breaking crests, streaks, surf on the shallows, the wake
        vec2 fp = vWorld.xz;
        float t1 = texture2D(uFoamTex, fp * 0.045 + uWind * uTime * 0.012).r;
        float t2 = texture2D(uFoamTex, fp * 0.13 - uWind * uTime * 0.02).r;
        float tex = t1 * 0.6 + t2 * 0.5;
        float cap = smoothstep(0.78 - uFoamAmt * 0.32, 0.12, jac) * smoothstep(0.0, 0.3, uFoamAmt);
        float foam = cap * smoothstep(0.32, 0.7, tex + cap * 0.35);
        float streak = texture2D(uFoamTex, fp * 0.018 + vec2(uTime * 0.002)).g;
        foam += smoothstep(0.52, 0.86, streak * tex) * uFoamAmt * 0.38;
        foam += smoothstep(0.64, 0.95, tex) * crest * uFoamAmt * 0.45;
        // surf: bands running in to the shore, thick only at the water's edge
        float shore = 1.0 - smoothstep(0.0, 1.1, depth);
        float band = 0.5 + 0.5 * sin(depth * 5.0 - uTime * 1.6 + tex * 4.0);
        float edge = 1.0 - smoothstep(0.0, 0.35, depth);
        foam += max(edge * smoothstep(0.2, 0.6, tex + edge * 0.3), shore * smoothstep(0.45, 0.85, band * tex + shore * 0.25)) * (0.4 + 0.6 * uFoamAmt);
        vec2 wUV = topUV(vWorld.xz, uWakeCenter, uWakeSize);
        if (wUV.x > 0.0 && wUV.x < 1.0 && wUV.y > 0.0 && wUV.y < 1.0) {
          float w = texture2D(uWake, wUV).r;
          // white and churned near the stern, a lacy scar further back
          float fine = texture2D(uFoamTex, fp * 0.42 + vec2(0.31, 0.17) * uTime * 0.05).r;
          float lace = smoothstep(0.3, 0.72, fine * 0.7 + tex * 0.35 + w * 0.32);
          foam += clamp(w * 1.15, 0.0, 1.0) * lace;
        }
        foam = clamp(foam, 0.0, 1.0) * (1.0 - smoothstep(600.0, 2400.0, dist) * 0.7);
        vec3 foamCol = vec3(0.92, 0.95, 0.96) * (skyLight * 0.9 + sunLight * 0.45 * max(N.y, 0.0));
        col = mix(col, foamCol, foam * 0.92);

        // the haze over distance, toward the sky's own horizon colour
        vec3 flatDir = normalize(vec3(-V.x, 0.0, -V.z));
        vec3 fogC = mix(airColor(flatDir), uHorizon, 0.82);
        float fog = 1.0 - exp(-pow(dist * uHaze, 2.0));
        col = mix(col, fogC, fog);

        float alpha = smoothstep(0.0, 0.3, depth) ;
        alpha = max(alpha, foam);
        gl_FragColor = vec4(col, alpha);
        #include <tonemapping_fragment>
        #include <colorspace_fragment>
      }`,
  });
  return { mat, uniforms };
}
export type OceanUniforms = ReturnType<typeof oceanMaterial>["uniforms"];

export function oceanMesh(mat: THREE.ShaderMaterial) {
  const m = new THREE.Mesh(discGeometry(), mat);
  m.frustumCulled = false;
  return m;
}

/** What renders the islands' ground as heights (for the seabed texture). */
export function seabedMaterial() {
  return new THREE.ShaderMaterial({
    vertexShader: "varying float vy; void main(){ vec4 w = modelMatrix * vec4(position, 1.0); vy = w.y; gl_Position = projectionMatrix * viewMatrix * w; }",
    fragmentShader: `varying float vy; void main(){ gl_FragColor = vec4(clamp((vy - (${SEABED_LOW.toFixed(1)})) / ${SEABED_SPAN.toFixed(1)}, 0.0, 1.0), 0.0, 0.0, 1.0); }`,
  });
}
