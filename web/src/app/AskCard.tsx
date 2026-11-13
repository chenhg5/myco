import { useState } from "react";
import { Check, CircleHelp, CornerDownLeft } from "lucide-react";
import type { Message } from "@/api";
import { MessageContent } from "@/app/Markdown";
import { MessageAttachments } from "@/app/attachments";
import { L, locale } from "@/lib/i18n";
import { cn } from "@/lib/utils";

/*
 * A question an agent asked the task's sender (myco message ask): the
 * question, its choices to click, and a line to answer in words. The task
 * waits until it is answered; the answer (a steer) resumes the same
 * session. Answered, the card keeps what was chosen. Someone watching
 * another's task sees it without the buttons.
 */

/** The question and its choices: the text before the trailing numbered
 *  list, and the list's items ("label — detail"). */
export function parseAsk(body: string): { question: string; options: { label: string; detail: string }[] } {
  const lines = body.replace(/\s+$/, "").split("\n");
  let start = lines.length;
  while (start > 0 && /^\s*\d+\.\s/.test(lines[start - 1])) start--;
  const options = lines.slice(start).map((l) => {
    const text = l.replace(/^\s*\d+\.\s+/, "").trim();
    const i = text.indexOf(" — ");
    return i > 0 ? { label: text.slice(0, i).trim(), detail: text.slice(i + 3).trim() } : { label: text, detail: "" };
  });
  return { question: lines.slice(0, start).join("\n").trim(), options };
}

function clock(iso: string): string {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? "" : d.toLocaleTimeString(locale(), { hour: "2-digit", minute: "2-digit", hour12: false });
}

export function AskCard({ hubId, ask, answer, open, canAnswer, onAnswer }: {
  hubId: string;
  ask: Message;
  /** the steer that answered it, once there is one */
  answer?: Message;
  /** the task still waits on this question */
  open: boolean;
  /** you sent the task: you may answer */
  canAnswer: boolean;
  onAnswer?: (text: string) => Promise<void>;
}) {
  const { question, options } = parseAsk(ask.body);
  const [text, setText] = useState("");
  const [sending, setSending] = useState<string | null>(null);
  const [error, setError] = useState("");
  const live = open && canAnswer && !answer && !!onAnswer;
  const chosen = answer ? options.findIndex((o) => answer.body.trim() === o.label || answer.body.trim().startsWith(`${o.label}\n`)) : -1;

  const send = async (value: string) => {
    if (!onAnswer || !value.trim() || sending) return;
    setSending(value);
    setError("");
    try {
      await onAnswer(value.trim());
      setText("");
    } catch (e) {
      setError((e as Error)?.message || L("没发出去", "Not sent"));
    } finally {
      setSending(null);
    }
  };

  return (
    <div className={cn("ask", live && "is-live", answer && "is-answered")} data-ask={ask.id}>
      <div className="ask-head">
        <CircleHelp className="ask-icon" />
        <span className="ask-title">
          {answer ? L("问过你", "Asked you") : open ? (canAnswer ? L("需要你决定", "Needs your decision") : L("在等对方决定", "Waiting for a decision")) : L("问过", "Asked")}
        </span>
        <time className="ask-time" dateTime={ask.created_at}>{clock(ask.created_at)}</time>
      </div>
      <div className="ask-question"><MessageContent body={question || ask.body} /></div>
      {ask.attachments && ask.attachments.length > 0 && <MessageAttachments hubId={hubId} items={ask.attachments} />}
      {options.length > 0 && (
        <div className="ask-options" role="group" aria-label={L("选一个", "Choose one")}>
          {options.map((o, i) => (
            <button key={i} type="button" className={cn("ask-option", chosen === i && "is-chosen")} disabled={!live || !!sending}
              onClick={() => void send(o.label)}>
              <span className="ask-option-n">{chosen === i ? <Check /> : i + 1}</span>
              <span className="ask-option-text">
                <span className="ask-option-label">{o.label}</span>
                {o.detail && <span className="ask-option-detail">{o.detail}</span>}
              </span>
            </button>
          ))}
        </div>
      )}
      {live && (
        <form className="ask-say" onSubmit={(e) => { e.preventDefault(); void send(text); }}>
          <input className="ask-say-input" value={text} onChange={(e) => setText(e.target.value)} disabled={!!sending}
            placeholder={options.length ? L("或者直接说…", "Or say it in words…") : L("写下你的回答…", "Write your answer…")} />
          <button type="submit" className="ask-say-send" disabled={!text.trim() || !!sending} aria-label={L("发送", "Send")} title={L("发送", "Send")}>
            <CornerDownLeft />
          </button>
        </form>
      )}
      {answer && (
        <div className="ask-answer">
          <Check />
          <span className="ask-answer-text">
            {chosen >= 0 ? L(`你选了：${options[chosen].label}`, `You chose: ${options[chosen].label}`) : L(`你回答：${answer.body.trim()}`, `You answered: ${answer.body.trim()}`)}
          </span>
          <time className="ask-time" dateTime={answer.created_at}>{clock(answer.created_at)}</time>
        </div>
      )}
      {error && <div className="ask-error">{error}</div>}
    </div>
  );
}
