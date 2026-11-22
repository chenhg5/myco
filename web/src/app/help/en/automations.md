# Automations

Automations have an agent work on a schedule, or when another system calls in.

## Making one

Automations › New automation. Start from a template (daily stand-up, weekly report, watch CI…) or from blank:

1. **Name**.
2. **Send to**: which agent does it.
3. **What to do**: a few lines, as you would say it in a chat.
4. **When**:
   - **Schedule**: every day, weekdays, some days of the week, every few hours, or a custom cron, in this computer's time zone.
   - **Webhook**: when CI, monitoring or another system calls an address; what it sends goes into the task as `{{payload}}`. Saving shows the address, a secret and a curl example.

"Skip it when the last one is not done yet" is on by default, so work does not pile up.

## Its runs

Open an automation for every run: whether it was sent, which task, how it went. "Run now" at the top right runs it once; ⋯ pauses, turns on or deletes it.
