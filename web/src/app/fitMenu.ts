/**
 * Keeps a pop-up menu that opens from the message box's bar (model, access,
 * quota) inside the chat column: it opens at its pill's left, and where
 * that would run past the column's right edge (a narrow window, a members
 * panel beside it) it moves left as far as it must, and is never wider
 * than the column. Pass as the menu's ref.
 */
export function fitMenu(el: HTMLElement | null) {
  if (!el) return;
  el.style.left = "";
  el.style.maxWidth = "";
  const box = (el.closest(".conv-detail") as HTMLElement | null) || document.documentElement;
  const b = box.getBoundingClientRect();
  const gap = 8;
  const room = b.width - 2 * gap;
  if (el.offsetWidth > room) el.style.maxWidth = `${Math.max(160, room)}px`;
  const r = el.getBoundingClientRect();
  const over = r.right - (b.right - gap);
  if (over <= 0) return;
  // (not past the column's left edge either)
  const shift = Math.min(over, Math.max(0, r.left - (b.left + gap)));
  el.style.left = `${-shift}px`;
}
