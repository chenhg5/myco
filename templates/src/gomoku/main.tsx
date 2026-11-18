// 五子棋: two players, people or agents, on a 15 × 15 board. Each stone is
// a record (round, number, where); a move's id is its place in the round,
// so two moves at once cannot both land. The page checks turns and finds
// five in a row; agents play by writing their move.
import "../_lib/sheet.css";
import "./gomoku.css";
import { useMemo, useState } from "react";
import { Flag, RotateCcw, Undo2, UserRound } from "lucide-react";
import { Store } from "../_lib/myco";
import { Bar, Kit, Toast, mount, titleSaver, useKit } from "../_lib/appkit";

type Game = { title?: string; black?: string; white?: string; round?: number; resigned?: string };
type Move = { x?: number; y?: number; stone?: "black" | "white"; round?: number; n?: number };
const N = 15;
const game = new Store<Game & Record<string, unknown>>("game");
const moves = new Store<Move & Record<string, unknown>>("moves");
const kit = new Kit([game, moves], { quiet: true });
const title = titleSaver(kit, game);
const COLS = "ABCDEFGHJKLMNOP";

type State = { board: (null | "black" | "white")[][]; list: { x: number; y: number; stone: "black" | "white"; id: string; by?: string }[]; turn: "black" | "white"; winner: null | "black" | "white"; line: [number, number][] };

/** The board of this round: moves in order, skipping any out of turn or on a stone. */
function play(round: number): State {
  const board: State["board"] = Array.from({ length: N }, () => Array(N).fill(null));
  const list: State["list"] = [];
  let turn: "black" | "white" = "black", winner: State["winner"] = null, line: [number, number][] = [];
  const ms = moves.rows.filter((r) => (r.data.round ?? 1) === round).sort((a, b) => (Number(a.data.n) || 0) - (Number(b.data.n) || 0));
  for (const r of ms) {
    const { x, y, stone } = r.data;
    if (winner || typeof x !== "number" || typeof y !== "number" || x < 0 || y < 0 || x >= N || y >= N || stone !== turn || board[y][x]) continue;
    board[y][x] = stone;
    list.push({ x, y, stone, id: r.id, by: r.updated_by });
    const five = fiveAt(board, x, y);
    if (five) { winner = stone; line = five; }
    turn = turn === "black" ? "white" : "black";
  }
  return { board, list, turn, winner, line };
}
function fiveAt(b: State["board"], x: number, y: number): [number, number][] | null {
  const s = b[y][x];
  for (const [dx, dy] of [[1, 0], [0, 1], [1, 1], [1, -1]]) {
    const run: [number, number][] = [[x, y]];
    for (const k of [1, -1]) for (let i = 1; i < 5; i++) { const X = x + dx * i * k, Y = y + dy * i * k; if (X < 0 || Y < 0 || X >= N || Y >= N || b[Y][X] !== s) break; run.push([X, Y]); }
    if (run.length >= 5) return run;
  }
  return null;
}

function App() {
  useKit(kit);
  const g = game.get("main")?.data || {};
  const round = g.round || 1;
  const st = useMemo(() => play(round), [kit.version, round]); // eslint-disable-line react-hooks/exhaustive-deps
  const [hover, setHover] = useState<[number, number] | null>(null);
  const me = kit.me;
  const mine = (s: "black" | "white") => !!me && g[s] === me;
  const resigned = g.resigned === "black" || g.resigned === "white" ? g.resigned : null;
  const over = !!st.winner || !!resigned;
  const myTurn = !over && mine(st.turn);
  const setGame = (patch: Game) => { kit.track(game.put({ ...(game.get("main")?.data || {}), ...patch }, "main")); kit.emit(); };
  const place = (x: number, y: number) => {
    if (!myTurn || st.board[y][x]) return;
    const n = st.list.length + 1;
    kit.track(moves.put({ x, y, stone: st.turn, round, n }, `r${round}-${n}`));
    kit.emit();
  };
  const undo = () => {
    const last = st.list[st.list.length - 1];
    if (!last || !mine(last.stone) || over) return;
    kit.track(moves.remove(last.id));
    kit.emit();
  };
  const seat = (s: "black" | "white") => {
    const name = window.prompt(s === "black" ? "谁执黑？写你自己，或者一个 agent 的名字（它看到轮到自己就会下）" : "谁执白？写你自己，或者一个 agent 的名字", g[s] || me);
    if (name !== null) setGame({ [s]: name.trim() });
  };
  const status = resigned ? `${resigned === "black" ? "黑" : "白"}方认输，${resigned === "black" ? "白" : "黑"}方赢了`
    : st.winner ? `${st.winner === "black" ? "黑" : "白"}方五子连珠，赢了！`
    : !g.black || !g.white ? "先坐下：点「黑方」「白方」写上谁来下" : `轮到${st.turn === "black" ? "黑" : "白"}方（${g[st.turn]}）${myTurn ? "，该你了" : ""}`;
  const last = st.list[st.list.length - 1];
  const W = 600, pad = 30, step = (W - pad * 2) / (N - 1);
  const winSet = new Set(st.line.map(([x, y]) => `${x},${y}`));
  return (
    <div className="app gm">
      <Bar kit={kit} title={title.get()} placeholder="五子棋" onTitle={title.set}>
        <button type="button" className="btn" onClick={undo} disabled={!last || !mine(last.stone) || over}><Undo2 className="ico" />悔棋</button>
        {!over && (mine("black") || mine("white")) && <button type="button" className="btn" onClick={() => { if (window.confirm("认输？")) setGame({ resigned: mine(st.turn) ? st.turn : mine("black") ? "black" : "white" }); }}><Flag className="ico" />认输</button>}
        <button type="button" className="btn btn-primary" onClick={() => setGame({ round: round + 1, resigned: "" })}><RotateCcw className="ico" />再来一局</button>
      </Bar>
      <div className="gm-main">
        <aside className="gm-side">
          {(["black", "white"] as const).map((s) => (
            <button key={s} type="button" className={`gm-seat${!over && st.turn === s ? " is-turn" : ""}`} onClick={() => seat(s)} title="点一下换人">
              <span className={`gm-stone is-${s}`} />
              <span className="gm-seat-text">
                <b>{s === "black" ? "黑方（先手）" : "白方"}</b>
                <span>{g[s] ? <>{g[s]}{mine(s) ? "（你）" : ""}</> : <span className="faint"><UserRound className="ico" />点这里坐下</span>}</span>
              </span>
            </button>
          ))}
          <div className={`gm-status${over ? " is-over" : myTurn ? " is-mine" : ""}`} role="status">{status}</div>
          <div className="gm-moves">
            <div className="gm-moves-head">第 {round} 局 · {st.list.length} 手</div>
            <ol>
              {st.list.slice(-12).map((m, i) => (
                <li key={m.id}><span className={`gm-dot is-${m.stone}`} />{st.list.length - Math.min(12, st.list.length) + i + 1}. {COLS[m.x]}{N - m.y}</li>
              ))}
            </ol>
          </div>
        </aside>
        <div className="gm-board-wrap">
          <svg className={`gm-board${myTurn ? " is-mine" : ""}`} viewBox={`0 0 ${W} ${W}`} onMouseLeave={() => setHover(null)}>
            <defs>
              <radialGradient id="gb" cx="35%" cy="30%" r="70%"><stop offset="0" stopColor="#666" /><stop offset="1" stopColor="#111" /></radialGradient>
              <radialGradient id="gw" cx="35%" cy="30%" r="75%"><stop offset="0" stopColor="#fff" /><stop offset="1" stopColor="#d6d6d6" /></radialGradient>
            </defs>
            <rect x="0" y="0" width={W} height={W} rx="10" className="gm-wood" />
            {Array.from({ length: N }, (_, i) => (
              <g key={i}>
                <line x1={pad} x2={W - pad} y1={pad + i * step} y2={pad + i * step} className="gm-grid" />
                <line y1={pad} y2={W - pad} x1={pad + i * step} x2={pad + i * step} className="gm-grid" />
                <text x={pad + i * step} y={W - 8} className="gm-coord" textAnchor="middle">{COLS[i]}</text>
                <text x={10} y={pad + i * step + 4} className="gm-coord" textAnchor="middle">{N - i}</text>
              </g>
            ))}
            {[[3, 3], [11, 3], [7, 7], [3, 11], [11, 11]].map(([x, y]) => <circle key={`${x}${y}`} cx={pad + x * step} cy={pad + y * step} r="4" className="gm-star" />)}
            {st.board.map((row, y) => row.map((s, x) => (
              <g key={`${x},${y}`} onMouseEnter={() => setHover([x, y])} onClick={() => place(x, y)}>
                <rect x={pad + x * step - step / 2} y={pad + y * step - step / 2} width={step} height={step} fill="transparent" />
                {s && <circle cx={pad + x * step} cy={pad + y * step} r={step * 0.44} fill={`url(#g${s === "black" ? "b" : "w"})`} className={`gm-s${winSet.has(`${x},${y}`) ? " is-win" : ""}`} />}
                {!s && myTurn && hover && hover[0] === x && hover[1] === y && <circle cx={pad + x * step} cy={pad + y * step} r={step * 0.44} className={`gm-ghost is-${st.turn}`} />}
              </g>
            )))}
            {last && <circle cx={pad + last.x * step} cy={pad + last.y * step} r="5" className={`gm-last on-${last.stone}`} />}
          </svg>
        </div>
      </div>
      <Toast kit={kit} />
    </div>
  );
}

void mount(kit, App, async () => {
  if (!game.get("main")) await game.put({ title: "五子棋", black: kit.me, white: "", round: 1 }, "main")?.catch(() => game.load());
});
