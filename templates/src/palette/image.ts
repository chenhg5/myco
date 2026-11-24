// Image files stay in this tab. Only extracted color values enter shared data.
import { extractColors } from "./engine";
export async function colorsFromFile(file: File): Promise<string[]> {
  if (!/^image\/(png|jpeg|webp|gif|avif)$/.test(file.type) || file.size > 20 * 1024 * 1024) throw new Error("image_format");
  const url = URL.createObjectURL(file), img = new Image();
  try {
    await new Promise<void>((resolve, reject) => { img.onload = () => resolve(); img.onerror = () => reject(new Error("image_decode")); img.src = url; });
    if (!img.naturalWidth || !img.naturalHeight || img.naturalWidth * img.naturalHeight > 100_000_000) throw new Error("image_size");
    const scale = Math.min(1, 128 / Math.max(img.naturalWidth, img.naturalHeight));
    const canvas = document.createElement("canvas"); canvas.width = Math.max(1, Math.round(img.naturalWidth * scale)); canvas.height = Math.max(1, Math.round(img.naturalHeight * scale));
    const ctx = canvas.getContext("2d", { willReadFrequently: true }); if (!ctx) throw new Error("canvas");
    ctx.drawImage(img, 0, 0, canvas.width, canvas.height);
    return extractColors(ctx.getImageData(0, 0, canvas.width, canvas.height).data, 6);
  } finally { URL.revokeObjectURL(url); img.src = ""; }
}
