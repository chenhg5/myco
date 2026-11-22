// Load the source SDK without shipping or relying on generated template assets.
import fs from 'node:fs';
import path from 'node:path';
import ts from 'typescript';

const compiled = new Map();
export async function actionModule(name) {
  if (compiled.has(name)) return compiled.get(name);
  let code = ts.transpileModule(fs.readFileSync(new URL(`./src/${name}.ts`, import.meta.url), 'utf8'), {
    compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
  }).outputText;
  for (const match of [...code.matchAll(/from "(\.[\w./-]+)"/g)]) code = code.replace(match[0], `from ${JSON.stringify(await actionModule(path.posix.join(path.posix.dirname(name), match[1])))}`);
  const url = `data:text/javascript;base64,${Buffer.from(code).toString('base64')}`;
  compiled.set(name, url);
  return url;
}
