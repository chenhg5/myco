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

// A transient Hub error preserves a connected video session; repeated failures
// and explicit revocation still terminate it.
let failures=0;
const resilient=new DesktopSession(async()=>{if(failures++===0)throw Error("unavailable");return {state:"connected"};});
resilient.pc={getStats:async()=>new Map(),close(){}};
await resilient.poll(()=>{},()=>{});assert.equal(resilient.ended,false);
await resilient.poll(()=>{},()=>{});assert.equal(resilient.failedPolls,0);
resilient.call=async()=>{throw Error("unavailable")};
await resilient.poll(()=>{},()=>{});await resilient.poll(()=>{},()=>{});assert.equal(resilient.ended,false);
await resilient.poll(()=>{},()=>{});assert.equal(resilient.ended,true);
const revoked=new DesktopSession(async()=>{throw Error("permission_denied")});revoked.pc={close(){}};
await revoked.poll(()=>{},()=>{});assert.equal(revoked.ended,true);
console.log("PASS transient signaling recovery, retry exhaustion and immediate revocation");

RTCPeerConnection.prototype.setRemoteDescription=async function(){};
const route=new DesktopSession(async(action)=>action==="connect"?{id:"route"}:{state:"connected",answer:{type:"answer",sdp:"v=0"}});
await route.connect({machine:"mac",control:false},"abcdef12",false,()=>{},()=>{},()=>{});
const savedTimeout=globalThis.setTimeout;let expire;
globalThis.setTimeout=(fn,ms,...args)=>ms===10000?(expire=fn,999999):savedTimeout(fn,ms,...args);
try {
 route.pc.connectionState="disconnected";route.pc.onconnectionstatechange();assert.equal(route.ended,false);
 route.pc.connectionState="connected";route.pc.onconnectionstatechange();assert.equal(route.disconnectTimer,undefined);
 route.pc.connectionState="disconnected";route.pc.onconnectionstatechange();expire();assert.equal(route.ended,true);
} finally {globalThis.setTimeout=savedTimeout;await route.close();}
console.log("PASS ICE disconnect recovery and grace expiry");
