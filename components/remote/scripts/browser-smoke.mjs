// Exercises a real browser decoder against the real optional component server.
// The synthetic FFmpeg source is injected by a Go test, never shipped as a mode.
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
import {execFileSync} from 'node:child_process';
import path from 'node:path';
let chromium;try{({chromium}=createRequire(import.meta.url)('playwright'));}catch{const p=execFileSync('npm',['root','-g'],{encoding:'utf8'}).trim();({chromium}=createRequire(path.join(p,'noop.js'))('playwright'));}
const browser=await chromium.launch(process.env.UI_SMOKE_EXECUTABLE?{executablePath:process.env.UI_SMOKE_EXECUTABLE}:{});
try{
 const page=await browser.newPage();
 await page.exposeFunction('answerOffer',async sdp=>{
  const r=await fetch(process.env.REMOTE_TEST_RPC_URL+'/sessions/browser',{method:'POST',headers:{'Content-Type':'application/json',Authorization:'Bearer '+process.env.REMOTE_TEST_RPC_TOKEN},body:JSON.stringify({description:{type:'offer',sdp},pairing_code:'abcdef12',control:false})});
  assert.equal(r.status,200);return r.json();
 });
 await page.setContent('<video autoplay playsinline muted></video>');
 await page.evaluate(async()=>{
  const pc=window.__pc=new RTCPeerConnection({iceServers:[]});
  pc.addTransceiver('video',{direction:'recvonly'});
  pc.ontrack=e=>{document.querySelector('video').srcObject=e.streams[0]||new MediaStream([e.track]);};
  await pc.setLocalDescription(await pc.createOffer());
  await new Promise(resolve=>{const check=()=>{if(pc.iceGatheringState==='complete')resolve();};pc.onicegatheringstatechange=check;check();});
  await pc.setRemoteDescription(await window.answerOffer(pc.localDescription.sdp));
 });
 await page.waitForFunction(()=>document.querySelector('video').videoWidth===640&&document.querySelector('video').currentTime>.3,{timeout:20000});
 const pixels=await page.evaluate(()=>{
  const canvas=document.createElement('canvas');canvas.width=64;canvas.height=36;const ctx=canvas.getContext('2d');ctx.drawImage(document.querySelector('video'),0,0,64,36);
  const values=ctx.getImageData(0,0,64,36).data;let colored=0;for(let i=0;i<values.length;i+=4)if(Math.max(values[i],values[i+1],values[i+2])-Math.min(values[i],values[i+1],values[i+2])>80)colored++;
  window.__pc.close();return colored;
 });
 assert(pixels>100,'decoded image must contain the synthetic color pattern');
 console.log('PASS real H.264 encode → RTP → encrypted WebRTC → Chromium decode (640×360), colored pixels:',pixels);
}finally{await browser.close();}
