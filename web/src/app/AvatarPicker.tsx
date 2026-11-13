import { useEffect, useRef, useState } from "react";
import { Check, ImagePlus, RotateCcw } from "lucide-react";
import type { Agent } from "@/api";
import { appApi } from "@/appapi";
import { ErrorText, Modal } from "@/app/dialogs";
import { Avatar, isWorkspace, LOOK_PARTS, lookOfSeed, lookWord, MushroomFace, parseLook, pictureOf, type Look } from "@/app/persona";
import { L } from "@/lib/i18n";
import { cn } from "@/lib/utils";

/*
 * Choosing a face. An agent can have a mushroom of its own (a colour and a
 * shape picked from a grid, a sprout or not) or a picture; a computer or a
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

/** A button that asks for a picture and hands it back made small. */
export function PictureButton({ onPicture, onError, label }: {
  onPicture: (dataUrl: string) => void;
  onError: (e: unknown) => void;
  label?: string;
}) {
  const input = useRef<HTMLInputElement>(null);
  return (
    <>
      <button type="button" className="app-btn" onClick={() => input.current?.click()}>
        <ImagePlus />{label || L("用一张照片", "Use a picture")}
      </button>
      <input ref={input} type="file" accept="image/png,image/jpeg,image/webp,image/gif" hidden
        onChange={(e) => {
          const f = e.target.files?.[0];
          e.target.value = "";
          if (!f) return;
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

  // The parts the grid builds on: the chosen ones, else its own.
  const base = choice.kind === "look" ? choice.look : lookOfSeed(seed);
  const pick = (l: Partial<Look>) => setChoice({ kind: "look", look: { ...base, ...l } });
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
                  style={{ background: c }} aria-label={L(`颜色 ${i + 1}`, `Colour ${i + 1}`)} onClick={() => pick({ cap: i })}>
                  {on && <Check />}
                </button>
              );
            })}
          </div>
          <div className="av-grid" role="radiogroup" aria-label={L("样子", "Shape")}>
            {Array.from({ length: LOOK_PARTS.patterns * LOOK_PARTS.faces }, (_, k) => {
              const pattern = Math.floor(k / LOOK_PARTS.faces);
              const face = k % LOOK_PARTS.faces;
              const look = { ...base, pattern, face };
              const on = choice.kind === "look" && base.pattern === pattern && base.face === face;
              return (
                <button key={k} type="button" role="radio" aria-checked={on} className={cn("av-cell", on && "is-on")}
                  aria-label={L(`样子 ${k + 1}`, `Shape ${k + 1}`)} onClick={() => pick({ pattern, face })}>
                  <span className="pa" style={{ width: 40, height: 40, ["--av-cap" as string]: LOOK_PARTS.caps[look.cap] }}>
                    <MushroomFace seed={seed} avatar={lookWord(look)} />
                  </span>
                </button>
              );
            })}
          </div>
          <label className="av-sprout">
            <input type="checkbox" checked={base.sprout} onChange={(e) => pick({ sprout: e.target.checked })} />
            {L("头上长一棵小芽", "A sprout on top")}
          </label>
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
