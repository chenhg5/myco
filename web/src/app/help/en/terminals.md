# Terminals

A terminal is a folder on a computer where agents can run commands; open it and you watch, and you can type commands there yourself. Open one when you want agents to run tests, builds or deployments, or when you just want to run a few commands on a server without logging in to it.

## Where terminals are

- The **Terminals** section of the sidebar lists every terminal in the workspace; click one to open it. A green dot means it is running a command; an amber dot means everything running there may be stuck.
- The "Terminals" list in a project's right column holds the terminals added to that project.
- On each computer's page, "Agents and terminals" lists the terminals on that computer.

With Simple mode on (Settings › Appearance), the sidebar hides the Terminals section, but computer pages and projects still show them.

## Opening a terminal

There are three ways in, all to the same dialog:

- **+** next to Terminals in the sidebar;
- **Terminal** at the top right of a computer's page;
- **+** next to Terminals in a project's right column (the terminal joins that project right away).

Then:

1. **On which computer**: pick a connected computer. One that does not let agents run commands yet is marked "Not allowed yet".
2. **Which folder**: type a folder, such as `~/code/my-project`. Folders used before on that computer are listed below; click one to fill it in.
3. Click **Add**.

It appears under Terminals in the sidebar. If you opened it from a project, every agent in the project can use it at once, including agents that join later; you can even open one before the project has any agents, and the first one to join can use it.

### A computer that is not connected yet

Choose **A computer not connected yet…** and the App gives you one command (one version for Mac / Linux / WSL, one for Windows). On that computer, open a terminal, **go into the folder agents should use first**, and run the line. It also connects the computer to the workspace, and this side ticks itself once it has run. The command works once, for 24 hours.

Running that command is the computer's consent: agents may run commands as you in that folder. So a terminal opened this way does not need the "allow" step below.

### Letting agents run commands

Whether agents may run commands on a computer is up to that computer; the App cannot switch it on from afar:

- The computer you are on: the dialog has **Allow on this computer**; click it and confirm.
- Another computer: run `myco machine shell-access --allow` on it, or click "Allow" in its own myco App. This side updates by itself and **Add** becomes clickable.

Think before you allow it: agents running commands there is just like you sitting at that computer. You can turn it off again in the same place.

![When that computer has not allowed it yet, Access shows the command to run](shot:computer-permission)

## Typing commands

Open a terminal and the whole dark panel is it. The last line is where you type, after the current folder in blue.

- **Enter** runs it; **Shift+Enter** adds a new line, for multi-line commands.
- **↑ / ↓** bring back commands sent in this terminal.
- While a command runs, the bottom line reads "Running · Ctrl+C to stop". With nothing typed, press **Ctrl+C**, or click that line, to stop it.
- Type `clear` or press **Ctrl+L** to clear the screen. Nothing is lost: a line at the top, "Cleared · show what was here", brings it back. The clear is kept on this computer, even after the App restarts.
- Each command is one line, with its output right below. When the previous command failed, the next prompt turns red.
- A long command is folded; click the command to expand it, and again to fold it.
- Hover over a command to see who sent it, when, how long it took and its exit code.

These keys are the same on Mac and Windows: Ctrl, not ⌘.

## How agents use a terminal

Once a terminal is in a project, every agent in the project can run commands there. Tell the coordinator "run the tests and see", and it hands the commands to the terminal itself.

- To see what agents ran, click the terminal in the project's right column, and it opens in a column next to the coordinator. Work an agent hands to the terminal shows as handoff cards in that agent's chat; open one to see the record between them: who ran which command, when, and what it printed. A record is view-only; you cannot type in it.
- A terminal can be in several projects at once. Take it out of a project and that project's agents can no longer use it.

### Commands that may be stuck

When commands that other agents sent to this computer have been quiet for a long time, an amber strip at the top of the terminal says so: "N commands on this computer may be stuck (from …)".

- Click **Show** to list them: the command, who sent it, and how long it has been quiet.
- When you can manage this computer, each has **Stop**, and the strip has **Stop all**.

When a command you sent yourself may be stuck, the command says "May be stuck" right there, with **Stop** and **Resend**.

![A terminal, with the amber strip about commands that may be stuck](shot:computer-stuck)

## Terminal settings

Open a terminal's settings with the settings button in its top bar, **Settings** on its row on the Computers page, or a right-click on it in the sidebar:

- **Name**: give it a name you will recognise, then click **Rename**.
- **Who can run commands here**: one tick per agent in the workspace; click **Save** when done. Tick some and only they can use it; save with none ticked and only you can use it, in its terminal. "Any agent can use it now" above the list means it is still open to every agent.
- **In which projects**: × on a tag takes it out of that project; **Put it in a project…** adds it to another, whose agents can then use it.
- **Open the terminal**: go straight to its panel.
- **Remove this terminal**: removes it after you confirm. Agents can no longer run commands there; nothing in the folder is touched, and conversations are kept.

![Terminal settings: name, who may use it, which projects](shot:computer-settings)

## Safety

- Commands run as the user that runs myco on that computer, so they can do as much as you could at that computer. Only give terminals to agents you trust, and narrow it with "Who can run commands here".
- To give agents just one folder, open a terminal in that folder only; do not hand over your whole home folder.
- To stop all agents from running commands on a computer: for this computer, click **Stop allowing** in the Access tab of its page; on another, run `myco machine shell-access --deny` there.

## Questions

**Add is greyed out.** That computer does not let agents run commands yet; follow "Letting agents run commands" above.

**A command never finishes.** The computer may be away, or the command may be waiting for input (a terminal cannot answer interactive prompts such as a password). Stop it with Ctrl+C and use a form that needs no input.
