# myco logo

A pixel mushroom and a pixel wordmark (since v0.19.13), in one ink color. The light parts (the cap's spots, the stem, the inside of the o) are cut out, so the logo sits on any background.

| File | Use |
|---|---|
| `myco-logo.svg` | Mushroom + wordmark, ink `#0a0a0a`, for light backgrounds |
| `myco-logo-dark.svg` | Mushroom + wordmark, `#fafafa`, for dark backgrounds |
| `myco-symbol.svg` | The mushroom alone: favicon, avatars, small sizes |
| `myco-wordmark.svg`, `myco-wordmark-dark.svg` | The wordmark alone |
| `myco-icon-512.png`, `myco-app-icon.svg` | App icon: the ink mushroom (`#141414`) on a cream (`#f3efe6`) rounded tile |

The App and the dashboard draw the logo from `web/src/components/brand.tsx` (`Logo`, `LogoSymbol`, `LogoWordmark`) in the current text color; `web/public/favicon.svg` follows the browser's light or dark scheme. The App's sidebar shows the wordmark alone (the mushroom is the app's icon).

The iOS app's icon is `ios/Myco/Assets.xcassets/AppIcon.appiconset/icon-1024.png`, rendered from `ios/design/app-icon.svg`: the same mushroom on the cream filling the whole square (the system rounds the corners itself), at the size it has on the Mac's tile; its welcome page draws the logo from `BrandLogo` (`ios/MycoKit/Sources/MycoUI/BrandLogo.swift`), the same paths.

The desktop app's icons are in `cmd/myco-app/build/`: `darwin/icon-1024.png` (the Big Sur grid: an 824 px tile in 1024, with a soft shadow), `darwin/tray-template.png` (the mushroom in black, a template image the menu bar tints), `windows/icon-256.png` (no tile: the mushroom alone fills the square, drawn like the tray icon, so it is as big on the taskbar as its neighbours), and `windows/tray.png` (the ink mushroom with cream spots and stem and a thin cream outline, so it reads on light and dark taskbars).
