/** Agent actions, independent of providers, browser bridges and HTTP routing.
 * The trusted transport enforces authorization. Client checks are usability
 * guards; constructing these types never grants access to a computer or page. */
export type ActionTarget = { kind: "machine" | "page"; id: string };
export type ActionDescriptor = {
  name: string;
  version: number;
  title: string;
  permission: string;
  input_schema: object;
  output_schema: object;
};
export type GrantRequest = {
  target: ActionTarget;
  permissions: string[];
  reason: string;
  message_id?: string;
  page_id?: string;
  ttl_seconds?: number;
};
export type ActionGrant = {
  id: string;
  agent: string;
  owner: string;
  target: ActionTarget;
  permissions: string[];
  message_id?: string;
  approval_id: string;
  state: "pending" | "active" | "denied" | "expired" | "revoked" | "ended";
  created_at: string;
  expires_at?: string;
};
export type ActionInvocation = {
  grant_id: string;
  action: string;
  version: number;
  args: unknown;
  /** Keep the same key when recovering a lost response; never retry input
   * with a fresh key automatically. Deduplication is enforced by the host. */
  idempotency_key: string;
};
export type ActionFailure = { code: string; message: string; retryable: boolean; applied?: number };
export type ActionJob<T = unknown> = {
  id: string;
  grant_id: string;
  action: string;
  state: "queued" | "running" | "succeeded" | "failed" | "cancelled";
  result?: T;
  error?: ActionFailure;
};
export type ActionArtifact = { id: string; media_type: string; bytes: number; expires_at: string };
export type ActionOptions = { signal?: AbortSignal };

/** Implemented by an authenticated framework adapter, never by forwarding
 * arbitrary page JavaScript to myco.os. An absent adapter means unsupported. */
export interface ActionTransport {
  requestGrant(request: GrantRequest, options?: ActionOptions): Promise<ActionGrant>;
  getGrant(id: string, options?: ActionOptions): Promise<ActionGrant>;
  endGrant(id: string, options?: ActionOptions): Promise<ActionGrant>;
  listActions(grant: string, options?: ActionOptions): Promise<ActionDescriptor[]>;
  invoke(request: ActionInvocation, options?: ActionOptions): Promise<ActionJob>;
  getJob(id: string, options?: ActionOptions): Promise<ActionJob>;
  cancelJob(id: string, options?: ActionOptions): Promise<ActionJob>;
  /** Rechecks the authenticated agent and live grant; no public URL fallback. */
  readArtifact(grant: string, id: string, options?: ActionOptions): Promise<Uint8Array>;
}

export class ActionError extends Error {
  readonly code: string;
  readonly retryable: boolean;
  readonly applied?: number;
  constructor(error: ActionFailure) {
    super(error.message);
    this.name = "ActionError";
    this.code = error.code;
    this.retryable = error.retryable;
    this.applied = error.applied;
  }
}
export function actionError(value: unknown): ActionError {
  if (value instanceof ActionError) return value;
  const e = value as (Partial<ActionFailure> & { name?: string }) | null;
  return new ActionError({ code: typeof e?.code === "string" ? e.code : e?.name === "AbortError" ? "cancelled" : "failed", message: e?.message || String(value), retryable: e?.retryable === true, applied: e?.applied });
}
function fail(code: string, message: string): never {
  throw new ActionError({ code, message, retryable: false });
}
function checkAbort(options?: ActionOptions) {
  if (options?.signal?.aborted) fail("cancelled", "Stopped waiting for the action.");
}
async function call<T>(fn: () => Promise<T>, options?: ActionOptions): Promise<T> {
  checkAbort(options);
  try { return await fn(); } catch (e) { throw actionError(e); }
}
function delay(ms: number, signal?: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    if (signal?.aborted) { reject(new ActionError({ code: "cancelled", message: "Stopped waiting for the action.", retryable: false })); return; }
    const stop = () => { clearTimeout(timer); signal?.removeEventListener("abort", stop); reject(new ActionError({ code: "cancelled", message: "Stopped waiting for the action.", retryable: false })); };
    const timer = setTimeout(() => { signal?.removeEventListener("abort", stop); resolve(); }, ms);
    signal?.addEventListener("abort", stop, { once: true });
  });
}

/** Framework lifecycle SDK. Does not know any provider's action names. */
export class ActionClient {
  constructor(private readonly transport: ActionTransport) {}
  request(request: GrantRequest, options?: ActionOptions) {
    return call(() => this.transport.requestGrant(request, options), options);
  }
  async session(id: string, options?: ActionOptions): Promise<ActionSession> {
    const grant = await call(() => this.transport.getGrant(id, options), options);
    if (grant.id !== id) fail("invalid_response", "The transport returned a different grant.");
    return new ActionSession(this.transport, grant);
  }
}

export class ActionSession {
  private value: ActionGrant;
  constructor(private readonly transport: ActionTransport, grant: ActionGrant) {
    this.value = structuredClone(grant);
  }
  /** A copy: mutating the caller's view cannot broaden local permission checks. */
  get grant(): ActionGrant { return structuredClone(this.value); }
  async refresh(options?: ActionOptions): Promise<ActionGrant> {
    const next = await call(() => this.transport.getGrant(this.value.id, options), options);
    if (next.id !== this.value.id) fail("invalid_response", "The transport returned a different grant.");
    this.value = structuredClone(next);
    return this.grant;
  }
  private active() {
    if (this.value.state !== "active") fail("permission_denied", `The grant is ${this.value.state}.`);
    if (!this.value.expires_at || !Number.isFinite(Date.parse(this.value.expires_at))) fail("invalid_response", "An active grant must have an expiry.");
    if (Date.parse(this.value.expires_at) <= Date.now()) fail("grant_expired", "The grant has expired.");
  }
  actions(options?: ActionOptions): Promise<ActionDescriptor[]> {
    this.active();
    return call(() => this.transport.listActions(this.value.id, options), options);
  }
  async invoke<T>(action: string, version: number, args: unknown, key: string, options?: ActionOptions): Promise<ActionTask<T>> {
    this.active();
    if (!key.trim()) fail("invalid_argument", "An idempotency key is required.");
    const descriptor = (await this.actions(options)).find(d => d.name === action && d.version === version);
    if (!descriptor) fail("unsupported", "This action version is unavailable on the target.");
    if (!this.value.permissions.includes(descriptor.permission)) fail("permission_denied", "This action needs a separately approved permission.");
    // Recheck after discovery: it may have taken long enough for expiry/end.
    this.active();
    const job = await call(() => this.transport.invoke({ grant_id: this.value.id, action, version, args, idempotency_key: key }, options), options);
    return new ActionTask<T>(this.transport, this.value.id, action, job);
  }
  read(artifact: ActionArtifact, options?: ActionOptions): Promise<Uint8Array> {
    this.active();
    if (!Number.isFinite(Date.parse(artifact.expires_at)) || Date.parse(artifact.expires_at) <= Date.now()) fail("artifact_expired", "The artifact has expired.");
    return call(() => this.transport.readArtifact(this.value.id, artifact.id, options), options);
  }
  async end(options?: ActionOptions): Promise<ActionGrant> {
    const next = await call(() => this.transport.endGrant(this.value.id, options), options);
    if (next.id !== this.value.id || !["ended", "revoked", "expired", "denied"].includes(next.state)) fail("invalid_response", "The transport did not end this grant.");
    this.value = structuredClone(next);
    return this.grant;
  }
}

export class ActionTask<T> {
  private value: ActionJob<T>;
  constructor(private readonly transport: ActionTransport, private readonly grant: string, private readonly action: string, job: ActionJob) {
    this.value = this.checked(job);
  }
  get job(): ActionJob<T> { return structuredClone(this.value); }
  private checked(job: ActionJob): ActionJob<T> {
    if (!job.id || job.grant_id !== this.grant || job.action !== this.action || (this.value && job.id !== this.value.id)) fail("invalid_response", "The transport returned a different action job.");
    if (!["queued", "running", "succeeded", "failed", "cancelled"].includes(job.state)) fail("invalid_response", "Unknown action job state.");
    return structuredClone(job) as ActionJob<T>;
  }
  private terminal() { return ["succeeded", "failed", "cancelled"].includes(this.value.state); }
  private update(job: ActionJob) {
    const next = this.checked(job);
    // A response from an earlier long poll may arrive after cancellation or
    // completion. It cannot reopen the terminal task or regress to queued.
    if (!this.terminal() && !(this.value.state === "running" && next.state === "queued")) this.value = next;
    return this.job;
  }
  async refresh(options?: ActionOptions): Promise<ActionJob<T>> {
    if (this.terminal()) return this.job;
    return this.update(await call(() => this.transport.getJob(this.value.id, options), options));
  }
  async cancel(options?: ActionOptions): Promise<ActionJob<T>> {
    if (this.terminal()) return this.job;
    return this.update(await call(() => this.transport.cancelJob(this.value.id, options), options));
  }
  /** Aborting stops this observer, not the native operation. Use cancel() to
   * request cancellation; neither cancellation nor a timeout undoes input. */
  async wait(options: ActionOptions & { interval_ms?: number; onUpdate?: (job: ActionJob<T>) => void } = {}): Promise<T> {
    const interval = options.interval_ms ?? 250;
    if (!Number.isFinite(interval) || interval < 10 || interval > 30_000) fail("invalid_argument", "Polling interval must be between 10 and 30000 ms.");
    let lastState = "";
    for (;;) {
      checkAbort(options);
      const job = this.job;
      if (job.state !== lastState) { options.onUpdate?.(job); lastState = job.state; }
      if (job.state === "succeeded") return job.result as T;
      if (job.state === "failed") throw new ActionError(job.error || { code: "failed", message: "The action failed.", retryable: false });
      if (job.state === "cancelled") fail("cancelled", "The action was cancelled; prior input may already have been applied.");
      await delay(interval, options.signal);
      await this.refresh(options);
    }
  }
}
