# Computers

Every agent runs on some computer: your laptop, a cloud server, or WSL inside Windows each counts as one. Once a computer is connected to the workspace, you can bring in the Claude Code and Codex sessions already on it, start new agents and open terminals there, and when something breaks, find out why, restart it or upgrade it from the App.

## The Computers page

Click **Computers** at the top of the sidebar. Three numbers sit at the top: how many are online, how many agents there are, and how many terminals. Below, each computer has a card:

- Its system icon and name; the one you are using carries a small "This computer" tag.
- Whether it is online, its system and its myco version. A computer that is away is faded.
- Along the bottom, small avatars of its agents and a count of agents and terminals. When an agent is close to its quota, an amber "95% quota" shows at the bottom right.

When some computers are behind on myco, a note above the list names them, and **Upgrade all** upgrades them together (see "Upgrading myco" below).

![The Computers page: a card per computer, with an upgrade note above](shot:computers)

If you created your workspace with "Start on this computer", this computer is already connected.

## Connecting a computer

Click **Connect a computer** at the top right. It opens Add an agent at the "Connect a computer" tile. Pick one of three ways at the top:

- **Over SSH**: any server you can already `ssh` into from this computer.
- **WSL on this Windows**: only on Windows, when WSL is installed.
- **Copy a command**: works for any computer; you run one line on it.

### Over SSH

1. The App reads your SSH config (`~/.ssh/config`) and lists one host per row. Ones you connected before say "Connected". For a host that is not in the config, click **One not in your config** and enter `user@address` and the port.
2. Tick the ones to connect (several at once is fine) and click **Connect**.
3. Each computer goes through three steps: log in and look around, install myco and connect it, wait for it on the workspace. Each step shows a spinner, a tick or a red cross.
4. When you connect just one and it works, the dialog switches to that computer and lists its sessions.

The App uses your own `ssh` and ssh-agent; it never reads or keeps keys, and it cannot type a password for you. If logging in asks for a password or a key passphrase, add the key to ssh-agent with `ssh-add`, check in a terminal that you can log in without typing anything, and click **Try again**. The first time you connect to a host, the App asks you to check and accept its fingerprint in a terminal. When something fails you get the reason in one sentence, a command you can copy, and "The last lines it printed". This way works for Linux and macOS only; for a Windows computer, use the PowerShell line under Copy a command.

### WSL

On Windows, choose **WSL on this Windows**. Each distribution is listed, marked "Default" and whether it is running. Tick the ones to connect and the App installs myco inside and connects it to the workspace, starting any that are stopped.

### Copy a command

1. Choose **Copy a command**, then that computer's system: **Mac / Linux / WSL** (one `curl` line) or **Windows (PowerShell)**.
2. Click **Copy**, open a terminal on that computer, paste and run it.
3. This side says "Waiting for it…" and switches to that computer once it connects.

The command works once, for 24 hours, and it is as good as a credential, so keep it out of public places. Below it there is also a sentence you can send to an agent already running on that computer, so it connects the computer for you.

Sometimes a card appears above the command, such as "Only computers on the same local network can connect". It means your workspace is only reachable from this computer and its network, so a cloud server cannot get in. Click **Open to the internet** on the card to fix that; see [Open to the internet](#help/public).

![Copy a command: first it checks who can connect, then you run the line on that computer](shot:add-agent-connect)

## A computer's page

Click a card on the Computers page. The title is the computer's name, and the line below shows whether it is online, its system and architecture, host name and myco version. Three buttons sit at the top right:

- **Terminal**: open a terminal in one of its folders. See [Terminals](#help/terminals).
- **Add an agent**: bring in a session it already has, or start a new one. See [Adding agents](#help/agents).
- **⋯**: Rename and Remove.

To go back to the list, use ‹ at the top left, or click **Computers** in the sidebar again. Four tabs follow, and the page always opens on the first.

### Agents and terminals

One row per agent on this computer: what it is doing, which folder, which project, and any quota note. Click a row for its profile card. Terminals come next, each with **Open** and **Settings**. When there are none, a card explains what to do, with a button.

### Access

This tab shows **that computer's own rules**. They live on that computer, not in the workspace, so people elsewhere cannot simply change them.

- **Where agents may start**: new agents can only start in these folders (and the folders inside them). Allowed folders show as small tags. When the App runs on that computer, or reaches it over SSH or WSL, you can type a folder and click **Allow**, or click × on a tag to stop allowing it; the next new agent follows the new list, with no restart. When the App cannot reach it, you get the command to run on that computer instead.
- **Highest access**: the most an agent there can be given: read only, its own folder, or full. This, too, is changed on that computer.
- **Agents run commands here**: terminals need this first. For the computer you are on, allow it right in the App. For another one, run `myco machine shell-access --allow` on it, or click "Allow" in its own myco App; this page updates by itself.

When the computer is away, or its myco is too old to say, the page tells you so.

![Access: allowed folders, highest access, and whether agents may run commands](shot:computer-details)

### AIs and tools

One row per AI: Claude Code, Codex, Gemini CLI, OpenCode. On the right is its status: **Ready**, **Signed out** or **Not installed**. Below, how many MCP servers, skills and plugins it has; expand a row to see each one by name and where it comes from. Only names are listed, never keys or address parameters.

To install or sign in to an AI on the computer you are using, go to Settings › Agents. For another computer, choose it under Add an agent › Start a new one, and you get the command to run there.

### Check and fix

Come here when a computer is away or its agents stop working. The App needs to reach it over SSH (or as WSL on this Windows): the first time it asks "Which one", so pick an SSH host or WSL distribution and click **Use this**. After that the line reads "Over SSH" and the host, and **Change** picks another.

- **Check**: is its runner running, can it reach the workspace, what does its log say last. Each finding gets a row with a green, amber or red dot, the worst first, and a sentence on what to do, plus a command to copy when one helps. At the end you can expand "The runner log's last 20 lines".
- **Restart runner**: restarts myco's runner on that computer.
- **Upgrade myco**: downloads the new version, swaps it in and restarts the runner, in a minute or two.

Typical findings: the runner is not running; it cannot reach the workspace's address (its network, a firewall, or a workspace that is only on a local network); the runner stops once the last SSH window closes; it goes through a proxy that does not work.

![Check and fix: one finding per row, each with what to do](shot:computer-check)

## Upgrading myco

When a computer's myco is behind, the top of its page says so; click **Upgrade**. Upgrading restarts myco there, so tasks running on it are cut short; pick a quiet moment. **Upgrade all** on the Computers page upgrades every computer that can be upgraded.

- If it is away: click once it is back, or run `myco upgrade` on that computer.
- If it is too old to upgrade from here: run `myco upgrade` on it once (or set up SSH under Check and fix and upgrade there). After that it upgrades with a click.

## Renaming and removing

- **Rename**: **⋯ › Rename** turns the title into a text box; Enter saves, Esc cancels. The name must not match another computer in the workspace. Only the name everyone sees changes, not the address.
- **Remove**: **⋯ › Remove** first lists the agents and terminals on it, which go with it; conversations are kept. myco on that computer is refused and stops; to uninstall it completely, run the `myco runner service uninstall` shown in the dialog on that computer. Removing cannot be undone.

## Remote Desktop

To see and control another computer's screen, use **Remote Desktop** from the app library (under Tools in [Apps](#help/apps)). Both computers need the myco App in the same workspace; no agents are needed.

1. On the computer to be controlled, make a Remote Desktop and switch to **Share this computer**. The first time, it installs an optional remote component, and the App asks for your permission first.
2. Click **Start sharing**, and a pairing code appears. By default the other side can only watch; to let it use the keyboard and mouse, tick "Allow keyboard and mouse control" before you start.
3. On the other computer (or an iPhone), open Remote Desktop, pick this computer under **Connect to a computer**, and enter the pairing code.

On a Mac that is being controlled, allow **Screen Recording** (to show the screen) and **Accessibility** (for keyboard and mouse) in System Settings. For now it works on unlocked Macs and Windows PCs on the same local network or Tailscale, without sound, and not on lock or login screens. The pairing code changes each time you share, so give it only to people you trust; **Stop sharing**, or closing the app, ends the connection.

## Questions

**A computer shows as away.** Check that it is on and online, then go to its Check and fix tab and click **Check**. A Mac that went to sleep with its lid closed also shows as away.

**It is connected, but its sessions are not listed.** That session's folder may not be under "Where agents may start". Allow it in the Access tab and try again.

**A cloud server cannot connect at all.** Your workspace is most likely only on your local network. [Open it to the internet](#help/public) first.
