import { useEffect, useRef, useState } from "react";
import { appApi, type LocalUpgrade } from "@/appapi";
import { CopyButton } from "@/app/copy";
import { ErrorText } from "@/app/dialogs";
import { E, L } from "@/lib/i18n";

/** The receipt survives reopening the App. A late read cannot undo a retry. */
export function RunnerUpdateNotice() {
 const [status,setStatus]=useState<LocalUpgrade|null>(null);
 const [error,setError]=useState<unknown>(null);
 const [busy,setBusy]=useState(false);
 const [generation,setGeneration]=useState(0);
 const epoch=useRef(0);
 const action=useRef<{ upgrade: boolean; baseline: string }|null>(null);
 const [actionError,setActionError]=useState<unknown>(null);
 useEffect(()=>{
  if(busy)return;
  let gone=false;
  const abort=new AbortController();
  let timer: ReturnType<typeof setTimeout>;
  const read=async()=>{
   const seen=epoch.current;
   try { const next=await appApi.localUpgrade(abort.signal);if(!gone&&seen===epoch.current){setStatus(next);setError(null);
    const pending=action.current;
    const observed=pending?.upgrade?(next.cli?.id||next.cli?.at):next.restart?.id;
    if(pending&&observed&&observed!==pending.baseline){action.current=null;setActionError(null)}} }
   catch(e){if(!gone&&seen===epoch.current)setError(e)}
   finally{if(!gone)timer=setTimeout(read,3000)}
  };
  void read();return()=>{gone=true;abort.abort();clearTimeout(timer)};
 },[generation,busy]);
 const retry=async(upgrade:boolean)=>{
  action.current={upgrade,baseline:upgrade?(status?.cli?.id||status?.cli?.at||""):(status?.restart?.id||"")};
  epoch.current++;setBusy(true);setActionError(null);
  // Stop the old poll before writing; its late response must not replace the result.
  setGeneration(g=>g+1);
  try {setStatus(await (upgrade?appApi.retryLocalUpgrade():appApi.restartLocalWhenIdle()))}
  catch(e){setActionError(e)}
  finally {setBusy(false);setGeneration(g=>g+1)}
 };
 const failed=status?.cli?.state==="failed";
 const upgrading=status?.cli?.state==="running";
 const mismatch=status?.pending;
 const runningVersion=status?.runner?.version || L("未知版本", "an unknown version");
 return <div data-testid="runner-update-notice">
  <div className="app-kv app-about-update">
   <span className="app-kv-label">{L("后台服务", "Background service")}</span>
   <span className="app-kv-value">
    <span className={`app-about-status${failed||status?.error?" bad":""}`}>
     {upgrading?L("正在更新命令行工具…", "Updating the command-line tool…")
      :failed?L("命令行工具升级失败", "Command-line update failed")
      :status?.runner?.state==="restart_unknown"?L("后台服务重启尚未确认", "Background restart not confirmed")
      :mismatch?L(E`runner 正在运行 ${runningVersion}，等待空闲后重启`, E`Runner on ${runningVersion}; restart pending until idle`)
      :status?.runner?`runner ${status.runner.version}`:L("尚未收到运行版本", "Running version not reported yet")}
    </span>
    {failed&&<button type="button" className="app-btn" disabled={busy} onClick={()=>void retry(true)}>{L("重试命令行升级", "Retry command-line update")}</button>}
    {mismatch&&!failed&&!upgrading&&!status?.unsupported&&<button type="button" className="app-btn" disabled={busy||status?.runner?.state==="restarting"||status?.runner?.state==="restart_unknown"} onClick={()=>void retry(false)}>{L("空闲时重启", "Restart when idle")}</button>}
   </span>
  </div>
  {status?.installed&&mismatch&&<p className="app-about-note">{L(E`已安装命令行工具 ${status.installed}。不会中断正在处理的任务。`, E`Command-line tool ${status.installed} is installed. Tasks already in progress will finish first.`)}</p>}
  {status?.expired&&mismatch&&<p className="app-about-note">{L("等待已超过六小时，自动重启已暂停；可再次选择空闲时重启。", "The six-hour wait ended. Automatic restart is paused; you can request another idle restart.")}</p>}
  {status?.unsupported&&mismatch&&<p className="app-about-note">{L("当前旧版 runner 不支持安全的空闲重启。请等所有任务结束后，在终端手动重启一次。", "This older runner cannot safely restart when idle. After all tasks finish, restart it once from a terminal.")} <CopyButton value="myco runner service restart" /></p>}
  {status?.runner?.state==="restart_unknown"&&<p className="app-about-note">{L("重启交接尚未确认，已暂停领取新任务。请检查本机服务后手动重启。", "Restart handoff is unconfirmed; new tasks are paused. Check the local service before restarting manually.")} <CopyButton value="myco runner service restart" /></p>}
  <ErrorText error={actionError || error || status?.cli?.error || status?.error || null} />
 </div>
}
