import type { Page } from "@/api";
import { ApiError, appApi } from "@/appapi";
import { L } from "@/lib/i18n";

const maxSize = 20 * 1024 * 1024;
const supported = { png: "image/png", jpg: "image/jpeg", webp: "image/webp", gif: "image/gif", mp4: "video/mp4", webm: "video/webm" } as const;

export function projectFileScope(page: Page | null) {
  return page?.project && page.template?.id === "tp_myco_media-kit" && /^pg_[a-z0-9]+$/.test(page.id)
    ? { project: page.project, prefix: "mk-" + page.id + "-" } : undefined;
}

export async function answerProjectFiles(
  hubId: string, page: Page | null, method: string, args: Record<string, unknown>,
): Promise<unknown> {
  const scope = projectFileScope(page);
  if (!scope) throw new ApiError(L("这个页面无法访问素材原文件。", "This page cannot access project media."), 403, "permission_denied");
  if (method === "projectFiles.info") {
    try {
      const q = await appApi.projectStorage(hubId, scope.project);
      if (q.files_max) return { used: q.files_used ?? 0, max: q.files_max, maxBytes: maxSize,
        bytesUsed: q.bytes_used ?? 0, bytesMax: q.bytes_max, docsUsed: q.docs_used ?? 0, docsMax: q.docs_max };
    } catch (error) {
      if (!(error instanceof ApiError) || ![404, 501].includes(error.status)) throw error;
    }
    // Older Hubs share a single 100-slot collection; do not silently apply
    // the new default to a server that still enforces the old quota.
    const { docs } = await appApi.project(hubId, scope.project);
    return { used: docs.length, max: 100, maxBytes: maxSize };
  }
  const name = typeof args.name === "string" ? args.name : "";
  // Restrict the bridge to files of this page. The renderer cannot name a
  // project, address a different page, or use arbitrary project documents.
  const match = name.startsWith(scope.prefix) && /^([a-z0-9_-]{1,42})-([a-z0-9]{8,32})\.(png|jpg|webp|gif|mp4|webm)$/.exec(name.slice(scope.prefix.length));
  if (!match) throw new ApiError(L("素材文件名无效。", "Invalid media file name."), 400, "invalid_argument");
  const type = supported[match[3] as keyof typeof supported];
  switch (method) {
    case "projectFiles.put": {
      if (page?.read_only) throw new ApiError(L("这个页面只能查看。", "This page is view only."), 403, "permission_denied");
      const blob = args.blob;
      if (!(blob instanceof Blob) || blob.size < 1 || blob.size > maxSize || blob.type !== type)
        throw new ApiError(L("请选择不超过 20 MB 的 PNG、JPEG、WebP、GIF、MP4 或 WebM 文件。", "Choose a PNG, JPEG, WebP, GIF, MP4, or WebM file up to 20 MB."), 400, "invalid_argument");
      await appApi.uploadProjectFile(hubId, scope.project, new File([blob], name, { type }), name);
      return true;
    }
    case "projectFiles.get": {
      const blob = await appApi.readProjectFile(hubId, scope.project, name);
      if (!blob.size || blob.size > maxSize || (blob.type !== type && blob.type !== "application/octet-stream"))
        throw new ApiError(L("原文件格式无效。", "Invalid original media format."), 400, "invalid_argument");
      return blob.type === type ? blob : blob.slice(0, blob.size, type);
    }
    case "projectFiles.download":
      await appApi.saveProjectFile(hubId, scope.project, name);
      return true;
    default:
      throw new Error("myco: unknown project file call");
  }
}
