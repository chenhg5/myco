// Real MediaRecorder → imported timeline → shared zoom renderer → real MP4.
// Screen pixels come from a deterministic test canvas, never a personal desktop.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import http from 'node:http';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {createRequire} from 'node:module';
import ts from 'typescript';
const here=path.dirname(fileURLToPath(import.meta.url)), root=path.resolve(here,'../internal/templates/official');
const shots=fs.readFileSync(path.join(here,'shots.mjs'),'utf8');
const bridge=shots.slice(shots.indexOf('const bridge = `')+16,shots.indexOf('})();`;')+5);
const zoomJS=ts.transpile(fs.readFileSync(path.join(here,'src/video/zoom.ts'),'utf8'),{target:99,module:99});
const {zoomTransform}=await import('data:text/javascript;base64,'+Buffer.from(zoomJS).toString('base64'));
const z={start:1,end:4,x:.75,y:.75,w:.25,h:.25,strength:1};
assert.equal(zoomTransform(z,0).scale,1);assert.equal(zoomTransform(z,1).scale,1);assert.equal(zoomTransform(z,2).scale,4);assert.equal(zoomTransform(z,4).scale,1);assert.equal(zoomTransform(z,2).x,.875);assert.ok(zoomTransform(z,1.1).scale>1&&zoomTransform(z,1.1).scale<4);
const server=http.createServer((req,res)=>{
 let p=new URL(req.url,'http://test').pathname;if(p.endsWith('/'))p+='index.html';
 if(p==='/render.mjs'||p==='/zoom.mjs'){
 const src=fs.readFileSync(path.join(here,'src/video',p==='/render.mjs'?'render.ts':'zoom.ts'),'utf8');
 res.setHeader('Content-Type','text/javascript');res.end(ts.transpile(src,{target:99,module:99}).replace('"./zoom"','"/zoom.mjs"'));return;
 }
 if(p==='/bridge.js'){res.setHeader('Content-Type','text/javascript');res.end(bridge);return;}
 const f=path.resolve(root,'.'+p);if(!f.startsWith(root+path.sep)||!fs.existsSync(f)){res.statusCode=404;res.end();return;}
 const types={'.html':'text/html','.js':'text/javascript','.css':'text/css','.svg':'image/svg+xml','.json':'application/json'};
 res.setHeader('Content-Type',types[path.extname(f)]||'application/octet-stream');let b=fs.readFileSync(f);
 if(f.endsWith('.html'))b=Buffer.from(String(b).replace('<head>','<head><script src="/bridge.js"></script>'));res.end(b);
});
await new Promise(r=>server.listen(0,'127.0.0.1',r));
let chromium;
for(const dir of [here,"/usr/local/lib/node_modules","/opt/node-tools/node_modules"]) {try{chromium=createRequire(path.join(dir,"noop.js"))("playwright").chromium;break;}catch{}}
if(!chromium)throw new Error("Playwright is not installed");
const browser=await chromium.launch({headless:true,executablePath:process.env.UI_SMOKE_EXECUTABLE, args:['--no-sandbox']});
const out=process.env.RECORDER_SHOTS||'/tmp/recorder-shots';fs.mkdirSync(out,{recursive:true});
try{
 for(const [lang,theme,width] of [['zh','light',1400],['en','dark',1200]]){
 const page=await browser.newPage({viewport:{width,height:860},colorScheme:theme});const errors=[];page.on('pageerror',e=>errors.push(e.message));
 await page.addInitScript(({lang})=>{
  document.addEventListener('DOMContentLoaded',()=>{document.documentElement.lang=lang;});
  window.__SAMPLE__={project:[{id:'main',title:lang==='en'?'Product walkthrough':'产品演示',ratio:'16:9',background:'#101010'}]};
  window.__FAKE_OS__={list:async()=>({os:'windows',caps:[]})};
  Object.defineProperty(navigator.mediaDevices,'getDisplayMedia',{value:async()=>{
   if(window.__deny)throw Object.assign(new Error('cancelled'),{name:'NotAllowedError'});
   const c=document.createElement('canvas');c.width=1280;c.height=720;const x=c.getContext('2d');let n=0;
   const draw=()=>{x.fillStyle='#ecebe8';x.fillRect(0,0,1280,720);x.fillStyle='#fff';x.fillRect(80,68,1120,584);x.fillStyle='#f4f4f4';x.fillRect(80,68,210,584);x.fillStyle='#1a1a1a';x.font='600 25px sans-serif';x.fillText('myco / workspace',115,120);x.font='600 52px sans-serif';x.fillText('A clearer way to work.',355,215);x.fillStyle='#777';x.font='24px sans-serif';x.fillText('Record an idea. Make it worth sharing.',355,270);x.fillStyle='#eee';x.fillRect(355,320,740,220);x.fillStyle='#222';x.fillRect(390,365,120+(n++%180),18);x.fillStyle='#888';x.font='18px sans-serif';x.fillText('Recording studio · test footage',390,480);};draw();const id=setInterval(draw,100);const s=c.captureStream(10);s.getVideoTracks()[0].addEventListener('ended',()=>clearInterval(id));return s;
  }});
 },{lang});
 await page.goto(`http://127.0.0.1:${server.address().port}/recorder/?myco_lang=${lang}`);
 // A focus zoom must move a redaction with its source, not leave it behind.
 assert.equal(await page.evaluate(async()=>{
  const {drawFrame}=await import('/render.mjs');
  const make=()=>{const c=document.createElement('canvas');c.width=c.height=100;return c;};
  const source=make(),sc=source.getContext('2d');sc.fillStyle='white';sc.fillRect(0,0,100,100);sc.fillStyle='black';for(let x=20;x<50;x+=2)sc.fillRect(x,20,1,30);
  const blur={id:'b',type:'blur',start:0,end:5,x:.2,y:.2,w:.3,h:.3,strength:1};
  const zoom={id:'z',type:'zoom',start:0,end:5,x:.2,y:.2,w:.3,h:.3,strength:0};
  const pic={image:source,w:100,h:100};const base=make(),actual=make(),expected=make();
  drawFrame(base.getContext('2d'),100,100,pic,[blur],2);
  drawFrame(actual.getContext('2d'),100,100,pic,[blur,zoom],2);
  // scale 2, center .35/.35 -> same as the explicitly flattened source
  const e=expected.getContext('2d');e.imageSmoothingQuality='high';e.fillStyle='white';e.fillRect(0,0,100,100);e.translate(50,50);e.scale(2,2);e.translate(-35,-35);e.drawImage(base,0,0);
  const a=actual.getContext('2d').getImageData(10,10,80,80).data,b=expected.getContext('2d').getImageData(10,10,80,80).data;
  return a.every((v,i)=>Math.abs(v-b[i])<3);
 }),true,'redaction follows zoom');
 const record=lang==='en'?'Record screen':'开始录屏', choose=lang==='en'?'Choose screen & record':'选择画面并开始',stop=lang==='en'?'Stop & edit':'停止并剪辑';
 await page.getByRole('button',{name:record,exact:true}).click();await page.screenshot({path:path.join(out,`recorder-${lang}-${theme}-capture.png`)});
 // Cancelled picker returns to a usable start button, without a stuck job.
 await page.evaluate(()=>window.__deny=true);await page.getByRole('button',{name:choose,exact:true}).click();await page.getByRole('alert').waitFor();await page.evaluate(()=>window.__deny=false);
 await page.getByRole('button',{name:choose,exact:true}).click();await page.waitForTimeout(2600);await page.getByRole('button',{name:stop,exact:true}).last().click();
 await page.locator('.media').first().waitFor({timeout:30000});await page.getByRole('dialog').waitFor({state:'hidden',timeout:30000});assert.equal(await page.locator('.media').count(),1);
 const media=await page.evaluate(()=>window.myco.data.list('media'));assert.equal(media.records[0].data.poster,'','screen pixels must never be uploaded in a poster');
 await page.getByRole('tab',{name:lang==='en'?'Focus zoom':'重点放大'}).click();await page.getByRole('button',{name:lang==='en'?'Add focus zoom':'添加重点放大',exact:true}).click();
 assert.equal(await page.locator('.stage-sel.is-zoom').count(),1);
 await page.getByRole('button',{name:lang==='en'?'Focus zoom':'重点放大',exact:false}).last().click().catch(()=>{});
 // A focused overlay is part of the edit, never baked into the source.
 const list=await page.evaluate(()=>window.myco.data.list('overlays'));assert.equal(list.records[0].data.type,'zoom');
 await page.getByRole("button",{name:lang==="en"?"Go to start":"到开头",exact:true}).click();await page.waitForTimeout(500);await page.screenshot({path:path.join(out,`recorder-${lang}-${theme}-edit.png`)});
 await page.evaluate(()=>{window.myco.download=async(name,blob)=>{window.__export={name,size:blob.size,bytes:Array.from(new Uint8Array(await blob.slice(0,32).arrayBuffer()))};return {path:'~/Downloads/'+name};};});
 await page.getByRole('button',{name:lang==='en'?'Export':'导出',exact:true}).click();await page.getByRole('radio',{name:'480p',exact:true}).click();await page.getByTestId('export-start').click();
 await page.waitForFunction(()=>window.__export?.size>1000,null,{timeout:60000});const mp4=await page.evaluate(()=>window.__export);assert.equal(String.fromCharCode(...mp4.bytes.slice(4,8)),'ftyp');
 assert.deepEqual(errors,[]);await page.close();console.log(`${lang}/${theme}: picker cancel, real capture/import, focus zoom, MP4 export passed`);
 }
}finally{await browser.close();await new Promise(r=>server.close(r));}
