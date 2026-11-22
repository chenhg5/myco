// IndexedDB is the source of truth. A write succeeds only after the transaction
// commits. Page and member names partition libraries on the same browser origin.
import type { Book, Settings, StoredBook } from "./model";
import { decodeText } from "./model";
export class Library {
  private database: Promise<IDBDatabase>;
  constructor(readonly scope: string) {
    this.database = new Promise((resolve, reject) => {
      const r = indexedDB.open("myco-reader", 1);
      r.onupgradeneeded = () => {
        r.result.createObjectStore("books");
        r.result.createObjectStore("settings");
      };
      r.onsuccess = () => resolve(r.result);
      r.onerror = () => reject(r.error);
      r.onblocked = () => reject(new Error("storage_blocked"));
    });
  }
  private async transact<T>(
    store: string,
    mode: IDBTransactionMode,
    run: (s: IDBObjectStore) => IDBRequest<T>,
  ): Promise<T> {
    const db = await this.database;
    return new Promise((resolve, reject) => {
      const tx = db.transaction(store, mode),
        req = run(tx.objectStore(store));
      tx.oncomplete = () => resolve(req.result);
      tx.onabort = () =>
        reject(tx.error || req.error || new Error("storage_failed"));
      tx.onerror = () => {
        /* onabort carries the failure, never report a successful request as a commit. */
      };
    });
  }
  private key(id: string) {
    return `${this.scope}/${id}`;
  }
  async list(): Promise<Book[]> {
    const all = await this.transact<StoredBook[]>("books", "readonly", (s) =>
      s.getAll(IDBKeyRange.bound(this.key(""), this.key("\uffff"))),
    );
    return all
      .map(({ file: _file, ...book }) => book)
      .sort((a, b) => (b.opened || b.added) - (a.opened || a.added));
  }
  get(id: string): Promise<StoredBook | undefined> {
    return this.transact("books", "readonly", (s) => s.get(this.key(id)));
  }
  // Read-modify-write in one transaction; independently saved reading positions,
  // covers and bookmarks cannot overwrite each other with an old file object.
  async patch(
    id: string,
    fields: Partial<Omit<StoredBook, "id" | "file">>,
  ): Promise<void> {
    const db = await this.database;
    return new Promise((resolve, reject) => {
      const tx = db.transaction("books", "readwrite"),
        store = tx.objectStore("books"),
        req = store.get(this.key(id));
      req.onsuccess = () => {
        if (req.result) store.put({ ...req.result, ...fields }, this.key(id));
      };
      tx.oncomplete = () => resolve();
      tx.onabort = () => reject(tx.error || new Error("storage_failed"));
    });
  }
  remove(id: string) {
    return this.transact("books", "readwrite", (s) => s.delete(this.key(id)));
  }
  settings(): Promise<Partial<Settings> | undefined> {
    return this.transact("settings", "readonly", (s) => s.get(this.scope));
  }
  saveSettings(s: Settings) {
    return this.transact("settings", "readwrite", (store) =>
      store.put(s, this.scope),
    );
  }
  async import(file: File): Promise<Book> {
    const format = /\.pdf$/i.test(file.name)
      ? "pdf"
      : /\.txt$/i.test(file.name)
        ? "txt"
        : null;
    if (!format) throw new Error("file_type");
    if (!file.size || file.size > (format === "pdf" ? 100 : 5) * 1024 * 1024)
      throw new Error("file_size");
    const bytes = await file.arrayBuffer();
    if (format === "txt") decodeText(bytes);
    else if (!new TextDecoder().decode(bytes.slice(0, 1024)).includes("%PDF-"))
      throw new Error("invalid_pdf");
    // Hash all bytes, not only a prefix. Secure contexts use SHA-256; older
    // local webviews get a full-file double hash rather than filename identity.
    let id: string;
    if (crypto.subtle)
      id = [...new Uint8Array(await crypto.subtle.digest("SHA-256", bytes))]
        .map((v) => v.toString(16).padStart(2, "0"))
        .join("");
    else {
      let a = 2166136261,
        b = 0x9e3779b9;
      for (const n of new Uint8Array(bytes)) {
        a = Math.imul(a ^ n, 16777619);
        b = Math.imul(b ^ n, 2246822519);
      }
      id = `${file.size}-${a >>> 0}-${b >>> 0}`;
    }
    const existing = await this.get(id);
    if (existing) {
      const { file: _file, ...book } = existing;
      return book;
    }
    const book: Book = {
      id,
      title: file.name.replace(/\.(pdf|txt)$/i, ""),
      name: file.name,
      format,
      size: file.size,
      added: Date.now(),
      opened: 0,
      position: 0,
      progress: 0,
      bookmarks: [],
    };
    try {
      await this.transact("books", "readwrite", (s) =>
        s.add({ ...book, file }, this.key(id)),
      );
    } catch (error) {
      // Another tab may have imported this exact file while it was hashing.
      // Never replace that tab's progress or bookmarks with a fresh entry.
      if ((error as DOMException)?.name !== "ConstraintError") throw error;
      const actual = await this.get(id);
      if (!actual) throw error;
      const { file: _file, ...saved } = actual;
      return saved;
    }
    return book;
  }
}
