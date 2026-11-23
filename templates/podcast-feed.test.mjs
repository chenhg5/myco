import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { createRequire } from "node:module";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { build } from "esbuild";

const require = createRequire(join(execFileSync("npm", ["root", "-g"], { encoding: "utf8" }).trim(), "noop.js"));
const { chromium } = require("playwright");
const bundle = await build({
  entryPoints: [fileURLToPath(new URL("./src/podcast/feed.ts", import.meta.url))],
  bundle: true, write: false, format: "iife", globalName: "PodcastFeed",
});
const browser = await chromium.launch({ executablePath: process.env.UI_SMOKE_EXECUTABLE });
try {
  const page = await browser.newPage();
  const requests = [];
  await page.route("**/*", route => {
    requests.push(route.request().url());
    return route.abort();
  });
  await page.evaluate(() => {
    window.myco = { features: ["net"], net: { fetch: async options => {
      window.requests.push(options);
      return window.feedResponse;
    } } };
    window.requests = [];
  });
  await page.addScriptTag({ content: bundle.outputFiles[0].text });

  const rss = [
    '<rss version="2.0" xmlns:p="http://www.itunes.com/dtds/podcast-1.0.dtd" xmlns:m="http://search.yahoo.com/mrss/">',
    '<channel xml:base="/public/"><title>早间节目</title><p:author>Host</p:author><language>zh-CN</language>',
    '<link>home</link><p:image href="cover.png"/>',
    '<description><![CDATA[<p>Hello &amp; <b>goodbye</b></p><p>Second line</p><script>window.feedExecuted=true</script>',
    '<img src="https://example.net/pixel" onerror="window.feedExecuted=true"><iframe src="https://example.net/frame">Hidden</iframe>]]></description>',
    '<item xml:base="entries/" xml:lang="en"><title>Pilot</title><guid isPermaLink="false">episode-one</guid>',
    '<link>./one</link><p:image href="javascript:alert(1)"/><p:duration>1:02:03</p:duration>',
    '<enclosure url="data:text/plain,no" type="audio/mpeg"/><enclosure url="document.pdf" type="application/pdf"/>',
    '<m:group><m:content url="track.mp3" type="audio/mpeg"/></m:group>',
    '<pubDate>Sat, 22 Nov 2025 12:34:00 +0000</pubDate></item>',
    '<item><title>Duplicate</title><guid>episode-one</guid><enclosure url="duplicate.ogg" type="audio/ogg"/></item>',
    '<item><title>Non-audio</title><enclosure url="poster.png"/></item>',
    '<item><title>Second</title><enclosure url="//cdn.example.org/2.opus?a=1&amp;b=2"/><pubDate>invalid</pubDate></item>',
    '</channel></rss>',
  ].join("");
  const report = await page.evaluate(source => {
    const parser = new DOMParser();
    let calls = 0;
    const inject = { parseFromString: (...args) => { calls++; return parser.parseFromString(...args); } };
    const show = PodcastFeed.parseFeed(source, "https://example.com/feed.xml", inject);
    return { show, calls, executed: window.feedExecuted === true };
  }, rss);
  assert.equal(report.calls, 2);
  assert.equal(report.executed, false);
  assert.equal(report.show.description, "Hello & goodbye Second line");
  assert.equal(report.show.link, "https://example.com/public/home");
  assert.equal(report.show.episodes.length, 2);
  assert.deepEqual(report.show.episodes[0], {
    guid: "episode-one", title: "Pilot", showTitle: "早间节目", author: "Host",
    artworkUrl: "https://example.com/public/cover.png", link: "https://example.com/public/entries/one",
    description: "", audioUrl: "https://example.com/public/entries/track.mp3",
    duration: 3723, publishedAt: "2025-11-22T12:34:00.000Z", language: "en",
  });
  assert.equal(report.show.episodes[1].guid, "https://cdn.example.org/2.opus?a=1&b=2");
  assert.equal(report.show.episodes[1].publishedAt, "");
  assert.equal(report.show.episodes[1].duration, null);

  const atom = [
    '<a:feed xmlns:a="http://www.w3.org/2005/Atom" xml:lang="fr" xml:base="/feeds/">',
    '<a:title>Épisodes</a:title><a:author><a:name>Auteur</a:name></a:author><a:logo>../cover.jpg</a:logo>',
    '<a:entry xml:base="season/"><a:id>urn:one</a:id><a:title>Title</a:title>',
    '<a:link rel="alternate" href="java&#x9;script:alert(1)"/><a:link href="episode.html"/>',
    '<a:link rel="enclosure" href="https://user:pass@example.org/one.mp3"/>',
    '<a:link rel="enclosure" type="video/mp4" href="movie.mp4"/>',
    '<a:link rel="enclosure" type="audio/mp4" href="audio.m4a"/>',
    '<a:summary type="xhtml"><div xmlns="http://www.w3.org/1999/xhtml"><p>First</p><p><b>Second</b></p></div></a:summary>',
    '<a:updated>2025-04-02T12:00:00Z</a:updated></a:entry></a:feed>',
  ].join("");
  const atomShow = await page.evaluate(xml => PodcastFeed.parseFeed(xml, "https://example.com/feed"), atom);
  assert.equal(atomShow.author, "Auteur");
  assert.deepEqual(atomShow.episodes[0], {
    guid: "urn:one", title: "Title", showTitle: "Épisodes", author: "Auteur",
    artworkUrl: "https://example.com/cover.jpg", link: "https://example.com/feeds/season/episode.html",
    description: "First Second", audioUrl: "https://example.com/feeds/season/audio.m4a", duration: null,
    publishedAt: "2025-04-02T12:00:00.000Z", language: "fr",
  });

  const bounds = await page.evaluate(() => {
    const code = callback => { try { callback(); } catch (error) { return error.code; } };
    const url = "https://example.org/feed";
    const items = Array.from({ length: 250 }, (_, i) =>
      '<item><enclosure url="' + i + '.mp3"/></item>').join("");
    return {
      episodes: PodcastFeed.parseFeed("<rss><channel>" + items + "</channel></rss>", url).episodes.length,
      malformed: code(() => PodcastFeed.parseFeed("<rss><channel></rss>", url)),
      html: code(() => PodcastFeed.parseFeed("<html><head/></html>", url)),
      entities: code(() => PodcastFeed.parseFeed('<!DOCTYPE rss [<!ENTITY external SYSTEM "https://example.org/xxe">]><rss><channel>&external;</channel></rss>', url)),
      size: code(() => PodcastFeed.parseFeed("播".repeat(900000), url)),
      scheme: code(() => PodcastFeed.parseFeed("<rss><channel/></rss>", "file:///tmp/feed")),
    };
  });
  assert.deepEqual(bounds, {
    episodes: 200, malformed: "invalid_feed", html: "invalid_feed", entities: "invalid_feed",
    size: "feed_too_large", scheme: "invalid_url",
  });

  const fetchReport = await page.evaluate(async xml => {
    const url = "https://example.com/feed";
    const code = async call => { try { await call(); } catch (error) { return [typeof error.code === "string" ? error.code : error.name, error.status]; } };
    window.feedResponse = { status: 200, url: "https://redirect.example.org/list/feed.xml", body: xml };
    const redirected = await PodcastFeed.fetchFeed(url);
    window.feedResponse = { ...window.feedResponse, truncated: true };
    const truncated = await code(() => PodcastFeed.fetchFeed(url));
    window.feedResponse = { status: 503 };
    const badStatus = await code(() => PodcastFeed.fetchFeed(url));

    const controller = new AbortController();
    controller.abort();
    const before = window.requests.length;
    const preAborted = await code(() => PodcastFeed.fetchFeed(url, controller.signal));
    const noRequest = window.requests.length === before;
    const active = new AbortController();
    window.feedResponse = new Promise(() => {});
    const inflight = code(() => PodcastFeed.fetchFeed(url, active.signal));
    await Promise.resolve();
    active.abort();
    const aborted = await inflight;
    const originalTimer = window.setTimeout;
    window.setTimeout = (callback, delay, ...args) => originalTimer(callback, delay === 12000 ? 5 : delay, ...args);
    const timedOut = await code(() => PodcastFeed.fetchFeed(url));
    window.setTimeout = originalTimer;
    return { request: window.requests[0], feedUrl: redirected.feedUrl, audio: redirected.episodes[0].audioUrl,
      truncated, badStatus, noRequest, preAborted, aborted, timedOut };
  }, rss);
  assert.equal(fetchReport.request.response, "text");
  assert.equal(fetchReport.request.timeout_ms, 12000);
  assert.equal(fetchReport.request.max_bytes, 2 * 1024 * 1024);
  assert.match(fetchReport.request.headers.Accept, /application\/atom\+xml/);
  assert.equal(fetchReport.feedUrl, "https://redirect.example.org/list/feed.xml");
  assert.equal(fetchReport.audio, "https://redirect.example.org/public/entries/track.mp3");
  assert.deepEqual(fetchReport.truncated, ["feed_too_large", undefined]);
  assert.deepEqual(fetchReport.badStatus, ["http_error", 503]);
  assert.deepEqual(fetchReport.preAborted, ["AbortError", undefined]);
  assert.deepEqual(fetchReport.aborted, ["AbortError", undefined]);
  assert.deepEqual(fetchReport.timedOut, ["timeout", undefined]);
  assert.equal(fetchReport.noRequest, true);
  assert.deepEqual(requests, []);
  console.log("PASS podcast RSS and Atom normalization, inert descriptions, feed limits and bridge cancellation");
} finally {
  await browser.close();
}
