// Original self-reflection items, not a reproduction of the official MBTI
// questionnaire. Version and item IDs stay fixed so saved answers remain valid.
export const QUESTIONNAIRE = "personality32-v1";
export const AXES = ["EI", "SN", "TF", "JP"] as const;
export type Axis = typeof AXES[number];
export type Question = { id: string; axis: Axis; pole: 1 | -1; text: string };
const items: Record<Axis, string[]> = {
  EI: ["群体活动后，我通常还想继续和大家聊一会儿。", "进入一个陌生的场合，我会主动开启话题。", "遇到新想法，我更喜欢边说边理清思路。", "休息时，我常通过与人互动恢复精力。", "连续社交之后，我需要一段独处时间充电。", "在讨论中，我习惯先想清楚，再开口。", "空闲的一天，我更想留一些不被打扰的时间。", "建立关系时，我更享受少数深入的交流。"],
  SN: ["我理解新事物时，会先留意具体事实和实例。", "解释一件事，我通常先讲清楚实际发生了什么。", "开始一个任务时，我倾向参考已有的经验和做法。", "做计划时，我会特别核对现实条件与细节。", "面对信息时，我常先联想到它背后的模式和可能性。", "我喜欢讨论一个想法未来可能带来的变化。", "了解一个主题时，我会先抓住整体图景。", "我容易被新颖的概念和未尝试过的方法吸引。"],
  TF: ["做决定时，我首先比较各方案的逻辑是否一致。", "遇到分歧时，我倾向先找出可共同检验的依据。", "评价一个方案时，我通常先分析它是否解决了问题。", "提供反馈时，我会先指出需要调整的关键问题。", "做决定时，我会先考虑它对相关人的影响。", "当规则与具体处境冲突时，我会认真衡量个人需求。", "评价一个方案时，我很重视它是否符合我珍视的价值。", "提供反馈时，我会先想怎样让对方更容易接受。"],
  JP: ["约定了重要事项后，我会尽早把时间和步骤定下来。", "有明确的安排，会让我更安心地推进事情。", "我通常喜欢先完成必须做的事，再随兴活动。", "面对一个选择，我更喜欢及时形成决定并继续推进。", "我喜欢在计划中为临时出现的机会留出空间。", "获得新信息时，我愿意重新调整原来的安排。", "探索一个任务时，我喜欢先试几种路线，再确定做法。", "如果条件允许，我愿意让一些选择保持开放。"],
};
// Interleave dimensions and balance the two directions throughout the flow.
export const QUESTIONS: Question[] = [0,4,1,5,2,6,3,7].flatMap(i => AXES.map(axis => ({ id: `${axis.toLowerCase()}-${i + 1}`, axis, pole: (i < 4 ? 1 : -1) as 1 | -1, text: items[axis][i] })));
export const DIMENSIONS: Record<Axis, { title: string; left: string; right: string; leftNote: string; rightNote: string }> = {
  EI: { title: "精力的去向", left: "外向", right: "内向", leftNote: "更容易在互动与外部活动中整理思路、获得能量。", rightNote: "更容易在独处与内在思考中整理思路、恢复能量。" },
  SN: { title: "留意的信息", left: "实感", right: "直觉", leftNote: "更容易先关注具体事实、经验和可观察的细节。", rightNote: "更容易先关注整体模式、联想和未来的可能性。" },
  TF: { title: "决定的依据", left: "思考", right: "情感", leftNote: "更容易先比较逻辑、原则和可检验的依据。", rightNote: "更容易先衡量价值、处境和决定对人的影响。" },
  JP: { title: "安排的方式", left: "判断", right: "知觉", leftNote: "更喜欢形成安排、明确节点，让事情逐步落定。", rightNote: "更喜欢保留弹性、探索选项，随着信息调整安排。" },
};
