// Collector only: no DOM UI, network requests, polling or password values.
(() => {
  if (window.__mycoRecorder) return;
  const state = window.__mycoRecorder = {enabled:false, pending:null, timer:0};
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
  const flush = () => {clearTimeout(state.timer);if(state.pending){send(state.pending);state.pending=null;}};
  state.flush=flush;
  const element = ev => {const n=ev.composedPath()[0];return n && n.nodeType===1?n:null;};
  const meaningful = e => e.closest('button,a,input,textarea,select,[role],label,[contenteditable]') || e;
  document.addEventListener('input',ev=>{
    if(!state.enabled || !ev.isTrusted)return;
    const e=element(ev);if(!e || e.tagName==='SELECT' || ['file','checkbox','radio'].includes(e.type))return;
    if(state.pending && state.pendingElement!==e)flush();
    const sensitive=e.type==='password' || /password/i.test(e.autocomplete||'') || e.hasAttribute('data-sensitive');
    state.pendingElement=e;
    state.pending={type:'type',target:window.__mycoDescribe(e),clear:true,sensitive,value:sensitive?'${password}':cut(e.isContentEditable?e.innerText:e.value,16000)};
    clearTimeout(state.timer);state.timer=setTimeout(flush,250);
  },true);
  for(const kind of ['click','contextmenu'])document.addEventListener(kind,ev=>{
    if(!state.enabled || !ev.isTrusted)return;flush();const raw=element(ev);if(!raw)return;const e=meaningful(raw);
    if(e.tagName==='SELECT'||e.tagName==='OPTION'||e.type==='file')return;
    // Keep the two native click events of a double click in order. The
    // second event's CDP clickCount=2 reproduces its dblclick event too.
    send({type:'click',target:window.__mycoDescribe(e),button:kind==='contextmenu'?'right':ev.button===1?'middle':'left',count:ev.detail===2?2:1});
  },true);
  document.addEventListener('change',ev=>{
    if(!state.enabled || !ev.isTrusted)return;const e=element(ev);if(!e)return;flush();
    if(e.tagName==='SELECT')send({type:'select',target:window.__mycoDescribe(e),value:cut(e.value,4000)});
    if(e.type==='file')send({type:'upload',target:window.__mycoDescribe(e),value:'${upload}',note:'Choose the file again at run time; local file paths are not recorded.'});
  },true);
  document.addEventListener('keydown',ev=>{
    if(!state.enabled || !ev.isTrusted || ['Control','Meta','Alt','Shift'].includes(ev.key))return;
    if(ev.key==='Tab' || ev.key==='Enter' || ev.key==='Escape' || ev.ctrlKey || ev.metaKey || ev.altKey || /^Arrow/.test(ev.key)) {
      flush();const e=element(ev);
      const key=[ev.ctrlKey?'Control':'',ev.metaKey?'Meta':'',ev.altKey?'Alt':'',ev.shiftKey?'Shift':'',ev.key].filter(Boolean).join('+');
      send({type:'press',target:e?window.__mycoDescribe(e):undefined,key});
    }
  },true);
  let scrollTimer=0;
  document.addEventListener('scroll',ev=>{if(!state.enabled || !ev.isTrusted)return;clearTimeout(scrollTimer);const e=ev.target===document?document.scrollingElement:ev.target;
    scrollTimer=setTimeout(()=>{flush();send({type:'scroll',target:window.__mycoDescribe(e),dy:e.scrollTop,value:'absolute'});},180);
  },true);
  document.addEventListener('dragstart',ev=>{if(state.enabled && ev.isTrusted)state.drag=element(ev)},true);
  document.addEventListener('drop',ev=>{if(!state.enabled || !ev.isTrusted || !state.drag)return;flush();send({type:'drag',target:window.__mycoDescribe(state.drag),to:window.__mycoDescribe(element(ev))});state.drag=null},true);
  window.addEventListener('beforeunload',flush,true);
  document.addEventListener('focusout',flush,true);
})();
