import { useEffect, useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Radio, TriangleAlert, WifiOff } from "lucide-react";
import { api } from "@/api";
import { ObservabilityDashboard, type NumericWidgetSource } from "@/components/observability-dashboard";
import { appendMetricUpdate, appendResourceUpdate, metricPoints, resourcePoints, seriesQuery } from "@/lib/series";
import type { Job, MetricSeriesResponse, ResourceSeriesResponse, SeriesUpdate } from "@/types";

export function MetricsPanel({ job }: { job: Job }) {
  const queryClient = useQueryClient();
  const [live, setLive] = useState<"connecting" | "connected" | "reconnecting">("connecting");
  const query = useMemo(() => seriesQuery(job, "all", "auto", Date.now()), [job.id, job.attempt_id, job.started_at, job.finished_at]);
  const metrics = useQuery({ queryKey: ["job-metrics", job.id, query], queryFn: () => api.metrics(job.id, query), staleTime: 30_000 });
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
    { kind:"resource",name:"cpu",title: "CPU", points: resourcePoints(resources.data.points, point => point.cpu_millis, 1000), format: cores, color: "#3b82f6" },
    { kind:"resource",name:"memory",title: "Memory", points: resourcePoints(resources.data.points, point => point.memory_bytes, 1073741824), format: gib, color: "#8b5cf6" },
    { kind:"resource",name:"gpu-utilization",title: "GPU utilization", points: resourcePoints(resources.data.points, point => point.gpu_utilization_basis_points, 100), format: percent, color: "#f59e0b" },
    { kind:"resource",name:"gpu-memory",title: "GPU memory", points: resourcePoints(resources.data.points, point => point.gpu_memory_bytes, 1073741824), format: gib, color: "#ef4444" },
  ] : [];
  const resourceSeries=allResourceSeries.filter(item => item.points.length > 0);
  const metricSeries:NumericWidgetSource[]=(metrics.data?.series??[]).map(series=>({kind:"metric",name:series.name,title:series.unit?`${series.name} · ${series.unit}`:series.name,points:metricPoints(series.points),summary:{last:series.last,min:series.min,max:series.max}}));
  const dashboardReady=metrics.isSuccess&&resources.isSuccess&&(!job.attempt_id||(checkpoints.isSuccess&&progress.isSuccess&&matrices.isSuccess));
  const liveStatus=!job.finished_at?(live === "connected" ? <span className="ml-1 flex items-center gap-1 text-[10px] text-emerald-600"><Radio className="size-3 animate-pulse"/>Live</span> : <span className="ml-1 flex items-center gap-1 text-[10px] text-amber-600"><WifiOff className="size-3"/>{live === "connecting" ? "Connecting" : "Reconnecting"}</span>):undefined;
  return <div className="space-y-4">
    {(metrics.data?.truncated || resources.data?.truncated) && <p className="flex shrink-0 items-center gap-2 rounded-md border border-amber-500/30 bg-amber-500/5 p-2 text-xs text-amber-700 dark:text-amber-300"><TriangleAlert className="size-4"/>The selected range exceeded the point limit. Choose a shorter range.</p>}
    {(metrics.isError||resources.isError||progress.isError||matrices.isError||checkpoints.isError)
      ? <div role="alert" className="grid min-h-[320px] place-items-center rounded-md border border-destructive/30 text-sm text-destructive">The observability dashboard could not be loaded.</div>
      : <ObservabilityDashboard attemptID={job.attempt_id??"unassigned"} ready={dashboardReady} numericSources={[...metricSeries,...resourceSeries]} progress={progress.data} matrices={matrices.data??[]} markers={markers} metricsDownloadURL={api.metricsCSV(job.id,query)} resourcesDownloadURL={api.resourcesCSV(job.id,query)} live={liveStatus}/>
    }
  </div>;
}

function cores(value: number) { return `${value.toFixed(2)} cores`; }
function gib(value: number) { return `${value.toFixed(2)} GiB`; }
function percent(value: number) { return `${value.toFixed(1)}%`; }
