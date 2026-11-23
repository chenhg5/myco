// Normalize RSS and Atom into display text and validated HTTP(S) URLs.
// Public feeds are loaded through the App's network bridge.
import { network } from "../_lib/net";

export type Episode = {
  guid: string;
  title: string;
  showTitle: string;
  author: string;
  artworkUrl: string;
  link: string;
  description: string;
  audioUrl: string;
  duration: number | null;
  publishedAt: string;
  language: string;
};

export type Show = {
  feedUrl: string;
  title: string;
  author: string;
  artworkUrl: string;
  link: string;
  description: string;
  language: string;
  episodes: Episode[];
};

export const MAX_FEED_BYTES = 2 * 1024 * 1024;
export const MAX_FEED_EPISODES = 200;
export const FEED_TIMEOUT_MS = 12000;
type FeedErrorCode = "invalid_url" | "invalid_feed" | "feed_too_large" | "http_error" | "timeout";
export class FeedError extends Error {
  constructor(readonly code: FeedErrorCode, readonly status?: number) { super(code); }
}

const xmlNS = "http://www.w3.org/XML/1998/namespace";
const atomNS = "http://www.w3.org/2005/Atom";
const rdfNS = "http://www.w3.org/1999/02/22-rdf-syntax-ns#";
const mediaNS = "http://search.yahoo.com/mrss";
const itunesNS = "http://www.itunes.com/dtds/podcast-1.0.dtd";
type Parser = Pick<DOMParser, "parseFromString">;

function namespace(element: Element, uri: string) {
  return element.namespaceURI?.replace(/\/$/, "") === uri.replace(/\/$/, "");
}

function children(element: Element, name: string) {
  return Array.from(element.children).filter(child => child.localName === name);
}

function child(element: Element, name: string, ns = element.namespaceURI) {
  return children(element, name).find(node => node.namespaceURI === ns);
}

function value(element: Element | undefined) { return element?.textContent?.trim() || ""; }

function httpURL(value: string, base?: string) {
  if (!value.trim() || /[\u0000-\u001f\u007f]/.test(value)) return "";
  try {
    const url = new URL(value.trim(), base);
    return /^https?:$/.test(url.protocol) && !url.username && !url.password ? url.href : "";
  } catch { return ""; }
}

function nodeURL(value: string, node: Element, feedUrl: string) {
  const bases: string[] = [];
  for (let element: Element | null = node; element; element = element.parentElement) {
    const base = element.getAttributeNS(xmlNS, "base");
    if (base) bases.push(base);
  }
  const base = bases.reverse().reduce((address, next) => address && httpURL(next, address), feedUrl);
  // Absolute URLs do not rely on a valid xml:base.
  return httpURL(value, base || undefined);
}

function attrURL(node: Element, key: string, feedUrl: string) {
  return nodeURL(node.getAttribute(key) || "", node, feedUrl);
}

function textURL(node: Element | undefined, feedUrl: string) {
  return node ? nodeURL(value(node), node, feedUrl) : "";
}

function language(node: Element, inherited = "") {
  return node.getAttributeNS(xmlNS, "lang")?.trim() || value(child(node, "language")) ||
    value(child(node, "language", "http://purl.org/dc/elements/1.1/")) || inherited;
}

function author(node: Element) {
  const field = child(node, "author");
  return value(field && child(field, "name")) || value(field) ||
    value(child(node, "author", itunesNS)) || value(child(node, "creator", "http://purl.org/dc/elements/1.1/"));
}

function artwork(node: Element, feedUrl: string) {
  const image = child(node, "image", itunesNS);
  const rssImage = child(node, "image");
  const thumbnail = children(node, "thumbnail").find(element => namespace(element, mediaNS));
  return (image && attrURL(image, "href", feedUrl)) || (thumbnail && attrURL(thumbnail, "url", feedUrl)) ||
    textURL(rssImage && child(rssImage, "url"), feedUrl) ||
    textURL(child(node, "logo"), feedUrl) || textURL(child(node, "icon"), feedUrl);
}

function link(node: Element, feedUrl: string, atom: boolean) {
  for (const element of children(node, "link")) {
    if (element.namespaceURI !== node.namespaceURI) continue;
    if (atom && !["", "alternate"].includes(element.getAttribute("rel") || "")) continue;
    const url = atom ? attrURL(element, "href", feedUrl) : textURL(element, feedUrl);
    if (url) return url;
  }
  return "";
}

function duration(value: string) {
  if (!/^\d+(?::[0-5]\d){0,2}(?:\.\d+)?$/.test(value)) return null;
  const seconds = value.split(":").reduce((total, part) => total * 60 + Number(part), 0);
  return Number.isFinite(seconds) ? seconds : null;
}

const blockedText = new Set(["script", "style", "template", "noscript", "iframe", "object"]);
const blockText = new Set(["br", "p", "div", "li", "blockquote", "tr", "section", "h1", "h2", "h3"]);
function plainText(root: Node) {
  const parts: string[] = [];
  // Use a stack rather than recursing into potentially deeply nested markup.
  const stack: (Node | string)[] = [root];
  while (stack.length) {
    const node = stack.pop()!;
    if (typeof node === "string") { parts.push(node); continue; }
    if (node.nodeType === 3 || node.nodeType === 4) { parts.push(node.nodeValue || ""); continue; }
    const name = (node as Element).localName?.toLowerCase();
    if (blockedText.has(name)) continue;
    const block = blockText.has(name);
    if (block) { parts.push(" "); stack.push(" "); }
    for (let child = node.lastChild; child; child = child.previousSibling) stack.push(child);
  }
  return parts.join("").replace(/\s+/g, " ").trim();
}

function description(node: Element, atom: boolean, template: HTMLTemplateElement) {
  for (const name of atom ? ["summary", "subtitle", "content"] : ["description", "encoded", "summary"]) {
    const field = name === "encoded"
      ? child(node, name, "http://purl.org/rss/1.0/modules/content/")
      : name === "summary" && !atom ? child(node, name, itunesNS) : child(node, name);
    if (!field) continue;
    if (field.children.length) return plainText(field);
    const text = value(field);
    if (!text) continue;
    if (atom && (field.getAttribute("type") || "text") === "text") return text.replace(/\s+/g, " ");
    // Template contents are inert. We read text without inserting any nodes
    // into the app document or following links or image URLs in the markup.
    template.innerHTML = text;
    return plainText(template.content);
  }
  return "";
}

function audio(node: Element, feedUrl: string, atom: boolean) {
  const groups = children(node, "group").filter(element => namespace(element, mediaNS));
  const media = [node, ...groups].flatMap(group => children(group, "content").filter(element => namespace(element, mediaNS)));
  const enclosures = atom ? children(node, "link").filter(element =>
    element.namespaceURI === node.namespaceURI && element.getAttribute("rel") === "enclosure")
    : children(node, "enclosure");
  for (const element of [...enclosures, ...media]) {
    const type = (element.getAttribute("type") || "").toLowerCase().split(";")[0].trim();
    const medium = (element.getAttribute("medium") || "").toLowerCase();
    if (medium && medium !== "audio") continue;
    if (type && !type.startsWith("audio/") && !["application/octet-stream", "binary/octet-stream", "application/ogg"].includes(type)) continue;
    const url = attrURL(element, atom && element.localName === "link" ? "href" : "url", feedUrl) ||
      nodeURL(element.getAttributeNS(rdfNS, "resource") || "", element, feedUrl);
    if (!url) continue;
    // Missing/generic MIME types are common; reject obvious non-audio attachments.
    if (!type.startsWith("audio/") && /\.(pdf|png|jpe?g|webp|gif|svg|mp4|webm|mov|m4v)$/i.test(new URL(url).pathname)) continue;
    return { url, duration: duration(element.getAttribute("duration") || "") };
  }
  return undefined;
}

function publishedAt(node: Element, atom: boolean) {
  const raw = atom ? value(child(node, "published")) || value(child(node, "updated")) :
    value(child(node, "pubDate")) || value(child(node, "date", "http://purl.org/dc/elements/1.1/"));
  const date = Date.parse(raw);
  return Number.isFinite(date) ? new Date(date).toISOString() : "";
}

export function parseFeed(source: string, url: string, parser: Parser = new DOMParser()): Show {
  const feedUrl = httpURL(url);
  if (!feedUrl) throw new FeedError("invalid_url");
  if (source.length > MAX_FEED_BYTES || new TextEncoder().encode(source).length > MAX_FEED_BYTES) throw new FeedError("feed_too_large");
  if (/<!DOCTYPE\s/i.test(source)) throw new FeedError("invalid_feed");
  const doc = parser.parseFromString(source, "application/xml");
  const root = doc.documentElement;
  if (!root || doc.doctype || ["http://www.mozilla.org/newlayout/xml/parsererror.xml", "http://www.w3.org/1999/xhtml"]
    .some(ns => doc.getElementsByTagNameNS(ns, "parsererror").length))
    throw new FeedError("invalid_feed");
  const atom = root.localName === "feed" && (!root.namespaceURI || root.namespaceURI === atomNS);
  const rdf = root.localName === "RDF" && namespace(root, rdfNS);
  const channel = atom ? root : root.localName === "rss" || rdf ? children(root, "channel")[0] : undefined;
  if (!channel) throw new FeedError("invalid_feed");
  const template = parser.parseFromString("", "text/html").createElement("template");
  const show: Show = {
    feedUrl, title: value(child(channel, "title")), author: author(channel),
    artworkUrl: artwork(channel, feedUrl), link: link(channel, feedUrl, atom),
    description: description(channel, atom, template), language: language(channel, language(root)),
    episodes: [],
  };
  const seen = new Set<string>();
  for (const item of (rdf ? root : channel).children) {
    if (item.localName !== (atom ? "entry" : "item") || item.namespaceURI !== channel.namespaceURI) continue;
    const selectedAudio = audio(item, feedUrl, atom);
    if (!selectedAudio) continue;
    const guid = value(child(item, atom ? "id" : "guid")) || (rdf && item.getAttributeNS(rdfNS, "about")) || selectedAudio.url;
    if (seen.has(guid)) continue;
    seen.add(guid);
    show.episodes.push({
      guid, title: value(child(item, "title")), showTitle: show.title, author: author(item) || show.author,
      artworkUrl: artwork(item, feedUrl) || show.artworkUrl, link: link(item, feedUrl, atom),
      description: description(item, atom, template), audioUrl: selectedAudio.url,
      duration: duration(value(child(item, "duration", itunesNS))) ?? selectedAudio.duration,
      publishedAt: publishedAt(item, atom), language: language(item, show.language),
    });
    if (show.episodes.length === MAX_FEED_EPISODES) break;
  }
  return show;
}

export async function fetchFeed(url: string, signal?: AbortSignal): Promise<Show> {
  const address = httpURL(url);
  if (!address) throw new FeedError("invalid_url");
  if (signal?.aborted) throw signal.reason ?? new DOMException("Aborted", "AbortError");
  let timer: ReturnType<typeof setTimeout> | undefined;
  let abort: (() => void) | undefined;
  try {
    const cancelled = new Promise<never>((_, reject) => {
      abort = () => reject(signal?.reason ?? new DOMException("Aborted", "AbortError"));
      signal?.addEventListener("abort", abort, { once: true });
      timer = setTimeout(() => reject(new FeedError("timeout")), FEED_TIMEOUT_MS);
    });
    // The bridge has no AbortSignal. Cancel the caller's wait and give the
    // underlying request the same deadline so abandoned requests stay bounded.
    const result = await Promise.race([cancelled, Promise.resolve().then(() => {
      if (signal?.aborted) throw signal.reason ?? new DOMException("Aborted", "AbortError");
      return network().fetch({
        url: address, response: "text", timeout_ms: FEED_TIMEOUT_MS, max_bytes: MAX_FEED_BYTES,
        headers: { Accept: "application/rss+xml, application/atom+xml, application/xml, text/xml, */*;q=0.1" },
      });
    })]);
    if (signal?.aborted) throw signal.reason ?? new DOMException("Aborted", "AbortError");
    if (result.status < 200 || result.status >= 300) throw new FeedError("http_error", result.status);
    if (result.truncated) throw new FeedError("feed_too_large");
    return parseFeed(result.body, result.url || address);
  } finally {
    clearTimeout(timer);
    if (abort) signal?.removeEventListener("abort", abort);
  }
}
