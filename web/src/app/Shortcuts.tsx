import { Fragment } from "react";
import { L } from "@/lib/i18n";

/*
 * Settings › 快捷键: every key the App listens for, to look up (not to
 * change yet). The keys themselves are handled where they act (AppMode for
 * the window-wide ones, the chat box, the terminal, the search box…); this
 * list is kept in step with them by hand, so add a row here with a new key.
 * On a Mac the keys show as ⌘ ⌥ ⇧ ⌃, elsewhere as Ctrl, Alt, Shift.
 */

/** A key, or a modifier: "mod" is ⌘ on a Mac and Ctrl elsewhere. */
type Key = string;
/** One way to press it: keys held together. */
type Combo = Key[];
type Shortcut = {
  label: () => string;
  note?: () => string;
  /** one or more ways (shown with "/" between) */
  keys: Combo[];
  /** on a Mac, when it is pressed otherwise there */
  mac?: Combo[];
};
type Group = { title: () => string; items: Shortcut[] };

export function isMacKeys(): boolean {
  const p = typeof navigator === "undefined" ? "" : navigator.platform || navigator.userAgent || "";
  return /Mac|iPhone|iPad/.test(p);
}

const GROUPS: Group[] = [
  {
    title: () => L("全局", "Anywhere"),
    items: [
      { label: () => L("搜索：找对话、成员、项目和消息", "Search: chats, members, projects and messages"), keys: [["mod", "K"]] },
      { label: () => L("后退 / 前进", "Back / forward"), note: () => L("回到刚才看的页面，和顶上的 ‹ › 一样；鼠标侧键也行", "To where you were, as ‹ › at the top do; a mouse's side buttons too"), keys: [["mod", "["], ["mod", "]"]] },
      { label: () => L("上一个 / 下一个对话", "Previous / next conversation"), note: () => L("按边栏里的顺序，项目和会话都算", "In the sidebar's order, projects and chats alike"), keys: [["alt", "↑"], ["alt", "↓"]] },
      { label: () => L("切换到第 1–9 个工作区", "Switch to workspace 1–9"), note: () => L("按左下角工作区列表的顺序", "In the order of the workspace list at the bottom left"), keys: [["mod", "1…9"]] },
      { label: () => L("新建项目", "New project"), keys: [["mod", "N"]] },
      { label: () => L("显示 / 隐藏边栏", "Show / hide the sidebar"), keys: [["mod", "\\"]] },
      { label: () => L("打开设置", "Open Settings"), keys: [["mod", ","]] },
      { label: () => L("查看快捷键（这一页）", "Keyboard shortcuts (this page)"), keys: [["mod", "/"]] },
      { label: () => L("刷新", "Refresh"), keys: [["mod", "R"]] },
      { label: () => L("文字放大 / 缩小", "Larger / smaller text"), keys: [["mod", "="], ["mod", "-"]] },
      { label: () => L("文字恢复标准大小", "Standard text size"), keys: [["mod", "0"]] },
      { label: () => L("切换浅色 / 深色", "Switch light / dark"), keys: [["mod", "shift", "L"]] },
      { label: () => L("关闭弹框、菜单和面板", "Close a dialog, menu or panel"), keys: [["Esc"]] },
    ],
  },
  {
    title: () => L("对话", "In a chat"),
    items: [
      { label: () => L("在这个对话里找", "Find in this chat"), keys: [["mod", "F"]] },
      { label: () => L("找到的上一处 / 下一处", "Previous / next match"), note: () => L("在对话的查找框里", "In the chat's find box"), keys: [["Enter"], ["shift", "Enter"]] },
      { label: () => L("添加文件", "Add files"), note: () => L("也可以直接粘贴或拖进输入框", "Or paste or drop them into the box"), keys: [["mod", "U"]] },
    ],
  },
  {
    title: () => L("输入框", "Writing a message"),
    items: [
      { label: () => L("发送", "Send"), keys: [["Enter"]] },
      { label: () => L("换行", "New line"), keys: [["shift", "Enter"]] },
      { label: () => L("排到正在做的事后面", "Queue it after the current task"), note: () => L("agent 正在干活时；直接回车是补充给正在做的这件事", "While the agent is at work; Enter alone adds to what it is doing"), keys: [["mod", "Enter"]] },
      { label: () => L("翻出之前发过的话", "Go through what you sent before"), note: () => L("输入框是空的时候", "From an empty box"), keys: [["↑"], ["↓"]] },
      { label: () => L("填入 agent 建议的下一句", "Use the agent's suggested reply"), keys: [["Tab"]] },
      { label: () => L("按住说话，松开结束", "Hold to talk, let go to stop"), note: () => L("光标在输入框里时", "With the caret in the box"), keys: [["ctrl", "M"]], mac: [["alt", "Space"], ["ctrl", "M"]] },
    ],
  },
  {
    title: () => L("终端", "In a terminal"),
    items: [
      { label: () => L("上一条 / 下一条命令", "Previous / next command"), keys: [["↑"], ["↓"]] },
      { label: () => L("清屏", "Clear the screen"), keys: [["ctrl", "L"]] },
      { label: () => L("停止正在跑的命令", "Stop the running command"), note: () => L("输入框是空的时候", "With nothing typed"), keys: [["ctrl", "C"]] },
    ],
  },
  {
    title: () => L("搜索框和设置", "Search and Settings"),
    items: [
      { label: () => L("在结果里上下选", "Move through the results"), keys: [["↑"], ["↓"]] },
      { label: () => L("打开选中的", "Open the one picked"), keys: [["Enter"]] },
      { label: () => L("在设置里搜索", "Search Settings"), note: () => L("设置打开时", "With Settings open"), keys: [["mod", "F"]] },
    ],
  },
];

const MAC: Record<string, string> = { mod: "⌘", alt: "⌥", shift: "⇧", ctrl: "⌃", Enter: "↩", Esc: "esc", Tab: "⇥" };
const OTHER: Record<string, string> = { mod: "Ctrl", alt: "Alt", shift: "Shift", ctrl: "Ctrl" };

function keyName(k: Key, mac: boolean): string {
  return (mac ? MAC[k] : OTHER[k]) ?? k;
}

/** One way to press it, as key caps. */
function Keys({ combo, mac }: { combo: Combo; mac: boolean }) {
  return (
    <span className="kb-combo">
      {combo.map((k, i) => (
        <Fragment key={i}>
          {!mac && i > 0 && <span className="kb-plus">+</span>}
          <kbd className="kb">{keyName(k, mac)}</kbd>
        </Fragment>
      ))}
    </span>
  );
}

export function ShortcutsSection() {
  const mac = isMacKeys();
  return (
    <div className="kb-page" data-testid="shortcuts">
      {GROUPS.map((g) => (
        <Fragment key={g.title()}>
          <h4 className="app-setting-group">{g.title()}</h4>
          <section className="app-setting-card app-setting-lines">
            {g.items.map((s) => {
              const ways = (mac && s.mac) || s.keys;
              return (
                <div className="app-setting-line kb-line" key={s.label()}>
                  <div className="app-setting-line-text">
                    <b>{s.label()}</b>
                    {s.note && <span>{s.note()}</span>}
                  </div>
                  <span className="kb-ways">
                    {ways.map((c, i) => (
                      <Fragment key={i}>
                        {i > 0 && <span className="kb-or">/</span>}
                        <Keys combo={c} mac={mac} />
                      </Fragment>
                    ))}
                  </span>
                </div>
              );
            })}
          </section>
        </Fragment>
      ))}
      <p className="app-small-note kb-foot">{L("快捷键暂时还不能自定义。", "Shortcuts can't be changed yet.")}</p>
    </div>
  );
}
