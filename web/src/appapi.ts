import type { Message, Overview } from "@/api";

// Types mirror the App backend (cmd/myco-app) which proxies to enrolled hubs.
export interface Hub {
  id: string;
  name: string;
  url: string;
  ok: boolean;
  error?: string;
  hub_version?: string;
  update_available?: boolean;
}

export interface HubOverview {
  id: string;
  name: string;
  ok: boolean;
  error?: string;
  overview?: Overview;
}

export interface AppOverview {
  generated_at: string;
  hubs: HubOverview[];
}

export interface NewHub { name: string; url: string; token: string }
export interface ProfileInput { display_name?: string; description?: string }

/** An error returned by the App backend or a hub (same shape as the hub API). */
export class ApiError extends Error {
  status: number;
  code?: string;
  suggestion?: string;
  constructor(message: string, status: number, code?: string, suggestion?: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.suggestion = suggestion;
  }
}

export function errorMessage(e: unknown): string {
  if (e instanceof ApiError) return e.message;
  return e instanceof Error ? e.message : String(e);
}

export function errorSuggestion(e: unknown): string | undefined {
  return e instanceof ApiError ? e.suggestion : undefined;
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = { Accept: "application/json" };
  if (body !== undefined) headers["Content-Type"] = "application/json";
  const r = await fetch(path, {
    method,
    credentials: "same-origin",
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const text = await r.text();
  let data: unknown = null;
  try { data = text ? JSON.parse(text) : null; } catch { /* non-JSON */ }
  if (!r.ok) {
    const e = (data ?? {}) as { error?: string; message?: string; suggestion?: string };
    throw new ApiError(e.message || `HTTP ${r.status}`, r.status, e.error, e.suggestion);
  }
  return data as T;
}

const enc = encodeURIComponent;

/**
 * Probe for App mode. Returns the hub list when the App backend answers 200;
 * otherwise null, which means "plain `myco ui`" and the existing UI is used.
 */
export async function probeAppMode(): Promise<Hub[] | null> {
  try {
    const r = await fetch("app/hubs", { credentials: "same-origin", headers: { Accept: "application/json" } });
    if (!r.ok) return null;
    const data: unknown = await r.json();
    return Array.isArray(data) ? (data as Hub[]) : null;
  } catch {
    return null;
  }
}

export const appApi = {
  hubs: () => request<Hub[]>("GET", "app/hubs"),
  addHub: (input: NewHub) => request<Hub>("POST", "app/hubs", input),
  removeHub: (id: string) => request<void>("DELETE", `app/hubs/${enc(id)}`),
  overview: () => request<AppOverview>("GET", "app/overview"),
  thread: (hubId: string, threadId: string) =>
    request<{ messages: Message[] }>("GET", `app/hubs/${enc(hubId)}/threads/${enc(threadId)}`),
  updateProfile: (hubId: string, name: string, input: ProfileInput) =>
    request<void>("PUT", `app/hubs/${enc(hubId)}/agents/${enc(name)}/profile`, input),
  updateAllowFrom: (hubId: string, name: string, allowFrom: string[]) =>
    request<void>("PUT", `app/hubs/${enc(hubId)}/agents/${enc(name)}/allow_from`, { allow_from: allowFrom }),
  kick: (hubId: string, name: string) =>
    request<void>("POST", `app/hubs/${enc(hubId)}/agents/${enc(name)}/kick`),
  removeAgent: (hubId: string, name: string) =>
    request<void>("DELETE", `app/hubs/${enc(hubId)}/agents/${enc(name)}`),
};
