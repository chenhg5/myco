// Only deliberately shared result summaries are page records. Private answer
// drafts and personal history never pass through these stores.
import { Store } from "../_lib/myco";
import { Kit } from "../_lib/appkit";
import type { SharedResult } from "./engine";
export const results = new Store<SharedResult & Record<string, unknown>>("results");
export const kit = new Kit([results], { quiet: true });
