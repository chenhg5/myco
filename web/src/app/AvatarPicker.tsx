import { useEffect, useRef, useState } from "react";
import { Check, ImagePlus, RotateCcw } from "lucide-react";
import type { Agent } from "@/api";
import { appApi } from "@/appapi";
import { ErrorText, Modal } from "@/app/dialogs";
import { accessoryFits, Avatar, isWorkspace, LOOK_PARTS, lookOf, lookWord, MushroomFace, parseLook, pictureOf, type Look } from "@/app/persona";
import { L, E } from "@/lib/i18n";
import { cn } from "@/lib/utils";

/*
 * Choosing a face. An agent can have a mushroom of its own (a colour, a
 * shape, spots and a face picked from grids, one thing to wear) or a picture; a computer or a
 * person a picture. The face is kept on the workspace, so everyone there
 * sees the same one. A picture is cut to a square from its middle and made
 * small here, before it is sent.
 */

/** A picture made into a small square, as a data: URL (WebP where the
 *  system can write it, else PNG, or JPEG on white when PNG is too big). */
export async function squarePicture(file: Blob, size = 256): Promise<string> {
  const url = URL.createObjectURL(file);
  try {
    const img = await new Promise<HTMLImageElement>((resolve, reject) => {
      const i = new Image();
      i.onload = () => resolve(i);
      i.onerror = () => reject(new Error(L("这个文件打不开，换一张图片试试", "This file cannot be opened; try another picture")));
      i.src = url;
    });
    const side = Math.min(img.naturalWidth, img.naturalHeight);
    if (!side) throw new Error(L("这张图片是空的", "This picture is empty"));
    const out = Math.min(size, side);
    const canvas = document.createElement("canvas");
    canvas.width = canvas.height = out;
    const g = canvas.getContext("2d");
    if (!g) throw new Error(L("没法处理图片", "Cannot work on the picture"));
    g.imageSmoothingQuality = "high";
    const sx = (img.naturalWidth - side) / 2;
    const sy = (img.naturalHeight - side) / 2;
    g.drawImage(img, sx, sy, side, side, 0, 0, out, out);
    let data = canvas.toDataURL("image/webp", 0.86);
    if (!data.startsWith("data:image/webp")) data = canvas.toDataURL("image/png");
    if (data.length > 160_000) {
      g.globalCompositeOperation = "destination-over";
      g.fillStyle = "#fff";
      g.fillRect(0, 0, out, out);
      data = canvas.toDataURL("image/jpeg", 0.86);
    }
    return data;
  } finally {
    URL.revokeObjectURL(url);
  }
}

/** The desktop app (macOS) has its own panel that shows pictures only:
 *  its web view's panel for a file input lets any file be chosen. */
let systemPicker = false;
export function setSystemPicker(on: boolean) { systemPicker = on; }

const PICTURE_TYPES = ["image/png", "image/jpeg", "image/webp", "image/gif"];
const PICTURE_EXT = /\.(png|jpe?g|webp|gif)$/i;
const notPicture = () => new Error(L("只能用图片：PNG、JPEG、WebP 或 GIF", "Only a picture can be used: PNG, JPEG, WebP or GIF"));

/** A button that asks for a picture and hands it back made small. Only
 *  pictures can be chosen; anything else is turned away with a word. */
export function PictureButton({ onPicture, onError, label }: {
  onPicture: (dataUrl: string) => void;
  onError: (e: unknown) => void;
  label?: string;
}) {
  const input = useRef<HTMLInputElement>(null);
  const pick = async () => {
    if (!systemPicker) { input.current?.click(); return; }
    try {
      const r = await appApi.pickImage();
      if (r.cancelled || !r.data) return;
      const bytes = Uint8Array.from(atob(r.data), (c) => c.charCodeAt(0));
      onPicture(await squarePicture(new Blob([bytes], { type: r.type })));
    } catch (e) {
      const code = (e as { code?: string })?.code;
      if (code === "not_supported") { systemPicker = false; input.current?.click(); return; }
      onError(code === "not_image" ? notPicture() : e);
    }
  };
  return (
    <>
      <button type="button" className="app-btn" onClick={() => void pick()}>
        <ImagePlus />{label || L("用一张照片", "Use a picture")}
      </button>
      <input ref={input} type="file" accept={[...PICTURE_TYPES, ".png", ".jpg", ".jpeg", ".webp", ".gif"].join(",")} hidden
        onChange={(e) => {
          const f = e.target.files?.[0];
          e.target.value = "";
          if (!f) return;
          if (!PICTURE_TYPES.includes(f.type) && !PICTURE_EXT.test(f.name)) { onError(notPicture()); return; }
          squarePicture(f).then(onPicture, onError);
        }} />
    </>
  );
}

type Choice =
  | { kind: "default" }
  | { kind: "look"; look: Look }
  | { kind: "pic"; data: string }
  /** the picture it has now, unchanged */
  | { kind: "kept" };

function wordOf(c: Choice, agent: Agent): string | undefined {
  if (c.kind === "default") return "";
  if (c.kind === "look") return lookWord(c.look);
  if (c.kind === "pic") return c.data;
  return agent.avatar;
}

/** The face of an agent as it would look with a choice. */
function Preview({ agent, choice, size }: { agent: Agent; choice: Choice; size: number }) {
  if (choice.kind === "pic") {
    return <span className="pa has-pic" style={{ width: size, height: size }}><img className="pa-pic" src={choice.data} alt="" /></span>;
  }
  const avatar = choice.kind === "look" ? lookWord(choice.look) : choice.kind === "kept" ? agent.avatar : undefined;
  return <Avatar agent={{ ...agent, avatar }} size={size} badge={false} />;
}

/** Choose an agent's (or a computer's) face; it is kept on the workspace. */
export function AvatarDialog({ open, onOpenChange, hubId, agent, onSaved }: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  hubId: string;
  agent: Agent;
  onSaved: () => void;
}) {
  const drawn = !isWorkspace(agent) && agent.kind !== "human" && agent.binding.harness !== "human";
  const seed = agent.id || agent.name;
  const [choice, setChoice] = useState<Choice>({ kind: "default" });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);

  // Start from the face it has, each time the dialog opens.
  useEffect(() => {
    if (!open) return;
    const look = parseLook(agent.avatar);
    setChoice(look ? { kind: "look", look } : pictureOf(agent) ? { kind: "kept" } : { kind: "default" });
    setError(null);
    setBusy(false);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  // The parts the grid builds on: the chosen ones, else the face it has.
  const base = choice.kind === "look" ? choice.look : lookOf(seed);
  // (what it wears comes off when the new shape has no room for it)
  const pick = (l: Partial<Look>) => {
    const next = { ...base, ...l };
    if (!accessoryFits(next.acc, next.shape)) next.acc = 0;
    next.sprout = next.acc === 1;
    setChoice({ kind: "look", look: next });
  };
  const cell = (look: Look) => (
    <span className="pa" style={{ width: 40, height: 40, ["--av-cap" as string]: LOOK_PARTS.caps[look.cap] }}>
      <MushroomFace seed={seed} avatar={lookWord(look)} />
    </span>
  );
  const unchanged = (wordOf(choice, agent) || "") === (agent.avatar || "");

  const save = async () => {
    setBusy(true);
    setError(null);
    try {
      await appApi.updateProfile(hubId, agent.name, { avatar: wordOf(choice, agent) ?? "" });
      onSaved();
      onOpenChange(false);
    } catch (e) {
      const msg = (e as Error)?.message || "";
      setError(/unknown field/i.test(msg)
        ? new Error(L("这个工作区的 myco 版本太旧，还不能换头像。升级工作区后再试。", "This workspace's myco is too old to change faces. Update the workspace and try again."))
        : e);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal open={open} onOpenChange={onOpenChange} className="av-modal"
      title={L("换个头像", "Change the face")}
      description={L("工作区里的人看到的都是这个头像。", "Everyone in the workspace sees this face.")}>
      <div className="av-top">
        <Preview agent={agent} choice={choice} size={72} />
        <div className="av-top-actions">
          <PictureButton onPicture={(data) => setChoice({ kind: "pic", data })} onError={setError} />
          {choice.kind !== "default" && (
            <button type="button" className="app-btn app-btn-quiet" onClick={() => setChoice({ kind: "default" })}>
              <RotateCcw />{L("用回原来的", "Back to the original")}
            </button>
          )}
        </div>
      </div>

      {drawn && (
        <div className="av-draw">
          <div className="av-label">{L("或者，挑一个蘑菇", "Or pick a mushroom")}</div>
          <div className="av-colors" role="radiogroup" aria-label={L("颜色", "Colour")}>
            {LOOK_PARTS.caps.map((c, i) => {
              const on = choice.kind === "look" && base.cap === i;
              return (
                <button key={c} type="button" role="radio" aria-checked={on} className={cn("av-color", on && "is-on")}
                  style={{ background: c }} aria-label={L(E`颜色 ${i + 1}`, E`Colour ${i + 1}`)} onClick={() => pick({ cap: i })}>
                  {on && <Check />}
                </button>
              );
            })}
          </div>
          <div className="av-label">{L("形状", "Shape")}</div>
          <div className="av-grid" role="radiogroup" aria-label={L("形状", "Shape")} data-group="shape">
            {Array.from({ length: LOOK_PARTS.shapes }, (_, shape) => {
              const on = choice.kind === "look" && base.shape === shape;
              const acc = accessoryFits(base.acc, shape) ? base.acc : 0;
              return (
                <button key={shape} type="button" role="radio" aria-checked={on} className={cn("av-cell", on && "is-on")}
                  aria-label={L(E`形状 ${shape + 1}`, E`Shape ${shape + 1}`)} onClick={() => pick({ shape })}>
                  {cell({ ...base, shape, acc, sprout: acc === 1 })}
                </button>
              );
            })}
          </div>
          <div className="av-label">{L("花纹和表情", "Spots and face")}</div>
          <div className="av-grid" role="radiogroup" aria-label={L("花纹和表情", "Spots and face")} data-group="face">
            {Array.from({ length: LOOK_PARTS.patterns * LOOK_PARTS.faces }, (_, k) => {
              const pattern = Math.floor(k / LOOK_PARTS.faces);
              const face = k % LOOK_PARTS.faces;
              const on = choice.kind === "look" && base.pattern === pattern && base.face === face;
              return (
                <button key={k} type="button" role="radio" aria-checked={on} className={cn("av-cell", on && "is-on")}
                  aria-label={L(E`花纹和表情 ${k + 1}`, E`Spots and face ${k + 1}`)} onClick={() => pick({ pattern, face })}>
                  {cell({ ...base, pattern, face })}
                </button>
              );
            })}
          </div>
          <div className="av-label">{L("戴点什么（最多一件）", "Something to wear (one at most)")}</div>
          <div className="av-grid is-acc" role="radiogroup" aria-label={L("配饰", "Accessory")} data-group="acc">
            {LOOK_PARTS.accessories.map((name, acc) => {
              const fits = accessoryFits(acc, base.shape);
              const on = choice.kind === "look" && base.acc === acc;
              return (
                <button key={name} type="button" role="radio" aria-checked={on} disabled={!fits} className={cn("av-cell", on && "is-on")}
                  aria-label={accessoryLabel(name)} title={fits ? accessoryLabel(name) : L(E`${accessoryLabel(name)}：这个形状戴不上`, E`${accessoryLabel(name)}: does not fit this shape`)}
                  onClick={() => pick({ acc })}>
                  {cell({ ...base, acc, sprout: acc === 1 })}
                </button>
              );
            })}
          </div>
        </div>
      )}

      <ErrorText error={error} />
      <div className="app-modal-actions">
        <button type="button" className="app-btn" onClick={() => onOpenChange(false)}>{L("取消", "Cancel")}</button>
        <button type="button" className="app-btn app-btn-primary" disabled={busy || unchanged} onClick={() => void save()}>
          {L("用这个头像", "Use this face")}
        </button>
      </div>
    </Modal>
  );
}

/** What an accessory is called. */
function accessoryLabel(name: string): string {
  switch (name) {
    case "none": return L("不戴", "Nothing");
    case "sprout": return L("小芽", "A sprout");
    case "leaf": return L("小叶子", "A leaf");
    case "bow": return L("蝴蝶结", "A bow");
    case "headphones": return L("耳机", "Headphones");
    case "scarf": return L("围巾", "A scarf");
    case "star": return L("小星星", "A star");
    case "flower": return L("小花", "A flower");
    default: return name;
  }
}
