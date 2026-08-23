import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Copy, LayoutDashboard, MoreHorizontal, PanelLeftClose, PanelLeftOpen, Pencil, Plus, Star, Trash2, TriangleAlert } from "lucide-react";
import { toast } from "sonner";
import { api } from "@/api";
import { DashboardTemplatePicker } from "@/components/dashboard-template-picker";
import { ObservabilityDashboard, type NumericWidgetSource } from "@/components/observability-dashboard";
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { appendMetricUpdate, appendResourceUpdate, metricPoints, resourcePoints, seriesQuery } from "@/lib/series";
import type { Job, MetricSeriesResponse, ResourceSeriesResponse, SeriesUpdate } from "@/types";
import type { DashboardList, DashboardPreference, DashboardSummary, DashboardTemplateReference, DashboardWidget } from "@/lib/dashboard-widgets";

export function MetricsPanel({ job, editMode=false,templateOpen=false,onTemplateOpenChange=()=>undefined }: { job: Job;editMode?:boolean;templateOpen?:boolean;onTemplateOpenChange?:(open:boolean)=>void }) {
  const queryClient = useQueryClient();
  const saveTimer=useRef<ReturnType<typeof setTimeout>|undefined>(undefined);
  const pendingSave=useRef<{jobID:string;dashboardID:string;widgets:DashboardWidget[]}|undefined>(undefined);
  const currentWidgets=useRef<DashboardWidget[]>([]);
  const [activeDashboardID,setActiveDashboardID]=useState("");
  const [dashboardWidgets,setDashboardWidgets]=useState<DashboardWidget[]|null|undefined>(undefined);
  const [replacement,setReplacement]=useState<{key:string;widgets:DashboardWidget[]}|undefined>(undefined);
  const [live, setLive] = useState<"connecting" | "connected" | "reconnecting">("connecting");
  const dashboards=useQuery({queryKey:["job-dashboards",job.id],queryFn:()=>api.dashboards(job.id)});
  useEffect(()=>{if(dashboards.data&&!activeDashboardID)setActiveDashboardID(dashboards.data.active_dashboard_id)},[dashboards.data,activeDashboardID]);
  const dashboard=useQuery({queryKey:["job-dashboard",job.id,activeDashboardID],queryFn:()=>api.dashboardByID(job.id,activeDashboardID),enabled:!!activeDashboardID});
  const catalog=useQuery({queryKey:["job-metric-catalog",job.id,job.attempt_id],queryFn:()=>api.metricCatalog(job.id,job.attempt_id),enabled:dashboard.isSuccess});
  useEffect(()=>{setDashboardWidgets(dashboard.data?.widgets);currentWidgets.current=[];setReplacement(undefined)},[activeDashboardID,dashboard.data]);
  const effectiveWidgets=dashboardWidgets===undefined?dashboard.data?.widgets:dashboardWidgets;
  const configuredNames=useMemo(()=>effectiveWidgets==null?null:[...new Set(effectiveWidgets.flatMap(widget=>widget.sources.filter(source=>source.kind==="metric").map(source=>source.name)))],[effectiveWidgets]);
  const snapshotAt=useMemo(()=>Date.now(),[job.id,job.attempt_id,job.started_at,job.finished_at]);
  const resourceQuery=useMemo(()=>seriesQuery(job,"all","auto",snapshotAt),[job.id,job.attempt_id,job.started_at,job.finished_at,snapshotAt]);
  const query = useMemo(() => {const params=new URLSearchParams(resourceQuery);const names=configuredNames??catalog.data?.map(item=>item.name);if(names)for(const name of names.length?names:["__jobdock_no_metric__"])params.append("name",name);return params.toString()}, [configuredNames,catalog.data,resourceQuery]);
  const metrics = useQuery({ queryKey: ["job-metrics", job.id, query], queryFn: () => api.metrics(job.id, query), staleTime: 30_000,enabled:dashboard.isSuccess&&catalog.isSuccess });
  const resources = useQuery({ queryKey: ["job-resources", job.id, resourceQuery], queryFn: () => api.resources(job.id, resourceQuery), staleTime: 30_000 });
  const checkpoints=useQuery({queryKey:["job-checkpoints",job.id,job.attempt_id],queryFn:()=>api.checkpoints(job.id,job.attempt_id!),enabled:!!job.attempt_id});
  const progress=useQuery({queryKey:["job-progress",job.id,job.attempt_id],queryFn:()=>api.progress(job.id,job.attempt_id!),enabled:!!job.attempt_id});
  const matrices=useQuery({queryKey:["job-matrices",job.id,job.attempt_id],queryFn:()=>api.matrices(job.id,job.attempt_id!),enabled:!!job.attempt_id});
  const snapshotsReady = !!metrics.data && !!resources.data;
  useEffect(() => {
    if (!snapshotsReady || !job.attempt_id || job.finished_at) return;
    const after = Math.min(metrics.data!.cursor, resources.data!.cursor), source = api.openSeriesStream(job.id, job.attempt_id, after);
    setLive("connecting");
    source.onopen = () => setLive("connected"); source.onerror = () => setLive("reconnecting");
    const receive = (event: MessageEvent) => {
      const update = JSON.parse(event.data) as SeriesUpdate;
      queryClient.setQueryData<MetricSeriesResponse>(["job-metrics", job.id, query], current => appendMetricUpdate(current, update));
      queryClient.setQueryData<ResourceSeriesResponse>(["job-resources", job.id, resourceQuery], current => appendResourceUpdate(current, update));
    };
    source.addEventListener("series", receive);
    return () => source.close();
  }, [snapshotsReady, query, resourceQuery, job.id, job.attempt_id, job.finished_at, queryClient]);
  useEffect(()=>{if(!job.attempt_id||job.finished_at)return;const source=api.openObservationStream(job.id,job.attempt_id);source.addEventListener("observation",()=>{queryClient.invalidateQueries({queryKey:["job-checkpoints",job.id,job.attempt_id]});queryClient.invalidateQueries({queryKey:["job-progress",job.id,job.attempt_id]});queryClient.invalidateQueries({queryKey:["job-matrices",job.id,job.attempt_id]})});return()=>source.close()},[job.id,job.attempt_id,job.finished_at,queryClient]);
  const markers=(checkpoints.data??[]).flatMap(checkpoint=>checkpoint.timestamp?[{id:checkpoint.id,timestamp:Date.parse(checkpoint.timestamp),label:checkpoint.label||checkpoint.id.slice(0,8),step:checkpoint.step,href:`/api/v1/jobs/${job.id}/attempts/${checkpoint.attempt_id}/checkpoints/${checkpoint.id}/archive.zip`}]:[]);
  const allResourceSeries:NumericWidgetSource[] = resources.data ? [
    { kind:"resource",name:"cpu",title: "CPU", unit:"cores",points: resourcePoints(resources.data.points, point => point.cpu_millis, 1000), format: cores, color: "#3b82f6" },
    { kind:"resource",name:"memory",title: "Memory", unit:"GiB",points: resourcePoints(resources.data.points, point => point.memory_bytes, 1073741824), format: gib, color: "#8b5cf6" },
    { kind:"resource",name:"gpu-utilization",title: "GPU utilization",unit:"%", points: resourcePoints(resources.data.points, point => point.gpu_utilization_basis_points, 100), format: percent, color: "#f59e0b" },
    { kind:"resource",name:"gpu-memory",title: "GPU memory",unit:"GiB", points: resourcePoints(resources.data.points, point => point.gpu_memory_bytes, 1073741824), format: gib, color: "#ef4444" },
  ] : [];
  const resourceSeries=allResourceSeries.filter(item => item.points.length > 0);
  const metricSeries:NumericWidgetSource[]=(catalog.data??[]).map(descriptor=>{const series=metrics.data?.series.find(item=>item.name===descriptor.name);return{kind:"metric",name:descriptor.name,title:descriptor.name,unit:series?.unit||descriptor.unit||"unitless",points:series?metricPoints(series.points):[],summary:series?{last:series.last,min:series.min,max:series.max}:undefined}});
  const dashboardReady=dashboard.isSuccess&&catalog.isSuccess&&metrics.isSuccess&&resources.isSuccess&&(!job.attempt_id||(checkpoints.isSuccess&&progress.isSuccess&&matrices.isSuccess));
  const saveDashboard=useCallback((widgets:DashboardWidget[])=>{if(!activeDashboardID)return;setDashboardWidgets(widgets);if(saveTimer.current)clearTimeout(saveTimer.current);const pending={jobID:job.id,dashboardID:activeDashboardID,widgets};pendingSave.current=pending;saveTimer.current=setTimeout(()=>{api.saveDashboard(pending.jobID,pending.widgets,undefined,pending.dashboardID).then(saved=>queryClient.setQueryData(["job-dashboard",pending.jobID,pending.dashboardID],saved)).catch((error:Error)=>toast.error("Dashboard could not be saved",{description:error.message}));if(pendingSave.current===pending)pendingSave.current=undefined},400)},[activeDashboardID,job.id,queryClient]);
  const replaceDashboard=useCallback(async(widgets:DashboardWidget[],materializedFrom:DashboardTemplateReference|null)=>{if(!activeDashboardID)return;const previous=structuredClone(currentWidgets.current);if(saveTimer.current)clearTimeout(saveTimer.current);pendingSave.current=undefined;setDashboardWidgets(widgets);setReplacement({key:`${Date.now()}-${Math.random()}`,widgets});try{const saved=await api.saveDashboard(job.id,widgets,materializedFrom,activeDashboardID);queryClient.setQueryData(["job-dashboard",job.id,activeDashboardID],saved)}catch(error){setDashboardWidgets(previous);setReplacement({key:`rollback-${Date.now()}-${Math.random()}`,widgets:previous});throw error}},[activeDashboardID,job.id,queryClient]);
  const rememberWidgets=useCallback((widgets:DashboardWidget[])=>{currentWidgets.current=structuredClone(widgets)},[]);
  const selectDashboard=useCallback(async(dashboardID:string)=>{const pending=pendingSave.current;if(pending){if(saveTimer.current)clearTimeout(saveTimer.current);pendingSave.current=undefined;await api.saveDashboard(pending.jobID,pending.widgets,undefined,pending.dashboardID)}await api.updateDashboard(job.id,dashboardID,{active:true});setDashboardWidgets(undefined);setActiveDashboardID(dashboardID);queryClient.setQueryData<DashboardList>(["job-dashboards",job.id],current=>current?{...current,active_dashboard_id:dashboardID}:current)},[job.id,queryClient]);
  useEffect(()=>()=>{if(saveTimer.current)clearTimeout(saveTimer.current);const pending=pendingSave.current;if(pending){pendingSave.current=undefined;void api.saveDashboard(pending.jobID,pending.widgets,undefined,pending.dashboardID).catch(()=>undefined)}},[job.id]);
  const changeDashboard=(dashboardID:string)=>void selectDashboard(dashboardID).catch((error:Error)=>toast.error("Dashboard could not be selected",{description:error.message})),changed=(dashboardID:string)=>{setDashboardWidgets(undefined);setActiveDashboardID(dashboardID);void queryClient.invalidateQueries({queryKey:["job-dashboards",job.id]})};
  return <div className="flex h-full min-h-0 min-w-0">
    {dashboards.data&&activeDashboardID?<DashboardRail jobID={job.id} dashboards={dashboards.data} activeID={activeDashboardID} onSelect={changeDashboard} onChanged={changed}/>:<div className="w-11 shrink-0 animate-pulse border-r bg-muted/20"/>}
    <div className="flex min-h-0 min-w-0 flex-1 flex-col gap-2 pl-2">
      {dashboard.data?.fallback_reason&&<p role="status" className="flex shrink-0 items-center gap-2 rounded-md border border-amber-500/30 bg-amber-500/5 p-2 text-xs text-amber-700 dark:text-amber-300"><TriangleAlert className="size-4"/>{dashboard.data.compatibility==="partially_compatible"?"Unsupported widgets were omitted; the compatible layout remains editable.":"The saved dashboard is incompatible, so JobDock loaded the safe default layout."}</p>}
      {(metrics.data?.truncated || resources.data?.truncated) && <p className="flex shrink-0 items-center gap-2 rounded-md border border-amber-500/30 bg-amber-500/5 p-2 text-xs text-amber-700 dark:text-amber-300"><TriangleAlert className="size-4"/>The selected range exceeded the point limit. Choose a shorter range.</p>}
      {(dashboards.isError||dashboard.isError||catalog.isError||metrics.isError||resources.isError||progress.isError||matrices.isError||checkpoints.isError)
        ? <div role="alert" className="grid min-h-[320px] place-items-center rounded-md border border-destructive/30 text-sm text-destructive">The observability dashboard could not be loaded.</div>
        : <div className="min-h-0 flex-1"><ObservabilityDashboard key={activeDashboardID} jobID={job.id} attemptID={job.attempt_id??"unassigned"} ready={dashboardReady} numericSources={[...metricSeries,...resourceSeries]} progress={progress.data} matrices={matrices.data??[]} markers={markers} initialWidgets={dashboard.data?.widgets} onWidgetsChange={saveDashboard} onWidgetsReady={rememberWidgets} replacement={replacement} editMode={editMode}/></div>
      }
      {job.attempt_id&&<DashboardTemplatePicker open={templateOpen} onOpenChange={onTemplateOpenChange} jobID={job.id} attemptID={job.attempt_id} currentWidgets={currentWidgets.current} currentMaterialization={dashboard.data?.materialized_from} onApply={replaceDashboard}/>}
    </div>
  </div>;
}

function DashboardRail({jobID,dashboards,activeID,onSelect,onChanged}:{jobID:string;dashboards:DashboardList;activeID:string;onSelect:(id:string)=>void;onChanged:(id:string)=>void}){
  const [dialog,setDialog]=useState<"create"|"rename"|null>(null),[name,setName]=useState(""),[deleteOpen,setDeleteOpen]=useState(false),[busy,setBusy]=useState(false),[collapsed,setCollapsed]=useState(()=>typeof window!=="undefined"&&!!window.matchMedia?.("(max-width: 900px)").matches);
  useEffect(()=>{const media=window.matchMedia?.("(max-width: 900px)");if(!media)return;const change=(event:MediaQueryListEvent)=>{if(event.matches)setCollapsed(true)};media.addEventListener?.("change",change);return()=>media.removeEventListener?.("change",change)},[]);
  const active=dashboards.items.find(item=>item.id===activeID)??dashboards.items[0];
  const openDialog=(kind:"create"|"rename")=>{setName(kind==="rename"?active?.name??"":"");setDialog(kind)};
  const mutate=async(action:()=>Promise<unknown>,success:string,nextID:string|((result:unknown)=>string)=activeID)=>{setBusy(true);try{const result=await action();toast.success(success);setDialog(null);onChanged(typeof nextID==="function"?nextID(result):nextID)}catch(error){toast.error(error instanceof Error?error.message:"Dashboard operation failed")}finally{setBusy(false)}};
  const copyName=uniqueCopyName(active?.name??"Dashboard",dashboards.items);
  const navigate=(event:React.KeyboardEvent<HTMLButtonElement>,index:number)=>{if(!["ArrowDown","ArrowUp","Home","End"].includes(event.key))return;event.preventDefault();const last=dashboards.items.length-1,next=event.key==="Home"?0:event.key==="End"?last:event.key==="ArrowDown"?(index+1)%dashboards.items.length:(index-1+dashboards.items.length)%dashboards.items.length,target=event.currentTarget.parentElement?.querySelectorAll<HTMLButtonElement>('[role="tab"]')[next];target?.focus();onSelect(dashboards.items[next].id)};
  return <aside aria-label="Dashboard navigation" className={`flex h-full min-h-0 shrink-0 flex-col border-r pr-2 transition-[width] duration-200 ${collapsed?"w-11":"w-44"}`}>
    <div className="mb-2 flex h-8 shrink-0 items-center gap-1"><Button variant="ghost" size="icon" className="size-8 shrink-0" aria-label={collapsed?"Expand dashboard rail":"Collapse dashboard rail"} onClick={()=>setCollapsed(value=>!value)}>{collapsed?<PanelLeftOpen className="size-4"/>:<PanelLeftClose className="size-4"/>}</Button>{!collapsed&&<span className="min-w-0 flex-1 truncate text-xs font-semibold uppercase tracking-wide text-muted-foreground">Dashboards</span>}</div>
    <div role="tablist" aria-label="Job dashboards" aria-orientation="vertical" className="min-h-0 flex-1 space-y-1 overflow-y-auto overflow-x-hidden">{dashboards.items.map((item,index)=><Tooltip key={item.id}><TooltipTrigger asChild><button type="button" role="tab" aria-selected={item.id===activeID} aria-label={collapsed?item.name:undefined} tabIndex={item.id===activeID?0:-1} onClick={()=>onSelect(item.id)} onKeyDown={event=>navigate(event,index)} className={`group/rail relative flex h-9 w-full items-center overflow-hidden rounded-md text-left text-sm outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring ${item.id===activeID?"bg-accent font-medium text-foreground":"text-muted-foreground hover:bg-accent/60 hover:text-foreground"} ${collapsed?"justify-center px-0":"gap-2 px-2"}`}>{item.id===activeID&&<span className="absolute inset-y-1 left-0 w-0.5 rounded-full bg-primary"/>}<span className="grid size-6 shrink-0 place-items-center rounded text-[10px] font-semibold uppercase">{collapsed?dashboardInitials(item.name):<LayoutDashboard className="size-3.5"/>}</span>{!collapsed&&<span className="min-w-0 flex-1 truncate">{item.name}</span>}{!collapsed&&item.is_default&&<Star className="size-3 shrink-0 fill-current text-amber-500"/>}</button></TooltipTrigger><TooltipContent side="right">{item.name}{item.is_default?" · default":""}</TooltipContent></Tooltip>)}</div>
    <div className={`mt-2 flex shrink-0 border-t pt-2 ${collapsed?"flex-col":"items-center"}`}><Button variant="ghost" size="icon" className="size-8" aria-label="Create dashboard" onClick={()=>openDialog("create")}><Plus className="size-4"/></Button><DropdownMenu><DropdownMenuTrigger asChild><Button variant="ghost" size="icon" className="size-8" aria-label="Manage active dashboard"><MoreHorizontal className="size-4"/></Button></DropdownMenuTrigger><DropdownMenuContent side="right" align="end"><DropdownMenuItem onSelect={()=>openDialog("rename")}><Pencil className="size-4"/>Rename</DropdownMenuItem><DropdownMenuItem onSelect={()=>void mutate(()=>api.createDashboard(jobID,copyName,activeID),"Dashboard duplicated",result=>(result as DashboardPreference).id)}><Copy className="size-4"/>Duplicate</DropdownMenuItem><DropdownMenuItem disabled={active?.is_default} onSelect={()=>void mutate(()=>api.updateDashboard(jobID,activeID,{is_default:true}),"Default dashboard updated")}><Star className="size-4"/>Make default</DropdownMenuItem><div className="my-1 h-px bg-border"/><DropdownMenuItem className="text-destructive focus:text-destructive" disabled={dashboards.items.length===1} onSelect={()=>setDeleteOpen(true)}><Trash2 className="size-4"/>Delete</DropdownMenuItem></DropdownMenuContent></DropdownMenu></div>
    <Dialog open={dialog!==null} onOpenChange={open=>!open&&setDialog(null)}><DialogContent><DialogHeader><DialogTitle>{dialog==="rename"?"Rename dashboard":"Create dashboard"}</DialogTitle><DialogDescription>{dialog==="rename"?"Change this dashboard's display name.":"Create an independent dashboard for this job."}</DialogDescription></DialogHeader><Input autoFocus aria-label="Dashboard name" maxLength={128} value={name} onChange={event=>setName(event.target.value)} onKeyDown={event=>{if(event.key==="Enter"&&name.trim())void mutate(()=>dialog==="rename"?api.updateDashboard(jobID,activeID,{name:name.trim()}):api.createDashboard(jobID,name.trim()),dialog==="rename"?"Dashboard renamed":"Dashboard created",dialog==="rename"?activeID:result=>(result as DashboardPreference).id)}}/><DialogFooter><Button variant="outline" onClick={()=>setDialog(null)}>Cancel</Button><Button disabled={busy||!name.trim()} onClick={()=>void mutate(()=>dialog==="rename"?api.updateDashboard(jobID,activeID,{name:name.trim()}):api.createDashboard(jobID,name.trim()),dialog==="rename"?"Dashboard renamed":"Dashboard created",dialog==="rename"?activeID:result=>(result as DashboardPreference).id)}>{dialog==="rename"?"Save":"Create"}</Button></DialogFooter></DialogContent></Dialog>
    <AlertDialog open={deleteOpen} onOpenChange={setDeleteOpen}><AlertDialogContent><AlertDialogHeader><AlertDialogTitle>Delete {active?.name}?</AlertDialogTitle><AlertDialogDescription>This removes only this dashboard configuration. Job attempts and telemetry remain available.</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel>Cancel</AlertDialogCancel><AlertDialogAction disabled={busy} onClick={event=>{event.preventDefault();setBusy(true);api.deleteDashboard(jobID,activeID).then(()=>{toast.success("Dashboard deleted");setDeleteOpen(false);onChanged(dashboards.default_dashboard_id===activeID?dashboards.items.find(item=>item.id!==activeID)?.id??"":dashboards.default_dashboard_id)}).catch((error:Error)=>toast.error(error.message)).finally(()=>setBusy(false))}}>Delete</AlertDialogAction></AlertDialogFooter></AlertDialogContent></AlertDialog>
  </aside>;
}

function uniqueCopyName(name:string,items:DashboardSummary[]){const names=new Set(items.map(item=>item.name.toLowerCase()));for(let index=1;index<100;index++){const candidate=index===1?`${name} copy`:`${name} copy ${index}`;if(!names.has(candidate.toLowerCase()))return candidate}return `${name} copy ${Date.now()}`}
function dashboardInitials(name:string){const words=name.trim().split(/\s+/).filter(Boolean);return(words.length>1?words.slice(0,2).map(word=>word[0]).join(""):name.slice(0,2)).toUpperCase()||"DB"}

function cores(value: number) { return `${value.toFixed(2)} cores`; }
function gib(value: number) { return `${value.toFixed(2)} GiB`; }
function percent(value: number) { return `${value.toFixed(1)}%`; }
