import { useEffect, useRef } from "react";
import type { BrowserProfile, BrowserProfileSettingsInput } from "@/appapi";
import { L } from "@/lib/i18n";

type SettingsReview = {
  name: string; proxy: string; bypass: string[]; username: boolean;
  password: "keep" | "set" | "clear" | "none";
  userAgent: string; lang: string; args: string[]; running: boolean;
};

// This object contains no password and is the only settings data put in the
// trusted modal's React state. The write-only request stays in its call closure.
export function browserSettingsReview(profile: BrowserProfile, settings: BrowserProfileSettingsInput): SettingsReview {
  if (!settings || typeof settings !== "object" || typeof settings.user_agent !== "string" || typeof settings.lang !== "string" || !Array.isArray(settings.args) || settings.args.some(v => typeof v !== "string")) throw Object.assign(new Error("invalid_argument"), {code:"invalid_argument"});
  if (settings.proxy) {
    const u = new URL(settings.proxy.server);
    if (!["http:", "https:", "socks5:"].includes(u.protocol) || u.username || u.password || !Array.isArray(settings.proxy.bypass)) throw Object.assign(new Error("invalid_argument"), {code:"invalid_argument"});
  }
  const proxy = settings.proxy;
  return {
    name: profile.name, proxy: proxy?.server || "", bypass: proxy?.bypass || [], username: !!proxy?.username,
    password: !proxy ? profile.settings?.proxy?.has_password ? "clear" : "none" : proxy.password === "" ? "clear" : proxy.password !== undefined ? "set" : profile.settings?.proxy?.has_password ? profile.settings.proxy.server === proxy.server && profile.settings.proxy.username === proxy.username ? "keep" : "clear" : "none",
    userAgent: settings.user_agent, lang: settings.lang, args: settings.args, running: profile.running,
  };
}
export type BrowserSettingsReview = SettingsReview;
export function BrowserProfileSettingsDetails({review, onRead}: {review: SettingsReview; onRead: (review: SettingsReview) => void}) {
  const content = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const el = content.current;
    if (!el) return;
    el.scrollTop = 0;
    const check = () => {
      if (el.clientHeight > 0 && el.scrollTop + el.clientHeight >= el.scrollHeight - 2) onRead(review);
    };
    const observer = new ResizeObserver(check);
    observer.observe(el);
    el.addEventListener("scroll", check);
    check();
    return () => { observer.disconnect(); el.removeEventListener("scroll", check); };
  }, [review, onRead]);
  const password = review.password === "set" ? L("设置新密码（不显示）", "Set a new password (hidden)") : review.password === "keep" ? L("保留已保存的密码", "Keep the saved password") : review.password === "clear" ? L("清除已保存的密码", "Clear the saved password") : L("无", "None");
  return <div ref={content} tabIndex={0} aria-label={L("浏览器设置清单", "Browser settings review")} className="pg-browser-approval pg-browser-settings" data-testid="browser-settings-review">
    <div className="pg-browser-approval-title">{review.name}</div>
    <p>{L("代理服务器可以看到这个资料经过它的网络流量。设置在下次启动浏览器时生效，不会重启当前浏览器。", "The proxy server can see this profile’s traffic passing through it. Settings apply the next time the browser starts; the current browser will stay open.")}</p>
    <dl>
      <dt>{L("代理服务器", "Proxy server")}</dt><dd className="pg-browser-settings-mono">{review.proxy || L("不使用代理", "No proxy")}</dd>
      <dt>{L("不走代理的地址", "Proxy bypass")}</dt><dd>{review.bypass.join("; ") || L("无", "None")}</dd>
      <dt>{L("代理账号", "Proxy username")}</dt><dd>{review.username ? L("已填写", "Provided") : L("无", "None")}</dd>
      <dt>{L("代理密码", "Proxy password")}</dt><dd>{password}</dd>
      <dt>User-Agent</dt><dd>{review.userAgent || L("Chrome 自带", "Chrome default")}</dd>
      <dt>{L("语言", "Language")}</dt><dd>{review.lang || L("系统默认", "System default")}</dd>
      <dt>{L("启动参数", "Launch arguments")}</dt><dd className="pg-browser-settings-mono">{review.args.length ? <pre>{review.args.join("\n")}</pre> : L("无", "None")}</dd>
    </dl>
    {review.userAgent && <p>{L("自定义 UA 可能与浏览器版本不一致，增加被网站识别的风险。", "A custom user agent may disagree with the browser version and make detection more likely.")}</p>}
  </div>;
}
