import { createContext, useCallback, useContext, useMemo, type Dispatch, type SetStateAction } from "react";
import {
  Background,
  BackgroundVariant,
  BaseEdge,
  ConnectionMode,
  Controls,
  Handle,
  MiniMap,
  Panel,
  Position,
  ReactFlow,
  getStraightPath,
  useInternalNode,
  useReactFlow,
  type Connection,
  type EdgeProps,
  type EdgeTypes,
  type NodeMouseHandler,
  type NodeProps,
  type NodeTypes,
  type OnEdgesChange,
  type OnNodesChange,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import {
  makeBorder,
  makeTerritory,
  neighborsByNode,
  uniqueTerritoryName,
  type BorderEdge,
  type EditorContinent,
  type TerritoryNode,
} from "./editorModel";

// ── Shared lookups for node/edge renderers ────────────────────────────────────

type CanvasLookups = {
  continentsByID: Map<string, EditorContinent>;
  degreeByID: Map<string, number>;
};

const CanvasContext = createContext<CanvasLookups>({ continentsByID: new Map(), degreeByID: new Map() });

const FALLBACK_COLOR = "#687a91";

// ── Territory node ────────────────────────────────────────────────────────────

const handleStyle = { width: 8, height: 8, border: "none" };

function TerritoryCard({ id, data, selected }: NodeProps<TerritoryNode>) {
  const { continentsByID, degreeByID } = useContext(CanvasContext);
  const continent = continentsByID.get(data.continentId);
  const color = continent?.color ?? FALLBACK_COLOR;
  const degree = degreeByID.get(id) ?? 0;
  return (
    <div
      className={`min-w-[120px] max-w-[180px] select-none rounded-lg border bg-gc-surface px-2.5 py-1.5 shadow-sm transition-colors ${
        selected ? "border-gc-accent ring-2 ring-gc-accent/30" : "border-gc-border hover:border-gc-muted"
      }`}
      style={{ borderLeft: `3px solid ${color}` }}
    >
      {/* Loose connection mode: either handle can start or end a border. */}
      <Handle type="source" position={Position.Left} style={{ ...handleStyle, background: color }} />
      <div className="truncate text-xs font-semibold text-gc-text" title={data.name}>
        {data.name.trim() || <span className="italic text-gc-danger">unnamed</span>}
      </div>
      <div className="mt-0.5 flex items-center justify-between gap-2 text-[10px] text-gc-muted">
        <span className="truncate">{continent?.name.trim() || "no continent"}</span>
        <span className={`shrink-0 font-mono ${degree === 0 ? "text-gc-danger" : ""}`}>{degree}⇄</span>
      </div>
      <Handle type="source" position={Position.Right} style={{ ...handleStyle, background: color }} />
    </div>
  );
}

// ── Border edge: straight line between node centers ──────────────────────────

function BorderLine({ id, source, target, selected }: EdgeProps<BorderEdge>) {
  const { continentsByID } = useContext(CanvasContext);
  const a = useInternalNode<TerritoryNode>(source);
  const b = useInternalNode<TerritoryNode>(target);
  if (!a || !b) return null;

  const center = (n: typeof a) => ({
    x: n.internals.positionAbsolute.x + (n.measured.width ?? 0) / 2,
    y: n.internals.positionAbsolute.y + (n.measured.height ?? 0) / 2,
  });
  const pa = center(a);
  const pb = center(b);
  const [path] = getStraightPath({ sourceX: pa.x, sourceY: pa.y, targetX: pb.x, targetY: pb.y });

  const crossContinent = a.data.continentId !== b.data.continentId;
  const color = crossContinent ? "#8a9ab0" : (continentsByID.get(a.data.continentId)?.color ?? FALLBACK_COLOR);
  return (
    <BaseEdge
      id={id}
      path={path}
      interactionWidth={16}
      style={{
        stroke: selected ? "#d4a847" : color,
        strokeWidth: selected ? 3 : 2,
        strokeOpacity: selected ? 1 : 0.7,
        strokeDasharray: crossContinent ? "6 4" : undefined,
      }}
    />
  );
}

const nodeTypes: NodeTypes = { territory: TerritoryCard };
const edgeTypes: EdgeTypes = { border: BorderLine };

// ── Canvas ────────────────────────────────────────────────────────────────────

type MapEditorCanvasProps = {
  nodes: TerritoryNode[];
  edges: BorderEdge[];
  continents: EditorContinent[];
  onNodesChange: OnNodesChange<TerritoryNode>;
  onEdgesChange: OnEdgesChange<BorderEdge>;
  setNodes: Dispatch<SetStateAction<TerritoryNode[]>>;
  setEdges: Dispatch<SetStateAction<BorderEdge[]>>;
  /** Continent new territories are added to. */
  activeContinentID: string;
  onActiveContinentChange: (id: string) => void;
  className?: string;
};

export function MapEditorCanvas({
  nodes,
  edges,
  continents,
  onNodesChange,
  onEdgesChange,
  setNodes,
  setEdges,
  activeContinentID,
  onActiveContinentChange,
  className,
}: MapEditorCanvasProps) {
  const { screenToFlowPosition } = useReactFlow();

  const lookups = useMemo<CanvasLookups>(() => {
    const degreeByID = new Map<string, number>();
    for (const [id, ns] of neighborsByNode(edges)) degreeByID.set(id, ns.length);
    return { continentsByID: new Map(continents.map((c) => [c.id, c])), degreeByID };
  }, [continents, edges]);

  const addTerritoryAt = useCallback(
    (clientX: number, clientY: number) => {
      const continent = continents.find((c) => c.id === activeContinentID) ?? continents[0];
      if (!continent) return;
      const pos = screenToFlowPosition({ x: clientX, y: clientY });
      setNodes((ns) => {
        // Offset by roughly half a card so it lands centered on the cursor.
        const node = makeTerritory(uniqueTerritoryName(continent.name.trim() || "Territory", ns), continent.id, pos.x - 75, pos.y - 22);
        // Select only the new node so the inspector opens on it.
        return [...ns.map((n) => (n.selected ? { ...n, selected: false } : n)), { ...node, selected: true }];
      });
    },
    [activeContinentID, continents, screenToFlowPosition, setNodes]
  );

  const toggleBorder = useCallback(
    (a: string, b: string) => {
      if (a === b) return;
      const border = makeBorder(a, b);
      setEdges((es) => (es.some((e) => e.id === border.id) ? es.filter((e) => e.id !== border.id) : [...es, border]));
    },
    [setEdges]
  );

  const onConnect = useCallback(
    (conn: Connection) => {
      if (!conn.source || !conn.target || conn.source === conn.target) return;
      const border = makeBorder(conn.source, conn.target);
      setEdges((es) => (es.some((e) => e.id === border.id) ? es : [...es, border]));
    },
    [setEdges]
  );

  // Alt/⌥+click a second territory to toggle a border with the selected one.
  // (Not shift: that's React Flow's box-select key, which captures clicks.)
  const onNodeClick = useCallback<NodeMouseHandler<TerritoryNode>>(
    (event, node) => {
      if (!event.altKey) return;
      const others = nodes.filter((n) => n.selected && n.id !== node.id);
      if (others.length !== 1) return;
      toggleBorder(others[0].id, node.id);
      // Keep the original selection so several borders can be toggled in a row.
      setNodes((ns) => ns.map((n) => ({ ...n, selected: n.id === others[0].id })));
    },
    [nodes, setNodes, toggleBorder]
  );

  return (
    <CanvasContext.Provider value={lookups}>
      <div
        className={className}
        onDoubleClick={(e) => {
          if ((e.target as Element).classList.contains("react-flow__pane")) addTerritoryAt(e.clientX, e.clientY);
        }}
      >
        <ReactFlow<TerritoryNode, BorderEdge>
          nodes={nodes}
          edges={edges}
          onNodesChange={onNodesChange}
          onEdgesChange={onEdgesChange}
          onConnect={onConnect}
          onNodeClick={onNodeClick}
          nodeTypes={nodeTypes}
          edgeTypes={edgeTypes}
          connectionMode={ConnectionMode.Loose}
          connectionRadius={40}
          deleteKeyCode={["Backspace", "Delete"]}
          multiSelectionKeyCode={["Meta", "Control"]}
          zoomOnDoubleClick={false}
          // Select on click, not mousedown, so onNodeClick still sees the
          // previous selection when alt+click toggles a border.
          selectNodesOnDrag={false}
          minZoom={0.15}
          maxZoom={2.5}
          fitView
          fitViewOptions={{ padding: 0.2, maxZoom: 1 }}
          colorMode="dark"
          proOptions={{ hideAttribution: true }}
          style={{ background: "transparent" }}
        >
          <Background variant={BackgroundVariant.Dots} gap={20} size={1} color="#263348" />
          <Controls showInteractive={false} />
          <MiniMap
            pannable
            zoomable
            style={{ background: "#141c28", border: "1px solid #263348" }}
            maskColor="rgba(12, 17, 24, 0.6)"
            nodeColor={(n) => lookups.continentsByID.get((n as TerritoryNode).data.continentId)?.color ?? FALLBACK_COLOR}
          />
          <Panel position="top-left">
            <div className="flex items-center gap-2 rounded-lg border border-gc-border bg-gc-surface/95 px-2.5 py-1.5 text-xs text-gc-muted shadow">
              <span>Add to</span>
              <select
                className="rounded border border-gc-border bg-gc-surface-2 px-1.5 py-0.5 text-xs text-gc-text outline-none focus:border-gc-accent"
                value={activeContinentID}
                onChange={(e) => onActiveContinentChange(e.target.value)}
              >
                {continents.map((c) => (
                  <option key={c.id} value={c.id}>
                    {c.name.trim() || "(unnamed)"}
                  </option>
                ))}
              </select>
              <button
                type="button"
                className="rounded border border-gc-border px-2 py-0.5 text-gc-text hover:bg-gc-surface-2"
                onClick={(e) => {
                  const rect = (e.currentTarget.closest(".react-flow") as HTMLElement).getBoundingClientRect();
                  addTerritoryAt(rect.left + rect.width / 2, rect.top + rect.height / 2);
                }}
              >
                + Territory
              </button>
            </div>
          </Panel>
          <Panel position="bottom-center">
            <div className="rounded-md bg-gc-surface/90 px-2.5 py-1 text-[10px] text-gc-muted">
              Double-click to add · drag between dots to connect · alt+click to toggle a border · Delete removes
              selection
            </div>
          </Panel>
        </ReactFlow>
      </div>
    </CanvasContext.Provider>
  );
}
