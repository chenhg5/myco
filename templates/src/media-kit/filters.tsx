import { useState } from "react";
import { t as uiText } from "../_lib/i18n";
import { aspects, kit, mediaAspect, platforms, type Picture } from "./model";

const languages = ["", "zh", "en"] as const;
const themes = ["", "light", "dark"] as const;
type Language = (typeof languages)[number];
type Theme = (typeof themes)[number];

function useAssetFilter<T extends string>(name: string, values: readonly T[]) {
  const key = JSON.stringify(["media-kit", kit.page, kit.me, name]);
  const [value, setValue] = useState<T>(() => {
    try {
      const saved = localStorage.getItem(key);
      if (values.includes(saved as T)) return saved as T;
    } catch { /* The filter still works in memory. */ }
    return values[0];
  });
  return [value, (next: T) => {
    setValue(next);
    try { localStorage.setItem(key, next); } catch { /* Storage may be unavailable. */ }
  }] as const;
}

export function useLanguageFilter(name: string) { return useAssetFilter(name, languages); }
export const platformName = (platform: (typeof platforms)[number]) => ({
  app: "App", mobile: uiText("移动端"), cli: "CLI", web: uiText("网页版"),
}[platform]);
export const aspectName = (aspect: (typeof aspects)[number]) => ({
  landscape: uiText("横图"), portrait: uiText("竖图"), square: uiText("方图"),
}[aspect]);

export function useAssetFilters(scope: "detail" | "cover") {
  const [language, setLanguage] = useLanguageFilter(scope + "-language");
  const [theme, setTheme] = useAssetFilter(scope + "-theme", themes);
  const [platform, setPlatform] = useAssetFilter(scope + "-platform", ["", ...platforms] as const);
  const [aspect, setAspect] = useAssetFilter(scope + "-aspect", ["", ...aspects] as const);
  function matches(data: Picture) {
    return (!theme || data.theme === theme) && (!platform || data.platform === platform) &&
      (!aspect || (data.aspect ?? mediaAspect(data.width, data.height)) === aspect);
  }
  return { language, setLanguage, theme, setTheme, platform, setPlatform, aspect, setAspect, matches };
}

export function AssetFilters({ filters, reset }: { filters: ReturnType<typeof useAssetFilters>; reset: () => void }) {
  return <>
    <LanguageFilter value={filters.language} change={value => { filters.setLanguage(value); reset(); }}/>
    <ThemeFilter value={filters.theme} change={value => { filters.setTheme(value); reset(); }}/>
    <label className="mk-filter">{uiText("素材平台")}<select className="sel" aria-label={uiText("素材平台")} value={filters.platform}
      onChange={event => { filters.setPlatform(event.target.value as typeof filters.platform); reset(); }}>
      <option value="">{uiText("全部")}</option>
      {platforms.map(value => <option key={value} value={value}>{platformName(value)}</option>)}
    </select></label>
    <label className="mk-filter">{uiText("画面方向")}<select className="sel" aria-label={uiText("画面方向")} value={filters.aspect}
      onChange={event => { filters.setAspect(event.target.value as typeof filters.aspect); reset(); }}>
      <option value="">{uiText("全部")}</option>
      {aspects.map(value => <option key={value} value={value}>{aspectName(value)}</option>)}
    </select></label>
  </>;
}

export function LanguageFilter({ value, change }: { value: Language; change: (value: Language) => void }) {
  return <label className="mk-filter">{uiText("语言")}<select className="sel" aria-label={uiText("语言")} value={value} onChange={event => change(event.target.value as Language)}>
    <option value="">{uiText("全部")}</option><option value="zh">中文</option><option value="en">English</option>
  </select></label>;
}

export function ThemeFilter({ value, change }: { value: Theme; change: (value: Theme) => void }) {
  return <label className="mk-filter">{uiText("显示主题")}<select className="sel" aria-label={uiText("显示主题")} value={value} onChange={event => change(event.target.value as Theme)}>
    <option value="">{uiText("全部")}</option><option value="light">{uiText("浅色")}</option><option value="dark">{uiText("深色")}</option>
  </select></label>;
}

export function ImageBadge({ picture }: { picture: Picture }) {
  const label = picture.language === "zh" ? "中" : picture.language === "en" ? "EN" : uiText("无字");
  const theme = picture.theme === "light" ? uiText("浅") : picture.theme === "dark" ? uiText("深") : "";
  return <span className="mk-image-badge">{[label, theme].filter(Boolean).join(" · ")}</span>;
}
