// Human UI transport: fixed native SSH actions, immutable binding on every call.
// Private profile data and command output never enter shared page records.
import { myco } from "../_lib/myco";
import type { Profile } from "./types";
export type NativeProfile = Profile & {
  destination: string;
  host: string;
  user: string;
  binding: string;
  agent_enabled: boolean;
  agent_hub?: string;
};
import { validResult, type RunResult } from "./protocol";
export type { RunResult } from "./protocol";
export type SSHAction =
  | "probe"
  | "overview"
  | "services.list"
  | "services.status"
  | "services.logs"
  | "services.start"
  | "services.stop"
  | "services.restart"
  | "containers.list"
  | "containers.logs"
  | "containers.start"
  | "containers.stop"
  | "containers.restart";
export function sshAvailable() {
  return !!myco?.features?.includes("ssh") && !!myco.os;
}
function bridge() {
  if (!sshAvailable())
    throw Object.assign(new Error("unsupported"), { code: "unsupported" });
  return myco!.os!;
}
function normalize(p: NativeProfile): NativeProfile {
  return { ...p, target: `${p.user}@${p.host}` };
}
export async function sshReady() {
  return sshAvailable() && (await myco!.os!.has("ssh.run"));
}
export async function profiles() {
  const r = await bridge().call<{ profiles: NativeProfile[] }>("ssh.profiles", {
    op: "list",
  });
  if (!Array.isArray(r.profiles)) throw new Error("invalid_response");
  return r.profiles.map(normalize);
}
export async function createProfile(d: {
  name: string;
  target: string;
  port?: number;
}) {
  return normalize(
    await bridge().call<NativeProfile>("ssh.profiles", {
      op: "create",
      name: d.name,
      destination: d.target,
      ...(d.port === undefined ? {} : { port: d.port }),
    }),
  );
}
export async function removeProfile(id: string) {
  return bridge().call("ssh.profiles", { op: "delete", id });
}
export async function setAgent(profile: NativeProfile, enabled: boolean) {
  return normalize(
    await bridge().call<NativeProfile>("ssh.profiles", {
      op: "agent_enabled",
      id: profile.id,
      agent_enabled: enabled,
    }),
  );
}
export async function run(
  profile: NativeProfile,
  action: SSHAction,
  args: { name?: string; lines?: number } = {},
): Promise<RunResult> {
  const r = await bridge().call<RunResult>(
    "ssh.run",
    { profile_id: profile.id, binding: profile.binding, action, ...args },
    { timeout: 35000 },
  );
  if (!validResult(r))
    throw Object.assign(new Error("invalid_response"), {
      code: "invalid_response",
    });
  return r;
}
export function requireSuccess(result: RunResult): RunResult {
  if (result.exit_code !== 0)
    throw Object.assign(
      new Error(result.stderr.trim() || `SSH exit ${result.exit_code}`),
      { code: "remote_exit", result },
    );
  return result;
}
