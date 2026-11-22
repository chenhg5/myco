// SDK/adapter contract tests. The fake service below is not proof that the Hub
// or native component implements grants; those have separate integration tests.
import assert from 'node:assert/strict';
import { actionModule } from './action-test-modules.mjs';

const { ActionClient, ActionError } = await import(await actionModule('_lib/actions'));
const { ActionHTTPTransport } = await import(await actionModule('_lib/actions-http'));
const { RemoteDesktopActions, desktopClick, desktopPoint, validateDesktopInput } = await import(await actionModule('remote-desktop/agent-actions'));
const future = () => new Date(Date.now() + 60_000).toISOString();
const grant = { id: 'g_1', agent: 'agent_1', owner: 'owner_1', target: { kind: 'machine', id: 'machine_1' }, permissions: ['remote.desktop.view'], approval_id: 'a_1', state: 'active', created_at: new Date().toISOString(), expires_at: future() };
const frame = { frame_id: 'frame_1', captured_at: new Date().toISOString(), display_id: 'display_1', width: 1440, height: 900, artifact: { id: 'art_1', media_type: 'image/jpeg', bytes: 3, expires_at: future() } };
const descriptor = name => ({ name: `remote.desktop.${name}`, version: 1, title: name, permission: name === 'input' ? 'remote.desktop.control' : 'remote.desktop.view', input_schema: {}, output_schema: {} });

const seen = [], jobs = new Map(), keys = new Map();
let current = structuredClone(grant), executionCount = 0, cancelled = 0, pollHook;
const fake = {
  requestGrant: async request => ({ ...structuredClone(grant), ...request, state: 'pending' }),
  listGrants: async () => [structuredClone(current)],
  getGrant: async () => structuredClone(current),
  endGrant: async () => { current.state = 'ended'; return structuredClone(current); },
  listActions: async () => {
    if (current.state !== 'active') throw { code: 'permission_denied', message: 'Grant ended on server', retryable: false };
    return [descriptor('observe'), descriptor('input')];
  },
  invoke: async request => {
    seen.push(structuredClone(request));
    const previous = keys.get(request.idempotency_key);
    if (previous) {
      if (JSON.stringify(request) !== JSON.stringify(previous.request)) throw { code: 'conflict', message: 'Key already used with different body', retryable: false };
      return structuredClone(previous.job);
    }
    executionCount++;
    const job = { id: `job_${executionCount}`, grant_id: request.grant_id, action: request.action, version: request.version, created_at: new Date().toISOString(), state: 'queued' };
    jobs.set(job.id, job); keys.set(request.idempotency_key, { request, job });
    return structuredClone(job);
  },
  getJob: async id => {
    if (pollHook) return pollHook(id);
    const job = jobs.get(id); job.state = 'succeeded'; job.result = frame;
    return structuredClone(job);
  },
  cancelJob: async id => { cancelled++; const job = jobs.get(id); job.state = 'cancelled'; return structuredClone(job); },
  readArtifact: async () => {
    if (current.state !== 'active') throw { code: 'expired', message: 'Access revoked', retryable: false };
    return new Uint8Array([1, 2, 3]);
  },
};
const client = new ActionClient(fake);
const requested = await client.request({ target: grant.target, permissions: grant.permissions, reason: 'Diagnose a screen' });
assert.equal(requested.state, 'pending');
assert.equal((await client.grants())[0].id, grant.id);
current.state = 'pending';
const pending = await client.session(grant.id);
await assert.rejects(async () => pending.actions(), { code: 'permission_denied' });
current.state = 'active';
const session = await client.session(grant.id), desktop = new RemoteDesktopActions(session);
session.grant.permissions.push('remote.desktop.control');
await assert.rejects(async () => desktop.input(frame, [{ kind: 'text', text: 'hello' }], 'forbidden'), { code: 'permission_denied' });
assert.equal(seen.length, 0, 'client-side permission guard must not dispatch');
const observe = await desktop.observe('observe-once');
const updates = [];
assert.deepEqual(await observe.wait({ interval_ms: 10, onUpdate: j => updates.push(j.state) }), frame);
assert.deepEqual(updates, ['queued', 'succeeded']);
await desktop.observe('observe-once');
assert.equal(executionCount, 1, 'explicit retry keeps key and recovers one job');
await assert.rejects(() => desktop.observe('observe-once', { max_width: 640 }), { code: 'conflict' });
assert.deepEqual(await desktop.read(frame), new Uint8Array([1, 2, 3]));
current.state = 'revoked';
await assert.rejects(() => desktop.read(frame), { code: 'expired' }, 'cached grant never replaces server authorization');
await session.refresh();
await assert.rejects(async () => desktop.observe('revoked'), { code: 'permission_denied' });
current = { ...structuredClone(grant), expires_at: '2000-01-01T00:00:00Z' };
const expired = await client.session(grant.id);
await assert.rejects(async () => expired.actions(), { code: 'grant_expired' });
current = { ...structuredClone(grant), permissions: ['remote.desktop.view', 'remote.desktop.control'] };
const controlled = new RemoteDesktopActions(await client.session(grant.id));
const sent = await controlled.input(frame, desktopClick(frame, 720, 450), 'click-once');
assert.equal(seen.at(-1).args.frame_id, frame.frame_id);
assert.equal(seen.at(-1).args.events.length, 2);
const ctl = new AbortController(); ctl.abort();
await assert.rejects(() => sent.wait({ signal: ctl.signal }), { code: 'cancelled' });
assert.equal(cancelled, 0, 'stopping an observer must not claim that input was undone');
await sent.cancel();
await assert.rejects(() => sent.wait(), { code: 'cancelled' });
assert.equal(cancelled, 1);
const failed = await controlled.observe('partial');
pollHook = id => ({ ...jobs.get(id), state: 'failed', error: { code: 'outcome_unknown', message: 'Native acknowledgement lost', retryable: false, applied: 1 } });
await assert.rejects(() => failed.wait({ interval_ms: 10 }), e => e instanceof ActionError && e.code === 'outcome_unknown' && e.applied === 1 && !e.retryable);
const wrong = await controlled.observe('wrong-job');
pollHook = id => ({ ...jobs.get(id), id: 'other-job' });
await assert.rejects(() => wrong.wait({ interval_ms: 10 }), { code: 'invalid_response' });
pollHook = id => ({ ...jobs.get(id), version: 2 });
await assert.rejects(() => wrong.refresh(), { code: 'invalid_response' });
pollHook = undefined;
const raced = await controlled.observe('cancel-race');
let finishPoll;
pollHook = id => new Promise(resolve => { const snapshot = structuredClone(jobs.get(id)); finishPoll = () => resolve({ ...snapshot, state: 'running' }); });
const oldPoll = raced.refresh();
await raced.cancel();
finishPoll(); await oldPoll;
assert.equal(raced.job.state, 'cancelled', 'late polling response cannot reopen a cancelled job');
pollHook = undefined;
await controlled.end();
await assert.rejects(async () => controlled.observe('ended'), { code: 'permission_denied' });
console.log('PASS SDK lifecycle: pending/expiry/revoke/end, permission isolation, dedup keys, native failure, cancellation, job binding');

validateDesktopInput([{ kind: 'text', text: '你好🙂' }, { kind: 'key', code: 'Enter', down: true }, { kind: 'key', code: 'Enter', down: false }]);
for (const events of [[], Array(65).fill({ kind: 'release' }), [{ kind: 'move', x: NaN, y: .5 }], [{ kind: 'button', x: .5, y: .5, button: 0, down: true, clicks: 4 }], [{ kind: 'text', text: 'valid' }, { kind: 'text', text: 'bad\n' }], [{ kind: 'text', text: '\ud800' }], [{ kind: 'text', text: '🙂'.repeat(257) }], [{ kind: 'key', code: 'arbitrary', down: true }], [{ kind: 'key', code: 'Enter', down: 1 }], [{ kind: 'scroll', delta: Infinity }], [{ kind: 'exec', shell: 'ignored' }]]) assert.throws(() => validateDesktopInput(events), { code: 'invalid_argument' });
assert.throws(() => desktopPoint(frame, 1440, 0), { code: 'invalid_argument' });
assert.deepEqual(desktopPoint(frame, 720, 450), { x: .5, y: .5 });
assert.throws(() => desktopPoint({ width: 0, height: 0 }, 0, 0), { code: 'invalid_argument' });
assert.throws(() => controlled.observe('bad-size', { max_width: 99999 }), { code: 'invalid_argument' });
console.log('PASS remote adapter: image coordinates, complete-batch validation, Unicode, text/key/scroll/batch limits');

const requests = [];
let reply = () => Response.json(grant);
const http = new ActionHTTPTransport(async (path, init) => { requests.push({ path, ...init }); return reply(path, init); });
await http.requestGrant({ target: grant.target, permissions: grant.permissions, reason: 'Check' });
assert.equal(requests.at(-1).path, '/v1/me/action-grants');
assert.equal(requests.at(-1).method, 'POST');
assert.equal(JSON.parse(requests.at(-1).body).reason, 'Check');
reply = () => Response.json({ actions: [descriptor('observe')] });
assert.equal((await http.listActions('g/a')).length, 1);
assert.equal(requests.at(-1).path, '/v1/me/action-grants/g%2Fa/actions');
reply = () => Response.json({ state: 'running' });
await http.getJob('j/1');
assert.equal(requests.at(-1).path, '/v1/me/action-jobs/j%2F1?wait=30s');
await http.getJob('j/1', { wait_seconds: 2 });
assert.equal(requests.at(-1).path, '/v1/me/action-jobs/j%2F1?wait_seconds=2');
await http.getGrant('g/1', { wait_seconds: 30 });
assert.equal(requests.at(-1).path, '/v1/me/action-grants/g%2F1?wait_seconds=30');
await assert.rejects(async () => http.getGrant('g', { wait_seconds: 61 }), { code: 'invalid_argument' });
reply = () => Response.json({ grants: [grant] });
assert.equal((await http.listGrants())[0].id, grant.id);
reply = () => Response.json({ error: 'unsupported', message: 'Old component', retryable: false }, { status: 400 });
await assert.rejects(() => http.getGrant('g'), { code: 'unsupported', retryable: false });
reply = () => new Response('invalid JSON');
await assert.rejects(() => http.getGrant('g'), { code: 'invalid_response' });
reply = () => Response.json(null, { status: 403 });
await assert.rejects(() => http.getGrant('g'), { code: 'permission_denied' });
reply = () => new Response(new Uint8Array([1, 2, 3]));
assert.deepEqual(await http.readArtifact('g', 'a'), new Uint8Array([1, 2, 3]));
reply = () => new Response(new Uint8Array(4 * 1024 * 1024 + 1));
await assert.rejects(() => http.readArtifact('g', 'large'), { code: 'too_large' });
reply = () => Response.json({ error: 'not_found', message: 'Grant ended', retryable: false }, { status: 410 });
await assert.rejects(() => http.readArtifact('g', 'ended'), { code: 'not_found', status: 410 });
let attempts = 0;
const disconnected = new ActionHTTPTransport(async () => { attempts++; throw new Error('Connection lost'); });
await assert.rejects(() => disconnected.invoke({ grant_id: 'g', action: 'remote.desktop.input', version: 1, args: {}, idempotency_key: 'once' }), { code: 'outcome_unknown', retryable: false });
assert.equal(attempts, 1, 'never automatically retry mutations');
const cycle = {}; cycle.self = cycle;
await assert.rejects(() => disconnected.invoke({ grant_id: 'g', action: 'example.action', version: 1, args: cycle, idempotency_key: 'bad-json' }), { code: 'invalid_argument' });
assert.equal(attempts, 1, 'invalid JSON must fail before dispatch');
console.log('PASS HTTP adapter: frozen routes/envelopes, encoded IDs, long poll, errors, private artifact size/expiry, no input retry');
