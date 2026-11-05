import { useState } from "react";
import { Check, Copy } from "lucide-react";
import { cn } from "@/lib/utils";

export function CopyText({ text, display, className }: { text: string; display?: string; className?: string }) {
  const [done, setDone] = useState(false);
  return (
    <button
      type="button"
      onClick={(e) => {
        e.stopPropagation();
        navigator.clipboard?.writeText(text).then(() => {
          setDone(true);
          setTimeout(() => setDone(false), 1200);
        });
      }}
      className={cn("group inline-flex max-w-full items-center gap-1 rounded font-mono text-xs text-muted-foreground transition hover:text-foreground", className)}
      title={text}
    >
      <span className="truncate">{display ?? text}</span>
      {done ? <Check className="h-3 w-3 shrink-0 text-working" /> : <Copy className="h-3 w-3 shrink-0 opacity-0 transition group-hover:opacity-100" />}
    </button>
  );
}
