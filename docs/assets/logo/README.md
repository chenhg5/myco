# myco logo

A pixel mushroom and a pixel wordmark (since v0.19.13), in one ink color. The light parts (the cap's spots, the stem, the inside of the o) are cut out, so the logo sits on any background.

| File | Use |
|---|---|
| `myco-logo.svg` | Mushroom + wordmark, ink `#0a0a0a`, for light backgrounds |
| `myco-logo-dark.svg` | Mushroom + wordmark, `#fafafa`, for dark backgrounds |
| `myco-symbol.svg` | The mushroom alone: favicon, avatars, small sizes |
| `myco-wordmark.svg`, `myco-wordmark-dark.svg` | The wordmark alone |
| `myco-icon-512.png`, `myco-app-icon.svg` | The previous app icon (the mycelium symbol), until the new one is chosen |

The App and the dashboard draw the logo from `web/src/components/brand.tsx` (`Logo`, `LogoSymbol`, `LogoWordmark`) in the current text color; `web/public/favicon.svg` follows the browser's light or dark scheme.
