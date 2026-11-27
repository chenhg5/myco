# Isolated Hub load testing

`myco-loadtest` starts a fresh local Hub child process and drives real HTTP
requests against it. It accepts no target URL, reads no myco configuration, and
never connects to an existing Hub. Use it to reproduce bottlenecks and compare
changes, not to infer production capacity from a development computer.

Build and run from this repository:

```sh
go build -o bin/myco-loadtest ./cmd/myco-loadtest
bin/myco-loadtest --out /path/to/new-run --users 1,5,20 --repeat 3
```

The parent directory must exist; `new-run` must not exist. The tool keeps all
artifacts and never removes or reuses an existing directory. Choose a volume
with enough space for the fixtures plus at least 5 GiB and 20% free space. Do not
place the results in a public checkout: the run database contains temporary
credential hashes and synthetic data, and host measurements can be private.

The default scenario has two devices and ten agents per person. One device per
person refreshes the overview and recent lists every five seconds. Every agent
maintains a 30-second inbox long poll and reports runtime every 30 seconds. The
20% active agents each receive one task per minute. Their simulated work lasts
five seconds; tasks go through send, pull, ack, progress, optional trace upload,
reply and a check of the stored task, reply count, and sender-visible trace.
No model, agent CLI, remote command, webhook or external notification is called.

Users have separate member/device credentials; agents are enrolled through their
owner's credentials. The first user owns the Hub; the others are ordinary
members. Fixtures include team and private agents, a shared project and private
projects. Initial and final probes verify device ownership and, when there are
at least three users, cross-member private-resource denial. The recent-list
workload rotates through thread, node/pair, project and ownership filters.
The recent-operation suffixes identify those variants: `[0]` ungrouped threads,
`[1]` pairs involving the caller, `[2]` other pairs, `[3]` the shared project,
`[4]` the caller's private project and `[5]` the ownership filter.

Each user tier and repetition gets its own Hub process and database. The tool
creates identities through the real API, stops the Hub, seeds closed historical
notes, including agent-to-agent conversations, through the SQLite repository, checks integrity and foreign keys, then
restarts it for measurement. Fixtures contain short and long threads and a mix
of 256-byte, 4-KiB, 32-KiB and 128-KiB bodies. Body sizes are spread across
parties independently; large private-project bodies and nonempty node pairs are
part of the fixture checks. The default is 1,000 historical
messages; set `--seed-messages 100000` to expose history-sensitive query costs.
The database is never seeded while its Hub is running.

## Timing and acceptance

Defaults are two minutes of warmup, ten minutes of measured arrivals and up to
two minutes to drain admitted tasks. Each tier repeats three times. `--help`
lists the bounded duration, arrival interval, queue and worker options. For a
short functional check, for example:

```sh
bin/myco-loadtest --out /path/to/new-smoke --users 1 --repeat 1 \
  --seed-messages 20 --warmup 1s --duration 5s --cooldown 10s \
  --task-every 2s --task-work 100ms --app-every 1s --runtime-every 2s
```

Short checks validate the tool, not capacity. Formal measurements should use the
normal optimized build, not a race-instrumented test binary. Do not run builds or
other heavy tests on the same computer during measurements.

Scheduled short operations and task arrivals use a fixed clock with seeded
initial offsets. Slower responses do not reduce the intended arrival rate.
Workers and the queue are bounded: full-queue drops, scheduling lag, attempted
requests, errors and unfinished tasks remain visible in the report. Task steps
respect their dependencies, and each agent processes one task at a time. The
report distinguishes warmup, steady-state arrivals and draining. A task's phase
is determined by its planned arrival, even if it completes during draining.

The initial acceptance thresholds are comparison targets, not an SLA:

- Short list/overview requests: p95 at most 500 ms, p99 at most 1.5 s.
- Other short requests: p95 at most 300 ms, p99 at most 1 s.
- Scheduled task arrival to receipt: p95 at most 1 s; simulated work is measured
  separately in the scheduled-to-validated task latency.
- Scheduling lag: p95 at most 100 ms. A saturated generator cannot establish a
  Hub capacity limit.
- No unexpected request failures, dropped operations, unfinished/failed tasks,
  duplicate claims, incorrect replies or failed permission probes.

Percentiles are bounded logarithmic-histogram bucket upper bounds at roughly
10% resolution; maxima are exact. Counts are always included. The automatic SLO
check requires at least 20 samples for p95 and 100 for p99; fewer samples are
reported but not presented as a reliable tail estimate.

Normal long-poll waits are a separate operation, excluded from short-request
latency acceptance. Canceled polls during shutdown are counted separately.
A sustained error/latency or client-saturation interval stops new arrivals after
30 seconds; low disk headroom or Hub RSS over 2 GiB also stops arrivals. Existing
work is given a bounded drain period. A rejected run returns a nonzero exit code
and preserves its partial report. Ctrl-C cancels requests and stops the owned
child; parent pipe EOF also shuts the child down.

## Artifacts and interpretation

- `manifest.json`: configuration, source/build settings when available, platform,
  CPU count and known limitations. A dirty build is identified by Go's VCS data.
- `report.json`: all completed tier reports, task accounting, route operations,
  scheduling lag, errors, SLO failures, database sizes and permission checks.
- `users-NNN-run-NN/report.json`: the individual report, including partial runs.
- `progress.json`: current phase and cumulative counters, updated every five
  seconds during a tier. The final `unfinished` count is computed at run end;
  current outstanding tasks are `scheduled - completed - failed`.
- `samples.jsonl`: UTC-stamped client interval histograms, server perf snapshots,
  active polls, database/WAL size, and Linux process/system/disk samples.
- `hub.log`: structured server logs, including SQL/COMMIT and request warnings.

The Hub runs as a separate child, but the generator shares its host. Linux
samples distinguish both processes' CPU and RSS, and include file descriptors,
logical/physical I/O, CPU steal/iowait and disk counters when the mounted device
can be identified. Resource sampling runs independently of HTTP metrics polling,
so a stalled perf call does not suppress the resource sample at a write spike.
Other platforms retain HTTP/task/pool metrics but mark host sampling unsupported;
the Linux resource guards are unavailable there.
Unavailable disk measurements are omitted, not reported as healthy zeros.
Process I/O counters are usable only when `io_available` is true. Raw system CPU
counters are retained so steal/iowait percentages can be audited.

`--profile` collects child CPU/heap profiles in the isolated directory. Profiled
runs have different overhead and must be labeled separately.

The server's `database` perf field describes this workspace's primary connection
pool. Wait counters are cumulative since `started_at`; compare differences only
while that value is unchanged. A pending wait increments the count immediately,
but contributes duration only after completion/cancellation. These shared
counters are not per-request percentiles. A final snapshot settles canceled
waits before the child exits. HTTP/SQL histogram windows may span phase boundaries;
use client phase counters and exact timestamps for comparison.

The first version tests one workspace over local HTTP. It does not cover TLS,
reverse proxies, independent load hosts, multiple workspaces, attachments,
workflow/automation bursts, dedicated exec-node fixtures, simultaneous reconnect
storms or a two-hour soak.
It also does not instrument HTTP payload byte counts or generate a production
capacity guarantee. Those scenarios require separate follow-up measurements.
Retain the hardware, seed, traffic schedule, build SHA and limitations with every
result. Compare matched runs, and repeat at least three times before attributing
a change to an optimization.
