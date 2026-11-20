import { useEffect, useRef, useState } from "react";

/*
 * ← → at the top of the window (AppMode's crumb bar), as in a browser:
 * back to where you were, and forward again. Kept in this window only.
 *
 * A place is AppMode's part (which workspace, which page, the project or
 * chats shown, the chat opened from the sidebar) plus, per page, what that
 * page has open inside it (a workflow and its run, an automation, a
 * computer): `sub`, keyed by page. AppMode records its part on every move
 * (navRecord); a page records what it opens (useNavSub). Going back or
 * forward hands the place to both to put back (onNavApply / useNavSub),
 * and what they set while putting it back is not a new move.
 */

export type NavPlace = {
  hub: string;
  page: string;
  /** the project (or "*" for the chats) shown: spaceKey */
  space?: string;
  /** the chat opened from the sidebar (navChat) */
  chat?: string | null;
  sub?: Record<string, unknown>;
};

const MAX = 100;
let stack: NavPlace[] = [];
let at = -1;
const EVENT = "myco-nav-changed";
const APPLY = "myco-nav-apply";

const sameBase = (a: NavPlace, b: NavPlace) => a.hub === b.hub && a.page === b.page && (a.space || "") === (b.space || "") && (a.chat || "") === (b.chat || "");
const changed = () => window.dispatchEvent(new CustomEvent(EVENT));

function push(p: NavPlace) {
  stack = [...stack.slice(0, at + 1), p].slice(-MAX);
  at = stack.length - 1;
  changed();
}

/** AppMode's part of where you are: a new place when it differs. */
export function navRecord(base: Omit<NavPlace, "sub">) {
  const cur = stack[at];
  if (cur && sameBase(cur, base)) return;
  push({ ...base, sub: {} });
}

/** A page's own part (what it has open): a new place when it differs. */
export function navSub(page: string, value: unknown) { recordSub(page, value); }
function recordSub(page: string, value: unknown) {
  const cur = stack[at];
  if (!cur || cur.page !== page) return;
  if (JSON.stringify(cur.sub?.[page] ?? null) === JSON.stringify(value ?? null)) return;
  push({ ...cur, sub: { ...cur.sub, [page]: value } });
}

export function canBack() { return at > 0; }
export function canForward() { return at < stack.length - 1; }

function go(by: -1 | 1) {
  const next = at + by;
  if (next < 0 || next >= stack.length) return;
  at = next;
  changed();
  window.dispatchEvent(new CustomEvent<NavPlace>(APPLY, { detail: stack[at] }));
}
export const navBack = () => go(-1);
export const navForward = () => go(1);

/** Whether ← and → can go anywhere now (the buttons' state). */
export function useNavState(): { back: boolean; forward: boolean } {
  const [, setN] = useState(0);
  useEffect(() => {
    const on = () => setN((n) => n + 1);
    window.addEventListener(EVENT, on);
    return () => window.removeEventListener(EVENT, on);
  }, []);
  return { back: canBack(), forward: canForward() };
}

/** A place to put back (AppMode's part). */
export function onNavApply(f: (p: NavPlace) => void): () => void {
  const on = (e: Event) => f((e as CustomEvent<NavPlace>).detail);
  window.addEventListener(APPLY, on);
  return () => window.removeEventListener(APPLY, on);
}

/**
 * A page's own part: `value` is what it has open now (plain data, `empty`
 * when nothing is); each change is recorded as a place. Going back or
 * forward to a place on this page calls `apply` with what it had open
 * (`empty` when nothing), and that change is not recorded again.
 */
export function useNavSub<T>(page: string, value: T, empty: T, apply: (v: T) => void) {
  const seen = useRef(JSON.stringify(value));
  const applyRef = useRef(apply);
  applyRef.current = apply;
  // Mounted by going back or forward to a place on this page (AppMode put
  // the page back first): what it had open, too.
  useEffect(() => {
    const cur = stack[at];
    if (!cur || cur.page !== page || cur.sub?.[page] == null) return;
    const v = cur.sub[page] as T;
    if (JSON.stringify(v) === seen.current) return;
    seen.current = JSON.stringify(v);
    applyRef.current(v);
  }, []); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    const key = JSON.stringify(value);
    if (key === seen.current) return;
    seen.current = key;
    recordSub(page, value);
  }, [page, value]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => onNavApply((p) => {
    if (p.page !== page) return;
    const v = (p.sub?.[page] as T | undefined) ?? empty;
    seen.current = JSON.stringify(v);
    applyRef.current(v);
  }), [page]); // eslint-disable-line react-hooks/exhaustive-deps
}

/** For tests: start again. */
export function navReset() { stack = []; at = -1; changed(); }
