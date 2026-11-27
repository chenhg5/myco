import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { transform } from "esbuild";

const source = await readFile(new URL("../src/lib/latency.ts", import.meta.url), "utf8");
const { code } = await transform(source, { loader: "ts", format: "esm" });
const { latencyMark, latencyPending, latencySend } = await import(`data:text/javascript;base64,${Buffer.from(code).toString("base64")}`);
const marks = (id) => performance.getEntriesByName("myco.latency").filter((e) => e.detail.message === id);
performance.clearMarks("myco.latency");

latencyMark("demo", "historical", "app_trace_received");
assert.equal(marks("historical").length, 0, "opening history must not publish a fresh-send sample");

const result = { id: "fresh", body: "private-fixture-content" };
assert.equal(await latencySend("demo", performance.now(), Promise.resolve(result)), result);
for (const stage of ["app_trace_received", "app_text_dom", "app_paint_opportunity"]) latencyMark("demo", "fresh", stage);
latencyMark("demo", "fresh", "app_trace_received");
assert.equal(marks("fresh").length, 5);
assert.equal(latencyPending("demo", "fresh"), false);
assert.ok(!JSON.stringify(marks("fresh").map((m) => m.detail)).includes("private-fixture-content"));
assert.ok(marks("fresh").every((m) => Object.keys(m.detail).sort().join(",") === "hub,message,observed_at_ms,stage"));

// Polling can reveal text while the send response is still held in flight.
const started = performance.now() - 10;
latencyMark("demo", "late-ack", "app_trace_received", started + 1);
latencyMark("demo", "late-ack", "app_text_dom", started + 2);
latencyMark("demo", "late-ack", "app_paint_opportunity", started + 3);
assert.equal(marks("late-ack").length, 0);
await latencySend("demo", started, Promise.resolve({ id: "late-ack" }));
assert.deepEqual(marks("late-ack").map((m) => m.detail.stage), ["app_send", "app_trace_received", "app_text_dom", "app_paint_opportunity", "app_send_ack"]);
assert.equal(marks("late-ack")[3].startTime, started + 3, "a delayed ACK must not move observed first text later");

const failure = new Error("fixture-rejection");
await assert.rejects(latencySend("demo", performance.now(), Promise.reject(failure)), (e) => e === failure);
for (let i = 0; i < 1000; i++) await latencySend("demo", performance.now(), Promise.resolve({ id: `bound-${i}` }));
assert.ok(performance.getEntriesByName("myco.latency").length <= 320);
console.log("latency: content-free, bounded, deduplicated, historical/late-ACK/rejection semantics passed");
