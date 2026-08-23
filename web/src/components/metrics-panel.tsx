import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { TriangleAlert } from "lucide-react";
import { toast } from "sonner";
import { api } from "@/api";
import { DashboardTemplatePicker } from "@/components/dashboard-template-picker";
import { ObservabilityDashboard, type NumericWidgetSource } from "@/components/observability-dashboard";
import { appendMetricUpdate, appendResourceUpdate, metricPoints, resourcePoints, seriesQuery } from "@/lib/series";
import type { Job, MetricSeriesResponse, ResourceSeriesResponse, SeriesUpdate } from "@/types";
import type { DashboardWidget } from "@/lib/dashboard-widgets";

export function MetricsPanel({ job, editMode=false,templateOpen=false,onTemplateOpenChange=()=>undefined }: { job: Job;editMode?:boolean;templateOpen?:boolean;onTemplateOpenChange?:(open:boolean)=>void }) {
  const queryClient = useQueryClient();
  const saveTimer=useRef<ReturnType<typeof setTimeout>|undefined>(undefined);
  const pendingSave=useRef<{jobID:string;widgets:DashboardWidget[]}|undefined>(undefined);
  const currentWidgets=useRef<DashboardWidget[]>([]);
  const [dashboardWidgets,setDashboardWidgets]=useState<DashboardWidget[]|null|undefined>(undefined);
  const [replacement,setReplacement]=useState<{key:string;widgets:DashboardWidget[]}|undefined>(undefined);
  const [live, setLive] = useState<"connecting" | "connected" | "reconnecting">("connecting");
  const dashboard=useQuery({queryKey:["job-dashboard",job.id],queryFn:()=>api.dashboard(job.id)});
  const catalog=useQuery({queryKey:["job-metric-catalog",job.id,job.attempt_id],queryFn:()=>api.metricCatalog(job.id,job.attempt_id),enabled:dashboard.isSuccess});
  useEffect(()=>{if(dashboard.data)setDashboardWidgets(dashboard.data.widgets)},[dashboard.data]);
  const effectiveWidgets=dashboardWidgets===undefined?dashboard.data?.widgets:dashboardWidgets;
  const configuredNames=useMemo(()=>effectiveWidgets==null?null:[...new Set(effectiveWidgets.flatMap(widget=>widget.sources.filter(source=>source.kind==="metric").map(source=>source.name)))],[effectiveWidgets]);
  const query = useMemo(() => {const params=new URLSearchParams(seriesQuery(job,"all","auto",Date.now()));const names=configuredNames??catalog.data?.map(item=>item.name);if(names)for(const name of names.length?names:["__jobdock_no_metric__"])params.append("name",name);return params.toString()}, [job.id, job.attempt_id, job.started_at, job.finished_at, configuredNames, catalog.data]);
  const metrics = useQuery({ queryKey: ["job-metrics", job.id, query], queryFn: () => api.metrics(job.id, query), staleTime: 30_000,enabled:dashboard.isSuccess&&catalog.isSuccess });
  const resources = useQuery({ queryKey: ["job-resources", job.id, query], queryFn: () => api.resources(job.id, query), staleTime: 30_000 });
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
      queryClient.setQueryData<ResourceSeriesResponse>(["job-resources", job.id, query], current => appendResourceUpdate(current, update));
    };
    source.addEventListener("series", receive);
    return () => source.close();
  }, [snapshotsReady, query, job.id, job.attempt_id, job.finished_at, queryClient]);
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
  const saveDashboard=useCallback((widgets:DashboardWidget[])=>{setDashboardWidgets(widgets);if(saveTimer.current)clearTimeout(saveTimer.current);const pending={jobID:job.id,widgets};pendingSave.current=pending;saveTimer.current=setTimeout(()=>{api.saveDashboard(pending.jobID,pending.widgets).catch((error:Error)=>toast.error("Dashboard could not be saved",{description:error.message}));if(pendingSave.current===pending)pendingSave.current=undefined},400)},[job.id]);
  const replaceDashboard=useCallback(async(widgets:DashboardWidget[])=>{const previous=structuredClone(currentWidgets.current);if(saveTimer.current)clearTimeout(saveTimer.current);pendingSave.current=undefined;setDashboardWidgets(widgets);setReplacement({key:`${Date.now()}-${Math.random()}`,widgets});try{await api.saveDashboard(job.id,widgets);queryClient.setQueryData(["job-dashboard",job.id],{schema_version:1,widgets})}catch(error){setDashboardWidgets(previous);setReplacement({key:`rollback-${Date.now()}-${Math.random()}`,widgets:previous});throw error}},[job.id,queryClient]);
  const rememberWidgets=useCallback((widgets:DashboardWidget[])=>{currentWidgets.current=structuredClone(widgets)},[]);
  useEffect(()=>()=>{if(saveTimer.current)clearTimeout(saveTimer.current);const pending=pendingSave.current;if(pending){pendingSave.current=undefined;void api.saveDashboard(pending.jobID,pending.widgets).catch(()=>undefined)}},[job.id]);
  return <div className="flex h-full min-h-0 flex-col gap-2">
    {(metrics.data?.truncated || resources.data?.truncated) && <p className="flex shrink-0 items-center gap-2 rounded-md border border-amber-500/30 bg-amber-500/5 p-2 text-xs text-amber-700 dark:text-amber-300"><TriangleAlert className="size-4"/>The selected range exceeded the point limit. Choose a shorter range.</p>}
    {(dashboard.isError||catalog.isError||metrics.isError||resources.isError||progress.isError||matrices.isError||checkpoints.isError)
      ? <div role="alert" className="grid min-h-[320px] place-items-center rounded-md border border-destructive/30 text-sm text-destructive">The observability dashboard could not be loaded.</div>
      : <div className="min-h-0 flex-1"><ObservabilityDashboard jobID={job.id} attemptID={job.attempt_id??"unassigned"} ready={dashboardReady} numericSources={[...metricSeries,...resourceSeries]} progress={progress.data} matrices={matrices.data??[]} markers={markers} initialWidgets={dashboard.data?.widgets} onWidgetsChange={saveDashboard} onWidgetsReady={rememberWidgets} replacement={replacement} editMode={editMode}/></div>
    }
    {job.attempt_id&&<DashboardTemplatePicker open={templateOpen} onOpenChange={onTemplateOpenChange} jobID={job.id} attemptID={job.attempt_id} currentWidgets={currentWidgets.current} onApply={replaceDashboard}/>}
  </div>;
}

function cores(value: number) { return `${value.toFixed(2)} cores`; }
function gib(value: number) { return `${value.toFixed(2)} GiB`; }
function percent(value: number) { return `${value.toFixed(1)}%`; }
