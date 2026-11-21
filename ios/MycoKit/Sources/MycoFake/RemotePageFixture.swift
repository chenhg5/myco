/// Only the explicit `-myco-start remote` demo. It uses the real WebKit
/// RTCPeerConnection and native page bridge without credentials or a host.
enum RemotePageFixture {
    static let html = #"""
    <!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1">
    <style>body{font:17px -apple-system;padding:24px;color:#222;background:#fafafa}button{font:inherit;display:block;min-height:48px;margin:16px 0;padding:10px;border:1px solid #bbb;border-radius:10px;background:#fff;color:#222}#status{min-height:80px}</style></head><body>
    <h1>Remote controller probe</h1><p id="status">Ready</p><p id="error"></p>
    <label>Local draft <textarea aria-label="Local draft" id="draft"></textarea></label>
    <button id="probe">Probe WebRTC</button><button id="connect">Connect test computer</button><button id="host">Attempt host action</button><button id="disconnect">Disconnect test session</button>
    <script>
    const status=document.getElementById('status');let offer,connection;
    myco.remote.onDisconnect(()=>{status.textContent='Disconnected';});
    document.getElementById('probe').onclick=async()=>{try{
      if(!RTCRtpReceiver.getCapabilities('video').codecs.some(c=>c.mimeType.toLowerCase()==='video/h264'))throw Error('H264 unavailable');
      const p=new RTCPeerConnection({iceServers:[]});p.addTransceiver('video',{direction:'recvonly'});offer=await p.createOffer();p.close();status.textContent='RTC ready';
    }catch(e){status.textContent='RTC failed: '+e.message;}};
    document.getElementById('connect').onclick=async()=>{try{connection=await myco.remote.call('connect',{machine:'app:fixture',name:'Untrusted page title',offer:{description:offer,pairing_code:'test-only',control:true}});status.textContent='Connected test session';}catch(e){status.textContent='Denied';document.getElementById('error').textContent=e.message||String(e);}};
    document.getElementById('disconnect').onclick=async()=>{await myco.remote.call('disconnect',{id:connection.id});status.textContent='Disconnected';};
    document.getElementById('host').onclick=async()=>{try{await myco.remote.call('enable');status.textContent='Unexpected host allowed';}catch(e){status.textContent='Host refused';}};
    </script></body></html>
    """#
}
