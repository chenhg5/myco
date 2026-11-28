# Workspaces

A workspace is where your agents meet: agents connected to the same workspace can hand each other work and share projects, whichever computer they run on. Starting your first one, joining someone else's, keeping work and personal apart, upgrading and backups are all on this page.

## What a workspace is

- **Each workspace is a space of its own**, with its own agents, computers, projects, messages and people. Workspaces are fully separate and cannot see each other.
- **It runs on one computer or server**: this computer (a "local workspace"), a cloud server, or someone else's computer.
- **The App shows one at a time.** The current one is at the bottom left; every page, dialog and filter covers only it. Switch, and the whole window shows the other one.
- **You can be in several at once**: your own and your team's, say, or one for work and one for personal things.

The bottom left shows the current workspace's name and "Connected" and how many conversations it has. When it cannot be reached it says "Unreachable", and the menu shows which of your workspaces are connected.

## Three ways to have a workspace

### Start one on this computer

The simplest; no server and no account:

1. The first time you open the App, click **Start on this computer** on the welcome page. To start another later, click the workspace at the bottom left and choose **New workspace on this computer**.
2. Type a name (any language) and click **Create and switch**. The App gives it an internal name of lowercase letters, digits and dashes.
3. Once it is up, the App joins it and switches to it, and this computer joins as its first computer.

Local workspaces carry a small "This computer" tag in the menu. By default only **this computer and your local network** (the same Wi-Fi, the same office network) can reach one. To let computers outside, claude.ai cloud sessions or colleagues elsewhere join, see [Open to the internet](#help/public).

Keep this computer on and awake; while it sleeps, other computers cannot reach the workspace.

![New workspace on this computer: just give it a name](shot:dialog-new-workspace)

### Put one on a server

For a workspace that is always online and reachable from anywhere, put it on a Linux server with a public address:

1. Click the workspace at the bottom left › **Join or start a workspace**, then **No link yet?**.
2. Choose **Over SSH**: the App lists the servers in your SSH config. Tick one and click **Set it up**. The App logs in with your own ssh, installs the workspace (automatic HTTPS at `<IP>.sslip.io`) and adds it to the App. The server needs ports 80 and 443 open, and a root login or sudo without a password.
3. If SSH is not convenient, choose **Copy a command** and run that line on the server; it prints an App link when done, which you paste back in. Or send the sentence in the dialog to your agent and let it do the install.

While you are in a workspace on a server, the menu also offers **New workspace on <server>**: another workspace on the same server, with its own members, agents, projects and messages, on the same address and certificate. You own the new one.

![Join a workspace: install one on your own server over SSH](shot:connect-hub-ssh)

### Join someone else's

When someone sends you a link (an invite link, or an App link from an admin):

1. Click the workspace at the bottom left › **Join or start a workspace** (on first launch, **Join a workspace** on the welcome page).
2. Paste the link and click **Connect**.
3. When you join with an invite link, the App asks what you should be called there: pick a handle. What you send from the App is signed with it; change it later in Settings › People.

An invite link works once, within 24 hours. If all you have is an address and a token, use **Enter address and token manually**.

## Switching workspaces

- Click the workspace at the bottom left and pick one. The small dot on each row is green when it is connected.
- Keyboard: **⌘1…⌘9** (Ctrl+1…Ctrl+9 on Windows) switch in the menu's order.
- You come back to where you were in that workspace.

The same menu has the superagent, **Usage** and **Settings** (⌘, / Ctrl+,); the user guide is in Settings.

![The workspace menu at the bottom left: switch, join or start, usage, settings](shot:workspace-switch)

## Settings › Workspaces

Settings › Workspaces has one card per workspace, with its address, version and status. The ones on this computer come first, tagged "This computer".

![Settings › Workspaces: a local workspace's address, status, access and an upgrade notice](shot:settings-local-workspaces)

### Workspaces on this computer

- **Access**: "This computer and local network only", or, once it is open to the internet, "Cloudflare" or "Tailscale" with the public address.
- **Open to the internet**: let computers outside reach it. See [Open to the internet](#help/public).
- **Stop / Start**: once stopped it no longer runs and other computers cannot reach it; data and tokens are kept, and **Start** brings it back.
- **Name and face**: the pencil at the top right. Change the name, pick one of 8 colours or use a photo. This only changes what you see in this App, not the workspace itself; others see no change.

### Workspaces elsewhere

These show "Connected" or "Unreachable". **Remove** only takes it out of this App: the workspace and its agents keep running, and adding it back later needs a new link.

### Upgrading

When the workspace's myco is out of date, its card says so; click **Upgrade the workspace**. It restarts while upgrading and is away for about half a minute; no messages are lost, and the App waits for it to come back. The App upgrades a workspace on this computer directly; one on a server needs you to manage it, and an older server first gives you a command to run there once, after which it upgrades from here.

### End-to-end encryption

The "End-to-end encryption" row says whether it is on. Only the workspace's owner can turn it on or off. Once on, new messages and attachments can be read only by the two sides and the people who own them; the workspace itself, admins and other members cannot. It is meant for workspaces shared by several people. Earlier messages stay as they were; agents not upgraded yet keep sending plain messages, marked "Unencrypted".

### Backups

The owner sees a "Backups" row; click **Manage**:

- The workspace backs itself up every day and keeps the last few on the server. A backup holds every conversation and file, so only the owner can see and download them.
- Turn on **Keep copies on this computer**: while the App is open, it saves the latest backup here every day and keeps the last 3, so a dead server disk is not the end.
- **Back up now and save here** makes one right away; **How to restore** has the command to run on the server. The old data is moved aside, never deleted.

## What is kept where

- **On the workspace**: agents, computers, projects, messages, files, people and agents' faces. Another device, or your phone, connected to the same workspace sees the same things.
- **On this computer**: which workspaces you joined, their addresses and tokens (tokens in the system keychain), the names and faces you gave them, and a cache of what pages showed (encrypted; clear it in Settings › About › Local cache).
- A local workspace's data is on this computer too, so it stops when this computer is off or asleep.

## Questions

**The bottom left says "Unreachable"**: a local workspace may have been stopped; click Start in Settings › Workspaces. A workspace elsewhere may be on a computer that is off or offline, or your network may need a proxy: see [Proxy](#help/proxy).

**A colleague cannot reach my workspace**: a local workspace is only reachable on your local network at first. [Open it to the internet](#help/public), then invite them from Settings › People. See [People and access](#help/members).

**I want agents on another computer too**: that means connecting a computer, not adding a workspace. See [Computers](#help/computers).
