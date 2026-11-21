import assert from 'node:assert/strict';
import fs from 'node:fs';
import http from 'node:http';
import path from 'node:path';
import {createRequire} from 'node:module';
const root=path.resolve(import.meta.dirname,'../internal/templates/official/remote-desktop');
const source=fs.readFileSync(path.resolve(import.meta.dirname,'../ios/MycoKit/Sources/MycoFeatures/PageBridgeScript.swift'),'utf8');
const bridge=source.match(/#"""([\s\S]*?)"""#/)[1];
const out=path.resolve(import.meta.dirname,'remote-mobile-shots');fs.mkdirSync(out,{recursive:true});
const server=http.createServer((req,res)=>{
 const url=new URL(req.url,'http://local');const file=path.resolve(root,'.'+url.pathname);
 if(!file.startsWith(root+path.sep)||!fs.existsSync(file)){res.writeHead(404).end();return;}
 res.setHeader('Content-Type',({'.html':'text/html','.js':'text/javascript','.css':'text/css','.svg':'image/svg+xml'})[path.extname(file)]||'application/octet-stream');
 let content=fs.readFileSync(file);
 if(file.endsWith('.html'))content=String(content).replace('<head>',`<head><script>
 window.__mycoInitialPreferences={language:${JSON.stringify(url.searchParams.get('lang')||'en')},theme:${JSON.stringify(url.searchParams.get('theme')||'light')}};
 window.__inputs=[];window.__actions=[];window.__supportsText=true;window.__allowControl=${JSON.stringify(url.searchParams.get('control')!=='false')};
 window.webkit={messageHandlers:{myco:{postMessage(d){let value;const a=d.args||{};
  if(d.method==='ready')value={page:{id:'pg_test',name:'Remote fixture'},me:{name:'me'},state:null,version:1,preferences:window.__mycoInitialPreferences};
  else if(d.method==='data.list')value={records:[],seq:0};
  else if(d.method==='remote.call'){window.__actions.push(a.action);if(a.action==='status')value={supported:false,installed:false,running:false,protocol:1,version:'controller',reason:'controller_only'};if(a.action==='devices')value=[{machine:'mac',name:'Studio Mac',os:'darwin',control:window.__allowControl}];if(a.action==='connect')value={id:'rd_test',machine:'mac',state:'connected',answer:{type:'answer',sdp:'v=0'}};if(a.action==='session')value={id:'rd_test',machine:'mac',state:'connected',answer:{type:'answer',sdp:'v=0'}};}
  setTimeout(()=>window.__mycoAnswer(d.id,{value:value??true}),0);
 }}}};
 class TestChannel{readyState='open';bufferedAmount=0;send(s){window.__inputs.push(JSON.parse(s));}close(){this.readyState='closed';this.onclose?.();}}
 class TestPeer extends EventTarget{iceGatheringState='complete';connectionState='new';localDescription={toJSON:()=>({type:'offer',sdp:'v=0'})};addTransceiver(){}createDataChannel(){return this.channel=new TestChannel();}async createOffer(){return {type:'offer',sdp:'v=0'};}async setLocalDescription(){}async setRemoteDescription(){this.connectionState='connected';this.onconnectionstatechange?.();const canvas=document.createElement('canvas');canvas.width=1280;canvas.height=720;const ctx=canvas.getContext('2d');ctx.fillStyle='#bfbfbf';ctx.fillRect(0,0,1280,720);ctx.fillStyle='#fafafa';ctx.fillRect(100,90,1080,530);ctx.fillStyle='#222';ctx.font='32px sans-serif';ctx.fillText('SIMULATED DESKTOP · UI TEST',145,165);ctx.font='24px sans-serif';ctx.fillText('Touchpad, keyboard and connection lifecycle',145,225);this.ontrack?.({streams:[canvas.captureStream(5)]});this.channel?.onmessage?.({data:JSON.stringify({type:'capabilities',input_features:window.__supportsText?['text']:[]})});}async getStats(){return new Map();}close(){this.connectionState='closed';}}
 window.RTCPeerConnection=TestPeer;
 </script><script>${bridge}</script>`);
 res.end(content);
});
await new Promise(r=>server.listen(0,'127.0.0.1',r));
let chromium;try{({chromium}=createRequire(import.meta.url)('playwright'));}catch{({chromium}=createRequire('/usr/local/lib/node_modules/playwright/package.json')('.'));}
const browser=await chromium.launch({headless:true,...(process.env.UI_SMOKE_EXECUTABLE?{executablePath:process.env.UI_SMOKE_EXECUTABLE}:{})});
try{
 for(const [width,height,lang,theme] of [[390,844,'zh','light'],[390,844,'en','dark'],[844,390,'zh','dark'],[844,390,'en','light'],[390,844,'zh','dark'],[390,844,'en','light']]){
  const context=await browser.newContext({viewport:{width,height},hasTouch:true,isMobile:true,deviceScaleFactor:1});const page=await context.newPage(),errors=[];page.on('pageerror',e=>errors.push(e.message));
  await page.goto(`http://127.0.0.1:${server.address().port}/index.html?lang=${lang}&theme=${theme}&control=${(width===390&&((lang==='zh'&&theme==='dark')||(lang==='en'&&theme==='light')))?'false':'true'}`);
  await page.locator('.rd-device').first().waitFor();
  assert.equal(await page.locator('.rd-tabs button').count(),1,'a phone must not offer host installation or sharing');
  assert.equal(await page.locator('[aria-label="组件设置"],[aria-label="Component settings"]').count(),0);
  const readOnly=width===390&&((lang==='zh'&&theme==='dark')||(lang==='en'&&theme==='light'));
  await page.locator('.rd-device').first().click();await page.locator('.rd-dialog .rd-field input').fill('a1b2c3d4');
  const permission=page.locator('.rd-dialog input[type="checkbox"]');
  if(readOnly){assert(await permission.isDisabled());await page.locator('.rd-control-help').waitFor();}else await permission.check();
  await page.locator('.rd-dialog .rd-primary').click();
  await page.locator('.rd-screen video').waitFor();await page.waitForFunction(()=>document.querySelector('video')?.videoWidth>0);
  let box=await page.locator('.rd-screen').boundingBox(),x=Math.round(box.x+box.width/2),y=Math.round(box.y+box.height/2);
  const cdp=await context.newCDPSession(page);const touch=(type,points)=>cdp.send('Input.dispatchTouchEvent',{type,touchPoints:points});
  await page.locator('.rd-session.is-expanded.is-immersive').waitFor();
  await page.getByRole('button',{name:lang==='zh'?'工具':'Tools',exact:true}).click();
  const inputCount=await page.evaluate(()=>window.__inputs.length);
  await touch('touchStart',[{x:x-30,y,id:1},{x:x+30,y,id:2}]);
  await touch('touchMove',[{x:x-65,y,id:1},{x:x+65,y,id:2}]);await touch('touchEnd',[]);
  assert(parseInt(await page.locator('.rd-zoom').innerText())>150,'pinch must zoom in any permission mode');
  assert(!(await page.evaluate(n=>window.__inputs.slice(n),inputCount)).some(e=>['scroll','move','button'].includes(e.kind)),'a pinch must not scroll or click on the host');
  await page.getByRole('button',{name:lang==='zh'?'适应画面':'Fit to screen',exact:true}).click();
  assert((await page.locator('.rd-session').boundingBox()).height>=height-2,'a phone connection must use the available height automatically');
  if(!readOnly){
  await touch('touchStart',[{x,y,id:1}]);await touch('touchEnd',[]);assert.equal(await page.evaluate(()=>window.__inputs.filter(x=>x.kind==='button').length),2);
  const old=await page.evaluate(()=>window.__inputs.length);await touch('touchStart',[{x,y,id:1}]);await touch('touchMove',[{x:x+45,y:y+15,id:1}]);await touch('touchEnd',[]);
  const moved=await page.evaluate(n=>window.__inputs.slice(n),old);assert(moved.some(x=>x.kind==='move'));assert(!moved.some(x=>x.kind==='button'),'moving is not dragging');
  await touch('touchStart',[{x:x-20,y,id:1},{x:x+20,y,id:2}]);await touch('touchMove',[{x:x-20,y:y-20,id:1},{x:x+20,y:y-20,id:2}]);await touch('touchEnd',[]);assert(await page.evaluate(()=>window.__inputs.some(x=>x.kind==='scroll')));
  await page.getByRole('button',{name:lang==='zh'?'放大画面':'Zoom in',exact:true}).click();
  assert.equal(await page.locator('.rd-zoom').innerText(),'150%');
  assert.match(await page.locator('.rd-screen video').getAttribute('style'),/scale\(1.5\)/);
  await page.getByRole('button',{name:lang==='zh'?'缩小画面':'Zoom out',exact:true}).click();
  const drag=page.locator('.rd-touch-row button').filter({hasText:lang==='zh'?'拖动':'Drag'}).first();
  await drag.click();assert.equal(await drag.getAttribute('aria-pressed'),'true');
  await touch('touchStart',[{x,y,id:1}]);await touch('touchMove',[{x:x+10,y:y+10,id:1}]);await touch('touchEnd',[]);await drag.click();assert.equal(await drag.getAttribute('aria-pressed'),'false');
  const keyboard=page.locator('.rd-touch-row button').filter({hasText:lang==='zh'?'键盘':'Keyboard'}).first();await keyboard.click();const field=page.locator('.rd-keyboard textarea');await field.fill('你好🙂');
  assert(await page.evaluate(()=>window.__inputs.some(x=>x.kind==='text'&&x.text==='你好🙂')));
  await field.dispatchEvent('compositionstart');await field.evaluate(el=>{Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype,'value').set.call(el,'中文组合');el.dispatchEvent(new InputEvent('input',{bubbles:true,data:'中文组合',isComposing:true}));});await field.dispatchEvent('compositionend',{data:'中文组合'});
  assert.equal(await page.evaluate(()=>window.__inputs.filter(x=>x.kind==='text'&&x.text==='中文组合').length),1,'composition must send once');
  }else {
    await page.getByRole('button',{name:lang==='zh'?'放大画面':'Zoom in',exact:true}).click();
    const before=await page.locator('video').getAttribute('style');
    await touch('touchStart',[{x,y,id:1}]);await touch('touchMove',[{x:x+25,y:y+15,id:1}]);await touch('touchEnd',[]);
    assert.notEqual(await page.locator('video').getAttribute('style'),before,'one finger pans a zoomed view-only desktop');
    assert(!await page.evaluate(()=>window.__inputs.some(e=>['move','button','key','text','scroll'].includes(e.kind))),'view-only gestures must never reach the host');
  }
  await page.screenshot({path:path.join(out,`${width}-${height}-${lang}-${theme}-tools.png`),fullPage:true});
  await page.getByRole('button',{name:lang==='zh'?'工具':'Tools',exact:true}).click();
  assert(!await page.locator('.rd-touch-controls').isVisible(),'tools must tuck away without shrinking the desktop');
  await page.screenshot({path:path.join(out,`${width}-${height}-${lang}-${theme}.png`),fullPage:true});
  assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),'phone viewport overflows');
  await page.evaluate(()=>window.__mycoEvent({myco:'remote-disconnect'}));await page.locator('.rd-device').first().waitFor();assert(await page.evaluate(()=>window.__actions.includes('disconnect')));
  assert.deepEqual(errors,[]);await context.close();console.log(`PASS mobile ${width}×${height} ${lang} ${theme}`);
 }
}finally{await browser.close();server.close();}
