# Rebuild 100 great apps with agents, free and open, always improving

**English** · [中文](open-apps.zh-CN.md) · [App list](apps.md) · [README](../README.md)

> We are using agents to rebuild 100 apps people rely on every day, and we are making all of them free and open, and we will keep improving them.
> These apps no longer split people into those who can pay and those who can't, or split users into people and agents.

## Why now

Building a good app used to take a team months or years. So it was expensive, so it charged money, so the good stuff went first to the people who could pay.

Agents change that. Today one person working with a few agents can build the core of a mature app, and it gets faster every month. We are entering an age of wishes: say what you want, and an agent builds it.

When building an app costs close to nothing, keeping it behind a paywall is fighting the tide. We would rather not stand in its way. We'd rather take a more open stance: **if it is this cheap to build, let everyone use it.**

## Two walls we want to take down

### The paywall

The best parts of many apps sit behind subscriptions and memberships: good templates, smooth editing, real collaboration, smart organization. None of it is especially hard. It is just expensive.

We want **equal access to good tools and to knowledge**:

- Students, freelancers, small teams and people just starting out get the same tools as people at big companies.
- What someone in a small town can do shouldn't be limited by a few subscriptions they can't afford.
- How the apps are built is open too: the code is open source, so anyone can learn from it, change it and use it.

### The data wall

Your documents, spreadsheets, notes, accounts and meeting records are yours. Yet they are often locked inside each app: your agent can't see them, you can't get them out, and switching tools means starting over.

With agents, that wall costs more than ever: however capable your agent is, it can't help with data it can't reach.

So every app we build is **made for agents from the start**:

- You and your agent work on the same page. The agent fills it in or edits it, and you see the change live.
- **With your permission**, your agent can read and write what's in these apps. Without it, no one can.
- Your data stays on your own computer or your own hub by default, and you can export it in common formats at any time (Markdown, Excel, CSV, MP4, PowerPoint and more).

## What we stand by

1. **Free and open.** The apps are free to use and the code is open source. No membership walls, and no selling your data.
2. **Your data is yours.** Local first. Recordings, photos and videos stay on the computer that opened them. You can export them and take them with you.
3. **People and agents are equal users.** Every app is designed for both from the start, not given an API as an afterthought.
4. **Access only with permission.** You decide what an agent may do, and you can take it back at any time.
5. **We learn from the experience; we don't copy.** We study what makes great apps good and build our own from scratch. We don't copy anyone's code, assets or trademarks, and we don't crack or get around anyone's payment or access controls.
6. **Always improving.** We don't ship a version and walk away. Every app keeps getting better from your feedback.

## What this is not

- **Not piracy or cracking.** We don't touch other people's software. We use agents to rebuild a similar experience from scratch and open-source it.
- **Not against people making money from apps.** Good products deserve to be paid for. We just believe that in the age of agents, "good enough to use every day" should be a public good everyone has. On top of that, anyone can still build better, more specialized things worth paying for.
- **Not a closed platform.** The apps run on your own computer and your own hub, the code is on GitHub, and anyone can change it or deploy it.

## How we pick the 100

We ask three questions:

1. **Do many people use it?** The kind of app you open every day or every week.
2. **Is it hard to get?** Does the good version need a payment, a membership or an expensive subscription?
3. **Do agents help a lot?** Does working with an agent make it clearly better?

**26** apps are available today, across creative design, office, development, productivity, everyday life and games. See the full list in [Apps](apps.md). We want to decide the rest together with you.

## Progress

| | |
|---|---|
| Available | 26 / 100 |
| Categories | Creative design 7 · Office 5 · Development 3 · Productivity 6 · Everyday life 3 · Games 2 |
| Next batch | Nominated and voted on by you in [Discussions](https://github.com/myco-ai/myco/discussions) |

## How to take part

### 1. Install myco and start using the apps

| Platform | Download |
|---|---|
| **Mac** | [myco.dmg](https://github.com/myco-ai/myco/releases/latest/download/myco.dmg): one download for Apple silicon and Intel, signed and notarized by Apple, updates itself |
| **Windows** (preview) | [myco-app-windows-amd64.exe](https://github.com/myco-ai/myco/releases/latest/download/myco-app-windows-amd64.exe) ([ARM](https://github.com/myco-ai/myco/releases/latest/download/myco-app-windows-arm64.exe)). Not code-signed yet: on first run choose *More info → Run anyway* |
| **Linux** | [myco-app-linux-amd64](https://github.com/myco-ai/myco/releases/latest/download/myco-app-linux-amd64) ([ARM](https://github.com/myco-ai/myco/releases/latest/download/myco-app-linux-arm64)), opens the app in your browser |

Open the app, follow the first screen to start a hub on this computer, and every app is under **Apps** in the sidebar.

Already using Claude Code, Codex or another agent that can run commands? Send it this and it will set things up for you:

```text
Set up myco for me: read https://raw.githubusercontent.com/myco-ai/myco/main/docs/agent-setup.md (if it doesn't open, read https://cdn.jsdelivr.net/gh/myco-ai/myco@main/docs/agent-setup.md) and follow it. Ask me before anything I need to decide, and before using sudo or changing system settings. Tell me the result when you're done.
```

More ways to install are in the [install guide](../INSTALL.md) and the [tutorial](tutorial.md).

### 2. Nominate the next app

Start a post in [Discussions](https://github.com/myco-ai/myco/discussions) and cover three things:

- **Which app**, and what you use it for.
- **What makes it good**: the two or three features you can't do without.
- **What makes it hard to get**: how much it costs, and which feature is locked.

Upvote other people's nominations. The ones with the most votes get built first.

### 3. Tell us what's wrong

Open an issue in [Issues](https://github.com/myco-ai/myco/issues). Include the app's name, the output of `myco version`, your OS, and how to reproduce it; a screenshot helps. Security problems go through [SECURITY.md](../SECURITY.md), not a public issue.

### 4. Build an app yourself

App development lives in [myco-ai/apps](https://github.com/myco-ai/apps), a separate repository with its own CI and releases. Make app changes in that repository; the host embeds a verified release pinned by `apps.lock.json` ([build instructions](building.md)).

Each app is a folder under `templates/src/`: a React page plus a few files in `public/`.

| File | What it does |
|---|---|
| `index.html`, `main.tsx`, `*.css` | The page itself |
| `public/myco.json` | The app's manifest: id, name, icon, category, description, and the data it stores with each field's type |
| `public/agent.md` | Tells agents how to use the app and fill in its data |
| `public/sample.json` | Sample data, used for screenshots and tests |
| `public/locales.json` | English and Chinese text |
| `public/shots/` | Screenshots for the app list (one light, one dark) |

The page reads and writes its data with `myco.data.*` (see the [HTTP API](api.md)) and loads nothing from the internet. An app that needs the computer itself (files, commands, the screen) uses [`myco.os`](app-os.md). The UI rules are in [docs/design/app-ui.md](design/app-ui.md).

**The quickest start:** copy an existing app, such as `templates/src/todo` (To-do list).

```bash
git clone https://github.com/myco-ai/apps && cd apps
cd templates && npm ci
cp -r src/todo src/my-app        # start from an existing app, then change the id, name and fields in public/myco.json
npm run build                    # type-check and build into dist/official/
node shots.mjs my-app && node build.mjs   # take the screenshots
cd .. && make check              # run every check before you commit
```

**Let your agent build it:** open Claude Code or Codex in the repository and tell it:

```text
Read AGENTS.md and templates/README.md. Using templates/src/todo as a reference, build a new myco app under templates/src/: <the app you want and its core features>. Follow the linked myco App UI guide in AGENTS.md for the UI, use myco.data for data, write agent.md and sample.json, and get npm run build, the screenshots and make check passing.
```

When it's ready, open a pull request in [myco-ai/apps](https://github.com/myco-ai/apps): one change per pull request, run `make pack`, and leave generated `dist/` out of Git. Read that repository's AGENTS.md for checks and host compatibility. For a bigger app, open an issue or post in Discussions first so no one duplicates the work.

All contributions are open under the [MIT License](../LICENSE).

### 5. Spread the word

Star the [repository](https://github.com/myco-ai/myco), and share your favourite app with a friend who needs it.

---

In the age of agents, good tools shouldn't belong to a few. Let's open up 100 great apps to everyone, together.
