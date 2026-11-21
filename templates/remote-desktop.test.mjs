import fs from "node:fs";
import assert from "node:assert/strict";
import ts from "typescript";
const raw = fs
  .readFileSync(new URL("./src/_lib/remote.ts", import.meta.url), "utf8")
  .replace(/import\s*\{\s*myco\s*\}\s*from\s*["']\.\/myco["'];?/, "const myco=undefined;");
const { outputText } = ts.transpileModule(raw, {
  compilerOptions: {
    module: ts.ModuleKind.ESNext,
    target: ts.ScriptTarget.ES2022,
  },
});
const { pointInVideo, DesktopSession } = await import(
  "data:text/javascript;base64," + Buffer.from(outputText).toString("base64")
);
assert.deepEqual(
  pointInVideo(
    600,
    400,
    { left: 100, top: 150, width: 1000, height: 500 },
    1920,
    1080,
  ),
  { x: 0.5, y: 0.5 },
);
assert.equal(
  pointInVideo(
    100,
    400,
    { left: 100, top: 150, width: 1000, height: 500 },
    1920,
    1080,
  ),
  null,
);
assert.equal(
  pointInVideo(1, 1, { left: 0, top: 0, width: 0, height: 0 }, 0, 0),
  null,
);
// Closing the viewer while the Hub creates a session must retract the late result.
globalThis.RTCRtpReceiver = {
  getCapabilities: () => ({ codecs: [{ mimeType: "video/H264" }] }),
};
globalThis.RTCPeerConnection = class {
  iceGatheringState = "complete";
  localDescription = { toJSON: () => ({ type: "offer", sdp: "v=0" }) };
  addTransceiver() {}
  async createOffer() {
    return { type: "offer", sdp: "v=0" };
  }
  async setLocalDescription() {}
  addEventListener() {}
  removeEventListener() {}
  close() {}
};
let resume, started;
const pending = new Promise((r) => {
    resume = r;
  }),
  begin = new Promise((r) => {
    started = r;
  });
const calls = [];
const viewer = new DesktopSession(async (action, args) => {
  calls.push([action, args]);
  if (action === "connect") {
    started();
    return pending;
  }
  return true;
});
const connection = viewer.connect(
  { machine: "mac", control: false },
  "abcdef12",
  false,
  () => {},
  () => {},
  () => {},
);
await begin;
await viewer.close();
resume({ id: "late", state: "pending" });
await connection;
assert(
  calls.some(([action, args]) => action === "disconnect" && args.id === "late"),
);
// A congested input channel closes cleanly instead of recursively sending release.
const congested = new DesktopSession(async () => true);
let closed = 0;
congested.channel = {
  readyState: "open",
  bufferedAmount: 70000,
  close() {
    closed++;
  },
  send() {
    throw Error("must not enqueue into a blocked channel");
  },
};
congested.input({ kind: "key", code: "KeyA", down: false });
await new Promise((r) => setTimeout(r, 0));
assert.equal(closed, 1);
console.log(
  "Remote Desktop coordinate, late-session and input-backpressure checks passed",
);
