# Links to a view inside an app

A public page link opens one myco app page. Recent clients can also preserve an
application-owned URL fragment to select a view inside that page:

```
https://your-hub/pages/pg_example#watch?source=my_catalog&movie=123&line=HD&episode=%E7%AC%AC02%E9%9B%86
```

The part before `#` is the ordinary page URL. Page permissions still apply.
The fragment is view selection, not an authorization token or an instruction to
perform a shared write. It is not sent to the Hub's HTTP file/data endpoints.
Do not put credentials, signed media URLs or sensitive personal data in a link:
people with the message or a copied link can read its fragment.

## Cinema: a movie and a particular episode

The player has a fullscreen button and an exit control inside the fullscreen
video. On macOS, fullscreen requires a desktop build that enables WKWebView
element fullscreen; updating the Cinema template alone does not enable it.

Use an existing Cinema page and one of its enabled, user-approved sources.
Agents can read `sources` through page data, search that catalog and read a movie's
details, then send a link with:

- `source`: the source **record ID**, not its endpoint URL.
- `movie`: the movie's catalog ID (`vod_id` as a string).
- `line`: optional exact playback-line label from the catalog.
- `episode`: optional exact episode label; requires `line`.

Encode values individually with `URLSearchParams` or another standard URL encoder.
A link with only source and movie uses personal resume history when available.
Explicit line/episode labels select that entry, respecting the person's intro-skip
setting. Removed sources, lines and episodes show an error; Cinema never silently
replaces a missing linked episode with the first episode. A link never adds a
source or writes to the watchlist. The person may need to tap Play on their device.

The player's **Copy playback link** button creates a public link to the current
movie, line and episode. Clipboard refusal shows a selectable link for manual
copying. An older client without public-page context shows an update hint.

For “find this show and send me episode two”, an agent reads the page's approved
sources, searches a reachable catalog, reads the chosen movie's exact line and
episode labels, and returns the resulting page link. No raw stream URL is needed.
The source must also be reachable from the person's device for playback.

## Client support

Clients with app-link support retain fragments from chat text, page cards and
pinned links. Navigating to a different view of an already-open page reuses the
page's tab. Clicking the same deep link again navigates again; it is not merely
bringing an old inner view to the front. Changing a route keeps the page document and its unfinished work in place.

This requires both a client that retains fragments and an application that handles
its own routes. Updating Cinema alone cannot add link handling to an older native
client. Include the ordinary page URL as a fallback when the client is unknown.
No Hub protocol or permission change is required. Query strings are not used as
application routes; the client still owns its private rendering URLs and language
parameters.

## For app developers

Templates can use `templates/src/_lib/page-route.ts`: `readAppRoute` reads a bounded
fragment, `appFragment` encodes values, and `publicAppLink` combines it with a public
page URL. Applications own their route names and business input validation. Handle
both the initial fragment and `hashchange`; `subscribeAppRoute` also handles
a deliberate repeated link from the trusted parent host. Keep fragments within
1,024 characters, reject invalid selectors visibly.
Never treat opening a route as approval to add a network source, execute a command,
or make another shared change.

Recent page bridges add optional `url` to `myco.ready.page`. It is the canonical
public Page URL, not the private loopback file URL used to render the app. Older
clients omit it; disable or explain link-copying rather than guessing a Hub URL
from `location`. Source and movie IDs have different meanings from record IDs;
read the application's `agent.md` before constructing a route.
