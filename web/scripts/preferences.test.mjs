import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { transform } from 'esbuild';
const source = await readFile(new URL('../src/lib/localPreferences.ts', import.meta.url), 'utf8');
const { code } = await transform(source, { loader: 'ts', format: 'esm' });
let generation = 0;
async function client(initial = {}, storageFails = false) {
  const entries = new Map(Object.entries(initial));
  globalThis.window = new EventTarget();
  globalThis.localStorage = {
    get length() { return entries.size; }, key(i) { return [...entries.keys()][i] ?? null; },
    getItem(k) { return entries.get(k) ?? null; },
    setItem(k,v) { if (storageFails) throw new DOMException('full', 'QuotaExceededError'); entries.set(k,v); },
    removeItem(k) { entries.delete(k); },
  };
  return import(`data:text/javascript;base64,${Buffer.from(code + `\n// instance ${generation++}`).toString('base64')}`);
}
let disk = {}, imports = 0, unavailable = false, rejectSave = false;
globalThis.fetch = async (url, opts = {}) => {
  if (unavailable) return new Response('{}',{status:500});
  const method = opts.method || 'GET';
  if (method !== 'GET') {
    if (rejectSave) return new Response('{}',{status:500});
    const body = JSON.parse(opts.body);
    if (url.endsWith('/import')) {
      imports++;
      for (const [k,v] of Object.entries(body.values)) if (!Object.hasOwn(disk,k)) disk[k]=v;
    } else if (url.includes('/hubs/')) {
      const names=JSON.parse(disk['myco-app-hub-names']||'{}'); names[url.split('/').at(-1)] = body.name;
      disk['myco-app-hub-names'] = JSON.stringify(names);
      disk['myco-app-hub-looks'] = JSON.stringify({demo:body.look});
    } else Object.assign(disk,body.values);
  }
  return Response.json({version:1,values:disk});
};
// Only allowlisted legacy choices migrate. Durable data wins over a stale origin.
let c=await client({'myco-theme':'dark','provider-key':'private','myco-app-sidebar':'37','myco-usage-days':'30'});
await c.initializePreferences();
assert.deepEqual(disk,{'myco-theme':'dark','myco-usage-days':'30'}); assert.equal(imports,1);
c.localPreferences.removeItem('myco-usage-days'); await c.flushPreferences();
assert.equal(disk['myco-usage-days'],null);
// Upgrade: a new browser origin has no localStorage, and its cache may be full.
c=await client({},true); await c.initializePreferences();
assert.equal(c.localPreferences.getItem('myco-theme'),'dark');
await c.saveWorkspaceAppearance('demo','Demo',{color:2,pic:'/app/preferences/images/'+'a'.repeat(64)+'.png'});
assert.equal(JSON.parse(disk['myco-app-hub-names']).demo,'Demo');
assert.equal(c.preferencesProblem(),false,'browser quota cannot fail a committed save');
c=await client({'myco-theme':'light','myco-usage-days':'90'}); await c.initializePreferences();
assert.equal(c.localPreferences.getItem('myco-theme'),'dark'); assert.equal(c.localPreferences.getItem('myco-usage-days'),null);
// Failed explicit saves are observable and leave the last saved avatar/name intact.
const before=JSON.stringify(disk); rejectSave=true;
await assert.rejects(c.saveWorkspaceAppearance('demo','Wrong',{color:3}));
assert.equal(JSON.stringify(disk),before); rejectSave=false;
// A failed initial read never writes defaults. Retry imports still-unmigrated data.
unavailable=true; c=await client({'myco-lang':'en'}); await c.initializePreferences();
assert.equal(c.preferencesProblem(),true); assert.equal(disk['myco-lang'],undefined);
unavailable=false; await c.flushPreferences(); assert.equal(disk['myco-lang'],'en');
assert.equal(c.preferencesProblem(),false);
// A delayed older write cannot erase a newer choice in the same client.
let release, first=true; const serve=globalThis.fetch;
globalThis.fetch=async(url,opts)=>{
 if(opts?.method==='PATCH' && first){first=false; await new Promise(r=>release=r);}
 return serve(url,opts);
};
c.localPreferences.setItem('myco-theme','light'); const saving=c.flushPreferences();
await new Promise(r=>setTimeout(r,0)); c.localPreferences.setItem('myco-theme','dark'); release(); await saving;
assert.equal(disk['myco-theme'],'dark');
// Compatibility: standalone web / old backends keep their existing storage path.
globalThis.fetch=async()=>new Response('{}',{status:404}); c=await client({'myco-lang':'zh'});
await c.initializePreferences(); c.localPreferences.setItem('myco-lang','en'); assert.equal(c.localPreferences.getItem('myco-lang'),'en');
globalThis.fetch=async()=>new Response('<!doctype html>',{headers:{'Content-Type':'text/html'}}); c=await client({'myco-theme':'dark'});
await c.initializePreferences(); assert.equal(c.preferencesProblem(),false); assert.equal(c.localPreferences.getItem('myco-theme'),'dark');
// An unavailable legacy background must not block an unrelated avatar/name.
disk={}; globalThis.fetch=serve;
globalThis.indexedDB={open(){const request={};setTimeout(()=>request.onerror?.(),0);return request;}};
c=await client({'myco-lang':'en','myco-chat-bg':JSON.stringify({def:{image:'missing-old-file'},by:{}})});
await c.initializePreferences(); assert.equal(disk['myco-lang'],'en'); assert.equal(disk['myco-chat-bg'],undefined);
assert.equal(c.preferencesProblem(),true);
await c.saveWorkspaceAppearance('demo','Still saveable',{color:3});
assert.equal(JSON.parse(disk['myco-app-hub-names']).demo,'Still saveable');
// Existing IndexedDB pictures are uploaded before their metadata is imported.
disk={}; let closed=0;
const picture = new Blob([new Uint8Array([137,80,78,71,13,10,26,10])],{type:'image/png'});
globalThis.indexedDB = {
  open() {
    const request = {};
    setTimeout(() => {
      request.result = {
        objectStoreNames: { contains: () => true },
        close() { closed++; },
        transaction() {
          const tx = {
            objectStore() {
              return {
                get() {
                  const read = {};
                  setTimeout(() => { read.result = picture; read.onsuccess?.(); tx.oncomplete?.(); }, 0);
                  return read;
                },
              };
            },
          };
          return tx;
        },
      };
      request.onsuccess?.();
    }, 0);
    return request;
  },
};
const pictureURL='/app/preferences/images/'+'b'.repeat(64)+'.png';
globalThis.fetch=async(url,opts)=>url==='/app/preferences/images' ? (assert.ok(opts.body instanceof Blob),Response.json({url:pictureURL})) : serve(url,opts);
c=await client({'myco-chat-bg':JSON.stringify({def:{image:'old-file'},by:{}})});
await c.initializePreferences(); assert.equal(JSON.parse(disk['myco-chat-bg']).def.image,pictureURL);
assert.equal(c.preferencesProblem(),false); assert.equal(closed,1);
console.log('preferences: migration, restart/new origin, quota, failed save, recovery, write ordering and old backend passed');
