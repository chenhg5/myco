# First-word latency

Measure before changing startup, scheduling, or session reuse. Keep failures and
missing boundaries in the report. No timing diagnostic records message content,
credentials, provider settings, or raw CLI output.

## Boundaries

| Source | Milestones | Meaning |
| --- | --- | --- |
| App User Timing (`myco.latency`) | `app_send`, `app_send_ack`, `app_trace_received`, `app_text_dom`, `app_paint_opportunity` | In-memory, bounded to 64 tasks. The send mark uses the original request start, even though the message ID arrives later. Provisional text observations survive a delayed POST response, and are published only when its message ID confirms a fresh send. Historical traces are not new sends. The ACK can arrive after text; it is not an obligatory critical-path boundary. DOM observation and a browser paint opportunity do not prove pixels were visible in an occluded native window. This measures live trace text; progress-only, trace-off, encrypted and borrowed/steered runs need separate correlation. |
| Hub `message latency` log | `received_at_ms`, `enqueued_at_ms`, `service_ms` | Outer HTTP handler entry (before authentication), successful new-message commit, and service completion. Duplicates/dry runs are not new enqueue observations. |
| Runner `runner startup latency` log | `profile_build_start`, `identity_ready`, `initial_provider_ready`, `binding_ready`, `profile_build_end` | Profile setup before inbox polling. An end record also occurs on failure; it does not imply the worker is ready. |
| Runner `run latency` log | `pull_received`, `runner_handle`, `pending_persisted`, `ack_done`, `thread_lock_acquired`, `session_checks_done`, `slot_acquired`, `attachments_ready`, `context_ready`, `settings_fetch_done`, `provider_apply_done`, `settings_hooks_done`, `run_settings_ready`, `request_ready` | Handle stages' `elapsed_ms` uses one local monotonic start. The separate pull record has `pull_wait_ms` (including intentional long-poll wait), not a Handle elapsed time. Queue/lock waits, pending-state persistence, context reads and run/provider settings are measured separately. |
| Harness callback in runner log | `process_started`, `first_cli_event`, `session_announced`, `first_model_event`, `first_text`, `harness_returned` | OS process start, JSON readiness, session metadata, first observable model-origin event, and text reception. A failed run's error text is not a valid successful-response sample. |
| Trace data | `observed_at_ms` | Runner source observation time (Unix ms). Deltas preserve the first piece's observation while coalescing. The outer trace `at` remains the Hub ingestion timestamp. Older runners omit the source timestamp. |
| Runner `trace latency` log | `first_text_upload_start`, `first_text_upload_done` | Local monotonic `buffer_ms` until the first text-bearing upload, and its HTTP `upload_ms`/`succeeded`. A failed upload remains a failed observation; a later text-bearing attempt is logged separately. Summary/off traces do not publish text. |

`thread.started` and Claude's `system/init` are **not** verified history-restore
completion. Do not label their interval as restoration alone. Codex exec may
expose reasoning/text only when an item completes; this is not the first wire
token. The interval to the first observable model event includes network,
provider queuing, inference and potentially unobserved CLI work. Do not subtract
it as if it were an exact measurement of model thinking.

Use local monotonic differences for stage durations. App, Hub and runner wall
clocks can differ: cross-host subtraction requires an independently measured
clock-offset interval and its uncertainty (for example, a bounded request/response
round trip containing a server timestamp). A negative difference is unknown,
not zero. Record intentional queueing, busy sessions, startup errors, retries,
versions, input size, background load and sample count.

The existing runner trace batch period is 700 ms and the App trace poll period
is 1500 ms. These are configured periods, **not** measured delays or guaranteed
upper bounds under load, errors, sleeping windows, or overlapping requests.

Harness records include an `invocation` number. A `harness_returned` record
includes `succeeded`; keep failed attempts and their time, but do not treat their
error text as the first successful response of a later attempt.

## CLI-only baseline (macOS / WSL)

```sh
python3 scripts/latency-eval/harness.py --out /tmp/myco-latency-results.json
```

Runs three fresh conversations per CLI, seeds a separate synthetic 64 KiB
conversation, then resumes that benchmark session three times. It retains the
existing login/provider/model/effort settings, creates only benchmark session
history, and does not change configuration, networking or proxies. Claude tools
are disabled; Codex uses its read-only sandbox and is asked not to use tools.
Tool-using, failed, or timed-out runs are not valid response samples. Raw prompts,
model text, errors, credential values and session IDs are never saved in the
result file. Session IDs are retained only in memory to resume the synthetic
session. The result file is created with mode 0600 and updated atomically after
each completed observation, so cancellation preserves earlier results.

Use `--ephemeral-only` for a fresh-session probe without persistent history;
it cannot measure resume. `--trials 1 --timeout 45 --history-bytes 32768` gives a
bounded pilot. Missing/blocked authentication or session storage is a failed
sample, not fast first-word latency. This script measures **only CLI stages**;
it cannot provide App/Hub/runner timings. Do not infer p95 from three samples.

To inspect App marks locally without message text:

```js
performance.getEntriesByName("myco.latency").map(({ startTime, detail }) => ({ startTime, ...detail }))
```

Marks are session-local diagnostics, not telemetry uploads or persisted settings.
