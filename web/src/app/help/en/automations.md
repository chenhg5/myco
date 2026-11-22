# Automations

Automations put agents to work on a schedule or when something happens: a summary of issues every morning, a report every Friday, a look at the log whenever CI fails. They run on the workspace, not on the computer in front of you, so they suit work that should happen on time, or as soon as something goes wrong.

## The Automations page

Click **Automations** at the top of the sidebar. Three numbers sit at the top: how many are running, how many are paused, and when the last run was and how it went.

Below, "Your automations" has one row each:

- The first line is its name.
- The second says when and to whom: "Weekdays 09:00 to PM (Codex)", or "On webhook starts Write, review, revise".
- The third says when it runs next ("Next tomorrow 09:00"), "Paused" for a paused one, or "Waiting for a call" for a webhook-only one; then the last result: a green dot for sent, grey for skipped, red for not sent.

On the right of each row are **Run** (send it once now) and **⋯**. Right-clicking a row gives the same menu: Runs, Edit, Run now, Pause / Resume, Delete.

"Start from one of these" always stays at the bottom of the page, so you can make another one from a template at any time.

![The Automations page: your automations on top, templates below](shot:automations)

## Making one

Click **New automation** at the top right, or one of the templates below: Morning summary, Friday report, Regular check, End-of-day notes, When CI fails. A template fills in the name, the text and the time for you to adjust; **Start from blank** fills in nothing.

The editor goes in the order an automation works:

1. **Name**: say, "Morning issue summary".
2. **What it does**: **Send an agent a task**, or **Start a workflow** (see "Starting a workflow" below).
3. **To whom**: pick an agent. The task arrives in your chat with it.
4. **What to ask**: write it as you would in a chat; the more specific, the better. For example, "Look at yesterday's new issues and PRs, pick the ones that matter, and reply with a list."
5. **When**: two cards, **On a schedule** and **Webhook**. Pick one, or both.
6. Click **Create**.

Anything still missing is written left of the button, such as "Say what to ask".

![New automation: name, what it does, to whom, when](shot:automation-editor)

### On a schedule

With **On a schedule** chosen, pick how often:

- **Daily** or **Weekdays**: then the hour and minute.
- **Weekly**: tick the days (more than one is fine), then the time.
- **Hourly**: every 1, 2, 3, 4, 6, 8 or 12 hours, at a minute past the hour.
- **Custom**: a cron expression with 5 fields.

The last line says it in plain words, such as "Weekdays 09:00 · next tomorrow 09:00", and names the time zone: a new automation uses this computer's time zone.

### Webhook

With **Webhook** chosen, an outside system (CI, monitoring, a form…) calls an address to run it once. What the call carries goes into your text:

- Write `{{payload}}` in "What to ask", and the call's whole content goes there; without it, the content goes at the end.
- Click the small `{{payload}}` block in the note to insert it at the end.

After saving, **Webhook address and secret** appears: the address, the secret, and a `curl` command to try it (Try it). **The secret is shown only this once**, so copy it now. The outside system POSTs to the address with the header `Authorization: Bearer <secret>`.

If the secret is lost or leaked, open **Edit** and click **New secret** at the bottom left, then hand out the new one.

The outside system has to be able to reach your workspace. While the workspace is only on this computer and its local network, a service like GitHub cannot call in; see [Open to the internet](#help/public) first.

### When the last one is not finished

"Skip a run while the last task is not finished" is ticked by default: if the agent has not finished the previous one, this run is skipped, so work does not pile up. Untick it to send every time anyway.

An automation's text is kept on the workspace, not end-to-end encrypted.

## Runs

Click an automation to open its page. At the top are its name and status (Running / Paused · when · next), and three buttons on the right: **Edit**, **Run now** and **⋯** (Pause / Resume, Delete).

The **Settings** card lists whom it goes to, when (with the time zone in small type), the webhook address (with copy), whether a run is skipped or sent anyway when the last one is unfinished, and what it asks.

**Runs** has one row per run:

- On the left, the result: Sent; Skipped, with why (say, the last task was not finished); or Not sent.
- A small tag says how it started: On schedule, Webhook, or By hand.
- The time: Today 09:00, Yesterday 18:30, with how long ago below it.
- **Chat**: opens the conversation with the task it sent, at that task.
- **The call** (webhook runs only): expands what the call carried: content type, size, whether it is JSON, the recorded headers, and the body (JSON indented, with copy). Only headers that say where it came from are kept: never the secret, signatures, cookies or the caller's address.

When some runs were skipped or not sent, **All / Sent / Skipped / Not sent** appear above the list as filters. The last 200 runs are kept.

![Runs: one webhook call expanded to show what it carried](shot:automation-call)

A message sent by an automation has a small "Auto" tag next to its time in the chat, so you can tell it was not sent by hand. The agent's answer arrives in your chat with it as usual.

## Starting a workflow

Besides sending one agent a task, an automation can start a [workflow](#help/workflows) on a schedule or on a call:

1. Under **What it does**, choose **Start a workflow**.
2. **Which workflow**: pick one.
3. A row appears for each input that workflow asks for (such as "What to change" or "Test command"); fill in what this run gets.
4. When a webhook starts it, you can use what the call carried: all of it as `{{payload}}`, or one field of its JSON as `{{payload.field}}`, for example `{{payload.issue.title}}`.

In this kind of automation's runs, each run has **The run**, which opens that workflow run directly. Here "the last one" means the last workflow run that has not ended.

![Start a workflow: fields from the webhook call go into the workflow's inputs](shot:automation-editor-workflow)

## Pausing and deleting

- **Pause**: ⋯ › Pause. It no longer sends on schedule, and webhook calls do not start it; **Resume** brings it back.
- **Delete**: ⋯ › Delete, then confirm. It sends nothing more, and its webhook address stops working. What it already sent and the agents' answers stay.

## Questions

**It was time, but the agent did nothing.** Look at Runs. Sent means the message arrived; the agent may be away, and the message waits until it is back. Skipped means the last one was not finished. Not sent says why.

**Does it run while my computer is off?** It sends. The workspace keeps the time and sends the task when it is due, but the agent doing the work needs its computer on to pick it up.

**It says this workspace has no automations yet.** The workspace's myco is too old; upgrade it as the page says.
