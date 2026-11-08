import type { HubOverview } from "@/appapi";
import type { AgentStatus } from "@/api";

/** One agent together with the hub it was reported by. */
export interface AgentRow {
  hub: HubOverview;
  st: AgentStatus;
}
