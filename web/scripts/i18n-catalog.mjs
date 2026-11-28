#!/usr/bin/env node
// Deterministic, offline extraction and placeholder checks. No translation service or credentials.
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");
const src = path.join(root, "web/src");
const english = new Map(), chinese = new Map();
const unresolved = [];
function add(map, text, file) {
  if (!text || /^https?:\/\//.test(text)) return;
  const files = map.get(text) || new Set(); files.add(path.relative(root, file)); map.set(text, files);
}
function texts(node) {
  if (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)) return [node.text];
  if (ts.isTaggedTemplateExpression(node) && node.tag.getText() === "E") {
    const t = node.template;
    if (ts.isNoSubstitutionTemplateLiteral(t)) return [t.text];
    const escaped = (text) => text.replace(/\{/g,"{{").replace(/\}/g,"}}");
    return [escaped(t.head.text) + t.templateSpans.map((s, i) => `{${i}}${escaped(s.literal.text)}`).join("")];
  }
  if (ts.isConditionalExpression(node)) return [...texts(node.whenTrue), ...texts(node.whenFalse)];
  if (ts.isParenthesizedExpression(node)) return texts(node.expression);
  return [];
}
function files(dir) { return fs.readdirSync(dir, { withFileTypes: true }).flatMap((e) => e.isDirectory() ? files(path.join(dir,e.name)) : /\.[tj]sx?$/.test(e.name) ? [path.join(dir,e.name)] : []); }
for (const file of files(src)) {
  const source = fs.readFileSync(file,"utf8");
  const tree = ts.createSourceFile(file,source,ts.ScriptTarget.Latest,true);
  const insert = [];
  function tag(node) {
    if (ts.isTemplateExpression(node)) insert.push(node.getStart(tree));
    else if (ts.isConditionalExpression(node)) { tag(node.whenTrue); tag(node.whenFalse); }
    else if (ts.isParenthesizedExpression(node)) tag(node.expression);
  }
  function visit(n) {
    if (ts.isCallExpression(n) && ts.isIdentifier(n.expression) && n.expression.text === "L" && n.arguments.length >= 2) {
      if (process.argv.includes("--tag")) { tag(n.arguments[0]); tag(n.arguments[1]); }
      for (const text of texts(n.arguments[0])) add(chinese,text,file);
      for (const text of texts(n.arguments[1])) add(english,text,file);
      if (!texts(n.arguments[1]).length) unresolved.push({file:path.relative(root,file),line:tree.getLineAndCharacterOfPosition(n.getStart(tree)).line+1,expression:n.arguments[1].getText(tree)});
    }
    // Bilingual helper arguments and status tables are resolved by L at render time.
    const pairs = ts.isArrayLiteralExpression(n) ? n.elements : ts.isCallExpression(n) ? n.arguments : [];
    for (let i = 0; i + 1 < pairs.length; i++) {
      if (ts.isStringLiteral(pairs[i]) && ts.isStringLiteral(pairs[i+1]) && /[\u3400-\u9fff]/.test(pairs[i].text)) {
        add(chinese,pairs[i].text,file); add(english,pairs[i+1].text,file);
      }
    }
    if(ts.isObjectLiteralExpression(n)) {
      const fields = new Map(n.properties.filter(ts.isPropertyAssignment).map((p)=>[p.name.getText(tree).replace(/["']/g,""),p.initializer]));
      for(const [zh,en] of [["zh","en"],["name","name_en"],["help","help_en"]]) {
        const a=fields.get(zh),b=fields.get(en);
        if(a&&b&&ts.isStringLiteral(a)&&ts.isStringLiteral(b)&&/[\u3400-\u9fff]/.test(a.text)){add(chinese,a.text,file);add(english,b.text,file);}
      }
    }
    ts.forEachChild(n,visit);
  }
  visit(tree);
  if (insert.length) {
    let changed = source;
    for (const at of [...new Set(insert)].sort((a,b)=>b-a)) changed=changed.slice(0,at)+"E"+changed.slice(at);
    changed=changed.replace(/import\s*\{([^}]+)\}\s*from\s*(["'])@\/lib\/i18n\2/, (all, names, quote) => /\bE\b/.test(names) ? all : `import {${names.trimEnd()}, E } from ${quote}@/lib/i18n${quote}`);
    fs.writeFileSync(file,changed);
  }
}
// The desktop's native menus use the same catalogs and English fallback.
for (const name of ["cmd/myco-app/appmenu_darwin.go","cmd/myco-app/traystatus.go","cmd/myco-app/traymenu.go","internal/console/notices/notices.go"]) {
  const file=path.join(root,name), text=fs.readFileSync(file,"utf8");
  for (const m of text.matchAll(/(?:\.s|choose)\(\s*("(?:\\.|[^"\\])*")\s*,\s*("(?:\\.|[^"\\])*")\s*\)/g)) {
    add(chinese,JSON.parse(m[1]),file); add(english,JSON.parse(m[2]),file);
  }
}
const noticesFile=path.join(root,"internal/console/notices/notices.go");
for(const m of fs.readFileSync(noticesFile,"utf8").matchAll(/uilocale\.Text\(language,\s*("(?:\\.|[^"\\])*")\s*,\s*("(?:\\.|[^"\\])*")/g)) {add(chinese,JSON.parse(m[1]),noticesFile);add(english,JSON.parse(m[2]),noticesFile);}
for (const entry of Object.values(JSON.parse(fs.readFileSync(path.join(src,"lib/app-locales.json"),"utf8")))) {
  for (const key of ["name","description"]) {
    if(entry.en?.[key]) add(english,entry.en[key],path.join(src,"lib/app-locales.json"));
    if(entry.zh?.[key]) add(chinese,entry.zh[key],path.join(src,"lib/app-locales.json"));
  }
}
for (const file of fs.readdirSync(path.join(src,"app/help/zh"))) if(file.endsWith(".md")) add(chinese,fs.readFileSync(path.join(src,"app/help/zh",file),"utf8"),path.join(src,"app/help/zh",file));
const manifest={english:[...english].sort(([a],[b])=>a.localeCompare(b,"en")).map(([text,source])=>({text,files:[...source]})),chinese:[...chinese].sort(([a],[b])=>a.localeCompare(b,"en")).map(([text,source])=>({text,files:[...source]})),unresolved};
if(process.argv.includes("--manifest")) process.stdout.write(JSON.stringify(manifest,null,2)+"\n");
if(process.argv.includes("--check")) {
  const signature=s=>(s.match(/\{\{|\}\}|\{\d+\}|%(?:\[\d+\])?[+#0 .\-\d]*[sdvfx]/g)||[]).filter(x=>x!=="{{"&&x!=="}}").sort().join("|");
  const errors=[];
  for(const language of ["zh-Hant","ja","ko","es","pt-BR"]) {
    const catalog=JSON.parse(fs.readFileSync(path.join(root,"internal/uilocale/catalogs",language+".json"),"utf8"));
    for(const {text} of language === "zh-Hant" ? manifest.chinese : manifest.english) {
      if(typeof catalog[text]!=="string" || !catalog[text].trim()) errors.push(`${language}: missing ${JSON.stringify(text)}`);
      else if(signature(text)!==signature(catalog[text])) errors.push(`${language}: placeholders differ in ${JSON.stringify(text)}`);
    }
  }
  if(errors.length) { console.error(errors.slice(0,40).join("\n")); console.error(`${errors.length} catalog errors`); process.exitCode=1; }
  else console.log(`Catalogs cover ${english.size} English and ${chinese.size} Chinese messages; placeholders match.`);
}
