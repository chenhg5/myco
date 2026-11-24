// Agent-facing domain adapter: resource-bound framework sessions only. No App
// bridge or SSH credentials, and no arbitrary remote shell action.
import {
  ActionError,
  type ActionSession,
  type ActionOptions,
  type ActionTask,
  type ActionArtifact,
} from "../_lib/actions";
import { validResult, type RunResult } from "./protocol";
export type SSHAction =
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
const allowed = new Set<SSHAction>([
  "overview",
  "services.list",
  "services.status",
  "services.logs",
  "services.start",
  "services.stop",
  "services.restart",
  "containers.list",
  "containers.logs",
  "containers.start",
  "containers.stop",
  "containers.restart",
]);
function invalid(message: string): never {
  throw new ActionError({
    code: "invalid_argument",
    message,
    retryable: false,
  });
}
export function validateOperation(
  action: SSHAction,
  args: { name?: string; lines?: number },
) {
  if (
    !allowed.has(action) ||
    !args ||
    typeof args !== "object" ||
    Array.isArray(args) ||
    Object.keys(args).some((k) => !["name", "lines"].includes(k))
  )
    invalid("Unsupported SSH action or argument.");
  if (
    args.lines !== undefined &&
    (!Number.isInteger(args.lines) || args.lines < 1 || args.lines > 200)
  )
    invalid("Log lines must be between 1 and 200.");
  const requiresName = ![
    "overview",
    "services.list",
    "containers.list",
  ].includes(action);
  if (requiresName && (typeof args.name !== "string" || !args.name))
    invalid("A resource name is required.");
  if (
    args.name !== undefined &&
    (!/^[A-Za-z0-9_][A-Za-z0-9_.@:-]{0,159}$/.test(args.name) ||
      (action.startsWith("services.") && !args.name.endsWith(".service")))
  )
    invalid("Unsupported service or container name.");
}
export class ServerSSHClient {
  private readonly targetKey: string;
  constructor(private readonly session: ActionSession) {
    const t = session.grant.target;
    if (
      t.kind !== "machine" ||
      t.resource?.provider !== "server.ssh" ||
      !t.resource.id ||
      !/^[a-f0-9]{64}$/.test(t.resource.binding)
    )
      invalid("A resource-bound server.ssh grant is required.");
    this.targetKey = JSON.stringify([t.kind, t.id, t.resource.provider, t.resource.id, t.resource.binding]);
  }
  run(
    action: SSHAction,
    args: { name?: string; lines?: number },
    key: string,
    options?: ActionOptions,
  ): Promise<
    ActionTask<
      | RunResult
      | {
          artifact: ActionArtifact;
          exit_code: number;
          truncated: boolean;
          duration_ms: number;
        }
    >
  > {
    const t = this.session.grant.target;
    if (
      JSON.stringify([t.kind, t.id, t.resource?.provider, t.resource?.id, t.resource?.binding]) !==
      this.targetKey
    )
      throw new ActionError({
        code: "target_changed",
        message: "The SSH grant target changed.",
        retryable: false,
      });
    validateOperation(action, args);
    return this.session.invoke(`server.ssh.${action}`, 1, args, key, options);
  }
  async readResult(
    result: RunResult | { artifact: ActionArtifact },
    options?: ActionOptions,
  ): Promise<RunResult> {
    let value: unknown = result;
    if ("artifact" in result) {
      const bytes = await this.session.read(result.artifact, options);
      try {
        value = JSON.parse(new TextDecoder().decode(bytes));
      } catch {
        throw new ActionError({
          code: "invalid_response",
          message: "The private SSH result is invalid JSON.",
          retryable: false,
        });
      }
    }
    if (!validResult(value))
      throw new ActionError({
        code: "invalid_response",
        message: "The SSH result has an invalid shape.",
        retryable: false,
      });
    return value;
  }
}
