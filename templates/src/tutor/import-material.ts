// Extract text locally and publish page-sized chunks to the workspace only
// after the learner explicitly imports a file. The PDF itself stays local.
import { uid } from "../_lib/appkit";
import { t as uiText } from "../_lib/i18n";
import { myco } from "../_lib/myco";
import { kit, materials, materialParts, type Material } from "./model";

export const FILE_SIZE_MB = 12;
export const PDF_PAGES = 120;
const MAX_CHARS = 240000;
const CHUNK = 3500;

function pieces(text: string, limit: number): string[] {
  const result: string[] = [];
  for (let start = 0; start < text.length;) {
    let end = Math.min(text.length, start + limit);
    // Keep a surrogate pair together so that astral characters survive JSON.
    if (end < text.length && /[\uD800-\uDBFF]/.test(text[end - 1])) end--;
    result.push(text.slice(start, end));
    start = end;
  }
  return result;
}

async function pdfPages(file: File, progress: (current: number, total: number) => void) {
  const [pdf, worker] = await Promise.all([
    import("pdfjs-dist/legacy/build/pdf.mjs"),
    import("pdfjs-dist/legacy/build/pdf.worker.min.mjs?url"),
  ]);
  pdf.GlobalWorkerOptions.workerSrc = worker.default;
  const job = pdf.getDocument({
    data: await file.arrayBuffer(), isEvalSupported: false, useWasm: false,
    disableFontFace: true, stopAtErrors: true,
  });
  let passwordRequired = false;
  job.onPassword = () => {
    passwordRequired = true;
    void job.destroy().catch(() => undefined);
  };
  try {
    const doc = await job.promise;
    if (doc.numPages > PDF_PAGES) throw new Error(uiText("每次最多导入 {0} 页。请拆成几份。", PDF_PAGES));
    let chars = 0;
    const pages: string[] = [];
    for (let i = 1; i <= doc.numPages; i++) {
      progress(i, doc.numPages);
      const page = await doc.getPage(i);
      const content = await page.getTextContent();
      const text = content.items.map((item) => "str" in item ? item.str + (item.hasEOL ? "\n" : item.str ? " " : "") : "").join("").trim();
      page.cleanup();
      chars += text.length;
      if (chars > MAX_CHARS) throw new Error(uiText("教材文字太长。请按章节拆开后再导入。"));
      pages.push(text);
    }
    return pages;
  } catch (error) {
    if (passwordRequired) throw new Error(uiText("这个 PDF 有密码。请先解锁再导入。"));
    if (error instanceof Error && ["InvalidPDFException", "UnknownErrorException", "ResponseException"].includes(error.name)) {
      throw new Error(uiText("读不出这个 PDF。请试试另存为 PDF，或改用文本教材。"));
    }
    throw error;
  } finally {
    await job.destroy().catch(() => undefined);
  }
}

async function textPages(file: File) {
  const text = (await file.text()).replace(/\r\n?/g, "\n").trim();
  if (text.length > MAX_CHARS) throw new Error(uiText("教材文字太长。请按章节拆开后再导入。"));
  return pieces(text, CHUNK);
}

export async function importMaterial(file: File, progress: (current: number, total: number) => void) {
  const isPdf = /\.pdf$/i.test(file.name);
  if (!isPdf && !/\.(txt|md|markdown)$/i.test(file.name)) throw new Error(uiText("请选择 PDF、TXT 或 Markdown 文件。"));
  if (file.size > FILE_SIZE_MB * 1024 * 1024) throw new Error(uiText("文件超过 {0} MB。请按章节拆分。", FILE_SIZE_MB));
  const pages = isPdf ? await pdfPages(file, progress) : await textPages(file);
  if (!pages.some((page) => page.trim())) throw new Error(uiText("没有找到可选中的文字。扫描版 PDF 请先做文字识别。"));
  const id = uid("m");
  const data: Material = {
    title: file.name.slice(0, 240), format: isPdf ? "pdf" : "text", pages: pages.length, order: Date.now(),
  };
  const saved: string[] = [];
  try {
    if (!myco) throw new Error("Missing page bridge");
    // Use server-issued IDs for chunks, so a write that is rejected doesn't
    // leave an optimistic record alive on this client.
    for (const [index, text] of pages.entries()) {
      for (const [order, part] of pieces(text, CHUNK).entries()) {
        const rec = await materialParts.put({ material: id, page: index + 1, order, text: part });
        if (rec) saved.push(rec.id);
      }
    }
    // Commit the catalogue entry last. Readers ignore parts with no entry.
    const record = await myco.data.put("tutor_materials", data, { id, ifVersion: 0 });
    materials.apply({ collection: "tutor_materials", record: id, data, version: record.version, op: "put", seq: 0 });
    return id;
  } catch {
    // If the final response was lost, check the catalogue before rolling back
    // chunks that might now belong to a committed material.
    const actual = await materials.load().catch(() => null);
    if (actual && materials.get(id)) return id;
    if (actual) await Promise.allSettled(saved.map((part) => materialParts.remove(part)));
    throw new Error(uiText("教材没有保存成功。请检查连接后重试。"));
  } finally {
    kit.emit();
  }
}
