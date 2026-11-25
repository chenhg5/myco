import assert from 'node:assert/strict';
import vm from 'node:vm';
import {readFileSync} from 'node:fs';
const handlers={},sent=[];
const document={addEventListener(type,f){handlers[type]=f},getElementById(){return null}};
const window={addEventListener(){},__mycoBrowserEvent(raw){sent.push(JSON.parse(raw))}};window.top=window;
vm.runInNewContext(readFileSync(new URL('./recorder.js',import.meta.url),'utf8'),{window,document,CSS:{escape:s=>s},setTimeout(){return 1},clearTimeout(){}});
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
console.log('recorder: password values never read; parameter reference and fallback locators retained');

window.__mycoRecorder.ignoreEvent=ev=>ev.panel===true;
handlers.click({isTrusted:true,panel:true,composedPath(){return[password]}});
handlers.input({isTrusted:true,panel:true,composedPath(){return[password]}});
window.__mycoRecorder.flush();
assert.equal(sent.length,1,'panel clicks/inputs must not become recording steps');
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
