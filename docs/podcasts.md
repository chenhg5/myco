# World Podcasts

Create a **World Podcasts** page from Apps. Rotate or zoom the globe and choose a
point, or use the Region selector. Points represent regional discovery channels,
not verified recording locations. Catalog regions may contain multiple languages.
Choose Stories, Technology, Culture or Life, then a show, or press Play to try a
show from the current results. Skip moves to another episode, then another show.

Search for a show, host or topic, or paste a publisher's public RSS/Atom feed URL.
The directory uses Apple Podcasts; episode audio and show notes come from the
publisher's RSS feed. A source may be unavailable on your network. Private feeds,
paid-only episodes and credentials in URLs are not supported.

Like an episode or follow a show to keep it in My library. Listening progress,
follows and likes stay on this device, scoped to this page and your myco identity.
They are not visible to agents or other page members and clearing device storage
removes them. Speed, seeking and a sleep timer are in the player.

Streaming requires a native App advertising `net-media`. If the page asks you to
update, updating the template alone cannot add that native capability. The browser
console cannot stream these files. Audio streams through the native App with Range
support; the template does not download entire episodes into memory. Desktop and
iOS availability depend on their installed native builds.

Page members and agents can suggest feeds by adding `stations` records with
`name`, `feed_url`, `region` (cn/us/gb/jp/de/fr/es/br/kr/in/au) and `topic`. These
suggestions are shared page data, separate from each person's private library.
