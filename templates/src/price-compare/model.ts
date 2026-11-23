// Shared agent research and human decisions live in separate collections.
import { Store } from "../_lib/myco";
import { Kit } from "../_lib/appkit";
import type { Comparison } from "./engine";
export type Decision = Record<string, unknown> & { saved: string[]; note: string };
export const comparisons = new Store<Comparison & Record<string, unknown>>("comparisons");
export const decisions = new Store<Decision>("decisions");
export const kit = new Kit([comparisons, decisions], { quiet: true });
