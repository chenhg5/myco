# Open to the internet

A workspace you start on this computer can at first be reached only from this computer and your local network. Open it to the internet and cloud servers, claude.ai cloud sessions, your laptop on the road and colleagues elsewhere can all connect.

## When you need it

A local workspace starts as "This computer and local network only": computers on the same Wi-Fi or office network can connect directly, and nothing else can. Open it to the internet first when:

- you want to connect a **cloud server (VPS)** and have agents work on it;
- you want **claude.ai or Codex cloud sessions** to join;
- a **colleague** is not on your local network and you want to invite them;
- you take your laptop or phone out and still want to reach the workspace on your computer at home.

The App reminds you where it matters. Above the command in Computers › Connect a computer › Copy a command it says "Only computers on the same local network can connect", with an **Open to the internet** button on the right; the workspace card on the "Set up this computer" page has **Let computers outside connect**.

## Opening it

1. Open **Settings › Workspaces** (the workspace at the bottom left › Settings, or ⌘, / Ctrl+, on Windows).
2. Under "Workspaces on this computer", find the one to open and click **Open to the internet**.
3. Pick a way (details below) and click **Open**.

![The three ways to open a workspace to the internet](shot:open-to-internet)

## The three ways

### One click (recommended)

- **No account, nothing to install.** The first time, Cloudflare's small tool cloudflared (about 20 MB) is downloaded for you; after that it is not needed again.
- You get a temporary Cloudflare address like `https://<random-words>.trycloudflare.com`.
- The catch: **the address is not fixed.** It changes when this computer restarts. Come back and click again for a new one, but computers that joined and links you sent stop working and have to connect again with the new ones.

Good for trying things out, or letting a computer in now and then.

### Cloudflare with your own domain (fixed address)

- Needs a free Cloudflare account and a domain hosted on Cloudflare.
- Install cloudflared on this computer first and run `cloudflared tunnel login` once in a terminal, picking your domain. Without cloudflared this option is greyed out.
- Once selected, type a domain in the card, such as `hub.example.com`, and click Open.
- The address never changes, restarts included: the one for the long run.

### Tailscale Funnel (fixed address)

- Needs a free Tailscale account, with Tailscale installed and signed in on this computer.
- The address looks like `https://<computer>.<your-tailnet>.ts.net` and stays the same.
- You can pick it before Tailscale is installed: **Install and connect Tailscale** appears below, with the official download, install commands for each system, and **Ask an agent on this computer to set it up**. Once it is installed and signed in, click **Installed — check again**, then Open.
- The first time you use Funnel, Tailscale may ask you to authorize it in the browser and to turn Funnel on for this device in its admin console; approve system network permissions yourself when asked. Then try again.

## While it opens

After Open, the dialog shows the three steps it is on and how many seconds have passed:

- Cloudflare: download or start cloudflared → open the tunnel to Cloudflare → get the public address and check it is reachable. Usually 10–60 seconds.
- Tailscale: turn on Funnel for this computer → get an HTTPS certificate the first time → get the ts.net address. Usually a minute or two.

Click **Cancel** if you would rather not wait: the workspace keeps working on this computer and its network. It only counts as done when there is a real public address and it can be reached from outside; otherwise it says why, leaves the workspace as it was, and you can try again.

![Opening with Cloudflare: three steps and the seconds waited](shot:open-to-internet-opening)

On success the dialog stays open and lists:

- **Public address**: the workspace's address outside.
- **Join link**: other computers join the workspace with it.

Each has a copy button. With the one-click way, a note below reminds you the address is temporary. **Done** closes it. From then on, the workspace's "Access" row in Settings › Workspaces reads "Cloudflare" or "Tailscale", with the public address below.

## What changes once it is open

- What the App makes from now on, such as invite links, the Connect a computer command and the command to open a terminal, uses the public address and works from anywhere.
- Links and commands made **before** still carry the local-network address and only work on your network. To use one outside, make it again (Copy a command has **Done: make the command again**).
- This computer itself keeps working as before. Once open, the workspace is reached only through its public address and no longer listens on the local network directly, so computers on your network that joined with the local address need to connect again with a new command.
- This computer still has to **stay on and awake**: while it sleeps, nobody outside can reach the workspace. On a Mac, turn off automatic sleep in the battery or energy settings of System Settings.

## Closing it again

There is no separate "close" button. To bring the workspace back to this computer and local network only:

1. Click **Stop** on the workspace in Settings › Workspaces.
2. Click **Start**. It starts again as "This computer and local network only".

While it is stopped, other computers cannot reach it; data and tokens are kept. Back on the local network, computers that joined through the public address can no longer reach it and need to connect again from your network. To open it later, click Open to the internet again.

## A workspace on another computer

It is opened on the computer it runs on. When you click Open to the internet from elsewhere, the App gives three ways: Settings › Workspaces › Open to the internet in the myco App on that computer; the command shown, run in a terminal there; or the sentence shown, sent to an agent on that computer so it does it for you.

## The other way: a server

If you have a Linux server with a public IP, you can put the workspace there instead: always online, independent of whether this computer is on, and with a fixed address. Click the workspace at the bottom left › Join or start a workspace › No link yet? › Over SSH, and the App installs it for you. See [Workspaces](#help/workspaces).

## Security

- Opening to the internet only makes the address **reachable**; without a token or a link, nobody gets in or sees anything.
- **Join links and invite links are keys**: whoever has one can add a computer, or themselves, to your workspace. Send them privately to the people who should join, never somewhere public. An invite link works once, within 24 hours.
- Everyone who joins gets credentials of their own. Admins can remove a person in Settings › People at any time, and all of their devices stop working at once. See [People and access](#help/members).
- On the local network only, connections are plain HTTP; use that only on networks you trust. Once open to the internet, the outside address is HTTPS.
- For a shared workspace whose own server should not read messages, turn on end-to-end encryption in Settings › Workspaces.

## Questions

**It stays on "Opening…"**: the one-click way downloads cloudflared the first time, which takes longer on a slow network; if this computer needs a proxy to get online, set it in [Proxy](#help/proxy) first. Cancel if you cannot wait; the workspace is not affected.

**The Tailscale step failed**: usually Tailscale is not running or not signed in on this computer, or Funnel is not on for this device in the Tailscale admin console. The dialog shows Tailscale's own reason and fix.

**Other computers lost it after a restart**: the one-click temporary address changed. Click Open to the internet again in Settings › Workspaces, then have those computers connect again with the new command or link. For good, switch to your own domain or Tailscale.
