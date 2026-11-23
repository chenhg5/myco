// Ranking must not promote mismatched SKUs, hidden conditions or missing data.
import assert from "node:assert/strict";
import fs from "node:fs";
import ts from "typescript";
const source=fs.readFileSync(new URL("src/price-compare/engine.ts",import.meta.url),"utf8");
const e=await import("data:text/javascript;base64,"+Buffer.from(ts.transpileModule(source,{compilerOptions:{target:99,module:99}}).outputText).toString("base64"));
const sample=JSON.parse(fs.readFileSync(new URL("src/price-compare/public/sample.json",import.meta.url),"utf8")).comparisons[0];
assert(e.validComparison(sample));
const now=Date.parse("2026-11-23T02:00:00Z");
const c={...sample,status:"ready"};
const o=c.offers[0];
assert.equal(e.ranked(c,now)[0].id,"a");
assert.equal(e.ranked(c,now).length,1,"Member price, unknown shipping and refurbished variant cannot win");
for(const[patch,why]of [
 [{currency:"USD"},"currency"],[{price:null},"price"],[{shipping:null},"shipping"],
 [{match:"uncertain"},"match"],[{spec:" "},"match"],[{availability:"unknown"},"stock"],
 [{availability:"unavailable"},"stock"],[{eligibility:"conditional"},"conditions"],[{eligibility:"unknown"},"conditions"],
 [{checked_at:"2026-11-21T01:00:00Z"},"stale"],[{checked_at:"2026-11-24T01:00:00Z"},"future"],
 [{expires_at:"2026-11-23T01:30:00Z"},"expired"]
])assert.equal(e.exclusion({...o,...patch},c,now),why);
assert.equal(e.ranked(sample,now).length,0,"Fictional examples never enter ranking");
const shipping={...o,id:"shipping",price:1880,shipping:30};
assert.equal(e.ranked({...c,offers:[shipping,o]},now)[0].id,"a","Sort by payable total, not headline price");
assert.equal(e.total({...o,price:0.1,shipping:0.2}),0.3);
assert.equal(e.total({...o,shipping:null}),null);
assert.equal(e.total({...o,cashback:"1000 yuan"}),1899,"Cashback is never silently subtracted");
for(const patch of [{price:-1},{price:"1"},{price:Infinity},{price:NaN},{price:1.001},{url:"javascript:alert(1)"},{url:"data:text/html,hi"},{url:"https://u:password@shop.example"},{checked_at:"2026-11-23"},{affiliate:"false"},{source:""},{shipping:-1}])assert.equal(e.validOffer({...o,...patch}),false,JSON.stringify(patch));
assert.equal(e.validComparison({...c,offers:[o,o]}),false,"Offer IDs must stay unique");
assert.equal(e.validComparison({...c,offers:[null]}),false);
assert.equal(e.validComparison({...c,offers:"[]"}),false);
assert.equal(e.validComparison({...c,currency:null}),false);
assert.equal(e.validComparison({...c,status:"success"}),false);
assert.equal(e.validComparison({...c,budget:-100}),false);
assert.equal(e.validComparison({...c,offers:Array.from({length:41},(_,i)=>({...o,id:String(i)}))}),false);
assert.equal(e.safeURL("https://shop.example/item?id=10"),"https://shop.example/item?id=10");
assert.equal(e.routeID("#compare?id=headphones"),"headphones");
for(const h of ["#compare?id=%","#compare?id=abc%2Fdef","#compare?id=a&url=evil","#watch?id=a","#compare?id="+"x".repeat(101)])assert.equal(e.routeID(h),"");
const prompt=e.promptFor("pg_test","headphones","black; 1 pair","headphones");
assert(prompt.includes("pg_test")&&prompt.includes("Preserve user decisions")&&prompt.includes("status=blocked"));
console.log("PASS shopping comparisons: exact SKU, conditional eligibility, currency, unknown shipping/price, stock, observation age, expiry, sample exclusion, rounding, safe links, malformed agent data and request contract");
