import { useId, useState } from "react";
import { ArrowUpRight, Check, Layers3, MoveHorizontal } from "lucide-react";
import { locale, t as uiText } from "../_lib/i18n";
import type { Diagram, LessonContent } from "./lesson-content";

function formatNumber(value: number) {
  return new Intl.NumberFormat(locale(), { maximumFractionDigits: 2 }).format(value);
}

function PrimeGrid() {
  const [end, setEnd] = useState(40);
  const prime = (n: number) => n > 1 && !Array.from({ length: Math.floor(Math.sqrt(n)) - 1 }, (_, i) => i + 2).some((d) => n % d === 0);
  const cells = Array.from({ length: end }, (_, i) => ({ n: i + 1, prime: prime(i + 1) }));
  return <>
    <label className="tu-range">{uiText("数到 {0}", end)}
      <input type="range" min="20" max="100" step="10" value={end} onChange={(e) => setEnd(Number(e.target.value))}/>
    </label>
    <div className="tu-prime-grid" aria-label={uiText("用实底标出质数")}>
      {cells.map(({ n, prime }) => <span key={n} data-prime={prime}>{n}</span>)}
    </div>
    <p className="tu-chart-readout" role="status">{uiText("前 {0} 个正整数里有 {1} 个质数。", end, cells.filter((cell) => cell.prime).length)}</p>
  </>;
}

function Coordinates({ chart }: { chart: Extract<Diagram, { type: "coordinate" }> }) {
  const [value, setValue] = useState(chart.point.x);
  const axisID = useId();
  const at = (x: number) => 74 + x * 366;
  const y = 112 - chart.point.y * 22;
  const onLine = Math.abs(value - chart.line) < 0.00001;
  return <>
    <svg className="tu-coordinate" viewBox="0 0 480 220" role="img" aria-label={chart.title}>
      <defs><marker id={axisID} viewBox="0 0 8 8" refX="6" refY="4" markerWidth="6" markerHeight="6" orient="auto">
        <path d="M0 0l8 4-8 4z" fill="currentColor"/>
      </marker></defs>
      <g stroke="var(--line)" strokeWidth="1">
        {[.25, .5, .75, 1].map((x) => <line key={x} x1={at(x)} x2={at(x)} y1="36" y2="182"/>)}
        {[56, 84, 140, 168].map((tick) => <line key={tick} x1="74" x2="440" y1={tick} y2={tick}/>)}
      </g>
      <g stroke="currentColor" strokeWidth="1.25">
        <line x1="74" x2="446" y1="112" y2="112" markerEnd={`url(#${axisID})`}/>
        <line x1="74" x2="74" y1="182" y2="34" markerEnd={`url(#${axisID})`}/>
      </g>
      <line className="tu-critical" x1={at(chart.line)} x2={at(chart.line)} y1="36" y2="182"/>
      <g fill="currentColor" fontSize="13">
        <text x="88" y="30">{chart.yLabel}</text>
        <text textAnchor="end" x="450" y="101">{chart.xLabel}</text>
        <text x="74" y="200" textAnchor="middle">0</text>
        <text x={at(chart.line)} y="200" textAnchor="middle">{formatNumber(chart.line)}</text>
        <text x="440" y="200" textAnchor="middle">1</text>
      </g>
      <circle cx={at(value)} cy={y} r="12" className="tu-coordinate-halo"/>
      <circle cx={at(value)} cy={y} r="5" fill="var(--accent)"/>
      <text x={at(value)} y={y - 16} textAnchor={value > .75 ? "end" : value < .25 ? "start" : "middle"} fill="var(--fg)" fontSize="13">{chart.point.label}</text>
    </svg>
    <label className="tu-range"><span><MoveHorizontal size={14}/>{uiText("移动示意点的横坐标")}</span>
      <input type="range" min="0" max="1" step="0.01" value={value} onChange={(e) => setValue(Number(e.target.value))}/>
    </label>
    <p className="tu-chart-readout" role="status">{uiText("{0} = {1}", chart.xLabel, formatNumber(value))} · {onLine ? uiText("点在标出的线上") : uiText("点在线外")}</p>
  </>;
}

function Sequence({ chart }: { chart: Extract<Diagram, { type: "sequence" }> }) {
  const [index, setIndex] = useState(0);
  return <>
    <div className="tu-sequence">
      {chart.steps.map((item, i) => <button type="button" key={i} aria-pressed={index === i} onClick={() => setIndex(i)}>{item.title}</button>)}
    </div>
    <p className="tu-chart-readout" role="status">{chart.steps[index].text}</p>
  </>;
}

function DiagramView({ chart }: { chart: Diagram }) {
  return <figure className="tu-figure">
    <figcaption><h3>{chart.title}</h3>{chart.description && <p>{chart.description}</p>}</figcaption>
    {chart.type === "primes" && <PrimeGrid/>}
    {chart.type === "coordinate" && <Coordinates chart={chart}/>}
    {chart.type === "sequence" && <Sequence chart={chart}/>}
  </figure>;
}

export function LessonView({ content, cite, pageLabel }: {
  content: LessonContent;
  cite?: (page: number) => void;
  pageLabel?: (page: number) => string;
}) {
  const [choice, setChoice] = useState<number | null>(null);
  const q = content.check;
  return <div className="tu-lesson">
    <header className="tu-lesson-head"><Layers3 size={19}/><h2>{content.title}</h2></header>
    {content.stages.map((stage, index) => <div key={index}>
      <section className="tu-lesson-stage"><h3>{stage.title}</h3><p>{stage.text}</p></section>
      {index === Math.min(1, content.stages.length - 1) && content.diagram && <DiagramView chart={content.diagram}/>}
    </div>)}
    {q && <section className="tu-check">
      <h3>{uiText("检查一下理解")}</h3>
      <p>{q.question}</p>
      <div className="tu-options">{q.choices.map((item, i) => <button type="button" aria-pressed={choice === i} key={i} onClick={() => setChoice(i)}>{item}</button>)}</div>
      {choice !== null && <p role="status" className={choice === q.answer ? "tu-correct" : "tu-feedback"}>
        {choice === q.answer ? <><Check size={15}/> {uiText("答对了")} {q.feedback}</> : uiText("再试一次")}
      </p>}
    </section>}
    {cite && pageLabel && content.sources.length > 0 && <section className="tu-citations"><h3>{uiText("教材依据")}</h3>
      {content.sources.map(({ page, quote }, i) => <button type="button" key={i} onClick={() => cite(page)}>
        <span>{pageLabel(page)}</span><q>{quote}</q><ArrowUpRight size={15}/>
      </button>)}
    </section>}
  </div>;
}
