import { useEffect, useLayoutEffect, useRef, useState } from "react";
import type { RefObject } from "react";
import { cn } from "@/lib/utils";
import { L } from "@/lib/i18n";

/** One thing that was asked, and how the answer starts. `who` is set when
 * it was not you who asked (two nodes talking). */
export interface Turn {
  id: string;
  who?: string;
  question: string;
  answer: string;
}

/**
 * A rail of small ticks at the left of a conversation, one per turn (a thing
 * you asked, or a task one node gave another): where you are, what each turn was about
 * (hover a tick), and a way to jump there (click it). The tick for the turn
 * in view is longer; ticks swell under the pointer.
 *
 * Each turn's message carries data-turn="<id>" inside the scroller.
 */
export function TurnRail({ turns, scroller }: { turns: Turn[]; scroller: RefObject<HTMLDivElement> }) {
  const [active, setActive] = useState(turns.length - 1);
  const [hover, setHover] = useState<number | null>(null);
  const [cardTop, setCardTop] = useState(0);
  const rail = useRef<HTMLDivElement>(null);
  const card = useRef<HTMLDivElement>(null);
  const ids = turns.map((t) => t.id).join(",");

  // The turn in view: the last one whose own message has reached the top of
  // the pane (so the turn you jumped to, or the one whose answer you are
  // reading). Positions are compared with each other only, so the page zoom
  // of the text-size setting does not matter.
  useEffect(() => {
    const el = scroller.current;
    if (!el) return;
    const find = () => {
      const box = el.getBoundingClientRect();
      const line = box.top + Math.max(32, box.height * 0.08);
      let at = 0;
      turns.forEach((t, i) => {
        const node = el.querySelector(`[data-turn="${CSS.escape(t.id)}"]`);
        if (node && node.getBoundingClientRect().top <= line) at = i;
      });
      // At the very end the last turn is the one in view, however short.
      if (el.scrollHeight - el.scrollTop - el.clientHeight < 4) at = turns.length - 1;
      setActive(at);
    };
    find();
    el.addEventListener("scroll", find, { passive: true });
    window.addEventListener("resize", find);
    return () => {
      el.removeEventListener("scroll", find);
      window.removeEventListener("resize", find);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ids, scroller]);

  // The preview sits beside its tick, kept inside the pane.
  useLayoutEffect(() => {
    if (hover === null) return;
    const r = rail.current;
    const pane = r?.parentElement;
    const tick = r?.children[hover] as HTMLElement | undefined;
    if (!r || !pane || !tick) return;
    const h = card.current?.offsetHeight || 96;
    const center = r.offsetTop - r.offsetHeight / 2 + tick.offsetTop + tick.offsetHeight / 2;
    setCardTop(Math.max(8, Math.min(center - h / 2, pane.clientHeight - h - 8)));
  }, [hover, ids]);

  if (turns.length < 2) return null;
  const shown = hover !== null ? turns[hover] : null;

  const go = (t: Turn) => {
    scroller.current?.querySelector(`[data-turn="${CSS.escape(t.id)}"]`)?.scrollIntoView({ block: "start", behavior: "smooth" });
  };

  return (
    <>
      <div className="conv-rail" ref={rail} role="navigation" aria-label={L("这段对话的每一轮", "The turns of this conversation")} onMouseLeave={() => setHover(null)}>
        {turns.map((t, i) => (
          <button
            key={t.id}
            type="button"
            className={cn("conv-rail-tick", i === active && "is-active")}
            aria-label={t.question}
            aria-current={i === active ? "true" : undefined}
            onMouseEnter={() => setHover(i)}
            onFocus={() => setHover(i)}
            onBlur={() => setHover(null)}
            onClick={() => go(t)}
          />
        ))}
      </div>
      {shown && (
        <div className="conv-rail-card" ref={card} style={{ top: cardTop }} aria-hidden>
          {shown.who && <div className="conv-rail-who">{shown.who}</div>}
          <div className="conv-rail-q">{shown.question}</div>
          {shown.answer && <div className="conv-rail-a">{shown.answer}</div>}
        </div>
      )}
    </>
  );
}
