import "../_lib/sheet.css";
import "./proxy.css";
import { useEffect, useRef, useState } from "react";
import {
  Activity,
  ArrowDown,
  ArrowUp,
  Check,
  ChevronRight,
  CircleHelp,
  Cable,
  Globe2,
  Layers2,
  List,
  LoaderCircle,
  LockKeyhole,
  Network,
  Search,
  Settings2,
  ShieldCheck,
  Unplug,
  X,
} from "lucide-react";
import { myco } from "../_lib/myco";
import { Kit, mount } from "../_lib/appkit";
import {
  bytes,
  rate,
  demoProfile,
  demoSnapshot,
  demoSamples,
  type Profile,
  type Snapshot,
  type Sample,
} from "./model";
const kit = new Kit([]);
const tabs = [
  { id: "overview", label: "概览", icon: Activity },
  { id: "proxies", label: "策略与节点", icon: Layers2 },
  { id: "connections", label: "连接", icon: Network },
  { id: "config", label: "运行配置", icon: Settings2 },
  { id: "log", label: "操作记录", icon: List },
];
const modeLabels: Record<string, string> = {
  rule: "规则",
  global: "全局",
  direct: "直连",
};
const call = <T,>(name: string, args: object = {}) => {
  if (!myco?.os)
    return Promise.reject(new Error("请更新 myco 桌面 App 后使用本机控制接口"));
  return myco.os.call<T>(name, args, { timeout: 15000 });
};
const errorText = (e: unknown) => {
  const message =
    e instanceof Error ? e.message : String((e as any)?.message || e);
  return /no such capability|keychain is unavailable/.test(message)
    ? "此 App 尚未提供代理控制接口或本机钥匙串，请更新桌面 App"
    : message;
};
function Chart({ samples }: { samples: Sample[] }) {
  const max = Math.max(1024, ...samples.flatMap((s) => [s.down, s.up])),
    w = 760,
    h = 160;
  const points = (key: "down" | "up") =>
    samples
      .map(
        (s, i) =>
          `${(i * w) / Math.max(1, samples.length - 1)},${h - 12 - (s[key] / max) * (h - 28)}`,
      )
      .join(" ");
  return (
    <div className="chart-wrap">
      <div className="chart-axis">
        <span>{bytes(max)}/s</span>
        <span>{bytes(max / 2)}/s</span>
        <span>0</span>
      </div>
      <svg
        viewBox={`0 0 ${w} ${h}`}
        role="img"
        aria-label="最近 90 秒的上下行速率"
        preserveAspectRatio="none"
      >
        <defs>
          <linearGradient id="shade" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0" stopColor="currentColor" stopOpacity=".13" />
            <stop offset="1" stopColor="currentColor" stopOpacity="0" />
          </linearGradient>
        </defs>
        {[20, 84, 148].map((y) => (
          <line key={y} x1="0" x2={w} y1={y} y2={y} className="gridline" />
        ))}
        {samples.length > 1 && (
          <>
            <polygon
              points={`0,${h} ${points("down")} ${w},${h}`}
              fill="url(#shade)"
            />
            <polyline
              points={points("down")}
              fill="none"
              stroke="currentColor"
              strokeWidth="1.9"
            />
            <polyline
              points={points("up")}
              fill="none"
              stroke="currentColor"
              strokeOpacity=".4"
              strokeWidth="1.4"
              strokeDasharray="5 4"
            />
          </>
        )}
      </svg>
      {samples.length < 2 && <span className="chart-empty">等待流量采样…</span>}
    </div>
  );
}
function App() {
  const [tab, setTab] = useState("overview"),
    [profile, setProfile] = useState<Profile>(),
    [snapshot, setSnapshot] = useState<Snapshot>(),
    [samples, setSamples] = useState<Sample[]>([]),
    [demo, setDemo] = useState(false),
    [loading, setLoading] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [checked, setChecked] = useState(false),
    [settings, setSettings] = useState(false),
    [filter, setFilter] = useState(""),
    [note, setNote] = useState(""),
    [logs, setLogs] = useState<{ at: string; text: string }[]>([]);
  const [endpoint, setEndpoint] = useState("http://127.0.0.1:9090"),
    [engine, setEngine] = useState<Profile["engine"]>("mihomo"),
    [secret, setSecret] = useState("");
  const prev = useRef<Snapshot>(),
    generation = useRef(0),
    polling = useRef(false),
    mounted = useRef(true);
  const record = (text: string) =>
    setLogs((l) =>
      [
        { at: new Date().toLocaleTimeString("zh-CN", { hour12: false }), text },
        ...l,
      ].slice(0, 100),
    );
  useEffect(() => {
    mounted.current = true;
    call<Profile>("proxy.profile", { action: "get" })
      .then((p) => {
        if (mounted.current) setProfile(p);
      })
      .catch((e) => {
        if ((e as any)?.code !== "not_found" && mounted.current)
          setError(errorText(e));
      })
      .finally(() => {
        if (mounted.current) setChecked(true);
      });
    return () => {
      mounted.current = false;
      generation.current++;
    };
  }, []);
  useEffect(() => {
    if (!profile || demo) return;
    let stopped = false;
    const gen = ++generation.current;
    async function tick() {
      if (stopped || polling.current) return;
      polling.current = true;
      setLoading(true);
      try {
        const s = await call<Snapshot>("proxy.control", { action: "snapshot" });
        if (stopped || gen !== generation.current) return;
        const point = rate(prev.current, s);
        prev.current = s;
        setSnapshot(s);
        setError("");
        if (point) setSamples((a) => [...a, point].slice(-45));
      } catch (e) {
        if (!stopped && gen === generation.current) {
          setError(errorText(e));
          prev.current = undefined;
          setSamples([]);
        }
      } finally {
        polling.current = false;
        if (!stopped) setLoading(false);
      }
    }
    void tick();
    const timer = window.setInterval(() => {
      if (!document.hidden) void tick();
    }, 2000);
    return () => {
      stopped = true;
      generation.current++;
      window.clearInterval(timer);
    };
  }, [profile, demo]);
  const addDemo = () => {
    generation.current++;
    setDemo(true);
    setProfile(demoProfile);
    setSnapshot(demoSnapshot);
    setSamples(demoSamples);
    setError("");
    setLoading(false);
    record("打开示例预览 · 不连接内核，不改本机配置");
  };
  const openSettings = () => {
    setEndpoint(
      demo
        ? "http://127.0.0.1:9090"
        : profile?.endpoint || "http://127.0.0.1:9090",
    );
    setEngine(profile?.engine || "mihomo");
    setSecret("");
    setSettings(true);
  };
  async function saveProfile(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setNote("");
    try {
      const p = await call<Profile>("proxy.profile", {
        action: "save",
        endpoint,
        engine,
        ...(secret !== "" || !profile || demo ? { secret } : {}),
      });
      generation.current++;
      setSecret("");
      prev.current = undefined;
      setSnapshot(undefined);
      setSamples([]);
      setDemo(false);
      setProfile(p);
      setError("");
      setSettings(false);
      record("已保存本机控制连接");
    } catch (e) {
      setNote(errorText(e));
    } finally {
      setBusy(false);
    }
  }
  async function action(args: object, label: string) {
    if (demo) {
      setNote("这是示例预览。连接本机内核后才能执行操作。");
      return;
    }
    setBusy(true);
    setNote("");
    try {
      const result = await call<{ delay?: number }>("proxy.control", args);
      record(
        label + (result?.delay !== undefined ? ` · ${result.delay} ms` : ""),
      );
      const s = await call<Snapshot>("proxy.control", { action: "snapshot" });
      setSnapshot(s);
      setError("");
    } catch (e) {
      setNote(errorText(e));
      record(label + " · 未完成");
    } finally {
      setBusy(false);
    }
  }
  async function forget() {
    setBusy(true);
    try {
      if (!demo) await call("proxy.profile", { action: "forget" });
      generation.current++;
      setProfile(undefined);
      setSnapshot(undefined);
      prev.current = undefined;
      setDemo(false);
      setSamples([]);
      setSettings(false);
      setSecret("");
      setError("");
      record("已断开控制连接 · 内核继续运行");
    } catch (e) {
      setNote(errorText(e));
    } finally {
      setBusy(false);
    }
  }
  const online = !!snapshot && !error,
    validTraffic =
      online && !snapshot?.warnings.some((w) => w.startsWith("/connections")),
    nodes = Object.entries(snapshot?.proxies || {}),
    groups = nodes.filter(([, n]) => n.all?.length),
    connections = snapshot?.traffic.connections || [],
    current = samples.at(-1),
    title = tabs.find((t) => t.id === tab)?.label;
  return (
    <div className="proxy-app">
      <aside className="rail">
        <div className="brand">
          <div className="brand-mark">
            <Cable size={21} />
          </div>
          <span>
            代理工作台<small>NETWORK CONSOLE</small>
          </span>
        </div>
        <div className="rail-label">工作空间</div>
        <nav>
          {tabs.map((t) => (
            <button
              key={t.id}
              aria-label={t.label}
              className={tab === t.id ? "active" : ""}
              onClick={() => setTab(t.id)}
            >
              <t.icon size={17} />
              <span>{t.label}</span>
              {t.id === "connections" && validTraffic && (
                <em>{connections.length}</em>
              )}
            </button>
          ))}
        </nav>
        <div className="rail-bottom">
          <ShieldCheck size={17} />
          <div>
            控制信息仅存本机<small>不与工作区共享</small>
          </div>
        </div>
      </aside>
      <div className="proxy-body">
        <header className="proxy-top">
          <div className="crumb">
            工作台 <ChevronRight size={13} />
            <strong>{title}</strong>
          </div>
          <div className="top-actions">
            {demo && <span className="badge">示例预览</span>}
            <span className="connection-state">
              <i className={online ? "on" : ""} />
              {demo
                ? "演示数据"
                : error
                  ? "连接中断"
                  : online
                    ? "已连接"
                    : loading
                      ? "正在连接"
                      : "未连接"}
            </span>
            <button
              className="icon-btn"
              aria-label="连接设置"
              onClick={openSettings}
            >
              <Settings2 size={17} />
            </button>
          </div>
        </header>
        <main className="proxy-main">
          <div className="page-heading">
            <div>
              <div className="eyebrow">
                {tab === "overview"
                  ? "YOUR NETWORK, AT A GLANCE"
                  : "NETWORK CONTROL"}
              </div>
              <h1>{title}</h1>
              <p>
                {tab === "overview"
                  ? "看清每一条连接，让网络按你的规则运行。"
                  : tab === "proxies"
                    ? "管理出口策略，手动选择节点或交给内核自动选择。"
                    : tab === "connections"
                      ? "查看当前流量经过哪里，按需断开单条连接。"
                      : tab === "config"
                        ? "只读检查当前配置；切换模式在概览中完成。"
                        : "这次打开工作台后的操作记录，仅在当前页面保留。"}
              </p>
            </div>
            <button className="btn" onClick={openSettings}>
              <Cable size={15} />
              {profile ? "管理连接" : "连接内核"}
            </button>
          </div>
          {note && (
            <div role="status" className="notice">
              {note}
              <button aria-label="关闭提示" onClick={() => setNote("")}>
                <X size={14} />
              </button>
            </div>
          )}
          {error && (
            <div role="alert" className="notice error">
              {error}。<button onClick={openSettings}>检查连接</button>
              {snapshot && <span>下方保留上次数据，非实时。</span>}
            </div>
          )}
          {demo && (
            <div className="demo-banner">
              <CircleHelp size={15} />
              示例预览 · 节点、连接和流量均为演示数据，不代表你的网络。
              <button onClick={openSettings}>
                接入真实内核 <ChevronRight size={13} />
              </button>
            </div>
          )}
          {!profile ? (
            <section className="empty-connect">
              <div className="empty-symbol">
                <Cable size={32} />
              </div>
              <h2>连接你正在使用的代理内核</h2>
              <p>
                支持 Mihomo 与启用了 Clash API 的 sing-box。
                <br />
                输入本机控制地址和密钥，即可查看并管理网络。
              </p>
              <div>
                <button className="btn btn-primary" onClick={openSettings}>
                  连接本机内核
                </button>
                <button className="btn" onClick={addDemo}>
                  先看示例
                </button>
              </div>
              <small>
                {checked
                  ? "管理已有内核，不会自动安装或启动代理服务。"
                  : "正在检查本机连接…"}
              </small>
            </section>
          ) : (
            <>
              {tab === "overview" && (
                <>
                  <section className="metrics">
                    {[
                      {
                        name: "下载速率",
                        value:
                          validTraffic && current ? bytes(current.down) : "—",
                        unit: "/s",
                        icon: ArrowDown,
                        sub: validTraffic
                          ? `累计 ${bytes(snapshot!.traffic.downloadTotal)}`
                          : "等待流量采样",
                      },
                      {
                        name: "上传速率",
                        value:
                          validTraffic && current ? bytes(current.up) : "—",
                        unit: "/s",
                        icon: ArrowUp,
                        sub: validTraffic
                          ? `累计 ${bytes(snapshot!.traffic.uploadTotal)}`
                          : "等待流量采样",
                      },
                      {
                        name: "当前连接",
                        value: validTraffic ? String(connections.length) : "—",
                        unit: "",
                        icon: Network,
                        sub: "活跃的网络连接",
                      },
                      {
                        name: "代理节点",
                        value: online
                          ? String(
                              nodes.filter(([, n]) => !n.all?.length).length,
                            )
                          : "—",
                        unit: "",
                        icon: Globe2,
                        sub: `${groups.length} 个策略组`,
                      },
                    ].map((m) => (
                      <div className="metric" key={m.name}>
                        <div>
                          {m.name}
                          <m.icon size={15} />
                        </div>
                        <strong>
                          {m.value}
                          <small>{m.unit}</small>
                        </strong>
                        <span>{m.sub}</span>
                      </div>
                    ))}
                  </section>
                  <section className="surface traffic">
                    <div className="section-head">
                      <h2>
                        实时流量{" "}
                        <span>{demo ? "示例曲线" : "约每 2 秒采样"}</span>
                      </h2>
                      <div className="legend">
                        <span>━ 下载</span>
                        <span>┄ 上传</span>
                      </div>
                    </div>
                    <Chart samples={samples} />
                    <div className="chart-footer">
                      <span>约 90 秒前</span>
                      <span>{loading && !demo ? "更新中…" : "现在"}</span>
                    </div>
                  </section>
                  <div className="overview-bottom">
                    <section className="surface">
                      <div className="section-head">
                        <h2>出站策略</h2>
                        <button onClick={() => setTab("proxies")}>
                          全部策略 <ChevronRight size={13} />
                        </button>
                      </div>
                      {groups.length ? (
                        groups.slice(0, 3).map(([id, n]) => (
                          <button
                            className="strategy-row"
                            key={id}
                            onClick={() => setTab("proxies")}
                          >
                            <div className="row-icon">
                              <Layers2 size={17} />
                            </div>
                            <div>
                              <strong>{id}</strong>
                              <small>
                                {n.type === "Selector"
                                  ? "手动选择"
                                  : "内核自动管理"}
                              </small>
                            </div>
                            <span>{n.now || "—"}</span>
                            <ChevronRight size={14} />
                          </button>
                        ))
                      ) : (
                        <p className="muted padded">
                          暂无策略组，或该内核未提供列表。
                        </p>
                      )}
                    </section>
                    <section className="surface">
                      <div className="section-head">
                        <h2>运行模式</h2>
                        <ShieldCheck size={16} />
                      </div>
                      <div className="mode-switch">
                        {Object.entries(modeLabels).map(([v, l]) => (
                          <button
                            disabled={
                              busy ||
                              (!!snapshot?.config["mode-list"]?.length &&
                                !snapshot.config["mode-list"].some(
                                  (m) => m.toLowerCase() === v,
                                )) ||
                              !online ||
                              demo ||
                              snapshot?.warnings.some((w) =>
                                w.startsWith("/configs"),
                              )
                            }
                            key={v}
                            className={
                              snapshot?.config.mode?.toLowerCase() === v
                                ? "selected"
                                : ""
                            }
                            onClick={() =>
                              action(
                                { action: "mode", mode: v },
                                `切换为${l}模式`,
                              )
                            }
                          >
                            {l}
                          </button>
                        ))}
                      </div>
                      <p className="mode-description">
                        {snapshot?.config.mode?.toLowerCase() === "rule"
                          ? "按内核现有规则选择代理或直连。"
                          : snapshot?.config.mode?.toLowerCase() === "global"
                            ? "所有流量使用全局策略。"
                            : "以当前内核返回的模式为准。"}
                      </p>
                      <div className="local-foot">
                        <LockKeyhole size={13} />
                        密钥由本机 App 保管
                      </div>
                    </section>
                  </div>
                </>
              )}
              {tab === "proxies" && (
                <>
                  <div className="section-head flat">
                    <h2>{groups.length} 个策略组</h2>
                    <span className="muted">只有手动选择组可以切换</span>
                  </div>
                  {groups.map(([id, n]) => (
                    <section key={id} className="surface proxy-group">
                      <div className="section-head">
                        <h2>
                          {id}
                          <span>{n.type}</span>
                        </h2>
                        <span className="muted">当前：{n.now || "—"}</span>
                      </div>
                      <div className="node-grid">
                        {n.all!.map((name) => {
                          const node = snapshot?.proxies?.[name],
                            d = node?.history?.at(-1)?.delay;
                          return (
                            <div
                              className={`node ${n.now === name ? "selected" : ""}`}
                              key={name}
                            >
                              <button
                                className="node-select"
                                disabled={
                                  busy ||
                                  !online ||
                                  demo ||
                                  n.type !== "Selector"
                                }
                                onClick={() =>
                                  action(
                                    { action: "select", group: id, name },
                                    `策略 ${id} → ${name}`,
                                  )
                                }
                              >
                                <div>
                                  <Globe2 size={16} />
                                  <strong>{name}</strong>
                                  {n.now === name && <Check size={15} />}
                                </div>
                                <small>
                                  {node?.type || "Proxy"} ·{" "}
                                  {d ? `${d} ms` : "未测速"}
                                </small>
                              </button>
                              <button
                                className="tool delay"
                                disabled={busy || !online || demo}
                                onClick={() =>
                                  action(
                                    { action: "delay", name },
                                    `测速 ${name}`,
                                  )
                                }
                              >
                                测速
                              </button>
                            </div>
                          );
                        })}
                      </div>
                    </section>
                  ))}
                  {!groups.length && (
                    <p className="muted">
                      暂无策略组，检查内核配置或连接状态。
                    </p>
                  )}
                </>
              )}
              {tab === "connections" && (
                <section className="surface">
                  <div className="section-head">
                    <h2>
                      活跃连接{" "}
                      <span>
                        {validTraffic ? connections.length : "数据不可用"}
                      </span>
                    </h2>
                    <label className="search">
                      <Search size={14} />
                      <input
                        aria-label="搜索连接"
                        placeholder="搜索域名、进程、策略"
                        value={filter}
                        onChange={(e) => setFilter(e.target.value)}
                      />
                    </label>
                  </div>
                  <div className="connection-table">
                    <table>
                      <thead>
                        <tr>
                          <th>目标 / 进程</th>
                          <th>策略链</th>
                          <th>下载 / 上传</th>
                          <th>操作</th>
                        </tr>
                      </thead>
                      <tbody>
                        {connections
                          .filter((c) =>
                            [
                              c.metadata.host,
                              c.metadata.destinationIP,
                              c.metadata.process,
                              ...c.chains,
                            ]
                              .join(" ")
                              .toLowerCase()
                              .includes(filter.toLowerCase()),
                          )
                          .map((c) => (
                            <tr key={c.id}>
                              <td>
                                <strong>
                                  {c.metadata.host ||
                                    c.metadata.destinationIP ||
                                    "未知目标"}
                                </strong>
                                <small>
                                  {c.metadata.process || "未知进程"} ·{" "}
                                  {c.metadata.network.toUpperCase()}:
                                  {c.metadata.destinationPort}
                                </small>
                              </td>
                              <td>
                                {c.chains.join(" › ") || "—"}
                                <small>{c.rule}</small>
                              </td>
                              <td>
                                {bytes(c.download)}
                                <small>↑ {bytes(c.upload)}</small>
                              </td>
                              <td>
                                <button
                                  className="tool"
                                  disabled={busy || !online || demo}
                                  onClick={() =>
                                    action(
                                      { action: "close", id: c.id },
                                      `断开连接 ${c.metadata.host || c.metadata.destinationIP}`,
                                    )
                                  }
                                >
                                  断开
                                </button>
                              </td>
                            </tr>
                          ))}
                      </tbody>
                    </table>
                    {!connections.length && (
                      <div className="table-empty">
                        {validTraffic ? "当前没有活跃连接" : "等待连接数据"}
                      </div>
                    )}
                  </div>
                </section>
              )}
              {tab === "config" && (
                <section className="surface config">
                  <div className="section-head">
                    <h2>内核状态</h2>
                    <span className="badge">只读摘要</span>
                  </div>
                  {[
                    [
                      "内核",
                      `${profile.engine} · ${snapshot?.version || "等待响应"}`,
                    ],
                    ["控制地址", profile.endpoint],
                    [
                      "控制密钥",
                      profile.has_secret
                        ? "已保存在本机（不会显示）"
                        : "未设置",
                    ],
                    [
                      "运行模式",
                      modeLabels[snapshot?.config.mode.toLowerCase() || ""] ||
                        snapshot?.config.mode ||
                        "—",
                    ],
                    ["混合端口", snapshot?.config["mixed-port"] || "未提供"],
                    [
                      "HTTP / SOCKS 端口",
                      `${snapshot?.config.port || "—"} / ${snapshot?.config["socks-port"] || "—"}`,
                    ],
                    [
                      "允许局域网",
                      snapshot?.warnings.some((w) => w.startsWith("/configs"))
                        ? "未提供"
                        : snapshot?.config["allow-lan"] === undefined
                          ? "未提供"
                          : snapshot.config["allow-lan"]
                            ? "已开启"
                            : "未开启",
                    ],
                    [
                      "TUN",
                      snapshot?.warnings.some((w) => w.startsWith("/configs"))
                        ? "未提供"
                        : snapshot?.config.tun?.enable
                          ? "内核报告已开启"
                          : "未开启 / 内核未提供",
                    ],
                  ].map(([k, v]) => (
                    <div className="config-row" key={k}>
                      <span>{k}</span>
                      <strong>{v}</strong>
                    </div>
                  ))}
                  <p className="padded muted">
                    工作台管理已有内核，不会修改系统代理、DNS、TUN
                    或配置文件。系统流量是否经过代理，取决于你已使用的代理客户端。
                  </p>
                </section>
              )}
              {tab === "log" && (
                <section className="surface">
                  <div className="section-head">
                    <h2>
                      本次操作 <span>最多 100 条</span>
                    </h2>
                    <button onClick={() => setLogs([])}>清空</button>
                  </div>
                  {logs.length ? (
                    logs.map((l, i) => (
                      <div className="log-row" key={i}>
                        <time>{l.at}</time>
                        <span>{l.text}</span>
                      </div>
                    ))
                  ) : (
                    <p className="padded muted">
                      还没有操作记录。这里不包含内核原始日志。
                    </p>
                  )}
                </section>
              )}
              {snapshot?.warnings.map((w) => (
                <div className="notice" key={w}>
                  {w}
                </div>
              ))}
              <footer className="page-footer">
                <span>
                  <i className={online ? "on" : ""} />
                  {profile.engine} {snapshot?.version || ""} ·{" "}
                  {profile.endpoint}
                </span>
                <span>
                  {demo
                    ? "示例预览"
                    : snapshot
                      ? `上次更新 ${new Date(snapshot.at).toLocaleTimeString("zh-CN", { hour12: false })}`
                      : "等待内核响应"}
                </span>
              </footer>
            </>
          )}
        </main>
      </div>
      {settings && (
        <div
          className="modal-shade"
          onClick={(e) => {
            if (e.target === e.currentTarget && !busy) {
              setSettings(false);
              setSecret("");
            }
          }}
        >
          <section
            role="dialog"
            aria-modal="true"
            aria-labelledby="connect-title"
            onKeyDown={(e) => {
              if (e.key === "Escape" && !busy) {
                setSettings(false);
                setSecret("");
              }
              if (e.key === "Tab") {
                const items = Array.from(
                  e.currentTarget.querySelectorAll<HTMLElement>(
                    "button:not(:disabled),input,select",
                  ),
                );
                const first = items[0],
                  last = items.at(-1);
                if (e.shiftKey && document.activeElement === first) {
                  e.preventDefault();
                  last?.focus();
                } else if (!e.shiftKey && document.activeElement === last) {
                  e.preventDefault();
                  first?.focus();
                }
              }
            }}
            className="connect-modal"
          >
            <div className="section-head">
              <h2 id="connect-title">连接本机内核</h2>
              <button
                aria-label="关闭连接设置"
                disabled={busy}
                onClick={() => {
                  setSettings(false);
                  setSecret("");
                }}
              >
                <X size={18} />
              </button>
            </div>
            <p>
              先在已有代理客户端中启用 Clash API，控制地址需使用本机回环 IP。
            </p>
            <form onSubmit={saveProfile}>
              <label>
                代理内核
                <select
                  value={engine}
                  onChange={(e) =>
                    setEngine(e.target.value as Profile["engine"])
                  }
                >
                  <option value="mihomo">Mihomo</option>
                  <option value="sing-box">sing-box（Clash API）</option>
                </select>
              </label>
              <label>
                控制地址
                <input
                  autoFocus
                  required
                  type="url"
                  value={endpoint}
                  placeholder="http://127.0.0.1:9090"
                  onChange={(e) => setEndpoint(e.target.value)}
                />
              </label>
              <label>
                控制密钥
                <input
                  type="password"
                  autoComplete="new-password"
                  value={secret}
                  placeholder={
                    profile?.has_secret && !demo
                      ? "留空保留已有密钥；换地址时自动清除"
                      : "内核未设密钥时可留空"
                  }
                  onChange={(e) => setSecret(e.target.value)}
                />
              </label>
              <small>
                <LockKeyhole size={13} />
                凭据按此页面存于本机 App，不写入工作区。Linux 无系统钥匙串时遵循
                App 的本机凭据文件设置。
              </small>
              {note && (
                <p role="alert" className="form-error">
                  {note}
                </p>
              )}
              <div className="modal-actions">
                {profile && (
                  <button
                    type="button"
                    className="tool"
                    disabled={busy}
                    onClick={forget}
                  >
                    <Unplug size={14} />
                    断开并忘记
                  </button>
                )}
                <span />
                <button
                  type="button"
                  className="btn"
                  disabled={busy}
                  onClick={() => {
                    setSettings(false);
                    setSecret("");
                  }}
                >
                  取消
                </button>
                <button className="btn btn-primary" disabled={busy}>
                  {busy ? (
                    <LoaderCircle size={15} className="spin" />
                  ) : (
                    <Check size={15} />
                  )}
                  保存并连接
                </button>
              </div>
            </form>
          </section>
        </div>
      )}
    </div>
  );
}
void mount(kit, App);
