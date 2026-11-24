// Pure adapters for the native SSH provider's bounded, fixed text protocols.
import type { Overview, Service, Container } from "./types";
export type RunResult = {
  stdout: string;
  stderr: string;
  exit_code: number;
  truncated: boolean;
  duration_ms: number;
};
export function validResult(r: unknown): r is RunResult {
  if (!r || typeof r !== "object") return false;
  const v = r as RunResult;
  return (
    typeof v.stdout === "string" &&
    typeof v.stderr === "string" &&
    Number.isInteger(v.exit_code) &&
    typeof v.truncated === "boolean" &&
    Number.isFinite(v.duration_ms) &&
    v.duration_ms >= 0
  );
}
function invalid(): never {
  throw Object.assign(new Error("invalid_response"), {
    code: "invalid_response",
  });
}
function complete(r: RunResult) {
  if (!validResult(r) || r.exit_code !== 0 || r.truncated) invalid();
}
export function parseOverview(r: RunResult): Overview {
  complete(r);
  const sections: Record<string, string[]> = {};
  let section = "";
  for (const line of r.stdout.split(/\r?\n/)) {
    const m = /^@@myco:([a-z0-9]+)$/.exec(line);
    if (m) {
      section = m[1];
      sections[section] = [];
    } else if (section) sections[section].push(line);
  }
  const text = (key: string) => (sections[key] || []).join("\n");
  const os = text("os").split("\n");
  if (!os[0]?.startsWith("Linux ")) invalid();
  const memory: Record<string, number> = {};
  for (const line of sections.memory || []) {
    const m = /^(\w+):\s+(\d+)\s*kB\s*$/.exec(line);
    if (m) memory[m[1]] = Number(m[2]) * 1024;
  }
  if (!(memory.MemTotal > 0)) invalid();
  const available =
    memory.MemAvailable ??
    (memory.MemFree !== undefined
      ? memory.MemFree + (memory.Buffers || 0) + (memory.Cached || 0)
      : NaN);
  if (!Number.isFinite(available)) invalid();
  const cpu = (key: string) =>
      text(key).trim().split(/\s+/).slice(1, 9).map(Number),
    a = cpu("cpu0"),
    b = cpu("cpu1");
  let cpuPercent: null | number = null;
  if (
    a.length >= 4 &&
    b.length === a.length &&
    [...a, ...b].every(Number.isFinite) &&
    b.every((v, i) => v >= a[i])
  ) {
    const total = b.reduce((s, n, i) => s + n - a[i], 0),
      idle = b[3] - a[3] + ((b[4] || 0) - (a[4] || 0));
    if (total > 0 && idle >= 0)
      cpuPercent = Math.max(0, Math.min(100, ((total - idle) / total) * 100));
  }
  const uptime = Number(text("uptime").split(/\s+/)[0]);
  if (!Number.isFinite(uptime) || uptime < 0) invalid();
  const load = (sections.uptime?.[1] || "")
    .split(/\s+/)
    .slice(0, 3)
    .map(Number);
  if (load.length !== 3 || !load.every(Number.isFinite)) invalid();
  const disks = [];
  for (const line of sections.disks || []) {
    const m = /^(.+?)\s+(\d+)\s+(\d+)\s+(\d+)\s+[\d.]+%\s+(.+)$/.exec(line);
    if (m) {
      const values = m.slice(2, 5).map((v) => Number(v) * 1024);
      if (!values.every((v) => Number.isFinite(v) && v >= 0)) invalid();
      disks.push({
        mount: m[5],
        total: values[0],
        used: values[1],
        available: values[2],
      });
    }
  }
  const identity = Object.fromEntries(
      (sections.identity || []).map((line) => {
        const at = line.indexOf("=");
        return at < 0 ? ["", ""] : [line.slice(0, at), line.slice(at + 1)];
      }),
    ),
    count = Number(identity.cpus);
  return {
    hostname: identity.hostname?.trim() || "",
    os: (
      os.find((v) => v.startsWith("PRETTY_NAME="))?.slice(12) || os[0]
    ).replace(/^["']|["']$/g, ""),
    kernel: os[0].split(/\s+/)[1] || "",
    arch: os[0].split(/\s+/)[2] || "",
    uptime,
    cpus: Number.isInteger(count) && count > 0 ? count : 0,
    cpu_percent: cpuPercent,
    memory_total: memory.MemTotal,
    memory_available: Math.min(memory.MemTotal, Math.max(0, available)),
    swap_total: memory.SwapTotal || 0,
    swap_free: memory.SwapFree || 0,
    load,
    disks,
    sampled_at: new Date().toISOString(),
  };
}
export function parseServices(r: RunResult): Service[] {
  complete(r);
  const items: Service[] = [];
  for (const line of r.stdout.split(/\r?\n/)) {
    if (!line.trim()) continue;
    const m =
      /^\s*(?:[●*]\s+)?(\S+\.service)\s+(\S+)\s+(\S+)\s+(\S+)(?:\s+(.*))?$/.exec(
        line,
      );
    if (!m) invalid();
    items.push({
      name: m[1],
      active: m[3],
      sub: m[4],
      description: m[5] || "",
    });
  }
  return items;
}
export function parseContainers(r: RunResult): Container[] {
  complete(r);
  return r.stdout
    .split(/\r?\n/)
    .filter((v) => v.trim())
    .map((line) => {
      let v: Record<string, unknown>;
      try {
        v = JSON.parse(line);
      } catch {
        invalid();
      }
      for (const key of ["ID", "Names", "Image", "State", "Status", "Ports"])
        if (typeof v[key] !== "string") invalid();
      return {
        id: v.ID as string,
        name: v.Names as string,
        image: v.Image as string,
        state: v.State as string,
        status: v.Status as string,
        ports: v.Ports as string,
      };
    });
}
