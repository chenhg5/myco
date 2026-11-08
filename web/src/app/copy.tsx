import { useCallback, useEffect, useRef, useState } from "react";
import { Check, Copy } from "lucide-react";

/** Clipboard copy with an execCommand fallback for non-secure contexts. */
export async function copyText(value: string) {
  try {
    if (!navigator.clipboard?.writeText) throw new Error("clipboard unavailable");
    await navigator.clipboard.writeText(value);
  } catch {
    const ta = document.createElement("textarea");
    ta.value = value;
    ta.style.position = "fixed";
    ta.style.opacity = "0";
    document.body.appendChild(ta);
    ta.select();
    try { document.execCommand("copy"); } catch { /* ignore */ }
    document.body.removeChild(ta);
  }
}

export function useCopy(value: string) {
  const [copied, setCopied] = useState(false);
  const timer = useRef<number | null>(null);
  useEffect(() => () => { if (timer.current !== null) window.clearTimeout(timer.current); }, []);
  const copy = useCallback(async () => {
    await copyText(value);
    setCopied(true);
    if (timer.current !== null) window.clearTimeout(timer.current);
    timer.current = window.setTimeout(() => setCopied(false), 1500);
  }, [value]);
  return { copied, copy };
}

export function CopyButton({ value }: { value: string }) {
  const { copied, copy } = useCopy(value);
  return (
    <button type="button" className="app-btn app-btn-small" onClick={() => void copy()}>
      {copied ? <Check /> : <Copy />}{copied ? "已复制" : "复制"}
    </button>
  );
}
