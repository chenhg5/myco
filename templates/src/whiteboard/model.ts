// Shared board records and their normalized, backwards-compatible projections.
import { Store } from "../_lib/myco";
import { Kit, titleSaver } from "../_lib/appkit";
import { boxes, type Item } from "./geometry";
export const board = new Store<{ title?: string }>("board");
export const items = new Store<Item>("items");
export const kit = new Kit([board, items]);
export const title = titleSaver(kit, board);
export const currentBoxes = () => boxes(items.rows);
export const maxZ = () =>
  items.rows.reduce((z, r) => Math.max(z, Number(r.data.z) || 0), 0);
