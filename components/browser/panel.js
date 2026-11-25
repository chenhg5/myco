// Isolated-world controls: the site cannot access the binding or closed root.
(() => {
 const state=window.__mycoRecorder;if(!state||state.panelInstalled)return;
 state.panelInstalled=true;
 if(window!==window.top)return;
 let model=null,render=()=>{};
 const zh=/^zh\b/i.test(navigator.language||''),t=(cn,en)=>zh?cn:en;
 let host,collapsed=false,selectedKey='',renderedSteps='';
 state.ignoreEvent=ev=>!!host&&ev.composedPath().includes(host);
 const command=(action,fields={})=>window.__mycoBrowserEvent(JSON.stringify({type:'panel',action,recording_id:model.id,...fields}));
 const mount=()=>{
  if(host||!document.documentElement)return;
  host=document.createElement('div');host.id='__myco_recording_panel__';
  host.style.cssText='all:initial!important;position:fixed!important;top:16px!important;right:16px!important;z-index:2147483647!important;display:block!important;visibility:visible!important';
  const root=host.attachShadow({mode:'closed'}),style=document.createElement('style');
  style.textContent=`
   :host{color-scheme:light dark}*{box-sizing:border-box}section{color-scheme:light;--bg:#fff;--fg:#202024;--muted:#6b6b73;--line:#dedee3;--button:#f1f1f3;background:var(--bg);color:var(--fg);font:12px/1.5 system-ui,-apple-system,sans-serif;width:min(320px,calc(100vw - 32px));border:1px solid var(--line);border-radius:12px;box-shadow:0 8px 32px #0003;overflow:hidden;max-height:calc(100vh - 32px);display:flex;flex-direction:column}
   @media(prefers-color-scheme:dark){section{color-scheme:dark;--bg:#242427;--fg:#eeeef0;--muted:#a0a0aa;--line:#46464e;--button:#35353b}}
   header{flex-shrink:0;display:flex;align-items:center;gap:8px;padding:10px 12px;cursor:move;touch-action:none;user-select:none}strong{font-size:12px;font-weight:600;flex:1}button,input,select{font:inherit;color:inherit}button{border:1px solid var(--line);border-radius:6px;background:var(--button);padding:5px 9px;cursor:pointer}button:disabled{opacity:.5;cursor:default}button:focus-visible,input:focus-visible,select:focus-visible{outline:2px solid var(--muted);outline-offset:2px}.toggle,.delete{padding:1px 6px;background:transparent}.body{padding:0 12px 12px;min-height:0;overflow:auto}.actions{display:flex;gap:8px}.actions button{flex:1}ol{list-style:none;margin:0 0 10px;padding:0;max-height:210px;overflow:auto}li{display:flex;gap:7px;padding:7px 0;border-top:1px solid var(--line)}.step{min-width:0;flex:1}.name{display:block;font-weight:500}.record-dot{color:#e5484d}.target{display:block;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:var(--muted);font-size:11px}.delete{align-self:center}.note{color:var(--muted);font-size:11px;margin:0 0 10px}.capture{margin:0 0 10px;padding:10px 0;border-top:1px solid var(--line)}label{display:block;margin:6px 0}input,select{width:100%;border:1px solid var(--line);border-radius:5px;background:var(--bg);padding:5px}p{margin:0 0 8px}.error{font-size:11px;margin-top:8px}[hidden]{display:none!important}`;
  root.append(style);
  const make=(tag,text,cls)=>{const el=document.createElement(tag);if(text)el.textContent=text;if(cls)el.className=cls;return el};
  const button=(text,action,cls)=>{const el=make('button',text,cls);el.type='button';el.addEventListener('click',ev=>{if(ev.isTrusted){ev.preventDefault();action(ev)}});return el};
  const box=make('section');box.setAttribute('aria-label',t('myco 录制面板','myco recorder'));
  const header=make('header'),title=make('strong','myco'),toggle=button('−',()=>{collapsed=!collapsed;render()},'toggle');header.append(title,toggle);box.append(header);
  const body=make('div',null,'body'),list=make('ol'),note=make('p',null,'note'),capture=make('div',null,'capture'),chosen=make('p');body.append(list,note,capture);capture.append(chosen);box.append(body);
  const whatLabel=make('label',t('抓取内容','Capture')),what=make('select');what.setAttribute('aria-label',t('抓取内容','Capture'));
  for(const [value,name]of [['text',t('文本','Text')],['attribute',t('属性','Attribute')],['html','HTML']]){const o=make('option',name);o.value=value;what.append(o)}whatLabel.append(what);capture.append(whatLabel);
  const attrLabel=make('label',t('属性名','Attribute name')),attr=make('input');attr.maxLength=160;attrLabel.append(attr);capture.append(attrLabel);
  const intoLabel=make('label',t('变量名','Variable name')),into=make('input');into.maxLength=64;into.pattern='[A-Za-z_][A-Za-z0-9_]{0,63}';into.required=true;intoLabel.append(into);capture.append(intoLabel);
  what.addEventListener('change',()=>{attrLabel.hidden=what.value!=='attribute'});
  const save=button(t('添加抓取步骤','Add extraction'),()=>{if(!into.reportValidity()||(what.value==='attribute'&&!attr.value.trim()))return;command('extract',{what:what.value,attr:what.value==='attribute'?attr.value.trim():'',into:into.value})});
  const cancel=button(t('取消','Cancel'),()=>command('cancel-pick')),formActions=make('div',null,'actions');formActions.append(save,cancel);capture.append(formActions);
  const actions=make('div',null,'actions'),start=button(t('开始录制','Start recording'),()=>command('start')),pick=button(t('抓取','Capture'),()=>{state.flush();command('pick')}),stop=button(t('结束录制','Stop recording'),()=>{state.flush();command('stop')});start.dataset.action='start';pick.dataset.action='pick';stop.dataset.action='stop';save.dataset.action='extract';cancel.dataset.action='cancel-pick';toggle.dataset.action='collapse';actions.append(start,pick,stop);body.append(actions);
  const error=make('p',null,'error');error.setAttribute('role','status');body.append(error);root.append(box);document.documentElement.append(host);
  const types={click:['↖',t('点击','Click')],type:['⌨',t('输入','Type')],navigate:['↗',t('打开网址','Navigate')],wait:['◷',t('等待','Wait')],extract:['▤',t('抓取','Extract')],press:['⌨',t('按键','Key')],select:['≡',t('选择','Select')],scroll:['↕',t('滚动','Scroll')],tab:['▣',t('标签页','Tab')],drag:['↔',t('拖动','Drag')],upload:['↑',t('上传','Upload')],dialog:['▢',t('对话框','Dialog')]};
  render=()=>{
   const steps=model.steps||[];title.replaceChildren();if(model.recording){title.append(make('span','● ','record-dot'),document.createTextNode(t(`录制中 · ${steps.length} 步`,`Recording · ${steps.length} steps`)))}else title.textContent='myco';
   toggle.textContent=collapsed?'+':'−';toggle.setAttribute('aria-label',collapsed?t('展开','Expand'):t('收起','Collapse'));toggle.setAttribute('aria-expanded',String(!collapsed));body.hidden=collapsed;
   const stepKey=JSON.stringify(steps);
   if(stepKey!==renderedSteps){renderedSteps=stepKey;list.replaceChildren();
   // The whole bounded buffer is available, recent-first, so any step can be deleted.
   for(const step of [...steps].reverse()){
    const row=make('li'),info=make('div',null,'step'),[icon,name]=types[step.type]||['·',step.type];const extractName=step.type==='extract'&&step.into?name+' '+({text:t('文本','text'),attribute:t('属性','attribute'),html:'HTML'}[step.what]||'')+' → '+step.into:name;info.append(make('span',icon+' '+(step.title||extractName),'name'),make('span',step.target||'—','target'));
    const del=button('×',()=>command('remove',{id:step.id}),'delete');del.dataset.stepId=step.id;del.setAttribute('aria-label',t('删除步骤','Delete step')+' '+(step.title||name));row.append(info,del);list.append(row);
   }
   }
   list.hidden=!steps.length;note.textContent=model.picking?t('点击网页中的元素以抓取。按 Esc 取消。','Click an element to capture it. Esc cancels.'):model.recording?t('在网页里操作，步骤会显示在这里。','Interact with the page; steps appear here.'):t('录制这个浏览器里的操作。','Record your actions in this browser.');
   capture.hidden=!model.selected;if(model.selected!==selectedKey){selectedKey=model.selected||'';into.value=model.next_variable||'data1';what.value='text';attr.value=''}chosen.textContent=model.selected||'';attrLabel.hidden=what.value!=='attribute';
   start.hidden=!!model.recording;pick.hidden=stop.hidden=!model.recording;start.disabled=!!model.busy&&!model.recording;pick.disabled=!!model.picking||!!model.selected;
   error.textContent=model.error?t('操作未完成：','Could not complete: ')+model.error:'';error.hidden=!model.error;
  };
  let drag=null;header.addEventListener('pointerdown',ev=>{if(!ev.isTrusted||ev.composedPath().some(el=>el.localName==='button'))return;const r=host.getBoundingClientRect();drag={x:ev.clientX-r.left,y:ev.clientY-r.top};header.setPointerCapture(ev.pointerId);ev.preventDefault()});
  header.addEventListener('pointermove',ev=>{if(!drag)return;const r=host.getBoundingClientRect();host.style.setProperty('left',Math.max(0,Math.min(innerWidth-r.width,ev.clientX-drag.x))+'px','important');host.style.setProperty('top',Math.max(0,Math.min(innerHeight-r.height,ev.clientY-drag.y))+'px','important');host.style.setProperty('right','auto','important')});header.addEventListener('pointerup',()=>{drag=null});header.addEventListener('pointercancel',()=>{drag=null});render();
 };
 const reconcile=next=>{
  model=next;
  if(next.hide){if(host)host.remove();return;}
  if(!document.documentElement)return;
  if(!host)mount();else if(!host.isConnected)document.documentElement.append(host);
  render();
 };
 state.renderPanel=reconcile;
 if(!document.documentElement){const observer=new MutationObserver(()=>{if(document.documentElement){observer.disconnect();if(model)reconcile(model)}});observer.observe(document,{childList:true,subtree:true})}
})();
