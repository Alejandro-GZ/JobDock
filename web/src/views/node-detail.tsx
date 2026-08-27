import type { ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { Activity, ArrowLeft, Boxes, CircuitBoard, Container, Cpu, Database, HardDrive, MemoryStick, Server, Thermometer } from "lucide-react";
import { toast } from "sonner";
import { api } from "@/api";
import { HeartbeatIndicator } from "@/components/heartbeat-indicator";
import { StatusBadge } from "@/components/status-badge";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { cn, formatBytes, relative } from "@/lib/utils";
import type { NodeAllocation, User } from "@/types";

const clamp = (value: number) => Math.max(0, Math.min(100, value));
const cores = (value: number) => `${(value / 1000).toFixed(value % 1000 ? 1 : 0)} cores`;
const percent = (value?: number) => value == null ? "Unavailable" : `${(value / 100).toFixed(0)}%`;

function ResourceMeter({ icon, label, total, reserved, observed, format, unavailable = false }: { icon: ReactNode; label: string; total: number; reserved?: number; observed: number; format: (value: number) => string; unavailable?: boolean }) {
  const observedWidth = total > 0 ? clamp(observed / total * 100) : 0;
  const reservedWidth = total > 0 && reserved != null ? clamp(reserved / total * 100) : 0;
  return <section className="min-w-0 space-y-2 border-r px-4 py-3 last:border-r-0">
    <div className="flex items-center gap-2 text-xs text-muted-foreground">{icon}<span>{label}</span><strong className="ml-auto font-mono text-foreground">{unavailable ? "N/A" : `${Math.round(observedWidth)}%`}</strong></div>
    <div className="relative h-2 overflow-hidden rounded-full bg-muted" role="meter" aria-label={`${label} observed usage`} aria-valuemin={0} aria-valuemax={total} aria-valuenow={observed}>
      {reserved != null && <div className="absolute inset-y-0 left-0 bg-amber-400/35" style={{ width: `${reservedWidth}%` }}/>}<div className="absolute inset-y-0 left-0 bg-primary" style={{ width: `${observedWidth}%` }}/>
    </div>
    <div className="flex justify-between gap-2 text-[10px] text-muted-foreground"><span>{format(observed)} observed</span>{reserved != null && <span>{format(reserved)} reserved</span>}<span>{total > 0 ? `${format(total)} total` : "Total unavailable"}</span></div>
  </section>;
}

function Pair({ label, value, mono = false }: { label: string; value?: string | number; mono?: boolean }) {
  return <div className="grid grid-cols-[9rem_1fr] gap-3 border-b py-2 last:border-0"><dt className="text-xs text-muted-foreground">{label}</dt><dd className={cn("min-w-0 break-words text-xs", mono && "font-mono")}>{value === "" || value == null ? "Unavailable" : value}</dd></div>;
}

function Section({ title, icon, children, open = false }: { title: string; icon: ReactNode; children: ReactNode; open?: boolean }) {
  return <details open={open} className="group overflow-hidden rounded-md border"><summary className="flex cursor-pointer list-none items-center gap-2 px-4 py-3 text-sm font-medium hover:bg-muted/40">{icon}{title}<span className="ml-auto text-xs text-muted-foreground group-open:hidden">Expand</span><span className="ml-auto hidden text-xs text-muted-foreground group-open:inline">Collapse</span></summary><div className="border-t">{children}</div></details>;
}

function TelemetryBadge({ value }: { value: NodeAllocation["telemetry_status"] }) {
  const tone = value === "fresh" ? "border-emerald-500/40 text-emerald-600" : value === "stale" ? "border-amber-500/40 text-amber-600" : "text-muted-foreground";
  return <Badge variant="outline" className={cn("text-[10px]", tone)}>{value}</Badge>;
}

export function NodeDetail({ user }: { user: User }) {
  const { id = "" } = useParams();
  const qc = useQueryClient();
  const query = useQuery({ queryKey: ["node", id], queryFn: () => api.node(id), refetchInterval: 2000, placeholderData: (previous) => previous });
  const action = useMutation({ mutationFn: (kind: "drain" | "resume") => api.setNode(id, kind), onSuccess: (_, kind) => { toast.success(`Node ${kind === "drain" ? "drained" : "resumed"}`); qc.invalidateQueries({ queryKey: ["node", id] }); qc.invalidateQueries({ queryKey: ["nodes"] }); }, onError: (error: Error) => toast.error(error.message) });
  if (query.isLoading) return <div className="space-y-3"><div className="h-10 animate-pulse rounded bg-muted"/><div className="h-32 animate-pulse rounded bg-muted"/><div className="h-64 animate-pulse rounded bg-muted"/></div>;
  if (!query.data) return <div className="space-y-4"><Button asChild variant="ghost" size="sm"><Link to="/nodes"><ArrowLeft className="size-4"/>Back to nodes</Link></Button><div className="rounded-md border border-destructive/30 p-6"><h1 className="font-semibold">Node unavailable</h1><p className="mt-1 text-sm text-muted-foreground">{query.error instanceof Error ? query.error.message : "The node no longer exists or cannot be read."}</p></div></div>;
  const { node, usage, allocations } = query.data;
  const allocatedGPUs = node.gpus.filter((gpu) => gpu.allocated).length;
  const observedGPUCapacity = node.gpus.length ? node.gpus.reduce((total, gpu) => total + (gpu.utilization_basis_points ?? 0), 0) / node.gpus.length : 0;
  const workspaceTotal = node.workspace_total_bytes ?? 0;
  const workspaceUsed = workspaceTotal > 0 ? Math.max(0, workspaceTotal - node.workspace_free_bytes) : 0;
  const system = node.system ?? {};
  const runtime = node.runtime ?? {};
  return <div className="space-y-4">
    <header className="flex items-start gap-3"><Button asChild variant="ghost" size="icon"><Link to="/nodes" aria-label="Back to nodes"><ArrowLeft className="size-4"/></Link></Button><div className="min-w-0 flex-1"><div className="flex items-center gap-2"><h1 className="truncate text-xl font-semibold">{node.name}</h1><HeartbeatIndicator status={node.status} lastHeartbeat={node.last_heartbeat}/><Badge variant="outline" className="font-mono text-[10px]">{node.status}</Badge></div><div className="mt-1 flex flex-wrap gap-x-3 gap-y-1 font-mono text-[10px] text-muted-foreground"><span>{node.id}</span><span>Heartbeat {relative(node.last_heartbeat)}</span></div></div>{user.role === "admin" && <Button size="sm" variant="outline" disabled={action.isPending} onClick={() => action.mutate(node.status === "DRAINING" ? "resume" : "drain")}>{node.status === "DRAINING" ? "Resume scheduling" : "Drain node"}</Button>}</header>

    <div className="grid overflow-hidden rounded-md border md:grid-cols-2 xl:grid-cols-4">
      <ResourceMeter icon={<Cpu className="size-3.5"/>} label="CPU" total={node.cpu_total_millis} reserved={node.cpu_allocated_millis} observed={usage.cpu_observed_millis} format={cores}/>
      <ResourceMeter icon={<MemoryStick className="size-3.5"/>} label="Memory" total={node.memory_total_bytes} reserved={node.memory_allocated_bytes} observed={usage.memory_observed_bytes} format={formatBytes}/>
      <ResourceMeter icon={<CircuitBoard className="size-3.5"/>} label="GPU capacity" total={10000} reserved={node.gpus.length ? allocatedGPUs / node.gpus.length * 10000 : 0} observed={observedGPUCapacity} format={(value) => `${Math.round(value / 100)}%`} unavailable={!node.gpus.length}/>
      <ResourceMeter icon={<HardDrive className="size-3.5"/>} label="Workspace" total={workspaceTotal} observed={workspaceUsed} format={formatBytes} unavailable={!workspaceTotal}/>
    </div>
    <div className="flex flex-wrap items-center gap-4 px-1 text-[10px] text-muted-foreground"><span className="flex items-center gap-1"><i className="size-2 rounded-full bg-primary"/>Observed JobDock usage</span><span className="flex items-center gap-1"><i className="size-2 rounded-full bg-amber-400/50"/>Scheduler reservation</span><span>{usage.latest_telemetry_at ? `Latest job telemetry ${relative(usage.latest_telemetry_at)}` : "No job telemetry reported"}</span></div>

    <Section title={`Active allocations (${allocations.length})`} icon={<Activity className="size-4"/>} open><div className="overflow-auto"><Table><TableHeader><TableRow><TableHead>Job</TableHead><TableHead>Status</TableHead><TableHead>CPU reserved / observed</TableHead><TableHead>Memory reserved / observed</TableHead><TableHead>Placement</TableHead><TableHead>Telemetry</TableHead></TableRow></TableHeader><TableBody>{allocations.length ? allocations.map((item) => <TableRow key={item.job_id}><TableCell><div className="font-medium">{item.job_name}</div>{item.can_open ? <Link to={`/jobs/${item.job_id}`} className="font-mono text-[10px] text-primary hover:underline">{item.job_id}</Link> : <span className="font-mono text-[10px] text-muted-foreground">{item.job_id}</span>}</TableCell><TableCell><StatusBadge status={item.status}/></TableCell><TableCell><div>{cores(item.reserved_cpu_millis)}</div><div className="text-[10px] text-muted-foreground">{item.observed_cpu_millis == null ? item.telemetry_status === "restricted" ? "Restricted" : "Unavailable" : cores(item.observed_cpu_millis)}</div></TableCell><TableCell><div>{formatBytes(item.reserved_memory_bytes)}</div><div className="text-[10px] text-muted-foreground">{item.observed_memory_bytes == null ? item.telemetry_status === "restricted" ? "Restricted" : "Unavailable" : formatBytes(item.observed_memory_bytes)}</div></TableCell><TableCell className="text-xs"><div>{item.cpu_package_id ? `Socket ${item.cpu_package_id}` : "Automatic CPU"}{item.cpu_set && ` · CPUs ${item.cpu_set}`}</div><div className="max-w-72 truncate font-mono text-[10px] text-muted-foreground" title={item.gpu_uuids.join(", ")}>{item.gpu_uuids.length ? item.gpu_uuids.join(", ") : "No GPU"}</div></TableCell><TableCell><TelemetryBadge value={item.telemetry_status}/><div className="mt-1 text-[10px] text-muted-foreground">{item.telemetry_captured_at ? relative(item.telemetry_captured_at) : "No sample"}</div></TableCell></TableRow>) : <TableRow><TableCell colSpan={6} className="h-20 text-center text-muted-foreground">No active jobs reserve resources on this node.</TableCell></TableRow>}</TableBody></Table></div></Section>

    <div className="grid gap-4 xl:grid-cols-2">
      <Section title={`CPU inventory (${node.cpu_packages.length || 1} package${node.cpu_packages.length === 1 ? "" : "s"})`} icon={<Cpu className="size-4"/>} open><div className="overflow-auto"><Table><TableHeader><TableRow><TableHead>Socket</TableHead><TableHead>Processor</TableHead><TableHead>Topology</TableHead><TableHead>Reserved</TableHead></TableRow></TableHeader><TableBody>{node.cpu_packages.length ? node.cpu_packages.map((cpu) => <TableRow key={cpu.id}><TableCell className="font-mono">{cpu.id}</TableCell><TableCell><div>{cpu.model || "Unavailable"}</div><div className="text-[10px] text-muted-foreground">{cpu.vendor || "Unknown vendor"}</div></TableCell><TableCell><div>{cpu.physical_cores} cores · {cpu.logical_cpus.length} threads</div><div className="max-w-72 break-words font-mono text-[10px] text-muted-foreground">Logical CPUs: {cpu.logical_cpus.join(", ")}</div></TableCell><TableCell>{cores(cpu.allocated_millis)} / {cores(cpu.total_millis)}</TableCell></TableRow>) : <TableRow><TableCell colSpan={4} className="h-20 text-center text-muted-foreground">Detailed CPU topology requires an agent with host_inventory_v1.</TableCell></TableRow>}</TableBody></Table></div></Section>
      <Section title={`GPU inventory (${node.gpus.length})`} icon={<CircuitBoard className="size-4"/>} open><div className="overflow-auto"><Table><TableHeader><TableRow><TableHead>Device</TableHead><TableHead>Architecture</TableHead><TableHead>Live</TableHead><TableHead>Allocation</TableHead></TableRow></TableHeader><TableBody>{node.gpus.length ? node.gpus.map((gpu) => <TableRow key={gpu.uuid}><TableCell><div>{gpu.model || "NVIDIA GPU"}</div><div className="font-mono text-[10px] text-muted-foreground" title={gpu.uuid}>{gpu.uuid}</div></TableCell><TableCell><div>{formatBytes(gpu.vram_bytes)} VRAM · CC {gpu.compute_capability || "N/A"}</div><div className="font-mono text-[10px] text-muted-foreground">{gpu.pci_bus_id || "PCI unavailable"} · driver {gpu.driver_version || "N/A"}</div></TableCell><TableCell><div>{percent(gpu.utilization_basis_points)} · {gpu.memory_used_bytes == null ? "Unavailable" : formatBytes(gpu.memory_used_bytes)}</div><div className="flex items-center gap-1 text-[10px] text-muted-foreground"><Thermometer className="size-3"/>{gpu.temperature_celsius == null ? "Unavailable" : `${gpu.temperature_celsius} °C`}</div></TableCell><TableCell>{gpu.allocated_job_id ? <div><Badge variant="secondary">Reserved</Badge>{allocations.find((item) => item.job_id === gpu.allocated_job_id)?.can_open ? <Link className="mt-1 block font-mono text-[10px] text-primary hover:underline" to={`/jobs/${gpu.allocated_job_id}`}>{gpu.allocated_job_id}</Link> : <span className="mt-1 block font-mono text-[10px] text-muted-foreground">{gpu.allocated_job_id}</span>}</div> : <Badge variant="outline">Free</Badge>}</TableCell></TableRow>) : <TableRow><TableCell colSpan={4} className="h-20 text-center text-muted-foreground">{node.gpu_discovery.message || "No NVIDIA GPUs were reported."}</TableCell></TableRow>}</TableBody></Table></div></Section>
    </div>

    <div className="grid gap-4 xl:grid-cols-2">
      <Section title="System architecture" icon={<Server className="size-4"/>}><dl className="px-4"><Pair label="Docker host" value={system.hostname}/><Pair label="Operating system" value={system.operating_system}/><Pair label="OS version" value={system.os_version}/><Pair label="OS type" value={system.os_type}/><Pair label="Kernel" value={system.kernel_version} mono/><Pair label="Architecture" value={system.architecture || node.architecture}/><Pair label="Memory" value={formatBytes(node.memory_total_bytes)}/></dl></Section>
      <Section title="Container runtime" icon={<Container className="size-4"/>}><dl className="px-4"><Pair label="Docker Engine" value={runtime.docker_version || node.docker_version}/><Pair label="Storage driver" value={runtime.storage_driver}/><Pair label="Cgroup driver" value={runtime.cgroup_driver}/><Pair label="Cgroup version" value={runtime.cgroup_version}/><Pair label="Agent" value={node.agent_version}/><Pair label="Protocol" value={node.protocol_version}/></dl></Section>
      <Section title="Workspace and diagnostics" icon={<Database className="size-4"/>}><dl className="px-4"><Pair label="Workspace total" value={workspaceTotal ? formatBytes(workspaceTotal) : undefined}/><Pair label="Workspace used" value={workspaceTotal ? formatBytes(workspaceUsed) : undefined}/><Pair label="Workspace free" value={formatBytes(node.workspace_free_bytes)}/><Pair label="GPU discovery" value={node.gpu_discovery.status}/><Pair label="Diagnostic code" value={node.gpu_discovery.error_code}/><Pair label="Diagnostic" value={node.gpu_discovery.message}/></dl></Section>
      <Section title="Metadata" icon={<Boxes className="size-4"/>}><div className="space-y-4 px-4 py-3"><div><h3 className="mb-2 text-xs text-muted-foreground">Labels</h3><div className="flex flex-wrap gap-1">{Object.entries(node.labels ?? {}).length ? Object.entries(node.labels ?? {}).map(([key, value]) => <Badge key={key} variant="secondary" className="font-mono text-[10px]">{key}={value}</Badge>) : <span className="text-xs text-muted-foreground">No labels</span>}</div></div><div><h3 className="mb-2 text-xs text-muted-foreground">Capabilities</h3><div className="flex flex-wrap gap-1">{(node.capabilities ?? []).length ? (node.capabilities ?? []).map((capability) => <Badge key={capability} variant="outline" className="font-mono text-[10px]">{capability}</Badge>) : <span className="text-xs text-muted-foreground">Legacy agent; no capabilities reported</span>}</div></div></div></Section>
    </div>
  </div>;
}
