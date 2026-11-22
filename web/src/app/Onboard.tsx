import { useEffect, useState } from "react";
import { Check, ChevronDown, X } from "lucide-react";
import { L } from "@/lib/i18n";
import { cn } from "@/lib/utils";

/*
 * After the first start (Welcome → Prepare), two quiet helps for someone new
 * to myco, both only on a computer that started that way (or asked again in
 * Settings › About), both easy to put away for good:
 *  - the 上手清单 at the foot of the sidebar: five things worth doing once,
 *    each ticked by itself when done, a click on one goes there;
 *  - an intro card at the top of 项目, 自动化, 应用 and 电脑 the first time:
 *    what the page is for, in two lines, and its most common action.
 * Kept on this computer (localStorage).
 */

const NEW = "myco-onboard";
const LIST_OFF = "myco-checklist-off";
const LIST_OPEN = "myco-checklist-open";
const SEEN = "myco-intro-seen";
const DID = "myco-onboard-did";
const EVENT = "myco-onboard-changed";

const get = (k: string) => { try { return localStorage.getItem(k); } catch { return null; } };
const set = (k: string, v: string | null) => {
  try { if (v === null) localStorage.removeItem(k); else localStorage.setItem(k, v); } catch { /* not kept */ }
  window.dispatchEvent(new CustomEvent(EVENT));
};

/** A new install (or Settings › About asking again): the helps come on. */
export function startOnboarding() {
  set(LIST_OFF, null);
  set(SEEN, null);
  set(DID, null);
  set(NEW, "1");
}

/** A step done by going somewhere (Settings › 工作区 for 认识你的工作区). */
export function noteDone(id: string) {
  const did = (get(DID) || "").split(",").filter(Boolean);
  if (!did.includes(id)) set(DID, [...did, id].join(","));
}

function useTick() {
  const [, setN] = useState(0);
  useEffect(() => {
    const on = () => setN((n) => n + 1);
    window.addEventListener(EVENT, on);
    window.addEventListener("storage", on);
    return () => { window.removeEventListener(EVENT, on); window.removeEventListener("storage", on); };
  }, []);
}

export type ChecklistStep = { key: string; title: string; hint: string; done: boolean; go?: () => void;
  /** done too once this intro card was read (or this was noted done: noteDone) */
  seen?: IntroId | "guide" };

/** The 上手清单: a card at the foot of the sidebar, folded to a line with
 *  its progress, open to the steps. Gone once put away. */
export function Checklist({ steps }: { steps: ChecklistStep[] }) {
  useTick();
  const [open, setOpen] = useState(() => get(LIST_OPEN) !== "0");
  if (get(NEW) !== "1" || get(LIST_OFF) === "1" || !steps.length) return null;
  const read = [...(get(SEEN) || "").split(","), ...(get(DID) || "").split(",")];
  steps = steps.map((s) => (s.seen && read.includes(s.seen) ? { ...s, done: true } : s));
  const done = steps.filter((s) => s.done).length;
  const all = done === steps.length;
  const toggle = () => { setOpen((o) => { set(LIST_OPEN, o ? "0" : "1"); return !o; }); };
  const close = () => set(LIST_OFF, "1");
  return (
    <section className={cn("ob-list", open && "is-open", all && "is-done")} data-testid="checklist" aria-label={L("上手清单", "Getting started")}>
      <div className="ob-list-head">
        <button type="button" className="ob-list-toggle" onClick={toggle} aria-expanded={open}>
          <span className="ob-list-title">{all ? L("都做完了", "All done") : L("上手清单", "Getting started")}</span>
          <span className="ob-list-count">{done}/{steps.length}</span>
          <ChevronDown className="ob-list-chev" />
        </button>
        <button type="button" className="ob-list-close" onClick={close} aria-label={L("不再显示上手清单", "Hide the checklist for good")} title={L("不再显示", "Don't show again")}><X /></button>
      </div>
      <div className="ob-bar" aria-hidden><i style={{ width: `${(done / steps.length) * 100}%` }} /></div>
      {open && (
        <ol className="ob-steps">
          {steps.map((s) => (
            <li key={s.key}>
              <button type="button" className={cn("ob-step", s.done && "is-done")} onClick={s.go} disabled={!s.go} data-step={s.key} title={s.hint}>
                <span className="ob-tick" aria-hidden>{s.done && <Check />}</span>
                <span className="ob-step-text"><b>{s.title}</b>{!s.done && <small>{s.hint}</small>}</span>
              </button>
            </li>
          ))}
        </ol>
      )}
      {open && all && <button type="button" className="ob-list-bye" onClick={close}>{L("收起清单", "Put it away")}</button>}
    </section>
  );
}

/** The first time on a page: what it is for, and its usual action. */
export function IntroCard({ id, title, text, action }: {
  id: "projects" | "automations" | "apps" | "computers" | "workspaces";
  title: string;
  text: string;
  action?: { label: string; onClick: () => void };
}) {
  useTick();
  if (get(NEW) !== "1") return null;
  const seen = (get(SEEN) || "").split(",").filter(Boolean);
  if (seen.includes(id)) return null;
  const dismiss = () => set(SEEN, [...seen, id].join(","));
  return (
    <aside className="ob-intro" data-testid={`intro-${id}`}>
      <div className="ob-intro-text">
        <b>{title}</b>
        <p>{text}</p>
      </div>
      <div className="ob-intro-actions">
        {action && <button type="button" className="app-btn app-btn-small" onClick={() => { dismiss(); action.onClick(); }}>{action.label}</button>}
        <button type="button" className="app-btn app-btn-quiet app-btn-small" onClick={dismiss}>{L("知道了", "Got it")}</button>
      </div>
    </aside>
  );
}

/** Words for the cards, in one place. */
export const INTROS = {
  projects: () => ({
    title: L("项目：几个 agent 一起做成一件事", "Projects: a few agents getting one thing done"),
    text: L("在这里对协调者说要做什么，它会拆成几步、分给成员；右边能看到每个成员在做什么，点一个就能单独和它聊。",
      "Tell the coordinator here what you want done: it breaks it down and hands it to the members. On the right is what each is doing; click one to talk to it alone."),
  }),
  automations: () => ({
    title: L("自动化：让 agent 到点自己干活", "Automations: agents at work on their own"),
    text: L("比如工作日早上汇总新 issue，或者 CI 挂了去看日志。它跑在工作区上，你的电脑关着也照常；回复在你和它的对话里。",
      "Say, a summary of new issues every weekday morning, or a look at the log when CI fails. It runs on the workspace, so it goes on while your computer is off; answers come in your chat with the agent."),
  }),
  apps: () => ({
    title: L("应用：你和 agent 都能用的小工具", "Apps: small tools for you and your agents"),
    text: L("表格、多维表格、文档、幻灯片……建一个，agent 也能读写它；agent 做的页面也会出现在这里。",
      "Sheets, bases, docs, slides… Make one and your agents can read and write it too; pages your agents make show up here as well."),
  }),
  workspaces: () => ({
    title: L("工作区：你的 agent 碰头的地方", "Workspaces: where your agents meet"),
    text: L("连到同一个工作区的 agent，不管在这台电脑、服务器还是同事的电脑上，都能互相派活、一起做项目，你在一个窗口里看着。现在它跑在这台电脑上；想让外面的电脑或同事加入，就开放到公网再发邀请。也可以再建一个工作区（比如工作和个人分开），或者加入别人的。",
      "Agents connected to the same workspace, on this computer, a server or a colleague's computer, can hand each other work and share projects while you watch in one window. It runs on this computer now; to let computers or people outside join, open it to the internet and invite them. You can also start another one (work and personal apart, say) or join someone else's."),
  }),
  computers: () => ({
    title: L("电脑：agent 在哪里干活", "Computers: where your agents work"),
    text: L("每台连上的电脑都能加 agent、开终端。想让 agent 在服务器或另一台电脑上干活，就把它连进来。",
      "On every computer connected you can add agents and open terminals. To have agents work on a server or another computer, connect it here."),
  }),
} satisfies Record<string, () => { title: string; text: string }>;

export function introOf(id: keyof typeof INTROS): { title: string; text: string } { return INTROS[id](); }
export type IntroId = keyof typeof INTROS;
