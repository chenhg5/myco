// Typed transport for the desktop's local database capabilities. Credentials and
// result sets never enter page data; only explicit shared queries use the Store.
import { os } from "../_lib/os";
import { uid } from "../_lib/appkit";
export type Engine = "mysql" | "postgres" | "sqlite" | "mongodb";
export type Profile = { id: string; name: string; engine: Engine; host: string; port: number; user: string; database: string; path: string; tls: "verify" | "require" | "disable"; auth_source: string; read_only: boolean; has_password: boolean; has_uri: boolean; version: number };
export type ProfileInput = Profile & { password?: string; uri?: string };
export type Cell = { kind: "null" | "text" | "number" | "bool" | "json" | "binary"; value?: string };
export type Column = { name: string; type: string; nullable: boolean; default: string; primary: boolean; generated: boolean };
export type Table = { schema: string; name: string; kind: string };
export type Row = { values: Cell[]; key?: Record<string, Cell>; token?: string; document?: string };
export type Result = { columns: Column[]; rows: Row[]; affected?: string; truncated: boolean; read_only: boolean; duration_ms: number };
export type Params = { connection: string; database?: string; schema?: string; table?: string; action?: string; sql?: string; mode?: string; limit?: number; offset?: number; order_by?: string; descending?: boolean; key?: Record<string, Cell>; token?: string; values?: Record<string, Cell>; query?: string; document?: string; original?: string; timeout_ms?: number };
export const profiles = () => os().call<Profile[]>("db.profile", { action: "list" });
export const saveProfile = (profile: ProfileInput) => os().call<Profile>("db.profile", { action: "save", profile });
export const testProfile = (profile: ProfileInput) => os().call<{ connected: boolean }>("db.profile", { action: "test", profile });
export const forgetProfile = (profile: Profile) => os().call("db.profile", { action: "forget", id: profile.id, version: profile.version });
export function request<T>(kind: "catalog" | "rows" | "query" | "mongo", params: Params) {
  const id = uid("db_");
  return { id, done: os().call<T>("db." + kind, { ...params, request_id: id }), cancel: () => os().call("db.cancel", { request_id: id }) };
}
export const engines: { id: Engine; name: string; port: number }[] = [{ id: "mysql", name: "MySQL", port: 3306 }, { id: "postgres", name: "PostgreSQL", port: 5432 }, { id: "sqlite", name: "SQLite", port: 0 }, { id: "mongodb", name: "MongoDB", port: 27017 }];
export const engineName = (type: Engine) => engines.find(e => e.id === type)?.name || type;
export const freshProfile = (): ProfileInput => ({ id: "", name: "", engine: "postgres", host: "127.0.0.1", port: 5432, user: "", database: "", path: "", tls: "disable", auth_source: "admin", read_only: true, has_password: false, has_uri: false, version: 0 });
