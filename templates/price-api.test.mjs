// Maishou request contract from the selected Skill; successful responses below
// are fixtures, not a claim of authenticated live acceptance. Anonymous 401
// envelopes were checked against the real service on 2026-11-23.
import assert from "node:assert/strict";
import http from "node:http";
import { spawnSync } from "node:child_process";
import { searchRequest, decode, amount, normalize, search, requestAPI, purchaseLink, safeProductURL, endpoints } from "./src/price-compare/public/agent-api.mjs";
const credentials={openid:"own-fixture-openid-private",invite_code:"own-fixture-invite",token:"own-fixture-token-private"},query="充电宝 + 10000mAh/USB-C";
const packet=searchRequest("jd",query,credentials,2),form=new URLSearchParams(packet.body);
assert.equal(packet.url,endpoints.search);assert.equal(form.get("keyword"),query);assert.equal(form.get("sourceType"),"2");assert.equal(form.get("page"),"2");assert.equal(form.get("openid"),credentials.openid);
assert.equal(form.get("apikey"),null,"Do not invent a generic API-key parameter");
for(const v of[null,undefined,"",true,"abc",-1,"-1",Infinity,"1.234"])assert.equal(amount(v),null);
assert.equal(amount("0"),0);assert.equal(amount("89.90"),89.9);
for(const[payload,code]of[
 [{status:"error",code:401,message:"用户未登录",data:null},"permission_denied"],
 [{status:"error",code:201,message:"淘宝未授权",data:null},"permission_denied"],
 [{status:"error",code:429,message:"限流 "+credentials.openid},"rate_limited"],
 [{status:"error",code:500,message:"网络超时"},"unavailable"],
 [{code:200,data:[]},"provider_error"],
 [{status:"error",code:200,data:[]},"provider_error"],
 [{status:"success",code:200,data:{}},"invalid_response"],
])assert.throws(()=>decode(payload),e=>e.code===code&&!e.message.includes(credentials.openid));
assert.deepEqual(decode({status:"success",code:200,data:[]}),[]);
const item={goodsId:"opaque-product-id",sourceType:"1",title:"Power bank",shopName:"Shop",actualPrice:"99.90",originalPrice:"199.90",couponPrice:"100",monthSales:50};
const time="2026-11-23T02:00:00Z",o=normalize(item,time,"taobao");assert.equal(o.price,99.9);assert.equal(o.shipping,null);assert.equal(o.match,"uncertain");assert.equal(o.eligibility,"unknown");assert.equal(o.availability,"unknown");assert.equal(o.link_kind,"evidence");assert.equal(o.affiliate,false);
assert.equal(normalize({...item,actualPrice:undefined},time).price,null,"Do not replace missing coupon price with original price");
assert.equal(normalize({...item,sourceType:"2",goodsId:"12345678901"},time,"jd").link_kind,"evidence","Numeric provider references must not become invented marketplace links");
assert.equal(normalize(item,time,"jd"),null,"Do not attribute a different platform to the requested one");
assert.equal(normalize({...item,goodsId:9007199254740994},time),null);
for(const url of["javascript:alert(1)","taobao://item/1","https://user:pass@shop.example/","https://shop.example/?apikey=leaked",`https://shop.example/?k=${credentials.token}`])assert.equal(safeProductURL(url,credentials),"");
assert.equal(safeProductURL("https://shop.example/item?id=100",credentials),"https://shop.example/item?id=100");
assert.equal(safeProductURL("https://shop.example/?share=secret%2Ftoken%2Bvalue",{token:"secret/token+value"}),"","Encoded credentials must not leak in shared links");
const calls=[];const fetcher=async(url,options)=>{calls.push({url,options});if(url===endpoints.search){const f=new URLSearchParams(options.body),source=f.get("sourceType");return Response.json(source==="2"?{status:"error",code:401,message:"openid invalid "+credentials.openid}:{status:"success",code:200,data:Array.from({length:20},(_,i)=>({...item,sourceType:source,goodsId:source+"_"+i}))});}if(url===endpoints.detail)return Response.json({status:"success",code:200,data:{}});return Response.json({status:"success",code:200,data:{appUrl:"https://shop.example/buy?id=1",kl:"copyable purchase code"}});};
const r=await search({query,credentials,selected:["taobao","jd","pdd"],pages:2,links:2,linkDelay:0,fetcher,now:()=>time});
assert.equal(r.comparison.offers.length,40);assert.equal(r.coverage.find(c=>c.platform==="jd").error.code,"permission_denied");assert(r.comparison.summary.includes("未完整查询"));assert.equal(calls.filter(x=>x.url===endpoints.detail).length,2);assert.equal(calls.filter(x=>x.url===endpoints.share).length,2);assert.equal(new Set(r.comparison.offers.map(o=>o.id)).size,40);
const share=JSON.parse(calls.find(x=>x.url===endpoints.share).options.body);assert.equal(share.inviteCode,credentials.invite_code);assert.equal(share.token,credentials.token);assert.equal(share.isDirectDetail,0);
assert.equal(calls[0].options.headers.openid,credentials.openid);assert.equal(calls[0].options.method,"POST");assert.equal(calls[0].options.redirect,"error");
assert(!JSON.stringify(r).includes(credentials.openid));assert(!JSON.stringify(r).includes(credentials.token));assert.equal(r.links.filter(x=>x.state==="ready").length,2);assert.equal(r.comparison.offers.filter(x=>x.affiliate).length,2);
const count=calls.length;assert.equal((await purchaseLink(o,{openid:credentials.openid},{fetcher})).state,"not_configured");assert.equal(calls.length,count,"No default invite or unsolicited referral request");
const tokenLink=await purchaseLink(o,credentials,{fetcher:async url=>Response.json({status:"success",code:200,data:url===endpoints.share?{schemaUrl:"taobao://item/1",kl:"purchase code only"}:{}})});assert.equal(tokenLink.offer.purchase_code,"purchase code only");assert.equal(tokenLink.offer.link_kind,"evidence");
let called=false;await assert.rejects(search({query,credentials:{},fetcher:async()=>{called=true}}),e=>e.code==="credentials_required");assert.equal(called,false);
await assert.rejects(requestAPI(packet,credentials,{fetcher:async()=>{throw Error(credentials.token)}}),e=>e.code==="unavailable"&&!e.message.includes(credentials.token));
const c=new AbortController();c.abort();await assert.rejects(search({query,credentials,signal:c.signal}),e=>e.code==="cancelled");
await assert.rejects(search({query,credentials,selected:["all","jd"]}),e=>e.code==="invalid_argument");
const leaked=async()=>Response.json({status:"success",code:200,data:[{...item,title:credentials.token}]});await assert.rejects(search({query,credentials,selected:["taobao"],links:0,fetcher:leaked}),e=>e.code==="invalid_response");
let mode="success";const server=http.createServer((req,res)=>{if(mode==="oversize"){res.end("x".repeat(2*1024*1024+1));return;}if(mode==="html"){res.end("<html>login</html>");return;}res.setHeader("Content-Type","application/json");res.end(JSON.stringify({status:"success",code:200,data:[item]}));});await new Promise(r=>server.listen(0,"127.0.0.1",r));const local=()=>fetch(`http://127.0.0.1:${server.address().port}`);
try{assert.equal((await requestAPI(packet,credentials,{fetcher:local})).length,1);mode="html";await assert.rejects(requestAPI(packet,credentials,{fetcher:local}),e=>e.code==="invalid_response");mode="oversize";await assert.rejects(requestAPI(packet,credentials,{fetcher:local}),e=>e.code==="too_large");}finally{await new Promise(r=>server.close(r));}
const child=spawnSync(process.execPath,[new URL("src/price-compare/public/agent-api.mjs",import.meta.url).pathname,"--query","powerbank"],{encoding:"utf8",env:{...process.env,MAISHOU_OPENID:"",MAISHOU_TOKEN:"",MAISHOU_INVITE_CODE:""}});assert.equal(child.status,4);assert.equal(JSON.parse(child.stderr).error,"credentials_required");assert.equal(child.stdout,"");
console.log("PASS Maishou adapter: reference form/headers, own OpenID/referral/token only, no invented API-key field, real anonymous error envelopes, strict success fixtures, bounded pagination, deduplication, partial platform failures, detail/share links and purchase codes, no third-party fallback, unknown SKU/stock/shipping, credential redaction, cancellation, real HTTP JSON/size handling and missing-credential CLI exit. Live authenticated acceptance remains pending.");
