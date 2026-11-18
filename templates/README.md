# myco's official templates (sources)

Each folder in `src/` is a template page (React, bundled with Vite) plus
its `public/` files: `myco.json` (the manifest), `agent.md` (how agents fill
it) and `sample.json`. `npm run build` (or `make templates`) writes them to
`internal/templates/official/<name>`, which the hub embeds and offers in every
workspace's library. Commit the built output; Go builds never run Node.

The pages read and write their data with the myco bridge (`myco.data.*`, see
docs/design/templates.md). They load nothing from the internet.
