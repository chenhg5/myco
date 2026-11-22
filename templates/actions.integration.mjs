// Run by TestAgentActionsSDK against an isolated real Hub. Never run against
// a user's workspace: the fixture has disposable in-memory test credentials.
import assert from 'node:assert/strict';
import { actionModule } from './action-test-modules.mjs';

let input = '';
for await (const part of process.stdin) input += part;
const fixture = JSON.parse(input);
input = '';
const { ActionClient } = await import(await actionModule('_lib/actions'));
const { ActionHTTPTransport } = await import(await actionModule('_lib/actions-http'));
const { RemoteDesktopActions, desktopClick } = await import(await actionModule('remote-desktop/agent-actions'));
const request = token => (path, init = {}) => fetch(fixture.url + path, {
  ...init, headers: { ...init.headers, Authorization: `Bearer ${token}` },
  signal: init.signal || AbortSignal.timeout(10_000),
});
const owner = request(fixture.owner);
const agent = new ActionHTTPTransport(request(fixture.agent));
const other = new ActionHTTPTransport(request(fixture.other));
const client = new ActionClient(agent);
async function hostCall(path, body) {
  const response = await owner(path, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
  assert.equal(response.ok, true, `Fixture host request failed: ${response.status}`);
  return response.json();
}
const permissions = ['remote.desktop.view', 'remote.desktop.control'];
const requested = await client.request({ target: { kind: 'machine', id: 'sdk-studio' }, permissions, reason: 'SDK integration only' });
assert.equal(requested.state, 'pending');
assert.notEqual(requested.target.id, 'sdk-studio', 'Hub resolves computer names to IDs');
assert.equal(requested.target_name, 'sdk-studio');
assert.deepEqual(requested.requested, [...permissions].sort());
assert.equal(requested.permissions, null, 'pending grants carry no approved permissions');
assert.equal((await client.grants()).length, 1);
const pending = await client.session(requested.id);
await assert.rejects(async () => pending.actions(), { code: 'permission_denied' });
await assert.rejects(async () => new RemoteDesktopActions(pending).input({ frame_id: 'not-authorized' }, [{ kind: 'release' }], 'pending-input'), { code: 'permission_denied' });
await assert.rejects(() => agent.invoke({ grant_id: requested.id, action: 'remote.desktop.observe', version: 1, args: {}, idempotency_key: 'pending' }), { code: 'invalid_state' });
// Approval and provider simulation use the isolated owner's credential,
// separately from the SDK's agent request adapter.
await hostCall(`/v1/console/approvals/${requested.approval_id}/approve`, { permissions: ['remote.desktop.view'], ttl_seconds: 120 });
const session = await client.session(requested.id, { wait_seconds: 1 });
assert.equal(session.grant.state, 'active');
assert.deepEqual(session.grant.permissions, ['remote.desktop.view']);
await assert.rejects(() => other.getGrant(requested.id), { code: 'not_found' });
const desktop = new RemoteDesktopActions(session);
await assert.rejects(() => desktop.observe('no-host'), { code: 'unsupported' });
await assert.rejects(() => agent.invoke({ grant_id: requested.id, action: 'remote.desktop.observe', version: 1, args: {}, idempotency_key: 'no-host-http' }), { code: 'unsupported', status: 501 });
const host = await hostCall('/v1/console/action-hosts', {
  machine_id: requested.target.id, provider: 'remote.desktop', generation: 'sdk-fixture-1',
  actions: [
    { name: 'remote.desktop.observe', version: 1, title: 'Observe', permission: permissions[0] },
    { name: 'remote.desktop.input', version: 1, title: 'Input', permission: permissions[1] },
  ],
});
const descriptors = await session.actions();
assert.equal(descriptors.length, 1);
assert.equal(descriptors[0].input_schema, undefined, 'schemas may be omitted by the wire contract');
const task = await desktop.observe('observe-once', { max_width: 800 });
assert.equal(task.job.version, 1);
assert.ok(task.job.created_at);
assert.equal((await desktop.observe('observe-once', { max_width: 800 })).job.id, task.job.id);
await assert.rejects(() => desktop.observe('observe-once', { max_width: 900 }), { code: 'conflict', status: 409 });
const pulled = await (await owner(`/v1/console/action-hosts/${host.id}/jobs?wait_seconds=1`)).json();
assert.equal(pulled.jobs.length, 1, 'idempotent admission produces one host job');
assert.deepEqual(pulled.jobs[0].args, { max_width: 800 });
const png = Uint8Array.from(Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aS1EAAAAASUVORK5CYII=', 'base64'));
const uploadPath = `/v1/console/action-jobs/${task.job.id}/artifacts?host_id=${host.id}&generation=sdk-fixture-1`;
const upload = await owner(uploadPath, { method: 'POST', headers: { 'Content-Type': 'image/png' }, body: png });
assert.equal(upload.ok, true);
const artifact = await upload.json();
const frame = { frame_id: 'sdk-frame-1', display_id: 'test-display', captured_at: new Date().toISOString(), width: 1, height: 1, artifact };
await hostCall(`/v1/console/action-jobs/${task.job.id}/complete`, { host_id: host.id, generation: 'sdk-fixture-1', state: 'succeeded', result: frame, artifact_ids: [artifact.id] });
const result = await task.wait({ interval_ms: 10 });
assert.deepEqual(result, frame);
assert.deepEqual(task.job.artifacts, [artifact]);
assert.ok(task.job.finished_at);
assert.deepEqual(await desktop.read(result), png);
await assert.rejects(() => other.readArtifact(requested.id, artifact.id), { code: 'not_found', status: 410 });
await assert.rejects(async () => desktop.input(frame, desktopClick(frame, 0, 0), 'view-cannot-input'), { code: 'permission_denied' });
await assert.rejects(() => agent.invoke({ grant_id: requested.id, action: 'remote.desktop.input', version: 1, args: { frame_id: frame.frame_id, events: desktopClick(frame, 0, 0) }, idempotency_key: 'bypass-sdk' }), { code: 'permission_denied' });
const cancelled = await desktop.observe('cancel-queued');
assert.equal((await cancelled.cancel()).state, 'cancelled');
await assert.rejects(() => cancelled.wait(), { code: 'cancelled' });
await hostCall(`/v1/console/action-grants/${requested.id}/revoke`, {});
// The SDK's cached grant is still locally active. The real Hub must enforce
// revocation again and return its actual 410/not_found, not a fake success.
await assert.rejects(() => desktop.read(result), { code: 'not_found', status: 410 });
assert.equal((await session.refresh()).state, 'revoked');
await assert.rejects(async () => desktop.observe('revoked'), { code: 'permission_denied' });

// A separately approved control grant exercises domain arguments/results on
// the same real transport. Execution below is deliberately a simulated host.
const controlRequest = await client.request({ target: requested.target, permissions, reason: 'Simulated control round trip' });
await hostCall(`/v1/console/approvals/${controlRequest.approval_id}/approve`, {});
const controlled = new RemoteDesktopActions(await client.session(controlRequest.id));
async function observeControl(key) {
  const task = await controlled.observe(key, { format: 'png' });
  const work = await (await owner(`/v1/console/action-hosts/${host.id}/jobs?wait_seconds=1`)).json();
  assert.equal(work.jobs.length, 1);
  assert.equal(work.jobs[0].grant.id, controlRequest.id);
  assert.equal(work.jobs[0].job.id, task.job.id);
  const upload = await owner(`/v1/console/action-jobs/${task.job.id}/artifacts?host_id=${host.id}&generation=sdk-fixture-1`, { method: 'POST', headers: { 'Content-Type': 'image/png' }, body: png });
  assert.equal(upload.ok, true);
  const artifact = await upload.json();
  const observed = { ...frame, frame_id: key, artifact };
  await hostCall(`/v1/console/action-jobs/${task.job.id}/complete`, { host_id: host.id, generation: 'sdk-fixture-1', state: 'succeeded', result: observed, artifact_ids: [artifact.id] });
  return task.wait({ interval_ms: 10 });
}
const before = await observeControl('control-before');
const events = [...desktopClick(before, 0, 0), { kind: 'text', text: '你好🙂' }];
const inputTask = await controlled.input(before, events, 'control-input');
const work = await (await owner(`/v1/console/action-hosts/${host.id}/jobs?wait_seconds=1`)).json();
assert.equal(work.jobs.length, 1);
assert.deepEqual(work.jobs[0].args, { frame_id: before.frame_id, events });
await hostCall(`/v1/console/action-jobs/${inputTask.job.id}/complete`, { host_id: host.id, generation: 'sdk-fixture-1', state: 'succeeded', result: { applied: events.length } });
assert.deepEqual(await inputTask.wait({ interval_ms: 10 }), { applied: 3 });
const after = await observeControl('control-after');
assert.notEqual(after.frame_id, before.frame_id);
for (const applied of [0, 1]) {
  const failed = await controlled.input(after, events, `partial-${applied}`);
  const work = await (await owner(`/v1/console/action-hosts/${host.id}/jobs?wait_seconds=1`)).json();
  assert.equal(work.jobs.length, 1);
  await hostCall(`/v1/console/action-jobs/${failed.job.id}/complete`, {
    host_id: host.id, generation: 'sdk-fixture-1', state: 'failed',
    error: { code: 'outcome_unknown', message: 'Simulated native acknowledgement failure', retryable: false, applied },
  });
  await assert.rejects(() => failed.wait({ interval_ms: 10 }), e => e.code === 'outcome_unknown' && e.applied === applied && !e.retryable);
}
assert.equal((await controlled.end()).state, 'ended');
await assert.rejects(() => agent.readArtifact(controlRequest.id, after.artifact.id), { code: 'not_found', status: 410 });
const ended = await client.request({ target: requested.target, permissions: ['remote.desktop.view'], reason: 'End pending request' });
assert.equal((await (await client.session(ended.id)).end()).state, 'ended');
console.log('PASS actual SDK → real Hub HTTP: name resolution, narrowed approval, private artifacts, 410, idempotency, permissions, cancellation, revoke and simulated observe/input/observe/end');
