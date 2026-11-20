# myco's official templates (sources)

Each folder in `src/` is a template page (React, bundled with Vite) plus
its `public/` files: `myco.json` (the manifest), `agent.md` (how agents fill
it) and `sample.json`. `npm run build` (or `make templates`) writes them to
`internal/templates/official/<name>`, which the hub embeds and offers in every
workspace's library. Commit the built output; Go builds never run Node.

The pages read and write their data with the myco bridge (`myco.data.*`, see
docs/design/templates.md). They load nothing from the internet.

Each app shows what it looks like in the App's new-page dialog: pictures in
its `public/shots/` (`1.webp` and its dark `1.dark.webp`, …). `shots.mjs`
takes them with the sample data, light and dark (Playwright; ffmpeg for
视频剪辑's clip): `node build.mjs && node shots.mjs [app…] && node build.mjs`.
