// React Flow owns the working viewport and drag positions. Shared records
// change only when the user finishes a gesture.
import { memo, useEffect, useMemo, useRef, useState } from "react";
import { Background, Controls, Handle, Position, ReactFlow, ReactFlowProvider, useNodesState, useReactFlow, ViewportPortal, getBezierPath,
  type Node, type NodeProps, type XYPosition } from "@xyflow/react";
import { Clapperboard, FileText, Image as ImageIcon, SquarePen, Link2 } from "lucide-react";
import { t as uiText } from "../_lib/i18n";
import { nodeWidth, nodeHeight, type Entry, type Kind } from "./model";
import "@xyflow/react/dist/style.css";

export const kinds = ["note", "text", "image", "video"] as const;
export const icons = { note: SquarePen, text: FileText, image: ImageIcon, video: Clapperboard };
export const kindName = (kind: Kind) => kind === "note" ? uiText("便签") : kind === "text" ? uiText("文本") :
  kind === "image" ? uiText("图片") : uiText("视频");

type CardNode = Node<{ entry: Entry; busy: boolean }, "card">;
const Card = memo(function Card({ data: { entry, busy }, selected }: NodeProps<CardNode>) {
  const { kind, title, output, prompt, from } = entry.data;
  const Icon = icons[kind];
  return <div className="ac-node" data-selected={selected}>
    {kind !== "note" && <Handle type="target" position={Position.Left} aria-label={uiText("输入端口")}/>}
    <span className="ac-node-kind"><Icon size={15}/>{kindName(kind)}{from && <Link2 size={13}/>}</span>
    <strong>{title || kindName(kind)}</strong>
    <span className="ac-node-preview">{busy ? uiText("生成中…") :
      output && (kind === "image" || kind === "video") ? uiText("生成结果") : output || prompt || uiText("点击编辑节点")}</span>
    <Handle type="source" position={Position.Right} aria-label={uiText("输出端口")}/>
  </div>;
});
const nodeTypes = { card: Card };

type Menu = { left: number; top: number; point: XYPosition; from?: string; pendingEdge?: string };
type Props = {
  entries: Entry[]; selected: string; busy: string; select: (id: string) => void;
  add: (kind: Kind, point: XYPosition, from?: string) => void;
  move: (id: string, point: XYPosition) => Promise<XYPosition>;
  connect: (source: string, target: string) => void;
};

function BoardView({ entries, selected, busy, select, add, move, connect }: Props) {
  const [nodes, setNodes, changeNodes] = useNodesState<CardNode>([]);
  const [menu, setMenu] = useState<Menu>();
  const stage = useRef<HTMLElement>(null);
  const firstItem = useRef<HTMLButtonElement>(null);
  const moving = useRef(new Set<string>());
  const pending = useRef(new Map<string, XYPosition>());
  const flow = useReactFlow<CardNode>();
  const edges = useMemo(() => {
    const ids = new Set(entries.map(entry => entry.id));
    return entries.filter(entry => ids.has(entry.data.from)).map(entry => ({
      id: "edge-" + entry.id, source: entry.data.from, target: entry.id, selectable: false,
    }));
  }, [entries]);

  useEffect(() => {
    setNodes(previous => {
      const byId = new Map(previous.map(node => [node.id, node]));
      return entries.map(entry => {
        const old = byId.get(entry.id);
        const position = pending.current.get(entry.id) || (moving.current.has(entry.id) && old?.position) ||
          { x: entry.data.x, y: entry.data.y };
        const data = old?.data.entry.data === entry.data && old.data.busy === (busy === entry.id)
          ? old.data : { entry: { ...entry }, busy: busy === entry.id };
        return { ...old, id: entry.id, type: "card", selected: entry.id === selected, position, data,
          width: nodeWidth, height: nodeHeight, ariaLabel: entry.data.title || kindName(entry.data.kind) };
      });
    });
  }, [entries, selected, busy, setNodes]);

  useEffect(() => { if (menu) firstItem.current?.focus(); }, [menu]);

  function showMenu(client: XYPosition, from?: string) {
    const rect = stage.current!.getBoundingClientRect();
    const point = flow.screenToFlowPosition(client);
    const source = from ? flow.getNode(from) : undefined;
    const pendingEdge = source ? getBezierPath({
      sourceX: source.position.x + nodeWidth, sourceY: source.position.y + nodeHeight / 2,
      sourcePosition: Position.Right, targetX: point.x, targetY: point.y, targetPosition: Position.Left,
    })[0] : undefined;
    setMenu({
      point, from, pendingEdge,
      left: Math.max(6, Math.min(client.x - rect.left, rect.width - 172)),
      top: Math.max(6, Math.min(client.y - rect.top, rect.height - 160)),
    });
  }

  return <section className="ac-stage" ref={stage} aria-label={uiText("节点画布")}
    onKeyDown={event => { if (event.key === "Escape") setMenu(undefined); }}
    onDoubleClickCapture={event => {
      if (event.target instanceof Element && event.target.matches(".react-flow__pane")) {
        showMenu({ x: event.clientX, y: event.clientY });
      }
    }}
    onPointerDownCapture={event => {
      if (event.button === 0 && event.target instanceof Element && event.target.matches(".react-flow__pane")) {
        setMenu(undefined);
        select("");
      }
    }}>
    <ReactFlow nodes={nodes} edges={edges} nodeTypes={nodeTypes} onNodesChange={changeNodes}
      defaultViewport={{ x: 24, y: 24, zoom: 1 }} minZoom={0.3} maxZoom={2.4} zoomOnDoubleClick={false}
      onlyRenderVisibleElements elevateNodesOnSelect deleteKeyCode={null} disableKeyboardA11y={false}
      onNodeClick={(_, node) => { setMenu(undefined); select(node.id); }}
      onPaneContextMenu={event => { event.preventDefault(); showMenu({ x: event.clientX, y: event.clientY }); }}
      onNodeContextMenu={(event, node) => { event.preventDefault(); select(node.id); showMenu({ x: event.clientX, y: event.clientY }, node.id); }}
      onConnect={connection => connect(connection.source, connection.target)}
      onConnectEnd={(event, state) => {
        if (state.isValid || state.toHandle || !state.fromHandle || !state.fromNode) return;
        const pointer = "changedTouches" in event ? event.changedTouches[0] : event;
        if (!pointer) return;
        const hit = document.elementFromPoint(pointer.clientX, pointer.clientY);
        if (!hit || !stage.current?.contains(hit)) return;
        const target = hit.closest<HTMLElement>(".react-flow__node")?.dataset.id;
        const from = state.fromNode.id;
        if (target && target !== from) {
          if (state.fromHandle.type === "source") connect(from, target);
          else connect(target, from);
          return;
        }
        if (state.fromHandle.type === "source" && hit.matches(".react-flow__pane")) {
          showMenu({ x: pointer.clientX, y: pointer.clientY }, from);
        }
      }}
      onNodeDragStart={(_, node) => { setMenu(undefined); moving.current.add(node.id); select(node.id); }}
      onNodeDragStop={(_, node) => {
        moving.current.delete(node.id);
        const position = node.position;
        pending.current.set(node.id, position);
        void move(node.id, position).then(saved => {
          if (pending.current.get(node.id) !== position) return;
          pending.current.delete(node.id);
          setNodes(nodes => nodes.map(item => item.id === node.id && !moving.current.has(node.id) ? { ...item, position: saved } : item));
        });
      }}
      ariaLabelConfig={{
        "controls.ariaLabel": uiText("画布缩放"),
        "controls.zoomIn.ariaLabel": uiText("放大"),
        "controls.zoomOut.ariaLabel": uiText("缩小"),
        "controls.fitView.ariaLabel": uiText("适应画布"),
      }}
      proOptions={{ hideAttribution: true }}>
      {menu?.pendingEdge && <ViewportPortal><svg className="ac-pending-edge" aria-hidden="true"><path d={menu.pendingEdge}/></svg></ViewportPortal>}
      <Background gap={20} size={1.25}/>
      <Controls showInteractive={false} aria-label={uiText("画布缩放")} />
    </ReactFlow>
    {!entries.length && <div className="ac-empty"><h2>{uiText("添加第一个节点")}</h2><p>{uiText("右键或双击画布添加节点。")}</p></div>}
    {menu && <div className="ac-create-menu" style={{ left: menu.left, top: menu.top }} aria-label={uiText("新增节点")}>
      <strong>{menu.from ? uiText("添加下游节点") : uiText("新增节点")}</strong>
      {(menu.from ? kinds.slice(1) : kinds).map((kind, index) => {
        const Icon = icons[kind];
        return <button type="button" key={kind} ref={index === 0 ? firstItem : undefined} onClick={() => {
          add(kind, menu.point, menu.from);
          setMenu(undefined);
        }}><Icon size={15}/>{kindName(kind)}</button>;
      })}
    </div>}
  </section>;
}

export function Board(props: Props) {
  return <ReactFlowProvider><BoardView {...props}/></ReactFlowProvider>;
}
