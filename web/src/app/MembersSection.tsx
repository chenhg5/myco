import { useCallback, useEffect, useState } from "react";
import { UserPlus } from "lucide-react";
import { appApi, type HubOverview } from "@/appapi";
import type { AuditEntry, Device, Member, MemberRole } from "@/api";
import { ConfirmDialog, ErrorText, Modal } from "@/app/dialogs";
import { CopyButton } from "@/app/copy";
import { L, locale } from "@/lib/i18n";
import { cn, timeAgo } from "@/lib/utils";
import { RowsSkeleton } from "@/app/Skeleton";

export function roleLabel(r: MemberRole): string {
  if (r === "owner") return L("所有者", "Owner");
  if (r === "admin") return L("管理员", "Admin");
  return L("成员", "Member");
}

/** Hubs that know about members (newer hubs); the first is shown by default. */
function memberHubs(hubsOv?: HubOverview[]) {
  return (hubsOv || []).filter((h) => h.ok && h.me);
}

function HubPicker({ hubs, value, onChange }: { hubs: HubOverview[]; value: string; onChange: (id: string) => void }) {
  if (hubs.length < 2) return null;
  return (
    <select className="app-select-field app-members-hub" value={value} onChange={(e) => onChange(e.target.value)}>
      {hubs.map((h) => <option key={h.id} value={h.id}>{h.name}</option>)}
    </select>
  );
}

function NoMembers({ hubsOv }: { hubsOv?: HubOverview[] }) {
  const any = (hubsOv || []).some((h) => h.ok);
  return (
    <section className="app-setting-card">
      <p className="app-small-note">
        {any
          ? L("这里的 Hub 还不支持多人。把 Hub 升级到最新版本后，就可以邀请成员、给每个人分配角色。", "These Hubs don’t support several people yet. Upgrade the Hub to the latest version to invite colleagues and give each a role.")
          : L("没有连上的 Hub。", "No Hub is connected.")}
      </p>
    </section>
  );
}

/** Settings → 成员: the people on a hub, invites, roles, removal. */
export function MembersSection({ hubsOv, onRefresh }: { hubsOv?: HubOverview[]; onRefresh: () => Promise<void> }) {
  const hubs = memberHubs(hubsOv);
  const [hubId, setHubId] = useState("");
  const hub = hubs.find((h) => h.id === hubId) || hubs[0];
  const me = hub?.me;
  const [members, setMembers] = useState<Member[] | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [inviteOpen, setInviteOpen] = useState(false);
  const [form, setForm] = useState({ name: "", display_name: "", role: "member" as MemberRole });
  const [link, setLink] = useState<{ name: string; link: string; command: string } | null>(null);
  const [busy, setBusy] = useState(false);
  const [formError, setFormError] = useState<unknown>(null);
  const [removing, setRemoving] = useState<Member | null>(null);
  const [renaming, setRenaming] = useState<Member | null>(null);
  const [newName, setNewName] = useState({ name: "", display_name: "" });

  const load = useCallback(async () => {
    if (!hub) return;
    try {
      setMembers((await appApi.members(hub.id)).members);
      setError(null);
    } catch (e) {
      setError(e);
    }
  }, [hub?.id]);
  useEffect(() => { setMembers(null); void load(); }, [load]);

  if (!hub || !me) return <NoMembers hubsOv={hubsOv} />;
  const manages = me.can_manage;
  const iAm = (m: Member) => me.member?.id === m.id;

  const act = async (fn: () => Promise<unknown>) => {
    setBusy(true);
    setFormError(null);
    try {
      await fn();
      await load();
      return true;
    } catch (e) {
      setFormError(e);
      return false;
    } finally {
      setBusy(false);
    }
  };
  const invite = async () => {
    let res: { member: Member; link: string; command?: string } | undefined;
    const ok = await act(async () => { res = await appApi.inviteMember(hub.id, { name: form.name.trim(), display_name: form.display_name.trim() || undefined, role: form.role }); });
    if (ok && res) setLink({ name: res.member.display_name || res.member.name, link: res.link, command: res.command || inviteCommand(res.link) });
  };
  const setRole = (m: Member, role: MemberRole) => void act(() => appApi.updateMember(hub.id, m.name, { role })).then(() => onRefresh());

  return (
    <>
      <div className="app-members-head">
        <HubPicker hubs={hubs} value={hub.id} onChange={setHubId} />
        <span className="app-small-note">
          {me.member ? L(`你是 ${me.member.display_name || me.member.name}（${roleLabel(me.member.role)}）`, `You are ${me.member.display_name || me.member.name} (${roleLabel(me.member.role)})`) : L("你用的是 Hub 的管理员令牌", "You are using the Hub’s admin token")}
        </span>
        {manages && (
          <button type="button" className="app-btn app-btn-primary app-members-invite" onClick={() => { setForm({ name: "", display_name: "", role: "member" }); setLink(null); setFormError(null); setInviteOpen(true); }}>
            <UserPlus />{L("邀请成员", "Invite a member")}
          </button>
        )}
      </div>

      <section className="app-setting-card app-members">
        <ErrorText error={error} />
        {members === null && !error && <RowsSkeleton rows={2} className="is-flat" />}
        {members?.map((m) => (
          <div key={m.id} className="app-member-row">
            <span className="app-member-avatar">{(m.display_name || m.name).slice(0, 1).toUpperCase()}</span>
            <div className="app-member-name">
              <b>{m.display_name || m.name}{iAm(m) && <span className="app-member-you">{L("你", "You")}</span>}</b>
              <small>@{m.name} · {L(`${timeAgo(m.created_at, Date.now())}加入`, `joined ${timeAgo(m.created_at, Date.now())}`)}</small>
            </div>
            {manages && m.role !== "owner" && !iAm(m) ? (
              <select className="app-select-field app-member-role" value={m.role} disabled={busy} onChange={(e) => setRole(m, e.target.value as MemberRole)}>
                <option value="member">{roleLabel("member")}</option>
                <option value="admin">{roleLabel("admin")}</option>
                {me.member?.role === "owner" || me.admin ? <option value="owner">{L("转为所有者", "Make owner")}</option> : null}
              </select>
            ) : (
              <span className={cn("app-member-badge", m.role)}>{roleLabel(m.role)}</span>
            )}
            <div className="app-member-actions">
              {(iAm(m) || manages) && (
                <button type="button" className="app-btn app-btn-plain app-btn-small" onClick={() => { setRenaming(m); setNewName({ name: m.name, display_name: m.display_name || "" }); setFormError(null); }}>
                  {L("改名", "Rename")}
                </button>
              )}
              {manages && m.role !== "owner" && !iAm(m) && (
                <button type="button" className="app-btn app-btn-plain app-btn-small app-danger-text" onClick={() => { setRemoving(m); setFormError(null); }}>
                  {L("移除", "Remove")}
                </button>
              )}
            </div>
          </div>
        ))}
        <p className="app-hint">
          {L("每个人都能看到所有节点和对话、给任何节点发消息；只能管理自己加进来的节点和机器。所有者和管理员可以管理所有人的。", "Everyone sees every node and conversation and can message any node, but manages only the nodes and machines they added. The owner and admins manage everyone’s.")}
        </p>
      </section>

      <DevicesCard hubId={hub.id} currentId={me.device?.id} />

      <Modal open={inviteOpen} onOpenChange={(o) => !busy && setInviteOpen(o)} title={L("邀请成员", "Invite a member")}
        description={link ? undefined : L("对方用这个链接以自己的身份加入这个 Hub：可以让自己的 agent 执行一条命令，不用装 App；也可以在 myco App 里打开。", "They join this Hub as themselves with the link: their agent runs one command, no app needed; or they open it in the myco app.")}>
        {link ? (
          <>
            <p className="app-modal-note">{L(`私下发给 ${link.name}，二选一：`, `Send this privately to ${link.name}; either way works:`)}</p>
            <div className="app-field-label" style={{ marginTop: 12 }}>{L("不用装 App：让对方的 agent（Claude Code、Codex）执行", "No app: their agent (Claude Code, Codex) runs")}</div>
            <div className="app-command"><code>{link.command}</code><CopyButton value={link.command} /></div>
            <div className="app-field-label" style={{ marginTop: 12 }}>{L("或者在 myco App 里「添加 Hub」时粘贴", "Or paste it under “Add Hub” in the myco app")}</div>
            <div className="app-command"><code>{link.link}</code><CopyButton value={link.link} /></div>
            <p className="app-modal-note">{L("只能用一次，24 小时内有效。命令会装好 myco，以对方的身份把那台电脑和执行命令的 agent 接进来，都归对方管；以后想用 App，在那台电脑上运行 myco member pair 拿新链接。", "It works once, within 24 hours. The command installs myco and joins that computer and the agent running it as them, theirs to manage; for the app later, they run myco member pair there.")}</p>
            <div className="app-modal-actions"><button type="button" className="app-btn app-btn-primary" onClick={() => setInviteOpen(false)}>{L("完成", "Done")}</button></div>
          </>
        ) : (
          <form onSubmit={(e) => { e.preventDefault(); void invite(); }}>
            <label className="app-field">
              <span className="app-field-label">{L("用户名", "Handle")}</span>
              <input className="app-input mono" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="laohu" disabled={busy} required autoFocus />
            </label>
            <label className="app-field">
              <span className="app-field-label">{L("名字（可选）", "Name (optional)")}</span>
              <input className="app-input" value={form.display_name} onChange={(e) => setForm({ ...form, display_name: e.target.value })} placeholder={L("老胡", "Lao Hu")} disabled={busy} />
            </label>
            <div className="app-field">
              <span className="app-field-label">{L("角色", "Role")}</span>
              <div className="seg seg-fill" role="tablist">
                {(["member", "admin"] as MemberRole[]).map((r) => (
                  <button key={r} type="button" role="tab" aria-selected={form.role === r} className={cn("seg-btn", form.role === r && "active")} onClick={() => setForm({ ...form, role: r })} disabled={busy}>{roleLabel(r)}</button>
                ))}
              </div>
              <span className="app-small-note">{form.role === "admin" ? L("管理员可以管理所有人的节点、机器和成员。", "Admins manage everyone’s nodes, machines and members.") : L("成员只能管理自己加进来的节点和机器。", "Members manage only the nodes and machines they add.")}</span>
            </div>
            <ErrorText error={formError} />
            <div className="app-modal-actions">
              <button type="button" className="app-btn" onClick={() => setInviteOpen(false)} disabled={busy}>{L("取消", "Cancel")}</button>
              <button type="submit" className="app-btn app-btn-primary" disabled={busy || !form.name.trim()}>{busy ? L("正在生成…", "Creating…") : L("生成邀请链接", "Create invite link")}</button>
            </div>
          </form>
        )}
      </Modal>

      <Modal open={!!renaming} onOpenChange={(o) => !busy && !o && setRenaming(null)} title={L("改名", "Rename")}
        description={L("用户名会显示为对方从 App 发出的消息的署名。", "The handle is shown as the sender of what they send from the app.")}>
        <form onSubmit={(e) => { e.preventDefault(); if (renaming) void act(() => appApi.updateMember(hub.id, renaming.name, { name: newName.name.trim(), display_name: newName.display_name.trim() })).then((ok) => { if (ok) { setRenaming(null); void onRefresh(); } }); }}>
          <label className="app-field">
            <span className="app-field-label">{L("用户名", "Handle")}</span>
            <input className="app-input mono" value={newName.name} onChange={(e) => setNewName({ ...newName, name: e.target.value })} disabled={busy} required />
          </label>
          <label className="app-field">
            <span className="app-field-label">{L("名字", "Name")}</span>
            <input className="app-input" value={newName.display_name} onChange={(e) => setNewName({ ...newName, display_name: e.target.value })} disabled={busy} />
          </label>
          <ErrorText error={formError} />
          <div className="app-modal-actions">
            <button type="button" className="app-btn" onClick={() => setRenaming(null)} disabled={busy}>{L("取消", "Cancel")}</button>
            <button type="submit" className="app-btn app-btn-primary" disabled={busy || !newName.name.trim()}>{L("保存", "Save")}</button>
          </div>
        </form>
      </Modal>

      <ConfirmDialog
        open={!!removing}
        onOpenChange={(o) => !o && setRemoving(null)}
        title={L(`移除 ${removing?.display_name || removing?.name || ""}？`, `Remove ${removing?.display_name || removing?.name || ""}?`)}
        description={L("对方的 App 和命令行凭据会立即失效。对方加进来的节点和机器会保留，可以在节点详情里转给别人。", "Their app and command-line credentials stop working right away. The nodes and machines they added stay; you can give them to someone else in the node details.")}
        confirmLabel={L("移除", "Remove")}
        busy={busy}
        error={formError ? <ErrorText error={formError} /> : undefined}
        onConfirm={() => { if (removing) void act(() => appApi.removeMember(hub.id, removing.name)).then((ok) => ok && setRemoving(null)); }}
      />
    </>
  );
}

const ACTIONS: Record<string, () => string> = {
  "member.invite": () => L("邀请成员", "Invited"),
  "member.update": () => L("修改成员", "Changed member"),
  "member.remove": () => L("移除成员", "Removed member"),
  "agent.register": () => L("登记节点", "Registered node"),
  "agent.rotate_token": () => L("重新签发节点令牌", "Reissued node token"),
  "agent.remove": () => L("删除节点", "Deleted node"),
  "agent.kick": () => L("踢下线", "Disconnected node"),
  "agent.labels": () => L("改分组和标签", "Changed group and tags"),
  "agent.update": () => L("修改节点", "Changed node"),
  "agent.owner": () => L("转交节点", "Gave node away"),
  "machine.rename": () => L("机器改名", "Renamed machine"),
  "machine.owner": () => L("转交机器", "Gave machine away"),
  "machine.spawn": () => L("在机器上新建", "Started on a machine"),
  "link.create": () => L("生成入网链接", "Created join link"),
  "link.revoke": () => L("撤销链接", "Revoked link"),
  "alias.set": () => L("设置别名", "Set alias"),
  "alias.remove": () => L("删除别名", "Removed alias"),
  "console_token.create": () => L("生成 App 链接", "Created app link"),
  "device.add": () => L("添加设备", "Added a device"),
  "device.remove": () => L("移除设备", "Removed a device"),
  "device.link": () => L("生成设备链接", "Created a device link"),
  "message.task": () => L("派任务", "Sent a task"),
  "message.note": () => L("发消息", "Sent a message"),
};

/** Settings → 审计: who did what (owner and admins). */
export function AuditSection({ hubsOv }: { hubsOv?: HubOverview[] }) {
  const hubs = memberHubs(hubsOv).filter((h) => h.me?.can_manage);
  const [hubId, setHubId] = useState("");
  const hub = hubs.find((h) => h.id === hubId) || hubs[0];
  const [actor, setActor] = useState("");
  const [entries, setEntries] = useState<AuditEntry[] | null>(null);
  const [more, setMore] = useState(true);
  const [error, setError] = useState<unknown>(null);
  const [people, setPeople] = useState<Member[]>([]);

  const load = useCallback(async (before?: number) => {
    if (!hub) return;
    try {
      const r = await appApi.audit(hub.id, { actor, before, limit: 100 });
      setEntries((prev) => (before && prev ? [...prev, ...r.entries] : r.entries));
      setMore(r.entries.length === 100);
      setError(null);
    } catch (e) {
      setError(e);
    }
  }, [hub?.id, actor]);
  useEffect(() => { setEntries(null); void load(); }, [load]);
  useEffect(() => { if (hub) appApi.members(hub.id, true).then((r) => setPeople(r.members)).catch(() => {}); }, [hub?.id]);

  if (!hub) {
    return memberHubs(hubsOv).length ? (
      <section className="app-setting-card"><p className="app-small-note">{L("只有所有者和管理员能看审计日志。", "Only the owner and admins can read the audit log.")}</p></section>
    ) : <NoMembers hubsOv={hubsOv} />;
  }
  const fmt = new Intl.DateTimeFormat(locale(), { month: "numeric", day: "numeric", hour: "2-digit", minute: "2-digit" });
  return (
    <>
      <div className="app-members-head">
        <HubPicker hubs={hubs} value={hub.id} onChange={setHubId} />
        <select className="app-select-field app-members-hub" value={actor} onChange={(e) => setActor(e.target.value)} aria-label={L("按人筛选", "Filter by person")}>
          <option value="">{L("所有人", "Everyone")}</option>
          {people.map((p) => <option key={p.id} value={p.name}>{p.display_name || p.name}</option>)}
          <option value="admin">{L("Hub 管理员令牌", "Hub admin token")}</option>
        </select>
      </div>
      <section className="app-setting-card app-audit">
        <ErrorText error={error} />
        {entries?.length === 0 && <div className="app-small-note">{L("还没有记录。", "Nothing recorded yet.")}</div>}
        {entries?.map((e) => (
          <div key={e.id} className="app-audit-row">
            <span className="app-audit-time">{fmt.format(new Date(e.at))}</span>
            <span className="app-audit-who">{e.actor === "admin" ? L("管理员令牌", "Admin token") : e.actor}</span>
            <span className="app-audit-what">
              <b>{ACTIONS[e.action]?.() || e.action}</b>
              {e.target && <span className="mono"> {e.target}</span>}
              {e.detail && <small>{e.detail}</small>}
            </span>
          </div>
        ))}
        {entries && more && entries.length > 0 && (
          <button type="button" className="app-btn app-btn-small app-audit-more" onClick={() => void load(new Date(entries[entries.length - 1].at).getTime())}>{L("更早的记录", "Older entries")}</button>
        )}
        <p className="app-hint">{L("记录管理操作和从 App 派出的任务，不记录消息内容。只追加，不能修改。", "Records administrative actions and tasks sent from the app, never message contents. Append-only.")}</p>
      </section>
    </>
  );
}

/** 我的设备: this person's apps on the hub, a one-time link to add another,
 *  and removing one (its credential stops working). */
function DevicesCard({ hubId, currentId }: { hubId: string; currentId?: string }) {
  const [devices, setDevices] = useState<Device[] | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [link, setLink] = useState<{ link: string; expires_at: string; command?: string } | null>(null);
  const [linkOpen, setLinkOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [removing, setRemoving] = useState<Device | null>(null);

  const load = useCallback(async () => {
    try {
      setDevices((await appApi.devices(hubId)).devices);
      setError(null);
    } catch (e) {
      setError(e);
    }
  }, [hubId]);
  useEffect(() => { setDevices(null); void load(); }, [load]);

  const makeLink = async () => {
    setLink(null);
    setLinkOpen(true);
    setBusy(true);
    setError(null);
    try { setLink(await appApi.createPairing(hubId)); } catch (e) { setError(e); } finally { setBusy(false); }
  };
  const remove = async () => {
    if (!removing) return;
    setBusy(true);
    try {
      await appApi.removeDevice(hubId, removing.id);
      setRemoving(null);
      await load();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };
  const fmt = (iso?: string) => (iso ? timeAgo(iso, Date.now()) : L("从未", "never"));
  return (
    <>
      <div className="app-members-head app-devices-head">
        <span className="app-group-title"><span className="name">{L("我的设备", "My devices")}</span></span>
        <button type="button" className="app-btn app-btn-small app-members-invite" onClick={() => void makeLink()}>{L("添加另一台设备", "Add another device")}</button>
      </div>
      <section className="app-setting-card app-members">
        {!linkOpen && <ErrorText error={error} />}
        {devices?.map((d) => (
          <div key={d.id} className="app-member-row">
            <div className="app-member-name">
              <b>{d.name}{d.id === currentId && <span className="app-member-you">{L("这台设备", "This device")}</span>}</b>
              <small>{L(`添加于 ${fmt(d.created_at)} · 最近使用 ${fmt(d.last_used_at)}`, `added ${fmt(d.created_at)} · last used ${fmt(d.last_used_at)}`)}</small>
            </div>
            <div className="app-member-actions">
              <button type="button" className="app-btn app-btn-plain app-btn-small app-danger-text" onClick={() => setRemoving(d)}>{L("移除", "Remove")}</button>
            </div>
          </div>
        ))}
        <p className="app-hint">{L("每台设备有自己的凭据，存在这台设备的钥匙串里。设备丢了或不用了，在这里移除，它就再也连不上这个 Hub。", "Each device has its own credential, kept in that device’s keychain. If a device is lost or retired, remove it here and it can no longer reach this Hub.")}</p>
      </section>

      <Modal open={linkOpen} onOpenChange={(o) => !busy && setLinkOpen(o)} title={L("添加另一台设备", "Add another device")}
        description={L("在另一台设备（比如 Windows 电脑）的 myco App 里「添加 Hub」时粘贴这个链接，它会以你的身份连上这个 Hub。", "Paste this link under “Add Hub” in the myco app on your other device (a Windows PC, say); it connects to this Hub as you.")}>
        {busy && !link && <div className="app-progress"><span className="app-spinner" />{L("正在生成链接…", "Creating link…")}</div>}
        <ErrorText error={linkOpen ? error : null} />
        {link && (
          <>
            <div className="app-command"><code>{link.link}</code><CopyButton value={link.link} /></div>
            <div className="app-field-label" style={{ marginTop: 12 }}>{L("另一台电脑不装 App，也可以让那里的 agent 执行：", "Or, on a computer without the app, have its agent run:")}</div>
            <div className="app-command"><code>{link.command || inviteCommand(link.link)}</code><CopyButton value={link.command || inviteCommand(link.link)} /></div>
            <p className="app-modal-note">{L("链接只能用一次，24 小时内有效（App 和命令二选一）。别发到公开的地方。", "It works once, within 24 hours (app or command, not both). Don’t post it anywhere public.")}</p>
          </>
        )}
        <div className="app-modal-actions"><button type="button" className="app-btn app-btn-primary" onClick={() => { setLinkOpen(false); void load(); }} disabled={busy}>{L("完成", "Done")}</button></div>
      </Modal>

      <ConfirmDialog
        open={!!removing}
        onOpenChange={(o) => !o && setRemoving(null)}
        title={L(`移除「${removing?.name || ""}」？`, `Remove “${removing?.name || ""}”?`)}
        description={removing?.id === currentId
          ? L("这就是你现在用的这台设备：移除后这个 App 会立即连不上这个 Hub，要用新的链接重新添加。", "This is the device you are using: this app loses access to this Hub right away, and you need a new link to add it again.")
          : L("那台设备上的 App 会立即连不上这个 Hub。", "The app on that device loses access to this Hub right away.")}
        confirmLabel={L("移除", "Remove")}
        busy={busy}
        onConfirm={() => void remove()}
      />
    </>
  );
}

/** The one-liner that uses a one-time hub link on the command line. */
function inviteCommand(link: string): string {
  return `curl -fsSL ${link} | bash`;
}
