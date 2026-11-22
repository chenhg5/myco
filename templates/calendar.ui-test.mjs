// Calendar-first workflows in both languages/themes, including touch and CAS undo.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import { startFixture } from './scheduling-test-fixture.mjs';
const out=process.env.CALENDAR_SHOTS||'/tmp/calendar-shots';fs.mkdirSync(out,{recursive:true});
const {browser,server,base,recs,reset}=await startFixture();
try{
 for(const[lang,theme,width]of[['zh','light',1400],['en','dark',1400],['zh','dark',760],['en','light',760],['zh','light',393],['en','dark',393]]){
  reset();const context=await browser.newContext({viewport:{width,height:900},colorScheme:theme,timezoneId:'Asia/Shanghai',isMobile:width<400,hasTouch:width<400}),p=await context.newPage(),errors=[];
  p.on('pageerror',e=>errors.push(e.message));p.setDefaultTimeout(10000);const L=(zh,en)=>lang==='zh'?zh:en;
  await p.goto(base+'?myco_lang='+lang);await p.locator('.pc-month-grid').waitFor();assert.equal(await p.locator('.sc-types').count(),0,'Calendar is the default home');
  assert.equal(await p.locator('.pc-cell').count(),42);assert(await p.locator('.pc-event').count()>10);
  assert(await p.evaluate(width=>document.documentElement.scrollWidth<=width,width),'No horizontal overflow');
  await p.screenshot({path:`${out}/${lang}-${theme}-${width}-month.png`});
  await p.locator('.pc-cell[data-date="2026-11-30"]').click({position:{x:2,y:2}});
  await p.getByLabel(L('日程标题','Event title'),{exact:true}).fill('Calendar test');
  await p.getByLabel(L('开始','Start'),{exact:true}).fill('2026-11-30T15:00');await p.getByLabel(L('结束','End'),{exact:true}).fill('2026-11-30T16:00');
  await p.getByRole('button',{name:L('保存','Save'),exact:true}).click();await p.locator('dialog').waitFor({state:'detached'});
  let row=[...recs.values()].find(r=>r.collection==='events'&&r.data.title==='Calendar test');assert(row);assert.equal(row.data.start,'2026-11-30T07:00:00.000Z');
  if(width===1400){
   await p.locator(`.pc-cell[data-date="2026-11-30"] [data-event="${row.id}"]`).dragTo(p.locator('.pc-cell[data-date="2026-12-01"]'),{targetPosition:{x:2,y:2}});
   await p.waitForFunction(async(id)=>(await myco.data.list('events')).records.find(r=>r.id===id)?.data.start.startsWith('2026-12-01'),row.id);
   await p.locator('.pc-history button').nth(0).click();await p.waitForFunction(async(id)=>(await myco.data.list('events')).records.find(r=>r.id===id)?.data.start.startsWith('2026-11-30'),row.id);
  }
  await p.getByRole('button',{name:L('查看 2026-11-30','View 2026-11-30'),exact:true}).click();
  await p.getByRole('combobox',{name:L('日历视图','Calendar view')}).selectOption('week');await p.locator('.pc-time-grid').waitFor();assert.equal(await p.locator('.pc-day-lane').count(),7);
  await p.screenshot({path:`${out}/${lang}-${theme}-${width}-week.png`});
  if(width<400){const scroll=p.locator('.pc-time-scroll'),box=await scroll.boundingBox(),before=await scroll.evaluate(e=>e.scrollTop),cdp=await context.newCDPSession(p);await cdp.send('Input.dispatchTouchEvent',{type:'touchStart',touchPoints:[{x:box.x+20,y:box.y+Math.min(box.height-20,400)}]});await cdp.send('Input.dispatchTouchEvent',{type:'touchMove',touchPoints:[{x:box.x+20,y:box.y+20}]});await cdp.send('Input.dispatchTouchEvent',{type:'touchEnd',touchPoints:[]});await p.waitForFunction(before=>document.querySelector('.pc-time-scroll').scrollTop>before,before);}
  await p.getByRole('combobox',{name:L('日历视图','Calendar view')}).selectOption('day');assert.equal(await p.locator('.pc-day-lane').count(),1);
  await p.getByRole('textbox',{name:L('搜索日程','Search events'),exact:true}).fill('Calendar test');await p.locator('.pc-results button').first().click();
  await p.getByRole('button',{name:L('删除','Delete'),exact:true}).click();await p.locator('dialog').waitFor({state:'detached'});assert(!recs.has('events/'+row.id));
  await p.locator('.pc-history button').nth(0).click();await p.locator('.pc-results button').first().waitFor();assert(recs.has('events/'+row.id));
  await p.locator('.pc-history button').nth(1).click();await p.waitForFunction(async(id)=>!(await myco.data.list('events')).records.some(r=>r.id===id),row.id);
  await p.locator('.pc-history button').nth(0).click();await p.locator('.pc-results button').first().waitFor();
  await p.getByRole('textbox',{name:L('搜索日程','Search events'),exact:true}).fill('');
  await p.getByRole('button',{name:L('新建日程','New event'),exact:true}).click();await p.getByLabel(L('日程标题','Event title'),{exact:true}).fill('All-day test');await p.getByLabel(L('全天','All day'),{exact:true}).check();await p.getByLabel(L('开始','Start'),{exact:true}).fill('2026-11-30');await p.getByLabel(L('结束','End'),{exact:true}).fill('2026-12-01');await p.getByLabel(L('重复','Repeat'),{exact:true}).selectOption('weekly');await p.getByLabel(L('重复截止日期（可选）','Repeat until (optional)'),{exact:true}).fill('2026-12-31');await p.getByRole('button',{name:L('保存','Save'),exact:true}).click();await p.locator('dialog').waitFor({state:'detached'});
  const ad=[...recs.values()].find(r=>r.data.title==='All-day test');assert.equal(ad.data.end,'2026-12-02');assert.equal(ad.data.repeat,'weekly');assert.equal(ad.data.until,'2026-12-31');
  await p.getByRole('button',{name:L('导出当前视图','Export current view'),exact:true}).click();const dl=await p.evaluate(()=>window.__downloads.at(-1));assert(dl.content.includes('VALUE=DATE:20261130'));
  // Secondary booking access remains usable and returns to the calendar.
  await p.getByRole('button',{name:L('预约管理','Appointments'),exact:true}).click();await p.locator('.sc-type').first().waitFor();await p.getByRole('button',{name:L('返回日历','Back to calendar'),exact:true}).click();await p.locator('.pc-month-grid').waitFor();
  assert.deepEqual(errors,[]);await context.close();
 }
 reset(true);const p=await browser.newPage({timezoneId:'Asia/Shanghai'});await p.goto(base+'?myco_lang=en');await p.locator('.pc-month-grid').waitFor();assert.equal(await p.locator('.sc-welcome').count(),0,'Empty calendar needs no booking profile');await p.getByRole('button',{name:'New event',exact:true}).click();await p.getByLabel('Event title',{exact:true}).fill('First event');await p.getByRole('button',{name:'Save',exact:true}).click();await p.locator('dialog').waitFor({state:'detached'});
 // A remote agent edit cannot be overwritten by an already-open editor.
 const row=[...recs.values()].find(r=>r.collection==='events');await p.locator(`[data-event="${row.id}"]`).first().click();await p.getByLabel('Event title',{exact:true}).fill('Stale title');const r=await fetch(base+'data',{method:'POST',body:JSON.stringify({c:'events',o:{id:row.id,ifVersion:row.version},data:{...row.data,title:'Agent title'}})});assert.equal(r.status,200);await p.getByRole('button',{name:'Save',exact:true}).click();await p.locator('dialog [role=alert]').waitFor();assert.equal(recs.get('events/'+row.id).data.title,'Agent title');await p.getByRole('button',{name:'Close',exact:true}).click();await p.evaluate(()=>{Date.now=()=>Date.parse('2026-11-23T00:00:00Z')});await p.getByRole('button',{name:'Today',exact:true}).click();assert.equal(await p.locator('.pc-cell[data-date="2026-11-23"] .pc-day-number.is-today').count(),1);await p.close();
 console.log('PASS calendar UI: calendar-first empty/sample pages, create, drag, series editor, day/week/month, delete/undo/redo, all-day export, secondary appointments, live CAS conflicts, locales/themes and phone touch scroll');console.log('Screenshots: '+out);
}finally{await browser.close();await new Promise(r=>server.close(r));}
