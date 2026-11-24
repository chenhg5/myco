# myco's official templates (sources)

Each folder in `src/` is a template page (React, bundled with Vite) plus
its `public/` files: `myco.json` (the manifest), `agent.md` (how agents fill
it) and `sample.json`. `npm run build` (or `make templates`) writes them to
`internal/templates/official/<name>`. All of them are published as the "apps"
release (apps.zip), which hubs fetch; the binary itself carries only the
everyday ones (`templates.Builtin`), compressed in
`internal/templates/builtin.zip`, which `make templates` also writes
(`go run ./internal/templates/genbuiltin`). Commit the built output and the
archive; Go builds never run Node.

The pages read and write their data with the myco bridge (`myco.data.*`, see
docs/api.md). They load nothing from the internet. An app that
needs the computer it runs on (files, commands, the screen) uses `myco.os`
(docs/app-os.md; types in `_lib/myco.ts`).

Each app shows what it looks like in the App's new-page dialog: pictures in
its `public/shots/` (`1.webp` and its dark `1.dark.webp`, …). `shots.mjs`
takes them with the sample data, light and dark (Playwright; ffmpeg for
视频剪辑's clip; for 修图, a photo: `MYCO_SHOTS_PHOTO`, or one from Unsplash): `node build.mjs && node shots.mjs [app…] && node build.mjs`. They are saved at most 1280 wide (they ship with every hub).
An app that uses the computer (`myco.os`) is shot on a made-up one,
`shots-os.js` (a Mac's disk for 空间清理): give its plan `os: true`.
