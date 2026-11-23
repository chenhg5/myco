// Rain: streaks falling around the eye (slanted by the wind), and the
// moment of a lightning flash in a storm. The sea's shader adds the
// raindrops' ripples; the sky darkens on its own.

import * as THREE from "three";

const COUNT = 5000, BOX = 50, HIGH = 30;

export class Rain {
  readonly mesh: THREE.LineSegments;
  private pos = new Float32Array(COUNT * 6);
  private seeds = new Float32Array(COUNT * 3);
  private mat: THREE.ShaderMaterial;
  private fall = 0;

  constructor() {
    for (let i = 0; i < COUNT; i++) this.seeds.set([Math.random(), Math.random(), Math.random()], i * 3);
    const g = new THREE.BufferGeometry();
    g.setAttribute("position", new THREE.BufferAttribute(this.pos, 3).setUsage(THREE.DynamicDrawUsage));
    this.mat = new THREE.ShaderMaterial({
      uniforms: { uAmount: { value: 0 }, uLight: { value: new THREE.Color(1, 1, 1) } },
      vertexShader: "varying float vEnd; void main(){ vEnd = mod(float(gl_VertexID), 2.0); gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0); }",
      fragmentShader: "uniform float uAmount; uniform vec3 uLight; varying float vEnd; void main(){ gl_FragColor = vec4(uLight, uAmount * 0.32 * (0.35 + 0.65 * vEnd)); \n#include <colorspace_fragment>\n }",
      transparent: true, depthWrite: false,
    });
    this.mesh = new THREE.LineSegments(g, this.mat);
    this.mesh.frustumCulled = false;
    this.mesh.visible = false;
  }

  /** Moves the drops on (around `eye`), with the wind's push (m/s, x and z). */
  update(eye: THREE.Vector3, dt: number, amount: number, wind: THREE.Vector2, light: THREE.Color) {
    this.mesh.visible = amount > 0.01;
    if (!this.mesh.visible) return;
    this.fall += dt * 11;
    const n = Math.round(COUNT * Math.min(1, amount)), p = this.pos, s = this.seeds;
    const wx = wind.x * 3 * amount, wz = wind.y * 3 * amount, len = 0.75;
    for (let i = 0; i < n; i++) {
      // each drop's place in a box that follows the eye (it wraps round)
      const h = HIGH - ((s[i * 3 + 2] * HIGH + this.fall) % HIGH);
      const up = h - HIGH * 0.35;
      const x = eye.x + ((((s[i * 3] * BOX + wx * this.fall / 11 - eye.x) % BOX) + BOX) % BOX) - BOX / 2;
      const z = eye.z + ((((s[i * 3 + 1] * BOX + wz * this.fall / 11 - eye.z) % BOX) + BOX) % BOX) - BOX / 2;
      const y = eye.y + up;
      p.set([x, y, z, x - wx * 0.07, y - len, z - wz * 0.07], i * 6);
    }
    this.mesh.geometry.setDrawRange(0, n * 2);
    (this.mesh.geometry.attributes.position as THREE.BufferAttribute).needsUpdate = true;
    this.mat.uniforms.uAmount.value = Math.min(1, amount);
    this.mat.uniforms.uLight.value.copy(light);
  }
}

/** Lightning: a flash now and then (strength 0–1 now), and when one struck. */
export class Lightning {
  private next = 4;
  private at = -100;
  private double = false;
  onStrike: (distance: number) => void = () => {};
  update(t: number, storm: boolean) {
    if (!storm) { this.next = t + 3; return 0; }
    if (t > this.next) {
      this.at = t;
      this.double = Math.random() < 0.5;
      this.next = t + 5 + Math.random() * 12;
      this.onStrike(300 + Math.random() * 2500);
    }
    const a = t - this.at;
    if (a < 0 || a > 0.6) return 0;
    const f = Math.exp(-a * 14) + (this.double ? 0.7 * Math.exp(-Math.max(0, a - 0.18) * 16) * (a > 0.18 ? 1 : 0) : 0);
    return Math.min(1, f);
  }
}
