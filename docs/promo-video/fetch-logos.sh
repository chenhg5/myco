#!/bin/sh
# The agents' marks, from their sources (trademarks of their owners; used to name the
# products myco works with):
#   Claude (Anthropic), Cursor  -> simple-icons 16.36.0 (npm)
#   Codex (OpenAI), Gemini      -> @lobehub/icons-static-svg 1.95.0 (npm)
#   Grok (xAI)                  -> https://grok.com/images/favicon.svg
# Rendered to 512px PNGs in $WORK/logos.
set -eu
HERE=$(cd "$(dirname "$0")" && pwd)
W=${WORK:?set WORK}; mkdir -p "$W/logos" "$W/si13"
cd "$W/si13" && npm i --silent --no-save @lobehub/icons-static-svg@1.95.0 >/dev/null
cd "$HERE"
cp node_modules/simple-icons/icons/claude.svg node_modules/simple-icons/icons/cursor.svg "$W/logos/"
cp "$W/si13/node_modules/@lobehub/icons-static-svg/icons/codex.svg" "$W/si13/node_modules/@lobehub/icons-static-svg/icons/gemini-color.svg" "$W/logos/"
curl -fsSL https://grok.com/images/favicon.svg -o "$W/logos/grok-favicon.svg"
node -e '
const { chromium } = require("playwright"); const fs = require("fs"); const W = process.env.WORK + "/logos/";
(async () => { const b = await chromium.launch(); const p = await b.newPage({ viewport: { width: 512, height: 512 } });
  for (const [n, f, fill] of [["claude","claude.svg","#fff"],["codex","codex.svg","#fff"],["gemini","gemini-color.svg",null],["cursor","cursor.svg","#fff"],["grok","grok-favicon.svg",null]]) {
    let svg = fs.readFileSync(W + f, "utf8"); if (fill) svg = svg.replace("<svg ", `<svg fill="${fill}" `).replace("fill=\"currentColor\"", `fill="${fill}"`);
    svg = svg.replace(/(height|width)="1em"/g, "");
    const s = fill || n === "gemini" ? 400 : 512;
    await p.setContent(`<body style="margin:0;display:flex;align-items:center;justify-content:center;width:512px;height:512px"><div style="width:${s}px;height:${s}px">${svg.replace("<svg ", "<svg width=\"100%\" height=\"100%\" ")}</div></body>`);
    await p.waitForTimeout(150); await p.screenshot({ path: W + n + ".png", omitBackground: true }); }
  await b.close(); })();'
echo "logos in $W/logos"
