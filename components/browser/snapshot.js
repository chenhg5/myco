() => {
  const nodes=[],seen=new Set();let unsafe=false;
  function walk(root,parent,depth) {
    if(!root||depth>40||nodes.length>=2000||seen.has(root))return;
    seen.add(root);
    for(const e of root.children||[]) {
      if(nodes.length>=2000)break;
      const i=nodes.length;
      nodes.push({tag:e.localName,role:e.getAttribute('role')||'',parent});
      if(e.type==='password'||/password|one-time-code/i.test(e.autocomplete||'')||e.hasAttribute('data-sensitive'))unsafe=true;
      if(e.localName==='iframe'||e.localName==='frame') {
        try {if(e.contentDocument)walk(e.contentDocument,i,depth+1);else unsafe=true;}catch(_){unsafe=true;}
      }
      if(e.shadowRoot)walk(e.shadowRoot,i,depth+1);
      walk(e,i,depth+1);
    }
  }
  walk(document,-1,0);
  return {nodes,unsafe};
}
