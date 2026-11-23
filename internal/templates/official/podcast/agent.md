# World Podcasts

A globe-based public podcast discovery and listening app. Globe points denote regional editorial channels, not verified studio locations. The Apple directory market is a discovery region, not an exact language filter. Playable audio comes from publishers' public RSS enclosures.

## Shared suggestions

`stations` records have `name` (text), `feed_url` (public HTTP(S) RSS/Atom URL), `region` (one of cn/us/gb/jp/de/fr/es/br/kr/in/au), and `topic` (text). Add a source only after confirming the publisher's public feed. These records are visible to page members. Never put credentials or private subscription tokens in them.

Likes, follows and listening progress are private device storage, scoped to page and person. Agents cannot read or edit that library through page data. Do not claim playback succeeded just because a record was saved.

Playback requires a native client advertising `net-media` with `myco.media.url`. Old clients and browser consoles show an upgrade message; they do not download the whole file through net.fetch. Desktop and iOS availability depends on the installed native build. Feed and catalog requests use the network bridge; source accessibility depends on the publisher and the user's network. Audio is not republished or stored by the template.

People can also paste a public RSS address into the search field. This is a temporary selection; following the show saves it privately on this device.
