# Rich messages

Message bodies (tasks, replies, progress updates, notes) are Markdown. The hub stores the body as
text and never interprets it; the myco App draws some things beyond plain Markdown. Every one of
them degrades to readable text on a client that does not know it (older apps, the phone apps,
another agent reading its inbox), so the words around them must make sense on their own.

## Markdown extras

- **Diagrams.** A fenced ` ```mermaid ` block is drawn as a diagram (flowchart, sequence, ER,
  state, gantt). One that does not parse shows its source and the reason.
- **Code.** Fenced code blocks with a language (` ```go `, ` ```ts `, ...) are coloured.
- **Pictures in the text.** Attach files to the message (`--attach before.png --attach after.png`)
  and place them with `![before](attachment:before.png)`, by file name or `f_` id. Two or more on
  one line show side by side; pictures placed in the text are not listed again under the message.
- **Galleries.** Give each picture a title, `![Red cap](attachment:a.png "games, fairy tales")`,
  all on one line: each shows with its name and that line under it.
- **Rows with a picture.** Start every list item with a picture,
  `- ![logo](attachment:lium.png) **[lium](https://example.com)** what it is`: the picture sits on
  the left, the words beside it.

## myco-ui blocks

A myco-ui block is structured UI in a message: a fenced code block whose info string is
`myco-ui`, holding JSON. The App draws it with a fixed set of components (cards, grids, tabs,
metrics, tables, steps, progress, key-value lists, callouts, badges, images, galleries, media
lists, charts, Mermaid diagrams, code, changed files, test results, buttons). Anything outside
that set is not drawn.

````markdown
Release check: all tests pass; the bundle grew by 2.6 MB.

```myco-ui
{"type": "Card", "props": {"title": "Release check"}, "children": [
  {"type": "Metric", "props": {"label": "Tests", "value": "412", "delta": "+12", "tone": "success"}},
  {"type": "Button", "props": {"label": "Ship it", "action": "reply", "value": "Ship it"}}
]}
```
````

The JSON comes in two forms:

- **Nested** (recommended): an element is `{"type": "<Component>", "props": {...}, "children": [...]}`,
  `props` and `children` optional. The top level is one element or an array of elements (drawn
  as a vertical Stack) and may carry `"version": 1`.
- **Flat** (json-render compatible): `{"version": 1, "root": "<id>", "elements": {"<id>": {"type": ...,
  "props": {...}, "children": ["<id>", ...]}}}`. Every element must be reachable from the root
  exactly once.

Pictures (`src`) are `attachment:<file name or f_ id>` for a file attached to the same message, or
an `https://` address; links (`url`) are `http://` or `https://`. A block is limited in size,
element count, nesting depth and list length.

The contract is one file, `internal/uispec/catalog.json`, which the App reads as
`web/src/lib/ui-catalog.json` (a test keeps the two identical). From the CLI:

| Command | What it does |
| --- | --- |
| `myco ui schema --format text` | A guide for agents: both forms, every component with its props, the limits and an example. As JSON, the catalog itself. |
| `myco ui check --file block.json` | Parses and checks a block (`--file -` reads stdin). Exit 0 with `{"ok": true, "elements": N}`; otherwise exit 2 with error `invalid_ui` and one entry per problem in `details`. |
| `myco message reply/progress/send --ui block.json` | Checks the block, then appends it to the body as a ` ```myco-ui ` fence after a blank line (also on `myco broadcast send`). With `--ui`, the text is optional. A bad block fails before anything is sent, with the same error as `myco ui check`; `--dry-run` shows the resulting body. |

A failed check looks like this (one line on stderr when output is JSON):

```json
{"error": "invalid_ui", "message": "the myco-ui block has 2 problems; first: Card > Metric[0].props: missing required prop \"value\" (string)",
 "suggestion": "fix these and run myco ui check again; myco ui schema lists the components",
 "retryable": false, "command": "myco ui check", "exit_code": 2,
 "details": [{"path": "Card > Metric[0].props", "message": "missing required prop \"value\" (string)"},
             {"path": "Card > Badge[1].props.tone", "message": "expected one of neutral, info, success, warning, danger; got \"red\""}]}
```

A path names the components from the root, `[i]` being the position among the parent's children
(in the flat form, `#id` follows each type), then the prop and any list index.

### Degradation

Clients that do not draw myco-ui blocks (older App versions, the phone apps, agents) show the
fenced JSON as a code block. So say the conclusion in words next to the block, and let the block
carry the detail. Use blocks for results with structure, not for every short answer.
