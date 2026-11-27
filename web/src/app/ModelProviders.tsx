import { useCallback, useEffect, useMemo, useState } from "react";
import { KeyRound, Lock, Plus } from "lucide-react";
import { appApi, type HubOverview, type ModelProviderInput } from "@/appapi";
import type { ModelProvider, RuntimeModel } from "@/api";
import { ConfirmDialog, ErrorText, Modal } from "@/app/dialogs";
import { recall, remember } from "@/lib/memo";
import { L } from "@/lib/i18n";
import { cn, timeAgo } from "@/lib/utils";

/** The vendors the App knows (internal/provider's catalogue): their two
 *  interfaces' addresses, filled in when one is picked. */
export const VENDORS: { id: string; name: string; anthropic: string; openai: string }[] = [
  { id: "minimax", name: "MiniMax", anthropic: "https://api.minimax.io/anthropic", openai: "https://api.minimax.io/v1" },
  { id: "deepseek", name: "DeepSeek", anthropic: "https://api.deepseek.com/anthropic", openai: "https://api.deepseek.com/v1" },
  { id: "kimi", name: "Kimi", anthropic: "https://api.moonshot.cn/anthropic", openai: "https://api.moonshot.cn/v1" },
  { id: "opencode_go", name: "OpenCode Go", anthropic: "https://opencode.ai/inference/go", openai: "https://opencode.ai/inference/go/v1" },
  { id: "zhipu", name: "智谱", anthropic: "https://open.bigmodel.cn/api/anthropic", openai: "https://open.bigmodel.cn/api/paas/v4" },
  { id: "qwen", name: "通义", anthropic: "https://dashscope.aliyuncs.com/apps/anthropic", openai: "https://dashscope.aliyuncs.com/compatible-mode/v1" },
  { id: "doubao", name: "豆包", anthropic: "https://ark.cn-beijing.volces.com/api/compatible", openai: "https://ark.cn-beijing.volces.com/api/v3" },
  { id: "openrouter", name: "OpenRouter", anthropic: "https://openrouter.ai/api", openai: "https://openrouter.ai/api/v1" },
];

export const vendorName = (id: string) => VENDORS.find((v) => v.id === id)?.name || L("自定义", "Custom");
const hostOf = (u?: string) => { try { return u ? new URL(u).host : ""; } catch { return u || ""; } };

/** The member's model providers on a hub, kept while the App is open. */
export function useModelProviders(hubId: string | undefined) {
  const key = `model-providers\u0000${hubId || ""}`;
  const [state, setState] = useState<{ e2e: boolean; providers: ModelProvider[] } | null>(() => (hubId ? recall(key) ?? null : null));
  const [error, setError] = useState<unknown>(null);
  const load = useCallback(async () => {
    if (!hubId) return;
    try {
      setState(remember(key, await appApi.modelProviders(hubId)));
      setError(null);
    } catch (e) {
      setError(e);
    }
  }, [hubId, key]);
  useEffect(() => { void load(); }, [load]);
  return { list: state, error, load };
}

/** Which agents use a provider, and which of them have an older key. */
function usersOf(hub: HubOverview | undefined, p: ModelProvider) {
  const agents = (hub?.overview?.agents || []).map((st) => st.agent).filter((a) => a.run_settings?.provider_ref?.id === p.id);
  return { all: agents, behind: agents.filter((a) => (a.run_settings?.provider_ref?.key_version || 0) < p.key_version) };
}

/** Settings › 模型供应商: the member's vendors and keys, for every computer
 *  they add an agent on. Keys are sealed for their own devices. */
export function ModelProvidersSection({ hubsOv, onRefresh, onOpenWorkspaces }: { hubsOv?: HubOverview[]; onRefresh: () => Promise<void>; onOpenWorkspaces?: () => void }) {
  const hubs = (hubsOv || []).filter((h) => h.ok && h.me?.member);
  const [hubId, setHubId] = useState("");
  const hub = hubs.find((h) => h.id === hubId) || hubs[0];
  const { list, error, load } = useModelProviders(hub?.id);
  const [editing, setEditing] = useState<ModelProvider | "new" | null>(null);
  const [deleting, setDeleting] = useState<ModelProvider | null>(null);
  const [busy, setBusy] = useState("");
  const [actionError, setActionError] = useState<unknown>(null);
  const [note, setNote] = useState("");
  if (!hub) {
    return (
      <section className="app-setting-card">
        <p className="app-small-note">{L("模型供应商存在工作区里，需要以成员身份连上一个工作区。", "Model providers are kept in a workspace: connect to one as a member.")}</p>
      </section>
    );
  }
  const e2e = list ? list.e2e : !!hub.e2e;
  const test = async (p: ModelProvider) => {
    setBusy(p.id);
    setActionError(null);
    setNote("");
    try {
      const r = await appApi.testModelProvider(hub.id, p.id);
      setNote(L(`「${p.name}」读到 ${r.models.length} 个模型。`, `“${p.name}”: ${r.models.length} models listed.`));
      await load();
    } catch (e) {
      setActionError(e);
    } finally {
      setBusy("");
    }
  };
  const push = async (p: ModelProvider) => {
    setBusy(p.id);
    setActionError(null);
    setNote("");
    try {
      const r = await appApi.pushModelProvider(hub.id, p.id);
      setNote(r.failed.length
        ? L(`新 key 已发给 ${r.pushed.length} 个 agent；${r.failed.map((f) => f.agent).join("、")} 没发成：${r.failed[0].error}`, `The new key went to ${r.pushed.length} agents; not to ${r.failed.map((f) => f.agent).join(", ")}: ${r.failed[0].error}`)
        : L(`新 key 已发给 ${r.pushed.length} 个 agent，它们从下一条消息开始用。`, `The new key went to ${r.pushed.length} agents; they use it from their next message.`));
      await onRefresh();
    } catch (e) {
      setActionError(e);
    } finally {
      setBusy("");
    }
  };
  const remove = async () => {
    if (!deleting) return;
    setBusy(deleting.id);
    try {
      await appApi.deleteModelProvider(hub.id, deleting.id);
      setDeleting(null);
      await load();
    } catch (e) {
      setActionError(e);
    } finally {
      setBusy("");
    }
  };
  const deletingUsers = deleting ? usersOf(hub, deleting).all.length : 0;
  const providers = list?.providers || [];
  const empty = !!list && providers.length === 0;
  return (
    <>
      {/* One row: what this is, which workspace (when there are several),
          and adding one (only when it can be). */}
      <div className="app-members-head pv-head">
        <span className="app-group-title">
          <span className="name">{L("我的模型供应商", "My model providers")}</span>
          {providers.length > 0 && <span className="app-group-note">{L(` · ${providers.length} 个`, ` · ${providers.length}`)}</span>}
        </span>
        {hubs.length > 1 && (
          <select className="app-select-field pv-hub" value={hub.id} onChange={(e) => setHubId(e.target.value)} aria-label={L("工作区", "Workspace")} title={L("存在哪个工作区", "Which workspace keeps them")}>
            {hubs.map((h) => <option key={h.id} value={h.id}>{h.name}</option>)}
          </select>
        )}
        {e2e && providers.length > 0 && (
          <button type="button" className="app-btn app-btn-small app-btn-primary pv-add" onClick={() => setEditing("new")} data-testid="providers-add">
            <Plus />{L("新增", "Add")}
          </button>
        )}
      </div>
      <section className="app-setting-card app-members" data-testid="providers">
        <ErrorText error={error || actionError} />
        {note && <p className="app-small-note app-offline-ok">{note}</p>}
        {!e2e ? (
          // Why it cannot be used yet, and the way there.
          <div className="pv-state" data-testid="providers-e2e-off">
            <span className="pv-state-icon"><Lock /></span>
            <div className="pv-state-body">
              <b>{L("先打开端到端加密", "Turn on end-to-end encryption first")}</b>
              <p>{L("key 会加密后存在工作区里，只有你自己的设备能解开，所以这个工作区要先打开端到端加密。",
                "Keys are kept in the workspace encrypted for your own devices, so this workspace needs end-to-end encryption first.")}</p>
              <div className="pv-state-actions">
                {onOpenWorkspaces && (
                  <button type="button" className="app-btn app-btn-small app-btn-primary" onClick={onOpenWorkspaces} data-testid="providers-open-e2e">
                    {L("去打开", "Turn it on")}
                  </button>
                )}
                <span className="pv-state-or">{L("或者照旧到 agent 所在的电脑上，在它的「服务商」里填。", "Or, as before, enter it on the agent's computer, in its Provider row.")}</span>
              </div>
            </div>
          </div>
        ) : empty ? (
          <div className="pv-state" data-testid="providers-empty">
            <span className="pv-state-icon"><KeyRound /></span>
            <div className="pv-state-body">
              <b>{L("还没有模型供应商", "No model providers yet")}</b>
              <p>{L("新增一个（比如 OpenCode Go、DeepSeek、Kimi），以后在任何一台电脑上新增 agent 都能直接选。",
                "Add one (OpenCode Go, DeepSeek, Kimi…), then pick it whenever you add an agent on any of your computers.")}</p>
              <div className="pv-state-actions">
                <button type="button" className="app-btn app-btn-small app-btn-primary" onClick={() => setEditing("new")} data-testid="providers-add">
                  <Plus />{L("新增模型供应商", "Add a model provider")}
                </button>
              </div>
            </div>
          </div>
        ) : null}
        {providers.map((p) => {
          const users = usersOf(hub, p);
          return (
            <div key={p.id} className="app-member-row" data-testid={`provider-${p.id}`}>
              <div className="app-member-name">
                <b>{p.name}</b>
                <small>
                  {[vendorName(p.vendor), hostOf(p.anthropic_base || p.openai_base),
                    p.key_readable ? (p.key_hint ? L(`key ···${p.key_hint}`, `key ···${p.key_hint}`) : L("key 已保存", "key saved")) : L("这台设备还解不开 key", "This device cannot open the key yet"),
                    users.all.length ? L(`${users.all.length} 个 agent 在用`, `${users.all.length} agents use it`) : "",
                    p.models?.length && p.models_at ? L(`${p.models.length} 个模型 · ${timeAgo(p.models_at, Date.now())}`, `${p.models.length} models · ${timeAgo(p.models_at, Date.now())}`) : ""]
                    .filter(Boolean).join(" · ")}
                </small>
                {p.seal_again ? (
                  <small className="app-field-hint is-bad" data-testid="provider-seal-again">{L("这是旧版本存的：地址没和 key 一起加密。编辑它，重新填一次 key 才能用。", "Saved by an older version, its addresses are not sealed with its key: edit it and enter the key again to use it.")}</small>
                ) : !p.key_readable && (
                  <small className="app-field-hint">{L("在你另一台已经能用它的设备上打开一次 App，这台就能解开；或者编辑它重新填 key。", "Open the App once on another of your devices that can use it, and this one can too; or edit it and enter the key again.")}</small>
                )}
                {p.settings_changed && (
                  <small className="app-field-hint is-bad" data-testid="provider-settings-changed">{L("工作区上记的地址和跟 key 一起加密的不一样，有人改过。用的仍是加密的那份（上面显示的就是）；不是你改的，请检查工作区的管理权限。", "The workspace's copy of its address differs from the one sealed with its key: someone changed it. The sealed one (shown) is still what is used; if it was not you, check who can manage the workspace.")}</small>
                )}
              </div>
              <div className="app-member-actions">
                {users.behind.length > 0 && p.key_readable && (
                  <button type="button" className={cn("app-btn app-btn-small")} disabled={!!busy} onClick={() => void push(p)} data-testid="provider-push">
                    {L(`推送新 key（${users.behind.length}）`, `Push new key (${users.behind.length})`)}
                  </button>
                )}
                <button type="button" className="app-btn app-btn-plain app-btn-small" disabled={!!busy || !p.key_readable} onClick={() => void test(p)}>
                  {busy === p.id ? L("正在测试", "Testing") : L("测试", "Test")}
                </button>
                <button type="button" className="app-btn app-btn-plain app-btn-small" disabled={!!busy} onClick={() => setEditing(p)}>{L("编辑", "Edit")}</button>
                <button type="button" className="app-btn app-btn-plain app-btn-small" disabled={!!busy} onClick={() => setDeleting(p)}>{L("删除", "Delete")}</button>
              </div>
            </div>
          );
        })}
      </section>
      {e2e && (
        <p className="app-hint pv-foot">{L("key 在这台电脑上加密后存到工作区，只有你自己的设备能解开，工作区看不到。新增 agent 时只写到那台电脑上（它的 env 文件，权限 600）。",
          "Keys are encrypted on this computer before they are kept in the workspace: only your own devices open them, the workspace cannot. When you add an agent, the key is written only to that computer (its env file, mode 600).")}</p>
      )}

      {editing && (
        <ProviderEditor hubId={hub.id} provider={editing === "new" ? null : editing} onClose={() => setEditing(null)}
          onSaved={async () => { setEditing(null); await load(); }} />
      )}
      <ConfirmDialog
        open={!!deleting}
        onOpenChange={(o) => !o && setDeleting(null)}
        title={L(`删除「${deleting?.name || ""}」？`, `Delete “${deleting?.name || ""}”?`)}
        description={deletingUsers
          ? L(`${deletingUsers} 个 agent 在用它。它们会照常用各自电脑上已经写好的那份，App 上会提示改选或者清掉。`, `${deletingUsers} agents use it. They keep using what is written on their computers; the App asks you to pick another or clear it.`)
          : L("没有 agent 在用它。", "No agent uses it.")}
        confirmLabel={L("删除", "Delete")}
        busy={!!busy}
        onConfirm={() => void remove()}
      />
    </>
  );
}

/** Adding or editing a provider: the vendor (its addresses filled in), the
 *  key (sent only to this App's backend, which seals it), a test. */
function ProviderEditor({ hubId, provider, onClose, onSaved }: { hubId: string; provider: ModelProvider | null; onClose: () => void; onSaved: () => Promise<void> }) {
  const [form, setForm] = useState<ModelProviderInput>(() => provider
    ? { name: provider.name, vendor: provider.vendor, anthropic_base: provider.anthropic_base || "", openai_base: provider.openai_base || "", small_model: provider.small_model || "" }
    : { name: VENDORS[0].name, vendor: VENDORS[0].id, anthropic_base: VENDORS[0].anthropic, openai_base: VENDORS[0].openai, small_model: "" });
  const [key, setKey] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [tested, setTested] = useState<RuntimeModel[] | null>(null);
  const [showBases, setShowBases] = useState(!!provider || false);
  const pick = (id: string) => {
    const v = VENDORS.find((x) => x.id === id);
    const named = !form.name.trim() || VENDORS.some((x) => x.name === form.name) || form.name === L("自定义", "Custom");
    setForm({ ...form, vendor: id, name: named ? (v?.name || L("自定义", "Custom")) : form.name,
      anthropic_base: v ? v.anthropic : form.anthropic_base, openai_base: v ? v.openai : form.openai_base });
    setTested(null);
    if (!v) setShowBases(true);
  };
  const badBase = useMemo(() => [form.anthropic_base, form.openai_base].some((u) => {
    if (!u?.trim()) return false;
    try { const x = new URL(u.trim()); return !/^https?:$/.test(x.protocol) || !!x.username || !!x.search || !!x.hash; } catch { return true; }
  }), [form.anthropic_base, form.openai_base]);
  const canSave = !!form.name.trim() && !badBase && !!(form.anthropic_base?.trim() || form.openai_base?.trim()) && (!!provider || !!key.trim());
  const input = (): ModelProviderInput => ({
    name: form.name.trim(), vendor: form.vendor, anthropic_base: form.anthropic_base?.trim() || "", openai_base: form.openai_base?.trim() || "",
    small_model: form.small_model?.trim() || "", ...(key.trim() ? { api_key: key.trim() } : {}),
  });
  const test = async () => {
    setBusy(true);
    setError(null);
    setTested(null);
    try {
      const r = key.trim() || !provider ? await appApi.testModelProvider(hubId, null, input()) : await appApi.testModelProvider(hubId, provider.id);
      setTested(r.models);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };
  const save = async () => {
    setBusy(true);
    setError(null);
    try {
      if (provider) await appApi.updateModelProvider(hubId, provider.id, input());
      else await appApi.createModelProvider(hubId, input());
      await onSaved();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };
  return (
    <Modal open onOpenChange={(o) => !o && !busy && onClose()} title={provider ? L("编辑模型供应商", "Edit model provider") : L("新增模型供应商", "Add a model provider")}
      description={L("选一家，填上它给你的 key。以后在任何一台电脑上新增 agent 都能直接选它。", "Pick a vendor and enter the key it gave you. Pick it whenever you add an agent on any of your computers.")}>
      <div className="app-provider-list" role="radiogroup" data-testid="provider-vendors">
        {[...VENDORS, { id: "custom", name: L("自定义", "Custom"), anthropic: "", openai: "" }].map((v) => (
          <label key={v.id} className={cn("app-provider-opt", form.vendor === v.id && "picked")}>
            <input type="radio" name="vendor" checked={form.vendor === v.id} onChange={() => pick(v.id)} disabled={busy} />
            <span><b>{v.name}</b><small>{v.id === "custom" ? L("Anthropic 或 OpenAI 兼容的地址", "An Anthropic- or OpenAI-compatible address") : hostOf(v.anthropic)}</small></span>
          </label>
        ))}
      </div>
      <label className="app-field">
        <span className="app-field-label">{L("名字", "Name")}</span>
        <input className="app-input" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} disabled={busy} data-testid="provider-name" />
      </label>
      {!showBases ? (
        <button type="button" className="app-kv-link" onClick={() => setShowBases(true)}>{L("地址和小模型", "Addresses and small model")}<span className="app-chev">›</span></button>
      ) : (
        <>
          <label className="app-field">
            <span className="app-field-label">{L("Anthropic 地址（Claude Code 用）", "Anthropic address (for Claude Code)")}</span>
            <input className="app-input" value={form.anthropic_base} spellCheck={false} onChange={(e) => setForm({ ...form, anthropic_base: e.target.value })} disabled={busy} data-testid="provider-anthropic" />
          </label>
          <label className="app-field">
            <span className="app-field-label">{L("OpenAI 地址（Codex 用）", "OpenAI address (for Codex)")}</span>
            <input className="app-input" value={form.openai_base} spellCheck={false} onChange={(e) => setForm({ ...form, openai_base: e.target.value })} disabled={busy} />
            {badBase && <span className="app-field-hint is-bad">{L("地址要以 http:// 或 https:// 开头，不带用户名、? 或 #", "Addresses start with http:// or https://, with no user name, ? or #")}</span>}
          </label>
          <label className="app-field">
            <span className="app-field-label">{L("小模型（可不填）", "Small model (optional)")}</span>
            <input className="app-input" value={form.small_model} spellCheck={false} placeholder={L("Claude Code 的后台小任务用；不填就用这家默认的", "For Claude Code's background work; blank: the vendor's usual one")}
              onChange={(e) => setForm({ ...form, small_model: e.target.value })} disabled={busy} />
          </label>
        </>
      )}
      <label className="app-field">
        <span className="app-field-label">{L("密钥", "Key")}</span>
        <input className="app-input" type="password" autoComplete="new-password" value={key} onChange={(e) => { setKey(e.target.value); setTested(null); }} disabled={busy}
          placeholder={provider ? L("不改就留空", "Leave blank to keep it") : L("填上这家给你的 key", "The key this vendor gave you")} data-testid="provider-api-key" />
        <span className="app-field-hint">{provider
          ? L("填了就换成新的 key，用它的 agent 会提示推送。", "A new key replaces it; agents that use it are offered the new one.")
          : L("在这台电脑上加密后才存到工作区，只有你自己的设备能解开。", "Encrypted on this computer before it is kept in the workspace; only your own devices open it.")}</span>
      </label>
      {tested && <p className="app-small-note app-offline-ok" data-testid="provider-tested">{L(`读到 ${tested.length} 个模型：${tested.slice(0, 4).map((m) => m.id).join("、")}${tested.length > 4 ? " 等" : ""}`, `${tested.length} models: ${tested.slice(0, 4).map((m) => m.id).join(", ")}${tested.length > 4 ? " and more" : ""}`)}</p>}
      <ErrorText error={error} />
      <div className="app-modal-actions">
        <button type="button" className="app-btn" onClick={() => void test()} disabled={busy || (!key.trim() && !provider) || badBase} data-testid="provider-test">
          {busy && !tested ? L("正在测试", "Testing") : L("测试", "Test")}
        </button>
        <button type="button" className="app-btn app-btn-primary" onClick={() => void save()} disabled={busy || !canSave} data-testid="provider-save-workspace">
          {busy ? L("正在保存", "Saving") : L("保存", "Save")}
        </button>
      </div>
    </Modal>
  );
}
