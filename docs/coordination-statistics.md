# Agent selection and task statistics

These commands are read-only and JSON-friendly. They require Hub support for
coordination statistics version 1. An older Hub returns `unsupported`, with an
upgrade suggestion. No runner upgrade is needed to record new acknowledgements.

```
myco agent pick --skill code --cli codex --count 3 --strict
myco agent stats --name worker --project demo --since 7d
myco stats coordinator --name coordinator --since 7d
```

`--since` is a positive duration (Go duration syntax, plus `d` for 24 hours),
with a default of 7 days and a maximum of 366 days. Times are UTC. Tasks are in
the cohort when the Hub accepted them within `[since, until]`; outcomes are their
current state when queried. A late reply contributes only once it exists. A task
received before the window does not enter it merely because it finished later.

## Read boundary

The Hub computes the results from task metadata; it does not select message bodies
or return bodies, errors, trace payloads, account identities or credentials. The
response says `scope: all` for Hub managers, or `participant_messages` otherwise.
An agent's history includes only tasks it sent or received; a member's includes
only tasks involving their own console/agents. A visible public agent profile
does **not** grant access to its unrelated task history. Rankings use the same
restricted historical sample. They can therefore differ between callers.

`--project` checks access to the project, filters its messages, and limits agents
to its current members. It does not expand message permissions. `--name` accepts
an agent name or alias. Without it, agent stats lists all visible agents;
coordinator stats defaults to the calling agent (admin/member tokens need a name).
Historical labels are not recorded: task types are the sorted, deduplicated current
capability and label tags, not a classification of task bodies or their frequency.
A history exceeding 100,000 matching tasks fails with a suggestion to narrow the
window/project instead of silently returning incomplete aggregates.

## Agent statistics

- `tasks`: all task messages in the cohort, including unfinished tasks.
- `done`: status `done` (including tasks closed without a reply).
- `stopped`: failed tasks bearing the Hub's sender-stop marker, or its App-stop
  prefix. `failed` counts other failed tasks. These outcomes are disjoint.
- `success_rate`: `done / (done + failed + stopped)`; unfinished tasks excluded.
- `receipt_to_reply`: seconds from Hub acceptance (`created_at`) to the earliest
  formal `reply` for that task. Failure events/progress/steers are not replies.
  This includes queueing, active work and holds, not just model execution.
- `queue_wait`: seconds from Hub acceptance to the first ACK. This includes runner
  slot waiting before ACK; it excludes later holds, releases and restart delays.
  The first ACK is recorded atomically starting with this Hub version and never
  overwritten by repeat ACKs. Old tasks without this timestamp are unknown; the
  migration does not substitute their last activity/update time.
- Every timing has `samples`, `median_seconds` and `p90_seconds`. Median averages
  the two middle values for an even sample; p90 is nearest rank `ceil(.9*n)`.
  An empty sample has null timings, not zero seconds.
- `redone`: an original task that was stopped or had a steer, followed by a later
  task from the same sender in the same thread, before `until`. A stopped task's
  replacement must follow its stop; a steered task's replacement must follow a
  steer. Each original counts once. `redone_share = redone / tasks`.
  This is an **observable resend heuristic**, not an assessment of work quality:
  a legitimate follow-up can qualify, and a resend in a new thread cannot.

## Coordinator statistics

- `person_tasks`: first console-origin task to this coordinator per thread within
  the cohort. Person-to-reply timings use those tasks' first formal reply.
- `person_followups`: additional task messages or steers from that same console
  sender to the coordinator, in the same thread, after the first task and before
  its reply (or `until` if unanswered). Steers answering an explicit question
  (`answers` set) are excluded. This counts follow-up events, not whether a person
  was impatient; another console/device is not assumed to be the same person.
- `delegated_tasks`: cohort tasks sent by the coordinator.
- `delegated_queued_share`: `sum(first_ack - accepted) / sum(end - accepted)` for
  delegated tasks with a known first ACK and an end. End is the first reply, or
  terminal update time for a task closed without a reply. It is a duration-weighted
  share, not the mean of task percentages. `queue_samples` reports coverage;
  unfinished/unstarted/legacy tasks are excluded and no positive denominator
  means null. Overlapping tasks are summed, not treated as wall-clock critical path.
- `reassigned`: sender-stopped delegated tasks followed by another task from the
  coordinator to a **different** agent in the same thread. Each original counts
  once. The response's `reassignment_method` identifies this historical inference;
  it cannot prove that the replacement has the same intent.
- `withdrawn`: remaining stopped delegated tasks. A replacement in another thread
  is not correlated. These are event-derived counts, not semantic classifications.

## Selection

`--skill` matches a current skill name, capability or exact label. `--cli` matches
runtime CLI, falling back to the binding harness for older runners. `--machine`
matches machine ID/name or binding environment. Profiles hidden from the caller,
and agents that do not accept tasks from the calling agent, are not recommended.
Selection never sends a task or reserves a slot.

Available agents sort ahead of those offline, away/updating, quota-blocked or
having at least two queued/delivered messages. `--strict` excludes that latter
group. Busy agents remain eligible but rank below otherwise equal idle agents.
Unknown quota/capacity stays unknown; an old runner is not assumed full.

Within the availability tier the score is:

```
100 - 20*queued - 15*(in_progress + held + stale)
+ 10 if a requested skill matched
+ 2*min(free_runner_slots, 4), or -20 if known slots are full
- 5 if quota is unknown
+ 20*(success_rate - .5) if there are terminal samples
- min(10, log(1 + median_receipt_to_reply_seconds)/2) if known
```

Ties sort by permanent agent name. Higher scores rank first. The one-line reason
includes availability inputs and any observed success/duration evidence; the
candidate also carries its historical sample sizes. Default count is 3, maximum
100. A score is a scheduling hint, not a claim that unlike tasks have equal cost.

Slot reports older than 90 seconds and quota reports older than 15 minutes are
unknown. Expired quota windows do not block work; `allowed_warning` is available.
Explicit usage rejection or an unexpired full usage window without an explicit
provider allowance blocks. Collector
authentication errors, missing collector credentials, quota-query throttling, and
unsupported/schema/network quota checks remain unknown: a failed quota query is
not evidence that the CLI cannot work. Account quota
visibility is enforced **before** scoring: someone else's private quota cannot
affect the reason, score or strict filtering. Shared account snapshots are used
where authorized. Slots are shared across a runner process, so free capacity is
an observation, not a promise that each candidate can reserve it independently.
