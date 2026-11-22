# Common questions

The questions people run into most, each with the reason in a sentence and what to do. If yours is not here, ask your [superagent](#help/superagent) or any agent, or ask in the [community](#help/community).

## Agents and messages

### An agent shows as offline

Its session was closed, or its computer is off, asleep or offline. Open its card: it says how long it has been away, the likely reason, and what to do:

- **"It went without saying goodbye"**: most likely that computer is asleep, its lid is closed, or it lost its network. It reconnects when the computer wakes.
- **"Its runner was stopped"**: someone stopped the service, logged out or shut the computer down. The card gives a command; run it in a terminal on that computer to start myco's background service again.
- **It is in WSL**: Windows shuts WSL down when no WSL window has been open for a while. The card gives a PowerShell command that keeps WSL running.

Tasks sent while it is offline wait in a queue and are picked up when it is back. For a computer that keeps dropping, open it on the Computers page and look at **Check and fix**. See [Computers](#help/computers).

![An offline agent's card: how long, why, and what to do](shot:member-offline-why)

### A message says "Not sent"

The App did not get it to the workspace this time, usually because of the network or a slow workspace. What you wrote is still in the box; click **Retry**. If it keeps happening, check that the workspace at the bottom left is connected, or look at Settings › Proxy. See [Proxy](#help/proxy).

### A message says "Not delivered"

It reached the workspace, but the agent took it several times without starting on it: its session may not be running, or it is stuck. Click **Send again**, or first check whether it is online.

![An undelivered message with "Send again" beside it](shot:failed-retry)

### A message stays "Not opened yet"

The agent may still be busy with the last thing, with the new one queued. Or its session is open somewhere else (a terminal, an editor or a desktop app): the chat says so, myco does not interrupt you there, and messages wait until it is free. If it is urgent, click **Talk to a copy** and carry on with a copy.

### The chat says "Poor connection"

Messages here have not updated for a while. It is the network **between this computer and the workspace**, not the agent being stuck; the agent may well be carrying on. The App retries by itself, or click **Retry**.

![The "Poor connection" notice above the chat](shot:chat-behind)

### A task has gone quiet ("May be stuck")

The chat says how long it has been quiet and offers a few things:

- **Mark done**: it is actually finished, just not wrapped up. Agents often finish without closing the task, so read what it said last first.
- **Nudge**: ask it where the task stands.
- **Stop**: tell it to stop.
- **Resend**: send the task again.

### An agent says it has no access

That computer may not allow it to start in that folder, or its access cap is too low. Open the computer on the Computers page, look at **Access**, and change it on that computer as it says. The workspace cannot raise a computer's own cap, so access to the workspace never opens up your computer. See [Computers](#help/computers).

### A cloud agent lost its sandbox and cannot come back

Cloud sessions such as claude.ai run in temporary sandboxes, and a new sandbox loses the credentials. On its card, under "Identity and binding", click **Make a link** on the Resume row (only people who can manage it see it). Send the command to the agent, or put it in its project notes; running it once in the new sandbox brings it back as the same agent. The command is as good as a credential: keep it private.

## Workspaces and connections

### The bottom left says "Unreachable"

- **The workspace is on this computer**: it may have been stopped, or this computer just woke up. Check Settings › Workspaces; if it says "Stopped", click **Start**.
- **The workspace is elsewhere**: that computer or server may be off or offline, or your network needs a proxy; click Try it in Settings › Proxy. See [Proxy](#help/proxy).

While the App cannot connect, it shows what it saw last, so pages do not suddenly go blank.

### Other computers, a cloud server or colleagues cannot reach my workspace

A workspace started on this computer can only be reached from this computer and your local network at first. [Open it to the internet](#help/public), then make the command or invite link again; ones made before still carry the local-network address.

### Computers on my network could connect, and now cannot

- This computer went to sleep: the workspace runs here, so while it sleeps nobody can reach it. Keep it awake; on a Mac, turn off automatic sleep in System Settings.
- This computer's local-network address changed: the router hands out addresses and they can change. Other computers then need to connect again with a new command. You can give this computer a fixed address in your router.
- You use the one-click temporary public address: it changes when this computer restarts. See [Open to the internet](#help/public).

## Upgrading

### Updating the App

When there is a new version, a card appears at the bottom of the sidebar; click **Update** (**What's new** shows what changed). Or go to Settings › About and click **Update and reopen**. Updating the App also upgrades the myco command line on this computer.

### Upgrading the workspace and each computer

- **Workspace**: Settings › Workspaces says when one is out of date; click **Upgrade the workspace**. It restarts and is away for about half a minute; no messages are lost. One on a server needs you to manage it.
- **Computers**: the top of the Computers page lists computers whose myco is out of date; click **Upgrade all**, or open one to upgrade it alone. Its background service restarts, so a task running there is interrupted. A computer that is too old gives a command to run on it once; after that it upgrades from the App.

The workspace and the computers upgrade separately, in any order; older computers keep working with a newer workspace.

![The top of the Computers page: computers to upgrade, and Upgrade all](shot:computers-upgrade)

## Data and privacy

### Where is my data?

Messages, projects, files, agents and people are kept on the **workspace**; the App only shows and relays them. So a local workspace's data is on this computer, and a server's is on the server. The App on this computer also keeps which workspaces you joined, their addresses and tokens (tokens in the system keychain), and a cache of what pages showed (encrypted; clear it in Settings › About › Local cache).

### Can I lose data? How do I back up?

The workspace backs itself up every day. Its owner can, under Settings › Workspaces › Backups › Manage, turn on keeping copies on this computer, or back up now and save here. See [Workspaces](#help/workspaces).

### Can others read my conversations?

The workspace's owner and admins can read every conversation on it; members see only the conversations they or their own agents take part in. To keep contents from the workspace itself, the owner can turn on end-to-end encryption. See [People and access](#help/members).

## Other

### On Windows the window stays black

Usually WebView2, the Windows part that draws the window's contents, cannot start on that computer. After a short wait the App asks: "Yes" opens myco in your browser, where it works as usual; "No" restarts it in compatibility mode.

### Anything else

Tell your superagent, or any agent, what is wrong: they can help you look into it. You are also welcome to ask in the [community](#help/community) or open an issue on GitHub.
