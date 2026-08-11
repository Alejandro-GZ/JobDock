import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { applyNodeChanges, Background, Controls, Handle, Position, ReactFlow, type Edge, type Node, type NodeChange, type NodeProps } from "@xyflow/react";
import { Bell, Box, Boxes, Server } from "lucide-react";
import "@xyflow/react/dist/style.css";
import { HeartbeatIndicator } from "@/components/heartbeat-indicator";
import { StatusBadge } from "@/components/status-badge";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn, formatBytes, relative } from "@/lib/utils";
import { diagramJobs, isJobUnseen } from "@/lib/seen-jobs";
import type { Job, Node as ResourceNode } from "@/types";

type TopologyData = { kind: "resource" | "dock" | "job"; label: string; href?: string; status?: string; heartbeat?: string; node?: ResourceNode; job?: Job; unseen?: boolean };
type TopologyNode = Node<TopologyData>;
const border: Record<string, string> = { ONLINE: "border-emerald-500/70 shadow-emerald-500/10", DEGRADED: "border-yellow-500/70 shadow-yellow-500/10", DRAINING: "border-blue-500/70 shadow-blue-500/10", OFFLINE: "border-red-500/60 shadow-red-500/10", RUNNING: "border-blue-500/70", LOST: "border-red-500/70", FAILED: "border-red-500/70", SUCCEEDED: "border-emerald-500/70", CANCELLED: "border-zinc-500/60" };

function TopologyCard({ data }: NodeProps<TopologyNode>) {
  const navigate = useNavigate(), activate = () => data.href && navigate(data.href);
  return <Tooltip><TooltipTrigger asChild><div role={data.href ? "link" : undefined} tabIndex={data.href ? 0 : -1} onClick={activate} onKeyDown={(event) => { if ((event.key === "Enter" || event.key === " ") && data.href) { event.preventDefault(); activate(); } }} className={cn("relative min-w-44 cursor-grab rounded-xl border-2 bg-background/95 px-3 py-2 shadow-lg outline-none transition-[transform,border-color] duration-300 active:cursor-grabbing focus-visible:ring-2 focus-visible:ring-ring", data.kind === "dock" && "min-w-32 border-primary bg-primary/5", border[data.status ?? ""])}>
    <Handle type="target" position={Position.Left} className={data.kind === "resource" ? "opacity-0" : "!bg-muted-foreground"}/>
    <div className="flex items-center gap-2">{data.kind === "resource" && <span key={data.heartbeat} className={cn("grid size-8 place-items-center rounded-full bg-muted", data.status !== "OFFLINE" && "animate-heartbeat")}><Server className="size-4"/></span>}{data.kind === "dock" && <Boxes className="size-5 text-primary"/>}{data.kind === "job" && <Box className="size-4"/>}<span className="min-w-0 flex-1 truncate text-xs font-semibold">{data.label}</span>{data.kind === "resource" && data.heartbeat && <HeartbeatIndicator size="sm" status={data.status ?? "OFFLINE"} lastHeartbeat={data.heartbeat}/>} {data.unseen && <Bell className="size-4 fill-amber-400 text-amber-500"/>}</div>
    {data.kind === "job" && data.status && <div className="mt-2"><StatusBadge status={data.status}/></div>}<Handle type="source" position={Position.Right} className={data.kind === "job" ? "opacity-0" : "!bg-muted-foreground"}/>
  </div></TooltipTrigger><TooltipContent className="max-w-80">{data.node && <><p className="font-medium">{data.node.name}</p><p>{(data.node.cpu_allocated_millis / 1000).toFixed(1)} / {(data.node.cpu_total_millis / 1000).toFixed(1)} CPU · {formatBytes(data.node.memory_allocated_bytes)} / {formatBytes(data.node.memory_total_bytes)}</p><p>{data.node.gpus.filter((gpu) => gpu.allocated).length} / {data.node.gpus.length} GPUs · heartbeat {relative(data.node.last_heartbeat)}</p><p>Labels: {Object.entries(data.node.labels).map(([key, value]) => `${key}=${value}`).join(", ") || "none"}</p>{data.node.gpu_discovery.message && <p>{data.node.gpu_discovery.message}</p>}</>}{data.job && <><p className="font-medium">{data.job.spec.name}</p><p>{data.job.status} · {data.job.spec.image}</p><p>{data.job.spec.resources.cpu_millis / 1000} CPU · {formatBytes(data.job.spec.resources.memory_bytes)} · {data.job.spec.resources.gpu.count} GPUs</p><p>Node: {data.job.assigned_node_id ?? "unassigned"}</p></>}{data.kind === "dock" && <p>JobDock orchestration control plane</p>}</TooltipContent></Tooltip>;
}
const nodeTypes = { topology: TopologyCard };

export function buildTopology(resourceNodes: ResourceNode[], jobs: Job[], userId: string, seen: Record<string, string>) {
  const selected = diagramJobs(jobs, userId, seen), contentHeight = Math.max(resourceNodes.length, selected.length, 1) * 96, dockY = Math.max(40, contentHeight / 2);
  const resources: TopologyNode[] = resourceNodes.map((node, index) => ({ id: `resource-${node.id}`, type: "topology", position: { x: 20, y: 20 + index * 96 }, data: { kind: "resource", label: node.name, href: `/nodes?node=${node.id}`, status: node.status, heartbeat: node.last_heartbeat, node } }));
  const dock: TopologyNode = { id: "dock", type: "topology", position: { x: 380, y: dockY }, data: { kind: "dock", label: "Dock" } };
  const jobNodes: TopologyNode[] = selected.map((job, index) => ({ id: `job-${job.id}`, type: "topology", position: { x: 730, y: 20 + index * 96 }, data: { kind: "job", label: job.spec.name, href: `/jobs/${job.id}`, status: job.status, job, unseen: isJobUnseen(job, seen) } }));
  const edges: Edge[] = [...resourceNodes.map((node) => ({ id: `resource-${node.id}-dock`, source: `resource-${node.id}`, target: "dock", animated: node.status === "ONLINE", style: { strokeWidth: 1.5 } })), ...selected.map((job) => ({ id: `dock-job-${job.id}`, source: "dock", target: `job-${job.id}`, animated: ["ASSIGNED", "PULLING_IMAGE", "STARTING", "RUNNING", "STOPPING"].includes(job.status), style: { strokeWidth: 1.5 } }))];
  return { nodes: [...resources, dock, ...jobNodes], edges, height: Math.max(360, Math.min(620, contentHeight + 80)) };
}

export function ResourceTopology({ resourceNodes, jobs, userId, seen }: { resourceNodes: ResourceNode[]; jobs: Job[]; userId: string; seen: Record<string, string> }) {
  const topology = useMemo(() => buildTopology(resourceNodes, jobs, userId, seen), [resourceNodes, jobs, userId, seen]);
  const canonical = useMemo(() => new Map(topology.nodes.map((node) => [node.id, node.position])), [topology.nodes]);
  const [flowNodes, setFlowNodes] = useState<TopologyNode[]>(topology.nodes);
  useEffect(() => setFlowNodes(topology.nodes), [topology.nodes]);
  const clamp = (node: TopologyNode) => { const home = canonical.get(node.id); if (!home) return; setFlowNodes((current) => current.map((item) => item.id === node.id ? { ...item, position: { x: Math.max(home.x - 40, Math.min(home.x + 40, node.position.x)), y: Math.max(home.y - 40, Math.min(home.y + 40, node.position.y)) } } : item)); };
  const reset = (node: TopologyNode) => { const home = canonical.get(node.id); if (home) setFlowNodes((current) => current.map((item) => item.id === node.id ? { ...item, position: home } : item)); };
  return <section><div className="mb-3 flex items-center justify-between"><h2 className="text-sm font-semibold">Resource topology</h2><span className="text-xs text-muted-foreground">Drag nodes to inspect · they snap back</span></div><div className="overflow-hidden rounded-md border bg-muted/10" style={{ height: topology.height }}><ReactFlow nodes={flowNodes} edges={topology.edges} nodeTypes={nodeTypes} onNodesChange={(changes: NodeChange<TopologyNode>[]) => setFlowNodes((nodes) => applyNodeChanges(changes, nodes))} onNodeDrag={(_, node) => clamp(node)} onNodeDragStop={(_, node) => reset(node)} fitView fitViewOptions={{ padding: .22 }} minZoom={.45} maxZoom={1.5} nodesConnectable={false} deleteKeyCode={null} proOptions={{ hideAttribution: true }}><Background gap={24} size={1}/><Controls showInteractive={false} position="bottom-right"/></ReactFlow></div></section>;
}
