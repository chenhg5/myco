import assert from 'node:assert/strict';
import vm from 'node:vm';
import {readFileSync} from 'node:fs';
const policySource=readFileSync(new URL('./dom_sensitive.js',import.meta.url),'utf8').replaceAll('__MYCO_SENSITIVE_CONFIG__',readFileSync(new URL('./dom_sensitive.json',import.meta.url),'utf8'));
const recorderSource=readFileSync(new URL('./recorder.js',import.meta.url),'utf8').replaceAll('__MYCO_SENSITIVE_POLICY__',policySource);

const handlers={},sent=[];
const document={addEventListener(type,f){handlers[type]=f},getElementById(){return null}};
const window={addEventListener(){},__mycoBrowserEvent(raw){sent.push(JSON.parse(raw))}};window.top=window;
vm.runInNewContext(recorderSource,{window,document,CSS:{escape:s=>s},setTimeout(){return 1},clearTimeout(){}});
window.__mycoRecorder.enabled=true;
let valueRead=false;
const password={nodeType:1,localName:'input',tagName:'INPUT',id:'password',type:'password',autocomplete:'current-password',labels:[],parentElement:null,hasAttribute(){return false},getAttribute(){return ''},getRootNode(){return document},get value(){valueRead=true;throw Error('password read')}};
handlers.input({isTrusted:true,composedPath(){return[password]}});
window.__mycoRecorder.flush();
assert.equal(valueRead,false,'collector must not read password values');
assert.equal(sent[0].sensitive,true);assert.equal(sent[0].value,'${password}');
assert.ok(sent[0].target.locators.length>=2);
handlers.input({isTrusted:false,composedPath(){return[password]}});
window.__mycoRecorder.flush();assert.equal(sent.length,1,'untrusted synthetic input was recorded');
const unnamed={nodeType:1,localName:'div',tagName:'DIV',id:'',labels:[],parentElement:null,hasAttribute(){return false},getAttribute(){return ''},getRootNode(){return document}};
const unnamedTarget=window.__mycoDescribe(unnamed);
assert.equal(unnamedTarget.locators.length,1,'unnamed elements have only a structural CSS locator');
assert.equal(unnamedTarget.locators[0].kind,'css');
assert.equal(unnamedTarget.locators[0].value,'div');
console.log('recorder: password values never read; semantic fallbacks and valid CSS-only targets retained');

window.__mycoRecorder.ignoreEvent=ev=>ev.panel===true;
handlers.click({isTrusted:true,panel:true,composedPath(){return[password]}});
handlers.input({isTrusted:true,panel:true,composedPath(){return[password]}});
window.__mycoRecorder.flush();
assert.equal(sent.length,1,'panel clicks/inputs must not become recording steps');
let contextChoices=0;
window.__mycoRecorder.contextMenu=()=>{contextChoices++;return true};
handlers.contextmenu({isTrusted:true,composedPath(){return[password]}});
handlers.contextmenu({isTrusted:false,composedPath(){return[password]}});
assert.equal(contextChoices,1,'only trusted context-menu events reach the panel');
assert.equal(sent.length,1,'panel context menu became a right-click step');
delete window.__mycoRecorder.contextMenu;

// Subframes get state synchronization but never mount controls. Reordered
// host responses must not roll a newer picking state back.
window.top={};
vm.runInNewContext(readFileSync(new URL('./panel.js',import.meta.url),'utf8'),{window,document:{createElement(){throw Error('iframe panel mounted')}}});
window.__mycoRecorder.updatePanel({revision:2,id:'rec',picking:true});
window.__mycoRecorder.updatePanel({revision:1,id:'old',picking:false});
assert.equal(window.__mycoRecorder.picking,true);
assert.equal(window.__mycoRecorder.recordingID,'rec');
console.log('recorder: panel events excluded, iframe has no UI, stale state rejected');

window.__mycoRecorder.updatePanel({revision:3,id:'rec',recording:false},true);
window.__mycoRecorder.updatePanel({revision:2,id:'rec',recording:true},true);
assert.equal(window.__mycoRecorder.enabled,false,'late context update restarted a stopped collector');


// Deterministic intent/quiet-window coverage; actual trusted input is also
// exercised by the native Chrome fixture.
function scrollFixture(){
  let now=0,sequence=0;const timers=new Map(),events={},output=[];
  const doc={addEventListener(name,fn){(events[name]??=[]).push(fn)},getElementById(){return null}};
  const el=(id,parent=null,tag='DIV')=>({id,nodeType:1,tagName:tag,localName:tag.toLowerCase(),parentElement:parent,clientLeft:0,clientWidth:100,scrollWidth:100,clientHeight:100,scrollHeight:600,scrollTop:0,labels:[],hasAttribute(){return false},getAttribute(){return ''},getRootNode(){return doc},getBoundingClientRect(){return{left:0,top:0,right:110,bottom:110}},closest(selector){return /input,textarea/.test(selector)&&['INPUT','TEXTAREA','SELECT','BUTTON'].includes(this.tagName)?this:null}});
  // closest() for ordinary divs must not invent an editable ancestor.
  const root=el('root',null,'HTML');root.closest=()=>null;doc.scrollingElement=root;
  const target=el('content',root);target.closest=selector=>/input,textarea/.test(selector)?null:target;
  const win={top:null,addEventListener(){},__mycoBrowserEvent(raw){output.push(JSON.parse(raw))}};win.top=win;
  const set=(fn,ms)=>{const id=++sequence;timers.set(id,{fn,at:now+ms});return id};
  vm.runInNewContext(recorderSource,{window:win,document:doc,CSS:{escape:s=>s},Date:{now:()=>now},WeakMap,innerWidth:110,innerHeight:110,getComputedStyle(){return{overflowY:'auto'}},setTimeout:set,clearTimeout:id=>timers.delete(id)});
  win.__mycoRecorder.enabled=true;win.__mycoRecorder.ignoreEvent=e=>!!e.panel;
  const advance=ms=>{const end=now+ms;for(;;){const next=[...timers].filter(([,v])=>v.at<=end).sort((a,b)=>a[1].at-b[1].at)[0];if(!next)break;now=next[1].at;timers.delete(next[0]);next[1].fn()}now=end};
  const event=(name,e=target,extra={})=>{const ev={type:name,target:e,isTrusted:true,button:0,buttons:1,pointerId:1,clientX:50,clientY:50,deltaY:80,composedPath(){return[e,doc]},...extra};for(const fn of events[name]||[])fn(ev)};
  const scroll=(value,e=target)=>{e.scrollTop=value;event('scroll',e)};
  return{win,doc,root,target,el,event,scroll,advance,output,steps:()=>output.filter(n=>n.type==='scroll')};
}
{
 const f=scrollFixture();f.scroll(50);f.advance(1000);assert.equal(f.steps().length,0,'unprompted scroll recorded');
 f.event('wheel');f.scroll(90);f.advance(180);assert.equal(f.steps().length,1,'wheel scroll missing');
 f.advance(901);f.scroll(100);f.advance(180);assert.equal(f.steps().length,1,'expired intent recorded a programmatic scroll');
}
{
 const f=scrollFixture();f.event('click');f.event('wheel');f.scroll(80);f.advance(401);assert.equal(f.steps().length,0,'click side-effect scroll recorded');
 f.event('wheel');f.scroll(120);f.advance(180);assert.equal(f.steps().length,1);
 f.event('focusin');f.scroll(150);f.advance(500);assert.equal(f.steps().length,1,'focus side-effect scroll recorded');
 const input=f.el('field',f.target,'INPUT');input.type='text';input.value='fixture';input.closest=()=>input;
 f.event('input',input);f.event('wheel');f.scroll(180);f.advance(500);assert.equal(f.steps().length,1,'input side-effect scroll recorded');
}
{
 const f=scrollFixture();f.event('wheel');f.scroll(20,f.root);f.advance(180);assert.equal(f.steps().length,0,'intent leaked into another container');
 f.scroll(80);f.advance(180);assert.equal(f.steps().length,1);
 f.event('wheel');f.target.scrollHeight+=20;f.scroll(100);f.advance(180);assert.equal(f.steps().length,1,'layout change used stale intent');
}
{
 const f=scrollFixture();f.event('keydown',f.target,{key:'PageDown'});f.scroll(80);f.advance(180);assert.equal(f.steps().length,1,'keyboard page scroll missing');
 assert.equal(f.output.filter(n=>n.type==='press').length,0,'keyboard scrolling recorded twice');
 const input=f.el('field',f.target,'INPUT');input.closest=()=>input;
 f.event('keydown',input,{key:'ArrowDown'});f.scroll(100);f.advance(500);assert.equal(f.steps().length,1,'caret-following scroll recorded');
}
{
 const f=scrollFixture();f.event('pointerdown',f.target,{clientX:105});f.event('pointermove',f.target,{clientX:105});f.scroll(80);f.event('pointerup',f.target,{clientX:105});f.event('click',f.target,{clientX:105});f.advance(180);
 assert.equal(f.steps().length,1,'scrollbar drag missing');assert.equal(f.output.filter(n=>n.type==='click').length,0,'scrollbar click recorded twice');
}
{
 const f=scrollFixture();for(const kind of ['wheel','pointerdown','keydown'])f.event(kind,f.target,{panel:true,key:'PageDown'});f.scroll(80);f.advance(500);assert.equal(f.steps().length,0,'panel armed page scrolling');
 f.event('wheel');f.scroll(100);f.win.__mycoRecorder.updatePanel({revision:1,id:'rec',picking:true});f.advance(500);assert.equal(f.steps().length,0,'pending scroll survived selection mode');
 f.win.__mycoRecorder.updatePanel({revision:2,id:'rec',picking:false});f.advance(401);
 f.event('wheel',f.target,{isTrusted:false});f.scroll(130);f.advance(500);assert.equal(f.steps().length,0,'synthetic wheel armed scrolling');
}
{
 const f=scrollFixture();f.event('wheel');f.event('keydown',f.target,{panel:true,key:'PageDown'});f.scroll(80);f.advance(500);assert.equal(f.steps().length,0,'panel key retained page scroll intent');
}
console.log('recorder: explicit container intent, quiet window, scrollbar/key paths and side-effect exclusions passed');

// All privacy paths use the shared classifier, including displayed passwords
// and common OTP aliases. The recorder must never read their values.
window.__mycoRecorder.enabled=true;
for(const fields of [{type:'text',autocomplete:'current-password'},{type:'text',autocomplete:'',name:'otp_code'},{type:'text',autocomplete:'',name:'sms_otp'}]){
 const field=Object.assign(Object.create(password),fields,{getAttribute(name){return name==='name'?this.name||'':''}});const before=sent.length;
 handlers.input({isTrusted:true,composedPath(){return[field]}});window.__mycoRecorder.flush();
 assert.equal(sent.length,before+1);assert.equal(sent.at(-1).sensitive,true);assert.equal(sent.at(-1).value,'${password}');
}
assert.equal(valueRead,false,'sensitive alias value read');

const sharedSensitive=vm.runInNewContext('('+policySource+')');
for(const name of ['footprint','passwordless','totp'])assert.equal(sharedSensitive({nodeType:1,localName:'input',getAttribute(key){return key==='name'?name:''}}),false);
const namedInput={nodeType:1,localName:'input',getAttribute(){return ''}};
const form={nodeType:1,localName:'form',name:namedInput,getAttribute(key){return key==='name'?'reset_password_form':''}};
assert.equal(sharedSensitive(form),false,'form name tainted its descendants');
assert.equal(sharedSensitive({...namedInput,parentElement:form}),false,'named form access tainted public field');
