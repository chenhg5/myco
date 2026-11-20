// 会议记录's data: each meeting (when, how long, where its recording is on
// the computer that made it, who the tracks are) and its transcript, one
// record per sentence. The recording stays on that computer; the text is
// the page's, so people and agents read and fix it together.
import { Store } from "../_lib/myco";
import { Kit } from "../_lib/appkit";

export type Track = "me" | "them";
export type Status = "recording" | "recorded" | "transcribing" | "done" | "failed";

export type Meeting = {
  title?: string; started_at?: string; seconds?: number; status?: Status;
  /** The computer it was recorded on (its recording is there). */
  computer?: string; dir?: string;
  files?: Partial<Record<Track, string>>;
  /** What each track recorded: mic, system, app:<id>. */
  sources?: Partial<Record<Track, string>>;
  source_name?: string;
  speakers?: Partial<Record<Track, string>>;
  model?: string; by?: string; error?: string;
};
export type Segment = { meeting?: string; track?: Track; start?: number; end?: number; text?: string; edited?: boolean };

export const meetings = new Store<Meeting & Record<string, unknown>>("meetings");
export const segments = new Store<Segment & Record<string, unknown>>("segments");
export const kit = new Kit([meetings, segments], { quiet: true });

export const num = (v: unknown, d = 0) => (typeof v === "number" && Number.isFinite(v) ? v : d);

/** Meetings, newest first. */
export const meetingList = () => [...meetings.rows].sort((a, b) => String(b.data.started_at || "").localeCompare(String(a.data.started_at || "")));

/** A meeting's sentences, in time. */
export function segmentsOf(id: string) {
  return segments.rows.filter((r) => r.data.meeting === id).sort((a, b) => num(a.data.start) - num(b.data.start));
}

export function speakerName(m: Meeting, t: Track | undefined) {
  const s = m.speakers?.[t || "them"];
  if (s) return s;
  return t === "me" ? "我" : "对方";
}

export const TRACK_COLOR: Record<Track, string> = { me: "#3370ff", them: "#f5813f" };

/** 12:05 or 1:02:05. */
export function clock(sec: number) {
  sec = Math.max(0, Math.floor(sec));
  const h = Math.floor(sec / 3600), m = Math.floor((sec % 3600) / 60), s = sec % 60;
  const p = (n: number) => String(n).padStart(2, "0");
  return h ? `${h}:${p(m)}:${p(s)}` : `${p(m)}:${p(s)}`;
}

/** 1 小时 3 分, 12 分 5 秒. */
export function length(sec: number) {
  sec = Math.round(sec);
  const h = Math.floor(sec / 3600), m = Math.floor((sec % 3600) / 60), s = sec % 60;
  if (h) return `${h} 小时 ${m} 分`;
  if (m) return `${m} 分 ${s} 秒`;
  return `${s} 秒`;
}

export function when(at?: string) {
  if (!at) return "";
  const d = new Date(at);
  if (Number.isNaN(d.getTime())) return "";
  const today = new Date();
  const same = d.toDateString() === today.toDateString();
  const hm = d.toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" });
  if (same) return `今天 ${hm}`;
  const y = new Date(today.getTime() - 86400000);
  if (d.toDateString() === y.toDateString()) return `昨天 ${hm}`;
  return `${d.getMonth() + 1}月${d.getDate()}日 ${hm}`;
}

/** Apps people meet in, by bundle id or name: these go first in the list. */
export const MEETING_APPS = [
  /zoom/i, /tencent\.meeting|腾讯会议|wemeet|voov/i, /feishu|lark/i, /teams/i, /dingtalk|钉钉/i, /webex/i, /slack/i, /discord/i,
  /wechat|微信/i, /chrome|safari|edge|firefox|arc|dia|browser/i,
];
export const meetingRank = (s: { name: string; bundle?: string; id: string }) => {
  const k = `${s.bundle || ""} ${s.name} ${s.id}`;
  const i = MEETING_APPS.findIndex((r) => r.test(k));
  return i < 0 ? 99 : i;
};

/** How alike two sentences are (0–1): the share of character pairs they have in common. */
export function alike(a: string, b: string) {
  const pairs = (s: string) => {
    const t = s.replace(/[\s，。,.!?！？、；;：:"'“”‘’]/g, "").toLowerCase();
    const out = new Map<string, number>();
    for (let i = 0; i + 1 < t.length; i++) out.set(t.slice(i, i + 2), (out.get(t.slice(i, i + 2)) || 0) + 1);
    return out;
  };
  const A = pairs(a), B = pairs(b);
  let both = 0, all = 0;
  for (const [k, n] of A) { both += Math.min(n, B.get(k) || 0); all += n; }
  for (const n of B.values()) all += n;
  return all ? (2 * both) / all : 0;
}
