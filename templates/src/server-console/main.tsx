// Native SSH workspace. Profiles, credentials and results remain private to the
// local App; shared records cannot trigger a connection or execute an action.
import "../_lib/sheet.css";
import "./server-console.css";
import { useEffect, useRef, useState } from "react";
import { Server, ShieldCheck, Plus, AlertCircle } from "lucide-react";
import { mount, Kit } from "../_lib/appkit";
import { t as uiText } from "../_lib/i18n";
import { Shell, type View } from "./shell";
import { Dashboard } from "./dashboard";
import { Resources } from "./resources";
import {
  AddProfile,
  ReviewOperation,
  LogViewer,
  type ProfileDraft,
} from "./dialogs";
import {
  profiles,
  createProfile,
  removeProfile,
  setAgent,
  run,
  sshReady,
  requireSuccess,
  type NativeProfile,
  type SSHAction,
} from "./ssh";
import { parseOverview, parseServices, parseContainers } from "./protocol";
import { percent } from "./format";
import type {
  Overview,
  Sample,
  Section,
  Service,
  Container,
  Operation,
  Activity,
} from "./types";
const kit = new Kit([], { quiet: true });
function message(e: unknown): string {
  const code = (e as { code?: string })?.code;
  if (code === "target_changed")
    return uiText(
      "SSH 配置或主机密钥已变化。请删除旧连接并重新核对、添加，原授权不能继续使用。",
    );
  if (code === "unsupported")
    return uiText(
      "当前目标或连接方式暂不支持。请使用 Linux 直连或 VPN 地址；暂不支持 ProxyJump。",
    );
  if (code === "host_key_not_trusted")
    return uiText("请先在本机终端核对并信任服务器主机密钥，再添加连接。");
  if (code === "ssh_failed" || code === "invalid_config")
    return uiText(
      "SSH 连接失败，请确认终端中可以用同一目标免交互登录，并检查本机 SSH 配置。",
    );
  if (code === "permission_denied")
    return uiText("当前账号无权执行此操作，请检查 SSH 账号权限。");
  if (code === "outcome_unknown" || code === "cancelled" || code === "timeout")
    return uiText(
      "连接已中断，操作结果可能未知。请先刷新资源状态，不要直接重复执行。",
    );
  return uiText("操作未完成，请检查网络、SSH 配置、主机密钥及账号权限。");
}
function App() {
  const [items, setItems] = useState<NativeProfile[]>([]),
    [selected, setSelected] = useState<NativeProfile | null>(null),
    [view, setView] = useState<View>("overview"),
    [supported, setSupported] = useState(false),
    [checking, setChecking] = useState(true),
    [connected, setConnected] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [add, setAdd] = useState(false);
  const [overview, setOverview] = useState<Overview | null>(null),
    [history, setHistory] = useState<Sample[]>([]),
    [services, setServices] = useState<Section<Service>>({
      available: false,
      items: [],
    }),
    [containers, setContainers] = useState<Section<Container>>({
      available: false,
      items: [],
    }),
    [activity, setActivity] = useState<Activity[]>([]),
    [logs, setLogs] = useState<{
      title: string;
      text: string;
      truncated: boolean;
    } | null>(null),
    [review, setReview] = useState<{
      profile: NativeProfile;
      name: string;
      action: SSHAction;
      operation: Operation;
    } | null>(null);
  const generation = useRef(0),
    current = useRef<NativeProfile | null>(null),
    locked = useRef(false),
    mounted = useRef(true);
  current.current = selected;
  const record = (value: Activity) =>
    setActivity((rows) =>
      [value, ...rows.filter((r) => r.id !== value.id)].slice(0, 100),
    );
  const reset = () => {
    generation.current++;
    setConnected(false);
    setOverview(null);
    setHistory([]);
    setServices({ available: false, items: [] });
    setContainers({ available: false, items: [] });
    setLogs(null);
    setError("");
  };
  useEffect(() => {
    let active = true;
    void (async () => {
      try {
        const ok = await sshReady();
        if (active) setSupported(ok);
        if (ok) {
          const list = await profiles();
          if (active) setItems(list);
        }
      } catch (e) {
        if (active) setError(message(e));
      } finally {
        if (active) setChecking(false);
      }
    })();
    return () => {
      active = false;
      mounted.current = false;
      generation.current++;
    };
  }, []);
  const refresh = async (p: NativeProfile = selected!) => {
    if (!p || locked.current) return;
    const seq = generation.current;
    locked.current = true;
    setBusy(true);
    setError("");
    try {
      const info = parseOverview(requireSuccess(await run(p, "overview")));
      if (seq !== generation.current || !mounted.current) return;
      setOverview(info);
      setConnected(true);
      setHistory((old) =>
        [
          ...old,
          {
            at: Date.now(),
            cpu: info.cpu_percent,
            memory: percent(
              info.memory_total - info.memory_available,
              info.memory_total,
            ),
          },
        ].slice(-60),
      );
      const results = await Promise.allSettled([
        run(p, "services.list"),
        run(p, "containers.list"),
      ]);
      if (seq !== generation.current || !mounted.current) return;
      const sections = results.map((r, i) => {
        if (r.status === "fulfilled" && r.value.exit_code === 0) {
          try {
            return {
              available: true,
              items: i ? parseContainers(r.value) : parseServices(r.value),
            };
          } catch {}
        }
        return {
          available: false,
          items: [],
          reason:
            r.status === "rejected"
              ? message(r.reason)
              : r.value.truncated
                ? uiText("返回内容已截断，无法完整展示资源列表。")
                : r.value.exit_code !== 0
                  ? r.value.stderr.trim().slice(0, 2048) ||
                    uiText("远程命令退出码 {0}", r.value.exit_code)
                  : uiText("返回格式无法识别，未把它当成空列表。"),
        };
      });
      setServices(sections[0] as Section<Service>);
      setContainers(sections[1] as Section<Container>);
    } catch (e) {
      if (seq === generation.current && mounted.current) {
        setConnected(false);
        setError(message(e));
        setHistory((old) =>
          [...old, { at: Date.now(), cpu: null, memory: null }].slice(-60),
        );
      }
    } finally {
      locked.current = false;
      if (mounted.current) setBusy(false);
    }
  };
  useEffect(() => {
    if (!connected || !selected) return;
    const timer = window.setInterval(() => {
      if (!document.hidden && current.current && !locked.current)
        void refresh(current.current);
    }, 15000);
    return () => window.clearInterval(timer);
  }, [connected, selected?.id]);
  const select = (id: string) => {
    if (locked.current) return;
    const p = items.find((v) => v.id === id);
    if (!p) return;
    reset();
    current.current = p;
    setSelected(p);
    void refresh(p);
  };
  const save = async (d: ProfileDraft) => {
    const p = await createProfile(d);
    setItems((old) => [...old, p]);
    reset();
    current.current = p;
    setSelected(p);
    await refresh(p);
  };
  const requestAction = (
    kind: "service" | "container",
    name: string,
    operation: Operation,
  ) => {
    if (selected && !busy)
      setReview({
        profile: { ...selected },
        name,
        action:
          `${kind === "service" ? "services" : "containers"}.${operation}` as SSHAction,
        operation,
      });
  };
  const execute = async () => {
    if (!review || locked.current)
      throw Object.assign(new Error("busy"), { code: "busy" });
    const target = review;
    if (
      target.profile.id !== current.current?.id ||
      target.profile.binding !== current.current.binding
    )
      throw new Error("target_changed");
    locked.current = true;
    setBusy(true);
    const id = crypto.randomUUID(),
      base = {
        id,
        at: Date.now(),
        target: target.profile.name,
        action: `${target.action} · ${target.name}`,
        detail: "",
      };
    record({ ...base, status: "running" });
    try {
      const result = await run(target.profile, target.action, {
        name: target.name,
      });
      requireSuccess(result);
      record({
        ...base,
        status: "succeeded",
        detail:
          result.stdout ||
          uiText("远程命令已返回成功；请以刷新后的资源状态为准。"),
      });
    } catch (e) {
      const code = (e as { code?: string })?.code;
      record({
        ...base,
        status: ["outcome_unknown", "cancelled", "timeout"].includes(code || "")
          ? "unknown"
          : "failed",
        detail: message(e),
      });
      throw e;
    } finally {
      locked.current = false;
      setBusy(false);
      void refresh(target.profile);
    }
  };
  const showLogs = async (kind: "service" | "container", name: string) => {
    if (!selected || locked.current) return;
    const p = selected,
      seq = generation.current;
    locked.current = true;
    setBusy(true);
    setError("");
    try {
      const r = requireSuccess(
        await run(p, kind === "service" ? "services.logs" : "containers.logs", {
          name,
          lines: 100,
        }),
      );
      if (seq === generation.current)
        setLogs({
          title: name,
          text: [r.stdout, r.stderr].filter(Boolean).join("\n"),
          truncated: r.truncated,
        });
    } catch (e) {
      if (seq === generation.current) setError(message(e));
    } finally {
      locked.current = false;
      if (mounted.current) setBusy(false);
    }
  };
  const toggleAgent = async () => {
    if (!selected || locked.current) return;
    const p = selected;
    locked.current = true;
    setBusy(true);
    setError("");
    try {
      const changed = await setAgent(p, !p.agent_enabled);
      setItems((old) => old.map((v) => (v.id === changed.id ? changed : v)));
      setSelected(changed);
    } catch (e) {
      setError(message(e));
    } finally {
      locked.current = false;
      setBusy(false);
    }
  };
  const forget = async () => {
    if (!selected || locked.current) return;
    const p = selected;
    locked.current = true;
    setBusy(true);
    try {
      await removeProfile(p.id);
      reset();
      setItems((old) => old.filter((v) => v.id !== p.id));
      current.current = null;
      setSelected(null);
    } catch (e) {
      setError(message(e));
    } finally {
      locked.current = false;
      setBusy(false);
    }
  };
  return (
    <Shell
      canAdd={supported && !checking && !busy}
      profiles={items}
      selected={selected}
      view={view}
      connected={connected}
      busy={busy}
      onSelect={select}
      onView={setView}
      onAdd={() => setAdd(true)}
      onRefresh={() => void refresh()}
      onDisconnect={() => {
        reset();
      }}
    >
      {error && (
        <div className="sv-banner" role="alert">
          <AlertCircle size={16} />
          {error}
        </div>
      )}
      {!supported ? (
        <section className="sv-empty sv-card">
          <Server size={35} />
          <h3>
            {checking
              ? uiText("正在检查 SSH 能力…")
              : uiText("在桌面 App 中管理你的服务器")}
          </h3>
          <p>
            {uiText(
              "需要本机 OpenSSH 与支持 SSH 的新版 myco 桌面 App。私钥和密码不会存入页面。",
            )}
          </p>
        </section>
      ) : !selected ? (
        <section className="sv-empty sv-card">
          <Server size={35} />
          <h3>{uiText("让服务器，井然有序。")}</h3>
          <p>{uiText("连接一台 Linux 主机，开始查看资源、管理服务和容器。")}</p>
          <button className="sv-button sv-primary" onClick={() => setAdd(true)}>
            <Plus size={16} />
            {uiText("添加 SSH 主机")}
          </button>
        </section>
      ) : (
        <>
          <div className="sv-context">
            <span>
              {selected.target}:{selected.port}
            </span>
            <span>
              {busy
                ? uiText("正在读取或执行…")
                : overview
                  ? uiText(
                      "上次读取 {0}",
                      new Date(overview.sampled_at).toLocaleTimeString(),
                    )
                  : uiText("尚未读取主机数据")}
            </span>
          </div>
          {view === "overview" &&
            (overview ? (
              <Dashboard data={overview} history={history} />
            ) : (
              <section className="sv-card sv-empty">
                <h3>
                  {busy
                    ? uiText("正在连接服务器…")
                    : uiText("尚未获得主机数据")}
                </h3>
                <p>
                  {uiText(
                    "点击刷新连接主机。首次连接前请确认终端中的 SSH 已能正常登录。",
                  )}
                </p>
              </section>
            ))}
          {view === "services" && (
            <Resources
              kind="service"
              loading={busy && !services.available && !services.reason}
              data={services}
              busy={busy || !connected}
              onAction={(name, op) => requestAction("service", name, op)}
              onLogs={(name) => void showLogs("service", name)}
            />
          )}
          {view === "containers" && (
            <Resources
              kind="container"
              loading={busy && !containers.available && !containers.reason}
              data={containers}
              busy={busy || !connected}
              onAction={(name, op) => requestAction("container", name, op)}
              onLogs={(name) => void showLogs("container", name)}
            />
          )}
          {view === "activity" && (
            <section className="sv-card">
              <header>
                <h2>{uiText("本次会话的操作记录")}</h2>
                <span>
                  {uiText("关闭页面后不保留；agent 操作另有框架审计。")}
                </span>
              </header>
              {activity.length ? (
                activity.map((a) => (
                  <div className="sv-activity" key={a.id}>
                    <time>{new Date(a.at).toLocaleTimeString()}</time>
                    <div>
                      <strong>{a.action}</strong>
                      <small>{a.target}</small>
                      <p>{a.detail}</p>
                    </div>
                    <span
                      className={`sv-status ${a.status === "succeeded" ? "good" : ""}`}
                    >
                      {uiText(
                        a.status === "running"
                          ? "执行中"
                          : a.status === "succeeded"
                            ? "已成功"
                            : a.status === "unknown"
                              ? "结果未知"
                              : "未成功",
                      )}
                    </span>
                  </div>
                ))
              ) : (
                <p className="sv-table-empty">{uiText("还没有执行管理操作")}</p>
              )}
            </section>
          )}
          {view === "agent" && (
            <section className="sv-card sv-agent">
              <header>
                <h2>{uiText("让 agent 帮你管理，权限由你决定。")}</h2>
                <ShieldCheck size={20} />
              </header>
              <div>
                <p>
                  {uiText(
                    "开启后，只向当前工作区公开此连接的名称和目标摘要，不公开密钥。Agent 仍需单独申请限时授权。",
                  )}
                </p>
                <dl className="sv-specs">
                  <div>
                    <dt>{uiText("SSH 目标")}</dt>
                    <dd>
                      {selected.target}:{selected.port}
                    </dd>
                  </div>
                  <div>
                    <dt>{uiText("目标绑定")}</dt>
                    <dd>
                      <code>{selected.binding}</code>
                    </dd>
                  </div>
                  {selected.agent_hub && (
                    <div>
                      <dt>{uiText("授权工作区")}</dt>
                      <dd>{selected.agent_hub}</dd>
                    </div>
                  )}
                  <div>
                    <dt>{uiText("Agent 状态")}</dt>
                    <dd>
                      {selected.agent_enabled
                        ? uiText("已启用，需要逐次授权")
                        : uiText("未启用")}
                    </dd>
                  </div>
                </dl>
                <button
                  className="sv-button"
                  disabled={busy}
                  onClick={() => void toggleAgent()}
                >
                  {selected.agent_enabled
                    ? uiText("关闭 Agent 访问")
                    : uiText("为当前工作区启用 Agent")}
                </button>
                <h3>{uiText("只读与管理分开授权")}</h3>
                <p>
                  {uiText(
                    "只读权限可查看资源和日志；管理权限才可启动、停止或重启服务、容器。没有任意 Shell 执行权限。",
                  )}
                </p>
                <pre className="sv-code">myco action targets</pre>
                <p>
                  {uiText(
                    "Agent 从发现列表获取机器、profile 和 binding，再发起授权申请。你在 myco 审批中核对真实目标并批准。",
                  )}
                </p>
                <p>
                  {uiText(
                    "SSH 配置或主机密钥变化后，旧授权失效。需要支持 SSH 资源授权的新版 Hub；旧 Hub 与加密工作区暂不支持。",
                  )}
                </p>
                <h3>{uiText("移除本机连接")}</h3>
                <p>
                  {uiText(
                    "只删除本机连接配置并关闭其 Agent 入口，不删除远端服务器。",
                  )}
                </p>
                <button
                  className="sv-button"
                  disabled={busy}
                  onClick={() => void forget()}
                >
                  {uiText("移除连接")}
                </button>
              </div>
            </section>
          )}
        </>
      )}
      {add && <AddProfile close={() => setAdd(false)} save={save} />}{" "}
      {review && (
        <ReviewOperation
          profile={review.profile}
          resource={review.name}
          operation={review.operation}
          close={() => setReview(null)}
          run={execute}
        />
      )}{" "}
      {logs && <LogViewer {...logs} close={() => setLogs(null)} />}
    </Shell>
  );
}
void mount(kit, App);
