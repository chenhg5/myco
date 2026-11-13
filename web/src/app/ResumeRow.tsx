import { useState } from "react";
import { appApi, isOldHub } from "@/appapi";
import type { AgentRow } from "@/app/types";
import { agentKind } from "@/app/nodes";
import { ConfirmDialog, ErrorText, Modal } from "@/app/dialogs";
import { CopyButton } from "@/app/copy";
import { hubHasResume } from "@/app/hubVersion";
import { timeAgo } from "@/lib/utils";
import { L } from "@/lib/i18n";

/** The agent's resume line: a command that brings the same agent (same name,
 *  id and history) back in a new sandbox. Shown to those who may manage it;
 *  a hub before v0.19.5 has no such line and the row stays away. */
export function ResumeRow({ row, now, mayManage, onRefresh }: {
  row: AgentRow;
  now: number;
  mayManage: boolean;
  onRefresh: () => Promise<void>;
}) {
  const ag = row.st.agent;
  // Set when the hub answered 404: older hubs, and the row is not for them.
  const [gone, setGone] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  // The line just made, to show in the dialog (it is shown once).
  const [made, setMade] = useState<{ url: string; command: string } | null>(null);
  const [confirm, setConfirm] = useState<"new" | "revoke" | null>(null);
  // The line shown was replaced: the old one no longer works.
  const [replaced, setReplaced] = useState(false);

  const has = !!ag.resume;
  if (!mayManage || gone || agentKind(ag) !== "agent" ||
    (!has && !hubHasResume(row.hub.hub_version))) return null;

  const fail = (e: unknown) => {
    if (isOldHub(e)) { setGone(true); setConfirm(null); } else setError(e);
  };
  const make = async () => {
    setBusy(true);
    setError(null);
    setConfirm(null);
    const old = has;
    try {
      const r = await appApi.createAgentResume(row.hub.id, ag.name);
      setMade(r);
      setReplaced(old);
      await onRefresh();
    } catch (e) {
      fail(e);
    } finally {
      setBusy(false);
    }
  };
  const revoke = async () => {
    setBusy(true);
    setError(null);
    try {
      await appApi.revokeAgentResume(row.hub.id, ag.name);
      setConfirm(null);
      await onRefresh();
    } catch (e) {
      fail(e);
    } finally {
      setBusy(false);
    }
  };

  const used = ag.resume?.last_used_at;
  const env = ag.resume?.last_used_env;

  return (
    <>
      <div className="app-kv has-help">
        <span className="app-kv-label">{L("重新接入", "Resume")}</span>
        <span className="app-kv-value app-resume-actions">
          <span className={has ? undefined : "app-kv-note"}>
            {has
              ? used
                ? L(
                  `上次重新接入：${timeAgo(used, now)}${env ? ` · 在 ${env}` : ""}`,
                  `Last resumed ${timeAgo(used, now)}${env ? ` · in ${env}` : ""}`,
                )
                : L("还没用过", "Not used yet")
              : L("还没有链接", "No link yet")}
          </span>
          {has ? (
            <>
              <button type="button" className="app-kv-link" onClick={() => setConfirm("new")} disabled={busy}>
                {L("生成新链接", "New link")}
              </button>
              <button type="button" className="app-kv-link" onClick={() => setConfirm("revoke")} disabled={busy}>
                {L("作废", "Revoke")}
              </button>
            </>
          ) : (
            <button type="button" className="app-kv-link" onClick={() => void make()} disabled={busy}>
              {busy ? L("正在生成…", "Making…") : L("生成链接", "Make a link")}
            </button>
          )}
        </span>
        <span className="app-kv-help">
          {has
            ? L("它是这个 agent 的凭证：发给它，或放进它的项目说明里，它在新沙箱里跑一次就接回来了。生成新链接会让旧的立刻失效。", "This line is the agent's credential: send it to the agent, or put it in its project notes; running it once in a new sandbox brings it back. Making a new link revokes the old one at once.")
            : L("临时沙箱里的 agent 丢了凭证时，用这条命令接回来，还是同一个 agent。", "When an agent in a temporary sandbox loses its credentials, this command brings it back as the same agent.")}
        </span>
        <ErrorText error={error} />
      </div>

      <Modal
        open={!!made}
        onOpenChange={(o) => !o && setMade(null)}
        title={L("重新接入链接", "Resume link")}
        description={L("把下面这一行发给那个 agent（或者放进它的项目说明或记忆里），它跑一下就回来了。", "Send the line below to the agent (or put it in its project notes or memory); running it brings the agent back.")}
      >
        {made && (
          <div className="app-command">
            <code>{made.command}</code>
            <CopyButton value={made.command} />
          </div>
        )}
        {replaced && (
          <div className="app-confirm-note">{L("刚才的链接已经作废，不能再用了。", "The previous link is revoked and no longer works.")}</div>
        )}
        <p className="app-small-note">{L("这一行等同于它的凭证，别公开。", "This line works like its credentials; don't publish it.")}</p>
        <div className="app-modal-actions">
          <button type="button" className="app-btn app-btn-primary" onClick={() => setMade(null)}>{L("完成", "Done")}</button>
        </div>
      </Modal>

      <ConfirmDialog
        open={confirm === "new"}
        onOpenChange={(o) => !o && !busy && setConfirm(null)}
        title={L("生成新的重新接入链接？", "Make a new resume link?")}
        description={L("生成后旧的链接会立刻作废：还拿着旧命令的沙箱就接不回来了。", "Making a new one revokes the old immediately: a sandbox still holding the old command can't come back with it.")}
        confirmLabel={L("生成新链接", "Make a new link")}
        onConfirm={() => void make()}
        busy={busy}
      />
      <ConfirmDialog
        open={confirm === "revoke"}
        onOpenChange={(o) => !o && !busy && setConfirm(null)}
        title={L("作废重新接入链接？", "Revoke the resume link?")}
        description={L("之后这条命令就不能用了；agent 丢了凭证时要重新生成一条。", "The command stops working; if the agent loses its credentials you'll need to make a new link.")}
        confirmLabel={L("作废", "Revoke")}
        onConfirm={() => void revoke()}
        busy={busy}
        error={<ErrorText error={error} />}
      />
    </>
  );
}
