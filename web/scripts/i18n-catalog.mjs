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
    return [t.head.text + t.templateSpans.map((s, i) => `{${i}}${s.literal.text}`).join("")];
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
    // A few existing status/access tables pass their pair to L at render time.
    if (ts.isArrayLiteralExpression(n) && n.elements.length === 2 && ts.isStringLiteral(n.elements[0]) && ts.isStringLiteral(n.elements[1]) && /[\u3400-\u9fff]/.test(n.elements[0].text)) {
      add(chinese,n.elements[0].text,file); add(english,n.elements[1].text,file);
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
for (const name of ["appmenu_darwin.go","traystatus.go","traymenu.go"]) {
  const file=path.join(root,"cmd/myco-app",name), text=fs.readFileSync(file,"utf8");
  for (const m of text.matchAll(/\.s\(\s*("(?:\\.|[^"\\])*")\s*,\s*("(?:\\.|[^"\\])*")\s*\)/g)) {
    add(chinese,JSON.parse(m[1]),file); add(english,JSON.parse(m[2]),file);
  }
}
for (const file of fs.readdirSync(path.join(src,"app/help/zh"))) if(file.endsWith(".md")) add(chinese,fs.readFileSync(path.join(src,"app/help/zh",file),"utf8"),path.join(src,"app/help/zh",file));
const manifest={english:[...english].sort(([a],[b])=>a.localeCompare(b,"en")).map(([text,source])=>({text,files:[...source]})),chinese:[...chinese].sort(([a],[b])=>a.localeCompare(b,"en")).map(([text,source])=>({text,files:[...source]})),unresolved};
if(process.argv.includes("--manifest")) process.stdout.write(JSON.stringify(manifest,null,2)+"\n");
if(process.argv.includes("--check")) {
  const signature=s=>(s.match(/\{\d+\}|%(?:\[\d+\])?[+#0 .\-\d]*[sdvfx]/g)||[]).sort().join("|");
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
