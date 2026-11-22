import { Check, Minus, Plus } from "lucide-react";
import { t as uiText } from "../_lib/i18n";
import { Dialog } from "./ui";
import { defaults, type Settings } from "./model";
export function ReadingSettings({
  value: s,
  pdf,
  onChange,
  onClose,
}: {
  value: Settings;
  pdf: boolean;
  onChange: (s: Settings) => void;
  onClose: () => void;
}) {
  const patch = (v: Partial<Settings>) => onChange({ ...s, ...v });
  const themes: [Settings["theme"], string][] = [
    ["auto", uiText("跟随应用")],
    ["paper", uiText("白纸")],
    ["warm", uiText("羊皮纸")],
    ["sage", uiText("青瓷")],
    ["night", uiText("夜读")],
  ];
  const effects: [Settings["effect"], string][] = [
    ["slide", uiText("平移")],
    ["leaf", uiText("书页")],
    ["fade", uiText("淡入")],
    ["none", uiText("无动画")],
  ];
  return (
    <Dialog title={uiText("阅读设置")} onClose={onClose} drawer>
      <div className="rd-settings">
        <fieldset>
          <legend>{uiText("阅读主题")}</legend>
          <div className="rd-theme-options">
            {themes.map(([theme, label]) => (
              <button
                key={theme}
                title={label}
                aria-label={label}
                aria-pressed={s.theme === theme}
                onClick={() => patch({ theme })}
                className={`rd-swatch rd-swatch-${theme} ${s.theme === theme ? "is-selected" : ""}`}
              >
                <span>{s.theme === theme ? <Check size={18} /> : "Aa"}</span>
                <small>{label}</small>
              </button>
            ))}
          </div>
        </fieldset>
        {!pdf ? (
          <>
            <fieldset>
              <legend>{uiText("字体")}</legend>
              <div className="rd-segments">
                {(
                  [
                    ["serif", uiText("宋体 / 衬线")],
                    ["sans", uiText("黑体 / 无衬线")],
                    ["mono", uiText("等宽")],
                  ] as const
                ).map(([font, name]) => (
                  <button
                    key={font}
                    aria-pressed={s.font === font}
                    onClick={() => patch({ font })}
                  >
                    {name}
                  </button>
                ))}
              </div>
            </fieldset>
            <fieldset>
              <legend>
                {uiText("字号")}
                <output>{s.size} px</output>
              </legend>
              <div className="rd-stepper">
                <button
                  aria-label={uiText("缩小字号")}
                  disabled={s.size <= 14}
                  onClick={() => patch({ size: s.size - 1 })}
                >
                  <Minus size={16} />
                </button>
                <input
                  aria-label={uiText("字号")}
                  type="range"
                  min={14}
                  max={36}
                  step={1}
                  value={s.size}
                  onChange={(e) => patch({ size: +e.target.value })}
                />
                <button
                  aria-label={uiText("增大字号")}
                  disabled={s.size >= 36}
                  onClick={() => patch({ size: s.size + 1 })}
                >
                  <Plus size={16} />
                </button>
              </div>
            </fieldset>
            <fieldset>
              <legend>
                {uiText("行间距")}
                <output>{s.line.toFixed(1)}</output>
              </legend>
              <input
                aria-label={uiText("行间距")}
                type="range"
                min={1.4}
                max={2.6}
                step={0.1}
                value={s.line}
                onChange={(e) => patch({ line: +e.target.value })}
              />
            </fieldset>
            <fieldset>
              <legend>
                {uiText("文字宽度")}
                <output>{s.width} px</output>
              </legend>
              <input
                aria-label={uiText("文字宽度")}
                type="range"
                min={480}
                max={960}
                step={40}
                value={s.width}
                onChange={(e) => patch({ width: +e.target.value })}
              />
            </fieldset>
            <label className="rd-check">
              <span>{uiText("加粗文字")}</span>
              <input
                type="checkbox"
                checked={s.bold}
                onChange={(e) => patch({ bold: e.target.checked })}
              />
            </label>
          </>
        ) : (
          <p className="rd-hint">
            {uiText(
              "PDF 保留原书排版。请用阅读工具栏调整缩放；字体与行距设置适用于 TXT。",
            )}
          </p>
        )}
        <fieldset>
          <legend>{uiText("翻页效果")}</legend>
          <div className="rd-segments rd-effects">
            {effects.map(([effect, label]) => (
              <button
                key={effect}
                aria-pressed={s.effect === effect}
                onClick={() => patch({ effect })}
              >
                {label}
              </button>
            ))}
          </div>
        </fieldset>
        <p className="rd-hint">
          {uiText(
            "左右滑动或使用方向键翻页。系统开启减少动态效果时，将自动关闭动画。",
          )}
        </p>
        <button className="rd-quiet" onClick={() => onChange({ ...defaults })}>
          {uiText("恢复默认设置")}
        </button>
      </div>
    </Dialog>
  );
}
