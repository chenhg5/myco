// Presentational overview cards. Values and chart samples must come from SSH,
// never from decorative random data. Native transport is composed by main.tsx.
import {
  Cpu,
  MemoryStick,
  HardDrive,
  Clock,
  Activity,
  Server,
} from "lucide-react";
import { t as uiText } from "../_lib/i18n";
import { bytes, duration, percent, chartPath } from "./format";
import type { Overview, Sample } from "./types";
function Meter({
  title,
  value,
  detail,
  points,
  icon,
}: {
  title: string;
  value: number | null;
  detail: string;
  points: (number | null)[];
  icon: React.ReactNode;
}) {
  return (
    <article className="sv-metric">
      <div className="sv-card-label">
        {icon}
        <span>{title}</span>
        <span className="sv-live-dot" />
      </div>
      <div className="sv-metric-number">
        {value === null ? "—" : value.toFixed(1)}
        <small>{value === null ? "" : "%"}</small>
      </div>
      <p>{detail}</p>
      <svg viewBox="0 0 240 48" preserveAspectRatio="none" aria-hidden="true">
        <path
          d={chartPath(points)}
          fill="none"
          stroke="currentColor"
          strokeWidth="1.5"
          vectorEffect="non-scaling-stroke"
        />
      </svg>
    </article>
  );
}
export function Dashboard({
  data,
  history,
}: {
  data: Overview;
  history: Sample[];
}) {
  const used = data.memory_total - data.memory_available,
    memory = percent(used, data.memory_total),
    disk = data.disks.find((d) => d.mount === "/") || data.disks[0];
  return (
    <>
      <section className="sv-metrics">
        <Meter
          title={uiText("处理器")}
          icon={<Cpu size={17} />}
          value={data.cpu_percent}
          detail={
            data.cpus
              ? uiText(
                  "{0} 核 · 负载 {1}",
                  data.cpus,
                  data.load.map((v) => v.toFixed(2)).join(" / "),
                )
              : uiText(
                  "核心数未知 · 负载 {0}",
                  data.load.map((v) => v.toFixed(2)).join(" / "),
                )
          }
          points={history.map((s) => s.cpu)}
        />
        <Meter
          title={uiText("内存")}
          icon={<MemoryStick size={17} />}
          value={memory}
          detail={`${bytes(used)} / ${bytes(data.memory_total)}`}
          points={history.map((s) => s.memory)}
        />
        <article className="sv-metric">
          <div className="sv-card-label">
            <HardDrive size={17} />
            <span>{uiText("系统磁盘")}</span>
          </div>
          <div className="sv-metric-number">
            {disk ? percent(disk.used, disk.total)?.toFixed(1) || "—" : "—"}
            <small>{disk ? "%" : ""}</small>
          </div>
          <p>
            {disk
              ? `${bytes(disk.used)} / ${bytes(disk.total)}`
              : uiText("暂时无法读取磁盘")}
          </p>
          <div className="sv-storage-track">
            <span
              style={{
                width: `${disk ? percent(disk.used, disk.total) || 0 : 0}%`,
              }}
            />
          </div>
        </article>
        <article className="sv-metric">
          <div className="sv-card-label">
            <Clock size={17} />
            <span>{uiText("运行时间")}</span>
          </div>
          <div className="sv-uptime">{duration(data.uptime)}</div>
          <p>{uiText("从上次启动至今")}</p>
          <div className="sv-system-mark">
            <Activity size={25} />
            <span>LINUX / SSH</span>
          </div>
        </article>
      </section>
      <div className="sv-overview-grid">
        <section className="sv-card">
          <header>
            <h2>{uiText("主机信息")}</h2>
            <Server size={17} />
          </header>
          <dl className="sv-specs">
            {[
              [uiText("主机名"), data.hostname],
              [uiText("操作系统"), data.os],
              [uiText("内核"), data.kernel],
              [uiText("架构"), data.arch],
              [
                uiText("交换空间"),
                `${bytes(data.swap_total - data.swap_free)} / ${bytes(data.swap_total)}`,
              ],
            ].map(([name, value]) => (
              <div key={name}>
                <dt>{name}</dt>
                <dd>{value || "—"}</dd>
              </div>
            ))}
          </dl>
        </section>
        <section className="sv-card">
          <header>
            <h2>{uiText("存储空间")}</h2>
            <span>{data.disks.length}</span>
          </header>
          <div className="sv-disks">
            {data.disks.map((d) => (
              <div key={d.mount}>
                <div>
                  <b>{d.mount}</b>
                  <span>
                    {bytes(d.available)} {uiText("可用")}
                  </span>
                </div>
                <div className="sv-storage-track">
                  <span
                    style={{ width: `${percent(d.used, d.total) || 0}%` }}
                  />
                </div>
                <small>
                  {bytes(d.used)} / {bytes(d.total)}
                </small>
              </div>
            ))}
          </div>
        </section>
      </div>
    </>
  );
}
