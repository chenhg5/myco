// A page's icon: a line icon in a quiet colour (as Notion's own icons), or
// an emoji. Stored in `icon` as "i:<name>:<tint>" (i:rocket:clay) or as
// the emoji itself.
import type { LucideIcon } from "lucide-react";
import {
  Archive, BookOpen, Bookmark, Box, Briefcase, Calendar, ChartBar, CheckSquare, Clipboard, Clock, Code, Coffee, Compass, Cpu, Database,
  FileText, Flag, Flame, FlaskConical, Folder, Gem, Globe, GraduationCap, Hand, Heart, Home, Image, Inbox, Key, Landmark, Layers, Leaf,
  Lightbulb, Link, ListChecks, Lock, Mail, Map, MessageCircle, Mic, Music, Notebook, Palette, PenLine, Pin, Plane, Puzzle, Rocket,
  Scale, Search, Settings, Shield, ShoppingBag, Sparkles, Star, Sun, Target, Terminal, Trophy, Users, Wrench, Zap,
} from "lucide-react";

export const GLYPHS: Record<string, LucideIcon> = {
  doc: FileText, notebook: Notebook, book: BookOpen, pen: PenLine, clipboard: Clipboard, checklist: ListChecks, check: CheckSquare, bookmark: Bookmark,
  pin: Pin, flag: Flag, target: Target, calendar: Calendar, clock: Clock, inbox: Inbox, mail: Mail, chat: MessageCircle,
  users: Users, hand: Hand, home: Home, briefcase: Briefcase, landmark: Landmark, globe: Globe, map: Map, compass: Compass,
  plane: Plane, rocket: Rocket, lightbulb: Lightbulb, sparkles: Sparkles, zap: Zap, flame: Flame, star: Star, heart: Heart,
  trophy: Trophy, gem: Gem, chart: ChartBar, database: Database, layers: Layers, box: Box, archive: Archive, folder: Folder,
  code: Code, terminal: Terminal, cpu: Cpu, settings: Settings, wrench: Wrench, flask: FlaskConical, puzzle: Puzzle, key: Key,
  lock: Lock, shield: Shield, scale: Scale, search: Search, link: Link, image: Image, palette: Palette, music: Music,
  mic: Mic, coffee: Coffee, leaf: Leaf, sun: Sun, cap: GraduationCap, bag: ShoppingBag,
};
/** The tints, quiet (notes.css has them, lighter in the dark). */
export const TINTS = ["gray", "stone", "sage", "mist", "clay", "ochre", "plum"] as const;

export type Icon = { kind: "glyph"; name: string; tint: string; G: LucideIcon } | { kind: "emoji"; ch: string } | null;
export function parseIcon(v: unknown): Icon {
  const s = String(v || "").trim();
  if (!s) return null;
  if (s.startsWith("i:")) {
    const [, name, tint] = s.split(":");
    if (!Object.prototype.hasOwnProperty.call(GLYPHS, name)) return null;
    const G = GLYPHS[name];
    return { kind: "glyph", name, tint: (TINTS as readonly string[]).includes(tint) ? tint : "gray", G };
  }
  return { kind: "emoji", ch: s };
}

/** An icon at a size (a row: 16; the page's own: 64). */
export function IconView({ v, size = 16 }: { v: unknown; size?: number }) {
  const ic = parseIcon(v);
  if (!ic) return <FileText className="nt-doc" width={size - 1} height={size - 1} strokeWidth={1.75} />;
  if (ic.kind === "emoji") return <span className="nt-emoji" style={{ fontSize: size - 1, width: size, height: size }}>{ic.ch}</span>;
  const G = ic.G;
  return <G className="nt-glyph" width={size} height={size} strokeWidth={size > 32 ? 1.4 : 1.8} style={{ color: `var(--tint-${ic.tint})` }} />;
}
