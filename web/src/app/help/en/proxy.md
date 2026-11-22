# Proxy

If your computer reaches the internet through a proxy (a local Clash or V2Ray, say), tell the App which one here. The App uses it to reach workspaces elsewhere, to check for updates, and for translation and voice services.

## Where to set it

Open **Settings › Proxy** (the workspace at the bottom left › Settings, or ⌘, / Ctrl+, on Windows; it is in the "This computer" group on the left). Typing "proxy" in the search box at the top of Settings finds it too.

This is a setting of this computer only: it is not kept on the workspace and affects nobody else.

![Settings › Proxy: a local SOCKS5 proxy set by hand](shot:settings-proxy)

## Three choices

At the top there are three options:

- **From the environment** (the default): uses the proxy in the environment variables (HTTPS_PROXY, HTTP_PROXY, ALL_PROXY), and connects directly when there is none. Most people never change it.
- **No proxy**: every connection goes direct, through no proxy. Pick this when a proxy app is running but you want the App to bypass it.
- **Set here**: fill in a proxy yourself; see below.

On a Mac, an App opened from the Dock or Launchpad usually does not see proxy variables you set in a terminal. So if your proxy app is simply "on" but the App cannot reach the outside, you most likely want **Set here**.

## Setting a proxy by hand

1. Choose **Set here**.
2. **Type**: SOCKS5, HTTP or HTTPS, whatever your proxy app offers.
3. **Host**: the machine the proxy runs on. For a proxy app on this computer, `127.0.0.1`.
4. **Port**: the port your proxy app shows. Common ones:
   - Clash and similar: HTTP is often 7890 and SOCKS5 7891 (some versions have one "mixed port", 7890, for both);
   - V2Ray and similar: SOCKS5 often 1080, HTTP 1087 or 8080; v2rayN on Windows defaults to SOCKS5 10808 and HTTP 10809.
   If unsure, look in your proxy app's settings.
5. **User, Password**: only if the proxy needs a login; leave them empty otherwise. The password is kept in the system keychain and not shown after saving; **Remove the password** clears it.
6. Click **Try it** first: the App connects once to the current workspace (GitHub when there is none) with what you filled in, and says how many milliseconds it took, or why it failed.
7. When it works, click **Save**. "Saved; in effect now" means you are done; no restart needed.

## When "Try it" fails

- **No answer within 12 seconds**: the host or port may be wrong, or the proxy itself cannot get out.
- **The proxy refused the connection**: the proxy app is not running, or the port is wrong.
- **That address was not found**: the Host field is mistyped.
- **The proxy wants a user name and password, or did not accept these**: add or fix them.

When a proxy you set by hand cannot be reached, the App's requests fail; they **never** quietly fall back to a direct connection. If you quit your proxy app, come back and switch to From the environment or No proxy.

## What goes through it, and what never does

**Through the proxy** (From the environment with a proxy set there, or Set here):

- reaching workspaces on other computers or servers;
- checking for updates;
- translation and voice services.

**Never through the proxy, always direct**:

- this computer's own addresses (`localhost`, `127.0.0.1`), such as a workspace on this computer;
- local-network addresses (`192.168.x.x`, `10.x.x.x` and the like, and names ending in `.local`).

So even when a proxy app does not exclude the local network, workspaces on this computer and your network keep working.

## It covers only the App

This setting affects only the App. myco's background service on the computer, and the Claude Code, Codex and other agents it wakes, use their own environment:

- When agents cannot reach their model ("Network unreachable", "request timed out"), run `myco runner service install` once more in a terminal where the proxy is set; it writes that terminal's proxy settings into the service.
- For agents on another computer to use a proxy, set it up on that computer the same way.

## On the phone

The iPhone App has Settings › Proxy too: by default it follows the iPhone's own proxy and VPN settings; you can also choose no proxy, or set a SOCKS5 / HTTP proxy by hand. To use Clash on your computer, enter the computer's local-network address (not 127.0.0.1) and turn on "Allow LAN" in Clash.

## Questions

**Some pages still fail after changing it**: it takes effect at once, and connections already open switch over when idle. Wait a few seconds and click Retry.

**The workspace at the bottom left says "Unreachable"**: click Try it here to see whether the proxy works. A workspace on this computer has nothing to do with the proxy; check whether it was stopped in Settings › Workspaces. See [Workspaces](#help/workspaces).

**Opening a workspace to the internet keeps waiting**: the one-click way downloads cloudflared the first time; if your network needs a proxy, set it here first. See [Open to the internet](#help/public).
