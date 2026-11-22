// Generic registration lifecycle; no application semantics or credentials here.
import assert from 'node:assert/strict';
import { build } from 'esbuild';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
const require = createRequire(import.meta.url);
globalThis.window = {};
async function load(entry) {
  const result = await build({entryPoints:[fileURLToPath(new URL(entry, import.meta.url))],bundle:true,write:false,platform:'node',format:'cjs',logLevel:'silent'});
  const module={exports:{}};new Function('module','exports','require',result.outputFiles[0].text)(module,module.exports,require);return module.exports;
}
const {registerActions,pageActionMaxBytes}=await load('./src/_lib/register-actions.ts');
let invoke, unregistered=0, calls=0;
const bridge={features:['page-actions'],actions:{register(descriptors,f){assert.deepEqual(descriptors,[{name:'page.export',version:1,input_schema:{type:'object'}}]);invoke=f;return()=>unregistered++;}}};
const definition={name:'page.export',version:1,input_schema:{type:'object'},handler:()=>{calls++;return {media_type:'image/svg+xml',data:new Blob(['<svg/>'])};}};
assert.equal(registerActions([definition],{}).supported,false);
assert.equal(registerActions([definition],{features:['page-actions']}).supported,false);
assert.throws(()=>registerActions([definition,definition],bridge),{code:'invalid_argument'});
assert.throws(()=>registerActions([{...definition,version:1.5}],bridge),{code:'invalid_argument'});
assert.throws(()=>registerActions([{...definition,name:42}],bridge),{code:'invalid_argument'});
const request=(signal=new AbortController().signal)=>({id:'job',action:'page.export',version:1,args:{format:'svg'},signal});
let registration=registerActions([definition],bridge);
assert.equal(await (await invoke(request())).data.text(),'<svg/>');
await assert.rejects(()=>invoke({...request(),version:2}),{code:'unsupported'});
const before=new AbortController();before.abort();
await assert.rejects(()=>invoke(request(before.signal)),{code:'cancelled'});
assert.equal(calls,1);
const between=new AbortController(), betweenResult=invoke(request(between.signal));between.abort();
await assert.rejects(()=>betweenResult,{code:'cancelled'});assert.equal(calls,1,'cancellation before the microtask never enters the handler');
registration.dispose();registration.dispose();assert.equal(unregistered,1);
await assert.rejects(()=>invoke(request()),{code:'cancelled'});
for(const result of [{media_type:'text/plain',data:new Uint8Array(0)},{media_type:'invalid',data:new Blob(['x'])},{media_type:'text/plain',data:new Blob(['x']),width:1.5},{media_type:'text/plain',data:new Uint8Array(pageActionMaxBytes+1)}]){
 registration=registerActions([{...definition,handler:()=>result}],bridge);
 await assert.rejects(()=>invoke(request()),{code:result.data.byteLength>pageActionMaxBytes?'too_large':'export_failed'});registration.dispose();
}
let finish,signal;
registration=registerActions([{...definition,handler:(_,ctx)=>{signal=ctx.signal;return new Promise(r=>finish=r);}}],bridge);
const pending=invoke(request());await Promise.resolve();registration.dispose();
await assert.rejects(()=>pending,{code:'cancelled'});assert.equal(signal.aborted,true);
finish({media_type:'text/plain',data:new Blob(['late'])});await Promise.resolve();
console.log('PASS page action registration: capability checks, bounds, versions, pre-dispatch cancellation and disposal');
