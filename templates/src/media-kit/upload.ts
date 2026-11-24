import { myco } from "../_lib/myco";
import { savePicture } from "./edit";
import { kit, maxImage, types, type Picture } from "./model";

export type PictureOptions = Pick<Picture, "language" | "theme" | "caption">;

export async function upload(entry: string, file: File, options: PictureOptions) {
  const extension = types[file.type as keyof typeof types];
  if (!extension || file.size > maxImage || !file.size) throw new Error("invalid_image");
  const bridge = myco?.projectFiles;
  if (!bridge) throw new Error("bridge_missing");
  const image = await createImageBitmap(file);
  try {
    const scale = Math.min(1, 350 / Math.max(image.width, image.height));
    const cover = document.createElement("canvas");
    cover.width = Math.max(1, Math.round(image.width * scale));
    cover.height = Math.max(1, Math.round(image.height * scale));
    const ctx = cover.getContext("2d");
    if (ctx) { ctx.fillStyle = "#f0f0f0"; ctx.fillRect(0, 0, cover.width, cover.height); ctx.drawImage(image, 0, 0, cover.width, cover.height); }
    const encoded = ctx ? cover.toDataURL("image/jpeg", 0.67) : "";
    const preview = encoded.length <= 18000 ? encoded : (ctx ? cover.toDataURL("image/jpeg", 0.4) : "");
    const id = crypto.randomUUID().replace(/-/g, "").slice(0, 20);
    const name = "mk-" + kit.page + "-" + entry + "-" + id + "." + extension;
    await bridge.put(name, file);
    try {
      await savePicture("img_" + id, {
        entry, file: name, name: file.name.slice(0, 200), width: image.width, height: image.height,
        bytes: file.size, preview: preview.length <= 18000 ? preview : "", order: Date.now(), ...options,
      }, 0);
    } catch {
      // Keep the already uploaded file identifiable for a retry by an agent.
      throw Object.assign(new Error("metadata_failed"), { file: name });
    }
  } finally { image.close(); }
}
