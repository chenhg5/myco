import { t as uiText, locale, language } from "./i18n";
// Record dates remain ISO dates in the viewer's own calendar day.
export const ymd = (d: Date) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
export const today = () => ymd(new Date());
export const addDays = (s: string, n: number) => { const d = new Date(`${s}T12:00:00`); d.setDate(d.getDate() + n); return ymd(d); };
export const WEEK = Array.from({length:7},(_,i)=>new Intl.DateTimeFormat(locale(),{weekday:language()==="en"?"short":"narrow"}).format(new Date(2026,10,15+i)));
export function dayName(s: string) {
 const now=today();
 if(s===now)return uiText("今天");if(s===addDays(now,1))return uiText("明天");if(s===addDays(now,-1))return uiText("昨天");
 const d=new Date(`${s}T12:00:00`),diff=Math.round((d.getTime()-new Date(`${now}T12:00:00`).getTime())/86400000);
 if(diff>1&&diff<7)return new Intl.DateTimeFormat(locale(),{weekday:"short"}).format(d);
 return new Intl.DateTimeFormat(locale(),{...(d.getFullYear()===new Date().getFullYear()?{}:{year:"numeric"}),month:"short",day:"numeric"}).format(d);
}
