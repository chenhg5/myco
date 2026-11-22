// Shared strict-version page bridge for scheduling browser regressions.
import fs from "node:fs";
import http from "node:http";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { createRequire } from "node:module";
import { execFileSync } from "node:child_process";
const here=path.dirname(fileURLToPath(import.meta.url)),root=path.resolve(here,"../internal/templates/official/scheduling");
export async function startFixture() {
  const sample=JSON.parse(fs.readFileSync(path.join(here,"src/scheduling/public/sample.json"),"utf8")),recs=new Map();
  let seq=0,events=[],race=false,pending=[];
  function reset(empty=false){recs.clear();seq=0;events=[];for(const[c,rows]of Object.entries(empty?{}:sample))for(const{id,...data}of rows)recs.set(c+"/"+id,{collection:c,id,data,version:1});}
  function put(body,res){const{c,data,o}=body,key=c+"/"+o.id,old=recs.get(key);if(o.ifVersion!==(old?.version||0)){res.writeHead(409,{"Content-Type":"application/json"});res.end(JSON.stringify({code:"conflict",message:"Someone changed this. Choose again.",latest:old}));return;}const row={collection:c,id:o.id,data,version:(old?.version||0)+1};recs.set(key,row);events.push({seq:++seq,collection:c,record:o.id,op:"put",data,version:row.version,by:"tester"});res.setHeader("Content-Type","application/json");res.end(JSON.stringify(row));}
  const bridge=`(()=>{const request=async(url,options)=>{const r=await fetch(url,options),v=await r.json();if(!r.ok)throw v;return v;};window.__downloads=[];window.myco={features:[],ready:Promise.resolve({page:{id:'pg_test',name:'Calendar'},me:{name:'tester'}}),data:{list:c=>request('/data?c='+encodeURIComponent(c)),put:(c,data,o)=>request('/data',{method:'POST',body:JSON.stringify({c,data,o})}),remove:(c,id,o)=>request('/data?c='+encodeURIComponent(c)+'&id='+encodeURIComponent(id)+'&version='+o.ifVersion,{method:'DELETE'}),on:f=>{let seen=0,active=true;const t=setInterval(async()=>{try{for(const e of await request('/events?since='+seen)){seen=Math.max(seen,e.seq);if(active)f(e);}}catch{}},100);return()=>{active=false;clearInterval(t)}}},download:async(name,content,type)=>{window.__downloads.push({name,content,type});return{path:'~/Downloads/'+name}}};Date.now=()=>Date.parse('2026-11-22T00:00:00Z');})();`;
  const server=http.createServer(async(req,res)=>{const u=new URL(req.url,"http://test");
    if(u.pathname==="/bridge.js"){res.setHeader("Content-Type","text/javascript");res.end(bridge);return;}
    if(u.pathname==="/events"){res.setHeader("Content-Type","application/json");res.end(JSON.stringify(events.filter(e=>e.seq>Number(u.searchParams.get("since")))));return;}
    if(u.pathname==="/data"&&req.method==="GET"){res.setHeader("Content-Type","application/json");res.end(JSON.stringify({records:[...recs.values()].filter(r=>r.collection===u.searchParams.get("c")),seq}));return;}
    if(u.pathname==="/data"&&req.method==="DELETE"){const c=u.searchParams.get("c"),id=u.searchParams.get("id"),old=recs.get(c+"/"+id);res.setHeader("Content-Type","application/json");if(!old||old.version!==Number(u.searchParams.get("version"))){res.writeHead(409);res.end(JSON.stringify({code:"conflict",message:"Event changed",latest:old}));return;}recs.delete(c+"/"+id);events.push({seq:++seq,collection:c,record:id,op:"delete",version:0,by:"tester"});res.end("true");return;}
    if(u.pathname==="/data"&&req.method==="POST"){let source="";for await(const chunk of req)source+=chunk;const body=JSON.parse(source);if(race&&body.c==="days"){pending.push([body,res]);if(pending.length===2){race=false;for(const[b,r]of pending)put(b,r);pending=[];}return;}put(body,res);return;}
    if(u.pathname==="/favicon.ico"){res.writeHead(204);res.end();return;}const f=path.resolve(root,"."+(u.pathname==="/"?"/index.html":u.pathname));if(!f.startsWith(root+path.sep)||!fs.existsSync(f)){res.writeHead(404);res.end();return;}res.setHeader("Content-Type",({".html":"text/html",".js":"text/javascript",".css":"text/css",".svg":"image/svg+xml",".webp":"image/webp"})[path.extname(f)]||"application/octet-stream");const content=fs.readFileSync(f);res.end(f.endsWith(".html")?String(content).replace("<head>",'<head><script src="/bridge.js"></script>'):content);
  });
  await new Promise(r=>server.listen(0,"127.0.0.1",r));
  const pw=createRequire(path.join(execFileSync("npm",["root","-g"],{encoding:"utf8"}).trim(),"noop.js"))("playwright");
  const browser=await pw.chromium.launch({headless:true,executablePath:process.env.UI_SMOKE_EXECUTABLE,args:["--no-sandbox"]});
  return{browser,server,recs,reset,base:`http://127.0.0.1:${server.address().port}/`,beginRace:()=>{race=true}};
}
