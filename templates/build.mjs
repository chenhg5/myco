// Builds each template in src/ into internal/templates/official/<name>
// (its page, bundled; its public/ files: myco.json, agent.md, sample.json).
// The hub embeds that folder; go builds never run Node, so the output is
// committed (as web/ builds into internal/web/dist).
import { build } from "vite";
import react from "@vitejs/plugin-react";
import { readdirSync, existsSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";

// BlockNote's emoji picker ships names in 24 languages (4 MB); the pages
// keep English and Chinese, other languages fall back to English.
const keepEmoji = new Set(["en", "zh", '"zh-hant"']);
const trimEmoji = {
  name: "myco-trim-emoji-locales",
  transform(code, id) {
    if (!id.replace(/\\/g, "/").endsWith("@blocknote/core/dist/emoji-data.js")) return null;
    return code.replace(/^\s*([\w"-]+): \(\) => import\("[^"]+"\),?\n/gm, (line, key) => (keepEmoji.has(key) ? line : ""));
  },
};

const here = path.dirname(fileURLToPath(import.meta.url));
const only = process.argv[2];
for (const name of readdirSync(path.join(here, "src"))) {
  if (only && name !== only) continue;
  const root = path.join(here, "src", name);
  if (!existsSync(path.join(root, "index.html"))) continue;
  await build({
    root,
    base: "./",
    logLevel: "warn",
    plugins: [react(), trimEmoji],
    publicDir: path.join(root, "public"),
    build: {
      outDir: path.join(here, "..", "internal", "templates", "official", name),
      emptyOutDir: true,
      assetsDir: "assets",
      chunkSizeWarningLimit: 4000,
      // one script and one stylesheet: a page is a few files
      rollupOptions: {
        output: { manualChunks: undefined },
        onwarn(w, warn) { if (w.code !== "INVALID_ANNOTATION") warn(w); },
      },
    },
  });
  console.log(`built ${name}`);
}
