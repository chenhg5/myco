// Only deliberately shared query snippets live on the page. Local connections,
// passwords, query history and database result rows never use this collection.
import { Store } from "../_lib/myco";
import { Kit } from "../_lib/appkit";
import type { Engine } from "./client";
export type Query = Record<string, unknown> & { title: string; engine: Engine; sql: string; note: string };
export const queries = new Store<Query>("queries");
export const kit = new Kit([queries], { quiet: true });
