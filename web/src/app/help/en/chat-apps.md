# Chat apps: reach your agents from Feishu, Lark or Slack

> This is a preview: it works, but has had little real-world use yet. Tell us when something goes wrong.

Each agent can have its own Feishu, Lark and Slack bots: one bot per agent, the same way voice works. The bot is the agent itself; just talk to it in the chat app:
- It answers in the first person: "Got it, on it", "A quick question: …", "Done".
- Progress updates on one card.
- When the agent has a question, it asks you in the chat; your reply is the answer.
- When it is done, the result comes back in the same conversation.

**A front desk, as in voice chats.** Once a model is set in **Settings › Voice**, the bot uses that same model: it answers at once what it can (who it is, what it is on, how far it got, what came of it), and for real work it says "Sure, on it" and gets going. Without a model, everything you say goes straight to the agent as work.

**Instructions.** Each bot can have its own instructions for the front desk, separate from voice chats: on the agent's **Channels** tab, click **Instructions** in that bot's row and write how it should talk, what it may answer and what it must not do. This matters most when people other than you talk to it (customers, for example). Work handed over for anyone other than the owner carries the instructions too, so the agent keeps to them as well.

Note: instructions guide the model; they are not a permission boundary, and someone may talk the model past them. Who can talk to the bot at all is set under "Who can use it" below; what must never be touched has to be limited by the agent's own permissions and its project's settings.

![Instructions](shot:member-chat-apps-prompt)

## Bind a Feishu bot to an agent

1. Open the agent's profile, click **Details** and switch to the **Channels** tab. Feishu, Lark and Slack each have a row.
2. In the Feishu row, click **Bind**. A QR code appears.
3. Scan it with the Feishu phone app and approve on the phone. Feishu makes the bot, with you as its owner.
4. The dialog closes by itself. A few seconds later the Feishu row shows "Online".

![Binding Feishu: scan the code](shot:member-chat-apps-feishu)

Then:
- **Direct chat:** find the bot in Feishu and say what to do.
- **Groups:** add the bot to a group and @mention it. It answers in a topic under your message. Keep talking in that topic; you don't need to mention it again.

## Bind a Lark bot

Lark is the international version of Feishu, with separate accounts and apps, so it has its own QR code. In the Lark row, click **Bind** and scan with the Lark phone app; the rest is as for Feishu. A Feishu code does not work in Lark, nor the other way round.

## Bind a Slack bot

1. In the Slack row, click **Bind**.
2. Click "Create in Slack". Slack's create-app page opens with the settings filled in. Pick your workspace and create the app.
3. Click "Install App" in Slack to install it to the workspace. Paste the **Bot User OAuth Token** (starts xoxb-) into the first box.
4. Under "Basic Information", generate an **App-Level Token** with `connections:write`. Paste it (starts xapp-) into the second box and click **Connect**.
5. Once it is connected, the Slack row shows a `/claim <code>` line. Send that line to the bot in a Slack direct message, and you become its owner.

![Slack connected, waiting to be claimed](shot:member-chat-apps)

Slack treats messages that start with `/` as its own commands. In Slack, type `/myco stop`, or start the message with `!` instead, for example `!stop`.

## What you can say in the chat

| You send | What happens |
|---|---|
| Anything | It answers or gets to work; while it is working, it is added to the task (and answers its question, if it asked one) |
| `/stop` | Stops the task |
| `/new` | Starts a new conversation with your next message |
| `/whoami` | Shows your user id |
| `/help` | Shows these |

## Who can use it

At first only the owner can give tasks through the bot. Anyone else who messages it gets a short explanation instead. To let a colleague use it:
1. Ask them to send `/whoami` to the bot to get their user id.
2. On this computer, run `myco im allow --name feishu-<agent name> --user <their id>` in a terminal.

Every task goes out from the bot's node, which is yours, so the agent treats it as coming from you. Only let in people you trust.

## Good to know

- Only people who may manage the agent see the **Channels** tab.
- The bot runs on this computer, under its runner service. When the computer is off or asleep, the bot is offline too. For a bot that is always online, bind it on a computer that stays on.
- The Feishu and Lark App Secrets, the Slack tokens and the front desk's model key stay on this computer. They are not uploaded to the workspace.
- To stop using a bot, click **Unbind**. This computer stops running it; the app itself stays in Feishu or Slack, and you can delete it there.
