# myco logo

| File | Use |
|---|---|
| `myco-logo.svg` | Symbol + wordmark, for light backgrounds |
| `myco-logo-dark.svg` | Symbol + wordmark, for dark backgrounds |
| `myco-symbol.svg` | The mycelium symbol alone: favicon, avatars, small sizes (reads down to 16 px) |
| `myco-wordmark.svg`, `myco-wordmark-dark.svg` | The wordmark alone |
| `myco-icon-512.png`, `myco-app-icon.svg` | App icon / avatar: gradient symbol on a graphite (`#2a2c30`) rounded tile |
| `../social-preview.png` | 1280×640 card for links (GitHub → Settings → Social preview) |

Colors: the symbol is a mint-to-teal gradient (since v0.18.7): `#5ee0a8 → #2fb889 → #0f8f84` on light backgrounds, `#a6f4c5 → #34d399 → #0f9488` on dark ones and in the app icon; flat uses keep brand green `#3a9a78`. The wordmark is ink `#0a0a0a` on light and `#fafafa` on dark.
The dashboard uses the same green as its `--brand` token (`web/src/index.css`) and draws the logo from
`web/src/components/brand.tsx`.
