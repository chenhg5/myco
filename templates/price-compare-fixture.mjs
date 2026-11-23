// A strict-CAS bridge with event delivery and the built app's ordinary CSP.
// This is transport for browser regressions; prices are explicitly test data.
import fs from "node:fs";
import http from "node:http";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { createRequire } from "node:module";
import { execFileSync } from "node:child_process";
const here=path.dirname(fileURLToPath(import.meta.url)),root=path.resolve(here,"../internal/templates/official/price-compare");
export async function startFixture() {
  const sample=JSON.parse(fs.readFileSync(path.join(here,"src/price-compare/public/sample.json"),"utf8")),recs=new Map();
  let seq=0,events=[];const writes=[];
  function reset(empty=false){recs.clear();seq=0;events=[];writes.length=0;for(const[c,rows]of Object.entries(empty?{}:sample))for(const{id,...data}of rows)recs.set(c+"/"+id,{collection:c,id,data,version:1});}
  function write(c,id,data){const old=recs.get(c+"/"+id);const row={collection:c,id,data,version:(old?.version||0)+1};recs.set(c+"/"+id,row);events.push({seq:++seq,collection:c,record:id,op:"put",data,version:row.version,by:"agent"});return row;}
  const bridge=`(()=>{const request=async(url,options)=>{const r=await fetch(url,options),v=await r.json();if(!r.ok)throw v;return v;};window.__downloads=[];window.__messages=[];window.__clipboard='';Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText:async x=>{if(window.__copyFail)throw Error('clipboard');window.__clipboard=x;}}});window.myco={features:[],ready:Promise.resolve({page:{id:'pg_test',name:'Comparison',url:'https://workspace.example/pages/pg_test'},me:{name:'tester'}}),send:async message=>{if(window.__sendFail)throw Error('send failed');window.__messages.push(message);return{id:'msg_test'}},data:{list:c=>request('/data?c='+encodeURIComponent(c)),put:(c,data,o)=>request('/data',{method:'POST',body:JSON.stringify({c,data,o})}),get:(c,id)=>request('/record?c='+encodeURIComponent(c)+'&id='+encodeURIComponent(id)),on:f=>{let seen=0,active=true;const t=setInterval(async()=>{try{for(const e of await request('/events?since='+seen)){seen=Math.max(seen,e.seq);if(active)f(e);}}catch{}},75);return()=>{active=false;clearInterval(t)}}},download:async(name,content,type)=>{window.__downloads.push({name,content,type});return{path:'~/Downloads/'+name}}};Date.now=()=>Date.parse('2026-11-23T02:00:00Z');})();`;
  const server=http.createServer(async(req,res)=>{const u=new URL(req.url,"http://test");
    const json=x=>{res.setHeader("Content-Type","application/json");res.end(JSON.stringify(x));};
    if(u.pathname==="/bridge.js"){res.setHeader("Content-Type","text/javascript");res.end(bridge);return;}
    if(u.pathname==="/events"){json(events.filter(e=>e.seq>Number(u.searchParams.get("since"))));return;}
    if(u.pathname==="/record"){json(recs.get(u.searchParams.get("c")+"/"+u.searchParams.get("id")));return;}
    if(u.pathname==="/data"&&req.method==="GET"){json({records:[...recs.values()].filter(r=>r.collection===u.searchParams.get("c")),seq});return;}
    if(u.pathname==="/data"&&req.method==="POST"){let source="";for await(const chunk of req)source+=chunk;const body=JSON.parse(source);writes.push(body);const{c,data,o}=body,old=recs.get(c+"/"+o.id);if(o.ifVersion!==(old?.version||0)){res.writeHead(409);json({code:"conflict",message:"Someone changed this.",latest:old});return;}json(write(c,o.id,data));return;}
    if(u.pathname==="/favicon.ico"){res.writeHead(204);res.end();return;}
    const f=path.resolve(root,"."+(u.pathname==="/"?"/index.html":u.pathname));if(!f.startsWith(root+path.sep)||!fs.existsSync(f)){res.writeHead(404);res.end();return;}
    res.setHeader("Content-Type",({".html":"text/html",".js":"text/javascript",".css":"text/css",".svg":"image/svg+xml",".webp":"image/webp"})[path.extname(f)]||"application/octet-stream");
    res.setHeader("Content-Security-Policy","default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; connect-src 'self'; object-src 'none'; base-uri 'self'");
    const content=fs.readFileSync(f);res.end(f.endsWith(".html")?String(content).replace("<head>",'<head><script src="/bridge.js"></script>'):content);
  });
  await new Promise(r=>server.listen(0,"127.0.0.1",r));
  const pw=createRequire(path.join(execFileSync("npm",["root","-g"],{encoding:"utf8"}).trim(),"noop.js"))("playwright");
  const browser=await pw.chromium.launch({headless:true,ignoreDefaultArgs:["--hide-scrollbars"],executablePath:process.env.UI_SMOKE_EXECUTABLE,args:["--no-sandbox"]});
  return{browser,server,recs,reset,writes,write,base:`http://127.0.0.1:${server.address().port}/`};
}
