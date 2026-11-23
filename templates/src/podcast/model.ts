// Shared records are curated feed suggestions; personal activity stays local.
import { Store } from "../_lib/myco";
import { Kit } from "../_lib/appkit";
export type Station = {
  name: string;
  feed_url: string;
  region: string;
  topic: string;
  [key: string]: unknown;
};
export const stations = new Store<Station>("stations");
export const kit = new Kit([stations], { quiet: true });
