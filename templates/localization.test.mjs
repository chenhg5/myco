import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import ts from "typescript";
import { i18nURL } from "./test-i18n.mjs";
const root = path.join(import.meta.dirname, "src"),
  dict = JSON.parse(
    fs.readFileSync(
      path.join(import.meta.dirname, "localization/en.json"),
      "utf8",
    ),
  );
let count = 0;
function scan(dir) {
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    const file = path.join(dir, e.name);
    if (e.isDirectory()) {
      if (!["public", "testdata"].includes(e.name)) scan(file);
      continue;
    }
    if (!/\.(ts|tsx)$/.test(file)) continue;
    const sf = ts.createSourceFile(
      file,
      fs.readFileSync(file, "utf8"),
      99,
      true,
    );
    function walk(n) {
      if (
        ts.isCallExpression(n) &&
        n.expression.getText(sf) === "uiText" &&
        ts.isStringLiteralLike(n.arguments[0])
      ) {
        const k = n.arguments[0].text;
        assert.equal(
          typeof dict[k],
          "string",
          `missing English UI string: ${k}`,
        );
        count++;
      }
      ts.forEachChild(n, walk);
    }
    walk(sf);
  }
}
scan(root);
for (const [k, v] of Object.entries(dict))
  assert.deepEqual(
    [...(v.match(/\{\d+\}/g) || [])].sort(),
    [...(k.match(/\{\d+\}/g) || [])].sort(),
    `placeholder mismatch: ${k}`,
  );
const m = await import(i18nURL);
globalThis.location = { search: "?myco_lang=en" };
assert.equal(m.t("删除"), "Delete");
assert.equal(
  m.t("添加「{0}」", "User title 用户标题"),
  "Add “User title 用户标题”",
);
assert.equal(m.locale(), "en-US");
globalThis.location.search = "?myco_lang=zh";
assert.equal(
  m.t("添加「{0}」", "User title 用户标题"),
  "添加「User title 用户标题」",
);
delete globalThis.location;
const fallback = JSON.parse(
  fs.readFileSync(
    path.resolve(import.meta.dirname, "../web/src/lib/app-locales.json"),
    "utf8",
  ),
);
for (const app of fs.readdirSync(root)) {
  const publicDir = path.join(root, app, "public"),
    file = path.join(publicDir, "myco.json");
  if (!fs.existsSync(file)) continue;
  const manifest = JSON.parse(fs.readFileSync(file, "utf8")),
    locales = JSON.parse(
      fs.readFileSync(path.join(publicDir, "locales.json"), "utf8"),
    );
  assert(
    locales.en.name && locales.en.description,
    app + " has English metadata",
  );
  assert.equal(locales.zh.name, manifest.name);
  assert.deepEqual(
    fallback[manifest.id],
    locales,
    "old-Hub catalog fallback stays current",
  );
}
console.log(
  `PASS ${count} UI calls, English coverage, interpolation, locale selection, untouched user text, all catalog sidecars and legacy metadata fallback`,
);
// Canvas/editor queries obey explicit App preferences even against the OS.
const events=new Set(),nativeEvents=new Set();let osDark=false;
globalThis.myco={features:['preferences'],preferences:{language:'en',theme:'dark'},on:(_name,f)=>{events.add(f);return()=>events.delete(f)}};
globalThis.window={matchMedia:()=>({get matches(){return osDark},addEventListener:(_n,f)=>nativeEvents.add(f),removeEventListener:(_n,f)=>nativeEvents.delete(f)})};
const appearanceSource=fs.readFileSync(path.join(root,'_lib/appearance.ts'),'utf8').replace(/import \{ myco \} from [^;]+;/,'const myco = globalThis.myco;');
const appearance=await import('data:text/javascript;base64,'+Buffer.from(ts.transpileModule(appearanceSource,{compilerOptions:{module:ts.ModuleKind.ES2022,target:ts.ScriptTarget.ES2022}}).outputText).toString('base64'));
const query=appearance.themeMedia();assert.equal(query.matches,true);let changes=0;const changed=()=>changes++;query.addEventListener('change',changed);
globalThis.myco.preferences.theme='light';for(const f of events)f();assert.equal(query.matches,false);assert.equal(changes,1);
osDark=true;for(const f of nativeEvents)f();assert.equal(query.matches,false);assert.equal(changes,1);
query.removeEventListener('change',changed);assert.equal(events.size,0);assert.equal(nativeEvents.size,0);globalThis.myco.features=[];assert.equal(query.matches,true);delete globalThis.myco;delete globalThis.window;
console.log('PASS explicit App theme, OS conflict, live changes, listener cleanup and old-App fallback');
