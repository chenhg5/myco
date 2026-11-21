// Source model tests run without a browser or bundler. Inline only the tiny
// display helper; production templates still use ordinary ES module imports.
import fs from "node:fs";
import ts from "typescript";
const source = fs
  .readFileSync(new URL("src/_lib/i18n.ts", import.meta.url), "utf8")
  .replace(
    /import english from [^;]+;/,
    `const english=${fs.readFileSync(new URL("localization/en.json", import.meta.url), "utf8")};`,
  );
export const i18nURL =
  "data:text/javascript;base64," +
  Buffer.from(
    ts.transpileModule(source, {
      compilerOptions: {
        module: ts.ModuleKind.ES2022,
        target: ts.ScriptTarget.ES2022,
      },
    }).outputText,
  ).toString("base64");
export const inlineI18n = (js) =>
  js.replace(/(["'])\.\.\/_lib\/i18n\1/g, JSON.stringify(i18nURL));
