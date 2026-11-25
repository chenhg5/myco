// Recorder UI adapted from BrowserWing recorder.js and float_button.js.
// Copyright (c) 2025 BrowserWing contributors. MIT: NOTICE-BrowserWing.txt.
// State and commands remain in myco's isolated world; the site cannot access
// the binding, closed root, recording ID or local capture controls.
(() => {
  const state = window.__mycoRecorder;
  if (!state || state.panelInstalled) return;
  state.panelInstalled = true;
  if (window !== window.top) return;
  const zh = /^zh\b/i.test(navigator.language || '');
  const t = (cn, en) => zh ? cn : en;
  let model, host, render = () => {}, collapsed = false;
  let localTarget = null, menuTarget = null, pendingDelete = null;
  let selectedKey = '', renderedSteps = '', previousID = '', previousStep = '';
  let noticeTimer, menu, removeDialog, highlight, highlightLabel, region, regionDrag, closeRegion = () => {};
  const command = (action, fields = {}) => {
    if (!model) return;
    window.__mycoBrowserEvent(JSON.stringify({type:'panel', action, recording_id:model.id, ...fields}));
  };
  state.ignoreEvent = ev => !!host && (ev.composedPath().includes(host) ||
    (ev.type === 'keydown' && ((menu && !menu.hidden) || pendingDelete || (region && !region.hidden))));
  const short = (value, limit = 110) => String(value || '').slice(0, limit);
  const summary = target => {
    const locator = target?.locators?.find(l => l.name || l.value);
    return short(locator?.name || locator?.value || '');
  };
  const mount = () => {
    if (host || !document.documentElement) return;
    host = document.createElement('div'); host.id = '__myco_recording_panel__';
    host.style.cssText = 'all:initial!important;position:fixed!important;top:20px!important;right:20px!important;z-index:2147483647!important;display:block!important;visibility:visible!important;pointer-events:none!important';
    const root = host.attachShadow({mode:'closed'}), style = document.createElement('style');
    style.textContent = `
      :host{color-scheme:light dark}*{box-sizing:border-box}button,input,select{font:inherit;color:inherit}button{cursor:pointer}button:focus-visible,input:focus-visible,select:focus-visible{outline:2px solid #6366f1;outline-offset:3px}[hidden]{display:none!important}
      section,.menu,.highlight-label{--bg:#fff;--end:#fafbfc;--text:#0f172a;--muted:#64748b;--line:rgba(0,0,0,.06);--card:#fff;--hover:#f1f5f9;--subtle:rgba(248,250,252,.8);--shadow:0 8px 32px rgba(0,0,0,.08),0 2px 8px rgba(0,0,0,.04)}
      @media(prefers-color-scheme:dark){section,.menu,.highlight-label{color-scheme:dark;--bg:#202127;--end:#191a20;--text:#f1f5f9;--muted:#a0a9b8;--line:rgba(255,255,255,.09);--card:#272930;--hover:#32353e;--subtle:rgba(30,32,40,.8);--shadow:0 8px 32px #0006,0 2px 8px #0004}}
      section{pointer-events:auto;width:min(360px,calc(100vw - 40px));max-height:calc(100vh - 40px);display:flex;flex-direction:column;background:linear-gradient(135deg,var(--bg),var(--end));color:var(--text);border:1px solid var(--line);border-radius:16px;box-shadow:var(--shadow);overflow:hidden;backdrop-filter:blur(10px);font:13px/1.5 -apple-system,BlinkMacSystemFont,'Segoe UI','SF Pro Display',Helvetica,Arial,sans-serif;letter-spacing:-.01em}
      section.idle{width:min(320px,calc(100vw - 40px))}header{flex-shrink:0;display:flex;align-items:center;gap:10px;padding:18px 20px 16px;border-bottom:1px solid var(--line);cursor:move;touch-action:none;user-select:none}strong{font-size:15px;font-weight:600;flex:1}.record-dot{width:10px;height:10px;flex-shrink:0;border-radius:50%;background:#ef4444;box-shadow:0 0 8px #ef444466;animation:record-pulse 2s cubic-bezier(.4,0,.6,1) infinite}.count{font-size:13px;font-weight:600;color:var(--muted);background:#64748b14;padding:4px 10px;border-radius:8px;white-space:nowrap}.toggle{border:0;background:transparent;color:var(--muted);font-size:18px;padding:0 3px;line-height:1.2}
      .body{display:flex;flex-direction:column;min-height:0;overflow:auto}.toolbar{padding:16px 20px;display:flex;gap:10px;border-bottom:1px solid var(--line);flex-shrink:0}.tool{flex:1;padding:8px 14px;background:#18181b;color:#fff;border:1.5px solid #ffffff1a;border-radius:10px;font-size:13px;font-weight:600;box-shadow:0 2px 8px #0003;transition:all .25s cubic-bezier(.4,0,.2,1)}.tool:hover{background:#27272a;transform:translateY(-1px);box-shadow:0 4px 12px #0004}.tool:active{transform:translateY(0)}button:disabled{opacity:.5;cursor:default;transform:none}
      ol{list-style:none;margin:0;padding:12px;max-height:280px;overflow:auto;min-height:0;flex-shrink:1}li{padding:12px 14px;margin:6px 0;background:var(--card);border:1px solid var(--line);border-radius:10px;display:flex;align-items:flex-start;gap:8px;box-shadow:0 1px 2px #0000000a;animation:step-in .2s ease-out;transition:background .25s,border-color .25s}li:hover{background:var(--hover);border-color:#64748b4d}.step{min-width:0;flex:1}.name{display:block;font-weight:700;font-size:13px}.target{display:block;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:var(--muted);font:11px/1.7 ui-monospace,SFMono-Regular,Menlo,monospace;margin-top:4px}.number{color:var(--muted);font-size:11px;margin-right:7px}.delete{border:0;background:transparent;color:var(--muted);border-radius:5px;padding:0 4px;font-size:18px;line-height:1.2}.delete:hover{color:#ef4444;background:#ef444414}.empty{padding:32px 24px;text-align:center;color:var(--muted);font-size:13px}.current{padding:14px 20px;background:var(--subtle);border-top:1px solid var(--line);font-size:12px;color:var(--muted);flex-shrink:0}.footer{padding:16px 20px 20px;border-top:1px solid var(--line);flex-shrink:0}.idle .footer{padding:20px 24px 24px}.primary{width:100%;padding:14px 20px;background:linear-gradient(135deg,#ef4444,#dc2626);color:white;border:0;border-radius:12px;font-size:14px;font-weight:600;box-shadow:0 4px 12px #ef444440,0 2px 4px #0000001a;transition:all .25s cubic-bezier(.4,0,.2,1)}.primary:hover{background:linear-gradient(135deg,#dc2626,#b91c1c);transform:translateY(-2px);box-shadow:0 6px 20px #ef444459,0 4px 8px #00000026}.primary:active{transform:translateY(0)}.start-dot{display:inline-block;width:10px;height:10px;border-radius:50%;background:white;margin-right:10px;box-shadow:0 0 8px #fff6}
      .capture,.delete-confirm{padding:16px 20px;border-top:1px solid var(--line);background:var(--subtle);flex-shrink:0}.capture p,.delete-confirm p{margin:0 0 10px;overflow-wrap:anywhere}label{display:block;margin:8px 0;color:var(--muted);font-size:12px}input,select{display:block;width:100%;border:1px solid var(--line);border-radius:8px;background:var(--card);padding:8px 10px;margin-top:5px;color:var(--text)}.actions{display:flex;gap:10px;margin-top:12px}.actions button{flex:1;border:1px solid var(--line);border-radius:8px;background:var(--card);padding:8px 12px}.actions .confirm{background:#18181b;color:white}.error{margin:0;padding:8px 20px 12px;color:#ef4444;font-size:12px}
      .menu{pointer-events:auto;position:fixed;background:var(--bg);color:var(--text);border:1px solid var(--line);border-radius:12px;box-shadow:0 8px 24px #0003,0 2px 8px #0001;padding:6px;min-width:180px;max-width:calc(100vw - 20px);font:600 13px/1.5 -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;backdrop-filter:blur(10px)}.menu button{display:block;width:100%;border:0;text-align:left;padding:10px 14px;background:transparent;border-radius:8px}.menu button:hover{background:var(--hover)}
      .highlight{pointer-events:none;position:fixed;border:2px solid #82dee4;border-radius:4px;box-shadow:0 0 0 2px #77c0fc63;transition:all .15s cubic-bezier(.4,0,.2,1)}.highlight-label{pointer-events:none;position:fixed;background:linear-gradient(135deg,#1e293b,#0f172a);color:#94a3b8;border:1px solid #ffffff1a;border-radius:6px;padding:6px 12px;box-shadow:0 4px 12px #0005;font:600 11px/1.4 ui-monospace,SFMono-Regular,Menlo,monospace;max-width:min(500px,calc(100vw - 20px));overflow-wrap:anywhere;white-space:pre-line}.highlight-label b{display:block;color:#60a5fa;font-size:12px;margin-bottom:2px}
      @keyframes record-pulse{0%,100%{opacity:1}50%{opacity:.5}}@keyframes step-in{from{opacity:0;transform:translateY(5px)}to{opacity:1;transform:none}}@media(prefers-reduced-motion:reduce){*{animation:none!important;transition:none!important}}
    `;
    style.textContent += `.region{pointer-events:auto;position:fixed;inset:0;width:100vw;height:100vh;background:#0005;cursor:crosshair;touch-action:none;outline:none}.region-box{position:fixed;border:2px dashed #6366f1;background:#6366f11a;pointer-events:none}.region-hint{position:fixed;top:10px;left:50%;transform:translateX(-50%);background:white;color:#0f172a;padding:8px 16px;border-radius:6px;box-shadow:0 2px 8px #0003;font:13px/1.5 system-ui;pointer-events:none}`;
    root.append(style);
    const make = (tag, text, cls) => {const el=document.createElement(tag);if(text)el.textContent=text;if(cls)el.className=cls;return el;};
    const button = (text, action, cls) => {
      const el=make('button',text,cls);el.type='button';
      el.addEventListener('click',ev=>{if(ev.isTrusted){ev.preventDefault();action(ev);}});return el;
    };
    const hideHighlight = () => {highlight.hidden=highlightLabel.hidden=true;};
    const box=make('section'),header=make('header'),dot=make('span',null,'record-dot');
    box.setAttribute('aria-label',t('myco 录制面板','myco recorder'));
    const title=make('strong'),count=make('span',null,'count');
    const toggle=button('−',()=>{collapsed=!collapsed;menu.hidden=true;render();},'toggle');toggle.dataset.action='collapse';
    header.append(dot,title,count,toggle);box.append(header);
    const body=make('div',null,'body'),toolbar=make('div',null,'toolbar');
    const pick=button(t('抓取数据 ▾','Extract data ▾'),ev=>{state.flush();openMenu(null,ev.currentTarget.getBoundingClientRect());},'tool');pick.dataset.action='pick';
    const screenshot=button(t('区域截图','Region screenshot'),()=>{state.flush();menu.hidden=true;hideHighlight();regionHint.textContent=t('拖动选择区域，按 Esc 取消','Drag to select a region. Esc cancels.');region.hidden=false;box.hidden=true;region.focus();},'tool');screenshot.dataset.action='region';
    toolbar.append(pick,screenshot);body.append(toolbar);
    const list=make('ol'),empty=make('div',t('开始操作网页，步骤会显示在这里','Interact with the page to record your steps'),'empty');body.append(list,empty);
    const current=make('div',null,'current');current.setAttribute('role','status');current.hidden=true;
    const showAction=text=>{clearTimeout(noticeTimer);current.textContent=text;current.hidden=!text;noticeTimer=setTimeout(()=>{current.hidden=true;},3000);};
    const capture=make('div',null,'capture'),chosen=make('p');capture.append(chosen);body.append(capture);
    const whatLabel=make('label',t('抓取内容','Capture')),what=make('select');what.setAttribute('aria-label',t('抓取内容','Capture'));
    for(const [value,name]of [['text',t('文本','Text')],['attribute',t('属性','Attribute')],['html','HTML']]){const option=make('option',name);option.value=value;what.append(option);}whatLabel.append(what);capture.append(whatLabel);
    const attrLabel=make('label',t('属性名','Attribute name')),attr=make('input');attr.maxLength=160;attrLabel.append(attr);capture.append(attrLabel);
    const intoLabel=make('label',t('变量名','Variable name')),into=make('input');into.maxLength=64;into.pattern='[A-Za-z_][A-Za-z0-9_]{0,63}';into.required=true;intoLabel.append(into);capture.append(intoLabel);
    what.addEventListener('change',()=>{attrLabel.hidden=what.value!=='attribute';});
    const save=button(t('添加抓取步骤','Add extraction'),()=>{
      if(!into.reportValidity()||(what.value==='attribute'&&!attr.value.trim()))return;
      const fields={what:what.value,attr:what.value==='attribute'?attr.value.trim():'',into:into.value};
      if(localTarget){command('extract-target',{...fields,target:localTarget});localTarget=null;render();}else command('extract',fields);
    },'confirm');save.dataset.action='extract';
    const cancel=button(t('取消','Cancel'),()=>{localTarget=null;command('cancel-pick');render();});cancel.dataset.action='cancel-pick';
    const captureActions=make('div',null,'actions');captureActions.append(save,cancel);capture.append(captureActions);
    removeDialog=make('div',null,'delete-confirm');removeDialog.setAttribute('role','alertdialog');removeDialog.setAttribute('aria-label',t('删除步骤','Delete step'));
    const removeText=make('p'),removeActions=make('div',null,'actions');
    const confirmRemove=button(t('删除','Delete'),()=>{const id=pendingDelete?.id;pendingDelete=null;render();if(id)command('remove',{id});showAction(t('步骤已删除','Step deleted'));},'confirm');confirmRemove.dataset.action='confirm-remove';
    const cancelRemove=button(t('取消','Cancel'),()=>{pendingDelete=null;render();});cancelRemove.dataset.action='cancel-remove';removeActions.append(cancelRemove,confirmRemove);removeDialog.append(removeText,removeActions);body.append(removeDialog);
    box.append(body,current);
    const footer=make('div',null,'footer'),start=button('',()=>command('start'),'primary'),stop=button(t('结束录制','Stop recording'),()=>{state.flush();command('stop');},'primary');
    start.append(make('span',null,'start-dot'),document.createTextNode(t('开始录制','Start recording')));start.dataset.action='start';stop.dataset.action='stop';footer.append(start,stop);box.append(footer);
    const error=make('p',null,'error');error.setAttribute('role','status');box.append(error);root.append(box);
    menu=make('div',null,'menu');menu.hidden=true;menu.setAttribute('role','menu');
    let pickedWhat='text';
    const openMenu=(target,rect)=>{
      menuTarget=target;menu.hidden=false;
      menu.style.left=Math.max(10,Math.min(innerWidth-menu.offsetWidth-10,rect.left))+'px';
      menu.style.top=Math.max(10,Math.min(innerHeight-menu.offsetHeight-10,rect.bottom+5))+'px';hideHighlight();
    };
    for(const [value,name]of [['text',t('抓取文本','Extract text')],['html',t('抓取 HTML','Extract HTML')],['attribute',t('抓取属性…','Extract attribute…')]]){
      const item=button(name,()=>{
        menu.hidden=true;pickedWhat=value;
        if(menuTarget){
          if(value==='attribute'){localTarget=menuTarget;selectedKey='';render();what.value=value;attrLabel.hidden=false;attr.focus();}
          else command('extract-target',{target:menuTarget,what:value});
        }else{localTarget=null;state.flush();command('pick');showAction(t('点击要抓取的网页元素，Esc 取消','Click an element to extract. Esc cancels.'));}
      });item.setAttribute('role','menuitem');item.dataset.action='capture-'+value;menu.append(item);
    }
    root.append(menu);
    highlight=make('div',null,'highlight');highlightLabel=make('div',null,'highlight-label');hideHighlight();root.append(highlight,highlightLabel);
    region=make('div',null,'region');region.hidden=true;region.tabIndex=-1;region.setAttribute('role','region');region.setAttribute('aria-label',t('选择截图区域','Select screenshot region'));
    const regionBox=make('div',null,'region-box'),regionHint=make('div',t('拖动选择区域，按 Esc 取消','Drag to select a region. Esc cancels.'),'region-hint');regionBox.hidden=true;region.append(regionBox,regionHint);root.append(region);
    closeRegion=()=>{if(regionDrag&&region.hasPointerCapture(regionDrag.pointer))region.releasePointerCapture(regionDrag.pointer);region.hidden=true;regionBox.hidden=true;regionDrag=null;box.hidden=false;};
    const regionPoint=ev=>({x:Math.max(0,Math.min(innerWidth,ev.clientX)),y:Math.max(0,Math.min(innerHeight,ev.clientY))});
    const regionRect=ev=>{const point=regionPoint(ev);return {x:Math.min(regionDrag.x,point.x),y:Math.min(regionDrag.y,point.y),width:Math.abs(point.x-regionDrag.x),height:Math.abs(point.y-regionDrag.y)};};
    region.addEventListener('pointerdown',ev=>{
      if(!ev.isTrusted||ev.button!==0||!model?.recording)return;
      ev.preventDefault();regionDrag={...regionPoint(ev),id:model.id,pointer:ev.pointerId};region.setPointerCapture(ev.pointerId);regionBox.hidden=false;Object.assign(regionBox.style,{left:regionDrag.x+'px',top:regionDrag.y+'px',width:'0px',height:'0px'});
    });
    region.addEventListener('pointermove',ev=>{
      if(!ev.isTrusted||!regionDrag||ev.pointerId!==regionDrag.pointer)return;
      const clip=regionRect(ev);Object.assign(regionBox.style,{left:clip.x+'px',top:clip.y+'px',width:clip.width+'px',height:clip.height+'px'});
      regionHint.textContent=Math.round(clip.width)+' × '+Math.round(clip.height)+' · '+t('Esc 取消','Esc cancels');
    });
    region.addEventListener('pointerup',ev=>{
      if(!ev.isTrusted||!regionDrag||ev.pointerId!==regionDrag.pointer)return;
      ev.preventDefault();const clip=regionRect(ev),id=regionDrag.id;closeRegion();
      if(model?.recording&&model.id===id&&clip.width>=10&&clip.height>=10)command('screenshot',{clip});
    });
    region.addEventListener('contextmenu',ev=>{if(ev.isTrusted)ev.preventDefault();});
    region.addEventListener('pointercancel',ev=>{if(ev.isTrusted)closeRegion();});
    window.addEventListener('resize',closeRegion);
    state.contextMenu=(ev,element)=>{
      if(!ev.isTrusted||!element||!model?.recording||state.picking||state.ignoreEvent(ev))return false;
      ev.preventDefault();ev.stopImmediatePropagation();
      openMenu(window.__mycoDescribe(element),{left:ev.clientX,bottom:ev.clientY});return true;
    };
    document.addEventListener('pointermove',ev=>{
      if(!ev.isTrusted||!model?.recording||model.hide||!menu.hidden||pendingDelete||!region.hidden||state.ignoreEvent(ev)){hideHighlight();return;}
      const element=ev.composedPath()[0];if(!element||element.nodeType!==1||element===document.body||element===document.documentElement){hideHighlight();return;}
      const rect=element.getBoundingClientRect();if(!rect.width||!rect.height){hideHighlight();return;}
      const target=window.__mycoDescribe(element);highlight.hidden=highlightLabel.hidden=false;
      Object.assign(highlight.style,{left:rect.left-3+'px',top:rect.top-3+'px',width:rect.width+6+'px',height:rect.height+6+'px'});
      const tag=make('b','<'+element.localName+'>');highlightLabel.replaceChildren(tag,document.createTextNode(summary(target)));
      highlightLabel.style.left=Math.max(10,Math.min(innerWidth-highlightLabel.offsetWidth-10,rect.left-3))+'px';
      highlightLabel.style.top=Math.max(10,Math.min(innerHeight-highlightLabel.offsetHeight-10,rect.top-highlightLabel.offsetHeight-8>10?rect.top-highlightLabel.offsetHeight-8:rect.bottom+8))+'px';
    },true);
    document.addEventListener('pointerdown',ev=>{if(ev.isTrusted&&!state.ignoreEvent(ev)){menu.hidden=true;hideHighlight();}},true);
    document.addEventListener('keydown',ev=>{if(ev.isTrusted&&ev.key==='Escape'){menu.hidden=true;pendingDelete=null;localTarget=null;closeRegion();hideHighlight();render();}},true);
    window.addEventListener('blur',hideHighlight);
    const types={click:['↖',t('点击','Click')],type:['⌨',t('输入','Type')],navigate:['↗',t('打开网址','Navigate')],wait:['◷',t('等待','Wait')],extract:['▤',t('抓取','Extract')],press:['⌨',t('按键','Key')],select:['≡',t('选择','Select')],scroll:['↕',t('滚动','Scroll')],tab:['▣',t('标签页','Tab')],drag:['↔',t('拖动','Drag')],upload:['↑',t('上传','Upload')],dialog:['▢',t('对话框','Dialog')],screenshot:['▧',t('截图','Screenshot')]};
    const stepName=step=>{const [icon,name]=types[step.type]||['·',step.type];return icon+' '+(step.title||(step.type==='screenshot'&&step.clip?name+' '+Math.round(step.clip.width)+' × '+Math.round(step.clip.height):'' )||(step.type==='extract'&&step.into?name+' '+({text:t('文本','text'),attribute:t('属性','attribute'),html:'HTML'}[step.what]||'')+' → '+step.into:name));};
    render=()=>{
      const steps=model.steps||[];
      if(previousID!==model.id){previousID=model.id;previousStep='';closeRegion();localTarget=null;pendingDelete=null;menu.hidden=true;selectedKey='';}
      if(!model.recording){closeRegion();localTarget=null;pendingDelete=null;menu.hidden=true;hideHighlight();current.hidden=true;clearTimeout(noticeTimer);}
      box.classList.toggle('idle',!model.recording);title.textContent=model.recording?t('正在录制','Recording'):t('myco 网页录制','myco web recorder');
      dot.hidden=count.hidden=!model.recording;count.textContent=t(`${steps.length} 步`,`${steps.length} steps`);
      toggle.textContent=collapsed?'+':'−';toggle.setAttribute('aria-label',collapsed?t('展开','Expand'):t('收起','Collapse'));toggle.setAttribute('aria-expanded',String(!collapsed));body.hidden=footer.hidden=collapsed;
      screenshot.hidden=!(model.features||[]).includes('screenshot_clip');screenshot.disabled=!!model.picking||!!model.selected||!!localTarget;toolbar.hidden=!model.recording;start.hidden=!!model.recording;stop.hidden=!model.recording;start.disabled=!!model.busy&&!model.recording;pick.disabled=!!model.picking||!!model.selected||!!localTarget;
      const stepKey=JSON.stringify(steps);
      if(stepKey!==renderedSteps){
        renderedSteps=stepKey;const wasNearEnd=list.scrollHeight-list.scrollTop-list.clientHeight<45;list.replaceChildren();
        for(const [index,step]of steps.entries()){
          const row=make('li'),info=make('div',null,'step'),name=make('span',null,'name');
          name.append(make('span','#'+(index+1),'number'),document.createTextNode(stepName(step)));info.append(name,make('span',step.clip?t(`区域 · x ${step.clip.x}, y ${step.clip.y}`,`Region · x ${step.clip.x}, y ${step.clip.y}`):step.target||'—','target'));
          const del=button('×',()=>{pendingDelete={id:step.id,name:stepName(step)};menu.hidden=true;render();confirmRemove.focus();},'delete');del.dataset.stepId=step.id;del.setAttribute('aria-label',t('删除步骤','Delete step')+' '+stepName(step));row.append(info,del);list.append(row);
        }
        const latest=steps.at(-1);
        if(latest&&latest.id!==previousStep){showAction(stepName(latest));if(wasNearEnd)list.scrollTop=list.scrollHeight;}
        previousStep=latest?.id||'';
      }
      list.hidden=!model.recording||!steps.length;empty.hidden=!model.recording||!!steps.length;
      const selected=localTarget?summary(localTarget):model.selected;
      capture.hidden=!model.recording||!selected;
      if(selected!==selectedKey){selectedKey=selected||'';into.value=model.next_variable||'data1';what.value=pickedWhat;attr.value='';}
      chosen.textContent=selected||'';attrLabel.hidden=what.value!=='attribute';
      removeDialog.hidden=!pendingDelete;removeText.textContent=pendingDelete?t('删除这一步？\n','Delete this step?\n')+pendingDelete.name:'';
      error.textContent=model.error?t('操作未完成：','Could not complete: ')+model.error:'';error.hidden=!model.error;
      if(collapsed)current.hidden=true;
    };
    let drag;
    header.addEventListener('pointerdown',ev=>{if(!ev.isTrusted||ev.composedPath().some(el=>el.localName==='button'))return;const rect=host.getBoundingClientRect();drag={x:ev.clientX-rect.left,y:ev.clientY-rect.top};header.setPointerCapture(ev.pointerId);ev.preventDefault();hideHighlight();});
    header.addEventListener('pointermove',ev=>{if(!ev.isTrusted||!drag)return;const rect=host.getBoundingClientRect();host.style.setProperty('left',Math.max(0,Math.min(innerWidth-rect.width,ev.clientX-drag.x))+'px','important');host.style.setProperty('top',Math.max(0,Math.min(innerHeight-rect.height,ev.clientY-drag.y))+'px','important');host.style.setProperty('right','auto','important');});
    header.addEventListener('pointerup',()=>{drag=null;});header.addEventListener('pointercancel',()=>{drag=null;});
    document.documentElement.append(host);render();
  };
  const reconcile=next=>{
    model=next;
    if(next.hide){closeRegion();if(host)host.remove();clearTimeout(noticeTimer);return;}
    if(!document.documentElement)return;
    if(!host)mount();else if(!host.isConnected)document.documentElement.append(host);
    render();
  };
  state.renderPanel=reconcile;
  if(!document.documentElement){const observer=new MutationObserver(()=>{if(document.documentElement){observer.disconnect();if(model)reconcile(model);}});observer.observe(document,{childList:true,subtree:true});}
})();
