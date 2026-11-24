// UI-facing view models, independent of native SSH transport and framework grants.
export type Profile = {
  id: string;
  name: string;
  target: string;
  port: number;
  fingerprint?: string;
};
export type Sample = { at: number; cpu: number | null; memory: number | null };
export type Disk = {
  mount: string;
  total: number;
  used: number;
  available: number;
};
export type Overview = {
  hostname: string;
  os: string;
  kernel: string;
  arch: string;
  uptime: number;
  cpus: number;
  cpu_percent: number | null;
  memory_total: number;
  memory_available: number;
  swap_total: number;
  swap_free: number;
  load: number[];
  disks: Disk[];
  sampled_at: string;
};
export type Service = {
  name: string;
  description: string;
  active: string;
  sub: string;
  enabled?: string;
};
export type Container = {
  id: string;
  name: string;
  image: string;
  state: string;
  status: string;
  ports: string;
};
export type Section<T> = { available: boolean; items: T[]; reason?: string };
export type Operation = "start" | "stop" | "restart";
export type Activity = {
  id: string;
  at: number;
  target: string;
  action: string;
  status: "running" | "succeeded" | "failed" | "unknown";
  detail: string;
};
