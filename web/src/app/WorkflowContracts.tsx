import { useEffect, useRef, useState } from "react";
import { ArrowDownToLine, ArrowUpFromLine, Bot, Braces, Monitor, Plus, UserCheck, X } from "lucide-react";
import type { WfSchema, WfStepDef, WorkflowDef } from "@/api";
import type { AgentRow } from "@/app/types";
import { nodeName } from "@/app/nodes";
import { Avatar } from "@/app/persona";
import { edgesOf, stepKind } from "@/app/WorkflowGraph";
import { L, E } from "@/lib/i18n";
import { cn } from "@/lib/utils";

/**
 * Inputs and outputs (输入和输出): what a run is given, and for each step
 * what it gets and what it must hand back. The hub checks them with all of
 * JSON Schema; here each field reads as a line: name, type, whether it is
 * required, what it is for, and its limits (one of, pattern, at least…).
 * The whole schema is a click away (JSON).
 */

const typeOf = (s?: WfSchema): string => {
  if (!s) return "";
  const t = Array.isArray(s.type) ? s.type.filter((x) => x !== "null")[0] : s.type;
  if (t) return t;
  if (s.properties) return "object";
  if (s.enum) return "enum";
  for (const k of ["oneOf", "anyOf"]) if (Array.isArray(s[k])) return k;
  return "";
};

export const typeWords = (t: string): string => ({
  string: L("文本", "text"), number: L("数字", "number"), integer: L("整数", "integer"), boolean: L("是 / 否", "yes / no"),
  array: L("列表", "list"), object: L("对象", "object"), null: L("空", "null"), enum: L("几个之一", "one of"),
  oneOf: L("几种之一", "one of"), anyOf: L("几种之一", "any of"),
} as Record<string, string>)[t] || t || L("任意", "any");

/** The limits a field has, as short chips. */
function limits(s: WfSchema): string[] {
  const out: string[] = [];
  if (Array.isArray(s.enum)) out.push(L(E`之一：${s.enum.map(String).join(" / ")}`, E`one of: ${s.enum.map(String).join(" / ")}`));
  if (s.const !== undefined) out.push(L(E`固定：${String(s.const)}`, E`exactly: ${String(s.const)}`));
  if (typeof s.pattern === "string") out.push(L(E`格式 ${s.pattern}`, E`pattern ${s.pattern}`));
  for (const [k, zh, en] of [["minLength", "至少", "≥"], ["maxLength", "最多", "≤"]] as const) {
    if (typeof s[k] === "number") out.push(L(E`${zh} ${s[k]} 个字`, E`${en} ${s[k]} chars`));
  }
  for (const [k, zh, en] of [["minimum", "≥", "≥"], ["maximum", "≤", "≤"], ["exclusiveMinimum", ">", ">"], ["exclusiveMaximum", "<", "<"]] as const) {
    if (typeof s[k] === "number") out.push(L(E`${zh} ${s[k]}`, E`${en} ${s[k]}`));
  }
  for (const [k, zh, en] of [["minItems", "至少", "≥"], ["maxItems", "最多", "≤"]] as const) {
    if (typeof s[k] === "number") out.push(L(E`${zh} ${s[k]} 项`, E`${en} ${s[k]} items`));
  }
  if (s.items && typeOf(s.items)) out.push(L(E`每项是${typeWords(typeOf(s.items))}`, E`of ${typeWords(typeOf(s.items))}`));
  if (s.default !== undefined) out.push(L(E`默认 ${JSON.stringify(s.default)}`, E`default ${JSON.stringify(s.default)}`));
  if (s.format && s.format !== "textarea") out.push(String(s.format));
  return out;
}

/** A schema as lines of fields (an object), or one line (anything else). */
export function SchemaView({ schema, empty }: { schema?: WfSchema; empty: string }) {
  const [raw, setRaw] = useState(false);
  if (!schema || Object.keys(schema).length === 0) return <div className="wfc-empty">{empty}</div>;
  const props = Object.entries(schema.properties || {});
  const required = new Set(schema.required || []);
  const extra = Object.keys(schema).filter((k) => !["type", "properties", "required", "title", "description", "additionalProperties"].includes(k));
  return (
    <div className="wfc-schema">
      {props.length > 0 ? props.map(([k, p]) => (
        <div key={k} className="wfc-field">
          <span className="wfc-name mono">{k}</span>
          <span className="wfc-type">{typeWords(typeOf(p))}</span>
          {required.has(k) ? <span className="wfc-req">{L("必填", "required")}</span> : <span className="wfc-opt">{L("可选", "optional")}</span>}
          <span className="wfc-desc">{p.title && p.title !== k ? p.title : ""}{p.description ? `${p.title && p.title !== k ? " · " : ""}${p.description}` : ""}</span>
          {limits(p).map((c) => <span key={c} className="wfc-chip">{c}</span>)}
        </div>
      )) : (
        <div className="wfc-field"><span className="wfc-type">{typeWords(typeOf(schema))}</span>{limits(schema).map((c) => <span key={c} className="wfc-chip">{c}</span>)}</div>
      )}
      {(extra.length > 0 || schema.additionalProperties === false) && (
        <div className="wfc-more">
          {schema.additionalProperties === false && <span className="wfc-chip">{L("不许多出别的字段", "no other fields")}</span>}
          {extra.filter((k) => !["$schema", "$id"].includes(k)).map((k) => <span key={k} className="wfc-chip mono">{k}</span>)}
        </div>
      )}
      <button type="button" className="app-group-action wfc-json" aria-expanded={raw} onClick={() => setRaw((r) => !r)}><Braces />{raw ? L("收起 JSON", "Hide JSON") : "JSON"}</button>
      {raw && <pre className="wfc-raw">{JSON.stringify(schema, null, 2)}</pre>}
    </div>
  );
}

const contractOf = (s: WfStepDef) => s.output || s.outputSchema;

/** What each step gets and hands back, and what a run is given. */
export function ContractsSection({ def, hubId, rows }: { def: WorkflowDef; hubId: string; rows: AgentRow[] }) {
  const entry = def.entry || def.steps[0]?.id;
  const edges = edgesOf(def);
  const rowOf = (n?: string) => (n ? rows.find((r) => r.hub.id === hubId && r.st.agent.name === n) : undefined);
  const gets = (s: WfStepDef): string => {
    if (s.inputSchema) return "";
    if (s.id === entry) return L("运行时填的输入", "The run's inputs");
    if (s.join) return L(E`等 ${(typeof s.join === "object" ? s.join.from : edges.filter((e) => e.to === s.id).map((e) => e.from)).join("、")} 都做完，按步骤收到它们的结果`, "Each step it waits for, by id");
    const into = edges.filter((e) => e.to === s.id);
    const mapped = into.filter((e) => e.map);
    if (mapped.length) return L(E`边上组装的：${Object.keys(mapped[0].map || {}).join("、")}`, E`Built on the edge: ${Object.keys(mapped[0].map || {}).join(", ")}`);
    return into.length ? L("上一步交回的结果", "What the step before handed back") : L("运行时填的输入", "The run's inputs");
  };
  return (
    <div className="aud-card wfc" data-testid="workflow-contracts">
      <div className="wfc-step wfc-run">
        <div className="wfc-step-head"><span className="wf-step-icon"><ArrowDownToLine /></span><b>{L("运行时要填的", "A run is given")}</b></div>
        <SchemaView schema={def.inputs} empty={L("不用填什么。", "Nothing.")} />
      </div>
      {def.steps.map((s) => {
        const kind = stepKind(s);
        const r = rowOf(kind === "computer" ? s.computer : s.agent);
        const c = contractOf(s);
        return (
          <div key={s.id} className="wfc-step" data-step={s.id}>
            <div className="wfc-step-head">
              <span className="wf-step-icon">{kind === "agent" && r ? <Avatar agent={r.st.agent} size={20} badge={false} /> : kind === "computer" ? <Monitor /> : kind === "human" ? <UserCheck /> : <Bot />}</span>
              <b>{s.title || s.id}</b>
              <span className="wfc-who">{s.role ? L(E`分工：${s.role}`, E`Role: ${s.role}`) : kind === "human" ? L("你来决定", "You decide") : r ? nodeName(r.st.agent, r.hub) : s.agent || s.computer}</span>
            </div>
            <div className="wfc-io">
              <div className="wfc-side">
                <span className="wfc-label"><ArrowDownToLine />{L("收到", "Gets")}</span>
                {s.inputSchema ? <SchemaView schema={s.inputSchema} empty="" /> : <div className="wfc-empty">{gets(s)}</div>}
              </div>
              <div className="wfc-side">
                <span className="wfc-label"><ArrowUpFromLine />{L("交回", "Hands back")}</span>
                {kind === "human" ? <div className="wfc-empty">{L(E`选一个：${(s.options || []).join(" / ")}，可以带一句话`, E`One of: ${(s.options || []).join(" / ")}, with a word`)}</div>
                  : kind === "computer" ? <div className="wfc-empty">{L(E`退出码、输出${s.parse === "json" ? "，输出按 JSON 读进 json" : ""}；退出码不是 0 算失败`, (s.parse === "json" ? E`Exit code and output, stdout read as JSON; non-zero fails` : E`Exit code and output; non-zero fails`))}</div>
                    : <SchemaView schema={c} empty={L("一段文字（没有约定结构）", "Text (no contract)")} />}
              </div>
            </div>
          </div>
        );
      })}
    </div>
  );
}

/* ---------- editing a contract: fields, or the JSON ---------- */

const simpleTypes = ["string", "number", "integer", "boolean", "array", "object"];
const simpleKeys = new Set(["type", "properties", "required"]);
const simpleFieldKeys = new Set(["type", "description", "title", "items", "format", "default", "enum"]);

/** Whether the form can show all of it (else: the JSON). */
function isSimple(s?: WfSchema): boolean {
  if (!s) return true;
  if (Object.keys(s).some((k) => !simpleKeys.has(k))) return false;
  if (s.type !== undefined && s.type !== "object") return false;
  return Object.values(s.properties || {}).every((p) => Object.keys(p).every((k) => simpleFieldKeys.has(k)) && typeof p.type === "string" && simpleTypes.includes(p.type));
}

type Field = { name: string; type: string; required: boolean; description: string; keep: WfSchema };

const fieldsOf = (s?: WfSchema): Field[] => Object.entries(s?.properties || {}).map(([name, p]) => ({
  name, type: typeof p.type === "string" ? p.type : "string", required: (s?.required || []).includes(name), description: p.description || "", keep: p,
}));

function schemaOf(fields: Field[]): WfSchema | undefined {
  const named = fields.filter((f) => f.name.trim());
  if (!named.length) return undefined;
  const properties: Record<string, WfSchema> = {};
  for (const f of named) {
    const p: WfSchema = { ...f.keep, type: f.type };
    if (f.description.trim()) p.description = f.description.trim(); else delete p.description;
    if (f.type !== "array") delete p.items;
    properties[f.name.trim()] = p;
  }
  const required = named.filter((f) => f.required).map((f) => f.name.trim());
  return { type: "object", ...(required.length ? { required } : {}), properties };
}

/**
 * A contract's fields as rows (name, type, required, what it is for). One
 * that uses more than the form shows (oneOf, pattern, …) is shown as it
 * reads (SchemaView), with 用 JSON 改 to change it; the JSON only when asked.
 */
export function ContractEditor({ schema, onChange, disabled, label, testid }: {
  schema?: WfSchema; onChange: (s: WfSchema | undefined) => void; disabled?: boolean; label: string; testid?: string;
}) {
  const [json, setJson] = useState(false);
  // more than the form shows, and the JSON not asked for: read it
  const [view, setView] = useState(() => !isSimple(schema));
  const [shown, setShown] = useState<WfSchema | undefined>(schema);
  const [fields, setFields] = useState<Field[]>(() => fieldsOf(schema));
  const [text, setText] = useState(() => (schema ? JSON.stringify(schema, null, 2) : ""));
  const [bad, setBad] = useState("");
  // Only a change is one (not the form first showing what it was given).
  const sent = useRef(JSON.stringify(schemaOf(fieldsOf(schema)) ?? null));
  useEffect(() => {
    if (json || view) return;
    const next = schemaOf(fields);
    const key = JSON.stringify(next ?? null);
    if (key === sent.current) return;
    sent.current = key;
    onChange(next);
  }, [fields]); // eslint-disable-line react-hooks/exhaustive-deps
  const set = (i: number, patch: Partial<Field>) => setFields((fs) => fs.map((f, j) => (j === i ? { ...f, ...patch } : f)));
  const toJSON = () => { setText(JSON.stringify(view ? shown || {} : schemaOf(fields) || {}, null, 2)); setJson(true); setView(false); };
  const parsed = (): { ok: boolean; s?: WfSchema } => { try { return { ok: true, s: text.trim() ? JSON.parse(text) : undefined }; } catch { return { ok: false }; } };
  const back = parsed();
  const formFits = back.ok && isSimple(back.s);
  const toForm = () => {
    if (!back.ok) { setBad(L("不是有效的 JSON", "Not valid JSON")); return; }
    if (!isSimple(back.s)) { setShown(back.s); setBad(""); setJson(false); setView(true); return; }
    setFields(fieldsOf(back.s)); setBad(""); setJson(false);
  };
  return (
    <div className="wfc-edit" data-testid={testid}>
      <div className="wfc-edit-head">
        <span>{label}</span>
        <button type="button" className="app-group-action" disabled={disabled} onClick={() => (json ? toForm() : toJSON())}>
          {!json ? L("用 JSON 改", "Edit as JSON") : formFits ? L("用表单改", "Use the form") : L("收起 JSON", "Hide the JSON")}</button>
      </div>
      {view ? (
        <>
          <SchemaView schema={shown} empty={L("没有约定", "No contract")} />
          <span className="app-small-note">{L("用到了表单写不了的规则（上面带框的小字），要改就「用 JSON 改」。", "It uses rules the form cannot show (the boxed words above): change it with Edit as JSON.")}</span>
        </>
      ) : json ? (
        <>
          <textarea className="app-input mono wfc-text" rows={6} value={text} disabled={disabled} spellCheck={false}
            aria-label={`${label} JSON`} placeholder={'{"type": "object", "required": ["approved"], "properties": {"approved": {"type": "boolean"}}}'}
            onChange={(e) => {
              setText(e.target.value);
              try { const s = e.target.value.trim() ? JSON.parse(e.target.value) : undefined; setBad(""); onChange(s); } catch { setBad(L("不是有效的 JSON", "Not valid JSON")); }
            }} />
          <span className="app-small-note">{L("完整的 JSON Schema（draft 2020-12）：oneOf、pattern、minimum、$defs 都行；保存时工作区会检查。", "Full JSON Schema (draft 2020-12): oneOf, pattern, minimum, $defs…; checked when saved.")}</span>
        </>
      ) : (
        <>
          {fields.map((f, i) => (
            <div key={i} className="wfc-row">
              <input className="app-input mono" value={f.name} disabled={disabled} placeholder={L("字段名", "field")} aria-label={L("字段名", "Field name")} onChange={(e) => set(i, { name: e.target.value })} />
              <select className="app-select-field" value={f.type} disabled={disabled} aria-label={L("类型", "Type")} onChange={(e) => set(i, { type: e.target.value })}>
                {simpleTypes.map((t) => <option key={t} value={t}>{typeWords(t)}</option>)}
              </select>
              <label className={cn("wfc-req-toggle", f.required && "is-on")}><input type="checkbox" checked={f.required} disabled={disabled} onChange={(e) => set(i, { required: e.target.checked })} />{L("必填", "Required")}</label>
              <input className="app-input" value={f.description} disabled={disabled} placeholder={L("说明（可选）", "What it is (optional)")} aria-label={L("说明", "Description")} onChange={(e) => set(i, { description: e.target.value })} />
              <button type="button" className="app-btn app-btn-plain" disabled={disabled} title={L("去掉这个字段", "Remove this field")} aria-label={L("去掉这个字段", "Remove this field")}
                onClick={() => setFields((fs) => fs.filter((_, j) => j !== i))}><X /></button>
            </div>
          ))}
          <button type="button" className="app-group-action wfc-add" disabled={disabled} onClick={() => setFields((fs) => [...fs, { name: "", type: "string", required: true, description: "", keep: {} }])}>
            <Plus />{L("加一个字段", "Add a field")}
          </button>
        </>
      )}
      {bad && <span className="app-modal-hint wfc-bad">{bad}</span>}
    </div>
  );
}
