import { lazy, Suspense, useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Cpu, ExternalLink, Gauge, HardDrive, ListTodo, Server } from "lucide-react";
import { useNavigate } from "react-router-dom";
import { api } from "@/api";
import { JobTable } from "@/components/job-table";
import { ResourceGauge } from "@/components/resource-gauge";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { seriesQuery } from "@/lib/series";
import { isJobUnseen, readSeenJobs } from "@/lib/seen-jobs";
import { cn, formatBytes } from "@/lib/utils";
import type { TopologyAnchor, TopologySelection } from "@/components/resource-topology";
import type { Job, Node, ResourcePoint, User } from "@/types";

const ResourceTopology = lazy(() => import("@/components/resource-topology").then(module => ({ default: module.ResourceTopology })));

export function Dashboard({ user }: { user: User }) {
  const navigate = useNavigate(), jobs = useQuery({ queryKey: ["jobs"], queryFn: api.jobs }), nodes = useQuery({ queryKey: ["nodes"], queryFn: api.nodes, refetchInterval: 2000 });
  const [seen, setSeen] = useState(() => readSeenJobs(user.id)), [poolOpen, setPoolOpen] = useState(false), [jobsOpen, setJobsOpen] = useState(false), [selection, setSelection] = useState<TopologySelection>();
  useEffect(() => {
    const refresh = () => setSeen(readSeenJobs(user.id));
    window.addEventListener("jobdock:seen-jobs-changed", refresh); window.addEventListener("storage", refresh);
    return () => { window.removeEventListener("jobdock:seen-jobs-changed", refresh); window.removeEventListener("storage", refresh); };
  }, [user.id]);
  const resourceNodes = nodes.data ?? [], allJobs = jobs.data ?? [];
  const selectedNode = selection?.kind === "resource" ? resourceNodes.find(node => node.id === selection.node.id) ?? selection.node : undefined;
  const selectedJob = selection?.kind === "job" ? allJobs.find(job => job.id === selection.job.id) ?? selection.job : undefined;
  const selectedHref = selection?.kind === "resource" ? `/nodes?node=${selectedNode?.id}` : selection?.kind === "job" ? `/jobs/${selectedJob?.id}` : "";
  const jobResources = useQuery({ queryKey: ["overview-job-resources", selectedJob?.id, selectedJob?.attempt_id], queryFn: () => api.resources(selectedJob!.id, seriesQuery(selectedJob!, "all", "auto", Date.now())), enabled: Boolean(selectedJob?.attempt_id), refetchInterval: selectedJob && !selectedJob.finished_at ? 5000 : false });
  const latest = jobResources.data?.points.at(-1);
  const pool = poolValues(resourceNodes), unseen = new Set(allJobs.filter(job => job.owner_id === user.id && isJobUnseen(job, seen)).map(job => job.id));
  return <div className="relative h-dvh min-h-0 overflow-hidden">
    <Suspense fallback={<div className="h-full animate-pulse bg-muted/30"/>}><ResourceTopology resourceNodes={resourceNodes} jobs={allJobs} userId={user.id} seen={seen} onSelectionChange={setSelection}/></Suspense>

    <div className="pointer-events-none absolute inset-x-0 top-0 z-20 flex flex-col items-center">
      {poolOpen && <section className="pointer-events-auto grid h-40 w-[min(48rem,calc(100%-1.5rem))] grid-cols-3 overflow-hidden rounded-b-lg border border-t-0 bg-background/95 shadow-lg backdrop-blur"><ResourceGauge label="CPU" value={pool.cpuUsed} total={pool.cpuTotal} display={`${(pool.cpuUsed / 1000).toFixed(1)} / ${(pool.cpuTotal / 1000).toFixed(1)} cores`} icon={Cpu}/><ResourceGauge label="Memory" value={pool.memUsed} total={pool.memTotal} display={`${formatBytes(pool.memUsed)} / ${formatBytes(pool.memTotal)}`} icon={HardDrive}/><ResourceGauge label="GPU" value={pool.gpuUsed} total={pool.gpuTotal} display={`${pool.gpuUsed} / ${pool.gpuTotal} devices`} icon={Server}/></section>}
      <Tooltip><TooltipTrigger asChild><button type="button" aria-label="Toggle pool usage" aria-expanded={poolOpen} onClick={() => { setPoolOpen(value => !value); setJobsOpen(false); }} className="pointer-events-auto -mt-px grid h-9 w-16 place-items-center rounded-b-md border border-t-0 bg-background/95 shadow-sm backdrop-blur"><Gauge className="size-4"/></button></TooltipTrigger><TooltipContent side="bottom">Pool usage</TooltipContent></Tooltip>
    </div>

    {selection && <SelectionGauges node={selectedNode} job={selectedJob} latest={latest} anchor={selection.anchor} onOpen={() => navigate(selectedHref)}/>}

    <div className="pointer-events-none absolute inset-x-0 bottom-0 z-20 flex flex-col-reverse items-center">
      {jobsOpen && <section className="pointer-events-auto max-h-[45dvh] w-[min(68rem,calc(100%-1.5rem))] overflow-auto rounded-t-lg border border-b-0 bg-background/95 shadow-lg backdrop-blur"><JobTable jobs={allJobs.slice(0, 10)} unseenJobIds={unseen}/></section>}
      <Tooltip><TooltipTrigger asChild><button type="button" aria-label="Toggle active and recent jobs" aria-expanded={jobsOpen} onClick={() => { setJobsOpen(value => !value); setPoolOpen(false); }} className="pointer-events-auto -mb-px grid h-9 w-16 place-items-center rounded-t-md border border-b-0 bg-background/95 shadow-sm backdrop-blur"><ListTodo className="size-4"/></button></TooltipTrigger><TooltipContent side="top">Active and recent jobs</TooltipContent></Tooltip>
    </div>
  </div>;
}

function SelectionGauges({ node, job, latest, anchor, onOpen }: { node?: Node; job?: Job; latest?: ResourcePoint; anchor: TopologyAnchor; onOpen: () => void }) {
  const gauges = node ? nodeGauges(node) : job ? jobGauges(job, latest) : [];
  const title = node?.name ?? job?.spec.name ?? "Selection";
  const above = anchor.side === "top";
  return <section onDoubleClick={onOpen} style={{ left: anchor.panelX, top: above ? anchor.y - 14 : anchor.y + 14 }} className={cn("fixed z-10 w-[min(38rem,calc(100vw-2rem))] -translate-x-1/2 rounded-lg border bg-background/95 p-2 shadow-xl backdrop-blur", above ? "-translate-y-full" : "translate-y-0")}><span aria-hidden style={{ left: `calc(50% + ${anchor.x - anchor.panelX}px)` }} className={cn("absolute size-3 -translate-x-1/2 rotate-45 bg-background", above ? "-bottom-1.5 border-b border-r" : "-top-1.5 border-l border-t")}/><div className="relative flex h-7 items-center justify-between px-1"><div><span className="text-xs font-semibold">{title}</span><span className="ml-2 text-[10px] text-muted-foreground">{node ? "Node telemetry" : "Latest job telemetry"}</span></div><Tooltip><TooltipTrigger asChild><Button type="button" variant="ghost" size="icon" className="size-6" onClick={event => { event.stopPropagation(); onOpen(); }} aria-label={`Open ${title}`}><ExternalLink className="size-3.5"/></Button></TooltipTrigger><TooltipContent>Open details</TooltipContent></Tooltip></div><div className="relative grid h-32 grid-cols-3 overflow-hidden">{gauges.map(gauge => <ResourceGauge key={gauge.label} {...gauge}/>)}</div></section>;
}

function poolValues(nodes: Node[]) {
  return { cpuTotal: nodes.reduce((total, node) => total + node.cpu_total_millis, 0), cpuUsed: nodes.reduce((total, node) => total + node.cpu_allocated_millis, 0), memTotal: nodes.reduce((total, node) => total + node.memory_total_bytes, 0), memUsed: nodes.reduce((total, node) => total + node.memory_allocated_bytes, 0), gpuTotal: nodes.reduce((total, node) => total + node.gpus.length, 0), gpuUsed: nodes.reduce((total, node) => total + node.gpus.filter(gpu => gpu.allocated).length, 0) };
}

function nodeGauges(node: Node) {
  return [
    { label: "CPU", value: node.cpu_allocated_millis, total: node.cpu_total_millis, display: `${(node.cpu_allocated_millis / 1000).toFixed(1)} / ${(node.cpu_total_millis / 1000).toFixed(1)} cores`, icon: Cpu },
    { label: "Memory", value: node.memory_allocated_bytes, total: node.memory_total_bytes, display: `${formatBytes(node.memory_allocated_bytes)} / ${formatBytes(node.memory_total_bytes)}`, icon: HardDrive },
    { label: "GPU", value: node.gpus.filter(gpu => gpu.allocated).length, total: node.gpus.length, display: `${node.gpus.filter(gpu => gpu.allocated).length} / ${node.gpus.length} devices`, icon: Server },
  ];
}

function jobGauges(job: Job, latest?: ResourcePoint) {
  const sampled = Boolean(latest), gpuSampled = latest?.gpu_utilization_basis_points != null;
  return [
    { label: "CPU", value: latest?.cpu_millis ?? 0, total: sampled ? job.spec.resources.cpu_millis : 0, display: sampled ? `${((latest?.cpu_millis ?? 0) / 1000).toFixed(2)} / ${(job.spec.resources.cpu_millis / 1000).toFixed(2)} cores` : `No sample · ${job.spec.resources.cpu_millis / 1000} cores reserved`, icon: Cpu },
    { label: "Memory", value: latest?.memory_bytes ?? 0, total: sampled ? job.spec.resources.memory_bytes : 0, display: sampled ? `${formatBytes(latest?.memory_bytes ?? 0)} / ${formatBytes(job.spec.resources.memory_bytes)}` : `No sample · ${formatBytes(job.spec.resources.memory_bytes)} reserved`, icon: HardDrive },
    { label: "GPU", value: gpuSampled ? (latest!.gpu_utilization_basis_points ?? 0) / 100 : 0, total: gpuSampled ? 100 : 0, display: gpuSampled ? `${((latest!.gpu_utilization_basis_points ?? 0) / 100).toFixed(1)}% · ${job.spec.resources.gpu.count} devices` : `No sample · ${job.spec.resources.gpu.count} devices reserved`, icon: Server },
  ];
}
