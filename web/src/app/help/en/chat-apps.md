# Chat apps: reach your agents from Feishu or Slack

You can hand out work without being at your computer. Send a bot a message in Feishu or Slack, and it goes to the agent you chose:
- Progress updates on one card.
- When the agent has a question, it asks you in the chat; your reply is the answer.
- When the agent is done, the result comes back in the same conversation.

## Connect Feishu

1. Open **Settings › Chat apps**.
2. Pick an agent under "Which agent gets the messages".
3. On the "Feishu / Lark" row, click **Connect**. A QR code appears.
4. Scan it with the Feishu or Lark phone app and approve on the phone. Feishu makes a bot for you, with you as its owner.
5. After a few seconds, "Feishu · Online" appears under "Connected".

![Chat apps settings with a Feishu QR code](shot:settings-chat-apps)

You can use it right away:
- **Direct chat:** find the bot in Feishu and say what to do.
- **Groups:** add the bot to a group and @mention it. It answers in a topic under your message. Keep talking in that topic; you don't need to mention it again.

## Connect Slack

1. On the "Slack" row, click **Connect**.
2. Click "Create in Slack". Slack's create-app page opens with the settings filled in. Pick your workspace and create the app.
3. Click "Install App" in Slack to install it to the workspace. Paste the **Bot User OAuth Token** (starts xoxb-) into the first box.
4. Under "Basic Information", generate an **App-Level Token** with `connections:write`. Paste it (starts xapp-) into the second box and click **Connect**.
5. Once it is connected, the row shows a `/claim <code>` line. Send that line to the bot in a Slack direct message, and you become its owner.

![Slack connected, waiting to be claimed](shot:settings-chat-apps-slack)

Slack treats messages that start with `/` as its own commands. So in Slack, type commands like `/myco agent name`, or start them with `!` instead, for example `!stop`.

## What you can say in the chat

| You send | What happens |
|---|---|
| What to do | Goes to the default agent; while it is working, it is added to its task (and answers its question, if it asked one) |
| `/to name what to do` | Sends it to another agent, this once |
| `/agent name` | Changes this chat's default agent |
| `/agents` | Shows who is online |
| `/stop` | Stops the task |
| `/new` | Starts a new conversation with your next message |
| `/whoami` | Shows your user id |

## Who can use it

At first only the owner can give tasks through the bot. Anyone else who messages it gets a short explanation instead. To let a colleague use it:
1. Ask them to send `/whoami` to the bot to get their user id.
2. On this computer, run `myco im allow --name feishu --user <their id>` in a terminal.

Every task goes out from the bot's node, which is yours, so agents treat it as coming from you. Only let in people you trust.

## Good to know

- The bot runs on this computer, under its runner service. When the computer is off or asleep, the bot is offline too. For a bot that is always online, connect it on a computer or server that stays on.
- The Feishu App Secret and the Slack tokens stay on this computer. They are not uploaded to the workspace.
- To change the default agent, use the dropdown on the bot's row under "Connected". To stop using a bot, click **Disconnect**. That removes the bot from this computer only; the app itself stays in Feishu or Slack, and you can delete it there.
