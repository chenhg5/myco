# Chat apps: talk to your agents from Feishu/Lark or Slack

A **bridge** is a bot in a chat app that you talk to like one of your agents:

- What you write to it becomes a task for an agent: the bridge's default one, or another you name with `/to <name>`.
- The agent's progress updates one status message in place.
- When the agent asks a question, the question appears in the chat, and your reply answers it.
- Its answer comes back as a new message in the same conversation.
- While a task is open, what you write is added to it (a steer). Once it is closed, your next message becomes a new task in the same myco thread, so the agent keeps its context.

The bot speaks as a node of yours on the hub, `im-<name>`, which receives through its inbox. In the App, its tasks show up like any other conversation, sent by that node.

Bridges connect out to the chat app, so the machine needs no public address:

- Feishu/Lark: the long connection.
- Slack: Socket Mode.

The machine's runner service, the one a join link installs, runs the bridges configured on it. A bridge starts within 30 seconds of being connected.

## Feishu / Lark: scan a QR code

On a machine that has joined your myco network:

```bash
myco im connect feishu --agent coder
```

1. The command prints a QR code. Pass `--qr-png qr.png` to also save it as an image.
2. Scan it with the Feishu or Lark phone app and approve. Feishu creates a personal bot for you, with its permissions and events already set up.
3. The command then:
   - saves the bot's credentials in `~/.myco/im/feishu.json` (mode 0600);
   - makes you the bot's owner, the only person it takes tasks from;
   - creates the `im-feishu` node on the hub.

Lark tenants are detected from the scan; `myco im connect lark` works the same way.

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
- **Commands:** Slack takes `/…` for its own commands. Use `/myco agent coder`, or start a message with `!`, for example `!stop`.

## Commands in the chat

| Command | What it does |
|---|---|
| *(plain text)* | A task for the chat's default agent, or an addition to the open task (which also answers its question) |
| `/to <agent> <text>` | This once, send it to another agent |
| `/agent <agent>` | Change this chat's default agent |
| `/agents` | Agents online |
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
myco im allow --name feishu --user ou_123     # --remove to take it back; * for everyone
```

People who aren't allowed get a one-line answer saying how to be let in. Nothing is sent to an agent for them.

Every task goes out from the bridge's node, which belongs to you. The task's text ends with who wrote it in the chat app, for example `（经飞书转述，来自 glenn）` or `(relayed from Slack, by U0123)`. Agents therefore treat it as coming from you: grant access to people you trust.

Other agents' messages to the bridge's node (`im-feishu`) are forwarded to the owner in a direct message.

## Managing bridges

```bash
myco im list                                   # bridges on this machine (no secrets shown)
myco im set --name feishu --agent reviewer     # default agent, --project, --lang zh|en
myco im serve [--name feishu]                  # run in the foreground (without the runner service)
myco im remove --name feishu                   # remove it here; the app and the hub node stay
```

A running bridge reads `allow` and `set` changes when it restarts.

Secrets are kept only on this machine, in `~/.myco/im/<name>.json` (0600), and never printed or logged:

- the Feishu App Secret;
- the Slack tokens.

Conversation state, meaning which chat maps to which myco thread and task, is kept in `~/.myco/im/<name>.state.json`.
