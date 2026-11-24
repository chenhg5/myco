// Human-triggered connection and mutation review. Native/agent authorization
// remains authoritative; this dialog never executes shell text or stores keys.
import { useEffect, useRef, useState, type ReactNode } from "react";
import { X, ShieldCheck, ArrowRight } from "lucide-react";
import { t as uiText } from "../_lib/i18n";
import type { Profile, Operation } from "./types";
export type ProfileDraft = { name: string; target: string; port?: number };
export function validDraft(d: ProfileDraft): boolean {
  return (
    d.name.trim().length > 0 &&
    d.name.length <= 80 &&
    d.target.length <= 255 &&
    /^(?:[A-Za-z0-9_.-]+@)?(?:[A-Za-z0-9][A-Za-z0-9_.-]*|\[[A-Fa-f0-9:]+\])$/.test(
      d.target,
    ) &&
    (d.port === undefined ||
      (Number.isInteger(d.port) && d.port >= 1 && d.port <= 65535))
  );
}
function Modal({
  title,
  close,
  children,
}: {
  title: string;
  close: () => void;
  children: ReactNode;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    const dialog = ref.current;
    dialog?.showModal();
    return () => {
      if (dialog?.open) dialog.close();
    };
  }, []);
  return (
    <dialog
      className="sv-modal"
      ref={ref}
      aria-label={title}
      onCancel={(e) => {
        e.preventDefault();
        close();
      }}
    >
      <header>
        <h2>{title}</h2>
        <button
          className="sv-icon-button"
          aria-label={uiText("关闭")}
          onClick={close}
        >
          <X size={17} />
        </button>
      </header>
      {children}
    </dialog>
  );
}
export function AddProfile({
  close,
  save,
}: {
  close: () => void;
  save: (draft: ProfileDraft) => Promise<void>;
}) {
  const [name, setName] = useState(""),
    [target, setTarget] = useState(""),
    [port, setPort] = useState(""),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const draft = {
    name: name.trim(),
    target: target.trim(),
    port: port.trim() ? Number(port) : undefined,
  };
  return (
    <Modal
      title={uiText("添加 SSH 主机")}
      close={() => {
        if (!busy) close();
      }}
    >
      <form
        onSubmit={(e) => {
          e.preventDefault();
          if (!validDraft(draft) || busy) return;
          setBusy(true);
          setError("");
          void save(draft)
            .then(close)
            .catch(() => {
              setError(
                uiText("连接未能建立，请检查 SSH 配置、主机密钥与网络。"),
              );
              setBusy(false);
            });
        }}
      >
        <p className="sv-modal-intro">
          {uiText("使用本机已有的 SSH 配置和密钥，无需把凭证交给页面。")}
        </p>
        <label>
          {uiText("显示名称")}
          <input
            autoFocus
            maxLength={80}
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder={uiText("例如：生产服务器")}
          />
        </label>
        <label>
          {uiText("SSH 配置别名或 user@host")}
          <input
            autoComplete="off"
            spellCheck={false}
            maxLength={255}
            value={target}
            onChange={(e) => setTarget(e.target.value)}
            placeholder="deploy@server.example"
          />
        </label>
        <label>
          {uiText("SSH 端口（留空使用配置）")}
          <input
            type="number"
            min={1}
            max={65535}
            value={port}
            onChange={(e) => setPort(e.target.value)}
          />
        </label>
        <p className="sv-security-note">
          <ShieldCheck size={16} />
          {uiText(
            "本机保存连接信息；首次使用新主机前，请在终端核对并信任它的 SSH 主机密钥。",
          )}
        </p>
        {error && (
          <p role="alert" className="sv-error">
            {error}
          </p>
        )}
        <footer>
          <button
            type="button"
            className="sv-button"
            disabled={busy}
            onClick={close}
          >
            {uiText("取消")}
          </button>
          <button
            type="submit"
            className="sv-button sv-primary"
            disabled={busy || !validDraft(draft)}
          >
            {busy ? uiText("正在连接…") : uiText("保存并连接")}
            <ArrowRight size={15} />
          </button>
        </footer>
      </form>
    </Modal>
  );
}
export function ReviewOperation({
  profile,
  resource,
  operation,
  close,
  run,
}: {
  profile: Profile;
  resource: string;
  operation: Operation;
  close: () => void;
  run: () => Promise<void>;
}) {
  const [busy, setBusy] = useState(false),
    [failed, setFailed] = useState(false),
    label = uiText(
      operation === "start" ? "启动" : operation === "stop" ? "停止" : "重启",
    );
  return (
    <Modal
      title={uiText("确认服务器操作")}
      close={() => {
        if (!busy) close();
      }}
    >
      <div className="sv-review">
        <span className="sv-review-label">{label}</span>
        <h3>{resource}</h3>
        <dl>
          <div>
            <dt>{uiText("服务器")}</dt>
            <dd>{profile.name}</dd>
          </div>
          <div>
            <dt>{uiText("SSH 目标")}</dt>
            <dd>
              {profile.target}:{profile.port}
            </dd>
          </div>
          {profile.fingerprint && (
            <div>
              <dt>{uiText("主机指纹")}</dt>
              <dd>{profile.fingerprint}</dd>
            </div>
          )}
        </dl>
        <p>{uiText("操作会影响这台服务器上的实际资源。请确认目标和操作。")}</p>
        {failed && (
          <p role="alert" className="sv-error">
            {uiText(
              "操作未确认成功。请查看操作记录和资源状态后再决定是否重试。",
            )}
          </p>
        )}
        <footer>
          <button className="sv-button" disabled={busy} onClick={close}>
            {uiText("取消")}
          </button>
          <button
            className="sv-button sv-primary"
            disabled={busy}
            onClick={() => {
              if (busy) return;
              setBusy(true);
              void run()
                .then(close)
                .catch(() => {
                  setBusy(false);
                  setFailed(true);
                });
            }}
          >
            {busy ? uiText("正在执行…") : uiText("确认{0}", label)}
          </button>
        </footer>
      </div>
    </Modal>
  );
}
export function LogViewer({
  title,
  text,
  truncated,
  close,
}: {
  title: string;
  text: string;
  truncated: boolean;
  close: () => void;
}) {
  return (
    <Modal title={title} close={close}>
      {truncated && (
        <p className="sv-security-note">
          {uiText("日志已截断，仅显示本次读取的有限内容。")}
        </p>
      )}
      <pre className="sv-log">{text || uiText("没有可展示的日志")}</pre>
    </Modal>
  );
}
