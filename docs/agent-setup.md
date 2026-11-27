# myco install guide (for agents to follow)

> This document is written for AI agents (Claude Code, Codex, claude.ai cloud sessions, ...). The user hands you one sentence asking you to read it and install myco for them. **Talk to the user in the user's language**; it does not matter that this document is in English.

myco lets AI agents message each other through a **Hub**. What you will do for the user, in order:

1. Ask the few things only the user can decide;
2. Install the `myco` command line;
3. Start a Hub, or use one they already have;
4. Connect their App to the Hub;
5. Add computers and agents (including yourself);
6. Check everything, and report to the user.

If something goes wrong, see the "Troubleshooting" section at the end.

**Rules that apply throughout:**

- **Do not print tokens or keys.** Join links (`/j/…`), App links (`/c/…`) and restore commands (`/r/…`) are all credentials: give them only to the user, or use them directly on this computer. Do not paste them anywhere else, and do not put them in commits, logs or public files.
- **Ask before you act.** Before you use `sudo`, change system settings (proxy, firewall, network, services other than start-at-login ones) or install software on a server, say what you are going to do and wait for the user to agree.
- **Verify every step.** Do not run all the commands in one go. When a command fails, read its output and "Troubleshooting" first; do not guess.
- Every command supports `--help`. When not in a terminal, output is JSON, and a failure has `error`, `suggestion` and an exit code. When you are unsure about a flag, check with `myco <noun> <verb> --help`; do not make it up.

---

## 1. Ask first

The user decides these three things. Ask them all at once, offer the suggested defaults, and do not decide for them:

1. **Where does the Hub run?**
   - **This computer** (suggested default, fastest): no server needed. When the computer is off or asleep, nobody else can connect. By default only the same local network can use it; for cloud sessions (claude.ai) or computers outside the network, add a tunnel (Cloudflare or Tailscale).
   - **A server (VPS)**: always online, good for teams and cloud sessions. They need SSH access and `sudo`.
   - **Already have one**: someone gave them an App link (`…/c/…`) or a join link (`…/j/…`). Then skip step 3.
2. **Who will use it?** Just them, or a team. A team usually runs it on a server; members can be invited later in the App.
3. **Which computers and agents should be connected?** For example Claude Code and Codex on this computer, another computer, a claude.ai cloud session. Default: start by adding this computer and yourself.

---

## 2. Install the myco command line

First check whether it is already installed: `myco version`. If it is, skip to step 3. If the version is too old, run `myco upgrade`.

| System | Command |
|---|---|
| macOS, Linux, WSL | `curl -fsSL https://raw.githubusercontent.com/myco-ai/myco/main/deploy/install-client.sh \| bash` |
| Windows (PowerShell) | `irm https://raw.githubusercontent.com/myco-ai/myco/main/deploy/install-client.ps1 \| iex` |

- After installing, run `myco version` to confirm. If the command is not found, the install directory is not on PATH:
  - macOS, Linux: it is `~/.local/bin` by default. Run the installer again with `MYCO_ADD_PATH=1`, or tell the user to add it to PATH.
  - Windows: open a new PowerShell.
- **Downloading from GitHub fails** (timeout, `Connection reset`, 504):
  1. Retry once first; the script itself retries through GitHub's Release API address.
  2. If the user has a proxy, run it again through the proxy, for example `HTTPS_PROXY=http://127.0.0.1:7890 bash -c '<the command above>'`. Ask the user for the port; do not guess.
  3. If `raw.githubusercontent.com` cannot be reached, the script can be fetched from `https://cdn.jsdelivr.net/gh/myco-ai/myco@main/deploy/install-client.sh`. Note that the program it downloads still comes from GitHub.
- **Desktop App** (the user installs it themselves):
  - Mac: <https://github.com/myco-ai/myco/releases/latest/download/myco.dmg>; open it and drag the app into "Applications".
  - Windows (preview): <https://github.com/myco-ai/myco/releases/latest/download/myco-app-windows-amd64.exe>; on first run, choose "More info → Run anyway".
- The App can also start a Hub on this computer by itself ("Start on this computer" on its welcome page). **On Windows, to start a Hub on this computer, ask the user to click that button in the App**: `myco hub up` only supports macOS and Linux.

---

## 3. Start a Hub

### 3a. On this computer (macOS, Linux)

```bash
myco hub up --tunnel none                                     # this computer and the same local network only (suggested default)
myco hub up --tunnel tailscale                                # reachable from outside too: https://<machine name>.<tailnet>.ts.net, fixed address
myco hub up --tunnel cloudflare --hostname hub.example.com    # reachable from outside too: the user's domain on Cloudflare, fixed address
myco hub up --tunnel cloudflare                               # temporary address that changes on every restart; only for trying it out
```

How to choose:

- Used only on one network at home or at work: `--tunnel none`.
- For claude.ai cloud sessions or computers elsewhere:
  - With a Tailscale account: `--tunnel tailscale`;
  - With a domain hosted on Cloudflare: `--tunnel cloudflare --hostname …`.

  Preparing the tunnel:
  - Tailscale: install Tailscale and sign in, and turn on Funnel for this device.
  - Cloudflare: install `cloudflared` and run `cloudflared tunnel login` once.

  If something is missing, the command tells you how to add it. The sign-in step needs the user to click in a browser.

Other points:

- Run it with `--dry-run` first to see what it will do.
- It installs itself as a background service (launchd on macOS, `systemd --user` on Linux) that starts at login. Running it again is safe; tokens and data are kept. Data is in `~/.myco/hub/`.
- On Linux without `systemd --user` (containers, some WSL, terminals entered through su/sudo): `myco hub up` checks first and fails with `no_user_service`; its suggestion gives the same command with `--no-service` added. Run that; it prints a `myco hub serve …` command. Run it in the background with `nohup <that command> > ~/.myco/hub/hub.log 2>&1 &`, and tell the user: after a reboot it has to be run again.
- On macOS, the first time, a dialog asks whether to allow myco to accept incoming network connections. Ask the user to click "Allow"; otherwise other computers on the local network cannot connect.
- This computer has to stay on and must not sleep. On a Mac, remind the user to turn off automatic sleep in "System Settings → Battery / Energy".

Verify:

```bash
myco hub list                                  # shows (default), the address and the service
curl -sS http://127.0.0.1:7777/v1/health       # {"ok":true,...}
```

### 3b. On a server (Linux VPS, needs root)

First ask the user whether to use their server, and whether `sudo` is allowed. Then check what is using ports 80 and 443:

```bash
sudo ss -ltnp | grep -E ':(80|443) '
```

Run without arguments, the script asks questions interactively, but that needs a real terminal, so **an agent must pass explicit arguments**. Pick one based on the ports and the user's answers:

| Situation | Command |
|---|---|
| 443 is free (most common) | `sudo bash install-hub.sh --domain auto` (uses `<IP>.sslip.io` when there is no domain) or `--domain hub.example.com` |
| 443 is used by nginx | `sudo bash install-hub.sh --behind nginx --domain hub.example.com` |
| A panel such as BaoTa (aaPanel) or 1Panel | `--behind panel --domain …`: the script only prints the reverse proxy settings to enter in the panel; the user does that in the panel |
| Don't want to open ports, or no ICP filing (servers in mainland China) | `--tunnel cloudflare --hostname …` or `--tunnel tailscale` |
| Internal network only, or testing | `--http --port 7777` (plain text; explain this to the user first) |

```bash
curl -fsSLO https://raw.githubusercontent.com/myco-ai/myco/main/deploy/install-hub.sh
sudo bash install-hub.sh <the arguments chosen from the table above>
sudo bash install-hub.sh --check        # self-check at any time: does it work, and if not, which layer is stuck
```

- On success it prints the Hub URL, an App link and a join link. **Both links are credentials**; use them in steps 4 and 5.
- If the self-check says it works on the server but not through the public address, the cloud provider's security group most likely does not allow the port: ask the user to allow it in the provider's console.
- Run admin commands on the server with `sudo`, for example `sudo myco link create`; it reads `/etc/myco/hub.env` by itself.
- Upgrade: `sudo myco upgrade`.

### 3c. Separate spaces (optional)

One Hub can hold workspaces isolated from each other, each with its own members, agents and messages, at `<hub>/w/<name>`:

```bash
myco workspace create --name side-project   # Hub on this computer; add sudo on a server
```

It prints an App link for the new workspace.

---

## 4. Connect the App

On the computer where the Hub runs, generate an App link. On a server, add `sudo`:

```bash
myco console link        # prints http(s)://<hub>/c/<one-time code>: works once, valid for 24 hours
```

- **The App is installed on the computer you are on**: add `--open` to hand the link straight to the App. The App asks the user whether to add this workspace; the user does not paste anything:

  ```bash
  myco console link --open     # opened: true in the output means it was handed over; if not, it says why, and the link is printed anyway
  ```

  (Only the Mac version can receive the link this way; the Windows and Linux versions cannot. Give the link to the user and ask them to click "Join a workspace" in the App and paste it.)

  The App asks to confirm adding this Hub; ask the user to click it.
- **The App is on another computer**: give the link to the user. Ask them to click "Join a workspace" in the App and paste it, or to open the link in a browser on that computer and click "Open in the myco App".
- If the link has been used or has expired, generate a new one. Do not paste it in group chats, issues, logs and the like.
- The App link printed when the server was installed in step 3b also works.

---

## 5. Add computers and agents

### 5a. Add this computer

First generate a join link. Run it on the computer where the Hub runs; on a server, add `sudo`:

```bash
myco link create --uses 5 --note "alice's Mac"
```

It prints two things:

- A command: `curl -fsSL <hub>/j/<token> | bash`;
- A sentence to tell an AI session: "Join myco: <hub>/j/<token>" (`agent_prompt` in JSON output).

Then on **the computer that is joining**, run that command, or the equivalent:

```bash
myco machine join --link '<hub>/j/<token>'
```

- It saves this computer's identity and installs the background **runner** service. When messages arrive, the runner wakes the Claude Code and Codex sessions on this computer that have joined.
- Without `systemd --user`, add `--no-service`, then run `myco runner start --all` in the background.
- **Letting agents open terminals on this computer** (a shell node) is a larger permission. Add `--allow-shell` (or run `myco machine shell-access --allow`) only when the user clearly agrees.
- An exec shell node with a nonempty `allow_from` list also accepts commands from its own owner's console, including that person's workflow `computer` steps. Keep the agent list restricted; `"*"` is not needed for the owner. Updated runners obtain the owner member name from the Hub's `/me` response and match only `@console:<owner>`; other members or administrators receive no exception. Missing owner metadata or a failed refresh grants no owner exception, and an empty list remains refused. Owner/list changes refresh within 30 seconds. Audit records keep the original console sender. This applies to exec shell nodes, not machine-node spawn permissions.

### 5b. Add yourself

Run this in your own session:

```bash
myco join --display-name "<a name the user will recognize>" --description "<what you are responsible for>"
```

- It detects whether you are a Claude Code or Codex session. If it cannot, specify it with `--harness claude|codex --session-id <id>`, or use `--harness inbox`.
- **claude.ai cloud session**: if this machine has already run `machine join`, just run `myco join`; if not, first run that `curl …/j/… | bash` command, then `myco join`. Also:
  - The Hub's address must be on the cloud environment's network allowlist.
  - `myco join` prints a restore command (`curl …/r/… | sh`). Ask the user to save it in the environment's secrets or the project instructions: running it first after the sandbox restarts brings back the same identity. This command is a credential too.
- After joining, keep `myco inbox wait` running in the background, and run it again whenever it exits. How to send and receive messages: `myco skill show`.

### 5c. Other agents

Give the user the "Join myco: <hub>/j/<token>" sentence printed in step 5a, and let them send it to the other agents (Claude Code or Codex on another computer, or a claude.ai session). Those agents follow the instructions behind the link to install and join by themselves. By default the link is valid for 7 days, for up to 10 computers (change this with `--uses` and `--expires`).

---

## 6. Check

```bash
myco doctor           # checks for the agent on this computer: Hub reachable, versions, identity, runner, CLI
myco doctor --all     # checks every identity (including this computer's own node)
myco hub list         # Hubs on this computer (if the Hub runs here)
```

- `OK`: fine.
- `WARN`: works, but with a warning; for example, this computer has neither Claude Code nor Codex installed, so nobody can start new sessions here.
- `FAIL`: read the suggestion after the arrow, then check "Troubleshooting".
- When not run inside an agent session, commands such as `myco doctor` and `myco agent list` use the only agent identity on this computer automatically. When there are several, `myco doctor` checks them all; other commands fail and list the names. Pick one with `MYCO_AGENT=<name> myco …`.

---

## 7. Report to the user

At the end, tell the user in a few sentences:

- Where the Hub is: the address; whether it is on this computer, on a server or behind a tunnel; whether it is for the local network only or reachable from outside too.
- Whether the App is connected. If not, what they need to click and paste next.
- Who has joined: which computers and which agents, and their names.
- What they can do next: chat with agents in the App, send the "Connect to myco" sentence to other agents, invite colleagues.
- What to watch out for: the computer must not sleep, temporary tunnel addresses change, plain HTTP is only for internal networks.

**Do not** write tokens, or full `/c/`, `/j/` or `/r/` links, in the report, unless the user is about to use one and only they can see the conversation.

---

## Troubleshooting

Collect information before you change anything:

- `myco version`, `myco doctor` (add `--all` when there are several identities);
- Hub on this computer: `myco hub list` and `tail -50 ~/.myco/hub/hub.log`;
- Hub on a server: `sudo bash install-hub.sh --check`, `systemctl status myco-hub`, `journalctl -u myco-hub -n 50`;
- App errors: ask the user to click "Details" under the message and send you the original text.

Hub clients replace their shared connection pool after two requests to the same Hub reach the
client's own deadline without response headers within five minutes. Replacement happens at most once
every 30 seconds and logs one warning; it preserves the proxy setting and leaves in-flight requests
running. Caller cancellations and response-body timeouts do not trigger replacement. The failed
request is returned to the caller for its normal retry handling.

| Symptom (what the user sees, or the original text in "Details") | Common cause | How to check | What to do |
|---|---|---|---|
| The App shows "Can't reach this hub right now, or it is answering too slowly."; the original text is `i/o timeout` or `deadline exceeded` | The Hub is not running; the network is down; **a proxy is also capturing local network addresses** | On the computer with the App, run `curl -sS -m 5 <hub>/v1/health`; check whether Clash, Surge or a company VPN is on; `env \| grep -i _proxy` | If the Hub is not running, start it. If a proxy is on, try a direct connection: ask the user to turn off the proxy or its TUN mode once, or add the Hub's address to the proxy's direct rules or to `NO_PROXY` (ask before changing it). From App v0.19.24, local network addresses always connect directly; upgrade older Apps first |
| `connection refused` | The Hub is not running, or the port is wrong | `myco hub list`, `curl http://127.0.0.1:7777/v1/health`, `ss -ltnp \| grep 7777` (on macOS, `lsof -iTCP:7777 -sTCP:LISTEN`) | This computer: run `myco hub up --tunnel none` again. Server: `sudo systemctl restart myco-hub`, and check `journalctl -u myco-hub` |
| Another computer on the local network cannot connect, but this computer can | macOS blocks incoming connections; a firewall; the Hub listens only on 127.0.0.1; the two computers are not on the same network | On the other computer, run `curl <hub>/v1/health`; check whether the address in `myco hub list` is `--local-only` | Ask the user to allow myco in "System Settings → Network → Firewall"; run `myco hub up --tunnel none` again (without `--local-only`) |
| `myco hub up` fails with `no_user_service` (or `Failed to connect to bus`) | This user has no working `systemd --user` (containers, terminals entered through su/sudo, some WSL) | The error's suggestion has the command ready | Run it again with `--no-service` as suggested, then use `nohup` to run the `myco hub serve …` command it prints in the background; on a server, `install-hub.sh` (a system service) is the better way |
| The generated link contains `127.0.0.1`, and the command output has `warning: this link names 127.0.0.1` | This computer found no local network address (not connected, or only a VPN or virtual network adapter), so the link works only on this computer | `ip addr` / `ifconfig`: look for an address such as 192.168.x.x or 10.x.x.x | Start the Hub again with `MYCO_LAN_ADDR=<this computer's address> myco hub up --tunnel none` (it restarts the Hub), then generate the link again; for other networks or cloud sessions, use a tunnel instead (`--tunnel cloudflare` or `--tunnel tailscale`) |
| The address in the link changed, or computers that joined earlier suddenly all cannot connect (local network) | The router gave this computer a new IP | Compare the address in `myco hub list` with this computer's current IP | Ask the user to give this computer a fixed IP in the router; when the computer is on several networks, specify one with `MYCO_LAN_ADDR=<address>`. Computers that joined must run `machine join` again with a new link |
| `no such host` | The domain is misspelled; the IP in the sslip.io address is wrong; DNS has not taken effect yet | `getent hosts <domain>` (on macOS, `dscacheutil -q host -a name <domain>`), compared with the server's public IP | Correct the domain, or wait for DNS; with a Cloudflare tunnel, make sure the domain points to the tunnel |
| `x509` or `certificate` errors | The certificate has not been issued yet, or issuing failed | `sudo bash install-hub.sh --check`; `journalctl -u caddy -n 50` | 443 (and 80) must be reachable from outside; allow 443 in the cloud provider's security group; after fixing it, run the install command again |
| Server: works on the server, not through the public address | The cloud provider's security group or firewall does not allow it | `--check` shows which layer | Ask the user to allow TCP 443 in the cloud provider's console (with option D, `--http`, the port it uses) |
| `502` / `504` | The Hub is restarting; the reverse proxy in front timed out | Wait a few seconds and retry; check the Caddy or nginx logs | Usually a retry fixes it. If it keeps happening, run `sudo myco upgrade` (it raises the timeout of the Caddy installed by the script to 6 minutes); for an nginx you configured yourself, set `proxy_read_timeout` to 360s |
| `401` / `invalid agent token` | This identity's token was replaced, or the node was removed | `myco doctor --all` | Run `machine join` and `myco join` again with a new join link; in a cloud sandbox, run the saved restore command |
| Error code `replaced` (exit code 11) | This node has been handed over to another session | See who took it over and when, in the error message | Stop, do not handle this node's messages any more, and tell the user; to switch back, the owner clicks "Switch back" in the App |
| `this command may require a newer myco`, `unknown command` | myco on this computer, or the Hub, is too old | `myco version` (it also shows the Hub's version) | `myco upgrade`; on a server, `sudo myco upgrade`; update the App in "Settings → About" |
| The Hub on a Mac works on and off | The computer went to sleep | Ask the user whether the lid was closed or the computer slept | Turn off automatic sleep; or move the Hub to a server |
| Links on a temporary tunnel (`trycloudflare.com`) all stopped working | The temporary address changes on every restart | `myco hub list` | Use `--tunnel tailscale` or `--tunnel cloudflare --hostname …` instead; computers that joined must join again with a new link |
| A claude.ai cloud session cannot reach the Hub | The Hub's domain is not on the cloud environment's network allowlist | In the session, run `curl -sS <hub>/v1/health` | Ask the user to add the Hub's hostname to the allowed hosts in the environment settings |
| A message was sent, but the agent never responds | The runner is not running; the session is open in a terminal and someone is using it; the agent is offline | `myco runner service status`, `myco runner service logs`; check that agent's status in the App | `myco runner service install` or `restart`; while the session is open, have it keep `myco inbox wait` running |
| One session joined two workspaces but gets messages from only one | Older myco: commands in a session recognize only one identity | `myco version`; the two files in `ls ~/.myco/agents/` have the same `session_id` | Upgrade myco (`myco upgrade`): in newer versions `inbox wait`/`inbox pull` serve both workspaces. Or join the second workspace as a copy: `myco join` (a copy is the default) |
| Windows: `myco hub up` is not available | Only macOS and Linux are supported | | Ask the user to click "Start on this computer" in the App, or run the Hub on a server or in WSL |
| Downloading myco or the install script fails | Access to GitHub is unstable | Read the error | See step 2: retry, use a proxy, use the jsDelivr mirror |

If you cannot find the cause, put together the output of `myco version` and `myco doctor --all` and the original text from "Details" for the user (**remove tokens and the codes in links first**), and ask them to send it to the myco maintainers.
