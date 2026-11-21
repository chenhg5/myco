# Recording studio

Use this app for screen demonstrations and tutorials. Recording is always started
by the person in the app with the system's screen picker. Never start recording
silently. Footage and originals stay on that computer; workspace records contain
only the edit. The agent cannot see the footage merely by reading these records.

The schema is shared with Video editor: project, media, clips, overlays, notes.
Clips use source seconds (`in`, `out`), speed, volume, muted, and order. Overlay
start/end use timeline seconds. Type `text` adds a caption; `blur` / `mosaic` cover
private details. Type `zoom` smoothly focuses the picture on the center of its
normalized x/y/w/h region, ramps in/out over up to 0.35 seconds, and reaches
2 + strength * 2 magnification (strength 0–1). Redactions zoom with the source picture; text stays in output
coordinates. Avoid overlapping zooms: the last record wins. Delete an overlay to
remove it; editing source files is unnecessary. The same renderer handles preview
and MP4 export. Use notes to ask the person which intervals matter.

First version: ten-minute recordings. Mac uses system recording (selected region
or main screen, optional microphone, no internal audio). Windows desktop capture
requires a separately installed FFmpeg with gdigrab and h264_mf; silent desktop
only. A browser with display capture may provide its own screen/window picker.
Unsupported hosts can still import existing videos. No auto cursor tracking,
auto captions, webcam or multi-track recording in this version.
