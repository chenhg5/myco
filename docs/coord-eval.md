# The coordination test

A fixed set of situations that checks how well a project coordinator (an
agent) splits a person's goal among other agents and puts their results
together. Run it after changing the coordinator's rules, adding a tool, or
changing its model, and compare the scores. The design is in the
`coordinator-tools` project document, section 7.

```bash
make coord-eval CLI=claude
make coord-eval CLI=codex COORD_EVAL_FLAGS="-scenario priority,collision -unit 10s"
```

It drives a real Claude Code or Codex session, so it spends tokens and is
not part of CI. `make test` runs the same scenarios against a scripted
coordinator instead, to test the harness itself.

## What runs

Each scenario starts a real hub in the test process (`internal/coordeval`,
as `internal/integration` does) with:

- **A person** (`tester`, a member at the App) who sends the coordinator a
  goal, and answers any question with "go on as I asked": nothing in the
  test is theirs to decide.
- **The coordinator under test** (`lead`), the coordinator of the `drill`
  project. For a real CLI, `myco runner start` serves its inbox with that
  CLI, and `myco` on the session's PATH acts as `lead` against the drill's
  hub (`MYCO_CONFIG`). Its working folder has a short `CLAUDE.md` /
  `AGENTS.md` saying who it is and how to reach the team; nothing about the
  scenarios. It uses whatever the CLI offers: the task board, alerts, fork
  and the rest, when `myco schema` has them.
- **Fake agents**: scripted workers with a seeded latency, a queue, and
  sometimes trouble: going offline halfway, a plausible wrong answer,
  editing the same file as another, or doing what the person said not to
  when the brief leaves that out. A fake agent recognises the pieces of work
  a brief names (`amber`, `harbor-map`, …) and answers with their results;
  a list without its numbers, or work of another kind, is sent back with a
  question. Names are neutral on purpose.

Times are in **units** (`-unit`, 5 s by default for a real CLI, which thinks
between commands): latencies, deadlines and thresholds. The run of a
scenario ends when every goal is answered, or at its deadline.

Scripted baselines (including the deliberately naive one) use a scenario
clock shared by the Hub, workers, coordinator and checker. It advances only
when all participants are waiting; runnable work and HTTP/SQLite latency do
not consume scenario time. Presence, delayed goals, work and deadlines use
the same clock. Real CLI evaluations keep system time, so their elapsed-time
scores still include command and network costs. A caller context remains an
independent wall-clock watchdog for deadlocks, not a scoring deadline.
`TestBaselineWithSlowTransport` adds delayed HTTP requests and can be run with
`GOMAXPROCS=1 go test -race ./internal/coordeval -count=20` to stress a slow host.

## The scenarios

| Name | Situation | Expected |
|---|---|---|
| `parallel-five` | Five independent lists to add up, three idle sums agents | Sent out in parallel, not all to one |
| `expert-busy` | The only diagrams agent has three tasks queued | Fork it, or tell the person why it waits; not just queue |
| `offline-midway` | A writer goes offline halfway through | Noticed within the threshold, given to another |
| `wrong-result` | One total comes back plausible but wrong | Checked; redone; never passed on |
| `collision` | Two changes to the same `settings.toml` | One after the other (or apart and merged); never both at once |
| `priority` | A long low-priority report runs; an urgent check comes | The urgent one is answered first |
| `constraint` | "Do not touch the legacy module" | In every brief; no work breaks it |

`coordeval -list` prints them.

## Scores

Each scenario scores 0–100 from the hub's records (every message, read from
its database) and what the fake agents did:

- **The result**: the coordinator's answer to the person carries every
  piece's right result and no wrong one; otherwise 0.
- **The person's constraint** (veto): every brief carries it and nothing
  breaks it; otherwise 0.
- **Time**: from the goal to its answer, divided by the critical path (the
  scenario's ideal, everything parallel that can be). Over 1.5×, −10 per
  1× more, at most −30.
- **Questions to the person** with nothing for them to decide (tasks to
  them, progress that asks): −10 each, at most −20.
- **Waste**: a piece given out again, a task stopped, a piece done twice,
  beyond what the scenario calls for: −5 each, at most −20.
- **Noticing trouble** (`offline-midway`): later than the threshold costs up
  to −20; never, −20.
- Scenario rules: all lists to one agent −30; queueing behind a busy agent
  without a word or a fork −40; the low-priority job finished before the
  urgent one −50; two agents editing the file at once fails the result.

## The report

`coordeval` writes `coord-eval-report.json` (each scenario's score and what
made it) and `coord-eval-report.md`, a myco-ui `Table` (see
[rich-messages.md](rich-messages.md)) to post in a conversation:

```bash
go run ./internal/coordeval/cmd/coordeval -cli baseline -unit 50ms
```

Flags: `-cli claude|codex|baseline|naive`, `-scenario a,b`, `-unit`, `-seed`,
`-model`, `-access` (default `workspace`), `-binary`, `-out`, `-keep` (keep
each scenario's hub database and runner files), `-v` (the runner's log).

## Testing the harness

`go test ./internal/coordeval` runs every scenario against `Baseline` (a
scripted coordinator that does what each one expects: it must pass all) and
`Baseline{Naive: true}` (everything to the first agent, unchecked: the
checkers must catch it). `internal/integration` drives the real-CLI path
with a fake CLI: the runner, `myco` on PATH and the scoring, without tokens.
