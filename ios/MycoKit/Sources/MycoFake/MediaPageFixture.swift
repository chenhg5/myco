/// Only the explicit `-myco-start media` demo: a page that plays a public
/// podcast through `myco.media.url` (the phone's own network, no hub), and
/// says what the player does, to check playing, pausing, seeking and
/// closing, and what the stream refuses.
enum MediaPageFixture {
    static let id = "pg_media"
    /// A publisher's public feed (BBC), read through the network bridge.
    static let feed = "https://podcasts.files.bbci.co.uk/p02nq0gn.rss"

    static let html = #"""
    <!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
    <style>body{font:16px -apple-system;padding:20px;color:#222;background:#fafafa}button{font:inherit;min-height:44px;margin:6px 6px 0 0;padding:6px 12px;border:1px solid #bbb;border-radius:10px;background:#fff;color:#222}p{margin:8px 0}</style></head><body>
    <h1>Media probe</h1>
    <p id="feed">Feed: reading</p><p id="state">State: idle</p><p id="time">Time: 0</p><p id="note">Note: none</p>
    <button id="play">Play</button><button id="pause">Pause</button><button id="seek">Seek 180</button>
    <p id="private">Private: not tried</p><p id="type">Type: not tried</p>
    <button id="tryPrivate">Try private</button><button id="tryType">Try feed as media</button>
    <audio id="player" preload="none"></audio>
    <script>
    var FEED = "FEED_URL", player = document.getElementById('player'), episode = null;
    function say(id, text) { document.getElementById(id).textContent = text; }
    ['playing','pause','waiting','seeked','ended'].forEach(function (name) {
      player.addEventListener(name, function () { say('state', 'State: ' + name); });
    });
    player.addEventListener('error', function () { say('state', 'State: error ' + (player.error && player.error.code)); });
    setInterval(function () { say('time', 'Time: ' + Math.floor(player.currentTime)); }, 300);
    if (!(myco.features || []).includes('net-media')) say('note', 'Note: no net-media');
    myco.net.fetch({ url: FEED, response: 'text' }).then(function (r) {
      var xml = new DOMParser().parseFromString(r.body, 'text/xml');
      var items = xml.querySelectorAll('item enclosure');
      episode = items.length ? items[0].getAttribute('url') : null;
      // `#track=<address>`: that instead (a network that cannot reach the
      // feed's audio directly still tests the stream).
      var given = new URLSearchParams(location.hash.slice(1)).get('track');
      if (given) episode = given;
      say('feed', 'Feed: ' + items.length + ' episodes' + (episode ? '' : ', none to play'));
    }).catch(function (e) { say('feed', 'Feed: failed ' + (e.message || e)); });
    document.getElementById('play').onclick = function () {
      if (!episode) return;
      if (!player.src) player.src = myco.media.url(episode);
      player.play().catch(function (e) { say('note', 'Note: play refused ' + e.name); });
    };
    document.getElementById('pause').onclick = function () { player.pause(); };
    document.getElementById('seek').onclick = function () { player.currentTime = 180; player.play(); };
    function tryStream(id, address) {
      fetch(myco.media.url(address)).then(function (r) {
        return r.text().then(function (t) { say(id, id[0].toUpperCase() + id.slice(1) + ': ' + r.status + ' ' + t.slice(0, 80)); });
      }).catch(function (e) { say(id, id[0].toUpperCase() + id.slice(1) + ': failed ' + (e.message || e)); });
    }
    document.getElementById('tryPrivate').onclick = function () { tryStream('private', 'http://192.168.1.1/a.mp3'); };
    document.getElementById('tryType').onclick = function () { tryStream('type', FEED); };
    </script></body></html>
    """#.replacingOccurrences(of: "FEED_URL", with: feed)
}
