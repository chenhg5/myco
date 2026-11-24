// Palette content and shared favorites are independent CAS records.
import { Store } from "../_lib/myco";
import { Kit } from "../_lib/appkit";
import { validPalette, type Palette } from "./engine";
export const palettes = new Store<Palette>("palettes");
export const favorites = new Store<Record<string, unknown> & { saved: boolean }>("favorites");
export const kit = new Kit([palettes, favorites], { quiet: true });
export const rows = () => palettes.rows.filter(r => validPalette(r.data)).sort((a, b) => String(b.data.updated_at).localeCompare(String(a.data.updated_at)) || a.id.localeCompare(b.id));
export const isSaved = (id: string) => favorites.get(id)?.data.saved === true;
