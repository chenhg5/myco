// Original short essays for the optional sample shelf. No third-party books.
export const demos = [
  {
    title: "山间来信",
    en: "Letters from the hills",
    text: "山间来信\n\n写给每一个愿意慢下来的人。\n\n",
    topics: [
      "第一章 清晨的窗",
      "第二章 一条小路",
      "第三章 雨后的森林",
      "第四章 回信",
    ],
  },
  {
    title: "阅读的形状",
    en: "The shape of reading",
    text: "阅读的形状\n\n留一点时间，给没有目的的阅读。\n\n",
    topics: ["第一章 一张白纸", "第二章 文字的呼吸", "第三章 在页边停留"],
  },
  {
    title: "慢下来的时光",
    en: "A slower kind of time",
    text: "慢下来的时光\n\n关于日常、散步与重新发现。\n\n",
    topics: ["第一章 下午四点", "第二章 城市的背面", "第三章 一杯茶的时间"],
  },
];
export function demoFile(index: number, english: boolean) {
  const d = demos[index];
  const paragraph = english
    ? "There is a quiet moment before the day begins. The light arrives at the window, and the room slowly takes its shape. A book lies open on the table. We do not need to hurry through it; a single page can be enough.\n\nOutside, the path follows the edge of the hill. Each time we walk it, something has changed: a new leaf, the sound of water, a small patch of sky. Reading is a little like walking this path. The words stay the same, but we arrive as someone new.\n\n"
    : "清晨的光从窗边慢慢移过来。桌上放着一本翻开的书，旁边是一杯还没有凉透的茶。山里的时间好像比城里宽一些，允许我们在一个句子前停留，也允许我们什么都不做，只听一会儿风。\n\n沿着屋后的小路往上走，会经过一片很安静的树林。叶子上还留着昨夜的雨水，脚步声落在松软的土地上。走过许多次的路，每一次仍然有新的风景：一朵刚开的花，一截倒下的树枝，或者远处传来的一声鸟鸣。\n\n阅读也是这样。我们以为自己在寻找答案，后来才发现，真正珍贵的也许是那个愿意停下来、认真看一看世界的自己。书页很薄，却能容纳许多尚未走过的地方。\n\n";
  const text =
    (english ? d.en + "\n\nAn original reading sample.\n\n" : d.text) +
    d.topics
      .map(
        (t, i) =>
          (english ? `Chapter ${i + 1}\n\n` : t + "\n\n") + paragraph.repeat(5),
      )
      .join("\n");
  return new File([text], (english ? d.en : d.title) + ".txt", {
    type: "text/plain",
  });
}
