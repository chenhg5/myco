// Fixed, component-owned code only. Never interpolate agent JavaScript.
function(op,arg) {
  const original=this, el=this.nodeType===1?this:this.parentElement;
  const parent=n=>n.parentElement||(n.getRootNode?.().host)||null;
  const sensitive=n=>n?.nodeType===1&&(n.matches('input[type="password"],[data-sensitive],[autocomplete="one-time-code"]')||/^(?:otp|one.?time(?:.?code)?|verification.?code|security.?code)$/i.test(n.getAttribute('name')||'')||/otp|one.?time/i.test(n.getAttribute('autocomplete')||''));
  let secret=false,panel=false;
  for(let n=el;n;n=parent(n)){if(sensitive(n))secret=true;if(n.id==='__myco_recording_panel__')panel=true;}
  const connected=!!original.isConnected&&!!el&&el.ownerDocument===document;
  let state=window.__mycoControlDOM;
  if(!state){
    state={clock:0,removed:new WeakMap(),versions:new WeakMap(),parents:new WeakMap(),roots:new WeakSet()};
    state.observe=root=>{if(state.roots.has(root))return;state.roots.add(root);new MutationObserver(records=>{for(const record of records)for(const node of record.removedNodes)state.removed.set(node,++state.clock);}).observe(root,{childList:true,subtree:true});};
    window.__mycoControlDOM=state;state.observe(document);
  }
  const ancestry=[];for(let n=original;n;n=parent(n)){ancestry.push(n);const root=n.getRootNode?.();if(root?.host)state.observe(root);}
  let generation=state.versions.get(original)||0;
  for(const node of [...ancestry,...(state.parents.get(original)||[])])generation=Math.max(generation,state.removed.get(node)||0);
  state.parents.set(original,ancestry);state.versions.set(original,generation);
  const rect=el?.getBoundingClientRect(),style=el&&getComputedStyle(el);
  const visible=!!rect&&rect.width>0&&rect.height>0&&style.visibility!=='hidden'&&style.display!=='none';
  const disabled=!!el?.disabled||el?.getAttribute('aria-disabled')==='true';
  const interactive=!!el?.matches('a[href],button,input,textarea,select,summary,[contenteditable],[tabindex],[role="button"],[role="link"],[role="checkbox"],[role="textbox"],[role="combobox"],[role="menuitem"]');
  const descendants=n=>[n,...(n.querySelectorAll?.('*')||[])];
  let axisAligned=true;
  for(let n=el;n;n=parent(n)){const m=getComputedStyle(n).transform;if(m&&m!=='none'){const t=new DOMMatrix(m);if(Math.abs(t.b)>0.0001||Math.abs(t.c)>0.0001||t.a<=0||t.d<=0)axisAligned=false;}}
  const info={generation,node_type:original.nodeType,scale_x:rect&&el.offsetWidth?rect.width/el.offsetWidth:1,scale_y:rect&&el.offsetHeight?rect.height/el.offsetHeight:1,axis_aligned:axisAligned,border_x:el?.clientLeft||0,border_y:el?.clientTop||0,connected,panel,sensitive:secret,disabled,interactive,visible,in_viewport:visible&&rect.bottom>0&&rect.right>0&&rect.top<innerHeight&&rect.left<innerWidth,rect:rect?{x:rect.x,y:rect.y,width:rect.width,height:rect.height}:null,value:!secret&&'value'in(el||{})?String(el.value):''};
  if(op==='inspect')return info;
  if(op==='verify_focus'){
    const root=el?.getRootNode();let active=root?.activeElement;
    while(active?.shadowRoot?.activeElement)active=active.shadowRoot.activeElement;
    return {safe:connected&&!secret&&!panel&&(active===el||!!el?.isContentEditable&&el.contains(active))};
  }
  if(op==='hit') {
    if(!connected||panel||!visible||disabled)return {hit:false};
    const root=el.getRootNode(),x=arg?.x??(rect.x+rect.width/2),y=arg?.y??(rect.y+rect.height/2);
    const hit=root.elementFromPoint?.(x,y)||document.elementFromPoint(x,y);
    return {hit:!!hit&&(hit===el||el.contains(hit))};
  }
  if(op==='describe')return connected&&!panel?window.__mycoDescribe(original):null;
  if(op==='select') {
    if(secret||disabled||!el.matches('select'))return {error:'target_not_interactable'};
    const option=[...el.options].find(o=>o.value===arg&&!o.disabled);
    if(!option)return {error:'target_not_found'};
    el.value=option.value;el.dispatchEvent(new Event('input',{bubbles:true}));el.dispatchEvent(new Event('change',{bubbles:true}));return {ok:true};
  }
  if(op==='scroll'){el.scrollBy(0,arg);return {ok:true};}
  if(op==='safe_document') {
    const scan=root=>{for(const n of root.querySelectorAll('*')){if(n.id==='__myco_recording_panel__')continue;if(sensitive(n))return false;if(n.shadowRoot&&!scan(n.shadowRoot))return false;}return true;};
    return {safe:scan(document)};
  }
  if(!connected||panel)return {error:'stale_ref'};
  if(op==='sensitive_focus')return {sensitive:secret};
  if(op==='extract') {
    if(secret)return {text:'[redacted]'};
    if(arg.what==='attribute') {
      if(/^(?:value|srcdoc|style|on.*)$/i.test(arg.attr))return {text:'[redacted]'};
      return {text:String(el.getAttribute(arg.attr)||'').slice(0,49152)};
    }
    const clone=original.cloneNode(true);
    for(const n of descendants(clone)) {
      if(n.nodeType!==1)continue;
      if(sensitive(n)||n.id==='__myco_recording_panel__'){n.replaceWith(document.createTextNode('[redacted]'));continue;}
      if(n.matches('script,style,iframe,object,embed')){n.remove();continue;}
      if(n.matches('input,textarea,select')){n.removeAttribute('value');n.textContent='';}
      for(const a of [...n.attributes])if(/^(?:on.*|srcdoc|value)$/i.test(a.name))n.removeAttribute(a.name);
    }
    return {text:String(arg.what==='html'?(clone.outerHTML||clone.textContent):clone.textContent||'').slice(0,49152)};
  }
  return {error:'unsupported'};
}
