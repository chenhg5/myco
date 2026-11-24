// Versioned writes shared by the forms and original-image import.
import { myco, type Store } from "../_lib/myco";
import { entries, images, kit, type Entry, type Picture } from "./model";

async function save<T extends Record<string, unknown>>(store: Store<T>, id: string, data: T, version: number) {
  const record = await myco!.data.put(store.collection, data, { id, ifVersion: version });
  store.apply({ seq: 0, collection: record.collection, record: record.id, op: "put", data: record.data, version: record.version });
  kit.emit();
  return record;
}

export const saveEntry = (id: string, data: Entry, version: number) => save(entries, id, data, version);
export const savePicture = (id: string, data: Picture, version: number) => save(images, id, data, version);
