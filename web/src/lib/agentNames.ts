import { useSyncExternalStore } from "react";

/*
 * Agents' own names (cloud-7, pm-codex…) in what people and agents write:
 * the App shows each as the agent's name for people, a link that opens its
 * chat (Markdown). The names are the current workspace's agents, set by
 * the App shell (AppMode) as its list changes.
 */
export type AgentNames = { hubId: string; names: Map<string, string> };

let current: AgentNames | null = null;
const subs = new Set<() => void>();

export function setAgentNames(next: AgentNames | null) {
  const same = current && next && current.hubId === next.hubId && current.names.size === next.names.size
    && [...next.names].every(([k, v]) => current!.names.get(k) === v);
  if (same || (!current && !next)) return;
  current = next;
  subs.forEach((f) => f());
}

export function useAgentNames(): AgentNames | null {
  return useSyncExternalStore((f) => { subs.add(f); return () => subs.delete(f); }, () => current);
}

/** Only a name that cannot be an ordinary word: it has a digit, a dash or
 *  an underscore in it (cloud-7, pm-codex; not "coder"), three or more
 *  characters. */
export function linkableName(name: string): boolean {
  return name.length >= 3 && /[-_0-9]/.test(name) && /^[A-Za-z0-9][A-Za-z0-9_.-]*$/.test(name);
}

/** Short names people write for an agent: a prefix of its own name cut at
 *  a dash (cloud-7 for cloud-7-claude-rmv6), with a digit in it, that is
 *  no other agent's name and starts no other agent's name. By short name,
 *  the full one. */
const aliasCache = new WeakMap<Map<string, string>, Map<string, string>>();
export function shortNames(names: Map<string, string>): Map<string, string> {
  const had = aliasCache.get(names);
  if (had) return had;
  const count = new Map<string, string[]>();
  for (const name of names.keys()) {
    const parts = name.split("-");
    for (let k = 2; k < parts.length; k++) {
      const p = parts.slice(0, k).join("-");
      count.set(p, [...(count.get(p) || []), name]);
    }
  }
  const out = new Map<string, string>();
  for (const [p, owners] of count) {
    if (owners.length === 1 && !names.has(p) && /\d/.test(p) && linkableName(p)) out.set(p, owners[0]);
  }
  aliasCache.set(names, out);
  return out;
}

const SKIP = new Set(["A", "CODE", "PRE", "KBD", "BUTTON", "TEXTAREA", "SCRIPT", "STYLE", "SVG"]);

/** The HTML with each agent name in its text (not in code, links, paths
 *  or addresses) as a link: the agent's name for people, its own name on
 *  hover. "@cloud-7" goes the same way. */
export function linkAgentNames(html: string, an: AgentNames | null): string {
  if (!an || !an.names.size || typeof document === "undefined") return html;
  const short = shortNames(an.names);
  const list = [...an.names.keys(), ...short.keys()].filter(linkableName).sort((a, b) => b.length - a.length);
  if (!list.length || !list.some((n) => html.includes(n))) return html;
  const esc = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  // (not inside a word, a path, an address or @console:x; a full stop
  // after it ends the sentence, ".md" after it is a file)
  const re = new RegExp(`(?<![A-Za-z0-9_\\-/.:@])@?(${list.map(esc).join("|")})(?![A-Za-z0-9_\\-/]|\\.[A-Za-z0-9])`, "g");
  const box = document.createElement("template");
  box.innerHTML = html;
  const walker = document.createTreeWalker(box.content, NodeFilter.SHOW_TEXT);
  const texts: Text[] = [];
  for (let n = walker.nextNode(); n; n = walker.nextNode()) {
    let skip = false;
    for (let p = n.parentElement; p; p = p.parentElement) {
      if (SKIP.has(p.tagName.toUpperCase()) || p.classList.contains("md-mermaid")) { skip = true; break; }
    }
    if (!skip && re.test(n.nodeValue || "")) texts.push(n as Text);
    re.lastIndex = 0;
  }
  if (!texts.length) return html;
  for (const t of texts) {
    const s = t.nodeValue || "";
    const frag = document.createDocumentFragment();
    let at = 0;
    for (const m of s.matchAll(re)) {
      const start = m.index ?? 0;
      if (start > at) frag.append(s.slice(at, start));
      const name = an.names.has(m[1]) ? m[1] : short.get(m[1]) || m[1];
      const a = document.createElement("a");
      a.className = "md-agent";
      a.setAttribute("role", "link");
      a.setAttribute("tabindex", "0");
      a.dataset.agent = name;
      a.dataset.hub = an.hubId;
      a.title = name;
      a.textContent = an.names.get(name) || name;
      frag.append(a);
      at = start + m[0].length;
    }
    if (at < s.length) frag.append(s.slice(at));
    t.replaceWith(frag);
  }
  return box.innerHTML;
}

/** Opens the chat of the agent a link names. */
export function openAgentLink(el: HTMLElement) {
  const agent = el.dataset.agent;
  const hubId = el.dataset.hub;
  if (agent && hubId) window.dispatchEvent(new CustomEvent("myco-open-member", { detail: { hubId, agent } }));
}
