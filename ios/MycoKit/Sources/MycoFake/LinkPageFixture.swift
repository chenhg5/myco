/// Only the explicit `-myco-start links` demo: a page that says what its
/// address's fragment is, how often it changed, how often it loaded, and
/// the address the bridge gives it, to check deep links into a page.
enum LinkPageFixture {
    static let id = "pg_cinema"

    static func link(episode: String) -> String {
        FakeHub.demoAddress.absoluteString + "/pages/\(id)#watch?source=src_1&movie=m42&line=线路一&episode=\(episode)"
    }

    static var html: String {
        #"""
        <!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
        <style>body{font:17px -apple-system;padding:24px;color:#222;background:#fafafa}a{display:block;margin:16px 0;font-size:20px}</style></head><body>
        <h1>Cinema probe</h1>
        <p id="episode">Episode: none</p><p id="changes">Changes: 0</p><p id="loads">Loads: 0</p><p id="url">URL: waiting</p>
        <a id="next" href="EPISODE2">Next episode</a>
        <script>
        var loads = Number(sessionStorage.getItem('loads') || 0) + 1; sessionStorage.setItem('loads', loads);
        document.getElementById('loads').textContent = 'Loads: ' + loads;
        var changes = 0;
        function show() {
          var params = new URLSearchParams(decodeURIComponent(location.hash.slice(1)).replace(/^watch\?/, ''));
          document.getElementById('episode').textContent = 'Episode: ' + (params.get('episode') || 'none') + ' / ' + (params.get('line') || '');
        }
        window.addEventListener('hashchange', function () { changes += 1; document.getElementById('changes').textContent = 'Changes: ' + changes; show(); });
        show();
        myco.ready.then(function (v) { document.getElementById('url').textContent = 'URL: ' + ((v.page && v.page.url) || 'none'); });
        </script></body></html>
        """#.replacingOccurrences(of: "EPISODE2", with: link(episode: "第2集"))
    }
}
