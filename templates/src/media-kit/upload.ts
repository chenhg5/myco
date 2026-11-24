import { myco } from "../_lib/myco";
import { savePicture } from "./edit";
import { canUseMedia, imageTypes, kit, maxFile, mediaTypes, type Picture } from "./model";

export type PictureOptions = Pick<Picture, "language" | "theme" | "caption">;

function makePreview(source: CanvasImageSource, width: number, height: number) {
  const scale = Math.min(1, 350 / Math.max(width, height));
  const canvas = document.createElement("canvas");
  canvas.width = Math.max(1, Math.round(width * scale));
  canvas.height = Math.max(1, Math.round(height * scale));
  const ctx = canvas.getContext("2d");
  if (!ctx) return "";
  ctx.fillStyle = "#f0f0f0";
  ctx.fillRect(0, 0, canvas.width, canvas.height);
  ctx.drawImage(source, 0, 0, canvas.width, canvas.height);
  for (const quality of [0.67, 0.4, 0.25]) {
    const preview = canvas.toDataURL("image/jpeg", quality);
    if (preview.length <= 18000) return preview;
  }
  return "";
}

async function inspectVideo(blob: Blob) {
  const video = document.createElement("video");
  const url = URL.createObjectURL(blob);
  video.muted = true;
  video.preload = "auto";
  try {
    await new Promise<void>((resolve, reject) => {
      const finish = (error?: Error) => {
        window.clearTimeout(timer);
        video.onloadeddata = null;
        video.onerror = null;
        if (error) reject(error); else resolve();
      };
      const timer = window.setTimeout(() => finish(new Error("video_timeout")), 15_000);
      video.onerror = () => finish(new Error("invalid_video"));
      video.onloadeddata = () => finish();
      video.src = url;
    });
    const { videoWidth: width, videoHeight: height } = video;
    if (!width || !height) throw new Error("invalid_video");
    return { width, height, preview: makePreview(video, width, height) };
  } finally {
    video.removeAttribute("src");
    video.load();
    URL.revokeObjectURL(url);
  }
}

async function inspectImage(blob: Blob) {
  const image = await createImageBitmap(blob);
  try {
    return { width: image.width, height: image.height, preview: makePreview(image, image.width, image.height) };
  } finally { image.close(); }
}

export async function upload(entry: string, file: File, options: PictureOptions) {
  const types = Object.entries(canUseMedia ? mediaTypes : imageTypes);
  const extension = file.name.split(".").pop()?.toLowerCase();
  const format = file.type && file.type !== "application/octet-stream"
    ? types.find(([, type]) => type === file.type)
    : types.find(([ext]) => ext === (extension === "jpeg" ? "jpg" : extension));
  if (!format || file.size > maxFile || !file.size) throw new Error("invalid_media");
  const bridge = myco?.projectFiles;
  if (!bridge) throw new Error("bridge_missing");
  const [suffix, type] = format;
  const blob = file.type === type ? file : file.slice(0, file.size, type);
  const kind = type.startsWith("video/") ? "video" : "image";
  const info = await (kind === "video" ? inspectVideo(blob) : inspectImage(blob));
  const id = crypto.randomUUID().replace(/-/g, "").slice(0, 20);
  const name = "mk-" + kit.page + "-" + entry + "-" + id + "." + suffix;
  await bridge.put(name, blob);
  try {
    await savePicture("img_" + id, {
      entry, file: name, name: file.name.slice(0, 200), kind, bytes: file.size, order: Date.now(), ...info, ...options,
    }, 0);
  } catch {
    // Keep the already uploaded file identifiable for a retry by an agent.
    throw Object.assign(new Error("metadata_failed"), { file: name });
  }
}
