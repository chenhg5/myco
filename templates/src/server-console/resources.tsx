// Service/container tables expose typed operations, not arbitrary shell text.
import { Play, Square, RotateCw, FileText, Search } from "lucide-react";
import { useState } from "react";
import { t as uiText } from "../_lib/i18n";
import type { Service, Container, Section, Operation } from "./types";
function Controls({
  name,
  busy,
  onAction,
  onLogs,
}: {
  name: string;
  busy: boolean;
  onAction: (action: Operation) => void;
  onLogs: () => void;
}) {
  return (
    <div className="sv-row-actions">
      {(
        [
          ["start", "启动", Play],
          ["stop", "停止", Square],
          ["restart", "重启", RotateCw],
        ] as const
      ).map(([op, label, Icon]) => (
        <button
          key={op}
          aria-label={`${uiText(label)} ${name}`}
          title={uiText(label)}
          disabled={busy}
          onClick={() => onAction(op)}
        >
          <Icon size={15} />
        </button>
      ))}
      <button
        aria-label={`${uiText("日志")} ${name}`}
        title={uiText("日志")}
        disabled={busy}
        onClick={onLogs}
      >
        <FileText size={15} />
      </button>
    </div>
  );
}
export function Resources({
  kind,
  data,
  busy,
  onAction,
  onLogs,
  loading = false,
}: {
  kind: "service" | "container";
  data: Section<Service> | Section<Container>;
  busy: boolean;
  loading?: boolean;
  onAction: (id: string, operation: Operation) => void;
  onLogs: (id: string) => void;
}) {
  const [query, setQuery] = useState("");
  const rows = data.items.filter((row) =>
    Object.values(row).join(" ").toLowerCase().includes(query.toLowerCase()),
  );
  return (
    <section className="sv-card">
      <header>
        <div>
          <h2>{kind === "service" ? uiText("系统服务") : uiText("容器")}</h2>
          <p>{uiText("从 SSH 主机实时读取；更改前会显示目标与具体操作。")}</p>
        </div>
        <label className="sv-search">
          <Search size={16} />
          <input
            aria-label={uiText("筛选资源")}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={uiText("搜索名称或状态")}
          />
        </label>
      </header>
      {loading ? (
        <div className="sv-empty" role="status">
          {uiText("正在读取或执行…")}
        </div>
      ) : !data.available ? (
        <div className="sv-empty">
          <h3>{uiText("此主机暂不支持此功能")}</h3>
          <p>
            {data.reason ||
              uiText("请确认对应组件已安装，且当前 SSH 用户有读取权限。")}
          </p>
        </div>
      ) : (
        <div className="sv-table-wrap">
          <table className="sv-table">
            <thead>
              <tr>
                <th>{uiText("名称")}</th>
                <th>{uiText("状态")}</th>
                <th>
                  {kind === "service" ? uiText("描述") : uiText("镜像 / 端口")}
                </th>
                <th>{uiText("操作")}</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((row) => {
                const service = row as Service,
                  container = row as Container,
                  id = kind === "service" ? service.name : container.id,
                  status =
                    kind === "service" ? service.active : container.state;
                return (
                  <tr key={id}>
                    <td>
                      <strong>{row.name}</strong>
                      <small>
                        {kind === "container"
                          ? container.id.slice(0, 12)
                          : service.sub}
                      </small>
                    </td>
                    <td>
                      <span
                        className={`sv-status ${["active", "running"].includes(status) ? "good" : ""}`}
                      >
                        {status}
                      </span>
                    </td>
                    <td>
                      {kind === "service" ? (
                        service.description
                      ) : (
                        <>
                          <span>{container.image}</span>
                          <small>{container.ports || "—"}</small>
                        </>
                      )}
                    </td>
                    <td>
                      <Controls
                        name={row.name}
                        busy={
                          busy ||
                          (kind === "service" &&
                            !/^[A-Za-z0-9_][A-Za-z0-9_.@:-]{0,159}$/.test(id))
                        }
                        onAction={(op) => onAction(id, op)}
                        onLogs={() => onLogs(id)}
                      />
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
          {!rows.length && (
            <p className="sv-table-empty">
              {query
                ? uiText("没有匹配的资源")
                : uiText("这台主机还没有可展示的资源")}
            </p>
          )}
        </div>
      )}
    </section>
  );
}
