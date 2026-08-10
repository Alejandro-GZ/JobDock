import { useQuery } from "@tanstack/react-query";
import { AlertTriangle, Cpu, HardDrive, Server } from "lucide-react";
import { api } from "@/api";
import { JobTable } from "@/components/job-table";
import { Progress } from "@/components/ui/progress";
import { StatusBadge } from "@/components/status-badge";
import { formatBytes } from "@/lib/utils";

export function Dashboard() { const jobs=useQuery({queryKey:["jobs"],queryFn:api.jobs});const nodes=useQuery({queryKey:["nodes"],queryFn:api.nodes,refetchInterval:10000});const ns=nodes.data??[];const js=jobs.data??[];
  const cpuTotal=ns.reduce((n,x)=>n+x.cpu_total_millis,0),cpuUsed=ns.reduce((n,x)=>n+x.cpu_allocated_millis,0),memTotal=ns.reduce((n,x)=>n+x.memory_total_bytes,0),memUsed=ns.reduce((n,x)=>n+x.memory_allocated_bytes,0),gpuTotal=ns.reduce((n,x)=>n+x.gpus.length,0),gpuUsed=ns.reduce((n,x)=>n+x.gpus.filter(g=>g.allocated).length,0);
  const metrics=[{label:"CPU",value:cpuUsed,total:cpuTotal,text:`${(cpuUsed/1000).toFixed(1)} / ${(cpuTotal/1000).toFixed(1)} cores`,icon:Cpu},{label:"Memory",value:memUsed,total:memTotal,text:`${formatBytes(memUsed)} / ${formatBytes(memTotal)}`,icon:HardDrive},{label:"GPU",value:gpuUsed,total:gpuTotal,text:`${gpuUsed} / ${gpuTotal}`,icon:Server}];
  return <div className="space-y-6"><div className="flex items-center justify-between"><h1 className="text-xl font-semibold tracking-tight">Overview</h1><span className="text-xs text-muted-foreground">{ns.filter(n=>n.status==="ONLINE").length} online · {js.filter(j=>j.status==="QUEUED").length} queued</span></div>
    <section className="grid overflow-hidden rounded-md border md:grid-cols-3">{metrics.map(({label,value,total,text,icon:Icon})=><div key={label} className="border-b p-4 last:border-0 md:border-b-0 md:border-r"><div className="mb-3 flex items-center justify-between text-xs"><span className="flex items-center gap-2 font-medium"><Icon className="size-4 text-muted-foreground"/>{label}</span><span className="font-mono text-muted-foreground">{text}</span></div><Progress value={total?value/total*100:0}/></div>)}</section>
    <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_320px]"><section><h2 className="mb-3 text-sm font-semibold">Active and recent jobs</h2><JobTable jobs={js.slice(0,10)}/></section><aside><h2 className="mb-3 text-sm font-semibold">Pool health</h2><div className="divide-y rounded-md border">{ns.map(node=><div key={node.id} className="p-3"><div className="flex items-center justify-between"><span className="truncate text-sm font-medium">{node.name}</span><StatusBadge status={node.status}/></div><div className="mt-2 flex justify-between text-xs text-muted-foreground"><span>{node.gpus.length} GPUs</span><span>{formatBytes(node.workspace_free_bytes)} free</span></div>{node.gpu_discovery.error_code&&<div className="mt-2 flex gap-2 text-xs text-amber-600"><AlertTriangle className="mt-0.5 size-3 shrink-0"/>{node.gpu_discovery.message}</div>}</div>)}{!ns.length&&<p className="p-6 text-center text-sm text-muted-foreground">No agents enrolled.</p>}</div></aside></div>
  </div>;
}
