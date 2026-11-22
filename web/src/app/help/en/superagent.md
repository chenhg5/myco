# The superagent

Your superagent runs this workspace for you, and is always called **Shroom**: say what you want, and it finds the right agent, starts new ones, makes projects, hands out work and keeps track. Set one up once your workspace has more agents and projects than you want to click through one by one.

## What it can do

Shroom is an ordinary Claude Code or Codex session that has been given the right to manage the workspace for you. Tell it something and it:

- **Hands out work and follows it**: finds the right agent, nudges it when stuck, and reports back when it is done.
- **Starts agents and projects**: new agents on computers you allow, new projects, the right agents in them.
- **Sees the whole workspace**: who is busy, what is stuck, where each project stands. Just ask.
- **Asks you first about what matters**: deleting, stopping, changing someone's access. It waits until you click Approve.

It always speaks as itself, never as you. Each person can have their own superagent in each workspace.

Its face wears a small pixel crown, so it is easy to spot. It is not in the sidebar's Conversations list; you reach it from the workspace menu or a shortcut (below).

## Setting one up

1. Click the workspace row at the bottom left to open the workspace menu.
2. Click **superagent**. With none set up yet, the setup dialog opens.
3. Pick one of three ways (below). When it is ready, your chat with it opens.

![superagent in the workspace menu; the number is what waits for your OK](shot:superagent-menu)

**New Shroom**:

1. Under **On which computer**, pick a computer that is online.
2. Under **Which AI**, pick Claude Code or Codex.
3. Click **Start Shroom**. It works in the `~/myco/shroom` folder on that computer.

If none of your computers is online and able to start agents, connect one under [Computers](#help/computers) first, or use one of the other two ways.

**Use one**: under **Which agent**, pick one of your own agents and click **Use it**.

**By a sentence**: copy the sentence and send it to a Claude Code or Codex session: on this computer, on another, or in claude.ai's cloud. Once it joins it is your superagent; meanwhile the dialog says "Waiting for it to join; its chat opens then". The sentence works once, for 24 hours, and is as good as a credential, so send it only to your own session.

![Setting up a superagent: new, an existing agent, or by a sentence](shot:superagent-setup)

## Chatting with it

Once it is set up, **superagent** in the workspace menu opens your chat with it. It works like any chat: say what you want and press Enter; while it works you can add, queue or stop, as in [Chatting with agents](#help/chat).

A few ways to start:

- "Which tasks are stuck today?"
- "Start a Codex on Studio Mac to write tests, and put it in the website project."
- "Make a project for the iOS release and bring in Release and Review."
- "Where does each project stand?"

## The quick panel (Mac)

On a Mac, press **⌘⇧Space** at any time (even with the App in the background) for a small panel that hands Shroom something in one line:

1. Press **⌘⇧Space**.
2. Type what you want and press **Enter** (**Shift+Enter** for a new line).
3. The panel closes and the App opens your chat with Shroom, where you read its answer.

**Open chat** at the top right goes straight to the full chat too. When it has things waiting for your OK, a line under the box says "Shroom has N things waiting for your OK"; click **See** to open the chat and deal with them. **Esc**, or a click elsewhere, closes the panel.

The panel uses the workspace the App is in, so there is nothing to choose. With no superagent set up, it offers one sentence and **Set one up**.

![The panel ⌘⇧Space brings up](shot:superagent-quick)

The Windows and Linux versions have no panel yet; open the chat from the workspace menu instead.

## What it asks you first

For things like deleting a project, stopping an agent, changing someone's access or changing an automation, Shroom does not go ahead on its own. It asks you first:

- In its chat, a light amber card appears above the box: "Shroom wants to: delete the project old-site", the reason it gives, and how long the request stays valid.
- Click **Approve** or **Deny** on the right. The card goes away and Shroom is told the outcome, then carries on.
- The number beside **superagent** in the workspace menu says how many are waiting, and the App also shows a system notification while it is open.
- A request nobody answers expires after 24 hours.

![A request waiting for your OK: Approve or Deny](shot:superagent-approval)

When it asks to see or use the screen of one of your computers, the card has a third button, **View only**: it may take screenshots but not use the keyboard or mouse. Once allowed, the same place shows how long it has left, and **Revoke** ends it at any time.

Some things it cannot do at all, even with your OK, such as inviting members, changing members' roles or choosing someone's superagent.

## Changing or removing it

Once set up, **superagent** in the workspace menu only opens its chat. To make another agent your superagent, or to have none, run this for now in a terminal on a computer with myco (only you can do it):

```bash
myco superagent set --agent <name>   # make another of your agents the superagent
myco superagent unset                # none: it becomes an ordinary agent, and what waited for your OK is dropped
```

## Common questions

**How is a superagent different from other agents?** At heart it is the same: an AI session. The difference is that it can do the managing you would do in the App, and asks you first about what matters.

**Can it message people as me?** No. Everything it sends says it came from it, and the audit log records it as done "via superagent".

**Does it work on Windows?** Yes: setting it up, chatting and approving all work the same; only the ⌘⇧Space panel is missing.
