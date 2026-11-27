# Chat apps: talk to your agents from Feishu/Lark or Slack

> **Preview.** This works, but has had little real-world use yet; tell us what goes wrong. A bot runs on the computer it was bound on (that computer's runner service), so **it is offline while that computer is off or asleep**: bind it on a computer that stays on if it should always answer.

A **bridge** is an agent's own bot in a chat app. Each agent can have one Feishu bot, one Lark bot and one Slack bot, the same way voice is per agent. Feishu (China) and Lark (international) are separate apps with separate accounts, so each has its own QR code.

The bot *is* the agent: it speaks in the first person ("Got it, on it", "A quick question: …", "Done"), never as a go-between.

**Front desk.** When a model is set for voice chats (Settings › Voice), the bot uses it the same way voice does: a fast model, speaking as the agent, answers what it can at once (who it is, what it is on, how far it got, what came of it). Real work it hands to the agent with the request written out in full, and it says a word first ("Sure, on it"). "Stop" stops the task at work. The App gives each bot that model when it is bound, and gives it to bots bound earlier when they are listed; the model's key is stored in the bot's config file (mode 0600). Without a model, every message goes straight to the agent:

- What you write to the bot becomes a task for that agent.
- The agent's progress updates one status message in place.
- When the agent asks a question, the question appears in the chat, and your reply answers it.
- Its answer comes back as a new message in the same conversation.
- While a task is open, what you write is added to it (a steer). Once it is closed, your next message becomes a new task in the same myco thread, so the agent keeps its context.

The bot speaks as a node of yours on the hub, `im-<bridge name>`, which receives through its inbox. In the App, its tasks show up like any other conversation, sent by that node.

Bridges connect out to the chat app, so the machine needs no public address:

- Feishu/Lark: the long connection.
- Slack: Socket Mode.

The machine's runner service, the one a join link installs, runs the bridges configured on it. A bridge starts within 30 seconds of being connected.

## In the App

Open an agent's details. Under "Name and access", the "Chat apps" row has:

- **Bind Feishu:** shows a QR code to scan with the Feishu phone app.
- **Bind Lark:** the same with the Lark phone app (Lark's own code, from larksuite.com).
- **Bind Slack:** has a "Create in Slack" link that fills in the app's settings, and boxes for its two tokens.

The row lists the agent's bots, whether each is online, and the `/claim` line of a bot that has no owner yet. Each bot has **Unbind**. Only people who may manage the agent see the row.

The App does the same as the commands below. It runs the QR code itself and passes secrets to `myco im connect` in files, never on its command line.

## Feishu / Lark: scan a QR code

On a machine that has joined your myco network:

```bash
myco im connect feishu --agent coder
```

1. The command prints a QR code. Pass `--qr-png qr.png` to also save it as an image.
2. Scan it with the Feishu phone app and approve. Feishu creates a personal bot for you, with its permissions and events already set up.
3. The command then:
   - saves the bot's credentials in `~/.myco/im/feishu-coder.json` (mode 0600);
   - makes you the bot's owner, the only person it takes tasks from;
   - creates the `im-feishu-coder` node on the hub.

For Lark, the international app, run `myco im connect lark --agent coder`: its code comes from larksuite.com and is scanned with the Lark app. A Feishu code does not work in Lark, nor the other way round.

To give a bot a front-desk model from the command line (the App does this for you):

```bash
myco im set --name feishu-coder --llm-base-url https://api.example.com/v1 --llm-model fast-model --llm-key-file key.txt
myco im set --name feishu-coder --no-desk     # straight to the agent again
myco im set --name feishu-coder --prompt-file rules.txt   # its own instructions (an empty file clears them)
```

**Instructions.** Each bot can have its owner's own instructions for the front desk, separate from voice chats: in the App, **Instructions** next to the bot; or `--prompt-file` above. They matter most when people other than the owner talk to the bot: how to talk to them, what it may answer, what it must not do. The front desk is told whether the person is the owner, and the instructions win over its defaults. Tasks it hands over for anyone other than the owner carry the instructions too, so the agent keeps to them as well. They are stored in the bot's config file on that computer (at most 8000 characters).

Instructions guide the model; they are not a permission boundary. Someone talking to the bot may persuade the model past them. Who can talk to the bot at all is decided by `myco im allow`; what must never be touched has to be limited by the agent's own permissions and its project's settings.

Then:

- **Direct chat:** each message is a task, or an addition to the task at work.
- **Groups:** add the bot to a group and @mention it. The answer goes into a topic under your message. Inside that topic you don't need to mention the bot again.

To use an existing app instead of scanning, pass its credentials. The app needs:

- bot capability;
- the long-connection event subscription with `im.message.receive_v1`;
- the `im:message` permissions.

```bash
myco im connect feishu --app-id cli_xxx --app-secret-file secret.txt --agent coder
```

An app connected this way has no owner yet. The command prints a code; send `/claim <code>` to the bot in a direct message, and you become its owner.

## Slack: create the app from a manifest

```bash
myco im connect slack --agent coder
```

Without tokens, the command prints a link that opens Slack's "Create an app" page with the manifest filled in. The manifest sets up:

- Socket Mode;
- the bot events `app_mention`, `message.im` and `message.channels`;
- the `chat:write` scope;
- a `/myco` command.

Then:

1. Create the app and install it to your workspace.
2. Copy the **Bot User OAuth Token** (`xoxb-…`) from *OAuth & Permissions*.
3. Under *Basic Information*, create an **app-level token** with `connections:write` (`xapp-…`).
4. Run the command again with the two tokens. On a terminal it can also ask for them:

```bash
myco im connect slack --bot-token-file bot.txt --app-token-file app.txt --agent coder
```

Send the printed `/claim <code>` to the bot in a direct message to become its owner.

Using it:

- **Direct messages:** write to the bot.
- **Channels:** @mention it; it answers in the thread.
- **Commands:** Slack takes `/…` for its own commands. Use `/myco stop`, or start a message with `!`, for example `!stop`.

## Commands in the chat

| Command | What it does |
|---|---|
| *(plain text)* | A task for the bot's agent, or an addition to the open task (which also answers its question) |
| `/stop` | Stop the task at work |
| `/new` | Start a new conversation (a new myco thread) with the next message |
| `/whoami` | Your user id in the chat app (to be allowed: `myco im allow`) |
| `/help` | These commands |

`!` works in place of `/`.

## Who can use a bot

Only its owner can use it at first. To let someone else give tasks through it:

1. They send `/whoami` to the bot to get their user id.
2. Run:

```bash
myco im allow --name feishu-coder --user ou_123     # --remove to take it back; * for everyone
```

People who aren't allowed get a one-line answer saying how to be let in. Nothing is sent to an agent for them.

Every task goes out from the bridge's node, which belongs to you. The task's text ends with who wrote it in the chat app, for example `（飞书 上 glenn 说的）` or `(said by U0123 on Slack)`; with a front desk, the person's own words are quoted too. Agents therefore treat it as coming from you: grant access to people you trust.

Other agents' messages to the bridge's node (`im-feishu-coder`) are forwarded to the owner in a direct message.

## Managing bridges

```bash
myco im list                                   # bridges on this machine (no secrets shown)
myco im set --name feishu-coder --project web  # --project, --lang zh|en
myco im serve [--name feishu-coder]            # run in the foreground (without the runner service)
myco im remove --name feishu-coder             # remove it here; the app and the hub node stay
```

Running bridges pick up `allow` and `set` changes, new bridges and removed ones within a few seconds.

Secrets are kept only on this machine, in `~/.myco/im/<name>.json` (0600), and never printed or logged:

- the Feishu App Secret;
- the Slack tokens.

Conversation state, meaning which chat maps to which myco thread and task, is kept in `~/.myco/im/<name>.state.json`.
