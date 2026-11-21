import { myco } from "./myco";
export type RemoteDevice = {
  machine: string;
  name: string;
  os: string;
  control: boolean;
};
export type RemoteStatus = {
  protocol: number;
  version: string;
  installed: boolean;
  supported: boolean;
  running: boolean;
  pairing_code?: string;
  reason?: string;
};
export type RemoteSignal = {
  id: string;
  machine: string;
  state: "pending" | "connected" | "failed" | "closed";
  answer?: RTCSessionDescriptionInit;
  error?: string;
};
export type RemoteStats = {
  fps: number;
  width: number;
  height: number;
  mbps: number;
  latency: number;
};
export const remoteAvailable = () =>
  !!myco?.remote && !!myco.features?.includes("remote");
export async function remoteCall<T>(
  action: string,
  args: unknown = {},
): Promise<T> {
  if (!remoteAvailable()) throw new Error("remote_unavailable");
  return myco!.remote!.call<T>(action, args);
}
export function pointInVideo(
  x: number,
  y: number,
  box: { left: number; top: number; width: number; height: number },
  width: number,
  height: number,
) {
  if (!width || !height || !box.width || !box.height) return null;
  const scale = Math.min(box.width / width, box.height / height),
    w = width * scale,
    h = height * scale;
  const px = (x - box.left - (box.width - w) / 2) / w,
    py = (y - box.top - (box.height - h) / 2) / h;
  return px >= 0 && px <= 1 && py >= 0 && py <= 1 ? { x: px, y: py } : null;
}
export class DesktopSession {
  private pc: RTCPeerConnection | null = null;
  private channel: RTCDataChannel | null = null;
  private timer: ReturnType<typeof setInterval> | undefined;
  private signal = "";
  private ended = false;
  private stateCallback: ((s: string) => void) | null = null;
  private lastBytes = 0;
  private lastAt = 0;
  private polling = false;
  supportsText = false;
  private stopListening?: () => void;
  constructor(private call: typeof remoteCall = remoteCall) {
    this.stopListening = myco?.remote?.onDisconnect?.(() => {
      if (!this.ended) { this.stateCallback?.("connection_lost"); void this.close(); }
    });
  }
  async connect(
    device: RemoteDevice,
    pairingCode: string,
    control: boolean,
    onMedia: (stream: MediaStream) => void,
    onState: (state: string) => void,
    onStats: (s: RemoteStats) => void,
  ) {
    if (typeof RTCPeerConnection === "undefined")
      throw new Error("video_unsupported");
    const caps = RTCRtpReceiver.getCapabilities?.("video");
    if (
      caps &&
      !caps.codecs.some((c) => c.mimeType.toLowerCase() === "video/h264")
    )
      throw new Error("video_unsupported");
    this.stateCallback = onState;
    const pc = (this.pc = new RTCPeerConnection({ iceServers: [] }));
    pc.addTransceiver("video", { direction: "recvonly" });
    if (control)
      this.channel = pc.createDataChannel("input", { ordered: true });
    if (this.channel) this.channel.onmessage = (event) => {
      if (typeof event.data !== "string" || event.data.length > 2048) return;
      try { const value = JSON.parse(event.data); if(value.type === "capabilities" && Array.isArray(value.input_features)) this.supportsText = value.input_features.includes("text"); } catch {}
    };
    if (this.channel)
      this.channel.onclose = () => {
        if (!this.ended) {
          onState("connection_lost");
          void this.close();
        }
      };
    pc.ontrack = (e) => onMedia(e.streams[0] || new MediaStream([e.track]));
    pc.onconnectionstatechange = () => {
      if (!this.ended) {
        onState(pc.connectionState);
        if (["failed", "closed", "disconnected"].includes(pc.connectionState))
          void this.close();
      }
    };
    const offer = await pc.createOffer();
    await pc.setLocalDescription(offer);
    await new Promise<void>((resolve, reject) => {
      const timeout = setTimeout(() => {
        pc.removeEventListener("icegatheringstatechange", check);
        reject(new Error("connection_timeout"));
      }, 12000);
      const check = () => {
        if (pc.iceGatheringState === "complete") {
          clearTimeout(timeout);
          pc.removeEventListener("icegatheringstatechange", check);
          resolve();
        }
      };
      pc.addEventListener("icegatheringstatechange", check);
      check();
    });
    if (this.ended) throw new Error("cancelled");
    const created = await this.call<RemoteSignal>("connect", {
      machine: device.machine,
      offer: {
        description: pc.localDescription?.toJSON(),
        pairing_code: pairingCode.trim().toLowerCase(),
        control,
      },
    });
    this.signal = created.id;
    if (this.ended) {
      await this.call("disconnect", { id: created.id });
      return;
    }
    const until = Date.now() + 30000;
    while (!this.ended) {
      const s = await this.call<RemoteSignal>("session", { id: this.signal });
      if (s.state === "failed" || s.state === "closed")
        throw new Error(s.error || "connection_closed");
      if (s.answer) {
        await pc.setRemoteDescription(s.answer);
        break;
      }
      if (Date.now() > until) throw new Error("connection_timeout");
      await new Promise((r) => setTimeout(r, 600));
    }
    if (this.ended) return;
    this.timer = setInterval(() => void this.poll(onState, onStats), 2500);
  }
  private async poll(
    onState: (s: string) => void,
    onStats: (s: RemoteStats) => void,
  ) {
    if (this.ended || this.polling || !this.pc) return;
    this.polling = true;
    try {
      const state = await this.call<RemoteSignal>("session", {
        id: this.signal,
      });
      if (state.state === "closed" || state.state === "failed") {
        onState(state.error || "closed");
        await this.close();
        return;
      }
      const reports = await this.pc.getStats();
      const out: RemoteStats = {
        fps: 0,
        width: 0,
        height: 0,
        mbps: 0,
        latency: 0,
      };
      reports.forEach((r: any) => {
        if (
          r.type === "inbound-rtp" &&
          (r.kind === "video" || r.mediaType === "video")
        ) {
          out.fps = r.framesPerSecond || 0;
          out.width = r.frameWidth || 0;
          out.height = r.frameHeight || 0;
          if (this.lastAt)
            out.mbps = Math.max(
              0,
              ((r.bytesReceived - this.lastBytes) * 8) /
                (r.timestamp - this.lastAt) /
                1000,
            );
          this.lastBytes = r.bytesReceived;
          this.lastAt = r.timestamp;
        }
        if (
          r.type === "candidate-pair" &&
          r.state === "succeeded" &&
          r.nominated
        )
          out.latency = Math.round((r.currentRoundTripTime || 0) * 1000);
      });
      onStats(out);
    } catch {
      onState("connection_lost");
      await this.close();
    } finally {
      this.polling = false;
    }
  }
  input(event: Record<string, unknown>) {
    if (this.channel?.readyState !== "open") return false;
    if (this.channel.bufferedAmount > 65536) {
      this.stateCallback?.("connection_lost");
      void this.close();
      return;
    }
    if (this.channel.bufferedAmount > 16384 && event.kind === "move") return;
    try {
      this.channel.send(JSON.stringify(event));
      return true;
    } catch {
      this.stateCallback?.("connection_lost");
      void this.close();
    }
  }
  async close() {
    if (this.ended) return;
    this.ended = true;
    this.stopListening?.(); this.stopListening = undefined;
    if (
      this.channel?.readyState === "open" &&
      this.channel.bufferedAmount < 65536
    ) {
      try {
        this.channel.send(JSON.stringify({ kind: "release" }));
      } catch {
        /* the host releases inputs when this channel closes */
      }
    }
    clearInterval(this.timer);
    this.channel?.close();
    this.pc?.close();
    this.pc = null;
    if (this.signal)
      await this.call("disconnect", { id: this.signal }).catch(() => {});
  }
}
