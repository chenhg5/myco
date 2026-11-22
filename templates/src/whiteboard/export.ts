// Standalone SVG export uses the same anchors and routes as the live canvas.
// Labels are escaped text; a stable light palette keeps exports shareable.
import { bounds, boxes, connector, number, type Entry } from "./geometry";
export const colors: Record<string, [string, string, string, string]> = {
  gray: ["#f5f4f1", "#303131", "#767875", "#b8bcba"],
  yellow: ["#f8efcc", "#49432c", "#b6a36a", "#cdbd84"],
  pink: ["#f5e4e7", "#47363d", "#ba939f", "#ccabb5"],
  blue: ["#e4edf5", "#2f3d49", "#91a7b9", "#aec2d2"],
  green: ["#e5eee6", "#344438", "#94ac99", "#acc5b2"],
  purple: ["#eee8f4", "#40384b", "#a89ab9", "#c0b2d1"],
  orange: ["#f6e8dc", "#4a3c30", "#ba9e85", "#d0b79f"],
};
export function svgDocument(rows: Entry[]) {
  const bs = boxes(rows),
    area = bounds(bs),
    esc = (v: string) =>
      v
        .replace(/[\u0000-\u0008\u000b\u000c\u000e-\u001f\ufffe\uffff\ud800-\udfff]/gu, "")
        .replace(/&/g, "&amp;")
        .replace(/</g, "&lt;")
        .replace(/>/g, "&gt;")
        .replace(/"/g, "&quot;");
  const paths = rows
    .filter((r) => r.data.type === "arrow")
    .flatMap(({ data: d }) => {
      const f = bs.find((b) => b.id === d.from),
        t = bs.find((b) => b.id === d.to);
      if (!f || !t) return [];
      const c = connector(f, t, d, bs),
        stroke = (colors[String(d.color)] || colors.gray)[2];
      return [
        `<path d="${c.path}" fill="none" stroke="${stroke}" stroke-width="${Math.max(1, Math.min(4, number(d.width, 2)))}" ${d.dash === "dashed" ? 'stroke-dasharray="8 5"' : ""} marker-end="url(#head)"/><text x="${c.label.x}" y="${c.label.y - 8}" text-anchor="middle" font-size="13">${esc(String(d.text || ""))}</text>`,
      ];
    });
  const shapes = bs.map((b) => {
    const palette = colors[b.color] || colors.gray;
    const shape =
      b.type === "text"
        ? ""
        : b.type === "ellipse"
          ? `<ellipse cx="${b.x + b.w / 2}" cy="${b.y + b.h / 2}" rx="${b.w / 2}" ry="${b.h / 2}" fill="#fff" stroke="${palette[2]}"/>`
          : `<rect x="${b.x}" y="${b.y}" width="${b.w}" height="${b.h}" rx="${b.type === "sticky" ? 3 : 10}" fill="${b.type === "sticky" ? palette[0] : "#fff"}" stroke="${palette[2]}"/>`;
    const per = Math.max(1, Math.floor((b.w - 32) / b.font_size)),
      lines = b.text.split("\n").flatMap((line) => {
        const chars = Array.from(line);
        return Array.from(
          { length: Math.max(1, Math.ceil(chars.length / per)) },
          (_, i) => chars.slice(i * per, (i + 1) * per).join(""),
        );
      });
    const center = b.align === "center",
      right = b.align === "right",
      x = center ? b.x + b.w / 2 : right ? b.x + b.w - 16 : b.x + 16;
    const y =
      b.type === "rect" || b.type === "ellipse"
        ? b.y +
          b.h / 2 -
          (lines.length - 1) * b.font_size * 0.75 +
          b.font_size * 0.35
        : b.y + 16 + b.font_size;
    return (
      shape +
      lines
        .map(
          (line, i) =>
            `<text x="${x}" y="${y + i * b.font_size * 1.5}" font-size="${b.font_size}" text-anchor="${center ? "middle" : right ? "end" : "start"}">${esc(line)}</text>`,
        )
        .join("")
    );
  });
  // Routes can run outside nodes; leave room for their outward stubs.
  const pad = 80,
    w = area.w + pad * 2,
    h = area.h + pad * 2,
    x = area.x - pad,
    y = area.y - pad;
  return `<svg xmlns="http://www.w3.org/2000/svg" width="${w}" height="${h}" viewBox="${x} ${y} ${w} ${h}" font-family="-apple-system,Segoe UI,sans-serif" fill="#252725"><defs><marker id="head" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto"><path d="M1 1L9 5L1 9" fill="none" stroke="#767875" stroke-width="1.5"/></marker></defs><rect x="${x}" y="${y}" width="${w}" height="${h}" fill="#fff"/>${paths.join("")}${shapes.join("")}</svg>`;
}
