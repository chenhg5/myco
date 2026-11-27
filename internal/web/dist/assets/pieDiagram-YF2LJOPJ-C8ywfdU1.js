import{p as N}from"./chunk-TMUBEWPD-CjW3MztF.js";import{g as B,s as U,a as K,b as V,n as Z,o as j,_ as i,l as C,c as q,C as H,G as J,I as Q,K as z,e as X,v as Y,L as tt,D as et}from"./mermaid.core-X8a6I0Is.js";import{p as at}from"./gitGraph-YCYPL57B-BYoZFqHR.js";import{o as rt}from"./d3-scale-35hnzshu.js";import"./index-CWOnPw2g.js";import"./pages-4sCTsh9E.js";import"./app-COyYLHT0.js";import"./min-D5ubI0tt.js";import"./_baseUniq-BWpP6Ij6.js";var it=et.pie,D={sections:new Map,showData:!1},m=D.sections,w=D.showData,st=structuredClone(it),ot=i(()=>structuredClone(st),"getConfig"),nt=i(()=>{m=new Map,w=D.showData,Y()},"clear"),lt=i(({label:t,value:a})=>{m.has(t)||(m.set(t,a),C.debug(`added new section: ${t}, with value: ${a}`))},"addSection"),ct=i(()=>m,"getSections"),pt=i(t=>{w=t},"setShowData"),dt=i(()=>w,"getShowData"),G={getConfig:ot,clear:nt,setDiagramTitle:j,getDiagramTitle:Z,setAccTitle:V,getAccTitle:K,setAccDescription:U,getAccDescription:B,addSection:lt,getSections:ct,setShowData:pt,getShowData:dt},gt=i((t,a)=>{N(t,a),a.setShowData(t.showData),t.sections.map(a.addSection)},"populateDb"),ut={parse:i(async t=>{const a=await at("pie",t);C.debug(a),gt(a,G)},"parse")},ft=i(t=>`
  .pieCircle{
    stroke: ${t.pieStrokeColor};
    stroke-width : ${t.pieStrokeWidth};
    opacity : ${t.pieOpacity};
  }
  .pieOuterCircle{
    stroke: ${t.pieOuterStrokeColor};
    stroke-width: ${t.pieOuterStrokeWidth};
    fill: none;
  }
  .pieTitleText {
    text-anchor: middle;
    font-size: ${t.pieTitleTextSize};
    fill: ${t.pieTitleTextColor};
    font-family: ${t.fontFamily};
  }
  .slice {
    font-family: ${t.fontFamily};
    fill: ${t.pieSectionTextColor};
    font-size:${t.pieSectionTextSize};
    // fill: white;
  }
  .legend text {
    fill: ${t.pieLegendTextColor};
    font-family: ${t.fontFamily};
    font-size: ${t.pieLegendTextSize};
  }
`,"getStyles"),mt=ft,ht=i(t=>{const a=[...t.entries()].map(s=>({label:s[0],value:s[1]})).sort((s,n)=>n.value-s.value);return tt().value(s=>s.value)(a)},"createPieArcs"),vt=i((t,a,F,s)=>{C.debug(`rendering pie chart
`+t);const n=s.db,$=q(),y=H(n.getConfig(),$.pie),T=40,o=18,d=4,c=450,h=c,v=J(a),l=v.append("g");l.attr("transform","translate("+h/2+","+c/2+")");const{themeVariables:r}=$;let[A]=Q(r.pieOuterStrokeWidth);A??(A=2);const _=y.textPosition,g=Math.min(h,c)/2-T,W=z().innerRadius(0).outerRadius(g),I=z().innerRadius(g*_).outerRadius(g*_);l.append("circle").attr("cx",0).attr("cy",0).attr("r",g+A/2).attr("class","pieOuterCircle");const b=n.getSections(),S=ht(b),L=[r.pie1,r.pie2,r.pie3,r.pie4,r.pie5,r.pie6,r.pie7,r.pie8,r.pie9,r.pie10,r.pie11,r.pie12],p=rt(L);l.selectAll("mySlices").data(S).enter().append("path").attr("d",W).attr("fill",e=>p(e.data.label)).attr("class","pieCircle");let E=0;b.forEach(e=>{E+=e}),l.selectAll("mySlices").data(S).enter().append("text").text(e=>(e.data.value/E*100).toFixed(0)+"%").attr("transform",e=>"translate("+I.centroid(e)+")").style("text-anchor","middle").attr("class","slice"),l.append("text").text(n.getDiagramTitle()).attr("x",0).attr("y",-400/2).attr("class","pieTitleText");const x=l.selectAll(".legend").data(p.domain()).enter().append("g").attr("class","legend").attr("transform",(e,u)=>{const f=o+d,O=f*p.domain().length/2,P=12*o,R=u*f-O;return"translate("+P+","+R+")"});x.append("rect").attr("width",o).attr("height",o).style("fill",p).style("stroke",p),x.data(S).append("text").attr("x",o+d).attr("y",o-d).text(e=>{const{label:u,value:f}=e.data;return n.getShowData()?`${u} [${f}]`:u});const M=Math.max(...x.selectAll("text").nodes().map(e=>(e==null?void 0:e.getBoundingClientRect().width)??0)),k=h+T+o+d+M;v.attr("viewBox",`0 0 ${k} ${c}`),X(v,c,k,y.useMaxWidth)},"draw"),St={draw:vt},bt={parser:ut,db:G,renderer:St,styles:mt};export{bt as diagram};
