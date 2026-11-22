// Clipboard events keep the selected graph (including its internal connectors)
// and readable plain text. Menu actions also work in desktop App webviews.
import { myco } from "../_lib/myco";
import { clipboardPrefix, parseClipboard, type Entry } from "./geometry";
const mime = "application/x-myco-whiteboard+json";
let memory = "",
  plain = "";
export function encode(rows: Entry[]) {
  return clipboardPrefix + JSON.stringify(rows);
}
export function fillClipboard(data: DataTransfer, rows: Entry[]) {
  memory = encode(rows);
  plain = rows
    .filter((r) => r.data.type !== "arrow")
    .map((r) => r.data.text || "")
    .join("\n");
  data.setData(mime, memory);
  data.setData("text/plain", plain);
  data.setData(
    "text/html",
    `<div data-myco-whiteboard="${encodeURIComponent(memory)}">${plain.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;")}</div>`,
  );
}
export function readClipboard(data: DataTransfer) {
  let graph = data.getData(mime);
  if (!graph) {
    const doc = new DOMParser().parseFromString(
        data.getData("text/html"),
        "text/html",
      ),
      encoded = doc
        .querySelector("[data-myco-whiteboard]")
        ?.getAttribute("data-myco-whiteboard");
    if (encoded)
      try {
        graph = decodeURIComponent(encoded);
      } catch {
        /* fall back to text */
      }
  }
  const text = data.getData("text/plain");
  return {
    rows:
      parseClipboard(graph || text) ||
      (text === plain ? parseClipboard(memory) : null),
    text,
  };
}
export async function write(rows: Entry[]): Promise<boolean> {
  memory = encode(rows);
  plain = rows
    .filter((r) => r.data.type !== "arrow")
    .map((r) => r.data.text || "")
    .join("\n");
  if (myco?.os?.clipboard) {
    try {
      await myco.os.clipboard.write({ text: memory });
      return true;
    } catch {
      /* try browser */
    }
  }
  try {
    if (navigator.clipboard?.write && typeof ClipboardItem !== "undefined") {
      const text = new DataTransfer();
      fillClipboard(text, rows);
      await navigator.clipboard.write([
        new ClipboardItem({
          "text/plain": new Blob([text.getData("text/plain")], {
            type: "text/plain",
          }),
          "text/html": new Blob([text.getData("text/html")], {
            type: "text/html",
          }),
        }),
      ]);
      return true;
    }
  } catch {
    /* try the webview's native copy command */
  }
  const before = document.activeElement as HTMLElement | null;
  const field = document.createElement("textarea");
  field.readOnly = true;
  field.value = memory;
  field.style.cssText = "position:fixed;left:-9999px;top:0;font-size:16px";
  const copied = (event: ClipboardEvent) => {
    if (event.clipboardData) {
      event.preventDefault();
      event.stopImmediatePropagation();
      fillClipboard(event.clipboardData, rows);
    }
  };
  document.addEventListener("copy", copied, true);
  document.body.append(field);
  field.focus();
  field.select();
  try {
    return document.execCommand("copy");
  } finally {
    document.removeEventListener("copy", copied, true);
    field.remove();
    before?.focus({ preventScroll: true });
  }
}
export async function read() {
  if (myco?.os?.clipboard) {
    try {
      const { text } = await myco.os.clipboard.read();
      return {
        rows:
          parseClipboard(text) ||
          (text === plain ? parseClipboard(memory) : null),
        text,
      };
    } catch {
      /* try browser */
    }
  }
  if (navigator.clipboard?.read) {
    const entries = await navigator.clipboard.read();
    for (const entry of entries) {
      const data = new DataTransfer();
      for (const type of ["text/html", "text/plain"])
        if (entry.types.includes(type))
          data.setData(type, await (await entry.getType(type)).text());
      return readClipboard(data);
    }
  }
  const text = await navigator.clipboard.readText();
  return {
    rows:
      parseClipboard(text) || (text === plain ? parseClipboard(memory) : null),
    text,
  };
}
