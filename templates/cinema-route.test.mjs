// Deep links select exact existing labels and contain no stream addresses.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import ts from 'typescript';
import {inlineI18n} from './test-i18n.mjs';
const url=s=>'data:text/javascript;base64,'+Buffer.from(s).toString('base64');
const compile=f=>ts.transpileModule(fs.readFileSync(new URL(f,import.meta.url),'utf8'),{compilerOptions:{module:ts.ModuleKind.ES2022,target:ts.ScriptTarget.ES2022}}).outputText;
const helper=url(compile('src/_lib/page-route.ts'));
const m=await import(url(inlineI18n(compile('src/cinema/route.ts')).replace('"../_lib/page-route"',JSON.stringify(helper))));
const h=await import(helper),r={source:'my_source',movie:'id/42',line:'高清 & 原声',episode:'第02集（下）'},fragment=m.watchFragment(r);
assert.deepEqual(m.readWatchRoute(fragment),r);assert.equal(m.readWatchRoute(''),null);assert.ok(!fragment.includes('http'));assert.equal(new URL(h.publicAppLink('https://hub.test/team/pages/pg_abc',fragment)).hash,fragment);
for(const f of ['#watch','#other?x=1','#watch?source=s&movie=1&episode=1','#watch?source=s&movie=1&source=t','#watch?source=s&movie=1&url=https://evil.test','#watch?source=s&movie=%00','#watch?source=s&movie=1&line=','#watch?source=s&movie=1&episode=%EF%BF%BD&line=a','#watch?source=s&movie=1&line='+ 'a'.repeat(201)])assert.throws(()=>m.readWatchRoute(f),m.WatchRouteError);
for(const u of ['https://x.test/not-a-page','javascript:alert(1)','https://user:password@x.test/pages/pg_a','https://x.test/pages/pg_a?token=secret'])assert.throws(()=>h.publicAppLink(u,fragment));
const movie={lines:[{name:'HD',episodes:[{name:'第01集',url:'https://media.test/1'},{name:'第02集',url:'https://media.test/2'}]},{name:r.line,episodes:[{name:r.episode,url:'https://media.test/3'}]}]};
assert.deepEqual(m.watchSelection(movie,r),{line:1,episode:0});assert.deepEqual(m.watchSelection(movie,{source:'s',movie:'42'}),{line:0,episode:0});
assert.throws(()=>m.watchSelection(movie,{...r,line:'Removed'}),m.WatchRouteError);assert.throws(()=>m.watchSelection(movie,{...r,line:'HD',episode:'第2集'}),m.WatchRouteError,'Never silently replace a missing label');
console.log('cinema routes: round-trip Unicode, exact line/episode selection, strict malformed inputs, safe public URLs and no fallback to a different episode passed');
// Only the parent host may request a repeated route. Cleanup removes listeners.
const listeners=new Map(),parent={};globalThis.window={parent,addEventListener:(k,f)=>listeners.set(k,f),removeEventListener:(k,f)=>{if(listeners.get(k)===f)listeners.delete(k)}};globalThis.location={hash:fragment};let visits=0;const stop=h.subscribeAppRoute(()=>visits++);
listeners.get('message')({source:{},data:{myco:'page-route',fragment:'#watch?source=foreign&movie=x'}});assert.equal(location.hash,fragment);assert.equal(visits,0);
listeners.get('message')({source:parent,data:{myco:'page-route',fragment}});assert.equal(visits,1);
listeners.get('message')({source:parent,data:{myco:'page-route',fragment:'#'+ 'x'.repeat(1025)}});assert.equal(location.hash,fragment);
listeners.get('message')({source:parent,data:{myco:'page-route',fragment:'#watch?source=s&movie=2'}});assert.equal(location.hash,'#watch?source=s&movie=2');listeners.get('hashchange')();assert.equal(visits,2);stop();assert.equal(listeners.size,0);
console.log('route subscription: foreign-frame messages rejected, same-route revisit delivered, oversized input ignored and listener cleanup passed');
