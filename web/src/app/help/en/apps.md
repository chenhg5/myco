# Apps

Apps are web tools you and your agents use together: tables, databases, documents, slides, an issue tracker, a whiteboard, plus a photo editor, a video editor, Remote Desktop and a few games. You click and edit in the App, agents read and write the same content with commands, and changes show up on both sides as they happen. Reach for an app when you want an agent to put its results in a table or a document, or to hand you a page you can click through.

## The app library

Click **Apps** at the top of the sidebar.

- At the top right are the search box, "Find an app", and a refresh button that checks the official apps for new versions.
- **Made recently**: the ones you made lately; click one to open it. Ones with a new version carry a small dot. Click **All** to see more.
- A row of shelves: **All / Office / Development / Tools / Life / Creative / Games / This workspace's**. Below, the apps on each shelf, each with its name and two lines about it.

![The app library: Made recently on top, the shelves below](shot:templates)

## What there is

- **Office**:
  - **Table**: fill it in together, like a shared spreadsheet. Columns have types (single-choice tags, dates, people…); select and copy, paste from Excel, drag to fill, filter, sort, search, and export to Excel or CSV.
  - **Database**: the same records as a table, a board, a calendar, a gallery or a form; click one to open it on the right.
  - **Document**: "/" inserts headings, lists, to-dos, tables and code blocks; export to Markdown or a web page.
  - **Slides**: an agent writes the content and the page lays it out; click anything to edit it, present full screen, and export a PowerPoint you can keep editing.
  - **Meeting notes**: record an online meeting, then turn it into text on this computer, with each speaker told apart.
- **Development**: Code editor, Issue tracker (issues and projects, Linear-style), Developer toolbox.
- **Tools**: To-do list, Notes, Disk cleanup, Remote Desktop, Proxy console, Stock watch.
- **Life**: Budget book, Habit tracker, Cinema.
- **Creative**: Whiteboard, Mind map, Design studio, Photo editor, Video editor, Motion video, Recording studio.
- **Games**: Gomoku and Sudoku, to play with colleagues or agents.

Not sure which one fits? Read the two lines on each card, or search for what you want to do.

## Making one

1. Click an app's card.
2. The dialog opens with its screenshots (arrows on the sides when there are several).
3. **Name**: give it a name, such as "Release tracker".
4. **Where**:
   - **No project**: only you and your agents can see it.
   - A project: everyone in it, people and agents, can use it.
5. Click **Make it**. It opens straight away in the panel on the right.

The first time you make a given app in this workspace, it comes with some sample content to show how it is used; delete what you don't need. Later ones of the same app start empty. Every app works empty.

![A table open in the panel on the right, next to the app library](shot:template-grid)

## Using the panel

Apps and the pages agents publish open in the same panel: beside the chat in a wide window, on top of it in a narrow one.

- At the top of the panel: the name, its version, who published it and how long ago; on the right, reload, copy link, full screen and close.
- Drag the panel's left edge to make it wider.
- Opening another one adds a tab in the panel, up to 8; opening more closes the oldest. Clicking one that is already open switches to its tab without reloading it, and switching away and back keeps anything half filled in.
- To get the panel out of the way, click **Tuck away** at its top; the pages stay. "Pages N" stays at the far right of the top bar; click it to bring them back. Only × closes them all.
- When a page has a newer version, a bar at the top says "A new version: version N"; it changes only when you click **Show it**, so what you are reading never moves by itself.
- When an official app has a new version, the panel says so and that your data stays the same; click **Upgrade**.

Apps follow the App's language and light or dark appearance. Your content and the names you chose are never translated.

## Letting agents use them

Agents can read and write the apps you make: one in a project can be used by every agent in that project; one in no project, by your own agents. Just say it in a chat:

- "Put today's progress in the Release tracker, one row per person."
- "Write up what we just agreed in that document."
- "Look at the issues assigned to you in the tracker, and mark them done when they are fixed."

What it changes shows up in the panel at once; a table even says who changed it last.

## Pages agents publish

Agents can also make a page and send it to you: a report, a chart, a checklist you can tick. Just ask, "Make this result into a page and send it to me."

Once published, a page card appears under the message: the name, its version, who published it and how long ago, and **Open** on the right. Click the card or the link to open it in the panel. Right-click the card to:

- **Pin above the chat**: keep a page you use often at the top of the chat.
- **Copy link**: paste it into a chat; colleagues who can see the page open the same page in their App.

When an agent runs a dev server on its computer, it can forward it as a live page: the card has an amber broadcast icon and a small "Live" tag, and reloads itself when the code changes; when the server stops it says "Live · offline". A `http://localhost:…` link in a message also opens in the panel; that is a server running on this computer.

Some pages can send messages to an agent for you, such as a "hand it to the agent" button at the bottom of a checklist. The first time, a dialog asks "Let this page message … as you?" and shows the exact words. After **Allow and send**, it may send directly until you quit the App, at most 5 messages a minute, and they appear in your chat with that agent.

![Two pages, each with its own tab in the panel](shot:page-tabs)

## Remote Desktop

**Remote Desktop** is on the Tools shelf. On one computer, **Share this computer**; on another computer or an iPhone, pick it under **Connect to a computer** and enter the pairing code, and you see its screen, and use its keyboard and mouse if allowed. How to set it up, which Mac permissions it needs and its current limits are in [Computers › Remote Desktop](#help/computers).

![Remote Desktop: this computer is sharing, with this session's pairing code](shot:remote-desktop-sharing)

## Making your own app

If the official ones are not enough, have an agent make one: "Use `myco template init` to make me a weekly-report app, and publish it to this workspace when it's done." It shows up on the "This workspace's" shelf, and your colleagues can make their own from it too.

## Questions

**It says this workspace has no apps yet.** The workspace's myco is too old; upgrade the workspace as it says.

**A page stays blank.** Click reload at the top right of the panel; a bigger page takes a few more seconds the first time. A blank `localhost` page usually means nothing is running on that port.
