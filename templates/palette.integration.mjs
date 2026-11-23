// Real Hub schema/CAS/change feed with an agent writing a production palette page.
// The token stays in the Node process; only data messages cross the browser bridge.
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import http from 'node:http';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {createRequire} from 'node:module';
import {execFileSync} from 'node:child_process';
const here=path.dirname(fileURLToPath(import.meta.url));
let input='';for await(const c of process.stdin)input+=c;const fixture=JSON.parse(input);
let browser;try{const require=createRequire(path.join(execFileSync('npm',['root','-g'],{encoding:'utf8'}).trim(),'noop.js'));browser=await require('playwright').chromium.launch({headless:true,args:['--no-sandbox']});}catch{console.log('SKIP Playwright Chromium is needed for palette browser acceptance');process.exit(0);}
const api=async(route,method='GET',body)=>{const response=await fetch(fixture.url+'/v1/pages/'+fixture.page+route,{method,headers:{Authorization:'Bearer '+fixture.token,'Content-Type':'application/json'},...(body===undefined?{}:{body:JSON.stringify(body)})});const data=response.status===204?null:await response.json();if(!response.ok)throw Object.assign(new Error(data.message||'Hub request failed'),{code:data.error,latest:data.latest});return data;};
const write=(collection,id,data,version=0)=>api('/data/'+collection+'/'+id,'PUT',{data,if_version:version});
const root=path.resolve(here,'../internal/templates/official/palette');
const server=http.createServer(async(req,res)=>{const pathname=new URL(req.url,'http://test').pathname,p=path.resolve(root,'.'+(pathname==='/'?'/index.html':pathname));if(!p.startsWith(root+path.sep)){res.writeHead(403);res.end();return;}try{const data=await fs.readFile(p);res.setHeader('Content-Type',({'.html':'text/html','.js':'text/javascript','.css':'text/css','.svg':'image/svg+xml'})[path.extname(p)]||'application/octet-stream');res.end(data);}catch{res.writeHead(404);res.end();}});
await new Promise(r=>server.listen(0,'127.0.0.1',r));const context=await browser.newContext({viewport:{width:1200,height:900}});
await context.exposeBinding('paletteData',async(_,method,c,data,opts)=>{try{return{value:await(method==='list'?api('/data?collection='+encodeURIComponent(c)):method==='changes'?api('/changes?after='+opts.after+'&wait=0'):method==='get'?api('/data/'+c+'/'+opts.id):write(c,opts.id,data,opts.ifVersion))};}catch(e){return{error:{code:e.code,message:e.message,latest:e.latest}};}});
await context.addInitScript(({page})=>{const call=async(...args)=>{const r=await window.paletteData(...args);if(r.error)throw r.error;return r.value;};let seq=0;window.myco={ready:Promise.resolve({page:{id:page,name:'Palette acceptance',url:'https://workspace.example/pages/'+page},me:{name:'palette-agent'}}),data:{list:async c=>{const r=await call('list',c);seq=Math.max(seq,r.seq);return r;},get:(c,id)=>call('get',c,null,{id}),put:(c,data,o)=>call('put',c,data,o),on:f=>{let active=true;const t=setInterval(async()=>{try{const r=await call('changes',null,null,{after:seq});for(const c of r.changes){seq=Math.max(seq,c.seq);if(active)f(c);}}catch{}},100);return()=>{active=false;clearInterval(t);}}}};},{page:fixture.page});
try{
 const sample=JSON.parse(await fs.readFile(path.join(root,'sample.json'),'utf8')).palettes[0],{id,...data}=sample;
 let snapshot=await write('palettes',id,data);
 const p=await context.newPage(),errors=[];p.on('pageerror',e=>errors.push(e.message));await p.goto(`http://127.0.0.1:${server.address().port}/?myco_lang=en#palette?id=${id}`);await p.getByRole('heading',{name:data.title,exact:true}).waitFor();
 await p.getByRole('button',{name:'Favorite this palette',exact:true}).click();await p.waitForFunction(()=>document.querySelector('.pa-feature-heading button[aria-pressed=true]'));
 const favorite=(await api('/data?collection=favorites')).records[0];assert.equal(favorite.data.saved,true);
 await p.getByRole('button',{name:'Edit colors',exact:true}).click();await p.getByLabel('Palette name',{exact:true}).fill('Local draft');
 snapshot=await write('palettes',id,{...data,title:'Agent updated the palette'},snapshot.version);await p.getByRole('heading',{name:'Agent updated the palette',exact:true}).waitFor();
 await p.getByRole('button',{name:'Save palette',exact:true}).click();await p.locator('dialog [role=alert]').waitFor();assert.equal((await api('/data?collection=palettes')).records[0].data.title,'Agent updated the palette');
 await p.getByRole('button',{name:'Save as new palette',exact:true}).click();await p.locator('dialog').waitFor({state:'detached'});await p.getByRole('heading',{name:'Local draft',exact:true}).waitFor();assert.equal((await api('/data?collection=palettes')).records.length,2);
 assert.deepEqual((await api('/data?collection=favorites')).records[0].data,favorite.data);await assert.rejects(write('palettes',id,data,1),e=>e.code==='conflict');assert.deepEqual(errors,[]);await p.close();
 console.log('PASS actual Hub: official palette schema, agent create/update, live UI, shared favorite isolation, stale draft refusal, save as new palette and strict CAS');
}finally{await browser.close();await new Promise(r=>server.close(r));}
