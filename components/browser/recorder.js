// Isolated-world collector. Controls live in panel.js; password values never leave the page.
(() => {
  if (window.__mycoRecorder) return;
  const state = window.__mycoRecorder = {enabled:false, pending:null, timer:0};
  state.updatePanel = (next,sync=false) => {
    if(next.revision < (state.revision || 0)) return;
    state.revision=next.revision;
    if(state.clearScroll && (state.recordingID!==next.id || state.picking!==!!next.picking || state.selected!==next.selected || (sync && !next.recording))) state.clearScroll(!!state.recordingID);
    if(sync) state.enabled=!!next.recording;
    state.picking=!!next.picking;
    state.selected=next.selected;
    state.recordingID=next.id;
    if(state.renderPanel) state.renderPanel(next);
  };
  const cut = (s,n=160) => String(s || '').slice(0,n);
  const css = e => {
    const path=[];
    for(let n=e;n && n.nodeType===1;n=n.parentElement) {
      if(n.id) {path.unshift('#'+CSS.escape(n.id));break;}
      let s=n.localName, siblings=n.parentElement && [...n.parentElement.children].filter(x=>x.localName===s);
      if(siblings && siblings.length>1)s+=':nth-of-type('+(siblings.indexOf(n)+1)+')';
      path.unshift(s);
    }
    return path.join(' > ');
  };
  const text = e => cut(e.innerText || e.textContent).trim().replace(/\s+/g,' ');
  const role = e => e.getAttribute('role') || ({BUTTON:'button',A:'link',TEXTAREA:'textbox',SELECT:'combobox'}[e.tagName]) ||
    (e.tagName==='INPUT' ? ({checkbox:'checkbox',radio:'radio',button:'button',submit:'button',range:'slider'}[e.type] || 'textbox') : e.isContentEditable?'textbox':'');
  const label = e => {
    const by=e.getAttribute('aria-labelledby');
    if(by) {const v=by.split(/\s+/).map(id=>e.ownerDocument.getElementById(id)?.textContent || '').join(' ').trim();if(v)return cut(v);}
    return cut(e.getAttribute('aria-label') || [...(e.labels||[])].map(x=>text(x)).join(' ') || e.getAttribute('title') || e.getAttribute('placeholder') || (['button','link'].includes(role(e))?text(e):''));
  };
  window.__mycoDescribe = e => {
    if(!e || e.nodeType!==1)return {locators:[]};
    const locators=[];
    for(const attr of ['data-testid','data-test','data-qa'])if(e.hasAttribute(attr))locators.push({kind:'testid',value:attr+'='+cut(e.getAttribute(attr),256)});
    const r=role(e),name=label(e);
    if(r && name)locators.push({kind:'role',role:r,name});
    const labels=[...(e.labels||[])].map(x=>text(x)).filter(Boolean);
    if(labels.length)locators.push({kind:'label',value:labels[0]});
    if(e.hasAttribute('placeholder'))locators.push({kind:'placeholder',value:cut(e.getAttribute('placeholder'))});
    if(['button','link'].includes(r) && text(e))locators.push({kind:'text',value:text(e)});
    locators.push({kind:'css',value:css(e)});
    if(e.id) {const quoted=e.id.includes('"')?null:'"'+e.id+'"';if(quoted)locators.push({kind:'xpath',value:'//*[@id='+quoted+']'});}
    const shadow=[];for(let root=e.getRootNode();root && root.host;root=root.host.getRootNode())shadow.unshift(css(root.host));
    const frame=[];try{for(let w=window;w!==w.top && w.frameElement;w=w.parent)frame.unshift(css(w.frameElement));}catch(_){}
    return {locators,frame,shadow};
  };
  const send = node => {if(state.enabled && typeof window.__mycoBrowserEvent==='function')window.__mycoBrowserEvent(JSON.stringify(node));};
  const flushText = () => {clearTimeout(state.timer);if(state.pending){send(state.pending);state.pending=null;}};
  let flushScroll=()=>{};
  const flush=()=>{flushText();flushScroll();};
  state.flush=flush;
  const ignored = ev => state.ignoreEvent && state.ignoreEvent(ev);
  const element = ev => {const n=ev.composedPath()[0];return n && n.nodeType===1?n:null;};
  const meaningful = e => e.closest('button,a,input,textarea,select,[role],label,[contenteditable]') || e;
  // Scroll events are trusted even when focus, layout or scrollIntoView caused
  // them. Require explicit intent in this container and a quiet action window.
  const quietMS=400, intentMS=900;
  let lastAction=-Infinity, epoch=0, intents=new WeakMap(), scrollTimer=0, pendingScroll=null, pointer=null, releasedPointer=null;
  const extent=e=>[e.scrollHeight,e.clientHeight,e.scrollWidth,e.clientWidth].join('/');
  const clearScroll=(action=false)=>{clearTimeout(scrollTimer);pendingScroll=null;intents=new WeakMap();pointer=null;epoch++;lastAction=action?Date.now():-Infinity;};
  state.clearScroll=clearScroll;
  flushScroll=()=>{
    clearTimeout(scrollTimer);const pending=pendingScroll;pendingScroll=null;
    if(!pending||!state.enabled||state.picking||pending.epoch!==epoch||Date.now()-lastAction<quietMS)return;
    const intent=intents.get(pending.element);
    if(!intent||Date.now()-intent.at>intentMS||intent.extent!==extent(pending.element))return;
    flushText();send({type:'scroll',target:window.__mycoDescribe(pending.element),dy:pending.element.scrollTop,value:'absolute'});
  };
  const activity=()=>{flushScroll();clearScroll(true);};
  const scrollers=e=>{
    const out=[];
    for(let n=e;n&&n.nodeType===1;n=n.parentElement||(n.getRootNode()?.host)){
      if(n===document.scrollingElement||(n.scrollHeight>n.clientHeight&&/^(auto|scroll|overlay)$/.test(getComputedStyle(n).overflowY)))out.push(n);
    }
    if(document.scrollingElement&&!out.includes(document.scrollingElement))out.push(document.scrollingElement);
    return out;
  };
  const arm=(e,direction)=>{
    if(Date.now()-lastAction<quietMS)return;
    const container=scrollers(e).find(n=>direction>0?n.scrollTop+n.clientHeight<n.scrollHeight:n.scrollTop>0);
    if(container)intents.set(container,{at:Date.now(),extent:extent(container)});
  };
  const scrollbar=(e,ev)=>{
    if(!e||e.scrollHeight<=e.clientHeight)return false;
    const r=e===document.scrollingElement?{left:0,top:0,right:innerWidth,bottom:innerHeight}:e.getBoundingClientRect();
    // The gutter may be on either side; overlay scrollbars use a narrow edge.
    return ev.clientY>=r.top&&ev.clientY<=r.bottom&&((ev.clientX>=r.right-12&&ev.clientX<=r.right)||(ev.clientX>=r.left&&ev.clientX<=r.left+Math.max(0,e.clientLeft||0)));
  };
  const scrollClick=ev=>releasedPointer&&Date.now()-releasedPointer.at<100&&scrollbar(releasedPointer.element,ev);
  document.addEventListener('wheel',ev=>{
    if(!state.enabled||!ev.isTrusted||state.picking)return;
    if(ignored(ev)){activity();return;}
    if(!ev.ctrlKey&&!ev.metaKey&&ev.deltaY)arm(element(ev),Math.sign(ev.deltaY));
  },{capture:true,passive:true});
  document.addEventListener('pointerdown',ev=>{
    if(!state.enabled||!ev.isTrusted||state.picking)return;
    if(ignored(ev)){activity();return;}
    const container=scrollers(element(ev)).find(e=>scrollbar(e,ev));
    if(container&&ev.button===0&&Date.now()-lastAction>=quietMS){
      pointer={id:ev.pointerId,element:container};intents.set(container,{at:Date.now(),extent:extent(container)});
    }else activity();
  },true);
  document.addEventListener('pointermove',ev=>{
    if(pointer&&ev.isTrusted&&ev.pointerId===pointer.id&&ev.buttons&1)intents.set(pointer.element,{at:Date.now(),extent:extent(pointer.element)});
  },true);
  document.addEventListener('pointerup',ev=>{if(pointer&&ev.isTrusted&&ev.pointerId===pointer.id){releasedPointer={element:pointer.element,at:Date.now()};pointer=null;}},true);
  document.addEventListener('pointercancel',()=>{pointer=null;},true);
  document.addEventListener('focusin',ev=>{if(state.enabled&&ev.isTrusted&&!pointer)activity();},true);
  const scrollingKey=ev=>{
    const e=element(ev);
    if(!e||e.isContentEditable||e.closest('input,textarea,select,button,[contenteditable]')||ev.ctrlKey||ev.metaKey||ev.altKey)return false;
    const direction=({PageDown:1,PageUp:-1,End:1,Home:-1,ArrowDown:1,ArrowUp:-1,' ':ev.shiftKey?-1:1})[ev.key];
    if(!direction)return false;
    arm(e,direction);return true;
  };

  document.addEventListener('input',ev=>{
    if(!state.enabled || !ev.isTrusted || ignored(ev) || state.picking)return;
    activity();const e=element(ev);if(!e || e.tagName==='SELECT' || ['file','checkbox','radio'].includes(e.type))return;
    if(state.pending && state.pendingElement!==e)flush();
    const sensitive=e.type==='password' || /password/i.test(e.autocomplete||'') || e.hasAttribute('data-sensitive');
    state.pendingElement=e;
    state.pending={type:'type',target:window.__mycoDescribe(e),clear:true,sensitive,value:sensitive?'${password}':cut(e.isContentEditable?e.innerText:e.value,16000)};
    clearTimeout(state.timer);state.timer=setTimeout(flush,250);
  },true);
  for(const kind of ['click','contextmenu'])document.addEventListener(kind,ev=>{
    if(!ev.isTrusted)return;
    if(ignored(ev)){activity();return;}
    if(scrollClick(ev))return;
    if(state.picking){
      ev.preventDefault();ev.stopImmediatePropagation();
      if(kind==='click'){const e=element(ev);if(e){state.picking=false;window.__mycoBrowserEvent(JSON.stringify({type:'panel',action:'picked',recording_id:state.recordingID,target:window.__mycoDescribe(e)}));}}
      return;
    }
    if(!state.enabled)return;activity();flush();const raw=element(ev);if(!raw)return;const e=meaningful(raw);
    if(e.tagName==='SELECT'||e.tagName==='OPTION'||e.type==='file')return;
    // Keep the two native click events of a double click in order. The
    // second event's CDP clickCount=2 reproduces its dblclick event too.
    send({type:'click',target:window.__mycoDescribe(e),button:kind==='contextmenu'?'right':ev.button===1?'middle':'left',count:ev.detail===2?2:1});
  },true);
  document.addEventListener('change',ev=>{
    if(!state.enabled || !ev.isTrusted || ignored(ev) || state.picking)return;activity();const e=element(ev);if(!e)return;flush();
    if(e.tagName==='SELECT')send({type:'select',target:window.__mycoDescribe(e),value:cut(e.value,4000)});
    if(e.type==='file')send({type:'upload',target:window.__mycoDescribe(e),value:'${upload}',note:'Choose the file again at run time; local file paths are not recorded.'});
  },true);
  document.addEventListener('keydown',ev=>{
    if(state.picking && ev.isTrusted && ev.key==='Escape'){ev.preventDefault();ev.stopImmediatePropagation();window.__mycoBrowserEvent(JSON.stringify({type:'panel',action:'cancel-pick',recording_id:state.recordingID}));return;}
    if(!state.enabled || !ev.isTrusted || state.picking)return;
    if(ignored(ev)){activity();return;}
    if(['Control','Meta','Alt','Shift'].includes(ev.key))return;
    if(scrollingKey(ev))return;
    activity();
    if(ev.key==='Tab' || ev.key==='Enter' || ev.key==='Escape' || ev.ctrlKey || ev.metaKey || ev.altKey || /^Arrow/.test(ev.key)) {
      flush();const e=element(ev);
      const key=[ev.ctrlKey?'Control':'',ev.metaKey?'Meta':'',ev.altKey?'Alt':'',ev.shiftKey?'Shift':'',ev.key].filter(Boolean).join('+');
      send({type:'press',target:e?window.__mycoDescribe(e):undefined,key});
    }
  },true);
  document.addEventListener('scroll',ev=>{
    if(!state.enabled||!ev.isTrusted||ignored(ev)||state.picking)return;
    const e=ev.target===document?document.scrollingElement:ev.target,intent=e&&intents.get(e);
    if(!intent||Date.now()-intent.at>intentMS||Date.now()-lastAction<quietMS||intent.extent!==extent(e))return;
    clearTimeout(scrollTimer);pendingScroll={element:e,epoch};scrollTimer=setTimeout(flushScroll,180);
  },true);
  document.addEventListener('dragstart',ev=>{if(state.enabled && ev.isTrusted && !ignored(ev) && !state.picking){activity();state.drag=element(ev)}},true);
  document.addEventListener('drop',ev=>{if(!state.enabled || !ev.isTrusted || ignored(ev) || state.picking || !state.drag)return;activity();flush();send({type:'drag',target:window.__mycoDescribe(state.drag),to:window.__mycoDescribe(element(ev))});state.drag=null},true);
  window.addEventListener('beforeunload',flush,true);
  document.addEventListener('focusout',()=>{flushText();if(state.enabled)activity();},true);
  window.addEventListener('blur',()=>{if(state.enabled)activity();},true);
})();
