# Apps

**English** · [中文](apps.zh-CN.md) · [README](../README.md)

We are rebuilding the apps people use every day on Mac, Windows and phone so that **agents can use them too**. You and your agent work on the same page: the agent fills it in or edits it, and you see the change live. Open them in the myco app under **Apps**; new ones arrive release by release.

This is part of **[Rebuild 100 great apps with agents, free and open, always improving](open-apps.md)**: why we are doing it, what we stand by, how to nominate the next app and how to build one yourself.

Apps that need the computer itself (files, the screen, the microphone, the network) ask it through the myco app; see [`myco.os`](app-os.md). Recordings, photos and videos stay on the computer that opened them.

Agent access currently uses shared page records. The [agent actions SDK](app-actions.md) separates framework authorization, SDK transport and application logic for future runtime actions; it does not yet enable agent remote control on existing installations.

## Creative

**AIGC Canvas** links notes, text, images, and videos into a shared workflow. Text generation uses OpenRouter, while images and videos use Replicate. Enter your own provider keys for the current session. Generated media links can expire, so save files you want to keep.

[Color studio](palette.md): extract colors from local images, save shared palettes and favorites, explore harmonies, check text contrast, and export SVG, CSS or JSON. Agents can create and revise the same palettes.

<table>
<tr>
<td width="50%"><picture><source media="(prefers-color-scheme: dark)" srcset="assets/apps/motion-dark.webp"><img src="assets/apps/motion.webp" alt="Motion video"></picture><br><b>Motion video</b><br>Promo clips, data recaps and vertical shorts: start from a template or describe it and the agent writes each scene (HTML and animation). Comment at a point in time for changes, remake from a reference video, add music and voice-over with captions, export MP4 on your computer.</td>
<td width="50%"><picture><source media="(prefers-color-scheme: dark)" srcset="assets/apps/design-studio-dark.webp"><img src="assets/apps/design-studio.webp" alt="Design studio"></picture><br><b>Design studio</b><br>Design websites, interfaces, posters and dashboards with agents. Edit canvases and layers, preview HTML, collect feedback and save versions; compatible Apps let agents export shared SVG, PNG, HTML or JSON files.</td>
</tr>
<tr>
<td width="50%"><picture><source media="(prefers-color-scheme: dark)" srcset="assets/apps/photo-dark.webp"><img src="assets/apps/photo.webp" alt="Photo editor"></picture><br><b>Photo editor</b><br>Crop and rotate, adjust colour, a dozen filters, retouching, text, stickers, doodles, mosaic and frames; hold to compare with the original. Photos stay on the computer that opened them; the agent can make the edits you describe.</td>
<td width="50%"><picture><source media="(prefers-color-scheme: dark)" srcset="assets/apps/video-dark.webp"><img src="assets/apps/video.webp" alt="Video editor"></picture><br><b>Video editor</b><br>Import an MP4, split, trim, reorder and change speed on a timeline, add text, blur faces and number plates, export MP4 on your computer. The agent edits from your notes.</td>
</tr>
<tr>
<td width="50%"><picture><source media="(prefers-color-scheme: dark)" srcset="assets/apps/recorder-dark.webp"><img src="assets/apps/recorder.webp" alt="Screen recorder"></picture><br><b>Screen recorder</b><br>Record the screen, then trim, split, add text and zoom in on what matters, and export MP4 on your computer. The edit plan can be shared with an agent.</td>
<td width="50%"><picture><source media="(prefers-color-scheme: dark)" srcset="assets/apps/mindmap-dark.webp"><img src="assets/apps/mindmap.webp" alt="Mind map"></picture><br><b>Mind map</b><br>The agent writes an outline and the page draws it as a mind map: Tab adds a child, Enter a sibling, drag to move, fold branches, export a Markdown outline.</td>
</tr>
<tr>
<td width="50%"><picture><source media="(prefers-color-scheme: dark)" srcset="assets/apps/whiteboard-dark.webp"><img src="assets/apps/whiteboard.webp" alt="Whiteboard"></picture><br><b>Whiteboard</b><br>An infinite canvas with sticky notes, text, shapes and connectors. Co-edit with agents and export SVG/PNG; compatible Apps let agents export shared boards in the background.</td>
<td width="50%"><picture><source media="(prefers-color-scheme: dark)" srcset="assets/apps/logo-studio-dark.webp"><img src="assets/apps/logo-studio.webp" alt="Logo Studio"></picture><br><b>Logo Studio</b><br>Send a brand brief to your agent, compare original geometric marks, and play back their construction. Edit shapes, position and color, then export a clean SVG.</td>
</tr>
</table>

[Database studio](database.md): local MySQL, PostgreSQL, SQLite and MongoDB connections, record editing and SQL/Extended JSON queries in a compatible desktop App. Only deliberately shared query drafts enter the page.

## Office

<table>
<tr>
<td width="50%"><picture><source media="(prefers-color-scheme: dark)" srcset="assets/apps/doc-dark.webp"><img src="assets/apps/doc.webp" alt="Document"></picture><br><b>Document</b><br>Write together like in Feishu Docs: '/' inserts headings, lists, to-dos, tables and code; each paragraph saves on its own; export Markdown or HTML.</td>
<td width="50%"><picture><source media="(prefers-color-scheme: dark)" srcset="assets/apps/table-dark.webp"><img src="assets/apps/table.webp" alt="Table"></picture><br><b>Table</b><br>A spreadsheet to fill in together: typed columns (tags, dates, people), copy and paste from Excel, fill down, filter, sort, export Excel or CSV.</td>
</tr>
<tr>
<td width="50%"><picture><source media="(prefers-color-scheme: dark)" srcset="assets/apps/base-dark.webp"><img src="assets/apps/base.webp" alt="Database"></picture><br><b>Database</b><br>One set of records as a table, board, calendar, gallery or form, like Feishu Base: typed fields, filters and sorting, details on the side, export Excel.</td>
<td width="50%"><picture><source media="(prefers-color-scheme: dark)" srcset="assets/apps/slides-dark.webp"><img src="assets/apps/slides.webp" alt="Slides"></picture><br><b>Slides</b><br>Agents write content; the page arranges covers, points, charts, cards, tables and quotes. Co-edit and present; compatible Apps let agents export editable PowerPoint files and Markdown outlines.</td>
</tr>
<tr>
<td width="50%"><picture><source media="(prefers-color-scheme: dark)" srcset="assets/apps/meeting-dark.webp"><img src="assets/apps/meeting.webp" alt="Meeting notes"></picture><br><b>Meeting notes</b><br>Record a meeting in Zoom, Tencent Meeting or Feishu, with your microphone and the others on separate tracks; transcribe it on your computer (nothing is uploaded), tell speakers apart, and let the agent write the minutes. Needs the myco app.</td>
<td width="50%"><picture><source media="(prefers-color-scheme: dark)" srcset="assets/apps/tutor-dark.webp"><img src="assets/apps/tutor.webp" alt="Learning workspace"></picture><br><b>Learning workspace</b><br>Import a PDF, text or Markdown textbook and ask about a source page. Your tutor adapts to your level with explanations, interactive diagrams, citations and short checks. Choose step-by-step tutoring to follow a learning path and continue from each lesson. Try the built-in Riemann hypothesis lesson at three levels.</td>
</tr>
<tr>
<td width="50%"><picture><source media="(prefers-color-scheme: dark)" srcset="assets/apps/scheduling-dark.webp"><img src="assets/apps/scheduling.webp" alt="Calendar"></picture><br><b>Calendar</b><br>A personal calendar with month, week and day views. Add and drag events, plan all-day and recurring entries, search and export calendars with agents. Appointment booking lives in a secondary screen. No external calendar sync.</td>
<td width="50%"><picture><source media="(prefers-color-scheme: dark)" srcset="assets/apps/podcast-dark.webp"><img src="assets/apps/podcast.webp" alt="World Podcasts"></picture><br><b>World Podcasts</b><br>Turn a 3D globe to discover public podcasts by region. Search or add RSS feeds, listen continuously, follow shows and like episodes. Speed, sleep timer and private device progress. Requires a native App with network media streaming. <a href="podcasts.md">Details</a>.</td>
</tr>
</table>

## Development

<table>
<tr>
<td width="50%"><picture><source media="(prefers-color-scheme: dark)" srcset="assets/apps/code-dark.webp"><img src="assets/apps/code.webp" alt="Code editor"></picture><br><b>Code editor</b><br>A VS Code-style editor with a file tree, tabs, highlighting, find and replace, multiple cursors and a minimap. The agent writes files and you edit them; preview HTML and Markdown, download the project as a zip.</td>
<td width="50%"><picture><source media="(prefers-color-scheme: dark)" srcset="assets/apps/issues-dark.webp"><img src="assets/apps/issues.webp" alt="Issue tracker"></picture><br><b>Issue tracker</b><br>Issues and projects, Linear-style: IDs, status, priority, assignee, labels, list and board, keyboard first. Assign an issue to an agent and watch it change the status and comment.</td>
</tr>
<tr>
<td width="50%"><picture><source media="(prefers-color-scheme: dark)" srcset="assets/apps/devtools-dark.webp"><img src="assets/apps/devtools.webp" alt="Developer toolbox"></picture><br><b>Developer toolbox</b><br>JSON formatting, Base64, URL encoding, MD5 and SHA, timestamps, UUIDs, JWT decoding, regex testing, text diff, case and base conversion. Everything runs locally.</td>
<td width="50%"></td>
</tr>
</table>

## Tools

<table>
<tr>
<td width="50%"><picture><source media="(prefers-color-scheme: dark)" srcset="assets/apps/disk-dark.webp"><img src="assets/apps/disk.webp" alt="Disk cleanup"></picture><br><b>Disk cleanup</b><br>See what fills this computer's disk as rings, with a list by size; drill into folders, collect what you don't need, then move it to the Trash (and put it back if you change your mind). Needs the myco app.</td>
<td width="50%"><picture><source media="(prefers-color-scheme: dark)" srcset="assets/apps/remote-desktop-dark.webp"><img src="assets/apps/remote-desktop.webp" alt="Remote desktop"></picture><br><b>Remote desktop</b><br>See and control a Mac or Windows PC in the same workspace from another computer or an iPhone (touchpad, keyboard and MAME arcade controller on iPhone). Off until you install it; direct over your LAN or Tailscale.</td>
</tr>
<tr>
<td width="50%"><picture><source media="(prefers-color-scheme: dark)" srcset="assets/apps/notes-dark.webp"><img src="assets/apps/notes.webp" alt="Notes"></picture><br><b>Notes</b><br>Notes like Notion: nested pages with icons and covers, '/' commands, search, favourites and a trash. The agent can create pages and write into them while you watch.</td>
<td width="50%"><picture><source media="(prefers-color-scheme: dark)" srcset="assets/apps/todo-dark.webp"><img src="assets/apps/todo.webp" alt="To-do list"></picture><br><b>To-do list</b><br>Shared to-dos: press Enter to add; understands 'tomorrow', 'Friday', '@name' and '!'. Due dates, owners, notes, drag to reorder; the agent can collect to-dos from a meeting.</td>
</tr>
<tr>
<td width="50%"><picture><source media="(prefers-color-scheme: dark)" srcset="assets/apps/stocks-dark.webp"><img src="assets/apps/stocks.webp" alt="Stock watch"></picture><br><b>Stock watch</b><br>A shared watchlist for China, Hong Kong and US stocks with daily, weekly and monthly candles, volume, moving averages and notes. Switchable data sources; quotes are fetched by the computer that opens it.</td>
<td width="50%"><picture><source media="(prefers-color-scheme: dark)" srcset="assets/apps/proxy-console-dark.webp"><img src="assets/apps/proxy-console.webp" alt="Proxy console"></picture><br><b>Proxy console</b><br>Manage the Mihomo and sing-box running on this computer: live traffic, policy groups, node speed tests, connections. The control key stays on this computer. Needs the desktop app.</td>
</tr>
</table>

## Life

<table>
<tr>
<td width="50%"><picture><source media="(prefers-color-scheme: dark)" srcset="assets/apps/reader-dark.webp"><img src="assets/apps/reader.webp" alt="Reader"></picture><br><b>Reader</b><br>A local PDF and TXT bookshelf with page transitions, paper themes, typography controls, contents, search, bookmarks and remembered progress. Original files stay on this device. <a href="reader.md">Details</a>.</td>
<td width="50%"><picture><source media="(prefers-color-scheme: dark)" srcset="assets/apps/price-compare-dark.webp"><img src="assets/apps/price-compare.webp" alt="Shopping comparison"></picture><br><b>Shopping comparison</b><br>An agent-side Maishou API adapter queries offers and purchase links; people compare specifications, conditions and sources. Requires your own Maishou authorization on the executing agent. Live authenticated acceptance awaits owner credentials. Save candidates and shared notes. <a href="shopping-comparison.md">Details</a>.</td>
</tr>
<tr>
<td width="50%"><picture><source media="(prefers-color-scheme: dark)" srcset="assets/apps/ledger-dark.webp"><img src="assets/apps/ledger.webp" alt="Budget book"></picture><br><b>Budget book</b><br>Income and spending by month, balance and budget left, where the money went by category, every entry by day. Keep it with family or a small team; the agent can file bills for you.</td>
<td width="50%"><picture><source media="(prefers-color-scheme: dark)" srcset="assets/apps/habit-dark.webp"><img src="assets/apps/habit.webp" alt="Habit tracker"></picture><br><b>Habit tracker</b><br>Check in on your habits together: this week's days, streaks, and who else has checked in. The agent can remind you and count.</td>
</tr>
<tr>
<td width="50%"><picture><source media="(prefers-color-scheme: dark)" srcset="assets/apps/cinema-dark.webp"><img src="assets/apps/cinema.webp" alt="Cinema"></picture><br><b>Cinema</b><br>Search films and series from your own sources, pick an episode and play, keep a watchlist; progress stays on this computer, with resume, speed and intro skipping. No sources included. Updated clients can open links directly to a movie, playback line and episode; use Copy playback link in the player. See <a href="app-links.md">App links</a>.</td>
<td width="50%"><picture><source media="(prefers-color-scheme: dark)" srcset="assets/apps/personality-dark.webp"><img src="assets/apps/personality.webp" alt="Personality explorer"></picture><br><b>Personality explorer</b><br>A gentle 32-question reflection exercise with original illustrations, four preference distributions and 16 perspectives. Answers and notes stay on this device; sharing a summary with the page is optional. Not the official MBTI® assessment or a diagnosis.</td>
</tr>
</table>

## Games

<table>
<tr>
<td width="50%"><picture><source media="(prefers-color-scheme: dark)" srcset="assets/apps/gomoku-dark.webp"><img src="assets/apps/gomoku.webp" alt="Gomoku"></picture><br><b>Gomoku</b><br>Play a game with a colleague or an agent: the page keeps turns and decides the winner; undo, resign, play again.</td>
<td width="50%"><picture><source media="(prefers-color-scheme: dark)" srcset="assets/apps/sudoku-dark.webp"><img src="assets/apps/sudoku.webp" alt="Sudoku"></picture><br><b>Sudoku</b><br>Solve one together: each cell saves on its own and shows who filled it, conflicts in red; easy, medium and hard puzzles with one answer each.</td>
</tr>
<tr>
<td width="50%"><picture><source media="(prefers-color-scheme: dark)" srcset="assets/apps/arcade-dark.webp"><img src="assets/apps/arcade.webp" alt="Arcade Emulator"></picture><br><b>Arcade Emulator</b><br>Verify and launch a local arcade ROM ZIP from the desktop App in a separate MAME window. Install MAME separately. On Windows, leave iPhone arcade controller compatibility enabled when using Remote Desktop. CHIP-8 mode includes the original Orbit Courier demo and touch controls. In NES mode, select a local .nes cartridge and play on desktop or mobile. Fullscreen keeps the touch controls visible, and Swap A/B changes the assignment.</td>
<td width="50%"></td>
</tr>
</table>
