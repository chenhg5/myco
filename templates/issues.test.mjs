import assert from 'node:assert/strict';import fs from 'node:fs';import ts from 'typescript';
const source=fs.readFileSync(new URL('./src/issues/views.ts',import.meta.url),'utf8');const v=await import('data:text/javascript;base64,'+Buffer.from(ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.ES2022}}).outputText).toString('base64'));
const p={id:'v1',name:'My queue',view:{kind:'project',id:'p1'},query:'text',filters:{status:['todo','progress'],priority:[1,2],assignee:['agent'],labels:['ui']},layout:'board'};
assert.equal(v.readViews(JSON.stringify([p])).length,1);assert(v.sameView(p,{...p,filters:{...p.filters,status:['progress','todo']}}));assert(!v.sameView(p,{...p,query:'other'}));assert(!v.sameView(p,{...p,view:{kind:'project',id:'p2'}}));assert(!v.sameView(p,{...p,layout:'list'}));
for(const raw of ['bad','null','{}',JSON.stringify([{...p,filters:null}]),JSON.stringify([{...p,layout:'script'}]),JSON.stringify([{...p,name:''}])])assert.deepEqual(v.readViews(raw),[]);
assert.equal(v.readViews(JSON.stringify([p,p])).length,1);assert.notEqual(v.shortcutKey('pg1','a'),v.shortcutKey('pg2','a'));assert.notEqual(v.shortcutKey('pg1','a'),v.shortcutKey('pg1','b'));
console.log('PASS saved view validation, restored scope/search/layout, order-independent filters and per-page/account keys.');
