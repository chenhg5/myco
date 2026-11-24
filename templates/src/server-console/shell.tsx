// Layout-only server workspace. Connection details are provided by the native
// private profile store; the shell never writes them to shared page records.
import type { ReactNode } from "react";
import {
  Server,
  LayoutDashboard,
  Boxes,
  Layers,
  History,
  ShieldCheck,
  Plus,
  RefreshCw,
  Unplug,
  ArrowUpRight,
} from "lucide-react";
import { t as uiText } from "../_lib/i18n";
import type { Profile } from "./types";
export type View =
  | "overview"
  | "services"
  | "containers"
  | "activity"
  | "agent";
const menu = [
  ["overview", "主机概览", LayoutDashboard],
  ["services", "系统服务", Layers],
  ["containers", "容器", Boxes],
  ["activity", "操作记录", History],
  ["agent", "Agent 协作", ShieldCheck],
] as const;
export function Shell({
  profiles,
  selected,
  view,
  connected,
  busy,
  onSelect,
  onView,
  onAdd,
  onRefresh,
  onDisconnect,
  children,
  canAdd = true,
}: {
  canAdd?: boolean;
  profiles: Profile[];
  selected: Profile | null;
  view: View;
  connected: boolean;
  busy: boolean;
  onSelect: (id: string) => void;
  onView: (v: View) => void;
  onAdd: () => void;
  onRefresh: () => void;
  onDisconnect: () => void;
  children: ReactNode;
}) {
  return (
    <div className="sv-app">
      <aside className="sv-sidebar">
        <div className="sv-brand">
          <span>
            <Server size={19} />
          </span>
          <div>
            <strong>{uiText("服务器")}</strong>
            <small>MYCO / INFRASTRUCTURE</small>
          </div>
        </div>
        <div className="sv-sidebar-heading">
          <span>{uiText("SSH 主机")}</span>
          <button
            aria-label={uiText("添加主机")}
            disabled={!canAdd}
            onClick={onAdd}
          >
            <Plus size={16} />
          </button>
        </div>
        <div className="sv-hosts">
          {profiles.map((p) => (
            <button
              key={p.id}
              className={selected?.id === p.id ? "selected" : ""}
              onClick={() => onSelect(p.id)}
            >
              <span
                className={`sv-host-dot ${selected?.id === p.id && connected ? "online" : ""}`}
              />
              <span>
                <b>{p.name}</b>
                <small>{p.target}</small>
              </span>
              {selected?.id === p.id && <ArrowUpRight size={14} />}
            </button>
          ))}
          {!profiles.length && <p>{uiText("把第一台服务器接进来。")}</p>}
        </div>
        <div className="sv-sidebar-rule" />
        <nav>
          {menu.map(([id, label, Icon]) => (
            <button
              key={id}
              className={view === id ? "active" : ""}
              onClick={() => onView(id)}
            >
              <Icon size={17} />
              {uiText(label)}
            </button>
          ))}
        </nav>
        <div className="sv-sidebar-foot">
          <ShieldCheck size={16} />
          <span>
            {uiText("凭证留在本机")}
            <small>{uiText("通过你自己的 SSH 连接")}</small>
          </span>
        </div>
      </aside>
      <section className="sv-workspace">
        <header className="sv-topbar">
          <div>
            <span className="sv-breadcrumb">
              {uiText("基础设施")}
              <i>/</i>
              {selected?.name || uiText("服务器")}
            </span>
            <h1>
              {uiText(menu.find(([id]) => id === view)?.[1] || "主机概览")}
            </h1>
          </div>
          <div className="sv-top-actions">
            <span className={`sv-connection ${connected ? "online" : ""}`}>
              <i />
              {connected ? uiText("已连接") : uiText("未连接")}
            </span>
            <button
              className="sv-button"
              onClick={onRefresh}
              disabled={!selected || busy}
            >
              <RefreshCw size={15} className={busy ? "sv-spin" : ""} />
              {uiText("刷新")}
            </button>
            {connected && (
              <button
                className="sv-icon-button"
                aria-label={uiText("断开连接")}
                onClick={onDisconnect}
              >
                <Unplug size={16} />
              </button>
            )}
          </div>
        </header>
        <main className="sv-content">{children}</main>
      </section>
    </div>
  );
}
