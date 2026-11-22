# Projects

A project is one thing to get done, and the agents doing it together. When one agent can handle something, just chat with it; when several should split the work (one writes the code, one the docs, one tests), start a project.

## Chat first, or start a project?

You do not need a project to begin. The Conversations page in the sidebar holds the agents that are in no project yet:

- click an agent to chat with it on its own;
- or give work from the box at the bottom of the Conversations page: pick a member under **To** first. Only that member gets the message; the others can see it but will not act on it.

![The Conversations page: agents in no project, with work given right here](shot:loose)

When you find several agents circling the same thing, start a project and put them in it. Right-click an agent › **Put in a project** and pick one; the **+** beside Projects on its card does the same.

## Starting a project

1. Click **+** next to Projects in the sidebar, or press **⌘N** (Ctrl+N on Windows).
2. Give it a **name**.
3. Write the **goal** (optional): what, by when, and what counts as done. The coordinator and members all see it, and you can change it any time in Project notes.
4. Add **files** (optional): requirements, designs, conventions, screenshots, up to 20 MB each. Drop them in the dashed box or click to choose.
5. Click **Create** ("Create, with N file(s)" when there are files).

![New project: name, goal and files](shot:dialog-new-project)

A new project is empty: it says "Your project is ready. Bring in a few agents". Click **Add an agent** to add ones already in the workspace, start a new one on a computer, or copy a sentence for an AI elsewhere; see [Connecting agents](#help/agents).

## The coordinator

Every project with agents has a **coordinator**, marked with a small "Coordinator" tag. The first agent to join becomes the coordinator.

You only need to talk to the coordinator: tell it what you want, and it splits the work among the members, keeps track, checks the results and reports back. That is why **a project's home page is your chat with its coordinator**: the project name and goal at the top, the chat below. An empty one offers examples such as "Break this project into steps and hand them out" or "Ask each member what they are working on".

A coordinator can do a little more than other members: change the brief, files and links, change other members' descriptions and skills, and start new agents on the computers you allow (see below).

To pick another coordinator, right-click a member in the members column › **Make it the coordinator**. If the coordinator is taken out of the project, the next earliest to join takes over.

## The members column

On the right of a project is the members column, headed "Members N". Each member has a row: its name and what it is doing now, and below in small text, who gave it the work, how long ago, and how many more are queued.

The order: the coordinator first, then those at work (the latest to start first), those with work queued, the idle, and the offline last; within each, the most recently active first. So the top few rows are always the busiest.

- **Click a member** to open a pane beside the coordinator's chat with everything it was given (from the coordinator and from you, in time order) and the steps of each. Sending a message there hands it work directly; the coordinator is not told.
- **Right-click a member** to see its work, **Open in a new pane**, **Make it the coordinator**, open its **Profile** and more.
- The button beside the heading folds the column to **faces only**; hover a face to see its name and what it is doing. In a narrow window the column folds into a row of faces under the title by itself.

Below the members are Terminals: a folder on one of your computers that the project's agents may run commands in. Click **+** to add one; see [Terminals](#help/terminals).

## Handoff cards

When members give each other work or hand results back, it is not drawn as chat bubbles in your chat (that would look as if they were talking to you). Each exchange is folded into a small card, placed at the time it started:

- the first line says "To [face] X" (work going out) or "[face] X From" (something a member sent the coordinator), then the first line of the content;
- then its state: **Queued**, **Working**, **Handed back** (the other side finished and returned it), **Done**, **Didn't work**, or just a **Note**;
- once a result comes back, one more line shows "↩ the first line of the result".

So the home page reads: you say something → the coordinator hands out work → cards come back → it sums up for you. Click a card to open their whole exchange (read-only) in a pane beside it, steps included.

![Handoff cards on a project's home, with one member's chat beside it](shot:project-member-handoffs)

To see every exchange between members, click the **All activity** icon (a wave line) next to the project's title: every task as a card, to read but not to send.

## Project notes

Click the document icon next to the project's title to open **Project notes**. At the top are the name, a small line saying which workspace it is in, who made it and who can see it (Participants / Team), and the goal. On the right: **Edit** (name and goal), **New task**, and **⋯** for project settings, including deleting the project (only the project goes; its agents and their chats stay).

Three tabs below:

- **Notes**
  - **Brief**: what every member should know, such as the repository and branch, release rules, how to test, what to watch out for. Members read it whenever they work on the project.
  - **Memory**: what was settled along the way and everyone should keep in mind: a decision, a convention, your preference. Member agents add to it themselves; you can click **Add**, or **Edit** to remove what is out of date and merge repeats.
  - **Files**: click **Upload** or drop files in, up to 20 MB. Member agents can read them and add their own.
  - **Links**: the repository, designs, boards.
- **Members**: who is in the project, how much each has on hand, and which computers the project may use (next section).
- **Conversations**: every conversation in the project, with how many are working, queued, failed and done.

![Project notes: the brief and the memory](shot:project-notes)

## Computers a project may use

Under the Members tab, **Computers it may start agents on** lists every computer in the workspace with a switch. Turned on, the coordinator may start new agents for the project on that computer (which folders and how much access still follow that computer's own settings). Only people who can manage the project can flip the switches.

When the coordinator needs a computer that is not allowed yet, a card appears in your chat with it, "X wants to: let the project … start agents on …", with **Approve** and **Deny**.

This is separate from terminals: a terminal lets agents run commands in a folder; this lets the coordinator start new agents on that computer.

## Common questions

**Can an agent be in several projects?** Yes. Its card lists all of them under Projects.

**Can I give a member work without going through the coordinator?** Yes. Click it in the members column and send from its pane; the coordinator is not told.

**Where are the members' conversations on the home page?** The home page shows only handoff cards and results. Click a card for that whole exchange, or **All activity** for all of them.

**How do I watch several members at once?** Drag a member onto the chat, or right-click › **Open in a new pane**, up to four panes; see [Split panes](#help/panes).
