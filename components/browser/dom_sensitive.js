function isSensitive(element, descendants=false) {
  const policy=__MYCO_SENSITIVE_CONFIG__;
  const namePattern=new RegExp(policy.name,'i'), autoPattern=new RegExp(policy.autocomplete,'i');
  const parent=n=>n?.parentElement||n?.getRootNode?.().host||null;
  const own=n=>!!n&&n.nodeType===1&&(String(n.type||n.getAttribute?.('type')||'').toLowerCase()==='password'||n.hasAttribute?.('data-sensitive')||namePattern.test(n.name||n.getAttribute?.('name')||'')||autoPattern.test(n.autocomplete||n.getAttribute?.('autocomplete')||''));
  for(let n=element;n;n=parent(n))if(own(n))return true;
  if(!descendants)return false;
  const seen=new Set(),pending=[element];let count=0;
  while(pending.length){
    const node=pending.pop();if(!node||seen.has(node))continue;seen.add(node);
    if(++count>50000||own(node))return true;
    for(const child of node.children||[])pending.push(child);
    if(node.shadowRoot)pending.push(node.shadowRoot);
    for(const label of node.labels||[])pending.push(label);
    for(const id of (node.getAttribute?.('aria-labelledby')||'').split(/\s+/))if(id)pending.push(node.ownerDocument?.getElementById(id));
  }
  return false;
}
