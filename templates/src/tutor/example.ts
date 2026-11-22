import { t as uiText } from "../_lib/i18n";
import type { LessonContent } from "./lesson-content";
import type { Level } from "./model";

export function levelLabels(): Record<Level, string> {
  return { beginner: uiText("零基础"), school: uiText("中学基础"), university: uiText("大学基础") };
}

export function riemannExample(level: Level): LessonContent {
  const title = uiText("黎曼猜想：从数质数到看零点");
  if (level === "beginner") return {
    title,
    stages: [
      { title: uiText("先找一找"), text: uiText("质数是大于 1、只能被 1 和自己整除的整数。7 是质数，8 不是，因为 8 可以分成 2 × 4。你能从 1 到 20 找出所有质数吗？") },
      { title: uiText("拉长数表"), text: uiText("下面的数表会标出质数。把范围从 20 拉到 100，看质数的个数与所有数字的个数如何变化。质数没有用完，但它们占的比例总体会变小。") },
      { title: uiText("猜想和它有什么关系？"), text: uiText("数学家可以估算一段很长的数表里有多少质数。黎曼猜想如果成立，我们就能更准确地说，这个估算最多会差多少。它说的是一个特殊函数里的点，这些点会影响质数个数的估算。") },
    ],
    diagram: { type: "primes", title: uiText("哪些数是质数？"), description: uiText("拖动滑块，看看从 1 数到不同的数时，你能找到多少质数。") },
    check: { question: uiText("为什么 9 不是质数？"), choices: [uiText("因为 9 = 3 × 3"), uiText("因为 9 是奇数"), uiText("因为 9 比 7 大")], answer: 0, feedback: uiText("3 是 9 除了 1 和它自己之外的因数。") },
    sources: [],
  };
  const diagram: LessonContent["diagram"] = {
    type: "coordinate", title: uiText("零点要排在哪条线上？"),
    description: uiText("把复数画成一个点，横坐标是实部。图中的圆点是示意点，移动它可以测试位置条件。"),
    xLabel: "Re(s)", yLabel: "Im(s)", line: .5, point: { x: .5, y: 1.6, label: uiText("示意点") },
  };
  const check: LessonContent["check"] = {
    question: uiText("若严格找到了实部为 0.6 的非平凡零点，这会怎样？"),
    choices: [uiText("足以推翻猜想"), uiText("证明猜想成立"), uiText("与猜想无关")],
    answer: 0, feedback: uiText("猜想断言每一个非平凡零点都满足实部等于 1/2，一个反例就足够。"),
  };
  return {
    title, diagram, check, sources: [],
    stages: level === "school" ? [
      { title: uiText("什么叫零点？"), text: uiText("代入某个数使函数的值等于 0，这个数就是零点。比如 f(x) = x² − 4 有两个零点，2 和 −2。黎曼研究的 ζ 函数也有零点，只是输入可以画在复数平面上。") },
      { title: uiText("用图读懂猜想"), text: uiText("黎曼猜想说，ζ 函数所有非平凡零点的横坐标都是 1/2。拖动下面的示意点，看看 0.5 和 0.6 有什么区别。知道某个点在线上，只说明它的位置符合猜想，要判定它是零点还要算函数值。") },
      { title: uiText("为什么值得研究？"), text: uiText("零点的位置会影响质数个数估算的误差。就像知道一把尺子最大会量错多少，一旦猜想成立，我们可以给估算加上更严格的误差范围。") },
    ] : [
      { title: uiText("从欧拉乘积出发"), text: uiText("在 Re(s) > 1 时，ζ(s) = Σ n⁻ˢ = ∏ₚ(1 − p⁻ˢ)⁻¹。将各项展开再相乘，每个正整数恰好由它唯一的质因数分解出现一次。这解释了为何 ζ(s) 能携带质数信息。") },
      { title: uiText("延拓与临界线"), text: uiText("ζ 函数可亚纯延拓到整个复平面，在 s = 1 有一个简单极点。其非平凡零点位于 0 < Re(s) < 1。黎曼猜想断言它们全在 Re(s) = 1/2 上。") },
      { title: uiText("零点如何影响误差"), text: uiText("在与质数计数相关的显式公式里，非平凡零点 ρ 会贡献含 x^ρ 的振荡项。它的增长量级取决于 Re(ρ)。因此把所有零点的实部控制在 1/2，可以约束质数分布偏离主项的程度。") },
    ],
  };
}
