# Cinema app

Open **Apps → Cinema** to create a player. Add your own source under **Sources**:
enter a name and a MacCMS-compatible JSON API endpoint, test the connection, then
save it. No catalog or playback source is included, including in the example page.
The interface follows the App's language and appearance.

Choose a source to browse its catalog and categories. Search one source or choose
**All sources** to search every enabled source. Open a title, choose a playback
line and episode, and use the video controls to play. Add titles to **Watchlist**
for later. Sources and the watchlist are shared by people opening this page;
do not put credentials or API keys in source addresses. An agent can configure the
same sources through page data commands; changes appear immediately in the app:

```sh
myco page data put --page <pg_id> --collection sources --id my_catalog \
  --json '{"name":"My catalog","endpoint":"https://example.com/api.php/provide/vod/","enabled":true,"order":1}'
```

Replace the example domain with the endpoint supplied by the person. To edit an
existing source, read it first and write the complete record with `--if-version`,
keeping the same record ID. Set `enabled` false to disable it.

**Recently played** and resume positions are kept only on this computer, separately
for each page and account. They do not sync. The player also supports playback
speed, previous/next episode, fullscreen, Space to pause, and Left/Right to seek
five seconds. Intro/outro skip settings are per film, in seconds; zero disables
skipping. Outro skipping advances only when there is another episode.

The computer opening the myco App fetches catalogs, images and media through
`myco.os.net.fetch`; sources must be reachable there. No separate web server or
original qiaoqiao service is needed. Use an App with the OS bridge and a browser
engine supporting MediaSource/ManagedMediaSource and the source's codecs. HLS
playlists, keys and segments may be up to 32 MB each. Small MP4/WebM files up to
32 MB are supported as whole-file downloads; use HLS for full-length films.
DRM and third-party embedded players are unsupported. A failed source or stream
shows a retry action; choose another playback line if the codec is unsupported.

Endpoints must use HTTP(S), without username/password, query parameters or
fragments. The app adds MacCMS `ac=detail`, `pg`, `wd`, `t` and `ids` parameters.
The JSON response must have a `list` array; optional `class` and `pagecount`
provide categories and pagination. The source's film metadata is displayed as
plain text, and source scripts or web players are never executed.
