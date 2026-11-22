# Whiteboard

Open **Apps → Whiteboard** for an infinite canvas. Its language and light/dark
appearance follow the myco App. New notes use a neutral gray; the contextual
palette offers muted cream, rose, blue, sage, lavender and apricot.

Double-click empty canvas for a note, or choose a tool and click to place a note,
heading, rectangle or ellipse. Double-click a node (or press Enter when selected)
to edit its text. Escape cancels text editing; Cmd/Ctrl+Enter saves it.

## Select, copy and arrange

Drag across empty canvas to box-select nodes. Shift-click adds or removes an item.
Cmd/Ctrl+A selects everything. Cmd/Ctrl+C, X and V copy, cut and paste; Cmd/Ctrl+D
duplicates. Copying a group also copies its internal connectors and attaches them
to the new nodes. Copying a connector includes its two endpoints. Pasting external
plain text creates a note. Text editing retains the normal system clipboard.

The selection toolbar and right-click menu offer copy, duplicate, delete and layer
commands. The toolbar also controls colors, text size/alignment and locking. With
multiple nodes selected, **Arrange** aligns edges or centers, or distributes three
or more nodes evenly. Locked nodes stay in place. Drag a corner to resize; hold
Shift for the original proportions. Arrow keys nudge one pixel, or ten with Shift.
Dragging snaps to nearby edges and centers; Alt temporarily disables snapping.

Each action is one undo step, including group movement, copying and deletion with
its connectors. Use Cmd/Ctrl+Z and Shift+Cmd/Ctrl+Z (or Ctrl+Y). Undo/redo retains up
to 100 local actions in the open page. If another person or agent changed those
records afterwards, undo leaves their newer content intact.

## Connect ideas

Select a node and drag one of its four edge connection points onto another node.
The connector snaps to an edge and follows the nodes when they move or resize.
Release over empty canvas to create another connected node in the same action.
Alternatively, choose **Connector**, then drag between nodes or click source and
target. Escape cancels a pending connection.

Select a connector for elbow, curve or straight routing, solid/dashed lines, color
and a text label. Double-click the connector to edit its label. Drag a selected
connector's endpoint to reconnect it. Elbow routes prefer paths outside node bounds;
complex diagrams may benefit from choosing different edge anchors or moving nodes.

## Navigate and export

Space-drag, the hand tool or the middle mouse button pans. A trackpad or mouse wheel
pans; Cmd/Ctrl+wheel zooms around the pointer. The zoom controls restore 100% or fit
all nodes. On a phone, drag empty canvas to pan and use two fingers to pan/zoom.
The contextual toolbar scrolls horizontally when space is limited.

**Export SVG** saves a standalone image with shapes, text, routes and labels, using
a light background. Clipboard availability depends on the host webview; when a
browser declines a toolbar clipboard action, use its normal copy/paste shortcuts.
If direct clipboard reading is unavailable (including some phone webviews), Paste
opens a small field: touch and hold there and use the system Paste command. It
accepts copied board graphs as well as plain text, then closes automatically.
Existing boards keep their record IDs and contents when upgraded. Agents edit the
same records through `myco page data`; the app's `agent.md` documents all fields.
