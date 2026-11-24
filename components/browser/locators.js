// Returns matches, in stable locator order. Arguments are data, never JS source.
(t, all) => {
 let root=document;
 for(const selector of (t.shadow||[])){const host=root.querySelector(selector);if(!host||!host.shadowRoot)return all?[]:null;root=host.shadowRoot;}
 const normalize=s=>String(s||'').trim().replace(/\s+/g,' ');
 const role=e=>e.getAttribute('role')||({BUTTON:'button',A:'link',TEXTAREA:'textbox',SELECT:'combobox'}[e.tagName])||
   (e.tagName==='INPUT'?({checkbox:'checkbox',radio:'radio',button:'button',submit:'button',range:'slider'}[e.type]||'textbox'):e.isContentEditable?'textbox':'');
 const name=e=>{
  const by=e.getAttribute('aria-labelledby');
  if(by){const s=by.split(/\s+/).map(id=>document.getElementById(id)?.textContent||'').join(' ');if(s.trim())return normalize(s)}
  return normalize(e.getAttribute('aria-label')||[...(e.labels||[])].map(x=>x.textContent).join(' ')||e.getAttribute('title')||e.getAttribute('placeholder')||(['button','link'].includes(role(e))?e.innerText||e.textContent:''));
 };
 let ambiguous=[];
 for(const l of t.locators||[]){
  let found=[];
  try{
   if(l.kind==='css')found=[...root.querySelectorAll(l.value)];
   else if(l.kind==='xpath'&&root===document){let q=document.evaluate(l.value,document,null,XPathResult.ORDERED_NODE_ITERATOR_TYPE,null),n;while(n=q.iterateNext())if(n.nodeType===1)found.push(n);}
   else{
    const candidates=[...root.querySelectorAll('*')];
    found=candidates.filter(e=>{
     switch(l.kind){
      case 'testid':{const pair=/^(data-testid|data-test|data-qa)=(.*)$/.exec(l.value||'');return pair?e.getAttribute(pair[1])===pair[2]:e.getAttribute('data-testid')===l.value;}
      case 'role':return role(e)===l.role&&(!l.name||name(e)===normalize(l.name));
      case 'label':return [...(e.labels||[])].some(x=>normalize(x.textContent)===normalize(l.value));
      case 'placeholder':return e.getAttribute('placeholder')===l.value;
      case 'text':return normalize(e.innerText||e.textContent)===normalize(l.value)&&![...e.children].some(c=>normalize(c.innerText||c.textContent)===normalize(l.value));
      default:return false;
     }
    });
   }
  }catch(_){}
  found=[...new Set(found)];
  if(found.length===1){
    if(all)return found;
    const e=found[0];
    if(e.isConnected&&e.getBoundingClientRect().width>0&&e.getBoundingClientRect().height>0&&getComputedStyle(e).visibility!=='hidden'&&!e.disabled)return e;
  } else if(found.length&&!ambiguous.length)ambiguous=found;
 }
 return all?ambiguous:null;
}
