import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { transform } from "esbuild";
const asURL = (source) => `data:text/javascript;base64,${Buffer.from(source).toString("base64")}`;
const localeSource = await readFile(new URL("../src/lib/i18n/locales.ts", import.meta.url), "utf8");
const { code: localeCode } = await transform(localeSource, { loader: "ts", format: "esm" });
const localeURL = asURL(localeCode);
const { resolveLanguage, formattingLocale, formatMessage, E, LANGUAGES } = await import(localeURL);
assert.equal(LANGUAGES.length, 7);
for (const [tags, expected] of [
  [["en-GB", "zh-CN"], "en"], [["zh-TW", "en"], "zh-Hant"], [["zh-HK"], "zh-Hant"], [["zh-Hant-HK"], "zh-Hant"],
  [["zh-Hans-SG"], "zh"], [["fr", "ja-JP", "zh-CN"], "ja"], [["ko-KR"], "ko"], [["es-MX"], "es"], [["pt-PT"], "pt-BR"], [["fr"], "en"],
]) assert.equal(resolveLanguage("system", tags), expected, tags.join(","));
assert.equal(resolveLanguage("ja", ["zh-CN"]), "ja");
assert.equal(resolveLanguage("future-locale", ["ko-KR"]), "ko");
assert.equal(formattingLocale("system", ["en-GB"]), "en-GB");
assert.equal(formattingLocale("system", ["es-MX"]), "es-MX");
assert.equal(formattingLocale("zh-Hant", []), "zh-TW");
assert.equal(formatMessage("{0}: {1}", ["<script>{1}</script>", "$&"]), "<script>{1}</script>: $&");
assert.deepEqual(E`Send ${"demo"} ${2}`, { source: "Send {0} {1}", values: ["demo", 2] });

let serial = 0;
const raw = await readFile(new URL("../src/lib/i18n.ts", import.meta.url), "utf8");
async function instance(saved, tags = ["en-US"]) {
  const settings = new Map(saved ? [["myco-lang", saved]] : []);
  globalThis.__languagePreferences = settings;
  Object.defineProperty(globalThis, "navigator", { configurable: true, value: { languages: tags } });
  globalThis.document = { documentElement: { lang: "" } };
  globalThis.location = { search: "" };
  globalThis.window = new EventTarget();
  const prefsURL = asURL("export const localPreferences = {getItem:k=>globalThis.__languagePreferences.get(k)||null,setItem:(k,v)=>globalThis.__languagePreferences.set(k,v)}");
  let source = raw.replace('"@/lib/localPreferences"', JSON.stringify(prefsURL)).replaceAll('"./i18n/locales"', JSON.stringify(localeURL));
  source = source.replace(/new URL\("[^\"]+\/catalogs\/([^\"]+)\.json", import\.meta\.url\)\.href/g, (_, l) => JSON.stringify(`catalog:${l}`));
  const { code } = await transform(source, { loader: "ts", format: "esm" });
  const module = await import(asURL(code + `\n// ${serial++}`));
  return { module, settings };
}
let requests=[];
globalThis.fetch=async (url) => { requests.push(url); return Response.json({"Settings":"設定", "Send {0}":"{0} に送信", "設定":"設定"}); };
let { module: m, settings } = await instance("en");
await m.initializeLanguage();assert.equal(requests.length,0,"English must not load another language pack");
await m.setLangChoice("ja");assert.equal(settings.get("myco-lang"),"ja");assert.equal(m.L("设置","Settings"),"設定");assert.equal(m.L("中文", E`Send ${"<demo>"}`),"<demo> に送信");
assert.equal(m.L("未翻译","Not translated"),"Not translated");assert.equal(m.L("x","toString"),"toString","prototype members are never translations");
assert.equal(document.documentElement.lang,"ja");
// A failed selection neither overwrites the durable choice nor strands the current UI.
globalThis.fetch=async()=>new Response("{}",{status:503});
await assert.rejects(m.setLangChoice("ko"));assert.equal(m.lang(),"ja");assert.equal(settings.get("myco-lang"),"ja");
globalThis.fetch=async()=>Response.json({Settings:"설정"});await m.setLangChoice("ko");assert.equal(m.L("设置","Settings"),"설정");
// Later selection wins even if a previous catalog fetch resolves last.
let release;globalThis.fetch=(url)=>url==="catalog:es"?new Promise(r=>release=()=>r(Response.json({Settings:"Ajustes"}))):Promise.resolve(Response.json({Settings:"Configurações"}));
const older=m.setLangChoice("es");const newer=m.setLangChoice("pt-BR");await newer;release();assert.equal(await older,false);assert.equal(m.lang(),"pt-BR");assert.equal(settings.get("myco-lang"),"pt-BR");
// Start after an App/profile restart: selected language is ready before first render.
requests=[];globalThis.fetch=async(url)=>{requests.push(url);return Response.json({Settings:"Configurações"});};
({ module:m }=await instance("pt-BR"));await m.initializeLanguage();assert.equal(m.L("设置","Settings"),"Configurações");assert.deepEqual(requests,["catalog:pt-BR"]);
// Corrupt/missing assets fall back to readable English and preserve the saved choice.
globalThis.fetch=async()=>new Response("{}",{status:503});
({ module:m,settings }=await instance("ja"));await m.initializeLanguage();assert.equal(m.lang(),"en");assert.equal(m.languageProblem(),true);assert.equal(settings.get("myco-lang"),"ja");
// System mode follows host ordering, and reacts only while System is selected.
globalThis.fetch=async()=>Response.json({Settings:"設定"});
({ module:m,settings }=await instance("system",["en-US","zh-CN"]));await m.initializeLanguage();
navigator.languages=["ja-JP"];window.dispatchEvent(new Event("languagechange"));await new Promise(r=>setTimeout(r,0));assert.equal(m.lang(),"ja");assert.equal(settings.get("myco-lang"),"system");
console.log("Seven-language matching, lazy packs, placeholders, persistence and failure recovery pass.");

({ module:m,settings }=await instance("future-locale",["en-US"]));await m.initializeLanguage();navigator.languages=["ja-JP"];window.dispatchEvent(new Event("languagechange"));await new Promise(r=>setTimeout(r,0));assert.equal(settings.get("myco-lang"),"future-locale","automatic host changes never overwrite an unknown future preference");

assert.equal(formatMessage(E`Literal {0}, ${"value {1}"}`.source, ["value {1}"]), "Literal {0}, value {1}");
