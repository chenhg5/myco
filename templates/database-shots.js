// In-memory database computer for screenshots and browser interaction tests only.
// Production templates never include this fixture or its example connections.
(() => {
  const engines=['postgres','mysql','sqlite','mongodb'];
  const connections=engines.map((engine,i)=>({id:'db_'+engine,name:['开发数据库','订单服务 · MySQL','本机资料库','文档仓库'][i],engine,host:'127.0.0.1',port:[5432,3306,0,27017][i],user:'developer',database:'studio',path:engine==='sqlite'?'/Users/designer/Projects/studio.db':'',tls:'disable',auth_source:'admin',read_only:false,has_password:engine!=='sqlite',has_uri:false,version:1}));
  const columns=[{name:'id',type:'bigint',primary:true,nullable:false,default:'auto',generated:false},{name:'name',type:'varchar(120)',primary:false,nullable:false,default:'',generated:false},{name:'category',type:'varchar(60)',primary:false,nullable:true,default:'',generated:false},{name:'price',type:'numeric(12,2)',primary:false,nullable:true,default:'',generated:false},{name:'stock',type:'integer',primary:false,nullable:true,default:'',generated:false},{name:'updated_at',type:'timestamp',primary:false,nullable:true,default:'now()',generated:false}];
  let counter=12;const names=['亚麻笔记本','陶瓷咖啡杯','桌面收纳盒','黄铜书签','帆布手提袋','玻璃花瓶','原木笔筒','不锈钢水杯','棉质桌布','便携台灯','香薰蜡烛','软木杯垫'];
  const cell=(kind,value)=>({kind,value:String(value)});
  let rows=names.map((name,i)=>({values:[cell('number',i+1),cell('text',name),cell('text',['文具','生活','办公'][i%3]),cell('number',['38.00','68.00','128.00','24.00','89.00','159.00'][i%6]),cell('number',[240,128,76,480,196,68][i%6]),cell('text','2026-11-23 14:32:10')],key:{id:cell('number',i+1)},token:'row-'+i}));
  window.__DB_CALLS=[];window.__DB_profiles=connections;window.__DB_rows=()=>rows;
  const fail=(code,message)=>{throw Object.assign(new Error(message),{code})};
  window.__FAKE_OS__={list:async()=>({os:'darwin',arch:'arm64',caps:['profile','catalog','query','rows','mongo','cancel'].map(n=>({name:'db.'+n,available:true}))}),fs:{pick:async()=>({paths:['/Users/designer/Projects/studio.db']})},call:async(name,a={})=>{
    window.__DB_CALLS.push({name,args:structuredClone(a)});
    if(name==='db.profile'){
      if(a.action==='list')return structuredClone(connections);
      if(a.action==='test'){if(window.__DB_testFail)fail('failed','Connection refused');return{connected:true}}
      if(a.action==='forget'){const i=connections.findIndex(p=>p.id===a.id);if(i>=0)connections.splice(i,1);return true}
      if(a.action==='save'){const p={...a.profile};const old=connections.find(x=>x.id===p.id);if(old&&p.version!==old.version)fail('conflict','Connection changed');p.id=p.id||'db_new_'+Date.now();p.version=(old?.version||0)+1;p.has_password=p.password!==undefined?!!p.password:!!old?.has_password;delete p.password;delete p.uri;const i=connections.findIndex(x=>x.id===p.id);i<0?connections.push(p):connections.splice(i,1,p);return structuredClone(p)}
    }
    if(name==='db.catalog'){
      if(a.action==='databases')return ['studio','analytics','postgres'];
      if(a.action==='tables')return [{schema:'public',name:'products',kind:'table'},{schema:'public',name:'orders',kind:'table'},{schema:'public',name:'customers',kind:'table'},{schema:'public',name:'inventory',kind:'table'},{schema:'public',name:'events',kind:'table'},{schema:'public',name:'weekly_sales',kind:'view'}];
    }
    if(name==='db.cancel'){window.__DB_canceled=true;return{cancel_requested:true}}
    const profile=connections.find(p=>p.id===a.connection),out=()=>({columns:structuredClone(columns),rows:structuredClone(rows),read_only:profile?.read_only||a.table==='weekly_sales',truncated:false,duration_ms:12});
    if(name==='db.query'){
      if(window.__DB_slow){window.__DB_canceled=false;await new Promise(resolve=>{const timer=setInterval(()=>{if(window.__DB_canceled){clearInterval(timer);resolve()}},30)});fail('cancelled','Query cancelled')}
      if(a.sql.includes('BAD SQL'))fail('failed','SQL syntax error');return a.mode==='execute'?{columns:[],rows:[],affected:'1',truncated:false,read_only:false,duration_ms:8}:out();
    }
    if(name==='db.rows'){
      if(a.action==='browse'){const value=out();value.truncated=window.__DB_nextPage||false;return value}
      if(a.action==='insert'){const values=columns.map(c=>a.values[c.name]||cell(c.type==='bigint'?'number':'text',c.name==='id'?++counter:''));rows.push({values,key:{id:values[0]},token:'new-'+counter});}
      else {const i=rows.findIndex(r=>r.key.id.value===a.key?.id.value);if(i<0||rows[i].token!==a.token||window.__DB_conflict)fail('conflict','This row changed; reload it first');if(a.action==='delete')rows.splice(i,1);else{rows[i].values=columns.map((c,j)=>a.values[c.name]||rows[i].values[j]);rows[i].token+='-updated'}}
      return{columns:[],rows:[],affected:'1',truncated:false,read_only:false,duration_ms:4};
    }
    if(name==='db.mongo'){
      const docs=rows.map(r=>{const document=JSON.stringify({_id:{$oid:'507f1f77bcf86cd7994390'+String(r.values[0].value).padStart(2,'0')},name:r.values[1].value,price:{$numberDecimal:r.values[3].value}});return{document,token:r.token,values:[cell('text',r.values[0].value),cell('json',document)]}});
      return{columns:[{name:'_id',type:'ObjectID',primary:true},{name:'document',type:'Extended JSON'}],rows:docs,read_only:false,truncated:false,duration_ms:9};
    }
    fail('unsupported','Unsupported fixture operation');
  }};
})();
