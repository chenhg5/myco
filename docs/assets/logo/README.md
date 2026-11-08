# myco logo

| File | Use |
|---|---|
| `myco-logo.svg` | Symbol + wordmark, for light backgrounds |
| `myco-logo-dark.svg` | Symbol + wordmark, for dark backgrounds |
| `myco-symbol.svg` | The mycelium symbol alone: favicon, avatars, small sizes (reads down to 16 px) |
| `myco-wordmark.svg`, `myco-wordmark-dark.svg` | The wordmark alone |
| `myco-icon-512.png` | App icon / avatar: green symbol on a black rounded tile |
| `../social-preview.png` | 1280×640 card for links (GitHub → Settings → Social preview) |

Colors: the symbol is always brand green `#3a9a78` (a softer green, since v0.17.2); the wordmark is ink `#0a0a0a` on light and `#fafafa` on dark.
The dashboard uses the same green as its `--brand` token (`web/src/index.css`) and draws the logo from
`web/src/components/brand.tsx`.
