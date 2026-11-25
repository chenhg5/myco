import type { Page, PageAttachment } from "@/api";
import { ApiError, appApi } from "@/appapi";
import { L } from "@/lib/i18n";

export const maxPageFile = 20 * 1024 * 1024;
export function pageFileScope(page: Page | null) {
  return page && !page.live && page.attachments_version === 1 ? { page: page.id } : undefined;
}
export function pageFilesUpgrade() {
  return new ApiError(L("请更新工作区和桌面 App 后使用页面附件。", "Update the workspace and desktop App to use page attachments."), 501, "unsupported");
}
function fileID(value: unknown) {
  const id = typeof value === "string" ? value : value && typeof value === "object" ? (value as { file_id?: unknown }).file_id : "";
  if (typeof id !== "string" || !/^paf_[a-z0-9]+$/.test(id)) throw new ApiError(L("页面附件编号无效。", "Invalid page attachment ID."), 400, "invalid_argument");
  return id;
}
function fileName(value: unknown) {
  if (typeof value !== "string" || !value || value.length > 200 || /[\\/\r\n\x00]/.test(value))
    throw new ApiError(L("附件文件名无效。", "Invalid attachment file name."), 400, "invalid_argument");
  return value;
}
export async function pageFileKey(collection: string, name: string, blob: Blob) {
  const hash = await crypto.subtle.digest("SHA-256", await blob.arrayBuffer());
  const content = [...new Uint8Array(hash)].map(b => b.toString(16).padStart(2, "0")).join("");
  const key = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(collection + "\0" + name + "\0" + (blob.type || "application/octet-stream") + "\0" + content));
  return "desktop:" + [...new Uint8Array(key)].map(b => b.toString(16).padStart(2, "0")).join("");
}
export async function answerPageFiles(hubId: string, page: Page | null, method: string, args: Record<string, unknown>): Promise<unknown> {
  const scope = pageFileScope(page);
  if (!scope) throw pageFilesUpgrade();
  switch (method) {
    case "pageFiles.info": return (await appApi.pageAttachments(hubId, scope.page, { limit: 1 })).usage;
    case "pageFiles.list": {
      const limit = args.limit === undefined ? 100 : Number(args.limit);
      if (!Number.isInteger(limit) || limit < 1 || limit > 1000) throw new ApiError("limit must be 1..1000", 400, "invalid_argument");
      return appApi.pageAttachments(hubId, scope.page, { limit, after: args.after ? fileID(args.after) : undefined });
    }
    case "pageFiles.resolve": return appApi.resolvePageAttachment(hubId, scope.page, fileName(args.name));
    case "pageFiles.put": {
      if (page?.read_only) throw new ApiError(L("这个页面只能查看。", "This page is view only."), 403, "permission_denied");
      const collection = String(args.collection || "");
      if (!/^[a-z][a-z0-9_-]{0,39}$/.test(collection)) throw new ApiError("Invalid collection", 400, "invalid_argument");
      const name = fileName(args.name), blob = args.blob;
      if (!(blob instanceof Blob) || blob.size > maxPageFile) throw new ApiError(L("附件不能超过 20 MiB。", "Attachments must not exceed 20 MiB."), 400, "invalid_argument");
      const key = typeof args.key === "string" && args.key ? args.key : await pageFileKey(collection, name, blob);
      if (key.length > 200) throw new ApiError("Upload key must be at most 200 characters", 400, "invalid_argument");
      return appApi.uploadPageAttachment(hubId, scope.page, collection, name, blob, key);
    }
    case "pageFiles.get": return appApi.readPageAttachment(hubId, scope.page, fileID(args.file));
    case "pageFiles.download":
      if (page?.template?.declares && !page.template.permissions?.includes("download"))
        throw new ApiError(L("此应用尚未声明下载权限，请先升级此应用。", "This app does not declare download permission. Update the app first."), 403, "permission_denied");
      await appApi.savePageAttachment(hubId, scope.page, fileID(args.file)); return true;
    case "pageFiles.remove":
      if (page?.read_only) throw new ApiError(L("这个页面只能查看。", "This page is view only."), 403, "permission_denied");
      await appApi.removePageAttachment(hubId, scope.page, fileID(args.file)); return true;
    default: throw new Error("myco: unknown page file call");
  }
}

export async function resolveLegacyPageFile(hubId: string, page: Page, name: string): Promise<PageAttachment | undefined> {
  if (!pageFileScope(page)) return;
  try { return await appApi.resolvePageAttachment(hubId, page.id, name); }
  catch (error) { if (error instanceof ApiError && error.status === 404) return; throw error; }
}
