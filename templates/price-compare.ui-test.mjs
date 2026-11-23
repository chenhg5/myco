// Production-bundle interactions: agent updates, eligibility, human choices,
// requests, exports, stale notes, malformed data, routes and real touch scroll.
import assert from "node:assert/strict";
import fs from "node:fs";
import { startFixture } from "./price-compare-fixture.mjs";
const out=process.env.PRICE_COMPARE_SHOTS||"/root/code/myco-artifacts/price-compare/screens";fs.mkdirSync(out,{recursive:true});
const f=await startFixture(),{browser,server,reset,recs,writes,write,base}=f;
try {
  for(const[lang,theme,width]of[["zh","light",1400],["en","dark",1400],["zh","dark",760],["en","light",393]]){
    reset();const context=await browser.newContext({viewport:{width,height:1000},colorScheme:theme,isMobile:width===393,hasTouch:width===393}),p=await context.newPage(),errors=[];
    p.on("pageerror",e=>errors.push(e.message));p.setDefaultTimeout(10000);const L=(zh,en)=>lang==="zh"?zh:en;
    await p.goto(base+"?myco_lang="+lang);await p.locator(".pc-offer").nth(3).waitFor();
    assert.equal(await p.locator(".pc-best").count(),0,"Samples do not advertise a real winner");
    assert.equal(await p.locator(".pc-offer-actions a").count(),0,"No buying links on fictional examples");
    assert.equal(writes.length,0,"Opening never writes defaults");
    await p.screenshot({path:`${out}/${lang}-${theme}-${width}.png`});
    const sample=recs.get("comparisons/headphones").data;
    write("comparisons","headphones",{...sample,status:"ready",title:"Agent verified comparison"});
    await p.getByRole("heading",{name:"Agent verified comparison"}).waitFor();await p.locator(".pc-best[data-offer=a]").waitFor();
    assert.equal(await p.locator(".pc-best").count(),1);
    await p.getByLabel(L("筛选平台","Filter platforms")).selectOption("平台 B");assert.equal(await p.locator(".pc-offer").count(),1);assert.equal(await p.locator(".pc-best").count(),0);
    await p.getByLabel(L("筛选平台","Filter platforms")).selectOption("");
    await p.getByLabel(L("搜索报价","Search offers")).fill("nothing matches");assert.equal(await p.locator(".pc-offer").count(),0);await p.getByLabel(L("搜索报价","Search offers")).fill("");
    await p.locator('[data-offer="b"]').getByRole("button",{name:L("收藏报价","Save offer"),exact:true}).click();
    await p.waitForFunction(()=>document.querySelector('[data-offer="b"] button[aria-pressed=true]'));
    await p.getByRole("button",{name:L("已收藏","Saved"),exact:true}).click();assert.equal(await p.locator(".pc-offer").count(),2);
    await p.getByRole("button",{name:L("已收藏","Saved"),exact:true}).click();
    await p.getByLabel(L("购买笔记","Buying notes")).fill("My shared buying note");await p.getByRole("button",{name:L("保存笔记","Save notes"),exact:true}).click();
    await p.waitForFunction(()=>!document.querySelector('.pc-note .btn'));
    assert.equal(recs.get("decisions/headphones").data.note,"My shared buying note");
    const oldDecision=structuredClone(recs.get("decisions/headphones").data);
    write("comparisons","headphones",{...sample,status:"ready",title:"A newer agent result"});
    await p.getByRole("heading",{name:"A newer agent result"}).waitFor();assert.deepEqual(recs.get("decisions/headphones").data,oldDecision);
    await p.getByRole("button",{name:L("导出","Export"),exact:true}).click();const exported=await p.evaluate(()=>JSON.parse(window.__downloads[0].content));assert.equal(exported.decision.note,"My shared buying note");assert.equal(exported.comparison.title,"A newer agent result");
    await p.getByRole("button",{name:L("分享比价","Share comparison"),exact:true}).click();assert.equal(await p.evaluate(()=>window.__clipboard),"https://workspace.example/pages/pg_test#compare?id=headphones");
    await p.getByRole("button",{name:L("请 agent 重新查价","Ask agent to check again"),exact:true}).click();await p.getByRole("button",{name:L("请 agent 查价","Ask agent to compare"),exact:true}).click();
    await p.locator("dialog").waitFor({state:"detached"});assert((await p.evaluate(()=>window.__messages[0])).includes("comparison headphones"));
    if(width===393){const cdp=await context.newCDPSession(p);await p.locator(".pc-main").evaluate(el=>el.scrollTop=0);await cdp.send("Input.dispatchTouchEvent",{type:"touchStart",touchPoints:[{x:190,y:850}]});for(const y of [750,650,550,450,350,250])await cdp.send("Input.dispatchTouchEvent",{type:"touchMove",touchPoints:[{x:190,y}]});await cdp.send("Input.dispatchTouchEvent",{type:"touchEnd",touchPoints:[]});await p.waitForFunction(()=>document.querySelector('.pc-main').scrollTop>0);}
    assert(await p.evaluate(w=>document.documentElement.scrollWidth<=w,width),"No content can expand the mobile viewport");assert.deepEqual(errors,[]);await context.close();
  }
  // Stale note cannot replace a concurrent user's text.
  reset();const p=await browser.newPage();await p.goto(base+"?myco_lang=en");await p.locator(".pc-offer").nth(3).waitFor();
  await p.getByLabel("Buying notes").fill("My stale draft");write("decisions","headphones",{saved:["b"],note:"Another person's latest note"});await p.waitForTimeout(200);
  await p.getByRole("button",{name:"Save notes",exact:true}).click();await p.locator(".pc-note [role=alert]").waitFor();assert.equal(recs.get("decisions/headphones").data.note,"Another person's latest note");
  await p.getByRole("button",{name:"Reload",exact:true}).click();assert.equal(await p.getByLabel("Buying notes").inputValue(),"Another person's latest note");
  // Expiry removes a former winner, even if the source snapshot itself was ready.
  const sample=recs.get("comparisons/headphones").data;write("comparisons","headphones",{...sample,status:"ready",offers:sample.offers.map(o=>({...o,expires_at:"2026-11-23T01:30:00Z"}))});
  await p.getByText("Offer expired",{exact:true}).first().waitFor();assert.equal(await p.locator(".pc-best").count(),0);
  // Literal hostile text remains text. Unsupported links invalidate the snapshot.
  write("comparisons","headphones",{...sample,title:'<img src=x onerror="window.pwned=1">'});await p.getByRole("heading",{name:'<img src=x onerror="window.pwned=1">'}).waitFor();assert.equal(await p.evaluate(()=>window.pwned),undefined);
  write("comparisons","broken",{...sample,title:"Broken",offers:[{...sample.offers[0],url:"javascript:alert(1)"}]});await p.getByRole("alert").filter({hasText:"incomplete"}).waitFor();assert.equal(await p.getByRole("button",{name:/Broken/}).count(),0);
  await p.goto(base+"?myco_lang=en#compare?id=missing");await p.getByRole("heading",{name:"Comparison not found"}).waitFor();await p.getByRole("button",{name:"Back to comparisons"}).click();await p.locator(".pc-offer").nth(3).waitFor();
  // Failed sends remain editable; old clients can copy without pretending a job ran.
  reset(true);await p.reload();await p.getByRole("button",{name:"Start your first comparison"}).click();await p.getByLabel("Product or model").fill("Real research request");await p.evaluate(()=>window.__sendFail=true);await p.getByRole("button",{name:"Ask agent to compare",exact:true}).click();await p.getByText("Could not send. Try again or copy the request to your agent.",{exact:true}).waitFor();assert.equal(recs.size,0);await p.getByRole("button",{name:"Close",exact:true}).last().click();
  await p.evaluate(()=>{delete myco.send;window.__copyFail=true});await p.getByRole("button",{name:"Start your first comparison"}).click();await p.getByLabel("Product or model").fill("Need comparison");await p.getByRole("button",{name:"Copy research request",exact:true}).click();await p.locator(".pc-prompt").waitFor();assert((await p.locator(".pc-prompt").inputValue()).includes("pg_test"));
  await p.close();console.log("PASS comparison UI: languages, themes, 1400/760/393 widths, touch scroll, sample safety, agent updates, filters, saves, preserved decisions, note conflicts, JSON export, direct links, expiry, hostile records, request failure and old-client copy fallback");
}finally{await browser.close();await new Promise(r=>server.close(r));}
