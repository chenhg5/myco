/** HTTP adapter for the framework action API. The caller supplies its existing
 * authenticated request function; no credential storage, login or App-token
 * fallback is introduced by this SDK. */
import { ActionError, type ActionTarget, type ActionDescriptor, type ActionGrant, type ActionInvocation, type ActionJob, type ActionOptions, type ActionTransport, type GrantRequest } from "./actions";

export type ActionRequest = (path: string, init: RequestInit) => Promise<Response>;
const grants = "/v1/me/action-grants";
const jobs = "/v1/me/action-jobs";
const idPath = (base: string, id: string) => `${base}/${encodeURIComponent(id)}`;
function waitQuery(options: ActionOptions | undefined, fallback = "") {
  const n = options?.wait_seconds;
  if (n === undefined) return fallback;
  if (!Number.isInteger(n) || n < 0 || n > 60) throw new ActionError({ code: "invalid_argument", message: "Wait must be an integer between 0 and 60 seconds.", retryable: false });
  return `?wait_seconds=${n}`;
}

export class ActionHTTPTransport implements ActionTransport {
  constructor(private readonly request: ActionRequest) {}
  private async response(path: string, method: string, body: unknown, options?: ActionOptions): Promise<Response> {
    let payload: string | undefined;
    try { payload = body === undefined ? undefined : JSON.stringify(body); }
    catch { throw new ActionError({ code: "invalid_argument", message: "Action arguments must be JSON serializable.", retryable: false }); }
    let response: Response;
    try {
      response = await this.request(path, {
        method, signal: options?.signal,
        ...(payload === undefined ? {} : { headers: { "Content-Type": "application/json" }, body: payload }),
      });
    } catch (e) {
      // A lost POST response may follow a successful OS operation. Never retry
      // a mutation here; callers can recover with its original idempotency key.
      if (options?.signal?.aborted) throw new ActionError({ code: "cancelled", message: "The request was interrupted; its outcome may be unknown.", retryable: false });
      throw new ActionError({ code: method === "GET" ? "unavailable" : "outcome_unknown", message: e instanceof Error ? e.message : "The action service is unavailable.", retryable: method === "GET" });
    }
    if (response.ok) return response;
    const raw: unknown = await response.json().catch(() => null);
    const error = raw && typeof raw === "object" ? raw as Record<string, unknown> : {};
    throw new ActionError({
      code: (typeof error.error === "string" ? error.error : undefined) || ({ 401: "permission_denied", 403: "permission_denied", 404: "not_found", 409: "conflict", 410: "expired" } as Record<number, string>)[response.status] || "failed",
      message: typeof error.message === "string" ? error.message : `Action request failed (${response.status}).`,
      retryable: error.retryable === true, applied: typeof error.applied === "number" ? error.applied : undefined,
    }, response.status);
  }
  private async json<T>(path: string, method: string, body?: unknown, options?: ActionOptions): Promise<T> {
    const response = await this.response(path, method, body, options);
    try { return await response.json() as T; }
    catch { throw new ActionError({ code: "invalid_response", message: "The action service returned invalid JSON.", retryable: false }); }
  }
  async listTargets(options?: ActionOptions): Promise<ActionTarget[]> {
    try {
      const result = await this.json<{ targets: ActionTarget[] }>(
        "/v1/me/action-targets",
        "GET",
        undefined,
        options,
      );
      if (
        !Array.isArray(result.targets) ||
        result.targets.some(
          (t) =>
            !t ||
            !["machine", "page"].includes(t.kind) ||
            typeof t.id !== "string" ||
            !t.id ||
            (t.resource &&
              [t.resource.provider, t.resource.id, t.resource.binding].some(
                (v) => typeof v !== "string" || !v,
              )),
        )
      )
        throw new ActionError({
          code: "invalid_response",
          message: "The service returned invalid action targets.",
          retryable: false,
        });
      return result.targets;
    } catch (e) {
      if (e instanceof ActionError && e.status === 404)
        throw new ActionError(
          {
            code: "unsupported",
            message: "This Hub does not support resource discovery.",
            retryable: false,
          },
          404,
        );
      throw e;
    }
  }
  requestGrant(request: GrantRequest, options?: ActionOptions) { return this.json<ActionGrant>(grants, "POST", request, options); }
  async listGrants(options?: ActionOptions): Promise<ActionGrant[]> {
    const result = await this.json<{ grants: ActionGrant[] }>(grants, "GET", undefined, options);
    if (!Array.isArray(result.grants)) throw new ActionError({ code: "invalid_response", message: "The service returned no grants.", retryable: false });
    return result.grants;
  }
  getGrant(id: string, options?: ActionOptions) { return this.json<ActionGrant>(idPath(grants, id) + waitQuery(options), "GET", undefined, options); }
  endGrant(id: string, options?: ActionOptions) { return this.json<ActionGrant>(`${idPath(grants, id)}/end`, "POST", {}, options); }
  async listActions(grant: string, options?: ActionOptions): Promise<ActionDescriptor[]> {
    const result = await this.json<{ actions: ActionDescriptor[] }>(`${idPath(grants, grant)}/actions`, "GET", undefined, options);
    if (!Array.isArray(result.actions)) throw new ActionError({ code: "invalid_response", message: "The service returned no action descriptors.", retryable: false });
    return result.actions;
  }
  invoke(request: ActionInvocation, options?: ActionOptions) { return this.json<ActionJob>(jobs, "POST", request, options); }
  getJob(id: string, options?: ActionOptions) { return this.json<ActionJob>(idPath(jobs, id) + waitQuery(options, "?wait=30s"), "GET", undefined, options); }
  cancelJob(id: string, options?: ActionOptions) { return this.json<ActionJob>(`${idPath(jobs, id)}/cancel`, "POST", {}, options); }
  async readArtifact(_grant: string, id: string, options?: ActionOptions): Promise<Uint8Array> {
    // The artifact endpoint binds its ID to the authenticated agent's grant.
    const response = await this.response(idPath("/v1/me/action-artifacts", id), "GET", undefined, options);
    const limit = 4 * 1024 * 1024;
    const tooLarge = () => new ActionError({ code: "too_large", message: "Action artifacts are limited to 4 MB.", retryable: false });
    if (Number(response.headers.get("Content-Length")) > limit) { await response.body?.cancel(); throw tooLarge(); }
    const reader = response.body?.getReader();
    if (!reader) return new Uint8Array();
    const chunks: Uint8Array[] = [];
    let size = 0;
    try {
      for (;;) {
        const chunk = await reader.read();
        if (chunk.done) break;
        size += chunk.value.byteLength;
        if (size > limit) { await reader.cancel(); throw tooLarge(); }
        chunks.push(chunk.value);
      }
    } finally { reader.releaseLock(); }
    const bytes = new Uint8Array(size);
    let offset = 0;
    for (const chunk of chunks) { bytes.set(chunk, offset); offset += chunk.byteLength; }
    return bytes;
  }
}
