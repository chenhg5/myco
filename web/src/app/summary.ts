import type { Message } from "@/api";
import { L, E } from "@/lib/i18n";

/** summarize is a message's body as one plain line, for lists and previews
 * (Markdown, tables and code blocks flattened; exec results summarized). */
export function summarize(m: Message): string {
  if (m.unreadable) return L("🔒 已加密，只有双方的主人能看", "🔒 Encrypted: only the owners of both sides can read it");
  const body = (m.body || "").trim();
  if (!body) return L("（空消息）", "(Empty message)");
  if (body[0] === "{") {
    try {
      const v: unknown = JSON.parse(body);
      if (v && typeof v === "object" && "exit_code" in v) {
        const code = (v as { exit_code?: unknown }).exit_code;
        return typeof code === "number"
          ? L((code === 0 ? E`命令执行完成（退出码 ${code}）` : E`命令执行失败（退出码 ${code}）`), (code === 0 ? E`Command succeeded (exit code ${code})` : E`Command failed (exit code ${code})`))
          : L("命令执行结果", "Command result");
      }
    } catch { /* ordinary text */ }
  }
  return body
    .replace(/```[\s\S]*?```/g, L("［代码］", "[code]"))
    .replace(/^\s*\|?[\s:|-]*-{3,}[\s:|-]*\|?\s*$/gm, "") // table separator rows
    .replace(/\s*\|\s*/g, " ") // table cell borders
    .replace(/[#>*`_~]/g, "")
    .replace(/\s+/g, " ")
    .trim();
}
