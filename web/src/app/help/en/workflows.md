# Workflows

A workflow turns something you do often into fixed steps: each step goes to an agent, to a computer, or stops to wait for your decision, and the workspace walks through them in order. Think "one writes, another reviews, back for changes up to three rounds, and you have the last word"; when you notice yourself giving the same instructions again and again, make it a workflow.

## The Workflows page

Click **Workflows** at the top of the sidebar. When something waits for your decision, an amber number shows next to it.

- **Waiting for you**: at the very top, one card per decision; see "Decisions waiting for you" below.
- **Overview**: cumulative runs, currently in progress (including decisions), successes and failures across all listed workflows and all versions. Cancelled runs are shown separately. Counts refresh automatically; older workspaces show a dash for unavailable totals.
- **Your workflows**: search the name, description or project; choose a project and filter by In progress, Awaiting decision, Last run failed, Never run or Off. Chip counts are workflows matching your search and project, not run counts. Sort by recent run, in progress first or name. Clear filters if nothing matches.
- Each row shows current activity independently of the latest run, even when the workflow is off or a newer run has already finished. Below are cumulative run, success, failure and cancellation counts; open it to inspect individual runs.
- Each row has **Run** and **⋯** (Edit, Turn off / Turn on, Delete) on the right; right-clicking gives the same menu.

Workflows run on the workspace too, so they keep going while your computer is off; each step's agent or computer just needs to be online to take its step.

![The Workflows page: decisions waiting for you on top, your workflows below](shot:workflows)

## Making one

Click **New workflow** at the top right. There are two ways.

### A ready-made one

Pick one, then choose who does the work and on which computer:

- **Write, review, revise**: one agent does it, another reviews; back with the comments if it does not pass, up to 3 rounds; you have the last word.
- **Fix until the tests pass**: an agent changes it, a computer runs the tests; on failure the error goes back, up to 3 times.
- **Split, then merge**: one agent splits the work in two, two agents work at once, then it is merged into one.
- **Ask before shipping**: an agent lists what goes out; the release command runs on a computer only after you say "Ship"; no answer in a day, nothing ships.

Once you have chosen, the steps are drawn right below. If they look right, click **Save workflow**.

![The ready-made "Fix until the tests pass": who changes it, which computer runs the tests, and the steps drawn below](shot:workflow-make)

### Have an agent write one

If none of those fit, choose **Have an agent write one**:

1. Pick an agent.
2. Under "How it goes", say in a sentence what the steps are, who does each, and when you want to decide. For example: "For each new request, Claude writes a plan and Codex reviews it; once approved Claude builds it, runs the tests on Studio Mac, and if they pass asks me whether to merge."
3. Click **Ask it**. This goes to the agent as a task, and you land in your chat with it. It writes the workflow, checks it and saves it, and it appears on the Workflows page.

To add or remove steps later, ask the agent that wrote it, too.

## Opening a workflow

Click a row to open it. Under the title: On / Off, how many steps, and which version; at the top right, **Edit**, **Run** and **⋯**. Four tabs follow:

- **Runs**: one row per run (opens here once it has run). **All / Going / Failed / Done** above the list filter it.
- **How it goes**: how many runs, the success rate, the time (median, and the slow end), the cost per run, a bar chart of recent run times, and per-step numbers (runs, failures, retries and send-backs, time, cost). The control at the top right shows one version only.
- **Steps**: the drawing (a new workflow opens here). Cards go from top to bottom, joined by arrows; a dashed line going back up says how many times it may loop; a label on a line says when it is taken ("approved", "not approved · up to 3×", "chose Keep it"); small red text left of a step says when the whole run fails ("fails after 3").
- **Inputs and outputs**: first "What a run is given", then one block per step, with what it gets on the left and what it hands back on the right: each field's name, type, whether it is required, and a description.

## Running it

1. Click **Run**.
2. Fill in what this workflow asks for, such as "What to change"; optional ones can stay empty.
3. Click **Start**. The first step goes out at once.

Tasks sent by a workflow carry a "Workflow" tag in that agent's chat, alongside its other tasks.

An [automation](#help/automations) can also start a workflow on a schedule or when a webhook is called.

## A run's page

Click a run in Runs to open it. The title is what the run was about; below it come the status first (Running, Waiting for you, Failed, Done), then the workflow, when it started, how long it took, who started it, and the version.

- **Numbers**: the total time, how much of it was work and how much waiting for a decision, how many steps it took, and the cost.
- **Each step**: one row per step, starting with a round mark: a green tick when done, a red cross when failed, an amber dot when waiting for you, a green dot in progress. It says who did it, the line it handed back or why it failed, and how long it took.
  - **Details** expands the task it got, its input, and the result it handed back; a result that fits the step's contract has a green "Fits its contract" beside it.
  - **Log** shows what it did, tool by tool, with its thinking.
  - **Chat** jumps to that task in its conversation.
  - Switch between **List** and **Timeline** at the top right.
- **The drawing** sits below, marking each step's status and how many times it ran (×2). Click a step in it, and Each step above shows only that step's attempts.
- **What it produced**: the pictures and files agents handed back, and the final result.
- **Scores**: click **Score it** to note how the run went (quality, tests passed, lines changed…) and whether higher or lower is better. Scores with the same name are averaged in How it goes and compared side by side.

At the top right: **Cancel run** stops one still running; **Retry from there** restarts a failed or cancelled one at the step where it stopped, keeping what was done before. **Compare** puts this run next to another: total time, cost, each step's time and task count, and the scores, with the differences worked out; it warns you when the two used different versions.

![A run: the numbers, the card waiting for your decision, and each step](shot:workflow-run)

## Decisions waiting for you

A workflow can have a "You decide" step: it stops there and asks you something, such as "Reviewed. Keep it?".

You find it:

- under **Waiting for you** at the top of the Workflows page, and on that run's page;
- as the amber number next to Workflows in the sidebar;
- as a system notification while the Mac or Windows App is open; you can also decide in the phone app.

The card names the workflow, the question, and when it expires. Pick an option (such as **Keep it** or **Drop it**), add a sentence under "A word to go with it (optional)" if you like, and the workflow carries on. If it expires with no choice, that counts as not doing it.

How long before it expires can be changed when editing that step (None, or 30 minutes up to 3 days). Who decides is set when the workflow is written; a common choice is "Whoever starts it decides".

## Editing

Click **Edit** for the editing page. The drawing is on the left, with a row of small tags above it: "Basics / 1. Write / 2. Review / …". Click a step in the drawing or a tag, and edit it on the right.

- **Basics**: a one-sentence description, "What a run is given", and how many times each loop may go back.
- **A step**: its name; **Who does it** (an agent, a computer, or "Whoever is free with a role…" plus a role such as review); **Timeout**; how many **Retries** after a failure; **What it is asked**; **What it hands back**; and the optional "Its input must fit".
- "What it is asked" can refer to other things: `{{inputs.task}}` is what the run was given, `{{steps.review.summary}}` is what an earlier step handed back; the workspace fills them in before sending.
- "What it hands back" has one row per field: name, type (text, number, yes / no, list…), required or not, and a description; **Add a field** adds another. This is the step's contract: a result that does not fit is sent back. For rules the form cannot show, use **Edit as JSON**.

Click **Save a new version** when done. It is saved as the next version, and runs already going keep the version they started with. If you leave with unsaved changes, you are asked "Leave without saving?" first. The name and adding or removing steps are not changed here; ask the agent that wrote it.

![Editing: click a step on the left, change who does it, what it is asked and what it hands back on the right](shot:workflow-edit)

## Turning off and deleting

- **Turn off**: ⋯ › Turn off. It can no longer be run, and its history stays; **Turn on** brings it back.
- **Delete**: ⋯ › Delete. Its runs go too, and ones still going are stopped first; the tasks it sent and the agents' answers stay.

## Questions

**A step does not move.** Check who it went to: when that agent or computer is away, the task waits. **Chat** shows how it is doing.

**A result was sent back.** What came back did not fit the step's "What it hands back" contract, and the reason says what is missing (for example, approved is missing). Change what the step asks for, or loosen the contract.
