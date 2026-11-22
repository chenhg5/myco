// Schedule the samples emitted by each emulated frame on the Web Audio clock.
// Keep a short bounded queue and discard pending sounds on pause or reload.
export const nesSampleRate = 48000;
export class NESAudio {
  private readonly left = new Float32Array(1200);
  private readonly right = new Float32Array(1200);
  private readonly sources = new Set<AudioBufferSourceNode>();
  private length = 0;
  private endsAt = 0;
  readonly context = new AudioContext({ sampleRate: nesSampleRate });
  sample(left: number, right: number) {
    if (this.length >= this.left.length) return;
    this.left[this.length] = left;
    this.right[this.length++] = right;
  }
  flush() {
    const count = this.length;
    this.length = 0;
    if (!count || this.context.state !== "running") return;
    const now = this.context.currentTime;
    if (this.endsAt > now + 0.085) return;
    const audio = this.context.createBuffer(2, count, nesSampleRate);
    audio.copyToChannel(this.left.subarray(0, count), 0);
    audio.copyToChannel(this.right.subarray(0, count), 1);
    const source = this.context.createBufferSource();
    source.buffer = audio;
    source.connect(this.context.destination);
    source.onended = () => { this.sources.delete(source); source.disconnect(); };
    const start = this.endsAt < now ? now + 0.022 : this.endsAt;
    source.start(start);
    this.endsAt = start + audio.duration;
    this.sources.add(source);
  }
  clear() {
    for (const source of this.sources) source.stop();
    this.sources.clear();
    this.length = 0;
    this.endsAt = 0;
  }
  close() { this.clear(); void this.context.close().catch(() => undefined); }
}
