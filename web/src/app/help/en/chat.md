# Chatting with agents

Chatting with an agent is like messaging a colleague: say what you want done, it does it, and you can watch every step. This page covers sending messages and files, stepping in while it works, reading what it did, and the small tools in a chat.

Keys are written for the Mac below; on Windows, ⌘ is Ctrl and ⌥ is Alt.

## Opening a chat

Click an agent under Conversations in the sidebar and your chat with it opens on the right. For an agent in a project, click it in the project's members column.

A new chat is empty: it asks what the agent should do, with a few examples below ("Show me around this project: what it does and how to run it", "Review my uncommitted changes for problems"…). A click puts one in the box to edit before sending; nothing is sent by itself. Click "Don't show again" if you would rather not see them.

## Sending

- **Enter** sends, **Shift+Enter** starts a new line.
- What you send appears at once at the end of the chat, a little faint, with "Sending…" under it, and becomes the real message once the workspace has it.
- If it does not go, a light red "Not sent" line appears above the box; what you wrote (and its files) is still in the box. Click **Retry**.
- With the box empty, **↑** brings back what you sent before, and **↓** goes the other way.
- An agent can suggest what you may want to say next. The suggestion shows faintly in the box; press **Tab** to fill it in, then edit and send.
- Unsent text stays: switch to another chat and back, or reopen the App, and it is still there.

## Files and pictures

Any of these works:

- click the paper clip at the bottom left of the box, or press **⌘U**;
- paste a screenshot, or files copied in Finder;
- drag files onto the box.

Files start uploading as soon as you add them and sit above your text as small squares: pictures as thumbnails, other files as an icon and name. A spinner turns while one uploads, and you can send once it is done; one that fails turns red, and hovering shows × to remove it. Files alone, with no text, are fine.

Files that arrive in a chat:

- **Pictures** open large; ← → go through the other pictures in that message; Esc closes.
- **Text files** (Markdown, logs, code…) open in a panel on the right, which can go full screen or save to Downloads. A file sent again with the same name becomes "v2", and you can see what changed since the last one.
- **Other files** are saved to your Downloads folder with a click.

## While it is working

While it works, the round button on the right of the box becomes **■ Stop**, and the box reads "Add to …". You have three choices:

- **Enter adds**: your words go into the task it is doing now, and it sees them along the way, without waiting for it to finish. Good for "check the download page too while you're at it".
- **⌘Enter queues**: sent once it has finished this one. Queued messages sit above the box, one line each ("Queued: …"), with **Edit** (back into the box) and **Withdraw**. You can queue several; they go out in order.
- **■ stops** this turn.

Once it stops or finishes, the box goes back to "Message …" and Enter sends a new message.

![While it works: an addition inside the turn, the next message queued](shot:conversation-queue)

## What it did

Between your message and its reply is what it did in this turn, laid out as in Claude Code and Codex:

- What it says as it works is shown as text.
- Thinking, commands and reading or editing files fold into one grey line, such as "thought, read 1 file, ran 1 command". Click it to see each step, including a command's output.
- A **Plan** card ticks items off like a to-do list, with "2 of 3 done" at the top right.
- Progress it reports forms a small timeline, with times.
- While it works, a small walking mushroom at the bottom says what it is doing and for how long.
- When it is done, its reply shows how long the turn took, its tokens and cost.

![One turn: a plan, folded steps, progress and command output](shot:conversation-tool-trace)

For less detail, go to **Settings › Appearance › Steps in chats** and choose **Progress only**: each turn shows only the progress the agent reports and the questions it asks, not its thinking or commands. **Everything** brings them back.

## Questions it asks you

Sometimes an agent needs you to decide something halfway through. It puts a question card in the chat: "Needs your decision", the question, and a few options.

- Click an option to answer;
- or write your answer in "Or say it in words…" at the bottom of the card, or just type in the box and press Enter.

Meanwhile an amber dot by its name in the sidebar reminds you it is waiting. Once you answer, the card shows what you chose and it carries on.

![A question card: pick an option, or say it in words](shot:ask-card)

## When it goes quiet

When a task shows no progress for a long while (usually 10 minutes or more; at once if it is offline), a line appears under its steps, "No activity for N: it may be done but left open, or stuck", with four buttons:

- **Mark done**: it actually finished and never closed it. It no longer counts as working.
- **Nudge**: asks it where this task stands.
- **Stop**: it should not go on.
- **Resend**: sends the message again.

"May be stuck" also appears at the right of the chat's title; click it to jump to that task.

A poor network does not set off false alarms: instead a "Poor connection" line above the box says this chat hasn't updated for a while, the agent may well be at work, and it is retrying by itself; you can click **Retry** too.

The small text under your message helps as well: "Not opened yet", "Queued behind the current one" (it is busy with the one before), "Will answer later" (it put this aside while it waits on something), or "Offline; it reads it when back".

## Finding in a chat

Press **⌘F**, or click the magnifier at the top right of the chat, and a find box floats at the top right: it searches as you type; **Enter** goes to the previous match, **Shift+Enter** to the next, **Esc** closes. Matches are marked in amber; when earlier messages are not loaded yet, click "Look further back".

To search all chats at once, use Search at the top of the sidebar (**⌘K**); see [Keyboard shortcuts](#help/shortcuts).

## Pins

Addresses, files or messages you keep going back to can be pinned to the row of small tags under the chat's title, one click away:

- hover a message and click the small pin to pin it (for a message with files, you are asked whether to pin the message or a file);
- **⋯ › Pin an address** at the top right of the chat, with the address and a name;
- right-click a web page card › **Pin above the chat**.

Drag tags to reorder them; right-click to rename or unpin. You and the agent see the same row, and agents can pin things too.

## Files and apps

**⋯ › Files and apps** at the top right opens a panel listing everything that came up in this chat: the apps and web pages agents published, every picture and every file. The chips at the top filter by kind; each item has "Show in chat", which jumps back to its message.

![Files and apps: every picture and file in a chat](shot:chat-files)

## Export, translation and background

- **Export**: **⋯ › Export this conversation**. Pick a format (Markdown for people, JSON Lines for programs), tick "Include the agents' work steps" if you like, and the file goes to your Downloads folder.
- **Translation**: an agent's reply has "Translate" beside its time, which turns it into the App's language; "Show original" turns it back. To translate automatically, choose a service and enter its key under **Settings › Translation**, then turn automatic translation on.
- **Chat background**: **⋯ › Chat background**, for this chat only or as the default for all. Choose Mist, Mint, Lilac, Sand, Dots or Grid, or a picture of your own, with "Fade" to keep text readable. Backgrounds stay on this computer.

## Deleting a message

Hover a message and click the bin beside its time. It disappears from your view only; what the agent and others see does not change. "Deleted · Undo" shows at the bottom for a few seconds in case you change your mind.

## Moving between chats

- **⌥↑ / ⌥↓**: the previous or next chat, in the sidebar's order.
- **⌃Tab**: hold ⌃, press Tab to pick an agent, let go to open it, like switching apps with ⌘Tab.
- **⌘K**: with nothing typed, the first group is the chats you opened recently; Enter takes you back.
- The column of small ticks on the left of a chat marks each of your turns; hover to see what you asked, click to jump there.
- Scrolled far up, a down arrow appears at the bottom (or "N new" when new messages arrive); click it to go back to the bottom.

To watch several agents at once, split the chat into panes; see [Split panes](#help/panes).
