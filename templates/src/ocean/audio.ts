// Sound, made here (no files): the sea (filtered noise that swells and
// falls like waves), spray that grows with speed, and a diesel engine
// whose pitch and chug follow the throttle. Starts on the first key or
// click (browsers allow sound only after one).

import { clamp } from "./noise";

export class Sound {
  private ctx: AudioContext | null = null;
  private master: GainNode | null = null;
  private sea: GainNode | null = null;
  private spray: GainNode | null = null;
  private engine: { gain: GainNode; osc: OscillatorNode; sub: OscillatorNode; filter: BiquadFilterNode; chug: OscillatorNode } | null = null;
  private swellAt = 0;
  private rain: GainNode | null = null;
  private noiseBuf: AudioBuffer | null = null;
  on = true;

  /** Creates the sounds (call from a key or click). */
  start() {
    if (this.ctx) { void this.ctx.resume(); return; }
    const AC = window.AudioContext || (window as unknown as { webkitAudioContext?: typeof AudioContext }).webkitAudioContext;
    if (!AC) return;
    const ctx = new AC();
    this.ctx = ctx;
    const master = ctx.createGain();
    master.gain.value = this.on ? 0.55 : 0;
    master.connect(ctx.destination);
    this.master = master;
    // brown-ish noise, looped
    const len = ctx.sampleRate * 4, buf = ctx.createBuffer(1, len, ctx.sampleRate), d = buf.getChannelData(0);
    let last = 0;
    for (let i = 0; i < len; i++) { last = (last + 0.035 * (Math.random() * 2 - 1)) / 1.035; d[i] = last * 3.2; }
    const noise = () => { const s = ctx.createBufferSource(); s.buffer = buf; s.loop = true; s.loopStart = Math.random() * 3; s.start(0, Math.random() * 3); return s; };
    // the sea: low rumble, swelling
    const seaF = ctx.createBiquadFilter(); seaF.type = "lowpass"; seaF.frequency.value = 520;
    const sea = ctx.createGain(); sea.gain.value = 0.5;
    noise().connect(seaF).connect(sea).connect(master);
    this.sea = sea;
    // spray: a hiss that comes with speed
    const sprF = ctx.createBiquadFilter(); sprF.type = "bandpass"; sprF.frequency.value = 1800; sprF.Q.value = 0.5;
    const spray = ctx.createGain(); spray.gain.value = 0;
    const white = ctx.createBuffer(1, len, ctx.sampleRate), w = white.getChannelData(0);
    for (let i = 0; i < len; i++) w[i] = Math.random() * 2 - 1;
    const ws = ctx.createBufferSource(); ws.buffer = white; ws.loop = true; ws.start();
    ws.connect(sprF).connect(spray).connect(master);
    this.spray = spray;
    // rain: a soft hiss on the sea
    const rf = ctx.createBiquadFilter(); rf.type = "highpass"; rf.frequency.value = 900;
    const rain = ctx.createGain(); rain.gain.value = 0;
    const rs = ctx.createBufferSource(); rs.buffer = white; rs.loop = true; rs.start(0, 1.3);
    rs.connect(rf).connect(rain).connect(master);
    this.rain = rain;
    this.noiseBuf = buf;
    // the engine: a sawtooth and a sub, through a low filter, chugging
    const osc = ctx.createOscillator(); osc.type = "sawtooth"; osc.frequency.value = 42;
    const sub = ctx.createOscillator(); sub.type = "square"; sub.frequency.value = 21;
    const filter = ctx.createBiquadFilter(); filter.type = "lowpass"; filter.frequency.value = 260; filter.Q.value = 2;
    const gain = ctx.createGain(); gain.gain.value = 0.05;
    const subG = ctx.createGain(); subG.gain.value = 0.35;
    const chug = ctx.createOscillator(); chug.frequency.value = 10;
    const chugG = ctx.createGain(); chugG.gain.value = 0.025;
    chug.connect(chugG).connect(gain.gain);
    osc.connect(filter); sub.connect(subG).connect(filter);
    filter.connect(gain).connect(master);
    osc.start(); sub.start(); chug.start();
    this.engine = { gain, osc, sub, filter, chug };
  }

  setOn(on: boolean) {
    this.on = on;
    if (this.ctx && this.master) this.master.gain.setTargetAtTime(on ? 0.55 : 0, this.ctx.currentTime, 0.1);
  }

  /** Follows the boat (a few times a second). */
  update(speed: number, throttle: number, sea: number, rain = 0) {
    const ctx = this.ctx;
    if (!ctx || !this.engine || !this.sea || !this.spray) return;
    const now = ctx.currentTime, th = Math.abs(throttle), v = clamp(Math.abs(speed) / 13, 0, 1);
    const f = 38 + th * 46 + v * 10;
    this.engine.osc.frequency.setTargetAtTime(f, now, 0.35);
    this.engine.sub.frequency.setTargetAtTime(f / 2, now, 0.35);
    this.engine.chug.frequency.setTargetAtTime(f / 4, now, 0.35);
    this.engine.filter.frequency.setTargetAtTime(220 + th * 520, now, 0.3);
    this.engine.gain.gain.setTargetAtTime(0.04 + th * 0.09, now, 0.3);
    this.spray.gain.setTargetAtTime(0.012 + v * 0.07 + sea * 0.015, now, 0.4);
    this.rain?.gain.setTargetAtTime(rain * 0.09, now, 1.2);
    // a wave swells every few seconds
    if (now > this.swellAt) {
      const dur = 2.5 + Math.random() * 4;
      this.sea.gain.setTargetAtTime((0.25 + Math.random() * 0.45) * (0.6 + sea * 0.6), now, dur / 3);
      this.swellAt = now + dur;
    }
  }

  /** Thunder from a strike `distance` metres away (sound at 340 m/s). */
  thunder(distance: number) {
    const ctx = this.ctx, master = this.master, buf = this.noiseBuf;
    if (!ctx || !master || !buf) return;
    const at = ctx.currentTime + distance / 340, near = Math.max(0.15, 1 - distance / 3000);
    const src = ctx.createBufferSource(); src.buffer = buf; src.playbackRate.value = 0.5;
    const f = ctx.createBiquadFilter(); f.type = "lowpass"; f.frequency.value = 160 + near * 500;
    const g = ctx.createGain(); g.gain.value = 0;
    g.gain.setValueAtTime(0, at);
    g.gain.linearRampToValueAtTime(0.9 * near, at + 0.08 + (1 - near) * 0.4);
    g.gain.exponentialRampToValueAtTime(0.001, at + 3.5 + Math.random() * 2);
    src.connect(f).connect(g).connect(master);
    src.start(at, Math.random() * 2);
    src.stop(at + 6.5);
  }

  stop() { void this.ctx?.close(); this.ctx = null; this.engine = null; }
}
