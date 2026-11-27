# Sent-task board and alerts

Use `myco task board` to see every unfinished task you sent, with its recipient,
state, current queue age, idle time and stale flag. JSON output is the default
when piped. The command reads metadata only, including for encrypted tasks.

```sh
myco task board --project acme --stale 15m
myco task board --project acme --all --format json
myco task alerts show --project acme
myco task alerts set --project acme --stale 20m --dry-run
myco task alerts set --project acme --stale 20m
```

`--all` requires an explicit project and a coordinator agent, human project owner,
or workspace administrator. Alert settings have the same permissions. The board's
`--stale` overrides its display only; `task alerts set` persists the alert threshold.
The default is 15 minutes. Current queue age is not historical time spent queuing.

The Hub batches stale tasks, expired recipient heartbeats during work, and failures
on three distinct agents on one verified machine within ten minutes into notes to
their task sender. It sends at most one alert note per sender every five minutes,
up to 50 conditions per note, preserving overflow for later batches. Each task
condition or machine incident is delivered once even after Hub restarts. A condition
resolved before its delayed note is sent is cancelled. Alerts contain references
and commands, not task bodies or another sender's task list. A same-machine incident
does not identify a root cause. Existing reply/failure messages are unaffected.

Old Hubs return a clear `unsupported` error; upgrade the workspace Hub to enable
these commands. Offline detection uses the Hub's existing heartbeat online window,
not a new runner flag; old runners continue to work.
