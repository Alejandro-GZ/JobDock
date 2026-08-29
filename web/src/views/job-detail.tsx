import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useParams } from "react-router-dom";
import { ArrowLeft, Cpu, Download, FileDown, HardDrive, History, LayoutTemplate, Package, PencilRuler, RotateCcw, Server, Square, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { api } from "@/api";
import { MetricsPanel } from "@/components/metrics-panel";
import { ResourceGauge } from "@/components/resource-gauge";
import { StatusBadge } from "@/components/status-badge";
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle, AlertDialogTrigger } from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { jobAtAttempt } from "@/lib/attempts";
import { markJobSeen } from "@/lib/seen-jobs";
import { seriesQuery } from "@/lib/series";
import { formatBytes, relative } from "@/lib/utils";
import type { Job, JobAttempt, Node, User } from "@/types";

const terminal = new Set(["SUCCEEDED", "FAILED", "CANCELLED", "DELETED"]);
const rerunnable = new Set(["SUCCEEDED", "FAILED", "CANCELLED"]);

export function JobDetail({ user }: { user: User }) {
  const { id = "" } = useParams(), navigate = useNavigate(), queryClient = useQueryClient();
  const [selectedAttemptID, setSelectedAttemptID] = useState(""),[historyOpen,setHistoryOpen]=useState(false),[activeTab,setActiveTab]=useState("overview"),[editingDashboard,setEditingDashboard]=useState(false),[templateOpen,setTemplateOpen]=useState(false),[reportOpen,setReportOpen]=useState(false);
  const previousCurrentAttempt = useRef("");
  const job = useQuery({ queryKey: ["job", id], queryFn: () => api.job(id), refetchInterval: 3000 });
  const attempts = useQuery({ queryKey: ["job-attempts", id], queryFn: () => api.attempts(id), refetchInterval: 3000 });
  const nodes=useQuery({queryKey:["nodes"],queryFn:api.nodes,refetchInterval:3000});
  useEffect(() => {
    const currentAttempt = job.data?.attempt_id || "";
    setSelectedAttemptID(selected => currentAttempt && currentAttempt !== previousCurrentAttempt.current ? currentAttempt : selected || currentAttempt || attempts.data?.[0]?.id || "");
    previousCurrentAttempt.current = currentAttempt;
  }, [job.data?.attempt_id, attempts.data]);
  const selectedAttempt = attempts.data?.find(item => item.id === selectedAttemptID);
  useEffect(() => { if (job.data) markJobSeen(user.id, job.data); }, [job.data, user.id]);

  const stop = useMutation({ mutationFn: () => api.stopJob(id), onSuccess: () => { toast.success("Stop requested"); queryClient.invalidateQueries({ queryKey: ["job", id] }); }, onError: (error: Error) => toast.error(error.message) });
  const rerun = useMutation({ mutationFn: () => api.rerunJob(id), onSuccess: () => { toast.success("Job queued for a new attempt"); queryClient.invalidateQueries({ queryKey: ["job", id] }); queryClient.invalidateQueries({ queryKey: ["job-attempts", id] }); }, onError: (error: Error) => toast.error(error.message) });
  const remove = useMutation({ mutationFn: () => api.deleteJob(id), onSuccess: () => { toast.success("Job deletion started"); navigate("/jobs"); }, onError: (error: Error) => toast.error(error.message) });
  const attemptJob = useMemo<Job | undefined>(() => !job.data || !selectedAttempt ? job.data : jobAtAttempt(job.data, selectedAttempt), [job.data, selectedAttempt]);
  const exportQuery=useMemo(()=>attemptJob?seriesQuery(attemptJob,"all","auto",Date.now()):"",[attemptJob]);
  const buildID=managedBuildID(job.data?.spec.image??"");
  const build=useQuery({queryKey:["build",buildID],queryFn:()=>api.build(buildID!),enabled:!!buildID,retry:false});

  if (!job.data) return <p className="text-sm text-muted-foreground">Loading job…</p>;
  const current = job.data, assignedNode=nodes.data?.find(node=>node.id===(selectedAttempt?.node_id||current.assigned_node_id));
  return <Tabs value={activeTab} onValueChange={value=>{setActiveTab(value);if(value!=="metrics"){setEditingDashboard(false);setTemplateOpen(false)}}} className="relative flex h-full min-h-0 flex-col gap-5 lg:pl-10">
    <Tooltip><TooltipTrigger asChild><Button asChild variant="ghost" size="icon" className="fixed top-6 z-20 hidden size-8 -translate-x-11 lg:inline-flex"><Link to="/jobs" aria-label="Back to jobs"><ArrowLeft className="size-4"/></Link></Button></TooltipTrigger><TooltipContent side="right">Back to jobs</TooltipContent></Tooltip>
    <div className="flex flex-wrap items-start justify-between gap-3">
      <div className="min-w-0">
        <div className="flex items-center gap-3"><Button asChild variant="ghost" size="icon" className="-ml-2 size-8 lg:hidden"><Link to="/jobs" aria-label="Back to jobs"><ArrowLeft className="size-4"/></Link></Button><h1 className="truncate text-xl font-semibold">{current.spec.name}</h1><StatusBadge status={current.status}/></div>
        <div className="mt-1 flex min-w-0 items-center gap-2 font-mono text-xs text-muted-foreground"><span className="truncate">{current.id}</span><span aria-hidden className="text-border">/</span>{attempts.data?.length?<Select value={selectedAttemptID} onValueChange={setSelectedAttemptID}><SelectTrigger aria-label="Select attempt" className="h-auto w-auto shrink-0 gap-1 border-0 bg-transparent p-0 font-mono text-xs text-muted-foreground shadow-none hover:text-foreground focus:ring-0 [&>svg]:size-3"><SelectValue/></SelectTrigger><SelectContent align="start">{attempts.data.map(item=><SelectItem key={item.id} value={item.id}>Attempt {item.attempt_number}</SelectItem>)}</SelectContent></Select>:<span>No attempts yet</span>}{attempts.data?.length?<Tooltip><TooltipTrigger asChild><Button variant="ghost" size="icon" className="size-6" aria-label="Open attempt history" onClick={()=>setHistoryOpen(true)}><History className="size-3.5"/></Button></TooltipTrigger><TooltipContent>Attempt history</TooltipContent></Tooltip>:null}</div>
      </div>
      <div className="flex items-center gap-2">{activeTab==="metrics"&&<><Button variant="outline" size="sm" className="h-9" disabled={editingDashboard} onClick={()=>setReportOpen(true)}><FileDown className="size-4"/>Export report</Button><Button variant="outline" size="sm" className="h-9" onClick={()=>setTemplateOpen(true)}><LayoutTemplate className="size-4"/>Templates</Button><Button variant={editingDashboard?"secondary":"outline"} size="sm" className="h-9" onClick={()=>setEditingDashboard(value=>!value)}><PencilRuler className="size-4"/>{editingDashboard?"Done":"Edit dashboard"}</Button></>}<TabsList><TabsTrigger value="overview">Overview</TabsTrigger><TabsTrigger value="metrics" disabled={!selectedAttempt}>Metrics</TabsTrigger><TabsTrigger value="misc">Misc</TabsTrigger></TabsList></div>
    </div>

    <TabsContent value="overview" className="min-h-0 flex-1 overflow-auto"><AttemptOverview job={current} selected={selectedAttempt} node={assignedNode} buildID={build.data?buildID:undefined} buildName={build.data?.name} onStop={()=>stop.mutate()} onRerun={()=>rerun.mutate()} stopping={stop.isPending} rerunning={rerun.isPending}/></TabsContent>
    <TabsContent value="metrics" className="mt-0 min-h-0 flex-1 lg:-ml-10">{attemptJob&&selectedAttempt&&<MetricsPanel job={attemptJob} editMode={editingDashboard} templateOpen={templateOpen} onTemplateOpenChange={setTemplateOpen} reportOpen={reportOpen} onReportOpenChange={setReportOpen}/>}</TabsContent>
    <TabsContent value="misc" className="min-h-0 flex-1 overflow-auto"><MiscPanel job={current} attempt={selectedAttempt} query={exportQuery} onDelete={()=>remove.mutate()} deleting={remove.isPending}/></TabsContent>
    <AttemptHistoryDialog attempts={attempts.data??[]} open={historyOpen} onOpenChange={setHistoryOpen} onSelect={attempt=>{setSelectedAttemptID(attempt.id);setHistoryOpen(false)}}/>
  </Tabs>;
}

function AttemptOverview({job,selected,node,buildID,buildName,onStop,onRerun,stopping,rerunning}:{job:Job;selected?:JobAttempt;node?:Node;buildID?:string;buildName?:string;onStop:()=>void;onRerun:()=>void;stopping:boolean;rerunning:boolean}){
  const outputBytes=selected?.outputs.reduce((total,output)=>total+output.size,0)??0,imageName=buildName||imageReferenceName(job.spec.image);
  return <div className="space-y-4">
    <dl className="grid overflow-hidden rounded-md border sm:grid-cols-2"><Metadata label="Image" icon={<Package className="size-4"/>} value={buildID?<Link className="font-medium hover:underline" to={`/builds/${buildID}`}>{imageName}</Link>:<span className="font-medium">{imageName}</span>}/><Metadata label="Assigned node" icon={<Server className="size-4"/>} value={node?<Link className="font-medium hover:underline" to={`/nodes?node=${node.id}`}>{node.name}</Link>:<span className="text-muted-foreground">{selected?.node_id||"Unassigned"}</span>}/></dl>
    <section><h2 className="mb-2 text-sm font-semibold">Allocated resources</h2><div className="grid h-40 grid-cols-3 overflow-hidden rounded-md border"><ResourceGauge label="CPU" value={job.spec.resources.cpu_millis} total={node?.cpu_total_millis??0} display={`${(job.spec.resources.cpu_millis/1000).toFixed(2)}${node?` / ${(node.cpu_total_millis/1000).toFixed(1)}`:""} cores`} icon={Cpu}/><ResourceGauge label="Memory" value={job.spec.resources.memory_bytes} total={node?.memory_total_bytes??0} display={`${formatBytes(job.spec.resources.memory_bytes)}${node?` / ${formatBytes(node.memory_total_bytes)}`:""}`} icon={HardDrive}/><ResourceGauge label="GPU" value={job.spec.resources.gpu.count} total={node?.gpus.length??0} display={`${job.spec.resources.gpu.count}${node?` / ${node.gpus.length}`:""} devices`} icon={Server}/></div></section>
    <section className="overflow-hidden rounded-md border"><div className="flex flex-wrap items-center gap-2 border-b px-4 py-3"><div><h2 className="text-sm font-semibold">Execution summary</h2><p className="text-xs text-muted-foreground">Attempt {selected?.attempt_number??"not assigned"}</p></div><div className="ml-auto flex flex-wrap gap-2">{selected&&<Button asChild variant="outline" size="sm"><a href={`/api/v1/jobs/${job.id}/attempts/${selected.id}/archive.zip`}><Download className="size-4"/>Attempt ZIP</a></Button>}{rerunnable.has(job.status)&&<Button variant="outline" size="sm" onClick={onRerun} disabled={rerunning}><RotateCcw className="size-4"/>{job.status==="CANCELLED"?"Resume":"Rerun"}</Button>}{!terminal.has(job.status)&&job.status!=="STOPPING"&&<Button variant="outline" size="sm" onClick={onStop} disabled={stopping}><Square className="size-4"/>Stop</Button>}</div></div>
      <dl className="grid border-b sm:grid-cols-3"><Fact label="Exit code" value={selected?.exit_code??"—"}/><Fact label="Finished" value={selected?.finished_at?relative(selected.finished_at):"—"}/><Fact label="Outputs" value={`${selected?.outputs.length??0} files · ${formatBytes(outputBytes)}`}/></dl>
      {selected?.failure_reason&&<p className="border-b border-destructive/30 bg-destructive/5 p-3 text-sm text-destructive">{selected.failure_reason}</p>}
      <div className="divide-y">{selected?.outputs.length?selected.outputs.map(output=><div key={output.path} className="flex items-center gap-3 px-4 py-3 text-xs"><span className="min-w-0 flex-1 truncate font-mono">{output.path}</span><span className="hidden text-muted-foreground sm:inline">{formatBytes(output.size)} · sha256:{output.sha256.slice(0,12)}</span><Button asChild variant="ghost" size="icon" className="size-7"><a href={api.attemptOutput(job.id,selected.id,output.path)} aria-label={`Download ${output.path}`}><Download className="size-3.5"/></a></Button></div>):<p className="px-4 py-6 text-center text-sm text-muted-foreground">No output files were collected for this attempt.</p>}</div>
    </section>
    {job.spec.inputs?.length?<section><h2 className="mb-2 text-sm font-semibold">Reused immutable inputs</h2><div className="divide-y rounded-md border">{job.spec.inputs.map(input=><div key={input.path} className="grid grid-cols-[1fr_auto] gap-3 p-3 text-xs"><span className="font-mono">{input.path}</span><span className="text-muted-foreground">{formatBytes(input.size)} · sha256:{input.sha256.slice(0,12)}</span></div>)}</div></section>:null}
  </div>
}

function MiscPanel({job,attempt,query,onDelete,deleting}:{job:Job;attempt?:JobAttempt;query:string;onDelete:()=>void;deleting:boolean}){
  return <div className="space-y-4"><section className="rounded-md border"><div className="border-b px-4 py-3"><h2 className="text-sm font-semibold">Downloads</h2><p className="text-xs text-muted-foreground">Attempt-scoped telemetry and execution records.</p></div><div className="grid gap-2 p-4 sm:grid-cols-3"><Button asChild variant="outline" disabled={!attempt}><a href={attempt?api.metricsCSV(job.id,query):undefined}><FileDown className="size-4"/>Metrics CSV</a></Button><Button asChild variant="outline" disabled={!attempt}><a href={attempt?api.resourcesCSV(job.id,query):undefined}><FileDown className="size-4"/>Resources CSV</a></Button><Button asChild variant="outline" disabled={!attempt}><a href={attempt?api.eventsDownload(job.id,attempt.id):undefined}><FileDown className="size-4"/>Events JSON</a></Button></div></section>
    <section className="rounded-md border border-destructive/30"><div className="px-4 py-3"><h2 className="text-sm font-semibold">Danger zone</h2><p className="mt-1 text-xs text-muted-foreground">Deletion permanently removes attempts, logs, outputs and dashboard configuration.</p></div><div className="flex justify-end border-t px-4 py-3"><AlertDialog><AlertDialogTrigger asChild><Button variant="destructive" size="sm" disabled={!terminal.has(job.status)||deleting}><Trash2 className="size-4"/>Delete job</Button></AlertDialogTrigger><AlertDialogContent><AlertDialogHeader><AlertDialogTitle>Delete this job?</AlertDialogTitle><AlertDialogDescription>All attempts, logs and output files will be permanently removed. The audit tombstone remains.</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel>Cancel</AlertDialogCancel><AlertDialogAction onClick={onDelete}>Delete job</AlertDialogAction></AlertDialogFooter></AlertDialogContent></AlertDialog></div></section></div>
}

function AttemptHistoryDialog({attempts,open,onOpenChange,onSelect}:{attempts:JobAttempt[];open:boolean;onOpenChange:(open:boolean)=>void;onSelect:(attempt:JobAttempt)=>void}){
  return <Dialog open={open} onOpenChange={onOpenChange}><DialogContent className="max-w-5xl"><DialogHeader><DialogTitle>Attempt history</DialogTitle><DialogDescription>Select an attempt to inspect its immutable execution record.</DialogDescription></DialogHeader><div className="max-h-[65dvh] overflow-auto rounded-md border"><table className="w-full text-left text-xs"><thead className="sticky top-0 border-b bg-background text-muted-foreground"><tr><th className="p-3">Attempt</th><th className="p-3">Status</th><th className="p-3">Node</th><th className="p-3">Started</th><th className="p-3">Duration</th><th className="p-3">Exit</th><th className="p-3">Outputs</th></tr></thead><tbody>{attempts.map(attempt=><tr key={attempt.id} className="cursor-pointer border-b hover:bg-muted/40 last:border-0" tabIndex={0} onClick={()=>onSelect(attempt)} onKeyDown={event=>{if(event.key==="Enter"||event.key===" "){event.preventDefault();onSelect(attempt)}}}><td className="p-3"><div className="font-medium">#{attempt.attempt_number}</div><div className="font-mono text-[10px] text-muted-foreground">{attempt.id}</div></td><td className="p-3"><StatusBadge status={attempt.status}/></td><td className="p-3 font-mono">{attempt.node_id}</td><td className="p-3">{attempt.started_at?relative(attempt.started_at):"—"}</td><td className="p-3">{duration(attempt)}</td><td className="p-3">{attempt.exit_code??"—"}</td><td className="p-3">{attempt.outputs.length}</td></tr>)}</tbody></table></div></DialogContent></Dialog>
}

function Metadata({label,icon,value}:{label:string;icon:ReactNode;value:ReactNode}){return <div className="flex items-start gap-3 border-b p-4 sm:border-b-0 sm:border-r last:border-0"><span className="mt-0.5 text-muted-foreground">{icon}</span><div className="min-w-0"><dt className="text-xs text-muted-foreground">{label}</dt><dd className="mt-1 truncate text-sm">{value}</dd></div></div>}
function Fact({label,value}:{label:string;value:ReactNode}){return <div className="border-b p-4 sm:border-b-0 sm:border-r last:border-0"><dt className="text-xs text-muted-foreground">{label}</dt><dd className="mt-1 text-sm font-medium">{value}</dd></div>}
function managedBuildID(image:string){const match=/^jobdock:\/\/build\/([^/@]+)@sha256:[0-9a-f]{64}$/i.exec(image);return match?.[1]}
function imageReferenceName(image:string){if(image.startsWith("jobdock://build/"))return "Managed build image";return image.split("@")[0]||image}
function duration(attempt:JobAttempt){if(!attempt.started_at)return"—";const end=attempt.finished_at?Date.parse(attempt.finished_at):Date.now(),seconds=Math.max(0,Math.round((end-Date.parse(attempt.started_at))/1000));return seconds<60?`${seconds}s`:`${Math.floor(seconds/60)}m ${seconds%60}s`}
