import { useEffect, useState } from "react";
import type { Project } from "@/api";
import { appApi, type Hub } from "@/appapi";
import { ErrorText } from "@/app/dialogs";
import { L, E } from "@/lib/i18n";

const GiB = 1024 ** 3;
const bytes = (n: number) => n >= GiB ? `${(n / GiB).toFixed(2)} GiB`
  : n >= 1024 ** 2 ? `${(n / 1024 ** 2).toFixed(1)} MiB`
  : n >= 1024 ? `${(n / 1024).toFixed(1)} KiB` : `${n} B`;

/** Workspace managers set overrides here; project owners cannot grant capacity. */
export function ProjectStorageSettings({ hub }: { hub: Hub }) {
  const [projects, setProjects] = useState<Project[]>([]);
  const [loading, setLoading] = useState(true);
  const [name, setName] = useState("");
  const [files, setFiles] = useState("");
  const [capacity, setCapacity] = useState("");
  const [saved, setSaved] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);

  useEffect(() => {
    let off = false;
    setLoading(true);
    setError(null);
    void appApi.projects(hub.id).then(r => {
      if (!off) {
        setProjects(r.projects);
        setName(r.projects[0]?.name || "");
        setLoading(false);
      }
    }, e => { if (!off) { setError(e); setLoading(false); } });
    return () => { off = true; };
  }, [hub.id]);

  const project = projects.find(p => p.name === name);
  useEffect(() => {
    setFiles(String(project?.files_max ?? 100));
    setCapacity(String((project?.bytes_max ?? 2 * GiB) / GiB));
  }, [project]);

  const save = async (reset = false) => {
    setBusy(true);
    setError(null);
    setSaved(false);
    try {
      const q = await appApi.setProjectStorage(hub.id, name, reset ? 0 : Number(files), reset ? 0 : Math.round(Number(capacity) * GiB));
      setProjects(ps => ps.map(p => p.name === name ? { ...p, ...q } : p));
      setSaved(true);
    } catch (e) { setError(e); }
    finally { setBusy(false); }
  };
  const valid = Number.isInteger(Number(files)) && Number(files) >= 1 && Number(files) <= 10000
    && Number(capacity) * GiB >= 1 && Number(capacity) <= 20;

  return <section className="app-setting-card" data-setting="project-storage" data-hub={hub.id}>
    <div className="app-setting-head"><div className="titles"><h3>{L("项目文件额度", "Project storage")}</h3><p>{hub.name}</p></div></div>
    {loading ? <p className="app-small-note">{L("正在读取项目…", "Loading projects…")}</p> : projects.length ? <>
      <label className="app-field"><span className="app-field-label">{L("项目", "Project")}</span>
        <select className="app-input" value={name} onChange={e => { setName(e.target.value); setSaved(false); }} disabled={busy}>
          {projects.map(p => <option key={p.name} value={p.name}>{p.title || p.name}</option>)}
        </select>
      </label>
      {project?.files_max ? <>
        <p className="app-small-note">{L(
          E`文件 ${project.files_used ?? 0}/${project.files_max} · ${bytes(project.bytes_used ?? 0)} / ${bytes(project.bytes_max ?? 0)} · 上下文文档 ${project.docs_used ?? 0}/100`,
          E`Files ${project.files_used ?? 0}/${project.files_max} · ${bytes(project.bytes_used ?? 0)} / ${bytes(project.bytes_max ?? 0)} · Context documents ${project.docs_used ?? 0}/100`,
        )}</p>
        <label className="app-field"><span className="app-field-label">{L("文件数上限", "File count limit")}</span>
          <input className="app-input" type="number" min="1" max="10000" step="1" value={files} onChange={e => { setFiles(e.target.value); setSaved(false); }} disabled={busy}/>
        </label>
        <label className="app-field"><span className="app-field-label">{L("项目总大小上限（GiB）", "Total project size limit (GiB)")}</span>
          <input className="app-input" type="number" min="0.000001" max="20" step="any" value={capacity} onChange={e => { setCapacity(e.target.value); setSaved(false); }} disabled={busy}/>
        </label>
        <p className="app-small-note">{L("文档仍限 100 个；单文件不超过 20 MiB。项目文件最多 10000 个、总大小最多 20 GiB。", "Context documents stay limited to 100; each file is capped at 20 MiB. At most 10,000 files and 20 GiB per project.")}</p>
        <div className="app-setting-actions">
          <button type="button" className="app-btn" disabled={busy} onClick={() => void save(true)}>{L("使用工作区默认值", "Use workspace defaults")}</button>
          <button type="button" className="app-btn app-btn-primary" disabled={busy || !valid} onClick={() => void save()}>{busy ? L("正在保存…", "Saving…") : L("保存", "Save")}</button>
        </div>
      </> : <p className="app-small-note">{L("升级 Hub 后可调整项目文件额度。", "Upgrade the Hub to configure project storage.")}</p>}
    </> : !error && <p className="app-small-note">{L("还没有项目。", "No projects yet.")}</p>}
    {saved && <p role="status" className="app-small-note">{L("已保存", "Saved")}</p>}
    <ErrorText error={error}/>
  </section>;
}
