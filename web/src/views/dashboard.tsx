import { lazy, Suspense, useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Cpu, HardDrive, Server } from "lucide-react";
import { api } from "@/api";
import { JobTable } from "@/components/job-table";
import { ResourceGauge } from "@/components/resource-gauge";
import { formatBytes } from "@/lib/utils";
import { isJobUnseen, readSeenJobs } from "@/lib/seen-jobs";
import type { User } from "@/types";
const ResourceTopology = lazy(() => import("@/components/resource-topology").then((module) => ({ default: module.ResourceTopology })));

export function Dashboard({ user }: { user: User }) {
  const jobs = useQuery({ queryKey: ["jobs"], queryFn: api.jobs });
  const nodes = useQuery({ queryKey: ["nodes"], queryFn: api.nodes, refetchInterval: 2000 });
  const [seen, setSeen] = useState(() => readSeenJobs(user.id));
  useEffect(() => {
    const refresh = () => setSeen(readSeenJobs(user.id));
    window.addEventListener("jobdock:seen-jobs-changed", refresh);
    window.addEventListener("storage", refresh);
    return () => { window.removeEventListener("jobdock:seen-jobs-changed", refresh); window.removeEventListener("storage", refresh); };
  }, [user.id]);
  const ns = nodes.data ?? [], js = jobs.data ?? [];
  const cpuTotal = ns.reduce((n, x) => n + x.cpu_total_millis, 0), cpuUsed = ns.reduce((n, x) => n + x.cpu_allocated_millis, 0);
  const memTotal = ns.reduce((n, x) => n + x.memory_total_bytes, 0), memUsed = ns.reduce((n, x) => n + x.memory_allocated_bytes, 0);
  const gpuTotal = ns.reduce((n, x) => n + x.gpus.length, 0), gpuUsed = ns.reduce((n, x) => n + x.gpus.filter((gpu) => gpu.allocated).length, 0);
  const unseen = new Set(js.filter((job) => job.owner_id === user.id && isJobUnseen(job, seen)).map((job) => job.id));
  return <div className="space-y-6">
    <div className="flex items-center justify-between"><h1 className="text-xl font-semibold tracking-tight">Overview</h1><span className="text-xs text-muted-foreground">{ns.filter((node) => node.status === "ONLINE").length} online · {js.filter((job) => job.status === "QUEUED").length} queued</span></div>
    <section className="grid overflow-hidden rounded-md border sm:grid-cols-3">
      <ResourceGauge label="CPU" value={cpuUsed} total={cpuTotal} display={`${(cpuUsed / 1000).toFixed(1)} / ${(cpuTotal / 1000).toFixed(1)} cores`} icon={Cpu}/>
      <ResourceGauge label="Memory" value={memUsed} total={memTotal} display={`${formatBytes(memUsed)} / ${formatBytes(memTotal)}`} icon={HardDrive}/>
      <ResourceGauge label="GPU" value={gpuUsed} total={gpuTotal} display={`${gpuUsed} / ${gpuTotal} devices`} icon={Server}/>
    </section>
    <Suspense fallback={<div className="h-[360px] animate-pulse rounded-md border bg-muted/30"/>}><ResourceTopology resourceNodes={ns} jobs={js} userId={user.id} seen={seen}/></Suspense>
    <section><h2 className="mb-3 text-sm font-semibold">Active and recent jobs</h2><JobTable jobs={js.slice(0, 10)} unseenJobIds={unseen}/></section>
  </div>;
}
