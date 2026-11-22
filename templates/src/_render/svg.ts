// Application rendering support, separate from the action transport SDK.
// Callers supply SVG produced by their own shared-content renderer.
export async function pngFromSVG(svg: string, width: number, height: number, maxWidth: number, signal: AbortSignal): Promise<{ data: Blob; width: number; height: number }> {
  if (![width, height, maxWidth].every(n => Number.isFinite(n) && n > 0)) throw Object.assign(new Error("Invalid image dimensions"), { code: "invalid_argument" });
  if (signal.aborted) throw Object.assign(new Error("Export cancelled"), { code: "cancelled" });
  const scale = Math.min(1, maxWidth / width, 4096 / height, Math.sqrt(8_000_000 / (width * height)));
  const w = Math.max(1, Math.round(width * scale)), h = Math.max(1, Math.round(height * scale));
  const image = new Image();
  const url = URL.createObjectURL(new Blob([svg], { type: "image/svg+xml" }));
  try {
    await new Promise<void>((resolve, reject) => {
      const abort = () => { image.src = ""; reject(Object.assign(new Error("Export cancelled"), { code: "cancelled" })); };
      const cleanup = () => signal.removeEventListener("abort", abort);
      image.onload = () => { cleanup(); resolve(); };
      image.onerror = () => { cleanup(); reject(Object.assign(new Error("Could not render the SVG"), { code: "export_failed" })); };
      signal.addEventListener("abort", abort, { once: true });
      image.src = url;
    });
    if (signal.aborted) throw Object.assign(new Error("Export cancelled"), { code: "cancelled" });
    const canvas = document.createElement("canvas");
    canvas.width = w; canvas.height = h;
    const ctx = canvas.getContext("2d");
    if (!ctx) throw Object.assign(new Error("Canvas is unavailable"), { code: "unsupported" });
    ctx.drawImage(image, 0, 0, w, h);
    const data = await new Promise<Blob>((resolve, reject) => canvas.toBlob(b => b ? resolve(b) : reject(Object.assign(new Error("Could not encode PNG"), { code: "export_failed" })), "image/png"));
    if (signal.aborted) throw Object.assign(new Error("Export cancelled"), { code: "cancelled" });
    return { data, width: w, height: h };
  } finally {
    image.onload = image.onerror = null;
    URL.revokeObjectURL(url);
  }
}
