(function(){const e=document.createElement("link").relList;if(e&&e.supports&&e.supports("modulepreload"))return;for(const r of document.querySelectorAll('link[rel="modulepreload"]'))i(r);new MutationObserver(r=>{for(const o of r)if(o.type==="childList")for(const s of o.addedNodes)s.tagName==="LINK"&&s.rel==="modulepreload"&&i(s)}).observe(document,{childList:!0,subtree:!0});function n(r){const o={};return r.integrity&&(o.integrity=r.integrity),r.referrerPolicy&&(o.referrerPolicy=r.referrerPolicy),r.crossOrigin==="use-credentials"?o.credentials="include":r.crossOrigin==="anonymous"?o.credentials="omit":o.credentials="same-origin",o}function i(r){if(r.ep)return;r.ep=!0;const o=n(r);fetch(r.href,o)}})();var _g={exports:{}},rc={},xg={exports:{}},We={};/**
 * @license React
 * react.production.min.js
 *
 * Copyright (c) Facebook, Inc. and its affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */var da=Symbol.for("react.element"),ky=Symbol.for("react.portal"),Uy=Symbol.for("react.fragment"),Fy=Symbol.for("react.strict_mode"),Oy=Symbol.for("react.profiler"),zy=Symbol.for("react.provider"),By=Symbol.for("react.context"),Hy=Symbol.for("react.forward_ref"),Vy=Symbol.for("react.suspense"),Gy=Symbol.for("react.memo"),Wy=Symbol.for("react.lazy"),Wf=Symbol.iterator;function Xy(t){return t===null||typeof t!="object"?null:(t=Wf&&t[Wf]||t["@@iterator"],typeof t=="function"?t:null)}var Sg={isMounted:function(){return!1},enqueueForceUpdate:function(){},enqueueReplaceState:function(){},enqueueSetState:function(){}},Mg=Object.assign,wg={};function qo(t,e,n){this.props=t,this.context=e,this.refs=wg,this.updater=n||Sg}qo.prototype.isReactComponent={};qo.prototype.setState=function(t,e){if(typeof t!="object"&&typeof t!="function"&&t!=null)throw Error("setState(...): takes an object of state variables to update or a function which returns an object of state variables.");this.updater.enqueueSetState(this,t,e,"setState")};qo.prototype.forceUpdate=function(t){this.updater.enqueueForceUpdate(this,t,"forceUpdate")};function Eg(){}Eg.prototype=qo.prototype;function wh(t,e,n){this.props=t,this.context=e,this.refs=wg,this.updater=n||Sg}var Eh=wh.prototype=new Eg;Eh.constructor=wh;Mg(Eh,qo.prototype);Eh.isPureReactComponent=!0;var Xf=Array.isArray,Tg=Object.prototype.hasOwnProperty,Th={current:null},Ag={key:!0,ref:!0,__self:!0,__source:!0};function Cg(t,e,n){var i,r={},o=null,s=null;if(e!=null)for(i in e.ref!==void 0&&(s=e.ref),e.key!==void 0&&(o=""+e.key),e)Tg.call(e,i)&&!Ag.hasOwnProperty(i)&&(r[i]=e[i]);var a=arguments.length-2;if(a===1)r.children=n;else if(1<a){for(var l=Array(a),c=0;c<a;c++)l[c]=arguments[c+2];r.children=l}if(t&&t.defaultProps)for(i in a=t.defaultProps,a)r[i]===void 0&&(r[i]=a[i]);return{$$typeof:da,type:t,key:o,ref:s,props:r,_owner:Th.current}}function jy(t,e){return{$$typeof:da,type:t.type,key:e,ref:t.ref,props:t.props,_owner:t._owner}}function Ah(t){return typeof t=="object"&&t!==null&&t.$$typeof===da}function Yy(t){var e={"=":"=0",":":"=2"};return"$"+t.replace(/[=:]/g,function(n){return e[n]})}var jf=/\/+/g;function Rc(t,e){return typeof t=="object"&&t!==null&&t.key!=null?Yy(""+t.key):e.toString(36)}function cl(t,e,n,i,r){var o=typeof t;(o==="undefined"||o==="boolean")&&(t=null);var s=!1;if(t===null)s=!0;else switch(o){case"string":case"number":s=!0;break;case"object":switch(t.$$typeof){case da:case ky:s=!0}}if(s)return s=t,r=r(s),t=i===""?"."+Rc(s,0):i,Xf(r)?(n="",t!=null&&(n=t.replace(jf,"$&/")+"/"),cl(r,e,n,"",function(c){return c})):r!=null&&(Ah(r)&&(r=jy(r,n+(!r.key||s&&s.key===r.key?"":(""+r.key).replace(jf,"$&/")+"/")+t)),e.push(r)),1;if(s=0,i=i===""?".":i+":",Xf(t))for(var a=0;a<t.length;a++){o=t[a];var l=i+Rc(o,a);s+=cl(o,e,n,l,r)}else if(l=Xy(t),typeof l=="function")for(t=l.call(t),a=0;!(o=t.next()).done;)o=o.value,l=i+Rc(o,a++),s+=cl(o,e,n,l,r);else if(o==="object")throw e=String(t),Error("Objects are not valid as a React child (found: "+(e==="[object Object]"?"object with keys {"+Object.keys(t).join(", ")+"}":e)+"). If you meant to render a collection of children, use an array instead.");return s}function Ma(t,e,n){if(t==null)return t;var i=[],r=0;return cl(t,i,"","",function(o){return e.call(n,o,r++)}),i}function qy(t){if(t._status===-1){var e=t._result;e=e(),e.then(function(n){(t._status===0||t._status===-1)&&(t._status=1,t._result=n)},function(n){(t._status===0||t._status===-1)&&(t._status=2,t._result=n)}),t._status===-1&&(t._status=0,t._result=e)}if(t._status===1)return t._result.default;throw t._result}var tn={current:null},ul={transition:null},$y={ReactCurrentDispatcher:tn,ReactCurrentBatchConfig:ul,ReactCurrentOwner:Th};function bg(){throw Error("act(...) is not supported in production builds of React.")}We.Children={map:Ma,forEach:function(t,e,n){Ma(t,function(){e.apply(this,arguments)},n)},count:function(t){var e=0;return Ma(t,function(){e++}),e},toArray:function(t){return Ma(t,function(e){return e})||[]},only:function(t){if(!Ah(t))throw Error("React.Children.only expected to receive a single React element child.");return t}};We.Component=qo;We.Fragment=Uy;We.Profiler=Oy;We.PureComponent=wh;We.StrictMode=Fy;We.Suspense=Vy;We.__SECRET_INTERNALS_DO_NOT_USE_OR_YOU_WILL_BE_FIRED=$y;We.act=bg;We.cloneElement=function(t,e,n){if(t==null)throw Error("React.cloneElement(...): The argument must be a React element, but you passed "+t+".");var i=Mg({},t.props),r=t.key,o=t.ref,s=t._owner;if(e!=null){if(e.ref!==void 0&&(o=e.ref,s=Th.current),e.key!==void 0&&(r=""+e.key),t.type&&t.type.defaultProps)var a=t.type.defaultProps;for(l in e)Tg.call(e,l)&&!Ag.hasOwnProperty(l)&&(i[l]=e[l]===void 0&&a!==void 0?a[l]:e[l])}var l=arguments.length-2;if(l===1)i.children=n;else if(1<l){a=Array(l);for(var c=0;c<l;c++)a[c]=arguments[c+2];i.children=a}return{$$typeof:da,type:t.type,key:r,ref:o,props:i,_owner:s}};We.createContext=function(t){return t={$$typeof:By,_currentValue:t,_currentValue2:t,_threadCount:0,Provider:null,Consumer:null,_defaultValue:null,_globalName:null},t.Provider={$$typeof:zy,_context:t},t.Consumer=t};We.createElement=Cg;We.createFactory=function(t){var e=Cg.bind(null,t);return e.type=t,e};We.createRef=function(){return{current:null}};We.forwardRef=function(t){return{$$typeof:Hy,render:t}};We.isValidElement=Ah;We.lazy=function(t){return{$$typeof:Wy,_payload:{_status:-1,_result:t},_init:qy}};We.memo=function(t,e){return{$$typeof:Gy,type:t,compare:e===void 0?null:e}};We.startTransition=function(t){var e=ul.transition;ul.transition={};try{t()}finally{ul.transition=e}};We.unstable_act=bg;We.useCallback=function(t,e){return tn.current.useCallback(t,e)};We.useContext=function(t){return tn.current.useContext(t)};We.useDebugValue=function(){};We.useDeferredValue=function(t){return tn.current.useDeferredValue(t)};We.useEffect=function(t,e){return tn.current.useEffect(t,e)};We.useId=function(){return tn.current.useId()};We.useImperativeHandle=function(t,e,n){return tn.current.useImperativeHandle(t,e,n)};We.useInsertionEffect=function(t,e){return tn.current.useInsertionEffect(t,e)};We.useLayoutEffect=function(t,e){return tn.current.useLayoutEffect(t,e)};We.useMemo=function(t,e){return tn.current.useMemo(t,e)};We.useReducer=function(t,e,n){return tn.current.useReducer(t,e,n)};We.useRef=function(t){return tn.current.useRef(t)};We.useState=function(t){return tn.current.useState(t)};We.useSyncExternalStore=function(t,e,n){return tn.current.useSyncExternalStore(t,e,n)};We.useTransition=function(){return tn.current.useTransition()};We.version="18.3.1";xg.exports=We;var ze=xg.exports;/**
 * @license React
 * react-jsx-runtime.production.min.js
 *
 * Copyright (c) Facebook, Inc. and its affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */var Ky=ze,Zy=Symbol.for("react.element"),Jy=Symbol.for("react.fragment"),Qy=Object.prototype.hasOwnProperty,e_=Ky.__SECRET_INTERNALS_DO_NOT_USE_OR_YOU_WILL_BE_FIRED.ReactCurrentOwner,t_={key:!0,ref:!0,__self:!0,__source:!0};function Rg(t,e,n){var i,r={},o=null,s=null;n!==void 0&&(o=""+n),e.key!==void 0&&(o=""+e.key),e.ref!==void 0&&(s=e.ref);for(i in e)Qy.call(e,i)&&!t_.hasOwnProperty(i)&&(r[i]=e[i]);if(t&&t.defaultProps)for(i in e=t.defaultProps,e)r[i]===void 0&&(r[i]=e[i]);return{$$typeof:Zy,type:t,key:o,ref:s,props:r,_owner:e_.current}}rc.Fragment=Jy;rc.jsx=Rg;rc.jsxs=Rg;_g.exports=rc;var V=_g.exports;const n_={退出全屏:"Exit fullscreen","交换 A/B":"Swap A/B","键盘：方向键移动，Z 为 A，X 为 B，Enter 开始。":"Keyboard: arrows to move, Z for A, X for B, and Enter to start.",继续游戏:"Resume game","如果你的文件是 .nes，请切换到 NES 导入。":"For .nes files, switch to NES to import your cartridge.","NES 程序已停止。请尝试重新载入 ROM。":"The NES game stopped. Try reloading the ROM.","这个 NES ROM 无法启动：{0}":"Could not start this NES ROM: {0}","请选择完整的 NES 卡带文件（.nes，最大 16 MB）。":"Choose a complete NES cartridge file (.nes, up to 16 MB).","NES 卡带":"NES cartridge","导入 .nes 卡带即可游玩":"Import a .nes cartridge to play","导入 .nes":"Import .nes","卡带只在当前设备读取。将附件保存到这台设备后即可选择。":"The cartridge stays on this device. Save an attachment here before selecting it.","NES 手柄":"NES controller",选择键:"Select","键盘：方向键移动，Z 为 B，X 为 A，Enter 开始。":"Keyboard: arrows to move, Z for B, X for A, and Enter to start.","兼容 iPhone 街机手柄":"Support the iPhone arcade controller",街机手柄:"Arcade controller",返回画面:"Back to screen",方向键上:"Up",方向键左:"Left",方向键右:"Right",方向键下:"Down","动作 A":"Action A","动作 B":"Action B",投币:"Insert coin",开始游戏:"Start game",暂停游戏:"Pause game","手柄尚未连上，请检查远程连接。":"The controller is not connected. Check the remote connection.","请让电脑上的 MAME 游戏窗口保持在前台。":"Keep the MAME game window focused on the computer.","选择 MAME 程序":"Choose MAME executable","请选择 mame 或 mame.exe 程序。":"Choose the mame or mame.exe executable.","未找到可运行的 MAME。安装后可以手动选择程序。":"Could not find a working MAME installation. Install MAME, then choose its executable.","无法打开文件选择器。":"Could not open the file picker.","选择 MAME ROM ZIP":"Choose MAME ROM ZIP","请选择保留 MAME 原始短名的 ZIP 文件。":"Choose a ZIP named with its original MAME short name.","ROM 校验未通过。请核对 MAME 版本，并把依赖的 BIOS 或父集放在同一文件夹里。":"ROM verification failed. Check the MAME version and put any required BIOS or parent set in the same folder.","MAME 已退出，代码 {0}。":"MAME exited with code {0}.","游戏窗口已关闭。":"Game window closed.","无法启动 MAME。":"Could not launch MAME.","无法停止 MAME，请在游戏窗口关闭它。":"Could not stop MAME. Close the game window.","正在校验 ROM…":"Verifying ROM…","MAME 已启动。切换到游戏窗口开始玩。":"MAME launched. Switch to the game window to play.","正在停止…":"Stopping…","MAME 街机":"MAME Arcade","用本机 MAME 打开街机 ROM ZIP，游戏会在独立窗口运行。":"Open an arcade ROM ZIP with your local MAME. The game runs in a separate window.","MAME 模式需要支持本机程序的 myco 桌面 App。":"MAME mode requires the myco desktop App with local program support.","连接 MAME":"Connect MAME","查找 MAME":"Find MAME","载入 ROM ZIP":"Load ROM ZIP",启动游戏:"Launch game",停止游戏:"Stop game","默认按 5 投币、1 开始、方向键移动，Ctrl 和 Alt 为动作键。":"By default, press 5 to insert a coin, 1 to start, arrow keys to move, and Ctrl and Alt for actions.","下载官方 MAME":"Download MAME","MAME 官方试玩 ROM":"Official MAME sample ROMs","程序已停止：{0}":"Program stopped: {0}","请选择 2 到 3584 字节的 CHIP-8 ROM。":"Choose a CHIP-8 ROM between 2 and 3584 bytes.","音频无法启动。":"Could not start audio.",轨道信使:"Orbit Courier",街机模拟器:"Arcade Emulator","打开 ROM":"Open ROM",游戏画面:"Game screen",运行中:"Running",开启声音:"Enable sound",关闭声音:"Mute sound","用方向键驾驶飞船，收集信标。触屏可按 2、4、6、8。":"Steer with the arrow keys and collect beacons. On touchscreens, use 2, 4, 6 and 8.",键盘与触屏:"Keyboard and touch","按键 {0}":"Key {0}",兼容模式:"Compatibility mode","现代 CHIP-8":"Modern CHIP-8",运行速度:"Emulation speed",载入原创演示:"Load the original demo","支持 CHIP-8 ROM。文件只在本机运行。":"Supports CHIP-8 ROMs. Files run on this device.",重启:"Restart",标志工坊:"Logo Studio",品牌名称:"Brand name","例如：山径咖啡":"e.g. Ridgeline Coffee","希望标志是什么样的？":"What should the logo look like?","说说品牌、想传达的感觉，以及标志会用在哪里。":"Describe the brand, the mood, and where the mark will appear.","请 agent 出方案":"Ask agent for concepts",准备设计需求:"Prepare design brief","等待设计方案…":"Waiting for logo concepts…","将需求发给你的 agent。":"Send the brief to your agent.",查看可复制的需求:"Show brief to copy","给 agent 的需求":"Brief for your agent",构图说明:"Design rationale",标志画布:"Logo canvas","提交品牌需求，或从空白方案开始。":"Send a brand brief or start with a blank concept.",设计方案:"Design concepts",空白方案:"Blank concept",未命名方案:"Untitled concept",构图辅助线:"Construction guides",暂停构图:"Pause construction",播放构图:"Play construction",构图进度:"Construction progress",直径:"Diameter","旋转（°）":"Rotation (°)",描边宽度:"Stroke width",仅用作辅助线:"Use as a guide only","请填写有效的 SVG 路径。":"Enter valid SVG path data.",保存图层:"Save layer","图层数据过大，请减少路径节点。":"Layer data is too large. Simplify the paths.",路径数据:"Path data",线段:"Line","请帮我设计 Logo。页面 {0}，briefs 记录 {1}。请阅读本页 agent.md 和需求，设计原创的几何矢量方案，把方案写入 concepts，并附构图说明与辅助线。":"Design a logo for me. Page {0}, record {1} in briefs. Read the page guide and brief, create original geometric vector concepts, and write them in concepts with a rationale and construction guides.",圆:"Circle",路径:"Path",填充:"Fill",辅助线:"Guide",添加图层:"Add layer","{0} 刚改了":"{0} just edited",标题:"Title","正在保存…":"Saving…",没保存上:"Could not save",已保存:"Saved",你是:"You are",撤销:"Undo","在 myco App 里打开这个网页，才能读写它的内容。":"Open this page in the myco App to read and edit its content.",今天:"Today",清除:"Clear",输入名字:"Enter a name","添加「{0}」":"Add “{0}”","删除了 {0} {1}":"Deleted {0} {1}",添加字段:"Add field",展开:"Expand",在上方插入:"Insert above",一行:"row",在下方插入:"Insert below","删除选中的 {0} {1}":"Delete selected {0} {1}","删除这{0}":"Delete this {0}","已存到「下载」":"Saved to Downloads","去掉{0}":"Remove {0}",搜索或新建选项:"Find or create an option",输入一个选项:"Enter an option",新建:"New","没有选项，输入一个新建":"No options yet. Type to create one.","未命名{0}":"Untitled {0}","你在这里改的，别人看到是 {0} 改的":"Others see your edits as {0}",导出:"Export","{0} 个筛选":"{0} filters",筛选:"Filter",设置筛选条件:"Set filters",符合:"Match",符合全部条件:"Match all conditions",符合任一条件:"Match any condition",字段:"Field",条件:"Condition",值:"Value",请选择:"Choose",输入值:"Enter a value",去掉这个条件:"Remove this condition",添加条件:"Add condition","{0} 个排序":"{0} sorts",排序:"Sort",设置排序条件:"Set sorting",升序:"Ascending",降序:"Descending",去掉这个排序:"Remove this sort",添加排序:"Add sort","搜索（Ctrl+F）":"Search (Ctrl+F)",搜索:"Search",字段名:"Field name",类型:"Type",选项:"Options","删除选项{0}":"Delete option {0}","+ 添加选项":"+ Add option",添加选项:"Add option",升序排列:"Sort ascending",降序排列:"Sort descending",在左边插入字段:"Insert field left",在右边插入字段:"Insert field right","删除字段「{0}」？这一列的内容也会看不到。":"Delete “{0}”? This column's contents will also be hidden.",删除字段:"Delete field","{0} / {1} 条":"{0} / {1} records","共 {0} 条":"{0} records",添加记录:"Add record","✓ 已勾选":"✓ Checked","看板按一个「单选」字段分组：先在表格里加一个单选字段（比如「状态」）。":"Boards group by a single-select field. Add one, such as “Status”, in the table first.",分组依据:"Group by","拖动卡片改「":"Drag cards to change “","」，也能调整顺序":"”, or reorder them",未分组:"Ungrouped",新建记录:"New record","日历按一个「日期」字段排：先在表格里加一个日期字段。":"Calendars use a date field. Add a date field in the table first.",日期字段:"Date field",上个月:"Previous month",年:"/",月:"/",下个月:"Next month","条没有「":"records without “","」，在表格里能看到":"” remain visible in the table",周:"Week","{0} 新建":"Created by {0}",还有:"Also",条:"records",已提交:"Submitted","这一条已经加到「":"This record was added to “","」里了。":"”.",再填一份:"Submit another",还什么都没填:"Nothing filled in yet","填好点「提交」，就是一条新记录。":"Fill this in and choose Submit to create a record.",提交:"Submit",未命名记录:"Untitled record",上一条:"Previous record",下一条:"Next record",删除这条记录:"Delete this record",删除:"Delete",关闭:"Close",未填写:"Not filled in","「{0}」超过 {1}KB，没保存：拆成几个文件吧":"“{0}” exceeds {1} KB and was not saved. Split it into smaller files.","新文件名（可以带路径，比如 src/app.js）":"New file name (paths such as src/app.js are supported)",新文件夹名:"New folder name",改名为:"Rename to","已经有「{0}」了":"“{0}” already exists","删除了 {0} 个文件":"Deleted {0} files",新文件:"New file",删除文件夹:"Delete folder",改名:"Rename",未命名项目:"Untitled project",预览:"Preview","下载 zip":"Download ZIP",文件:"Files",新文件夹:"New folder","还没有文件。点上面的 ＋ 新建，或者让 agent 写：「在代码编辑器里写一个贪吃蛇的 index.html」。":"No files yet. Choose + above, or ask an agent to write an index.html game in the code editor.","关掉 {0}":"Close {0}",从左边打开一个文件:"Open a file from the sidebar",行:"Line","，列":", column","版本 {0}":"Version {0}",已保存版本快照:"Version snapshot saved","请在 myco App 中打开，再交给 agent":"Open this in the myco App before sending to an agent.","已交给对话中的 agent；作品会在这里更新":"Sent to the agent in your conversation. Updates will appear here.","图片未能导出，请改用 SVG":"Could not export the image. Try SVG instead.","PNG 导出失败":"PNG export failed",已保存到下载文件夹:"Saved to Downloads","图片请小于 150 KB，可先压缩后导入":"Use an image under 150 KB. Compress it before importing.","请导入 PNG、JPEG 或 WebP 图片":"Import a PNG, JPEG or WebP image.",回到工作台:"Back to workspace",设计工作台:"Design studio",所有作品:"All designs",作品标题:"Design title",切换浅色:"Switch to light",切换深色:"Switch to dark",保存版本:"Save version",新建设计:"New design",工作空间:"Workspace",设计类型:"Design type",我的工作空间:"My workspace","和你的 agent 一起创造":"Create with your agent","从一个想法，":"From an idea,","到一件好作品。":"to something worth making.","网页、海报、演示与更多。把灵感、设计和反馈留在同一个地方。":"Websites, posters, presentations and more. Keep ideas, designs and feedback together.",开始一次新的探索:"Start a new exploration","你想设计什么？":"What would you like to design?","选择一个起点，随时自由调整":"Choose a starting point. Make it your own.",最近的作品:"Recent designs",搜索作品:"Find designs","搜索作品…":"Search designs…",清空搜索:"Clear search",已完成:"Done",待评审:"In review",草稿:"Draft",没有找到作品:"No designs found","这里，留给下一个好想法":"Room for your next idea","试试别的关键词。":"Try a different search.","选一种产物开始，画布、需求和反馈都会为你准备好。":"Choose an output to start with a canvas, brief and feedback ready to go.",创建第一件作品:"Create your first design","少一点干扰，多一点创造。":"Less distraction. More creation.",显示图层:"Show layers","撤销 Ctrl+Z":"Undo Ctrl+Z","重做 Ctrl+Shift+Z":"Redo Ctrl+Shift+Z",显示属性:"Show properties",图层:"Layers",插入:"Insert",画布:"Canvas",添加画布:"Add canvas",隐藏图层:"Hide layer","显示 {0}":"Show {0}","隐藏 {0}":"Hide {0}",图片:"Image",新画布:"New canvas","图片支持 PNG、JPEG、WebP，最大 150 KB。":"PNG, JPEG and WebP images up to 150 KB.",复制作品:"Duplicate design",删除作品:"Delete design","HTML 原型":"HTML prototype","只预览自包含的 HTML，外部资源不会加载。":"Only self-contained HTML is previewed. External resources are not loaded.","HTML 已更新":"HTML updated",应用:"Apply","HTML 源码":"HTML source","粘贴完整的 HTML、内联 CSS 和 JavaScript。也可以让 agent 直接写到这件作品的 html 字段。":"Paste complete HTML with inline CSS and JavaScript. An agent can also write to this design's html field.","移除 HTML，使用画布预览":"Remove HTML and use the canvas preview",交互原型预览:"Interactive prototype preview","图层 {0}":"Layer {0}",还没有画布:"No canvas yet",交互预览:"Interactive preview","拖动图层 · 方向键微调 · Shift 加速":"Drag layers · Arrow keys to nudge · Shift for larger steps","选择一个图层，开始设计":"Select a layer to start designing",缩小:"Zoom out",适应画布:"Fit canvas",放大:"Zoom in",解锁图层:"Unlock layer",锁定图层:"Lock layer",图层名称:"Layer name","图层{0}":"Layer {0}",文字:"Text",图层文字:"Layer text",字号:"Font size",字重:"Font weight",常规:"Regular",中等:"Medium",半粗:"Semibold",粗体:"Bold",填色:"Fill",图层填色:"Layer fill",填色色值:"Fill color",圆角:"Corner radius",不透明度:"Opacity",向上一层:"Bring forward",向下一层:"Send backward","复制图层 Ctrl+D":"Duplicate layer Ctrl+D",删除图层:"Delete layer",画布设置:"Canvas settings",画布名称:"Canvas name",宽度:"Width",画布宽度:"Canvas width",高度:"Height",画布高度:"Canvas height",背景:"Background",画布背景:"Canvas background","选中画布上的图层，可以调整文字、颜色、尺寸和位置。":"Select a layer to edit its text, color, size and position.",复制画布:"Duplicate canvas",删除画布:"Delete canvas","清晰的层次。克制的色彩。":"Clear hierarchy. Restrained color.","让内容，成为主角。":"Let the content lead.",作品需求:"Design brief",想做什么:"What to make",设计需求:"Brief","为谁设计？想传达什么？有什么必须保留的内容？":"Who is it for? What should it say? What needs to stay?",状态:"Status",作品状态:"Design status",工作空间的设计约束:"Workspace design guidelines",设计约束:"Design guidelines","默认黑白灰；系统字体；留白充足…":"Black, white and grey by default; system fonts; generous spacing…","正在发送…":"Sending…","交给 agent 继续设计":"Continue with an agent","需求会发到你与这个工作台创建者的对话中。agent 更新作品后，画布实时同步。":"Your brief goes to the conversation with this workspace's creator. Agent updates appear on the canvas in real time.",一起把它变得更好:"Make it better together",反馈内容:"Feedback","哪里做得好？还有什么可以改进？":"What works well? What could improve?",添加反馈:"Add feedback",成员:"Member",重新打开:"Reopen",标记已解决:"Mark resolved","还没有反馈。把发现和想法写在这里，下一轮设计就有了方向。":"No feedback yet. Leave your observations and ideas here to guide the next iteration.","每一次探索，都有迹可循":"Keep a trail of every exploration",保存当前版本:"Save current version",已恢复版本:"Version restored",恢复:"Restore","在尝试新的方向前保存一个版本，随时可以回来。":"Save a version before trying another direction. You can always come back.",共享工作空间:"Shared workspace","每件作品，都始于一个想法。":"Every design starts with an idea.",作品名称:"Design name",新作品名称:"New design name",给这个想法起个名字:"Give this idea a name","设计需求（可稍后填写）":"Brief (optional)",新作品需求:"New design brief","为谁设计？要传达什么？":"Who is it for? What should it communicate?","默认黑白灰 · 自动保存":"Black, white and grey · Saved automatically",创建画布:"Create canvas",取消:"Cancel",确认:"Confirm",复制:"Copy",复制好了:"Copied","没复制上，选中文字手动复制吧":"Could not copy. Select the text and copy it manually.",'粘贴 JSON，比如 {"name": "myco", "ok": true}':'Paste JSON, such as {"name": "myco", "ok": true}',"粘贴 Base64":"Paste Base64",要编码的文字:"Text to encode","不是有效的 Base64":"Invalid Base64","粘贴一个网址或 a=1&b=2":"Paste a URL or a=1&b=2",要处理的文字或网址:"Text or URL to process","有不完整的 % 编码":"Incomplete percent encoding","要算哈希的文字（按 UTF-8）":"Text to hash (UTF-8)","认得秒、毫秒，或 2026-11-18 12:30 这样的日期":"Accepts seconds, milliseconds, or a date such as 2026-11-18 12:30","时间戳（秒或毫秒），或一个日期；空着就是现在":"Timestamp or date; leave empty for now",现在:"Now",本地时间:"Local time",秒:"seconds",毫秒:"milliseconds",再生成:"Generate again",数量:"Count","粘贴 JWT（eyJ…）":"Paste a JWT (eyJ…)",已过期:"Expired",还没过期:"Not expired","签名没有校验（需要密钥）。":"Signature not verified (requires a key).","正则，比如 (\\d{3})-(\\d{4})":"Pattern, such as (\\d{3})-(\\d{4})",要匹配的文字:"Text to match",处匹配:"matches",原来的:"Original",改过的:"Changed",删了:"Removed","行，加了":"lines; added","比如 user profile id、userProfileId、user_profile_id":"For example: user profile id, userProfileId, user_profile_id","不是 {0} 进制的数":"Not a base-{0} number",输入一个数:"Enter a number",十进制:"Decimal",十六进制:"Hexadecimal",二进制:"Binary",八进制:"Octal","#3370ff、rgb(51 112 255) 或 hsl(222 100% 60%)":"#3370ff, rgb(51 112 255), or hsl(222 100% 60%)",取色:"Pick color",找工具:"Find a tool","收藏起来，大家都能看到。起个名字：":"Save for everyone. Give it a name:",收藏:"Save",收藏的:"Saved tools","JWT 是用两个点隔开的三段":"A JWT has three parts separated by two dots","太长了：两边各几千行以内":"Too long: keep each side to a few thousand lines","十六进制颜色是 3、4、6 或 8 位":"Hex colors have 3, 4, 6 or 8 digits","认得 #3370ff、rgb(51 112 255)、hsl(222 100% 60%)":"Accepts #3370ff, rgb(51 112 255), hsl(222 100% 60%)","腾出了 {0}，{1} 项在废纸篓里，倒掉之前都能放回去":"Freed {0}. {1} items are in Trash and can be restored until it is emptied.","有 {0} 项没移走：{1}":"Could not move {0} items: {1}",空间清理:"Disk cleanup","个文件 ·":"files ·","{0} 秒":"{0} seconds","{0} 分 {1} 秒":"{0} min {1} sec",目前最大的:"Largest so far",停止:"Stop",磁盘:"Disk",位置:"Location",可用:"Available",重新扫描:"Scan again",有:"There are","处没读到（没有权限），实际占用会比这里多。":"locations that could not be read (permission denied). Actual usage may be higher.","给 myco「完全磁盘访问」":"Give myco Full Disk Access",不再提示:"Do not show again","{0} 里 {1} 个更小的项目":"{1} smaller items in {0}","扫了 {0} 个文件，用了 {1} 秒{2}":"Scanned {0} files in {1} seconds{2}",这里有什么:"What's here",最大的文件:"Largest files",在访达里显示:"Show in Finder",已在待清理里:"Already selected for cleanup",加到待清理:"Add to cleanup",个更小的项目:"smaller items",这里是空的:"This folder is empty","拿掉 {0}":"Remove {0}","把要清理的拖到这里，或者点每行的 +":"Drag items here to clean up, or choose + on a row","项 ·":"items ·",清空:"Clear all",移到废纸篓:"Move to Trash",把这:"Move these","项移到废纸篓？":"items to Trash?",一共:"Total","。倒掉废纸篓之前，都能放回原处。":". Items can be restored until Trash is emptied."," · 外接":" · External"," · 网络":" · Network",已用:"Used","/ 共":"/ Total",扫描:"Scan",只看一个文件夹:"Scan one folder","比整块盘快：比如个人文件夹、下载":"Faster than a whole disk: try Home or Downloads",个人文件夹:"Home","选一个…":"Choose…","上次清理：":"Last cleanup:",在:"on",上腾出了:"freed","项）":"items)","{0} 的占用图":"Storage map of {0}",点开看里面:"Open to explore","太小，没有分开画":"Too small to draw separately",点中间回上一层:"Click the center to go up","{0} 个文件":"{0} files","别人刚改过同一段，已换成最新的":"Someone just edited the same paragraph. Showing the latest version.","网页（.html）":"Web page (.html)",无标题:"Untitled",五子棋:"Gomoku",悔棋:"Undo move","认输？":"Resign?",认输:"Resign",再来一局:"Play again",点一下换人:"Click to change player","黑方（先手）":"Black (first)",白方:"White","（你）":"(you)",点这里坐下:"Take this seat",第:"Round","局 ·":"·",手:"moves","不再打卡「{0}」？打过的卡会留着。":"Stop tracking “{0}”? Existing check-ins will stay.",删除了一个习惯:"Habit deleted",习惯打卡:"Habit tracker",今天打了卡:"Checked in today",前一周:"Previous week",后一周:"Next week",连续:"Streak",本月:"This month",天:"days",删除这个习惯:"Delete this habit","：已打卡":": checked in",换个图标:"Change icon","加一个习惯：每天喝 8 杯水":"Add a habit: drink 8 glasses of water daily",新习惯:"New habit",添加:"Add",格子里的小圆点是别人打的卡:"Dots mark check-ins by other people",没有:"None","改状态…":"Change status…","改优先级…":"Change priority…","指派给…（可以写 agent 的名字）":"Assign to… (an agent name also works)","指派给「{0}」":"Assign to “{0}”",不指派:"Unassign","加标签…":"Add label…","新标签「{0}」":"New label “{0}”","放进项目…":"Move to project…",不放进项目:"No project",已指派给你:"Assigned to you",名称和编号前缀:"Name and issue prefix",任务追踪:"Issue tracker",名称:"Name",新建问题:"New issue",我的问题:"My issues",全部问题:"All issues",进行中:"In progress",待规划:"Backlog",项目:"Projects",新建项目:"New project",还没有项目:"No projects yet",显示:"Show",列表:"List",看板:"Board","命令（⌘K）":"Commands (⌘K)",编号前缀:"Issue prefix",问题显示成:"Issues appear as","-12；改了以后所有问题跟着变。":"-12; changing this updates all issues.",还没有问题:"No issues yet",按:"Press","新建一个，或者让 agent 帮你把要做的事建成问题。":"or ask an agent to turn your work into issues.",这里没有问题:"No issues here",收起:"Collapse","新建「{0}」的问题":"New issue in “{0}”","优先级：{0}":"Priority: {0}","状态：{0}":"Status: {0}","（没有标题）":"(No title)","负责人：{0}{1}":"Assignee: {0}{1}",指派:"Assign",拖到这里:"Drop here","已复制，发给负责的 agent 就行":"Copied. Send it to the agent responsible.",没复制上:"Could not copy","已删除 {0}":"Deleted {0}","复制一段话，交给 agent 去做":"Copy instructions for an agent","交给 agent":"Give to an agent",复制编号:"Copy ID","已复制 {0}":"Copied {0}","关闭（Esc）":"Close (Esc)",问题标题:"Issue title",未指派:"Unassigned",标签:"Labels","截止 {0}":"Due {0}",截止日期:"Due date","写点描述…（支持 Markdown）":"Add a description… (Markdown supported)","写点描述…":"Add a description…",动态:"Activity",创建了这个问题:"created this issue",取消了:"canceled",完成了:"completed",删除评论:"Delete comment","写评论…（⌘⏎ 发送；agent 也在这里说做了什么）":"Write a comment… (⌘⏎ to send; agents report their work here too)",评论:"Comment","已创建 {0}":"Created {0}","补充描述…（支持 Markdown）":"Add details… (Markdown supported)",负责人:"Assignee",继续新建:"Create another","状态…":"Status…","优先级…":"Priority…","指派给…":"Assign to…","标签…":"Labels…","项目…":"Project…",还没有:"None yet",可选的:"available",清除全部:"Clear all",完成:"Done","去掉「{0}」筛选":"Remove “{0}” filter","点左边「项目」旁的 ＋ 新建一个，把相关的问题放进去，就能看到进度。":"Choose + next to Projects in the sidebar, then group related issues to track progress.",目标日期:"Target date",进度:"Progress",计划中:"Planned",项目名:"Project name","删除项目（问题会留下）":"Delete project (keep issues)",删除项目:"Delete project","已删除项目，问题都还在":"Project deleted. Issues are still here.","项目状态…":"Project status…","负责人…":"Assignee…",搜索问题:"Search issues","{0} 改成「{1}」":"Set {0} to “{1}”","{0} 指派给我":"Assign {0} to me","{0} 交给 agent（复制给它的话）":"Give {0} to an agent (copy instructions)","去「我的问题」":"Go to My issues","去「全部问题」":"Go to All issues","去「进行中」":"Go to In progress","去「待规划」":"Go to Backlog","去「项目」":"Go to Projects","去项目「{0}」":"Go to project “{0}”",按列表显示:"Show as list",按看板显示:"Show as board",命令:"Commands","输入命令…":"Type a command…",没有这个命令:"No matching command",金额要是正数:"Amount must be positive",删除了一笔:"Entry deleted",记账本:"Budget book",月预算:"Monthly budget",不设:"None",支出:"Expenses",收入:"Income",结余:"Balance",预算剩余:"Budget left",金额:"Amount","备注（午饭、打车…）":"Note (lunch, taxi…)",备注:"Note",日期:"Date",记一笔:"Add entry","这个月还没有记录。在上面记一笔，或者让 agent 把账单整理进来。":"No entries this month. Add one above, or ask an agent to import your bills.","支出 {0}":"Expenses {0}","收入 {0}":"Income {0}",其他:"Other",删除这一笔:"Delete this entry",支出分类:"Expense categories",已经在录了:"Already recording",至少录一个声音:"Choose at least one audio source",没存上:"Could not save","没分出说话人：{0}":"Could not separate speakers: {0}","{0}分成了 {1} 个人":"{0}separated into {1} speakers",会议记录:"Meeting notes",新录音:"New recording",会议:"Meetings","录音中 ·":"Recording ·",转写中:"Transcribing"," · 没转写":" · Not transcribed"," · 转写没成":" · Transcription failed","录音和转写要在 myco App 里；这里能看和改文字。":"Recording and transcription need the myco App. You can read and edit text here.","开会时录下你说的和别人说的，结束后在这台电脑上转成文字，录音不上传。":"Record your voice and others during a meeting, then transcribe locally. Audio is not uploaded.","没录上：":"Could not record:",关掉:"Dismiss",我:"Me",麦克风:"Microphone",你说的话:"Your voice",录麦克风:"Record microphone",别人:"Others","会议开始后，会议软件才会出现在这里":"Your meeting app appears here once the meeting starts",刷新:"Refresh",别人的声音:"Other voices","这台电脑录不了电脑放出的声音（macOS 14.2 起才行），可以只录麦克风。":"This computer cannot record system audio (macOS 14.2+ required). You can record the microphone.",转写:"Transcription",开始录音:"Start recording","录别人之前，先告诉参会的人。录音时可以收起网页面板；关掉网页会停止录音。":"Let participants know before recording. You can hide the panel; closing this page stops recording.","正在解开{0}":"Extracting {0}","正在下载{0}":"Downloading {0}",请稍候:"Please wait",正在看转写装好了没有:"Checking local transcription setup",本机转写:"Local transcription","SenseVoice · 中英粤日韩 · 录完自动转，自动分出不同的人":"SenseVoice · Chinese, English, Cantonese, Japanese, Korean · Auto-transcription and speaker separation","没下成：":"Download failed:","SenseVoice · 中英粤日韩 · 分出不同的人还要再下载约 35 MB":"SenseVoice · Chinese, English, Cantonese, Japanese, Korean · Speaker separation needs another 35 MB",下载:"Download",先下载转写模型:"Download the transcription model first","约 {0} MB，只下一次，之后都在这台电脑上转":"About {0} MB, once only. Future transcription runs on this computer.",再试一次:"Try again",正在结束:"Finishing",已暂停:"Paused",录音中:"Recording",电脑的声音:"System audio","一直没有声音：会议在出声吗？":"No audio detected. Is your meeting playing sound?","一直没有声音：麦克风打开了吗？":"No audio detected. Is your microphone on?",继续:"Resume",暂停:"Pause",结束录音:"Finish recording",结束后在这台电脑上转成文字:"Transcribe locally after recording","（先在「新录音」里下载转写模型）":"(download the model in New recording first)","。可以收起网页面板，关掉网页会停止录音。":". You can hide the panel; closing this page stops recording.",会议名称:"Meeting name","已存到 {0}":"Saved to {0}","删除「{0}」和它的文字稿？{1}":"Delete “{0}” and its transcript? {1}"," · 录在 {0}":" · Recorded on {0}",搜索文字稿:"Search transcript",上一处:"Previous match",下一处:"Next match",关掉搜索:"Close search","搜索（⌘F）":"Search (⌘F)",纪要:"Notes",更多:"More","麦克风这边有几个人？":"How many speakers on the microphone?","别人里有几个人？":"How many other speakers?",自动:"Automatic",导出文字稿:"Export transcript","导出 Markdown":"Export Markdown","导出字幕（SRT）":"Export subtitles (SRT)",显示录音文件:"Show recording file",重新分说话人:"Separate speakers again","正在下载分说话人的模型（约 35 MB）":"Downloading speaker separation models (about 35 MB)","分说话人（先下载 35 MB）":"Separate speakers (download 35 MB first)","重新转写会换掉现在的文字稿，包括改过的字。":"Transcribing again replaces the current transcript, including edits.",重新转写:"Transcribe again",正在分说话人:"Separating speakers","排队中：前一场转完就转这场":"Queued: starts after the previous transcription","转写没成：":"Transcription failed:",从这里播放:"Play from here","单击一句从那里播放，双击改字，右键有更多。空格播放或暂停，← → 前后 5 秒，⇧← ⇧→ 上一句、下一句，⌘F 搜索。":"Click a sentence to play from it; double-click to edit; right-click for more. Space plays or pauses, ← → skips 5 seconds, ⇧← ⇧→ changes sentence, ⌘F searches.",改字:"Edit text",改成:"Change to",说的:"said by",已复制:"Copied",复制不了:"Could not copy",已删掉这句:"Sentence deleted",删除这句:"Delete this sentence","录音文件不在了（移走或删掉了），这里只能看文字。":"Recording file is missing or moved. Only the transcript is available here.","录音在 {0} 上，这里只能看文字。":"The recording is on {0}. Only the transcript is available here.",显示所有人:"Show everyone","只看{0}说的":"Show only {0}",名字:"Name","改名字（这场会议里都改）":"Rename (throughout this meeting)","改过 · 双击再改":"Edited · Double-click to edit again",正在转写:"Transcribing",我说的:"What I said",别人说的:"What others said",还没有文字稿:"No transcript yet","这场会议没有录音。":"This meeting has no recording.","录音在 {0} 上，在那台电脑上打开这个网页来转写。":"The recording is on {0}. Open this page there to transcribe it.","录音不在这台电脑上。":"The recording is not on this computer.","转写要先下载模型。":"Download the model before transcribing.","在这台电脑上转，用时大约是录音的十分之一。":"Transcribes locally in about one tenth of the recording's duration.",转成文字:"Transcribe","后退 15 秒":"Back 15 seconds",播放:"Play","播放 / 暂停（空格）":"Play / Pause (Space)","前进 15 秒":"Forward 15 seconds",播放速度:"Playback speed",跳过没人说话的地方:"Skip gaps between speakers",跳过静音:"Skip silence",播放时文字跟着走:"Scroll with playback",跟随:"Follow","已请 agent 写纪要，写好会出现在这里":"Asked an agent to write notes. They will appear here when ready.",写的:"written by","agent 正在写":"Agent is writing","请它写的，写好会出现在这里。":"was asked to write these. They will appear here when ready.","让 agent 写纪要":"Ask an agent for notes","它读完文字稿，写出总结、决定、待办（谁、什么、何时）和按话题分的章节，点章节就跳到那一段；内容多时再建一份文档和一张白板。":"It reads the transcript and writes a summary, decisions, action items (who, what, when) and chapters linked to audio. For longer meetings, it can also create a document and whiteboard.",再请一次:"Ask again","先转成文字。":"Transcribe first.",总结:"Summary",章节:"Chapters",决定:"Decisions",待办:"Action items","完整纪要（文档）":"Full notes (document)",白板:"Whiteboard","请 agent 重写":"Ask agent to rewrite","删除了 {0} 个主题":"Deleted {0} topics",未命名思维导图:"Untitled mind map",导出大纲:"Export outline",中心主题:"Central topic",主题:"Topic",子主题:"Subtopic",同级主题:"Sibling topic","删除（Delete）":"Delete (Delete)",适应画面:"Fit to screen","选中一个主题：Tab 加子主题，Enter 加同级，空格折叠，Delete 删除；拖到另一个主题上就挪过去":"Select a topic: Tab adds a child, Enter a sibling, Space folds, Delete removes. Drag it onto another topic to move it.","{0} 刚改了这一页":"{0} just edited this page","已移到回收站：{0}":"Moved to Trash: {0}",笔记:"Notes",新建页面:"New page",页面:"Pages",还没有页面:"No pages yet",回收站:"Trash"," · 你是 {0}":" · You are {0}",图标:"Icon",去掉:"Remove",颜色:"Color",在里面新建页面:"Add page inside","已复制，发给 agent，再告诉它要写什么":"Copied. Send this to an agent and say what to write.",取消收藏:"Remove favorite",换封面:"Change cover",换图标:"Change icon",添加图标:"Add icon",添加封面:"Add cover",添加子页面:"Add subpage",去掉封面:"Remove cover","交给 agent 写":"Ask an agent to write",移到回收站:"Move to Trash","新建一页开始写，「/」插入标题、列表、待办；页面可以一层层套起来。也可以让 agent 帮你整理成页面。":"Start a page and use “/” for headings, lists and to-dos. Nest pages or ask an agent to organize them.",回收站是空的:"Trash is empty","含 {0} 个子页面":"Includes {0} subpages","彻底删除「{0}」和它的子页面？删了就找不回来了。":"Permanently delete “{0}” and its subpages? This cannot be undone.",彻底删除:"Delete permanently","搜索页面的标题和内容…":"Search page titles and content…",最近:"Recent","没有找到「":"No results for “",已撤销:"Undone",已重做:"Redone",这不是图片:"This is not an image","读不了 {0}：这台电脑的浏览器不认识这种图片":"Cannot read {0}: this browser does not support this image format",修图:"Photo editor","裁剪、调色、滤镜、磨皮美白、文字、贴纸、涂鸦、马赛克、边框，改了随时能退回去。":"Crop, adjust, filter, retouch, add text, stickers, drawings, mosaics and borders. Every edit can be undone.",选择照片:"Choose photos","也可以把照片拖进来，或者直接粘贴":"Or drop photos here or paste them","照片只存在这台电脑上，hub 只记一张小图和你做了哪些修改。":"Photos stay on this computer. The Hub stores only a small preview and your edit instructions.",工具:"Tools","撤销 (⌘Z)":"Undo (⌘Z)",重做:"Redo","重做 (⇧⌘Z)":"Redo (⇧⌘Z)",按住看原图:"Hold to see the original",对比原图:"Compare original",已回到原图:"Restored original",还原:"Reset",原图:"Original","松开，加进来":"Drop to add",原图不在这台电脑上:"Original is not on this computer","它是 {0} 加的。":"Added by {0}.","把同一张照片（":"Choose or drop the same photo (","）选进来或拖进来，就能接着修。":") to continue editing.",这一页的照片:"Photos on this page",从这一页移走:"Remove from this page","已移走 {0}":"Removed {0}",加照片:"Add photos",没导出来:"Export failed",格式:"Format",尺寸:"Size","（加上边框）":"(including border)",画质:"Quality","正在导出…":"Exporting…",存到下载:"Save to Downloads",双击还原:"Double-click to reset",别的颜色:"Other colors",裁剪和旋转:"Crop and rotate",向左转:"Rotate left",向右转:"Rotate right",左右翻:"Flip horizontally",上下翻:"Flip vertically",拉直:"Straighten","拖动框的边和角来裁剪，拖动框内移动它。":"Drag the frame's edges or corners to crop, or drag inside to move it.",调色:"Adjust",全部还原:"Reset all",滤镜:"Filters",强度:"Intensity",人像:"Portrait",磨皮:"Smooth skin",美白:"Brighten skin","按肤色找到皮肤，只在平滑的地方磨，眼睛、头发和轮廓保持清楚。":"Detects skin tones and smooths flat areas while keeping eyes, hair and edges clear.",写点什么:"Write something",加粗:"Bold",大小:"Size",角度:"Angle",删除这段文字:"Delete this text","点图上的文字来改它，拖动移动，拖右下角旋转和缩放。":"Select text on the photo to edit. Drag to move; drag its bottom-right corner to rotate and resize.","选一种样式加到图上。":"Choose a style to add to the photo.",贴纸:"Stickers",删除这个贴纸:"Delete this sticker","点一个贴到图上，拖动移动，拖右下角旋转和缩放。":"Choose a sticker, then drag to move. Drag its bottom-right corner to rotate and resize.",马赛克:"Mosaic",涂鸦:"Draw",笔刷大小:"Brush size",撤掉一笔:"Undo stroke",全部擦掉:"Erase all","在图上涂抹要遮住的地方：人脸、车牌、聊天记录里的名字。":"Paint over areas to hide: faces, number plates or names in chats.","在图上按住拖动来画。":"Hold and drag on the photo to draw.",边框:"Border",边框颜色:"Border color",旋转和缩放:"Rotate and resize","拖动：旋转、缩放":"Drag to rotate and resize",已结束:"Ended",多选:"Multiple choice",单选:"Single choice",编辑:"Edit",重新开始:"Restart","票 ·":"votes ·",发起:"Start","问题，比如：团建去哪？":"Question, such as: Where should we meet?",问题:"Question","补充说明（可不填）":"Additional details (optional)",补充说明:"Additional details","选项 {0}":"Option {0}",去掉这一项:"Remove this option",加一个选项:"Add an option",保存:"Save",概览:"Overview",策略与节点:"Policies & nodes",连接:"Connections",运行配置:"Configuration",操作记录:"Activity log",规则:"Rule",全局:"Global",直连:"Direct","请更新 myco 桌面 App 后使用本机控制接口":"Update the myco desktop App to use the local controller API","最近 90 秒的上下行速率":"Upload and download rates over the last 90 seconds","等待流量采样…":"Waiting for traffic samples…","这是示例预览。连接本机内核后才能执行操作。":"This is a demo. Connect a local core before performing actions.",代理工作台:"Proxy console",控制信息仅存本机:"Controller data stays local",不与工作区共享:"Not shared with the workspace",工作台:"Workspace",示例预览:"Demo preview",演示数据:"Demo data",连接中断:"Disconnected",已连接:"Connected",正在连接:"Connecting",未连接:"Not connected",连接设置:"Connection settings","看清每一条连接，让网络按你的规则运行。":"See every connection. Let your rules guide the network.","管理出口策略，手动选择节点或交给内核自动选择。":"Manage outbound policies. Select a node or let the core choose automatically.","查看当前流量经过哪里，按需断开单条连接。":"See where traffic goes and close individual connections when needed.","只读检查当前配置；切换模式在概览中完成。":"Inspect the current configuration. Change mode in Overview.","这次打开工作台后的操作记录，仅在当前页面保留。":"Actions from this visit, kept only in the current page.",管理连接:"Manage connection",连接内核:"Connect core",关闭提示:"Dismiss notice",检查连接:"Check connection","下方保留上次数据，非实时。":"Retained data below is from the last update, not live.","示例预览 · 节点、连接和流量均为演示数据，不代表你的网络。":"Demo preview · Nodes, connections and traffic are examples, not your network.",接入真实内核:"Connect a real core",连接你正在使用的代理内核:"Connect your running proxy core","支持 Mihomo 与启用了 Clash API 的 sing-box。":"Supports Mihomo and sing-box with Clash API enabled.","输入本机控制地址和密钥，即可查看并管理网络。":"Enter a local controller address and secret to view and manage your network.",连接本机内核:"Connect local core",先看示例:"Preview demo","管理已有内核，不会自动安装或启动代理服务。":"Manages an existing core. Does not install or start a proxy service.","正在检查本机连接…":"Checking local connection…",实时流量:"Live traffic",示例曲线:"Example chart","约每 2 秒采样":"Samples about every 2 seconds","━ 下载":"━ Download","┄ 上传":"┄ Upload","约 90 秒前":"About 90 seconds ago","更新中…":"Updating…",出站策略:"Outbound policies",全部策略:"All policies",手动选择:"Manual selection",内核自动管理:"Managed by core","暂无策略组，或该内核未提供列表。":"No policy groups, or this core does not expose them.",运行模式:"Routing mode","按内核现有规则选择代理或直连。":"Use the core's existing rules to choose proxy or direct connections.","所有流量使用全局策略。":"All traffic uses the global policy.","以当前内核返回的模式为准。":"Shows the mode reported by the core.","密钥由本机 App 保管":"Secret kept by the local App",个策略组:"policy groups",只有手动选择组可以切换:"Only manual selection groups can be changed","当前：":"Current:",未测速:"Not tested",测速:"Test delay","暂无策略组，检查内核配置或连接状态。":"No policy groups. Check the core configuration or connection.",活跃连接:"Active connections",数据不可用:"Data unavailable",搜索连接:"Search connections","搜索域名、进程、策略":"Search host, process or policy","目标 / 进程":"Destination / Process",策略链:"Policy chain","下载 / 上传":"Download / Upload",操作:"Actions",未知目标:"Unknown destination",未知进程:"Unknown process",断开:"Disconnect",当前没有活跃连接:"No active connections",等待连接数据:"Waiting for connection data",内核状态:"Core status",只读摘要:"Read-only summary","工作台管理已有内核，不会修改系统代理、DNS、TUN 或配置文件。系统流量是否经过代理，取决于你已使用的代理客户端。":"This console manages an existing core. It does not change system proxy, DNS, TUN or config files. System traffic routing depends on your existing proxy client.",本次操作:"This visit's actions","最多 100 条":"Up to 100 entries","还没有操作记录。这里不包含内核原始日志。":"No actions yet. This does not include the core's raw logs.","上次更新 {0}":"Updated {0}",等待内核响应:"Waiting for core response",关闭连接设置:"Close connection settings","先在已有代理客户端中启用 Clash API，控制地址需使用本机回环 IP。":"Enable Clash API in your existing proxy client. Use a literal loopback IP for the controller.",代理内核:"Proxy core",控制地址:"Controller address",控制密钥:"Controller secret","留空保留已有密钥；换地址时自动清除":"Leave blank to keep the secret; changing the address clears it",内核未设密钥时可留空:"May be blank if the core has no secret","凭据按此页面存于本机 App，不写入工作区。Linux 无系统钥匙串时遵循 App 的本机凭据文件设置。":"Credentials stay in the local App, scoped to this page, never in the workspace. Linux follows the App's local credential-file setting when no keychain is available.",断开并忘记:"Disconnect and forget",保存并连接:"Save and connect",删除了一页:"Slide deleted",未命名幻灯片:"Untitled slides",放映:"Present","PowerPoint（.pptx，能接着改）":"PowerPoint (.pptx, editable)","Markdown 大纲（.md）":"Markdown outline (.md)",所有页:"All slides","第 {0} 页：{1}":"Slide {0}: {1}",复制这一页:"Duplicate this slide",删除这一页:"Delete this slide",新建页:"New slide","还没有内容。新建一页，或者让 agent 按大纲写：「用这个幻灯片做一份 Q4 发布会的 PPT，8 页左右」。":"No content yet. Add a slide, or ask an agent to turn an outline into an eight-slide launch presentation.",新建封面:"New cover","演讲者备注（放映时不显示，导出到 PPT 的备注里）":"Speaker notes (hidden during presentation, exported as PowerPoint notes)",演讲者备注:"Speaker notes",版式:"Layout",图表:"Chart","系列{0}":"Series {0}",系列名:"Series name",加一个系列:"Add series",类别:"Category",数值:"Value",删掉这一行:"Delete this row",加一行:"Add row",卡片:"Cards",加一张:"Add card",去掉最后一张:"Remove last card",表格:"Table",加一列:"Add column",去掉最后一行:"Remove last row",去掉最后一列:"Remove last column",上一页:"Previous slide",下一页:"Next slide",结束放映:"End presentation",要点:"Key points",在右边填图表的数据:"Enter chart data on the right",列名:"Column name",引言:"Quote",数字或标题:"Number or title",说明:"Description",图表导出失败:"Chart export failed",开:"Open",高:"High",低:"Low",收:"Close",量:"Volume","成交量 · 股":"Volume · shares","请在支持 myco.os 的新版桌面 App 中打开，才能获取行情":"Open this in a newer desktop App with myco.os to fetch market data","数据源限流，请稍后刷新或切换数据源":"Provider rate limit reached. Refresh later or choose another provider.","数据源请求失败（HTTP {0}），请稍后重试或切换数据源":"Provider request failed (HTTP {0}). Try again later or switch providers.","数据源返回内容过大，已停止读取":"Provider response is too large; stopped reading","数据源没有返回有效 JSON，请切换数据源":"Provider returned invalid JSON. Choose another provider.",已在自选中:"Already in watchlist",已移出自选:"Removed from watchlist",图表还没准备好:"Chart is not ready yet",数据源设置已保存到本机:"Provider settings saved on this computer",行情工作台:"Stock watch","A 股":"A-shares",港股:"Hong Kong",美股:"US",设置:"Settings",添加自选:"Add to watchlist",我的自选:"My watchlist","跨市场，一处关注":"Follow markets in one place",搜索自选:"Search watchlist",名称或代码:"Name or symbol",全部:"All","名称 / 代码":"Name / Symbol","价格 / 涨跌幅":"Price / Change",暂不可用:"Unavailable","读取中…":"Loading…"," · 缓存":" · Cached",没有找到自选股:"No matching stocks",这个市场还没有自选股:"No stocks in this market yet",添加一只:"Add a stock",行情从这台电脑获取:"Market data is fetched by this computer",自选列表与备注在工作区共享:"Watchlist and notes are shared in the workspace",移出自选:"Remove from watchlist",导出行情:"Export market data",涨跌基准未提供:"Change baseline unavailable",等待有效报价:"Waiting for a valid quote","源站报价 {0}":"Source quote {0}",报价时间未提供:"Quote time unavailable",与前一交易日收盘相比:"Compared with the previous trading day's close",不复权:"Unadjusted","K 线图":"Candlestick chart",收盘价折线:"Close-price line",展开图表:"Expand chart",正在读取:"Loading","行情…":"market data…",这次没能取到行情:"Could not fetch market data",重试:"Retry",切换数据源:"Change provider","选择股票后显示 K 线":"Choose a stock to see its chart","离线缓存 · 不是最新行情":"Offline cache · Not current market data",本机缓存:"Local cache",源站数据:"Source data","根 · 获取于":"bars · Fetched",观察笔记:"Watch notes","与 agent 共享":"Shared with agents","写下关注这只股票的原因、重要价格或待观察的变化…":"Why are you watching this stock? Note key prices or changes to follow…",数据说明:"About the data",管理来源:"Manage providers","报价与 K 线均来自所选数据源。源站不可用时会提示错误，不使用模拟价格。":"Quotes and bars come from your selected provider. Errors are shown when a source is unavailable; no simulated prices are used.","末根 K 线可能尚未收盘；成交量单位为股。各市场币种独立，不进行汇率换算。":"The last bar may still be open. Volume is in shares. Each market keeps its own currency; no currency conversion is applied.","把关注的市场，":"Bring the markets you follow","放在同一个视野里。":"into one view.","A 股、港股、美股。自选、K 线和观察笔记，":"A-shares, Hong Kong and US stocks. Watchlists, charts and notes,","跟着你的思路走。":"organized around your thinking.",添加第一只自选股:"Add your first stock","每 60 秒刷新 · 后台暂停":"Refreshes every 60 seconds · Pauses in background",手动刷新:"Refresh manually","行情仅作信息展示 · 延迟以源站为准":"For information only · Delays depend on the source",显示与数据设置:"Display and data settings",添加自选股:"Add a stock",股票名称或代码:"Stock name or symbol","常用名称，或输入 600519 / 000001.SZ":"Common name, or 600519 / 000001.SZ","常用名称，或输入 00700 / 0700.HK":"Common name, or 00700 / 0700.HK","常用名称，或输入 AAPL / MSFT":"Common name, or AAPL / MSFT","名称匹配常用列表；任意股票可直接输入代码":"Common names use a shortlist; enter any stock's symbol directly","常用股票 · 添加前验证行情是否可用":"Common stocks · Availability checked before adding","没有匹配。请直接输入这个市场的证券代码。":"No match. Enter this market's security symbol directly.","正在验证代码与行情…":"Checking symbol and market data…","数据源：":"Provider:",行情数据源:"Market data provider","优先 Yahoo Finance；不可用时，A 股和港股尝试腾讯行情。美股不会降级到不完整的数据源。":"Tries Yahoo Finance first; falls back to Tencent for A-shares and Hong Kong. US data never falls back to an incomplete source.","覆盖 A 股、港股、美股。使用公开接口，无需密钥；可能限流或变更。":"Covers A-shares, Hong Kong and US stocks. Public API, no key needed; rate limits and changes may occur.","支持沪深 A 股与港股历史行情。美股和北交所历史数据不在此适配器的支持范围。":"Shanghai/Shenzhen A-shares and Hong Kong historical data. US and Beijing exchange history are not supported by this adapter.","接入自己的 AKShare、Tushare 或其他服务。网关统一返回证券、报价与 OHLCV，界面无需修改。":"Connect your own AKShare, Tushare or other service. Return normalized securities, quotes and OHLCV without changing the UI.",网关地址:"Gateway URL",行情网关地址:"Market data gateway URL","将请求此地址下的 /bars。请在网关服务端管理密钥，地址中不要放凭证。":"Requests /bars under this URL. Manage keys on the gateway server; do not put credentials in the URL.",自动刷新:"Auto-refresh","每 60 秒获取；窗口在后台时暂停":"Fetch every 60 seconds; pause while the window is in the background",涨跌颜色:"Price change colors",红涨绿跌:"Red up, green down",绿涨红跌:"Green up, red down",外观:"Appearance",跟随系统:"Follow system",浅色:"Light",深色:"Dark","设置与行情缓存只保存在这台电脑；自选股与观察笔记通过工作区共享。使用公开接口时，请遵守数据源的使用条款。":"Settings and market data cache stay on this computer. Watchlists and notes are shared in the workspace. Follow providers' terms when using public APIs.",不绑定任何一家数据商:"Independent of any single data provider",保存设置:"Save settings","导出 CSV":"Export CSV","导出 JSON（含来源与时间）":"Export JSON (with source and time)","导出图表 PNG":"Export chart PNG",数据源没有返回有效报价:"Provider returned no valid quote","没有可用的 K 线数据，请核对代码或切换数据源":"No valid price bars. Check the symbol or change provider.","Yahoo Finance 暂时没有这只股票的数据":"Yahoo Finance currently has no data for this stock",数据源返回了不同的证券代码:"Provider returned a different security symbol",数据源币种与所选市场不一致:"Provider currency does not match the selected market","腾讯历史行情适配器不支持美股，请选择 Yahoo 或自定义网关":"Tencent historical data does not support US stocks. Choose Yahoo or a custom gateway.",腾讯行情暂时不可用:"Tencent data is temporarily unavailable","腾讯历史行情不支持美股，请切换数据源":"Tencent history does not support US stocks. Change provider.",请填写有效的行情网关地址:"Enter a valid market data gateway URL","网关需使用 HTTPS（本机可用 HTTP），地址不带凭证、查询参数或片段":"Use HTTPS (HTTP is allowed locally), without credentials, query parameters or fragments",网关返回的证券或周期不匹配:"Gateway returned a different security or interval","网关需返回不复权价格、股数成交量和正确币种":"Gateway must return unadjusted prices, share-count volume and the correct currency","网关没有返回有效 K 线":"Gateway returned no valid price bars",行情请求失败:"Market data request failed","这个数据源不支持所选市场，请切换数据源":"This provider does not support the selected market. Change provider.","换一道新题？这道题填的会清掉。":"Start a new puzzle? This puzzle's entries will be cleared.",这道题没有答案:"This puzzle has no solution",数独:"Sudoku",提示一格:"Hint one cell",难度:"Difficulty",换一道:"New puzzle","第 {0} 行第 {1} 列{2}":"Row {0}, column {1}{2}","{0} 填的":"Entered by {0}","全部填对了！":"All correct!","还没有题：点上面的「换一道」":"No puzzle yet. Choose New puzzle above.","还差 {0} 格{1}":"{0} cells left{1}",擦掉:"Erase",谁填了几格:"Cells filled by each player","点一格，按数字键或下面的数字填；方向键移动，退格擦掉。别人填的格子是他的颜色。":"Select a cell and type a number, or use the numbers below. Arrow keys move; Backspace erases. Other players' cells use their color.",新增一行:"Add row","{0} / {1} 行":"{0} / {1} rows","共 {0} 行":"{0} rows",删除了一条待办:"To-do deleted",标为没做完:"Mark incomplete",标为做完:"Mark complete",取消重要:"Remove importance",标为重要:"Mark important",详情:"Details",截止:"Due",去掉截止日期:"Remove due date",明天:"Tomorrow",下周:"Next week",谁来做:"Who will do this",我来:"Assign to me","{0} 最后改的":"Last edited by {0}",待办清单:"To-do list","日 周":"D W","添加待办，回车保存（可以写「明天」「周五」「@名字」「!」）":"Add a to-do and press Enter (supports “tomorrow”, “Friday”, “@name” and “!”)",添加待办:"Add to-do","都做完了。在上面加一条，或者让 agent 把会议里的待办整理进来。":"All done. Add another above, or ask an agent to organize action items from a meeting.","这里没有要做的。":"Nothing to do here.",时间线上还没有视频:"No video on the timeline","本机没有「{0}」，先在素材里选上这个文件":"“{0}” is not on this computer. Select the file in Media first.","这个浏览器不能编码视频（需要 WebCodecs）":"This browser cannot encode video (WebCodecs required)","「{0}」读不出来：换成 mp4（H.264）试试":"Cannot read “{0}”. Try MP4 (H.264).","选的文件和原来的「{0}」不一样（大小或内容不同）。还是用它代替吗？大家看到的都会换成它。":"The selected file differs from “{0}” in size or content. Replace it for everyone?","找到了，接着剪吧":"Found it. Continue editing.",未命名视频:"Untitled video",画面比例:"Aspect ratio","让 agent 一起剪":"Edit with an agent","在「备注」里给时间点留话（「这段删掉」「给这个人脸打码」「加一句字幕」），再把这段话发给 agent。它按备注改剪辑方案，改动马上出现在这里。":"Leave time-linked notes (“cut this”, “blur this face”, “add a caption”), then send the instructions to an agent. Its edits appear here immediately.",去写备注:"Add notes","已复制，发给任意一个 agent 就行":"Copied. Send this to any agent.","没复制上，请手动选中复制":"Could not copy. Select and copy manually.",分辨率:"Resolution",原始:"Original",帧率:"Frame rate",帧:"frames","帧 ·":"frames ·","· 约":"· About","分辨率和帧率越低导出越快；在这台电脑上完成，不上传。":"Lower resolutions and frame rates export faster. Everything runs locally; no upload.","这个版本的 myco App 还存不了视频：先把 App 更新到 v0.20.27 或更新的版本再导出。":"This myco App cannot save video yet. Update to v0.20.27 or newer before exporting.",开始导出:"Start export","先合成声音…":"Mixing audio first…","一帧一帧地画出来。":"Rendering frame by frame.","收尾…":"Finishing…","% · 已用":"% · Elapsed"," · 还要约 {0}":" · About {0} left",已存到:"Saved to","MB，用了":"MB, took","这个浏览器不能编码声音，导出的视频没有声音。换 Chrome 或 Edge 打开再导出一次。":"This browser cannot encode audio, so the export is silent. Open in Chrome or Edge and export again.",好:"OK","视频做好了（":"Video is ready (","），但没存到「下载」：":"), but could not save to Downloads:","关掉这个框之前它一直在，不用重新导出。":"Keep this dialog open to retry without exporting again.",用浏览器下载:"Download with browser",不要了:"Discard",再存一次:"Save again",导出视频:"Export video",导入视频:"Import video",视频:"Video",放到时间线末尾:"Append to timeline","本机没有，选文件":"Missing locally; choose file","从项目里去掉（连同时间线上用到它的片段）":"Remove from project (including its timeline clips)","已去掉「{0}」":"Removed “{0}”","支持 mp4、mov、webm。文件只存在这台电脑的浏览器里，不会上传；换一台电脑打开时，选一下同一个文件就能接着剪。":"Supports MP4, MOV and WebM. Files stay in this browser and are not uploaded. Select the same file on another computer to continue editing.","点一个样式，在播放头处加 3 秒文字。在画面上拖动它换位置，在时间线上拖两端改时长。":"Choose a style to add three seconds of text at the playhead. Drag it in the preview to move; drag its timeline edges to adjust duration.",模糊:"Blur","在播放头处加一块打码，默认盖到这段视频结束。在画面上拖动方框盖住人脸、车牌，拖四角改大小；在时间线上拖两端改出现的时间。":"Add a mask at the playhead through the end of this clip. Drag it over a face or plate, resize with its corners, and adjust its timeline edges to set timing.","在 {0} 留一条备注，比如「这段删掉」「这里给人脸打码」":"Note at {0}, such as “cut this” or “blur this face”",加备注:"Add note",标成没处理:"Mark unhandled",标成已处理:"Mark handled",删除备注:"Delete note","在时间点上留备注，然后把下面这段话发给 agent。它会读剪辑方案、按备注改，改动马上出现在这里。":"Leave time-linked notes, then send the instructions below to an agent. It reads the edit plan and applies your notes; changes appear here immediately.","现在有 {0} 条没处理。":"{0} unhandled notes.","复制给 agent 的话":"Copy agent instructions",视频片段:"Video clip",变速:"Change speed",速度:"Speed",变速后时长:"Duration at this speed","，声音不变调":", preserving audio pitch",音量:"Volume",取消静音:"Unmute",静音:"Mute",用原片的哪一段:"Source segment",到:"To",原片长:"Source duration","；在时间线上拖片段两端也能裁剪":"; you can also trim by dragging the clip's timeline edges",把播放头放到这段中间再分割:"Place the playhead inside this clip before splitting",分割:"Split",打码:"Mask",内容:"Content",样式:"Style",方式:"Method",范围:"Range",盖满画面:"Fill frame",放回中间:"Recenter",出现时间:"Timing",作品:"Output","，30 帧每秒":", 30 frames per second","比例和视频不一样时，空出来的地方是这个颜色":"Used for empty space when the aspect ratio differs from the video",总时长:"Total duration",片段:"Clips",文字和打码:"Text and masks",快捷键:"Shortcuts",这里不能全屏:"Fullscreen is unavailable here","本机没有「":"“","」，在左边「素材」里点「本机没有，选文件」选一下它":"” is missing locally. Choose Missing locally; choose file in Media on the left.",导入视频开始剪辑:"Import a video to start editing","点这里选文件，或者把 mp4 拖进来。视频只存在这台电脑上，剪辑方案存在 myco 里，agent 也能改。":"Choose a file or drop an MP4 here. Video stays on this computer. The edit plan lives in myco, where agents can edit it too.","到开头 (Home)":"Go to start (Home)",到开头:"Go to start","上一帧 (←)":"Previous frame (←)",上一帧:"Previous frame","播放 / 暂停 (空格)":"Play / Pause (Space)","下一帧 (→)":"Next frame (→)",下一帧:"Next frame","到结尾 (End)":"Go to end (End)",到结尾:"Go to end",全屏:"Fullscreen",时间线:"Timeline","在播放头处分割 (S)":"Split at playhead (S)",播放头要在一段视频中间:"Place the playhead inside a clip","复制一段 (⌘D)":"Duplicate clip (⌘D)","删除 (Delete)":"Delete (Delete)",缩放时间线:"Timeline zoom",适合宽度:"Fit width",整条时间线放进视野:"Fit the whole timeline in view",主轨道:"Main track","备注：{0}":"Note: {0}","把视频拖到这里，或者在左边「素材」里点 ＋":"Drop video here, or choose + in Media on the left",本机没有这个文件:"This file is not on this computer","删除了 {0} 个":"Deleted {0} items",白板上还什么都没有:"Nothing on this whiteboard yet",未命名白板:"Untitled whiteboard",导出图片:"Export image",选中:"Selected",个:"items","恢复 100%":"Reset to 100%","双击空白处贴一张便签，或者从左边选个工具。也可以让 agent 来：「在白板上把这次复盘的要点贴成便签」。":"Double-click an empty area to add a sticky note, or choose a tool on the left. You can also ask an agent to put your retrospective's key points on this board.","别人刚改过，已显示最新的":"Someone just edited this. Showing the latest version.",文本:"Text",多行文本:"Long text",数字:"Number",勾选:"Checkbox",人员:"Person",链接:"Link","在 myco App 里打开这个应用，才能用这台电脑":"Open this app in the myco App to use this computer.","这个 myco App 还不能让应用用电脑：更新 App 后再试":"Update the myco App to let apps use this computer.",没存下来:"Could not save",包含:"Contains",不包含:"Does not contain",等于:"Equals",不等于:"Does not equal",大于:"Greater than",小于:"Less than",为空:"Is empty",不为空:"Is not empty",已勾选:"Checked",未勾选:"Unchecked",记录:"Record",日历:"Calendar",画廊:"Gallery",表单:"Form",设计:"Design",刚刚:"Just now",形状:"Shape",已同步:"Synced",矩形:"Rectangle",椭圆:"Ellipse",线条:"Line","删除这件作品？":"Delete this design?","作品将移出工作台。已有的导出文件和版本快照保留。":"The design will leave the workspace. Existing exports and version snapshots remain.",属性:"Properties",需求:"Brief",反馈:"Feedback",版本:"Versions",宽:"Width","删除这个画布？":"Delete this canvas?","画布上的图层会一起删除，可以撤销。":"Its layers will also be deleted. You can undo this.","当前设计会先保存为新快照，再恢复所选版本。":"The current design is saved as a snapshot before restoring this version.","SVG · 当前画布":"SVG · Current canvas","PNG · 当前画布":"PNG · Current canvas","HTML · 完整作品":"HTML · Full design","JSON · 可编辑设计数据":"JSON · Editable design data",网页:"Website","落地页、官网、交互原型":"Landing pages, websites and interactive prototypes",移动应用:"Mobile app","界面、流程、产品概念":"Interfaces, flows and product concepts",海报:"Poster","活动、品牌、宣传物料":"Events, brands and promotional materials",演示文稿:"Presentation","提案、发布会、故事讲述":"Proposals, launches and storytelling",社交图片:"Social image","封面、卡片、内容配图":"Covers, cards and content images",数据看板:"Dashboard","指标、报告、管理界面":"Metrics, reports and management interfaces","JSON 格式化":"JSON formatter","格式化、压缩、校验，出错时指出位置":"Format, minify and validate with error locations","编码、解码，支持中文和 URL 安全的写法":"Encode and decode Unicode or URL-safe Base64","URL 编解码":"URL encoder","编码、解码，拆开查询参数":"Encode, decode and inspect query parameters",哈希:"Hashes",时间戳:"Timestamps","秒、毫秒和日期互转":"Convert seconds, milliseconds and dates","批量生成 UUID v4":"Generate UUID v4 in batches","JWT 解析":"JWT inspector","看 header、payload 和过期时间":"Inspect headers, payloads and expiration",正则测试:"Regex tester","高亮匹配，列出分组":"Highlight matches and list capture groups",文本对比:"Text diff",逐行比较两段文字:"Compare two texts line by line",命名转换:"Case converter",进制转换:"Number bases","二、八、十、十六进制":"Binary, octal, decimal and hexadecimal",颜色转换:"Color converter",格式化:"Format",压缩:"Minify",编码:"Encode","编码（URL 安全）":"Encode (URL-safe)",解码:"Decode",拆参数:"Query parameters",区分大小写:"Case-sensitive",不区分:"Case-insensitive",昨天:"Yesterday",选一个文件夹来扫描:"Choose a folder to scan",这台电脑:"This computer",标题没保存上:"Could not save the title","谁执黑？写你自己，或者一个 agent 的名字（它看到轮到自己就会下）":"Who plays black? Enter your name or an agent name; the agent can play on its turn.","谁执白？写你自己，或者一个 agent 的名字":"Who plays white? Enter your name or an agent name.",黑:"Black",白:"White","先坐下：点「黑方」「白方」写上谁来下":"Choose Black or White and enter who will play.","，该你了":", your turn",新项目:"New project","（我）":"(me)",有人:"Someone",优先级:"Priority",评审中:"In review",已取消:"Canceled",紧急:"Urgent",中:"Medium",无优先级:"No priority",收支:"Income / Expense",分类:"Category",对方:"Others",电脑的全部声音:"All system audio",所有应用放出的声音:"Audio from all apps",正在出声:"Playing audio",只录这个应用:"Record only this app",不录:"Do not record",只录麦克风:"Record microphone only",转写引擎:"Transcription engine",断句模型:"Voice activity model",识别模型:"Recognition model","录音会移进废纸篓。":"The recording will be moved to Trash.",另一台电脑:"Another computer",没发出去:"Could not send",思维导图:"Mind map","输入文字，或者按「/」插入标题、列表、待办…":"Type, or use “/” for headings, lists and to-dos…",你:"You",复制一份:"Duplicate",裁剪:"Crop",亮度:"Brightness",对比度:"Contrast",饱和度:"Saturation",色温:"Temperature",色调:"Tint",高光:"Highlights",阴影:"Shadows",锐化:"Sharpness",暗角:"Vignette",褪色:"Fade",颗粒:"Grain",清新:"Fresh",暖阳:"Sunlight",胶片:"Film",日系:"Soft daylight",鲜艳:"Vivid",冷调:"Cool",复古:"Vintage",港风:"Hong Kong",美食:"Food",夜景:"Night",黑白:"Black & white",银盐:"Silver",旧照:"Old photo",自由:"Free",原比例:"Original ratio",无:"None",白边:"White border",细边:"Thin border",拍立得:"Instant photo",黑体:"Sans serif",宋体:"Serif",楷体:"Calligraphy",等宽:"Monospace",自然:"Natural",清透:"Clear",精致:"Refined",描边:"Outline",手写:"Handwritten",印章:"Stamp",底色:"Background",画笔:"Pen",荧光笔:"Highlighter",霓虹:"Neon","此 App 尚未提供代理控制接口或本机钥匙串，请更新桌面 App":"Update the desktop App for local proxy control and credential storage.","打开示例预览 · 不连接内核，不改本机配置":"Opened demo preview · No core connection or configuration changes",已保存本机控制连接:"Local controller connection saved"," · 未完成":" · Not completed","已断开控制连接 · 内核继续运行":"Controller disconnected · The core keeps running",下载速率:"Download rate",等待流量采样:"Waiting for traffic samples",上传速率:"Upload rate",当前连接:"Connections",活跃的网络连接:"Active network connections",代理节点:"Outbound nodes",内核:"Core",等待响应:"Waiting for response","已保存在本机（不会显示）":"Saved locally (never displayed)",未设置:"Not set",混合端口:"Mixed port",未提供:"Unavailable","HTTP / SOCKS 端口":"HTTP / SOCKS ports",允许局域网:"Allow LAN",已开启:"Enabled",未开启:"Disabled",内核报告已开启:"Reported enabled by core","未开启 / 内核未提供":"Disabled / Not exposed by core",柱状:"Bar",折线:"Line",饼图:"Pie",封面:"Cover",章节页:"Section",标题和要点:"Title and key points",要点和图表:"Key points and chart",数字卡片:"Number cards",结束页:"End slide",简约白:"Minimal white",深蓝:"Navy",暖色:"Warm",科技:"Technology",自动选择:"Automatic",腾讯行情:"Tencent Finance",自定义网关:"Custom gateway",时间未提供:"Time unavailable",行情加载失败:"Could not load market data","无法确认这个代码，请核对后重试":"Could not validate this symbol. Check it and try again.","导出失败，请重试":"Export failed. Try again.",昨收:"Previous close",最新日开盘:"Latest daily open",最新日最高:"Latest daily high",最新日最低:"Latest daily low","日 K":"Daily","周 K":"Weekly","月 K":"Monthly","公开接口，延迟以源站为准；不复权日线，周/月线由日线聚合":"Public API; delays depend on source. Unadjusted daily bars; weekly/monthly bars aggregate daily bars.","公开接口，延迟以源站为准；不复权；A 股成交量由手换算为股":"Public API; delays depend on source. Unadjusted prices; A-share volume converted from lots to shares.","由你的行情网关提供，延迟与授权以该服务为准":"From your market data gateway; delays and access depend on that service.","首选数据源暂不可用，已切换至 ":"Preferred provider unavailable. Switched to ","刷新失败，显示上次缓存；不是最新行情":"Refresh failed. Showing cached data, not current quotes.",简单:"Easy",困难:"Hard",我的:"Mine",重要:"Important",最高:"Highest",标准:"Standard",小文件:"Smaller file",素材:"Media",不知道为什么:"Unknown reason",导出失败:"Export failed",正在导出:"Exporting",导出好了:"Exported","做好了，还没存上":"Ready, but not saved yet",没导出成功:"Export unsuccessful",原始比例:"Original ratio","16:9 横屏":"16:9 landscape","9:16 竖屏":"9:16 portrait","1:1 方形":"1:1 square",大标题:"Headline",字幕:"Subtitle",角标:"Corner label",上:"Top",下:"Bottom",空格:"Space","播放 / 暂停":"Play / Pause",在播放头处分割:"Split at playhead",删除选中的:"Delete selected","撤销 / 重做":"Undo / Redo","一帧一帧走（加 ⇧ 一秒）":"Step one frame (⇧ for one second)","⌘ + 滚轮":"⌘ + Scroll",选择:"Select",拖动画布:"Pan canvas",便签:"Sticky note","连线：从一个拖到另一个":"Connect: drag from one item to another",多维表格:"Database",文档:"Document",幻灯片:"Slides",代码编辑器:"Code editor",开发者工具箱:"Developer toolbox",视频剪辑:"Video editor","累计 {0}":"Total {0}","{0} 个策略组":"{0} policy groups",字幕放在底部:"Subtitle at the bottom",描边大字:"Outlined headline",居中:"Center","跟随 App":"Follow App",星期:"Weekday",暂未保存:"Not saved yet","删除文件夹「{0}」和里面的 {1} 个文件？":"Delete folder “{0}” and its {1} files?","删除「{0}」？":"Delete “{0}”?","没发送成功，请重试":"Could not send. Try again.",麦克风这边算作一个人:"Treating the microphone side as one speaker",别人算作一个人:"Treating others as one speaker",已停止:"Stopped","「{0}」没导入成功":"Could not import “{0}”",餐饮:"Food",交通:"Transport",购物:"Shopping",住房:"Housing",日用:"Household",娱乐:"Entertainment",医疗:"Health",学习:"Learning",人情:"Gifts",工资:"Salary",奖金:"Bonus",报销:"Reimbursement",理财:"Investments","{0} 小时 {1} 分":"{0} hr {1} min","今天 {0}":"Today {0}","昨天 {0}":"Yesterday {0}","{0}方认输，{1}方赢了":"{0} resigned. {1} wins.","{0}方五子连珠，赢了！":"{0} wins with five in a row!","轮到{0}方（{1}）{2}":"{0} to play ({1}){2}",保存失败:"Save failed","这个版本的 myco App 还不能录音和转写，更新 App 后就能用；这里能看和改文字。":"Update the myco App for recording and transcription. You can still read and edit text here.","这个 myco App 还不能播放录音，更新后就能听。":"Update the myco App to play this recording.",这个网页出错了:"This page encountered an error","会议和文字稿都还在。把这段话发给做这个应用的 agent，或者重新加载试试。":"Your meetings and transcripts are still here. Send these details to the app's agent, or try reloading.",重新加载:"Reload","Mac 首次使用要允许屏幕录制；锁屏、登录界面暂不支持。":"On your Mac, allow screen recording before connecting. Lock and sign-in screens are not supported yet.","两边保持 myco 和此应用打开，关闭应用即停止共享。":"Keep myco and this app open on both computers. Closing the app stops sharing.",了解怎么共享:"How sharing works","仅在使用远控时运行，可随时卸载。不会自动开放这台电脑。":"Runs only when needed and can be uninstalled anytime. Installing does not enable incoming connections.",仅查看:"View only",你的电脑:"Your computers",使用时启动:"Starts when needed",停止共享:"Stop sharing",允许键盘和鼠标操作:"Allow keyboard and mouse control","先使用同一局域网或 Tailscale。当前版本不自动配置公网中继。":"Start on the same local network or Tailscale. This version does not set up a public relay.","先在这台电脑上安装可选远控组件。":"Install the optional remote component on this computer first.",共享这台电脑:"Share this computer",刷新电脑列表:"Refresh computers",加密连接:"Encrypted connection",卸载组件:"Uninstall component","另一个远程桌面应用正在使用本机组件。请先在那里停止共享。":"Another Remote Desktop app is using this computer's component. Stop sharing there first.",可以操作:"Control enabled",可连接:"Available",启动方式:"Startup","在另一台电脑中打开此应用，启用共享后就会出现在这里。":"Open this app on another computer and enable sharing to see it here.","在同一工作区打开远程桌面，输入这里的配对码即可连接。":"Open Remote Desktop in the same workspace and enter this pairing code to connect.","在需要连接的 Mac 或 Windows 上打开远程桌面，选择「共享这台电脑」。":"On the Mac or Windows computer you want to reach, open Remote Desktop and choose Share this computer.",声音:"Audio",复制配对码:"Copy pairing code",安装可选远控组件:"Install the optional remote component",安装状态:"Installation",安装组件:"Install component",已安装:"Installed",开启共享:"Start sharing","当前 App 不支持 H.264 实时画面，请更新桌面 App。":"This App cannot display live H.264 video. Update the desktop App.",当前版本不传输声音:"Audio is not transmitted in this version","按需安装 · 默认关闭 · 随时断开":"Optional install · Off by default · Disconnect anytime","换个关键词试试。":"Try another search.",搜索电脑:"Search computers","操作未获允许。需要时可以重新发起。":"This action was not allowed. You can request it again when needed.",操作键盘和鼠标:"Use keyboard and mouse",支持键鼠控制:"Keyboard and mouse available","无法复制，请手动输入配对码。":"Could not copy. Enter the pairing code manually.",未安装:"Not installed",本次共享的配对码:"Pairing code for this session","正在下载安装…":"Downloading and installing…",正在接收画面:"Receiving video","正在连接…":"Connecting…","此工作区还不支持远程桌面，请更新工作区服务。":"This workspace does not support Remote Desktop yet. Update its server.",没有匹配的电脑:"No matching computers","没有收到画面。请在远端授予屏幕录制权限，并确认桌面已解锁。":"No video received. Allow screen recording on the remote computer and make sure its desktop is unlocked.","界面随 App 切换语言和深浅色。采集与传输在独立组件中运行。":"Language and appearance follow the App. Capture and streaming run in a separate component.",组件版本:"Component version",组件设置:"Component settings","被控端目前支持 Mac 和 Windows。":"Mac and Windows hosts are supported at this time.",让你的另一台电脑连进来:"Connect from your other computer","请只交给你信任的人。每次重新开启共享都会更换。":"Share only with someone you trust. The code changes every time you start sharing.","请在支持远程桌面的新版 myco 桌面 App 中打开。":"Open this in a newer myco desktop App that supports Remote Desktop.","输入远端「共享这台电脑」页面上的配对码。":"Enter the code shown on the remote computer's Share this computer page.",返回:"Back",还没有电脑在共享:"No computers are sharing yet",这台电脑正在共享:"This computer is sharing","这台电脑正在被连接，请稍后再试。":"This computer is already in use. Try again later.",远控组件:"Remote component",远程桌面:"Remote Desktop",远程桌面功能:"Remote Desktop modes",远程桌面画面:"Remote desktop video","远程画面，点击后使用键盘和鼠标":"Remote screen. Click to use the keyboard and mouse.","远端尚未允许键鼠控制，或未授予辅助功能权限。可以改用仅查看。":"The host has not allowed control or is missing Accessibility permission. Try view-only instead.","远端已结束共享。":"The remote computer stopped sharing.","连接你的电脑，接着手头的工作。":"Connect to your computer. Pick up where you left off.","连接到 {0}":"Connect to {0}","连接前 myco 会再次向你确认授权。":"myco will ask for your permission before connecting.",连接只由你开启:"You decide when to connect","连接已断开，请确认远端仍在共享后重新连接。":"Disconnected. Make sure the remote computer is still sharing, then connect again.",连接电脑:"Connect to a computer","连接超时。请确认两台电脑在同一局域网，或已通过 Tailscale 连通。":"Connection timed out. Use the same local network or connect both computers through Tailscale.","配对尝试过多，请等一分钟再试。":"Too many pairing attempts. Wait a minute and try again.",配对码:"Pairing code","配对码不对，请查看远端电脑上当前显示的配对码。":"The pairing code is incorrect. Check the code currently shown on the remote computer.","需要远端当前的配对码。画面和键鼠通过独立加密通道传输，不经过聊天消息。":"Requires the host's current pairing code. Video and input use a separate encrypted channel, outside chat messages.","默认仅查看。Mac 需要给远控组件授予辅助功能权限。":"View-only by default. On Mac, give the remote component Accessibility permission to enable control.","组件支持 Apple 芯片或 Intel Mac，以及 x64 Windows。":"Hosts support Apple silicon or Intel Macs, and x64 Windows.","远控组件暂时无法下载，请稍后重试或使用已发布组件的新版 App。":"The remote component is unavailable for download. Try later or use an App build with published components.","组件校验失败，安装已取消。请重试，不要手动运行下载文件。":"Component validation failed. Installation was cancelled. Try again; do not run downloaded files manually.","这台电脑已停止共享，请刷新列表后重试。":"This computer stopped sharing. Refresh the list and try again.","Mac 首次使用要允许屏幕录制；锁屏、登录和管理员确认窗口暂不支持。":"On Mac, allow screen recording first. Lock, sign-in and administrator confirmation screens are not supported yet.","远端组件需要更新才能输入这段文字，文字已保留。":"Update the remote component to enter this text. Your text has been kept.","触控板：单指移动，轻点单击，双指滚动。":"Touchpad: slide to move, tap to click, use two fingers to scroll.",键盘:"Keyboard",右键:"Right click",拖动:"Drag",结束拖动:"End drag",输入文字到远端:"Type on the remote computer",键入或粘贴文字:"Type or paste text","先松开组合键，再输入这段文字。":"Release the modifier keys before entering this text.","一次最多输入256个字符，文字已保留。":"Enter up to 256 characters at a time. Your text has been kept.",放大画面:"Zoom in",缩小画面:"Zoom out",画面缩放:"Zoom","手机仅作为控制端，关闭页面即断开。":"This phone is a controller. Closing the page disconnects.",展开画面:"Expand desktop",收起画面:"Collapse desktop","单指移动指针，轻点单击；双指滚动，捏合缩放。":"Slide to move the pointer, tap to click. Use two fingers to scroll or pinch to zoom.","仅查看：双指捏合缩放，放大后单指移动画面。":"View only: pinch to zoom, then slide with one finger to pan.","这台电脑只允许查看。要操作键鼠，请在电脑上停止共享，勾选「允许键盘和鼠标操作」，再重新共享并连接。Mac 还需允许辅助功能权限。":"This computer allows viewing only. On the computer, stop sharing, turn on “Allow keyboard and mouse”, then share and connect again. Mac also requires Accessibility permission.","仅查看画面，可捏合缩放":"View-only desktop, pinch to zoom","文字未能全部送出，请检查连接后再确认远端内容。":"Text could not be fully sent. Check the connection and remote content before retrying.","单指移动指针，轻点单击；双指缩放和移画面。":"One finger moves the pointer; tap to click. Two fingers zoom and pan the view.",双指滚动:"Two-finger scroll",即时输入:"Live typing",即时输入到远端:"Type directly into the remote computer","先写好，再发送；也可用键盘麦克风听写":"Write, then send. You can also dictate with the keyboard microphone.",发送文字:"Send text",收起键盘:"Hide keyboard","先点远端输入位置；发送文字不会额外按回车。":"Focus the remote field first. Sending text does not add Enter.",切回双指缩放:"Switch back to pinch zoom","双指滚动 · 点此缩放":"Scroll mode · Tap to zoom","双指滚动远端；要缩放请关闭滚动模式。":"Two fingers scroll the remote computer. Turn scroll mode off to zoom.","双指捏合缩放；双指按住片刻再拖动，滚动远端。":"Pinch to zoom. Hold two fingers briefly, then drag to scroll remotely.","远端组件较旧，只支持短英文；更新后可发送中文。":"Older remote component: short ASCII text only. Update it for other languages.",操作与画面:"Controls and display",收起工具:"Hide tools",滚动:"Scroll","图片太大，放不进视频里":"The picture is too large to keep in the video",还没有场景:"There are no scenes yet","有场景的动画代码出错了，先修好再导出：{0}":"A scene's animation code has an error. Fix it before exporting: {0}",已删除场景:"Scene deleted",动效视频:"Motion video","让 agent 做":"Ask an agent",场景:"Scenes",添加场景:"Add a scene","还没有场景。从左边「添加场景」或中间选一个开头。":"No scenes yet. Use “Add a scene” on the left, or pick a start in the middle.",上移:"Move up",下移:"Move down",在后面添加场景:"Add a scene after","场景 {0}":"Scene {0}",选一个场景:"Pick a scene",做一个动效视频:"Make a motion video","从一个现成的开头改文字和图片，或者说清楚要什么，让 agent 来做。":"Start from a ready-made video and change its words and pictures, or say what you want and let an agent make it.",从一个场景开始:"Start with one scene",回到开头:"Back to the start","暂停（空格）":"Pause (Space)","播放（空格）":"Play (Space)",拖动改时长:"Drag to change the length","时长（秒）":"Length (seconds)",进场转场:"Transition in","动画代码出错：{0}":"Animation code error: {0}","这个场景没有可以在这里改的文字。打开下面的代码改，或者让 agent 改。":"This scene has no text to change here. Edit its code below, or ask an agent.",代码:"Code",换一张:"Replace",选图片:"Choose a picture",移除:"Remove",图片没加上:"The picture was not added",动画:"Animation",强调色:"Accent",第二强调色:"Second accent",配色:"Colours",字体:"Font","配色和字体对所有场景生效。":"Colours and font apply to every scene.","请在「动效视频」网页里做视频。":"Please make a video in the Motion video page.","网页 {0}「{1}」。":"Page {0}, “{1}”.","只改场景 {0}（第 {1} 个，「{2}」）。":"Change only scene {0} (number {1}, “{2}”).","要求：{0}":"What I want: {0}","（写在这里）":"(write it here)","按这个网页的 agent.md：场景写在 scenes（html、css、script、fields、duration、transition），比例和配色在 project。写完回我一句。":"Follow the page's agent.md: scenes go in scenes (html, css, script, fields, duration, transition); the ratio and colours are in project. Reply to me when it is done.","已经发给 agent，做好后这里会自动更新":"Sent to the agent. This page updates as it works","说清楚视频讲什么、给谁看、多长、什么风格。agent 会写好场景放进来，你再改文字和时长。":"Say what the video is about, who it is for, how long and in what style. The agent writes the scenes here; then you adjust the words and lengths.","比如：给我们的记账 App 做一个 20 秒的竖版宣传，突出「拍照记账」和「每月报表」，活泼一点":"For example: a 20-second vertical promo for our expense app, showing “snap a receipt” and “monthly reports”, upbeat","只改选中的场景「{0}」":"Change only the selected scene “{0}”",发给这个网页的发布者:"Goes to whoever published this page",复制这段话:"Copy the message","发给 agent":"Send to agent","在这台电脑上一帧一帧画出来，不上传。":"Drawn frame by frame on this computer; nothing is uploaded.","已存到 {0}（{1} MB，用了 {2}）":"Saved to {0} ({1} MB, took {2})","视频做好了，但没存到「下载」：":"The video is ready but was not saved to Downloads:",直接切:"Cut",淡入:"Fade",推入:"Push",上推:"Push up",缩放:"Zoom",擦除:"Wipe",圆体:"Rounded",大标题逐字升起:"A big title rising letter by letter",小标题:"Kicker","让 agent 一起做事":"Agents that work together",副标题:"Subtitle","一个地方，看见每个 agent 在做什么":"One place to see what every agent is doing",冲击字:"Punch words",几个词一个接一个砸出来:"A few words slammed in one after another","第 1 个词":"Word 1",不用装:"No setup","第 2 个词":"Word 2",不用等:"No waiting","第 3 个词":"Word 3",打开就能做:"Just open it",三个要点:"Three points",标题加三张卡片依次出现:"A heading and three cards in turn","三件事，一个地方":"Three things, one place","第 1 条":"Point 1",派活:"Hand out work","第 1 条说明":"Point 1 detail","把任务发给最合适的 agent":"Send each task to the right agent","第 2 条":"Point 2",盯进度:"Follow progress","第 2 条说明":"Point 2 detail","谁在做什么，做到哪了":"Who is doing what, and how far along","第 3 条":"Point 3",拿结果:"Get results","第 3 条说明":"Point 3 detail",做完的东西直接能用:"Finished work, ready to use",数据:"Numbers","数字滚动，柱子按数值长出来":"A number counts up; bars grow to their values",大数字:"Big number",单位:"Unit","条消息每天在 agent 之间往来":"messages a day between agents","第 1 项数值":"Value 1","第 1 项":"Label 1","1 月":"Jan","第 2 项数值":"Value 2","第 2 项":"Label 2","2 月":"Feb","第 3 项数值":"Value 3","第 3 项":"Label 3","3 月":"Mar","第 4 项数值":"Value 4","第 4 项":"Label 4","4 月":"Apr",金句:"Quote",一句话像读出来一样逐字亮起:"A sentence lighting up as if read aloud","好的工具，让人和 agent 都专注在真正重要的事上。":"Good tools let people and agents focus on what really matters.",署名:"Attribution","myco 团队":"The myco team",图片展示:"Picture","截图或产品图缓缓推近，旁边配文字":"A screenshot or product shot slowly zooming, with words beside it",对话:"Chat","所有 agent，一眼看清":"Every agent at a glance","谁在做什么、卡在哪里、做完了什么，都在一页里":"Who is doing what, what is stuck and what is done, on one page",三个步骤:"Three steps","一条线连起三步，依次点亮":"A line joining three steps that light up in turn",三步开始:"Start in three steps","第 1 步":"Step 1","装上 App":"Install the App","第 1 步说明":"Step 1 detail","Mac 和 Windows 都行":"On Mac or Windows","第 2 步":"Step 2","连上 agent":"Connect agents","第 2 步说明":"Step 2 detail","Claude Code、Codex 一条命令":"Claude Code or Codex in one command","第 3 步":"Step 3",开始派活:"Hand out work","第 3 步说明":"Step 3 detail","说一句话，它们就动起来":"Say a word and they get going",片尾:"End card","Logo、名字和一句话，最后定格":"Logo, name and a line to end on",一句话:"Tagline","让你的 agent 们一起干活":"Get your agents working together",网址或按钮:"Link or button",产品宣传片:"Product promo","标题、要点、数据、截图、步骤、片尾，约 28 秒":"Title, points, numbers, screenshot, steps, end card; about 28 seconds",竖版短视频:"Vertical short","冲击字开头，适合抖音、视频号，约 17 秒":"Opens with punch words, for TikTok or Reels; about 17 seconds",数据快报:"Data story","一个结论、一组数据、一句总结":"A headline, a set of numbers, a closing line",未命名模板:"Untitled template",这不是动效视频的模板文件:"This is not a Motion video template file",批注:"Notes",分镜:"Shots","这个浏览器不能编码声音，视频没有声音。":"This browser cannot encode sound, so the video has none.","这台电脑上没有「{0}」，它没混进去。":"“{0}” is not on this computer, so it was left out of the sound.","已存成场景模板，「添加场景」里能找到":"Saved as a scene template; find it under Add scene",已存成模板:"Saved as a template","{0} 条没处理":"{0} open",存成场景模板:"Save as scene template",存下的场景:"Saved scenes",存下的模板:"Saved templates","{0} 个场景":"{0} scenes",现成的开头:"Ready-made starts",照一段视频做:"Copy a video","在这里留批注（N）":"Leave a note here (N)","批注：{0}":"Note: {0}","拖动改时长（会吸到节拍上，按住 Alt 不吸）":"Drag to change the length (snaps to beats; hold Alt to move freely)","这台电脑上没有「{0}」":"“{0}” is not on this computer","有想照着做的视频？在「分镜」里拆开交给 agent":"Have a video to copy? Take it apart under Shots for the agent","（场景 {0}「{1}」）":" (scene {0} “{1}”)","请按批注改「动效视频」网页里的视频。":"Please change the video in the Motion video page as the notes ask.","没处理的批注（时间是视频里的时间）：":"Open notes (times are in the video):","按这个网页的 agent.md 改 scenes（和 project），只改批注说到的地方；改完一条就把那条批注（notes）的 done 设成 true。写完回我一句。":"Follow this page's agent.md: change scenes (and project) only where the notes say; when a note is handled, set its done (in notes) to true. Reply with a line when you are done.","在 {0} 留一条批注，比如「这里慢一点」「标题换成…」":"A note at {0}, like “slower here” or “change the title to…”",加批注:"Add note","把播放头放到要改的地方，写下要怎么改。批注会挂在时间线上，攒几条后一起让 agent 改。":"Put the playhead where something should change and write what. Notes sit on the timeline; collect a few, then have an agent make them.",删除批注:"Delete note","让 agent 改这 {0} 条":"Have an agent make these {0}","复制给 agent 的话（{0} 条）":"Copy the request for an agent ({0})",改:"Edit",写分镜:"Write shots","## 1. 开头 0–3 秒\n- 画面：…\n- 文字：…\n- 动效：…":`## 1. Opening 0–3 s
- Picture: …
- Words: …
- Motion: …`,"分镜写下每个场景讲什么、多长、怎么动，还有哪些要保留（品牌色、节奏）。让 agent 做视频时它会先写好这里；改视频时它也照着这里。":"The shot plan says what each scene shows, how long, how it moves, and what must stay (brand colours, pacing). An agent writes it first when making a video, and follows it when changing one.","拆好了：{0} 个关键帧和台词":"Done: {0} key frames and the lines","拆好了 {0} 个关键帧；没转台词：{1}":"Done: {0} key frames; no lines: {1}","拆好了 {0} 个关键帧（没听到说话）":"Done: {0} key frames (no speech heard)",没拆成:"Could not take it apart","请照参考视频在「动效视频」网页里做一个视频。":"Please make a video in the Motion video page after a reference video.","参考视频「{0}」（{1}）已经拆好：关键帧在 ref_frames（time 是参考视频里的秒数，data 是 JPEG 的 data URL，存成文件再看），台词在 reference 的 transcript。":"The reference video “{0}” ({1}) is taken apart: key frames are in ref_frames (time is seconds of the reference, data is a JPEG data URL: save it to a file to look at it), the lines are reference.transcript.","照它的镜头、节奏和动效重做，内容换成：{0}":"Remake it with its shots, pacing and motion, with this content: {0}","（照原样）":"(as it is)","先把逐镜的分镜写进 project.motion，再按 agent.md 写 scenes。写完回我一句。":"First write the shot-by-shot plan into project.motion, then write the scenes following agent.md. Reply with a line when you are done.",找镜头:"Finding shots",读声音:"Reading sound",转台词:"Transcribing",参考视频:"Reference video","选一段喜欢的视频，在这台电脑上拆出每个镜头的画面和台词，交给 agent 照着做。视频不会上传。":"Pick a video you like: this computer takes out a frame of each shot and its lines for an agent to copy. The video is not uploaded.","{0} 个镜头":"{0} shots"," · 有台词":" · with lines",台词:"Lines","内容换成什么，比如：换成我们的记账 App，突出拍照记账；保留它的节奏和转场":"What to put in it, e.g. our budgeting app, photo receipts first; keep its pacing and transitions","让 agent 照着做":"Have an agent copy it",换一段参考视频:"Pick another reference",选参考视频:"Pick a reference video",这个文件读不出声音:"No sound could be read from this file",没加上:"Not added",加音乐:"Add music",加配音:"Add voice","音乐和配音文件只存在这台电脑上，网页里只记它们放在哪。加了音乐会自动找节拍，场景切换可以对上节拍；配音可以在本机转成字幕。":"Music and voice files stay on this computer; the page keeps only where they play. Music gets its beats found so scene cuts can land on them; voice can become captions on this computer.",音乐:"Music",配音:"Voice","这台电脑上没有这个文件（只存在加它的那台电脑上）。":"This file is not on this computer (only on the one that added it).",选这个文件:"Pick the file",从第几秒开始:"Starts at (s)",跳过文件开头:"Skip the file's first (s)","音量 {0}%":"Volume {0}%","淡入（秒）":"Fade in (s)","淡出（秒）":"Fade out (s)","{0} 拍每分钟 · 视频里 {1} 拍":"{0} BPM · {1} beats in the video",没找到节拍:"No beat found","{0} 个场景的切换对上了节拍":"{0} scene cuts moved onto the beat",场景切换已经在节拍上:"Scene cuts are on the beat already",切换对上节拍:"Cut on the beat","视频 {0} 结束，后面的 {1} 不会导出。":"The video ends at {0}; the last {1} is not exported.","它在视频结束之后，听不到。":"It starts after the video ends, so it is not heard.",底部:"Bottom",字幕条:"Box",顶部:"Top",不显示:"Off",小:"S",大:"L",下载语音识别:"Downloading speech recognition",没下成:"Download failed",这台电脑上没有这个配音文件:"This voice file is not on this computer","识别「{0}」":"Transcribing “{0}”","加了 {0} 条字幕":"Added {0} captions",没听到说话:"No speech heard",没识别成:"Could not transcribe","配音转字幕要先下载语音识别（约 {0} MB，只下一次）":"Captions from voice need speech recognition (about {0} MB, downloaded once)","从「{0}」生成字幕":"Captions from “{0}”","在 {0} 加一条":"Add one at {0}","加了配音就能自动生成字幕；也可以在这里一条条加。字幕会画进导出的视频里。":"Add a voice track to make captions from it, or add them here one by one. Captions are drawn into the exported video.","字幕 {0}–{1}":"Caption {0}–{1}",模板:"Templates",导入:"Import",模板已导入:"Template imported",模板的名字:"Template name",存:"Save",把这个视频存成模板:"Save this video as a template","存成模板后，打开这个网页的人都能用它做新视频：场景、配色、分镜都在，改文字就行。也能导出成文件，带到别的网页。":"Everyone who opens this page can make new videos from a template: its scenes, colours and shot plan come along, so only the words change. Export it as a file to take it to another page.","已换成模板「{0}」":"Now made from “{0}”",用这个:"Use",已删除模板:"Template deleted","这个视频打不开（换成 mp4 试试）":"This video cannot be opened (try an MP4)","本机还没有语音识别（在「声音」里下载后再试）":"Speech recognition is not on this computer yet (download it under Sound)","在 myco App 里打开才能识别语音":"Open this in the myco App to recognise speech","更新 myco App 后才能识别语音":"Update the myco App to recognise speech","这段文字未输入。请在被控电脑更新远控组件，然后重新连接。":"Text was not entered. Update the remote component on the controlled computer, then reconnect.",清除并关闭:"Clear and dismiss","先停止共享，再更新组件。":"Stop sharing before updating the component.","按当前桌面 App 配套的版本安装，不是自动追踪最新组件；如仍不支持所需功能，请先更新桌面 App。更新失败不会卸载旧组件。":"Installs the component version paired with this desktop App, not the latest published version automatically. Update the desktop App first if a feature is missing. A failed download keeps the old component.","安装 App 配套版本":"Install App-compatible version",重点放大:"Focus zoom","在要强调的位置添加放大，拖动画面中的方框选择焦点。预览和导出都会平滑放大再还原。":"Add a zoom where it matters. Drag the box to pick a focus point. Preview and export smoothly zoom in and back out.",添加重点放大:"Add focus zoom",拖动画面中的方框选择焦点:"Drag the box in the preview to choose a focus point.",放大倍率:"Zoom","开始（秒）":"Start (seconds)","结束（秒）":"End (seconds)","导入没完成，原片已保留，可以重试。":"Import did not finish. The original is kept; try importing again.","原片已保存在本机，但还没导入。":"The original is saved on this computer but has not been imported.","当前窗口不支持录屏。请在新版 Mac App 中使用，或用 Chrome / Edge 打开后录制；也可以导入已有视频。":"This window cannot record screens. Use an updated Mac App or Chrome / Edge. You can also import an existing video.","录制中断，已保留收到的片段。":"Recording was interrupted. Received footage has been kept.","没有录到画面，请重新选择屏幕。":"No footage was recorded. Please select a screen again.","录屏未获允许或已取消，请重新选择。":"Recording was denied or cancelled. Please select again.",录制中:"Recording",开始录屏:"Record screen",停止并剪辑:"Stop & edit","录制 → 剪辑 → 导出":"RECORD → EDIT → EXPORT",录下你的下一次演示:"Your next demo starts here","视频留在这台电脑。停止录制后会自动放进时间线，可以裁剪、加文字和重点放大。":"Your video stays on this computer. Stop recording to trim, add text and focus zooms on the timeline.",由系统选择录制区域:"Choose a recording region","取消选择则录制主屏幕；系统工具栏也可以停止录制。":"Turn off to record the main screen. You can also stop using the system toolbar.",同时录制麦克风:"Record microphone","默认不录声音；此版本不采集电脑内部声音。":"Sound is off by default. This version does not capture internal system audio.","由浏览器选择屏幕或窗口；声音是否可用取决于系统和你选择的共享选项。":"Choose a screen or window in the browser. Audio availability depends on your system and sharing options.",查看本机原片:"Show original file",重新导入原片:"Retry import",选择画面并开始:"Choose screen & record","正在导入原片…":"Importing recording…",请在系统窗口中选择画面:"Choose what to record in the system picker","正在录制，可以切到其他窗口":"Recording. You can switch to other windows.","此版本录制 Windows 整个桌面，不采集声音。":"This version records the entire Windows desktop without audio.",保存原片:"Save original",未命名录屏:"Untitled recording","录制屏幕，开始创作":"Record your screen. Start creating.","单次最多 10 分钟，浏览器录制和自动导入最多 512 MB。请避免展示密码和私人信息。":"Up to 10 minutes. Browser recordings and automatic imports are limited to 512 MB. Keep private information off screen.","原片超过 512 MB，已保留在本机；请导入较短的视频。":"The original is over 512 MB and is kept on this computer. Import a shorter video.","无法连接这台电脑的录屏功能，请关闭应用页面后重新打开。":"Cannot reach this computer’s recording features. Close and reopen this app page.","这台 Windows 电脑还没有配置录屏所需的 FFmpeg。可以先导入已有视频进行剪辑。":"This Windows computer does not have FFmpeg configured for recording yet. You can import an existing video to edit.","当前打开的 App 还不支持原生录屏。请在新版 myco 或 myco-preview 中重新打开本应用；这不是系统权限被拒绝。":"This App does not support native screen recording yet. Reopen this app in an updated myco or myco-preview; this is not a system permission denial.","屏幕选择器没有打开或已被取消。当前嵌入窗口可能不支持浏览器录屏；请在支持原生录屏的 myco App 中重试。":"The screen picker did not open or was cancelled. This embedded window may not support browser recording. Try a myco App with native recording support.","首次使用：在系统设置 → 隐私与安全性 → 屏幕与系统音频录制中，允许当前使用的 myco-preview 或 myco。如系统要求退出并重新打开，请重开后再试。麦克风需另行允许。":"First use: in System Settings → Privacy & Security → Screen & System Audio Recording, allow the myco-preview or myco you are using. If macOS requests a restart, quit and reopen it before retrying. Microphone permission is separate.",打开录屏权限设置:"Open recording permissions","请允许 myco-preview 的屏幕录制权限。点击下方“打开录屏权限设置”，打开当前 App 的开关；如系统要求，请退出并重新打开 App。":"Allow screen recording for myco-preview. Use “Open recording permissions” below and enable the App you are using. Quit and reopen it if macOS requests it.","请输入不带密钥、查询参数或片段的 HTTP(S) 接口地址":"Enter an HTTP(S) API address without secrets, query parameters or a fragment","这个地址没有返回兼容的影视列表，请核对 MacCMS JSON 接口":"This address did not return a compatible catalog. Check the MacCMS JSON API endpoint.","线路 {0}":"Line {0}","第 {0} 集":"Episode {0}",请给影视源起个名字:"Give this source a name",这个影视源已经添加过了:"This source has already been added",已删除影视源:"Source deleted",已移出待看:"Removed from watchlist",这个播放地址不受支持:"This playback address is not supported","连接失败（HTTP {0}），请稍后重试或换个影视源":"Connection failed (HTTP {0}). Try again later or choose another source.","返回的内容太大，请换一条播放线路":"The response is too large. Choose another playback line.","播放线路连接失败，请重试或切换线路":"Could not connect to this playback line. Retry or choose another line.","这个文件无法载入或超过 32 MB，请选择 HLS 播放线路":"This file could not load or exceeds 32 MB. Choose an HLS playback line.","当前 App 不支持分段视频播放，请更新 App 或换到桌面端打开":"This App cannot play segmented video. Update the App or open it on a desktop.","这个视频无法解码，请切换播放线路":"This video could not be decoded. Choose another playback line.",影视播放器:"Video player","正在载入视频…":"Loading video…",重试播放:"Retry playback",上一集:"Previous episode",下一集:"Next episode",跳过片头片尾:"Skip intro and outro","片头（秒）":"Intro (seconds)","片尾（秒）":"Outro (seconds)","填 0 关闭；片尾自动进入下一集。仅保存在这台电脑。":"Enter 0 to disable. The outro advances to the next episode. Saved only on this computer.","从 {0} 继续":"Resume from {0}",影视源已移除:"Source removed",打开影片:"Open film",影视源:"Sources","影视源连接失败，请检查地址后重试":"Could not connect to the source. Check its address and try again.","添加自己的 MacCMS JSON 接口，不预设任何影视源。影视源与待看清单由此页面共享，请勿填写密钥。":"Add your own MacCMS JSON API. No sources are preset. Sources and the watchlist are shared by this page; do not enter secrets.",已启用:"Enabled",编辑影视源:"Edit source",添加影视源:"Add source",影视源名称:"Source name","例如：我的片库":"For example: My catalog",接口地址:"API address","支持 MacCMS 兼容的 JSON 接口和 HLS 播放线路，不是网站首页地址。":"Use a MacCMS-compatible JSON API with HLS playback lines, rather than a website home page.",启用这个影视源:"Enable this source","正在测试连接…":"Testing connection…",接口可用:"API is available",测试连接:"Test connection",保存影视源:"Save source","这个影视源已移除或停用，请在影视源设置中重新添加或启用":"This source was removed or disabled. Add it again or enable it in Sources.","影视源已找不到这部影片，请重新搜索":"This film is no longer in the source. Search again.","影片载入失败，请重试或换个影视源":"Could not load this film. Try again or choose another source.",已删除播放记录:"Playback history removed",影视:"Cinema","你的片库，自己的影视源":"Your films, your sources",影视内容:"Cinema sections",发现:"Discover",待看:"Watchlist",最近播放:"Recently played",播放进度仅保存在这台电脑:"Playback progress stays on this computer",选择影视源:"Choose source",所有影视源:"All sources",搜索影片:"Search films","搜索电影、剧集":"Search movies and series",清除搜索:"Clear search",影片分类:"Film category",全部分类:"All categories",还没有启用的影视源:"No sources are enabled",先添加你的影视源:"Add your first source","接入自己的片库后，就能搜索影片、选择集数并继续观看。":"Connect your catalog to search films, choose episodes and resume watching.",管理影视源:"Manage sources","支持 MacCMS JSON 接口，不内置影视源":"Supports MacCMS JSON APIs. No sources are bundled.","正在搜索影片…":"Searching films…","正在载入片库…":"Loading catalog…",先看待看清单:"Open watchlist",暂时连不上影视源:"Could not connect to the source","「{0}」的搜索结果":"Results for “{0}”",片库更新:"Catalog updates","第 {0} 页":"Page {0}","第 {0} / {1} 页":"Page {0} of {1}",搜索所有影视源:"Search all sources",没有找到影片:"No films found","输入片名，在已启用的影视源里一起搜索。":"Enter a title to search all enabled sources.","换个关键词、分类或影视源再试试。":"Try a different keyword, category or source.",留给下一次的好片:"Saved for another evening",接着上次看:"Pick up where you left off",与此页面共享:"Shared by this page",仅保存在这台电脑:"Saved only on this computer",待看清单还是空的:"Your watchlist is empty",还没有播放记录:"No playback history yet","打开一部影片，点「加入待看」留到下次。":"Open a film and choose Add to watchlist to save it for later.","开始观看后，会在这里记住集数和进度。":"Once you start watching, your episode and progress will appear here.",去发现影片:"Discover films","正在载入影片…":"Loading film…",已加入待看:"On watchlist",加入待看:"Add to watchlist","这部影片还没有可用的播放线路，请换个影视源搜索。":"This film has no playback lines yet. Search another source.",剧情简介:"Synopsis",暂无简介:"No synopsis available","主演：{0}":"Cast: {0}","导演：{0}":"Director: {0}",播放线路:"Playback line",选集:"Episodes","共 {0} 集":"{0} episodes","优先选择 HLS 线路；空格暂停，左右键快进或快退。":"Choose an HLS line. Space pauses; Left/Right seeks.","{0} 个影视源暂时无法连接，已显示其余结果":"{0} sources could not connect. Results from the others are shown.",接口地址无效:"Invalid API address",更多操作:"More actions",已停用:"Disabled","新线路没有同名集数，已选第一集":"No matching episode on this line. The first episode is selected.","当前 myco App 尚不支持应用联网，请更新 App 后再试":"This myco App does not yet support network access for apps. Update the App and try again.","请在 myco App 中打开，才能让应用联网":"Open this page in the myco App to give it network access",框选录制区域:"Choose a recording region","拖动框选后点击开始录制；App 会暂时隐藏，点击菜单栏“停止录制”返回剪辑。":"Drag a region and start recording. The App hides; use the floating Stop & edit button to return.","区域框选需要更新 myco App；当前可以取消此选项录制主屏幕。":"Region selection needs an updated myco App. You can record the main screen with this option off.","已取消选择，尚未生成视频。":"Selection cancelled; no video was recorded.","请拖动框选区域，再点击开始录制":"Drag a region, then click Start Recording","区域框选需要更新 myco App；当前将录制主屏幕。":"Update myco App to select a region; this App will record the main screen.","正在选择区域…":"Selecting a region…","缓慢进入，停留，再平滑还原；短片段会自动缩短过渡。":"Ease in, hold, then return smoothly. Short clips fit the transitions automatically.","放大过渡（秒）":"Zoom-in transition (s)","还原过渡（秒）":"Zoom-out transition (s)","拖动框选后点击开始录制；录制时保留区域边框，点击悬浮条“停止并剪辑”返回。":"Drag a region and start recording. Its outline stays visible; click Stop & edit on the floating bar to return.",中性灰:"Warm gray",奶油黄:"Cream",雾玫瑰:"Rose",雾蓝:"Mist blue",鼠尾草:"Sage",浅紫:"Lavender",杏色:"Apricot",绘图工具:"Drawing tools",选择样式:"Selection style","已选 {0} 项":"{0} selected",连线:"Connector",连线类型:"Connector type",曲线:"Curve",直线:"Straight",线条样式:"Line style",实线:"Solid",虚线:"Dashed",连线文字:"Connector label",添加说明:"Add a label",文字对齐:"Text alignment",左对齐:"Align left",右对齐:"Align right",排列元素:"Arrange elements",排列:"Arrange",水平居中:"Center horizontally",垂直居中:"Center vertically",顶端对齐:"Align top",底端对齐:"Align bottom",水平等距:"Distribute horizontally",垂直等距:"Distribute vertically",至少选择三个元素:"Select at least three elements",创建副本:"Duplicate","创建副本（⌘/Ctrl+D）":"Duplicate (⌘/Ctrl+D)",锁定:"Lock",解锁:"Unlock",从一个想法开始:"Start with an idea","双击贴便签，拖出连接点画流程。":"Double-click for a note. Drag a connection point to build a flow.","也可以粘贴文字，或让 agent 帮你整理想法。":"Paste text or ask an agent to organize your ideas.","请使用 ⌘/Ctrl+V 粘贴，或在画布中长按粘贴":"Use ⌘/Ctrl+V, or touch and hold on the canvas to paste.","请使用 ⌘/Ctrl+C 复制":"Use ⌘/Ctrl+C to copy.",已剪切:"Cut",剪切:"Cut",粘贴:"Paste",全选:"Select all",置于顶层:"Bring to front",置于底层:"Send to back","撤销（⌘/Ctrl+Z）":"Undo (⌘/Ctrl+Z)","重做（⌘/Ctrl+Shift+Z）":"Redo (⌘/Ctrl+Shift+Z)",操作帮助:"Help and shortcuts","导出 SVG":"Export SVG",白板画布:"Whiteboard canvas",拖动起点:"Drag start point",拖动终点:"Drag end point",调整大小:"Resize","从{0}侧连接":"Connect from {0}",左:"left",右:"right","选择目标，或在空白处创建节点":"Choose a target, or release on empty canvas to add a node","拖动框选 · 空格平移 · 双击编辑":"Drag to select · Space to pan · Double-click to edit",让想法自然成形:"Give your ideas room","拖动框选，Shift 加选；拖出元素边缘的连接点即可连线。":"Drag to select, Shift to add. Drag an edge connection point to connect elements.","⌘/Ctrl+C / X / V 复制、剪切、粘贴；⌘/Ctrl+D 创建副本。":"⌘/Ctrl+C / X / V to copy, cut and paste; ⌘/Ctrl+D to duplicate.","⌘/Ctrl+Z 撤销；Shift+⌘/Ctrl+Z 重做；方向键微调，Shift 移动 10 像素。":"⌘/Ctrl+Z to undo; Shift+⌘/Ctrl+Z to redo. Arrow keys nudge; Shift moves 10 pixels.","空格或中键平移，⌘/Ctrl+滚轮缩放；手机双指缩放和平移。":"Space or the middle mouse button to pan; ⌘/Ctrl+wheel to zoom. Use two fingers on a phone.","拖动时自动吸附对齐，按住 Alt 暂时关闭；Shift 调整大小保持比例。":"Dragging snaps to alignment guides; hold Alt to disable. Shift-resize keeps proportions.","这些元素已被其他人修改，未覆盖最新内容":"These elements changed since your edit. Their latest content was kept.",折线连接:"Elbow",粘贴到白板:"Paste onto the board","在下方长按并选择「粘贴」，即可插入复制的元素或文字。":"Touch and hold below, then choose Paste to insert your copied elements or text.",在此使用系统粘贴:"Use system paste here","检查标题、时长和预约范围，再保存。":"Check the title, duration and booking range, then save.","时段不能重叠，结束时间要晚于开始时间。":"Time ranges must not overlap, and must end after they start.","填写主持人和有效时区。":"Enter a host name and a valid time zone.","日历时区在创建后固定；访客可以切换显示时区。":"The calendar time zone is fixed after setup. Visitors can change their display time zone.","这一天的预约记录已满，请选择其他日期。":"This day has reached its booking history limit. Choose another date.","没有保存预约，请检查最新时段后重试。":"The booking was not saved. Check the latest times and try again.","请等上一项预约保存完成。":"Wait for the previous booking change to finish.","这个时段已不可预约，请重新选一个。":"This time is no longer available. Choose another one.","填写姓名和有效邮箱，备注不超过 1000 字。":"Enter your name and a valid email. Keep notes under 1,000 characters.","没有找到这项预约，请刷新页面。":"This booking could not be found. Refresh the page.","原时段已经被占用，无法恢复预约。":"This time has been taken. The booking cannot be restored.",已取消预约:"Booking cancelled",时区:"Time zone",编辑预约类型:"Edit event type",新建预约类型:"New event type",预约名称:"Event name","例如：30 分钟交流":"e.g. 30-minute conversation","时长（分钟）":"Duration (minutes)",可预约未来几天:"Days into the future",地点或会议链接:"Location or meeting link","例如：线上会议，或填写地址":"e.g. Video call or a street address","至少提前（分钟）":"Minimum notice (minutes)","会前留空（分钟）":"Buffer before (minutes)","会后留空（分钟）":"Buffer after (minutes)",开放预约:"Accept bookings","所有类型共用一位主持人的时间；留空时间也会避开其他预约。":"All event types share one host. Buffers also block other bookings.","没保存上，请重试。":"Could not save. Try again.",不开放:"Unavailable",移除时段:"Remove time range",添加时段:"Add time range",每周可用时间:"Weekly availability","按 {0} 安排，所有预约类型共用。":"Scheduled in {0}, shared by all event types.",例外日期:"Date overrides","假日休息，或为某一天单独安排时间。例外会替代每周安排。":"Take a day off or set special hours. Overrides replace the weekly schedule.",添加日期:"Add date",移除例外:"Remove override",可用时间已保存:"Availability saved",预约主页:"Booking profile",页面名称:"Page name",我的预约日历:"My booking calendar",主持人:"Host",自我介绍:"About you",日历时区:"Calendar time zone","这是共享页面内的预约。能打开此页面的人和 agent 都能查看预约信息；不会发送邮件，也不会同步外部日历。":"Bookings are shared within this page. People and agents who can open it can view booking details. No email is sent, and external calendars are not synced.",返回预约类型:"Back to event types","这个预约类型已不存在。":"This event type no longer exists.",预约已确认:"Booking confirmed","时间已经为你留好。可下载日历文件，或在「预约记录」里查看与取消。":"Your time is reserved. Download a calendar file, or view and cancel in Bookings.","不会发送确认邮件；请保存日历文件。":"No confirmation email is sent. Save the calendar file.",添加到日历:"Add to calendar",显示时区:"Display time zone","只有当前页面的预约会占用时间。":"Only bookings on this page block time.",选择日期:"Choose a date","按 15 分钟间隔显示可约时段。":"Available starts are shown every 15 minutes.",找下一个可约日期:"Find the next available day","预约范围内没有更多时段，可调整可用时间。":"No more times in the booking window. You can adjust availability.",你的姓名:"Your name","备注（可选）":"Notes (optional)","姓名、邮箱和备注对本页面成员可见。":"Your name, email and notes are visible to page members.",确认预约:"Confirm booking","这一天没有可约时段，试试其他日期。":"No times available on this day. Try another date.","这个类型已暂停预约。":"Bookings for this event type are paused.",即将到来:"Upcoming","搜索姓名、邮箱或预约":"Search name, email or event",搜索预约:"Search bookings",没有匹配的预约:"No matching bookings",这里还没有预约:"No bookings here yet","试试其他关键词。":"Try another search.","在预约类型里选择一个时间，新的预约会出现在这里。":"Choose a time from an event type. New bookings will appear here.","时间按 {0} 显示":"Times shown in {0}",预约详情:"Booking details","访客时区：{0}":"Visitor time zone: {0}",取消预约:"Cancel booking",预约类型:"Event types",预约记录:"Bookings",可用时间:"Availability",日程预约:"Scheduling","和人、agent 共用一份日程。":"One calendar for people and agents.",选择预约时间:"Choose a time","把合适的时间，留给重要的事。":"Make time for what matters.",新建类型:"New event type",让下一次见面更简单:"Make your next meeting easier","先设置主持人和时区，再建立预约类型。你和 agent 都能一起安排。":"Set your host and time zone, then create event types. Plan together with your agents.",设置预约主页:"Set up booking profile","按用途设置时长、地点与留空时间。":"Set durations, locations and buffers for each kind of meeting.",搜索预约类型:"Search event types",地点待定:"Location to be decided",开放中:"Active",建立你的第一个预约类型:"Create your first event type","例如 15 分钟交流、设计评审，或 60 分钟咨询。":"Try a 15-minute conversation, design review or 60-minute consultation.",没有匹配的预约类型:"No matching event types",开放:"Enable",更换:"Change",预约:"Book",导出日历:"Export calendar",开始时间:"Start time",结束时间:"End time","{0} 分钟":"{0} minutes",邮箱:"Email",已确认:"Confirmed",姓名:"Name",未命名:"Untitled","保存失败，可能有人刚修改过。请关闭后重新打开再试。":"Could not save; someone may have changed this. Close and reopen, then try again.","这个时段刚被其他人修改，请重新选择。":"Someone just changed this time. Choose again.","未能确认保存结果，请先查看预约记录。":"The save could not be confirmed. Check Bookings before trying again.","预约数据有误，请让 agent 检查后再预约。":"Some booking data is invalid. Ask an agent to check it before booking.",已开始浏览器下载:"Browser download started",学习工作台:"Learning workspace",学习内容:"Learn",学习主题:"Subjects",未命名主题:"Untitled subject",加一个学习主题:"Add a subject",新主题名称:"New subject name",添加主题:"Add subject",从一个想学的主题开始:"Start with something you want to learn","写下主题，或请 agent 帮你搭一条学习路径。":"Name a subject, or ask an agent to build a learning path.",主题名称:"Subject name","这次想学会什么？":"What do you want to learn?",学习目标:"Learning goal",题已掌握:"questions mastered",学习视图:"Learning views",练习:"Practice",知识卡:"Lesson cards",未命名知识卡:"Untitled lesson card",加一张知识卡:"Add a lesson card",新知识卡标题:"New lesson title",添加知识卡:"Add lesson card",知识卡标题:"Lesson title",讲解:"Explanation","把这个概念讲清楚。也可以让 agent 来写。":"Explain this concept clearly. You can also ask an agent to write it.",记住这句:"Key takeaway",一句话总结:"One-sentence summary",还没有知识卡:"No lesson cards yet","自己写一张，或让 agent 按学习目标生成。":"Write one, or ask an agent to generate cards for your goal.",随堂练习:"Practice questions",题待复习:"questions to review",还没有练习题:"No practice questions yet","添加一道题，或请 agent 根据知识卡出题。":"Add a question, or ask an agent to create one from a lesson.","第 {0} 题":"Question {0}",已掌握:"Mastered",题目内容:"Question prompt",输入题目:"Enter a question","选项 {0} 内容":"Option {0} text",写一个选项:"Write an option",正确选项:"Correct option",提交答案:"Submit answer",答对了:"Correct","可以请 agent 解释这道题。":"Ask an agent to explain this question.","答案讲解（可选）":"Answer explanation (optional)",答案讲解:"Answer explanation",加一道练习题:"Add a practice question",新题目:"New question",添加题目:"Add question",学习进度:"Learning progress",练习题:"Questions",待复习:"To review",复习清单:"Review list",未命名题目:"Untitled question","今天没有待复习的题。":"No questions are due for review today.",编辑题目:"Edit questions",完成编辑:"Done editing",导师对话:"Tutor chat",学习笔记:"Study notes",练习与复习:"Practice & review",讲懂一个概念:"Explain a concept","用一个生活例子解释贝叶斯定理。":"Explain Bayes’ theorem with an everyday example.",一步步学:"Learn step by step","从零开始教我 SQL，每次只讲一步。":"Teach me SQL from scratch, one step at a time.",考考我:"Quiz me","考考我对大语言模型的理解，一次问一题。":"Quiz me on large language models, one question at a time.","请担任我的学习导师。学习页面 ID：{0}。请读取本页 agent.md 和 tutor_requests 中 ID 为 {1} 的记录，结合相关课程和之前的问答，用我提问的语言辅导我。把这一轮的回答写进 tutor_answers，记录 ID 必须与请求相同。\n\n我的问题：{2}":`Please be my tutor. Study page ID: {0}. Read its agent.md and the tutor_requests record with ID {1}. Use the relevant lessons and prior turns to tutor me in the language of my question. Write your answer to tutor_answers using the same record ID as the request.

My question: {2}`,"已复制，粘贴到你常用的 agent 对话。":"Copied. Paste it into a conversation with your agent.","已发给导师。你可以继续看笔记，导师的回答会显示在这里。":"Sent to your tutor. You can keep reading your notes. The answer will appear here.","暂时发不出去。可以复制这条提问，发给你常用的 agent。":"Could not send. Copy this question into a conversation with your agent.","问题没有保存，请检查连接后重试。":"Could not save your question. Check your connection and try again.","今天想学什么？":"What would you like to learn?","让导师讲懂一个概念、带你一步步学，或者出题检验理解。":"Ask for an explanation, learn step by step, or test your understanding.",导师:"Tutor","等待导师回复…":"Waiting for your tutor…",未能确认发送:"Could not confirm delivery","复制提问后，发给你的导师":"Copy your question and send it to your tutor",导师回复比平时久:"Your tutor is taking longer than usual",学习路线:"Learning path","第 {0} 步，共 {1} 步":"Step {0} of {1}","下一步：{0}":"Next: {0}",继续下一步:"Continue to the next step",已学完这条路线:"Learning path complete","每次学一步，做完小测就可以继续。":"Learn one step at a time and continue after a quick check.","请继续学习路线的下一步：{0}。承接上一课，用一个例子讲解，并留一道理解题。":"Continue with the next step: {0}. Build on the last lesson, teach with an example, and add a quick check.","请接着上一课讲下一步，保持学习基础和教材不变。先说明学习路线，再讲当前一步并留一道理解题。":"Continue where the last lesson ended at the same level and with the same material. Outline the learning path, teach the current step, and add a quick check.","请担任我的学习导师。学习页面 ID：{0}。请读取本页 agent.md 和 tutor_requests 中 ID 为 {1} 的记录。按所选方式和基础，用我的语言从直觉和例子讲起；若附了教材，请读对应原文并引用。分步辅导请给出学习路线，若有 previous 则接着上一课讲。把回答写进 tutor_answers，使用同一个记录 ID。\n\n我的问题：{2}":`Be my tutor. Study page ID: {0}. Read agent.md on this page and record {1} in tutor_requests. Follow the selected mode and level and teach in my language, starting with intuition and examples. If a source is attached, read and cite it. In guided mode, include a learning path and continue the preceding lesson when previous is set. Write your answer to tutor_answers using the same record ID.

My question: {2}`,"复制给 agent":"Copy for an agent",移除学习上下文:"Remove study context",问导师:"Ask tutor","输入你想学的内容，或接着追问…":"Ask about a topic, or follow up…",辅导方式:"Tutoring mode","正在准备…":"Preparing…","问题会发给发布这个页面的 agent。":"Questions go to the agent who published this page.","把问题复制到 agent 对话，回答会显示在这里。":"Copy your question into an agent conversation. The answer will appear here.",给导师的消息:"Message for your tutor","先添加学习主题，或让导师帮你制定学习计划。":"Add a subject, or ask the tutor to help you make a plan.",结合当前内容问导师:"Ask about this material",全部练习:"All questions",我的日历:"My calendars",新建日程:"New event",编辑日程:"Edit event",日程标题:"Event title",添加标题:"Add a title",全天:"All day",开始:"Start",结束:"End",日程时区:"Event time zone",所属日历:"Calendar",工作:"Work",生活:"Life",重复:"Repeat",不重复:"Does not repeat",每天:"Daily",每周:"Weekly",每月:"Monthly","重复截止日期（可选）":"Repeat until (optional)","修改、移动或删除会应用到整组重复日程。":"Editing, moving or deleting applies to the entire recurring series.","显示为忙碌，预约时避开":"Show as busy and avoid when booking","请等上一项日程保存完成。":"Wait for the previous event change to finish.","日程保存未确认，请查看最新内容后再试。":"The event save could not be confirmed. Check the latest data before retrying.","检查标题和起止时间；单项日程最长 31 天。":"Check the title and start/end times. An event can last up to 31 days.",日程已保存:"Event saved",已删除日程:"Event deleted","这项日程刚被修改，请重新拖动。":"This event just changed. Drag it again.","这个本地时间不存在，请选择其他时间。":"This local time does not exist. Choose another time.",返回日历:"Back to calendar",预约管理:"Appointments",重复日程:"Recurring event",月视图:"Month view","查看 {0}":"View {0}","另外 {0} 项":"+{0} more","{0} 全天":"{0} all day","{0} 时间轴":"{0} timeline",已确认的预约:"Confirmed appointments","日程随当前页面共享，可与 agent 一起安排。":"Events follow this page’s sharing permissions. Plan together with agents.",日历视图:"Calendar view",日:"Day",导出当前视图:"Export current view",搜索日程:"Search events","部分日程数据有误，请让 agent 检查后再编辑。":"Some event data is invalid. Ask an agent to check it before editing.","没有匹配的日程，试试其他关键词。":"No matching events. Try another search.",管理这项预约:"Manage this appointment","跳到 {0}":"Jump to {0}",地点:"Location",时间:"Time",搜索结果:"Search results",按月:"Month",按周:"Week",按日:"Day","请担任我的学习导师。学习页面 ID：{0}。请读取本页 agent.md 和 tutor_requests 中 ID 为 {1} 的记录。按所选学习基础，用我的语言从直觉和例子讲起；若附了教材，请读对应原文并引用。把分层讲解、交互图和理解检查写进 tutor_answers，使用同一个记录 ID。\n\n我的问题：{2}":`Be my tutor. Study page ID: {0}. Read its agent.md and the tutor_requests record with ID {1}. Teach in my language, starting with intuition and an example at my selected level. If I attached material, read and cite the relevant passage. Write the layered lesson, interactive diagram and understanding check to tutor_answers using the same record ID.

My question: {2}`,"每次最多导入 {0} 页。请拆成几份。":"Import up to {0} pages at a time. Split the file into parts.","教材文字太长。请按章节拆开后再导入。":"This material has too much text. Split it into chapters and try again.","这个 PDF 有密码。请先解锁再导入。":"This PDF is password protected. Unlock it before importing.","读不出这个 PDF。请试试另存为 PDF，或改用文本教材。":"Could not read this PDF. Re-save it as PDF or use a text file.","请选择 PDF、TXT 或 Markdown 文件。":"Choose a PDF, TXT or Markdown file.","文件超过 {0} MB。请按章节拆分。":"This file is larger than {0} MB. Split it into chapters.","没有找到可选中的文字。扫描版 PDF 请先做文字识别。":"No selectable text found. Run text recognition on scanned PDFs first.","教材没有保存成功。请检查连接后重试。":"Could not save the material. Check your connection and try again.",教材库:"Materials","选一页原文，带着它问导师。":"Open a source page and ask your tutor about it.","正在读取教材…":"Reading material…","正在读取第 {0} 页，共 {1} 页":"Reading page {0} of {1}",导入教材:"Import material",选择教材文件:"Choose a file","支持 PDF、TXT 和 Markdown，最大 {0} MB，PDF 最多 {1} 页。":"PDF, TXT and Markdown up to {0} MB. PDF files can have up to {1} pages.","只共享提取的文字，PDF 保留页码。扫描件需要先做文字识别。":"Share extracted text only. PDF page numbers are preserved. Scans need text recognition first.","教材库是空的。导入文件后，教材会显示在这里。":"Imported materials will appear here.",教材原文:"Source text",关闭教材:"Close source","这一页没有可显示的文字。":"There is no text to display on this page.",结合这页问导师:"Ask about this page",移除教材引用:"Remove attached source","第 {0} 段":"Section {0}","{0} 段":"{0} sections",示例课:"Sample lesson",打开示例课:"Open sample lesson",去问导师:"Ask your tutor","切换学习基础，看看同一个问题如何讲给不同的人。":"Switch levels to see different ways of teaching the same question.","延伸阅读：克雷数学研究所":"Further reading at the Clay Mathematics Institute",学习基础:"Learning level",零基础:"Beginner",中学基础:"Secondary school",大学基础:"University","黎曼猜想：从数质数到看零点":"The Riemann hypothesis: From primes to zeros",先找一找:"Find the primes","质数是大于 1、只能被 1 和自己整除的整数。7 是质数，8 不是，因为 8 可以分成 2 × 4。你能从 1 到 20 找出所有质数吗？":"A prime is a whole number greater than 1 that divides evenly only by 1 and itself. Seven is prime. Eight can be split into 2 × 4, so it is composite. Can you find all the primes from 1 to 20?",拉长数表:"Extend the grid","下面的数表会标出质数。把范围从 20 拉到 100，看质数的个数与所有数字的个数如何变化。质数没有用完，但它们占的比例总体会变小。":"The grid highlights the primes. Increase the range from 20 to 100 and compare the number of primes with the total. There are infinitely many primes, and their share of the numbers tends to shrink.","猜想和它有什么关系？":"Where does the hypothesis come in?","数学家可以估算一段很长的数表里有多少质数。黎曼猜想如果成立，我们就能更准确地说，这个估算最多会差多少。它说的是一个特殊函数里的点，这些点会影响质数个数的估算。":"Mathematicians can estimate how many primes there are up to a large number. If the Riemann hypothesis holds, we can put a tighter limit on the estimate’s error. The hypothesis concerns points of a special function that influence that estimate.","哪些数是质数？":"Which numbers are prime?","拖动滑块，看看从 1 数到不同的数时，你能找到多少质数。":"Move the slider to count primes in different ranges.","为什么 9 不是质数？":"Why is 9 composite?","因为 9 = 3 × 3":"Because 9 = 3 × 3","因为 9 是奇数":"Because 9 is odd","因为 9 比 7 大":"Because 9 is greater than 7","3 是 9 除了 1 和它自己之外的因数。":"Three is a factor of nine besides 1 and 9.","零点要排在哪条线上？":"Which line would the zeros lie on?","把复数画成一个点，横坐标是实部。图中的圆点是示意点，移动它可以测试位置条件。":"Plot a complex number as a point. Its horizontal coordinate is the real part. Move the illustrative point to try the position condition.",示意点:"Example point","数到 {0}":"Count to {0}",用实底标出质数:"Primes are highlighted","前 {0} 个正整数里有 {1} 个质数。":"There are {1} primes among the first {0} positive integers.",移动示意点的横坐标:"Move the example point horizontally",点在标出的线上:"Point is on the marked line",点在线外:"Point is off the line","{0} = {1}":"{0} = {1}","若严格找到了实部为 0.6 的非平凡零点，这会怎样？":"What if we rigorously found a nontrivial zero with real part 0.6?",足以推翻猜想:"It would disprove the hypothesis",证明猜想成立:"It would prove the hypothesis",与猜想无关:"It would have no bearing","猜想断言每一个非平凡零点都满足实部等于 1/2，一个反例就足够。":"The claim covers every nontrivial zero. One counterexample is enough.","什么叫零点？":"What is a zero?","代入某个数使函数的值等于 0，这个数就是零点。比如 f(x) = x² − 4 有两个零点，2 和 −2。黎曼研究的 ζ 函数也有零点，只是输入可以画在复数平面上。":"A zero is an input that makes a function equal zero. For example, f(x) = x² − 4 has two zeros, 2 and −2. The zeta function Riemann studied has zeros too, and its inputs can be plotted in the complex plane.",用图读懂猜想:"Read the hypothesis from the diagram","黎曼猜想说，ζ 函数所有非平凡零点的横坐标都是 1/2。拖动下面的示意点，看看 0.5 和 0.6 有什么区别。知道某个点在线上，只说明它的位置符合猜想，要判定它是零点还要算函数值。":"The hypothesis says every nontrivial zero of zeta has horizontal coordinate 1/2. Move the illustrative point between 0.5 and 0.6. A point on the line passes the location test. To know whether it is a zero, you still have to evaluate the function.","为什么值得研究？":"Why does it matter?","零点的位置会影响质数个数估算的误差。就像知道一把尺子最大会量错多少，一旦猜想成立，我们可以给估算加上更严格的误差范围。":"The zeros’ locations affect the error in estimates of prime counts. If the hypothesis holds, we can put a tighter bound on the size of that error, like knowing the maximum error of a measuring tool.",从欧拉乘积出发:"Start from the Euler product","在 Re(s) > 1 时，ζ(s) = Σ n⁻ˢ = ∏ₚ(1 − p⁻ˢ)⁻¹。将各项展开再相乘，每个正整数恰好由它唯一的质因数分解出现一次。这解释了为何 ζ(s) 能携带质数信息。":"For Re(s) > 1, ζ(s) = Σ n⁻ˢ = ∏ₚ(1 − p⁻ˢ)⁻¹. Expanding the product gives every positive integer exactly once by unique prime factorization. This is how ζ(s) encodes information about primes.",延拓与临界线:"Continuation and the critical line","ζ 函数可亚纯延拓到整个复平面，在 s = 1 有一个简单极点。其非平凡零点位于 0 < Re(s) < 1。黎曼猜想断言它们全在 Re(s) = 1/2 上。":"Zeta extends meromorphically to the whole complex plane with a simple pole at s = 1. The nontrivial zeros lie in 0 < Re(s) < 1. The hypothesis puts all of them on Re(s) = 1/2.",零点如何影响误差:"How zeros affect the error","在与质数计数相关的显式公式里，非平凡零点 ρ 会贡献含 x^ρ 的振荡项。它的增长量级取决于 Re(ρ)。因此把所有零点的实部控制在 1/2，可以约束质数分布偏离主项的程度。":"In explicit formulas related to prime counts, a nontrivial zero ρ contributes an oscillatory term involving x^ρ. Its growth depends on Re(ρ). Controlling all these real parts at 1/2 constrains how far the prime count can deviate from the main term.",检查一下理解:"Check your understanding",教材依据:"Source passages","{0} 页":"{0} pages",接近中线:"Near the middle","本次更偏向 {0}":"Leaning toward {0} this time","这组回答没有明显偏向，两种方式都可以是你的资源。":"These answers show no clear lean. Both approaches can be resources for you.",你可能带来的力量:"Strengths you might bring",怎样合作更舒服:"Working well together",给自己一个小练习:"A small experiment to try",留给自己的一问:"A question for yourself",如何理解这份探索:"How to read this exploration","这是一份原创的 32 题偏好探索问卷，借用 E–I、S–N、T–F、J–P 四组偏好帮助你反思，不是官方 MBTI® 测评，也未作为标准化心理量表进行验证。":"This original 32-item reflection exercise draws on the E–I, S–N, T–F and J–P preference pairs. It is not the official MBTI® assessment and has not been validated as a standardized psychological scale.","百分比只是这次作答按公开规则换算的分布，不是能力、性格强度或准确概率。接近中线的维度显示 X，表示本次没有明显偏向。":"Percentages show how these answers distribute under the published scoring rules. They do not measure ability, personality strength or the probability of being correct. A dimension near the middle shows X, meaning no clear lean this time.","偏好会受情境和经历影响。所有倾向都各有价值，这份结果不用于招聘筛选、能力判断或心理诊断。你比四个字母更丰富。":"Context and experience can affect preferences. Every tendency has value. These results are not for hiring, judging ability or psychological diagnosis. You are more than four letters.","逐题答案和本机记录不写入共享页面。只有你确认分享时，摘要才会出现在页面分享中。":"Individual answers and device history are not written to the shared page. Only a summary you confirm sharing appears in Page shares.",阅读偏好框架的介绍:"Read about the preference framework","这份摘要已在页面分享中，可先移除旧分享再重新分享。":"This summary is already shared. Remove the earlier share before sharing it again.","分享未确认，请先检查页面分享列表。":"Sharing was not confirmed. Check Page shares first.",结果摘要已分享到本页面:"Result summary shared with this page","已移除页面分享，不影响本机记录":"Page share removed; your device record is unchanged","还有未回答的问题，请回看。":"Some questions are unanswered. Please review them.",已删除本机记录:"Device record deleted",人格探索:"Personality explorer",应用导航:"App navigation",探索:"Explore","16 种偏好":"16 preferences",我的记录:"My journal",关于这份探索:"About this exploration","给自己，一段好奇的时间":"A little time to be curious about you","认识自己，":"Get to know yourself,","从一点好奇开始。":"one curious step at a time.","你怎样充电、理解世界、做出选择？从日常的小片刻出发，发现属于你的偏好地图。":"How do you recharge, take in the world and make choices? Start with everyday moments and discover your own map of preferences.",继续我的探索:"Continue exploring",开始探索:"Begin exploring","已回答 {0} / {1}":"Answered {0} / {1}","32 个小问题 · 约 5 分钟":"32 small questions · About 5 minutes","不必登录额外账号 · 答案仅留在本机":"No extra account · Answers stay on this device",从生活出发:"Start with everyday life","没有标准答案，选更像日常的自己。":"There is no right answer. Choose what feels like your everyday self.",看见四组偏好:"See four preference pairs","不只四个字母，也看看你怎样在两端之间选择。":"Go beyond four letters and notice how you choose between both ends.",把理解带回生活:"Bring insight into daily life","读一读、写一笔，让新的认识变成温柔的小行动。":"Read, reflect and turn a little understanding into a gentle action.","原创自我探索问卷，非官方 MBTI® 测评；分布不是能力或准确概率。":"An original reflection exercise, not the official MBTI® assessment. Distributions do not measure ability or accuracy.",了解更多:"Learn more","每一种，都有自己的光":"Every way has its own light","16 种偏好，无限种你":"16 preferences. Countless ways to be you.","把描述当作观察的起点。没有更好的类型，也没有一种描述能概括整个人。":"Use each description as a starting point for observation. No type is better, and no description captures a whole person.","一点一点，认识自己":"Get to know yourself, little by little","本机保留最近 12 次完成记录。清除浏览器数据会移除本机答案和小记。":"Your latest 12 completed sessions stay on this device. Clearing browser data removes your local answers and notes.",本机记录:"On this device",页面分享:"Page shares","你的小地图，从这里开始":"Your little map starts here","完成一次探索后，可以回来看看自己留下的足迹。":"After an exploration, come back to the thoughts you left along the way.",还没有留下小记:"No note yet",删除本机记录:"Delete device record","这里只展示主动分享的摘要，页面成员和有权限的 agent 可见。":"Only deliberately shared summaries appear here, visible to page members and authorized agents.","页面还没有分享的摘要。":"No summaries have been shared with this page yet.","这份摘要的版本或数据不受支持。":"This summary has an unsupported version or invalid data.",示例结果:"Example result",未署名:"Unsigned",移除页面分享:"Remove page share","此设备暂时无法保存；关闭页面后，本次答案可能无法保留。":"This device cannot save right now. Your answers may be lost when you close the page.",分享的探索摘要:"Shared exploration summary",远景规划者:"Long-view planner","把远处的可能性，慢慢整理成清晰的路径。":"Turn distant possibilities into a clearer path, one step at a time.","你可能喜欢独立思考，寻找信息之间的结构，并把想法放到较长的时间里检验。":"You may enjoy thinking independently, finding patterns in information and testing ideas over a longer horizon.","有明确的目标、充分的准备时间和讨论依据时，你可能更容易投入。也可以主动说出你尚未讲明的推理过程。":"Clear aims, preparation time and evidence to discuss may help you engage. Try sharing the reasoning you have not yet said aloud.","在追求完整方案时，试着给现实反馈留一个更早的入口。一个小实验，有时比更长的推演更有帮助。":"While seeking a complete plan, invite real-world feedback sooner. A small experiment can sometimes teach more than further analysis.","最近哪一个计划，值得先做一个不完美的小版本？":"Which recent plan deserves a small, imperfect first version?",思辨探索者:"Thoughtful investigator","沿着一个好问题，发现新的理解方式。":"Follow a good question toward a new way of understanding.","你可能享受追问、拆解概念和比较不同解释，对暂时没有定论的问题也能保持好奇。":"You may enjoy questioning, taking concepts apart and comparing explanations, staying curious when an answer is still uncertain.","允许提问、探索假设，并把讨论与最终决定分开的环境，可能让你更自在。":"Space to ask questions, explore assumptions and separate discussion from a final decision may help you feel at ease.","把好奇心与一个小小的完成节点连接起来，让别人也能看见你的思考成果。":"Connect your curiosity to a small finishing point so others can see the results of your thinking.","哪一个有趣的问题，可以在今天变成一段清楚的说明？":"Which interesting question could become a clear explanation today?",目标推动者:"Purposeful mover","让想法有方向，也让行动有着落。":"Give ideas direction and action somewhere to land.","你可能容易看见目标、资源与行动之间的关系，愿意推动决定，并组织事情向前发展。":"You may readily connect goals, resources and action, with a willingness to make decisions and organize progress.","你可能偏爱明确的责任与直接的沟通。给别人留出表达顾虑的时间，也能让方向更稳。":"Clear responsibilities and direct communication may suit you. Giving others time to voice concerns can make the direction steadier.","在提高效率之前，先确认大家对问题的理解是否一致。慢一点听，有时能省下后面的返工。":"Before improving efficiency, check that everyone understands the problem in the same way. Listening a little longer can prevent rework.","最近一次推进决定时，还有谁的顾虑值得听一听？":"Whose concern deserves a hearing before the next decision moves ahead?",灵感试验者:"Idea experimenter","换一个角度，事情也许还有别的可能。":"Try another angle. There may be another possibility.","你可能喜欢快速联想、挑战假设，并在交流和尝试中发现新的路线。":"You may like making quick connections, challenging assumptions and finding new routes through conversation and experiments.","开放的讨论和允许试错的空间可能很重要。也可以先说明你是在探索观点，而不一定在反对某个人。":"Open discussion and room for mistakes may matter to you. Explain when you are exploring a viewpoint rather than opposing a person.","挑选一个值得留下的想法，为它安排下一步，而不是急着寻找更多新选项。":"Choose one idea worth keeping and give it a next step before seeking more options.","你愿意把哪一个新点子，连续推进一周？":"Which new idea would you like to work on for a whole week?",深思引路者:"Reflective guide","在细微的感受里，看见更长远的意义。":"Notice a longer meaning in the smaller things people feel.","你可能会把对人的观察与整体理解放在一起，认真思考一件事真正重要的部分。":"You may connect observations about people with a wider view, thinking carefully about what truly matters in a situation.","有准备的深度交流、彼此信任的关系和清楚的意义，可能让你更愿意表达。":"Prepared, thoughtful conversations, mutual trust and a clear sense of meaning may help you express yourself.","在照顾别人的期待之前，也为自己的界限留出位置。不必等到想法完全成熟才说出来。":"Alongside other people's expectations, make room for your own boundaries. Your ideas need not be fully formed before you share them.","现在有什么需要，是你可以更直接表达的？":"What need could you express more directly right now?",价值守望者:"Values keeper","让选择靠近自己真正珍视的事。":"Let your choices move closer to what you truly care about.","你可能很在意内在的一致感，愿意理解不同人的经历，并为有意义的可能性保留空间。":"You may care about inner consistency, be willing to understand different experiences and make room for meaningful possibilities.","被认真倾听、允许有自己的节奏，以及知道事情为何值得做，可能比外部催促更有帮助。":"Being heard, having your own pace and knowing why something matters may help more than outside pressure.","把珍视的价值落在一个具体的小行动上。行动不必完美，也可以是真诚的开始。":"Express a value you cherish through one small action. It can be an honest beginning without being perfect.","今天哪一件很小的事，能体现你珍视的价值？":"What small action today could express a value you cherish?",共识连结者:"Common-ground connector","让不同的人，在理解中找到共同方向。":"Help different people find a shared direction through understanding.","你可能容易留意群体中的感受与需要，愿意鼓励别人，并推动有共同意义的行动。":"You may notice feelings and needs within a group, encourage others and move toward action with shared meaning.","坦诚的反馈和能看见彼此贡献的环境，可能让你更有投入感。也可以邀请安静的人用自己的方式表达。":"Honest feedback and visible appreciation of contributions may help you engage. Invite quieter people to express themselves in their own way.","不必替所有人承担情绪。照顾关系的同时，也让分歧和个人边界有被看见的机会。":"You do not have to carry everyone's emotions. Make room for disagreement and personal boundaries while caring for relationships.","哪一项责任，可以交还给更适合承担它的人？":"Which responsibility could return to someone better placed to carry it?",可能性发现者:"Possibility finder","带着好奇，与人和世界交换新的可能。":"Meet people and the world with a curiosity for what could be.","你可能会被新的连接、故事和想法点亮，喜欢从不同经历中寻找意义与机会。":"New connections, stories and ideas may light you up as you look for meaning and opportunity across different experiences.","开放、真诚、有探索空间的交流，可能更能激发你。把你的期待说具体，也能帮助别人跟上。":"Open, sincere conversation with room to explore may inspire you. Making your expectations concrete can help others follow along.","为最在意的一两个方向留下稳定的行动节奏，让灵感有机会长成成果。":"Give one or two directions you care about a steady rhythm of action so inspiration has time to grow into something real.","最近哪一个让你心动的方向，值得先留出一小时？":"Which exciting direction deserves an hour of your time soon?",稳健执行者:"Steady builder","把重要的事情，一步一步认真做好。":"Give important things your care, one practical step at a time.","你可能重视事实、经验和承诺，善于留意现实条件，并通过可靠的步骤把事情推进。":"You may value facts, experience and commitments, noticing real-world conditions and moving ahead through dependable steps.","清楚的要求、可用的信息和稳定的约定，可能让你更容易发挥。遇到变化时，提前说明影响也很重要。":"Clear requirements, useful information and reliable agreements may help you contribute. Explain the impact when things change.","在熟悉的方法之外，给新做法一个小范围试验的机会。变化不一定意味着放弃可靠。":"Give a new approach a small trial alongside familiar methods. Change does not have to mean giving up reliability.","哪一个旧流程，可以安全地试一点新办法？":"Which familiar process could safely try a little something new?",细致关照者:"Attentive supporter","用看得见的小事，照顾重要的人与承诺。":"Care for people and commitments through things they can see.","你可能会记住具体的需要与细节，愿意提供实际支持，也很认真地对待彼此的约定。":"You may remember specific needs and details, offer practical support and take shared commitments seriously.","尊重付出、明确期待、允许慢慢建立信任的环境，可能让你更安心。":"Appreciation of effort, clear expectations and time to build trust may help you feel secure.","在答应之前先看看自己的余量。表达需要和说不，也是一种让关系长久的方式。":"Check your capacity before saying yes. Expressing needs and saying no can also help relationships last.","最近哪一次帮忙，可以先问问自己是否有余力？":"Before which next favor could you check how much energy you have?",秩序组织者:"Practical organizer","把分散的资源，组织成可以推进的行动。":"Bring scattered resources into action that can move ahead.","你可能喜欢把要求讲清楚，依据现实信息做决定，并通过明确的分工让事情落实。":"You may like clarifying requirements, making decisions from practical information and using clear responsibilities to follow through.","直接、可靠的沟通可能很合你的节奏。对不同做事方式多问一句理由，也能发现新的解法。":"Direct, reliable communication may suit your pace. Asking why someone works differently can reveal another solution.","除了是否按时完成，也留意大家如何理解这项工作。好的秩序可以容纳合理的例外。":"Alongside deadlines, notice how people understand the work. Useful structure can make room for reasonable exceptions.","哪一条安排，可以在听到反馈后调整得更合适？":"Which arrangement could become more useful after hearing feedback?",协作照料者:"Caring collaborator","让合作有温度，也让日常更有秩序。":"Bring warmth to working together and structure to everyday life.","你可能会主动维系联系，关注具体的需要，并通过可见的支持让群体更容易协作。":"You may maintain connections, notice specific needs and offer visible support that helps a group work together.","及时的回应、清楚的约定和彼此认可，可能让你更有安全感。":"Timely responses, clear agreements and mutual appreciation may help you feel more secure.","不必用每个人的满意来衡量自己的价值。分歧可以被讨论，边界也可以被尊重。":"Everyone's satisfaction need not measure your worth. Disagreements can be discussed and boundaries respected.","在哪件事上，你可以温和而清楚地表达不同意见？":"Where could you express a different view gently and clearly?",实践拆解者:"Hands-on investigator","先看它怎样运转，再动手找到办法。":"See how it works, then find a way through doing.","你可能喜欢直接观察、分析机制，并在具体尝试中调整做法，面对实际问题时比较灵活。":"You may enjoy direct observation, analyzing mechanisms and adapting through practical experiments, staying flexible with real problems.","清楚的问题、足够的自主空间和少一些不必要的程序，可能更合你的节奏。":"A clear problem, enough independence and fewer unnecessary procedures may suit your rhythm.","把你已经看见的判断依据说出来，让别人不必猜测。也为较长远的影响留一点思考时间。":"Share the evidence behind what you see so others need not guess. Leave a little time to consider longer-term effects too.","哪一个你觉得显而易见的判断，值得向伙伴解释一下？":"Which judgment that feels obvious to you would be useful to explain to a partner?",感受创作者:"Experiential creator","在具体的体验里，找到属于自己的表达。":"Find your own expression through things you experience.","你可能很容易留意眼前的感受与细节，重视真实的个人体验，并愿意用行动或作品表达价值。":"You may notice immediate feelings and details, value authentic experience and express what matters through action or creative work.","尊重个人空间、少一点强迫、能亲自体验的方式，可能让你更自在。":"Respect for personal space, less pressure and the chance to experience things firsthand may help you feel at ease.","除了当下的感受，也给未来的自己留一个小约定。明确表达偏好，能减少别人对你的猜测。":"Alongside present feelings, make one small commitment to your future self. Expressing preferences can spare others from guessing.","你希望别人了解的一个偏好，可以怎样具体说出来？":"How could you express one preference you would like others to understand?",即兴行动者:"In-the-moment mover","走近真实现场，在行动中找到下一步。":"Step into the real situation and find the next step in action.","你可能喜欢及时反馈，容易捕捉具体机会，并在变化中迅速尝试可行的办法。":"You may enjoy immediate feedback, spot practical opportunities and quickly test workable approaches as circumstances change.","直接、实际、有行动空间的合作，可能比冗长的推演更容易吸引你。":"Direct, practical collaboration with room to act may attract you more than a long theoretical discussion.","在开始之前，留一分钟看看风险和后续影响。灵活行动与必要准备可以一起存在。":"Before starting, take a minute to check risks and later effects. Flexible action and useful preparation can work together.","下一次马上行动前，你愿意先确认哪一个关键条件？":"What key condition would you check before acting straight away next time?",当下分享者:"Present-moment sharer","把真实的感受，变成可以一起经历的片刻。":"Turn real feelings into moments you can experience together.","你可能会留意现场的氛围和人的反应，喜欢通过实际参与与分享，让事情更有生命力。":"You may notice the atmosphere and people's responses, bringing things to life through practical participation and sharing.","有互动、能看见具体进展，并允许个人表达的环境，可能让你更投入。":"Interaction, visible progress and room for personal expression may help you engage.","给重要而不紧急的事留一个固定位置，让长远的愿望也能得到照顾。":"Give important but unhurried things a regular place so longer-term wishes receive care too.","哪一个长期愿望，可以在本周安排一次小小的行动？":"Which longer-term wish could get a small action on this week's calendar?",仍在探索中的你:"Still exploring","有些偏好接近中线，不必急着选定一个标签。":"Some preferences sit near the middle. There is no need to settle on a label.","这次作答在部分维度上没有明显偏向。你的选择也许会随着情境、最近的经历和理解题目的方式而变化。":"Some dimensions show no clear lean this time. Choices may shift with context, recent experiences and how you read the questions.","比起四个字母，具体说出你怎样更容易恢复精力、理解信息和参与合作，往往更有帮助。":"Describing how you recharge, understand information and work with others can help more than four letters.","留意生活中的真实例子。你可以过一段时间再来，也可以阅读相邻类型，看看哪些描述更贴近自己。":"Notice real examples in your life. Return after a while or read neighboring types and see which parts feel familiar.","在哪一种具体情境中，你的偏好最容易发生变化？":"In which particular situation do your preferences change most easily?","群体活动后，我通常还想继续和大家聊一会儿。":"After a group activity, I usually want to keep chatting with everyone a little longer.","进入一个陌生的场合，我会主动开启话题。":"In an unfamiliar setting, I tend to start a conversation.","遇到新想法，我更喜欢边说边理清思路。":"With a new idea, I prefer to think it through by talking.","休息时，我常通过与人互动恢复精力。":"During a break, interacting with people often restores my energy.","连续社交之后，我需要一段独处时间充电。":"After a lot of socializing, I need some time alone to recharge.","在讨论中，我习惯先想清楚，再开口。":"In a discussion, I tend to think things through before speaking.","空闲的一天，我更想留一些不被打扰的时间。":"On a free day, I prefer to leave some time without interruptions.","建立关系时，我更享受少数深入的交流。":"When building relationships, I enjoy a few deeper conversations most.","我理解新事物时，会先留意具体事实和实例。":"When learning about something new, I notice concrete facts and examples first.","解释一件事，我通常先讲清楚实际发生了什么。":"When explaining something, I usually begin with what actually happened.","开始一个任务时，我倾向参考已有的经验和做法。":"When starting a task, I tend to refer to existing experience and methods.","做计划时，我会特别核对现实条件与细节。":"When making a plan, I pay special attention to practical conditions and details.","面对信息时，我常先联想到它背后的模式和可能性。":"When taking in information, I often first connect it to patterns and possibilities.","我喜欢讨论一个想法未来可能带来的变化。":"I enjoy discussing the changes an idea might bring in the future.","了解一个主题时，我会先抓住整体图景。":"When exploring a topic, I like to grasp the overall picture first.","我容易被新颖的概念和未尝试过的方法吸引。":"New concepts and untried approaches easily catch my interest.","做决定时，我首先比较各方案的逻辑是否一致。":"When deciding, I first compare whether the options are logically consistent.","遇到分歧时，我倾向先找出可共同检验的依据。":"When views differ, I tend to look first for evidence we can test together.","评价一个方案时，我通常先分析它是否解决了问题。":"When evaluating an approach, I first analyze whether it solves the problem.","提供反馈时，我会先指出需要调整的关键问题。":"When giving feedback, I first point out the key issues to adjust.","做决定时，我会先考虑它对相关人的影响。":"When deciding, I first consider the impact on the people involved.","当规则与具体处境冲突时，我会认真衡量个人需求。":"When a rule conflicts with a particular situation, I carefully weigh individual needs.","评价一个方案时，我很重视它是否符合我珍视的价值。":"When evaluating an approach, I care a lot about whether it fits my values.","提供反馈时，我会先想怎样让对方更容易接受。":"When giving feedback, I first consider how to help the other person receive it.","约定了重要事项后，我会尽早把时间和步骤定下来。":"Once an important commitment is agreed, I like to settle the time and steps early.","有明确的安排，会让我更安心地推进事情。":"A clear arrangement helps me feel at ease as I move things forward.","我通常喜欢先完成必须做的事，再随兴活动。":"I usually prefer to finish necessary tasks before doing things spontaneously.","面对一个选择，我更喜欢及时形成决定并继续推进。":"When facing a choice, I prefer to decide promptly and move ahead.","我喜欢在计划中为临时出现的机会留出空间。":"I like to leave room in a plan for unexpected opportunities.","获得新信息时，我愿意重新调整原来的安排。":"When new information arrives, I am willing to adjust an earlier arrangement.","探索一个任务时，我喜欢先试几种路线，再确定做法。":"When exploring a task, I like to try several routes before settling on an approach.","如果条件允许，我愿意让一些选择保持开放。":"When circumstances allow, I am happy to keep some choices open.",精力的去向:"Where your energy goes",外向:"Extraversion",内向:"Introversion","更容易在互动与外部活动中整理思路、获得能量。":"Interaction and outside activities may help you organize thoughts and gain energy.","更容易在独处与内在思考中整理思路、恢复能量。":"Time alone and inner reflection may help you organize thoughts and restore energy.",留意的信息:"What you notice",实感:"Sensing",直觉:"Intuition","更容易先关注具体事实、经验和可观察的细节。":"Concrete facts, experience and observable details may draw your attention first.","更容易先关注整体模式、联想和未来的可能性。":"Overall patterns, connections and future possibilities may draw your attention first.",决定的依据:"How you decide",思考:"Thinking",情感:"Feeling","更容易先比较逻辑、原则和可检验的依据。":"You may first compare logic, principles and evidence that can be tested.","更容易先衡量价值、处境和决定对人的影响。":"You may first weigh values, circumstances and a decision's impact on people.",安排的方式:"How you organize",判断:"Judging",知觉:"Perceiving","更喜欢形成安排、明确节点，让事情逐步落定。":"You may prefer arrangements and clear milestones that gradually settle things.","更喜欢保留弹性、探索选项，随着信息调整安排。":"You may prefer flexibility, open options and adjusting as information arrives.",很不像我:"Very unlike me",不太像我:"Somewhat unlike me",不确定:"Unsure",比较像我:"Somewhat like me",很像我:"Very like me","留一点时间，听听自己":"Take a little time to listen to yourself","不用选最好，只选更像的":"Choose what fits, rather than what seems best",让答案靠近日常的你:"Let your answers reflect everyday you","慢慢来，快要完成了":"Take your time. You are nearly there.",暂存并返回:"Save and return",答题进度:"Question progress","没有对错，也不必符合别人的期待。":"There is no right or wrong, and no expectation to live up to.","想想最近的日常，哪一个选项更接近你？":"Think about recent everyday life. Which option feels closest?",这句话有多像你:"How much is this like you?",上一题:"Previous",看看我的结果:"See my results",下一题:"Next","答案仅保存在这台设备，可以稍后继续。":"Answers stay on this device. You can continue later.","这一次，你更像":"This time, you lean toward","一张理解自己的小地图，不是定义你的标签。":"A small map for understanding yourself, rather than a label that defines you.","分享失败，请重试":"Sharing failed. Please try again.",返回探索:"Back to explore",仅本机可见:"Only on this device",也可以读读相邻的可能:"Read about nearby possibilities too","探索 16 种偏好":"Explore 16 preferences",写给自己的小记:"A note to yourself","哪些贴近你？哪些不像？记录一个真实的例子。":"What feels familiar? What does not? Record one real example.","从一个最近的小片刻开始……":"Start with a small, recent moment…","仅保存在本机，不会自动更新已分享的摘要。":"Saved only on this device. Earlier shared summaries do not update automatically.",回看我的答案:"Review my answers",摘要已复制:"Summary copied","复制失败，请使用导出":"Copy failed. Try exporting instead.",复制摘要:"Copy summary",导出小记:"Export note",分享摘要到页面:"Share summary with page","署名（可留空）":"Signature (optional)",同时分享这篇小记:"Include this note too","你还没有写小记。":"You have not written a note yet.","分享后可以移除；已经被他人复制的内容无法收回。":"You can remove a share later. Copies already made by others cannot be recalled.","正在分享……":"Sharing…",确认分享:"Confirm share","没能保存到下载文件夹，可以改用浏览器下载。":"Could not save to Downloads. You can download it through the browser instead.","已保存：{0}":"Saved: {0}","页面成员和有权读取页面的 agent 可以看到你的四维分布、完成时间、分享账号和署名。逐题答案不会分享。":"Page members and agents with page access can see your four distributions, completion time, sharing account and signature. Individual answers will not be shared.","每组偏好有 8 题，两种方向各 4 题。选项从 −2 到 2 分，反向题倒置后求和。左端比例为 50 加总分除以 16 再乘 50；总分在 −2 到 2 之间时显示 X。":"Each pair has eight items, four facing each direction. Responses range from −2 to 2; reverse-facing items invert the sign before summing. The left percentage is 50 plus the total divided by 16 times 50. Totals between −2 and 2 show X.",这个播放页链接不完整或格式不正确:"This playback link is incomplete or invalid.",这个播放页链接不受支持:"This playback link is not supported.",指定集数的链接还需要包含播放线路:"A link to an episode must also include its playback line.","链接指定的播放线路已不存在，请重新选择线路":"The linked playback line is no longer available. Choose a line again.","链接指定的集数已不存在，请重新选择集数":"The linked episode is no longer available. Choose an episode again.",链接中的影片:"Linked title","更新 myco 后即可复制播放页链接":"Update myco to copy playback links.",播放页链接已复制:"Playback link copied","无法生成播放页链接，请更新 myco 后重试":"Could not create a playback link. Update myco and try again.",无法打开播放页链接:"Could not open playback link",返回影视:"Back to Cinema",复制播放页链接:"Copy playback link","自动复制未成功，可以长按或选中下面的链接复制。":"Automatic copying failed. Select or press and hold the link below to copy it.",播放页链接:"Playback link","复制当前作品、线路和集数的链接":"Copy a link to this title, playback line and episode","PDF ≤ 100 MB · TXT ≤ 5 MB":"PDF ≤ 100 MB · TXT ≤ 5 MB","PDF 保留原书排版。请用阅读工具栏调整缩放；字体与行距设置适用于 TXT。":"PDF keeps its original layout. Use the toolbar to zoom; font and spacing settings apply to TXT books.","PDF 密码":"PDF password","PDF 超过 10,000 页，请拆分后阅读。":"This PDF has over 10,000 pages. Split it into smaller files to read it.",专注阅读:"Focus mode",书名:"Title",书架:"Library",书架分类:"Library filters",书签:"Bookmarks",书籍:"Books",书页:"Leaf","书页之间，世界慢一点。":"A little less rush. A few more pages.",从书架移除:"Remove from library",从头开始:"Start from the beginning","从本机书架移除这本书和它的进度、书签？原始文件不会被删除。":"Remove this book, progress and bookmarks from this local library? Your original file stays untouched.",保存书名:"Save title",保存原文件:"Save original file",先读一篇示例:"Try a sample read",全屏阅读:"Read in fullscreen",全部书籍:"All books",关键词:"Keywords",再添一本:"One more book",列表视图:"List view",删除书签:"Delete bookmark",加粗文字:"Bold text","原书与进度仅保存在本机，不会上传或跨设备同步。请保留原文件。":"Books and progress stay on this device. They are not uploaded or synced. Keep your original files.",增大字号:"Increase font size",夜读:"Night","宋体 / 衬线":"Serif","密码不正确，请再试一次。":"Incorrect password. Please try again.","密码只用于本次打开，不会保存。":"The password is used for this session only and will not be saved.",导入书籍:"Import books",导入时间:"Date added",封面视图:"Cover view","左右滑动或使用方向键翻页。系统开启减少动态效果时，将自动关闭动画。":"Swipe left or right, or use the arrow keys. Page animations turn off when Reduce Motion is enabled.","已加入书架，共 {0} 本。重复文件会自动合并。":"Added {0} books to your library. Duplicate files are merged automatically.","已读 {0}%":"{0}% read",已读完:"Finished",平移:"Slide",恢复适合宽度:"Reset to fit width",恢复默认设置:"Restore defaults",我的书架:"My library",我的书签:"Your bookmarks",打开书籍:"Open book","找不到这本书的本机文件，请重新导入。":"The local file is missing. Please import this book again.","找到 {0} 处（最多显示 100 处）":"Found {0} matches (showing up to 100)","把喜欢的书，放在这里":"A home for your favorite books","拖入 PDF 或 TXT，开启一段安静的阅读。":"Drop a PDF or TXT file here. Make a little room for reading.",排列顺序:"Sort books",接着上次读:"PICK UP WHERE YOU LEFT OFF",搜索书中内容:"Search in book",搜索书名:"Find a book",搜索书架:"Search library","操作未能保存。请检查本机剩余空间，并确保允许保存网站数据。请勿清除书籍原文件。":"Could not save. Check available space and allow website storage. Keep your original book files.",放大页面:"Zoom in","文件为空或过大：PDF 最多 100 MB，TXT 最多 5 MB。":"The file is empty or too large. PDF limit: 100 MB; TXT limit: 5 MB.",文字宽度:"Text width",无动画:"None","无法打开这本书。PDF 可能已损坏，或文字编码不受支持。请尝试重新导入。":"Could not open this book. The PDF may be damaged or the text encoding unsupported. Try importing it again.","无法读取这个文本。支持 UTF-8、UTF-16 和 GB18030 编码。":"Could not read this text. Supported encodings: UTF-8, UTF-16 and GB18030.",显示工具栏:"Show toolbar",最近阅读:"Last read",未开始:"Unread","未找到内容。扫描版 PDF 需要先做文字识别。":"No matches. Scanned PDFs need OCR before their text can be searched.","松手，加入书架":"Drop to add to your library","正在导入 {0} / {1}：{2}":"Importing {0} / {1}: {2}","正在打开书架…":"Opening your library…","正在打开书籍…":"Opening book…","正在搜索…":"Searching…","正在搜索第 {0} 页…":"Searching page {0}…","正在绘制页面…":"Rendering page…",正在读:"Reading",没有完成:"Could not complete","没有找到书籍，试试其他关键词或分类。":"No books found. Try a different search or filter.",添加书签:"Add bookmark",白纸:"Paper",目录:"Contents",知道了:"Got it",确认移除:"Remove book",示例书架:"Sample books",移除本页书签:"Remove bookmark","管理《{0}》":"Manage “{0}”",管理书籍:"Manage book","给自己，一点阅读的时间":"MAKE A LITTLE TIME TO READ",继续阅读:"Continue reading",缩小字号:"Decrease font size",缩小页面:"Zoom out",羊皮纸:"Warm",翻页效果:"Page transition",行间距:"Line spacing","请在 myco App 中打开阅读应用。":"Open Reader inside the myco App.","请选择 PDF 或 TXT 文件。":"Choose a PDF or TXT file.","读到喜欢的地方，点一下顶部的书签。":"Found a passage to return to? Tap the bookmark at the top.",跟随应用:"App theme",输入关键词:"Enter keywords",返回书架:"Back to library","这一页未能显示，请重试或切换页面。":"This page could not be displayed. Try again or turn to another page.","这个文件不是有效的 PDF，请检查原文件。":"This is not a valid PDF. Check the original file.","这本 PDF 需要密码":"This PDF needs a password","这本书没有目录，可以用底部进度条跳转。":"This book has no table of contents. Use the progress bar below to move through it.",选择书籍:"Choose books",阅读:"Reader","阅读《{0}》":"Read “{0}”",阅读主题:"Reading theme",阅读设置:"Reading settings",阅读进度:"Reading progress",青瓷:"Sage","黑体 / 无衬线":"Sans serif","此文件超过应用的 20 MB 保存上限，请自行保留导入时的原文件。":"This file exceeds the App’s 20 MB download limit. Keep the original file you imported.",翻页方式:"Reading mode",左右逐页:"Horizontal pages",上下逐页:"Vertical pages",连续滚动:"Continuous scroll","像长文一样自然滚动，进度会自动记住。":"Scroll naturally through the book. Your place is saved automatically.","向上滑动读下一页，向下滑动回上一页。":"Swipe up for the next page, or down for the previous page.","向左滑动读下一页，向右滑动回上一页。":"Swipe left for the next page, or right for the previous page.","翻页方式支持手势、滚轮和方向键。系统开启减少动态效果时，将自动关闭动画。":"Navigate with gestures, the wheel or arrow keys. Reduce Motion disables page animations.",恢复适合页面:"Reset to fit page","3D 画面没能启动：{0}":"The 3D view could not start: {0}","W/S 油门 · A/D 转舵 · 拖动看四周 · M 地图":"W/S throttle · A/D steer · drag to look around · M map","{0} 公里":"{0} km","{0} 米":"{0} m","{0}风":"{0} wind",东:"E",东北:"NE",东南:"SE",北:"N",南:"S",西:"W",西北:"NW",西南:"SW",中浪:"Moderate",大浪:"Rough",平静:"Calm",倒车:"Astern",停车:"Stop",关掉声音:"Mute sound",打开声音:"Turn sound on",减油门:"Less throttle","减油门、倒车":"Less throttle, astern",加油门:"More throttle",出发点改到了这里:"Start moved here","到过 {0} / {1} 处":"Visited {0} of {1}",双击:"Double-click",取消航向:"Clear course",右舵:"Steer right",左舵:"Steer left",回到出发点:"Back to start","在 {0} 附近":"Near {0}",地图:"Map","地图（M）":"Map (M)","打开地图（M）":"Open map (M)",多云:"Cloudy",晴:"Clear",阴:"Overcast",大家看到的一样:"Everyone sees the same",天气:"Weather",就在旁边:"Right here",岛:"Island",沙洲:"Sandbar",海蚀柱:"Sea stack",礁石:"Rocks","开往 {0} · {1}":"To {0} · {1}",开往这里:"Set course",开阔海面:"Open sea","打不开 3D 海面":"Can't open the 3D sea",把这里设为出发点:"Start here",拉近拉远:"Zoom in and out",无名:"Unnamed","油门 {0}%":"Throttle {0}%","油门（W / S）":"Throttle (W / S)",海况:"Sea",海况和天气:"Sea and weather","海面：用 W A S D 或方向键开船":"The sea: steer the boat with W A S D or the arrow keys",滚轮:"Scroll",空挡:"Neutral","航向 {0}°":"Heading {0}°",航海:"Voyage",节:"kn",视角回到船后:"View back behind the boat","触底了，倒车或转个方向":"Aground: back off or turn away",转舵:"Steer",转视角:"Look around","这台电脑上画不了 3D（浏览器没有 WebGL）。":"This computer can't draw 3D (no WebGL in its browser).",这片海叫什么:"What is this sea called",风向:"Wind",操作方法:"Controls"};function i_(){var n;const t=new URLSearchParams(typeof location>"u"?"":location.search).get("myco_lang");if(t==="en"||t==="zh")return t;const e=globalThis.myco;return((n=e==null?void 0:e.preferences)==null?void 0:n.language)||(typeof document<"u"&&document.documentElement.lang.toLowerCase().startsWith("en")?"en":"zh")}function ee(t,...e){return(i_()==="en"?n_[t]??t:t).replace(/\{(\d+)\}/g,(i,r)=>Number(r)<e.length?String(e[Number(r)]??""):i)}/**
 * @license lucide-react v0.460.0 - ISC
 *
 * This source code is licensed under the ISC license.
 * See the LICENSE file in the root directory of this source tree.
 */const r_=t=>t.replace(/([a-z0-9])([A-Z])/g,"$1-$2").toLowerCase(),Pg=(...t)=>t.filter((e,n,i)=>!!e&&e.trim()!==""&&i.indexOf(e)===n).join(" ").trim();/**
 * @license lucide-react v0.460.0 - ISC
 *
 * This source code is licensed under the ISC license.
 * See the LICENSE file in the root directory of this source tree.
 */var o_={xmlns:"http://www.w3.org/2000/svg",width:24,height:24,viewBox:"0 0 24 24",fill:"none",stroke:"currentColor",strokeWidth:2,strokeLinecap:"round",strokeLinejoin:"round"};/**
 * @license lucide-react v0.460.0 - ISC
 *
 * This source code is licensed under the ISC license.
 * See the LICENSE file in the root directory of this source tree.
 */const s_=ze.forwardRef(({color:t="currentColor",size:e=24,strokeWidth:n=2,absoluteStrokeWidth:i,className:r="",children:o,iconNode:s,...a},l)=>ze.createElement("svg",{ref:l,...o_,width:e,height:e,stroke:t,strokeWidth:i?Number(n)*24/Number(e):n,className:Pg("lucide",r),...a},[...s.map(([c,d])=>ze.createElement(c,d)),...Array.isArray(o)?o:[o]]));/**
 * @license lucide-react v0.460.0 - ISC
 *
 * This source code is licensed under the ISC license.
 * See the LICENSE file in the root directory of this source tree.
 */const In=(t,e)=>{const n=ze.forwardRef(({className:i,...r},o)=>ze.createElement(s_,{ref:o,iconNode:e,className:Pg(`lucide-${r_(t)}`,i),...r}));return n.displayName=`${t}`,n};/**
 * @license lucide-react v0.460.0 - ISC
 *
 * This source code is licensed under the ISC license.
 * See the LICENSE file in the root directory of this source tree.
 */const a_=In("ChevronDown",[["path",{d:"m6 9 6 6 6-6",key:"qrunsl"}]]);/**
 * @license lucide-react v0.460.0 - ISC
 *
 * This source code is licensed under the ISC license.
 * See the LICENSE file in the root directory of this source tree.
 */const l_=In("ChevronLeft",[["path",{d:"m15 18-6-6 6-6",key:"1wnfg3"}]]);/**
 * @license lucide-react v0.460.0 - ISC
 *
 * This source code is licensed under the ISC license.
 * See the LICENSE file in the root directory of this source tree.
 */const c_=In("ChevronRight",[["path",{d:"m9 18 6-6-6-6",key:"mthhwq"}]]);/**
 * @license lucide-react v0.460.0 - ISC
 *
 * This source code is licensed under the ISC license.
 * See the LICENSE file in the root directory of this source tree.
 */const u_=In("ChevronUp",[["path",{d:"m18 15-6-6-6 6",key:"153udz"}]]);/**
 * @license lucide-react v0.460.0 - ISC
 *
 * This source code is licensed under the ISC license.
 * See the LICENSE file in the root directory of this source tree.
 */const d_=In("Keyboard",[["path",{d:"M10 8h.01",key:"1r9ogq"}],["path",{d:"M12 12h.01",key:"1mp3jc"}],["path",{d:"M14 8h.01",key:"1primd"}],["path",{d:"M16 12h.01",key:"1l6xoz"}],["path",{d:"M18 8h.01",key:"emo2bl"}],["path",{d:"M6 8h.01",key:"x9i8wu"}],["path",{d:"M7 16h10",key:"wp8him"}],["path",{d:"M8 12h.01",key:"czm47f"}],["rect",{width:"20",height:"16",x:"2",y:"4",rx:"2",key:"18n3k1"}]]);/**
 * @license lucide-react v0.460.0 - ISC
 *
 * This source code is licensed under the ISC license.
 * See the LICENSE file in the root directory of this source tree.
 */const h_=In("Map",[["path",{d:"M14.106 5.553a2 2 0 0 0 1.788 0l3.659-1.83A1 1 0 0 1 21 4.619v12.764a1 1 0 0 1-.553.894l-4.553 2.277a2 2 0 0 1-1.788 0l-4.212-2.106a2 2 0 0 0-1.788 0l-3.659 1.83A1 1 0 0 1 3 19.381V6.618a1 1 0 0 1 .553-.894l4.553-2.277a2 2 0 0 1 1.788 0z",key:"169xi5"}],["path",{d:"M15 5.764v15",key:"1pn4in"}],["path",{d:"M9 3.236v15",key:"1uimfh"}]]);/**
 * @license lucide-react v0.460.0 - ISC
 *
 * This source code is licensed under the ISC license.
 * See the LICENSE file in the root directory of this source tree.
 */const f_=In("Navigation",[["polygon",{points:"3 11 22 2 13 21 11 13 3 11",key:"1ltx0t"}]]);/**
 * @license lucide-react v0.460.0 - ISC
 *
 * This source code is licensed under the ISC license.
 * See the LICENSE file in the root directory of this source tree.
 */const p_=In("SlidersHorizontal",[["line",{x1:"21",x2:"14",y1:"4",y2:"4",key:"obuewd"}],["line",{x1:"10",x2:"3",y1:"4",y2:"4",key:"1q6298"}],["line",{x1:"21",x2:"12",y1:"12",y2:"12",key:"1iu8h1"}],["line",{x1:"8",x2:"3",y1:"12",y2:"12",key:"ntss68"}],["line",{x1:"21",x2:"16",y1:"20",y2:"20",key:"14d8ph"}],["line",{x1:"12",x2:"3",y1:"20",y2:"20",key:"m0wm8r"}],["line",{x1:"14",x2:"14",y1:"2",y2:"6",key:"14e1ph"}],["line",{x1:"8",x2:"8",y1:"10",y2:"14",key:"1i6ji0"}],["line",{x1:"16",x2:"16",y1:"18",y2:"22",key:"1lctlv"}]]);/**
 * @license lucide-react v0.460.0 - ISC
 *
 * This source code is licensed under the ISC license.
 * See the LICENSE file in the root directory of this source tree.
 */const m_=In("Square",[["rect",{width:"18",height:"18",x:"3",y:"3",rx:"2",key:"afitv7"}]]);/**
 * @license lucide-react v0.460.0 - ISC
 *
 * This source code is licensed under the ISC license.
 * See the LICENSE file in the root directory of this source tree.
 */const g_=In("Volume2",[["path",{d:"M11 4.702a.705.705 0 0 0-1.203-.498L6.413 7.587A1.4 1.4 0 0 1 5.416 8H3a1 1 0 0 0-1 1v6a1 1 0 0 0 1 1h2.416a1.4 1.4 0 0 1 .997.413l3.383 3.384A.705.705 0 0 0 11 19.298z",key:"uqj9uw"}],["path",{d:"M16 9a5 5 0 0 1 0 6",key:"1q6k2b"}],["path",{d:"M19.364 18.364a9 9 0 0 0 0-12.728",key:"ijwkga"}]]);/**
 * @license lucide-react v0.460.0 - ISC
 *
 * This source code is licensed under the ISC license.
 * See the LICENSE file in the root directory of this source tree.
 */const v_=In("VolumeX",[["path",{d:"M11 4.702a.705.705 0 0 0-1.203-.498L6.413 7.587A1.4 1.4 0 0 1 5.416 8H3a1 1 0 0 0-1 1v6a1 1 0 0 0 1 1h2.416a1.4 1.4 0 0 1 .997.413l3.383 3.384A.705.705 0 0 0 11 19.298z",key:"uqj9uw"}],["line",{x1:"22",x2:"16",y1:"9",y2:"15",key:"1ewh16"}],["line",{x1:"16",x2:"22",y1:"9",y2:"15",key:"5ykzw1"}]]);/**
 * @license lucide-react v0.460.0 - ISC
 *
 * This source code is licensed under the ISC license.
 * See the LICENSE file in the root directory of this source tree.
 */const oc=In("X",[["path",{d:"M18 6 6 18",key:"1bl5f8"}],["path",{d:"m6 6 12 12",key:"d8bk6v"}]]);var Dg={exports:{}},Sn={},Lg={exports:{}},Ng={};/**
 * @license React
 * scheduler.production.min.js
 *
 * Copyright (c) Facebook, Inc. and its affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */(function(t){function e(D,G){var Z=D.length;D.push(G);e:for(;0<Z;){var le=Z-1>>>1,Se=D[le];if(0<r(Se,G))D[le]=G,D[Z]=Se,Z=le;else break e}}function n(D){return D.length===0?null:D[0]}function i(D){if(D.length===0)return null;var G=D[0],Z=D.pop();if(Z!==G){D[0]=Z;e:for(var le=0,Se=D.length,Ne=Se>>>1;le<Ne;){var q=2*(le+1)-1,re=D[q],he=q+1,ae=D[he];if(0>r(re,Z))he<Se&&0>r(ae,re)?(D[le]=ae,D[he]=Z,le=he):(D[le]=re,D[q]=Z,le=q);else if(he<Se&&0>r(ae,Z))D[le]=ae,D[he]=Z,le=he;else break e}}return G}function r(D,G){var Z=D.sortIndex-G.sortIndex;return Z!==0?Z:D.id-G.id}if(typeof performance=="object"&&typeof performance.now=="function"){var o=performance;t.unstable_now=function(){return o.now()}}else{var s=Date,a=s.now();t.unstable_now=function(){return s.now()-a}}var l=[],c=[],d=1,f=null,h=3,p=!1,v=!1,_=!1,m=typeof setTimeout=="function"?setTimeout:null,u=typeof clearTimeout=="function"?clearTimeout:null,g=typeof setImmediate<"u"?setImmediate:null;typeof navigator<"u"&&navigator.scheduling!==void 0&&navigator.scheduling.isInputPending!==void 0&&navigator.scheduling.isInputPending.bind(navigator.scheduling);function y(D){for(var G=n(c);G!==null;){if(G.callback===null)i(c);else if(G.startTime<=D)i(c),G.sortIndex=G.expirationTime,e(l,G);else break;G=n(c)}}function x(D){if(_=!1,y(D),!v)if(n(l)!==null)v=!0,W(C);else{var G=n(c);G!==null&&$(x,G.startTime-D)}}function C(D,G){v=!1,_&&(_=!1,u(b),b=-1),p=!0;var Z=h;try{for(y(G),f=n(l);f!==null&&(!(f.expirationTime>G)||D&&!P());){var le=f.callback;if(typeof le=="function"){f.callback=null,h=f.priorityLevel;var Se=le(f.expirationTime<=G);G=t.unstable_now(),typeof Se=="function"?f.callback=Se:f===n(l)&&i(l),y(G)}else i(l);f=n(l)}if(f!==null)var Ne=!0;else{var q=n(c);q!==null&&$(x,q.startTime-G),Ne=!1}return Ne}finally{f=null,h=Z,p=!1}}var A=!1,T=null,b=-1,w=5,S=-1;function P(){return!(t.unstable_now()-S<w)}function z(){if(T!==null){var D=t.unstable_now();S=D;var G=!0;try{G=T(!0,D)}finally{G?F():(A=!1,T=null)}}else A=!1}var F;if(typeof g=="function")F=function(){g(z)};else if(typeof MessageChannel<"u"){var Y=new MessageChannel,j=Y.port2;Y.port1.onmessage=z,F=function(){j.postMessage(null)}}else F=function(){m(z,0)};function W(D){T=D,A||(A=!0,F())}function $(D,G){b=m(function(){D(t.unstable_now())},G)}t.unstable_IdlePriority=5,t.unstable_ImmediatePriority=1,t.unstable_LowPriority=4,t.unstable_NormalPriority=3,t.unstable_Profiling=null,t.unstable_UserBlockingPriority=2,t.unstable_cancelCallback=function(D){D.callback=null},t.unstable_continueExecution=function(){v||p||(v=!0,W(C))},t.unstable_forceFrameRate=function(D){0>D||125<D?console.error("forceFrameRate takes a positive int between 0 and 125, forcing frame rates higher than 125 fps is not supported"):w=0<D?Math.floor(1e3/D):5},t.unstable_getCurrentPriorityLevel=function(){return h},t.unstable_getFirstCallbackNode=function(){return n(l)},t.unstable_next=function(D){switch(h){case 1:case 2:case 3:var G=3;break;default:G=h}var Z=h;h=G;try{return D()}finally{h=Z}},t.unstable_pauseExecution=function(){},t.unstable_requestPaint=function(){},t.unstable_runWithPriority=function(D,G){switch(D){case 1:case 2:case 3:case 4:case 5:break;default:D=3}var Z=h;h=D;try{return G()}finally{h=Z}},t.unstable_scheduleCallback=function(D,G,Z){var le=t.unstable_now();switch(typeof Z=="object"&&Z!==null?(Z=Z.delay,Z=typeof Z=="number"&&0<Z?le+Z:le):Z=le,D){case 1:var Se=-1;break;case 2:Se=250;break;case 5:Se=1073741823;break;case 4:Se=1e4;break;default:Se=5e3}return Se=Z+Se,D={id:d++,callback:G,priorityLevel:D,startTime:Z,expirationTime:Se,sortIndex:-1},Z>le?(D.sortIndex=Z,e(c,D),n(l)===null&&D===n(c)&&(_?(u(b),b=-1):_=!0,$(x,Z-le))):(D.sortIndex=Se,e(l,D),v||p||(v=!0,W(C))),D},t.unstable_shouldYield=P,t.unstable_wrapCallback=function(D){var G=h;return function(){var Z=h;h=G;try{return D.apply(this,arguments)}finally{h=Z}}}})(Ng);Lg.exports=Ng;var y_=Lg.exports;/**
 * @license React
 * react-dom.production.min.js
 *
 * Copyright (c) Facebook, Inc. and its affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */var __=ze,_n=y_;function ie(t){for(var e="https://reactjs.org/docs/error-decoder.html?invariant="+t,n=1;n<arguments.length;n++)e+="&args[]="+encodeURIComponent(arguments[n]);return"Minified React error #"+t+"; visit "+e+" for the full message or use the non-minified dev environment for full errors and additional helpful warnings."}var Ig=new Set,Ws={};function Or(t,e){Io(t,e),Io(t+"Capture",e)}function Io(t,e){for(Ws[t]=e,t=0;t<e.length;t++)Ig.add(e[t])}var Mi=!(typeof window>"u"||typeof window.document>"u"||typeof window.document.createElement>"u"),Bu=Object.prototype.hasOwnProperty,x_=/^[:A-Z_a-z\u00C0-\u00D6\u00D8-\u00F6\u00F8-\u02FF\u0370-\u037D\u037F-\u1FFF\u200C-\u200D\u2070-\u218F\u2C00-\u2FEF\u3001-\uD7FF\uF900-\uFDCF\uFDF0-\uFFFD][:A-Z_a-z\u00C0-\u00D6\u00D8-\u00F6\u00F8-\u02FF\u0370-\u037D\u037F-\u1FFF\u200C-\u200D\u2070-\u218F\u2C00-\u2FEF\u3001-\uD7FF\uF900-\uFDCF\uFDF0-\uFFFD\-.0-9\u00B7\u0300-\u036F\u203F-\u2040]*$/,Yf={},qf={};function S_(t){return Bu.call(qf,t)?!0:Bu.call(Yf,t)?!1:x_.test(t)?qf[t]=!0:(Yf[t]=!0,!1)}function M_(t,e,n,i){if(n!==null&&n.type===0)return!1;switch(typeof e){case"function":case"symbol":return!0;case"boolean":return i?!1:n!==null?!n.acceptsBooleans:(t=t.toLowerCase().slice(0,5),t!=="data-"&&t!=="aria-");default:return!1}}function w_(t,e,n,i){if(e===null||typeof e>"u"||M_(t,e,n,i))return!0;if(i)return!1;if(n!==null)switch(n.type){case 3:return!e;case 4:return e===!1;case 5:return isNaN(e);case 6:return isNaN(e)||1>e}return!1}function nn(t,e,n,i,r,o,s){this.acceptsBooleans=e===2||e===3||e===4,this.attributeName=i,this.attributeNamespace=r,this.mustUseProperty=n,this.propertyName=t,this.type=e,this.sanitizeURL=o,this.removeEmptyString=s}var Ut={};"children dangerouslySetInnerHTML defaultValue defaultChecked innerHTML suppressContentEditableWarning suppressHydrationWarning style".split(" ").forEach(function(t){Ut[t]=new nn(t,0,!1,t,null,!1,!1)});[["acceptCharset","accept-charset"],["className","class"],["htmlFor","for"],["httpEquiv","http-equiv"]].forEach(function(t){var e=t[0];Ut[e]=new nn(e,1,!1,t[1],null,!1,!1)});["contentEditable","draggable","spellCheck","value"].forEach(function(t){Ut[t]=new nn(t,2,!1,t.toLowerCase(),null,!1,!1)});["autoReverse","externalResourcesRequired","focusable","preserveAlpha"].forEach(function(t){Ut[t]=new nn(t,2,!1,t,null,!1,!1)});"allowFullScreen async autoFocus autoPlay controls default defer disabled disablePictureInPicture disableRemotePlayback formNoValidate hidden loop noModule noValidate open playsInline readOnly required reversed scoped seamless itemScope".split(" ").forEach(function(t){Ut[t]=new nn(t,3,!1,t.toLowerCase(),null,!1,!1)});["checked","multiple","muted","selected"].forEach(function(t){Ut[t]=new nn(t,3,!0,t,null,!1,!1)});["capture","download"].forEach(function(t){Ut[t]=new nn(t,4,!1,t,null,!1,!1)});["cols","rows","size","span"].forEach(function(t){Ut[t]=new nn(t,6,!1,t,null,!1,!1)});["rowSpan","start"].forEach(function(t){Ut[t]=new nn(t,5,!1,t.toLowerCase(),null,!1,!1)});var Ch=/[\-:]([a-z])/g;function bh(t){return t[1].toUpperCase()}"accent-height alignment-baseline arabic-form baseline-shift cap-height clip-path clip-rule color-interpolation color-interpolation-filters color-profile color-rendering dominant-baseline enable-background fill-opacity fill-rule flood-color flood-opacity font-family font-size font-size-adjust font-stretch font-style font-variant font-weight glyph-name glyph-orientation-horizontal glyph-orientation-vertical horiz-adv-x horiz-origin-x image-rendering letter-spacing lighting-color marker-end marker-mid marker-start overline-position overline-thickness paint-order panose-1 pointer-events rendering-intent shape-rendering stop-color stop-opacity strikethrough-position strikethrough-thickness stroke-dasharray stroke-dashoffset stroke-linecap stroke-linejoin stroke-miterlimit stroke-opacity stroke-width text-anchor text-decoration text-rendering underline-position underline-thickness unicode-bidi unicode-range units-per-em v-alphabetic v-hanging v-ideographic v-mathematical vector-effect vert-adv-y vert-origin-x vert-origin-y word-spacing writing-mode xmlns:xlink x-height".split(" ").forEach(function(t){var e=t.replace(Ch,bh);Ut[e]=new nn(e,1,!1,t,null,!1,!1)});"xlink:actuate xlink:arcrole xlink:role xlink:show xlink:title xlink:type".split(" ").forEach(function(t){var e=t.replace(Ch,bh);Ut[e]=new nn(e,1,!1,t,"http://www.w3.org/1999/xlink",!1,!1)});["xml:base","xml:lang","xml:space"].forEach(function(t){var e=t.replace(Ch,bh);Ut[e]=new nn(e,1,!1,t,"http://www.w3.org/XML/1998/namespace",!1,!1)});["tabIndex","crossOrigin"].forEach(function(t){Ut[t]=new nn(t,1,!1,t.toLowerCase(),null,!1,!1)});Ut.xlinkHref=new nn("xlinkHref",1,!1,"xlink:href","http://www.w3.org/1999/xlink",!0,!1);["src","href","action","formAction"].forEach(function(t){Ut[t]=new nn(t,1,!1,t.toLowerCase(),null,!0,!0)});function Rh(t,e,n,i){var r=Ut.hasOwnProperty(e)?Ut[e]:null;(r!==null?r.type!==0:i||!(2<e.length)||e[0]!=="o"&&e[0]!=="O"||e[1]!=="n"&&e[1]!=="N")&&(w_(e,n,r,i)&&(n=null),i||r===null?S_(e)&&(n===null?t.removeAttribute(e):t.setAttribute(e,""+n)):r.mustUseProperty?t[r.propertyName]=n===null?r.type===3?!1:"":n:(e=r.attributeName,i=r.attributeNamespace,n===null?t.removeAttribute(e):(r=r.type,n=r===3||r===4&&n===!0?"":""+n,i?t.setAttributeNS(i,e,n):t.setAttribute(e,n))))}var Ai=__.__SECRET_INTERNALS_DO_NOT_USE_OR_YOU_WILL_BE_FIRED,wa=Symbol.for("react.element"),lo=Symbol.for("react.portal"),co=Symbol.for("react.fragment"),Ph=Symbol.for("react.strict_mode"),Hu=Symbol.for("react.profiler"),kg=Symbol.for("react.provider"),Ug=Symbol.for("react.context"),Dh=Symbol.for("react.forward_ref"),Vu=Symbol.for("react.suspense"),Gu=Symbol.for("react.suspense_list"),Lh=Symbol.for("react.memo"),Fi=Symbol.for("react.lazy"),Fg=Symbol.for("react.offscreen"),$f=Symbol.iterator;function os(t){return t===null||typeof t!="object"?null:(t=$f&&t[$f]||t["@@iterator"],typeof t=="function"?t:null)}var yt=Object.assign,Pc;function Ts(t){if(Pc===void 0)try{throw Error()}catch(n){var e=n.stack.trim().match(/\n( *(at )?)/);Pc=e&&e[1]||""}return`
`+Pc+t}var Dc=!1;function Lc(t,e){if(!t||Dc)return"";Dc=!0;var n=Error.prepareStackTrace;Error.prepareStackTrace=void 0;try{if(e)if(e=function(){throw Error()},Object.defineProperty(e.prototype,"props",{set:function(){throw Error()}}),typeof Reflect=="object"&&Reflect.construct){try{Reflect.construct(e,[])}catch(c){var i=c}Reflect.construct(t,[],e)}else{try{e.call()}catch(c){i=c}t.call(e.prototype)}else{try{throw Error()}catch(c){i=c}t()}}catch(c){if(c&&i&&typeof c.stack=="string"){for(var r=c.stack.split(`
`),o=i.stack.split(`
`),s=r.length-1,a=o.length-1;1<=s&&0<=a&&r[s]!==o[a];)a--;for(;1<=s&&0<=a;s--,a--)if(r[s]!==o[a]){if(s!==1||a!==1)do if(s--,a--,0>a||r[s]!==o[a]){var l=`
`+r[s].replace(" at new "," at ");return t.displayName&&l.includes("<anonymous>")&&(l=l.replace("<anonymous>",t.displayName)),l}while(1<=s&&0<=a);break}}}finally{Dc=!1,Error.prepareStackTrace=n}return(t=t?t.displayName||t.name:"")?Ts(t):""}function E_(t){switch(t.tag){case 5:return Ts(t.type);case 16:return Ts("Lazy");case 13:return Ts("Suspense");case 19:return Ts("SuspenseList");case 0:case 2:case 15:return t=Lc(t.type,!1),t;case 11:return t=Lc(t.type.render,!1),t;case 1:return t=Lc(t.type,!0),t;default:return""}}function Wu(t){if(t==null)return null;if(typeof t=="function")return t.displayName||t.name||null;if(typeof t=="string")return t;switch(t){case co:return"Fragment";case lo:return"Portal";case Hu:return"Profiler";case Ph:return"StrictMode";case Vu:return"Suspense";case Gu:return"SuspenseList"}if(typeof t=="object")switch(t.$$typeof){case Ug:return(t.displayName||"Context")+".Consumer";case kg:return(t._context.displayName||"Context")+".Provider";case Dh:var e=t.render;return t=t.displayName,t||(t=e.displayName||e.name||"",t=t!==""?"ForwardRef("+t+")":"ForwardRef"),t;case Lh:return e=t.displayName||null,e!==null?e:Wu(t.type)||"Memo";case Fi:e=t._payload,t=t._init;try{return Wu(t(e))}catch{}}return null}function T_(t){var e=t.type;switch(t.tag){case 24:return"Cache";case 9:return(e.displayName||"Context")+".Consumer";case 10:return(e._context.displayName||"Context")+".Provider";case 18:return"DehydratedFragment";case 11:return t=e.render,t=t.displayName||t.name||"",e.displayName||(t!==""?"ForwardRef("+t+")":"ForwardRef");case 7:return"Fragment";case 5:return e;case 4:return"Portal";case 3:return"Root";case 6:return"Text";case 16:return Wu(e);case 8:return e===Ph?"StrictMode":"Mode";case 22:return"Offscreen";case 12:return"Profiler";case 21:return"Scope";case 13:return"Suspense";case 19:return"SuspenseList";case 25:return"TracingMarker";case 1:case 0:case 17:case 2:case 14:case 15:if(typeof e=="function")return e.displayName||e.name||null;if(typeof e=="string")return e}return null}function rr(t){switch(typeof t){case"boolean":case"number":case"string":case"undefined":return t;case"object":return t;default:return""}}function Og(t){var e=t.type;return(t=t.nodeName)&&t.toLowerCase()==="input"&&(e==="checkbox"||e==="radio")}function A_(t){var e=Og(t)?"checked":"value",n=Object.getOwnPropertyDescriptor(t.constructor.prototype,e),i=""+t[e];if(!t.hasOwnProperty(e)&&typeof n<"u"&&typeof n.get=="function"&&typeof n.set=="function"){var r=n.get,o=n.set;return Object.defineProperty(t,e,{configurable:!0,get:function(){return r.call(this)},set:function(s){i=""+s,o.call(this,s)}}),Object.defineProperty(t,e,{enumerable:n.enumerable}),{getValue:function(){return i},setValue:function(s){i=""+s},stopTracking:function(){t._valueTracker=null,delete t[e]}}}}function Ea(t){t._valueTracker||(t._valueTracker=A_(t))}function zg(t){if(!t)return!1;var e=t._valueTracker;if(!e)return!0;var n=e.getValue(),i="";return t&&(i=Og(t)?t.checked?"true":"false":t.value),t=i,t!==n?(e.setValue(t),!0):!1}function bl(t){if(t=t||(typeof document<"u"?document:void 0),typeof t>"u")return null;try{return t.activeElement||t.body}catch{return t.body}}function Xu(t,e){var n=e.checked;return yt({},e,{defaultChecked:void 0,defaultValue:void 0,value:void 0,checked:n??t._wrapperState.initialChecked})}function Kf(t,e){var n=e.defaultValue==null?"":e.defaultValue,i=e.checked!=null?e.checked:e.defaultChecked;n=rr(e.value!=null?e.value:n),t._wrapperState={initialChecked:i,initialValue:n,controlled:e.type==="checkbox"||e.type==="radio"?e.checked!=null:e.value!=null}}function Bg(t,e){e=e.checked,e!=null&&Rh(t,"checked",e,!1)}function ju(t,e){Bg(t,e);var n=rr(e.value),i=e.type;if(n!=null)i==="number"?(n===0&&t.value===""||t.value!=n)&&(t.value=""+n):t.value!==""+n&&(t.value=""+n);else if(i==="submit"||i==="reset"){t.removeAttribute("value");return}e.hasOwnProperty("value")?Yu(t,e.type,n):e.hasOwnProperty("defaultValue")&&Yu(t,e.type,rr(e.defaultValue)),e.checked==null&&e.defaultChecked!=null&&(t.defaultChecked=!!e.defaultChecked)}function Zf(t,e,n){if(e.hasOwnProperty("value")||e.hasOwnProperty("defaultValue")){var i=e.type;if(!(i!=="submit"&&i!=="reset"||e.value!==void 0&&e.value!==null))return;e=""+t._wrapperState.initialValue,n||e===t.value||(t.value=e),t.defaultValue=e}n=t.name,n!==""&&(t.name=""),t.defaultChecked=!!t._wrapperState.initialChecked,n!==""&&(t.name=n)}function Yu(t,e,n){(e!=="number"||bl(t.ownerDocument)!==t)&&(n==null?t.defaultValue=""+t._wrapperState.initialValue:t.defaultValue!==""+n&&(t.defaultValue=""+n))}var As=Array.isArray;function Eo(t,e,n,i){if(t=t.options,e){e={};for(var r=0;r<n.length;r++)e["$"+n[r]]=!0;for(n=0;n<t.length;n++)r=e.hasOwnProperty("$"+t[n].value),t[n].selected!==r&&(t[n].selected=r),r&&i&&(t[n].defaultSelected=!0)}else{for(n=""+rr(n),e=null,r=0;r<t.length;r++){if(t[r].value===n){t[r].selected=!0,i&&(t[r].defaultSelected=!0);return}e!==null||t[r].disabled||(e=t[r])}e!==null&&(e.selected=!0)}}function qu(t,e){if(e.dangerouslySetInnerHTML!=null)throw Error(ie(91));return yt({},e,{value:void 0,defaultValue:void 0,children:""+t._wrapperState.initialValue})}function Jf(t,e){var n=e.value;if(n==null){if(n=e.children,e=e.defaultValue,n!=null){if(e!=null)throw Error(ie(92));if(As(n)){if(1<n.length)throw Error(ie(93));n=n[0]}e=n}e==null&&(e=""),n=e}t._wrapperState={initialValue:rr(n)}}function Hg(t,e){var n=rr(e.value),i=rr(e.defaultValue);n!=null&&(n=""+n,n!==t.value&&(t.value=n),e.defaultValue==null&&t.defaultValue!==n&&(t.defaultValue=n)),i!=null&&(t.defaultValue=""+i)}function Qf(t){var e=t.textContent;e===t._wrapperState.initialValue&&e!==""&&e!==null&&(t.value=e)}function Vg(t){switch(t){case"svg":return"http://www.w3.org/2000/svg";case"math":return"http://www.w3.org/1998/Math/MathML";default:return"http://www.w3.org/1999/xhtml"}}function $u(t,e){return t==null||t==="http://www.w3.org/1999/xhtml"?Vg(e):t==="http://www.w3.org/2000/svg"&&e==="foreignObject"?"http://www.w3.org/1999/xhtml":t}var Ta,Gg=function(t){return typeof MSApp<"u"&&MSApp.execUnsafeLocalFunction?function(e,n,i,r){MSApp.execUnsafeLocalFunction(function(){return t(e,n,i,r)})}:t}(function(t,e){if(t.namespaceURI!=="http://www.w3.org/2000/svg"||"innerHTML"in t)t.innerHTML=e;else{for(Ta=Ta||document.createElement("div"),Ta.innerHTML="<svg>"+e.valueOf().toString()+"</svg>",e=Ta.firstChild;t.firstChild;)t.removeChild(t.firstChild);for(;e.firstChild;)t.appendChild(e.firstChild)}});function Xs(t,e){if(e){var n=t.firstChild;if(n&&n===t.lastChild&&n.nodeType===3){n.nodeValue=e;return}}t.textContent=e}var Ds={animationIterationCount:!0,aspectRatio:!0,borderImageOutset:!0,borderImageSlice:!0,borderImageWidth:!0,boxFlex:!0,boxFlexGroup:!0,boxOrdinalGroup:!0,columnCount:!0,columns:!0,flex:!0,flexGrow:!0,flexPositive:!0,flexShrink:!0,flexNegative:!0,flexOrder:!0,gridArea:!0,gridRow:!0,gridRowEnd:!0,gridRowSpan:!0,gridRowStart:!0,gridColumn:!0,gridColumnEnd:!0,gridColumnSpan:!0,gridColumnStart:!0,fontWeight:!0,lineClamp:!0,lineHeight:!0,opacity:!0,order:!0,orphans:!0,tabSize:!0,widows:!0,zIndex:!0,zoom:!0,fillOpacity:!0,floodOpacity:!0,stopOpacity:!0,strokeDasharray:!0,strokeDashoffset:!0,strokeMiterlimit:!0,strokeOpacity:!0,strokeWidth:!0},C_=["Webkit","ms","Moz","O"];Object.keys(Ds).forEach(function(t){C_.forEach(function(e){e=e+t.charAt(0).toUpperCase()+t.substring(1),Ds[e]=Ds[t]})});function Wg(t,e,n){return e==null||typeof e=="boolean"||e===""?"":n||typeof e!="number"||e===0||Ds.hasOwnProperty(t)&&Ds[t]?(""+e).trim():e+"px"}function Xg(t,e){t=t.style;for(var n in e)if(e.hasOwnProperty(n)){var i=n.indexOf("--")===0,r=Wg(n,e[n],i);n==="float"&&(n="cssFloat"),i?t.setProperty(n,r):t[n]=r}}var b_=yt({menuitem:!0},{area:!0,base:!0,br:!0,col:!0,embed:!0,hr:!0,img:!0,input:!0,keygen:!0,link:!0,meta:!0,param:!0,source:!0,track:!0,wbr:!0});function Ku(t,e){if(e){if(b_[t]&&(e.children!=null||e.dangerouslySetInnerHTML!=null))throw Error(ie(137,t));if(e.dangerouslySetInnerHTML!=null){if(e.children!=null)throw Error(ie(60));if(typeof e.dangerouslySetInnerHTML!="object"||!("__html"in e.dangerouslySetInnerHTML))throw Error(ie(61))}if(e.style!=null&&typeof e.style!="object")throw Error(ie(62))}}function Zu(t,e){if(t.indexOf("-")===-1)return typeof e.is=="string";switch(t){case"annotation-xml":case"color-profile":case"font-face":case"font-face-src":case"font-face-uri":case"font-face-format":case"font-face-name":case"missing-glyph":return!1;default:return!0}}var Ju=null;function Nh(t){return t=t.target||t.srcElement||window,t.correspondingUseElement&&(t=t.correspondingUseElement),t.nodeType===3?t.parentNode:t}var Qu=null,To=null,Ao=null;function ep(t){if(t=pa(t)){if(typeof Qu!="function")throw Error(ie(280));var e=t.stateNode;e&&(e=uc(e),Qu(t.stateNode,t.type,e))}}function jg(t){To?Ao?Ao.push(t):Ao=[t]:To=t}function Yg(){if(To){var t=To,e=Ao;if(Ao=To=null,ep(t),e)for(t=0;t<e.length;t++)ep(e[t])}}function qg(t,e){return t(e)}function $g(){}var Nc=!1;function Kg(t,e,n){if(Nc)return t(e,n);Nc=!0;try{return qg(t,e,n)}finally{Nc=!1,(To!==null||Ao!==null)&&($g(),Yg())}}function js(t,e){var n=t.stateNode;if(n===null)return null;var i=uc(n);if(i===null)return null;n=i[e];e:switch(e){case"onClick":case"onClickCapture":case"onDoubleClick":case"onDoubleClickCapture":case"onMouseDown":case"onMouseDownCapture":case"onMouseMove":case"onMouseMoveCapture":case"onMouseUp":case"onMouseUpCapture":case"onMouseEnter":(i=!i.disabled)||(t=t.type,i=!(t==="button"||t==="input"||t==="select"||t==="textarea")),t=!i;break e;default:t=!1}if(t)return null;if(n&&typeof n!="function")throw Error(ie(231,e,typeof n));return n}var ed=!1;if(Mi)try{var ss={};Object.defineProperty(ss,"passive",{get:function(){ed=!0}}),window.addEventListener("test",ss,ss),window.removeEventListener("test",ss,ss)}catch{ed=!1}function R_(t,e,n,i,r,o,s,a,l){var c=Array.prototype.slice.call(arguments,3);try{e.apply(n,c)}catch(d){this.onError(d)}}var Ls=!1,Rl=null,Pl=!1,td=null,P_={onError:function(t){Ls=!0,Rl=t}};function D_(t,e,n,i,r,o,s,a,l){Ls=!1,Rl=null,R_.apply(P_,arguments)}function L_(t,e,n,i,r,o,s,a,l){if(D_.apply(this,arguments),Ls){if(Ls){var c=Rl;Ls=!1,Rl=null}else throw Error(ie(198));Pl||(Pl=!0,td=c)}}function zr(t){var e=t,n=t;if(t.alternate)for(;e.return;)e=e.return;else{t=e;do e=t,e.flags&4098&&(n=e.return),t=e.return;while(t)}return e.tag===3?n:null}function Zg(t){if(t.tag===13){var e=t.memoizedState;if(e===null&&(t=t.alternate,t!==null&&(e=t.memoizedState)),e!==null)return e.dehydrated}return null}function tp(t){if(zr(t)!==t)throw Error(ie(188))}function N_(t){var e=t.alternate;if(!e){if(e=zr(t),e===null)throw Error(ie(188));return e!==t?null:t}for(var n=t,i=e;;){var r=n.return;if(r===null)break;var o=r.alternate;if(o===null){if(i=r.return,i!==null){n=i;continue}break}if(r.child===o.child){for(o=r.child;o;){if(o===n)return tp(r),t;if(o===i)return tp(r),e;o=o.sibling}throw Error(ie(188))}if(n.return!==i.return)n=r,i=o;else{for(var s=!1,a=r.child;a;){if(a===n){s=!0,n=r,i=o;break}if(a===i){s=!0,i=r,n=o;break}a=a.sibling}if(!s){for(a=o.child;a;){if(a===n){s=!0,n=o,i=r;break}if(a===i){s=!0,i=o,n=r;break}a=a.sibling}if(!s)throw Error(ie(189))}}if(n.alternate!==i)throw Error(ie(190))}if(n.tag!==3)throw Error(ie(188));return n.stateNode.current===n?t:e}function Jg(t){return t=N_(t),t!==null?Qg(t):null}function Qg(t){if(t.tag===5||t.tag===6)return t;for(t=t.child;t!==null;){var e=Qg(t);if(e!==null)return e;t=t.sibling}return null}var e0=_n.unstable_scheduleCallback,np=_n.unstable_cancelCallback,I_=_n.unstable_shouldYield,k_=_n.unstable_requestPaint,Mt=_n.unstable_now,U_=_n.unstable_getCurrentPriorityLevel,Ih=_n.unstable_ImmediatePriority,t0=_n.unstable_UserBlockingPriority,Dl=_n.unstable_NormalPriority,F_=_n.unstable_LowPriority,n0=_n.unstable_IdlePriority,sc=null,ii=null;function O_(t){if(ii&&typeof ii.onCommitFiberRoot=="function")try{ii.onCommitFiberRoot(sc,t,void 0,(t.current.flags&128)===128)}catch{}}var Xn=Math.clz32?Math.clz32:H_,z_=Math.log,B_=Math.LN2;function H_(t){return t>>>=0,t===0?32:31-(z_(t)/B_|0)|0}var Aa=64,Ca=4194304;function Cs(t){switch(t&-t){case 1:return 1;case 2:return 2;case 4:return 4;case 8:return 8;case 16:return 16;case 32:return 32;case 64:case 128:case 256:case 512:case 1024:case 2048:case 4096:case 8192:case 16384:case 32768:case 65536:case 131072:case 262144:case 524288:case 1048576:case 2097152:return t&4194240;case 4194304:case 8388608:case 16777216:case 33554432:case 67108864:return t&130023424;case 134217728:return 134217728;case 268435456:return 268435456;case 536870912:return 536870912;case 1073741824:return 1073741824;default:return t}}function Ll(t,e){var n=t.pendingLanes;if(n===0)return 0;var i=0,r=t.suspendedLanes,o=t.pingedLanes,s=n&268435455;if(s!==0){var a=s&~r;a!==0?i=Cs(a):(o&=s,o!==0&&(i=Cs(o)))}else s=n&~r,s!==0?i=Cs(s):o!==0&&(i=Cs(o));if(i===0)return 0;if(e!==0&&e!==i&&!(e&r)&&(r=i&-i,o=e&-e,r>=o||r===16&&(o&4194240)!==0))return e;if(i&4&&(i|=n&16),e=t.entangledLanes,e!==0)for(t=t.entanglements,e&=i;0<e;)n=31-Xn(e),r=1<<n,i|=t[n],e&=~r;return i}function V_(t,e){switch(t){case 1:case 2:case 4:return e+250;case 8:case 16:case 32:case 64:case 128:case 256:case 512:case 1024:case 2048:case 4096:case 8192:case 16384:case 32768:case 65536:case 131072:case 262144:case 524288:case 1048576:case 2097152:return e+5e3;case 4194304:case 8388608:case 16777216:case 33554432:case 67108864:return-1;case 134217728:case 268435456:case 536870912:case 1073741824:return-1;default:return-1}}function G_(t,e){for(var n=t.suspendedLanes,i=t.pingedLanes,r=t.expirationTimes,o=t.pendingLanes;0<o;){var s=31-Xn(o),a=1<<s,l=r[s];l===-1?(!(a&n)||a&i)&&(r[s]=V_(a,e)):l<=e&&(t.expiredLanes|=a),o&=~a}}function nd(t){return t=t.pendingLanes&-1073741825,t!==0?t:t&1073741824?1073741824:0}function i0(){var t=Aa;return Aa<<=1,!(Aa&4194240)&&(Aa=64),t}function Ic(t){for(var e=[],n=0;31>n;n++)e.push(t);return e}function ha(t,e,n){t.pendingLanes|=e,e!==536870912&&(t.suspendedLanes=0,t.pingedLanes=0),t=t.eventTimes,e=31-Xn(e),t[e]=n}function W_(t,e){var n=t.pendingLanes&~e;t.pendingLanes=e,t.suspendedLanes=0,t.pingedLanes=0,t.expiredLanes&=e,t.mutableReadLanes&=e,t.entangledLanes&=e,e=t.entanglements;var i=t.eventTimes;for(t=t.expirationTimes;0<n;){var r=31-Xn(n),o=1<<r;e[r]=0,i[r]=-1,t[r]=-1,n&=~o}}function kh(t,e){var n=t.entangledLanes|=e;for(t=t.entanglements;n;){var i=31-Xn(n),r=1<<i;r&e|t[i]&e&&(t[i]|=e),n&=~r}}var tt=0;function r0(t){return t&=-t,1<t?4<t?t&268435455?16:536870912:4:1}var o0,Uh,s0,a0,l0,id=!1,ba=[],qi=null,$i=null,Ki=null,Ys=new Map,qs=new Map,zi=[],X_="mousedown mouseup touchcancel touchend touchstart auxclick dblclick pointercancel pointerdown pointerup dragend dragstart drop compositionend compositionstart keydown keypress keyup input textInput copy cut paste click change contextmenu reset submit".split(" ");function ip(t,e){switch(t){case"focusin":case"focusout":qi=null;break;case"dragenter":case"dragleave":$i=null;break;case"mouseover":case"mouseout":Ki=null;break;case"pointerover":case"pointerout":Ys.delete(e.pointerId);break;case"gotpointercapture":case"lostpointercapture":qs.delete(e.pointerId)}}function as(t,e,n,i,r,o){return t===null||t.nativeEvent!==o?(t={blockedOn:e,domEventName:n,eventSystemFlags:i,nativeEvent:o,targetContainers:[r]},e!==null&&(e=pa(e),e!==null&&Uh(e)),t):(t.eventSystemFlags|=i,e=t.targetContainers,r!==null&&e.indexOf(r)===-1&&e.push(r),t)}function j_(t,e,n,i,r){switch(e){case"focusin":return qi=as(qi,t,e,n,i,r),!0;case"dragenter":return $i=as($i,t,e,n,i,r),!0;case"mouseover":return Ki=as(Ki,t,e,n,i,r),!0;case"pointerover":var o=r.pointerId;return Ys.set(o,as(Ys.get(o)||null,t,e,n,i,r)),!0;case"gotpointercapture":return o=r.pointerId,qs.set(o,as(qs.get(o)||null,t,e,n,i,r)),!0}return!1}function c0(t){var e=Er(t.target);if(e!==null){var n=zr(e);if(n!==null){if(e=n.tag,e===13){if(e=Zg(n),e!==null){t.blockedOn=e,l0(t.priority,function(){s0(n)});return}}else if(e===3&&n.stateNode.current.memoizedState.isDehydrated){t.blockedOn=n.tag===3?n.stateNode.containerInfo:null;return}}}t.blockedOn=null}function dl(t){if(t.blockedOn!==null)return!1;for(var e=t.targetContainers;0<e.length;){var n=rd(t.domEventName,t.eventSystemFlags,e[0],t.nativeEvent);if(n===null){n=t.nativeEvent;var i=new n.constructor(n.type,n);Ju=i,n.target.dispatchEvent(i),Ju=null}else return e=pa(n),e!==null&&Uh(e),t.blockedOn=n,!1;e.shift()}return!0}function rp(t,e,n){dl(t)&&n.delete(e)}function Y_(){id=!1,qi!==null&&dl(qi)&&(qi=null),$i!==null&&dl($i)&&($i=null),Ki!==null&&dl(Ki)&&(Ki=null),Ys.forEach(rp),qs.forEach(rp)}function ls(t,e){t.blockedOn===e&&(t.blockedOn=null,id||(id=!0,_n.unstable_scheduleCallback(_n.unstable_NormalPriority,Y_)))}function $s(t){function e(r){return ls(r,t)}if(0<ba.length){ls(ba[0],t);for(var n=1;n<ba.length;n++){var i=ba[n];i.blockedOn===t&&(i.blockedOn=null)}}for(qi!==null&&ls(qi,t),$i!==null&&ls($i,t),Ki!==null&&ls(Ki,t),Ys.forEach(e),qs.forEach(e),n=0;n<zi.length;n++)i=zi[n],i.blockedOn===t&&(i.blockedOn=null);for(;0<zi.length&&(n=zi[0],n.blockedOn===null);)c0(n),n.blockedOn===null&&zi.shift()}var Co=Ai.ReactCurrentBatchConfig,Nl=!0;function q_(t,e,n,i){var r=tt,o=Co.transition;Co.transition=null;try{tt=1,Fh(t,e,n,i)}finally{tt=r,Co.transition=o}}function $_(t,e,n,i){var r=tt,o=Co.transition;Co.transition=null;try{tt=4,Fh(t,e,n,i)}finally{tt=r,Co.transition=o}}function Fh(t,e,n,i){if(Nl){var r=rd(t,e,n,i);if(r===null)Wc(t,e,i,Il,n),ip(t,i);else if(j_(r,t,e,n,i))i.stopPropagation();else if(ip(t,i),e&4&&-1<X_.indexOf(t)){for(;r!==null;){var o=pa(r);if(o!==null&&o0(o),o=rd(t,e,n,i),o===null&&Wc(t,e,i,Il,n),o===r)break;r=o}r!==null&&i.stopPropagation()}else Wc(t,e,i,null,n)}}var Il=null;function rd(t,e,n,i){if(Il=null,t=Nh(i),t=Er(t),t!==null)if(e=zr(t),e===null)t=null;else if(n=e.tag,n===13){if(t=Zg(e),t!==null)return t;t=null}else if(n===3){if(e.stateNode.current.memoizedState.isDehydrated)return e.tag===3?e.stateNode.containerInfo:null;t=null}else e!==t&&(t=null);return Il=t,null}function u0(t){switch(t){case"cancel":case"click":case"close":case"contextmenu":case"copy":case"cut":case"auxclick":case"dblclick":case"dragend":case"dragstart":case"drop":case"focusin":case"focusout":case"input":case"invalid":case"keydown":case"keypress":case"keyup":case"mousedown":case"mouseup":case"paste":case"pause":case"play":case"pointercancel":case"pointerdown":case"pointerup":case"ratechange":case"reset":case"resize":case"seeked":case"submit":case"touchcancel":case"touchend":case"touchstart":case"volumechange":case"change":case"selectionchange":case"textInput":case"compositionstart":case"compositionend":case"compositionupdate":case"beforeblur":case"afterblur":case"beforeinput":case"blur":case"fullscreenchange":case"focus":case"hashchange":case"popstate":case"select":case"selectstart":return 1;case"drag":case"dragenter":case"dragexit":case"dragleave":case"dragover":case"mousemove":case"mouseout":case"mouseover":case"pointermove":case"pointerout":case"pointerover":case"scroll":case"toggle":case"touchmove":case"wheel":case"mouseenter":case"mouseleave":case"pointerenter":case"pointerleave":return 4;case"message":switch(U_()){case Ih:return 1;case t0:return 4;case Dl:case F_:return 16;case n0:return 536870912;default:return 16}default:return 16}}var Wi=null,Oh=null,hl=null;function d0(){if(hl)return hl;var t,e=Oh,n=e.length,i,r="value"in Wi?Wi.value:Wi.textContent,o=r.length;for(t=0;t<n&&e[t]===r[t];t++);var s=n-t;for(i=1;i<=s&&e[n-i]===r[o-i];i++);return hl=r.slice(t,1<i?1-i:void 0)}function fl(t){var e=t.keyCode;return"charCode"in t?(t=t.charCode,t===0&&e===13&&(t=13)):t=e,t===10&&(t=13),32<=t||t===13?t:0}function Ra(){return!0}function op(){return!1}function Mn(t){function e(n,i,r,o,s){this._reactName=n,this._targetInst=r,this.type=i,this.nativeEvent=o,this.target=s,this.currentTarget=null;for(var a in t)t.hasOwnProperty(a)&&(n=t[a],this[a]=n?n(o):o[a]);return this.isDefaultPrevented=(o.defaultPrevented!=null?o.defaultPrevented:o.returnValue===!1)?Ra:op,this.isPropagationStopped=op,this}return yt(e.prototype,{preventDefault:function(){this.defaultPrevented=!0;var n=this.nativeEvent;n&&(n.preventDefault?n.preventDefault():typeof n.returnValue!="unknown"&&(n.returnValue=!1),this.isDefaultPrevented=Ra)},stopPropagation:function(){var n=this.nativeEvent;n&&(n.stopPropagation?n.stopPropagation():typeof n.cancelBubble!="unknown"&&(n.cancelBubble=!0),this.isPropagationStopped=Ra)},persist:function(){},isPersistent:Ra}),e}var $o={eventPhase:0,bubbles:0,cancelable:0,timeStamp:function(t){return t.timeStamp||Date.now()},defaultPrevented:0,isTrusted:0},zh=Mn($o),fa=yt({},$o,{view:0,detail:0}),K_=Mn(fa),kc,Uc,cs,ac=yt({},fa,{screenX:0,screenY:0,clientX:0,clientY:0,pageX:0,pageY:0,ctrlKey:0,shiftKey:0,altKey:0,metaKey:0,getModifierState:Bh,button:0,buttons:0,relatedTarget:function(t){return t.relatedTarget===void 0?t.fromElement===t.srcElement?t.toElement:t.fromElement:t.relatedTarget},movementX:function(t){return"movementX"in t?t.movementX:(t!==cs&&(cs&&t.type==="mousemove"?(kc=t.screenX-cs.screenX,Uc=t.screenY-cs.screenY):Uc=kc=0,cs=t),kc)},movementY:function(t){return"movementY"in t?t.movementY:Uc}}),sp=Mn(ac),Z_=yt({},ac,{dataTransfer:0}),J_=Mn(Z_),Q_=yt({},fa,{relatedTarget:0}),Fc=Mn(Q_),ex=yt({},$o,{animationName:0,elapsedTime:0,pseudoElement:0}),tx=Mn(ex),nx=yt({},$o,{clipboardData:function(t){return"clipboardData"in t?t.clipboardData:window.clipboardData}}),ix=Mn(nx),rx=yt({},$o,{data:0}),ap=Mn(rx),ox={Esc:"Escape",Spacebar:" ",Left:"ArrowLeft",Up:"ArrowUp",Right:"ArrowRight",Down:"ArrowDown",Del:"Delete",Win:"OS",Menu:"ContextMenu",Apps:"ContextMenu",Scroll:"ScrollLock",MozPrintableKey:"Unidentified"},sx={8:"Backspace",9:"Tab",12:"Clear",13:"Enter",16:"Shift",17:"Control",18:"Alt",19:"Pause",20:"CapsLock",27:"Escape",32:" ",33:"PageUp",34:"PageDown",35:"End",36:"Home",37:"ArrowLeft",38:"ArrowUp",39:"ArrowRight",40:"ArrowDown",45:"Insert",46:"Delete",112:"F1",113:"F2",114:"F3",115:"F4",116:"F5",117:"F6",118:"F7",119:"F8",120:"F9",121:"F10",122:"F11",123:"F12",144:"NumLock",145:"ScrollLock",224:"Meta"},ax={Alt:"altKey",Control:"ctrlKey",Meta:"metaKey",Shift:"shiftKey"};function lx(t){var e=this.nativeEvent;return e.getModifierState?e.getModifierState(t):(t=ax[t])?!!e[t]:!1}function Bh(){return lx}var cx=yt({},fa,{key:function(t){if(t.key){var e=ox[t.key]||t.key;if(e!=="Unidentified")return e}return t.type==="keypress"?(t=fl(t),t===13?"Enter":String.fromCharCode(t)):t.type==="keydown"||t.type==="keyup"?sx[t.keyCode]||"Unidentified":""},code:0,location:0,ctrlKey:0,shiftKey:0,altKey:0,metaKey:0,repeat:0,locale:0,getModifierState:Bh,charCode:function(t){return t.type==="keypress"?fl(t):0},keyCode:function(t){return t.type==="keydown"||t.type==="keyup"?t.keyCode:0},which:function(t){return t.type==="keypress"?fl(t):t.type==="keydown"||t.type==="keyup"?t.keyCode:0}}),ux=Mn(cx),dx=yt({},ac,{pointerId:0,width:0,height:0,pressure:0,tangentialPressure:0,tiltX:0,tiltY:0,twist:0,pointerType:0,isPrimary:0}),lp=Mn(dx),hx=yt({},fa,{touches:0,targetTouches:0,changedTouches:0,altKey:0,metaKey:0,ctrlKey:0,shiftKey:0,getModifierState:Bh}),fx=Mn(hx),px=yt({},$o,{propertyName:0,elapsedTime:0,pseudoElement:0}),mx=Mn(px),gx=yt({},ac,{deltaX:function(t){return"deltaX"in t?t.deltaX:"wheelDeltaX"in t?-t.wheelDeltaX:0},deltaY:function(t){return"deltaY"in t?t.deltaY:"wheelDeltaY"in t?-t.wheelDeltaY:"wheelDelta"in t?-t.wheelDelta:0},deltaZ:0,deltaMode:0}),vx=Mn(gx),yx=[9,13,27,32],Hh=Mi&&"CompositionEvent"in window,Ns=null;Mi&&"documentMode"in document&&(Ns=document.documentMode);var _x=Mi&&"TextEvent"in window&&!Ns,h0=Mi&&(!Hh||Ns&&8<Ns&&11>=Ns),cp=" ",up=!1;function f0(t,e){switch(t){case"keyup":return yx.indexOf(e.keyCode)!==-1;case"keydown":return e.keyCode!==229;case"keypress":case"mousedown":case"focusout":return!0;default:return!1}}function p0(t){return t=t.detail,typeof t=="object"&&"data"in t?t.data:null}var uo=!1;function xx(t,e){switch(t){case"compositionend":return p0(e);case"keypress":return e.which!==32?null:(up=!0,cp);case"textInput":return t=e.data,t===cp&&up?null:t;default:return null}}function Sx(t,e){if(uo)return t==="compositionend"||!Hh&&f0(t,e)?(t=d0(),hl=Oh=Wi=null,uo=!1,t):null;switch(t){case"paste":return null;case"keypress":if(!(e.ctrlKey||e.altKey||e.metaKey)||e.ctrlKey&&e.altKey){if(e.char&&1<e.char.length)return e.char;if(e.which)return String.fromCharCode(e.which)}return null;case"compositionend":return h0&&e.locale!=="ko"?null:e.data;default:return null}}var Mx={color:!0,date:!0,datetime:!0,"datetime-local":!0,email:!0,month:!0,number:!0,password:!0,range:!0,search:!0,tel:!0,text:!0,time:!0,url:!0,week:!0};function dp(t){var e=t&&t.nodeName&&t.nodeName.toLowerCase();return e==="input"?!!Mx[t.type]:e==="textarea"}function m0(t,e,n,i){jg(i),e=kl(e,"onChange"),0<e.length&&(n=new zh("onChange","change",null,n,i),t.push({event:n,listeners:e}))}var Is=null,Ks=null;function wx(t){A0(t,0)}function lc(t){var e=po(t);if(zg(e))return t}function Ex(t,e){if(t==="change")return e}var g0=!1;if(Mi){var Oc;if(Mi){var zc="oninput"in document;if(!zc){var hp=document.createElement("div");hp.setAttribute("oninput","return;"),zc=typeof hp.oninput=="function"}Oc=zc}else Oc=!1;g0=Oc&&(!document.documentMode||9<document.documentMode)}function fp(){Is&&(Is.detachEvent("onpropertychange",v0),Ks=Is=null)}function v0(t){if(t.propertyName==="value"&&lc(Ks)){var e=[];m0(e,Ks,t,Nh(t)),Kg(wx,e)}}function Tx(t,e,n){t==="focusin"?(fp(),Is=e,Ks=n,Is.attachEvent("onpropertychange",v0)):t==="focusout"&&fp()}function Ax(t){if(t==="selectionchange"||t==="keyup"||t==="keydown")return lc(Ks)}function Cx(t,e){if(t==="click")return lc(e)}function bx(t,e){if(t==="input"||t==="change")return lc(e)}function Rx(t,e){return t===e&&(t!==0||1/t===1/e)||t!==t&&e!==e}var Yn=typeof Object.is=="function"?Object.is:Rx;function Zs(t,e){if(Yn(t,e))return!0;if(typeof t!="object"||t===null||typeof e!="object"||e===null)return!1;var n=Object.keys(t),i=Object.keys(e);if(n.length!==i.length)return!1;for(i=0;i<n.length;i++){var r=n[i];if(!Bu.call(e,r)||!Yn(t[r],e[r]))return!1}return!0}function pp(t){for(;t&&t.firstChild;)t=t.firstChild;return t}function mp(t,e){var n=pp(t);t=0;for(var i;n;){if(n.nodeType===3){if(i=t+n.textContent.length,t<=e&&i>=e)return{node:n,offset:e-t};t=i}e:{for(;n;){if(n.nextSibling){n=n.nextSibling;break e}n=n.parentNode}n=void 0}n=pp(n)}}function y0(t,e){return t&&e?t===e?!0:t&&t.nodeType===3?!1:e&&e.nodeType===3?y0(t,e.parentNode):"contains"in t?t.contains(e):t.compareDocumentPosition?!!(t.compareDocumentPosition(e)&16):!1:!1}function _0(){for(var t=window,e=bl();e instanceof t.HTMLIFrameElement;){try{var n=typeof e.contentWindow.location.href=="string"}catch{n=!1}if(n)t=e.contentWindow;else break;e=bl(t.document)}return e}function Vh(t){var e=t&&t.nodeName&&t.nodeName.toLowerCase();return e&&(e==="input"&&(t.type==="text"||t.type==="search"||t.type==="tel"||t.type==="url"||t.type==="password")||e==="textarea"||t.contentEditable==="true")}function Px(t){var e=_0(),n=t.focusedElem,i=t.selectionRange;if(e!==n&&n&&n.ownerDocument&&y0(n.ownerDocument.documentElement,n)){if(i!==null&&Vh(n)){if(e=i.start,t=i.end,t===void 0&&(t=e),"selectionStart"in n)n.selectionStart=e,n.selectionEnd=Math.min(t,n.value.length);else if(t=(e=n.ownerDocument||document)&&e.defaultView||window,t.getSelection){t=t.getSelection();var r=n.textContent.length,o=Math.min(i.start,r);i=i.end===void 0?o:Math.min(i.end,r),!t.extend&&o>i&&(r=i,i=o,o=r),r=mp(n,o);var s=mp(n,i);r&&s&&(t.rangeCount!==1||t.anchorNode!==r.node||t.anchorOffset!==r.offset||t.focusNode!==s.node||t.focusOffset!==s.offset)&&(e=e.createRange(),e.setStart(r.node,r.offset),t.removeAllRanges(),o>i?(t.addRange(e),t.extend(s.node,s.offset)):(e.setEnd(s.node,s.offset),t.addRange(e)))}}for(e=[],t=n;t=t.parentNode;)t.nodeType===1&&e.push({element:t,left:t.scrollLeft,top:t.scrollTop});for(typeof n.focus=="function"&&n.focus(),n=0;n<e.length;n++)t=e[n],t.element.scrollLeft=t.left,t.element.scrollTop=t.top}}var Dx=Mi&&"documentMode"in document&&11>=document.documentMode,ho=null,od=null,ks=null,sd=!1;function gp(t,e,n){var i=n.window===n?n.document:n.nodeType===9?n:n.ownerDocument;sd||ho==null||ho!==bl(i)||(i=ho,"selectionStart"in i&&Vh(i)?i={start:i.selectionStart,end:i.selectionEnd}:(i=(i.ownerDocument&&i.ownerDocument.defaultView||window).getSelection(),i={anchorNode:i.anchorNode,anchorOffset:i.anchorOffset,focusNode:i.focusNode,focusOffset:i.focusOffset}),ks&&Zs(ks,i)||(ks=i,i=kl(od,"onSelect"),0<i.length&&(e=new zh("onSelect","select",null,e,n),t.push({event:e,listeners:i}),e.target=ho)))}function Pa(t,e){var n={};return n[t.toLowerCase()]=e.toLowerCase(),n["Webkit"+t]="webkit"+e,n["Moz"+t]="moz"+e,n}var fo={animationend:Pa("Animation","AnimationEnd"),animationiteration:Pa("Animation","AnimationIteration"),animationstart:Pa("Animation","AnimationStart"),transitionend:Pa("Transition","TransitionEnd")},Bc={},x0={};Mi&&(x0=document.createElement("div").style,"AnimationEvent"in window||(delete fo.animationend.animation,delete fo.animationiteration.animation,delete fo.animationstart.animation),"TransitionEvent"in window||delete fo.transitionend.transition);function cc(t){if(Bc[t])return Bc[t];if(!fo[t])return t;var e=fo[t],n;for(n in e)if(e.hasOwnProperty(n)&&n in x0)return Bc[t]=e[n];return t}var S0=cc("animationend"),M0=cc("animationiteration"),w0=cc("animationstart"),E0=cc("transitionend"),T0=new Map,vp="abort auxClick cancel canPlay canPlayThrough click close contextMenu copy cut drag dragEnd dragEnter dragExit dragLeave dragOver dragStart drop durationChange emptied encrypted ended error gotPointerCapture input invalid keyDown keyPress keyUp load loadedData loadedMetadata loadStart lostPointerCapture mouseDown mouseMove mouseOut mouseOver mouseUp paste pause play playing pointerCancel pointerDown pointerMove pointerOut pointerOver pointerUp progress rateChange reset resize seeked seeking stalled submit suspend timeUpdate touchCancel touchEnd touchStart volumeChange scroll toggle touchMove waiting wheel".split(" ");function ar(t,e){T0.set(t,e),Or(e,[t])}for(var Hc=0;Hc<vp.length;Hc++){var Vc=vp[Hc],Lx=Vc.toLowerCase(),Nx=Vc[0].toUpperCase()+Vc.slice(1);ar(Lx,"on"+Nx)}ar(S0,"onAnimationEnd");ar(M0,"onAnimationIteration");ar(w0,"onAnimationStart");ar("dblclick","onDoubleClick");ar("focusin","onFocus");ar("focusout","onBlur");ar(E0,"onTransitionEnd");Io("onMouseEnter",["mouseout","mouseover"]);Io("onMouseLeave",["mouseout","mouseover"]);Io("onPointerEnter",["pointerout","pointerover"]);Io("onPointerLeave",["pointerout","pointerover"]);Or("onChange","change click focusin focusout input keydown keyup selectionchange".split(" "));Or("onSelect","focusout contextmenu dragend focusin keydown keyup mousedown mouseup selectionchange".split(" "));Or("onBeforeInput",["compositionend","keypress","textInput","paste"]);Or("onCompositionEnd","compositionend focusout keydown keypress keyup mousedown".split(" "));Or("onCompositionStart","compositionstart focusout keydown keypress keyup mousedown".split(" "));Or("onCompositionUpdate","compositionupdate focusout keydown keypress keyup mousedown".split(" "));var bs="abort canplay canplaythrough durationchange emptied encrypted ended error loadeddata loadedmetadata loadstart pause play playing progress ratechange resize seeked seeking stalled suspend timeupdate volumechange waiting".split(" "),Ix=new Set("cancel close invalid load scroll toggle".split(" ").concat(bs));function yp(t,e,n){var i=t.type||"unknown-event";t.currentTarget=n,L_(i,e,void 0,t),t.currentTarget=null}function A0(t,e){e=(e&4)!==0;for(var n=0;n<t.length;n++){var i=t[n],r=i.event;i=i.listeners;e:{var o=void 0;if(e)for(var s=i.length-1;0<=s;s--){var a=i[s],l=a.instance,c=a.currentTarget;if(a=a.listener,l!==o&&r.isPropagationStopped())break e;yp(r,a,c),o=l}else for(s=0;s<i.length;s++){if(a=i[s],l=a.instance,c=a.currentTarget,a=a.listener,l!==o&&r.isPropagationStopped())break e;yp(r,a,c),o=l}}}if(Pl)throw t=td,Pl=!1,td=null,t}function ht(t,e){var n=e[dd];n===void 0&&(n=e[dd]=new Set);var i=t+"__bubble";n.has(i)||(C0(e,t,2,!1),n.add(i))}function Gc(t,e,n){var i=0;e&&(i|=4),C0(n,t,i,e)}var Da="_reactListening"+Math.random().toString(36).slice(2);function Js(t){if(!t[Da]){t[Da]=!0,Ig.forEach(function(n){n!=="selectionchange"&&(Ix.has(n)||Gc(n,!1,t),Gc(n,!0,t))});var e=t.nodeType===9?t:t.ownerDocument;e===null||e[Da]||(e[Da]=!0,Gc("selectionchange",!1,e))}}function C0(t,e,n,i){switch(u0(e)){case 1:var r=q_;break;case 4:r=$_;break;default:r=Fh}n=r.bind(null,e,n,t),r=void 0,!ed||e!=="touchstart"&&e!=="touchmove"&&e!=="wheel"||(r=!0),i?r!==void 0?t.addEventListener(e,n,{capture:!0,passive:r}):t.addEventListener(e,n,!0):r!==void 0?t.addEventListener(e,n,{passive:r}):t.addEventListener(e,n,!1)}function Wc(t,e,n,i,r){var o=i;if(!(e&1)&&!(e&2)&&i!==null)e:for(;;){if(i===null)return;var s=i.tag;if(s===3||s===4){var a=i.stateNode.containerInfo;if(a===r||a.nodeType===8&&a.parentNode===r)break;if(s===4)for(s=i.return;s!==null;){var l=s.tag;if((l===3||l===4)&&(l=s.stateNode.containerInfo,l===r||l.nodeType===8&&l.parentNode===r))return;s=s.return}for(;a!==null;){if(s=Er(a),s===null)return;if(l=s.tag,l===5||l===6){i=o=s;continue e}a=a.parentNode}}i=i.return}Kg(function(){var c=o,d=Nh(n),f=[];e:{var h=T0.get(t);if(h!==void 0){var p=zh,v=t;switch(t){case"keypress":if(fl(n)===0)break e;case"keydown":case"keyup":p=ux;break;case"focusin":v="focus",p=Fc;break;case"focusout":v="blur",p=Fc;break;case"beforeblur":case"afterblur":p=Fc;break;case"click":if(n.button===2)break e;case"auxclick":case"dblclick":case"mousedown":case"mousemove":case"mouseup":case"mouseout":case"mouseover":case"contextmenu":p=sp;break;case"drag":case"dragend":case"dragenter":case"dragexit":case"dragleave":case"dragover":case"dragstart":case"drop":p=J_;break;case"touchcancel":case"touchend":case"touchmove":case"touchstart":p=fx;break;case S0:case M0:case w0:p=tx;break;case E0:p=mx;break;case"scroll":p=K_;break;case"wheel":p=vx;break;case"copy":case"cut":case"paste":p=ix;break;case"gotpointercapture":case"lostpointercapture":case"pointercancel":case"pointerdown":case"pointermove":case"pointerout":case"pointerover":case"pointerup":p=lp}var _=(e&4)!==0,m=!_&&t==="scroll",u=_?h!==null?h+"Capture":null:h;_=[];for(var g=c,y;g!==null;){y=g;var x=y.stateNode;if(y.tag===5&&x!==null&&(y=x,u!==null&&(x=js(g,u),x!=null&&_.push(Qs(g,x,y)))),m)break;g=g.return}0<_.length&&(h=new p(h,v,null,n,d),f.push({event:h,listeners:_}))}}if(!(e&7)){e:{if(h=t==="mouseover"||t==="pointerover",p=t==="mouseout"||t==="pointerout",h&&n!==Ju&&(v=n.relatedTarget||n.fromElement)&&(Er(v)||v[wi]))break e;if((p||h)&&(h=d.window===d?d:(h=d.ownerDocument)?h.defaultView||h.parentWindow:window,p?(v=n.relatedTarget||n.toElement,p=c,v=v?Er(v):null,v!==null&&(m=zr(v),v!==m||v.tag!==5&&v.tag!==6)&&(v=null)):(p=null,v=c),p!==v)){if(_=sp,x="onMouseLeave",u="onMouseEnter",g="mouse",(t==="pointerout"||t==="pointerover")&&(_=lp,x="onPointerLeave",u="onPointerEnter",g="pointer"),m=p==null?h:po(p),y=v==null?h:po(v),h=new _(x,g+"leave",p,n,d),h.target=m,h.relatedTarget=y,x=null,Er(d)===c&&(_=new _(u,g+"enter",v,n,d),_.target=y,_.relatedTarget=m,x=_),m=x,p&&v)t:{for(_=p,u=v,g=0,y=_;y;y=Gr(y))g++;for(y=0,x=u;x;x=Gr(x))y++;for(;0<g-y;)_=Gr(_),g--;for(;0<y-g;)u=Gr(u),y--;for(;g--;){if(_===u||u!==null&&_===u.alternate)break t;_=Gr(_),u=Gr(u)}_=null}else _=null;p!==null&&_p(f,h,p,_,!1),v!==null&&m!==null&&_p(f,m,v,_,!0)}}e:{if(h=c?po(c):window,p=h.nodeName&&h.nodeName.toLowerCase(),p==="select"||p==="input"&&h.type==="file")var C=Ex;else if(dp(h))if(g0)C=bx;else{C=Ax;var A=Tx}else(p=h.nodeName)&&p.toLowerCase()==="input"&&(h.type==="checkbox"||h.type==="radio")&&(C=Cx);if(C&&(C=C(t,c))){m0(f,C,n,d);break e}A&&A(t,h,c),t==="focusout"&&(A=h._wrapperState)&&A.controlled&&h.type==="number"&&Yu(h,"number",h.value)}switch(A=c?po(c):window,t){case"focusin":(dp(A)||A.contentEditable==="true")&&(ho=A,od=c,ks=null);break;case"focusout":ks=od=ho=null;break;case"mousedown":sd=!0;break;case"contextmenu":case"mouseup":case"dragend":sd=!1,gp(f,n,d);break;case"selectionchange":if(Dx)break;case"keydown":case"keyup":gp(f,n,d)}var T;if(Hh)e:{switch(t){case"compositionstart":var b="onCompositionStart";break e;case"compositionend":b="onCompositionEnd";break e;case"compositionupdate":b="onCompositionUpdate";break e}b=void 0}else uo?f0(t,n)&&(b="onCompositionEnd"):t==="keydown"&&n.keyCode===229&&(b="onCompositionStart");b&&(h0&&n.locale!=="ko"&&(uo||b!=="onCompositionStart"?b==="onCompositionEnd"&&uo&&(T=d0()):(Wi=d,Oh="value"in Wi?Wi.value:Wi.textContent,uo=!0)),A=kl(c,b),0<A.length&&(b=new ap(b,t,null,n,d),f.push({event:b,listeners:A}),T?b.data=T:(T=p0(n),T!==null&&(b.data=T)))),(T=_x?xx(t,n):Sx(t,n))&&(c=kl(c,"onBeforeInput"),0<c.length&&(d=new ap("onBeforeInput","beforeinput",null,n,d),f.push({event:d,listeners:c}),d.data=T))}A0(f,e)})}function Qs(t,e,n){return{instance:t,listener:e,currentTarget:n}}function kl(t,e){for(var n=e+"Capture",i=[];t!==null;){var r=t,o=r.stateNode;r.tag===5&&o!==null&&(r=o,o=js(t,n),o!=null&&i.unshift(Qs(t,o,r)),o=js(t,e),o!=null&&i.push(Qs(t,o,r))),t=t.return}return i}function Gr(t){if(t===null)return null;do t=t.return;while(t&&t.tag!==5);return t||null}function _p(t,e,n,i,r){for(var o=e._reactName,s=[];n!==null&&n!==i;){var a=n,l=a.alternate,c=a.stateNode;if(l!==null&&l===i)break;a.tag===5&&c!==null&&(a=c,r?(l=js(n,o),l!=null&&s.unshift(Qs(n,l,a))):r||(l=js(n,o),l!=null&&s.push(Qs(n,l,a)))),n=n.return}s.length!==0&&t.push({event:e,listeners:s})}var kx=/\r\n?/g,Ux=/\u0000|\uFFFD/g;function xp(t){return(typeof t=="string"?t:""+t).replace(kx,`
`).replace(Ux,"")}function La(t,e,n){if(e=xp(e),xp(t)!==e&&n)throw Error(ie(425))}function Ul(){}var ad=null,ld=null;function cd(t,e){return t==="textarea"||t==="noscript"||typeof e.children=="string"||typeof e.children=="number"||typeof e.dangerouslySetInnerHTML=="object"&&e.dangerouslySetInnerHTML!==null&&e.dangerouslySetInnerHTML.__html!=null}var ud=typeof setTimeout=="function"?setTimeout:void 0,Fx=typeof clearTimeout=="function"?clearTimeout:void 0,Sp=typeof Promise=="function"?Promise:void 0,Ox=typeof queueMicrotask=="function"?queueMicrotask:typeof Sp<"u"?function(t){return Sp.resolve(null).then(t).catch(zx)}:ud;function zx(t){setTimeout(function(){throw t})}function Xc(t,e){var n=e,i=0;do{var r=n.nextSibling;if(t.removeChild(n),r&&r.nodeType===8)if(n=r.data,n==="/$"){if(i===0){t.removeChild(r),$s(e);return}i--}else n!=="$"&&n!=="$?"&&n!=="$!"||i++;n=r}while(n);$s(e)}function Zi(t){for(;t!=null;t=t.nextSibling){var e=t.nodeType;if(e===1||e===3)break;if(e===8){if(e=t.data,e==="$"||e==="$!"||e==="$?")break;if(e==="/$")return null}}return t}function Mp(t){t=t.previousSibling;for(var e=0;t;){if(t.nodeType===8){var n=t.data;if(n==="$"||n==="$!"||n==="$?"){if(e===0)return t;e--}else n==="/$"&&e++}t=t.previousSibling}return null}var Ko=Math.random().toString(36).slice(2),ei="__reactFiber$"+Ko,ea="__reactProps$"+Ko,wi="__reactContainer$"+Ko,dd="__reactEvents$"+Ko,Bx="__reactListeners$"+Ko,Hx="__reactHandles$"+Ko;function Er(t){var e=t[ei];if(e)return e;for(var n=t.parentNode;n;){if(e=n[wi]||n[ei]){if(n=e.alternate,e.child!==null||n!==null&&n.child!==null)for(t=Mp(t);t!==null;){if(n=t[ei])return n;t=Mp(t)}return e}t=n,n=t.parentNode}return null}function pa(t){return t=t[ei]||t[wi],!t||t.tag!==5&&t.tag!==6&&t.tag!==13&&t.tag!==3?null:t}function po(t){if(t.tag===5||t.tag===6)return t.stateNode;throw Error(ie(33))}function uc(t){return t[ea]||null}var hd=[],mo=-1;function lr(t){return{current:t}}function pt(t){0>mo||(t.current=hd[mo],hd[mo]=null,mo--)}function lt(t,e){mo++,hd[mo]=t.current,t.current=e}var or={},jt=lr(or),ln=lr(!1),Lr=or;function ko(t,e){var n=t.type.contextTypes;if(!n)return or;var i=t.stateNode;if(i&&i.__reactInternalMemoizedUnmaskedChildContext===e)return i.__reactInternalMemoizedMaskedChildContext;var r={},o;for(o in n)r[o]=e[o];return i&&(t=t.stateNode,t.__reactInternalMemoizedUnmaskedChildContext=e,t.__reactInternalMemoizedMaskedChildContext=r),r}function cn(t){return t=t.childContextTypes,t!=null}function Fl(){pt(ln),pt(jt)}function wp(t,e,n){if(jt.current!==or)throw Error(ie(168));lt(jt,e),lt(ln,n)}function b0(t,e,n){var i=t.stateNode;if(e=e.childContextTypes,typeof i.getChildContext!="function")return n;i=i.getChildContext();for(var r in i)if(!(r in e))throw Error(ie(108,T_(t)||"Unknown",r));return yt({},n,i)}function Ol(t){return t=(t=t.stateNode)&&t.__reactInternalMemoizedMergedChildContext||or,Lr=jt.current,lt(jt,t),lt(ln,ln.current),!0}function Ep(t,e,n){var i=t.stateNode;if(!i)throw Error(ie(169));n?(t=b0(t,e,Lr),i.__reactInternalMemoizedMergedChildContext=t,pt(ln),pt(jt),lt(jt,t)):pt(ln),lt(ln,n)}var mi=null,dc=!1,jc=!1;function R0(t){mi===null?mi=[t]:mi.push(t)}function Vx(t){dc=!0,R0(t)}function cr(){if(!jc&&mi!==null){jc=!0;var t=0,e=tt;try{var n=mi;for(tt=1;t<n.length;t++){var i=n[t];do i=i(!0);while(i!==null)}mi=null,dc=!1}catch(r){throw mi!==null&&(mi=mi.slice(t+1)),e0(Ih,cr),r}finally{tt=e,jc=!1}}return null}var go=[],vo=0,zl=null,Bl=0,Tn=[],An=0,Nr=null,vi=1,yi="";function yr(t,e){go[vo++]=Bl,go[vo++]=zl,zl=t,Bl=e}function P0(t,e,n){Tn[An++]=vi,Tn[An++]=yi,Tn[An++]=Nr,Nr=t;var i=vi;t=yi;var r=32-Xn(i)-1;i&=~(1<<r),n+=1;var o=32-Xn(e)+r;if(30<o){var s=r-r%5;o=(i&(1<<s)-1).toString(32),i>>=s,r-=s,vi=1<<32-Xn(e)+r|n<<r|i,yi=o+t}else vi=1<<o|n<<r|i,yi=t}function Gh(t){t.return!==null&&(yr(t,1),P0(t,1,0))}function Wh(t){for(;t===zl;)zl=go[--vo],go[vo]=null,Bl=go[--vo],go[vo]=null;for(;t===Nr;)Nr=Tn[--An],Tn[An]=null,yi=Tn[--An],Tn[An]=null,vi=Tn[--An],Tn[An]=null}var yn=null,vn=null,mt=!1,Gn=null;function D0(t,e){var n=Rn(5,null,null,0);n.elementType="DELETED",n.stateNode=e,n.return=t,e=t.deletions,e===null?(t.deletions=[n],t.flags|=16):e.push(n)}function Tp(t,e){switch(t.tag){case 5:var n=t.type;return e=e.nodeType!==1||n.toLowerCase()!==e.nodeName.toLowerCase()?null:e,e!==null?(t.stateNode=e,yn=t,vn=Zi(e.firstChild),!0):!1;case 6:return e=t.pendingProps===""||e.nodeType!==3?null:e,e!==null?(t.stateNode=e,yn=t,vn=null,!0):!1;case 13:return e=e.nodeType!==8?null:e,e!==null?(n=Nr!==null?{id:vi,overflow:yi}:null,t.memoizedState={dehydrated:e,treeContext:n,retryLane:1073741824},n=Rn(18,null,null,0),n.stateNode=e,n.return=t,t.child=n,yn=t,vn=null,!0):!1;default:return!1}}function fd(t){return(t.mode&1)!==0&&(t.flags&128)===0}function pd(t){if(mt){var e=vn;if(e){var n=e;if(!Tp(t,e)){if(fd(t))throw Error(ie(418));e=Zi(n.nextSibling);var i=yn;e&&Tp(t,e)?D0(i,n):(t.flags=t.flags&-4097|2,mt=!1,yn=t)}}else{if(fd(t))throw Error(ie(418));t.flags=t.flags&-4097|2,mt=!1,yn=t}}}function Ap(t){for(t=t.return;t!==null&&t.tag!==5&&t.tag!==3&&t.tag!==13;)t=t.return;yn=t}function Na(t){if(t!==yn)return!1;if(!mt)return Ap(t),mt=!0,!1;var e;if((e=t.tag!==3)&&!(e=t.tag!==5)&&(e=t.type,e=e!=="head"&&e!=="body"&&!cd(t.type,t.memoizedProps)),e&&(e=vn)){if(fd(t))throw L0(),Error(ie(418));for(;e;)D0(t,e),e=Zi(e.nextSibling)}if(Ap(t),t.tag===13){if(t=t.memoizedState,t=t!==null?t.dehydrated:null,!t)throw Error(ie(317));e:{for(t=t.nextSibling,e=0;t;){if(t.nodeType===8){var n=t.data;if(n==="/$"){if(e===0){vn=Zi(t.nextSibling);break e}e--}else n!=="$"&&n!=="$!"&&n!=="$?"||e++}t=t.nextSibling}vn=null}}else vn=yn?Zi(t.stateNode.nextSibling):null;return!0}function L0(){for(var t=vn;t;)t=Zi(t.nextSibling)}function Uo(){vn=yn=null,mt=!1}function Xh(t){Gn===null?Gn=[t]:Gn.push(t)}var Gx=Ai.ReactCurrentBatchConfig;function us(t,e,n){if(t=n.ref,t!==null&&typeof t!="function"&&typeof t!="object"){if(n._owner){if(n=n._owner,n){if(n.tag!==1)throw Error(ie(309));var i=n.stateNode}if(!i)throw Error(ie(147,t));var r=i,o=""+t;return e!==null&&e.ref!==null&&typeof e.ref=="function"&&e.ref._stringRef===o?e.ref:(e=function(s){var a=r.refs;s===null?delete a[o]:a[o]=s},e._stringRef=o,e)}if(typeof t!="string")throw Error(ie(284));if(!n._owner)throw Error(ie(290,t))}return t}function Ia(t,e){throw t=Object.prototype.toString.call(e),Error(ie(31,t==="[object Object]"?"object with keys {"+Object.keys(e).join(", ")+"}":t))}function Cp(t){var e=t._init;return e(t._payload)}function N0(t){function e(u,g){if(t){var y=u.deletions;y===null?(u.deletions=[g],u.flags|=16):y.push(g)}}function n(u,g){if(!t)return null;for(;g!==null;)e(u,g),g=g.sibling;return null}function i(u,g){for(u=new Map;g!==null;)g.key!==null?u.set(g.key,g):u.set(g.index,g),g=g.sibling;return u}function r(u,g){return u=tr(u,g),u.index=0,u.sibling=null,u}function o(u,g,y){return u.index=y,t?(y=u.alternate,y!==null?(y=y.index,y<g?(u.flags|=2,g):y):(u.flags|=2,g)):(u.flags|=1048576,g)}function s(u){return t&&u.alternate===null&&(u.flags|=2),u}function a(u,g,y,x){return g===null||g.tag!==6?(g=Qc(y,u.mode,x),g.return=u,g):(g=r(g,y),g.return=u,g)}function l(u,g,y,x){var C=y.type;return C===co?d(u,g,y.props.children,x,y.key):g!==null&&(g.elementType===C||typeof C=="object"&&C!==null&&C.$$typeof===Fi&&Cp(C)===g.type)?(x=r(g,y.props),x.ref=us(u,g,y),x.return=u,x):(x=xl(y.type,y.key,y.props,null,u.mode,x),x.ref=us(u,g,y),x.return=u,x)}function c(u,g,y,x){return g===null||g.tag!==4||g.stateNode.containerInfo!==y.containerInfo||g.stateNode.implementation!==y.implementation?(g=eu(y,u.mode,x),g.return=u,g):(g=r(g,y.children||[]),g.return=u,g)}function d(u,g,y,x,C){return g===null||g.tag!==7?(g=Dr(y,u.mode,x,C),g.return=u,g):(g=r(g,y),g.return=u,g)}function f(u,g,y){if(typeof g=="string"&&g!==""||typeof g=="number")return g=Qc(""+g,u.mode,y),g.return=u,g;if(typeof g=="object"&&g!==null){switch(g.$$typeof){case wa:return y=xl(g.type,g.key,g.props,null,u.mode,y),y.ref=us(u,null,g),y.return=u,y;case lo:return g=eu(g,u.mode,y),g.return=u,g;case Fi:var x=g._init;return f(u,x(g._payload),y)}if(As(g)||os(g))return g=Dr(g,u.mode,y,null),g.return=u,g;Ia(u,g)}return null}function h(u,g,y,x){var C=g!==null?g.key:null;if(typeof y=="string"&&y!==""||typeof y=="number")return C!==null?null:a(u,g,""+y,x);if(typeof y=="object"&&y!==null){switch(y.$$typeof){case wa:return y.key===C?l(u,g,y,x):null;case lo:return y.key===C?c(u,g,y,x):null;case Fi:return C=y._init,h(u,g,C(y._payload),x)}if(As(y)||os(y))return C!==null?null:d(u,g,y,x,null);Ia(u,y)}return null}function p(u,g,y,x,C){if(typeof x=="string"&&x!==""||typeof x=="number")return u=u.get(y)||null,a(g,u,""+x,C);if(typeof x=="object"&&x!==null){switch(x.$$typeof){case wa:return u=u.get(x.key===null?y:x.key)||null,l(g,u,x,C);case lo:return u=u.get(x.key===null?y:x.key)||null,c(g,u,x,C);case Fi:var A=x._init;return p(u,g,y,A(x._payload),C)}if(As(x)||os(x))return u=u.get(y)||null,d(g,u,x,C,null);Ia(g,x)}return null}function v(u,g,y,x){for(var C=null,A=null,T=g,b=g=0,w=null;T!==null&&b<y.length;b++){T.index>b?(w=T,T=null):w=T.sibling;var S=h(u,T,y[b],x);if(S===null){T===null&&(T=w);break}t&&T&&S.alternate===null&&e(u,T),g=o(S,g,b),A===null?C=S:A.sibling=S,A=S,T=w}if(b===y.length)return n(u,T),mt&&yr(u,b),C;if(T===null){for(;b<y.length;b++)T=f(u,y[b],x),T!==null&&(g=o(T,g,b),A===null?C=T:A.sibling=T,A=T);return mt&&yr(u,b),C}for(T=i(u,T);b<y.length;b++)w=p(T,u,b,y[b],x),w!==null&&(t&&w.alternate!==null&&T.delete(w.key===null?b:w.key),g=o(w,g,b),A===null?C=w:A.sibling=w,A=w);return t&&T.forEach(function(P){return e(u,P)}),mt&&yr(u,b),C}function _(u,g,y,x){var C=os(y);if(typeof C!="function")throw Error(ie(150));if(y=C.call(y),y==null)throw Error(ie(151));for(var A=C=null,T=g,b=g=0,w=null,S=y.next();T!==null&&!S.done;b++,S=y.next()){T.index>b?(w=T,T=null):w=T.sibling;var P=h(u,T,S.value,x);if(P===null){T===null&&(T=w);break}t&&T&&P.alternate===null&&e(u,T),g=o(P,g,b),A===null?C=P:A.sibling=P,A=P,T=w}if(S.done)return n(u,T),mt&&yr(u,b),C;if(T===null){for(;!S.done;b++,S=y.next())S=f(u,S.value,x),S!==null&&(g=o(S,g,b),A===null?C=S:A.sibling=S,A=S);return mt&&yr(u,b),C}for(T=i(u,T);!S.done;b++,S=y.next())S=p(T,u,b,S.value,x),S!==null&&(t&&S.alternate!==null&&T.delete(S.key===null?b:S.key),g=o(S,g,b),A===null?C=S:A.sibling=S,A=S);return t&&T.forEach(function(z){return e(u,z)}),mt&&yr(u,b),C}function m(u,g,y,x){if(typeof y=="object"&&y!==null&&y.type===co&&y.key===null&&(y=y.props.children),typeof y=="object"&&y!==null){switch(y.$$typeof){case wa:e:{for(var C=y.key,A=g;A!==null;){if(A.key===C){if(C=y.type,C===co){if(A.tag===7){n(u,A.sibling),g=r(A,y.props.children),g.return=u,u=g;break e}}else if(A.elementType===C||typeof C=="object"&&C!==null&&C.$$typeof===Fi&&Cp(C)===A.type){n(u,A.sibling),g=r(A,y.props),g.ref=us(u,A,y),g.return=u,u=g;break e}n(u,A);break}else e(u,A);A=A.sibling}y.type===co?(g=Dr(y.props.children,u.mode,x,y.key),g.return=u,u=g):(x=xl(y.type,y.key,y.props,null,u.mode,x),x.ref=us(u,g,y),x.return=u,u=x)}return s(u);case lo:e:{for(A=y.key;g!==null;){if(g.key===A)if(g.tag===4&&g.stateNode.containerInfo===y.containerInfo&&g.stateNode.implementation===y.implementation){n(u,g.sibling),g=r(g,y.children||[]),g.return=u,u=g;break e}else{n(u,g);break}else e(u,g);g=g.sibling}g=eu(y,u.mode,x),g.return=u,u=g}return s(u);case Fi:return A=y._init,m(u,g,A(y._payload),x)}if(As(y))return v(u,g,y,x);if(os(y))return _(u,g,y,x);Ia(u,y)}return typeof y=="string"&&y!==""||typeof y=="number"?(y=""+y,g!==null&&g.tag===6?(n(u,g.sibling),g=r(g,y),g.return=u,u=g):(n(u,g),g=Qc(y,u.mode,x),g.return=u,u=g),s(u)):n(u,g)}return m}var Fo=N0(!0),I0=N0(!1),Hl=lr(null),Vl=null,yo=null,jh=null;function Yh(){jh=yo=Vl=null}function qh(t){var e=Hl.current;pt(Hl),t._currentValue=e}function md(t,e,n){for(;t!==null;){var i=t.alternate;if((t.childLanes&e)!==e?(t.childLanes|=e,i!==null&&(i.childLanes|=e)):i!==null&&(i.childLanes&e)!==e&&(i.childLanes|=e),t===n)break;t=t.return}}function bo(t,e){Vl=t,jh=yo=null,t=t.dependencies,t!==null&&t.firstContext!==null&&(t.lanes&e&&(an=!0),t.firstContext=null)}function Ln(t){var e=t._currentValue;if(jh!==t)if(t={context:t,memoizedValue:e,next:null},yo===null){if(Vl===null)throw Error(ie(308));yo=t,Vl.dependencies={lanes:0,firstContext:t}}else yo=yo.next=t;return e}var Tr=null;function $h(t){Tr===null?Tr=[t]:Tr.push(t)}function k0(t,e,n,i){var r=e.interleaved;return r===null?(n.next=n,$h(e)):(n.next=r.next,r.next=n),e.interleaved=n,Ei(t,i)}function Ei(t,e){t.lanes|=e;var n=t.alternate;for(n!==null&&(n.lanes|=e),n=t,t=t.return;t!==null;)t.childLanes|=e,n=t.alternate,n!==null&&(n.childLanes|=e),n=t,t=t.return;return n.tag===3?n.stateNode:null}var Oi=!1;function Kh(t){t.updateQueue={baseState:t.memoizedState,firstBaseUpdate:null,lastBaseUpdate:null,shared:{pending:null,interleaved:null,lanes:0},effects:null}}function U0(t,e){t=t.updateQueue,e.updateQueue===t&&(e.updateQueue={baseState:t.baseState,firstBaseUpdate:t.firstBaseUpdate,lastBaseUpdate:t.lastBaseUpdate,shared:t.shared,effects:t.effects})}function xi(t,e){return{eventTime:t,lane:e,tag:0,payload:null,callback:null,next:null}}function Ji(t,e,n){var i=t.updateQueue;if(i===null)return null;if(i=i.shared,Ke&2){var r=i.pending;return r===null?e.next=e:(e.next=r.next,r.next=e),i.pending=e,Ei(t,n)}return r=i.interleaved,r===null?(e.next=e,$h(i)):(e.next=r.next,r.next=e),i.interleaved=e,Ei(t,n)}function pl(t,e,n){if(e=e.updateQueue,e!==null&&(e=e.shared,(n&4194240)!==0)){var i=e.lanes;i&=t.pendingLanes,n|=i,e.lanes=n,kh(t,n)}}function bp(t,e){var n=t.updateQueue,i=t.alternate;if(i!==null&&(i=i.updateQueue,n===i)){var r=null,o=null;if(n=n.firstBaseUpdate,n!==null){do{var s={eventTime:n.eventTime,lane:n.lane,tag:n.tag,payload:n.payload,callback:n.callback,next:null};o===null?r=o=s:o=o.next=s,n=n.next}while(n!==null);o===null?r=o=e:o=o.next=e}else r=o=e;n={baseState:i.baseState,firstBaseUpdate:r,lastBaseUpdate:o,shared:i.shared,effects:i.effects},t.updateQueue=n;return}t=n.lastBaseUpdate,t===null?n.firstBaseUpdate=e:t.next=e,n.lastBaseUpdate=e}function Gl(t,e,n,i){var r=t.updateQueue;Oi=!1;var o=r.firstBaseUpdate,s=r.lastBaseUpdate,a=r.shared.pending;if(a!==null){r.shared.pending=null;var l=a,c=l.next;l.next=null,s===null?o=c:s.next=c,s=l;var d=t.alternate;d!==null&&(d=d.updateQueue,a=d.lastBaseUpdate,a!==s&&(a===null?d.firstBaseUpdate=c:a.next=c,d.lastBaseUpdate=l))}if(o!==null){var f=r.baseState;s=0,d=c=l=null,a=o;do{var h=a.lane,p=a.eventTime;if((i&h)===h){d!==null&&(d=d.next={eventTime:p,lane:0,tag:a.tag,payload:a.payload,callback:a.callback,next:null});e:{var v=t,_=a;switch(h=e,p=n,_.tag){case 1:if(v=_.payload,typeof v=="function"){f=v.call(p,f,h);break e}f=v;break e;case 3:v.flags=v.flags&-65537|128;case 0:if(v=_.payload,h=typeof v=="function"?v.call(p,f,h):v,h==null)break e;f=yt({},f,h);break e;case 2:Oi=!0}}a.callback!==null&&a.lane!==0&&(t.flags|=64,h=r.effects,h===null?r.effects=[a]:h.push(a))}else p={eventTime:p,lane:h,tag:a.tag,payload:a.payload,callback:a.callback,next:null},d===null?(c=d=p,l=f):d=d.next=p,s|=h;if(a=a.next,a===null){if(a=r.shared.pending,a===null)break;h=a,a=h.next,h.next=null,r.lastBaseUpdate=h,r.shared.pending=null}}while(!0);if(d===null&&(l=f),r.baseState=l,r.firstBaseUpdate=c,r.lastBaseUpdate=d,e=r.shared.interleaved,e!==null){r=e;do s|=r.lane,r=r.next;while(r!==e)}else o===null&&(r.shared.lanes=0);kr|=s,t.lanes=s,t.memoizedState=f}}function Rp(t,e,n){if(t=e.effects,e.effects=null,t!==null)for(e=0;e<t.length;e++){var i=t[e],r=i.callback;if(r!==null){if(i.callback=null,i=n,typeof r!="function")throw Error(ie(191,r));r.call(i)}}}var ma={},ri=lr(ma),ta=lr(ma),na=lr(ma);function Ar(t){if(t===ma)throw Error(ie(174));return t}function Zh(t,e){switch(lt(na,e),lt(ta,t),lt(ri,ma),t=e.nodeType,t){case 9:case 11:e=(e=e.documentElement)?e.namespaceURI:$u(null,"");break;default:t=t===8?e.parentNode:e,e=t.namespaceURI||null,t=t.tagName,e=$u(e,t)}pt(ri),lt(ri,e)}function Oo(){pt(ri),pt(ta),pt(na)}function F0(t){Ar(na.current);var e=Ar(ri.current),n=$u(e,t.type);e!==n&&(lt(ta,t),lt(ri,n))}function Jh(t){ta.current===t&&(pt(ri),pt(ta))}var gt=lr(0);function Wl(t){for(var e=t;e!==null;){if(e.tag===13){var n=e.memoizedState;if(n!==null&&(n=n.dehydrated,n===null||n.data==="$?"||n.data==="$!"))return e}else if(e.tag===19&&e.memoizedProps.revealOrder!==void 0){if(e.flags&128)return e}else if(e.child!==null){e.child.return=e,e=e.child;continue}if(e===t)break;for(;e.sibling===null;){if(e.return===null||e.return===t)return null;e=e.return}e.sibling.return=e.return,e=e.sibling}return null}var Yc=[];function Qh(){for(var t=0;t<Yc.length;t++)Yc[t]._workInProgressVersionPrimary=null;Yc.length=0}var ml=Ai.ReactCurrentDispatcher,qc=Ai.ReactCurrentBatchConfig,Ir=0,vt=null,Tt=null,Dt=null,Xl=!1,Us=!1,ia=0,Wx=0;function Ot(){throw Error(ie(321))}function ef(t,e){if(e===null)return!1;for(var n=0;n<e.length&&n<t.length;n++)if(!Yn(t[n],e[n]))return!1;return!0}function tf(t,e,n,i,r,o){if(Ir=o,vt=e,e.memoizedState=null,e.updateQueue=null,e.lanes=0,ml.current=t===null||t.memoizedState===null?qx:$x,t=n(i,r),Us){o=0;do{if(Us=!1,ia=0,25<=o)throw Error(ie(301));o+=1,Dt=Tt=null,e.updateQueue=null,ml.current=Kx,t=n(i,r)}while(Us)}if(ml.current=jl,e=Tt!==null&&Tt.next!==null,Ir=0,Dt=Tt=vt=null,Xl=!1,e)throw Error(ie(300));return t}function nf(){var t=ia!==0;return ia=0,t}function Jn(){var t={memoizedState:null,baseState:null,baseQueue:null,queue:null,next:null};return Dt===null?vt.memoizedState=Dt=t:Dt=Dt.next=t,Dt}function Nn(){if(Tt===null){var t=vt.alternate;t=t!==null?t.memoizedState:null}else t=Tt.next;var e=Dt===null?vt.memoizedState:Dt.next;if(e!==null)Dt=e,Tt=t;else{if(t===null)throw Error(ie(310));Tt=t,t={memoizedState:Tt.memoizedState,baseState:Tt.baseState,baseQueue:Tt.baseQueue,queue:Tt.queue,next:null},Dt===null?vt.memoizedState=Dt=t:Dt=Dt.next=t}return Dt}function ra(t,e){return typeof e=="function"?e(t):e}function $c(t){var e=Nn(),n=e.queue;if(n===null)throw Error(ie(311));n.lastRenderedReducer=t;var i=Tt,r=i.baseQueue,o=n.pending;if(o!==null){if(r!==null){var s=r.next;r.next=o.next,o.next=s}i.baseQueue=r=o,n.pending=null}if(r!==null){o=r.next,i=i.baseState;var a=s=null,l=null,c=o;do{var d=c.lane;if((Ir&d)===d)l!==null&&(l=l.next={lane:0,action:c.action,hasEagerState:c.hasEagerState,eagerState:c.eagerState,next:null}),i=c.hasEagerState?c.eagerState:t(i,c.action);else{var f={lane:d,action:c.action,hasEagerState:c.hasEagerState,eagerState:c.eagerState,next:null};l===null?(a=l=f,s=i):l=l.next=f,vt.lanes|=d,kr|=d}c=c.next}while(c!==null&&c!==o);l===null?s=i:l.next=a,Yn(i,e.memoizedState)||(an=!0),e.memoizedState=i,e.baseState=s,e.baseQueue=l,n.lastRenderedState=i}if(t=n.interleaved,t!==null){r=t;do o=r.lane,vt.lanes|=o,kr|=o,r=r.next;while(r!==t)}else r===null&&(n.lanes=0);return[e.memoizedState,n.dispatch]}function Kc(t){var e=Nn(),n=e.queue;if(n===null)throw Error(ie(311));n.lastRenderedReducer=t;var i=n.dispatch,r=n.pending,o=e.memoizedState;if(r!==null){n.pending=null;var s=r=r.next;do o=t(o,s.action),s=s.next;while(s!==r);Yn(o,e.memoizedState)||(an=!0),e.memoizedState=o,e.baseQueue===null&&(e.baseState=o),n.lastRenderedState=o}return[o,i]}function O0(){}function z0(t,e){var n=vt,i=Nn(),r=e(),o=!Yn(i.memoizedState,r);if(o&&(i.memoizedState=r,an=!0),i=i.queue,rf(V0.bind(null,n,i,t),[t]),i.getSnapshot!==e||o||Dt!==null&&Dt.memoizedState.tag&1){if(n.flags|=2048,oa(9,H0.bind(null,n,i,r,e),void 0,null),Lt===null)throw Error(ie(349));Ir&30||B0(n,e,r)}return r}function B0(t,e,n){t.flags|=16384,t={getSnapshot:e,value:n},e=vt.updateQueue,e===null?(e={lastEffect:null,stores:null},vt.updateQueue=e,e.stores=[t]):(n=e.stores,n===null?e.stores=[t]:n.push(t))}function H0(t,e,n,i){e.value=n,e.getSnapshot=i,G0(e)&&W0(t)}function V0(t,e,n){return n(function(){G0(e)&&W0(t)})}function G0(t){var e=t.getSnapshot;t=t.value;try{var n=e();return!Yn(t,n)}catch{return!0}}function W0(t){var e=Ei(t,1);e!==null&&jn(e,t,1,-1)}function Pp(t){var e=Jn();return typeof t=="function"&&(t=t()),e.memoizedState=e.baseState=t,t={pending:null,interleaved:null,lanes:0,dispatch:null,lastRenderedReducer:ra,lastRenderedState:t},e.queue=t,t=t.dispatch=Yx.bind(null,vt,t),[e.memoizedState,t]}function oa(t,e,n,i){return t={tag:t,create:e,destroy:n,deps:i,next:null},e=vt.updateQueue,e===null?(e={lastEffect:null,stores:null},vt.updateQueue=e,e.lastEffect=t.next=t):(n=e.lastEffect,n===null?e.lastEffect=t.next=t:(i=n.next,n.next=t,t.next=i,e.lastEffect=t)),t}function X0(){return Nn().memoizedState}function gl(t,e,n,i){var r=Jn();vt.flags|=t,r.memoizedState=oa(1|e,n,void 0,i===void 0?null:i)}function hc(t,e,n,i){var r=Nn();i=i===void 0?null:i;var o=void 0;if(Tt!==null){var s=Tt.memoizedState;if(o=s.destroy,i!==null&&ef(i,s.deps)){r.memoizedState=oa(e,n,o,i);return}}vt.flags|=t,r.memoizedState=oa(1|e,n,o,i)}function Dp(t,e){return gl(8390656,8,t,e)}function rf(t,e){return hc(2048,8,t,e)}function j0(t,e){return hc(4,2,t,e)}function Y0(t,e){return hc(4,4,t,e)}function q0(t,e){if(typeof e=="function")return t=t(),e(t),function(){e(null)};if(e!=null)return t=t(),e.current=t,function(){e.current=null}}function $0(t,e,n){return n=n!=null?n.concat([t]):null,hc(4,4,q0.bind(null,e,t),n)}function of(){}function K0(t,e){var n=Nn();e=e===void 0?null:e;var i=n.memoizedState;return i!==null&&e!==null&&ef(e,i[1])?i[0]:(n.memoizedState=[t,e],t)}function Z0(t,e){var n=Nn();e=e===void 0?null:e;var i=n.memoizedState;return i!==null&&e!==null&&ef(e,i[1])?i[0]:(t=t(),n.memoizedState=[t,e],t)}function J0(t,e,n){return Ir&21?(Yn(n,e)||(n=i0(),vt.lanes|=n,kr|=n,t.baseState=!0),e):(t.baseState&&(t.baseState=!1,an=!0),t.memoizedState=n)}function Xx(t,e){var n=tt;tt=n!==0&&4>n?n:4,t(!0);var i=qc.transition;qc.transition={};try{t(!1),e()}finally{tt=n,qc.transition=i}}function Q0(){return Nn().memoizedState}function jx(t,e,n){var i=er(t);if(n={lane:i,action:n,hasEagerState:!1,eagerState:null,next:null},ev(t))tv(e,n);else if(n=k0(t,e,n,i),n!==null){var r=Zt();jn(n,t,i,r),nv(n,e,i)}}function Yx(t,e,n){var i=er(t),r={lane:i,action:n,hasEagerState:!1,eagerState:null,next:null};if(ev(t))tv(e,r);else{var o=t.alternate;if(t.lanes===0&&(o===null||o.lanes===0)&&(o=e.lastRenderedReducer,o!==null))try{var s=e.lastRenderedState,a=o(s,n);if(r.hasEagerState=!0,r.eagerState=a,Yn(a,s)){var l=e.interleaved;l===null?(r.next=r,$h(e)):(r.next=l.next,l.next=r),e.interleaved=r;return}}catch{}finally{}n=k0(t,e,r,i),n!==null&&(r=Zt(),jn(n,t,i,r),nv(n,e,i))}}function ev(t){var e=t.alternate;return t===vt||e!==null&&e===vt}function tv(t,e){Us=Xl=!0;var n=t.pending;n===null?e.next=e:(e.next=n.next,n.next=e),t.pending=e}function nv(t,e,n){if(n&4194240){var i=e.lanes;i&=t.pendingLanes,n|=i,e.lanes=n,kh(t,n)}}var jl={readContext:Ln,useCallback:Ot,useContext:Ot,useEffect:Ot,useImperativeHandle:Ot,useInsertionEffect:Ot,useLayoutEffect:Ot,useMemo:Ot,useReducer:Ot,useRef:Ot,useState:Ot,useDebugValue:Ot,useDeferredValue:Ot,useTransition:Ot,useMutableSource:Ot,useSyncExternalStore:Ot,useId:Ot,unstable_isNewReconciler:!1},qx={readContext:Ln,useCallback:function(t,e){return Jn().memoizedState=[t,e===void 0?null:e],t},useContext:Ln,useEffect:Dp,useImperativeHandle:function(t,e,n){return n=n!=null?n.concat([t]):null,gl(4194308,4,q0.bind(null,e,t),n)},useLayoutEffect:function(t,e){return gl(4194308,4,t,e)},useInsertionEffect:function(t,e){return gl(4,2,t,e)},useMemo:function(t,e){var n=Jn();return e=e===void 0?null:e,t=t(),n.memoizedState=[t,e],t},useReducer:function(t,e,n){var i=Jn();return e=n!==void 0?n(e):e,i.memoizedState=i.baseState=e,t={pending:null,interleaved:null,lanes:0,dispatch:null,lastRenderedReducer:t,lastRenderedState:e},i.queue=t,t=t.dispatch=jx.bind(null,vt,t),[i.memoizedState,t]},useRef:function(t){var e=Jn();return t={current:t},e.memoizedState=t},useState:Pp,useDebugValue:of,useDeferredValue:function(t){return Jn().memoizedState=t},useTransition:function(){var t=Pp(!1),e=t[0];return t=Xx.bind(null,t[1]),Jn().memoizedState=t,[e,t]},useMutableSource:function(){},useSyncExternalStore:function(t,e,n){var i=vt,r=Jn();if(mt){if(n===void 0)throw Error(ie(407));n=n()}else{if(n=e(),Lt===null)throw Error(ie(349));Ir&30||B0(i,e,n)}r.memoizedState=n;var o={value:n,getSnapshot:e};return r.queue=o,Dp(V0.bind(null,i,o,t),[t]),i.flags|=2048,oa(9,H0.bind(null,i,o,n,e),void 0,null),n},useId:function(){var t=Jn(),e=Lt.identifierPrefix;if(mt){var n=yi,i=vi;n=(i&~(1<<32-Xn(i)-1)).toString(32)+n,e=":"+e+"R"+n,n=ia++,0<n&&(e+="H"+n.toString(32)),e+=":"}else n=Wx++,e=":"+e+"r"+n.toString(32)+":";return t.memoizedState=e},unstable_isNewReconciler:!1},$x={readContext:Ln,useCallback:K0,useContext:Ln,useEffect:rf,useImperativeHandle:$0,useInsertionEffect:j0,useLayoutEffect:Y0,useMemo:Z0,useReducer:$c,useRef:X0,useState:function(){return $c(ra)},useDebugValue:of,useDeferredValue:function(t){var e=Nn();return J0(e,Tt.memoizedState,t)},useTransition:function(){var t=$c(ra)[0],e=Nn().memoizedState;return[t,e]},useMutableSource:O0,useSyncExternalStore:z0,useId:Q0,unstable_isNewReconciler:!1},Kx={readContext:Ln,useCallback:K0,useContext:Ln,useEffect:rf,useImperativeHandle:$0,useInsertionEffect:j0,useLayoutEffect:Y0,useMemo:Z0,useReducer:Kc,useRef:X0,useState:function(){return Kc(ra)},useDebugValue:of,useDeferredValue:function(t){var e=Nn();return Tt===null?e.memoizedState=t:J0(e,Tt.memoizedState,t)},useTransition:function(){var t=Kc(ra)[0],e=Nn().memoizedState;return[t,e]},useMutableSource:O0,useSyncExternalStore:z0,useId:Q0,unstable_isNewReconciler:!1};function Hn(t,e){if(t&&t.defaultProps){e=yt({},e),t=t.defaultProps;for(var n in t)e[n]===void 0&&(e[n]=t[n]);return e}return e}function gd(t,e,n,i){e=t.memoizedState,n=n(i,e),n=n==null?e:yt({},e,n),t.memoizedState=n,t.lanes===0&&(t.updateQueue.baseState=n)}var fc={isMounted:function(t){return(t=t._reactInternals)?zr(t)===t:!1},enqueueSetState:function(t,e,n){t=t._reactInternals;var i=Zt(),r=er(t),o=xi(i,r);o.payload=e,n!=null&&(o.callback=n),e=Ji(t,o,r),e!==null&&(jn(e,t,r,i),pl(e,t,r))},enqueueReplaceState:function(t,e,n){t=t._reactInternals;var i=Zt(),r=er(t),o=xi(i,r);o.tag=1,o.payload=e,n!=null&&(o.callback=n),e=Ji(t,o,r),e!==null&&(jn(e,t,r,i),pl(e,t,r))},enqueueForceUpdate:function(t,e){t=t._reactInternals;var n=Zt(),i=er(t),r=xi(n,i);r.tag=2,e!=null&&(r.callback=e),e=Ji(t,r,i),e!==null&&(jn(e,t,i,n),pl(e,t,i))}};function Lp(t,e,n,i,r,o,s){return t=t.stateNode,typeof t.shouldComponentUpdate=="function"?t.shouldComponentUpdate(i,o,s):e.prototype&&e.prototype.isPureReactComponent?!Zs(n,i)||!Zs(r,o):!0}function iv(t,e,n){var i=!1,r=or,o=e.contextType;return typeof o=="object"&&o!==null?o=Ln(o):(r=cn(e)?Lr:jt.current,i=e.contextTypes,o=(i=i!=null)?ko(t,r):or),e=new e(n,o),t.memoizedState=e.state!==null&&e.state!==void 0?e.state:null,e.updater=fc,t.stateNode=e,e._reactInternals=t,i&&(t=t.stateNode,t.__reactInternalMemoizedUnmaskedChildContext=r,t.__reactInternalMemoizedMaskedChildContext=o),e}function Np(t,e,n,i){t=e.state,typeof e.componentWillReceiveProps=="function"&&e.componentWillReceiveProps(n,i),typeof e.UNSAFE_componentWillReceiveProps=="function"&&e.UNSAFE_componentWillReceiveProps(n,i),e.state!==t&&fc.enqueueReplaceState(e,e.state,null)}function vd(t,e,n,i){var r=t.stateNode;r.props=n,r.state=t.memoizedState,r.refs={},Kh(t);var o=e.contextType;typeof o=="object"&&o!==null?r.context=Ln(o):(o=cn(e)?Lr:jt.current,r.context=ko(t,o)),r.state=t.memoizedState,o=e.getDerivedStateFromProps,typeof o=="function"&&(gd(t,e,o,n),r.state=t.memoizedState),typeof e.getDerivedStateFromProps=="function"||typeof r.getSnapshotBeforeUpdate=="function"||typeof r.UNSAFE_componentWillMount!="function"&&typeof r.componentWillMount!="function"||(e=r.state,typeof r.componentWillMount=="function"&&r.componentWillMount(),typeof r.UNSAFE_componentWillMount=="function"&&r.UNSAFE_componentWillMount(),e!==r.state&&fc.enqueueReplaceState(r,r.state,null),Gl(t,n,r,i),r.state=t.memoizedState),typeof r.componentDidMount=="function"&&(t.flags|=4194308)}function zo(t,e){try{var n="",i=e;do n+=E_(i),i=i.return;while(i);var r=n}catch(o){r=`
Error generating stack: `+o.message+`
`+o.stack}return{value:t,source:e,stack:r,digest:null}}function Zc(t,e,n){return{value:t,source:null,stack:n??null,digest:e??null}}function yd(t,e){try{console.error(e.value)}catch(n){setTimeout(function(){throw n})}}var Zx=typeof WeakMap=="function"?WeakMap:Map;function rv(t,e,n){n=xi(-1,n),n.tag=3,n.payload={element:null};var i=e.value;return n.callback=function(){ql||(ql=!0,bd=i),yd(t,e)},n}function ov(t,e,n){n=xi(-1,n),n.tag=3;var i=t.type.getDerivedStateFromError;if(typeof i=="function"){var r=e.value;n.payload=function(){return i(r)},n.callback=function(){yd(t,e)}}var o=t.stateNode;return o!==null&&typeof o.componentDidCatch=="function"&&(n.callback=function(){yd(t,e),typeof i!="function"&&(Qi===null?Qi=new Set([this]):Qi.add(this));var s=e.stack;this.componentDidCatch(e.value,{componentStack:s!==null?s:""})}),n}function Ip(t,e,n){var i=t.pingCache;if(i===null){i=t.pingCache=new Zx;var r=new Set;i.set(e,r)}else r=i.get(e),r===void 0&&(r=new Set,i.set(e,r));r.has(n)||(r.add(n),t=dS.bind(null,t,e,n),e.then(t,t))}function kp(t){do{var e;if((e=t.tag===13)&&(e=t.memoizedState,e=e!==null?e.dehydrated!==null:!0),e)return t;t=t.return}while(t!==null);return null}function Up(t,e,n,i,r){return t.mode&1?(t.flags|=65536,t.lanes=r,t):(t===e?t.flags|=65536:(t.flags|=128,n.flags|=131072,n.flags&=-52805,n.tag===1&&(n.alternate===null?n.tag=17:(e=xi(-1,1),e.tag=2,Ji(n,e,1))),n.lanes|=1),t)}var Jx=Ai.ReactCurrentOwner,an=!1;function $t(t,e,n,i){e.child=t===null?I0(e,null,n,i):Fo(e,t.child,n,i)}function Fp(t,e,n,i,r){n=n.render;var o=e.ref;return bo(e,r),i=tf(t,e,n,i,o,r),n=nf(),t!==null&&!an?(e.updateQueue=t.updateQueue,e.flags&=-2053,t.lanes&=~r,Ti(t,e,r)):(mt&&n&&Gh(e),e.flags|=1,$t(t,e,i,r),e.child)}function Op(t,e,n,i,r){if(t===null){var o=n.type;return typeof o=="function"&&!ff(o)&&o.defaultProps===void 0&&n.compare===null&&n.defaultProps===void 0?(e.tag=15,e.type=o,sv(t,e,o,i,r)):(t=xl(n.type,null,i,e,e.mode,r),t.ref=e.ref,t.return=e,e.child=t)}if(o=t.child,!(t.lanes&r)){var s=o.memoizedProps;if(n=n.compare,n=n!==null?n:Zs,n(s,i)&&t.ref===e.ref)return Ti(t,e,r)}return e.flags|=1,t=tr(o,i),t.ref=e.ref,t.return=e,e.child=t}function sv(t,e,n,i,r){if(t!==null){var o=t.memoizedProps;if(Zs(o,i)&&t.ref===e.ref)if(an=!1,e.pendingProps=i=o,(t.lanes&r)!==0)t.flags&131072&&(an=!0);else return e.lanes=t.lanes,Ti(t,e,r)}return _d(t,e,n,i,r)}function av(t,e,n){var i=e.pendingProps,r=i.children,o=t!==null?t.memoizedState:null;if(i.mode==="hidden")if(!(e.mode&1))e.memoizedState={baseLanes:0,cachePool:null,transitions:null},lt(xo,mn),mn|=n;else{if(!(n&1073741824))return t=o!==null?o.baseLanes|n:n,e.lanes=e.childLanes=1073741824,e.memoizedState={baseLanes:t,cachePool:null,transitions:null},e.updateQueue=null,lt(xo,mn),mn|=t,null;e.memoizedState={baseLanes:0,cachePool:null,transitions:null},i=o!==null?o.baseLanes:n,lt(xo,mn),mn|=i}else o!==null?(i=o.baseLanes|n,e.memoizedState=null):i=n,lt(xo,mn),mn|=i;return $t(t,e,r,n),e.child}function lv(t,e){var n=e.ref;(t===null&&n!==null||t!==null&&t.ref!==n)&&(e.flags|=512,e.flags|=2097152)}function _d(t,e,n,i,r){var o=cn(n)?Lr:jt.current;return o=ko(e,o),bo(e,r),n=tf(t,e,n,i,o,r),i=nf(),t!==null&&!an?(e.updateQueue=t.updateQueue,e.flags&=-2053,t.lanes&=~r,Ti(t,e,r)):(mt&&i&&Gh(e),e.flags|=1,$t(t,e,n,r),e.child)}function zp(t,e,n,i,r){if(cn(n)){var o=!0;Ol(e)}else o=!1;if(bo(e,r),e.stateNode===null)vl(t,e),iv(e,n,i),vd(e,n,i,r),i=!0;else if(t===null){var s=e.stateNode,a=e.memoizedProps;s.props=a;var l=s.context,c=n.contextType;typeof c=="object"&&c!==null?c=Ln(c):(c=cn(n)?Lr:jt.current,c=ko(e,c));var d=n.getDerivedStateFromProps,f=typeof d=="function"||typeof s.getSnapshotBeforeUpdate=="function";f||typeof s.UNSAFE_componentWillReceiveProps!="function"&&typeof s.componentWillReceiveProps!="function"||(a!==i||l!==c)&&Np(e,s,i,c),Oi=!1;var h=e.memoizedState;s.state=h,Gl(e,i,s,r),l=e.memoizedState,a!==i||h!==l||ln.current||Oi?(typeof d=="function"&&(gd(e,n,d,i),l=e.memoizedState),(a=Oi||Lp(e,n,a,i,h,l,c))?(f||typeof s.UNSAFE_componentWillMount!="function"&&typeof s.componentWillMount!="function"||(typeof s.componentWillMount=="function"&&s.componentWillMount(),typeof s.UNSAFE_componentWillMount=="function"&&s.UNSAFE_componentWillMount()),typeof s.componentDidMount=="function"&&(e.flags|=4194308)):(typeof s.componentDidMount=="function"&&(e.flags|=4194308),e.memoizedProps=i,e.memoizedState=l),s.props=i,s.state=l,s.context=c,i=a):(typeof s.componentDidMount=="function"&&(e.flags|=4194308),i=!1)}else{s=e.stateNode,U0(t,e),a=e.memoizedProps,c=e.type===e.elementType?a:Hn(e.type,a),s.props=c,f=e.pendingProps,h=s.context,l=n.contextType,typeof l=="object"&&l!==null?l=Ln(l):(l=cn(n)?Lr:jt.current,l=ko(e,l));var p=n.getDerivedStateFromProps;(d=typeof p=="function"||typeof s.getSnapshotBeforeUpdate=="function")||typeof s.UNSAFE_componentWillReceiveProps!="function"&&typeof s.componentWillReceiveProps!="function"||(a!==f||h!==l)&&Np(e,s,i,l),Oi=!1,h=e.memoizedState,s.state=h,Gl(e,i,s,r);var v=e.memoizedState;a!==f||h!==v||ln.current||Oi?(typeof p=="function"&&(gd(e,n,p,i),v=e.memoizedState),(c=Oi||Lp(e,n,c,i,h,v,l)||!1)?(d||typeof s.UNSAFE_componentWillUpdate!="function"&&typeof s.componentWillUpdate!="function"||(typeof s.componentWillUpdate=="function"&&s.componentWillUpdate(i,v,l),typeof s.UNSAFE_componentWillUpdate=="function"&&s.UNSAFE_componentWillUpdate(i,v,l)),typeof s.componentDidUpdate=="function"&&(e.flags|=4),typeof s.getSnapshotBeforeUpdate=="function"&&(e.flags|=1024)):(typeof s.componentDidUpdate!="function"||a===t.memoizedProps&&h===t.memoizedState||(e.flags|=4),typeof s.getSnapshotBeforeUpdate!="function"||a===t.memoizedProps&&h===t.memoizedState||(e.flags|=1024),e.memoizedProps=i,e.memoizedState=v),s.props=i,s.state=v,s.context=l,i=c):(typeof s.componentDidUpdate!="function"||a===t.memoizedProps&&h===t.memoizedState||(e.flags|=4),typeof s.getSnapshotBeforeUpdate!="function"||a===t.memoizedProps&&h===t.memoizedState||(e.flags|=1024),i=!1)}return xd(t,e,n,i,o,r)}function xd(t,e,n,i,r,o){lv(t,e);var s=(e.flags&128)!==0;if(!i&&!s)return r&&Ep(e,n,!1),Ti(t,e,o);i=e.stateNode,Jx.current=e;var a=s&&typeof n.getDerivedStateFromError!="function"?null:i.render();return e.flags|=1,t!==null&&s?(e.child=Fo(e,t.child,null,o),e.child=Fo(e,null,a,o)):$t(t,e,a,o),e.memoizedState=i.state,r&&Ep(e,n,!0),e.child}function cv(t){var e=t.stateNode;e.pendingContext?wp(t,e.pendingContext,e.pendingContext!==e.context):e.context&&wp(t,e.context,!1),Zh(t,e.containerInfo)}function Bp(t,e,n,i,r){return Uo(),Xh(r),e.flags|=256,$t(t,e,n,i),e.child}var Sd={dehydrated:null,treeContext:null,retryLane:0};function Md(t){return{baseLanes:t,cachePool:null,transitions:null}}function uv(t,e,n){var i=e.pendingProps,r=gt.current,o=!1,s=(e.flags&128)!==0,a;if((a=s)||(a=t!==null&&t.memoizedState===null?!1:(r&2)!==0),a?(o=!0,e.flags&=-129):(t===null||t.memoizedState!==null)&&(r|=1),lt(gt,r&1),t===null)return pd(e),t=e.memoizedState,t!==null&&(t=t.dehydrated,t!==null)?(e.mode&1?t.data==="$!"?e.lanes=8:e.lanes=1073741824:e.lanes=1,null):(s=i.children,t=i.fallback,o?(i=e.mode,o=e.child,s={mode:"hidden",children:s},!(i&1)&&o!==null?(o.childLanes=0,o.pendingProps=s):o=gc(s,i,0,null),t=Dr(t,i,n,null),o.return=e,t.return=e,o.sibling=t,e.child=o,e.child.memoizedState=Md(n),e.memoizedState=Sd,t):sf(e,s));if(r=t.memoizedState,r!==null&&(a=r.dehydrated,a!==null))return Qx(t,e,s,i,a,r,n);if(o){o=i.fallback,s=e.mode,r=t.child,a=r.sibling;var l={mode:"hidden",children:i.children};return!(s&1)&&e.child!==r?(i=e.child,i.childLanes=0,i.pendingProps=l,e.deletions=null):(i=tr(r,l),i.subtreeFlags=r.subtreeFlags&14680064),a!==null?o=tr(a,o):(o=Dr(o,s,n,null),o.flags|=2),o.return=e,i.return=e,i.sibling=o,e.child=i,i=o,o=e.child,s=t.child.memoizedState,s=s===null?Md(n):{baseLanes:s.baseLanes|n,cachePool:null,transitions:s.transitions},o.memoizedState=s,o.childLanes=t.childLanes&~n,e.memoizedState=Sd,i}return o=t.child,t=o.sibling,i=tr(o,{mode:"visible",children:i.children}),!(e.mode&1)&&(i.lanes=n),i.return=e,i.sibling=null,t!==null&&(n=e.deletions,n===null?(e.deletions=[t],e.flags|=16):n.push(t)),e.child=i,e.memoizedState=null,i}function sf(t,e){return e=gc({mode:"visible",children:e},t.mode,0,null),e.return=t,t.child=e}function ka(t,e,n,i){return i!==null&&Xh(i),Fo(e,t.child,null,n),t=sf(e,e.pendingProps.children),t.flags|=2,e.memoizedState=null,t}function Qx(t,e,n,i,r,o,s){if(n)return e.flags&256?(e.flags&=-257,i=Zc(Error(ie(422))),ka(t,e,s,i)):e.memoizedState!==null?(e.child=t.child,e.flags|=128,null):(o=i.fallback,r=e.mode,i=gc({mode:"visible",children:i.children},r,0,null),o=Dr(o,r,s,null),o.flags|=2,i.return=e,o.return=e,i.sibling=o,e.child=i,e.mode&1&&Fo(e,t.child,null,s),e.child.memoizedState=Md(s),e.memoizedState=Sd,o);if(!(e.mode&1))return ka(t,e,s,null);if(r.data==="$!"){if(i=r.nextSibling&&r.nextSibling.dataset,i)var a=i.dgst;return i=a,o=Error(ie(419)),i=Zc(o,i,void 0),ka(t,e,s,i)}if(a=(s&t.childLanes)!==0,an||a){if(i=Lt,i!==null){switch(s&-s){case 4:r=2;break;case 16:r=8;break;case 64:case 128:case 256:case 512:case 1024:case 2048:case 4096:case 8192:case 16384:case 32768:case 65536:case 131072:case 262144:case 524288:case 1048576:case 2097152:case 4194304:case 8388608:case 16777216:case 33554432:case 67108864:r=32;break;case 536870912:r=268435456;break;default:r=0}r=r&(i.suspendedLanes|s)?0:r,r!==0&&r!==o.retryLane&&(o.retryLane=r,Ei(t,r),jn(i,t,r,-1))}return hf(),i=Zc(Error(ie(421))),ka(t,e,s,i)}return r.data==="$?"?(e.flags|=128,e.child=t.child,e=hS.bind(null,t),r._reactRetry=e,null):(t=o.treeContext,vn=Zi(r.nextSibling),yn=e,mt=!0,Gn=null,t!==null&&(Tn[An++]=vi,Tn[An++]=yi,Tn[An++]=Nr,vi=t.id,yi=t.overflow,Nr=e),e=sf(e,i.children),e.flags|=4096,e)}function Hp(t,e,n){t.lanes|=e;var i=t.alternate;i!==null&&(i.lanes|=e),md(t.return,e,n)}function Jc(t,e,n,i,r){var o=t.memoizedState;o===null?t.memoizedState={isBackwards:e,rendering:null,renderingStartTime:0,last:i,tail:n,tailMode:r}:(o.isBackwards=e,o.rendering=null,o.renderingStartTime=0,o.last=i,o.tail=n,o.tailMode=r)}function dv(t,e,n){var i=e.pendingProps,r=i.revealOrder,o=i.tail;if($t(t,e,i.children,n),i=gt.current,i&2)i=i&1|2,e.flags|=128;else{if(t!==null&&t.flags&128)e:for(t=e.child;t!==null;){if(t.tag===13)t.memoizedState!==null&&Hp(t,n,e);else if(t.tag===19)Hp(t,n,e);else if(t.child!==null){t.child.return=t,t=t.child;continue}if(t===e)break e;for(;t.sibling===null;){if(t.return===null||t.return===e)break e;t=t.return}t.sibling.return=t.return,t=t.sibling}i&=1}if(lt(gt,i),!(e.mode&1))e.memoizedState=null;else switch(r){case"forwards":for(n=e.child,r=null;n!==null;)t=n.alternate,t!==null&&Wl(t)===null&&(r=n),n=n.sibling;n=r,n===null?(r=e.child,e.child=null):(r=n.sibling,n.sibling=null),Jc(e,!1,r,n,o);break;case"backwards":for(n=null,r=e.child,e.child=null;r!==null;){if(t=r.alternate,t!==null&&Wl(t)===null){e.child=r;break}t=r.sibling,r.sibling=n,n=r,r=t}Jc(e,!0,n,null,o);break;case"together":Jc(e,!1,null,null,void 0);break;default:e.memoizedState=null}return e.child}function vl(t,e){!(e.mode&1)&&t!==null&&(t.alternate=null,e.alternate=null,e.flags|=2)}function Ti(t,e,n){if(t!==null&&(e.dependencies=t.dependencies),kr|=e.lanes,!(n&e.childLanes))return null;if(t!==null&&e.child!==t.child)throw Error(ie(153));if(e.child!==null){for(t=e.child,n=tr(t,t.pendingProps),e.child=n,n.return=e;t.sibling!==null;)t=t.sibling,n=n.sibling=tr(t,t.pendingProps),n.return=e;n.sibling=null}return e.child}function eS(t,e,n){switch(e.tag){case 3:cv(e),Uo();break;case 5:F0(e);break;case 1:cn(e.type)&&Ol(e);break;case 4:Zh(e,e.stateNode.containerInfo);break;case 10:var i=e.type._context,r=e.memoizedProps.value;lt(Hl,i._currentValue),i._currentValue=r;break;case 13:if(i=e.memoizedState,i!==null)return i.dehydrated!==null?(lt(gt,gt.current&1),e.flags|=128,null):n&e.child.childLanes?uv(t,e,n):(lt(gt,gt.current&1),t=Ti(t,e,n),t!==null?t.sibling:null);lt(gt,gt.current&1);break;case 19:if(i=(n&e.childLanes)!==0,t.flags&128){if(i)return dv(t,e,n);e.flags|=128}if(r=e.memoizedState,r!==null&&(r.rendering=null,r.tail=null,r.lastEffect=null),lt(gt,gt.current),i)break;return null;case 22:case 23:return e.lanes=0,av(t,e,n)}return Ti(t,e,n)}var hv,wd,fv,pv;hv=function(t,e){for(var n=e.child;n!==null;){if(n.tag===5||n.tag===6)t.appendChild(n.stateNode);else if(n.tag!==4&&n.child!==null){n.child.return=n,n=n.child;continue}if(n===e)break;for(;n.sibling===null;){if(n.return===null||n.return===e)return;n=n.return}n.sibling.return=n.return,n=n.sibling}};wd=function(){};fv=function(t,e,n,i){var r=t.memoizedProps;if(r!==i){t=e.stateNode,Ar(ri.current);var o=null;switch(n){case"input":r=Xu(t,r),i=Xu(t,i),o=[];break;case"select":r=yt({},r,{value:void 0}),i=yt({},i,{value:void 0}),o=[];break;case"textarea":r=qu(t,r),i=qu(t,i),o=[];break;default:typeof r.onClick!="function"&&typeof i.onClick=="function"&&(t.onclick=Ul)}Ku(n,i);var s;n=null;for(c in r)if(!i.hasOwnProperty(c)&&r.hasOwnProperty(c)&&r[c]!=null)if(c==="style"){var a=r[c];for(s in a)a.hasOwnProperty(s)&&(n||(n={}),n[s]="")}else c!=="dangerouslySetInnerHTML"&&c!=="children"&&c!=="suppressContentEditableWarning"&&c!=="suppressHydrationWarning"&&c!=="autoFocus"&&(Ws.hasOwnProperty(c)?o||(o=[]):(o=o||[]).push(c,null));for(c in i){var l=i[c];if(a=r!=null?r[c]:void 0,i.hasOwnProperty(c)&&l!==a&&(l!=null||a!=null))if(c==="style")if(a){for(s in a)!a.hasOwnProperty(s)||l&&l.hasOwnProperty(s)||(n||(n={}),n[s]="");for(s in l)l.hasOwnProperty(s)&&a[s]!==l[s]&&(n||(n={}),n[s]=l[s])}else n||(o||(o=[]),o.push(c,n)),n=l;else c==="dangerouslySetInnerHTML"?(l=l?l.__html:void 0,a=a?a.__html:void 0,l!=null&&a!==l&&(o=o||[]).push(c,l)):c==="children"?typeof l!="string"&&typeof l!="number"||(o=o||[]).push(c,""+l):c!=="suppressContentEditableWarning"&&c!=="suppressHydrationWarning"&&(Ws.hasOwnProperty(c)?(l!=null&&c==="onScroll"&&ht("scroll",t),o||a===l||(o=[])):(o=o||[]).push(c,l))}n&&(o=o||[]).push("style",n);var c=o;(e.updateQueue=c)&&(e.flags|=4)}};pv=function(t,e,n,i){n!==i&&(e.flags|=4)};function ds(t,e){if(!mt)switch(t.tailMode){case"hidden":e=t.tail;for(var n=null;e!==null;)e.alternate!==null&&(n=e),e=e.sibling;n===null?t.tail=null:n.sibling=null;break;case"collapsed":n=t.tail;for(var i=null;n!==null;)n.alternate!==null&&(i=n),n=n.sibling;i===null?e||t.tail===null?t.tail=null:t.tail.sibling=null:i.sibling=null}}function zt(t){var e=t.alternate!==null&&t.alternate.child===t.child,n=0,i=0;if(e)for(var r=t.child;r!==null;)n|=r.lanes|r.childLanes,i|=r.subtreeFlags&14680064,i|=r.flags&14680064,r.return=t,r=r.sibling;else for(r=t.child;r!==null;)n|=r.lanes|r.childLanes,i|=r.subtreeFlags,i|=r.flags,r.return=t,r=r.sibling;return t.subtreeFlags|=i,t.childLanes=n,e}function tS(t,e,n){var i=e.pendingProps;switch(Wh(e),e.tag){case 2:case 16:case 15:case 0:case 11:case 7:case 8:case 12:case 9:case 14:return zt(e),null;case 1:return cn(e.type)&&Fl(),zt(e),null;case 3:return i=e.stateNode,Oo(),pt(ln),pt(jt),Qh(),i.pendingContext&&(i.context=i.pendingContext,i.pendingContext=null),(t===null||t.child===null)&&(Na(e)?e.flags|=4:t===null||t.memoizedState.isDehydrated&&!(e.flags&256)||(e.flags|=1024,Gn!==null&&(Dd(Gn),Gn=null))),wd(t,e),zt(e),null;case 5:Jh(e);var r=Ar(na.current);if(n=e.type,t!==null&&e.stateNode!=null)fv(t,e,n,i,r),t.ref!==e.ref&&(e.flags|=512,e.flags|=2097152);else{if(!i){if(e.stateNode===null)throw Error(ie(166));return zt(e),null}if(t=Ar(ri.current),Na(e)){i=e.stateNode,n=e.type;var o=e.memoizedProps;switch(i[ei]=e,i[ea]=o,t=(e.mode&1)!==0,n){case"dialog":ht("cancel",i),ht("close",i);break;case"iframe":case"object":case"embed":ht("load",i);break;case"video":case"audio":for(r=0;r<bs.length;r++)ht(bs[r],i);break;case"source":ht("error",i);break;case"img":case"image":case"link":ht("error",i),ht("load",i);break;case"details":ht("toggle",i);break;case"input":Kf(i,o),ht("invalid",i);break;case"select":i._wrapperState={wasMultiple:!!o.multiple},ht("invalid",i);break;case"textarea":Jf(i,o),ht("invalid",i)}Ku(n,o),r=null;for(var s in o)if(o.hasOwnProperty(s)){var a=o[s];s==="children"?typeof a=="string"?i.textContent!==a&&(o.suppressHydrationWarning!==!0&&La(i.textContent,a,t),r=["children",a]):typeof a=="number"&&i.textContent!==""+a&&(o.suppressHydrationWarning!==!0&&La(i.textContent,a,t),r=["children",""+a]):Ws.hasOwnProperty(s)&&a!=null&&s==="onScroll"&&ht("scroll",i)}switch(n){case"input":Ea(i),Zf(i,o,!0);break;case"textarea":Ea(i),Qf(i);break;case"select":case"option":break;default:typeof o.onClick=="function"&&(i.onclick=Ul)}i=r,e.updateQueue=i,i!==null&&(e.flags|=4)}else{s=r.nodeType===9?r:r.ownerDocument,t==="http://www.w3.org/1999/xhtml"&&(t=Vg(n)),t==="http://www.w3.org/1999/xhtml"?n==="script"?(t=s.createElement("div"),t.innerHTML="<script><\/script>",t=t.removeChild(t.firstChild)):typeof i.is=="string"?t=s.createElement(n,{is:i.is}):(t=s.createElement(n),n==="select"&&(s=t,i.multiple?s.multiple=!0:i.size&&(s.size=i.size))):t=s.createElementNS(t,n),t[ei]=e,t[ea]=i,hv(t,e,!1,!1),e.stateNode=t;e:{switch(s=Zu(n,i),n){case"dialog":ht("cancel",t),ht("close",t),r=i;break;case"iframe":case"object":case"embed":ht("load",t),r=i;break;case"video":case"audio":for(r=0;r<bs.length;r++)ht(bs[r],t);r=i;break;case"source":ht("error",t),r=i;break;case"img":case"image":case"link":ht("error",t),ht("load",t),r=i;break;case"details":ht("toggle",t),r=i;break;case"input":Kf(t,i),r=Xu(t,i),ht("invalid",t);break;case"option":r=i;break;case"select":t._wrapperState={wasMultiple:!!i.multiple},r=yt({},i,{value:void 0}),ht("invalid",t);break;case"textarea":Jf(t,i),r=qu(t,i),ht("invalid",t);break;default:r=i}Ku(n,r),a=r;for(o in a)if(a.hasOwnProperty(o)){var l=a[o];o==="style"?Xg(t,l):o==="dangerouslySetInnerHTML"?(l=l?l.__html:void 0,l!=null&&Gg(t,l)):o==="children"?typeof l=="string"?(n!=="textarea"||l!=="")&&Xs(t,l):typeof l=="number"&&Xs(t,""+l):o!=="suppressContentEditableWarning"&&o!=="suppressHydrationWarning"&&o!=="autoFocus"&&(Ws.hasOwnProperty(o)?l!=null&&o==="onScroll"&&ht("scroll",t):l!=null&&Rh(t,o,l,s))}switch(n){case"input":Ea(t),Zf(t,i,!1);break;case"textarea":Ea(t),Qf(t);break;case"option":i.value!=null&&t.setAttribute("value",""+rr(i.value));break;case"select":t.multiple=!!i.multiple,o=i.value,o!=null?Eo(t,!!i.multiple,o,!1):i.defaultValue!=null&&Eo(t,!!i.multiple,i.defaultValue,!0);break;default:typeof r.onClick=="function"&&(t.onclick=Ul)}switch(n){case"button":case"input":case"select":case"textarea":i=!!i.autoFocus;break e;case"img":i=!0;break e;default:i=!1}}i&&(e.flags|=4)}e.ref!==null&&(e.flags|=512,e.flags|=2097152)}return zt(e),null;case 6:if(t&&e.stateNode!=null)pv(t,e,t.memoizedProps,i);else{if(typeof i!="string"&&e.stateNode===null)throw Error(ie(166));if(n=Ar(na.current),Ar(ri.current),Na(e)){if(i=e.stateNode,n=e.memoizedProps,i[ei]=e,(o=i.nodeValue!==n)&&(t=yn,t!==null))switch(t.tag){case 3:La(i.nodeValue,n,(t.mode&1)!==0);break;case 5:t.memoizedProps.suppressHydrationWarning!==!0&&La(i.nodeValue,n,(t.mode&1)!==0)}o&&(e.flags|=4)}else i=(n.nodeType===9?n:n.ownerDocument).createTextNode(i),i[ei]=e,e.stateNode=i}return zt(e),null;case 13:if(pt(gt),i=e.memoizedState,t===null||t.memoizedState!==null&&t.memoizedState.dehydrated!==null){if(mt&&vn!==null&&e.mode&1&&!(e.flags&128))L0(),Uo(),e.flags|=98560,o=!1;else if(o=Na(e),i!==null&&i.dehydrated!==null){if(t===null){if(!o)throw Error(ie(318));if(o=e.memoizedState,o=o!==null?o.dehydrated:null,!o)throw Error(ie(317));o[ei]=e}else Uo(),!(e.flags&128)&&(e.memoizedState=null),e.flags|=4;zt(e),o=!1}else Gn!==null&&(Dd(Gn),Gn=null),o=!0;if(!o)return e.flags&65536?e:null}return e.flags&128?(e.lanes=n,e):(i=i!==null,i!==(t!==null&&t.memoizedState!==null)&&i&&(e.child.flags|=8192,e.mode&1&&(t===null||gt.current&1?At===0&&(At=3):hf())),e.updateQueue!==null&&(e.flags|=4),zt(e),null);case 4:return Oo(),wd(t,e),t===null&&Js(e.stateNode.containerInfo),zt(e),null;case 10:return qh(e.type._context),zt(e),null;case 17:return cn(e.type)&&Fl(),zt(e),null;case 19:if(pt(gt),o=e.memoizedState,o===null)return zt(e),null;if(i=(e.flags&128)!==0,s=o.rendering,s===null)if(i)ds(o,!1);else{if(At!==0||t!==null&&t.flags&128)for(t=e.child;t!==null;){if(s=Wl(t),s!==null){for(e.flags|=128,ds(o,!1),i=s.updateQueue,i!==null&&(e.updateQueue=i,e.flags|=4),e.subtreeFlags=0,i=n,n=e.child;n!==null;)o=n,t=i,o.flags&=14680066,s=o.alternate,s===null?(o.childLanes=0,o.lanes=t,o.child=null,o.subtreeFlags=0,o.memoizedProps=null,o.memoizedState=null,o.updateQueue=null,o.dependencies=null,o.stateNode=null):(o.childLanes=s.childLanes,o.lanes=s.lanes,o.child=s.child,o.subtreeFlags=0,o.deletions=null,o.memoizedProps=s.memoizedProps,o.memoizedState=s.memoizedState,o.updateQueue=s.updateQueue,o.type=s.type,t=s.dependencies,o.dependencies=t===null?null:{lanes:t.lanes,firstContext:t.firstContext}),n=n.sibling;return lt(gt,gt.current&1|2),e.child}t=t.sibling}o.tail!==null&&Mt()>Bo&&(e.flags|=128,i=!0,ds(o,!1),e.lanes=4194304)}else{if(!i)if(t=Wl(s),t!==null){if(e.flags|=128,i=!0,n=t.updateQueue,n!==null&&(e.updateQueue=n,e.flags|=4),ds(o,!0),o.tail===null&&o.tailMode==="hidden"&&!s.alternate&&!mt)return zt(e),null}else 2*Mt()-o.renderingStartTime>Bo&&n!==1073741824&&(e.flags|=128,i=!0,ds(o,!1),e.lanes=4194304);o.isBackwards?(s.sibling=e.child,e.child=s):(n=o.last,n!==null?n.sibling=s:e.child=s,o.last=s)}return o.tail!==null?(e=o.tail,o.rendering=e,o.tail=e.sibling,o.renderingStartTime=Mt(),e.sibling=null,n=gt.current,lt(gt,i?n&1|2:n&1),e):(zt(e),null);case 22:case 23:return df(),i=e.memoizedState!==null,t!==null&&t.memoizedState!==null!==i&&(e.flags|=8192),i&&e.mode&1?mn&1073741824&&(zt(e),e.subtreeFlags&6&&(e.flags|=8192)):zt(e),null;case 24:return null;case 25:return null}throw Error(ie(156,e.tag))}function nS(t,e){switch(Wh(e),e.tag){case 1:return cn(e.type)&&Fl(),t=e.flags,t&65536?(e.flags=t&-65537|128,e):null;case 3:return Oo(),pt(ln),pt(jt),Qh(),t=e.flags,t&65536&&!(t&128)?(e.flags=t&-65537|128,e):null;case 5:return Jh(e),null;case 13:if(pt(gt),t=e.memoizedState,t!==null&&t.dehydrated!==null){if(e.alternate===null)throw Error(ie(340));Uo()}return t=e.flags,t&65536?(e.flags=t&-65537|128,e):null;case 19:return pt(gt),null;case 4:return Oo(),null;case 10:return qh(e.type._context),null;case 22:case 23:return df(),null;case 24:return null;default:return null}}var Ua=!1,Vt=!1,iS=typeof WeakSet=="function"?WeakSet:Set,ge=null;function _o(t,e){var n=t.ref;if(n!==null)if(typeof n=="function")try{n(null)}catch(i){xt(t,e,i)}else n.current=null}function Ed(t,e,n){try{n()}catch(i){xt(t,e,i)}}var Vp=!1;function rS(t,e){if(ad=Nl,t=_0(),Vh(t)){if("selectionStart"in t)var n={start:t.selectionStart,end:t.selectionEnd};else e:{n=(n=t.ownerDocument)&&n.defaultView||window;var i=n.getSelection&&n.getSelection();if(i&&i.rangeCount!==0){n=i.anchorNode;var r=i.anchorOffset,o=i.focusNode;i=i.focusOffset;try{n.nodeType,o.nodeType}catch{n=null;break e}var s=0,a=-1,l=-1,c=0,d=0,f=t,h=null;t:for(;;){for(var p;f!==n||r!==0&&f.nodeType!==3||(a=s+r),f!==o||i!==0&&f.nodeType!==3||(l=s+i),f.nodeType===3&&(s+=f.nodeValue.length),(p=f.firstChild)!==null;)h=f,f=p;for(;;){if(f===t)break t;if(h===n&&++c===r&&(a=s),h===o&&++d===i&&(l=s),(p=f.nextSibling)!==null)break;f=h,h=f.parentNode}f=p}n=a===-1||l===-1?null:{start:a,end:l}}else n=null}n=n||{start:0,end:0}}else n=null;for(ld={focusedElem:t,selectionRange:n},Nl=!1,ge=e;ge!==null;)if(e=ge,t=e.child,(e.subtreeFlags&1028)!==0&&t!==null)t.return=e,ge=t;else for(;ge!==null;){e=ge;try{var v=e.alternate;if(e.flags&1024)switch(e.tag){case 0:case 11:case 15:break;case 1:if(v!==null){var _=v.memoizedProps,m=v.memoizedState,u=e.stateNode,g=u.getSnapshotBeforeUpdate(e.elementType===e.type?_:Hn(e.type,_),m);u.__reactInternalSnapshotBeforeUpdate=g}break;case 3:var y=e.stateNode.containerInfo;y.nodeType===1?y.textContent="":y.nodeType===9&&y.documentElement&&y.removeChild(y.documentElement);break;case 5:case 6:case 4:case 17:break;default:throw Error(ie(163))}}catch(x){xt(e,e.return,x)}if(t=e.sibling,t!==null){t.return=e.return,ge=t;break}ge=e.return}return v=Vp,Vp=!1,v}function Fs(t,e,n){var i=e.updateQueue;if(i=i!==null?i.lastEffect:null,i!==null){var r=i=i.next;do{if((r.tag&t)===t){var o=r.destroy;r.destroy=void 0,o!==void 0&&Ed(e,n,o)}r=r.next}while(r!==i)}}function pc(t,e){if(e=e.updateQueue,e=e!==null?e.lastEffect:null,e!==null){var n=e=e.next;do{if((n.tag&t)===t){var i=n.create;n.destroy=i()}n=n.next}while(n!==e)}}function Td(t){var e=t.ref;if(e!==null){var n=t.stateNode;switch(t.tag){case 5:t=n;break;default:t=n}typeof e=="function"?e(t):e.current=t}}function mv(t){var e=t.alternate;e!==null&&(t.alternate=null,mv(e)),t.child=null,t.deletions=null,t.sibling=null,t.tag===5&&(e=t.stateNode,e!==null&&(delete e[ei],delete e[ea],delete e[dd],delete e[Bx],delete e[Hx])),t.stateNode=null,t.return=null,t.dependencies=null,t.memoizedProps=null,t.memoizedState=null,t.pendingProps=null,t.stateNode=null,t.updateQueue=null}function gv(t){return t.tag===5||t.tag===3||t.tag===4}function Gp(t){e:for(;;){for(;t.sibling===null;){if(t.return===null||gv(t.return))return null;t=t.return}for(t.sibling.return=t.return,t=t.sibling;t.tag!==5&&t.tag!==6&&t.tag!==18;){if(t.flags&2||t.child===null||t.tag===4)continue e;t.child.return=t,t=t.child}if(!(t.flags&2))return t.stateNode}}function Ad(t,e,n){var i=t.tag;if(i===5||i===6)t=t.stateNode,e?n.nodeType===8?n.parentNode.insertBefore(t,e):n.insertBefore(t,e):(n.nodeType===8?(e=n.parentNode,e.insertBefore(t,n)):(e=n,e.appendChild(t)),n=n._reactRootContainer,n!=null||e.onclick!==null||(e.onclick=Ul));else if(i!==4&&(t=t.child,t!==null))for(Ad(t,e,n),t=t.sibling;t!==null;)Ad(t,e,n),t=t.sibling}function Cd(t,e,n){var i=t.tag;if(i===5||i===6)t=t.stateNode,e?n.insertBefore(t,e):n.appendChild(t);else if(i!==4&&(t=t.child,t!==null))for(Cd(t,e,n),t=t.sibling;t!==null;)Cd(t,e,n),t=t.sibling}var It=null,Vn=!1;function Pi(t,e,n){for(n=n.child;n!==null;)vv(t,e,n),n=n.sibling}function vv(t,e,n){if(ii&&typeof ii.onCommitFiberUnmount=="function")try{ii.onCommitFiberUnmount(sc,n)}catch{}switch(n.tag){case 5:Vt||_o(n,e);case 6:var i=It,r=Vn;It=null,Pi(t,e,n),It=i,Vn=r,It!==null&&(Vn?(t=It,n=n.stateNode,t.nodeType===8?t.parentNode.removeChild(n):t.removeChild(n)):It.removeChild(n.stateNode));break;case 18:It!==null&&(Vn?(t=It,n=n.stateNode,t.nodeType===8?Xc(t.parentNode,n):t.nodeType===1&&Xc(t,n),$s(t)):Xc(It,n.stateNode));break;case 4:i=It,r=Vn,It=n.stateNode.containerInfo,Vn=!0,Pi(t,e,n),It=i,Vn=r;break;case 0:case 11:case 14:case 15:if(!Vt&&(i=n.updateQueue,i!==null&&(i=i.lastEffect,i!==null))){r=i=i.next;do{var o=r,s=o.destroy;o=o.tag,s!==void 0&&(o&2||o&4)&&Ed(n,e,s),r=r.next}while(r!==i)}Pi(t,e,n);break;case 1:if(!Vt&&(_o(n,e),i=n.stateNode,typeof i.componentWillUnmount=="function"))try{i.props=n.memoizedProps,i.state=n.memoizedState,i.componentWillUnmount()}catch(a){xt(n,e,a)}Pi(t,e,n);break;case 21:Pi(t,e,n);break;case 22:n.mode&1?(Vt=(i=Vt)||n.memoizedState!==null,Pi(t,e,n),Vt=i):Pi(t,e,n);break;default:Pi(t,e,n)}}function Wp(t){var e=t.updateQueue;if(e!==null){t.updateQueue=null;var n=t.stateNode;n===null&&(n=t.stateNode=new iS),e.forEach(function(i){var r=fS.bind(null,t,i);n.has(i)||(n.add(i),i.then(r,r))})}}function Fn(t,e){var n=e.deletions;if(n!==null)for(var i=0;i<n.length;i++){var r=n[i];try{var o=t,s=e,a=s;e:for(;a!==null;){switch(a.tag){case 5:It=a.stateNode,Vn=!1;break e;case 3:It=a.stateNode.containerInfo,Vn=!0;break e;case 4:It=a.stateNode.containerInfo,Vn=!0;break e}a=a.return}if(It===null)throw Error(ie(160));vv(o,s,r),It=null,Vn=!1;var l=r.alternate;l!==null&&(l.return=null),r.return=null}catch(c){xt(r,e,c)}}if(e.subtreeFlags&12854)for(e=e.child;e!==null;)yv(e,t),e=e.sibling}function yv(t,e){var n=t.alternate,i=t.flags;switch(t.tag){case 0:case 11:case 14:case 15:if(Fn(e,t),Zn(t),i&4){try{Fs(3,t,t.return),pc(3,t)}catch(_){xt(t,t.return,_)}try{Fs(5,t,t.return)}catch(_){xt(t,t.return,_)}}break;case 1:Fn(e,t),Zn(t),i&512&&n!==null&&_o(n,n.return);break;case 5:if(Fn(e,t),Zn(t),i&512&&n!==null&&_o(n,n.return),t.flags&32){var r=t.stateNode;try{Xs(r,"")}catch(_){xt(t,t.return,_)}}if(i&4&&(r=t.stateNode,r!=null)){var o=t.memoizedProps,s=n!==null?n.memoizedProps:o,a=t.type,l=t.updateQueue;if(t.updateQueue=null,l!==null)try{a==="input"&&o.type==="radio"&&o.name!=null&&Bg(r,o),Zu(a,s);var c=Zu(a,o);for(s=0;s<l.length;s+=2){var d=l[s],f=l[s+1];d==="style"?Xg(r,f):d==="dangerouslySetInnerHTML"?Gg(r,f):d==="children"?Xs(r,f):Rh(r,d,f,c)}switch(a){case"input":ju(r,o);break;case"textarea":Hg(r,o);break;case"select":var h=r._wrapperState.wasMultiple;r._wrapperState.wasMultiple=!!o.multiple;var p=o.value;p!=null?Eo(r,!!o.multiple,p,!1):h!==!!o.multiple&&(o.defaultValue!=null?Eo(r,!!o.multiple,o.defaultValue,!0):Eo(r,!!o.multiple,o.multiple?[]:"",!1))}r[ea]=o}catch(_){xt(t,t.return,_)}}break;case 6:if(Fn(e,t),Zn(t),i&4){if(t.stateNode===null)throw Error(ie(162));r=t.stateNode,o=t.memoizedProps;try{r.nodeValue=o}catch(_){xt(t,t.return,_)}}break;case 3:if(Fn(e,t),Zn(t),i&4&&n!==null&&n.memoizedState.isDehydrated)try{$s(e.containerInfo)}catch(_){xt(t,t.return,_)}break;case 4:Fn(e,t),Zn(t);break;case 13:Fn(e,t),Zn(t),r=t.child,r.flags&8192&&(o=r.memoizedState!==null,r.stateNode.isHidden=o,!o||r.alternate!==null&&r.alternate.memoizedState!==null||(cf=Mt())),i&4&&Wp(t);break;case 22:if(d=n!==null&&n.memoizedState!==null,t.mode&1?(Vt=(c=Vt)||d,Fn(e,t),Vt=c):Fn(e,t),Zn(t),i&8192){if(c=t.memoizedState!==null,(t.stateNode.isHidden=c)&&!d&&t.mode&1)for(ge=t,d=t.child;d!==null;){for(f=ge=d;ge!==null;){switch(h=ge,p=h.child,h.tag){case 0:case 11:case 14:case 15:Fs(4,h,h.return);break;case 1:_o(h,h.return);var v=h.stateNode;if(typeof v.componentWillUnmount=="function"){i=h,n=h.return;try{e=i,v.props=e.memoizedProps,v.state=e.memoizedState,v.componentWillUnmount()}catch(_){xt(i,n,_)}}break;case 5:_o(h,h.return);break;case 22:if(h.memoizedState!==null){jp(f);continue}}p!==null?(p.return=h,ge=p):jp(f)}d=d.sibling}e:for(d=null,f=t;;){if(f.tag===5){if(d===null){d=f;try{r=f.stateNode,c?(o=r.style,typeof o.setProperty=="function"?o.setProperty("display","none","important"):o.display="none"):(a=f.stateNode,l=f.memoizedProps.style,s=l!=null&&l.hasOwnProperty("display")?l.display:null,a.style.display=Wg("display",s))}catch(_){xt(t,t.return,_)}}}else if(f.tag===6){if(d===null)try{f.stateNode.nodeValue=c?"":f.memoizedProps}catch(_){xt(t,t.return,_)}}else if((f.tag!==22&&f.tag!==23||f.memoizedState===null||f===t)&&f.child!==null){f.child.return=f,f=f.child;continue}if(f===t)break e;for(;f.sibling===null;){if(f.return===null||f.return===t)break e;d===f&&(d=null),f=f.return}d===f&&(d=null),f.sibling.return=f.return,f=f.sibling}}break;case 19:Fn(e,t),Zn(t),i&4&&Wp(t);break;case 21:break;default:Fn(e,t),Zn(t)}}function Zn(t){var e=t.flags;if(e&2){try{e:{for(var n=t.return;n!==null;){if(gv(n)){var i=n;break e}n=n.return}throw Error(ie(160))}switch(i.tag){case 5:var r=i.stateNode;i.flags&32&&(Xs(r,""),i.flags&=-33);var o=Gp(t);Cd(t,o,r);break;case 3:case 4:var s=i.stateNode.containerInfo,a=Gp(t);Ad(t,a,s);break;default:throw Error(ie(161))}}catch(l){xt(t,t.return,l)}t.flags&=-3}e&4096&&(t.flags&=-4097)}function oS(t,e,n){ge=t,_v(t)}function _v(t,e,n){for(var i=(t.mode&1)!==0;ge!==null;){var r=ge,o=r.child;if(r.tag===22&&i){var s=r.memoizedState!==null||Ua;if(!s){var a=r.alternate,l=a!==null&&a.memoizedState!==null||Vt;a=Ua;var c=Vt;if(Ua=s,(Vt=l)&&!c)for(ge=r;ge!==null;)s=ge,l=s.child,s.tag===22&&s.memoizedState!==null?Yp(r):l!==null?(l.return=s,ge=l):Yp(r);for(;o!==null;)ge=o,_v(o),o=o.sibling;ge=r,Ua=a,Vt=c}Xp(t)}else r.subtreeFlags&8772&&o!==null?(o.return=r,ge=o):Xp(t)}}function Xp(t){for(;ge!==null;){var e=ge;if(e.flags&8772){var n=e.alternate;try{if(e.flags&8772)switch(e.tag){case 0:case 11:case 15:Vt||pc(5,e);break;case 1:var i=e.stateNode;if(e.flags&4&&!Vt)if(n===null)i.componentDidMount();else{var r=e.elementType===e.type?n.memoizedProps:Hn(e.type,n.memoizedProps);i.componentDidUpdate(r,n.memoizedState,i.__reactInternalSnapshotBeforeUpdate)}var o=e.updateQueue;o!==null&&Rp(e,o,i);break;case 3:var s=e.updateQueue;if(s!==null){if(n=null,e.child!==null)switch(e.child.tag){case 5:n=e.child.stateNode;break;case 1:n=e.child.stateNode}Rp(e,s,n)}break;case 5:var a=e.stateNode;if(n===null&&e.flags&4){n=a;var l=e.memoizedProps;switch(e.type){case"button":case"input":case"select":case"textarea":l.autoFocus&&n.focus();break;case"img":l.src&&(n.src=l.src)}}break;case 6:break;case 4:break;case 12:break;case 13:if(e.memoizedState===null){var c=e.alternate;if(c!==null){var d=c.memoizedState;if(d!==null){var f=d.dehydrated;f!==null&&$s(f)}}}break;case 19:case 17:case 21:case 22:case 23:case 25:break;default:throw Error(ie(163))}Vt||e.flags&512&&Td(e)}catch(h){xt(e,e.return,h)}}if(e===t){ge=null;break}if(n=e.sibling,n!==null){n.return=e.return,ge=n;break}ge=e.return}}function jp(t){for(;ge!==null;){var e=ge;if(e===t){ge=null;break}var n=e.sibling;if(n!==null){n.return=e.return,ge=n;break}ge=e.return}}function Yp(t){for(;ge!==null;){var e=ge;try{switch(e.tag){case 0:case 11:case 15:var n=e.return;try{pc(4,e)}catch(l){xt(e,n,l)}break;case 1:var i=e.stateNode;if(typeof i.componentDidMount=="function"){var r=e.return;try{i.componentDidMount()}catch(l){xt(e,r,l)}}var o=e.return;try{Td(e)}catch(l){xt(e,o,l)}break;case 5:var s=e.return;try{Td(e)}catch(l){xt(e,s,l)}}}catch(l){xt(e,e.return,l)}if(e===t){ge=null;break}var a=e.sibling;if(a!==null){a.return=e.return,ge=a;break}ge=e.return}}var sS=Math.ceil,Yl=Ai.ReactCurrentDispatcher,af=Ai.ReactCurrentOwner,Dn=Ai.ReactCurrentBatchConfig,Ke=0,Lt=null,Et=null,kt=0,mn=0,xo=lr(0),At=0,sa=null,kr=0,mc=0,lf=0,Os=null,sn=null,cf=0,Bo=1/0,pi=null,ql=!1,bd=null,Qi=null,Fa=!1,Xi=null,$l=0,zs=0,Rd=null,yl=-1,_l=0;function Zt(){return Ke&6?Mt():yl!==-1?yl:yl=Mt()}function er(t){return t.mode&1?Ke&2&&kt!==0?kt&-kt:Gx.transition!==null?(_l===0&&(_l=i0()),_l):(t=tt,t!==0||(t=window.event,t=t===void 0?16:u0(t.type)),t):1}function jn(t,e,n,i){if(50<zs)throw zs=0,Rd=null,Error(ie(185));ha(t,n,i),(!(Ke&2)||t!==Lt)&&(t===Lt&&(!(Ke&2)&&(mc|=n),At===4&&Bi(t,kt)),un(t,i),n===1&&Ke===0&&!(e.mode&1)&&(Bo=Mt()+500,dc&&cr()))}function un(t,e){var n=t.callbackNode;G_(t,e);var i=Ll(t,t===Lt?kt:0);if(i===0)n!==null&&np(n),t.callbackNode=null,t.callbackPriority=0;else if(e=i&-i,t.callbackPriority!==e){if(n!=null&&np(n),e===1)t.tag===0?Vx(qp.bind(null,t)):R0(qp.bind(null,t)),Ox(function(){!(Ke&6)&&cr()}),n=null;else{switch(r0(i)){case 1:n=Ih;break;case 4:n=t0;break;case 16:n=Dl;break;case 536870912:n=n0;break;default:n=Dl}n=Cv(n,xv.bind(null,t))}t.callbackPriority=e,t.callbackNode=n}}function xv(t,e){if(yl=-1,_l=0,Ke&6)throw Error(ie(327));var n=t.callbackNode;if(Ro()&&t.callbackNode!==n)return null;var i=Ll(t,t===Lt?kt:0);if(i===0)return null;if(i&30||i&t.expiredLanes||e)e=Kl(t,i);else{e=i;var r=Ke;Ke|=2;var o=Mv();(Lt!==t||kt!==e)&&(pi=null,Bo=Mt()+500,Pr(t,e));do try{cS();break}catch(a){Sv(t,a)}while(!0);Yh(),Yl.current=o,Ke=r,Et!==null?e=0:(Lt=null,kt=0,e=At)}if(e!==0){if(e===2&&(r=nd(t),r!==0&&(i=r,e=Pd(t,r))),e===1)throw n=sa,Pr(t,0),Bi(t,i),un(t,Mt()),n;if(e===6)Bi(t,i);else{if(r=t.current.alternate,!(i&30)&&!aS(r)&&(e=Kl(t,i),e===2&&(o=nd(t),o!==0&&(i=o,e=Pd(t,o))),e===1))throw n=sa,Pr(t,0),Bi(t,i),un(t,Mt()),n;switch(t.finishedWork=r,t.finishedLanes=i,e){case 0:case 1:throw Error(ie(345));case 2:_r(t,sn,pi);break;case 3:if(Bi(t,i),(i&130023424)===i&&(e=cf+500-Mt(),10<e)){if(Ll(t,0)!==0)break;if(r=t.suspendedLanes,(r&i)!==i){Zt(),t.pingedLanes|=t.suspendedLanes&r;break}t.timeoutHandle=ud(_r.bind(null,t,sn,pi),e);break}_r(t,sn,pi);break;case 4:if(Bi(t,i),(i&4194240)===i)break;for(e=t.eventTimes,r=-1;0<i;){var s=31-Xn(i);o=1<<s,s=e[s],s>r&&(r=s),i&=~o}if(i=r,i=Mt()-i,i=(120>i?120:480>i?480:1080>i?1080:1920>i?1920:3e3>i?3e3:4320>i?4320:1960*sS(i/1960))-i,10<i){t.timeoutHandle=ud(_r.bind(null,t,sn,pi),i);break}_r(t,sn,pi);break;case 5:_r(t,sn,pi);break;default:throw Error(ie(329))}}}return un(t,Mt()),t.callbackNode===n?xv.bind(null,t):null}function Pd(t,e){var n=Os;return t.current.memoizedState.isDehydrated&&(Pr(t,e).flags|=256),t=Kl(t,e),t!==2&&(e=sn,sn=n,e!==null&&Dd(e)),t}function Dd(t){sn===null?sn=t:sn.push.apply(sn,t)}function aS(t){for(var e=t;;){if(e.flags&16384){var n=e.updateQueue;if(n!==null&&(n=n.stores,n!==null))for(var i=0;i<n.length;i++){var r=n[i],o=r.getSnapshot;r=r.value;try{if(!Yn(o(),r))return!1}catch{return!1}}}if(n=e.child,e.subtreeFlags&16384&&n!==null)n.return=e,e=n;else{if(e===t)break;for(;e.sibling===null;){if(e.return===null||e.return===t)return!0;e=e.return}e.sibling.return=e.return,e=e.sibling}}return!0}function Bi(t,e){for(e&=~lf,e&=~mc,t.suspendedLanes|=e,t.pingedLanes&=~e,t=t.expirationTimes;0<e;){var n=31-Xn(e),i=1<<n;t[n]=-1,e&=~i}}function qp(t){if(Ke&6)throw Error(ie(327));Ro();var e=Ll(t,0);if(!(e&1))return un(t,Mt()),null;var n=Kl(t,e);if(t.tag!==0&&n===2){var i=nd(t);i!==0&&(e=i,n=Pd(t,i))}if(n===1)throw n=sa,Pr(t,0),Bi(t,e),un(t,Mt()),n;if(n===6)throw Error(ie(345));return t.finishedWork=t.current.alternate,t.finishedLanes=e,_r(t,sn,pi),un(t,Mt()),null}function uf(t,e){var n=Ke;Ke|=1;try{return t(e)}finally{Ke=n,Ke===0&&(Bo=Mt()+500,dc&&cr())}}function Ur(t){Xi!==null&&Xi.tag===0&&!(Ke&6)&&Ro();var e=Ke;Ke|=1;var n=Dn.transition,i=tt;try{if(Dn.transition=null,tt=1,t)return t()}finally{tt=i,Dn.transition=n,Ke=e,!(Ke&6)&&cr()}}function df(){mn=xo.current,pt(xo)}function Pr(t,e){t.finishedWork=null,t.finishedLanes=0;var n=t.timeoutHandle;if(n!==-1&&(t.timeoutHandle=-1,Fx(n)),Et!==null)for(n=Et.return;n!==null;){var i=n;switch(Wh(i),i.tag){case 1:i=i.type.childContextTypes,i!=null&&Fl();break;case 3:Oo(),pt(ln),pt(jt),Qh();break;case 5:Jh(i);break;case 4:Oo();break;case 13:pt(gt);break;case 19:pt(gt);break;case 10:qh(i.type._context);break;case 22:case 23:df()}n=n.return}if(Lt=t,Et=t=tr(t.current,null),kt=mn=e,At=0,sa=null,lf=mc=kr=0,sn=Os=null,Tr!==null){for(e=0;e<Tr.length;e++)if(n=Tr[e],i=n.interleaved,i!==null){n.interleaved=null;var r=i.next,o=n.pending;if(o!==null){var s=o.next;o.next=r,i.next=s}n.pending=i}Tr=null}return t}function Sv(t,e){do{var n=Et;try{if(Yh(),ml.current=jl,Xl){for(var i=vt.memoizedState;i!==null;){var r=i.queue;r!==null&&(r.pending=null),i=i.next}Xl=!1}if(Ir=0,Dt=Tt=vt=null,Us=!1,ia=0,af.current=null,n===null||n.return===null){At=1,sa=e,Et=null;break}e:{var o=t,s=n.return,a=n,l=e;if(e=kt,a.flags|=32768,l!==null&&typeof l=="object"&&typeof l.then=="function"){var c=l,d=a,f=d.tag;if(!(d.mode&1)&&(f===0||f===11||f===15)){var h=d.alternate;h?(d.updateQueue=h.updateQueue,d.memoizedState=h.memoizedState,d.lanes=h.lanes):(d.updateQueue=null,d.memoizedState=null)}var p=kp(s);if(p!==null){p.flags&=-257,Up(p,s,a,o,e),p.mode&1&&Ip(o,c,e),e=p,l=c;var v=e.updateQueue;if(v===null){var _=new Set;_.add(l),e.updateQueue=_}else v.add(l);break e}else{if(!(e&1)){Ip(o,c,e),hf();break e}l=Error(ie(426))}}else if(mt&&a.mode&1){var m=kp(s);if(m!==null){!(m.flags&65536)&&(m.flags|=256),Up(m,s,a,o,e),Xh(zo(l,a));break e}}o=l=zo(l,a),At!==4&&(At=2),Os===null?Os=[o]:Os.push(o),o=s;do{switch(o.tag){case 3:o.flags|=65536,e&=-e,o.lanes|=e;var u=rv(o,l,e);bp(o,u);break e;case 1:a=l;var g=o.type,y=o.stateNode;if(!(o.flags&128)&&(typeof g.getDerivedStateFromError=="function"||y!==null&&typeof y.componentDidCatch=="function"&&(Qi===null||!Qi.has(y)))){o.flags|=65536,e&=-e,o.lanes|=e;var x=ov(o,a,e);bp(o,x);break e}}o=o.return}while(o!==null)}Ev(n)}catch(C){e=C,Et===n&&n!==null&&(Et=n=n.return);continue}break}while(!0)}function Mv(){var t=Yl.current;return Yl.current=jl,t===null?jl:t}function hf(){(At===0||At===3||At===2)&&(At=4),Lt===null||!(kr&268435455)&&!(mc&268435455)||Bi(Lt,kt)}function Kl(t,e){var n=Ke;Ke|=2;var i=Mv();(Lt!==t||kt!==e)&&(pi=null,Pr(t,e));do try{lS();break}catch(r){Sv(t,r)}while(!0);if(Yh(),Ke=n,Yl.current=i,Et!==null)throw Error(ie(261));return Lt=null,kt=0,At}function lS(){for(;Et!==null;)wv(Et)}function cS(){for(;Et!==null&&!I_();)wv(Et)}function wv(t){var e=Av(t.alternate,t,mn);t.memoizedProps=t.pendingProps,e===null?Ev(t):Et=e,af.current=null}function Ev(t){var e=t;do{var n=e.alternate;if(t=e.return,e.flags&32768){if(n=nS(n,e),n!==null){n.flags&=32767,Et=n;return}if(t!==null)t.flags|=32768,t.subtreeFlags=0,t.deletions=null;else{At=6,Et=null;return}}else if(n=tS(n,e,mn),n!==null){Et=n;return}if(e=e.sibling,e!==null){Et=e;return}Et=e=t}while(e!==null);At===0&&(At=5)}function _r(t,e,n){var i=tt,r=Dn.transition;try{Dn.transition=null,tt=1,uS(t,e,n,i)}finally{Dn.transition=r,tt=i}return null}function uS(t,e,n,i){do Ro();while(Xi!==null);if(Ke&6)throw Error(ie(327));n=t.finishedWork;var r=t.finishedLanes;if(n===null)return null;if(t.finishedWork=null,t.finishedLanes=0,n===t.current)throw Error(ie(177));t.callbackNode=null,t.callbackPriority=0;var o=n.lanes|n.childLanes;if(W_(t,o),t===Lt&&(Et=Lt=null,kt=0),!(n.subtreeFlags&2064)&&!(n.flags&2064)||Fa||(Fa=!0,Cv(Dl,function(){return Ro(),null})),o=(n.flags&15990)!==0,n.subtreeFlags&15990||o){o=Dn.transition,Dn.transition=null;var s=tt;tt=1;var a=Ke;Ke|=4,af.current=null,rS(t,n),yv(n,t),Px(ld),Nl=!!ad,ld=ad=null,t.current=n,oS(n),k_(),Ke=a,tt=s,Dn.transition=o}else t.current=n;if(Fa&&(Fa=!1,Xi=t,$l=r),o=t.pendingLanes,o===0&&(Qi=null),O_(n.stateNode),un(t,Mt()),e!==null)for(i=t.onRecoverableError,n=0;n<e.length;n++)r=e[n],i(r.value,{componentStack:r.stack,digest:r.digest});if(ql)throw ql=!1,t=bd,bd=null,t;return $l&1&&t.tag!==0&&Ro(),o=t.pendingLanes,o&1?t===Rd?zs++:(zs=0,Rd=t):zs=0,cr(),null}function Ro(){if(Xi!==null){var t=r0($l),e=Dn.transition,n=tt;try{if(Dn.transition=null,tt=16>t?16:t,Xi===null)var i=!1;else{if(t=Xi,Xi=null,$l=0,Ke&6)throw Error(ie(331));var r=Ke;for(Ke|=4,ge=t.current;ge!==null;){var o=ge,s=o.child;if(ge.flags&16){var a=o.deletions;if(a!==null){for(var l=0;l<a.length;l++){var c=a[l];for(ge=c;ge!==null;){var d=ge;switch(d.tag){case 0:case 11:case 15:Fs(8,d,o)}var f=d.child;if(f!==null)f.return=d,ge=f;else for(;ge!==null;){d=ge;var h=d.sibling,p=d.return;if(mv(d),d===c){ge=null;break}if(h!==null){h.return=p,ge=h;break}ge=p}}}var v=o.alternate;if(v!==null){var _=v.child;if(_!==null){v.child=null;do{var m=_.sibling;_.sibling=null,_=m}while(_!==null)}}ge=o}}if(o.subtreeFlags&2064&&s!==null)s.return=o,ge=s;else e:for(;ge!==null;){if(o=ge,o.flags&2048)switch(o.tag){case 0:case 11:case 15:Fs(9,o,o.return)}var u=o.sibling;if(u!==null){u.return=o.return,ge=u;break e}ge=o.return}}var g=t.current;for(ge=g;ge!==null;){s=ge;var y=s.child;if(s.subtreeFlags&2064&&y!==null)y.return=s,ge=y;else e:for(s=g;ge!==null;){if(a=ge,a.flags&2048)try{switch(a.tag){case 0:case 11:case 15:pc(9,a)}}catch(C){xt(a,a.return,C)}if(a===s){ge=null;break e}var x=a.sibling;if(x!==null){x.return=a.return,ge=x;break e}ge=a.return}}if(Ke=r,cr(),ii&&typeof ii.onPostCommitFiberRoot=="function")try{ii.onPostCommitFiberRoot(sc,t)}catch{}i=!0}return i}finally{tt=n,Dn.transition=e}}return!1}function $p(t,e,n){e=zo(n,e),e=rv(t,e,1),t=Ji(t,e,1),e=Zt(),t!==null&&(ha(t,1,e),un(t,e))}function xt(t,e,n){if(t.tag===3)$p(t,t,n);else for(;e!==null;){if(e.tag===3){$p(e,t,n);break}else if(e.tag===1){var i=e.stateNode;if(typeof e.type.getDerivedStateFromError=="function"||typeof i.componentDidCatch=="function"&&(Qi===null||!Qi.has(i))){t=zo(n,t),t=ov(e,t,1),e=Ji(e,t,1),t=Zt(),e!==null&&(ha(e,1,t),un(e,t));break}}e=e.return}}function dS(t,e,n){var i=t.pingCache;i!==null&&i.delete(e),e=Zt(),t.pingedLanes|=t.suspendedLanes&n,Lt===t&&(kt&n)===n&&(At===4||At===3&&(kt&130023424)===kt&&500>Mt()-cf?Pr(t,0):lf|=n),un(t,e)}function Tv(t,e){e===0&&(t.mode&1?(e=Ca,Ca<<=1,!(Ca&130023424)&&(Ca=4194304)):e=1);var n=Zt();t=Ei(t,e),t!==null&&(ha(t,e,n),un(t,n))}function hS(t){var e=t.memoizedState,n=0;e!==null&&(n=e.retryLane),Tv(t,n)}function fS(t,e){var n=0;switch(t.tag){case 13:var i=t.stateNode,r=t.memoizedState;r!==null&&(n=r.retryLane);break;case 19:i=t.stateNode;break;default:throw Error(ie(314))}i!==null&&i.delete(e),Tv(t,n)}var Av;Av=function(t,e,n){if(t!==null)if(t.memoizedProps!==e.pendingProps||ln.current)an=!0;else{if(!(t.lanes&n)&&!(e.flags&128))return an=!1,eS(t,e,n);an=!!(t.flags&131072)}else an=!1,mt&&e.flags&1048576&&P0(e,Bl,e.index);switch(e.lanes=0,e.tag){case 2:var i=e.type;vl(t,e),t=e.pendingProps;var r=ko(e,jt.current);bo(e,n),r=tf(null,e,i,t,r,n);var o=nf();return e.flags|=1,typeof r=="object"&&r!==null&&typeof r.render=="function"&&r.$$typeof===void 0?(e.tag=1,e.memoizedState=null,e.updateQueue=null,cn(i)?(o=!0,Ol(e)):o=!1,e.memoizedState=r.state!==null&&r.state!==void 0?r.state:null,Kh(e),r.updater=fc,e.stateNode=r,r._reactInternals=e,vd(e,i,t,n),e=xd(null,e,i,!0,o,n)):(e.tag=0,mt&&o&&Gh(e),$t(null,e,r,n),e=e.child),e;case 16:i=e.elementType;e:{switch(vl(t,e),t=e.pendingProps,r=i._init,i=r(i._payload),e.type=i,r=e.tag=mS(i),t=Hn(i,t),r){case 0:e=_d(null,e,i,t,n);break e;case 1:e=zp(null,e,i,t,n);break e;case 11:e=Fp(null,e,i,t,n);break e;case 14:e=Op(null,e,i,Hn(i.type,t),n);break e}throw Error(ie(306,i,""))}return e;case 0:return i=e.type,r=e.pendingProps,r=e.elementType===i?r:Hn(i,r),_d(t,e,i,r,n);case 1:return i=e.type,r=e.pendingProps,r=e.elementType===i?r:Hn(i,r),zp(t,e,i,r,n);case 3:e:{if(cv(e),t===null)throw Error(ie(387));i=e.pendingProps,o=e.memoizedState,r=o.element,U0(t,e),Gl(e,i,null,n);var s=e.memoizedState;if(i=s.element,o.isDehydrated)if(o={element:i,isDehydrated:!1,cache:s.cache,pendingSuspenseBoundaries:s.pendingSuspenseBoundaries,transitions:s.transitions},e.updateQueue.baseState=o,e.memoizedState=o,e.flags&256){r=zo(Error(ie(423)),e),e=Bp(t,e,i,n,r);break e}else if(i!==r){r=zo(Error(ie(424)),e),e=Bp(t,e,i,n,r);break e}else for(vn=Zi(e.stateNode.containerInfo.firstChild),yn=e,mt=!0,Gn=null,n=I0(e,null,i,n),e.child=n;n;)n.flags=n.flags&-3|4096,n=n.sibling;else{if(Uo(),i===r){e=Ti(t,e,n);break e}$t(t,e,i,n)}e=e.child}return e;case 5:return F0(e),t===null&&pd(e),i=e.type,r=e.pendingProps,o=t!==null?t.memoizedProps:null,s=r.children,cd(i,r)?s=null:o!==null&&cd(i,o)&&(e.flags|=32),lv(t,e),$t(t,e,s,n),e.child;case 6:return t===null&&pd(e),null;case 13:return uv(t,e,n);case 4:return Zh(e,e.stateNode.containerInfo),i=e.pendingProps,t===null?e.child=Fo(e,null,i,n):$t(t,e,i,n),e.child;case 11:return i=e.type,r=e.pendingProps,r=e.elementType===i?r:Hn(i,r),Fp(t,e,i,r,n);case 7:return $t(t,e,e.pendingProps,n),e.child;case 8:return $t(t,e,e.pendingProps.children,n),e.child;case 12:return $t(t,e,e.pendingProps.children,n),e.child;case 10:e:{if(i=e.type._context,r=e.pendingProps,o=e.memoizedProps,s=r.value,lt(Hl,i._currentValue),i._currentValue=s,o!==null)if(Yn(o.value,s)){if(o.children===r.children&&!ln.current){e=Ti(t,e,n);break e}}else for(o=e.child,o!==null&&(o.return=e);o!==null;){var a=o.dependencies;if(a!==null){s=o.child;for(var l=a.firstContext;l!==null;){if(l.context===i){if(o.tag===1){l=xi(-1,n&-n),l.tag=2;var c=o.updateQueue;if(c!==null){c=c.shared;var d=c.pending;d===null?l.next=l:(l.next=d.next,d.next=l),c.pending=l}}o.lanes|=n,l=o.alternate,l!==null&&(l.lanes|=n),md(o.return,n,e),a.lanes|=n;break}l=l.next}}else if(o.tag===10)s=o.type===e.type?null:o.child;else if(o.tag===18){if(s=o.return,s===null)throw Error(ie(341));s.lanes|=n,a=s.alternate,a!==null&&(a.lanes|=n),md(s,n,e),s=o.sibling}else s=o.child;if(s!==null)s.return=o;else for(s=o;s!==null;){if(s===e){s=null;break}if(o=s.sibling,o!==null){o.return=s.return,s=o;break}s=s.return}o=s}$t(t,e,r.children,n),e=e.child}return e;case 9:return r=e.type,i=e.pendingProps.children,bo(e,n),r=Ln(r),i=i(r),e.flags|=1,$t(t,e,i,n),e.child;case 14:return i=e.type,r=Hn(i,e.pendingProps),r=Hn(i.type,r),Op(t,e,i,r,n);case 15:return sv(t,e,e.type,e.pendingProps,n);case 17:return i=e.type,r=e.pendingProps,r=e.elementType===i?r:Hn(i,r),vl(t,e),e.tag=1,cn(i)?(t=!0,Ol(e)):t=!1,bo(e,n),iv(e,i,r),vd(e,i,r,n),xd(null,e,i,!0,t,n);case 19:return dv(t,e,n);case 22:return av(t,e,n)}throw Error(ie(156,e.tag))};function Cv(t,e){return e0(t,e)}function pS(t,e,n,i){this.tag=t,this.key=n,this.sibling=this.child=this.return=this.stateNode=this.type=this.elementType=null,this.index=0,this.ref=null,this.pendingProps=e,this.dependencies=this.memoizedState=this.updateQueue=this.memoizedProps=null,this.mode=i,this.subtreeFlags=this.flags=0,this.deletions=null,this.childLanes=this.lanes=0,this.alternate=null}function Rn(t,e,n,i){return new pS(t,e,n,i)}function ff(t){return t=t.prototype,!(!t||!t.isReactComponent)}function mS(t){if(typeof t=="function")return ff(t)?1:0;if(t!=null){if(t=t.$$typeof,t===Dh)return 11;if(t===Lh)return 14}return 2}function tr(t,e){var n=t.alternate;return n===null?(n=Rn(t.tag,e,t.key,t.mode),n.elementType=t.elementType,n.type=t.type,n.stateNode=t.stateNode,n.alternate=t,t.alternate=n):(n.pendingProps=e,n.type=t.type,n.flags=0,n.subtreeFlags=0,n.deletions=null),n.flags=t.flags&14680064,n.childLanes=t.childLanes,n.lanes=t.lanes,n.child=t.child,n.memoizedProps=t.memoizedProps,n.memoizedState=t.memoizedState,n.updateQueue=t.updateQueue,e=t.dependencies,n.dependencies=e===null?null:{lanes:e.lanes,firstContext:e.firstContext},n.sibling=t.sibling,n.index=t.index,n.ref=t.ref,n}function xl(t,e,n,i,r,o){var s=2;if(i=t,typeof t=="function")ff(t)&&(s=1);else if(typeof t=="string")s=5;else e:switch(t){case co:return Dr(n.children,r,o,e);case Ph:s=8,r|=8;break;case Hu:return t=Rn(12,n,e,r|2),t.elementType=Hu,t.lanes=o,t;case Vu:return t=Rn(13,n,e,r),t.elementType=Vu,t.lanes=o,t;case Gu:return t=Rn(19,n,e,r),t.elementType=Gu,t.lanes=o,t;case Fg:return gc(n,r,o,e);default:if(typeof t=="object"&&t!==null)switch(t.$$typeof){case kg:s=10;break e;case Ug:s=9;break e;case Dh:s=11;break e;case Lh:s=14;break e;case Fi:s=16,i=null;break e}throw Error(ie(130,t==null?t:typeof t,""))}return e=Rn(s,n,e,r),e.elementType=t,e.type=i,e.lanes=o,e}function Dr(t,e,n,i){return t=Rn(7,t,i,e),t.lanes=n,t}function gc(t,e,n,i){return t=Rn(22,t,i,e),t.elementType=Fg,t.lanes=n,t.stateNode={isHidden:!1},t}function Qc(t,e,n){return t=Rn(6,t,null,e),t.lanes=n,t}function eu(t,e,n){return e=Rn(4,t.children!==null?t.children:[],t.key,e),e.lanes=n,e.stateNode={containerInfo:t.containerInfo,pendingChildren:null,implementation:t.implementation},e}function gS(t,e,n,i,r){this.tag=e,this.containerInfo=t,this.finishedWork=this.pingCache=this.current=this.pendingChildren=null,this.timeoutHandle=-1,this.callbackNode=this.pendingContext=this.context=null,this.callbackPriority=0,this.eventTimes=Ic(0),this.expirationTimes=Ic(-1),this.entangledLanes=this.finishedLanes=this.mutableReadLanes=this.expiredLanes=this.pingedLanes=this.suspendedLanes=this.pendingLanes=0,this.entanglements=Ic(0),this.identifierPrefix=i,this.onRecoverableError=r,this.mutableSourceEagerHydrationData=null}function pf(t,e,n,i,r,o,s,a,l){return t=new gS(t,e,n,a,l),e===1?(e=1,o===!0&&(e|=8)):e=0,o=Rn(3,null,null,e),t.current=o,o.stateNode=t,o.memoizedState={element:i,isDehydrated:n,cache:null,transitions:null,pendingSuspenseBoundaries:null},Kh(o),t}function vS(t,e,n){var i=3<arguments.length&&arguments[3]!==void 0?arguments[3]:null;return{$$typeof:lo,key:i==null?null:""+i,children:t,containerInfo:e,implementation:n}}function bv(t){if(!t)return or;t=t._reactInternals;e:{if(zr(t)!==t||t.tag!==1)throw Error(ie(170));var e=t;do{switch(e.tag){case 3:e=e.stateNode.context;break e;case 1:if(cn(e.type)){e=e.stateNode.__reactInternalMemoizedMergedChildContext;break e}}e=e.return}while(e!==null);throw Error(ie(171))}if(t.tag===1){var n=t.type;if(cn(n))return b0(t,n,e)}return e}function Rv(t,e,n,i,r,o,s,a,l){return t=pf(n,i,!0,t,r,o,s,a,l),t.context=bv(null),n=t.current,i=Zt(),r=er(n),o=xi(i,r),o.callback=e??null,Ji(n,o,r),t.current.lanes=r,ha(t,r,i),un(t,i),t}function vc(t,e,n,i){var r=e.current,o=Zt(),s=er(r);return n=bv(n),e.context===null?e.context=n:e.pendingContext=n,e=xi(o,s),e.payload={element:t},i=i===void 0?null:i,i!==null&&(e.callback=i),t=Ji(r,e,s),t!==null&&(jn(t,r,s,o),pl(t,r,s)),s}function Zl(t){if(t=t.current,!t.child)return null;switch(t.child.tag){case 5:return t.child.stateNode;default:return t.child.stateNode}}function Kp(t,e){if(t=t.memoizedState,t!==null&&t.dehydrated!==null){var n=t.retryLane;t.retryLane=n!==0&&n<e?n:e}}function mf(t,e){Kp(t,e),(t=t.alternate)&&Kp(t,e)}function yS(){return null}var Pv=typeof reportError=="function"?reportError:function(t){console.error(t)};function gf(t){this._internalRoot=t}yc.prototype.render=gf.prototype.render=function(t){var e=this._internalRoot;if(e===null)throw Error(ie(409));vc(t,e,null,null)};yc.prototype.unmount=gf.prototype.unmount=function(){var t=this._internalRoot;if(t!==null){this._internalRoot=null;var e=t.containerInfo;Ur(function(){vc(null,t,null,null)}),e[wi]=null}};function yc(t){this._internalRoot=t}yc.prototype.unstable_scheduleHydration=function(t){if(t){var e=a0();t={blockedOn:null,target:t,priority:e};for(var n=0;n<zi.length&&e!==0&&e<zi[n].priority;n++);zi.splice(n,0,t),n===0&&c0(t)}};function vf(t){return!(!t||t.nodeType!==1&&t.nodeType!==9&&t.nodeType!==11)}function _c(t){return!(!t||t.nodeType!==1&&t.nodeType!==9&&t.nodeType!==11&&(t.nodeType!==8||t.nodeValue!==" react-mount-point-unstable "))}function Zp(){}function _S(t,e,n,i,r){if(r){if(typeof i=="function"){var o=i;i=function(){var c=Zl(s);o.call(c)}}var s=Rv(e,i,t,0,null,!1,!1,"",Zp);return t._reactRootContainer=s,t[wi]=s.current,Js(t.nodeType===8?t.parentNode:t),Ur(),s}for(;r=t.lastChild;)t.removeChild(r);if(typeof i=="function"){var a=i;i=function(){var c=Zl(l);a.call(c)}}var l=pf(t,0,!1,null,null,!1,!1,"",Zp);return t._reactRootContainer=l,t[wi]=l.current,Js(t.nodeType===8?t.parentNode:t),Ur(function(){vc(e,l,n,i)}),l}function xc(t,e,n,i,r){var o=n._reactRootContainer;if(o){var s=o;if(typeof r=="function"){var a=r;r=function(){var l=Zl(s);a.call(l)}}vc(e,s,t,r)}else s=_S(n,e,t,r,i);return Zl(s)}o0=function(t){switch(t.tag){case 3:var e=t.stateNode;if(e.current.memoizedState.isDehydrated){var n=Cs(e.pendingLanes);n!==0&&(kh(e,n|1),un(e,Mt()),!(Ke&6)&&(Bo=Mt()+500,cr()))}break;case 13:Ur(function(){var i=Ei(t,1);if(i!==null){var r=Zt();jn(i,t,1,r)}}),mf(t,1)}};Uh=function(t){if(t.tag===13){var e=Ei(t,134217728);if(e!==null){var n=Zt();jn(e,t,134217728,n)}mf(t,134217728)}};s0=function(t){if(t.tag===13){var e=er(t),n=Ei(t,e);if(n!==null){var i=Zt();jn(n,t,e,i)}mf(t,e)}};a0=function(){return tt};l0=function(t,e){var n=tt;try{return tt=t,e()}finally{tt=n}};Qu=function(t,e,n){switch(e){case"input":if(ju(t,n),e=n.name,n.type==="radio"&&e!=null){for(n=t;n.parentNode;)n=n.parentNode;for(n=n.querySelectorAll("input[name="+JSON.stringify(""+e)+'][type="radio"]'),e=0;e<n.length;e++){var i=n[e];if(i!==t&&i.form===t.form){var r=uc(i);if(!r)throw Error(ie(90));zg(i),ju(i,r)}}}break;case"textarea":Hg(t,n);break;case"select":e=n.value,e!=null&&Eo(t,!!n.multiple,e,!1)}};qg=uf;$g=Ur;var xS={usingClientEntryPoint:!1,Events:[pa,po,uc,jg,Yg,uf]},hs={findFiberByHostInstance:Er,bundleType:0,version:"18.3.1",rendererPackageName:"react-dom"},SS={bundleType:hs.bundleType,version:hs.version,rendererPackageName:hs.rendererPackageName,rendererConfig:hs.rendererConfig,overrideHookState:null,overrideHookStateDeletePath:null,overrideHookStateRenamePath:null,overrideProps:null,overridePropsDeletePath:null,overridePropsRenamePath:null,setErrorHandler:null,setSuspenseHandler:null,scheduleUpdate:null,currentDispatcherRef:Ai.ReactCurrentDispatcher,findHostInstanceByFiber:function(t){return t=Jg(t),t===null?null:t.stateNode},findFiberByHostInstance:hs.findFiberByHostInstance||yS,findHostInstancesForRefresh:null,scheduleRefresh:null,scheduleRoot:null,setRefreshHandler:null,getCurrentFiber:null,reconcilerVersion:"18.3.1-next-f1338f8080-20240426"};if(typeof __REACT_DEVTOOLS_GLOBAL_HOOK__<"u"){var Oa=__REACT_DEVTOOLS_GLOBAL_HOOK__;if(!Oa.isDisabled&&Oa.supportsFiber)try{sc=Oa.inject(SS),ii=Oa}catch{}}Sn.__SECRET_INTERNALS_DO_NOT_USE_OR_YOU_WILL_BE_FIRED=xS;Sn.createPortal=function(t,e){var n=2<arguments.length&&arguments[2]!==void 0?arguments[2]:null;if(!vf(e))throw Error(ie(200));return vS(t,e,null,n)};Sn.createRoot=function(t,e){if(!vf(t))throw Error(ie(299));var n=!1,i="",r=Pv;return e!=null&&(e.unstable_strictMode===!0&&(n=!0),e.identifierPrefix!==void 0&&(i=e.identifierPrefix),e.onRecoverableError!==void 0&&(r=e.onRecoverableError)),e=pf(t,1,!1,null,null,n,!1,i,r),t[wi]=e.current,Js(t.nodeType===8?t.parentNode:t),new gf(e)};Sn.findDOMNode=function(t){if(t==null)return null;if(t.nodeType===1)return t;var e=t._reactInternals;if(e===void 0)throw typeof t.render=="function"?Error(ie(188)):(t=Object.keys(t).join(","),Error(ie(268,t)));return t=Jg(e),t=t===null?null:t.stateNode,t};Sn.flushSync=function(t){return Ur(t)};Sn.hydrate=function(t,e,n){if(!_c(e))throw Error(ie(200));return xc(null,t,e,!0,n)};Sn.hydrateRoot=function(t,e,n){if(!vf(t))throw Error(ie(405));var i=n!=null&&n.hydratedSources||null,r=!1,o="",s=Pv;if(n!=null&&(n.unstable_strictMode===!0&&(r=!0),n.identifierPrefix!==void 0&&(o=n.identifierPrefix),n.onRecoverableError!==void 0&&(s=n.onRecoverableError)),e=Rv(e,null,t,1,n??null,r,!1,o,s),t[wi]=e.current,Js(t),i)for(t=0;t<i.length;t++)n=i[t],r=n._getVersion,r=r(n._source),e.mutableSourceEagerHydrationData==null?e.mutableSourceEagerHydrationData=[n,r]:e.mutableSourceEagerHydrationData.push(n,r);return new yc(e)};Sn.render=function(t,e,n){if(!_c(e))throw Error(ie(200));return xc(null,t,e,!1,n)};Sn.unmountComponentAtNode=function(t){if(!_c(t))throw Error(ie(40));return t._reactRootContainer?(Ur(function(){xc(null,null,t,!1,function(){t._reactRootContainer=null,t[wi]=null})}),!0):!1};Sn.unstable_batchedUpdates=uf;Sn.unstable_renderSubtreeIntoContainer=function(t,e,n,i){if(!_c(n))throw Error(ie(200));if(t==null||t._reactInternals===void 0)throw Error(ie(38));return xc(t,e,n,!1,i)};Sn.version="18.3.1-next-f1338f8080-20240426";function Dv(){if(!(typeof __REACT_DEVTOOLS_GLOBAL_HOOK__>"u"||typeof __REACT_DEVTOOLS_GLOBAL_HOOK__.checkDCE!="function"))try{__REACT_DEVTOOLS_GLOBAL_HOOK__.checkDCE(Dv)}catch(t){console.error(t)}}Dv(),Dg.exports=Sn;var MS=Dg.exports,Lv,Jp=MS;Lv=Jp.createRoot,Jp.hydrateRoot;const Po=window.myco;class Nv{constructor(e){this.collection=e,this.rows=[],this.queues=new Map,this.waiting=new Map,this.listeners=[]}sort(){this.rows.sort((e,n)=>{const i=e.data.order,r=n.data.order;return typeof i=="number"&&typeof r=="number"?i-r:0})}get(e){return this.rows.find(n=>n.id===e)||null}async load(){const e=await Po.data.list(this.collection);return this.rows=e.records,this.sort(),this.rows}upsert(e){const n=this.get(e.id);n?n.version<=e.version&&(n.data=e.data,n.version=e.version):this.rows.push({collection:this.collection,id:e.id,data:e.data,version:e.version}),this.sort()}pending(e){return(this.waiting.get(e)||0)>0}put(e,n=""){if(n){const o=this.get(n);o?o.data=e:(this.rows.push({collection:this.collection,id:n,data:e,version:0}),this.sort())}const i=async()=>{const o=n?this.get(n):null;try{const s=await Po.data.put(this.collection,e,{id:n,ifVersion:o?o.version:void 0});n&&this.waiting.set(n,(this.waiting.get(n)||1)-1);const a=this.get(s.id);return n&&this.pending(n)&&a?a.version=Math.max(a.version,s.version):this.upsert(s),this.get(s.id)}catch(s){n&&this.waiting.set(n,(this.waiting.get(n)||1)-1);const a=s;throw a.code==="conflict"&&a.latest&&(this.upsert(a.latest),this.fire({seq:0,collection:this.collection,record:a.latest.id,op:"put",data:a.latest.data,version:a.latest.version})),s}};if(!n)return i();this.waiting.set(n,(this.waiting.get(n)||0)+1);const r=(this.queues.get(n)||Promise.resolve()).then(i,i);return this.queues.set(n,r.catch(()=>{})),r}async remove(e){const n=this.rows.findIndex(o=>o.id===e);n>=0&&this.rows.splice(n,1);const i=async()=>{const o=this.get(e),s=o==null?void 0:o.data,a=o==null?void 0:o.version;await Po.data.remove(this.collection,e).catch(c=>{if(c.code!=="not_found")throw c});const l=this.get(e);l&&this.pending(e)?l.version=0:l&&l.data===s&&l.version===a&&this.rows.splice(this.rows.indexOf(l),1)},r=(this.queues.get(e)||Promise.resolve()).then(i,i);this.queues.set(e,r.catch(()=>{})),await r}on(e){this.listeners.push(e)}fire(e){for(const n of this.listeners)n(e)}apply(e){if(e.collection!==this.collection)return!1;const n=this.get(e.record);if(e.op==="delete"){if(!n)return!1;this.rows.splice(this.rows.indexOf(n),1)}else{if(this.pending(e.record)||n&&n.version>=e.version)return!1;this.upsert({collection:e.collection,id:e.record,data:e.data||{},version:e.version})}return this.fire(e),!0}}function wS(t,e){Po.data.on(n=>{let i=!1;for(const r of t)r.apply(n)&&(i=!0);i&&e&&e(n)})}class ES{constructor(e,n={}){this.stores=e,this.opts=n,this.me="",this.page="",this.pending=0,this.failed=!1,this.version=0,this.note=null,this.subs=new Set,this.noteTimer=0,this.emit=()=>{this.version++;for(const i of this.subs)i()},this.subscribe=i=>(this.subs.add(i),()=>{this.subs.delete(i)})}flash(e,n){this.note={text:e,undo:n},window.clearTimeout(this.noteTimer),this.noteTimer=window.setTimeout(()=>{this.note=null,this.emit()},n?6e3:2600),this.emit()}track(e){return e?(this.pending++,this.emit(),e.then(n=>(this.pending--,this.failed=!1,this.emit(),n),n=>{this.pending--,this.failed=(n==null?void 0:n.code)!=="conflict",this.flash((n==null?void 0:n.code)==="conflict"?ee("别人刚改过，已显示最新的"):(n==null?void 0:n.message)||ee("没保存上"))})):Promise.resolve(void 0)}async start(e,n){var r,o;const i=await Po.ready;this.me=((r=i.me)==null?void 0:r.name)||"",this.page=((o=i.page)==null?void 0:o.id)||"",await Promise.all(this.stores.map(s=>s.load())),e&&await e(),wS(this.stores,s=>{this.emit(),n==null||n(s),!this.opts.quiet&&s.by&&s.by!==this.me&&this.flash(ee("{0} 刚改了",s.by))})}}function TS(t){return ze.useSyncExternalStore(t.subscribe,()=>t.version)}async function AS(t,e,n,i){const r=Lv(document.getElementById("root")),o=V.jsx("div",{className:"outside",children:ee("在 myco App 里打开这个网页，才能读写它的内容。")});if(!Po){r.render(o);return}try{await t.start(n,i)}catch{r.render(o);return}r.render(V.jsx(e,{}))}/**
 * @license
 * Copyright 2010-2024 Three.js Authors
 * SPDX-License-Identifier: MIT
 */const yf="170",CS=0,Qp=1,bS=2,Iv=1,RS=2,fi=3,sr=0,Jt=1,bn=2,nr=0,Do=1,em=2,tm=3,nm=4,kv=5,Mr=100,PS=101,DS=102,LS=103,Uv=104,NS=200,Ld=201,IS=202,kS=203,Nd=204,Id=205,US=206,FS=207,OS=208,zS=209,BS=210,HS=211,VS=212,GS=213,WS=214,kd=0,Ud=1,Fd=2,Ho=3,Od=4,zd=5,Bd=6,Hd=7,Fv=0,XS=1,jS=2,ir=0,YS=1,qS=2,$S=3,Ov=4,KS=5,ZS=6,JS=7,zv=300,Vo=301,Go=302,Vd=303,Gd=304,Sc=306,Wo=1e3,Cr=1001,Wd=1002,dn=1003,QS=1004,za=1005,Kt=1006,tu=1007,ni=1008,oi=1009,Bv=1010,Hv=1011,aa=1012,_f=1013,Fr=1014,Pn=1015,Zo=1016,xf=1017,Sf=1018,Xo=1020,Vv=35902,Gv=1021,Wv=1022,Wt=1023,Xv=1024,jv=1025,Lo=1026,jo=1027,Mf=1028,wf=1029,Yv=1030,Ef=1031,Tf=1033,Sl=33776,Ml=33777,wl=33778,El=33779,Xd=35840,jd=35841,Yd=35842,qd=35843,$d=36196,Kd=37492,Zd=37496,Jd=37808,Qd=37809,eh=37810,th=37811,nh=37812,ih=37813,rh=37814,oh=37815,sh=37816,ah=37817,lh=37818,ch=37819,uh=37820,dh=37821,Tl=36492,hh=36494,fh=36495,qv=36283,ph=36284,mh=36285,gh=36286,eM=3200,tM=3201,$v=0,nM=1,Hi="",gn="srgb",Jo="srgb-linear",Mc="linear",it="srgb",Wr=7680,im=519,iM=512,rM=513,oM=514,Kv=515,sM=516,aM=517,lM=518,cM=519,rm=35044,uM=35048,om="300 es",_i=2e3,Jl=2001;class Qo{addEventListener(e,n){this._listeners===void 0&&(this._listeners={});const i=this._listeners;i[e]===void 0&&(i[e]=[]),i[e].indexOf(n)===-1&&i[e].push(n)}hasEventListener(e,n){if(this._listeners===void 0)return!1;const i=this._listeners;return i[e]!==void 0&&i[e].indexOf(n)!==-1}removeEventListener(e,n){if(this._listeners===void 0)return;const r=this._listeners[e];if(r!==void 0){const o=r.indexOf(n);o!==-1&&r.splice(o,1)}}dispatchEvent(e){if(this._listeners===void 0)return;const i=this._listeners[e.type];if(i!==void 0){e.target=this;const r=i.slice(0);for(let o=0,s=r.length;o<s;o++)r[o].call(this,e);e.target=null}}}const Bt=["00","01","02","03","04","05","06","07","08","09","0a","0b","0c","0d","0e","0f","10","11","12","13","14","15","16","17","18","19","1a","1b","1c","1d","1e","1f","20","21","22","23","24","25","26","27","28","29","2a","2b","2c","2d","2e","2f","30","31","32","33","34","35","36","37","38","39","3a","3b","3c","3d","3e","3f","40","41","42","43","44","45","46","47","48","49","4a","4b","4c","4d","4e","4f","50","51","52","53","54","55","56","57","58","59","5a","5b","5c","5d","5e","5f","60","61","62","63","64","65","66","67","68","69","6a","6b","6c","6d","6e","6f","70","71","72","73","74","75","76","77","78","79","7a","7b","7c","7d","7e","7f","80","81","82","83","84","85","86","87","88","89","8a","8b","8c","8d","8e","8f","90","91","92","93","94","95","96","97","98","99","9a","9b","9c","9d","9e","9f","a0","a1","a2","a3","a4","a5","a6","a7","a8","a9","aa","ab","ac","ad","ae","af","b0","b1","b2","b3","b4","b5","b6","b7","b8","b9","ba","bb","bc","bd","be","bf","c0","c1","c2","c3","c4","c5","c6","c7","c8","c9","ca","cb","cc","cd","ce","cf","d0","d1","d2","d3","d4","d5","d6","d7","d8","d9","da","db","dc","dd","de","df","e0","e1","e2","e3","e4","e5","e6","e7","e8","e9","ea","eb","ec","ed","ee","ef","f0","f1","f2","f3","f4","f5","f6","f7","f8","f9","fa","fb","fc","fd","fe","ff"],nu=Math.PI/180,vh=180/Math.PI;function ga(){const t=Math.random()*4294967295|0,e=Math.random()*4294967295|0,n=Math.random()*4294967295|0,i=Math.random()*4294967295|0;return(Bt[t&255]+Bt[t>>8&255]+Bt[t>>16&255]+Bt[t>>24&255]+"-"+Bt[e&255]+Bt[e>>8&255]+"-"+Bt[e>>16&15|64]+Bt[e>>24&255]+"-"+Bt[n&63|128]+Bt[n>>8&255]+"-"+Bt[n>>16&255]+Bt[n>>24&255]+Bt[i&255]+Bt[i>>8&255]+Bt[i>>16&255]+Bt[i>>24&255]).toLowerCase()}function Gt(t,e,n){return Math.max(e,Math.min(n,t))}function dM(t,e){return(t%e+e)%e}function iu(t,e,n){return(1-n)*t+n*e}function fs(t,e){switch(e.constructor){case Float32Array:return t;case Uint32Array:return t/4294967295;case Uint16Array:return t/65535;case Uint8Array:return t/255;case Int32Array:return Math.max(t/2147483647,-1);case Int16Array:return Math.max(t/32767,-1);case Int8Array:return Math.max(t/127,-1);default:throw new Error("Invalid component type.")}}function on(t,e){switch(e.constructor){case Float32Array:return t;case Uint32Array:return Math.round(t*4294967295);case Uint16Array:return Math.round(t*65535);case Uint8Array:return Math.round(t*255);case Int32Array:return Math.round(t*2147483647);case Int16Array:return Math.round(t*32767);case Int8Array:return Math.round(t*127);default:throw new Error("Invalid component type.")}}class Re{constructor(e=0,n=0){Re.prototype.isVector2=!0,this.x=e,this.y=n}get width(){return this.x}set width(e){this.x=e}get height(){return this.y}set height(e){this.y=e}set(e,n){return this.x=e,this.y=n,this}setScalar(e){return this.x=e,this.y=e,this}setX(e){return this.x=e,this}setY(e){return this.y=e,this}setComponent(e,n){switch(e){case 0:this.x=n;break;case 1:this.y=n;break;default:throw new Error("index is out of range: "+e)}return this}getComponent(e){switch(e){case 0:return this.x;case 1:return this.y;default:throw new Error("index is out of range: "+e)}}clone(){return new this.constructor(this.x,this.y)}copy(e){return this.x=e.x,this.y=e.y,this}add(e){return this.x+=e.x,this.y+=e.y,this}addScalar(e){return this.x+=e,this.y+=e,this}addVectors(e,n){return this.x=e.x+n.x,this.y=e.y+n.y,this}addScaledVector(e,n){return this.x+=e.x*n,this.y+=e.y*n,this}sub(e){return this.x-=e.x,this.y-=e.y,this}subScalar(e){return this.x-=e,this.y-=e,this}subVectors(e,n){return this.x=e.x-n.x,this.y=e.y-n.y,this}multiply(e){return this.x*=e.x,this.y*=e.y,this}multiplyScalar(e){return this.x*=e,this.y*=e,this}divide(e){return this.x/=e.x,this.y/=e.y,this}divideScalar(e){return this.multiplyScalar(1/e)}applyMatrix3(e){const n=this.x,i=this.y,r=e.elements;return this.x=r[0]*n+r[3]*i+r[6],this.y=r[1]*n+r[4]*i+r[7],this}min(e){return this.x=Math.min(this.x,e.x),this.y=Math.min(this.y,e.y),this}max(e){return this.x=Math.max(this.x,e.x),this.y=Math.max(this.y,e.y),this}clamp(e,n){return this.x=Math.max(e.x,Math.min(n.x,this.x)),this.y=Math.max(e.y,Math.min(n.y,this.y)),this}clampScalar(e,n){return this.x=Math.max(e,Math.min(n,this.x)),this.y=Math.max(e,Math.min(n,this.y)),this}clampLength(e,n){const i=this.length();return this.divideScalar(i||1).multiplyScalar(Math.max(e,Math.min(n,i)))}floor(){return this.x=Math.floor(this.x),this.y=Math.floor(this.y),this}ceil(){return this.x=Math.ceil(this.x),this.y=Math.ceil(this.y),this}round(){return this.x=Math.round(this.x),this.y=Math.round(this.y),this}roundToZero(){return this.x=Math.trunc(this.x),this.y=Math.trunc(this.y),this}negate(){return this.x=-this.x,this.y=-this.y,this}dot(e){return this.x*e.x+this.y*e.y}cross(e){return this.x*e.y-this.y*e.x}lengthSq(){return this.x*this.x+this.y*this.y}length(){return Math.sqrt(this.x*this.x+this.y*this.y)}manhattanLength(){return Math.abs(this.x)+Math.abs(this.y)}normalize(){return this.divideScalar(this.length()||1)}angle(){return Math.atan2(-this.y,-this.x)+Math.PI}angleTo(e){const n=Math.sqrt(this.lengthSq()*e.lengthSq());if(n===0)return Math.PI/2;const i=this.dot(e)/n;return Math.acos(Gt(i,-1,1))}distanceTo(e){return Math.sqrt(this.distanceToSquared(e))}distanceToSquared(e){const n=this.x-e.x,i=this.y-e.y;return n*n+i*i}manhattanDistanceTo(e){return Math.abs(this.x-e.x)+Math.abs(this.y-e.y)}setLength(e){return this.normalize().multiplyScalar(e)}lerp(e,n){return this.x+=(e.x-this.x)*n,this.y+=(e.y-this.y)*n,this}lerpVectors(e,n,i){return this.x=e.x+(n.x-e.x)*i,this.y=e.y+(n.y-e.y)*i,this}equals(e){return e.x===this.x&&e.y===this.y}fromArray(e,n=0){return this.x=e[n],this.y=e[n+1],this}toArray(e=[],n=0){return e[n]=this.x,e[n+1]=this.y,e}fromBufferAttribute(e,n){return this.x=e.getX(n),this.y=e.getY(n),this}rotateAround(e,n){const i=Math.cos(n),r=Math.sin(n),o=this.x-e.x,s=this.y-e.y;return this.x=o*i-s*r+e.x,this.y=o*r+s*i+e.y,this}random(){return this.x=Math.random(),this.y=Math.random(),this}*[Symbol.iterator](){yield this.x,yield this.y}}class Be{constructor(e,n,i,r,o,s,a,l,c){Be.prototype.isMatrix3=!0,this.elements=[1,0,0,0,1,0,0,0,1],e!==void 0&&this.set(e,n,i,r,o,s,a,l,c)}set(e,n,i,r,o,s,a,l,c){const d=this.elements;return d[0]=e,d[1]=r,d[2]=a,d[3]=n,d[4]=o,d[5]=l,d[6]=i,d[7]=s,d[8]=c,this}identity(){return this.set(1,0,0,0,1,0,0,0,1),this}copy(e){const n=this.elements,i=e.elements;return n[0]=i[0],n[1]=i[1],n[2]=i[2],n[3]=i[3],n[4]=i[4],n[5]=i[5],n[6]=i[6],n[7]=i[7],n[8]=i[8],this}extractBasis(e,n,i){return e.setFromMatrix3Column(this,0),n.setFromMatrix3Column(this,1),i.setFromMatrix3Column(this,2),this}setFromMatrix4(e){const n=e.elements;return this.set(n[0],n[4],n[8],n[1],n[5],n[9],n[2],n[6],n[10]),this}multiply(e){return this.multiplyMatrices(this,e)}premultiply(e){return this.multiplyMatrices(e,this)}multiplyMatrices(e,n){const i=e.elements,r=n.elements,o=this.elements,s=i[0],a=i[3],l=i[6],c=i[1],d=i[4],f=i[7],h=i[2],p=i[5],v=i[8],_=r[0],m=r[3],u=r[6],g=r[1],y=r[4],x=r[7],C=r[2],A=r[5],T=r[8];return o[0]=s*_+a*g+l*C,o[3]=s*m+a*y+l*A,o[6]=s*u+a*x+l*T,o[1]=c*_+d*g+f*C,o[4]=c*m+d*y+f*A,o[7]=c*u+d*x+f*T,o[2]=h*_+p*g+v*C,o[5]=h*m+p*y+v*A,o[8]=h*u+p*x+v*T,this}multiplyScalar(e){const n=this.elements;return n[0]*=e,n[3]*=e,n[6]*=e,n[1]*=e,n[4]*=e,n[7]*=e,n[2]*=e,n[5]*=e,n[8]*=e,this}determinant(){const e=this.elements,n=e[0],i=e[1],r=e[2],o=e[3],s=e[4],a=e[5],l=e[6],c=e[7],d=e[8];return n*s*d-n*a*c-i*o*d+i*a*l+r*o*c-r*s*l}invert(){const e=this.elements,n=e[0],i=e[1],r=e[2],o=e[3],s=e[4],a=e[5],l=e[6],c=e[7],d=e[8],f=d*s-a*c,h=a*l-d*o,p=c*o-s*l,v=n*f+i*h+r*p;if(v===0)return this.set(0,0,0,0,0,0,0,0,0);const _=1/v;return e[0]=f*_,e[1]=(r*c-d*i)*_,e[2]=(a*i-r*s)*_,e[3]=h*_,e[4]=(d*n-r*l)*_,e[5]=(r*o-a*n)*_,e[6]=p*_,e[7]=(i*l-c*n)*_,e[8]=(s*n-i*o)*_,this}transpose(){let e;const n=this.elements;return e=n[1],n[1]=n[3],n[3]=e,e=n[2],n[2]=n[6],n[6]=e,e=n[5],n[5]=n[7],n[7]=e,this}getNormalMatrix(e){return this.setFromMatrix4(e).invert().transpose()}transposeIntoArray(e){const n=this.elements;return e[0]=n[0],e[1]=n[3],e[2]=n[6],e[3]=n[1],e[4]=n[4],e[5]=n[7],e[6]=n[2],e[7]=n[5],e[8]=n[8],this}setUvTransform(e,n,i,r,o,s,a){const l=Math.cos(o),c=Math.sin(o);return this.set(i*l,i*c,-i*(l*s+c*a)+s+e,-r*c,r*l,-r*(-c*s+l*a)+a+n,0,0,1),this}scale(e,n){return this.premultiply(ru.makeScale(e,n)),this}rotate(e){return this.premultiply(ru.makeRotation(-e)),this}translate(e,n){return this.premultiply(ru.makeTranslation(e,n)),this}makeTranslation(e,n){return e.isVector2?this.set(1,0,e.x,0,1,e.y,0,0,1):this.set(1,0,e,0,1,n,0,0,1),this}makeRotation(e){const n=Math.cos(e),i=Math.sin(e);return this.set(n,-i,0,i,n,0,0,0,1),this}makeScale(e,n){return this.set(e,0,0,0,n,0,0,0,1),this}equals(e){const n=this.elements,i=e.elements;for(let r=0;r<9;r++)if(n[r]!==i[r])return!1;return!0}fromArray(e,n=0){for(let i=0;i<9;i++)this.elements[i]=e[i+n];return this}toArray(e=[],n=0){const i=this.elements;return e[n]=i[0],e[n+1]=i[1],e[n+2]=i[2],e[n+3]=i[3],e[n+4]=i[4],e[n+5]=i[5],e[n+6]=i[6],e[n+7]=i[7],e[n+8]=i[8],e}clone(){return new this.constructor().fromArray(this.elements)}}const ru=new Be;function Zv(t){for(let e=t.length-1;e>=0;--e)if(t[e]>=65535)return!0;return!1}function Ql(t){return document.createElementNS("http://www.w3.org/1999/xhtml",t)}function hM(){const t=Ql("canvas");return t.style.display="block",t}const sm={};function Rs(t){t in sm||(sm[t]=!0,console.warn(t))}function fM(t,e,n){return new Promise(function(i,r){function o(){switch(t.clientWaitSync(e,t.SYNC_FLUSH_COMMANDS_BIT,0)){case t.WAIT_FAILED:r();break;case t.TIMEOUT_EXPIRED:setTimeout(o,n);break;default:i()}}setTimeout(o,n)})}function pM(t){const e=t.elements;e[2]=.5*e[2]+.5*e[3],e[6]=.5*e[6]+.5*e[7],e[10]=.5*e[10]+.5*e[11],e[14]=.5*e[14]+.5*e[15]}function mM(t){const e=t.elements;e[11]===-1?(e[10]=-e[10]-1,e[14]=-e[14]):(e[10]=-e[10],e[14]=-e[14]+1)}const $e={enabled:!0,workingColorSpace:Jo,spaces:{},convert:function(t,e,n){return this.enabled===!1||e===n||!e||!n||(this.spaces[e].transfer===it&&(t.r=Si(t.r),t.g=Si(t.g),t.b=Si(t.b)),this.spaces[e].primaries!==this.spaces[n].primaries&&(t.applyMatrix3(this.spaces[e].toXYZ),t.applyMatrix3(this.spaces[n].fromXYZ)),this.spaces[n].transfer===it&&(t.r=No(t.r),t.g=No(t.g),t.b=No(t.b))),t},fromWorkingColorSpace:function(t,e){return this.convert(t,this.workingColorSpace,e)},toWorkingColorSpace:function(t,e){return this.convert(t,e,this.workingColorSpace)},getPrimaries:function(t){return this.spaces[t].primaries},getTransfer:function(t){return t===Hi?Mc:this.spaces[t].transfer},getLuminanceCoefficients:function(t,e=this.workingColorSpace){return t.fromArray(this.spaces[e].luminanceCoefficients)},define:function(t){Object.assign(this.spaces,t)},_getMatrix:function(t,e,n){return t.copy(this.spaces[e].toXYZ).multiply(this.spaces[n].fromXYZ)},_getDrawingBufferColorSpace:function(t){return this.spaces[t].outputColorSpaceConfig.drawingBufferColorSpace},_getUnpackColorSpace:function(t=this.workingColorSpace){return this.spaces[t].workingColorSpaceConfig.unpackColorSpace}};function Si(t){return t<.04045?t*.0773993808:Math.pow(t*.9478672986+.0521327014,2.4)}function No(t){return t<.0031308?t*12.92:1.055*Math.pow(t,.41666)-.055}const am=[.64,.33,.3,.6,.15,.06],lm=[.2126,.7152,.0722],cm=[.3127,.329],um=new Be().set(.4123908,.3575843,.1804808,.212639,.7151687,.0721923,.0193308,.1191948,.9505322),dm=new Be().set(3.2409699,-1.5373832,-.4986108,-.9692436,1.8759675,.0415551,.0556301,-.203977,1.0569715);$e.define({[Jo]:{primaries:am,whitePoint:cm,transfer:Mc,toXYZ:um,fromXYZ:dm,luminanceCoefficients:lm,workingColorSpaceConfig:{unpackColorSpace:gn},outputColorSpaceConfig:{drawingBufferColorSpace:gn}},[gn]:{primaries:am,whitePoint:cm,transfer:it,toXYZ:um,fromXYZ:dm,luminanceCoefficients:lm,outputColorSpaceConfig:{drawingBufferColorSpace:gn}}});let Xr;class gM{static getDataURL(e){if(/^data:/i.test(e.src)||typeof HTMLCanvasElement>"u")return e.src;let n;if(e instanceof HTMLCanvasElement)n=e;else{Xr===void 0&&(Xr=Ql("canvas")),Xr.width=e.width,Xr.height=e.height;const i=Xr.getContext("2d");e instanceof ImageData?i.putImageData(e,0,0):i.drawImage(e,0,0,e.width,e.height),n=Xr}return n.width>2048||n.height>2048?(console.warn("THREE.ImageUtils.getDataURL: Image converted to jpg for performance reasons",e),n.toDataURL("image/jpeg",.6)):n.toDataURL("image/png")}static sRGBToLinear(e){if(typeof HTMLImageElement<"u"&&e instanceof HTMLImageElement||typeof HTMLCanvasElement<"u"&&e instanceof HTMLCanvasElement||typeof ImageBitmap<"u"&&e instanceof ImageBitmap){const n=Ql("canvas");n.width=e.width,n.height=e.height;const i=n.getContext("2d");i.drawImage(e,0,0,e.width,e.height);const r=i.getImageData(0,0,e.width,e.height),o=r.data;for(let s=0;s<o.length;s++)o[s]=Si(o[s]/255)*255;return i.putImageData(r,0,0),n}else if(e.data){const n=e.data.slice(0);for(let i=0;i<n.length;i++)n instanceof Uint8Array||n instanceof Uint8ClampedArray?n[i]=Math.floor(Si(n[i]/255)*255):n[i]=Si(n[i]);return{data:n,width:e.width,height:e.height}}else return console.warn("THREE.ImageUtils.sRGBToLinear(): Unsupported image type. No color space conversion applied."),e}}let vM=0;class Jv{constructor(e=null){this.isSource=!0,Object.defineProperty(this,"id",{value:vM++}),this.uuid=ga(),this.data=e,this.dataReady=!0,this.version=0}set needsUpdate(e){e===!0&&this.version++}toJSON(e){const n=e===void 0||typeof e=="string";if(!n&&e.images[this.uuid]!==void 0)return e.images[this.uuid];const i={uuid:this.uuid,url:""},r=this.data;if(r!==null){let o;if(Array.isArray(r)){o=[];for(let s=0,a=r.length;s<a;s++)r[s].isDataTexture?o.push(ou(r[s].image)):o.push(ou(r[s]))}else o=ou(r);i.url=o}return n||(e.images[this.uuid]=i),i}}function ou(t){return typeof HTMLImageElement<"u"&&t instanceof HTMLImageElement||typeof HTMLCanvasElement<"u"&&t instanceof HTMLCanvasElement||typeof ImageBitmap<"u"&&t instanceof ImageBitmap?gM.getDataURL(t):t.data?{data:Array.from(t.data),width:t.width,height:t.height,type:t.data.constructor.name}:(console.warn("THREE.Texture: Unable to serialize Texture."),{})}let yM=0;class Qt extends Qo{constructor(e=Qt.DEFAULT_IMAGE,n=Qt.DEFAULT_MAPPING,i=Cr,r=Cr,o=Kt,s=ni,a=Wt,l=oi,c=Qt.DEFAULT_ANISOTROPY,d=Hi){super(),this.isTexture=!0,Object.defineProperty(this,"id",{value:yM++}),this.uuid=ga(),this.name="",this.source=new Jv(e),this.mipmaps=[],this.mapping=n,this.channel=0,this.wrapS=i,this.wrapT=r,this.magFilter=o,this.minFilter=s,this.anisotropy=c,this.format=a,this.internalFormat=null,this.type=l,this.offset=new Re(0,0),this.repeat=new Re(1,1),this.center=new Re(0,0),this.rotation=0,this.matrixAutoUpdate=!0,this.matrix=new Be,this.generateMipmaps=!0,this.premultiplyAlpha=!1,this.flipY=!0,this.unpackAlignment=4,this.colorSpace=d,this.userData={},this.version=0,this.onUpdate=null,this.isRenderTargetTexture=!1,this.pmremVersion=0}get image(){return this.source.data}set image(e=null){this.source.data=e}updateMatrix(){this.matrix.setUvTransform(this.offset.x,this.offset.y,this.repeat.x,this.repeat.y,this.rotation,this.center.x,this.center.y)}clone(){return new this.constructor().copy(this)}copy(e){return this.name=e.name,this.source=e.source,this.mipmaps=e.mipmaps.slice(0),this.mapping=e.mapping,this.channel=e.channel,this.wrapS=e.wrapS,this.wrapT=e.wrapT,this.magFilter=e.magFilter,this.minFilter=e.minFilter,this.anisotropy=e.anisotropy,this.format=e.format,this.internalFormat=e.internalFormat,this.type=e.type,this.offset.copy(e.offset),this.repeat.copy(e.repeat),this.center.copy(e.center),this.rotation=e.rotation,this.matrixAutoUpdate=e.matrixAutoUpdate,this.matrix.copy(e.matrix),this.generateMipmaps=e.generateMipmaps,this.premultiplyAlpha=e.premultiplyAlpha,this.flipY=e.flipY,this.unpackAlignment=e.unpackAlignment,this.colorSpace=e.colorSpace,this.userData=JSON.parse(JSON.stringify(e.userData)),this.needsUpdate=!0,this}toJSON(e){const n=e===void 0||typeof e=="string";if(!n&&e.textures[this.uuid]!==void 0)return e.textures[this.uuid];const i={metadata:{version:4.6,type:"Texture",generator:"Texture.toJSON"},uuid:this.uuid,name:this.name,image:this.source.toJSON(e).uuid,mapping:this.mapping,channel:this.channel,repeat:[this.repeat.x,this.repeat.y],offset:[this.offset.x,this.offset.y],center:[this.center.x,this.center.y],rotation:this.rotation,wrap:[this.wrapS,this.wrapT],format:this.format,internalFormat:this.internalFormat,type:this.type,colorSpace:this.colorSpace,minFilter:this.minFilter,magFilter:this.magFilter,anisotropy:this.anisotropy,flipY:this.flipY,generateMipmaps:this.generateMipmaps,premultiplyAlpha:this.premultiplyAlpha,unpackAlignment:this.unpackAlignment};return Object.keys(this.userData).length>0&&(i.userData=this.userData),n||(e.textures[this.uuid]=i),i}dispose(){this.dispatchEvent({type:"dispose"})}transformUv(e){if(this.mapping!==zv)return e;if(e.applyMatrix3(this.matrix),e.x<0||e.x>1)switch(this.wrapS){case Wo:e.x=e.x-Math.floor(e.x);break;case Cr:e.x=e.x<0?0:1;break;case Wd:Math.abs(Math.floor(e.x)%2)===1?e.x=Math.ceil(e.x)-e.x:e.x=e.x-Math.floor(e.x);break}if(e.y<0||e.y>1)switch(this.wrapT){case Wo:e.y=e.y-Math.floor(e.y);break;case Cr:e.y=e.y<0?0:1;break;case Wd:Math.abs(Math.floor(e.y)%2)===1?e.y=Math.ceil(e.y)-e.y:e.y=e.y-Math.floor(e.y);break}return this.flipY&&(e.y=1-e.y),e}set needsUpdate(e){e===!0&&(this.version++,this.source.needsUpdate=!0)}set needsPMREMUpdate(e){e===!0&&this.pmremVersion++}}Qt.DEFAULT_IMAGE=null;Qt.DEFAULT_MAPPING=zv;Qt.DEFAULT_ANISOTROPY=1;class St{constructor(e=0,n=0,i=0,r=1){St.prototype.isVector4=!0,this.x=e,this.y=n,this.z=i,this.w=r}get width(){return this.z}set width(e){this.z=e}get height(){return this.w}set height(e){this.w=e}set(e,n,i,r){return this.x=e,this.y=n,this.z=i,this.w=r,this}setScalar(e){return this.x=e,this.y=e,this.z=e,this.w=e,this}setX(e){return this.x=e,this}setY(e){return this.y=e,this}setZ(e){return this.z=e,this}setW(e){return this.w=e,this}setComponent(e,n){switch(e){case 0:this.x=n;break;case 1:this.y=n;break;case 2:this.z=n;break;case 3:this.w=n;break;default:throw new Error("index is out of range: "+e)}return this}getComponent(e){switch(e){case 0:return this.x;case 1:return this.y;case 2:return this.z;case 3:return this.w;default:throw new Error("index is out of range: "+e)}}clone(){return new this.constructor(this.x,this.y,this.z,this.w)}copy(e){return this.x=e.x,this.y=e.y,this.z=e.z,this.w=e.w!==void 0?e.w:1,this}add(e){return this.x+=e.x,this.y+=e.y,this.z+=e.z,this.w+=e.w,this}addScalar(e){return this.x+=e,this.y+=e,this.z+=e,this.w+=e,this}addVectors(e,n){return this.x=e.x+n.x,this.y=e.y+n.y,this.z=e.z+n.z,this.w=e.w+n.w,this}addScaledVector(e,n){return this.x+=e.x*n,this.y+=e.y*n,this.z+=e.z*n,this.w+=e.w*n,this}sub(e){return this.x-=e.x,this.y-=e.y,this.z-=e.z,this.w-=e.w,this}subScalar(e){return this.x-=e,this.y-=e,this.z-=e,this.w-=e,this}subVectors(e,n){return this.x=e.x-n.x,this.y=e.y-n.y,this.z=e.z-n.z,this.w=e.w-n.w,this}multiply(e){return this.x*=e.x,this.y*=e.y,this.z*=e.z,this.w*=e.w,this}multiplyScalar(e){return this.x*=e,this.y*=e,this.z*=e,this.w*=e,this}applyMatrix4(e){const n=this.x,i=this.y,r=this.z,o=this.w,s=e.elements;return this.x=s[0]*n+s[4]*i+s[8]*r+s[12]*o,this.y=s[1]*n+s[5]*i+s[9]*r+s[13]*o,this.z=s[2]*n+s[6]*i+s[10]*r+s[14]*o,this.w=s[3]*n+s[7]*i+s[11]*r+s[15]*o,this}divide(e){return this.x/=e.x,this.y/=e.y,this.z/=e.z,this.w/=e.w,this}divideScalar(e){return this.multiplyScalar(1/e)}setAxisAngleFromQuaternion(e){this.w=2*Math.acos(e.w);const n=Math.sqrt(1-e.w*e.w);return n<1e-4?(this.x=1,this.y=0,this.z=0):(this.x=e.x/n,this.y=e.y/n,this.z=e.z/n),this}setAxisAngleFromRotationMatrix(e){let n,i,r,o;const l=e.elements,c=l[0],d=l[4],f=l[8],h=l[1],p=l[5],v=l[9],_=l[2],m=l[6],u=l[10];if(Math.abs(d-h)<.01&&Math.abs(f-_)<.01&&Math.abs(v-m)<.01){if(Math.abs(d+h)<.1&&Math.abs(f+_)<.1&&Math.abs(v+m)<.1&&Math.abs(c+p+u-3)<.1)return this.set(1,0,0,0),this;n=Math.PI;const y=(c+1)/2,x=(p+1)/2,C=(u+1)/2,A=(d+h)/4,T=(f+_)/4,b=(v+m)/4;return y>x&&y>C?y<.01?(i=0,r=.707106781,o=.707106781):(i=Math.sqrt(y),r=A/i,o=T/i):x>C?x<.01?(i=.707106781,r=0,o=.707106781):(r=Math.sqrt(x),i=A/r,o=b/r):C<.01?(i=.707106781,r=.707106781,o=0):(o=Math.sqrt(C),i=T/o,r=b/o),this.set(i,r,o,n),this}let g=Math.sqrt((m-v)*(m-v)+(f-_)*(f-_)+(h-d)*(h-d));return Math.abs(g)<.001&&(g=1),this.x=(m-v)/g,this.y=(f-_)/g,this.z=(h-d)/g,this.w=Math.acos((c+p+u-1)/2),this}setFromMatrixPosition(e){const n=e.elements;return this.x=n[12],this.y=n[13],this.z=n[14],this.w=n[15],this}min(e){return this.x=Math.min(this.x,e.x),this.y=Math.min(this.y,e.y),this.z=Math.min(this.z,e.z),this.w=Math.min(this.w,e.w),this}max(e){return this.x=Math.max(this.x,e.x),this.y=Math.max(this.y,e.y),this.z=Math.max(this.z,e.z),this.w=Math.max(this.w,e.w),this}clamp(e,n){return this.x=Math.max(e.x,Math.min(n.x,this.x)),this.y=Math.max(e.y,Math.min(n.y,this.y)),this.z=Math.max(e.z,Math.min(n.z,this.z)),this.w=Math.max(e.w,Math.min(n.w,this.w)),this}clampScalar(e,n){return this.x=Math.max(e,Math.min(n,this.x)),this.y=Math.max(e,Math.min(n,this.y)),this.z=Math.max(e,Math.min(n,this.z)),this.w=Math.max(e,Math.min(n,this.w)),this}clampLength(e,n){const i=this.length();return this.divideScalar(i||1).multiplyScalar(Math.max(e,Math.min(n,i)))}floor(){return this.x=Math.floor(this.x),this.y=Math.floor(this.y),this.z=Math.floor(this.z),this.w=Math.floor(this.w),this}ceil(){return this.x=Math.ceil(this.x),this.y=Math.ceil(this.y),this.z=Math.ceil(this.z),this.w=Math.ceil(this.w),this}round(){return this.x=Math.round(this.x),this.y=Math.round(this.y),this.z=Math.round(this.z),this.w=Math.round(this.w),this}roundToZero(){return this.x=Math.trunc(this.x),this.y=Math.trunc(this.y),this.z=Math.trunc(this.z),this.w=Math.trunc(this.w),this}negate(){return this.x=-this.x,this.y=-this.y,this.z=-this.z,this.w=-this.w,this}dot(e){return this.x*e.x+this.y*e.y+this.z*e.z+this.w*e.w}lengthSq(){return this.x*this.x+this.y*this.y+this.z*this.z+this.w*this.w}length(){return Math.sqrt(this.x*this.x+this.y*this.y+this.z*this.z+this.w*this.w)}manhattanLength(){return Math.abs(this.x)+Math.abs(this.y)+Math.abs(this.z)+Math.abs(this.w)}normalize(){return this.divideScalar(this.length()||1)}setLength(e){return this.normalize().multiplyScalar(e)}lerp(e,n){return this.x+=(e.x-this.x)*n,this.y+=(e.y-this.y)*n,this.z+=(e.z-this.z)*n,this.w+=(e.w-this.w)*n,this}lerpVectors(e,n,i){return this.x=e.x+(n.x-e.x)*i,this.y=e.y+(n.y-e.y)*i,this.z=e.z+(n.z-e.z)*i,this.w=e.w+(n.w-e.w)*i,this}equals(e){return e.x===this.x&&e.y===this.y&&e.z===this.z&&e.w===this.w}fromArray(e,n=0){return this.x=e[n],this.y=e[n+1],this.z=e[n+2],this.w=e[n+3],this}toArray(e=[],n=0){return e[n]=this.x,e[n+1]=this.y,e[n+2]=this.z,e[n+3]=this.w,e}fromBufferAttribute(e,n){return this.x=e.getX(n),this.y=e.getY(n),this.z=e.getZ(n),this.w=e.getW(n),this}random(){return this.x=Math.random(),this.y=Math.random(),this.z=Math.random(),this.w=Math.random(),this}*[Symbol.iterator](){yield this.x,yield this.y,yield this.z,yield this.w}}class _M extends Qo{constructor(e=1,n=1,i={}){super(),this.isRenderTarget=!0,this.width=e,this.height=n,this.depth=1,this.scissor=new St(0,0,e,n),this.scissorTest=!1,this.viewport=new St(0,0,e,n);const r={width:e,height:n,depth:1};i=Object.assign({generateMipmaps:!1,internalFormat:null,minFilter:Kt,depthBuffer:!0,stencilBuffer:!1,resolveDepthBuffer:!0,resolveStencilBuffer:!0,depthTexture:null,samples:0,count:1},i);const o=new Qt(r,i.mapping,i.wrapS,i.wrapT,i.magFilter,i.minFilter,i.format,i.type,i.anisotropy,i.colorSpace);o.flipY=!1,o.generateMipmaps=i.generateMipmaps,o.internalFormat=i.internalFormat,this.textures=[];const s=i.count;for(let a=0;a<s;a++)this.textures[a]=o.clone(),this.textures[a].isRenderTargetTexture=!0;this.depthBuffer=i.depthBuffer,this.stencilBuffer=i.stencilBuffer,this.resolveDepthBuffer=i.resolveDepthBuffer,this.resolveStencilBuffer=i.resolveStencilBuffer,this.depthTexture=i.depthTexture,this.samples=i.samples}get texture(){return this.textures[0]}set texture(e){this.textures[0]=e}setSize(e,n,i=1){if(this.width!==e||this.height!==n||this.depth!==i){this.width=e,this.height=n,this.depth=i;for(let r=0,o=this.textures.length;r<o;r++)this.textures[r].image.width=e,this.textures[r].image.height=n,this.textures[r].image.depth=i;this.dispose()}this.viewport.set(0,0,e,n),this.scissor.set(0,0,e,n)}clone(){return new this.constructor().copy(this)}copy(e){this.width=e.width,this.height=e.height,this.depth=e.depth,this.scissor.copy(e.scissor),this.scissorTest=e.scissorTest,this.viewport.copy(e.viewport),this.textures.length=0;for(let i=0,r=e.textures.length;i<r;i++)this.textures[i]=e.textures[i].clone(),this.textures[i].isRenderTargetTexture=!0;const n=Object.assign({},e.texture.image);return this.texture.source=new Jv(n),this.depthBuffer=e.depthBuffer,this.stencilBuffer=e.stencilBuffer,this.resolveDepthBuffer=e.resolveDepthBuffer,this.resolveStencilBuffer=e.resolveStencilBuffer,e.depthTexture!==null&&(this.depthTexture=e.depthTexture.clone()),this.samples=e.samples,this}dispose(){this.dispatchEvent({type:"dispose"})}}class qn extends _M{constructor(e=1,n=1,i={}){super(e,n,i),this.isWebGLRenderTarget=!0}}class Qv extends Qt{constructor(e=null,n=1,i=1,r=1){super(null),this.isDataArrayTexture=!0,this.image={data:e,width:n,height:i,depth:r},this.magFilter=dn,this.minFilter=dn,this.wrapR=Cr,this.generateMipmaps=!1,this.flipY=!1,this.unpackAlignment=1,this.layerUpdates=new Set}addLayerUpdate(e){this.layerUpdates.add(e)}clearLayerUpdates(){this.layerUpdates.clear()}}class xM extends Qt{constructor(e=null,n=1,i=1,r=1){super(null),this.isData3DTexture=!0,this.image={data:e,width:n,height:i,depth:r},this.magFilter=dn,this.minFilter=dn,this.wrapR=Cr,this.generateMipmaps=!1,this.flipY=!1,this.unpackAlignment=1}}class es{constructor(e=0,n=0,i=0,r=1){this.isQuaternion=!0,this._x=e,this._y=n,this._z=i,this._w=r}static slerpFlat(e,n,i,r,o,s,a){let l=i[r+0],c=i[r+1],d=i[r+2],f=i[r+3];const h=o[s+0],p=o[s+1],v=o[s+2],_=o[s+3];if(a===0){e[n+0]=l,e[n+1]=c,e[n+2]=d,e[n+3]=f;return}if(a===1){e[n+0]=h,e[n+1]=p,e[n+2]=v,e[n+3]=_;return}if(f!==_||l!==h||c!==p||d!==v){let m=1-a;const u=l*h+c*p+d*v+f*_,g=u>=0?1:-1,y=1-u*u;if(y>Number.EPSILON){const C=Math.sqrt(y),A=Math.atan2(C,u*g);m=Math.sin(m*A)/C,a=Math.sin(a*A)/C}const x=a*g;if(l=l*m+h*x,c=c*m+p*x,d=d*m+v*x,f=f*m+_*x,m===1-a){const C=1/Math.sqrt(l*l+c*c+d*d+f*f);l*=C,c*=C,d*=C,f*=C}}e[n]=l,e[n+1]=c,e[n+2]=d,e[n+3]=f}static multiplyQuaternionsFlat(e,n,i,r,o,s){const a=i[r],l=i[r+1],c=i[r+2],d=i[r+3],f=o[s],h=o[s+1],p=o[s+2],v=o[s+3];return e[n]=a*v+d*f+l*p-c*h,e[n+1]=l*v+d*h+c*f-a*p,e[n+2]=c*v+d*p+a*h-l*f,e[n+3]=d*v-a*f-l*h-c*p,e}get x(){return this._x}set x(e){this._x=e,this._onChangeCallback()}get y(){return this._y}set y(e){this._y=e,this._onChangeCallback()}get z(){return this._z}set z(e){this._z=e,this._onChangeCallback()}get w(){return this._w}set w(e){this._w=e,this._onChangeCallback()}set(e,n,i,r){return this._x=e,this._y=n,this._z=i,this._w=r,this._onChangeCallback(),this}clone(){return new this.constructor(this._x,this._y,this._z,this._w)}copy(e){return this._x=e.x,this._y=e.y,this._z=e.z,this._w=e.w,this._onChangeCallback(),this}setFromEuler(e,n=!0){const i=e._x,r=e._y,o=e._z,s=e._order,a=Math.cos,l=Math.sin,c=a(i/2),d=a(r/2),f=a(o/2),h=l(i/2),p=l(r/2),v=l(o/2);switch(s){case"XYZ":this._x=h*d*f+c*p*v,this._y=c*p*f-h*d*v,this._z=c*d*v+h*p*f,this._w=c*d*f-h*p*v;break;case"YXZ":this._x=h*d*f+c*p*v,this._y=c*p*f-h*d*v,this._z=c*d*v-h*p*f,this._w=c*d*f+h*p*v;break;case"ZXY":this._x=h*d*f-c*p*v,this._y=c*p*f+h*d*v,this._z=c*d*v+h*p*f,this._w=c*d*f-h*p*v;break;case"ZYX":this._x=h*d*f-c*p*v,this._y=c*p*f+h*d*v,this._z=c*d*v-h*p*f,this._w=c*d*f+h*p*v;break;case"YZX":this._x=h*d*f+c*p*v,this._y=c*p*f+h*d*v,this._z=c*d*v-h*p*f,this._w=c*d*f-h*p*v;break;case"XZY":this._x=h*d*f-c*p*v,this._y=c*p*f-h*d*v,this._z=c*d*v+h*p*f,this._w=c*d*f+h*p*v;break;default:console.warn("THREE.Quaternion: .setFromEuler() encountered an unknown order: "+s)}return n===!0&&this._onChangeCallback(),this}setFromAxisAngle(e,n){const i=n/2,r=Math.sin(i);return this._x=e.x*r,this._y=e.y*r,this._z=e.z*r,this._w=Math.cos(i),this._onChangeCallback(),this}setFromRotationMatrix(e){const n=e.elements,i=n[0],r=n[4],o=n[8],s=n[1],a=n[5],l=n[9],c=n[2],d=n[6],f=n[10],h=i+a+f;if(h>0){const p=.5/Math.sqrt(h+1);this._w=.25/p,this._x=(d-l)*p,this._y=(o-c)*p,this._z=(s-r)*p}else if(i>a&&i>f){const p=2*Math.sqrt(1+i-a-f);this._w=(d-l)/p,this._x=.25*p,this._y=(r+s)/p,this._z=(o+c)/p}else if(a>f){const p=2*Math.sqrt(1+a-i-f);this._w=(o-c)/p,this._x=(r+s)/p,this._y=.25*p,this._z=(l+d)/p}else{const p=2*Math.sqrt(1+f-i-a);this._w=(s-r)/p,this._x=(o+c)/p,this._y=(l+d)/p,this._z=.25*p}return this._onChangeCallback(),this}setFromUnitVectors(e,n){let i=e.dot(n)+1;return i<Number.EPSILON?(i=0,Math.abs(e.x)>Math.abs(e.z)?(this._x=-e.y,this._y=e.x,this._z=0,this._w=i):(this._x=0,this._y=-e.z,this._z=e.y,this._w=i)):(this._x=e.y*n.z-e.z*n.y,this._y=e.z*n.x-e.x*n.z,this._z=e.x*n.y-e.y*n.x,this._w=i),this.normalize()}angleTo(e){return 2*Math.acos(Math.abs(Gt(this.dot(e),-1,1)))}rotateTowards(e,n){const i=this.angleTo(e);if(i===0)return this;const r=Math.min(1,n/i);return this.slerp(e,r),this}identity(){return this.set(0,0,0,1)}invert(){return this.conjugate()}conjugate(){return this._x*=-1,this._y*=-1,this._z*=-1,this._onChangeCallback(),this}dot(e){return this._x*e._x+this._y*e._y+this._z*e._z+this._w*e._w}lengthSq(){return this._x*this._x+this._y*this._y+this._z*this._z+this._w*this._w}length(){return Math.sqrt(this._x*this._x+this._y*this._y+this._z*this._z+this._w*this._w)}normalize(){let e=this.length();return e===0?(this._x=0,this._y=0,this._z=0,this._w=1):(e=1/e,this._x=this._x*e,this._y=this._y*e,this._z=this._z*e,this._w=this._w*e),this._onChangeCallback(),this}multiply(e){return this.multiplyQuaternions(this,e)}premultiply(e){return this.multiplyQuaternions(e,this)}multiplyQuaternions(e,n){const i=e._x,r=e._y,o=e._z,s=e._w,a=n._x,l=n._y,c=n._z,d=n._w;return this._x=i*d+s*a+r*c-o*l,this._y=r*d+s*l+o*a-i*c,this._z=o*d+s*c+i*l-r*a,this._w=s*d-i*a-r*l-o*c,this._onChangeCallback(),this}slerp(e,n){if(n===0)return this;if(n===1)return this.copy(e);const i=this._x,r=this._y,o=this._z,s=this._w;let a=s*e._w+i*e._x+r*e._y+o*e._z;if(a<0?(this._w=-e._w,this._x=-e._x,this._y=-e._y,this._z=-e._z,a=-a):this.copy(e),a>=1)return this._w=s,this._x=i,this._y=r,this._z=o,this;const l=1-a*a;if(l<=Number.EPSILON){const p=1-n;return this._w=p*s+n*this._w,this._x=p*i+n*this._x,this._y=p*r+n*this._y,this._z=p*o+n*this._z,this.normalize(),this}const c=Math.sqrt(l),d=Math.atan2(c,a),f=Math.sin((1-n)*d)/c,h=Math.sin(n*d)/c;return this._w=s*f+this._w*h,this._x=i*f+this._x*h,this._y=r*f+this._y*h,this._z=o*f+this._z*h,this._onChangeCallback(),this}slerpQuaternions(e,n,i){return this.copy(e).slerp(n,i)}random(){const e=2*Math.PI*Math.random(),n=2*Math.PI*Math.random(),i=Math.random(),r=Math.sqrt(1-i),o=Math.sqrt(i);return this.set(r*Math.sin(e),r*Math.cos(e),o*Math.sin(n),o*Math.cos(n))}equals(e){return e._x===this._x&&e._y===this._y&&e._z===this._z&&e._w===this._w}fromArray(e,n=0){return this._x=e[n],this._y=e[n+1],this._z=e[n+2],this._w=e[n+3],this._onChangeCallback(),this}toArray(e=[],n=0){return e[n]=this._x,e[n+1]=this._y,e[n+2]=this._z,e[n+3]=this._w,e}fromBufferAttribute(e,n){return this._x=e.getX(n),this._y=e.getY(n),this._z=e.getZ(n),this._w=e.getW(n),this._onChangeCallback(),this}toJSON(){return this.toArray()}_onChange(e){return this._onChangeCallback=e,this}_onChangeCallback(){}*[Symbol.iterator](){yield this._x,yield this._y,yield this._z,yield this._w}}class L{constructor(e=0,n=0,i=0){L.prototype.isVector3=!0,this.x=e,this.y=n,this.z=i}set(e,n,i){return i===void 0&&(i=this.z),this.x=e,this.y=n,this.z=i,this}setScalar(e){return this.x=e,this.y=e,this.z=e,this}setX(e){return this.x=e,this}setY(e){return this.y=e,this}setZ(e){return this.z=e,this}setComponent(e,n){switch(e){case 0:this.x=n;break;case 1:this.y=n;break;case 2:this.z=n;break;default:throw new Error("index is out of range: "+e)}return this}getComponent(e){switch(e){case 0:return this.x;case 1:return this.y;case 2:return this.z;default:throw new Error("index is out of range: "+e)}}clone(){return new this.constructor(this.x,this.y,this.z)}copy(e){return this.x=e.x,this.y=e.y,this.z=e.z,this}add(e){return this.x+=e.x,this.y+=e.y,this.z+=e.z,this}addScalar(e){return this.x+=e,this.y+=e,this.z+=e,this}addVectors(e,n){return this.x=e.x+n.x,this.y=e.y+n.y,this.z=e.z+n.z,this}addScaledVector(e,n){return this.x+=e.x*n,this.y+=e.y*n,this.z+=e.z*n,this}sub(e){return this.x-=e.x,this.y-=e.y,this.z-=e.z,this}subScalar(e){return this.x-=e,this.y-=e,this.z-=e,this}subVectors(e,n){return this.x=e.x-n.x,this.y=e.y-n.y,this.z=e.z-n.z,this}multiply(e){return this.x*=e.x,this.y*=e.y,this.z*=e.z,this}multiplyScalar(e){return this.x*=e,this.y*=e,this.z*=e,this}multiplyVectors(e,n){return this.x=e.x*n.x,this.y=e.y*n.y,this.z=e.z*n.z,this}applyEuler(e){return this.applyQuaternion(hm.setFromEuler(e))}applyAxisAngle(e,n){return this.applyQuaternion(hm.setFromAxisAngle(e,n))}applyMatrix3(e){const n=this.x,i=this.y,r=this.z,o=e.elements;return this.x=o[0]*n+o[3]*i+o[6]*r,this.y=o[1]*n+o[4]*i+o[7]*r,this.z=o[2]*n+o[5]*i+o[8]*r,this}applyNormalMatrix(e){return this.applyMatrix3(e).normalize()}applyMatrix4(e){const n=this.x,i=this.y,r=this.z,o=e.elements,s=1/(o[3]*n+o[7]*i+o[11]*r+o[15]);return this.x=(o[0]*n+o[4]*i+o[8]*r+o[12])*s,this.y=(o[1]*n+o[5]*i+o[9]*r+o[13])*s,this.z=(o[2]*n+o[6]*i+o[10]*r+o[14])*s,this}applyQuaternion(e){const n=this.x,i=this.y,r=this.z,o=e.x,s=e.y,a=e.z,l=e.w,c=2*(s*r-a*i),d=2*(a*n-o*r),f=2*(o*i-s*n);return this.x=n+l*c+s*f-a*d,this.y=i+l*d+a*c-o*f,this.z=r+l*f+o*d-s*c,this}project(e){return this.applyMatrix4(e.matrixWorldInverse).applyMatrix4(e.projectionMatrix)}unproject(e){return this.applyMatrix4(e.projectionMatrixInverse).applyMatrix4(e.matrixWorld)}transformDirection(e){const n=this.x,i=this.y,r=this.z,o=e.elements;return this.x=o[0]*n+o[4]*i+o[8]*r,this.y=o[1]*n+o[5]*i+o[9]*r,this.z=o[2]*n+o[6]*i+o[10]*r,this.normalize()}divide(e){return this.x/=e.x,this.y/=e.y,this.z/=e.z,this}divideScalar(e){return this.multiplyScalar(1/e)}min(e){return this.x=Math.min(this.x,e.x),this.y=Math.min(this.y,e.y),this.z=Math.min(this.z,e.z),this}max(e){return this.x=Math.max(this.x,e.x),this.y=Math.max(this.y,e.y),this.z=Math.max(this.z,e.z),this}clamp(e,n){return this.x=Math.max(e.x,Math.min(n.x,this.x)),this.y=Math.max(e.y,Math.min(n.y,this.y)),this.z=Math.max(e.z,Math.min(n.z,this.z)),this}clampScalar(e,n){return this.x=Math.max(e,Math.min(n,this.x)),this.y=Math.max(e,Math.min(n,this.y)),this.z=Math.max(e,Math.min(n,this.z)),this}clampLength(e,n){const i=this.length();return this.divideScalar(i||1).multiplyScalar(Math.max(e,Math.min(n,i)))}floor(){return this.x=Math.floor(this.x),this.y=Math.floor(this.y),this.z=Math.floor(this.z),this}ceil(){return this.x=Math.ceil(this.x),this.y=Math.ceil(this.y),this.z=Math.ceil(this.z),this}round(){return this.x=Math.round(this.x),this.y=Math.round(this.y),this.z=Math.round(this.z),this}roundToZero(){return this.x=Math.trunc(this.x),this.y=Math.trunc(this.y),this.z=Math.trunc(this.z),this}negate(){return this.x=-this.x,this.y=-this.y,this.z=-this.z,this}dot(e){return this.x*e.x+this.y*e.y+this.z*e.z}lengthSq(){return this.x*this.x+this.y*this.y+this.z*this.z}length(){return Math.sqrt(this.x*this.x+this.y*this.y+this.z*this.z)}manhattanLength(){return Math.abs(this.x)+Math.abs(this.y)+Math.abs(this.z)}normalize(){return this.divideScalar(this.length()||1)}setLength(e){return this.normalize().multiplyScalar(e)}lerp(e,n){return this.x+=(e.x-this.x)*n,this.y+=(e.y-this.y)*n,this.z+=(e.z-this.z)*n,this}lerpVectors(e,n,i){return this.x=e.x+(n.x-e.x)*i,this.y=e.y+(n.y-e.y)*i,this.z=e.z+(n.z-e.z)*i,this}cross(e){return this.crossVectors(this,e)}crossVectors(e,n){const i=e.x,r=e.y,o=e.z,s=n.x,a=n.y,l=n.z;return this.x=r*l-o*a,this.y=o*s-i*l,this.z=i*a-r*s,this}projectOnVector(e){const n=e.lengthSq();if(n===0)return this.set(0,0,0);const i=e.dot(this)/n;return this.copy(e).multiplyScalar(i)}projectOnPlane(e){return su.copy(this).projectOnVector(e),this.sub(su)}reflect(e){return this.sub(su.copy(e).multiplyScalar(2*this.dot(e)))}angleTo(e){const n=Math.sqrt(this.lengthSq()*e.lengthSq());if(n===0)return Math.PI/2;const i=this.dot(e)/n;return Math.acos(Gt(i,-1,1))}distanceTo(e){return Math.sqrt(this.distanceToSquared(e))}distanceToSquared(e){const n=this.x-e.x,i=this.y-e.y,r=this.z-e.z;return n*n+i*i+r*r}manhattanDistanceTo(e){return Math.abs(this.x-e.x)+Math.abs(this.y-e.y)+Math.abs(this.z-e.z)}setFromSpherical(e){return this.setFromSphericalCoords(e.radius,e.phi,e.theta)}setFromSphericalCoords(e,n,i){const r=Math.sin(n)*e;return this.x=r*Math.sin(i),this.y=Math.cos(n)*e,this.z=r*Math.cos(i),this}setFromCylindrical(e){return this.setFromCylindricalCoords(e.radius,e.theta,e.y)}setFromCylindricalCoords(e,n,i){return this.x=e*Math.sin(n),this.y=i,this.z=e*Math.cos(n),this}setFromMatrixPosition(e){const n=e.elements;return this.x=n[12],this.y=n[13],this.z=n[14],this}setFromMatrixScale(e){const n=this.setFromMatrixColumn(e,0).length(),i=this.setFromMatrixColumn(e,1).length(),r=this.setFromMatrixColumn(e,2).length();return this.x=n,this.y=i,this.z=r,this}setFromMatrixColumn(e,n){return this.fromArray(e.elements,n*4)}setFromMatrix3Column(e,n){return this.fromArray(e.elements,n*3)}setFromEuler(e){return this.x=e._x,this.y=e._y,this.z=e._z,this}setFromColor(e){return this.x=e.r,this.y=e.g,this.z=e.b,this}equals(e){return e.x===this.x&&e.y===this.y&&e.z===this.z}fromArray(e,n=0){return this.x=e[n],this.y=e[n+1],this.z=e[n+2],this}toArray(e=[],n=0){return e[n]=this.x,e[n+1]=this.y,e[n+2]=this.z,e}fromBufferAttribute(e,n){return this.x=e.getX(n),this.y=e.getY(n),this.z=e.getZ(n),this}random(){return this.x=Math.random(),this.y=Math.random(),this.z=Math.random(),this}randomDirection(){const e=Math.random()*Math.PI*2,n=Math.random()*2-1,i=Math.sqrt(1-n*n);return this.x=i*Math.cos(e),this.y=n,this.z=i*Math.sin(e),this}*[Symbol.iterator](){yield this.x,yield this.y,yield this.z}}const su=new L,hm=new es;class Br{constructor(e=new L(1/0,1/0,1/0),n=new L(-1/0,-1/0,-1/0)){this.isBox3=!0,this.min=e,this.max=n}set(e,n){return this.min.copy(e),this.max.copy(n),this}setFromArray(e){this.makeEmpty();for(let n=0,i=e.length;n<i;n+=3)this.expandByPoint(On.fromArray(e,n));return this}setFromBufferAttribute(e){this.makeEmpty();for(let n=0,i=e.count;n<i;n++)this.expandByPoint(On.fromBufferAttribute(e,n));return this}setFromPoints(e){this.makeEmpty();for(let n=0,i=e.length;n<i;n++)this.expandByPoint(e[n]);return this}setFromCenterAndSize(e,n){const i=On.copy(n).multiplyScalar(.5);return this.min.copy(e).sub(i),this.max.copy(e).add(i),this}setFromObject(e,n=!1){return this.makeEmpty(),this.expandByObject(e,n)}clone(){return new this.constructor().copy(this)}copy(e){return this.min.copy(e.min),this.max.copy(e.max),this}makeEmpty(){return this.min.x=this.min.y=this.min.z=1/0,this.max.x=this.max.y=this.max.z=-1/0,this}isEmpty(){return this.max.x<this.min.x||this.max.y<this.min.y||this.max.z<this.min.z}getCenter(e){return this.isEmpty()?e.set(0,0,0):e.addVectors(this.min,this.max).multiplyScalar(.5)}getSize(e){return this.isEmpty()?e.set(0,0,0):e.subVectors(this.max,this.min)}expandByPoint(e){return this.min.min(e),this.max.max(e),this}expandByVector(e){return this.min.sub(e),this.max.add(e),this}expandByScalar(e){return this.min.addScalar(-e),this.max.addScalar(e),this}expandByObject(e,n=!1){e.updateWorldMatrix(!1,!1);const i=e.geometry;if(i!==void 0){const o=i.getAttribute("position");if(n===!0&&o!==void 0&&e.isInstancedMesh!==!0)for(let s=0,a=o.count;s<a;s++)e.isMesh===!0?e.getVertexPosition(s,On):On.fromBufferAttribute(o,s),On.applyMatrix4(e.matrixWorld),this.expandByPoint(On);else e.boundingBox!==void 0?(e.boundingBox===null&&e.computeBoundingBox(),Ba.copy(e.boundingBox)):(i.boundingBox===null&&i.computeBoundingBox(),Ba.copy(i.boundingBox)),Ba.applyMatrix4(e.matrixWorld),this.union(Ba)}const r=e.children;for(let o=0,s=r.length;o<s;o++)this.expandByObject(r[o],n);return this}containsPoint(e){return e.x>=this.min.x&&e.x<=this.max.x&&e.y>=this.min.y&&e.y<=this.max.y&&e.z>=this.min.z&&e.z<=this.max.z}containsBox(e){return this.min.x<=e.min.x&&e.max.x<=this.max.x&&this.min.y<=e.min.y&&e.max.y<=this.max.y&&this.min.z<=e.min.z&&e.max.z<=this.max.z}getParameter(e,n){return n.set((e.x-this.min.x)/(this.max.x-this.min.x),(e.y-this.min.y)/(this.max.y-this.min.y),(e.z-this.min.z)/(this.max.z-this.min.z))}intersectsBox(e){return e.max.x>=this.min.x&&e.min.x<=this.max.x&&e.max.y>=this.min.y&&e.min.y<=this.max.y&&e.max.z>=this.min.z&&e.min.z<=this.max.z}intersectsSphere(e){return this.clampPoint(e.center,On),On.distanceToSquared(e.center)<=e.radius*e.radius}intersectsPlane(e){let n,i;return e.normal.x>0?(n=e.normal.x*this.min.x,i=e.normal.x*this.max.x):(n=e.normal.x*this.max.x,i=e.normal.x*this.min.x),e.normal.y>0?(n+=e.normal.y*this.min.y,i+=e.normal.y*this.max.y):(n+=e.normal.y*this.max.y,i+=e.normal.y*this.min.y),e.normal.z>0?(n+=e.normal.z*this.min.z,i+=e.normal.z*this.max.z):(n+=e.normal.z*this.max.z,i+=e.normal.z*this.min.z),n<=-e.constant&&i>=-e.constant}intersectsTriangle(e){if(this.isEmpty())return!1;this.getCenter(ps),Ha.subVectors(this.max,ps),jr.subVectors(e.a,ps),Yr.subVectors(e.b,ps),qr.subVectors(e.c,ps),Di.subVectors(Yr,jr),Li.subVectors(qr,Yr),dr.subVectors(jr,qr);let n=[0,-Di.z,Di.y,0,-Li.z,Li.y,0,-dr.z,dr.y,Di.z,0,-Di.x,Li.z,0,-Li.x,dr.z,0,-dr.x,-Di.y,Di.x,0,-Li.y,Li.x,0,-dr.y,dr.x,0];return!au(n,jr,Yr,qr,Ha)||(n=[1,0,0,0,1,0,0,0,1],!au(n,jr,Yr,qr,Ha))?!1:(Va.crossVectors(Di,Li),n=[Va.x,Va.y,Va.z],au(n,jr,Yr,qr,Ha))}clampPoint(e,n){return n.copy(e).clamp(this.min,this.max)}distanceToPoint(e){return this.clampPoint(e,On).distanceTo(e)}getBoundingSphere(e){return this.isEmpty()?e.makeEmpty():(this.getCenter(e.center),e.radius=this.getSize(On).length()*.5),e}intersect(e){return this.min.max(e.min),this.max.min(e.max),this.isEmpty()&&this.makeEmpty(),this}union(e){return this.min.min(e.min),this.max.max(e.max),this}applyMatrix4(e){return this.isEmpty()?this:(li[0].set(this.min.x,this.min.y,this.min.z).applyMatrix4(e),li[1].set(this.min.x,this.min.y,this.max.z).applyMatrix4(e),li[2].set(this.min.x,this.max.y,this.min.z).applyMatrix4(e),li[3].set(this.min.x,this.max.y,this.max.z).applyMatrix4(e),li[4].set(this.max.x,this.min.y,this.min.z).applyMatrix4(e),li[5].set(this.max.x,this.min.y,this.max.z).applyMatrix4(e),li[6].set(this.max.x,this.max.y,this.min.z).applyMatrix4(e),li[7].set(this.max.x,this.max.y,this.max.z).applyMatrix4(e),this.setFromPoints(li),this)}translate(e){return this.min.add(e),this.max.add(e),this}equals(e){return e.min.equals(this.min)&&e.max.equals(this.max)}}const li=[new L,new L,new L,new L,new L,new L,new L,new L],On=new L,Ba=new Br,jr=new L,Yr=new L,qr=new L,Di=new L,Li=new L,dr=new L,ps=new L,Ha=new L,Va=new L,hr=new L;function au(t,e,n,i,r){for(let o=0,s=t.length-3;o<=s;o+=3){hr.fromArray(t,o);const a=r.x*Math.abs(hr.x)+r.y*Math.abs(hr.y)+r.z*Math.abs(hr.z),l=e.dot(hr),c=n.dot(hr),d=i.dot(hr);if(Math.max(-Math.max(l,c,d),Math.min(l,c,d))>a)return!1}return!0}const SM=new Br,ms=new L,lu=new L;class va{constructor(e=new L,n=-1){this.isSphere=!0,this.center=e,this.radius=n}set(e,n){return this.center.copy(e),this.radius=n,this}setFromPoints(e,n){const i=this.center;n!==void 0?i.copy(n):SM.setFromPoints(e).getCenter(i);let r=0;for(let o=0,s=e.length;o<s;o++)r=Math.max(r,i.distanceToSquared(e[o]));return this.radius=Math.sqrt(r),this}copy(e){return this.center.copy(e.center),this.radius=e.radius,this}isEmpty(){return this.radius<0}makeEmpty(){return this.center.set(0,0,0),this.radius=-1,this}containsPoint(e){return e.distanceToSquared(this.center)<=this.radius*this.radius}distanceToPoint(e){return e.distanceTo(this.center)-this.radius}intersectsSphere(e){const n=this.radius+e.radius;return e.center.distanceToSquared(this.center)<=n*n}intersectsBox(e){return e.intersectsSphere(this)}intersectsPlane(e){return Math.abs(e.distanceToPoint(this.center))<=this.radius}clampPoint(e,n){const i=this.center.distanceToSquared(e);return n.copy(e),i>this.radius*this.radius&&(n.sub(this.center).normalize(),n.multiplyScalar(this.radius).add(this.center)),n}getBoundingBox(e){return this.isEmpty()?(e.makeEmpty(),e):(e.set(this.center,this.center),e.expandByScalar(this.radius),e)}applyMatrix4(e){return this.center.applyMatrix4(e),this.radius=this.radius*e.getMaxScaleOnAxis(),this}translate(e){return this.center.add(e),this}expandByPoint(e){if(this.isEmpty())return this.center.copy(e),this.radius=0,this;ms.subVectors(e,this.center);const n=ms.lengthSq();if(n>this.radius*this.radius){const i=Math.sqrt(n),r=(i-this.radius)*.5;this.center.addScaledVector(ms,r/i),this.radius+=r}return this}union(e){return e.isEmpty()?this:this.isEmpty()?(this.copy(e),this):(this.center.equals(e.center)===!0?this.radius=Math.max(this.radius,e.radius):(lu.subVectors(e.center,this.center).setLength(e.radius),this.expandByPoint(ms.copy(e.center).add(lu)),this.expandByPoint(ms.copy(e.center).sub(lu))),this)}equals(e){return e.center.equals(this.center)&&e.radius===this.radius}clone(){return new this.constructor().copy(this)}}const ci=new L,cu=new L,Ga=new L,Ni=new L,uu=new L,Wa=new L,du=new L;class MM{constructor(e=new L,n=new L(0,0,-1)){this.origin=e,this.direction=n}set(e,n){return this.origin.copy(e),this.direction.copy(n),this}copy(e){return this.origin.copy(e.origin),this.direction.copy(e.direction),this}at(e,n){return n.copy(this.origin).addScaledVector(this.direction,e)}lookAt(e){return this.direction.copy(e).sub(this.origin).normalize(),this}recast(e){return this.origin.copy(this.at(e,ci)),this}closestPointToPoint(e,n){n.subVectors(e,this.origin);const i=n.dot(this.direction);return i<0?n.copy(this.origin):n.copy(this.origin).addScaledVector(this.direction,i)}distanceToPoint(e){return Math.sqrt(this.distanceSqToPoint(e))}distanceSqToPoint(e){const n=ci.subVectors(e,this.origin).dot(this.direction);return n<0?this.origin.distanceToSquared(e):(ci.copy(this.origin).addScaledVector(this.direction,n),ci.distanceToSquared(e))}distanceSqToSegment(e,n,i,r){cu.copy(e).add(n).multiplyScalar(.5),Ga.copy(n).sub(e).normalize(),Ni.copy(this.origin).sub(cu);const o=e.distanceTo(n)*.5,s=-this.direction.dot(Ga),a=Ni.dot(this.direction),l=-Ni.dot(Ga),c=Ni.lengthSq(),d=Math.abs(1-s*s);let f,h,p,v;if(d>0)if(f=s*l-a,h=s*a-l,v=o*d,f>=0)if(h>=-v)if(h<=v){const _=1/d;f*=_,h*=_,p=f*(f+s*h+2*a)+h*(s*f+h+2*l)+c}else h=o,f=Math.max(0,-(s*h+a)),p=-f*f+h*(h+2*l)+c;else h=-o,f=Math.max(0,-(s*h+a)),p=-f*f+h*(h+2*l)+c;else h<=-v?(f=Math.max(0,-(-s*o+a)),h=f>0?-o:Math.min(Math.max(-o,-l),o),p=-f*f+h*(h+2*l)+c):h<=v?(f=0,h=Math.min(Math.max(-o,-l),o),p=h*(h+2*l)+c):(f=Math.max(0,-(s*o+a)),h=f>0?o:Math.min(Math.max(-o,-l),o),p=-f*f+h*(h+2*l)+c);else h=s>0?-o:o,f=Math.max(0,-(s*h+a)),p=-f*f+h*(h+2*l)+c;return i&&i.copy(this.origin).addScaledVector(this.direction,f),r&&r.copy(cu).addScaledVector(Ga,h),p}intersectSphere(e,n){ci.subVectors(e.center,this.origin);const i=ci.dot(this.direction),r=ci.dot(ci)-i*i,o=e.radius*e.radius;if(r>o)return null;const s=Math.sqrt(o-r),a=i-s,l=i+s;return l<0?null:a<0?this.at(l,n):this.at(a,n)}intersectsSphere(e){return this.distanceSqToPoint(e.center)<=e.radius*e.radius}distanceToPlane(e){const n=e.normal.dot(this.direction);if(n===0)return e.distanceToPoint(this.origin)===0?0:null;const i=-(this.origin.dot(e.normal)+e.constant)/n;return i>=0?i:null}intersectPlane(e,n){const i=this.distanceToPlane(e);return i===null?null:this.at(i,n)}intersectsPlane(e){const n=e.distanceToPoint(this.origin);return n===0||e.normal.dot(this.direction)*n<0}intersectBox(e,n){let i,r,o,s,a,l;const c=1/this.direction.x,d=1/this.direction.y,f=1/this.direction.z,h=this.origin;return c>=0?(i=(e.min.x-h.x)*c,r=(e.max.x-h.x)*c):(i=(e.max.x-h.x)*c,r=(e.min.x-h.x)*c),d>=0?(o=(e.min.y-h.y)*d,s=(e.max.y-h.y)*d):(o=(e.max.y-h.y)*d,s=(e.min.y-h.y)*d),i>s||o>r||((o>i||isNaN(i))&&(i=o),(s<r||isNaN(r))&&(r=s),f>=0?(a=(e.min.z-h.z)*f,l=(e.max.z-h.z)*f):(a=(e.max.z-h.z)*f,l=(e.min.z-h.z)*f),i>l||a>r)||((a>i||i!==i)&&(i=a),(l<r||r!==r)&&(r=l),r<0)?null:this.at(i>=0?i:r,n)}intersectsBox(e){return this.intersectBox(e,ci)!==null}intersectTriangle(e,n,i,r,o){uu.subVectors(n,e),Wa.subVectors(i,e),du.crossVectors(uu,Wa);let s=this.direction.dot(du),a;if(s>0){if(r)return null;a=1}else if(s<0)a=-1,s=-s;else return null;Ni.subVectors(this.origin,e);const l=a*this.direction.dot(Wa.crossVectors(Ni,Wa));if(l<0)return null;const c=a*this.direction.dot(uu.cross(Ni));if(c<0||l+c>s)return null;const d=-a*Ni.dot(du);return d<0?null:this.at(d/s,o)}applyMatrix4(e){return this.origin.applyMatrix4(e),this.direction.transformDirection(e),this}equals(e){return e.origin.equals(this.origin)&&e.direction.equals(this.direction)}clone(){return new this.constructor().copy(this)}}class rt{constructor(e,n,i,r,o,s,a,l,c,d,f,h,p,v,_,m){rt.prototype.isMatrix4=!0,this.elements=[1,0,0,0,0,1,0,0,0,0,1,0,0,0,0,1],e!==void 0&&this.set(e,n,i,r,o,s,a,l,c,d,f,h,p,v,_,m)}set(e,n,i,r,o,s,a,l,c,d,f,h,p,v,_,m){const u=this.elements;return u[0]=e,u[4]=n,u[8]=i,u[12]=r,u[1]=o,u[5]=s,u[9]=a,u[13]=l,u[2]=c,u[6]=d,u[10]=f,u[14]=h,u[3]=p,u[7]=v,u[11]=_,u[15]=m,this}identity(){return this.set(1,0,0,0,0,1,0,0,0,0,1,0,0,0,0,1),this}clone(){return new rt().fromArray(this.elements)}copy(e){const n=this.elements,i=e.elements;return n[0]=i[0],n[1]=i[1],n[2]=i[2],n[3]=i[3],n[4]=i[4],n[5]=i[5],n[6]=i[6],n[7]=i[7],n[8]=i[8],n[9]=i[9],n[10]=i[10],n[11]=i[11],n[12]=i[12],n[13]=i[13],n[14]=i[14],n[15]=i[15],this}copyPosition(e){const n=this.elements,i=e.elements;return n[12]=i[12],n[13]=i[13],n[14]=i[14],this}setFromMatrix3(e){const n=e.elements;return this.set(n[0],n[3],n[6],0,n[1],n[4],n[7],0,n[2],n[5],n[8],0,0,0,0,1),this}extractBasis(e,n,i){return e.setFromMatrixColumn(this,0),n.setFromMatrixColumn(this,1),i.setFromMatrixColumn(this,2),this}makeBasis(e,n,i){return this.set(e.x,n.x,i.x,0,e.y,n.y,i.y,0,e.z,n.z,i.z,0,0,0,0,1),this}extractRotation(e){const n=this.elements,i=e.elements,r=1/$r.setFromMatrixColumn(e,0).length(),o=1/$r.setFromMatrixColumn(e,1).length(),s=1/$r.setFromMatrixColumn(e,2).length();return n[0]=i[0]*r,n[1]=i[1]*r,n[2]=i[2]*r,n[3]=0,n[4]=i[4]*o,n[5]=i[5]*o,n[6]=i[6]*o,n[7]=0,n[8]=i[8]*s,n[9]=i[9]*s,n[10]=i[10]*s,n[11]=0,n[12]=0,n[13]=0,n[14]=0,n[15]=1,this}makeRotationFromEuler(e){const n=this.elements,i=e.x,r=e.y,o=e.z,s=Math.cos(i),a=Math.sin(i),l=Math.cos(r),c=Math.sin(r),d=Math.cos(o),f=Math.sin(o);if(e.order==="XYZ"){const h=s*d,p=s*f,v=a*d,_=a*f;n[0]=l*d,n[4]=-l*f,n[8]=c,n[1]=p+v*c,n[5]=h-_*c,n[9]=-a*l,n[2]=_-h*c,n[6]=v+p*c,n[10]=s*l}else if(e.order==="YXZ"){const h=l*d,p=l*f,v=c*d,_=c*f;n[0]=h+_*a,n[4]=v*a-p,n[8]=s*c,n[1]=s*f,n[5]=s*d,n[9]=-a,n[2]=p*a-v,n[6]=_+h*a,n[10]=s*l}else if(e.order==="ZXY"){const h=l*d,p=l*f,v=c*d,_=c*f;n[0]=h-_*a,n[4]=-s*f,n[8]=v+p*a,n[1]=p+v*a,n[5]=s*d,n[9]=_-h*a,n[2]=-s*c,n[6]=a,n[10]=s*l}else if(e.order==="ZYX"){const h=s*d,p=s*f,v=a*d,_=a*f;n[0]=l*d,n[4]=v*c-p,n[8]=h*c+_,n[1]=l*f,n[5]=_*c+h,n[9]=p*c-v,n[2]=-c,n[6]=a*l,n[10]=s*l}else if(e.order==="YZX"){const h=s*l,p=s*c,v=a*l,_=a*c;n[0]=l*d,n[4]=_-h*f,n[8]=v*f+p,n[1]=f,n[5]=s*d,n[9]=-a*d,n[2]=-c*d,n[6]=p*f+v,n[10]=h-_*f}else if(e.order==="XZY"){const h=s*l,p=s*c,v=a*l,_=a*c;n[0]=l*d,n[4]=-f,n[8]=c*d,n[1]=h*f+_,n[5]=s*d,n[9]=p*f-v,n[2]=v*f-p,n[6]=a*d,n[10]=_*f+h}return n[3]=0,n[7]=0,n[11]=0,n[12]=0,n[13]=0,n[14]=0,n[15]=1,this}makeRotationFromQuaternion(e){return this.compose(wM,e,EM)}lookAt(e,n,i){const r=this.elements;return fn.subVectors(e,n),fn.lengthSq()===0&&(fn.z=1),fn.normalize(),Ii.crossVectors(i,fn),Ii.lengthSq()===0&&(Math.abs(i.z)===1?fn.x+=1e-4:fn.z+=1e-4,fn.normalize(),Ii.crossVectors(i,fn)),Ii.normalize(),Xa.crossVectors(fn,Ii),r[0]=Ii.x,r[4]=Xa.x,r[8]=fn.x,r[1]=Ii.y,r[5]=Xa.y,r[9]=fn.y,r[2]=Ii.z,r[6]=Xa.z,r[10]=fn.z,this}multiply(e){return this.multiplyMatrices(this,e)}premultiply(e){return this.multiplyMatrices(e,this)}multiplyMatrices(e,n){const i=e.elements,r=n.elements,o=this.elements,s=i[0],a=i[4],l=i[8],c=i[12],d=i[1],f=i[5],h=i[9],p=i[13],v=i[2],_=i[6],m=i[10],u=i[14],g=i[3],y=i[7],x=i[11],C=i[15],A=r[0],T=r[4],b=r[8],w=r[12],S=r[1],P=r[5],z=r[9],F=r[13],Y=r[2],j=r[6],W=r[10],$=r[14],D=r[3],G=r[7],Z=r[11],le=r[15];return o[0]=s*A+a*S+l*Y+c*D,o[4]=s*T+a*P+l*j+c*G,o[8]=s*b+a*z+l*W+c*Z,o[12]=s*w+a*F+l*$+c*le,o[1]=d*A+f*S+h*Y+p*D,o[5]=d*T+f*P+h*j+p*G,o[9]=d*b+f*z+h*W+p*Z,o[13]=d*w+f*F+h*$+p*le,o[2]=v*A+_*S+m*Y+u*D,o[6]=v*T+_*P+m*j+u*G,o[10]=v*b+_*z+m*W+u*Z,o[14]=v*w+_*F+m*$+u*le,o[3]=g*A+y*S+x*Y+C*D,o[7]=g*T+y*P+x*j+C*G,o[11]=g*b+y*z+x*W+C*Z,o[15]=g*w+y*F+x*$+C*le,this}multiplyScalar(e){const n=this.elements;return n[0]*=e,n[4]*=e,n[8]*=e,n[12]*=e,n[1]*=e,n[5]*=e,n[9]*=e,n[13]*=e,n[2]*=e,n[6]*=e,n[10]*=e,n[14]*=e,n[3]*=e,n[7]*=e,n[11]*=e,n[15]*=e,this}determinant(){const e=this.elements,n=e[0],i=e[4],r=e[8],o=e[12],s=e[1],a=e[5],l=e[9],c=e[13],d=e[2],f=e[6],h=e[10],p=e[14],v=e[3],_=e[7],m=e[11],u=e[15];return v*(+o*l*f-r*c*f-o*a*h+i*c*h+r*a*p-i*l*p)+_*(+n*l*p-n*c*h+o*s*h-r*s*p+r*c*d-o*l*d)+m*(+n*c*f-n*a*p-o*s*f+i*s*p+o*a*d-i*c*d)+u*(-r*a*d-n*l*f+n*a*h+r*s*f-i*s*h+i*l*d)}transpose(){const e=this.elements;let n;return n=e[1],e[1]=e[4],e[4]=n,n=e[2],e[2]=e[8],e[8]=n,n=e[6],e[6]=e[9],e[9]=n,n=e[3],e[3]=e[12],e[12]=n,n=e[7],e[7]=e[13],e[13]=n,n=e[11],e[11]=e[14],e[14]=n,this}setPosition(e,n,i){const r=this.elements;return e.isVector3?(r[12]=e.x,r[13]=e.y,r[14]=e.z):(r[12]=e,r[13]=n,r[14]=i),this}invert(){const e=this.elements,n=e[0],i=e[1],r=e[2],o=e[3],s=e[4],a=e[5],l=e[6],c=e[7],d=e[8],f=e[9],h=e[10],p=e[11],v=e[12],_=e[13],m=e[14],u=e[15],g=f*m*c-_*h*c+_*l*p-a*m*p-f*l*u+a*h*u,y=v*h*c-d*m*c-v*l*p+s*m*p+d*l*u-s*h*u,x=d*_*c-v*f*c+v*a*p-s*_*p-d*a*u+s*f*u,C=v*f*l-d*_*l-v*a*h+s*_*h+d*a*m-s*f*m,A=n*g+i*y+r*x+o*C;if(A===0)return this.set(0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0);const T=1/A;return e[0]=g*T,e[1]=(_*h*o-f*m*o-_*r*p+i*m*p+f*r*u-i*h*u)*T,e[2]=(a*m*o-_*l*o+_*r*c-i*m*c-a*r*u+i*l*u)*T,e[3]=(f*l*o-a*h*o-f*r*c+i*h*c+a*r*p-i*l*p)*T,e[4]=y*T,e[5]=(d*m*o-v*h*o+v*r*p-n*m*p-d*r*u+n*h*u)*T,e[6]=(v*l*o-s*m*o-v*r*c+n*m*c+s*r*u-n*l*u)*T,e[7]=(s*h*o-d*l*o+d*r*c-n*h*c-s*r*p+n*l*p)*T,e[8]=x*T,e[9]=(v*f*o-d*_*o-v*i*p+n*_*p+d*i*u-n*f*u)*T,e[10]=(s*_*o-v*a*o+v*i*c-n*_*c-s*i*u+n*a*u)*T,e[11]=(d*a*o-s*f*o-d*i*c+n*f*c+s*i*p-n*a*p)*T,e[12]=C*T,e[13]=(d*_*r-v*f*r+v*i*h-n*_*h-d*i*m+n*f*m)*T,e[14]=(v*a*r-s*_*r-v*i*l+n*_*l+s*i*m-n*a*m)*T,e[15]=(s*f*r-d*a*r+d*i*l-n*f*l-s*i*h+n*a*h)*T,this}scale(e){const n=this.elements,i=e.x,r=e.y,o=e.z;return n[0]*=i,n[4]*=r,n[8]*=o,n[1]*=i,n[5]*=r,n[9]*=o,n[2]*=i,n[6]*=r,n[10]*=o,n[3]*=i,n[7]*=r,n[11]*=o,this}getMaxScaleOnAxis(){const e=this.elements,n=e[0]*e[0]+e[1]*e[1]+e[2]*e[2],i=e[4]*e[4]+e[5]*e[5]+e[6]*e[6],r=e[8]*e[8]+e[9]*e[9]+e[10]*e[10];return Math.sqrt(Math.max(n,i,r))}makeTranslation(e,n,i){return e.isVector3?this.set(1,0,0,e.x,0,1,0,e.y,0,0,1,e.z,0,0,0,1):this.set(1,0,0,e,0,1,0,n,0,0,1,i,0,0,0,1),this}makeRotationX(e){const n=Math.cos(e),i=Math.sin(e);return this.set(1,0,0,0,0,n,-i,0,0,i,n,0,0,0,0,1),this}makeRotationY(e){const n=Math.cos(e),i=Math.sin(e);return this.set(n,0,i,0,0,1,0,0,-i,0,n,0,0,0,0,1),this}makeRotationZ(e){const n=Math.cos(e),i=Math.sin(e);return this.set(n,-i,0,0,i,n,0,0,0,0,1,0,0,0,0,1),this}makeRotationAxis(e,n){const i=Math.cos(n),r=Math.sin(n),o=1-i,s=e.x,a=e.y,l=e.z,c=o*s,d=o*a;return this.set(c*s+i,c*a-r*l,c*l+r*a,0,c*a+r*l,d*a+i,d*l-r*s,0,c*l-r*a,d*l+r*s,o*l*l+i,0,0,0,0,1),this}makeScale(e,n,i){return this.set(e,0,0,0,0,n,0,0,0,0,i,0,0,0,0,1),this}makeShear(e,n,i,r,o,s){return this.set(1,i,o,0,e,1,s,0,n,r,1,0,0,0,0,1),this}compose(e,n,i){const r=this.elements,o=n._x,s=n._y,a=n._z,l=n._w,c=o+o,d=s+s,f=a+a,h=o*c,p=o*d,v=o*f,_=s*d,m=s*f,u=a*f,g=l*c,y=l*d,x=l*f,C=i.x,A=i.y,T=i.z;return r[0]=(1-(_+u))*C,r[1]=(p+x)*C,r[2]=(v-y)*C,r[3]=0,r[4]=(p-x)*A,r[5]=(1-(h+u))*A,r[6]=(m+g)*A,r[7]=0,r[8]=(v+y)*T,r[9]=(m-g)*T,r[10]=(1-(h+_))*T,r[11]=0,r[12]=e.x,r[13]=e.y,r[14]=e.z,r[15]=1,this}decompose(e,n,i){const r=this.elements;let o=$r.set(r[0],r[1],r[2]).length();const s=$r.set(r[4],r[5],r[6]).length(),a=$r.set(r[8],r[9],r[10]).length();this.determinant()<0&&(o=-o),e.x=r[12],e.y=r[13],e.z=r[14],zn.copy(this);const c=1/o,d=1/s,f=1/a;return zn.elements[0]*=c,zn.elements[1]*=c,zn.elements[2]*=c,zn.elements[4]*=d,zn.elements[5]*=d,zn.elements[6]*=d,zn.elements[8]*=f,zn.elements[9]*=f,zn.elements[10]*=f,n.setFromRotationMatrix(zn),i.x=o,i.y=s,i.z=a,this}makePerspective(e,n,i,r,o,s,a=_i){const l=this.elements,c=2*o/(n-e),d=2*o/(i-r),f=(n+e)/(n-e),h=(i+r)/(i-r);let p,v;if(a===_i)p=-(s+o)/(s-o),v=-2*s*o/(s-o);else if(a===Jl)p=-s/(s-o),v=-s*o/(s-o);else throw new Error("THREE.Matrix4.makePerspective(): Invalid coordinate system: "+a);return l[0]=c,l[4]=0,l[8]=f,l[12]=0,l[1]=0,l[5]=d,l[9]=h,l[13]=0,l[2]=0,l[6]=0,l[10]=p,l[14]=v,l[3]=0,l[7]=0,l[11]=-1,l[15]=0,this}makeOrthographic(e,n,i,r,o,s,a=_i){const l=this.elements,c=1/(n-e),d=1/(i-r),f=1/(s-o),h=(n+e)*c,p=(i+r)*d;let v,_;if(a===_i)v=(s+o)*f,_=-2*f;else if(a===Jl)v=o*f,_=-1*f;else throw new Error("THREE.Matrix4.makeOrthographic(): Invalid coordinate system: "+a);return l[0]=2*c,l[4]=0,l[8]=0,l[12]=-h,l[1]=0,l[5]=2*d,l[9]=0,l[13]=-p,l[2]=0,l[6]=0,l[10]=_,l[14]=-v,l[3]=0,l[7]=0,l[11]=0,l[15]=1,this}equals(e){const n=this.elements,i=e.elements;for(let r=0;r<16;r++)if(n[r]!==i[r])return!1;return!0}fromArray(e,n=0){for(let i=0;i<16;i++)this.elements[i]=e[i+n];return this}toArray(e=[],n=0){const i=this.elements;return e[n]=i[0],e[n+1]=i[1],e[n+2]=i[2],e[n+3]=i[3],e[n+4]=i[4],e[n+5]=i[5],e[n+6]=i[6],e[n+7]=i[7],e[n+8]=i[8],e[n+9]=i[9],e[n+10]=i[10],e[n+11]=i[11],e[n+12]=i[12],e[n+13]=i[13],e[n+14]=i[14],e[n+15]=i[15],e}}const $r=new L,zn=new rt,wM=new L(0,0,0),EM=new L(1,1,1),Ii=new L,Xa=new L,fn=new L,fm=new rt,pm=new es;class $n{constructor(e=0,n=0,i=0,r=$n.DEFAULT_ORDER){this.isEuler=!0,this._x=e,this._y=n,this._z=i,this._order=r}get x(){return this._x}set x(e){this._x=e,this._onChangeCallback()}get y(){return this._y}set y(e){this._y=e,this._onChangeCallback()}get z(){return this._z}set z(e){this._z=e,this._onChangeCallback()}get order(){return this._order}set order(e){this._order=e,this._onChangeCallback()}set(e,n,i,r=this._order){return this._x=e,this._y=n,this._z=i,this._order=r,this._onChangeCallback(),this}clone(){return new this.constructor(this._x,this._y,this._z,this._order)}copy(e){return this._x=e._x,this._y=e._y,this._z=e._z,this._order=e._order,this._onChangeCallback(),this}setFromRotationMatrix(e,n=this._order,i=!0){const r=e.elements,o=r[0],s=r[4],a=r[8],l=r[1],c=r[5],d=r[9],f=r[2],h=r[6],p=r[10];switch(n){case"XYZ":this._y=Math.asin(Gt(a,-1,1)),Math.abs(a)<.9999999?(this._x=Math.atan2(-d,p),this._z=Math.atan2(-s,o)):(this._x=Math.atan2(h,c),this._z=0);break;case"YXZ":this._x=Math.asin(-Gt(d,-1,1)),Math.abs(d)<.9999999?(this._y=Math.atan2(a,p),this._z=Math.atan2(l,c)):(this._y=Math.atan2(-f,o),this._z=0);break;case"ZXY":this._x=Math.asin(Gt(h,-1,1)),Math.abs(h)<.9999999?(this._y=Math.atan2(-f,p),this._z=Math.atan2(-s,c)):(this._y=0,this._z=Math.atan2(l,o));break;case"ZYX":this._y=Math.asin(-Gt(f,-1,1)),Math.abs(f)<.9999999?(this._x=Math.atan2(h,p),this._z=Math.atan2(l,o)):(this._x=0,this._z=Math.atan2(-s,c));break;case"YZX":this._z=Math.asin(Gt(l,-1,1)),Math.abs(l)<.9999999?(this._x=Math.atan2(-d,c),this._y=Math.atan2(-f,o)):(this._x=0,this._y=Math.atan2(a,p));break;case"XZY":this._z=Math.asin(-Gt(s,-1,1)),Math.abs(s)<.9999999?(this._x=Math.atan2(h,c),this._y=Math.atan2(a,o)):(this._x=Math.atan2(-d,p),this._y=0);break;default:console.warn("THREE.Euler: .setFromRotationMatrix() encountered an unknown order: "+n)}return this._order=n,i===!0&&this._onChangeCallback(),this}setFromQuaternion(e,n,i){return fm.makeRotationFromQuaternion(e),this.setFromRotationMatrix(fm,n,i)}setFromVector3(e,n=this._order){return this.set(e.x,e.y,e.z,n)}reorder(e){return pm.setFromEuler(this),this.setFromQuaternion(pm,e)}equals(e){return e._x===this._x&&e._y===this._y&&e._z===this._z&&e._order===this._order}fromArray(e){return this._x=e[0],this._y=e[1],this._z=e[2],e[3]!==void 0&&(this._order=e[3]),this._onChangeCallback(),this}toArray(e=[],n=0){return e[n]=this._x,e[n+1]=this._y,e[n+2]=this._z,e[n+3]=this._order,e}_onChange(e){return this._onChangeCallback=e,this}_onChangeCallback(){}*[Symbol.iterator](){yield this._x,yield this._y,yield this._z,yield this._order}}$n.DEFAULT_ORDER="XYZ";class ey{constructor(){this.mask=1}set(e){this.mask=(1<<e|0)>>>0}enable(e){this.mask|=1<<e|0}enableAll(){this.mask=-1}toggle(e){this.mask^=1<<e|0}disable(e){this.mask&=~(1<<e|0)}disableAll(){this.mask=0}test(e){return(this.mask&e.mask)!==0}isEnabled(e){return(this.mask&(1<<e|0))!==0}}let TM=0;const mm=new L,Kr=new es,ui=new rt,ja=new L,gs=new L,AM=new L,CM=new es,gm=new L(1,0,0),vm=new L(0,1,0),ym=new L(0,0,1),_m={type:"added"},bM={type:"removed"},Zr={type:"childadded",child:null},hu={type:"childremoved",child:null};class Xt extends Qo{constructor(){super(),this.isObject3D=!0,Object.defineProperty(this,"id",{value:TM++}),this.uuid=ga(),this.name="",this.type="Object3D",this.parent=null,this.children=[],this.up=Xt.DEFAULT_UP.clone();const e=new L,n=new $n,i=new es,r=new L(1,1,1);function o(){i.setFromEuler(n,!1)}function s(){n.setFromQuaternion(i,void 0,!1)}n._onChange(o),i._onChange(s),Object.defineProperties(this,{position:{configurable:!0,enumerable:!0,value:e},rotation:{configurable:!0,enumerable:!0,value:n},quaternion:{configurable:!0,enumerable:!0,value:i},scale:{configurable:!0,enumerable:!0,value:r},modelViewMatrix:{value:new rt},normalMatrix:{value:new Be}}),this.matrix=new rt,this.matrixWorld=new rt,this.matrixAutoUpdate=Xt.DEFAULT_MATRIX_AUTO_UPDATE,this.matrixWorldAutoUpdate=Xt.DEFAULT_MATRIX_WORLD_AUTO_UPDATE,this.matrixWorldNeedsUpdate=!1,this.layers=new ey,this.visible=!0,this.castShadow=!1,this.receiveShadow=!1,this.frustumCulled=!0,this.renderOrder=0,this.animations=[],this.userData={}}onBeforeShadow(){}onAfterShadow(){}onBeforeRender(){}onAfterRender(){}applyMatrix4(e){this.matrixAutoUpdate&&this.updateMatrix(),this.matrix.premultiply(e),this.matrix.decompose(this.position,this.quaternion,this.scale)}applyQuaternion(e){return this.quaternion.premultiply(e),this}setRotationFromAxisAngle(e,n){this.quaternion.setFromAxisAngle(e,n)}setRotationFromEuler(e){this.quaternion.setFromEuler(e,!0)}setRotationFromMatrix(e){this.quaternion.setFromRotationMatrix(e)}setRotationFromQuaternion(e){this.quaternion.copy(e)}rotateOnAxis(e,n){return Kr.setFromAxisAngle(e,n),this.quaternion.multiply(Kr),this}rotateOnWorldAxis(e,n){return Kr.setFromAxisAngle(e,n),this.quaternion.premultiply(Kr),this}rotateX(e){return this.rotateOnAxis(gm,e)}rotateY(e){return this.rotateOnAxis(vm,e)}rotateZ(e){return this.rotateOnAxis(ym,e)}translateOnAxis(e,n){return mm.copy(e).applyQuaternion(this.quaternion),this.position.add(mm.multiplyScalar(n)),this}translateX(e){return this.translateOnAxis(gm,e)}translateY(e){return this.translateOnAxis(vm,e)}translateZ(e){return this.translateOnAxis(ym,e)}localToWorld(e){return this.updateWorldMatrix(!0,!1),e.applyMatrix4(this.matrixWorld)}worldToLocal(e){return this.updateWorldMatrix(!0,!1),e.applyMatrix4(ui.copy(this.matrixWorld).invert())}lookAt(e,n,i){e.isVector3?ja.copy(e):ja.set(e,n,i);const r=this.parent;this.updateWorldMatrix(!0,!1),gs.setFromMatrixPosition(this.matrixWorld),this.isCamera||this.isLight?ui.lookAt(gs,ja,this.up):ui.lookAt(ja,gs,this.up),this.quaternion.setFromRotationMatrix(ui),r&&(ui.extractRotation(r.matrixWorld),Kr.setFromRotationMatrix(ui),this.quaternion.premultiply(Kr.invert()))}add(e){if(arguments.length>1){for(let n=0;n<arguments.length;n++)this.add(arguments[n]);return this}return e===this?(console.error("THREE.Object3D.add: object can't be added as a child of itself.",e),this):(e&&e.isObject3D?(e.removeFromParent(),e.parent=this,this.children.push(e),e.dispatchEvent(_m),Zr.child=e,this.dispatchEvent(Zr),Zr.child=null):console.error("THREE.Object3D.add: object not an instance of THREE.Object3D.",e),this)}remove(e){if(arguments.length>1){for(let i=0;i<arguments.length;i++)this.remove(arguments[i]);return this}const n=this.children.indexOf(e);return n!==-1&&(e.parent=null,this.children.splice(n,1),e.dispatchEvent(bM),hu.child=e,this.dispatchEvent(hu),hu.child=null),this}removeFromParent(){const e=this.parent;return e!==null&&e.remove(this),this}clear(){return this.remove(...this.children)}attach(e){return this.updateWorldMatrix(!0,!1),ui.copy(this.matrixWorld).invert(),e.parent!==null&&(e.parent.updateWorldMatrix(!0,!1),ui.multiply(e.parent.matrixWorld)),e.applyMatrix4(ui),e.removeFromParent(),e.parent=this,this.children.push(e),e.updateWorldMatrix(!1,!0),e.dispatchEvent(_m),Zr.child=e,this.dispatchEvent(Zr),Zr.child=null,this}getObjectById(e){return this.getObjectByProperty("id",e)}getObjectByName(e){return this.getObjectByProperty("name",e)}getObjectByProperty(e,n){if(this[e]===n)return this;for(let i=0,r=this.children.length;i<r;i++){const s=this.children[i].getObjectByProperty(e,n);if(s!==void 0)return s}}getObjectsByProperty(e,n,i=[]){this[e]===n&&i.push(this);const r=this.children;for(let o=0,s=r.length;o<s;o++)r[o].getObjectsByProperty(e,n,i);return i}getWorldPosition(e){return this.updateWorldMatrix(!0,!1),e.setFromMatrixPosition(this.matrixWorld)}getWorldQuaternion(e){return this.updateWorldMatrix(!0,!1),this.matrixWorld.decompose(gs,e,AM),e}getWorldScale(e){return this.updateWorldMatrix(!0,!1),this.matrixWorld.decompose(gs,CM,e),e}getWorldDirection(e){this.updateWorldMatrix(!0,!1);const n=this.matrixWorld.elements;return e.set(n[8],n[9],n[10]).normalize()}raycast(){}traverse(e){e(this);const n=this.children;for(let i=0,r=n.length;i<r;i++)n[i].traverse(e)}traverseVisible(e){if(this.visible===!1)return;e(this);const n=this.children;for(let i=0,r=n.length;i<r;i++)n[i].traverseVisible(e)}traverseAncestors(e){const n=this.parent;n!==null&&(e(n),n.traverseAncestors(e))}updateMatrix(){this.matrix.compose(this.position,this.quaternion,this.scale),this.matrixWorldNeedsUpdate=!0}updateMatrixWorld(e){this.matrixAutoUpdate&&this.updateMatrix(),(this.matrixWorldNeedsUpdate||e)&&(this.matrixWorldAutoUpdate===!0&&(this.parent===null?this.matrixWorld.copy(this.matrix):this.matrixWorld.multiplyMatrices(this.parent.matrixWorld,this.matrix)),this.matrixWorldNeedsUpdate=!1,e=!0);const n=this.children;for(let i=0,r=n.length;i<r;i++)n[i].updateMatrixWorld(e)}updateWorldMatrix(e,n){const i=this.parent;if(e===!0&&i!==null&&i.updateWorldMatrix(!0,!1),this.matrixAutoUpdate&&this.updateMatrix(),this.matrixWorldAutoUpdate===!0&&(this.parent===null?this.matrixWorld.copy(this.matrix):this.matrixWorld.multiplyMatrices(this.parent.matrixWorld,this.matrix)),n===!0){const r=this.children;for(let o=0,s=r.length;o<s;o++)r[o].updateWorldMatrix(!1,!0)}}toJSON(e){const n=e===void 0||typeof e=="string",i={};n&&(e={geometries:{},materials:{},textures:{},images:{},shapes:{},skeletons:{},animations:{},nodes:{}},i.metadata={version:4.6,type:"Object",generator:"Object3D.toJSON"});const r={};r.uuid=this.uuid,r.type=this.type,this.name!==""&&(r.name=this.name),this.castShadow===!0&&(r.castShadow=!0),this.receiveShadow===!0&&(r.receiveShadow=!0),this.visible===!1&&(r.visible=!1),this.frustumCulled===!1&&(r.frustumCulled=!1),this.renderOrder!==0&&(r.renderOrder=this.renderOrder),Object.keys(this.userData).length>0&&(r.userData=this.userData),r.layers=this.layers.mask,r.matrix=this.matrix.toArray(),r.up=this.up.toArray(),this.matrixAutoUpdate===!1&&(r.matrixAutoUpdate=!1),this.isInstancedMesh&&(r.type="InstancedMesh",r.count=this.count,r.instanceMatrix=this.instanceMatrix.toJSON(),this.instanceColor!==null&&(r.instanceColor=this.instanceColor.toJSON())),this.isBatchedMesh&&(r.type="BatchedMesh",r.perObjectFrustumCulled=this.perObjectFrustumCulled,r.sortObjects=this.sortObjects,r.drawRanges=this._drawRanges,r.reservedRanges=this._reservedRanges,r.visibility=this._visibility,r.active=this._active,r.bounds=this._bounds.map(a=>({boxInitialized:a.boxInitialized,boxMin:a.box.min.toArray(),boxMax:a.box.max.toArray(),sphereInitialized:a.sphereInitialized,sphereRadius:a.sphere.radius,sphereCenter:a.sphere.center.toArray()})),r.maxInstanceCount=this._maxInstanceCount,r.maxVertexCount=this._maxVertexCount,r.maxIndexCount=this._maxIndexCount,r.geometryInitialized=this._geometryInitialized,r.geometryCount=this._geometryCount,r.matricesTexture=this._matricesTexture.toJSON(e),this._colorsTexture!==null&&(r.colorsTexture=this._colorsTexture.toJSON(e)),this.boundingSphere!==null&&(r.boundingSphere={center:r.boundingSphere.center.toArray(),radius:r.boundingSphere.radius}),this.boundingBox!==null&&(r.boundingBox={min:r.boundingBox.min.toArray(),max:r.boundingBox.max.toArray()}));function o(a,l){return a[l.uuid]===void 0&&(a[l.uuid]=l.toJSON(e)),l.uuid}if(this.isScene)this.background&&(this.background.isColor?r.background=this.background.toJSON():this.background.isTexture&&(r.background=this.background.toJSON(e).uuid)),this.environment&&this.environment.isTexture&&this.environment.isRenderTargetTexture!==!0&&(r.environment=this.environment.toJSON(e).uuid);else if(this.isMesh||this.isLine||this.isPoints){r.geometry=o(e.geometries,this.geometry);const a=this.geometry.parameters;if(a!==void 0&&a.shapes!==void 0){const l=a.shapes;if(Array.isArray(l))for(let c=0,d=l.length;c<d;c++){const f=l[c];o(e.shapes,f)}else o(e.shapes,l)}}if(this.isSkinnedMesh&&(r.bindMode=this.bindMode,r.bindMatrix=this.bindMatrix.toArray(),this.skeleton!==void 0&&(o(e.skeletons,this.skeleton),r.skeleton=this.skeleton.uuid)),this.material!==void 0)if(Array.isArray(this.material)){const a=[];for(let l=0,c=this.material.length;l<c;l++)a.push(o(e.materials,this.material[l]));r.material=a}else r.material=o(e.materials,this.material);if(this.children.length>0){r.children=[];for(let a=0;a<this.children.length;a++)r.children.push(this.children[a].toJSON(e).object)}if(this.animations.length>0){r.animations=[];for(let a=0;a<this.animations.length;a++){const l=this.animations[a];r.animations.push(o(e.animations,l))}}if(n){const a=s(e.geometries),l=s(e.materials),c=s(e.textures),d=s(e.images),f=s(e.shapes),h=s(e.skeletons),p=s(e.animations),v=s(e.nodes);a.length>0&&(i.geometries=a),l.length>0&&(i.materials=l),c.length>0&&(i.textures=c),d.length>0&&(i.images=d),f.length>0&&(i.shapes=f),h.length>0&&(i.skeletons=h),p.length>0&&(i.animations=p),v.length>0&&(i.nodes=v)}return i.object=r,i;function s(a){const l=[];for(const c in a){const d=a[c];delete d.metadata,l.push(d)}return l}}clone(e){return new this.constructor().copy(this,e)}copy(e,n=!0){if(this.name=e.name,this.up.copy(e.up),this.position.copy(e.position),this.rotation.order=e.rotation.order,this.quaternion.copy(e.quaternion),this.scale.copy(e.scale),this.matrix.copy(e.matrix),this.matrixWorld.copy(e.matrixWorld),this.matrixAutoUpdate=e.matrixAutoUpdate,this.matrixWorldAutoUpdate=e.matrixWorldAutoUpdate,this.matrixWorldNeedsUpdate=e.matrixWorldNeedsUpdate,this.layers.mask=e.layers.mask,this.visible=e.visible,this.castShadow=e.castShadow,this.receiveShadow=e.receiveShadow,this.frustumCulled=e.frustumCulled,this.renderOrder=e.renderOrder,this.animations=e.animations.slice(),this.userData=JSON.parse(JSON.stringify(e.userData)),n===!0)for(let i=0;i<e.children.length;i++){const r=e.children[i];this.add(r.clone())}return this}}Xt.DEFAULT_UP=new L(0,1,0);Xt.DEFAULT_MATRIX_AUTO_UPDATE=!0;Xt.DEFAULT_MATRIX_WORLD_AUTO_UPDATE=!0;const Bn=new L,di=new L,fu=new L,hi=new L,Jr=new L,Qr=new L,xm=new L,pu=new L,mu=new L,gu=new L,vu=new St,yu=new St,_u=new St;class Wn{constructor(e=new L,n=new L,i=new L){this.a=e,this.b=n,this.c=i}static getNormal(e,n,i,r){r.subVectors(i,n),Bn.subVectors(e,n),r.cross(Bn);const o=r.lengthSq();return o>0?r.multiplyScalar(1/Math.sqrt(o)):r.set(0,0,0)}static getBarycoord(e,n,i,r,o){Bn.subVectors(r,n),di.subVectors(i,n),fu.subVectors(e,n);const s=Bn.dot(Bn),a=Bn.dot(di),l=Bn.dot(fu),c=di.dot(di),d=di.dot(fu),f=s*c-a*a;if(f===0)return o.set(0,0,0),null;const h=1/f,p=(c*l-a*d)*h,v=(s*d-a*l)*h;return o.set(1-p-v,v,p)}static containsPoint(e,n,i,r){return this.getBarycoord(e,n,i,r,hi)===null?!1:hi.x>=0&&hi.y>=0&&hi.x+hi.y<=1}static getInterpolation(e,n,i,r,o,s,a,l){return this.getBarycoord(e,n,i,r,hi)===null?(l.x=0,l.y=0,"z"in l&&(l.z=0),"w"in l&&(l.w=0),null):(l.setScalar(0),l.addScaledVector(o,hi.x),l.addScaledVector(s,hi.y),l.addScaledVector(a,hi.z),l)}static getInterpolatedAttribute(e,n,i,r,o,s){return vu.setScalar(0),yu.setScalar(0),_u.setScalar(0),vu.fromBufferAttribute(e,n),yu.fromBufferAttribute(e,i),_u.fromBufferAttribute(e,r),s.setScalar(0),s.addScaledVector(vu,o.x),s.addScaledVector(yu,o.y),s.addScaledVector(_u,o.z),s}static isFrontFacing(e,n,i,r){return Bn.subVectors(i,n),di.subVectors(e,n),Bn.cross(di).dot(r)<0}set(e,n,i){return this.a.copy(e),this.b.copy(n),this.c.copy(i),this}setFromPointsAndIndices(e,n,i,r){return this.a.copy(e[n]),this.b.copy(e[i]),this.c.copy(e[r]),this}setFromAttributeAndIndices(e,n,i,r){return this.a.fromBufferAttribute(e,n),this.b.fromBufferAttribute(e,i),this.c.fromBufferAttribute(e,r),this}clone(){return new this.constructor().copy(this)}copy(e){return this.a.copy(e.a),this.b.copy(e.b),this.c.copy(e.c),this}getArea(){return Bn.subVectors(this.c,this.b),di.subVectors(this.a,this.b),Bn.cross(di).length()*.5}getMidpoint(e){return e.addVectors(this.a,this.b).add(this.c).multiplyScalar(1/3)}getNormal(e){return Wn.getNormal(this.a,this.b,this.c,e)}getPlane(e){return e.setFromCoplanarPoints(this.a,this.b,this.c)}getBarycoord(e,n){return Wn.getBarycoord(e,this.a,this.b,this.c,n)}getInterpolation(e,n,i,r,o){return Wn.getInterpolation(e,this.a,this.b,this.c,n,i,r,o)}containsPoint(e){return Wn.containsPoint(e,this.a,this.b,this.c)}isFrontFacing(e){return Wn.isFrontFacing(this.a,this.b,this.c,e)}intersectsBox(e){return e.intersectsTriangle(this)}closestPointToPoint(e,n){const i=this.a,r=this.b,o=this.c;let s,a;Jr.subVectors(r,i),Qr.subVectors(o,i),pu.subVectors(e,i);const l=Jr.dot(pu),c=Qr.dot(pu);if(l<=0&&c<=0)return n.copy(i);mu.subVectors(e,r);const d=Jr.dot(mu),f=Qr.dot(mu);if(d>=0&&f<=d)return n.copy(r);const h=l*f-d*c;if(h<=0&&l>=0&&d<=0)return s=l/(l-d),n.copy(i).addScaledVector(Jr,s);gu.subVectors(e,o);const p=Jr.dot(gu),v=Qr.dot(gu);if(v>=0&&p<=v)return n.copy(o);const _=p*c-l*v;if(_<=0&&c>=0&&v<=0)return a=c/(c-v),n.copy(i).addScaledVector(Qr,a);const m=d*v-p*f;if(m<=0&&f-d>=0&&p-v>=0)return xm.subVectors(o,r),a=(f-d)/(f-d+(p-v)),n.copy(r).addScaledVector(xm,a);const u=1/(m+_+h);return s=_*u,a=h*u,n.copy(i).addScaledVector(Jr,s).addScaledVector(Qr,a)}equals(e){return e.a.equals(this.a)&&e.b.equals(this.b)&&e.c.equals(this.c)}}const ty={aliceblue:15792383,antiquewhite:16444375,aqua:65535,aquamarine:8388564,azure:15794175,beige:16119260,bisque:16770244,black:0,blanchedalmond:16772045,blue:255,blueviolet:9055202,brown:10824234,burlywood:14596231,cadetblue:6266528,chartreuse:8388352,chocolate:13789470,coral:16744272,cornflowerblue:6591981,cornsilk:16775388,crimson:14423100,cyan:65535,darkblue:139,darkcyan:35723,darkgoldenrod:12092939,darkgray:11119017,darkgreen:25600,darkgrey:11119017,darkkhaki:12433259,darkmagenta:9109643,darkolivegreen:5597999,darkorange:16747520,darkorchid:10040012,darkred:9109504,darksalmon:15308410,darkseagreen:9419919,darkslateblue:4734347,darkslategray:3100495,darkslategrey:3100495,darkturquoise:52945,darkviolet:9699539,deeppink:16716947,deepskyblue:49151,dimgray:6908265,dimgrey:6908265,dodgerblue:2003199,firebrick:11674146,floralwhite:16775920,forestgreen:2263842,fuchsia:16711935,gainsboro:14474460,ghostwhite:16316671,gold:16766720,goldenrod:14329120,gray:8421504,green:32768,greenyellow:11403055,grey:8421504,honeydew:15794160,hotpink:16738740,indianred:13458524,indigo:4915330,ivory:16777200,khaki:15787660,lavender:15132410,lavenderblush:16773365,lawngreen:8190976,lemonchiffon:16775885,lightblue:11393254,lightcoral:15761536,lightcyan:14745599,lightgoldenrodyellow:16448210,lightgray:13882323,lightgreen:9498256,lightgrey:13882323,lightpink:16758465,lightsalmon:16752762,lightseagreen:2142890,lightskyblue:8900346,lightslategray:7833753,lightslategrey:7833753,lightsteelblue:11584734,lightyellow:16777184,lime:65280,limegreen:3329330,linen:16445670,magenta:16711935,maroon:8388608,mediumaquamarine:6737322,mediumblue:205,mediumorchid:12211667,mediumpurple:9662683,mediumseagreen:3978097,mediumslateblue:8087790,mediumspringgreen:64154,mediumturquoise:4772300,mediumvioletred:13047173,midnightblue:1644912,mintcream:16121850,mistyrose:16770273,moccasin:16770229,navajowhite:16768685,navy:128,oldlace:16643558,olive:8421376,olivedrab:7048739,orange:16753920,orangered:16729344,orchid:14315734,palegoldenrod:15657130,palegreen:10025880,paleturquoise:11529966,palevioletred:14381203,papayawhip:16773077,peachpuff:16767673,peru:13468991,pink:16761035,plum:14524637,powderblue:11591910,purple:8388736,rebeccapurple:6697881,red:16711680,rosybrown:12357519,royalblue:4286945,saddlebrown:9127187,salmon:16416882,sandybrown:16032864,seagreen:3050327,seashell:16774638,sienna:10506797,silver:12632256,skyblue:8900331,slateblue:6970061,slategray:7372944,slategrey:7372944,snow:16775930,springgreen:65407,steelblue:4620980,tan:13808780,teal:32896,thistle:14204888,tomato:16737095,turquoise:4251856,violet:15631086,wheat:16113331,white:16777215,whitesmoke:16119285,yellow:16776960,yellowgreen:10145074},ki={h:0,s:0,l:0},Ya={h:0,s:0,l:0};function xu(t,e,n){return n<0&&(n+=1),n>1&&(n-=1),n<1/6?t+(e-t)*6*n:n<1/2?e:n<2/3?t+(e-t)*6*(2/3-n):t}class we{constructor(e,n,i){return this.isColor=!0,this.r=1,this.g=1,this.b=1,this.set(e,n,i)}set(e,n,i){if(n===void 0&&i===void 0){const r=e;r&&r.isColor?this.copy(r):typeof r=="number"?this.setHex(r):typeof r=="string"&&this.setStyle(r)}else this.setRGB(e,n,i);return this}setScalar(e){return this.r=e,this.g=e,this.b=e,this}setHex(e,n=gn){return e=Math.floor(e),this.r=(e>>16&255)/255,this.g=(e>>8&255)/255,this.b=(e&255)/255,$e.toWorkingColorSpace(this,n),this}setRGB(e,n,i,r=$e.workingColorSpace){return this.r=e,this.g=n,this.b=i,$e.toWorkingColorSpace(this,r),this}setHSL(e,n,i,r=$e.workingColorSpace){if(e=dM(e,1),n=Gt(n,0,1),i=Gt(i,0,1),n===0)this.r=this.g=this.b=i;else{const o=i<=.5?i*(1+n):i+n-i*n,s=2*i-o;this.r=xu(s,o,e+1/3),this.g=xu(s,o,e),this.b=xu(s,o,e-1/3)}return $e.toWorkingColorSpace(this,r),this}setStyle(e,n=gn){function i(o){o!==void 0&&parseFloat(o)<1&&console.warn("THREE.Color: Alpha component of "+e+" will be ignored.")}let r;if(r=/^(\w+)\(([^\)]*)\)/.exec(e)){let o;const s=r[1],a=r[2];switch(s){case"rgb":case"rgba":if(o=/^\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*(?:,\s*(\d*\.?\d+)\s*)?$/.exec(a))return i(o[4]),this.setRGB(Math.min(255,parseInt(o[1],10))/255,Math.min(255,parseInt(o[2],10))/255,Math.min(255,parseInt(o[3],10))/255,n);if(o=/^\s*(\d+)\%\s*,\s*(\d+)\%\s*,\s*(\d+)\%\s*(?:,\s*(\d*\.?\d+)\s*)?$/.exec(a))return i(o[4]),this.setRGB(Math.min(100,parseInt(o[1],10))/100,Math.min(100,parseInt(o[2],10))/100,Math.min(100,parseInt(o[3],10))/100,n);break;case"hsl":case"hsla":if(o=/^\s*(\d*\.?\d+)\s*,\s*(\d*\.?\d+)\%\s*,\s*(\d*\.?\d+)\%\s*(?:,\s*(\d*\.?\d+)\s*)?$/.exec(a))return i(o[4]),this.setHSL(parseFloat(o[1])/360,parseFloat(o[2])/100,parseFloat(o[3])/100,n);break;default:console.warn("THREE.Color: Unknown color model "+e)}}else if(r=/^\#([A-Fa-f\d]+)$/.exec(e)){const o=r[1],s=o.length;if(s===3)return this.setRGB(parseInt(o.charAt(0),16)/15,parseInt(o.charAt(1),16)/15,parseInt(o.charAt(2),16)/15,n);if(s===6)return this.setHex(parseInt(o,16),n);console.warn("THREE.Color: Invalid hex color "+e)}else if(e&&e.length>0)return this.setColorName(e,n);return this}setColorName(e,n=gn){const i=ty[e.toLowerCase()];return i!==void 0?this.setHex(i,n):console.warn("THREE.Color: Unknown color "+e),this}clone(){return new this.constructor(this.r,this.g,this.b)}copy(e){return this.r=e.r,this.g=e.g,this.b=e.b,this}copySRGBToLinear(e){return this.r=Si(e.r),this.g=Si(e.g),this.b=Si(e.b),this}copyLinearToSRGB(e){return this.r=No(e.r),this.g=No(e.g),this.b=No(e.b),this}convertSRGBToLinear(){return this.copySRGBToLinear(this),this}convertLinearToSRGB(){return this.copyLinearToSRGB(this),this}getHex(e=gn){return $e.fromWorkingColorSpace(Ht.copy(this),e),Math.round(Gt(Ht.r*255,0,255))*65536+Math.round(Gt(Ht.g*255,0,255))*256+Math.round(Gt(Ht.b*255,0,255))}getHexString(e=gn){return("000000"+this.getHex(e).toString(16)).slice(-6)}getHSL(e,n=$e.workingColorSpace){$e.fromWorkingColorSpace(Ht.copy(this),n);const i=Ht.r,r=Ht.g,o=Ht.b,s=Math.max(i,r,o),a=Math.min(i,r,o);let l,c;const d=(a+s)/2;if(a===s)l=0,c=0;else{const f=s-a;switch(c=d<=.5?f/(s+a):f/(2-s-a),s){case i:l=(r-o)/f+(r<o?6:0);break;case r:l=(o-i)/f+2;break;case o:l=(i-r)/f+4;break}l/=6}return e.h=l,e.s=c,e.l=d,e}getRGB(e,n=$e.workingColorSpace){return $e.fromWorkingColorSpace(Ht.copy(this),n),e.r=Ht.r,e.g=Ht.g,e.b=Ht.b,e}getStyle(e=gn){$e.fromWorkingColorSpace(Ht.copy(this),e);const n=Ht.r,i=Ht.g,r=Ht.b;return e!==gn?`color(${e} ${n.toFixed(3)} ${i.toFixed(3)} ${r.toFixed(3)})`:`rgb(${Math.round(n*255)},${Math.round(i*255)},${Math.round(r*255)})`}offsetHSL(e,n,i){return this.getHSL(ki),this.setHSL(ki.h+e,ki.s+n,ki.l+i)}add(e){return this.r+=e.r,this.g+=e.g,this.b+=e.b,this}addColors(e,n){return this.r=e.r+n.r,this.g=e.g+n.g,this.b=e.b+n.b,this}addScalar(e){return this.r+=e,this.g+=e,this.b+=e,this}sub(e){return this.r=Math.max(0,this.r-e.r),this.g=Math.max(0,this.g-e.g),this.b=Math.max(0,this.b-e.b),this}multiply(e){return this.r*=e.r,this.g*=e.g,this.b*=e.b,this}multiplyScalar(e){return this.r*=e,this.g*=e,this.b*=e,this}lerp(e,n){return this.r+=(e.r-this.r)*n,this.g+=(e.g-this.g)*n,this.b+=(e.b-this.b)*n,this}lerpColors(e,n,i){return this.r=e.r+(n.r-e.r)*i,this.g=e.g+(n.g-e.g)*i,this.b=e.b+(n.b-e.b)*i,this}lerpHSL(e,n){this.getHSL(ki),e.getHSL(Ya);const i=iu(ki.h,Ya.h,n),r=iu(ki.s,Ya.s,n),o=iu(ki.l,Ya.l,n);return this.setHSL(i,r,o),this}setFromVector3(e){return this.r=e.x,this.g=e.y,this.b=e.z,this}applyMatrix3(e){const n=this.r,i=this.g,r=this.b,o=e.elements;return this.r=o[0]*n+o[3]*i+o[6]*r,this.g=o[1]*n+o[4]*i+o[7]*r,this.b=o[2]*n+o[5]*i+o[8]*r,this}equals(e){return e.r===this.r&&e.g===this.g&&e.b===this.b}fromArray(e,n=0){return this.r=e[n],this.g=e[n+1],this.b=e[n+2],this}toArray(e=[],n=0){return e[n]=this.r,e[n+1]=this.g,e[n+2]=this.b,e}fromBufferAttribute(e,n){return this.r=e.getX(n),this.g=e.getY(n),this.b=e.getZ(n),this}toJSON(){return this.getHex()}*[Symbol.iterator](){yield this.r,yield this.g,yield this.b}}const Ht=new we;we.NAMES=ty;let RM=0;class ya extends Qo{static get type(){return"Material"}get type(){return this.constructor.type}set type(e){}constructor(){super(),this.isMaterial=!0,Object.defineProperty(this,"id",{value:RM++}),this.uuid=ga(),this.name="",this.blending=Do,this.side=sr,this.vertexColors=!1,this.opacity=1,this.transparent=!1,this.alphaHash=!1,this.blendSrc=Nd,this.blendDst=Id,this.blendEquation=Mr,this.blendSrcAlpha=null,this.blendDstAlpha=null,this.blendEquationAlpha=null,this.blendColor=new we(0,0,0),this.blendAlpha=0,this.depthFunc=Ho,this.depthTest=!0,this.depthWrite=!0,this.stencilWriteMask=255,this.stencilFunc=im,this.stencilRef=0,this.stencilFuncMask=255,this.stencilFail=Wr,this.stencilZFail=Wr,this.stencilZPass=Wr,this.stencilWrite=!1,this.clippingPlanes=null,this.clipIntersection=!1,this.clipShadows=!1,this.shadowSide=null,this.colorWrite=!0,this.precision=null,this.polygonOffset=!1,this.polygonOffsetFactor=0,this.polygonOffsetUnits=0,this.dithering=!1,this.alphaToCoverage=!1,this.premultipliedAlpha=!1,this.forceSinglePass=!1,this.visible=!0,this.toneMapped=!0,this.userData={},this.version=0,this._alphaTest=0}get alphaTest(){return this._alphaTest}set alphaTest(e){this._alphaTest>0!=e>0&&this.version++,this._alphaTest=e}onBeforeRender(){}onBeforeCompile(){}customProgramCacheKey(){return this.onBeforeCompile.toString()}setValues(e){if(e!==void 0)for(const n in e){const i=e[n];if(i===void 0){console.warn(`THREE.Material: parameter '${n}' has value of undefined.`);continue}const r=this[n];if(r===void 0){console.warn(`THREE.Material: '${n}' is not a property of THREE.${this.type}.`);continue}r&&r.isColor?r.set(i):r&&r.isVector3&&i&&i.isVector3?r.copy(i):this[n]=i}}toJSON(e){const n=e===void 0||typeof e=="string";n&&(e={textures:{},images:{}});const i={metadata:{version:4.6,type:"Material",generator:"Material.toJSON"}};i.uuid=this.uuid,i.type=this.type,this.name!==""&&(i.name=this.name),this.color&&this.color.isColor&&(i.color=this.color.getHex()),this.roughness!==void 0&&(i.roughness=this.roughness),this.metalness!==void 0&&(i.metalness=this.metalness),this.sheen!==void 0&&(i.sheen=this.sheen),this.sheenColor&&this.sheenColor.isColor&&(i.sheenColor=this.sheenColor.getHex()),this.sheenRoughness!==void 0&&(i.sheenRoughness=this.sheenRoughness),this.emissive&&this.emissive.isColor&&(i.emissive=this.emissive.getHex()),this.emissiveIntensity!==void 0&&this.emissiveIntensity!==1&&(i.emissiveIntensity=this.emissiveIntensity),this.specular&&this.specular.isColor&&(i.specular=this.specular.getHex()),this.specularIntensity!==void 0&&(i.specularIntensity=this.specularIntensity),this.specularColor&&this.specularColor.isColor&&(i.specularColor=this.specularColor.getHex()),this.shininess!==void 0&&(i.shininess=this.shininess),this.clearcoat!==void 0&&(i.clearcoat=this.clearcoat),this.clearcoatRoughness!==void 0&&(i.clearcoatRoughness=this.clearcoatRoughness),this.clearcoatMap&&this.clearcoatMap.isTexture&&(i.clearcoatMap=this.clearcoatMap.toJSON(e).uuid),this.clearcoatRoughnessMap&&this.clearcoatRoughnessMap.isTexture&&(i.clearcoatRoughnessMap=this.clearcoatRoughnessMap.toJSON(e).uuid),this.clearcoatNormalMap&&this.clearcoatNormalMap.isTexture&&(i.clearcoatNormalMap=this.clearcoatNormalMap.toJSON(e).uuid,i.clearcoatNormalScale=this.clearcoatNormalScale.toArray()),this.dispersion!==void 0&&(i.dispersion=this.dispersion),this.iridescence!==void 0&&(i.iridescence=this.iridescence),this.iridescenceIOR!==void 0&&(i.iridescenceIOR=this.iridescenceIOR),this.iridescenceThicknessRange!==void 0&&(i.iridescenceThicknessRange=this.iridescenceThicknessRange),this.iridescenceMap&&this.iridescenceMap.isTexture&&(i.iridescenceMap=this.iridescenceMap.toJSON(e).uuid),this.iridescenceThicknessMap&&this.iridescenceThicknessMap.isTexture&&(i.iridescenceThicknessMap=this.iridescenceThicknessMap.toJSON(e).uuid),this.anisotropy!==void 0&&(i.anisotropy=this.anisotropy),this.anisotropyRotation!==void 0&&(i.anisotropyRotation=this.anisotropyRotation),this.anisotropyMap&&this.anisotropyMap.isTexture&&(i.anisotropyMap=this.anisotropyMap.toJSON(e).uuid),this.map&&this.map.isTexture&&(i.map=this.map.toJSON(e).uuid),this.matcap&&this.matcap.isTexture&&(i.matcap=this.matcap.toJSON(e).uuid),this.alphaMap&&this.alphaMap.isTexture&&(i.alphaMap=this.alphaMap.toJSON(e).uuid),this.lightMap&&this.lightMap.isTexture&&(i.lightMap=this.lightMap.toJSON(e).uuid,i.lightMapIntensity=this.lightMapIntensity),this.aoMap&&this.aoMap.isTexture&&(i.aoMap=this.aoMap.toJSON(e).uuid,i.aoMapIntensity=this.aoMapIntensity),this.bumpMap&&this.bumpMap.isTexture&&(i.bumpMap=this.bumpMap.toJSON(e).uuid,i.bumpScale=this.bumpScale),this.normalMap&&this.normalMap.isTexture&&(i.normalMap=this.normalMap.toJSON(e).uuid,i.normalMapType=this.normalMapType,i.normalScale=this.normalScale.toArray()),this.displacementMap&&this.displacementMap.isTexture&&(i.displacementMap=this.displacementMap.toJSON(e).uuid,i.displacementScale=this.displacementScale,i.displacementBias=this.displacementBias),this.roughnessMap&&this.roughnessMap.isTexture&&(i.roughnessMap=this.roughnessMap.toJSON(e).uuid),this.metalnessMap&&this.metalnessMap.isTexture&&(i.metalnessMap=this.metalnessMap.toJSON(e).uuid),this.emissiveMap&&this.emissiveMap.isTexture&&(i.emissiveMap=this.emissiveMap.toJSON(e).uuid),this.specularMap&&this.specularMap.isTexture&&(i.specularMap=this.specularMap.toJSON(e).uuid),this.specularIntensityMap&&this.specularIntensityMap.isTexture&&(i.specularIntensityMap=this.specularIntensityMap.toJSON(e).uuid),this.specularColorMap&&this.specularColorMap.isTexture&&(i.specularColorMap=this.specularColorMap.toJSON(e).uuid),this.envMap&&this.envMap.isTexture&&(i.envMap=this.envMap.toJSON(e).uuid,this.combine!==void 0&&(i.combine=this.combine)),this.envMapRotation!==void 0&&(i.envMapRotation=this.envMapRotation.toArray()),this.envMapIntensity!==void 0&&(i.envMapIntensity=this.envMapIntensity),this.reflectivity!==void 0&&(i.reflectivity=this.reflectivity),this.refractionRatio!==void 0&&(i.refractionRatio=this.refractionRatio),this.gradientMap&&this.gradientMap.isTexture&&(i.gradientMap=this.gradientMap.toJSON(e).uuid),this.transmission!==void 0&&(i.transmission=this.transmission),this.transmissionMap&&this.transmissionMap.isTexture&&(i.transmissionMap=this.transmissionMap.toJSON(e).uuid),this.thickness!==void 0&&(i.thickness=this.thickness),this.thicknessMap&&this.thicknessMap.isTexture&&(i.thicknessMap=this.thicknessMap.toJSON(e).uuid),this.attenuationDistance!==void 0&&this.attenuationDistance!==1/0&&(i.attenuationDistance=this.attenuationDistance),this.attenuationColor!==void 0&&(i.attenuationColor=this.attenuationColor.getHex()),this.size!==void 0&&(i.size=this.size),this.shadowSide!==null&&(i.shadowSide=this.shadowSide),this.sizeAttenuation!==void 0&&(i.sizeAttenuation=this.sizeAttenuation),this.blending!==Do&&(i.blending=this.blending),this.side!==sr&&(i.side=this.side),this.vertexColors===!0&&(i.vertexColors=!0),this.opacity<1&&(i.opacity=this.opacity),this.transparent===!0&&(i.transparent=!0),this.blendSrc!==Nd&&(i.blendSrc=this.blendSrc),this.blendDst!==Id&&(i.blendDst=this.blendDst),this.blendEquation!==Mr&&(i.blendEquation=this.blendEquation),this.blendSrcAlpha!==null&&(i.blendSrcAlpha=this.blendSrcAlpha),this.blendDstAlpha!==null&&(i.blendDstAlpha=this.blendDstAlpha),this.blendEquationAlpha!==null&&(i.blendEquationAlpha=this.blendEquationAlpha),this.blendColor&&this.blendColor.isColor&&(i.blendColor=this.blendColor.getHex()),this.blendAlpha!==0&&(i.blendAlpha=this.blendAlpha),this.depthFunc!==Ho&&(i.depthFunc=this.depthFunc),this.depthTest===!1&&(i.depthTest=this.depthTest),this.depthWrite===!1&&(i.depthWrite=this.depthWrite),this.colorWrite===!1&&(i.colorWrite=this.colorWrite),this.stencilWriteMask!==255&&(i.stencilWriteMask=this.stencilWriteMask),this.stencilFunc!==im&&(i.stencilFunc=this.stencilFunc),this.stencilRef!==0&&(i.stencilRef=this.stencilRef),this.stencilFuncMask!==255&&(i.stencilFuncMask=this.stencilFuncMask),this.stencilFail!==Wr&&(i.stencilFail=this.stencilFail),this.stencilZFail!==Wr&&(i.stencilZFail=this.stencilZFail),this.stencilZPass!==Wr&&(i.stencilZPass=this.stencilZPass),this.stencilWrite===!0&&(i.stencilWrite=this.stencilWrite),this.rotation!==void 0&&this.rotation!==0&&(i.rotation=this.rotation),this.polygonOffset===!0&&(i.polygonOffset=!0),this.polygonOffsetFactor!==0&&(i.polygonOffsetFactor=this.polygonOffsetFactor),this.polygonOffsetUnits!==0&&(i.polygonOffsetUnits=this.polygonOffsetUnits),this.linewidth!==void 0&&this.linewidth!==1&&(i.linewidth=this.linewidth),this.dashSize!==void 0&&(i.dashSize=this.dashSize),this.gapSize!==void 0&&(i.gapSize=this.gapSize),this.scale!==void 0&&(i.scale=this.scale),this.dithering===!0&&(i.dithering=!0),this.alphaTest>0&&(i.alphaTest=this.alphaTest),this.alphaHash===!0&&(i.alphaHash=!0),this.alphaToCoverage===!0&&(i.alphaToCoverage=!0),this.premultipliedAlpha===!0&&(i.premultipliedAlpha=!0),this.forceSinglePass===!0&&(i.forceSinglePass=!0),this.wireframe===!0&&(i.wireframe=!0),this.wireframeLinewidth>1&&(i.wireframeLinewidth=this.wireframeLinewidth),this.wireframeLinecap!=="round"&&(i.wireframeLinecap=this.wireframeLinecap),this.wireframeLinejoin!=="round"&&(i.wireframeLinejoin=this.wireframeLinejoin),this.flatShading===!0&&(i.flatShading=!0),this.visible===!1&&(i.visible=!1),this.toneMapped===!1&&(i.toneMapped=!1),this.fog===!1&&(i.fog=!1),Object.keys(this.userData).length>0&&(i.userData=this.userData);function r(o){const s=[];for(const a in o){const l=o[a];delete l.metadata,s.push(l)}return s}if(n){const o=r(e.textures),s=r(e.images);o.length>0&&(i.textures=o),s.length>0&&(i.images=s)}return i}clone(){return new this.constructor().copy(this)}copy(e){this.name=e.name,this.blending=e.blending,this.side=e.side,this.vertexColors=e.vertexColors,this.opacity=e.opacity,this.transparent=e.transparent,this.blendSrc=e.blendSrc,this.blendDst=e.blendDst,this.blendEquation=e.blendEquation,this.blendSrcAlpha=e.blendSrcAlpha,this.blendDstAlpha=e.blendDstAlpha,this.blendEquationAlpha=e.blendEquationAlpha,this.blendColor.copy(e.blendColor),this.blendAlpha=e.blendAlpha,this.depthFunc=e.depthFunc,this.depthTest=e.depthTest,this.depthWrite=e.depthWrite,this.stencilWriteMask=e.stencilWriteMask,this.stencilFunc=e.stencilFunc,this.stencilRef=e.stencilRef,this.stencilFuncMask=e.stencilFuncMask,this.stencilFail=e.stencilFail,this.stencilZFail=e.stencilZFail,this.stencilZPass=e.stencilZPass,this.stencilWrite=e.stencilWrite;const n=e.clippingPlanes;let i=null;if(n!==null){const r=n.length;i=new Array(r);for(let o=0;o!==r;++o)i[o]=n[o].clone()}return this.clippingPlanes=i,this.clipIntersection=e.clipIntersection,this.clipShadows=e.clipShadows,this.shadowSide=e.shadowSide,this.colorWrite=e.colorWrite,this.precision=e.precision,this.polygonOffset=e.polygonOffset,this.polygonOffsetFactor=e.polygonOffsetFactor,this.polygonOffsetUnits=e.polygonOffsetUnits,this.dithering=e.dithering,this.alphaTest=e.alphaTest,this.alphaHash=e.alphaHash,this.alphaToCoverage=e.alphaToCoverage,this.premultipliedAlpha=e.premultipliedAlpha,this.forceSinglePass=e.forceSinglePass,this.visible=e.visible,this.toneMapped=e.toneMapped,this.userData=JSON.parse(JSON.stringify(e.userData)),this}dispose(){this.dispatchEvent({type:"dispose"})}set needsUpdate(e){e===!0&&this.version++}onBuild(){console.warn("Material: onBuild() has been removed.")}}class ny extends ya{static get type(){return"MeshBasicMaterial"}constructor(e){super(),this.isMeshBasicMaterial=!0,this.color=new we(16777215),this.map=null,this.lightMap=null,this.lightMapIntensity=1,this.aoMap=null,this.aoMapIntensity=1,this.specularMap=null,this.alphaMap=null,this.envMap=null,this.envMapRotation=new $n,this.combine=Fv,this.reflectivity=1,this.refractionRatio=.98,this.wireframe=!1,this.wireframeLinewidth=1,this.wireframeLinecap="round",this.wireframeLinejoin="round",this.fog=!0,this.setValues(e)}copy(e){return super.copy(e),this.color.copy(e.color),this.map=e.map,this.lightMap=e.lightMap,this.lightMapIntensity=e.lightMapIntensity,this.aoMap=e.aoMap,this.aoMapIntensity=e.aoMapIntensity,this.specularMap=e.specularMap,this.alphaMap=e.alphaMap,this.envMap=e.envMap,this.envMapRotation.copy(e.envMapRotation),this.combine=e.combine,this.reflectivity=e.reflectivity,this.refractionRatio=e.refractionRatio,this.wireframe=e.wireframe,this.wireframeLinewidth=e.wireframeLinewidth,this.wireframeLinecap=e.wireframeLinecap,this.wireframeLinejoin=e.wireframeLinejoin,this.fog=e.fog,this}}const wt=new L,qa=new Re;class en{constructor(e,n,i=!1){if(Array.isArray(e))throw new TypeError("THREE.BufferAttribute: array should be a Typed Array.");this.isBufferAttribute=!0,this.name="",this.array=e,this.itemSize=n,this.count=e!==void 0?e.length/n:0,this.normalized=i,this.usage=rm,this.updateRanges=[],this.gpuType=Pn,this.version=0}onUploadCallback(){}set needsUpdate(e){e===!0&&this.version++}setUsage(e){return this.usage=e,this}addUpdateRange(e,n){this.updateRanges.push({start:e,count:n})}clearUpdateRanges(){this.updateRanges.length=0}copy(e){return this.name=e.name,this.array=new e.array.constructor(e.array),this.itemSize=e.itemSize,this.count=e.count,this.normalized=e.normalized,this.usage=e.usage,this.gpuType=e.gpuType,this}copyAt(e,n,i){e*=this.itemSize,i*=n.itemSize;for(let r=0,o=this.itemSize;r<o;r++)this.array[e+r]=n.array[i+r];return this}copyArray(e){return this.array.set(e),this}applyMatrix3(e){if(this.itemSize===2)for(let n=0,i=this.count;n<i;n++)qa.fromBufferAttribute(this,n),qa.applyMatrix3(e),this.setXY(n,qa.x,qa.y);else if(this.itemSize===3)for(let n=0,i=this.count;n<i;n++)wt.fromBufferAttribute(this,n),wt.applyMatrix3(e),this.setXYZ(n,wt.x,wt.y,wt.z);return this}applyMatrix4(e){for(let n=0,i=this.count;n<i;n++)wt.fromBufferAttribute(this,n),wt.applyMatrix4(e),this.setXYZ(n,wt.x,wt.y,wt.z);return this}applyNormalMatrix(e){for(let n=0,i=this.count;n<i;n++)wt.fromBufferAttribute(this,n),wt.applyNormalMatrix(e),this.setXYZ(n,wt.x,wt.y,wt.z);return this}transformDirection(e){for(let n=0,i=this.count;n<i;n++)wt.fromBufferAttribute(this,n),wt.transformDirection(e),this.setXYZ(n,wt.x,wt.y,wt.z);return this}set(e,n=0){return this.array.set(e,n),this}getComponent(e,n){let i=this.array[e*this.itemSize+n];return this.normalized&&(i=fs(i,this.array)),i}setComponent(e,n,i){return this.normalized&&(i=on(i,this.array)),this.array[e*this.itemSize+n]=i,this}getX(e){let n=this.array[e*this.itemSize];return this.normalized&&(n=fs(n,this.array)),n}setX(e,n){return this.normalized&&(n=on(n,this.array)),this.array[e*this.itemSize]=n,this}getY(e){let n=this.array[e*this.itemSize+1];return this.normalized&&(n=fs(n,this.array)),n}setY(e,n){return this.normalized&&(n=on(n,this.array)),this.array[e*this.itemSize+1]=n,this}getZ(e){let n=this.array[e*this.itemSize+2];return this.normalized&&(n=fs(n,this.array)),n}setZ(e,n){return this.normalized&&(n=on(n,this.array)),this.array[e*this.itemSize+2]=n,this}getW(e){let n=this.array[e*this.itemSize+3];return this.normalized&&(n=fs(n,this.array)),n}setW(e,n){return this.normalized&&(n=on(n,this.array)),this.array[e*this.itemSize+3]=n,this}setXY(e,n,i){return e*=this.itemSize,this.normalized&&(n=on(n,this.array),i=on(i,this.array)),this.array[e+0]=n,this.array[e+1]=i,this}setXYZ(e,n,i,r){return e*=this.itemSize,this.normalized&&(n=on(n,this.array),i=on(i,this.array),r=on(r,this.array)),this.array[e+0]=n,this.array[e+1]=i,this.array[e+2]=r,this}setXYZW(e,n,i,r,o){return e*=this.itemSize,this.normalized&&(n=on(n,this.array),i=on(i,this.array),r=on(r,this.array),o=on(o,this.array)),this.array[e+0]=n,this.array[e+1]=i,this.array[e+2]=r,this.array[e+3]=o,this}onUpload(e){return this.onUploadCallback=e,this}clone(){return new this.constructor(this.array,this.itemSize).copy(this)}toJSON(){const e={itemSize:this.itemSize,type:this.array.constructor.name,array:Array.from(this.array),normalized:this.normalized};return this.name!==""&&(e.name=this.name),this.usage!==rm&&(e.usage=this.usage),e}}class iy extends en{constructor(e,n,i){super(new Uint16Array(e),n,i)}}class ry extends en{constructor(e,n,i){super(new Uint32Array(e),n,i)}}class ct extends en{constructor(e,n,i){super(new Float32Array(e),n,i)}}let PM=0;const En=new rt,Su=new Xt,eo=new L,pn=new Br,vs=new Br,Pt=new L;class Yt extends Qo{constructor(){super(),this.isBufferGeometry=!0,Object.defineProperty(this,"id",{value:PM++}),this.uuid=ga(),this.name="",this.type="BufferGeometry",this.index=null,this.indirect=null,this.attributes={},this.morphAttributes={},this.morphTargetsRelative=!1,this.groups=[],this.boundingBox=null,this.boundingSphere=null,this.drawRange={start:0,count:1/0},this.userData={}}getIndex(){return this.index}setIndex(e){return Array.isArray(e)?this.index=new(Zv(e)?ry:iy)(e,1):this.index=e,this}setIndirect(e){return this.indirect=e,this}getIndirect(){return this.indirect}getAttribute(e){return this.attributes[e]}setAttribute(e,n){return this.attributes[e]=n,this}deleteAttribute(e){return delete this.attributes[e],this}hasAttribute(e){return this.attributes[e]!==void 0}addGroup(e,n,i=0){this.groups.push({start:e,count:n,materialIndex:i})}clearGroups(){this.groups=[]}setDrawRange(e,n){this.drawRange.start=e,this.drawRange.count=n}applyMatrix4(e){const n=this.attributes.position;n!==void 0&&(n.applyMatrix4(e),n.needsUpdate=!0);const i=this.attributes.normal;if(i!==void 0){const o=new Be().getNormalMatrix(e);i.applyNormalMatrix(o),i.needsUpdate=!0}const r=this.attributes.tangent;return r!==void 0&&(r.transformDirection(e),r.needsUpdate=!0),this.boundingBox!==null&&this.computeBoundingBox(),this.boundingSphere!==null&&this.computeBoundingSphere(),this}applyQuaternion(e){return En.makeRotationFromQuaternion(e),this.applyMatrix4(En),this}rotateX(e){return En.makeRotationX(e),this.applyMatrix4(En),this}rotateY(e){return En.makeRotationY(e),this.applyMatrix4(En),this}rotateZ(e){return En.makeRotationZ(e),this.applyMatrix4(En),this}translate(e,n,i){return En.makeTranslation(e,n,i),this.applyMatrix4(En),this}scale(e,n,i){return En.makeScale(e,n,i),this.applyMatrix4(En),this}lookAt(e){return Su.lookAt(e),Su.updateMatrix(),this.applyMatrix4(Su.matrix),this}center(){return this.computeBoundingBox(),this.boundingBox.getCenter(eo).negate(),this.translate(eo.x,eo.y,eo.z),this}setFromPoints(e){const n=this.getAttribute("position");if(n===void 0){const i=[];for(let r=0,o=e.length;r<o;r++){const s=e[r];i.push(s.x,s.y,s.z||0)}this.setAttribute("position",new ct(i,3))}else{for(let i=0,r=n.count;i<r;i++){const o=e[i];n.setXYZ(i,o.x,o.y,o.z||0)}e.length>n.count&&console.warn("THREE.BufferGeometry: Buffer size too small for points data. Use .dispose() and create a new geometry."),n.needsUpdate=!0}return this}computeBoundingBox(){this.boundingBox===null&&(this.boundingBox=new Br);const e=this.attributes.position,n=this.morphAttributes.position;if(e&&e.isGLBufferAttribute){console.error("THREE.BufferGeometry.computeBoundingBox(): GLBufferAttribute requires a manual bounding box.",this),this.boundingBox.set(new L(-1/0,-1/0,-1/0),new L(1/0,1/0,1/0));return}if(e!==void 0){if(this.boundingBox.setFromBufferAttribute(e),n)for(let i=0,r=n.length;i<r;i++){const o=n[i];pn.setFromBufferAttribute(o),this.morphTargetsRelative?(Pt.addVectors(this.boundingBox.min,pn.min),this.boundingBox.expandByPoint(Pt),Pt.addVectors(this.boundingBox.max,pn.max),this.boundingBox.expandByPoint(Pt)):(this.boundingBox.expandByPoint(pn.min),this.boundingBox.expandByPoint(pn.max))}}else this.boundingBox.makeEmpty();(isNaN(this.boundingBox.min.x)||isNaN(this.boundingBox.min.y)||isNaN(this.boundingBox.min.z))&&console.error('THREE.BufferGeometry.computeBoundingBox(): Computed min/max have NaN values. The "position" attribute is likely to have NaN values.',this)}computeBoundingSphere(){this.boundingSphere===null&&(this.boundingSphere=new va);const e=this.attributes.position,n=this.morphAttributes.position;if(e&&e.isGLBufferAttribute){console.error("THREE.BufferGeometry.computeBoundingSphere(): GLBufferAttribute requires a manual bounding sphere.",this),this.boundingSphere.set(new L,1/0);return}if(e){const i=this.boundingSphere.center;if(pn.setFromBufferAttribute(e),n)for(let o=0,s=n.length;o<s;o++){const a=n[o];vs.setFromBufferAttribute(a),this.morphTargetsRelative?(Pt.addVectors(pn.min,vs.min),pn.expandByPoint(Pt),Pt.addVectors(pn.max,vs.max),pn.expandByPoint(Pt)):(pn.expandByPoint(vs.min),pn.expandByPoint(vs.max))}pn.getCenter(i);let r=0;for(let o=0,s=e.count;o<s;o++)Pt.fromBufferAttribute(e,o),r=Math.max(r,i.distanceToSquared(Pt));if(n)for(let o=0,s=n.length;o<s;o++){const a=n[o],l=this.morphTargetsRelative;for(let c=0,d=a.count;c<d;c++)Pt.fromBufferAttribute(a,c),l&&(eo.fromBufferAttribute(e,c),Pt.add(eo)),r=Math.max(r,i.distanceToSquared(Pt))}this.boundingSphere.radius=Math.sqrt(r),isNaN(this.boundingSphere.radius)&&console.error('THREE.BufferGeometry.computeBoundingSphere(): Computed radius is NaN. The "position" attribute is likely to have NaN values.',this)}}computeTangents(){const e=this.index,n=this.attributes;if(e===null||n.position===void 0||n.normal===void 0||n.uv===void 0){console.error("THREE.BufferGeometry: .computeTangents() failed. Missing required attributes (index, position, normal or uv)");return}const i=n.position,r=n.normal,o=n.uv;this.hasAttribute("tangent")===!1&&this.setAttribute("tangent",new en(new Float32Array(4*i.count),4));const s=this.getAttribute("tangent"),a=[],l=[];for(let b=0;b<i.count;b++)a[b]=new L,l[b]=new L;const c=new L,d=new L,f=new L,h=new Re,p=new Re,v=new Re,_=new L,m=new L;function u(b,w,S){c.fromBufferAttribute(i,b),d.fromBufferAttribute(i,w),f.fromBufferAttribute(i,S),h.fromBufferAttribute(o,b),p.fromBufferAttribute(o,w),v.fromBufferAttribute(o,S),d.sub(c),f.sub(c),p.sub(h),v.sub(h);const P=1/(p.x*v.y-v.x*p.y);isFinite(P)&&(_.copy(d).multiplyScalar(v.y).addScaledVector(f,-p.y).multiplyScalar(P),m.copy(f).multiplyScalar(p.x).addScaledVector(d,-v.x).multiplyScalar(P),a[b].add(_),a[w].add(_),a[S].add(_),l[b].add(m),l[w].add(m),l[S].add(m))}let g=this.groups;g.length===0&&(g=[{start:0,count:e.count}]);for(let b=0,w=g.length;b<w;++b){const S=g[b],P=S.start,z=S.count;for(let F=P,Y=P+z;F<Y;F+=3)u(e.getX(F+0),e.getX(F+1),e.getX(F+2))}const y=new L,x=new L,C=new L,A=new L;function T(b){C.fromBufferAttribute(r,b),A.copy(C);const w=a[b];y.copy(w),y.sub(C.multiplyScalar(C.dot(w))).normalize(),x.crossVectors(A,w);const P=x.dot(l[b])<0?-1:1;s.setXYZW(b,y.x,y.y,y.z,P)}for(let b=0,w=g.length;b<w;++b){const S=g[b],P=S.start,z=S.count;for(let F=P,Y=P+z;F<Y;F+=3)T(e.getX(F+0)),T(e.getX(F+1)),T(e.getX(F+2))}}computeVertexNormals(){const e=this.index,n=this.getAttribute("position");if(n!==void 0){let i=this.getAttribute("normal");if(i===void 0)i=new en(new Float32Array(n.count*3),3),this.setAttribute("normal",i);else for(let h=0,p=i.count;h<p;h++)i.setXYZ(h,0,0,0);const r=new L,o=new L,s=new L,a=new L,l=new L,c=new L,d=new L,f=new L;if(e)for(let h=0,p=e.count;h<p;h+=3){const v=e.getX(h+0),_=e.getX(h+1),m=e.getX(h+2);r.fromBufferAttribute(n,v),o.fromBufferAttribute(n,_),s.fromBufferAttribute(n,m),d.subVectors(s,o),f.subVectors(r,o),d.cross(f),a.fromBufferAttribute(i,v),l.fromBufferAttribute(i,_),c.fromBufferAttribute(i,m),a.add(d),l.add(d),c.add(d),i.setXYZ(v,a.x,a.y,a.z),i.setXYZ(_,l.x,l.y,l.z),i.setXYZ(m,c.x,c.y,c.z)}else for(let h=0,p=n.count;h<p;h+=3)r.fromBufferAttribute(n,h+0),o.fromBufferAttribute(n,h+1),s.fromBufferAttribute(n,h+2),d.subVectors(s,o),f.subVectors(r,o),d.cross(f),i.setXYZ(h+0,d.x,d.y,d.z),i.setXYZ(h+1,d.x,d.y,d.z),i.setXYZ(h+2,d.x,d.y,d.z);this.normalizeNormals(),i.needsUpdate=!0}}normalizeNormals(){const e=this.attributes.normal;for(let n=0,i=e.count;n<i;n++)Pt.fromBufferAttribute(e,n),Pt.normalize(),e.setXYZ(n,Pt.x,Pt.y,Pt.z)}toNonIndexed(){function e(a,l){const c=a.array,d=a.itemSize,f=a.normalized,h=new c.constructor(l.length*d);let p=0,v=0;for(let _=0,m=l.length;_<m;_++){a.isInterleavedBufferAttribute?p=l[_]*a.data.stride+a.offset:p=l[_]*d;for(let u=0;u<d;u++)h[v++]=c[p++]}return new en(h,d,f)}if(this.index===null)return console.warn("THREE.BufferGeometry.toNonIndexed(): BufferGeometry is already non-indexed."),this;const n=new Yt,i=this.index.array,r=this.attributes;for(const a in r){const l=r[a],c=e(l,i);n.setAttribute(a,c)}const o=this.morphAttributes;for(const a in o){const l=[],c=o[a];for(let d=0,f=c.length;d<f;d++){const h=c[d],p=e(h,i);l.push(p)}n.morphAttributes[a]=l}n.morphTargetsRelative=this.morphTargetsRelative;const s=this.groups;for(let a=0,l=s.length;a<l;a++){const c=s[a];n.addGroup(c.start,c.count,c.materialIndex)}return n}toJSON(){const e={metadata:{version:4.6,type:"BufferGeometry",generator:"BufferGeometry.toJSON"}};if(e.uuid=this.uuid,e.type=this.type,this.name!==""&&(e.name=this.name),Object.keys(this.userData).length>0&&(e.userData=this.userData),this.parameters!==void 0){const l=this.parameters;for(const c in l)l[c]!==void 0&&(e[c]=l[c]);return e}e.data={attributes:{}};const n=this.index;n!==null&&(e.data.index={type:n.array.constructor.name,array:Array.prototype.slice.call(n.array)});const i=this.attributes;for(const l in i){const c=i[l];e.data.attributes[l]=c.toJSON(e.data)}const r={};let o=!1;for(const l in this.morphAttributes){const c=this.morphAttributes[l],d=[];for(let f=0,h=c.length;f<h;f++){const p=c[f];d.push(p.toJSON(e.data))}d.length>0&&(r[l]=d,o=!0)}o&&(e.data.morphAttributes=r,e.data.morphTargetsRelative=this.morphTargetsRelative);const s=this.groups;s.length>0&&(e.data.groups=JSON.parse(JSON.stringify(s)));const a=this.boundingSphere;return a!==null&&(e.data.boundingSphere={center:a.center.toArray(),radius:a.radius}),e}clone(){return new this.constructor().copy(this)}copy(e){this.index=null,this.attributes={},this.morphAttributes={},this.groups=[],this.boundingBox=null,this.boundingSphere=null;const n={};this.name=e.name;const i=e.index;i!==null&&this.setIndex(i.clone(n));const r=e.attributes;for(const c in r){const d=r[c];this.setAttribute(c,d.clone(n))}const o=e.morphAttributes;for(const c in o){const d=[],f=o[c];for(let h=0,p=f.length;h<p;h++)d.push(f[h].clone(n));this.morphAttributes[c]=d}this.morphTargetsRelative=e.morphTargetsRelative;const s=e.groups;for(let c=0,d=s.length;c<d;c++){const f=s[c];this.addGroup(f.start,f.count,f.materialIndex)}const a=e.boundingBox;a!==null&&(this.boundingBox=a.clone());const l=e.boundingSphere;return l!==null&&(this.boundingSphere=l.clone()),this.drawRange.start=e.drawRange.start,this.drawRange.count=e.drawRange.count,this.userData=e.userData,this}dispose(){this.dispatchEvent({type:"dispose"})}}const Sm=new rt,fr=new MM,$a=new va,Mm=new L,Ka=new L,Za=new L,Ja=new L,Mu=new L,Qa=new L,wm=new L,el=new L;class Ct extends Xt{constructor(e=new Yt,n=new ny){super(),this.isMesh=!0,this.type="Mesh",this.geometry=e,this.material=n,this.updateMorphTargets()}copy(e,n){return super.copy(e,n),e.morphTargetInfluences!==void 0&&(this.morphTargetInfluences=e.morphTargetInfluences.slice()),e.morphTargetDictionary!==void 0&&(this.morphTargetDictionary=Object.assign({},e.morphTargetDictionary)),this.material=Array.isArray(e.material)?e.material.slice():e.material,this.geometry=e.geometry,this}updateMorphTargets(){const n=this.geometry.morphAttributes,i=Object.keys(n);if(i.length>0){const r=n[i[0]];if(r!==void 0){this.morphTargetInfluences=[],this.morphTargetDictionary={};for(let o=0,s=r.length;o<s;o++){const a=r[o].name||String(o);this.morphTargetInfluences.push(0),this.morphTargetDictionary[a]=o}}}}getVertexPosition(e,n){const i=this.geometry,r=i.attributes.position,o=i.morphAttributes.position,s=i.morphTargetsRelative;n.fromBufferAttribute(r,e);const a=this.morphTargetInfluences;if(o&&a){Qa.set(0,0,0);for(let l=0,c=o.length;l<c;l++){const d=a[l],f=o[l];d!==0&&(Mu.fromBufferAttribute(f,e),s?Qa.addScaledVector(Mu,d):Qa.addScaledVector(Mu.sub(n),d))}n.add(Qa)}return n}raycast(e,n){const i=this.geometry,r=this.material,o=this.matrixWorld;r!==void 0&&(i.boundingSphere===null&&i.computeBoundingSphere(),$a.copy(i.boundingSphere),$a.applyMatrix4(o),fr.copy(e.ray).recast(e.near),!($a.containsPoint(fr.origin)===!1&&(fr.intersectSphere($a,Mm)===null||fr.origin.distanceToSquared(Mm)>(e.far-e.near)**2))&&(Sm.copy(o).invert(),fr.copy(e.ray).applyMatrix4(Sm),!(i.boundingBox!==null&&fr.intersectsBox(i.boundingBox)===!1)&&this._computeIntersections(e,n,fr)))}_computeIntersections(e,n,i){let r;const o=this.geometry,s=this.material,a=o.index,l=o.attributes.position,c=o.attributes.uv,d=o.attributes.uv1,f=o.attributes.normal,h=o.groups,p=o.drawRange;if(a!==null)if(Array.isArray(s))for(let v=0,_=h.length;v<_;v++){const m=h[v],u=s[m.materialIndex],g=Math.max(m.start,p.start),y=Math.min(a.count,Math.min(m.start+m.count,p.start+p.count));for(let x=g,C=y;x<C;x+=3){const A=a.getX(x),T=a.getX(x+1),b=a.getX(x+2);r=tl(this,u,e,i,c,d,f,A,T,b),r&&(r.faceIndex=Math.floor(x/3),r.face.materialIndex=m.materialIndex,n.push(r))}}else{const v=Math.max(0,p.start),_=Math.min(a.count,p.start+p.count);for(let m=v,u=_;m<u;m+=3){const g=a.getX(m),y=a.getX(m+1),x=a.getX(m+2);r=tl(this,s,e,i,c,d,f,g,y,x),r&&(r.faceIndex=Math.floor(m/3),n.push(r))}}else if(l!==void 0)if(Array.isArray(s))for(let v=0,_=h.length;v<_;v++){const m=h[v],u=s[m.materialIndex],g=Math.max(m.start,p.start),y=Math.min(l.count,Math.min(m.start+m.count,p.start+p.count));for(let x=g,C=y;x<C;x+=3){const A=x,T=x+1,b=x+2;r=tl(this,u,e,i,c,d,f,A,T,b),r&&(r.faceIndex=Math.floor(x/3),r.face.materialIndex=m.materialIndex,n.push(r))}}else{const v=Math.max(0,p.start),_=Math.min(l.count,p.start+p.count);for(let m=v,u=_;m<u;m+=3){const g=m,y=m+1,x=m+2;r=tl(this,s,e,i,c,d,f,g,y,x),r&&(r.faceIndex=Math.floor(m/3),n.push(r))}}}}function DM(t,e,n,i,r,o,s,a){let l;if(e.side===Jt?l=i.intersectTriangle(s,o,r,!0,a):l=i.intersectTriangle(r,o,s,e.side===sr,a),l===null)return null;el.copy(a),el.applyMatrix4(t.matrixWorld);const c=n.ray.origin.distanceTo(el);return c<n.near||c>n.far?null:{distance:c,point:el.clone(),object:t}}function tl(t,e,n,i,r,o,s,a,l,c){t.getVertexPosition(a,Ka),t.getVertexPosition(l,Za),t.getVertexPosition(c,Ja);const d=DM(t,e,n,i,Ka,Za,Ja,wm);if(d){const f=new L;Wn.getBarycoord(wm,Ka,Za,Ja,f),r&&(d.uv=Wn.getInterpolatedAttribute(r,a,l,c,f,new Re)),o&&(d.uv1=Wn.getInterpolatedAttribute(o,a,l,c,f,new Re)),s&&(d.normal=Wn.getInterpolatedAttribute(s,a,l,c,f,new L),d.normal.dot(i.direction)>0&&d.normal.multiplyScalar(-1));const h={a,b:l,c,normal:new L,materialIndex:0};Wn.getNormal(Ka,Za,Ja,h.normal),d.face=h,d.barycoord=f}return d}class ts extends Yt{constructor(e=1,n=1,i=1,r=1,o=1,s=1){super(),this.type="BoxGeometry",this.parameters={width:e,height:n,depth:i,widthSegments:r,heightSegments:o,depthSegments:s};const a=this;r=Math.floor(r),o=Math.floor(o),s=Math.floor(s);const l=[],c=[],d=[],f=[];let h=0,p=0;v("z","y","x",-1,-1,i,n,e,s,o,0),v("z","y","x",1,-1,i,n,-e,s,o,1),v("x","z","y",1,1,e,i,n,r,s,2),v("x","z","y",1,-1,e,i,-n,r,s,3),v("x","y","z",1,-1,e,n,i,r,o,4),v("x","y","z",-1,-1,e,n,-i,r,o,5),this.setIndex(l),this.setAttribute("position",new ct(c,3)),this.setAttribute("normal",new ct(d,3)),this.setAttribute("uv",new ct(f,2));function v(_,m,u,g,y,x,C,A,T,b,w){const S=x/T,P=C/b,z=x/2,F=C/2,Y=A/2,j=T+1,W=b+1;let $=0,D=0;const G=new L;for(let Z=0;Z<W;Z++){const le=Z*P-F;for(let Se=0;Se<j;Se++){const Ne=Se*S-z;G[_]=Ne*g,G[m]=le*y,G[u]=Y,c.push(G.x,G.y,G.z),G[_]=0,G[m]=0,G[u]=A>0?1:-1,d.push(G.x,G.y,G.z),f.push(Se/T),f.push(1-Z/b),$+=1}}for(let Z=0;Z<b;Z++)for(let le=0;le<T;le++){const Se=h+le+j*Z,Ne=h+le+j*(Z+1),q=h+(le+1)+j*(Z+1),re=h+(le+1)+j*Z;l.push(Se,Ne,re),l.push(Ne,q,re),D+=6}a.addGroup(p,D,w),p+=D,h+=$}}copy(e){return super.copy(e),this.parameters=Object.assign({},e.parameters),this}static fromJSON(e){return new ts(e.width,e.height,e.depth,e.widthSegments,e.heightSegments,e.depthSegments)}}function Yo(t){const e={};for(const n in t){e[n]={};for(const i in t[n]){const r=t[n][i];r&&(r.isColor||r.isMatrix3||r.isMatrix4||r.isVector2||r.isVector3||r.isVector4||r.isTexture||r.isQuaternion)?r.isRenderTargetTexture?(console.warn("UniformsUtils: Textures of render targets cannot be cloned via cloneUniforms() or mergeUniforms()."),e[n][i]=null):e[n][i]=r.clone():Array.isArray(r)?e[n][i]=r.slice():e[n][i]=r}}return e}function qt(t){const e={};for(let n=0;n<t.length;n++){const i=Yo(t[n]);for(const r in i)e[r]=i[r]}return e}function LM(t){const e=[];for(let n=0;n<t.length;n++)e.push(t[n].clone());return e}function oy(t){const e=t.getRenderTarget();return e===null?t.outputColorSpace:e.isXRRenderTarget===!0?e.texture.colorSpace:$e.workingColorSpace}const NM={clone:Yo,merge:qt};var IM=`void main() {
	gl_Position = projectionMatrix * modelViewMatrix * vec4( position, 1.0 );
}`,kM=`void main() {
	gl_FragColor = vec4( 1.0, 0.0, 0.0, 1.0 );
}`;class xn extends ya{static get type(){return"ShaderMaterial"}constructor(e){super(),this.isShaderMaterial=!0,this.defines={},this.uniforms={},this.uniformsGroups=[],this.vertexShader=IM,this.fragmentShader=kM,this.linewidth=1,this.wireframe=!1,this.wireframeLinewidth=1,this.fog=!1,this.lights=!1,this.clipping=!1,this.forceSinglePass=!0,this.extensions={clipCullDistance:!1,multiDraw:!1},this.defaultAttributeValues={color:[1,1,1],uv:[0,0],uv1:[0,0]},this.index0AttributeName=void 0,this.uniformsNeedUpdate=!1,this.glslVersion=null,e!==void 0&&this.setValues(e)}copy(e){return super.copy(e),this.fragmentShader=e.fragmentShader,this.vertexShader=e.vertexShader,this.uniforms=Yo(e.uniforms),this.uniformsGroups=LM(e.uniformsGroups),this.defines=Object.assign({},e.defines),this.wireframe=e.wireframe,this.wireframeLinewidth=e.wireframeLinewidth,this.fog=e.fog,this.lights=e.lights,this.clipping=e.clipping,this.extensions=Object.assign({},e.extensions),this.glslVersion=e.glslVersion,this}toJSON(e){const n=super.toJSON(e);n.glslVersion=this.glslVersion,n.uniforms={};for(const r in this.uniforms){const s=this.uniforms[r].value;s&&s.isTexture?n.uniforms[r]={type:"t",value:s.toJSON(e).uuid}:s&&s.isColor?n.uniforms[r]={type:"c",value:s.getHex()}:s&&s.isVector2?n.uniforms[r]={type:"v2",value:s.toArray()}:s&&s.isVector3?n.uniforms[r]={type:"v3",value:s.toArray()}:s&&s.isVector4?n.uniforms[r]={type:"v4",value:s.toArray()}:s&&s.isMatrix3?n.uniforms[r]={type:"m3",value:s.toArray()}:s&&s.isMatrix4?n.uniforms[r]={type:"m4",value:s.toArray()}:n.uniforms[r]={value:s}}Object.keys(this.defines).length>0&&(n.defines=this.defines),n.vertexShader=this.vertexShader,n.fragmentShader=this.fragmentShader,n.lights=this.lights,n.clipping=this.clipping;const i={};for(const r in this.extensions)this.extensions[r]===!0&&(i[r]=!0);return Object.keys(i).length>0&&(n.extensions=i),n}}class sy extends Xt{constructor(){super(),this.isCamera=!0,this.type="Camera",this.matrixWorldInverse=new rt,this.projectionMatrix=new rt,this.projectionMatrixInverse=new rt,this.coordinateSystem=_i}copy(e,n){return super.copy(e,n),this.matrixWorldInverse.copy(e.matrixWorldInverse),this.projectionMatrix.copy(e.projectionMatrix),this.projectionMatrixInverse.copy(e.projectionMatrixInverse),this.coordinateSystem=e.coordinateSystem,this}getWorldDirection(e){return super.getWorldDirection(e).negate()}updateMatrixWorld(e){super.updateMatrixWorld(e),this.matrixWorldInverse.copy(this.matrixWorld).invert()}updateWorldMatrix(e,n){super.updateWorldMatrix(e,n),this.matrixWorldInverse.copy(this.matrixWorld).invert()}clone(){return new this.constructor().copy(this)}}const Ui=new L,Em=new Re,Tm=new Re;class Cn extends sy{constructor(e=50,n=1,i=.1,r=2e3){super(),this.isPerspectiveCamera=!0,this.type="PerspectiveCamera",this.fov=e,this.zoom=1,this.near=i,this.far=r,this.focus=10,this.aspect=n,this.view=null,this.filmGauge=35,this.filmOffset=0,this.updateProjectionMatrix()}copy(e,n){return super.copy(e,n),this.fov=e.fov,this.zoom=e.zoom,this.near=e.near,this.far=e.far,this.focus=e.focus,this.aspect=e.aspect,this.view=e.view===null?null:Object.assign({},e.view),this.filmGauge=e.filmGauge,this.filmOffset=e.filmOffset,this}setFocalLength(e){const n=.5*this.getFilmHeight()/e;this.fov=vh*2*Math.atan(n),this.updateProjectionMatrix()}getFocalLength(){const e=Math.tan(nu*.5*this.fov);return .5*this.getFilmHeight()/e}getEffectiveFOV(){return vh*2*Math.atan(Math.tan(nu*.5*this.fov)/this.zoom)}getFilmWidth(){return this.filmGauge*Math.min(this.aspect,1)}getFilmHeight(){return this.filmGauge/Math.max(this.aspect,1)}getViewBounds(e,n,i){Ui.set(-1,-1,.5).applyMatrix4(this.projectionMatrixInverse),n.set(Ui.x,Ui.y).multiplyScalar(-e/Ui.z),Ui.set(1,1,.5).applyMatrix4(this.projectionMatrixInverse),i.set(Ui.x,Ui.y).multiplyScalar(-e/Ui.z)}getViewSize(e,n){return this.getViewBounds(e,Em,Tm),n.subVectors(Tm,Em)}setViewOffset(e,n,i,r,o,s){this.aspect=e/n,this.view===null&&(this.view={enabled:!0,fullWidth:1,fullHeight:1,offsetX:0,offsetY:0,width:1,height:1}),this.view.enabled=!0,this.view.fullWidth=e,this.view.fullHeight=n,this.view.offsetX=i,this.view.offsetY=r,this.view.width=o,this.view.height=s,this.updateProjectionMatrix()}clearViewOffset(){this.view!==null&&(this.view.enabled=!1),this.updateProjectionMatrix()}updateProjectionMatrix(){const e=this.near;let n=e*Math.tan(nu*.5*this.fov)/this.zoom,i=2*n,r=this.aspect*i,o=-.5*r;const s=this.view;if(this.view!==null&&this.view.enabled){const l=s.fullWidth,c=s.fullHeight;o+=s.offsetX*r/l,n-=s.offsetY*i/c,r*=s.width/l,i*=s.height/c}const a=this.filmOffset;a!==0&&(o+=e*a/this.getFilmWidth()),this.projectionMatrix.makePerspective(o,o+r,n,n-i,e,this.far,this.coordinateSystem),this.projectionMatrixInverse.copy(this.projectionMatrix).invert()}toJSON(e){const n=super.toJSON(e);return n.object.fov=this.fov,n.object.zoom=this.zoom,n.object.near=this.near,n.object.far=this.far,n.object.focus=this.focus,n.object.aspect=this.aspect,this.view!==null&&(n.object.view=Object.assign({},this.view)),n.object.filmGauge=this.filmGauge,n.object.filmOffset=this.filmOffset,n}}const to=-90,no=1;class UM extends Xt{constructor(e,n,i){super(),this.type="CubeCamera",this.renderTarget=i,this.coordinateSystem=null,this.activeMipmapLevel=0;const r=new Cn(to,no,e,n);r.layers=this.layers,this.add(r);const o=new Cn(to,no,e,n);o.layers=this.layers,this.add(o);const s=new Cn(to,no,e,n);s.layers=this.layers,this.add(s);const a=new Cn(to,no,e,n);a.layers=this.layers,this.add(a);const l=new Cn(to,no,e,n);l.layers=this.layers,this.add(l);const c=new Cn(to,no,e,n);c.layers=this.layers,this.add(c)}updateCoordinateSystem(){const e=this.coordinateSystem,n=this.children.concat(),[i,r,o,s,a,l]=n;for(const c of n)this.remove(c);if(e===_i)i.up.set(0,1,0),i.lookAt(1,0,0),r.up.set(0,1,0),r.lookAt(-1,0,0),o.up.set(0,0,-1),o.lookAt(0,1,0),s.up.set(0,0,1),s.lookAt(0,-1,0),a.up.set(0,1,0),a.lookAt(0,0,1),l.up.set(0,1,0),l.lookAt(0,0,-1);else if(e===Jl)i.up.set(0,-1,0),i.lookAt(-1,0,0),r.up.set(0,-1,0),r.lookAt(1,0,0),o.up.set(0,0,1),o.lookAt(0,1,0),s.up.set(0,0,-1),s.lookAt(0,-1,0),a.up.set(0,-1,0),a.lookAt(0,0,1),l.up.set(0,-1,0),l.lookAt(0,0,-1);else throw new Error("THREE.CubeCamera.updateCoordinateSystem(): Invalid coordinate system: "+e);for(const c of n)this.add(c),c.updateMatrixWorld()}update(e,n){this.parent===null&&this.updateMatrixWorld();const{renderTarget:i,activeMipmapLevel:r}=this;this.coordinateSystem!==e.coordinateSystem&&(this.coordinateSystem=e.coordinateSystem,this.updateCoordinateSystem());const[o,s,a,l,c,d]=this.children,f=e.getRenderTarget(),h=e.getActiveCubeFace(),p=e.getActiveMipmapLevel(),v=e.xr.enabled;e.xr.enabled=!1;const _=i.texture.generateMipmaps;i.texture.generateMipmaps=!1,e.setRenderTarget(i,0,r),e.render(n,o),e.setRenderTarget(i,1,r),e.render(n,s),e.setRenderTarget(i,2,r),e.render(n,a),e.setRenderTarget(i,3,r),e.render(n,l),e.setRenderTarget(i,4,r),e.render(n,c),i.texture.generateMipmaps=_,e.setRenderTarget(i,5,r),e.render(n,d),e.setRenderTarget(f,h,p),e.xr.enabled=v,i.texture.needsPMREMUpdate=!0}}class ay extends Qt{constructor(e,n,i,r,o,s,a,l,c,d){e=e!==void 0?e:[],n=n!==void 0?n:Vo,super(e,n,i,r,o,s,a,l,c,d),this.isCubeTexture=!0,this.flipY=!1}get images(){return this.image}set images(e){this.image=e}}class FM extends qn{constructor(e=1,n={}){super(e,e,n),this.isWebGLCubeRenderTarget=!0;const i={width:e,height:e,depth:1},r=[i,i,i,i,i,i];this.texture=new ay(r,n.mapping,n.wrapS,n.wrapT,n.magFilter,n.minFilter,n.format,n.type,n.anisotropy,n.colorSpace),this.texture.isRenderTargetTexture=!0,this.texture.generateMipmaps=n.generateMipmaps!==void 0?n.generateMipmaps:!1,this.texture.minFilter=n.minFilter!==void 0?n.minFilter:Kt}fromEquirectangularTexture(e,n){this.texture.type=n.type,this.texture.colorSpace=n.colorSpace,this.texture.generateMipmaps=n.generateMipmaps,this.texture.minFilter=n.minFilter,this.texture.magFilter=n.magFilter;const i={uniforms:{tEquirect:{value:null}},vertexShader:`

				varying vec3 vWorldDirection;

				vec3 transformDirection( in vec3 dir, in mat4 matrix ) {

					return normalize( ( matrix * vec4( dir, 0.0 ) ).xyz );

				}

				void main() {

					vWorldDirection = transformDirection( position, modelMatrix );

					#include <begin_vertex>
					#include <project_vertex>

				}
			`,fragmentShader:`

				uniform sampler2D tEquirect;

				varying vec3 vWorldDirection;

				#include <common>

				void main() {

					vec3 direction = normalize( vWorldDirection );

					vec2 sampleUV = equirectUv( direction );

					gl_FragColor = texture2D( tEquirect, sampleUV );

				}
			`},r=new ts(5,5,5),o=new xn({name:"CubemapFromEquirect",uniforms:Yo(i.uniforms),vertexShader:i.vertexShader,fragmentShader:i.fragmentShader,side:Jt,blending:nr});o.uniforms.tEquirect.value=n;const s=new Ct(r,o),a=n.minFilter;return n.minFilter===ni&&(n.minFilter=Kt),new UM(1,10,this).update(e,s),n.minFilter=a,s.geometry.dispose(),s.material.dispose(),this}clear(e,n,i,r){const o=e.getRenderTarget();for(let s=0;s<6;s++)e.setRenderTarget(this,s),e.clear(n,i,r);e.setRenderTarget(o)}}const wu=new L,OM=new L,zM=new Be;class xr{constructor(e=new L(1,0,0),n=0){this.isPlane=!0,this.normal=e,this.constant=n}set(e,n){return this.normal.copy(e),this.constant=n,this}setComponents(e,n,i,r){return this.normal.set(e,n,i),this.constant=r,this}setFromNormalAndCoplanarPoint(e,n){return this.normal.copy(e),this.constant=-n.dot(this.normal),this}setFromCoplanarPoints(e,n,i){const r=wu.subVectors(i,n).cross(OM.subVectors(e,n)).normalize();return this.setFromNormalAndCoplanarPoint(r,e),this}copy(e){return this.normal.copy(e.normal),this.constant=e.constant,this}normalize(){const e=1/this.normal.length();return this.normal.multiplyScalar(e),this.constant*=e,this}negate(){return this.constant*=-1,this.normal.negate(),this}distanceToPoint(e){return this.normal.dot(e)+this.constant}distanceToSphere(e){return this.distanceToPoint(e.center)-e.radius}projectPoint(e,n){return n.copy(e).addScaledVector(this.normal,-this.distanceToPoint(e))}intersectLine(e,n){const i=e.delta(wu),r=this.normal.dot(i);if(r===0)return this.distanceToPoint(e.start)===0?n.copy(e.start):null;const o=-(e.start.dot(this.normal)+this.constant)/r;return o<0||o>1?null:n.copy(e.start).addScaledVector(i,o)}intersectsLine(e){const n=this.distanceToPoint(e.start),i=this.distanceToPoint(e.end);return n<0&&i>0||i<0&&n>0}intersectsBox(e){return e.intersectsPlane(this)}intersectsSphere(e){return e.intersectsPlane(this)}coplanarPoint(e){return e.copy(this.normal).multiplyScalar(-this.constant)}applyMatrix4(e,n){const i=n||zM.getNormalMatrix(e),r=this.coplanarPoint(wu).applyMatrix4(e),o=this.normal.applyMatrix3(i).normalize();return this.constant=-r.dot(o),this}translate(e){return this.constant-=e.dot(this.normal),this}equals(e){return e.normal.equals(this.normal)&&e.constant===this.constant}clone(){return new this.constructor().copy(this)}}const pr=new va,nl=new L;class Af{constructor(e=new xr,n=new xr,i=new xr,r=new xr,o=new xr,s=new xr){this.planes=[e,n,i,r,o,s]}set(e,n,i,r,o,s){const a=this.planes;return a[0].copy(e),a[1].copy(n),a[2].copy(i),a[3].copy(r),a[4].copy(o),a[5].copy(s),this}copy(e){const n=this.planes;for(let i=0;i<6;i++)n[i].copy(e.planes[i]);return this}setFromProjectionMatrix(e,n=_i){const i=this.planes,r=e.elements,o=r[0],s=r[1],a=r[2],l=r[3],c=r[4],d=r[5],f=r[6],h=r[7],p=r[8],v=r[9],_=r[10],m=r[11],u=r[12],g=r[13],y=r[14],x=r[15];if(i[0].setComponents(l-o,h-c,m-p,x-u).normalize(),i[1].setComponents(l+o,h+c,m+p,x+u).normalize(),i[2].setComponents(l+s,h+d,m+v,x+g).normalize(),i[3].setComponents(l-s,h-d,m-v,x-g).normalize(),i[4].setComponents(l-a,h-f,m-_,x-y).normalize(),n===_i)i[5].setComponents(l+a,h+f,m+_,x+y).normalize();else if(n===Jl)i[5].setComponents(a,f,_,y).normalize();else throw new Error("THREE.Frustum.setFromProjectionMatrix(): Invalid coordinate system: "+n);return this}intersectsObject(e){if(e.boundingSphere!==void 0)e.boundingSphere===null&&e.computeBoundingSphere(),pr.copy(e.boundingSphere).applyMatrix4(e.matrixWorld);else{const n=e.geometry;n.boundingSphere===null&&n.computeBoundingSphere(),pr.copy(n.boundingSphere).applyMatrix4(e.matrixWorld)}return this.intersectsSphere(pr)}intersectsSprite(e){return pr.center.set(0,0,0),pr.radius=.7071067811865476,pr.applyMatrix4(e.matrixWorld),this.intersectsSphere(pr)}intersectsSphere(e){const n=this.planes,i=e.center,r=-e.radius;for(let o=0;o<6;o++)if(n[o].distanceToPoint(i)<r)return!1;return!0}intersectsBox(e){const n=this.planes;for(let i=0;i<6;i++){const r=n[i];if(nl.x=r.normal.x>0?e.max.x:e.min.x,nl.y=r.normal.y>0?e.max.y:e.min.y,nl.z=r.normal.z>0?e.max.z:e.min.z,r.distanceToPoint(nl)<0)return!1}return!0}containsPoint(e){const n=this.planes;for(let i=0;i<6;i++)if(n[i].distanceToPoint(e)<0)return!1;return!0}clone(){return new this.constructor().copy(this)}}function ly(){let t=null,e=!1,n=null,i=null;function r(o,s){n(o,s),i=t.requestAnimationFrame(r)}return{start:function(){e!==!0&&n!==null&&(i=t.requestAnimationFrame(r),e=!0)},stop:function(){t.cancelAnimationFrame(i),e=!1},setAnimationLoop:function(o){n=o},setContext:function(o){t=o}}}function BM(t){const e=new WeakMap;function n(a,l){const c=a.array,d=a.usage,f=c.byteLength,h=t.createBuffer();t.bindBuffer(l,h),t.bufferData(l,c,d),a.onUploadCallback();let p;if(c instanceof Float32Array)p=t.FLOAT;else if(c instanceof Uint16Array)a.isFloat16BufferAttribute?p=t.HALF_FLOAT:p=t.UNSIGNED_SHORT;else if(c instanceof Int16Array)p=t.SHORT;else if(c instanceof Uint32Array)p=t.UNSIGNED_INT;else if(c instanceof Int32Array)p=t.INT;else if(c instanceof Int8Array)p=t.BYTE;else if(c instanceof Uint8Array)p=t.UNSIGNED_BYTE;else if(c instanceof Uint8ClampedArray)p=t.UNSIGNED_BYTE;else throw new Error("THREE.WebGLAttributes: Unsupported buffer data format: "+c);return{buffer:h,type:p,bytesPerElement:c.BYTES_PER_ELEMENT,version:a.version,size:f}}function i(a,l,c){const d=l.array,f=l.updateRanges;if(t.bindBuffer(c,a),f.length===0)t.bufferSubData(c,0,d);else{f.sort((p,v)=>p.start-v.start);let h=0;for(let p=1;p<f.length;p++){const v=f[h],_=f[p];_.start<=v.start+v.count+1?v.count=Math.max(v.count,_.start+_.count-v.start):(++h,f[h]=_)}f.length=h+1;for(let p=0,v=f.length;p<v;p++){const _=f[p];t.bufferSubData(c,_.start*d.BYTES_PER_ELEMENT,d,_.start,_.count)}l.clearUpdateRanges()}l.onUploadCallback()}function r(a){return a.isInterleavedBufferAttribute&&(a=a.data),e.get(a)}function o(a){a.isInterleavedBufferAttribute&&(a=a.data);const l=e.get(a);l&&(t.deleteBuffer(l.buffer),e.delete(a))}function s(a,l){if(a.isInterleavedBufferAttribute&&(a=a.data),a.isGLBufferAttribute){const d=e.get(a);(!d||d.version<a.version)&&e.set(a,{buffer:a.buffer,type:a.type,bytesPerElement:a.elementSize,version:a.version});return}const c=e.get(a);if(c===void 0)e.set(a,n(a,l));else if(c.version<a.version){if(c.size!==a.array.byteLength)throw new Error("THREE.WebGLAttributes: The size of the buffer attribute's array buffer does not match the original size. Resizing buffer attributes is not supported.");i(c.buffer,a,l),c.version=a.version}}return{get:r,remove:o,update:s}}class Hr extends Yt{constructor(e=1,n=1,i=1,r=1){super(),this.type="PlaneGeometry",this.parameters={width:e,height:n,widthSegments:i,heightSegments:r};const o=e/2,s=n/2,a=Math.floor(i),l=Math.floor(r),c=a+1,d=l+1,f=e/a,h=n/l,p=[],v=[],_=[],m=[];for(let u=0;u<d;u++){const g=u*h-s;for(let y=0;y<c;y++){const x=y*f-o;v.push(x,-g,0),_.push(0,0,1),m.push(y/a),m.push(1-u/l)}}for(let u=0;u<l;u++)for(let g=0;g<a;g++){const y=g+c*u,x=g+c*(u+1),C=g+1+c*(u+1),A=g+1+c*u;p.push(y,x,A),p.push(x,C,A)}this.setIndex(p),this.setAttribute("position",new ct(v,3)),this.setAttribute("normal",new ct(_,3)),this.setAttribute("uv",new ct(m,2))}copy(e){return super.copy(e),this.parameters=Object.assign({},e.parameters),this}static fromJSON(e){return new Hr(e.width,e.height,e.widthSegments,e.heightSegments)}}var HM=`#ifdef USE_ALPHAHASH
	if ( diffuseColor.a < getAlphaHashThreshold( vPosition ) ) discard;
#endif`,VM=`#ifdef USE_ALPHAHASH
	const float ALPHA_HASH_SCALE = 0.05;
	float hash2D( vec2 value ) {
		return fract( 1.0e4 * sin( 17.0 * value.x + 0.1 * value.y ) * ( 0.1 + abs( sin( 13.0 * value.y + value.x ) ) ) );
	}
	float hash3D( vec3 value ) {
		return hash2D( vec2( hash2D( value.xy ), value.z ) );
	}
	float getAlphaHashThreshold( vec3 position ) {
		float maxDeriv = max(
			length( dFdx( position.xyz ) ),
			length( dFdy( position.xyz ) )
		);
		float pixScale = 1.0 / ( ALPHA_HASH_SCALE * maxDeriv );
		vec2 pixScales = vec2(
			exp2( floor( log2( pixScale ) ) ),
			exp2( ceil( log2( pixScale ) ) )
		);
		vec2 alpha = vec2(
			hash3D( floor( pixScales.x * position.xyz ) ),
			hash3D( floor( pixScales.y * position.xyz ) )
		);
		float lerpFactor = fract( log2( pixScale ) );
		float x = ( 1.0 - lerpFactor ) * alpha.x + lerpFactor * alpha.y;
		float a = min( lerpFactor, 1.0 - lerpFactor );
		vec3 cases = vec3(
			x * x / ( 2.0 * a * ( 1.0 - a ) ),
			( x - 0.5 * a ) / ( 1.0 - a ),
			1.0 - ( ( 1.0 - x ) * ( 1.0 - x ) / ( 2.0 * a * ( 1.0 - a ) ) )
		);
		float threshold = ( x < ( 1.0 - a ) )
			? ( ( x < a ) ? cases.x : cases.y )
			: cases.z;
		return clamp( threshold , 1.0e-6, 1.0 );
	}
#endif`,GM=`#ifdef USE_ALPHAMAP
	diffuseColor.a *= texture2D( alphaMap, vAlphaMapUv ).g;
#endif`,WM=`#ifdef USE_ALPHAMAP
	uniform sampler2D alphaMap;
#endif`,XM=`#ifdef USE_ALPHATEST
	#ifdef ALPHA_TO_COVERAGE
	diffuseColor.a = smoothstep( alphaTest, alphaTest + fwidth( diffuseColor.a ), diffuseColor.a );
	if ( diffuseColor.a == 0.0 ) discard;
	#else
	if ( diffuseColor.a < alphaTest ) discard;
	#endif
#endif`,jM=`#ifdef USE_ALPHATEST
	uniform float alphaTest;
#endif`,YM=`#ifdef USE_AOMAP
	float ambientOcclusion = ( texture2D( aoMap, vAoMapUv ).r - 1.0 ) * aoMapIntensity + 1.0;
	reflectedLight.indirectDiffuse *= ambientOcclusion;
	#if defined( USE_CLEARCOAT ) 
		clearcoatSpecularIndirect *= ambientOcclusion;
	#endif
	#if defined( USE_SHEEN ) 
		sheenSpecularIndirect *= ambientOcclusion;
	#endif
	#if defined( USE_ENVMAP ) && defined( STANDARD )
		float dotNV = saturate( dot( geometryNormal, geometryViewDir ) );
		reflectedLight.indirectSpecular *= computeSpecularOcclusion( dotNV, ambientOcclusion, material.roughness );
	#endif
#endif`,qM=`#ifdef USE_AOMAP
	uniform sampler2D aoMap;
	uniform float aoMapIntensity;
#endif`,$M=`#ifdef USE_BATCHING
	#if ! defined( GL_ANGLE_multi_draw )
	#define gl_DrawID _gl_DrawID
	uniform int _gl_DrawID;
	#endif
	uniform highp sampler2D batchingTexture;
	uniform highp usampler2D batchingIdTexture;
	mat4 getBatchingMatrix( const in float i ) {
		int size = textureSize( batchingTexture, 0 ).x;
		int j = int( i ) * 4;
		int x = j % size;
		int y = j / size;
		vec4 v1 = texelFetch( batchingTexture, ivec2( x, y ), 0 );
		vec4 v2 = texelFetch( batchingTexture, ivec2( x + 1, y ), 0 );
		vec4 v3 = texelFetch( batchingTexture, ivec2( x + 2, y ), 0 );
		vec4 v4 = texelFetch( batchingTexture, ivec2( x + 3, y ), 0 );
		return mat4( v1, v2, v3, v4 );
	}
	float getIndirectIndex( const in int i ) {
		int size = textureSize( batchingIdTexture, 0 ).x;
		int x = i % size;
		int y = i / size;
		return float( texelFetch( batchingIdTexture, ivec2( x, y ), 0 ).r );
	}
#endif
#ifdef USE_BATCHING_COLOR
	uniform sampler2D batchingColorTexture;
	vec3 getBatchingColor( const in float i ) {
		int size = textureSize( batchingColorTexture, 0 ).x;
		int j = int( i );
		int x = j % size;
		int y = j / size;
		return texelFetch( batchingColorTexture, ivec2( x, y ), 0 ).rgb;
	}
#endif`,KM=`#ifdef USE_BATCHING
	mat4 batchingMatrix = getBatchingMatrix( getIndirectIndex( gl_DrawID ) );
#endif`,ZM=`vec3 transformed = vec3( position );
#ifdef USE_ALPHAHASH
	vPosition = vec3( position );
#endif`,JM=`vec3 objectNormal = vec3( normal );
#ifdef USE_TANGENT
	vec3 objectTangent = vec3( tangent.xyz );
#endif`,QM=`float G_BlinnPhong_Implicit( ) {
	return 0.25;
}
float D_BlinnPhong( const in float shininess, const in float dotNH ) {
	return RECIPROCAL_PI * ( shininess * 0.5 + 1.0 ) * pow( dotNH, shininess );
}
vec3 BRDF_BlinnPhong( const in vec3 lightDir, const in vec3 viewDir, const in vec3 normal, const in vec3 specularColor, const in float shininess ) {
	vec3 halfDir = normalize( lightDir + viewDir );
	float dotNH = saturate( dot( normal, halfDir ) );
	float dotVH = saturate( dot( viewDir, halfDir ) );
	vec3 F = F_Schlick( specularColor, 1.0, dotVH );
	float G = G_BlinnPhong_Implicit( );
	float D = D_BlinnPhong( shininess, dotNH );
	return F * ( G * D );
} // validated`,ew=`#ifdef USE_IRIDESCENCE
	const mat3 XYZ_TO_REC709 = mat3(
		 3.2404542, -0.9692660,  0.0556434,
		-1.5371385,  1.8760108, -0.2040259,
		-0.4985314,  0.0415560,  1.0572252
	);
	vec3 Fresnel0ToIor( vec3 fresnel0 ) {
		vec3 sqrtF0 = sqrt( fresnel0 );
		return ( vec3( 1.0 ) + sqrtF0 ) / ( vec3( 1.0 ) - sqrtF0 );
	}
	vec3 IorToFresnel0( vec3 transmittedIor, float incidentIor ) {
		return pow2( ( transmittedIor - vec3( incidentIor ) ) / ( transmittedIor + vec3( incidentIor ) ) );
	}
	float IorToFresnel0( float transmittedIor, float incidentIor ) {
		return pow2( ( transmittedIor - incidentIor ) / ( transmittedIor + incidentIor ));
	}
	vec3 evalSensitivity( float OPD, vec3 shift ) {
		float phase = 2.0 * PI * OPD * 1.0e-9;
		vec3 val = vec3( 5.4856e-13, 4.4201e-13, 5.2481e-13 );
		vec3 pos = vec3( 1.6810e+06, 1.7953e+06, 2.2084e+06 );
		vec3 var = vec3( 4.3278e+09, 9.3046e+09, 6.6121e+09 );
		vec3 xyz = val * sqrt( 2.0 * PI * var ) * cos( pos * phase + shift ) * exp( - pow2( phase ) * var );
		xyz.x += 9.7470e-14 * sqrt( 2.0 * PI * 4.5282e+09 ) * cos( 2.2399e+06 * phase + shift[ 0 ] ) * exp( - 4.5282e+09 * pow2( phase ) );
		xyz /= 1.0685e-7;
		vec3 rgb = XYZ_TO_REC709 * xyz;
		return rgb;
	}
	vec3 evalIridescence( float outsideIOR, float eta2, float cosTheta1, float thinFilmThickness, vec3 baseF0 ) {
		vec3 I;
		float iridescenceIOR = mix( outsideIOR, eta2, smoothstep( 0.0, 0.03, thinFilmThickness ) );
		float sinTheta2Sq = pow2( outsideIOR / iridescenceIOR ) * ( 1.0 - pow2( cosTheta1 ) );
		float cosTheta2Sq = 1.0 - sinTheta2Sq;
		if ( cosTheta2Sq < 0.0 ) {
			return vec3( 1.0 );
		}
		float cosTheta2 = sqrt( cosTheta2Sq );
		float R0 = IorToFresnel0( iridescenceIOR, outsideIOR );
		float R12 = F_Schlick( R0, 1.0, cosTheta1 );
		float T121 = 1.0 - R12;
		float phi12 = 0.0;
		if ( iridescenceIOR < outsideIOR ) phi12 = PI;
		float phi21 = PI - phi12;
		vec3 baseIOR = Fresnel0ToIor( clamp( baseF0, 0.0, 0.9999 ) );		vec3 R1 = IorToFresnel0( baseIOR, iridescenceIOR );
		vec3 R23 = F_Schlick( R1, 1.0, cosTheta2 );
		vec3 phi23 = vec3( 0.0 );
		if ( baseIOR[ 0 ] < iridescenceIOR ) phi23[ 0 ] = PI;
		if ( baseIOR[ 1 ] < iridescenceIOR ) phi23[ 1 ] = PI;
		if ( baseIOR[ 2 ] < iridescenceIOR ) phi23[ 2 ] = PI;
		float OPD = 2.0 * iridescenceIOR * thinFilmThickness * cosTheta2;
		vec3 phi = vec3( phi21 ) + phi23;
		vec3 R123 = clamp( R12 * R23, 1e-5, 0.9999 );
		vec3 r123 = sqrt( R123 );
		vec3 Rs = pow2( T121 ) * R23 / ( vec3( 1.0 ) - R123 );
		vec3 C0 = R12 + Rs;
		I = C0;
		vec3 Cm = Rs - T121;
		for ( int m = 1; m <= 2; ++ m ) {
			Cm *= r123;
			vec3 Sm = 2.0 * evalSensitivity( float( m ) * OPD, float( m ) * phi );
			I += Cm * Sm;
		}
		return max( I, vec3( 0.0 ) );
	}
#endif`,tw=`#ifdef USE_BUMPMAP
	uniform sampler2D bumpMap;
	uniform float bumpScale;
	vec2 dHdxy_fwd() {
		vec2 dSTdx = dFdx( vBumpMapUv );
		vec2 dSTdy = dFdy( vBumpMapUv );
		float Hll = bumpScale * texture2D( bumpMap, vBumpMapUv ).x;
		float dBx = bumpScale * texture2D( bumpMap, vBumpMapUv + dSTdx ).x - Hll;
		float dBy = bumpScale * texture2D( bumpMap, vBumpMapUv + dSTdy ).x - Hll;
		return vec2( dBx, dBy );
	}
	vec3 perturbNormalArb( vec3 surf_pos, vec3 surf_norm, vec2 dHdxy, float faceDirection ) {
		vec3 vSigmaX = normalize( dFdx( surf_pos.xyz ) );
		vec3 vSigmaY = normalize( dFdy( surf_pos.xyz ) );
		vec3 vN = surf_norm;
		vec3 R1 = cross( vSigmaY, vN );
		vec3 R2 = cross( vN, vSigmaX );
		float fDet = dot( vSigmaX, R1 ) * faceDirection;
		vec3 vGrad = sign( fDet ) * ( dHdxy.x * R1 + dHdxy.y * R2 );
		return normalize( abs( fDet ) * surf_norm - vGrad );
	}
#endif`,nw=`#if NUM_CLIPPING_PLANES > 0
	vec4 plane;
	#ifdef ALPHA_TO_COVERAGE
		float distanceToPlane, distanceGradient;
		float clipOpacity = 1.0;
		#pragma unroll_loop_start
		for ( int i = 0; i < UNION_CLIPPING_PLANES; i ++ ) {
			plane = clippingPlanes[ i ];
			distanceToPlane = - dot( vClipPosition, plane.xyz ) + plane.w;
			distanceGradient = fwidth( distanceToPlane ) / 2.0;
			clipOpacity *= smoothstep( - distanceGradient, distanceGradient, distanceToPlane );
			if ( clipOpacity == 0.0 ) discard;
		}
		#pragma unroll_loop_end
		#if UNION_CLIPPING_PLANES < NUM_CLIPPING_PLANES
			float unionClipOpacity = 1.0;
			#pragma unroll_loop_start
			for ( int i = UNION_CLIPPING_PLANES; i < NUM_CLIPPING_PLANES; i ++ ) {
				plane = clippingPlanes[ i ];
				distanceToPlane = - dot( vClipPosition, plane.xyz ) + plane.w;
				distanceGradient = fwidth( distanceToPlane ) / 2.0;
				unionClipOpacity *= 1.0 - smoothstep( - distanceGradient, distanceGradient, distanceToPlane );
			}
			#pragma unroll_loop_end
			clipOpacity *= 1.0 - unionClipOpacity;
		#endif
		diffuseColor.a *= clipOpacity;
		if ( diffuseColor.a == 0.0 ) discard;
	#else
		#pragma unroll_loop_start
		for ( int i = 0; i < UNION_CLIPPING_PLANES; i ++ ) {
			plane = clippingPlanes[ i ];
			if ( dot( vClipPosition, plane.xyz ) > plane.w ) discard;
		}
		#pragma unroll_loop_end
		#if UNION_CLIPPING_PLANES < NUM_CLIPPING_PLANES
			bool clipped = true;
			#pragma unroll_loop_start
			for ( int i = UNION_CLIPPING_PLANES; i < NUM_CLIPPING_PLANES; i ++ ) {
				plane = clippingPlanes[ i ];
				clipped = ( dot( vClipPosition, plane.xyz ) > plane.w ) && clipped;
			}
			#pragma unroll_loop_end
			if ( clipped ) discard;
		#endif
	#endif
#endif`,iw=`#if NUM_CLIPPING_PLANES > 0
	varying vec3 vClipPosition;
	uniform vec4 clippingPlanes[ NUM_CLIPPING_PLANES ];
#endif`,rw=`#if NUM_CLIPPING_PLANES > 0
	varying vec3 vClipPosition;
#endif`,ow=`#if NUM_CLIPPING_PLANES > 0
	vClipPosition = - mvPosition.xyz;
#endif`,sw=`#if defined( USE_COLOR_ALPHA )
	diffuseColor *= vColor;
#elif defined( USE_COLOR )
	diffuseColor.rgb *= vColor;
#endif`,aw=`#if defined( USE_COLOR_ALPHA )
	varying vec4 vColor;
#elif defined( USE_COLOR )
	varying vec3 vColor;
#endif`,lw=`#if defined( USE_COLOR_ALPHA )
	varying vec4 vColor;
#elif defined( USE_COLOR ) || defined( USE_INSTANCING_COLOR ) || defined( USE_BATCHING_COLOR )
	varying vec3 vColor;
#endif`,cw=`#if defined( USE_COLOR_ALPHA )
	vColor = vec4( 1.0 );
#elif defined( USE_COLOR ) || defined( USE_INSTANCING_COLOR ) || defined( USE_BATCHING_COLOR )
	vColor = vec3( 1.0 );
#endif
#ifdef USE_COLOR
	vColor *= color;
#endif
#ifdef USE_INSTANCING_COLOR
	vColor.xyz *= instanceColor.xyz;
#endif
#ifdef USE_BATCHING_COLOR
	vec3 batchingColor = getBatchingColor( getIndirectIndex( gl_DrawID ) );
	vColor.xyz *= batchingColor.xyz;
#endif`,uw=`#define PI 3.141592653589793
#define PI2 6.283185307179586
#define PI_HALF 1.5707963267948966
#define RECIPROCAL_PI 0.3183098861837907
#define RECIPROCAL_PI2 0.15915494309189535
#define EPSILON 1e-6
#ifndef saturate
#define saturate( a ) clamp( a, 0.0, 1.0 )
#endif
#define whiteComplement( a ) ( 1.0 - saturate( a ) )
float pow2( const in float x ) { return x*x; }
vec3 pow2( const in vec3 x ) { return x*x; }
float pow3( const in float x ) { return x*x*x; }
float pow4( const in float x ) { float x2 = x*x; return x2*x2; }
float max3( const in vec3 v ) { return max( max( v.x, v.y ), v.z ); }
float average( const in vec3 v ) { return dot( v, vec3( 0.3333333 ) ); }
highp float rand( const in vec2 uv ) {
	const highp float a = 12.9898, b = 78.233, c = 43758.5453;
	highp float dt = dot( uv.xy, vec2( a,b ) ), sn = mod( dt, PI );
	return fract( sin( sn ) * c );
}
#ifdef HIGH_PRECISION
	float precisionSafeLength( vec3 v ) { return length( v ); }
#else
	float precisionSafeLength( vec3 v ) {
		float maxComponent = max3( abs( v ) );
		return length( v / maxComponent ) * maxComponent;
	}
#endif
struct IncidentLight {
	vec3 color;
	vec3 direction;
	bool visible;
};
struct ReflectedLight {
	vec3 directDiffuse;
	vec3 directSpecular;
	vec3 indirectDiffuse;
	vec3 indirectSpecular;
};
#ifdef USE_ALPHAHASH
	varying vec3 vPosition;
#endif
vec3 transformDirection( in vec3 dir, in mat4 matrix ) {
	return normalize( ( matrix * vec4( dir, 0.0 ) ).xyz );
}
vec3 inverseTransformDirection( in vec3 dir, in mat4 matrix ) {
	return normalize( ( vec4( dir, 0.0 ) * matrix ).xyz );
}
mat3 transposeMat3( const in mat3 m ) {
	mat3 tmp;
	tmp[ 0 ] = vec3( m[ 0 ].x, m[ 1 ].x, m[ 2 ].x );
	tmp[ 1 ] = vec3( m[ 0 ].y, m[ 1 ].y, m[ 2 ].y );
	tmp[ 2 ] = vec3( m[ 0 ].z, m[ 1 ].z, m[ 2 ].z );
	return tmp;
}
bool isPerspectiveMatrix( mat4 m ) {
	return m[ 2 ][ 3 ] == - 1.0;
}
vec2 equirectUv( in vec3 dir ) {
	float u = atan( dir.z, dir.x ) * RECIPROCAL_PI2 + 0.5;
	float v = asin( clamp( dir.y, - 1.0, 1.0 ) ) * RECIPROCAL_PI + 0.5;
	return vec2( u, v );
}
vec3 BRDF_Lambert( const in vec3 diffuseColor ) {
	return RECIPROCAL_PI * diffuseColor;
}
vec3 F_Schlick( const in vec3 f0, const in float f90, const in float dotVH ) {
	float fresnel = exp2( ( - 5.55473 * dotVH - 6.98316 ) * dotVH );
	return f0 * ( 1.0 - fresnel ) + ( f90 * fresnel );
}
float F_Schlick( const in float f0, const in float f90, const in float dotVH ) {
	float fresnel = exp2( ( - 5.55473 * dotVH - 6.98316 ) * dotVH );
	return f0 * ( 1.0 - fresnel ) + ( f90 * fresnel );
} // validated`,dw=`#ifdef ENVMAP_TYPE_CUBE_UV
	#define cubeUV_minMipLevel 4.0
	#define cubeUV_minTileSize 16.0
	float getFace( vec3 direction ) {
		vec3 absDirection = abs( direction );
		float face = - 1.0;
		if ( absDirection.x > absDirection.z ) {
			if ( absDirection.x > absDirection.y )
				face = direction.x > 0.0 ? 0.0 : 3.0;
			else
				face = direction.y > 0.0 ? 1.0 : 4.0;
		} else {
			if ( absDirection.z > absDirection.y )
				face = direction.z > 0.0 ? 2.0 : 5.0;
			else
				face = direction.y > 0.0 ? 1.0 : 4.0;
		}
		return face;
	}
	vec2 getUV( vec3 direction, float face ) {
		vec2 uv;
		if ( face == 0.0 ) {
			uv = vec2( direction.z, direction.y ) / abs( direction.x );
		} else if ( face == 1.0 ) {
			uv = vec2( - direction.x, - direction.z ) / abs( direction.y );
		} else if ( face == 2.0 ) {
			uv = vec2( - direction.x, direction.y ) / abs( direction.z );
		} else if ( face == 3.0 ) {
			uv = vec2( - direction.z, direction.y ) / abs( direction.x );
		} else if ( face == 4.0 ) {
			uv = vec2( - direction.x, direction.z ) / abs( direction.y );
		} else {
			uv = vec2( direction.x, direction.y ) / abs( direction.z );
		}
		return 0.5 * ( uv + 1.0 );
	}
	vec3 bilinearCubeUV( sampler2D envMap, vec3 direction, float mipInt ) {
		float face = getFace( direction );
		float filterInt = max( cubeUV_minMipLevel - mipInt, 0.0 );
		mipInt = max( mipInt, cubeUV_minMipLevel );
		float faceSize = exp2( mipInt );
		highp vec2 uv = getUV( direction, face ) * ( faceSize - 2.0 ) + 1.0;
		if ( face > 2.0 ) {
			uv.y += faceSize;
			face -= 3.0;
		}
		uv.x += face * faceSize;
		uv.x += filterInt * 3.0 * cubeUV_minTileSize;
		uv.y += 4.0 * ( exp2( CUBEUV_MAX_MIP ) - faceSize );
		uv.x *= CUBEUV_TEXEL_WIDTH;
		uv.y *= CUBEUV_TEXEL_HEIGHT;
		#ifdef texture2DGradEXT
			return texture2DGradEXT( envMap, uv, vec2( 0.0 ), vec2( 0.0 ) ).rgb;
		#else
			return texture2D( envMap, uv ).rgb;
		#endif
	}
	#define cubeUV_r0 1.0
	#define cubeUV_m0 - 2.0
	#define cubeUV_r1 0.8
	#define cubeUV_m1 - 1.0
	#define cubeUV_r4 0.4
	#define cubeUV_m4 2.0
	#define cubeUV_r5 0.305
	#define cubeUV_m5 3.0
	#define cubeUV_r6 0.21
	#define cubeUV_m6 4.0
	float roughnessToMip( float roughness ) {
		float mip = 0.0;
		if ( roughness >= cubeUV_r1 ) {
			mip = ( cubeUV_r0 - roughness ) * ( cubeUV_m1 - cubeUV_m0 ) / ( cubeUV_r0 - cubeUV_r1 ) + cubeUV_m0;
		} else if ( roughness >= cubeUV_r4 ) {
			mip = ( cubeUV_r1 - roughness ) * ( cubeUV_m4 - cubeUV_m1 ) / ( cubeUV_r1 - cubeUV_r4 ) + cubeUV_m1;
		} else if ( roughness >= cubeUV_r5 ) {
			mip = ( cubeUV_r4 - roughness ) * ( cubeUV_m5 - cubeUV_m4 ) / ( cubeUV_r4 - cubeUV_r5 ) + cubeUV_m4;
		} else if ( roughness >= cubeUV_r6 ) {
			mip = ( cubeUV_r5 - roughness ) * ( cubeUV_m6 - cubeUV_m5 ) / ( cubeUV_r5 - cubeUV_r6 ) + cubeUV_m5;
		} else {
			mip = - 2.0 * log2( 1.16 * roughness );		}
		return mip;
	}
	vec4 textureCubeUV( sampler2D envMap, vec3 sampleDir, float roughness ) {
		float mip = clamp( roughnessToMip( roughness ), cubeUV_m0, CUBEUV_MAX_MIP );
		float mipF = fract( mip );
		float mipInt = floor( mip );
		vec3 color0 = bilinearCubeUV( envMap, sampleDir, mipInt );
		if ( mipF == 0.0 ) {
			return vec4( color0, 1.0 );
		} else {
			vec3 color1 = bilinearCubeUV( envMap, sampleDir, mipInt + 1.0 );
			return vec4( mix( color0, color1, mipF ), 1.0 );
		}
	}
#endif`,hw=`vec3 transformedNormal = objectNormal;
#ifdef USE_TANGENT
	vec3 transformedTangent = objectTangent;
#endif
#ifdef USE_BATCHING
	mat3 bm = mat3( batchingMatrix );
	transformedNormal /= vec3( dot( bm[ 0 ], bm[ 0 ] ), dot( bm[ 1 ], bm[ 1 ] ), dot( bm[ 2 ], bm[ 2 ] ) );
	transformedNormal = bm * transformedNormal;
	#ifdef USE_TANGENT
		transformedTangent = bm * transformedTangent;
	#endif
#endif
#ifdef USE_INSTANCING
	mat3 im = mat3( instanceMatrix );
	transformedNormal /= vec3( dot( im[ 0 ], im[ 0 ] ), dot( im[ 1 ], im[ 1 ] ), dot( im[ 2 ], im[ 2 ] ) );
	transformedNormal = im * transformedNormal;
	#ifdef USE_TANGENT
		transformedTangent = im * transformedTangent;
	#endif
#endif
transformedNormal = normalMatrix * transformedNormal;
#ifdef FLIP_SIDED
	transformedNormal = - transformedNormal;
#endif
#ifdef USE_TANGENT
	transformedTangent = ( modelViewMatrix * vec4( transformedTangent, 0.0 ) ).xyz;
	#ifdef FLIP_SIDED
		transformedTangent = - transformedTangent;
	#endif
#endif`,fw=`#ifdef USE_DISPLACEMENTMAP
	uniform sampler2D displacementMap;
	uniform float displacementScale;
	uniform float displacementBias;
#endif`,pw=`#ifdef USE_DISPLACEMENTMAP
	transformed += normalize( objectNormal ) * ( texture2D( displacementMap, vDisplacementMapUv ).x * displacementScale + displacementBias );
#endif`,mw=`#ifdef USE_EMISSIVEMAP
	vec4 emissiveColor = texture2D( emissiveMap, vEmissiveMapUv );
	#ifdef DECODE_VIDEO_TEXTURE_EMISSIVE
		emissiveColor = sRGBTransferEOTF( emissiveColor );
	#endif
	totalEmissiveRadiance *= emissiveColor.rgb;
#endif`,gw=`#ifdef USE_EMISSIVEMAP
	uniform sampler2D emissiveMap;
#endif`,vw="gl_FragColor = linearToOutputTexel( gl_FragColor );",yw=`vec4 LinearTransferOETF( in vec4 value ) {
	return value;
}
vec4 sRGBTransferEOTF( in vec4 value ) {
	return vec4( mix( pow( value.rgb * 0.9478672986 + vec3( 0.0521327014 ), vec3( 2.4 ) ), value.rgb * 0.0773993808, vec3( lessThanEqual( value.rgb, vec3( 0.04045 ) ) ) ), value.a );
}
vec4 sRGBTransferOETF( in vec4 value ) {
	return vec4( mix( pow( value.rgb, vec3( 0.41666 ) ) * 1.055 - vec3( 0.055 ), value.rgb * 12.92, vec3( lessThanEqual( value.rgb, vec3( 0.0031308 ) ) ) ), value.a );
}`,_w=`#ifdef USE_ENVMAP
	#ifdef ENV_WORLDPOS
		vec3 cameraToFrag;
		if ( isOrthographic ) {
			cameraToFrag = normalize( vec3( - viewMatrix[ 0 ][ 2 ], - viewMatrix[ 1 ][ 2 ], - viewMatrix[ 2 ][ 2 ] ) );
		} else {
			cameraToFrag = normalize( vWorldPosition - cameraPosition );
		}
		vec3 worldNormal = inverseTransformDirection( normal, viewMatrix );
		#ifdef ENVMAP_MODE_REFLECTION
			vec3 reflectVec = reflect( cameraToFrag, worldNormal );
		#else
			vec3 reflectVec = refract( cameraToFrag, worldNormal, refractionRatio );
		#endif
	#else
		vec3 reflectVec = vReflect;
	#endif
	#ifdef ENVMAP_TYPE_CUBE
		vec4 envColor = textureCube( envMap, envMapRotation * vec3( flipEnvMap * reflectVec.x, reflectVec.yz ) );
	#else
		vec4 envColor = vec4( 0.0 );
	#endif
	#ifdef ENVMAP_BLENDING_MULTIPLY
		outgoingLight = mix( outgoingLight, outgoingLight * envColor.xyz, specularStrength * reflectivity );
	#elif defined( ENVMAP_BLENDING_MIX )
		outgoingLight = mix( outgoingLight, envColor.xyz, specularStrength * reflectivity );
	#elif defined( ENVMAP_BLENDING_ADD )
		outgoingLight += envColor.xyz * specularStrength * reflectivity;
	#endif
#endif`,xw=`#ifdef USE_ENVMAP
	uniform float envMapIntensity;
	uniform float flipEnvMap;
	uniform mat3 envMapRotation;
	#ifdef ENVMAP_TYPE_CUBE
		uniform samplerCube envMap;
	#else
		uniform sampler2D envMap;
	#endif
	
#endif`,Sw=`#ifdef USE_ENVMAP
	uniform float reflectivity;
	#if defined( USE_BUMPMAP ) || defined( USE_NORMALMAP ) || defined( PHONG ) || defined( LAMBERT )
		#define ENV_WORLDPOS
	#endif
	#ifdef ENV_WORLDPOS
		varying vec3 vWorldPosition;
		uniform float refractionRatio;
	#else
		varying vec3 vReflect;
	#endif
#endif`,Mw=`#ifdef USE_ENVMAP
	#if defined( USE_BUMPMAP ) || defined( USE_NORMALMAP ) || defined( PHONG ) || defined( LAMBERT )
		#define ENV_WORLDPOS
	#endif
	#ifdef ENV_WORLDPOS
		
		varying vec3 vWorldPosition;
	#else
		varying vec3 vReflect;
		uniform float refractionRatio;
	#endif
#endif`,ww=`#ifdef USE_ENVMAP
	#ifdef ENV_WORLDPOS
		vWorldPosition = worldPosition.xyz;
	#else
		vec3 cameraToVertex;
		if ( isOrthographic ) {
			cameraToVertex = normalize( vec3( - viewMatrix[ 0 ][ 2 ], - viewMatrix[ 1 ][ 2 ], - viewMatrix[ 2 ][ 2 ] ) );
		} else {
			cameraToVertex = normalize( worldPosition.xyz - cameraPosition );
		}
		vec3 worldNormal = inverseTransformDirection( transformedNormal, viewMatrix );
		#ifdef ENVMAP_MODE_REFLECTION
			vReflect = reflect( cameraToVertex, worldNormal );
		#else
			vReflect = refract( cameraToVertex, worldNormal, refractionRatio );
		#endif
	#endif
#endif`,Ew=`#ifdef USE_FOG
	vFogDepth = - mvPosition.z;
#endif`,Tw=`#ifdef USE_FOG
	varying float vFogDepth;
#endif`,Aw=`#ifdef USE_FOG
	#ifdef FOG_EXP2
		float fogFactor = 1.0 - exp( - fogDensity * fogDensity * vFogDepth * vFogDepth );
	#else
		float fogFactor = smoothstep( fogNear, fogFar, vFogDepth );
	#endif
	gl_FragColor.rgb = mix( gl_FragColor.rgb, fogColor, fogFactor );
#endif`,Cw=`#ifdef USE_FOG
	uniform vec3 fogColor;
	varying float vFogDepth;
	#ifdef FOG_EXP2
		uniform float fogDensity;
	#else
		uniform float fogNear;
		uniform float fogFar;
	#endif
#endif`,bw=`#ifdef USE_GRADIENTMAP
	uniform sampler2D gradientMap;
#endif
vec3 getGradientIrradiance( vec3 normal, vec3 lightDirection ) {
	float dotNL = dot( normal, lightDirection );
	vec2 coord = vec2( dotNL * 0.5 + 0.5, 0.0 );
	#ifdef USE_GRADIENTMAP
		return vec3( texture2D( gradientMap, coord ).r );
	#else
		vec2 fw = fwidth( coord ) * 0.5;
		return mix( vec3( 0.7 ), vec3( 1.0 ), smoothstep( 0.7 - fw.x, 0.7 + fw.x, coord.x ) );
	#endif
}`,Rw=`#ifdef USE_LIGHTMAP
	uniform sampler2D lightMap;
	uniform float lightMapIntensity;
#endif`,Pw=`LambertMaterial material;
material.diffuseColor = diffuseColor.rgb;
material.specularStrength = specularStrength;`,Dw=`varying vec3 vViewPosition;
struct LambertMaterial {
	vec3 diffuseColor;
	float specularStrength;
};
void RE_Direct_Lambert( const in IncidentLight directLight, const in vec3 geometryPosition, const in vec3 geometryNormal, const in vec3 geometryViewDir, const in vec3 geometryClearcoatNormal, const in LambertMaterial material, inout ReflectedLight reflectedLight ) {
	float dotNL = saturate( dot( geometryNormal, directLight.direction ) );
	vec3 irradiance = dotNL * directLight.color;
	reflectedLight.directDiffuse += irradiance * BRDF_Lambert( material.diffuseColor );
}
void RE_IndirectDiffuse_Lambert( const in vec3 irradiance, const in vec3 geometryPosition, const in vec3 geometryNormal, const in vec3 geometryViewDir, const in vec3 geometryClearcoatNormal, const in LambertMaterial material, inout ReflectedLight reflectedLight ) {
	reflectedLight.indirectDiffuse += irradiance * BRDF_Lambert( material.diffuseColor );
}
#define RE_Direct				RE_Direct_Lambert
#define RE_IndirectDiffuse		RE_IndirectDiffuse_Lambert`,Lw=`uniform bool receiveShadow;
uniform vec3 ambientLightColor;
#if defined( USE_LIGHT_PROBES )
	uniform vec3 lightProbe[ 9 ];
#endif
vec3 shGetIrradianceAt( in vec3 normal, in vec3 shCoefficients[ 9 ] ) {
	float x = normal.x, y = normal.y, z = normal.z;
	vec3 result = shCoefficients[ 0 ] * 0.886227;
	result += shCoefficients[ 1 ] * 2.0 * 0.511664 * y;
	result += shCoefficients[ 2 ] * 2.0 * 0.511664 * z;
	result += shCoefficients[ 3 ] * 2.0 * 0.511664 * x;
	result += shCoefficients[ 4 ] * 2.0 * 0.429043 * x * y;
	result += shCoefficients[ 5 ] * 2.0 * 0.429043 * y * z;
	result += shCoefficients[ 6 ] * ( 0.743125 * z * z - 0.247708 );
	result += shCoefficients[ 7 ] * 2.0 * 0.429043 * x * z;
	result += shCoefficients[ 8 ] * 0.429043 * ( x * x - y * y );
	return result;
}
vec3 getLightProbeIrradiance( const in vec3 lightProbe[ 9 ], const in vec3 normal ) {
	vec3 worldNormal = inverseTransformDirection( normal, viewMatrix );
	vec3 irradiance = shGetIrradianceAt( worldNormal, lightProbe );
	return irradiance;
}
vec3 getAmbientLightIrradiance( const in vec3 ambientLightColor ) {
	vec3 irradiance = ambientLightColor;
	return irradiance;
}
float getDistanceAttenuation( const in float lightDistance, const in float cutoffDistance, const in float decayExponent ) {
	float distanceFalloff = 1.0 / max( pow( lightDistance, decayExponent ), 0.01 );
	if ( cutoffDistance > 0.0 ) {
		distanceFalloff *= pow2( saturate( 1.0 - pow4( lightDistance / cutoffDistance ) ) );
	}
	return distanceFalloff;
}
float getSpotAttenuation( const in float coneCosine, const in float penumbraCosine, const in float angleCosine ) {
	return smoothstep( coneCosine, penumbraCosine, angleCosine );
}
#if NUM_DIR_LIGHTS > 0
	struct DirectionalLight {
		vec3 direction;
		vec3 color;
	};
	uniform DirectionalLight directionalLights[ NUM_DIR_LIGHTS ];
	void getDirectionalLightInfo( const in DirectionalLight directionalLight, out IncidentLight light ) {
		light.color = directionalLight.color;
		light.direction = directionalLight.direction;
		light.visible = true;
	}
#endif
#if NUM_POINT_LIGHTS > 0
	struct PointLight {
		vec3 position;
		vec3 color;
		float distance;
		float decay;
	};
	uniform PointLight pointLights[ NUM_POINT_LIGHTS ];
	void getPointLightInfo( const in PointLight pointLight, const in vec3 geometryPosition, out IncidentLight light ) {
		vec3 lVector = pointLight.position - geometryPosition;
		light.direction = normalize( lVector );
		float lightDistance = length( lVector );
		light.color = pointLight.color;
		light.color *= getDistanceAttenuation( lightDistance, pointLight.distance, pointLight.decay );
		light.visible = ( light.color != vec3( 0.0 ) );
	}
#endif
#if NUM_SPOT_LIGHTS > 0
	struct SpotLight {
		vec3 position;
		vec3 direction;
		vec3 color;
		float distance;
		float decay;
		float coneCos;
		float penumbraCos;
	};
	uniform SpotLight spotLights[ NUM_SPOT_LIGHTS ];
	void getSpotLightInfo( const in SpotLight spotLight, const in vec3 geometryPosition, out IncidentLight light ) {
		vec3 lVector = spotLight.position - geometryPosition;
		light.direction = normalize( lVector );
		float angleCos = dot( light.direction, spotLight.direction );
		float spotAttenuation = getSpotAttenuation( spotLight.coneCos, spotLight.penumbraCos, angleCos );
		if ( spotAttenuation > 0.0 ) {
			float lightDistance = length( lVector );
			light.color = spotLight.color * spotAttenuation;
			light.color *= getDistanceAttenuation( lightDistance, spotLight.distance, spotLight.decay );
			light.visible = ( light.color != vec3( 0.0 ) );
		} else {
			light.color = vec3( 0.0 );
			light.visible = false;
		}
	}
#endif
#if NUM_RECT_AREA_LIGHTS > 0
	struct RectAreaLight {
		vec3 color;
		vec3 position;
		vec3 halfWidth;
		vec3 halfHeight;
	};
	uniform sampler2D ltc_1;	uniform sampler2D ltc_2;
	uniform RectAreaLight rectAreaLights[ NUM_RECT_AREA_LIGHTS ];
#endif
#if NUM_HEMI_LIGHTS > 0
	struct HemisphereLight {
		vec3 direction;
		vec3 skyColor;
		vec3 groundColor;
	};
	uniform HemisphereLight hemisphereLights[ NUM_HEMI_LIGHTS ];
	vec3 getHemisphereLightIrradiance( const in HemisphereLight hemiLight, const in vec3 normal ) {
		float dotNL = dot( normal, hemiLight.direction );
		float hemiDiffuseWeight = 0.5 * dotNL + 0.5;
		vec3 irradiance = mix( hemiLight.groundColor, hemiLight.skyColor, hemiDiffuseWeight );
		return irradiance;
	}
#endif`,Nw=`#ifdef USE_ENVMAP
	vec3 getIBLIrradiance( const in vec3 normal ) {
		#ifdef ENVMAP_TYPE_CUBE_UV
			vec3 worldNormal = inverseTransformDirection( normal, viewMatrix );
			vec4 envMapColor = textureCubeUV( envMap, envMapRotation * worldNormal, 1.0 );
			return PI * envMapColor.rgb * envMapIntensity;
		#else
			return vec3( 0.0 );
		#endif
	}
	vec3 getIBLRadiance( const in vec3 viewDir, const in vec3 normal, const in float roughness ) {
		#ifdef ENVMAP_TYPE_CUBE_UV
			vec3 reflectVec = reflect( - viewDir, normal );
			reflectVec = normalize( mix( reflectVec, normal, roughness * roughness) );
			reflectVec = inverseTransformDirection( reflectVec, viewMatrix );
			vec4 envMapColor = textureCubeUV( envMap, envMapRotation * reflectVec, roughness );
			return envMapColor.rgb * envMapIntensity;
		#else
			return vec3( 0.0 );
		#endif
	}
	#ifdef USE_ANISOTROPY
		vec3 getIBLAnisotropyRadiance( const in vec3 viewDir, const in vec3 normal, const in float roughness, const in vec3 bitangent, const in float anisotropy ) {
			#ifdef ENVMAP_TYPE_CUBE_UV
				vec3 bentNormal = cross( bitangent, viewDir );
				bentNormal = normalize( cross( bentNormal, bitangent ) );
				bentNormal = normalize( mix( bentNormal, normal, pow2( pow2( 1.0 - anisotropy * ( 1.0 - roughness ) ) ) ) );
				return getIBLRadiance( viewDir, bentNormal, roughness );
			#else
				return vec3( 0.0 );
			#endif
		}
	#endif
#endif`,Iw=`ToonMaterial material;
material.diffuseColor = diffuseColor.rgb;`,kw=`varying vec3 vViewPosition;
struct ToonMaterial {
	vec3 diffuseColor;
};
void RE_Direct_Toon( const in IncidentLight directLight, const in vec3 geometryPosition, const in vec3 geometryNormal, const in vec3 geometryViewDir, const in vec3 geometryClearcoatNormal, const in ToonMaterial material, inout ReflectedLight reflectedLight ) {
	vec3 irradiance = getGradientIrradiance( geometryNormal, directLight.direction ) * directLight.color;
	reflectedLight.directDiffuse += irradiance * BRDF_Lambert( material.diffuseColor );
}
void RE_IndirectDiffuse_Toon( const in vec3 irradiance, const in vec3 geometryPosition, const in vec3 geometryNormal, const in vec3 geometryViewDir, const in vec3 geometryClearcoatNormal, const in ToonMaterial material, inout ReflectedLight reflectedLight ) {
	reflectedLight.indirectDiffuse += irradiance * BRDF_Lambert( material.diffuseColor );
}
#define RE_Direct				RE_Direct_Toon
#define RE_IndirectDiffuse		RE_IndirectDiffuse_Toon`,Uw=`BlinnPhongMaterial material;
material.diffuseColor = diffuseColor.rgb;
material.specularColor = specular;
material.specularShininess = shininess;
material.specularStrength = specularStrength;`,Fw=`varying vec3 vViewPosition;
struct BlinnPhongMaterial {
	vec3 diffuseColor;
	vec3 specularColor;
	float specularShininess;
	float specularStrength;
};
void RE_Direct_BlinnPhong( const in IncidentLight directLight, const in vec3 geometryPosition, const in vec3 geometryNormal, const in vec3 geometryViewDir, const in vec3 geometryClearcoatNormal, const in BlinnPhongMaterial material, inout ReflectedLight reflectedLight ) {
	float dotNL = saturate( dot( geometryNormal, directLight.direction ) );
	vec3 irradiance = dotNL * directLight.color;
	reflectedLight.directDiffuse += irradiance * BRDF_Lambert( material.diffuseColor );
	reflectedLight.directSpecular += irradiance * BRDF_BlinnPhong( directLight.direction, geometryViewDir, geometryNormal, material.specularColor, material.specularShininess ) * material.specularStrength;
}
void RE_IndirectDiffuse_BlinnPhong( const in vec3 irradiance, const in vec3 geometryPosition, const in vec3 geometryNormal, const in vec3 geometryViewDir, const in vec3 geometryClearcoatNormal, const in BlinnPhongMaterial material, inout ReflectedLight reflectedLight ) {
	reflectedLight.indirectDiffuse += irradiance * BRDF_Lambert( material.diffuseColor );
}
#define RE_Direct				RE_Direct_BlinnPhong
#define RE_IndirectDiffuse		RE_IndirectDiffuse_BlinnPhong`,Ow=`PhysicalMaterial material;
material.diffuseColor = diffuseColor.rgb * ( 1.0 - metalnessFactor );
vec3 dxy = max( abs( dFdx( nonPerturbedNormal ) ), abs( dFdy( nonPerturbedNormal ) ) );
float geometryRoughness = max( max( dxy.x, dxy.y ), dxy.z );
material.roughness = max( roughnessFactor, 0.0525 );material.roughness += geometryRoughness;
material.roughness = min( material.roughness, 1.0 );
#ifdef IOR
	material.ior = ior;
	#ifdef USE_SPECULAR
		float specularIntensityFactor = specularIntensity;
		vec3 specularColorFactor = specularColor;
		#ifdef USE_SPECULAR_COLORMAP
			specularColorFactor *= texture2D( specularColorMap, vSpecularColorMapUv ).rgb;
		#endif
		#ifdef USE_SPECULAR_INTENSITYMAP
			specularIntensityFactor *= texture2D( specularIntensityMap, vSpecularIntensityMapUv ).a;
		#endif
		material.specularF90 = mix( specularIntensityFactor, 1.0, metalnessFactor );
	#else
		float specularIntensityFactor = 1.0;
		vec3 specularColorFactor = vec3( 1.0 );
		material.specularF90 = 1.0;
	#endif
	material.specularColor = mix( min( pow2( ( material.ior - 1.0 ) / ( material.ior + 1.0 ) ) * specularColorFactor, vec3( 1.0 ) ) * specularIntensityFactor, diffuseColor.rgb, metalnessFactor );
#else
	material.specularColor = mix( vec3( 0.04 ), diffuseColor.rgb, metalnessFactor );
	material.specularF90 = 1.0;
#endif
#ifdef USE_CLEARCOAT
	material.clearcoat = clearcoat;
	material.clearcoatRoughness = clearcoatRoughness;
	material.clearcoatF0 = vec3( 0.04 );
	material.clearcoatF90 = 1.0;
	#ifdef USE_CLEARCOATMAP
		material.clearcoat *= texture2D( clearcoatMap, vClearcoatMapUv ).x;
	#endif
	#ifdef USE_CLEARCOAT_ROUGHNESSMAP
		material.clearcoatRoughness *= texture2D( clearcoatRoughnessMap, vClearcoatRoughnessMapUv ).y;
	#endif
	material.clearcoat = saturate( material.clearcoat );	material.clearcoatRoughness = max( material.clearcoatRoughness, 0.0525 );
	material.clearcoatRoughness += geometryRoughness;
	material.clearcoatRoughness = min( material.clearcoatRoughness, 1.0 );
#endif
#ifdef USE_DISPERSION
	material.dispersion = dispersion;
#endif
#ifdef USE_IRIDESCENCE
	material.iridescence = iridescence;
	material.iridescenceIOR = iridescenceIOR;
	#ifdef USE_IRIDESCENCEMAP
		material.iridescence *= texture2D( iridescenceMap, vIridescenceMapUv ).r;
	#endif
	#ifdef USE_IRIDESCENCE_THICKNESSMAP
		material.iridescenceThickness = (iridescenceThicknessMaximum - iridescenceThicknessMinimum) * texture2D( iridescenceThicknessMap, vIridescenceThicknessMapUv ).g + iridescenceThicknessMinimum;
	#else
		material.iridescenceThickness = iridescenceThicknessMaximum;
	#endif
#endif
#ifdef USE_SHEEN
	material.sheenColor = sheenColor;
	#ifdef USE_SHEEN_COLORMAP
		material.sheenColor *= texture2D( sheenColorMap, vSheenColorMapUv ).rgb;
	#endif
	material.sheenRoughness = clamp( sheenRoughness, 0.07, 1.0 );
	#ifdef USE_SHEEN_ROUGHNESSMAP
		material.sheenRoughness *= texture2D( sheenRoughnessMap, vSheenRoughnessMapUv ).a;
	#endif
#endif
#ifdef USE_ANISOTROPY
	#ifdef USE_ANISOTROPYMAP
		mat2 anisotropyMat = mat2( anisotropyVector.x, anisotropyVector.y, - anisotropyVector.y, anisotropyVector.x );
		vec3 anisotropyPolar = texture2D( anisotropyMap, vAnisotropyMapUv ).rgb;
		vec2 anisotropyV = anisotropyMat * normalize( 2.0 * anisotropyPolar.rg - vec2( 1.0 ) ) * anisotropyPolar.b;
	#else
		vec2 anisotropyV = anisotropyVector;
	#endif
	material.anisotropy = length( anisotropyV );
	if( material.anisotropy == 0.0 ) {
		anisotropyV = vec2( 1.0, 0.0 );
	} else {
		anisotropyV /= material.anisotropy;
		material.anisotropy = saturate( material.anisotropy );
	}
	material.alphaT = mix( pow2( material.roughness ), 1.0, pow2( material.anisotropy ) );
	material.anisotropyT = tbn[ 0 ] * anisotropyV.x + tbn[ 1 ] * anisotropyV.y;
	material.anisotropyB = tbn[ 1 ] * anisotropyV.x - tbn[ 0 ] * anisotropyV.y;
#endif`,zw=`struct PhysicalMaterial {
	vec3 diffuseColor;
	float roughness;
	vec3 specularColor;
	float specularF90;
	float dispersion;
	#ifdef USE_CLEARCOAT
		float clearcoat;
		float clearcoatRoughness;
		vec3 clearcoatF0;
		float clearcoatF90;
	#endif
	#ifdef USE_IRIDESCENCE
		float iridescence;
		float iridescenceIOR;
		float iridescenceThickness;
		vec3 iridescenceFresnel;
		vec3 iridescenceF0;
	#endif
	#ifdef USE_SHEEN
		vec3 sheenColor;
		float sheenRoughness;
	#endif
	#ifdef IOR
		float ior;
	#endif
	#ifdef USE_TRANSMISSION
		float transmission;
		float transmissionAlpha;
		float thickness;
		float attenuationDistance;
		vec3 attenuationColor;
	#endif
	#ifdef USE_ANISOTROPY
		float anisotropy;
		float alphaT;
		vec3 anisotropyT;
		vec3 anisotropyB;
	#endif
};
vec3 clearcoatSpecularDirect = vec3( 0.0 );
vec3 clearcoatSpecularIndirect = vec3( 0.0 );
vec3 sheenSpecularDirect = vec3( 0.0 );
vec3 sheenSpecularIndirect = vec3(0.0 );
vec3 Schlick_to_F0( const in vec3 f, const in float f90, const in float dotVH ) {
    float x = clamp( 1.0 - dotVH, 0.0, 1.0 );
    float x2 = x * x;
    float x5 = clamp( x * x2 * x2, 0.0, 0.9999 );
    return ( f - vec3( f90 ) * x5 ) / ( 1.0 - x5 );
}
float V_GGX_SmithCorrelated( const in float alpha, const in float dotNL, const in float dotNV ) {
	float a2 = pow2( alpha );
	float gv = dotNL * sqrt( a2 + ( 1.0 - a2 ) * pow2( dotNV ) );
	float gl = dotNV * sqrt( a2 + ( 1.0 - a2 ) * pow2( dotNL ) );
	return 0.5 / max( gv + gl, EPSILON );
}
float D_GGX( const in float alpha, const in float dotNH ) {
	float a2 = pow2( alpha );
	float denom = pow2( dotNH ) * ( a2 - 1.0 ) + 1.0;
	return RECIPROCAL_PI * a2 / pow2( denom );
}
#ifdef USE_ANISOTROPY
	float V_GGX_SmithCorrelated_Anisotropic( const in float alphaT, const in float alphaB, const in float dotTV, const in float dotBV, const in float dotTL, const in float dotBL, const in float dotNV, const in float dotNL ) {
		float gv = dotNL * length( vec3( alphaT * dotTV, alphaB * dotBV, dotNV ) );
		float gl = dotNV * length( vec3( alphaT * dotTL, alphaB * dotBL, dotNL ) );
		float v = 0.5 / ( gv + gl );
		return saturate(v);
	}
	float D_GGX_Anisotropic( const in float alphaT, const in float alphaB, const in float dotNH, const in float dotTH, const in float dotBH ) {
		float a2 = alphaT * alphaB;
		highp vec3 v = vec3( alphaB * dotTH, alphaT * dotBH, a2 * dotNH );
		highp float v2 = dot( v, v );
		float w2 = a2 / v2;
		return RECIPROCAL_PI * a2 * pow2 ( w2 );
	}
#endif
#ifdef USE_CLEARCOAT
	vec3 BRDF_GGX_Clearcoat( const in vec3 lightDir, const in vec3 viewDir, const in vec3 normal, const in PhysicalMaterial material) {
		vec3 f0 = material.clearcoatF0;
		float f90 = material.clearcoatF90;
		float roughness = material.clearcoatRoughness;
		float alpha = pow2( roughness );
		vec3 halfDir = normalize( lightDir + viewDir );
		float dotNL = saturate( dot( normal, lightDir ) );
		float dotNV = saturate( dot( normal, viewDir ) );
		float dotNH = saturate( dot( normal, halfDir ) );
		float dotVH = saturate( dot( viewDir, halfDir ) );
		vec3 F = F_Schlick( f0, f90, dotVH );
		float V = V_GGX_SmithCorrelated( alpha, dotNL, dotNV );
		float D = D_GGX( alpha, dotNH );
		return F * ( V * D );
	}
#endif
vec3 BRDF_GGX( const in vec3 lightDir, const in vec3 viewDir, const in vec3 normal, const in PhysicalMaterial material ) {
	vec3 f0 = material.specularColor;
	float f90 = material.specularF90;
	float roughness = material.roughness;
	float alpha = pow2( roughness );
	vec3 halfDir = normalize( lightDir + viewDir );
	float dotNL = saturate( dot( normal, lightDir ) );
	float dotNV = saturate( dot( normal, viewDir ) );
	float dotNH = saturate( dot( normal, halfDir ) );
	float dotVH = saturate( dot( viewDir, halfDir ) );
	vec3 F = F_Schlick( f0, f90, dotVH );
	#ifdef USE_IRIDESCENCE
		F = mix( F, material.iridescenceFresnel, material.iridescence );
	#endif
	#ifdef USE_ANISOTROPY
		float dotTL = dot( material.anisotropyT, lightDir );
		float dotTV = dot( material.anisotropyT, viewDir );
		float dotTH = dot( material.anisotropyT, halfDir );
		float dotBL = dot( material.anisotropyB, lightDir );
		float dotBV = dot( material.anisotropyB, viewDir );
		float dotBH = dot( material.anisotropyB, halfDir );
		float V = V_GGX_SmithCorrelated_Anisotropic( material.alphaT, alpha, dotTV, dotBV, dotTL, dotBL, dotNV, dotNL );
		float D = D_GGX_Anisotropic( material.alphaT, alpha, dotNH, dotTH, dotBH );
	#else
		float V = V_GGX_SmithCorrelated( alpha, dotNL, dotNV );
		float D = D_GGX( alpha, dotNH );
	#endif
	return F * ( V * D );
}
vec2 LTC_Uv( const in vec3 N, const in vec3 V, const in float roughness ) {
	const float LUT_SIZE = 64.0;
	const float LUT_SCALE = ( LUT_SIZE - 1.0 ) / LUT_SIZE;
	const float LUT_BIAS = 0.5 / LUT_SIZE;
	float dotNV = saturate( dot( N, V ) );
	vec2 uv = vec2( roughness, sqrt( 1.0 - dotNV ) );
	uv = uv * LUT_SCALE + LUT_BIAS;
	return uv;
}
float LTC_ClippedSphereFormFactor( const in vec3 f ) {
	float l = length( f );
	return max( ( l * l + f.z ) / ( l + 1.0 ), 0.0 );
}
vec3 LTC_EdgeVectorFormFactor( const in vec3 v1, const in vec3 v2 ) {
	float x = dot( v1, v2 );
	float y = abs( x );
	float a = 0.8543985 + ( 0.4965155 + 0.0145206 * y ) * y;
	float b = 3.4175940 + ( 4.1616724 + y ) * y;
	float v = a / b;
	float theta_sintheta = ( x > 0.0 ) ? v : 0.5 * inversesqrt( max( 1.0 - x * x, 1e-7 ) ) - v;
	return cross( v1, v2 ) * theta_sintheta;
}
vec3 LTC_Evaluate( const in vec3 N, const in vec3 V, const in vec3 P, const in mat3 mInv, const in vec3 rectCoords[ 4 ] ) {
	vec3 v1 = rectCoords[ 1 ] - rectCoords[ 0 ];
	vec3 v2 = rectCoords[ 3 ] - rectCoords[ 0 ];
	vec3 lightNormal = cross( v1, v2 );
	if( dot( lightNormal, P - rectCoords[ 0 ] ) < 0.0 ) return vec3( 0.0 );
	vec3 T1, T2;
	T1 = normalize( V - N * dot( V, N ) );
	T2 = - cross( N, T1 );
	mat3 mat = mInv * transposeMat3( mat3( T1, T2, N ) );
	vec3 coords[ 4 ];
	coords[ 0 ] = mat * ( rectCoords[ 0 ] - P );
	coords[ 1 ] = mat * ( rectCoords[ 1 ] - P );
	coords[ 2 ] = mat * ( rectCoords[ 2 ] - P );
	coords[ 3 ] = mat * ( rectCoords[ 3 ] - P );
	coords[ 0 ] = normalize( coords[ 0 ] );
	coords[ 1 ] = normalize( coords[ 1 ] );
	coords[ 2 ] = normalize( coords[ 2 ] );
	coords[ 3 ] = normalize( coords[ 3 ] );
	vec3 vectorFormFactor = vec3( 0.0 );
	vectorFormFactor += LTC_EdgeVectorFormFactor( coords[ 0 ], coords[ 1 ] );
	vectorFormFactor += LTC_EdgeVectorFormFactor( coords[ 1 ], coords[ 2 ] );
	vectorFormFactor += LTC_EdgeVectorFormFactor( coords[ 2 ], coords[ 3 ] );
	vectorFormFactor += LTC_EdgeVectorFormFactor( coords[ 3 ], coords[ 0 ] );
	float result = LTC_ClippedSphereFormFactor( vectorFormFactor );
	return vec3( result );
}
#if defined( USE_SHEEN )
float D_Charlie( float roughness, float dotNH ) {
	float alpha = pow2( roughness );
	float invAlpha = 1.0 / alpha;
	float cos2h = dotNH * dotNH;
	float sin2h = max( 1.0 - cos2h, 0.0078125 );
	return ( 2.0 + invAlpha ) * pow( sin2h, invAlpha * 0.5 ) / ( 2.0 * PI );
}
float V_Neubelt( float dotNV, float dotNL ) {
	return saturate( 1.0 / ( 4.0 * ( dotNL + dotNV - dotNL * dotNV ) ) );
}
vec3 BRDF_Sheen( const in vec3 lightDir, const in vec3 viewDir, const in vec3 normal, vec3 sheenColor, const in float sheenRoughness ) {
	vec3 halfDir = normalize( lightDir + viewDir );
	float dotNL = saturate( dot( normal, lightDir ) );
	float dotNV = saturate( dot( normal, viewDir ) );
	float dotNH = saturate( dot( normal, halfDir ) );
	float D = D_Charlie( sheenRoughness, dotNH );
	float V = V_Neubelt( dotNV, dotNL );
	return sheenColor * ( D * V );
}
#endif
float IBLSheenBRDF( const in vec3 normal, const in vec3 viewDir, const in float roughness ) {
	float dotNV = saturate( dot( normal, viewDir ) );
	float r2 = roughness * roughness;
	float a = roughness < 0.25 ? -339.2 * r2 + 161.4 * roughness - 25.9 : -8.48 * r2 + 14.3 * roughness - 9.95;
	float b = roughness < 0.25 ? 44.0 * r2 - 23.7 * roughness + 3.26 : 1.97 * r2 - 3.27 * roughness + 0.72;
	float DG = exp( a * dotNV + b ) + ( roughness < 0.25 ? 0.0 : 0.1 * ( roughness - 0.25 ) );
	return saturate( DG * RECIPROCAL_PI );
}
vec2 DFGApprox( const in vec3 normal, const in vec3 viewDir, const in float roughness ) {
	float dotNV = saturate( dot( normal, viewDir ) );
	const vec4 c0 = vec4( - 1, - 0.0275, - 0.572, 0.022 );
	const vec4 c1 = vec4( 1, 0.0425, 1.04, - 0.04 );
	vec4 r = roughness * c0 + c1;
	float a004 = min( r.x * r.x, exp2( - 9.28 * dotNV ) ) * r.x + r.y;
	vec2 fab = vec2( - 1.04, 1.04 ) * a004 + r.zw;
	return fab;
}
vec3 EnvironmentBRDF( const in vec3 normal, const in vec3 viewDir, const in vec3 specularColor, const in float specularF90, const in float roughness ) {
	vec2 fab = DFGApprox( normal, viewDir, roughness );
	return specularColor * fab.x + specularF90 * fab.y;
}
#ifdef USE_IRIDESCENCE
void computeMultiscatteringIridescence( const in vec3 normal, const in vec3 viewDir, const in vec3 specularColor, const in float specularF90, const in float iridescence, const in vec3 iridescenceF0, const in float roughness, inout vec3 singleScatter, inout vec3 multiScatter ) {
#else
void computeMultiscattering( const in vec3 normal, const in vec3 viewDir, const in vec3 specularColor, const in float specularF90, const in float roughness, inout vec3 singleScatter, inout vec3 multiScatter ) {
#endif
	vec2 fab = DFGApprox( normal, viewDir, roughness );
	#ifdef USE_IRIDESCENCE
		vec3 Fr = mix( specularColor, iridescenceF0, iridescence );
	#else
		vec3 Fr = specularColor;
	#endif
	vec3 FssEss = Fr * fab.x + specularF90 * fab.y;
	float Ess = fab.x + fab.y;
	float Ems = 1.0 - Ess;
	vec3 Favg = Fr + ( 1.0 - Fr ) * 0.047619;	vec3 Fms = FssEss * Favg / ( 1.0 - Ems * Favg );
	singleScatter += FssEss;
	multiScatter += Fms * Ems;
}
#if NUM_RECT_AREA_LIGHTS > 0
	void RE_Direct_RectArea_Physical( const in RectAreaLight rectAreaLight, const in vec3 geometryPosition, const in vec3 geometryNormal, const in vec3 geometryViewDir, const in vec3 geometryClearcoatNormal, const in PhysicalMaterial material, inout ReflectedLight reflectedLight ) {
		vec3 normal = geometryNormal;
		vec3 viewDir = geometryViewDir;
		vec3 position = geometryPosition;
		vec3 lightPos = rectAreaLight.position;
		vec3 halfWidth = rectAreaLight.halfWidth;
		vec3 halfHeight = rectAreaLight.halfHeight;
		vec3 lightColor = rectAreaLight.color;
		float roughness = material.roughness;
		vec3 rectCoords[ 4 ];
		rectCoords[ 0 ] = lightPos + halfWidth - halfHeight;		rectCoords[ 1 ] = lightPos - halfWidth - halfHeight;
		rectCoords[ 2 ] = lightPos - halfWidth + halfHeight;
		rectCoords[ 3 ] = lightPos + halfWidth + halfHeight;
		vec2 uv = LTC_Uv( normal, viewDir, roughness );
		vec4 t1 = texture2D( ltc_1, uv );
		vec4 t2 = texture2D( ltc_2, uv );
		mat3 mInv = mat3(
			vec3( t1.x, 0, t1.y ),
			vec3(    0, 1,    0 ),
			vec3( t1.z, 0, t1.w )
		);
		vec3 fresnel = ( material.specularColor * t2.x + ( vec3( 1.0 ) - material.specularColor ) * t2.y );
		reflectedLight.directSpecular += lightColor * fresnel * LTC_Evaluate( normal, viewDir, position, mInv, rectCoords );
		reflectedLight.directDiffuse += lightColor * material.diffuseColor * LTC_Evaluate( normal, viewDir, position, mat3( 1.0 ), rectCoords );
	}
#endif
void RE_Direct_Physical( const in IncidentLight directLight, const in vec3 geometryPosition, const in vec3 geometryNormal, const in vec3 geometryViewDir, const in vec3 geometryClearcoatNormal, const in PhysicalMaterial material, inout ReflectedLight reflectedLight ) {
	float dotNL = saturate( dot( geometryNormal, directLight.direction ) );
	vec3 irradiance = dotNL * directLight.color;
	#ifdef USE_CLEARCOAT
		float dotNLcc = saturate( dot( geometryClearcoatNormal, directLight.direction ) );
		vec3 ccIrradiance = dotNLcc * directLight.color;
		clearcoatSpecularDirect += ccIrradiance * BRDF_GGX_Clearcoat( directLight.direction, geometryViewDir, geometryClearcoatNormal, material );
	#endif
	#ifdef USE_SHEEN
		sheenSpecularDirect += irradiance * BRDF_Sheen( directLight.direction, geometryViewDir, geometryNormal, material.sheenColor, material.sheenRoughness );
	#endif
	reflectedLight.directSpecular += irradiance * BRDF_GGX( directLight.direction, geometryViewDir, geometryNormal, material );
	reflectedLight.directDiffuse += irradiance * BRDF_Lambert( material.diffuseColor );
}
void RE_IndirectDiffuse_Physical( const in vec3 irradiance, const in vec3 geometryPosition, const in vec3 geometryNormal, const in vec3 geometryViewDir, const in vec3 geometryClearcoatNormal, const in PhysicalMaterial material, inout ReflectedLight reflectedLight ) {
	reflectedLight.indirectDiffuse += irradiance * BRDF_Lambert( material.diffuseColor );
}
void RE_IndirectSpecular_Physical( const in vec3 radiance, const in vec3 irradiance, const in vec3 clearcoatRadiance, const in vec3 geometryPosition, const in vec3 geometryNormal, const in vec3 geometryViewDir, const in vec3 geometryClearcoatNormal, const in PhysicalMaterial material, inout ReflectedLight reflectedLight) {
	#ifdef USE_CLEARCOAT
		clearcoatSpecularIndirect += clearcoatRadiance * EnvironmentBRDF( geometryClearcoatNormal, geometryViewDir, material.clearcoatF0, material.clearcoatF90, material.clearcoatRoughness );
	#endif
	#ifdef USE_SHEEN
		sheenSpecularIndirect += irradiance * material.sheenColor * IBLSheenBRDF( geometryNormal, geometryViewDir, material.sheenRoughness );
	#endif
	vec3 singleScattering = vec3( 0.0 );
	vec3 multiScattering = vec3( 0.0 );
	vec3 cosineWeightedIrradiance = irradiance * RECIPROCAL_PI;
	#ifdef USE_IRIDESCENCE
		computeMultiscatteringIridescence( geometryNormal, geometryViewDir, material.specularColor, material.specularF90, material.iridescence, material.iridescenceFresnel, material.roughness, singleScattering, multiScattering );
	#else
		computeMultiscattering( geometryNormal, geometryViewDir, material.specularColor, material.specularF90, material.roughness, singleScattering, multiScattering );
	#endif
	vec3 totalScattering = singleScattering + multiScattering;
	vec3 diffuse = material.diffuseColor * ( 1.0 - max( max( totalScattering.r, totalScattering.g ), totalScattering.b ) );
	reflectedLight.indirectSpecular += radiance * singleScattering;
	reflectedLight.indirectSpecular += multiScattering * cosineWeightedIrradiance;
	reflectedLight.indirectDiffuse += diffuse * cosineWeightedIrradiance;
}
#define RE_Direct				RE_Direct_Physical
#define RE_Direct_RectArea		RE_Direct_RectArea_Physical
#define RE_IndirectDiffuse		RE_IndirectDiffuse_Physical
#define RE_IndirectSpecular		RE_IndirectSpecular_Physical
float computeSpecularOcclusion( const in float dotNV, const in float ambientOcclusion, const in float roughness ) {
	return saturate( pow( dotNV + ambientOcclusion, exp2( - 16.0 * roughness - 1.0 ) ) - 1.0 + ambientOcclusion );
}`,Bw=`
vec3 geometryPosition = - vViewPosition;
vec3 geometryNormal = normal;
vec3 geometryViewDir = ( isOrthographic ) ? vec3( 0, 0, 1 ) : normalize( vViewPosition );
vec3 geometryClearcoatNormal = vec3( 0.0 );
#ifdef USE_CLEARCOAT
	geometryClearcoatNormal = clearcoatNormal;
#endif
#ifdef USE_IRIDESCENCE
	float dotNVi = saturate( dot( normal, geometryViewDir ) );
	if ( material.iridescenceThickness == 0.0 ) {
		material.iridescence = 0.0;
	} else {
		material.iridescence = saturate( material.iridescence );
	}
	if ( material.iridescence > 0.0 ) {
		material.iridescenceFresnel = evalIridescence( 1.0, material.iridescenceIOR, dotNVi, material.iridescenceThickness, material.specularColor );
		material.iridescenceF0 = Schlick_to_F0( material.iridescenceFresnel, 1.0, dotNVi );
	}
#endif
IncidentLight directLight;
#if ( NUM_POINT_LIGHTS > 0 ) && defined( RE_Direct )
	PointLight pointLight;
	#if defined( USE_SHADOWMAP ) && NUM_POINT_LIGHT_SHADOWS > 0
	PointLightShadow pointLightShadow;
	#endif
	#pragma unroll_loop_start
	for ( int i = 0; i < NUM_POINT_LIGHTS; i ++ ) {
		pointLight = pointLights[ i ];
		getPointLightInfo( pointLight, geometryPosition, directLight );
		#if defined( USE_SHADOWMAP ) && ( UNROLLED_LOOP_INDEX < NUM_POINT_LIGHT_SHADOWS )
		pointLightShadow = pointLightShadows[ i ];
		directLight.color *= ( directLight.visible && receiveShadow ) ? getPointShadow( pointShadowMap[ i ], pointLightShadow.shadowMapSize, pointLightShadow.shadowIntensity, pointLightShadow.shadowBias, pointLightShadow.shadowRadius, vPointShadowCoord[ i ], pointLightShadow.shadowCameraNear, pointLightShadow.shadowCameraFar ) : 1.0;
		#endif
		RE_Direct( directLight, geometryPosition, geometryNormal, geometryViewDir, geometryClearcoatNormal, material, reflectedLight );
	}
	#pragma unroll_loop_end
#endif
#if ( NUM_SPOT_LIGHTS > 0 ) && defined( RE_Direct )
	SpotLight spotLight;
	vec4 spotColor;
	vec3 spotLightCoord;
	bool inSpotLightMap;
	#if defined( USE_SHADOWMAP ) && NUM_SPOT_LIGHT_SHADOWS > 0
	SpotLightShadow spotLightShadow;
	#endif
	#pragma unroll_loop_start
	for ( int i = 0; i < NUM_SPOT_LIGHTS; i ++ ) {
		spotLight = spotLights[ i ];
		getSpotLightInfo( spotLight, geometryPosition, directLight );
		#if ( UNROLLED_LOOP_INDEX < NUM_SPOT_LIGHT_SHADOWS_WITH_MAPS )
		#define SPOT_LIGHT_MAP_INDEX UNROLLED_LOOP_INDEX
		#elif ( UNROLLED_LOOP_INDEX < NUM_SPOT_LIGHT_SHADOWS )
		#define SPOT_LIGHT_MAP_INDEX NUM_SPOT_LIGHT_MAPS
		#else
		#define SPOT_LIGHT_MAP_INDEX ( UNROLLED_LOOP_INDEX - NUM_SPOT_LIGHT_SHADOWS + NUM_SPOT_LIGHT_SHADOWS_WITH_MAPS )
		#endif
		#if ( SPOT_LIGHT_MAP_INDEX < NUM_SPOT_LIGHT_MAPS )
			spotLightCoord = vSpotLightCoord[ i ].xyz / vSpotLightCoord[ i ].w;
			inSpotLightMap = all( lessThan( abs( spotLightCoord * 2. - 1. ), vec3( 1.0 ) ) );
			spotColor = texture2D( spotLightMap[ SPOT_LIGHT_MAP_INDEX ], spotLightCoord.xy );
			directLight.color = inSpotLightMap ? directLight.color * spotColor.rgb : directLight.color;
		#endif
		#undef SPOT_LIGHT_MAP_INDEX
		#if defined( USE_SHADOWMAP ) && ( UNROLLED_LOOP_INDEX < NUM_SPOT_LIGHT_SHADOWS )
		spotLightShadow = spotLightShadows[ i ];
		directLight.color *= ( directLight.visible && receiveShadow ) ? getShadow( spotShadowMap[ i ], spotLightShadow.shadowMapSize, spotLightShadow.shadowIntensity, spotLightShadow.shadowBias, spotLightShadow.shadowRadius, vSpotLightCoord[ i ] ) : 1.0;
		#endif
		RE_Direct( directLight, geometryPosition, geometryNormal, geometryViewDir, geometryClearcoatNormal, material, reflectedLight );
	}
	#pragma unroll_loop_end
#endif
#if ( NUM_DIR_LIGHTS > 0 ) && defined( RE_Direct )
	DirectionalLight directionalLight;
	#if defined( USE_SHADOWMAP ) && NUM_DIR_LIGHT_SHADOWS > 0
	DirectionalLightShadow directionalLightShadow;
	#endif
	#pragma unroll_loop_start
	for ( int i = 0; i < NUM_DIR_LIGHTS; i ++ ) {
		directionalLight = directionalLights[ i ];
		getDirectionalLightInfo( directionalLight, directLight );
		#if defined( USE_SHADOWMAP ) && ( UNROLLED_LOOP_INDEX < NUM_DIR_LIGHT_SHADOWS )
		directionalLightShadow = directionalLightShadows[ i ];
		directLight.color *= ( directLight.visible && receiveShadow ) ? getShadow( directionalShadowMap[ i ], directionalLightShadow.shadowMapSize, directionalLightShadow.shadowIntensity, directionalLightShadow.shadowBias, directionalLightShadow.shadowRadius, vDirectionalShadowCoord[ i ] ) : 1.0;
		#endif
		RE_Direct( directLight, geometryPosition, geometryNormal, geometryViewDir, geometryClearcoatNormal, material, reflectedLight );
	}
	#pragma unroll_loop_end
#endif
#if ( NUM_RECT_AREA_LIGHTS > 0 ) && defined( RE_Direct_RectArea )
	RectAreaLight rectAreaLight;
	#pragma unroll_loop_start
	for ( int i = 0; i < NUM_RECT_AREA_LIGHTS; i ++ ) {
		rectAreaLight = rectAreaLights[ i ];
		RE_Direct_RectArea( rectAreaLight, geometryPosition, geometryNormal, geometryViewDir, geometryClearcoatNormal, material, reflectedLight );
	}
	#pragma unroll_loop_end
#endif
#if defined( RE_IndirectDiffuse )
	vec3 iblIrradiance = vec3( 0.0 );
	vec3 irradiance = getAmbientLightIrradiance( ambientLightColor );
	#if defined( USE_LIGHT_PROBES )
		irradiance += getLightProbeIrradiance( lightProbe, geometryNormal );
	#endif
	#if ( NUM_HEMI_LIGHTS > 0 )
		#pragma unroll_loop_start
		for ( int i = 0; i < NUM_HEMI_LIGHTS; i ++ ) {
			irradiance += getHemisphereLightIrradiance( hemisphereLights[ i ], geometryNormal );
		}
		#pragma unroll_loop_end
	#endif
#endif
#if defined( RE_IndirectSpecular )
	vec3 radiance = vec3( 0.0 );
	vec3 clearcoatRadiance = vec3( 0.0 );
#endif`,Hw=`#if defined( RE_IndirectDiffuse )
	#ifdef USE_LIGHTMAP
		vec4 lightMapTexel = texture2D( lightMap, vLightMapUv );
		vec3 lightMapIrradiance = lightMapTexel.rgb * lightMapIntensity;
		irradiance += lightMapIrradiance;
	#endif
	#if defined( USE_ENVMAP ) && defined( STANDARD ) && defined( ENVMAP_TYPE_CUBE_UV )
		iblIrradiance += getIBLIrradiance( geometryNormal );
	#endif
#endif
#if defined( USE_ENVMAP ) && defined( RE_IndirectSpecular )
	#ifdef USE_ANISOTROPY
		radiance += getIBLAnisotropyRadiance( geometryViewDir, geometryNormal, material.roughness, material.anisotropyB, material.anisotropy );
	#else
		radiance += getIBLRadiance( geometryViewDir, geometryNormal, material.roughness );
	#endif
	#ifdef USE_CLEARCOAT
		clearcoatRadiance += getIBLRadiance( geometryViewDir, geometryClearcoatNormal, material.clearcoatRoughness );
	#endif
#endif`,Vw=`#if defined( RE_IndirectDiffuse )
	RE_IndirectDiffuse( irradiance, geometryPosition, geometryNormal, geometryViewDir, geometryClearcoatNormal, material, reflectedLight );
#endif
#if defined( RE_IndirectSpecular )
	RE_IndirectSpecular( radiance, iblIrradiance, clearcoatRadiance, geometryPosition, geometryNormal, geometryViewDir, geometryClearcoatNormal, material, reflectedLight );
#endif`,Gw=`#if defined( USE_LOGDEPTHBUF )
	gl_FragDepth = vIsPerspective == 0.0 ? gl_FragCoord.z : log2( vFragDepth ) * logDepthBufFC * 0.5;
#endif`,Ww=`#if defined( USE_LOGDEPTHBUF )
	uniform float logDepthBufFC;
	varying float vFragDepth;
	varying float vIsPerspective;
#endif`,Xw=`#ifdef USE_LOGDEPTHBUF
	varying float vFragDepth;
	varying float vIsPerspective;
#endif`,jw=`#ifdef USE_LOGDEPTHBUF
	vFragDepth = 1.0 + gl_Position.w;
	vIsPerspective = float( isPerspectiveMatrix( projectionMatrix ) );
#endif`,Yw=`#ifdef USE_MAP
	vec4 sampledDiffuseColor = texture2D( map, vMapUv );
	#ifdef DECODE_VIDEO_TEXTURE
		sampledDiffuseColor = sRGBTransferEOTF( sampledDiffuseColor );
	#endif
	diffuseColor *= sampledDiffuseColor;
#endif`,qw=`#ifdef USE_MAP
	uniform sampler2D map;
#endif`,$w=`#if defined( USE_MAP ) || defined( USE_ALPHAMAP )
	#if defined( USE_POINTS_UV )
		vec2 uv = vUv;
	#else
		vec2 uv = ( uvTransform * vec3( gl_PointCoord.x, 1.0 - gl_PointCoord.y, 1 ) ).xy;
	#endif
#endif
#ifdef USE_MAP
	diffuseColor *= texture2D( map, uv );
#endif
#ifdef USE_ALPHAMAP
	diffuseColor.a *= texture2D( alphaMap, uv ).g;
#endif`,Kw=`#if defined( USE_POINTS_UV )
	varying vec2 vUv;
#else
	#if defined( USE_MAP ) || defined( USE_ALPHAMAP )
		uniform mat3 uvTransform;
	#endif
#endif
#ifdef USE_MAP
	uniform sampler2D map;
#endif
#ifdef USE_ALPHAMAP
	uniform sampler2D alphaMap;
#endif`,Zw=`float metalnessFactor = metalness;
#ifdef USE_METALNESSMAP
	vec4 texelMetalness = texture2D( metalnessMap, vMetalnessMapUv );
	metalnessFactor *= texelMetalness.b;
#endif`,Jw=`#ifdef USE_METALNESSMAP
	uniform sampler2D metalnessMap;
#endif`,Qw=`#ifdef USE_INSTANCING_MORPH
	float morphTargetInfluences[ MORPHTARGETS_COUNT ];
	float morphTargetBaseInfluence = texelFetch( morphTexture, ivec2( 0, gl_InstanceID ), 0 ).r;
	for ( int i = 0; i < MORPHTARGETS_COUNT; i ++ ) {
		morphTargetInfluences[i] =  texelFetch( morphTexture, ivec2( i + 1, gl_InstanceID ), 0 ).r;
	}
#endif`,e1=`#if defined( USE_MORPHCOLORS )
	vColor *= morphTargetBaseInfluence;
	for ( int i = 0; i < MORPHTARGETS_COUNT; i ++ ) {
		#if defined( USE_COLOR_ALPHA )
			if ( morphTargetInfluences[ i ] != 0.0 ) vColor += getMorph( gl_VertexID, i, 2 ) * morphTargetInfluences[ i ];
		#elif defined( USE_COLOR )
			if ( morphTargetInfluences[ i ] != 0.0 ) vColor += getMorph( gl_VertexID, i, 2 ).rgb * morphTargetInfluences[ i ];
		#endif
	}
#endif`,t1=`#ifdef USE_MORPHNORMALS
	objectNormal *= morphTargetBaseInfluence;
	for ( int i = 0; i < MORPHTARGETS_COUNT; i ++ ) {
		if ( morphTargetInfluences[ i ] != 0.0 ) objectNormal += getMorph( gl_VertexID, i, 1 ).xyz * morphTargetInfluences[ i ];
	}
#endif`,n1=`#ifdef USE_MORPHTARGETS
	#ifndef USE_INSTANCING_MORPH
		uniform float morphTargetBaseInfluence;
		uniform float morphTargetInfluences[ MORPHTARGETS_COUNT ];
	#endif
	uniform sampler2DArray morphTargetsTexture;
	uniform ivec2 morphTargetsTextureSize;
	vec4 getMorph( const in int vertexIndex, const in int morphTargetIndex, const in int offset ) {
		int texelIndex = vertexIndex * MORPHTARGETS_TEXTURE_STRIDE + offset;
		int y = texelIndex / morphTargetsTextureSize.x;
		int x = texelIndex - y * morphTargetsTextureSize.x;
		ivec3 morphUV = ivec3( x, y, morphTargetIndex );
		return texelFetch( morphTargetsTexture, morphUV, 0 );
	}
#endif`,i1=`#ifdef USE_MORPHTARGETS
	transformed *= morphTargetBaseInfluence;
	for ( int i = 0; i < MORPHTARGETS_COUNT; i ++ ) {
		if ( morphTargetInfluences[ i ] != 0.0 ) transformed += getMorph( gl_VertexID, i, 0 ).xyz * morphTargetInfluences[ i ];
	}
#endif`,r1=`float faceDirection = gl_FrontFacing ? 1.0 : - 1.0;
#ifdef FLAT_SHADED
	vec3 fdx = dFdx( vViewPosition );
	vec3 fdy = dFdy( vViewPosition );
	vec3 normal = normalize( cross( fdx, fdy ) );
#else
	vec3 normal = normalize( vNormal );
	#ifdef DOUBLE_SIDED
		normal *= faceDirection;
	#endif
#endif
#if defined( USE_NORMALMAP_TANGENTSPACE ) || defined( USE_CLEARCOAT_NORMALMAP ) || defined( USE_ANISOTROPY )
	#ifdef USE_TANGENT
		mat3 tbn = mat3( normalize( vTangent ), normalize( vBitangent ), normal );
	#else
		mat3 tbn = getTangentFrame( - vViewPosition, normal,
		#if defined( USE_NORMALMAP )
			vNormalMapUv
		#elif defined( USE_CLEARCOAT_NORMALMAP )
			vClearcoatNormalMapUv
		#else
			vUv
		#endif
		);
	#endif
	#if defined( DOUBLE_SIDED ) && ! defined( FLAT_SHADED )
		tbn[0] *= faceDirection;
		tbn[1] *= faceDirection;
	#endif
#endif
#ifdef USE_CLEARCOAT_NORMALMAP
	#ifdef USE_TANGENT
		mat3 tbn2 = mat3( normalize( vTangent ), normalize( vBitangent ), normal );
	#else
		mat3 tbn2 = getTangentFrame( - vViewPosition, normal, vClearcoatNormalMapUv );
	#endif
	#if defined( DOUBLE_SIDED ) && ! defined( FLAT_SHADED )
		tbn2[0] *= faceDirection;
		tbn2[1] *= faceDirection;
	#endif
#endif
vec3 nonPerturbedNormal = normal;`,o1=`#ifdef USE_NORMALMAP_OBJECTSPACE
	normal = texture2D( normalMap, vNormalMapUv ).xyz * 2.0 - 1.0;
	#ifdef FLIP_SIDED
		normal = - normal;
	#endif
	#ifdef DOUBLE_SIDED
		normal = normal * faceDirection;
	#endif
	normal = normalize( normalMatrix * normal );
#elif defined( USE_NORMALMAP_TANGENTSPACE )
	vec3 mapN = texture2D( normalMap, vNormalMapUv ).xyz * 2.0 - 1.0;
	mapN.xy *= normalScale;
	normal = normalize( tbn * mapN );
#elif defined( USE_BUMPMAP )
	normal = perturbNormalArb( - vViewPosition, normal, dHdxy_fwd(), faceDirection );
#endif`,s1=`#ifndef FLAT_SHADED
	varying vec3 vNormal;
	#ifdef USE_TANGENT
		varying vec3 vTangent;
		varying vec3 vBitangent;
	#endif
#endif`,a1=`#ifndef FLAT_SHADED
	varying vec3 vNormal;
	#ifdef USE_TANGENT
		varying vec3 vTangent;
		varying vec3 vBitangent;
	#endif
#endif`,l1=`#ifndef FLAT_SHADED
	vNormal = normalize( transformedNormal );
	#ifdef USE_TANGENT
		vTangent = normalize( transformedTangent );
		vBitangent = normalize( cross( vNormal, vTangent ) * tangent.w );
	#endif
#endif`,c1=`#ifdef USE_NORMALMAP
	uniform sampler2D normalMap;
	uniform vec2 normalScale;
#endif
#ifdef USE_NORMALMAP_OBJECTSPACE
	uniform mat3 normalMatrix;
#endif
#if ! defined ( USE_TANGENT ) && ( defined ( USE_NORMALMAP_TANGENTSPACE ) || defined ( USE_CLEARCOAT_NORMALMAP ) || defined( USE_ANISOTROPY ) )
	mat3 getTangentFrame( vec3 eye_pos, vec3 surf_norm, vec2 uv ) {
		vec3 q0 = dFdx( eye_pos.xyz );
		vec3 q1 = dFdy( eye_pos.xyz );
		vec2 st0 = dFdx( uv.st );
		vec2 st1 = dFdy( uv.st );
		vec3 N = surf_norm;
		vec3 q1perp = cross( q1, N );
		vec3 q0perp = cross( N, q0 );
		vec3 T = q1perp * st0.x + q0perp * st1.x;
		vec3 B = q1perp * st0.y + q0perp * st1.y;
		float det = max( dot( T, T ), dot( B, B ) );
		float scale = ( det == 0.0 ) ? 0.0 : inversesqrt( det );
		return mat3( T * scale, B * scale, N );
	}
#endif`,u1=`#ifdef USE_CLEARCOAT
	vec3 clearcoatNormal = nonPerturbedNormal;
#endif`,d1=`#ifdef USE_CLEARCOAT_NORMALMAP
	vec3 clearcoatMapN = texture2D( clearcoatNormalMap, vClearcoatNormalMapUv ).xyz * 2.0 - 1.0;
	clearcoatMapN.xy *= clearcoatNormalScale;
	clearcoatNormal = normalize( tbn2 * clearcoatMapN );
#endif`,h1=`#ifdef USE_CLEARCOATMAP
	uniform sampler2D clearcoatMap;
#endif
#ifdef USE_CLEARCOAT_NORMALMAP
	uniform sampler2D clearcoatNormalMap;
	uniform vec2 clearcoatNormalScale;
#endif
#ifdef USE_CLEARCOAT_ROUGHNESSMAP
	uniform sampler2D clearcoatRoughnessMap;
#endif`,f1=`#ifdef USE_IRIDESCENCEMAP
	uniform sampler2D iridescenceMap;
#endif
#ifdef USE_IRIDESCENCE_THICKNESSMAP
	uniform sampler2D iridescenceThicknessMap;
#endif`,p1=`#ifdef OPAQUE
diffuseColor.a = 1.0;
#endif
#ifdef USE_TRANSMISSION
diffuseColor.a *= material.transmissionAlpha;
#endif
gl_FragColor = vec4( outgoingLight, diffuseColor.a );`,m1=`vec3 packNormalToRGB( const in vec3 normal ) {
	return normalize( normal ) * 0.5 + 0.5;
}
vec3 unpackRGBToNormal( const in vec3 rgb ) {
	return 2.0 * rgb.xyz - 1.0;
}
const float PackUpscale = 256. / 255.;const float UnpackDownscale = 255. / 256.;const float ShiftRight8 = 1. / 256.;
const float Inv255 = 1. / 255.;
const vec4 PackFactors = vec4( 1.0, 256.0, 256.0 * 256.0, 256.0 * 256.0 * 256.0 );
const vec2 UnpackFactors2 = vec2( UnpackDownscale, 1.0 / PackFactors.g );
const vec3 UnpackFactors3 = vec3( UnpackDownscale / PackFactors.rg, 1.0 / PackFactors.b );
const vec4 UnpackFactors4 = vec4( UnpackDownscale / PackFactors.rgb, 1.0 / PackFactors.a );
vec4 packDepthToRGBA( const in float v ) {
	if( v <= 0.0 )
		return vec4( 0., 0., 0., 0. );
	if( v >= 1.0 )
		return vec4( 1., 1., 1., 1. );
	float vuf;
	float af = modf( v * PackFactors.a, vuf );
	float bf = modf( vuf * ShiftRight8, vuf );
	float gf = modf( vuf * ShiftRight8, vuf );
	return vec4( vuf * Inv255, gf * PackUpscale, bf * PackUpscale, af );
}
vec3 packDepthToRGB( const in float v ) {
	if( v <= 0.0 )
		return vec3( 0., 0., 0. );
	if( v >= 1.0 )
		return vec3( 1., 1., 1. );
	float vuf;
	float bf = modf( v * PackFactors.b, vuf );
	float gf = modf( vuf * ShiftRight8, vuf );
	return vec3( vuf * Inv255, gf * PackUpscale, bf );
}
vec2 packDepthToRG( const in float v ) {
	if( v <= 0.0 )
		return vec2( 0., 0. );
	if( v >= 1.0 )
		return vec2( 1., 1. );
	float vuf;
	float gf = modf( v * 256., vuf );
	return vec2( vuf * Inv255, gf );
}
float unpackRGBAToDepth( const in vec4 v ) {
	return dot( v, UnpackFactors4 );
}
float unpackRGBToDepth( const in vec3 v ) {
	return dot( v, UnpackFactors3 );
}
float unpackRGToDepth( const in vec2 v ) {
	return v.r * UnpackFactors2.r + v.g * UnpackFactors2.g;
}
vec4 pack2HalfToRGBA( const in vec2 v ) {
	vec4 r = vec4( v.x, fract( v.x * 255.0 ), v.y, fract( v.y * 255.0 ) );
	return vec4( r.x - r.y / 255.0, r.y, r.z - r.w / 255.0, r.w );
}
vec2 unpackRGBATo2Half( const in vec4 v ) {
	return vec2( v.x + ( v.y / 255.0 ), v.z + ( v.w / 255.0 ) );
}
float viewZToOrthographicDepth( const in float viewZ, const in float near, const in float far ) {
	return ( viewZ + near ) / ( near - far );
}
float orthographicDepthToViewZ( const in float depth, const in float near, const in float far ) {
	return depth * ( near - far ) - near;
}
float viewZToPerspectiveDepth( const in float viewZ, const in float near, const in float far ) {
	return ( ( near + viewZ ) * far ) / ( ( far - near ) * viewZ );
}
float perspectiveDepthToViewZ( const in float depth, const in float near, const in float far ) {
	return ( near * far ) / ( ( far - near ) * depth - far );
}`,g1=`#ifdef PREMULTIPLIED_ALPHA
	gl_FragColor.rgb *= gl_FragColor.a;
#endif`,v1=`vec4 mvPosition = vec4( transformed, 1.0 );
#ifdef USE_BATCHING
	mvPosition = batchingMatrix * mvPosition;
#endif
#ifdef USE_INSTANCING
	mvPosition = instanceMatrix * mvPosition;
#endif
mvPosition = modelViewMatrix * mvPosition;
gl_Position = projectionMatrix * mvPosition;`,y1=`#ifdef DITHERING
	gl_FragColor.rgb = dithering( gl_FragColor.rgb );
#endif`,_1=`#ifdef DITHERING
	vec3 dithering( vec3 color ) {
		float grid_position = rand( gl_FragCoord.xy );
		vec3 dither_shift_RGB = vec3( 0.25 / 255.0, -0.25 / 255.0, 0.25 / 255.0 );
		dither_shift_RGB = mix( 2.0 * dither_shift_RGB, -2.0 * dither_shift_RGB, grid_position );
		return color + dither_shift_RGB;
	}
#endif`,x1=`float roughnessFactor = roughness;
#ifdef USE_ROUGHNESSMAP
	vec4 texelRoughness = texture2D( roughnessMap, vRoughnessMapUv );
	roughnessFactor *= texelRoughness.g;
#endif`,S1=`#ifdef USE_ROUGHNESSMAP
	uniform sampler2D roughnessMap;
#endif`,M1=`#if NUM_SPOT_LIGHT_COORDS > 0
	varying vec4 vSpotLightCoord[ NUM_SPOT_LIGHT_COORDS ];
#endif
#if NUM_SPOT_LIGHT_MAPS > 0
	uniform sampler2D spotLightMap[ NUM_SPOT_LIGHT_MAPS ];
#endif
#ifdef USE_SHADOWMAP
	#if NUM_DIR_LIGHT_SHADOWS > 0
		uniform sampler2D directionalShadowMap[ NUM_DIR_LIGHT_SHADOWS ];
		varying vec4 vDirectionalShadowCoord[ NUM_DIR_LIGHT_SHADOWS ];
		struct DirectionalLightShadow {
			float shadowIntensity;
			float shadowBias;
			float shadowNormalBias;
			float shadowRadius;
			vec2 shadowMapSize;
		};
		uniform DirectionalLightShadow directionalLightShadows[ NUM_DIR_LIGHT_SHADOWS ];
	#endif
	#if NUM_SPOT_LIGHT_SHADOWS > 0
		uniform sampler2D spotShadowMap[ NUM_SPOT_LIGHT_SHADOWS ];
		struct SpotLightShadow {
			float shadowIntensity;
			float shadowBias;
			float shadowNormalBias;
			float shadowRadius;
			vec2 shadowMapSize;
		};
		uniform SpotLightShadow spotLightShadows[ NUM_SPOT_LIGHT_SHADOWS ];
	#endif
	#if NUM_POINT_LIGHT_SHADOWS > 0
		uniform sampler2D pointShadowMap[ NUM_POINT_LIGHT_SHADOWS ];
		varying vec4 vPointShadowCoord[ NUM_POINT_LIGHT_SHADOWS ];
		struct PointLightShadow {
			float shadowIntensity;
			float shadowBias;
			float shadowNormalBias;
			float shadowRadius;
			vec2 shadowMapSize;
			float shadowCameraNear;
			float shadowCameraFar;
		};
		uniform PointLightShadow pointLightShadows[ NUM_POINT_LIGHT_SHADOWS ];
	#endif
	float texture2DCompare( sampler2D depths, vec2 uv, float compare ) {
		return step( compare, unpackRGBAToDepth( texture2D( depths, uv ) ) );
	}
	vec2 texture2DDistribution( sampler2D shadow, vec2 uv ) {
		return unpackRGBATo2Half( texture2D( shadow, uv ) );
	}
	float VSMShadow (sampler2D shadow, vec2 uv, float compare ){
		float occlusion = 1.0;
		vec2 distribution = texture2DDistribution( shadow, uv );
		float hard_shadow = step( compare , distribution.x );
		if (hard_shadow != 1.0 ) {
			float distance = compare - distribution.x ;
			float variance = max( 0.00000, distribution.y * distribution.y );
			float softness_probability = variance / (variance + distance * distance );			softness_probability = clamp( ( softness_probability - 0.3 ) / ( 0.95 - 0.3 ), 0.0, 1.0 );			occlusion = clamp( max( hard_shadow, softness_probability ), 0.0, 1.0 );
		}
		return occlusion;
	}
	float getShadow( sampler2D shadowMap, vec2 shadowMapSize, float shadowIntensity, float shadowBias, float shadowRadius, vec4 shadowCoord ) {
		float shadow = 1.0;
		shadowCoord.xyz /= shadowCoord.w;
		shadowCoord.z += shadowBias;
		bool inFrustum = shadowCoord.x >= 0.0 && shadowCoord.x <= 1.0 && shadowCoord.y >= 0.0 && shadowCoord.y <= 1.0;
		bool frustumTest = inFrustum && shadowCoord.z <= 1.0;
		if ( frustumTest ) {
		#if defined( SHADOWMAP_TYPE_PCF )
			vec2 texelSize = vec2( 1.0 ) / shadowMapSize;
			float dx0 = - texelSize.x * shadowRadius;
			float dy0 = - texelSize.y * shadowRadius;
			float dx1 = + texelSize.x * shadowRadius;
			float dy1 = + texelSize.y * shadowRadius;
			float dx2 = dx0 / 2.0;
			float dy2 = dy0 / 2.0;
			float dx3 = dx1 / 2.0;
			float dy3 = dy1 / 2.0;
			shadow = (
				texture2DCompare( shadowMap, shadowCoord.xy + vec2( dx0, dy0 ), shadowCoord.z ) +
				texture2DCompare( shadowMap, shadowCoord.xy + vec2( 0.0, dy0 ), shadowCoord.z ) +
				texture2DCompare( shadowMap, shadowCoord.xy + vec2( dx1, dy0 ), shadowCoord.z ) +
				texture2DCompare( shadowMap, shadowCoord.xy + vec2( dx2, dy2 ), shadowCoord.z ) +
				texture2DCompare( shadowMap, shadowCoord.xy + vec2( 0.0, dy2 ), shadowCoord.z ) +
				texture2DCompare( shadowMap, shadowCoord.xy + vec2( dx3, dy2 ), shadowCoord.z ) +
				texture2DCompare( shadowMap, shadowCoord.xy + vec2( dx0, 0.0 ), shadowCoord.z ) +
				texture2DCompare( shadowMap, shadowCoord.xy + vec2( dx2, 0.0 ), shadowCoord.z ) +
				texture2DCompare( shadowMap, shadowCoord.xy, shadowCoord.z ) +
				texture2DCompare( shadowMap, shadowCoord.xy + vec2( dx3, 0.0 ), shadowCoord.z ) +
				texture2DCompare( shadowMap, shadowCoord.xy + vec2( dx1, 0.0 ), shadowCoord.z ) +
				texture2DCompare( shadowMap, shadowCoord.xy + vec2( dx2, dy3 ), shadowCoord.z ) +
				texture2DCompare( shadowMap, shadowCoord.xy + vec2( 0.0, dy3 ), shadowCoord.z ) +
				texture2DCompare( shadowMap, shadowCoord.xy + vec2( dx3, dy3 ), shadowCoord.z ) +
				texture2DCompare( shadowMap, shadowCoord.xy + vec2( dx0, dy1 ), shadowCoord.z ) +
				texture2DCompare( shadowMap, shadowCoord.xy + vec2( 0.0, dy1 ), shadowCoord.z ) +
				texture2DCompare( shadowMap, shadowCoord.xy + vec2( dx1, dy1 ), shadowCoord.z )
			) * ( 1.0 / 17.0 );
		#elif defined( SHADOWMAP_TYPE_PCF_SOFT )
			vec2 texelSize = vec2( 1.0 ) / shadowMapSize;
			float dx = texelSize.x;
			float dy = texelSize.y;
			vec2 uv = shadowCoord.xy;
			vec2 f = fract( uv * shadowMapSize + 0.5 );
			uv -= f * texelSize;
			shadow = (
				texture2DCompare( shadowMap, uv, shadowCoord.z ) +
				texture2DCompare( shadowMap, uv + vec2( dx, 0.0 ), shadowCoord.z ) +
				texture2DCompare( shadowMap, uv + vec2( 0.0, dy ), shadowCoord.z ) +
				texture2DCompare( shadowMap, uv + texelSize, shadowCoord.z ) +
				mix( texture2DCompare( shadowMap, uv + vec2( -dx, 0.0 ), shadowCoord.z ),
					 texture2DCompare( shadowMap, uv + vec2( 2.0 * dx, 0.0 ), shadowCoord.z ),
					 f.x ) +
				mix( texture2DCompare( shadowMap, uv + vec2( -dx, dy ), shadowCoord.z ),
					 texture2DCompare( shadowMap, uv + vec2( 2.0 * dx, dy ), shadowCoord.z ),
					 f.x ) +
				mix( texture2DCompare( shadowMap, uv + vec2( 0.0, -dy ), shadowCoord.z ),
					 texture2DCompare( shadowMap, uv + vec2( 0.0, 2.0 * dy ), shadowCoord.z ),
					 f.y ) +
				mix( texture2DCompare( shadowMap, uv + vec2( dx, -dy ), shadowCoord.z ),
					 texture2DCompare( shadowMap, uv + vec2( dx, 2.0 * dy ), shadowCoord.z ),
					 f.y ) +
				mix( mix( texture2DCompare( shadowMap, uv + vec2( -dx, -dy ), shadowCoord.z ),
						  texture2DCompare( shadowMap, uv + vec2( 2.0 * dx, -dy ), shadowCoord.z ),
						  f.x ),
					 mix( texture2DCompare( shadowMap, uv + vec2( -dx, 2.0 * dy ), shadowCoord.z ),
						  texture2DCompare( shadowMap, uv + vec2( 2.0 * dx, 2.0 * dy ), shadowCoord.z ),
						  f.x ),
					 f.y )
			) * ( 1.0 / 9.0 );
		#elif defined( SHADOWMAP_TYPE_VSM )
			shadow = VSMShadow( shadowMap, shadowCoord.xy, shadowCoord.z );
		#else
			shadow = texture2DCompare( shadowMap, shadowCoord.xy, shadowCoord.z );
		#endif
		}
		return mix( 1.0, shadow, shadowIntensity );
	}
	vec2 cubeToUV( vec3 v, float texelSizeY ) {
		vec3 absV = abs( v );
		float scaleToCube = 1.0 / max( absV.x, max( absV.y, absV.z ) );
		absV *= scaleToCube;
		v *= scaleToCube * ( 1.0 - 2.0 * texelSizeY );
		vec2 planar = v.xy;
		float almostATexel = 1.5 * texelSizeY;
		float almostOne = 1.0 - almostATexel;
		if ( absV.z >= almostOne ) {
			if ( v.z > 0.0 )
				planar.x = 4.0 - v.x;
		} else if ( absV.x >= almostOne ) {
			float signX = sign( v.x );
			planar.x = v.z * signX + 2.0 * signX;
		} else if ( absV.y >= almostOne ) {
			float signY = sign( v.y );
			planar.x = v.x + 2.0 * signY + 2.0;
			planar.y = v.z * signY - 2.0;
		}
		return vec2( 0.125, 0.25 ) * planar + vec2( 0.375, 0.75 );
	}
	float getPointShadow( sampler2D shadowMap, vec2 shadowMapSize, float shadowIntensity, float shadowBias, float shadowRadius, vec4 shadowCoord, float shadowCameraNear, float shadowCameraFar ) {
		float shadow = 1.0;
		vec3 lightToPosition = shadowCoord.xyz;
		
		float lightToPositionLength = length( lightToPosition );
		if ( lightToPositionLength - shadowCameraFar <= 0.0 && lightToPositionLength - shadowCameraNear >= 0.0 ) {
			float dp = ( lightToPositionLength - shadowCameraNear ) / ( shadowCameraFar - shadowCameraNear );			dp += shadowBias;
			vec3 bd3D = normalize( lightToPosition );
			vec2 texelSize = vec2( 1.0 ) / ( shadowMapSize * vec2( 4.0, 2.0 ) );
			#if defined( SHADOWMAP_TYPE_PCF ) || defined( SHADOWMAP_TYPE_PCF_SOFT ) || defined( SHADOWMAP_TYPE_VSM )
				vec2 offset = vec2( - 1, 1 ) * shadowRadius * texelSize.y;
				shadow = (
					texture2DCompare( shadowMap, cubeToUV( bd3D + offset.xyy, texelSize.y ), dp ) +
					texture2DCompare( shadowMap, cubeToUV( bd3D + offset.yyy, texelSize.y ), dp ) +
					texture2DCompare( shadowMap, cubeToUV( bd3D + offset.xyx, texelSize.y ), dp ) +
					texture2DCompare( shadowMap, cubeToUV( bd3D + offset.yyx, texelSize.y ), dp ) +
					texture2DCompare( shadowMap, cubeToUV( bd3D, texelSize.y ), dp ) +
					texture2DCompare( shadowMap, cubeToUV( bd3D + offset.xxy, texelSize.y ), dp ) +
					texture2DCompare( shadowMap, cubeToUV( bd3D + offset.yxy, texelSize.y ), dp ) +
					texture2DCompare( shadowMap, cubeToUV( bd3D + offset.xxx, texelSize.y ), dp ) +
					texture2DCompare( shadowMap, cubeToUV( bd3D + offset.yxx, texelSize.y ), dp )
				) * ( 1.0 / 9.0 );
			#else
				shadow = texture2DCompare( shadowMap, cubeToUV( bd3D, texelSize.y ), dp );
			#endif
		}
		return mix( 1.0, shadow, shadowIntensity );
	}
#endif`,w1=`#if NUM_SPOT_LIGHT_COORDS > 0
	uniform mat4 spotLightMatrix[ NUM_SPOT_LIGHT_COORDS ];
	varying vec4 vSpotLightCoord[ NUM_SPOT_LIGHT_COORDS ];
#endif
#ifdef USE_SHADOWMAP
	#if NUM_DIR_LIGHT_SHADOWS > 0
		uniform mat4 directionalShadowMatrix[ NUM_DIR_LIGHT_SHADOWS ];
		varying vec4 vDirectionalShadowCoord[ NUM_DIR_LIGHT_SHADOWS ];
		struct DirectionalLightShadow {
			float shadowIntensity;
			float shadowBias;
			float shadowNormalBias;
			float shadowRadius;
			vec2 shadowMapSize;
		};
		uniform DirectionalLightShadow directionalLightShadows[ NUM_DIR_LIGHT_SHADOWS ];
	#endif
	#if NUM_SPOT_LIGHT_SHADOWS > 0
		struct SpotLightShadow {
			float shadowIntensity;
			float shadowBias;
			float shadowNormalBias;
			float shadowRadius;
			vec2 shadowMapSize;
		};
		uniform SpotLightShadow spotLightShadows[ NUM_SPOT_LIGHT_SHADOWS ];
	#endif
	#if NUM_POINT_LIGHT_SHADOWS > 0
		uniform mat4 pointShadowMatrix[ NUM_POINT_LIGHT_SHADOWS ];
		varying vec4 vPointShadowCoord[ NUM_POINT_LIGHT_SHADOWS ];
		struct PointLightShadow {
			float shadowIntensity;
			float shadowBias;
			float shadowNormalBias;
			float shadowRadius;
			vec2 shadowMapSize;
			float shadowCameraNear;
			float shadowCameraFar;
		};
		uniform PointLightShadow pointLightShadows[ NUM_POINT_LIGHT_SHADOWS ];
	#endif
#endif`,E1=`#if ( defined( USE_SHADOWMAP ) && ( NUM_DIR_LIGHT_SHADOWS > 0 || NUM_POINT_LIGHT_SHADOWS > 0 ) ) || ( NUM_SPOT_LIGHT_COORDS > 0 )
	vec3 shadowWorldNormal = inverseTransformDirection( transformedNormal, viewMatrix );
	vec4 shadowWorldPosition;
#endif
#if defined( USE_SHADOWMAP )
	#if NUM_DIR_LIGHT_SHADOWS > 0
		#pragma unroll_loop_start
		for ( int i = 0; i < NUM_DIR_LIGHT_SHADOWS; i ++ ) {
			shadowWorldPosition = worldPosition + vec4( shadowWorldNormal * directionalLightShadows[ i ].shadowNormalBias, 0 );
			vDirectionalShadowCoord[ i ] = directionalShadowMatrix[ i ] * shadowWorldPosition;
		}
		#pragma unroll_loop_end
	#endif
	#if NUM_POINT_LIGHT_SHADOWS > 0
		#pragma unroll_loop_start
		for ( int i = 0; i < NUM_POINT_LIGHT_SHADOWS; i ++ ) {
			shadowWorldPosition = worldPosition + vec4( shadowWorldNormal * pointLightShadows[ i ].shadowNormalBias, 0 );
			vPointShadowCoord[ i ] = pointShadowMatrix[ i ] * shadowWorldPosition;
		}
		#pragma unroll_loop_end
	#endif
#endif
#if NUM_SPOT_LIGHT_COORDS > 0
	#pragma unroll_loop_start
	for ( int i = 0; i < NUM_SPOT_LIGHT_COORDS; i ++ ) {
		shadowWorldPosition = worldPosition;
		#if ( defined( USE_SHADOWMAP ) && UNROLLED_LOOP_INDEX < NUM_SPOT_LIGHT_SHADOWS )
			shadowWorldPosition.xyz += shadowWorldNormal * spotLightShadows[ i ].shadowNormalBias;
		#endif
		vSpotLightCoord[ i ] = spotLightMatrix[ i ] * shadowWorldPosition;
	}
	#pragma unroll_loop_end
#endif`,T1=`float getShadowMask() {
	float shadow = 1.0;
	#ifdef USE_SHADOWMAP
	#if NUM_DIR_LIGHT_SHADOWS > 0
	DirectionalLightShadow directionalLight;
	#pragma unroll_loop_start
	for ( int i = 0; i < NUM_DIR_LIGHT_SHADOWS; i ++ ) {
		directionalLight = directionalLightShadows[ i ];
		shadow *= receiveShadow ? getShadow( directionalShadowMap[ i ], directionalLight.shadowMapSize, directionalLight.shadowIntensity, directionalLight.shadowBias, directionalLight.shadowRadius, vDirectionalShadowCoord[ i ] ) : 1.0;
	}
	#pragma unroll_loop_end
	#endif
	#if NUM_SPOT_LIGHT_SHADOWS > 0
	SpotLightShadow spotLight;
	#pragma unroll_loop_start
	for ( int i = 0; i < NUM_SPOT_LIGHT_SHADOWS; i ++ ) {
		spotLight = spotLightShadows[ i ];
		shadow *= receiveShadow ? getShadow( spotShadowMap[ i ], spotLight.shadowMapSize, spotLight.shadowIntensity, spotLight.shadowBias, spotLight.shadowRadius, vSpotLightCoord[ i ] ) : 1.0;
	}
	#pragma unroll_loop_end
	#endif
	#if NUM_POINT_LIGHT_SHADOWS > 0
	PointLightShadow pointLight;
	#pragma unroll_loop_start
	for ( int i = 0; i < NUM_POINT_LIGHT_SHADOWS; i ++ ) {
		pointLight = pointLightShadows[ i ];
		shadow *= receiveShadow ? getPointShadow( pointShadowMap[ i ], pointLight.shadowMapSize, pointLight.shadowIntensity, pointLight.shadowBias, pointLight.shadowRadius, vPointShadowCoord[ i ], pointLight.shadowCameraNear, pointLight.shadowCameraFar ) : 1.0;
	}
	#pragma unroll_loop_end
	#endif
	#endif
	return shadow;
}`,A1=`#ifdef USE_SKINNING
	mat4 boneMatX = getBoneMatrix( skinIndex.x );
	mat4 boneMatY = getBoneMatrix( skinIndex.y );
	mat4 boneMatZ = getBoneMatrix( skinIndex.z );
	mat4 boneMatW = getBoneMatrix( skinIndex.w );
#endif`,C1=`#ifdef USE_SKINNING
	uniform mat4 bindMatrix;
	uniform mat4 bindMatrixInverse;
	uniform highp sampler2D boneTexture;
	mat4 getBoneMatrix( const in float i ) {
		int size = textureSize( boneTexture, 0 ).x;
		int j = int( i ) * 4;
		int x = j % size;
		int y = j / size;
		vec4 v1 = texelFetch( boneTexture, ivec2( x, y ), 0 );
		vec4 v2 = texelFetch( boneTexture, ivec2( x + 1, y ), 0 );
		vec4 v3 = texelFetch( boneTexture, ivec2( x + 2, y ), 0 );
		vec4 v4 = texelFetch( boneTexture, ivec2( x + 3, y ), 0 );
		return mat4( v1, v2, v3, v4 );
	}
#endif`,b1=`#ifdef USE_SKINNING
	vec4 skinVertex = bindMatrix * vec4( transformed, 1.0 );
	vec4 skinned = vec4( 0.0 );
	skinned += boneMatX * skinVertex * skinWeight.x;
	skinned += boneMatY * skinVertex * skinWeight.y;
	skinned += boneMatZ * skinVertex * skinWeight.z;
	skinned += boneMatW * skinVertex * skinWeight.w;
	transformed = ( bindMatrixInverse * skinned ).xyz;
#endif`,R1=`#ifdef USE_SKINNING
	mat4 skinMatrix = mat4( 0.0 );
	skinMatrix += skinWeight.x * boneMatX;
	skinMatrix += skinWeight.y * boneMatY;
	skinMatrix += skinWeight.z * boneMatZ;
	skinMatrix += skinWeight.w * boneMatW;
	skinMatrix = bindMatrixInverse * skinMatrix * bindMatrix;
	objectNormal = vec4( skinMatrix * vec4( objectNormal, 0.0 ) ).xyz;
	#ifdef USE_TANGENT
		objectTangent = vec4( skinMatrix * vec4( objectTangent, 0.0 ) ).xyz;
	#endif
#endif`,P1=`float specularStrength;
#ifdef USE_SPECULARMAP
	vec4 texelSpecular = texture2D( specularMap, vSpecularMapUv );
	specularStrength = texelSpecular.r;
#else
	specularStrength = 1.0;
#endif`,D1=`#ifdef USE_SPECULARMAP
	uniform sampler2D specularMap;
#endif`,L1=`#if defined( TONE_MAPPING )
	gl_FragColor.rgb = toneMapping( gl_FragColor.rgb );
#endif`,N1=`#ifndef saturate
#define saturate( a ) clamp( a, 0.0, 1.0 )
#endif
uniform float toneMappingExposure;
vec3 LinearToneMapping( vec3 color ) {
	return saturate( toneMappingExposure * color );
}
vec3 ReinhardToneMapping( vec3 color ) {
	color *= toneMappingExposure;
	return saturate( color / ( vec3( 1.0 ) + color ) );
}
vec3 CineonToneMapping( vec3 color ) {
	color *= toneMappingExposure;
	color = max( vec3( 0.0 ), color - 0.004 );
	return pow( ( color * ( 6.2 * color + 0.5 ) ) / ( color * ( 6.2 * color + 1.7 ) + 0.06 ), vec3( 2.2 ) );
}
vec3 RRTAndODTFit( vec3 v ) {
	vec3 a = v * ( v + 0.0245786 ) - 0.000090537;
	vec3 b = v * ( 0.983729 * v + 0.4329510 ) + 0.238081;
	return a / b;
}
vec3 ACESFilmicToneMapping( vec3 color ) {
	const mat3 ACESInputMat = mat3(
		vec3( 0.59719, 0.07600, 0.02840 ),		vec3( 0.35458, 0.90834, 0.13383 ),
		vec3( 0.04823, 0.01566, 0.83777 )
	);
	const mat3 ACESOutputMat = mat3(
		vec3(  1.60475, -0.10208, -0.00327 ),		vec3( -0.53108,  1.10813, -0.07276 ),
		vec3( -0.07367, -0.00605,  1.07602 )
	);
	color *= toneMappingExposure / 0.6;
	color = ACESInputMat * color;
	color = RRTAndODTFit( color );
	color = ACESOutputMat * color;
	return saturate( color );
}
const mat3 LINEAR_REC2020_TO_LINEAR_SRGB = mat3(
	vec3( 1.6605, - 0.1246, - 0.0182 ),
	vec3( - 0.5876, 1.1329, - 0.1006 ),
	vec3( - 0.0728, - 0.0083, 1.1187 )
);
const mat3 LINEAR_SRGB_TO_LINEAR_REC2020 = mat3(
	vec3( 0.6274, 0.0691, 0.0164 ),
	vec3( 0.3293, 0.9195, 0.0880 ),
	vec3( 0.0433, 0.0113, 0.8956 )
);
vec3 agxDefaultContrastApprox( vec3 x ) {
	vec3 x2 = x * x;
	vec3 x4 = x2 * x2;
	return + 15.5 * x4 * x2
		- 40.14 * x4 * x
		+ 31.96 * x4
		- 6.868 * x2 * x
		+ 0.4298 * x2
		+ 0.1191 * x
		- 0.00232;
}
vec3 AgXToneMapping( vec3 color ) {
	const mat3 AgXInsetMatrix = mat3(
		vec3( 0.856627153315983, 0.137318972929847, 0.11189821299995 ),
		vec3( 0.0951212405381588, 0.761241990602591, 0.0767994186031903 ),
		vec3( 0.0482516061458583, 0.101439036467562, 0.811302368396859 )
	);
	const mat3 AgXOutsetMatrix = mat3(
		vec3( 1.1271005818144368, - 0.1413297634984383, - 0.14132976349843826 ),
		vec3( - 0.11060664309660323, 1.157823702216272, - 0.11060664309660294 ),
		vec3( - 0.016493938717834573, - 0.016493938717834257, 1.2519364065950405 )
	);
	const float AgxMinEv = - 12.47393;	const float AgxMaxEv = 4.026069;
	color *= toneMappingExposure;
	color = LINEAR_SRGB_TO_LINEAR_REC2020 * color;
	color = AgXInsetMatrix * color;
	color = max( color, 1e-10 );	color = log2( color );
	color = ( color - AgxMinEv ) / ( AgxMaxEv - AgxMinEv );
	color = clamp( color, 0.0, 1.0 );
	color = agxDefaultContrastApprox( color );
	color = AgXOutsetMatrix * color;
	color = pow( max( vec3( 0.0 ), color ), vec3( 2.2 ) );
	color = LINEAR_REC2020_TO_LINEAR_SRGB * color;
	color = clamp( color, 0.0, 1.0 );
	return color;
}
vec3 NeutralToneMapping( vec3 color ) {
	const float StartCompression = 0.8 - 0.04;
	const float Desaturation = 0.15;
	color *= toneMappingExposure;
	float x = min( color.r, min( color.g, color.b ) );
	float offset = x < 0.08 ? x - 6.25 * x * x : 0.04;
	color -= offset;
	float peak = max( color.r, max( color.g, color.b ) );
	if ( peak < StartCompression ) return color;
	float d = 1. - StartCompression;
	float newPeak = 1. - d * d / ( peak + d - StartCompression );
	color *= newPeak / peak;
	float g = 1. - 1. / ( Desaturation * ( peak - newPeak ) + 1. );
	return mix( color, vec3( newPeak ), g );
}
vec3 CustomToneMapping( vec3 color ) { return color; }`,I1=`#ifdef USE_TRANSMISSION
	material.transmission = transmission;
	material.transmissionAlpha = 1.0;
	material.thickness = thickness;
	material.attenuationDistance = attenuationDistance;
	material.attenuationColor = attenuationColor;
	#ifdef USE_TRANSMISSIONMAP
		material.transmission *= texture2D( transmissionMap, vTransmissionMapUv ).r;
	#endif
	#ifdef USE_THICKNESSMAP
		material.thickness *= texture2D( thicknessMap, vThicknessMapUv ).g;
	#endif
	vec3 pos = vWorldPosition;
	vec3 v = normalize( cameraPosition - pos );
	vec3 n = inverseTransformDirection( normal, viewMatrix );
	vec4 transmitted = getIBLVolumeRefraction(
		n, v, material.roughness, material.diffuseColor, material.specularColor, material.specularF90,
		pos, modelMatrix, viewMatrix, projectionMatrix, material.dispersion, material.ior, material.thickness,
		material.attenuationColor, material.attenuationDistance );
	material.transmissionAlpha = mix( material.transmissionAlpha, transmitted.a, material.transmission );
	totalDiffuse = mix( totalDiffuse, transmitted.rgb, material.transmission );
#endif`,k1=`#ifdef USE_TRANSMISSION
	uniform float transmission;
	uniform float thickness;
	uniform float attenuationDistance;
	uniform vec3 attenuationColor;
	#ifdef USE_TRANSMISSIONMAP
		uniform sampler2D transmissionMap;
	#endif
	#ifdef USE_THICKNESSMAP
		uniform sampler2D thicknessMap;
	#endif
	uniform vec2 transmissionSamplerSize;
	uniform sampler2D transmissionSamplerMap;
	uniform mat4 modelMatrix;
	uniform mat4 projectionMatrix;
	varying vec3 vWorldPosition;
	float w0( float a ) {
		return ( 1.0 / 6.0 ) * ( a * ( a * ( - a + 3.0 ) - 3.0 ) + 1.0 );
	}
	float w1( float a ) {
		return ( 1.0 / 6.0 ) * ( a *  a * ( 3.0 * a - 6.0 ) + 4.0 );
	}
	float w2( float a ){
		return ( 1.0 / 6.0 ) * ( a * ( a * ( - 3.0 * a + 3.0 ) + 3.0 ) + 1.0 );
	}
	float w3( float a ) {
		return ( 1.0 / 6.0 ) * ( a * a * a );
	}
	float g0( float a ) {
		return w0( a ) + w1( a );
	}
	float g1( float a ) {
		return w2( a ) + w3( a );
	}
	float h0( float a ) {
		return - 1.0 + w1( a ) / ( w0( a ) + w1( a ) );
	}
	float h1( float a ) {
		return 1.0 + w3( a ) / ( w2( a ) + w3( a ) );
	}
	vec4 bicubic( sampler2D tex, vec2 uv, vec4 texelSize, float lod ) {
		uv = uv * texelSize.zw + 0.5;
		vec2 iuv = floor( uv );
		vec2 fuv = fract( uv );
		float g0x = g0( fuv.x );
		float g1x = g1( fuv.x );
		float h0x = h0( fuv.x );
		float h1x = h1( fuv.x );
		float h0y = h0( fuv.y );
		float h1y = h1( fuv.y );
		vec2 p0 = ( vec2( iuv.x + h0x, iuv.y + h0y ) - 0.5 ) * texelSize.xy;
		vec2 p1 = ( vec2( iuv.x + h1x, iuv.y + h0y ) - 0.5 ) * texelSize.xy;
		vec2 p2 = ( vec2( iuv.x + h0x, iuv.y + h1y ) - 0.5 ) * texelSize.xy;
		vec2 p3 = ( vec2( iuv.x + h1x, iuv.y + h1y ) - 0.5 ) * texelSize.xy;
		return g0( fuv.y ) * ( g0x * textureLod( tex, p0, lod ) + g1x * textureLod( tex, p1, lod ) ) +
			g1( fuv.y ) * ( g0x * textureLod( tex, p2, lod ) + g1x * textureLod( tex, p3, lod ) );
	}
	vec4 textureBicubic( sampler2D sampler, vec2 uv, float lod ) {
		vec2 fLodSize = vec2( textureSize( sampler, int( lod ) ) );
		vec2 cLodSize = vec2( textureSize( sampler, int( lod + 1.0 ) ) );
		vec2 fLodSizeInv = 1.0 / fLodSize;
		vec2 cLodSizeInv = 1.0 / cLodSize;
		vec4 fSample = bicubic( sampler, uv, vec4( fLodSizeInv, fLodSize ), floor( lod ) );
		vec4 cSample = bicubic( sampler, uv, vec4( cLodSizeInv, cLodSize ), ceil( lod ) );
		return mix( fSample, cSample, fract( lod ) );
	}
	vec3 getVolumeTransmissionRay( const in vec3 n, const in vec3 v, const in float thickness, const in float ior, const in mat4 modelMatrix ) {
		vec3 refractionVector = refract( - v, normalize( n ), 1.0 / ior );
		vec3 modelScale;
		modelScale.x = length( vec3( modelMatrix[ 0 ].xyz ) );
		modelScale.y = length( vec3( modelMatrix[ 1 ].xyz ) );
		modelScale.z = length( vec3( modelMatrix[ 2 ].xyz ) );
		return normalize( refractionVector ) * thickness * modelScale;
	}
	float applyIorToRoughness( const in float roughness, const in float ior ) {
		return roughness * clamp( ior * 2.0 - 2.0, 0.0, 1.0 );
	}
	vec4 getTransmissionSample( const in vec2 fragCoord, const in float roughness, const in float ior ) {
		float lod = log2( transmissionSamplerSize.x ) * applyIorToRoughness( roughness, ior );
		return textureBicubic( transmissionSamplerMap, fragCoord.xy, lod );
	}
	vec3 volumeAttenuation( const in float transmissionDistance, const in vec3 attenuationColor, const in float attenuationDistance ) {
		if ( isinf( attenuationDistance ) ) {
			return vec3( 1.0 );
		} else {
			vec3 attenuationCoefficient = -log( attenuationColor ) / attenuationDistance;
			vec3 transmittance = exp( - attenuationCoefficient * transmissionDistance );			return transmittance;
		}
	}
	vec4 getIBLVolumeRefraction( const in vec3 n, const in vec3 v, const in float roughness, const in vec3 diffuseColor,
		const in vec3 specularColor, const in float specularF90, const in vec3 position, const in mat4 modelMatrix,
		const in mat4 viewMatrix, const in mat4 projMatrix, const in float dispersion, const in float ior, const in float thickness,
		const in vec3 attenuationColor, const in float attenuationDistance ) {
		vec4 transmittedLight;
		vec3 transmittance;
		#ifdef USE_DISPERSION
			float halfSpread = ( ior - 1.0 ) * 0.025 * dispersion;
			vec3 iors = vec3( ior - halfSpread, ior, ior + halfSpread );
			for ( int i = 0; i < 3; i ++ ) {
				vec3 transmissionRay = getVolumeTransmissionRay( n, v, thickness, iors[ i ], modelMatrix );
				vec3 refractedRayExit = position + transmissionRay;
		
				vec4 ndcPos = projMatrix * viewMatrix * vec4( refractedRayExit, 1.0 );
				vec2 refractionCoords = ndcPos.xy / ndcPos.w;
				refractionCoords += 1.0;
				refractionCoords /= 2.0;
		
				vec4 transmissionSample = getTransmissionSample( refractionCoords, roughness, iors[ i ] );
				transmittedLight[ i ] = transmissionSample[ i ];
				transmittedLight.a += transmissionSample.a;
				transmittance[ i ] = diffuseColor[ i ] * volumeAttenuation( length( transmissionRay ), attenuationColor, attenuationDistance )[ i ];
			}
			transmittedLight.a /= 3.0;
		
		#else
		
			vec3 transmissionRay = getVolumeTransmissionRay( n, v, thickness, ior, modelMatrix );
			vec3 refractedRayExit = position + transmissionRay;
			vec4 ndcPos = projMatrix * viewMatrix * vec4( refractedRayExit, 1.0 );
			vec2 refractionCoords = ndcPos.xy / ndcPos.w;
			refractionCoords += 1.0;
			refractionCoords /= 2.0;
			transmittedLight = getTransmissionSample( refractionCoords, roughness, ior );
			transmittance = diffuseColor * volumeAttenuation( length( transmissionRay ), attenuationColor, attenuationDistance );
		
		#endif
		vec3 attenuatedColor = transmittance * transmittedLight.rgb;
		vec3 F = EnvironmentBRDF( n, v, specularColor, specularF90, roughness );
		float transmittanceFactor = ( transmittance.r + transmittance.g + transmittance.b ) / 3.0;
		return vec4( ( 1.0 - F ) * attenuatedColor, 1.0 - ( 1.0 - transmittedLight.a ) * transmittanceFactor );
	}
#endif`,U1=`#if defined( USE_UV ) || defined( USE_ANISOTROPY )
	varying vec2 vUv;
#endif
#ifdef USE_MAP
	varying vec2 vMapUv;
#endif
#ifdef USE_ALPHAMAP
	varying vec2 vAlphaMapUv;
#endif
#ifdef USE_LIGHTMAP
	varying vec2 vLightMapUv;
#endif
#ifdef USE_AOMAP
	varying vec2 vAoMapUv;
#endif
#ifdef USE_BUMPMAP
	varying vec2 vBumpMapUv;
#endif
#ifdef USE_NORMALMAP
	varying vec2 vNormalMapUv;
#endif
#ifdef USE_EMISSIVEMAP
	varying vec2 vEmissiveMapUv;
#endif
#ifdef USE_METALNESSMAP
	varying vec2 vMetalnessMapUv;
#endif
#ifdef USE_ROUGHNESSMAP
	varying vec2 vRoughnessMapUv;
#endif
#ifdef USE_ANISOTROPYMAP
	varying vec2 vAnisotropyMapUv;
#endif
#ifdef USE_CLEARCOATMAP
	varying vec2 vClearcoatMapUv;
#endif
#ifdef USE_CLEARCOAT_NORMALMAP
	varying vec2 vClearcoatNormalMapUv;
#endif
#ifdef USE_CLEARCOAT_ROUGHNESSMAP
	varying vec2 vClearcoatRoughnessMapUv;
#endif
#ifdef USE_IRIDESCENCEMAP
	varying vec2 vIridescenceMapUv;
#endif
#ifdef USE_IRIDESCENCE_THICKNESSMAP
	varying vec2 vIridescenceThicknessMapUv;
#endif
#ifdef USE_SHEEN_COLORMAP
	varying vec2 vSheenColorMapUv;
#endif
#ifdef USE_SHEEN_ROUGHNESSMAP
	varying vec2 vSheenRoughnessMapUv;
#endif
#ifdef USE_SPECULARMAP
	varying vec2 vSpecularMapUv;
#endif
#ifdef USE_SPECULAR_COLORMAP
	varying vec2 vSpecularColorMapUv;
#endif
#ifdef USE_SPECULAR_INTENSITYMAP
	varying vec2 vSpecularIntensityMapUv;
#endif
#ifdef USE_TRANSMISSIONMAP
	uniform mat3 transmissionMapTransform;
	varying vec2 vTransmissionMapUv;
#endif
#ifdef USE_THICKNESSMAP
	uniform mat3 thicknessMapTransform;
	varying vec2 vThicknessMapUv;
#endif`,F1=`#if defined( USE_UV ) || defined( USE_ANISOTROPY )
	varying vec2 vUv;
#endif
#ifdef USE_MAP
	uniform mat3 mapTransform;
	varying vec2 vMapUv;
#endif
#ifdef USE_ALPHAMAP
	uniform mat3 alphaMapTransform;
	varying vec2 vAlphaMapUv;
#endif
#ifdef USE_LIGHTMAP
	uniform mat3 lightMapTransform;
	varying vec2 vLightMapUv;
#endif
#ifdef USE_AOMAP
	uniform mat3 aoMapTransform;
	varying vec2 vAoMapUv;
#endif
#ifdef USE_BUMPMAP
	uniform mat3 bumpMapTransform;
	varying vec2 vBumpMapUv;
#endif
#ifdef USE_NORMALMAP
	uniform mat3 normalMapTransform;
	varying vec2 vNormalMapUv;
#endif
#ifdef USE_DISPLACEMENTMAP
	uniform mat3 displacementMapTransform;
	varying vec2 vDisplacementMapUv;
#endif
#ifdef USE_EMISSIVEMAP
	uniform mat3 emissiveMapTransform;
	varying vec2 vEmissiveMapUv;
#endif
#ifdef USE_METALNESSMAP
	uniform mat3 metalnessMapTransform;
	varying vec2 vMetalnessMapUv;
#endif
#ifdef USE_ROUGHNESSMAP
	uniform mat3 roughnessMapTransform;
	varying vec2 vRoughnessMapUv;
#endif
#ifdef USE_ANISOTROPYMAP
	uniform mat3 anisotropyMapTransform;
	varying vec2 vAnisotropyMapUv;
#endif
#ifdef USE_CLEARCOATMAP
	uniform mat3 clearcoatMapTransform;
	varying vec2 vClearcoatMapUv;
#endif
#ifdef USE_CLEARCOAT_NORMALMAP
	uniform mat3 clearcoatNormalMapTransform;
	varying vec2 vClearcoatNormalMapUv;
#endif
#ifdef USE_CLEARCOAT_ROUGHNESSMAP
	uniform mat3 clearcoatRoughnessMapTransform;
	varying vec2 vClearcoatRoughnessMapUv;
#endif
#ifdef USE_SHEEN_COLORMAP
	uniform mat3 sheenColorMapTransform;
	varying vec2 vSheenColorMapUv;
#endif
#ifdef USE_SHEEN_ROUGHNESSMAP
	uniform mat3 sheenRoughnessMapTransform;
	varying vec2 vSheenRoughnessMapUv;
#endif
#ifdef USE_IRIDESCENCEMAP
	uniform mat3 iridescenceMapTransform;
	varying vec2 vIridescenceMapUv;
#endif
#ifdef USE_IRIDESCENCE_THICKNESSMAP
	uniform mat3 iridescenceThicknessMapTransform;
	varying vec2 vIridescenceThicknessMapUv;
#endif
#ifdef USE_SPECULARMAP
	uniform mat3 specularMapTransform;
	varying vec2 vSpecularMapUv;
#endif
#ifdef USE_SPECULAR_COLORMAP
	uniform mat3 specularColorMapTransform;
	varying vec2 vSpecularColorMapUv;
#endif
#ifdef USE_SPECULAR_INTENSITYMAP
	uniform mat3 specularIntensityMapTransform;
	varying vec2 vSpecularIntensityMapUv;
#endif
#ifdef USE_TRANSMISSIONMAP
	uniform mat3 transmissionMapTransform;
	varying vec2 vTransmissionMapUv;
#endif
#ifdef USE_THICKNESSMAP
	uniform mat3 thicknessMapTransform;
	varying vec2 vThicknessMapUv;
#endif`,O1=`#if defined( USE_UV ) || defined( USE_ANISOTROPY )
	vUv = vec3( uv, 1 ).xy;
#endif
#ifdef USE_MAP
	vMapUv = ( mapTransform * vec3( MAP_UV, 1 ) ).xy;
#endif
#ifdef USE_ALPHAMAP
	vAlphaMapUv = ( alphaMapTransform * vec3( ALPHAMAP_UV, 1 ) ).xy;
#endif
#ifdef USE_LIGHTMAP
	vLightMapUv = ( lightMapTransform * vec3( LIGHTMAP_UV, 1 ) ).xy;
#endif
#ifdef USE_AOMAP
	vAoMapUv = ( aoMapTransform * vec3( AOMAP_UV, 1 ) ).xy;
#endif
#ifdef USE_BUMPMAP
	vBumpMapUv = ( bumpMapTransform * vec3( BUMPMAP_UV, 1 ) ).xy;
#endif
#ifdef USE_NORMALMAP
	vNormalMapUv = ( normalMapTransform * vec3( NORMALMAP_UV, 1 ) ).xy;
#endif
#ifdef USE_DISPLACEMENTMAP
	vDisplacementMapUv = ( displacementMapTransform * vec3( DISPLACEMENTMAP_UV, 1 ) ).xy;
#endif
#ifdef USE_EMISSIVEMAP
	vEmissiveMapUv = ( emissiveMapTransform * vec3( EMISSIVEMAP_UV, 1 ) ).xy;
#endif
#ifdef USE_METALNESSMAP
	vMetalnessMapUv = ( metalnessMapTransform * vec3( METALNESSMAP_UV, 1 ) ).xy;
#endif
#ifdef USE_ROUGHNESSMAP
	vRoughnessMapUv = ( roughnessMapTransform * vec3( ROUGHNESSMAP_UV, 1 ) ).xy;
#endif
#ifdef USE_ANISOTROPYMAP
	vAnisotropyMapUv = ( anisotropyMapTransform * vec3( ANISOTROPYMAP_UV, 1 ) ).xy;
#endif
#ifdef USE_CLEARCOATMAP
	vClearcoatMapUv = ( clearcoatMapTransform * vec3( CLEARCOATMAP_UV, 1 ) ).xy;
#endif
#ifdef USE_CLEARCOAT_NORMALMAP
	vClearcoatNormalMapUv = ( clearcoatNormalMapTransform * vec3( CLEARCOAT_NORMALMAP_UV, 1 ) ).xy;
#endif
#ifdef USE_CLEARCOAT_ROUGHNESSMAP
	vClearcoatRoughnessMapUv = ( clearcoatRoughnessMapTransform * vec3( CLEARCOAT_ROUGHNESSMAP_UV, 1 ) ).xy;
#endif
#ifdef USE_IRIDESCENCEMAP
	vIridescenceMapUv = ( iridescenceMapTransform * vec3( IRIDESCENCEMAP_UV, 1 ) ).xy;
#endif
#ifdef USE_IRIDESCENCE_THICKNESSMAP
	vIridescenceThicknessMapUv = ( iridescenceThicknessMapTransform * vec3( IRIDESCENCE_THICKNESSMAP_UV, 1 ) ).xy;
#endif
#ifdef USE_SHEEN_COLORMAP
	vSheenColorMapUv = ( sheenColorMapTransform * vec3( SHEEN_COLORMAP_UV, 1 ) ).xy;
#endif
#ifdef USE_SHEEN_ROUGHNESSMAP
	vSheenRoughnessMapUv = ( sheenRoughnessMapTransform * vec3( SHEEN_ROUGHNESSMAP_UV, 1 ) ).xy;
#endif
#ifdef USE_SPECULARMAP
	vSpecularMapUv = ( specularMapTransform * vec3( SPECULARMAP_UV, 1 ) ).xy;
#endif
#ifdef USE_SPECULAR_COLORMAP
	vSpecularColorMapUv = ( specularColorMapTransform * vec3( SPECULAR_COLORMAP_UV, 1 ) ).xy;
#endif
#ifdef USE_SPECULAR_INTENSITYMAP
	vSpecularIntensityMapUv = ( specularIntensityMapTransform * vec3( SPECULAR_INTENSITYMAP_UV, 1 ) ).xy;
#endif
#ifdef USE_TRANSMISSIONMAP
	vTransmissionMapUv = ( transmissionMapTransform * vec3( TRANSMISSIONMAP_UV, 1 ) ).xy;
#endif
#ifdef USE_THICKNESSMAP
	vThicknessMapUv = ( thicknessMapTransform * vec3( THICKNESSMAP_UV, 1 ) ).xy;
#endif`,z1=`#if defined( USE_ENVMAP ) || defined( DISTANCE ) || defined ( USE_SHADOWMAP ) || defined ( USE_TRANSMISSION ) || NUM_SPOT_LIGHT_COORDS > 0
	vec4 worldPosition = vec4( transformed, 1.0 );
	#ifdef USE_BATCHING
		worldPosition = batchingMatrix * worldPosition;
	#endif
	#ifdef USE_INSTANCING
		worldPosition = instanceMatrix * worldPosition;
	#endif
	worldPosition = modelMatrix * worldPosition;
#endif`;const B1=`varying vec2 vUv;
uniform mat3 uvTransform;
void main() {
	vUv = ( uvTransform * vec3( uv, 1 ) ).xy;
	gl_Position = vec4( position.xy, 1.0, 1.0 );
}`,H1=`uniform sampler2D t2D;
uniform float backgroundIntensity;
varying vec2 vUv;
void main() {
	vec4 texColor = texture2D( t2D, vUv );
	#ifdef DECODE_VIDEO_TEXTURE
		texColor = vec4( mix( pow( texColor.rgb * 0.9478672986 + vec3( 0.0521327014 ), vec3( 2.4 ) ), texColor.rgb * 0.0773993808, vec3( lessThanEqual( texColor.rgb, vec3( 0.04045 ) ) ) ), texColor.w );
	#endif
	texColor.rgb *= backgroundIntensity;
	gl_FragColor = texColor;
	#include <tonemapping_fragment>
	#include <colorspace_fragment>
}`,V1=`varying vec3 vWorldDirection;
#include <common>
void main() {
	vWorldDirection = transformDirection( position, modelMatrix );
	#include <begin_vertex>
	#include <project_vertex>
	gl_Position.z = gl_Position.w;
}`,G1=`#ifdef ENVMAP_TYPE_CUBE
	uniform samplerCube envMap;
#elif defined( ENVMAP_TYPE_CUBE_UV )
	uniform sampler2D envMap;
#endif
uniform float flipEnvMap;
uniform float backgroundBlurriness;
uniform float backgroundIntensity;
uniform mat3 backgroundRotation;
varying vec3 vWorldDirection;
#include <cube_uv_reflection_fragment>
void main() {
	#ifdef ENVMAP_TYPE_CUBE
		vec4 texColor = textureCube( envMap, backgroundRotation * vec3( flipEnvMap * vWorldDirection.x, vWorldDirection.yz ) );
	#elif defined( ENVMAP_TYPE_CUBE_UV )
		vec4 texColor = textureCubeUV( envMap, backgroundRotation * vWorldDirection, backgroundBlurriness );
	#else
		vec4 texColor = vec4( 0.0, 0.0, 0.0, 1.0 );
	#endif
	texColor.rgb *= backgroundIntensity;
	gl_FragColor = texColor;
	#include <tonemapping_fragment>
	#include <colorspace_fragment>
}`,W1=`varying vec3 vWorldDirection;
#include <common>
void main() {
	vWorldDirection = transformDirection( position, modelMatrix );
	#include <begin_vertex>
	#include <project_vertex>
	gl_Position.z = gl_Position.w;
}`,X1=`uniform samplerCube tCube;
uniform float tFlip;
uniform float opacity;
varying vec3 vWorldDirection;
void main() {
	vec4 texColor = textureCube( tCube, vec3( tFlip * vWorldDirection.x, vWorldDirection.yz ) );
	gl_FragColor = texColor;
	gl_FragColor.a *= opacity;
	#include <tonemapping_fragment>
	#include <colorspace_fragment>
}`,j1=`#include <common>
#include <batching_pars_vertex>
#include <uv_pars_vertex>
#include <displacementmap_pars_vertex>
#include <morphtarget_pars_vertex>
#include <skinning_pars_vertex>
#include <logdepthbuf_pars_vertex>
#include <clipping_planes_pars_vertex>
varying vec2 vHighPrecisionZW;
void main() {
	#include <uv_vertex>
	#include <batching_vertex>
	#include <skinbase_vertex>
	#include <morphinstance_vertex>
	#ifdef USE_DISPLACEMENTMAP
		#include <beginnormal_vertex>
		#include <morphnormal_vertex>
		#include <skinnormal_vertex>
	#endif
	#include <begin_vertex>
	#include <morphtarget_vertex>
	#include <skinning_vertex>
	#include <displacementmap_vertex>
	#include <project_vertex>
	#include <logdepthbuf_vertex>
	#include <clipping_planes_vertex>
	vHighPrecisionZW = gl_Position.zw;
}`,Y1=`#if DEPTH_PACKING == 3200
	uniform float opacity;
#endif
#include <common>
#include <packing>
#include <uv_pars_fragment>
#include <map_pars_fragment>
#include <alphamap_pars_fragment>
#include <alphatest_pars_fragment>
#include <alphahash_pars_fragment>
#include <logdepthbuf_pars_fragment>
#include <clipping_planes_pars_fragment>
varying vec2 vHighPrecisionZW;
void main() {
	vec4 diffuseColor = vec4( 1.0 );
	#include <clipping_planes_fragment>
	#if DEPTH_PACKING == 3200
		diffuseColor.a = opacity;
	#endif
	#include <map_fragment>
	#include <alphamap_fragment>
	#include <alphatest_fragment>
	#include <alphahash_fragment>
	#include <logdepthbuf_fragment>
	float fragCoordZ = 0.5 * vHighPrecisionZW[0] / vHighPrecisionZW[1] + 0.5;
	#if DEPTH_PACKING == 3200
		gl_FragColor = vec4( vec3( 1.0 - fragCoordZ ), opacity );
	#elif DEPTH_PACKING == 3201
		gl_FragColor = packDepthToRGBA( fragCoordZ );
	#elif DEPTH_PACKING == 3202
		gl_FragColor = vec4( packDepthToRGB( fragCoordZ ), 1.0 );
	#elif DEPTH_PACKING == 3203
		gl_FragColor = vec4( packDepthToRG( fragCoordZ ), 0.0, 1.0 );
	#endif
}`,q1=`#define DISTANCE
varying vec3 vWorldPosition;
#include <common>
#include <batching_pars_vertex>
#include <uv_pars_vertex>
#include <displacementmap_pars_vertex>
#include <morphtarget_pars_vertex>
#include <skinning_pars_vertex>
#include <clipping_planes_pars_vertex>
void main() {
	#include <uv_vertex>
	#include <batching_vertex>
	#include <skinbase_vertex>
	#include <morphinstance_vertex>
	#ifdef USE_DISPLACEMENTMAP
		#include <beginnormal_vertex>
		#include <morphnormal_vertex>
		#include <skinnormal_vertex>
	#endif
	#include <begin_vertex>
	#include <morphtarget_vertex>
	#include <skinning_vertex>
	#include <displacementmap_vertex>
	#include <project_vertex>
	#include <worldpos_vertex>
	#include <clipping_planes_vertex>
	vWorldPosition = worldPosition.xyz;
}`,$1=`#define DISTANCE
uniform vec3 referencePosition;
uniform float nearDistance;
uniform float farDistance;
varying vec3 vWorldPosition;
#include <common>
#include <packing>
#include <uv_pars_fragment>
#include <map_pars_fragment>
#include <alphamap_pars_fragment>
#include <alphatest_pars_fragment>
#include <alphahash_pars_fragment>
#include <clipping_planes_pars_fragment>
void main () {
	vec4 diffuseColor = vec4( 1.0 );
	#include <clipping_planes_fragment>
	#include <map_fragment>
	#include <alphamap_fragment>
	#include <alphatest_fragment>
	#include <alphahash_fragment>
	float dist = length( vWorldPosition - referencePosition );
	dist = ( dist - nearDistance ) / ( farDistance - nearDistance );
	dist = saturate( dist );
	gl_FragColor = packDepthToRGBA( dist );
}`,K1=`varying vec3 vWorldDirection;
#include <common>
void main() {
	vWorldDirection = transformDirection( position, modelMatrix );
	#include <begin_vertex>
	#include <project_vertex>
}`,Z1=`uniform sampler2D tEquirect;
varying vec3 vWorldDirection;
#include <common>
void main() {
	vec3 direction = normalize( vWorldDirection );
	vec2 sampleUV = equirectUv( direction );
	gl_FragColor = texture2D( tEquirect, sampleUV );
	#include <tonemapping_fragment>
	#include <colorspace_fragment>
}`,J1=`uniform float scale;
attribute float lineDistance;
varying float vLineDistance;
#include <common>
#include <uv_pars_vertex>
#include <color_pars_vertex>
#include <fog_pars_vertex>
#include <morphtarget_pars_vertex>
#include <logdepthbuf_pars_vertex>
#include <clipping_planes_pars_vertex>
void main() {
	vLineDistance = scale * lineDistance;
	#include <uv_vertex>
	#include <color_vertex>
	#include <morphinstance_vertex>
	#include <morphcolor_vertex>
	#include <begin_vertex>
	#include <morphtarget_vertex>
	#include <project_vertex>
	#include <logdepthbuf_vertex>
	#include <clipping_planes_vertex>
	#include <fog_vertex>
}`,Q1=`uniform vec3 diffuse;
uniform float opacity;
uniform float dashSize;
uniform float totalSize;
varying float vLineDistance;
#include <common>
#include <color_pars_fragment>
#include <uv_pars_fragment>
#include <map_pars_fragment>
#include <fog_pars_fragment>
#include <logdepthbuf_pars_fragment>
#include <clipping_planes_pars_fragment>
void main() {
	vec4 diffuseColor = vec4( diffuse, opacity );
	#include <clipping_planes_fragment>
	if ( mod( vLineDistance, totalSize ) > dashSize ) {
		discard;
	}
	vec3 outgoingLight = vec3( 0.0 );
	#include <logdepthbuf_fragment>
	#include <map_fragment>
	#include <color_fragment>
	outgoingLight = diffuseColor.rgb;
	#include <opaque_fragment>
	#include <tonemapping_fragment>
	#include <colorspace_fragment>
	#include <fog_fragment>
	#include <premultiplied_alpha_fragment>
}`,eE=`#include <common>
#include <batching_pars_vertex>
#include <uv_pars_vertex>
#include <envmap_pars_vertex>
#include <color_pars_vertex>
#include <fog_pars_vertex>
#include <morphtarget_pars_vertex>
#include <skinning_pars_vertex>
#include <logdepthbuf_pars_vertex>
#include <clipping_planes_pars_vertex>
void main() {
	#include <uv_vertex>
	#include <color_vertex>
	#include <morphinstance_vertex>
	#include <morphcolor_vertex>
	#include <batching_vertex>
	#if defined ( USE_ENVMAP ) || defined ( USE_SKINNING )
		#include <beginnormal_vertex>
		#include <morphnormal_vertex>
		#include <skinbase_vertex>
		#include <skinnormal_vertex>
		#include <defaultnormal_vertex>
	#endif
	#include <begin_vertex>
	#include <morphtarget_vertex>
	#include <skinning_vertex>
	#include <project_vertex>
	#include <logdepthbuf_vertex>
	#include <clipping_planes_vertex>
	#include <worldpos_vertex>
	#include <envmap_vertex>
	#include <fog_vertex>
}`,tE=`uniform vec3 diffuse;
uniform float opacity;
#ifndef FLAT_SHADED
	varying vec3 vNormal;
#endif
#include <common>
#include <dithering_pars_fragment>
#include <color_pars_fragment>
#include <uv_pars_fragment>
#include <map_pars_fragment>
#include <alphamap_pars_fragment>
#include <alphatest_pars_fragment>
#include <alphahash_pars_fragment>
#include <aomap_pars_fragment>
#include <lightmap_pars_fragment>
#include <envmap_common_pars_fragment>
#include <envmap_pars_fragment>
#include <fog_pars_fragment>
#include <specularmap_pars_fragment>
#include <logdepthbuf_pars_fragment>
#include <clipping_planes_pars_fragment>
void main() {
	vec4 diffuseColor = vec4( diffuse, opacity );
	#include <clipping_planes_fragment>
	#include <logdepthbuf_fragment>
	#include <map_fragment>
	#include <color_fragment>
	#include <alphamap_fragment>
	#include <alphatest_fragment>
	#include <alphahash_fragment>
	#include <specularmap_fragment>
	ReflectedLight reflectedLight = ReflectedLight( vec3( 0.0 ), vec3( 0.0 ), vec3( 0.0 ), vec3( 0.0 ) );
	#ifdef USE_LIGHTMAP
		vec4 lightMapTexel = texture2D( lightMap, vLightMapUv );
		reflectedLight.indirectDiffuse += lightMapTexel.rgb * lightMapIntensity * RECIPROCAL_PI;
	#else
		reflectedLight.indirectDiffuse += vec3( 1.0 );
	#endif
	#include <aomap_fragment>
	reflectedLight.indirectDiffuse *= diffuseColor.rgb;
	vec3 outgoingLight = reflectedLight.indirectDiffuse;
	#include <envmap_fragment>
	#include <opaque_fragment>
	#include <tonemapping_fragment>
	#include <colorspace_fragment>
	#include <fog_fragment>
	#include <premultiplied_alpha_fragment>
	#include <dithering_fragment>
}`,nE=`#define LAMBERT
varying vec3 vViewPosition;
#include <common>
#include <batching_pars_vertex>
#include <uv_pars_vertex>
#include <displacementmap_pars_vertex>
#include <envmap_pars_vertex>
#include <color_pars_vertex>
#include <fog_pars_vertex>
#include <normal_pars_vertex>
#include <morphtarget_pars_vertex>
#include <skinning_pars_vertex>
#include <shadowmap_pars_vertex>
#include <logdepthbuf_pars_vertex>
#include <clipping_planes_pars_vertex>
void main() {
	#include <uv_vertex>
	#include <color_vertex>
	#include <morphinstance_vertex>
	#include <morphcolor_vertex>
	#include <batching_vertex>
	#include <beginnormal_vertex>
	#include <morphnormal_vertex>
	#include <skinbase_vertex>
	#include <skinnormal_vertex>
	#include <defaultnormal_vertex>
	#include <normal_vertex>
	#include <begin_vertex>
	#include <morphtarget_vertex>
	#include <skinning_vertex>
	#include <displacementmap_vertex>
	#include <project_vertex>
	#include <logdepthbuf_vertex>
	#include <clipping_planes_vertex>
	vViewPosition = - mvPosition.xyz;
	#include <worldpos_vertex>
	#include <envmap_vertex>
	#include <shadowmap_vertex>
	#include <fog_vertex>
}`,iE=`#define LAMBERT
uniform vec3 diffuse;
uniform vec3 emissive;
uniform float opacity;
#include <common>
#include <packing>
#include <dithering_pars_fragment>
#include <color_pars_fragment>
#include <uv_pars_fragment>
#include <map_pars_fragment>
#include <alphamap_pars_fragment>
#include <alphatest_pars_fragment>
#include <alphahash_pars_fragment>
#include <aomap_pars_fragment>
#include <lightmap_pars_fragment>
#include <emissivemap_pars_fragment>
#include <envmap_common_pars_fragment>
#include <envmap_pars_fragment>
#include <fog_pars_fragment>
#include <bsdfs>
#include <lights_pars_begin>
#include <normal_pars_fragment>
#include <lights_lambert_pars_fragment>
#include <shadowmap_pars_fragment>
#include <bumpmap_pars_fragment>
#include <normalmap_pars_fragment>
#include <specularmap_pars_fragment>
#include <logdepthbuf_pars_fragment>
#include <clipping_planes_pars_fragment>
void main() {
	vec4 diffuseColor = vec4( diffuse, opacity );
	#include <clipping_planes_fragment>
	ReflectedLight reflectedLight = ReflectedLight( vec3( 0.0 ), vec3( 0.0 ), vec3( 0.0 ), vec3( 0.0 ) );
	vec3 totalEmissiveRadiance = emissive;
	#include <logdepthbuf_fragment>
	#include <map_fragment>
	#include <color_fragment>
	#include <alphamap_fragment>
	#include <alphatest_fragment>
	#include <alphahash_fragment>
	#include <specularmap_fragment>
	#include <normal_fragment_begin>
	#include <normal_fragment_maps>
	#include <emissivemap_fragment>
	#include <lights_lambert_fragment>
	#include <lights_fragment_begin>
	#include <lights_fragment_maps>
	#include <lights_fragment_end>
	#include <aomap_fragment>
	vec3 outgoingLight = reflectedLight.directDiffuse + reflectedLight.indirectDiffuse + totalEmissiveRadiance;
	#include <envmap_fragment>
	#include <opaque_fragment>
	#include <tonemapping_fragment>
	#include <colorspace_fragment>
	#include <fog_fragment>
	#include <premultiplied_alpha_fragment>
	#include <dithering_fragment>
}`,rE=`#define MATCAP
varying vec3 vViewPosition;
#include <common>
#include <batching_pars_vertex>
#include <uv_pars_vertex>
#include <color_pars_vertex>
#include <displacementmap_pars_vertex>
#include <fog_pars_vertex>
#include <normal_pars_vertex>
#include <morphtarget_pars_vertex>
#include <skinning_pars_vertex>
#include <logdepthbuf_pars_vertex>
#include <clipping_planes_pars_vertex>
void main() {
	#include <uv_vertex>
	#include <color_vertex>
	#include <morphinstance_vertex>
	#include <morphcolor_vertex>
	#include <batching_vertex>
	#include <beginnormal_vertex>
	#include <morphnormal_vertex>
	#include <skinbase_vertex>
	#include <skinnormal_vertex>
	#include <defaultnormal_vertex>
	#include <normal_vertex>
	#include <begin_vertex>
	#include <morphtarget_vertex>
	#include <skinning_vertex>
	#include <displacementmap_vertex>
	#include <project_vertex>
	#include <logdepthbuf_vertex>
	#include <clipping_planes_vertex>
	#include <fog_vertex>
	vViewPosition = - mvPosition.xyz;
}`,oE=`#define MATCAP
uniform vec3 diffuse;
uniform float opacity;
uniform sampler2D matcap;
varying vec3 vViewPosition;
#include <common>
#include <dithering_pars_fragment>
#include <color_pars_fragment>
#include <uv_pars_fragment>
#include <map_pars_fragment>
#include <alphamap_pars_fragment>
#include <alphatest_pars_fragment>
#include <alphahash_pars_fragment>
#include <fog_pars_fragment>
#include <normal_pars_fragment>
#include <bumpmap_pars_fragment>
#include <normalmap_pars_fragment>
#include <logdepthbuf_pars_fragment>
#include <clipping_planes_pars_fragment>
void main() {
	vec4 diffuseColor = vec4( diffuse, opacity );
	#include <clipping_planes_fragment>
	#include <logdepthbuf_fragment>
	#include <map_fragment>
	#include <color_fragment>
	#include <alphamap_fragment>
	#include <alphatest_fragment>
	#include <alphahash_fragment>
	#include <normal_fragment_begin>
	#include <normal_fragment_maps>
	vec3 viewDir = normalize( vViewPosition );
	vec3 x = normalize( vec3( viewDir.z, 0.0, - viewDir.x ) );
	vec3 y = cross( viewDir, x );
	vec2 uv = vec2( dot( x, normal ), dot( y, normal ) ) * 0.495 + 0.5;
	#ifdef USE_MATCAP
		vec4 matcapColor = texture2D( matcap, uv );
	#else
		vec4 matcapColor = vec4( vec3( mix( 0.2, 0.8, uv.y ) ), 1.0 );
	#endif
	vec3 outgoingLight = diffuseColor.rgb * matcapColor.rgb;
	#include <opaque_fragment>
	#include <tonemapping_fragment>
	#include <colorspace_fragment>
	#include <fog_fragment>
	#include <premultiplied_alpha_fragment>
	#include <dithering_fragment>
}`,sE=`#define NORMAL
#if defined( FLAT_SHADED ) || defined( USE_BUMPMAP ) || defined( USE_NORMALMAP_TANGENTSPACE )
	varying vec3 vViewPosition;
#endif
#include <common>
#include <batching_pars_vertex>
#include <uv_pars_vertex>
#include <displacementmap_pars_vertex>
#include <normal_pars_vertex>
#include <morphtarget_pars_vertex>
#include <skinning_pars_vertex>
#include <logdepthbuf_pars_vertex>
#include <clipping_planes_pars_vertex>
void main() {
	#include <uv_vertex>
	#include <batching_vertex>
	#include <beginnormal_vertex>
	#include <morphinstance_vertex>
	#include <morphnormal_vertex>
	#include <skinbase_vertex>
	#include <skinnormal_vertex>
	#include <defaultnormal_vertex>
	#include <normal_vertex>
	#include <begin_vertex>
	#include <morphtarget_vertex>
	#include <skinning_vertex>
	#include <displacementmap_vertex>
	#include <project_vertex>
	#include <logdepthbuf_vertex>
	#include <clipping_planes_vertex>
#if defined( FLAT_SHADED ) || defined( USE_BUMPMAP ) || defined( USE_NORMALMAP_TANGENTSPACE )
	vViewPosition = - mvPosition.xyz;
#endif
}`,aE=`#define NORMAL
uniform float opacity;
#if defined( FLAT_SHADED ) || defined( USE_BUMPMAP ) || defined( USE_NORMALMAP_TANGENTSPACE )
	varying vec3 vViewPosition;
#endif
#include <packing>
#include <uv_pars_fragment>
#include <normal_pars_fragment>
#include <bumpmap_pars_fragment>
#include <normalmap_pars_fragment>
#include <logdepthbuf_pars_fragment>
#include <clipping_planes_pars_fragment>
void main() {
	vec4 diffuseColor = vec4( 0.0, 0.0, 0.0, opacity );
	#include <clipping_planes_fragment>
	#include <logdepthbuf_fragment>
	#include <normal_fragment_begin>
	#include <normal_fragment_maps>
	gl_FragColor = vec4( packNormalToRGB( normal ), diffuseColor.a );
	#ifdef OPAQUE
		gl_FragColor.a = 1.0;
	#endif
}`,lE=`#define PHONG
varying vec3 vViewPosition;
#include <common>
#include <batching_pars_vertex>
#include <uv_pars_vertex>
#include <displacementmap_pars_vertex>
#include <envmap_pars_vertex>
#include <color_pars_vertex>
#include <fog_pars_vertex>
#include <normal_pars_vertex>
#include <morphtarget_pars_vertex>
#include <skinning_pars_vertex>
#include <shadowmap_pars_vertex>
#include <logdepthbuf_pars_vertex>
#include <clipping_planes_pars_vertex>
void main() {
	#include <uv_vertex>
	#include <color_vertex>
	#include <morphcolor_vertex>
	#include <batching_vertex>
	#include <beginnormal_vertex>
	#include <morphinstance_vertex>
	#include <morphnormal_vertex>
	#include <skinbase_vertex>
	#include <skinnormal_vertex>
	#include <defaultnormal_vertex>
	#include <normal_vertex>
	#include <begin_vertex>
	#include <morphtarget_vertex>
	#include <skinning_vertex>
	#include <displacementmap_vertex>
	#include <project_vertex>
	#include <logdepthbuf_vertex>
	#include <clipping_planes_vertex>
	vViewPosition = - mvPosition.xyz;
	#include <worldpos_vertex>
	#include <envmap_vertex>
	#include <shadowmap_vertex>
	#include <fog_vertex>
}`,cE=`#define PHONG
uniform vec3 diffuse;
uniform vec3 emissive;
uniform vec3 specular;
uniform float shininess;
uniform float opacity;
#include <common>
#include <packing>
#include <dithering_pars_fragment>
#include <color_pars_fragment>
#include <uv_pars_fragment>
#include <map_pars_fragment>
#include <alphamap_pars_fragment>
#include <alphatest_pars_fragment>
#include <alphahash_pars_fragment>
#include <aomap_pars_fragment>
#include <lightmap_pars_fragment>
#include <emissivemap_pars_fragment>
#include <envmap_common_pars_fragment>
#include <envmap_pars_fragment>
#include <fog_pars_fragment>
#include <bsdfs>
#include <lights_pars_begin>
#include <normal_pars_fragment>
#include <lights_phong_pars_fragment>
#include <shadowmap_pars_fragment>
#include <bumpmap_pars_fragment>
#include <normalmap_pars_fragment>
#include <specularmap_pars_fragment>
#include <logdepthbuf_pars_fragment>
#include <clipping_planes_pars_fragment>
void main() {
	vec4 diffuseColor = vec4( diffuse, opacity );
	#include <clipping_planes_fragment>
	ReflectedLight reflectedLight = ReflectedLight( vec3( 0.0 ), vec3( 0.0 ), vec3( 0.0 ), vec3( 0.0 ) );
	vec3 totalEmissiveRadiance = emissive;
	#include <logdepthbuf_fragment>
	#include <map_fragment>
	#include <color_fragment>
	#include <alphamap_fragment>
	#include <alphatest_fragment>
	#include <alphahash_fragment>
	#include <specularmap_fragment>
	#include <normal_fragment_begin>
	#include <normal_fragment_maps>
	#include <emissivemap_fragment>
	#include <lights_phong_fragment>
	#include <lights_fragment_begin>
	#include <lights_fragment_maps>
	#include <lights_fragment_end>
	#include <aomap_fragment>
	vec3 outgoingLight = reflectedLight.directDiffuse + reflectedLight.indirectDiffuse + reflectedLight.directSpecular + reflectedLight.indirectSpecular + totalEmissiveRadiance;
	#include <envmap_fragment>
	#include <opaque_fragment>
	#include <tonemapping_fragment>
	#include <colorspace_fragment>
	#include <fog_fragment>
	#include <premultiplied_alpha_fragment>
	#include <dithering_fragment>
}`,uE=`#define STANDARD
varying vec3 vViewPosition;
#ifdef USE_TRANSMISSION
	varying vec3 vWorldPosition;
#endif
#include <common>
#include <batching_pars_vertex>
#include <uv_pars_vertex>
#include <displacementmap_pars_vertex>
#include <color_pars_vertex>
#include <fog_pars_vertex>
#include <normal_pars_vertex>
#include <morphtarget_pars_vertex>
#include <skinning_pars_vertex>
#include <shadowmap_pars_vertex>
#include <logdepthbuf_pars_vertex>
#include <clipping_planes_pars_vertex>
void main() {
	#include <uv_vertex>
	#include <color_vertex>
	#include <morphinstance_vertex>
	#include <morphcolor_vertex>
	#include <batching_vertex>
	#include <beginnormal_vertex>
	#include <morphnormal_vertex>
	#include <skinbase_vertex>
	#include <skinnormal_vertex>
	#include <defaultnormal_vertex>
	#include <normal_vertex>
	#include <begin_vertex>
	#include <morphtarget_vertex>
	#include <skinning_vertex>
	#include <displacementmap_vertex>
	#include <project_vertex>
	#include <logdepthbuf_vertex>
	#include <clipping_planes_vertex>
	vViewPosition = - mvPosition.xyz;
	#include <worldpos_vertex>
	#include <shadowmap_vertex>
	#include <fog_vertex>
#ifdef USE_TRANSMISSION
	vWorldPosition = worldPosition.xyz;
#endif
}`,dE=`#define STANDARD
#ifdef PHYSICAL
	#define IOR
	#define USE_SPECULAR
#endif
uniform vec3 diffuse;
uniform vec3 emissive;
uniform float roughness;
uniform float metalness;
uniform float opacity;
#ifdef IOR
	uniform float ior;
#endif
#ifdef USE_SPECULAR
	uniform float specularIntensity;
	uniform vec3 specularColor;
	#ifdef USE_SPECULAR_COLORMAP
		uniform sampler2D specularColorMap;
	#endif
	#ifdef USE_SPECULAR_INTENSITYMAP
		uniform sampler2D specularIntensityMap;
	#endif
#endif
#ifdef USE_CLEARCOAT
	uniform float clearcoat;
	uniform float clearcoatRoughness;
#endif
#ifdef USE_DISPERSION
	uniform float dispersion;
#endif
#ifdef USE_IRIDESCENCE
	uniform float iridescence;
	uniform float iridescenceIOR;
	uniform float iridescenceThicknessMinimum;
	uniform float iridescenceThicknessMaximum;
#endif
#ifdef USE_SHEEN
	uniform vec3 sheenColor;
	uniform float sheenRoughness;
	#ifdef USE_SHEEN_COLORMAP
		uniform sampler2D sheenColorMap;
	#endif
	#ifdef USE_SHEEN_ROUGHNESSMAP
		uniform sampler2D sheenRoughnessMap;
	#endif
#endif
#ifdef USE_ANISOTROPY
	uniform vec2 anisotropyVector;
	#ifdef USE_ANISOTROPYMAP
		uniform sampler2D anisotropyMap;
	#endif
#endif
varying vec3 vViewPosition;
#include <common>
#include <packing>
#include <dithering_pars_fragment>
#include <color_pars_fragment>
#include <uv_pars_fragment>
#include <map_pars_fragment>
#include <alphamap_pars_fragment>
#include <alphatest_pars_fragment>
#include <alphahash_pars_fragment>
#include <aomap_pars_fragment>
#include <lightmap_pars_fragment>
#include <emissivemap_pars_fragment>
#include <iridescence_fragment>
#include <cube_uv_reflection_fragment>
#include <envmap_common_pars_fragment>
#include <envmap_physical_pars_fragment>
#include <fog_pars_fragment>
#include <lights_pars_begin>
#include <normal_pars_fragment>
#include <lights_physical_pars_fragment>
#include <transmission_pars_fragment>
#include <shadowmap_pars_fragment>
#include <bumpmap_pars_fragment>
#include <normalmap_pars_fragment>
#include <clearcoat_pars_fragment>
#include <iridescence_pars_fragment>
#include <roughnessmap_pars_fragment>
#include <metalnessmap_pars_fragment>
#include <logdepthbuf_pars_fragment>
#include <clipping_planes_pars_fragment>
void main() {
	vec4 diffuseColor = vec4( diffuse, opacity );
	#include <clipping_planes_fragment>
	ReflectedLight reflectedLight = ReflectedLight( vec3( 0.0 ), vec3( 0.0 ), vec3( 0.0 ), vec3( 0.0 ) );
	vec3 totalEmissiveRadiance = emissive;
	#include <logdepthbuf_fragment>
	#include <map_fragment>
	#include <color_fragment>
	#include <alphamap_fragment>
	#include <alphatest_fragment>
	#include <alphahash_fragment>
	#include <roughnessmap_fragment>
	#include <metalnessmap_fragment>
	#include <normal_fragment_begin>
	#include <normal_fragment_maps>
	#include <clearcoat_normal_fragment_begin>
	#include <clearcoat_normal_fragment_maps>
	#include <emissivemap_fragment>
	#include <lights_physical_fragment>
	#include <lights_fragment_begin>
	#include <lights_fragment_maps>
	#include <lights_fragment_end>
	#include <aomap_fragment>
	vec3 totalDiffuse = reflectedLight.directDiffuse + reflectedLight.indirectDiffuse;
	vec3 totalSpecular = reflectedLight.directSpecular + reflectedLight.indirectSpecular;
	#include <transmission_fragment>
	vec3 outgoingLight = totalDiffuse + totalSpecular + totalEmissiveRadiance;
	#ifdef USE_SHEEN
		float sheenEnergyComp = 1.0 - 0.157 * max3( material.sheenColor );
		outgoingLight = outgoingLight * sheenEnergyComp + sheenSpecularDirect + sheenSpecularIndirect;
	#endif
	#ifdef USE_CLEARCOAT
		float dotNVcc = saturate( dot( geometryClearcoatNormal, geometryViewDir ) );
		vec3 Fcc = F_Schlick( material.clearcoatF0, material.clearcoatF90, dotNVcc );
		outgoingLight = outgoingLight * ( 1.0 - material.clearcoat * Fcc ) + ( clearcoatSpecularDirect + clearcoatSpecularIndirect ) * material.clearcoat;
	#endif
	#include <opaque_fragment>
	#include <tonemapping_fragment>
	#include <colorspace_fragment>
	#include <fog_fragment>
	#include <premultiplied_alpha_fragment>
	#include <dithering_fragment>
}`,hE=`#define TOON
varying vec3 vViewPosition;
#include <common>
#include <batching_pars_vertex>
#include <uv_pars_vertex>
#include <displacementmap_pars_vertex>
#include <color_pars_vertex>
#include <fog_pars_vertex>
#include <normal_pars_vertex>
#include <morphtarget_pars_vertex>
#include <skinning_pars_vertex>
#include <shadowmap_pars_vertex>
#include <logdepthbuf_pars_vertex>
#include <clipping_planes_pars_vertex>
void main() {
	#include <uv_vertex>
	#include <color_vertex>
	#include <morphinstance_vertex>
	#include <morphcolor_vertex>
	#include <batching_vertex>
	#include <beginnormal_vertex>
	#include <morphnormal_vertex>
	#include <skinbase_vertex>
	#include <skinnormal_vertex>
	#include <defaultnormal_vertex>
	#include <normal_vertex>
	#include <begin_vertex>
	#include <morphtarget_vertex>
	#include <skinning_vertex>
	#include <displacementmap_vertex>
	#include <project_vertex>
	#include <logdepthbuf_vertex>
	#include <clipping_planes_vertex>
	vViewPosition = - mvPosition.xyz;
	#include <worldpos_vertex>
	#include <shadowmap_vertex>
	#include <fog_vertex>
}`,fE=`#define TOON
uniform vec3 diffuse;
uniform vec3 emissive;
uniform float opacity;
#include <common>
#include <packing>
#include <dithering_pars_fragment>
#include <color_pars_fragment>
#include <uv_pars_fragment>
#include <map_pars_fragment>
#include <alphamap_pars_fragment>
#include <alphatest_pars_fragment>
#include <alphahash_pars_fragment>
#include <aomap_pars_fragment>
#include <lightmap_pars_fragment>
#include <emissivemap_pars_fragment>
#include <gradientmap_pars_fragment>
#include <fog_pars_fragment>
#include <bsdfs>
#include <lights_pars_begin>
#include <normal_pars_fragment>
#include <lights_toon_pars_fragment>
#include <shadowmap_pars_fragment>
#include <bumpmap_pars_fragment>
#include <normalmap_pars_fragment>
#include <logdepthbuf_pars_fragment>
#include <clipping_planes_pars_fragment>
void main() {
	vec4 diffuseColor = vec4( diffuse, opacity );
	#include <clipping_planes_fragment>
	ReflectedLight reflectedLight = ReflectedLight( vec3( 0.0 ), vec3( 0.0 ), vec3( 0.0 ), vec3( 0.0 ) );
	vec3 totalEmissiveRadiance = emissive;
	#include <logdepthbuf_fragment>
	#include <map_fragment>
	#include <color_fragment>
	#include <alphamap_fragment>
	#include <alphatest_fragment>
	#include <alphahash_fragment>
	#include <normal_fragment_begin>
	#include <normal_fragment_maps>
	#include <emissivemap_fragment>
	#include <lights_toon_fragment>
	#include <lights_fragment_begin>
	#include <lights_fragment_maps>
	#include <lights_fragment_end>
	#include <aomap_fragment>
	vec3 outgoingLight = reflectedLight.directDiffuse + reflectedLight.indirectDiffuse + totalEmissiveRadiance;
	#include <opaque_fragment>
	#include <tonemapping_fragment>
	#include <colorspace_fragment>
	#include <fog_fragment>
	#include <premultiplied_alpha_fragment>
	#include <dithering_fragment>
}`,pE=`uniform float size;
uniform float scale;
#include <common>
#include <color_pars_vertex>
#include <fog_pars_vertex>
#include <morphtarget_pars_vertex>
#include <logdepthbuf_pars_vertex>
#include <clipping_planes_pars_vertex>
#ifdef USE_POINTS_UV
	varying vec2 vUv;
	uniform mat3 uvTransform;
#endif
void main() {
	#ifdef USE_POINTS_UV
		vUv = ( uvTransform * vec3( uv, 1 ) ).xy;
	#endif
	#include <color_vertex>
	#include <morphinstance_vertex>
	#include <morphcolor_vertex>
	#include <begin_vertex>
	#include <morphtarget_vertex>
	#include <project_vertex>
	gl_PointSize = size;
	#ifdef USE_SIZEATTENUATION
		bool isPerspective = isPerspectiveMatrix( projectionMatrix );
		if ( isPerspective ) gl_PointSize *= ( scale / - mvPosition.z );
	#endif
	#include <logdepthbuf_vertex>
	#include <clipping_planes_vertex>
	#include <worldpos_vertex>
	#include <fog_vertex>
}`,mE=`uniform vec3 diffuse;
uniform float opacity;
#include <common>
#include <color_pars_fragment>
#include <map_particle_pars_fragment>
#include <alphatest_pars_fragment>
#include <alphahash_pars_fragment>
#include <fog_pars_fragment>
#include <logdepthbuf_pars_fragment>
#include <clipping_planes_pars_fragment>
void main() {
	vec4 diffuseColor = vec4( diffuse, opacity );
	#include <clipping_planes_fragment>
	vec3 outgoingLight = vec3( 0.0 );
	#include <logdepthbuf_fragment>
	#include <map_particle_fragment>
	#include <color_fragment>
	#include <alphatest_fragment>
	#include <alphahash_fragment>
	outgoingLight = diffuseColor.rgb;
	#include <opaque_fragment>
	#include <tonemapping_fragment>
	#include <colorspace_fragment>
	#include <fog_fragment>
	#include <premultiplied_alpha_fragment>
}`,gE=`#include <common>
#include <batching_pars_vertex>
#include <fog_pars_vertex>
#include <morphtarget_pars_vertex>
#include <skinning_pars_vertex>
#include <logdepthbuf_pars_vertex>
#include <shadowmap_pars_vertex>
void main() {
	#include <batching_vertex>
	#include <beginnormal_vertex>
	#include <morphinstance_vertex>
	#include <morphnormal_vertex>
	#include <skinbase_vertex>
	#include <skinnormal_vertex>
	#include <defaultnormal_vertex>
	#include <begin_vertex>
	#include <morphtarget_vertex>
	#include <skinning_vertex>
	#include <project_vertex>
	#include <logdepthbuf_vertex>
	#include <worldpos_vertex>
	#include <shadowmap_vertex>
	#include <fog_vertex>
}`,vE=`uniform vec3 color;
uniform float opacity;
#include <common>
#include <packing>
#include <fog_pars_fragment>
#include <bsdfs>
#include <lights_pars_begin>
#include <logdepthbuf_pars_fragment>
#include <shadowmap_pars_fragment>
#include <shadowmask_pars_fragment>
void main() {
	#include <logdepthbuf_fragment>
	gl_FragColor = vec4( color, opacity * ( 1.0 - getShadowMask() ) );
	#include <tonemapping_fragment>
	#include <colorspace_fragment>
	#include <fog_fragment>
}`,yE=`uniform float rotation;
uniform vec2 center;
#include <common>
#include <uv_pars_vertex>
#include <fog_pars_vertex>
#include <logdepthbuf_pars_vertex>
#include <clipping_planes_pars_vertex>
void main() {
	#include <uv_vertex>
	vec4 mvPosition = modelViewMatrix[ 3 ];
	vec2 scale = vec2( length( modelMatrix[ 0 ].xyz ), length( modelMatrix[ 1 ].xyz ) );
	#ifndef USE_SIZEATTENUATION
		bool isPerspective = isPerspectiveMatrix( projectionMatrix );
		if ( isPerspective ) scale *= - mvPosition.z;
	#endif
	vec2 alignedPosition = ( position.xy - ( center - vec2( 0.5 ) ) ) * scale;
	vec2 rotatedPosition;
	rotatedPosition.x = cos( rotation ) * alignedPosition.x - sin( rotation ) * alignedPosition.y;
	rotatedPosition.y = sin( rotation ) * alignedPosition.x + cos( rotation ) * alignedPosition.y;
	mvPosition.xy += rotatedPosition;
	gl_Position = projectionMatrix * mvPosition;
	#include <logdepthbuf_vertex>
	#include <clipping_planes_vertex>
	#include <fog_vertex>
}`,_E=`uniform vec3 diffuse;
uniform float opacity;
#include <common>
#include <uv_pars_fragment>
#include <map_pars_fragment>
#include <alphamap_pars_fragment>
#include <alphatest_pars_fragment>
#include <alphahash_pars_fragment>
#include <fog_pars_fragment>
#include <logdepthbuf_pars_fragment>
#include <clipping_planes_pars_fragment>
void main() {
	vec4 diffuseColor = vec4( diffuse, opacity );
	#include <clipping_planes_fragment>
	vec3 outgoingLight = vec3( 0.0 );
	#include <logdepthbuf_fragment>
	#include <map_fragment>
	#include <alphamap_fragment>
	#include <alphatest_fragment>
	#include <alphahash_fragment>
	outgoingLight = diffuseColor.rgb;
	#include <opaque_fragment>
	#include <tonemapping_fragment>
	#include <colorspace_fragment>
	#include <fog_fragment>
}`,Ve={alphahash_fragment:HM,alphahash_pars_fragment:VM,alphamap_fragment:GM,alphamap_pars_fragment:WM,alphatest_fragment:XM,alphatest_pars_fragment:jM,aomap_fragment:YM,aomap_pars_fragment:qM,batching_pars_vertex:$M,batching_vertex:KM,begin_vertex:ZM,beginnormal_vertex:JM,bsdfs:QM,iridescence_fragment:ew,bumpmap_pars_fragment:tw,clipping_planes_fragment:nw,clipping_planes_pars_fragment:iw,clipping_planes_pars_vertex:rw,clipping_planes_vertex:ow,color_fragment:sw,color_pars_fragment:aw,color_pars_vertex:lw,color_vertex:cw,common:uw,cube_uv_reflection_fragment:dw,defaultnormal_vertex:hw,displacementmap_pars_vertex:fw,displacementmap_vertex:pw,emissivemap_fragment:mw,emissivemap_pars_fragment:gw,colorspace_fragment:vw,colorspace_pars_fragment:yw,envmap_fragment:_w,envmap_common_pars_fragment:xw,envmap_pars_fragment:Sw,envmap_pars_vertex:Mw,envmap_physical_pars_fragment:Nw,envmap_vertex:ww,fog_vertex:Ew,fog_pars_vertex:Tw,fog_fragment:Aw,fog_pars_fragment:Cw,gradientmap_pars_fragment:bw,lightmap_pars_fragment:Rw,lights_lambert_fragment:Pw,lights_lambert_pars_fragment:Dw,lights_pars_begin:Lw,lights_toon_fragment:Iw,lights_toon_pars_fragment:kw,lights_phong_fragment:Uw,lights_phong_pars_fragment:Fw,lights_physical_fragment:Ow,lights_physical_pars_fragment:zw,lights_fragment_begin:Bw,lights_fragment_maps:Hw,lights_fragment_end:Vw,logdepthbuf_fragment:Gw,logdepthbuf_pars_fragment:Ww,logdepthbuf_pars_vertex:Xw,logdepthbuf_vertex:jw,map_fragment:Yw,map_pars_fragment:qw,map_particle_fragment:$w,map_particle_pars_fragment:Kw,metalnessmap_fragment:Zw,metalnessmap_pars_fragment:Jw,morphinstance_vertex:Qw,morphcolor_vertex:e1,morphnormal_vertex:t1,morphtarget_pars_vertex:n1,morphtarget_vertex:i1,normal_fragment_begin:r1,normal_fragment_maps:o1,normal_pars_fragment:s1,normal_pars_vertex:a1,normal_vertex:l1,normalmap_pars_fragment:c1,clearcoat_normal_fragment_begin:u1,clearcoat_normal_fragment_maps:d1,clearcoat_pars_fragment:h1,iridescence_pars_fragment:f1,opaque_fragment:p1,packing:m1,premultiplied_alpha_fragment:g1,project_vertex:v1,dithering_fragment:y1,dithering_pars_fragment:_1,roughnessmap_fragment:x1,roughnessmap_pars_fragment:S1,shadowmap_pars_fragment:M1,shadowmap_pars_vertex:w1,shadowmap_vertex:E1,shadowmask_pars_fragment:T1,skinbase_vertex:A1,skinning_pars_vertex:C1,skinning_vertex:b1,skinnormal_vertex:R1,specularmap_fragment:P1,specularmap_pars_fragment:D1,tonemapping_fragment:L1,tonemapping_pars_fragment:N1,transmission_fragment:I1,transmission_pars_fragment:k1,uv_pars_fragment:U1,uv_pars_vertex:F1,uv_vertex:O1,worldpos_vertex:z1,background_vert:B1,background_frag:H1,backgroundCube_vert:V1,backgroundCube_frag:G1,cube_vert:W1,cube_frag:X1,depth_vert:j1,depth_frag:Y1,distanceRGBA_vert:q1,distanceRGBA_frag:$1,equirect_vert:K1,equirect_frag:Z1,linedashed_vert:J1,linedashed_frag:Q1,meshbasic_vert:eE,meshbasic_frag:tE,meshlambert_vert:nE,meshlambert_frag:iE,meshmatcap_vert:rE,meshmatcap_frag:oE,meshnormal_vert:sE,meshnormal_frag:aE,meshphong_vert:lE,meshphong_frag:cE,meshphysical_vert:uE,meshphysical_frag:dE,meshtoon_vert:hE,meshtoon_frag:fE,points_vert:pE,points_frag:mE,shadow_vert:gE,shadow_frag:vE,sprite_vert:yE,sprite_frag:_E},ce={common:{diffuse:{value:new we(16777215)},opacity:{value:1},map:{value:null},mapTransform:{value:new Be},alphaMap:{value:null},alphaMapTransform:{value:new Be},alphaTest:{value:0}},specularmap:{specularMap:{value:null},specularMapTransform:{value:new Be}},envmap:{envMap:{value:null},envMapRotation:{value:new Be},flipEnvMap:{value:-1},reflectivity:{value:1},ior:{value:1.5},refractionRatio:{value:.98}},aomap:{aoMap:{value:null},aoMapIntensity:{value:1},aoMapTransform:{value:new Be}},lightmap:{lightMap:{value:null},lightMapIntensity:{value:1},lightMapTransform:{value:new Be}},bumpmap:{bumpMap:{value:null},bumpMapTransform:{value:new Be},bumpScale:{value:1}},normalmap:{normalMap:{value:null},normalMapTransform:{value:new Be},normalScale:{value:new Re(1,1)}},displacementmap:{displacementMap:{value:null},displacementMapTransform:{value:new Be},displacementScale:{value:1},displacementBias:{value:0}},emissivemap:{emissiveMap:{value:null},emissiveMapTransform:{value:new Be}},metalnessmap:{metalnessMap:{value:null},metalnessMapTransform:{value:new Be}},roughnessmap:{roughnessMap:{value:null},roughnessMapTransform:{value:new Be}},gradientmap:{gradientMap:{value:null}},fog:{fogDensity:{value:25e-5},fogNear:{value:1},fogFar:{value:2e3},fogColor:{value:new we(16777215)}},lights:{ambientLightColor:{value:[]},lightProbe:{value:[]},directionalLights:{value:[],properties:{direction:{},color:{}}},directionalLightShadows:{value:[],properties:{shadowIntensity:1,shadowBias:{},shadowNormalBias:{},shadowRadius:{},shadowMapSize:{}}},directionalShadowMap:{value:[]},directionalShadowMatrix:{value:[]},spotLights:{value:[],properties:{color:{},position:{},direction:{},distance:{},coneCos:{},penumbraCos:{},decay:{}}},spotLightShadows:{value:[],properties:{shadowIntensity:1,shadowBias:{},shadowNormalBias:{},shadowRadius:{},shadowMapSize:{}}},spotLightMap:{value:[]},spotShadowMap:{value:[]},spotLightMatrix:{value:[]},pointLights:{value:[],properties:{color:{},position:{},decay:{},distance:{}}},pointLightShadows:{value:[],properties:{shadowIntensity:1,shadowBias:{},shadowNormalBias:{},shadowRadius:{},shadowMapSize:{},shadowCameraNear:{},shadowCameraFar:{}}},pointShadowMap:{value:[]},pointShadowMatrix:{value:[]},hemisphereLights:{value:[],properties:{direction:{},skyColor:{},groundColor:{}}},rectAreaLights:{value:[],properties:{color:{},position:{},width:{},height:{}}},ltc_1:{value:null},ltc_2:{value:null}},points:{diffuse:{value:new we(16777215)},opacity:{value:1},size:{value:1},scale:{value:1},map:{value:null},alphaMap:{value:null},alphaMapTransform:{value:new Be},alphaTest:{value:0},uvTransform:{value:new Be}},sprite:{diffuse:{value:new we(16777215)},opacity:{value:1},center:{value:new Re(.5,.5)},rotation:{value:0},map:{value:null},mapTransform:{value:new Be},alphaMap:{value:null},alphaMapTransform:{value:new Be},alphaTest:{value:0}}},Qn={basic:{uniforms:qt([ce.common,ce.specularmap,ce.envmap,ce.aomap,ce.lightmap,ce.fog]),vertexShader:Ve.meshbasic_vert,fragmentShader:Ve.meshbasic_frag},lambert:{uniforms:qt([ce.common,ce.specularmap,ce.envmap,ce.aomap,ce.lightmap,ce.emissivemap,ce.bumpmap,ce.normalmap,ce.displacementmap,ce.fog,ce.lights,{emissive:{value:new we(0)}}]),vertexShader:Ve.meshlambert_vert,fragmentShader:Ve.meshlambert_frag},phong:{uniforms:qt([ce.common,ce.specularmap,ce.envmap,ce.aomap,ce.lightmap,ce.emissivemap,ce.bumpmap,ce.normalmap,ce.displacementmap,ce.fog,ce.lights,{emissive:{value:new we(0)},specular:{value:new we(1118481)},shininess:{value:30}}]),vertexShader:Ve.meshphong_vert,fragmentShader:Ve.meshphong_frag},standard:{uniforms:qt([ce.common,ce.envmap,ce.aomap,ce.lightmap,ce.emissivemap,ce.bumpmap,ce.normalmap,ce.displacementmap,ce.roughnessmap,ce.metalnessmap,ce.fog,ce.lights,{emissive:{value:new we(0)},roughness:{value:1},metalness:{value:0},envMapIntensity:{value:1}}]),vertexShader:Ve.meshphysical_vert,fragmentShader:Ve.meshphysical_frag},toon:{uniforms:qt([ce.common,ce.aomap,ce.lightmap,ce.emissivemap,ce.bumpmap,ce.normalmap,ce.displacementmap,ce.gradientmap,ce.fog,ce.lights,{emissive:{value:new we(0)}}]),vertexShader:Ve.meshtoon_vert,fragmentShader:Ve.meshtoon_frag},matcap:{uniforms:qt([ce.common,ce.bumpmap,ce.normalmap,ce.displacementmap,ce.fog,{matcap:{value:null}}]),vertexShader:Ve.meshmatcap_vert,fragmentShader:Ve.meshmatcap_frag},points:{uniforms:qt([ce.points,ce.fog]),vertexShader:Ve.points_vert,fragmentShader:Ve.points_frag},dashed:{uniforms:qt([ce.common,ce.fog,{scale:{value:1},dashSize:{value:1},totalSize:{value:2}}]),vertexShader:Ve.linedashed_vert,fragmentShader:Ve.linedashed_frag},depth:{uniforms:qt([ce.common,ce.displacementmap]),vertexShader:Ve.depth_vert,fragmentShader:Ve.depth_frag},normal:{uniforms:qt([ce.common,ce.bumpmap,ce.normalmap,ce.displacementmap,{opacity:{value:1}}]),vertexShader:Ve.meshnormal_vert,fragmentShader:Ve.meshnormal_frag},sprite:{uniforms:qt([ce.sprite,ce.fog]),vertexShader:Ve.sprite_vert,fragmentShader:Ve.sprite_frag},background:{uniforms:{uvTransform:{value:new Be},t2D:{value:null},backgroundIntensity:{value:1}},vertexShader:Ve.background_vert,fragmentShader:Ve.background_frag},backgroundCube:{uniforms:{envMap:{value:null},flipEnvMap:{value:-1},backgroundBlurriness:{value:0},backgroundIntensity:{value:1},backgroundRotation:{value:new Be}},vertexShader:Ve.backgroundCube_vert,fragmentShader:Ve.backgroundCube_frag},cube:{uniforms:{tCube:{value:null},tFlip:{value:-1},opacity:{value:1}},vertexShader:Ve.cube_vert,fragmentShader:Ve.cube_frag},equirect:{uniforms:{tEquirect:{value:null}},vertexShader:Ve.equirect_vert,fragmentShader:Ve.equirect_frag},distanceRGBA:{uniforms:qt([ce.common,ce.displacementmap,{referencePosition:{value:new L},nearDistance:{value:1},farDistance:{value:1e3}}]),vertexShader:Ve.distanceRGBA_vert,fragmentShader:Ve.distanceRGBA_frag},shadow:{uniforms:qt([ce.lights,ce.fog,{color:{value:new we(0)},opacity:{value:1}}]),vertexShader:Ve.shadow_vert,fragmentShader:Ve.shadow_frag}};Qn.physical={uniforms:qt([Qn.standard.uniforms,{clearcoat:{value:0},clearcoatMap:{value:null},clearcoatMapTransform:{value:new Be},clearcoatNormalMap:{value:null},clearcoatNormalMapTransform:{value:new Be},clearcoatNormalScale:{value:new Re(1,1)},clearcoatRoughness:{value:0},clearcoatRoughnessMap:{value:null},clearcoatRoughnessMapTransform:{value:new Be},dispersion:{value:0},iridescence:{value:0},iridescenceMap:{value:null},iridescenceMapTransform:{value:new Be},iridescenceIOR:{value:1.3},iridescenceThicknessMinimum:{value:100},iridescenceThicknessMaximum:{value:400},iridescenceThicknessMap:{value:null},iridescenceThicknessMapTransform:{value:new Be},sheen:{value:0},sheenColor:{value:new we(0)},sheenColorMap:{value:null},sheenColorMapTransform:{value:new Be},sheenRoughness:{value:1},sheenRoughnessMap:{value:null},sheenRoughnessMapTransform:{value:new Be},transmission:{value:0},transmissionMap:{value:null},transmissionMapTransform:{value:new Be},transmissionSamplerSize:{value:new Re},transmissionSamplerMap:{value:null},thickness:{value:0},thicknessMap:{value:null},thicknessMapTransform:{value:new Be},attenuationDistance:{value:0},attenuationColor:{value:new we(0)},specularColor:{value:new we(1,1,1)},specularColorMap:{value:null},specularColorMapTransform:{value:new Be},specularIntensity:{value:1},specularIntensityMap:{value:null},specularIntensityMapTransform:{value:new Be},anisotropyVector:{value:new Re},anisotropyMap:{value:null},anisotropyMapTransform:{value:new Be}}]),vertexShader:Ve.meshphysical_vert,fragmentShader:Ve.meshphysical_frag};const il={r:0,b:0,g:0},mr=new $n,xE=new rt;function SE(t,e,n,i,r,o,s){const a=new we(0);let l=o===!0?0:1,c,d,f=null,h=0,p=null;function v(g){let y=g.isScene===!0?g.background:null;return y&&y.isTexture&&(y=(g.backgroundBlurriness>0?n:e).get(y)),y}function _(g){let y=!1;const x=v(g);x===null?u(a,l):x&&x.isColor&&(u(x,1),y=!0);const C=t.xr.getEnvironmentBlendMode();C==="additive"?i.buffers.color.setClear(0,0,0,1,s):C==="alpha-blend"&&i.buffers.color.setClear(0,0,0,0,s),(t.autoClear||y)&&(i.buffers.depth.setTest(!0),i.buffers.depth.setMask(!0),i.buffers.color.setMask(!0),t.clear(t.autoClearColor,t.autoClearDepth,t.autoClearStencil))}function m(g,y){const x=v(y);x&&(x.isCubeTexture||x.mapping===Sc)?(d===void 0&&(d=new Ct(new ts(1,1,1),new xn({name:"BackgroundCubeMaterial",uniforms:Yo(Qn.backgroundCube.uniforms),vertexShader:Qn.backgroundCube.vertexShader,fragmentShader:Qn.backgroundCube.fragmentShader,side:Jt,depthTest:!1,depthWrite:!1,fog:!1})),d.geometry.deleteAttribute("normal"),d.geometry.deleteAttribute("uv"),d.onBeforeRender=function(C,A,T){this.matrixWorld.copyPosition(T.matrixWorld)},Object.defineProperty(d.material,"envMap",{get:function(){return this.uniforms.envMap.value}}),r.update(d)),mr.copy(y.backgroundRotation),mr.x*=-1,mr.y*=-1,mr.z*=-1,x.isCubeTexture&&x.isRenderTargetTexture===!1&&(mr.y*=-1,mr.z*=-1),d.material.uniforms.envMap.value=x,d.material.uniforms.flipEnvMap.value=x.isCubeTexture&&x.isRenderTargetTexture===!1?-1:1,d.material.uniforms.backgroundBlurriness.value=y.backgroundBlurriness,d.material.uniforms.backgroundIntensity.value=y.backgroundIntensity,d.material.uniforms.backgroundRotation.value.setFromMatrix4(xE.makeRotationFromEuler(mr)),d.material.toneMapped=$e.getTransfer(x.colorSpace)!==it,(f!==x||h!==x.version||p!==t.toneMapping)&&(d.material.needsUpdate=!0,f=x,h=x.version,p=t.toneMapping),d.layers.enableAll(),g.unshift(d,d.geometry,d.material,0,0,null)):x&&x.isTexture&&(c===void 0&&(c=new Ct(new Hr(2,2),new xn({name:"BackgroundMaterial",uniforms:Yo(Qn.background.uniforms),vertexShader:Qn.background.vertexShader,fragmentShader:Qn.background.fragmentShader,side:sr,depthTest:!1,depthWrite:!1,fog:!1})),c.geometry.deleteAttribute("normal"),Object.defineProperty(c.material,"map",{get:function(){return this.uniforms.t2D.value}}),r.update(c)),c.material.uniforms.t2D.value=x,c.material.uniforms.backgroundIntensity.value=y.backgroundIntensity,c.material.toneMapped=$e.getTransfer(x.colorSpace)!==it,x.matrixAutoUpdate===!0&&x.updateMatrix(),c.material.uniforms.uvTransform.value.copy(x.matrix),(f!==x||h!==x.version||p!==t.toneMapping)&&(c.material.needsUpdate=!0,f=x,h=x.version,p=t.toneMapping),c.layers.enableAll(),g.unshift(c,c.geometry,c.material,0,0,null))}function u(g,y){g.getRGB(il,oy(t)),i.buffers.color.setClear(il.r,il.g,il.b,y,s)}return{getClearColor:function(){return a},setClearColor:function(g,y=1){a.set(g),l=y,u(a,l)},getClearAlpha:function(){return l},setClearAlpha:function(g){l=g,u(a,l)},render:_,addToRenderList:m}}function ME(t,e){const n=t.getParameter(t.MAX_VERTEX_ATTRIBS),i={},r=h(null);let o=r,s=!1;function a(S,P,z,F,Y){let j=!1;const W=f(F,z,P);o!==W&&(o=W,c(o.object)),j=p(S,F,z,Y),j&&v(S,F,z,Y),Y!==null&&e.update(Y,t.ELEMENT_ARRAY_BUFFER),(j||s)&&(s=!1,x(S,P,z,F),Y!==null&&t.bindBuffer(t.ELEMENT_ARRAY_BUFFER,e.get(Y).buffer))}function l(){return t.createVertexArray()}function c(S){return t.bindVertexArray(S)}function d(S){return t.deleteVertexArray(S)}function f(S,P,z){const F=z.wireframe===!0;let Y=i[S.id];Y===void 0&&(Y={},i[S.id]=Y);let j=Y[P.id];j===void 0&&(j={},Y[P.id]=j);let W=j[F];return W===void 0&&(W=h(l()),j[F]=W),W}function h(S){const P=[],z=[],F=[];for(let Y=0;Y<n;Y++)P[Y]=0,z[Y]=0,F[Y]=0;return{geometry:null,program:null,wireframe:!1,newAttributes:P,enabledAttributes:z,attributeDivisors:F,object:S,attributes:{},index:null}}function p(S,P,z,F){const Y=o.attributes,j=P.attributes;let W=0;const $=z.getAttributes();for(const D in $)if($[D].location>=0){const Z=Y[D];let le=j[D];if(le===void 0&&(D==="instanceMatrix"&&S.instanceMatrix&&(le=S.instanceMatrix),D==="instanceColor"&&S.instanceColor&&(le=S.instanceColor)),Z===void 0||Z.attribute!==le||le&&Z.data!==le.data)return!0;W++}return o.attributesNum!==W||o.index!==F}function v(S,P,z,F){const Y={},j=P.attributes;let W=0;const $=z.getAttributes();for(const D in $)if($[D].location>=0){let Z=j[D];Z===void 0&&(D==="instanceMatrix"&&S.instanceMatrix&&(Z=S.instanceMatrix),D==="instanceColor"&&S.instanceColor&&(Z=S.instanceColor));const le={};le.attribute=Z,Z&&Z.data&&(le.data=Z.data),Y[D]=le,W++}o.attributes=Y,o.attributesNum=W,o.index=F}function _(){const S=o.newAttributes;for(let P=0,z=S.length;P<z;P++)S[P]=0}function m(S){u(S,0)}function u(S,P){const z=o.newAttributes,F=o.enabledAttributes,Y=o.attributeDivisors;z[S]=1,F[S]===0&&(t.enableVertexAttribArray(S),F[S]=1),Y[S]!==P&&(t.vertexAttribDivisor(S,P),Y[S]=P)}function g(){const S=o.newAttributes,P=o.enabledAttributes;for(let z=0,F=P.length;z<F;z++)P[z]!==S[z]&&(t.disableVertexAttribArray(z),P[z]=0)}function y(S,P,z,F,Y,j,W){W===!0?t.vertexAttribIPointer(S,P,z,Y,j):t.vertexAttribPointer(S,P,z,F,Y,j)}function x(S,P,z,F){_();const Y=F.attributes,j=z.getAttributes(),W=P.defaultAttributeValues;for(const $ in j){const D=j[$];if(D.location>=0){let G=Y[$];if(G===void 0&&($==="instanceMatrix"&&S.instanceMatrix&&(G=S.instanceMatrix),$==="instanceColor"&&S.instanceColor&&(G=S.instanceColor)),G!==void 0){const Z=G.normalized,le=G.itemSize,Se=e.get(G);if(Se===void 0)continue;const Ne=Se.buffer,q=Se.type,re=Se.bytesPerElement,he=q===t.INT||q===t.UNSIGNED_INT||G.gpuType===_f;if(G.isInterleavedBufferAttribute){const ae=G.data,Pe=ae.stride,ne=G.offset;if(ae.isInstancedInterleavedBuffer){for(let ve=0;ve<D.locationSize;ve++)u(D.location+ve,ae.meshPerAttribute);S.isInstancedMesh!==!0&&F._maxInstanceCount===void 0&&(F._maxInstanceCount=ae.meshPerAttribute*ae.count)}else for(let ve=0;ve<D.locationSize;ve++)m(D.location+ve);t.bindBuffer(t.ARRAY_BUFFER,Ne);for(let ve=0;ve<D.locationSize;ve++)y(D.location+ve,le/D.locationSize,q,Z,Pe*re,(ne+le/D.locationSize*ve)*re,he)}else{if(G.isInstancedBufferAttribute){for(let ae=0;ae<D.locationSize;ae++)u(D.location+ae,G.meshPerAttribute);S.isInstancedMesh!==!0&&F._maxInstanceCount===void 0&&(F._maxInstanceCount=G.meshPerAttribute*G.count)}else for(let ae=0;ae<D.locationSize;ae++)m(D.location+ae);t.bindBuffer(t.ARRAY_BUFFER,Ne);for(let ae=0;ae<D.locationSize;ae++)y(D.location+ae,le/D.locationSize,q,Z,le*re,le/D.locationSize*ae*re,he)}}else if(W!==void 0){const Z=W[$];if(Z!==void 0)switch(Z.length){case 2:t.vertexAttrib2fv(D.location,Z);break;case 3:t.vertexAttrib3fv(D.location,Z);break;case 4:t.vertexAttrib4fv(D.location,Z);break;default:t.vertexAttrib1fv(D.location,Z)}}}}g()}function C(){b();for(const S in i){const P=i[S];for(const z in P){const F=P[z];for(const Y in F)d(F[Y].object),delete F[Y];delete P[z]}delete i[S]}}function A(S){if(i[S.id]===void 0)return;const P=i[S.id];for(const z in P){const F=P[z];for(const Y in F)d(F[Y].object),delete F[Y];delete P[z]}delete i[S.id]}function T(S){for(const P in i){const z=i[P];if(z[S.id]===void 0)continue;const F=z[S.id];for(const Y in F)d(F[Y].object),delete F[Y];delete z[S.id]}}function b(){w(),s=!0,o!==r&&(o=r,c(o.object))}function w(){r.geometry=null,r.program=null,r.wireframe=!1}return{setup:a,reset:b,resetDefaultState:w,dispose:C,releaseStatesOfGeometry:A,releaseStatesOfProgram:T,initAttributes:_,enableAttribute:m,disableUnusedAttributes:g}}function wE(t,e,n){let i;function r(c){i=c}function o(c,d){t.drawArrays(i,c,d),n.update(d,i,1)}function s(c,d,f){f!==0&&(t.drawArraysInstanced(i,c,d,f),n.update(d,i,f))}function a(c,d,f){if(f===0)return;e.get("WEBGL_multi_draw").multiDrawArraysWEBGL(i,c,0,d,0,f);let p=0;for(let v=0;v<f;v++)p+=d[v];n.update(p,i,1)}function l(c,d,f,h){if(f===0)return;const p=e.get("WEBGL_multi_draw");if(p===null)for(let v=0;v<c.length;v++)s(c[v],d[v],h[v]);else{p.multiDrawArraysInstancedWEBGL(i,c,0,d,0,h,0,f);let v=0;for(let _=0;_<f;_++)v+=d[_]*h[_];n.update(v,i,1)}}this.setMode=r,this.render=o,this.renderInstances=s,this.renderMultiDraw=a,this.renderMultiDrawInstances=l}function EE(t,e,n,i){let r;function o(){if(r!==void 0)return r;if(e.has("EXT_texture_filter_anisotropic")===!0){const T=e.get("EXT_texture_filter_anisotropic");r=t.getParameter(T.MAX_TEXTURE_MAX_ANISOTROPY_EXT)}else r=0;return r}function s(T){return!(T!==Wt&&i.convert(T)!==t.getParameter(t.IMPLEMENTATION_COLOR_READ_FORMAT))}function a(T){const b=T===Zo&&(e.has("EXT_color_buffer_half_float")||e.has("EXT_color_buffer_float"));return!(T!==oi&&i.convert(T)!==t.getParameter(t.IMPLEMENTATION_COLOR_READ_TYPE)&&T!==Pn&&!b)}function l(T){if(T==="highp"){if(t.getShaderPrecisionFormat(t.VERTEX_SHADER,t.HIGH_FLOAT).precision>0&&t.getShaderPrecisionFormat(t.FRAGMENT_SHADER,t.HIGH_FLOAT).precision>0)return"highp";T="mediump"}return T==="mediump"&&t.getShaderPrecisionFormat(t.VERTEX_SHADER,t.MEDIUM_FLOAT).precision>0&&t.getShaderPrecisionFormat(t.FRAGMENT_SHADER,t.MEDIUM_FLOAT).precision>0?"mediump":"lowp"}let c=n.precision!==void 0?n.precision:"highp";const d=l(c);d!==c&&(console.warn("THREE.WebGLRenderer:",c,"not supported, using",d,"instead."),c=d);const f=n.logarithmicDepthBuffer===!0,h=n.reverseDepthBuffer===!0&&e.has("EXT_clip_control"),p=t.getParameter(t.MAX_TEXTURE_IMAGE_UNITS),v=t.getParameter(t.MAX_VERTEX_TEXTURE_IMAGE_UNITS),_=t.getParameter(t.MAX_TEXTURE_SIZE),m=t.getParameter(t.MAX_CUBE_MAP_TEXTURE_SIZE),u=t.getParameter(t.MAX_VERTEX_ATTRIBS),g=t.getParameter(t.MAX_VERTEX_UNIFORM_VECTORS),y=t.getParameter(t.MAX_VARYING_VECTORS),x=t.getParameter(t.MAX_FRAGMENT_UNIFORM_VECTORS),C=v>0,A=t.getParameter(t.MAX_SAMPLES);return{isWebGL2:!0,getMaxAnisotropy:o,getMaxPrecision:l,textureFormatReadable:s,textureTypeReadable:a,precision:c,logarithmicDepthBuffer:f,reverseDepthBuffer:h,maxTextures:p,maxVertexTextures:v,maxTextureSize:_,maxCubemapSize:m,maxAttributes:u,maxVertexUniforms:g,maxVaryings:y,maxFragmentUniforms:x,vertexTextures:C,maxSamples:A}}function TE(t){const e=this;let n=null,i=0,r=!1,o=!1;const s=new xr,a=new Be,l={value:null,needsUpdate:!1};this.uniform=l,this.numPlanes=0,this.numIntersection=0,this.init=function(f,h){const p=f.length!==0||h||i!==0||r;return r=h,i=f.length,p},this.beginShadows=function(){o=!0,d(null)},this.endShadows=function(){o=!1},this.setGlobalState=function(f,h){n=d(f,h,0)},this.setState=function(f,h,p){const v=f.clippingPlanes,_=f.clipIntersection,m=f.clipShadows,u=t.get(f);if(!r||v===null||v.length===0||o&&!m)o?d(null):c();else{const g=o?0:i,y=g*4;let x=u.clippingState||null;l.value=x,x=d(v,h,y,p);for(let C=0;C!==y;++C)x[C]=n[C];u.clippingState=x,this.numIntersection=_?this.numPlanes:0,this.numPlanes+=g}};function c(){l.value!==n&&(l.value=n,l.needsUpdate=i>0),e.numPlanes=i,e.numIntersection=0}function d(f,h,p,v){const _=f!==null?f.length:0;let m=null;if(_!==0){if(m=l.value,v!==!0||m===null){const u=p+_*4,g=h.matrixWorldInverse;a.getNormalMatrix(g),(m===null||m.length<u)&&(m=new Float32Array(u));for(let y=0,x=p;y!==_;++y,x+=4)s.copy(f[y]).applyMatrix4(g,a),s.normal.toArray(m,x),m[x+3]=s.constant}l.value=m,l.needsUpdate=!0}return e.numPlanes=_,e.numIntersection=0,m}}function AE(t){let e=new WeakMap;function n(s,a){return a===Vd?s.mapping=Vo:a===Gd&&(s.mapping=Go),s}function i(s){if(s&&s.isTexture){const a=s.mapping;if(a===Vd||a===Gd)if(e.has(s)){const l=e.get(s).texture;return n(l,s.mapping)}else{const l=s.image;if(l&&l.height>0){const c=new FM(l.height);return c.fromEquirectangularTexture(t,s),e.set(s,c),s.addEventListener("dispose",r),n(c.texture,s.mapping)}else return null}}return s}function r(s){const a=s.target;a.removeEventListener("dispose",r);const l=e.get(a);l!==void 0&&(e.delete(a),l.dispose())}function o(){e=new WeakMap}return{get:i,dispose:o}}class _a extends sy{constructor(e=-1,n=1,i=1,r=-1,o=.1,s=2e3){super(),this.isOrthographicCamera=!0,this.type="OrthographicCamera",this.zoom=1,this.view=null,this.left=e,this.right=n,this.top=i,this.bottom=r,this.near=o,this.far=s,this.updateProjectionMatrix()}copy(e,n){return super.copy(e,n),this.left=e.left,this.right=e.right,this.top=e.top,this.bottom=e.bottom,this.near=e.near,this.far=e.far,this.zoom=e.zoom,this.view=e.view===null?null:Object.assign({},e.view),this}setViewOffset(e,n,i,r,o,s){this.view===null&&(this.view={enabled:!0,fullWidth:1,fullHeight:1,offsetX:0,offsetY:0,width:1,height:1}),this.view.enabled=!0,this.view.fullWidth=e,this.view.fullHeight=n,this.view.offsetX=i,this.view.offsetY=r,this.view.width=o,this.view.height=s,this.updateProjectionMatrix()}clearViewOffset(){this.view!==null&&(this.view.enabled=!1),this.updateProjectionMatrix()}updateProjectionMatrix(){const e=(this.right-this.left)/(2*this.zoom),n=(this.top-this.bottom)/(2*this.zoom),i=(this.right+this.left)/2,r=(this.top+this.bottom)/2;let o=i-e,s=i+e,a=r+n,l=r-n;if(this.view!==null&&this.view.enabled){const c=(this.right-this.left)/this.view.fullWidth/this.zoom,d=(this.top-this.bottom)/this.view.fullHeight/this.zoom;o+=c*this.view.offsetX,s=o+c*this.view.width,a-=d*this.view.offsetY,l=a-d*this.view.height}this.projectionMatrix.makeOrthographic(o,s,a,l,this.near,this.far,this.coordinateSystem),this.projectionMatrixInverse.copy(this.projectionMatrix).invert()}toJSON(e){const n=super.toJSON(e);return n.object.zoom=this.zoom,n.object.left=this.left,n.object.right=this.right,n.object.top=this.top,n.object.bottom=this.bottom,n.object.near=this.near,n.object.far=this.far,this.view!==null&&(n.object.view=Object.assign({},this.view)),n}}const So=4,Am=[.125,.215,.35,.446,.526,.582],wr=20,Eu=new _a,Cm=new we;let Tu=null,Au=0,Cu=0,bu=!1;const Sr=(1+Math.sqrt(5))/2,io=1/Sr,bm=[new L(-Sr,io,0),new L(Sr,io,0),new L(-io,0,Sr),new L(io,0,Sr),new L(0,Sr,-io),new L(0,Sr,io),new L(-1,1,-1),new L(1,1,-1),new L(-1,1,1),new L(1,1,1)];class yh{constructor(e){this._renderer=e,this._pingPongRenderTarget=null,this._lodMax=0,this._cubeSize=0,this._lodPlanes=[],this._sizeLods=[],this._sigmas=[],this._blurMaterial=null,this._cubemapMaterial=null,this._equirectMaterial=null,this._compileMaterial(this._blurMaterial)}fromScene(e,n=0,i=.1,r=100){Tu=this._renderer.getRenderTarget(),Au=this._renderer.getActiveCubeFace(),Cu=this._renderer.getActiveMipmapLevel(),bu=this._renderer.xr.enabled,this._renderer.xr.enabled=!1,this._setSize(256);const o=this._allocateTargets();return o.depthBuffer=!0,this._sceneToCubeUV(e,i,r,o),n>0&&this._blur(o,0,0,n),this._applyPMREM(o),this._cleanup(o),o}fromEquirectangular(e,n=null){return this._fromTexture(e,n)}fromCubemap(e,n=null){return this._fromTexture(e,n)}compileCubemapShader(){this._cubemapMaterial===null&&(this._cubemapMaterial=Dm(),this._compileMaterial(this._cubemapMaterial))}compileEquirectangularShader(){this._equirectMaterial===null&&(this._equirectMaterial=Pm(),this._compileMaterial(this._equirectMaterial))}dispose(){this._dispose(),this._cubemapMaterial!==null&&this._cubemapMaterial.dispose(),this._equirectMaterial!==null&&this._equirectMaterial.dispose()}_setSize(e){this._lodMax=Math.floor(Math.log2(e)),this._cubeSize=Math.pow(2,this._lodMax)}_dispose(){this._blurMaterial!==null&&this._blurMaterial.dispose(),this._pingPongRenderTarget!==null&&this._pingPongRenderTarget.dispose();for(let e=0;e<this._lodPlanes.length;e++)this._lodPlanes[e].dispose()}_cleanup(e){this._renderer.setRenderTarget(Tu,Au,Cu),this._renderer.xr.enabled=bu,e.scissorTest=!1,rl(e,0,0,e.width,e.height)}_fromTexture(e,n){e.mapping===Vo||e.mapping===Go?this._setSize(e.image.length===0?16:e.image[0].width||e.image[0].image.width):this._setSize(e.image.width/4),Tu=this._renderer.getRenderTarget(),Au=this._renderer.getActiveCubeFace(),Cu=this._renderer.getActiveMipmapLevel(),bu=this._renderer.xr.enabled,this._renderer.xr.enabled=!1;const i=n||this._allocateTargets();return this._textureToCubeUV(e,i),this._applyPMREM(i),this._cleanup(i),i}_allocateTargets(){const e=3*Math.max(this._cubeSize,112),n=4*this._cubeSize,i={magFilter:Kt,minFilter:Kt,generateMipmaps:!1,type:Zo,format:Wt,colorSpace:Jo,depthBuffer:!1},r=Rm(e,n,i);if(this._pingPongRenderTarget===null||this._pingPongRenderTarget.width!==e||this._pingPongRenderTarget.height!==n){this._pingPongRenderTarget!==null&&this._dispose(),this._pingPongRenderTarget=Rm(e,n,i);const{_lodMax:o}=this;({sizeLods:this._sizeLods,lodPlanes:this._lodPlanes,sigmas:this._sigmas}=CE(o)),this._blurMaterial=bE(o,e,n)}return r}_compileMaterial(e){const n=new Ct(this._lodPlanes[0],e);this._renderer.compile(n,Eu)}_sceneToCubeUV(e,n,i,r){const a=new Cn(90,1,n,i),l=[1,-1,1,1,1,1],c=[1,1,1,-1,-1,-1],d=this._renderer,f=d.autoClear,h=d.toneMapping;d.getClearColor(Cm),d.toneMapping=ir,d.autoClear=!1;const p=new ny({name:"PMREM.Background",side:Jt,depthWrite:!1,depthTest:!1}),v=new Ct(new ts,p);let _=!1;const m=e.background;m?m.isColor&&(p.color.copy(m),e.background=null,_=!0):(p.color.copy(Cm),_=!0);for(let u=0;u<6;u++){const g=u%3;g===0?(a.up.set(0,l[u],0),a.lookAt(c[u],0,0)):g===1?(a.up.set(0,0,l[u]),a.lookAt(0,c[u],0)):(a.up.set(0,l[u],0),a.lookAt(0,0,c[u]));const y=this._cubeSize;rl(r,g*y,u>2?y:0,y,y),d.setRenderTarget(r),_&&d.render(v,a),d.render(e,a)}v.geometry.dispose(),v.material.dispose(),d.toneMapping=h,d.autoClear=f,e.background=m}_textureToCubeUV(e,n){const i=this._renderer,r=e.mapping===Vo||e.mapping===Go;r?(this._cubemapMaterial===null&&(this._cubemapMaterial=Dm()),this._cubemapMaterial.uniforms.flipEnvMap.value=e.isRenderTargetTexture===!1?-1:1):this._equirectMaterial===null&&(this._equirectMaterial=Pm());const o=r?this._cubemapMaterial:this._equirectMaterial,s=new Ct(this._lodPlanes[0],o),a=o.uniforms;a.envMap.value=e;const l=this._cubeSize;rl(n,0,0,3*l,2*l),i.setRenderTarget(n),i.render(s,Eu)}_applyPMREM(e){const n=this._renderer,i=n.autoClear;n.autoClear=!1;const r=this._lodPlanes.length;for(let o=1;o<r;o++){const s=Math.sqrt(this._sigmas[o]*this._sigmas[o]-this._sigmas[o-1]*this._sigmas[o-1]),a=bm[(r-o-1)%bm.length];this._blur(e,o-1,o,s,a)}n.autoClear=i}_blur(e,n,i,r,o){const s=this._pingPongRenderTarget;this._halfBlur(e,s,n,i,r,"latitudinal",o),this._halfBlur(s,e,i,i,r,"longitudinal",o)}_halfBlur(e,n,i,r,o,s,a){const l=this._renderer,c=this._blurMaterial;s!=="latitudinal"&&s!=="longitudinal"&&console.error("blur direction must be either latitudinal or longitudinal!");const d=3,f=new Ct(this._lodPlanes[r],c),h=c.uniforms,p=this._sizeLods[i]-1,v=isFinite(o)?Math.PI/(2*p):2*Math.PI/(2*wr-1),_=o/v,m=isFinite(o)?1+Math.floor(d*_):wr;m>wr&&console.warn(`sigmaRadians, ${o}, is too large and will clip, as it requested ${m} samples when the maximum is set to ${wr}`);const u=[];let g=0;for(let T=0;T<wr;++T){const b=T/_,w=Math.exp(-b*b/2);u.push(w),T===0?g+=w:T<m&&(g+=2*w)}for(let T=0;T<u.length;T++)u[T]=u[T]/g;h.envMap.value=e.texture,h.samples.value=m,h.weights.value=u,h.latitudinal.value=s==="latitudinal",a&&(h.poleAxis.value=a);const{_lodMax:y}=this;h.dTheta.value=v,h.mipInt.value=y-i;const x=this._sizeLods[r],C=3*x*(r>y-So?r-y+So:0),A=4*(this._cubeSize-x);rl(n,C,A,3*x,2*x),l.setRenderTarget(n),l.render(f,Eu)}}function CE(t){const e=[],n=[],i=[];let r=t;const o=t-So+1+Am.length;for(let s=0;s<o;s++){const a=Math.pow(2,r);n.push(a);let l=1/a;s>t-So?l=Am[s-t+So-1]:s===0&&(l=0),i.push(l);const c=1/(a-2),d=-c,f=1+c,h=[d,d,f,d,f,f,d,d,f,f,d,f],p=6,v=6,_=3,m=2,u=1,g=new Float32Array(_*v*p),y=new Float32Array(m*v*p),x=new Float32Array(u*v*p);for(let A=0;A<p;A++){const T=A%3*2/3-1,b=A>2?0:-1,w=[T,b,0,T+2/3,b,0,T+2/3,b+1,0,T,b,0,T+2/3,b+1,0,T,b+1,0];g.set(w,_*v*A),y.set(h,m*v*A);const S=[A,A,A,A,A,A];x.set(S,u*v*A)}const C=new Yt;C.setAttribute("position",new en(g,_)),C.setAttribute("uv",new en(y,m)),C.setAttribute("faceIndex",new en(x,u)),e.push(C),r>So&&r--}return{lodPlanes:e,sizeLods:n,sigmas:i}}function Rm(t,e,n){const i=new qn(t,e,n);return i.texture.mapping=Sc,i.texture.name="PMREM.cubeUv",i.scissorTest=!0,i}function rl(t,e,n,i,r){t.viewport.set(e,n,i,r),t.scissor.set(e,n,i,r)}function bE(t,e,n){const i=new Float32Array(wr),r=new L(0,1,0);return new xn({name:"SphericalGaussianBlur",defines:{n:wr,CUBEUV_TEXEL_WIDTH:1/e,CUBEUV_TEXEL_HEIGHT:1/n,CUBEUV_MAX_MIP:`${t}.0`},uniforms:{envMap:{value:null},samples:{value:1},weights:{value:i},latitudinal:{value:!1},dTheta:{value:0},mipInt:{value:0},poleAxis:{value:r}},vertexShader:Cf(),fragmentShader:`

			precision mediump float;
			precision mediump int;

			varying vec3 vOutputDirection;

			uniform sampler2D envMap;
			uniform int samples;
			uniform float weights[ n ];
			uniform bool latitudinal;
			uniform float dTheta;
			uniform float mipInt;
			uniform vec3 poleAxis;

			#define ENVMAP_TYPE_CUBE_UV
			#include <cube_uv_reflection_fragment>

			vec3 getSample( float theta, vec3 axis ) {

				float cosTheta = cos( theta );
				// Rodrigues' axis-angle rotation
				vec3 sampleDirection = vOutputDirection * cosTheta
					+ cross( axis, vOutputDirection ) * sin( theta )
					+ axis * dot( axis, vOutputDirection ) * ( 1.0 - cosTheta );

				return bilinearCubeUV( envMap, sampleDirection, mipInt );

			}

			void main() {

				vec3 axis = latitudinal ? poleAxis : cross( poleAxis, vOutputDirection );

				if ( all( equal( axis, vec3( 0.0 ) ) ) ) {

					axis = vec3( vOutputDirection.z, 0.0, - vOutputDirection.x );

				}

				axis = normalize( axis );

				gl_FragColor = vec4( 0.0, 0.0, 0.0, 1.0 );
				gl_FragColor.rgb += weights[ 0 ] * getSample( 0.0, axis );

				for ( int i = 1; i < n; i++ ) {

					if ( i >= samples ) {

						break;

					}

					float theta = dTheta * float( i );
					gl_FragColor.rgb += weights[ i ] * getSample( -1.0 * theta, axis );
					gl_FragColor.rgb += weights[ i ] * getSample( theta, axis );

				}

			}
		`,blending:nr,depthTest:!1,depthWrite:!1})}function Pm(){return new xn({name:"EquirectangularToCubeUV",uniforms:{envMap:{value:null}},vertexShader:Cf(),fragmentShader:`

			precision mediump float;
			precision mediump int;

			varying vec3 vOutputDirection;

			uniform sampler2D envMap;

			#include <common>

			void main() {

				vec3 outputDirection = normalize( vOutputDirection );
				vec2 uv = equirectUv( outputDirection );

				gl_FragColor = vec4( texture2D ( envMap, uv ).rgb, 1.0 );

			}
		`,blending:nr,depthTest:!1,depthWrite:!1})}function Dm(){return new xn({name:"CubemapToCubeUV",uniforms:{envMap:{value:null},flipEnvMap:{value:-1}},vertexShader:Cf(),fragmentShader:`

			precision mediump float;
			precision mediump int;

			uniform float flipEnvMap;

			varying vec3 vOutputDirection;

			uniform samplerCube envMap;

			void main() {

				gl_FragColor = textureCube( envMap, vec3( flipEnvMap * vOutputDirection.x, vOutputDirection.yz ) );

			}
		`,blending:nr,depthTest:!1,depthWrite:!1})}function Cf(){return`

		precision mediump float;
		precision mediump int;

		attribute float faceIndex;

		varying vec3 vOutputDirection;

		// RH coordinate system; PMREM face-indexing convention
		vec3 getDirection( vec2 uv, float face ) {

			uv = 2.0 * uv - 1.0;

			vec3 direction = vec3( uv, 1.0 );

			if ( face == 0.0 ) {

				direction = direction.zyx; // ( 1, v, u ) pos x

			} else if ( face == 1.0 ) {

				direction = direction.xzy;
				direction.xz *= -1.0; // ( -u, 1, -v ) pos y

			} else if ( face == 2.0 ) {

				direction.x *= -1.0; // ( -u, v, 1 ) pos z

			} else if ( face == 3.0 ) {

				direction = direction.zyx;
				direction.xz *= -1.0; // ( -1, v, -u ) neg x

			} else if ( face == 4.0 ) {

				direction = direction.xzy;
				direction.xy *= -1.0; // ( -u, -1, v ) neg y

			} else if ( face == 5.0 ) {

				direction.z *= -1.0; // ( u, v, -1 ) neg z

			}

			return direction;

		}

		void main() {

			vOutputDirection = getDirection( uv, faceIndex );
			gl_Position = vec4( position, 1.0 );

		}
	`}function RE(t){let e=new WeakMap,n=null;function i(a){if(a&&a.isTexture){const l=a.mapping,c=l===Vd||l===Gd,d=l===Vo||l===Go;if(c||d){let f=e.get(a);const h=f!==void 0?f.texture.pmremVersion:0;if(a.isRenderTargetTexture&&a.pmremVersion!==h)return n===null&&(n=new yh(t)),f=c?n.fromEquirectangular(a,f):n.fromCubemap(a,f),f.texture.pmremVersion=a.pmremVersion,e.set(a,f),f.texture;if(f!==void 0)return f.texture;{const p=a.image;return c&&p&&p.height>0||d&&p&&r(p)?(n===null&&(n=new yh(t)),f=c?n.fromEquirectangular(a):n.fromCubemap(a),f.texture.pmremVersion=a.pmremVersion,e.set(a,f),a.addEventListener("dispose",o),f.texture):null}}}return a}function r(a){let l=0;const c=6;for(let d=0;d<c;d++)a[d]!==void 0&&l++;return l===c}function o(a){const l=a.target;l.removeEventListener("dispose",o);const c=e.get(l);c!==void 0&&(e.delete(l),c.dispose())}function s(){e=new WeakMap,n!==null&&(n.dispose(),n=null)}return{get:i,dispose:s}}function PE(t){const e={};function n(i){if(e[i]!==void 0)return e[i];let r;switch(i){case"WEBGL_depth_texture":r=t.getExtension("WEBGL_depth_texture")||t.getExtension("MOZ_WEBGL_depth_texture")||t.getExtension("WEBKIT_WEBGL_depth_texture");break;case"EXT_texture_filter_anisotropic":r=t.getExtension("EXT_texture_filter_anisotropic")||t.getExtension("MOZ_EXT_texture_filter_anisotropic")||t.getExtension("WEBKIT_EXT_texture_filter_anisotropic");break;case"WEBGL_compressed_texture_s3tc":r=t.getExtension("WEBGL_compressed_texture_s3tc")||t.getExtension("MOZ_WEBGL_compressed_texture_s3tc")||t.getExtension("WEBKIT_WEBGL_compressed_texture_s3tc");break;case"WEBGL_compressed_texture_pvrtc":r=t.getExtension("WEBGL_compressed_texture_pvrtc")||t.getExtension("WEBKIT_WEBGL_compressed_texture_pvrtc");break;default:r=t.getExtension(i)}return e[i]=r,r}return{has:function(i){return n(i)!==null},init:function(){n("EXT_color_buffer_float"),n("WEBGL_clip_cull_distance"),n("OES_texture_float_linear"),n("EXT_color_buffer_half_float"),n("WEBGL_multisampled_render_to_texture"),n("WEBGL_render_shared_exponent")},get:function(i){const r=n(i);return r===null&&Rs("THREE.WebGLRenderer: "+i+" extension not supported."),r}}}function DE(t,e,n,i){const r={},o=new WeakMap;function s(f){const h=f.target;h.index!==null&&e.remove(h.index);for(const v in h.attributes)e.remove(h.attributes[v]);for(const v in h.morphAttributes){const _=h.morphAttributes[v];for(let m=0,u=_.length;m<u;m++)e.remove(_[m])}h.removeEventListener("dispose",s),delete r[h.id];const p=o.get(h);p&&(e.remove(p),o.delete(h)),i.releaseStatesOfGeometry(h),h.isInstancedBufferGeometry===!0&&delete h._maxInstanceCount,n.memory.geometries--}function a(f,h){return r[h.id]===!0||(h.addEventListener("dispose",s),r[h.id]=!0,n.memory.geometries++),h}function l(f){const h=f.attributes;for(const v in h)e.update(h[v],t.ARRAY_BUFFER);const p=f.morphAttributes;for(const v in p){const _=p[v];for(let m=0,u=_.length;m<u;m++)e.update(_[m],t.ARRAY_BUFFER)}}function c(f){const h=[],p=f.index,v=f.attributes.position;let _=0;if(p!==null){const g=p.array;_=p.version;for(let y=0,x=g.length;y<x;y+=3){const C=g[y+0],A=g[y+1],T=g[y+2];h.push(C,A,A,T,T,C)}}else if(v!==void 0){const g=v.array;_=v.version;for(let y=0,x=g.length/3-1;y<x;y+=3){const C=y+0,A=y+1,T=y+2;h.push(C,A,A,T,T,C)}}else return;const m=new(Zv(h)?ry:iy)(h,1);m.version=_;const u=o.get(f);u&&e.remove(u),o.set(f,m)}function d(f){const h=o.get(f);if(h){const p=f.index;p!==null&&h.version<p.version&&c(f)}else c(f);return o.get(f)}return{get:a,update:l,getWireframeAttribute:d}}function LE(t,e,n){let i;function r(h){i=h}let o,s;function a(h){o=h.type,s=h.bytesPerElement}function l(h,p){t.drawElements(i,p,o,h*s),n.update(p,i,1)}function c(h,p,v){v!==0&&(t.drawElementsInstanced(i,p,o,h*s,v),n.update(p,i,v))}function d(h,p,v){if(v===0)return;e.get("WEBGL_multi_draw").multiDrawElementsWEBGL(i,p,0,o,h,0,v);let m=0;for(let u=0;u<v;u++)m+=p[u];n.update(m,i,1)}function f(h,p,v,_){if(v===0)return;const m=e.get("WEBGL_multi_draw");if(m===null)for(let u=0;u<h.length;u++)c(h[u]/s,p[u],_[u]);else{m.multiDrawElementsInstancedWEBGL(i,p,0,o,h,0,_,0,v);let u=0;for(let g=0;g<v;g++)u+=p[g]*_[g];n.update(u,i,1)}}this.setMode=r,this.setIndex=a,this.render=l,this.renderInstances=c,this.renderMultiDraw=d,this.renderMultiDrawInstances=f}function NE(t){const e={geometries:0,textures:0},n={frame:0,calls:0,triangles:0,points:0,lines:0};function i(o,s,a){switch(n.calls++,s){case t.TRIANGLES:n.triangles+=a*(o/3);break;case t.LINES:n.lines+=a*(o/2);break;case t.LINE_STRIP:n.lines+=a*(o-1);break;case t.LINE_LOOP:n.lines+=a*o;break;case t.POINTS:n.points+=a*o;break;default:console.error("THREE.WebGLInfo: Unknown draw mode:",s);break}}function r(){n.calls=0,n.triangles=0,n.points=0,n.lines=0}return{memory:e,render:n,programs:null,autoReset:!0,reset:r,update:i}}function IE(t,e,n){const i=new WeakMap,r=new St;function o(s,a,l){const c=s.morphTargetInfluences,d=a.morphAttributes.position||a.morphAttributes.normal||a.morphAttributes.color,f=d!==void 0?d.length:0;let h=i.get(a);if(h===void 0||h.count!==f){let S=function(){b.dispose(),i.delete(a),a.removeEventListener("dispose",S)};var p=S;h!==void 0&&h.texture.dispose();const v=a.morphAttributes.position!==void 0,_=a.morphAttributes.normal!==void 0,m=a.morphAttributes.color!==void 0,u=a.morphAttributes.position||[],g=a.morphAttributes.normal||[],y=a.morphAttributes.color||[];let x=0;v===!0&&(x=1),_===!0&&(x=2),m===!0&&(x=3);let C=a.attributes.position.count*x,A=1;C>e.maxTextureSize&&(A=Math.ceil(C/e.maxTextureSize),C=e.maxTextureSize);const T=new Float32Array(C*A*4*f),b=new Qv(T,C,A,f);b.type=Pn,b.needsUpdate=!0;const w=x*4;for(let P=0;P<f;P++){const z=u[P],F=g[P],Y=y[P],j=C*A*4*P;for(let W=0;W<z.count;W++){const $=W*w;v===!0&&(r.fromBufferAttribute(z,W),T[j+$+0]=r.x,T[j+$+1]=r.y,T[j+$+2]=r.z,T[j+$+3]=0),_===!0&&(r.fromBufferAttribute(F,W),T[j+$+4]=r.x,T[j+$+5]=r.y,T[j+$+6]=r.z,T[j+$+7]=0),m===!0&&(r.fromBufferAttribute(Y,W),T[j+$+8]=r.x,T[j+$+9]=r.y,T[j+$+10]=r.z,T[j+$+11]=Y.itemSize===4?r.w:1)}}h={count:f,texture:b,size:new Re(C,A)},i.set(a,h),a.addEventListener("dispose",S)}if(s.isInstancedMesh===!0&&s.morphTexture!==null)l.getUniforms().setValue(t,"morphTexture",s.morphTexture,n);else{let v=0;for(let m=0;m<c.length;m++)v+=c[m];const _=a.morphTargetsRelative?1:1-v;l.getUniforms().setValue(t,"morphTargetBaseInfluence",_),l.getUniforms().setValue(t,"morphTargetInfluences",c)}l.getUniforms().setValue(t,"morphTargetsTexture",h.texture,n),l.getUniforms().setValue(t,"morphTargetsTextureSize",h.size)}return{update:o}}function kE(t,e,n,i){let r=new WeakMap;function o(l){const c=i.render.frame,d=l.geometry,f=e.get(l,d);if(r.get(f)!==c&&(e.update(f),r.set(f,c)),l.isInstancedMesh&&(l.hasEventListener("dispose",a)===!1&&l.addEventListener("dispose",a),r.get(l)!==c&&(n.update(l.instanceMatrix,t.ARRAY_BUFFER),l.instanceColor!==null&&n.update(l.instanceColor,t.ARRAY_BUFFER),r.set(l,c))),l.isSkinnedMesh){const h=l.skeleton;r.get(h)!==c&&(h.update(),r.set(h,c))}return f}function s(){r=new WeakMap}function a(l){const c=l.target;c.removeEventListener("dispose",a),n.remove(c.instanceMatrix),c.instanceColor!==null&&n.remove(c.instanceColor)}return{update:o,dispose:s}}class cy extends Qt{constructor(e,n,i,r,o,s,a,l,c,d=Lo){if(d!==Lo&&d!==jo)throw new Error("DepthTexture format must be either THREE.DepthFormat or THREE.DepthStencilFormat");i===void 0&&d===Lo&&(i=Fr),i===void 0&&d===jo&&(i=Xo),super(null,r,o,s,a,l,d,i,c),this.isDepthTexture=!0,this.image={width:e,height:n},this.magFilter=a!==void 0?a:dn,this.minFilter=l!==void 0?l:dn,this.flipY=!1,this.generateMipmaps=!1,this.compareFunction=null}copy(e){return super.copy(e),this.compareFunction=e.compareFunction,this}toJSON(e){const n=super.toJSON(e);return this.compareFunction!==null&&(n.compareFunction=this.compareFunction),n}}const uy=new Qt,Lm=new cy(1,1),dy=new Qv,hy=new xM,fy=new ay,Nm=[],Im=[],km=new Float32Array(16),Um=new Float32Array(9),Fm=new Float32Array(4);function ns(t,e,n){const i=t[0];if(i<=0||i>0)return t;const r=e*n;let o=Nm[r];if(o===void 0&&(o=new Float32Array(r),Nm[r]=o),e!==0){i.toArray(o,0);for(let s=1,a=0;s!==e;++s)a+=n,t[s].toArray(o,a)}return o}function bt(t,e){if(t.length!==e.length)return!1;for(let n=0,i=t.length;n<i;n++)if(t[n]!==e[n])return!1;return!0}function Rt(t,e){for(let n=0,i=e.length;n<i;n++)t[n]=e[n]}function wc(t,e){let n=Im[e];n===void 0&&(n=new Int32Array(e),Im[e]=n);for(let i=0;i!==e;++i)n[i]=t.allocateTextureUnit();return n}function UE(t,e){const n=this.cache;n[0]!==e&&(t.uniform1f(this.addr,e),n[0]=e)}function FE(t,e){const n=this.cache;if(e.x!==void 0)(n[0]!==e.x||n[1]!==e.y)&&(t.uniform2f(this.addr,e.x,e.y),n[0]=e.x,n[1]=e.y);else{if(bt(n,e))return;t.uniform2fv(this.addr,e),Rt(n,e)}}function OE(t,e){const n=this.cache;if(e.x!==void 0)(n[0]!==e.x||n[1]!==e.y||n[2]!==e.z)&&(t.uniform3f(this.addr,e.x,e.y,e.z),n[0]=e.x,n[1]=e.y,n[2]=e.z);else if(e.r!==void 0)(n[0]!==e.r||n[1]!==e.g||n[2]!==e.b)&&(t.uniform3f(this.addr,e.r,e.g,e.b),n[0]=e.r,n[1]=e.g,n[2]=e.b);else{if(bt(n,e))return;t.uniform3fv(this.addr,e),Rt(n,e)}}function zE(t,e){const n=this.cache;if(e.x!==void 0)(n[0]!==e.x||n[1]!==e.y||n[2]!==e.z||n[3]!==e.w)&&(t.uniform4f(this.addr,e.x,e.y,e.z,e.w),n[0]=e.x,n[1]=e.y,n[2]=e.z,n[3]=e.w);else{if(bt(n,e))return;t.uniform4fv(this.addr,e),Rt(n,e)}}function BE(t,e){const n=this.cache,i=e.elements;if(i===void 0){if(bt(n,e))return;t.uniformMatrix2fv(this.addr,!1,e),Rt(n,e)}else{if(bt(n,i))return;Fm.set(i),t.uniformMatrix2fv(this.addr,!1,Fm),Rt(n,i)}}function HE(t,e){const n=this.cache,i=e.elements;if(i===void 0){if(bt(n,e))return;t.uniformMatrix3fv(this.addr,!1,e),Rt(n,e)}else{if(bt(n,i))return;Um.set(i),t.uniformMatrix3fv(this.addr,!1,Um),Rt(n,i)}}function VE(t,e){const n=this.cache,i=e.elements;if(i===void 0){if(bt(n,e))return;t.uniformMatrix4fv(this.addr,!1,e),Rt(n,e)}else{if(bt(n,i))return;km.set(i),t.uniformMatrix4fv(this.addr,!1,km),Rt(n,i)}}function GE(t,e){const n=this.cache;n[0]!==e&&(t.uniform1i(this.addr,e),n[0]=e)}function WE(t,e){const n=this.cache;if(e.x!==void 0)(n[0]!==e.x||n[1]!==e.y)&&(t.uniform2i(this.addr,e.x,e.y),n[0]=e.x,n[1]=e.y);else{if(bt(n,e))return;t.uniform2iv(this.addr,e),Rt(n,e)}}function XE(t,e){const n=this.cache;if(e.x!==void 0)(n[0]!==e.x||n[1]!==e.y||n[2]!==e.z)&&(t.uniform3i(this.addr,e.x,e.y,e.z),n[0]=e.x,n[1]=e.y,n[2]=e.z);else{if(bt(n,e))return;t.uniform3iv(this.addr,e),Rt(n,e)}}function jE(t,e){const n=this.cache;if(e.x!==void 0)(n[0]!==e.x||n[1]!==e.y||n[2]!==e.z||n[3]!==e.w)&&(t.uniform4i(this.addr,e.x,e.y,e.z,e.w),n[0]=e.x,n[1]=e.y,n[2]=e.z,n[3]=e.w);else{if(bt(n,e))return;t.uniform4iv(this.addr,e),Rt(n,e)}}function YE(t,e){const n=this.cache;n[0]!==e&&(t.uniform1ui(this.addr,e),n[0]=e)}function qE(t,e){const n=this.cache;if(e.x!==void 0)(n[0]!==e.x||n[1]!==e.y)&&(t.uniform2ui(this.addr,e.x,e.y),n[0]=e.x,n[1]=e.y);else{if(bt(n,e))return;t.uniform2uiv(this.addr,e),Rt(n,e)}}function $E(t,e){const n=this.cache;if(e.x!==void 0)(n[0]!==e.x||n[1]!==e.y||n[2]!==e.z)&&(t.uniform3ui(this.addr,e.x,e.y,e.z),n[0]=e.x,n[1]=e.y,n[2]=e.z);else{if(bt(n,e))return;t.uniform3uiv(this.addr,e),Rt(n,e)}}function KE(t,e){const n=this.cache;if(e.x!==void 0)(n[0]!==e.x||n[1]!==e.y||n[2]!==e.z||n[3]!==e.w)&&(t.uniform4ui(this.addr,e.x,e.y,e.z,e.w),n[0]=e.x,n[1]=e.y,n[2]=e.z,n[3]=e.w);else{if(bt(n,e))return;t.uniform4uiv(this.addr,e),Rt(n,e)}}function ZE(t,e,n){const i=this.cache,r=n.allocateTextureUnit();i[0]!==r&&(t.uniform1i(this.addr,r),i[0]=r);let o;this.type===t.SAMPLER_2D_SHADOW?(Lm.compareFunction=Kv,o=Lm):o=uy,n.setTexture2D(e||o,r)}function JE(t,e,n){const i=this.cache,r=n.allocateTextureUnit();i[0]!==r&&(t.uniform1i(this.addr,r),i[0]=r),n.setTexture3D(e||hy,r)}function QE(t,e,n){const i=this.cache,r=n.allocateTextureUnit();i[0]!==r&&(t.uniform1i(this.addr,r),i[0]=r),n.setTextureCube(e||fy,r)}function eT(t,e,n){const i=this.cache,r=n.allocateTextureUnit();i[0]!==r&&(t.uniform1i(this.addr,r),i[0]=r),n.setTexture2DArray(e||dy,r)}function tT(t){switch(t){case 5126:return UE;case 35664:return FE;case 35665:return OE;case 35666:return zE;case 35674:return BE;case 35675:return HE;case 35676:return VE;case 5124:case 35670:return GE;case 35667:case 35671:return WE;case 35668:case 35672:return XE;case 35669:case 35673:return jE;case 5125:return YE;case 36294:return qE;case 36295:return $E;case 36296:return KE;case 35678:case 36198:case 36298:case 36306:case 35682:return ZE;case 35679:case 36299:case 36307:return JE;case 35680:case 36300:case 36308:case 36293:return QE;case 36289:case 36303:case 36311:case 36292:return eT}}function nT(t,e){t.uniform1fv(this.addr,e)}function iT(t,e){const n=ns(e,this.size,2);t.uniform2fv(this.addr,n)}function rT(t,e){const n=ns(e,this.size,3);t.uniform3fv(this.addr,n)}function oT(t,e){const n=ns(e,this.size,4);t.uniform4fv(this.addr,n)}function sT(t,e){const n=ns(e,this.size,4);t.uniformMatrix2fv(this.addr,!1,n)}function aT(t,e){const n=ns(e,this.size,9);t.uniformMatrix3fv(this.addr,!1,n)}function lT(t,e){const n=ns(e,this.size,16);t.uniformMatrix4fv(this.addr,!1,n)}function cT(t,e){t.uniform1iv(this.addr,e)}function uT(t,e){t.uniform2iv(this.addr,e)}function dT(t,e){t.uniform3iv(this.addr,e)}function hT(t,e){t.uniform4iv(this.addr,e)}function fT(t,e){t.uniform1uiv(this.addr,e)}function pT(t,e){t.uniform2uiv(this.addr,e)}function mT(t,e){t.uniform3uiv(this.addr,e)}function gT(t,e){t.uniform4uiv(this.addr,e)}function vT(t,e,n){const i=this.cache,r=e.length,o=wc(n,r);bt(i,o)||(t.uniform1iv(this.addr,o),Rt(i,o));for(let s=0;s!==r;++s)n.setTexture2D(e[s]||uy,o[s])}function yT(t,e,n){const i=this.cache,r=e.length,o=wc(n,r);bt(i,o)||(t.uniform1iv(this.addr,o),Rt(i,o));for(let s=0;s!==r;++s)n.setTexture3D(e[s]||hy,o[s])}function _T(t,e,n){const i=this.cache,r=e.length,o=wc(n,r);bt(i,o)||(t.uniform1iv(this.addr,o),Rt(i,o));for(let s=0;s!==r;++s)n.setTextureCube(e[s]||fy,o[s])}function xT(t,e,n){const i=this.cache,r=e.length,o=wc(n,r);bt(i,o)||(t.uniform1iv(this.addr,o),Rt(i,o));for(let s=0;s!==r;++s)n.setTexture2DArray(e[s]||dy,o[s])}function ST(t){switch(t){case 5126:return nT;case 35664:return iT;case 35665:return rT;case 35666:return oT;case 35674:return sT;case 35675:return aT;case 35676:return lT;case 5124:case 35670:return cT;case 35667:case 35671:return uT;case 35668:case 35672:return dT;case 35669:case 35673:return hT;case 5125:return fT;case 36294:return pT;case 36295:return mT;case 36296:return gT;case 35678:case 36198:case 36298:case 36306:case 35682:return vT;case 35679:case 36299:case 36307:return yT;case 35680:case 36300:case 36308:case 36293:return _T;case 36289:case 36303:case 36311:case 36292:return xT}}class MT{constructor(e,n,i){this.id=e,this.addr=i,this.cache=[],this.type=n.type,this.setValue=tT(n.type)}}class wT{constructor(e,n,i){this.id=e,this.addr=i,this.cache=[],this.type=n.type,this.size=n.size,this.setValue=ST(n.type)}}class ET{constructor(e){this.id=e,this.seq=[],this.map={}}setValue(e,n,i){const r=this.seq;for(let o=0,s=r.length;o!==s;++o){const a=r[o];a.setValue(e,n[a.id],i)}}}const Ru=/(\w+)(\])?(\[|\.)?/g;function Om(t,e){t.seq.push(e),t.map[e.id]=e}function TT(t,e,n){const i=t.name,r=i.length;for(Ru.lastIndex=0;;){const o=Ru.exec(i),s=Ru.lastIndex;let a=o[1];const l=o[2]==="]",c=o[3];if(l&&(a=a|0),c===void 0||c==="["&&s+2===r){Om(n,c===void 0?new MT(a,t,e):new wT(a,t,e));break}else{let f=n.map[a];f===void 0&&(f=new ET(a),Om(n,f)),n=f}}}class Al{constructor(e,n){this.seq=[],this.map={};const i=e.getProgramParameter(n,e.ACTIVE_UNIFORMS);for(let r=0;r<i;++r){const o=e.getActiveUniform(n,r),s=e.getUniformLocation(n,o.name);TT(o,s,this)}}setValue(e,n,i,r){const o=this.map[n];o!==void 0&&o.setValue(e,i,r)}setOptional(e,n,i){const r=n[i];r!==void 0&&this.setValue(e,i,r)}static upload(e,n,i,r){for(let o=0,s=n.length;o!==s;++o){const a=n[o],l=i[a.id];l.needsUpdate!==!1&&a.setValue(e,l.value,r)}}static seqWithValue(e,n){const i=[];for(let r=0,o=e.length;r!==o;++r){const s=e[r];s.id in n&&i.push(s)}return i}}function zm(t,e,n){const i=t.createShader(e);return t.shaderSource(i,n),t.compileShader(i),i}const AT=37297;let CT=0;function bT(t,e){const n=t.split(`
`),i=[],r=Math.max(e-6,0),o=Math.min(e+6,n.length);for(let s=r;s<o;s++){const a=s+1;i.push(`${a===e?">":" "} ${a}: ${n[s]}`)}return i.join(`
`)}const Bm=new Be;function RT(t){$e._getMatrix(Bm,$e.workingColorSpace,t);const e=`mat3( ${Bm.elements.map(n=>n.toFixed(4))} )`;switch($e.getTransfer(t)){case Mc:return[e,"LinearTransferOETF"];case it:return[e,"sRGBTransferOETF"];default:return console.warn("THREE.WebGLProgram: Unsupported color space: ",t),[e,"LinearTransferOETF"]}}function Hm(t,e,n){const i=t.getShaderParameter(e,t.COMPILE_STATUS),r=t.getShaderInfoLog(e).trim();if(i&&r==="")return"";const o=/ERROR: 0:(\d+)/.exec(r);if(o){const s=parseInt(o[1]);return n.toUpperCase()+`

`+r+`

`+bT(t.getShaderSource(e),s)}else return r}function PT(t,e){const n=RT(e);return[`vec4 ${t}( vec4 value ) {`,`	return ${n[1]}( vec4( value.rgb * ${n[0]}, value.a ) );`,"}"].join(`
`)}function DT(t,e){let n;switch(e){case YS:n="Linear";break;case qS:n="Reinhard";break;case $S:n="Cineon";break;case Ov:n="ACESFilmic";break;case ZS:n="AgX";break;case JS:n="Neutral";break;case KS:n="Custom";break;default:console.warn("THREE.WebGLProgram: Unsupported toneMapping:",e),n="Linear"}return"vec3 "+t+"( vec3 color ) { return "+n+"ToneMapping( color ); }"}const ol=new L;function LT(){$e.getLuminanceCoefficients(ol);const t=ol.x.toFixed(4),e=ol.y.toFixed(4),n=ol.z.toFixed(4);return["float luminance( const in vec3 rgb ) {",`	const vec3 weights = vec3( ${t}, ${e}, ${n} );`,"	return dot( weights, rgb );","}"].join(`
`)}function NT(t){return[t.extensionClipCullDistance?"#extension GL_ANGLE_clip_cull_distance : require":"",t.extensionMultiDraw?"#extension GL_ANGLE_multi_draw : require":""].filter(Ps).join(`
`)}function IT(t){const e=[];for(const n in t){const i=t[n];i!==!1&&e.push("#define "+n+" "+i)}return e.join(`
`)}function kT(t,e){const n={},i=t.getProgramParameter(e,t.ACTIVE_ATTRIBUTES);for(let r=0;r<i;r++){const o=t.getActiveAttrib(e,r),s=o.name;let a=1;o.type===t.FLOAT_MAT2&&(a=2),o.type===t.FLOAT_MAT3&&(a=3),o.type===t.FLOAT_MAT4&&(a=4),n[s]={type:o.type,location:t.getAttribLocation(e,s),locationSize:a}}return n}function Ps(t){return t!==""}function Vm(t,e){const n=e.numSpotLightShadows+e.numSpotLightMaps-e.numSpotLightShadowsWithMaps;return t.replace(/NUM_DIR_LIGHTS/g,e.numDirLights).replace(/NUM_SPOT_LIGHTS/g,e.numSpotLights).replace(/NUM_SPOT_LIGHT_MAPS/g,e.numSpotLightMaps).replace(/NUM_SPOT_LIGHT_COORDS/g,n).replace(/NUM_RECT_AREA_LIGHTS/g,e.numRectAreaLights).replace(/NUM_POINT_LIGHTS/g,e.numPointLights).replace(/NUM_HEMI_LIGHTS/g,e.numHemiLights).replace(/NUM_DIR_LIGHT_SHADOWS/g,e.numDirLightShadows).replace(/NUM_SPOT_LIGHT_SHADOWS_WITH_MAPS/g,e.numSpotLightShadowsWithMaps).replace(/NUM_SPOT_LIGHT_SHADOWS/g,e.numSpotLightShadows).replace(/NUM_POINT_LIGHT_SHADOWS/g,e.numPointLightShadows)}function Gm(t,e){return t.replace(/NUM_CLIPPING_PLANES/g,e.numClippingPlanes).replace(/UNION_CLIPPING_PLANES/g,e.numClippingPlanes-e.numClipIntersection)}const UT=/^[ \t]*#include +<([\w\d./]+)>/gm;function _h(t){return t.replace(UT,OT)}const FT=new Map;function OT(t,e){let n=Ve[e];if(n===void 0){const i=FT.get(e);if(i!==void 0)n=Ve[i],console.warn('THREE.WebGLRenderer: Shader chunk "%s" has been deprecated. Use "%s" instead.',e,i);else throw new Error("Can not resolve #include <"+e+">")}return _h(n)}const zT=/#pragma unroll_loop_start\s+for\s*\(\s*int\s+i\s*=\s*(\d+)\s*;\s*i\s*<\s*(\d+)\s*;\s*i\s*\+\+\s*\)\s*{([\s\S]+?)}\s+#pragma unroll_loop_end/g;function Wm(t){return t.replace(zT,BT)}function BT(t,e,n,i){let r="";for(let o=parseInt(e);o<parseInt(n);o++)r+=i.replace(/\[\s*i\s*\]/g,"[ "+o+" ]").replace(/UNROLLED_LOOP_INDEX/g,o);return r}function Xm(t){let e=`precision ${t.precision} float;
	precision ${t.precision} int;
	precision ${t.precision} sampler2D;
	precision ${t.precision} samplerCube;
	precision ${t.precision} sampler3D;
	precision ${t.precision} sampler2DArray;
	precision ${t.precision} sampler2DShadow;
	precision ${t.precision} samplerCubeShadow;
	precision ${t.precision} sampler2DArrayShadow;
	precision ${t.precision} isampler2D;
	precision ${t.precision} isampler3D;
	precision ${t.precision} isamplerCube;
	precision ${t.precision} isampler2DArray;
	precision ${t.precision} usampler2D;
	precision ${t.precision} usampler3D;
	precision ${t.precision} usamplerCube;
	precision ${t.precision} usampler2DArray;
	`;return t.precision==="highp"?e+=`
#define HIGH_PRECISION`:t.precision==="mediump"?e+=`
#define MEDIUM_PRECISION`:t.precision==="lowp"&&(e+=`
#define LOW_PRECISION`),e}function HT(t){let e="SHADOWMAP_TYPE_BASIC";return t.shadowMapType===Iv?e="SHADOWMAP_TYPE_PCF":t.shadowMapType===RS?e="SHADOWMAP_TYPE_PCF_SOFT":t.shadowMapType===fi&&(e="SHADOWMAP_TYPE_VSM"),e}function VT(t){let e="ENVMAP_TYPE_CUBE";if(t.envMap)switch(t.envMapMode){case Vo:case Go:e="ENVMAP_TYPE_CUBE";break;case Sc:e="ENVMAP_TYPE_CUBE_UV";break}return e}function GT(t){let e="ENVMAP_MODE_REFLECTION";if(t.envMap)switch(t.envMapMode){case Go:e="ENVMAP_MODE_REFRACTION";break}return e}function WT(t){let e="ENVMAP_BLENDING_NONE";if(t.envMap)switch(t.combine){case Fv:e="ENVMAP_BLENDING_MULTIPLY";break;case XS:e="ENVMAP_BLENDING_MIX";break;case jS:e="ENVMAP_BLENDING_ADD";break}return e}function XT(t){const e=t.envMapCubeUVHeight;if(e===null)return null;const n=Math.log2(e)-2,i=1/e;return{texelWidth:1/(3*Math.max(Math.pow(2,n),7*16)),texelHeight:i,maxMip:n}}function jT(t,e,n,i){const r=t.getContext(),o=n.defines;let s=n.vertexShader,a=n.fragmentShader;const l=HT(n),c=VT(n),d=GT(n),f=WT(n),h=XT(n),p=NT(n),v=IT(o),_=r.createProgram();let m,u,g=n.glslVersion?"#version "+n.glslVersion+`
`:"";n.isRawShaderMaterial?(m=["#define SHADER_TYPE "+n.shaderType,"#define SHADER_NAME "+n.shaderName,v].filter(Ps).join(`
`),m.length>0&&(m+=`
`),u=["#define SHADER_TYPE "+n.shaderType,"#define SHADER_NAME "+n.shaderName,v].filter(Ps).join(`
`),u.length>0&&(u+=`
`)):(m=[Xm(n),"#define SHADER_TYPE "+n.shaderType,"#define SHADER_NAME "+n.shaderName,v,n.extensionClipCullDistance?"#define USE_CLIP_DISTANCE":"",n.batching?"#define USE_BATCHING":"",n.batchingColor?"#define USE_BATCHING_COLOR":"",n.instancing?"#define USE_INSTANCING":"",n.instancingColor?"#define USE_INSTANCING_COLOR":"",n.instancingMorph?"#define USE_INSTANCING_MORPH":"",n.useFog&&n.fog?"#define USE_FOG":"",n.useFog&&n.fogExp2?"#define FOG_EXP2":"",n.map?"#define USE_MAP":"",n.envMap?"#define USE_ENVMAP":"",n.envMap?"#define "+d:"",n.lightMap?"#define USE_LIGHTMAP":"",n.aoMap?"#define USE_AOMAP":"",n.bumpMap?"#define USE_BUMPMAP":"",n.normalMap?"#define USE_NORMALMAP":"",n.normalMapObjectSpace?"#define USE_NORMALMAP_OBJECTSPACE":"",n.normalMapTangentSpace?"#define USE_NORMALMAP_TANGENTSPACE":"",n.displacementMap?"#define USE_DISPLACEMENTMAP":"",n.emissiveMap?"#define USE_EMISSIVEMAP":"",n.anisotropy?"#define USE_ANISOTROPY":"",n.anisotropyMap?"#define USE_ANISOTROPYMAP":"",n.clearcoatMap?"#define USE_CLEARCOATMAP":"",n.clearcoatRoughnessMap?"#define USE_CLEARCOAT_ROUGHNESSMAP":"",n.clearcoatNormalMap?"#define USE_CLEARCOAT_NORMALMAP":"",n.iridescenceMap?"#define USE_IRIDESCENCEMAP":"",n.iridescenceThicknessMap?"#define USE_IRIDESCENCE_THICKNESSMAP":"",n.specularMap?"#define USE_SPECULARMAP":"",n.specularColorMap?"#define USE_SPECULAR_COLORMAP":"",n.specularIntensityMap?"#define USE_SPECULAR_INTENSITYMAP":"",n.roughnessMap?"#define USE_ROUGHNESSMAP":"",n.metalnessMap?"#define USE_METALNESSMAP":"",n.alphaMap?"#define USE_ALPHAMAP":"",n.alphaHash?"#define USE_ALPHAHASH":"",n.transmission?"#define USE_TRANSMISSION":"",n.transmissionMap?"#define USE_TRANSMISSIONMAP":"",n.thicknessMap?"#define USE_THICKNESSMAP":"",n.sheenColorMap?"#define USE_SHEEN_COLORMAP":"",n.sheenRoughnessMap?"#define USE_SHEEN_ROUGHNESSMAP":"",n.mapUv?"#define MAP_UV "+n.mapUv:"",n.alphaMapUv?"#define ALPHAMAP_UV "+n.alphaMapUv:"",n.lightMapUv?"#define LIGHTMAP_UV "+n.lightMapUv:"",n.aoMapUv?"#define AOMAP_UV "+n.aoMapUv:"",n.emissiveMapUv?"#define EMISSIVEMAP_UV "+n.emissiveMapUv:"",n.bumpMapUv?"#define BUMPMAP_UV "+n.bumpMapUv:"",n.normalMapUv?"#define NORMALMAP_UV "+n.normalMapUv:"",n.displacementMapUv?"#define DISPLACEMENTMAP_UV "+n.displacementMapUv:"",n.metalnessMapUv?"#define METALNESSMAP_UV "+n.metalnessMapUv:"",n.roughnessMapUv?"#define ROUGHNESSMAP_UV "+n.roughnessMapUv:"",n.anisotropyMapUv?"#define ANISOTROPYMAP_UV "+n.anisotropyMapUv:"",n.clearcoatMapUv?"#define CLEARCOATMAP_UV "+n.clearcoatMapUv:"",n.clearcoatNormalMapUv?"#define CLEARCOAT_NORMALMAP_UV "+n.clearcoatNormalMapUv:"",n.clearcoatRoughnessMapUv?"#define CLEARCOAT_ROUGHNESSMAP_UV "+n.clearcoatRoughnessMapUv:"",n.iridescenceMapUv?"#define IRIDESCENCEMAP_UV "+n.iridescenceMapUv:"",n.iridescenceThicknessMapUv?"#define IRIDESCENCE_THICKNESSMAP_UV "+n.iridescenceThicknessMapUv:"",n.sheenColorMapUv?"#define SHEEN_COLORMAP_UV "+n.sheenColorMapUv:"",n.sheenRoughnessMapUv?"#define SHEEN_ROUGHNESSMAP_UV "+n.sheenRoughnessMapUv:"",n.specularMapUv?"#define SPECULARMAP_UV "+n.specularMapUv:"",n.specularColorMapUv?"#define SPECULAR_COLORMAP_UV "+n.specularColorMapUv:"",n.specularIntensityMapUv?"#define SPECULAR_INTENSITYMAP_UV "+n.specularIntensityMapUv:"",n.transmissionMapUv?"#define TRANSMISSIONMAP_UV "+n.transmissionMapUv:"",n.thicknessMapUv?"#define THICKNESSMAP_UV "+n.thicknessMapUv:"",n.vertexTangents&&n.flatShading===!1?"#define USE_TANGENT":"",n.vertexColors?"#define USE_COLOR":"",n.vertexAlphas?"#define USE_COLOR_ALPHA":"",n.vertexUv1s?"#define USE_UV1":"",n.vertexUv2s?"#define USE_UV2":"",n.vertexUv3s?"#define USE_UV3":"",n.pointsUvs?"#define USE_POINTS_UV":"",n.flatShading?"#define FLAT_SHADED":"",n.skinning?"#define USE_SKINNING":"",n.morphTargets?"#define USE_MORPHTARGETS":"",n.morphNormals&&n.flatShading===!1?"#define USE_MORPHNORMALS":"",n.morphColors?"#define USE_MORPHCOLORS":"",n.morphTargetsCount>0?"#define MORPHTARGETS_TEXTURE_STRIDE "+n.morphTextureStride:"",n.morphTargetsCount>0?"#define MORPHTARGETS_COUNT "+n.morphTargetsCount:"",n.doubleSided?"#define DOUBLE_SIDED":"",n.flipSided?"#define FLIP_SIDED":"",n.shadowMapEnabled?"#define USE_SHADOWMAP":"",n.shadowMapEnabled?"#define "+l:"",n.sizeAttenuation?"#define USE_SIZEATTENUATION":"",n.numLightProbes>0?"#define USE_LIGHT_PROBES":"",n.logarithmicDepthBuffer?"#define USE_LOGDEPTHBUF":"",n.reverseDepthBuffer?"#define USE_REVERSEDEPTHBUF":"","uniform mat4 modelMatrix;","uniform mat4 modelViewMatrix;","uniform mat4 projectionMatrix;","uniform mat4 viewMatrix;","uniform mat3 normalMatrix;","uniform vec3 cameraPosition;","uniform bool isOrthographic;","#ifdef USE_INSTANCING","	attribute mat4 instanceMatrix;","#endif","#ifdef USE_INSTANCING_COLOR","	attribute vec3 instanceColor;","#endif","#ifdef USE_INSTANCING_MORPH","	uniform sampler2D morphTexture;","#endif","attribute vec3 position;","attribute vec3 normal;","attribute vec2 uv;","#ifdef USE_UV1","	attribute vec2 uv1;","#endif","#ifdef USE_UV2","	attribute vec2 uv2;","#endif","#ifdef USE_UV3","	attribute vec2 uv3;","#endif","#ifdef USE_TANGENT","	attribute vec4 tangent;","#endif","#if defined( USE_COLOR_ALPHA )","	attribute vec4 color;","#elif defined( USE_COLOR )","	attribute vec3 color;","#endif","#ifdef USE_SKINNING","	attribute vec4 skinIndex;","	attribute vec4 skinWeight;","#endif",`
`].filter(Ps).join(`
`),u=[Xm(n),"#define SHADER_TYPE "+n.shaderType,"#define SHADER_NAME "+n.shaderName,v,n.useFog&&n.fog?"#define USE_FOG":"",n.useFog&&n.fogExp2?"#define FOG_EXP2":"",n.alphaToCoverage?"#define ALPHA_TO_COVERAGE":"",n.map?"#define USE_MAP":"",n.matcap?"#define USE_MATCAP":"",n.envMap?"#define USE_ENVMAP":"",n.envMap?"#define "+c:"",n.envMap?"#define "+d:"",n.envMap?"#define "+f:"",h?"#define CUBEUV_TEXEL_WIDTH "+h.texelWidth:"",h?"#define CUBEUV_TEXEL_HEIGHT "+h.texelHeight:"",h?"#define CUBEUV_MAX_MIP "+h.maxMip+".0":"",n.lightMap?"#define USE_LIGHTMAP":"",n.aoMap?"#define USE_AOMAP":"",n.bumpMap?"#define USE_BUMPMAP":"",n.normalMap?"#define USE_NORMALMAP":"",n.normalMapObjectSpace?"#define USE_NORMALMAP_OBJECTSPACE":"",n.normalMapTangentSpace?"#define USE_NORMALMAP_TANGENTSPACE":"",n.emissiveMap?"#define USE_EMISSIVEMAP":"",n.anisotropy?"#define USE_ANISOTROPY":"",n.anisotropyMap?"#define USE_ANISOTROPYMAP":"",n.clearcoat?"#define USE_CLEARCOAT":"",n.clearcoatMap?"#define USE_CLEARCOATMAP":"",n.clearcoatRoughnessMap?"#define USE_CLEARCOAT_ROUGHNESSMAP":"",n.clearcoatNormalMap?"#define USE_CLEARCOAT_NORMALMAP":"",n.dispersion?"#define USE_DISPERSION":"",n.iridescence?"#define USE_IRIDESCENCE":"",n.iridescenceMap?"#define USE_IRIDESCENCEMAP":"",n.iridescenceThicknessMap?"#define USE_IRIDESCENCE_THICKNESSMAP":"",n.specularMap?"#define USE_SPECULARMAP":"",n.specularColorMap?"#define USE_SPECULAR_COLORMAP":"",n.specularIntensityMap?"#define USE_SPECULAR_INTENSITYMAP":"",n.roughnessMap?"#define USE_ROUGHNESSMAP":"",n.metalnessMap?"#define USE_METALNESSMAP":"",n.alphaMap?"#define USE_ALPHAMAP":"",n.alphaTest?"#define USE_ALPHATEST":"",n.alphaHash?"#define USE_ALPHAHASH":"",n.sheen?"#define USE_SHEEN":"",n.sheenColorMap?"#define USE_SHEEN_COLORMAP":"",n.sheenRoughnessMap?"#define USE_SHEEN_ROUGHNESSMAP":"",n.transmission?"#define USE_TRANSMISSION":"",n.transmissionMap?"#define USE_TRANSMISSIONMAP":"",n.thicknessMap?"#define USE_THICKNESSMAP":"",n.vertexTangents&&n.flatShading===!1?"#define USE_TANGENT":"",n.vertexColors||n.instancingColor||n.batchingColor?"#define USE_COLOR":"",n.vertexAlphas?"#define USE_COLOR_ALPHA":"",n.vertexUv1s?"#define USE_UV1":"",n.vertexUv2s?"#define USE_UV2":"",n.vertexUv3s?"#define USE_UV3":"",n.pointsUvs?"#define USE_POINTS_UV":"",n.gradientMap?"#define USE_GRADIENTMAP":"",n.flatShading?"#define FLAT_SHADED":"",n.doubleSided?"#define DOUBLE_SIDED":"",n.flipSided?"#define FLIP_SIDED":"",n.shadowMapEnabled?"#define USE_SHADOWMAP":"",n.shadowMapEnabled?"#define "+l:"",n.premultipliedAlpha?"#define PREMULTIPLIED_ALPHA":"",n.numLightProbes>0?"#define USE_LIGHT_PROBES":"",n.decodeVideoTexture?"#define DECODE_VIDEO_TEXTURE":"",n.decodeVideoTextureEmissive?"#define DECODE_VIDEO_TEXTURE_EMISSIVE":"",n.logarithmicDepthBuffer?"#define USE_LOGDEPTHBUF":"",n.reverseDepthBuffer?"#define USE_REVERSEDEPTHBUF":"","uniform mat4 viewMatrix;","uniform vec3 cameraPosition;","uniform bool isOrthographic;",n.toneMapping!==ir?"#define TONE_MAPPING":"",n.toneMapping!==ir?Ve.tonemapping_pars_fragment:"",n.toneMapping!==ir?DT("toneMapping",n.toneMapping):"",n.dithering?"#define DITHERING":"",n.opaque?"#define OPAQUE":"",Ve.colorspace_pars_fragment,PT("linearToOutputTexel",n.outputColorSpace),LT(),n.useDepthPacking?"#define DEPTH_PACKING "+n.depthPacking:"",`
`].filter(Ps).join(`
`)),s=_h(s),s=Vm(s,n),s=Gm(s,n),a=_h(a),a=Vm(a,n),a=Gm(a,n),s=Wm(s),a=Wm(a),n.isRawShaderMaterial!==!0&&(g=`#version 300 es
`,m=[p,"#define attribute in","#define varying out","#define texture2D texture"].join(`
`)+`
`+m,u=["#define varying in",n.glslVersion===om?"":"layout(location = 0) out highp vec4 pc_fragColor;",n.glslVersion===om?"":"#define gl_FragColor pc_fragColor","#define gl_FragDepthEXT gl_FragDepth","#define texture2D texture","#define textureCube texture","#define texture2DProj textureProj","#define texture2DLodEXT textureLod","#define texture2DProjLodEXT textureProjLod","#define textureCubeLodEXT textureLod","#define texture2DGradEXT textureGrad","#define texture2DProjGradEXT textureProjGrad","#define textureCubeGradEXT textureGrad"].join(`
`)+`
`+u);const y=g+m+s,x=g+u+a,C=zm(r,r.VERTEX_SHADER,y),A=zm(r,r.FRAGMENT_SHADER,x);r.attachShader(_,C),r.attachShader(_,A),n.index0AttributeName!==void 0?r.bindAttribLocation(_,0,n.index0AttributeName):n.morphTargets===!0&&r.bindAttribLocation(_,0,"position"),r.linkProgram(_);function T(P){if(t.debug.checkShaderErrors){const z=r.getProgramInfoLog(_).trim(),F=r.getShaderInfoLog(C).trim(),Y=r.getShaderInfoLog(A).trim();let j=!0,W=!0;if(r.getProgramParameter(_,r.LINK_STATUS)===!1)if(j=!1,typeof t.debug.onShaderError=="function")t.debug.onShaderError(r,_,C,A);else{const $=Hm(r,C,"vertex"),D=Hm(r,A,"fragment");console.error("THREE.WebGLProgram: Shader Error "+r.getError()+" - VALIDATE_STATUS "+r.getProgramParameter(_,r.VALIDATE_STATUS)+`

Material Name: `+P.name+`
Material Type: `+P.type+`

Program Info Log: `+z+`
`+$+`
`+D)}else z!==""?console.warn("THREE.WebGLProgram: Program Info Log:",z):(F===""||Y==="")&&(W=!1);W&&(P.diagnostics={runnable:j,programLog:z,vertexShader:{log:F,prefix:m},fragmentShader:{log:Y,prefix:u}})}r.deleteShader(C),r.deleteShader(A),b=new Al(r,_),w=kT(r,_)}let b;this.getUniforms=function(){return b===void 0&&T(this),b};let w;this.getAttributes=function(){return w===void 0&&T(this),w};let S=n.rendererExtensionParallelShaderCompile===!1;return this.isReady=function(){return S===!1&&(S=r.getProgramParameter(_,AT)),S},this.destroy=function(){i.releaseStatesOfProgram(this),r.deleteProgram(_),this.program=void 0},this.type=n.shaderType,this.name=n.shaderName,this.id=CT++,this.cacheKey=e,this.usedTimes=1,this.program=_,this.vertexShader=C,this.fragmentShader=A,this}let YT=0;class qT{constructor(){this.shaderCache=new Map,this.materialCache=new Map}update(e){const n=e.vertexShader,i=e.fragmentShader,r=this._getShaderStage(n),o=this._getShaderStage(i),s=this._getShaderCacheForMaterial(e);return s.has(r)===!1&&(s.add(r),r.usedTimes++),s.has(o)===!1&&(s.add(o),o.usedTimes++),this}remove(e){const n=this.materialCache.get(e);for(const i of n)i.usedTimes--,i.usedTimes===0&&this.shaderCache.delete(i.code);return this.materialCache.delete(e),this}getVertexShaderID(e){return this._getShaderStage(e.vertexShader).id}getFragmentShaderID(e){return this._getShaderStage(e.fragmentShader).id}dispose(){this.shaderCache.clear(),this.materialCache.clear()}_getShaderCacheForMaterial(e){const n=this.materialCache;let i=n.get(e);return i===void 0&&(i=new Set,n.set(e,i)),i}_getShaderStage(e){const n=this.shaderCache;let i=n.get(e);return i===void 0&&(i=new $T(e),n.set(e,i)),i}}class $T{constructor(e){this.id=YT++,this.code=e,this.usedTimes=0}}function KT(t,e,n,i,r,o,s){const a=new ey,l=new qT,c=new Set,d=[],f=r.logarithmicDepthBuffer,h=r.vertexTextures;let p=r.precision;const v={MeshDepthMaterial:"depth",MeshDistanceMaterial:"distanceRGBA",MeshNormalMaterial:"normal",MeshBasicMaterial:"basic",MeshLambertMaterial:"lambert",MeshPhongMaterial:"phong",MeshToonMaterial:"toon",MeshStandardMaterial:"physical",MeshPhysicalMaterial:"physical",MeshMatcapMaterial:"matcap",LineBasicMaterial:"basic",LineDashedMaterial:"dashed",PointsMaterial:"points",ShadowMaterial:"shadow",SpriteMaterial:"sprite"};function _(w){return c.add(w),w===0?"uv":`uv${w}`}function m(w,S,P,z,F){const Y=z.fog,j=F.geometry,W=w.isMeshStandardMaterial?z.environment:null,$=(w.isMeshStandardMaterial?n:e).get(w.envMap||W),D=$&&$.mapping===Sc?$.image.height:null,G=v[w.type];w.precision!==null&&(p=r.getMaxPrecision(w.precision),p!==w.precision&&console.warn("THREE.WebGLProgram.getParameters:",w.precision,"not supported, using",p,"instead."));const Z=j.morphAttributes.position||j.morphAttributes.normal||j.morphAttributes.color,le=Z!==void 0?Z.length:0;let Se=0;j.morphAttributes.position!==void 0&&(Se=1),j.morphAttributes.normal!==void 0&&(Se=2),j.morphAttributes.color!==void 0&&(Se=3);let Ne,q,re,he;if(G){const et=Qn[G];Ne=et.vertexShader,q=et.fragmentShader}else Ne=w.vertexShader,q=w.fragmentShader,l.update(w),re=l.getVertexShaderID(w),he=l.getFragmentShaderID(w);const ae=t.getRenderTarget(),Pe=t.state.buffers.depth.getReversed(),ne=F.isInstancedMesh===!0,ve=F.isBatchedMesh===!0,je=!!w.map,Ue=!!w.matcap,ot=!!$,I=!!w.aoMap,Nt=!!w.lightMap,Xe=!!w.bumpMap,Ge=!!w.normalMap,Me=!!w.displacementMap,nt=!!w.emissiveMap,De=!!w.metalnessMap,R=!!w.roughnessMap,M=w.anisotropy>0,O=w.clearcoat>0,J=w.dispersion>0,te=w.iridescence>0,K=w.sheen>0,Ce=w.transmission>0,de=M&&!!w.anisotropyMap,ye=O&&!!w.clearcoatMap,qe=O&&!!w.clearcoatNormalMap,oe=O&&!!w.clearcoatRoughnessMap,_e=te&&!!w.iridescenceMap,Le=te&&!!w.iridescenceThicknessMap,Ie=K&&!!w.sheenColorMap,xe=K&&!!w.sheenRoughnessMap,Ye=!!w.specularMap,He=!!w.specularColorMap,st=!!w.specularIntensityMap,N=Ce&&!!w.transmissionMap,ue=Ce&&!!w.thicknessMap,X=!!w.gradientMap,Q=!!w.alphaMap,me=w.alphaTest>0,fe=!!w.alphaHash,Fe=!!w.extensions;let _t=ir;w.toneMapped&&(ae===null||ae.isXRRenderTarget===!0)&&(_t=t.toneMapping);const Ft={shaderID:G,shaderType:w.type,shaderName:w.name,vertexShader:Ne,fragmentShader:q,defines:w.defines,customVertexShaderID:re,customFragmentShaderID:he,isRawShaderMaterial:w.isRawShaderMaterial===!0,glslVersion:w.glslVersion,precision:p,batching:ve,batchingColor:ve&&F._colorsTexture!==null,instancing:ne,instancingColor:ne&&F.instanceColor!==null,instancingMorph:ne&&F.morphTexture!==null,supportsVertexTextures:h,outputColorSpace:ae===null?t.outputColorSpace:ae.isXRRenderTarget===!0?ae.texture.colorSpace:Jo,alphaToCoverage:!!w.alphaToCoverage,map:je,matcap:Ue,envMap:ot,envMapMode:ot&&$.mapping,envMapCubeUVHeight:D,aoMap:I,lightMap:Nt,bumpMap:Xe,normalMap:Ge,displacementMap:h&&Me,emissiveMap:nt,normalMapObjectSpace:Ge&&w.normalMapType===nM,normalMapTangentSpace:Ge&&w.normalMapType===$v,metalnessMap:De,roughnessMap:R,anisotropy:M,anisotropyMap:de,clearcoat:O,clearcoatMap:ye,clearcoatNormalMap:qe,clearcoatRoughnessMap:oe,dispersion:J,iridescence:te,iridescenceMap:_e,iridescenceThicknessMap:Le,sheen:K,sheenColorMap:Ie,sheenRoughnessMap:xe,specularMap:Ye,specularColorMap:He,specularIntensityMap:st,transmission:Ce,transmissionMap:N,thicknessMap:ue,gradientMap:X,opaque:w.transparent===!1&&w.blending===Do&&w.alphaToCoverage===!1,alphaMap:Q,alphaTest:me,alphaHash:fe,combine:w.combine,mapUv:je&&_(w.map.channel),aoMapUv:I&&_(w.aoMap.channel),lightMapUv:Nt&&_(w.lightMap.channel),bumpMapUv:Xe&&_(w.bumpMap.channel),normalMapUv:Ge&&_(w.normalMap.channel),displacementMapUv:Me&&_(w.displacementMap.channel),emissiveMapUv:nt&&_(w.emissiveMap.channel),metalnessMapUv:De&&_(w.metalnessMap.channel),roughnessMapUv:R&&_(w.roughnessMap.channel),anisotropyMapUv:de&&_(w.anisotropyMap.channel),clearcoatMapUv:ye&&_(w.clearcoatMap.channel),clearcoatNormalMapUv:qe&&_(w.clearcoatNormalMap.channel),clearcoatRoughnessMapUv:oe&&_(w.clearcoatRoughnessMap.channel),iridescenceMapUv:_e&&_(w.iridescenceMap.channel),iridescenceThicknessMapUv:Le&&_(w.iridescenceThicknessMap.channel),sheenColorMapUv:Ie&&_(w.sheenColorMap.channel),sheenRoughnessMapUv:xe&&_(w.sheenRoughnessMap.channel),specularMapUv:Ye&&_(w.specularMap.channel),specularColorMapUv:He&&_(w.specularColorMap.channel),specularIntensityMapUv:st&&_(w.specularIntensityMap.channel),transmissionMapUv:N&&_(w.transmissionMap.channel),thicknessMapUv:ue&&_(w.thicknessMap.channel),alphaMapUv:Q&&_(w.alphaMap.channel),vertexTangents:!!j.attributes.tangent&&(Ge||M),vertexColors:w.vertexColors,vertexAlphas:w.vertexColors===!0&&!!j.attributes.color&&j.attributes.color.itemSize===4,pointsUvs:F.isPoints===!0&&!!j.attributes.uv&&(je||Q),fog:!!Y,useFog:w.fog===!0,fogExp2:!!Y&&Y.isFogExp2,flatShading:w.flatShading===!0,sizeAttenuation:w.sizeAttenuation===!0,logarithmicDepthBuffer:f,reverseDepthBuffer:Pe,skinning:F.isSkinnedMesh===!0,morphTargets:j.morphAttributes.position!==void 0,morphNormals:j.morphAttributes.normal!==void 0,morphColors:j.morphAttributes.color!==void 0,morphTargetsCount:le,morphTextureStride:Se,numDirLights:S.directional.length,numPointLights:S.point.length,numSpotLights:S.spot.length,numSpotLightMaps:S.spotLightMap.length,numRectAreaLights:S.rectArea.length,numHemiLights:S.hemi.length,numDirLightShadows:S.directionalShadowMap.length,numPointLightShadows:S.pointShadowMap.length,numSpotLightShadows:S.spotShadowMap.length,numSpotLightShadowsWithMaps:S.numSpotLightShadowsWithMaps,numLightProbes:S.numLightProbes,numClippingPlanes:s.numPlanes,numClipIntersection:s.numIntersection,dithering:w.dithering,shadowMapEnabled:t.shadowMap.enabled&&P.length>0,shadowMapType:t.shadowMap.type,toneMapping:_t,decodeVideoTexture:je&&w.map.isVideoTexture===!0&&$e.getTransfer(w.map.colorSpace)===it,decodeVideoTextureEmissive:nt&&w.emissiveMap.isVideoTexture===!0&&$e.getTransfer(w.emissiveMap.colorSpace)===it,premultipliedAlpha:w.premultipliedAlpha,doubleSided:w.side===bn,flipSided:w.side===Jt,useDepthPacking:w.depthPacking>=0,depthPacking:w.depthPacking||0,index0AttributeName:w.index0AttributeName,extensionClipCullDistance:Fe&&w.extensions.clipCullDistance===!0&&i.has("WEBGL_clip_cull_distance"),extensionMultiDraw:(Fe&&w.extensions.multiDraw===!0||ve)&&i.has("WEBGL_multi_draw"),rendererExtensionParallelShaderCompile:i.has("KHR_parallel_shader_compile"),customProgramCacheKey:w.customProgramCacheKey()};return Ft.vertexUv1s=c.has(1),Ft.vertexUv2s=c.has(2),Ft.vertexUv3s=c.has(3),c.clear(),Ft}function u(w){const S=[];if(w.shaderID?S.push(w.shaderID):(S.push(w.customVertexShaderID),S.push(w.customFragmentShaderID)),w.defines!==void 0)for(const P in w.defines)S.push(P),S.push(w.defines[P]);return w.isRawShaderMaterial===!1&&(g(S,w),y(S,w),S.push(t.outputColorSpace)),S.push(w.customProgramCacheKey),S.join()}function g(w,S){w.push(S.precision),w.push(S.outputColorSpace),w.push(S.envMapMode),w.push(S.envMapCubeUVHeight),w.push(S.mapUv),w.push(S.alphaMapUv),w.push(S.lightMapUv),w.push(S.aoMapUv),w.push(S.bumpMapUv),w.push(S.normalMapUv),w.push(S.displacementMapUv),w.push(S.emissiveMapUv),w.push(S.metalnessMapUv),w.push(S.roughnessMapUv),w.push(S.anisotropyMapUv),w.push(S.clearcoatMapUv),w.push(S.clearcoatNormalMapUv),w.push(S.clearcoatRoughnessMapUv),w.push(S.iridescenceMapUv),w.push(S.iridescenceThicknessMapUv),w.push(S.sheenColorMapUv),w.push(S.sheenRoughnessMapUv),w.push(S.specularMapUv),w.push(S.specularColorMapUv),w.push(S.specularIntensityMapUv),w.push(S.transmissionMapUv),w.push(S.thicknessMapUv),w.push(S.combine),w.push(S.fogExp2),w.push(S.sizeAttenuation),w.push(S.morphTargetsCount),w.push(S.morphAttributeCount),w.push(S.numDirLights),w.push(S.numPointLights),w.push(S.numSpotLights),w.push(S.numSpotLightMaps),w.push(S.numHemiLights),w.push(S.numRectAreaLights),w.push(S.numDirLightShadows),w.push(S.numPointLightShadows),w.push(S.numSpotLightShadows),w.push(S.numSpotLightShadowsWithMaps),w.push(S.numLightProbes),w.push(S.shadowMapType),w.push(S.toneMapping),w.push(S.numClippingPlanes),w.push(S.numClipIntersection),w.push(S.depthPacking)}function y(w,S){a.disableAll(),S.supportsVertexTextures&&a.enable(0),S.instancing&&a.enable(1),S.instancingColor&&a.enable(2),S.instancingMorph&&a.enable(3),S.matcap&&a.enable(4),S.envMap&&a.enable(5),S.normalMapObjectSpace&&a.enable(6),S.normalMapTangentSpace&&a.enable(7),S.clearcoat&&a.enable(8),S.iridescence&&a.enable(9),S.alphaTest&&a.enable(10),S.vertexColors&&a.enable(11),S.vertexAlphas&&a.enable(12),S.vertexUv1s&&a.enable(13),S.vertexUv2s&&a.enable(14),S.vertexUv3s&&a.enable(15),S.vertexTangents&&a.enable(16),S.anisotropy&&a.enable(17),S.alphaHash&&a.enable(18),S.batching&&a.enable(19),S.dispersion&&a.enable(20),S.batchingColor&&a.enable(21),w.push(a.mask),a.disableAll(),S.fog&&a.enable(0),S.useFog&&a.enable(1),S.flatShading&&a.enable(2),S.logarithmicDepthBuffer&&a.enable(3),S.reverseDepthBuffer&&a.enable(4),S.skinning&&a.enable(5),S.morphTargets&&a.enable(6),S.morphNormals&&a.enable(7),S.morphColors&&a.enable(8),S.premultipliedAlpha&&a.enable(9),S.shadowMapEnabled&&a.enable(10),S.doubleSided&&a.enable(11),S.flipSided&&a.enable(12),S.useDepthPacking&&a.enable(13),S.dithering&&a.enable(14),S.transmission&&a.enable(15),S.sheen&&a.enable(16),S.opaque&&a.enable(17),S.pointsUvs&&a.enable(18),S.decodeVideoTexture&&a.enable(19),S.decodeVideoTextureEmissive&&a.enable(20),S.alphaToCoverage&&a.enable(21),w.push(a.mask)}function x(w){const S=v[w.type];let P;if(S){const z=Qn[S];P=NM.clone(z.uniforms)}else P=w.uniforms;return P}function C(w,S){let P;for(let z=0,F=d.length;z<F;z++){const Y=d[z];if(Y.cacheKey===S){P=Y,++P.usedTimes;break}}return P===void 0&&(P=new jT(t,S,w,o),d.push(P)),P}function A(w){if(--w.usedTimes===0){const S=d.indexOf(w);d[S]=d[d.length-1],d.pop(),w.destroy()}}function T(w){l.remove(w)}function b(){l.dispose()}return{getParameters:m,getProgramCacheKey:u,getUniforms:x,acquireProgram:C,releaseProgram:A,releaseShaderCache:T,programs:d,dispose:b}}function ZT(){let t=new WeakMap;function e(s){return t.has(s)}function n(s){let a=t.get(s);return a===void 0&&(a={},t.set(s,a)),a}function i(s){t.delete(s)}function r(s,a,l){t.get(s)[a]=l}function o(){t=new WeakMap}return{has:e,get:n,remove:i,update:r,dispose:o}}function JT(t,e){return t.groupOrder!==e.groupOrder?t.groupOrder-e.groupOrder:t.renderOrder!==e.renderOrder?t.renderOrder-e.renderOrder:t.material.id!==e.material.id?t.material.id-e.material.id:t.z!==e.z?t.z-e.z:t.id-e.id}function jm(t,e){return t.groupOrder!==e.groupOrder?t.groupOrder-e.groupOrder:t.renderOrder!==e.renderOrder?t.renderOrder-e.renderOrder:t.z!==e.z?e.z-t.z:t.id-e.id}function Ym(){const t=[];let e=0;const n=[],i=[],r=[];function o(){e=0,n.length=0,i.length=0,r.length=0}function s(f,h,p,v,_,m){let u=t[e];return u===void 0?(u={id:f.id,object:f,geometry:h,material:p,groupOrder:v,renderOrder:f.renderOrder,z:_,group:m},t[e]=u):(u.id=f.id,u.object=f,u.geometry=h,u.material=p,u.groupOrder=v,u.renderOrder=f.renderOrder,u.z=_,u.group=m),e++,u}function a(f,h,p,v,_,m){const u=s(f,h,p,v,_,m);p.transmission>0?i.push(u):p.transparent===!0?r.push(u):n.push(u)}function l(f,h,p,v,_,m){const u=s(f,h,p,v,_,m);p.transmission>0?i.unshift(u):p.transparent===!0?r.unshift(u):n.unshift(u)}function c(f,h){n.length>1&&n.sort(f||JT),i.length>1&&i.sort(h||jm),r.length>1&&r.sort(h||jm)}function d(){for(let f=e,h=t.length;f<h;f++){const p=t[f];if(p.id===null)break;p.id=null,p.object=null,p.geometry=null,p.material=null,p.group=null}}return{opaque:n,transmissive:i,transparent:r,init:o,push:a,unshift:l,finish:d,sort:c}}function QT(){let t=new WeakMap;function e(i,r){const o=t.get(i);let s;return o===void 0?(s=new Ym,t.set(i,[s])):r>=o.length?(s=new Ym,o.push(s)):s=o[r],s}function n(){t=new WeakMap}return{get:e,dispose:n}}function eA(){const t={};return{get:function(e){if(t[e.id]!==void 0)return t[e.id];let n;switch(e.type){case"DirectionalLight":n={direction:new L,color:new we};break;case"SpotLight":n={position:new L,direction:new L,color:new we,distance:0,coneCos:0,penumbraCos:0,decay:0};break;case"PointLight":n={position:new L,color:new we,distance:0,decay:0};break;case"HemisphereLight":n={direction:new L,skyColor:new we,groundColor:new we};break;case"RectAreaLight":n={color:new we,position:new L,halfWidth:new L,halfHeight:new L};break}return t[e.id]=n,n}}}function tA(){const t={};return{get:function(e){if(t[e.id]!==void 0)return t[e.id];let n;switch(e.type){case"DirectionalLight":n={shadowIntensity:1,shadowBias:0,shadowNormalBias:0,shadowRadius:1,shadowMapSize:new Re};break;case"SpotLight":n={shadowIntensity:1,shadowBias:0,shadowNormalBias:0,shadowRadius:1,shadowMapSize:new Re};break;case"PointLight":n={shadowIntensity:1,shadowBias:0,shadowNormalBias:0,shadowRadius:1,shadowMapSize:new Re,shadowCameraNear:1,shadowCameraFar:1e3};break}return t[e.id]=n,n}}}let nA=0;function iA(t,e){return(e.castShadow?2:0)-(t.castShadow?2:0)+(e.map?1:0)-(t.map?1:0)}function rA(t){const e=new eA,n=tA(),i={version:0,hash:{directionalLength:-1,pointLength:-1,spotLength:-1,rectAreaLength:-1,hemiLength:-1,numDirectionalShadows:-1,numPointShadows:-1,numSpotShadows:-1,numSpotMaps:-1,numLightProbes:-1},ambient:[0,0,0],probe:[],directional:[],directionalShadow:[],directionalShadowMap:[],directionalShadowMatrix:[],spot:[],spotLightMap:[],spotShadow:[],spotShadowMap:[],spotLightMatrix:[],rectArea:[],rectAreaLTC1:null,rectAreaLTC2:null,point:[],pointShadow:[],pointShadowMap:[],pointShadowMatrix:[],hemi:[],numSpotLightShadowsWithMaps:0,numLightProbes:0};for(let c=0;c<9;c++)i.probe.push(new L);const r=new L,o=new rt,s=new rt;function a(c){let d=0,f=0,h=0;for(let w=0;w<9;w++)i.probe[w].set(0,0,0);let p=0,v=0,_=0,m=0,u=0,g=0,y=0,x=0,C=0,A=0,T=0;c.sort(iA);for(let w=0,S=c.length;w<S;w++){const P=c[w],z=P.color,F=P.intensity,Y=P.distance,j=P.shadow&&P.shadow.map?P.shadow.map.texture:null;if(P.isAmbientLight)d+=z.r*F,f+=z.g*F,h+=z.b*F;else if(P.isLightProbe){for(let W=0;W<9;W++)i.probe[W].addScaledVector(P.sh.coefficients[W],F);T++}else if(P.isDirectionalLight){const W=e.get(P);if(W.color.copy(P.color).multiplyScalar(P.intensity),P.castShadow){const $=P.shadow,D=n.get(P);D.shadowIntensity=$.intensity,D.shadowBias=$.bias,D.shadowNormalBias=$.normalBias,D.shadowRadius=$.radius,D.shadowMapSize=$.mapSize,i.directionalShadow[p]=D,i.directionalShadowMap[p]=j,i.directionalShadowMatrix[p]=P.shadow.matrix,g++}i.directional[p]=W,p++}else if(P.isSpotLight){const W=e.get(P);W.position.setFromMatrixPosition(P.matrixWorld),W.color.copy(z).multiplyScalar(F),W.distance=Y,W.coneCos=Math.cos(P.angle),W.penumbraCos=Math.cos(P.angle*(1-P.penumbra)),W.decay=P.decay,i.spot[_]=W;const $=P.shadow;if(P.map&&(i.spotLightMap[C]=P.map,C++,$.updateMatrices(P),P.castShadow&&A++),i.spotLightMatrix[_]=$.matrix,P.castShadow){const D=n.get(P);D.shadowIntensity=$.intensity,D.shadowBias=$.bias,D.shadowNormalBias=$.normalBias,D.shadowRadius=$.radius,D.shadowMapSize=$.mapSize,i.spotShadow[_]=D,i.spotShadowMap[_]=j,x++}_++}else if(P.isRectAreaLight){const W=e.get(P);W.color.copy(z).multiplyScalar(F),W.halfWidth.set(P.width*.5,0,0),W.halfHeight.set(0,P.height*.5,0),i.rectArea[m]=W,m++}else if(P.isPointLight){const W=e.get(P);if(W.color.copy(P.color).multiplyScalar(P.intensity),W.distance=P.distance,W.decay=P.decay,P.castShadow){const $=P.shadow,D=n.get(P);D.shadowIntensity=$.intensity,D.shadowBias=$.bias,D.shadowNormalBias=$.normalBias,D.shadowRadius=$.radius,D.shadowMapSize=$.mapSize,D.shadowCameraNear=$.camera.near,D.shadowCameraFar=$.camera.far,i.pointShadow[v]=D,i.pointShadowMap[v]=j,i.pointShadowMatrix[v]=P.shadow.matrix,y++}i.point[v]=W,v++}else if(P.isHemisphereLight){const W=e.get(P);W.skyColor.copy(P.color).multiplyScalar(F),W.groundColor.copy(P.groundColor).multiplyScalar(F),i.hemi[u]=W,u++}}m>0&&(t.has("OES_texture_float_linear")===!0?(i.rectAreaLTC1=ce.LTC_FLOAT_1,i.rectAreaLTC2=ce.LTC_FLOAT_2):(i.rectAreaLTC1=ce.LTC_HALF_1,i.rectAreaLTC2=ce.LTC_HALF_2)),i.ambient[0]=d,i.ambient[1]=f,i.ambient[2]=h;const b=i.hash;(b.directionalLength!==p||b.pointLength!==v||b.spotLength!==_||b.rectAreaLength!==m||b.hemiLength!==u||b.numDirectionalShadows!==g||b.numPointShadows!==y||b.numSpotShadows!==x||b.numSpotMaps!==C||b.numLightProbes!==T)&&(i.directional.length=p,i.spot.length=_,i.rectArea.length=m,i.point.length=v,i.hemi.length=u,i.directionalShadow.length=g,i.directionalShadowMap.length=g,i.pointShadow.length=y,i.pointShadowMap.length=y,i.spotShadow.length=x,i.spotShadowMap.length=x,i.directionalShadowMatrix.length=g,i.pointShadowMatrix.length=y,i.spotLightMatrix.length=x+C-A,i.spotLightMap.length=C,i.numSpotLightShadowsWithMaps=A,i.numLightProbes=T,b.directionalLength=p,b.pointLength=v,b.spotLength=_,b.rectAreaLength=m,b.hemiLength=u,b.numDirectionalShadows=g,b.numPointShadows=y,b.numSpotShadows=x,b.numSpotMaps=C,b.numLightProbes=T,i.version=nA++)}function l(c,d){let f=0,h=0,p=0,v=0,_=0;const m=d.matrixWorldInverse;for(let u=0,g=c.length;u<g;u++){const y=c[u];if(y.isDirectionalLight){const x=i.directional[f];x.direction.setFromMatrixPosition(y.matrixWorld),r.setFromMatrixPosition(y.target.matrixWorld),x.direction.sub(r),x.direction.transformDirection(m),f++}else if(y.isSpotLight){const x=i.spot[p];x.position.setFromMatrixPosition(y.matrixWorld),x.position.applyMatrix4(m),x.direction.setFromMatrixPosition(y.matrixWorld),r.setFromMatrixPosition(y.target.matrixWorld),x.direction.sub(r),x.direction.transformDirection(m),p++}else if(y.isRectAreaLight){const x=i.rectArea[v];x.position.setFromMatrixPosition(y.matrixWorld),x.position.applyMatrix4(m),s.identity(),o.copy(y.matrixWorld),o.premultiply(m),s.extractRotation(o),x.halfWidth.set(y.width*.5,0,0),x.halfHeight.set(0,y.height*.5,0),x.halfWidth.applyMatrix4(s),x.halfHeight.applyMatrix4(s),v++}else if(y.isPointLight){const x=i.point[h];x.position.setFromMatrixPosition(y.matrixWorld),x.position.applyMatrix4(m),h++}else if(y.isHemisphereLight){const x=i.hemi[_];x.direction.setFromMatrixPosition(y.matrixWorld),x.direction.transformDirection(m),_++}}}return{setup:a,setupView:l,state:i}}function qm(t){const e=new rA(t),n=[],i=[];function r(d){c.camera=d,n.length=0,i.length=0}function o(d){n.push(d)}function s(d){i.push(d)}function a(){e.setup(n)}function l(d){e.setupView(n,d)}const c={lightsArray:n,shadowsArray:i,camera:null,lights:e,transmissionRenderTarget:{}};return{init:r,state:c,setupLights:a,setupLightsView:l,pushLight:o,pushShadow:s}}function oA(t){let e=new WeakMap;function n(r,o=0){const s=e.get(r);let a;return s===void 0?(a=new qm(t),e.set(r,[a])):o>=s.length?(a=new qm(t),s.push(a)):a=s[o],a}function i(){e=new WeakMap}return{get:n,dispose:i}}class sA extends ya{static get type(){return"MeshDepthMaterial"}constructor(e){super(),this.isMeshDepthMaterial=!0,this.depthPacking=eM,this.map=null,this.alphaMap=null,this.displacementMap=null,this.displacementScale=1,this.displacementBias=0,this.wireframe=!1,this.wireframeLinewidth=1,this.setValues(e)}copy(e){return super.copy(e),this.depthPacking=e.depthPacking,this.map=e.map,this.alphaMap=e.alphaMap,this.displacementMap=e.displacementMap,this.displacementScale=e.displacementScale,this.displacementBias=e.displacementBias,this.wireframe=e.wireframe,this.wireframeLinewidth=e.wireframeLinewidth,this}}class aA extends ya{static get type(){return"MeshDistanceMaterial"}constructor(e){super(),this.isMeshDistanceMaterial=!0,this.map=null,this.alphaMap=null,this.displacementMap=null,this.displacementScale=1,this.displacementBias=0,this.setValues(e)}copy(e){return super.copy(e),this.map=e.map,this.alphaMap=e.alphaMap,this.displacementMap=e.displacementMap,this.displacementScale=e.displacementScale,this.displacementBias=e.displacementBias,this}}const lA=`void main() {
	gl_Position = vec4( position, 1.0 );
}`,cA=`uniform sampler2D shadow_pass;
uniform vec2 resolution;
uniform float radius;
#include <packing>
void main() {
	const float samples = float( VSM_SAMPLES );
	float mean = 0.0;
	float squared_mean = 0.0;
	float uvStride = samples <= 1.0 ? 0.0 : 2.0 / ( samples - 1.0 );
	float uvStart = samples <= 1.0 ? 0.0 : - 1.0;
	for ( float i = 0.0; i < samples; i ++ ) {
		float uvOffset = uvStart + i * uvStride;
		#ifdef HORIZONTAL_PASS
			vec2 distribution = unpackRGBATo2Half( texture2D( shadow_pass, ( gl_FragCoord.xy + vec2( uvOffset, 0.0 ) * radius ) / resolution ) );
			mean += distribution.x;
			squared_mean += distribution.y * distribution.y + distribution.x * distribution.x;
		#else
			float depth = unpackRGBAToDepth( texture2D( shadow_pass, ( gl_FragCoord.xy + vec2( 0.0, uvOffset ) * radius ) / resolution ) );
			mean += depth;
			squared_mean += depth * depth;
		#endif
	}
	mean = mean / samples;
	squared_mean = squared_mean / samples;
	float std_dev = sqrt( squared_mean - mean * mean );
	gl_FragColor = pack2HalfToRGBA( vec2( mean, std_dev ) );
}`;function uA(t,e,n){let i=new Af;const r=new Re,o=new Re,s=new St,a=new sA({depthPacking:tM}),l=new aA,c={},d=n.maxTextureSize,f={[sr]:Jt,[Jt]:sr,[bn]:bn},h=new xn({defines:{VSM_SAMPLES:8},uniforms:{shadow_pass:{value:null},resolution:{value:new Re},radius:{value:4}},vertexShader:lA,fragmentShader:cA}),p=h.clone();p.defines.HORIZONTAL_PASS=1;const v=new Yt;v.setAttribute("position",new en(new Float32Array([-1,-1,.5,3,-1,.5,-1,3,.5]),3));const _=new Ct(v,h),m=this;this.enabled=!1,this.autoUpdate=!0,this.needsUpdate=!1,this.type=Iv;let u=this.type;this.render=function(A,T,b){if(m.enabled===!1||m.autoUpdate===!1&&m.needsUpdate===!1||A.length===0)return;const w=t.getRenderTarget(),S=t.getActiveCubeFace(),P=t.getActiveMipmapLevel(),z=t.state;z.setBlending(nr),z.buffers.color.setClear(1,1,1,1),z.buffers.depth.setTest(!0),z.setScissorTest(!1);const F=u!==fi&&this.type===fi,Y=u===fi&&this.type!==fi;for(let j=0,W=A.length;j<W;j++){const $=A[j],D=$.shadow;if(D===void 0){console.warn("THREE.WebGLShadowMap:",$,"has no shadow.");continue}if(D.autoUpdate===!1&&D.needsUpdate===!1)continue;r.copy(D.mapSize);const G=D.getFrameExtents();if(r.multiply(G),o.copy(D.mapSize),(r.x>d||r.y>d)&&(r.x>d&&(o.x=Math.floor(d/G.x),r.x=o.x*G.x,D.mapSize.x=o.x),r.y>d&&(o.y=Math.floor(d/G.y),r.y=o.y*G.y,D.mapSize.y=o.y)),D.map===null||F===!0||Y===!0){const le=this.type!==fi?{minFilter:dn,magFilter:dn}:{};D.map!==null&&D.map.dispose(),D.map=new qn(r.x,r.y,le),D.map.texture.name=$.name+".shadowMap",D.camera.updateProjectionMatrix()}t.setRenderTarget(D.map),t.clear();const Z=D.getViewportCount();for(let le=0;le<Z;le++){const Se=D.getViewport(le);s.set(o.x*Se.x,o.y*Se.y,o.x*Se.z,o.y*Se.w),z.viewport(s),D.updateMatrices($,le),i=D.getFrustum(),x(T,b,D.camera,$,this.type)}D.isPointLightShadow!==!0&&this.type===fi&&g(D,b),D.needsUpdate=!1}u=this.type,m.needsUpdate=!1,t.setRenderTarget(w,S,P)};function g(A,T){const b=e.update(_);h.defines.VSM_SAMPLES!==A.blurSamples&&(h.defines.VSM_SAMPLES=A.blurSamples,p.defines.VSM_SAMPLES=A.blurSamples,h.needsUpdate=!0,p.needsUpdate=!0),A.mapPass===null&&(A.mapPass=new qn(r.x,r.y)),h.uniforms.shadow_pass.value=A.map.texture,h.uniforms.resolution.value=A.mapSize,h.uniforms.radius.value=A.radius,t.setRenderTarget(A.mapPass),t.clear(),t.renderBufferDirect(T,null,b,h,_,null),p.uniforms.shadow_pass.value=A.mapPass.texture,p.uniforms.resolution.value=A.mapSize,p.uniforms.radius.value=A.radius,t.setRenderTarget(A.map),t.clear(),t.renderBufferDirect(T,null,b,p,_,null)}function y(A,T,b,w){let S=null;const P=b.isPointLight===!0?A.customDistanceMaterial:A.customDepthMaterial;if(P!==void 0)S=P;else if(S=b.isPointLight===!0?l:a,t.localClippingEnabled&&T.clipShadows===!0&&Array.isArray(T.clippingPlanes)&&T.clippingPlanes.length!==0||T.displacementMap&&T.displacementScale!==0||T.alphaMap&&T.alphaTest>0||T.map&&T.alphaTest>0){const z=S.uuid,F=T.uuid;let Y=c[z];Y===void 0&&(Y={},c[z]=Y);let j=Y[F];j===void 0&&(j=S.clone(),Y[F]=j,T.addEventListener("dispose",C)),S=j}if(S.visible=T.visible,S.wireframe=T.wireframe,w===fi?S.side=T.shadowSide!==null?T.shadowSide:T.side:S.side=T.shadowSide!==null?T.shadowSide:f[T.side],S.alphaMap=T.alphaMap,S.alphaTest=T.alphaTest,S.map=T.map,S.clipShadows=T.clipShadows,S.clippingPlanes=T.clippingPlanes,S.clipIntersection=T.clipIntersection,S.displacementMap=T.displacementMap,S.displacementScale=T.displacementScale,S.displacementBias=T.displacementBias,S.wireframeLinewidth=T.wireframeLinewidth,S.linewidth=T.linewidth,b.isPointLight===!0&&S.isMeshDistanceMaterial===!0){const z=t.properties.get(S);z.light=b}return S}function x(A,T,b,w,S){if(A.visible===!1)return;if(A.layers.test(T.layers)&&(A.isMesh||A.isLine||A.isPoints)&&(A.castShadow||A.receiveShadow&&S===fi)&&(!A.frustumCulled||i.intersectsObject(A))){A.modelViewMatrix.multiplyMatrices(b.matrixWorldInverse,A.matrixWorld);const F=e.update(A),Y=A.material;if(Array.isArray(Y)){const j=F.groups;for(let W=0,$=j.length;W<$;W++){const D=j[W],G=Y[D.materialIndex];if(G&&G.visible){const Z=y(A,G,w,S);A.onBeforeShadow(t,A,T,b,F,Z,D),t.renderBufferDirect(b,null,F,Z,A,D),A.onAfterShadow(t,A,T,b,F,Z,D)}}}else if(Y.visible){const j=y(A,Y,w,S);A.onBeforeShadow(t,A,T,b,F,j,null),t.renderBufferDirect(b,null,F,j,A,null),A.onAfterShadow(t,A,T,b,F,j,null)}}const z=A.children;for(let F=0,Y=z.length;F<Y;F++)x(z[F],T,b,w,S)}function C(A){A.target.removeEventListener("dispose",C);for(const b in c){const w=c[b],S=A.target.uuid;S in w&&(w[S].dispose(),delete w[S])}}}const dA={[kd]:Ud,[Fd]:Bd,[Od]:Hd,[Ho]:zd,[Ud]:kd,[Bd]:Fd,[Hd]:Od,[zd]:Ho};function hA(t,e){function n(){let N=!1;const ue=new St;let X=null;const Q=new St(0,0,0,0);return{setMask:function(me){X!==me&&!N&&(t.colorMask(me,me,me,me),X=me)},setLocked:function(me){N=me},setClear:function(me,fe,Fe,_t,Ft){Ft===!0&&(me*=_t,fe*=_t,Fe*=_t),ue.set(me,fe,Fe,_t),Q.equals(ue)===!1&&(t.clearColor(me,fe,Fe,_t),Q.copy(ue))},reset:function(){N=!1,X=null,Q.set(-1,0,0,0)}}}function i(){let N=!1,ue=!1,X=null,Q=null,me=null;return{setReversed:function(fe){if(ue!==fe){const Fe=e.get("EXT_clip_control");ue?Fe.clipControlEXT(Fe.LOWER_LEFT_EXT,Fe.ZERO_TO_ONE_EXT):Fe.clipControlEXT(Fe.LOWER_LEFT_EXT,Fe.NEGATIVE_ONE_TO_ONE_EXT);const _t=me;me=null,this.setClear(_t)}ue=fe},getReversed:function(){return ue},setTest:function(fe){fe?ae(t.DEPTH_TEST):Pe(t.DEPTH_TEST)},setMask:function(fe){X!==fe&&!N&&(t.depthMask(fe),X=fe)},setFunc:function(fe){if(ue&&(fe=dA[fe]),Q!==fe){switch(fe){case kd:t.depthFunc(t.NEVER);break;case Ud:t.depthFunc(t.ALWAYS);break;case Fd:t.depthFunc(t.LESS);break;case Ho:t.depthFunc(t.LEQUAL);break;case Od:t.depthFunc(t.EQUAL);break;case zd:t.depthFunc(t.GEQUAL);break;case Bd:t.depthFunc(t.GREATER);break;case Hd:t.depthFunc(t.NOTEQUAL);break;default:t.depthFunc(t.LEQUAL)}Q=fe}},setLocked:function(fe){N=fe},setClear:function(fe){me!==fe&&(ue&&(fe=1-fe),t.clearDepth(fe),me=fe)},reset:function(){N=!1,X=null,Q=null,me=null,ue=!1}}}function r(){let N=!1,ue=null,X=null,Q=null,me=null,fe=null,Fe=null,_t=null,Ft=null;return{setTest:function(et){N||(et?ae(t.STENCIL_TEST):Pe(t.STENCIL_TEST))},setMask:function(et){ue!==et&&!N&&(t.stencilMask(et),ue=et)},setFunc:function(et,kn,si){(X!==et||Q!==kn||me!==si)&&(t.stencilFunc(et,kn,si),X=et,Q=kn,me=si)},setOp:function(et,kn,si){(fe!==et||Fe!==kn||_t!==si)&&(t.stencilOp(et,kn,si),fe=et,Fe=kn,_t=si)},setLocked:function(et){N=et},setClear:function(et){Ft!==et&&(t.clearStencil(et),Ft=et)},reset:function(){N=!1,ue=null,X=null,Q=null,me=null,fe=null,Fe=null,_t=null,Ft=null}}}const o=new n,s=new i,a=new r,l=new WeakMap,c=new WeakMap;let d={},f={},h=new WeakMap,p=[],v=null,_=!1,m=null,u=null,g=null,y=null,x=null,C=null,A=null,T=new we(0,0,0),b=0,w=!1,S=null,P=null,z=null,F=null,Y=null;const j=t.getParameter(t.MAX_COMBINED_TEXTURE_IMAGE_UNITS);let W=!1,$=0;const D=t.getParameter(t.VERSION);D.indexOf("WebGL")!==-1?($=parseFloat(/^WebGL (\d)/.exec(D)[1]),W=$>=1):D.indexOf("OpenGL ES")!==-1&&($=parseFloat(/^OpenGL ES (\d)/.exec(D)[1]),W=$>=2);let G=null,Z={};const le=t.getParameter(t.SCISSOR_BOX),Se=t.getParameter(t.VIEWPORT),Ne=new St().fromArray(le),q=new St().fromArray(Se);function re(N,ue,X,Q){const me=new Uint8Array(4),fe=t.createTexture();t.bindTexture(N,fe),t.texParameteri(N,t.TEXTURE_MIN_FILTER,t.NEAREST),t.texParameteri(N,t.TEXTURE_MAG_FILTER,t.NEAREST);for(let Fe=0;Fe<X;Fe++)N===t.TEXTURE_3D||N===t.TEXTURE_2D_ARRAY?t.texImage3D(ue,0,t.RGBA,1,1,Q,0,t.RGBA,t.UNSIGNED_BYTE,me):t.texImage2D(ue+Fe,0,t.RGBA,1,1,0,t.RGBA,t.UNSIGNED_BYTE,me);return fe}const he={};he[t.TEXTURE_2D]=re(t.TEXTURE_2D,t.TEXTURE_2D,1),he[t.TEXTURE_CUBE_MAP]=re(t.TEXTURE_CUBE_MAP,t.TEXTURE_CUBE_MAP_POSITIVE_X,6),he[t.TEXTURE_2D_ARRAY]=re(t.TEXTURE_2D_ARRAY,t.TEXTURE_2D_ARRAY,1,1),he[t.TEXTURE_3D]=re(t.TEXTURE_3D,t.TEXTURE_3D,1,1),o.setClear(0,0,0,1),s.setClear(1),a.setClear(0),ae(t.DEPTH_TEST),s.setFunc(Ho),Xe(!1),Ge(Qp),ae(t.CULL_FACE),I(nr);function ae(N){d[N]!==!0&&(t.enable(N),d[N]=!0)}function Pe(N){d[N]!==!1&&(t.disable(N),d[N]=!1)}function ne(N,ue){return f[N]!==ue?(t.bindFramebuffer(N,ue),f[N]=ue,N===t.DRAW_FRAMEBUFFER&&(f[t.FRAMEBUFFER]=ue),N===t.FRAMEBUFFER&&(f[t.DRAW_FRAMEBUFFER]=ue),!0):!1}function ve(N,ue){let X=p,Q=!1;if(N){X=h.get(ue),X===void 0&&(X=[],h.set(ue,X));const me=N.textures;if(X.length!==me.length||X[0]!==t.COLOR_ATTACHMENT0){for(let fe=0,Fe=me.length;fe<Fe;fe++)X[fe]=t.COLOR_ATTACHMENT0+fe;X.length=me.length,Q=!0}}else X[0]!==t.BACK&&(X[0]=t.BACK,Q=!0);Q&&t.drawBuffers(X)}function je(N){return v!==N?(t.useProgram(N),v=N,!0):!1}const Ue={[Mr]:t.FUNC_ADD,[PS]:t.FUNC_SUBTRACT,[DS]:t.FUNC_REVERSE_SUBTRACT};Ue[LS]=t.MIN,Ue[Uv]=t.MAX;const ot={[NS]:t.ZERO,[Ld]:t.ONE,[IS]:t.SRC_COLOR,[Nd]:t.SRC_ALPHA,[BS]:t.SRC_ALPHA_SATURATE,[OS]:t.DST_COLOR,[US]:t.DST_ALPHA,[kS]:t.ONE_MINUS_SRC_COLOR,[Id]:t.ONE_MINUS_SRC_ALPHA,[zS]:t.ONE_MINUS_DST_COLOR,[FS]:t.ONE_MINUS_DST_ALPHA,[HS]:t.CONSTANT_COLOR,[VS]:t.ONE_MINUS_CONSTANT_COLOR,[GS]:t.CONSTANT_ALPHA,[WS]:t.ONE_MINUS_CONSTANT_ALPHA};function I(N,ue,X,Q,me,fe,Fe,_t,Ft,et){if(N===nr){_===!0&&(Pe(t.BLEND),_=!1);return}if(_===!1&&(ae(t.BLEND),_=!0),N!==kv){if(N!==m||et!==w){if((u!==Mr||x!==Mr)&&(t.blendEquation(t.FUNC_ADD),u=Mr,x=Mr),et)switch(N){case Do:t.blendFuncSeparate(t.ONE,t.ONE_MINUS_SRC_ALPHA,t.ONE,t.ONE_MINUS_SRC_ALPHA);break;case em:t.blendFunc(t.ONE,t.ONE);break;case tm:t.blendFuncSeparate(t.ZERO,t.ONE_MINUS_SRC_COLOR,t.ZERO,t.ONE);break;case nm:t.blendFuncSeparate(t.ZERO,t.SRC_COLOR,t.ZERO,t.SRC_ALPHA);break;default:console.error("THREE.WebGLState: Invalid blending: ",N);break}else switch(N){case Do:t.blendFuncSeparate(t.SRC_ALPHA,t.ONE_MINUS_SRC_ALPHA,t.ONE,t.ONE_MINUS_SRC_ALPHA);break;case em:t.blendFunc(t.SRC_ALPHA,t.ONE);break;case tm:t.blendFuncSeparate(t.ZERO,t.ONE_MINUS_SRC_COLOR,t.ZERO,t.ONE);break;case nm:t.blendFunc(t.ZERO,t.SRC_COLOR);break;default:console.error("THREE.WebGLState: Invalid blending: ",N);break}g=null,y=null,C=null,A=null,T.set(0,0,0),b=0,m=N,w=et}return}me=me||ue,fe=fe||X,Fe=Fe||Q,(ue!==u||me!==x)&&(t.blendEquationSeparate(Ue[ue],Ue[me]),u=ue,x=me),(X!==g||Q!==y||fe!==C||Fe!==A)&&(t.blendFuncSeparate(ot[X],ot[Q],ot[fe],ot[Fe]),g=X,y=Q,C=fe,A=Fe),(_t.equals(T)===!1||Ft!==b)&&(t.blendColor(_t.r,_t.g,_t.b,Ft),T.copy(_t),b=Ft),m=N,w=!1}function Nt(N,ue){N.side===bn?Pe(t.CULL_FACE):ae(t.CULL_FACE);let X=N.side===Jt;ue&&(X=!X),Xe(X),N.blending===Do&&N.transparent===!1?I(nr):I(N.blending,N.blendEquation,N.blendSrc,N.blendDst,N.blendEquationAlpha,N.blendSrcAlpha,N.blendDstAlpha,N.blendColor,N.blendAlpha,N.premultipliedAlpha),s.setFunc(N.depthFunc),s.setTest(N.depthTest),s.setMask(N.depthWrite),o.setMask(N.colorWrite);const Q=N.stencilWrite;a.setTest(Q),Q&&(a.setMask(N.stencilWriteMask),a.setFunc(N.stencilFunc,N.stencilRef,N.stencilFuncMask),a.setOp(N.stencilFail,N.stencilZFail,N.stencilZPass)),nt(N.polygonOffset,N.polygonOffsetFactor,N.polygonOffsetUnits),N.alphaToCoverage===!0?ae(t.SAMPLE_ALPHA_TO_COVERAGE):Pe(t.SAMPLE_ALPHA_TO_COVERAGE)}function Xe(N){S!==N&&(N?t.frontFace(t.CW):t.frontFace(t.CCW),S=N)}function Ge(N){N!==CS?(ae(t.CULL_FACE),N!==P&&(N===Qp?t.cullFace(t.BACK):N===bS?t.cullFace(t.FRONT):t.cullFace(t.FRONT_AND_BACK))):Pe(t.CULL_FACE),P=N}function Me(N){N!==z&&(W&&t.lineWidth(N),z=N)}function nt(N,ue,X){N?(ae(t.POLYGON_OFFSET_FILL),(F!==ue||Y!==X)&&(t.polygonOffset(ue,X),F=ue,Y=X)):Pe(t.POLYGON_OFFSET_FILL)}function De(N){N?ae(t.SCISSOR_TEST):Pe(t.SCISSOR_TEST)}function R(N){N===void 0&&(N=t.TEXTURE0+j-1),G!==N&&(t.activeTexture(N),G=N)}function M(N,ue,X){X===void 0&&(G===null?X=t.TEXTURE0+j-1:X=G);let Q=Z[X];Q===void 0&&(Q={type:void 0,texture:void 0},Z[X]=Q),(Q.type!==N||Q.texture!==ue)&&(G!==X&&(t.activeTexture(X),G=X),t.bindTexture(N,ue||he[N]),Q.type=N,Q.texture=ue)}function O(){const N=Z[G];N!==void 0&&N.type!==void 0&&(t.bindTexture(N.type,null),N.type=void 0,N.texture=void 0)}function J(){try{t.compressedTexImage2D.apply(t,arguments)}catch(N){console.error("THREE.WebGLState:",N)}}function te(){try{t.compressedTexImage3D.apply(t,arguments)}catch(N){console.error("THREE.WebGLState:",N)}}function K(){try{t.texSubImage2D.apply(t,arguments)}catch(N){console.error("THREE.WebGLState:",N)}}function Ce(){try{t.texSubImage3D.apply(t,arguments)}catch(N){console.error("THREE.WebGLState:",N)}}function de(){try{t.compressedTexSubImage2D.apply(t,arguments)}catch(N){console.error("THREE.WebGLState:",N)}}function ye(){try{t.compressedTexSubImage3D.apply(t,arguments)}catch(N){console.error("THREE.WebGLState:",N)}}function qe(){try{t.texStorage2D.apply(t,arguments)}catch(N){console.error("THREE.WebGLState:",N)}}function oe(){try{t.texStorage3D.apply(t,arguments)}catch(N){console.error("THREE.WebGLState:",N)}}function _e(){try{t.texImage2D.apply(t,arguments)}catch(N){console.error("THREE.WebGLState:",N)}}function Le(){try{t.texImage3D.apply(t,arguments)}catch(N){console.error("THREE.WebGLState:",N)}}function Ie(N){Ne.equals(N)===!1&&(t.scissor(N.x,N.y,N.z,N.w),Ne.copy(N))}function xe(N){q.equals(N)===!1&&(t.viewport(N.x,N.y,N.z,N.w),q.copy(N))}function Ye(N,ue){let X=c.get(ue);X===void 0&&(X=new WeakMap,c.set(ue,X));let Q=X.get(N);Q===void 0&&(Q=t.getUniformBlockIndex(ue,N.name),X.set(N,Q))}function He(N,ue){const Q=c.get(ue).get(N);l.get(ue)!==Q&&(t.uniformBlockBinding(ue,Q,N.__bindingPointIndex),l.set(ue,Q))}function st(){t.disable(t.BLEND),t.disable(t.CULL_FACE),t.disable(t.DEPTH_TEST),t.disable(t.POLYGON_OFFSET_FILL),t.disable(t.SCISSOR_TEST),t.disable(t.STENCIL_TEST),t.disable(t.SAMPLE_ALPHA_TO_COVERAGE),t.blendEquation(t.FUNC_ADD),t.blendFunc(t.ONE,t.ZERO),t.blendFuncSeparate(t.ONE,t.ZERO,t.ONE,t.ZERO),t.blendColor(0,0,0,0),t.colorMask(!0,!0,!0,!0),t.clearColor(0,0,0,0),t.depthMask(!0),t.depthFunc(t.LESS),s.setReversed(!1),t.clearDepth(1),t.stencilMask(4294967295),t.stencilFunc(t.ALWAYS,0,4294967295),t.stencilOp(t.KEEP,t.KEEP,t.KEEP),t.clearStencil(0),t.cullFace(t.BACK),t.frontFace(t.CCW),t.polygonOffset(0,0),t.activeTexture(t.TEXTURE0),t.bindFramebuffer(t.FRAMEBUFFER,null),t.bindFramebuffer(t.DRAW_FRAMEBUFFER,null),t.bindFramebuffer(t.READ_FRAMEBUFFER,null),t.useProgram(null),t.lineWidth(1),t.scissor(0,0,t.canvas.width,t.canvas.height),t.viewport(0,0,t.canvas.width,t.canvas.height),d={},G=null,Z={},f={},h=new WeakMap,p=[],v=null,_=!1,m=null,u=null,g=null,y=null,x=null,C=null,A=null,T=new we(0,0,0),b=0,w=!1,S=null,P=null,z=null,F=null,Y=null,Ne.set(0,0,t.canvas.width,t.canvas.height),q.set(0,0,t.canvas.width,t.canvas.height),o.reset(),s.reset(),a.reset()}return{buffers:{color:o,depth:s,stencil:a},enable:ae,disable:Pe,bindFramebuffer:ne,drawBuffers:ve,useProgram:je,setBlending:I,setMaterial:Nt,setFlipSided:Xe,setCullFace:Ge,setLineWidth:Me,setPolygonOffset:nt,setScissorTest:De,activeTexture:R,bindTexture:M,unbindTexture:O,compressedTexImage2D:J,compressedTexImage3D:te,texImage2D:_e,texImage3D:Le,updateUBOMapping:Ye,uniformBlockBinding:He,texStorage2D:qe,texStorage3D:oe,texSubImage2D:K,texSubImage3D:Ce,compressedTexSubImage2D:de,compressedTexSubImage3D:ye,scissor:Ie,viewport:xe,reset:st}}function $m(t,e,n,i){const r=fA(i);switch(n){case Gv:return t*e;case Xv:return t*e;case jv:return t*e*2;case Mf:return t*e/r.components*r.byteLength;case wf:return t*e/r.components*r.byteLength;case Yv:return t*e*2/r.components*r.byteLength;case Ef:return t*e*2/r.components*r.byteLength;case Wv:return t*e*3/r.components*r.byteLength;case Wt:return t*e*4/r.components*r.byteLength;case Tf:return t*e*4/r.components*r.byteLength;case Sl:case Ml:return Math.floor((t+3)/4)*Math.floor((e+3)/4)*8;case wl:case El:return Math.floor((t+3)/4)*Math.floor((e+3)/4)*16;case jd:case qd:return Math.max(t,16)*Math.max(e,8)/4;case Xd:case Yd:return Math.max(t,8)*Math.max(e,8)/2;case $d:case Kd:return Math.floor((t+3)/4)*Math.floor((e+3)/4)*8;case Zd:return Math.floor((t+3)/4)*Math.floor((e+3)/4)*16;case Jd:return Math.floor((t+3)/4)*Math.floor((e+3)/4)*16;case Qd:return Math.floor((t+4)/5)*Math.floor((e+3)/4)*16;case eh:return Math.floor((t+4)/5)*Math.floor((e+4)/5)*16;case th:return Math.floor((t+5)/6)*Math.floor((e+4)/5)*16;case nh:return Math.floor((t+5)/6)*Math.floor((e+5)/6)*16;case ih:return Math.floor((t+7)/8)*Math.floor((e+4)/5)*16;case rh:return Math.floor((t+7)/8)*Math.floor((e+5)/6)*16;case oh:return Math.floor((t+7)/8)*Math.floor((e+7)/8)*16;case sh:return Math.floor((t+9)/10)*Math.floor((e+4)/5)*16;case ah:return Math.floor((t+9)/10)*Math.floor((e+5)/6)*16;case lh:return Math.floor((t+9)/10)*Math.floor((e+7)/8)*16;case ch:return Math.floor((t+9)/10)*Math.floor((e+9)/10)*16;case uh:return Math.floor((t+11)/12)*Math.floor((e+9)/10)*16;case dh:return Math.floor((t+11)/12)*Math.floor((e+11)/12)*16;case Tl:case hh:case fh:return Math.ceil(t/4)*Math.ceil(e/4)*16;case qv:case ph:return Math.ceil(t/4)*Math.ceil(e/4)*8;case mh:case gh:return Math.ceil(t/4)*Math.ceil(e/4)*16}throw new Error(`Unable to determine texture byte length for ${n} format.`)}function fA(t){switch(t){case oi:case Bv:return{byteLength:1,components:1};case aa:case Hv:case Zo:return{byteLength:2,components:1};case xf:case Sf:return{byteLength:2,components:4};case Fr:case _f:case Pn:return{byteLength:4,components:1};case Vv:return{byteLength:4,components:3}}throw new Error(`Unknown texture type ${t}.`)}function pA(t,e,n,i,r,o,s){const a=e.has("WEBGL_multisampled_render_to_texture")?e.get("WEBGL_multisampled_render_to_texture"):null,l=typeof navigator>"u"?!1:/OculusBrowser/g.test(navigator.userAgent),c=new Re,d=new WeakMap;let f;const h=new WeakMap;let p=!1;try{p=typeof OffscreenCanvas<"u"&&new OffscreenCanvas(1,1).getContext("2d")!==null}catch{}function v(R,M){return p?new OffscreenCanvas(R,M):Ql("canvas")}function _(R,M,O){let J=1;const te=De(R);if((te.width>O||te.height>O)&&(J=O/Math.max(te.width,te.height)),J<1)if(typeof HTMLImageElement<"u"&&R instanceof HTMLImageElement||typeof HTMLCanvasElement<"u"&&R instanceof HTMLCanvasElement||typeof ImageBitmap<"u"&&R instanceof ImageBitmap||typeof VideoFrame<"u"&&R instanceof VideoFrame){const K=Math.floor(J*te.width),Ce=Math.floor(J*te.height);f===void 0&&(f=v(K,Ce));const de=M?v(K,Ce):f;return de.width=K,de.height=Ce,de.getContext("2d").drawImage(R,0,0,K,Ce),console.warn("THREE.WebGLRenderer: Texture has been resized from ("+te.width+"x"+te.height+") to ("+K+"x"+Ce+")."),de}else return"data"in R&&console.warn("THREE.WebGLRenderer: Image in DataTexture is too big ("+te.width+"x"+te.height+")."),R;return R}function m(R){return R.generateMipmaps}function u(R){t.generateMipmap(R)}function g(R){return R.isWebGLCubeRenderTarget?t.TEXTURE_CUBE_MAP:R.isWebGL3DRenderTarget?t.TEXTURE_3D:R.isWebGLArrayRenderTarget||R.isCompressedArrayTexture?t.TEXTURE_2D_ARRAY:t.TEXTURE_2D}function y(R,M,O,J,te=!1){if(R!==null){if(t[R]!==void 0)return t[R];console.warn("THREE.WebGLRenderer: Attempt to use non-existing WebGL internal format '"+R+"'")}let K=M;if(M===t.RED&&(O===t.FLOAT&&(K=t.R32F),O===t.HALF_FLOAT&&(K=t.R16F),O===t.UNSIGNED_BYTE&&(K=t.R8)),M===t.RED_INTEGER&&(O===t.UNSIGNED_BYTE&&(K=t.R8UI),O===t.UNSIGNED_SHORT&&(K=t.R16UI),O===t.UNSIGNED_INT&&(K=t.R32UI),O===t.BYTE&&(K=t.R8I),O===t.SHORT&&(K=t.R16I),O===t.INT&&(K=t.R32I)),M===t.RG&&(O===t.FLOAT&&(K=t.RG32F),O===t.HALF_FLOAT&&(K=t.RG16F),O===t.UNSIGNED_BYTE&&(K=t.RG8)),M===t.RG_INTEGER&&(O===t.UNSIGNED_BYTE&&(K=t.RG8UI),O===t.UNSIGNED_SHORT&&(K=t.RG16UI),O===t.UNSIGNED_INT&&(K=t.RG32UI),O===t.BYTE&&(K=t.RG8I),O===t.SHORT&&(K=t.RG16I),O===t.INT&&(K=t.RG32I)),M===t.RGB_INTEGER&&(O===t.UNSIGNED_BYTE&&(K=t.RGB8UI),O===t.UNSIGNED_SHORT&&(K=t.RGB16UI),O===t.UNSIGNED_INT&&(K=t.RGB32UI),O===t.BYTE&&(K=t.RGB8I),O===t.SHORT&&(K=t.RGB16I),O===t.INT&&(K=t.RGB32I)),M===t.RGBA_INTEGER&&(O===t.UNSIGNED_BYTE&&(K=t.RGBA8UI),O===t.UNSIGNED_SHORT&&(K=t.RGBA16UI),O===t.UNSIGNED_INT&&(K=t.RGBA32UI),O===t.BYTE&&(K=t.RGBA8I),O===t.SHORT&&(K=t.RGBA16I),O===t.INT&&(K=t.RGBA32I)),M===t.RGB&&O===t.UNSIGNED_INT_5_9_9_9_REV&&(K=t.RGB9_E5),M===t.RGBA){const Ce=te?Mc:$e.getTransfer(J);O===t.FLOAT&&(K=t.RGBA32F),O===t.HALF_FLOAT&&(K=t.RGBA16F),O===t.UNSIGNED_BYTE&&(K=Ce===it?t.SRGB8_ALPHA8:t.RGBA8),O===t.UNSIGNED_SHORT_4_4_4_4&&(K=t.RGBA4),O===t.UNSIGNED_SHORT_5_5_5_1&&(K=t.RGB5_A1)}return(K===t.R16F||K===t.R32F||K===t.RG16F||K===t.RG32F||K===t.RGBA16F||K===t.RGBA32F)&&e.get("EXT_color_buffer_float"),K}function x(R,M){let O;return R?M===null||M===Fr||M===Xo?O=t.DEPTH24_STENCIL8:M===Pn?O=t.DEPTH32F_STENCIL8:M===aa&&(O=t.DEPTH24_STENCIL8,console.warn("DepthTexture: 16 bit depth attachment is not supported with stencil. Using 24-bit attachment.")):M===null||M===Fr||M===Xo?O=t.DEPTH_COMPONENT24:M===Pn?O=t.DEPTH_COMPONENT32F:M===aa&&(O=t.DEPTH_COMPONENT16),O}function C(R,M){return m(R)===!0||R.isFramebufferTexture&&R.minFilter!==dn&&R.minFilter!==Kt?Math.log2(Math.max(M.width,M.height))+1:R.mipmaps!==void 0&&R.mipmaps.length>0?R.mipmaps.length:R.isCompressedTexture&&Array.isArray(R.image)?M.mipmaps.length:1}function A(R){const M=R.target;M.removeEventListener("dispose",A),b(M),M.isVideoTexture&&d.delete(M)}function T(R){const M=R.target;M.removeEventListener("dispose",T),S(M)}function b(R){const M=i.get(R);if(M.__webglInit===void 0)return;const O=R.source,J=h.get(O);if(J){const te=J[M.__cacheKey];te.usedTimes--,te.usedTimes===0&&w(R),Object.keys(J).length===0&&h.delete(O)}i.remove(R)}function w(R){const M=i.get(R);t.deleteTexture(M.__webglTexture);const O=R.source,J=h.get(O);delete J[M.__cacheKey],s.memory.textures--}function S(R){const M=i.get(R);if(R.depthTexture&&(R.depthTexture.dispose(),i.remove(R.depthTexture)),R.isWebGLCubeRenderTarget)for(let J=0;J<6;J++){if(Array.isArray(M.__webglFramebuffer[J]))for(let te=0;te<M.__webglFramebuffer[J].length;te++)t.deleteFramebuffer(M.__webglFramebuffer[J][te]);else t.deleteFramebuffer(M.__webglFramebuffer[J]);M.__webglDepthbuffer&&t.deleteRenderbuffer(M.__webglDepthbuffer[J])}else{if(Array.isArray(M.__webglFramebuffer))for(let J=0;J<M.__webglFramebuffer.length;J++)t.deleteFramebuffer(M.__webglFramebuffer[J]);else t.deleteFramebuffer(M.__webglFramebuffer);if(M.__webglDepthbuffer&&t.deleteRenderbuffer(M.__webglDepthbuffer),M.__webglMultisampledFramebuffer&&t.deleteFramebuffer(M.__webglMultisampledFramebuffer),M.__webglColorRenderbuffer)for(let J=0;J<M.__webglColorRenderbuffer.length;J++)M.__webglColorRenderbuffer[J]&&t.deleteRenderbuffer(M.__webglColorRenderbuffer[J]);M.__webglDepthRenderbuffer&&t.deleteRenderbuffer(M.__webglDepthRenderbuffer)}const O=R.textures;for(let J=0,te=O.length;J<te;J++){const K=i.get(O[J]);K.__webglTexture&&(t.deleteTexture(K.__webglTexture),s.memory.textures--),i.remove(O[J])}i.remove(R)}let P=0;function z(){P=0}function F(){const R=P;return R>=r.maxTextures&&console.warn("THREE.WebGLTextures: Trying to use "+R+" texture units while this GPU supports only "+r.maxTextures),P+=1,R}function Y(R){const M=[];return M.push(R.wrapS),M.push(R.wrapT),M.push(R.wrapR||0),M.push(R.magFilter),M.push(R.minFilter),M.push(R.anisotropy),M.push(R.internalFormat),M.push(R.format),M.push(R.type),M.push(R.generateMipmaps),M.push(R.premultiplyAlpha),M.push(R.flipY),M.push(R.unpackAlignment),M.push(R.colorSpace),M.join()}function j(R,M){const O=i.get(R);if(R.isVideoTexture&&Me(R),R.isRenderTargetTexture===!1&&R.version>0&&O.__version!==R.version){const J=R.image;if(J===null)console.warn("THREE.WebGLRenderer: Texture marked for update but no image data found.");else if(J.complete===!1)console.warn("THREE.WebGLRenderer: Texture marked for update but image is incomplete");else{q(O,R,M);return}}n.bindTexture(t.TEXTURE_2D,O.__webglTexture,t.TEXTURE0+M)}function W(R,M){const O=i.get(R);if(R.version>0&&O.__version!==R.version){q(O,R,M);return}n.bindTexture(t.TEXTURE_2D_ARRAY,O.__webglTexture,t.TEXTURE0+M)}function $(R,M){const O=i.get(R);if(R.version>0&&O.__version!==R.version){q(O,R,M);return}n.bindTexture(t.TEXTURE_3D,O.__webglTexture,t.TEXTURE0+M)}function D(R,M){const O=i.get(R);if(R.version>0&&O.__version!==R.version){re(O,R,M);return}n.bindTexture(t.TEXTURE_CUBE_MAP,O.__webglTexture,t.TEXTURE0+M)}const G={[Wo]:t.REPEAT,[Cr]:t.CLAMP_TO_EDGE,[Wd]:t.MIRRORED_REPEAT},Z={[dn]:t.NEAREST,[QS]:t.NEAREST_MIPMAP_NEAREST,[za]:t.NEAREST_MIPMAP_LINEAR,[Kt]:t.LINEAR,[tu]:t.LINEAR_MIPMAP_NEAREST,[ni]:t.LINEAR_MIPMAP_LINEAR},le={[iM]:t.NEVER,[cM]:t.ALWAYS,[rM]:t.LESS,[Kv]:t.LEQUAL,[oM]:t.EQUAL,[lM]:t.GEQUAL,[sM]:t.GREATER,[aM]:t.NOTEQUAL};function Se(R,M){if(M.type===Pn&&e.has("OES_texture_float_linear")===!1&&(M.magFilter===Kt||M.magFilter===tu||M.magFilter===za||M.magFilter===ni||M.minFilter===Kt||M.minFilter===tu||M.minFilter===za||M.minFilter===ni)&&console.warn("THREE.WebGLRenderer: Unable to use linear filtering with floating point textures. OES_texture_float_linear not supported on this device."),t.texParameteri(R,t.TEXTURE_WRAP_S,G[M.wrapS]),t.texParameteri(R,t.TEXTURE_WRAP_T,G[M.wrapT]),(R===t.TEXTURE_3D||R===t.TEXTURE_2D_ARRAY)&&t.texParameteri(R,t.TEXTURE_WRAP_R,G[M.wrapR]),t.texParameteri(R,t.TEXTURE_MAG_FILTER,Z[M.magFilter]),t.texParameteri(R,t.TEXTURE_MIN_FILTER,Z[M.minFilter]),M.compareFunction&&(t.texParameteri(R,t.TEXTURE_COMPARE_MODE,t.COMPARE_REF_TO_TEXTURE),t.texParameteri(R,t.TEXTURE_COMPARE_FUNC,le[M.compareFunction])),e.has("EXT_texture_filter_anisotropic")===!0){if(M.magFilter===dn||M.minFilter!==za&&M.minFilter!==ni||M.type===Pn&&e.has("OES_texture_float_linear")===!1)return;if(M.anisotropy>1||i.get(M).__currentAnisotropy){const O=e.get("EXT_texture_filter_anisotropic");t.texParameterf(R,O.TEXTURE_MAX_ANISOTROPY_EXT,Math.min(M.anisotropy,r.getMaxAnisotropy())),i.get(M).__currentAnisotropy=M.anisotropy}}}function Ne(R,M){let O=!1;R.__webglInit===void 0&&(R.__webglInit=!0,M.addEventListener("dispose",A));const J=M.source;let te=h.get(J);te===void 0&&(te={},h.set(J,te));const K=Y(M);if(K!==R.__cacheKey){te[K]===void 0&&(te[K]={texture:t.createTexture(),usedTimes:0},s.memory.textures++,O=!0),te[K].usedTimes++;const Ce=te[R.__cacheKey];Ce!==void 0&&(te[R.__cacheKey].usedTimes--,Ce.usedTimes===0&&w(M)),R.__cacheKey=K,R.__webglTexture=te[K].texture}return O}function q(R,M,O){let J=t.TEXTURE_2D;(M.isDataArrayTexture||M.isCompressedArrayTexture)&&(J=t.TEXTURE_2D_ARRAY),M.isData3DTexture&&(J=t.TEXTURE_3D);const te=Ne(R,M),K=M.source;n.bindTexture(J,R.__webglTexture,t.TEXTURE0+O);const Ce=i.get(K);if(K.version!==Ce.__version||te===!0){n.activeTexture(t.TEXTURE0+O);const de=$e.getPrimaries($e.workingColorSpace),ye=M.colorSpace===Hi?null:$e.getPrimaries(M.colorSpace),qe=M.colorSpace===Hi||de===ye?t.NONE:t.BROWSER_DEFAULT_WEBGL;t.pixelStorei(t.UNPACK_FLIP_Y_WEBGL,M.flipY),t.pixelStorei(t.UNPACK_PREMULTIPLY_ALPHA_WEBGL,M.premultiplyAlpha),t.pixelStorei(t.UNPACK_ALIGNMENT,M.unpackAlignment),t.pixelStorei(t.UNPACK_COLORSPACE_CONVERSION_WEBGL,qe);let oe=_(M.image,!1,r.maxTextureSize);oe=nt(M,oe);const _e=o.convert(M.format,M.colorSpace),Le=o.convert(M.type);let Ie=y(M.internalFormat,_e,Le,M.colorSpace,M.isVideoTexture);Se(J,M);let xe;const Ye=M.mipmaps,He=M.isVideoTexture!==!0,st=Ce.__version===void 0||te===!0,N=K.dataReady,ue=C(M,oe);if(M.isDepthTexture)Ie=x(M.format===jo,M.type),st&&(He?n.texStorage2D(t.TEXTURE_2D,1,Ie,oe.width,oe.height):n.texImage2D(t.TEXTURE_2D,0,Ie,oe.width,oe.height,0,_e,Le,null));else if(M.isDataTexture)if(Ye.length>0){He&&st&&n.texStorage2D(t.TEXTURE_2D,ue,Ie,Ye[0].width,Ye[0].height);for(let X=0,Q=Ye.length;X<Q;X++)xe=Ye[X],He?N&&n.texSubImage2D(t.TEXTURE_2D,X,0,0,xe.width,xe.height,_e,Le,xe.data):n.texImage2D(t.TEXTURE_2D,X,Ie,xe.width,xe.height,0,_e,Le,xe.data);M.generateMipmaps=!1}else He?(st&&n.texStorage2D(t.TEXTURE_2D,ue,Ie,oe.width,oe.height),N&&n.texSubImage2D(t.TEXTURE_2D,0,0,0,oe.width,oe.height,_e,Le,oe.data)):n.texImage2D(t.TEXTURE_2D,0,Ie,oe.width,oe.height,0,_e,Le,oe.data);else if(M.isCompressedTexture)if(M.isCompressedArrayTexture){He&&st&&n.texStorage3D(t.TEXTURE_2D_ARRAY,ue,Ie,Ye[0].width,Ye[0].height,oe.depth);for(let X=0,Q=Ye.length;X<Q;X++)if(xe=Ye[X],M.format!==Wt)if(_e!==null)if(He){if(N)if(M.layerUpdates.size>0){const me=$m(xe.width,xe.height,M.format,M.type);for(const fe of M.layerUpdates){const Fe=xe.data.subarray(fe*me/xe.data.BYTES_PER_ELEMENT,(fe+1)*me/xe.data.BYTES_PER_ELEMENT);n.compressedTexSubImage3D(t.TEXTURE_2D_ARRAY,X,0,0,fe,xe.width,xe.height,1,_e,Fe)}M.clearLayerUpdates()}else n.compressedTexSubImage3D(t.TEXTURE_2D_ARRAY,X,0,0,0,xe.width,xe.height,oe.depth,_e,xe.data)}else n.compressedTexImage3D(t.TEXTURE_2D_ARRAY,X,Ie,xe.width,xe.height,oe.depth,0,xe.data,0,0);else console.warn("THREE.WebGLRenderer: Attempt to load unsupported compressed texture format in .uploadTexture()");else He?N&&n.texSubImage3D(t.TEXTURE_2D_ARRAY,X,0,0,0,xe.width,xe.height,oe.depth,_e,Le,xe.data):n.texImage3D(t.TEXTURE_2D_ARRAY,X,Ie,xe.width,xe.height,oe.depth,0,_e,Le,xe.data)}else{He&&st&&n.texStorage2D(t.TEXTURE_2D,ue,Ie,Ye[0].width,Ye[0].height);for(let X=0,Q=Ye.length;X<Q;X++)xe=Ye[X],M.format!==Wt?_e!==null?He?N&&n.compressedTexSubImage2D(t.TEXTURE_2D,X,0,0,xe.width,xe.height,_e,xe.data):n.compressedTexImage2D(t.TEXTURE_2D,X,Ie,xe.width,xe.height,0,xe.data):console.warn("THREE.WebGLRenderer: Attempt to load unsupported compressed texture format in .uploadTexture()"):He?N&&n.texSubImage2D(t.TEXTURE_2D,X,0,0,xe.width,xe.height,_e,Le,xe.data):n.texImage2D(t.TEXTURE_2D,X,Ie,xe.width,xe.height,0,_e,Le,xe.data)}else if(M.isDataArrayTexture)if(He){if(st&&n.texStorage3D(t.TEXTURE_2D_ARRAY,ue,Ie,oe.width,oe.height,oe.depth),N)if(M.layerUpdates.size>0){const X=$m(oe.width,oe.height,M.format,M.type);for(const Q of M.layerUpdates){const me=oe.data.subarray(Q*X/oe.data.BYTES_PER_ELEMENT,(Q+1)*X/oe.data.BYTES_PER_ELEMENT);n.texSubImage3D(t.TEXTURE_2D_ARRAY,0,0,0,Q,oe.width,oe.height,1,_e,Le,me)}M.clearLayerUpdates()}else n.texSubImage3D(t.TEXTURE_2D_ARRAY,0,0,0,0,oe.width,oe.height,oe.depth,_e,Le,oe.data)}else n.texImage3D(t.TEXTURE_2D_ARRAY,0,Ie,oe.width,oe.height,oe.depth,0,_e,Le,oe.data);else if(M.isData3DTexture)He?(st&&n.texStorage3D(t.TEXTURE_3D,ue,Ie,oe.width,oe.height,oe.depth),N&&n.texSubImage3D(t.TEXTURE_3D,0,0,0,0,oe.width,oe.height,oe.depth,_e,Le,oe.data)):n.texImage3D(t.TEXTURE_3D,0,Ie,oe.width,oe.height,oe.depth,0,_e,Le,oe.data);else if(M.isFramebufferTexture){if(st)if(He)n.texStorage2D(t.TEXTURE_2D,ue,Ie,oe.width,oe.height);else{let X=oe.width,Q=oe.height;for(let me=0;me<ue;me++)n.texImage2D(t.TEXTURE_2D,me,Ie,X,Q,0,_e,Le,null),X>>=1,Q>>=1}}else if(Ye.length>0){if(He&&st){const X=De(Ye[0]);n.texStorage2D(t.TEXTURE_2D,ue,Ie,X.width,X.height)}for(let X=0,Q=Ye.length;X<Q;X++)xe=Ye[X],He?N&&n.texSubImage2D(t.TEXTURE_2D,X,0,0,_e,Le,xe):n.texImage2D(t.TEXTURE_2D,X,Ie,_e,Le,xe);M.generateMipmaps=!1}else if(He){if(st){const X=De(oe);n.texStorage2D(t.TEXTURE_2D,ue,Ie,X.width,X.height)}N&&n.texSubImage2D(t.TEXTURE_2D,0,0,0,_e,Le,oe)}else n.texImage2D(t.TEXTURE_2D,0,Ie,_e,Le,oe);m(M)&&u(J),Ce.__version=K.version,M.onUpdate&&M.onUpdate(M)}R.__version=M.version}function re(R,M,O){if(M.image.length!==6)return;const J=Ne(R,M),te=M.source;n.bindTexture(t.TEXTURE_CUBE_MAP,R.__webglTexture,t.TEXTURE0+O);const K=i.get(te);if(te.version!==K.__version||J===!0){n.activeTexture(t.TEXTURE0+O);const Ce=$e.getPrimaries($e.workingColorSpace),de=M.colorSpace===Hi?null:$e.getPrimaries(M.colorSpace),ye=M.colorSpace===Hi||Ce===de?t.NONE:t.BROWSER_DEFAULT_WEBGL;t.pixelStorei(t.UNPACK_FLIP_Y_WEBGL,M.flipY),t.pixelStorei(t.UNPACK_PREMULTIPLY_ALPHA_WEBGL,M.premultiplyAlpha),t.pixelStorei(t.UNPACK_ALIGNMENT,M.unpackAlignment),t.pixelStorei(t.UNPACK_COLORSPACE_CONVERSION_WEBGL,ye);const qe=M.isCompressedTexture||M.image[0].isCompressedTexture,oe=M.image[0]&&M.image[0].isDataTexture,_e=[];for(let Q=0;Q<6;Q++)!qe&&!oe?_e[Q]=_(M.image[Q],!0,r.maxCubemapSize):_e[Q]=oe?M.image[Q].image:M.image[Q],_e[Q]=nt(M,_e[Q]);const Le=_e[0],Ie=o.convert(M.format,M.colorSpace),xe=o.convert(M.type),Ye=y(M.internalFormat,Ie,xe,M.colorSpace),He=M.isVideoTexture!==!0,st=K.__version===void 0||J===!0,N=te.dataReady;let ue=C(M,Le);Se(t.TEXTURE_CUBE_MAP,M);let X;if(qe){He&&st&&n.texStorage2D(t.TEXTURE_CUBE_MAP,ue,Ye,Le.width,Le.height);for(let Q=0;Q<6;Q++){X=_e[Q].mipmaps;for(let me=0;me<X.length;me++){const fe=X[me];M.format!==Wt?Ie!==null?He?N&&n.compressedTexSubImage2D(t.TEXTURE_CUBE_MAP_POSITIVE_X+Q,me,0,0,fe.width,fe.height,Ie,fe.data):n.compressedTexImage2D(t.TEXTURE_CUBE_MAP_POSITIVE_X+Q,me,Ye,fe.width,fe.height,0,fe.data):console.warn("THREE.WebGLRenderer: Attempt to load unsupported compressed texture format in .setTextureCube()"):He?N&&n.texSubImage2D(t.TEXTURE_CUBE_MAP_POSITIVE_X+Q,me,0,0,fe.width,fe.height,Ie,xe,fe.data):n.texImage2D(t.TEXTURE_CUBE_MAP_POSITIVE_X+Q,me,Ye,fe.width,fe.height,0,Ie,xe,fe.data)}}}else{if(X=M.mipmaps,He&&st){X.length>0&&ue++;const Q=De(_e[0]);n.texStorage2D(t.TEXTURE_CUBE_MAP,ue,Ye,Q.width,Q.height)}for(let Q=0;Q<6;Q++)if(oe){He?N&&n.texSubImage2D(t.TEXTURE_CUBE_MAP_POSITIVE_X+Q,0,0,0,_e[Q].width,_e[Q].height,Ie,xe,_e[Q].data):n.texImage2D(t.TEXTURE_CUBE_MAP_POSITIVE_X+Q,0,Ye,_e[Q].width,_e[Q].height,0,Ie,xe,_e[Q].data);for(let me=0;me<X.length;me++){const Fe=X[me].image[Q].image;He?N&&n.texSubImage2D(t.TEXTURE_CUBE_MAP_POSITIVE_X+Q,me+1,0,0,Fe.width,Fe.height,Ie,xe,Fe.data):n.texImage2D(t.TEXTURE_CUBE_MAP_POSITIVE_X+Q,me+1,Ye,Fe.width,Fe.height,0,Ie,xe,Fe.data)}}else{He?N&&n.texSubImage2D(t.TEXTURE_CUBE_MAP_POSITIVE_X+Q,0,0,0,Ie,xe,_e[Q]):n.texImage2D(t.TEXTURE_CUBE_MAP_POSITIVE_X+Q,0,Ye,Ie,xe,_e[Q]);for(let me=0;me<X.length;me++){const fe=X[me];He?N&&n.texSubImage2D(t.TEXTURE_CUBE_MAP_POSITIVE_X+Q,me+1,0,0,Ie,xe,fe.image[Q]):n.texImage2D(t.TEXTURE_CUBE_MAP_POSITIVE_X+Q,me+1,Ye,Ie,xe,fe.image[Q])}}}m(M)&&u(t.TEXTURE_CUBE_MAP),K.__version=te.version,M.onUpdate&&M.onUpdate(M)}R.__version=M.version}function he(R,M,O,J,te,K){const Ce=o.convert(O.format,O.colorSpace),de=o.convert(O.type),ye=y(O.internalFormat,Ce,de,O.colorSpace),qe=i.get(M),oe=i.get(O);if(oe.__renderTarget=M,!qe.__hasExternalTextures){const _e=Math.max(1,M.width>>K),Le=Math.max(1,M.height>>K);te===t.TEXTURE_3D||te===t.TEXTURE_2D_ARRAY?n.texImage3D(te,K,ye,_e,Le,M.depth,0,Ce,de,null):n.texImage2D(te,K,ye,_e,Le,0,Ce,de,null)}n.bindFramebuffer(t.FRAMEBUFFER,R),Ge(M)?a.framebufferTexture2DMultisampleEXT(t.FRAMEBUFFER,J,te,oe.__webglTexture,0,Xe(M)):(te===t.TEXTURE_2D||te>=t.TEXTURE_CUBE_MAP_POSITIVE_X&&te<=t.TEXTURE_CUBE_MAP_NEGATIVE_Z)&&t.framebufferTexture2D(t.FRAMEBUFFER,J,te,oe.__webglTexture,K),n.bindFramebuffer(t.FRAMEBUFFER,null)}function ae(R,M,O){if(t.bindRenderbuffer(t.RENDERBUFFER,R),M.depthBuffer){const J=M.depthTexture,te=J&&J.isDepthTexture?J.type:null,K=x(M.stencilBuffer,te),Ce=M.stencilBuffer?t.DEPTH_STENCIL_ATTACHMENT:t.DEPTH_ATTACHMENT,de=Xe(M);Ge(M)?a.renderbufferStorageMultisampleEXT(t.RENDERBUFFER,de,K,M.width,M.height):O?t.renderbufferStorageMultisample(t.RENDERBUFFER,de,K,M.width,M.height):t.renderbufferStorage(t.RENDERBUFFER,K,M.width,M.height),t.framebufferRenderbuffer(t.FRAMEBUFFER,Ce,t.RENDERBUFFER,R)}else{const J=M.textures;for(let te=0;te<J.length;te++){const K=J[te],Ce=o.convert(K.format,K.colorSpace),de=o.convert(K.type),ye=y(K.internalFormat,Ce,de,K.colorSpace),qe=Xe(M);O&&Ge(M)===!1?t.renderbufferStorageMultisample(t.RENDERBUFFER,qe,ye,M.width,M.height):Ge(M)?a.renderbufferStorageMultisampleEXT(t.RENDERBUFFER,qe,ye,M.width,M.height):t.renderbufferStorage(t.RENDERBUFFER,ye,M.width,M.height)}}t.bindRenderbuffer(t.RENDERBUFFER,null)}function Pe(R,M){if(M&&M.isWebGLCubeRenderTarget)throw new Error("Depth Texture with cube render targets is not supported");if(n.bindFramebuffer(t.FRAMEBUFFER,R),!(M.depthTexture&&M.depthTexture.isDepthTexture))throw new Error("renderTarget.depthTexture must be an instance of THREE.DepthTexture");const J=i.get(M.depthTexture);J.__renderTarget=M,(!J.__webglTexture||M.depthTexture.image.width!==M.width||M.depthTexture.image.height!==M.height)&&(M.depthTexture.image.width=M.width,M.depthTexture.image.height=M.height,M.depthTexture.needsUpdate=!0),j(M.depthTexture,0);const te=J.__webglTexture,K=Xe(M);if(M.depthTexture.format===Lo)Ge(M)?a.framebufferTexture2DMultisampleEXT(t.FRAMEBUFFER,t.DEPTH_ATTACHMENT,t.TEXTURE_2D,te,0,K):t.framebufferTexture2D(t.FRAMEBUFFER,t.DEPTH_ATTACHMENT,t.TEXTURE_2D,te,0);else if(M.depthTexture.format===jo)Ge(M)?a.framebufferTexture2DMultisampleEXT(t.FRAMEBUFFER,t.DEPTH_STENCIL_ATTACHMENT,t.TEXTURE_2D,te,0,K):t.framebufferTexture2D(t.FRAMEBUFFER,t.DEPTH_STENCIL_ATTACHMENT,t.TEXTURE_2D,te,0);else throw new Error("Unknown depthTexture format")}function ne(R){const M=i.get(R),O=R.isWebGLCubeRenderTarget===!0;if(M.__boundDepthTexture!==R.depthTexture){const J=R.depthTexture;if(M.__depthDisposeCallback&&M.__depthDisposeCallback(),J){const te=()=>{delete M.__boundDepthTexture,delete M.__depthDisposeCallback,J.removeEventListener("dispose",te)};J.addEventListener("dispose",te),M.__depthDisposeCallback=te}M.__boundDepthTexture=J}if(R.depthTexture&&!M.__autoAllocateDepthBuffer){if(O)throw new Error("target.depthTexture not supported in Cube render targets");Pe(M.__webglFramebuffer,R)}else if(O){M.__webglDepthbuffer=[];for(let J=0;J<6;J++)if(n.bindFramebuffer(t.FRAMEBUFFER,M.__webglFramebuffer[J]),M.__webglDepthbuffer[J]===void 0)M.__webglDepthbuffer[J]=t.createRenderbuffer(),ae(M.__webglDepthbuffer[J],R,!1);else{const te=R.stencilBuffer?t.DEPTH_STENCIL_ATTACHMENT:t.DEPTH_ATTACHMENT,K=M.__webglDepthbuffer[J];t.bindRenderbuffer(t.RENDERBUFFER,K),t.framebufferRenderbuffer(t.FRAMEBUFFER,te,t.RENDERBUFFER,K)}}else if(n.bindFramebuffer(t.FRAMEBUFFER,M.__webglFramebuffer),M.__webglDepthbuffer===void 0)M.__webglDepthbuffer=t.createRenderbuffer(),ae(M.__webglDepthbuffer,R,!1);else{const J=R.stencilBuffer?t.DEPTH_STENCIL_ATTACHMENT:t.DEPTH_ATTACHMENT,te=M.__webglDepthbuffer;t.bindRenderbuffer(t.RENDERBUFFER,te),t.framebufferRenderbuffer(t.FRAMEBUFFER,J,t.RENDERBUFFER,te)}n.bindFramebuffer(t.FRAMEBUFFER,null)}function ve(R,M,O){const J=i.get(R);M!==void 0&&he(J.__webglFramebuffer,R,R.texture,t.COLOR_ATTACHMENT0,t.TEXTURE_2D,0),O!==void 0&&ne(R)}function je(R){const M=R.texture,O=i.get(R),J=i.get(M);R.addEventListener("dispose",T);const te=R.textures,K=R.isWebGLCubeRenderTarget===!0,Ce=te.length>1;if(Ce||(J.__webglTexture===void 0&&(J.__webglTexture=t.createTexture()),J.__version=M.version,s.memory.textures++),K){O.__webglFramebuffer=[];for(let de=0;de<6;de++)if(M.mipmaps&&M.mipmaps.length>0){O.__webglFramebuffer[de]=[];for(let ye=0;ye<M.mipmaps.length;ye++)O.__webglFramebuffer[de][ye]=t.createFramebuffer()}else O.__webglFramebuffer[de]=t.createFramebuffer()}else{if(M.mipmaps&&M.mipmaps.length>0){O.__webglFramebuffer=[];for(let de=0;de<M.mipmaps.length;de++)O.__webglFramebuffer[de]=t.createFramebuffer()}else O.__webglFramebuffer=t.createFramebuffer();if(Ce)for(let de=0,ye=te.length;de<ye;de++){const qe=i.get(te[de]);qe.__webglTexture===void 0&&(qe.__webglTexture=t.createTexture(),s.memory.textures++)}if(R.samples>0&&Ge(R)===!1){O.__webglMultisampledFramebuffer=t.createFramebuffer(),O.__webglColorRenderbuffer=[],n.bindFramebuffer(t.FRAMEBUFFER,O.__webglMultisampledFramebuffer);for(let de=0;de<te.length;de++){const ye=te[de];O.__webglColorRenderbuffer[de]=t.createRenderbuffer(),t.bindRenderbuffer(t.RENDERBUFFER,O.__webglColorRenderbuffer[de]);const qe=o.convert(ye.format,ye.colorSpace),oe=o.convert(ye.type),_e=y(ye.internalFormat,qe,oe,ye.colorSpace,R.isXRRenderTarget===!0),Le=Xe(R);t.renderbufferStorageMultisample(t.RENDERBUFFER,Le,_e,R.width,R.height),t.framebufferRenderbuffer(t.FRAMEBUFFER,t.COLOR_ATTACHMENT0+de,t.RENDERBUFFER,O.__webglColorRenderbuffer[de])}t.bindRenderbuffer(t.RENDERBUFFER,null),R.depthBuffer&&(O.__webglDepthRenderbuffer=t.createRenderbuffer(),ae(O.__webglDepthRenderbuffer,R,!0)),n.bindFramebuffer(t.FRAMEBUFFER,null)}}if(K){n.bindTexture(t.TEXTURE_CUBE_MAP,J.__webglTexture),Se(t.TEXTURE_CUBE_MAP,M);for(let de=0;de<6;de++)if(M.mipmaps&&M.mipmaps.length>0)for(let ye=0;ye<M.mipmaps.length;ye++)he(O.__webglFramebuffer[de][ye],R,M,t.COLOR_ATTACHMENT0,t.TEXTURE_CUBE_MAP_POSITIVE_X+de,ye);else he(O.__webglFramebuffer[de],R,M,t.COLOR_ATTACHMENT0,t.TEXTURE_CUBE_MAP_POSITIVE_X+de,0);m(M)&&u(t.TEXTURE_CUBE_MAP),n.unbindTexture()}else if(Ce){for(let de=0,ye=te.length;de<ye;de++){const qe=te[de],oe=i.get(qe);n.bindTexture(t.TEXTURE_2D,oe.__webglTexture),Se(t.TEXTURE_2D,qe),he(O.__webglFramebuffer,R,qe,t.COLOR_ATTACHMENT0+de,t.TEXTURE_2D,0),m(qe)&&u(t.TEXTURE_2D)}n.unbindTexture()}else{let de=t.TEXTURE_2D;if((R.isWebGL3DRenderTarget||R.isWebGLArrayRenderTarget)&&(de=R.isWebGL3DRenderTarget?t.TEXTURE_3D:t.TEXTURE_2D_ARRAY),n.bindTexture(de,J.__webglTexture),Se(de,M),M.mipmaps&&M.mipmaps.length>0)for(let ye=0;ye<M.mipmaps.length;ye++)he(O.__webglFramebuffer[ye],R,M,t.COLOR_ATTACHMENT0,de,ye);else he(O.__webglFramebuffer,R,M,t.COLOR_ATTACHMENT0,de,0);m(M)&&u(de),n.unbindTexture()}R.depthBuffer&&ne(R)}function Ue(R){const M=R.textures;for(let O=0,J=M.length;O<J;O++){const te=M[O];if(m(te)){const K=g(R),Ce=i.get(te).__webglTexture;n.bindTexture(K,Ce),u(K),n.unbindTexture()}}}const ot=[],I=[];function Nt(R){if(R.samples>0){if(Ge(R)===!1){const M=R.textures,O=R.width,J=R.height;let te=t.COLOR_BUFFER_BIT;const K=R.stencilBuffer?t.DEPTH_STENCIL_ATTACHMENT:t.DEPTH_ATTACHMENT,Ce=i.get(R),de=M.length>1;if(de)for(let ye=0;ye<M.length;ye++)n.bindFramebuffer(t.FRAMEBUFFER,Ce.__webglMultisampledFramebuffer),t.framebufferRenderbuffer(t.FRAMEBUFFER,t.COLOR_ATTACHMENT0+ye,t.RENDERBUFFER,null),n.bindFramebuffer(t.FRAMEBUFFER,Ce.__webglFramebuffer),t.framebufferTexture2D(t.DRAW_FRAMEBUFFER,t.COLOR_ATTACHMENT0+ye,t.TEXTURE_2D,null,0);n.bindFramebuffer(t.READ_FRAMEBUFFER,Ce.__webglMultisampledFramebuffer),n.bindFramebuffer(t.DRAW_FRAMEBUFFER,Ce.__webglFramebuffer);for(let ye=0;ye<M.length;ye++){if(R.resolveDepthBuffer&&(R.depthBuffer&&(te|=t.DEPTH_BUFFER_BIT),R.stencilBuffer&&R.resolveStencilBuffer&&(te|=t.STENCIL_BUFFER_BIT)),de){t.framebufferRenderbuffer(t.READ_FRAMEBUFFER,t.COLOR_ATTACHMENT0,t.RENDERBUFFER,Ce.__webglColorRenderbuffer[ye]);const qe=i.get(M[ye]).__webglTexture;t.framebufferTexture2D(t.DRAW_FRAMEBUFFER,t.COLOR_ATTACHMENT0,t.TEXTURE_2D,qe,0)}t.blitFramebuffer(0,0,O,J,0,0,O,J,te,t.NEAREST),l===!0&&(ot.length=0,I.length=0,ot.push(t.COLOR_ATTACHMENT0+ye),R.depthBuffer&&R.resolveDepthBuffer===!1&&(ot.push(K),I.push(K),t.invalidateFramebuffer(t.DRAW_FRAMEBUFFER,I)),t.invalidateFramebuffer(t.READ_FRAMEBUFFER,ot))}if(n.bindFramebuffer(t.READ_FRAMEBUFFER,null),n.bindFramebuffer(t.DRAW_FRAMEBUFFER,null),de)for(let ye=0;ye<M.length;ye++){n.bindFramebuffer(t.FRAMEBUFFER,Ce.__webglMultisampledFramebuffer),t.framebufferRenderbuffer(t.FRAMEBUFFER,t.COLOR_ATTACHMENT0+ye,t.RENDERBUFFER,Ce.__webglColorRenderbuffer[ye]);const qe=i.get(M[ye]).__webglTexture;n.bindFramebuffer(t.FRAMEBUFFER,Ce.__webglFramebuffer),t.framebufferTexture2D(t.DRAW_FRAMEBUFFER,t.COLOR_ATTACHMENT0+ye,t.TEXTURE_2D,qe,0)}n.bindFramebuffer(t.DRAW_FRAMEBUFFER,Ce.__webglMultisampledFramebuffer)}else if(R.depthBuffer&&R.resolveDepthBuffer===!1&&l){const M=R.stencilBuffer?t.DEPTH_STENCIL_ATTACHMENT:t.DEPTH_ATTACHMENT;t.invalidateFramebuffer(t.DRAW_FRAMEBUFFER,[M])}}}function Xe(R){return Math.min(r.maxSamples,R.samples)}function Ge(R){const M=i.get(R);return R.samples>0&&e.has("WEBGL_multisampled_render_to_texture")===!0&&M.__useRenderToTexture!==!1}function Me(R){const M=s.render.frame;d.get(R)!==M&&(d.set(R,M),R.update())}function nt(R,M){const O=R.colorSpace,J=R.format,te=R.type;return R.isCompressedTexture===!0||R.isVideoTexture===!0||O!==Jo&&O!==Hi&&($e.getTransfer(O)===it?(J!==Wt||te!==oi)&&console.warn("THREE.WebGLTextures: sRGB encoded textures have to use RGBAFormat and UnsignedByteType."):console.error("THREE.WebGLTextures: Unsupported texture color space:",O)),M}function De(R){return typeof HTMLImageElement<"u"&&R instanceof HTMLImageElement?(c.width=R.naturalWidth||R.width,c.height=R.naturalHeight||R.height):typeof VideoFrame<"u"&&R instanceof VideoFrame?(c.width=R.displayWidth,c.height=R.displayHeight):(c.width=R.width,c.height=R.height),c}this.allocateTextureUnit=F,this.resetTextureUnits=z,this.setTexture2D=j,this.setTexture2DArray=W,this.setTexture3D=$,this.setTextureCube=D,this.rebindTextures=ve,this.setupRenderTarget=je,this.updateRenderTargetMipmap=Ue,this.updateMultisampleRenderTarget=Nt,this.setupDepthRenderbuffer=ne,this.setupFrameBufferTexture=he,this.useMultisampledRTT=Ge}function mA(t,e){function n(i,r=Hi){let o;const s=$e.getTransfer(r);if(i===oi)return t.UNSIGNED_BYTE;if(i===xf)return t.UNSIGNED_SHORT_4_4_4_4;if(i===Sf)return t.UNSIGNED_SHORT_5_5_5_1;if(i===Vv)return t.UNSIGNED_INT_5_9_9_9_REV;if(i===Bv)return t.BYTE;if(i===Hv)return t.SHORT;if(i===aa)return t.UNSIGNED_SHORT;if(i===_f)return t.INT;if(i===Fr)return t.UNSIGNED_INT;if(i===Pn)return t.FLOAT;if(i===Zo)return t.HALF_FLOAT;if(i===Gv)return t.ALPHA;if(i===Wv)return t.RGB;if(i===Wt)return t.RGBA;if(i===Xv)return t.LUMINANCE;if(i===jv)return t.LUMINANCE_ALPHA;if(i===Lo)return t.DEPTH_COMPONENT;if(i===jo)return t.DEPTH_STENCIL;if(i===Mf)return t.RED;if(i===wf)return t.RED_INTEGER;if(i===Yv)return t.RG;if(i===Ef)return t.RG_INTEGER;if(i===Tf)return t.RGBA_INTEGER;if(i===Sl||i===Ml||i===wl||i===El)if(s===it)if(o=e.get("WEBGL_compressed_texture_s3tc_srgb"),o!==null){if(i===Sl)return o.COMPRESSED_SRGB_S3TC_DXT1_EXT;if(i===Ml)return o.COMPRESSED_SRGB_ALPHA_S3TC_DXT1_EXT;if(i===wl)return o.COMPRESSED_SRGB_ALPHA_S3TC_DXT3_EXT;if(i===El)return o.COMPRESSED_SRGB_ALPHA_S3TC_DXT5_EXT}else return null;else if(o=e.get("WEBGL_compressed_texture_s3tc"),o!==null){if(i===Sl)return o.COMPRESSED_RGB_S3TC_DXT1_EXT;if(i===Ml)return o.COMPRESSED_RGBA_S3TC_DXT1_EXT;if(i===wl)return o.COMPRESSED_RGBA_S3TC_DXT3_EXT;if(i===El)return o.COMPRESSED_RGBA_S3TC_DXT5_EXT}else return null;if(i===Xd||i===jd||i===Yd||i===qd)if(o=e.get("WEBGL_compressed_texture_pvrtc"),o!==null){if(i===Xd)return o.COMPRESSED_RGB_PVRTC_4BPPV1_IMG;if(i===jd)return o.COMPRESSED_RGB_PVRTC_2BPPV1_IMG;if(i===Yd)return o.COMPRESSED_RGBA_PVRTC_4BPPV1_IMG;if(i===qd)return o.COMPRESSED_RGBA_PVRTC_2BPPV1_IMG}else return null;if(i===$d||i===Kd||i===Zd)if(o=e.get("WEBGL_compressed_texture_etc"),o!==null){if(i===$d||i===Kd)return s===it?o.COMPRESSED_SRGB8_ETC2:o.COMPRESSED_RGB8_ETC2;if(i===Zd)return s===it?o.COMPRESSED_SRGB8_ALPHA8_ETC2_EAC:o.COMPRESSED_RGBA8_ETC2_EAC}else return null;if(i===Jd||i===Qd||i===eh||i===th||i===nh||i===ih||i===rh||i===oh||i===sh||i===ah||i===lh||i===ch||i===uh||i===dh)if(o=e.get("WEBGL_compressed_texture_astc"),o!==null){if(i===Jd)return s===it?o.COMPRESSED_SRGB8_ALPHA8_ASTC_4x4_KHR:o.COMPRESSED_RGBA_ASTC_4x4_KHR;if(i===Qd)return s===it?o.COMPRESSED_SRGB8_ALPHA8_ASTC_5x4_KHR:o.COMPRESSED_RGBA_ASTC_5x4_KHR;if(i===eh)return s===it?o.COMPRESSED_SRGB8_ALPHA8_ASTC_5x5_KHR:o.COMPRESSED_RGBA_ASTC_5x5_KHR;if(i===th)return s===it?o.COMPRESSED_SRGB8_ALPHA8_ASTC_6x5_KHR:o.COMPRESSED_RGBA_ASTC_6x5_KHR;if(i===nh)return s===it?o.COMPRESSED_SRGB8_ALPHA8_ASTC_6x6_KHR:o.COMPRESSED_RGBA_ASTC_6x6_KHR;if(i===ih)return s===it?o.COMPRESSED_SRGB8_ALPHA8_ASTC_8x5_KHR:o.COMPRESSED_RGBA_ASTC_8x5_KHR;if(i===rh)return s===it?o.COMPRESSED_SRGB8_ALPHA8_ASTC_8x6_KHR:o.COMPRESSED_RGBA_ASTC_8x6_KHR;if(i===oh)return s===it?o.COMPRESSED_SRGB8_ALPHA8_ASTC_8x8_KHR:o.COMPRESSED_RGBA_ASTC_8x8_KHR;if(i===sh)return s===it?o.COMPRESSED_SRGB8_ALPHA8_ASTC_10x5_KHR:o.COMPRESSED_RGBA_ASTC_10x5_KHR;if(i===ah)return s===it?o.COMPRESSED_SRGB8_ALPHA8_ASTC_10x6_KHR:o.COMPRESSED_RGBA_ASTC_10x6_KHR;if(i===lh)return s===it?o.COMPRESSED_SRGB8_ALPHA8_ASTC_10x8_KHR:o.COMPRESSED_RGBA_ASTC_10x8_KHR;if(i===ch)return s===it?o.COMPRESSED_SRGB8_ALPHA8_ASTC_10x10_KHR:o.COMPRESSED_RGBA_ASTC_10x10_KHR;if(i===uh)return s===it?o.COMPRESSED_SRGB8_ALPHA8_ASTC_12x10_KHR:o.COMPRESSED_RGBA_ASTC_12x10_KHR;if(i===dh)return s===it?o.COMPRESSED_SRGB8_ALPHA8_ASTC_12x12_KHR:o.COMPRESSED_RGBA_ASTC_12x12_KHR}else return null;if(i===Tl||i===hh||i===fh)if(o=e.get("EXT_texture_compression_bptc"),o!==null){if(i===Tl)return s===it?o.COMPRESSED_SRGB_ALPHA_BPTC_UNORM_EXT:o.COMPRESSED_RGBA_BPTC_UNORM_EXT;if(i===hh)return o.COMPRESSED_RGB_BPTC_SIGNED_FLOAT_EXT;if(i===fh)return o.COMPRESSED_RGB_BPTC_UNSIGNED_FLOAT_EXT}else return null;if(i===qv||i===ph||i===mh||i===gh)if(o=e.get("EXT_texture_compression_rgtc"),o!==null){if(i===Tl)return o.COMPRESSED_RED_RGTC1_EXT;if(i===ph)return o.COMPRESSED_SIGNED_RED_RGTC1_EXT;if(i===mh)return o.COMPRESSED_RED_GREEN_RGTC2_EXT;if(i===gh)return o.COMPRESSED_SIGNED_RED_GREEN_RGTC2_EXT}else return null;return i===Xo?t.UNSIGNED_INT_24_8:t[i]!==void 0?t[i]:null}return{convert:n}}class gA extends Cn{constructor(e=[]){super(),this.isArrayCamera=!0,this.cameras=e}}class br extends Xt{constructor(){super(),this.isGroup=!0,this.type="Group"}}const vA={type:"move"};class Pu{constructor(){this._targetRay=null,this._grip=null,this._hand=null}getHandSpace(){return this._hand===null&&(this._hand=new br,this._hand.matrixAutoUpdate=!1,this._hand.visible=!1,this._hand.joints={},this._hand.inputState={pinching:!1}),this._hand}getTargetRaySpace(){return this._targetRay===null&&(this._targetRay=new br,this._targetRay.matrixAutoUpdate=!1,this._targetRay.visible=!1,this._targetRay.hasLinearVelocity=!1,this._targetRay.linearVelocity=new L,this._targetRay.hasAngularVelocity=!1,this._targetRay.angularVelocity=new L),this._targetRay}getGripSpace(){return this._grip===null&&(this._grip=new br,this._grip.matrixAutoUpdate=!1,this._grip.visible=!1,this._grip.hasLinearVelocity=!1,this._grip.linearVelocity=new L,this._grip.hasAngularVelocity=!1,this._grip.angularVelocity=new L),this._grip}dispatchEvent(e){return this._targetRay!==null&&this._targetRay.dispatchEvent(e),this._grip!==null&&this._grip.dispatchEvent(e),this._hand!==null&&this._hand.dispatchEvent(e),this}connect(e){if(e&&e.hand){const n=this._hand;if(n)for(const i of e.hand.values())this._getHandJoint(n,i)}return this.dispatchEvent({type:"connected",data:e}),this}disconnect(e){return this.dispatchEvent({type:"disconnected",data:e}),this._targetRay!==null&&(this._targetRay.visible=!1),this._grip!==null&&(this._grip.visible=!1),this._hand!==null&&(this._hand.visible=!1),this}update(e,n,i){let r=null,o=null,s=null;const a=this._targetRay,l=this._grip,c=this._hand;if(e&&n.session.visibilityState!=="visible-blurred"){if(c&&e.hand){s=!0;for(const _ of e.hand.values()){const m=n.getJointPose(_,i),u=this._getHandJoint(c,_);m!==null&&(u.matrix.fromArray(m.transform.matrix),u.matrix.decompose(u.position,u.rotation,u.scale),u.matrixWorldNeedsUpdate=!0,u.jointRadius=m.radius),u.visible=m!==null}const d=c.joints["index-finger-tip"],f=c.joints["thumb-tip"],h=d.position.distanceTo(f.position),p=.02,v=.005;c.inputState.pinching&&h>p+v?(c.inputState.pinching=!1,this.dispatchEvent({type:"pinchend",handedness:e.handedness,target:this})):!c.inputState.pinching&&h<=p-v&&(c.inputState.pinching=!0,this.dispatchEvent({type:"pinchstart",handedness:e.handedness,target:this}))}else l!==null&&e.gripSpace&&(o=n.getPose(e.gripSpace,i),o!==null&&(l.matrix.fromArray(o.transform.matrix),l.matrix.decompose(l.position,l.rotation,l.scale),l.matrixWorldNeedsUpdate=!0,o.linearVelocity?(l.hasLinearVelocity=!0,l.linearVelocity.copy(o.linearVelocity)):l.hasLinearVelocity=!1,o.angularVelocity?(l.hasAngularVelocity=!0,l.angularVelocity.copy(o.angularVelocity)):l.hasAngularVelocity=!1));a!==null&&(r=n.getPose(e.targetRaySpace,i),r===null&&o!==null&&(r=o),r!==null&&(a.matrix.fromArray(r.transform.matrix),a.matrix.decompose(a.position,a.rotation,a.scale),a.matrixWorldNeedsUpdate=!0,r.linearVelocity?(a.hasLinearVelocity=!0,a.linearVelocity.copy(r.linearVelocity)):a.hasLinearVelocity=!1,r.angularVelocity?(a.hasAngularVelocity=!0,a.angularVelocity.copy(r.angularVelocity)):a.hasAngularVelocity=!1,this.dispatchEvent(vA)))}return a!==null&&(a.visible=r!==null),l!==null&&(l.visible=o!==null),c!==null&&(c.visible=s!==null),this}_getHandJoint(e,n){if(e.joints[n.jointName]===void 0){const i=new br;i.matrixAutoUpdate=!1,i.visible=!1,e.joints[n.jointName]=i,e.add(i)}return e.joints[n.jointName]}}const yA=`
void main() {

	gl_Position = vec4( position, 1.0 );

}`,_A=`
uniform sampler2DArray depthColor;
uniform float depthWidth;
uniform float depthHeight;

void main() {

	vec2 coord = vec2( gl_FragCoord.x / depthWidth, gl_FragCoord.y / depthHeight );

	if ( coord.x >= 1.0 ) {

		gl_FragDepth = texture( depthColor, vec3( coord.x - 1.0, coord.y, 1 ) ).r;

	} else {

		gl_FragDepth = texture( depthColor, vec3( coord.x, coord.y, 0 ) ).r;

	}

}`;class xA{constructor(){this.texture=null,this.mesh=null,this.depthNear=0,this.depthFar=0}init(e,n,i){if(this.texture===null){const r=new Qt,o=e.properties.get(r);o.__webglTexture=n.texture,(n.depthNear!=i.depthNear||n.depthFar!=i.depthFar)&&(this.depthNear=n.depthNear,this.depthFar=n.depthFar),this.texture=r}}getMesh(e){if(this.texture!==null&&this.mesh===null){const n=e.cameras[0].viewport,i=new xn({vertexShader:yA,fragmentShader:_A,uniforms:{depthColor:{value:this.texture},depthWidth:{value:n.z},depthHeight:{value:n.w}}});this.mesh=new Ct(new Hr(20,20),i)}return this.mesh}reset(){this.texture=null,this.mesh=null}getDepthTexture(){return this.texture}}class SA extends Qo{constructor(e,n){super();const i=this;let r=null,o=1,s=null,a="local-floor",l=1,c=null,d=null,f=null,h=null,p=null,v=null;const _=new xA,m=n.getContextAttributes();let u=null,g=null;const y=[],x=[],C=new Re;let A=null;const T=new Cn;T.viewport=new St;const b=new Cn;b.viewport=new St;const w=[T,b],S=new gA;let P=null,z=null;this.cameraAutoUpdate=!0,this.enabled=!1,this.isPresenting=!1,this.getController=function(q){let re=y[q];return re===void 0&&(re=new Pu,y[q]=re),re.getTargetRaySpace()},this.getControllerGrip=function(q){let re=y[q];return re===void 0&&(re=new Pu,y[q]=re),re.getGripSpace()},this.getHand=function(q){let re=y[q];return re===void 0&&(re=new Pu,y[q]=re),re.getHandSpace()};function F(q){const re=x.indexOf(q.inputSource);if(re===-1)return;const he=y[re];he!==void 0&&(he.update(q.inputSource,q.frame,c||s),he.dispatchEvent({type:q.type,data:q.inputSource}))}function Y(){r.removeEventListener("select",F),r.removeEventListener("selectstart",F),r.removeEventListener("selectend",F),r.removeEventListener("squeeze",F),r.removeEventListener("squeezestart",F),r.removeEventListener("squeezeend",F),r.removeEventListener("end",Y),r.removeEventListener("inputsourceschange",j);for(let q=0;q<y.length;q++){const re=x[q];re!==null&&(x[q]=null,y[q].disconnect(re))}P=null,z=null,_.reset(),e.setRenderTarget(u),p=null,h=null,f=null,r=null,g=null,Ne.stop(),i.isPresenting=!1,e.setPixelRatio(A),e.setSize(C.width,C.height,!1),i.dispatchEvent({type:"sessionend"})}this.setFramebufferScaleFactor=function(q){o=q,i.isPresenting===!0&&console.warn("THREE.WebXRManager: Cannot change framebuffer scale while presenting.")},this.setReferenceSpaceType=function(q){a=q,i.isPresenting===!0&&console.warn("THREE.WebXRManager: Cannot change reference space type while presenting.")},this.getReferenceSpace=function(){return c||s},this.setReferenceSpace=function(q){c=q},this.getBaseLayer=function(){return h!==null?h:p},this.getBinding=function(){return f},this.getFrame=function(){return v},this.getSession=function(){return r},this.setSession=async function(q){if(r=q,r!==null){if(u=e.getRenderTarget(),r.addEventListener("select",F),r.addEventListener("selectstart",F),r.addEventListener("selectend",F),r.addEventListener("squeeze",F),r.addEventListener("squeezestart",F),r.addEventListener("squeezeend",F),r.addEventListener("end",Y),r.addEventListener("inputsourceschange",j),m.xrCompatible!==!0&&await n.makeXRCompatible(),A=e.getPixelRatio(),e.getSize(C),r.renderState.layers===void 0){const re={antialias:m.antialias,alpha:!0,depth:m.depth,stencil:m.stencil,framebufferScaleFactor:o};p=new XRWebGLLayer(r,n,re),r.updateRenderState({baseLayer:p}),e.setPixelRatio(1),e.setSize(p.framebufferWidth,p.framebufferHeight,!1),g=new qn(p.framebufferWidth,p.framebufferHeight,{format:Wt,type:oi,colorSpace:e.outputColorSpace,stencilBuffer:m.stencil})}else{let re=null,he=null,ae=null;m.depth&&(ae=m.stencil?n.DEPTH24_STENCIL8:n.DEPTH_COMPONENT24,re=m.stencil?jo:Lo,he=m.stencil?Xo:Fr);const Pe={colorFormat:n.RGBA8,depthFormat:ae,scaleFactor:o};f=new XRWebGLBinding(r,n),h=f.createProjectionLayer(Pe),r.updateRenderState({layers:[h]}),e.setPixelRatio(1),e.setSize(h.textureWidth,h.textureHeight,!1),g=new qn(h.textureWidth,h.textureHeight,{format:Wt,type:oi,depthTexture:new cy(h.textureWidth,h.textureHeight,he,void 0,void 0,void 0,void 0,void 0,void 0,re),stencilBuffer:m.stencil,colorSpace:e.outputColorSpace,samples:m.antialias?4:0,resolveDepthBuffer:h.ignoreDepthValues===!1})}g.isXRRenderTarget=!0,this.setFoveation(l),c=null,s=await r.requestReferenceSpace(a),Ne.setContext(r),Ne.start(),i.isPresenting=!0,i.dispatchEvent({type:"sessionstart"})}},this.getEnvironmentBlendMode=function(){if(r!==null)return r.environmentBlendMode},this.getDepthTexture=function(){return _.getDepthTexture()};function j(q){for(let re=0;re<q.removed.length;re++){const he=q.removed[re],ae=x.indexOf(he);ae>=0&&(x[ae]=null,y[ae].disconnect(he))}for(let re=0;re<q.added.length;re++){const he=q.added[re];let ae=x.indexOf(he);if(ae===-1){for(let ne=0;ne<y.length;ne++)if(ne>=x.length){x.push(he),ae=ne;break}else if(x[ne]===null){x[ne]=he,ae=ne;break}if(ae===-1)break}const Pe=y[ae];Pe&&Pe.connect(he)}}const W=new L,$=new L;function D(q,re,he){W.setFromMatrixPosition(re.matrixWorld),$.setFromMatrixPosition(he.matrixWorld);const ae=W.distanceTo($),Pe=re.projectionMatrix.elements,ne=he.projectionMatrix.elements,ve=Pe[14]/(Pe[10]-1),je=Pe[14]/(Pe[10]+1),Ue=(Pe[9]+1)/Pe[5],ot=(Pe[9]-1)/Pe[5],I=(Pe[8]-1)/Pe[0],Nt=(ne[8]+1)/ne[0],Xe=ve*I,Ge=ve*Nt,Me=ae/(-I+Nt),nt=Me*-I;if(re.matrixWorld.decompose(q.position,q.quaternion,q.scale),q.translateX(nt),q.translateZ(Me),q.matrixWorld.compose(q.position,q.quaternion,q.scale),q.matrixWorldInverse.copy(q.matrixWorld).invert(),Pe[10]===-1)q.projectionMatrix.copy(re.projectionMatrix),q.projectionMatrixInverse.copy(re.projectionMatrixInverse);else{const De=ve+Me,R=je+Me,M=Xe-nt,O=Ge+(ae-nt),J=Ue*je/R*De,te=ot*je/R*De;q.projectionMatrix.makePerspective(M,O,J,te,De,R),q.projectionMatrixInverse.copy(q.projectionMatrix).invert()}}function G(q,re){re===null?q.matrixWorld.copy(q.matrix):q.matrixWorld.multiplyMatrices(re.matrixWorld,q.matrix),q.matrixWorldInverse.copy(q.matrixWorld).invert()}this.updateCamera=function(q){if(r===null)return;let re=q.near,he=q.far;_.texture!==null&&(_.depthNear>0&&(re=_.depthNear),_.depthFar>0&&(he=_.depthFar)),S.near=b.near=T.near=re,S.far=b.far=T.far=he,(P!==S.near||z!==S.far)&&(r.updateRenderState({depthNear:S.near,depthFar:S.far}),P=S.near,z=S.far),T.layers.mask=q.layers.mask|2,b.layers.mask=q.layers.mask|4,S.layers.mask=T.layers.mask|b.layers.mask;const ae=q.parent,Pe=S.cameras;G(S,ae);for(let ne=0;ne<Pe.length;ne++)G(Pe[ne],ae);Pe.length===2?D(S,T,b):S.projectionMatrix.copy(T.projectionMatrix),Z(q,S,ae)};function Z(q,re,he){he===null?q.matrix.copy(re.matrixWorld):(q.matrix.copy(he.matrixWorld),q.matrix.invert(),q.matrix.multiply(re.matrixWorld)),q.matrix.decompose(q.position,q.quaternion,q.scale),q.updateMatrixWorld(!0),q.projectionMatrix.copy(re.projectionMatrix),q.projectionMatrixInverse.copy(re.projectionMatrixInverse),q.isPerspectiveCamera&&(q.fov=vh*2*Math.atan(1/q.projectionMatrix.elements[5]),q.zoom=1)}this.getCamera=function(){return S},this.getFoveation=function(){if(!(h===null&&p===null))return l},this.setFoveation=function(q){l=q,h!==null&&(h.fixedFoveation=q),p!==null&&p.fixedFoveation!==void 0&&(p.fixedFoveation=q)},this.hasDepthSensing=function(){return _.texture!==null},this.getDepthSensingMesh=function(){return _.getMesh(S)};let le=null;function Se(q,re){if(d=re.getViewerPose(c||s),v=re,d!==null){const he=d.views;p!==null&&(e.setRenderTargetFramebuffer(g,p.framebuffer),e.setRenderTarget(g));let ae=!1;he.length!==S.cameras.length&&(S.cameras.length=0,ae=!0);for(let ne=0;ne<he.length;ne++){const ve=he[ne];let je=null;if(p!==null)je=p.getViewport(ve);else{const ot=f.getViewSubImage(h,ve);je=ot.viewport,ne===0&&(e.setRenderTargetTextures(g,ot.colorTexture,h.ignoreDepthValues?void 0:ot.depthStencilTexture),e.setRenderTarget(g))}let Ue=w[ne];Ue===void 0&&(Ue=new Cn,Ue.layers.enable(ne),Ue.viewport=new St,w[ne]=Ue),Ue.matrix.fromArray(ve.transform.matrix),Ue.matrix.decompose(Ue.position,Ue.quaternion,Ue.scale),Ue.projectionMatrix.fromArray(ve.projectionMatrix),Ue.projectionMatrixInverse.copy(Ue.projectionMatrix).invert(),Ue.viewport.set(je.x,je.y,je.width,je.height),ne===0&&(S.matrix.copy(Ue.matrix),S.matrix.decompose(S.position,S.quaternion,S.scale)),ae===!0&&S.cameras.push(Ue)}const Pe=r.enabledFeatures;if(Pe&&Pe.includes("depth-sensing")){const ne=f.getDepthInformation(he[0]);ne&&ne.isValid&&ne.texture&&_.init(e,ne,r.renderState)}}for(let he=0;he<y.length;he++){const ae=x[he],Pe=y[he];ae!==null&&Pe!==void 0&&Pe.update(ae,re,c||s)}le&&le(q,re),re.detectedPlanes&&i.dispatchEvent({type:"planesdetected",data:re}),v=null}const Ne=new ly;Ne.setAnimationLoop(Se),this.setAnimationLoop=function(q){le=q},this.dispose=function(){}}}const gr=new $n,MA=new rt;function wA(t,e){function n(m,u){m.matrixAutoUpdate===!0&&m.updateMatrix(),u.value.copy(m.matrix)}function i(m,u){u.color.getRGB(m.fogColor.value,oy(t)),u.isFog?(m.fogNear.value=u.near,m.fogFar.value=u.far):u.isFogExp2&&(m.fogDensity.value=u.density)}function r(m,u,g,y,x){u.isMeshBasicMaterial||u.isMeshLambertMaterial?o(m,u):u.isMeshToonMaterial?(o(m,u),f(m,u)):u.isMeshPhongMaterial?(o(m,u),d(m,u)):u.isMeshStandardMaterial?(o(m,u),h(m,u),u.isMeshPhysicalMaterial&&p(m,u,x)):u.isMeshMatcapMaterial?(o(m,u),v(m,u)):u.isMeshDepthMaterial?o(m,u):u.isMeshDistanceMaterial?(o(m,u),_(m,u)):u.isMeshNormalMaterial?o(m,u):u.isLineBasicMaterial?(s(m,u),u.isLineDashedMaterial&&a(m,u)):u.isPointsMaterial?l(m,u,g,y):u.isSpriteMaterial?c(m,u):u.isShadowMaterial?(m.color.value.copy(u.color),m.opacity.value=u.opacity):u.isShaderMaterial&&(u.uniformsNeedUpdate=!1)}function o(m,u){m.opacity.value=u.opacity,u.color&&m.diffuse.value.copy(u.color),u.emissive&&m.emissive.value.copy(u.emissive).multiplyScalar(u.emissiveIntensity),u.map&&(m.map.value=u.map,n(u.map,m.mapTransform)),u.alphaMap&&(m.alphaMap.value=u.alphaMap,n(u.alphaMap,m.alphaMapTransform)),u.bumpMap&&(m.bumpMap.value=u.bumpMap,n(u.bumpMap,m.bumpMapTransform),m.bumpScale.value=u.bumpScale,u.side===Jt&&(m.bumpScale.value*=-1)),u.normalMap&&(m.normalMap.value=u.normalMap,n(u.normalMap,m.normalMapTransform),m.normalScale.value.copy(u.normalScale),u.side===Jt&&m.normalScale.value.negate()),u.displacementMap&&(m.displacementMap.value=u.displacementMap,n(u.displacementMap,m.displacementMapTransform),m.displacementScale.value=u.displacementScale,m.displacementBias.value=u.displacementBias),u.emissiveMap&&(m.emissiveMap.value=u.emissiveMap,n(u.emissiveMap,m.emissiveMapTransform)),u.specularMap&&(m.specularMap.value=u.specularMap,n(u.specularMap,m.specularMapTransform)),u.alphaTest>0&&(m.alphaTest.value=u.alphaTest);const g=e.get(u),y=g.envMap,x=g.envMapRotation;y&&(m.envMap.value=y,gr.copy(x),gr.x*=-1,gr.y*=-1,gr.z*=-1,y.isCubeTexture&&y.isRenderTargetTexture===!1&&(gr.y*=-1,gr.z*=-1),m.envMapRotation.value.setFromMatrix4(MA.makeRotationFromEuler(gr)),m.flipEnvMap.value=y.isCubeTexture&&y.isRenderTargetTexture===!1?-1:1,m.reflectivity.value=u.reflectivity,m.ior.value=u.ior,m.refractionRatio.value=u.refractionRatio),u.lightMap&&(m.lightMap.value=u.lightMap,m.lightMapIntensity.value=u.lightMapIntensity,n(u.lightMap,m.lightMapTransform)),u.aoMap&&(m.aoMap.value=u.aoMap,m.aoMapIntensity.value=u.aoMapIntensity,n(u.aoMap,m.aoMapTransform))}function s(m,u){m.diffuse.value.copy(u.color),m.opacity.value=u.opacity,u.map&&(m.map.value=u.map,n(u.map,m.mapTransform))}function a(m,u){m.dashSize.value=u.dashSize,m.totalSize.value=u.dashSize+u.gapSize,m.scale.value=u.scale}function l(m,u,g,y){m.diffuse.value.copy(u.color),m.opacity.value=u.opacity,m.size.value=u.size*g,m.scale.value=y*.5,u.map&&(m.map.value=u.map,n(u.map,m.uvTransform)),u.alphaMap&&(m.alphaMap.value=u.alphaMap,n(u.alphaMap,m.alphaMapTransform)),u.alphaTest>0&&(m.alphaTest.value=u.alphaTest)}function c(m,u){m.diffuse.value.copy(u.color),m.opacity.value=u.opacity,m.rotation.value=u.rotation,u.map&&(m.map.value=u.map,n(u.map,m.mapTransform)),u.alphaMap&&(m.alphaMap.value=u.alphaMap,n(u.alphaMap,m.alphaMapTransform)),u.alphaTest>0&&(m.alphaTest.value=u.alphaTest)}function d(m,u){m.specular.value.copy(u.specular),m.shininess.value=Math.max(u.shininess,1e-4)}function f(m,u){u.gradientMap&&(m.gradientMap.value=u.gradientMap)}function h(m,u){m.metalness.value=u.metalness,u.metalnessMap&&(m.metalnessMap.value=u.metalnessMap,n(u.metalnessMap,m.metalnessMapTransform)),m.roughness.value=u.roughness,u.roughnessMap&&(m.roughnessMap.value=u.roughnessMap,n(u.roughnessMap,m.roughnessMapTransform)),u.envMap&&(m.envMapIntensity.value=u.envMapIntensity)}function p(m,u,g){m.ior.value=u.ior,u.sheen>0&&(m.sheenColor.value.copy(u.sheenColor).multiplyScalar(u.sheen),m.sheenRoughness.value=u.sheenRoughness,u.sheenColorMap&&(m.sheenColorMap.value=u.sheenColorMap,n(u.sheenColorMap,m.sheenColorMapTransform)),u.sheenRoughnessMap&&(m.sheenRoughnessMap.value=u.sheenRoughnessMap,n(u.sheenRoughnessMap,m.sheenRoughnessMapTransform))),u.clearcoat>0&&(m.clearcoat.value=u.clearcoat,m.clearcoatRoughness.value=u.clearcoatRoughness,u.clearcoatMap&&(m.clearcoatMap.value=u.clearcoatMap,n(u.clearcoatMap,m.clearcoatMapTransform)),u.clearcoatRoughnessMap&&(m.clearcoatRoughnessMap.value=u.clearcoatRoughnessMap,n(u.clearcoatRoughnessMap,m.clearcoatRoughnessMapTransform)),u.clearcoatNormalMap&&(m.clearcoatNormalMap.value=u.clearcoatNormalMap,n(u.clearcoatNormalMap,m.clearcoatNormalMapTransform),m.clearcoatNormalScale.value.copy(u.clearcoatNormalScale),u.side===Jt&&m.clearcoatNormalScale.value.negate())),u.dispersion>0&&(m.dispersion.value=u.dispersion),u.iridescence>0&&(m.iridescence.value=u.iridescence,m.iridescenceIOR.value=u.iridescenceIOR,m.iridescenceThicknessMinimum.value=u.iridescenceThicknessRange[0],m.iridescenceThicknessMaximum.value=u.iridescenceThicknessRange[1],u.iridescenceMap&&(m.iridescenceMap.value=u.iridescenceMap,n(u.iridescenceMap,m.iridescenceMapTransform)),u.iridescenceThicknessMap&&(m.iridescenceThicknessMap.value=u.iridescenceThicknessMap,n(u.iridescenceThicknessMap,m.iridescenceThicknessMapTransform))),u.transmission>0&&(m.transmission.value=u.transmission,m.transmissionSamplerMap.value=g.texture,m.transmissionSamplerSize.value.set(g.width,g.height),u.transmissionMap&&(m.transmissionMap.value=u.transmissionMap,n(u.transmissionMap,m.transmissionMapTransform)),m.thickness.value=u.thickness,u.thicknessMap&&(m.thicknessMap.value=u.thicknessMap,n(u.thicknessMap,m.thicknessMapTransform)),m.attenuationDistance.value=u.attenuationDistance,m.attenuationColor.value.copy(u.attenuationColor)),u.anisotropy>0&&(m.anisotropyVector.value.set(u.anisotropy*Math.cos(u.anisotropyRotation),u.anisotropy*Math.sin(u.anisotropyRotation)),u.anisotropyMap&&(m.anisotropyMap.value=u.anisotropyMap,n(u.anisotropyMap,m.anisotropyMapTransform))),m.specularIntensity.value=u.specularIntensity,m.specularColor.value.copy(u.specularColor),u.specularColorMap&&(m.specularColorMap.value=u.specularColorMap,n(u.specularColorMap,m.specularColorMapTransform)),u.specularIntensityMap&&(m.specularIntensityMap.value=u.specularIntensityMap,n(u.specularIntensityMap,m.specularIntensityMapTransform))}function v(m,u){u.matcap&&(m.matcap.value=u.matcap)}function _(m,u){const g=e.get(u).light;m.referencePosition.value.setFromMatrixPosition(g.matrixWorld),m.nearDistance.value=g.shadow.camera.near,m.farDistance.value=g.shadow.camera.far}return{refreshFogUniforms:i,refreshMaterialUniforms:r}}function EA(t,e,n,i){let r={},o={},s=[];const a=t.getParameter(t.MAX_UNIFORM_BUFFER_BINDINGS);function l(g,y){const x=y.program;i.uniformBlockBinding(g,x)}function c(g,y){let x=r[g.id];x===void 0&&(v(g),x=d(g),r[g.id]=x,g.addEventListener("dispose",m));const C=y.program;i.updateUBOMapping(g,C);const A=e.render.frame;o[g.id]!==A&&(h(g),o[g.id]=A)}function d(g){const y=f();g.__bindingPointIndex=y;const x=t.createBuffer(),C=g.__size,A=g.usage;return t.bindBuffer(t.UNIFORM_BUFFER,x),t.bufferData(t.UNIFORM_BUFFER,C,A),t.bindBuffer(t.UNIFORM_BUFFER,null),t.bindBufferBase(t.UNIFORM_BUFFER,y,x),x}function f(){for(let g=0;g<a;g++)if(s.indexOf(g)===-1)return s.push(g),g;return console.error("THREE.WebGLRenderer: Maximum number of simultaneously usable uniforms groups reached."),0}function h(g){const y=r[g.id],x=g.uniforms,C=g.__cache;t.bindBuffer(t.UNIFORM_BUFFER,y);for(let A=0,T=x.length;A<T;A++){const b=Array.isArray(x[A])?x[A]:[x[A]];for(let w=0,S=b.length;w<S;w++){const P=b[w];if(p(P,A,w,C)===!0){const z=P.__offset,F=Array.isArray(P.value)?P.value:[P.value];let Y=0;for(let j=0;j<F.length;j++){const W=F[j],$=_(W);typeof W=="number"||typeof W=="boolean"?(P.__data[0]=W,t.bufferSubData(t.UNIFORM_BUFFER,z+Y,P.__data)):W.isMatrix3?(P.__data[0]=W.elements[0],P.__data[1]=W.elements[1],P.__data[2]=W.elements[2],P.__data[3]=0,P.__data[4]=W.elements[3],P.__data[5]=W.elements[4],P.__data[6]=W.elements[5],P.__data[7]=0,P.__data[8]=W.elements[6],P.__data[9]=W.elements[7],P.__data[10]=W.elements[8],P.__data[11]=0):(W.toArray(P.__data,Y),Y+=$.storage/Float32Array.BYTES_PER_ELEMENT)}t.bufferSubData(t.UNIFORM_BUFFER,z,P.__data)}}}t.bindBuffer(t.UNIFORM_BUFFER,null)}function p(g,y,x,C){const A=g.value,T=y+"_"+x;if(C[T]===void 0)return typeof A=="number"||typeof A=="boolean"?C[T]=A:C[T]=A.clone(),!0;{const b=C[T];if(typeof A=="number"||typeof A=="boolean"){if(b!==A)return C[T]=A,!0}else if(b.equals(A)===!1)return b.copy(A),!0}return!1}function v(g){const y=g.uniforms;let x=0;const C=16;for(let T=0,b=y.length;T<b;T++){const w=Array.isArray(y[T])?y[T]:[y[T]];for(let S=0,P=w.length;S<P;S++){const z=w[S],F=Array.isArray(z.value)?z.value:[z.value];for(let Y=0,j=F.length;Y<j;Y++){const W=F[Y],$=_(W),D=x%C,G=D%$.boundary,Z=D+G;x+=G,Z!==0&&C-Z<$.storage&&(x+=C-Z),z.__data=new Float32Array($.storage/Float32Array.BYTES_PER_ELEMENT),z.__offset=x,x+=$.storage}}}const A=x%C;return A>0&&(x+=C-A),g.__size=x,g.__cache={},this}function _(g){const y={boundary:0,storage:0};return typeof g=="number"||typeof g=="boolean"?(y.boundary=4,y.storage=4):g.isVector2?(y.boundary=8,y.storage=8):g.isVector3||g.isColor?(y.boundary=16,y.storage=12):g.isVector4?(y.boundary=16,y.storage=16):g.isMatrix3?(y.boundary=48,y.storage=48):g.isMatrix4?(y.boundary=64,y.storage=64):g.isTexture?console.warn("THREE.WebGLRenderer: Texture samplers can not be part of an uniforms group."):console.warn("THREE.WebGLRenderer: Unsupported uniform value type.",g),y}function m(g){const y=g.target;y.removeEventListener("dispose",m);const x=s.indexOf(y.__bindingPointIndex);s.splice(x,1),t.deleteBuffer(r[y.id]),delete r[y.id],delete o[y.id]}function u(){for(const g in r)t.deleteBuffer(r[g]);s=[],r={},o={}}return{bind:l,update:c,dispose:u}}class TA{constructor(e={}){const{canvas:n=hM(),context:i=null,depth:r=!0,stencil:o=!1,alpha:s=!1,antialias:a=!1,premultipliedAlpha:l=!0,preserveDrawingBuffer:c=!1,powerPreference:d="default",failIfMajorPerformanceCaveat:f=!1,reverseDepthBuffer:h=!1}=e;this.isWebGLRenderer=!0;let p;if(i!==null){if(typeof WebGLRenderingContext<"u"&&i instanceof WebGLRenderingContext)throw new Error("THREE.WebGLRenderer: WebGL 1 is not supported since r163.");p=i.getContextAttributes().alpha}else p=s;const v=new Uint32Array(4),_=new Int32Array(4);let m=null,u=null;const g=[],y=[];this.domElement=n,this.debug={checkShaderErrors:!0,onShaderError:null},this.autoClear=!0,this.autoClearColor=!0,this.autoClearDepth=!0,this.autoClearStencil=!0,this.sortObjects=!0,this.clippingPlanes=[],this.localClippingEnabled=!1,this._outputColorSpace=gn,this.toneMapping=ir,this.toneMappingExposure=1;const x=this;let C=!1,A=0,T=0,b=null,w=-1,S=null;const P=new St,z=new St;let F=null;const Y=new we(0);let j=0,W=n.width,$=n.height,D=1,G=null,Z=null;const le=new St(0,0,W,$),Se=new St(0,0,W,$);let Ne=!1;const q=new Af;let re=!1,he=!1;const ae=new rt,Pe=new rt,ne=new L,ve=new St,je={background:null,fog:null,environment:null,overrideMaterial:null,isScene:!0};let Ue=!1;function ot(){return b===null?D:1}let I=i;function Nt(E,k){return n.getContext(E,k)}try{const E={alpha:!0,depth:r,stencil:o,antialias:a,premultipliedAlpha:l,preserveDrawingBuffer:c,powerPreference:d,failIfMajorPerformanceCaveat:f};if("setAttribute"in n&&n.setAttribute("data-engine",`three.js r${yf}`),n.addEventListener("webglcontextlost",Q,!1),n.addEventListener("webglcontextrestored",me,!1),n.addEventListener("webglcontextcreationerror",fe,!1),I===null){const k="webgl2";if(I=Nt(k,E),I===null)throw Nt(k)?new Error("Error creating WebGL context with your selected attributes."):new Error("Error creating WebGL context.")}}catch(E){throw console.error("THREE.WebGLRenderer: "+E.message),E}let Xe,Ge,Me,nt,De,R,M,O,J,te,K,Ce,de,ye,qe,oe,_e,Le,Ie,xe,Ye,He,st,N;function ue(){Xe=new PE(I),Xe.init(),He=new mA(I,Xe),Ge=new EE(I,Xe,e,He),Me=new hA(I,Xe),Ge.reverseDepthBuffer&&h&&Me.buffers.depth.setReversed(!0),nt=new NE(I),De=new ZT,R=new pA(I,Xe,Me,De,Ge,He,nt),M=new AE(x),O=new RE(x),J=new BM(I),st=new ME(I,J),te=new DE(I,J,nt,st),K=new kE(I,te,J,nt),Ie=new IE(I,Ge,R),oe=new TE(De),Ce=new KT(x,M,O,Xe,Ge,st,oe),de=new wA(x,De),ye=new QT,qe=new oA(Xe),Le=new SE(x,M,O,Me,K,p,l),_e=new uA(x,K,Ge),N=new EA(I,nt,Ge,Me),xe=new wE(I,Xe,nt),Ye=new LE(I,Xe,nt),nt.programs=Ce.programs,x.capabilities=Ge,x.extensions=Xe,x.properties=De,x.renderLists=ye,x.shadowMap=_e,x.state=Me,x.info=nt}ue();const X=new SA(x,I);this.xr=X,this.getContext=function(){return I},this.getContextAttributes=function(){return I.getContextAttributes()},this.forceContextLoss=function(){const E=Xe.get("WEBGL_lose_context");E&&E.loseContext()},this.forceContextRestore=function(){const E=Xe.get("WEBGL_lose_context");E&&E.restoreContext()},this.getPixelRatio=function(){return D},this.setPixelRatio=function(E){E!==void 0&&(D=E,this.setSize(W,$,!1))},this.getSize=function(E){return E.set(W,$)},this.setSize=function(E,k,B=!0){if(X.isPresenting){console.warn("THREE.WebGLRenderer: Can't change size while VR device is presenting.");return}W=E,$=k,n.width=Math.floor(E*D),n.height=Math.floor(k*D),B===!0&&(n.style.width=E+"px",n.style.height=k+"px"),this.setViewport(0,0,E,k)},this.getDrawingBufferSize=function(E){return E.set(W*D,$*D).floor()},this.setDrawingBufferSize=function(E,k,B){W=E,$=k,D=B,n.width=Math.floor(E*B),n.height=Math.floor(k*B),this.setViewport(0,0,E,k)},this.getCurrentViewport=function(E){return E.copy(P)},this.getViewport=function(E){return E.copy(le)},this.setViewport=function(E,k,B,H){E.isVector4?le.set(E.x,E.y,E.z,E.w):le.set(E,k,B,H),Me.viewport(P.copy(le).multiplyScalar(D).round())},this.getScissor=function(E){return E.copy(Se)},this.setScissor=function(E,k,B,H){E.isVector4?Se.set(E.x,E.y,E.z,E.w):Se.set(E,k,B,H),Me.scissor(z.copy(Se).multiplyScalar(D).round())},this.getScissorTest=function(){return Ne},this.setScissorTest=function(E){Me.setScissorTest(Ne=E)},this.setOpaqueSort=function(E){G=E},this.setTransparentSort=function(E){Z=E},this.getClearColor=function(E){return E.copy(Le.getClearColor())},this.setClearColor=function(){Le.setClearColor.apply(Le,arguments)},this.getClearAlpha=function(){return Le.getClearAlpha()},this.setClearAlpha=function(){Le.setClearAlpha.apply(Le,arguments)},this.clear=function(E=!0,k=!0,B=!0){let H=0;if(E){let U=!1;if(b!==null){const se=b.texture.format;U=se===Tf||se===Ef||se===wf}if(U){const se=b.texture.type,pe=se===oi||se===Fr||se===aa||se===Xo||se===xf||se===Sf,Ee=Le.getClearColor(),Te=Le.getClearAlpha(),ke=Ee.r,Oe=Ee.g,Ae=Ee.b;pe?(v[0]=ke,v[1]=Oe,v[2]=Ae,v[3]=Te,I.clearBufferuiv(I.COLOR,0,v)):(_[0]=ke,_[1]=Oe,_[2]=Ae,_[3]=Te,I.clearBufferiv(I.COLOR,0,_))}else H|=I.COLOR_BUFFER_BIT}k&&(H|=I.DEPTH_BUFFER_BIT),B&&(H|=I.STENCIL_BUFFER_BIT,this.state.buffers.stencil.setMask(4294967295)),I.clear(H)},this.clearColor=function(){this.clear(!0,!1,!1)},this.clearDepth=function(){this.clear(!1,!0,!1)},this.clearStencil=function(){this.clear(!1,!1,!0)},this.dispose=function(){n.removeEventListener("webglcontextlost",Q,!1),n.removeEventListener("webglcontextrestored",me,!1),n.removeEventListener("webglcontextcreationerror",fe,!1),ye.dispose(),qe.dispose(),De.dispose(),M.dispose(),O.dispose(),K.dispose(),st.dispose(),N.dispose(),Ce.dispose(),X.dispose(),X.removeEventListener("sessionstart",Uf),X.removeEventListener("sessionend",Ff),ur.stop()};function Q(E){E.preventDefault(),console.log("THREE.WebGLRenderer: Context Lost."),C=!0}function me(){console.log("THREE.WebGLRenderer: Context Restored."),C=!1;const E=nt.autoReset,k=_e.enabled,B=_e.autoUpdate,H=_e.needsUpdate,U=_e.type;ue(),nt.autoReset=E,_e.enabled=k,_e.autoUpdate=B,_e.needsUpdate=H,_e.type=U}function fe(E){console.error("THREE.WebGLRenderer: A WebGL context could not be created. Reason: ",E.statusMessage)}function Fe(E){const k=E.target;k.removeEventListener("dispose",Fe),_t(k)}function _t(E){Ft(E),De.remove(E)}function Ft(E){const k=De.get(E).programs;k!==void 0&&(k.forEach(function(B){Ce.releaseProgram(B)}),E.isShaderMaterial&&Ce.releaseShaderCache(E))}this.renderBufferDirect=function(E,k,B,H,U,se){k===null&&(k=je);const pe=U.isMesh&&U.matrixWorld.determinant()<0,Ee=Ly(E,k,B,H,U);Me.setMaterial(H,pe);let Te=B.index,ke=1;if(H.wireframe===!0){if(Te=te.getWireframeAttribute(B),Te===void 0)return;ke=2}const Oe=B.drawRange,Ae=B.attributes.position;let Ze=Oe.start*ke,at=(Oe.start+Oe.count)*ke;se!==null&&(Ze=Math.max(Ze,se.start*ke),at=Math.min(at,(se.start+se.count)*ke)),Te!==null?(Ze=Math.max(Ze,0),at=Math.min(at,Te.count)):Ae!=null&&(Ze=Math.max(Ze,0),at=Math.min(at,Ae.count));const ut=at-Ze;if(ut<0||ut===1/0)return;st.setup(U,H,Ee,B,Te);let rn,Je=xe;if(Te!==null&&(rn=J.get(Te),Je=Ye,Je.setIndex(rn)),U.isMesh)H.wireframe===!0?(Me.setLineWidth(H.wireframeLinewidth*ot()),Je.setMode(I.LINES)):Je.setMode(I.TRIANGLES);else if(U.isLine){let be=H.linewidth;be===void 0&&(be=1),Me.setLineWidth(be*ot()),U.isLineSegments?Je.setMode(I.LINES):U.isLineLoop?Je.setMode(I.LINE_LOOP):Je.setMode(I.LINE_STRIP)}else U.isPoints?Je.setMode(I.POINTS):U.isSprite&&Je.setMode(I.TRIANGLES);if(U.isBatchedMesh)if(U._multiDrawInstances!==null)Je.renderMultiDrawInstances(U._multiDrawStarts,U._multiDrawCounts,U._multiDrawCount,U._multiDrawInstances);else if(Xe.get("WEBGL_multi_draw"))Je.renderMultiDraw(U._multiDrawStarts,U._multiDrawCounts,U._multiDrawCount);else{const be=U._multiDrawStarts,ai=U._multiDrawCounts,Qe=U._multiDrawCount,Un=Te?J.get(Te).bytesPerElement:1,Vr=De.get(H).currentProgram.getUniforms();for(let hn=0;hn<Qe;hn++)Vr.setValue(I,"_gl_DrawID",hn),Je.render(be[hn]/Un,ai[hn])}else if(U.isInstancedMesh)Je.renderInstances(Ze,ut,U.count);else if(B.isInstancedBufferGeometry){const be=B._maxInstanceCount!==void 0?B._maxInstanceCount:1/0,ai=Math.min(B.instanceCount,be);Je.renderInstances(Ze,ut,ai)}else Je.render(Ze,ut)};function et(E,k,B){E.transparent===!0&&E.side===bn&&E.forceSinglePass===!1?(E.side=Jt,E.needsUpdate=!0,Sa(E,k,B),E.side=sr,E.needsUpdate=!0,Sa(E,k,B),E.side=bn):Sa(E,k,B)}this.compile=function(E,k,B=null){B===null&&(B=E),u=qe.get(B),u.init(k),y.push(u),B.traverseVisible(function(U){U.isLight&&U.layers.test(k.layers)&&(u.pushLight(U),U.castShadow&&u.pushShadow(U))}),E!==B&&E.traverseVisible(function(U){U.isLight&&U.layers.test(k.layers)&&(u.pushLight(U),U.castShadow&&u.pushShadow(U))}),u.setupLights();const H=new Set;return E.traverse(function(U){if(!(U.isMesh||U.isPoints||U.isLine||U.isSprite))return;const se=U.material;if(se)if(Array.isArray(se))for(let pe=0;pe<se.length;pe++){const Ee=se[pe];et(Ee,B,U),H.add(Ee)}else et(se,B,U),H.add(se)}),y.pop(),u=null,H},this.compileAsync=function(E,k,B=null){const H=this.compile(E,k,B);return new Promise(U=>{function se(){if(H.forEach(function(pe){De.get(pe).currentProgram.isReady()&&H.delete(pe)}),H.size===0){U(E);return}setTimeout(se,10)}Xe.get("KHR_parallel_shader_compile")!==null?se():setTimeout(se,10)})};let kn=null;function si(E){kn&&kn(E)}function Uf(){ur.stop()}function Ff(){ur.start()}const ur=new ly;ur.setAnimationLoop(si),typeof self<"u"&&ur.setContext(self),this.setAnimationLoop=function(E){kn=E,X.setAnimationLoop(E),E===null?ur.stop():ur.start()},X.addEventListener("sessionstart",Uf),X.addEventListener("sessionend",Ff),this.render=function(E,k){if(k!==void 0&&k.isCamera!==!0){console.error("THREE.WebGLRenderer.render: camera is not an instance of THREE.Camera.");return}if(C===!0)return;if(E.matrixWorldAutoUpdate===!0&&E.updateMatrixWorld(),k.parent===null&&k.matrixWorldAutoUpdate===!0&&k.updateMatrixWorld(),X.enabled===!0&&X.isPresenting===!0&&(X.cameraAutoUpdate===!0&&X.updateCamera(k),k=X.getCamera()),E.isScene===!0&&E.onBeforeRender(x,E,k,b),u=qe.get(E,y.length),u.init(k),y.push(u),Pe.multiplyMatrices(k.projectionMatrix,k.matrixWorldInverse),q.setFromProjectionMatrix(Pe),he=this.localClippingEnabled,re=oe.init(this.clippingPlanes,he),m=ye.get(E,g.length),m.init(),g.push(m),X.enabled===!0&&X.isPresenting===!0){const se=x.xr.getDepthSensingMesh();se!==null&&bc(se,k,-1/0,x.sortObjects)}bc(E,k,0,x.sortObjects),m.finish(),x.sortObjects===!0&&m.sort(G,Z),Ue=X.enabled===!1||X.isPresenting===!1||X.hasDepthSensing()===!1,Ue&&Le.addToRenderList(m,E),this.info.render.frame++,re===!0&&oe.beginShadows();const B=u.state.shadowsArray;_e.render(B,E,k),re===!0&&oe.endShadows(),this.info.autoReset===!0&&this.info.reset();const H=m.opaque,U=m.transmissive;if(u.setupLights(),k.isArrayCamera){const se=k.cameras;if(U.length>0)for(let pe=0,Ee=se.length;pe<Ee;pe++){const Te=se[pe];zf(H,U,E,Te)}Ue&&Le.render(E);for(let pe=0,Ee=se.length;pe<Ee;pe++){const Te=se[pe];Of(m,E,Te,Te.viewport)}}else U.length>0&&zf(H,U,E,k),Ue&&Le.render(E),Of(m,E,k);b!==null&&(R.updateMultisampleRenderTarget(b),R.updateRenderTargetMipmap(b)),E.isScene===!0&&E.onAfterRender(x,E,k),st.resetDefaultState(),w=-1,S=null,y.pop(),y.length>0?(u=y[y.length-1],re===!0&&oe.setGlobalState(x.clippingPlanes,u.state.camera)):u=null,g.pop(),g.length>0?m=g[g.length-1]:m=null};function bc(E,k,B,H){if(E.visible===!1)return;if(E.layers.test(k.layers)){if(E.isGroup)B=E.renderOrder;else if(E.isLOD)E.autoUpdate===!0&&E.update(k);else if(E.isLight)u.pushLight(E),E.castShadow&&u.pushShadow(E);else if(E.isSprite){if(!E.frustumCulled||q.intersectsSprite(E)){H&&ve.setFromMatrixPosition(E.matrixWorld).applyMatrix4(Pe);const pe=K.update(E),Ee=E.material;Ee.visible&&m.push(E,pe,Ee,B,ve.z,null)}}else if((E.isMesh||E.isLine||E.isPoints)&&(!E.frustumCulled||q.intersectsObject(E))){const pe=K.update(E),Ee=E.material;if(H&&(E.boundingSphere!==void 0?(E.boundingSphere===null&&E.computeBoundingSphere(),ve.copy(E.boundingSphere.center)):(pe.boundingSphere===null&&pe.computeBoundingSphere(),ve.copy(pe.boundingSphere.center)),ve.applyMatrix4(E.matrixWorld).applyMatrix4(Pe)),Array.isArray(Ee)){const Te=pe.groups;for(let ke=0,Oe=Te.length;ke<Oe;ke++){const Ae=Te[ke],Ze=Ee[Ae.materialIndex];Ze&&Ze.visible&&m.push(E,pe,Ze,B,ve.z,Ae)}}else Ee.visible&&m.push(E,pe,Ee,B,ve.z,null)}}const se=E.children;for(let pe=0,Ee=se.length;pe<Ee;pe++)bc(se[pe],k,B,H)}function Of(E,k,B,H){const U=E.opaque,se=E.transmissive,pe=E.transparent;u.setupLightsView(B),re===!0&&oe.setGlobalState(x.clippingPlanes,B),H&&Me.viewport(P.copy(H)),U.length>0&&xa(U,k,B),se.length>0&&xa(se,k,B),pe.length>0&&xa(pe,k,B),Me.buffers.depth.setTest(!0),Me.buffers.depth.setMask(!0),Me.buffers.color.setMask(!0),Me.setPolygonOffset(!1)}function zf(E,k,B,H){if((B.isScene===!0?B.overrideMaterial:null)!==null)return;u.state.transmissionRenderTarget[H.id]===void 0&&(u.state.transmissionRenderTarget[H.id]=new qn(1,1,{generateMipmaps:!0,type:Xe.has("EXT_color_buffer_half_float")||Xe.has("EXT_color_buffer_float")?Zo:oi,minFilter:ni,samples:4,stencilBuffer:o,resolveDepthBuffer:!1,resolveStencilBuffer:!1,colorSpace:$e.workingColorSpace}));const se=u.state.transmissionRenderTarget[H.id],pe=H.viewport||P;se.setSize(pe.z,pe.w);const Ee=x.getRenderTarget();x.setRenderTarget(se),x.getClearColor(Y),j=x.getClearAlpha(),j<1&&x.setClearColor(16777215,.5),x.clear(),Ue&&Le.render(B);const Te=x.toneMapping;x.toneMapping=ir;const ke=H.viewport;if(H.viewport!==void 0&&(H.viewport=void 0),u.setupLightsView(H),re===!0&&oe.setGlobalState(x.clippingPlanes,H),xa(E,B,H),R.updateMultisampleRenderTarget(se),R.updateRenderTargetMipmap(se),Xe.has("WEBGL_multisampled_render_to_texture")===!1){let Oe=!1;for(let Ae=0,Ze=k.length;Ae<Ze;Ae++){const at=k[Ae],ut=at.object,rn=at.geometry,Je=at.material,be=at.group;if(Je.side===bn&&ut.layers.test(H.layers)){const ai=Je.side;Je.side=Jt,Je.needsUpdate=!0,Bf(ut,B,H,rn,Je,be),Je.side=ai,Je.needsUpdate=!0,Oe=!0}}Oe===!0&&(R.updateMultisampleRenderTarget(se),R.updateRenderTargetMipmap(se))}x.setRenderTarget(Ee),x.setClearColor(Y,j),ke!==void 0&&(H.viewport=ke),x.toneMapping=Te}function xa(E,k,B){const H=k.isScene===!0?k.overrideMaterial:null;for(let U=0,se=E.length;U<se;U++){const pe=E[U],Ee=pe.object,Te=pe.geometry,ke=H===null?pe.material:H,Oe=pe.group;Ee.layers.test(B.layers)&&Bf(Ee,k,B,Te,ke,Oe)}}function Bf(E,k,B,H,U,se){E.onBeforeRender(x,k,B,H,U,se),E.modelViewMatrix.multiplyMatrices(B.matrixWorldInverse,E.matrixWorld),E.normalMatrix.getNormalMatrix(E.modelViewMatrix),U.onBeforeRender(x,k,B,H,E,se),U.transparent===!0&&U.side===bn&&U.forceSinglePass===!1?(U.side=Jt,U.needsUpdate=!0,x.renderBufferDirect(B,k,H,U,E,se),U.side=sr,U.needsUpdate=!0,x.renderBufferDirect(B,k,H,U,E,se),U.side=bn):x.renderBufferDirect(B,k,H,U,E,se),E.onAfterRender(x,k,B,H,U,se)}function Sa(E,k,B){k.isScene!==!0&&(k=je);const H=De.get(E),U=u.state.lights,se=u.state.shadowsArray,pe=U.state.version,Ee=Ce.getParameters(E,U.state,se,k,B),Te=Ce.getProgramCacheKey(Ee);let ke=H.programs;H.environment=E.isMeshStandardMaterial?k.environment:null,H.fog=k.fog,H.envMap=(E.isMeshStandardMaterial?O:M).get(E.envMap||H.environment),H.envMapRotation=H.environment!==null&&E.envMap===null?k.environmentRotation:E.envMapRotation,ke===void 0&&(E.addEventListener("dispose",Fe),ke=new Map,H.programs=ke);let Oe=ke.get(Te);if(Oe!==void 0){if(H.currentProgram===Oe&&H.lightsStateVersion===pe)return Vf(E,Ee),Oe}else Ee.uniforms=Ce.getUniforms(E),E.onBeforeCompile(Ee,x),Oe=Ce.acquireProgram(Ee,Te),ke.set(Te,Oe),H.uniforms=Ee.uniforms;const Ae=H.uniforms;return(!E.isShaderMaterial&&!E.isRawShaderMaterial||E.clipping===!0)&&(Ae.clippingPlanes=oe.uniform),Vf(E,Ee),H.needsLights=Iy(E),H.lightsStateVersion=pe,H.needsLights&&(Ae.ambientLightColor.value=U.state.ambient,Ae.lightProbe.value=U.state.probe,Ae.directionalLights.value=U.state.directional,Ae.directionalLightShadows.value=U.state.directionalShadow,Ae.spotLights.value=U.state.spot,Ae.spotLightShadows.value=U.state.spotShadow,Ae.rectAreaLights.value=U.state.rectArea,Ae.ltc_1.value=U.state.rectAreaLTC1,Ae.ltc_2.value=U.state.rectAreaLTC2,Ae.pointLights.value=U.state.point,Ae.pointLightShadows.value=U.state.pointShadow,Ae.hemisphereLights.value=U.state.hemi,Ae.directionalShadowMap.value=U.state.directionalShadowMap,Ae.directionalShadowMatrix.value=U.state.directionalShadowMatrix,Ae.spotShadowMap.value=U.state.spotShadowMap,Ae.spotLightMatrix.value=U.state.spotLightMatrix,Ae.spotLightMap.value=U.state.spotLightMap,Ae.pointShadowMap.value=U.state.pointShadowMap,Ae.pointShadowMatrix.value=U.state.pointShadowMatrix),H.currentProgram=Oe,H.uniformsList=null,Oe}function Hf(E){if(E.uniformsList===null){const k=E.currentProgram.getUniforms();E.uniformsList=Al.seqWithValue(k.seq,E.uniforms)}return E.uniformsList}function Vf(E,k){const B=De.get(E);B.outputColorSpace=k.outputColorSpace,B.batching=k.batching,B.batchingColor=k.batchingColor,B.instancing=k.instancing,B.instancingColor=k.instancingColor,B.instancingMorph=k.instancingMorph,B.skinning=k.skinning,B.morphTargets=k.morphTargets,B.morphNormals=k.morphNormals,B.morphColors=k.morphColors,B.morphTargetsCount=k.morphTargetsCount,B.numClippingPlanes=k.numClippingPlanes,B.numIntersection=k.numClipIntersection,B.vertexAlphas=k.vertexAlphas,B.vertexTangents=k.vertexTangents,B.toneMapping=k.toneMapping}function Ly(E,k,B,H,U){k.isScene!==!0&&(k=je),R.resetTextureUnits();const se=k.fog,pe=H.isMeshStandardMaterial?k.environment:null,Ee=b===null?x.outputColorSpace:b.isXRRenderTarget===!0?b.texture.colorSpace:Jo,Te=(H.isMeshStandardMaterial?O:M).get(H.envMap||pe),ke=H.vertexColors===!0&&!!B.attributes.color&&B.attributes.color.itemSize===4,Oe=!!B.attributes.tangent&&(!!H.normalMap||H.anisotropy>0),Ae=!!B.morphAttributes.position,Ze=!!B.morphAttributes.normal,at=!!B.morphAttributes.color;let ut=ir;H.toneMapped&&(b===null||b.isXRRenderTarget===!0)&&(ut=x.toneMapping);const rn=B.morphAttributes.position||B.morphAttributes.normal||B.morphAttributes.color,Je=rn!==void 0?rn.length:0,be=De.get(H),ai=u.state.lights;if(re===!0&&(he===!0||E!==S)){const wn=E===S&&H.id===w;oe.setState(H,E,wn)}let Qe=!1;H.version===be.__version?(be.needsLights&&be.lightsStateVersion!==ai.state.version||be.outputColorSpace!==Ee||U.isBatchedMesh&&be.batching===!1||!U.isBatchedMesh&&be.batching===!0||U.isBatchedMesh&&be.batchingColor===!0&&U.colorTexture===null||U.isBatchedMesh&&be.batchingColor===!1&&U.colorTexture!==null||U.isInstancedMesh&&be.instancing===!1||!U.isInstancedMesh&&be.instancing===!0||U.isSkinnedMesh&&be.skinning===!1||!U.isSkinnedMesh&&be.skinning===!0||U.isInstancedMesh&&be.instancingColor===!0&&U.instanceColor===null||U.isInstancedMesh&&be.instancingColor===!1&&U.instanceColor!==null||U.isInstancedMesh&&be.instancingMorph===!0&&U.morphTexture===null||U.isInstancedMesh&&be.instancingMorph===!1&&U.morphTexture!==null||be.envMap!==Te||H.fog===!0&&be.fog!==se||be.numClippingPlanes!==void 0&&(be.numClippingPlanes!==oe.numPlanes||be.numIntersection!==oe.numIntersection)||be.vertexAlphas!==ke||be.vertexTangents!==Oe||be.morphTargets!==Ae||be.morphNormals!==Ze||be.morphColors!==at||be.toneMapping!==ut||be.morphTargetsCount!==Je)&&(Qe=!0):(Qe=!0,be.__version=H.version);let Un=be.currentProgram;Qe===!0&&(Un=Sa(H,k,U));let Vr=!1,hn=!1,is=!1;const dt=Un.getUniforms(),Kn=be.uniforms;if(Me.useProgram(Un.program)&&(Vr=!0,hn=!0,is=!0),H.id!==w&&(w=H.id,hn=!0),Vr||S!==E){Me.buffers.depth.getReversed()?(ae.copy(E.projectionMatrix),pM(ae),mM(ae),dt.setValue(I,"projectionMatrix",ae)):dt.setValue(I,"projectionMatrix",E.projectionMatrix),dt.setValue(I,"viewMatrix",E.matrixWorldInverse);const bi=dt.map.cameraPosition;bi!==void 0&&bi.setValue(I,ne.setFromMatrixPosition(E.matrixWorld)),Ge.logarithmicDepthBuffer&&dt.setValue(I,"logDepthBufFC",2/(Math.log(E.far+1)/Math.LN2)),(H.isMeshPhongMaterial||H.isMeshToonMaterial||H.isMeshLambertMaterial||H.isMeshBasicMaterial||H.isMeshStandardMaterial||H.isShaderMaterial)&&dt.setValue(I,"isOrthographic",E.isOrthographicCamera===!0),S!==E&&(S=E,hn=!0,is=!0)}if(U.isSkinnedMesh){dt.setOptional(I,U,"bindMatrix"),dt.setOptional(I,U,"bindMatrixInverse");const wn=U.skeleton;wn&&(wn.boneTexture===null&&wn.computeBoneTexture(),dt.setValue(I,"boneTexture",wn.boneTexture,R))}U.isBatchedMesh&&(dt.setOptional(I,U,"batchingTexture"),dt.setValue(I,"batchingTexture",U._matricesTexture,R),dt.setOptional(I,U,"batchingIdTexture"),dt.setValue(I,"batchingIdTexture",U._indirectTexture,R),dt.setOptional(I,U,"batchingColorTexture"),U._colorsTexture!==null&&dt.setValue(I,"batchingColorTexture",U._colorsTexture,R));const rs=B.morphAttributes;if((rs.position!==void 0||rs.normal!==void 0||rs.color!==void 0)&&Ie.update(U,B,Un),(hn||be.receiveShadow!==U.receiveShadow)&&(be.receiveShadow=U.receiveShadow,dt.setValue(I,"receiveShadow",U.receiveShadow)),H.isMeshGouraudMaterial&&H.envMap!==null&&(Kn.envMap.value=Te,Kn.flipEnvMap.value=Te.isCubeTexture&&Te.isRenderTargetTexture===!1?-1:1),H.isMeshStandardMaterial&&H.envMap===null&&k.environment!==null&&(Kn.envMapIntensity.value=k.environmentIntensity),hn&&(dt.setValue(I,"toneMappingExposure",x.toneMappingExposure),be.needsLights&&Ny(Kn,is),se&&H.fog===!0&&de.refreshFogUniforms(Kn,se),de.refreshMaterialUniforms(Kn,H,D,$,u.state.transmissionRenderTarget[E.id]),Al.upload(I,Hf(be),Kn,R)),H.isShaderMaterial&&H.uniformsNeedUpdate===!0&&(Al.upload(I,Hf(be),Kn,R),H.uniformsNeedUpdate=!1),H.isSpriteMaterial&&dt.setValue(I,"center",U.center),dt.setValue(I,"modelViewMatrix",U.modelViewMatrix),dt.setValue(I,"normalMatrix",U.normalMatrix),dt.setValue(I,"modelMatrix",U.matrixWorld),H.isShaderMaterial||H.isRawShaderMaterial){const wn=H.uniformsGroups;for(let bi=0,Ri=wn.length;bi<Ri;bi++){const Gf=wn[bi];N.update(Gf,Un),N.bind(Gf,Un)}}return Un}function Ny(E,k){E.ambientLightColor.needsUpdate=k,E.lightProbe.needsUpdate=k,E.directionalLights.needsUpdate=k,E.directionalLightShadows.needsUpdate=k,E.pointLights.needsUpdate=k,E.pointLightShadows.needsUpdate=k,E.spotLights.needsUpdate=k,E.spotLightShadows.needsUpdate=k,E.rectAreaLights.needsUpdate=k,E.hemisphereLights.needsUpdate=k}function Iy(E){return E.isMeshLambertMaterial||E.isMeshToonMaterial||E.isMeshPhongMaterial||E.isMeshStandardMaterial||E.isShadowMaterial||E.isShaderMaterial&&E.lights===!0}this.getActiveCubeFace=function(){return A},this.getActiveMipmapLevel=function(){return T},this.getRenderTarget=function(){return b},this.setRenderTargetTextures=function(E,k,B){De.get(E.texture).__webglTexture=k,De.get(E.depthTexture).__webglTexture=B;const H=De.get(E);H.__hasExternalTextures=!0,H.__autoAllocateDepthBuffer=B===void 0,H.__autoAllocateDepthBuffer||Xe.has("WEBGL_multisampled_render_to_texture")===!0&&(console.warn("THREE.WebGLRenderer: Render-to-texture extension was disabled because an external texture was provided"),H.__useRenderToTexture=!1)},this.setRenderTargetFramebuffer=function(E,k){const B=De.get(E);B.__webglFramebuffer=k,B.__useDefaultFramebuffer=k===void 0},this.setRenderTarget=function(E,k=0,B=0){b=E,A=k,T=B;let H=!0,U=null,se=!1,pe=!1;if(E){const Te=De.get(E);if(Te.__useDefaultFramebuffer!==void 0)Me.bindFramebuffer(I.FRAMEBUFFER,null),H=!1;else if(Te.__webglFramebuffer===void 0)R.setupRenderTarget(E);else if(Te.__hasExternalTextures)R.rebindTextures(E,De.get(E.texture).__webglTexture,De.get(E.depthTexture).__webglTexture);else if(E.depthBuffer){const Ae=E.depthTexture;if(Te.__boundDepthTexture!==Ae){if(Ae!==null&&De.has(Ae)&&(E.width!==Ae.image.width||E.height!==Ae.image.height))throw new Error("WebGLRenderTarget: Attached DepthTexture is initialized to the incorrect size.");R.setupDepthRenderbuffer(E)}}const ke=E.texture;(ke.isData3DTexture||ke.isDataArrayTexture||ke.isCompressedArrayTexture)&&(pe=!0);const Oe=De.get(E).__webglFramebuffer;E.isWebGLCubeRenderTarget?(Array.isArray(Oe[k])?U=Oe[k][B]:U=Oe[k],se=!0):E.samples>0&&R.useMultisampledRTT(E)===!1?U=De.get(E).__webglMultisampledFramebuffer:Array.isArray(Oe)?U=Oe[B]:U=Oe,P.copy(E.viewport),z.copy(E.scissor),F=E.scissorTest}else P.copy(le).multiplyScalar(D).floor(),z.copy(Se).multiplyScalar(D).floor(),F=Ne;if(Me.bindFramebuffer(I.FRAMEBUFFER,U)&&H&&Me.drawBuffers(E,U),Me.viewport(P),Me.scissor(z),Me.setScissorTest(F),se){const Te=De.get(E.texture);I.framebufferTexture2D(I.FRAMEBUFFER,I.COLOR_ATTACHMENT0,I.TEXTURE_CUBE_MAP_POSITIVE_X+k,Te.__webglTexture,B)}else if(pe){const Te=De.get(E.texture),ke=k||0;I.framebufferTextureLayer(I.FRAMEBUFFER,I.COLOR_ATTACHMENT0,Te.__webglTexture,B||0,ke)}w=-1},this.readRenderTargetPixels=function(E,k,B,H,U,se,pe){if(!(E&&E.isWebGLRenderTarget)){console.error("THREE.WebGLRenderer.readRenderTargetPixels: renderTarget is not THREE.WebGLRenderTarget.");return}let Ee=De.get(E).__webglFramebuffer;if(E.isWebGLCubeRenderTarget&&pe!==void 0&&(Ee=Ee[pe]),Ee){Me.bindFramebuffer(I.FRAMEBUFFER,Ee);try{const Te=E.texture,ke=Te.format,Oe=Te.type;if(!Ge.textureFormatReadable(ke)){console.error("THREE.WebGLRenderer.readRenderTargetPixels: renderTarget is not in RGBA or implementation defined format.");return}if(!Ge.textureTypeReadable(Oe)){console.error("THREE.WebGLRenderer.readRenderTargetPixels: renderTarget is not in UnsignedByteType or implementation defined type.");return}k>=0&&k<=E.width-H&&B>=0&&B<=E.height-U&&I.readPixels(k,B,H,U,He.convert(ke),He.convert(Oe),se)}finally{const Te=b!==null?De.get(b).__webglFramebuffer:null;Me.bindFramebuffer(I.FRAMEBUFFER,Te)}}},this.readRenderTargetPixelsAsync=async function(E,k,B,H,U,se,pe){if(!(E&&E.isWebGLRenderTarget))throw new Error("THREE.WebGLRenderer.readRenderTargetPixels: renderTarget is not THREE.WebGLRenderTarget.");let Ee=De.get(E).__webglFramebuffer;if(E.isWebGLCubeRenderTarget&&pe!==void 0&&(Ee=Ee[pe]),Ee){const Te=E.texture,ke=Te.format,Oe=Te.type;if(!Ge.textureFormatReadable(ke))throw new Error("THREE.WebGLRenderer.readRenderTargetPixelsAsync: renderTarget is not in RGBA or implementation defined format.");if(!Ge.textureTypeReadable(Oe))throw new Error("THREE.WebGLRenderer.readRenderTargetPixelsAsync: renderTarget is not in UnsignedByteType or implementation defined type.");if(k>=0&&k<=E.width-H&&B>=0&&B<=E.height-U){Me.bindFramebuffer(I.FRAMEBUFFER,Ee);const Ae=I.createBuffer();I.bindBuffer(I.PIXEL_PACK_BUFFER,Ae),I.bufferData(I.PIXEL_PACK_BUFFER,se.byteLength,I.STREAM_READ),I.readPixels(k,B,H,U,He.convert(ke),He.convert(Oe),0);const Ze=b!==null?De.get(b).__webglFramebuffer:null;Me.bindFramebuffer(I.FRAMEBUFFER,Ze);const at=I.fenceSync(I.SYNC_GPU_COMMANDS_COMPLETE,0);return I.flush(),await fM(I,at,4),I.bindBuffer(I.PIXEL_PACK_BUFFER,Ae),I.getBufferSubData(I.PIXEL_PACK_BUFFER,0,se),I.deleteBuffer(Ae),I.deleteSync(at),se}else throw new Error("THREE.WebGLRenderer.readRenderTargetPixelsAsync: requested read bounds are out of range.")}},this.copyFramebufferToTexture=function(E,k=null,B=0){E.isTexture!==!0&&(Rs("WebGLRenderer: copyFramebufferToTexture function signature has changed."),k=arguments[0]||null,E=arguments[1]);const H=Math.pow(2,-B),U=Math.floor(E.image.width*H),se=Math.floor(E.image.height*H),pe=k!==null?k.x:0,Ee=k!==null?k.y:0;R.setTexture2D(E,0),I.copyTexSubImage2D(I.TEXTURE_2D,B,0,0,pe,Ee,U,se),Me.unbindTexture()},this.copyTextureToTexture=function(E,k,B=null,H=null,U=0){E.isTexture!==!0&&(Rs("WebGLRenderer: copyTextureToTexture function signature has changed."),H=arguments[0]||null,E=arguments[1],k=arguments[2],U=arguments[3]||0,B=null);let se,pe,Ee,Te,ke,Oe,Ae,Ze,at;const ut=E.isCompressedTexture?E.mipmaps[U]:E.image;B!==null?(se=B.max.x-B.min.x,pe=B.max.y-B.min.y,Ee=B.isBox3?B.max.z-B.min.z:1,Te=B.min.x,ke=B.min.y,Oe=B.isBox3?B.min.z:0):(se=ut.width,pe=ut.height,Ee=ut.depth||1,Te=0,ke=0,Oe=0),H!==null?(Ae=H.x,Ze=H.y,at=H.z):(Ae=0,Ze=0,at=0);const rn=He.convert(k.format),Je=He.convert(k.type);let be;k.isData3DTexture?(R.setTexture3D(k,0),be=I.TEXTURE_3D):k.isDataArrayTexture||k.isCompressedArrayTexture?(R.setTexture2DArray(k,0),be=I.TEXTURE_2D_ARRAY):(R.setTexture2D(k,0),be=I.TEXTURE_2D),I.pixelStorei(I.UNPACK_FLIP_Y_WEBGL,k.flipY),I.pixelStorei(I.UNPACK_PREMULTIPLY_ALPHA_WEBGL,k.premultiplyAlpha),I.pixelStorei(I.UNPACK_ALIGNMENT,k.unpackAlignment);const ai=I.getParameter(I.UNPACK_ROW_LENGTH),Qe=I.getParameter(I.UNPACK_IMAGE_HEIGHT),Un=I.getParameter(I.UNPACK_SKIP_PIXELS),Vr=I.getParameter(I.UNPACK_SKIP_ROWS),hn=I.getParameter(I.UNPACK_SKIP_IMAGES);I.pixelStorei(I.UNPACK_ROW_LENGTH,ut.width),I.pixelStorei(I.UNPACK_IMAGE_HEIGHT,ut.height),I.pixelStorei(I.UNPACK_SKIP_PIXELS,Te),I.pixelStorei(I.UNPACK_SKIP_ROWS,ke),I.pixelStorei(I.UNPACK_SKIP_IMAGES,Oe);const is=E.isDataArrayTexture||E.isData3DTexture,dt=k.isDataArrayTexture||k.isData3DTexture;if(E.isRenderTargetTexture||E.isDepthTexture){const Kn=De.get(E),rs=De.get(k),wn=De.get(Kn.__renderTarget),bi=De.get(rs.__renderTarget);Me.bindFramebuffer(I.READ_FRAMEBUFFER,wn.__webglFramebuffer),Me.bindFramebuffer(I.DRAW_FRAMEBUFFER,bi.__webglFramebuffer);for(let Ri=0;Ri<Ee;Ri++)is&&I.framebufferTextureLayer(I.READ_FRAMEBUFFER,I.COLOR_ATTACHMENT0,De.get(E).__webglTexture,U,Oe+Ri),E.isDepthTexture?(dt&&I.framebufferTextureLayer(I.DRAW_FRAMEBUFFER,I.COLOR_ATTACHMENT0,De.get(k).__webglTexture,U,at+Ri),I.blitFramebuffer(Te,ke,se,pe,Ae,Ze,se,pe,I.DEPTH_BUFFER_BIT,I.NEAREST)):dt?I.copyTexSubImage3D(be,U,Ae,Ze,at+Ri,Te,ke,se,pe):I.copyTexSubImage2D(be,U,Ae,Ze,at+Ri,Te,ke,se,pe);Me.bindFramebuffer(I.READ_FRAMEBUFFER,null),Me.bindFramebuffer(I.DRAW_FRAMEBUFFER,null)}else dt?E.isDataTexture||E.isData3DTexture?I.texSubImage3D(be,U,Ae,Ze,at,se,pe,Ee,rn,Je,ut.data):k.isCompressedArrayTexture?I.compressedTexSubImage3D(be,U,Ae,Ze,at,se,pe,Ee,rn,ut.data):I.texSubImage3D(be,U,Ae,Ze,at,se,pe,Ee,rn,Je,ut):E.isDataTexture?I.texSubImage2D(I.TEXTURE_2D,U,Ae,Ze,se,pe,rn,Je,ut.data):E.isCompressedTexture?I.compressedTexSubImage2D(I.TEXTURE_2D,U,Ae,Ze,ut.width,ut.height,rn,ut.data):I.texSubImage2D(I.TEXTURE_2D,U,Ae,Ze,se,pe,rn,Je,ut);I.pixelStorei(I.UNPACK_ROW_LENGTH,ai),I.pixelStorei(I.UNPACK_IMAGE_HEIGHT,Qe),I.pixelStorei(I.UNPACK_SKIP_PIXELS,Un),I.pixelStorei(I.UNPACK_SKIP_ROWS,Vr),I.pixelStorei(I.UNPACK_SKIP_IMAGES,hn),U===0&&k.generateMipmaps&&I.generateMipmap(be),Me.unbindTexture()},this.copyTextureToTexture3D=function(E,k,B=null,H=null,U=0){return E.isTexture!==!0&&(Rs("WebGLRenderer: copyTextureToTexture3D function signature has changed."),B=arguments[0]||null,H=arguments[1]||null,E=arguments[2],k=arguments[3],U=arguments[4]||0),Rs('WebGLRenderer: copyTextureToTexture3D function has been deprecated. Use "copyTextureToTexture" instead.'),this.copyTextureToTexture(E,k,B,H,U)},this.initRenderTarget=function(E){De.get(E).__webglFramebuffer===void 0&&R.setupRenderTarget(E)},this.initTexture=function(E){E.isCubeTexture?R.setTextureCube(E,0):E.isData3DTexture?R.setTexture3D(E,0):E.isDataArrayTexture||E.isCompressedArrayTexture?R.setTexture2DArray(E,0):R.setTexture2D(E,0),Me.unbindTexture()},this.resetState=function(){A=0,T=0,b=null,Me.reset(),st.reset()},typeof __THREE_DEVTOOLS__<"u"&&__THREE_DEVTOOLS__.dispatchEvent(new CustomEvent("observe",{detail:this}))}get coordinateSystem(){return _i}get outputColorSpace(){return this._outputColorSpace}set outputColorSpace(e){this._outputColorSpace=e;const n=this.getContext();n.drawingBufferColorspace=$e._getDrawingBufferColorSpace(e),n.unpackColorSpace=$e._getUnpackColorSpace()}}class bf{constructor(e,n=25e-5){this.isFogExp2=!0,this.name="",this.color=new we(e),this.density=n}clone(){return new bf(this.color,this.density)}toJSON(){return{type:"FogExp2",name:this.name,color:this.color.getHex(),density:this.density}}}class Bs extends Xt{constructor(){super(),this.isScene=!0,this.type="Scene",this.background=null,this.environment=null,this.fog=null,this.backgroundBlurriness=0,this.backgroundIntensity=1,this.backgroundRotation=new $n,this.environmentIntensity=1,this.environmentRotation=new $n,this.overrideMaterial=null,typeof __THREE_DEVTOOLS__<"u"&&__THREE_DEVTOOLS__.dispatchEvent(new CustomEvent("observe",{detail:this}))}copy(e,n){return super.copy(e,n),e.background!==null&&(this.background=e.background.clone()),e.environment!==null&&(this.environment=e.environment.clone()),e.fog!==null&&(this.fog=e.fog.clone()),this.backgroundBlurriness=e.backgroundBlurriness,this.backgroundIntensity=e.backgroundIntensity,this.backgroundRotation.copy(e.backgroundRotation),this.environmentIntensity=e.environmentIntensity,this.environmentRotation.copy(e.environmentRotation),e.overrideMaterial!==null&&(this.overrideMaterial=e.overrideMaterial.clone()),this.matrixAutoUpdate=e.matrixAutoUpdate,this}toJSON(e){const n=super.toJSON(e);return this.fog!==null&&(n.object.fog=this.fog.toJSON()),this.backgroundBlurriness>0&&(n.object.backgroundBlurriness=this.backgroundBlurriness),this.backgroundIntensity!==1&&(n.object.backgroundIntensity=this.backgroundIntensity),n.object.backgroundRotation=this.backgroundRotation.toArray(),this.environmentIntensity!==1&&(n.object.environmentIntensity=this.environmentIntensity),n.object.environmentRotation=this.environmentRotation.toArray(),n}}class Ec extends Qt{constructor(e=null,n=1,i=1,r,o,s,a,l,c=dn,d=dn,f,h){super(null,s,a,l,c,d,r,o,f,h),this.isDataTexture=!0,this.image={data:e,width:n,height:i},this.generateMipmaps=!1,this.flipY=!1,this.unpackAlignment=1}}class Km extends en{constructor(e,n,i,r=1){super(e,n,i),this.isInstancedBufferAttribute=!0,this.meshPerAttribute=r}copy(e){return super.copy(e),this.meshPerAttribute=e.meshPerAttribute,this}toJSON(){const e=super.toJSON();return e.meshPerAttribute=this.meshPerAttribute,e.isInstancedBufferAttribute=!0,e}}const ro=new rt,Zm=new rt,sl=[],Jm=new Br,AA=new rt,ys=new Ct,_s=new va;class CA extends Ct{constructor(e,n,i){super(e,n),this.isInstancedMesh=!0,this.instanceMatrix=new Km(new Float32Array(i*16),16),this.instanceColor=null,this.morphTexture=null,this.count=i,this.boundingBox=null,this.boundingSphere=null;for(let r=0;r<i;r++)this.setMatrixAt(r,AA)}computeBoundingBox(){const e=this.geometry,n=this.count;this.boundingBox===null&&(this.boundingBox=new Br),e.boundingBox===null&&e.computeBoundingBox(),this.boundingBox.makeEmpty();for(let i=0;i<n;i++)this.getMatrixAt(i,ro),Jm.copy(e.boundingBox).applyMatrix4(ro),this.boundingBox.union(Jm)}computeBoundingSphere(){const e=this.geometry,n=this.count;this.boundingSphere===null&&(this.boundingSphere=new va),e.boundingSphere===null&&e.computeBoundingSphere(),this.boundingSphere.makeEmpty();for(let i=0;i<n;i++)this.getMatrixAt(i,ro),_s.copy(e.boundingSphere).applyMatrix4(ro),this.boundingSphere.union(_s)}copy(e,n){return super.copy(e,n),this.instanceMatrix.copy(e.instanceMatrix),e.morphTexture!==null&&(this.morphTexture=e.morphTexture.clone()),e.instanceColor!==null&&(this.instanceColor=e.instanceColor.clone()),this.count=e.count,e.boundingBox!==null&&(this.boundingBox=e.boundingBox.clone()),e.boundingSphere!==null&&(this.boundingSphere=e.boundingSphere.clone()),this}getColorAt(e,n){n.fromArray(this.instanceColor.array,e*3)}getMatrixAt(e,n){n.fromArray(this.instanceMatrix.array,e*16)}getMorphAt(e,n){const i=n.morphTargetInfluences,r=this.morphTexture.source.data.data,o=i.length+1,s=e*o+1;for(let a=0;a<i.length;a++)i[a]=r[s+a]}raycast(e,n){const i=this.matrixWorld,r=this.count;if(ys.geometry=this.geometry,ys.material=this.material,ys.material!==void 0&&(this.boundingSphere===null&&this.computeBoundingSphere(),_s.copy(this.boundingSphere),_s.applyMatrix4(i),e.ray.intersectsSphere(_s)!==!1))for(let o=0;o<r;o++){this.getMatrixAt(o,ro),Zm.multiplyMatrices(i,ro),ys.matrixWorld=Zm,ys.raycast(e,sl);for(let s=0,a=sl.length;s<a;s++){const l=sl[s];l.instanceId=o,l.object=this,n.push(l)}sl.length=0}}setColorAt(e,n){this.instanceColor===null&&(this.instanceColor=new Km(new Float32Array(this.instanceMatrix.count*3).fill(1),3)),n.toArray(this.instanceColor.array,e*3)}setMatrixAt(e,n){n.toArray(this.instanceMatrix.array,e*16)}setMorphAt(e,n){const i=n.morphTargetInfluences,r=i.length+1;this.morphTexture===null&&(this.morphTexture=new Ec(new Float32Array(r*this.count),r,this.count,Mf,Pn));const o=this.morphTexture.source.data.data;let s=0;for(let c=0;c<i.length;c++)s+=i[c];const a=this.geometry.morphTargetsRelative?1:1-s,l=r*e;o[l]=a,o.set(i,l+1)}updateMorphTargets(){}dispose(){return this.dispatchEvent({type:"dispose"}),this.morphTexture!==null&&(this.morphTexture.dispose(),this.morphTexture=null),this}}class Ci{constructor(){this.type="Curve",this.arcLengthDivisions=200}getPoint(){return console.warn("THREE.Curve: .getPoint() not implemented."),null}getPointAt(e,n){const i=this.getUtoTmapping(e);return this.getPoint(i,n)}getPoints(e=5){const n=[];for(let i=0;i<=e;i++)n.push(this.getPoint(i/e));return n}getSpacedPoints(e=5){const n=[];for(let i=0;i<=e;i++)n.push(this.getPointAt(i/e));return n}getLength(){const e=this.getLengths();return e[e.length-1]}getLengths(e=this.arcLengthDivisions){if(this.cacheArcLengths&&this.cacheArcLengths.length===e+1&&!this.needsUpdate)return this.cacheArcLengths;this.needsUpdate=!1;const n=[];let i,r=this.getPoint(0),o=0;n.push(0);for(let s=1;s<=e;s++)i=this.getPoint(s/e),o+=i.distanceTo(r),n.push(o),r=i;return this.cacheArcLengths=n,n}updateArcLengths(){this.needsUpdate=!0,this.getLengths()}getUtoTmapping(e,n){const i=this.getLengths();let r=0;const o=i.length;let s;n?s=n:s=e*i[o-1];let a=0,l=o-1,c;for(;a<=l;)if(r=Math.floor(a+(l-a)/2),c=i[r]-s,c<0)a=r+1;else if(c>0)l=r-1;else{l=r;break}if(r=l,i[r]===s)return r/(o-1);const d=i[r],h=i[r+1]-d,p=(s-d)/h;return(r+p)/(o-1)}getTangent(e,n){let r=e-1e-4,o=e+1e-4;r<0&&(r=0),o>1&&(o=1);const s=this.getPoint(r),a=this.getPoint(o),l=n||(s.isVector2?new Re:new L);return l.copy(a).sub(s).normalize(),l}getTangentAt(e,n){const i=this.getUtoTmapping(e);return this.getTangent(i,n)}computeFrenetFrames(e,n){const i=new L,r=[],o=[],s=[],a=new L,l=new rt;for(let p=0;p<=e;p++){const v=p/e;r[p]=this.getTangentAt(v,new L)}o[0]=new L,s[0]=new L;let c=Number.MAX_VALUE;const d=Math.abs(r[0].x),f=Math.abs(r[0].y),h=Math.abs(r[0].z);d<=c&&(c=d,i.set(1,0,0)),f<=c&&(c=f,i.set(0,1,0)),h<=c&&i.set(0,0,1),a.crossVectors(r[0],i).normalize(),o[0].crossVectors(r[0],a),s[0].crossVectors(r[0],o[0]);for(let p=1;p<=e;p++){if(o[p]=o[p-1].clone(),s[p]=s[p-1].clone(),a.crossVectors(r[p-1],r[p]),a.length()>Number.EPSILON){a.normalize();const v=Math.acos(Gt(r[p-1].dot(r[p]),-1,1));o[p].applyMatrix4(l.makeRotationAxis(a,v))}s[p].crossVectors(r[p],o[p])}if(n===!0){let p=Math.acos(Gt(o[0].dot(o[e]),-1,1));p/=e,r[0].dot(a.crossVectors(o[0],o[e]))>0&&(p=-p);for(let v=1;v<=e;v++)o[v].applyMatrix4(l.makeRotationAxis(r[v],p*v)),s[v].crossVectors(r[v],o[v])}return{tangents:r,normals:o,binormals:s}}clone(){return new this.constructor().copy(this)}copy(e){return this.arcLengthDivisions=e.arcLengthDivisions,this}toJSON(){const e={metadata:{version:4.6,type:"Curve",generator:"Curve.toJSON"}};return e.arcLengthDivisions=this.arcLengthDivisions,e.type=this.type,e}fromJSON(e){return this.arcLengthDivisions=e.arcLengthDivisions,this}}class py extends Ci{constructor(e=0,n=0,i=1,r=1,o=0,s=Math.PI*2,a=!1,l=0){super(),this.isEllipseCurve=!0,this.type="EllipseCurve",this.aX=e,this.aY=n,this.xRadius=i,this.yRadius=r,this.aStartAngle=o,this.aEndAngle=s,this.aClockwise=a,this.aRotation=l}getPoint(e,n=new Re){const i=n,r=Math.PI*2;let o=this.aEndAngle-this.aStartAngle;const s=Math.abs(o)<Number.EPSILON;for(;o<0;)o+=r;for(;o>r;)o-=r;o<Number.EPSILON&&(s?o=0:o=r),this.aClockwise===!0&&!s&&(o===r?o=-r:o=o-r);const a=this.aStartAngle+e*o;let l=this.aX+this.xRadius*Math.cos(a),c=this.aY+this.yRadius*Math.sin(a);if(this.aRotation!==0){const d=Math.cos(this.aRotation),f=Math.sin(this.aRotation),h=l-this.aX,p=c-this.aY;l=h*d-p*f+this.aX,c=h*f+p*d+this.aY}return i.set(l,c)}copy(e){return super.copy(e),this.aX=e.aX,this.aY=e.aY,this.xRadius=e.xRadius,this.yRadius=e.yRadius,this.aStartAngle=e.aStartAngle,this.aEndAngle=e.aEndAngle,this.aClockwise=e.aClockwise,this.aRotation=e.aRotation,this}toJSON(){const e=super.toJSON();return e.aX=this.aX,e.aY=this.aY,e.xRadius=this.xRadius,e.yRadius=this.yRadius,e.aStartAngle=this.aStartAngle,e.aEndAngle=this.aEndAngle,e.aClockwise=this.aClockwise,e.aRotation=this.aRotation,e}fromJSON(e){return super.fromJSON(e),this.aX=e.aX,this.aY=e.aY,this.xRadius=e.xRadius,this.yRadius=e.yRadius,this.aStartAngle=e.aStartAngle,this.aEndAngle=e.aEndAngle,this.aClockwise=e.aClockwise,this.aRotation=e.aRotation,this}}class bA extends py{constructor(e,n,i,r,o,s){super(e,n,i,i,r,o,s),this.isArcCurve=!0,this.type="ArcCurve"}}function Rf(){let t=0,e=0,n=0,i=0;function r(o,s,a,l){t=o,e=a,n=-3*o+3*s-2*a-l,i=2*o-2*s+a+l}return{initCatmullRom:function(o,s,a,l,c){r(s,a,c*(a-o),c*(l-s))},initNonuniformCatmullRom:function(o,s,a,l,c,d,f){let h=(s-o)/c-(a-o)/(c+d)+(a-s)/d,p=(a-s)/d-(l-s)/(d+f)+(l-a)/f;h*=d,p*=d,r(s,a,h,p)},calc:function(o){const s=o*o,a=s*o;return t+e*o+n*s+i*a}}}const al=new L,Du=new Rf,Lu=new Rf,Nu=new Rf;class my extends Ci{constructor(e=[],n=!1,i="centripetal",r=.5){super(),this.isCatmullRomCurve3=!0,this.type="CatmullRomCurve3",this.points=e,this.closed=n,this.curveType=i,this.tension=r}getPoint(e,n=new L){const i=n,r=this.points,o=r.length,s=(o-(this.closed?0:1))*e;let a=Math.floor(s),l=s-a;this.closed?a+=a>0?0:(Math.floor(Math.abs(a)/o)+1)*o:l===0&&a===o-1&&(a=o-2,l=1);let c,d;this.closed||a>0?c=r[(a-1)%o]:(al.subVectors(r[0],r[1]).add(r[0]),c=al);const f=r[a%o],h=r[(a+1)%o];if(this.closed||a+2<o?d=r[(a+2)%o]:(al.subVectors(r[o-1],r[o-2]).add(r[o-1]),d=al),this.curveType==="centripetal"||this.curveType==="chordal"){const p=this.curveType==="chordal"?.5:.25;let v=Math.pow(c.distanceToSquared(f),p),_=Math.pow(f.distanceToSquared(h),p),m=Math.pow(h.distanceToSquared(d),p);_<1e-4&&(_=1),v<1e-4&&(v=_),m<1e-4&&(m=_),Du.initNonuniformCatmullRom(c.x,f.x,h.x,d.x,v,_,m),Lu.initNonuniformCatmullRom(c.y,f.y,h.y,d.y,v,_,m),Nu.initNonuniformCatmullRom(c.z,f.z,h.z,d.z,v,_,m)}else this.curveType==="catmullrom"&&(Du.initCatmullRom(c.x,f.x,h.x,d.x,this.tension),Lu.initCatmullRom(c.y,f.y,h.y,d.y,this.tension),Nu.initCatmullRom(c.z,f.z,h.z,d.z,this.tension));return i.set(Du.calc(l),Lu.calc(l),Nu.calc(l)),i}copy(e){super.copy(e),this.points=[];for(let n=0,i=e.points.length;n<i;n++){const r=e.points[n];this.points.push(r.clone())}return this.closed=e.closed,this.curveType=e.curveType,this.tension=e.tension,this}toJSON(){const e=super.toJSON();e.points=[];for(let n=0,i=this.points.length;n<i;n++){const r=this.points[n];e.points.push(r.toArray())}return e.closed=this.closed,e.curveType=this.curveType,e.tension=this.tension,e}fromJSON(e){super.fromJSON(e),this.points=[];for(let n=0,i=e.points.length;n<i;n++){const r=e.points[n];this.points.push(new L().fromArray(r))}return this.closed=e.closed,this.curveType=e.curveType,this.tension=e.tension,this}}function Qm(t,e,n,i,r){const o=(i-e)*.5,s=(r-n)*.5,a=t*t,l=t*a;return(2*n-2*i+o+s)*l+(-3*n+3*i-2*o-s)*a+o*t+n}function RA(t,e){const n=1-t;return n*n*e}function PA(t,e){return 2*(1-t)*t*e}function DA(t,e){return t*t*e}function Hs(t,e,n,i){return RA(t,e)+PA(t,n)+DA(t,i)}function LA(t,e){const n=1-t;return n*n*n*e}function NA(t,e){const n=1-t;return 3*n*n*t*e}function IA(t,e){return 3*(1-t)*t*t*e}function kA(t,e){return t*t*t*e}function Vs(t,e,n,i,r){return LA(t,e)+NA(t,n)+IA(t,i)+kA(t,r)}class UA extends Ci{constructor(e=new Re,n=new Re,i=new Re,r=new Re){super(),this.isCubicBezierCurve=!0,this.type="CubicBezierCurve",this.v0=e,this.v1=n,this.v2=i,this.v3=r}getPoint(e,n=new Re){const i=n,r=this.v0,o=this.v1,s=this.v2,a=this.v3;return i.set(Vs(e,r.x,o.x,s.x,a.x),Vs(e,r.y,o.y,s.y,a.y)),i}copy(e){return super.copy(e),this.v0.copy(e.v0),this.v1.copy(e.v1),this.v2.copy(e.v2),this.v3.copy(e.v3),this}toJSON(){const e=super.toJSON();return e.v0=this.v0.toArray(),e.v1=this.v1.toArray(),e.v2=this.v2.toArray(),e.v3=this.v3.toArray(),e}fromJSON(e){return super.fromJSON(e),this.v0.fromArray(e.v0),this.v1.fromArray(e.v1),this.v2.fromArray(e.v2),this.v3.fromArray(e.v3),this}}class FA extends Ci{constructor(e=new L,n=new L,i=new L,r=new L){super(),this.isCubicBezierCurve3=!0,this.type="CubicBezierCurve3",this.v0=e,this.v1=n,this.v2=i,this.v3=r}getPoint(e,n=new L){const i=n,r=this.v0,o=this.v1,s=this.v2,a=this.v3;return i.set(Vs(e,r.x,o.x,s.x,a.x),Vs(e,r.y,o.y,s.y,a.y),Vs(e,r.z,o.z,s.z,a.z)),i}copy(e){return super.copy(e),this.v0.copy(e.v0),this.v1.copy(e.v1),this.v2.copy(e.v2),this.v3.copy(e.v3),this}toJSON(){const e=super.toJSON();return e.v0=this.v0.toArray(),e.v1=this.v1.toArray(),e.v2=this.v2.toArray(),e.v3=this.v3.toArray(),e}fromJSON(e){return super.fromJSON(e),this.v0.fromArray(e.v0),this.v1.fromArray(e.v1),this.v2.fromArray(e.v2),this.v3.fromArray(e.v3),this}}class OA extends Ci{constructor(e=new Re,n=new Re){super(),this.isLineCurve=!0,this.type="LineCurve",this.v1=e,this.v2=n}getPoint(e,n=new Re){const i=n;return e===1?i.copy(this.v2):(i.copy(this.v2).sub(this.v1),i.multiplyScalar(e).add(this.v1)),i}getPointAt(e,n){return this.getPoint(e,n)}getTangent(e,n=new Re){return n.subVectors(this.v2,this.v1).normalize()}getTangentAt(e,n){return this.getTangent(e,n)}copy(e){return super.copy(e),this.v1.copy(e.v1),this.v2.copy(e.v2),this}toJSON(){const e=super.toJSON();return e.v1=this.v1.toArray(),e.v2=this.v2.toArray(),e}fromJSON(e){return super.fromJSON(e),this.v1.fromArray(e.v1),this.v2.fromArray(e.v2),this}}class zA extends Ci{constructor(e=new L,n=new L){super(),this.isLineCurve3=!0,this.type="LineCurve3",this.v1=e,this.v2=n}getPoint(e,n=new L){const i=n;return e===1?i.copy(this.v2):(i.copy(this.v2).sub(this.v1),i.multiplyScalar(e).add(this.v1)),i}getPointAt(e,n){return this.getPoint(e,n)}getTangent(e,n=new L){return n.subVectors(this.v2,this.v1).normalize()}getTangentAt(e,n){return this.getTangent(e,n)}copy(e){return super.copy(e),this.v1.copy(e.v1),this.v2.copy(e.v2),this}toJSON(){const e=super.toJSON();return e.v1=this.v1.toArray(),e.v2=this.v2.toArray(),e}fromJSON(e){return super.fromJSON(e),this.v1.fromArray(e.v1),this.v2.fromArray(e.v2),this}}class BA extends Ci{constructor(e=new Re,n=new Re,i=new Re){super(),this.isQuadraticBezierCurve=!0,this.type="QuadraticBezierCurve",this.v0=e,this.v1=n,this.v2=i}getPoint(e,n=new Re){const i=n,r=this.v0,o=this.v1,s=this.v2;return i.set(Hs(e,r.x,o.x,s.x),Hs(e,r.y,o.y,s.y)),i}copy(e){return super.copy(e),this.v0.copy(e.v0),this.v1.copy(e.v1),this.v2.copy(e.v2),this}toJSON(){const e=super.toJSON();return e.v0=this.v0.toArray(),e.v1=this.v1.toArray(),e.v2=this.v2.toArray(),e}fromJSON(e){return super.fromJSON(e),this.v0.fromArray(e.v0),this.v1.fromArray(e.v1),this.v2.fromArray(e.v2),this}}class gy extends Ci{constructor(e=new L,n=new L,i=new L){super(),this.isQuadraticBezierCurve3=!0,this.type="QuadraticBezierCurve3",this.v0=e,this.v1=n,this.v2=i}getPoint(e,n=new L){const i=n,r=this.v0,o=this.v1,s=this.v2;return i.set(Hs(e,r.x,o.x,s.x),Hs(e,r.y,o.y,s.y),Hs(e,r.z,o.z,s.z)),i}copy(e){return super.copy(e),this.v0.copy(e.v0),this.v1.copy(e.v1),this.v2.copy(e.v2),this}toJSON(){const e=super.toJSON();return e.v0=this.v0.toArray(),e.v1=this.v1.toArray(),e.v2=this.v2.toArray(),e}fromJSON(e){return super.fromJSON(e),this.v0.fromArray(e.v0),this.v1.fromArray(e.v1),this.v2.fromArray(e.v2),this}}class HA extends Ci{constructor(e=[]){super(),this.isSplineCurve=!0,this.type="SplineCurve",this.points=e}getPoint(e,n=new Re){const i=n,r=this.points,o=(r.length-1)*e,s=Math.floor(o),a=o-s,l=r[s===0?s:s-1],c=r[s],d=r[s>r.length-2?r.length-1:s+1],f=r[s>r.length-3?r.length-1:s+2];return i.set(Qm(a,l.x,c.x,d.x,f.x),Qm(a,l.y,c.y,d.y,f.y)),i}copy(e){super.copy(e),this.points=[];for(let n=0,i=e.points.length;n<i;n++){const r=e.points[n];this.points.push(r.clone())}return this}toJSON(){const e=super.toJSON();e.points=[];for(let n=0,i=this.points.length;n<i;n++){const r=this.points[n];e.points.push(r.toArray())}return e}fromJSON(e){super.fromJSON(e),this.points=[];for(let n=0,i=e.points.length;n<i;n++){const r=e.points[n];this.points.push(new Re().fromArray(r))}return this}}var VA=Object.freeze({__proto__:null,ArcCurve:bA,CatmullRomCurve3:my,CubicBezierCurve:UA,CubicBezierCurve3:FA,EllipseCurve:py,LineCurve:OA,LineCurve3:zA,QuadraticBezierCurve:BA,QuadraticBezierCurve3:gy,SplineCurve:HA});class Pf extends Yt{constructor(e=1,n=1,i=1,r=32,o=1,s=!1,a=0,l=Math.PI*2){super(),this.type="CylinderGeometry",this.parameters={radiusTop:e,radiusBottom:n,height:i,radialSegments:r,heightSegments:o,openEnded:s,thetaStart:a,thetaLength:l};const c=this;r=Math.floor(r),o=Math.floor(o);const d=[],f=[],h=[],p=[];let v=0;const _=[],m=i/2;let u=0;g(),s===!1&&(e>0&&y(!0),n>0&&y(!1)),this.setIndex(d),this.setAttribute("position",new ct(f,3)),this.setAttribute("normal",new ct(h,3)),this.setAttribute("uv",new ct(p,2));function g(){const x=new L,C=new L;let A=0;const T=(n-e)/i;for(let b=0;b<=o;b++){const w=[],S=b/o,P=S*(n-e)+e;for(let z=0;z<=r;z++){const F=z/r,Y=F*l+a,j=Math.sin(Y),W=Math.cos(Y);C.x=P*j,C.y=-S*i+m,C.z=P*W,f.push(C.x,C.y,C.z),x.set(j,T,W).normalize(),h.push(x.x,x.y,x.z),p.push(F,1-S),w.push(v++)}_.push(w)}for(let b=0;b<r;b++)for(let w=0;w<o;w++){const S=_[w][b],P=_[w+1][b],z=_[w+1][b+1],F=_[w][b+1];(e>0||w!==0)&&(d.push(S,P,F),A+=3),(n>0||w!==o-1)&&(d.push(P,z,F),A+=3)}c.addGroup(u,A,0),u+=A}function y(x){const C=v,A=new Re,T=new L;let b=0;const w=x===!0?e:n,S=x===!0?1:-1;for(let z=1;z<=r;z++)f.push(0,m*S,0),h.push(0,S,0),p.push(.5,.5),v++;const P=v;for(let z=0;z<=r;z++){const Y=z/r*l+a,j=Math.cos(Y),W=Math.sin(Y);T.x=w*W,T.y=m*S,T.z=w*j,f.push(T.x,T.y,T.z),h.push(0,S,0),A.x=j*.5+.5,A.y=W*.5*S+.5,p.push(A.x,A.y),v++}for(let z=0;z<r;z++){const F=C+z,Y=P+z;x===!0?d.push(Y,Y+1,F):d.push(Y+1,Y,F),b+=3}c.addGroup(u,b,x===!0?1:2),u+=b}}copy(e){return super.copy(e),this.parameters=Object.assign({},e.parameters),this}static fromJSON(e){return new Pf(e.radiusTop,e.radiusBottom,e.height,e.radialSegments,e.heightSegments,e.openEnded,e.thetaStart,e.thetaLength)}}class Tc extends Yt{constructor(e=1,n=32,i=16,r=0,o=Math.PI*2,s=0,a=Math.PI){super(),this.type="SphereGeometry",this.parameters={radius:e,widthSegments:n,heightSegments:i,phiStart:r,phiLength:o,thetaStart:s,thetaLength:a},n=Math.max(3,Math.floor(n)),i=Math.max(2,Math.floor(i));const l=Math.min(s+a,Math.PI);let c=0;const d=[],f=new L,h=new L,p=[],v=[],_=[],m=[];for(let u=0;u<=i;u++){const g=[],y=u/i;let x=0;u===0&&s===0?x=.5/n:u===i&&l===Math.PI&&(x=-.5/n);for(let C=0;C<=n;C++){const A=C/n;f.x=-e*Math.cos(r+A*o)*Math.sin(s+y*a),f.y=e*Math.cos(s+y*a),f.z=e*Math.sin(r+A*o)*Math.sin(s+y*a),v.push(f.x,f.y,f.z),h.copy(f).normalize(),_.push(h.x,h.y,h.z),m.push(A+x,1-y),g.push(c++)}d.push(g)}for(let u=0;u<i;u++)for(let g=0;g<n;g++){const y=d[u][g+1],x=d[u][g],C=d[u+1][g],A=d[u+1][g+1];(u!==0||s>0)&&p.push(y,x,A),(u!==i-1||l<Math.PI)&&p.push(x,C,A)}this.setIndex(p),this.setAttribute("position",new ct(v,3)),this.setAttribute("normal",new ct(_,3)),this.setAttribute("uv",new ct(m,2))}copy(e){return super.copy(e),this.parameters=Object.assign({},e.parameters),this}static fromJSON(e){return new Tc(e.radius,e.widthSegments,e.heightSegments,e.phiStart,e.phiLength,e.thetaStart,e.thetaLength)}}class Df extends Yt{constructor(e=1,n=.4,i=12,r=48,o=Math.PI*2){super(),this.type="TorusGeometry",this.parameters={radius:e,tube:n,radialSegments:i,tubularSegments:r,arc:o},i=Math.floor(i),r=Math.floor(r);const s=[],a=[],l=[],c=[],d=new L,f=new L,h=new L;for(let p=0;p<=i;p++)for(let v=0;v<=r;v++){const _=v/r*o,m=p/i*Math.PI*2;f.x=(e+n*Math.cos(m))*Math.cos(_),f.y=(e+n*Math.cos(m))*Math.sin(_),f.z=n*Math.sin(m),a.push(f.x,f.y,f.z),d.x=e*Math.cos(_),d.y=e*Math.sin(_),h.subVectors(f,d).normalize(),l.push(h.x,h.y,h.z),c.push(v/r),c.push(p/i)}for(let p=1;p<=i;p++)for(let v=1;v<=r;v++){const _=(r+1)*p+v-1,m=(r+1)*(p-1)+v-1,u=(r+1)*(p-1)+v,g=(r+1)*p+v;s.push(_,m,g),s.push(m,u,g)}this.setIndex(s),this.setAttribute("position",new ct(a,3)),this.setAttribute("normal",new ct(l,3)),this.setAttribute("uv",new ct(c,2))}copy(e){return super.copy(e),this.parameters=Object.assign({},e.parameters),this}static fromJSON(e){return new Df(e.radius,e.tube,e.radialSegments,e.tubularSegments,e.arc)}}class Lf extends Yt{constructor(e=new gy(new L(-1,-1,0),new L(-1,1,0),new L(1,1,0)),n=64,i=1,r=8,o=!1){super(),this.type="TubeGeometry",this.parameters={path:e,tubularSegments:n,radius:i,radialSegments:r,closed:o};const s=e.computeFrenetFrames(n,o);this.tangents=s.tangents,this.normals=s.normals,this.binormals=s.binormals;const a=new L,l=new L,c=new Re;let d=new L;const f=[],h=[],p=[],v=[];_(),this.setIndex(v),this.setAttribute("position",new ct(f,3)),this.setAttribute("normal",new ct(h,3)),this.setAttribute("uv",new ct(p,2));function _(){for(let y=0;y<n;y++)m(y);m(o===!1?n:0),g(),u()}function m(y){d=e.getPointAt(y/n,d);const x=s.normals[y],C=s.binormals[y];for(let A=0;A<=r;A++){const T=A/r*Math.PI*2,b=Math.sin(T),w=-Math.cos(T);l.x=w*x.x+b*C.x,l.y=w*x.y+b*C.y,l.z=w*x.z+b*C.z,l.normalize(),h.push(l.x,l.y,l.z),a.x=d.x+i*l.x,a.y=d.y+i*l.y,a.z=d.z+i*l.z,f.push(a.x,a.y,a.z)}}function u(){for(let y=1;y<=n;y++)for(let x=1;x<=r;x++){const C=(r+1)*(y-1)+(x-1),A=(r+1)*y+(x-1),T=(r+1)*y+x,b=(r+1)*(y-1)+x;v.push(C,A,b),v.push(A,T,b)}}function g(){for(let y=0;y<=n;y++)for(let x=0;x<=r;x++)c.x=y/n,c.y=x/r,p.push(c.x,c.y)}}copy(e){return super.copy(e),this.parameters=Object.assign({},e.parameters),this}toJSON(){const e=super.toJSON();return e.path=this.parameters.path.toJSON(),e}static fromJSON(e){return new Lf(new VA[e.path.type]().fromJSON(e.path),e.tubularSegments,e.radius,e.radialSegments,e.closed)}}class ec extends ya{static get type(){return"MeshStandardMaterial"}constructor(e){super(),this.isMeshStandardMaterial=!0,this.defines={STANDARD:""},this.color=new we(16777215),this.roughness=1,this.metalness=0,this.map=null,this.lightMap=null,this.lightMapIntensity=1,this.aoMap=null,this.aoMapIntensity=1,this.emissive=new we(0),this.emissiveIntensity=1,this.emissiveMap=null,this.bumpMap=null,this.bumpScale=1,this.normalMap=null,this.normalMapType=$v,this.normalScale=new Re(1,1),this.displacementMap=null,this.displacementScale=1,this.displacementBias=0,this.roughnessMap=null,this.metalnessMap=null,this.alphaMap=null,this.envMap=null,this.envMapRotation=new $n,this.envMapIntensity=1,this.wireframe=!1,this.wireframeLinewidth=1,this.wireframeLinecap="round",this.wireframeLinejoin="round",this.flatShading=!1,this.fog=!0,this.setValues(e)}copy(e){return super.copy(e),this.defines={STANDARD:""},this.color.copy(e.color),this.roughness=e.roughness,this.metalness=e.metalness,this.map=e.map,this.lightMap=e.lightMap,this.lightMapIntensity=e.lightMapIntensity,this.aoMap=e.aoMap,this.aoMapIntensity=e.aoMapIntensity,this.emissive.copy(e.emissive),this.emissiveMap=e.emissiveMap,this.emissiveIntensity=e.emissiveIntensity,this.bumpMap=e.bumpMap,this.bumpScale=e.bumpScale,this.normalMap=e.normalMap,this.normalMapType=e.normalMapType,this.normalScale.copy(e.normalScale),this.displacementMap=e.displacementMap,this.displacementScale=e.displacementScale,this.displacementBias=e.displacementBias,this.roughnessMap=e.roughnessMap,this.metalnessMap=e.metalnessMap,this.alphaMap=e.alphaMap,this.envMap=e.envMap,this.envMapRotation.copy(e.envMapRotation),this.envMapIntensity=e.envMapIntensity,this.wireframe=e.wireframe,this.wireframeLinewidth=e.wireframeLinewidth,this.wireframeLinecap=e.wireframeLinecap,this.wireframeLinejoin=e.wireframeLinejoin,this.flatShading=e.flatShading,this.fog=e.fog,this}}class GA extends Xt{constructor(e,n=1){super(),this.isLight=!0,this.type="Light",this.color=new we(e),this.intensity=n}dispose(){}copy(e,n){return super.copy(e,n),this.color.copy(e.color),this.intensity=e.intensity,this}toJSON(e){const n=super.toJSON(e);return n.object.color=this.color.getHex(),n.object.intensity=this.intensity,this.groundColor!==void 0&&(n.object.groundColor=this.groundColor.getHex()),this.distance!==void 0&&(n.object.distance=this.distance),this.angle!==void 0&&(n.object.angle=this.angle),this.decay!==void 0&&(n.object.decay=this.decay),this.penumbra!==void 0&&(n.object.penumbra=this.penumbra),this.shadow!==void 0&&(n.object.shadow=this.shadow.toJSON()),this.target!==void 0&&(n.object.target=this.target.uuid),n}}const Iu=new rt,eg=new L,tg=new L;class WA{constructor(e){this.camera=e,this.intensity=1,this.bias=0,this.normalBias=0,this.radius=1,this.blurSamples=8,this.mapSize=new Re(512,512),this.map=null,this.mapPass=null,this.matrix=new rt,this.autoUpdate=!0,this.needsUpdate=!1,this._frustum=new Af,this._frameExtents=new Re(1,1),this._viewportCount=1,this._viewports=[new St(0,0,1,1)]}getViewportCount(){return this._viewportCount}getFrustum(){return this._frustum}updateMatrices(e){const n=this.camera,i=this.matrix;eg.setFromMatrixPosition(e.matrixWorld),n.position.copy(eg),tg.setFromMatrixPosition(e.target.matrixWorld),n.lookAt(tg),n.updateMatrixWorld(),Iu.multiplyMatrices(n.projectionMatrix,n.matrixWorldInverse),this._frustum.setFromProjectionMatrix(Iu),i.set(.5,0,0,.5,0,.5,0,.5,0,0,.5,.5,0,0,0,1),i.multiply(Iu)}getViewport(e){return this._viewports[e]}getFrameExtents(){return this._frameExtents}dispose(){this.map&&this.map.dispose(),this.mapPass&&this.mapPass.dispose()}copy(e){return this.camera=e.camera.clone(),this.intensity=e.intensity,this.bias=e.bias,this.radius=e.radius,this.mapSize.copy(e.mapSize),this}clone(){return new this.constructor().copy(this)}toJSON(){const e={};return this.intensity!==1&&(e.intensity=this.intensity),this.bias!==0&&(e.bias=this.bias),this.normalBias!==0&&(e.normalBias=this.normalBias),this.radius!==1&&(e.radius=this.radius),(this.mapSize.x!==512||this.mapSize.y!==512)&&(e.mapSize=this.mapSize.toArray()),e.camera=this.camera.toJSON(!1).object,delete e.camera.matrix,e}}class XA extends WA{constructor(){super(new _a(-5,5,5,-5,.5,500)),this.isDirectionalLightShadow=!0}}class jA extends GA{constructor(e,n){super(e,n),this.isDirectionalLight=!0,this.type="DirectionalLight",this.position.copy(Xt.DEFAULT_UP),this.updateMatrix(),this.target=new Xt,this.shadow=new XA}dispose(){this.shadow.dispose()}copy(e){return super.copy(e),this.target=e.target.clone(),this.shadow=e.shadow.clone(),this}}typeof __THREE_DEVTOOLS__<"u"&&__THREE_DEVTOOLS__.dispatchEvent(new CustomEvent("register",{detail:{revision:yf}}));typeof window<"u"&&(window.__THREE__?console.warn("WARNING: Multiple instances of Three.js being imported."):window.__THREE__=yf);function ll(t,e,n){let i=t*374761393+e*668265263+n*2147483647|0;return i=Math.imul(i^i>>>13,1274126177),i^=i>>>16,(i>>>0)/4294967296}const ng=t=>t*t*(3-2*t);function la(t,e,n=0){const i=Math.floor(t),r=Math.floor(e),o=ng(t-i),s=ng(e-r),a=ll(i,r,n),l=ll(i+1,r,n),c=ll(i,r+1,n),d=ll(i+1,r+1,n);return a+(l-a)*o+(c-a)*s+(a-l-c+d)*o*s}function Nf(t,e,n=0,i=5){let r=0,o=.5,s=1,a=0;for(let l=0;l<i;l++)r+=o*la(t*s,e*s,n+l*17),a+=o,o*=.5,s*=2.03;return r/a}function YA(t,e,n=0,i=5){let r=0,o=.5,s=1,a=0;for(let l=0;l<i;l++){const c=1-Math.abs(la(t*s,e*s,n+l*31)*2-1);r+=o*c*c,a+=o,o*=.5,s*=2.1}return r/a}function Ac(t){let e=t|0||1;return()=>(e=Math.imul(e^e>>>15,2246822507)+2654435769|0,e^=e>>>13,e=Math.imul(e,3266489909),e^=e>>>16,(e>>>0)/4294967296)}const ji=(t,e,n)=>{const i=Math.min(1,Math.max(0,(n-t)/(e-t)));return i*i*(3-2*i)},ft=(t,e,n)=>Math.min(n,Math.max(e,t)),Yi=(t,e,n)=>t+(e-t)*n,qA=["island","rocks","stack","sandbar"],Cl=-24;class vy{constructor(e){this.isle=e,this.mounds=[];const{size:n,height:i,kind:r,seed:o}=e,s=Ac(o*7919+13);if(r==="stack"){const a=1+Math.floor(s()*2.2);for(let l=0;l<a;l++){const c=s()*Math.PI*2,d=l?n*(1.1+s()*.9):0;this.mounds.push({x:Math.cos(c)*d,z:Math.sin(c)*d,r:n*(l?.55+s()*.35:1),h:i*(l?.45+s()*.4:1),steep:7})}this.shelf=n*2.6}else if(r==="rocks"){const a=4+Math.floor(s()*5);for(let l=0;l<a;l++){const c=s()*Math.PI*2,d=Math.sqrt(s())*n*.8;this.mounds.push({x:Math.cos(c)*d,z:Math.sin(c)*d,r:n*(.12+s()*.2),h:Math.max(.8,i*(.25+s()*.75)),steep:2.5})}this.shelf=n*1.5}else if(r==="sandbar")this.mounds.push({x:0,z:0,r:n,h:Math.min(i,2.2),steep:.6}),this.shelf=n*2.2;else{this.mounds.push({x:0,z:0,r:n*.8,h:i,steep:.85});const a=2+Math.floor(s()*3);for(let l=0;l<a;l++){const c=s()*Math.PI*2,d=n*(.25+s()*.4);this.mounds.push({x:Math.cos(c)*d,z:Math.sin(c)*d,r:n*(.3+s()*.3),h:i*(.35+s()*.45),steep:1.1})}this.shelf=n*1.9}this.reach=this.shelf*1.15}local(e,n){const{size:i,kind:r,seed:o}=this.isle,s=Math.hypot(e,n);if(s>this.reach)return Cl;const a=Math.atan2(n,e),l=1+.28*(Nf(Math.cos(a)*1.3+5,Math.sin(a)*1.3+5,o,3)-.5)*2,c=s/(this.shelf*l);let d=Cl+(Cl*-1-(r==="rocks"?1.2:r==="stack"?6:1.6))*(1-ji(.35,1,c));r==="sandbar"&&(d=Math.max(d,-.9-3*ji(.2,.7,c)));for(const f of this.mounds){const h=Math.hypot(e-f.x,n-f.z)/(f.r*l);if(h>=1.6)continue;let p;if(f.steep>=5){const _=.06*(la(Math.atan2(n-f.z,e-f.x)*3.2,.5,o+7)-.5);p=Math.pow(Math.max(0,1-Math.pow(h+_,3)),.42)}else p=Math.pow(Math.max(0,1-h*h),f.steep);let v=f.h*p;if(r!=="sandbar"&&p>0){const _=YA((e+300)/(i*.35),(n+300)/(i*.35),o,5);v+=f.h*(f.steep>=5?.12:.42)*(_-.45)*Math.min(1,p*3),f.steep>=5&&(v+=f.h*.08*(la(e*.3,n*.3,o)-.5)*ji(.6,1,h))}d=Math.max(d,v-(f.steep>=5?0:1.6*ji(.85,1.3,h)))}return d>-1.2&&d<1.8&&r!=="stack"&&(d=d<0?d:1.8*Math.pow(d/1.8,1.6)),d}at(e,n){return this.local(e-this.isle.x,n-this.isle.z)}}function $A(t,e,n){let i=Cl;for(const r of t)Math.abs(e-r.isle.x)>r.reach||Math.abs(n-r.isle.z)>r.reach||(i=Math.max(i,r.at(e,n)));return i}const ku=new we(.62,.53,.36),KA=new we(.28,.24,.17),ZA=new we(.028,.027,.025),JA=new we(.075,.07,.062),QA=new we(.03,.075,.016),eC=new we(.075,.12,.028),tC=new we(.42,.4,.3);function nC(t){const{isle:e}=t,n=t.reach*2,i=ft(n/200,.45,4),r=Math.min(320,Math.ceil(n/i)),o=new Hr(n,n,r,r);o.rotateX(-Math.PI/2);const s=o.attributes.position;for(let d=0;d<s.count;d++)s.setY(d,t.local(s.getX(d),s.getZ(d)));o.computeVertexNormals();const a=o.attributes.normal,l=new Float32Array(s.count*3),c=new we;for(let d=0;d<s.count;d++){const f=s.getX(d),h=s.getY(d),p=s.getZ(d),v=a.getY(d),_=la(f*.15+e.seed,p*.15,e.seed+3);if(h<-1.5?c.copy(tC).lerp(ku,.4):h<.25?c.copy(KA).lerp(ku,ji(-1.5,.25,h)*.6):h<2.2&&v>.8&&e.kind!=="stack"?c.copy(ku):c.copy(ZA).lerp(JA,_),(e.kind==="island"||e.kind==="stack"&&h>e.height*.85)&&h>2&&v>.7){const u=ji(.7,.86,v)*ji(2.5,5,h);c.lerp(QA.clone().lerp(eC,Nf(f*.05,p*.05,e.seed+9,3)),u)}c.multiplyScalar(.85+.3*_),l.set([c.r,c.g,c.b],d*3)}return o.setAttribute("color",new en(l,3)),o.translate(e.x,0,e.z),o.computeBoundingSphere(),o}function iC(){const t=[],e=[];for(let r=0;r<=8;r++){const o=r/8;e.push(new L(Math.pow(o,1.8)*1.6,o*7.5,0))}const n=new Lf(new my(e),10,.17,6);ig(n,new we(.11,.075,.045)),t.push(n);const i=e[e.length-1];for(let r=0;r<9;r++){const o=r/9*Math.PI*2+r%2*.2,s=new Yt,a=[],l=8,c=3.6+r%3*.4;for(let f=0;f<=l;f++){const h=f/l,p=Math.sin(h*Math.PI)*.65+.05,v=h*c,_=.6*h-2.1*h*h;a.push(v,_,-p,v,_+.12,0,v,_,p)}const d=[];for(let f=0;f<l;f++){const h=f*3;d.push(h,h+3,h+1,h+1,h+3,h+4,h+1,h+4,h+2,h+2,h+4,h+5)}s.setAttribute("position",new ct(a,3)),s.setIndex(d),s.rotateY(o),s.translate(i.x,i.y,i.z),s.computeVertexNormals(),ig(s,new we(.035,.09,.018).lerp(new we(.085,.14,.03),r%3/3)),t.push(s)}return yy(t)}function rC(t){const{isle:e}=t,n=[];if(e.kind!=="island"||e.palms<=0)return n;const i=Ac(e.seed*31+5),r=Math.min(160,Math.round(ft(e.palms,0,1)*e.size*e.size/140));for(let o=0;n.length<r&&o<r*25;o++){const s=i()*Math.PI*2,a=Math.sqrt(i())*t.reach,l=e.x+Math.cos(s)*a,c=e.z+Math.sin(s)*a,d=t.at(l,c);d<1.1||d>e.height*.7||Math.abs(t.at(l+1,c)-d)+Math.abs(t.at(l,c+1)-d)>.95||Nf(l*.02,c*.02,e.seed+4,3)<.42&&i()<.6||n.push({x:l,y:d-.2,z:c,s:.75+i()*.55,a:i()*Math.PI*2})}return n}function oC(t,e=1.5){const n=t.reach*2,i=ft(Math.ceil(n/e),8,256),r=new Uint8Array(i*i);for(let o=0;o<i;o++)for(let s=0;s<i;s++){const a=t.isle.x-t.reach+(s+.5)*(n/i),l=t.isle.z-t.reach+(o+.5)*(n/i),c=t.at(a,l);r[o*i+s]=c>0?2:c>-3?1:0}return{n:i,ext:n,cells:r}}function ig(t,e){const n=t.attributes.position.count,i=new Float32Array(n*3);for(let r=0;r<n;r++)i.set([e.r,e.g,e.b],r*3);t.setAttribute("color",new en(i,3))}function yy(t){const e=[],n=[],i=[],r=[];let o=0;for(let a of t){a.index||(a=a.toNonIndexed()),a.attributes.normal||a.computeVertexNormals();const l=a.attributes.position,c=a.attributes.normal,d=a.attributes.color;for(let f=0;f<l.count;f++)e.push(l.getX(f),l.getY(f),l.getZ(f)),n.push(c.getX(f),c.getY(f),c.getZ(f)),i.push(d?d.getX(f):1,d?d.getY(f):1,d?d.getZ(f):1);if(a.index)for(let f=0;f<a.index.count;f++)r.push(a.index.getX(f)+o);else for(let f=0;f<l.count;f++)r.push(f+o);o+=l.count}const s=new Yt;return s.setAttribute("position",new ct(e,3)),s.setAttribute("normal",new ct(n,3)),s.setAttribute("color",new ct(i,3)),s.setIndex(r),s}function sC(t,e){const n=(a,l)=>typeof a=="number"&&Number.isFinite(a)?a:l,i=qA.includes(String(e.kind))?e.kind:"island",r=ft(n(e.size,i==="stack"?14:i==="rocks"?60:120),4,1500),o=ft(n(e.height,i==="stack"?28:i==="rocks"?3:i==="sandbar"?1.2:30),.5,400);let s=n(e.seed,NaN);if(!Number.isFinite(s)){s=0;for(const a of t)s=s*31+a.charCodeAt(0)|0}return{id:t,name:String(e.name||""),x:n(e.x,0),z:n(e.z,0),size:r,height:o,kind:i,palms:ft(n(e.palms,i==="island"?.5:0),0,1),seed:Math.abs(s)%1e5,note:String(e.note||"")}}const If=new Nv("world"),_y=new Nv("islands"),Vi=new ES([If,_y],{quiet:!0}),Gs=(t,e)=>typeof t=="number"&&Number.isFinite(t)?t:e,aC=["clear","cloudy","overcast"],lC=["calm","moderate","rough"];function Cc(){var t;return((t=If.get("main"))==null?void 0:t.data)||{}}function rg(t=Cc()){return{time:Math.min(19.5,Math.max(5,Gs(t.time,15))),weather:aC.includes(t.weather)?t.weather:"cloudy",sea:lC.includes(t.sea)?t.sea:"moderate",wind:(Gs(t.wind,225)%360+360)%360}}function og(t=Cc()){return{x:Gs(t.start_x,0),z:Gs(t.start_z,0),heading:Gs(t.start_heading,0)*Math.PI/180}}function cC(){return _y.rows.map(t=>sC(t.id,t.data))}function Uu(t){return Vi.track(If.put({...Cc(),...t},"main"))}const xy=()=>`myco-ocean:${Vi.page||"page"}`;function Sy(){try{return JSON.parse(localStorage.getItem(xy())||"{}")}catch{return{}}}function Fu(t){try{localStorage.setItem(xy(),JSON.stringify({...Sy(),...t}))}catch{}}const xs=(t,e,n)=>t.clone().lerp(e,ft(n,0,1)),vr=(t,e,n)=>new we(t,e,n);function uC(t,e){const i=(ft(t,5,19.5)-6)/12,r=Math.max(.03,Math.sin(ft(i,0,1)*Math.PI)*(62*Math.PI)/180+.02),o=(90+i*180)*(Math.PI/180),s=new L(Math.sin(o)*Math.cos(r),Math.sin(r),-Math.cos(o)*Math.cos(r)).normalize(),a=e==="clear"?.18:e==="cloudy"?.5:.92,l=1-ji(.02,.45,s.y),c=vr(1,.52,.26),d=vr(1,.95,.88),f=xs(d,c,l).multiplyScalar(Yi(3.2,.9,l)*(1-.78*a*a));let h=xs(vr(.16,.36,.72),vr(.1,.16,.34),l*.9),p=xs(vr(.66,.78,.88),vr(.98,.66,.44),l*.85);const v=vr(.62,.66,.7).multiplyScalar(Yi(1,.55,l));return h=xs(h,v.clone().multiplyScalar(.92),a*a*1.05),p=xs(p,v,a*a*.9),{sun:s,sunColor:f,zenith:h,horizon:p,cover:a,haze:Yi(42e-5,72e-5,a),ambient:Yi(1,.6,l)}}function dC(){return{uSun:{value:new L(0,1,0)},uSunColor:{value:new we(1,1,1)},uZenith:{value:new we},uHorizon:{value:new we},uCover:{value:.4},uHaze:{value:2e-4},uTime:{value:0},uWind:{value:new Re(1,0)}}}function hC(t,e,n){t.uSun.value.copy(e.sun),t.uSunColor.value.copy(e.sunColor),t.uZenith.value.copy(e.zenith),t.uHorizon.value.copy(e.horizon),t.uCover.value=e.cover,t.uHaze.value=e.haze;const i=(n+180)*Math.PI/180;t.uWind.value.set(Math.sin(i),-Math.cos(i))}const My=`
uniform vec3 uSun;
uniform vec3 uSunColor;
uniform vec3 uZenith;
uniform vec3 uHorizon;
uniform float uCover;
uniform float uHaze;
uniform float uTime;
uniform vec2 uWind;

float hash12(vec2 p) { vec3 p3 = fract(vec3(p.xyx) * 0.1031); p3 += dot(p3, p3.yzx + 33.33); return fract((p3.x + p3.y) * p3.z); }
float vnoise(vec2 p) {
  vec2 i = floor(p), f = fract(p);
  vec2 u = f * f * (3.0 - 2.0 * f);
  return mix(mix(hash12(i), hash12(i + vec2(1, 0)), u.x), mix(hash12(i + vec2(0, 1)), hash12(i + vec2(1, 1)), u.x), u.y);
}
float cloudFbm(vec2 p, int octaves) {
  float s = 0.0, a = 0.5;
  mat2 m = mat2(1.6, 1.2, -1.2, 1.6);
  for (int i = 0; i < 6; i++) { if (i >= octaves) break; s += a * vnoise(p); p = m * p; a *= 0.5; }
  return s;
}

// The clear air: blue above, pale at the horizon, a glow around the sun.
vec3 airColor(vec3 d) {
  float y = max(d.y, 0.0);
  vec3 c = mix(uHorizon, uZenith, pow(y, 0.45));
  float mu = max(dot(d, uSun), 0.0);
  float glow = pow(mu, 8.0) * 0.35 + pow(mu, 64.0) * 0.6;
  c += uSunColor * glow * (0.25 + 0.75 * (1.0 - uCover)) * 0.35;
  return c;
}

// Cloud cover in a direction (0–1) and its light.
vec4 clouds(vec3 d, int octaves) {
  if (d.y < 0.01) return vec4(0.0);
  vec2 p = d.xz / (d.y + 0.06) * 1.4;
  p += uWind * uTime * 0.004;
  float n = cloudFbm(p * 0.9, octaves);
  float thr = 1.0 - uCover;
  float c = smoothstep(thr * 0.62 + 0.12, thr * 0.62 + 0.42, n);
  // lit from the sun's side: a second look a little toward it
  float n2 = cloudFbm(p * 0.9 + uSun.xz * 0.08, octaves);
  float lit = clamp(0.55 + (n - n2) * 3.5, 0.0, 1.0);
  float mu = max(dot(d, uSun), 0.0);
  vec3 dark = mix(uHorizon, vec3(dot(uHorizon, vec3(0.333))), 0.6) * 0.72;
  vec3 bright = uSunColor * 0.34 + uZenith * 0.25 + vec3(0.42);
  vec3 col = mix(dark, bright, lit) + uSunColor * pow(mu, 12.0) * 0.25 * (1.0 - c);
  float fadeH = smoothstep(0.0, 0.18, d.y);
  return vec4(col, c * fadeH);
}

// The sky as rough water sees it: the clouds blurred into their average.
vec3 skyMean(vec3 d) {
  vec3 c = airColor(d);
  vec3 dark = mix(uHorizon, vec3(dot(uHorizon, vec3(0.333))), 0.6) * 0.72;
  vec3 bright = uSunColor * 0.34 + uZenith * 0.25 + vec3(0.42);
  c = mix(c, mix(dark, bright, 0.55), uCover * 0.85 * smoothstep(0.0, 0.18, d.y));
  float h = 1.0 - smoothstep(-0.02, 0.12, d.y);
  return mix(c, uHorizon, h * 0.85);
}

vec3 skyColor(vec3 d, int octaves, bool withSun) {
  vec3 c = airColor(d);
  vec4 cl = clouds(d, octaves);
  c = mix(c, cl.rgb, cl.a * 0.92);
  if (withSun) {
    float mu = dot(d, uSun);
    float disk = smoothstep(0.99955, 0.99975, mu);
    c += uSunColor * disk * 6.0 * (1.0 - cl.a) * (1.0 - uCover * 0.8);
  }
  // the low haze over the sea
  float h = 1.0 - smoothstep(-0.02, 0.12, d.y);
  return mix(c, uHorizon, h * 0.85);
}
`;function sg(t){const e=new xn({uniforms:t,vertexShader:`
      varying vec3 vDir;
      void main() {
        vDir = position;
        vec4 p = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
        gl_Position = p.xyww;
      }`,fragmentShader:My+`
      varying vec3 vDir;
      void main() {
        vec3 d = normalize(vDir);
        vec3 c = skyColor(d, 5, true);
        gl_FragColor = vec4(c, 1.0);
        #include <tonemapping_fragment>
        #include <colorspace_fragment>
      }`,side:Jt,depthWrite:!1}),n=new Ct(new Tc(1e3,48,24),e);return n.frustumCulled=!1,n.renderOrder=-10,n}const ca={calm:{longest:34,height:.45,chop:.45,foam:.12},moderate:{longest:58,height:1,chop:.7,foam:.5},rough:{longest:90,height:1.9,chop:.82,foam:.95}},Mo=22,fC=9.81;function pC(t,e,n=Mo,i=1.2,r,o=7){const s=ca[t]||ca.moderate,a=Ac(o),l=s.longest,c=Math.pow(i/l,1/Math.max(1,n-1)),d=(e+180)*Math.PI/180,f=[];for(let p=0;p<n;p++){const v=l*Math.pow(c,p),_=.35+.9*(p/n),m=d+(a()*2-1)*_,u=2*Math.PI/v;f.push({dx:Math.sin(m),dz:-Math.cos(m),k:u,w:Math.sqrt(fC*u),a:.0052*v*s.height*(.7+.6*a()),q:0,p:a()*Math.PI*2})}const h=f.reduce((p,v)=>p+v.k*v.a,0);for(const p of f)p.q=s.chop/Math.max(h,1e-6);return f}function ag(t,e,n,i){let r=e,o=0,s=n;for(const a of t){const l=a.k*(a.dx*e+a.dz*n)-a.w*i+a.p,c=Math.cos(l),d=Math.sin(l);r+=a.q*a.a*a.dx*c,s+=a.q*a.a*a.dz*c,o+=a.a*d}return[r,o,s]}function mC(t,e,n,i){let r=e,o=n;for(let s=0;s<3;s++){const[a,,l]=ag(t,r,o,i);r+=e-a,o+=n-l}return ag(t,r,o,i)[1]}function gC(t,e){const n=new Float32Array(e*4),i=new Float32Array(e*4);return t.slice(0,e).forEach((r,o)=>{n.set([r.dx,r.dz,r.k,r.w],o*4),i.set([r.a,r.q,r.p,0],o*4)}),{A:n,B:i}}const ti=7.4,wy=2.6,kf=-.62;function Gi(t){return wy/2*(t<.58?.9+.1*Math.sin(t/.58*(Math.PI/2)):Math.pow(Math.max(0,Math.cos((t-.58)/.42*(Math.PI/2))),.75))}const lg=t=>kf+.28*Math.pow(Math.max(0,(t-.62)/.38),2),gi=t=>1.02+.6*t*t,Rr=t=>gi(t)-.36;function Ey(t,e){const n=t.attributes.position,i=new Float32Array(n.count*3);for(let r=0;r<n.count;r++){const o=e(n.getX(r),n.getY(r),n.getZ(r));i.set([o.r,o.g,o.b],r*3)}return t.setAttribute("color",new en(i,3)),t}const tc=(t,e)=>Ey(t,()=>e),vC=new we(.62,.64,.66),yC=new we(.025,.027,.03),_C=new we(.42,.035,.025),xh=new we(.42,.04,.025),xC=new we(.72,.72,.7),SC=new we(.17,.14,.11),MC=new we(.01,.01,.012),wC=new we(.015,.07,.42),cg=new we(.88,.2,.02),Ss=new we(.28,.29,.3);function EC(){const n=[],i=[],r=(f,h,p)=>{const v=[],_=(f-.5)*ti,m=Gi(f);for(let u=0;u<=10;u++){const g=u/10;let y,x;if(p)y=m*.9,x=Yi(gi(f),Rr(f),g);else{const C=Yi(2.6,1.15,Math.pow(f,2));y=m*(1-Math.pow(1-g,C)),x=Yi(lg(f),gi(f),g)}v.push(n.length/3),n.push(h*y,x,_)}return v},o=(f,h)=>{for(let p=0;p<f.length-1;p++)for(let v=0;v<f[p].length-1;v++){const _=f[p][v],m=f[p+1][v],u=f[p][v+1],g=f[p+1][v+1];h?i.push(_,u,m,m,u,g):i.push(_,m,u,m,g,u)}};for(const f of[1,-1]){const h=[],p=[];for(let _=0;_<=28;_++){const m=_/28;h.push(r(m,f,!1)),p.push(r(m,f,!0))}o(h,f<0),o(p,f>0);const v=[];for(let _=0;_<=28;_++){const m=_/28,u=(m-.5)*ti,g=Gi(m),y=n.length/3;n.push(f*g,gi(m)+.04,u);const x=n.length/3;n.push(f*g*.9,gi(m)+.04,u),v.push([y,x])}o(v,f>0)}const s=[];for(let f=0;f<=28;f++){const h=f/28,p=(h-.5)*ti,v=Gi(h)*.9,_=n.length/3;n.push(-v,Rr(h),p);const m=n.length/3;n.push(v,Rr(h),p),s.push([_,m])}o(s,!0);const a=-.5*ti,l=Gi(0),c=[];for(let f=0;f<=10;f++){const h=f/10,p=l*(1-Math.pow(1-h,2.6)),v=Yi(lg(0),gi(0),h),_=n.length/3;n.push(-p,v,a);const m=n.length/3;n.push(p,v,a),c.push([_,m])}o(c,!1);const d=new Yt;return d.setAttribute("position",new ct(n,3)),d.setIndex(i),d.computeVertexNormals(),Ey(d,(f,h,p)=>{const v=p/ti+.5;return Math.abs(f)<Gi(v)*.905&&h<=Rr(v)+.01?SC:h>gi(v)-.02?xh:h<-.05?yC:h<.08?_C:vC})}function Ms(t,e,n,i,r,o,s){const a=new ts(t,e,n);return a.translate(i,r,o),tc(a,s)}function oo(t,e,n,i,r,o,s=10,a=0,l=0){const c=new Pf(t,t,e,s);return a&&c.rotateX(a),l&&c.rotateZ(l),c.translate(n,i,r),tc(c,o)}function TC(){const t=new br,e=new ec({vertexColors:!0,roughness:.55,metalness:.05,side:bn}),n=new ec({color:724756,roughness:.08,metalness:.2}),i=-.25,r=Rr(.47),o=1.75,s=1.55,a=2.1,l=[EC(),Ms(o,s,a,0,r+s/2,i,xC),Ms(o+.22,.12,a+.35,0,r+s+.06,i+.05,xh),Ms(o+.06,.1,a+.06,0,r+.05,i,xh),oo(.05,2.4,0,r+s+1.25,i-.3,Ss),oo(.03,1.4,0,r+s+2.05,i-.3,Ss,6,0,Math.PI/2),oo(.015,1.6,.55,r+s+.9,i-.85,Ss,5),oo(.015,1.2,-.55,r+s+.7,i-.85,Ss,5),Ms(.5,.12,.18,0,r+s+.3,i+.3,MC),oo(.28,.85,-.62,Rr(.12)+.43,-2.6,wC,14),Ms(.9,.42,.6,.45,Rr(.1)+.21,-2.75,new we(.25,.42,.55)),tc(new Tc(.22,12,8).translate(-Gi(.2)-.05,gi(.2)-.25,-1.9),cg),tc(new Df(.32,.07,6,16).translate(0,r+.95,i-a/2-.08),cg),...[.7,.78,.86].flatMap(p=>[1,-1].map(v=>oo(.025,.45,v*Gi(p)*.92,gi(p)+.22,(p-.5)*ti,Ss,5)))],c=new Ct(yy(l),e);t.add(c);const d=new br,f=(p,v,_,m,u,g)=>{const y=new Ct(new Hr(p,v),n);y.position.set(_,m,u),y.rotation.y=g,d.add(y)},h=r+s*.68;for(const p of[-.56,0,.56])f(.48,.55,p,h,i+a/2+.006,0);for(const p of[1,-1])for(const v of[i+.45,i-.4])f(.68,.5,p*(o/2+.006),h,v,p*Math.PI/2);return f(.9,.45,0,h,i-a/2-.006,Math.PI),t.add(d),t.traverse(p=>{p.castShadow=!0}),t}const Sh=13;function ug(t=0,e=0,n=0){return{x:t,z:e,heading:n,speed:0,throttle:0,rudder:0,y:0,vy:0,pitch:0,roll:0,vp:0,vr:0,aground:1/0}}function AC(t,e,n,i,r){n=Math.min(n,.05),e.stop?t.throttle=0:e.throttle&&(t.throttle=ft(t.throttle+e.throttle*n*.55,-.4,1)),t.rudder+=((e.steer||0)-t.rudder)*Math.min(1,n*(e.steer?3.2:4.5));const o=t.throttle*(t.throttle>0?3.4:2.2),s=.06*t.speed+.0145*t.speed*Math.abs(t.speed);t.speed+=(o-s)*n,Math.abs(t.speed)<.02&&!t.throttle&&(t.speed=0);const a=ft(Math.abs(t.speed)/3,0,1)*Math.sign(t.speed||1)+(t.throttle>0?.25*t.throttle:0);t.heading+=t.rudder*.42*a*n*(1-.35*ft(t.speed/Sh,0,1)),t.heading=(t.heading%(Math.PI*2)+Math.PI*2)%(Math.PI*2);const l=Math.sin(t.heading),c=-Math.cos(t.heading);let d=t.x+l*t.speed*n,f=t.z+c*t.speed*n;const h=(t.speed>=0?1:-1)*(ti/2-.3);Math.max(r(d+l*h,f+c*h),r(d,f))>kf-.15&&(d=t.x-l*t.speed*n*.5,f=t.z-c*t.speed*n*.5,Math.abs(t.speed)>1.2&&(t.aground=0),t.speed*=-.25),t.x=d,t.z=f,t.aground+=n;const v=Math.cos(t.heading),_=Math.sin(t.heading),m=i(t.x+l*2.6,t.z+c*2.6),u=i(t.x-l*2.6,t.z-c*2.6),g=i(t.x+v*1.1,t.z+_*1.1),y=i(t.x-v*1.1,t.z-_*1.1),x=ft(t.speed/Sh,-.4,1),C=(m+u+g+y)/4+.12*x,A=Math.atan2(m-u,5.2)+.06*x*(1-x*.5),T=Math.atan2(y-g,2.2)+t.rudder*.07*ft(Math.abs(t.speed)/6,0,1);t.vy+=((C-t.y)*14-t.vy*4.2)*n,t.y+=t.vy*n,t.vp+=((A-t.pitch)*10-t.vp*3.4)*n,t.pitch+=t.vp*n,t.vr+=((T-t.roll)*7-t.vr*2.2)*n,t.roll+=t.vr*n,t.aground<.35&&(t.roll+=Math.sin(t.aground*60)*.01)}function CC(t,e){t.position.set(e.x,e.y,e.z),t.rotation.set(-e.pitch,Math.PI-e.heading,e.roll,"YXZ")}const bC=`
float hullHalfWidth(float s) {
  float hw = ${(wy/2).toFixed(3)};
  if (s < 0.58) return hw * (0.9 + 0.1 * sin(s / 0.58 * 1.5708));
  return hw * pow(max(cos((s - 0.58) / 0.42 * 1.5708), 0.0), 0.75);
}
// the hull's width at a height (as hull() lofts it): water inside it is not drawn
bool insideHull(vec3 local) {
  float s = local.z / ${ti.toFixed(3)} + 0.5;
  if (s < 0.0 || s > 1.0) return false;
  float keel = ${kf.toFixed(3)} + 0.28 * pow(max(0.0, (s - 0.62) / 0.38), 2.0);
  float gun = 1.02 + 0.6 * s * s;
  float t = clamp((local.y - keel) / (gun - keel), 0.0, 1.0);
  if (local.y < keel) return false;
  float p = mix(2.6, 1.15, s * s);
  return abs(local.x) < hullHalfWidth(s) * (1.0 - pow(1.0 - t, p)) * 0.985;
}
`,nc=256,wo=256,ic={calm:{wind:5,hs:.45,chop:.75,foam:.855},moderate:{wind:9,hs:1.3,chop:1.15,foam:.8},rough:{wind:14,hs:2.8,chop:1.3,foam:.835}},dg=9.81;function RC(t,e,n=nc,i=wo,r=4242){const o=ic[t]||ic.moderate,s=(e+180)*Math.PI/180,a=Math.sin(s),l=-Math.cos(s),c=o.wind*o.wind/dg,d=c*.0015,f=Ac(r),h=()=>{const C=Math.max(1e-9,f()),A=f();return Math.sqrt(-2*Math.log(C))*Math.cos(2*Math.PI*A)},p=new Float32Array(n*n*2),v=(C,A)=>{const T=Math.hypot(C,A);if(T<1e-6)return 0;const b=(C*a+A*l)/T;let w=Math.exp(-1/(T*c)**2)/T**4*b*b*Math.exp(-(T*T)*d*d);return b<0&&(w*=.07),w},_=C=>2*Math.PI*(C<n/2?C:C-n)/i;for(let C=0;C<n;C++)for(let A=0;A<n;A++){const T=Math.sqrt(v(_(A),_(C))/2);p[(C*n+A)*2]=h()*T,p[(C*n+A)*2+1]=h()*T}let m=0;for(let C=0;C<n*n;C++)m+=2*(p[C*2]**2+p[C*2+1]**2);const u=o.hs/4/Math.sqrt(Math.max(m,1e-12));for(let C=0;C<p.length;C++)p[C]*=u;const g=new Float32Array(n*n*4);for(let C=0;C<n;C++)for(let A=0;A<n;A++){const T=C*n+A,b=(n-A)%n,w=(n-C)%n,S=w*n+b;g.set([p[T*2],p[T*2+1],p[S*2],-p[S*2+1]],T*4)}const y=[],x=2*Math.PI/10;for(let C=0;C<n;C++)for(let A=0;A<n;A++){const T=_(A),b=_(C),w=Math.hypot(T,b);if(w<1e-6||w>x)continue;const S=g.subarray((C*n+A)*4,(C*n+A)*4+4);y.push({kx:T,kz:b,k:w,w:Math.sqrt(dg*w),a:S[0],b:S[1],c:S[2],d:S[3]})}return{tex:g,modes:y,chop:o.chop,foam:o.foam}}class hg{constructor(e,n){this.modes=e,this.chop=n,this.hr=new Float32Array(0),this.hi=new Float32Array(0),this.at=NaN}prepare(e){if(e===this.at)return;const n=this.modes;this.hr.length!==n.length&&(this.hr=new Float32Array(n.length),this.hi=new Float32Array(n.length));for(let i=0;i<n.length;i++){const r=Math.cos(n[i].w*e),o=Math.sin(n[i].w*e);this.hr[i]=n[i].a*r-n[i].b*o+n[i].c*r+n[i].d*o,this.hi[i]=n[i].a*o+n[i].b*r-n[i].c*o+n[i].d*r}this.at=e}displace(e,n,i){this.prepare(i);const r=this.modes;let o=0,s=0,a=0;for(let l=0;l<r.length;l++){const c=r[l].kx*e+r[l].kz*n,d=Math.cos(c),f=Math.sin(c),h=this.hr[l]*d-this.hi[l]*f,p=this.hr[l]*f+this.hi[l]*d;o+=h,s+=r[l].kx/r[l].k*p,a+=r[l].kz/r[l].k*p}return[o,this.chop*s,this.chop*a]}heightAt(e,n,i){let r=e,o=n;for(let s=0;s<2;s++){const[,a,l]=this.displace(r,o,i);r=e-a,o=n-l}return this.displace(r,o,i)[0]}}const PC="void main(){ gl_Position = vec4(position.xy, 0.0, 1.0); }",fg=`vec2 cmul(vec2 a, vec2 b){ return vec2(a.x * b.x - a.y * b.y, a.x * b.y + a.y * b.x); }
`;class pg{constructor(e,n=nc){this.r=e,this.n=n,this.foamAt=0,this.scene=new Bs,this.cam=new _a(-1,1,1,-1,0,1),this.long=new hg([],1),this.chop=1,this.foamRate=.815;const i=(s=dn)=>new qn(n,n,{type:Pn,format:Wt,minFilter:s,magFilter:s,depthBuffer:!1,generateMipmaps:!1}),r=s=>{const a=new qn(n,n,{type:Zo,format:Wt,minFilter:s?ni:Kt,magFilter:Kt,depthBuffer:!1,generateMipmaps:s});return a.texture.wrapS=a.texture.wrapT=Wo,a};this.work=[i(),i(),i(),i()],this.disp=r(!1),this.slope=r(!0),this.foam=[r(!0),r(!0)],this.h0=new Ec(new Float32Array(n*n*4),n,n,Wt,Pn),this.h0.needsUpdate=!0;const o=(s,a)=>new xn({vertexShader:PC,fragmentShader:s,uniforms:a,depthTest:!1,depthWrite:!1});this.mats={spec:o(fg+`
        uniform sampler2D uH0; uniform float uT, uN, uL; uniform int uPart;
        void main() {
          vec2 n = floor(gl_FragCoord.xy);
          vec2 m = vec2(n.x < uN * 0.5 ? n.x : n.x - uN, n.y < uN * 0.5 ? n.y : n.y - uN);
          vec2 k = 6.2831853 * m / uL;
          float kl = length(k);
          if (kl < 1e-6) { gl_FragColor = vec4(0.0); return; }
          float w = sqrt(9.81 * kl);
          vec4 h0 = texture2D(uH0, (n + 0.5) / uN);
          float c = cos(w * uT), s = sin(w * uT);
          vec2 h = cmul(h0.xy, vec2(c, s)) + cmul(h0.zw, vec2(c, -s));
          vec2 ih = vec2(-h.y, h.x);
          vec2 A, B, C, D;
          if (uPart == 0) { A = -(k.x / kl) * ih; B = h; C = -(k.y / kl) * ih; D = k.x * ih; }
          else { A = k.y * ih; B = (k.x * k.x / kl) * h; C = (k.y * k.y / kl) * h; D = (k.x * k.y / kl) * h; }
          // two real fields in one complex number: A + iB
          gl_FragColor = vec4(A.x - B.y, A.y + B.x, C.x - D.y, C.y + D.x);
        }`,{uH0:{value:this.h0},uT:{value:0},uN:{value:n},uL:{value:wo},uPart:{value:0}}),fft:o(fg+`
        uniform sampler2D uIn; uniform float uN, uSize; uniform int uHoriz;
        void main() {
          vec2 fc = floor(gl_FragCoord.xy);
          float index = uHoriz == 1 ? fc.x : fc.y;
          float half_ = uSize * 0.5;
          float even = floor(index / uSize) * half_ + mod(index, half_);
          vec2 ce = uHoriz == 1 ? vec2(even, fc.y) : vec2(fc.x, even);
          vec2 co = uHoriz == 1 ? vec2(even + uN * 0.5, fc.y) : vec2(fc.x, even + uN * 0.5);
          vec4 e = texture2D(uIn, (ce + 0.5) / uN), o = texture2D(uIn, (co + 0.5) / uN);
          float a = 6.2831853 * index / uSize;
          vec2 tw = vec2(cos(a), sin(a));
          gl_FragColor = vec4(e.xy + cmul(tw, o.xy), e.zw + cmul(tw, o.zw));
        }`,{uIn:{value:null},uN:{value:n},uSize:{value:2},uHoriz:{value:1}}),out:o(`
        uniform sampler2D uA, uB; uniform float uN, uChop; uniform int uWhich;
        void main() {
          vec2 uv = (floor(gl_FragCoord.xy) + 0.5) / uN;
          vec4 a = texture2D(uA, uv), b = texture2D(uB, uv);
          if (uWhich == 0) { gl_FragColor = vec4(uChop * a.x, a.y, uChop * a.z, 0.0); return; }
          float jxx = 1.0 + uChop * b.y, jzz = 1.0 + uChop * b.z, jxz = uChop * b.w;
          float J = jxx * jzz - jxz * jxz;
          // slopes of the displaced surface, and how squeezed it is
          gl_FragColor = vec4(a.w / max(jxx, 0.1), b.x / max(jzz, 0.1), J, 0.0);
        }`,{uA:{value:null},uB:{value:null},uN:{value:n},uChop:{value:1},uWhich:{value:0}}),foam:o(`
        uniform sampler2D uPrev, uSlope; uniform float uN, uDt, uRate;
        void main() {
          vec2 uv = (floor(gl_FragCoord.xy) + 0.5) / uN;
          float J = texture2D(uSlope, uv).z;
          // (it lasts a few seconds: crests moving on leave streaks behind)
          float kept = texture2D(uPrev, uv).r * exp(-uDt * 0.32);
          float made = smoothstep(uRate, uRate - 0.1, J);
          gl_FragColor = vec4(clamp(max(kept, made), 0.0, 1.0), 0.0, 0.0, 1.0);
        }`,{uPrev:{value:null},uSlope:{value:null},uN:{value:n},uDt:{value:0},uRate:{value:.815}})},this.quad=new Ct(new Hr(2,2),this.mats.spec),this.quad.frustumCulled=!1,this.scene.add(this.quad)}static supported(e){return e.capabilities.isWebGL2&&e.extensions.has("EXT_color_buffer_float")}get foamTexture(){return this.foam[this.foamAt].texture}setSea(e,n){const i=RC(e,n,this.n);this.h0.image.data.set(i.tex),this.h0.needsUpdate=!0,this.long=new hg(i.modes,i.chop),this.chop=i.chop,this.foamRate=i.foam}pass(e,n){this.quad.material=e,this.r.setRenderTarget(n),this.r.render(this.scene,this.cam)}update(e,n){const i=this.r.getRenderTarget(),r=this.r.autoClear;this.r.autoClear=!1;const o=this.mats,[s,a,l,c]=this.work;o.spec.uniforms.uT.value=e,o.spec.uniforms.uPart.value=0,this.pass(o.spec,s),o.spec.uniforms.uPart.value=1,this.pass(o.spec,l);const d=Math.log2(this.n),f=(_,m)=>{let u=_,g=m;for(const y of[1,0])for(let x=0;x<d;x++)o.fft.uniforms.uIn.value=u.texture,o.fft.uniforms.uSize.value=2**(x+1),o.fft.uniforms.uHoriz.value=y,this.pass(o.fft,g),[u,g]=[g,u];return u},h=f(s,a),p=f(l,c);o.out.uniforms.uA.value=h.texture,o.out.uniforms.uB.value=p.texture,o.out.uniforms.uChop.value=this.chop,o.out.uniforms.uWhich.value=0,this.pass(o.out,this.disp),o.out.uniforms.uWhich.value=1,this.pass(o.out,this.slope);const v=1-this.foamAt;o.foam.uniforms.uPrev.value=this.foam[this.foamAt].texture,o.foam.uniforms.uSlope.value=this.slope.texture,o.foam.uniforms.uDt.value=Math.min(n,.25),o.foam.uniforms.uRate.value=this.foamRate,this.pass(o.foam,this.foam[v]),this.foamAt=v,this.r.setRenderTarget(i),this.r.autoClear=r}dispose(){for(const e of[...this.work,this.disp,this.slope,...this.foam])e.dispose();this.h0.dispose();for(const e of Object.values(this.mats))e.dispose();this.quad.geometry.dispose()}}const Ty=-12,Ay=14,mg=.55;function DC(t=.4,e=7e3,n=220,i=1.034){const r=[0,0,0],o=[t],s=[],a=[];for(let c=t;c<e;c*=i)a.push(c);a.push(e*1.6),a.forEach((c,d)=>{for(let h=0;h<n;h++){const p=h/n*Math.PI*2;r.push(Math.cos(p)*c,0,Math.sin(p)*c),o.push(Math.max(c*(i-1),2*Math.PI*c/n))}const f=1+d*n;if(d===0)for(let h=0;h<n;h++)s.push(0,f+(h+1)%n,f+h);else{const h=f-n;for(let p=0;p<n;p++){const v=h+p,_=h+(p+1)%n,m=f+p,u=f+(p+1)%n;s.push(v,_,m,_,u,m)}}});const l=new Yt;return l.setAttribute("position",new ct(r,3)),l.setAttribute("spacing",new ct(o,1)),l.setIndex(s),l}function LC(t=256){const e=new Uint8Array(t*t*4),n=[];let i=12345;const r=()=>(i=i*1103515245+12345&2147483647)/2147483647;for(let a=0;a<90;a++)n.push([r()*t,r()*t]);const o=(a,l,c)=>{const d=Math.floor(a),f=Math.floor(l),h=a-d,p=l-f,v=(u,g)=>{const y=Math.sin((u%c+c)%c*127.1+(g%c+c)%c*311.7)*43758.5453;return y-Math.floor(y)},_=h*h*(3-2*h),m=p*p*(3-2*p);return v(d,f)*(1-_)*(1-m)+v(d+1,f)*_*(1-m)+v(d,f+1)*(1-_)*m+v(d+1,f+1)*_*m};for(let a=0;a<t;a++)for(let l=0;l<t;l++){let c=1e9,d=1e9;for(const[_,m]of n){let u=Math.abs(l-_),g=Math.abs(a-m);u=Math.min(u,t-u),g=Math.min(g,t-g);const y=u*u+g*g;y<c?(d=c,c=y):y<d&&(d=y)}const f=Math.min(1,(Math.sqrt(d)-Math.sqrt(c))/22),h=o(l/32,a/32,t/32)*.45+o(l/16,a/16,t/16)*.3+o(l/8,a/8,t/8)*.17+o(l/4,a/4,t/4)*.08,p=Math.max(0,Math.min(1,h*1.15*(.75+.25*(1-f))-.05)),v=(a*t+l)*4;e[v]=p*255,e[v+1]=h*255,e[v+2]=(1-f)*255,e[v+3]=255}const s=new Ec(e,t,t,Wt);return s.wrapS=s.wrapT=Wo,s.magFilter=Kt,s.minFilter=ni,s.generateMipmaps=!0,s.needsUpdate=!0,s}function NC(t=256){let e=777;const n=()=>(e=e*1103515245+12345&2147483647)/2147483647,i=[];for(let c=0;c<140;c++){const d=2+Math.pow(n(),1.6)*42,f=n()*Math.PI*2,h=Math.round(Math.cos(f)*d),p=Math.round(Math.sin(f)*d);!h&&!p||i.push({kx:h,ky:p,a:Math.pow(Math.hypot(h,p),-1.75),p:n()*Math.PI*2})}const r=new Float32Array(t*t),o=new Float32Array(t*t);let s=0;for(const c of i){const d=2*Math.PI*c.kx/t,f=2*Math.PI*c.ky/t,h=c.a*2*Math.PI*c.kx,p=c.a*2*Math.PI*c.ky;for(let v=0;v<t;v++)for(let _=0;_<t;_++){const m=Math.cos(d*_+f*v+c.p);r[v*t+_]+=h*m,o[v*t+_]+=p*m}}for(let c=0;c<r.length;c++)s=Math.max(s,Math.abs(r[c]),Math.abs(o[c]));const a=new Uint8Array(t*t*4);for(let c=0;c<r.length;c++)a[c*4]=Math.round((.5+.5*r[c]/s)*255),a[c*4+1]=Math.round((.5+.5*o[c]/s)*255),a[c*4+3]=255;const l=new Ec(a,t,t,Wt);return l.wrapS=l.wrapT=Wo,l.magFilter=Kt,l.minFilter=ni,l.generateMipmaps=!0,l.anisotropy=4,l.needsUpdate=!0,l}function IC(t,e=!1){const n={...t,uWA:{value:new Float32Array(Mo*4)},uWB:{value:new Float32Array(Mo*4)},uRipple:{value:NC()},uRippleAmt:{value:1},uCenter:{value:new Re},uPix:{value:.001},uFoamAmt:{value:.5},uFoamTex:{value:LC()},uSeabed:{value:null},uSeabedCenter:{value:new Re},uSeabedSize:{value:1600},uWake:{value:null},uWakeCenter:{value:new Re},uWakeSize:{value:240},uBoatInv:{value:new rt},uDisp:{value:null},uSlope:{value:null},uFoamMap:{value:null},uHs:{value:1.3}};return{mat:new xn({uniforms:n,transparent:!0,defines:e?{USE_FFT:""}:{},vertexShader:`
      #define TILE ${wo.toFixed(1)}
      uniform sampler2D uDisp;
      float vhash(vec2 p) { vec3 p3 = fract(vec3(p.xyx) * 0.1031); p3 += dot(p3, p3.yzx + 33.33); return fract((p3.x + p3.y) * p3.z); }
      float vnoise2(vec2 p) { vec2 i = floor(p), f = fract(p); vec2 u = f * f * (3.0 - 2.0 * f);
        return mix(mix(vhash(i), vhash(i + vec2(1, 0)), u.x), mix(vhash(i + vec2(0, 1)), vhash(i + vec2(1, 1)), u.x), u.y); }
      #define GEO ${Mo}
      uniform vec4 uWA[GEO];
      uniform vec4 uWB[GEO];
      uniform vec2 uCenter;
      uniform float uTime;
      uniform sampler2D uWake;
      uniform vec2 uWakeCenter;
      uniform float uWakeSize;
      attribute float spacing;
      varying vec3 vWorld;
      varying vec2 vFlat;
      void main() {
        vec2 p = position.xz + uCenter;
        vec3 d = vec3(0.0);
        #ifdef USE_FFT
        // the FFT patch (repeating), calmer and rougher over large areas so
        // the repeat does not show; thinned out where the mesh is coarse
        float fade = 1.0 - smoothstep(${(wo/nc*1.5).toFixed(2)}, ${(wo/nc*7).toFixed(2)}, spacing);
        float ampm = 0.75 + 0.5 * vnoise2(p / 900.0 + 3.1);
        d = texture2D(uDisp, p / TILE).xyz * fade * ampm;
        #else
        for (int i = 0; i < GEO; i++) {
          vec4 A = uWA[i]; vec4 B = uWB[i];
          float L = 6.2831853 / A.z;
          float f = smoothstep(spacing * 2.5, spacing * 6.0, L);
          float th = A.z * dot(A.xy, p) - A.w * uTime + B.z;
          float c = cos(th), s = sin(th);
          d.xz += f * B.y * B.x * A.xy * c;
          d.y += f * B.x * s;
        }
        #endif
        vec3 wp = vec3(p.x + d.x, d.y, p.y + d.z);
        // the wake's ridges lift the water (up to about half a metre)
        vec2 wuv = vec2((p.x - uWakeCenter.x) / uWakeSize + 0.5, -(p.y - uWakeCenter.y) / uWakeSize + 0.5);
        if (wuv.x > 0.0 && wuv.x < 1.0 && wuv.y > 0.0 && wuv.y < 1.0) wp.y += (texture2D(uWake, wuv).b - 0.5) * ${mg.toFixed(2)} * 2.0;
        vWorld = wp;
        vFlat = p;
        gl_Position = projectionMatrix * viewMatrix * vec4(wp, 1.0);
      }`,fragmentShader:My+bC+`
      #define GEO ${Mo}
      uniform vec4 uWA[GEO];
      uniform vec4 uWB[GEO];
      uniform sampler2D uRipple;
      uniform float uRippleAmt;
      uniform float uPix;
      uniform float uFoamAmt;
      uniform sampler2D uFoamTex;
      uniform sampler2D uSeabed;
      uniform vec2 uSeabedCenter;
      uniform float uSeabedSize;
      uniform sampler2D uWake;
      uniform vec2 uWakeCenter;
      uniform float uWakeSize;
      uniform mat4 uBoatInv;
      uniform sampler2D uSlope;
      uniform sampler2D uFoamMap;
      uniform float uHs;
      varying vec3 vWorld;
      varying vec2 vFlat;
      #define TILE ${wo.toFixed(1)}

      vec2 topUV(vec2 p, vec2 c, float size) { return vec2((p.x - c.x) / size + 0.5, -(p.y - c.y) / size + 0.5); }

      void main() {
        vec3 boatLocal = (uBoatInv * vec4(vWorld, 1.0)).xyz;
        if (insideHull(boatLocal)) discard;

        vec3 toEye = cameraPosition - vWorld;
        float dist = length(toEye);
        vec3 V = toEye / dist;
        float foot = dist * uPix / max(sqrt(abs(V.y)), 0.12);

        // the surface's slope here, from every wave (and fine ripples near by)
        vec2 p = vFlat;
        float nx = 0.0, nz = 0.0, comp = 0.0, foamMap = 0.0;
        #ifdef USE_FFT
        float ampm = 0.75 + 0.5 * vnoise(p / 900.0 + 3.1);
        vec4 sl = texture2D(uSlope, p / TILE);
        nx = -sl.x * ampm; nz = -sl.y * ampm;
        comp = 1.0 - sl.z;
        foamMap = texture2D(uFoamMap, p / TILE).r * smoothstep(0.75, 1.05, ampm);
        #else
        for (int i = 0; i < GEO; i++) {
          vec4 A = uWA[i]; vec4 B = uWB[i];
          float L = 6.2831853 / A.z;
          float f = smoothstep(foot * 1.5, foot * 5.0, L);
          float th = A.z * dot(A.xy, p) - A.w * uTime + B.z;
          float c = cos(th), s = sin(th);
          float ka = A.z * B.x * f;
          nx -= A.x * ka * c;
          nz -= A.y * ka * c;
          comp += B.y * ka * s;
        }
        #endif
        // fine ripples: two layers drifting with the wind, at two scales
        vec2 wd = uWind;
        vec2 r1 = texture2D(uRipple, p / 7.3 + wd * uTime * 0.045).rg * 2.0 - 1.0;
        vec2 r2 = texture2D(uRipple, mat2(0.8, 0.6, -0.6, 0.8) * p / 2.9 + vec2(-wd.y, wd.x) * uTime * 0.03 + wd * uTime * 0.06).rg * 2.0 - 1.0;
        vec2 r3 = texture2D(uRipple, mat2(0.6, -0.8, 0.8, 0.6) * p / 23.0 + wd * uTime * 0.02).rg * 2.0 - 1.0;
        float near = 1.0 - smoothstep(80.0, 900.0, dist);
        #ifdef USE_FFT
        vec2 rip = (r1 * 0.09 + r2 * 0.07 * near) * uRippleAmt;
        #else
        vec2 rip = (r1 * 0.16 + r2 * 0.09 * near + r3 * 0.22) * uRippleAmt;
        #endif
        // the wake here: foam, bubbles, and the slope of its ridges
        vec4 wk = vec4(0.0, 0.0, 0.5, 0.0);
        vec2 wUV = topUV(vWorld.xz, uWakeCenter, uWakeSize);
        if (wUV.x > 0.002 && wUV.x < 0.998 && wUV.y > 0.002 && wUV.y < 0.998) {
          wk = texture2D(uWake, wUV);
          float e = 1.0 / 512.0, m = uWakeSize / 512.0 * 2.0;
          float k = ${mg.toFixed(2)} * 2.0;
          float dx = (texture2D(uWake, wUV + vec2(e, 0.0)).b - texture2D(uWake, wUV - vec2(e, 0.0)).b) * k / m;
          float dz = -(texture2D(uWake, wUV + vec2(0.0, e)).b - texture2D(uWake, wUV - vec2(0.0, e)).b) * k / m;
          nx -= dx; nz -= dz;
        }
        #ifdef USE_FFT
        vec3 N = normalize(vec3(nx + rip.x, 1.0, nz + rip.y));
        #else
        vec3 N = normalize(vec3(nx + rip.x, 1.0 - comp, nz + rip.y));
        #endif
        float jac = 1.0 - comp; // small where the crest is pinched (it breaks)

        // the ground under the water
        vec2 sbUV = topUV(vWorld.xz, uSeabedCenter, uSeabedSize);
        float ground = -60.0;
        if (sbUV.x > 0.0 && sbUV.x < 1.0 && sbUV.y > 0.0 && sbUV.y < 1.0) {
          float e = texture2D(uSeabed, sbUV).r;
          ground = e > 0.002 ? e * ${Ay.toFixed(1)} + ${Ty.toFixed(1)} : -60.0;
        }
        float depth = max(vWorld.y - ground, 0.0);

        // light on things here: the sky's and the sun's
        vec3 L = uSun;
        vec3 skyLight = mix(uHorizon, uZenith, 0.5) * 0.9;
        vec3 sunLight = uSunColor * max(L.y, 0.0);

        // the reflected sky
        vec3 R = reflect(-V, N);
        R.y = abs(R.y);
        // (clouds sharp in the water close by, blurred further off where the
        // surface is rough at the scale of a pixel)
        float blur = clamp(foot * 9.0 + smoothstep(8.0, 160.0, dist), 0.0, 1.0);
        vec3 refl = blur < 0.98 ? mix(skyColor(R, 3, false), skyMean(R), blur) : skyMean(R);
        float F = 0.02 + 0.98 * pow(1.0 - max(dot(N, V), 0.0), 5.0);
        F = min(F, 0.72);

        float crest = clamp(vWorld.y / max(uHs, 0.3) * 1.3 + 0.35, 0.0, 1.0);
        // the water's own colour
        #ifdef USE_FFT
        vec3 deep = vec3(0.005, 0.1, 0.125);
        #else
        vec3 deep = vec3(0.008, 0.13, 0.16);
        #endif
        vec3 shallow = vec3(0.06, 0.56, 0.52);
        vec3 sand = vec3(0.42, 0.52, 0.40);
        float sh = exp(-depth * 0.16) * (1.0 - smoothstep(7.0, 11.5, depth));
        vec3 body = mix(deep, shallow, sh);
        body = mix(body, sand, exp(-depth * 0.75) * 0.6);
        // light through the crests: brighter, greener where the wave stands up
        float through = pow(max(dot(-V, normalize(vec3(L.x, 0.25, L.z))), 0.0), 3.0);
        #ifdef USE_FFT
        body += vec3(0.0, 0.3, 0.26) * crest * crest * (0.3 + 1.5 * through);
        #else
        body += vec3(0.02, 0.2, 0.17) * crest * (0.35 + 1.2 * through);
        #endif
        // bubbles stirred under the surface: paler, greener water
        body = mix(body, vec3(0.15, 0.66, 0.62), clamp(wk.g, 0.0, 1.0) * 0.7);
        body *= skyLight * 0.75 + sunLight * 0.35;

        vec3 col = mix(body, refl, F);

        // the sun's glitter
        vec3 H = normalize(V + L);
        float shin = mix(900.0, 60.0, clamp(foot * 8.0, 0.0, 1.0));
        float spec = pow(max(dot(N, H), 0.0), shin) * (shin + 8.0) / 25.0;
        col += uSunColor * spec * F * 1.4 * (1.0 - uCover * 0.6);

        // foam: breaking crests, surf on the shallows, the wake. Each gives an
        // amount (0–1); foamLook turns an amount into foam: solid where fresh,
        // lace with holes as it thins, specks at the end (never a smooth blob)
        vec2 fp = vWorld.xz;
        float t1 = texture2D(uFoamTex, fp * 0.045 + uWind * uTime * 0.012).r;
        float t2 = texture2D(uFoamTex, fp * 0.13 - uWind * uTime * 0.02).r;
        float tex = t1 * 0.6 + t2 * 0.5;
        // (foam is drawn out into streaks along the wind)
        vec2 wf = vec2(dot(fp, uWind), dot(fp, vec2(-uWind.y, uWind.x)));
        float l1 = texture2D(uFoamTex, wf * vec2(0.2, 0.45) + 0.13).r;
        float l2 = texture2D(uFoamTex, wf * vec2(0.06, 0.15) + 0.37).r;
        float l3 = texture2D(uFoamTex, fp * 1.3 + 0.71).b;
        float lace = smoothstep(0.25, 0.85, l1 * 0.5 + l2 * 0.38 + l3 * 0.32 - 0.1);
        // (an amount a covers about 85·a % of the water with lace)
        #define foamLook(a) (smoothstep(0.0, 0.08, lace - (1.0 - 0.85 * clamp(a, 0.0, 1.0))) * mix(0.72, 1.0, lace))
        #ifdef USE_FFT
        float foam = foamLook(foamMap);
        #else
        float cap = smoothstep(0.78 - uFoamAmt * 0.32, 0.12, jac) * smoothstep(0.0, 0.3, uFoamAmt);
        float foam = foamLook(cap);
        #endif
        // surf: bands running in to the shore, thick only at the water's edge
        float shore = 1.0 - smoothstep(0.0, 1.1, depth);
        float band = 0.5 + 0.5 * sin(depth * 5.0 - uTime * 1.6 + tex * 4.0);
        float edge = 1.0 - smoothstep(0.0, 0.35, depth);
        float surf = max(edge * 0.95, shore * band * 0.8) * (0.4 + 0.6 * uFoamAmt);
        foam = max(foam, foamLook(surf));
        {
          // the wake's churn, and a little foam where the V's ridges stand highest
          float w = wk.r;
          float ridge = clamp((wk.b - 0.5) * 2.0, 0.0, 1.0);
          foam = max(foam, foamLook(w * 0.9));
          foam = max(foam, foamLook(smoothstep(0.4, 0.9, ridge) * 0.55));
        }
        foam = clamp(foam, 0.0, 1.0) * (1.0 - smoothstep(600.0, 2400.0, dist) * 0.7);
        vec3 foamCol = vec3(0.92, 0.95, 0.96) * (skyLight * 0.9 + sunLight * 0.45 * max(N.y, 0.0));
        col = mix(col, foamCol, foam * 0.92);

        // the haze over distance, toward the sky's own horizon colour
        vec3 flatDir = normalize(vec3(-V.x, 0.0, -V.z));
        vec3 fogC = mix(airColor(flatDir), uHorizon, 0.82);
        float fog = 1.0 - exp(-pow(dist * uHaze, 2.0));
        col = mix(col, fogC, fog);

        float alpha = smoothstep(0.0, 0.3, depth) ;
        alpha = max(alpha, foam);
        gl_FragColor = vec4(col, alpha);
        #include <tonemapping_fragment>
        #include <colorspace_fragment>
      }`}),uniforms:n}}function kC(t){const e=new Ct(DC(),t);return e.frustumCulled=!1,e}function UC(){return new xn({vertexShader:"varying float vy; void main(){ vec4 w = modelMatrix * vec4(position, 1.0); vy = w.y; gl_Position = projectionMatrix * viewMatrix * w; }",fragmentShader:`varying float vy; void main(){ gl_FragColor = vec4(clamp((vy - (${Ty.toFixed(1)})) / ${Ay.toFixed(1)}, 0.0, 1.0), 0.0, 0.0, 1.0); }`})}const ao=240,Ou=512,ws=16,Es=3200;class FC{constructor(){this.target=new qn(Ou,Ou,{type:oi,depthBuffer:!1}),this.center=new Re,this.marks=[],this.last=-1,this.scene=new Bs,this.cam=new _a(-ao/2,ao/2,ao/2,-ao/2,-10,10),this.geo=new Yt,this.pos=new Float32Array(6*3*Es),this.val=new Float32Array(6*3*Es),this.across=new Float32Array(6*Es),this.along=new Float32Array(6*Es),this.n=0,this.target.texture.minFilter=this.target.texture.magFilter=Kt;const e=(r,o)=>new en(r,o).setUsage(uM);this.geo.setAttribute("position",e(this.pos,3)),this.geo.setAttribute("v",e(this.val,3)),this.geo.setAttribute("c",e(this.across,1)),this.geo.setAttribute("a",e(this.along,1));const n=new xn({vertexShader:"attribute vec3 v; attribute float c; attribute float a; varying vec3 vv; varying float vc; varying float va; void main(){ vv = v; vc = c; va = a; gl_Position = projectionMatrix * viewMatrix * vec4(position, 1.0); }",fragmentShader:`varying vec3 vv; varying float vc; varying float va;
        void main(){
          float edge = sqrt(max(0.0, 1.0 - vc * vc));
          float ridge = cos(clamp(vc, -1.0, 1.0) * 1.5708); ridge *= ridge;
          float crests = 0.55 + 0.45 * sin(va + 0.9 * sin(va * 0.37));
          gl_FragColor = vec4(vv.x * edge, vv.y * edge, 0.5 + 0.5 * vv.z * ridge * crests, 1.0);
        }`,blending:kv,blendEquation:Uv,blendSrc:Ld,blendDst:Ld,depthTest:!1,depthWrite:!1,side:bn}),i=new Ct(this.geo,n);i.frustumCulled=!1,this.scene.add(i),this.cam.up.set(0,0,-1)}reset(){this.marks=[],this.last=-1}add(e,n){for(;this.marks.length&&n-this.marks[0].t>ws;)this.marks.shift();n-this.last<.1||(this.last=n,this.marks.push({x:e.x,z:e.z,fx:Math.sin(e.heading),fz:-Math.cos(e.heading),v:Math.max(0,e.speed),t:n}))}band(e,n,i,r,o,s,a,l,c,d){if(this.n>=Es||Math.max(...a,...l)<=.003)return;let f=i-e,h=r-n;const p=Math.hypot(f,h)||1;f/=p,h/=p;const v=-h,_=f,m=this.n*18,u=this.n*6,g=this.n*18,y=[e+v*o,n+_*o],x=[e-v*o,n-_*o],C=[i+v*s,r+_*s],A=[i-v*s,r-_*s];this.pos.set([y[0],0,y[1],x[0],0,x[1],C[0],0,C[1],C[0],0,C[1],x[0],0,x[1],A[0],0,A[1]],m),this.val.set([...a,...a,...l,...l,...a,...l],g),this.across.set([1,-1,1,1,-1,-1],u),this.along.set([c,c,d,d,c,d],u),this.n++}render(e,n,i){const r=ao/Ou;this.center.set(Math.round(n.x/r)*r,Math.round(n.z/r)*r),this.cam.position.set(this.center.x,5,this.center.y),this.cam.lookAt(this.center.x,0,this.center.y),this.n=0;const o={x:n.x,z:n.z,fx:Math.sin(n.heading),fz:-Math.cos(n.heading),v:Math.max(0,n.speed),t:i},s=[...this.marks,o],a=(_,m)=>[_.x+_.fx*ti*m,_.z+_.fz*ti*m],l=_=>ft(_.v/Sh,0,1);let c=0;for(let _=s.length-1;_>0;_--){const m=s[_],u=s[_-1],g=i-m.t,y=i-u.t,x=l(m),C=l(u),A=Math.hypot(m.x-u.x,m.z-u.z),T=c,b=c+A;c=b;const[w,S]=a(m,-.5),[P,z]=a(u,-.5),F=(W,$)=>[Math.min(1,$*2.6+($>.04?.15:0))*Math.pow(ft(1-W/(ws*.3),0,1),1.6),Math.min(1,$*2.2+($>.04?.2:0))*Math.pow(ft(1-W/ws,0,1),1.2),0],Y=(W,$)=>.9+$*.7+W*(.35+$*.45);this.band(w,S,P,z,Y(g,x),Y(y,C),F(g,x),F(y,C),T,b);const j=(W,$)=>[0,Math.min(.55,$*1.4)*Math.pow(ft(1-W/ws,0,1),1.5),0];this.band(w,S,P,z,Y(g,x)*1.4+.6,Y(y,C)*1.4+.6,j(g,x),j(y,C),T,b);for(const W of[1,-1]){const[$,D]=a(m,.42),[G,Z]=a(u,.42),le=g*m.v*.354,Se=y*u.v*.354,Ne=$-m.fz*W*le,q=D+m.fx*W*le,re=G-u.fz*W*Se,he=Z+u.fx*W*Se,ae=(ne,ve)=>{const je=Math.pow(ft(1-ne/(ws*.7),0,1),1.6)*ft(ne*3,0,1);return[Math.min(.85,ve*2)*je*Math.pow(ft(1-ne/6,0,1),1.3),Math.min(.25,ve*.8)*je,Math.min(1,ve*2.6)*je]},Pe=ne=>1.5+ne*.45;this.band(Ne,q,re,he,Pe(g),Pe(y),ae(g,x),ae(y,C),T*2.1,b*2.1)}}const d=l(o);if(d>.02)for(const _ of[1,-1]){const m=[.9,.72,.52,.32].map(u=>{const g=Gi(u)+.15+(.9-u)*.9,[y,x]=a(o,u-.5);return[y-o.fz*_*g,x+o.fx*_*g,u]});for(let u=0;u<m.length-1;u++){const g=y=>[Math.min(1,d*2.4)*(.5+.5*y),Math.min(1,d*2),Math.min(.8,d*2.2)*y];this.band(m[u][0],m[u][1],m[u+1][0],m[u+1][1],.35+(.9-m[u][2])*.5,.35+(.9-m[u+1][2])*.5,g(m[u][2]),g(m[u+1][2]),u*2,u*2+2)}}const f=this.n;this.geo.setDrawRange(0,f*6);for(const _ of["position","v","c","a"])this.geo.attributes[_].needsUpdate=!0;const h=e.getRenderTarget(),p=e.getClearColor(new we),v=e.getClearAlpha();e.setRenderTarget(this.target),e.setClearColor(new we(0,0,.5),1),e.clear(!0,!1,!1),e.render(this.scene,this.cam),e.setRenderTarget(h),e.setClearColor(p,v)}dispose(){this.target.dispose(),this.geo.dispose()}}const gg={KeyW:"up",ArrowUp:"up",KeyS:"down",ArrowDown:"down",KeyA:"left",ArrowLeft:"left",KeyD:"right",ArrowRight:"right",Space:"stop"};class OC{constructor(){this.held=new Set,this.tapped=new Set,this.onAny=()=>{}}press(e,n){n?(this.held.add(e),this.tapped.add(e),this.onAny()):this.held.delete(e)}release(){this.held.clear()}bind(e){const n=s=>{const a=s.target;return!!a&&(a.tagName==="INPUT"||a.tagName==="TEXTAREA"||a.isContentEditable)},i=s=>{const a=gg[s.code];!a||n(s)||s.metaKey||s.ctrlKey||s.altKey||(s.preventDefault(),this.press(a,!0))},r=s=>{const a=gg[s.code];a&&this.press(a,!1)},o=()=>this.release();return e.addEventListener("keydown",i),e.addEventListener("keyup",r),e.addEventListener("blur",o),()=>{e.removeEventListener("keydown",i),e.removeEventListener("keyup",r),e.removeEventListener("blur",o)}}read(){const e=i=>this.held.has(i)||this.tapped.has(i),n={throttle:e("up")&&!e("down")?1:e("down")&&!e("up")?-1:0,steer:e("left")&&!e("right")?-1:e("right")&&!e("left")?1:0,stop:e("stop")};return this.tapped.clear(),n}}const so=1600,vg=1024;class zC{constructor(e){this.canvas=e,this.controls=new OC,this.boat=ug(),this.scene=new Bs,this.camera=new Cn(55,1,.3,2e4),this.sky=dC(),this.dome=sg(this.sky),this.fft=null,this.sun=new jA(16777215,1),this.env=null,this.model=TC(),this.wake=new FC,this.waves=[],this.shapes=[],this.isles=new br,this.seabed={scene:new Bs,target:new qn(vg,vg,{depthBuffer:!0}),center:new Re(1e9,1e9),dirty:!0,cam:new _a(-so/2,so/2,so/2,-so/2,-100,600)},this.settings={time:15,weather:"cloudy",sea:"moderate",wind:225},this.t=0,this.last=0,this.raf=0,this.frames=[],this.scale=1,this.size={w:1,h:1},this.view={heading:0,yaw:0,pitch:.2,dist:19,touched:-1e9,pointers:new Map,pinch:0},this.labelsAt=0,this.status={boat:this.boat,near:null,labels:[],fps:60,scale:1},this.onFrame=()=>{},this.ground=(n,i)=>$A(this.shapes,n,i),this.waterAt=(n,i)=>this.fft?this.fft.long.heightAt(n,i,this.t):mC(this.waves,n,i,this.t),this.renderer=new TA({canvas:e,antialias:!0,powerPreference:"high-performance"}),this.renderer.toneMapping=Ov,this.renderer.toneMappingExposure=.85,this.renderer.outputColorSpace=gn,this.pmrem=new yh(this.renderer);try{pg.supported(this.renderer)&&(this.fft=new pg(this.renderer))}catch{this.fft=null}this.ocean=IC(this.sky,!!this.fft),this.fft&&(this.ocean.uniforms.uDisp.value=this.fft.disp.texture,this.ocean.uniforms.uSlope.value=this.fft.slope.texture),this.scene.add(this.dome,kC(this.ocean.mat),this.isles,this.model,this.sun,this.sun.target),this.ocean.uniforms.uSeabed.value=this.seabed.target.texture,this.ocean.uniforms.uSeabedSize.value=so,this.ocean.uniforms.uWake.value=this.wake.target.texture,this.ocean.uniforms.uWakeSize.value=ao,this.seabed.cam.up.set(0,0,-1),this.scene.fog=new bf(12372176,5e-4),this.setSettings(this.settings),this.bindView()}setSettings(e){var a,l,c,d;this.settings=e;const n=uC(e.time,e.weather);hC(this.sky,n,e.wind),this.sun.color.copy(n.sunColor).multiplyScalar(1/2.2),this.sun.intensity=2.2,this.scene.fog.color.copy(n.horizon),this.scene.fog.density=n.haze,this.waves=pC(e.sea,e.wind);const i=gC(this.waves,Mo);this.ocean.uniforms.uWA.value=i.A,this.ocean.uniforms.uWB.value=i.B,this.ocean.uniforms.uFoamAmt.value=((a=ca[e.sea])==null?void 0:a.foam)??.5,this.ocean.uniforms.uRippleAmt.value=.55+.45*(((l=ca[e.sea])==null?void 0:l.height)??1),(c=this.fft)==null||c.setSea(e.sea,e.wind),this.ocean.uniforms.uHs.value=(ic[e.sea]??ic.moderate).hs;const r=new Bs,o=sg(this.sky);r.add(o);const s=this.pmrem.fromScene(r,0,1,2e3);o.geometry.dispose(),o.material.dispose(),(d=this.env)==null||d.dispose(),this.env=s,this.scene.environment=s.texture,this.scene.environmentIntensity=.85}setIslands(e){for(const s of[...this.isles.children])this.isles.remove(s),yg(s);for(const s of[...this.seabed.scene.children])this.seabed.scene.remove(s);this.shapes=e.map(s=>new vy(s));const n=new ec({vertexColors:!0,roughness:.93,metalness:0}),i=new ec({vertexColors:!0,roughness:.85,side:bn}),r=iC(),o=UC();for(const s of this.shapes){const a=nC(s);this.isles.add(new Ct(a,n)),this.seabed.scene.add(new Ct(a,o));const l=rC(s);if(l.length){const c=new CA(r,i,l.length),d=new rt,f=new es;l.forEach((h,p)=>{f.setFromEuler(new $n(0,h.a,(h.s-1)*.2)),d.compose(new L(h.x,h.y,h.z),f,new L(h.s,h.s,h.s)),c.setMatrixAt(p,d)}),c.computeBoundingSphere(),this.isles.add(c)}}this.seabed.dirty=!0}get fftSea(){return!!this.fft}moveBoat(e,n,i){Object.assign(this.boat,ug(e,n,i)),this.view.heading=i,this.wake.reset()}start(){const e=n=>{this.raf=requestAnimationFrame(e);const i=this.last?Math.min(.5,(n-this.last)/1e3):1/60;this.last=n,this.frame(i)};this.raf=requestAnimationFrame(e)}stop(){cancelAnimationFrame(this.raf),this.raf=0,this.last=0}resize(){const e=this.canvas.parentElement||this.canvas,n=Math.max(1,e.clientWidth),i=Math.max(1,e.clientHeight),r=Math.min(window.devicePixelRatio||1,2)*this.scale;(n!==this.size.w||i!==this.size.h||Math.abs(this.renderer.getPixelRatio()-r)>.01)&&(this.size={w:n,h:i},this.renderer.setPixelRatio(r),this.renderer.setSize(n,i,!1),this.camera.aspect=n/i,this.camera.updateProjectionMatrix())}frame(e){this.resize(),this.advance(e),CC(this.model,this.boat),this.model.updateMatrixWorld(),this.ocean.uniforms.uBoatInv.value.copy(this.model.matrixWorld).invert(),this.follow(e),this.ocean.uniforms.uCenter.value.set(this.camera.position.x,this.camera.position.z),this.ocean.uniforms.uPix.value=2*Math.tan(this.camera.fov*Math.PI/360)/(this.size.h*this.renderer.getPixelRatio()),this.dome.position.copy(this.camera.position),this.sun.position.set(this.boat.x,0,this.boat.z).addScaledVector(this.sky.uSun.value,300),this.sun.target.position.set(this.boat.x,0,this.boat.z);const n=this.seabed;if(n.dirty||Math.hypot(this.boat.x-n.center.x,this.boat.z-n.center.y)>so*.25){n.center.set(Math.round(this.boat.x),Math.round(this.boat.z)),n.cam.position.set(n.center.x,400,n.center.y),n.cam.lookAt(n.center.x,0,n.center.y);const i=this.renderer,r=i.getRenderTarget();i.setRenderTarget(n.target),i.setClearColor(0,1),i.clear(),i.render(n.scene,n.cam),i.setRenderTarget(r),this.ocean.uniforms.uSeabedCenter.value.copy(n.center),n.dirty=!1}this.wake.render(this.renderer,this.boat,this.t),this.fft&&(this.fft.update(this.t,e),this.ocean.uniforms.uFoamMap.value=this.fft.foamTexture),this.ocean.uniforms.uWakeCenter.value.copy(this.wake.center),this.renderer.render(this.scene,this.camera),this.measure(e),this.report()}advance(e){const n=this.controls.read(),i=Math.max(1,Math.ceil(e/(1/30)));for(let r=0;r<i;r++)this.t+=e/i,AC(this.boat,n,e/i,this.waterAt,this.ground),this.wake.add(this.boat,this.t);this.sky.uTime.value=this.t}follow(e){const n=this.view,i=this.boat;let r=i.heading-n.heading;r=Math.atan2(Math.sin(r),Math.cos(r)),n.heading+=r*Math.min(1,e*1.6),performance.now()-n.touched>4e3&&Math.abs(i.speed)>1&&(n.yaw*=Math.exp(-e*.8));const o=n.heading+n.yaw,s=Math.sin(o),a=-Math.cos(o),l=n.dist*Math.cos(n.pitch),c=new L(i.x,i.y+1.8,i.z),d=new L(i.x-s*l,i.y+2+n.dist*Math.sin(n.pitch),i.z-a*l),f=Math.max(this.waterAt(d.x,d.z)+1.2,this.ground(d.x,d.z)+2);d.y<f&&(d.y=f),this.camera.position.copy(d),this.camera.lookAt(c)}bindView(){const e=this.canvas,n=this.view;e.style.touchAction="none",e.addEventListener("pointerdown",r=>{e.setPointerCapture(r.pointerId),n.pointers.set(r.pointerId,{x:r.clientX,y:r.clientY}),n.touched=performance.now()});const i=r=>{n.pointers.delete(r.pointerId),n.pinch=0};e.addEventListener("pointerup",i),e.addEventListener("pointercancel",i),e.addEventListener("pointermove",r=>{const o=n.pointers.get(r.pointerId);if(o){if(n.touched=performance.now(),n.pointers.size===2){const[s,a]=[...n.pointers.values()];o.x=r.clientX,o.y=r.clientY;const l=Math.hypot(s.x-a.x,s.y-a.y);n.pinch&&(n.dist=ft(n.dist*(n.pinch/l),7,140)),n.pinch=l;return}n.yaw-=(r.clientX-o.x)*.006,n.pitch=ft(n.pitch+(r.clientY-o.y)*.004,.02,1.35),o.x=r.clientX,o.y=r.clientY}}),e.addEventListener("wheel",r=>{r.preventDefault(),n.touched=performance.now(),n.dist=ft(n.dist*Math.exp(r.deltaY*.0012),7,140)},{passive:!1}),e.addEventListener("dblclick",()=>{n.yaw=0,n.pitch=.2,n.dist=19})}measure(e){if(this.frames.push(e),this.frames.length<45)return;const n=this.frames.reduce((i,r)=>i+r,0)/this.frames.length;this.frames=[],this.status.fps=Math.round(1/n),n>1/40&&this.scale>.5?this.scale=Math.max(.5,this.scale-.1):n<1/57&&this.scale<1&&(this.scale=Math.min(1,this.scale+.05)),this.status.scale=this.scale}report(){const e=this.boat;let n=null;for(const r of this.shapes){const o=Math.hypot(e.x-r.isle.x,e.z-r.isle.z)-r.isle.size;o<Math.max(140,r.isle.size*.9)&&(!n||o<n.dist)&&(n={isle:r.isle,dist:Math.max(0,o)})}this.status.near=n;const i=performance.now();if(i-this.labelsAt>120){this.labelsAt=i;const r=[],o=new L;for(const a of this.shapes){if(!a.isle.name)continue;const l=Math.hypot(e.x-a.isle.x,e.z-a.isle.z);l>3200||l<a.isle.size+60||(o.set(a.isle.x,Math.max(6,a.isle.height)+8,a.isle.z).project(this.camera),!(o.z>1||Math.abs(o.x)>1.05||Math.abs(o.y)>1.05)&&r.push({id:a.isle.id,name:a.isle.name,x:(o.x*.5+.5)*this.size.w,y:(-o.y*.5+.5)*this.size.h,dist:l}))}r.sort((a,l)=>a.dist-l.dist);const s=[];for(const a of r)s.every(l=>Math.abs(l.y-a.y)>26||Math.abs(l.x-a.x)>120)&&s.push(a);this.status.labels=s}this.onFrame(this.status)}dispose(){var e,n;this.stop(),yg(this.scene),this.wake.dispose(),(e=this.fft)==null||e.dispose(),this.seabed.target.dispose(),(n=this.env)==null||n.dispose(),this.pmrem.dispose(),this.renderer.dispose()}}function yg(t){t.traverse(e=>{var r,o,s;const n=e;(o=(r=n.geometry)==null?void 0:r.dispose)==null||o.call(r);const i=n.material;Array.isArray(i)?i.forEach(a=>a.dispose()):(s=i==null?void 0:i.dispose)==null||s.call(i)})}function BC(){var t;try{const e=document.createElement("canvas"),n=e.getContext("webgl2")||e.getContext("webgl");return(t=n==null?void 0:n.getExtension("WEBGL_lose_context"))==null||t.loseContext(),!!n}catch{return!1}}class HC{constructor(){this.ctx=null,this.master=null,this.sea=null,this.spray=null,this.engine=null,this.swellAt=0,this.on=!0}start(){if(this.ctx){this.ctx.resume();return}const e=window.AudioContext||window.webkitAudioContext;if(!e)return;const n=new e;this.ctx=n;const i=n.createGain();i.gain.value=this.on?.55:0,i.connect(n.destination),this.master=i;const r=n.sampleRate*4,o=n.createBuffer(1,r,n.sampleRate),s=o.getChannelData(0);let a=0;for(let T=0;T<r;T++)a=(a+.035*(Math.random()*2-1))/1.035,s[T]=a*3.2;const l=()=>{const T=n.createBufferSource();return T.buffer=o,T.loop=!0,T.loopStart=Math.random()*3,T.start(0,Math.random()*3),T},c=n.createBiquadFilter();c.type="lowpass",c.frequency.value=520;const d=n.createGain();d.gain.value=.5,l().connect(c).connect(d).connect(i),this.sea=d;const f=n.createBiquadFilter();f.type="bandpass",f.frequency.value=1800,f.Q.value=.5;const h=n.createGain();h.gain.value=0;const p=n.createBuffer(1,r,n.sampleRate),v=p.getChannelData(0);for(let T=0;T<r;T++)v[T]=Math.random()*2-1;const _=n.createBufferSource();_.buffer=p,_.loop=!0,_.start(),_.connect(f).connect(h).connect(i),this.spray=h;const m=n.createOscillator();m.type="sawtooth",m.frequency.value=42;const u=n.createOscillator();u.type="square",u.frequency.value=21;const g=n.createBiquadFilter();g.type="lowpass",g.frequency.value=260,g.Q.value=2;const y=n.createGain();y.gain.value=.05;const x=n.createGain();x.gain.value=.35;const C=n.createOscillator();C.frequency.value=10;const A=n.createGain();A.gain.value=.025,C.connect(A).connect(y.gain),m.connect(g),u.connect(x).connect(g),g.connect(y).connect(i),m.start(),u.start(),C.start(),this.engine={gain:y,osc:m,sub:u,filter:g,chug:C}}setOn(e){this.on=e,this.ctx&&this.master&&this.master.gain.setTargetAtTime(e?.55:0,this.ctx.currentTime,.1)}update(e,n,i){const r=this.ctx;if(!r||!this.engine||!this.sea||!this.spray)return;const o=r.currentTime,s=Math.abs(n),a=ft(Math.abs(e)/13,0,1),l=38+s*46+a*10;if(this.engine.osc.frequency.setTargetAtTime(l,o,.35),this.engine.sub.frequency.setTargetAtTime(l/2,o,.35),this.engine.chug.frequency.setTargetAtTime(l/4,o,.35),this.engine.filter.frequency.setTargetAtTime(220+s*520,o,.3),this.engine.gain.gain.setTargetAtTime(.04+s*.09,o,.3),this.spray.gain.setTargetAtTime(.012+a*.07+i*.015,o,.4),o>this.swellAt){const c=2.5+Math.random()*4;this.sea.gain.setTargetAtTime((.25+Math.random()*.45)*(.6+i*.6),o,c/3),this.swellAt=o+c}}stop(){var e;(e=this.ctx)==null||e.close(),this.ctx=null,this.engine=null}}function Cy(t,e){return t.map(n=>{const i=new vy(n),r=oC(i,Math.max(1.5,i.reach/90)),o=document.createElement("canvas");o.width=o.height=r.n;const s=o.getContext("2d",{willReadFrequently:!0}),a=s.createImageData(r.n,r.n),l=f=>(s.fillStyle=f,s.fillRect(0,0,1,1),s.getImageData(0,0,1,1).data),c=l(e.land),d=l(e.shallow);s.clearRect(0,0,1,1);for(let f=0;f<r.cells.length;f++){const h=r.cells[f];if(!h)continue;const p=h===2?c:d;a.data.set([p[0],p[1],p[2],h===2?255:150],f*4)}return s.putImageData(a,0,0),{isle:n,img:o,ext:r.ext}})}function Mh(t){const e=getComputedStyle(t),n=(i,r)=>e.getPropertyValue(i).trim()||r;return{sea:n("--oc-sea","#e9ecef"),shallow:n("--oc-shallow","#cfd6db"),land:n("--oc-land","#5b5f63"),ink:n("--fg","#111"),faint:n("--faint","#999"),accent:n("--fg","#111"),bg:n("--bg","#fff")}}function by(t,e,n,i,r,o,s,a={}){const l=h=>e/2+(h-i.cx)*i.scale,c=h=>n/2+(h-i.cz)*i.scale;t.fillStyle=s.sea,t.fillRect(0,0,e,n),t.imageSmoothingEnabled=!0;for(const h of r){const p=h.ext/2,v=l(h.isle.x-p),_=c(h.isle.z-p),m=h.ext*i.scale;v>e||_>n||v+m<0||_+m<0||t.drawImage(h.img,v,_,m,m)}if(a.course&&(t.strokeStyle=s.ink,t.globalAlpha=.55,t.setLineDash([4,4]),t.lineWidth=1.5,t.beginPath(),t.moveTo(l(o.x),c(o.z)),t.lineTo(l(a.course.x),c(a.course.z)),t.stroke(),t.setLineDash([]),t.globalAlpha=1),a.names){t.font="500 12px -apple-system, 'PingFang SC', 'Microsoft YaHei', 'Segoe UI', sans-serif",t.textAlign="center",t.textBaseline="top";for(const h of r){if(!h.isle.name)continue;const p=l(h.isle.x),v=c(h.isle.z)+Math.max(6,h.isle.size*.9*i.scale)+4;if(p<-60||p>e+60||v<-20||v>n+20)continue;const _=a.selected===h.isle.id;t.lineWidth=3,t.strokeStyle=s.sea,t.strokeText(h.isle.name,p,v),t.fillStyle=_?s.ink:a.found&&!a.found.has(h.isle.id)?s.faint:s.ink,t.fillText(h.isle.name,p,v),_&&(t.strokeStyle=s.ink,t.lineWidth=1.5,t.beginPath(),t.arc(l(h.isle.x),c(h.isle.z),Math.max(8,h.isle.size*i.scale*1.15),0,Math.PI*2),t.stroke())}}const d=l(o.x),f=c(o.z);t.save(),t.translate(d,f),t.rotate(o.heading),t.beginPath(),t.moveTo(0,-8),t.lineTo(5.5,6),t.lineTo(0,3),t.lineTo(-5.5,6),t.closePath(),t.fillStyle=s.ink,t.fill(),t.lineWidth=1.5,t.strokeStyle=s.bg,t.stroke(),t.restore()}function ua(t){return t>=1e3?ee("{0} 公里",(t/1e3).toFixed(t>=1e4?0:1)):ee("{0} 米",Math.round(t/10)*10)}const VC=1.94384,zu={island:()=>ee("岛"),rocks:()=>ee("礁石"),stack:()=>ee("海蚀柱"),sandbar:()=>ee("沙洲")},Ry=t=>{var e;return((e=zu[t])==null?void 0:e.call(zu))??t},GC={clear:()=>ee("晴"),cloudy:()=>ee("多云"),overcast:()=>ee("阴")},WC={calm:()=>ee("平静"),moderate:()=>ee("中浪"),rough:()=>ee("大浪")},Py=()=>[ee("北"),ee("东北"),ee("东"),ee("东南"),ee("南"),ee("西南"),ee("西"),ee("西北")],Dy=t=>Py()[Math.round((t%360+360)%360/45)%8],XC=t=>`${String(Math.floor(t)).padStart(2,"0")}:${String(Math.round(t%1*60)%60).padStart(2,"0")}`;function jC({heading:t,course:e}){const n=t*180/Math.PI,i=[];for(let o=Math.floor((n-70)/15)*15;o<=n+70;o+=15){const s=(o-n)*2.2,a=(o%360+360)%360,l=a%45===0;i.push(V.jsx("span",{className:`oc-tick${l?" is-main":""}`,style:{transform:`translateX(${s}px)`},children:l?V.jsx("b",{children:Py()[a/45]}):null},o))}let r=null;if(e!=null){let o=(e*180/Math.PI-n)%360;o>180&&(o-=360),o<-180&&(o+=360),r=V.jsx("span",{className:"oc-course",style:{transform:`translateX(${Math.max(-150,Math.min(150,o*2.2))}px)`}})}return V.jsxs("div",{className:"oc-compass","aria-label":ee("航向 {0}°",Math.round(n)%360),children:[V.jsxs("div",{className:"oc-ticks",children:[i,r]}),V.jsx("span",{className:"oc-needle"})]})}function YC({s:t}){const e=t.boat,n=Math.round(e.heading*180/Math.PI)%360,i=e.throttle;return V.jsxs("div",{className:"oc-gauges",children:[V.jsxs("div",{className:"oc-speed",children:[V.jsx("b",{children:(Math.abs(e.speed)*VC).toFixed(1)}),V.jsx("span",{children:ee("节")})]}),V.jsxs("div",{className:"oc-lever",title:ee("油门（W / S）"),children:[V.jsx("span",{className:"oc-lever-zero"}),V.jsx("span",{className:`oc-lever-fill${i<0?" is-back":""}`,style:i>=0?{left:"28.57%",width:`${i*71.43}%`}:{left:`${28.57+i*71.43}%`,width:`${-i*71.43}%`}})]}),V.jsxs("div",{className:"oc-gauge-note",children:[i<-.01?ee("倒车"):i>.01?ee("油门 {0}%",Math.round(i*100)):ee("空挡")," · ",Dy(n)," ",n,"°"]})]})}function qC({get:t,isles:e,course:n,onOpen:i}){const r=ze.useRef(null);return ze.useEffect(()=>{var p;const o=r.current;let s=[],a=0,l=Mh(o),c=0;const d=()=>{l=Mh(o),s=Cy(e,l)};d();const f=window.matchMedia("(prefers-color-scheme: dark)");(p=f.addEventListener)==null||p.call(f,"change",d);const h=v=>{if(a=requestAnimationFrame(h),v-c<66)return;c=v;const _=t();if(!_)return;const m=window.devicePixelRatio||1,u=o.clientWidth;o.width!==Math.round(u*m)&&(o.width=o.height=Math.round(u*m));const g=o.getContext("2d");g.setTransform(m,0,0,m,0,0),by(g,u,u,{cx:_.boat.x,cz:_.boat.z,scale:u/1500},s,_.boat,l,{course:n})};return a=requestAnimationFrame(h),()=>{var v;cancelAnimationFrame(a),(v=f.removeEventListener)==null||v.call(f,"change",d)}},[e,n,t]),V.jsxs("button",{type:"button",className:"oc-mini",onClick:i,"aria-label":ee("打开地图（M）"),title:ee("打开地图（M）"),children:[V.jsx("canvas",{ref:r}),V.jsx("span",{className:"oc-mini-n",children:ee("北")})]})}function $C({get:t,isles:e,found:n,course:i,onCourse:r,onClose:o}){var _,m,u;const s=ze.useRef(null),a=(_=t())==null?void 0:_.boat,[l,c]=ze.useState(i?((m=e.find(g=>g.name===i.name))==null?void 0:m.id)??null:null),d=ze.useRef({cx:(a==null?void 0:a.x)??0,cz:(a==null?void 0:a.z)??0,scale:0}),f=ze.useRef(null),h=e.find(g=>g.id===l)||null;ze.useEffect(()=>{var T;const g=s.current,y=Mh(g),x=Cy(e,y);if(!d.current.scale){const b=(T=t())==null?void 0:T.boat,w=[...e.map(j=>j.x-j.size),...e.map(j=>j.x+j.size),(b==null?void 0:b.x)??0],S=[...e.map(j=>j.z-j.size),...e.map(j=>j.z+j.size),(b==null?void 0:b.z)??0],P=g.clientWidth||800,z=g.clientHeight||600,F=Math.max(400,Math.max(...w)-Math.min(...w)),Y=Math.max(400,Math.max(...S)-Math.min(...S));d.current={cx:(Math.max(...w)+Math.min(...w))/2,cz:(Math.max(...S)+Math.min(...S))/2,scale:Math.min(P/F,z/Y)*.8}}let C=0;const A=()=>{C=requestAnimationFrame(A);const b=t();if(!b)return;const w=window.devicePixelRatio||1,S=g.clientWidth,P=g.clientHeight;(g.width!==Math.round(S*w)||g.height!==Math.round(P*w))&&(g.width=Math.round(S*w),g.height=Math.round(P*w));const z=g.getContext("2d");z.setTransform(w,0,0,w,0,0),by(z,S,P,d.current,x,b.boat,y,{names:!0,course:i,selected:l??void 0,found:n})};return C=requestAnimationFrame(A),()=>cancelAnimationFrame(C)},[e,i,l,n,t]);const p=g=>{const y=s.current.getBoundingClientRect(),x=d.current;return{x:x.cx+(g.clientX-y.left-y.width/2)/x.scale,z:x.cz+(g.clientY-y.top-y.height/2)/x.scale}},v=(u=t())==null?void 0:u.boat;return V.jsxs("div",{className:"oc-sheet",role:"dialog","aria-label":ee("地图"),children:[V.jsxs("div",{className:"oc-sheet-head",children:[V.jsx("b",{children:ee("地图")}),V.jsx("span",{className:"faint",children:ee("到过 {0} / {1} 处",e.filter(g=>n.has(g.id)).length,e.length)}),V.jsx("span",{className:"top-gap"}),V.jsx("button",{type:"button",className:"icon-btn",onClick:o,"aria-label":ee("关闭"),children:V.jsx(oc,{className:"ico"})})]}),V.jsxs("div",{className:"oc-map-wrap",children:[V.jsx("canvas",{ref:s,className:"oc-map",onPointerDown:g=>{g.target.setPointerCapture(g.pointerId),f.current={x:g.clientX,y:g.clientY,moved:!1}},onPointerMove:g=>{const y=f.current;if(!y)return;const x=g.clientX-y.x,C=g.clientY-y.y;Math.abs(x)+Math.abs(C)>3&&(y.moved=!0),d.current.cx-=x/d.current.scale,d.current.cz-=C/d.current.scale,y.x=g.clientX,y.y=g.clientY},onPointerUp:g=>{const y=f.current;if(f.current=null,y!=null&&y.moved)return;const x=p(g),C=d.current,A=e.map(T=>({i:T,d:Math.hypot(T.x-x.x,T.z-x.z)-Math.max(T.size*1.1,14/C.scale)})).filter(T=>T.d<0).sort((T,b)=>T.d-b.d)[0];c(A?A.i.id:null)},onWheel:g=>{const y=d.current;y.scale=Math.min(4,Math.max(.02,y.scale*Math.exp(-g.deltaY*.0015)))}}),V.jsx("div",{className:"oc-places","aria-label":ee("地点"),children:e.filter(g=>g.name).map(g=>({i:g,d:v?Math.max(0,Math.hypot(v.x-g.x,v.z-g.z)-g.size):0})).sort((g,y)=>g.d-y.d).map(({i:g,d:y})=>V.jsxs("button",{type:"button",className:`oc-place${l===g.id?" is-on":""}`,onClick:()=>{c(g.id);const x=d.current;x.cx=g.x,x.cz=g.z},children:[V.jsx("span",{className:`oc-place-dot${n.has(g.id)?" is-found":""}`}),V.jsx("b",{children:g.name}),V.jsx("span",{className:"faint",children:ua(y)})]},g.id))}),h&&V.jsxs("div",{className:"oc-pick",children:[V.jsxs("div",{className:"oc-pick-head",children:[V.jsx("b",{children:h.name||ee("无名")}),V.jsxs("span",{className:"faint",children:[Ry(h.kind),v?` · ${ua(Math.max(0,Math.hypot(v.x-h.x,v.z-h.z)-h.size))}`:""]})]}),h.note&&V.jsx("p",{className:"oc-pick-note",children:h.note.split(/\n{2,}/)[0].slice(0,160)}),V.jsx("div",{className:"oc-pick-row",children:(i==null?void 0:i.name)===h.name?V.jsx("button",{type:"button",className:"btn",onClick:()=>r(null),children:ee("取消航向")}):V.jsxs("button",{type:"button",className:"btn btn-primary",onClick:()=>{r({x:h.x,z:h.z,name:h.name||ee("无名")}),o()},children:[V.jsx(f_,{className:"ico"}),ee("开往这里")]})})]})]})]})}function KC({isle:t,dist:e,onClose:n}){const i=t.note.split(/\n{2,}/).map(r=>r.trim()).filter(Boolean).slice(0,3);return V.jsxs("div",{className:"oc-card",role:"status",children:[V.jsxs("div",{className:"oc-card-head",children:[V.jsx("b",{children:t.name||ee("无名")}),V.jsxs("span",{className:"faint",children:[Ry(t.kind)," · ",e<15?ee("就在旁边"):ua(e)]}),V.jsx("span",{className:"top-gap"}),V.jsx("button",{type:"button",className:"icon-btn",onClick:n,"aria-label":ee("收起"),children:V.jsx(oc,{className:"ico"})})]}),i.map((r,o)=>V.jsx("p",{children:r.replace(/[*_`#>]/g,"")},o))]})}function ZC({labels:t}){return V.jsx(V.Fragment,{children:t.map(e=>V.jsxs("div",{className:"oc-label",style:{left:Math.round(e.x),top:Math.round(e.y)},children:[V.jsx("b",{children:e.name}),V.jsx("span",{children:ua(e.dist)})]},e.id))})}function JC({value:t,title:e,onChange:n,onTitle:i,onStartHere:r,onGoStart:o,onClose:s}){return V.jsxs("div",{className:"oc-panel pop",role:"dialog","aria-label":ee("海况和天气"),children:[V.jsxs("div",{className:"oc-panel-head",children:[V.jsx("b",{children:ee("海况和天气")}),V.jsx("span",{className:"faint",children:ee("大家看到的一样")}),V.jsx("span",{className:"top-gap"}),V.jsx("button",{type:"button",className:"icon-btn",onClick:s,"aria-label":ee("关闭"),children:V.jsx(oc,{className:"ico"})})]}),V.jsxs("label",{className:"oc-field",children:[V.jsx("span",{children:ee("名字")}),V.jsx("input",{className:"inp",value:e,maxLength:60,placeholder:ee("这片海叫什么"),onChange:a=>i(a.target.value)})]}),V.jsxs("label",{className:"oc-field",children:[V.jsxs("span",{children:[ee("时间")," ",V.jsx("b",{className:"oc-num",children:XC(t.time)})]}),V.jsx("input",{type:"range",min:5,max:19.5,step:.25,value:t.time,onChange:a=>n({...t,time:Number(a.target.value)})})]}),V.jsxs("div",{className:"oc-field",children:[V.jsx("span",{children:ee("天气")}),V.jsx("div",{className:"seg",children:["clear","cloudy","overcast"].map(a=>V.jsx("button",{type:"button",className:t.weather===a?"is-on":"",onClick:()=>n({...t,weather:a}),children:GC[a]()},a))})]}),V.jsxs("div",{className:"oc-field",children:[V.jsx("span",{children:ee("海况")}),V.jsx("div",{className:"seg",children:["calm","moderate","rough"].map(a=>V.jsx("button",{type:"button",className:t.sea===a?"is-on":"",onClick:()=>n({...t,sea:a}),children:WC[a]()},a))})]}),V.jsxs("label",{className:"oc-field",children:[V.jsxs("span",{children:[ee("风向")," ",V.jsxs("b",{className:"oc-num",children:[ee("{0}风",Dy(t.wind))," ",Math.round(t.wind),"°"]})]}),V.jsx("input",{type:"range",min:0,max:359,step:1,value:t.wind,onChange:a=>n({...t,wind:Number(a.target.value)})})]}),V.jsxs("div",{className:"oc-panel-foot",children:[V.jsx("button",{type:"button",className:"btn",onClick:o,children:ee("回到出发点")}),V.jsx("button",{type:"button",className:"btn",onClick:r,children:ee("把这里设为出发点")})]})]})}function QC({onClose:t}){const e=[["W / ↑",ee("加油门")],["S / ↓",ee("减油门、倒车")],["A D / ← →",ee("转舵")],[ee("空格"),ee("停车")],[ee("拖动"),ee("转视角")],[ee("滚轮"),ee("拉近拉远")],[ee("双击"),ee("视角回到船后")],["M",ee("地图")]];return V.jsxs("div",{className:"oc-keys pop",role:"dialog","aria-label":ee("操作方法"),children:[V.jsxs("div",{className:"oc-panel-head",children:[V.jsx("b",{children:ee("操作方法")}),V.jsx("span",{className:"top-gap"}),V.jsx("button",{type:"button",className:"icon-btn",onClick:t,"aria-label":ee("关闭"),children:V.jsx(oc,{className:"ico"})})]}),V.jsx("dl",{children:e.map(([n,i])=>V.jsxs("div",{children:[V.jsx("dt",{children:n}),V.jsx("dd",{children:i})]},n))})]})}function eb({controls:t}){const e=(n,i,r)=>V.jsx("button",{type:"button",className:"oc-pad-btn","aria-label":i,onPointerDown:o=>{o.target.setPointerCapture(o.pointerId),t.press(n,!0)},onPointerUp:()=>t.press(n,!1),onPointerCancel:()=>t.press(n,!1),onContextMenu:o=>o.preventDefault(),children:r});return V.jsxs(V.Fragment,{children:[V.jsxs("div",{className:"oc-pad is-left",children:[e("left",ee("左舵"),V.jsx(l_,{className:"ico"})),e("right",ee("右舵"),V.jsx(c_,{className:"ico"}))]}),V.jsxs("div",{className:"oc-pad is-right",children:[e("up",ee("加油门"),V.jsx(u_,{className:"ico"})),e("stop",ee("停车"),V.jsx(m_,{className:"ico"})),e("down",ee("减油门"),V.jsx(a_,{className:"ico"}))]})]})}const tb=()=>{var t;return((t=window.matchMedia)==null?void 0:t.call(window,"(pointer: coarse)").matches)??!1};function nb(){TS(Vi);const t=ze.useRef(null),e=ze.useRef(null),n=ze.useRef(null),i=ze.useRef(new HC),[r,o]=ze.useState(null),[s,a]=ze.useState(()=>BC()?"":ee("这台电脑上画不了 3D（浏览器没有 WebGL）。")),[l,c]=ze.useState(""),[d,f]=ze.useState(null),[h,p]=ze.useState(null),[v,_]=ze.useState(null),[m,u]=ze.useState(null),[g,y]=ze.useState(()=>new Set),[x,C]=ze.useState(!0),[A,T]=ze.useState(!0),[b]=ze.useState(tb),w=Cc(),S=d??rg(w),P=ze.useMemo(()=>cC(),[Vi.version]),z=JSON.stringify(P),F=ze.useCallback(()=>n.current,[]);ze.useEffect(()=>{if(!t.current)return;let ne;try{ne=new zC(t.current)}catch(Me){a(ee("3D 画面没能启动：{0}",Me.message));return}e.current=ne;const ve=Sy();y(new Set(ve.found||[])),typeof ve.soundOn=="boolean"&&(C(ve.soundOn),i.current.on=ve.soundOn);const je=og();typeof ve.x=="number"&&typeof ve.z=="number"?ne.moveBoat(ve.x,ve.z,ve.heading||0):ne.moveBoat(je.x,je.z,je.heading);let Ue=0;ne.onFrame=Me=>{n.current=Me;const nt=performance.now();nt-Ue>120&&(Ue=nt,o({...Me}),i.current.update(Me.boat.speed,Me.boat.throttle,ca[rg().sea].height/1.9))};const ot=ne.controls.bind(window);ne.controls.onAny=()=>{T(!1),i.current.start()};const I=()=>i.current.start();window.addEventListener("pointerdown",I,{once:!0}),ne.start(),window.__ocean=ne;const Nt=()=>{const Me=ne.boat;Fu({x:Math.round(Me.x*10)/10,z:Math.round(Me.z*10)/10,heading:Math.round(Me.heading*1e3)/1e3})},Xe=window.setInterval(Nt,2e3);window.addEventListener("pagehide",Nt);const Ge=()=>{document.hidden?ne.stop():ne.start()};return document.addEventListener("visibilitychange",Ge),()=>{Nt(),ot(),window.clearInterval(Xe),window.removeEventListener("pagehide",Nt),document.removeEventListener("visibilitychange",Ge),i.current.stop(),ne.dispose(),e.current=null}},[]);const Y=JSON.stringify(S);ze.useEffect(()=>{var ne;(ne=e.current)==null||ne.setSettings(S)},[Y]),ze.useEffect(()=>{var ne;(ne=e.current)==null||ne.setIslands(P)},[z]);const j=ze.useRef(0),W=ne=>{f(ne),window.clearTimeout(j.current),j.current=window.setTimeout(()=>{Uu({time:ne.time,weather:ne.weather,sea:ne.sea,wind:ne.wind}).finally(()=>f(null))},450)},$=ze.useRef(0),D=ne=>{p(ne),window.clearTimeout($.current),$.current=window.setTimeout(()=>{Uu({title:ne.trim()}).finally(()=>p(null))},500)},G=(r==null?void 0:r.near)??null;ze.useEffect(()=>{if(!G||g.has(G.isle.id))return;const ne=new Set(g);ne.add(G.isle.id),y(ne),Fu({found:[...ne]})},[G==null?void 0:G.isle.id]),ze.useEffect(()=>{G||_(null)},[G==null?void 0:G.isle.id]),ze.useEffect(()=>{m&&G&&G.isle.name===m.name&&G.dist<60&&u(null)},[G==null?void 0:G.dist,m]),ze.useEffect(()=>{const ne=ve=>{var Ue,ot;if(ve.key==="Escape"){c(""),(ot=(Ue=document.activeElement)==null?void 0:Ue.blur)==null||ot.call(Ue);return}const je=ve.target;je.tagName==="INPUT"||je.tagName==="TEXTAREA"||ve.code==="KeyM"&&!ve.metaKey&&!ve.ctrlKey&&(ve.preventDefault(),c(I=>I==="map"?"":"map"))};return window.addEventListener("keydown",ne),()=>window.removeEventListener("keydown",ne)},[]);const Z=()=>{const ne=!x;C(ne),i.current.setOn(ne),i.current.start(),Fu({soundOn:ne})},le=()=>{var ve;const ne=og();(ve=e.current)==null||ve.moveBoat(ne.x,ne.z,ne.heading),c("")},Se=()=>{var ve;const ne=(ve=e.current)==null?void 0:ve.boat;ne&&(Uu({start_x:Math.round(ne.x),start_z:Math.round(ne.z),start_heading:Math.round(ne.heading*180/Math.PI)}),Vi.flash(ee("出发点改到了这里")))},Ne=r==null?void 0:r.boat,q=m&&Ne?Math.atan2(m.x-Ne.x,-(m.z-Ne.z)):null,re=m&&Ne?Math.hypot(m.x-Ne.x,m.z-Ne.z):0,he=h??(w.title||ee("航海")),ae=G?ee("在 {0} 附近",G.isle.name||ee("无名")):ee("开阔海面"),Pe=Ne&&Ne.aground<1.2;return s?V.jsx("div",{className:"oc oc-fail",children:V.jsxs("div",{children:[V.jsx("b",{children:ee("打不开 3D 海面")}),V.jsx("p",{children:s})]})}):V.jsxs("div",{className:"oc",children:[V.jsx("canvas",{ref:t,className:"oc-canvas",tabIndex:0,"aria-label":ee("海面：用 W A S D 或方向键开船")}),r&&V.jsx(ZC,{labels:r.labels}),V.jsxs("div",{className:"oc-top",children:[V.jsxs("div",{className:"oc-chip",children:[V.jsx("b",{children:he}),V.jsx("span",{children:ae})]}),V.jsx("span",{className:"top-gap"}),V.jsxs("div",{className:"oc-chip oc-tools",children:[V.jsx("button",{type:"button",className:"icon-btn",onClick:()=>c(l==="map"?"":"map"),"aria-label":ee("地图（M）"),title:ee("地图（M）"),children:V.jsx(h_,{className:"ico"})}),V.jsx("button",{type:"button",className:`icon-btn${l==="settings"?" is-on":""}`,onClick:()=>c(l==="settings"?"":"settings"),"aria-label":ee("海况和天气"),title:ee("海况和天气"),children:V.jsx(p_,{className:"ico"})}),V.jsx("button",{type:"button",className:"icon-btn",onClick:Z,"aria-label":ee(x?"关掉声音":"打开声音"),title:ee(x?"关掉声音":"打开声音"),children:x?V.jsx(g_,{className:"ico"}):V.jsx(v_,{className:"ico"})}),V.jsx("button",{type:"button",className:`icon-btn${l==="keys"?" is-on":""}`,onClick:()=>c(l==="keys"?"":"keys"),"aria-label":ee("操作方法"),title:ee("操作方法"),children:V.jsx(d_,{className:"ico"})})]})]}),G&&v!==G.isle.id&&l!=="map"&&V.jsx(KC,{isle:G.isle,dist:G.dist,onClose:()=>_(G.isle.id)}),m&&Ne&&l!=="map"&&V.jsx("button",{type:"button",className:"oc-chip oc-goal",onClick:()=>u(null),title:ee("取消航向"),children:ee("开往 {0} · {1}",m.name,ua(re))}),Pe&&V.jsx("div",{className:"oc-chip oc-alert",role:"alert",children:ee("触底了，倒车或转个方向")}),Ne&&V.jsxs("div",{className:"oc-bottom",children:[!b&&V.jsx(qC,{get:F,isles:P,course:m,onOpen:()=>c("map")}),V.jsxs("div",{className:"oc-dash",children:[V.jsx(jC,{heading:Ne.heading,course:q}),V.jsx(YC,{s:r})]})]}),A&&!b&&l===""&&V.jsx("div",{className:"oc-hint",children:ee("W/S 油门 · A/D 转舵 · 拖动看四周 · M 地图")}),b&&r&&e.current&&V.jsx(eb,{controls:e.current.controls}),l==="settings"&&V.jsx(JC,{value:S,title:he,onChange:W,onTitle:D,onStartHere:Se,onGoStart:le,onClose:()=>c("")}),l==="keys"&&V.jsx(QC,{onClose:()=>c("")}),l==="map"&&V.jsx($C,{get:F,isles:P,found:g,course:m,onCourse:u,onClose:()=>c("")}),Vi.note&&V.jsx("div",{className:"note",role:"status",children:Vi.note.text})]})}AS(Vi,nb);
