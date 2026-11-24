# Promotional screenshot scenes

Use synthetic data only. The runner starts a fresh SHOWCASE mock per language,
serves the actual App through Vite on unused loopback ports, and closes its
servers and browser afterward. It clears inherited myco, model-provider and mock
configuration from the child mock's environment, does not attach a real OS
backend, and blocks browser requests outside loopback. No screenshots are uploaded
by this command. Use the separate apps checkout at `apps.lock.json`'s source commit (`APPS_TEST_ROOT`,
or `.cache/apps-source`). Prepare its verified release bundles with
`python3 scripts/prepare-apps.py --apps-dir /path/to/apps/dist/official` first.
See [host build instructions](../../../docs/building.md).

From the repository root (Node 18+, `npm --prefix web ci`, global Playwright with
Chromium installed):

```sh
node web/scripts/media-kit-shots.mjs --out ../media-kit-output
```

The output is a manifest and 2880 × 2000 PNGs (1440 × 1000 at 2×). Every scene
gets Chinese and English light screenshots. The first six scenes also get both
dark versions; individual scenes may also specify `themes` explicitly. The same scene function and translated synthetic data are used
for both languages. English workflow and automation fixture copy is translated
separately from the product's own UI dictionary. A failed scene writes a
`.FAILED.png` and debug text and makes the run exit unsuccessfully; it does not
add a successful screenshot to the manifest.

```sh
# Iterate on a scene without touching any real workspace.
node web/scripts/media-kit-shots.mjs --out ../media-kit-check --scene coedit --lang en
# Rerender one failed case without losing the other completed shots.
node web/scripts/media-kit-shots.mjs --out ../media-kit-output --scene motion --lang en --resume
# Optional local videos of the scenes that have interaction actions.
# Requires ffmpeg; WebM clips capped at 18 seconds stay on disk and are never uploaded.
node web/scripts/media-kit-shots.mjs --out ../media-kit-video --scene teamwork,progress,terminal,workflow,automation,focus-mode --video
```

Add a scene in `scenes.mjs`: a stable `id`, bilingual title and pitches, category
tags, an optional Chinese `description` naming the visible elements, and an async `prepare` function operating the real UI or local mock API.
Optional `video` performs a visible interaction after the still image. Put
app-specific fixture preparation in `apps.mjs`. Never make a placeholder scene
use the project home as a substitute: declare `skip` with the exact reason when
native capabilities are unavailable. Recording and real remote control are
currently skipped, as are mobile and CLI (outside this script's scope).

The additional `MOCK_MEDIA_KIT=1` fixture mode is enabled only by this runner.
It aligns the SHOWCASE project with the mock's studio workflows/pages and offers
an in-memory rich-message fixture. It does not execute model requests, SSH or
system commands. Disk scanning and SSH overview are in-memory simulations;
their screenshots are UI demonstrations, not native execution evidence.

## Review, then upload

Inspect the paired images, check `manifest.json` for errors, and review the
English `.text.txt` files for untranslated fixture copy. The upload helper
requires an explicit `--upload` and refuses a failed capture run. It checks the
project's 100-file quota before writing, preserves existing published entries,
creates separate `feature-*` drafts, includes `App` and category tags, records
actual language/theme/dimensions plus platform/aspect/kind/tags/description, and adds small JPEG gallery previews. Python 3
with Pillow is required for previews. It never uploads videos or debug files.

```sh
node web/scripts/media-kit/upload.mjs --page <page-id> --project <project-name> --out ../media-kit-output
node web/scripts/media-kit/upload.mjs --page <page-id> --project <project-name> --out ../media-kit-output --upload
# Update metadata for existing files without uploading the originals again:
node web/scripts/media-kit/upload.mjs --page <page-id> --project <project-name> --out ../media-kit-output --upload --metadata-only
```

Both `--page` and `--project` are required, including for a dry run. Choose a destination you are authorized to update; the helper has no default workspace, project or page. Uploads reuse deterministic filenames and CAS
versions; a retry completes the same records instead of allocating duplicate
file slots. Concurrent record conflicts stop for review. `uploaded.json` records
the entry IDs and final project file count.

## Interaction collection

The interaction batch adds `agent-switcher`, `focus-mode`, `next-suggestion`,
`turn-nav`, `session-handover`, and `chat-background`. Each uses the real App UI
with synthetic completed turns. Capture both languages in light mode:

```sh
node web/scripts/media-kit-shots.mjs --out ../interaction-shots --scene focus,agent-switcher,focus-mode,next-suggestion,turn-nav,session-handover,chat-background
# The two required interaction videos stay local:
node web/scripts/media-kit-shots.mjs --out ../interaction-clips --scene agent-switcher,focus-mode --video
```

`focus` retains its existing media entry/file IDs but now demonstrates only
side-by-side conversations. The separate `focus-mode` entry demonstrates the
quiet single-task view. This batch allocates twelve new image files and replaces
the two existing split-view images; it never starts an actual session handover.

The `runner-queue` scene demonstrates a task waiting for the computer's shared
CLI capacity, in both languages, using the mock admission fixture:

```sh
APPS_TEST_ROOT=/path/to/apps node web/scripts/media-kit-shots.mjs --out ../runner-queue-shots --scene runner-queue
```
