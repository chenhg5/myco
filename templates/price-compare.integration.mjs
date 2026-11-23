// Built template + real Hub schema, agent token, CAS and changes endpoint.
// Only the native bridge transport is adapted for the browser test.
import assert from "node:assert/strict";
import fs from "node:fs/promises";
import http from "node:http";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { createRequire } from "node:module";
import { execFileSync } from "node:child_process";
const here=path.dirname(fileURLToPath(import.meta.url));
let source="";for await(const c of process.stdin)source+=c;const fixture=JSON.parse(source);
let browser;
try{const require=createRequire(path.join(execFileSync("npm",["root","-g"],{encoding:"utf8"}).trim(),"noop.js"));browser=await require("playwright").chromium.launch({headless:true,args:["--no-sandbox"]});}
catch{console.log("SKIP Playwright Chromium is needed for shopping comparison acceptance");process.exit(0);}
const api=async(route,method="GET",body)=>{const response=await fetch(fixture.url+"/v1/pages/"+fixture.page+route,{method,headers:{Authorization:"Bearer "+fixture.token,"Content-Type":"application/json"},...(body===undefined?{}:{body:JSON.stringify(body)})});const data=response.status===204?null:await response.json();if(!response.ok)throw Object.assign(new Error(data.message||"Hub request failed"),{code:data.error,latest:data.latest});return data;};
const write=(collection,id,data,version=0)=>api("/data/"+collection+"/"+id,"PUT",{data,if_version:version});
const root=path.resolve(here,"../internal/templates/official/price-compare");
const server=http.createServer(async(req,res)=>{const pathname=new URL(req.url,"http://test").pathname;const p=path.resolve(root,"."+(pathname==="/"?"/index.html":pathname));if(!p.startsWith(root+path.sep)){res.writeHead(403);res.end();return;}try{const data=await fs.readFile(p);res.setHeader("Content-Type",({".html":"text/html",".js":"text/javascript",".css":"text/css",".svg":"image/svg+xml"})[path.extname(p)]||"application/octet-stream");res.end(data);}catch{res.writeHead(404);res.end();}});
await new Promise(r=>server.listen(0,"127.0.0.1",r));const context=await browser.newContext({viewport:{width:1200,height:900}});
await context.exposeBinding("comparisonData",async(_,method,c,data,opts)=>{try{return{value:await(method==="list"?api("/data?collection="+encodeURIComponent(c)):method==="changes"?api("/changes?after="+opts.after+"&wait=0"):write(c,opts.id,data,opts.ifVersion))};}catch(e){return{error:{code:e.code,message:e.message,latest:e.latest}};}});
await context.addInitScript(({page})=>{const call=async(...args)=>{const r=await window.comparisonData(...args);if(r.error)throw r.error;return r.value;};let seq=0;window.myco={ready:Promise.resolve({page:{id:page,name:"Shopping acceptance"},me:{name:"comparison-agent"}}),data:{list:async c=>{const r=await call("list",c);seq=Math.max(seq,r.seq);return r;},put:(c,data,o)=>call("put",c,data,o),on:f=>{const t=setInterval(async()=>{const r=await call("changes",null,null,{after:seq});for(const c of r.changes){seq=Math.max(seq,c.seq);f(c);}},100);return()=>clearInterval(t);}}};},{page:fixture.page});
try{
  const sample=JSON.parse(await fs.readFile(path.join(root,"sample.json"),"utf8")).comparisons[0];const{id,...data}=sample;
  // Test data only: page schema is real; no product-price assertion is made.
  const now=new Date().toISOString();data.status="ready";data.updated_at=now;data.title="Real Hub transport acceptance";data.offers=data.offers.map(o=>({...o,checked_at:now}));
  let snapshot=await write("comparisons",id,data);
  const page=await context.newPage(),errors=[];page.on("pageerror",e=>errors.push(e.message));await page.goto(`http://127.0.0.1:${server.address().port}/?myco_lang=en#compare?id=${id}`);
  await page.locator(".pc-best[data-offer=a]").waitFor();await page.locator('[data-offer=a]').getByRole("button",{name:"Save offer",exact:true}).click();
  await page.waitForFunction(()=>document.querySelector('[data-offer=a] button[aria-pressed=true]'));await page.getByLabel("Buying notes").fill("Keep user decisions across refresh");await page.getByRole("button",{name:"Save notes",exact:true}).click();await page.waitForFunction(()=>!document.querySelector('.pc-note .btn'));
  const decision=(await api("/data?collection=decisions")).records[0];assert(decision.data.saved.includes("a"));assert.equal(decision.data.note,"Keep user decisions across refresh");
  snapshot=await write("comparisons",id,{...data,title:"Agent refreshed via real Hub",offers:data.offers.map(o=>({...o,eligibility:"conditional"}))},snapshot.version);
  await page.getByRole("heading",{name:"Agent refreshed via real Hub"}).waitFor();await page.waitForFunction(()=>!document.querySelector('.pc-best'));
  assert.deepEqual((await api("/data?collection=decisions")).records[0].data,decision.data);
  await assert.rejects(write("comparisons",id,data,1),e=>e.code==="conflict");
  const adapter=await import(pathToFileURL(path.join(root,"agent-api.mjs")).href);
  const fakeCredentials={openid:"fixture-own-openid-private",invite_code:"fixture-own-referral"};
  const product={goodsId:"opaque_jd_fixture",sourceType:"2",title:"Direct API pipeline fixture",actualPrice:"99.90",originalPrice:"199.90",couponPrice:"100",shopName:"Test shop"};
  const generated=await adapter.search({query:"Direct API pipeline fixture",spec:"Test only; no real quote",credentials:fakeCredentials,selected:["jd"],linkDelay:0,fetcher:async url=>Response.json({status:"success",code:200,data:url===adapter.endpoints.search?[product]:url===adapter.endpoints.share?{appUrl:"https://shop.example/product/100",kl:"fixture purchase code"}:{}})});
  const apiRecord=await write("comparisons","api-direct",generated.comparison);
  assert.equal(apiRecord.data.offers[0].price,99.9);assert.equal(apiRecord.data.offers[0].provider_ref.provider,"maishou");assert(!JSON.stringify(apiRecord).includes(fakeCredentials.openid));
  await page.evaluate(()=>location.hash="#compare?id=api-direct");await page.getByRole("heading",{name:"Direct API pipeline fixture",level:1}).waitFor();await page.getByRole("button",{name:"Copy purchase code"}).waitFor();assert.equal(await page.locator(".pc-best").count(),0,"An API candidate does not prove exact SKU/shipping/eligibility");assert.equal(await page.getByRole("link",{name:"View product",exact:true}).getAttribute("href"),"https://shop.example/product/100");
  assert.deepEqual(errors,[]);await page.close();console.log("PASS actual Hub: official schema, agent create/update, browser changes feed, shipping-aware winner, conditional exclusion, saved candidates, notes, refresh preservation, stale-version refusal, and built Maishou adapter fixture -> real Hub -> UI with purchase code. Real provider acceptance remains pending owner credentials.");
}finally{await browser.close();await new Promise(r=>server.close(r));}
