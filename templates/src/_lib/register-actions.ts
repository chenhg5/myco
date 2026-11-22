// Typed registration and lifecycle only. Applications own their action names,
// argument schemas, content validation and render/export implementations.
import { myco, type PageActionBridge, type PageActionDescriptor, type PageActionFile, type PageActionRequest } from "./myco";
import { ActionError } from "./actions";

export const pageActionMaxBytes = 4 << 20;
export type PageActionContext = { id: string; signal: AbortSignal };
export type PageAction = PageActionDescriptor & {
  handler(args: unknown, context: PageActionContext): PageActionFile | Promise<PageActionFile>;
};
export type ActionRegistration = { supported: boolean; dispose(): void };
type RegistrationBridge = { features?: string[]; actions?: PageActionBridge };

function error(code: string, message: string): never {
  throw new ActionError({ code, message, retryable: false });
}
function check(signal: AbortSignal) { if (signal.aborted) error("cancelled", "The page action was stopped."); }

/** Old bridges remain usable for editing/manual exports. Registering support
 * does not approve a job or make a host available: the framework owns that. */
export function registerActions(definitions: readonly PageAction[], bridge: RegistrationBridge | undefined = myco): ActionRegistration {
  const actions = new Map<string, PageAction>();
  for (const definition of definitions) {
    if (typeof definition.name !== "string" || !definition.name || definition.name.length > 128 || !Number.isSafeInteger(definition.version) || definition.version < 1 ||
      typeof definition.handler !== "function" || !definition.input_schema || typeof definition.input_schema !== "object" || Array.isArray(definition.input_schema))
      error("invalid_argument", "An action requires a name, version, object input schema and handler.");
    const key = definition.name + ":" + definition.version;
    if (actions.has(key)) error("invalid_argument", "Duplicate action name and version.");
    actions.set(key, definition);
  }
  if (!bridge?.features?.includes("page-actions") || !bridge.actions?.register) return { supported: false, dispose() {} };
  let disposed = false;
  const active = new Set<AbortController>();
  const invoke = async (request: PageActionRequest): Promise<PageActionFile> => {
    if (disposed) error("cancelled", "The action registration was disposed.");
    const action = actions.get(request.action + ":" + request.version);
    if (!action) error("unsupported", "This application has no matching action version.");
    const controller = new AbortController();
    const abort = () => controller.abort();
    request.signal.addEventListener("abort", abort, { once: true });
    if (request.signal.aborted) abort();
    active.add(controller);
    let stopped: (() => void) | undefined;
    try {
      check(controller.signal);
      const cancelled = new Promise<never>((_, reject) => {
        stopped = () => reject(new ActionError({ code: "cancelled", message: "The page action was stopped.", retryable: false }));
        controller.signal.addEventListener("abort", stopped, { once: true });
      });
      const result = await Promise.race([Promise.resolve().then(() => {
        check(controller.signal);
        return action.handler(request.args ?? {}, { id: request.id, signal: controller.signal });
      }), cancelled]);
      check(controller.signal);
      if (!result || typeof result.media_type !== "string" || result.media_type.length > 256 || !/^[\w.+-]+\/[\w.+-]+(?:;[\x20-\x7e]+)?$/.test(result.media_type))
        error("export_failed", "The application returned an invalid media type.");
      const data = result.data;
      const size = data instanceof Blob ? data.size : data instanceof ArrayBuffer || data instanceof Uint8Array ? data.byteLength : -1;
      if (size < 1) error("export_failed", "The application returned no file bytes.");
      if (size > pageActionMaxBytes) error("too_large", "The export exceeds the host's 4 MB artifact limit.");
      for (const key of ["width", "height", "pages"] as const) {
        const value = result[key];
        if (value !== undefined && (!Number.isSafeInteger(value) || value < 1)) error("export_failed", "Invalid export dimensions or page count.");
      }
      return result;
    } finally {
      if (stopped) controller.signal.removeEventListener("abort", stopped);
      request.signal.removeEventListener("abort", abort);
      active.delete(controller);
    }
  };
  const descriptors = definitions.map(({ name, version, input_schema }) => ({ name, version, input_schema }));
  const unregister = bridge.actions.register(descriptors, invoke);
  return { supported: true, dispose() {
    if (disposed) return;
    disposed = true;
    for (const controller of active) controller.abort();
    active.clear();
    unregister();
  } };
}
