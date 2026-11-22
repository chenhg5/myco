// Production template bundles + the actual pagebridge.js in sandboxed frames.
// --hub additionally exercises the actual agent SDK/Hub/private artifact route;
// the parent renderer transport is simulated, not a native desktop acceptance.
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import http from 'node:http';
import {createRequire} from 'node:module';
import {execFileSync} from 'node:child_process';
import {fileURLToPath} from 'node:url';
import {actionModule} from './action-test-modules.mjs';
import {build} from 'esbuild';
import JSZip from 'jszip';
const here=path.dirname(fileURLToPath(import.meta.url)),root=path.resolve(here,'../internal/templates/official');
let fixture;
if(process.argv.includes('--hub')){let s='';for await(const p of process.stdin)s+=p;fixture=JSON.parse(s);}
let chromium;
try{
 try{chromium=createRequire(import.meta.url)('playwright').chromium;}
 catch{const require=createRequire(path.join(execFileSync(process.platform==='win32'?'npm.cmd':'npm',['root','-g'],{encoding:'utf8'}).trim(),'noop.js'));chromium=require('playwright').chromium;}
}
catch{console.log('SKIP Playwright is not installed');process.exit(0);}
let browser;
try{browser=await chromium.launch({headless:true,args:['--no-sandbox']});}
catch{console.log('SKIP Chromium is not installed');process.exit(0);}
const bridge=await fs.readFile(path.resolve(here,'../internal/console/pagebridge.js'),'utf8');
const sdk=(await build({entryPoints:[path.join(here,'src/_lib/register-actions.ts')],bundle:true,write:false,format:'iife',globalName:'actionSDK',logLevel:'silent'})).outputFiles[0].text;
const server=http.createServer(async(req,res)=>{
 const url=new URL(req.url,'http://test');
 if(url.pathname==='/'){res.setHeader('Content-Type','text/html');res.end('<!doctype html><html><body></body></html>');return;}
 if(url.pathname==='/test-sdk.js'){res.setHeader('Content-Type','text/javascript');res.end(sdk);return;}
 const name=path.resolve(root,'.'+url.pathname);
 if(!name.startsWith(root+path.sep)){res.writeHead(403).end();return;}
 try{let content=await fs.readFile(name);const ext=path.extname(name);res.setHeader('Content-Type',({'.html':'text/html','.js':'text/javascript','.css':'text/css','.svg':'image/svg+xml'})[ext]||'application/octet-stream');
 if(ext==='.html')content=content.toString().replace('<head>','<head><script>'+bridge.replace(/<\/script/gi,'<\\/script')+'</script>');res.end(content);
 }catch{res.writeHead(404).end();}
});
await new Promise(r=>server.listen(0,'127.0.0.1',r));
const origin='http://127.0.0.1:'+server.address().port;
const errors=[],forbidden=[];
const request=token=>(route,init={})=>fetch(fixture.url+route,{...init,headers:{...init.headers,Authorization:'Bearer '+token},signal:AbortSignal.timeout(15000)});
let agent,client,owner,host;
async function hostCall(route,body,headers){const r=await owner(route,{method:'POST',headers:headers||{'Content-Type':'application/json'},body:headers?body:JSON.stringify(body)});const value=await r.json();assert(r.ok,'host request failed '+r.status+': '+value.error+' '+value.message);return value;}
if(fixture){
 const {ActionClient}=await import(await actionModule('_lib/actions'));const {ActionHTTPTransport}=await import(await actionModule('_lib/actions-http'));
 agent=new ActionHTTPTransport(request(fixture.agent));client=new ActionClient(agent);owner=request(fixture.owner);
 host=await hostCall('/v1/console/action-hosts',{provider:'page',generation:'page-apps-1',actions:[{name:'page.export',version:1,title:'Export',permission:'page.export'}]});
}
const output=fixture?.output||process.env.PAGE_EXPORT_SHOTS;
if(output)await fs.mkdir(output,{recursive:true});
let counter=0;
async function assertButtonLabel(page, bytes, mediaType = 'image/png') {
 const ink=await page.evaluate(async ({bytes,mediaType})=>{
  const image=new Image(),url=URL.createObjectURL(new Blob([new Uint8Array(bytes)],{type:mediaType}));
  try {
   await new Promise((resolve,reject)=>{image.onload=resolve;image.onerror=reject;image.src=url;});
   const scale=Math.min(1,640/image.width),canvas=document.createElement('canvas');
   canvas.width=Math.round(image.width*scale);canvas.height=Math.round(image.height*scale);
   const ctx=canvas.getContext('2d');ctx.drawImage(image,0,0,canvas.width,canvas.height);
   const k=canvas.width/1440,pixels=ctx.getImageData(0,0,canvas.width,canvas.height).data;
   let min=Infinity,max=-Infinity,count=0;
   for(let y=Math.ceil(495*k);y<Math.floor(547*k);y++)for(let x=Math.ceil(112*k);x<Math.floor(200*k);x++){
    const offset=(y*canvas.width+x)*4;
    // Small scaled glyphs are antialiased; distinguish ink from #222.
    if(pixels[offset]>140&&pixels[offset+1]>140&&pixels[offset+2]>140){min=Math.min(min,y);max=Math.max(max,y);count++;}
   }
   return {min,max,count,center:521*k};
  } finally {URL.revokeObjectURL(url);}
 },{bytes:Array.from(bytes),mediaType});
 assert(ink.count>5,'button label disappeared from the export: '+JSON.stringify(ink));
 assert(Math.abs((ink.min+ink.max)/2-ink.center)<=3,'button label is vertically displaced: '+JSON.stringify(ink));
}
try{
 for(const app of ['whiteboard','design-studio','slides']){
  const sample=JSON.parse(await fs.readFile(path.join(root,app,'sample.json'),'utf8'));
  let records=Object.entries(sample).flatMap(([collection,rows])=>rows.map(({id,...data})=>({collection,id,data,version:1})));
  const pageID=fixture?.pages[app]||'test-page';
  if(fixture){
   // Fixtures are isolated; load the same samples the user can opt into.
   for(const r of records){const response=await request(fixture.agent)('/v1/pages/'+pageID+'/data/'+r.collection+'/'+r.id,{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({data:r.data,if_version:0})});assert(response.ok,'sample write failed '+response.status);}
  }
  const context=await browser.newContext({viewport:{width:1280,height:800}});
  await context.exposeBinding('readShared',async(_,method,args)=>{
   if(!['data.list','data.get'].includes(method))throw Error('unexpected data access');
   if(fixture){const route='/v1/pages/'+pageID+'/data'+(method==='data.get'?'/'+encodeURIComponent(args.collection)+'/'+encodeURIComponent(args.id):'?collection='+encodeURIComponent(args.collection||''));const response=await owner(route);const value=await response.json();return response.ok?{value}:{error:value.message||value.error,code:value.error};}
   if(method==='data.list')return {value:{records:records.filter(r=>!args.collection||r.collection===args.collection),seq:1}};
   const record=records.find(r=>r.collection===args.collection&&r.id===args.id);return record?{value:record}:{error:'Missing record',code:'not_found'};
  });
  const page=await context.newPage();page.on('pageerror',e=>errors.push(e.message));
  await page.goto(origin);
  await page.evaluate(({app,pageID})=>{
   window.results={};window.registered=null;window.pending={};window.forbidden=[];
   const frame=document.createElement('iframe');frame.sandbox='allow-scripts allow-same-origin';frame.style='width:1280px;height:800px';document.body.append(frame);window.frame=frame;
   window.addEventListener('message',async e=>{
    if(e.source!==frame.contentWindow||e.origin!==location.origin)return;
    const d=e.data;if(!d)return;
    if(d.myco==='call'){
     let response;
     if(d.method==='ready')response={value:{page:{id:pageID,name:'Shared export'},me:{name:'Test owner'},render:{instance:'render-instance'},preferences:{language:'en',theme:'light'}}};
     else if(['data.list','data.get'].includes(d.method))response=await window.readShared(d.method,d.args);
     else if(d.method==='data.watch')response={value:true};
     else if(d.method==='storage.get')response={value:null};
     else{window.forbidden.push(d.method);response={error:'Rendering is read-only',code:'unavailable'};}
     frame.contentWindow.postMessage({myco:'result',id:d.id,...response},location.origin);
    }else if(d.myco==='actions.registered')window.registered=d;
    else if(d.myco==='actions.result'){
     if(d.result){const {data,...meta}=d.result;window.results[d.id]={...meta,bytes:Array.from(new Uint8Array(data))};}
     else window.results[d.id]={error:d.error};
    }
   });
   frame.src='/'+app+'/index.html';
  },{app,pageID});
  await page.waitForFunction(()=>window.registered,{timeout:10000});
  assert.deepEqual(await page.evaluate(()=>window.registered.actions),[{name:'page.export',version:1}]);
  const invoke=async(args,options={})=>{
   const id='call-'+(++counter);
   await page.evaluate(({id,args,...options})=>window.frame.contentWindow.postMessage({myco:'actions.invoke',instance:'render-instance',id,action:'page.export',version:1,args,...options},location.origin),{id,args,...options});
   await page.waitForFunction(id=>window.results[id],id,{timeout:20000});
   return page.evaluate(id=>window.results[id],id);
  };
  const wrapped=async(args)=>{
   if(!fixture)return invoke(args);
   const grant=await client.request({target:{kind:'page',id:pageID},permissions:['page.export'],reason:'Template export integration'});assert.equal(grant.state,'active');
   const session=await client.session(grant.id);const task=await session.invoke('page.export',1,args,'export-'+(++counter));
   const response=await owner('/v1/console/action-hosts/'+host.id+'/jobs?wait_seconds=0');assert(response.ok);const jobs=(await response.json()).jobs;assert.equal(jobs.length,1);assert.equal(jobs[0].job.id,task.job.id);
   const rendered=await invoke(args);assert(!rendered.error,JSON.stringify(rendered.error));
   const {bytes,...metadata}=rendered;
   const artifact=await hostCall('/v1/console/action-jobs/'+task.job.id+'/artifacts?host_id='+host.id+'&generation=page-apps-1',new Uint8Array(bytes),{'Content-Type':metadata.media_type});
   await hostCall('/v1/console/action-jobs/'+task.job.id+'/complete',{host_id:host.id,generation:'page-apps-1',state:'succeeded',result:{...metadata,artifact},artifact_ids:[artifact.id]});
   const result=await task.wait();assert.equal(result.artifact.id,artifact.id);const file=await session.read(result.artifact);assert.deepEqual(Array.from(file),bytes);
   await session.end();await assert.rejects(()=>agent.readArtifact(grant.id,artifact.id),{status:410});
   return rendered;
  };
  const cases=app==='whiteboard'?[{format:'svg'},{format:'png',max_width:640}]:app==='design-studio'?[{format:'svg',artifact_id:'design-website'},{format:'png',artifact_id:'design-website',max_width:640},{format:'json',artifact_id:'design-website'},{format:'html',artifact_id:'design-website'}]:[{format:'pptx'},{format:'md',slide_ids:['s2','s1']}];
  for(const args of cases){
   const result=await wrapped(args);assert(!result.error,JSON.stringify(result.error));const bytes=Buffer.from(result.bytes);assert(bytes.length>10);
   if(output)await fs.writeFile(path.join(output,app+'.'+args.format),bytes);
   if(args.format==='png'){
    assert(bytes.subarray(0,8).equals(Buffer.from([137,80,78,71,13,10,26,10])));assert(result.width<=640);assert.equal(bytes.readUInt32BE(16),result.width);
    if(app==='design-studio'){
     // The sample's white label belongs inside its dark button, vertically
     // centered. Inspect actual encoded pixels, not SVG attribute spelling.
     await assertButtonLabel(page,result.bytes);
    }
   }
   if(args.format==='svg'){
    assert(bytes.toString().startsWith('<svg'));assert(!bytes.toString().includes('<script>'));
    // Simulate a renderer that ignores SVG's optional top-edge baseline hint.
    // The old WebKit output placed the label against the button's top edge.
    if(app==='design-studio')await assertButtonLabel(page,Buffer.from(bytes.toString().replace(/\sdominant-baseline="[^"]*"/g,'')),'image/svg+xml');
   }
   if(args.format==='pptx'){
    const zip=await JSZip.loadAsync(bytes);assert.equal(result.pages,8);
    assert.equal(Object.keys(zip.files).filter(k=>/^ppt\/slides\/slide\d+\.xml$/.test(k)).length,8);
    assert((await zip.file('ppt/slides/slide1.xml').async('string')).includes('Q4 产品发布会'));
    assert((await zip.file('ppt/slides/slide4.xml').async('string')).includes('<c:chart'));
   }
   if(args.format==='md'){assert(bytes.toString().indexOf('议程')<bytes.toString().lastIndexOf('Q4 产品发布会'));assert.equal(result.pages,2);}
   if(args.format==='json')assert.equal(JSON.parse(bytes).title,'日常的形状');
  }
  assert.equal((await invoke({format:'pdf'})).error.code,'invalid_argument');
  assert.equal((await invoke(cases[0],{version:2})).error.code,'unsupported');
  if(app==='design-studio'){
   assert.equal((await invoke({format:'svg',artifact_id:'missing'})).error.code,'not_found');
   assert.equal((await invoke({format:'svg',artifact_id:'design-website',frame_id:'missing'})).error.code,'invalid_argument');
  }
  if(app==='slides'){
   assert.equal((await invoke({format:'md',slide_ids:['s1','s1']})).error.code,'invalid_argument');
   assert.equal((await invoke({format:'md',slide_ids:['missing']})).error.code,'not_found');
  }
  // No watch event: the editor's cached Store is deliberately stale. Exports
  // must fetch the newly committed shared data instead of exporting that cache.
  const record=records.find(r=>r.collection===(app==='whiteboard'?'items':app==='slides'?'slides':'artifacts'));
  const updated=structuredClone(record.data),fresh='Fresh shared content 中文';
  if(app==='design-studio')updated.frames[0].nodes.find(n=>n.kind==='text').text=fresh;
  else if(app==='slides')updated.title=fresh;
  else updated.text=fresh;
  if(fixture){
   const route='/v1/pages/'+pageID+'/data/'+record.collection+'/'+record.id;
   const old=await (await request(fixture.agent)(route)).json();
   const response=await request(fixture.agent)(route,{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({data:updated,if_version:old.version})});assert(response.ok,'fresh content update failed');
  }else record.data=updated;
  const freshResult=await invoke(app==='slides'?{format:'md'}:cases[0]);
  assert(Buffer.from(freshResult.bytes).toString().includes(fresh),'handler exported stale editor content');
  if(!fixture&&app==='whiteboard'){
   const original=records;records=Array.from({length:257},(_,i)=>({collection:'items',id:'n'+i,data:{type:'rect',text:'node'}}));
   assert.equal((await invoke({format:'svg'})).error.code,'too_large','large boards fail instead of truncating');records=original;
  }
  if(!fixture&&app==='design-studio'){
   record.data.frames.push({...structuredClone(record.data.frames[0]),id:'second-frame'});
   assert.equal((await invoke(cases[0])).error.code,'invalid_argument','multi-frame images require an explicit selection');
   assert(!(await invoke({...cases[0],frame_id:record.data.frames[0].id})).error);record.data.frames.pop();
  }
  const unknown='wrong-instance';await page.evaluate(id=>window.frame.contentWindow.postMessage({myco:'actions.invoke',instance:'other',id,action:'page.export',version:1,args:{format:'svg'}},location.origin),unknown);
  await page.waitForTimeout(100);assert.equal(await page.evaluate(id=>window.results[id],unknown),undefined);
  const frame=page.frames().find(f=>f.url().includes('/'+app+'/index.html'));
  await frame.addScriptTag({url:origin+'/test-sdk.js'});
  await frame.evaluate(()=>{
   window.testAborted=false;
   window.testRegistration=actionSDK.registerActions([{name:'page.export',version:1,input_schema:{type:'object'},handler:(_,ctx)=>new Promise(resolve=>{
    window.finishLate=()=>resolve({media_type:'text/plain',data:new Blob(['late'])});
    ctx.signal.addEventListener('abort',()=>window.testAborted=true,{once:true});
   })}]);
  });
  await page.evaluate(()=>window.frame.contentWindow.postMessage({myco:'actions.invoke',instance:'render-instance',id:'cancelled-call',action:'page.export',version:1,args:{}},location.origin));
  await frame.waitForFunction(()=>window.finishLate);
  await page.evaluate(()=>window.frame.contentWindow.postMessage({myco:'actions.cancel',instance:'render-instance',id:'cancelled-call'},location.origin));
  await frame.waitForFunction(()=>window.testAborted);
  await frame.evaluate(()=>{window.finishLate();window.testRegistration.dispose();});
  await page.waitForTimeout(100);assert.equal(await page.evaluate(()=>window.results['cancelled-call']),undefined,'cancelled SDK handler must never publish a late file');
  forbidden.push(...await page.evaluate(()=>window.forbidden));
  if(output)await page.screenshot({path:path.join(output,app+'-renderer.png')});
  await context.close();console.log('PASS '+app+': production export bytes, validation, versions, instance guard'+(fixture?', real Hub grants/private artifacts/revocation':''));
 }
 assert.deepEqual(forbidden,[],'rendering must not request writes, OS, network or downloads');assert.deepEqual(errors,[]);
}finally{await browser.close();await new Promise(r=>server.close(r));}
