// Types mirror internal/api (the hub's JSON contract).
export interface Binding { harness?: string; session_id?: string; environment?: string; workdir?: string }
export interface Delivery { mode: string; url?: string; has_secret?: boolean; params?: Record<string, string> }
export interface Agent {
  id: string; name: string; display_name?: string; aliases?: string[]; description: string; capabilities: string[]; labels: string[];
  binding: Binding; delivery: Delivery; allow_from: string[]; enrolled_by?: string;
  online: boolean; created_at: string; last_seen_at?: string;
}
export interface Attachment { file_id: string; name: string; size: number; sha256: string; content_type?: string }
export interface Message {
  id: string; from: string; to: string; thread_id: string; reply_to?: string; type: string; body: string;
  status: string; attempts: number; error?: string; broadcast_id?: string; attachments?: Attachment[];
  created_at: string; updated_at: string;
}
export type State = "working" | "pending" | "idle" | "offline";
export interface AgentStatus { agent: Agent; state: State; queued: number; in_progress: number; held: number; current?: Message[] }
export interface Overview {
  generated_at: string; viewer: { kind: "agent" | "admin"; name?: string }; online_window_seconds: number;
  totals: { agents: number; working: number; pending: number; idle: number; offline: number };
  agents: AgentStatus[]; recent: Message[];
}
export interface Session { hub: string; version: string; agent?: string; admin: boolean }
export type ViewAs = "agent" | "admin";

async function get<T>(path: string): Promise<T> {
  const r = await fetch(path, { credentials: "same-origin", headers: { Accept: "application/json" } });
  const text = await r.text();
  let data: unknown = null;
  try { data = text ? JSON.parse(text) : null; } catch { /* non-JSON */ }
  if (!r.ok) {
    const e = data as { message?: string; suggestion?: string } | null;
    throw new Error([e?.message || `HTTP ${r.status}`, e?.suggestion].filter(Boolean).join(" — "));
  }
  return data as T;
}

export const api = {
  session: () => get<Session>("api/session"),
  overview: (as: ViewAs) => get<Overview>(`api/overview?as=${as}`),
  thread: (id: string, as: ViewAs) => get<{ messages: Message[] }>(`api/threads/${encodeURIComponent(id)}?as=${as}`),
};
