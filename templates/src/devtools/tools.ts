import { t as uiText } from "../_lib/i18n";
// The toolbox's sums: everything runs here, in the page.
const enc = new TextEncoder(), dec = new TextDecoder();

export const b64encode = (s: string, url = false) => {
  let bin = ""; for (const b of enc.encode(s)) bin += String.fromCharCode(b);
  const out = btoa(bin);
  return url ? out.replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "") : out;
};
export const b64decode = (s: string) => {
  let t = s.trim().replace(/-/g, "+").replace(/_/g, "/").replace(/\s+/g, "");
  while (t.length % 4) t += "=";
  const bin = atob(t), bytes = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
  return dec.decode(bytes);
};

// MD5 (RFC 1321), for text as UTF-8.
export function md5(s: string): string {
  const msg = enc.encode(s), n = (((msg.length + 8) >> 6) + 1) * 16, w = new Uint32Array(n);
  for (let i = 0; i < msg.length; i++) w[i >> 2] |= msg[i] << ((i % 4) * 8);
  w[msg.length >> 2] |= 0x80 << ((msg.length % 4) * 8);
  w[n - 2] = (msg.length * 8) >>> 0;
  w[n - 1] = Math.floor((msg.length * 8) / 0x100000000);
  const K = Array.from({ length: 64 }, (_, i) => Math.floor(Math.abs(Math.sin(i + 1)) * 0x100000000) >>> 0);
  const R = [7, 12, 17, 22, 5, 9, 14, 20, 4, 11, 16, 23, 6, 10, 15, 21];
  let a0 = 0x67452301, b0 = 0xefcdab89, c0 = 0x98badcfe, d0 = 0x10325476;
  for (let o = 0; o < n; o += 16) {
    let A = a0, B = b0, C = c0, D = d0;
    for (let i = 0; i < 64; i++) {
      let F: number, g: number;
      if (i < 16) { F = (B & C) | (~B & D); g = i; }
      else if (i < 32) { F = (D & B) | (~D & C); g = (5 * i + 1) % 16; }
      else if (i < 48) { F = B ^ C ^ D; g = (3 * i + 5) % 16; }
      else { F = C ^ (B | ~D); g = (7 * i) % 16; }
      const t = D; D = C; C = B;
      const x = (A + F + K[i] + w[o + g]) >>> 0, r = R[(i >> 4) * 4 + (i % 4)];
      B = (B + ((x << r) | (x >>> (32 - r)))) >>> 0;
      A = t;
    }
    a0 = (a0 + A) >>> 0; b0 = (b0 + B) >>> 0; c0 = (c0 + C) >>> 0; d0 = (d0 + D) >>> 0;
  }
  return [a0, b0, c0, d0].map((v) => Array.from({ length: 4 }, (_, i) => ((v >>> (i * 8)) & 255).toString(16).padStart(2, "0")).join("")).join("");
}
export async function sha(alg: "SHA-1" | "SHA-256" | "SHA-512", s: string) {
  const h = await crypto.subtle.digest(alg, enc.encode(s));
  return [...new Uint8Array(h)].map((b) => b.toString(16).padStart(2, "0")).join("");
}
export const uuid = () => crypto.randomUUID ? crypto.randomUUID() : "10000000-1000-4000-8000-100000000000".replace(/[018]/g, (c) => (Number(c) ^ (crypto.getRandomValues(new Uint8Array(1))[0] & (15 >> (Number(c) / 4)))).toString(16));

export function jwt(token: string) {
  const parts = token.trim().split(".");
  if (parts.length < 2) throw new Error(uiText("JWT 是用两个点隔开的三段"));
  const part = (s: string) => JSON.parse(b64decode(s));
  return { header: part(parts[0]), payload: part(parts[1]), signed: !!parts[2] };
}

/** Line diff (longest common subsequence): each line kept, removed or added. */
export function diff(a: string, b: string): { op: " " | "-" | "+"; text: string }[] {
  const x = a.split("\n"), y = b.split("\n"), n = x.length, m = y.length;
  if (n * m > 4_000_000) throw new Error(uiText("太长了：两边各几千行以内"));
  const L: Uint32Array[] = Array.from({ length: n + 1 }, () => new Uint32Array(m + 1));
  for (let i = n - 1; i >= 0; i--) for (let j = m - 1; j >= 0; j--) L[i][j] = x[i] === y[j] ? L[i + 1][j + 1] + 1 : Math.max(L[i + 1][j], L[i][j + 1]);
  const out: { op: " " | "-" | "+"; text: string }[] = [];
  let i = 0, j = 0;
  while (i < n && j < m) {
    if (x[i] === y[j]) { out.push({ op: " ", text: x[i] }); i++; j++; }
    else if (L[i + 1][j] >= L[i][j + 1]) out.push({ op: "-", text: x[i++] });
    else out.push({ op: "+", text: y[j++] });
  }
  while (i < n) out.push({ op: "-", text: x[i++] });
  while (j < m) out.push({ op: "+", text: y[j++] });
  return out;
}

export const cases = (s: string) => {
  const words = s.replace(/([a-z0-9])([A-Z])/g, "$1 $2").replace(/[_\-./]+/g, " ").trim().split(/\s+/).filter(Boolean).map((w) => w.toLowerCase());
  const cap = (w: string) => w.charAt(0).toUpperCase() + w.slice(1);
  return {
    camelCase: words.map((w, i) => (i ? cap(w) : w)).join(""), PascalCase: words.map(cap).join(""), snake_case: words.join("_"),
    "kebab-case": words.join("-"), CONSTANT_CASE: words.join("_").toUpperCase(), "dot.case": words.join("."), "Title Case": words.map(cap).join(" "),
  };
};

export function color(s: string) {
  const t = s.trim();
  let r = 0, g = 0, b = 0, a = 1;
  let m = /^#?([0-9a-f]{3,8})$/i.exec(t);
  if (m) {
    let h = m[1];
    if (h.length === 3 || h.length === 4) h = h.split("").map((c) => c + c).join("");
    if (h.length !== 6 && h.length !== 8) throw new Error(uiText("十六进制颜色是 3、4、6 或 8 位"));
    r = parseInt(h.slice(0, 2), 16); g = parseInt(h.slice(2, 4), 16); b = parseInt(h.slice(4, 6), 16); if (h.length === 8) a = parseInt(h.slice(6, 8), 16) / 255;
  } else if ((m = /^rgba?\(\s*(\d+)[\s,]+(\d+)[\s,]+(\d+)(?:[\s,/]+([\d.]+%?))?\s*\)$/i.exec(t))) {
    r = +m[1]; g = +m[2]; b = +m[3]; if (m[4]) a = m[4].endsWith("%") ? parseFloat(m[4]) / 100 : +m[4];
  } else if ((m = /^hsla?\(\s*([\d.]+)(?:deg)?[\s,]+([\d.]+)%[\s,]+([\d.]+)%(?:[\s,/]+([\d.]+%?))?\s*\)$/i.exec(t))) {
    const H = +m[1] / 360, S = +m[2] / 100, Lt = +m[3] / 100;
    const f = (n: number) => { const k = (n + H * 12) % 12, q = S * Math.min(Lt, 1 - Lt); return Math.round(255 * (Lt - q * Math.max(-1, Math.min(k - 3, 9 - k, 1)))); };
    r = f(0); g = f(8); b = f(4); if (m[4]) a = m[4].endsWith("%") ? parseFloat(m[4]) / 100 : +m[4];
  } else throw new Error(uiText("认得 #3370ff、rgb(51 112 255)、hsl(222 100% 60%)"));
  const hex = `#${[r, g, b].map((v) => v.toString(16).padStart(2, "0")).join("")}${a < 1 ? Math.round(a * 255).toString(16).padStart(2, "0") : ""}`;
  const R = r / 255, G = g / 255, B = b / 255, max = Math.max(R, G, B), min = Math.min(R, G, B), l = (max + min) / 2;
  let h = 0, s2 = 0;
  if (max !== min) { const d = max - min; s2 = l > 0.5 ? d / (2 - max - min) : d / (max + min); h = max === R ? (G - B) / d + (G < B ? 6 : 0) : max === G ? (B - R) / d + 2 : (R - G) / d + 4; h /= 6; }
  return { hex, rgb: `rgb(${r} ${g} ${b}${a < 1 ? ` / ${+a.toFixed(2)}` : ""})`, hsl: `hsl(${Math.round(h * 360)} ${Math.round(s2 * 100)}% ${Math.round(l * 100)}%${a < 1 ? ` / ${+a.toFixed(2)}` : ""})`, css: hex };
}
