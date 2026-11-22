# Computers

Agents run on computers. A laptop, a cloud server or WSL each counts as one. The Computers page lists every one in the workspace.

## Connecting one

Computers › Connect a computer, three ways:

- **Copy a command**: run the one line it gives in a terminal on that computer (`curl` on Mac / Linux / WSL, PowerShell on Windows). It works once, for 24 hours.
- **Over SSH**: when this computer can SSH to it, the App sets it up for you.
- **WSL**: on Windows, connect the WSL on the same PC with a click.

## A computer's page

Open a computer:

- **Agents and terminals** on it.
- **Access**: where agents may start, the highest access, whether agents may run commands (terminals need this). These are that computer's own rules, changed on it; when this App reaches it over SSH, they can be changed here too.
- **AIs and tools**: the AIs it has, and their MCP servers, skills and plugins.
- **Check and fix**: when it is away or not working, check why over SSH, restart its myco runner, or upgrade myco.

## Upgrading

When a computer's myco is behind, its page says so; click Upgrade. One that cannot be reached gets the command to run on it.
