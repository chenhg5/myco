// Provider credentials are supplied by the running session, never the graph.
import { network } from "../_lib/net";
import { cutOutput, mediaAddress, type CanvasNode } from "./model";

export type Keys = { text: string; media: string };
const replicate = "https://api.replicate.com/v1";
const openRouter = "https://openrouter.ai/api/v1/chat/completions";

async function request(url: string, key: string, method: "GET" | "POST", data?: unknown, headers?: Record<string, string>) {
  const result = await network().fetch({
    url, headers: { Authorization: "Bearer " + key, "Content-Type": "application/json", ...headers },
    method, body: data === undefined ? undefined : JSON.stringify(data),
    redirect: false, response: "text", timeout_ms: 20000, max_bytes: 1024 * 1024,
  });
  if (result.truncated) throw new Error("provider_error");
  if (result.status === 401 || result.status === 403) throw new Error("provider_key");
  if (result.status < 200 || result.status >= 300) throw new Error("provider_error");
  try { return result.body ? JSON.parse(result.body) : null; }
  catch { throw new Error("provider_error"); }
}

function inputJSON(raw: string, upstream: string) {
  let input: unknown;
  try { input = raw.trim() ? JSON.parse(raw) : {}; }
  catch { throw new Error("invalid_options"); }
  if (!input || typeof input !== "object" || Array.isArray(input)) throw new Error("invalid_options");
  function replace(value: unknown): unknown {
    if (value === "{{upstream}}") {
      if (!upstream) throw new Error("missing_input");
      return upstream;
    }
    if (Array.isArray(value)) return value.map(replace);
    if (value && typeof value === "object") return Object.fromEntries(Object.entries(value).map(([key, value]) => [key, replace(value)]));
    return value;
  }
  return replace(input) as Record<string, unknown>;
}

function pause(signal: AbortSignal) {
  return new Promise<void>((resolve, reject) => {
    const stop = () => { clearTimeout(timer); reject(signal.reason); };
    const timer = setTimeout(() => { signal.removeEventListener("abort", stop); resolve(); }, 2000);
    signal.addEventListener("abort", stop, { once: true });
    if (signal.aborted) stop();
  });
}

export function checkInputs(node: CanvasNode, upstream: string, keys: Keys) {
  if (node.kind === "note") return;
  if (!node.prompt.trim()) throw new Error("missing_prompt");
  const model = node.model.trim();
  if (!model || (node.kind !== "text" && !/^[a-z\d_.-]+\/[a-z\d_.-]+(?::[a-f\d]{64})?$/i.test(model)))
    throw new Error("missing_model");
  if (node.kind !== "text") inputJSON(node.options, upstream);
  if (!(node.kind === "text" ? keys.text : keys.media).trim()) throw new Error("missing_key");
}

export async function generate(node: CanvasNode, upstream: string, keys: Keys, signal: AbortSignal) {
  signal.throwIfAborted();
  if (node.kind === "note") return node.prompt;
  checkInputs(node, upstream, keys);
  const model = node.model.trim();
  if (node.kind === "text") {
    const content = node.prompt + (upstream ? "\n\n" + upstream : "");
    const response = await request(openRouter, keys.text.trim(), "POST", {
      model, messages: [{ role: "user", content }], max_tokens: 1500,
    });
    signal.throwIfAborted();
    const output = response?.choices?.[0]?.message?.content;
    if (typeof output !== "string" || !output.trim()) throw new Error("empty_output");
    return cutOutput(output);
  }
  const token = keys.media.trim();
  const options = inputJSON(node.options, upstream);
  const useRawInput = node.options.includes("{{upstream}}");
  const input = { ...options, prompt: node.prompt + (upstream && !useRawInput ? "\n\n" + upstream : "") };
  const deadline = Date.now() + 300000;
  const version = model.split(":")[1];
  const endpoint = version ? replicate + "/predictions" : replicate + "/models/" + model + "/predictions";
  let prediction = await request(endpoint, token, "POST", version ? { version, input } : { input }, { "Cancel-After": "5m" });
  const id = prediction?.id;
  if (typeof id !== "string" || !/^[a-z0-9_-]+$/i.test(id)) throw new Error("provider_error");
  const poll = replicate + "/predictions/" + id;
  try {
    while (prediction?.status === "starting" || prediction?.status === "processing") {
      signal.throwIfAborted();
      if (Date.now() >= deadline) throw new Error("provider_timeout");
      await pause(signal);
      prediction = await request(poll, token, "GET");
    }
    signal.throwIfAborted();
    if (prediction?.status !== "succeeded") throw new Error("provider_error");
    const outputs = Array.isArray(prediction.output) ? prediction.output : [prediction.output];
    const url = outputs.map((value: unknown) => typeof value === "string" ? mediaAddress(value) : "").find(Boolean);
    if (!url || url.length > 4000) throw new Error("empty_output");
    return url;
  } catch (error) {
    if (signal.aborted || (error as Error).message === "provider_timeout") {
      void request(poll + "/cancel", token, "POST").catch(() => undefined);
    }
    throw error;
  }
}
