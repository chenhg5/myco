// Production UI regression: units follow the OS, persist locally, and preserve zero allocation.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import http from 'node:http';
import {createRequire} from 'node:module';
import {execFileSync} from 'node:child_process';
import {fileURLToPath} from 'node:url';
const here=path.dirname(fileURLToPath(import.meta.url)),root=path.resolve(here,'../internal/templates/official/disk');
const fake=fs.readFileSync(path.join(here,'shots-os.js'),'utf8').replaceAll('os: "darwin"','os: window.__TEST_OS__').replace('largest.sort((a, b) => b.size - a.size);','largest.sort((a, b) => b.size - a.size); largest.unshift({path: ROOT+"/sparse.bin",size:16777216,disk:0,modified:0});');
const bridge=`window.myco={features:['os'],os:window.__FAKE_OS__,ready:Promise.resolve({page:{id:'pg_disk'},me:{name:'tester'}}),data:{list:async()=>({records:[],seq:0}),on:()=>()=>{} }};`;
const server=http.createServer((req,res)=>{const u=new URL(req.url,'http://test');if(u.pathname==='/fixture.js'){res.setHeader('Content-Type','text/javascript');res.end(fake+bridge);return;}const f=path.resolve(root,'.'+(u.pathname==='/'?'/index.html':u.pathname));if(!f.startsWith(root+path.sep)||!fs.existsSync(f)){res.writeHead(404);res.end();return;}res.setHeader('Content-Type',({'.html':'text/html','.js':'text/javascript','.css':'text/css','.svg':'image/svg+xml'})[path.extname(f)]||'application/octet-stream');res.setHeader('Content-Security-Policy',"default-src 'self';script-src 'self';style-src 'self' 'unsafe-inline';img-src 'self' data: blob:;connect-src 'self';object-src 'none'");const b=fs.readFileSync(f);res.end(f.endsWith('.html')?String(b).replace('<head>','<head><script src="/fixture.js"></script>'):b);});
await new Promise(r=>server.listen(0,'127.0.0.1',r));
const require=createRequire(path.join(execFileSync('npm',['root','-g'],{encoding:'utf8'}).trim(),'noop.js'));
const browser=await require('playwright').chromium.launch({headless:true,args:['--no-sandbox']});
try{for(const [os,theme,width]of[['windows','light',1200],['windows','dark',760],['darwin','light',1200],['darwin','dark',760]]){
 const context=await browser.newContext({viewport:{width,height:900},colorScheme:theme});await context.addInitScript(os=>{window.__TEST_OS__=os;},os);const p=await context.newPage(),errors=[];p.on('pageerror',e=>errors.push(e.message));
 await p.goto(`http://127.0.0.1:${server.address().port}/?myco_lang=zh`);await p.locator('.dk-disk-free').first().waitFor();const unit=p.getByLabel('容量单位');assert.equal(await unit.inputValue(),'auto');await p.waitForFunction(binary=>document.querySelector('.dk-disk-free').textContent.includes(binary?'GiB':'GB'),os==='windows');
 await unit.selectOption('decimal');assert((await p.locator('.dk-disk-foot').first().innerText()).includes('433 GB'),'Used space retains the current user availability/quota boundary');
 await unit.selectOption('binary');await p.locator('.dk-disk .btn-primary').first().click();await p.locator('.sb-size').waitFor();assert((await p.locator('.sb-size').textContent()).includes('GiB'));
 await unit.selectOption('decimal');assert((await p.locator('.sb-size').textContent()).includes('GB'));
 await p.locator('.dk-tabs button').nth(1).click();const row=p.locator('.dk-row').filter({hasText:'sparse.bin'});assert.equal(await row.locator('.dk-size').innerText(),'0 B');await row.hover();await row.locator('button').last().click();assert((await p.locator('.dk-basket-total').innerText()).includes('0 B'));
 assert(await p.evaluate(w=>document.documentElement.scrollWidth<=w,width));assert.deepEqual(errors,[]);await p.reload();await p.locator('.dk-disk-free').first().waitFor();assert.equal(await unit.inputValue(),'decimal');await context.close();
}console.log('PASS disk units: Windows binary/Mac decimal defaults, local preference, map/list/basket consistency, zero allocation, user availability accounting, light/dark at 1200/760.');}finally{await browser.close();await new Promise(r=>server.close(r));}
