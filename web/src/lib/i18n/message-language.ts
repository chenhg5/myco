import { isChinese, type Lang } from "./locales";

/** Detect clear script differences; never guess between Latin languages. */
export function inOtherLanguage(text: string, to: Lang): boolean {
  const prose = text.replace(/```[\s\S]*?(```|$)/g, " ").replace(/`[^`\n]*`/g, " ")
    .replace(/https?:\/\/\S+/g, " ").replace(/[\w.-]*[/\\][\w./\\-]+/g, " ");
  const han = (prose.match(/[㐀-鿿豈-﫿]/g) || []).length;
  const kana = (prose.match(/[ぁ-ヿ]/g) || []).length;
  const hangul = (prose.match(/[가-힣]/g) || []).length;
  const words = (prose.match(/[A-Za-zÀ-ž]{2,}/g) || []).length;
  if (isChinese(to)) return kana >= 3 || hangul >= 3 || (words >= 4 && han < words * .5);
  if (to === "ja") return hangul >= 3 || (words >= 4 && han + kana < words * .5);
  if (to === "ko") return kana >= 3 || (han >= 6 && !hangul) || (words >= 4 && hangul < words * .5);
  return han + kana + hangul >= 6 && han + kana + hangul > words * 2;
}
