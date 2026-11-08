import { Plus, Server, Trash2, TriangleAlert } from "lucide-react";
import type { Hub, HubOverview } from "@/appapi";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

function host(url: string) {
  try { return new URL(url).host || url; } catch { return url; }
}

export function HubsSidebar({ hubs, ov, selected, onSelect, onAdd, onRemove }: {
  hubs: Hub[];
  ov: HubOverview[] | undefined;
  selected: string;
  onSelect: (id: string) => void;
  onAdd: () => void;
  onRemove: (hub: Hub) => void;
}) {
  const counts = new Map((ov || []).map((h) => [h.id, h.overview?.agents.length]));
  return (
    <aside className="flex shrink-0 flex-col border-b md:w-60 md:border-b-0 md:border-r">
      <div className="flex items-center justify-between px-4 pb-2 pt-4">
        <span className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">Hubs</span>
        <span className="text-xs tabular-nums text-muted-foreground">{hubs.length}</span>
      </div>
      <div className="flex gap-2 overflow-x-auto px-3 pb-3 scrollbar-thin md:flex-1 md:flex-col md:overflow-y-auto md:overflow-x-hidden">
        <button
          type="button"
          onClick={() => onSelect("all")}
          className={cn(
            "flex shrink-0 items-center gap-2 rounded-lg border px-3 py-2 text-left text-sm transition-colors md:w-full",
            selected === "all" ? "border-foreground/20 bg-muted/60" : "border-transparent hover:bg-muted/40",
          )}
        >
          <Server className="h-4 w-4 text-muted-foreground" />
          <span className="font-medium">All hubs</span>
        </button>
        {hubs.map((h) => {
          const active = selected === h.id;
          return (
            <div
              key={h.id}
              className={cn(
                "group flex w-56 shrink-0 items-start gap-2 rounded-lg border px-3 py-2 transition-colors md:w-full",
                active ? "border-foreground/20 bg-muted/60" : "border-transparent hover:bg-muted/40",
              )}
            >
              <button type="button" onClick={() => onSelect(h.id)} className="flex min-w-0 flex-1 items-start gap-2 text-left">
                <span className={cn("mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full", h.ok ? "bg-working" : "bg-destructive")} />
                <span className="min-w-0 flex-1">
                  <span className="flex items-center gap-1.5">
                    <span className="truncate text-sm font-medium">{h.name}</span>
                    {h.update_available && (
                      <span className="shrink-0 rounded-full border border-brand/40 bg-brand/10 px-1.5 text-[10px] font-medium text-brand">update</span>
                    )}
                  </span>
                  <span className="block truncate font-mono text-[11px] text-muted-foreground">{host(h.url)}</span>
                  <span className="mt-0.5 flex items-center gap-1.5 text-[11px] text-muted-foreground">
                    {h.ok ? (
                      <>
                        <span className="truncate">{h.hub_version ? `v${h.hub_version}` : "connected"}</span>
                        {counts.get(h.id) !== undefined && <span className="tabular-nums">· {counts.get(h.id)} agents</span>}
                      </>
                    ) : (
                      <span className="inline-flex min-w-0 items-center gap-1 text-destructive">
                        <TriangleAlert className="h-3 w-3 shrink-0" /><span className="truncate">{h.error || "unreachable"}</span>
                      </span>
                    )}
                  </span>
                </span>
              </button>
              <button
                type="button"
                onClick={() => onRemove(h)}
                aria-label={`Remove ${h.name}`}
                title="Remove hub"
                className="rounded p-1 text-muted-foreground/60 opacity-0 transition hover:bg-accent hover:text-destructive focus:opacity-100 group-hover:opacity-100"
              >
                <Trash2 className="h-3.5 w-3.5" />
              </button>
            </div>
          );
        })}
      </div>
      <div className="hidden border-t p-3 md:block">
        <Button variant="outline" size="sm" className="w-full" onClick={onAdd}>
          <Plus /> Add hub
        </Button>
      </div>
    </aside>
  );
}
