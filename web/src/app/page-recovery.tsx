import { localPreferences } from "@/lib/localPreferences";
import { useEffect, useRef, useState } from "react";
import { appApi } from "@/appapi";
import { Modal } from "@/app/dialogs";
import { L } from "@/lib/i18n";

const templates = ["tp_myco_video", "tp_myco_photo", "tp_myco_motion", "tp_myco_reader"];
export const canRecoverTemplate = (id?: string) => !!id && templates.includes(id);
const storageKey = "myco-page-recovery-completed";
export const recoveryLabel = () => L("恢复旧本机文件", "Recover older local files");

function readCompleted(): string[] {
  try {
    const entries: unknown = JSON.parse(localPreferences.getItem(storageKey) || "[]");
    return Array.isArray(entries) ? entries.filter((value): value is string => typeof value === "string") : [];
  } catch { return []; }
}

type Source = { index: number; template: string; label: string };
type Entry = { id: string; name: string };
type Fallback = { name: string; url: string };

export function PageRecovery({ enabled, template, language, showing, onClose, onAvailable }: {
  enabled: boolean; template?: string; language: string; showing: boolean; onClose: () => void; onAvailable: (sources: string[]) => void;
}) {
  const [address, setAddress] = useState("");
  const [checked, setChecked] = useState(false);
  const [sources, setSources] = useState<Source[]>([]);
  const [selected, setSelected] = useState<number>();
  const [entries, setEntries] = useState<Entry[]>([]);
  const [more, setMore] = useState(false);
  const [saved, setSaved] = useState<Record<string, boolean>>({});
  const [saving, setSaving] = useState("");
  const [fallback, setFallback] = useState<Record<string, Fallback>>({});
  const [error, setError] = useState(false);
  const completed = useRef<string[]>([]);
  const addresses = useRef(new Set<string>());
  const frame = useRef<HTMLIFrameElement>(null);
  const source = sources.find(item => item.index === selected) ?? sources.find(item => item.template === template) ?? sources[0];
  const sourceIndex = showing ? source?.index : undefined;

  useEffect(() => {
    if (!enabled || address) return;
    let alive = true;
    void appApi.openPageStorageRecovery().then(({ url }) => {
      if (alive) setAddress(url);
    }).catch(() => { /* Recovery must not block opening the app. */ });
    return () => { alive = false; };
  }, [enabled, address]);

  const send = (type: string, value: object = {}) => {
    if (address) frame.current?.contentWindow?.postMessage({ type, ...value }, new URL(address).origin);
  };

  useEffect(() => {
    if (showing) return;
    setSelected(undefined);
    setEntries([]);
    setSaved({});
    setSaving("");
    setError(false);
    setFallback({});
    for (const url of addresses.current) URL.revokeObjectURL(url);
    addresses.current.clear();
  }, [showing]);
  useEffect(() => () => {
    for (const url of addresses.current) URL.revokeObjectURL(url);
  }, []);

  useEffect(() => {
    if (!address) return;
    const origin = new URL(address).origin;
    const receive = (event: MessageEvent) => {
      if (event.source !== frame.current?.contentWindow || event.origin !== origin || !event.data) return;
      const data = event.data;
      if (data.type === "myco-recovery-sources" && Array.isArray(data.sources)) {
        const available: Source[] = data.sources.filter((item: Source) => canRecoverTemplate(item.template));
        setChecked(true);
        setSources(available);
        onAvailable(available.map(item => item.template));
        if (showing && !available.length) onClose();
      }
      if (showing && data.type === "myco-recovery-rows" && data.index === sourceIndex) {
        setEntries(current => data.append ? [...current, ...data.entries] : data.entries);
        setMore(data.more);
      }
      if (showing && data.type === "myco-recovery-error") {
        setError(true);
        setSaving("");
      }
      if (showing && data.type === "myco-recovery-file" && data.file instanceof Blob && typeof data.name === "string" && data.id === saving) {
        const { id, file, name } = data;
        void appApi.pageDownload(name, file, file.type).then(() => {
          setSaved(current => ({ ...current, [id]: true }));
        }, () => {
          // Some originals are outside the general download handler's limits.
          const url = URL.createObjectURL(file);
          addresses.current.add(url);
          setFallback(current => ({ ...current, [id]: { name, url } }));
        }).finally(() => setSaving(""));
      }
    };
    window.addEventListener("message", receive);
    return () => window.removeEventListener("message", receive);
  }, [address, sourceIndex, showing, saving, onClose, onAvailable]);

  useEffect(() => {
    if (sourceIndex === undefined) return;
    setEntries([]);
    setMore(false);
    setError(false);
    send("myco-recovery-list", { index: sourceIndex });
  }, [address, sourceIndex]);

  const confirm = (id: string) => {
    completed.current = [...new Set([...readCompleted(), ...completed.current, id])];
    try { localPreferences.setItem(storageKey, JSON.stringify(completed.current)); } catch { /* Keep progress in this panel. */ }
    setEntries(current => current.filter(item => item.id !== id));
    send("myco-recovery-complete", { id, index: sourceIndex });
  };
  const probe = address ? new URL(address) : undefined;
  probe?.searchParams.set("myco_lang", language);
  probe?.searchParams.set("probe", "1");
  return <>
    {probe && <iframe ref={frame} hidden title="Recovery inventory" data-checked={checked} sandbox="allow-scripts allow-same-origin" src={probe.href} referrerPolicy="no-referrer"
      onLoad={() => send("myco-recovery-configure", { completed: [...new Set([...readCompleted(), ...completed.current])] })} />}
    <Modal open={showing} onOpenChange={(open) => { if (!open) onClose(); }} title={recoveryLabel()} className="pg-recovery-dialog" wide>
      <p>{L("保存副本后在应用中导入。旧库里的原件会保留。", "Save a copy and import it into the app. The older original stays in place.")}</p>
      {source && <label className="pg-recovery-source">
        {L("应用", "Application")}
        <select className="app-input" value={source.index} onChange={(event) => setSelected(Number(event.target.value))}>
          {sources.map(item => <option key={item.index} value={item.index}>{item.label}</option>)}
        </select>
      </label>}
      {error && <p role="alert">{L("无法读取旧库，请关闭后重试。", "Could not read older storage. Close this dialog and try again.")}</p>}
      <ul className="pg-recovery-files">
        {entries.map(item => <li key={item.id}>
          <span>{item.name}</span>
          {saved[item.id] ? <button type="button" className="app-btn" onClick={() => confirm(item.id)}>{L("标记已恢复", "Mark recovered")}</button>
            : fallback[item.id] ? <a className="app-btn" href={fallback[item.id].url} download={fallback[item.id].name}
              onClick={() => setSaved(current => ({ ...current, [item.id]: true }))}>{L("下载原件", "Download original")}</a>
            : <button type="button" className="app-btn" disabled={!!saving} onClick={() => {
              setSaving(item.id); setError(false); send("myco-recovery-save", { index: sourceIndex, id: item.id });
            }}>{saving === item.id ? L("正在保存…", "Saving…") : L("保存副本", "Save copy")}</button>}
        </li>)}
      </ul>
      {more && <button type="button" className="app-btn" onClick={() => send("myco-recovery-list", { index: sourceIndex, append: true })}>{L("显示更多", "Show more")}</button>}
      <div className="app-modal-actions"><button type="button" className="app-btn" onClick={onClose}>{L("关闭", "Close")}</button></div>
    </Modal>
  </>;
}
