import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";
import { L } from "@/lib/i18n";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

export function timeAgo(iso?: string, now = Date.now()): string {
  if (!iso) return L("从未", "never");
  const s = Math.max(0, Math.round((now - new Date(iso).getTime()) / 1000));
  if (s < 5) return L("刚刚", "just now");
  if (s < 60) return L(`${s} 秒前`, `${s}s ago`);
  const m = Math.round(s / 60);
  if (m < 60) return L(`${m} 分钟前`, `${m} min ago`);
  const h = Math.round(m / 60);
  if (h < 48) return L(`${h} 小时前`, `${h} h ago`);
  const d = Math.round(h / 24);
  return L(`${d} 天前`, d === 1 ? "1 day ago" : `${d} days ago`);
}

export function clock(iso: string): string {
  const d = new Date(iso);
  return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });
}

export function bytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}
