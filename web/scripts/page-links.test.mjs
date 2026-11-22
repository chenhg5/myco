// Public links preserve opaque app routes without changing the authorized page.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import ts from 'typescript';
const js=ts.transpileModule(fs.readFileSync(new URL('../src/lib/page-links.ts',import.meta.url),'utf8'),{compilerOptions:{module:ts.ModuleKind.ES2022,target:ts.ScriptTarget.ES2022}}).outputText;
const {parsePageLink:parse,pageFragmentURL:withFragment}=await import('data:text/javascript;base64,'+Buffer.from(js).toString('base64'));
const hubs=[{id:'one',url:'https://hub.test/team',aliases:['http://192.168.1.2:7780']}];
assert.deepEqual(parse('https://hub.test/team/pages/pg_abc',hubs),{hubId:'one',id:'pg_abc'});
const f='#watch?source=catalog&movie=42&line=HD&episode='+encodeURIComponent('第02集');
assert.deepEqual(parse('https://hub.test/team/pages/pg_abc'+f,hubs),{hubId:'one',id:'pg_abc',fragment:f});
assert.equal(parse('http://192.168.1.2:7780/pages/pg_abc'+f,hubs)?.fragment,f);
for(const href of ['https://evil.test/team/pages/pg_abc'+f,'https://hub.test.evil/team/pages/pg_abc','https://hub.test/team/pages/pg_abc/file','https://hub.test/team/pages/pg_abc?token=x'+f,'https://alice:secret@hub.test/team/pages/pg_abc','javascript:alert(1)','https://hub.test/team/pages/pg_abc#'+ 'x'.repeat(1025),'https://hub.test/team/pages/pg_abc#hello\nworld']) assert.equal(parse(href,hubs),null,href);
const localized='http://127.0.0.1:9000/page/pg_abc/index.html?key=private&myco_lang=en';
const actual=new URL(withFragment(localized,f));assert.equal(actual.search,'?key=private&myco_lang=en');assert.equal(actual.hash,f);assert.equal(new URL(withFragment(localized+'#old')).hash,'');
assert.ok(withFragment(localized,'#').endsWith('#'));
console.log('page links: canonical pages, workspace aliases, encoded labels, foreign workspaces, credentials, query rejection and bounded fragments passed');
