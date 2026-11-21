// 空间清理's data: what was cleaned (when, on which computer, how much),
// for everyone in the page to see. What is on the disk is never stored:
// it is read from the computer each time.
import { Store } from "../_lib/myco";
import { Kit } from "../_lib/appkit";
import type { OSScanNode } from "../_lib/myco";

export type Cleanup = { at?: string; computer?: string; disk?: string; freed?: number; count?: number; by?: string };
export const cleanups = new Store<Cleanup & Record<string, unknown>>("cleanups");
export const kit = new Kit([cleanups], { quiet: true });

/** A scanned tree, with each node's parent (to go up, and to fix totals). */
export class Tree {
  parent = new Map<OSScanNode, OSScanNode | null>();
  byPath = new Map<string, OSScanNode>();
  constructor(readonly root: OSScanNode) { this.index(root, null); }
  index(n: OSScanNode, p: OSScanNode | null) {
    this.parent.set(n, p);
    this.byPath.set(n.path, n);
    for (const k of n.children || []) this.index(k, n);
  }
  /** The node and its parents, from the root. */
  chain(n: OSScanNode): OSScanNode[] {
    const out: OSScanNode[] = [];
    for (let c: OSScanNode | null | undefined = n; c; c = this.parent.get(c)) out.unshift(c);
    return out;
  }
  /** The nearest node at or above a path (an item folded into "smaller items" has none of its own). */
  nearest(path: string): OSScanNode | null {
    for (let p = path; p; ) {
      const n = this.byPath.get(p);
      if (n) return n;
      const i = Math.max(p.lastIndexOf("/"), p.lastIndexOf("\\"));
      if (i <= 0) break;
      p = p.slice(0, i);
    }
    return this.byPath.get(this.root.path) || null;
  }
  /** Replaces a folder's children with a finer scan of it. */
  graft(n: OSScanNode, fresh: OSScanNode) {
    n.children = fresh.children;
    n.more = fresh.more; n.more_size = fresh.more_size; n.more_disk = fresh.more_disk; n.more_files = fresh.more_files;
    for (const k of n.children || []) this.index(k, n);
  }
  /** Takes away what was moved to the trash: the node goes, its parents shrink. */
  remove(path: string, disk: number, size: number, files: number) {
    const own = this.byPath.get(path);
    const at = own ? this.parent.get(own) : this.nearest(path);
    if (own && at) {
      at.children = (at.children || []).filter((k) => k !== own);
      this.byPath.delete(path);
    } else if (at && !own) {
      // (it was among the smaller items)
      at.more_disk = Math.max(0, (at.more_disk || 0) - disk);
      at.more_size = Math.max(0, (at.more_size || 0) - size);
      at.more_files = Math.max(0, (at.more_files || 0) - files);
    }
    for (let c: OSScanNode | null | undefined = at; c; c = this.parent.get(c)) {
      c.disk = Math.max(0, c.disk - disk);
      c.size = Math.max(0, c.size - size);
      c.files = Math.max(0, c.files - files);
    }
  }
}
