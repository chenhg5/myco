#!/usr/bin/env node
// App-owned Maishou HTTP client for agents (Node 20+), never loaded by the UI.
// Protocol reference: SkillHub @user_48ebdf60/findbestprice 1.0.3. No bundled
// identity, referral code or unverified API-key authentication is copied.
import { readFile, writeFile, stat } from "node:fs/promises";
import { spawnSync } from "node:child_process";
import { pathToFileURL } from "node:url";
import { setTimeout as pause } from "node:timers/promises";

export class APIError extends Error {
  constructor(code, message, retryable = false) { super(message); this.code = code; this.retryable = retryable; }
}
const fail = (code, message, retryable) => { throw new APIError(code, message, retryable); };
export const platforms = Object.freeze({
  all:{name:"全部",source:"0"},taobao:{name:"淘宝/天猫",source:"1"},jd:{name:"京东",source:"2"},
  pdd:{name:"拼多多",source:"3"},suning:{name:"苏宁",source:"4"},vip:{name:"唯品会",source:"5"},douyin:{name:"抖音",source:"7"},kuaishou:{name:"快手",source:"8"},
});
const names = Object.fromEntries(Object.values(platforms).map(p=>[p.source,p.name]));
export const endpoints = Object.freeze({
  search:"https://appapi.maishou88.com/api/v1/homepage/searchList",
  detail:"https://appapi.maishou88.com/api/v3/goods/detail",
  share:"https://msapi.maishou88.com/api/v1/share/getTargetUrl",
});
const text = (v, limit = 1000) => typeof v === "string" || typeof v === "number" ? String(v).slice(0,limit) : "";
export function amount(value) {
  if (value === null || value === undefined || value === "" || typeof value === "boolean") return null;
  if (typeof value !== "number" && (typeof value !== "string" || !/^\d+(?:\.\d{1,2})?$/.test(value.trim()))) return null;
  const n = Number(value);
  return Number.isFinite(n) && n >= 0 && n <= 1e9 && Math.abs(n * 100 - Math.round(n * 100)) < .0001 ? n : null;
}
export function credentialsValid(c) {
  if (!c || typeof c.openid !== "string" || !c.openid.trim() || c.openid.length > 512 || /[\r\n]/.test(c.openid)) fail("credentials_required","Configure your own MAISHOU_OPENID or a private --config-file. The reference protocol uses OpenID; no standard API-key field is documented.");
  for (const k of ["invite_code","token"]) if (c[k] !== undefined && (typeof c[k] !== "string" || c[k].length > 4096 || /[\r\n]/.test(c[k]))) fail("credentials_required","Provider credentials have an invalid format.");
  return c;
}
export function searchRequest(platform, query, credentials, page = 1) {
  credentialsValid(credentials);
  if (!platforms[platform] || typeof query !== "string" || !query.trim() || query.length > 200 || !Number.isInteger(page) || page < 1 || page > 3) fail("invalid_argument","Provide a query of 1–200 characters, a supported platform and page 1–3.");
  return {url:endpoints.search,body:new URLSearchParams({isCoupon:"0",keyword:query.trim(),openid:credentials.openid,order:"desc",page:String(page),pddListId:"",sort:"",sourceType:platforms[platform].source,user_id:""}).toString(),json:false};
}
function businessError(code, message) {
  // Classify known errors, but never echo the raw message: it can contain keys.
  if (Number(code)===429) return new APIError("rate_limited","Maishou rate limit prevents this request. Check the provider account.",true);
  if ([401,403].includes(Number(code)) || /openid|apikey|token|密钥|签名|授权|登录|权限|认证|invalid.*key/i.test(message)) return new APIError("permission_denied","Maishou rejected this identity or platform authorization. Check your own account and supplier-issued credentials.");
  if (/频|限流|次数|配额|quota|rate|余额|扣费|付费/i.test(message)) return new APIError("rate_limited","Maishou quota, rate limit or account plan prevents this request. Check the provider account.",true);
  if (/超时|timeout|繁忙|稍后/i.test(message)) return new APIError("unavailable","Maishou reports a temporary failure. No search-engine fallback was used.",true);
  return new APIError("provider_error",`Maishou rejected the operation (business status ${/^-?\d{1,6}$/.test(String(code))?code:"unknown"}). No empty result was inferred.`);
}
export function decode(payload, kind="search") {
  if (!payload || typeof payload!=="object" || Array.isArray(payload)) fail("invalid_response","Maishou returned an unexpected response shape.");
  // Real anonymous responses use {status:"error",code:401|201,...}. A real
  // authenticated success still needs acceptance with the owner's credential.
  if (payload.status!=="success" || Number(payload.code)!==200) throw businessError(payload.code,typeof payload.message==="string"?payload.message:"");
  if (kind==="search" ? !Array.isArray(payload.data) : !payload.data || typeof payload.data!=="object" || Array.isArray(payload.data)) fail("invalid_response","A successful Maishou response had an unexpected data shape.");
  return payload.data;
}
async function readJSON(response) {
  if ([401,403].includes(response.status)) fail("permission_denied","Maishou rejected authentication or access.");
  if (response.status===429) fail("rate_limited","Maishou rate limit reached.",true);
  if (!response.ok) fail("unavailable","Maishou HTTP request failed.",true);
  const reader=response.body?.getReader();if(!reader)fail("invalid_response","Maishou response was empty.");
  let size=0;const chunks=[];
  while(true){const{done,value}=await reader.read();if(done)break;size+=value.byteLength;if(size>2*1024*1024){await reader.cancel();fail("too_large","Maishou response exceeded 2 MiB.");}chunks.push(value);}
  const bytes=new Uint8Array(size);let at=0;for(const c of chunks){bytes.set(c,at);at+=c.byteLength;}
  try{return JSON.parse(new TextDecoder().decode(bytes).replace(/^\uFEFF/,""));}catch{fail("invalid_response","Maishou response was not valid JSON.");}
}
export async function requestAPI(request, credentials, {fetcher=fetch,signal}={}) {
  credentialsValid(credentials);
  if(!Object.values(endpoints).includes(request.url))fail("invalid_argument","Unknown Maishou endpoint.");
  const timeout=AbortSignal.timeout(15000);
  try{
    const response=await fetcher(request.url,{method:"POST",redirect:"error",signal:signal?AbortSignal.any([signal,timeout]):timeout,headers:{Accept:"application/json","Content-Type":request.json?"application/json":"application/x-www-form-urlencoded",openid:credentials.openid,version:"3.7.7.2","User-Agent":"myco-price-compare/1.1"},body:request.json?JSON.stringify(request.body):request.body});
    return decode(await readJSON(response),request.url===endpoints.search?"search":"object");
  }catch(e){if(e instanceof APIError)throw e;if(signal?.aborted)fail("cancelled","Query was cancelled.");if(timeout.aborted)fail("timeout","Maishou operation timed out.",true);fail("unavailable","Could not reach Maishou securely. No credentials were sent to a redirected host.",true);}
}
export function safeProductURL(value, credentials={}) {
  if(typeof value!=="string"||value.length>2048)return "";
  try{const u=new URL(value);if(!["http:","https:"].includes(u.protocol)||u.username||u.password)return "";
    if([...u.searchParams.keys()].some(k=>/^(?:openid|api_?key|access_token|authorization|user_token)$/i.test(k)))return "";
    const decoded=decodeURIComponent(value);
    if([credentials.openid,credentials.token].some(v=>typeof v==="string"&&v.length>=6&&(value.includes(v)||decoded.includes(v))))return "";
    return u.href;
  }catch{return "";}
}
export function normalize(item, checkedAt, requested="all") {
  if(!item||typeof item!=="object"||Array.isArray(item))return null;
  const rawID=item.goodsId;if(typeof rawID==="number"&&!Number.isSafeInteger(rawID))return null;
  const goodsID=text(rawID,200),source=text(item.sourceType,5),title=text(item.title,300);
  if(!goodsID||!title||!names[source]||source==="0"||(requested!=="all"&&source!==platforms[requested].source))return null;
  const id=`ms-${source}-${Buffer.from(goodsID).toString("base64url")}`;if(id.length>100)return null;
  // goodsId is a provider reference, not a proven public marketplace SKU. Only
  // the detail/share flow may supply a purchase link; never invent one here.
  const url="https://apps.apple.com/cn/app/id1310256718",link_kind="evidence";
  const coupon=amount(item.couponPrice);
  return {id,title,platform:names[source],shop:text(item.shopName,200),spec:"",currency:"CNY",price:amount(item.actualPrice),shipping:null,match:"uncertain",availability:"unknown",eligibility:"unknown",
    conditions:"买手 API 商品级券后报价，尚未核对最低价对应的 SKU、运费、库存和领取资格；平台入口不代表该条价格的公开证据。",
    coupon:coupon!==null&&coupon>0?`API 券面额 ¥${coupon}；券后价由接口给出，不重复扣减；领取条件待确认。`:"",cashback:"",source:`买手 API · ${names[source]}`,url,link_kind,checked_at:checkedAt,expires_at:"",affiliate:false,
    provider_ref:{provider:"maishou",goods_id:goodsID,source_type:source}};
}
export async function purchaseLink(offer, credentials, options={}) {
  if(!credentials.invite_code)return {state:"not_configured",offer};
  const args={goodsId:offer.provider_ref.goods_id,sourceType:offer.provider_ref.source_type,inviteCode:credentials.invite_code,supplierCode:"",activityId:"",isShare:"1",token:credentials.token||""};
  await requestAPI({url:endpoints.detail,json:true,body:{...args,keyword:"",usageScene:5}},credentials,options);
  const data=await requestAPI({url:endpoints.share,json:true,body:{...args,isDirectDetail:0}},credentials,options);
  const url=safeProductURL(data.appUrl,credentials)||safeProductURL(data.schemaUrl,credentials);
  const purchase_code=typeof data.kl==="string"?data.kl.slice(0,2000):"";
  if(!url&&!purchase_code)fail("link_unavailable","Maishou did not return a safe public purchase URL or purchase code.");
  return {state:"ready",offer:{...offer,...(url?{url,link_kind:"product"}:{}),...(purchase_code?{purchase_code}:{}),affiliate:true}};
}
function publicError(error){return {code:error instanceof APIError?error.code:"unavailable",message:error instanceof APIError?error.message:"Maishou operation failed.",retryable:!!error.retryable};}
export async function search({query,spec="",credentials,selected=["taobao","jd","pdd"],pages=1,links=3,fetcher=fetch,signal,now=()=>new Date().toISOString(),linkDelay=1000}) {
  if(!Array.isArray(selected)||!selected.length||new Set(selected).size!==selected.length||selected.some(p=>!platforms[p])||(selected.includes("all")&&selected.length!==1))fail("invalid_argument","Select unique supported platforms, or all alone.");
  if(!Number.isInteger(pages)||pages<1||pages>3||!Number.isInteger(links)||links<0||links>10)fail("invalid_argument","Pages must be 1–3 and purchase links must be 0–10.");
  searchRequest(selected[0],query,credentials);const found=[],seen=new Set(),coverage=[];
  for(const platform of selected){const part={platform,pages:0,offers:0,dropped:0,error:null};
    try{for(let page=1;page<=pages;page++){if(signal?.aborted)fail("cancelled","Query was cancelled.");const data=await requestAPI(searchRequest(platform,query,credentials,page),credentials,{fetcher,signal});part.pages++;let added=0;
      for(const item of data){const o=normalize(item,now(),platform);if(!o){part.dropped++;continue;}if(!seen.has(o.id)){seen.add(o.id);found.push(o);part.offers++;added++;}}
      if(!data.length||!added)break;
    }}catch(error){if(error.code==="cancelled")throw error;part.error=publicError(error);}coverage.push(part);
  }
  const groups=selected.includes("all")?[found]:selected.map(p=>found.filter(o=>o.platform===platforms[p].name)),offers=[];
  for(let i=0;offers.length<Math.min(40,found.length);i++)for(const g of groups)if(g[i]&&offers.length<40)offers.push(g[i]);
  const linkResults=[];
  if(credentials.invite_code){const picks=[...offers].filter(o=>o.price!==null).sort((a,b)=>a.price-b.price).slice(0,links);
    for(const offer of picks){if(signal?.aborted)fail("cancelled","Query was cancelled.");try{const r=await purchaseLink(offer,credentials,{fetcher,signal});offers[offers.findIndex(o=>o.id===offer.id)]=r.offer;linkResults.push({id:offer.id,state:r.state});}catch(error){if(error.code==="cancelled")throw error;linkResults.push({id:offer.id,state:"failed",error:publicError(error)});}if(linkDelay>0)await pause(linkDelay,undefined,{signal}).catch(()=>fail("cancelled","Query was cancelled."));}
  }
  const summary=["来源：买手商品 API；未使用网页搜索补齐结果。",...coverage.map(c=>`${platforms[c.platform].name}：${c.pages} 页，${c.offers} 条；${c.error?c.error.code+"（未完整查询）":"查询完成"}${c.dropped?`，丢弃 ${c.dropped} 条格式不完整数据`:""}。`),credentials.invite_code?`购买链接：${linkResults.filter(x=>x.state==="ready").length} 条已生成，${linkResults.filter(x=>x.state==="failed").length} 条失败；使用当前配置的邀请码归属。`:"未配置自己的邀请码，未请求推广链接；不使用第三方默认邀请码。",`${found.length>offers.length?`共返回 ${found.length} 条，页面保留各平台合计 ${offers.length} 条；`:""}仅限本次查询候选，不是全网最低。规格、运费、库存和资格尚需确认；API 查询时间不保证源站价格的更新时间。`].join("\n");
  const result={comparison:{title:text(query,200),query:query.trim(),spec:text(spec,1000),currency:"CNY",budget:0,status:offers.length?"ready":"blocked",summary,updated_at:now(),offers},coverage,links:linkResults};
  const serialized=JSON.stringify(result);
  if([credentials.openid,credentials.token].some(v=>typeof v==="string"&&v.length>=6&&[v,JSON.stringify(v).slice(1,-1),encodeURIComponent(v)].some(encoded=>serialized.includes(encoded))))fail("invalid_response","Provider data unexpectedly contained a credential; nothing was published.");
  return result;
}
async function credential(file){let c={};if(file){const info=await stat(file).catch(()=>fail("credentials_required","The private configuration file could not be read."));if(process.platform!=="win32"&&(info.mode&0o077))fail("permission_denied","Configuration file must be private (chmod 600).");if(info.size>16384)fail("invalid_argument","Configuration file is too large.");try{c=JSON.parse(await readFile(file,"utf8"));}catch{fail("credentials_required","Configuration file must be valid JSON.");}if(!c||typeof c!=="object"||Array.isArray(c))fail("credentials_required","Configuration must be an object.");}
  return credentialsValid({openid:c.openid??process.env.MAISHOU_OPENID,invite_code:c.invite_code??process.env.MAISHOU_INVITE_CODE??"",token:c.token??process.env.MAISHOU_TOKEN??""});}
function myco(args,input){const r=spawnSync(process.platform==="win32"?"myco.exe":"myco",args,{input,encoding:"utf8",maxBuffer:1024*1024});if(r.status!==0)fail("page_write_failed","myco rejected the operation. Check page access, template version and CAS version; no automatic retry was performed.");try{return JSON.parse(r.stdout);}catch{fail("page_write_failed","myco returned an unexpected response.");}}
export async function main(argv=process.argv.slice(2)) {
  if(!argv.length||argv.includes("--help")){console.log("Node 20+: node agent-api.mjs --query MODEL --spec REQUIREMENTS [--platforms taobao,jd,pdd|all|suning,vip,douyin,kuaishou] [--pages 1..3] [--links 0..10] [--config-file PRIVATE_JSON] [--out comparison.json] [--page pg_ID --id comparison_ID]\nCredentials: your own MAISHOU_OPENID, optional MAISHOU_INVITE_CODE and MAISHOU_TOKEN, or private JSON {openid,invite_code,token}. No bundled identity, referral or generic API-key fallback. --page publishes through the existing myco CLI identity with CAS.");return;}
  const o={},allowed=new Set(["query","spec","platforms","pages","links","config-file","out","page","id"]);for(let i=0;i<argv.length;i+=2){const k=argv[i]?.replace(/^--/,"");if(!argv[i]?.startsWith("--")||!allowed.has(k)||o[k]!==undefined||!argv[i+1]||argv[i+1].startsWith("--"))fail("invalid_argument","Unknown, duplicate or incomplete option. Run --help.");o[k]=argv[i+1];}
  if((o.page&&(!/^pg_[a-z0-9]+$/.test(o.page)||!/^[\w-]{1,100}$/.test(o.id||"")))||(o.id&&!o.page))fail("invalid_argument","Publishing needs --page pg_ID and --id comparison_ID.");
  const credentials=await credential(o["config-file"]);
  const before=o.page?myco(["page","data","list","--page",o.page,"--collection","comparisons","--format","json"]).records.find(r=>r.id===o.id):undefined;
  const controller=new AbortController(),interrupt=()=>controller.abort();process.once("SIGINT",interrupt);
  try{const r=await search({query:o.query,spec:o.spec,credentials,selected:o.platforms?.split(","),pages:Number(o.pages||1),links:Number(o.links??3),signal:controller.signal});if(controller.signal.aborted)fail("cancelled","Query was cancelled before publishing.");
    const comparison={...(before?.data||{}),...r.comparison},json=JSON.stringify(comparison,null,2);if(Buffer.byteLength(json,"utf8")>58000)fail("too_large","Comparison exceeds 58 KB. Narrow the query.");if(o.out)await writeFile(o.out,json+"\n");let record;
    if(o.page)record=myco(["page","data","put","--page",o.page,"--collection","comparisons","--id",o.id,"--file","-","--if-version",String(before?.version||0),"--format","json"],json);
    console.log(JSON.stringify({provider:"maishou",status:comparison.status,offers:comparison.offers.length,coverage:r.coverage,links:r.links,...(o.out?{file:o.out}:{}),...(record?{page:o.page,comparison:o.id}:{}),...(!o.page&&!o.out?{comparison}:{})},null,2));if(r.coverage.some(c=>c.error)||r.links.some(c=>c.error))process.exitCode=6;
  }finally{process.removeListener("SIGINT",interrupt);}
}
if(process.argv[1]&&import.meta.url===pathToFileURL(process.argv[1]).href)main().catch(e=>{console.error(JSON.stringify({error:e instanceof APIError?e.code:"failed",message:e instanceof APIError?e.message:"The API operation failed. No credentials or provider body were logged.",retryable:!!e.retryable}));process.exitCode=e instanceof APIError&&e.code==="credentials_required"?4:1;});
