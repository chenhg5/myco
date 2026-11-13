import { useRef } from "react";

/*
 * ↑ / ↓ in a chat box go through what you sent before, newest first, as a
 * terminal does. It starts from an empty box only, so it never takes the
 * arrow keys from text you are writing; while you go through them, ↑ works
 * from the first line and ↓ from the last (a long message still moves
 * the caret inside it); ↓ past the newest gives the empty box back. Typing
 * in a recalled message makes it yours to send, and the keys move the
 * caret again.
 */
export function useInputHistory(items: string[], draft: string, setDraft: (v: string) => void) {
  const pos = useRef(-1);

  /** Call when the text changes by typing: browsing ends. */
  const reset = () => { pos.current = -1; };

  /** Handles ↑ / ↓; true when it did (the caller stops there). */
  const onKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>): boolean => {
    if (e.key !== "ArrowUp" && e.key !== "ArrowDown") return false;
    if (e.altKey || e.metaKey || e.ctrlKey || e.shiftKey || e.nativeEvent.isComposing || items.length === 0) return false;
    const el = e.currentTarget;
    const value = el.value;
    if (pos.current === -1 && (e.key === "ArrowDown" || value.trim() !== "")) return false;
    if (e.key === "ArrowUp" && value.slice(0, el.selectionStart).includes("\n")) return false;
    if (e.key === "ArrowDown" && value.slice(el.selectionEnd).includes("\n")) return false;
    e.preventDefault();
    let next = pos.current;
    if (e.key === "ArrowUp") next = next === -1 ? items.length - 1 : Math.max(0, next - 1);
    else next = next + 1 < items.length ? next + 1 : -1;
    pos.current = next;
    const text = next === -1 ? "" : items[next];
    setDraft(text);
    // The caret after the text, once it is in the box.
    requestAnimationFrame(() => {
      el.selectionStart = el.selectionEnd = text.length;
    });
    return true;
  };

  return { onKeyDown, reset, browsing: () => pos.current !== -1, draft };
}

/** What you sent, oldest first, without a message repeated right after itself. */
export function sentTexts(bodies: string[], max = 50): string[] {
  const out: string[] = [];
  for (const b of bodies) {
    const t = b.trim();
    if (t && out[out.length - 1] !== t) out.push(t);
  }
  return out.slice(-max);
}

const SENT_KEY = "myco-sent";

/** What was sent from a box that has no conversation of its own to read
 *  them from (the group box), kept in this App. */
export function keptSent(key: string): string[] {
  try {
    const v: unknown = JSON.parse(localStorage.getItem(`${SENT_KEY}\u0000${key}`) || "[]");
    return Array.isArray(v) ? v.filter((x): x is string => typeof x === "string") : [];
  } catch {
    return [];
  }
}

export function keepSent(key: string, text: string) {
  const list = sentTexts([...keptSent(key), text]);
  try { localStorage.setItem(`${SENT_KEY}\u0000${key}`, JSON.stringify(list)); } catch { /* not kept */ }
}
