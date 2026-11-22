import { t } from "../_lib/i18n";
import { Plus, Trash2 } from "lucide-react";
import { addOverlay, patch, removeOverlay } from "./edit";
import { overlayList, overlays, total } from "./model";
import { player } from "./player";
import type { Sel } from "./timeline";

export function FocusPanel({setSel}: {setSel: (s: Sel) => void}) {
 return <div className="pane"><h3>{t("重点放大")}</h3><p className="hint">{t("在要强调的位置添加放大，拖动画面中的方框选择焦点。预览和导出都会平滑放大再还原。")}</p>
 <button className="btn" disabled={!total()} onClick={() => {
 const start = Math.min(player.time, Math.max(0, total() - .1));
 const id = addOverlay({type:"zoom",start,end:Math.min(total(),start+4),x:.375,y:.375,w:.25,h:.25,strength:.5,fade_in:.9,fade_out:.9,lane:2});
 player.pause();player.seek(start+.05);setSel({kind:"overlay",id});
 }}><Plus className="ico" />{t("添加重点放大")}</button>
 {overlayList().filter(o=>o.type==="zoom").map(o=><button className="btn focus-item" key={o.id} onClick={()=>{player.seek((o.start+o.end)/2);setSel({kind:"overlay",id:o.id});}}>{t("重点放大")} · {o.start.toFixed(1)}–{o.end.toFixed(1)}s</button>)}</div>;
}
export function FocusProps({id}:{id:string}) {
 const o=overlayList().find(o=>o.id===id)!;
 const set=(p:Record<string,unknown>)=>patch(overlays,id,p,`focus-${id}`);
 return <div className="pane props"><h3>{t("重点放大")}</h3>
 <p className="hint">{t("拖动画面中的方框选择焦点")}</p>
 <label className="focus-field">{t("放大倍率")} · {(2+o.strength*2).toFixed(1)}×<input aria-label={t("放大倍率")} type="range" min="0" max="1" step=".05" value={o.strength} onChange={e=>set({strength:+e.target.value})}/></label>
 <p className="hint">{t("缓慢进入，停留，再平滑还原；短片段会自动缩短过渡。")}</p>
 <label className="focus-field">{t("放大过渡（秒）")}<input className="inp" aria-label={t("放大过渡（秒）")} type="number" min=".1" max="3" step=".1" value={o.fade_in} onChange={e=>set({fade_in:Math.max(.1,Math.min(3,+e.target.value||.9))})}/></label>
 <label className="focus-field">{t("还原过渡（秒）")}<input className="inp" aria-label={t("还原过渡（秒）")} type="number" min=".1" max="3" step=".1" value={o.fade_out} onChange={e=>set({fade_out:Math.max(.1,Math.min(3,+e.target.value||.9))})}/></label>
 <label className="focus-field">{t("开始（秒）")}<input className="inp" type="number" min="0" step=".1" value={o.start} onChange={e=>set({start:Math.max(0,Math.min(+e.target.value,o.end-.1))})}/></label>
 <label className="focus-field">{t("结束（秒）")}<input className="inp" type="number" min={o.start+.1} max={total()} step=".1" value={o.end} onChange={e=>set({end:Math.max(o.start+.1,Math.min(total(),+e.target.value))})}/></label>
 <button className="btn" onClick={()=>removeOverlay(id)}><Trash2 className="ico"/>{t("删除")}</button></div>;
}
