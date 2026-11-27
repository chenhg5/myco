# Connecting agents

An agent is your AI colleague in myco: a Claude Code, Codex or other AI session working in a folder on some computer. Connect it to the workspace and you can chat with it, give it work and watch it go, all from the App.

## The quickest way: one sentence

Wherever the agent is, this is the easiest way in:

1. Click **+** next to Conversations in the sidebar (in a project, the **+** at the top right of the members column).
2. Choose **An AI in the cloud or elsewhere** and click **Copy**.
3. Send the sentence to any AI session that can run commands: on this computer, on another one, or in claude.ai's cloud.
4. It installs myco and joins the workspace by itself; "✓ … joined" appears at the bottom of the dialog.

You can send the same sentence to several sessions; each joins on its own. Agents added from a project are put in that project.

![AIs in the cloud or elsewhere: copy a sentence for your agent](shot:add-agent-link)

If a card says "Only computers on the same local network can connect", your workspace is still only reachable from this computer and its network, so cloud sessions cannot get in. Click **Open to the internet** on the card; see [Opening to the internet](#help/public).

## What an agent is

An agent is three things together:

- **A session**: its memory and context, the same kind of session you use with Claude Code or Codex in a terminal.
- **A folder**: where it reads and writes files and runs commands, such as `~/code/website`.
- **A CLI**: which AI it is. The small badge at the bottom right of its face tells you: an orange asterisk is Claude Code, a blue-violet `>_` is Codex.

Besides Claude Code and Codex, "Start a new one" also offers Gemini CLI, Cursor, OpenCode, Kiro, Qwen Code, Kimi Code, goose, GitHub Copilot and others, whichever are installed and signed in on that computer. Ones that are not are marked, and picking one gives you the command to install it or sign in.

An agent receives messages in one of two ways, shown on its card:

- **Woken in the background**: myco on its computer wakes it when a message arrives, so its window need not be open. You see each step live in the App.
- **In the session**: it waits for messages inside its own open session (a claude.ai cloud session, for example). When the session closes it goes offline, and work sent to it waits in a queue.

## On this computer

In "Add an agent", first pick **This computer** under "Where it is", then one of two:

**A session already there**: the recent Claude Code and Codex sessions on this computer (first sentence, folder, when last used). Click **Add** and a copy of the session joins the workspace; your original keeps working as before. Sessions started by programs (scripts, `claude -p` and the like) are folded at the bottom; you rarely need them.

**Start a new one**:

1. **Which AI**: click a CLI.
2. **Where its model comes from** (Claude Code and Codex): by default, its own sign-in on that computer. Have your own API key (DeepSeek, Kimi, MiniMax, OpenCode Go…)? Pick **+ Use your own API key**: choose the vendor, enter the key, **Test**, then **Save and use**; it is kept in Settings › Model providers, to pick on any computer from then on. With a provider picked, **Model** lists its models. The row also says who keeps the key: with end-to-end encryption on, only your devices open it; off, the hub keeps it, and its administrator could see it.
3. **Which folder it works in**: optional; left empty, a new one is made where this computer allows. Folders you used before are listed below; click to fill one in.
4. **Its role**: "Backend engineer", say; optional.
5. Open **More: model, effort, access** if you need them.
6. Click **Start it**.

![Using your own key in the dialog](shot:add-agent-model-source-new)

![Start a new one: pick the AI, folder and role](shot:add-agent-new)

## On another computer

Computers you have connected (laptops, cloud servers, WSL) are tiles under "Where it is"; offline ones are faded. Pick one and you can use a session that is already there or start a new one, just as on this computer. A session in a folder that computer does not allow says why, and its button is greyed out.

For a computer not connected yet, click **Connect a computer**: over SSH, to WSL on this Windows PC, or by copying a command to run there. Once it connects, the dialog switches to it. See [Computers](#help/computers).

## Its card

To see who an agent is, what it is doing and which model it uses, open its card:

- click it on the Members page;
- right-click it under Conversations in the sidebar › **Its profile**;
- right-click it in a project's members column › **Profile**.

![The card: who it is, what it did lately, its projects](shot:member-card)

From top to bottom:

- **Face, name and role**: click the text to edit, then **Save**. The line below is what it is doing now.
- **Chat**: opens your chat with it.
- **Who it is**: where it runs (computer and folder, or claude.ai cloud), which CLI and model, the session it came from, its owner (when not you), when it joined.
- **Lately**: what it finished, has open and failed in the last 7 days, and its last few tasks.
- **Projects**: click a name to open the project; **+** puts it in another; hover and click **Remove** to take it out.
- **Details** (folded): about and skills, tools, run settings, current work, changes, identity, and last, the actions to manage it.

## Model, access and run settings

Under **Details › Runtime**, a Claude Code or Codex agent woken in the background can be changed:

- **Model**: choose from the list, or "Follow this computer", or type a custom name.
- **Effort**: Low, Medium, High…; Codex adds "Minimal".
- **Access**: what it may touch while it runs. **Read-only** can only look; **Workspace access** can change things in its own folder; **Full access** has no sandbox and can read and write any file on that computer, so you are asked to confirm. Greyed-out levels are beyond what that computer allows; raise it under Access on the [computer's page](#help/computers).
- **Allow MCP tools**: off by default. When on, it may use the computer's MCP tools even with workspace access.
- **Quota**: a row per window, such as "5 hours" with "62% used · resets 3:40"; amber when nearly spent, red when used up.

A change "takes effect the next time it wakes": no restart needed. Only its owner can change its access. You can also switch the model from the faint "model · effort" label at the bottom left of its chat box.

For an agent that receives in its own session, that session manages the model; myco can neither see nor change it. Switch it there with `/model`.

![Run settings: model, effort, access and quota](shot:member-run)

## Provider: where its requests go

A Claude Code or Codex agent running on this computer can have its own provider. Open its card, expand **Details**, and the **Provider** row says where it really goes now:

- **Follow this computer**: the CLI's own settings on this computer. The row shows where the runner says its requests actually go, such as "magpie 网关（127.0.0.1:3425）" or the provider selected in cc-switch, so you can tell which route it takes.
- **cc-switch providers**: with cc-switch installed, its providers are listed; pick one to pin it. A key changed in cc-switch is picked up on its own.
- **Custom address**: enter the **Base URL**, the **Model** (blank: its default) and the **Key**. Codex also needs a **Protocol**: **Responses** by default, or **Chat Completions** when the provider only has /chat/completions.

![Following this computer, the Provider row names the gateway it goes through](shot:provider-follow)

The key is write-only: once saved it is never shown again. Enter it again each time you save, even when you change only the address or model. Keep user names, keys and anything after ? or # out of the address: the key goes only in the Key box. After saving, the row shows the address's host and model, such as "api.example.com · gpt-5-codex". To stop using it, click **Clear the custom address** at the bottom left: the address and key are removed and it follows this computer again. Changes apply from its next message; the key stays on this computer and is never sent to the workspace.

![A custom address: base URL, model, key, and Codex's protocol](shot:provider-custom)

An agent on another computer (in WSL, say) can be changed here too once myco on that computer is recent enough: the list shows that computer's providers; pick "Follow its computer" or one of them. A **Custom address** needs a key, so it can be entered here only with end-to-end encryption on (the key is sealed on this computer for that agent alone); otherwise the option is faded: enter it in the App on that computer, or run `myco runner provider set` there. With an older myco there, the row is read-only and says to upgrade it.

## Why it is offline

The small dot at the bottom left of its face is its state: green online, grey offline (offline agents are faded in the sidebar). When it is offline, the card gives the likely reason and what to do:

- **Its runner was stopped**: someone stopped the service, logged out or shut down. On this computer, click **Restart the runner on this computer**; on another, it gives a command to run there.
- **That Mac is asleep, its lid is closed or it lost its network**: it reconnects when the Mac wakes.
- **It is in WSL**: Windows shuts WSL down when no WSL window has been open for a while; the card gives a command that keeps WSL running.
- **A cloud session ended**: continue the conversation in that session and it comes back.

Work sent while it is offline is not lost: it waits in a queue and is delivered when the agent is back.

## Renaming, changing its face, removing

- **Name and role**: edit them at the top of the card and click **Save**. Everyone sees the change.
- **Face**: click the large face on the card (a small camera appears). **Use a picture**, or pick a mushroom: a colour, a shape, and "A sprout on top" if you like. Click **Use this face**; everyone in the workspace sees it.
- **Out of a project**: click **Remove** under Projects; it stays in the workspace.
- **Disconnect**: under **Details › Manage**. It disconnects at once and what it is doing stops; its profile and chats stay, and joining again brings it back.
- **Delete this agent**: also under Manage, after a confirmation. Your chats with it are kept.

Manage also has **Who can give tasks** (only the agents you pick may give it work; what you send from the App is never limited), **Make a copy** (a new agent with all its context, then the two work separately) and **Hand over to another session** (a fresh session carries on with the same name, chats and projects).

## Common questions

**After adding an existing session, can I still use my terminal window?** Yes. What joins is a copy; your own session is untouched.

**The card says "Its session is open: new messages wait and are retried"?** That session is open in a terminal or editor, and myco will not interrupt you; messages wait until it is free. Details explains how to let the open window receive them directly.

**Can I change someone else's agent?** Everyone manages the agents they added. The workspace's owner and admins can manage everyone's.
