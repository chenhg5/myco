import { useCallback, useEffect, useMemo, useState } from "react";
import { ChevronDown, KeyRound, Plus } from "lucide-react";
import { ApiError, appApi, type HubOverview, type ModelProviderInput } from "@/appapi";
import type { ModelProvider, ModelProviderTestResponse } from "@/api";
import { ConfirmDialog, ErrorText, Modal } from "@/app/dialogs";
import { recall, remember } from "@/lib/memo";
import { L, E } from "@/lib/i18n";
import { cn, timeAgo } from "@/lib/utils";

/** The vendors the App knows (internal/provider's catalogue): their two
 *  interfaces' addresses, filled in when one is picked. A name with no
 *  English of its own is a getter, read in the App's language. */
export const VENDORS: { id: string; name: string; anthropic: string; openai: string }[] = [
  { id: "minimax", name: "MiniMax", anthropic: "https://api.minimax.io/anthropic", openai: "https://api.minimax.io/v1" },
  { id: "deepseek", name: "DeepSeek", anthropic: "https://api.deepseek.com/anthropic", openai: "https://api.deepseek.com/v1" },
  { id: "kimi", name: "Kimi", anthropic: "https://api.moonshot.cn/anthropic", openai: "https://api.moonshot.cn/v1" },
  { id: "opencode_go", name: "OpenCode Go", anthropic: "https://opencode.ai/inference/go", openai: "https://opencode.ai/inference/go/v1" },
  { id: "zhipu", get name() { return L("智谱", "Zhipu GLM"); }, anthropic: "https://open.bigmodel.cn/api/anthropic", openai: "https://open.bigmodel.cn/api/paas/v4" },
  { id: "qwen", get name() { return L("通义", "Qwen (Alibaba Cloud)"); }, anthropic: "https://dashscope.aliyuncs.com/apps/anthropic", openai: "https://dashscope.aliyuncs.com/compatible-mode/v1" },
  { id: "doubao", get name() { return L("豆包", "Doubao (Volcengine)"); }, anthropic: "https://ark.cn-beijing.volces.com/api/compatible", openai: "https://ark.cn-beijing.volces.com/api/v3" },
  { id: "openrouter", name: "OpenRouter", anthropic: "https://openrouter.ai/api", openai: "https://openrouter.ai/api/v1" },
];

export const vendorName = (id: string) => VENDORS.find((v) => v.id === id)?.name || L("自定义", "Custom");
const hostOf = (u?: string) => { try { return u ? new URL(u).host : ""; } catch { return u || ""; } };

const MINIMAX_REGIONS = {
  international: { anthropic_base: "https://api.minimax.io/anthropic", openai_base: "https://api.minimax.io/v1" },
  china: { anthropic_base: "https://api.minimaxi.com/anthropic", openai_base: "https://api.minimaxi.com/v1" },
};
type MiniMaxRegionID = keyof typeof MINIMAX_REGIONS;
function miniMaxRegion(input: ModelProviderInput): MiniMaxRegionID | "" {
  const bases = [input.anthropic_base, input.openai_base].filter(Boolean);
  for (const region of ["international", "china"] as const) {
    const expected = MINIMAX_REGIONS[region];
    if (bases.length && (!input.anthropic_base || input.anthropic_base === expected.anthropic_base) && (!input.openai_base || input.openai_base === expected.openai_base)) return region;
  }
  return "";
}
function MiniMaxRegion({ value, onChange, disabled, testId, compact }: { value: MiniMaxRegionID | ""; onChange: (value: MiniMaxRegionID) => void; disabled: boolean; testId: string;
  /** beside the vendor (the provider dialog): "地区", its hint said under both */
  compact?: boolean }) {
  return <label className="app-field">
    <span className="app-field-label">{compact ? L("地区", "Region") : L("MiniMax 地区", "MiniMax region")}</span>
    <select className="app-select-field" value={value} onChange={(e) => onChange(e.target.value as MiniMaxRegionID)} disabled={disabled} data-testid={testId}>
      {!value && <option value="" disabled>{L("自定义地址", "Custom addresses")}</option>}
      <option value="international">{compact ? L("国际", "International") : L("国际 · api.minimax.io", "International · api.minimax.io")}</option>
      <option value="china">{compact ? L("国内", "China") : L("国内 · api.minimaxi.com", "China · api.minimaxi.com")}</option>
    </select>
    {!compact && <span className="app-field-hint">{L("key 在哪个站创建，就选哪个地区；两站的 key 不通用。", "Choose the region where you created the key; keys are not shared between the two sites.")}</span>}
  </label>;
}
function testResultText(result: ModelProviderTestResponse) {
  const ms = result.models;
  const names = ms.slice(0, 4).map((m) => m.id);
  return result.models_source === "catalog"
    ? L(E`对话验证通过，可选 ${ms.length} 个预设模型：${names.join("、")}`, E`Completion verified; ${ms.length} preset models: ${names.join(", ")}`)
    : L(E`读到 ${ms.length} 个模型：${names.join("、")}${ms.length > 4 ? " 等" : ""}`, (ms.length > 4 ? E`${ms.length} models: ${names.join(", ")} and more` : E`${ms.length} models: ${names.join(", ")}`));
}
function providerErrorSummary(error: unknown) {
  return error instanceof ApiError && error.code === "provider_region_or_key" ? {
    message: L("MiniMax 返回 HTTP 401：key 或地区不匹配。", "MiniMax returned HTTP 401: check the key and region."),
    suggestion: L("选择创建 key 的地区（国际或国内），再测试；也请确认 key 仍然有效。", "Choose the region where the key was created (International or China), then test again; also check that the key is still valid."),
  } : undefined;
}

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

/** Who keeps keys in a workspace, in one line (i119): with end-to-end
 *  encryption off the hub does, and its administrator could read them. */
export function custodyLine(e2e: boolean): string {
  return e2e
    ? L("key 加密后存在工作区里，只有你自己的设备能解开。", "Keys are encrypted for your own devices before they are kept in the workspace.")
    : L("key 保存在 Hub 上，Hub 管理员能看到；团队共用建议开启端到端加密。", "Keys are kept on the hub, where its administrator could see them; for a shared team, turn on end-to-end encryption.");
}

/** Settings › 模型供应商: the member's vendors and keys, for every computer
 *  they add an agent on. With end-to-end encryption on, keys are sealed for
 *  their own devices; off, the hub keeps them (i119). */
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
  const [moving, setMoving] = useState<ModelProvider | null>(null);
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
      setNote(r.models_source === "catalog" ? testResultText(r) : L(E`「${p.name}」读到 ${r.models.length} 个模型。`, E`“${p.name}”: ${r.models.length} models listed.`));
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
        ? L(E`新 key 已发给 ${r.pushed.length} 个 agent；${r.failed.map((f) => f.agent).join("、")} 没发成：${r.failed[0].error}`, E`The new key went to ${r.pushed.length} agents; not to ${r.failed.map((f) => f.agent).join(", ")}: ${r.failed[0].error}`)
        : L(E`新 key 已发给 ${r.pushed.length} 个 agent，它们从下一条消息开始用。`, E`The new key went to ${r.pushed.length} agents; they use it from their next message.`));
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
  const moveToHub = async () => {
    if (!moving) return;
    setBusy(moving.id);
    setActionError(null);
    try {
      await appApi.moveModelProvider(hub.id, moving.id, "hub");
      setMoving(null);
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
          and adding one. */}
      <div className="app-members-head pv-head">
        <span className="app-group-title">
          <span className="name">{L("我的模型供应商", "My model providers")}</span>
          {providers.length > 0 && <span className="app-group-note">{L(E` · ${providers.length} 个`, E` · ${providers.length}`)}</span>}
        </span>
        {hubs.length > 1 && (
          <select className="app-select-field pv-hub" value={hub.id} onChange={(e) => setHubId(e.target.value)} aria-label={L("工作区", "Workspace")} title={L("存在哪个工作区", "Which workspace keeps them")}>
            {hubs.map((h) => <option key={h.id} value={h.id}>{h.name}</option>)}
          </select>
        )}
        {providers.length > 0 && (
          <button type="button" className="app-btn app-btn-small app-btn-primary pv-add" onClick={() => setEditing("new")} data-testid="providers-add">
            <Plus />{L("新增", "Add")}
          </button>
        )}
      </div>
      <section className="app-setting-card app-members" data-testid="providers">
        <ErrorText error={error || actionError} summary={providerErrorSummary(error || actionError)} />
        {note && <p className="app-small-note app-offline-ok">{note}</p>}
        {!e2e && (
          // Who keeps keys, said once (i119): nothing is in the way.
          <p className="app-small-note" data-testid="providers-hub-kept">
            {custodyLine(false)}{" "}
            {onOpenWorkspaces && (
              <button type="button" className="app-kv-link" onClick={onOpenWorkspaces} data-testid="providers-open-e2e">{L("开启端到端加密", "Turn on encryption")}</button>
            )}
          </p>
        )}
        {empty && (
          <div className="pv-state" data-testid="providers-empty">
            <span className="pv-state-icon"><KeyRound /></span>
            <div className="pv-state-body">
              <b>{L("还没有模型供应商", "No model providers yet")}</b>
              <p>{L("有自己的 key（DeepSeek、Kimi、MiniMax、OpenCode Go……）就新增一个，以后在任何一台电脑上新增 agent 都能直接选。",
                "Have your own key (DeepSeek, Kimi, MiniMax, OpenCode Go…)? Add it, then pick it whenever you add an agent on any of your computers.")}</p>
              <div className="pv-state-actions">
                <button type="button" className="app-btn app-btn-small app-btn-primary" onClick={() => setEditing("new")} data-testid="providers-add">
                  <Plus />{L("新增模型供应商", "Add a model provider")}
                </button>
              </div>
            </div>
          </div>
        )}
        {providers.map((p) => {
          const users = usersOf(hub, p);
          return (
            <div key={p.id} className="app-member-row" data-testid={`provider-${p.id}`}>
              <div className="app-member-name">
                <b>{p.name}</b>
                <small>
                  {[vendorName(p.vendor), hostOf(p.anthropic_base || p.openai_base),
                    p.key_lost ? L("key 已丢失", "key lost")
                      : p.key_readable ? (p.key_hint ? L(E`key ···${p.key_hint}`, E`key ···${p.key_hint}`) : L("key 已保存", "key saved")) : L("这台设备还解不开 key", "This device cannot open the key yet"),
                    p.custody === "hub" ? L("Hub 保管", "Kept by the hub") : L("端到端加密", "End-to-end encrypted"),
                    users.all.length ? L(E`${users.all.length} 个 agent 在用`, E`${users.all.length} agents use it`) : "",
                    p.models?.length && p.models_at ? L(E`${p.models.length} 个模型 · ${timeAgo(p.models_at, Date.now())}`, E`${p.models.length} models · ${timeAgo(p.models_at, Date.now())}`) : ""]
                    .filter(Boolean).join(" · ")}
                </small>
                {p.key_lost ? (
                  <small className="app-field-hint is-bad" data-testid="provider-key-lost">{L("Hub 上用来解开 key 的密钥文件不见了（比如换了服务器没带上）。编辑它，重新填一次 key 就能用；或者从备份恢复 Hub。", "The hub's key file that opens it is gone (moved to another server without it?). Edit it and enter the key again, or restore the hub from a backup.")}</small>
                ) : p.move_to_hub ? (
                  <small className="app-field-hint" data-testid="provider-move-to-hub">{L("工作区关了端到端加密：交给 Hub 保管后才能继续用。", "The workspace has end-to-end encryption off now: hand the key to the hub to keep using it.")}</small>
                ) : p.seal_again ? (
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
                    {L(E`推送新 key（${users.behind.length}）`, E`Push new key (${users.behind.length})`)}
                  </button>
                )}
                {p.move_to_hub && p.key_readable && (
                  <button type="button" className="app-btn app-btn-small" disabled={!!busy} onClick={() => setMoving(p)} data-testid="provider-move">
                    {L("交给 Hub 保管", "Hand to the hub")}
                  </button>
                )}
                <button type="button" className="app-btn app-btn-plain app-btn-small" disabled={!!busy || !p.key_readable || !!p.move_to_hub} onClick={() => void test(p)}>
                  {busy === p.id ? L("正在测试", "Testing") : L("测试", "Test")}
                </button>
                <button type="button" className="app-btn app-btn-plain app-btn-small" disabled={!!busy} onClick={() => setEditing(p)}>{L("编辑", "Edit")}</button>
                <button type="button" className="app-btn app-btn-plain app-btn-small" disabled={!!busy} onClick={() => setDeleting(p)}>{L("删除", "Delete")}</button>
              </div>
            </div>
          );
        })}
      </section>
      <p className="app-hint pv-foot">{e2e
        ? L("key 在这台电脑上加密后存到工作区，只有你自己的设备能解开，工作区看不到。新增 agent 时只写到那台电脑上（它的 env 文件，权限 600）。",
          "Keys are encrypted on this computer before they are kept in the workspace: only your own devices open them, the workspace cannot. When you add an agent, the key is written only to that computer (its env file, mode 600).")
        : L("key 由 Hub 用工作区自己的密钥加密后保存，只发给你选的那台电脑上的 agent（写进它的 env 文件，权限 600）。打开端到端加密后，这台 App 会把它改成只有你的设备能解开。",
          "The hub keeps keys encrypted with the workspace's own key, and sends one only to the agent you pick (into its env file, mode 600). Once end-to-end encryption is on, this App re-encrypts them for your devices alone.")}</p>

      {editing && (
        <ProviderEditor hubId={hub.id} e2e={e2e} provider={editing === "new" ? null : editing} onClose={() => setEditing(null)}
          onSaved={async () => { setEditing(null); await load(); }} />
      )}
      <ConfirmDialog
        open={!!moving}
        onOpenChange={(o) => !o && setMoving(null)}
        title={L(E`把「${moving?.name || ""}」交给 Hub 保管？`, E`Hand “${moving?.name || ""}” to the hub?`)}
        description={L("key 会在这台电脑上解开，交给 Hub 保存。Hub 管理员能看到它。以后打开端到端加密，会再改回只有你的设备能解开。", "The key is opened on this computer and given to the hub to keep. The hub's administrator could see it. Turn end-to-end encryption on later, and it is encrypted for your devices alone again.")}
        confirmLabel={L("交给 Hub", "Hand it over")}
        busy={!!busy}
        onConfirm={() => void moveToHub()}
      />
      <ConfirmDialog
        open={!!deleting}
        onOpenChange={(o) => !o && setDeleting(null)}
        title={L(E`删除「${deleting?.name || ""}」？`, E`Delete “${deleting?.name || ""}”?`)}
        description={deletingUsers
          ? L(E`${deletingUsers} 个 agent 在用它。它们会照常用各自电脑上已经写好的那份，App 上会提示改选或者清掉。`, E`${deletingUsers} agents use it. They keep using what is written on their computers; the App asks you to pick another or clear it.`)
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
function ProviderEditor({ hubId, e2e, provider, onClose, onSaved }: { hubId: string; e2e: boolean; provider: ModelProvider | null; onClose: () => void; onSaved: () => Promise<void> }) {
  const [form, setForm] = useState<ModelProviderInput>(() => provider
    ? { name: provider.name, vendor: provider.vendor, anthropic_base: provider.anthropic_base || "", openai_base: provider.openai_base || "", small_model: provider.small_model || "" }
    : { name: VENDORS[0].name, vendor: VENDORS[0].id, anthropic_base: VENDORS[0].anthropic, openai_base: VENDORS[0].openai, small_model: "" });
  const [key, setKey] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [tested, setTested] = useState<ModelProviderTestResponse | null>(null);
  const [showBases, setShowBases] = useState(!!provider && !VENDORS.some((v) => v.id === provider.vendor));
  const custom = !VENDORS.some((v) => v.id === form.vendor);
  const pick = (id: string) => {
    const v = VENDORS.find((x) => x.id === id);
    const named = !form.name.trim() || VENDORS.some((x) => x.name === form.name) || form.name === L("自定义", "Custom");
    setForm({ ...form, vendor: id, name: named ? (v?.name || L("自定义", "Custom")) : form.name,
      // (a custom address starts empty: not the last vendor's, passed off as its own)
      anthropic_base: v ? v.anthropic : VENDORS.some((x) => x.id === form.vendor) ? "" : form.anthropic_base,
      openai_base: v ? v.openai : VENDORS.some((x) => x.id === form.vendor) ? "" : form.openai_base });
    setTested(null);
    // a custom address must be given; a vendor's own are its defaults
    if (!v) setShowBases(true);
    else if (!provider) setShowBases(false);
  };
  const badBase = useMemo(() => [form.anthropic_base, form.openai_base].some((u) => {
    if (!u?.trim()) return false;
    try { const x = new URL(u.trim()); return !/^https?:$/.test(x.protocol) || !!x.username || !!x.search || !!x.hash; } catch { return true; }
  }), [form.anthropic_base, form.openai_base]);
  const canSave = !!form.name.trim() && !badBase && !!(form.anthropic_base?.trim() || form.openai_base?.trim()) && (!!provider || !!key.trim());
  // With no newly entered key, testing uses the saved, sealed addresses.
  // Do not show a test of the old region as a test of an unsaved new one.
  const testNeedsSave = !!provider && !key.trim() && (form.vendor !== provider.vendor
    || (form.anthropic_base || "") !== (provider.anthropic_base || "") || (form.openai_base || "") !== (provider.openai_base || ""));
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
      setTested(r);
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
      {/* Which vendor: a select (its catalogue, then a custom address), the
          region beside it for MiniMax, and the address it uses under both. */}
      <div className="pv-vendor-row" data-testid="provider-vendors">
        <label className="app-field">
          <span className="app-field-label">{L("供应商", "Vendor")}</span>
          <select className="app-select-field" value={VENDORS.some((v) => v.id === form.vendor) ? form.vendor : "custom"} onChange={(e) => pick(e.target.value)} disabled={busy} data-testid="provider-vendor">
            {VENDORS.map((v) => <option key={v.id} value={v.id}>{v.name}</option>)}
            <option value="custom">{L("自定义地址…", "A custom address…")}</option>
          </select>
        </label>
        {form.vendor === "minimax" && <MiniMaxRegion compact value={miniMaxRegion(form)} onChange={(region) => { setForm({ ...form, ...MINIMAX_REGIONS[region] }); setTested(null); }} disabled={busy} testId="provider-minimax-region" />}
      </div>
      <span className="app-field-hint pv-vendor-hint" data-testid="provider-host">
        {custom
          ? L("填它的 Anthropic 或 OpenAI 兼容的地址（在下面），至少一个。", "Enter its Anthropic- or OpenAI-compatible address below; at least one.")
          : <>{L("地址：", "Address: ")}<span className="mono">{hostOf(form.anthropic_base || form.openai_base) || "—"}</span>
            {form.vendor === "minimax" && L("。key 在哪个站创建，就选哪个地区；两站的 key 不通用。", ". Choose the region where you created the key; keys are not shared between the sites.")}</>}
      </span>
      <label className="app-field">
        <span className="app-field-label">{L("密钥", "Key")}</span>
        <input className="app-input" type="password" autoComplete="new-password" value={key} onChange={(e) => { setKey(e.target.value); setTested(null); }} disabled={busy}
          placeholder={provider ? L("不改就留空", "Leave blank to keep it") : L("填上这家给你的 key", "The key this vendor gave you")} data-testid="provider-api-key" />
        <span className="app-field-hint">{provider
          ? L("填了就换成新的 key，用它的 agent 会提示推送。", "A new key replaces it; agents that use it are offered the new one.")
          : e2e ? L("在这台电脑上加密后才存到工作区，只有你自己的设备能解开。", "Encrypted on this computer before it is kept in the workspace; only your own devices open it.")
          : L("由 Hub 保管（Hub 管理员能看到），只发给你选的那台电脑上的 agent。", "The hub keeps it (its administrator could see it) and sends it only to the agent you pick.")}</span>
      </label>
      <label className="app-field">
        <span className="app-field-label">{L("名字", "Name")}</span>
        <input className="app-input" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} disabled={busy} data-testid="provider-name" />
      </label>
      {/* Addresses and the small model: folded unless they must be given (a
          custom address) or are being edited. */}
      <div className={cn("pv-bases", showBases && "is-open")}>
        <button type="button" className="pv-bases-head" aria-expanded={showBases} onClick={() => setShowBases((x) => !x)} disabled={busy || custom}>
          <ChevronDown className="pv-bases-chev" />
          <span>{L("地址和小模型", "Addresses and small model")}</span>
          {!showBases && <span className="pv-bases-sum">{L("用这家的默认值", "This vendor's defaults")}</span>}
        </button>
        {showBases && <div className="pv-bases-body">
          <label className="app-field">
            <span className="app-field-label">{L("Anthropic 地址（Claude Code 用）", "Anthropic address (for Claude Code)")}</span>
            <input className="app-input mono" value={form.anthropic_base} spellCheck={false} onChange={(e) => setForm({ ...form, anthropic_base: e.target.value })} disabled={busy} data-testid="provider-anthropic"
              placeholder="https://api.example.com/anthropic" />
          </label>
          <label className="app-field">
            <span className="app-field-label">{L("OpenAI 地址（Codex 用）", "OpenAI address (for Codex)")}</span>
            <input className="app-input mono" value={form.openai_base} spellCheck={false} onChange={(e) => setForm({ ...form, openai_base: e.target.value })} disabled={busy}
              placeholder="https://api.example.com/v1" />
            {badBase && <span className="app-field-hint is-bad">{L("地址要以 http:// 或 https:// 开头，不带用户名、? 或 #", "Addresses start with http:// or https://, with no user name, ? or #")}</span>}
          </label>
          <label className="app-field">
            <span className="app-field-label">{L("小模型（可不填）", "Small model (optional)")}</span>
            <input className="app-input mono" value={form.small_model} spellCheck={false} placeholder={L("Claude Code 的后台小任务用；不填就用这家默认的", "For Claude Code's background work; blank: the vendor's usual one")}
              onChange={(e) => setForm({ ...form, small_model: e.target.value })} disabled={busy} />
          </label>
        </div>}
      </div>
      {tested && <p className="app-small-note app-offline-ok" data-testid="provider-tested">{testResultText(tested)}</p>}
      {testNeedsSave && <p className="app-field-hint" data-testid="provider-test-save-first">{L("先保存地区或地址的修改，再测试；也可以重新填 key 后立即测试。", "Save the region or address change before testing, or enter the key again to test it now.")}</p>}
      <ErrorText error={error} summary={providerErrorSummary(error)} />
      <div className="app-modal-actions">
        <button type="button" className="app-btn" onClick={() => void test()} disabled={busy || (!key.trim() && !provider) || badBase || testNeedsSave} data-testid="provider-test">
          {busy && !tested ? L("正在测试", "Testing") : L("测试", "Test")}
        </button>
        <button type="button" className="app-btn app-btn-primary" onClick={() => void save()} disabled={busy || !canSave} data-testid="provider-save-workspace">
          {busy ? L("正在保存", "Saving") : L("保存", "Save")}
        </button>
      </div>
    </Modal>
  );
}

/** 「模型从哪来」, at the top of the new-agent dialog (i119): the computer's
 *  own sign-in, one of the member's model providers, or one added right
 *  here with their own API key (saved, then picked). With a provider, the
 *  model is one of the models it listed. Who keeps the key is said, not
 *  asked. */
export function ModelSource({ hubId, cli, machineName, value, onChange, model, onModel, busy }: {
  hubId: string;
  cli: string;
  machineName: string;
  /** the chosen provider's id; "" for the computer's own */
  value: string;
  onChange: (id: string) => void;
  model: string;
  onModel: (m: string) => void;
  busy?: boolean;
}) {
  const mine = useModelProviders(hubId);
  const [adding, setAdding] = useState(false);
  useEffect(() => setAdding(false), [cli]);
  if (cli !== "claude" && cli !== "codex") return null;
  const e2e = !!mine.list?.e2e;
  const usable = (mine.list?.providers || []).filter((p) => p.key_readable && !p.move_to_hub && (cli === "claude" ? p.anthropic_base : p.openai_base));
  const chosen = usable.find((p) => p.id === value);
  const program = cli === "codex" ? "Codex" : "Claude Code";
  const models = chosen?.models || [];
  return (
    <div className="app-field model-source" data-testid="model-source">
      <span className="app-field-label">{L("模型从哪来", "Where its model comes from")}</span>
      <div className="app-provider-list" role="radiogroup" aria-label={L("模型从哪来", "Where its model comes from")}>
        <label className={cn("app-provider-opt", !value && !adding && "picked")}>
          <input type="radio" name="model-source" checked={!value && !adding} onChange={() => { onChange(""); onModel(""); setAdding(false); }} disabled={busy} />
          <span>
            <b>{L(E`「${machineName}」上的登录或设置`, E`“${machineName}”'s own sign-in or settings`)}</b>
            <small>{L(E`用那台电脑上 ${program} 自己的账号`, E`${program}'s own account on that computer`)}</small>
          </span>
        </label>
        {usable.map((p) => (
          <label key={p.id} className={cn("app-provider-opt", value === p.id && !adding && "picked")} data-testid={`model-source-${p.id}`}>
            <input type="radio" name="model-source" checked={value === p.id && !adding} onChange={() => { onChange(p.id); onModel(""); setAdding(false); }} disabled={busy} />
            <span>
              <b>{p.name}</b>
              <small>{[vendorName(p.vendor), p.models?.length ? L(E`${p.models.length} 个模型`, E`${p.models.length} models`) : "", p.key_hint ? `key ···${p.key_hint}` : ""].filter(Boolean).join(" · ")}</small>
            </span>
          </label>
        ))}
        <label className={cn("app-provider-opt", adding && "picked")} data-testid="model-source-add">
          <input type="radio" name="model-source" checked={adding} onChange={() => setAdding(true)} disabled={busy} />
          <span>
            <b>{L("用自己的 API key", "Use your own API key")}</b>
            <small>{usable.length
              ? L("再接一家", "Add another vendor")
              : L("有自己的 key（DeepSeek、Kimi、MiniMax、OpenCode Go……）可以直接接", "Have your own key (DeepSeek, Kimi, MiniMax, OpenCode Go…)? Use it directly")}</small>
          </span>
        </label>
      </div>
      {adding && (
        <QuickProvider hubId={hubId} cli={cli} e2e={e2e} busy={busy}
          onSaved={async (p, first) => { await mine.load(); setAdding(false); onChange(p.id); onModel(first || ""); }} />
      )}
      {chosen && !adding && (
        <label className="app-field" data-testid="model-source-model">
          <span className="app-field-label">{L("模型", "Model")}</span>
          {models.length ? (
            <select className="app-select-field" value={model} onChange={(e) => onModel(e.target.value)} disabled={busy}>
              <option value="">{L("它默认的模型", "Its default model")}</option>
              {models.map((m) => <option key={m.id} value={m.id}>{m.name ? `${m.name} (${m.id})` : m.id}</option>)}
            </select>
          ) : (
            <input className="app-input mono" value={model} onChange={(e) => onModel(e.target.value.trim())} disabled={busy} spellCheck={false}
              placeholder={L("不填就用它默认的；在 设置 › 模型供应商 里点「测试」可以读出列表", "Blank: its default. Test it in Settings › Model providers to list the models")} />
          )}
        </label>
      )}
      {chosen && !adding && <span className="app-field-hint" data-testid="model-source-custody">{custodyLine(e2e)}</span>}
    </div>
  );
}

/** Adding a provider inside another dialog: a vendor, its key, a test,
 *  "Save and use" (it is then tested once more, to list its models). */
function QuickProvider({ hubId, cli, e2e, busy, onSaved }: {
  hubId: string;
  cli: string;
  e2e: boolean;
  busy?: boolean;
  onSaved: (p: ModelProvider, firstModel: string) => Promise<void>;
}) {
  const [vendor, setVendor] = useState(VENDORS[0].id);
  const [region, setRegion] = useState<MiniMaxRegionID>("international");
  const [base, setBase] = useState("");
  const [key, setKey] = useState("");
  const [working, setWorking] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [tested, setTested] = useState<ModelProviderTestResponse | null>(null);
  const v = VENDORS.find((x) => x.id === vendor);
  const custom = !v;
  const input = (): ModelProviderInput => {
    const name = v?.name || (() => { try { return new URL(base.trim()).host; } catch { return L("自定义", "Custom"); } })();
    return custom
      ? { name, vendor: "custom", anthropic_base: cli === "claude" ? base.trim() : "", openai_base: cli === "codex" ? base.trim() : "", small_model: "", api_key: key.trim() }
      : { name, vendor, ...(vendor === "minimax" ? MINIMAX_REGIONS[region] : { anthropic_base: v.anthropic, openai_base: v.openai }), small_model: "", api_key: key.trim() };
  };
  const badBase = custom && (() => { try { const x = new URL(base.trim()); return !/^https?:$/.test(x.protocol) || !!x.username || !!x.search || !!x.hash; } catch { return true; } })();
  const ready = !!key.trim() && !badBase && !working && !busy;
  const test = async () => {
    setWorking(true);
    setError(null);
    setTested(null);
    try {
      setTested(await appApi.testModelProvider(hubId, null, input()));
    } catch (e) {
      setError(e);
    } finally {
      setWorking(false);
    }
  };
  const save = async () => {
    setWorking(true);
    setError(null);
    try {
      const p = await appApi.createModelProvider(hubId, input());
      // its models, kept for the model list (a vendor that cannot list them
      // still works: the model is then typed)
      await appApi.testModelProvider(hubId, p.id).catch(() => undefined);
      await onSaved(p, "");
    } catch (e) {
      setError(e);
    } finally {
      setWorking(false);
    }
  };
  return (
    <div className="app-provider-manual" data-testid="model-source-new">
      <div className="ms-row">
      <label className="app-field ms-vendor">
        <span className="app-field-label">{L("哪一家", "Which vendor")}</span>
        <select className="app-select-field" value={vendor} onChange={(e) => { setVendor(e.target.value); setTested(null); }} disabled={working || busy} data-testid="model-source-vendor">
          {VENDORS.map((x) => <option key={x.id} value={x.id}>{x.name}</option>)}
          <option value="custom">{L("自定义地址", "A custom address")}</option>
        </select>
      </label>
      {vendor === "minimax" && <MiniMaxRegion value={region} onChange={(next) => { setRegion(next); setTested(null); }} disabled={working || !!busy} testId="model-source-minimax-region" />}
      {custom && (
        <label className="app-field">
          <span className="app-field-label">{cli === "codex" ? L("OpenAI 兼容的地址", "OpenAI-compatible address") : L("Anthropic 兼容的地址", "Anthropic-compatible address")}</span>
          <input className="app-input mono" value={base} onChange={(e) => { setBase(e.target.value); setTested(null); }} spellCheck={false} disabled={working || busy}
            placeholder={cli === "codex" ? "https://api.example.com/v1" : "https://api.example.com/anthropic"} data-testid="model-source-base" />
          {base.trim() && badBase && <span className="app-field-hint is-bad">{L("地址要以 http:// 或 https:// 开头，不带用户名、? 或 #", "Addresses start with http:// or https://, with no user name, ? or #")}</span>}
        </label>
      )}
      <label className="app-field ms-key">
        <span className="app-field-label">{L("API key", "API key")}</span>
        <input className="app-input" type="password" autoComplete="new-password" value={key} onChange={(e) => { setKey(e.target.value); setTested(null); }}
          disabled={working || busy} placeholder={L("填上这家给你的 key", "The key this vendor gave you")} data-testid="model-source-key" />
      </label>
      </div>
      {tested && <p className="app-small-note app-offline-ok" data-testid="model-source-tested">{testResultText(tested)}</p>}
      <ErrorText error={error} summary={providerErrorSummary(error)} />
      {/* One line about where it goes and who can read it; the buttons beside it. */}
      <div className="ms-foot">
        <span className="ms-note" data-testid="model-source-custody">
          {!e2e && L("存进「我的模型供应商」，以后哪台电脑都能直接选。", "Saved to My model providers, for any computer later. ")}{custodyLine(e2e)}
        </span>
        <button type="button" className="app-btn app-btn-small" onClick={() => void test()} disabled={!ready} data-testid="model-source-test">
          {working && !tested ? L("正在测试", "Testing") : L("测试", "Test")}
        </button>
        <button type="button" className="app-btn app-btn-small app-btn-primary" onClick={() => void save()} disabled={!ready} data-testid="model-source-save">
          {L("保存并选用", "Save and use")}
        </button>
      </div>
    </div>
  );
}
