// SDK contract tests. The real Hub/approval/sshd journey lives in Go integration
// tests; these check that JavaScript preserves its resource boundary and results.
import assert from "node:assert/strict";
import { actionModule } from "./action-test-modules.mjs";
const { ActionClient, ActionSession } = await import(
  await actionModule("_lib/actions")
);
const { ActionHTTPTransport } = await import(
  await actionModule("_lib/actions-http")
);
const { ServerSSHClient, validateOperation } = await import(
  await actionModule("server-console/agent-actions")
);
const { parseOverview, parseServices, parseContainers } = await import(
  await actionModule("server-console/protocol")
);
const target = {
  kind: "machine",
  id: "mch_1",
  resource: {
    provider: "server.ssh",
    id: "ssh_1",
    binding: "a".repeat(64),
    name: "Fixture",
    summary: "root@fixture:22",
  },
};
let request;
const http = new ActionHTTPTransport(async (path, init) => {
  request = { path, init };
  return new Response(JSON.stringify({ targets: [target] }), {
    status: 200,
    headers: { "content-type": "application/json" },
  });
});
assert.deepEqual(await new ActionClient(http).targets(), [target]);
assert.equal(request.path, "/v1/me/action-targets");
const old = new ActionHTTPTransport(
  async () =>
    new Response(JSON.stringify({ error: "not_found", message: "old Hub" }), {
      status: 404,
    }),
);
await assert.rejects(old.listTargets(), (e) => e.code === "unsupported");
await assert.rejects(
  new ActionClient({}).targets(),
  (e) => e.code === "unsupported",
);
for (const [name, args] of [
  ["services.restart", { name: "ssh.service;reboot" }],
  ["services.stop", { name: "default.target" }],
  ["containers.logs", { name: "web", lines: 201 }],
  ["overview", { profile_id: "another" }],
  ["containers.restart", { name: "-force" }],
])
  assert.throws(
    () => validateOperation(name, args),
    (e) => e.code === "invalid_argument",
  );
const now = new Date().toISOString(),
  expiry = new Date(Date.now() + 60000).toISOString();
let grant = {
    id: "g1",
    agent: "a",
    owner: "o",
    target,
    permissions: ["server.ssh.read"],
    state: "active",
    created_at: now,
    expires_at: expiry,
    approval_id: "ap1",
  },
  calls = [];
const result = {
  stdout: "",
  stderr: "permission denied",
  exit_code: 1,
  truncated: false,
  duration_ms: 3,
};
const transport = {
  getGrant: async () => grant,
  listActions: async () => [
    {
      name: "server.ssh.services.restart",
      version: 1,
      title: "Restart",
      permission: "server.ssh.manage",
    },
    {
      name: "server.ssh.overview",
      version: 1,
      title: "Overview",
      permission: "server.ssh.read",
    },
  ],
  invoke: async (r) => {
    calls.push(r);
    return {
      id: "j1",
      grant_id: "g1",
      action: r.action,
      version: 1,
      state: "succeeded",
      result,
      created_at: now,
    };
  },
};
const session = new ActionSession(transport, grant),
  sdk = new ServerSSHClient(session);
await assert.rejects(
  sdk.run("services.restart", { name: "web.service" }, "k1"),
  (e) => e.code === "permission_denied",
);
assert.equal(calls.length, 0);
const task = await sdk.run("overview", {}, "read1");
assert.deepEqual(task.job.result, result);
assert.equal((await sdk.readResult(result)).exit_code, 1);
assert.deepEqual(calls[0].args, {});
assert.equal(calls[0].idempotency_key, "read1");
grant = {
  ...grant,
  target: {
    ...target,
    resource: { ...target.resource, binding: "b".repeat(64) },
  },
};
await session.refresh();
assert.throws(
  () => sdk.run("overview", {}, "read2"),
  (e) => e.code === "target_changed",
);
assert.throws(
  () =>
    new ServerSSHClient(
      new ActionSession(transport, {
        ...grant,
        target: { kind: "machine", id: "mch_1" },
      }),
    ),
  (e) => e.code === "invalid_argument",
);
const success = (stdout) => ({
  stdout,
  stderr: "",
  exit_code: 0,
  truncated: false,
  duration_ms: 1,
});
assert.deepEqual(
  parseServices(success("nginx.service loaded active running Web server\n")),
  [
    {
      name: "nginx.service",
      active: "active",
      sub: "running",
      description: "Web server",
    },
  ],
);
assert.deepEqual(parseServices(success("")), []);
assert.throws(() => parseServices({ ...success(""), exit_code: 1 }));
assert.throws(() =>
  parseServices({
    ...success("nginx.service loaded active running web"),
    truncated: true,
  }),
);
assert.deepEqual(
  parseContainers(
    success(
      '{"ID":"abc","Names":"web","Image":"nginx:alpine","State":"running","Status":"Up","Ports":"80/tcp"}',
    ),
  ).map((c) => c.id),
  ["abc"],
);
assert.throws(() => parseContainers(success("not JSON")));
assert.throws(() => parseOverview(success("@@myco:os\nLinux 6 x86_64\n")));
console.log(
  "PASS resource discovery, old-Hub failure, grant binding, permission boundaries, nonzero outcomes and strict resource adapters",
);
